package extractor

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/rag"
)

func TestTitleExtractor(t *testing.T) {
	e := NewTitleExtractor(
		WithMaxTitleLen(50),
		WithTitleFromFirstLine(true),
	)

	ctx := context.Background()

	// 测试从首行提取
	doc := rag.Document{
		Content: "Introduction to Go Programming\n\nGo is a statically typed, compiled language...",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	title, ok := result["title"].(string)
	if !ok {
		t.Fatal("expected title in result")
	}
	if title != "Introduction to Go Programming" {
		t.Errorf("expected 'Introduction to Go Programming', got %q", title)
	}
}

func TestTitleExtractor_WithExistingTitle(t *testing.T) {
	e := NewTitleExtractor()
	ctx := context.Background()

	doc := rag.Document{
		Content:  "Some content",
		Metadata: map[string]any{"title": "Existing Title"},
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result["title"] != "Existing Title" {
		t.Errorf("expected existing title to be preserved")
	}
}

func TestSummaryExtractor(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "This is a generated summary.", nil
	}

	e := NewSummaryExtractor(mockLLM)
	ctx := context.Background()

	doc := rag.Document{
		Content: "Long document content here that needs to be summarized...",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	summary, ok := result["summary"].(string)
	if !ok {
		t.Fatal("expected summary in result")
	}
	if summary != "This is a generated summary." {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestKeywordExtractor_Simple(t *testing.T) {
	e := NewKeywordExtractor(
		WithSimpleExtraction(true),
		WithMaxKeywords(5),
	)

	ctx := context.Background()
	doc := rag.Document{
		Content: "Go programming language. Go is fast. Go is simple. Python is also good. Programming is fun.",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	keywords, ok := result["keywords"].([]string)
	if !ok {
		t.Fatal("expected keywords in result")
	}

	if len(keywords) == 0 {
		t.Error("expected at least one keyword")
	}
}

func TestKeywordExtractor_WithLLM(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return `["Go", "programming", "language"]`, nil
	}

	e := NewKeywordExtractor(
		WithKeywordLLM(mockLLM),
		WithMaxKeywords(5),
	)

	ctx := context.Background()
	doc := rag.Document{
		Content: "Go is a programming language...",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	keywords, ok := result["keywords"].([]string)
	if !ok {
		t.Fatal("expected keywords in result")
	}

	if len(keywords) != 3 {
		t.Errorf("expected 3 keywords, got %d", len(keywords))
	}
}

func TestQuestionsExtractor(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return `["What is Go?", "Why use Go?"]`, nil
	}

	e := NewQuestionsExtractor(mockLLM, WithNumQuestions(3))
	ctx := context.Background()

	doc := rag.Document{
		Content: "Go is a programming language designed at Google.",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	questions, ok := result["questions"].([]string)
	if !ok {
		t.Fatal("expected questions in result")
	}

	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}
}

func TestEntityExtractor(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return `{"entities": {"person": ["John"], "organization": ["Google"]}}`, nil
	}

	e := NewEntityExtractor(mockLLM)
	ctx := context.Background()

	doc := rag.Document{
		Content: "John works at Google.",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	entities, ok := result["entities"].(map[string][]string)
	if !ok {
		t.Fatal("expected entities in result")
	}

	if len(entities["person"]) != 1 || entities["person"][0] != "John" {
		t.Error("expected person John")
	}
	if len(entities["organization"]) != 1 || entities["organization"][0] != "Google" {
		t.Error("expected organization Google")
	}
}

func TestCompositeExtractor(t *testing.T) {
	e := NewCompositeExtractor(
		NewTitleExtractor(),
		NewSimpleExtractor(),
	)

	ctx := context.Background()
	doc := rag.Document{
		Content: "Test Document Title\n\nThis is the content of the test document.",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// 应该有标题
	if _, ok := result["title"]; !ok {
		t.Error("expected title in result")
	}

	// 应该有字符数
	if _, ok := result["char_count"]; !ok {
		t.Error("expected char_count in result")
	}
}

func TestSimpleExtractor(t *testing.T) {
	e := NewSimpleExtractor()
	ctx := context.Background()

	doc := rag.Document{
		Content: "Hello world! Visit https://example.com or email test@example.com. Date: 2024-01-15",
	}

	result, err := e.Extract(ctx, doc)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// 字符数
	if result["char_count"].(int) != len(doc.Content) {
		t.Error("incorrect char_count")
	}

	// URL
	urls, ok := result["urls"].([]string)
	if !ok || len(urls) == 0 {
		t.Error("expected URLs")
	}

	// Email
	emails, ok := result["emails"].([]string)
	if !ok || len(emails) == 0 {
		t.Error("expected emails")
	}

	// Dates
	dates, ok := result["dates"].([]string)
	if !ok || len(dates) == 0 {
		t.Error("expected dates")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"Hello world, this is English text.", "en"},
		{"这是中文文本。", "zh"},
		{"这是一段包含少量English的中文文本。", "zh"}, // 中文字符更多
	}

	for _, tt := range tests {
		result := detectLanguage(tt.content)
		if result != tt.expected {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.content, result, tt.expected)
		}
	}
}

func TestExtractDates(t *testing.T) {
	content := "Date: 2024-01-15, another date: 2024/02/20, and 2024年3月1日"
	dates := extractDates(content)

	if len(dates) < 2 {
		t.Errorf("expected at least 2 dates, got %d", len(dates))
	}
}

func TestExtractURLs(t *testing.T) {
	content := "Visit https://example.com and http://test.org/path?query=1"
	urls := extractURLs(content)

	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(urls))
	}
}

func TestExtractEmails(t *testing.T) {
	content := "Contact: test@example.com or admin@company.org"
	emails := extractEmails(content)

	if len(emails) != 2 {
		t.Errorf("expected 2 emails, got %d", len(emails))
	}
}

func TestParseKeywordsJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{`["a", "b", "c"]`, 3},
		{`Response: ["x", "y"]`, 2},
		{`invalid`, 0},
	}

	for _, tt := range tests {
		result := parseKeywordsJSON(tt.input)
		if len(result) != tt.expected {
			t.Errorf("parseKeywordsJSON(%q) returned %d items, want %d", tt.input, len(result), tt.expected)
		}
	}
}

func TestIsStopWord(t *testing.T) {
	if !isStopWord("the") {
		t.Error("'the' should be a stop word")
	}
	if !isStopWord("是") {
		t.Error("'是' should be a stop word")
	}
	if isStopWord("programming") {
		t.Error("'programming' should not be a stop word")
	}
}
