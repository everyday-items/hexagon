package devui

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/hexagon/hooks"
	"github.com/everyday-items/hexagon/observe/tracer"
	"github.com/everyday-items/toolkit/util/idgen"
)

// Collector 事件收集器
//
// 收集器实现了 hooks.RunHook、hooks.ToolHook、hooks.LLMHook、hooks.RetrieverHook 接口，
// 用于收集 Agent 执行过程中的各种事件，并广播给 SSE 订阅者。
//
// 特性：
//   - 实现所有 Hook 接口，自动收集事件
//   - 环形缓冲区存储历史事件
//   - 支持多个 SSE 订阅者
//   - 内置 MemoryTracer 用于 Span 追踪
type Collector struct {
	// events 事件缓冲区
	events *RingBuffer

	// subscribers SSE 订阅者列表
	subscribers map[uint64]chan *Event
	subID       atomic.Uint64 // 订阅者 ID 生成器
	subMu       sync.RWMutex

	// tracer 内置追踪器
	tracer *tracer.MemoryTracer

	// enabled 是否启用
	enabled bool

	// stats 统计信息
	stats struct {
		totalEvents   atomic.Int64
		agentRuns     atomic.Int64
		llmCalls      atomic.Int64
		toolCalls     atomic.Int64
		retrieverRuns atomic.Int64
		errors        atomic.Int64
	}
}

// NewCollector 创建事件收集器
//
// 参数：
//   - maxEvents: 最大事件缓存数量
func NewCollector(maxEvents int) *Collector {
	return &Collector{
		events:      NewRingBuffer(maxEvents),
		subscribers: make(map[uint64]chan *Event),
		tracer:      tracer.NewMemoryTracer(),
		enabled:     true,
	}
}

// Name 返回钩子名称
func (c *Collector) Name() string {
	return "devui-collector"
}

// Enabled 返回钩子是否启用
func (c *Collector) Enabled() bool {
	return c.enabled
}

// SetEnabled 设置钩子是否启用
func (c *Collector) SetEnabled(enabled bool) {
	c.enabled = enabled
}

// Tracer 返回内置的 MemoryTracer
func (c *Collector) Tracer() *tracer.MemoryTracer {
	return c.tracer
}

// Events 返回事件缓冲区
func (c *Collector) Events() *RingBuffer {
	return c.events
}

// Subscribe 订阅事件流
// 返回事件通道和取消订阅函数
//
// 注意：取消订阅函数只能调用一次，重复调用会导致 panic
func (c *Collector) Subscribe() (<-chan *Event, func()) {
	ch := make(chan *Event, 100) // 缓冲区防止阻塞
	id := c.subID.Add(1)

	c.subMu.Lock()
	c.subscribers[id] = ch
	c.subMu.Unlock()

	unsubscribe := func() {
		c.subMu.Lock()
		// 先从 map 中删除，确保 broadcast 不会再向此 channel 发送数据
		// 然后在锁内关闭 channel，避免竞态条件导致向已关闭 channel 发送数据
		if _, exists := c.subscribers[id]; exists {
			delete(c.subscribers, id)
			close(ch)
		}
		c.subMu.Unlock()
	}

	return ch, unsubscribe
}

// SubscriberCount 返回当前订阅者数量
func (c *Collector) SubscriberCount() int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscribers)
}

// broadcast 广播事件给所有订阅者
func (c *Collector) broadcast(e *Event) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	for _, ch := range c.subscribers {
		// 非阻塞发送，如果通道满了就跳过
		select {
		case ch <- e.Clone():
		default:
			// 通道满了，跳过此订阅者
		}
	}
}

// emit 发送事件
func (c *Collector) emit(e *Event) {
	// 更新统计
	c.stats.totalEvents.Add(1)

	// 存储到缓冲区
	c.events.Push(e)

	// 广播给订阅者
	c.broadcast(e)
}

// createEvent 创建基础事件
func (c *Collector) createEvent(eventType EventType, traceID, spanID, agentID, agentName string) *Event {
	e := AcquireEvent()
	e.ID = idgen.NanoID()
	e.Type = eventType
	e.Timestamp = time.Now()
	e.TraceID = traceID
	e.SpanID = spanID
	e.AgentID = agentID
	e.AgentName = agentName
	return e
}

// Stats 返回统计信息
func (c *Collector) Stats() CollectorStats {
	return CollectorStats{
		TotalEvents:   c.stats.totalEvents.Load(),
		AgentRuns:     c.stats.agentRuns.Load(),
		LLMCalls:      c.stats.llmCalls.Load(),
		ToolCalls:     c.stats.toolCalls.Load(),
		RetrieverRuns: c.stats.retrieverRuns.Load(),
		Errors:        c.stats.errors.Load(),
		Subscribers:   c.SubscriberCount(),
		BufferSize:    c.events.Size(),
	}
}

// CollectorStats 收集器统计信息
type CollectorStats struct {
	TotalEvents   int64 `json:"total_events"`
	AgentRuns     int64 `json:"agent_runs"`
	LLMCalls      int64 `json:"llm_calls"`
	ToolCalls     int64 `json:"tool_calls"`
	RetrieverRuns int64 `json:"retriever_runs"`
	Errors        int64 `json:"errors"`
	Subscribers   int   `json:"subscribers"`
	BufferSize    int   `json:"buffer_size"`
}

// ============================================================================
// 实现 hooks.RunHook 接口
// ============================================================================

// OnStart Agent 开始执行
func (c *Collector) OnStart(ctx context.Context, evt *hooks.RunStartEvent) error {
	c.stats.agentRuns.Add(1)

	e := c.createEvent(EventAgentStart, "", "", evt.AgentID, "")
	e.Data["run_id"] = evt.RunID
	e.Data["input"] = evt.Input
	if evt.Metadata != nil {
		e.Data["metadata"] = evt.Metadata
	}

	c.emit(e)
	ReleaseEvent(e)

	return nil
}

// OnEnd Agent 执行结束
func (c *Collector) OnEnd(ctx context.Context, evt *hooks.RunEndEvent) error {
	e := c.createEvent(EventAgentEnd, "", "", evt.AgentID, "")
	e.Data["run_id"] = evt.RunID
	e.Data["output"] = evt.Output
	e.Data["duration_ms"] = evt.Duration
	if evt.Metadata != nil {
		e.Data["metadata"] = evt.Metadata
	}

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// OnError Agent 执行错误
func (c *Collector) OnError(ctx context.Context, evt *hooks.ErrorEvent) error {
	c.stats.errors.Add(1)

	e := c.createEvent(EventError, "", "", evt.AgentID, "")
	e.Data["run_id"] = evt.RunID
	e.Data["source"] = "agent"
	e.Data["message"] = evt.Error.Error()

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// ============================================================================
// 实现 hooks.ToolHook 接口
// ============================================================================

// OnToolStart 工具开始执行
func (c *Collector) OnToolStart(ctx context.Context, evt *hooks.ToolStartEvent) error {
	c.stats.toolCalls.Add(1)

	e := c.createEvent(EventToolCall, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["tool_id"] = evt.ToolID
	e.Data["tool_name"] = evt.ToolName
	e.Data["input"] = evt.Input

	c.emit(e)
	ReleaseEvent(e)

	return nil
}

// OnToolEnd 工具执行结束
func (c *Collector) OnToolEnd(ctx context.Context, evt *hooks.ToolEndEvent) error {
	eventType := EventToolResult
	e := c.createEvent(eventType, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["tool_id"] = evt.ToolID
	e.Data["tool_name"] = evt.ToolName
	e.Data["output"] = evt.Output
	e.Data["duration_ms"] = evt.Duration

	if evt.Error != nil {
		c.stats.errors.Add(1)
		e.Data["error"] = evt.Error.Error()
	}

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// ============================================================================
// 实现 hooks.LLMHook 接口
// ============================================================================

// OnLLMStart LLM 调用开始
func (c *Collector) OnLLMStart(ctx context.Context, evt *hooks.LLMStartEvent) error {
	c.stats.llmCalls.Add(1)

	e := c.createEvent(EventLLMRequest, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["provider"] = evt.Provider
	e.Data["model"] = evt.Model
	e.Data["messages"] = evt.Messages
	e.Data["temperature"] = evt.Temperature

	c.emit(e)
	ReleaseEvent(e)

	return nil
}

// OnLLMStream LLM 流式输出
func (c *Collector) OnLLMStream(ctx context.Context, evt *hooks.LLMStreamEvent) error {
	e := c.createEvent(EventLLMStream, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["model"] = evt.Model
	e.Data["content"] = evt.Content
	e.Data["index"] = evt.ChunkIndex

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// OnLLMEnd LLM 调用结束
func (c *Collector) OnLLMEnd(ctx context.Context, evt *hooks.LLMEndEvent) error {
	e := c.createEvent(EventLLMResponse, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["model"] = evt.Model
	e.Data["response"] = evt.Response
	e.Data["prompt_tokens"] = evt.PromptTokens
	e.Data["completion_tokens"] = evt.CompletionTokens
	e.Data["total_tokens"] = evt.PromptTokens + evt.CompletionTokens
	e.Data["duration_ms"] = evt.Duration

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// ============================================================================
// 实现 hooks.RetrieverHook 接口
// ============================================================================

// OnRetrieverStart 检索开始
func (c *Collector) OnRetrieverStart(ctx context.Context, evt *hooks.RetrieverStartEvent) error {
	c.stats.retrieverRuns.Add(1)

	e := c.createEvent(EventRetrieverStart, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["query"] = evt.Query
	e.Data["top_k"] = evt.TopK

	c.emit(e)
	ReleaseEvent(e)

	return nil
}

// OnRetrieverEnd 检索结束
func (c *Collector) OnRetrieverEnd(ctx context.Context, evt *hooks.RetrieverEndEvent) error {
	e := c.createEvent(EventRetrieverEnd, "", "", "", "")
	e.Data["run_id"] = evt.RunID
	e.Data["query"] = evt.Query
	e.Data["doc_count"] = len(evt.Documents)
	e.Data["documents"] = evt.Documents
	e.Data["duration_ms"] = evt.Duration

	c.emit(e)
	ReleaseEvent(e)
	return nil
}

// ============================================================================
// 图编排事件（手动触发）
// ============================================================================

// EmitGraphStart 发送图执行开始事件
func (c *Collector) EmitGraphStart(runID, graphID, graphName string, state map[string]any) {
	e := c.createEvent(EventGraphStart, "", "", "", "")
	e.Data["run_id"] = runID
	e.Data["graph_id"] = graphID
	e.Data["graph_name"] = graphName
	e.Data["state"] = state

	c.emit(e)
	ReleaseEvent(e)
}

// EmitGraphNode 发送图节点执行事件
func (c *Collector) EmitGraphNode(runID, graphID, nodeID, nodeName string, state map[string]any, durationMs int64) {
	e := c.createEvent(EventGraphNode, "", "", "", "")
	e.Data["run_id"] = runID
	e.Data["graph_id"] = graphID
	e.Data["node_id"] = nodeID
	e.Data["node_name"] = nodeName
	e.Data["state"] = state
	e.Data["duration_ms"] = durationMs

	c.emit(e)
	ReleaseEvent(e)
}

// EmitGraphEnd 发送图执行结束事件
func (c *Collector) EmitGraphEnd(runID, graphID string, state map[string]any, durationMs int64) {
	e := c.createEvent(EventGraphEnd, "", "", "", "")
	e.Data["run_id"] = runID
	e.Data["graph_id"] = graphID
	e.Data["state"] = state
	e.Data["duration_ms"] = durationMs

	c.emit(e)
	ReleaseEvent(e)
}

// EmitStateChange 发送状态变更事件
func (c *Collector) EmitStateChange(agentID, key string, oldValue, newValue any) {
	e := c.createEvent(EventStateChange, "", "", agentID, "")
	e.Data["key"] = key
	e.Data["old_value"] = oldValue
	e.Data["new_value"] = newValue

	c.emit(e)
	ReleaseEvent(e)
}

// EmitError 发送错误事件
func (c *Collector) EmitError(runID, source, message, stack string) {
	c.stats.errors.Add(1)

	e := c.createEvent(EventError, "", "", "", "")
	e.Data["run_id"] = runID
	e.Data["source"] = source
	e.Data["message"] = message
	if stack != "" {
		e.Data["stack"] = stack
	}

	c.emit(e)
	ReleaseEvent(e)
}

// ============================================================================
// 接口断言
// ============================================================================

var (
	_ hooks.RunHook       = (*Collector)(nil)
	_ hooks.ToolHook      = (*Collector)(nil)
	_ hooks.LLMHook       = (*Collector)(nil)
	_ hooks.RetrieverHook = (*Collector)(nil)
)
