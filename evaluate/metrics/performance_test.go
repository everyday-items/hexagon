package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/hexagon-codes/hexagon/evaluate"
)

// ============== LatencyEvaluator 测试 ==============

// TestLatencyEvaluator 测试延迟评估器
func TestLatencyEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewLatencyEvaluator()

		if eval.Name() != "latency" {
			t.Errorf("Name() = %s, want latency", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		eval := NewLatencyEvaluator(
			WithMaxLatency(10000),
			WithTargetLatency(2000),
		)

		if eval.maxLatencyMs != 10000 {
			t.Errorf("maxLatencyMs = %f, want 10000", eval.maxLatencyMs)
		}
		if eval.targetLatencyMs != 2000 {
			t.Errorf("targetLatencyMs = %f, want 2000", eval.targetLatencyMs)
		}
	})

	t.Run("正常延迟", func(t *testing.T) {
		eval := NewLatencyEvaluator(
			WithMaxLatency(5000),
			WithTargetLatency(1000),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration:     500 * time.Millisecond, // 500ms，低于目标
				TTFBDuration: 100 * time.Millisecond,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 低于目标延迟，满分
		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("低于最大延迟应该通过")
		}
	})

	t.Run("超限延迟", func(t *testing.T) {
		eval := NewLatencyEvaluator(
			WithMaxLatency(5000),
			WithTargetLatency(1000),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 6 * time.Second, // 6000ms，超过最大延迟
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("超限延迟分数应为 0，实际 %f", result.Score)
		}

		if result.Passed == nil || *result.Passed {
			t.Error("超过最大延迟不应通过")
		}
	})

	t.Run("中等延迟线性衰减", func(t *testing.T) {
		eval := NewLatencyEvaluator(
			WithMaxLatency(5000),
			WithTargetLatency(1000),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 3 * time.Second, // 3000ms，介于目标和最大之间
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 3000ms: score = 1.0 - (3000-1000)/(5000-1000) = 1.0 - 0.5 = 0.5
		if result.Score != 0.5 {
			t.Errorf("Score = %f, want 0.5", result.Score)
		}
	})

	t.Run("无计时信息", func(t *testing.T) {
		eval := NewLatencyEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: nil,
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("无计时信息分数应为 0，实际 %f", result.Score)
		}
	})
}

// ============== CostEvaluator 测试 ==============

// TestCostEvaluator 测试成本评估器
func TestCostEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewCostEvaluator()

		if eval.Name() != "cost" {
			t.Errorf("Name() = %s, want cost", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		eval := NewCostEvaluator(
			WithMaxCost(0.5),
			WithTargetCost(0.05),
		)

		if eval.maxCost != 0.5 {
			t.Errorf("maxCost = %f, want 0.5", eval.maxCost)
		}
		if eval.targetCost != 0.05 {
			t.Errorf("targetCost = %f, want 0.05", eval.targetCost)
		}
	})

	t.Run("正常成本", func(t *testing.T) {
		eval := NewCostEvaluator(
			WithMaxCost(0.1),
			WithTargetCost(0.01),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Cost: &evaluate.CostInfo{
				Cost:         0.005, // 低于目标成本
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("低于最大成本应该通过")
		}
	})

	t.Run("超限成本", func(t *testing.T) {
		eval := NewCostEvaluator(
			WithMaxCost(0.1),
			WithTargetCost(0.01),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Cost: &evaluate.CostInfo{
				Cost: 0.2, // 超过最大成本
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("超限成本分数应为 0，实际 %f", result.Score)
		}

		if result.Passed == nil || *result.Passed {
			t.Error("超过最大成本不应通过")
		}
	})

	t.Run("无成本信息", func(t *testing.T) {
		eval := NewCostEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Cost: nil,
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 无成本信息假设为 0 成本，满分
		if result.Score != 1.0 {
			t.Errorf("无成本信息分数应为 1.0，实际 %f", result.Score)
		}
	})
}

// ============== ThroughputEvaluator 测试 ==============

// TestThroughputEvaluator 测试吞吐量评估器
func TestThroughputEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewThroughputEvaluator()

		if eval.Name() != "throughput" {
			t.Errorf("Name() = %s, want throughput", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		eval := NewThroughputEvaluator(
			WithMinThroughput(20),
			WithTargetThroughput(100),
		)

		if eval.minThroughput != 20 {
			t.Errorf("minThroughput = %f, want 20", eval.minThroughput)
		}
		if eval.targetThroughput != 100 {
			t.Errorf("targetThroughput = %f, want 100", eval.targetThroughput)
		}
	})

	t.Run("正常吞吐", func(t *testing.T) {
		eval := NewThroughputEvaluator(
			WithMinThroughput(10),
			WithTargetThroughput(50),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 1 * time.Second,
			},
			Cost: &evaluate.CostInfo{
				OutputTokens: 100, // 100 tokens / 1 秒 = 100 tps，超过目标
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("高吞吐应该通过")
		}
	})

	t.Run("低吞吐", func(t *testing.T) {
		eval := NewThroughputEvaluator(
			WithMinThroughput(10),
			WithTargetThroughput(50),
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 1 * time.Second,
			},
			Cost: &evaluate.CostInfo{
				OutputTokens: 5, // 5 tps，低于最小吞吐量
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("低吞吐分数应为 0，实际 %f", result.Score)
		}

		if result.Passed == nil || *result.Passed {
			t.Error("低吞吐不应通过")
		}
	})

	t.Run("无计时信息", func(t *testing.T) {
		eval := NewThroughputEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: nil,
			Cost: &evaluate.CostInfo{
				OutputTokens: 100,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("无计时信息分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("无成本信息", func(t *testing.T) {
		eval := NewThroughputEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 1 * time.Second,
			},
			Cost: nil,
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("无成本信息分数应为 0，实际 %f", result.Score)
		}
	})
}

// ============== CompositePerformanceEvaluator 测试 ==============

// TestCompositePerformanceEvaluator 测试综合性能评估器
func TestCompositePerformanceEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewCompositePerformanceEvaluator()

		if eval.Name() != "performance" {
			t.Errorf("Name() = %s, want performance", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义权重", func(t *testing.T) {
		eval := NewCompositePerformanceEvaluator(
			WithPerformanceWeights(0.5, 0.3, 0.2),
		)

		if eval.latencyWeight != 0.5 {
			t.Errorf("latencyWeight = %f, want 0.5", eval.latencyWeight)
		}
		if eval.costWeight != 0.3 {
			t.Errorf("costWeight = %f, want 0.3", eval.costWeight)
		}
		if eval.throughputWeight != 0.2 {
			t.Errorf("throughputWeight = %f, want 0.2", eval.throughputWeight)
		}
	})

	t.Run("综合评估", func(t *testing.T) {
		eval := NewCompositePerformanceEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration:     500 * time.Millisecond, // 低于目标延迟
				TTFBDuration: 100 * time.Millisecond,
			},
			Cost: &evaluate.CostInfo{
				Cost:         0.005, // 低于目标成本
				OutputTokens: 100,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 所有子指标都满分，综合分数应为 1.0
		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}

		if result.SubScores == nil {
			t.Error("SubScores 不应为空")
		}

		if result.Details == nil {
			t.Error("Details 不应为空")
		}
	})

	t.Run("无性能信息", func(t *testing.T) {
		eval := NewCompositePerformanceEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 无任何性能信息，分数应为 0
		if result.Score != 0 {
			t.Errorf("无性能信息分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("仅有延迟信息", func(t *testing.T) {
		eval := NewCompositePerformanceEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Timing: &evaluate.TimingInfo{
				Duration: 500 * time.Millisecond,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 仅延迟分数有效，权重归一化后应为满分
		if result.Score != 1.0 {
			t.Errorf("仅延迟满分时综合分数应为 1.0，实际 %f", result.Score)
		}
	})
}
