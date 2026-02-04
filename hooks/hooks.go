// Package hooks 提供 Hexagon AI Agent 框架的钩子/回调系统
//
// 钩子系统允许在 Agent 执行的各个阶段插入自定义逻辑，借鉴 LangChain 的回调系统设计。
//
// 支持的钩子点：
//   - OnStart: Agent 开始执行前
//   - OnEnd: Agent 执行完成后
//   - OnError: 发生错误时
//   - OnToolStart: 工具调用开始前
//   - OnToolEnd: 工具调用完成后
//   - OnLLMStart: LLM 调用开始前
//   - OnLLMEnd: LLM 调用完成后
//   - OnRetrieverStart: 检索开始前
//   - OnRetrieverEnd: 检索完成后
//
// 主要类型：
//   - RunHook: Agent 运行钩子
//   - ToolHook: 工具调用钩子
//   - LLMHook: LLM 调用钩子
//   - RetrieverHook: 检索钩子
//   - Manager: 钩子管理器，统一管理和触发钩子
//
// 使用示例：
//
//	manager := NewManager()
//	manager.RegisterRunHook(myRunHook)
//	manager.TriggerRunStart(ctx, &RunStartEvent{...})
package hooks

import (
	"context"
	"sync"
)

// ============== Timing 时机声明（借鉴 Eino TimingChecker 设计） ==============
// 允许 Hook 声明关心的执行时机，Manager 只调用关心该时机的 Hook
// 避免无效调用，提升性能

// Timing 时机类型（位掩码，可组合多个时机）
type Timing uint32

const (
	// TimingNone 无时机（不关心任何事件）
	TimingNone Timing = 0

	// Run 相关时机
	TimingRunStart Timing = 1 << iota // Agent 开始执行
	TimingRunEnd                       // Agent 执行完成
	TimingRunError                     // 发生错误

	// Tool 相关时机
	TimingToolStart // 工具调用开始
	TimingToolEnd   // 工具调用完成

	// LLM 相关时机
	TimingLLMStart  // LLM 调用开始
	TimingLLMEnd    // LLM 调用完成
	TimingLLMStream // LLM 流式输出

	// Retriever 相关时机
	TimingRetrieverStart // 检索开始
	TimingRetrieverEnd   // 检索完成

	// 便捷组合
	TimingRunAll       = TimingRunStart | TimingRunEnd | TimingRunError
	TimingToolAll      = TimingToolStart | TimingToolEnd
	TimingLLMAll       = TimingLLMStart | TimingLLMEnd | TimingLLMStream
	TimingRetrieverAll = TimingRetrieverStart | TimingRetrieverEnd
	TimingAll          = TimingRunAll | TimingToolAll | TimingLLMAll | TimingRetrieverAll
)

// Has 检查是否包含指定时机
func (t Timing) Has(timing Timing) bool {
	return t&timing != 0
}

// String 返回时机的字符串表示
func (t Timing) String() string {
	if t == TimingNone {
		return "none"
	}
	var s string
	timings := []struct {
		t Timing
		n string
	}{
		{TimingRunStart, "run_start"},
		{TimingRunEnd, "run_end"},
		{TimingRunError, "run_error"},
		{TimingToolStart, "tool_start"},
		{TimingToolEnd, "tool_end"},
		{TimingLLMStart, "llm_start"},
		{TimingLLMEnd, "llm_end"},
		{TimingLLMStream, "llm_stream"},
		{TimingRetrieverStart, "retriever_start"},
		{TimingRetrieverEnd, "retriever_end"},
	}
	for _, tt := range timings {
		if t.Has(tt.t) {
			if s != "" {
				s += "|"
			}
			s += tt.n
		}
	}
	return s
}

// TimingChecker 时机检查器接口
// Hook 可选实现此接口来声明关心的时机
// 如果未实现，默认关心所有时机
type TimingChecker interface {
	// Timings 返回关心的时机（位掩码）
	// 返回 TimingAll 表示关心所有时机
	// 返回 TimingNone 表示不关心任何时机（禁用）
	Timings() Timing
}

// Hook 钩子接口
type Hook interface {
	// Name 返回钩子名称
	Name() string

	// Enabled 是否启用
	Enabled() bool
}

// RunHook Agent 运行钩子
type RunHook interface {
	Hook
	// OnStart Agent 开始执行
	OnStart(ctx context.Context, event *RunStartEvent) error
	// OnEnd Agent 执行完成
	OnEnd(ctx context.Context, event *RunEndEvent) error
	// OnError 发生错误
	OnError(ctx context.Context, event *ErrorEvent) error
}

// ToolHook 工具调用钩子
type ToolHook interface {
	Hook
	// OnToolStart 工具调用开始
	OnToolStart(ctx context.Context, event *ToolStartEvent) error
	// OnToolEnd 工具调用完成
	OnToolEnd(ctx context.Context, event *ToolEndEvent) error
}

// LLMHook LLM 调用钩子
type LLMHook interface {
	Hook
	// OnLLMStart LLM 调用开始
	OnLLMStart(ctx context.Context, event *LLMStartEvent) error
	// OnLLMEnd LLM 调用完成
	OnLLMEnd(ctx context.Context, event *LLMEndEvent) error
	// OnLLMStream LLM 流式输出
	OnLLMStream(ctx context.Context, event *LLMStreamEvent) error
}

// RetrieverHook 检索钩子
type RetrieverHook interface {
	Hook
	// OnRetrieverStart 检索开始
	OnRetrieverStart(ctx context.Context, event *RetrieverStartEvent) error
	// OnRetrieverEnd 检索完成
	OnRetrieverEnd(ctx context.Context, event *RetrieverEndEvent) error
}

// ============== Events ==============

// RunStartEvent Agent 开始执行事件
type RunStartEvent struct {
	RunID    string         `json:"run_id"`
	AgentID  string         `json:"agent_id"`
	Input    any            `json:"input"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RunEndEvent Agent 执行完成事件
type RunEndEvent struct {
	RunID    string         `json:"run_id"`
	AgentID  string         `json:"agent_id"`
	Output   any            `json:"output"`
	Duration int64          `json:"duration_ms"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ErrorEvent 错误事件
type ErrorEvent struct {
	RunID    string         `json:"run_id"`
	AgentID  string         `json:"agent_id"`
	Error    error          `json:"error"`
	Phase    string         `json:"phase"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolStartEvent 工具调用开始事件
type ToolStartEvent struct {
	RunID      string         `json:"run_id"`
	ToolName   string         `json:"tool_name"`
	ToolID     string         `json:"tool_id"`
	Input      map[string]any `json:"input"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToolEndEvent 工具调用完成事件
type ToolEndEvent struct {
	RunID    string         `json:"run_id"`
	ToolName string         `json:"tool_name"`
	ToolID   string         `json:"tool_id"`
	Output   any            `json:"output"`
	Duration int64          `json:"duration_ms"`
	Error    error          `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// LLMStartEvent LLM 调用开始事件
type LLMStartEvent struct {
	RunID       string         `json:"run_id"`
	RequestID   string         `json:"request_id"`
	Model       string         `json:"model"`
	Provider    string         `json:"provider"`
	Messages    []any          `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// LLMEndEvent LLM 调用完成事件
type LLMEndEvent struct {
	RunID            string         `json:"run_id"`
	RequestID        string         `json:"request_id"`
	Model            string         `json:"model"`
	Response         any            `json:"response"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	Duration         int64          `json:"duration_ms"`
	Error            error          `json:"error,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// LLMStreamEvent LLM 流式输出事件
type LLMStreamEvent struct {
	RunID      string `json:"run_id"`
	RequestID  string `json:"request_id"`
	Model      string `json:"model"`
	Content    string `json:"content"`
	ChunkIndex int    `json:"chunk_index"`
}

// RetrieverStartEvent 检索开始事件
type RetrieverStartEvent struct {
	RunID    string         `json:"run_id"`
	QueryID  string         `json:"query_id"`
	Query    string         `json:"query"`
	TopK     int            `json:"top_k"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RetrieverEndEvent 检索完成事件
type RetrieverEndEvent struct {
	RunID     string         `json:"run_id"`
	QueryID   string         `json:"query_id"`
	Query     string         `json:"query"`
	Documents []any          `json:"documents"`
	DocCount  int            `json:"doc_count"`
	Duration  int64          `json:"duration_ms"`
	Error     error          `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ============== HookManager ==============

// Manager 钩子管理器
type Manager struct {
	runHooks       []RunHook
	toolHooks      []ToolHook
	llmHooks       []LLMHook
	retrieverHooks []RetrieverHook
	mu             sync.RWMutex
}

// NewManager 创建钩子管理器
func NewManager() *Manager {
	return &Manager{
		runHooks:       make([]RunHook, 0),
		toolHooks:      make([]ToolHook, 0),
		llmHooks:       make([]LLMHook, 0),
		retrieverHooks: make([]RetrieverHook, 0),
	}
}

// RegisterRunHook 注册运行钩子
func (m *Manager) RegisterRunHook(hook RunHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runHooks = append(m.runHooks, hook)
}

// RegisterToolHook 注册工具钩子
func (m *Manager) RegisterToolHook(hook ToolHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolHooks = append(m.toolHooks, hook)
}

// RegisterLLMHook 注册 LLM 钩子
func (m *Manager) RegisterLLMHook(hook LLMHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmHooks = append(m.llmHooks, hook)
}

// RegisterRetrieverHook 注册检索钩子
func (m *Manager) RegisterRetrieverHook(hook RetrieverHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieverHooks = append(m.retrieverHooks, hook)
}

// checkTiming 检查 Hook 是否关心指定时机
// 如果 Hook 实现了 TimingChecker 接口，检查其声明的时机
// 否则默认关心所有时机
func checkTiming(hook Hook, timing Timing) bool {
	if tc, ok := hook.(TimingChecker); ok {
		return tc.Timings().Has(timing)
	}
	return true // 未实现 TimingChecker 则默认关心所有时机
}

// TriggerRunStart 触发运行开始事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingRunStart 时机的 Hook。
func (m *Manager) TriggerRunStart(ctx context.Context, event *RunStartEvent) error {
	m.mu.RLock()
	hooks := make([]RunHook, len(m.runHooks))
	copy(hooks, m.runHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingRunStart) {
			if err := hook.OnStart(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerRunEnd 触发运行结束事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingRunEnd 时机的 Hook。
func (m *Manager) TriggerRunEnd(ctx context.Context, event *RunEndEvent) error {
	m.mu.RLock()
	hooks := make([]RunHook, len(m.runHooks))
	copy(hooks, m.runHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingRunEnd) {
			if err := hook.OnEnd(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerError 触发错误事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingRunError 时机的 Hook。
func (m *Manager) TriggerError(ctx context.Context, event *ErrorEvent) error {
	m.mu.RLock()
	hooks := make([]RunHook, len(m.runHooks))
	copy(hooks, m.runHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingRunError) {
			if err := hook.OnError(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerToolStart 触发工具开始事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingToolStart 时机的 Hook。
func (m *Manager) TriggerToolStart(ctx context.Context, event *ToolStartEvent) error {
	m.mu.RLock()
	hooks := make([]ToolHook, len(m.toolHooks))
	copy(hooks, m.toolHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingToolStart) {
			if err := hook.OnToolStart(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerToolEnd 触发工具结束事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingToolEnd 时机的 Hook。
func (m *Manager) TriggerToolEnd(ctx context.Context, event *ToolEndEvent) error {
	m.mu.RLock()
	hooks := make([]ToolHook, len(m.toolHooks))
	copy(hooks, m.toolHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingToolEnd) {
			if err := hook.OnToolEnd(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerLLMStart 触发 LLM 开始事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingLLMStart 时机的 Hook。
func (m *Manager) TriggerLLMStart(ctx context.Context, event *LLMStartEvent) error {
	m.mu.RLock()
	hooks := make([]LLMHook, len(m.llmHooks))
	copy(hooks, m.llmHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingLLMStart) {
			if err := hook.OnLLMStart(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerLLMEnd 触发 LLM 结束事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingLLMEnd 时机的 Hook。
func (m *Manager) TriggerLLMEnd(ctx context.Context, event *LLMEndEvent) error {
	m.mu.RLock()
	hooks := make([]LLMHook, len(m.llmHooks))
	copy(hooks, m.llmHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingLLMEnd) {
			if err := hook.OnLLMEnd(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerLLMStream 触发 LLM 流式事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingLLMStream 时机的 Hook。
func (m *Manager) TriggerLLMStream(ctx context.Context, event *LLMStreamEvent) error {
	m.mu.RLock()
	hooks := make([]LLMHook, len(m.llmHooks))
	copy(hooks, m.llmHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingLLMStream) {
			if err := hook.OnLLMStream(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerRetrieverStart 触发检索开始事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingRetrieverStart 时机的 Hook。
func (m *Manager) TriggerRetrieverStart(ctx context.Context, event *RetrieverStartEvent) error {
	m.mu.RLock()
	hooks := make([]RetrieverHook, len(m.retrieverHooks))
	copy(hooks, m.retrieverHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingRetrieverStart) {
			if err := hook.OnRetrieverStart(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerRetrieverEnd 触发检索结束事件
//
// 线程安全：在迭代前创建钩子列表的副本，避免并发修改问题。
// TimingChecker：只调用关心 TimingRetrieverEnd 时机的 Hook。
func (m *Manager) TriggerRetrieverEnd(ctx context.Context, event *RetrieverEndEvent) error {
	m.mu.RLock()
	hooks := make([]RetrieverHook, len(m.retrieverHooks))
	copy(hooks, m.retrieverHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook.Enabled() && checkTiming(hook, TimingRetrieverEnd) {
			if err := hook.OnRetrieverEnd(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

// ============== Context Helpers ==============

type hookManagerKey struct{}

// ContextWithManager 将钩子管理器添加到 context
func ContextWithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, hookManagerKey{}, m)
}

// ManagerFromContext 从 context 获取钩子管理器
func ManagerFromContext(ctx context.Context) *Manager {
	if m, ok := ctx.Value(hookManagerKey{}).(*Manager); ok {
		return m
	}
	return nil
}
