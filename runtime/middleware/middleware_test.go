package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/hexagon-codes/ai-core/llm"
	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

type testProvider struct {
	responses []*llm.CompletionResponse
	calls     int
}

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) Complete(context.Context, llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if p.calls >= len(p.responses) {
		return &llm.CompletionResponse{Content: "done"}, nil
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

func (p *testProvider) Stream(context.Context, llm.CompletionRequest) (*llm.Stream, error) {
	return nil, hruntime.ErrNilStream
}

type denyPermission struct{}

func (denyPermission) CheckTool(context.Context, llm.ToolCall) error {
	return errors.New("denied")
}

type unusedToolExecutor struct {
	called bool
}

func (e *unusedToolExecutor) Execute(context.Context, llm.ToolCall) (hruntime.ToolResult, error) {
	e.called = true
	return hruntime.ToolResult{Content: "should not run"}, nil
}

func TestReasoningSanitizerEmitsEvent(t *testing.T) {
	provider := &testProvider{responses: []*llm.CompletionResponse{{Content: "<think>hidden</think>final"}}}
	runner := hruntime.NewRunner(hruntime.Config{
		ProviderSelector: hruntime.StaticProviderSelector{Provider: provider, Name: "test", Model: "m"},
		Middleware:       []hruntime.Middleware{ReasoningSanitizer{}},
	})

	var sanitized bool
	result, err := runner.RunWithSink(context.Background(), hruntime.Request{
		ID:       "run-sanitize",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, hruntime.EventSinkFunc(func(_ context.Context, event hruntime.Event) error {
		if event.Type == hruntime.EventReasoningSanitized {
			sanitized = true
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("RunWithSink error = %v", err)
	}
	if result.Content != "final" {
		t.Fatalf("Content = %q, want final", result.Content)
	}
	if !sanitized {
		t.Fatal("ReasoningSanitized event not emitted")
	}
}

func TestPermissionDenyEmitsEventAndSkipsTool(t *testing.T) {
	provider := &testProvider{responses: []*llm.CompletionResponse{{
		ToolCalls: []llm.ToolCall{{ID: "tc1", Name: "shell", Arguments: `{}`}},
	}}}
	executor := &unusedToolExecutor{}
	runner := hruntime.NewRunner(hruntime.Config{
		ProviderSelector: hruntime.StaticProviderSelector{Provider: provider, Name: "test", Model: "m"},
		ToolExecutor:     executor,
		Middleware:       []hruntime.Middleware{Permission{Checker: denyPermission{}}},
	})

	events := map[hruntime.EventType]bool{}
	_, err := runner.RunWithSink(context.Background(), hruntime.Request{
		ID:       "run-permission",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "use tool"}},
		Tools:    []llm.ToolDefinition{{Type: "function", Function: llm.ToolFunctionDef{Name: "shell"}}},
	}, hruntime.EventSinkFunc(func(_ context.Context, event hruntime.Event) error {
		events[event.Type] = true
		return nil
	}))
	if err == nil {
		t.Fatal("expected permission error")
	}
	if executor.called {
		t.Fatal("tool executor called after permission denied")
	}
	if !events[hruntime.EventPermissionRequested] || !events[hruntime.EventPermissionDenied] {
		t.Fatalf("permission events missing: %#v", events)
	}
}
