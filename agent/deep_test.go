package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/everyday-items/ai-core/llm"
)

// ============== DeepAgent 测试 ==============

func TestDeepAgent_Creation(t *testing.T) {
	base := newMockAgent("base", nil)
	deep := NewDeepAgent("test-deep", base)

	if deep.Name() != "test-deep" {
		t.Fatalf("expected name 'test-deep', got %q", deep.Name())
	}
	if deep.ID() == "" {
		t.Fatal("expected non-empty ID")
	}
	if deep.maxDepth != 3 {
		t.Fatalf("expected default maxDepth=3, got %d", deep.maxDepth)
	}
}

func TestDeepAgent_WithMaxDepth(t *testing.T) {
	base := newMockAgent("base", nil)
	deep := NewDeepAgent("test", base, WithMaxDepth(5))

	if deep.maxDepth != 5 {
		t.Fatalf("expected maxDepth=5, got %d", deep.maxDepth)
	}
}

func TestDeepAgent_WithSubAgentFactory(t *testing.T) {
	base := newMockAgent("base", nil)
	factoryCalled := false

	deep := NewDeepAgent("test", base, WithSubAgentFactory(func(task string) Agent {
		factoryCalled = true
		return newMockAgent("sub", nil)
	}))

	if deep.subAgentFn == nil {
		t.Fatal("expected non-nil subAgentFn")
	}
	_ = factoryCalled
}

func TestDeepAgent_NoLLM(t *testing.T) {
	base := newMockAgent("base", nil)
	deep := NewDeepAgent("test", base)

	_, err := deep.Run(context.Background(), Input{Query: "test"})
	if err == nil {
		t.Fatal("expected error for nil LLM")
	}
	if !strings.Contains(err.Error(), "LLM not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeepAgent_DirectAnswer(t *testing.T) {
	// LLM 直接回答，不调用任何工具
	mockLLM := &svMockLLM{
		completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Content: "Direct answer: 42",
			}, nil
		},
	}

	base := newSvMockAgent("base", mockLLM, nil)
	deep := NewDeepAgent("test", base)

	output, err := deep.Run(context.Background(), Input{Query: "what is 42?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Content != "Direct answer: 42" {
		t.Fatalf("expected 'Direct answer: 42', got %q", output.Content)
	}
	if output.Metadata["agent_type"] != "deep" {
		t.Fatal("expected agent_type=deep in metadata")
	}
	if output.Metadata["depth"] != 0 {
		t.Fatalf("expected depth=0, got %v", output.Metadata["depth"])
	}
}

func TestDeepAgent_CreateSubtask(t *testing.T) {
	callCount := 0

	// 第一次：创建子任务
	// 第二次（depth=1 子任务）：直接回答
	// 第三次（depth=0 接收子任务结果后）：给出最终答案
	mockLLM := &svMockLLM{
		completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			callCount++

			// 检查是否为子任务调用（深度 > 0 的消息有 [Subtask] 前缀）
			for _, msg := range req.Messages {
				if msg.Role == llm.RoleUser && strings.Contains(msg.Content, "[Subtask") {
					return &llm.CompletionResponse{
						Content: "Sub-result: AI is transformative",
					}, nil
				}
			}

			// 检查是否已有 tool 结果（第三次调用）
			for _, msg := range req.Messages {
				if msg.Role == llm.RoleTool {
					return &llm.CompletionResponse{
						Content: "Final: " + msg.Content,
					}, nil
				}
			}

			// 第一次调用：创建子任务
			return &llm.CompletionResponse{
				Content: "Let me break this down",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call-1",
						Name:      "create_subtask",
						Arguments: `{"task":"research about AI impact"}`,
					},
				},
			}, nil
		},
	}

	base := newSvMockAgent("base", mockLLM, nil)
	deep := NewDeepAgent("test", base, WithMaxDepth(3))

	output, err := deep.Run(context.Background(), Input{Query: "analyze AI"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.Content, "Final:") {
		t.Fatalf("expected final answer, got %q", output.Content)
	}
	if len(output.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(output.ToolCalls))
	}
	if output.ToolCalls[0].Name != "create_subtask" {
		t.Fatalf("expected tool name 'create_subtask', got %q", output.ToolCalls[0].Name)
	}
}

func TestDeepAgent_MaxDepthPreventsSubtask(t *testing.T) {
	// 在最大深度时，不应该提供 create_subtask 工具
	mockLLM := &svMockLLM{
		completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			// 检查是否有 create_subtask 工具
			hasSubtask := false
			for _, t := range req.Tools {
				if t.Function.Name == "create_subtask" {
					hasSubtask = true
				}
			}

			if hasSubtask {
				return &llm.CompletionResponse{
					Content: "has_subtask_tool",
				}, nil
			}
			return &llm.CompletionResponse{
				Content: "no_subtask_tool",
			}, nil
		},
	}

	base := newSvMockAgent("base", mockLLM, nil)

	// maxDepth=1 意味着 depth 0 处不应有 create_subtask（因为 0 < 1-1 == false）
	deep := NewDeepAgent("test", base, WithMaxDepth(1))

	output, err := deep.Run(context.Background(), Input{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Content != "no_subtask_tool" {
		t.Fatalf("expected 'no_subtask_tool' at maxDepth=1, got %q", output.Content)
	}
}

func TestDeepAgent_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	base := newSvMockAgent("base", &svMockLLM{}, nil)
	deep := NewDeepAgent("test", base)

	_, err := deep.Run(ctx, Input{Query: "test"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestDeepAgent_LLMError(t *testing.T) {
	mockLLM := &svMockLLM{
		completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return nil, fmt.Errorf("LLM error")
		},
	}

	base := newSvMockAgent("base", mockLLM, nil)
	deep := NewDeepAgent("test", base)

	_, err := deep.Run(context.Background(), Input{Query: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "LLM failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeepAgent_Invoke(t *testing.T) {
	mockLLM := &svMockLLM{
		completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{Content: "ok"}, nil
		},
	}
	base := newSvMockAgent("base", mockLLM, nil)
	deep := NewDeepAgent("test", base)

	output, err := deep.Invoke(context.Background(), Input{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Content != "ok" {
		t.Fatalf("expected 'ok', got %q", output.Content)
	}
}

func TestDeepAgent_AgentInterface(t *testing.T) {
	var _ Agent = (*DeepAgent)(nil)
}
