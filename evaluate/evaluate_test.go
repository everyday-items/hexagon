package evaluate_test

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/evaluate"
)

// MockEvaluator 用于测试的模拟评估器
type MockEvaluator struct {
	name        string
	description string
	score       float64
	requiresLLM bool
	err         error
}

func NewMockEvaluator(name string, score float64) *MockEvaluator {
	return &MockEvaluator{
		name:        name,
		description: "Mock evaluator for testing",
		score:       score,
		requiresLLM: false,
	}
}

func (m *MockEvaluator) Name() string {
	return m.name
}

func (m *MockEvaluator) Description() string {
	return m.description
}

func (m *MockEvaluator) Evaluate(ctx context.Context, input evaluate.EvalInput) (*evaluate.EvalResult, error) {
	if m.err != nil {
		return nil, m.err
	}

	passed := m.score >= 0.7
	return &evaluate.EvalResult{
		Name:   m.name,
		Score:  m.score,
		Passed: &passed,
		Reason: "Mock evaluation",
		Details: map[string]any{
			"query":    input.Query,
			"response": input.Response,
		},
	}, nil
}

func (m *MockEvaluator) RequiresLLM() bool {
	return m.requiresLLM
}

// TestEvalInput 测试评估输入
func TestEvalInput(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		input := evaluate.EvalInput{
			Query:    "What is AI?",
			Response: "AI stands for Artificial Intelligence",
			Context:  []string{"context1", "context2"},
			Reference: "Reference answer",
			Metadata: map[string]any{
				"key": "value",
			},
		}

		if input.Query != "What is AI?" {
			t.Errorf("Query = %s, want What is AI?", input.Query)
		}
		if len(input.Context) != 2 {
			t.Errorf("Context length = %d, want 2", len(input.Context))
		}
	})

	t.Run("WithTiming", func(t *testing.T) {
		start := time.Now()
		end := start.Add(100 * time.Millisecond)

		input := evaluate.EvalInput{
			Query:    "test",
			Response: "response",
			Timing: &evaluate.TimingInfo{
				StartTime: start,
				EndTime:   end,
				Duration:  end.Sub(start),
			},
		}

		if input.Timing == nil {
			t.Fatal("Timing should not be nil")
		}
		if input.Timing.Duration < 100*time.Millisecond {
			t.Errorf("Duration = %v, want >= 100ms", input.Timing.Duration)
		}
	})

	t.Run("WithCost", func(t *testing.T) {
		input := evaluate.EvalInput{
			Query:    "test",
			Response: "response",
			Cost: &evaluate.CostInfo{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				Cost:         0.01,
			},
		}

		if input.Cost == nil {
			t.Fatal("Cost should not be nil")
		}
		if input.Cost.TotalTokens != 150 {
			t.Errorf("TotalTokens = %d, want 150", input.Cost.TotalTokens)
		}
	})
}

// TestEvalResult 测试评估结果
func TestEvalResult(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		passed := true
		result := evaluate.EvalResult{
			Name:   "test-eval",
			Score:  0.85,
			Passed: &passed,
			Reason: "Good quality",
			Details: map[string]any{
				"metric": "value",
			},
			SubScores: map[string]float64{
				"sub1": 0.8,
				"sub2": 0.9,
			},
		}

		if result.Name != "test-eval" {
			t.Errorf("Name = %s, want test-eval", result.Name)
		}
		if result.Score != 0.85 {
			t.Errorf("Score = %f, want 0.85", result.Score)
		}
		if !*result.Passed {
			t.Error("Passed should be true")
		}
		if len(result.SubScores) != 2 {
			t.Errorf("SubScores length = %d, want 2", len(result.SubScores))
		}
	})

	t.Run("WithError", func(t *testing.T) {
		result := evaluate.EvalResult{
			Name:  "test-eval",
			Score: 0,
			Error: "evaluation failed",
		}

		if result.Error == "" {
			t.Error("Error should not be empty")
		}
	})
}

// TestMockEvaluator 测试模拟评估器
func TestMockEvaluator(t *testing.T) {
	ctx := context.Background()
	eval := NewMockEvaluator("mock-eval", 0.9)

	t.Run("Name", func(t *testing.T) {
		if eval.Name() != "mock-eval" {
			t.Errorf("Name() = %s, want mock-eval", eval.Name())
		}
	})

	t.Run("Description", func(t *testing.T) {
		desc := eval.Description()
		if desc == "" {
			t.Error("Description() should not be empty")
		}
	})

	t.Run("Evaluate", func(t *testing.T) {
		input := evaluate.EvalInput{
			Query:    "test query",
			Response: "test response",
		}

		result, err := eval.Evaluate(ctx, input)
		if err != nil {
			t.Fatalf("Evaluate() error = %v", err)
		}

		if result.Score != 0.9 {
			t.Errorf("Score = %f, want 0.9", result.Score)
		}
		if !*result.Passed {
			t.Error("Passed should be true for score 0.9")
		}
	})

	t.Run("RequiresLLM", func(t *testing.T) {
		if eval.RequiresLLM() {
			t.Error("Mock evaluator should not require LLM")
		}
	})
}

// TestDataset 测试数据集
func TestDataset(t *testing.T) {
	t.Run("NewDataset", func(t *testing.T) {
		dataset := &evaluate.Dataset{
			Name:        "test-dataset",
			Description: "Test dataset for evaluation",
			Samples:     []evaluate.Sample{},
		}

		if dataset.Name != "test-dataset" {
			t.Errorf("Name = %s, want test-dataset", dataset.Name)
		}
	})

	t.Run("WithSamples", func(t *testing.T) {
		samples := []evaluate.Sample{
			{
				Query:     "query1",
				Reference: "answer1",
			},
			{
				Query:     "query2",
				Reference: "answer2",
			},
		}

		dataset := &evaluate.Dataset{
			Name:    "test-dataset",
			Samples: samples,
		}

		if len(dataset.Samples) != 2 {
			t.Errorf("Samples length = %d, want 2", len(dataset.Samples))
		}
	})
}

// TestSample 测试样本
func TestSample(t *testing.T) {
	sample := evaluate.Sample{
		ID:        "sample-1",
		Query:     "What is Go?",
		Reference: "Go is a programming language",
		Context:   []string{"Go documentation", "Go tutorial"},
		Metadata: map[string]any{
			"difficulty": "easy",
		},
	}

	if sample.ID != "sample-1" {
		t.Errorf("ID = %s, want sample-1", sample.ID)
	}
	if sample.Query != "What is Go?" {
		t.Errorf("Query = %s, want What is Go?", sample.Query)
	}
	if len(sample.Context) != 2 {
		t.Errorf("Context length = %d, want 2", len(sample.Context))
	}
}

// TestEvalConfig 测试评估配置
func TestEvalConfig(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		config := evaluate.DefaultEvalConfig()
		if config == nil {
			t.Fatal("DefaultEvalConfig() returned nil")
		}

		if config.Concurrency <= 0 {
			t.Error("Concurrency should be positive")
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		evaluators := []evaluate.Evaluator{
			NewMockEvaluator("eval1", 0.8),
			NewMockEvaluator("eval2", 0.9),
		}

		config := &evaluate.EvalConfig{
			Concurrency: 4,
			Evaluators:  evaluators,
			StopOnError: true,
		}

		if config.Concurrency != 4 {
			t.Errorf("Concurrency = %d, want 4", config.Concurrency)
		}
		if len(config.Evaluators) != 2 {
			t.Errorf("Evaluators length = %d, want 2", len(config.Evaluators))
		}
		if !config.StopOnError {
			t.Error("StopOnError should be true")
		}
	})
}

// TestSystemResponse 测试系统响应
func TestSystemResponse(t *testing.T) {
	response := &evaluate.SystemResponse{
		Response: "This is a response",
		Context:  []string{"context1", "context2"},
		Timing: &evaluate.TimingInfo{
			Duration: 100 * time.Millisecond,
		},
		Cost: &evaluate.CostInfo{
			TotalTokens: 150,
			Cost:        0.01,
		},
		Metadata: map[string]any{
			"model": "gpt-4",
		},
	}

	if response.Response != "This is a response" {
		t.Errorf("Response = %s, want This is a response", response.Response)
	}
	if len(response.Context) != 2 {
		t.Errorf("Context length = %d, want 2", len(response.Context))
	}
	if response.Timing == nil {
		t.Fatal("Timing should not be nil")
	}
	if response.Cost == nil {
		t.Fatal("Cost should not be nil")
	}
}

// MockSystem 模拟被测系统
type MockSystem struct {
	response *evaluate.SystemResponse
	err      error
}

func (m *MockSystem) Run(ctx context.Context, query string) (*evaluate.SystemResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// TestSystemUnderTest 测试被测系统接口
func TestSystemUnderTest(t *testing.T) {
	ctx := context.Background()

	t.Run("MockSystem", func(t *testing.T) {
		system := &MockSystem{
			response: &evaluate.SystemResponse{
				Response: "test response",
				Context:  []string{"context"},
			},
		}

		resp, err := system.Run(ctx, "test query")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if resp.Response != "test response" {
			t.Errorf("Response = %s, want test response", resp.Response)
		}
	})

	t.Run("SystemFunc", func(t *testing.T) {
		system := evaluate.SystemFunc(func(ctx context.Context, query string) (*evaluate.SystemResponse, error) {
			return &evaluate.SystemResponse{
				Response: "func response",
			}, nil
		})

		resp, err := system.Run(ctx, "test")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if resp.Response != "func response" {
			t.Errorf("Response = %s, want func response", resp.Response)
		}
	})
}

// TestMetricSummary 测试指标汇总
func TestMetricSummary(t *testing.T) {
	passRate := 0.90
	summary := &evaluate.MetricSummary{
		Name:         "test-metric",
		Mean:         0.85,
		Median:       0.87,
		StdDev:       0.05,
		Min:          0.70,
		Max:          0.95,
		PassRate:     &passRate,
		Distribution: map[string]int{
			"0.7-0.8": 2,
			"0.8-0.9": 5,
			"0.9-1.0": 3,
		},
	}

	if summary.Name != "test-metric" {
		t.Errorf("Name = %s, want test-metric", summary.Name)
	}
	if summary.Mean != 0.85 {
		t.Errorf("Mean = %f, want 0.85", summary.Mean)
	}
	if summary.PassRate == nil || *summary.PassRate != 0.90 {
		t.Errorf("PassRate = %v, want 0.90", summary.PassRate)
	}
}

// TestSampleResult 测试样本结果
func TestSampleResult(t *testing.T) {
	result := evaluate.SampleResult{
		SampleID: "sample-1",
		Query:    "test query",
		Response: "test response",
		Results: map[string]*evaluate.EvalResult{
			"metric1": {Name: "metric1", Score: 0.8},
			"metric2": {Name: "metric2", Score: 0.9},
		},
		Duration: 100 * time.Millisecond,
	}

	if result.SampleID != "sample-1" {
		t.Errorf("SampleID = %s, want sample-1", result.SampleID)
	}
	if len(result.Results) != 2 {
		t.Errorf("Results length = %d, want 2", len(result.Results))
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", result.Duration)
	}
}

// BenchmarkEvaluator 基准测试
func BenchmarkEvaluator(b *testing.B) {
	ctx := context.Background()
	eval := NewMockEvaluator("bench-eval", 0.8)
	input := evaluate.EvalInput{
		Query:    "benchmark query",
		Response: "benchmark response",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate(ctx, input)
	}
}
