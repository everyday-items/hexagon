package agent

import (
	"context"
	"testing"

	"github.com/everyday-items/ai-core/llm"
)

func TestNewReflection(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewReflection(
		[]Option{
			WithLLM(provider),
			WithName("TestReflectionAgent"),
		},
		WithReflectionMaxIterations(5),
		WithReflectionQualityTarget(0.9),
	)

	if agent == nil {
		t.Fatal("NewReflection returned nil")
	}

	if agent.Name() != "TestReflectionAgent" {
		t.Errorf("expected name 'TestReflectionAgent', got %s", agent.Name())
	}

	if agent.maxIterations != 5 {
		t.Errorf("expected maxIterations=5, got %d", agent.maxIterations)
	}

	if agent.qualityTarget != 0.9 {
		t.Errorf("expected qualityTarget=0.9, got %f", agent.qualityTarget)
	}
}

func TestNewReflection_DefaultValues(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewReflection(
		[]Option{WithLLM(provider)},
	)

	if agent.maxIterations != 3 {
		t.Errorf("expected default maxIterations=3, got %d", agent.maxIterations)
	}

	if agent.qualityTarget != 0.8 {
		t.Errorf("expected default qualityTarget=0.8, got %f", agent.qualityTarget)
	}

	if agent.minIterations != 1 {
		t.Errorf("expected default minIterations=1, got %d", agent.minIterations)
	}

	if agent.Name() != "ReflectionAgent" {
		t.Errorf("expected default name 'ReflectionAgent', got %s", agent.Name())
	}
}

func TestReflection_Run_HighQuality(t *testing.T) {
	// 模拟高质量响应，应该一次通过
	provider := &mockLLMProvider{
		responses: []string{
			"这是一个高质量的回答。",
			`{"quality": 0.95, "strengths": ["清晰"], "weaknesses": [], "suggestions": [], "should_retry": false}`,
		},
	}

	agent := NewReflection(
		[]Option{WithLLM(provider)},
		WithReflectionQualityTarget(0.8),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "测试问题"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if output.Content == "" {
		t.Error("expected non-empty output content")
	}

	// 检查元数据
	if output.Metadata == nil {
		t.Error("expected metadata in output")
	} else {
		if output.Metadata["iterations"] == nil {
			t.Error("expected iterations in metadata")
		}
	}
}

func TestReflection_Run_NeedsRetry(t *testing.T) {
	// 模拟需要重试的情况
	callCount := 0
	provider := &mockLLMProviderWithCallback{
		callback: func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			callCount++
			if callCount%2 == 1 {
				// 奇数次调用是任务执行
				return &llm.CompletionResponse{
					Content: "回答版本 " + string(rune('0'+callCount)),
				}, nil
			}
			// 偶数次调用是反思
			if callCount == 2 {
				// 第一次反思，质量不达标
				return &llm.CompletionResponse{
					Content: `{"quality": 0.5, "strengths": [], "weaknesses": ["不完整"], "suggestions": ["添加更多细节"], "should_retry": true, "feedback": "请添加更多细节"}`,
				}, nil
			}
			// 后续反思，质量达标
			return &llm.CompletionResponse{
				Content: `{"quality": 0.9, "strengths": ["完整"], "weaknesses": [], "suggestions": [], "should_retry": false}`,
			}, nil
		},
	}

	agent := NewReflection(
		[]Option{WithLLM(provider)},
		WithReflectionQualityTarget(0.8),
		WithReflectionMaxIterations(5),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "测试问题"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 应该至少有 2 次迭代
	iterations, ok := output.Metadata["iterations"].(int)
	if !ok {
		t.Error("expected iterations in metadata")
	} else if iterations < 2 {
		t.Errorf("expected at least 2 iterations, got %d", iterations)
	}
}

func TestReflection_WithReflector(t *testing.T) {
	provider := &mockLLMProvider{
		response: "测试回答",
	}

	mockReflector := &mockReflector{
		reflection: &Reflection{
			Quality:     0.85,
			Strengths:   []string{"清晰"},
			Weaknesses:  []string{},
			Suggestions: []string{},
			ShouldRetry: false,
		},
	}

	agent := NewReflection(
		[]Option{WithLLM(provider)},
		WithReflector(mockReflector),
		WithReflectionQualityTarget(0.8),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "测试"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 应该使用自定义反思器
	finalQuality, ok := output.Metadata["final_quality"].(float32)
	if !ok {
		t.Error("expected final_quality in metadata")
	} else if finalQuality != 0.85 {
		t.Errorf("expected final_quality=0.85, got %f", finalQuality)
	}
}

func TestReflection_MinIterations(t *testing.T) {
	// 即使质量达标，也要执行最小迭代次数
	callCount := 0
	provider := &mockLLMProviderWithCallback{
		callback: func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			callCount++
			if callCount%2 == 1 {
				return &llm.CompletionResponse{Content: "回答"}, nil
			}
			// 每次反思都返回高质量
			return &llm.CompletionResponse{
				Content: `{"quality": 0.99, "strengths": ["优秀"], "weaknesses": [], "suggestions": [], "should_retry": false}`,
			}, nil
		},
	}

	agent := NewReflection(
		[]Option{WithLLM(provider)},
		WithReflectionMinIterations(2),
		WithReflectionQualityTarget(0.8),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "测试"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	iterations := output.Metadata["iterations"].(int)
	if iterations < 2 {
		t.Errorf("expected at least 2 iterations due to minIterations, got %d", iterations)
	}
}

func TestReflection_BuildFeedback(t *testing.T) {
	agent := &ReflectionAgent{}

	// 有 Feedback 字段时直接使用
	reflection := &Reflection{
		Feedback: "直接反馈",
	}
	feedback := agent.buildFeedback(reflection)
	if feedback != "直接反馈" {
		t.Errorf("expected '直接反馈', got %s", feedback)
	}

	// 没有 Feedback 时从 Weaknesses 和 Suggestions 构建
	reflection = &Reflection{
		Weaknesses:  []string{"问题1", "问题2"},
		Suggestions: []string{"建议1"},
	}
	feedback = agent.buildFeedback(reflection)
	if feedback == "" {
		t.Error("expected non-empty feedback")
	}
}

func TestLLMReflector(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"quality": 0.75, "strengths": ["好"], "weaknesses": ["不足"], "suggestions": ["改进"], "should_retry": true}`,
	}

	reflector := NewLLMReflector(provider)

	ctx := context.Background()
	reflection, err := reflector.Reflect(ctx, Input{Query: "问题"}, Output{Content: "回答"})
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	if reflection.Quality != 0.75 {
		t.Errorf("expected quality=0.75, got %f", reflection.Quality)
	}

	if !reflection.ShouldRetry {
		t.Error("expected should_retry=true")
	}
}

func TestLLMReflector_ScoreQuality(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"quality": 0.88, "strengths": [], "weaknesses": [], "suggestions": [], "should_retry": false}`,
	}

	reflector := NewLLMReflector(provider)

	ctx := context.Background()
	score, err := reflector.ScoreQuality(ctx, Input{Query: "问题"}, Output{Content: "回答"})
	if err != nil {
		t.Fatalf("ScoreQuality failed: %v", err)
	}

	if score != 0.88 {
		t.Errorf("expected score=0.88, got %f", score)
	}
}

// mockReflector 模拟反思器
type mockReflector struct {
	reflection *Reflection
}

func (r *mockReflector) Reflect(ctx context.Context, input Input, output Output) (*Reflection, error) {
	return r.reflection, nil
}

func (r *mockReflector) ScoreQuality(ctx context.Context, input Input, output Output) (float32, error) {
	return r.reflection.Quality, nil
}

// mockLLMProviderWithCallback 支持回调的模拟 LLM Provider
type mockLLMProviderWithCallback struct {
	callback func(req llm.CompletionRequest) (*llm.CompletionResponse, error)
}

func (p *mockLLMProviderWithCallback) Name() string { return "mock_callback" }

func (p *mockLLMProviderWithCallback) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{Name: "mock-model"}}
}

func (p *mockLLMProviderWithCallback) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return p.callback(req)
}

func (p *mockLLMProviderWithCallback) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	return nil, nil
}

func (p *mockLLMProviderWithCallback) CountTokens(messages []llm.Message) (int, error) {
	return 100, nil
}
