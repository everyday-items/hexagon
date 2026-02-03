package metrics

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/evaluate"
)

func TestBLEUEvaluator(t *testing.T) {
	evaluator := NewBLEUEvaluator(
		WithBLEUMaxN(4),
		WithBLEUThreshold(0.3),
	)

	tests := []struct {
		name      string
		reference string
		response  string
		wantScore float64 // 近似值
	}{
		{
			name:      "完全匹配",
			reference: "the cat sat on the mat",
			response:  "the cat sat on the mat",
			wantScore: 1.0,
		},
		{
			name:      "部分匹配",
			reference: "the cat sat on the mat",
			response:  "the cat is on the mat",
			wantScore: 0.5, // 近似
		},
		{
			name:      "完全不匹配",
			reference: "the cat sat on the mat",
			response:  "hello world",
			wantScore: 0.0,
		},
		{
			name:      "空响应",
			reference: "the cat sat on the mat",
			response:  "",
			wantScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := evaluate.EvalInput{
				Reference: tt.reference,
				Response:  tt.response,
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if result == nil {
				t.Fatal("Evaluate() returned nil result")
			}

			// 检查分数范围
			if result.Score < 0 || result.Score > 1 {
				t.Errorf("Score out of range: got %v, want [0, 1]", result.Score)
			}

			// 完全匹配和完全不匹配的情况精确检查
			if tt.name == "完全匹配" && result.Score != tt.wantScore {
				t.Errorf("Score = %v, want %v", result.Score, tt.wantScore)
			}
			if tt.name == "完全不匹配" && result.Score != tt.wantScore {
				t.Errorf("Score = %v, want %v", result.Score, tt.wantScore)
			}
		})
	}
}

func TestROUGEEvaluator(t *testing.T) {
	tests := []struct {
		name    string
		variant string
	}{
		{"ROUGE-1", "rouge-1"},
		{"ROUGE-2", "rouge-2"},
		{"ROUGE-L", "rouge-l"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := NewROUGEEvaluator(
				WithROUGEVariant(tt.variant),
				WithROUGEThreshold(0.3),
			)

			input := evaluate.EvalInput{
				Reference: "the cat sat on the mat",
				Response:  "the cat is on the mat",
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if result.Score < 0 || result.Score > 1 {
				t.Errorf("Score out of range: got %v, want [0, 1]", result.Score)
			}

			// 检查 Details
			if result.Details == nil {
				t.Error("Details is nil")
			}
		})
	}
}

func TestF1Evaluator(t *testing.T) {
	evaluator := NewF1Evaluator(WithF1Threshold(0.7))

	input := evaluate.EvalInput{
		Reference: "machine learning artificial intelligence",
		Response:  "artificial intelligence deep learning",
	}

	result, err := evaluator.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if result.Score < 0 || result.Score > 1 {
		t.Errorf("Score out of range: got %v, want [0, 1]", result.Score)
	}

	// F1 应该在 0 和 1 之间
	if result.Details == nil {
		t.Error("Details is nil")
	}

	precision, ok := result.Details["precision"].(float64)
	if !ok {
		t.Error("precision not found in details")
	}
	if precision < 0 || precision > 1 {
		t.Errorf("precision out of range: got %v", precision)
	}

	recall, ok := result.Details["recall"].(float64)
	if !ok {
		t.Error("recall not found in details")
	}
	if recall < 0 || recall > 1 {
		t.Errorf("recall out of range: got %v", recall)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // token count
	}{
		{"简单句子", "hello world", 2},
		{"标点符号", "hello, world!", 2},
		{"多空格", "hello   world", 2},
		{"空字符串", "", 0},
		{"中英混合", "hello 世界", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != tt.want {
				t.Errorf("tokenize() = %v tokens, want %v tokens", len(got), tt.want)
			}
		})
	}
}

func TestLongestCommonSubsequence(t *testing.T) {
	tests := []struct {
		name string
		seq1 []string
		seq2 []string
		want int
	}{
		{
			name: "完全匹配",
			seq1: []string{"a", "b", "c"},
			seq2: []string{"a", "b", "c"},
			want: 3,
		},
		{
			name: "部分匹配",
			seq1: []string{"a", "b", "c", "d"},
			seq2: []string{"a", "c", "d"},
			want: 3,
		},
		{
			name: "无匹配",
			seq1: []string{"a", "b"},
			seq2: []string{"c", "d"},
			want: 0,
		},
		{
			name: "空序列",
			seq1: []string{},
			seq2: []string{"a", "b"},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := longestCommonSubsequence(tt.seq1, tt.seq2)
			if got != tt.want {
				t.Errorf("longestCommonSubsequence() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkTokenize(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog. This is a benchmark test for tokenization performance."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tokenize(text)
	}
}

func BenchmarkLCS(b *testing.B) {
	seq1 := []string{"the", "quick", "brown", "fox", "jumps", "over", "the", "lazy", "dog"}
	seq2 := []string{"the", "fast", "brown", "fox", "jumps", "over", "a", "lazy", "dog"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = longestCommonSubsequence(seq1, seq2)
	}
}
