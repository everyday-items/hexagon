package runtime

import (
	"context"

	"github.com/hexagon-codes/ai-core/llm"
)

// StreamMode controls how the runtime invokes the provider and projects output.
type StreamMode string

const (
	// StreamModeOff uses provider.Complete and only emits lifecycle events.
	StreamModeOff StreamMode = "off"
	// StreamModeEvents uses provider.Complete while still emitting runtime events.
	StreamModeEvents StreamMode = "events"
	// StreamModeTokens uses provider.Stream and emits LLMChunk events.
	StreamModeTokens StreamMode = "tokens"
)

// Request is the product-neutral input for an agent runtime run.
type Request struct {
	ID           string
	Messages     []llm.Message
	Tools        []llm.ToolDefinition
	ProviderName string
	ModelName    string
	Metadata     map[string]any
	Limits       Limits
	Strategy     Strategy
	StreamMode   StreamMode
}

// Limits constrains a runtime run.
type Limits struct {
	MaxTurns int
}

// Result is the product-neutral output of an agent runtime run.
type Result struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCallRecord
	Usage     llm.Usage
	Metadata  map[string]any
}

// State is the mutable state owned by the runtime state machine.
type State struct {
	Request Request

	Messages  []llm.Message
	ToolCalls []ToolCallRecord
	Usage     llm.Usage

	Turn       int
	Final      bool
	FinalText  string
	Reasoning  string
	Attributes map[string]any

	emit func(context.Context, Event) error
}

// ToolCallRecord records a completed tool call.
type ToolCallRecord struct {
	ID        string
	Name      string
	Arguments string
	Result    ToolResult
}

// AddUsage accumulates token usage.
func (s *State) AddUsage(u llm.Usage) {
	s.Usage.PromptTokens += u.PromptTokens
	s.Usage.CompletionTokens += u.CompletionTokens
	s.Usage.TotalTokens += u.TotalTokens
}

// Emit lets middleware publish runtime events without knowing the runner internals.
func (s *State) Emit(ctx context.Context, event Event) error {
	if s == nil || s.emit == nil {
		return nil
	}
	event.State = s
	return s.emit(ctx, event)
}
