package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/everyday-items/hexagon/testing/mock"
)

func TestBaseAgentRun(t *testing.T) {
	mockLLM := mock.FixedProvider("Hello, I'm an AI assistant!")

	agent := NewBaseAgent(
		WithName("test-agent"),
		WithLLM(mockLLM),
		WithSystemPrompt("You are a helpful assistant"),
	)

	output, err := agent.Run(context.Background(), Input{Query: "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Content != "Hello, I'm an AI assistant!" {
		t.Errorf("expected response content, got '%s'", output.Content)
	}

	// 验证 LLM 被调用
	if mockLLM.CallCount() != 1 {
		t.Errorf("expected 1 LLM call, got %d", mockLLM.CallCount())
	}

	// 验证消息包含系统提示词
	lastCall := mockLLM.LastCall()
	if lastCall == nil {
		t.Fatal("expected LLM call record")
	}

	if len(lastCall.Messages) != 2 {
		t.Errorf("expected 2 messages (system + user), got %d", len(lastCall.Messages))
	}
}

func TestBaseAgentRunWithoutLLM(t *testing.T) {
	agent := NewBaseAgent(WithName("no-llm-agent"))

	_, err := agent.Run(context.Background(), Input{Query: "Hello"})
	if err == nil {
		t.Error("expected error when LLM not configured")
	}
}

func TestBaseAgentRunWithLLMError(t *testing.T) {
	expectedErr := errors.New("LLM service unavailable")
	mockLLM := mock.ErrorProvider(expectedErr)

	agent := NewBaseAgent(
		WithName("error-agent"),
		WithLLM(mockLLM),
	)

	_, err := agent.Run(context.Background(), Input{Query: "Hello"})
	if err == nil {
		t.Error("expected error from LLM")
	}
}

func TestBaseAgentStream(t *testing.T) {
	mockLLM := mock.FixedProvider("Streaming response")

	agent := NewBaseAgent(
		WithName("stream-agent"),
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

	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(outputs))
	}

	if outputs[0].Content != "Streaming response" {
		t.Errorf("expected 'Streaming response', got '%s'", outputs[0].Content)
	}
}

func TestBaseAgentBatch(t *testing.T) {
	mockLLM := mock.NewLLMProvider("batch")
	mockLLM.AddResponse("Response 1")
	mockLLM.AddResponse("Response 2")
	mockLLM.AddResponse("Response 3")

	agent := NewBaseAgent(
		WithName("batch-agent"),
		WithLLM(mockLLM),
	)

	inputs := []Input{
		{Query: "Question 1"},
		{Query: "Question 2"},
		{Query: "Question 3"},
	}

	outputs, err := agent.Batch(context.Background(), inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(outputs) != 3 {
		t.Errorf("expected 3 outputs, got %d", len(outputs))
	}

	expectedResponses := []string{"Response 1", "Response 2", "Response 3"}
	for i, output := range outputs {
		if output.Content != expectedResponses[i] {
			t.Errorf("output %d: expected '%s', got '%s'", i, expectedResponses[i], output.Content)
		}
	}
}

func TestBaseAgentSchema(t *testing.T) {
	agent := NewBaseAgent(WithName("schema-agent"))

	inputSchema := agent.InputSchema()
	if inputSchema == nil {
		t.Error("expected non-nil input schema")
	}

	outputSchema := agent.OutputSchema()
	if outputSchema == nil {
		t.Error("expected non-nil output schema")
	}
}

func TestBaseAgentWithTools(t *testing.T) {
	mockLLM := mock.FixedProvider("Tool test response")

	agent := NewBaseAgent(
		WithName("tool-agent"),
		WithLLM(mockLLM),
	)

	// 验证工具列表初始为空
	if len(agent.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(agent.Tools()))
	}
}

func TestBaseAgentWithMemory(t *testing.T) {
	agent := NewBaseAgent(WithName("memory-agent"))

	// 验证默认内存不为空
	if agent.Memory() == nil {
		t.Error("expected default memory to be set")
	}
}

func TestBaseAgentContextCancellation(t *testing.T) {
	mockLLM := mock.FixedProvider("Should not return")

	agent := NewBaseAgent(
		WithName("cancel-agent"),
		WithLLM(mockLLM),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := agent.Run(ctx, Input{Query: "Hello"})
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}
