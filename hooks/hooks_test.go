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

// ============== TimingChecker 测试 ==============

// timingAwareHook 实现 TimingChecker 的钩子
// 只关心 RunEnd 和 RunError，不关心 RunStart
type timingAwareHook struct {
	name       string
	enabled    bool
	timings    Timing
	startCount int32
	endCount   int32
	errorCount int32
}

func (h *timingAwareHook) Name() string  { return h.name }
func (h *timingAwareHook) Enabled() bool { return h.enabled }
func (h *timingAwareHook) Timings() Timing { return h.timings }

func (h *timingAwareHook) OnStart(ctx context.Context, event *RunStartEvent) error {
	atomic.AddInt32(&h.startCount, 1)
	return nil
}
func (h *timingAwareHook) OnEnd(ctx context.Context, event *RunEndEvent) error {
	atomic.AddInt32(&h.endCount, 1)
	return nil
}
func (h *timingAwareHook) OnError(ctx context.Context, event *ErrorEvent) error {
	atomic.AddInt32(&h.errorCount, 1)
	return nil
}

func TestTimingChecker_OnlyEndTiming(t *testing.T) {
	manager := NewManager()
	// 这个 hook 只关心 RunEnd，不关心 RunStart 和 RunError
	hook := &timingAwareHook{
		name:    "end-only-hook",
		enabled: true,
		timings: TimingRunEnd,
	}

	manager.RegisterRunHook(hook)

	ctx := context.Background()

	// 触发 RunStart - 不应该被调用
	manager.TriggerRunStart(ctx, &RunStartEvent{RunID: "run-1"})
	if hook.startCount != 0 {
		t.Errorf("expected startCount 0 (timing not in Timings), got %d", hook.startCount)
	}

	// 触发 RunEnd - 应该被调用
	manager.TriggerRunEnd(ctx, &RunEndEvent{RunID: "run-1"})
	if hook.endCount != 1 {
		t.Errorf("expected endCount 1, got %d", hook.endCount)
	}

	// 触发 RunError - 不应该被调用
	manager.TriggerError(ctx, &ErrorEvent{RunID: "run-1", Error: errors.New("test")})
	if hook.errorCount != 0 {
		t.Errorf("expected errorCount 0 (timing not in Timings), got %d", hook.errorCount)
	}
}

func TestTimingChecker_MultipleTimings(t *testing.T) {
	manager := NewManager()
	// 这个 hook 关心 RunStart 和 RunError，不关心 RunEnd
	hook := &timingAwareHook{
		name:    "start-error-hook",
		enabled: true,
		timings: TimingRunStart | TimingRunError,
	}

	manager.RegisterRunHook(hook)

	ctx := context.Background()

	// 触发 RunStart - 应该被调用
	manager.TriggerRunStart(ctx, &RunStartEvent{RunID: "run-1"})
	if hook.startCount != 1 {
		t.Errorf("expected startCount 1, got %d", hook.startCount)
	}

	// 触发 RunEnd - 不应该被调用
	manager.TriggerRunEnd(ctx, &RunEndEvent{RunID: "run-1"})
	if hook.endCount != 0 {
		t.Errorf("expected endCount 0 (timing not in Timings), got %d", hook.endCount)
	}

	// 触发 RunError - 应该被调用
	manager.TriggerError(ctx, &ErrorEvent{RunID: "run-1", Error: errors.New("test")})
	if hook.errorCount != 1 {
		t.Errorf("expected errorCount 1, got %d", hook.errorCount)
	}
}

func TestTimingChecker_NoTimingChecker(t *testing.T) {
	manager := NewManager()
	// mockRunHook 没有实现 TimingChecker，应该默认关心所有时机
	hook := &mockRunHook{name: "no-timing-hook", enabled: true}

	manager.RegisterRunHook(hook)

	ctx := context.Background()

	// 所有事件都应该被调用
	manager.TriggerRunStart(ctx, &RunStartEvent{RunID: "run-1"})
	manager.TriggerRunEnd(ctx, &RunEndEvent{RunID: "run-1"})
	manager.TriggerError(ctx, &ErrorEvent{RunID: "run-1", Error: errors.New("test")})

	if hook.startCount != 1 {
		t.Errorf("expected startCount 1, got %d", hook.startCount)
	}
	if hook.endCount != 1 {
		t.Errorf("expected endCount 1, got %d", hook.endCount)
	}
	if hook.errorCount != 1 {
		t.Errorf("expected errorCount 1, got %d", hook.errorCount)
	}
}

func TestTimingChecker_TimingNone(t *testing.T) {
	manager := NewManager()
	// TimingNone 意味着不关心任何事件
	hook := &timingAwareHook{
		name:    "none-timing-hook",
		enabled: true,
		timings: TimingNone,
	}

	manager.RegisterRunHook(hook)

	ctx := context.Background()

	// 所有事件都不应该被调用
	manager.TriggerRunStart(ctx, &RunStartEvent{RunID: "run-1"})
	manager.TriggerRunEnd(ctx, &RunEndEvent{RunID: "run-1"})
	manager.TriggerError(ctx, &ErrorEvent{RunID: "run-1", Error: errors.New("test")})

	if hook.startCount != 0 || hook.endCount != 0 || hook.errorCount != 0 {
		t.Errorf("expected all counts 0 with TimingNone, got start=%d end=%d error=%d",
			hook.startCount, hook.endCount, hook.errorCount)
	}
}

func TestTiming_Has(t *testing.T) {
	tests := []struct {
		timing   Timing
		check    Timing
		expected bool
	}{
		{TimingRunStart, TimingRunStart, true},
		{TimingRunStart, TimingRunEnd, false},
		{TimingRunStart | TimingRunEnd, TimingRunStart, true},
		{TimingRunStart | TimingRunEnd, TimingRunEnd, true},
		{TimingRunStart | TimingRunEnd, TimingRunError, false},
		{TimingRunAll, TimingRunStart, true},
		{TimingRunAll, TimingRunEnd, true},
		{TimingRunAll, TimingRunError, true},
		{TimingRunAll, TimingToolStart, false},
		{TimingAll, TimingLLMStream, true},
		{TimingNone, TimingRunStart, false},
	}

	for _, tt := range tests {
		result := tt.timing.Has(tt.check)
		if result != tt.expected {
			t.Errorf("Timing(%s).Has(%s) = %v, want %v",
				tt.timing.String(), tt.check.String(), result, tt.expected)
		}
	}
}

func TestTiming_String(t *testing.T) {
	tests := []struct {
		timing   Timing
		contains string
	}{
		{TimingNone, "none"},
		{TimingRunStart, "run_start"},
		{TimingRunEnd, "run_end"},
		{TimingRunStart | TimingRunEnd, "run_start"},
		{TimingLLMStream, "llm_stream"},
	}

	for _, tt := range tests {
		s := tt.timing.String()
		if tt.timing == TimingNone {
			if s != tt.contains {
				t.Errorf("Timing.String() = %s, want %s", s, tt.contains)
			}
		} else if len(s) == 0 {
			t.Errorf("Timing.String() returned empty string for %d", tt.timing)
		}
	}
}
