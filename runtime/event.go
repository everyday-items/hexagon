package runtime

import (
	"context"
	"time"

	"github.com/hexagon-codes/ai-core/llm"
)

const EventVersionV1 = "runtime.event.v1"

// EventType identifies a runtime event.
type EventType string

const (
	EventRunStarted          EventType = "run_started"
	EventProviderSelected    EventType = "provider_selected"
	EventLLMStarted          EventType = "llm_started"
	EventLLMChunk            EventType = "llm_chunk"
	EventLLMCompleted        EventType = "llm_completed"
	EventToolCallStarted     EventType = "tool_call_started"
	EventToolCallCompleted   EventType = "tool_call_completed"
	EventToolCallFailed      EventType = "tool_call_failed"
	EventProviderFallback    EventType = "provider_fallback"
	EventBudgetChecked       EventType = "budget_checked"
	EventBudgetExceeded      EventType = "budget_exceeded"
	EventContextCompacted    EventType = "context_compacted"
	EventPermissionRequested EventType = "permission_requested"
	EventPermissionApproved  EventType = "permission_approved"
	EventPermissionDenied    EventType = "permission_denied"
	EventReasoningSanitized  EventType = "reasoning_sanitized"
	EventCheckpointSaved     EventType = "checkpoint_saved"
	EventRunFinished         EventType = "run_finished"
	EventRunFailed           EventType = "run_failed"
)

// StateSummary is a stable, low-cardinality view of runtime state for event consumers.
type StateSummary struct {
	Turn      int  `json:"turn"`
	Final     bool `json:"final"`
	Messages  int  `json:"messages"`
	ToolCalls int  `json:"tool_calls"`
}

// Redaction describes whether sensitive event payload fields were redacted.
type Redaction struct {
	Applied bool     `json:"applied,omitempty"`
	Fields  []string `json:"fields,omitempty"`
}

// Event is emitted by the runtime state machine.
type Event struct {
	Version      string
	Type         EventType
	RunID        string
	RequestID    string
	SessionID    string
	Turn         int
	Sequence     int64
	Timestamp    time.Time
	TraceID      string
	SpanID       string
	ParentSpanID string

	State        *State
	StateSummary StateSummary
	Response     *llm.CompletionResponse
	Chunk        *llm.StreamChunk
	ToolCall     *llm.ToolCall
	ToolResult   *ToolResult
	Error        error
	RuntimeError *RuntimeError
	Metadata     map[string]any
	Payload      any
	Redaction    Redaction
}

// EventSink consumes runtime events.
type EventSink interface {
	Emit(ctx context.Context, event Event) error
}

// EventSinkFunc adapts a function to EventSink.
type EventSinkFunc func(ctx context.Context, event Event) error

// Emit implements EventSink.
func (f EventSinkFunc) Emit(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

func emit(ctx context.Context, sink EventSink, event Event) error {
	if sink == nil {
		return nil
	}
	return sink.Emit(ctx, event)
}
