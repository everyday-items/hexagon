// Package metrics 提供 AI 系统的评估指标
package metrics

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== RelevanceEvaluator ==============

// RelevanceEvaluator 评估检索文档与查询的相关性
type RelevanceEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
}

// RelevanceOption RelevanceEvaluator 选项
type RelevanceOption func(*RelevanceEvaluator)

// WithRelevanceThreshold 设置通过阈值
func WithRelevanceThreshold(threshold float64) RelevanceOption {
	return func(e *RelevanceEvaluator) {
		e.threshold = threshold
	}
}

// NewRelevanceEvaluator 创建相关性评估器
func NewRelevanceEvaluator(llm evaluate.LLMJudge, opts ...RelevanceOption) *RelevanceEvaluator {
	e := &RelevanceEvaluator{
		llm:       llm,
		threshold: 0.7,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *RelevanceEvaluator) Name() string {
	return "relevance"
}

// Description 返回评估器描述
func (e *RelevanceEvaluator) Description() string {
	return "Evaluates how relevant the retrieved context is to the query"
}

// RequiresLLM 返回是否需要 LLM
func (e *RelevanceEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行相关性评估
func (e *RelevanceEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if len(input.Context) == 0 {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "No context provided for relevance evaluation",
			Duration: time.Since(start),
		}, nil
	}

	// 评估每个上下文片段的相关性
	var totalScore float64
	subScores := make(map[string]float64)
	reasons := []string{}

	for i, ctxText := range input.Context {
		score, reason, err := e.evaluateContext(ctx, ctxText, input.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate context %d: %w", i, err)
		}

		key := fmt.Sprintf("context_%d", i)
		subScores[key] = score
		totalScore += score
		reasons = append(reasons, fmt.Sprintf("Context %d: %.2f - %s", i+1, score, reason))
	}

	avgScore := totalScore / float64(len(input.Context))
	passed := avgScore >= e.threshold

	return &evaluate.EvalResult{
		Name:      e.Name(),
		Score:     avgScore,
		Passed:    &passed,
		Reason:    strings.Join(reasons, "\n"),
		SubScores: subScores,
		Details: map[string]any{
			"num_contexts": len(input.Context),
			"threshold":    e.threshold,
		},
		Duration: time.Since(start),
	}, nil
}

func (e *RelevanceEvaluator) evaluateContext(ctx context.Context, contextText, query string) (float64, string, error) {
	prompt := fmt.Sprintf(`You are evaluating the relevance of a retrieved context to a user query.

Query: %s

Context: %s

Rate the relevance on a scale of 0-10:
- 0: Completely irrelevant
- 5: Somewhat relevant, contains some related information
- 10: Highly relevant, directly addresses the query

Respond in the following format:
Score: [0-10]
Reason: [Brief explanation]`, query, truncateText(contextText, 2000))

	response, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return 0, "", err
	}

	score, reason := parseScoreResponse(response)
	return score / 10.0, reason, nil
}

var _ evaluate.Evaluator = (*RelevanceEvaluator)(nil)

// ============== ContextRelevanceEvaluator ==============

// ContextRelevanceEvaluator 评估上下文与回答的相关性
type ContextRelevanceEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
}

// ContextRelevanceOption 选项
type ContextRelevanceOption func(*ContextRelevanceEvaluator)

// WithContextRelevanceThreshold 设置阈值
func WithContextRelevanceThreshold(threshold float64) ContextRelevanceOption {
	return func(e *ContextRelevanceEvaluator) {
		e.threshold = threshold
	}
}

// NewContextRelevanceEvaluator 创建上下文相关性评估器
func NewContextRelevanceEvaluator(llm evaluate.LLMJudge, opts ...ContextRelevanceOption) *ContextRelevanceEvaluator {
	e := &ContextRelevanceEvaluator{
		llm:       llm,
		threshold: 0.7,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *ContextRelevanceEvaluator) Name() string {
	return "context_relevance"
}

// Description 返回评估器描述
func (e *ContextRelevanceEvaluator) Description() string {
	return "Evaluates how much of the context is used in generating the response"
}

// RequiresLLM 返回是否需要 LLM
func (e *ContextRelevanceEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行评估
func (e *ContextRelevanceEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if len(input.Context) == 0 || input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Missing context or response",
			Duration: time.Since(start),
		}, nil
	}

	combinedContext := strings.Join(input.Context, "\n\n")

	prompt := fmt.Sprintf(`You are evaluating how well a response utilizes the provided context.

Context:
%s

Response:
%s

Evaluate the following:
1. How much of the context information is used in the response?
2. Are there parts of the response that could not be derived from the context?

Rate on a scale of 0-10:
- 0: Response completely ignores the context
- 5: Response partially uses the context
- 10: Response fully utilizes relevant parts of the context

Respond in the following format:
Score: [0-10]
Reason: [Brief explanation]`, truncateText(combinedContext, 3000), truncateText(input.Response, 1000))

	response, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return nil, err
	}

	score, reason := parseScoreResponse(response)
	normalizedScore := score / 10.0
	passed := normalizedScore >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  normalizedScore,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"threshold": e.threshold,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*ContextRelevanceEvaluator)(nil)

// ============== 辅助函数 ==============

// parseScoreResponse 解析 LLM 的评分响应
func parseScoreResponse(response string) (float64, string) {
	// 尝试匹配 "Score: X" 格式
	scoreRegex := regexp.MustCompile(`(?i)score:\s*(\d+(?:\.\d+)?)`)
	reasonRegex := regexp.MustCompile(`(?i)reason:\s*(.+)`)

	scoreMatch := scoreRegex.FindStringSubmatch(response)
	reasonMatch := reasonRegex.FindStringSubmatch(response)

	var score float64
	var reason string

	if len(scoreMatch) > 1 {
		score, _ = strconv.ParseFloat(scoreMatch[1], 64)
	}

	if len(reasonMatch) > 1 {
		reason = strings.TrimSpace(reasonMatch[1])
	} else {
		reason = response
	}

	return score, reason
}

// truncateText 截断文本
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}
