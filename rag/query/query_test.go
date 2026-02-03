package query

import (
	"context"
	"testing"
)

// MockLLMProvider 模拟的 LLM 提供者
type MockLLMProvider struct {
	response string
	err      error
}

func (m *MockLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestQueryExpander_Expand(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		strategy string
		llmResp  string
		wantMin  int // 最少返回数量
	}{
		{
			name:     "LLM策略",
			query:    "machine learning",
			strategy: "llm",
			llmResp:  "1. ML\n2. artificial intelligence\n3. deep learning",
			wantMin:  2, // 至少包含原始查询和一个扩展
		},
		{
			name:     "同义词策略",
			query:    "AI systems",
			strategy: "synonym",
			llmResp:  "",
			wantMin:  1, // 至少包含原始查询
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockLLMProvider{response: tt.llmResp}
			expander := NewQueryExpander(mock,
				WithExpanderStrategy(tt.strategy),
				WithExpanderMaxExpansions(3),
			)

			got, err := expander.Expand(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}

			if len(got) < tt.wantMin {
				t.Errorf("Expand() returned %d queries, want at least %d", len(got), tt.wantMin)
			}

			// 应该包含原始查询
			found := false
			for _, q := range got {
				if q == tt.query {
					found = true
					break
				}
			}
			if !found && tt.strategy != "synonym" {
				t.Error("Expand() should include original query")
			}
		})
	}
}

func TestQueryRewriter_Rewrite(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		mode    string
		llmResp string
	}{
		{
			name:    "clarify模式",
			query:   "tell me about ml",
			mode:    "clarify",
			llmResp: "tell me about machine learning",
		},
		{
			name:    "simplify模式",
			query:   "can you please explain machine learning",
			mode:    "simplify",
			llmResp: "explain machine learning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockLLMProvider{response: tt.llmResp}
			rewriter := NewQueryRewriter(mock, WithRewriterMode(tt.mode))

			got, err := rewriter.Rewrite(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Rewrite() error = %v", err)
			}

			if got == "" {
				t.Error("Rewrite() returned empty string")
			}
		})
	}
}

func TestMultiQueryGenerator_Generate(t *testing.T) {
	mock := &MockLLMProvider{
		response: "1. What is machine learning?\n2. How does ML work?\n3. ML applications",
	}

	generator := NewMultiQueryGenerator(mock,
		WithNumQueries(3),
		WithIncludeSelf(true),
	)

	got, err := generator.Generate(context.Background(), "machine learning")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(got) < 2 {
		t.Errorf("Generate() returned %d queries, want at least 2", len(got))
	}

	// 应该包含原始查询
	found := false
	for _, q := range got {
		if q == "machine learning" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Generate() should include original query when includeSelf=true")
	}
}

func TestHyDEGenerator_Generate(t *testing.T) {
	mock := &MockLLMProvider{
		response: "Machine learning is a subset of artificial intelligence that enables systems to learn from data.",
	}

	generator := NewHyDEGenerator(mock,
		WithHyDEDocLength(200),
		WithHyDETemperature(0.7),
	)

	got, err := generator.Generate(context.Background(), "What is machine learning?")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if got == "" {
		t.Error("Generate() returned empty string")
	}

	if len(got) < 10 {
		t.Error("Generate() returned too short document")
	}
}

func TestQueryProcessor_Process(t *testing.T) {
	mock := &MockLLMProvider{
		response: "artificial intelligence\nmachine intelligence",
	}

	expander := NewQueryExpander(mock, WithExpanderStrategy("llm"))
	rewriter := NewQueryRewriter(mock, WithRewriterMode("clarify"))

	processor := NewQueryProcessor(
		WithExpander(expander),
		WithRewriter(rewriter),
	)

	result, err := processor.Process(context.Background(), "tell me about AI")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result == nil {
		t.Fatal("Process() returned nil")
	}

	if result.Original == "" {
		t.Error("Original query is empty")
	}

	// 应该有重写结果
	if result.Rewritten == "" {
		t.Error("Rewritten query is empty")
	}

	// 应该有扩展结果
	if len(result.Expanded) == 0 {
		t.Error("No expanded queries")
	}

	// GetAllQueries 应该返回所有查询
	allQueries := result.GetAllQueries()
	if len(allQueries) == 0 {
		t.Error("GetAllQueries() returned empty slice")
	}
}

func BenchmarkQueryExpander(b *testing.B) {
	mock := &MockLLMProvider{response: "AI\nartificial intelligence\nML"}
	expander := NewQueryExpander(mock, WithExpanderStrategy("synonym"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = expander.Expand(context.Background(), "machine learning")
	}
}
