// Package metrics 提供 Hexagon AI Agent 框架的业务指标采集
package metrics

import (
	"context"
	"sync"
	"time"
)

// maxStatsEntries 单个统计 map 的最大条目数
// 防止恶意或错误使用导致内存无限增长
const maxStatsEntries = 10000

// HexagonMetrics Hexagon 业务指标收集器
//
// 提供 Agent、LLM、Tool、RAG 等组件的统一指标采集。
// 设计用于与 hooks 系统集成，自动收集执行指标。
//
// 内存安全：每个统计 map 最多保存 10000 个条目，超出后新条目将被忽略。
//
// 使用示例:
//
//	collector := NewHexagonMetrics(metrics)
//	collector.RecordAgentRun(ctx, "my-agent", time.Second, nil)
//	collector.RecordLLMCall(ctx, "openai", "gpt-4", 100, 50, time.Second, nil)
type HexagonMetrics struct {
	metrics Metrics

	// 聚合统计
	mu             sync.RWMutex
	agentStats     map[string]*AgentStats
	llmStats       map[string]*LLMStats
	toolStats      map[string]*ToolStats
	retrievalStats *RetrievalStats
}

// AgentStats Agent 统计信息
type AgentStats struct {
	TotalRuns      int64
	TotalErrors    int64
	TotalDuration  time.Duration
	LastRunTime    time.Time
	AverageDuration time.Duration
}

// LLMStats LLM 统计信息
type LLMStats struct {
	TotalCalls        int64
	TotalErrors       int64
	TotalPromptTokens int64
	TotalCompletionTokens int64
	TotalDuration     time.Duration
	LastCallTime      time.Time
	AverageDuration   time.Duration
}

// ToolStats Tool 统计信息
type ToolStats struct {
	TotalCalls    int64
	TotalErrors   int64
	TotalDuration time.Duration
	LastCallTime  time.Time
	AverageDuration time.Duration
}

// RetrievalStats 检索统计信息
type RetrievalStats struct {
	TotalRetrievals int64
	TotalDocuments  int64
	TotalDuration   time.Duration
	LastRetrievalTime time.Time
	AverageDuration   time.Duration
	AverageDocCount   float64
}

// NewHexagonMetrics 创建 Hexagon 业务指标收集器
//
// 参数:
//   - metrics: 底层指标实现，nil 时使用内存实现
//
// 返回:
//   - 新的收集器实例
func NewHexagonMetrics(metrics Metrics) *HexagonMetrics {
	if metrics == nil {
		metrics = NewMemoryMetrics()
	}

	return &HexagonMetrics{
		metrics:        metrics,
		agentStats:     make(map[string]*AgentStats),
		llmStats:       make(map[string]*LLMStats),
		toolStats:      make(map[string]*ToolStats),
		retrievalStats: &RetrievalStats{},
	}
}

// ============== Agent 指标 ==============

// RecordAgentRun 记录 Agent 执行
//
// 参数:
//   - ctx: 上下文
//   - agentName: Agent 名称
//   - duration: 执行时长
//   - err: 执行错误（可为 nil）
func (h *HexagonMetrics) RecordAgentRun(ctx context.Context, agentName string, duration time.Duration, err error) {
	// 增加执行次数
	h.metrics.Counter(MetricAgentRunsTotal, "agent", agentName).Inc()

	// 记录耗时
	h.metrics.Timer(MetricAgentRunDuration, "agent", agentName).ObserveDuration(duration)

	// 记录错误
	if err != nil {
		h.metrics.Counter(MetricAgentRunErrors, "agent", agentName).Inc()
	}

	// 更新聚合统计
	h.mu.Lock()
	defer h.mu.Unlock()

	stats, ok := h.agentStats[agentName]
	if !ok {
		// 检查 map 大小限制
		if len(h.agentStats) >= maxStatsEntries {
			return // 达到上限，忽略新条目
		}
		stats = &AgentStats{}
		h.agentStats[agentName] = stats
	}

	stats.TotalRuns++
	stats.TotalDuration += duration
	stats.LastRunTime = time.Now()
	if err != nil {
		stats.TotalErrors++
	}
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalRuns)
}

// SetAgentActive 设置活跃 Agent 数量
func (h *HexagonMetrics) SetAgentActive(count int) {
	h.metrics.Gauge(MetricAgentActiveCount).Set(float64(count))
}

// IncrementAgentActive 增加活跃 Agent 数量
func (h *HexagonMetrics) IncrementAgentActive() {
	h.metrics.Gauge(MetricAgentActiveCount).Inc()
}

// DecrementAgentActive 减少活跃 Agent 数量
func (h *HexagonMetrics) DecrementAgentActive() {
	h.metrics.Gauge(MetricAgentActiveCount).Dec()
}

// GetAgentStats 获取 Agent 统计信息
func (h *HexagonMetrics) GetAgentStats(agentName string) *AgentStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if stats, ok := h.agentStats[agentName]; ok {
		// 返回副本
		copy := *stats
		return &copy
	}
	return nil
}

// GetAllAgentStats 获取所有 Agent 统计信息
func (h *HexagonMetrics) GetAllAgentStats() map[string]*AgentStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*AgentStats)
	for name, stats := range h.agentStats {
		copy := *stats
		result[name] = &copy
	}
	return result
}

// ============== LLM 指标 ==============

// RecordLLMCall 记录 LLM 调用
//
// 参数:
//   - ctx: 上下文
//   - provider: LLM 提供商 (如 "openai")
//   - model: 模型名称 (如 "gpt-4")
//   - promptTokens: 提示 token 数量
//   - completionTokens: 完成 token 数量
//   - duration: 调用时长
//   - err: 调用错误（可为 nil）
func (h *HexagonMetrics) RecordLLMCall(
	ctx context.Context,
	provider, model string,
	promptTokens, completionTokens int,
	duration time.Duration,
	err error,
) {
	// 增加调用次数
	h.metrics.Counter(MetricLLMCallsTotal, "provider", provider, "model", model).Inc()

	// 记录耗时
	h.metrics.Timer(MetricLLMCallDuration, "provider", provider, "model", model).ObserveDuration(duration)

	// 记录 Token
	h.metrics.Counter(MetricLLMPromptTokens, "provider", provider, "model", model).Add(float64(promptTokens))
	h.metrics.Counter(MetricLLMCompletionTokens, "provider", provider, "model", model).Add(float64(completionTokens))

	// 记录错误
	if err != nil {
		h.metrics.Counter(MetricLLMCallErrors, "provider", provider, "model", model).Inc()
	}

	// 更新聚合统计
	h.mu.Lock()
	defer h.mu.Unlock()

	key := provider + "/" + model
	stats, ok := h.llmStats[key]
	if !ok {
		// 检查 map 大小限制
		if len(h.llmStats) >= maxStatsEntries {
			return // 达到上限，忽略新条目
		}
		stats = &LLMStats{}
		h.llmStats[key] = stats
	}

	stats.TotalCalls++
	stats.TotalPromptTokens += int64(promptTokens)
	stats.TotalCompletionTokens += int64(completionTokens)
	stats.TotalDuration += duration
	stats.LastCallTime = time.Now()
	if err != nil {
		stats.TotalErrors++
	}
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalCalls)
}

// GetLLMStats 获取 LLM 统计信息
func (h *HexagonMetrics) GetLLMStats(provider, model string) *LLMStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := provider + "/" + model
	if stats, ok := h.llmStats[key]; ok {
		copy := *stats
		return &copy
	}
	return nil
}

// GetAllLLMStats 获取所有 LLM 统计信息
func (h *HexagonMetrics) GetAllLLMStats() map[string]*LLMStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*LLMStats)
	for key, stats := range h.llmStats {
		copy := *stats
		result[key] = &copy
	}
	return result
}

// ============== Tool 指标 ==============

// RecordToolCall 记录工具调用
//
// 参数:
//   - ctx: 上下文
//   - toolName: 工具名称
//   - duration: 调用时长
//   - err: 调用错误（可为 nil）
func (h *HexagonMetrics) RecordToolCall(ctx context.Context, toolName string, duration time.Duration, err error) {
	// 增加调用次数
	h.metrics.Counter(MetricToolCallsTotal, "tool", toolName).Inc()

	// 记录耗时
	h.metrics.Timer(MetricToolCallDuration, "tool", toolName).ObserveDuration(duration)

	// 记录错误
	if err != nil {
		h.metrics.Counter(MetricToolCallErrors, "tool", toolName).Inc()
	}

	// 更新聚合统计
	h.mu.Lock()
	defer h.mu.Unlock()

	stats, ok := h.toolStats[toolName]
	if !ok {
		// 检查 map 大小限制
		if len(h.toolStats) >= maxStatsEntries {
			return // 达到上限，忽略新条目
		}
		stats = &ToolStats{}
		h.toolStats[toolName] = stats
	}

	stats.TotalCalls++
	stats.TotalDuration += duration
	stats.LastCallTime = time.Now()
	if err != nil {
		stats.TotalErrors++
	}
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalCalls)
}

// GetToolStats 获取工具统计信息
func (h *HexagonMetrics) GetToolStats(toolName string) *ToolStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if stats, ok := h.toolStats[toolName]; ok {
		copy := *stats
		return &copy
	}
	return nil
}

// GetAllToolStats 获取所有工具统计信息
func (h *HexagonMetrics) GetAllToolStats() map[string]*ToolStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*ToolStats)
	for name, stats := range h.toolStats {
		copy := *stats
		result[name] = &copy
	}
	return result
}

// ============== RAG 指标 ==============

// RecordRetrieval 记录检索操作
//
// 参数:
//   - ctx: 上下文
//   - retrieverName: 检索器名称
//   - docCount: 检索到的文档数量
//   - duration: 检索时长
func (h *HexagonMetrics) RecordRetrieval(ctx context.Context, retrieverName string, docCount int, duration time.Duration) {
	// 增加检索次数
	h.metrics.Counter(MetricRetrievalTotal, "retriever", retrieverName).Inc()

	// 记录耗时
	h.metrics.Timer(MetricRetrievalDuration, "retriever", retrieverName).ObserveDuration(duration)

	// 记录文档数量
	h.metrics.Histogram(MetricRetrievalDocCount, "retriever", retrieverName).Observe(float64(docCount))

	// 更新聚合统计
	h.mu.Lock()
	defer h.mu.Unlock()

	stats := h.retrievalStats
	stats.TotalRetrievals++
	stats.TotalDocuments += int64(docCount)
	stats.TotalDuration += duration
	stats.LastRetrievalTime = time.Now()
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalRetrievals)
	stats.AverageDocCount = float64(stats.TotalDocuments) / float64(stats.TotalRetrievals)
}

// GetRetrievalStats 获取检索统计信息
func (h *HexagonMetrics) GetRetrievalStats() *RetrievalStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	copy := *h.retrievalStats
	return &copy
}

// ============== 综合指标 ==============

// Summary Hexagon 指标汇总
type Summary struct {
	Timestamp       time.Time                `json:"timestamp"`
	AgentStats      map[string]*AgentStats   `json:"agent_stats"`
	LLMStats        map[string]*LLMStats     `json:"llm_stats"`
	ToolStats       map[string]*ToolStats    `json:"tool_stats"`
	RetrievalStats  *RetrievalStats          `json:"retrieval_stats"`
	TotalAgentRuns  int64                    `json:"total_agent_runs"`
	TotalLLMCalls   int64                    `json:"total_llm_calls"`
	TotalToolCalls  int64                    `json:"total_tool_calls"`
	TotalRetrievals int64                    `json:"total_retrievals"`
	TotalErrors     int64                    `json:"total_errors"`
	TotalTokens     int64                    `json:"total_tokens"`
}

// GetSummary 获取指标汇总
func (h *HexagonMetrics) GetSummary() *Summary {
	h.mu.RLock()
	defer h.mu.RUnlock()

	summary := &Summary{
		Timestamp:      time.Now(),
		AgentStats:     make(map[string]*AgentStats),
		LLMStats:       make(map[string]*LLMStats),
		ToolStats:      make(map[string]*ToolStats),
		RetrievalStats: nil,
	}

	// 复制 Agent 统计
	for name, stats := range h.agentStats {
		copy := *stats
		summary.AgentStats[name] = &copy
		summary.TotalAgentRuns += stats.TotalRuns
		summary.TotalErrors += stats.TotalErrors
	}

	// 复制 LLM 统计
	for key, stats := range h.llmStats {
		copy := *stats
		summary.LLMStats[key] = &copy
		summary.TotalLLMCalls += stats.TotalCalls
		summary.TotalErrors += stats.TotalErrors
		summary.TotalTokens += stats.TotalPromptTokens + stats.TotalCompletionTokens
	}

	// 复制 Tool 统计
	for name, stats := range h.toolStats {
		copy := *stats
		summary.ToolStats[name] = &copy
		summary.TotalToolCalls += stats.TotalCalls
		summary.TotalErrors += stats.TotalErrors
	}

	// 复制 Retrieval 统计
	if h.retrievalStats != nil {
		copy := *h.retrievalStats
		summary.RetrievalStats = &copy
		summary.TotalRetrievals = h.retrievalStats.TotalRetrievals
	}

	return summary
}

// Reset 重置所有统计
func (h *HexagonMetrics) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.agentStats = make(map[string]*AgentStats)
	h.llmStats = make(map[string]*LLMStats)
	h.toolStats = make(map[string]*ToolStats)
	h.retrievalStats = &RetrievalStats{}
}

// ============== Hooks 集成 ==============

// AgentRunHook 返回用于 Agent 执行的 Hook 函数
//
// 用于与 hooks 系统集成，自动记录 Agent 执行指标
//
// 使用示例:
//
//	hooks.OnAgentRun(collector.AgentRunHook())
func (h *HexagonMetrics) AgentRunHook() func(ctx context.Context, agentName string, duration time.Duration, err error) {
	return func(ctx context.Context, agentName string, duration time.Duration, err error) {
		h.RecordAgentRun(ctx, agentName, duration, err)
	}
}

// LLMCallHook 返回用于 LLM 调用的 Hook 函数
func (h *HexagonMetrics) LLMCallHook() func(ctx context.Context, provider, model string, promptTokens, completionTokens int, duration time.Duration, err error) {
	return func(ctx context.Context, provider, model string, promptTokens, completionTokens int, duration time.Duration, err error) {
		h.RecordLLMCall(ctx, provider, model, promptTokens, completionTokens, duration, err)
	}
}

// ToolCallHook 返回用于工具调用的 Hook 函数
func (h *HexagonMetrics) ToolCallHook() func(ctx context.Context, toolName string, duration time.Duration, err error) {
	return func(ctx context.Context, toolName string, duration time.Duration, err error) {
		h.RecordToolCall(ctx, toolName, duration, err)
	}
}

// RetrievalHook 返回用于检索的 Hook 函数
func (h *HexagonMetrics) RetrievalHook() func(ctx context.Context, retrieverName string, docCount int, duration time.Duration) {
	return func(ctx context.Context, retrieverName string, docCount int, duration time.Duration) {
		h.RecordRetrieval(ctx, retrieverName, docCount, duration)
	}
}

// ============== 全局实例 ==============

var (
	globalHexagonMetrics   *HexagonMetrics
	globalHexagonMetricsMu sync.RWMutex
)

// GetHexagonMetrics 获取全局 Hexagon 指标收集器
//
// 如果尚未初始化，将创建默认实例。
// 线程安全：此方法是并发安全的。
func GetHexagonMetrics() *HexagonMetrics {
	globalHexagonMetricsMu.RLock()
	if globalHexagonMetrics != nil {
		defer globalHexagonMetricsMu.RUnlock()
		return globalHexagonMetrics
	}
	globalHexagonMetricsMu.RUnlock()

	// 需要初始化，使用写锁
	globalHexagonMetricsMu.Lock()
	defer globalHexagonMetricsMu.Unlock()
	// 双重检查
	if globalHexagonMetrics == nil {
		globalHexagonMetrics = NewHexagonMetrics(nil)
	}
	return globalHexagonMetrics
}

// SetHexagonMetrics 设置全局 Hexagon 指标收集器
//
// 线程安全：此方法是并发安全的。
func SetHexagonMetrics(m *HexagonMetrics) {
	globalHexagonMetricsMu.Lock()
	defer globalHexagonMetricsMu.Unlock()
	globalHexagonMetrics = m
}
