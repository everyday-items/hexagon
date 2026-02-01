// Package splitter 提供 RAG 系统的文档分割器
//
// Splitter 用于将长文档分割成适合向量化的小块：
//   - CharacterSplitter: 按字符数分割
//   - RecursiveSplitter: 递归分割（优先保持语义完整）
//   - MarkdownSplitter: Markdown 专用分割（按标题分割）
//   - SentenceSplitter: 按句子分割
package splitter

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== CharacterSplitter ==============

// CharacterSplitter 按字符数分割
type CharacterSplitter struct {
	chunkSize    int
	chunkOverlap int
	separator    string
}

// CharacterOption CharacterSplitter 选项
type CharacterOption func(*CharacterSplitter)

// WithChunkSize 设置分块大小
func WithChunkSize(size int) CharacterOption {
	return func(s *CharacterSplitter) {
		s.chunkSize = size
	}
}

// WithChunkOverlap 设置分块重叠
func WithChunkOverlap(overlap int) CharacterOption {
	return func(s *CharacterSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithSeparator 设置分隔符
func WithSeparator(sep string) CharacterOption {
	return func(s *CharacterSplitter) {
		s.separator = sep
	}
}

// NewCharacterSplitter 创建字符分割器
func NewCharacterSplitter(opts ...CharacterOption) *CharacterSplitter {
	s := &CharacterSplitter{
		chunkSize:    1000,
		chunkOverlap: 200,
		separator:    "\n\n",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 分割文档
func (s *CharacterSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks := s.splitText(doc.Content)
		for i, chunk := range chunks {
			newDoc := rag.Document{
				ID:      util.GenerateID("chunk"),
				Content: chunk,
				Source:  doc.Source,
				Metadata: copyMetadata(doc.Metadata, map[string]any{
					"chunk_index": i,
					"chunk_total": len(chunks),
					"parent_id":   doc.ID,
					"splitter":    "character",
				}),
				CreatedAt: time.Now(),
			}
			result = append(result, newDoc)
		}
	}

	return result, nil
}

func (s *CharacterSplitter) splitText(text string) []string {
	// 按分隔符分割
	parts := strings.Split(text, s.separator)

	var chunks []string
	var currentChunk strings.Builder

	for _, part := range parts {
		partLen := utf8.RuneCountInString(part)
		currentLen := utf8.RuneCountInString(currentChunk.String())

		if currentLen+partLen > s.chunkSize && currentLen > 0 {
			// 保存当前块
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// 处理重叠
			if s.chunkOverlap > 0 {
				overlap := getOverlap(currentChunk.String(), s.chunkOverlap)
				currentChunk.Reset()
				currentChunk.WriteString(overlap)
			} else {
				currentChunk.Reset()
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(s.separator)
		}
		currentChunk.WriteString(part)
	}

	// 添加最后一块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// Name 返回分割器名称
func (s *CharacterSplitter) Name() string {
	return "CharacterSplitter"
}

var _ rag.Splitter = (*CharacterSplitter)(nil)

// ============== RecursiveSplitter ==============

// RecursiveSplitter 递归分割器
// 按照分隔符优先级递归分割，尽量保持语义完整
type RecursiveSplitter struct {
	chunkSize    int
	chunkOverlap int
	separators   []string
}

// RecursiveOption RecursiveSplitter 选项
type RecursiveOption func(*RecursiveSplitter)

// WithRecursiveChunkSize 设置分块大小
func WithRecursiveChunkSize(size int) RecursiveOption {
	return func(s *RecursiveSplitter) {
		s.chunkSize = size
	}
}

// WithRecursiveChunkOverlap 设置分块重叠
func WithRecursiveChunkOverlap(overlap int) RecursiveOption {
	return func(s *RecursiveSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithSeparators 设置分隔符列表（按优先级）
func WithSeparators(seps []string) RecursiveOption {
	return func(s *RecursiveSplitter) {
		s.separators = seps
	}
}

// NewRecursiveSplitter 创建递归分割器
func NewRecursiveSplitter(opts ...RecursiveOption) *RecursiveSplitter {
	s := &RecursiveSplitter{
		chunkSize:    1000,
		chunkOverlap: 200,
		separators:   []string{"\n\n", "\n", "。", ".", " ", ""},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 分割文档
func (s *RecursiveSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks := s.splitTextRecursive(doc.Content, s.separators)
		for i, chunk := range chunks {
			newDoc := rag.Document{
				ID:      util.GenerateID("chunk"),
				Content: chunk,
				Source:  doc.Source,
				Metadata: copyMetadata(doc.Metadata, map[string]any{
					"chunk_index": i,
					"chunk_total": len(chunks),
					"parent_id":   doc.ID,
					"splitter":    "recursive",
				}),
				CreatedAt: time.Now(),
			}
			result = append(result, newDoc)
		}
	}

	return result, nil
}

func (s *RecursiveSplitter) splitTextRecursive(text string, separators []string) []string {
	if len(separators) == 0 {
		// 最后一级：按字符硬分割
		return s.splitBySize(text)
	}

	separator := separators[0]
	remainingSeparators := separators[1:]

	var chunks []string

	if separator == "" {
		// 按字符分割
		return s.splitBySize(text)
	}

	parts := strings.Split(text, separator)
	var currentChunk strings.Builder

	for _, part := range parts {
		partLen := utf8.RuneCountInString(part)
		currentLen := utf8.RuneCountInString(currentChunk.String())

		// 如果单个 part 就超过了 chunkSize，需要继续递归分割
		if partLen > s.chunkSize {
			// 先保存当前块
			if currentLen > 0 {
				chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
				currentChunk.Reset()
			}
			// 递归分割这个大块
			subChunks := s.splitTextRecursive(part, remainingSeparators)
			chunks = append(chunks, subChunks...)
			continue
		}

		// 正常累积
		if currentLen+partLen+len(separator) > s.chunkSize && currentLen > 0 {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// 处理重叠
			if s.chunkOverlap > 0 {
				overlap := getOverlap(currentChunk.String(), s.chunkOverlap)
				currentChunk.Reset()
				currentChunk.WriteString(overlap)
			} else {
				currentChunk.Reset()
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(separator)
		}
		currentChunk.WriteString(part)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

func (s *RecursiveSplitter) splitBySize(text string) []string {
	runes := []rune(text)
	var chunks []string

	for i := 0; i < len(runes); i += s.chunkSize - s.chunkOverlap {
		end := i + s.chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
	}

	return chunks
}

// Name 返回分割器名称
func (s *RecursiveSplitter) Name() string {
	return "RecursiveSplitter"
}

var _ rag.Splitter = (*RecursiveSplitter)(nil)

// ============== MarkdownSplitter ==============

// MarkdownSplitter Markdown 专用分割器
// 按标题层级分割，保持文档结构
type MarkdownSplitter struct {
	chunkSize       int
	chunkOverlap    int
	headersToSplit  []string // 要分割的标题级别，如 ["#", "##", "###"]
	returnEachLine  bool     // 是否将每行作为单独的块
	stripHeaders    bool     // 是否移除标题
	codeBlockAware  bool     // 是否识别代码块
}

// MarkdownSplitterOption MarkdownSplitter 选项
type MarkdownSplitterOption func(*MarkdownSplitter)

// WithMarkdownChunkSize 设置分块大小
func WithMarkdownChunkSize(size int) MarkdownSplitterOption {
	return func(s *MarkdownSplitter) {
		s.chunkSize = size
	}
}

// WithMarkdownChunkOverlap 设置分块重叠
func WithMarkdownChunkOverlap(overlap int) MarkdownSplitterOption {
	return func(s *MarkdownSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithHeadersToSplit 设置要分割的标题级别
func WithHeadersToSplit(headers []string) MarkdownSplitterOption {
	return func(s *MarkdownSplitter) {
		s.headersToSplit = headers
	}
}

// WithCodeBlockAware 设置是否识别代码块
func WithCodeBlockAware(aware bool) MarkdownSplitterOption {
	return func(s *MarkdownSplitter) {
		s.codeBlockAware = aware
	}
}

// NewMarkdownSplitter 创建 Markdown 分割器
func NewMarkdownSplitter(opts ...MarkdownSplitterOption) *MarkdownSplitter {
	s := &MarkdownSplitter{
		chunkSize:      1000,
		chunkOverlap:   200,
		headersToSplit: []string{"#", "##"},
		codeBlockAware: true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 分割 Markdown 文档
func (s *MarkdownSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks := s.splitMarkdown(doc.Content)
		for i, chunk := range chunks {
			newDoc := rag.Document{
				ID:      util.GenerateID("chunk"),
				Content: chunk.content,
				Source:  doc.Source,
				Metadata: copyMetadata(doc.Metadata, map[string]any{
					"chunk_index": i,
					"chunk_total": len(chunks),
					"parent_id":   doc.ID,
					"splitter":    "markdown",
					"header":      chunk.header,
					"header_path": chunk.headerPath,
				}),
				CreatedAt: time.Now(),
			}
			result = append(result, newDoc)
		}
	}

	return result, nil
}

type mdChunk struct {
	content    string
	header     string
	headerPath string
}

func (s *MarkdownSplitter) splitMarkdown(text string) []mdChunk {
	lines := strings.Split(text, "\n")
	var chunks []mdChunk
	var currentContent strings.Builder
	var headerStack []string
	inCodeBlock := false

	for _, line := range lines {
		// 检测代码块
		if s.codeBlockAware && strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
			continue
		}

		// 在代码块内，不处理标题
		if inCodeBlock {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
			continue
		}

		// 检测标题
		headerLevel := s.detectHeader(line)
		if headerLevel != "" {
			// 保存当前块
			if currentContent.Len() > 0 {
				chunks = append(chunks, mdChunk{
					content:    strings.TrimSpace(currentContent.String()),
					header:     getLastHeader(headerStack),
					headerPath: strings.Join(headerStack, " > "),
				})
				currentContent.Reset()
			}

			// 更新标题栈
			headerStack = s.updateHeaderStack(headerStack, headerLevel, line)
		}

		currentContent.WriteString(line)
		currentContent.WriteString("\n")
	}

	// 添加最后一块
	if currentContent.Len() > 0 {
		chunks = append(chunks, mdChunk{
			content:    strings.TrimSpace(currentContent.String()),
			header:     getLastHeader(headerStack),
			headerPath: strings.Join(headerStack, " > "),
		})
	}

	// 如果块太大，进一步分割
	var finalChunks []mdChunk
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk.content) > s.chunkSize {
			subSplitter := NewRecursiveSplitter(
				WithRecursiveChunkSize(s.chunkSize),
				WithRecursiveChunkOverlap(s.chunkOverlap),
			)
			subTexts := subSplitter.splitTextRecursive(chunk.content, []string{"\n\n", "\n", "。", ".", " "})
			for _, subText := range subTexts {
				finalChunks = append(finalChunks, mdChunk{
					content:    subText,
					header:     chunk.header,
					headerPath: chunk.headerPath,
				})
			}
		} else {
			finalChunks = append(finalChunks, chunk)
		}
	}

	return finalChunks
}

func (s *MarkdownSplitter) detectHeader(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, h := range s.headersToSplit {
		if strings.HasPrefix(trimmed, h+" ") {
			return h
		}
	}
	return ""
}

func (s *MarkdownSplitter) updateHeaderStack(stack []string, level string, line string) []string {
	headerDepth := len(level) // # = 1, ## = 2, etc.

	// 截断到当前级别
	if headerDepth <= len(stack) {
		stack = stack[:headerDepth-1]
	}

	// 添加当前标题
	title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), level))
	return append(stack, title)
}

// Name 返回分割器名称
func (s *MarkdownSplitter) Name() string {
	return "MarkdownSplitter"
}

var _ rag.Splitter = (*MarkdownSplitter)(nil)

// ============== SentenceSplitter ==============

// SentenceSplitter 按句子分割
type SentenceSplitter struct {
	chunkSize     int
	chunkOverlap  int
	sentenceEnds  []string
}

// SentenceOption SentenceSplitter 选项
type SentenceOption func(*SentenceSplitter)

// WithSentenceChunkSize 设置分块大小
func WithSentenceChunkSize(size int) SentenceOption {
	return func(s *SentenceSplitter) {
		s.chunkSize = size
	}
}

// WithSentenceChunkOverlap 设置分块重叠
func WithSentenceChunkOverlap(overlap int) SentenceOption {
	return func(s *SentenceSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithSentenceEnds 设置句子结束符
func WithSentenceEnds(ends []string) SentenceOption {
	return func(s *SentenceSplitter) {
		s.sentenceEnds = ends
	}
}

// NewSentenceSplitter 创建句子分割器
func NewSentenceSplitter(opts ...SentenceOption) *SentenceSplitter {
	s := &SentenceSplitter{
		chunkSize:    1000,
		chunkOverlap: 200,
		sentenceEnds: []string{"。", "！", "？", ".", "!", "?", "\n"},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 按句子分割文档
func (s *SentenceSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks := s.splitBySentence(doc.Content)
		for i, chunk := range chunks {
			newDoc := rag.Document{
				ID:      util.GenerateID("chunk"),
				Content: chunk,
				Source:  doc.Source,
				Metadata: copyMetadata(doc.Metadata, map[string]any{
					"chunk_index": i,
					"chunk_total": len(chunks),
					"parent_id":   doc.ID,
					"splitter":    "sentence",
				}),
				CreatedAt: time.Now(),
			}
			result = append(result, newDoc)
		}
	}

	return result, nil
}

func (s *SentenceSplitter) splitBySentence(text string) []string {
	// 构建正则表达式
	pattern := strings.Join(s.sentenceEnds, "|")
	re := regexp.MustCompile("([" + regexp.QuoteMeta(pattern) + "])")

	// 分割句子（保留分隔符）
	parts := re.Split(text, -1)
	delimiters := re.FindAllString(text, -1)

	var sentences []string
	for i, part := range parts {
		sentence := part
		if i < len(delimiters) {
			sentence += delimiters[i]
		}
		sentence = strings.TrimSpace(sentence)
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
	}

	// 合并句子成块
	var chunks []string
	var currentChunk strings.Builder

	for _, sentence := range sentences {
		sentenceLen := utf8.RuneCountInString(sentence)
		currentLen := utf8.RuneCountInString(currentChunk.String())

		if currentLen+sentenceLen > s.chunkSize && currentLen > 0 {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
			currentChunk.Reset()
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// Name 返回分割器名称
func (s *SentenceSplitter) Name() string {
	return "SentenceSplitter"
}

var _ rag.Splitter = (*SentenceSplitter)(nil)

// ============== 辅助函数 ==============

// copyMetadata 复制并合并元数据
func copyMetadata(src map[string]any, extra map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range src {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

// getOverlap 获取重叠部分
func getOverlap(text string, overlap int) string {
	runes := []rune(text)
	if len(runes) <= overlap {
		return text
	}
	return string(runes[len(runes)-overlap:])
}

// getLastHeader 获取最后一个标题
func getLastHeader(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}
