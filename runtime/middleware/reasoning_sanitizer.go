package middleware

import (
	"context"
	"regexp"
	"strings"

	"github.com/hexagon-codes/ai-core/llm"
	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

var (
	pairedThinking = regexp.MustCompile(`(?s)<think>.*?</think>|<thinking>.*?</thinking>|<reasoning>.*?</reasoning>`)
	tailThinking   = regexp.MustCompile(`(?s)<think>.*$|<thinking>.*$|<reasoning>.*$`)
)

// ReasoningSanitizer strips model-private reasoning tags from final content.
type ReasoningSanitizer struct{}

func (ReasoningSanitizer) BeforeLLM(context.Context, *hruntime.State) error { return nil }

func (ReasoningSanitizer) AfterLLM(ctx context.Context, state *hruntime.State, resp *llm.CompletionResponse) error {
	if resp == nil {
		return nil
	}
	original := resp.Content
	clean := pairedThinking.ReplaceAllString(resp.Content, "")
	clean = tailThinking.ReplaceAllString(clean, "")
	resp.Content = strings.TrimSpace(clean)
	if resp.Content != strings.TrimSpace(original) {
		return state.Emit(ctx, hruntime.Event{
			Type: hruntime.EventReasoningSanitized,
			Metadata: map[string]any{
				"original_bytes": len(original),
				"clean_bytes":    len(resp.Content),
			},
		})
	}
	return nil
}

func (ReasoningSanitizer) BeforeTool(context.Context, *hruntime.State, llm.ToolCall) error {
	return nil
}

func (ReasoningSanitizer) AfterTool(context.Context, *hruntime.State, llm.ToolCall, hruntime.ToolResult) error {
	return nil
}

func (ReasoningSanitizer) Finalize(context.Context, *hruntime.State) error { return nil }
