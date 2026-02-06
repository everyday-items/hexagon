package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/everyday-items/hexagon/rag"
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
	customLoader := func(path string) rag.Loader {
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
	var _ rag.Loader = (*TextLoader)(nil)
	var _ rag.Loader = (*MarkdownLoader)(nil)
	var _ rag.Loader = (*DirectoryLoader)(nil)
	var _ rag.Loader = (*URLLoader)(nil)
	var _ rag.Loader = (*ReaderLoader)(nil)
	var _ rag.Loader = (*StringLoader)(nil)
}

// ============== TextLoader 补充测试 ==============

// TestTextLoader_Load_VerifyAllFields 验证加载后 Document 所有字段
func TestTextLoader_Load_VerifyAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "一段中文内容\nSecond line"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewTextLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]

	// 验证 ID 非空且以 doc- 开头
	if doc.ID == "" {
		t.Error("ID 不应为空")
	}
	if !strings.HasPrefix(doc.ID, "doc-") {
		t.Errorf("ID 应以 doc- 开头, 实际 %s", doc.ID)
	}

	// 验证 Content
	if doc.Content != content {
		t.Errorf("Content 不匹配: 期望 %q, 实际 %q", content, doc.Content)
	}

	// 验证 Source
	if doc.Source != path {
		t.Errorf("Source 不匹配: 期望 %s, 实际 %s", path, doc.Source)
	}

	// 验证 Metadata
	if doc.Metadata["loader"] != "text" {
		t.Errorf("Metadata[loader] 期望 text, 实际 %v", doc.Metadata["loader"])
	}
	if doc.Metadata["file_path"] != path {
		t.Errorf("Metadata[file_path] 期望 %s, 实际 %v", path, doc.Metadata["file_path"])
	}
	if doc.Metadata["file_name"] != "sample.txt" {
		t.Errorf("Metadata[file_name] 期望 sample.txt, 实际 %v", doc.Metadata["file_name"])
	}

	// 验证 CreatedAt 非零值
	if doc.CreatedAt.IsZero() {
		t.Error("CreatedAt 不应为零值")
	}
}

// TestTextLoader_Load_EmptyFile 加载空文件
func TestTextLoader_Load_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewTextLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空文件不应报错: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "" {
		t.Errorf("空文件 Content 应为空, 实际 %q", docs[0].Content)
	}
}

// TestTextLoader_Load_LargeContent 加载大文件
func TestTextLoader_Load_LargeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	// 生成 10KB 内容
	largeContent := strings.Repeat("abcdefghij", 1024)
	if err := os.WriteFile(path, []byte(largeContent), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewTextLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if docs[0].Content != largeContent {
		t.Error("大文件内容不匹配")
	}
}

// TestTextLoader_Name 验证 Name 方法返回值
func TestTextLoader_Name(t *testing.T) {
	l := NewTextLoader("any-path")
	if got := l.Name(); got != "TextLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "TextLoader")
	}
}

// TestTextLoader_DefaultEncoding 验证默认编码
func TestTextLoader_DefaultEncoding(t *testing.T) {
	l := NewTextLoader("/some/path")
	if l.encoding != "utf-8" {
		t.Errorf("默认编码应为 utf-8, 实际 %s", l.encoding)
	}
}

// ============== MarkdownLoader 补充测试 ==============

// TestMarkdownLoader_Load_BasicMarkdown 加载基础 Markdown 文件（无 front matter）
func TestMarkdownLoader_Load_BasicMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "basic.md")
	content := "# Title\n\nSome **bold** text and `code`.\n\n## Section\n\nMore content."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]
	// 无 front matter 时 Content 应保持原样
	if doc.Content != content {
		t.Errorf("Content 不匹配:\n期望: %q\n实际: %q", content, doc.Content)
	}
	if doc.Metadata["loader"] != "markdown" {
		t.Errorf("Metadata[loader] 期望 markdown, 实际 %v", doc.Metadata["loader"])
	}
	if doc.Metadata["file_name"] != "basic.md" {
		t.Errorf("Metadata[file_name] 期望 basic.md, 实际 %v", doc.Metadata["file_name"])
	}
}

// TestMarkdownLoader_ExtractMetadata_YAMLFrontMatter 提取 YAML front matter
func TestMarkdownLoader_ExtractMetadata_YAMLFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.md")
	content := `---
title: 我的文档
author: 张三
tags: go, ai
---

# 正文标题

这是正文内容。`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path, WithExtractMetadata(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if doc.Metadata["title"] != "我的文档" {
		t.Errorf("Metadata[title] 期望 '我的文档', 实际 %v", doc.Metadata["title"])
	}
	if doc.Metadata["author"] != "张三" {
		t.Errorf("Metadata[author] 期望 '张三', 实际 %v", doc.Metadata["author"])
	}
	if doc.Metadata["tags"] != "go, ai" {
		t.Errorf("Metadata[tags] 期望 'go, ai', 实际 %v", doc.Metadata["tags"])
	}
	// 正文不应包含 front matter 分隔符
	if strings.Contains(doc.Content, "---") {
		t.Error("Content 不应包含 front matter 分隔符")
	}
	if !strings.Contains(doc.Content, "正文标题") {
		t.Error("Content 应包含正文")
	}
}

// TestMarkdownLoader_WithRemoveImages_MultipleImages 移除多个图片
func TestMarkdownLoader_WithRemoveImages_MultipleImages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.md")
	content := "Text ![img1](a.png) middle ![img2](b.jpg) end"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path, WithRemoveImages(true), WithExtractMetadata(false))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if strings.Contains(doc.Content, "![") {
		t.Error("Content 中不应包含图片标记")
	}
	if !strings.Contains(doc.Content, "Text") {
		t.Error("Content 应保留 Text")
	}
	if !strings.Contains(doc.Content, "middle") {
		t.Error("Content 应保留 middle")
	}
	if !strings.Contains(doc.Content, "end") {
		t.Error("Content 应保留 end")
	}
}

// TestMarkdownLoader_WithRemoveLinks_MultipleLinks 移除多个链接
func TestMarkdownLoader_WithRemoveLinks_MultipleLinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.md")
	content := "Visit [Google](https://google.com) and [GitHub](https://github.com)."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path, WithRemoveLinks(true), WithExtractMetadata(false))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if strings.Contains(doc.Content, "](") {
		t.Error("Content 中不应包含链接 Markdown 语法")
	}
	if !strings.Contains(doc.Content, "Google") {
		t.Error("Content 应保留链接文字 Google")
	}
	if !strings.Contains(doc.Content, "GitHub") {
		t.Error("Content 应保留链接文字 GitHub")
	}
}

// TestMarkdownLoader_CombinedOptions 同时启用多个选项
func TestMarkdownLoader_CombinedOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "combined.md")
	content := `---
title: Combined
---

Some text with ![img](pic.png) and [link](http://x.com) here.`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path,
		WithExtractMetadata(true),
		WithRemoveImages(true),
		WithRemoveLinks(true),
	)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	// front matter 被提取
	if doc.Metadata["title"] != "Combined" {
		t.Errorf("Metadata[title] 期望 Combined, 实际 %v", doc.Metadata["title"])
	}
	// 图片被移除
	if strings.Contains(doc.Content, "![") {
		t.Error("图片应被移除")
	}
	// 链接被移除但文字保留
	if strings.Contains(doc.Content, "](") {
		t.Error("链接语法应被移除")
	}
	if !strings.Contains(doc.Content, "link") {
		t.Error("链接文字应保留")
	}
}

// TestMarkdownLoader_NoFrontMatter 无 front matter 的文件
func TestMarkdownLoader_NoFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nofm.md")
	content := "Just plain markdown without front matter."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path, WithExtractMetadata(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if doc.Content != content {
		t.Errorf("Content 应保持不变: 期望 %q, 实际 %q", content, doc.Content)
	}
	// 默认 metadata 应存在
	if doc.Metadata["loader"] != "markdown" {
		t.Errorf("Metadata[loader] 期望 markdown, 实际 %v", doc.Metadata["loader"])
	}
}

// TestMarkdownLoader_Load_FileNotFound 文件不存在
func TestMarkdownLoader_Load_FileNotFound(t *testing.T) {
	l := NewMarkdownLoader("/nonexistent/path/doc.md")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("文件不存在应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to read file") {
		t.Errorf("错误信息应包含 'failed to read file', 实际: %v", err)
	}
}

// TestMarkdownLoader_Name 验证 Name 方法
func TestMarkdownLoader_Name(t *testing.T) {
	l := NewMarkdownLoader("any.md")
	if got := l.Name(); got != "MarkdownLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "MarkdownLoader")
	}
}

// TestMarkdownLoader_ExtractMetadataDisabled 禁用元数据提取
func TestMarkdownLoader_ExtractMetadataDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disable_meta.md")
	content := `---
title: ShouldNotExtract
---

Body content.`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewMarkdownLoader(path, WithExtractMetadata(false))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	// 禁用元数据提取时，title 不应出现在 metadata 中
	if _, ok := doc.Metadata["title"]; ok {
		t.Error("禁用 ExtractMetadata 后不应提取 title")
	}
	// Content 应包含原始 front matter 分隔符
	if !strings.Contains(doc.Content, "---") {
		t.Error("禁用 ExtractMetadata 后 Content 应保留 front matter")
	}
}

// ============== DirectoryLoader 补充测试 ==============

// TestDirectoryLoader_Load_RecursiveWithSubDirs 递归加载包含子目录的目录
func TestDirectoryLoader_Load_RecursiveWithSubDirs(t *testing.T) {
	dir := t.TempDir()
	// 创建多层目录结构
	sub1 := filepath.Join(dir, "sub1")
	sub2 := filepath.Join(dir, "sub1", "sub2")
	os.MkdirAll(sub2, 0755)

	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(sub1, "level1.txt"), []byte("level1"), 0644)
	os.WriteFile(filepath.Join(sub2, "level2.txt"), []byte("level2"), 0644)

	l := NewDirectoryLoader(dir, WithRecursive(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("期望 3 个文档（递归模式）, 实际 %d", len(docs))
	}
}

// TestDirectoryLoader_Load_NonRecursive 非递归只加载顶层
func TestDirectoryLoader_Load_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)

	os.WriteFile(filepath.Join(dir, "top.txt"), []byte("top"), 0644)
	os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("nested"), 0644)

	l := NewDirectoryLoader(dir, WithRecursive(false))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("非递归模式期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "top" {
		t.Errorf("非递归应只加载顶层文件, Content=%q", docs[0].Content)
	}
}

// TestDirectoryLoader_Load_WithPattern 使用 glob 模式匹配
func TestDirectoryLoader_Load_WithPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# MD"), 0644)
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("TXT"), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("PNG"), 0644)

	l := NewDirectoryLoader(dir, WithPattern("*.md"))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("Pattern *.md 应匹配 1 个文件, 实际 %d", len(docs))
	}
}

// TestDirectoryLoader_Load_CustomLoaderFunc 自定义 LoaderFunc
func TestDirectoryLoader_Load_CustomLoaderFunc(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	customCalled := false
	l := NewDirectoryLoader(dir, WithLoaderFunc(func(path string) rag.Loader {
		customCalled = true
		return NewTextLoader(path)
	}))

	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if !customCalled {
		t.Error("自定义 LoaderFunc 未被调用")
	}
	if len(docs) != 1 {
		t.Errorf("期望 1 个文档, 实际 %d", len(docs))
	}
}

// TestDirectoryLoader_Load_EmptyDirectory 空目录
func TestDirectoryLoader_Load_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	l := NewDirectoryLoader(dir)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空目录不应报错: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("空目录应返回 0 个文档, 实际 %d", len(docs))
	}
}

// TestDirectoryLoader_Load_NonExistentDir 不存在的目录
func TestDirectoryLoader_Load_NonExistentDir(t *testing.T) {
	l := NewDirectoryLoader("/nonexistent/dir/path")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("不存在的目录应返回错误")
	}
}

// TestDirectoryLoader_Load_ContextCancel 上下文取消中断加载
func TestDirectoryLoader_Load_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	// 创建一些文件
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		os.WriteFile(name, []byte("content"), 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	l := NewDirectoryLoader(dir)
	_, err := l.Load(ctx)
	if err == nil {
		t.Error("已取消的 context 应导致错误")
	}
}

// TestDirectoryLoader_Name 验证 Name 方法
func TestDirectoryLoader_Name(t *testing.T) {
	l := NewDirectoryLoader("any-dir")
	if got := l.Name(); got != "DirectoryLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "DirectoryLoader")
	}
}

// TestDirectoryLoader_DefaultLoaderFunc_MarkdownDetection 默认 LoaderFunc 能识别 Markdown 文件
func TestDirectoryLoader_DefaultLoaderFunc_MarkdownDetection(t *testing.T) {
	dir := t.TempDir()
	mdContent := `---
title: FromDir
---

# DirectoryMD`
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte(mdContent), 0644)
	os.WriteFile(filepath.Join(dir, "note.markdown"), []byte("# Note"), 0644)

	l := NewDirectoryLoader(dir)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档, 实际 %d", len(docs))
	}

	// 检查其中一个是否使用了 MarkdownLoader 的元数据提取
	foundMDMeta := false
	for _, doc := range docs {
		if doc.Metadata["loader"] == "markdown" {
			foundMDMeta = true
			break
		}
	}
	if !foundMDMeta {
		t.Error("至少一个 .md 文件应使用 MarkdownLoader 加载")
	}
}

// ============== URLLoader 补充测试 ==============

// TestURLLoader_Load_Success 成功加载并验证所有 Metadata 字段
func TestURLLoader_Load_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != `{"key":"value"}` {
		t.Errorf("Content 不匹配: %q", doc.Content)
	}
	if doc.Source != server.URL {
		t.Errorf("Source 应为 %s, 实际 %s", server.URL, doc.Source)
	}
	if doc.Metadata["url"] != server.URL {
		t.Errorf("Metadata[url] 应为 %s", server.URL)
	}
	if doc.Metadata["content_type"] != "application/json; charset=utf-8" {
		t.Errorf("Metadata[content_type] 不匹配: %v", doc.Metadata["content_type"])
	}
	if doc.Metadata["status_code"] != 200 {
		t.Errorf("Metadata[status_code] 应为 200, 实际 %v", doc.Metadata["status_code"])
	}
	if doc.Metadata["loader"] != "url" {
		t.Errorf("Metadata[loader] 应为 url, 实际 %v", doc.Metadata["loader"])
	}
}

// TestURLLoader_Load_Non200 非 200 状态码应报错
func TestURLLoader_Load_Non200(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"403 Forbidden", http.StatusForbidden},
		{"404 Not Found", http.StatusNotFound},
		{"500 Server Error", http.StatusInternalServerError},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			l := NewURLLoader(server.URL)
			_, err := l.Load(context.Background())
			if err == nil {
				t.Errorf("状态码 %d 应返回错误", tt.statusCode)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", tt.statusCode)) {
				t.Errorf("错误信息应包含状态码 %d: %v", tt.statusCode, err)
			}
		})
	}
}

// TestURLLoader_WithHeaders_CustomHeaders 自定义请求头
func TestURLLoader_WithHeaders_CustomHeaders(t *testing.T) {
	var receivedAuth string
	var receivedCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL, WithHeaders(map[string]string{
		"Authorization":   "Bearer test-token",
		"X-Custom-Header": "custom-value",
	}))
	_, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if receivedAuth != "Bearer test-token" {
		t.Errorf("服务器应收到 Authorization 头: %q", receivedAuth)
	}
	if receivedCustom != "custom-value" {
		t.Errorf("服务器应收到 X-Custom-Header: %q", receivedCustom)
	}
}

// TestURLLoader_WithUserAgent 自定义 User-Agent
func TestURLLoader_WithUserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL, WithUserAgent("MyBot/2.0"))
	_, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if receivedUA != "MyBot/2.0" {
		t.Errorf("服务器应收到 User-Agent=MyBot/2.0, 实际 %q", receivedUA)
	}
}

// TestURLLoader_DefaultUserAgent 默认 User-Agent
func TestURLLoader_DefaultUserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	l := NewURLLoader(server.URL)
	_, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if receivedUA != "Hexagon-RAG/1.0" {
		t.Errorf("默认 User-Agent 应为 Hexagon-RAG/1.0, 实际 %q", receivedUA)
	}
}

// TestURLLoader_Name 验证 Name 方法
func TestURLLoader_Name(t *testing.T) {
	l := NewURLLoader("https://example.com")
	if got := l.Name(); got != "URLLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "URLLoader")
	}
}

// ============== ReaderLoader 补充测试 ==============

// TestReaderLoader_Load_VerifyFields 验证所有 Document 字段
func TestReaderLoader_Load_VerifyFields(t *testing.T) {
	content := "Reader content with 中文"
	reader := strings.NewReader(content)
	l := NewReaderLoader(reader, "custom-source")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != content {
		t.Errorf("Content 不匹配")
	}
	if doc.Source != "custom-source" {
		t.Errorf("Source 应为 custom-source, 实际 %s", doc.Source)
	}
	if doc.Metadata["loader"] != "reader" {
		t.Errorf("Metadata[loader] 应为 reader")
	}
	if doc.ID == "" {
		t.Error("ID 不应为空")
	}
	if doc.CreatedAt.IsZero() {
		t.Error("CreatedAt 不应为零值")
	}
}

// TestReaderLoader_Load_EmptyReader 空 Reader
func TestReaderLoader_Load_EmptyReader(t *testing.T) {
	reader := strings.NewReader("")
	l := NewReaderLoader(reader, "empty")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空 Reader 不应报错: %v", err)
	}

	if docs[0].Content != "" {
		t.Errorf("空 Reader 的 Content 应为空, 实际 %q", docs[0].Content)
	}
}

// TestReaderLoader_Name 验证 Name 方法
func TestReaderLoader_Name(t *testing.T) {
	l := NewReaderLoader(strings.NewReader(""), "src")
	if got := l.Name(); got != "ReaderLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "ReaderLoader")
	}
}

// ============== StringLoader 补充测试 ==============

// TestStringLoader_Load_EmptyString 空字符串
func TestStringLoader_Load_EmptyString(t *testing.T) {
	l := NewStringLoader("", "empty-source")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空字符串不应报错: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "" {
		t.Errorf("空字符串 Content 应为空, 实际 %q", docs[0].Content)
	}
	if docs[0].Source != "empty-source" {
		t.Errorf("Source 应为 empty-source, 实际 %s", docs[0].Source)
	}
}

// TestStringLoader_Load_VerifyFields 验证所有字段
func TestStringLoader_Load_VerifyFields(t *testing.T) {
	l := NewStringLoader("Hello 世界!", "my-source")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if doc.Content != "Hello 世界!" {
		t.Errorf("Content 不匹配: %q", doc.Content)
	}
	if doc.Source != "my-source" {
		t.Errorf("Source 不匹配: %s", doc.Source)
	}
	if doc.Metadata["loader"] != "string" {
		t.Errorf("Metadata[loader] 应为 string")
	}
	if doc.ID == "" {
		t.Error("ID 不应为空")
	}
	if doc.CreatedAt.IsZero() {
		t.Error("CreatedAt 不应为零值")
	}
}

// TestStringLoader_Name 验证 Name 方法
func TestStringLoader_Name(t *testing.T) {
	l := NewStringLoader("c", "s")
	if got := l.Name(); got != "StringLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "StringLoader")
	}
}

// TestStringLoader_Load_MultilineContent 多行内容
func TestStringLoader_Load_MultilineContent(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\n"
	l := NewStringLoader(content, "multiline")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if docs[0].Content != content {
		t.Errorf("多行 Content 不匹配")
	}
}

// ============== 辅助函数补充测试 ==============

// TestExtractFrontMatter_EmptyContent 空内容
func TestExtractFrontMatter_EmptyContent(t *testing.T) {
	metadata := map[string]any{"loader": "test"}
	body, meta := extractFrontMatter("", metadata)
	if body != "" {
		t.Errorf("空内容应返回空 body, 实际 %q", body)
	}
	if meta["loader"] != "test" {
		t.Error("原始 metadata 应保留")
	}
}

// TestExtractFrontMatter_OnlyDelimiters 只有分隔符
func TestExtractFrontMatter_OnlyDelimiters(t *testing.T) {
	content := "---\n---\n"
	metadata := map[string]any{}
	body, _ := extractFrontMatter(content, metadata)
	// 分隔符之间无内容，body 应为空或只有空白
	body = strings.TrimSpace(body)
	if body != "" {
		t.Errorf("只有分隔符时 body 应为空, 实际 %q", body)
	}
}

// TestExtractFrontMatter_ComplexValues front matter 带冒号值
func TestExtractFrontMatter_ComplexValues(t *testing.T) {
	content := `---
url: https://example.com:8080/path
description: This is: a test
---

Body`
	metadata := map[string]any{}
	body, meta := extractFrontMatter(content, metadata)

	if meta["url"] != "https://example.com:8080/path" {
		t.Errorf("url 应正确解析, 实际 %v", meta["url"])
	}
	if meta["description"] != "This is: a test" {
		t.Errorf("description 应正确解析, 实际 %v", meta["description"])
	}
	if !strings.Contains(body, "Body") {
		t.Error("body 应包含正文")
	}
}

// TestExtractFrontMatter_PreservesExistingMetadata 保留已有 metadata
func TestExtractFrontMatter_PreservesExistingMetadata(t *testing.T) {
	content := `---
title: New
---

Text`
	metadata := map[string]any{
		"loader":    "markdown",
		"file_path": "/some/path",
	}
	_, meta := extractFrontMatter(content, metadata)

	if meta["loader"] != "markdown" {
		t.Error("已有的 loader 字段应保留")
	}
	if meta["file_path"] != "/some/path" {
		t.Error("已有的 file_path 字段应保留")
	}
	if meta["title"] != "New" {
		t.Error("新提取的 title 应存在")
	}
}

// TestRemoveMarkdownImages_NoImages 无图片
func TestRemoveMarkdownImages_NoImages(t *testing.T) {
	input := "Just text without any images."
	result := removeMarkdownImages(input)
	if result != input {
		t.Errorf("无图片时内容应不变: %q", result)
	}
}

// TestRemoveMarkdownImages_Multiple 多个图片
func TestRemoveMarkdownImages_Multiple(t *testing.T) {
	input := "A ![a](1.png) B ![b](2.png) C"
	result := removeMarkdownImages(input)
	if strings.Contains(result, "![") {
		t.Errorf("所有图片应被移除: %q", result)
	}
	if !strings.Contains(result, "A") || !strings.Contains(result, "B") || !strings.Contains(result, "C") {
		t.Error("图片周围的文本应保留")
	}
}

// TestRemoveMarkdownImages_NestedBrackets 嵌套括号
func TestRemoveMarkdownImages_NestedBrackets(t *testing.T) {
	input := "Before ![alt text (with parens)](url.png) After"
	result := removeMarkdownImages(input)
	// 至少 "Before" 和 "After" 应保留（具体行为取决于实现）
	if !strings.Contains(result, "Before") {
		t.Error("Before 应保留")
	}
}

// TestRemoveMarkdownImages_UnclosedBracket 未闭合的图片标记
func TestRemoveMarkdownImages_UnclosedBracket(t *testing.T) {
	input := "Before ![alt text no closing paren After"
	result := removeMarkdownImages(input)
	// 未闭合时应保留原始内容
	if !strings.Contains(result, "Before") {
		t.Error("未闭合标记不应导致数据丢失")
	}
}

// TestRemoveMarkdownLinks_NoLinks 无链接
func TestRemoveMarkdownLinks_NoLinks(t *testing.T) {
	input := "Plain text without links."
	result := removeMarkdownLinks(input)
	if result != input {
		t.Errorf("无链接时内容应不变: %q", result)
	}
}

// TestRemoveMarkdownLinks_MultipleLinks 多个链接
func TestRemoveMarkdownLinks_MultipleLinks(t *testing.T) {
	input := "[A](url1) text [B](url2) more [C](url3)"
	result := removeMarkdownLinks(input)
	if strings.Contains(result, "](") {
		t.Errorf("所有链接语法应被移除: %q", result)
	}
	if !strings.Contains(result, "A") || !strings.Contains(result, "B") || !strings.Contains(result, "C") {
		t.Error("链接文字应全部保留")
	}
}

// TestRemoveMarkdownLinks_EmptyLinkText 空链接文字
func TestRemoveMarkdownLinks_EmptyLinkText(t *testing.T) {
	input := "Before [](http://empty.com) After"
	result := removeMarkdownLinks(input)
	if strings.Contains(result, "](") {
		t.Errorf("空链接文字时语法也应被移除: %q", result)
	}
	if !strings.Contains(result, "Before") || !strings.Contains(result, "After") {
		t.Error("周围文本应保留")
	}
}

// TestRemoveMarkdownLinks_UnclosedBracket 未闭合的链接
func TestRemoveMarkdownLinks_UnclosedBracket(t *testing.T) {
	input := "Before [text without closing After"
	result := removeMarkdownLinks(input)
	// 未闭合时应保留原始内容
	if result != input {
		t.Errorf("未闭合的链接应保留原始内容: %q", result)
	}
}

// ============== CompositeLoader 测试 ==============

// TestCompositeLoader_Load 组合加载器
func TestCompositeLoader_Load(t *testing.T) {
	loader1 := NewStringLoader("content1", "source1")
	loader2 := NewStringLoader("content2", "source2")

	cl := NewCompositeLoader(loader1, loader2)
	docs, err := cl.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("期望 2 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "content1" {
		t.Errorf("第 1 个文档 Content 不匹配: %q", docs[0].Content)
	}
	if docs[1].Content != "content2" {
		t.Errorf("第 2 个文档 Content 不匹配: %q", docs[1].Content)
	}
}

// TestCompositeLoader_AddLoader 动态添加加载器
func TestCompositeLoader_AddLoader(t *testing.T) {
	cl := NewCompositeLoader()
	cl.AddLoader(NewStringLoader("first", "s1"))
	cl.AddLoader(NewStringLoader("second", "s2"))

	docs, err := cl.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("期望 2 个文档, 实际 %d", len(docs))
	}
}

// TestCompositeLoader_Name 验证名称
func TestCompositeLoader_Name(t *testing.T) {
	cl := NewCompositeLoader()
	if got := cl.Name(); got != "CompositeLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "CompositeLoader")
	}
}

// TestCompositeLoader_Empty 空组合加载器
func TestCompositeLoader_Empty(t *testing.T) {
	cl := NewCompositeLoader()
	docs, err := cl.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空 CompositeLoader 不应报错: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("空 CompositeLoader 应返回 0 个文档, 实际 %d", len(docs))
	}
}

// ============== YAMLLoader 测试 ==============

// TestYAMLLoader_Load 加载 YAML 文件
func TestYAMLLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "key: value\nlist:\n  - item1\n  - item2"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewYAMLLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != content {
		t.Errorf("Content 不匹配")
	}
	if doc.Metadata["loader"] != "yaml" {
		t.Errorf("Metadata[loader] 应为 yaml")
	}
	if doc.Metadata["file_name"] != "config.yaml" {
		t.Errorf("Metadata[file_name] 应为 config.yaml")
	}
}

// TestYAMLLoader_Name 验证名称
func TestYAMLLoader_Name(t *testing.T) {
	l := NewYAMLLoader("test.yaml")
	if got := l.Name(); got != "YAMLLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "YAMLLoader")
	}
}

// TestYAMLLoader_Load_FileNotFound 文件不存在
func TestYAMLLoader_Load_FileNotFound(t *testing.T) {
	l := NewYAMLLoader("/nonexistent/file.yaml")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("文件不存在应返回错误")
	}
}

// TestYAMLLoader_Options 选项设置
func TestYAMLLoader_Options(t *testing.T) {
	l := NewYAMLLoader("test.yaml",
		WithYAMLContentKey("body"),
		WithMultiDoc(true),
	)
	if l.contentKey != "body" {
		t.Errorf("contentKey 应为 body, 实际 %s", l.contentKey)
	}
	if !l.isMultiDoc {
		t.Error("isMultiDoc 应为 true")
	}
}

// ============== S3Loader 测试 ==============

// TestS3Loader_Name 验证名称
func TestS3Loader_Name(t *testing.T) {
	l := NewS3Loader("my-bucket")
	if got := l.Name(); got != "S3Loader" {
		t.Errorf("Name() = %q, 期望 %q", got, "S3Loader")
	}
}

// TestS3Loader_Options 选项设置
func TestS3Loader_Options(t *testing.T) {
	l := NewS3Loader("bucket",
		WithS3Prefix("docs/"),
		WithS3Region("us-west-2"),
		WithS3Extensions([]string{".txt", ".md"}),
	)
	if l.prefix != "docs/" {
		t.Errorf("prefix 应为 docs/, 实际 %s", l.prefix)
	}
	if l.region != "us-west-2" {
		t.Errorf("region 应为 us-west-2, 实际 %s", l.region)
	}
	if len(l.extensions) != 2 {
		t.Errorf("extensions 长度应为 2, 实际 %d", len(l.extensions))
	}
}

// TestS3Loader_Load_Placeholder 占位实现应返回错误
func TestS3Loader_Load_Placeholder(t *testing.T) {
	l := NewS3Loader("my-bucket")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("S3Loader 占位实现应返回错误")
	}
}

// ============== DatabaseLoader 测试 ==============

// TestDatabaseLoader_Name 验证名称
func TestDatabaseLoader_Name(t *testing.T) {
	l := NewDatabaseLoader("postgres", "dsn")
	if got := l.Name(); got != "DatabaseLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "DatabaseLoader")
	}
}

// TestDatabaseLoader_Options 选项设置
func TestDatabaseLoader_Options(t *testing.T) {
	l := NewDatabaseLoader("mysql", "user:pass@tcp(localhost:3306)/db",
		WithDBQuery("SELECT * FROM docs"),
		WithDBContentColumn("body"),
		WithDBMetadataColumns([]string{"title", "author"}),
	)
	if l.query != "SELECT * FROM docs" {
		t.Errorf("query 不匹配")
	}
	if l.contentCol != "body" {
		t.Errorf("contentCol 应为 body")
	}
	if len(l.metadataCols) != 2 {
		t.Errorf("metadataCols 长度应为 2")
	}
}

// TestDatabaseLoader_Load_Placeholder 占位实现应返回错误
func TestDatabaseLoader_Load_Placeholder(t *testing.T) {
	l := NewDatabaseLoader("sqlite", "test.db")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("DatabaseLoader 占位实现应返回错误")
	}
}

// ============== GitHubLoader (loader_extended.go) 测试 ==============

// TestGitHubLoader_Name 验证名称
func TestGitHubLoader_Name(t *testing.T) {
	l := NewGitHubLoader("owner", "repo")
	if got := l.Name(); got != "GitHubLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "GitHubLoader")
	}
}

// TestGitHubLoader_Options 选项设置
func TestGitHubLoader_Options(t *testing.T) {
	l := NewGitHubLoader("owner", "repo",
		WithGitHubBranch("develop"),
		WithGitHubPath("src/"),
		WithGitHubExtensions([]string{".go", ".md"}),
		WithGitHubToken("ghp_xxx"),
	)
	if l.branch != "develop" {
		t.Errorf("branch 应为 develop, 实际 %s", l.branch)
	}
	if l.path != "src/" {
		t.Errorf("path 应为 src/, 实际 %s", l.path)
	}
	if len(l.extensions) != 2 {
		t.Errorf("extensions 长度应为 2")
	}
	if l.token != "ghp_xxx" {
		t.Errorf("token 不匹配")
	}
}

// TestGitHubLoader_DefaultBranch 默认分支
func TestGitHubLoader_DefaultBranch(t *testing.T) {
	l := NewGitHubLoader("owner", "repo")
	if l.branch != "main" {
		t.Errorf("默认分支应为 main, 实际 %s", l.branch)
	}
}

// ============== NotionLoader 测试 ==============

// TestNotionLoader_Name 验证名称
func TestNotionLoader_Name(t *testing.T) {
	l := NewNotionLoader("api-key")
	if got := l.Name(); got != "NotionLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "NotionLoader")
	}
}

// TestNotionLoader_Options 选项设置
func TestNotionLoader_Options(t *testing.T) {
	l := NewNotionLoader("key",
		WithNotionDatabaseID("db-123"),
		WithNotionPageID("page-456"),
	)
	if l.databaseID != "db-123" {
		t.Errorf("databaseID 不匹配")
	}
	if l.pageID != "page-456" {
		t.Errorf("pageID 不匹配")
	}
}

// TestNotionLoader_Load_NeitherPageNorDB 既没有 pageID 也没有 databaseID
func TestNotionLoader_Load_NeitherPageNorDB(t *testing.T) {
	l := NewNotionLoader("key")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("没有指定 pageID 或 databaseID 应返回错误")
	}
}

// ============== SlackLoader 测试 ==============

// TestSlackLoader_Name 验证名称
func TestSlackLoader_Name(t *testing.T) {
	l := NewSlackLoader("token")
	if got := l.Name(); got != "SlackLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "SlackLoader")
	}
}

// TestSlackLoader_Options 选项设置
func TestSlackLoader_Options(t *testing.T) {
	l := NewSlackLoader("token",
		WithSlackChannelID("C123"),
		WithSlackLimit(50),
	)
	if l.channelID != "C123" {
		t.Errorf("channelID 不匹配")
	}
	if l.limit != 50 {
		t.Errorf("limit 应为 50, 实际 %d", l.limit)
	}
}

// TestSlackLoader_Load_NoChannelID 没有 channelID 应返回错误
func TestSlackLoader_Load_NoChannelID(t *testing.T) {
	l := NewSlackLoader("token")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("没有 channelID 应返回错误")
	}
}

// ============== HTMLLoader 测试 ==============

// TestHTMLLoader_Load 加载 HTML 文件
func TestHTMLLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	htmlContent := `<html><head><title>Test Page</title></head><body><p>Hello World</p></body></html>`
	if err := os.WriteFile(path, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	l := NewHTMLLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]
	if !strings.Contains(doc.Content, "Hello World") {
		t.Error("Content 应包含 Hello World")
	}
	if doc.Metadata["title"] != "Test Page" {
		t.Errorf("Metadata[title] 应为 Test Page, 实际 %v", doc.Metadata["title"])
	}
	if doc.Metadata["loader"] != "html" {
		t.Errorf("Metadata[loader] 应为 html")
	}
}

// TestHTMLLoader_Name 验证名称
func TestHTMLLoader_Name(t *testing.T) {
	l := NewHTMLLoader("test.html")
	if got := l.Name(); got != "HTMLLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "HTMLLoader")
	}
}

// TestHTMLLoader_RemoveScripts 移除脚本
func TestHTMLLoader_RemoveScripts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.html")
	htmlContent := `<html><body><p>Text</p><script>alert('xss')</script><p>More</p></body></html>`
	os.WriteFile(path, []byte(htmlContent), 0644)

	l := NewHTMLLoader(path, WithHTMLRemoveScripts(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if strings.Contains(docs[0].Content, "alert") {
		t.Error("脚本内容应被移除")
	}
	if !strings.Contains(docs[0].Content, "Text") {
		t.Error("正文应保留")
	}
}

// TestHTMLLoader_RemoveStyles 移除样式
func TestHTMLLoader_RemoveStyles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "style.html")
	htmlContent := `<html><head><style>body{color:red}</style></head><body><p>Content</p></body></html>`
	os.WriteFile(path, []byte(htmlContent), 0644)

	l := NewHTMLLoader(path, WithHTMLRemoveStyles(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if strings.Contains(docs[0].Content, "color:red") {
		t.Error("样式内容应被移除")
	}
}

// TestHTMLLoader_FromReader 从 Reader 加载
func TestHTMLLoader_FromReader(t *testing.T) {
	htmlContent := `<html><head><title>Reader Title</title></head><body>Reader Body</body></html>`
	reader := strings.NewReader(htmlContent)

	l := NewHTMLLoaderFromReader(reader, "http://example.com")
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	doc := docs[0]
	if doc.Source != "http://example.com" {
		t.Errorf("Source 应为 URL: %s", doc.Source)
	}
	if doc.Metadata["url"] != "http://example.com" {
		t.Errorf("Metadata[url] 不匹配")
	}
	if doc.Metadata["title"] != "Reader Title" {
		t.Errorf("Metadata[title] 应为 Reader Title")
	}
}

// TestHTMLLoader_Options HTML 选项
func TestHTMLLoader_Options(t *testing.T) {
	l := NewHTMLLoader("test.html",
		WithHTMLRemoveScripts(false),
		WithHTMLRemoveStyles(false),
		WithHTMLExtractTitle(false),
	)
	if l.removeScripts {
		t.Error("removeScripts 应为 false")
	}
	if l.removeStyles {
		t.Error("removeStyles 应为 false")
	}
	if l.extractTitle {
		t.Error("extractTitle 应为 false")
	}
}

// ============== CSVLoader 测试 ==============

// TestCSVLoader_Load 加载 CSV 文件
func TestCSVLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "name,age,city\nAlice,30,Beijing\nBob,25,Shanghai"
	os.WriteFile(path, []byte(csvContent), 0644)

	l := NewCSVLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("期望 2 个文档（2 行数据）, 实际 %d", len(docs))
	}

	// 默认 contentColumn 为第一列(index 0)
	if docs[0].Content != "Alice" {
		t.Errorf("第 1 行 Content 应为 Alice, 实际 %q", docs[0].Content)
	}
	if docs[0].Metadata["loader"] != "csv" {
		t.Errorf("Metadata[loader] 应为 csv")
	}
}

// TestCSVLoader_Name 验证名称
func TestCSVLoader_Name(t *testing.T) {
	l := NewCSVLoader("test.csv")
	if got := l.Name(); got != "CSVLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "CSVLoader")
	}
}

// TestCSVLoader_Options 选项设置
func TestCSVLoader_Options(t *testing.T) {
	l := NewCSVLoader("test.csv",
		WithCSVSeparator('\t'),
		WithCSVHeader(false),
		WithCSVContentColumn("body"),
		WithCSVMetadataColumns([]string{"title"}),
	)
	if l.separator != '\t' {
		t.Error("separator 应为 tab")
	}
	if l.hasHeader {
		t.Error("hasHeader 应为 false")
	}
	if l.contentColumn != "body" {
		t.Errorf("contentColumn 应为 body")
	}
}

// TestCSVLoader_ContentColumn 指定内容列
func TestCSVLoader_ContentColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "id,text,label\n1,Hello World,pos\n2,Goodbye,neg"
	os.WriteFile(path, []byte(csvContent), 0644)

	l := NewCSVLoader(path, WithCSVContentColumn("text"))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if docs[0].Content != "Hello World" {
		t.Errorf("指定 text 列后 Content 应为 Hello World, 实际 %q", docs[0].Content)
	}
}

// ============== JSONLoader 测试 ==============

// TestJSONLoader_Load 加载 JSON 文件
func TestJSONLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	jsonContent := `{"name":"test","items":[1,2,3]}`
	os.WriteFile(path, []byte(jsonContent), 0644)

	l := NewJSONLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != jsonContent {
		t.Errorf("Content 不匹配")
	}
	if docs[0].Metadata["loader"] != "json" {
		t.Errorf("Metadata[loader] 应为 json")
	}
}

// TestJSONLoader_Name 验证名称
func TestJSONLoader_Name(t *testing.T) {
	l := NewJSONLoader("test.json")
	if got := l.Name(); got != "JSONLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "JSONLoader")
	}
}

// TestJSONLoader_Options 选项设置
func TestJSONLoader_Options(t *testing.T) {
	l := NewJSONLoader("test.json",
		WithJSONContentKey("body"),
		WithJSONMetadataKeys([]string{"title", "author"}),
	)
	if l.contentKey != "body" {
		t.Errorf("contentKey 应为 body")
	}
	if len(l.metadataKeys) != 2 {
		t.Errorf("metadataKeys 长度应为 2")
	}
}

// TestJSONLoader_Load_FileNotFound 文件不存在
func TestJSONLoader_Load_FileNotFound(t *testing.T) {
	l := NewJSONLoader("/nonexistent/data.json")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("文件不存在应返回错误")
	}
}

// ============== PDFLoader 测试 ==============

// TestPDFLoader_Name 验证名称
func TestPDFLoader_Name(t *testing.T) {
	l := NewPDFLoader("test.pdf")
	if got := l.Name(); got != "PDFLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "PDFLoader")
	}
}

// TestPDFLoader_Options 选项设置
func TestPDFLoader_Options(t *testing.T) {
	l := NewPDFLoader("test.pdf",
		WithPDFSplitPages(true),
		WithPDFPageRange(2, 5),
		WithPDFPassword("secret"),
		WithPDFExtractMetadata(false),
	)
	if !l.splitPages {
		t.Error("splitPages 应为 true")
	}
	if l.startPage != 2 {
		t.Errorf("startPage 应为 2, 实际 %d", l.startPage)
	}
	if l.endPage != 5 {
		t.Errorf("endPage 应为 5, 实际 %d", l.endPage)
	}
	if l.password != "secret" {
		t.Errorf("password 不匹配")
	}
	if l.extractMetadata {
		t.Error("extractMetadata 应为 false")
	}
}

// TestPDFLoader_Load_FileNotFound 文件不存在
func TestPDFLoader_Load_FileNotFound(t *testing.T) {
	l := NewPDFLoader("/nonexistent/doc.pdf")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("文件不存在应返回错误")
	}
}

// TestPDFLoader_CustomParser 自定义 PDF 解析器
func TestPDFLoader_CustomParser(t *testing.T) {
	parser := &mockPDFParser{
		pages: []string{"Page 1 content", "Page 2 content"},
	}
	l := NewPDFLoaderFromReader(strings.NewReader("fake pdf"), WithPDFParser(parser))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档（非分页模式）, 实际 %d", len(docs))
	}
	if !strings.Contains(docs[0].Content, "Page 1 content") {
		t.Error("Content 应包含 Page 1 content")
	}
	if !strings.Contains(docs[0].Content, "Page 2 content") {
		t.Error("Content 应包含 Page 2 content")
	}
}

// TestPDFLoader_CustomParser_SplitPages 分页模式
func TestPDFLoader_CustomParser_SplitPages(t *testing.T) {
	parser := &mockPDFParser{
		pages: []string{"Page 1", "Page 2", "Page 3"},
	}
	l := NewPDFLoaderFromReader(strings.NewReader("fake"),
		WithPDFParser(parser),
		WithPDFSplitPages(true),
	)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("分页模式期望 3 个文档, 实际 %d", len(docs))
	}
}

// TestPDFLoader_CustomParser_PageRange 页面范围
func TestPDFLoader_CustomParser_PageRange(t *testing.T) {
	parser := &mockPDFParser{
		pages: []string{"P1", "P2", "P3", "P4", "P5"},
	}
	l := NewPDFLoaderFromReader(strings.NewReader("fake"),
		WithPDFParser(parser),
		WithPDFSplitPages(true),
		WithPDFPageRange(2, 4),
	)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("页面范围 2-4 期望 3 个文档, 实际 %d", len(docs))
	}
}

// mockPDFParser 用于测试的 PDF 解析器
type mockPDFParser struct {
	pages []string
}

func (p *mockPDFParser) Parse(ctx context.Context, r io.Reader) (*PDFDocument, error) {
	return &PDFDocument{
		Pages: p.pages,
		Metadata: PDFMetadata{
			PageCount: len(p.pages),
			Title:     "Test PDF",
		},
	}, nil
}

// ============== DOCXLoader 测试 ==============

// TestDOCXLoader_Name 验证名称
func TestDOCXLoader_Name(t *testing.T) {
	l := NewDOCXLoader("test.docx")
	if got := l.Name(); got != "DOCXLoader" {
		t.Errorf("Name() = %q, 期望 %q", got, "DOCXLoader")
	}
}

// TestDOCXLoader_Options 选项设置
func TestDOCXLoader_Options(t *testing.T) {
	l := NewDOCXLoader("test.docx",
		WithDOCXPreserveParagraphs(false),
		WithDOCXExtractMetadata(false),
		WithDOCXSplitByHeading(true),
	)
	if l.preserveParagraphs {
		t.Error("preserveParagraphs 应为 false")
	}
	if l.extractMetadata {
		t.Error("extractMetadata 应为 false")
	}
	if !l.splitByHeading {
		t.Error("splitByHeading 应为 true")
	}
}

// TestDOCXLoader_Load_FileNotFound 文件不存在
func TestDOCXLoader_Load_FileNotFound(t *testing.T) {
	l := NewDOCXLoader("/nonexistent/doc.docx")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("文件不存在应返回错误")
	}
}

// ============== Connector 测试 ==============

// TestGitHubConnector_Name 验证名称
func TestGitHubConnector_Name(t *testing.T) {
	c := NewGitHubConnector(&GitHubConfig{
		Owner: "owner",
		Repo:  "repo",
	})
	if got := c.Name(); got != "github" {
		t.Errorf("Name() = %q, 期望 %q", got, "github")
	}
}

// TestGitHubConnector_DefaultBranch 默认分支
func TestGitHubConnector_DefaultBranch(t *testing.T) {
	c := NewGitHubConnector(&GitHubConfig{
		Owner: "owner",
		Repo:  "repo",
	})
	if c.branch != "main" {
		t.Errorf("默认分支应为 main, 实际 %s", c.branch)
	}
}

// TestNotionConnector_Name 验证名称
func TestNotionConnector_Name(t *testing.T) {
	c := NewNotionConnector(&NotionConfig{Token: "token"})
	if got := c.Name(); got != "notion" {
		t.Errorf("Name() = %q, 期望 %q", got, "notion")
	}
}

// TestNotionConnector_Load_NoIDs 没有指定 ID
func TestNotionConnector_Load_NoIDs(t *testing.T) {
	c := NewNotionConnector(&NotionConfig{Token: "token"})
	_, err := c.Load(context.Background())
	if err == nil {
		t.Error("没有指定 pageID 或 databaseID 应返回错误")
	}
}

// TestSlackConnector_Name 验证名称
func TestSlackConnector_Name(t *testing.T) {
	c := NewSlackConnector(&SlackConfig{Token: "token", ChannelID: "C123"})
	if got := c.Name(); got != "slack" {
		t.Errorf("Name() = %q, 期望 %q", got, "slack")
	}
}

// TestDatabaseConnector_Name 验证名称
func TestDatabaseConnector_Name(t *testing.T) {
	c := NewDatabaseConnector(&DatabaseConfig{})
	if got := c.Name(); got != "database" {
		t.Errorf("Name() = %q, 期望 %q", got, "database")
	}
}

// TestDatabaseConnector_Load_NilDB DB 为 nil
func TestDatabaseConnector_Load_NilDB(t *testing.T) {
	c := NewDatabaseConnector(&DatabaseConfig{})
	_, err := c.Load(context.Background())
	if err == nil {
		t.Error("DB 为 nil 应返回错误")
	}
}

// TestWebAPIConnector_Name 验证名称
func TestWebAPIConnector_Name(t *testing.T) {
	c := NewWebAPIConnector(&WebAPIConfig{URL: "http://api.example.com"})
	if got := c.Name(); got != "web_api" {
		t.Errorf("Name() = %q, 期望 %q", got, "web_api")
	}
}

// TestWebAPIConnector_DefaultMethod 默认方法
func TestWebAPIConnector_DefaultMethod(t *testing.T) {
	c := NewWebAPIConnector(&WebAPIConfig{URL: "http://api.example.com"})
	if c.method != "GET" {
		t.Errorf("默认方法应为 GET, 实际 %s", c.method)
	}
}

// TestWebAPIConnector_Load_Success 成功调用 Web API
func TestWebAPIConnector_Load_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":1,"text":"item1"},{"id":2,"text":"item2"}]}`))
	}))
	defer server.Close()

	c := NewWebAPIConnector(&WebAPIConfig{URL: server.URL})
	docs, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) < 1 {
		t.Error("应至少返回 1 个文档")
	}
}

// TestWebAPIConnector_Load_PlainText 非 JSON 响应
func TestWebAPIConnector_Load_PlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain text response"))
	}))
	defer server.Close()

	c := NewWebAPIConnector(&WebAPIConfig{URL: server.URL})
	docs, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "plain text response" {
		t.Errorf("Content 不匹配: %q", docs[0].Content)
	}
}

// TestWebAPIConnector_Load_AuthError 认证错误
func TestWebAPIConnector_Load_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewWebAPIConnector(&WebAPIConfig{URL: server.URL})
	_, err := c.Load(context.Background())
	if err == nil {
		t.Error("401 应返回错误")
	}
}

// ============== 辅助函数: HTML 解析测试 ==============

// TestStripHTMLTags 移除 HTML 标签
func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", " Hello "},
		{"<b>Bold</b> text", " Bold  text"},
		{"No tags", "No tags"},
		{"<div><p>Nested</p></div>", "  Nested  "},
	}

	for _, tt := range tests {
		result := stripHTMLTags(tt.input)
		if result != tt.expected {
			t.Errorf("stripHTMLTags(%q) = %q, 期望 %q", tt.input, result, tt.expected)
		}
	}
}

// TestCleanWhitespace 清理空白
func TestCleanWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello  World  ", "Hello World"},
		{"No\n\nextra\t\tspaces", "No extra spaces"},
		{"already clean", "already clean"},
		{"   ", ""},
	}

	for _, tt := range tests {
		result := cleanWhitespace(tt.input)
		if result != tt.expected {
			t.Errorf("cleanWhitespace(%q) = %q, 期望 %q", tt.input, result, tt.expected)
		}
	}
}

// TestRemoveHTMLTag 移除指定 HTML 标签
func TestRemoveHTMLTag(t *testing.T) {
	input := `<p>Text</p><script>alert('x')</script><p>More</p>`
	result := removeHTMLTag(input, "script")
	if strings.Contains(result, "alert") {
		t.Errorf("script 标签应被移除: %q", result)
	}
	if !strings.Contains(result, "Text") || !strings.Contains(result, "More") {
		t.Error("非目标标签应保留")
	}
}

// TestExtractHTMLTag 提取 HTML 标签内容
func TestExtractHTMLTag(t *testing.T) {
	html := `<html><head><title>My Title</title></head><body>Body</body></html>`

	title := extractHTMLTag(html, "title")
	if title != "My Title" {
		t.Errorf("提取 title 失败: %q", title)
	}

	body := extractHTMLTag(html, "body")
	if body != "Body" {
		t.Errorf("提取 body 失败: %q", body)
	}

	// 不存在的标签
	result := extractHTMLTag(html, "footer")
	if result != "" {
		t.Errorf("不存在的标签应返回空: %q", result)
	}
}

// TestSplitCSVLine CSV 行分割
func TestSplitCSVLine(t *testing.T) {
	tests := []struct {
		line   string
		sep    rune
		expect []string
	}{
		{"a,b,c", ',', []string{"a", "b", "c"}},
		{`"hello, world",b,c`, ',', []string{"hello, world", "b", "c"}},
		{"x\ty\tz", '\t', []string{"x", "y", "z"}},
		{"single", ',', []string{"single"}},
	}

	for _, tt := range tests {
		result := splitCSVLine(tt.line, tt.sep)
		if len(result) != len(tt.expect) {
			t.Errorf("splitCSVLine(%q) 长度 %d, 期望 %d", tt.line, len(result), len(tt.expect))
			continue
		}
		for i := range tt.expect {
			if result[i] != tt.expect[i] {
				t.Errorf("splitCSVLine(%q)[%d] = %q, 期望 %q", tt.line, i, result[i], tt.expect[i])
			}
		}
	}
}

// ============== redirectTransport 用于拦截 HTTP 请求到测试服务器 ==============

// redirectTransport 将所有 HTTP 请求重定向到 httptest.Server
type redirectTransport struct {
	server *httptest.Server
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = rt.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// ============== GitHubConnector HTTP 测试 ==============

// TestGitHubConnector_Load_Files 测试加载文件列表
func TestGitHubConnector_Load_Files(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/contents/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "file1.go", "path": "src/file1.go", "type": "file", "download_url": "http://" + r.Host + "/raw/file1.go", "size": 100},
				{"name": "dir1", "path": "src/dir1", "type": "dir", "download_url": "", "size": 0},
			})
		} else if strings.Contains(r.URL.Path, "/raw/") {
			w.Write([]byte("package main\nfunc main() {}"))
		}
	}))
	defer server.Close()

	gc := NewGitHubConnector(&GitHubConfig{Owner: "o", Repo: "r", Token: "tok"})
	gc.client = &http.Client{Transport: &redirectTransport{server: server}}

	docs, err := gc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档(跳过 dir), 实际 %d", len(docs))
	}
	if docs[0].Content != "package main\nfunc main() {}" {
		t.Errorf("内容不匹配: %q", docs[0].Content)
	}
}

// TestGitHubConnector_Load_SingleFile 测试加载单个文件 (API 返回对象而非数组)
func TestGitHubConnector_Load_SingleFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/contents/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"name": "README.md", "path": "README.md", "content": "base64content",
				"encoding": "base64", "download_url": "http://" + r.Host + "/raw/README.md",
			})
		} else {
			w.Write([]byte("# README"))
		}
	}))
	defer server.Close()

	gc := NewGitHubConnector(&GitHubConfig{Owner: "o", Repo: "r"})
	gc.client = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := gc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1, 实际 %d", len(docs))
	}
}

// TestGitHubConnector_Load_Issues 测试加载 Issues
func TestGitHubConnector_Load_Issues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"number": 1, "title": "Bug Report", "body": "Something broke",
				"state": "open", "user": map[string]string{"login": "alice"},
				"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-02T00:00:00Z",
				"labels": []map[string]string{{"name": "bug"}},
			},
		})
	}))
	defer server.Close()

	gc := NewGitHubConnector(&GitHubConfig{Owner: "o", Repo: "r", LoadType: GitHubLoadIssues})
	gc.client = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := gc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个 issue, 实际 %d", len(docs))
	}
	if !strings.Contains(docs[0].Content, "Bug Report") {
		t.Error("内容应包含标题")
	}
}

// TestGitHubConnector_Load_PRs 测试加载 Pull Requests
func TestGitHubConnector_Load_PRs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"number": 42, "title": "Add feature", "body": "New feature description",
				"state": "merged", "user": map[string]string{"login": "bob"},
				"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-03T00:00:00Z",
				"merged_at": "2024-01-03T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	gc := NewGitHubConnector(&GitHubConfig{Owner: "o", Repo: "r", LoadType: GitHubLoadPRs})
	gc.client = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := gc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个 PR, 实际 %d", len(docs))
	}
}

// TestGitHubConnector_doRequest_Errors 测试 doRequest 各种错误码
func TestGitHubConnector_doRequest_Errors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{"auth_401", 401, ErrAuthFailed},
		{"auth_403", 403, ErrAuthFailed},
		{"not_found", 404, ErrNotFound},
		{"rate_limited", 429, ErrRateLimited},
		{"server_error", 500, ErrConnectorFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			gc := NewGitHubConnector(&GitHubConfig{Owner: "o", Repo: "r", Token: "tok"})
			gc.client = &http.Client{Transport: &redirectTransport{server: server}}
			_, err := gc.Load(context.Background())
			if err == nil {
				t.Errorf("期望错误, 但成功了")
			}
		})
	}
}

// ============== NotionConnector HTTP 测试 ==============

// TestNotionConnector_Load_PageSuccess 测试加载 Notion 页面
func TestNotionConnector_Load_PageSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"type": "paragraph",
					"paragraph": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Hello from Notion"}},
					},
				},
				{
					"type": "heading_1",
					"heading_1": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Title"}},
					},
				},
				{
					"type": "heading_2",
					"heading_2": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Subtitle"}},
					},
				},
				{
					"type": "heading_3",
					"heading_3": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Section"}},
					},
				},
				{
					"type": "bulleted_list_item",
					"bulleted_list_item": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Item 1"}},
					},
				},
				{
					"type": "numbered_list_item",
					"numbered_list_item": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Step 1"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	nc := NewNotionConnector(&NotionConfig{Token: "tok", PageID: "page-123"})
	nc.client = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := nc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if !strings.Contains(docs[0].Content, "Hello from Notion") {
		t.Error("内容应包含段落文字")
	}
	if !strings.Contains(docs[0].Content, "# Title") {
		t.Error("内容应包含一级标题")
	}
}

// TestNotionConnector_loadPage_AuthFailed 测试认证失败
func TestNotionConnector_loadPage_AuthFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer server.Close()

	nc := NewNotionConnector(&NotionConfig{Token: "bad", PageID: "page-123"})
	nc.client = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := nc.Load(context.Background())
	if err == nil {
		t.Error("认证失败应返回错误")
	}
}

// TestNotionConnector_loadPage_NotFound 测试页面不存在
func TestNotionConnector_loadPage_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	nc := NewNotionConnector(&NotionConfig{Token: "tok", PageID: "no-page"})
	nc.client = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := nc.Load(context.Background())
	if err == nil {
		t.Error("404 应返回错误")
	}
}

// TestNotionConnector_loadPage_ServerError 测试服务器错误
func TestNotionConnector_loadPage_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	nc := NewNotionConnector(&NotionConfig{Token: "tok", PageID: "page-123"})
	nc.client = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := nc.Load(context.Background())
	if err == nil {
		t.Error("500 应返回错误")
	}
}

// ============== SlackConnector HTTP 测试 ==============

// TestSlackConnector_Load_Success 测试成功加载 Slack 消息
func TestSlackConnector_Load_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"messages": []map[string]any{
				{"type": "message", "user": "U123", "text": "Hello!", "ts": "1234567890.000100"},
				{"type": "message", "user": "U456", "text": "Hi!", "ts": "1234567891.000100"},
			},
		})
	}))
	defer server.Close()

	sc := NewSlackConnector(&SlackConfig{Token: "tok", ChannelID: "C123"})
	sc.client = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := sc.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 个消息, 实际 %d", len(docs))
	}
}

// TestSlackConnector_Load_InvalidAuth 测试认证失败
func TestSlackConnector_Load_InvalidAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "invalid_auth",
		})
	}))
	defer server.Close()

	sc := NewSlackConnector(&SlackConfig{Token: "bad", ChannelID: "C123"})
	sc.client = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := sc.Load(context.Background())
	if err == nil {
		t.Error("认证失败应返回错误")
	}
}

// TestSlackConnector_Load_APIError 测试 API 错误
func TestSlackConnector_Load_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "channel_not_found",
		})
	}))
	defer server.Close()

	sc := NewSlackConnector(&SlackConfig{Token: "tok", ChannelID: "C999"})
	sc.client = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := sc.Load(context.Background())
	if err == nil {
		t.Error("API 错误应返回错误")
	}
}

// ============== DatabaseConnector applyTemplate 测试 ==============

// TestDatabaseConnector_applyTemplate 测试模板替换
func TestDatabaseConnector_applyTemplate(t *testing.T) {
	dc := NewDatabaseConnector(&DatabaseConfig{
		Template: "名称: {{.name}}, 年龄: {{.age}}",
	})
	result := dc.applyTemplate(dc.template, map[string]any{
		"name": "Alice",
		"age":  30,
	})
	if !strings.Contains(result, "Alice") || !strings.Contains(result, "30") {
		t.Errorf("模板替换失败: %q", result)
	}
}

// ============== GitHubLoader HTTP 测试 ==============

// TestGitHubLoader_Load_Success 测试 GitHubLoader 加载
func TestGitHubLoader_Load_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/git/trees/") {
			json.NewEncoder(w).Encode(map[string]any{
				"tree": []map[string]any{
					{"path": "README.md", "type": "blob", "url": "..."},
					{"path": "src", "type": "tree", "url": "..."},
					{"path": "src/main.go", "type": "blob", "url": "..."},
				},
			})
		} else {
			// raw content
			w.Write([]byte("file content for " + r.URL.Path))
		}
	}))
	defer server.Close()

	l := NewGitHubLoader("owner", "repo", WithGitHubToken("tok"))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 个文件(跳过 tree), 实际 %d", len(docs))
	}
}

// TestGitHubLoader_Load_WithFilters 测试扩展名过滤和路径过滤
func TestGitHubLoader_Load_WithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/git/trees/") {
			json.NewEncoder(w).Encode(map[string]any{
				"tree": []map[string]any{
					{"path": "src/main.go", "type": "blob"},
					{"path": "src/util.go", "type": "blob"},
					{"path": "docs/README.md", "type": "blob"},
				},
			})
		} else {
			w.Write([]byte("content"))
		}
	}))
	defer server.Close()

	l := NewGitHubLoader("owner", "repo",
		WithGitHubPath("src"),
		WithGitHubExtensions([]string{".go"}),
		WithGitHubBranch("dev"),
	)
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 个 .go 文件在 src 下, 实际 %d", len(docs))
	}
}

// TestGitHubLoader_Load_APIError 测试 API 错误
func TestGitHubLoader_Load_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	l := NewGitHubLoader("owner", "repo")
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("API 错误应返回错误")
	}
}

// ============== NotionLoader HTTP 测试 ==============

// TestNotionLoader_loadPage_Success 测试 NotionLoader 加载页面
func TestNotionLoader_loadPage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"type": "paragraph",
					"paragraph": map[string]any{
						"rich_text": []map[string]string{{"plain_text": "Page content"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	l := NewNotionLoader("api-key", WithNotionPageID("page-1"))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if !strings.Contains(docs[0].Content, "Page content") {
		t.Error("内容不匹配")
	}
}

// TestNotionLoader_loadDatabase_Success 测试 NotionLoader 加载数据库
func TestNotionLoader_loadDatabase_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/databases/") {
			// 数据库查询返回页面 ID 列表
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": "page-a"},
					{"id": "page-b"},
				},
			})
		} else {
			// 页面内容
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"type": "paragraph",
						"paragraph": map[string]any{
							"rich_text": []map[string]string{{"plain_text": "DB page content"}},
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	l := NewNotionLoader("api-key", WithNotionDatabaseID("db-1"))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档, 实际 %d", len(docs))
	}
}

// TestNotionLoader_Load_NoIDError 没有指定任何 ID
func TestNotionLoader_Load_NoIDError(t *testing.T) {
	l := NewNotionLoader("api-key")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("没有指定 ID 应返回错误")
	}
}

// TestNotionLoader_loadPage_APIError 测试 API 错误
func TestNotionLoader_loadPage_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	l := NewNotionLoader("api-key", WithNotionPageID("page-1"))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("API 错误应返回错误")
	}
}

// ============== SlackLoader HTTP 测试 ==============

// TestSlackLoader_Load_Success 测试 SlackLoader 成功加载
func TestSlackLoader_Load_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"messages": []map[string]any{
				{"text": "msg1", "user": "U1", "ts": "1000.1"},
				{"text": "msg2", "user": "U2", "ts": "1000.2"},
			},
		})
	}))
	defer server.Close()

	l := NewSlackLoader("tok", WithSlackChannelID("C123"), WithSlackLimit(50))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 个消息, 实际 %d", len(docs))
	}
}

// TestSlackLoader_Load_NoChannel 测试没有频道 ID
func TestSlackLoader_Load_NoChannel(t *testing.T) {
	l := NewSlackLoader("tok")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("没有频道 ID 应返回错误")
	}
}

// TestSlackLoader_Load_APIError 测试 Slack API 错误
func TestSlackLoader_Load_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "channel_not_found",
		})
	}))
	defer server.Close()

	l := NewSlackLoader("tok", WithSlackChannelID("C999"))
	l.httpClient = &http.Client{Transport: &redirectTransport{server: server}}
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("API 错误应返回错误")
	}
}

// ============== SimplePDFParser 测试 ==============

// TestSimplePDFParser_Parse_ValidPDF 测试解析有效 PDF
func TestSimplePDFParser_Parse_ValidPDF(t *testing.T) {
	// 构造包含文本的简单 PDF 数据
	fakePDF := []byte("%PDF-1.4\n1 0 obj\n(Hello World) Tj\n(Second Line) Tj\nendobj\n%%EOF")

	parser := &SimplePDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(fakePDF))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if len(doc.Pages) == 0 {
		t.Fatal("应至少有一页")
	}
	combined := strings.Join(doc.Pages, " ")
	if !strings.Contains(combined, "Hello World") {
		t.Errorf("应包含提取的文字, 实际: %q", combined)
	}
}

// TestSimplePDFParser_Parse_InvalidPDF 测试解析无效 PDF
func TestSimplePDFParser_Parse_InvalidPDF(t *testing.T) {
	parser := &SimplePDFParser{}
	_, err := parser.Parse(context.Background(), bytes.NewReader([]byte("not a pdf")))
	if err == nil {
		t.Error("无效 PDF 应返回错误")
	}
}

// TestSimplePDFParser_Parse_EmptyText 测试没有可提取文本的 PDF
func TestSimplePDFParser_Parse_EmptyText(t *testing.T) {
	// PDF 头有效但无文本
	fakePDF := []byte("%PDF-1.4\n%%EOF")
	parser := &SimplePDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(fakePDF))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	// 应返回占位文本
	if len(doc.Pages) == 0 {
		t.Fatal("应至少有一页(占位)")
	}
}

// TestSimplePDFParser_extractTextFromPDF_EscapeChars 测试转义字符处理
func TestSimplePDFParser_extractTextFromPDF_EscapeChars(t *testing.T) {
	parser := &SimplePDFParser{}
	data := []byte("%PDF-1.4\n(Line1\\nLine2\\tTab\\rReturn\\\\Backslash) Tj\n%%EOF")
	pages := parser.extractTextFromPDF(data)
	if len(pages) == 0 {
		t.Fatal("应至少提取一页")
	}
	text := pages[0]
	if !strings.Contains(text, "Line1") {
		t.Errorf("应包含 Line1: %q", text)
	}
}

// ============== DOCXLoader 测试 ==============

// createTestDOCX 创建测试用的 DOCX 内存文件
func createTestDOCX(docXML, coreXML string) *bytes.Reader {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	if docXML != "" {
		f, _ := w.Create("word/document.xml")
		f.Write([]byte(docXML))
	}

	if coreXML != "" {
		f, _ := w.Create("docProps/core.xml")
		f.Write([]byte(coreXML))
	}

	w.Close()
	return bytes.NewReader(buf.Bytes())
}

// TestDOCXLoader_Load_FromReader 测试从 Reader 加载 DOCX
func TestDOCXLoader_Load_FromReader(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Hello World</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second paragraph</w:t></w:r></w:p>
  </w:body>
</w:document>`

	coreXML := `<?xml version="1.0" encoding="UTF-8"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Test Document</dc:title>
  <dc:creator>Test Author</dc:creator>
  <dc:subject>Test Subject</dc:subject>
</cp:coreProperties>`

	reader := createTestDOCX(docXML, coreXML)
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()))

	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if !strings.Contains(docs[0].Content, "Hello World") {
		t.Errorf("内容应包含 Hello World: %q", docs[0].Content)
	}
}

// TestDOCXLoader_Load_SplitByHeading 测试按标题分割
func TestDOCXLoader_Load_SplitByHeading(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chapter 1</w:t></w:r></w:p>
    <w:p><w:r><w:t>Content of chapter 1</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Chapter 2</w:t></w:r></w:p>
    <w:p><w:r><w:t>Content of chapter 2</w:t></w:r></w:p>
  </w:body>
</w:document>`

	reader := createTestDOCX(docXML, "")
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()), WithDOCXSplitByHeading(true))

	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) < 2 {
		t.Fatalf("期望至少 2 个文档(按标题分割), 实际 %d", len(docs))
	}
}

// TestDOCXLoader_Load_NoDocumentXML 测试缺少 document.xml
func TestDOCXLoader_Load_NoDocumentXML(t *testing.T) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	f, _ := w.Create("other.txt")
	f.Write([]byte("not a docx"))
	w.Close()

	reader := bytes.NewReader(buf.Bytes())
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()))
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("缺少 document.xml 应返回错误")
	}
}

// TestDOCXLoader_Load_InvalidXML 测试无效 XML（降级到简单提取）
func TestDOCXLoader_Load_InvalidXML(t *testing.T) {
	// 使用未转义的 & 确保 xml.Unmarshal 失败，触发 extractTextSimple 降级路径
	docXML := `<doc>&invalid<w:t>extracted text</w:t></doc>`
	reader := createTestDOCX(docXML, "")
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()),
		WithDOCXPreserveParagraphs(false),
		WithDOCXExtractMetadata(false),
	)

	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("应返回文档")
	}
	if !strings.Contains(docs[0].Content, "extracted text") {
		t.Errorf("应通过简单提取获得文字: %q", docs[0].Content)
	}
}

// TestDOCXLoader_Load_WithMetadata 测试元数据提取
func TestDOCXLoader_Load_WithMetadata(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>Content</w:t></w:r></w:p></w:body>
</w:document>`

	coreXML := `<?xml version="1.0" encoding="UTF-8"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:dcterms="http://purl.org/dc/terms/">
  <dc:title>My Title</dc:title>
  <dc:creator>Author Name</dc:creator>
  <dc:subject>Subject</dc:subject>
  <dc:description>Description</dc:description>
  <cp:keywords>key1,key2</cp:keywords>
  <dcterms:created>2024-01-01T00:00:00Z</dcterms:created>
  <dcterms:modified>2024-06-01T00:00:00Z</dcterms:modified>
</cp:coreProperties>`

	reader := createTestDOCX(docXML, coreXML)
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("应返回文档")
	}
}

// TestPDFLoader_Load_FromReader 测试从 Reader 加载 PDF
func TestPDFLoader_Load_FromReader(t *testing.T) {
	fakePDF := []byte("%PDF-1.4\n(Page one text) Tj\n%%EOF")
	l := NewPDFLoaderFromReader(bytes.NewReader(fakePDF), WithPDFSplitPages(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("应返回至少 1 个文档")
	}
}

// TestPDFLoader_Load_InvalidPageRange 测试无效页面范围
func TestPDFLoader_Load_InvalidPageRange(t *testing.T) {
	fakePDF := []byte("%PDF-1.4\n(Text) Tj\n%%EOF")
	l := NewPDFLoaderFromReader(bytes.NewReader(fakePDF), WithPDFPageRange(10, 20))
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("无效页面范围应返回错误")
	}
}

// 确保 xml 和 json 包被使用（抑制 unused import 警告）
var (
	_ = xml.Unmarshal
	_ = json.Marshal
)

// 确保 rag 和其他已导入包被使用
var _ = rag.Document{}
var _ = fmt.Sprint
var _ = filepath.Base
var _ = io.ReadAll
var _ = os.ReadFile
