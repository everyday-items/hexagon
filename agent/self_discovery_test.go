package agent

import (
	"context"
	"testing"

	"github.com/everyday-items/ai-core/llm"
)

func TestNewSelfDiscovery(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewSelfDiscovery(
		[]Option{
			WithLLM(provider),
			WithName("TestSelfDiscoveryAgent"),
		},
		WithSelfDiscoveryMaxModules(5),
	)

	if agent == nil {
		t.Fatal("NewSelfDiscovery returned nil")
	}

	if agent.Name() != "TestSelfDiscoveryAgent" {
		t.Errorf("expected name 'TestSelfDiscoveryAgent', got %s", agent.Name())
	}

	if agent.maxModules != 5 {
		t.Errorf("expected maxModules=5, got %d", agent.maxModules)
	}
}

func TestNewSelfDiscovery_DefaultValues(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewSelfDiscovery(
		[]Option{WithLLM(provider)},
	)

	if agent.maxModules != 3 {
		t.Errorf("expected default maxModules=3, got %d", agent.maxModules)
	}

	// 应该有默认模块
	if len(agent.modules) == 0 {
		t.Error("expected default modules")
	}

	if agent.Name() != "SelfDiscoveryAgent" {
		t.Errorf("expected default name 'SelfDiscoveryAgent', got %s", agent.Name())
	}
}

func TestSelfDiscovery_WithCustomModules(t *testing.T) {
	provider := &mockLLMProvider{}

	customModule := ReasoningModule{
		Name:        "自定义模块",
		Description: "用于测试的自定义推理模块",
		Template:    "1. 步骤一\n2. 步骤二",
	}

	agent := NewSelfDiscovery(
		[]Option{WithLLM(provider)},
		WithSelfDiscoveryModules(customModule),
	)

	found := false
	for _, m := range agent.modules {
		if m.Name == "自定义模块" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected custom module to be added")
	}
}

func TestSelfDiscovery_Run(t *testing.T) {
	// 模拟 LLM 响应
	callCount := 0
	provider := &mockLLMProviderWithCallback{
		callback: func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			callCount++
			switch callCount {
			case 1: // SELECT 阶段
				return &llm.CompletionResponse{
					Content: `{"selected_modules": [{"name": "逐步推理", "reason": "适合分解问题"}]}`,
					Usage:   llm.Usage{TotalTokens: 50},
				}, nil
			case 2: // ADAPT 阶段
				return &llm.CompletionResponse{
					Content: `{"adapted_modules": [{"name": "逐步推理", "adapted_steps": ["理解问题", "分析数据", "得出结论"]}]}`,
					Usage:   llm.Usage{TotalTokens: 60},
				}, nil
			case 3: // IMPLEMENT 阶段
				return &llm.CompletionResponse{
					Content: "推理结构：\n1. 首先理解问题\n2. 分析相关数据\n3. 得出结论",
					Usage:   llm.Usage{TotalTokens: 70},
				}, nil
			case 4: // EXECUTE 阶段
				return &llm.CompletionResponse{
					Content: "根据分析，答案是42。",
					Usage:   llm.Usage{TotalTokens: 80},
				}, nil
			default:
				return &llm.CompletionResponse{Content: "默认响应"}, nil
			}
		},
	}

	agent := NewSelfDiscovery(
		[]Option{WithLLM(provider)},
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "计算问题"})
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
		selectedModules, ok := output.Metadata["selected_modules"].([]string)
		if !ok {
			t.Error("expected selected_modules in metadata")
		} else if len(selectedModules) == 0 {
			t.Error("expected at least one selected module")
		}

		if output.Metadata["reasoning_structure"] == nil {
			t.Error("expected reasoning_structure in metadata")
		}
	}

	// 检查 Token 使用
	if output.Usage.TotalTokens == 0 {
		t.Error("expected non-zero token usage")
	}
}

func TestSelfDiscovery_SelectModules(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"selected_modules": [{"name": "批判性思维", "reason": "需要评估论点"}]}`,
	}

	agent := NewSelfDiscovery(
		[]Option{WithLLM(provider)},
	)

	ctx := context.Background()
	modules, _, err := agent.selectModules(ctx, Input{Query: "评估这个论点"})
	if err != nil {
		t.Fatalf("selectModules failed: %v", err)
	}

	if len(modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(modules))
	}

	if len(modules) > 0 && modules[0].Name != "批判性思维" {
		t.Errorf("expected '批判性思维', got %s", modules[0].Name)
	}
}

func TestSelfDiscovery_AdaptModules(t *testing.T) {
	provider := &mockLLMProvider{
		response: `{"adapted_modules": [{"name": "逐步推理", "adapted_steps": ["步骤A", "步骤B"]}]}`,
	}

	agent := NewSelfDiscovery(
		[]Option{WithLLM(provider)},
	)

	ctx := context.Background()
	modules := []ReasoningModule{StepByStepModule}
	adapted, _, err := agent.adaptModules(ctx, Input{Query: "测试"}, modules)
	if err != nil {
		t.Fatalf("adaptModules failed: %v", err)
	}

	if len(adapted) != 1 {
		t.Errorf("expected 1 adapted module, got %d", len(adapted))
	}

	if len(adapted) > 0 && len(adapted[0].Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(adapted[0].Steps))
	}
}

func TestDefaultReasoningModules(t *testing.T) {
	// 验证默认模块都有必要的字段
	for _, m := range DefaultReasoningModules {
		if m.Name == "" {
			t.Error("module has empty name")
		}
		if m.Description == "" {
			t.Errorf("module %s has empty description", m.Name)
		}
		if m.Template == "" {
			t.Errorf("module %s has empty template", m.Name)
		}
	}

	// 验证默认模块数量
	expectedCount := 8
	if len(DefaultReasoningModules) != expectedCount {
		t.Errorf("expected %d default modules, got %d", expectedCount, len(DefaultReasoningModules))
	}
}

func TestMergeUsage(t *testing.T) {
	a := llm.Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}
	b := llm.Usage{
		PromptTokens:     5,
		CompletionTokens: 15,
		TotalTokens:      20,
	}

	result := mergeUsage(a, b)

	if result.PromptTokens != 15 {
		t.Errorf("expected PromptTokens=15, got %d", result.PromptTokens)
	}
	if result.CompletionTokens != 35 {
		t.Errorf("expected CompletionTokens=35, got %d", result.CompletionTokens)
	}
	if result.TotalTokens != 50 {
		t.Errorf("expected TotalTokens=50, got %d", result.TotalTokens)
	}
}

func TestGetModuleNames(t *testing.T) {
	modules := []ReasoningModule{
		{Name: "模块A"},
		{Name: "模块B"},
		{Name: "模块C"},
	}

	names := getModuleNames(modules)

	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}

	expected := []string{"模块A", "模块B", "模块C"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], name)
		}
	}
}
