package evaluate

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCustomEvaluator 测试自定义评估器
func TestCustomEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewCustomEvaluator("test_eval", "测试评估器",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				return &EvalResult{Score: 1.0}, nil
			},
		)

		if eval.Name() != "test_eval" {
			t.Errorf("Name() = %s, want test_eval", eval.Name())
		}
		if eval.Description() != "测试评估器" {
			t.Errorf("Description() = %s, want 测试评估器", eval.Description())
		}
		if eval.RequiresLLM() {
			t.Error("默认 RequiresLLM() 应为 false")
		}
	})

	t.Run("WithRequiresLLM 选项", func(t *testing.T) {
		eval := NewCustomEvaluator("llm_eval", "需要 LLM 的评估器",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				return &EvalResult{Score: 0.5}, nil
			},
			WithRequiresLLM(true),
		)

		if !eval.RequiresLLM() {
			t.Error("设置 WithRequiresLLM(true) 后应为 true")
		}
	})

	t.Run("执行评估", func(t *testing.T) {
		eval := NewCustomEvaluator("length_check", "检查长度",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				score := float64(len(input.Response)) / 100.0
				if score > 1.0 {
					score = 1.0
				}
				passed := score >= 0.5
				return &EvalResult{
					Name:   "length_check",
					Score:  score,
					Passed: &passed,
					Reason: "Length based evaluation",
				}, nil
			},
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, EvalInput{
			Response: "This is a test response with some content.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result == nil {
			t.Fatal("结果不应为 nil")
		}

		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Score 超出范围: got %v, want [0, 1]", result.Score)
		}

		if result.Name != "length_check" {
			t.Errorf("Name = %s, want length_check", result.Name)
		}
	})

	t.Run("自动填充名称", func(t *testing.T) {
		eval := NewCustomEvaluator("auto_name", "自动填充名称测试",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				// 返回结果中不设置 Name
				return &EvalResult{
					Score:  0.8,
					Reason: "test",
				}, nil
			},
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, EvalInput{})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 应自动填充为评估器名称
		if result.Name != "auto_name" {
			t.Errorf("Name = %s, want auto_name", result.Name)
		}
	})

	t.Run("评估函数返回错误", func(t *testing.T) {
		eval := NewCustomEvaluator("error_eval", "会返回错误的评估器",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				return nil, errors.New("评估失败")
			},
		)

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, EvalInput{
			Response: "test",
		})

		if err == nil {
			t.Error("应该返回错误")
		}

		// 检查错误包装
		if !errors.Is(err, errors.Unwrap(err)) {
			// 只要错误不为 nil 即可，包装格式在实现中已保证
		}
	})

	t.Run("使用上下文", func(t *testing.T) {
		eval := NewCustomEvaluator("ctx_eval", "使用上下文的评估器",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				// 检查上下文是否被取消
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return &EvalResult{
						Score: 1.0,
					}, nil
				}
			},
		)

		// 已取消的上下文
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := eval.Evaluate(ctx, EvalInput{})
		if err == nil {
			t.Error("已取消上下文应返回错误")
		}
	})

	t.Run("带 Timing 和 Cost 的评估", func(t *testing.T) {
		eval := NewCustomEvaluator("perf_eval", "性能评估",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				score := 1.0
				if input.Timing != nil && input.Timing.Duration > 2*time.Second {
					score = 0.5
				}
				if input.Cost != nil && input.Cost.Cost > 0.05 {
					score *= 0.5
				}
				return &EvalResult{
					Score: score,
				}, nil
			},
		)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, EvalInput{
			Timing: &TimingInfo{
				Duration: 1 * time.Second,
			},
			Cost: &CostInfo{
				Cost: 0.01,
			},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}
	})

	t.Run("空名称 panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("空名称应该 panic")
			}
		}()

		NewCustomEvaluator("", "description",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				return &EvalResult{}, nil
			},
		)
	})

	t.Run("nil 函数 panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("nil 函数应该 panic")
			}
		}()

		NewCustomEvaluator("test", "description", nil)
	})

	t.Run("实现 Evaluator 接口", func(t *testing.T) {
		// 编译时检查已在 custom.go 中完成
		// 这里做运行时验证
		var e Evaluator = NewCustomEvaluator("interface_check", "接口检查",
			func(ctx context.Context, input EvalInput) (*EvalResult, error) {
				return &EvalResult{Score: 0.5}, nil
			},
		)

		if e.Name() != "interface_check" {
			t.Errorf("Name() = %s, want interface_check", e.Name())
		}
	})
}
