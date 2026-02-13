// Package main 演示 Hexagon 钩子系统
//
// 钩子系统允许在 Agent 执行的各个阶段插入自定义逻辑：
//   - RunHook: Agent 运行生命周期钩子 (开始/结束/错误)
//   - ToolHook: 工具调用钩子 (调用前/调用后)
//   - 钩子管理器: 统一注册和触发钩子
//
// 运行方式:
//
//	go run ./examples/hooks/
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/everyday-items/hexagon/hooks"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: Run 生命周期钩子 ===")
	runLifecycleHooks(ctx)

	fmt.Println("\n=== 示例 2: Tool 调用钩子 ===")
	runToolHooks(ctx)
}

// ============== 自定义 RunHook ==============

// LoggingRunHook 日志记录钩子
type LoggingRunHook struct {
	start time.Time
}

func (h *LoggingRunHook) Name() string    { return "logging" }
func (h *LoggingRunHook) Enabled() bool   { return true }
func (h *LoggingRunHook) Timings() hooks.Timing { return hooks.TimingRunAll }

func (h *LoggingRunHook) OnStart(ctx context.Context, event *hooks.RunStartEvent) error {
	h.start = time.Now()
	fmt.Printf("  [日志] Agent %s 开始执行, 输入: %v\n", event.AgentID, event.Input)
	return nil
}

func (h *LoggingRunHook) OnEnd(ctx context.Context, event *hooks.RunEndEvent) error {
	fmt.Printf("  [日志] Agent %s 执行完成, 耗时: %v\n", event.AgentID, time.Since(h.start))
	return nil
}

func (h *LoggingRunHook) OnError(ctx context.Context, event *hooks.ErrorEvent) error {
	fmt.Printf("  [日志] Agent %s 出错: %v (阶段: %s)\n", event.AgentID, event.Error, event.Phase)
	return nil
}

// ============== 自定义 ToolHook ==============

// AuditToolHook 工具审计钩子
type AuditToolHook struct{}

func (h *AuditToolHook) Name() string    { return "audit" }
func (h *AuditToolHook) Enabled() bool   { return true }
func (h *AuditToolHook) Timings() hooks.Timing { return hooks.TimingToolAll }

func (h *AuditToolHook) OnToolStart(ctx context.Context, event *hooks.ToolStartEvent) error {
	fmt.Printf("  [审计] 工具 %s 调用开始, 参数: %v\n", event.ToolName, event.Input)
	return nil
}

func (h *AuditToolHook) OnToolEnd(ctx context.Context, event *hooks.ToolEndEvent) error {
	fmt.Printf("  [审计] 工具 %s 调用完成, 耗时: %dms\n", event.ToolName, event.Duration)
	return nil
}

// ============== 示例函数 ==============

func runLifecycleHooks(ctx context.Context) {
	manager := hooks.NewManager()

	// 注册钩子
	manager.RegisterRunHook(&LoggingRunHook{})

	// 模拟 Agent 执行生命周期
	runID := "run-001"
	agentID := "agent-demo"

	// 触发开始事件
	manager.TriggerRunStart(ctx, &hooks.RunStartEvent{
		RunID:   runID,
		AgentID: agentID,
		Input:   "你好，请帮我分析数据",
	})

	// 模拟处理
	time.Sleep(50 * time.Millisecond)

	// 触发完成事件
	manager.TriggerRunEnd(ctx, &hooks.RunEndEvent{
		RunID:    runID,
		AgentID:  agentID,
		Output:   "分析完成",
		Duration: 50,
	})

	// 触发错误事件
	manager.TriggerError(ctx, &hooks.ErrorEvent{
		RunID:   runID,
		AgentID: agentID,
		Error:   fmt.Errorf("模拟错误"),
		Phase:   "tool_execution",
	})
}

func runToolHooks(ctx context.Context) {
	manager := hooks.NewManager()

	// 注册工具钩子
	manager.RegisterToolHook(&AuditToolHook{})

	runID := "run-002"

	// 模拟工具调用
	manager.TriggerToolStart(ctx, &hooks.ToolStartEvent{
		RunID:    runID,
		ToolName: "web_search",
		ToolID:   "tool-001",
		Input:    map[string]any{"query": "Go 语言最佳实践"},
	})

	time.Sleep(30 * time.Millisecond)

	manager.TriggerToolEnd(ctx, &hooks.ToolEndEvent{
		RunID:    runID,
		ToolName: "web_search",
		ToolID:   "tool-001",
		Output:   "搜索结果...",
		Duration: 30,
	})
}
