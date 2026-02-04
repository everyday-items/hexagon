// Package prometheus 提供 Prometheus 指标导出
//
// 本文件提供基于钩子的自动 Prometheus 指标收集，包括：
//   - MetricsRunHook: 自动收集 Agent 运行指标
//   - MetricsToolHook: 自动收集工具调用指标
//   - MetricsLLMHook: 自动收集 LLM 调用指标
//   - MetricsRetrieverHook: 自动收集检索指标
//
// 使用示例:
//
//	// 设置自动指标收集
//	prometheus.SetupMetrics(hookManager)
package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/hooks"
	"github.com/everyday-items/hexagon/observe/metrics"
)

// ============== Metrics Hooks 配置 ==============

// MetricsHooksConfig 指标钩子配置
type MetricsHooksConfig struct {
	// Metrics 指标实现（可选，默认使用全局指标）
	Metrics metrics.Metrics
}

// MetricsHooksOption 指标钩子选项
type MetricsHooksOption func(*MetricsHooksConfig)

// WithMetricsInstance 使用指定的指标实例
func WithMetricsInstance(m metrics.Metrics) MetricsHooksOption {
	return func(c *MetricsHooksConfig) {
		c.Metrics = m
	}
}

// ============== 运行时状态追踪 ==============

// metricsState 指标收集的运行时状态
type metricsState struct {
	runStartTimes      sync.Map // runID -> time.Time
	toolStartTimes     sync.Map // toolID -> time.Time
	llmStartTimes      sync.Map // requestID -> time.Time
	retrieverStartTimes sync.Map // queryID -> time.Time
}

var globalMetricsState = &metricsState{}

// ============== Metrics Hooks ==============

// MetricsRunHook 基于 Metrics 接口的运行指标钩子
type MetricsRunHook struct {
	metrics metrics.Metrics
	state   *metricsState
	enabled bool
}

// NewMetricsRunHook 创建运行指标钩子
func NewMetricsRunHook(opts ...MetricsHooksOption) *MetricsRunHook {
	config := &MetricsHooksConfig{}
	for _, opt := range opts {
		opt(config)
	}

	m := config.Metrics
	if m == nil {
		m = metrics.GetGlobalMetrics()
	}

	return &MetricsRunHook{
		metrics: m,
		state:   globalMetricsState,
		enabled: true,
	}
}

// Name 返回钩子名称
func (h *MetricsRunHook) Name() string { return "prometheus-metrics-run" }

// Enabled 返回钩子是否启用
func (h *MetricsRunHook) Enabled() bool { return h.enabled }

// Timings 返回关心的时机
func (h *MetricsRunHook) Timings() hooks.Timing {
	return hooks.TimingRunStart | hooks.TimingRunEnd | hooks.TimingRunError
}

// OnStart 运行开始时记录开始时间
func (h *MetricsRunHook) OnStart(ctx context.Context, event *hooks.RunStartEvent) error {
	h.state.runStartTimes.Store(event.RunID, time.Now())
	h.metrics.Gauge(metrics.MetricAgentActiveCount, "agent", event.AgentID).Inc()
	return nil
}

// OnEnd 运行结束时记录指标
func (h *MetricsRunHook) OnEnd(ctx context.Context, event *hooks.RunEndEvent) error {
	h.metrics.Gauge(metrics.MetricAgentActiveCount, "agent", event.AgentID).Dec()

	// 计算持续时间
	if startTimeI, ok := h.state.runStartTimes.LoadAndDelete(event.RunID); ok {
		startTime := startTimeI.(time.Time)
		duration := time.Since(startTime)
		h.metrics.Timer(metrics.MetricAgentRunDuration, "agent", event.AgentID).ObserveDuration(duration)
	}

	h.metrics.Counter(metrics.MetricAgentRunsTotal, "agent", event.AgentID, "status", "success").Inc()
	return nil
}

// OnError 运行错误时记录错误指标
func (h *MetricsRunHook) OnError(ctx context.Context, event *hooks.ErrorEvent) error {
	h.metrics.Gauge(metrics.MetricAgentActiveCount, "agent", event.AgentID).Dec()
	h.state.runStartTimes.Delete(event.RunID)

	h.metrics.Counter(metrics.MetricAgentRunsTotal, "agent", event.AgentID, "status", "error").Inc()

	errorType := "unknown"
	if event.Phase != "" {
		errorType = event.Phase
	}
	h.metrics.Counter(metrics.MetricAgentRunErrors, "agent", event.AgentID, "error_type", errorType).Inc()
	return nil
}

// MetricsToolHook 基于 Metrics 接口的工具指标钩子
type MetricsToolHook struct {
	metrics metrics.Metrics
	state   *metricsState
	enabled bool
}

// NewMetricsToolHook 创建工具指标钩子
func NewMetricsToolHook(opts ...MetricsHooksOption) *MetricsToolHook {
	config := &MetricsHooksConfig{}
	for _, opt := range opts {
		opt(config)
	}

	m := config.Metrics
	if m == nil {
		m = metrics.GetGlobalMetrics()
	}

	return &MetricsToolHook{
		metrics: m,
		state:   globalMetricsState,
		enabled: true,
	}
}

// Name 返回钩子名称
func (h *MetricsToolHook) Name() string { return "prometheus-metrics-tool" }

// Enabled 返回钩子是否启用
func (h *MetricsToolHook) Enabled() bool { return h.enabled }

// Timings 返回关心的时机
func (h *MetricsToolHook) Timings() hooks.Timing {
	return hooks.TimingToolStart | hooks.TimingToolEnd
}

// OnToolStart 工具开始时记录开始时间
func (h *MetricsToolHook) OnToolStart(ctx context.Context, event *hooks.ToolStartEvent) error {
	h.state.toolStartTimes.Store(event.ToolID, time.Now())
	return nil
}

// OnToolEnd 工具结束时记录指标
func (h *MetricsToolHook) OnToolEnd(ctx context.Context, event *hooks.ToolEndEvent) error {
	// 计算持续时间
	if startTimeI, ok := h.state.toolStartTimes.LoadAndDelete(event.ToolID); ok {
		startTime := startTimeI.(time.Time)
		duration := time.Since(startTime)
		h.metrics.Timer(metrics.MetricToolCallDuration, "tool", event.ToolName).ObserveDuration(duration)
	}

	if event.Error != nil {
		h.metrics.Counter(metrics.MetricToolCallsTotal, "tool", event.ToolName, "status", "error").Inc()
		h.metrics.Counter(metrics.MetricToolCallErrors, "tool", event.ToolName, "error_type", "execution").Inc()
	} else {
		h.metrics.Counter(metrics.MetricToolCallsTotal, "tool", event.ToolName, "status", "success").Inc()
	}
	return nil
}

// MetricsLLMHook 基于 Metrics 接口的 LLM 指标钩子
type MetricsLLMHook struct {
	metrics metrics.Metrics
	state   *metricsState
	enabled bool
}

// NewMetricsLLMHook 创建 LLM 指标钩子
func NewMetricsLLMHook(opts ...MetricsHooksOption) *MetricsLLMHook {
	config := &MetricsHooksConfig{}
	for _, opt := range opts {
		opt(config)
	}

	m := config.Metrics
	if m == nil {
		m = metrics.GetGlobalMetrics()
	}

	return &MetricsLLMHook{
		metrics: m,
		state:   globalMetricsState,
		enabled: true,
	}
}

// Name 返回钩子名称
func (h *MetricsLLMHook) Name() string { return "prometheus-metrics-llm" }

// Enabled 返回钩子是否启用
func (h *MetricsLLMHook) Enabled() bool { return h.enabled }

// Timings 返回关心的时机
func (h *MetricsLLMHook) Timings() hooks.Timing {
	return hooks.TimingLLMStart | hooks.TimingLLMEnd
}

// OnLLMStart LLM 开始时记录开始时间
func (h *MetricsLLMHook) OnLLMStart(ctx context.Context, event *hooks.LLMStartEvent) error {
	h.state.llmStartTimes.Store(event.RequestID, time.Now())
	return nil
}

// OnLLMEnd LLM 结束时记录指标
func (h *MetricsLLMHook) OnLLMEnd(ctx context.Context, event *hooks.LLMEndEvent) error {
	provider := event.Model // 暂时用 model 作为 provider
	model := event.Model

	// 计算持续时间
	if startTimeI, ok := h.state.llmStartTimes.LoadAndDelete(event.RequestID); ok {
		startTime := startTimeI.(time.Time)
		duration := time.Since(startTime)
		h.metrics.Timer(metrics.MetricLLMCallDuration, "provider", provider, "model", model).ObserveDuration(duration)
	}

	// 记录 token 使用
	h.metrics.Counter(metrics.MetricLLMPromptTokens, "provider", provider, "model", model).Add(float64(event.PromptTokens))
	h.metrics.Counter(metrics.MetricLLMCompletionTokens, "provider", provider, "model", model).Add(float64(event.CompletionTokens))

	if event.Error != nil {
		h.metrics.Counter(metrics.MetricLLMCallsTotal, "provider", provider, "model", model, "status", "error").Inc()
		h.metrics.Counter(metrics.MetricLLMCallErrors, "provider", provider, "model", model, "error_type", "api").Inc()
	} else {
		h.metrics.Counter(metrics.MetricLLMCallsTotal, "provider", provider, "model", model, "status", "success").Inc()
	}
	return nil
}

// OnLLMStream LLM 流式输出时不记录额外指标
func (h *MetricsLLMHook) OnLLMStream(ctx context.Context, event *hooks.LLMStreamEvent) error {
	return nil
}

// MetricsRetrieverHook 基于 Metrics 接口的检索指标钩子
type MetricsRetrieverHook struct {
	metrics metrics.Metrics
	state   *metricsState
	enabled bool
}

// NewMetricsRetrieverHook 创建检索指标钩子
func NewMetricsRetrieverHook(opts ...MetricsHooksOption) *MetricsRetrieverHook {
	config := &MetricsHooksConfig{}
	for _, opt := range opts {
		opt(config)
	}

	m := config.Metrics
	if m == nil {
		m = metrics.GetGlobalMetrics()
	}

	return &MetricsRetrieverHook{
		metrics: m,
		state:   globalMetricsState,
		enabled: true,
	}
}

// Name 返回钩子名称
func (h *MetricsRetrieverHook) Name() string { return "prometheus-metrics-retriever" }

// Enabled 返回钩子是否启用
func (h *MetricsRetrieverHook) Enabled() bool { return h.enabled }

// Timings 返回关心的时机
func (h *MetricsRetrieverHook) Timings() hooks.Timing {
	return hooks.TimingRetrieverStart | hooks.TimingRetrieverEnd
}

// OnRetrieverStart 检索开始时记录开始时间
func (h *MetricsRetrieverHook) OnRetrieverStart(ctx context.Context, event *hooks.RetrieverStartEvent) error {
	h.state.retrieverStartTimes.Store(event.QueryID, time.Now())
	return nil
}

// OnRetrieverEnd 检索结束时记录指标
func (h *MetricsRetrieverHook) OnRetrieverEnd(ctx context.Context, event *hooks.RetrieverEndEvent) error {
	retriever := "default"

	// 计算持续时间
	if startTimeI, ok := h.state.retrieverStartTimes.LoadAndDelete(event.QueryID); ok {
		startTime := startTimeI.(time.Time)
		duration := time.Since(startTime)
		h.metrics.Timer(metrics.MetricRetrievalDuration, "retriever", retriever).ObserveDuration(duration)
	}

	// 记录文档数量
	h.metrics.Histogram(metrics.MetricRetrievalDocCount, "retriever", retriever).Observe(float64(event.DocCount))

	if event.Error != nil {
		h.metrics.Counter(metrics.MetricRetrievalTotal, "retriever", retriever, "status", "error").Inc()
	} else {
		h.metrics.Counter(metrics.MetricRetrievalTotal, "retriever", retriever, "status", "success").Inc()
	}
	return nil
}

// ============== 快捷函数 ==============

// SetupMetrics 设置完整指标收集
// 创建并注册所有指标钩子
func SetupMetrics(manager *hooks.Manager, opts ...MetricsHooksOption) {
	manager.RegisterRunHook(NewMetricsRunHook(opts...))
	manager.RegisterToolHook(NewMetricsToolHook(opts...))
	manager.RegisterLLMHook(NewMetricsLLMHook(opts...))
	manager.RegisterRetrieverHook(NewMetricsRetrieverHook(opts...))
}

// SetupMetricsWithExporter 设置带导出器的指标收集
// 将指标收集到内存，可通过 GetGlobalMetrics 获取快照
func SetupMetricsWithExporter(manager *hooks.Manager) *metrics.MemoryMetrics {
	m := metrics.NewMemoryMetrics()
	metrics.SetGlobalMetrics(m)
	opts := []MetricsHooksOption{WithMetricsInstance(m)}
	SetupMetrics(manager, opts...)
	return m
}
