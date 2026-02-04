package selfrag

import (
	"context"
	"testing"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/rag"
)

// mockRetriever 模拟检索器
type mockRetriever struct {
	docs []rag.Document
}

func (r *mockRetriever) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	return r.docs, nil
}

// mockCritic 模拟批评器
type mockCritic struct {
	needRetrieval       bool
	relevanceScore      float32
	faithfulnessScore   float32
	completenessScore   float32
}

func (c *mockCritic) NeedsRetrieval(ctx context.Context, query string) (bool, float32, error) {
	return c.needRetrieval, 0.9, nil
}

func (c *mockCritic) IsRelevant(ctx context.Context, query string, doc rag.Document) (bool, float32, error) {
	return c.relevanceScore >= 0.5, c.relevanceScore, nil
}

func (c *mockCritic) IsFaithful(ctx context.Context, response string, sources []rag.Document) (bool, float32, error) {
	return c.faithfulnessScore >= 0.7, c.faithfulnessScore, nil
}

func (c *mockCritic) IsComplete(ctx context.Context, query string, response string) (bool, float32, error) {
	return c.completenessScore >= 0.7, c.completenessScore, nil
}

// mockLLMProvider 模拟 LLM
type mockLLMProvider struct {
	response string
}

func (p *mockLLMProvider) Name() string { return "mock" }
func (p *mockLLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{Name: "mock"}}
}
func (p *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Content: p.response}, nil
}
func (p *mockLLMProvider) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	return nil, nil
}
func (p *mockLLMProvider) CountTokens(messages []llm.Message) (int, error) {
	return 100, nil
}

func TestNew(t *testing.T) {
	retriever := &mockRetriever{}
	provider := &mockLLMProvider{}

	selfRAG := New(retriever, provider,
		WithMaxRetries(5),
		WithRelevanceThreshold(0.8),
		WithFaithfulnessThreshold(0.9),
		WithTopK(10),
	)

	if selfRAG == nil {
		t.Fatal("New returned nil")
	}

	if selfRAG.maxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", selfRAG.maxRetries)
	}

	if selfRAG.relevanceThreshold != 0.8 {
		t.Errorf("expected relevanceThreshold=0.8, got %f", selfRAG.relevanceThreshold)
	}

	if selfRAG.faithfulnessThreshold != 0.9 {
		t.Errorf("expected faithfulnessThreshold=0.9, got %f", selfRAG.faithfulnessThreshold)
	}

	if selfRAG.topK != 10 {
		t.Errorf("expected topK=10, got %d", selfRAG.topK)
	}
}

func TestNew_DefaultValues(t *testing.T) {
	retriever := &mockRetriever{}
	provider := &mockLLMProvider{}

	selfRAG := New(retriever, provider)

	if selfRAG.maxRetries != 3 {
		t.Errorf("expected default maxRetries=3, got %d", selfRAG.maxRetries)
	}

	if selfRAG.relevanceThreshold != 0.7 {
		t.Errorf("expected default relevanceThreshold=0.7, got %f", selfRAG.relevanceThreshold)
	}

	// 应该有默认的 LLM Critic
	if selfRAG.critic == nil {
		t.Error("expected default critic")
	}
}

func TestSelfRAG_Query_WithRetrieval(t *testing.T) {
	retriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "Go 是一种编程语言"},
			{ID: "doc2", Content: "Go 由 Google 开发"},
		},
	}

	provider := &mockLLMProvider{
		response: "Go 是由 Google 开发的编程语言。",
	}

	critic := &mockCritic{
		needRetrieval:     true,
		relevanceScore:    0.9,
		faithfulnessScore: 0.85,
		completenessScore: 0.9,
	}

	selfRAG := New(retriever, provider, WithCritic(critic))

	ctx := context.Background()
	response, err := selfRAG.Query(ctx, "什么是 Go?")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Content == "" {
		t.Error("expected non-empty content")
	}

	if !response.NeedRetrieval {
		t.Error("expected NeedRetrieval=true")
	}

	if len(response.Sources) == 0 {
		t.Error("expected sources")
	}

	if response.FaithfulnessScore != 0.85 {
		t.Errorf("expected FaithfulnessScore=0.85, got %f", response.FaithfulnessScore)
	}
}

func TestSelfRAG_Query_WithoutRetrieval(t *testing.T) {
	retriever := &mockRetriever{}
	provider := &mockLLMProvider{
		response: "1 + 1 = 2",
	}

	critic := &mockCritic{
		needRetrieval:     false,
		completenessScore: 1.0,
	}

	selfRAG := New(retriever, provider, WithCritic(critic))

	ctx := context.Background()
	response, err := selfRAG.Query(ctx, "1 + 1 等于多少?")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.NeedRetrieval {
		t.Error("expected NeedRetrieval=false")
	}

	if len(response.Sources) != 0 {
		t.Error("expected no sources when retrieval not needed")
	}

	// 没有检索时忠实度应该为 1
	if response.FaithfulnessScore != 1.0 {
		t.Errorf("expected FaithfulnessScore=1.0, got %f", response.FaithfulnessScore)
	}
}

func TestSelfRAG_Query_FilterIrrelevant(t *testing.T) {
	retriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "相关内容"},
			{ID: "doc2", Content: "不相关内容"},
		},
	}

	provider := &mockLLMProvider{
		response: "回答",
	}

	// 只有分数 >= 0.5 才相关，但阈值是 0.7
	critic := &mockCritic{
		needRetrieval:     true,
		relevanceScore:    0.6, // 低于阈值
		faithfulnessScore: 0.8,
		completenessScore: 0.8,
	}

	selfRAG := New(retriever, provider,
		WithCritic(critic),
		WithRelevanceThreshold(0.7),
	)

	ctx := context.Background()
	response, err := selfRAG.Query(ctx, "问题")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// 文档应该被过滤
	if len(response.Sources) != 0 {
		t.Errorf("expected 0 sources after filtering, got %d", len(response.Sources))
	}
}

func TestLLMCritic_NeedsRetrieval(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"need_retrieval": true, "confidence": 0.9, "reason": "需要查找事实信息"}`,
	}

	critic := NewLLMCritic(provider)

	ctx := context.Background()
	needRetrieval, confidence, err := critic.NeedsRetrieval(ctx, "Go 是什么?")
	if err != nil {
		t.Fatalf("NeedsRetrieval failed: %v", err)
	}

	if !needRetrieval {
		t.Error("expected needRetrieval=true")
	}

	if confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got %f", confidence)
	}
}

func TestLLMCritic_IsRelevant(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"is_relevant": true, "score": 0.85, "reason": "文档包含相关信息"}`,
	}

	critic := NewLLMCritic(provider)

	ctx := context.Background()
	isRelevant, score, err := critic.IsRelevant(ctx, "Go 是什么?", rag.Document{Content: "Go 是编程语言"})
	if err != nil {
		t.Fatalf("IsRelevant failed: %v", err)
	}

	if !isRelevant {
		t.Error("expected isRelevant=true")
	}

	if score != 0.85 {
		t.Errorf("expected score=0.85, got %f", score)
	}
}

func TestLLMCritic_IsFaithful(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"is_faithful": true, "score": 0.95, "issues": []}`,
	}

	critic := NewLLMCritic(provider)

	ctx := context.Background()
	sources := []rag.Document{{Content: "Go 是 Google 开发的编程语言"}}
	isFaithful, score, err := critic.IsFaithful(ctx, "Go 是由 Google 开发的语言。", sources)
	if err != nil {
		t.Fatalf("IsFaithful failed: %v", err)
	}

	if !isFaithful {
		t.Error("expected isFaithful=true")
	}

	if score != 0.95 {
		t.Errorf("expected score=0.95, got %f", score)
	}
}

func TestLLMCritic_IsComplete(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"is_complete": true, "score": 0.9, "missing": []}`,
	}

	critic := NewLLMCritic(provider)

	ctx := context.Background()
	isComplete, score, err := critic.IsComplete(ctx, "什么是 Go?", "Go 是一种编程语言，由 Google 开发...")
	if err != nil {
		t.Fatalf("IsComplete failed: %v", err)
	}

	if !isComplete {
		t.Error("expected isComplete=true")
	}

	if score != 0.9 {
		t.Errorf("expected score=0.9, got %f", score)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"key": "value"}`, `{"key": "value"}`},
		{`Some text {"key": "value"} more text`, `{"key": "value"}`},
		{`no json here`, `{}`},
		{`{incomplete`, `{}`},
	}

	for _, tt := range tests {
		result := extractJSON(tt.input)
		if result != tt.expected {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		text     string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"longer text here", 10, "longer tex..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncateText(tt.text, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, result, tt.expected)
		}
	}
}
