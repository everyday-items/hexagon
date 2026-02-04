package loader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============== TextLoader 测试 ==============

func TestNewTextLoader(t *testing.T) {
	l := NewTextLoader("/path/to/file.txt")
	if l == nil {
		t.Fatal("NewTextLoader returned nil")
	}

	if l.path != "/path/to/file.txt" {
		t.Errorf("expected path=/path/to/file.txt, got %s", l.path)
	}
	if l.encoding != "utf-8" {
		t.Errorf("expected encoding=utf-8, got %s", l.encoding)
	}
	if l.Name() != "TextLoader" {
		t.Errorf("expected name=TextLoader, got %s", l.Name())
	}
}

func TestTextLoader_Load(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "Hello, World!\nThis is a test file."
	tmpFile.WriteString(content)
	tmpFile.Close()

	l := NewTextLoader(tmpFile.Name())
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != content {
		t.Errorf("content mismatch: got %q", doc.Content)
	}
	if doc.Source != tmpFile.Name() {
		t.Errorf("expected source=%s, got %s", tmpFile.Name(), doc.Source)
	}
	if doc.Metadata["loader"] != "text" {
		t.Errorf("expected loader=text, got %v", doc.Metadata["loader"])
	}
	if doc.Metadata["file_path"] != tmpFile.Name() {
		t.Errorf("expected file_path=%s, got %v", tmpFile.Name(), doc.Metadata["file_path"])
	}
}

func TestTextLoader_Load_FileNotFound(t *testing.T) {
	l := NewTextLoader("/nonexistent/file.txt")
	ctx := context.Background()

	_, err := l.Load(ctx)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ============== MarkdownLoader 测试 ==============

func TestNewMarkdownLoader(t *testing.T) {
	l := NewMarkdownLoader("/path/to/file.md")
	if l == nil {
		t.Fatal("NewMarkdownLoader returned nil")
	}

	if l.path != "/path/to/file.md" {
		t.Errorf("expected path=/path/to/file.md, got %s", l.path)
	}
	if !l.extractMetadata {
		t.Error("extractMetadata should be true by default")
	}
	if l.Name() != "MarkdownLoader" {
		t.Errorf("expected name=MarkdownLoader, got %s", l.Name())
	}
}

func TestNewMarkdownLoader_WithOptions(t *testing.T) {
	l := NewMarkdownLoader("/path/to/file.md",
		WithRemoveImages(true),
		WithRemoveLinks(true),
		WithExtractMetadata(false),
	)

	if !l.removeImages {
		t.Error("removeImages should be true")
	}
	if !l.removeLinks {
		t.Error("removeLinks should be true")
	}
	if l.extractMetadata {
		t.Error("extractMetadata should be false")
	}
}

func TestMarkdownLoader_Load(t *testing.T) {
	// 创建临时 Markdown 文件
	tmpFile, err := os.CreateTemp("", "test*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `---
title: Test Document
author: Tester
---

# Hello

This is a markdown file.

![Image](image.png)

[Link](https://example.com)
`
	tmpFile.WriteString(content)
	tmpFile.Close()

	l := NewMarkdownLoader(tmpFile.Name())
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Metadata["loader"] != "markdown" {
		t.Errorf("expected loader=markdown, got %v", doc.Metadata["loader"])
	}
	if doc.Metadata["title"] != "Test Document" {
		t.Errorf("expected title=Test Document, got %v", doc.Metadata["title"])
	}
	if doc.Metadata["author"] != "Tester" {
		t.Errorf("expected author=Tester, got %v", doc.Metadata["author"])
	}
}

func TestMarkdownLoader_RemoveImages(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "Before ![Alt](image.png) After"
	tmpFile.WriteString(content)
	tmpFile.Close()

	l := NewMarkdownLoader(tmpFile.Name(), WithRemoveImages(true))
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if strings.Contains(docs[0].Content, "![") {
		t.Error("images should be removed")
	}
	if !strings.Contains(docs[0].Content, "Before") || !strings.Contains(docs[0].Content, "After") {
		t.Error("surrounding text should be preserved")
	}
}

func TestMarkdownLoader_RemoveLinks(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "Click [here](https://example.com) for more"
	tmpFile.WriteString(content)
	tmpFile.Close()

	l := NewMarkdownLoader(tmpFile.Name(), WithRemoveLinks(true))
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if strings.Contains(docs[0].Content, "[here]") {
		t.Error("link markdown should be removed")
	}
	if !strings.Contains(docs[0].Content, "here") {
		t.Error("link text should be preserved")
	}
}

// ============== DirectoryLoader 测试 ==============

func TestNewDirectoryLoader(t *testing.T) {
	l := NewDirectoryLoader("/path/to/dir")
	if l == nil {
		t.Fatal("NewDirectoryLoader returned nil")
	}

	if l.path != "/path/to/dir" {
		t.Errorf("expected path=/path/to/dir, got %s", l.path)
	}
	if l.pattern != "*" {
		t.Errorf("expected pattern=*, got %s", l.pattern)
	}
	if !l.recursive {
		t.Error("recursive should be true by default")
	}
	if l.Name() != "DirectoryLoader" {
		t.Errorf("expected name=DirectoryLoader, got %s", l.Name())
	}
}

func TestNewDirectoryLoader_WithOptions(t *testing.T) {
	customLoader := func(path string) Loader {
		return NewTextLoader(path)
	}

	l := NewDirectoryLoader("/path/to/dir",
		WithPattern("*.txt"),
		WithRecursive(false),
		WithLoaderFunc(customLoader),
	)

	if l.pattern != "*.txt" {
		t.Errorf("expected pattern=*.txt, got %s", l.pattern)
	}
	if l.recursive {
		t.Error("recursive should be false")
	}
}

func TestDirectoryLoader_Load(t *testing.T) {
	// 创建临时目录和文件
	tmpDir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建文件
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Content 1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("Content 2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file3.md"), []byte("# Markdown"), 0644)

	l := NewDirectoryLoader(tmpDir, WithPattern("*.txt"))
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("expected 2 documents for *.txt pattern, got %d", len(docs))
	}
}

func TestDirectoryLoader_Load_Recursive(t *testing.T) {
	// 创建临时目录结构
	tmpDir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建子目录
	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)

	os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("Root"), 0644)
	os.WriteFile(filepath.Join(subDir, "sub.txt"), []byte("Sub"), 0644)

	// 递归模式
	l := NewDirectoryLoader(tmpDir, WithRecursive(true))
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) < 2 {
		t.Errorf("expected at least 2 documents with recursive, got %d", len(docs))
	}

	// 非递归模式
	l = NewDirectoryLoader(tmpDir, WithRecursive(false))
	docs, err = l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("expected 1 document without recursive, got %d", len(docs))
	}
}

func TestDirectoryLoader_Load_ContextCancelled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("Content"), 0644)

	l := NewDirectoryLoader(tmpDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = l.Load(ctx)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

// ============== URLLoader 测试 ==============

func TestNewURLLoader(t *testing.T) {
	l := NewURLLoader("https://example.com")
	if l == nil {
		t.Fatal("NewURLLoader returned nil")
	}

	if l.url != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %s", l.url)
	}
	if l.userAgent != "Hexagon-RAG/1.0" {
		t.Errorf("expected userAgent=Hexagon-RAG/1.0, got %s", l.userAgent)
	}
	if l.Name() != "URLLoader" {
		t.Errorf("expected name=URLLoader, got %s", l.Name())
	}
}

func TestNewURLLoader_WithOptions(t *testing.T) {
	client := &http.Client{}
	headers := map[string]string{"Authorization": "Bearer token"}

	l := NewURLLoader("https://example.com",
		WithHTTPClient(client),
		WithHeaders(headers),
		WithUserAgent("CustomAgent/1.0"),
	)

	if l.client != client {
		t.Error("client not set correctly")
	}
	if l.headers["Authorization"] != "Bearer token" {
		t.Error("headers not set correctly")
	}
	if l.userAgent != "CustomAgent/1.0" {
		t.Errorf("expected userAgent=CustomAgent/1.0, got %s", l.userAgent)
	}
}

func TestURLLoader_Load(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello from server"))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL)
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != "Hello from server" {
		t.Errorf("content mismatch: got %q", doc.Content)
	}
	if doc.Source != server.URL {
		t.Errorf("expected source=%s, got %s", server.URL, doc.Source)
	}
	if doc.Metadata["loader"] != "url" {
		t.Errorf("expected loader=url, got %v", doc.Metadata["loader"])
	}
	if doc.Metadata["content_type"] != "text/plain" {
		t.Errorf("expected content_type=text/plain, got %v", doc.Metadata["content_type"])
	}
}

func TestURLLoader_Load_Error(t *testing.T) {
	// 创建返回错误的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	l := NewURLLoader(server.URL)
	ctx := context.Background()

	_, err := l.Load(ctx)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestURLLoader_Load_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Content"))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Load(ctx)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

// ============== ReaderLoader 测试 ==============

func TestNewReaderLoader(t *testing.T) {
	reader := strings.NewReader("test content")
	l := NewReaderLoader(reader, "test-source")

	if l == nil {
		t.Fatal("NewReaderLoader returned nil")
	}
	if l.source != "test-source" {
		t.Errorf("expected source=test-source, got %s", l.source)
	}
	if l.Name() != "ReaderLoader" {
		t.Errorf("expected name=ReaderLoader, got %s", l.Name())
	}
}

func TestReaderLoader_Load(t *testing.T) {
	reader := strings.NewReader("Hello from reader")
	l := NewReaderLoader(reader, "memory")
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != "Hello from reader" {
		t.Errorf("content mismatch: got %q", doc.Content)
	}
	if doc.Source != "memory" {
		t.Errorf("expected source=memory, got %s", doc.Source)
	}
	if doc.Metadata["loader"] != "reader" {
		t.Errorf("expected loader=reader, got %v", doc.Metadata["loader"])
	}
}

func TestReaderLoader_Load_Error(t *testing.T) {
	// 创建一个会失败的 reader
	failReader := &errorReader{}
	l := NewReaderLoader(failReader, "error")
	ctx := context.Background()

	_, err := l.Load(ctx)
	if err == nil {
		t.Error("expected error from failing reader")
	}
}

type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// ============== StringLoader 测试 ==============

func TestNewStringLoader(t *testing.T) {
	l := NewStringLoader("content", "source")
	if l == nil {
		t.Fatal("NewStringLoader returned nil")
	}

	if l.content != "content" {
		t.Errorf("expected content=content, got %s", l.content)
	}
	if l.source != "source" {
		t.Errorf("expected source=source, got %s", l.source)
	}
	if l.Name() != "StringLoader" {
		t.Errorf("expected name=StringLoader, got %s", l.Name())
	}
}

func TestStringLoader_Load(t *testing.T) {
	l := NewStringLoader("Hello, World!", "inline")
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != "Hello, World!" {
		t.Errorf("content mismatch: got %q", doc.Content)
	}
	if doc.Source != "inline" {
		t.Errorf("expected source=inline, got %s", doc.Source)
	}
	if doc.Metadata["loader"] != "string" {
		t.Errorf("expected loader=string, got %v", doc.Metadata["loader"])
	}
}

// ============== 辅助函数测试 ==============

func TestExtractFrontMatter(t *testing.T) {
	content := `---
title: Test
author: Tester
---

Body content here.
`
	metadata := map[string]any{"loader": "markdown"}

	body, resultMeta := extractFrontMatter(content, metadata)

	if !strings.Contains(body, "Body content") {
		t.Error("body should contain main content")
	}
	if strings.Contains(body, "---") {
		t.Error("body should not contain front matter delimiters")
	}
	if resultMeta["title"] != "Test" {
		t.Errorf("expected title=Test, got %v", resultMeta["title"])
	}
	if resultMeta["author"] != "Tester" {
		t.Errorf("expected author=Tester, got %v", resultMeta["author"])
	}
	if resultMeta["loader"] != "markdown" {
		t.Error("original metadata should be preserved")
	}
}

func TestExtractFrontMatter_NoFrontMatter(t *testing.T) {
	content := "Just regular content without front matter."
	metadata := map[string]any{}

	body, _ := extractFrontMatter(content, metadata)

	if body != content {
		t.Error("content should be unchanged when no front matter")
	}
}

func TestRemoveMarkdownImages(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"![Alt](image.png)", ""},
		{"Before ![Alt](url) After", "Before  After"},
		{"No images here", "No images here"},
		{"![](empty.png)", ""},
	}

	for _, tt := range tests {
		result := removeMarkdownImages(tt.input)
		if result != tt.expected {
			t.Errorf("removeMarkdownImages(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRemoveMarkdownLinks(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[Link](https://example.com)", "Link"},
		{"Before [click here](url) After", "Before click here After"},
		{"No links here", "No links here"},
		{"[Text]()", "Text"},
	}

	for _, tt := range tests {
		result := removeMarkdownLinks(tt.input)
		if result != tt.expected {
			t.Errorf("removeMarkdownLinks(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// ============== 接口实现测试 ==============

func TestInterfaceImplementation(t *testing.T) {
	var _ Loader = (*TextLoader)(nil)
	var _ Loader = (*MarkdownLoader)(nil)
	var _ Loader = (*DirectoryLoader)(nil)
	var _ Loader = (*URLLoader)(nil)
	var _ Loader = (*ReaderLoader)(nil)
	var _ Loader = (*StringLoader)(nil)
}

// Loader 接口定义（用于本地测试）
type Loader interface {
	Load(ctx context.Context) ([]interface{ GetContent() string }, error)
	Name() string
}
