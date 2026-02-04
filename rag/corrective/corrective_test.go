package corrective

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

// mockEvaluator 模拟评估器
type mockEvaluator struct {
	results map[string]EvaluationResult
	scores  map[string]float32
}

func (e *mockEvaluator) Evaluate(ctx context.Context, query string, doc rag.Document) (EvaluationResult, float32, error) {
	if result, ok := e.results[doc.ID]; ok {
		return result, e.scores[doc.ID], nil
	}
	return ResultAmbiguous, 0.5, nil
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

	crag := New(retriever,
		WithLLM(provider),
		WithRelevanceThreshold(0.6),
		WithAmbiguousThreshold(0.8),
		WithTopK(10),
	)

	if crag == nil {
		t.Fatal("New returned nil")
	}

	if crag.relevanceThreshold != 0.6 {
		t.Errorf("expected relevanceThreshold=0.6, got %f", crag.relevanceThreshold)
	}

	if crag.ambiguousThreshold != 0.8 {
		t.Errorf("expected ambiguousThreshold=0.8, got %f", crag.ambiguousThreshold)
	}

	if crag.topK != 10 {
		t.Errorf("expected topK=10, got %d", crag.topK)
	}
}

func TestNew_DefaultValues(t *testing.T) {
	retriever := &mockRetriever{}

	crag := New(retriever)

	if crag.relevanceThreshold != 0.5 {
		t.Errorf("expected default relevanceThreshold=0.5, got %f", crag.relevanceThreshold)
	}

	if crag.ambiguousThreshold != 0.7 {
		t.Errorf("expected default ambiguousThreshold=0.7, got %f", crag.ambiguousThreshold)
	}

	if crag.topK != 5 {
		t.Errorf("expected default topK=5, got %d", crag.topK)
	}
}

func TestCorrectiveRAG_Retrieve_AllCorrect(t *testing.T) {
	primaryRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "相关文档1", Score: 0.9},
			{ID: "doc2", Content: "相关文档2", Score: 0.85},
		},
	}

	evaluator := &mockEvaluator{
		results: map[string]EvaluationResult{
			"doc1": ResultCorrect,
			"doc2": ResultCorrect,
		},
		scores: map[string]float32{
			"doc1": 0.9,
			"doc2": 0.85,
		},
	}

	crag := New(primaryRetriever, WithEvaluator(evaluator))

	ctx := context.Background()
	docs, err := crag.Retrieve(ctx, "测试查询")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("expected 2 docs, got %d", len(docs))
	}
}

func TestCorrectiveRAG_Retrieve_WithFallback(t *testing.T) {
	primaryRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "不相关文档", Score: 0.3},
		},
	}

	fallbackRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "web1", Content: "网络搜索结果", Score: 0.8},
			{ID: "web2", Content: "另一个结果", Score: 0.75},
		},
	}

	evaluator := &mockEvaluator{
		results: map[string]EvaluationResult{
			"doc1": ResultIncorrect,
		},
		scores: map[string]float32{
			"doc1": 0.2,
		},
	}

	crag := New(primaryRetriever,
		WithFallbackRetriever(fallbackRetriever),
		WithEvaluator(evaluator),
	)

	ctx := context.Background()
	docs, err := crag.Retrieve(ctx, "测试查询")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// 应该返回备选检索结果
	if len(docs) != 2 {
		t.Errorf("expected 2 fallback docs, got %d", len(docs))
	}

	// 检查是否是备选结果
	if len(docs) > 0 && docs[0].ID != "web1" && docs[0].ID != "web2" {
		t.Error("expected fallback docs")
	}
}

func TestCorrectiveRAG_Retrieve_MixedResults(t *testing.T) {
	primaryRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "相关", Score: 0.9},
			{ID: "doc2", Content: "模糊", Score: 0.6},
			{ID: "doc3", Content: "不相关", Score: 0.3},
		},
	}

	fallbackRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "web1", Content: "补充", Score: 0.8},
		},
	}

	evaluator := &mockEvaluator{
		results: map[string]EvaluationResult{
			"doc1": ResultCorrect,
			"doc2": ResultAmbiguous,
			"doc3": ResultIncorrect,
		},
		scores: map[string]float32{
			"doc1": 0.9,
			"doc2": 0.6,
			"doc3": 0.3,
		},
	}

	crag := New(primaryRetriever,
		WithFallbackRetriever(fallbackRetriever),
		WithEvaluator(evaluator),
	)

	ctx := context.Background()
	docs, err := crag.Retrieve(ctx, "测试查询")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// 应该包含正确和模糊的结果
	if len(docs) < 2 {
		t.Errorf("expected at least 2 docs, got %d", len(docs))
	}
}

func TestCorrectiveRAG_Retrieve_EmptyPrimary(t *testing.T) {
	primaryRetriever := &mockRetriever{
		docs: []rag.Document{},
	}

	fallbackRetriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "web1", Content: "备选结果", Score: 0.8},
		},
	}

	crag := New(primaryRetriever, WithFallbackRetriever(fallbackRetriever))

	ctx := context.Background()
	docs, err := crag.Retrieve(ctx, "测试查询")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("expected 1 fallback doc, got %d", len(docs))
	}
}

func TestCorrectiveRAG_EvaluateByScore(t *testing.T) {
	crag := &CorrectiveRAG{
		relevanceThreshold: 0.5,
		ambiguousThreshold: 0.7,
	}

	tests := []struct {
		score    float32
		expected EvaluationResult
	}{
		{0.8, ResultCorrect},
		{0.7, ResultCorrect},
		{0.6, ResultAmbiguous},
		{0.5, ResultAmbiguous},
		{0.4, ResultIncorrect},
		{0.2, ResultIncorrect},
	}

	for _, tt := range tests {
		result := crag.evaluateResultByScore(tt.score)
		if result != tt.expected {
			t.Errorf("evaluateResultByScore(%f) = %s, want %s", tt.score, result, tt.expected)
		}
	}
}

func TestCorrectiveRAG_MergeResults(t *testing.T) {
	crag := &CorrectiveRAG{topK: 5}

	correct := []rag.Document{
		{ID: "c1", Content: "correct1", Metadata: make(map[string]any)},
	}
	ambiguous := []rag.Document{
		{ID: "a1", Content: "ambiguous1", Metadata: make(map[string]any)},
	}
	fallback := []rag.Document{
		{ID: "f1", Content: "fallback1", Metadata: make(map[string]any)},
		{ID: "c1", Content: "duplicate", Metadata: make(map[string]any)}, // 重复 ID
	}

	merged := crag.mergeResults(correct, ambiguous, fallback)

	// 应该去重
	if len(merged) != 3 {
		t.Errorf("expected 3 docs after dedup, got %d", len(merged))
	}

	// 检查顺序：correct -> fallback -> ambiguous
	expectedOrder := []string{"c1", "f1", "a1"}
	for i, doc := range merged {
		if doc.ID != expectedOrder[i] {
			t.Errorf("position %d: expected %s, got %s", i, expectedOrder[i], doc.ID)
		}
	}
}

func TestLLMEvaluator(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"evaluation": "correct", "score": 0.85, "reason": "文档相关"}`,
	}

	evaluator := NewLLMEvaluator(provider)

	ctx := context.Background()
	result, score, err := evaluator.Evaluate(ctx, "什么是 Go?", rag.Document{Content: "Go 是编程语言"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result != ResultCorrect {
		t.Errorf("expected ResultCorrect, got %s", result)
	}

	if score != 0.85 {
		t.Errorf("expected score=0.85, got %f", score)
	}
}

func TestLLMQueryRewriter(t *testing.T) {
	provider := &mockLLMProvider{
		response: "Go programming language features",
	}

	rewriter := NewLLMQueryRewriter(provider)

	ctx := context.Background()
	rewritten, err := rewriter.Rewrite(ctx, "Go 有什么特点啊?")
	if err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	if rewritten == "" {
		t.Error("expected non-empty rewritten query")
	}

	if rewritten == "Go 有什么特点啊?" {
		t.Error("expected query to be rewritten")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"key": "value"}`, `{"key": "value"}`},
		{`text {"key": "value"} more`, `{"key": "value"}`},
		{`no json`, `{}`},
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
		{"longer text", 5, "longe..."},
	}

	for _, tt := range tests {
		result := truncateText(tt.text, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, result, tt.expected)
		}
	}
}
