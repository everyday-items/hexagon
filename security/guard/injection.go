package guard

import (
	"context"
	"regexp"
	"strings"
)

// PromptInjectionGuard Prompt 注入检测守卫
type PromptInjectionGuard struct {
	config    *GuardConfig
	patterns  []*injectionPattern
	enabled   bool
}

// injectionPattern 注入模式
type injectionPattern struct {
	name     string
	pattern  *regexp.Regexp
	severity string
	score    float64
}

// NewPromptInjectionGuard 创建 Prompt 注入守卫
func NewPromptInjectionGuard(opts ...PromptInjectionOption) *PromptInjectionGuard {
	g := &PromptInjectionGuard{
		config:  DefaultConfig(),
		enabled: true,
	}

	// 默认模式
	g.patterns = defaultInjectionPatterns()

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// PromptInjectionOption 配置选项
type PromptInjectionOption func(*PromptInjectionGuard)

// WithInjectionConfig 设置配置
func WithInjectionConfig(cfg *GuardConfig) PromptInjectionOption {
	return func(g *PromptInjectionGuard) {
		g.config = cfg
	}
}

// WithCustomPatterns 添加自定义模式
func WithCustomPatterns(patterns map[string]string) PromptInjectionOption {
	return func(g *PromptInjectionGuard) {
		for name, pattern := range patterns {
			if re, err := regexp.Compile(pattern); err == nil {
				g.patterns = append(g.patterns, &injectionPattern{
					name:     name,
					pattern:  re,
					severity: "high",
					score:    0.8,
				})
			}
		}
	}
}

// Name 返回名称
func (g *PromptInjectionGuard) Name() string {
	return "prompt_injection"
}

// Enabled 返回是否启用
func (g *PromptInjectionGuard) Enabled() bool {
	return g.enabled && g.config.Enabled
}

// Check 执行检查
func (g *PromptInjectionGuard) Check(ctx context.Context, input string) (*CheckResult, error) {
	if !g.Enabled() {
		return &CheckResult{Passed: true}, nil
	}

	var findings []Finding
	var maxScore float64 = 0

	// 转换为小写进行检查
	lowerInput := strings.ToLower(input)

	for _, p := range g.patterns {
		matches := p.pattern.FindAllStringIndex(lowerInput, -1)
		for _, match := range matches {
			findings = append(findings, Finding{
				Type:     p.name,
				Text:     input[match[0]:match[1]],
				Position: Position{Start: match[0], End: match[1]},
				Severity: p.severity,
			})
			if p.score > maxScore {
				maxScore = p.score
			}
		}
	}

	// 额外检查：启发式规则
	heuristicScore := g.checkHeuristics(lowerInput)
	if heuristicScore > maxScore {
		maxScore = heuristicScore
	}

	passed := maxScore < g.config.Threshold

	result := &CheckResult{
		Passed:   passed,
		Score:    maxScore,
		Category: "prompt_injection",
		Findings: findings,
	}

	if !passed {
		result.Reason = "Potential prompt injection detected"
	}

	return result, nil
}

// checkHeuristics 启发式检查
func (g *PromptInjectionGuard) checkHeuristics(input string) float64 {
	var score float64 = 0

	// 检查可疑关键词密度
	suspiciousKeywords := []string{
		"ignore", "forget", "disregard", "override",
		"system prompt", "new instructions", "act as",
		"pretend", "roleplay", "jailbreak",
		"you are now", "you must", "bypass",
	}

	keywordCount := 0
	for _, kw := range suspiciousKeywords {
		if strings.Contains(input, kw) {
			keywordCount++
		}
	}

	if keywordCount > 0 {
		score = float64(keywordCount) * 0.15
		if score > 0.9 {
			score = 0.9
		}
	}

	// 检查特殊字符模式
	if strings.Contains(input, "```") && strings.Contains(input, "system") {
		score += 0.3
	}

	// 检查换行符滥用
	newlineCount := strings.Count(input, "\n")
	if newlineCount > 10 && len(input) < 500 {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// IsInputGuard 标记为输入守卫
func (g *PromptInjectionGuard) IsInputGuard() {}

// 确保实现了接口
var _ InputGuard = (*PromptInjectionGuard)(nil)

// defaultInjectionPatterns 默认注入模式
func defaultInjectionPatterns() []*injectionPattern {
	patterns := []struct {
		name     string
		pattern  string
		severity string
		score    float64
	}{
		// 直接指令覆盖
		{"direct_override", `(?i)(ignore|forget|disregard).{0,20}(previous|above|prior|all).{0,20}(instructions?|rules?|prompts?)`, "critical", 0.95},
		{"new_instructions", `(?i)(new|different|updated).{0,20}(instructions?|rules?|prompts?).{0,20}(are|is|:)`, "high", 0.85},

		// 角色扮演注入
		{"role_hijack", `(?i)(you are now|act as|pretend to be|roleplay as).{0,50}(assistant|ai|bot|system)`, "high", 0.85},
		{"identity_override", `(?i)(forget|ignore).{0,20}(you are|your role|your identity)`, "critical", 0.9},

		// 系统提示词提取
		{"prompt_leak", `(?i)(show|reveal|display|print|output).{0,30}(system|original|initial).{0,20}(prompt|instructions?)`, "high", 0.85},
		{"repeat_prompt", `(?i)(repeat|echo|say).{0,20}(everything|all).{0,20}(above|before|previous)`, "medium", 0.7},

		// 分隔符注入
		{"delimiter_injection", `(?i)(\[system\]|\[assistant\]|\[user\]|<\|im_start\|>|<\|im_end\|>)`, "critical", 0.95},
		{"markdown_injection", `(?i)(###|---).{0,10}(system|instructions?|new role)`, "high", 0.8},

		// DAN 类攻击
		{"jailbreak_attempt", `(?i)(jailbreak|dan|do anything now|developer mode|unleashed)`, "critical", 0.95},
		{"bypass_attempt", `(?i)(bypass|circumvent|workaround).{0,20}(safety|filter|restriction|rule)`, "high", 0.85},

		// 编码绕过
		{"encoding_bypass", `(?i)(base64|hex|rot13|unicode).{0,20}(decode|encode|convert)`, "medium", 0.7},

		// 虚假输出
		{"fake_output", `(?i)(output|response|answer).{0,10}(:|=).{0,20}(yes|allowed|permitted|successful)`, "high", 0.8},
	}

	result := make([]*injectionPattern, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p.pattern); err == nil {
			result = append(result, &injectionPattern{
				name:     p.name,
				pattern:  re,
				severity: p.severity,
				score:    p.score,
			})
		}
	}

	return result
}

// PIIGuard PII 检测守卫
type PIIGuard struct {
	config   *GuardConfig
	patterns []*piiPattern
	enabled  bool
}

// piiPattern PII 模式
type piiPattern struct {
	name    string
	pattern *regexp.Regexp
	redact  func(string) string
}

// NewPIIGuard 创建 PII 守卫
func NewPIIGuard(opts ...PIIOption) *PIIGuard {
	g := &PIIGuard{
		config:  DefaultConfig(),
		enabled: true,
	}

	g.patterns = defaultPIIPatterns()

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// PIIOption 配置选项
type PIIOption func(*PIIGuard)

// WithPIIConfig 设置配置
func WithPIIConfig(cfg *GuardConfig) PIIOption {
	return func(g *PIIGuard) {
		g.config = cfg
	}
}

// Name 返回名称
func (g *PIIGuard) Name() string {
	return "pii_detection"
}

// Enabled 返回是否启用
func (g *PIIGuard) Enabled() bool {
	return g.enabled && g.config.Enabled
}

// Check 执行检查
func (g *PIIGuard) Check(ctx context.Context, input string) (*CheckResult, error) {
	if !g.Enabled() {
		return &CheckResult{Passed: true}, nil
	}

	var findings []Finding
	var maxScore float64 = 0

	for _, p := range g.patterns {
		matches := p.pattern.FindAllStringIndex(input, -1)
		for _, match := range matches {
			findings = append(findings, Finding{
				Type:     p.name,
				Text:     "[REDACTED]", // 不输出实际 PII
				Position: Position{Start: match[0], End: match[1]},
				Severity: "high",
			})
			maxScore = 0.9
		}
	}

	passed := maxScore < g.config.Threshold

	result := &CheckResult{
		Passed:   passed,
		Score:    maxScore,
		Category: "pii",
		Findings: findings,
	}

	if !passed {
		result.Reason = "PII detected in input"
	}

	return result, nil
}

// Redact 脱敏处理
func (g *PIIGuard) Redact(input string) string {
	result := input
	for _, p := range g.patterns {
		result = p.pattern.ReplaceAllStringFunc(result, p.redact)
	}
	return result
}

// IsInputGuard 标记为输入守卫
func (g *PIIGuard) IsInputGuard() {}

var _ InputGuard = (*PIIGuard)(nil)

// defaultPIIPatterns 默认 PII 模式
func defaultPIIPatterns() []*piiPattern {
	return []*piiPattern{
		// 邮箱
		{
			name:    "email",
			pattern: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			redact:  func(s string) string { return "[EMAIL]" },
		},
		// 手机号（中国）
		{
			name:    "phone_cn",
			pattern: regexp.MustCompile(`1[3-9]\d{9}`),
			redact:  func(s string) string { return "[PHONE]" },
		},
		// 身份证号（中国）
		{
			name:    "id_card_cn",
			pattern: regexp.MustCompile(`[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
			redact:  func(s string) string { return "[ID_CARD]" },
		},
		// 信用卡号
		{
			name:    "credit_card",
			pattern: regexp.MustCompile(`\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}`),
			redact:  func(s string) string { return "[CREDIT_CARD]" },
		},
		// 银行卡号
		{
			name:    "bank_card",
			pattern: regexp.MustCompile(`\d{16,19}`),
			redact:  func(s string) string { return "[BANK_CARD]" },
		},
		// IP 地址
		{
			name:    "ip_address",
			pattern: regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`),
			redact:  func(s string) string { return "[IP_ADDRESS]" },
		},
	}
}
