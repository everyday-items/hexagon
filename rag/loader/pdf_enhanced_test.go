package loader

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================================================
// 辅助函数: 构造最小合法 PDF 二进制
// ============================================================

// buildMinimalPDF 构造一个最小合法 PDF，包含指定的未压缩内容流
//
// PDF 结构:
//
//	1 0 obj - Catalog
//	2 0 obj - Pages
//	3 0 obj - Page
//	4 0 obj - Content stream
//	5 0 obj - Info dict (可选)
//	xref + trailer
func buildMinimalPDF(contentStream string, info map[string]string) []byte {
	var buf bytes.Buffer

	// PDF 头
	buf.WriteString("%PDF-1.4\n")

	// 对象 1: Catalog
	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 对象 2: Pages
	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// 对象 3: Page
	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n")

	// 对象 4: Content stream
	obj4Offset := buf.Len()
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n",
		len(contentStream), contentStream))

	// 对象 5: Info（可选）
	obj5Offset := 0
	hasInfo := len(info) > 0
	if hasInfo {
		obj5Offset = buf.Len()
		buf.WriteString("5 0 obj\n<< ")
		for k, v := range info {
			buf.WriteString(fmt.Sprintf("/%s %s ", k, v))
		}
		buf.WriteString(">>\nendobj\n")
	}

	// xref 表
	xrefOffset := buf.Len()
	objCount := 5
	if hasInfo {
		objCount = 6
	}
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", objCount))
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj4Offset))
	if hasInfo {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj5Offset))
	}

	// trailer
	if hasInfo {
		buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\n", objCount))
	} else {
		buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", objCount))
	}
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}

// buildCompressedPDF 构造包含 FlateDecode 压缩流的 PDF
//
// 使用 zlib 压缩（PDF FlateDecode 标准格式，包含 2 字节头和校验和）
func buildCompressedPDF(contentStream string) []byte {
	// 使用 zlib 压缩（PDF FlateDecode 的标准格式）
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write([]byte(contentStream))
	w.Close()
	compData := compressed.Bytes()

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n")

	obj4Offset := buf.Len()
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n",
		len(compData)))
	buf.Write(compData)
	buf.WriteString("\nendstream\nendobj\n")

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 5\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj4Offset))
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\n"))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}

// buildMultiPagePDF 构造多页 PDF
func buildMultiPagePDF(pages []string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	numPages := len(pages)
	// 对象编号分配:
	// 1 - Catalog
	// 2 - Pages
	// 3..3+n-1 - Page 对象
	// 3+n..3+2n-1 - Content 对象

	offsets := make(map[int]int)

	// 对象 1: Catalog
	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 对象 2: Pages
	offsets[2] = buf.Len()
	var kids strings.Builder
	kids.WriteString("[")
	for i := 0; i < numPages; i++ {
		if i > 0 {
			kids.WriteString(" ")
		}
		kids.WriteString(fmt.Sprintf("%d 0 R", 3+i))
	}
	kids.WriteString("]")
	buf.WriteString(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids %s /Count %d >>\nendobj\n",
		kids.String(), numPages))

	// Page 和 Content 对象
	for i := 0; i < numPages; i++ {
		pageID := 3 + i
		contentID := 3 + numPages + i

		offsets[pageID] = buf.Len()
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /Contents %d 0 R >>\nendobj\n",
			pageID, contentID))

		offsets[contentID] = buf.Len()
		content := pages[i]
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n",
			contentID, len(content), content))
	}

	// xref
	totalObjs := 3 + 2*numPages
	xrefOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", totalObjs))
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i < totalObjs; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}

	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", totalObjs))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}

// ============================================================
// 测试用例
// ============================================================

// TestEnhancedPDFParser_BasicText 测试基础文本提取
func TestEnhancedPDFParser_BasicText(t *testing.T) {
	content := "BT\n(Hello World) Tj\nET"
	pdfData := buildMinimalPDF(content, nil)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("未提取到任何页面")
	}

	if !strings.Contains(doc.Pages[0], "Hello World") {
		t.Errorf("页面内容不包含预期文本，实际: %q", doc.Pages[0])
	}
}

// TestEnhancedPDFParser_CompressedStream 测试 FlateDecode 解压
func TestEnhancedPDFParser_CompressedStream(t *testing.T) {
	content := "BT\n(Compressed Text Here) Tj\nET"
	pdfData := buildCompressedPDF(content)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("未提取到任何页面")
	}

	if !strings.Contains(doc.Pages[0], "Compressed Text Here") {
		t.Errorf("解压后文本不正确，实际: %q", doc.Pages[0])
	}
}

// TestEnhancedPDFParser_HexString 测试 hex 字符串解码
func TestEnhancedPDFParser_HexString(t *testing.T) {
	// "One" 的 hex 编码: 4F6E65
	content := "BT\n<4F6E65> Tj\nET"
	pdfData := buildMinimalPDF(content, nil)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("未提取到任何页面")
	}

	if !strings.Contains(doc.Pages[0], "One") {
		t.Errorf("hex 字符串解码失败，实际: %q", doc.Pages[0])
	}
}

// TestEnhancedPDFParser_MultiPage 测试多页分割
func TestEnhancedPDFParser_MultiPage(t *testing.T) {
	pages := []string{
		"BT\n(Page One Content) Tj\nET",
		"BT\n(Page Two Content) Tj\nET",
		"BT\n(Page Three Content) Tj\nET",
	}
	pdfData := buildMultiPagePDF(pages)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) < 3 {
		t.Fatalf("期望至少 3 页，实际 %d 页", len(doc.Pages))
	}

	// 验证每页内容
	expectedTexts := []string{"Page One Content", "Page Two Content", "Page Three Content"}
	for i, expected := range expectedTexts {
		found := false
		for _, page := range doc.Pages {
			if strings.Contains(page, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("第 %d 页未找到预期文本 %q，所有页面: %v", i+1, expected, doc.Pages)
		}
	}

	if doc.Metadata.PageCount != len(doc.Pages) {
		t.Errorf("PageCount 不匹配: 期望 %d，实际 %d", len(doc.Pages), doc.Metadata.PageCount)
	}
}

// TestEnhancedPDFParser_Metadata 测试元数据提取
func TestEnhancedPDFParser_Metadata(t *testing.T) {
	content := "BT\n(Test) Tj\nET"
	info := map[string]string{
		"Title":        "(Test Document)",
		"Author":       "(John Doe)",
		"Subject":      "(Testing)",
		"Creator":      "(TestSuite)",
		"Producer":     "(Go PDF Builder)",
		"CreationDate": "(D:20230615120000+08'00')",
	}
	pdfData := buildMinimalPDF(content, info)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if doc.Metadata.Title != "Test Document" {
		t.Errorf("Title 不匹配: 期望 %q, 实际 %q", "Test Document", doc.Metadata.Title)
	}
	if doc.Metadata.Author != "John Doe" {
		t.Errorf("Author 不匹配: 期望 %q, 实际 %q", "John Doe", doc.Metadata.Author)
	}
	if doc.Metadata.Subject != "Testing" {
		t.Errorf("Subject 不匹配: 期望 %q, 实际 %q", "Testing", doc.Metadata.Subject)
	}
	if doc.Metadata.Creator != "TestSuite" {
		t.Errorf("Creator 不匹配: 期望 %q, 实际 %q", "TestSuite", doc.Metadata.Creator)
	}
	if doc.Metadata.Producer != "Go PDF Builder" {
		t.Errorf("Producer 不匹配: 期望 %q, 实际 %q", "Go PDF Builder", doc.Metadata.Producer)
	}
	if doc.Metadata.CreationDate.IsZero() {
		t.Error("CreationDate 为零值")
	} else {
		expected := time.Date(2023, 6, 15, 12, 0, 0, 0,
			time.FixedZone("UTC+8", 8*3600))
		if !doc.Metadata.CreationDate.Equal(expected) {
			t.Errorf("CreationDate 不匹配: 期望 %v, 实际 %v", expected, doc.Metadata.CreationDate)
		}
	}
}

// TestEnhancedPDFParser_TJArray 测试 TJ 数组操作符
func TestEnhancedPDFParser_TJArray(t *testing.T) {
	content := "BT\n[(H) 10 (ello) -300 (World)] TJ\nET"
	pdfData := buildMinimalPDF(content, nil)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("未提取到任何页面")
	}

	// -300 间距应产生空格
	page := doc.Pages[0]
	if !strings.Contains(page, "Hello") {
		t.Errorf("TJ 数组未正确拼接，实际: %q", page)
	}
	if !strings.Contains(page, "World") {
		t.Errorf("TJ 数组缺少 World，实际: %q", page)
	}
}

// TestEnhancedPDFParser_TextMovement 测试文本位置移动（换行）
func TestEnhancedPDFParser_TextMovement(t *testing.T) {
	content := "BT\n1 0 0 1 72 720 Tm\n(Line One) Tj\n0 -14 Td\n(Line Two) Tj\nET"
	pdfData := buildMinimalPDF(content, nil)

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("未提取到任何页面")
	}

	page := doc.Pages[0]
	if !strings.Contains(page, "Line One") || !strings.Contains(page, "Line Two") {
		t.Errorf("文本移动解析不正确，实际: %q", page)
	}
}

// TestEnhancedPDFParser_InvalidPDF 测试无效 PDF 输入
func TestEnhancedPDFParser_InvalidPDF(t *testing.T) {
	parser := &EnhancedPDFParser{}

	// 非 PDF 数据
	_, err := parser.Parse(context.Background(), bytes.NewReader([]byte("not a pdf")))
	if err == nil {
		t.Error("期望非 PDF 输入返回错误")
	}

	// 空数据
	_, err = parser.Parse(context.Background(), bytes.NewReader([]byte{}))
	if err == nil {
		t.Error("期望空输入返回错误")
	}
}

// TestEnhancedPDFParser_FallbackToSimple 测试降级到简单解析器
func TestEnhancedPDFParser_FallbackToSimple(t *testing.T) {
	// 构造一个有效 PDF 签名但没有标准结构的 PDF
	data := []byte("%PDF-1.4\n(Some text in parens) more data\n%%EOF\n")

	parser := &EnhancedPDFParser{}
	doc, err := parser.Parse(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("降级解析失败: %v", err)
	}

	// 降级后应该能提取到一些文本
	if len(doc.Pages) == 0 {
		t.Fatal("降级后未提取到任何页面")
	}
}

// ============================================================
// parseContentStream 单元测试
// ============================================================

// TestParseContentStream 测试内容流解析
func TestParseContentStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单 Tj",
			input:    "BT\n(Hello) Tj\nET",
			expected: "Hello",
		},
		{
			name:     "多个 Tj",
			input:    "BT\n(Hello) Tj\n( World) Tj\nET",
			expected: "Hello World",
		},
		{
			name:     "TJ 数组",
			input:    "BT\n[(AB) 50 (CD)] TJ\nET",
			expected: "ABCD",
		},
		{
			name:     "单引号操作符",
			input:    "BT\n(Line1) Tj\n(Line2) '\nET",
			expected: "Line1\nLine2",
		},
		{
			name:     "T* 换行",
			input:    "BT\n(First) Tj\nT*\n(Second) Tj\nET",
			expected: "First\nSecond",
		},
		{
			name:     "空内容",
			input:    "q\nQ\n",
			expected: "",
		},
		{
			name:     "多个 BT/ET 块",
			input:    "BT\n(Block1) Tj\nET\nBT\n(Block2) Tj\nET",
			expected: "Block1\nBlock2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseContentStream([]byte(tt.input))
			if strings.TrimSpace(result) != strings.TrimSpace(tt.expected) {
				t.Errorf("期望 %q, 实际 %q", tt.expected, result)
			}
		})
	}
}

// ============================================================
// 编码解码测试
// ============================================================

// TestDecodePDFString 测试 PDF 字符串解码
func TestDecodePDFString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "括号字符串",
			input:    "(Hello World)",
			expected: "Hello World",
		},
		{
			name:     "hex 字符串",
			input:    "<48656C6C6F>",
			expected: "Hello",
		},
		{
			name:     "转义换行",
			input:    "(Line1\\nLine2)",
			expected: "Line1\nLine2",
		},
		{
			name:     "转义括号",
			input:    "(Open\\(Close\\))",
			expected: "Open(Close)",
		},
		{
			name:     "八进制编码",
			input:    "(\\110\\145\\154\\154\\157)",
			expected: "Hello",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "空括号",
			input:    "()",
			expected: "",
		},
		{
			name:     "空 hex",
			input:    "<>",
			expected: "",
		},
		{
			name:     "hex 奇数长度",
			input:    "<4F6E650>",
			expected: "One\x00",
		},
		{
			name:     "无包裹",
			input:    "plain text",
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodePDFString(tt.input)
			if result != tt.expected {
				t.Errorf("期望 %q, 实际 %q", tt.expected, result)
			}
		})
	}
}

// TestDecodePDFDate 测试 PDF 日期解析
func TestDecodePDFDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			name:     "完整日期带时区",
			input:    "D:20230615120000+08'00'",
			expected: time.Date(2023, 6, 15, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)),
		},
		{
			name:     "UTC 时区",
			input:    "D:20230101000000Z",
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "仅年月日",
			input:    "D:20230615",
			expected: time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "仅年份",
			input:    "D:2023",
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "无 D: 前缀",
			input:    "20230615120000",
			expected: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "带括号",
			input:    "(D:20230615120000)",
			expected: time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "负时区",
			input:    "D:20230615120000-05'00'",
			expected: time.Date(2023, 6, 15, 12, 0, 0, 0, time.FixedZone("UTC-5", -5*3600)),
		},
		{
			name:     "无效输入",
			input:    "xxx",
			expected: time.Time{},
		},
		{
			name:     "空字符串",
			input:    "",
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodePDFDate(tt.input)
			if !result.Equal(tt.expected) {
				t.Errorf("期望 %v, 实际 %v", tt.expected, result)
			}
		})
	}
}

// TestDecodeHexString 测试 hex 字符串解码
func TestDecodeHexString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "标准 hex",
			input:    "48656C6C6F",
			expected: "Hello",
		},
		{
			name:     "带空格",
			input:    "48 65 6C 6C 6F",
			expected: "Hello",
		},
		{
			name:     "小写 hex",
			input:    "48656c6c6f",
			expected: "Hello",
		},
		{
			name:     "空 hex",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeHexString(tt.input)
			if result != tt.expected {
				t.Errorf("期望 %q, 实际 %q", tt.expected, result)
			}
		})
	}
}

// TestDecodeWinAnsi 测试 WinAnsi 编码转换
func TestDecodeWinAnsi(t *testing.T) {
	// ASCII 范围正常通过
	result := decodeWinAnsi([]byte("Hello"))
	if result != "Hello" {
		t.Errorf("ASCII 解码失败: %q", result)
	}

	// 特殊字符映射
	result = decodeWinAnsi([]byte{0x80}) // € (Euro sign)
	if result != "\u20AC" {
		t.Errorf("Euro sign 解码失败: %q (期望 €)", result)
	}

	result = decodeWinAnsi([]byte{0x93}) // " (左双引号)
	if result != "\u201C" {
		t.Errorf("左双引号解码失败: %q", result)
	}
}

// TestExtractTJText 测试 TJ 数组文本提取
func TestExtractTJText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "简单数组",
			input:    "[(Hello)] TJ",
			contains: "Hello",
		},
		{
			name:     "带间距",
			input:    "[(H) 10 (ello)] TJ",
			contains: "Hello",
		},
		{
			name:     "大间距产生空格",
			input:    "[(Hello) -500 (World)] TJ",
			contains: "Hello World",
		},
		{
			name:     "hex 元素",
			input:    "[<48656C6C6F>] TJ",
			contains: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTJText(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("期望包含 %q, 实际 %q", tt.contains, result)
			}
		})
	}
}

// TestParsePDFDict 测试 PDF 字典解析
func TestParsePDFDict(t *testing.T) {
	data := []byte(`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`)
	dict := parsePDFDict(data)

	if dict["Type"] != "/Page" {
		t.Errorf("Type 不匹配: %q", dict["Type"])
	}
	if !strings.Contains(dict["Parent"], "2 0 R") {
		t.Errorf("Parent 不匹配: %q", dict["Parent"])
	}
	if !strings.Contains(dict["Contents"], "4 0 R") {
		t.Errorf("Contents 不匹配: %q", dict["Contents"])
	}
}

// TestDecompressFlate_Zlib 测试 zlib 格式解压（标准 FlateDecode）
func TestDecompressFlate_Zlib(t *testing.T) {
	original := []byte("This is test content for compression")

	// 使用 zlib 压缩
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(original)
	w.Close()

	// 解压
	decompressed, err := decompressFlate(buf.Bytes())
	if err != nil {
		t.Fatalf("zlib 解压失败: %v", err)
	}

	if string(decompressed) != string(original) {
		t.Errorf("解压内容不匹配: 期望 %q, 实际 %q", original, decompressed)
	}
}

// TestDecompressFlate_RawDeflate 测试 raw deflate 格式解压（降级路径）
func TestDecompressFlate_RawDeflate(t *testing.T) {
	original := []byte("This is test content for raw deflate")

	// 使用 raw deflate 压缩
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("deflate 压缩创建失败: %v", err)
	}
	w.Write(original)
	w.Close()

	// 解压
	decompressed, err := decompressFlate(buf.Bytes())
	if err != nil {
		t.Fatalf("raw deflate 解压失败: %v", err)
	}

	if string(decompressed) != string(original) {
		t.Errorf("解压内容不匹配: 期望 %q, 实际 %q", original, decompressed)
	}
}

// TestDecompressFlate_Invalid 测试无效数据解压
func TestDecompressFlate_Invalid(t *testing.T) {
	_, err := decompressFlate([]byte("not compressed data"))
	if err == nil {
		t.Error("期望无效数据解压返回错误")
	}
}

// TestEnhancedPDFParser_WithPDFLoader 测试通过 PDFLoader 集成使用
func TestEnhancedPDFParser_WithPDFLoader(t *testing.T) {
	content := "BT\n(Integration Test) Tj\nET"
	pdfData := buildMinimalPDF(content, nil)

	loader := NewPDFLoaderFromReader(
		bytes.NewReader(pdfData),
		WithPDFParser(&EnhancedPDFParser{}),
		WithPDFSplitPages(true),
	)

	docs, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if len(docs) == 0 {
		t.Fatal("未加载到任何文档")
	}

	if !strings.Contains(docs[0].Content, "Integration Test") {
		t.Errorf("文档内容不包含预期文本: %q", docs[0].Content)
	}
}

// TestFindMatchingParen 测试括号匹配
func TestFindMatchingParen(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		expected int
	}{
		{
			name:     "简单括号",
			input:    "(hello)",
			start:    0,
			expected: 6,
		},
		{
			name:     "嵌套括号",
			input:    "(a(b)c)",
			start:    0,
			expected: 6,
		},
		{
			name:     "转义括号",
			input:    `(a\)b)`,
			start:    0,
			expected: 5,
		},
		{
			name:     "无匹配",
			input:    "(unclosed",
			start:    0,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findMatchingParen(tt.input, tt.start)
			if result != tt.expected {
				t.Errorf("期望 %d, 实际 %d", tt.expected, result)
			}
		})
	}
}

// TestExtractFirstInt 测试整数提取
func TestExtractFirstInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"\n  12345\n%%EOF", 12345},
		{"abc", 0},
		{"  0  ", 0},
		{"  42xyz", 42},
	}

	for _, tt := range tests {
		result := extractFirstInt([]byte(tt.input))
		if result != tt.expected {
			t.Errorf("extractFirstInt(%q) = %d, 期望 %d", tt.input, result, tt.expected)
		}
	}
}
