package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============== 测试辅助函数 ==============

// buildTestDOCX 在内存中构建一个合法的 DOCX (ZIP) 文件，
// 将 paragraphs 切片中的每个字符串作为一个 <w:p> 段落写入 word/document.xml。
// 返回临时文件路径，测试结束后自动清理。
func buildTestDOCX(t *testing.T, paragraphs []string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.docx")

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// 构建 word/document.xml
	var xmlBody strings.Builder
	xmlBody.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	xmlBody.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	xmlBody.WriteString(`<w:body>`)
	for _, p := range paragraphs {
		xmlBody.WriteString(`<w:p><w:r><w:t>`)
		xmlBody.WriteString(p)
		xmlBody.WriteString(`</w:t></w:r></w:p>`)
	}
	xmlBody.WriteString(`</w:body></w:document>`)

	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatalf("创建 document.xml 失败: %v", err)
	}
	if _, err := f.Write([]byte(xmlBody.String())); err != nil {
		t.Fatalf("写入 document.xml 失败: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	return path
}

// buildTestDOCXWithMeta 构建带元数据 (docProps/core.xml) 的 DOCX 文件。
// title 和 creator 写入 core.xml，paragraphs 写入 document.xml。
func buildTestDOCXWithMeta(t *testing.T, paragraphs []string, title, creator string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "meta.docx")

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// 构建 word/document.xml
	var xmlBody strings.Builder
	xmlBody.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	xmlBody.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	xmlBody.WriteString(`<w:body>`)
	for _, p := range paragraphs {
		xmlBody.WriteString(`<w:p><w:r><w:t>`)
		xmlBody.WriteString(p)
		xmlBody.WriteString(`</w:t></w:r></w:p>`)
	}
	xmlBody.WriteString(`</w:body></w:document>`)

	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatalf("创建 document.xml 失败: %v", err)
	}
	f.Write([]byte(xmlBody.String()))

	// 构建 docProps/core.xml（符合 Open Packaging Conventions 元数据格式）
	var coreBuf strings.Builder
	coreBuf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	coreBuf.WriteString(`<cp:coreProperties`)
	coreBuf.WriteString(` xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"`)
	coreBuf.WriteString(` xmlns:dc="http://purl.org/dc/elements/1.1/"`)
	coreBuf.WriteString(` xmlns:dcterms="http://purl.org/dc/terms/">`)
	if title != "" {
		coreBuf.WriteString(fmt.Sprintf(`<dc:title>%s</dc:title>`, title))
	}
	if creator != "" {
		coreBuf.WriteString(fmt.Sprintf(`<dc:creator>%s</dc:creator>`, creator))
	}
	coreBuf.WriteString(`</cp:coreProperties>`)

	cf, err := w.Create("docProps/core.xml")
	if err != nil {
		t.Fatalf("创建 core.xml 失败: %v", err)
	}
	cf.Write([]byte(coreBuf.String()))

	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	return path
}

// buildTestDOCXWithHeadings 构建带标题样式的 DOCX 文件，
// sections 的每个元素是 [heading, content] 对。
func buildTestDOCXWithHeadings(t *testing.T, sections [][2]string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "headings.docx")

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	var xmlBody strings.Builder
	xmlBody.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	xmlBody.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	xmlBody.WriteString(`<w:body>`)
	for i, sec := range sections {
		// 标题段落，使用 Heading{i+1} 样式
		xmlBody.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:pStyle w:val="Heading%d"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`,
			i+1, sec[0],
		))
		// 正文段落
		xmlBody.WriteString(fmt.Sprintf(
			`<w:p><w:r><w:t>%s</w:t></w:r></w:p>`,
			sec[1],
		))
	}
	xmlBody.WriteString(`</w:body></w:document>`)

	f, _ := w.Create("word/document.xml")
	f.Write([]byte(xmlBody.String()))
	w.Close()

	os.WriteFile(path, buf.Bytes(), 0644)
	return path
}

// ============== DOCXLoader 测试 ==============

// TestDOCXLoader_Basic 测试基本文本提取功能。
// 验证从 DOCX 文件中正确读取多个段落的文本内容。
func TestDOCXLoader_Basic(t *testing.T) {
	paragraphs := []string{
		"这是第一个段落",
		"This is the second paragraph",
		"第三段：混合中英文 mixed content",
	}
	path := buildTestDOCX(t, paragraphs)

	l := NewDOCXLoader(path)
	ctx := context.Background()

	docs, err := l.Load(ctx)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望返回 1 个文档, 实际返回 %d 个", len(docs))
	}

	doc := docs[0]

	// 验证所有段落内容都被提取
	for _, p := range paragraphs {
		if !strings.Contains(doc.Content, p) {
			t.Errorf("文档内容应包含段落 %q, 实际内容: %q", p, doc.Content)
		}
	}

	// 验证 Source 为文件路径
	if doc.Source != path {
		t.Errorf("Source 应为 %q, 实际为 %q", path, doc.Source)
	}

	// 验证基础元数据
	if doc.Metadata["loader"] != "docx" {
		t.Errorf("Metadata[loader] 应为 docx, 实际为 %v", doc.Metadata["loader"])
	}
	if doc.ID == "" {
		t.Error("文档 ID 不应为空")
	}
	if doc.CreatedAt.IsZero() {
		t.Error("CreatedAt 不应为零值")
	}
}

// TestDOCXLoader_PreserveParagraphs 测试段落结构保留功能。
// 启用 preserveParagraphs 时，段落之间应以双换行分隔；
// 禁用时，段落之间以单换行分隔（FullText 模式）。
func TestDOCXLoader_PreserveParagraphs(t *testing.T) {
	paragraphs := []string{
		"段落一：引言",
		"段落二：正文",
		"段落三：结论",
	}

	// 子测试: 保留段落结构（默认行为）
	// 注意：同一个 section 内的段落以单换行分隔，不同 section 之间以双换行分隔。
	// 普通段落（无 Heading 样式）都归入同一个 section，因此以 "\n" 分隔。
	t.Run("启用段落保留", func(t *testing.T) {
		path := buildTestDOCX(t, paragraphs)
		l := NewDOCXLoader(path, WithDOCXPreserveParagraphs(true))

		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		content := docs[0].Content
		// 同一 section 内段落以 "\n" 分隔
		if !strings.Contains(content, "段落一：引言\n段落二：正文") {
			t.Errorf("同一 section 内段落之间应以换行分隔, 实际内容: %q", content)
		}
		// 所有段落都应出现
		for _, p := range paragraphs {
			if !strings.Contains(content, p) {
				t.Errorf("内容应包含 %q", p)
			}
		}
	})

	// 子测试: 不保留段落结构
	t.Run("禁用段落保留", func(t *testing.T) {
		path := buildTestDOCX(t, paragraphs)
		l := NewDOCXLoader(path, WithDOCXPreserveParagraphs(false))

		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		content := docs[0].Content
		// 禁用段落保留时，使用 FullText（段落以单换行分隔）
		if strings.Contains(content, "\n\n") {
			t.Errorf("禁用段落保留时不应出现双换行, 实际内容: %q", content)
		}
		// 但仍包含所有段落文本
		for _, p := range paragraphs {
			if !strings.Contains(content, p) {
				t.Errorf("内容应包含 %q", p)
			}
		}
	})
}

// TestDOCXLoader_Metadata 测试文档元数据提取功能。
// 验证从 docProps/core.xml 中正确读取标题 (title) 和作者 (author)。
func TestDOCXLoader_Metadata(t *testing.T) {
	// 子测试: 启用元数据提取
	t.Run("提取标题和作者", func(t *testing.T) {
		path := buildTestDOCXWithMeta(t,
			[]string{"文档正文内容"},
			"测试文档标题",
			"张三",
		)

		l := NewDOCXLoader(path, WithDOCXExtractMetadata(true))
		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		doc := docs[0]
		if doc.Metadata["title"] != "测试文档标题" {
			t.Errorf("Metadata[title] 应为 %q, 实际为 %v", "测试文档标题", doc.Metadata["title"])
		}
		if doc.Metadata["author"] != "张三" {
			t.Errorf("Metadata[author] 应为 %q, 实际为 %v", "张三", doc.Metadata["author"])
		}
	})

	// 子测试: 禁用元数据提取
	t.Run("禁用元数据提取", func(t *testing.T) {
		path := buildTestDOCXWithMeta(t,
			[]string{"内容"},
			"标题",
			"作者",
		)

		l := NewDOCXLoader(path, WithDOCXExtractMetadata(false))
		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		doc := docs[0]
		// 禁用元数据提取时，不应包含 title 和 author 字段
		if _, ok := doc.Metadata["title"]; ok {
			t.Error("禁用元数据提取时不应包含 title")
		}
		if _, ok := doc.Metadata["author"]; ok {
			t.Error("禁用元数据提取时不应包含 author")
		}
		// 但基础元数据仍然存在
		if doc.Metadata["loader"] != "docx" {
			t.Error("基础元数据 loader 应始终存在")
		}
	})
}

// TestDOCXLoader_InvalidFile 测试非法文件的错误处理。
// 包括：不存在的文件路径、非 ZIP 格式的文件内容。
func TestDOCXLoader_InvalidFile(t *testing.T) {
	ctx := context.Background()

	// 子测试: 文件路径不存在
	t.Run("不存在的文件", func(t *testing.T) {
		l := NewDOCXLoader("/tmp/absolutely_nonexistent_file_xyz.docx")
		_, err := l.Load(ctx)
		if err == nil {
			t.Error("打开不存在的文件应返回错误")
		}
	})

	// 子测试: 文件不是有效的 ZIP/DOCX 格式
	t.Run("非ZIP格式文件", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "invalid.docx")
		// 写入纯文本内容（非 ZIP 格式）
		if err := os.WriteFile(path, []byte("这不是一个DOCX文件"), 0644); err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}

		l := NewDOCXLoader(path)
		_, err := l.Load(ctx)
		if err == nil {
			t.Error("非 ZIP 格式文件应返回错误")
		}
	})

	// 子测试: 有效 ZIP 但缺少 word/document.xml
	t.Run("ZIP中缺少document.xml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nodoc.docx")

		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)
		f, _ := w.Create("other/file.txt")
		f.Write([]byte("不相关的内容"))
		w.Close()

		os.WriteFile(path, buf.Bytes(), 0644)

		l := NewDOCXLoader(path)
		_, err := l.Load(ctx)
		if err == nil {
			t.Error("缺少 document.xml 的 ZIP 文件应返回错误")
		}
		if !strings.Contains(err.Error(), "document.xml") {
			t.Errorf("错误信息应提及 document.xml, 实际: %v", err)
		}
	})
}

// TestDOCXLoader_EmptyDocument 测试空文档处理。
// 验证 document.xml 中无段落时的行为。
func TestDOCXLoader_EmptyDocument(t *testing.T) {
	// 构建不包含任何段落的 DOCX
	path := buildTestDOCX(t, []string{})

	l := NewDOCXLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("加载空文档不应报错, 实际: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("空文档也应返回 1 个文档对象, 实际返回 %d 个", len(docs))
	}

	// 空文档的内容应为空字符串
	if docs[0].Content != "" {
		t.Errorf("空文档的内容应为空, 实际: %q", docs[0].Content)
	}
}

// TestDOCXLoader_FromReaderMultiParagraph 测试从 io.ReaderAt 加载多段落文档。
// 验证 NewDOCXLoaderFromReader 路径正确处理多段落内容。
func TestDOCXLoader_FromReaderMultiParagraph(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Reader 第一段</w:t></w:r></w:p>
    <w:p><w:r><w:t>Reader 第二段</w:t></w:r></w:p>
    <w:p><w:r><w:t>Reader 第三段</w:t></w:r></w:p>
  </w:body>
</w:document>`

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	f, _ := w.Create("word/document.xml")
	f.Write([]byte(docXML))
	w.Close()

	reader := bytes.NewReader(buf.Bytes())
	l := NewDOCXLoaderFromReader(reader, int64(reader.Len()))

	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	// 验证所有段落被正确提取
	content := docs[0].Content
	for _, expected := range []string{"Reader 第一段", "Reader 第二段", "Reader 第三段"} {
		if !strings.Contains(content, expected) {
			t.Errorf("内容应包含 %q, 实际: %q", expected, content)
		}
	}

	// 从 Reader 加载时 Source 应为 "reader"（因为没有文件路径）
	if docs[0].Source != "reader" {
		t.Errorf("从 Reader 加载时 Source 应为 reader, 实际: %q", docs[0].Source)
	}
}

// ============== HTMLLoader 测试 ==============

// TestHTMLLoader_Basic 测试 HTML 基本文本提取。
// 验证从 HTML 文件中正确提取文本内容，移除标签。
func TestHTMLLoader_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "basic.html")
	htmlContent := `<!DOCTYPE html>
<html>
<head><title>基本测试</title></head>
<body>
  <h1>标题</h1>
  <p>这是正文段落。</p>
  <p>第二个段落，包含<strong>加粗</strong>文字。</p>
</body>
</html>`
	os.WriteFile(path, []byte(htmlContent), 0644)

	l := NewHTMLLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]

	// 验证文本被正确提取
	if !strings.Contains(doc.Content, "标题") {
		t.Error("内容应包含标题文字")
	}
	if !strings.Contains(doc.Content, "这是正文段落") {
		t.Error("内容应包含正文段落")
	}
	if !strings.Contains(doc.Content, "加粗") {
		t.Error("内容应包含加粗文字（标签已移除但文字保留）")
	}

	// 验证 HTML 标签已被移除
	if strings.Contains(doc.Content, "<h1>") || strings.Contains(doc.Content, "<p>") {
		t.Error("内容不应包含 HTML 标签")
	}

	// 验证标题被提取到元数据
	if doc.Metadata["title"] != "基本测试" {
		t.Errorf("Metadata[title] 应为 %q, 实际为 %v", "基本测试", doc.Metadata["title"])
	}
}

// TestHTMLLoader_StripTags 测试 HTML 标签剥离功能。
// 验证各种 HTML 标签（包括嵌套标签、自闭合标签等）被正确移除。
func TestHTMLLoader_StripTags(t *testing.T) {
	testCases := []struct {
		name     string
		html     string
		expected string    // 期望内容中包含的文本
		absent   []string  // 期望内容中不包含的文本
	}{
		{
			name:     "嵌套标签",
			html:     `<body><div><span>嵌套文本</span></div></body>`,
			expected: "嵌套文本",
			absent:   []string{"<div>", "<span>", "</span>", "</div>"},
		},
		{
			name:     "链接标签",
			html:     `<body><a href="http://example.com">链接文字</a></body>`,
			expected: "链接文字",
			absent:   []string{"<a", "href", "</a>"},
		},
		{
			name:     "图片标签",
			html:     `<body>前文<img src="photo.jpg" alt="照片"/>后文</body>`,
			expected: "前文",
			absent:   []string{"<img", "src="},
		},
		{
			name:     "表格标签",
			html:     `<body><table><tr><td>单元格1</td><td>单元格2</td></tr></table></body>`,
			expected: "单元格1",
			absent:   []string{"<table>", "<tr>", "<td>"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.html)
			l := NewHTMLLoaderFromReader(reader, "")

			docs, err := l.Load(context.Background())
			if err != nil {
				t.Fatalf("Load 失败: %v", err)
			}

			content := docs[0].Content
			if !strings.Contains(content, tc.expected) {
				t.Errorf("内容应包含 %q, 实际: %q", tc.expected, content)
			}
			for _, a := range tc.absent {
				if strings.Contains(content, a) {
					t.Errorf("内容不应包含 %q, 实际: %q", a, content)
				}
			}
		})
	}
}

// TestHTMLLoader_Metadata 测试 HTML 元数据提取。
// 验证标题提取、URL 设置、以及各选项对元数据的影响。
func TestHTMLLoader_Metadata(t *testing.T) {
	// 子测试: 从文件加载时的元数据
	t.Run("文件加载元数据", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "meta.html")
		html := `<html><head><title>页面标题</title></head><body>内容</body></html>`
		os.WriteFile(path, []byte(html), 0644)

		l := NewHTMLLoader(path, WithHTMLExtractTitle(true))
		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		meta := docs[0].Metadata
		if meta["title"] != "页面标题" {
			t.Errorf("title 应为 %q, 实际: %v", "页面标题", meta["title"])
		}
		if meta["loader"] != "html" {
			t.Errorf("loader 应为 html, 实际: %v", meta["loader"])
		}
		// file_name 应为文件名
		if meta["file_name"] != "meta.html" {
			t.Errorf("file_name 应为 meta.html, 实际: %v", meta["file_name"])
		}
	})

	// 子测试: 从 Reader 加载时 URL 作为 Source
	t.Run("Reader加载URL元数据", func(t *testing.T) {
		html := `<html><head><title>远程页面</title></head><body>远程内容</body></html>`
		reader := strings.NewReader(html)

		l := NewHTMLLoaderFromReader(reader, "https://example.com/page")
		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		doc := docs[0]
		if doc.Source != "https://example.com/page" {
			t.Errorf("Source 应为 URL, 实际: %q", doc.Source)
		}
		if doc.Metadata["url"] != "https://example.com/page" {
			t.Errorf("Metadata[url] 应为 URL, 实际: %v", doc.Metadata["url"])
		}
	})

	// 子测试: 禁用标题提取
	t.Run("禁用标题提取", func(t *testing.T) {
		html := `<html><head><title>不该出现</title></head><body>内容</body></html>`
		reader := strings.NewReader(html)

		l := NewHTMLLoaderFromReader(reader, "", WithHTMLExtractTitle(false))
		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		if _, ok := docs[0].Metadata["title"]; ok {
			t.Error("禁用标题提取时不应包含 title 元数据")
		}
	})
}

// TestHTMLLoader_ScriptAndStyleRemoval 测试脚本和样式移除的组合场景。
// 验证同时包含 script 和 style 时各选项的效果。
func TestHTMLLoader_ScriptAndStyleRemoval(t *testing.T) {
	htmlContent := `<html>
<head>
  <style>.highlight { color: red; }</style>
  <title>混合内容</title>
</head>
<body>
  <p>可见文本</p>
  <script>var x = "隐藏的脚本";</script>
  <style>body { background: blue; }</style>
  <p>更多可见文本</p>
</body>
</html>`

	// 子测试: 默认移除脚本和样式
	t.Run("默认移除", func(t *testing.T) {
		reader := strings.NewReader(htmlContent)
		l := NewHTMLLoaderFromReader(reader, "")

		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		content := docs[0].Content
		if strings.Contains(content, "隐藏的脚本") {
			t.Error("默认应移除 script 内容")
		}
		if strings.Contains(content, "background") {
			t.Error("默认应移除 style 内容")
		}
		if !strings.Contains(content, "可见文本") {
			t.Error("正文内容应保留")
		}
	})

	// 子测试: 保留脚本（不移除）
	t.Run("保留脚本", func(t *testing.T) {
		reader := strings.NewReader(htmlContent)
		l := NewHTMLLoaderFromReader(reader, "",
			WithHTMLRemoveScripts(false),
			WithHTMLRemoveStyles(true),
		)

		docs, err := l.Load(context.Background())
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}

		content := docs[0].Content
		// 不移除 script 时，脚本文本会作为内容一部分被提取
		// （标签本身被 stripHTMLTags 去掉，但标签之间的文本会保留）
		if !strings.Contains(content, "可见文本") {
			t.Error("正文内容应保留")
		}
	})
}

// ============== JSONLoader 测试 ==============

// TestJSONLoader_BasicLoad 测试 JSON 文件的基本加载功能。
// 验证 JSON 内容被完整保留为文档 Content。
func TestJSONLoader_BasicLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "simple.json")
	jsonContent := `{"title":"测试","content":"这是JSON内容","count":42}`
	os.WriteFile(path, []byte(jsonContent), 0644)

	l := NewJSONLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	doc := docs[0]

	// JSON 加载器将整个 JSON 作为内容
	if doc.Content != jsonContent {
		t.Errorf("Content 应与原始 JSON 一致\n期望: %q\n实际: %q", jsonContent, doc.Content)
	}

	// 验证基础元数据
	if doc.Metadata["loader"] != "json" {
		t.Errorf("Metadata[loader] 应为 json, 实际: %v", doc.Metadata["loader"])
	}
	if doc.Source != path {
		t.Errorf("Source 应为 %q, 实际: %q", path, doc.Source)
	}
	if doc.Metadata["file_name"] != "simple.json" {
		t.Errorf("Metadata[file_name] 应为 simple.json, 实际: %v", doc.Metadata["file_name"])
	}
	if doc.ID == "" {
		t.Error("文档 ID 不应为空")
	}
}

// TestJSONLoader_NestedContent 测试嵌套 JSON 对象的加载。
// 验证复杂的嵌套结构被完整保留。
func TestJSONLoader_NestedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested.json")

	// 构建嵌套 JSON 结构
	nested := map[string]any{
		"document": map[string]any{
			"title": "嵌套文档",
			"body": map[string]any{
				"sections": []any{
					map[string]any{
						"heading": "第一章",
						"content": "第一章内容",
					},
					map[string]any{
						"heading": "第二章",
						"content": "第二章内容",
					},
				},
			},
			"metadata": map[string]any{
				"author":  "测试作者",
				"version": 2,
			},
		},
	}

	data, err := json.MarshalIndent(nested, "", "  ")
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	os.WriteFile(path, data, 0644)

	l := NewJSONLoader(path, WithJSONContentKey("document.body"))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	content := docs[0].Content

	// 整个 JSON 应作为 Content 保留
	if !strings.Contains(content, "嵌套文档") {
		t.Error("内容应包含嵌套字段值 '嵌套文档'")
	}
	if !strings.Contains(content, "第一章") {
		t.Error("内容应包含嵌套数组元素 '第一章'")
	}
	if !strings.Contains(content, "测试作者") {
		t.Error("内容应包含深层嵌套字段 '测试作者'")
	}
}

// TestJSONLoader_Array 测试 JSON 数组的加载。
// 验证顶层为数组的 JSON 文件能被正确加载。
func TestJSONLoader_Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "array.json")

	// 顶层数组 JSON
	items := []map[string]any{
		{"id": 1, "text": "第一条记录", "tag": "A"},
		{"id": 2, "text": "第二条记录", "tag": "B"},
		{"id": 3, "text": "第三条记录", "tag": "C"},
	}

	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	os.WriteFile(path, data, 0644)

	l := NewJSONLoader(path,
		WithJSONContentKey("text"),
		WithJSONMetadataKeys([]string{"id", "tag"}),
	)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}

	content := docs[0].Content

	// 当前 JSONLoader 实现将整个 JSON 作为单个文档的 Content
	if !strings.Contains(content, "第一条记录") {
		t.Error("内容应包含 '第一条记录'")
	}
	if !strings.Contains(content, "第二条记录") {
		t.Error("内容应包含 '第二条记录'")
	}
	if !strings.Contains(content, "第三条记录") {
		t.Error("内容应包含 '第三条记录'")
	}

	// 验证 Source 为文件路径
	if docs[0].Source != path {
		t.Errorf("Source 应为 %q, 实际: %q", path, docs[0].Source)
	}
}

// TestJSONLoader_FileNotExist 测试不存在的 JSON 文件。
func TestJSONLoader_FileNotExist(t *testing.T) {
	l := NewJSONLoader("/tmp/nonexistent_json_xyz_12345.json")
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("不存在的文件应返回错误")
	}
}

// TestJSONLoader_EmptyObject 测试空 JSON 对象。
func TestJSONLoader_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	l := NewJSONLoader(path)
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("加载空 JSON 对象不应报错: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("期望 1 个文档, 实际 %d", len(docs))
	}
	if docs[0].Content != "{}" {
		t.Errorf("空 JSON 对象的 Content 应为 {}, 实际: %q", docs[0].Content)
	}
}

// TestJSONLoader_OptionsApplied 验证选项被正确应用到 JSONLoader 实例。
func TestJSONLoader_OptionsApplied(t *testing.T) {
	keys := []string{"title", "author", "date"}
	l := NewJSONLoader("test.json",
		WithJSONContentKey("body"),
		WithJSONMetadataKeys(keys),
	)

	if l.contentKey != "body" {
		t.Errorf("contentKey 应为 body, 实际: %q", l.contentKey)
	}
	if len(l.metadataKeys) != 3 {
		t.Errorf("metadataKeys 长度应为 3, 实际: %d", len(l.metadataKeys))
	}
	for i, k := range keys {
		if l.metadataKeys[i] != k {
			t.Errorf("metadataKeys[%d] 应为 %q, 实际: %q", i, k, l.metadataKeys[i])
		}
	}
}

// ============== Name() 方法测试 ==============

// TestLoaderNames 验证所有加载器的 Name() 返回正确的名称。
func TestLoaderNames(t *testing.T) {
	tests := []struct {
		name     string
		loader   interface{ Name() string }
		expected string
	}{
		{
			name:     "DOCXLoader名称",
			loader:   NewDOCXLoader("test.docx"),
			expected: "DOCXLoader",
		},
		{
			name:     "HTMLLoader名称",
			loader:   NewHTMLLoader("test.html"),
			expected: "HTMLLoader",
		},
		{
			name:     "JSONLoader名称",
			loader:   NewJSONLoader("test.json"),
			expected: "JSONLoader",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.loader.Name(); got != tc.expected {
				t.Errorf("Name() = %q, 期望 %q", got, tc.expected)
			}
		})
	}
}

// ============== DOCX 按标题分割测试 ==============

// TestDOCXLoader_SplitByHeadingMultipleSections 测试按标题分割为多个文档。
// 验证每个 Heading 样式的段落开始一个新的章节。
func TestDOCXLoader_SplitByHeadingMultipleSections(t *testing.T) {
	sections := [][2]string{
		{"引言", "这是引言部分的内容。"},
		{"方法", "这是方法部分的内容。"},
		{"结论", "这是结论部分的内容。"},
	}
	path := buildTestDOCXWithHeadings(t, sections)

	l := NewDOCXLoader(path, WithDOCXSplitByHeading(true))
	docs, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	// 按标题分割应产生 3 个文档
	if len(docs) < 3 {
		t.Fatalf("期望至少 3 个文档（按标题分割）, 实际 %d", len(docs))
	}

	// 验证每个文档包含对应章节内容
	for i, sec := range sections {
		if i >= len(docs) {
			break
		}
		if !strings.Contains(docs[i].Content, sec[1]) {
			t.Errorf("文档 %d 应包含章节内容 %q, 实际: %q", i, sec[1], docs[i].Content)
		}
	}

	// 验证 Source 包含 section 索引
	for i, doc := range docs {
		expectedSuffix := fmt.Sprintf("#section=%d", i)
		if !strings.Contains(doc.Source, expectedSuffix) {
			t.Errorf("文档 %d 的 Source 应包含 %q, 实际: %q", i, expectedSuffix, doc.Source)
		}
	}
}

// 确保 bytes 包被使用（避免 unused import 警告）
var _ = bytes.NewReader
