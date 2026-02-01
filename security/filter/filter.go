// Package filter 提供 Hexagon AI Agent 框架的内容过滤
//
// 支持敏感词过滤、有害内容检测、成人内容过滤等功能。
package filter

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// ContentFilter 内容过滤器接口
type ContentFilter interface {
	// Name 过滤器名称
	Name() string

	// Filter 过滤内容
	Filter(ctx context.Context, content string) (*FilterResult, error)

	// FilterBatch 批量过滤
	FilterBatch(ctx context.Context, contents []string) ([]*FilterResult, error)
}

// FilterResult 过滤结果
type FilterResult struct {
	// Original 原始内容
	Original string `json:"original"`

	// Filtered 过滤后的内容
	Filtered string `json:"filtered"`

	// Passed 是否通过
	Passed bool `json:"passed"`

	// Score 风险分数（0-1）
	Score float64 `json:"score"`

	// Category 内容分类
	Category ContentCategory `json:"category"`

	// Findings 发现的问题
	Findings []Finding `json:"findings,omitempty"`

	// Action 建议的处理动作
	Action FilterAction `json:"action"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Finding 发现的问题
type Finding struct {
	// Type 问题类型
	Type FindingType `json:"type"`

	// Content 问题内容
	Content string `json:"content"`

	// Position 位置
	Position int `json:"position"`

	// Length 长度
	Length int `json:"length"`

	// Severity 严重程度
	Severity Severity `json:"severity"`

	// Category 类别
	Category string `json:"category"`
}

// ContentCategory 内容类别
type ContentCategory string

const (
	CategorySafe       ContentCategory = "safe"
	CategorySensitive  ContentCategory = "sensitive"
	CategoryHarmful    ContentCategory = "harmful"
	CategoryAdult      ContentCategory = "adult"
	CategoryViolence   ContentCategory = "violence"
	CategoryHate       ContentCategory = "hate"
	CategorySpam       ContentCategory = "spam"
	CategoryScam       ContentCategory = "scam"
	CategoryIllegal    ContentCategory = "illegal"
)

// FindingType 发现类型
type FindingType string

const (
	FindingSensitiveWord FindingType = "sensitive_word"
	FindingToxicity      FindingType = "toxicity"
	FindingAdultContent  FindingType = "adult_content"
	FindingViolence      FindingType = "violence"
	FindingHateSpeech    FindingType = "hate_speech"
	FindingSpam          FindingType = "spam"
	FindingScam          FindingType = "scam"
	FindingPersonalInfo  FindingType = "personal_info"
	FindingMalware       FindingType = "malware"
)

// Severity 严重程度
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// FilterAction 过滤动作
type FilterAction string

const (
	ActionAllow   FilterAction = "allow"
	ActionWarn    FilterAction = "warn"
	ActionRedact  FilterAction = "redact"
	ActionBlock   FilterAction = "block"
	ActionReview  FilterAction = "review"
)

// FilterConfig 过滤器配置
type FilterConfig struct {
	// Enabled 是否启用
	Enabled bool

	// Threshold 阈值
	Threshold float64

	// Action 默认动作
	Action FilterAction

	// Categories 要检测的类别
	Categories []ContentCategory

	// Allowlist 白名单
	Allowlist []string

	// CustomPatterns 自定义模式
	CustomPatterns []string
}

// DefaultFilterConfig 默认配置
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Action:     ActionWarn,
		Categories: []ContentCategory{CategoryHarmful, CategoryAdult, CategoryHate},
	}
}

// ============== Sensitive Word Filter ==============

// SensitiveWordFilter 敏感词过滤器
type SensitiveWordFilter struct {
	// Config 配置
	config FilterConfig

	// Words 敏感词列表
	words map[string]SensitiveWord

	// Trie AC 自动机
	trie *ACTrie

	// Categories 词汇分类
	categories map[string][]string

	mu sync.RWMutex
}

// SensitiveWord 敏感词
type SensitiveWord struct {
	Word     string   `json:"word"`
	Category string   `json:"category"`
	Severity Severity `json:"severity"`
	Action   FilterAction `json:"action"`
}

// NewSensitiveWordFilter 创建敏感词过滤器
func NewSensitiveWordFilter(opts ...FilterOption) *SensitiveWordFilter {
	f := &SensitiveWordFilter{
		config:     DefaultFilterConfig(),
		words:      make(map[string]SensitiveWord),
		categories: make(map[string][]string),
	}

	for _, opt := range opts {
		opt(&f.config)
	}

	// 初始化默认敏感词
	f.initDefaultWords()

	// 构建 Trie
	f.buildTrie()

	return f
}

// FilterOption 过滤选项
type FilterOption func(*FilterConfig)

// WithFilterThreshold 设置阈值
func WithFilterThreshold(threshold float64) FilterOption {
	return func(c *FilterConfig) {
		c.Threshold = threshold
	}
}

// WithFilterAction 设置默认动作
func WithFilterAction(action FilterAction) FilterOption {
	return func(c *FilterConfig) {
		c.Action = action
	}
}

// WithFilterCategories 设置检测类别
func WithFilterCategories(categories ...ContentCategory) FilterOption {
	return func(c *FilterConfig) {
		c.Categories = categories
	}
}

// Name 返回过滤器名称
func (f *SensitiveWordFilter) Name() string {
	return "sensitive_word_filter"
}

// AddWord 添加敏感词
func (f *SensitiveWordFilter) AddWord(word, category string, severity Severity) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.words[strings.ToLower(word)] = SensitiveWord{
		Word:     word,
		Category: category,
		Severity: severity,
		Action:   f.config.Action,
	}
	f.categories[category] = append(f.categories[category], word)
}

// AddWords 批量添加敏感词
func (f *SensitiveWordFilter) AddWords(words []string, category string, severity Severity) {
	for _, word := range words {
		f.AddWord(word, category, severity)
	}
	f.buildTrie()
}

// RemoveWord 移除敏感词
func (f *SensitiveWordFilter) RemoveWord(word string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.words, strings.ToLower(word))
}

// Filter 过滤内容
func (f *SensitiveWordFilter) Filter(ctx context.Context, content string) (*FilterResult, error) {
	result := &FilterResult{
		Original: content,
		Filtered: content,
		Passed:   true,
		Score:    0,
		Category: CategorySafe,
		Findings: make([]Finding, 0),
		Action:   ActionAllow,
		Metadata: make(map[string]any),
	}

	if !f.config.Enabled {
		return result, nil
	}

	// 检查白名单
	for _, allowed := range f.config.Allowlist {
		if strings.Contains(content, allowed) {
			return result, nil
		}
	}

	// 使用 Trie 查找敏感词
	lowerContent := strings.ToLower(content)
	matches := f.trie.Match(lowerContent)

	if len(matches) == 0 {
		return result, nil
	}

	// 处理匹配结果
	maxSeverity := SeverityLow
	filtered := content

	for _, match := range matches {
		f.mu.RLock()
		sw, ok := f.words[match.Word]
		f.mu.RUnlock()

		if !ok {
			continue
		}

		finding := Finding{
			Type:     FindingSensitiveWord,
			Content:  match.Word,
			Position: match.Position,
			Length:   len(match.Word),
			Severity: sw.Severity,
			Category: sw.Category,
		}
		result.Findings = append(result.Findings, finding)

		// 更新最高严重程度
		if severityLevel(sw.Severity) > severityLevel(maxSeverity) {
			maxSeverity = sw.Severity
		}

		// 脱敏处理
		if f.config.Action == ActionRedact {
			filtered = redactWord(filtered, match.Word, match.Position)
		}
	}

	// 计算分数
	result.Score = float64(len(matches)) / float64(len(strings.Fields(content))+1)
	if result.Score > 1 {
		result.Score = 1
	}

	// 确定动作
	if result.Score >= f.config.Threshold {
		result.Passed = false
		result.Action = f.config.Action
		result.Category = CategorySensitive
	}

	result.Filtered = filtered
	result.Metadata["match_count"] = len(matches)
	result.Metadata["max_severity"] = maxSeverity

	return result, nil
}

// FilterBatch 批量过滤
func (f *SensitiveWordFilter) FilterBatch(ctx context.Context, contents []string) ([]*FilterResult, error) {
	results := make([]*FilterResult, len(contents))
	for i, content := range contents {
		result, err := f.Filter(ctx, content)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// initDefaultWords 初始化默认敏感词
func (f *SensitiveWordFilter) initDefaultWords() {
	// 这里只添加一些示例，实际应用中应从配置文件加载
	// 暴力相关
	violenceWords := []string{"kill", "murder", "attack", "bomb", "terrorist"}
	for _, w := range violenceWords {
		f.words[w] = SensitiveWord{Word: w, Category: "violence", Severity: SeverityHigh}
	}

	// 仇恨言论相关
	hateWords := []string{"racist", "discrimination"}
	for _, w := range hateWords {
		f.words[w] = SensitiveWord{Word: w, Category: "hate", Severity: SeverityHigh}
	}
}

// buildTrie 构建 Trie
func (f *SensitiveWordFilter) buildTrie() {
	f.mu.Lock()
	defer f.mu.Unlock()

	words := make([]string, 0, len(f.words))
	for word := range f.words {
		words = append(words, word)
	}

	f.trie = NewACTrie(words)
}

// ============== AC Trie ==============

// ACTrie Aho-Corasick 自动机
type ACTrie struct {
	root *trieNode
}

type trieNode struct {
	children map[rune]*trieNode
	fail     *trieNode
	output   []string
}

// TrieMatch 匹配结果
type TrieMatch struct {
	Word     string
	Position int
}

// NewACTrie 创建 AC 自动机
func NewACTrie(words []string) *ACTrie {
	trie := &ACTrie{
		root: &trieNode{
			children: make(map[rune]*trieNode),
			output:   make([]string, 0),
		},
	}

	// 构建 Trie 树
	for _, word := range words {
		trie.insert(word)
	}

	// 构建失败指针
	trie.buildFailure()

	return trie
}

// insert 插入词
func (t *ACTrie) insert(word string) {
	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			node.children[ch] = &trieNode{
				children: make(map[rune]*trieNode),
				output:   make([]string, 0),
			}
		}
		node = node.children[ch]
	}
	node.output = append(node.output, word)
}

// buildFailure 构建失败指针
func (t *ACTrie) buildFailure() {
	queue := make([]*trieNode, 0)

	// 第一层的失败指针指向根
	for _, child := range t.root.children {
		child.fail = t.root
		queue = append(queue, child)
	}

	// BFS 构建其他层的失败指针
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for ch, child := range current.children {
			queue = append(queue, child)

			fail := current.fail
			for fail != nil && fail.children[ch] == nil {
				fail = fail.fail
			}

			if fail == nil {
				child.fail = t.root
			} else {
				child.fail = fail.children[ch]
			}

			// 合并输出
			if child.fail != nil {
				child.output = append(child.output, child.fail.output...)
			}
		}
	}
}

// Match 匹配
func (t *ACTrie) Match(text string) []TrieMatch {
	matches := make([]TrieMatch, 0)
	node := t.root

	for i, ch := range text {
		// 失败跳转
		for node != t.root && node.children[ch] == nil {
			node = node.fail
		}

		if node.children[ch] != nil {
			node = node.children[ch]
		}

		// 输出匹配
		for _, word := range node.output {
			matches = append(matches, TrieMatch{
				Word:     word,
				Position: i - len(word) + 1,
			})
		}
	}

	return matches
}

// ============== Helper Functions ==============

func severityLevel(s Severity) int {
	switch s {
	case SeverityLow:
		return 1
	case SeverityMedium:
		return 2
	case SeverityHigh:
		return 3
	case SeverityCritical:
		return 4
	default:
		return 0
	}
}

func redactWord(content, word string, position int) string {
	// 用 * 替换敏感词
	replacement := strings.Repeat("*", len(word))
	return content[:position] + replacement + content[position+len(word):]
}

// ============== Toxicity Filter ==============

// ToxicityFilter 有害内容过滤器
type ToxicityFilter struct {
	config FilterConfig

	// 有害模式
	patterns []*regexp.Regexp

	// 分类器（可扩展为 ML 模型）
	classifier ToxicityClassifier
}

// ToxicityClassifier 有害内容分类器接口
type ToxicityClassifier interface {
	Classify(ctx context.Context, content string) (*ToxicityScore, error)
}

// ToxicityScore 有害分数
type ToxicityScore struct {
	Overall      float64 `json:"overall"`
	Hate         float64 `json:"hate"`
	Violence     float64 `json:"violence"`
	Sexual       float64 `json:"sexual"`
	SelfHarm     float64 `json:"self_harm"`
	Harassment   float64 `json:"harassment"`
	Threatening  float64 `json:"threatening"`
}

// NewToxicityFilter 创建有害内容过滤器
func NewToxicityFilter(opts ...FilterOption) *ToxicityFilter {
	f := &ToxicityFilter{
		config:   DefaultFilterConfig(),
		patterns: make([]*regexp.Regexp, 0),
	}

	for _, opt := range opts {
		opt(&f.config)
	}

	// 初始化默认模式
	f.initDefaultPatterns()

	return f
}

// SetClassifier 设置分类器
func (f *ToxicityFilter) SetClassifier(classifier ToxicityClassifier) {
	f.classifier = classifier
}

// Name 返回过滤器名称
func (f *ToxicityFilter) Name() string {
	return "toxicity_filter"
}

// Filter 过滤内容
func (f *ToxicityFilter) Filter(ctx context.Context, content string) (*FilterResult, error) {
	result := &FilterResult{
		Original: content,
		Filtered: content,
		Passed:   true,
		Score:    0,
		Category: CategorySafe,
		Findings: make([]Finding, 0),
		Action:   ActionAllow,
		Metadata: make(map[string]any),
	}

	if !f.config.Enabled {
		return result, nil
	}

	// 使用正则模式检测
	for _, pattern := range f.patterns {
		matches := pattern.FindAllStringIndex(content, -1)
		for _, match := range matches {
			result.Findings = append(result.Findings, Finding{
				Type:     FindingToxicity,
				Content:  content[match[0]:match[1]],
				Position: match[0],
				Length:   match[1] - match[0],
				Severity: SeverityMedium,
			})
		}
	}

	// 使用分类器（如果有）
	if f.classifier != nil {
		score, err := f.classifier.Classify(ctx, content)
		if err == nil {
			result.Score = score.Overall
			result.Metadata["toxicity_scores"] = score

			if score.Overall >= f.config.Threshold {
				result.Passed = false
				result.Category = CategoryHarmful
				result.Action = f.config.Action
			}
		}
	} else {
		// 基于规则的简单评分
		result.Score = float64(len(result.Findings)) / 10.0
		if result.Score > 1 {
			result.Score = 1
		}
	}

	if len(result.Findings) > 0 && result.Score >= f.config.Threshold {
		result.Passed = false
		result.Category = CategoryHarmful
		result.Action = f.config.Action
	}

	return result, nil
}

// FilterBatch 批量过滤
func (f *ToxicityFilter) FilterBatch(ctx context.Context, contents []string) ([]*FilterResult, error) {
	results := make([]*FilterResult, len(contents))
	for i, content := range contents {
		result, err := f.Filter(ctx, content)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// initDefaultPatterns 初始化默认模式
func (f *ToxicityFilter) initDefaultPatterns() {
	patterns := []string{
		`(?i)\b(hate|hatred)\s+(you|him|her|them)\b`,
		`(?i)\b(kill|murder|destroy)\s+(you|him|her|them|yourself)\b`,
		`(?i)\b(threat|threatening)\b`,
	}

	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			f.patterns = append(f.patterns, re)
		}
	}
}

// ============== Content Classifier ==============

// RuleBasedClassifier 基于规则的分类器
type RuleBasedClassifier struct {
	hatePatterns     []*regexp.Regexp
	violencePatterns []*regexp.Regexp
	sexualPatterns   []*regexp.Regexp
}

// NewRuleBasedClassifier 创建规则分类器
func NewRuleBasedClassifier() *RuleBasedClassifier {
	c := &RuleBasedClassifier{}
	c.initPatterns()
	return c
}

func (c *RuleBasedClassifier) initPatterns() {
	// 仇恨言论模式
	c.hatePatterns = compilePatterns([]string{
		`(?i)\bracist\b`,
		`(?i)\bdiscrimination\b`,
	})

	// 暴力模式
	c.violencePatterns = compilePatterns([]string{
		`(?i)\b(kill|murder|attack)\b`,
		`(?i)\bviolence\b`,
	})

	// 性相关模式
	c.sexualPatterns = compilePatterns([]string{
		`(?i)\b(porn|xxx)\b`,
	})
}

// Classify 分类
func (c *RuleBasedClassifier) Classify(ctx context.Context, content string) (*ToxicityScore, error) {
	score := &ToxicityScore{}

	lowerContent := strings.ToLower(content)

	// 计算各类分数
	score.Hate = c.countMatches(lowerContent, c.hatePatterns)
	score.Violence = c.countMatches(lowerContent, c.violencePatterns)
	score.Sexual = c.countMatches(lowerContent, c.sexualPatterns)

	// 计算总分
	score.Overall = (score.Hate + score.Violence + score.Sexual) / 3

	return score, nil
}

func (c *RuleBasedClassifier) countMatches(content string, patterns []*regexp.Regexp) float64 {
	count := 0
	for _, p := range patterns {
		count += len(p.FindAllString(content, -1))
	}

	// 归一化到 0-1
	if count > 10 {
		count = 10
	}
	return float64(count) / 10.0
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	result := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			result = append(result, re)
		}
	}
	return result
}

// ============== Filter Chain ==============

// FilterChain 过滤器链
type FilterChain struct {
	filters []ContentFilter
	mode    ChainMode
}

// ChainMode 链模式
type ChainMode int

const (
	// ChainModeAll 所有过滤器都通过
	ChainModeAll ChainMode = iota

	// ChainModeAny 任一过滤器不通过
	ChainModeAny

	// ChainModeFirst 第一个不通过就停止
	ChainModeFirst
)

// NewFilterChain 创建过滤器链
func NewFilterChain(mode ChainMode, filters ...ContentFilter) *FilterChain {
	return &FilterChain{
		filters: filters,
		mode:    mode,
	}
}

// AddFilter 添加过滤器
func (c *FilterChain) AddFilter(filter ContentFilter) {
	c.filters = append(c.filters, filter)
}

// Name 返回名称
func (c *FilterChain) Name() string {
	return "filter_chain"
}

// Filter 过滤
func (c *FilterChain) Filter(ctx context.Context, content string) (*FilterResult, error) {
	combinedResult := &FilterResult{
		Original: content,
		Filtered: content,
		Passed:   true,
		Score:    0,
		Category: CategorySafe,
		Findings: make([]Finding, 0),
		Action:   ActionAllow,
		Metadata: make(map[string]any),
	}

	var totalScore float64
	filterCount := 0

	for _, filter := range c.filters {
		result, err := filter.Filter(ctx, content)
		if err != nil {
			continue
		}

		filterCount++
		totalScore += result.Score
		combinedResult.Findings = append(combinedResult.Findings, result.Findings...)

		// 根据模式处理
		switch c.mode {
		case ChainModeFirst:
			if !result.Passed {
				return result, nil
			}
		case ChainModeAny:
			if !result.Passed {
				combinedResult.Passed = false
				if severityLevel(Severity(result.Category)) > severityLevel(Severity(combinedResult.Category)) {
					combinedResult.Category = result.Category
				}
			}
		case ChainModeAll:
			combinedResult.Passed = combinedResult.Passed && result.Passed
		}

		// 更新 filtered 内容
		if result.Action == ActionRedact {
			content = result.Filtered
		}
	}

	combinedResult.Filtered = content
	if filterCount > 0 {
		combinedResult.Score = totalScore / float64(filterCount)
	}

	return combinedResult, nil
}

// FilterBatch 批量过滤
func (c *FilterChain) FilterBatch(ctx context.Context, contents []string) ([]*FilterResult, error) {
	results := make([]*FilterResult, len(contents))
	for i, content := range contents {
		result, err := c.Filter(ctx, content)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// ============== Text Normalizer ==============

// NormalizeText 规范化文本（用于预处理）
func NormalizeText(text string) string {
	// 转小写
	text = strings.ToLower(text)

	// 移除多余空格
	text = strings.Join(strings.Fields(text), " ")

	// 移除特殊字符变体
	text = normalizeChars(text)

	return text
}

// normalizeChars 规范化字符
func normalizeChars(text string) string {
	var result strings.Builder

	for _, r := range text {
		// 替换常见的字符变体
		switch {
		case unicode.Is(unicode.Mn, r): // 跳过组合标记
			continue
		case r == '@':
			result.WriteRune('a')
		case r == '0':
			result.WriteRune('o')
		case r == '1':
			result.WriteRune('l')
		case r == '3':
			result.WriteRune('e')
		case r == '4':
			result.WriteRune('a')
		case r == '5':
			result.WriteRune('s')
		default:
			result.WriteRune(r)
		}
	}

	return result.String()
}
