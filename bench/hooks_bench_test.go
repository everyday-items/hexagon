package bench

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/hooks"
)

// nopRunHook 空操作钩子，用于测量钩子触发开销
type nopRunHook struct{}

func (h *nopRunHook) Name() string  { return "nop" }
func (h *nopRunHook) Enabled() bool { return true }
func (h *nopRunHook) OnStart(ctx context.Context, event *hooks.RunStartEvent) error {
	return nil
}
func (h *nopRunHook) OnEnd(ctx context.Context, event *hooks.RunEndEvent) error {
	return nil
}
func (h *nopRunHook) OnError(ctx context.Context, event *hooks.ErrorEvent) error {
	return nil
}

// nopToolHook 空工具钩子
type nopToolHook struct{}

func (h *nopToolHook) Name() string  { return "nop-tool" }
func (h *nopToolHook) Enabled() bool { return true }
func (h *nopToolHook) OnToolStart(ctx context.Context, event *hooks.ToolStartEvent) error {
	return nil
}
func (h *nopToolHook) OnToolEnd(ctx context.Context, event *hooks.ToolEndEvent) error {
	return nil
}

// BenchmarkHooksManagerCreation 测试钩子管理器创建性能
func BenchmarkHooksManagerCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hooks.NewManager()
	}
}

// BenchmarkHooksTriggerRunStart 测试触发 RunStart 事件性能（1 个钩子）
func BenchmarkHooksTriggerRunStart(b *testing.B) {
	m := hooks.NewManager()
	m.RegisterRunHook(&nopRunHook{})
	ctx := context.Background()
	event := &hooks.RunStartEvent{
		RunID:   "run-bench",
		AgentID: "agent-bench",
		Input:   "test input",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.TriggerRunStart(ctx, event)
	}
}

// BenchmarkHooksTriggerRunStartMultiple 测试触发 RunStart 事件性能（10 个钩子）
func BenchmarkHooksTriggerRunStartMultiple(b *testing.B) {
	m := hooks.NewManager()
	for i := 0; i < 10; i++ {
		m.RegisterRunHook(&nopRunHook{})
	}
	ctx := context.Background()
	event := &hooks.RunStartEvent{
		RunID:   "run-bench",
		AgentID: "agent-bench",
		Input:   "test input",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.TriggerRunStart(ctx, event)
	}
}

// BenchmarkHooksTriggerToolStart 测试触发 ToolStart 事件性能
func BenchmarkHooksTriggerToolStart(b *testing.B) {
	m := hooks.NewManager()
	m.RegisterToolHook(&nopToolHook{})
	ctx := context.Background()
	event := &hooks.ToolStartEvent{
		RunID:    "run-bench",
		ToolName: "web_search",
		ToolID:   "tool-001",
		Input:    map[string]any{"query": "test"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.TriggerToolStart(ctx, event)
	}
}

// BenchmarkHooksTriggerRunStartConcurrent 测试并发触发事件性能
func BenchmarkHooksTriggerRunStartConcurrent(b *testing.B) {
	m := hooks.NewManager()
	m.RegisterRunHook(&nopRunHook{})
	ctx := context.Background()
	event := &hooks.RunStartEvent{
		RunID:   "run-bench",
		AgentID: "agent-bench",
		Input:   "test input",
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.TriggerRunStart(ctx, event)
		}
	})
}

// BenchmarkHooksNoHooks 测试无钩子时的触发开销（基线）
func BenchmarkHooksNoHooks(b *testing.B) {
	m := hooks.NewManager()
	ctx := context.Background()
	event := &hooks.RunStartEvent{
		RunID:   "run-bench",
		AgentID: "agent-bench",
		Input:   "test input",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.TriggerRunStart(ctx, event)
	}
}
