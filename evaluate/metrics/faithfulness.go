// Package metrics 提供 AI 系统的评估指标
package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== FaithfulnessEvaluator ==============

// FaithfulnessEvaluator 评估回答对检索上下文的忠实度
// 检查回答是否完全基于提供的上下文，没有编造信息
type FaithfulnessEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
	strict    bool // 严格模式：任何编造都导致失败
}

// FaithfulnessOption FaithfulnessEvaluator 选项
type FaithfulnessOption func(*FaithfulnessEvaluator)

// WithFaithfulnessThreshold 设置通过阈值
func WithFaithfulnessThreshold(threshold float64) FaithfulnessOption {
	return func(e *FaithfulnessEvaluator) {
		e.threshold = threshold
	}
}

// WithFaithfulnessStrict 设置严格模式
func WithFaithfulnessStrict(strict bool) FaithfulnessOption {
	return func(e *FaithfulnessEvaluator) {
		e.strict = strict
	}
}

// NewFaithfulnessEvaluator 创建忠实度评估器
func NewFaithfulnessEvaluator(llm evaluate.LLMJudge, opts ...FaithfulnessOption) *FaithfulnessEvaluator {
	e := &FaithfulnessEvaluator{
		llm:       llm,
		threshold: 0.7,
		strict:    false,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *FaithfulnessEvaluator) Name() string {
	return "faithfulness"
}

// Description 返回评估器描述
func (e *FaithfulnessEvaluator) Description() string {
	return "Evaluates whether the response is faithful to the provided context (no hallucinations)"
}

// RequiresLLM 返回是否需要 LLM
func (e *FaithfulnessEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行忠实度评估
func (e *FaithfulnessEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if len(input.Context) == 0 {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    1.0, // 没有上下文，无法判断是否编造
			Reason:   "No context provided, cannot evaluate faithfulness",
			Duration: time.Since(start),
		}, nil
	}

	if input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Empty response",
			Duration: time.Since(start),
		}, nil
	}

	// 1. 提取回答中的声明
	claims, err := e.extractClaims(ctx, input.Response)
	if err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	if len(claims) == 0 {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    1.0,
			Reason:   "No verifiable claims found in response",
			Duration: time.Since(start),
		}, nil
	}

	// 2. 验证每个声明是否有上下文支持
	combinedContext := strings.Join(input.Context, "\n\n")
	supportedClaims := 0
	unsupportedClaims := []string{}
	subScores := make(map[string]float64)

	for i, claim := range claims {
		supported, err := e.verifyClaim(ctx, claim, combinedContext)
		if err != nil {
			return nil, fmt.Errorf("failed to verify claim %d: %w", i, err)
		}

		key := fmt.Sprintf("claim_%d", i)
		if supported {
			subScores[key] = 1.0
			supportedClaims++
		} else {
			subScores[key] = 0.0
			unsupportedClaims = append(unsupportedClaims, claim)
		}
	}

	// 计算分数
	score := float64(supportedClaims) / float64(len(claims))

	// 严格模式：任何编造都导致失败
	if e.strict && len(unsupportedClaims) > 0 {
		score = 0
	}

	passed := score >= e.threshold

	var reason string
	if len(unsupportedClaims) > 0 {
		reason = fmt.Sprintf("Found %d unsupported claims out of %d total claims: %v",
			len(unsupportedClaims), len(claims), unsupportedClaims)
	} else {
		reason = fmt.Sprintf("All %d claims are supported by the context", len(claims))
	}

	return &evaluate.EvalResult{
		Name:      e.Name(),
		Score:     score,
		Passed:    &passed,
		Reason:    reason,
		SubScores: subScores,
		Details: map[string]any{
			"total_claims":       len(claims),
			"supported_claims":   supportedClaims,
			"unsupported_claims": unsupportedClaims,
			"threshold":          e.threshold,
			"strict_mode":        e.strict,
		},
		Duration: time.Since(start),
	}, nil
}

func (e *FaithfulnessEvaluator) extractClaims(ctx context.Context, response string) ([]string, error) {
	prompt := fmt.Sprintf(`Extract all factual claims from the following response.
A claim is a statement that can be verified as true or false.
Skip opinions, questions, and general statements.

Response:
%s

List each claim on a separate line, starting with "- ".
If there are no verifiable claims, respond with "No claims found."`, truncateText(response, 2000))

	result, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return nil, err
	}

	if strings.Contains(result, "No claims found") {
		return nil, nil
	}

	var claims []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			claim := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			if claim != "" {
				claims = append(claims, claim)
			}
		}
	}

	return claims, nil
}

func (e *FaithfulnessEvaluator) verifyClaim(ctx context.Context, claim, context string) (bool, error) {
	prompt := fmt.Sprintf(`Determine if the following claim is supported by the provided context.

Context:
%s

Claim: %s

Is this claim supported by the context?
- Answer "YES" if the claim can be directly verified from the context
- Answer "NO" if the claim contains information not found in the context or contradicts it
- Answer "PARTIAL" if the claim is partially supported

Respond with: YES, NO, or PARTIAL
Then explain briefly.`, truncateText(context, 3000), claim)

	result, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return false, err
	}

	result = strings.ToUpper(strings.TrimSpace(result))
	return strings.HasPrefix(result, "YES") || strings.HasPrefix(result, "PARTIAL"), nil
}

var _ evaluate.Evaluator = (*FaithfulnessEvaluator)(nil)

// ============== HallucinationEvaluator ==============

// HallucinationEvaluator 检测回答中的幻觉
// 与 Faithfulness 互补，专注于检测明显的编造
type HallucinationEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
}

// HallucinationOption 选项
type HallucinationOption func(*HallucinationEvaluator)

// WithHallucinationThreshold 设置阈值
func WithHallucinationThreshold(threshold float64) HallucinationOption {
	return func(e *HallucinationEvaluator) {
		e.threshold = threshold
	}
}

// NewHallucinationEvaluator 创建幻觉检测评估器
func NewHallucinationEvaluator(llm evaluate.LLMJudge, opts ...HallucinationOption) *HallucinationEvaluator {
	e := &HallucinationEvaluator{
		llm:       llm,
		threshold: 0.8,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *HallucinationEvaluator) Name() string {
	return "hallucination"
}

// Description 返回评估器描述
func (e *HallucinationEvaluator) Description() string {
	return "Detects hallucinations in the response that are not supported by the context"
}

// RequiresLLM 返回是否需要 LLM
func (e *HallucinationEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行幻觉检测
func (e *HallucinationEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    1.0,
			Reason:   "Empty response, no hallucination possible",
			Duration: time.Since(start),
		}, nil
	}

	contextText := ""
	if len(input.Context) > 0 {
		contextText = strings.Join(input.Context, "\n\n")
	}

	prompt := fmt.Sprintf(`Analyze the following response for hallucinations (made-up or incorrect information).

%s

Response:
%s

Identify any statements that:
1. Are factually incorrect
2. Make claims not supported by the context (if provided)
3. Contain made-up details, names, numbers, or citations

Rate the response on a scale of 0-10:
- 0: Contains severe hallucinations
- 5: Contains minor inaccuracies
- 10: No hallucinations detected

Respond in the following format:
Score: [0-10]
Hallucinations: [List any found, or "None"]
Reason: [Brief explanation]`,
		func() string {
			if contextText != "" {
				return fmt.Sprintf("Context:\n%s", truncateText(contextText, 2000))
			}
			return "(No context provided)"
		}(),
		truncateText(input.Response, 1500))

	result, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return nil, err
	}

	score, reason := parseScoreResponse(result)
	normalizedScore := score / 10.0
	passed := normalizedScore >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  normalizedScore,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"threshold":    e.threshold,
			"has_context":  len(input.Context) > 0,
			"full_analysis": result,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*HallucinationEvaluator)(nil)
