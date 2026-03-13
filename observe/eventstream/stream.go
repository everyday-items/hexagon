// Package eventstream 提供 Agent 执行事件流
//
// 为 Agent 运行过程提供实时事件推送，支持：
//   - 多订阅者并发接收
//   - 非阻塞发布（缓冲满时丢弃）
//   - 同步发布（等待所有订阅者消费）
//   - 便捷的事件发射方法
//
// 使用示例：
//
//	stream := eventstream.New(eventstream.WithBufferSize(200))
//	ch, unsub := stream.Subscribe()
//	defer unsub()
//
//	stream.Emit(eventstream.EventAgentStart, "agent-1", map[string]any{"model": "gpt-4"})
//	event := <-ch
package eventstream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/toolkit/event"
)

// ============== 事件类型 ==============

// EventType 事件类型
type EventType string

const (
	// EventAgentStart Agent 开始执行
	EventAgentStart EventType = "agent.start"

	// EventAgentEnd Agent 执行完成
	EventAgentEnd EventType = "agent.end"

	// EventAgentError Agent 执行出错
	EventAgentError EventType = "agent.error"

	// EventToolCall 工具调用开始
	EventToolCall EventType = "tool.call"

	// EventToolResult 工具调用结果
	EventToolResult EventType = "tool.result"

	// EventLLMRequest LLM 请求发出
	EventLLMRequest EventType = "llm.request"

	// EventLLMResponse LLM 响应接收
	EventLLMResponse EventType = "llm.response"

	// EventStateChange 状态变更
	EventStateChange EventType = "state.change"

	// EventCheckpoint 检查点保存
	EventCheckpoint EventType = "checkpoint"

	// EventMessage 消息事件
	EventMessage EventType = "message"
)

// ============== 事件 ==============

// Event Agent 执行事件
type Event struct {
	// Type 事件类型
	Type EventType `json:"type"`

	// AgentID Agent 标识
	AgentID string `json:"agent_id"`

	// Timestamp 事件时间
	Timestamp time.Time `json:"timestamp"`

	// Data 事件数据
	Data map[string]any `json:"data,omitempty"`

	// TraceID 追踪 ID
	TraceID string `json:"trace_id,omitempty"`

	// SpanID 跨度 ID
	SpanID string `json:"span_id,omitempty"`
}

// ============== 事件流 ==============

// Stream Agent 事件流
//
// 支持多个订阅者同时接收事件。
// 线程安全。
type Stream struct {
	subscribers map[uint64]chan Event
	mu          sync.RWMutex
	nextID      atomic.Uint64
	closed      atomic.Bool
	bufferSize  int
}

// Option 配置选项
type Option func(*Stream)

// WithBufferSize 设置订阅者通道缓冲大小
//
// 默认 100。缓冲满时非阻塞发布会丢弃事件。
func WithBufferSize(size int) Option {
	return func(s *Stream) {
		s.bufferSize = size
	}
}

// New 创建事件流
func New(opts ...Option) *Stream {
	s := &Stream{
		subscribers: make(map[uint64]chan Event),
		bufferSize:  100,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Subscribe 订阅事件流
//
// 返回事件接收通道和取消订阅函数。
// 调用方必须在不再需要时调用 unsubscribe。
func (s *Stream) Subscribe() (<-chan Event, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID.Add(1)
	ch := make(chan Event, s.bufferSize)
	s.subscribers[id] = ch

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, exists := s.subscribers[id]; exists {
			delete(s.subscribers, id)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Publish 非阻塞发布事件
//
// 向所有订阅者发送事件。若某个订阅者的缓冲区满，该订阅者会丢失此事件。
func (s *Stream) Publish(event Event) {
	if s.closed.Load() {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// 缓冲区满，丢弃事件
		}
	}
}

// PublishSync 同步发布事件（阻塞直到所有订阅者消费或 ctx 取消）
func (s *Stream) PublishSync(ctx context.Context, event Event) error {
	if s.closed.Load() {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Emit 便捷方法：创建事件并发布
func (s *Stream) Emit(eventType EventType, agentID string, data map[string]any) {
	s.Publish(Event{
		Type:      eventType,
		AgentID:   agentID,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// EmitWithTrace 带追踪信息的便捷发布
func (s *Stream) EmitWithTrace(eventType EventType, agentID, traceID, spanID string, data map[string]any) {
	s.Publish(Event{
		Type:      eventType,
		AgentID:   agentID,
		Timestamp: time.Now(),
		Data:      data,
		TraceID:   traceID,
		SpanID:    spanID,
	})
}

// Close 关闭事件流，关闭所有订阅者通道
func (s *Stream) Close() {
	if s.closed.Swap(true) {
		return // 已经关闭
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, id)
	}
}

// SubscriberCount 返回当前订阅者数量
func (s *Stream) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

// BridgeTo 将事件流桥接到 toolkit 事件总线
//
// 所有通过 Stream 发布的事件会同时发送到指定的 toolkit event.Bus。
// 事件类型映射为 string(EventType)，Payload 为 Event 结构体。
// 返回取消桥接的函数。
func (s *Stream) BridgeTo(bus *event.Bus) func() {
	ch, unsub := s.Subscribe()
	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				bus.Publish(event.Event{
					Type:      string(evt.Type),
					Source:    evt.AgentID,
					Payload:   evt,
					Timestamp: evt.Timestamp,
				})
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		unsub()
	}
}
