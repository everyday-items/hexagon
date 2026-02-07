// Package guardrails 提供 Agent 输出质量护栏
//
// 与 security/guard 不同，guardrails 关注的是输出质量而非安全性：
//   - 函数式检查：长度、关键词、格式、自定义规则
//   - LLM-as-Judge：使用 LLM 评估输出质量
//   - 幻觉检测：检测输出是否包含虚构信息
//   - 相关性检查：检测输出是否与输入相关
//
// 使用示例：
//
//	rail := NewGuardrail(
//	    WithLengthCheck(50, 2000),
//	    WithKeywordCheck([]string{"错误", "无法"}),
//	    WithLLMJudge(llmProvider, "model-name"),
//	)
//	result, err := rail.Check(ctx, input, output)
//	if !result.Passed {
//	    // 输出质量不合格，需要重新生成
//	}
package guardrails

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/everyday-items/ai-core/llm"
)

// CheckResult 检查结果
type CheckResult struct {
	// Passed 是否通过
	Passed bool `json:"passed"`

	// Score 质量分数 (0-1，越高质量越好)
	Score float64 `json:"score"`

	// Reason 不通过的原因
	Reason string `json:"reason,omitempty"`

	// Suggestions 改进建议
	Suggestions []string `json:"suggestions,omitempty"`

	// Details 详细检查结果
	Details []CheckDetail `json:"details,omitempty"`
}

// CheckDetail 单项检查详情
type CheckDetail struct {
	// Name 检查项名称
	Name string `json:"name"`

	// Passed 是否通过
	Passed bool `json:"passed"`

	// Score 分数
	Score float64 `json:"score"`

	// Message 说明信息
	Message string `json:"message,omitempty"`
}

// Checker 单项检查器接口
type Checker interface {
	// Name 检查器名称
	Name() string

	// Check 执行检查
	// input 是用户输入，output 是 Agent 输出
	Check(ctx context.Context, input, output string) (*CheckDetail, error)
}

// Guardrail 输出护栏
// 组合多个 Checker 对 Agent 输出进行质量检查
type Guardrail struct {
	checkers   []Checker
	threshold  float64 // 通过阈值（加权平均分需要达到的最低分数）
	failFast   bool    // 首个失败即停止
}

// GuardrailOption 护栏配置选项
type GuardrailOption func(*Guardrail)

// WithThreshold 设置通过阈值（0-1）
func WithThreshold(threshold float64) GuardrailOption {
	return func(g *Guardrail) {
		g.threshold = threshold
	}
}

// WithFailFast 设置失败快速返回
func WithFailFast(failFast bool) GuardrailOption {
	return func(g *Guardrail) {
		g.failFast = failFast
	}
}

// WithChecker 添加自定义检查器
func WithChecker(checker Checker) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, checker)
	}
}

// WithLengthCheck 添加长度检查
func WithLengthCheck(minLen, maxLen int) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &lengthChecker{minLen: minLen, maxLen: maxLen})
	}
}

// WithKeywordCheck 添加关键词检查（输出不应包含这些关键词）
func WithKeywordCheck(forbidden []string) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &keywordChecker{forbidden: forbidden})
	}
}

// WithFormatCheck 添加格式检查
func WithFormatCheck(format OutputFormat) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &formatChecker{format: format})
	}
}

// WithLLMJudge 添加 LLM-as-Judge 检查
func WithLLMJudge(provider llm.Provider, model string) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &llmJudgeChecker{
			provider: provider,
			model:    model,
		})
	}
}

// WithHallucinationCheck 添加幻觉检测
func WithHallucinationCheck(provider llm.Provider, model string, context string) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &hallucinationChecker{
			provider:   provider,
			model:      model,
			refContext: context,
		})
	}
}

// WithRelevanceCheck 添加相关性检查
func WithRelevanceCheck(provider llm.Provider, model string) GuardrailOption {
	return func(g *Guardrail) {
		g.checkers = append(g.checkers, &relevanceChecker{
			provider: provider,
			model:    model,
		})
	}
}

// NewGuardrail 创建输出护栏
func NewGuardrail(opts ...GuardrailOption) *Guardrail {
	g := &Guardrail{
		threshold: 0.7,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Check 执行所有检查
func (g *Guardrail) Check(ctx context.Context, input, output string) (*CheckResult, error) {
	if len(g.checkers) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}

	result := &CheckResult{
		Details: make([]CheckDetail, 0, len(g.checkers)),
	}

	totalScore := 0.0
	allPassed := true

	for _, checker := range g.checkers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		detail, err := checker.Check(ctx, input, output)
		if err != nil {
			detail = &CheckDetail{
				Name:    checker.Name(),
				Passed:  false,
				Score:   0,
				Message: fmt.Sprintf("检查出错: %v", err),
			}
		}

		result.Details = append(result.Details, *detail)
		totalScore += detail.Score

		if !detail.Passed {
			allPassed = false
			result.Suggestions = append(result.Suggestions, detail.Message)

			if g.failFast {
				result.Passed = false
				result.Score = totalScore / float64(len(result.Details))
				result.Reason = detail.Message
				return result, nil
			}
		}
	}

	result.Score = totalScore / float64(len(g.checkers))
	result.Passed = allPassed && result.Score >= g.threshold

	if !result.Passed && result.Reason == "" {
		result.Reason = fmt.Sprintf("综合得分 %.2f 低于阈值 %.2f", result.Score, g.threshold)
	}

	return result, nil
}

// ============== 内置检查器实现 ==============

// OutputFormat 输出格式类型
type OutputFormat int

const (
	// FormatJSON JSON 格式
	FormatJSON OutputFormat = iota

	// FormatMarkdown Markdown 格式
	FormatMarkdown

	// FormatPlainText 纯文本格式
	FormatPlainText
)

// lengthChecker 长度检查器
type lengthChecker struct {
	minLen int
	maxLen int
}

func (c *lengthChecker) Name() string { return "length_check" }

func (c *lengthChecker) Check(_ context.Context, _, output string) (*CheckDetail, error) {
	length := utf8.RuneCountInString(output)

	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: true,
		Score:  1.0,
	}

	if c.minLen > 0 && length < c.minLen {
		detail.Passed = false
		detail.Score = float64(length) / float64(c.minLen)
		detail.Message = fmt.Sprintf("输出长度 %d 低于最小要求 %d", length, c.minLen)
	}

	if c.maxLen > 0 && length > c.maxLen {
		detail.Passed = false
		detail.Score = float64(c.maxLen) / float64(length)
		detail.Message = fmt.Sprintf("输出长度 %d 超过最大限制 %d", length, c.maxLen)
	}

	return detail, nil
}

// keywordChecker 关键词检查器
type keywordChecker struct {
	forbidden []string
}

func (c *keywordChecker) Name() string { return "keyword_check" }

func (c *keywordChecker) Check(_ context.Context, _, output string) (*CheckDetail, error) {
	outputLower := strings.ToLower(output)

	var found []string
	for _, kw := range c.forbidden {
		if strings.Contains(outputLower, strings.ToLower(kw)) {
			found = append(found, kw)
		}
	}

	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: len(found) == 0,
		Score:  1.0 - float64(len(found))/float64(len(c.forbidden)+1),
	}

	if len(found) > 0 {
		detail.Message = fmt.Sprintf("输出包含禁止关键词: %s", strings.Join(found, ", "))
	}

	return detail, nil
}

// formatChecker 格式检查器
type formatChecker struct {
	format OutputFormat
}

func (c *formatChecker) Name() string { return "format_check" }

func (c *formatChecker) Check(_ context.Context, _, output string) (*CheckDetail, error) {
	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: true,
		Score:  1.0,
	}

	switch c.format {
	case FormatJSON:
		trimmed := strings.TrimSpace(output)
		if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
			detail.Passed = false
			detail.Score = 0
			detail.Message = "输出不是有效的 JSON 格式"
		}

	case FormatMarkdown:
		// 检查是否包含 Markdown 标记
		hasMarkdown := strings.Contains(output, "#") ||
			strings.Contains(output, "**") ||
			strings.Contains(output, "- ") ||
			strings.Contains(output, "```")
		if !hasMarkdown {
			detail.Passed = true // Markdown 检查不强制
			detail.Score = 0.5
			detail.Message = "输出缺少 Markdown 格式标记"
		}
	}

	return detail, nil
}

// llmJudgeChecker LLM-as-Judge 检查器
// 使用 LLM 评估输出质量
type llmJudgeChecker struct {
	provider llm.Provider
	model    string
}

func (c *llmJudgeChecker) Name() string { return "llm_judge" }

func (c *llmJudgeChecker) Check(ctx context.Context, input, output string) (*CheckDetail, error) {
	prompt := fmt.Sprintf(`请评估以下 AI 助手的回复质量。

用户问题: %s

AI 回复: %s

请从以下维度评分（1-10分）:
1. 准确性：信息是否准确
2. 完整性：是否充分回答了问题
3. 清晰度：表达是否清晰易懂
4. 相关性：是否与问题相关

只回复一个 1-10 的整数分数，不要解释。`, input, output)

	req := llm.CompletionRequest{
		Model: c.model,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: floatPtr(0.0),
		MaxTokens:   10,
	}

	resp, err := c.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM Judge 调用失败: %w", err)
	}

	// 解析分数
	score := parseScore(resp.Content)

	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: score >= 0.6,
		Score:  score,
	}

	if score < 0.6 {
		detail.Message = fmt.Sprintf("LLM 评估质量分数 %.1f 不达标", score*10)
	}

	return detail, nil
}

// hallucinationChecker 幻觉检测器
type hallucinationChecker struct {
	provider   llm.Provider
	model      string
	refContext string // 参考上下文
}

func (c *hallucinationChecker) Name() string { return "hallucination_check" }

func (c *hallucinationChecker) Check(ctx context.Context, input, output string) (*CheckDetail, error) {
	prompt := fmt.Sprintf(`判断以下 AI 回复中是否存在虚构信息（幻觉）。

参考信息: %s

用户问题: %s

AI 回复: %s

只回复 "是"（存在幻觉）或 "否"（不存在幻觉），不要解释。`, c.refContext, input, output)

	req := llm.CompletionRequest{
		Model: c.model,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: floatPtr(0.0),
		MaxTokens:   10,
	}

	resp, err := c.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("幻觉检测调用失败: %w", err)
	}

	hasHallucination := strings.Contains(resp.Content, "是")

	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: !hasHallucination,
		Score:  1.0,
	}

	if hasHallucination {
		detail.Score = 0.0
		detail.Message = "检测到输出可能包含虚构信息"
	}

	return detail, nil
}

// relevanceChecker 相关性检查器
type relevanceChecker struct {
	provider llm.Provider
	model    string
}

func (c *relevanceChecker) Name() string { return "relevance_check" }

func (c *relevanceChecker) Check(ctx context.Context, input, output string) (*CheckDetail, error) {
	prompt := fmt.Sprintf(`评估以下 AI 回复与用户问题的相关性。

用户问题: %s

AI 回复: %s

相关性评分（1-10分），只回复数字:`, input, output)

	req := llm.CompletionRequest{
		Model: c.model,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: floatPtr(0.0),
		MaxTokens:   10,
	}

	resp, err := c.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("相关性检查调用失败: %w", err)
	}

	score := parseScore(resp.Content)

	detail := &CheckDetail{
		Name:   c.Name(),
		Passed: score >= 0.5,
		Score:  score,
	}

	if score < 0.5 {
		detail.Message = "回复与问题相关性不足"
	}

	return detail, nil
}

// ============== 辅助函数 ==============

// parseScore 从 LLM 回复中解析分数 (1-10 → 0-1)
func parseScore(content string) float64 {
	content = strings.TrimSpace(content)
	for _, ch := range content {
		if ch >= '0' && ch <= '9' {
			score := float64(ch-'0') / 10.0
			return score
		}
	}
	return 0.5 // 无法解析时返回中等分数
}

func floatPtr(f float64) *float64 {
	return &f
}
