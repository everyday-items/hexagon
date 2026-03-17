package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/hexagon-codes/hexagon/evaluate"
)

// ============== CorrectnessEvaluator 测试 ==============

// TestCorrectnessEvaluator 测试正确性评估器
func TestCorrectnessEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm)

		if eval.Name() != "correctness" {
			t.Errorf("Name() = %s, want correctness", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		// 默认模式 semantic 需要 LLM
		if !eval.RequiresLLM() {
			t.Error("默认模式 RequiresLLM() 应为 true")
		}
	})

	t.Run("exact 模式不需要 LLM", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeExact))

		if eval.RequiresLLM() {
			t.Error("exact 模式 RequiresLLM() 应为 false")
		}
	})

	t.Run("exact 模式精确匹配", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeExact))

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "Go is a programming language",
			Reference: "Go is a programming language",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 1.0 {
			t.Errorf("精确匹配分数应为 1.0，实际 %f", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("精确匹配应该通过")
		}
	})

	t.Run("exact 模式部分匹配", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeExact))

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "Go is a compiled language",
			Reference: "Go is a programming language",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 部分匹配，F1 分数应在 0 到 1 之间
		if result.Score <= 0 || result.Score >= 1.0 {
			t.Errorf("部分匹配分数应在 (0, 1) 之间，实际 %f", result.Score)
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm,
			WithCorrectnessThreshold(0.9),
			WithCorrectnessMode(CorrectnessModeFactual),
		)

		if eval.threshold != 0.9 {
			t.Errorf("threshold = %f, want 0.9", eval.threshold)
		}
		if eval.mode != CorrectnessModeFactual {
			t.Errorf("mode = %s, want factual", eval.mode)
		}
	})

	t.Run("空输入", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeExact))

		ctx := context.Background()

		// 空响应
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "",
			Reference: "reference",
		})
		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}
		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("无参考答案", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeExact))

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "some response",
			Reference: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("无参考答案分数应为 0，实际 %f", result.Score)
		}
		if result.Reason == "" {
			t.Error("Reason 不应为空")
		}
	})

	t.Run("semantic 模式评估", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 9\nReason: Semantically equivalent"}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeSemanticSimilarity))

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "Go is developed by Google",
			Reference: "Go was created at Google",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 9/10 = 0.9
		if result.Score != 0.9 {
			t.Errorf("Score = %f, want 0.9", result.Score)
		}
	})

	t.Run("factual 模式评估", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 7\nErrors: None\nReason: Mostly correct"}
		eval := NewCorrectnessEvaluator(llm, WithCorrectnessMode(CorrectnessModeFactual))

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:     "What is Go?",
			Response:  "Go is a language by Google",
			Reference: "Go is a programming language created at Google",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 7/10 = 0.7
		if result.Score != 0.7 {
			t.Errorf("Score = %f, want 0.7", result.Score)
		}
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errors.New("LLM 错误")}
		eval := NewCorrectnessEvaluator(llm) // 默认 semantic 模式

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response:  "response",
			Reference: "reference",
		})

		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// ============== AnswerQualityEvaluator 测试 ==============

// TestAnswerQualityEvaluator 测试回答质量评估器
func TestAnswerQualityEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewAnswerQualityEvaluator(llm)

		if eval.Name() != "answer_quality" {
			t.Errorf("Name() = %s, want answer_quality", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if !eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 true")
		}

		// 检查默认评估标准
		if len(eval.criteria) != 4 {
			t.Errorf("默认 criteria 数量 = %d, want 4", len(eval.criteria))
		}
	})

	t.Run("自定义选项", func(t *testing.T) {
		llm := &mockLLMJudge{}
		customCriteria := []string{"Clarity", "Depth"}
		eval := NewAnswerQualityEvaluator(llm,
			WithAnswerQualityThreshold(0.85),
			WithAnswerQualityCriteria(customCriteria),
		)

		if eval.threshold != 0.85 {
			t.Errorf("threshold = %f, want 0.85", eval.threshold)
		}
		if len(eval.criteria) != 2 {
			t.Errorf("criteria 数量 = %d, want 2", len(eval.criteria))
		}
	})

	t.Run("空响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewAnswerQualityEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "test",
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("评估质量", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Overall Score: 8\nReason: Good quality answer"}
		eval := NewAnswerQualityEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "What is Go?",
			Response: "Go is a statically typed, compiled language designed at Google.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 8/10 = 0.8
		if result.Score != 0.8 {
			t.Errorf("Score = %f, want 0.8", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("0.8 >= 0.7 应该通过")
		}

		if result.Details == nil {
			t.Error("Details 不应为空")
		}
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errors.New("LLM 错误")}
		eval := NewAnswerQualityEvaluator(llm)

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "test",
			Response: "response",
		})

		if err == nil {
			t.Error("应该返回错误")
		}
	})
}
