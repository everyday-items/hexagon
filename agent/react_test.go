package agent

import (
	"context"
	"testing"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/testing/mock"
)

func TestReActAgentCreation(t *testing.T) {
	agent := NewReAct(
		WithName("react-test"),
		WithDescription("Test ReAct Agent"),
	)

	if agent.Name() != "react-test" {
		t.Errorf("expected name 'react-test', got '%s'", agent.Name())
	}

	if agent.Description() != "Test ReAct Agent" {
		t.Errorf("expected description, got '%s'", agent.Description())
	}
}

func TestReActAgentDefaultName(t *testing.T) {
	agent := NewReAct()

	if agent.Name() != "ReActAgent" {
		t.Errorf("expected default name 'ReActAgent', got '%s'", agent.Name())
	}
}

func TestReActAgentDefaultSystemPrompt(t *testing.T) {
	agent := NewReAct()

	config := agent.Config()
	if config.SystemPrompt == "" {
		t.Error("expected default system prompt to be set")
	}
}

func TestReActAgentRunWithoutLLM(t *testing.T) {
	agent := NewReAct(WithName("no-llm"))

	_, err := agent.Run(context.Background(), Input{Query: "Hello"})
	if err == nil {
		t.Error("expected error when LLM not configured")
	}
}

func TestReActAgentSimpleRun(t *testing.T) {
	mockLLM := mock.FixedProvider("This is a ReAct response")

	agent := NewReAct(
		WithName("react-simple"),
		WithLLM(mockLLM),
	)

	output, err := agent.Run(context.Background(), Input{Query: "What is 2+2?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Content != "This is a ReAct response" {
		t.Errorf("expected response content, got '%s'", output.Content)
	}
}

func TestReActAgentWithToolCall(t *testing.T) {
	// 创建模拟 LLM，先返回工具调用，再返回最终响应
	mockLLM := mock.NewLLMProvider("react-tool")
	mockLLM.AddToolCallResponse([]llm.ToolCall{
		{
			ID:        "call_1",
			Type:      "function",
			Name:      "calculator",
			Arguments: `{"expression": "2+2"}`,
		},
	})
	mockLLM.AddResponse("The answer is 4")

	// 创建模拟工具，使用正确的 API
	mockTool := mock.NewTool("calculator", mock.WithToolDescription("Perform calculations"))
	mockTool.AddResult(map[string]any{"result": 4})

	agent := NewReAct(
		WithName("react-tool"),
		WithLLM(mockLLM),
		WithTools(mockTool),
	)

	output, err := agent.Run(context.Background(), Input{Query: "What is 2+2?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证工具被调用
	if mockTool.CallCount() != 1 {
		t.Errorf("expected tool to be called once, got %d", mockTool.CallCount())
	}

	// 验证有工具调用记录
	if len(output.ToolCalls) == 0 {
		t.Error("expected tool calls in output")
	}

	if output.Content != "The answer is 4" {
		t.Errorf("expected 'The answer is 4', got '%s'", output.Content)
	}
}

func TestReActAgentMaxIterations(t *testing.T) {
	// 创建总是返回工具调用的 LLM
	mockLLM := mock.NewLLMProvider("infinite-tool")
	for i := 0; i < 20; i++ {
		mockLLM.AddToolCallResponse([]llm.ToolCall{
			{
				ID:        "call",
				Type:      "function",
				Name:      "test_tool",
				Arguments: "{}",
			},
		})
	}

	// 使用 FixedTool 创建总是返回相同结果的工具
	mockTool := mock.FixedTool("test_tool", "ok")

	agent := NewReAct(
		WithName("max-iter"),
		WithLLM(mockLLM),
		WithTools(mockTool),
		WithMaxIterations(3),
	)

	_, err := agent.Run(context.Background(), Input{Query: "Loop test"})
	// 应该在达到最大迭代次数后停止，但不一定报错
	// 主要验证不会无限循环
	_ = err

	// 验证工具调用次数不超过最大迭代次数
	if mockTool.CallCount() > 3 {
		t.Errorf("expected at most 3 tool calls, got %d", mockTool.CallCount())
	}
}

func TestReActAgentWithRole(t *testing.T) {
	role := Role{
		Name:      "Mathematician",
		Goal:      "Solve math problems",
		Backstory: "Expert in mathematics",
	}

	agent := NewReAct(
		WithRole(role),
	)

	if agent.Role().Name != "Mathematician" {
		t.Errorf("expected role 'Mathematician', got '%s'", agent.Role().Name)
	}
}

func TestReActAgentStream(t *testing.T) {
	mockLLM := mock.FixedProvider("Streaming ReAct response")

	agent := NewReAct(
		WithName("react-stream"),
		WithLLM(mockLLM),
	)

	stream, err := agent.Stream(context.Background(), Input{Query: "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputs, err := stream.Collect(context.Background())
	if err != nil {
		t.Fatalf("failed to collect stream: %v", err)
	}

	if len(outputs) == 0 {
		t.Error("expected at least one output")
	}
}
