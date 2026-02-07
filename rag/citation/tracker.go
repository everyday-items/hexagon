package citation

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/rag"
)

// AutoTracker 自动引文追踪器
//
// 作为 RAG 管道的中间件自动追踪引文来源，无需手动配置。
// 它包装了 Retriever 和 Synthesizer，在检索和合成过程中
// 自动建立答案与源文档之间的双向映射关系。
//
// 功能：
//   - 自动在 RAG 管道中插入引文追踪
//   - 追踪每段回答文本对应的源文档
//   - 自动计算引文置信度（基于文本重叠度）
//   - 支持多种输出格式：行内引用、脚注、参考文献列表
//   - 线程安全，可并发使用
//
// 使用示例：
//
//	tracker := citation.NewAutoTracker(retriever, llmProvider)
//	result, err := tracker.QueryWithCitations(ctx, "用户问题")
//	// result.Content 包含自动添加的引用标记
//	// result.SourceMap 包含句子→来源文档的映射
type AutoTracker struct {
	// retriever 包装的检索器
	retriever rag.Retriever

	// llm LLM 提供者
	llm llm.Provider

	// engine 引用引擎
	engine *CitationEngine

	// config 追踪器配置
	config trackerConfig

	// history 追踪历史记录（用于审计和调试）
	history []TrackRecord
	mu      sync.RWMutex
}

// trackerConfig 追踪器配置
type trackerConfig struct {
	// topK 检索文档数
	topK int

	// minConfidence 最小引用置信度（低于此值的引用将被忽略）
	minConfidence float32

	// enableHistory 是否记录追踪历史
	enableHistory bool

	// style 引用样式
	style CitationStyle

	// autoVerify 是否自动验证引用准确性
	autoVerify bool
}

// TrackerOption 追踪器配置选项
type TrackerOption func(*trackerConfig)

// WithTrackerTopK 设置检索文档数
func WithTrackerTopK(k int) TrackerOption {
	return func(c *trackerConfig) {
		if k > 0 {
			c.topK = k
		}
	}
}

// WithMinConfidence 设置最小引用置信度
func WithMinConfidence(conf float32) TrackerOption {
	return func(c *trackerConfig) {
		if conf > 0 && conf <= 1.0 {
			c.minConfidence = conf
		}
	}
}

// WithHistory 启用追踪历史记录
func WithHistory(enabled bool) TrackerOption {
	return func(c *trackerConfig) {
		c.enableHistory = enabled
	}
}

// WithTrackerStyle 设置引用样式
func WithTrackerStyle(style CitationStyle) TrackerOption {
	return func(c *trackerConfig) {
		c.style = style
	}
}

// WithAutoVerify 启用自动验证引用准确性
func WithAutoVerify(enabled bool) TrackerOption {
	return func(c *trackerConfig) {
		c.autoVerify = enabled
	}
}

// TrackRecord 追踪记录
type TrackRecord struct {
	// Query 用户查询
	Query string `json:"query"`

	// SourceDocs 检索到的源文档
	SourceDocs []rag.Document `json:"source_docs"`

	// Citations 引用列表
	Citations []Citation `json:"citations"`

	// SourceMap 句子→来源映射
	SourceMap map[string][]string `json:"source_map"`

	// Confidence 整体引用置信度
	Confidence float32 `json:"confidence"`
}

// TrackedResponse 带自动追踪的响应
type TrackedResponse struct {
	// Content 带引用标记的回答
	Content string `json:"content"`

	// RawContent 原始回答（无引用标记）
	RawContent string `json:"raw_content"`

	// Citations 引用列表
	Citations []Citation `json:"citations"`

	// Sources 来源文档
	Sources []rag.Document `json:"sources"`

	// SourceMap 句子→来源文档 ID 映射
	SourceMap map[string][]string `json:"source_map"`

	// Bibliography 参考文献
	Bibliography string `json:"bibliography"`

	// Confidence 整体引用置信度 (0-1)
	Confidence float32 `json:"confidence"`

	// UnverifiedClaims 无法验证来源的声明
	UnverifiedClaims []string `json:"unverified_claims,omitempty"`
}

// NewAutoTracker 创建自动引文追踪器
func NewAutoTracker(retriever rag.Retriever, provider llm.Provider, opts ...TrackerOption) *AutoTracker {
	config := trackerConfig{
		topK:          5,
		minConfidence: 0.3,
		enableHistory: false,
		style:         StyleNumeric,
		autoVerify:    false,
	}

	for _, opt := range opts {
		opt(&config)
	}

	engine := New(
		retriever,
		provider,
		WithCitationStyle(config.style),
		WithTopK(config.topK),
	)

	return &AutoTracker{
		retriever: retriever,
		llm:       provider,
		engine:    engine,
		config:    config,
		history:   make([]TrackRecord, 0),
	}
}

// QueryWithCitations 执行带自动引文追踪的查询
//
// 自动完成以下步骤：
//  1. 检索相关文档
//  2. 生成带引用的回答
//  3. 建立句子→来源的映射关系
//  4. 计算引用置信度
//  5. 可选：验证引用准确性
func (t *AutoTracker) QueryWithCitations(ctx context.Context, query string) (*TrackedResponse, error) {
	// 1. 使用引用引擎生成带引用的回答
	cited, err := t.engine.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("引用查询失败: %w", err)
	}

	// 2. 建立句子→来源映射
	sourceMap := t.buildSourceMap(cited.Content, cited.Citations, cited.Sources)

	// 3. 计算引用置信度
	confidence := t.calculateConfidence(cited.Citations, cited.Sources)

	// 4. 过滤低置信度引用
	filteredCitations := t.filterLowConfidence(cited.Citations, cited.Sources)

	// 5. 识别未验证的声明
	var unverified []string
	if t.config.autoVerify {
		unverified = t.findUnverifiedClaims(cited.Content, cited.Citations)
	}

	response := &TrackedResponse{
		Content:          cited.Content,
		RawContent:       cited.RawContent,
		Citations:        filteredCitations,
		Sources:          cited.Sources,
		SourceMap:        sourceMap,
		Bibliography:     cited.Bibliography,
		Confidence:       confidence,
		UnverifiedClaims: unverified,
	}

	// 6. 记录追踪历史
	if t.config.enableHistory {
		t.mu.Lock()
		t.history = append(t.history, TrackRecord{
			Query:      query,
			SourceDocs: cited.Sources,
			Citations:  filteredCitations,
			SourceMap:  sourceMap,
			Confidence: confidence,
		})
		t.mu.Unlock()
	}

	return response, nil
}

// Retrieve 作为 Retriever 的包装，在检索时自动记录来源
//
// 实现 rag.Retriever 接口，可以直接替换原有的 Retriever。
func (t *AutoTracker) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	return t.retriever.Retrieve(ctx, query, opts...)
}

// History 返回追踪历史记录
func (t *AutoTracker) History() []TrackRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]TrackRecord, len(t.history))
	copy(result, t.history)
	return result
}

// ClearHistory 清除追踪历史
func (t *AutoTracker) ClearHistory() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history = t.history[:0]
}

// buildSourceMap 构建句子→来源文档映射
//
// 将回答按句子分割，然后根据引用标记找到每个句子对应的来源文档。
func (t *AutoTracker) buildSourceMap(content string, citations []Citation, sources []rag.Document) map[string][]string {
	sourceMap := make(map[string][]string)

	// 按句子分割
	sentences := splitSentences(content)

	// 为每个句子查找对应的引用
	for _, sentence := range sentences {
		clean := strings.TrimSpace(sentence)
		if clean == "" {
			continue
		}

		var sourceIDs []string
		for _, citation := range citations {
			// 检查句子是否包含该引用标记
			if strings.Contains(sentence, citation.Marker) {
				sourceIDs = append(sourceIDs, citation.SourceID)
			}
		}

		if len(sourceIDs) > 0 {
			// 清理句子中的引用标记
			cleanSentence := t.engine.stripCitations(clean)
			if cleanSentence != "" {
				sourceMap[cleanSentence] = sourceIDs
			}
		}
	}

	return sourceMap
}

// calculateConfidence 计算整体引用置信度
//
// 基于以下因素：
//   - 引用覆盖率（有多少句子被引用）
//   - 引用与来源的文本重叠度
func (t *AutoTracker) calculateConfidence(citations []Citation, sources []rag.Document) float32 {
	if len(citations) == 0 || len(sources) == 0 {
		return 0
	}

	var totalOverlap float32
	validCount := 0

	for _, citation := range citations {
		// 查找对应的源文档
		var sourceContent string
		for _, src := range sources {
			if src.ID == citation.SourceID {
				sourceContent = src.Content
				break
			}
		}

		if sourceContent == "" || citation.Text == "" {
			continue
		}

		// 计算文本重叠度
		overlap := computeTextOverlap(citation.Text, sourceContent)
		totalOverlap += overlap
		validCount++
	}

	if validCount == 0 {
		return 0
	}

	return totalOverlap / float32(validCount)
}

// filterLowConfidence 过滤低置信度引用
func (t *AutoTracker) filterLowConfidence(citations []Citation, sources []rag.Document) []Citation {
	if t.config.minConfidence <= 0 {
		return citations
	}

	var filtered []Citation
	for _, citation := range citations {
		var sourceContent string
		for _, src := range sources {
			if src.ID == citation.SourceID {
				sourceContent = src.Content
				break
			}
		}

		if sourceContent == "" {
			// 无法验证，保留
			filtered = append(filtered, citation)
			continue
		}

		overlap := computeTextOverlap(citation.Text, sourceContent)
		if overlap >= t.config.minConfidence {
			filtered = append(filtered, citation)
		}
	}

	return filtered
}

// findUnverifiedClaims 识别未被任何引用支持的声明
func (t *AutoTracker) findUnverifiedClaims(content string, citations []Citation) []string {
	sentences := splitSentences(content)
	var unverified []string

	for _, sentence := range sentences {
		clean := strings.TrimSpace(sentence)
		if clean == "" {
			continue
		}

		// 检查句子是否包含任何引用标记
		hasCitation := false
		for _, citation := range citations {
			if strings.Contains(sentence, citation.Marker) {
				hasCitation = true
				break
			}
		}

		if !hasCitation {
			// 忽略太短的句子（可能是标题、标点等）
			stripped := t.engine.stripCitations(clean)
			if len([]rune(stripped)) > 10 {
				unverified = append(unverified, stripped)
			}
		}
	}

	return unverified
}

// ============== 辅助函数 ==============

// splitSentences 按句子分割文本
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)

		// 句子结束标记
		if r == '。' || r == '.' || r == '！' || r == '?' || r == '\n' {
			s := current.String()
			if strings.TrimSpace(s) != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
			continue
		}

		// 分号也可以作为分割点
		if r == '；' || r == ';' {
			s := current.String()
			if strings.TrimSpace(s) != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
			continue
		}

		_ = i
	}

	// 最后一个片段
	if current.Len() > 0 {
		s := current.String()
		if strings.TrimSpace(s) != "" {
			sentences = append(sentences, s)
		}
	}

	return sentences
}

// computeTextOverlap 计算两段文本的重叠度 (0-1)
//
// 使用词语级别的 Jaccard 相似度。
func computeTextOverlap(text, source string) float32 {
	if text == "" || source == "" {
		return 0
	}

	textWords := tokenize(text)
	sourceWords := tokenize(source)

	if len(textWords) == 0 || len(sourceWords) == 0 {
		return 0
	}

	// 构建源文本词集
	sourceSet := make(map[string]bool, len(sourceWords))
	for _, w := range sourceWords {
		sourceSet[w] = true
	}

	// 计算重叠词数
	overlap := 0
	for _, w := range textWords {
		if sourceSet[w] {
			overlap++
		}
	}

	// Jaccard 相似度的简化版本：重叠数 / 文本词数
	return float32(overlap) / float32(len(textWords))
}

// tokenize 简单分词（按空格和标点分割）
func tokenize(text string) []string {
	// 将标点替换为空格
	replacer := strings.NewReplacer(
		"，", " ", "。", " ", "？", " ", "！", " ",
		"；", " ", "：", " ", "（", " ", "）", " ",
		",", " ", ".", " ", "?", " ", "!", " ",
		";", " ", ":", " ", "(", " ", ")", " ",
		"[", " ", "]", " ", "\n", " ", "\t", " ",
	)
	cleaned := replacer.Replace(text)

	words := strings.Fields(cleaned)

	// 过滤太短的词
	var result []string
	for _, w := range words {
		if len([]rune(w)) >= 2 {
			result = append(result, w)
		}
	}

	return result
}
