// Package loader 提供 RAG 系统的文档加载器
//
// 本文件实现 PDF 文档加载器，支持：
//   - 纯文本 PDF 提取
//   - 基于页面的加载
//   - 元数据提取（标题、作者、创建时间等）

package loader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// PDFLoader PDF 文档加载器
//
// 支持从 PDF 文件中提取文本内容。
// 注意：此实现为基础版本，使用简单的文本提取。
// 对于复杂的 PDF（包含图片、表格等），建议使用专业的 PDF 库。
type PDFLoader struct {
	// path 文件路径
	path string

	// reader 可选的 io.Reader
	reader io.Reader

	// splitPages 是否按页面分割为多个文档
	splitPages bool

	// startPage 开始页码（1-indexed）
	startPage int

	// endPage 结束页码（0 表示到末尾）
	endPage int

	// extractMetadata 是否提取元数据
	extractMetadata bool

	// password PDF 密码（如果需要）
	password string

	// pdfParser PDF 解析器接口（用于依赖注入）
	pdfParser PDFParser
}

// PDFParser PDF 解析器接口
//
// 定义 PDF 解析的抽象接口，允许使用不同的 PDF 库实现。
// 默认提供基础的文本提取实现。
type PDFParser interface {
	// Parse 解析 PDF 并返回页面内容
	Parse(ctx context.Context, r io.Reader) (*PDFDocument, error)
}

// PDFDocument 解析后的 PDF 文档
type PDFDocument struct {
	// Pages 页面内容列表
	Pages []string

	// Metadata PDF 元数据
	Metadata PDFMetadata
}

// PDFMetadata PDF 元数据
type PDFMetadata struct {
	// Title 标题
	Title string `json:"title,omitempty"`

	// Author 作者
	Author string `json:"author,omitempty"`

	// Subject 主题
	Subject string `json:"subject,omitempty"`

	// Creator 创建者
	Creator string `json:"creator,omitempty"`

	// Producer 生产者
	Producer string `json:"producer,omitempty"`

	// CreationDate 创建日期
	CreationDate time.Time `json:"creation_date,omitempty"`

	// ModDate 修改日期
	ModDate time.Time `json:"mod_date,omitempty"`

	// PageCount 页数
	PageCount int `json:"page_count"`
}

// PDFOption PDF 加载器选项
type PDFOption func(*PDFLoader)

// WithPDFSplitPages 按页面分割
func WithPDFSplitPages(split bool) PDFOption {
	return func(l *PDFLoader) {
		l.splitPages = split
	}
}

// WithPDFPageRange 设置页面范围
func WithPDFPageRange(start, end int) PDFOption {
	return func(l *PDFLoader) {
		l.startPage = start
		l.endPage = end
	}
}

// WithPDFPassword 设置密码
func WithPDFPassword(password string) PDFOption {
	return func(l *PDFLoader) {
		l.password = password
	}
}

// WithPDFExtractMetadata 设置是否提取元数据
func WithPDFExtractMetadata(extract bool) PDFOption {
	return func(l *PDFLoader) {
		l.extractMetadata = extract
	}
}

// WithPDFParser 设置自定义 PDF 解析器
func WithPDFParser(parser PDFParser) PDFOption {
	return func(l *PDFLoader) {
		l.pdfParser = parser
	}
}

// NewPDFLoader 创建 PDF 加载器
func NewPDFLoader(path string, opts ...PDFOption) *PDFLoader {
	l := &PDFLoader{
		path:            path,
		splitPages:      false,
		startPage:       1,
		endPage:         0,
		extractMetadata: true,
		pdfParser:       &SimplePDFParser{},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewPDFLoaderFromReader 从 Reader 创建 PDF 加载器
func NewPDFLoaderFromReader(r io.Reader, opts ...PDFOption) *PDFLoader {
	l := &PDFLoader{
		reader:          r,
		splitPages:      false,
		startPage:       1,
		endPage:         0,
		extractMetadata: true,
		pdfParser:       &SimplePDFParser{},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 PDF 文档
func (l *PDFLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var reader io.Reader

	if l.reader != nil {
		reader = l.reader
	} else {
		file, err := os.Open(l.path)
		if err != nil {
			return nil, fmt.Errorf("failed to open PDF file %s: %w", l.path, err)
		}
		defer file.Close()
		reader = file
	}

	// 解析 PDF
	pdfDoc, err := l.pdfParser.Parse(ctx, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// 确定页面范围
	startIdx := l.startPage - 1
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := len(pdfDoc.Pages)
	if l.endPage > 0 && l.endPage < endIdx {
		endIdx = l.endPage
	}
	if startIdx >= endIdx {
		return nil, fmt.Errorf("invalid page range: %d-%d", l.startPage, l.endPage)
	}

	// 构建文档
	var docs []rag.Document
	source := l.path
	if source == "" {
		source = "reader"
	}

	baseMetadata := map[string]any{
		"loader":     "pdf",
		"file_path":  l.path,
		"file_name":  filepath.Base(l.path),
		"page_count": pdfDoc.Metadata.PageCount,
	}

	// 添加 PDF 元数据
	if l.extractMetadata {
		if pdfDoc.Metadata.Title != "" {
			baseMetadata["title"] = pdfDoc.Metadata.Title
		}
		if pdfDoc.Metadata.Author != "" {
			baseMetadata["author"] = pdfDoc.Metadata.Author
		}
		if pdfDoc.Metadata.Subject != "" {
			baseMetadata["subject"] = pdfDoc.Metadata.Subject
		}
		if !pdfDoc.Metadata.CreationDate.IsZero() {
			baseMetadata["creation_date"] = pdfDoc.Metadata.CreationDate
		}
	}

	if l.splitPages {
		// 每页一个文档
		for i := startIdx; i < endIdx; i++ {
			pageNum := i + 1
			content := pdfDoc.Pages[i]

			metadata := make(map[string]any)
			for k, v := range baseMetadata {
				metadata[k] = v
			}
			metadata["page_number"] = pageNum

			doc := rag.Document{
				ID:        util.GenerateID("doc"),
				Content:   content,
				Source:    fmt.Sprintf("%s#page=%d", source, pageNum),
				Metadata:  metadata,
				CreatedAt: time.Now(),
			}
			docs = append(docs, doc)
		}
	} else {
		// 合并所有页面为一个文档
		var contentParts []string
		for i := startIdx; i < endIdx; i++ {
			contentParts = append(contentParts, pdfDoc.Pages[i])
		}

		doc := rag.Document{
			ID:        util.GenerateID("doc"),
			Content:   strings.Join(contentParts, "\n\n---\n\n"),
			Source:    source,
			Metadata:  baseMetadata,
			CreatedAt: time.Now(),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *PDFLoader) Name() string {
	return "PDFLoader"
}

var _ rag.Loader = (*PDFLoader)(nil)

// ============== SimplePDFParser ==============

// SimplePDFParser 简单的 PDF 解析器
//
// 这是一个基础实现，从 PDF 二进制流中提取文本。
// 对于复杂的 PDF 文档，建议使用专业的 PDF 库（如 pdfcpu、unidoc 等）。
type SimplePDFParser struct{}

// Parse 解析 PDF
//
// 注意：这是一个简化的实现，主要用于演示。
// 实际使用中建议集成专业的 PDF 解析库。
func (p *SimplePDFParser) Parse(ctx context.Context, r io.Reader) (*PDFDocument, error) {
	// 读取所有内容
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF data: %w", err)
	}

	// 检查 PDF 签名
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return nil, fmt.Errorf("not a valid PDF file")
	}

	// 简单的文本提取：查找流对象中的文本
	// 注意：这是非常简化的实现，不能处理所有 PDF 格式
	pages := p.extractTextFromPDF(data)

	if len(pages) == 0 {
		// 如果没有提取到文本，返回一个空页面
		pages = []string{"[PDF content extraction requires a full PDF parser]"}
	}

	return &PDFDocument{
		Pages: pages,
		Metadata: PDFMetadata{
			PageCount: len(pages),
		},
	}, nil
}

// extractTextFromPDF 从 PDF 数据中提取文本
//
// 这是一个非常简化的实现，只能处理简单的文本 PDF。
// 实际应用中应使用专业的 PDF 库。
func (p *SimplePDFParser) extractTextFromPDF(data []byte) []string {
	var pages []string
	var currentPage strings.Builder

	// 查找文本流
	// PDF 文本通常在 BT...ET 块中
	// 格式可能是: (text) Tj 或 [(text)] TJ

	dataStr := string(data)

	// 简单的文本提取：查找括号中的文本
	inText := false
	escape := false

	for i := 0; i < len(dataStr); i++ {
		c := dataStr[i]

		if escape {
			if inText {
				switch c {
				case 'n':
					currentPage.WriteByte('\n')
				case 'r':
					currentPage.WriteByte('\r')
				case 't':
					currentPage.WriteByte('\t')
				default:
					currentPage.WriteByte(c)
				}
			}
			escape = false
			continue
		}

		if c == '\\' {
			escape = true
			continue
		}

		if c == '(' && !inText {
			inText = true
			continue
		}

		if c == ')' && inText {
			inText = false
			currentPage.WriteByte(' ')
			continue
		}

		if inText {
			currentPage.WriteByte(c)
		}

		// 检测页面分隔
		if i > 5 && dataStr[i-5:i] == "Page " && !inText {
			if currentPage.Len() > 0 {
				pages = append(pages, strings.TrimSpace(currentPage.String()))
				currentPage.Reset()
			}
		}
	}

	// 添加最后一页
	if currentPage.Len() > 0 {
		pages = append(pages, strings.TrimSpace(currentPage.String()))
	}

	// 如果没有分页，将所有内容作为一页
	if len(pages) == 0 && currentPage.Len() > 0 {
		pages = []string{strings.TrimSpace(currentPage.String())}
	}

	return pages
}
