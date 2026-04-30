package runtime

import (
	"context"

	"github.com/hexagon-codes/ai-core/llm"
)

// Provider is the LLM port used by the runtime.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error)
}

// ProviderSelection describes the selected model backend.
type ProviderSelection struct {
	Provider Provider
	Name     string
	Model    string
}

// ProviderSelector selects the primary provider and optional fallback.
type ProviderSelector interface {
	Select(ctx context.Context, req Request) (ProviderSelection, error)
	Fallback(ctx context.Context, failed ProviderSelection, err error) (ProviderSelection, error)
}

// StaticProviderSelector is a single-provider selector.
type StaticProviderSelector struct {
	Provider Provider
	Name     string
	Model    string
}

// Select returns the configured provider.
func (s StaticProviderSelector) Select(context.Context, Request) (ProviderSelection, error) {
	name := s.Name
	if name == "" && s.Provider != nil {
		name = s.Provider.Name()
	}
	return ProviderSelection{Provider: s.Provider, Name: name, Model: s.Model}, nil
}

// Fallback returns no fallback.
func (s StaticProviderSelector) Fallback(context.Context, ProviderSelection, error) (ProviderSelection, error) {
	return ProviderSelection{}, ErrNoFallback
}

// ToolExecutor executes tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, call llm.ToolCall) (ToolResult, error)
}

// ToolResult is a product-neutral tool result.
type ToolResult struct {
	Content string
	Raw     any
	Error   string
}

// Permission checks whether a tool call can execute.
type Permission interface {
	CheckTool(ctx context.Context, call llm.ToolCall) error
}

// Memory is the optional long-term memory port.
type Memory interface {
	Search(ctx context.Context, query string, limit int) ([]MemoryEntry, error)
	Save(ctx context.Context, entry MemoryEntry) error
}

// MemoryEntry is a product-neutral memory record.
type MemoryEntry struct {
	Role    string
	Content string
}

// CheckpointStore persists runtime state snapshots.
type CheckpointStore interface {
	Save(ctx context.Context, state State) (string, error)
	Load(ctx context.Context, id string) (*State, error)
}
