package tracer

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// DefaultSpan 默认 Span 实现
type DefaultSpan struct {
	mu sync.RWMutex

	spanID    string
	traceID   string
	parentID  string
	name      string
	kind      SpanKind
	startTime time.Time
	endTime   time.Time

	attributes map[string]any
	events     []SpanEvent
	status     SpanStatus

	input      any
	output     any
	tokenUsage TokenUsage

	recording bool
}

// SpanEvent Span 事件
type SpanEvent struct {
	Name       string         `json:"name"`
	Time       time.Time      `json:"time"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanStatus Span 状态
type SpanStatus struct {
	Code    StatusCode `json:"code"`
	Message string     `json:"message,omitempty"`
}

// NewSpan 创建新的 Span
func NewSpan(name string, traceID string, opts ...SpanOption) *DefaultSpan {
	cfg := &SpanConfig{
		Kind:       SpanKindInternal,
		Attributes: make(map[string]any),
		StartTime:  time.Now(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	span := &DefaultSpan{
		spanID:     util.SpanID(),
		traceID:    traceID,
		name:       name,
		kind:       cfg.Kind,
		startTime:  cfg.StartTime,
		attributes: cfg.Attributes,
		events:     make([]SpanEvent, 0),
		recording:  true,
	}

	if cfg.Parent != nil {
		span.parentID = cfg.Parent.SpanID()
	}

	return span
}

// SpanID 返回 Span ID
func (s *DefaultSpan) SpanID() string {
	return s.spanID
}

// TraceID 返回 Trace ID
func (s *DefaultSpan) TraceID() string {
	return s.traceID
}

// ParentID 返回父 Span ID
func (s *DefaultSpan) ParentID() string {
	return s.parentID
}

// SetName 设置名称
func (s *DefaultSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

// SetInput 设置输入
func (s *DefaultSpan) SetInput(input any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input = input
}

// SetOutput 设置输出
func (s *DefaultSpan) SetOutput(output any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output = output
}

// SetTokenUsage 设置 Token 使用量
func (s *DefaultSpan) SetTokenUsage(usage TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenUsage = usage
	s.attributes[AttrLLMPromptTokens] = usage.PromptTokens
	s.attributes[AttrLLMCompletionTokens] = usage.CompletionTokens
	s.attributes[AttrLLMTotalTokens] = usage.TotalTokens
}

// SetAttribute 设置属性
func (s *DefaultSpan) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = value
}

// SetAttributes 批量设置属性
func (s *DefaultSpan) SetAttributes(attrs map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// AddEvent 添加事件
func (s *DefaultSpan) AddEvent(name string, attrs ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event := SpanEvent{
		Name:       name,
		Time:       time.Now(),
		Attributes: make(map[string]any),
	}

	// 解析 key-value 对
	for i := 0; i < len(attrs)-1; i += 2 {
		if key, ok := attrs[i].(string); ok {
			event.Attributes[key] = attrs[i+1]
		}
	}

	s.events = append(s.events, event)
}

// RecordError 记录错误
func (s *DefaultSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = SpanStatus{
		Code:    StatusCodeError,
		Message: err.Error(),
	}
	s.attributes[AttrErrorType] = "error"
	s.attributes[AttrErrorMessage] = err.Error()

	s.events = append(s.events, SpanEvent{
		Name: "exception",
		Time: time.Now(),
		Attributes: map[string]any{
			"exception.message": err.Error(),
		},
	})
}

// SetStatus 设置状态
func (s *DefaultSpan) SetStatus(code StatusCode, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = SpanStatus{Code: code, Message: message}
}

// End 结束 Span
func (s *DefaultSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endTime = time.Now()
	s.recording = false
}

// EndWithError 结束 Span 并记录错误
func (s *DefaultSpan) EndWithError(err error) {
	s.RecordError(err)
	s.End()
}

// IsRecording 是否正在记录
func (s *DefaultSpan) IsRecording() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recording
}

// Duration 返回 Span 持续时间
func (s *DefaultSpan) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.endTime.IsZero() {
		return time.Since(s.startTime)
	}
	return s.endTime.Sub(s.startTime)
}

// Attributes 返回所有属性
func (s *DefaultSpan) Attributes() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attrs := make(map[string]any, len(s.attributes))
	for k, v := range s.attributes {
		attrs[k] = v
	}
	return attrs
}

// Events 返回所有事件
func (s *DefaultSpan) Events() []SpanEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]SpanEvent, len(s.events))
	copy(events, s.events)
	return events
}

// SpanData 导出 Span 数据
type SpanData struct {
	SpanID     string         `json:"span_id"`
	TraceID    string         `json:"trace_id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	StartTime  time.Time      `json:"start_time"`
	EndTime    time.Time      `json:"end_time,omitempty"`
	Duration   time.Duration  `json:"duration"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Events     []SpanEvent    `json:"events,omitempty"`
	Status     SpanStatus     `json:"status"`
	Input      any            `json:"input,omitempty"`
	Output     any            `json:"output,omitempty"`
	TokenUsage TokenUsage     `json:"token_usage,omitempty"`
}

// Export 导出 Span 数据
func (s *DefaultSpan) Export() SpanData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SpanData{
		SpanID:     s.spanID,
		TraceID:    s.traceID,
		ParentID:   s.parentID,
		Name:       s.name,
		Kind:       s.kindString(),
		StartTime:  s.startTime,
		EndTime:    s.endTime,
		Duration:   s.Duration(),
		Attributes: s.Attributes(),
		Events:     s.Events(),
		Status:     s.status,
		Input:      s.input,
		Output:     s.output,
		TokenUsage: s.tokenUsage,
	}
}

// ToJSON 转换为 JSON
func (s *DefaultSpan) ToJSON() ([]byte, error) {
	return json.Marshal(s.Export())
}

func (s *DefaultSpan) kindString() string {
	switch s.kind {
	case SpanKindAgent:
		return "agent"
	case SpanKindLLM:
		return "llm"
	case SpanKindTool:
		return "tool"
	case SpanKindRetrieval:
		return "retrieval"
	case SpanKindEmbedding:
		return "embedding"
	default:
		return "internal"
	}
}

// 确保实现了 Span 接口
var _ Span = (*DefaultSpan)(nil)

// NoopSpan 空 Span（用于禁用追踪）
type NoopSpan struct{}

func (s *NoopSpan) SpanID() string                            { return "" }
func (s *NoopSpan) TraceID() string                           { return "" }
func (s *NoopSpan) SetName(name string)                       {}
func (s *NoopSpan) SetInput(input any)                        {}
func (s *NoopSpan) SetOutput(output any)                      {}
func (s *NoopSpan) SetTokenUsage(usage TokenUsage)            {}
func (s *NoopSpan) SetAttribute(key string, value any)        {}
func (s *NoopSpan) SetAttributes(attrs map[string]any)        {}
func (s *NoopSpan) AddEvent(name string, attrs ...any)        {}
func (s *NoopSpan) RecordError(err error)                     {}
func (s *NoopSpan) SetStatus(code StatusCode, message string) {}
func (s *NoopSpan) End()                                      {}
func (s *NoopSpan) EndWithError(err error)                    {}
func (s *NoopSpan) IsRecording() bool                         { return false }

var _ Span = (*NoopSpan)(nil)

// StartSpan 便捷函数：从 context 开始新 Span
func StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	tracer := TracerFromContext(ctx)
	if tracer == nil {
		// 返回 NoopSpan
		return ctx, &NoopSpan{}
	}
	return tracer.StartSpan(ctx, name, opts...)
}

// StartAgentSpan 开始 Agent Span
func StartAgentSpan(ctx context.Context, agentID, agentName string) (context.Context, Span) {
	ctx, span := StartSpan(ctx, "agent.run", WithSpanKind(SpanKindAgent))
	span.SetAttribute(AttrAgentID, agentID)
	span.SetAttribute(AttrAgentName, agentName)
	return ctx, span
}

// StartLLMSpan 开始 LLM Span
func StartLLMSpan(ctx context.Context, provider, model string) (context.Context, Span) {
	ctx, span := StartSpan(ctx, "llm.call", WithSpanKind(SpanKindLLM))
	span.SetAttribute(AttrLLMProvider, provider)
	span.SetAttribute(AttrLLMModel, model)
	return ctx, span
}

// StartToolSpan 开始 Tool Span
func StartToolSpan(ctx context.Context, toolName string) (context.Context, Span) {
	ctx, span := StartSpan(ctx, "tool.execute", WithSpanKind(SpanKindTool))
	span.SetAttribute(AttrToolName, toolName)
	return ctx, span
}

// StartRetrievalSpan 开始 Retrieval Span
func StartRetrievalSpan(ctx context.Context, query string, topK int) (context.Context, Span) {
	ctx, span := StartSpan(ctx, "retrieval.search", WithSpanKind(SpanKindRetrieval))
	span.SetAttribute(AttrRetrievalQuery, query)
	span.SetAttribute(AttrRetrievalTopK, topK)
	return ctx, span
}
