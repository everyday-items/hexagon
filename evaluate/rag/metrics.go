// Package rag 提供 RAG 系统评估指标
//
// 本包实现了完整的 RAG 评估指标体系：
//   - Faithfulness（忠实度）：回答是否基于检索到的上下文
//   - Relevancy（相关性）：回答是否与问题相关
//   - Context Precision（上下文精度）：检索到的上下文是否精确
//   - Context Recall（上下文召回）：是否检索到所有相关上下文
//   - Answer Correctness（答案正确性）：答案是否正确
//   - Hallucination（幻觉检测）：是否包含虚构内容
//
// 设计借鉴：
//   - RAGAS: RAG 评估框架
//   - LlamaIndex: 评估指标
//   - TruLens: RAG 三角评估
//
// 使用示例：
//
//	evaluator := rag.NewEvaluator(llmProvider)
//	result, err := evaluator.Evaluate(ctx, &EvaluationInput{
//	    Question: "What is AI?",
//	    Answer: "AI is artificial intelligence...",
//	    Contexts: []string{"AI stands for..."},
//	})
package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ============== 错误定义 ==============

var (
	// ErrEvaluationFailed 评估失败
	ErrEvaluationFailed = errors.New("evaluation failed")

	// ErrNoLLMProvider 未提供 LLM Provider
	ErrNoLLMProvider = errors.New("no LLM provider")

	// ErrInvalidInput 无效输入
	ErrInvalidInput = errors.New("invalid evaluation input")
)

// ============== 评估输入输出 ==============

// EvaluationInput 评估输入
type EvaluationInput struct {
	// Question 用户问题
	Question string `json:"question"`

	// Answer 生成的回答
	Answer string `json:"answer"`

	// Contexts 检索到的上下文
	Contexts []string `json:"contexts"`

	// GroundTruth 真实答案（可选，用于答案正确性评估）
	GroundTruth string `json:"ground_truth,omitempty"`

	// GroundTruthContexts 真实相关上下文（可选，用于召回评估）
	GroundTruthContexts []string `json:"ground_truth_contexts,omitempty"`
}

// Validate 验证输入
func (input *EvaluationInput) Validate() error {
	if input.Question == "" {
		return fmt.Errorf("%w: question is required", ErrInvalidInput)
	}
	if input.Answer == "" {
		return fmt.Errorf("%w: answer is required", ErrInvalidInput)
	}
	return nil
}

// EvaluationResult 评估结果
type EvaluationResult struct {
	// Faithfulness 忠实度得分 (0-1)
	Faithfulness float64 `json:"faithfulness"`

	// Relevancy 相关性得分 (0-1)
	Relevancy float64 `json:"relevancy"`

	// ContextPrecision 上下文精度 (0-1)
	ContextPrecision float64 `json:"context_precision"`

	// ContextRecall 上下文召回 (0-1)
	ContextRecall float64 `json:"context_recall"`

	// AnswerCorrectness 答案正确性 (0-1)
	AnswerCorrectness float64 `json:"answer_correctness,omitempty"`

	// Hallucination 幻觉得分 (0-1，越低越好)
	Hallucination float64 `json:"hallucination"`

	// OverallScore 综合得分 (0-1)
	OverallScore float64 `json:"overall_score"`

	// Details 详细信息
	Details map[string]any `json:"details,omitempty"`
}

// CalculateOverall 计算综合得分
func (r *EvaluationResult) CalculateOverall(weights *MetricWeights) {
	if weights == nil {
		weights = DefaultMetricWeights()
	}

	total := weights.Faithfulness + weights.Relevancy + weights.ContextPrecision +
		weights.ContextRecall + weights.AnswerCorrectness

	r.OverallScore = (r.Faithfulness*weights.Faithfulness +
		r.Relevancy*weights.Relevancy +
		r.ContextPrecision*weights.ContextPrecision +
		r.ContextRecall*weights.ContextRecall +
		r.AnswerCorrectness*weights.AnswerCorrectness) / total
}

// MetricWeights 指标权重
type MetricWeights struct {
	Faithfulness      float64 `json:"faithfulness"`
	Relevancy         float64 `json:"relevancy"`
	ContextPrecision  float64 `json:"context_precision"`
	ContextRecall     float64 `json:"context_recall"`
	AnswerCorrectness float64 `json:"answer_correctness"`
}

// DefaultMetricWeights 默认权重
func DefaultMetricWeights() *MetricWeights {
	return &MetricWeights{
		Faithfulness:      0.25,
		Relevancy:         0.25,
		ContextPrecision:  0.2,
		ContextRecall:     0.2,
		AnswerCorrectness: 0.1,
	}
}

// ============== LLM Provider 接口 ==============

// LLMProvider LLM 提供者接口（简化版）
type LLMProvider interface {
	// Complete 执行补全
	Complete(ctx context.Context, prompt string) (string, error)
}

// ============== 评估器 ==============

// Evaluator RAG 评估器
type Evaluator struct {
	llm     LLMProvider
	weights *MetricWeights
	config  *EvaluatorConfig
}

// EvaluatorConfig 评估器配置
type EvaluatorConfig struct {
	// EnableDetailedAnalysis 启用详细分析
	EnableDetailedAnalysis bool

	// ParallelEvaluation 并行评估
	ParallelEvaluation bool

	// Timeout 超时时间（秒）
	Timeout int
}

// DefaultEvaluatorConfig 默认评估器配置
func DefaultEvaluatorConfig() *EvaluatorConfig {
	return &EvaluatorConfig{
		EnableDetailedAnalysis: true,
		ParallelEvaluation:     true,
		Timeout:                60,
	}
}

// NewEvaluator 创建评估器
func NewEvaluator(llm LLMProvider, config ...*EvaluatorConfig) *Evaluator {
	cfg := DefaultEvaluatorConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	return &Evaluator{
		llm:     llm,
		weights: DefaultMetricWeights(),
		config:  cfg,
	}
}

// WithWeights 设置权重
func (e *Evaluator) WithWeights(weights *MetricWeights) *Evaluator {
	e.weights = weights
	return e
}

// Evaluate 执行完整评估
func (e *Evaluator) Evaluate(ctx context.Context, input *EvaluationInput) (*EvaluationResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	result := &EvaluationResult{
		Details: make(map[string]any),
	}

	// 评估忠实度
	faithfulness, err := e.evaluateFaithfulness(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("faithfulness evaluation failed: %w", err)
	}
	result.Faithfulness = faithfulness

	// 评估相关性
	relevancy, err := e.evaluateRelevancy(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("relevancy evaluation failed: %w", err)
	}
	result.Relevancy = relevancy

	// 评估上下文精度
	precision, err := e.evaluateContextPrecision(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("context precision evaluation failed: %w", err)
	}
	result.ContextPrecision = precision

	// 评估上下文召回
	recall, err := e.evaluateContextRecall(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("context recall evaluation failed: %w", err)
	}
	result.ContextRecall = recall

	// 评估答案正确性（如果提供了真实答案）
	if input.GroundTruth != "" {
		correctness, err := e.evaluateAnswerCorrectness(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("answer correctness evaluation failed: %w", err)
		}
		result.AnswerCorrectness = correctness
	}

	// 评估幻觉
	hallucination, err := e.evaluateHallucination(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("hallucination evaluation failed: %w", err)
	}
	result.Hallucination = hallucination

	// 计算综合得分
	result.CalculateOverall(e.weights)

	return result, nil
}

// evaluateFaithfulness 评估忠实度
func (e *Evaluator) evaluateFaithfulness(ctx context.Context, input *EvaluationInput) (float64, error) {
	if e.llm == nil {
		// 使用基于规则的简单评估
		return e.simpleFaithfulness(input), nil
	}

	prompt := fmt.Sprintf(`Evaluate the faithfulness of the answer based on the given context.

Context:
%s

Answer:
%s

Faithfulness measures whether the answer is grounded in the context.
Score from 0 to 1, where:
- 1.0: Fully faithful, all claims are supported by context
- 0.0: Completely unfaithful, no claims are supported

Respond with ONLY a number between 0 and 1.`,
		strings.Join(input.Contexts, "\n\n"),
		input.Answer)

	return e.getLLMScore(ctx, prompt)
}

// evaluateRelevancy 评估相关性
func (e *Evaluator) evaluateRelevancy(ctx context.Context, input *EvaluationInput) (float64, error) {
	if e.llm == nil {
		return e.simpleRelevancy(input), nil
	}

	prompt := fmt.Sprintf(`Evaluate the relevancy of the answer to the question.

Question:
%s

Answer:
%s

Relevancy measures whether the answer addresses the question.
Score from 0 to 1, where:
- 1.0: Perfectly relevant, directly answers the question
- 0.0: Completely irrelevant

Respond with ONLY a number between 0 and 1.`,
		input.Question,
		input.Answer)

	return e.getLLMScore(ctx, prompt)
}

// evaluateContextPrecision 评估上下文精度
func (e *Evaluator) evaluateContextPrecision(ctx context.Context, input *EvaluationInput) (float64, error) {
	if len(input.Contexts) == 0 {
		return 0, nil
	}

	if e.llm == nil {
		return e.simpleContextPrecision(input), nil
	}

	prompt := fmt.Sprintf(`Evaluate the precision of the retrieved contexts for the question.

Question:
%s

Retrieved Contexts:
%s

Context Precision measures what proportion of retrieved contexts are relevant.
Score from 0 to 1, where:
- 1.0: All contexts are highly relevant
- 0.0: No contexts are relevant

Respond with ONLY a number between 0 and 1.`,
		input.Question,
		formatContexts(input.Contexts))

	return e.getLLMScore(ctx, prompt)
}

// evaluateContextRecall 评估上下文召回
func (e *Evaluator) evaluateContextRecall(ctx context.Context, input *EvaluationInput) (float64, error) {
	if len(input.GroundTruthContexts) == 0 {
		// 没有真实上下文，使用 LLM 估计
		if e.llm == nil {
			return 0.5, nil // 默认中等
		}

		prompt := fmt.Sprintf(`Estimate the context recall for answering this question.

Question:
%s

Answer:
%s

Retrieved Contexts:
%s

Context Recall measures whether all necessary information was retrieved.
Score from 0 to 1, where:
- 1.0: All necessary information is present in contexts
- 0.0: Critical information is missing

Respond with ONLY a number between 0 and 1.`,
			input.Question,
			input.Answer,
			formatContexts(input.Contexts))

		return e.getLLMScore(ctx, prompt)
	}

	// 有真实上下文，计算召回率
	return calculateRecall(input.Contexts, input.GroundTruthContexts), nil
}

// evaluateAnswerCorrectness 评估答案正确性
func (e *Evaluator) evaluateAnswerCorrectness(ctx context.Context, input *EvaluationInput) (float64, error) {
	if input.GroundTruth == "" {
		return 0, nil
	}

	if e.llm == nil {
		return e.simpleCorrectness(input), nil
	}

	prompt := fmt.Sprintf(`Evaluate the correctness of the generated answer compared to the ground truth.

Question:
%s

Generated Answer:
%s

Ground Truth:
%s

Answer Correctness measures semantic similarity and factual accuracy.
Score from 0 to 1, where:
- 1.0: Semantically identical, completely correct
- 0.0: Completely wrong or contradictory

Respond with ONLY a number between 0 and 1.`,
		input.Question,
		input.Answer,
		input.GroundTruth)

	return e.getLLMScore(ctx, prompt)
}

// evaluateHallucination 评估幻觉
func (e *Evaluator) evaluateHallucination(ctx context.Context, input *EvaluationInput) (float64, error) {
	if e.llm == nil {
		return 1 - e.simpleFaithfulness(input), nil
	}

	prompt := fmt.Sprintf(`Detect hallucination in the answer.

Context:
%s

Answer:
%s

Hallucination Score measures the degree of fabricated content not supported by context.
Score from 0 to 1, where:
- 0.0: No hallucination, everything is grounded
- 1.0: Severe hallucination, mostly fabricated

Respond with ONLY a number between 0 and 1.`,
		strings.Join(input.Contexts, "\n\n"),
		input.Answer)

	return e.getLLMScore(ctx, prompt)
}

// getLLMScore 获取 LLM 评分
func (e *Evaluator) getLLMScore(ctx context.Context, prompt string) (float64, error) {
	response, err := e.llm.Complete(ctx, prompt)
	if err != nil {
		return 0, err
	}

	// 解析得分
	response = strings.TrimSpace(response)
	var score float64
	_, err = fmt.Sscanf(response, "%f", &score)
	if err != nil {
		// 尝试提取数字
		for _, part := range strings.Fields(response) {
			if _, err := fmt.Sscanf(part, "%f", &score); err == nil {
				break
			}
		}
	}

	// 确保在 0-1 范围内
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score, nil
}

// ============== 简单评估方法（无 LLM）==============

// simpleFaithfulness 简单忠实度评估
func (e *Evaluator) simpleFaithfulness(input *EvaluationInput) float64 {
	if len(input.Contexts) == 0 {
		return 0
	}

	// 计算答案中的词汇有多少出现在上下文中
	answerWords := tokenize(input.Answer)
	contextWords := make(map[string]bool)
	for _, ctx := range input.Contexts {
		for _, word := range tokenize(ctx) {
			contextWords[word] = true
		}
	}

	matches := 0
	for _, word := range answerWords {
		if contextWords[word] {
			matches++
		}
	}

	if len(answerWords) == 0 {
		return 0
	}
	return float64(matches) / float64(len(answerWords))
}

// simpleRelevancy 简单相关性评估
func (e *Evaluator) simpleRelevancy(input *EvaluationInput) float64 {
	// 计算问题和答案的词汇重叠
	questionWords := tokenize(input.Question)
	answerWords := tokenize(input.Answer)

	questionSet := make(map[string]bool)
	for _, word := range questionWords {
		questionSet[word] = true
	}

	matches := 0
	for _, word := range answerWords {
		if questionSet[word] {
			matches++
		}
	}

	if len(questionWords) == 0 {
		return 0
	}
	return float64(matches) / float64(len(questionWords))
}

// simpleContextPrecision 简单上下文精度评估
func (e *Evaluator) simpleContextPrecision(input *EvaluationInput) float64 {
	if len(input.Contexts) == 0 {
		return 0
	}

	questionWords := tokenize(input.Question)
	questionSet := make(map[string]bool)
	for _, word := range questionWords {
		questionSet[word] = true
	}

	relevant := 0
	for _, ctx := range input.Contexts {
		ctxWords := tokenize(ctx)
		matches := 0
		for _, word := range ctxWords {
			if questionSet[word] {
				matches++
			}
		}
		if float64(matches)/float64(len(questionWords)+1) > 0.2 {
			relevant++
		}
	}

	return float64(relevant) / float64(len(input.Contexts))
}

// simpleCorrectness 简单正确性评估
func (e *Evaluator) simpleCorrectness(input *EvaluationInput) float64 {
	if input.GroundTruth == "" {
		return 0
	}

	// 计算答案和真实答案的词汇重叠
	answerWords := tokenize(input.Answer)
	truthWords := tokenize(input.GroundTruth)

	answerSet := make(map[string]bool)
	for _, word := range answerWords {
		answerSet[word] = true
	}

	truthSet := make(map[string]bool)
	for _, word := range truthWords {
		truthSet[word] = true
	}

	// Jaccard 相似度
	intersection := 0
	for word := range answerSet {
		if truthSet[word] {
			intersection++
		}
	}

	union := len(answerSet) + len(truthSet) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// ============== 辅助函数 ==============

// formatContexts 格式化上下文
func formatContexts(contexts []string) string {
	var parts []string
	for i, ctx := range contexts {
		parts = append(parts, fmt.Sprintf("[Context %d]: %s", i+1, ctx))
	}
	return strings.Join(parts, "\n\n")
}

// calculateRecall 计算召回率
func calculateRecall(retrieved, groundTruth []string) float64 {
	if len(groundTruth) == 0 {
		return 0
	}

	// 简化实现：基于文本相似度
	matches := 0
	for _, gt := range groundTruth {
		gtWords := make(map[string]bool)
		for _, word := range tokenize(gt) {
			gtWords[word] = true
		}

		for _, ret := range retrieved {
			retWords := tokenize(ret)
			overlap := 0
			for _, word := range retWords {
				if gtWords[word] {
					overlap++
				}
			}
			// 如果重叠超过 50%，认为是匹配
			if float64(overlap)/float64(len(gtWords)+1) > 0.5 {
				matches++
				break
			}
		}
	}

	return float64(matches) / float64(len(groundTruth))
}

// tokenize 分词
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// 简单分词：按空格和标点分割
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	// 过滤停用词
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"of": true, "in": true, "to": true, "for": true, "with": true,
		"on": true, "at": true, "by": true, "from": true, "as": true,
		"and": true, "or": true, "but": true, "not": true, "that": true,
		"this": true, "it": true, "its": true,
	}

	result := make([]string, 0)
	for _, word := range words {
		if len(word) > 1 && !stopWords[word] {
			result = append(result, word)
		}
	}

	return result
}

// ============== 批量评估 ==============

// BatchEvaluationResult 批量评估结果
type BatchEvaluationResult struct {
	// Results 各项结果
	Results []*EvaluationResult `json:"results"`

	// AverageScores 平均得分
	AverageScores *EvaluationResult `json:"average_scores"`

	// TotalSamples 总样本数
	TotalSamples int `json:"total_samples"`

	// SuccessCount 成功数
	SuccessCount int `json:"success_count"`

	// FailureCount 失败数
	FailureCount int `json:"failure_count"`
}

// EvaluateBatch 批量评估
func (e *Evaluator) EvaluateBatch(ctx context.Context, inputs []*EvaluationInput) (*BatchEvaluationResult, error) {
	result := &BatchEvaluationResult{
		Results:      make([]*EvaluationResult, len(inputs)),
		TotalSamples: len(inputs),
	}

	// 累积得分
	var totalFaithfulness, totalRelevancy, totalContextPrecision float64
	var totalContextRecall, totalAnswerCorrectness, totalHallucination float64

	for i, input := range inputs {
		evalResult, err := e.Evaluate(ctx, input)
		if err != nil {
			result.FailureCount++
			continue
		}

		result.Results[i] = evalResult
		result.SuccessCount++

		totalFaithfulness += evalResult.Faithfulness
		totalRelevancy += evalResult.Relevancy
		totalContextPrecision += evalResult.ContextPrecision
		totalContextRecall += evalResult.ContextRecall
		totalAnswerCorrectness += evalResult.AnswerCorrectness
		totalHallucination += evalResult.Hallucination
	}

	// 计算平均值
	if result.SuccessCount > 0 {
		n := float64(result.SuccessCount)
		result.AverageScores = &EvaluationResult{
			Faithfulness:      totalFaithfulness / n,
			Relevancy:         totalRelevancy / n,
			ContextPrecision:  totalContextPrecision / n,
			ContextRecall:     totalContextRecall / n,
			AnswerCorrectness: totalAnswerCorrectness / n,
			Hallucination:     totalHallucination / n,
		}
		result.AverageScores.CalculateOverall(e.weights)
	}

	return result, nil
}
