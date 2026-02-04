package tracer

import (
	"context"
	"sync"

	"github.com/everyday-items/hexagon/internal/util"
)

// 默认最大 Span 数量
const defaultMaxSpans = 10000

// MemoryTracer 内存追踪器
//
// 将所有 Span 存储在内存中，适合开发和测试。
// 使用环形缓冲区防止 OOM：当 Span 数量超过 maxSpans 时，
// 自动丢弃最旧的 Span。
//
// 线程安全：所有方法都是并发安全的
type MemoryTracer struct {
	spans    []*DefaultSpan
	head     int // 环形缓冲区头部（下一个写入位置）
	size     int // 当前 Span 数量
	maxSpans int // 最大 Span 数量
	mu       sync.RWMutex
	traceID  string
}

// MemoryTracerOption MemoryTracer 配置选项
type MemoryTracerOption func(*MemoryTracer)

// WithMaxSpans 设置最大 Span 数量
func WithMaxSpans(max int) MemoryTracerOption {
	return func(t *MemoryTracer) {
		if max > 0 {
			t.maxSpans = max
		}
	}
}

// NewMemoryTracer 创建内存追踪器
func NewMemoryTracer(opts ...MemoryTracerOption) *MemoryTracer {
	t := &MemoryTracer{
		maxSpans: defaultMaxSpans,
		traceID:  util.TraceID(),
	}
	for _, opt := range opts {
		opt(t)
	}
	// 初始化环形缓冲区
	t.spans = make([]*DefaultSpan, t.maxSpans)
	return t
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
	// 使用环形缓冲区存储 Span
	t.spans[t.head] = span
	t.head = (t.head + 1) % t.maxSpans
	if t.size < t.maxSpans {
		t.size++
	}
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

// Spans 返回所有 Span（从最旧到最新）
func (t *MemoryTracer) Spans() []*DefaultSpan {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.size == 0 {
		return nil
	}

	result := make([]*DefaultSpan, 0, t.size)

	// 计算起始位置
	start := 0
	if t.size == t.maxSpans {
		start = t.head // 如果已满，从 head 开始是最旧的
	}

	for i := 0; i < t.size; i++ {
		idx := (start + i) % t.maxSpans
		if t.spans[idx] != nil {
			result = append(result, t.spans[idx])
		}
	}

	return result
}

// RecentSpans 返回最近 n 个 Span（从最新到最旧）
func (t *MemoryTracer) RecentSpans(n int) []*DefaultSpan {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if n <= 0 || t.size == 0 {
		return nil
	}

	if n > t.size {
		n = t.size
	}

	result := make([]*DefaultSpan, n)

	for i := 0; i < n; i++ {
		// 从 head-1 开始往回取
		idx := (t.head - 1 - i + t.maxSpans) % t.maxSpans
		if t.spans[idx] != nil {
			result[i] = t.spans[idx]
		}
	}

	return result
}

// Size 返回当前 Span 数量
func (t *MemoryTracer) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// MaxSpans 返回最大 Span 数量
func (t *MemoryTracer) MaxSpans() int {
	return t.maxSpans
}

// Clear 清除所有 Span
func (t *MemoryTracer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	// 清空所有引用
	for i := range t.spans {
		t.spans[i] = nil
	}
	t.head = 0
	t.size = 0
	t.traceID = util.TraceID()
}

// Export 导出所有 Span 数据（从最旧到最新）
func (t *MemoryTracer) Export() []SpanData {
	spans := t.Spans()
	data := make([]SpanData, len(spans))
	for i, span := range spans {
		if span != nil {
			data[i] = span.Export()
		}
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
