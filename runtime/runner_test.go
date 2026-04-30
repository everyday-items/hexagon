package runtime

import (
	"context"
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
