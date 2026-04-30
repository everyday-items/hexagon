package middleware

import (
	"context"
	"fmt"

	"github.com/hexagon-codes/ai-core/llm"
	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

// Budget enforces simple token and turn budgets.
type Budget struct {
	MaxTokens int
}

func (b Budget) BeforeLLM(ctx context.Context, state *hruntime.State) error {
	_ = state.Emit(ctx, hruntime.Event{
		Type: hruntime.EventBudgetChecked,
		Metadata: map[string]any{
			"used_tokens": state.Usage.TotalTokens,
			"max_tokens":  b.MaxTokens,
		},
	})
	if b.MaxTokens > 0 && state.Usage.TotalTokens >= b.MaxTokens {
		err := fmt.Errorf("runtime budget exceeded: tokens=%d max=%d", state.Usage.TotalTokens, b.MaxTokens)
		_ = state.Emit(ctx, hruntime.Event{
			Type:  hruntime.EventBudgetExceeded,
			Error: err,
			Metadata: map[string]any{
				"used_tokens": state.Usage.TotalTokens,
				"max_tokens":  b.MaxTokens,
			},
		})
		return err
	}
	return nil
}

func (Budget) AfterLLM(context.Context, *hruntime.State, *llm.CompletionResponse) error {
	return nil
}

func (Budget) BeforeTool(context.Context, *hruntime.State, llm.ToolCall) error { return nil }

func (Budget) AfterTool(context.Context, *hruntime.State, llm.ToolCall, hruntime.ToolResult) error {
	return nil
}

func (Budget) Finalize(context.Context, *hruntime.State) error { return nil }
