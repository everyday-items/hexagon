// Package router 提供 LLM 智能路由功能
//
// 本包实现了多种 LLM Provider 路由策略：
//   - 优先级路由：按 Provider 优先级选择
//   - 成本路由：选择最低成本的 Provider
//   - 轮询路由：均衡负载
//   - 降级路由：失败自动切换下一个
//   - 复杂度路由：根据查询复杂度选择
//
// 使用示例：
//
//	r := router.New(configs, router.WithStrategy(router.StrategyCost))
//	resp, err := r.Complete(ctx, req)
package router

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/everyday-items/ai-core/llm"
)

// ============== 错误定义 ==============

var (
	// ErrNoProvider 没有可用的 Provider
	ErrNoProvider = errors.New("router: 没有可用的 LLM Provider")

	// ErrAllFailed 所有 Provider 均失败
	ErrAllFailed = errors.New("router: 所有 Provider 均失败")
)

// ============== 路由策略 ==============

// Strategy 路由策略类型
type Strategy int

const (
	// StrategyPriority 按优先级路由（优先级数值越小越优先）
	StrategyPriority Strategy = iota

	// StrategyCost 按成本路由（选择 CostPerToken 最低的）
	StrategyCost

	// StrategyRoundRobin 轮询路由（均衡负载）
	StrategyRoundRobin

	// StrategyFallback 降级路由（依次尝试，失败切换下一个）
	StrategyFallback

	// StrategyComplexity 复杂度路由（根据查询长度/复杂度选择）
	StrategyComplexity
)

// ============== Provider 配置 ==============

// ProviderConfig 描述一个 LLM Provider 及其属性
type ProviderConfig struct {
	// Provider LLM Provider 实例
	Provider llm.Provider

	// Name 名称标识（若为空则使用 Provider.Name()）
	Name string

	// Priority 优先级，数值越小越优先
	Priority int

	// CostPerToken 每 Token 费用（用于成本路由）
	CostPerToken float64

	// MaxTokens 该 Provider 支持的最大 Token 数
	MaxTokens int

	// Models 该 Provider 支持的模型列表
	Models []string

	// Weight 权重（用于加权轮询，暂保留）
	Weight float64

	// Enabled 是否启用
	Enabled bool

	// Tier 复杂度层级（0=简单 1=中等 2=复杂）
	Tier int
}

// name 返回 Provider 标识名
func (c *ProviderConfig) name() string {
	if c.Name != "" {
		return c.Name
	}
	if c.Provider != nil {
		return c.Provider.Name()
	}
	return "unknown"
}

// supportsModel 检查是否支持指定模型
func (c *ProviderConfig) supportsModel(model string) bool {
	if len(c.Models) == 0 {
		return true // 未指定模型列表则认为支持所有
	}
	for _, m := range c.Models {
		if m == model {
			return true
		}
	}
	return false
}

// ============== 路由器 ==============

// Router LLM 智能路由器
//
// 根据指定策略在多个 LLM Provider 之间选择最合适的一个。
// 线程安全，可在多 goroutine 环境中使用。
type Router struct {
	configs  []ProviderConfig
	strategy Strategy
	fallback bool // 路由失败后是否尝试降级
	mu       sync.RWMutex
	rrIndex  atomic.Uint64
}

// Option 路由器配置选项
type Option func(*Router)

// WithStrategy 设置路由策略
func WithStrategy(s Strategy) Option {
	return func(r *Router) {
		r.strategy = s
	}
}

// WithFallback 设置是否启用降级（非 StrategyFallback 策略时也可开启）
func WithFallback(enabled bool) Option {
	return func(r *Router) {
		r.fallback = enabled
	}
}

// New 创建路由器
//
// configs 为可用的 Provider 配置列表，至少需要一个已启用的 Provider。
func New(configs []ProviderConfig, opts ...Option) *Router {
	cfgCopy := make([]ProviderConfig, len(configs))
	copy(cfgCopy, configs)
	r := &Router{
		configs:  cfgCopy,
		strategy: StrategyPriority,
		fallback: false, // 默认关闭降级，仅 StrategyFallback 走降级路径
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route 根据策略选择一个 Provider
func (r *Router) Route(ctx context.Context, req llm.CompletionRequest) (llm.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	enabled := r.enabledConfigs(req.Model)
	if len(enabled) == 0 {
		return nil, ErrNoProvider
	}

	switch r.strategy {
	case StrategyCost:
		return r.routeByCost(enabled), nil
	case StrategyRoundRobin:
		return r.routeByRoundRobin(enabled), nil
	case StrategyComplexity:
		return r.routeByComplexity(enabled, req), nil
	case StrategyFallback:
		return enabled[0].Provider, nil // Fallback 由 Complete 控制
	default: // StrategyPriority
		return r.routeByPriority(enabled), nil
	}
}

// Complete 路由并执行 LLM 调用
//
// 若策略为 StrategyFallback 或开启了 fallback，会在失败时自动尝试下一个 Provider。
func (r *Router) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	r.mu.RLock()
	enabled := r.enabledConfigs(req.Model)
	r.mu.RUnlock()

	if len(enabled) == 0 {
		return nil, ErrNoProvider
	}

	// 降级模式：依次尝试每个 Provider
	if r.strategy == StrategyFallback || r.fallback {
		var lastErr error
		for _, cfg := range enabled {
			resp, err := cfg.Provider.Complete(ctx, req)
			if err == nil {
				return resp, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("%w: %v", ErrAllFailed, lastErr)
	}

	// 非降级模式：选择一个 Provider 执行
	provider, err := r.Route(ctx, req)
	if err != nil {
		return nil, err
	}
	return provider.Complete(ctx, req)
}

// Stream 路由并执行流式调用
func (r *Router) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	r.mu.RLock()
	enabled := r.enabledConfigs(req.Model)
	r.mu.RUnlock()

	if len(enabled) == 0 {
		return nil, ErrNoProvider
	}

	if r.strategy == StrategyFallback || r.fallback {
		var lastErr error
		for _, cfg := range enabled {
			s, err := cfg.Provider.Stream(ctx, req)
			if err == nil {
				return s, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("%w: %v", ErrAllFailed, lastErr)
	}

	provider, err := r.Route(ctx, req)
	if err != nil {
		return nil, err
	}
	return provider.Stream(ctx, req)
}

// AddProvider 动态添加 Provider
func (r *Router) AddProvider(cfg ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs = append(r.configs, cfg)
}

// RemoveProvider 按名称移除 Provider
func (r *Router) RemoveProvider(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cfg := range r.configs {
		if cfg.name() == name {
			r.configs = append(r.configs[:i], r.configs[i+1:]...)
			return
		}
	}
}

// ============== 内部路由方法 ==============

// enabledConfigs 返回已启用且支持指定模型的配置
func (r *Router) enabledConfigs(model string) []ProviderConfig {
	var result []ProviderConfig
	for _, cfg := range r.configs {
		if !cfg.Enabled {
			continue
		}
		if model != "" && !cfg.supportsModel(model) {
			continue
		}
		result = append(result, cfg)
	}
	return result
}

// routeByPriority 按优先级选择（数值最小的）
func (r *Router) routeByPriority(configs []ProviderConfig) llm.Provider {
	sorted := make([]ProviderConfig, len(configs))
	copy(sorted, configs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted[0].Provider
}

// routeByCost 按成本选择（CostPerToken 最低的）
func (r *Router) routeByCost(configs []ProviderConfig) llm.Provider {
	best := configs[0]
	for _, cfg := range configs[1:] {
		if cfg.CostPerToken < best.CostPerToken {
			best = cfg
		}
	}
	return best.Provider
}

// routeByRoundRobin 轮询选择
func (r *Router) routeByRoundRobin(configs []ProviderConfig) llm.Provider {
	idx := r.rrIndex.Add(1) - 1
	return configs[idx%uint64(len(configs))].Provider
}

// routeByComplexity 按查询复杂度选择
//
// 综合评估消息数量、总长度、是否包含代码、MaxTokens 要求等因素，
// 生成 0-100 的复杂度评分，映射到 Tier (0=简单, 1=中等, 2=复杂)。
func (r *Router) routeByComplexity(configs []ProviderConfig, req llm.CompletionRequest) llm.Provider {
	score := estimateComplexity(req)

	// 评分映射到 Tier: 0-33 简单, 34-66 中等, 67-100 复杂
	var targetTier int
	switch {
	case score > 66:
		targetTier = 2
	case score > 33:
		targetTier = 1
	default:
		targetTier = 0
	}

	// 查找匹配 Tier 的 Provider
	for _, cfg := range configs {
		if cfg.Tier == targetTier {
			return cfg.Provider
		}
	}
	// 未找到匹配的，选择 Tier 最接近的
	best := configs[0]
	bestDiff := best.Tier - targetTier
	if bestDiff < 0 {
		bestDiff = -bestDiff
	}
	for _, cfg := range configs[1:] {
		diff := cfg.Tier - targetTier
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best = cfg
			bestDiff = diff
		}
	}
	return best.Provider
}

// EstimateComplexity 估算查询复杂度
//
// 返回 0-100 的分数，考虑以下因素：
//   - 消息数量（每条加 5 分，上限 30）
//   - 总长度（每 100 字符加 5 分，上限 40）
//   - 是否包含代码（+20 分）
//   - MaxTokens 要求较高（+10 分）
func EstimateComplexity(req llm.CompletionRequest) int {
	return estimateComplexity(req)
}

func estimateComplexity(req llm.CompletionRequest) int {
	score := 0

	// 消息数量
	msgScore := len(req.Messages) * 5
	if msgScore > 30 {
		msgScore = 30
	}
	score += msgScore

	// 总长度和代码检测
	totalLen := 0
	hasCode := false
	for _, msg := range req.Messages {
		totalLen += len(msg.Content)
		if strings.Contains(msg.Content, "```") || strings.Contains(msg.Content, "func ") {
			hasCode = true
		}
	}

	lenScore := (totalLen / 100) * 5
	if lenScore > 40 {
		lenScore = 40
	}
	score += lenScore

	if hasCode {
		score += 20
	}

	if req.MaxTokens > 2000 {
		score += 10
	}

	if score > 100 {
		score = 100
	}
	return score
}
