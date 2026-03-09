package metrics

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/evaluate"
)

// ============== ConsistencyEvaluator 测试 ==============

// TestConsistencyEvaluator 测试一致性评估器
func TestConsistencyEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 8\nReason: Consistent"}
		eval := NewConsistencyEvaluator(llm)

		if eval.Name() != "consistency" {
			t.Errorf("Name() = %s, want consistency", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if !eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 true")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewConsistencyEvaluator(llm, WithConsistencyThreshold(0.9))

		if eval.threshold != 0.9 {
			t.Errorf("threshold = %f, want 0.9", eval.threshold)
		}
	})

	t.Run("空输入", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewConsistencyEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
		if result.Reason != "Empty response" {
			t.Errorf("Reason = %q, want %q", result.Reason, "Empty response")
		}
	})

	t.Run("评估一致性", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 8\nContradictions: None\nReason: Internally consistent"}
		eval := NewConsistencyEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "Go is a programming language. Go was created at Google.",
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
		llm := &mockLLMJudge{err: errMock}
		eval := NewConsistencyEvaluator(llm)

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "some response",
		})

		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// ============== PerplexityEvaluator 测试 ==============

// TestPerplexityEvaluator 测试困惑度评估器
func TestPerplexityEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewPerplexityEvaluator()

		if eval.Name() != "perplexity" {
			t.Errorf("Name() = %s, want perplexity", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		eval := NewPerplexityEvaluator(WithPerplexityThreshold(100.0))

		if eval.threshold != 100.0 {
			t.Errorf("threshold = %f, want 100.0", eval.threshold)
		}
	})

	t.Run("空响应", func(t *testing.T) {
		eval := NewPerplexityEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 空响应得满分
		if result.Score != 1.0 {
			t.Errorf("空响应分数应为 1.0，实际 %f", result.Score)
		}
	})

	t.Run("计算验证", func(t *testing.T) {
		eval := NewPerplexityEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "The quick brown fox jumps over the lazy dog. The fox is very quick and brown.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数应在 0-1 之间
		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Score 超出范围: got %v, want [0, 1]", result.Score)
		}

		// 检查 Details 中包含困惑度信息
		if result.Details == nil {
			t.Error("Details 不应为空")
		}

		if _, ok := result.Details["perplexity"]; !ok {
			t.Error("Details 应包含 perplexity")
		}
		if _, ok := result.Details["threshold"]; !ok {
			t.Error("Details 应包含 threshold")
		}
	})

	t.Run("单词文本边界情况", func(t *testing.T) {
		eval := NewPerplexityEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "hello",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 单词文本，困惑度为 0，归一化分数应为 1.0
		if result.Score != 1.0 {
			t.Errorf("单词文本分数应为 1.0，实际 %f", result.Score)
		}
	})

	t.Run("重复文本低困惑度", func(t *testing.T) {
		eval := NewPerplexityEvaluator()

		ctx := context.Background()
		// 重复文本应有较低的困惑度
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "the cat the cat the cat the cat the cat the cat",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Score 超出范围: got %v, want [0, 1]", result.Score)
		}

		if result.Passed == nil {
			t.Error("Passed 不应为 nil")
		}
	})
}

// ============== DiversityEvaluator 测试 ==============

// TestDiversityEvaluator 测试多样性评估器
func TestDiversityEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		eval := NewDiversityEvaluator()

		if eval.Name() != "diversity" {
			t.Errorf("Name() = %s, want diversity", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 false")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		eval := NewDiversityEvaluator(WithDiversityThreshold(0.8))

		if eval.threshold != 0.8 {
			t.Errorf("threshold = %f, want 0.8", eval.threshold)
		}
	})

	t.Run("空输入", func(t *testing.T) {
		eval := NewDiversityEvaluator()

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("高多样性文本", func(t *testing.T) {
		eval := NewDiversityEvaluator()

		ctx := context.Background()
		// 每个词都不同的文本，TTR 应该较高
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "Go is a statically typed compiled language designed at Google by Robert Griesemer Rob Pike and Ken Thompson",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score < 0.5 {
			t.Errorf("高多样性文本分数应较高，实际 %f", result.Score)
		}

		if result.Details == nil {
			t.Error("Details 不应为空")
		}

		if _, ok := result.Details["ttr"]; !ok {
			t.Error("Details 应包含 ttr")
		}
		if _, ok := result.Details["bigram_diversity"]; !ok {
			t.Error("Details 应包含 bigram_diversity")
		}
	})

	t.Run("低多样性文本", func(t *testing.T) {
		eval := NewDiversityEvaluator()

		ctx := context.Background()
		// 大量重复词的文本，TTR 应该较低
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "the the the the the the the the the the the the",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score > 0.3 {
			t.Errorf("低多样性文本分数应较低，实际 %f", result.Score)
		}
	})
}

// ============== CompletenessEvaluator 测试 ==============

// TestCompletenessEvaluator 测试完整性评估器
func TestCompletenessEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCompletenessEvaluator(llm)

		if eval.Name() != "completeness" {
			t.Errorf("Name() = %s, want completeness", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if !eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 true")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCompletenessEvaluator(llm, WithCompletenessThreshold(0.85))

		if eval.threshold != 0.85 {
			t.Errorf("threshold = %f, want 0.85", eval.threshold)
		}
	})

	t.Run("缺少查询或响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewCompletenessEvaluator(llm)

		ctx := context.Background()

		// 无查询
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "some response",
		})
		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}
		if result.Score != 0 {
			t.Errorf("无查询时分数应为 0，实际 %f", result.Score)
		}

		// 无响应
		result, err = eval.Evaluate(ctx, evaluate.EvalInput{
			Query: "some query",
		})
		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}
		if result.Score != 0 {
			t.Errorf("无响应时分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("评估完整性", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 9\nMissing Aspects: None\nReason: Comprehensive coverage"}
		eval := NewCompletenessEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Query:    "What is Go?",
			Response: "Go is a statically typed, compiled language designed at Google.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 9/10 = 0.9
		if result.Score != 0.9 {
			t.Errorf("Score = %f, want 0.9", result.Score)
		}

		if result.Passed == nil || !*result.Passed {
			t.Error("0.9 >= 0.7 应该通过")
		}
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errMock}
		eval := NewCompletenessEvaluator(llm)

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

// ============== ToxicityEvaluator 测试 ==============

// TestToxicityEvaluator 测试毒性评估器
func TestToxicityEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewToxicityEvaluator(llm)

		if eval.Name() != "toxicity" {
			t.Errorf("Name() = %s, want toxicity", eval.Name())
		}
		if eval.Description() == "" {
			t.Error("Description() 不应为空")
		}
		if !eval.RequiresLLM() {
			t.Error("RequiresLLM() 应为 true")
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewToxicityEvaluator(llm, WithToxicityThreshold(0.5))

		if eval.threshold != 0.5 {
			t.Errorf("threshold = %f, want 0.5", eval.threshold)
		}
	})

	t.Run("空响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewToxicityEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 空响应无毒性
		if result.Score != 0 {
			t.Errorf("空响应分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("无毒性内容", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 0\nCategories: None\nReason: Safe content"}
		eval := NewToxicityEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "Go is a wonderful programming language.",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 0/10 = 0.0（无毒性）
		if result.Score != 0 {
			t.Errorf("Score = %f, want 0", result.Score)
		}

		// 毒性分数低于阈值应通过
		if result.Passed == nil || !*result.Passed {
			t.Error("0.0 < 0.3 应该通过")
		}
	})

	t.Run("高毒性内容", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 8\nCategories: Offensive language\nReason: Contains harmful content"}
		eval := NewToxicityEvaluator(llm)

		ctx := context.Background()
		result, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "some toxic text",
		})

		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		// 分数 8/10 = 0.8
		if result.Score != 0.8 {
			t.Errorf("Score = %f, want 0.8", result.Score)
		}

		// 毒性分数高于阈值不通过
		if result.Passed == nil || *result.Passed {
			t.Error("0.8 >= 0.3 不应通过")
		}
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errMock}
		eval := NewToxicityEvaluator(llm)

		ctx := context.Background()
		_, err := eval.Evaluate(ctx, evaluate.EvalInput{
			Response: "some response",
		})

		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// errMock 用于测试的模拟错误
var errMock = errForTest("mock error")

// errForTest 测试用错误类型
type errForTest string

func (e errForTest) Error() string { return string(e) }
