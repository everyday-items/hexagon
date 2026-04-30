package runtime

import (
	"context"

	"github.com/hexagon-codes/ai-core/llm"
)

// Middleware observes or modifies runtime state at well-defined lifecycle points.
type Middleware interface {
	BeforeLLM(ctx context.Context, state *State) error
	AfterLLM(ctx context.Context, state *State, resp *llm.CompletionResponse) error
	BeforeTool(ctx context.Context, state *State, call llm.ToolCall) error
	AfterTool(ctx context.Context, state *State, call llm.ToolCall, result ToolResult) error
	Finalize(ctx context.Context, state *State) error
}

// MiddlewareFuncSet is a convenience middleware implementation.
type MiddlewareFuncSet struct {
	BeforeLLMFunc  func(context.Context, *State) error
	AfterLLMFunc   func(context.Context, *State, *llm.CompletionResponse) error
	BeforeToolFunc func(context.Context, *State, llm.ToolCall) error
	AfterToolFunc  func(context.Context, *State, llm.ToolCall, ToolResult) error
	FinalizeFunc   func(context.Context, *State) error
}

func (m MiddlewareFuncSet) BeforeLLM(ctx context.Context, s *State) error {
	if m.BeforeLLMFunc != nil {
		return m.BeforeLLMFunc(ctx, s)
	}
	return nil
}
func (m MiddlewareFuncSet) AfterLLM(ctx context.Context, s *State, r *llm.CompletionResponse) error {
	if m.AfterLLMFunc != nil {
		return m.AfterLLMFunc(ctx, s, r)
	}
	return nil
}
func (m MiddlewareFuncSet) BeforeTool(ctx context.Context, s *State, c llm.ToolCall) error {
	if m.BeforeToolFunc != nil {
		return m.BeforeToolFunc(ctx, s, c)
	}
	return nil
}
func (m MiddlewareFuncSet) AfterTool(ctx context.Context, s *State, c llm.ToolCall, r ToolResult) error {
	if m.AfterToolFunc != nil {
		return m.AfterToolFunc(ctx, s, c, r)
	}
	return nil
}
func (m MiddlewareFuncSet) Finalize(ctx context.Context, s *State) error {
	if m.FinalizeFunc != nil {
		return m.FinalizeFunc(ctx, s)
	}
	return nil
}

func runBeforeLLM(ctx context.Context, mw []Middleware, state *State) error {
	for _, m := range mw {
		if err := m.BeforeLLM(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

func runAfterLLM(ctx context.Context, mw []Middleware, state *State, resp *llm.CompletionResponse) error {
	for _, m := range mw {
		if err := m.AfterLLM(ctx, state, resp); err != nil {
			return err
		}
	}
	return nil
}

func runBeforeTool(ctx context.Context, mw []Middleware, state *State, call llm.ToolCall) error {
	for _, m := range mw {
		if err := m.BeforeTool(ctx, state, call); err != nil {
			return err
		}
	}
	return nil
}

func runAfterTool(ctx context.Context, mw []Middleware, state *State, call llm.ToolCall, result ToolResult) error {
	for _, m := range mw {
		if err := m.AfterTool(ctx, state, call, result); err != nil {
			return err
		}
	}
	return nil
}

func runFinalize(ctx context.Context, mw []Middleware, state *State) error {
	for _, m := range mw {
		if err := m.Finalize(ctx, state); err != nil {
			return err
		}
	}
	return nil
}
