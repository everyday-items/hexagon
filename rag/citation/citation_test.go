package citation

import (
	"context"
	"strings"
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

	engine := New(retriever, provider,
		WithCitationStyle(StyleFootnote),
		WithTopK(10),
		WithMinSimilarity(0.6),
	)

	if engine == nil {
		t.Fatal("New returned nil")
	}

	if engine.style != StyleFootnote {
		t.Errorf("expected style=StyleFootnote, got %s", engine.style)
	}

	if engine.topK != 10 {
		t.Errorf("expected topK=10, got %d", engine.topK)
	}

	if engine.minSimilarity != 0.6 {
		t.Errorf("expected minSimilarity=0.6, got %f", engine.minSimilarity)
	}
}

func TestNew_DefaultValues(t *testing.T) {
	retriever := &mockRetriever{}
	provider := &mockLLMProvider{}

	engine := New(retriever, provider)

	if engine.style != StyleNumeric {
		t.Errorf("expected default style=StyleNumeric, got %s", engine.style)
	}

	if engine.topK != 5 {
		t.Errorf("expected default topK=5, got %d", engine.topK)
	}

	if engine.minSimilarity != 0.5 {
		t.Errorf("expected default minSimilarity=0.5, got %f", engine.minSimilarity)
	}
}

func TestCitationEngine_Query(t *testing.T) {
	retriever := &mockRetriever{
		docs: []rag.Document{
			{ID: "doc1", Content: "Go 是由 Google 开发的编程语言", Source: "Go官方文档"},
			{ID: "doc2", Content: "Go 于 2009 年发布", Source: "维基百科"},
		},
	}

	provider := &mockLLMProvider{
		response: "Go 是一种编程语言[1]，由 Google 开发。它于 2009 年首次发布[2]。",
	}

	engine := New(retriever, provider)

	ctx := context.Background()
	response, err := engine.Query(ctx, "什么是 Go?")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// 检查内容
	if response.Content == "" {
		t.Error("expected non-empty content")
	}

	// 检查引用
	if len(response.Citations) == 0 {
		t.Error("expected citations")
	}

	// 检查来源
	if len(response.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(response.Sources))
	}

	// 检查原始内容（无引用标记）
	if strings.Contains(response.RawContent, "[1]") || strings.Contains(response.RawContent, "[2]") {
		t.Error("raw content should not contain citation markers")
	}
}

func TestCitationEngine_ParseCitations(t *testing.T) {
	engine := &CitationEngine{style: StyleNumeric}

	docs := []rag.Document{
		{ID: "doc1", Content: "内容1", Source: "来源1"},
		{ID: "doc2", Content: "内容2", Source: "来源2"},
	}

	content := "这是第一个引用[1]，这是第二个[2]，这里同时引用[1][2]。"

	citations := engine.parseCitations(content, docs)

	if len(citations) != 2 {
		t.Errorf("expected 2 unique citations, got %d", len(citations))
	}

	// 检查引用 1
	found1 := false
	found2 := false
	for _, c := range citations {
		if c.Index == 1 {
			found1 = true
			if c.SourceID != "doc1" {
				t.Errorf("expected SourceID='doc1', got %s", c.SourceID)
			}
		}
		if c.Index == 2 {
			found2 = true
			if c.SourceID != "doc2" {
				t.Errorf("expected SourceID='doc2', got %s", c.SourceID)
			}
		}
	}

	if !found1 {
		t.Error("citation 1 not found")
	}
	if !found2 {
		t.Error("citation 2 not found")
	}
}

func TestCitationEngine_FormatMarker(t *testing.T) {
	tests := []struct {
		style    CitationStyle
		index    int
		expected string
	}{
		{StyleNumeric, 1, "[1]"},
		{StyleNumeric, 10, "[10]"},
		{StyleFootnote, 1, "¹"},
		{StyleFootnote, 5, "⁵"},
		{StyleAuthorYear, 1, "(Source 1)"},
	}

	for _, tt := range tests {
		engine := &CitationEngine{style: tt.style}
		result := engine.formatMarker(tt.index)
		if result != tt.expected {
			t.Errorf("formatMarker(%s, %d) = %s, want %s",
				tt.style, tt.index, result, tt.expected)
		}
	}
}

func TestCitationEngine_ExtractTitle(t *testing.T) {
	engine := &CitationEngine{}

	tests := []struct {
		doc      rag.Document
		expected string
	}{
		{
			doc:      rag.Document{Metadata: map[string]any{"title": "测试标题"}},
			expected: "测试标题",
		},
		{
			doc:      rag.Document{Source: "source.txt"},
			expected: "source.txt",
		},
		{
			doc:      rag.Document{Content: "第一行内容\n第二行"},
			expected: "第一行内容",
		},
		{
			doc:      rag.Document{ID: "doc123"},
			expected: "文档 doc123",
		},
	}

	for _, tt := range tests {
		result := engine.extractTitle(tt.doc)
		if result != tt.expected {
			t.Errorf("extractTitle() = %s, want %s", result, tt.expected)
		}
	}
}

func TestCitationEngine_StripCitations(t *testing.T) {
	engine := &CitationEngine{}

	tests := []struct {
		input    string
		expected string
	}{
		{
			"这是一段文字[1]，带有引用[2]。",
			"这是一段文字，带有引用。",
		},
		{
			"多个引用[1][2][3]放在一起。",
			"多个引用放在一起。",
		},
		{
			"没有引用的文字。",
			"没有引用的文字。",
		},
	}

	for _, tt := range tests {
		result := engine.stripCitations(tt.input)
		if result != tt.expected {
			t.Errorf("stripCitations(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCitationEngine_GenerateBibliography(t *testing.T) {
	engine := &CitationEngine{style: StyleNumeric}

	citations := []Citation{
		{Index: 1, Marker: "[1]", SourceTitle: "来源1", SourceURL: "http://example.com/1"},
		{Index: 2, Marker: "[2]", SourceTitle: "来源2"},
	}

	docs := []rag.Document{}

	bibliography := engine.generateBibliography(citations, docs)

	if !strings.Contains(bibliography, "参考文献") {
		t.Error("bibliography should contain header")
	}

	if !strings.Contains(bibliography, "来源1") {
		t.Error("bibliography should contain source 1")
	}

	if !strings.Contains(bibliography, "来源2") {
		t.Error("bibliography should contain source 2")
	}

	if !strings.Contains(bibliography, "http://example.com/1") {
		t.Error("bibliography should contain URL")
	}
}

func TestCitationEngine_GenerateStructuredCitation(t *testing.T) {
	retriever := &mockRetriever{}
	provider := &mockLLMProvider{
		response: `{
			"answer": "Go 是一种编程语言[1]。",
			"citations": [
				{"index": 1, "text": "Go 是一种编程语言", "source_index": 1}
			]
		}`,
	}

	engine := New(retriever, provider)

	docs := []rag.Document{
		{ID: "doc1", Content: "Go 语言介绍", Source: "Go文档"},
	}

	ctx := context.Background()
	response, err := engine.GenerateStructuredCitation(ctx, "什么是 Go?", docs)
	if err != nil {
		t.Fatalf("GenerateStructuredCitation failed: %v", err)
	}

	if response.Content == "" {
		t.Error("expected non-empty content")
	}

	if len(response.Citations) == 0 {
		t.Error("expected citations")
	}

	if response.Citations[0].Text != "Go 是一种编程语言" {
		t.Errorf("unexpected citation text: %s", response.Citations[0].Text)
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
