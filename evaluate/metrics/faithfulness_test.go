package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/everyday-items/hexagon/evaluate"
)

// TestFaithfulnessEvaluator 测试忠实度评估器
func TestFaithfulnessEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewFaithfulnessEvaluator(llm)

		if eval.Name() != "faithfulness" {
			t.Errorf("Name() = %s, want faithfulness", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if !eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 true")
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewFaithfulnessEvaluator(llm,
			WithFaithfulnessThreshold(0.8),
			WithFaithfulnessStrict(true),
		)

		if eval.threshold != 0.8 {
			t.Errorf("threshold = %f, want 0.8", eval.threshold)
		}
		if !eval.strict {
			t.Error("strict 应为 true")
		}
	})

	t.Run("无上下文", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewFaithfulnessEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "test response",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 无上下文时返回 1.0（无法判断是否编造）
		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}
	})

	t.Run("空响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewFaithfulnessEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Context:  []string{"some context"},
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("无可验证声明", func(t *testing.T) {
		llm := &mockLLMJudge{response: "No claims found."}
		eval := NewFaithfulnessEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Context:  []string{"context"},
			Response: "Just a question?",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("无声明时分数应为 1.0，实际 %f", result.Score)
		}
	})

	t.Run("所有声明得到支持", func(t *testing.T) {
		callCount := 0
		llm := &mockLLMJudge{}

		// 第一次调用返回声明列表
		// 后续调用返回验证结果
		originalJudge := llm.Judge
		llm = &mockLLMJudge{}

		eval := &FaithfulnessEvaluator{
			llm:       &mockLLMJudge{},
			threshold: 0.7,
		}

		// 模拟正常流程
		eval.llm = &sequentialMockLLM{
			responses: []string{
				"- Claim 1\n- Claim 2",
				"YES - Supported",
				"YES - Supported",
			},
		}

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Context:  []string{"context supporting claims"},
			Response: "Claim 1. Claim 2.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("所有声明得到支持时分数应为 1.0，实际 %f", result.Score)
		}

		_ = callCount
		_ = originalJudge
	})

	t.Run("严格模式", func(t *testing.T) {
		eval := &FaithfulnessEvaluator{
			llm: &sequentialMockLLM{
				responses: []string{
					"- Claim 1\n- Claim 2",
					"YES",
					"NO - Unsupported",
				},
			},
			threshold: 0.5,
			strict:    true,
		}

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Context:  []string{"context"},
			Response: "response with claims",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 严格模式下，任何编造都导致分数为 0
		if result.Score != 0 {
			t.Errorf("严格模式下有编造时分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errors.New("LLM 错误")}
		eval := NewFaithfulnessEvaluator(llm)

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Context:  []string{"context"},
			Response: "response",
		})

		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// TestHallucinationEvaluator 测试幻觉检测评估器
func TestHallucinationEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewHallucinationEvaluator(llm)

		if eval.Name() != "hallucination" {
			t.Errorf("Name() = %s, want hallucination", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewHallucinationEvaluator(llm, WithHallucinationThreshold(0.9))

		if eval.threshold != 0.9 {
			t.Errorf("threshold = %f, want 0.9", eval.threshold)
		}
	})

	t.Run("空响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewHallucinationEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("空响应分数应为 1.0，实际 %f", result.Score)
		}
	})

	t.Run("检测幻觉", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 3\nHallucinations: Made up stats\nReason: Contains fabricated data"}
		eval := NewHallucinationEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "test",
			Response: "response with hallucinations",
			Context:  []string{"context"},
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 3/10 = 0.3
		if result.Score != 0.3 {
			t.Errorf("Score = %f, want 0.3", result.Score)
		}

		if result.Passed == nil || *result.Passed {
			t.Error("0.3 < 0.8 不应通过")
		}
	})

	t.Run("无幻觉", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 10\nHallucinations: None\nReason: All accurate"}
		eval := NewHallucinationEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "test",
			Response: "accurate response",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}
	})
}

// sequentialMockLLM 按顺序返回预设响应的模拟 LLM
type sequentialMockLLM struct {
	responses []string
	index     int
}

func (m *sequentialMockLLM) Judge(ctx context.Context, prompt string) (string, error) {
	if m.index >= len(m.responses) {
		return "", errors.New("no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}
