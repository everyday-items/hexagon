package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hexagon-codes/ai-core/llm"
)

type fakeProvider struct {
	name        string
	responses   []*llm.CompletionResponse
	stream      string
	streamInput *closeTrackingReader
	calls       int
	lastRequest llm.CompletionRequest
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	p.lastRequest = req
	if p.calls >= len(p.responses) {
		return &llm.CompletionResponse{Content: "done"}, nil
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

func (p *fakeProvider) Stream(_ context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	p.lastRequest = req
	if p.streamInput != nil {
		return llm.NewStream(p.streamInput, llm.StreamOpenAIFormat), nil
	}
	return llm.NewStream(strings.NewReader(p.stream), llm.StreamOpenAIFormat), nil
}

type closeTrackingReader struct {
	*strings.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

type fakeToolExecutor struct {
	calls int
}

func TestRunnerStreamEmitsChunksAndAggregatesResult(t *testing.T) {
	provider := &fakeProvider{
		name: "fake",
		stream: strings.Join([]string{
			`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"reasoning":"think "}}]}`,
			`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"content":"hel"}}]}`,
			`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
			"",
		}, "\n"),
	}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
	})

	var events []Event
	result, err := runner.Stream(context.Background(), Request{
		ID:       "run-1",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, EventSinkFunc(func(_ context.Context, event Event) error {
		events = append(events, event)
		return nil
	}))
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("Content = %q, want hello", result.Content)
	}
	if result.Reasoning != "think " {
		t.Fatalf("Reasoning = %q, want think", result.Reasoning)
	}
	var chunks int
	for i, event := range events {
		if event.Version != EventVersionV1 {
			t.Fatalf("event[%d].Version = %q", i, event.Version)
		}
		if event.Sequence != int64(i+1) {
			t.Fatalf("event[%d].Sequence = %d, want %d", i, event.Sequence, i+1)
		}
		if event.RequestID != "run-1" {
			t.Fatalf("event[%d].RequestID = %q, want run-1", i, event.RequestID)
		}
		if event.Type == EventLLMChunk {
			chunks++
		}
	}
	if chunks != 4 {
		t.Fatalf("LLMChunk events = %d, want 4", chunks)
	}
	if events[len(events)-1].Type != EventRunFinished {
		t.Fatalf("last event = %s, want %s", events[len(events)-1].Type, EventRunFinished)
	}
}

func TestRunnerStreamClosesProviderStream(t *testing.T) {
	reader := &closeTrackingReader{Reader: strings.NewReader(strings.Join([]string{
		`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
		"",
	}, "\n"))}
	provider := &fakeProvider{name: "fake", streamInput: reader}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
	})

	if _, err := runner.Stream(context.Background(), Request{
		ID:       "run-close",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if !reader.closed {
		t.Fatal("provider stream reader was not closed")
	}
}

func TestRunnerStreamReturnsParseErrors(t *testing.T) {
	provider := &fakeProvider{
		name: "fake",
		stream: strings.Join([]string{
			`data: {"id":"c1","model":"fake-model","choices":[{"delta":{"content":"partial"}}]}`,
			`data: {not-json}`,
			`data: [DONE]`,
			"",
		}, "\n"),
	}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
	})

	if _, err := runner.Stream(context.Background(), Request{
		ID:       "run-bad-stream",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, nil); err == nil {
		t.Fatal("Stream() error = nil, want parse error")
	}
}

func (e *fakeToolExecutor) Execute(_ context.Context, call llm.ToolCall) (ToolResult, error) {
	e.calls++
	return ToolResult{Content: `{"ok":true}`}, nil
}

func TestRunnerNoToolPath(t *testing.T) {
	provider := &fakeProvider{name: "fake", responses: []*llm.CompletionResponse{{Content: "hello"}}}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
	})

	result, err := runner.Run(context.Background(), Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("Content = %q, want hello", result.Content)
	}
}

type testPrefixStrategy struct{}

func (testPrefixStrategy) Name() string                                      { return "test-prefix" }
func (testPrefixStrategy) BuildSystemPrefix(context.Context, Request) string { return "runtime prefix" }
func (testPrefixStrategy) BeforeTurn(context.Context, *State) error          { return nil }
func (testPrefixStrategy) AfterLLM(context.Context, *State) error            { return nil }
func (testPrefixStrategy) ShouldContinue(context.Context, *State) bool       { return false }
func (testPrefixStrategy) Finalize(context.Context, *State) error            { return nil }

func TestRunnerPrependsStrategyPrefixWhenNoSystemMessage(t *testing.T) {
	provider := &fakeProvider{name: "fake", responses: []*llm.CompletionResponse{{Content: "hello"}}}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
	})

	if _, err := runner.Run(context.Background(), Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Strategy: testPrefixStrategy{},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(provider.lastRequest.Messages) == 0 {
		t.Fatal("provider received no messages")
	}
	first := provider.lastRequest.Messages[0]
	if first.Role != llm.RoleSystem {
		t.Fatalf("first role = %s, want system", first.Role)
	}
	if first.Content != "runtime prefix" {
		t.Fatalf("first content = %q, want runtime prefix", first.Content)
	}
}

func TestRunnerToolLoop(t *testing.T) {
	provider := &fakeProvider{name: "fake", responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "echo", Arguments: `{"x":1}`}}},
		{Content: "final"},
	}}
	executor := &fakeToolExecutor{}
	runner := NewRunner(Config{
		ProviderSelector: StaticProviderSelector{Provider: provider, Name: "fake", Model: "fake-model"},
		ToolExecutor:     executor,
	})

	result, err := runner.Run(context.Background(), Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "use tool"}},
		Tools:    []llm.ToolDefinition{{Type: "function", Function: llm.ToolFunctionDef{Name: "echo"}}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if executor.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", executor.calls)
	}
	if result.Content != "final" {
		t.Fatalf("Content = %q, want final", result.Content)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "echo" {
		t.Fatalf("ToolCalls = %#v, want echo", result.ToolCalls)
	}
}

type continuationProvider struct {
	complete func(llm.CompletionRequest) (*llm.CompletionResponse, error)
	stream   func(llm.CompletionRequest) (*llm.Stream, error)
}

func (*continuationProvider) Name() string { return "continuation" }
func (p *continuationProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return p.complete(req)
}
func (p *continuationProvider) Stream(_ context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	return p.stream(req)
}

type continuationReadFailure struct{}

func (continuationReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("503 interrupted model stream")
}

// 同步和流式续接均保留同次工具事实、原有用量及已显示前缀，并使用新模型的输入策略。
func TestRunnerContinuePreservesCompletedState(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "stream"}[streaming], func(t *testing.T) {
			primaryCalls := 0
			primary := &continuationProvider{}
			primary.complete = func(llm.CompletionRequest) (*llm.CompletionResponse, error) {
				primaryCalls++
				if primaryCalls == 1 {
					return &llm.CompletionResponse{ToolCalls: []llm.ToolCall{{ID: "one", Name: "write_once", Arguments: "{}"}}, Usage: llm.Usage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}}, nil
				}
				return nil, errors.New("503 interrupted model")
			}
			primary.stream = func(llm.CompletionRequest) (*llm.Stream, error) {
				primaryCalls++
				if primaryCalls == 1 {
					return llm.NewStream(strings.NewReader("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"one\",\"type\":\"function\",\"function\":{\"name\":\"write_once\",\"arguments\":\"{}\"}}]}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\ndata: [DONE]\n\n"), llm.StreamOpenAIFormat), nil
				}
				return llm.NewStream(io.MultiReader(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_\"}}]}\n\n"), continuationReadFailure{}), llm.StreamOpenAIFormat), nil
			}
			tools := &fakeToolExecutor{}
			runner := NewRunner(Config{ProviderSelector: StaticProviderSelector{Provider: primary}, ToolExecutor: tools})
			request := Request{ID: "same-run", Messages: []llm.Message{llm.SystemMessage("local-only-policy"), llm.UserMessage("current task")}}
			var visible strings.Builder
			sink := EventSinkFunc(func(_ context.Context, e Event) error {
				if e.Chunk != nil {
					visible.WriteString(e.Chunk.Content)
				}
				return nil
			})
			var err error
			if streaming {
				_, err = runner.Stream(t.Context(), request, sink)
			} else {
				_, err = runner.Run(t.Context(), request)
			}
			var interrupted *ProviderCallError
			if !errors.As(err, &interrupted) {
				t.Fatalf("provider progress missing: %v", err)
			}
			check := func(req llm.CompletionRequest) {
				t.Helper()
				found := false
				for _, m := range req.Messages {
					if strings.Contains(m.Content, "local-only-policy") {
						t.Fatal("old provider input leaked")
					}
					if m.Role == llm.RoleTool {
						found = true
					}
				}
				if !found {
					t.Fatal("completed tool result missing")
				}
			}
			backup := &continuationProvider{complete: func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
				check(req)
				return &llm.CompletionResponse{Content: "DONE", Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}}, nil
			}, stream: func(req llm.CompletionRequest) (*llm.Stream, error) {
				check(req)
				return llm.NewStream(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"PART\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"IAL_DONE\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"), llm.StreamOpenAIFormat), nil
			}}
			continued := NewRunner(Config{ProviderSelector: StaticProviderSelector{Provider: backup}, ToolExecutor: tools})
			request.Messages = []llm.Message{llm.SystemMessage("new provider policy"), llm.UserMessage("current task")}
			request.StreamMode = StreamModeOff
			if streaming {
				request.StreamMode = StreamModeTokens
			}
			result, err := continued.Continue(t.Context(), request, interrupted, sink)
			if err != nil || result == nil {
				t.Fatalf("continue: %+v %v", result, err)
			}
			want := "DONE"
			if streaming {
				want = "PARTIAL_DONE"
				if visible.String() != want {
					t.Fatalf("visible stream repeated or lost prefix: %q", visible.String())
				}
			}
			if result.Content != want || tools.calls != 1 || len(result.ToolCalls) != 1 || result.Usage.TotalTokens != 15 {
				t.Fatalf("continuation lost progress: result=%+v tool_calls=%d", result, tools.calls)
			}
		})
	}
}
