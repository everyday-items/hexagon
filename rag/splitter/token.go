// Package splitter 提供 RAG 系统的文档分割器
//
// token.go 实现 Token 级分割器：
//   - TokenSplitter: 按模型 Token 数而非字符数分割
//   - Tokenizer: 分词器接口，支持 tiktoken/sentencepiece 等
//   - SimpleTokenizer: 基于空格/标点的简单分词器
//   - TiktokenTokenizer: 对接 OpenAI tiktoken 的分词器
//
// 对标 LangChain/LlamaIndex 的 Token-based Splitting。
//
// 使用示例：
//
//	// 使用简单分词器（无外部依赖）
//	splitter := NewTokenSplitter(
//	    WithTokenChunkSize(512),
//	    WithTokenOverlap(50),
//	    WithTokenizer(NewSimpleTokenizer()),
//	)
//
//	// 使用 tiktoken（精确按 GPT 模型 token 分割）
//	splitter := NewTokenSplitter(
//	    WithTokenChunkSize(512),
//	    WithTokenizer(NewTiktokenTokenizer("gpt-4")),
//	)
//
//	docs, err := splitter.Split(ctx, inputDocs)
package splitter

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== Tokenizer 接口 ==============

// Tokenizer 分词器接口
// 将文本转换为 token 序列，用于精确控制分块大小
type Tokenizer interface {
	// Encode 将文本编码为 token ID 序列
	Encode(text string) []int

	// Decode 将 token ID 序列解码为文本
	Decode(tokens []int) string

	// CountTokens 计算文本的 token 数量
	CountTokens(text string) int

	// Name 返回分词器名称
	Name() string
}

// ============== 简单分词器 ==============

// SimpleTokenizer 基于空格和标点的简单分词器
// 不需要外部依赖，适合中英文混合文本
// 估算规则：英文约 1 词 = 1.3 token，中文约 1 字 = 1.5 token
type SimpleTokenizer struct {
	// avgTokenLength 平均每个 token 的字符数
	avgTokenLength float64
}

// SimpleTokenizerOption 简单分词器选项
type SimpleTokenizerOption func(*SimpleTokenizer)

// WithAvgTokenLength 设置平均 token 长度
func WithAvgTokenLength(length float64) SimpleTokenizerOption {
	return func(t *SimpleTokenizer) {
		t.avgTokenLength = length
	}
}

// NewSimpleTokenizer 创建简单分词器
func NewSimpleTokenizer(opts ...SimpleTokenizerOption) *SimpleTokenizer {
	t := &SimpleTokenizer{
		avgTokenLength: 4.0, // 英文平均约 4 字符一个 token
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Encode 简单编码（分词为 token）
func (t *SimpleTokenizer) Encode(text string) []int {
	tokens := t.tokenize(text)
	ids := make([]int, len(tokens))
	for i := range tokens {
		ids[i] = i // 简单 ID 分配
	}
	return ids
}

// Decode 简单解码
func (t *SimpleTokenizer) Decode(tokens []int) string {
	// 简单分词器不支持精确解码
	return ""
}

// CountTokens 计算 token 数
func (t *SimpleTokenizer) CountTokens(text string) int {
	return len(t.tokenize(text))
}

// Name 返回名称
func (t *SimpleTokenizer) Name() string {
	return "simple"
}

// tokenize 将文本分词
func (t *SimpleTokenizer) tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if current.Len() > 0 {
				word := current.String()
				// 估算单词的 token 数
				runeCount := utf8.RuneCountInString(word)
				if float64(runeCount) > t.avgTokenLength*2 {
					// 长词拆分
					runes := []rune(word)
					for i := 0; i < len(runes); i += int(t.avgTokenLength) {
						end := i + int(t.avgTokenLength)
						if end > len(runes) {
							end = len(runes)
						}
						tokens = append(tokens, string(runes[i:end]))
					}
				} else {
					tokens = append(tokens, word)
				}
				current.Reset()
			}
			// 标点符号作为单独 token
			if !unicode.IsSpace(r) {
				tokens = append(tokens, string(r))
			}
		} else if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			// CJK 字符：每个字作为一个 token
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// ============== Tiktoken 分词器 ==============

// tokenRatios 按字符类型的 token 比率
// 不同的 BPE 编码对不同字符类型有不同的压缩率
type tokenRatios struct {
	// asciiWord 英文单词中每个字符的 token 比率（如 "hello" → ~1.3 token）
	asciiWord float64
	// cjk CJK 字符（中日韩）每个字符的 token 比率
	cjk float64
	// digit 数字每个字符的 token 比率
	digit float64
	// punctuation 标点符号每个字符的 token 比率
	punctuation float64
	// whitespace 空白字符每个字符的 token 比率
	whitespace float64
}

// cl100kRatios cl100k_base 编码的 token 比率（GPT-4, GPT-3.5-turbo 系列）
// 基于实际 tiktoken 统计的近似值
var cl100kRatios = tokenRatios{
	asciiWord:   0.25,  // 平均 4 字符 = 1 token
	cjk:         0.5,   // 平均 2 字符 = 1 token（CJK 通常 1-2 字符/token）
	digit:       0.33,  // 平均 3 位数字 = 1 token
	punctuation: 1.0,   // 每个标点约 1 token
	whitespace:  0.25,  // 空白通常与前后词合并
}

// o200kRatios o200k_base 编码的 token 比率（GPT-4o 系列）
// 更大的词表带来更好的压缩率
var o200kRatios = tokenRatios{
	asciiWord:   0.22,  // 略优于 cl100k
	cjk:         0.45,  // 略优于 cl100k
	digit:       0.28,
	punctuation: 0.9,
	whitespace:  0.2,
}

// modelToRatios 模型名到 token 比率的映射
var modelToRatios = map[string]*tokenRatios{
	// cl100k_base 模型
	"gpt-4":              &cl100kRatios,
	"gpt-4-turbo":        &cl100kRatios,
	"gpt-3.5-turbo":      &cl100kRatios,
	"text-embedding-ada-002": &cl100kRatios,
	// o200k_base 模型
	"gpt-4o":             &o200kRatios,
	"gpt-4o-mini":        &o200kRatios,
}

// TiktokenTokenizer 基于 tiktoken 的分词器
// 使用按字符类型差异化的 token 比率进行估算。
// 无实际 BPE 词表，Encode 降级为 fallback 编码。
// 如需精确分词，建议集成 Go 原生 tiktoken 库。
type TiktokenTokenizer struct {
	model    string
	ratios   *tokenRatios
	fallback *SimpleTokenizer
}

// NewTiktokenTokenizer 创建 tiktoken 分词器
// 根据模型名自动选择合适的 token 比率
func NewTiktokenTokenizer(model string) *TiktokenTokenizer {
	ratios := &cl100kRatios // 默认 cl100k
	// 按前缀匹配模型
	for prefix, r := range modelToRatios {
		if strings.HasPrefix(model, prefix) {
			ratios = r
			break
		}
	}

	return &TiktokenTokenizer{
		model:    model,
		ratios:   ratios,
		fallback: NewSimpleTokenizer(),
	}
}

// Encode 编码（降级为 fallback，无实际 BPE 词表）
func (t *TiktokenTokenizer) Encode(text string) []int {
	return t.fallback.Encode(text)
}

// Decode 解码
func (t *TiktokenTokenizer) Decode(tokens []int) string {
	return t.fallback.Decode(tokens)
}

// CountTokens 按字符类型差异化估算 token 数
// 逐字符分类（ASCII 词、CJK、数字、标点、空白），乘以对应比率
func (t *TiktokenTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	var asciiChars, cjkChars, digitChars, punctChars, spaceChars int

	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			spaceChars++
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			cjkChars++
		case unicode.IsDigit(r):
			digitChars++
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			punctChars++
		default:
			asciiChars++
		}
	}

	total := float64(asciiChars)*t.ratios.asciiWord +
		float64(cjkChars)*t.ratios.cjk +
		float64(digitChars)*t.ratios.digit +
		float64(punctChars)*t.ratios.punctuation +
		float64(spaceChars)*t.ratios.whitespace

	// 至少 1 个 token（非空文本）
	result := int(math.Ceil(total))
	if result < 1 {
		result = 1
	}
	return result
}

// Name 返回名称
func (t *TiktokenTokenizer) Name() string {
	return "tiktoken-" + t.model
}

// 确保 TiktokenTokenizer 实现 Tokenizer 接口
var _ Tokenizer = (*TiktokenTokenizer)(nil)

// ============== Token 级分割器 ==============

// TokenSplitter 基于 Token 的文档分割器
// 使用分词器精确按 token 数量分割，而非按字符数
type TokenSplitter struct {
	tokenizer    Tokenizer
	chunkSize    int    // 每块最大 token 数
	chunkOverlap int    // 块间重叠 token 数
	separator    string // 首选分割点
}

// TokenSplitterOption TokenSplitter 选项
type TokenSplitterOption func(*TokenSplitter)

// WithTokenChunkSize 设置分块大小（token 数）
func WithTokenChunkSize(size int) TokenSplitterOption {
	return func(s *TokenSplitter) {
		s.chunkSize = size
	}
}

// WithTokenOverlap 设置重叠大小（token 数）
func WithTokenOverlap(overlap int) TokenSplitterOption {
	return func(s *TokenSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithTokenizer 设置分词器
func WithTokenizerOpt(tokenizer Tokenizer) TokenSplitterOption {
	return func(s *TokenSplitter) {
		s.tokenizer = tokenizer
	}
}

// WithTokenSeparator 设置首选分割点
func WithTokenSeparator(sep string) TokenSplitterOption {
	return func(s *TokenSplitter) {
		s.separator = sep
	}
}

// NewTokenSplitter 创建 Token 级分割器
func NewTokenSplitter(opts ...TokenSplitterOption) *TokenSplitter {
	s := &TokenSplitter{
		tokenizer:    NewSimpleTokenizer(),
		chunkSize:    512,
		chunkOverlap: 50,
		separator:    "\n",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 按 Token 数分割文档
func (s *TokenSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks := s.splitText(doc.Content)
		for i, chunk := range chunks {
			metadata := make(map[string]any)
			for k, v := range doc.Metadata {
				metadata[k] = v
			}
			metadata["chunk_index"] = i
			metadata["chunk_tokens"] = s.tokenizer.CountTokens(chunk)
			metadata["tokenizer"] = s.tokenizer.Name()
			metadata["parent_id"] = doc.ID

			result = append(result, rag.Document{
				ID:        util.GenerateID("token-chunk"),
				Content:   chunk,
				Metadata:  metadata,
				Source:    doc.Source,
				CreatedAt: time.Now(),
			})
		}
	}

	return result, nil
}

// splitText 按 token 分割文本
func (s *TokenSplitter) splitText(text string) []string {
	// 先按分隔符分段
	segments := strings.Split(text, s.separator)
	if len(segments) == 1 && s.separator != " " {
		// 如果没有分隔符，尝试按空格分
		segments = strings.Fields(text)
	}

	var chunks []string
	var currentChunk strings.Builder
	currentTokens := 0

	for _, segment := range segments {
		segTokens := s.tokenizer.CountTokens(segment)

		// 单段超过 chunk 大小，强制切割
		if segTokens > s.chunkSize {
			// 先保存当前块
			if currentChunk.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
				currentChunk.Reset()
				currentTokens = 0
			}

			// 按字符切割大段
			runes := []rune(segment)
			start := 0
			for start < len(runes) {
				// 找到大约 chunkSize 个 token 对应的字符位置
				end := start + s.chunkSize*4 // 估算：1 token ≈ 4 字符
				if end > len(runes) {
					end = len(runes)
				}

				// 精确调整
				chunk := string(runes[start:end])
				for s.tokenizer.CountTokens(chunk) > s.chunkSize && end > start+1 {
					end--
					chunk = string(runes[start:end])
				}

				chunks = append(chunks, strings.TrimSpace(chunk))

				// 计算重叠
				overlapChars := s.chunkOverlap * 4
				start = end - overlapChars
				if start < 0 || start <= end-len(runes) {
					start = end
				}
			}
			continue
		}

		// 添加段到当前块
		if currentTokens+segTokens > s.chunkSize && currentChunk.Len() > 0 {
			// 当前块已满，保存并开始新块（带重叠）
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// 创建重叠内容
			overlapText := s.getOverlap(currentChunk.String())
			currentChunk.Reset()
			if overlapText != "" {
				currentChunk.WriteString(overlapText)
				currentTokens = s.tokenizer.CountTokens(overlapText)
			} else {
				currentTokens = 0
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(s.separator)
			currentTokens++ // 分隔符大约 1 个 token
		}
		currentChunk.WriteString(segment)
		currentTokens += segTokens
	}

	// 最后一块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// getOverlap 获取重叠部分
func (s *TokenSplitter) getOverlap(text string) string {
	if s.chunkOverlap <= 0 {
		return ""
	}

	// 从文本末尾取 chunkOverlap 个 token
	runes := []rune(text)
	// 估算重叠字符数
	overlapChars := s.chunkOverlap * 4
	if overlapChars >= len(runes) {
		return text
	}

	overlap := string(runes[len(runes)-overlapChars:])

	// 尝试在单词/句子边界切割
	if idx := strings.LastIndex(overlap, s.separator); idx > 0 {
		overlap = overlap[idx+len(s.separator):]
	}

	return overlap
}

// Name 返回分割器名称
func (s *TokenSplitter) Name() string {
	return "token_splitter"
}
