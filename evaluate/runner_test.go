package evaluate

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockEvaluator 用于测试的模拟评估器
type mockEvaluator struct {
	name  string
	score float64
	err   error
}

func (m *mockEvaluator) Name() string        { return m.name }
func (m *mockEvaluator) Description() string { return "mock evaluator" }
func (m *mockEvaluator) RequiresLLM() bool   { return false }

func (m *mockEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	passed := m.score >= 0.7
	return &EvalResult{
		Name:   m.name,
		Score:  m.score,
		Passed: &passed,
		Reason: "mock result",
	}, nil
}

// mockSystem 用于测试的模拟被测系统
type mockSystem struct {
	response *SystemResponse
	err      error
}

func (m *mockSystem) Run(ctx context.Context, query string) (*SystemResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// TestNewRunner 测试创建运行器
func TestNewRunner(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		runner := NewRunner(nil)
		if runner == nil {
			t.Fatal("NewRunner 返回 nil")
		}
		if runner.config == nil {
			t.Error("config 不应为 nil")
		}
	})

	t.Run("自定义配置", func(t *testing.T) {
		config := &EvalConfig{
			Concurrency: 4,
			Timeout:     30 * time.Second,
		}
		runner := NewRunner(config)

		if runner.config.Concurrency != 4 {
			t.Errorf("Concurrency = %d, want 4", runner.config.Concurrency)
		}
	})
}

// TestRunnerAddEvaluator 测试添加评估器
func TestRunnerAddEvaluator(t *testing.T) {
	runner := NewRunner(nil)

	runner.AddEvaluator(&mockEvaluator{name: "eval1", score: 0.8})
	runner.AddEvaluator(&mockEvaluator{name: "eval2", score: 0.9})

	if len(runner.evaluators) != 2 {
		t.Errorf("评估器数量 = %d, want 2", len(runner.evaluators))
	}
}

// TestRunnerAddEvaluators 测试批量添加评估器
func TestRunnerAddEvaluators(t *testing.T) {
	runner := NewRunner(nil)

	runner.AddEvaluators(
		&mockEvaluator{name: "eval1", score: 0.8},
		&mockEvaluator{name: "eval2", score: 0.9},
		&mockEvaluator{name: "eval3", score: 0.7},
	)

	if len(runner.evaluators) != 3 {
		t.Errorf("评估器数量 = %d, want 3", len(runner.evaluators))
	}
}

// TestRunnerEvaluateSingle 测试单个评估
func TestRunnerEvaluateSingle(t *testing.T) {
	t.Run("成功评估", func(t *testing.T) {
		runner := NewRunner(nil)
		runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.85})

		ctx := context.Background()
		input := EvalInput{
			Query:    "test query",
			Response: "test response",
		}

		results, err := runner.EvaluateSingle(ctx, input)
		if err != nil {
			t.Fatalf("EvaluateSingle 错误: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("结果数量 = %d, want 1", len(results))
		}

		if results["test"].Score != 0.85 {
			t.Errorf("分数 = %f, want 0.85", results["test"].Score)
		}
	})

	t.Run("评估器错误", func(t *testing.T) {
		runner := NewRunner(nil)
		runner.AddEvaluator(&mockEvaluator{name: "test", err: errors.New("评估错误")})

		ctx := context.Background()
		input := EvalInput{Query: "test", Response: "response"}

		results, err := runner.EvaluateSingle(ctx, input)
		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}

		if results["test"].Error == "" {
			t.Error("应该记录错误信息")
		}
	})
}

// TestRunnerEvaluateBatch 测试批量评估
func TestRunnerEvaluateBatch(t *testing.T) {
	runner := NewRunner(&EvalConfig{Concurrency: 2})
	runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.8})

	ctx := context.Background()
	inputs := []EvalInput{
		{Query: "q1", Response: "r1"},
		{Query: "q2", Response: "r2"},
		{Query: "q3", Response: "r3"},
	}

	results, err := runner.EvaluateBatch(ctx, inputs)
	if err != nil {
		t.Fatalf("EvaluateBatch 错误: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("结果数量 = %d, want 3", len(results))
	}

	for i, res := range results {
		if res == nil {
			t.Errorf("结果 %d 为 nil", i)
		}
	}
}

// TestRunnerEvaluateDataset 测试数据集评估
func TestRunnerEvaluateDataset(t *testing.T) {
	t.Run("成功评估数据集", func(t *testing.T) {
		runner := NewRunner(&EvalConfig{Concurrency: 2})
		runner.AddEvaluator(&mockEvaluator{name: "metric1", score: 0.8})

		system := &mockSystem{
			response: &SystemResponse{
				Response: "test response",
				Context:  []string{"context"},
			},
		}

		dataset := &Dataset{
			Name: "test-dataset",
			Samples: []Sample{
				{ID: "1", Query: "q1", Reference: "r1"},
				{ID: "2", Query: "q2", Reference: "r2"},
			},
		}

		ctx := context.Background()
		report, err := runner.EvaluateDataset(ctx, dataset, system)

		if err != nil {
			t.Fatalf("EvaluateDataset 错误: %v", err)
		}

		if report == nil {
			t.Fatal("report 不应为 nil")
		}

		if report.TotalSamples != 2 {
			t.Errorf("TotalSamples = %d, want 2", report.TotalSamples)
		}

		if report.SuccessSamples != 2 {
			t.Errorf("SuccessSamples = %d, want 2", report.SuccessSamples)
		}
	})

	t.Run("无评估器", func(t *testing.T) {
		runner := NewRunner(nil)

		dataset := &Dataset{Name: "test"}
		system := &mockSystem{}

		ctx := context.Background()
		_, err := runner.EvaluateDataset(ctx, dataset, system)

		if err == nil {
			t.Error("应该返回错误：无评估器")
		}
	})

	t.Run("系统错误", func(t *testing.T) {
		runner := NewRunner(&EvalConfig{Concurrency: 1})
		runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.8})

		system := &mockSystem{err: errors.New("系统错误")}

		dataset := &Dataset{
			Name:    "test-dataset",
			Samples: []Sample{{ID: "1", Query: "q1"}},
		}

		ctx := context.Background()
		report, err := runner.EvaluateDataset(ctx, dataset, system)

		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}

		if report.FailedSamples != 1 {
			t.Errorf("FailedSamples = %d, want 1", report.FailedSamples)
		}
	})
}

// TestSystemFunc 测试函数式被测系统
func TestSystemFunc(t *testing.T) {
	system := SystemFunc(func(ctx context.Context, query string) (*SystemResponse, error) {
		return &SystemResponse{
			Response: "response for: " + query,
		}, nil
	})

	ctx := context.Background()
	resp, err := system.Run(ctx, "test query")

	if err != nil {
		t.Fatalf("Run 错误: %v", err)
	}

	if resp.Response != "response for: test query" {
		t.Errorf("Response = %s", resp.Response)
	}
}

// TestQuickEval 测试快速评估
func TestQuickEval(t *testing.T) {
	ctx := context.Background()
	evaluators := []Evaluator{
		&mockEvaluator{name: "eval1", score: 0.8},
		&mockEvaluator{name: "eval2", score: 0.9},
	}
	input := EvalInput{Query: "test", Response: "response"}

	results, err := QuickEval(ctx, evaluators, input)
	if err != nil {
		t.Fatalf("QuickEval 错误: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("结果数量 = %d, want 2", len(results))
	}
}

// TestRunnerTimeout 测试超时处理
func TestRunnerTimeout(t *testing.T) {
	runner := NewRunner(&EvalConfig{
		Concurrency: 1,
		Timeout:     100 * time.Millisecond,
	})
	runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.8})

	system := SystemFunc(func(ctx context.Context, query string) (*SystemResponse, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return &SystemResponse{Response: "ok"}, nil
		}
	})

	dataset := &Dataset{
		Name:    "test",
		Samples: []Sample{{ID: "1", Query: "q1"}},
	}

	ctx := context.Background()
	report, err := runner.EvaluateDataset(ctx, dataset, system)

	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}

	if report.SuccessSamples != 1 {
		t.Errorf("SuccessSamples = %d, want 1", report.SuccessSamples)
	}
}

// TestEvalReportDuration 测试报告时间计算
func TestEvalReportDuration(t *testing.T) {
	runner := NewRunner(&EvalConfig{Concurrency: 1})
	runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.8})

	system := &mockSystem{
		response: &SystemResponse{Response: "ok"},
	}

	dataset := &Dataset{
		Name:    "test",
		Samples: []Sample{{ID: "1", Query: "q1"}},
	}

	ctx := context.Background()
	report, _ := runner.EvaluateDataset(ctx, dataset, system)

	if report.Duration <= 0 {
		t.Error("Duration 应该大于 0")
	}

	if report.EndTime.Before(report.StartTime) {
		t.Error("EndTime 应该在 StartTime 之后")
	}
}

// TestMetricSummaryCalculation 测试指标汇总计算
func TestMetricSummaryCalculation(t *testing.T) {
	runner := NewRunner(&EvalConfig{Concurrency: 1})
	runner.AddEvaluator(&mockEvaluator{name: "test", score: 0.8})

	system := &mockSystem{
		response: &SystemResponse{Response: "ok"},
	}

	dataset := &Dataset{
		Name: "test",
		Samples: []Sample{
			{ID: "1", Query: "q1"},
			{ID: "2", Query: "q2"},
		},
	}

	ctx := context.Background()
	report, _ := runner.EvaluateDataset(ctx, dataset, system)

	summary := report.Summary["test"]
	if summary == nil {
		t.Fatal("Summary 不应为 nil")
	}

	if summary.Count != 2 {
		t.Errorf("Count = %d, want 2", summary.Count)
	}

	if summary.Mean != 0.8 {
		t.Errorf("Mean = %f, want 0.8", summary.Mean)
	}
}
