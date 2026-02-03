// Package metrics 提供 NLP 评估指标
package metrics

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== BLEU Score ==============

// BLEUEvaluator BLEU (Bilingual Evaluation Understudy) 评估器
//
// 主要用于机器翻译质量评估，通过计算 n-gram 精确度来衡量
// 生成文本与参考文本的相似程度。
type BLEUEvaluator struct {
	maxN      int     // 最大 n-gram 长度
	threshold float64 // 通过阈值
}

// BLEUOption BLEU 评估器选项
type BLEUOption func(*BLEUEvaluator)

// WithBLEUMaxN 设置最大 n-gram 长度
func WithBLEUMaxN(maxN int) BLEUOption {
	return func(e *BLEUEvaluator) {
		e.maxN = maxN
	}
}

// WithBLEUThreshold 设置通过阈值
func WithBLEUThreshold(threshold float64) BLEUOption {
	return func(e *BLEUEvaluator) {
		e.threshold = threshold
	}
}

// NewBLEUEvaluator 创建 BLEU 评估器
func NewBLEUEvaluator(opts ...BLEUOption) *BLEUEvaluator {
	e := &BLEUEvaluator{
		maxN:      4, // 默认 BLEU-4
		threshold: 0.3,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *BLEUEvaluator) Name() string {
	return "bleu"
}

// Description 返回评估器描述
func (e *BLEUEvaluator) Description() string {
	return "Evaluates text quality using BLEU score (n-gram precision)"
}

// RequiresLLM 返回是否需要 LLM
func (e *BLEUEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行 BLEU 评估
func (e *BLEUEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Reference == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "No reference text provided",
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

	// 计算 BLEU 分数
	score, details := e.calculateBLEU(input.Response, input.Reference)

	passed := score >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: fmt.Sprintf("BLEU-%d score: %.4f", e.maxN, score),
		Details: map[string]any{
			"bleu_score":  score,
			"max_n":       e.maxN,
			"precision_1": details["precision_1"],
			"precision_2": details["precision_2"],
			"precision_3": details["precision_3"],
			"precision_4": details["precision_4"],
			"bp":          details["bp"], // brevity penalty
		},
		Duration: time.Since(start),
	}, nil
}

// calculateBLEU 计算 BLEU 分数
func (e *BLEUEvaluator) calculateBLEU(candidate, reference string) (float64, map[string]float64) {
	candidateTokens := tokenize(candidate)
	referenceTokens := tokenize(reference)

	details := make(map[string]float64)

	// 计算各个 n-gram 的精确度
	var precisions []float64
	for n := 1; n <= e.maxN; n++ {
		precision := e.calculateNGramPrecision(candidateTokens, referenceTokens, n)
		precisions = append(precisions, precision)
		details[fmt.Sprintf("precision_%d", n)] = precision
	}

	// 计算几何平均
	geometricMean := 1.0
	for _, p := range precisions {
		if p == 0 {
			return 0, details
		}
		geometricMean *= p
	}
	geometricMean = math.Pow(geometricMean, 1.0/float64(len(precisions)))

	// 计算简短惩罚 (Brevity Penalty)
	bp := e.calculateBrevityPenalty(len(candidateTokens), len(referenceTokens))
	details["bp"] = bp

	bleuScore := bp * geometricMean
	return bleuScore, details
}

// calculateNGramPrecision 计算 n-gram 精确度
func (e *BLEUEvaluator) calculateNGramPrecision(candidate, reference []string, n int) float64 {
	if len(candidate) < n {
		return 0
	}

	// 生成 n-grams
	candidateNGrams := generateNGrams(candidate, n)
	referenceNGrams := generateNGrams(reference, n)

	// 计算匹配的 n-grams
	matched := 0
	refCounts := make(map[string]int)
	for _, ngram := range referenceNGrams {
		refCounts[ngram]++
	}

	for _, ngram := range candidateNGrams {
		if count, exists := refCounts[ngram]; exists && count > 0 {
			matched++
			refCounts[ngram]--
		}
	}

	if len(candidateNGrams) == 0 {
		return 0
	}

	return float64(matched) / float64(len(candidateNGrams))
}

// calculateBrevityPenalty 计算简短惩罚
func (e *BLEUEvaluator) calculateBrevityPenalty(candidateLen, referenceLen int) float64 {
	if candidateLen >= referenceLen {
		return 1.0
	}
	return math.Exp(1.0 - float64(referenceLen)/float64(candidateLen))
}

var _ evaluate.Evaluator = (*BLEUEvaluator)(nil)

// ============== ROUGE Score ==============

// ROUGEEvaluator ROUGE (Recall-Oriented Understudy for Gisting Evaluation) 评估器
//
// 主要用于文本摘要质量评估，通过计算 n-gram 召回率来衡量
// 生成摘要与参考摘要的覆盖程度。
type ROUGEEvaluator struct {
	variant   string  // ROUGE 变体: rouge-1, rouge-2, rouge-l
	threshold float64 // 通过阈值
}

// ROUGEOption ROUGE 评估器选项
type ROUGEOption func(*ROUGEEvaluator)

// WithROUGEVariant 设置 ROUGE 变体
func WithROUGEVariant(variant string) ROUGEOption {
	return func(e *ROUGEEvaluator) {
		e.variant = variant
	}
}

// WithROUGEThreshold 设置通过阈值
func WithROUGEThreshold(threshold float64) ROUGEOption {
	return func(e *ROUGEEvaluator) {
		e.threshold = threshold
	}
}

// NewROUGEEvaluator 创建 ROUGE 评估器
func NewROUGEEvaluator(opts ...ROUGEOption) *ROUGEEvaluator {
	e := &ROUGEEvaluator{
		variant:   "rouge-1", // 默认 ROUGE-1
		threshold: 0.3,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *ROUGEEvaluator) Name() string {
	return "rouge"
}

// Description 返回评估器描述
func (e *ROUGEEvaluator) Description() string {
	return "Evaluates summary quality using ROUGE score (n-gram recall)"
}

// RequiresLLM 返回是否需要 LLM
func (e *ROUGEEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行 ROUGE 评估
func (e *ROUGEEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Reference == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "No reference text provided",
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

	// 根据变体计算分数
	var score float64
	var details map[string]float64

	switch e.variant {
	case "rouge-1":
		score, details = e.calculateROUGEN(input.Response, input.Reference, 1)
	case "rouge-2":
		score, details = e.calculateROUGEN(input.Response, input.Reference, 2)
	case "rouge-l":
		score, details = e.calculateROUGEL(input.Response, input.Reference)
	default:
		score, details = e.calculateROUGEN(input.Response, input.Reference, 1)
	}

	passed := score >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: fmt.Sprintf("%s F1 score: %.4f", strings.ToUpper(e.variant), score),
		Details: map[string]any{
			"variant":   e.variant,
			"precision": details["precision"],
			"recall":    details["recall"],
			"f1":        details["f1"],
		},
		Duration: time.Since(start),
	}, nil
}

// calculateROUGEN 计算 ROUGE-N 分数
func (e *ROUGEEvaluator) calculateROUGEN(candidate, reference string, n int) (float64, map[string]float64) {
	candidateTokens := tokenize(candidate)
	referenceTokens := tokenize(reference)

	candidateNGrams := generateNGrams(candidateTokens, n)
	referenceNGrams := generateNGrams(referenceTokens, n)

	if len(referenceNGrams) == 0 {
		return 0, map[string]float64{"precision": 0, "recall": 0, "f1": 0}
	}

	// 计算匹配的 n-grams
	matched := 0
	refCounts := make(map[string]int)
	for _, ngram := range referenceNGrams {
		refCounts[ngram]++
	}

	for _, ngram := range candidateNGrams {
		if count, exists := refCounts[ngram]; exists && count > 0 {
			matched++
			refCounts[ngram]--
		}
	}

	recall := float64(matched) / float64(len(referenceNGrams))

	precision := 0.0
	if len(candidateNGrams) > 0 {
		precision = float64(matched) / float64(len(candidateNGrams))
	}

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	return f1, map[string]float64{
		"precision": precision,
		"recall":    recall,
		"f1":        f1,
	}
}

// calculateROUGEL 计算 ROUGE-L 分数（最长公共子序列）
func (e *ROUGEEvaluator) calculateROUGEL(candidate, reference string) (float64, map[string]float64) {
	candidateTokens := tokenize(candidate)
	referenceTokens := tokenize(reference)

	lcsLen := longestCommonSubsequence(candidateTokens, referenceTokens)

	if len(referenceTokens) == 0 {
		return 0, map[string]float64{"precision": 0, "recall": 0, "f1": 0}
	}

	recall := float64(lcsLen) / float64(len(referenceTokens))

	precision := 0.0
	if len(candidateTokens) > 0 {
		precision = float64(lcsLen) / float64(len(candidateTokens))
	}

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	return f1, map[string]float64{
		"precision": precision,
		"recall":    recall,
		"f1":        f1,
	}
}

var _ evaluate.Evaluator = (*ROUGEEvaluator)(nil)

// ============== F1 Score Evaluator ==============

// F1Evaluator F1 分数评估器
//
// 用于分类任务的准确性评估，计算精确率和召回率的调和平均。
type F1Evaluator struct {
	threshold float64
}

// F1Option F1 评估器选项
type F1Option func(*F1Evaluator)

// WithF1Threshold 设置通过阈值
func WithF1Threshold(threshold float64) F1Option {
	return func(e *F1Evaluator) {
		e.threshold = threshold
	}
}

// NewF1Evaluator 创建 F1 评估器
func NewF1Evaluator(opts ...F1Option) *F1Evaluator {
	e := &F1Evaluator{
		threshold: 0.7,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *F1Evaluator) Name() string {
	return "f1_score"
}

// Description 返回评估器描述
func (e *F1Evaluator) Description() string {
	return "Evaluates classification accuracy using F1 score"
}

// RequiresLLM 返回是否需要 LLM
func (e *F1Evaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行 F1 评估
func (e *F1Evaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Reference == "" || input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Missing reference or response",
			Duration: time.Since(start),
		}, nil
	}

	// 分词并计算 F1
	candidateTokens := tokenize(input.Response)
	referenceTokens := tokenize(input.Reference)

	// 计算交集
	candidateSet := make(map[string]bool)
	for _, token := range candidateTokens {
		candidateSet[token] = true
	}

	referenceSet := make(map[string]bool)
	for _, token := range referenceTokens {
		referenceSet[token] = true
	}

	// 计算 TP (True Positive)
	tp := 0
	for token := range candidateSet {
		if referenceSet[token] {
			tp++
		}
	}

	// 计算精确率和召回率
	precision := 0.0
	if len(candidateSet) > 0 {
		precision = float64(tp) / float64(len(candidateSet))
	}

	recall := 0.0
	if len(referenceSet) > 0 {
		recall = float64(tp) / float64(len(referenceSet))
	}

	// 计算 F1
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	passed := f1 >= e.threshold

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  f1,
		Passed: &passed,
		Reason: fmt.Sprintf("F1: %.4f (Precision: %.4f, Recall: %.4f)", f1, precision, recall),
		Details: map[string]any{
			"precision": precision,
			"recall":    recall,
			"f1":        f1,
			"tp":        tp,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*F1Evaluator)(nil)

// ============== Coherence Evaluator ==============

// CoherenceEvaluator 连贯性评估器
//
// 评估文本的逻辑连贯性和流畅度。
type CoherenceEvaluator struct {
	llm       evaluate.LLMJudge
	threshold float64
}

// CoherenceOption 连贯性评估器选项
type CoherenceOption func(*CoherenceEvaluator)

// WithCoherenceThreshold 设置通过阈值
func WithCoherenceThreshold(threshold float64) CoherenceOption {
	return func(e *CoherenceEvaluator) {
		e.threshold = threshold
	}
}

// NewCoherenceEvaluator 创建连贯性评估器
func NewCoherenceEvaluator(llm evaluate.LLMJudge, opts ...CoherenceOption) *CoherenceEvaluator {
	e := &CoherenceEvaluator{
		llm:       llm,
		threshold: 0.7,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *CoherenceEvaluator) Name() string {
	return "coherence"
}

// Description 返回评估器描述
func (e *CoherenceEvaluator) Description() string {
	return "Evaluates logical coherence and fluency of the text"
}

// RequiresLLM 返回是否需要 LLM
func (e *CoherenceEvaluator) RequiresLLM() bool {
	return true
}

// Evaluate 执行连贯性评估
func (e *CoherenceEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Response == "" {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Empty response",
			Duration: time.Since(start),
		}, nil
	}

	prompt := fmt.Sprintf(`Evaluate the coherence and logical flow of the following text.

Text:
%s

Consider:
1. Does the text flow logically from one point to the next?
2. Are ideas connected smoothly?
3. Is there a clear structure?
4. Are there any contradictions or logical gaps?

Rate on a scale of 0-10:
- 0: Incoherent, no logical flow
- 5: Somewhat coherent, but with some gaps
- 10: Highly coherent, excellent logical flow

Respond in the following format:
Score: [0-10]
Reason: [Brief explanation]`, truncateText(input.Response, 2000))

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
			"threshold": e.threshold,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*CoherenceEvaluator)(nil)

// ============== 辅助函数 ==============

// tokenize 分词
//
// 优化版本：预分配容量，减少内存分配
func tokenize(text string) []string {
	text = strings.ToLower(text)

	// 预估token数量（平均词长5个字符）
	estimatedTokens := len(text) / 5
	if estimatedTokens == 0 {
		estimatedTokens = 1
	}
	tokens := make([]string, 0, estimatedTokens)

	var current strings.Builder
	current.Grow(20) // 预分配builder容量

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// generateNGrams 生成 n-grams
func generateNGrams(tokens []string, n int) []string {
	if len(tokens) < n {
		return []string{}
	}

	ngrams := make([]string, 0, len(tokens)-n+1)
	for i := 0; i <= len(tokens)-n; i++ {
		ngram := strings.Join(tokens[i:i+n], " ")
		ngrams = append(ngrams, ngram)
	}

	return ngrams
}

// longestCommonSubsequence 计算最长公共子序列长度
//
// 优化版本：使用滚动数组，空间复杂度从 O(m*n) 降到 O(n)
func longestCommonSubsequence(seq1, seq2 []string) int {
	m, n := len(seq1), len(seq2)
	if m == 0 || n == 0 {
		return 0
	}

	// 确保 n 是较小的维度
	if m < n {
		seq1, seq2 = seq2, seq1
		m, n = n, m
	}

	// 只需要两行：当前行和上一行
	prev := make([]int, n+1)
	curr := make([]int, n+1)

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if seq1[i-1] == seq2[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = max(prev[j], curr[j-1])
			}
		}
		// 交换prev和curr
		prev, curr = curr, prev
	}

	return prev[n]
}

// max 返回两个整数中的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
