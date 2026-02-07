package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ============== 辅助函数 ==============

// createTestCSV 将 CSV 内容写入临时文件并返回文件路径
func createTestCSV(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test*.csv")
	if err != nil {
		t.Fatalf("创建临时 CSV 文件失败: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时 CSV 文件失败: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

// createTestXLSX 根据给定的行数据创建一个有效的 .xlsx 临时文件
// xlsx 本质是 ZIP 包，内含 xl/sharedStrings.xml 和 xl/worksheets/sheet1.xml
// 所有单元格值均通过共享字符串表（t="s"）引用
func createTestXLSX(t *testing.T, rows [][]string) string {
	t.Helper()

	// 收集所有唯一字符串，构建共享字符串表
	stringIndex := make(map[string]int)
	var sharedStrings []string
	for _, row := range rows {
		for _, cell := range row {
			if _, exists := stringIndex[cell]; !exists {
				stringIndex[cell] = len(sharedStrings)
				sharedStrings = append(sharedStrings, cell)
			}
		}
	}

	// 构建 sharedStrings.xml
	var ssBuf bytes.Buffer
	ssBuf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	ssBuf.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`)
	ssBuf.WriteString(fmt.Sprintf(` count="%d" uniqueCount="%d">`, len(sharedStrings), len(sharedStrings)))
	for _, s := range sharedStrings {
		ssBuf.WriteString(fmt.Sprintf(`<si><t>%s</t></si>`, xmlEscape(s)))
	}
	ssBuf.WriteString(`</sst>`)

	// 构建 sheet1.xml
	var sheetBuf bytes.Buffer
	sheetBuf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheetBuf.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sheetBuf.WriteString(`<sheetData>`)
	for rowIdx, row := range rows {
		sheetBuf.WriteString(fmt.Sprintf(`<row r="%d">`, rowIdx+1))
		for colIdx, cell := range row {
			colName := colIndexToName(colIdx)
			cellRef := fmt.Sprintf("%s%d", colName, rowIdx+1)
			idx := stringIndex[cell]
			sheetBuf.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, cellRef, idx))
		}
		sheetBuf.WriteString(`</row>`)
	}
	sheetBuf.WriteString(`</sheetData></worksheet>`)

	// 使用 archive/zip 创建 xlsx 文件
	tmpFile, err := os.CreateTemp("", "test*.xlsx")
	if err != nil {
		t.Fatalf("创建临时 xlsx 文件失败: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	w := zip.NewWriter(tmpFile)

	// 写入 sharedStrings.xml
	ssWriter, err := w.Create("xl/sharedStrings.xml")
	if err != nil {
		t.Fatalf("创建 sharedStrings.xml 失败: %v", err)
	}
	ssWriter.Write(ssBuf.Bytes())

	// 写入 sheet1.xml
	sheetWriter, err := w.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatalf("创建 sheet1.xml 失败: %v", err)
	}
	sheetWriter.Write(sheetBuf.Bytes())

	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}
	tmpFile.Close()

	return tmpFile.Name()
}

// createTestPPTX 根据给定的幻灯片文本列表创建一个有效的 .pptx 临时文件
// pptx 本质是 ZIP 包，内含 ppt/slides/slide*.xml
// 每个幻灯片 XML 包含 <a:t> 标签存放文本内容
func createTestPPTX(t *testing.T, slides []string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test*.pptx")
	if err != nil {
		t.Fatalf("创建临时 pptx 文件失败: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	w := zip.NewWriter(tmpFile)

	for i, text := range slides {
		// 构建幻灯片 XML，使用 <a:t> 标签包裹文本
		slideXML := fmt.Sprintf(
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
				`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`+
				` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">`+
				`<p:cSld><p:spTree><p:sp><p:txBody>`+
				`<a:p><a:r><a:t>%s</a:t></a:r></a:p>`+
				`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
			xmlEscape(text),
		)

		fileName := fmt.Sprintf("ppt/slides/slide%d.xml", i+1)
		fw, err := w.Create(fileName)
		if err != nil {
			t.Fatalf("创建 %s 失败: %v", fileName, err)
		}
		fw.Write([]byte(slideXML))
	}

	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}
	tmpFile.Close()

	return tmpFile.Name()
}

// colIndexToName 将从 0 开始的列索引转换为 Excel 列名
// 例如: 0→"A", 1→"B", 25→"Z", 26→"AA"
func colIndexToName(idx int) string {
	name := ""
	idx++ // 转为从 1 开始
	for idx > 0 {
		idx--
		name = string(rune('A'+idx%26)) + name
		idx /= 26
	}
	return name
}

// xmlEscape 对 XML 特殊字符进行转义
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ============== CSVLoader 测试 ==============

// TestCSVLoader_Basic 测试基本 CSV 解析功能
// 验证默认配置下能正确解析带表头的 CSV 文件
func TestCSVLoader_Basic(t *testing.T) {
	csv := "name,age,city\nAlice,30,Beijing\nBob,25,Shanghai\n"
	path := createTestCSV(t, csv)

	loader := NewCSVLoader(path)
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 CSV 失败: %v", err)
	}

	// 默认有表头，第一列为内容列，应有 2 个文档
	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
	}

	// 验证第一行内容（默认取第一列 name）
	if docs[0].Content != "Alice" {
		t.Errorf("第一个文档内容期望 'Alice'，实际 %q", docs[0].Content)
	}
	if docs[1].Content != "Bob" {
		t.Errorf("第二个文档内容期望 'Bob'，实际 %q", docs[1].Content)
	}

	// 验证元数据
	if docs[0].Metadata["loader"] != "csv" {
		t.Errorf("期望 loader 元数据为 'csv'，实际 %v", docs[0].Metadata["loader"])
	}
	if docs[0].Metadata["file_path"] != path {
		t.Errorf("期望 file_path=%s，实际 %v", path, docs[0].Metadata["file_path"])
	}

	// 验证 Source 字段包含行号信息
	if !strings.Contains(docs[0].Source, path) {
		t.Errorf("期望 Source 包含文件路径，实际 %q", docs[0].Source)
	}

	// 验证 Name 方法
	if loader.Name() != "CSVLoader" {
		t.Errorf("期望 Name() 返回 'CSVLoader'，实际 %q", loader.Name())
	}

	// 验证表头列被写入元数据
	if docs[0].Metadata["name"] != "Alice" {
		t.Errorf("期望 name 元数据为 'Alice'，实际 %v", docs[0].Metadata["name"])
	}
	if docs[0].Metadata["age"] != "30" {
		t.Errorf("期望 age 元数据为 '30'，实际 %v", docs[0].Metadata["age"])
	}
	if docs[0].Metadata["city"] != "Beijing" {
		t.Errorf("期望 city 元数据为 'Beijing'，实际 %v", docs[0].Metadata["city"])
	}
}

// TestCSVLoader_CustomSeparator 测试自定义分隔符
// 验证分号和制表符分隔的 CSV 能被正确解析
func TestCSVLoader_CustomSeparator(t *testing.T) {
	// 测试分号分隔符
	t.Run("分号分隔", func(t *testing.T) {
		csv := "name;age;city\nAlice;30;Beijing\nBob;25;Shanghai\n"
		path := createTestCSV(t, csv)

		loader := NewCSVLoader(path, WithCSVSeparator(';'))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载分号分隔 CSV 失败: %v", err)
		}

		if len(docs) != 2 {
			t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
		}

		// 使用分号分隔时，默认内容列为第一列
		if docs[0].Content != "Alice" {
			t.Errorf("期望内容为 'Alice'，实际 %q", docs[0].Content)
		}
		if docs[0].Metadata["city"] != "Beijing" {
			t.Errorf("期望 city 为 'Beijing'，实际 %v", docs[0].Metadata["city"])
		}
	})

	// 测试制表符分隔符
	t.Run("制表符分隔", func(t *testing.T) {
		csv := "name\tage\tcity\nAlice\t30\tBeijing\n"
		path := createTestCSV(t, csv)

		loader := NewCSVLoader(path, WithCSVSeparator('\t'))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载制表符分隔 CSV 失败: %v", err)
		}

		if len(docs) != 1 {
			t.Fatalf("期望 1 个文档，实际得到 %d 个", len(docs))
		}

		if docs[0].Content != "Alice" {
			t.Errorf("期望内容为 'Alice'，实际 %q", docs[0].Content)
		}
	})
}

// TestCSVLoader_WithHeader 测试表头处理
// 验证开启/关闭表头对解析结果的影响
func TestCSVLoader_WithHeader(t *testing.T) {
	// 测试无表头模式
	t.Run("无表头", func(t *testing.T) {
		csv := "Alice,30,Beijing\nBob,25,Shanghai\n"
		path := createTestCSV(t, csv)

		loader := NewCSVLoader(path, WithCSVHeader(false))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载 CSV 失败: %v", err)
		}

		// 无表头时所有行都是数据行
		if len(docs) != 2 {
			t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
		}

		// 无表头时第一列为内容
		if docs[0].Content != "Alice" {
			t.Errorf("期望内容为 'Alice'，实际 %q", docs[0].Content)
		}

		// 无表头时不应有列名元数据（除了 loader、file_path、row）
		if _, exists := docs[0].Metadata["name"]; exists {
			t.Error("无表头模式不应有列名元数据")
		}
	})

	// 测试有表头模式（默认行为）
	t.Run("有表头", func(t *testing.T) {
		csv := "name,age\nAlice,30\n"
		path := createTestCSV(t, csv)

		loader := NewCSVLoader(path, WithCSVHeader(true))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载 CSV 失败: %v", err)
		}

		// 有表头时第一行为表头，只有 1 个数据行
		if len(docs) != 1 {
			t.Fatalf("期望 1 个文档，实际得到 %d 个", len(docs))
		}

		// 表头列名应出现在元数据中
		if docs[0].Metadata["name"] != "Alice" {
			t.Errorf("期望 name 元数据为 'Alice'，实际 %v", docs[0].Metadata["name"])
		}
		if docs[0].Metadata["age"] != "30" {
			t.Errorf("期望 age 元数据为 '30'，实际 %v", docs[0].Metadata["age"])
		}
	})

	// 测试 WithCSVNoHeader 选项
	t.Run("NoHeader选项", func(t *testing.T) {
		csv := "Alice,30\nBob,25\n"
		path := createTestCSV(t, csv)

		loader := NewCSVLoader(path, WithCSVNoHeader())
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载 CSV 失败: %v", err)
		}

		// 无表头时所有行都是数据
		if len(docs) != 2 {
			t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
		}
	})
}

// TestCSVLoader_ContentColumnSelection 测试指定内容列
// 验证通过 WithCSVContentColumn 可以选择特定列作为文档内容
func TestCSVLoader_ContentColumnSelection(t *testing.T) {
	csv := "id,description,category\n1,这是一段描述,技术\n2,另一段描述,生活\n"
	path := createTestCSV(t, csv)

	loader := NewCSVLoader(path, WithCSVContentColumn("description"))
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 CSV 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
	}

	// 验证内容列为 description 而非默认的第一列 id
	if docs[0].Content != "这是一段描述" {
		t.Errorf("期望内容为 '这是一段描述'，实际 %q", docs[0].Content)
	}
	if docs[1].Content != "另一段描述" {
		t.Errorf("期望内容为 '另一段描述'，实际 %q", docs[1].Content)
	}

	// 验证所有列仍然在元数据中
	if docs[0].Metadata["id"] != "1" {
		t.Errorf("期望 id 元数据为 '1'，实际 %v", docs[0].Metadata["id"])
	}
	if docs[0].Metadata["category"] != "技术" {
		t.Errorf("期望 category 元数据为 '技术'，实际 %v", docs[0].Metadata["category"])
	}
}

// TestCSVLoader_EmptyFile 测试空文件处理
// 验证空文件不会导致错误，而是返回空文档列表
func TestCSVLoader_EmptyFile(t *testing.T) {
	// 测试完全空的文件
	t.Run("完全空文件", func(t *testing.T) {
		path := createTestCSV(t, "")

		loader := NewCSVLoader(path)
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载空 CSV 不应报错: %v", err)
		}

		if len(docs) != 0 {
			t.Errorf("期望 0 个文档，实际得到 %d 个", len(docs))
		}
	})

	// 测试只有表头没有数据的文件
	t.Run("仅有表头", func(t *testing.T) {
		path := createTestCSV(t, "name,age,city\n")

		loader := NewCSVLoader(path)
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载仅有表头的 CSV 不应报错: %v", err)
		}

		if len(docs) != 0 {
			t.Errorf("期望 0 个文档，实际得到 %d 个", len(docs))
		}
	})

	// 测试文件不存在的情况
	t.Run("文件不存在", func(t *testing.T) {
		loader := NewCSVLoader("/nonexistent/path/data.csv")
		ctx := context.Background()

		_, err := loader.Load(ctx)
		if err == nil {
			t.Error("期望加载不存在的文件时返回错误")
		}
	})
}

// ============== ExcelLoader 测试 ==============

// TestExcelLoader_Basic 测试基本 xlsx 文本提取
// 验证能从有效的 xlsx 文件中正确提取行数据
func TestExcelLoader_Basic(t *testing.T) {
	rows := [][]string{
		{"name", "age", "city"},
		{"Alice", "30", "Beijing"},
		{"Bob", "25", "Shanghai"},
	}
	path := createTestXLSX(t, rows)

	loader := NewExcelLoader(path)
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 Excel 失败: %v", err)
	}

	// 第一行是表头（默认 hasHeader=true），所以应有 2 个文档
	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
	}

	// 验证文档内容格式（ExcelLoader 将每列拼接为 "列名: 值" 格式）
	if !strings.Contains(docs[0].Content, "name: Alice") {
		t.Errorf("期望文档内容包含 'name: Alice'，实际 %q", docs[0].Content)
	}
	if !strings.Contains(docs[0].Content, "age: 30") {
		t.Errorf("期望文档内容包含 'age: 30'，实际 %q", docs[0].Content)
	}
	if !strings.Contains(docs[0].Content, "city: Beijing") {
		t.Errorf("期望文档内容包含 'city: Beijing'，实际 %q", docs[0].Content)
	}

	// 验证元数据
	if docs[0].Metadata["loader"] != "excel" {
		t.Errorf("期望 loader 元数据为 'excel'，实际 %v", docs[0].Metadata["loader"])
	}
	if docs[0].Metadata["row_index"] != 1 {
		t.Errorf("期望 row_index 为 1，实际 %v", docs[0].Metadata["row_index"])
	}

	// 验证 Name 方法
	if loader.Name() != "ExcelLoader" {
		t.Errorf("期望 Name() 返回 'ExcelLoader'，实际 %q", loader.Name())
	}
}

// TestExcelLoader_SharedStrings 测试共享字符串引用
// 验证 xlsx 中通过共享字符串表引用的单元格值能被正确解析
func TestExcelLoader_SharedStrings(t *testing.T) {
	// 使用重复的字符串值来确保共享字符串表被正确使用
	rows := [][]string{
		{"类别", "值"},
		{"水果", "苹果"},
		{"水果", "香蕉"},    // "水果" 重复，应共享同一字符串索引
		{"蔬菜", "西红柿"},
	}
	path := createTestXLSX(t, rows)

	loader := NewExcelLoader(path)
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 Excel 失败: %v", err)
	}

	if len(docs) != 3 {
		t.Fatalf("期望 3 个文档，实际得到 %d 个", len(docs))
	}

	// 验证重复字符串能被正确解析
	if !strings.Contains(docs[0].Content, "类别: 水果") {
		t.Errorf("期望包含 '类别: 水果'，实际 %q", docs[0].Content)
	}
	if !strings.Contains(docs[0].Content, "值: 苹果") {
		t.Errorf("期望包含 '值: 苹果'，实际 %q", docs[0].Content)
	}

	// 第二行也是"水果"
	if !strings.Contains(docs[1].Content, "类别: 水果") {
		t.Errorf("期望包含 '类别: 水果'，实际 %q", docs[1].Content)
	}
	if !strings.Contains(docs[1].Content, "值: 香蕉") {
		t.Errorf("期望包含 '值: 香蕉'，实际 %q", docs[1].Content)
	}

	// 第三行
	if !strings.Contains(docs[2].Content, "类别: 蔬菜") {
		t.Errorf("期望包含 '类别: 蔬菜'，实际 %q", docs[2].Content)
	}
}

// TestExcelLoader_MultiColumn 测试多列数据提取
// 验证包含多列的 Excel 文件能正确生成文档内容
func TestExcelLoader_MultiColumn(t *testing.T) {
	rows := [][]string{
		{"编号", "姓名", "部门", "职位", "工龄"},
		{"001", "张三", "研发部", "工程师", "5"},
		{"002", "李四", "市场部", "经理", "8"},
	}
	path := createTestXLSX(t, rows)

	loader := NewExcelLoader(path)
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 Excel 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("期望 2 个文档，实际得到 %d 个", len(docs))
	}

	// 验证第一个文档包含所有列信息
	expected := []string{"编号: 001", "姓名: 张三", "部门: 研发部", "职位: 工程师", "工龄: 5"}
	for _, exp := range expected {
		if !strings.Contains(docs[0].Content, exp) {
			t.Errorf("期望文档包含 %q，实际内容:\n%s", exp, docs[0].Content)
		}
	}

	// 验证第二个文档
	expected2 := []string{"编号: 002", "姓名: 李四", "部门: 市场部", "职位: 经理", "工龄: 8"}
	for _, exp := range expected2 {
		if !strings.Contains(docs[1].Content, exp) {
			t.Errorf("期望文档包含 %q，实际内容:\n%s", exp, docs[1].Content)
		}
	}

	// 验证 file_name 元数据包含文件名
	fileName, ok := docs[0].Metadata["file_name"].(string)
	if !ok || fileName == "" {
		t.Error("期望 file_name 元数据为非空字符串")
	}
}

// ============== PPTXLoader 测试 ==============

// TestPPTXLoader_Basic 测试基本 pptx 幻灯片文本提取
// 验证能从单张幻灯片中正确提取文本内容
func TestPPTXLoader_Basic(t *testing.T) {
	slides := []string{"Hello World"}
	path := createTestPPTX(t, slides)

	loader := NewPPTXLoader(path)
	ctx := context.Background()

	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("加载 PPTX 失败: %v", err)
	}

	// 默认 slidePerDoc=true，1 张幻灯片应产生 1 个文档
	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档，实际得到 %d 个", len(docs))
	}

	// 验证文本内容
	if !strings.Contains(docs[0].Content, "Hello World") {
		t.Errorf("期望内容包含 'Hello World'，实际 %q", docs[0].Content)
	}

	// 验证元数据
	if docs[0].Metadata["loader"] != "pptx" {
		t.Errorf("期望 loader 元数据为 'pptx'，实际 %v", docs[0].Metadata["loader"])
	}
	if docs[0].Metadata["slide_index"] != 1 {
		t.Errorf("期望 slide_index 为 1，实际 %v", docs[0].Metadata["slide_index"])
	}
	if docs[0].Metadata["total_slides"] != 1 {
		t.Errorf("期望 total_slides 为 1，实际 %v", docs[0].Metadata["total_slides"])
	}

	// 验证 Name 方法
	if loader.Name() != "PPTXLoader" {
		t.Errorf("期望 Name() 返回 'PPTXLoader'，实际 %q", loader.Name())
	}

	// 验证 Source
	if docs[0].Source != path {
		t.Errorf("期望 Source=%s，实际 %q", path, docs[0].Source)
	}
}

// TestPPTXLoader_MultipleSlides 测试多张幻灯片的提取和排序
// 验证多张幻灯片按序号正确排序，且每张幻灯片生成独立文档
func TestPPTXLoader_MultipleSlides(t *testing.T) {
	// 测试每张幻灯片独立文档模式
	t.Run("每张幻灯片一个文档", func(t *testing.T) {
		slides := []string{
			"第一页：项目介绍",
			"第二页：技术方案",
			"第三页：进度安排",
		}
		path := createTestPPTX(t, slides)

		loader := NewPPTXLoader(path, WithSlidePerDoc(true))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载 PPTX 失败: %v", err)
		}

		if len(docs) != 3 {
			t.Fatalf("期望 3 个文档，实际得到 %d 个", len(docs))
		}

		// 验证幻灯片顺序
		if !strings.Contains(docs[0].Content, "第一页") {
			t.Errorf("第一个文档应包含第一页内容，实际 %q", docs[0].Content)
		}
		if !strings.Contains(docs[1].Content, "第二页") {
			t.Errorf("第二个文档应包含第二页内容，实际 %q", docs[1].Content)
		}
		if !strings.Contains(docs[2].Content, "第三页") {
			t.Errorf("第三个文档应包含第三页内容，实际 %q", docs[2].Content)
		}

		// 验证 slide_index 元数据递增
		for i, doc := range docs {
			idx, ok := doc.Metadata["slide_index"].(int)
			if !ok {
				t.Errorf("slide_index 应为 int 类型，文档 %d", i)
				continue
			}
			if idx != i+1 {
				t.Errorf("期望 slide_index=%d，实际 %d", i+1, idx)
			}
		}

		// 验证 total_slides 元数据
		for _, doc := range docs {
			if doc.Metadata["total_slides"] != 3 {
				t.Errorf("期望 total_slides=3，实际 %v", doc.Metadata["total_slides"])
			}
		}
	})

	// 测试合并为单个文档模式
	t.Run("合并为单个文档", func(t *testing.T) {
		slides := []string{
			"第一页内容",
			"第二页内容",
		}
		path := createTestPPTX(t, slides)

		loader := NewPPTXLoader(path, WithSlidePerDoc(false))
		ctx := context.Background()

		docs, err := loader.Load(ctx)
		if err != nil {
			t.Fatalf("加载 PPTX 失败: %v", err)
		}

		// 合并模式下应只有一个文档
		if len(docs) != 1 {
			t.Fatalf("期望 1 个文档，实际得到 %d 个", len(docs))
		}

		// 验证合并后的内容包含所有幻灯片的文本
		if !strings.Contains(docs[0].Content, "第一页内容") {
			t.Errorf("合并文档应包含第一页内容，实际 %q", docs[0].Content)
		}
		if !strings.Contains(docs[0].Content, "第二页内容") {
			t.Errorf("合并文档应包含第二页内容，实际 %q", docs[0].Content)
		}

		// 验证 total_slides 元数据
		if docs[0].Metadata["total_slides"] != 2 {
			t.Errorf("期望 total_slides=2，实际 %v", docs[0].Metadata["total_slides"])
		}
	})
}

// TestPPTXLoader_InvalidFile 测试无效文件处理
// 验证各种无效输入场景能正确返回错误
func TestPPTXLoader_InvalidFile(t *testing.T) {
	// 测试文件不存在
	t.Run("文件不存在", func(t *testing.T) {
		loader := NewPPTXLoader("/nonexistent/presentation.pptx")
		ctx := context.Background()

		_, err := loader.Load(ctx)
		if err == nil {
			t.Error("期望加载不存在的文件时返回错误")
		}
	})

	// 测试非 ZIP 格式文件
	t.Run("非ZIP格式", func(t *testing.T) {
		// 创建一个普通文本文件伪装成 pptx
		tmpFile, err := os.CreateTemp("", "invalid*.pptx")
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		t.Cleanup(func() { os.Remove(tmpFile.Name()) })

		tmpFile.WriteString("this is not a valid pptx file")
		tmpFile.Close()

		loader := NewPPTXLoader(tmpFile.Name())
		ctx := context.Background()

		_, err = loader.Load(ctx)
		if err == nil {
			t.Error("期望加载无效 PPTX 文件时返回错误")
		}
	})

	// 测试空 ZIP 文件（无幻灯片）
	t.Run("空ZIP无幻灯片", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "empty*.pptx")
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		t.Cleanup(func() { os.Remove(tmpFile.Name()) })

		// 创建一个合法但不包含幻灯片的 ZIP 文件
		w := zip.NewWriter(tmpFile)
		// 写入一个无关文件
		fw, err := w.Create("content_types.xml")
		if err != nil {
			t.Fatalf("创建 ZIP 条目失败: %v", err)
		}
		fw.Write([]byte("<Types/>"))
		w.Close()
		tmpFile.Close()

		loader := NewPPTXLoader(tmpFile.Name())
		ctx := context.Background()

		_, err = loader.Load(ctx)
		if err == nil {
			t.Error("期望加载无幻灯片的 PPTX 时返回错误")
		}
	})
}
