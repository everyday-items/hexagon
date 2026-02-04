// Package pii 提供 PII（个人身份信息）检测与脱敏功能
//
// 本包实现 PII 检测与处理：
//   - 检测：识别文本中的 PII
//   - 脱敏：对 PII 进行遮蔽或替换
//   - 多语言：支持中文和英文
//   - 可扩展：支持自定义 PII 类型
//
// 设计借鉴：
//   - Microsoft Presidio: PII 检测
//   - AWS Comprehend: PII 识别
//   - Google DLP: 数据脱敏
package pii

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// ============== PII 类型定义 ==============

// PIIType PII 类型
type PIIType string

const (
	// PIITypePhone 电话号码
	PIITypePhone PIIType = "phone"

	// PIITypeEmail 邮箱地址
	PIITypeEmail PIIType = "email"

	// PIITypeIDCard 身份证号
	PIITypeIDCard PIIType = "id_card"

	// PIITypeCreditCard 信用卡号
	PIITypeCreditCard PIIType = "credit_card"

	// PIITypeBankAccount 银行账号
	PIITypeBankAccount PIIType = "bank_account"

	// PIITypeName 姓名
	PIITypeName PIIType = "name"

	// PIITypeAddress 地址
	PIITypeAddress PIIType = "address"

	// PIITypePassport 护照号
	PIITypePassport PIIType = "passport"

	// PIITypeSSN 社会安全号（美国）
	PIITypeSSN PIIType = "ssn"

	// PIITypeLicense 驾照号
	PIITypeLicense PIIType = "license"

	// PIITypeIP IP 地址
	PIITypeIP PIIType = "ip"

	// PIITypeMac MAC 地址
	PIITypeMac PIIType = "mac"

	// PIITypeURL URL
	PIITypeURL PIIType = "url"

	// PIITypeDate 日期
	PIITypeDate PIIType = "date"

	// PIITypeCustom 自定义类型
	PIITypeCustom PIIType = "custom"
)

// PIIEntity 检测到的 PII 实体
type PIIEntity struct {
	// Type PII 类型
	Type PIIType `json:"type"`

	// Value 原始值
	Value string `json:"value"`

	// Start 起始位置
	Start int `json:"start"`

	// End 结束位置
	End int `json:"end"`

	// Score 置信度（0-1）
	Score float64 `json:"score"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ============== 检测器接口 ==============

// Detector PII 检测器接口
type Detector interface {
	// Detect 检测 PII
	Detect(ctx context.Context, text string) ([]*PIIEntity, error)

	// SupportedTypes 支持的 PII 类型
	SupportedTypes() []PIIType
}

// ============== 正则检测器 ==============

// RegexDetector 基于正则表达式的检测器
type RegexDetector struct {
	patterns map[PIIType][]*Pattern
	mu       sync.RWMutex
}

// Pattern 正则模式
type Pattern struct {
	// Name 模式名称
	Name string

	// Regex 正则表达式
	Regex *regexp.Regexp

	// Score 默认置信度
	Score float64

	// Validator 验证函数（可选）
	Validator func(match string) bool
}

// NewRegexDetector 创建正则检测器
func NewRegexDetector() *RegexDetector {
	d := &RegexDetector{
		patterns: make(map[PIIType][]*Pattern),
	}

	// 注册默认模式
	d.registerDefaultPatterns()

	return d
}

// registerDefaultPatterns 注册默认模式
func (d *RegexDetector) registerDefaultPatterns() {
	// 中国手机号
	d.RegisterPattern(PIITypePhone, &Pattern{
		Name:  "china_mobile",
		Regex: regexp.MustCompile(`1[3-9]\d{9}`),
		Score: 0.9,
	})

	// 美国电话
	d.RegisterPattern(PIITypePhone, &Pattern{
		Name:  "us_phone",
		Regex: regexp.MustCompile(`\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`),
		Score: 0.85,
	})

	// 邮箱
	d.RegisterPattern(PIITypeEmail, &Pattern{
		Name:  "email",
		Regex: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Score: 0.95,
	})

	// 中国身份证号（18位）
	d.RegisterPattern(PIITypeIDCard, &Pattern{
		Name:      "china_id_card",
		Regex:     regexp.MustCompile(`[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
		Score:     0.95,
		Validator: validateChinaIDCard,
	})

	// 信用卡号（简化）
	d.RegisterPattern(PIITypeCreditCard, &Pattern{
		Name:      "credit_card",
		Regex:     regexp.MustCompile(`(?:\d{4}[-\s]?){3}\d{4}`),
		Score:     0.8,
		Validator: validateCreditCard,
	})

	// 银行账号
	d.RegisterPattern(PIITypeBankAccount, &Pattern{
		Name:  "bank_account",
		Regex: regexp.MustCompile(`\d{16,19}`),
		Score: 0.6, // 较低置信度，因为可能误报
	})

	// 美国 SSN
	d.RegisterPattern(PIITypeSSN, &Pattern{
		Name:  "us_ssn",
		Regex: regexp.MustCompile(`\d{3}-\d{2}-\d{4}`),
		Score: 0.9,
	})

	// IP 地址 (IPv4)
	d.RegisterPattern(PIITypeIP, &Pattern{
		Name:  "ipv4",
		Regex: regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),
		Score: 0.95,
	})

	// IP 地址 (IPv6)
	d.RegisterPattern(PIITypeIP, &Pattern{
		Name:  "ipv6",
		Regex: regexp.MustCompile(`([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}`),
		Score: 0.95,
	})

	// MAC 地址
	d.RegisterPattern(PIITypeMac, &Pattern{
		Name:  "mac",
		Regex: regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`),
		Score: 0.95,
	})

	// URL
	d.RegisterPattern(PIITypeURL, &Pattern{
		Name:  "url",
		Regex: regexp.MustCompile(`https?://[^\s<>\[\]{}|\\^` + "`" + `]+`),
		Score: 0.9,
	})

	// 日期（多种格式）
	d.RegisterPattern(PIITypeDate, &Pattern{
		Name:  "date_ymd",
		Regex: regexp.MustCompile(`\d{4}[-/年]\d{1,2}[-/月]\d{1,2}[日]?`),
		Score: 0.7,
	})

	// 护照号（中国）
	d.RegisterPattern(PIITypePassport, &Pattern{
		Name:  "china_passport",
		Regex: regexp.MustCompile(`[EeKkGgDdSsPpHh]\d{8}`),
		Score: 0.85,
	})
}

// RegisterPattern 注册模式
func (d *RegexDetector) RegisterPattern(piiType PIIType, pattern *Pattern) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.patterns[piiType] = append(d.patterns[piiType], pattern)
}

// Detect 检测 PII
func (d *RegexDetector) Detect(ctx context.Context, text string) ([]*PIIEntity, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var entities []*PIIEntity

	for piiType, patterns := range d.patterns {
		for _, pattern := range patterns {
			matches := pattern.Regex.FindAllStringIndex(text, -1)
			for _, match := range matches {
				value := text[match[0]:match[1]]

				// 可选验证
				if pattern.Validator != nil && !pattern.Validator(value) {
					continue
				}

				entities = append(entities, &PIIEntity{
					Type:  piiType,
					Value: value,
					Start: match[0],
					End:   match[1],
					Score: pattern.Score,
					Metadata: map[string]any{
						"pattern": pattern.Name,
					},
				})
			}
		}
	}

	// 去重（保留置信度最高的）
	entities = d.dedup(entities)

	return entities, nil
}

// dedup 去重
func (d *RegexDetector) dedup(entities []*PIIEntity) []*PIIEntity {
	if len(entities) == 0 {
		return entities
	}

	// 按位置分组，保留置信度最高的
	posMap := make(map[string]*PIIEntity)
	for _, e := range entities {
		key := fmt.Sprintf("%d-%d", e.Start, e.End)
		if existing, ok := posMap[key]; ok {
			if e.Score > existing.Score {
				posMap[key] = e
			}
		} else {
			posMap[key] = e
		}
	}

	result := make([]*PIIEntity, 0, len(posMap))
	for _, e := range posMap {
		result = append(result, e)
	}

	return result
}

// SupportedTypes 支持的类型
func (d *RegexDetector) SupportedTypes() []PIIType {
	d.mu.RLock()
	defer d.mu.RUnlock()

	types := make([]PIIType, 0, len(d.patterns))
	for t := range d.patterns {
		types = append(types, t)
	}
	return types
}

// ============== 验证函数 ==============

// validateChinaIDCard 验证中国身份证号
func validateChinaIDCard(id string) bool {
	if len(id) != 18 {
		return false
	}

	// 加权因子
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	// 校验码对应值
	checkCodes := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

	sum := 0
	for i := 0; i < 17; i++ {
		digit := int(id[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}
		sum += digit * weights[i]
	}

	checkCode := checkCodes[sum%11]
	lastChar := id[17]
	if lastChar == 'x' {
		lastChar = 'X'
	}

	return checkCode == lastChar
}

// validateCreditCard 验证信用卡号（Luhn 算法）
func validateCreditCard(number string) bool {
	// 移除空格和连字符
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")

	if len(number) < 13 || len(number) > 19 {
		return false
	}

	sum := 0
	double := false

	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}

		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		double = !double
	}

	return sum%10 == 0
}

// ============== 脱敏器 ==============

// Anonymizer 脱敏器
type Anonymizer struct {
	// strategies 脱敏策略
	strategies map[PIIType]AnonymizeStrategy

	// defaultStrategy 默认策略
	defaultStrategy AnonymizeStrategy
}

// AnonymizeStrategy 脱敏策略
type AnonymizeStrategy interface {
	// Anonymize 脱敏
	Anonymize(entity *PIIEntity) string
}

// MaskStrategy 遮蔽策略
type MaskStrategy struct {
	// MaskChar 遮蔽字符
	MaskChar rune

	// PreserveFirst 保留前几位
	PreserveFirst int

	// PreserveLast 保留后几位
	PreserveLast int
}

// Anonymize 遮蔽脱敏
func (s *MaskStrategy) Anonymize(entity *PIIEntity) string {
	value := entity.Value
	length := len([]rune(value))

	if length <= s.PreserveFirst+s.PreserveLast {
		return strings.Repeat(string(s.MaskChar), length)
	}

	runes := []rune(value)
	first := string(runes[:s.PreserveFirst])
	last := string(runes[length-s.PreserveLast:])
	middle := strings.Repeat(string(s.MaskChar), length-s.PreserveFirst-s.PreserveLast)

	return first + middle + last
}

// ReplaceStrategy 替换策略
type ReplaceStrategy struct {
	// Replacement 替换文本
	Replacement string
}

// Anonymize 替换脱敏
func (s *ReplaceStrategy) Anonymize(entity *PIIEntity) string {
	return s.Replacement
}

// HashStrategy 哈希策略
type HashStrategy struct {
	// Prefix 前缀
	Prefix string

	// Length 长度（用于截取原始值的前 N 个字符进行哈希）
	// 如果 Length <= 0 或大于原始值长度，将使用整个值
	Length int
}

// Anonymize 哈希脱敏
func (s *HashStrategy) Anonymize(entity *PIIEntity) string {
	if entity == nil || entity.Value == "" {
		return s.Prefix + "00000000"
	}

	// 安全地截取字符串，防止越界
	valueToHash := entity.Value
	if s.Length > 0 && s.Length < len(entity.Value) {
		valueToHash = entity.Value[:s.Length]
	}

	return fmt.Sprintf("%s%x", s.Prefix, valueToHash)
}

// NewAnonymizer 创建脱敏器
func NewAnonymizer() *Anonymizer {
	a := &Anonymizer{
		strategies:      make(map[PIIType]AnonymizeStrategy),
		defaultStrategy: &MaskStrategy{MaskChar: '*', PreserveFirst: 0, PreserveLast: 0},
	}

	// 设置默认策略
	a.SetStrategy(PIITypePhone, &MaskStrategy{MaskChar: '*', PreserveFirst: 3, PreserveLast: 4})
	a.SetStrategy(PIITypeEmail, &MaskStrategy{MaskChar: '*', PreserveFirst: 2, PreserveLast: 0})
	a.SetStrategy(PIITypeIDCard, &MaskStrategy{MaskChar: '*', PreserveFirst: 6, PreserveLast: 4})
	a.SetStrategy(PIITypeCreditCard, &MaskStrategy{MaskChar: '*', PreserveFirst: 4, PreserveLast: 4})
	a.SetStrategy(PIITypeName, &ReplaceStrategy{Replacement: "[姓名]"})
	a.SetStrategy(PIITypeAddress, &ReplaceStrategy{Replacement: "[地址]"})

	return a
}

// SetStrategy 设置策略
func (a *Anonymizer) SetStrategy(piiType PIIType, strategy AnonymizeStrategy) {
	a.strategies[piiType] = strategy
}

// Anonymize 执行脱敏
func (a *Anonymizer) Anonymize(text string, entities []*PIIEntity) string {
	if len(entities) == 0 {
		return text
	}

	// 按位置倒序排序，从后往前替换
	sortedEntities := make([]*PIIEntity, len(entities))
	copy(sortedEntities, entities)
	for i := 0; i < len(sortedEntities)-1; i++ {
		for j := i + 1; j < len(sortedEntities); j++ {
			if sortedEntities[i].Start < sortedEntities[j].Start {
				sortedEntities[i], sortedEntities[j] = sortedEntities[j], sortedEntities[i]
			}
		}
	}

	result := text
	for _, entity := range sortedEntities {
		strategy := a.strategies[entity.Type]
		if strategy == nil {
			strategy = a.defaultStrategy
		}

		replacement := strategy.Anonymize(entity)
		result = result[:entity.Start] + replacement + result[entity.End:]
	}

	return result
}

// ============== PII 处理器 ==============

// Processor PII 处理器
type Processor struct {
	// detector 检测器
	detector Detector

	// anonymizer 脱敏器
	anonymizer *Anonymizer

	// config 配置
	config ProcessorConfig
}

// ProcessorConfig 处理器配置
type ProcessorConfig struct {
	// EnabledTypes 启用的 PII 类型（空表示全部）
	EnabledTypes []PIIType

	// MinScore 最小置信度
	MinScore float64

	// AutoAnonymize 自动脱敏
	AutoAnonymize bool
}

// DefaultProcessorConfig 默认配置
var DefaultProcessorConfig = ProcessorConfig{
	MinScore:      0.8,
	AutoAnonymize: true,
}

// NewProcessor 创建处理器
func NewProcessor(config ...ProcessorConfig) *Processor {
	cfg := DefaultProcessorConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return &Processor{
		detector:   NewRegexDetector(),
		anonymizer: NewAnonymizer(),
		config:     cfg,
	}
}

// ProcessResult 处理结果
type ProcessResult struct {
	// OriginalText 原始文本
	OriginalText string `json:"original_text"`

	// AnonymizedText 脱敏后文本
	AnonymizedText string `json:"anonymized_text,omitempty"`

	// Entities 检测到的实体
	Entities []*PIIEntity `json:"entities"`

	// HasPII 是否包含 PII
	HasPII bool `json:"has_pii"`
}

// Process 处理文本
func (p *Processor) Process(ctx context.Context, text string) (*ProcessResult, error) {
	// 检测 PII
	entities, err := p.detector.Detect(ctx, text)
	if err != nil {
		return nil, err
	}

	// 过滤
	filtered := p.filterEntities(entities)

	result := &ProcessResult{
		OriginalText: text,
		Entities:     filtered,
		HasPII:       len(filtered) > 0,
	}

	// 自动脱敏
	if p.config.AutoAnonymize && len(filtered) > 0 {
		result.AnonymizedText = p.anonymizer.Anonymize(text, filtered)
	}

	return result, nil
}

// filterEntities 过滤实体
func (p *Processor) filterEntities(entities []*PIIEntity) []*PIIEntity {
	var filtered []*PIIEntity

	enabledSet := make(map[PIIType]bool)
	for _, t := range p.config.EnabledTypes {
		enabledSet[t] = true
	}

	for _, entity := range entities {
		// 检查类型
		if len(p.config.EnabledTypes) > 0 && !enabledSet[entity.Type] {
			continue
		}

		// 检查置信度
		if entity.Score < p.config.MinScore {
			continue
		}

		filtered = append(filtered, entity)
	}

	return filtered
}

// Detect 仅检测（不脱敏）
func (p *Processor) Detect(ctx context.Context, text string) ([]*PIIEntity, error) {
	entities, err := p.detector.Detect(ctx, text)
	if err != nil {
		return nil, err
	}
	return p.filterEntities(entities), nil
}

// Anonymize 脱敏
func (p *Processor) Anonymize(text string, entities []*PIIEntity) string {
	return p.anonymizer.Anonymize(text, entities)
}

// SetDetector 设置检测器
func (p *Processor) SetDetector(detector Detector) {
	p.detector = detector
}

// SetAnonymizer 设置脱敏器
func (p *Processor) SetAnonymizer(anonymizer *Anonymizer) {
	p.anonymizer = anonymizer
}

// ============== 全局处理器 ==============

var (
	globalProcessor     *Processor
	globalProcessorOnce sync.Once
)

// Global 获取全局处理器
func Global() *Processor {
	globalProcessorOnce.Do(func() {
		globalProcessor = NewProcessor()
	})
	return globalProcessor
}

// DetectPII 检测 PII（使用全局处理器）
func DetectPII(ctx context.Context, text string) ([]*PIIEntity, error) {
	return Global().Detect(ctx, text)
}

// AnonymizeText 脱敏文本（使用全局处理器）
func AnonymizeText(ctx context.Context, text string) (string, error) {
	result, err := Global().Process(ctx, text)
	if err != nil {
		return "", err
	}
	if result.AnonymizedText != "" {
		return result.AnonymizedText, nil
	}
	return text, nil
}
