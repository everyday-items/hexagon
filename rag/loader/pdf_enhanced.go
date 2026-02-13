// Package loader 提供 RAG 系统的文档加载器
//
// 本文件实现增强型 PDF 解析器 EnhancedPDFParser，支持：
//   - xref 交叉引用表解析和对象定位
//   - FlateDecode (zlib) 压缩流解压
//   - hex 字符串 <4F6E65> 解码
//   - BT/ET 文本操作符解析 (Tj/TJ/'/" 操作符)
//   - /Info 字典元数据提取
//
// 仅依赖标准库（compress/flate），无额外三方依赖。

package loader

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// EnhancedPDFParser 增强型 PDF 解析器
//
// 支持现代 PDF 的核心特性：xref 表、压缩流、文本操作符解析、元数据提取。
// 适用于大部分标准 PDF 文件的文本提取场景。
//
// 限制：
//   - 不支持加密 PDF
//   - 不支持 CMap 字体映射（CID 字体可能输出为乱码）
//   - 不支持 XRef Stream（PDF 1.5+）的交叉引用流
//   - 不处理图片、表格等非文本内容
type EnhancedPDFParser struct{}

// Parse 解析 PDF 并返回结构化文档
//
// 解析流程:
//  1. 读取全部数据并验证 PDF 签名
//  2. 解析 xref 交叉引用表定位对象
//  3. 提取 /Info 字典中的元数据
//  4. 遍历页面树获取每页内容流
//  5. 解压并解析内容流中的文本操作符
//
// 如果增强解析失败，自动降级到简单文本提取。
func (p *EnhancedPDFParser) Parse(ctx context.Context, r io.Reader) (*PDFDocument, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 PDF 数据失败: %w", err)
	}

	// 验证 PDF 签名
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return nil, fmt.Errorf("不是有效的 PDF 文件")
	}

	parser := &pdfInternalParser{
		data:    data,
		objects: make(map[int]pdfObject),
	}

	// 解析 xref 表
	parser.parseXRef()

	// 解析所有对象
	parser.parseAllObjects()

	// 提取元数据
	metadata := parser.extractMetadata()

	// 提取页面文本
	pages := parser.extractPages()

	// 如果没有提取到任何文本，降级到简单解析
	if len(pages) == 0 || allEmpty(pages) {
		fallback := &SimplePDFParser{}
		return fallback.Parse(ctx, bytes.NewReader(data))
	}

	metadata.PageCount = len(pages)

	return &PDFDocument{
		Pages:    pages,
		Metadata: metadata,
	}, nil
}

// allEmpty 检查所有页面是否为空
func allEmpty(pages []string) bool {
	for _, p := range pages {
		if strings.TrimSpace(p) != "" {
			return false
		}
	}
	return true
}

// pdfObject 表示一个 PDF 间接对象
type pdfObject struct {
	// id 对象编号
	id int
	// gen 生成号
	gen int
	// offset 在文件中的偏移量
	offset int
	// raw 对象的原始字节内容
	raw []byte
	// stream 对象关联的流数据（如果有）
	stream []byte
	// dict 对象字典（键值对）
	dict map[string]string
}

// pdfInternalParser PDF 内部解析状态机
type pdfInternalParser struct {
	// data PDF 文件的完整字节内容
	data []byte
	// objects 通过 xref 定位到的所有对象，按对象 ID 索引
	objects map[int]pdfObject
	// xrefOffsets xref 表中记录的对象偏移量，按对象 ID 索引
	xrefOffsets map[int]int
}

// parseXRef 解析 xref 交叉引用表
//
// xref 表位于 PDF 文件末尾，通过 startxref 标记定位。
// 表中记录了每个对象在文件中的字节偏移量。
// 同时支持 xref 表和非标准格式（直接扫描对象定义）。
func (p *pdfInternalParser) parseXRef() {
	p.xrefOffsets = make(map[int]int)

	// 从文件末尾查找 startxref
	tail := p.data
	if len(tail) > 1024 {
		tail = tail[len(tail)-1024:]
	}

	startxrefIdx := bytes.LastIndex(tail, []byte("startxref"))
	if startxrefIdx >= 0 {
		// 提取 xref 偏移量
		after := tail[startxrefIdx+9:]
		xrefOffset := extractFirstInt(after)
		if xrefOffset > 0 && xrefOffset < len(p.data) {
			p.parseXRefTable(xrefOffset)
		}
	}

	// 补充：直接扫描对象定义（处理无 xref 表或 xref stream 的情况）
	p.scanObjectDefinitions()
}

// parseXRefTable 解析标准 xref 表
//
// 格式:
//
//	xref
//	0 6
//	0000000000 65535 f
//	0000000009 00000 n
//	...
func (p *pdfInternalParser) parseXRefTable(offset int) {
	if offset >= len(p.data) {
		return
	}

	section := p.data[offset:]

	// 确认是 xref 关键字
	if !bytes.HasPrefix(bytes.TrimSpace(section), []byte("xref")) {
		return
	}

	lines := bytes.Split(section, []byte("\n"))
	objID := 0
	count := 0
	headerParsed := false

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// 跳过 xref 关键字
		if bytes.Equal(line, []byte("xref")) {
			continue
		}

		// 遇到 trailer 停止
		if bytes.HasPrefix(line, []byte("trailer")) {
			break
		}

		parts := bytes.Fields(line)

		// 子节头: startID count
		if len(parts) == 2 && !headerParsed || (len(parts) == 2 && count == 0) {
			id, err1 := strconv.Atoi(string(parts[0]))
			cnt, err2 := strconv.Atoi(string(parts[1]))
			if err1 == nil && err2 == nil {
				objID = id
				count = cnt
				headerParsed = true
				continue
			}
		}

		// 条目: offset gen f/n
		if len(parts) == 3 && headerParsed && count > 0 {
			off, err := strconv.Atoi(string(parts[0]))
			if err == nil && bytes.Equal(parts[2], []byte("n")) {
				p.xrefOffsets[objID] = off
			}
			objID++
			count--
		}
	}
}

// scanObjectDefinitions 直接扫描对象定义
//
// 作为 xref 解析的补充，通过正则匹配 "N 0 obj" 模式
// 直接从文件中定位对象位置。这对于 xref 表损坏或
// 使用 xref stream (PDF 1.5+) 的文件特别有用。
func (p *pdfInternalParser) scanObjectDefinitions() {
	// 匹配 "N G obj" 模式（其中 N 为对象编号，G 为生成号）
	re := regexp.MustCompile(`(\d+)\s+(\d+)\s+obj\b`)
	matches := re.FindAllIndex(p.data, -1)

	for _, loc := range matches {
		prefix := p.data[loc[0]:loc[1]]
		parts := bytes.Fields(prefix)
		if len(parts) >= 2 {
			id, err := strconv.Atoi(string(parts[0]))
			if err == nil {
				// 只在 xref 中没有记录时添加
				if _, exists := p.xrefOffsets[id]; !exists {
					p.xrefOffsets[id] = loc[0]
				}
			}
		}
	}
}

// parseAllObjects 解析所有已定位的 PDF 对象
func (p *pdfInternalParser) parseAllObjects() {
	for id, offset := range p.xrefOffsets {
		obj := p.parseObjectAt(id, offset)
		if obj != nil {
			p.objects[id] = *obj
		}
	}
}

// parseObjectAt 在指定偏移量处解析 PDF 对象
//
// PDF 对象格式:
//
//	N G obj
//	  << /Key Value ... >>    % 字典
//	  stream                  % 可选的流数据
//	  ...bytes...
//	  endstream
//	endobj
func (p *pdfInternalParser) parseObjectAt(id, offset int) *pdfObject {
	if offset >= len(p.data) {
		return nil
	}

	// 查找 endobj
	endIdx := bytes.Index(p.data[offset:], []byte("endobj"))
	if endIdx < 0 {
		return nil
	}

	raw := p.data[offset : offset+endIdx+6]

	obj := &pdfObject{
		id:     id,
		offset: offset,
		raw:    raw,
	}

	// 解析字典
	obj.dict = parsePDFDict(raw)

	// 解析流
	streamStart := bytes.Index(raw, []byte("stream"))
	if streamStart >= 0 {
		// stream 关键字后的 \r\n 或 \n
		streamDataStart := streamStart + 6
		if streamDataStart < len(raw) && raw[streamDataStart] == '\r' {
			streamDataStart++
		}
		if streamDataStart < len(raw) && raw[streamDataStart] == '\n' {
			streamDataStart++
		}

		streamEnd := bytes.Index(raw[streamDataStart:], []byte("endstream"))
		if streamEnd >= 0 {
			streamBytes := raw[streamDataStart : streamDataStart+streamEnd]
			// PDF 规范: endstream 前有一个 EOL 标记（\n 或 \r\n），不属于流数据
			if len(streamBytes) > 0 && streamBytes[len(streamBytes)-1] == '\n' {
				streamBytes = streamBytes[:len(streamBytes)-1]
				if len(streamBytes) > 0 && streamBytes[len(streamBytes)-1] == '\r' {
					streamBytes = streamBytes[:len(streamBytes)-1]
				}
			} else if len(streamBytes) > 0 && streamBytes[len(streamBytes)-1] == '\r' {
				streamBytes = streamBytes[:len(streamBytes)-1]
			}
			obj.stream = streamBytes
		}
	}

	return obj
}

// parsePDFDict 解析 PDF 字典 << ... >>
//
// 返回键值对映射，键为 /Name 格式（不含前导斜杠），值为原始字符串。
// Name 类型的值保留前导斜杠（如 /Page、/FlateDecode）。
// 支持嵌套字典、数组、字符串和间接引用的正确解析。
func parsePDFDict(data []byte) map[string]string {
	dict := make(map[string]string)
	s := string(data)

	// 查找最外层字典
	dictStart := strings.Index(s, "<<")
	if dictStart < 0 {
		return dict
	}

	// 找到匹配的 >>
	depth := 0
	dictEnd := -1
	for i := dictStart; i < len(s)-1; i++ {
		if s[i] == '<' && s[i+1] == '<' {
			depth++
			i++
		} else if s[i] == '>' && s[i+1] == '>' {
			depth--
			if depth == 0 {
				dictEnd = i + 2
				break
			}
			i++
		}
	}

	if dictEnd < 0 {
		dictEnd = len(s)
	}

	content := s[dictStart+2 : dictEnd-2]
	pos := 0

	for {
		// 跳过空白，找到下一个 key（必须以 / 开头）
		pos = pdfSkipWhitespace(content, pos)
		if pos >= len(content) || content[pos] != '/' {
			break
		}

		// 读取 key 名称（跳过前导 /）
		pos++
		keyStart := pos
		for pos < len(content) && pdfIsNameChar(content[pos]) {
			pos++
		}
		key := content[keyStart:pos]
		if key == "" {
			break
		}

		// 跳过空白
		pos = pdfSkipWhitespace(content, pos)
		if pos >= len(content) {
			dict[key] = ""
			break
		}

		// 读取 value（根据首字符判断类型）
		value, newPos := pdfReadValue(content, pos)
		dict[key] = value
		pos = newPos
	}

	return dict
}

// pdfSkipWhitespace 跳过 PDF 空白字符
func pdfSkipWhitespace(s string, pos int) int {
	for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t' || s[pos] == '\r' || s[pos] == '\n') {
		pos++
	}
	return pos
}

// pdfIsNameChar 判断字符是否为 PDF Name 的合法组成字符
//
// PDF Name 可包含字母、数字、下划线。
// 不包含空白和定界符 (/ < > ( ) [ ] { } %)。
func pdfIsNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' || c == '+'
}

// pdfReadValue 从指定位置读取一个 PDF 值
//
// 支持的值类型：
//   - /Name  — 名称（如 /Page, /FlateDecode）
//   - (text) — 括号字符串
//   - <hex>  — 十六进制字符串
//   - <<..>> — 嵌套字典
//   - [...]  — 数组
//   - 数字、布尔、null、间接引用 (N G R)
//
// 返回值的字符串表示和新的位置。
func pdfReadValue(s string, pos int) (string, int) {
	if pos >= len(s) {
		return "", pos
	}

	switch {
	case s[pos] == '/':
		// Name 值: /SomeName
		start := pos
		pos++
		for pos < len(s) && pdfIsNameChar(s[pos]) {
			pos++
		}
		return s[start:pos], pos

	case s[pos] == '(':
		// 括号字符串: (text)
		start := pos
		depth := 0
		for pos < len(s) {
			if pos > start && s[pos-1] == '\\' {
				pos++
				continue
			}
			if s[pos] == '(' {
				depth++
			} else if s[pos] == ')' {
				depth--
				if depth == 0 {
					pos++
					return s[start:pos], pos
				}
			}
			pos++
		}
		return s[start:pos], pos

	case s[pos] == '<':
		if pos+1 < len(s) && s[pos+1] == '<' {
			// 嵌套字典: << ... >>
			start := pos
			depth := 0
			for pos < len(s)-1 {
				if s[pos] == '<' && s[pos+1] == '<' {
					depth++
					pos += 2
				} else if s[pos] == '>' && s[pos+1] == '>' {
					depth--
					pos += 2
					if depth == 0 {
						return s[start:pos], pos
					}
				} else {
					pos++
				}
			}
			return s[start:pos], pos
		}
		// hex 字符串: <hex>
		start := pos
		end := strings.Index(s[pos:], ">")
		if end >= 0 {
			pos += end + 1
			return s[start:pos], pos
		}
		pos++
		return s[start:pos], pos

	case s[pos] == '[':
		// 数组: [...]
		start := pos
		depth := 0
		for pos < len(s) {
			if s[pos] == '[' {
				depth++
			} else if s[pos] == ']' {
				depth--
				if depth == 0 {
					pos++
					return s[start:pos], pos
				}
			}
			pos++
		}
		return s[start:pos], pos

	default:
		// 数字、布尔、null、间接引用 (N G R)
		start := pos
		for pos < len(s) {
			c := s[pos]
			// 遇到定界符停止
			if c == '/' || c == '<' || c == '(' || c == '[' || c == '>' || c == ']' {
				break
			}
			pos++
		}
		return strings.TrimSpace(s[start:pos]), pos
	}
}

// extractMetadata 从 /Info 字典中提取元数据
//
// 搜索 trailer 中的 /Info 引用，然后解析对应对象中的标准字段:
// /Title, /Author, /Subject, /Creator, /Producer, /CreationDate, /ModDate
func (p *pdfInternalParser) extractMetadata() PDFMetadata {
	meta := PDFMetadata{}

	// 从 trailer 查找 /Info 引用
	infoID := p.findTrailerRef("Info")
	if infoID <= 0 {
		// 直接遍历所有对象查找包含元数据字段的对象
		for _, obj := range p.objects {
			if _, hasTitle := obj.dict["Title"]; hasTitle {
				if _, hasAuthor := obj.dict["Author"]; hasAuthor {
					meta.Title = decodePDFString(obj.dict["Title"])
					meta.Author = decodePDFString(obj.dict["Author"])
					meta.Subject = decodePDFString(obj.dict["Subject"])
					meta.Creator = decodePDFString(obj.dict["Creator"])
					meta.Producer = decodePDFString(obj.dict["Producer"])
					if cd, ok := obj.dict["CreationDate"]; ok {
						meta.CreationDate = decodePDFDate(cd)
					}
					if md, ok := obj.dict["ModDate"]; ok {
						meta.ModDate = decodePDFDate(md)
					}
					return meta
				}
			}
		}
		return meta
	}

	// 从 /Info 对象提取
	if obj, ok := p.objects[infoID]; ok {
		meta.Title = decodePDFString(obj.dict["Title"])
		meta.Author = decodePDFString(obj.dict["Author"])
		meta.Subject = decodePDFString(obj.dict["Subject"])
		meta.Creator = decodePDFString(obj.dict["Creator"])
		meta.Producer = decodePDFString(obj.dict["Producer"])
		if cd, ok := obj.dict["CreationDate"]; ok {
			meta.CreationDate = decodePDFDate(cd)
		}
		if md, ok := obj.dict["ModDate"]; ok {
			meta.ModDate = decodePDFDate(md)
		}
	}

	return meta
}

// findTrailerRef 从 trailer 字典中查找指定键的间接引用
//
// trailer 字典格式:
//
//	trailer
//	<< /Size N /Root X 0 R /Info Y 0 R >>
//
// 返回引用的对象 ID，未找到返回 0。
func (p *pdfInternalParser) findTrailerRef(key string) int {
	// 从文件末尾查找 trailer
	tail := p.data
	if len(tail) > 4096 {
		tail = tail[len(tail)-4096:]
	}

	trailerIdx := bytes.LastIndex(tail, []byte("trailer"))
	if trailerIdx < 0 {
		return 0
	}

	trailerContent := string(tail[trailerIdx:])

	// 查找 /Key N 0 R 模式
	pattern := fmt.Sprintf(`/%s\s+(\d+)\s+\d+\s+R`, key)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(trailerContent)
	if len(match) >= 2 {
		id, err := strconv.Atoi(match[1])
		if err == nil {
			return id
		}
	}

	return 0
}

// extractPages 提取所有页面文本
//
// 解析流程:
//  1. 从 trailer 找到 /Root (Catalog)
//  2. 从 Catalog 找到 /Pages (页面树根)
//  3. 递归遍历页面树收集所有 /Page 对象
//  4. 提取每个 Page 的 /Contents 流并解析文本
func (p *pdfInternalParser) extractPages() []string {
	// 收集所有 Page 类型对象
	var pageObjs []pdfObject
	for _, obj := range p.objects {
		if typeVal, ok := obj.dict["Type"]; ok {
			if strings.Contains(typeVal, "Page") && !strings.Contains(typeVal, "Pages") {
				pageObjs = append(pageObjs, obj)
			}
		}
	}

	// 按对象 ID 排序，保证页面顺序
	sort.Slice(pageObjs, func(i, j int) bool {
		return pageObjs[i].id < pageObjs[j].id
	})

	// 如果没有找到 Page 对象，尝试按 Contents 流提取
	if len(pageObjs) == 0 {
		return p.extractPagesFromStreams()
	}

	var pages []string
	for _, page := range pageObjs {
		text := p.extractPageText(page)
		pages = append(pages, text)
	}

	return pages
}

// extractPageText 提取单个页面的文本内容
//
// 页面文本存储在 /Contents 引用的流对象中。
// /Contents 可以是单个间接引用（N 0 R）或数组 [N 0 R M 0 R ...]。
func (p *pdfInternalParser) extractPageText(page pdfObject) string {
	contentsRef, ok := page.dict["Contents"]
	if !ok {
		return ""
	}

	// 解析 /Contents 引用
	refs := p.parseRefs(contentsRef)
	if len(refs) == 0 {
		// 页面自身包含流数据
		if len(page.stream) > 0 {
			return p.extractTextFromStream(page)
		}
		return ""
	}

	var sb strings.Builder
	for _, ref := range refs {
		if obj, ok := p.objects[ref]; ok {
			text := p.extractTextFromStream(obj)
			if sb.Len() > 0 && text != "" {
				sb.WriteByte('\n')
			}
			sb.WriteString(text)
		}
	}

	return sb.String()
}

// parseRefs 解析间接引用或引用数组
//
// 支持格式:
//   - 单个引用: "N 0 R"
//   - 数组引用: "[N 0 R M 0 R ...]"
func (p *pdfInternalParser) parseRefs(s string) []int {
	re := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	matches := re.FindAllStringSubmatch(s, -1)

	var refs []int
	for _, m := range matches {
		if id, err := strconv.Atoi(m[1]); err == nil {
			refs = append(refs, id)
		}
	}
	return refs
}

// extractTextFromStream 从对象流中提取文本
//
// 处理步骤:
//  1. 获取流数据
//  2. 如果有 /Filter FlateDecode，使用 flate 解压
//  3. 解析解压后的内容流，提取 BT/ET 块中的文本
func (p *pdfInternalParser) extractTextFromStream(obj pdfObject) string {
	streamData := obj.stream
	if len(streamData) == 0 {
		return ""
	}

	// 检查压缩类型
	filter := obj.dict["Filter"]
	if strings.Contains(filter, "FlateDecode") {
		decompressed, err := decompressFlate(streamData)
		if err == nil {
			streamData = decompressed
		}
		// 解压失败则尝试原始数据
	}

	return parseContentStream(streamData)
}

// decompressFlate 使用 zlib/deflate 解压流数据
//
// PDF 的 FlateDecode 过滤器基于 zlib (RFC 1950) 格式，
// 包含 2 字节头和 4 字节 Adler-32 校验和。
// 先尝试 zlib 解压，失败则尝试 raw deflate。
func decompressFlate(data []byte) ([]byte, error) {
	// 注意: 不使用 TrimRight 处理二进制数据，因为压缩流的校验和字节
	// 可能恰好与 \r\n\t 等字符码位相同，TrimRight 会破坏数据完整性。
	// 流数据的 EOL 剥离已在 parseObjectAt 中完成。

	// 先尝试 zlib（标准 FlateDecode 格式）
	if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		var buf bytes.Buffer
		if _, copyErr := io.Copy(&buf, r); copyErr == nil || buf.Len() > 0 {
			r.Close()
			if buf.Len() > 0 {
				return buf.Bytes(), nil
			}
		}
		r.Close()
	}

	// 降级到 raw deflate
	reader := flate.NewReader(bytes.NewReader(data))
	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)
	reader.Close()
	if buf.Len() > 0 {
		return buf.Bytes(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("flate 解压失败: %w", err)
	}

	return nil, fmt.Errorf("flate 解压失败: 无输出数据")
}

// parseContentStream 解析 PDF 内容流中的文本操作符
//
// PDF 文本操作符定义在 BT (Begin Text) 和 ET (End Text) 块中。
// 常见的文本绘制操作符：
//   - Tj: 显示字符串，如 (Hello) Tj
//   - TJ: 显示字符串数组，如 [(H) 80 (ello)] TJ
//   - ': 移至下一行并显示字符串
//   - ": 设置字间距和字符间距后显示
//   - Td/TD: 移动文本位置（用于检测换行）
//   - Tm: 设置文本矩阵（用于检测换行）
//   - T*: 移至下一行
func parseContentStream(data []byte) string {
	var sb strings.Builder
	s := string(data)

	// 查找所有 BT...ET 块
	for {
		btIdx := strings.Index(s, "BT")
		if btIdx < 0 {
			break
		}
		etIdx := strings.Index(s[btIdx:], "ET")
		if etIdx < 0 {
			break
		}

		block := s[btIdx : btIdx+etIdx+2]
		text := extractTextFromBTBlock(block)
		if text != "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(text)
		}

		s = s[btIdx+etIdx+2:]
	}

	return strings.TrimSpace(sb.String())
}

// extractTextFromBTBlock 从 BT...ET 文本块中提取文本
//
// 逐行解析操作符，识别文本绘制命令和位置移动命令。
func extractTextFromBTBlock(block string) string {
	var sb strings.Builder
	lastY := 0.0
	hasPosition := false

	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "BT" || line == "ET" {
			continue
		}

		// Td/TD 操作符: 移动文本位置 (tx ty Td)
		// 当 y 坐标变化时插入换行
		if strings.HasSuffix(line, "Td") || strings.HasSuffix(line, "TD") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				if ty, err := strconv.ParseFloat(parts[len(parts)-3], 64); err == nil {
					if hasPosition && ty != 0 && ty != lastY {
						sb.WriteByte('\n')
					}
					lastY = ty
					hasPosition = true
				}
			}
			continue
		}

		// T* 操作符: 移至下一行
		if line == "T*" {
			sb.WriteByte('\n')
			continue
		}

		// Tm 操作符: 设置文本矩阵 (a b c d tx ty Tm)
		if strings.HasSuffix(line, "Tm") {
			parts := strings.Fields(line)
			if len(parts) >= 7 {
				if ty, err := strconv.ParseFloat(parts[len(parts)-2], 64); err == nil {
					if hasPosition && ty != lastY {
						sb.WriteByte('\n')
					}
					lastY = ty
					hasPosition = true
				}
			}
			continue
		}

		// Tj 操作符: (string) Tj
		if strings.HasSuffix(line, "Tj") {
			text := extractTjText(line)
			sb.WriteString(text)
			continue
		}

		// TJ 操作符: [(string1) num (string2)] TJ
		if strings.HasSuffix(line, "TJ") {
			text := extractTJText(line)
			sb.WriteString(text)
			continue
		}

		// ' 操作符: (string) '
		if strings.HasSuffix(line, "'") && strings.Contains(line, "(") {
			sb.WriteByte('\n')
			text := extractTjText(line)
			sb.WriteString(text)
			continue
		}

		// " 操作符: aw ac (string) "
		if strings.HasSuffix(line, "\"") && strings.Contains(line, "(") {
			sb.WriteByte('\n')
			text := extractTjText(line)
			sb.WriteString(text)
			continue
		}
	}

	return strings.TrimSpace(sb.String())
}

// extractTjText 从 Tj 操作中提取文本
//
// 格式: (text) Tj 或 <hex> Tj
func extractTjText(line string) string {
	// 提取括号字符串
	text := extractParenString(line)
	if text != "" {
		return text
	}
	// 提取 hex 字符串
	return extractHexInLine(line)
}

// extractTJText 从 TJ 数组操作中提取文本
//
// 格式: [(text1) -100 (text2) 50 (text3)] TJ
// 数字表示字间距调整（负数表示加宽）。
// 当间距超过阈值时插入空格。
func extractTJText(line string) string {
	// 查找数组 [...]
	arrStart := strings.Index(line, "[")
	arrEnd := strings.LastIndex(line, "]")
	if arrStart < 0 || arrEnd < 0 || arrEnd <= arrStart {
		return ""
	}

	arr := line[arrStart+1 : arrEnd]
	var sb strings.Builder

	i := 0
	for i < len(arr) {
		switch arr[i] {
		case '(':
			// 括号字符串
			end := findMatchingParen(arr, i)
			if end > i {
				text := decodeLiteralString(arr[i+1 : end])
				sb.WriteString(text)
				i = end + 1
			} else {
				i++
			}
		case '<':
			// hex 字符串
			end := strings.Index(arr[i:], ">")
			if end > 0 {
				hexStr := arr[i+1 : i+end]
				sb.WriteString(decodeHexString(hexStr))
				i = i + end + 1
			} else {
				i++
			}
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.':
			// 数字（字间距调整）
			numEnd := i + 1
			for numEnd < len(arr) && (arr[numEnd] == '.' || arr[numEnd] == '-' ||
				(arr[numEnd] >= '0' && arr[numEnd] <= '9')) {
				numEnd++
			}
			// 大的负数间距意味着单词间空格
			numStr := arr[i:numEnd]
			if val, err := strconv.ParseFloat(numStr, 64); err == nil && val < -200 {
				sb.WriteByte(' ')
			}
			i = numEnd
		default:
			i++
		}
	}

	return sb.String()
}

// findMatchingParen 查找匹配的右括号位置
//
// 处理括号嵌套和反斜杠转义。
func findMatchingParen(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		if i > start && s[i-1] == '\\' {
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// extractParenString 从行中提取第一个括号字符串
func extractParenString(line string) string {
	start := strings.Index(line, "(")
	if start < 0 {
		return ""
	}
	end := findMatchingParen(line, start)
	if end < 0 {
		return ""
	}
	return decodeLiteralString(line[start+1 : end])
}

// extractHexInLine 从行中提取第一个 hex 字符串
func extractHexInLine(line string) string {
	start := strings.Index(line, "<")
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start:], ">")
	if end < 0 {
		return ""
	}
	// 排除字典标记 <<
	if start+1 < len(line) && line[start+1] == '<' {
		return ""
	}
	return decodeHexString(line[start+1 : start+end])
}

// extractPagesFromStreams 从包含文本操作符的流中提取页面
//
// 当页面树结构无法解析时的备用方案：
// 扫描所有包含流数据的对象，提取其中的文本内容。
func (p *pdfInternalParser) extractPagesFromStreams() []string {
	var pages []string

	// 按对象 ID 排序遍历
	var ids []int
	for id := range p.objects {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		obj := p.objects[id]
		if len(obj.stream) == 0 {
			continue
		}

		text := p.extractTextFromStream(obj)
		if strings.TrimSpace(text) != "" {
			pages = append(pages, text)
		}
	}

	return pages
}

// extractFirstInt 从字节切片中提取第一个整数
func extractFirstInt(data []byte) int {
	s := strings.TrimSpace(string(data))
	var numStr strings.Builder
	started := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			numStr.WriteRune(c)
			started = true
		} else if started {
			break
		}
	}
	if numStr.Len() == 0 {
		return 0
	}
	n, err := strconv.Atoi(numStr.String())
	if err != nil {
		return 0
	}
	return n
}
