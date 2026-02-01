// Package metrics 提供 AI 系统的评估指标
package metrics

import (
	"context"
	"time"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== LatencyEvaluator ==============

// LatencyEvaluator 评估系统延迟
type LatencyEvaluator struct {
	maxLatencyMs float64 // 最大可接受延迟（毫秒）
	targetLatencyMs float64 // 目标延迟（毫秒）
}

// LatencyOption LatencyEvaluator 选项
type LatencyOption func(*LatencyEvaluator)

// WithMaxLatency 设置最大延迟（毫秒）
func WithMaxLatency(ms float64) LatencyOption {
	return func(e *LatencyEvaluator) {
		e.maxLatencyMs = ms
	}
}

// WithTargetLatency 设置目标延迟（毫秒）
func WithTargetLatency(ms float64) LatencyOption {
	return func(e *LatencyEvaluator) {
		e.targetLatencyMs = ms
	}
}

// NewLatencyEvaluator 创建延迟评估器
func NewLatencyEvaluator(opts ...LatencyOption) *LatencyEvaluator {
	e := &LatencyEvaluator{
		maxLatencyMs:    5000,  // 5秒
		targetLatencyMs: 1000,  // 1秒
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *LatencyEvaluator) Name() string {
	return "latency"
}

// Description 返回评估器描述
func (e *LatencyEvaluator) Description() string {
	return "Evaluates response latency against target thresholds"
}

// RequiresLLM 返回是否需要 LLM
func (e *LatencyEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行延迟评估
func (e *LatencyEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Timing == nil {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "No timing information provided",
			Duration: time.Since(start),
		}, nil
	}

	latencyMs := float64(input.Timing.Duration.Milliseconds())

	// 计算分数
	var score float64
	var reason string

	if latencyMs <= e.targetLatencyMs {
		// 达到或超过目标延迟
		score = 1.0
		reason = "Excellent latency, within target"
	} else if latencyMs <= e.maxLatencyMs {
		// 在目标和最大之间，线性衰减
		score = 1.0 - (latencyMs-e.targetLatencyMs)/(e.maxLatencyMs-e.targetLatencyMs)
		reason = "Acceptable latency, above target but within maximum"
	} else {
		// 超过最大延迟
		score = 0
		reason = "Latency exceeds maximum threshold"
	}

	passed := latencyMs <= e.maxLatencyMs

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"latency_ms":     latencyMs,
			"target_ms":      e.targetLatencyMs,
			"max_ms":         e.maxLatencyMs,
			"ttfb_ms":        float64(input.Timing.TTFBDuration.Milliseconds()),
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*LatencyEvaluator)(nil)

// ============== CostEvaluator ==============

// CostEvaluator 评估系统成本
type CostEvaluator struct {
	maxCost    float64 // 最大可接受成本（美元）
	targetCost float64 // 目标成本（美元）
}

// CostOption CostEvaluator 选项
type CostOption func(*CostEvaluator)

// WithMaxCost 设置最大成本（美元）
func WithMaxCost(cost float64) CostOption {
	return func(e *CostEvaluator) {
		e.maxCost = cost
	}
}

// WithTargetCost 设置目标成本（美元）
func WithTargetCost(cost float64) CostOption {
	return func(e *CostEvaluator) {
		e.targetCost = cost
	}
}

// NewCostEvaluator 创建成本评估器
func NewCostEvaluator(opts ...CostOption) *CostEvaluator {
	e := &CostEvaluator{
		maxCost:    0.1,   // 10 美分
		targetCost: 0.01,  // 1 美分
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *CostEvaluator) Name() string {
	return "cost"
}

// Description 返回评估器描述
func (e *CostEvaluator) Description() string {
	return "Evaluates response cost against target thresholds"
}

// RequiresLLM 返回是否需要 LLM
func (e *CostEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行成本评估
func (e *CostEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Cost == nil {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    1.0, // 无成本信息，假设为 0 成本
			Reason:   "No cost information provided",
			Duration: time.Since(start),
		}, nil
	}

	cost := input.Cost.Cost

	// 计算分数
	var score float64
	var reason string

	if cost <= e.targetCost {
		score = 1.0
		reason = "Cost within target"
	} else if cost <= e.maxCost {
		score = 1.0 - (cost-e.targetCost)/(e.maxCost-e.targetCost)
		reason = "Cost above target but within maximum"
	} else {
		score = 0
		reason = "Cost exceeds maximum threshold"
	}

	passed := cost <= e.maxCost

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"cost_usd":        cost,
			"target_cost":     e.targetCost,
			"max_cost":        e.maxCost,
			"input_tokens":    input.Cost.InputTokens,
			"output_tokens":   input.Cost.OutputTokens,
			"total_tokens":    input.Cost.TotalTokens,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*CostEvaluator)(nil)

// ============== ThroughputEvaluator ==============

// ThroughputEvaluator 评估系统吞吐量
type ThroughputEvaluator struct {
	minThroughput    float64 // 最小吞吐量（tokens/秒）
	targetThroughput float64 // 目标吞吐量（tokens/秒）
}

// ThroughputOption 选项
type ThroughputOption func(*ThroughputEvaluator)

// WithMinThroughput 设置最小吞吐量
func WithMinThroughput(tps float64) ThroughputOption {
	return func(e *ThroughputEvaluator) {
		e.minThroughput = tps
	}
}

// WithTargetThroughput 设置目标吞吐量
func WithTargetThroughput(tps float64) ThroughputOption {
	return func(e *ThroughputEvaluator) {
		e.targetThroughput = tps
	}
}

// NewThroughputEvaluator 创建吞吐量评估器
func NewThroughputEvaluator(opts ...ThroughputOption) *ThroughputEvaluator {
	e := &ThroughputEvaluator{
		minThroughput:    10,   // 10 tokens/秒
		targetThroughput: 50,   // 50 tokens/秒
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *ThroughputEvaluator) Name() string {
	return "throughput"
}

// Description 返回评估器描述
func (e *ThroughputEvaluator) Description() string {
	return "Evaluates response generation throughput (tokens per second)"
}

// RequiresLLM 返回是否需要 LLM
func (e *ThroughputEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行吞吐量评估
func (e *ThroughputEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	if input.Timing == nil || input.Cost == nil {
		return &evaluate.EvalResult{
			Name:     e.Name(),
			Score:    0,
			Reason:   "Missing timing or cost information for throughput calculation",
			Duration: time.Since(start),
		}, nil
	}

	// 计算吞吐量
	durationSec := input.Timing.Duration.Seconds()
	if durationSec == 0 {
		durationSec = 0.001 // 避免除零
	}

	outputTokens := float64(input.Cost.OutputTokens)
	throughput := outputTokens / durationSec

	// 计算分数
	var score float64
	var reason string

	if throughput >= e.targetThroughput {
		score = 1.0
		reason = "Excellent throughput"
	} else if throughput >= e.minThroughput {
		score = (throughput - e.minThroughput) / (e.targetThroughput - e.minThroughput)
		reason = "Acceptable throughput"
	} else {
		score = 0
		reason = "Throughput below minimum threshold"
	}

	passed := throughput >= e.minThroughput

	return &evaluate.EvalResult{
		Name:   e.Name(),
		Score:  score,
		Passed: &passed,
		Reason: reason,
		Details: map[string]any{
			"throughput_tps":    throughput,
			"target_tps":        e.targetThroughput,
			"min_tps":           e.minThroughput,
			"output_tokens":     input.Cost.OutputTokens,
			"duration_sec":      durationSec,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*ThroughputEvaluator)(nil)

// ============== CompositePerformanceEvaluator ==============

// CompositePerformanceEvaluator 综合性能评估器
// 结合延迟、成本、吞吐量等指标
type CompositePerformanceEvaluator struct {
	latencyWeight    float64
	costWeight       float64
	throughputWeight float64
	latencyEval      *LatencyEvaluator
	costEval         *CostEvaluator
	throughputEval   *ThroughputEvaluator
}

// CompositePerformanceOption 选项
type CompositePerformanceOption func(*CompositePerformanceEvaluator)

// WithPerformanceWeights 设置权重
func WithPerformanceWeights(latency, cost, throughput float64) CompositePerformanceOption {
	return func(e *CompositePerformanceEvaluator) {
		e.latencyWeight = latency
		e.costWeight = cost
		e.throughputWeight = throughput
	}
}

// NewCompositePerformanceEvaluator 创建综合性能评估器
func NewCompositePerformanceEvaluator(opts ...CompositePerformanceOption) *CompositePerformanceEvaluator {
	e := &CompositePerformanceEvaluator{
		latencyWeight:    0.4,
		costWeight:       0.3,
		throughputWeight: 0.3,
		latencyEval:      NewLatencyEvaluator(),
		costEval:         NewCostEvaluator(),
		throughputEval:   NewThroughputEvaluator(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Name 返回评估器名称
func (e *CompositePerformanceEvaluator) Name() string {
	return "performance"
}

// Description 返回评估器描述
func (e *CompositePerformanceEvaluator) Description() string {
	return "Composite performance evaluation combining latency, cost, and throughput"
}

// RequiresLLM 返回是否需要 LLM
func (e *CompositePerformanceEvaluator) RequiresLLM() bool {
	return false
}

// Evaluate 执行综合性能评估
func (e *CompositePerformanceEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	start := time.Now()

	subScores := make(map[string]float64)
	var weightedScore float64
	totalWeight := 0.0

	// 延迟评估
	if input.Timing != nil {
		latencyResult, _ := e.latencyEval.Evaluate(ctx, input)
		if latencyResult != nil {
			subScores["latency"] = latencyResult.Score
			weightedScore += latencyResult.Score * e.latencyWeight
			totalWeight += e.latencyWeight
		}
	}

	// 成本评估
	if input.Cost != nil {
		costResult, _ := e.costEval.Evaluate(ctx, input)
		if costResult != nil {
			subScores["cost"] = costResult.Score
			weightedScore += costResult.Score * e.costWeight
			totalWeight += e.costWeight
		}
	}

	// 吞吐量评估
	if input.Timing != nil && input.Cost != nil {
		throughputResult, _ := e.throughputEval.Evaluate(ctx, input)
		if throughputResult != nil {
			subScores["throughput"] = throughputResult.Score
			weightedScore += throughputResult.Score * e.throughputWeight
			totalWeight += e.throughputWeight
		}
	}

	// 计算最终分数
	var finalScore float64
	if totalWeight > 0 {
		finalScore = weightedScore / totalWeight
	}

	return &evaluate.EvalResult{
		Name:      e.Name(),
		Score:     finalScore,
		SubScores: subScores,
		Details: map[string]any{
			"latency_weight":    e.latencyWeight,
			"cost_weight":       e.costWeight,
			"throughput_weight": e.throughputWeight,
		},
		Duration: time.Since(start),
	}, nil
}

var _ evaluate.Evaluator = (*CompositePerformanceEvaluator)(nil)
