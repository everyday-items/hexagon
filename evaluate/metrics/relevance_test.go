package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/everyday-items/hexagon/evaluate"
)

// mockLLMJudge 用于测试的模拟 LLM 判断器
type mockLLMJudge struct {
	response string
	err      error
}

func (m *mockLLMJudge) Judge(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// TestRelevanceEvaluator 测试相关性评估器
func TestRelevanceEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 8\nReason: Relevant"}
		eval := NewRelevanceEvaluator(llm)

		if eval.Name() != "relevance" {
			t.Errorf("Name() = %s, want relevance", eval.Name())
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
		eval := NewRelevanceEvaluator(llm, WithRelevanceThreshold(0.8))

		if eval.threshold != 0.8 {
			t.Errorf("threshold = %f, want 0.8", eval.threshold)
		}
	})

	t.Run("无上下文", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewRelevanceEvaluator(llm)

		ctx := context.Background()
		input := evaluate.EvalInput{
			Query:    "test query",
			Response: "test response",
			Context:  nil, // 无上下文
		}

		result, err := eval.Evaluate(ctx, input)
		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0 {
			t.Errorf("无上下文时分数应为 0，实际 %f", result.Score)
		}
	})

	t.Run("评估上下文", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 8\nReason: Good relevance"}
		eval := NewRelevanceEvaluator(llm)

		ctx := context.Background()
		input := evaluate.EvalInput{
			Query:    "What is Go?",
			Response: "Go is a programming language",
			Context:  []string{"Go documentation", "Go tutorial"},
		}

		result, err := eval.Evaluate(ctx, input)
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
	})

	t.Run("LLM 错误", func(t *testing.T) {
		llm := &mockLLMJudge{err: errors.New("LLM 错误")}
		eval := NewRelevanceEvaluator(llm)

		ctx := context.Background()
		input := evaluate.EvalInput{
			Query:   "test",
			Context: []string{"context"},
		}

		_, err := eval.Evaluate(ctx, input)
		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// TestContextRelevanceEvaluator 测试上下文相关性评估器
func TestContextRelevanceEvaluator(t *testing.T) {
	t.Run("创建评估器", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewContextRelevanceEvaluator(llm)

		if eval.Name() != "context_relevance" {
			t.Errorf("Name() = %s, want context_relevance", eval.Name())
		}
	})

	t.Run("自定义阈值", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewContextRelevanceEvaluator(llm, WithContextRelevanceThreshold(0.85))

		if eval.threshold != 0.85 {
			t.Errorf("threshold = %f, want 0.85", eval.threshold)
		}
	})

	t.Run("缺少上下文或响应", func(t *testing.T) {
		llm := &mockLLMJudge{}
		eval := NewContextRelevanceEvaluator(llm)

		ctx := context.Background()

		// 无上下文
		result, _ := eval.Evaluate(ctx, evaluate.EvalInput{Response: "test"})
		if result.Score != 0 {
			t.Error("无上下文时分数应为 0")
		}

		// 无响应
		result, _ = eval.Evaluate(ctx, evaluate.EvalInput{Context: []string{"ctx"}})
		if result.Score != 0 {
			t.Error("无响应时分数应为 0")
		}
	})

	t.Run("评估上下文相关性", func(t *testing.T) {
		llm := &mockLLMJudge{response: "Score: 9\nReason: Good utilization"}
		eval := NewContextRelevanceEvaluator(llm)

		ctx := context.Background()
		input := evaluate.EvalInput{
			Query:    "test",
			Response: "response",
			Context:  []string{"context 1", "context 2"},
		}

		result, err := eval.Evaluate(ctx, input)
		if err != nil {
			t.Fatalf("Evaluate 错误: %v", err)
		}

		if result.Score != 0.9 {
			t.Errorf("Score = %f, want 0.9", result.Score)
		}
	})
}

// TestParseScoreResponse 测试评分响应解析
func TestParseScoreResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantScore  float64
		wantReason string
	}{
		{
			name:       "标准格式",
			response:   "Score: 8\nReason: Good quality",
			wantScore:  8,
			wantReason: "Good quality",
		},
		{
			name:       "小写",
			response:   "score: 7.5\nreason: Acceptable",
			wantScore:  7.5,
			wantReason: "Acceptable",
		},
		{
			name:       "只有分数",
			response:   "Score: 10",
			wantScore:  10,
			wantReason: "Score: 10",
		},
		{
			name:       "无格式",
			response:   "Just some text",
			wantScore:  0,
			wantReason: "Just some text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := parseScoreResponse(tt.response)
			if score != tt.wantScore {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

// TestTruncateText 测试文本截断
func TestTruncateText(t *testing.T) {
	tests := []struct {
		text   string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
		{"hello", 5, "hello"},
		{"你好世界", 2, "你好..."},
	}

	for _, tt := range tests {
		got := truncateText(tt.text, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
		}
	}
}
