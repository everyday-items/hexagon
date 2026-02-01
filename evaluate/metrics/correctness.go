// Package metrics 提供 AI 系统的评估指标
package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== CorrectnessEvaluator ==============

// CorrectnessEvaluator 评估回答的正确性
// 通过与参考答案对比来评估
type CorrectnessEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
	mode      CorrectnessMode
}

// CorrectnessMode 正确性评估模式
type CorrectnessMode string

const (
	// CorrectnessModeExact 精确匹配
	CorrectnessModeExact CorrectnessMode = "exact"
	// CorrectnessModeSemanticSimilarity 语义相似度
	CorrectnessModeSemanticSimilarity CorrectnessMode = "semantic"
	// CorrectnessModeFactual 事实正确性
	CorrectnessModeFactual CorrectnessMode = "factual"
)

// CorrectnessOption CorrectnessEvaluator 选项
type CorrectnessOption func(*CorrectnessEvaluator)

// WithCorrectnessThreshold 设置通过阈值
func WithCorrectnessThreshold(threshold float64) CorrectnessOption {
	return func(e *CorrectnessEvaluator) {
		e.threshold = threshold
	}
}

// WithCorrectnessMode 设置评估模式
func WithCorrectnessMode(mode CorrectnessMode) CorrectnessOption {
	return func(e *CorrectnessEvaluator) {
		e.mode = mode
	}
}

// NewCorrectnessEvaluator 创建正确性评估器
func NewCorrectnessEvaluator(llm evaluate.LLMJudge, opts ...CorrectnessOption) *CorrectnessEvaluator {
	e := &CorrectnessEvaluator{
		llm:       llm,
		threshold: 0.7,
		mode:      CorrectnessModeSemanticSimilarity,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *CorrectnessEvaluator) Name() string {
	return "correctness"
}

// Description 返回评估器描述
func (e *CorrectnessEvaluator) Description() string {
	return "Evaluates whether the response is correct compared to the reference answer"
}

// RequiresLLM 返回是否需要 LLM
func (e *CorrectnessEvaluator) RequiresLLM() bool {
	return e.mode != CorrectnessModeExact
}

// Evaluate 执行正确性评估
func (e *CorrectnessEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Reference == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "No reference answer provided for correctness evaluation",
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

	var score float64
	var reason string
	var err error

	switch e.mode {
	case CorrectnessModeExact:
		score, reason = e.evaluateExact(input.Response, input.Reference)
	case CorrectnessModeSemanticSimilarity:
		score, reason, err = e.evaluateSemantic(ctx, input.Response, input.Reference)
	case CorrectnessModeFactual:
		score, reason, err = e.evaluateFactual(ctx, input.Response, input.Reference, input.Query)
	default:
		score, reason, err = e.evaluateSemantic(ctx, input.Response, input.Reference)
	}

	if err != nil {
		return nil, err
	}

	passed := score >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"mode":      string(e.mode),
			"threshold": e.threshold,
		},
		Duration: time.Since(start),
	}, nil
}

func (e *CorrectnessEvaluator) evaluateExact(response, reference string) (float64, string) {
	response = strings.TrimSpace(strings.ToLower(response))
	reference = strings.TrimSpace(strings.ToLower(reference))

	if response == reference {
		return 1.0, "Exact match"
	}

	// 计算简单的相似度（基于公共词）
	responseWords := strings.Fields(response)
	referenceWords := strings.Fields(reference)

	if len(referenceWords) == 0 {
		return 0, "Empty reference"
	}

	common := 0
	refSet := make(map[string]bool)
	for _, w := range referenceWords {
		refSet[w] = true
	}

	for _, w := range responseWords {
		if refSet[w] {
			common++
		}
	}

	precision := float64(common) / float64(len(responseWords))
	recall := float64(common) / float64(len(referenceWords))

	if precision+recall == 0 {
		return 0, "No common words"
	}

	// F1 分数
	f1 := 2 * precision * recall / (precision + recall)
	return f1, fmt.Sprintf("F1 score based on word overlap: precision=%.2f, recall=%.2f", precision, recall)
}

func (e *CorrectnessEvaluator) evaluateSemantic(ctx context.Context, response, reference string) (float64, string, error) {
	prompt := fmt.Sprintf(`Compare the following response with the reference answer and evaluate semantic similarity.

Reference Answer:
%s

Actual Response:
%s

Evaluate how semantically similar the response is to the reference:
- Consider meaning, not just exact wording
- Check if the key information is preserved
- Check if there are any contradictions

Rate on a scale of 0-10:
- 0: Completely different meaning
- 5: Partially similar, some key information missing or wrong
- 10: Semantically equivalent

Respond in the following format:
Score: [0-10]
Reason: [Brief explanation]`, truncateText(reference, 1500), truncateText(response, 1500))

	result, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return 0, "", err
	}

	score, reason := parseScoreResponse(result)
	return score / 10.0, reason, nil
}

func (e *CorrectnessEvaluator) evaluateFactual(ctx context.Context, response, reference, query string) (float64, string, error) {
	prompt := fmt.Sprintf(`Evaluate the factual correctness of a response compared to the reference answer.

Question: %s

Reference Answer:
%s

Actual Response:
%s

Evaluate:
1. Are the facts in the response correct?
2. Does the response answer the question correctly?
3. Are there any factual errors or omissions?

Rate on a scale of 0-10:
- 0: Factually incorrect
- 5: Partially correct, some errors or missing information
- 10: Fully correct

Respond in the following format:
Score: [0-10]
Errors: [List any factual errors, or "None"]
Reason: [Brief explanation]`, query, truncateText(reference, 1500), truncateText(response, 1500))

	result, err := e.llm.Judge(ctx, prompt)
	if err != nil {
		return 0, "", err
	}

	score, reason := parseScoreResponse(result)
	return score / 10.0, reason, nil
}

var _ evaluate.Evaluator = (*CorrectnessEvaluator)(nil)

// ============== AnswerQualityEvaluator ==============

// AnswerQualityEvaluator 评估回答的整体质量
type AnswerQualityEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
	criteria  []string
}

// AnswerQualityOption 选项
type AnswerQualityOption func(*AnswerQualityEvaluator)

// WithAnswerQualityThreshold 设置阈值
func WithAnswerQualityThreshold(threshold float64) AnswerQualityOption {
	return func(e *AnswerQualityEvaluator) {
		e.threshold = threshold
	}
}

// WithAnswerQualityCriteria 设置评估标准
func WithAnswerQualityCriteria(criteria []string) AnswerQualityOption {
	return func(e *AnswerQualityEvaluator) {
		e.criteria = criteria
	}
}

// NewAnswerQualityEvaluator 创建回答质量评估器
func NewAnswerQualityEvaluator(llm evaluate.LLMJudge, opts ...AnswerQualityOption) *AnswerQualityEvaluator {
	e := &AnswerQualityEvaluator{
		llm:       llm,
		threshold: 0.7,
		criteria: []string{
			"Completeness: Does the answer fully address the question?",
			"Clarity: Is the answer clear and easy to understand?",
			"Conciseness: Is the answer appropriately concise without unnecessary information?",
			"Accuracy: Are the statements accurate and well-supported?",
		},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *AnswerQualityEvaluator) Name() string {
	return "answer_quality"
}

// Description 返回评估器描述
func (e *AnswerQualityEvaluator) Description() string {
	return "Evaluates the overall quality of the response"
}

// RequiresLLM 返回是否需要 LLM
func (e *AnswerQualityEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行质量评估
func (e *AnswerQualityEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Empty response",
			Duration: time.Since(start),
		}, nil
	}

	criteriaText := strings.Join(e.criteria, "\n")

	prompt := fmt.Sprintf(`Evaluate the quality of the following response to the given query.

Query: %s

Response:
%s

Evaluate based on these criteria:
%s

For each criterion, give a score from 0-10.
Then provide an overall score (0-10).

Respond in the following format:
[Criterion 1]: [score]
[Criterion 2]: [score]
...
Overall Score: [0-10]
Reason: [Brief overall assessment]`, input.Query, truncateText(input.Response, 2000), criteriaText)

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
			"criteria":      e.criteria,
			"threshold":     e.threshold,
			"full_analysis": result,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*AnswerQualityEvaluator)(nil)
