package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hexagon-codes/ai-core/llm"
)

// Runner executes agent requests using a unified state machine.
type Runner interface {
	Run(ctx context.Context, req Request) (*Result, error)
	Stream(ctx context.Context, req Request, sink EventSink) (*Result, error)
}

// Config configures DefaultRunner.
type Config struct {
	ProviderSelector ProviderSelector
	ToolExecutor     ToolExecutor
	Middleware       []Middleware
	DefaultMaxTurns  int
}

// DefaultRunner is the default runtime kernel implementation.
type DefaultRunner struct {
	selector        ProviderSelector
	toolExecutor    ToolExecutor
	middleware      []Middleware
	defaultMaxTurns int
}

// NewRunner creates a runtime runner.
func NewRunner(cfg Config) *DefaultRunner {
	maxTurns := cfg.DefaultMaxTurns
	if maxTurns <= 0 {
		maxTurns = 5
	}
	return &DefaultRunner{
		selector:        cfg.ProviderSelector,
		toolExecutor:    cfg.ToolExecutor,
		middleware:      append([]Middleware(nil), cfg.Middleware...),
		defaultMaxTurns: maxTurns,
	}
}

// Run executes a request and aggregates the final result.
func (r *DefaultRunner) Run(ctx context.Context, req Request) (*Result, error) {
	req.StreamMode = StreamModeOff
	return r.RunWithSink(ctx, req, nil)
}

// RunWithSink executes a request and also emits events to the supplied sink.
func (r *DefaultRunner) RunWithSink(ctx context.Context, req Request, sink EventSink) (*Result, error) {
	if req.StreamMode == "" {
		req.StreamMode = StreamModeEvents
	}
	return r.runStateMachine(ctx, req, sink)
}

// Stream executes the same state machine while emitting events to sink.
func (r *DefaultRunner) Stream(ctx context.Context, req Request, sink EventSink) (*Result, error) {
	if req.StreamMode == "" || req.StreamMode == StreamModeOff {
		req.StreamMode = StreamModeTokens
	}
	return r.runStateMachine(ctx, req, sink)
}

func (r *DefaultRunner) runStateMachine(ctx context.Context, req Request, sink EventSink) (*Result, error) {
	if r.selector == nil {
		return nil, ErrNoProvider
	}
	emitter := newRunEmitter(req, sink)
	strategy := strategyOrDefault(req.Strategy)
	state := &State{
		Request:    req,
		Messages:   append([]llm.Message(nil), req.Messages...),
		Attributes: map[string]any{},
		emit:       emitter.emit,
	}
	if prefix := strings.TrimSpace(strategy.BuildSystemPrefix(ctx, req)); prefix != "" {
		state.Messages = prependSystemPrefix(state.Messages, prefix)
	}

	selection, err := r.selector.Select(ctx, req)
	if err != nil {
		return nil, err
	}
	if selection.Provider == nil {
		return nil, ErrNoProvider
	}
	state.Attributes["provider"] = selection.Name
	state.Attributes["model"] = selection.Model

	if err := emitter.emit(ctx, Event{Type: EventRunStarted, State: state}); err != nil {
		return nil, err
	}
	if err := emitter.emit(ctx, Event{
		Type:  EventProviderSelected,
		State: state,
		Metadata: map[string]any{
			"provider": selection.Name,
			"model":    selection.Model,
		},
	}); err != nil {
		return nil, err
	}

	maxTurns := req.Limits.MaxTurns
	if maxTurns <= 0 {
		maxTurns = r.defaultMaxTurns
	}

	for state.Turn = 0; state.Turn < maxTurns; state.Turn++ {
		if err := ctx.Err(); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("context_canceled", err)})
			return nil, err
		}
		if err := strategy.BeforeTurn(ctx, state); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("strategy_before_turn", err)})
			return nil, err
		}
		if err := runBeforeLLM(ctx, r.middleware, state); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("middleware_before_llm", err)})
			return nil, err
		}

		callReq := llm.CompletionRequest{
			Model:    selection.Model,
			Messages: state.Messages,
			Tools:    req.Tools,
			Metadata: req.Metadata,
		}
		if err := emitter.emit(ctx, Event{
			Type:  EventLLMStarted,
			State: state,
			Metadata: map[string]any{
				"provider": selection.Name,
				"model":    selection.Model,
				"stream":   req.StreamMode == StreamModeTokens,
			},
		}); err != nil {
			return nil, err
		}
		resp, err := r.callProvider(ctx, req, selection, callReq, state, emitter)
		if err != nil {
			next, fbErr := r.selector.Fallback(ctx, selection, err)
			if fbErr == nil && next.Provider != nil {
				_ = emitter.emit(ctx, Event{
					Type:         EventProviderFallback,
					State:        state,
					Error:        err,
					RuntimeError: runtimeError("provider_call_failed", err),
					Metadata: map[string]any{
						"from": selection.Name,
						"to":   next.Name,
					},
				})
				selection = next
				state.Attributes["provider"] = selection.Name
				state.Attributes["model"] = selection.Model
				callReq.Model = selection.Model
				resp, err = r.callProvider(ctx, req, selection, callReq, state, emitter)
			}
		}
		if err != nil {
			runErr := fmt.Errorf("llm complete: %w", err)
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: runErr, RuntimeError: runtimeError("llm_complete", runErr)})
			return nil, runErr
		}

		state.AddUsage(resp.Usage)
		if err := runAfterLLM(ctx, r.middleware, state, resp); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("middleware_after_llm", err)})
			return nil, err
		}
		if err := emitter.emit(ctx, Event{Type: EventLLMCompleted, State: state, Response: resp}); err != nil {
			return nil, err
		}

		if len(resp.ToolCalls) == 0 {
			state.Final = true
			state.FinalText = resp.Content
			if err := strategy.AfterLLM(ctx, state); err != nil {
				_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("strategy_after_llm", err)})
				return nil, err
			}
			break
		}

		state.Messages = append(state.Messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: toolCallRefs(resp.ToolCalls),
		})

		if err := r.executeToolCalls(ctx, state, resp.ToolCalls, emitter); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("tool_execution", err)})
			return nil, err
		}
		if err := strategy.AfterLLM(ctx, state); err != nil {
			_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("strategy_after_llm", err)})
			return nil, err
		}
		if !strategy.ShouldContinue(ctx, state) {
			break
		}
	}

	if !state.Final {
		err := ErrMaxTurns
		_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("max_turns", err)})
		return nil, err
	}
	if err := strategy.Finalize(ctx, state); err != nil {
		_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("strategy_finalize", err)})
		return nil, err
	}
	if err := runFinalize(ctx, r.middleware, state); err != nil {
		_ = emitter.emit(ctx, Event{Type: EventRunFailed, State: state, Error: err, RuntimeError: runtimeError("middleware_finalize", err)})
		return nil, err
	}
	result := stateResult(state)
	if err := emitter.emit(ctx, Event{Type: EventRunFinished, State: state, Payload: result}); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *DefaultRunner) callProvider(ctx context.Context, req Request, selection ProviderSelection, callReq llm.CompletionRequest, state *State, emitter *runEmitter) (*llm.CompletionResponse, error) {
	if req.StreamMode != StreamModeTokens {
		return selection.Provider.Complete(ctx, callReq)
	}
	stream, err := selection.Provider.Stream(ctx, callReq)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, ErrNilStream
	}
	defer stream.Close()
	for chunk := range stream.Chunks() {
		if chunk == nil {
			continue
		}
		if chunk.Reasoning != "" {
			state.Reasoning += chunk.Reasoning
		}
		if err := emitter.emit(ctx, Event{
			Type:  EventLLMChunk,
			State: state,
			Chunk: chunk,
			Metadata: map[string]any{
				"provider": selection.Name,
				"model":    selection.Model,
			},
		}); err != nil {
			return nil, err
		}
	}
	result := stream.Result()
	if err := streamError(stream); err != nil {
		return nil, fmt.Errorf("stream read: %w", err)
	}
	if result == nil {
		return &llm.CompletionResponse{Model: selection.Model}, nil
	}
	return &llm.CompletionResponse{
		ID:           result.ID,
		Model:        result.Model,
		Content:      result.Content,
		ToolCalls:    result.ToolCalls,
		Usage:        result.Usage,
		FinishReason: result.FinishReason,
	}, nil
}

func streamError(stream *llm.Stream) error {
	if stream == nil {
		return nil
	}
	select {
	case err := <-stream.Errors():
		return err
	default:
		return nil
	}
}

func (r *DefaultRunner) executeToolCalls(ctx context.Context, state *State, calls []llm.ToolCall, emitter *runEmitter) error {
	for _, call := range calls {
		if err := runBeforeTool(ctx, r.middleware, state, call); err != nil {
			return err
		}
		if err := emitter.emit(ctx, Event{Type: EventToolCallStarted, State: state, ToolCall: &call}); err != nil {
			return err
		}
		var result ToolResult
		var err error
		if r.toolExecutor == nil {
			result = ToolResult{Content: fmt.Sprintf("Error: tool %q not available", call.Name), Error: "tool executor not configured"}
		} else {
			result, err = r.toolExecutor.Execute(ctx, call)
			if err != nil {
				result = ToolResult{Content: fmt.Sprintf("Error: %v", err), Error: err.Error()}
				if emitErr := emitter.emit(ctx, Event{Type: EventToolCallFailed, State: state, ToolCall: &call, ToolResult: &result, Error: err, RuntimeError: runtimeError("tool_call_failed", err)}); emitErr != nil {
					return emitErr
				}
			}
		}
		record := ToolCallRecord{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
			Result:    result,
		}
		state.ToolCalls = append(state.ToolCalls, record)
		state.Messages = append(state.Messages, llm.Message{
			Role:       llm.RoleTool,
			Content:    result.Content,
			ToolCallID: call.ID,
		})
		if err := runAfterTool(ctx, r.middleware, state, call, result); err != nil {
			return err
		}
		if err := emitter.emit(ctx, Event{Type: EventToolCallCompleted, State: state, ToolCall: &call, ToolResult: &result}); err != nil {
			return err
		}
	}
	return nil
}

type runEmitter struct {
	req  Request
	sink EventSink
	seq  int64
}

func newRunEmitter(req Request, sink EventSink) *runEmitter {
	return &runEmitter{req: req, sink: sink}
}

func (e *runEmitter) emit(ctx context.Context, event Event) error {
	e.seq++
	event.Version = EventVersionV1
	event.Sequence = e.seq
	event.Timestamp = time.Now()
	event.RunID = e.req.ID
	event.RequestID = requestID(e.req)
	if event.State != nil {
		event.Turn = event.State.Turn
		event.StateSummary = StateSummary{
			Turn:      event.State.Turn,
			Final:     event.State.Final,
			Messages:  len(event.State.Messages),
			ToolCalls: len(event.State.ToolCalls),
		}
	}
	return emit(ctx, e.sink, event)
}

func requestID(req Request) string {
	if req.ID != "" {
		return req.ID
	}
	if req.Metadata != nil {
		if v, ok := req.Metadata["request_id"].(string); ok {
			return v
		}
	}
	return ""
}

func stateResult(state *State) *Result {
	if state == nil {
		return nil
	}
	return &Result{
		Content:   state.FinalText,
		Reasoning: state.Reasoning,
		ToolCalls: append([]ToolCallRecord(nil), state.ToolCalls...),
		Usage:     state.Usage,
		Metadata:  state.Attributes,
	}
}

func prependSystemPrefix(messages []llm.Message, prefix string) []llm.Message {
	if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
		messages[0].Content = prefix + "\n" + messages[0].Content
		return messages
	}
	return append([]llm.Message{{Role: llm.RoleSystem, Content: prefix}}, messages...)
}

func toolCallRefs(calls []llm.ToolCall) []llm.ToolCallRef {
	refs := make([]llm.ToolCallRef, 0, len(calls))
	for _, c := range calls {
		refs = append(refs, llm.ToolCallRef{
			ID:        c.ID,
			Name:      c.Name,
			Arguments: c.Arguments,
		})
	}
	return refs
}
