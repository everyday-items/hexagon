// Package loader 提供 RAG 系统的文档加载器
//
// 本文件实现 Word 文档 (DOCX) 加载器，支持：
//   - 文本内容提取
//   - 段落结构保留
//   - 文档元数据提取（标题、作者等）

package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// DOCXLoader Word 文档加载器
//
// 支持从 DOCX 文件中提取文本内容。
// DOCX 是基于 Open XML 格式的文档，本质是一个 ZIP 文件。
type DOCXLoader struct {
	// path 文件路径
	path string

	// reader 可选的 io.ReaderAt
	reader io.ReaderAt
	size   int64

	// preserveParagraphs 是否保留段落结构
	preserveParagraphs bool

	// extractMetadata 是否提取元数据
	extractMetadata bool

	// includeStyles 是否包含样式信息
	includeStyles bool

	// splitByHeading 是否按标题分割
	splitByHeading bool
}

// DOCXOption DOCX 加载器选项
type DOCXOption func(*DOCXLoader)

// WithDOCXPreserveParagraphs 保留段落结构
func WithDOCXPreserveParagraphs(preserve bool) DOCXOption {
	return func(l *DOCXLoader) {
		l.preserveParagraphs = preserve
	}
}

// WithDOCXExtractMetadata 提取元数据
func WithDOCXExtractMetadata(extract bool) DOCXOption {
	return func(l *DOCXLoader) {
		l.extractMetadata = extract
	}
}

// WithDOCXSplitByHeading 按标题分割
func WithDOCXSplitByHeading(split bool) DOCXOption {
	return func(l *DOCXLoader) {
		l.splitByHeading = split
	}
}

// NewDOCXLoader 创建 DOCX 加载器
func NewDOCXLoader(path string, opts ...DOCXOption) *DOCXLoader {
	l := &DOCXLoader{
		path:               path,
		preserveParagraphs: true,
		extractMetadata:    true,
		includeStyles:      false,
		splitByHeading:     false,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewDOCXLoaderFromReader 从 ReaderAt 创建 DOCX 加载器
func NewDOCXLoaderFromReader(r io.ReaderAt, size int64, opts ...DOCXOption) *DOCXLoader {
	l := &DOCXLoader{
		reader:             r,
		size:               size,
		preserveParagraphs: true,
		extractMetadata:    true,
		includeStyles:      false,
		splitByHeading:     false,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 DOCX 文档
func (l *DOCXLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var zipReader *zip.Reader

	if l.reader != nil {
		var err error
		zipReader, err = zip.NewReader(l.reader, l.size)
		if err != nil {
			return nil, fmt.Errorf("failed to open DOCX from reader: %w", err)
		}
	} else {
		file, err := os.Open(l.path)
		if err != nil {
			return nil, fmt.Errorf("failed to open DOCX file %s: %w", l.path, err)
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to stat file: %w", err)
		}

		zipReader, err = zip.NewReader(file, stat.Size())
		if err != nil {
			return nil, fmt.Errorf("failed to open DOCX as ZIP: %w", err)
		}
	}

	// 解析文档
	docContent, err := l.parseDocument(zipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DOCX: %w", err)
	}

	// 提取元数据
	metadata := map[string]any{
		"loader":    "docx",
		"file_path": l.path,
		"file_name": filepath.Base(l.path),
	}

	if l.extractMetadata {
		docMeta, err := l.parseMetadata(zipReader)
		if err == nil && docMeta != nil {
			if docMeta.Title != "" {
				metadata["title"] = docMeta.Title
			}
			if docMeta.Creator != "" {
				metadata["author"] = docMeta.Creator
			}
			if docMeta.Subject != "" {
				metadata["subject"] = docMeta.Subject
			}
			if docMeta.Description != "" {
				metadata["description"] = docMeta.Description
			}
			if docMeta.Keywords != "" {
				metadata["keywords"] = docMeta.Keywords
			}
			if !docMeta.Created.IsZero() {
				metadata["created"] = docMeta.Created
			}
			if !docMeta.Modified.IsZero() {
				metadata["modified"] = docMeta.Modified
			}
		}
	}

	source := l.path
	if source == "" {
		source = "reader"
	}

	// 构建文档
	if l.splitByHeading && len(docContent.Sections) > 1 {
		// 按标题分割为多个文档
		var docs []rag.Document
		for i, section := range docContent.Sections {
			sectionMeta := make(map[string]any)
			for k, v := range metadata {
				sectionMeta[k] = v
			}
			sectionMeta["section_index"] = i
			if section.Heading != "" {
				sectionMeta["section_heading"] = section.Heading
			}

			doc := rag.Document{
				ID:        util.GenerateID("doc"),
				Content:   section.Content,
				Source:    fmt.Sprintf("%s#section=%d", source, i),
				Metadata:  sectionMeta,
				CreatedAt: time.Now(),
			}
			docs = append(docs, doc)
		}
		return docs, nil
	}

	// 返回单个文档
	content := docContent.FullText
	if l.preserveParagraphs {
		var paragraphs []string
		for _, section := range docContent.Sections {
			paragraphs = append(paragraphs, section.Content)
		}
		content = strings.Join(paragraphs, "\n\n")
	}

	doc := rag.Document{
		ID:        util.GenerateID("doc"),
		Content:   content,
		Source:    source,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *DOCXLoader) Name() string {
	return "DOCXLoader"
}

var _ rag.Loader = (*DOCXLoader)(nil)

// ============== DOCX 解析 ==============

// docxContent 解析后的文档内容
type docxContent struct {
	// FullText 完整文本
	FullText string

	// Sections 按标题分割的章节
	Sections []docxSection
}

// docxSection 文档章节
type docxSection struct {
	// Heading 标题
	Heading string

	// Content 内容
	Content string

	// Level 标题级别（1-9，0 表示正文）
	Level int
}

// docxMetadata 文档元数据
type docxMetadata struct {
	Title       string
	Creator     string
	Subject     string
	Description string
	Keywords    string
	Created     time.Time
	Modified    time.Time
}

// parseDocument 解析文档内容
func (l *DOCXLoader) parseDocument(zipReader *zip.Reader) (*docxContent, error) {
	// 查找主文档文件
	var documentFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "word/document.xml" {
			documentFile = f
			break
		}
	}

	if documentFile == nil {
		return nil, fmt.Errorf("document.xml not found in DOCX")
	}

	// 读取文档内容
	rc, err := documentFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open document.xml: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read document.xml: %w", err)
	}

	// 解析 XML
	return l.parseDocumentXML(data)
}

// parseDocumentXML 解析文档 XML
func (l *DOCXLoader) parseDocumentXML(data []byte) (*docxContent, error) {
	// Word XML 命名空间
	type textRun struct {
		Text string `xml:",chardata"`
	}

	type paragraph struct {
		Runs []struct {
			Text []textRun `xml:"t"`
		} `xml:"r"`
		// 段落属性（用于检测标题）
		Properties struct {
			Style struct {
				Val string `xml:"val,attr"`
			} `xml:"pStyle"`
		} `xml:"pPr"`
	}

	type body struct {
		Paragraphs []paragraph `xml:"p"`
	}

	type document struct {
		Body body `xml:"body"`
	}

	var doc document
	if err := xml.Unmarshal(data, &doc); err != nil {
		// 尝试简单的文本提取
		return l.extractTextSimple(data), nil
	}

	content := &docxContent{
		Sections: make([]docxSection, 0),
	}

	var fullText strings.Builder
	var currentSection docxSection

	for _, para := range doc.Body.Paragraphs {
		var paraText strings.Builder

		for _, run := range para.Runs {
			for _, t := range run.Text {
				paraText.WriteString(t.Text)
			}
		}

		text := paraText.String()
		if text == "" {
			continue
		}

		// 检测是否是标题
		style := para.Properties.Style.Val
		isHeading := strings.HasPrefix(strings.ToLower(style), "heading") ||
			strings.HasPrefix(strings.ToLower(style), "title")

		if isHeading && l.splitByHeading {
			// 保存当前章节
			if currentSection.Content != "" {
				content.Sections = append(content.Sections, currentSection)
			}

			// 开始新章节
			level := 1
			if len(style) > 7 {
				// 尝试提取标题级别
				levelStr := style[len(style)-1:]
				if levelStr >= "1" && levelStr <= "9" {
					level = int(levelStr[0] - '0')
				}
			}

			currentSection = docxSection{
				Heading: text,
				Level:   level,
			}
		} else {
			// 添加到当前章节
			if currentSection.Content != "" {
				currentSection.Content += "\n"
			}
			currentSection.Content += text
		}

		// 添加到完整文本
		if fullText.Len() > 0 {
			fullText.WriteString("\n")
		}
		fullText.WriteString(text)
	}

	// 保存最后一个章节
	if currentSection.Content != "" {
		content.Sections = append(content.Sections, currentSection)
	}

	// 如果没有章节，创建一个默认章节
	if len(content.Sections) == 0 {
		content.Sections = []docxSection{{Content: fullText.String()}}
	}

	content.FullText = fullText.String()
	return content, nil
}

// extractTextSimple 简单文本提取（XML 解析失败时的降级方案）
func (l *DOCXLoader) extractTextSimple(data []byte) *docxContent {
	var text strings.Builder

	// 查找 <w:t> 标签中的文本
	dataStr := string(data)
	for {
		start := strings.Index(dataStr, "<w:t")
		if start == -1 {
			break
		}

		// 找到 > 结束
		gtPos := strings.Index(dataStr[start:], ">")
		if gtPos == -1 {
			break
		}
		start += gtPos + 1

		// 找到结束标签
		end := strings.Index(dataStr[start:], "</w:t>")
		if end == -1 {
			break
		}

		// 提取文本
		text.WriteString(dataStr[start : start+end])
		dataStr = dataStr[start+end+6:]
	}

	content := text.String()
	return &docxContent{
		FullText: content,
		Sections: []docxSection{{Content: content}},
	}
}

// parseMetadata 解析文档元数据
func (l *DOCXLoader) parseMetadata(zipReader *zip.Reader) (*docxMetadata, error) {
	// 查找核心属性文件
	var corePropsFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "docProps/core.xml" {
			corePropsFile = f
			break
		}
	}

	if corePropsFile == nil {
		return nil, nil // 没有元数据文件
	}

	rc, err := corePropsFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	// 解析元数据 XML
	type coreProperties struct {
		Title       string `xml:"title"`
		Creator     string `xml:"creator"`
		Subject     string `xml:"subject"`
		Description string `xml:"description"`
		Keywords    string `xml:"keywords"`
		Created     string `xml:"created"`
		Modified    string `xml:"modified"`
	}

	var props coreProperties
	if err := xml.Unmarshal(data, &props); err != nil {
		return nil, err
	}

	meta := &docxMetadata{
		Title:       props.Title,
		Creator:     props.Creator,
		Subject:     props.Subject,
		Description: props.Description,
		Keywords:    props.Keywords,
	}

	// 解析日期
	if props.Created != "" {
		if t, err := time.Parse(time.RFC3339, props.Created); err == nil {
			meta.Created = t
		}
	}
	if props.Modified != "" {
		if t, err := time.Parse(time.RFC3339, props.Modified); err == nil {
			meta.Modified = t
		}
	}

	return meta, nil
}

// ============== HTML Loader ==============

// HTMLLoader HTML 文档加载器
type HTMLLoader struct {
	// path 文件路径
	path string

	// reader io.Reader
	reader io.Reader

	// url URL（用于设置 Source）
	url string

	// removeScripts 移除脚本
	removeScripts bool

	// removeStyles 移除样式
	removeStyles bool

	// extractTitle 提取标题
	extractTitle bool
}

// HTMLOption HTML 加载器选项
type HTMLOption func(*HTMLLoader)

// WithHTMLRemoveScripts 移除脚本
func WithHTMLRemoveScripts(remove bool) HTMLOption {
	return func(l *HTMLLoader) {
		l.removeScripts = remove
	}
}

// WithHTMLRemoveStyles 移除样式
func WithHTMLRemoveStyles(remove bool) HTMLOption {
	return func(l *HTMLLoader) {
		l.removeStyles = remove
	}
}

// WithHTMLExtractTitle 提取标题
func WithHTMLExtractTitle(extract bool) HTMLOption {
	return func(l *HTMLLoader) {
		l.extractTitle = extract
	}
}

// NewHTMLLoader 创建 HTML 加载器
func NewHTMLLoader(path string, opts ...HTMLOption) *HTMLLoader {
	l := &HTMLLoader{
		path:          path,
		removeScripts: true,
		removeStyles:  true,
		extractTitle:  true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewHTMLLoaderFromReader 从 Reader 创建 HTML 加载器
func NewHTMLLoaderFromReader(r io.Reader, url string, opts ...HTMLOption) *HTMLLoader {
	l := &HTMLLoader{
		reader:        r,
		url:           url,
		removeScripts: true,
		removeStyles:  true,
		extractTitle:  true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 HTML 文档
func (l *HTMLLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var content []byte
	var err error

	if l.reader != nil {
		content, err = io.ReadAll(l.reader)
	} else {
		content, err = os.ReadFile(l.path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read HTML: %w", err)
	}

	// 解析 HTML
	text, title := l.parseHTML(string(content))

	source := l.path
	if l.url != "" {
		source = l.url
	}

	metadata := map[string]any{
		"loader":    "html",
		"file_path": l.path,
		"file_name": filepath.Base(l.path),
	}

	if title != "" {
		metadata["title"] = title
	}
	if l.url != "" {
		metadata["url"] = l.url
	}

	doc := rag.Document{
		ID:        util.GenerateID("doc"),
		Content:   text,
		Source:    source,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *HTMLLoader) Name() string {
	return "HTMLLoader"
}

var _ rag.Loader = (*HTMLLoader)(nil)

// parseHTML 解析 HTML 内容
func (l *HTMLLoader) parseHTML(html string) (text, title string) {
	// 移除脚本
	if l.removeScripts {
		html = removeHTMLTag(html, "script")
	}

	// 移除样式
	if l.removeStyles {
		html = removeHTMLTag(html, "style")
	}

	// 提取标题
	if l.extractTitle {
		title = extractHTMLTag(html, "title")
	}

	// 提取正文
	body := extractHTMLTag(html, "body")
	if body == "" {
		body = html
	}

	// 移除所有 HTML 标签
	text = stripHTMLTags(body)

	// 清理空白
	text = cleanWhitespace(text)

	return text, title
}

// removeHTMLTag 移除指定的 HTML 标签及其内容
func removeHTMLTag(html, tag string) string {
	result := html
	for {
		start := strings.Index(strings.ToLower(result), "<"+tag)
		if start == -1 {
			break
		}

		end := strings.Index(strings.ToLower(result[start:]), "</"+tag+">")
		if end == -1 {
			// 自闭合标签
			end = strings.Index(result[start:], ">")
			if end == -1 {
				break
			}
			result = result[:start] + result[start+end+1:]
		} else {
			result = result[:start] + result[start+end+len(tag)+3:]
		}
	}
	return result
}

// extractHTMLTag 提取指定 HTML 标签的内容
func extractHTMLTag(html, tag string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<"+tag)
	if start == -1 {
		return ""
	}

	// 找到开始标签的结束位置
	gtPos := strings.Index(html[start:], ">")
	if gtPos == -1 {
		return ""
	}
	start += gtPos + 1

	end := strings.Index(lower[start:], "</"+tag+">")
	if end == -1 {
		return ""
	}

	return html[start : start+end]
}

// stripHTMLTags 移除所有 HTML 标签
func stripHTMLTags(html string) string {
	var result strings.Builder
	inTag := false

	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteRune(' ') // 标签替换为空格
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// cleanWhitespace 清理多余空白
func cleanWhitespace(text string) string {
	// 替换多个空白为单个空格
	var result strings.Builder
	lastWasSpace := false

	for _, r := range text {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace {
			if !lastWasSpace {
				result.WriteRune(' ')
			}
			lastWasSpace = true
		} else {
			result.WriteRune(r)
			lastWasSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}

// ============== CSV Loader ==============

// CSVLoader CSV 文件加载器
type CSVLoader struct {
	// path 文件路径
	path string

	// reader io.Reader
	reader io.Reader

	// separator 分隔符
	separator rune

	// hasHeader 是否有表头
	hasHeader bool

	// contentColumn 内容列名或索引
	contentColumn string

	// metadataColumns 元数据列名列表
	metadataColumns []string
}

// CSVOption CSV 加载器选项
type CSVOption func(*CSVLoader)

// WithCSVSeparator 设置分隔符
func WithCSVSeparator(sep rune) CSVOption {
	return func(l *CSVLoader) {
		l.separator = sep
	}
}

// WithCSVHeader 设置是否有表头
func WithCSVHeader(hasHeader bool) CSVOption {
	return func(l *CSVLoader) {
		l.hasHeader = hasHeader
	}
}

// WithCSVContentColumn 设置内容列
func WithCSVContentColumn(column string) CSVOption {
	return func(l *CSVLoader) {
		l.contentColumn = column
	}
}

// WithCSVMetadataColumns 设置元数据列
func WithCSVMetadataColumns(columns []string) CSVOption {
	return func(l *CSVLoader) {
		l.metadataColumns = columns
	}
}

// NewCSVLoader 创建 CSV 加载器
func NewCSVLoader(path string, opts ...CSVOption) *CSVLoader {
	l := &CSVLoader{
		path:      path,
		separator: ',',
		hasHeader: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 CSV 文件
func (l *CSVLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var content []byte
	var err error

	if l.reader != nil {
		content, err = io.ReadAll(l.reader)
	} else {
		content, err = os.ReadFile(l.path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	// 简单的 CSV 解析
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty CSV file")
	}

	// 解析表头
	var headers []string
	startRow := 0
	if l.hasHeader {
		headers = splitCSVLine(lines[0], l.separator)
		startRow = 1
	}

	// 确定内容列索引
	contentIdx := 0
	if l.contentColumn != "" {
		for i, h := range headers {
			if h == l.contentColumn {
				contentIdx = i
				break
			}
		}
	}

	// 解析数据行
	var docs []rag.Document
	for i := startRow; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := splitCSVLine(line, l.separator)
		if len(fields) == 0 {
			continue
		}

		// 获取内容
		var content string
		if contentIdx < len(fields) {
			content = fields[contentIdx]
		}

		// 构建元数据
		metadata := map[string]any{
			"loader":    "csv",
			"file_path": l.path,
			"row":       i,
		}

		if l.hasHeader {
			for j, h := range headers {
				if j < len(fields) {
					metadata[h] = fields[j]
				}
			}
		}

		doc := rag.Document{
			ID:        util.GenerateID("doc"),
			Content:   content,
			Source:    fmt.Sprintf("%s#row=%d", l.path, i),
			Metadata:  metadata,
			CreatedAt: time.Now(),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *CSVLoader) Name() string {
	return "CSVLoader"
}

var _ rag.Loader = (*CSVLoader)(nil)

// splitCSVLine 分割 CSV 行
func splitCSVLine(line string, sep rune) []string {
	var fields []string
	var current bytes.Buffer
	inQuotes := false

	for _, r := range line {
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if r == sep && !inQuotes {
			fields = append(fields, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}

	fields = append(fields, strings.TrimSpace(current.String()))
	return fields
}

// ============== JSON Loader ==============

// JSONLoader JSON 文件加载器
type JSONLoader struct {
	// path 文件路径
	path string

	// reader io.Reader
	reader io.Reader

	// contentKey JSON 内容键路径（如 "data.content"）
	contentKey string

	// metadataKeys 元数据键列表
	metadataKeys []string

	// jqFilter jq 风格的过滤表达式
	jqFilter string
}

// JSONOption JSON 加载器选项
type JSONOption func(*JSONLoader)

// WithJSONContentKey 设置内容键
func WithJSONContentKey(key string) JSONOption {
	return func(l *JSONLoader) {
		l.contentKey = key
	}
}

// WithJSONMetadataKeys 设置元数据键
func WithJSONMetadataKeys(keys []string) JSONOption {
	return func(l *JSONLoader) {
		l.metadataKeys = keys
	}
}

// NewJSONLoader 创建 JSON 加载器
func NewJSONLoader(path string, opts ...JSONOption) *JSONLoader {
	l := &JSONLoader{
		path:       path,
		contentKey: "content",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 JSON 文件
func (l *JSONLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var content []byte
	var err error

	if l.reader != nil {
		content, err = io.ReadAll(l.reader)
	} else {
		content, err = os.ReadFile(l.path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read JSON: %w", err)
	}

	// 将整个 JSON 作为内容
	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: string(content),
		Source:  l.path,
		Metadata: map[string]any{
			"loader":    "json",
			"file_path": l.path,
			"file_name": filepath.Base(l.path),
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *JSONLoader) Name() string {
	return "JSONLoader"
}

var _ rag.Loader = (*JSONLoader)(nil)
