// Package cost 提供 Hexagon AI Agent 框架的成本控制
//
// CostController 用于控制 Agent 的资源消耗，包括：
// - Token 使用限制
// - API 调用频率限制
// - 成本预算控制
package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/toolkit/util/rate"
)

// Controller 成本控制器
type Controller struct {
	mu sync.RWMutex

	// 预算相关
	budget    float64 // 总预算（美元）
	used      float64 // 已使用金额
	remaining float64 // 剩余金额

	// Token 相关
	maxTokensPerRequest int64 // 单次请求最大 Token
	maxTokensPerSession int64 // 单次会话最大 Token
	maxTokensTotal      int64 // 总 Token 限制
	usedTokens          int64 // 已使用 Token

	// 速率限制 (使用 toolkit SlidingWindow)
	requestsPerMinute int                  // 每分钟请求数
	rateLimiter       *rate.SlidingWindow  // 滑动窗口限流器

	// 回调
	onBudgetExceeded func(used, budget float64)
	onTokensExceeded func(used, limit int64)
	onRateExceeded   func(requests, limit int)

	// 定价表（每 1000 Token）
	pricing map[string]ModelPricing
}

// ModelPricing 模型定价
type ModelPricing struct {
	PromptPrice     float64 // 输入 Token 价格（每 1000 Token）
	CompletionPrice float64 // 输出 Token 价格（每 1000 Token）
}

// DefaultPricing 默认定价表
var DefaultPricing = map[string]ModelPricing{
	// OpenAI
	"gpt-4":         {PromptPrice: 0.03, CompletionPrice: 0.06},
	"gpt-4-turbo":   {PromptPrice: 0.01, CompletionPrice: 0.03},
	"gpt-4o":        {PromptPrice: 0.005, CompletionPrice: 0.015},
	"gpt-4o-mini":   {PromptPrice: 0.00015, CompletionPrice: 0.0006},
	"gpt-3.5-turbo": {PromptPrice: 0.0005, CompletionPrice: 0.0015},

	// Anthropic
	"claude-3-opus":   {PromptPrice: 0.015, CompletionPrice: 0.075},
	"claude-3-sonnet": {PromptPrice: 0.003, CompletionPrice: 0.015},
	"claude-3-haiku":  {PromptPrice: 0.00025, CompletionPrice: 0.00125},

	// DeepSeek
	"deepseek-chat":     {PromptPrice: 0.00014, CompletionPrice: 0.00028},
	"deepseek-reasoner": {PromptPrice: 0.00055, CompletionPrice: 0.00219},

	// 默认
	"default": {PromptPrice: 0.001, CompletionPrice: 0.002},
}

// ControllerOption 控制器选项
type ControllerOption func(*Controller)

// NewController 创建成本控制器
func NewController(opts ...ControllerOption) *Controller {
	c := &Controller{
		pricing:             DefaultPricing,
		requestsPerMinute:   60,
		maxTokensPerRequest: 8000,
		maxTokensPerSession: 100000,
		maxTokensTotal:      1000000,
	}

	for _, opt := range opts {
		opt(c)
	}

	// 初始化滑动窗口限流器
	c.rateLimiter = rate.NewSlidingWindow(c.requestsPerMinute, time.Minute)

	return c
}

// WithBudget 设置预算
func WithBudget(budget float64) ControllerOption {
	return func(c *Controller) {
		c.budget = budget
		c.remaining = budget
	}
}

// WithMaxTokensPerRequest 设置单次请求最大 Token
func WithMaxTokensPerRequest(tokens int64) ControllerOption {
	return func(c *Controller) {
		c.maxTokensPerRequest = tokens
	}
}

// WithMaxTokensPerSession 设置单次会话最大 Token
func WithMaxTokensPerSession(tokens int64) ControllerOption {
	return func(c *Controller) {
		c.maxTokensPerSession = tokens
	}
}

// WithMaxTokensTotal 设置总 Token 限制
func WithMaxTokensTotal(tokens int64) ControllerOption {
	return func(c *Controller) {
		c.maxTokensTotal = tokens
	}
}

// WithRequestsPerMinute 设置每分钟请求数
func WithRequestsPerMinute(rpm int) ControllerOption {
	return func(c *Controller) {
		c.requestsPerMinute = rpm
	}
}

// WithPricing 设置自定义定价表
func WithPricing(pricing map[string]ModelPricing) ControllerOption {
	return func(c *Controller) {
		for k, v := range pricing {
			c.pricing[k] = v
		}
	}
}

// OnBudgetExceeded 设置预算超限回调
func OnBudgetExceeded(fn func(used, budget float64)) ControllerOption {
	return func(c *Controller) {
		c.onBudgetExceeded = fn
	}
}

// OnTokensExceeded 设置 Token 超限回调
func OnTokensExceeded(fn func(used, limit int64)) ControllerOption {
	return func(c *Controller) {
		c.onTokensExceeded = fn
	}
}

// OnRateExceeded 设置速率超限回调
func OnRateExceeded(fn func(requests, limit int)) ControllerOption {
	return func(c *Controller) {
		c.onRateExceeded = fn
	}
}

// TokenUsage Token 使用量
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CheckRequest 检查是否可以发起请求
func (c *Controller) CheckRequest(ctx context.Context, estimatedTokens int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查单次请求 Token 限制
	if estimatedTokens > c.maxTokensPerRequest {
		return fmt.Errorf("request tokens %d exceeds limit %d", estimatedTokens, c.maxTokensPerRequest)
	}

	// 检查总 Token 限制
	if c.maxTokensTotal > 0 && c.usedTokens+estimatedTokens > c.maxTokensTotal {
		if c.onTokensExceeded != nil {
			c.onTokensExceeded(c.usedTokens, c.maxTokensTotal)
		}
		return fmt.Errorf("total tokens would exceed limit: %d + %d > %d",
			c.usedTokens, estimatedTokens, c.maxTokensTotal)
	}

	// 检查速率限制 (使用 toolkit SlidingWindow)
	allowed, count := c.rateLimiter.TryAllow()
	if !allowed {
		if c.onRateExceeded != nil {
			c.onRateExceeded(count, c.requestsPerMinute)
		}
		return fmt.Errorf("rate limit exceeded: %d requests in last minute (limit: %d)",
			count, c.requestsPerMinute)
	}

	return nil
}

// RecordUsage 记录使用量
func (c *Controller) RecordUsage(model string, usage TokenUsage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 注意：请求已在 CheckRequest 的 TryAllow 中记录，此处不再重复记录

	// 累加 Token
	c.usedTokens += int64(usage.TotalTokens)

	// 计算成本
	pricing, ok := c.pricing[model]
	if !ok {
		pricing = c.pricing["default"]
	}

	cost := (float64(usage.PromptTokens) / 1000 * pricing.PromptPrice) +
		(float64(usage.CompletionTokens) / 1000 * pricing.CompletionPrice)

	c.used += cost
	c.remaining = c.budget - c.used

	// 检查预算
	if c.budget > 0 && c.used > c.budget {
		if c.onBudgetExceeded != nil {
			c.onBudgetExceeded(c.used, c.budget)
		}
		return fmt.Errorf("budget exceeded: $%.4f used of $%.4f budget", c.used, c.budget)
	}

	return nil
}

// Stats 返回统计信息
func (c *Controller) Stats() ControllerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ControllerStats{
		Budget:           c.budget,
		Used:             c.used,
		Remaining:        c.remaining,
		UsedTokens:       c.usedTokens,
		MaxTokensTotal:   c.maxTokensTotal,
		RequestsLastMin:  c.rateLimiter.Count(),
		RequestsPerMin:   c.requestsPerMinute,
	}
}

// ControllerStats 控制器统计
type ControllerStats struct {
	Budget           float64 `json:"budget"`
	Used             float64 `json:"used"`
	Remaining        float64 `json:"remaining"`
	UsedTokens       int64   `json:"used_tokens"`
	MaxTokensTotal   int64   `json:"max_tokens_total"`
	RequestsLastMin  int     `json:"requests_last_min"`
	RequestsPerMin   int     `json:"requests_per_min"`
}

// Reset 重置统计
func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.used = 0
	c.remaining = c.budget
	c.usedTokens = 0
	c.rateLimiter.Reset()
}

// EstimateCost 估算成本
func (c *Controller) EstimateCost(model string, promptTokens, completionTokens int) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pricing, ok := c.pricing[model]
	if !ok {
		pricing = c.pricing["default"]
	}

	return (float64(promptTokens) / 1000 * pricing.PromptPrice) +
		(float64(completionTokens) / 1000 * pricing.CompletionPrice)
}

// RemainingBudget 返回剩余预算
func (c *Controller) RemainingBudget() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.remaining
}

// RemainingTokens 返回剩余 Token
func (c *Controller) RemainingTokens() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxTokensTotal - c.usedTokens
}

// CanAfford 检查是否能负担指定成本
func (c *Controller) CanAfford(estimatedCost float64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.budget <= 0 {
		return true // 无预算限制
	}
	return c.remaining >= estimatedCost
}

// Context key
type controllerKey struct{}

// ContextWithController 将控制器添加到 context
func ContextWithController(ctx context.Context, c *Controller) context.Context {
	return context.WithValue(ctx, controllerKey{}, c)
}

// ControllerFromContext 从 context 获取控制器
func ControllerFromContext(ctx context.Context) *Controller {
	if c, ok := ctx.Value(controllerKey{}).(*Controller); ok {
		return c
	}
	return nil
}

// CheckAndRecord 检查并记录（便捷函数）
func CheckAndRecord(ctx context.Context, model string, usage TokenUsage) error {
	c := ControllerFromContext(ctx)
	if c == nil {
		return nil // 没有控制器，跳过检查
	}

	if err := c.CheckRequest(ctx, int64(usage.TotalTokens)); err != nil {
		return err
	}

	return c.RecordUsage(model, usage)
}
