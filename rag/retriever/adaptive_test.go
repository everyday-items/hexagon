package retriever

import (
	"context"
	"errors"
	"testing"

	"github.com/everyday-items/hexagon/rag"
)

// ============== RuleClassifier 测试 ==============

func TestRuleClassifier_SimpleQuery(t *testing.T) {
	classifier := NewRuleClassifier()
	ctx := context.Background()

	tests := []struct {
		query      string
		complexity QueryComplexity
	}{
		{"Go", ComplexitySimple},
		{"What is Go?", ComplexitySimple},
		{"简单查询", ComplexitySimple},
	}

	for _, tt := range tests {
		result, err := classifier.Classify(ctx, tt.query)
		if err != nil {
			t.Fatalf("Classify failed for %q: %v", tt.query, err)
		}

		if result.Complexity != tt.complexity {
			t.Errorf("query %q: expected complexity %v, got %v",
				tt.query, tt.complexity, result.Complexity)
		}

		if len(result.Keywords) == 0 {
			t.Errorf("expected keywords to be extracted for %q", tt.query)
		}

		if result.Score <= 0 || result.Score > 1 {
			t.Errorf("expected score between 0 and 1, got %f", result.Score)
		}
	}
}

func TestRuleClassifier_ComplexQuery(t *testing.T) {
	classifier := NewRuleClassifier()
	ctx := context.Background()

	complexQueries := []string{
		"How does Go's garbage collector work and what are the trade-offs compared to manual memory management?",
		"为什么 Go 语言选择了 CSP 模型而不是传统的共享内存并发模型？这样做的优势和劣势分别是什么？",
		"Explain the impact of Go's interface design on software architecture? What are the implications?",
	}

	for _, query := range complexQueries {
		result, err := classifier.Classify(ctx, query)
		if err != nil {
			t.Fatalf("Classify failed for %q: %v", query, err)
		}

		if result.Complexity != ComplexityComplex {
			t.Errorf("query %q: expected ComplexityComplex, got %v",
				query, result.Complexity)
		}
	}
}

func TestRuleClassifier_ComparativeQuery(t *testing.T) {
	classifier := NewRuleClassifier()
	ctx := context.Background()

	comparativeQueries := []string{
		"Go vs Rust performance comparison",
		"比较 Python 和 Java 的优缺点",
		"What's the difference between channels and mutexes?",
		"对比 gRPC 和 REST API",
	}

	for _, query := range comparativeQueries {
		result, err := classifier.Classify(ctx, query)
		if err != nil {
			t.Fatalf("Classify failed for %q: %v", query, err)
		}

		if result.Type != QueryTypeComparative {
			t.Errorf("query %q: expected QueryTypeComparative, got %v",
				query, result.Type)
		}
	}
}

func TestRuleClassifier_AggregationQuery(t *testing.T) {
	classifier := NewRuleClassifier()
	ctx := context.Background()

	aggregationQueries := []string{
		"List all Go web frameworks",
		"列出所有支持的数据库",
		"How many goroutines can run concurrently?",
		"哪些公司在使用 Go 语言？",
	}

	for _, query := range aggregationQueries {
		result, err := classifier.Classify(ctx, query)
		if err != nil {
			t.Fatalf("Classify failed for %q: %v", query, err)
		}

		if result.Type != QueryTypeAggregation {
			t.Errorf("query %q: expected QueryTypeAggregation, got %v",
				query, result.Type)
		}
	}
}

// ============== AdaptiveRetriever 测试 ==============

func TestAdaptiveRetriever_Basic(t *testing.T) {
	baseDocs := []rag.Document{
		{ID: "doc1", Content: "Go programming language", Score: 0.9},
		{ID: "doc2", Content: "Python for data science", Score: 0.8},
	}
	baseRet := &mockRetriever{docs: baseDocs}

	r := NewAdaptiveRetriever(
		WithBaseRetriever(baseRet),
		WithDefaultTopK(5),
	)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "programming")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestAdaptiveRetriever_ComplexityStrategy(t *testing.T) {
	baseDocs := []rag.Document{
		{ID: "doc1", Content: "test content", Score: 0.9},
	}
	baseRet := &mockRetriever{docs: baseDocs}

	// 自定义复杂度策略
	r := NewAdaptiveRetriever(
		WithBaseRetriever(baseRet),
		WithComplexityStrategy(ComplexityComplex, &RetrievalStrategy{
			TopK:        15,
			MinScore:    0.2,
			UseReranker: true,
			MultiQuery:  true,
		}),
	)

	ctx := context.Background()

	// 简单查询
	_, err := r.Retrieve(ctx, "Go")
	if err != nil {
		t.Fatalf("Retrieve failed for simple query: %v", err)
	}

	// 复杂查询
	complexQuery := "How does Go's garbage collector work and what are the performance implications in high-throughput systems?"
	_, err = r.Retrieve(ctx, complexQuery)
	if err != nil {
		t.Fatalf("Retrieve failed for complex query: %v", err)
	}
}

func TestAdaptiveRetriever_NoClassifier(t *testing.T) {
	baseDocs := []rag.Document{
		{ID: "doc1", Content: "test", Score: 0.9},
	}
	baseRet := &mockRetriever{docs: baseDocs}

	// 不设置分类器，应该使用默认的 RuleClassifier
	r := NewAdaptiveRetriever(
		WithBaseRetriever(baseRet),
	)

	if r.classifier == nil {
		t.Error("expected default classifier to be set")
	}

	ctx := context.Background()
	_, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
}

func TestAdaptiveRetriever_NoBaseRetriever(t *testing.T) {
	// 没有设置 base retriever
	r := NewAdaptiveRetriever()

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 应该返回 nil（没有检索器可用）
	if results != nil {
		t.Errorf("expected nil results when no retriever available, got %d docs", len(results))
	}
}

func TestAdaptiveRetriever_NamedRetriever(t *testing.T) {
	vectorDocs := []rag.Document{
		{ID: "v1", Content: "vector", Score: 0.9},
	}
	keywordDocs := []rag.Document{
		{ID: "k1", Content: "keyword", Score: 0.8},
	}

	vectorRet := &mockRetriever{docs: vectorDocs}
	keywordRet := &mockRetriever{docs: keywordDocs}

	r := NewAdaptiveRetriever(
		WithBaseRetriever(vectorRet),
		WithNamedRetriever("keyword", keywordRet),
		WithComplexityStrategy(ComplexityComplex, &RetrievalStrategy{
			TopK:          10,
			RetrieverName: "keyword", // 复杂查询使用关键词检索
		}),
	)

	ctx := context.Background()

	// 简单查询应该使用基础检索器（vector）
	simpleResults, err := r.Retrieve(ctx, "simple")
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(simpleResults) > 0 && simpleResults[0].ID != "v1" {
		t.Errorf("expected simple query to use vector retriever")
	}

	// 复杂查询应该使用指定的检索器（keyword）
	complexQuery := "How does this work and why is it important for understanding the underlying mechanisms?"
	complexResults, err := r.Retrieve(ctx, complexQuery)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(complexResults) > 0 && complexResults[0].ID != "k1" {
		t.Errorf("expected complex query to use keyword retriever")
	}
}

func TestAdaptiveRetriever_QueryTypeAdjustment(t *testing.T) {
	baseDocs := []rag.Document{
		{ID: "doc1", Content: "test", Score: 0.9},
	}
	baseRet := &mockRetriever{docs: baseDocs}

	r := NewAdaptiveRetriever(
		WithBaseRetriever(baseRet),
	)

	ctx := context.Background()

	// 测试比较型查询
	_, err := r.Retrieve(ctx, "Compare Go and Rust")
	if err != nil {
		t.Fatalf("Retrieve failed for comparative query: %v", err)
	}

	// 测试聚合型查询
	_, err = r.Retrieve(ctx, "List all available options")
	if err != nil {
		t.Fatalf("Retrieve failed for aggregation query: %v", err)
	}

	// 测试分析型查询
	_, err = r.Retrieve(ctx, "Why is Go good for concurrent programming?")
	if err != nil {
		t.Fatalf("Retrieve failed for analytical query: %v", err)
	}
}

func TestAdaptiveRetriever_ClassifierError(t *testing.T) {
	baseDocs := []rag.Document{
		{ID: "doc1", Content: "test", Score: 0.9},
	}
	baseRet := &mockRetriever{docs: baseDocs}

	// 创建会失败的分类器
	failingClassifier := &mockFailingClassifier{}

	r := NewAdaptiveRetriever(
		WithBaseRetriever(baseRet),
		WithClassifier(failingClassifier),
		WithDefaultTopK(3),
		WithDefaultMinScore(0.5),
	)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test query")
	if err != nil {
		t.Fatalf("Retrieve should not fail when classifier fails: %v", err)
	}

	// 应该回退到默认策略
	if len(results) == 0 {
		t.Error("expected results with fallback strategy")
	}
}

// ============== Mock 实现 ==============

// mockFailingClassifier 总是失败的分类器
type mockFailingClassifier struct{}

func (m *mockFailingClassifier) Classify(_ context.Context, _ string) (*QueryClassification, error) {
	return nil, errors.New("classification failed")
}

// ============== adjustForQueryType 测试 ==============

func TestAdjustForQueryType(t *testing.T) {
	r := NewAdaptiveRetriever()

	baseStrategy := &RetrievalStrategy{
		TopK:        5,
		MinScore:    0.5,
		UseReranker: false,
	}

	// 测试比较型查询调整
	adjusted := r.adjustForQueryType(baseStrategy, QueryTypeComparative)
	if adjusted.TopK < 8 {
		t.Errorf("expected TopK >= 8 for comparative query, got %d", adjusted.TopK)
	}
	if !adjusted.UseReranker {
		t.Error("expected UseReranker=true for comparative query")
	}

	// 测试聚合型查询调整
	adjusted = r.adjustForQueryType(baseStrategy, QueryTypeAggregation)
	if adjusted.TopK < 10 {
		t.Errorf("expected TopK >= 10 for aggregation query, got %d", adjusted.TopK)
	}
	if adjusted.MinScore > 0.3 {
		t.Errorf("expected MinScore <= 0.3 for aggregation query, got %f", adjusted.MinScore)
	}

	// 测试分析型查询调整
	adjusted = r.adjustForQueryType(baseStrategy, QueryTypeAnalytical)
	if !adjusted.UseReranker {
		t.Error("expected UseReranker=true for analytical query")
	}

	// 测试事实型查询（应该不变）
	adjusted = r.adjustForQueryType(baseStrategy, QueryTypeFactual)
	if adjusted.TopK != baseStrategy.TopK {
		t.Error("expected no TopK change for factual query")
	}
}
