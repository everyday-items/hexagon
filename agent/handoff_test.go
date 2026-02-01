package agent

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/testing/mock"
)

func TestTransferToTool(t *testing.T) {
	targetAgent := NewBaseAgent(WithName("target-agent"))

	tool := TransferTo(targetAgent)

	if tool == nil {
		t.Fatal("expected non-nil transfer tool")
	}

	if tool.Name() != "transfer_to_target-agent" {
		t.Errorf("expected tool name 'transfer_to_target-agent', got '%s'", tool.Name())
	}
}

func TestTransferToToolDescription(t *testing.T) {
	targetAgent := NewBaseAgent(
		WithName("helper"),
		WithDescription("A helpful assistant"),
	)

	tool := TransferTo(targetAgent)

	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
}

func TestTransferToToolSchema(t *testing.T) {
	targetAgent := NewBaseAgent(WithName("target"))

	tool := TransferTo(targetAgent)

	schema := tool.Schema()
	if schema == nil {
		t.Error("expected non-nil schema")
	}
}

func TestTransferToToolValidate(t *testing.T) {
	targetAgent := NewBaseAgent(WithName("target"))
	tool := TransferTo(targetAgent)

	// 缺少 message 应该失败
	err := tool.Validate(map[string]any{})
	if err == nil {
		t.Error("expected error when message is missing")
	}

	// 有 message 应该成功
	err = tool.Validate(map[string]any{"message": "Hello"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTransferToToolExecute(t *testing.T) {
	targetAgent := NewBaseAgent(WithName("target"))
	tool := TransferTo(targetAgent)

	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "Please help with this",
		"reason":  "Need specialized help",
		"context": map[string]any{"key": "value"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected successful result")
	}

	handoff, ok := result.Output.(Handoff)
	if !ok {
		t.Fatal("expected Handoff in result output")
	}

	if handoff.TargetAgent.Name() != "target" {
		t.Errorf("expected target agent 'target', got '%s'", handoff.TargetAgent.Name())
	}

	if handoff.Message != "Please help with this" {
		t.Errorf("expected message 'Please help with this', got '%s'", handoff.Message)
	}

	if handoff.Reason != "Need specialized help" {
		t.Errorf("expected reason 'Need specialized help', got '%s'", handoff.Reason)
	}
}

func TestHandoffCreation(t *testing.T) {
	agent := NewBaseAgent(WithName("next-agent"))

	handoff := Handoff{
		TargetAgent: agent,
		Message:     "Continue from here",
		Reason:      "Task completed",
		Context:     map[string]any{"key": "value"},
	}

	if handoff.TargetAgent.Name() != "next-agent" {
		t.Errorf("expected agent name 'next-agent', got '%s'", handoff.TargetAgent.Name())
	}

	if handoff.Message != "Continue from here" {
		t.Error("expected message to be set")
	}
}

func TestHandoffHandler(t *testing.T) {
	var receivedHandoff *Handoff

	handler := &HandoffHandler{
		OnHandoff: func(ctx context.Context, h Handoff) error {
			receivedHandoff = &h
			return nil
		},
	}

	targetAgent := NewBaseAgent(WithName("target"))
	tool := TransferTo(targetAgent)

	result, _ := tool.Execute(context.Background(), map[string]any{
		"message": "Test message",
	})

	handoff, err := handler.ProcessToolResult(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if handoff == nil {
		t.Fatal("expected handoff to be detected")
	}

	if receivedHandoff == nil {
		t.Error("expected OnHandoff callback to be called")
	}
}

func TestSwarmRunnerCreation(t *testing.T) {
	initialAgent := NewBaseAgent(WithName("initial"))

	runner := NewSwarmRunner(initialAgent)

	if runner == nil {
		t.Fatal("expected non-nil SwarmRunner")
	}

	if runner.InitialAgent.Name() != "initial" {
		t.Errorf("expected initial agent 'initial', got '%s'", runner.InitialAgent.Name())
	}

	if runner.MaxHandoffs != 10 {
		t.Errorf("expected default max handoffs 10, got %d", runner.MaxHandoffs)
	}
}

func TestSwarmRunnerRun(t *testing.T) {
	mockLLM := mock.FixedProvider("Hello from initial agent")

	initialAgent := NewReAct(
		WithName("initial"),
		WithLLM(mockLLM),
	)

	runner := NewSwarmRunner(initialAgent)

	output, err := runner.Run(context.Background(), Input{Query: "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Content != "Hello from initial agent" {
		t.Errorf("expected response content, got '%s'", output.Content)
	}
}

func TestSwarmRunnerWithContextVariables(t *testing.T) {
	mockLLM := mock.FixedProvider("Response with context")

	initialAgent := NewReAct(
		WithName("context-agent"),
		WithLLM(mockLLM),
	)

	runner := NewSwarmRunner(initialAgent)

	initialVars := ContextVariables{
		"user_id": "123",
		"session": "abc",
	}

	ctx := ContextWithVariables(context.Background(), initialVars)

	output, err := runner.Run(ctx, Input{Query: "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestSwarmRunnerContextCancellation(t *testing.T) {
	mockLLM := mock.FixedProvider("Should not return")

	initialAgent := NewReAct(
		WithName("cancel-agent"),
		WithLLM(mockLLM),
	)

	runner := NewSwarmRunner(initialAgent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := runner.Run(ctx, Input{Query: "Hello"})
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}

func TestContextWithVariablesIntegration(t *testing.T) {
	ctx := context.Background()
	vars := ContextVariables{
		"key1": "value1",
		"key2": 42,
	}

	ctx = ContextWithVariables(ctx, vars)

	retrieved := VariablesFromContext(ctx)
	if retrieved == nil {
		t.Fatal("expected variables in context")
	}

	val, ok := retrieved.Get("key1")
	if !ok || val != "value1" {
		t.Error("expected key1 value")
	}
}

func TestContextVariablesOperations(t *testing.T) {
	vars := ContextVariables{
		"initial": "value",
	}

	// Test Set
	vars.Set("new_key", "new_value")
	val, ok := vars.Get("new_key")
	if !ok || val != "new_value" {
		t.Error("expected Set to work")
	}

	// Test Clone
	cloned := vars.Clone()
	cloned.Set("cloned_only", "value")

	_, exists := vars.Get("cloned_only")
	if exists {
		t.Error("clone should not affect original")
	}

	// Test Merge
	other := ContextVariables{"merged": "data"}
	vars.Merge(other)

	val, ok = vars.Get("merged")
	if !ok || val != "data" {
		t.Error("expected Merge to work")
	}
}
