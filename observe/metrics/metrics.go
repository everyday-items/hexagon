// Package metrics 提供 Hexagon AI Agent 框架的指标收集
//
// Metrics 用于收集 Agent 执行过程中的各种指标，如请求次数、延迟、Token 使用等。
// 支持 Prometheus 格式，可以与各种监控系统集成。
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 指标接口
type Metrics interface {
	// Counter 获取或创建计数器
	Counter(name string, tags ...string) Counter

	// Histogram 获取或创建直方图
	Histogram(name string, tags ...string) Histogram

	// Gauge 获取或创建仪表盘
	Gauge(name string, tags ...string) Gauge

	// Timer 获取或创建计时器
	Timer(name string, tags ...string) Timer
}

// Counter 计数器接口
type Counter interface {
	// Inc 增加 1
	Inc()

	// Add 增加指定值
	Add(delta float64)

	// Value 获取当前值
	Value() float64
}

// Histogram 直方图接口
type Histogram interface {
	// Observe 记录观测值
	Observe(value float64)

	// Count 获取观测次数
	Count() uint64

	// Sum 获取总和
	Sum() float64
}

// Gauge 仪表盘接口
type Gauge interface {
	// Set 设置值
	Set(value float64)

	// Inc 增加 1
	Inc()

	// Dec 减少 1
	Dec()

	// Add 增加指定值
	Add(delta float64)

	// Value 获取当前值
	Value() float64
}

// Timer 计时器接口
type Timer interface {
	// ObserveDuration 记录持续时间
	ObserveDuration(d time.Duration)

	// Time 计时执行函数
	Time(fn func())

	// NewTimer 创建新的计时任务
	NewTimer() *TimerContext
}

// TimerContext 计时上下文
type TimerContext struct {
	start time.Time
	timer Timer
}

// Stop 停止计时并记录
func (tc *TimerContext) Stop() time.Duration {
	d := time.Since(tc.start)
	tc.timer.ObserveDuration(d)
	return d
}

// ============== 内存实现 ==============

// MemoryMetrics 内存指标实现
type MemoryMetrics struct {
	counters   map[string]*memoryCounter
	histograms map[string]*memoryHistogram
	gauges     map[string]*memoryGauge
	timers     map[string]*memoryTimer
	mu         sync.RWMutex
}

// NewMemoryMetrics 创建内存指标
func NewMemoryMetrics() *MemoryMetrics {
	return &MemoryMetrics{
		counters:   make(map[string]*memoryCounter),
		histograms: make(map[string]*memoryHistogram),
		gauges:     make(map[string]*memoryGauge),
		timers:     make(map[string]*memoryTimer),
	}
}

// Counter 获取或创建计数器
func (m *MemoryMetrics) Counter(name string, tags ...string) Counter {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := buildKey(name, tags)
	if c, ok := m.counters[key]; ok {
		return c
	}

	c := &memoryCounter{name: name, tags: tags}
	m.counters[key] = c
	return c
}

// Histogram 获取或创建直方图
func (m *MemoryMetrics) Histogram(name string, tags ...string) Histogram {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := buildKey(name, tags)
	if h, ok := m.histograms[key]; ok {
		return h
	}

	h := &memoryHistogram{name: name, tags: tags}
	m.histograms[key] = h
	return h
}

// Gauge 获取或创建仪表盘
func (m *MemoryMetrics) Gauge(name string, tags ...string) Gauge {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := buildKey(name, tags)
	if g, ok := m.gauges[key]; ok {
		return g
	}

	g := &memoryGauge{name: name, tags: tags}
	m.gauges[key] = g
	return g
}

// Timer 获取或创建计时器
func (m *MemoryMetrics) Timer(name string, tags ...string) Timer {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := buildKey(name, tags)
	if t, ok := m.timers[key]; ok {
		return t
	}

	t := &memoryTimer{name: name, tags: tags}
	m.timers[key] = t
	return t
}

// Snapshot 获取所有指标快照
func (m *MemoryMetrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := MetricsSnapshot{
		Counters:   make(map[string]float64),
		Gauges:     make(map[string]float64),
		Histograms: make(map[string]HistogramSnapshot),
		Timers:     make(map[string]HistogramSnapshot),
	}

	for k, c := range m.counters {
		snapshot.Counters[k] = c.Value()
	}

	for k, g := range m.gauges {
		snapshot.Gauges[k] = g.Value()
	}

	for k, h := range m.histograms {
		snapshot.Histograms[k] = HistogramSnapshot{
			Count: h.Count(),
			Sum:   h.Sum(),
		}
	}

	for k, t := range m.timers {
		snapshot.Timers[k] = HistogramSnapshot{
			Count: t.histogram.Count(),
			Sum:   t.histogram.Sum(),
		}
	}

	return snapshot
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	Counters   map[string]float64           `json:"counters"`
	Gauges     map[string]float64           `json:"gauges"`
	Histograms map[string]HistogramSnapshot `json:"histograms"`
	Timers     map[string]HistogramSnapshot `json:"timers"`
}

// HistogramSnapshot 直方图快照
type HistogramSnapshot struct {
	Count uint64  `json:"count"`
	Sum   float64 `json:"sum"`
}

// ============== 内部实现 ==============

type memoryCounter struct {
	name  string
	tags  []string
	value atomic.Uint64
}

func (c *memoryCounter) Inc() {
	c.Add(1)
}

func (c *memoryCounter) Add(delta float64) {
	// 使用 uint64 存储（乘以 1000 保留精度）
	for {
		old := c.value.Load()
		new := old + uint64(delta*1000)
		if c.value.CompareAndSwap(old, new) {
			break
		}
	}
}

func (c *memoryCounter) Value() float64 {
	return float64(c.value.Load()) / 1000
}

type memoryHistogram struct {
	name  string
	tags  []string
	count atomic.Uint64
	sum   atomic.Uint64
	mu    sync.Mutex
}

func (h *memoryHistogram) Observe(value float64) {
	h.count.Add(1)
	// 使用 uint64 存储（乘以 1000 保留精度）
	for {
		old := h.sum.Load()
		new := old + uint64(value*1000)
		if h.sum.CompareAndSwap(old, new) {
			break
		}
	}
}

func (h *memoryHistogram) Count() uint64 {
	return h.count.Load()
}

func (h *memoryHistogram) Sum() float64 {
	return float64(h.sum.Load()) / 1000
}

type memoryGauge struct {
	name  string
	tags  []string
	value atomic.Uint64
}

func (g *memoryGauge) Set(value float64) {
	g.value.Store(uint64(value * 1000))
}

func (g *memoryGauge) Inc() {
	g.Add(1)
}

func (g *memoryGauge) Dec() {
	g.Add(-1)
}

func (g *memoryGauge) Add(delta float64) {
	for {
		old := g.value.Load()
		// 处理负数
		newVal := int64(old) + int64(delta*1000)
		if newVal < 0 {
			newVal = 0
		}
		if g.value.CompareAndSwap(old, uint64(newVal)) {
			break
		}
	}
}

func (g *memoryGauge) Value() float64 {
	return float64(g.value.Load()) / 1000
}

type memoryTimer struct {
	name      string
	tags      []string
	histogram memoryHistogram
}

func (t *memoryTimer) ObserveDuration(d time.Duration) {
	t.histogram.Observe(d.Seconds())
}

func (t *memoryTimer) Time(fn func()) {
	start := time.Now()
	fn()
	t.ObserveDuration(time.Since(start))
}

func (t *memoryTimer) NewTimer() *TimerContext {
	return &TimerContext{
		start: time.Now(),
		timer: t,
	}
}

func buildKey(name string, tags []string) string {
	key := name
	for i := 0; i < len(tags)-1; i += 2 {
		key += "," + tags[i] + "=" + tags[i+1]
	}
	return key
}

// 确保实现了接口
var (
	_ Metrics   = (*MemoryMetrics)(nil)
	_ Counter   = (*memoryCounter)(nil)
	_ Histogram = (*memoryHistogram)(nil)
	_ Gauge     = (*memoryGauge)(nil)
	_ Timer     = (*memoryTimer)(nil)
)

// ============== 预定义指标名称 ==============

const (
	// Agent 相关
	MetricAgentRunsTotal     = "hexagon_agent_runs_total"
	MetricAgentRunDuration   = "hexagon_agent_run_duration_seconds"
	MetricAgentRunErrors     = "hexagon_agent_run_errors_total"
	MetricAgentActiveCount   = "hexagon_agent_active_count"

	// LLM 相关
	MetricLLMCallsTotal       = "hexagon_llm_calls_total"
	MetricLLMCallDuration     = "hexagon_llm_call_duration_seconds"
	MetricLLMCallErrors       = "hexagon_llm_call_errors_total"
	MetricLLMPromptTokens     = "hexagon_llm_prompt_tokens_total"
	MetricLLMCompletionTokens = "hexagon_llm_completion_tokens_total"

	// Tool 相关
	MetricToolCallsTotal   = "hexagon_tool_calls_total"
	MetricToolCallDuration = "hexagon_tool_call_duration_seconds"
	MetricToolCallErrors   = "hexagon_tool_call_errors_total"

	// RAG 相关
	MetricRetrievalTotal    = "hexagon_retrieval_total"
	MetricRetrievalDuration = "hexagon_retrieval_duration_seconds"
	MetricRetrievalDocCount = "hexagon_retrieval_doc_count"
)

// ============== 全局指标 ==============

var (
	globalMetrics Metrics = NewMemoryMetrics()
	globalMu      sync.RWMutex
)

// SetGlobalMetrics 设置全局指标
func SetGlobalMetrics(m Metrics) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalMetrics = m
}

// GetGlobalMetrics 获取全局指标
func GetGlobalMetrics() Metrics {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalMetrics
}

// ============== 便捷函数 ==============

// IncCounter 增加全局计数器
func IncCounter(name string, tags ...string) {
	GetGlobalMetrics().Counter(name, tags...).Inc()
}

// AddCounter 增加全局计数器指定值
func AddCounter(name string, delta float64, tags ...string) {
	GetGlobalMetrics().Counter(name, tags...).Add(delta)
}

// ObserveHistogram 记录直方图观测值
func ObserveHistogram(name string, value float64, tags ...string) {
	GetGlobalMetrics().Histogram(name, tags...).Observe(value)
}

// SetGauge 设置仪表盘值
func SetGauge(name string, value float64, tags ...string) {
	GetGlobalMetrics().Gauge(name, tags...).Set(value)
}

// TimeDuration 记录持续时间
func TimeDuration(name string, d time.Duration, tags ...string) {
	GetGlobalMetrics().Timer(name, tags...).ObserveDuration(d)
}

// StartTimer 开始计时并返回计时上下文
func StartTimer(name string, tags ...string) *TimerContext {
	return GetGlobalMetrics().Timer(name, tags...).NewTimer()
}
