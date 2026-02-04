// Package advisor 提供 Hexagon 框架的切面/拦截器系统
//
// 本包实现了完整的 AOP（面向切面编程）能力：
//   - 八种切面时机：OnStart, OnEnd, OnError, OnStreamStart, OnStreamEnd, OnStreamChunk, OnRetry, OnTimeout
//   - 三级作用域：Global（全局）, Type（类型）, Node（节点）
//   - 预置切面：Logging, Tracing, Metrics, Retry, Timeout, Cache, RateLimit 等
//
// 设计借鉴：
//   - Semantic Kernel: Filter 机制
//   - Spring AI: Advisor API (CallAdvisor/StreamAdvisor)
//   - Eino: Callbacks 五切面
//   - ASP.NET: Middleware 模式
//
// 使用示例：
//
//	// 创建切面链
//	chain := advisor.NewChain(
//	    advisor.Logging,
//	    advisor.Tracing,
//	    advisor.Retry(3),
//	)
//
//	// 在图编排中使用
//	graph.Compile(ctx,
//	    WithAdvisor(loggingAdvisor, ScopeGlobal),
//	    WithAdvisor(tokenCountAdvisor, ScopeType, TypeFilter("ChatModel")),
//	)
package advisor

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/hexagon/stream"
)

// ============== 切面时机 ==============

// Timing 切面时机
type Timing int

const (
	// === 基础切面 ===
	OnStart Timing = iota // 执行前
	OnEnd                  // 执行后
	OnError                // 出错时

	// === 流式切面 ===
	OnStreamStart // 流开始
	OnStreamEnd   // 流结束
	OnStreamChunk // 每个流块

	// === 扩展切面 ===
	OnRetry   // 重试前
	OnTimeout // 超时时
)

func (t Timing) String() string {
	switch t {
	case OnStart:
		return "OnStart"
	case OnEnd:
		return "OnEnd"
	case OnError:
		return "OnError"
	case OnStreamStart:
		return "OnStreamStart"
	case OnStreamEnd:
		return "OnStreamEnd"
	case OnStreamChunk:
		return "OnStreamChunk"
	case OnRetry:
		return "OnRetry"
	case OnTimeout:
		return "OnTimeout"
	default:
		return "Unknown"
	}
}

// ============== 作用域 ==============

// Scope 切面作用域
type Scope int

const (
	ScopeGlobal Scope = iota // 全局：影响所有组件
	ScopeType                 // 类型：影响特定类型组件
	ScopeNode                 // 节点：只影响特定节点
)

func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "Global"
	case ScopeType:
		return "Type"
	case ScopeNode:
		return "Node"
	default:
		return "Unknown"
	}
}

// ============== 请求/响应类型 ==============

// CallRequest 调用请求
type CallRequest struct {
	RunID       string         // 运行ID
	Component   string         // 组件名称
	ComponentType string       // 组件类型
	NodeID      string         // 节点ID（图中）
	Input       any            // 输入数据
	Options     []any          // 执行选项
	Metadata    map[string]any // 元数据
	IsRetry     bool           // 是否为重试
	RetryCount  int            // 重试次数
	StartTime   time.Time      // 开始时间
}

// CallResponse 调用响应
type CallResponse struct {
	RunID    string         // 运行ID
	Output   any            // 输出数据
	Error    error          // 错误
	Duration time.Duration  // 耗时
	Metadata map[string]any // 元数据
}

// StreamRequest 流式请求
type StreamRequest struct {
	CallRequest
	IsStreaming bool // 是否为流式调用
}

// StreamResponse 流式响应
type StreamResponse struct {
	RunID    string         // 运行ID
	Stream   any            // 流对象
	Error    error          // 错误
	Metadata map[string]any // 元数据
}

// ChunkContext 流块上下文
type ChunkContext struct {
	RunID     string        // 运行ID
	ChunkIdx  int           // 块索引
	Chunk     any           // 块数据
	IsFirst   bool          // 是否第一块
	IsLast    bool          // 是否最后一块
	Elapsed   time.Duration // 已耗时
	Metadata  map[string]any
}

// RetryContext 重试上下文
type RetryContext struct {
	RunID      string        // 运行ID
	Attempt    int           // 当前尝试次数
	MaxRetries int           // 最大重试次数
	LastError  error         // 上次错误
	Delay      time.Duration // 延迟时间
	Metadata   map[string]any
}

// TimeoutContext 超时上下文
type TimeoutContext struct {
	RunID    string        // 运行ID
	Timeout  time.Duration // 超时时间
	Elapsed  time.Duration // 已耗时
	Metadata map[string]any
}

// ============== Advisor 接口 ==============

// Advisor 切面接口
type Advisor interface {
	// Name 切面名称
	Name() string

	// Order 执行顺序（数值越小越先执行）
	Order() int

	// AdviseCall 非流式切面
	AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error)

	// AdviseStream 流式切面
	AdviseStream(ctx context.Context, req *StreamRequest, next StreamHandler) (*StreamResponse, error)
}

// CallHandler 调用处理器
type CallHandler func(ctx context.Context, req *CallRequest) (*CallResponse, error)

// StreamHandler 流处理器
type StreamHandler func(ctx context.Context, req *StreamRequest) (*StreamResponse, error)

// ============== 扩展 Advisor 接口 ==============

// ChunkAdvisor 流块切面（可选实现）
type ChunkAdvisor interface {
	Advisor
	// OnChunk 处理每个流块
	OnChunk(ctx context.Context, chunk *ChunkContext) error
}

// RetryAdvisor 重试切面（可选实现）
type RetryAdvisor interface {
	Advisor
	// OnRetry 重试前回调
	OnRetry(ctx context.Context, retry *RetryContext) error
}

// TimeoutAdvisor 超时切面（可选实现）
type TimeoutAdvisor interface {
	Advisor
	// OnTimeout 超时回调
	OnTimeout(ctx context.Context, timeout *TimeoutContext) error
}

// ============== BaseAdvisor 基础实现 ==============

// BaseAdvisor 提供 Advisor 接口的基础实现
type BaseAdvisor struct {
	name  string
	order int

	onStart       func(ctx context.Context, req *CallRequest) context.Context
	onEnd         func(ctx context.Context, req *CallRequest, resp *CallResponse)
	onError       func(ctx context.Context, req *CallRequest, err error)
	onStreamStart func(ctx context.Context, req *StreamRequest) context.Context
	onStreamEnd   func(ctx context.Context, req *StreamRequest, resp *StreamResponse)
	onStreamChunk func(ctx context.Context, chunk *ChunkContext) error
	onRetry       func(ctx context.Context, retry *RetryContext) error
	onTimeout     func(ctx context.Context, timeout *TimeoutContext) error
}

// NewAdvisor 创建基础切面
func NewAdvisor(name string, order int) *BaseAdvisor {
	return &BaseAdvisor{
		name:  name,
		order: order,
	}
}

func (a *BaseAdvisor) Name() string { return a.name }
func (a *BaseAdvisor) Order() int   { return a.order }

// AdviseCall 非流式切面
func (a *BaseAdvisor) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	// OnStart
	if a.onStart != nil {
		ctx = a.onStart(ctx, req)
	}

	// 执行
	resp, err := next(ctx, req)

	// OnError
	if err != nil && a.onError != nil {
		a.onError(ctx, req, err)
	}

	// OnEnd
	if a.onEnd != nil {
		a.onEnd(ctx, req, resp)
	}

	return resp, err
}

// AdviseStream 流式切面
func (a *BaseAdvisor) AdviseStream(ctx context.Context, req *StreamRequest, next StreamHandler) (*StreamResponse, error) {
	// OnStreamStart
	if a.onStreamStart != nil {
		ctx = a.onStreamStart(ctx, req)
	}

	// 执行
	resp, err := next(ctx, req)

	// OnError
	if err != nil && a.onError != nil {
		a.onError(ctx, &req.CallRequest, err)
	}

	// OnStreamEnd
	if a.onStreamEnd != nil {
		a.onStreamEnd(ctx, req, resp)
	}

	return resp, err
}

// OnChunk 处理流块
func (a *BaseAdvisor) OnChunk(ctx context.Context, chunk *ChunkContext) error {
	if a.onStreamChunk != nil {
		return a.onStreamChunk(ctx, chunk)
	}
	return nil
}

// OnRetry 重试回调
func (a *BaseAdvisor) OnRetry(ctx context.Context, retry *RetryContext) error {
	if a.onRetry != nil {
		return a.onRetry(ctx, retry)
	}
	return nil
}

// OnTimeout 超时回调
func (a *BaseAdvisor) OnTimeout(ctx context.Context, timeout *TimeoutContext) error {
	if a.onTimeout != nil {
		return a.onTimeout(ctx, timeout)
	}
	return nil
}

// === Builder 方法 ===

func (a *BaseAdvisor) WithOnStart(fn func(ctx context.Context, req *CallRequest) context.Context) *BaseAdvisor {
	a.onStart = fn
	return a
}

func (a *BaseAdvisor) WithOnEnd(fn func(ctx context.Context, req *CallRequest, resp *CallResponse)) *BaseAdvisor {
	a.onEnd = fn
	return a
}

func (a *BaseAdvisor) WithOnError(fn func(ctx context.Context, req *CallRequest, err error)) *BaseAdvisor {
	a.onError = fn
	return a
}

func (a *BaseAdvisor) WithOnStreamStart(fn func(ctx context.Context, req *StreamRequest) context.Context) *BaseAdvisor {
	a.onStreamStart = fn
	return a
}

func (a *BaseAdvisor) WithOnStreamEnd(fn func(ctx context.Context, req *StreamRequest, resp *StreamResponse)) *BaseAdvisor {
	a.onStreamEnd = fn
	return a
}

func (a *BaseAdvisor) WithOnStreamChunk(fn func(ctx context.Context, chunk *ChunkContext) error) *BaseAdvisor {
	a.onStreamChunk = fn
	return a
}

func (a *BaseAdvisor) WithOnRetry(fn func(ctx context.Context, retry *RetryContext) error) *BaseAdvisor {
	a.onRetry = fn
	return a
}

func (a *BaseAdvisor) WithOnTimeout(fn func(ctx context.Context, timeout *TimeoutContext) error) *BaseAdvisor {
	a.onTimeout = fn
	return a
}

// ============== AdvisorChain 切面链 ==============

// Chain 切面链
type Chain struct {
	advisors []Advisor
	mu       sync.RWMutex
}

// NewChain 创建切面链
func NewChain(advisors ...Advisor) *Chain {
	c := &Chain{
		advisors: make([]Advisor, 0, len(advisors)),
	}
	for _, a := range advisors {
		c.Add(a)
	}
	return c
}

// Add 添加切面
func (c *Chain) Add(advisor Advisor) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advisors = append(c.advisors, advisor)
	// 按 Order 排序
	sort.Slice(c.advisors, func(i, j int) bool {
		return c.advisors[i].Order() < c.advisors[j].Order()
	})
	return c
}

// ExecuteCall 执行非流式调用
func (c *Chain) ExecuteCall(ctx context.Context, req *CallRequest, final CallHandler) (*CallResponse, error) {
	c.mu.RLock()
	advisors := make([]Advisor, len(c.advisors))
	copy(advisors, c.advisors)
	c.mu.RUnlock()

	// 构建调用链
	handler := final
	for i := len(advisors) - 1; i >= 0; i-- {
		a := advisors[i]
		next := handler
		handler = func(ctx context.Context, req *CallRequest) (*CallResponse, error) {
			return a.AdviseCall(ctx, req, next)
		}
	}

	return handler(ctx, req)
}

// ExecuteStream 执行流式调用
func (c *Chain) ExecuteStream(ctx context.Context, req *StreamRequest, final StreamHandler) (*StreamResponse, error) {
	c.mu.RLock()
	advisors := make([]Advisor, len(c.advisors))
	copy(advisors, c.advisors)
	c.mu.RUnlock()

	// 构建调用链
	handler := final
	for i := len(advisors) - 1; i >= 0; i-- {
		a := advisors[i]
		next := handler
		handler = func(ctx context.Context, req *StreamRequest) (*StreamResponse, error) {
			return a.AdviseStream(ctx, req, next)
		}
	}

	return handler(ctx, req)
}

// ============== ScopedAdvisor 作用域切面 ==============

// ScopedAdvisor 带作用域的切面
type ScopedAdvisor struct {
	Advisor    Advisor
	Scope      Scope
	TypeFilter func(componentType string) bool
	NodeFilter func(nodeID string) bool
}

// NewScopedAdvisor 创建作用域切面
func NewScopedAdvisor(advisor Advisor, scope Scope, opts ...ScopeOption) *ScopedAdvisor {
	sa := &ScopedAdvisor{
		Advisor: advisor,
		Scope:   scope,
	}
	for _, opt := range opts {
		opt(sa)
	}
	return sa
}

// ScopeOption 作用域选项
type ScopeOption func(*ScopedAdvisor)

// WithTypeFilter 设置类型过滤
func WithTypeFilter(filter func(componentType string) bool) ScopeOption {
	return func(sa *ScopedAdvisor) {
		sa.TypeFilter = filter
	}
}

// WithNodeFilter 设置节点过滤
func WithNodeFilter(filter func(nodeID string) bool) ScopeOption {
	return func(sa *ScopedAdvisor) {
		sa.NodeFilter = filter
	}
}

// TypeFilterExact 精确类型匹配
func TypeFilterExact(types ...string) func(string) bool {
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	return func(componentType string) bool {
		return typeSet[componentType]
	}
}

// NodeFilterExact 精确节点匹配
func NodeFilterExact(nodeIDs ...string) func(string) bool {
	nodeSet := make(map[string]bool)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}
	return func(nodeID string) bool {
		return nodeSet[nodeID]
	}
}

// Matches 检查是否匹配
func (sa *ScopedAdvisor) Matches(componentType, nodeID string) bool {
	switch sa.Scope {
	case ScopeGlobal:
		return true
	case ScopeType:
		if sa.TypeFilter != nil {
			return sa.TypeFilter(componentType)
		}
		return true
	case ScopeNode:
		if sa.NodeFilter != nil {
			return sa.NodeFilter(nodeID)
		}
		return true
	default:
		return false
	}
}

// ============== Manager 切面管理器 ==============

// Manager 切面管理器
type Manager struct {
	globalAdvisors []*ScopedAdvisor
	typeAdvisors   []*ScopedAdvisor
	nodeAdvisors   []*ScopedAdvisor
	mu             sync.RWMutex
}

// NewManager 创建切面管理器
func NewManager() *Manager {
	return &Manager{
		globalAdvisors: make([]*ScopedAdvisor, 0),
		typeAdvisors:   make([]*ScopedAdvisor, 0),
		nodeAdvisors:   make([]*ScopedAdvisor, 0),
	}
}

// Register 注册切面
func (m *Manager) Register(advisor Advisor, scope Scope, opts ...ScopeOption) {
	sa := NewScopedAdvisor(advisor, scope, opts...)
	m.mu.Lock()
	defer m.mu.Unlock()

	switch scope {
	case ScopeGlobal:
		m.globalAdvisors = append(m.globalAdvisors, sa)
	case ScopeType:
		m.typeAdvisors = append(m.typeAdvisors, sa)
	case ScopeNode:
		m.nodeAdvisors = append(m.nodeAdvisors, sa)
	}
}

// GetChain 获取指定组件的切面链
func (m *Manager) GetChain(componentType, nodeID string) *Chain {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chain := NewChain()

	// 添加全局切面
	for _, sa := range m.globalAdvisors {
		chain.Add(sa.Advisor)
	}

	// 添加类型切面
	for _, sa := range m.typeAdvisors {
		if sa.Matches(componentType, nodeID) {
			chain.Add(sa.Advisor)
		}
	}

	// 添加节点切面
	for _, sa := range m.nodeAdvisors {
		if sa.Matches(componentType, nodeID) {
			chain.Add(sa.Advisor)
		}
	}

	return chain
}

// ============== Context Helpers ==============

type advisorManagerKey struct{}

// ContextWithManager 将切面管理器添加到 context
func ContextWithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, advisorManagerKey{}, m)
}

// ManagerFromContext 从 context 获取切面管理器
func ManagerFromContext(ctx context.Context) *Manager {
	if m, ok := ctx.Value(advisorManagerKey{}).(*Manager); ok {
		return m
	}
	return nil
}

// ============== 预置切面 ==============

// Logging 日志切面
var Logging = NewAdvisor("logging", 0).
	WithOnStart(func(ctx context.Context, req *CallRequest) context.Context {
		log.Printf("[%s] %s.%s started, input: %v", req.RunID, req.ComponentType, req.Component, req.Input)
		return ctx
	}).
	WithOnEnd(func(ctx context.Context, req *CallRequest, resp *CallResponse) {
		log.Printf("[%s] %s.%s completed in %v", req.RunID, req.ComponentType, req.Component, resp.Duration)
	}).
	WithOnError(func(ctx context.Context, req *CallRequest, err error) {
		log.Printf("[%s] %s.%s error: %v", req.RunID, req.ComponentType, req.Component, err)
	})

// MetricsAdvisor 指标切面
type metricsAdvisor struct {
	*BaseAdvisor
	callCount    int64
	errorCount   int64
	totalLatency int64
}

// NewMetricsAdvisor 创建指标切面
func NewMetricsAdvisor() *metricsAdvisor {
	a := &metricsAdvisor{
		BaseAdvisor: NewAdvisor("metrics", 1),
	}
	a.WithOnEnd(func(ctx context.Context, req *CallRequest, resp *CallResponse) {
		atomic.AddInt64(&a.callCount, 1)
		atomic.AddInt64(&a.totalLatency, int64(resp.Duration))
	})
	a.WithOnError(func(ctx context.Context, req *CallRequest, err error) {
		atomic.AddInt64(&a.errorCount, 1)
	})
	return a
}

func (a *metricsAdvisor) CallCount() int64 {
	return atomic.LoadInt64(&a.callCount)
}

func (a *metricsAdvisor) ErrorCount() int64 {
	return atomic.LoadInt64(&a.errorCount)
}

func (a *metricsAdvisor) AvgLatency() time.Duration {
	count := atomic.LoadInt64(&a.callCount)
	if count == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&a.totalLatency) / count)
}

// Metrics 指标切面实例
var Metrics = NewMetricsAdvisor()

// RetryAdvisorImpl 重试切面
type retryAdvisorImpl struct {
	*BaseAdvisor
	maxRetries int
	backoff    time.Duration
}

// NewRetryAdvisor 创建重试切面
func NewRetryAdvisor(maxRetries int, backoff time.Duration) Advisor {
	a := &retryAdvisorImpl{
		BaseAdvisor: NewAdvisor("retry", 10),
		maxRetries:  maxRetries,
		backoff:     backoff,
	}
	return a
}

func (a *retryAdvisorImpl) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		if attempt > 0 {
			req.IsRetry = true
			req.RetryCount = attempt
			time.Sleep(a.backoff * time.Duration(attempt))
		}

		resp, err := next(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Retry 创建重试切面
func Retry(maxRetries int) Advisor {
	return NewRetryAdvisor(maxRetries, time.Second)
}

// TimeoutAdvisorImpl 超时切面
type timeoutAdvisorImpl struct {
	*BaseAdvisor
	timeout time.Duration
}

// NewTimeoutAdvisor 创建超时切面
func NewTimeoutAdvisor(timeout time.Duration) Advisor {
	return &timeoutAdvisorImpl{
		BaseAdvisor: NewAdvisor("timeout", 5),
		timeout:     timeout,
	}
}

func (a *timeoutAdvisorImpl) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return next(ctx, req)
}

// Timeout 创建超时切面
func Timeout(d time.Duration) Advisor {
	return NewTimeoutAdvisor(d)
}

// CacheAdvisorImpl 缓存切面
type cacheAdvisorImpl struct {
	*BaseAdvisor
	cache sync.Map
	ttl   time.Duration
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewCacheAdvisor 创建缓存切面
func NewCacheAdvisor(ttl time.Duration) *cacheAdvisorImpl {
	return &cacheAdvisorImpl{
		BaseAdvisor: NewAdvisor("cache", 2),
		ttl:         ttl,
	}
}

func (a *cacheAdvisorImpl) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	key := fmt.Sprintf("%s:%s:%v", req.Component, req.NodeID, req.Input)

	// 检查缓存
	if entry, ok := a.cache.Load(key); ok {
		ce := entry.(*cacheEntry)
		if time.Now().Before(ce.expiresAt) {
			return &CallResponse{
				RunID:  req.RunID,
				Output: ce.value,
				Metadata: map[string]any{
					"cache_hit": true,
				},
			}, nil
		}
	}

	// 执行并缓存
	resp, err := next(ctx, req)
	if err == nil && resp != nil {
		a.cache.Store(key, &cacheEntry{
			value:     resp.Output,
			expiresAt: time.Now().Add(a.ttl),
		})
	}

	return resp, err
}

// Cache 创建缓存切面
func Cache(ttl time.Duration) Advisor {
	return NewCacheAdvisor(ttl)
}

// RateLimitAdvisorImpl 限流切面
type rateLimitAdvisorImpl struct {
	*BaseAdvisor
	rate   int           // 每秒请求数
	tokens chan struct{}
}

// NewRateLimitAdvisor 创建限流切面
func NewRateLimitAdvisor(ratePerSecond int) *rateLimitAdvisorImpl {
	a := &rateLimitAdvisorImpl{
		BaseAdvisor: NewAdvisor("ratelimit", 3),
		rate:        ratePerSecond,
		tokens:      make(chan struct{}, ratePerSecond),
	}

	// 填充令牌
	for i := 0; i < ratePerSecond; i++ {
		a.tokens <- struct{}{}
	}

	// 定期补充令牌
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(ratePerSecond))
		defer ticker.Stop()
		for range ticker.C {
			select {
			case a.tokens <- struct{}{}:
			default:
			}
		}
	}()

	return a
}

func (a *rateLimitAdvisorImpl) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	select {
	case <-a.tokens:
		return next(ctx, req)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// RateLimit 创建限流切面
func RateLimit(ratePerSecond int) Advisor {
	return NewRateLimitAdvisor(ratePerSecond)
}

// TokenBudgetAdvisorImpl Token 预算切面
type tokenBudgetAdvisorImpl struct {
	*BaseAdvisor
	budget    int64
	used      int64
	onExceed  func(ctx context.Context, used, budget int64)
}

// NewTokenBudgetAdvisor 创建 Token 预算切面
func NewTokenBudgetAdvisor(budget int64) *tokenBudgetAdvisorImpl {
	return &tokenBudgetAdvisorImpl{
		BaseAdvisor: NewAdvisor("token_budget", 4),
		budget:      budget,
	}
}

func (a *tokenBudgetAdvisorImpl) AdviseCall(ctx context.Context, req *CallRequest, next CallHandler) (*CallResponse, error) {
	used := atomic.LoadInt64(&a.used)
	if used >= a.budget {
		if a.onExceed != nil {
			a.onExceed(ctx, used, a.budget)
		}
		return nil, fmt.Errorf("token budget exceeded: %d/%d", used, a.budget)
	}

	resp, err := next(ctx, req)

	// 从 metadata 中获取 token 使用量
	if resp != nil && resp.Metadata != nil {
		if tokens, ok := resp.Metadata["total_tokens"].(int); ok {
			atomic.AddInt64(&a.used, int64(tokens))
		}
	}

	return resp, err
}

func (a *tokenBudgetAdvisorImpl) WithOnExceed(fn func(ctx context.Context, used, budget int64)) *tokenBudgetAdvisorImpl {
	a.onExceed = fn
	return a
}

func (a *tokenBudgetAdvisorImpl) Used() int64 {
	return atomic.LoadInt64(&a.used)
}

func (a *tokenBudgetAdvisorImpl) Remaining() int64 {
	return a.budget - atomic.LoadInt64(&a.used)
}

// TokenBudget 创建 Token 预算切面
func TokenBudget(budget int64) *tokenBudgetAdvisorImpl {
	return NewTokenBudgetAdvisor(budget)
}

// ============== 流式切面包装 ==============

// WrapStreamWithChunk 包装流以支持 OnStreamChunk 切面
func WrapStreamWithChunk[T any](sr *stream.StreamReader[T], advisors []Advisor, runID string) *stream.StreamReader[T] {
	reader, writer := stream.Pipe[T](10)

	go func() {
		defer writer.Close()
		idx := 0
		startTime := time.Now()

		for {
			item, err := sr.Recv()
			if err != nil {
				return
			}

			// 调用 OnStreamChunk
			chunk := &ChunkContext{
				RunID:    runID,
				ChunkIdx: idx,
				Chunk:    item,
				IsFirst:  idx == 0,
				IsLast:   false, // 无法预知
				Elapsed:  time.Since(startTime),
			}

			for _, a := range advisors {
				if ca, ok := a.(ChunkAdvisor); ok {
					if err := ca.OnChunk(context.Background(), chunk); err != nil {
						writer.CloseWithError(err)
						return
					}
				}
			}

			writer.Send(item)
			idx++
		}
	}()

	return reader
}
