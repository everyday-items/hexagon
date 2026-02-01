package hooks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// mockRunHook 模拟运行钩子
type mockRunHook struct {
	name       string
	enabled    bool
	startCount int32
	endCount   int32
	errorCount int32
}

func (h *mockRunHook) Name() string    { return h.name }
func (h *mockRunHook) Enabled() bool   { return h.enabled }
func (h *mockRunHook) OnStart(ctx context.Context, event *RunStartEvent) error {
	atomic.AddInt32(&h.startCount, 1)
	return nil
}
func (h *mockRunHook) OnEnd(ctx context.Context, event *RunEndEvent) error {
	atomic.AddInt32(&h.endCount, 1)
	return nil
}
func (h *mockRunHook) OnError(ctx context.Context, event *ErrorEvent) error {
	atomic.AddInt32(&h.errorCount, 1)
	return nil
}

// mockToolHook 模拟工具钩子
type mockToolHook struct {
	name       string
	enabled    bool
	startCount int32
	endCount   int32
}

func (h *mockToolHook) Name() string    { return h.name }
func (h *mockToolHook) Enabled() bool   { return h.enabled }
func (h *mockToolHook) OnToolStart(ctx context.Context, event *ToolStartEvent) error {
	atomic.AddInt32(&h.startCount, 1)
	return nil
}
func (h *mockToolHook) OnToolEnd(ctx context.Context, event *ToolEndEvent) error {
	atomic.AddInt32(&h.endCount, 1)
	return nil
}

func TestHookManager_RegisterAndTrigger(t *testing.T) {
	manager := NewManager()
	hook := &mockRunHook{name: "test-hook", enabled: true}

	manager.RegisterRunHook(hook)

	ctx := context.Background()

	// 测试触发开始事件
	err := manager.TriggerRunStart(ctx, &RunStartEvent{
		RunID:   "run-1",
		AgentID: "agent-1",
		Input:   "test input",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hook.startCount != 1 {
		t.Errorf("expected startCount 1, got %d", hook.startCount)
	}

	// 测试触发结束事件
	err = manager.TriggerRunEnd(ctx, &RunEndEvent{
		RunID:    "run-1",
		AgentID:  "agent-1",
		Output:   "test output",
		Duration: 100,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hook.endCount != 1 {
		t.Errorf("expected endCount 1, got %d", hook.endCount)
	}
}

func TestHookManager_DisabledHook(t *testing.T) {
	manager := NewManager()
	hook := &mockRunHook{name: "disabled-hook", enabled: false}

	manager.RegisterRunHook(hook)

	ctx := context.Background()
	err := manager.TriggerRunStart(ctx, &RunStartEvent{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 禁用的钩子不应被调用
	if hook.startCount != 0 {
		t.Errorf("expected startCount 0 for disabled hook, got %d", hook.startCount)
	}
}

func TestHookManager_ToolHooks(t *testing.T) {
	manager := NewManager()
	hook := &mockToolHook{name: "tool-hook", enabled: true}

	manager.RegisterToolHook(hook)

	ctx := context.Background()

	// 测试工具开始事件
	err := manager.TriggerToolStart(ctx, &ToolStartEvent{
		RunID:    "run-1",
		ToolName: "calculator",
		ToolID:   "tool-1",
		Input:    map[string]any{"a": 1, "b": 2},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hook.startCount != 1 {
		t.Errorf("expected startCount 1, got %d", hook.startCount)
	}

	// 测试工具结束事件
	err = manager.TriggerToolEnd(ctx, &ToolEndEvent{
		RunID:    "run-1",
		ToolName: "calculator",
		ToolID:   "tool-1",
		Output:   3,
		Duration: 10,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hook.endCount != 1 {
		t.Errorf("expected endCount 1, got %d", hook.endCount)
	}
}

func TestHookManager_ErrorEvent(t *testing.T) {
	manager := NewManager()
	hook := &mockRunHook{name: "error-hook", enabled: true}

	manager.RegisterRunHook(hook)

	ctx := context.Background()
	testErr := errors.New("test error")

	err := manager.TriggerError(ctx, &ErrorEvent{
		RunID:   "run-1",
		AgentID: "agent-1",
		Error:   testErr,
		Phase:   "tool_execution",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hook.errorCount != 1 {
		t.Errorf("expected errorCount 1, got %d", hook.errorCount)
	}
}

func TestHookManager_Context(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// 测试添加到 context
	ctx = ContextWithManager(ctx, manager)

	// 测试从 context 获取
	retrieved := ManagerFromContext(ctx)
	if retrieved == nil {
		t.Error("expected manager from context")
	}
	if retrieved != manager {
		t.Error("expected same manager instance")
	}

	// 测试空 context
	emptyCtx := context.Background()
	nilManager := ManagerFromContext(emptyCtx)
	if nilManager != nil {
		t.Error("expected nil manager from empty context")
	}
}

func TestRunStartEvent(t *testing.T) {
	event := &RunStartEvent{
		RunID:    "run-123",
		AgentID:  "agent-456",
		Input:    "test query",
		Metadata: map[string]any{"key": "value"},
	}

	if event.RunID != "run-123" {
		t.Errorf("expected RunID 'run-123', got '%s'", event.RunID)
	}
	if event.Metadata["key"] != "value" {
		t.Error("expected metadata to contain key=value")
	}
}

func TestToolStartEvent(t *testing.T) {
	event := &ToolStartEvent{
		RunID:    "run-123",
		ToolName: "web_search",
		ToolID:   "tool-789",
		Input:    map[string]any{"query": "golang"},
	}

	if event.ToolName != "web_search" {
		t.Errorf("expected ToolName 'web_search', got '%s'", event.ToolName)
	}
	if event.Input["query"] != "golang" {
		t.Error("expected input query to be 'golang'")
	}
}
