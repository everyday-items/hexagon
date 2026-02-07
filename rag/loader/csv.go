// Package loader 提供 RAG 系统的文档加载器
//
// csv.go 实现 CSV 文件加载器和 Excel/PPTX 加载器：
//   - CSVLoader: 标准 CSV 格式解析，支持自定义分隔符、内容列指定
//   - ExcelLoader: Excel (.xlsx) 文件加载
//   - PPTXLoader: PowerPoint (.pptx) 文件加载
//
// 使用示例：
//
//	loader := NewCSVLoader("data.csv",
//	    WithCSVSeparator(';'),
//	    WithCSVContentColumn("description"),
//	)
//	docs, err := loader.Load(ctx)
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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== CSVLoader ==============

// CSVLoader CSV 文件加载器
// 将 CSV 文件中的每行数据转换为一个文档
type CSVLoader struct {
	// path 文件路径
	path string

	// separator 分隔符（默认逗号）
	separator rune

	// hasHeader 是否有表头
	hasHeader bool

	// contentColumn 内容列名（为空时使用第一列）
	contentColumn string

	// metadataColumns 元数据列名列表
	metadataColumns []string

	// rowsPerDoc 每个文档包含的行数（0 表示每行一个文档）
	rowsPerDoc int
}

// CSVOption CSV 加载器选项
type CSVOption func(*CSVLoader)

// WithCSVSeparator 设置 CSV 分隔符
func WithCSVSeparator(sep rune) CSVOption {
	return func(l *CSVLoader) {
		l.separator = sep
	}
}

// WithCSVDelimiter 设置 CSV 分隔符（WithCSVSeparator 的别名）
func WithCSVDelimiter(delim rune) CSVOption {
	return func(l *CSVLoader) {
		l.separator = delim
	}
}

// WithCSVHeader 设置是否有表头
func WithCSVHeader(hasHeader bool) CSVOption {
	return func(l *CSVLoader) {
		l.hasHeader = hasHeader
	}
}

// WithCSVNoHeader 表示 CSV 无表头行
func WithCSVNoHeader() CSVOption {
	return func(l *CSVLoader) {
		l.hasHeader = false
	}
}

// WithCSVContentColumn 设置内容列名
func WithCSVContentColumn(column string) CSVOption {
	return func(l *CSVLoader) {
		l.contentColumn = column
	}
}

// WithCSVContentColumns 设置多个内容列名
func WithCSVContentColumns(cols ...string) CSVOption {
	return func(l *CSVLoader) {
		if len(cols) > 0 {
			l.contentColumn = cols[0]
		}
	}
}

// WithCSVMetadataColumns 设置元数据列
func WithCSVMetadataColumns(columns []string) CSVOption {
	return func(l *CSVLoader) {
		l.metadataColumns = columns
	}
}

// WithCSVRowsPerDoc 设置每个文档包含的行数
func WithCSVRowsPerDoc(rows int) CSVOption {
	return func(l *CSVLoader) {
		l.rowsPerDoc = rows
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
	content, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("无法读取 CSV 文件 %s: %w", l.path, err)
	}

	// 分割行
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return nil, nil
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
	if l.contentColumn != "" && l.hasHeader {
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
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := splitCSVLine(line, l.separator)
		if len(fields) == 0 {
			continue
		}

		// 获取内容
		var docContent string
		if contentIdx < len(fields) {
			docContent = fields[contentIdx]
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
			Content:   docContent,
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
// 支持双引号包裹的字段（字段内可包含分隔符和换行）
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

// ============== ExcelLoader ==============

// ExcelLoader Excel 文件加载器（.xlsx 格式）
// 将 .xlsx 文件中的每行数据转换为一个文档
//
// 实现原理：.xlsx 文件本质是 ZIP 包，内含 XML 描述的工作表数据。
// 当前为基础实现，如需完整的 Excel 功能建议使用 excelize 等第三方库。
type ExcelLoader struct {
	path            string
	sheetName       string
	contentColumns  []string
	metadataColumns []string
	hasHeader       bool
}

// ExcelOption Excel 加载器配置选项
type ExcelOption func(*ExcelLoader)

// WithExcelSheet 设置工作表名称（空表示第一个工作表）
func WithExcelSheet(name string) ExcelOption {
	return func(l *ExcelLoader) {
		l.sheetName = name
	}
}

// WithExcelContentColumns 设置用作内容的列名
func WithExcelContentColumns(cols ...string) ExcelOption {
	return func(l *ExcelLoader) {
		l.contentColumns = cols
	}
}

// WithExcelMetadataColumns 设置用作元数据的列名
func WithExcelMetadataColumns(cols ...string) ExcelOption {
	return func(l *ExcelLoader) {
		l.metadataColumns = cols
	}
}

// NewExcelLoader 创建 Excel 加载器
func NewExcelLoader(path string, opts ...ExcelOption) *ExcelLoader {
	l := &ExcelLoader{
		path:      path,
		hasHeader: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 Excel 文件
// 将 xlsx 文件每行转换为一个 Document
func (l *ExcelLoader) Load(ctx context.Context) ([]rag.Document, error) {
	rows, err := readXLSXSimple(l.path)
	if err != nil {
		return nil, fmt.Errorf("读取 Excel 文件失败 %s: %w", l.path, err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// 处理表头
	var headers []string
	startRow := 0
	if l.hasHeader && len(rows) > 0 {
		headers = rows[0]
		startRow = 1
	} else {
		for i := range rows[0] {
			headers = append(headers, fmt.Sprintf("col_%d", i))
		}
	}

	var docs []rag.Document
	for i := startRow; i < len(rows); i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		row := rows[i]
		var parts []string
		for j, cell := range row {
			if j < len(headers) && strings.TrimSpace(cell) != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", headers[j], cell))
			}
		}

		if len(parts) == 0 {
			continue
		}

		doc := rag.Document{
			ID:      util.GenerateID("xlsx"),
			Content: strings.Join(parts, "\n"),
			Source:  l.path,
			Metadata: map[string]any{
				"loader":    "excel",
				"file_path": l.path,
				"file_name": filepath.Base(l.path),
				"row_index": i,
			},
			CreatedAt: time.Now(),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *ExcelLoader) Name() string {
	return "ExcelLoader"
}

var _ rag.Loader = (*ExcelLoader)(nil)

// ============== XLSX XML 结构体 ==============

// xlsxSharedStrings 共享字符串表
// xlsx 中相同的字符串只存储一次，单元格通过索引引用
type xlsxSharedStrings struct {
	XMLName xml.Name    `xml:"sst"`
	SI      []xlsxSI    `xml:"si"`
}

// xlsxSI 共享字符串条目
type xlsxSI struct {
	T string  `xml:"t"`
	R []xlsxR `xml:"r"` // 富文本格式：多段文本
}

// xlsxR 富文本段落
type xlsxR struct {
	T string `xml:"t"`
}

// xlsxWorksheet 工作表
type xlsxWorksheet struct {
	XMLName   xml.Name       `xml:"worksheet"`
	SheetData xlsxSheetData  `xml:"sheetData"`
}

// xlsxSheetData 工作表数据
type xlsxSheetData struct {
	Rows []xlsxRow `xml:"row"`
}

// xlsxRow 行
type xlsxRow struct {
	R     int        `xml:"r,attr"` // 行号（从1开始）
	Cells []xlsxCell `xml:"c"`
}

// xlsxCell 单元格
type xlsxCell struct {
	R string `xml:"r,attr"` // 单元格引用（如 "A1"）
	T string `xml:"t,attr"` // 类型：s=共享字符串, 空=数值/日期
	V string `xml:"v"`      // 值
}

// readXLSXSimple 基于 archive/zip + encoding/xml 的 xlsx 解析
// xlsx 本质是 ZIP 文件，内部包含：
//   - xl/sharedStrings.xml: 共享字符串表
//   - xl/worksheets/sheet1.xml: 第一个工作表数据
//
// 当前实现仅解析第一个工作表，支持共享字符串引用和空单元格填充。
// 如需完整 Excel 功能（公式计算、样式、多工作表等），建议使用 excelize 库。
func readXLSXSimple(path string) ([][]string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 xlsx 文件: %w", err)
	}
	defer r.Close()

	// 1. 解析共享字符串表
	sharedStrings, err := parseSharedStrings(r)
	if err != nil {
		// 共享字符串表可选（纯数值表格可能没有）
		sharedStrings = nil
	}

	// 2. 查找并解析第一个工作表
	var sheetFile *zip.File
	for _, f := range r.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheetFile = f
			break
		}
	}
	if sheetFile == nil {
		return nil, fmt.Errorf("xlsx 中未找到 sheet1.xml")
	}

	rc, err := sheetFile.Open()
	if err != nil {
		return nil, fmt.Errorf("打开 sheet1.xml 失败: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("读取 sheet1.xml 失败: %w", err)
	}

	var ws xlsxWorksheet
	if err := xml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("解析 sheet1.xml 失败: %w", err)
	}

	// 3. 构建二维字符串数组
	var rows [][]string
	for _, row := range ws.SheetData.Rows {
		var cells []string
		for _, cell := range row.Cells {
			// 获取列索引，用于处理空单元格
			colIdx := colNameToIndex(extractColName(cell.R))
			// 填充跳过的空单元格
			for len(cells) < colIdx {
				cells = append(cells, "")
			}

			// 解析单元格值
			value := cell.V
			if cell.T == "s" && sharedStrings != nil {
				// 共享字符串引用
				idx, err := strconv.Atoi(value)
				if err == nil && idx >= 0 && idx < len(sharedStrings) {
					value = sharedStrings[idx]
				}
			}
			cells = append(cells, value)
		}
		rows = append(rows, cells)
	}

	return rows, nil
}

// parseSharedStrings 解析共享字符串表
func parseSharedStrings(r *zip.ReadCloser) ([]string, error) {
	var ssFile *zip.File
	for _, f := range r.File {
		if f.Name == "xl/sharedStrings.xml" {
			ssFile = f
			break
		}
	}
	if ssFile == nil {
		return nil, fmt.Errorf("未找到 sharedStrings.xml")
	}

	rc, err := ssFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var ss xlsxSharedStrings
	if err := xml.Unmarshal(data, &ss); err != nil {
		return nil, err
	}

	result := make([]string, len(ss.SI))
	for i, si := range ss.SI {
		if si.T != "" {
			result[i] = si.T
		} else if len(si.R) > 0 {
			// 富文本：拼接所有段落
			var buf strings.Builder
			for _, run := range si.R {
				buf.WriteString(run.T)
			}
			result[i] = buf.String()
		}
	}
	return result, nil
}

// extractColName 从单元格引用中提取列名（如 "AA1" → "AA"）
func extractColName(ref string) string {
	for i, r := range ref {
		if r >= '0' && r <= '9' {
			return ref[:i]
		}
	}
	return ref
}

// colNameToIndex 将列名转换为从 0 开始的索引
// 例如: "A"→0, "B"→1, "Z"→25, "AA"→26, "AB"→27
func colNameToIndex(name string) int {
	idx := 0
	for _, c := range strings.ToUpper(name) {
		idx = idx*26 + int(c-'A') + 1
	}
	return idx - 1
}

// ============== PPTXLoader ==============

// PPTXLoader PowerPoint 文件加载器（.pptx 格式）
// 提取每张幻灯片的文本内容，每张幻灯片生成一个文档
type PPTXLoader struct {
	path        string
	slidePerDoc bool // true: 每张幻灯片一个文档; false: 整个文件一个文档
}

// PPTXOption PPTX 加载器配置选项
type PPTXOption func(*PPTXLoader)

// WithSlidePerDoc 每张幻灯片生成一个文档
func WithSlidePerDoc(enabled bool) PPTXOption {
	return func(l *PPTXLoader) {
		l.slidePerDoc = enabled
	}
}

// NewPPTXLoader 创建 PPTX 加载器
func NewPPTXLoader(path string, opts ...PPTXOption) *PPTXLoader {
	l := &PPTXLoader{
		path:        path,
		slidePerDoc: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 PPTX 文件
func (l *PPTXLoader) Load(ctx context.Context) ([]rag.Document, error) {
	slides, err := readPPTXSlides(l.path)
	if err != nil {
		return nil, fmt.Errorf("读取 PPTX 文件失败 %s: %w", l.path, err)
	}

	if len(slides) == 0 {
		return nil, nil
	}

	if l.slidePerDoc {
		var docs []rag.Document
		for i, slide := range slides {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			content := strings.TrimSpace(slide)
			if content == "" {
				continue
			}

			doc := rag.Document{
				ID:      util.GenerateID("pptx"),
				Content: content,
				Source:  l.path,
				Metadata: map[string]any{
					"loader":       "pptx",
					"file_path":    l.path,
					"file_name":    filepath.Base(l.path),
					"slide_index":  i + 1,
					"total_slides": len(slides),
				},
				CreatedAt: time.Now(),
			}
			docs = append(docs, doc)
		}
		return docs, nil
	}

	// 整个文件合并为一个文档
	var allContent []string
	for i, slide := range slides {
		content := strings.TrimSpace(slide)
		if content != "" {
			allContent = append(allContent, fmt.Sprintf("--- 幻灯片 %d ---\n%s", i+1, content))
		}
	}

	doc := rag.Document{
		ID:      util.GenerateID("pptx"),
		Content: strings.Join(allContent, "\n\n"),
		Source:  l.path,
		Metadata: map[string]any{
			"loader":       "pptx",
			"file_path":    l.path,
			"file_name":    filepath.Base(l.path),
			"total_slides": len(slides),
		},
		CreatedAt: time.Now(),
	}
	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *PPTXLoader) Name() string {
	return "PPTXLoader"
}

var _ rag.Loader = (*PPTXLoader)(nil)

// ============== PPTX XML 结构体 ==============

// pptxSlide 幻灯片
type pptxSlide struct {
	XMLName xml.Name  `xml:"sld"`
	CSld    pptxCSld  `xml:"cSld"`
}

// pptxCSld 幻灯片内容
type pptxCSld struct {
	SpTree pptxSpTree `xml:"spTree"`
}

// pptxSpTree 形状树
type pptxSpTree struct {
	SPs []pptxSP `xml:"sp"`
}

// pptxSP 形状（包含文本框等）
type pptxSP struct {
	TxBody *pptxTxBody `xml:"txBody"`
}

// pptxTxBody 文本体
type pptxTxBody struct {
	Paragraphs []pptxParagraph `xml:"p"`
}

// pptxParagraph 段落
type pptxParagraph struct {
	Runs []pptxRun `xml:"r"`
}

// pptxRun 文本段
type pptxRun struct {
	T string `xml:"t"`
}

// pptxSlidePattern 用于匹配 ppt/slides/slideN.xml 文件名
var pptxSlidePattern = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// readPPTXSlides 读取 PPTX 文件的幻灯片内容
// pptx 本质是 ZIP 文件，内部包含 ppt/slides/slide*.xml
//
// 解析流程：
//  1. 解压 ZIP 文件
//  2. 找到所有 ppt/slides/slide*.xml 文件
//  3. 按幻灯片序号排序
//  4. 逐个解析 XML，提取文本内容
//  5. 如果 XML 命名空间解析失败，降级为正则提取 <a:t> 标签内容
func readPPTXSlides(path string) ([]string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 pptx 文件: %w", err)
	}
	defer r.Close()

	// 收集所有幻灯片文件并按序号排序
	type slideEntry struct {
		num  int
		file *zip.File
	}
	var slideFiles []slideEntry

	for _, f := range r.File {
		matches := pptxSlidePattern.FindStringSubmatch(f.Name)
		if matches == nil {
			continue
		}
		num, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		slideFiles = append(slideFiles, slideEntry{num: num, file: f})
	}

	sort.Slice(slideFiles, func(i, j int) bool {
		return slideFiles[i].num < slideFiles[j].num
	})

	if len(slideFiles) == 0 {
		return nil, fmt.Errorf("pptx 中未找到幻灯片文件")
	}

	// 逐个解析幻灯片
	var slides []string
	for _, entry := range slideFiles {
		text, err := extractSlideText(entry.file)
		if err != nil {
			// 降级：XML 解析失败时用正则提取
			text, err = extractPPTXTextSimple(entry.file)
			if err != nil {
				continue // 跳过无法解析的幻灯片
			}
		}
		slides = append(slides, text)
	}

	return slides, nil
}

// extractSlideText 从幻灯片 XML 中提取文本内容
func extractSlideText(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	var slide pptxSlide
	if err := xml.Unmarshal(data, &slide); err != nil {
		return "", err
	}

	var paragraphs []string
	for _, sp := range slide.CSld.SpTree.SPs {
		if sp.TxBody == nil {
			continue
		}
		for _, p := range sp.TxBody.Paragraphs {
			var line strings.Builder
			for _, run := range p.Runs {
				line.WriteString(run.T)
			}
			text := strings.TrimSpace(line.String())
			if text != "" {
				paragraphs = append(paragraphs, text)
			}
		}
	}

	return strings.Join(paragraphs, "\n"), nil
}

// pptxTextTagPattern 用于从 XML 中提取 <a:t> 标签内容
var pptxTextTagPattern = regexp.MustCompile(`<a:t[^>]*>([^<]*)</a:t>`)

// extractPPTXTextSimple 降级方案：用正则从 XML 中提取文本
// 当 XML 命名空间解析失败时使用
func extractPPTXTextSimple(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	matches := pptxTextTagPattern.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return "", nil
	}

	var parts []string
	for _, m := range matches {
		text := strings.TrimSpace(string(m[1]))
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " "), nil
}
