package middleware

import (
	"context"

	"github.com/hexagon-codes/ai-core/llm"
	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

// Permission delegates tool checks to a runtime permission port.
type Permission struct {
	Checker hruntime.Permission
}

func (Permission) BeforeLLM(context.Context, *hruntime.State) error { return nil }

func (Permission) AfterLLM(context.Context, *hruntime.State, *llm.CompletionResponse) error {
	return nil
}

func (p Permission) BeforeTool(ctx context.Context, state *hruntime.State, call llm.ToolCall) error {
	_ = state.Emit(ctx, hruntime.Event{
		Type:     hruntime.EventPermissionRequested,
		ToolCall: &call,
	})
	if p.Checker == nil {
		_ = state.Emit(ctx, hruntime.Event{
			Type:     hruntime.EventPermissionApproved,
			ToolCall: &call,
			Metadata: map[string]any{
				"checker": "none",
			},
		})
		return nil
	}
	if err := p.Checker.CheckTool(ctx, call); err != nil {
		_ = state.Emit(ctx, hruntime.Event{
			Type:     hruntime.EventPermissionDenied,
			ToolCall: &call,
			Error:    err,
		})
		return err
	}
	return state.Emit(ctx, hruntime.Event{
		Type:     hruntime.EventPermissionApproved,
		ToolCall: &call,
	})
}

func (Permission) AfterTool(context.Context, *hruntime.State, llm.ToolCall, hruntime.ToolResult) error {
	return nil
}

func (Permission) Finalize(context.Context, *hruntime.State) error { return nil }
