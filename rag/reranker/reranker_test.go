package reranker

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/rag"
)

func TestCrossEncoderRerankerCreation(t *testing.T) {
	r := NewCrossEncoderReranker()

	if r.Name() != "CrossEncoderReranker" {
		t.Errorf("expected name 'CrossEncoderReranker', got '%s'", r.Name())
	}
}

func TestCrossEncoderRerankerWithOptions(t *testing.T) {
	r := NewCrossEncoderReranker(
		WithCrossEncoderModel("http://localhost:9000/rerank"),
		WithCrossEncoderBatchSize(16),
		WithCrossEncoderTopK(5),
	)

	if r.modelURL != "http://localhost:9000/rerank" {
		t.Errorf("expected model URL, got '%s'", r.modelURL)
	}

	if r.batchSize != 16 {
		t.Errorf("expected batchSize 16, got %d", r.batchSize)
	}

	if r.topK != 5 {
		t.Errorf("expected topK 5, got %d", r.topK)
	}
}

func TestCrossEncoderRerankerEmptyDocs(t *testing.T) {
	r := NewCrossEncoderReranker()

	docs, err := r.Rerank(context.Background(), "query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestCohereRerankerCreation(t *testing.T) {
	r := NewCohereReranker("test-api-key")

	if r.Name() != "CohereReranker" {
		t.Errorf("expected name 'CohereReranker', got '%s'", r.Name())
	}

	if r.apiKey != "test-api-key" {
		t.Errorf("expected API key 'test-api-key', got '%s'", r.apiKey)
	}
}

func TestCohereRerankerWithOptions(t *testing.T) {
	r := NewCohereReranker("key",
		WithCohereAPIKey("new-key"),
		WithCohereModel("rerank-multilingual-v3.0"),
		WithCohereTopK(20),
	)

	if r.apiKey != "new-key" {
		t.Errorf("expected API key 'new-key', got '%s'", r.apiKey)
	}

	if r.model != "rerank-multilingual-v3.0" {
		t.Errorf("expected model 'rerank-multilingual-v3.0', got '%s'", r.model)
	}

	if r.topK != 20 {
		t.Errorf("expected topK 20, got %d", r.topK)
	}
}

func TestCohereRerankerEmptyDocs(t *testing.T) {
	r := NewCohereReranker("key")

	docs, err := r.Rerank(context.Background(), "query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

// mockLLMProvider 模拟 LLM Provider
type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestLLMRerankerCreation(t *testing.T) {
	llm := &mockLLMProvider{response: "5"}
	r := NewLLMReranker(llm)

	if r.Name() != "LLMReranker" {
		t.Errorf("expected name 'LLMReranker', got '%s'", r.Name())
	}
}

func TestLLMRerankerWithOptions(t *testing.T) {
	llm := &mockLLMProvider{response: "5"}
	r := NewLLMReranker(llm,
		WithLLMRerankerTopK(15),
		WithLLMRerankerConcurrency(10),
	)

	if r.topK != 15 {
		t.Errorf("expected topK 15, got %d", r.topK)
	}

	if r.concurrency != 10 {
		t.Errorf("expected concurrency 10, got %d", r.concurrency)
	}
}

func TestLLMRerankerEmptyDocs(t *testing.T) {
	llm := &mockLLMProvider{response: "5"}
	r := NewLLMReranker(llm)

	docs, err := r.Rerank(context.Background(), "query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestLLMRerankerRerank(t *testing.T) {
	llm := &mockLLMProvider{response: "8"} // 分数 8/10 = 0.8
	r := NewLLMReranker(llm, WithLLMRerankerTopK(2))

	docs := []rag.Document{
		{ID: "1", Content: "Document 1"},
		{ID: "2", Content: "Document 2"},
		{ID: "3", Content: "Document 3"},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 docs (topK), got %d", len(result))
	}
}

func TestRRFRerankerCreation(t *testing.T) {
	r := NewRRFReranker()

	if r.Name() != "RRFReranker" {
		t.Errorf("expected name 'RRFReranker', got '%s'", r.Name())
	}
}

func TestRRFRerankerWithOptions(t *testing.T) {
	r := NewRRFReranker(
		WithRRFK(100),
		WithRRFTopK(5),
	)

	if r.k != 100 {
		t.Errorf("expected k 100, got %f", r.k)
	}

	if r.topK != 5 {
		t.Errorf("expected topK 5, got %d", r.topK)
	}
}

func TestRRFRerankerRerank(t *testing.T) {
	r := NewRRFReranker(WithRRFTopK(2))

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.9},
		{ID: "2", Content: "Doc 2", Score: 0.8},
		{ID: "3", Content: "Doc 3", Score: 0.7},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d", len(result))
	}

	// 按原始分数排序后，应该是 doc1, doc2
	if result[0].ID != "1" {
		t.Errorf("expected first doc ID '1', got '%s'", result[0].ID)
	}
}

func TestRRFFuseRankings(t *testing.T) {
	r := NewRRFReranker(WithRRFTopK(10))

	// 两个不同的排名列表
	ranking1 := []rag.Document{
		{ID: "a", Content: "Doc A"},
		{ID: "b", Content: "Doc B"},
		{ID: "c", Content: "Doc C"},
	}

	ranking2 := []rag.Document{
		{ID: "b", Content: "Doc B"},
		{ID: "c", Content: "Doc C"},
		{ID: "a", Content: "Doc A"},
	}

	result := r.FuseRankings(ranking1, ranking2)

	if len(result) != 3 {
		t.Errorf("expected 3 docs, got %d", len(result))
	}

	// doc B 在两个列表中排名都较高，应该排在前面
	// (1/(60+1) + 1/(60+1)) > (1/(60+1) + 1/(60+3))
	if result[0].ID != "b" {
		t.Errorf("expected first doc ID 'b' (highest RRF score), got '%s'", result[0].ID)
	}
}

func TestScoreRerankerCreation(t *testing.T) {
	r := NewScoreReranker()

	if r.Name() != "ScoreReranker" {
		t.Errorf("expected name 'ScoreReranker', got '%s'", r.Name())
	}
}

func TestScoreRerankerWithOptions(t *testing.T) {
	r := NewScoreReranker(
		WithScoreMin(0.5),
		WithScoreTopK(3),
		WithScoreNormalize(true),
	)

	if r.minScore != 0.5 {
		t.Errorf("expected minScore 0.5, got %f", r.minScore)
	}

	if r.topK != 3 {
		t.Errorf("expected topK 3, got %d", r.topK)
	}

	if !r.normalize {
		t.Error("expected normalize to be true")
	}
}

func TestScoreRerankerEmptyDocs(t *testing.T) {
	r := NewScoreReranker()

	docs, err := r.Rerank(context.Background(), "query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestScoreRerankerFiltering(t *testing.T) {
	r := NewScoreReranker(WithScoreMin(0.5))

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.9},
		{ID: "2", Content: "Doc 2", Score: 0.3}, // 低于阈值
		{ID: "3", Content: "Doc 3", Score: 0.7},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 docs (filtered), got %d", len(result))
	}

	for _, doc := range result {
		if doc.Score < 0.5 {
			t.Errorf("doc %s has score %f below threshold", doc.ID, doc.Score)
		}
	}
}

func TestScoreRerankerNormalize(t *testing.T) {
	r := NewScoreReranker(WithScoreNormalize(true), WithScoreTopK(10))

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.8},
		{ID: "2", Content: "Doc 2", Score: 0.4},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 归一化后，最高分应该是 1.0，最低分应该是 0.0
	if result[0].Score != 1.0 {
		t.Errorf("expected normalized max score 1.0, got %f", result[0].Score)
	}

	if result[1].Score != 0.0 {
		t.Errorf("expected normalized min score 0.0, got %f", result[1].Score)
	}
}

func TestScoreRerankerTopK(t *testing.T) {
	r := NewScoreReranker(WithScoreTopK(2))

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.9},
		{ID: "2", Content: "Doc 2", Score: 0.8},
		{ID: "3", Content: "Doc 3", Score: 0.7},
		{ID: "4", Content: "Doc 4", Score: 0.6},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 docs (topK), got %d", len(result))
	}

	if result[0].ID != "1" || result[1].ID != "2" {
		t.Error("expected docs with highest scores")
	}
}

func TestChainRerankerCreation(t *testing.T) {
	r := NewChainReranker()

	if r.Name() != "ChainReranker" {
		t.Errorf("expected name 'ChainReranker', got '%s'", r.Name())
	}
}

func TestChainRerankerRerank(t *testing.T) {
	// 创建链式重排序器：先过滤低分，再取 TopK
	r := NewChainReranker(
		NewScoreReranker(WithScoreMin(0.5), WithScoreTopK(10)),
		NewScoreReranker(WithScoreTopK(2)),
	)

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.9},
		{ID: "2", Content: "Doc 2", Score: 0.3}, // 将被过滤
		{ID: "3", Content: "Doc 3", Score: 0.7},
		{ID: "4", Content: "Doc 4", Score: 0.6},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 过滤后剩 3 个，再取 TopK=2
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d", len(result))
	}
}

func TestChainRerankerEmpty(t *testing.T) {
	r := NewChainReranker() // 空链

	docs := []rag.Document{
		{ID: "1", Content: "Doc 1", Score: 0.9},
	}

	result, err := r.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 空链应该原样返回
	if len(result) != 1 {
		t.Errorf("expected 1 doc, got %d", len(result))
	}
}

func TestRankedDocument(t *testing.T) {
	rd := RankedDocument{
		Document: rag.Document{
			ID:      "1",
			Content: "Test content",
			Score:   0.8,
		},
		RelevanceScore: 0.9,
		OriginalRank:   2,
		NewRank:        1,
	}

	if rd.ID != "1" {
		t.Errorf("expected ID '1', got '%s'", rd.ID)
	}

	if rd.RelevanceScore != 0.9 {
		t.Errorf("expected RelevanceScore 0.9, got %f", rd.RelevanceScore)
	}

	if rd.OriginalRank != 2 {
		t.Errorf("expected OriginalRank 2, got %d", rd.OriginalRank)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"hello", 5, "hello"},
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, expected %q", tc.input, tc.maxLen, result, tc.expected)
		}
	}
}
