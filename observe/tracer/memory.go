package tracer

import (
	"context"
	"sync"

	"github.com/everyday-items/hexagon/internal/util"
)

// MemoryTracer 内存追踪器
// 将所有 Span 存储在内存中，适合开发和测试
type MemoryTracer struct {
	spans   []*DefaultSpan
	mu      sync.RWMutex
	traceID string
}

// NewMemoryTracer 创建内存追踪器
func NewMemoryTracer() *MemoryTracer {
	return &MemoryTracer{
		spans:   make([]*DefaultSpan, 0),
		traceID: util.TraceID(),
	}
}

// StartSpan 开始新 Span
func (t *MemoryTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	// 获取父 Span
	var parentSpan Span
	if parent := SpanFromContext(ctx); parent != nil {
		parentSpan = parent
	}

	// 添加父 Span 选项
	if parentSpan != nil {
		opts = append([]SpanOption{WithParent(parentSpan)}, opts...)
	}

	span := NewSpan(name, t.traceID, opts...)

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return ContextWithSpan(ctx, span), span
}

// ExtractTraceID 提取 Trace ID
func (t *MemoryTracer) ExtractTraceID(ctx context.Context) string {
	if span := SpanFromContext(ctx); span != nil {
		return span.TraceID()
	}
	return t.traceID
}

// InjectTraceID 注入 Trace ID
func (t *MemoryTracer) InjectTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

type traceIDKey struct{}

// Shutdown 关闭追踪器
func (t *MemoryTracer) Shutdown(ctx context.Context) error {
	return nil
}

// Spans 返回所有 Span
func (t *MemoryTracer) Spans() []*DefaultSpan {
	t.mu.RLock()
	defer t.mu.RUnlock()
	spans := make([]*DefaultSpan, len(t.spans))
	copy(spans, t.spans)
	return spans
}

// Clear 清除所有 Span
func (t *MemoryTracer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = make([]*DefaultSpan, 0)
	t.traceID = util.TraceID()
}

// Export 导出所有 Span 数据
func (t *MemoryTracer) Export() []SpanData {
	t.mu.RLock()
	defer t.mu.RUnlock()
	data := make([]SpanData, len(t.spans))
	for i, span := range t.spans {
		data[i] = span.Export()
	}
	return data
}

// 确保实现了 Tracer 接口
var _ Tracer = (*MemoryTracer)(nil)

// NoopTracer 空追踪器
type NoopTracer struct{}

func NewNoopTracer() *NoopTracer {
	return &NoopTracer{}
}

func (t *NoopTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	return ctx, &NoopSpan{}
}

func (t *NoopTracer) ExtractTraceID(ctx context.Context) string {
	return ""
}

func (t *NoopTracer) InjectTraceID(ctx context.Context, traceID string) context.Context {
	return ctx
}

func (t *NoopTracer) Shutdown(ctx context.Context) error {
	return nil
}

var _ Tracer = (*NoopTracer)(nil)

// GlobalTracer 全局追踪器
var (
	globalTracer Tracer = NewNoopTracer()
	globalMu     sync.RWMutex
)

// SetGlobalTracer 设置全局追踪器
func SetGlobalTracer(t Tracer) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalTracer = t
}

// GetGlobalTracer 获取全局追踪器
func GetGlobalTracer() Tracer {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalTracer
}

// Start 使用全局追踪器开始 Span
func Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	return GetGlobalTracer().StartSpan(ctx, name, opts...)
}
