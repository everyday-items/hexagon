// Package loader 提供 RAG 系统的文档加载器
//
// Loader 用于从各种来源加载文档：
//   - TextLoader: 纯文本文件
//   - MarkdownLoader: Markdown 文件
//   - DirectoryLoader: 目录批量加载
//   - URLLoader: 从 URL 加载
package loader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== TextLoader ==============

// TextLoader 纯文本文件加载器
type TextLoader struct {
	path     string
	encoding string
}

// NewTextLoader 创建文本加载器
func NewTextLoader(path string) *TextLoader {
	return &TextLoader{
		path:     path,
		encoding: "utf-8",
	}
}

// Load 加载文本文件
func (l *TextLoader) Load(ctx context.Context) ([]rag.Document, error) {
	content, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", l.path, err)
	}

	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: string(content),
		Source:  l.path,
		Metadata: map[string]any{
			"loader":    "text",
			"file_path": l.path,
			"file_name": filepath.Base(l.path),
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *TextLoader) Name() string {
	return "TextLoader"
}

var _ rag.Loader = (*TextLoader)(nil)

// ============== MarkdownLoader ==============

// MarkdownLoader Markdown 文件加载器
type MarkdownLoader struct {
	path            string
	removeImages    bool
	removeLinks     bool
	extractMetadata bool
}

// MarkdownOption Markdown 加载器选项
type MarkdownOption func(*MarkdownLoader)

// WithRemoveImages 移除图片
func WithRemoveImages(remove bool) MarkdownOption {
	return func(l *MarkdownLoader) {
		l.removeImages = remove
	}
}

// WithRemoveLinks 移除链接
func WithRemoveLinks(remove bool) MarkdownOption {
	return func(l *MarkdownLoader) {
		l.removeLinks = remove
	}
}

// WithExtractMetadata 提取 front matter 元数据
func WithExtractMetadata(extract bool) MarkdownOption {
	return func(l *MarkdownLoader) {
		l.extractMetadata = extract
	}
}

// NewMarkdownLoader 创建 Markdown 加载器
func NewMarkdownLoader(path string, opts ...MarkdownOption) *MarkdownLoader {
	l := &MarkdownLoader{
		path:            path,
		extractMetadata: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 Markdown 文件
func (l *MarkdownLoader) Load(ctx context.Context) ([]rag.Document, error) {
	content, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", l.path, err)
	}

	text := string(content)
	metadata := map[string]any{
		"loader":    "markdown",
		"file_path": l.path,
		"file_name": filepath.Base(l.path),
	}

	// 提取 front matter
	if l.extractMetadata {
		text, metadata = extractFrontMatter(text, metadata)
	}

	// 处理图片和链接
	if l.removeImages {
		text = removeMarkdownImages(text)
	}
	if l.removeLinks {
		text = removeMarkdownLinks(text)
	}

	doc := rag.Document{
		ID:        util.GenerateID("doc"),
		Content:   text,
		Source:    l.path,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *MarkdownLoader) Name() string {
	return "MarkdownLoader"
}

var _ rag.Loader = (*MarkdownLoader)(nil)

// ============== DirectoryLoader ==============

// DirectoryLoader 目录批量加载器
type DirectoryLoader struct {
	path       string
	pattern    string // glob 模式
	recursive  bool
	loaderFunc func(path string) rag.Loader
}

// DirectoryOption 目录加载器选项
type DirectoryOption func(*DirectoryLoader)

// WithPattern 设置文件匹配模式
func WithPattern(pattern string) DirectoryOption {
	return func(l *DirectoryLoader) {
		l.pattern = pattern
	}
}

// WithRecursive 设置是否递归
func WithRecursive(recursive bool) DirectoryOption {
	return func(l *DirectoryLoader) {
		l.recursive = recursive
	}
}

// WithLoaderFunc 设置自定义加载器工厂
func WithLoaderFunc(fn func(path string) rag.Loader) DirectoryOption {
	return func(l *DirectoryLoader) {
		l.loaderFunc = fn
	}
}

// NewDirectoryLoader 创建目录加载器
func NewDirectoryLoader(path string, opts ...DirectoryOption) *DirectoryLoader {
	l := &DirectoryLoader{
		path:      path,
		pattern:   "*",
		recursive: true,
		loaderFunc: func(p string) rag.Loader {
			ext := strings.ToLower(filepath.Ext(p))
			switch ext {
			case ".md", ".markdown":
				return NewMarkdownLoader(p)
			default:
				return NewTextLoader(p)
			}
		},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载目录中的所有文件
func (l *DirectoryLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var docs []rag.Document

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查 context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 跳过目录
		if info.IsDir() {
			if !l.recursive && path != l.path {
				return filepath.SkipDir
			}
			return nil
		}

		// 匹配模式
		matched, err := filepath.Match(l.pattern, filepath.Base(path))
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}

		// 加载文件
		loader := l.loaderFunc(path)
		fileDocs, err := loader.Load(ctx)
		if err != nil {
			// 记录加载失败的文件（便于排查问题），继续处理其他文件
			fmt.Fprintf(os.Stderr, "[WARN] hexagon/rag/loader: 加载文件 %s 失败: %v\n", path, err)
			return nil
		}

		docs = append(docs, fileDocs...)
		return nil
	}

	if err := filepath.Walk(l.path, walkFn); err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", l.path, err)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *DirectoryLoader) Name() string {
	return "DirectoryLoader"
}

var _ rag.Loader = (*DirectoryLoader)(nil)

// ============== URLLoader ==============

// URLLoader URL 加载器
type URLLoader struct {
	url       string
	client    *http.Client
	headers   map[string]string
	userAgent string
}

// URLOption URL 加载器选项
type URLOption func(*URLLoader)

// WithHTTPClient 设置 HTTP 客户端
func WithHTTPClient(client *http.Client) URLOption {
	return func(l *URLLoader) {
		l.client = client
	}
}

// WithHeaders 设置请求头
func WithHeaders(headers map[string]string) URLOption {
	return func(l *URLLoader) {
		l.headers = headers
	}
}

// WithUserAgent 设置 User-Agent
func WithUserAgent(ua string) URLOption {
	return func(l *URLLoader) {
		l.userAgent = ua
	}
}

// NewURLLoader 创建 URL 加载器
func NewURLLoader(url string, opts ...URLOption) *URLLoader {
	l := &URLLoader{
		url:       url,
		client:    &http.Client{Timeout: 30 * time.Second},
		userAgent: "Hexagon-RAG/1.0",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 从 URL 加载内容
func (l *URLLoader) Load(ctx context.Context) ([]rag.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", l.userAgent)
	for k, v := range l.headers {
		req.Header.Set(k, v)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL %s: %w", l.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, l.url)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 检测内容类型
	contentType := resp.Header.Get("Content-Type")

	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: string(content),
		Source:  l.url,
		Metadata: map[string]any{
			"loader":       "url",
			"url":          l.url,
			"content_type": contentType,
			"status_code":  resp.StatusCode,
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *URLLoader) Name() string {
	return "URLLoader"
}

var _ rag.Loader = (*URLLoader)(nil)

// ============== ReaderLoader ==============

// ReaderLoader 从 io.Reader 加载
type ReaderLoader struct {
	reader io.Reader
	source string
}

// NewReaderLoader 创建 Reader 加载器
func NewReaderLoader(r io.Reader, source string) *ReaderLoader {
	return &ReaderLoader{
		reader: r,
		source: source,
	}
}

// Load 从 Reader 加载
func (l *ReaderLoader) Load(ctx context.Context) ([]rag.Document, error) {
	content, err := io.ReadAll(l.reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}

	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: string(content),
		Source:  l.source,
		Metadata: map[string]any{
			"loader": "reader",
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *ReaderLoader) Name() string {
	return "ReaderLoader"
}

var _ rag.Loader = (*ReaderLoader)(nil)

// ============== StringLoader ==============

// StringLoader 从字符串加载
type StringLoader struct {
	content string
	source  string
}

// NewStringLoader 创建字符串加载器
func NewStringLoader(content, source string) *StringLoader {
	return &StringLoader{
		content: content,
		source:  source,
	}
}

// Load 从字符串加载
func (l *StringLoader) Load(ctx context.Context) ([]rag.Document, error) {
	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: l.content,
		Source:  l.source,
		Metadata: map[string]any{
			"loader": "string",
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *StringLoader) Name() string {
	return "StringLoader"
}

var _ rag.Loader = (*StringLoader)(nil)

// ============== 辅助函数 ==============

// extractFrontMatter 提取 YAML front matter
func extractFrontMatter(content string, metadata map[string]any) (string, map[string]any) {
	if !strings.HasPrefix(content, "---") {
		return content, metadata
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var frontMatter []string
	var body []string
	inFrontMatter := false
	frontMatterEnd := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			} else {
				frontMatterEnd = true
				continue
			}
		}

		if inFrontMatter && !frontMatterEnd {
			frontMatter = append(frontMatter, line)
		} else if frontMatterEnd {
			body = append(body, line)
		}
	}

	// 简单解析 front matter (key: value)
	for _, line := range frontMatter {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			metadata[key] = value
		}
	}

	return strings.Join(body, "\n"), metadata
}

// removeMarkdownImages 移除 Markdown 图片
// 使用 strings.Builder 避免 O(n²) 的循环字符串拼接
func removeMarkdownImages(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	i := 0
	for i < len(content) {
		// 查找 ![
		start := strings.Index(content[i:], "![")
		if start == -1 {
			b.WriteString(content[i:])
			break
		}
		b.WriteString(content[i : i+start])
		// 查找配对的 )
		end := strings.Index(content[i+start:], ")")
		if end == -1 {
			b.WriteString(content[i+start:])
			break
		}
		i = i + start + end + 1
	}
	return b.String()
}

// removeMarkdownLinks 移除 Markdown 链接，保留文字
// 使用 strings.Builder 避免 O(n²) 的循环字符串拼接
func removeMarkdownLinks(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	i := 0
	for i < len(content) {
		// 查找 [
		start := strings.Index(content[i:], "[")
		if start == -1 {
			b.WriteString(content[i:])
			break
		}
		// 查找 ](
		mid := strings.Index(content[i+start:], "](")
		if mid == -1 {
			b.WriteString(content[i:])
			break
		}
		// 查找 )
		end := strings.Index(content[i+start+mid:], ")")
		if end == -1 {
			b.WriteString(content[i:])
			break
		}
		// 保留 [ 之前的内容和链接文字
		b.WriteString(content[i : i+start])
		b.WriteString(content[i+start+1 : i+start+mid]) // 链接文字
		i = i + start + mid + end + 1
	}
	return b.String()
}
