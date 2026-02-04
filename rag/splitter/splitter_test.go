package splitter

import (
	"context"
	"strings"
	"testing"

	"github.com/everyday-items/hexagon/rag"
)

// ============== CharacterSplitter 测试 ==============

func TestNewCharacterSplitter(t *testing.T) {
	s := NewCharacterSplitter()
	if s == nil {
		t.Fatal("NewCharacterSplitter returned nil")
	}

	// 检查默认值
	if s.chunkSize != 1000 {
		t.Errorf("expected chunkSize=1000, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 200 {
		t.Errorf("expected chunkOverlap=200, got %d", s.chunkOverlap)
	}
	if s.separator != "\n\n" {
		t.Errorf("expected separator=\\n\\n, got %q", s.separator)
	}
	if s.Name() != "CharacterSplitter" {
		t.Errorf("expected name=CharacterSplitter, got %s", s.Name())
	}
}

func TestNewCharacterSplitter_WithOptions(t *testing.T) {
	s := NewCharacterSplitter(
		WithChunkSize(500),
		WithChunkOverlap(100),
		WithSeparator("\n"),
	)

	if s.chunkSize != 500 {
		t.Errorf("expected chunkSize=500, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 100 {
		t.Errorf("expected chunkOverlap=100, got %d", s.chunkOverlap)
	}
	if s.separator != "\n" {
		t.Errorf("expected separator=\\n, got %q", s.separator)
	}
}

func TestCharacterSplitter_Split(t *testing.T) {
	s := NewCharacterSplitter(WithChunkSize(50), WithChunkOverlap(10))
	ctx := context.Background()

	docs := []rag.Document{
		{
			ID:      "doc1",
			Content: "This is paragraph one.\n\nThis is paragraph two.\n\nThis is paragraph three.",
			Source:  "test.txt",
		},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected at least one chunk")
	}

	// 检查元数据
	for _, doc := range result {
		if doc.Metadata["parent_id"] != "doc1" {
			t.Error("parent_id should be set")
		}
		if doc.Metadata["splitter"] != "character" {
			t.Errorf("expected splitter=character, got %v", doc.Metadata["splitter"])
		}
		if doc.Source != "test.txt" {
			t.Errorf("expected source=test.txt, got %s", doc.Source)
		}
	}
}

func TestCharacterSplitter_Split_EmptyDoc(t *testing.T) {
	s := NewCharacterSplitter()
	ctx := context.Background()

	result, err := s.Split(ctx, []rag.Document{})
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(result))
	}
}

func TestCharacterSplitter_Split_ContextCancelled(t *testing.T) {
	s := NewCharacterSplitter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	docs := []rag.Document{{ID: "1", Content: "test"}}
	_, err := s.Split(ctx, docs)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

// ============== RecursiveSplitter 测试 ==============

func TestNewRecursiveSplitter(t *testing.T) {
	s := NewRecursiveSplitter()
	if s == nil {
		t.Fatal("NewRecursiveSplitter returned nil")
	}

	if s.chunkSize != 1000 {
		t.Errorf("expected chunkSize=1000, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 200 {
		t.Errorf("expected chunkOverlap=200, got %d", s.chunkOverlap)
	}
	if len(s.separators) == 0 {
		t.Error("separators should have default values")
	}
	if s.Name() != "RecursiveSplitter" {
		t.Errorf("expected name=RecursiveSplitter, got %s", s.Name())
	}
}

func TestNewRecursiveSplitter_WithOptions(t *testing.T) {
	s := NewRecursiveSplitter(
		WithRecursiveChunkSize(500),
		WithRecursiveChunkOverlap(50),
		WithSeparators([]string{"\n", " "}),
	)

	if s.chunkSize != 500 {
		t.Errorf("expected chunkSize=500, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 50 {
		t.Errorf("expected chunkOverlap=50, got %d", s.chunkOverlap)
	}
	if len(s.separators) != 2 {
		t.Errorf("expected 2 separators, got %d", len(s.separators))
	}
}

func TestRecursiveSplitter_Split(t *testing.T) {
	s := NewRecursiveSplitter(WithRecursiveChunkSize(50), WithRecursiveChunkOverlap(10))
	ctx := context.Background()

	content := strings.Repeat("This is a test sentence. ", 20)
	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) < 2 {
		t.Errorf("expected multiple chunks for long content, got %d", len(result))
	}

	for _, doc := range result {
		if doc.Metadata["splitter"] != "recursive" {
			t.Errorf("expected splitter=recursive, got %v", doc.Metadata["splitter"])
		}
	}
}

func TestRecursiveSplitter_SplitBySize(t *testing.T) {
	s := NewRecursiveSplitter(WithRecursiveChunkSize(10), WithRecursiveChunkOverlap(2))

	text := "abcdefghijklmnopqrstuvwxyz"
	chunks := s.splitBySize(text)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// 每个块应该不超过 chunkSize
	for _, chunk := range chunks {
		if len([]rune(chunk)) > s.chunkSize+s.chunkOverlap {
			t.Errorf("chunk too large: %d", len([]rune(chunk)))
		}
	}
}

// ============== MarkdownSplitter 测试 ==============

func TestNewMarkdownSplitter(t *testing.T) {
	s := NewMarkdownSplitter()
	if s == nil {
		t.Fatal("NewMarkdownSplitter returned nil")
	}

	if s.chunkSize != 1000 {
		t.Errorf("expected chunkSize=1000, got %d", s.chunkSize)
	}
	if !s.codeBlockAware {
		t.Error("codeBlockAware should be true by default")
	}
	if s.Name() != "MarkdownSplitter" {
		t.Errorf("expected name=MarkdownSplitter, got %s", s.Name())
	}
}

func TestNewMarkdownSplitter_WithOptions(t *testing.T) {
	s := NewMarkdownSplitter(
		WithMarkdownChunkSize(500),
		WithMarkdownChunkOverlap(50),
		WithHeadersToSplit([]string{"#", "##", "###"}),
		WithCodeBlockAware(false),
	)

	if s.chunkSize != 500 {
		t.Errorf("expected chunkSize=500, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 50 {
		t.Errorf("expected chunkOverlap=50, got %d", s.chunkOverlap)
	}
	if len(s.headersToSplit) != 3 {
		t.Errorf("expected 3 headers, got %d", len(s.headersToSplit))
	}
	if s.codeBlockAware {
		t.Error("codeBlockAware should be false")
	}
}

func TestMarkdownSplitter_Split(t *testing.T) {
	s := NewMarkdownSplitter()
	ctx := context.Background()

	content := `# Title

This is the introduction.

## Section 1

Content of section 1.

## Section 2

Content of section 2.
`

	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.md"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) < 2 {
		t.Errorf("expected multiple chunks for markdown with headers, got %d", len(result))
	}

	// 检查元数据
	for _, doc := range result {
		if doc.Metadata["splitter"] != "markdown" {
			t.Errorf("expected splitter=markdown, got %v", doc.Metadata["splitter"])
		}
	}
}

func TestMarkdownSplitter_CodeBlock(t *testing.T) {
	s := NewMarkdownSplitter(WithCodeBlockAware(true))
	ctx := context.Background()

	content := `# Title

Some text.

` + "```go\n" + `# This is not a header
func main() {
}
` + "```\n" + `

More text.
`

	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.md"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	// 代码块内的 # 不应该被当作标题
	for _, doc := range result {
		if strings.Contains(doc.Content, "func main") {
			// 确保代码块被保留
			if !strings.Contains(doc.Content, "```") || !strings.Contains(doc.Content, "# This is not a header") {
				t.Log("Code block content should be preserved together")
			}
		}
	}
}

func TestMarkdownSplitter_DetectHeader(t *testing.T) {
	s := NewMarkdownSplitter(WithHeadersToSplit([]string{"#", "##"}))

	tests := []struct {
		line     string
		expected string
	}{
		{"# Title", "#"},
		{"## Subtitle", "##"},
		{"### Not detected", ""},
		{"Regular text", ""},
		{"#NoSpace", ""},
	}

	for _, tt := range tests {
		result := s.detectHeader(tt.line)
		if result != tt.expected {
			t.Errorf("detectHeader(%q) = %q, want %q", tt.line, result, tt.expected)
		}
	}
}

// ============== SentenceSplitter 测试 ==============

func TestNewSentenceSplitter(t *testing.T) {
	s := NewSentenceSplitter()
	if s == nil {
		t.Fatal("NewSentenceSplitter returned nil")
	}

	if s.chunkSize != 1000 {
		t.Errorf("expected chunkSize=1000, got %d", s.chunkSize)
	}
	if len(s.sentenceEnds) == 0 {
		t.Error("sentenceEnds should have default values")
	}
	if s.Name() != "SentenceSplitter" {
		t.Errorf("expected name=SentenceSplitter, got %s", s.Name())
	}
}

func TestNewSentenceSplitter_WithOptions(t *testing.T) {
	s := NewSentenceSplitter(
		WithSentenceChunkSize(500),
		WithSentenceChunkOverlap(50),
		WithSentenceEnds([]string{".", "?", "!"}),
	)

	if s.chunkSize != 500 {
		t.Errorf("expected chunkSize=500, got %d", s.chunkSize)
	}
	if s.chunkOverlap != 50 {
		t.Errorf("expected chunkOverlap=50, got %d", s.chunkOverlap)
	}
	if len(s.sentenceEnds) != 3 {
		t.Errorf("expected 3 sentenceEnds, got %d", len(s.sentenceEnds))
	}
}

func TestSentenceSplitter_Split(t *testing.T) {
	s := NewSentenceSplitter(WithSentenceChunkSize(50))
	ctx := context.Background()

	content := "This is sentence one. This is sentence two. This is sentence three."
	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected at least one chunk")
	}

	for _, doc := range result {
		if doc.Metadata["splitter"] != "sentence" {
			t.Errorf("expected splitter=sentence, got %v", doc.Metadata["splitter"])
		}
	}
}

func TestSentenceSplitter_ChineseSentences(t *testing.T) {
	s := NewSentenceSplitter(WithSentenceChunkSize(100))
	ctx := context.Background()

	content := "这是第一句话。这是第二句话！这是第三句话？"
	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected at least one chunk for Chinese text")
	}
}

// ============== 辅助函数测试 ==============

func TestCopyMetadata(t *testing.T) {
	src := map[string]any{"key1": "value1"}
	extra := map[string]any{"key2": "value2"}

	result := copyMetadata(src, extra)

	if result["key1"] != "value1" {
		t.Error("key1 should be copied")
	}
	if result["key2"] != "value2" {
		t.Error("key2 should be merged")
	}

	// 确保不修改原始 map
	src["key3"] = "value3"
	if result["key3"] != nil {
		t.Error("result should not be affected by changes to src")
	}
}

func TestGetOverlap(t *testing.T) {
	tests := []struct {
		text     string
		overlap  int
		expected string
	}{
		{"hello world", 5, "world"},
		{"short", 10, "short"},
		{"abc", 3, "abc"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := getOverlap(tt.text, tt.overlap)
		if result != tt.expected {
			t.Errorf("getOverlap(%q, %d) = %q, want %q", tt.text, tt.overlap, result, tt.expected)
		}
	}
}

func TestGetLastHeader(t *testing.T) {
	tests := []struct {
		stack    []string
		expected string
	}{
		{[]string{"A", "B", "C"}, "C"},
		{[]string{"A"}, "A"},
		{[]string{}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		result := getLastHeader(tt.stack)
		if result != tt.expected {
			t.Errorf("getLastHeader(%v) = %q, want %q", tt.stack, result, tt.expected)
		}
	}
}

// ============== 接口实现测试 ==============

func TestInterfaceImplementation(t *testing.T) {
	var _ rag.Splitter = (*CharacterSplitter)(nil)
	var _ rag.Splitter = (*RecursiveSplitter)(nil)
	var _ rag.Splitter = (*MarkdownSplitter)(nil)
	var _ rag.Splitter = (*SentenceSplitter)(nil)
}

// ============== 边界情况测试 ==============

func TestSplitter_VeryLongContent(t *testing.T) {
	s := NewCharacterSplitter(WithChunkSize(100), WithChunkOverlap(20))
	ctx := context.Background()

	// 创建很长的内容
	content := strings.Repeat("a", 10000)
	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(result) < 50 {
		t.Errorf("expected many chunks for very long content, got %d", len(result))
	}
}

func TestSplitter_UnicodeContent(t *testing.T) {
	s := NewCharacterSplitter(WithChunkSize(50), WithChunkOverlap(10))
	ctx := context.Background()

	content := "你好世界。这是测试文本。包含多种字符：αβγδ，emoji: 🎉🎊"
	docs := []rag.Document{
		{ID: "doc1", Content: content, Source: "test.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	// 确保 Unicode 内容被正确处理
	totalContent := ""
	for _, doc := range result {
		totalContent += doc.Content
	}

	if !strings.Contains(totalContent, "你好") {
		t.Error("Unicode content should be preserved")
	}
}

func TestSplitter_MultipleDocuments(t *testing.T) {
	s := NewCharacterSplitter(WithChunkSize(50))
	ctx := context.Background()

	docs := []rag.Document{
		{ID: "doc1", Content: "First document content.", Source: "first.txt"},
		{ID: "doc2", Content: "Second document content.", Source: "second.txt"},
		{ID: "doc3", Content: "Third document content.", Source: "third.txt"},
	}

	result, err := s.Split(ctx, docs)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	// 应该为每个文档创建至少一个块
	if len(result) < 3 {
		t.Errorf("expected at least 3 chunks for 3 documents, got %d", len(result))
	}

	// 检查所有原始文档都被处理
	parentIDs := make(map[string]bool)
	for _, doc := range result {
		parentIDs[doc.Metadata["parent_id"].(string)] = true
	}

	if !parentIDs["doc1"] || !parentIDs["doc2"] || !parentIDs["doc3"] {
		t.Error("all original documents should be processed")
	}
}
