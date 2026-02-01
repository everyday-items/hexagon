// Package tracer 提供 Hexagon AI Agent 框架的分布式追踪能力
//
// Tracer 用于追踪 Agent 执行过程中的每个步骤，包括 LLM 调用、工具执行等。
// 支持 OpenTelemetry 标准，可以与 Jaeger、Zipkin 等后端集成。
//
// 主要类型：
//   - Tracer: 追踪器接口，负责创建和管理 Span
//   - Span: 追踪单元，记录一次操作的开始、结束、属性和事件
//   - TokenUsage: Token 使用量统计
//
// Span 类型：
//   - SpanKindAgent: Agent 执行
//   - SpanKindLLM: LLM 调用
//   - SpanKindTool: 工具执行
//   - SpanKindRetrieval: 检索操作
//   - SpanKindEmbedding: 嵌入操作
//
// 使用示例：
//
//	tracer := NewMemoryTracer()
//	ctx, span := tracer.StartSpan(ctx, "agent.run", WithSpanKind(SpanKindAgent))
//	defer span.End()
package tracer

import (
	"context"
	"time"
)

// Tracer 追踪器接口
type Tracer interface {
	// StartSpan 开始一个新的 Span
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)

	// ExtractTraceID 从 context 中提取 Trace ID
	ExtractTraceID(ctx context.Context) string

	// InjectTraceID 将 Trace ID 注入到 context 中
	InjectTraceID(ctx context.Context, traceID string) context.Context

	// Shutdown 关闭追踪器
	Shutdown(ctx context.Context) error
}

// Span 追踪 Span 接口
type Span interface {
	// SpanID 返回 Span ID
	SpanID() string

	// TraceID 返回 Trace ID
	TraceID() string

	// SetName 设置 Span 名称
	SetName(name string)

	// SetInput 设置输入
	SetInput(input any)

	// SetOutput 设置输出
	SetOutput(output any)

	// SetTokenUsage 设置 Token 使用量
	SetTokenUsage(usage TokenUsage)

	// SetAttribute 设置属性
	SetAttribute(key string, value any)

	// SetAttributes 批量设置属性
	SetAttributes(attrs map[string]any)

	// AddEvent 添加事件
	AddEvent(name string, attrs ...any)

	// RecordError 记录错误
	RecordError(err error)

	// SetStatus 设置状态
	SetStatus(code StatusCode, message string)

	// End 结束 Span
	End()

	// EndWithError 结束 Span 并记录错误
	EndWithError(err error)

	// IsRecording 是否正在记录
	IsRecording() bool
}

// TokenUsage Token 使用量
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StatusCode 状态码
type StatusCode int

const (
	// StatusCodeUnset 未设置
	StatusCodeUnset StatusCode = iota
	// StatusCodeOK 成功
	StatusCodeOK
	// StatusCodeError 错误
	StatusCodeError
)

// SpanKind Span 类型
type SpanKind int

const (
	// SpanKindInternal 内部操作
	SpanKindInternal SpanKind = iota
	// SpanKindAgent Agent 执行
	SpanKindAgent
	// SpanKindLLM LLM 调用
	SpanKindLLM
	// SpanKindTool 工具执行
	SpanKindTool
	// SpanKindRetrieval 检索操作
	SpanKindRetrieval
	// SpanKindEmbedding 嵌入操作
	SpanKindEmbedding
)

// SpanConfig Span 配置
type SpanConfig struct {
	Kind       SpanKind
	Attributes map[string]any
	StartTime  time.Time
	Parent     Span
}

// SpanOption Span 选项
type SpanOption func(*SpanConfig)

// WithSpanKind 设置 Span 类型
func WithSpanKind(kind SpanKind) SpanOption {
	return func(c *SpanConfig) {
		c.Kind = kind
	}
}

// WithAttributes 设置属性
func WithAttributes(attrs map[string]any) SpanOption {
	return func(c *SpanConfig) {
		c.Attributes = attrs
	}
}

// WithStartTime 设置开始时间
func WithStartTime(t time.Time) SpanOption {
	return func(c *SpanConfig) {
		c.StartTime = t
	}
}

// WithParent 设置父 Span
func WithParent(parent Span) SpanOption {
	return func(c *SpanConfig) {
		c.Parent = parent
	}
}

// 常用属性键
const (
	// Agent 相关
	AttrAgentID   = "agent.id"
	AttrAgentName = "agent.name"
	AttrAgentRole = "agent.role"

	// LLM 相关
	AttrLLMProvider         = "llm.provider"
	AttrLLMModel            = "llm.model"
	AttrLLMPromptTokens     = "llm.prompt_tokens"
	AttrLLMCompletionTokens = "llm.completion_tokens"
	AttrLLMTotalTokens      = "llm.total_tokens"
	AttrLLMTemperature      = "llm.temperature"
	AttrLLMMaxTokens        = "llm.max_tokens"

	// Tool 相关
	AttrToolName      = "tool.name"
	AttrToolArguments = "tool.arguments"
	AttrToolResult    = "tool.result"

	// Retrieval 相关
	AttrRetrievalQuery    = "retrieval.query"
	AttrRetrievalTopK     = "retrieval.top_k"
	AttrRetrievalDocCount = "retrieval.doc_count"

	// 错误相关
	AttrErrorType    = "error.type"
	AttrErrorMessage = "error.message"
)

// SpanFromContext 从 context 中获取当前 Span
func SpanFromContext(ctx context.Context) Span {
	if span, ok := ctx.Value(spanContextKey{}).(Span); ok {
		return span
	}
	return nil
}

// ContextWithSpan 将 Span 添加到 context
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

type spanContextKey struct{}

// TracerFromContext 从 context 中获取 Tracer
func TracerFromContext(ctx context.Context) Tracer {
	if tracer, ok := ctx.Value(tracerContextKey{}).(Tracer); ok {
		return tracer
	}
	return nil
}

// ContextWithTracer 将 Tracer 添加到 context
func ContextWithTracer(ctx context.Context, tracer Tracer) context.Context {
	return context.WithValue(ctx, tracerContextKey{}, tracer)
}

type tracerContextKey struct{}
