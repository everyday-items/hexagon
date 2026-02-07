package retriever

import (
	"context"
	"errors"
	"testing"

	"github.com/everyday-items/hexagon/rag"
)

// ============== Mock 实现 ==============

// mockRetriever 模拟检索器
type mockRetriever struct {
	docs []rag.Document
	err  error
}

func (m *mockRetriever) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.docs, nil
}

// mockReranker 模拟重排序器
type mockReranker struct {
	err error
}

func (m *mockReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	// 简单反转顺序作为重排序
	reranked := make([]rag.Document, len(docs))
	for i, doc := range docs {
		reranked[len(docs)-1-i] = doc
	}
	return reranked, nil
}

// ============== KeywordRetriever 测试 ==============

func TestKeywordRetriever_Basic(t *testing.T) {
	docs := []rag.Document{
		{ID: "doc1", Content: "Go is a programming language designed at Google"},
		{ID: "doc2", Content: "Python is a high-level programming language"},
		{ID: "doc3", Content: "JavaScript is widely used for web development"},
	}

	r := NewKeywordRetriever(docs)
	ctx := context.Background()

	results, err := r.Retrieve(ctx, "programming language")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}

	// 验证分数已设置
	for _, doc := range results {
		if doc.Score <= 0 {
			t.Errorf("expected positive score, got %f", doc.Score)
		}
	}
}

func TestKeywordRetriever_TopK(t *testing.T) {
	docs := []rag.Document{
		{ID: "doc1", Content: "Go programming"},
		{ID: "doc2", Content: "Python programming"},
		{ID: "doc3", Content: "JavaScript programming"},
		{ID: "doc4", Content: "Rust programming"},
	}

	r := NewKeywordRetriever(docs, WithKeywordTopK(2))
	ctx := context.Background()

	results, err := r.Retrieve(ctx, "programming")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestKeywordRetriever_AddDocuments(t *testing.T) {
	r := NewKeywordRetriever(nil)
	ctx := context.Background()

	// 初始为空
	results, err := r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) != 0 {
		t.Error("expected empty results")
	}

	// 添加文档
	r.AddDocuments([]rag.Document{
		{ID: "doc1", Content: "test document"},
	})

	results, err = r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result after adding documents")
	}
}

func TestKeywordRetriever_EmptyQuery(t *testing.T) {
	docs := []rag.Document{
		{ID: "doc1", Content: "test content"},
	}

	r := NewKeywordRetriever(docs)
	ctx := context.Background()

	results, err := r.Retrieve(ctx, "")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// 空查询应该返回空结果
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestKeywordRetriever_ContextCancellation(t *testing.T) {
	// 创建大量文档
	docs := make([]rag.Document, 1000)
	for i := 0; i < 1000; i++ {
		docs[i] = rag.Document{
			ID:      string(rune('a' + i%26)),
			Content: "test content with various keywords",
		}
	}

	r := NewKeywordRetriever(docs)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := r.Retrieve(ctx, "keywords")
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// ============== HybridRetriever 测试 ==============

func TestHybridRetriever(t *testing.T) {
	vectorDocs := []rag.Document{
		{ID: "v1", Content: "vector doc 1", Score: 0.9},
		{ID: "v2", Content: "vector doc 2", Score: 0.8},
	}
	keywordDocs := []rag.Document{
		{ID: "k1", Content: "keyword doc 1", Score: 0.85},
		{ID: "k2", Content: "keyword doc 2", Score: 0.75},
	}

	vectorRet := &mockRetriever{docs: vectorDocs}
	keywordRet := &mockRetriever{docs: keywordDocs}

	r := NewHybridRetriever(vectorRet, keywordRet,
		WithHybridTopK(3),
		WithVectorWeight(0.6),
		WithKeywordWeight(0.4),
	)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}

	// 验证分数已设置
	for _, doc := range results {
		if doc.Score <= 0 {
			t.Errorf("expected positive score, got %f", doc.Score)
		}
	}
}

// ============== MultiRetriever 测试 ==============

func TestMultiRetriever(t *testing.T) {
	ret1 := &mockRetriever{docs: []rag.Document{
		{ID: "doc1", Content: "from retriever 1", Score: 0.9},
	}}
	ret2 := &mockRetriever{docs: []rag.Document{
		{ID: "doc2", Content: "from retriever 2", Score: 0.8},
	}}

	r := NewMultiRetriever([]rag.Retriever{ret1, ret2},
		WithMultiTopK(2),
		WithDedupe(false),
	)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestMultiRetriever_Dedupe(t *testing.T) {
	// 两个检索器返回相同的文档
	ret1 := &mockRetriever{docs: []rag.Document{
		{ID: "doc1", Content: "duplicate", Score: 0.9},
		{ID: "doc2", Content: "unique1", Score: 0.8},
	}}
	ret2 := &mockRetriever{docs: []rag.Document{
		{ID: "doc1", Content: "duplicate", Score: 0.85},
		{ID: "doc3", Content: "unique2", Score: 0.7},
	}}

	// 测试启用去重
	r := NewMultiRetriever([]rag.Retriever{ret1, ret2}, WithDedupe(true))
	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// 应该只包含 3 个文档（doc1 去重）
	if len(results) != 3 {
		t.Errorf("expected 3 unique docs with dedupe, got %d", len(results))
	}

	// 测试禁用去重
	r2 := NewMultiRetriever([]rag.Retriever{ret1, ret2}, WithDedupe(false))
	results2, err := r2.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// 应该包含 4 个文档（doc1 重复）
	if len(results2) != 4 {
		t.Errorf("expected 4 docs without dedupe, got %d", len(results2))
	}
}

// ============== RerankerRetriever 测试 ==============

func TestRerankerRetriever(t *testing.T) {
	docs := []rag.Document{
		{ID: "doc1", Content: "first", Score: 0.5},
		{ID: "doc2", Content: "second", Score: 0.6},
		{ID: "doc3", Content: "third", Score: 0.7},
	}

	baseRet := &mockRetriever{docs: docs}
	reranker := &mockReranker{}

	r := NewRerankerRetriever(baseRet, reranker,
		WithRerankerTopK(2),
		WithFetchK(3),
	)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results (topK), got %d", len(results))
	}

	// 验证顺序被反转（因为 mock reranker 反转顺序）
	if results[0].ID != "doc3" {
		t.Errorf("expected first result to be doc3 after reranking, got %s", results[0].ID)
	}
}

// ============== 辅助函数测试 ==============

func TestBm25Score(t *testing.T) {
	query := "programming language"
	content := "Go is a programming language designed at Google"

	queryTerms := tokenize(query)
	score := bm25Score(queryTerms, content)

	if score <= 0 {
		t.Error("expected positive BM25 score")
	}

	// 测试不匹配的情况
	score2 := bm25Score(queryTerms, "completely different content")
	if score2 >= score {
		t.Error("expected lower score for unrelated content")
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected int // 期望的 token 数量
	}{
		{"hello world", 2},
		{"Go is great!", 3},
		{"中文分词测试", 1}, // tokenize 将连续的中文字符作为一个 token
		{"", 0},
		{"   spaces   ", 1},
		{"hello123world", 1},
	}

	for _, tt := range tests {
		tokens := tokenize(tt.input)
		if len(tokens) != tt.expected {
			t.Errorf("tokenize(%q) = %d tokens, want %d", tt.input, len(tokens), tt.expected)
		}
	}
}

func TestMatchFilter(t *testing.T) {
	metadata := map[string]any{
		"type":   "article",
		"author": "John",
		"year":   2023,
	}

	// 完全匹配
	if !matchFilter(metadata, map[string]any{"type": "article"}) {
		t.Error("expected filter to match")
	}

	// 多条件匹配
	if !matchFilter(metadata, map[string]any{"type": "article", "author": "John"}) {
		t.Error("expected multi-condition filter to match")
	}

	// 不匹配
	if matchFilter(metadata, map[string]any{"type": "blog"}) {
		t.Error("expected filter not to match")
	}

	// 空过滤器应该匹配
	if !matchFilter(metadata, map[string]any{}) {
		t.Error("expected empty filter to match")
	}

	// nil metadata 只匹配空过滤器
	if !matchFilter(nil, map[string]any{}) {
		t.Error("expected nil metadata to match empty filter")
	}
	if matchFilter(nil, map[string]any{"type": "article"}) {
		t.Error("expected nil metadata not to match non-empty filter")
	}
}
