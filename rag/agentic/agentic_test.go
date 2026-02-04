package agentic

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

// mockLLMProvider 模拟 LLM
type mockLLMProvider struct {
	responses []string
	callCount int
}

func (p *mockLLMProvider) Name() string { return "mock" }
func (p *mockLLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{Name: "mock"}}
}
func (p *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	response := ""
	if p.callCount < len(p.responses) {
		response = p.responses[p.callCount]
	}
	p.callCount++
	return &llm.CompletionResponse{Content: response}, nil
}
func (p *mockLLMProvider) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	return nil, nil
}
func (p *mockLLMProvider) CountTokens(messages []llm.Message) (int, error) {
	return 100, nil
}

func TestNew(t *testing.T) {
	provider := &mockLLMProvider{}
	retriever := &mockRetriever{}

	arag := New(provider,
		WithRetriever("knowledge", retriever),
		WithMaxSteps(10),
		WithTopK(5),
	)

	if arag == nil {
		t.Fatal("New returned nil")
	}

	if arag.maxSteps != 10 {
		t.Errorf("expected maxSteps=10, got %d", arag.maxSteps)
	}

	if arag.topK != 5 {
		t.Errorf("expected topK=5, got %d", arag.topK)
	}

	if len(arag.retrievers) != 1 {
		t.Errorf("expected 1 retriever, got %d", len(arag.retrievers))
	}
}

func TestNew_DefaultValues(t *testing.T) {
	provider := &mockLLMProvider{}

	arag := New(provider)

	if arag.maxSteps != 5 {
		t.Errorf("expected default maxSteps=5, got %d", arag.maxSteps)
	}

	if arag.topK != 3 {
		t.Errorf("expected default topK=3, got %d", arag.topK)
	}
}

func TestAgenticRAG_Query(t *testing.T) {
	provider := &mockLLMProvider{
		responses: []string{
			// 1. 生成计划
			`{"analysis": "简单查询", "steps": [{"type": "retrieve", "query": "Go 语言", "source": "knowledge"}]}`,
			// 2. 综合回答
			"Go 是一种编程语言。",
		},
	}

	retriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "Go 是由 Google 开发的编程语言"},
		},
	}

	arag := New(provider, WithRetriever("knowledge", retriever))

	ctx := context.Background()
	response, err := arag.Query(ctx, "什么是 Go?")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Content == "" {
		t.Error("expected non-empty content")
	}

	if len(response.ExecutedSteps) == 0 {
		t.Error("expected executed steps")
	}

	if len(response.Sources) == 0 {
		t.Error("expected sources")
	}
}

func TestAgenticRAG_Query_MultipleRetrievers(t *testing.T) {
	provider := &mockLLMProvider{
		responses: []string{
			// 计划使用两个来源
			`{"analysis": "需要多来源", "steps": [{"type": "retrieve", "query": "Go", "source": "knowledge"}, {"type": "retrieve", "query": "Go trends", "source": "web"}]}`,
			// 综合
			"综合回答",
		},
	}

	knowledgeRetriever := &mockRetriever{
		docs: []rag.Document{{ID: "k1", Content: "知识库内容"}},
	}
	webRetriever := &mockRetriever{
		docs: []rag.Document{{ID: "w1", Content: "网络内容"}},
	}

	arag := New(provider,
		WithRetriever("knowledge", knowledgeRetriever),
		WithRetriever("web", webRetriever),
	)

	ctx := context.Background()
	response, err := arag.Query(ctx, "Go 最新趋势")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// 应该有来自两个来源的文档
	if len(response.Sources) < 2 {
		t.Logf("Note: got %d sources", len(response.Sources))
	}
}

func TestAgenticRAG_Query_WithReasoning(t *testing.T) {
	provider := &mockLLMProvider{
		responses: []string{
			// 计划包含推理步骤
			`{"analysis": "需要推理", "steps": [{"type": "retrieve", "query": "数据", "source": "knowledge"}, {"type": "reason", "reasoning": "分析数据"}]}`,
			// 推理结果
			"推理结论：数据表明...",
			// 综合
			"最终回答",
		},
	}

	retriever := &mockRetriever{
		docs: []rag.Document{{ID: "d1", Content: "数据内容"}},
	}

	arag := New(provider, WithRetriever("knowledge", retriever))

	ctx := context.Background()
	response, err := arag.Query(ctx, "分析问题")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// 检查是否包含推理步骤
	hasReasonStep := false
	for _, step := range response.ExecutedSteps {
		if step.Type == StepTypeReason {
			hasReasonStep = true
			break
		}
	}

	if !hasReasonStep {
		t.Error("expected at least one reason step")
	}
}

func TestAgenticRAG_CreateDefaultPlan(t *testing.T) {
	provider := &mockLLMProvider{}

	arag := New(provider,
		WithRetriever("source1", &mockRetriever{}),
		WithRetriever("source2", &mockRetriever{}),
	)

	plan := arag.createDefaultPlan("测试查询")

	if plan.Query != "测试查询" {
		t.Errorf("expected query='测试查询', got %s", plan.Query)
	}

	// 应该有多个检索步骤（每个来源一个）加一个推理步骤
	retrieveCount := 0
	reasonCount := 0
	for _, step := range plan.Steps {
		if step.Type == StepTypeRetrieve {
			retrieveCount++
		} else if step.Type == StepTypeReason {
			reasonCount++
		}
	}

	if retrieveCount != 2 {
		t.Errorf("expected 2 retrieve steps, got %d", retrieveCount)
	}

	if reasonCount != 1 {
		t.Errorf("expected 1 reason step, got %d", reasonCount)
	}
}

func TestNeedsMoreInfo(t *testing.T) {
	tests := []struct {
		conclusion string
		expected   bool
	}{
		{"答案已经完整", false},
		{"需要更多信息来确定", true},
		{"信息不足，无法判断", true},
		{"这是最终结论", false},
		{"无法确定具体原因", true},
	}

	for _, tt := range tests {
		result := needsMoreInfo(tt.conclusion)
		if result != tt.expected {
			t.Errorf("needsMoreInfo(%q) = %v, want %v", tt.conclusion, result, tt.expected)
		}
	}
}

func TestDeduplicateDocs(t *testing.T) {
	docs := []rag.Document{
		{ID: "a", Content: "A"},
		{ID: "b", Content: "B"},
		{ID: "a", Content: "A duplicate"},
		{ID: "c", Content: "C"},
	}

	result := deduplicateDocs(docs)

	if len(result) != 3 {
		t.Errorf("expected 3 unique docs, got %d", len(result))
	}

	// 检查是否保留了第一个 a
	for _, doc := range result {
		if doc.ID == "a" && doc.Content != "A" {
			t.Error("expected first occurrence of doc 'a' to be kept")
		}
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
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncateText(tt.text, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, result, tt.expected)
		}
	}
}
