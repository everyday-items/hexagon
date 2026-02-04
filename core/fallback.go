// Package core 提供 Hexagon 框架的核心接口和类型
//
// 本文件实现 WithFallback 机制：
//   - Fallback: 降级处理
//   - Retry: 重试机制
//   - CircuitBreaker: 熔断器
//   - RunnableWithFallback: 带降级的 Runnable
//
// 设计借鉴：
//   - LangChain: Runnable.with_fallbacks()
//   - Resilience4j: 弹性模式
//   - Polly: 弹性和瞬态故障处理
package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ============== 错误定义 ==============

var (
	// ErrAllFallbacksFailed 所有降级都失败
	ErrAllFallbacksFailed = errors.New("all fallbacks failed")

	// ErrCircuitOpen 熔断器打开
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrMaxRetriesExceeded 超过最大重试次数
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// ============== Fallback 接口 ==============

// FallbackOption Fallback 选项
type FallbackOption func(*FallbackConfig)

// FallbackConfig Fallback 配置
type FallbackConfig struct {
	// ExceptionsToHandle 要处理的异常类型
	ExceptionsToHandle []error

	// OnFallback 降级回调
	OnFallback func(err error, fallbackIndex int)
}

// WithExceptions 设置要处理的异常类型
func WithExceptions(errs ...error) FallbackOption {
	return func(c *FallbackConfig) {
		c.ExceptionsToHandle = errs
	}
}

// WithFallbackCallback 设置降级回调
func WithFallbackCallback(fn func(err error, fallbackIndex int)) FallbackOption {
	return func(c *FallbackConfig) {
		c.OnFallback = fn
	}
}

// ============== RunnableWithFallback ==============

// RunnableWithFallback 带降级的 Runnable
type RunnableWithFallback[I, O any] struct {
	primary   Runnable[I, O]
	fallbacks []Runnable[I, O]
	config    *FallbackConfig
}

// WithFallback 创建带降级的 Runnable
//
// 示例:
//
//	runnable := core.WithFallback(
//	    primaryRunnable,
//	    fallbackRunnable1,
//	    fallbackRunnable2,
//	)
//	result, err := runnable.Invoke(ctx, input)
func WithFallback[I, O any](primary Runnable[I, O], fallbacks ...Runnable[I, O]) *RunnableWithFallback[I, O] {
	return &RunnableWithFallback[I, O]{
		primary:   primary,
		fallbacks: fallbacks,
		config:    &FallbackConfig{},
	}
}

// WithOptions 设置选项
func (r *RunnableWithFallback[I, O]) WithOptions(opts ...FallbackOption) *RunnableWithFallback[I, O] {
	for _, opt := range opts {
		opt(r.config)
	}
	return r
}

// Name 返回名称
func (r *RunnableWithFallback[I, O]) Name() string {
	return r.primary.Name() + "_with_fallback"
}

// Description 返回描述
func (r *RunnableWithFallback[I, O]) Description() string {
	return r.primary.Description()
}

// InputSchema 返回输入 Schema
func (r *RunnableWithFallback[I, O]) InputSchema() *Schema {
	return r.primary.InputSchema()
}

// OutputSchema 返回输出 Schema
func (r *RunnableWithFallback[I, O]) OutputSchema() *Schema {
	return r.primary.OutputSchema()
}

// Invoke 执行（带降级）
func (r *RunnableWithFallback[I, O]) Invoke(ctx context.Context, input I, opts ...Option) (O, error) {
	// 先尝试主 Runnable
	result, err := r.primary.Invoke(ctx, input, opts...)
	if err == nil {
		return result, nil
	}

	// 检查是否应该降级
	if !r.shouldFallback(err) {
		return result, err
	}

	// 尝试降级 Runnables
	for i, fallback := range r.fallbacks {
		if r.config.OnFallback != nil {
			r.config.OnFallback(err, i)
		}

		result, err = fallback.Invoke(ctx, input, opts...)
		if err == nil {
			return result, nil
		}

		if !r.shouldFallback(err) {
			return result, err
		}
	}

	var zero O
	return zero, ErrAllFallbacksFailed
}

// Stream 流式执行（带降级）
func (r *RunnableWithFallback[I, O]) Stream(ctx context.Context, input I, opts ...Option) (*StreamReader[O], error) {
	stream, err := r.primary.Stream(ctx, input, opts...)
	if err == nil {
		return stream, nil
	}

	if !r.shouldFallback(err) {
		return nil, err
	}

	for i, fallback := range r.fallbacks {
		if r.config.OnFallback != nil {
			r.config.OnFallback(err, i)
		}

		stream, err = fallback.Stream(ctx, input, opts...)
		if err == nil {
			return stream, nil
		}

		if !r.shouldFallback(err) {
			return nil, err
		}
	}

	return nil, ErrAllFallbacksFailed
}

// Batch 批量执行（带降级）
func (r *RunnableWithFallback[I, O]) Batch(ctx context.Context, inputs []I, opts ...Option) ([]O, error) {
	results, err := r.primary.Batch(ctx, inputs, opts...)
	if err == nil {
		return results, nil
	}

	if !r.shouldFallback(err) {
		return nil, err
	}

	for i, fallback := range r.fallbacks {
		if r.config.OnFallback != nil {
			r.config.OnFallback(err, i)
		}

		results, err = fallback.Batch(ctx, inputs, opts...)
		if err == nil {
			return results, nil
		}

		if !r.shouldFallback(err) {
			return nil, err
		}
	}

	return nil, ErrAllFallbacksFailed
}

// Collect 流收集（带降级）
func (r *RunnableWithFallback[I, O]) Collect(ctx context.Context, input *StreamReader[I], opts ...Option) (O, error) {
	return r.primary.Collect(ctx, input, opts...)
}

// Transform 流转换（带降级）
func (r *RunnableWithFallback[I, O]) Transform(ctx context.Context, input *StreamReader[I], opts ...Option) (*StreamReader[O], error) {
	return r.primary.Transform(ctx, input, opts...)
}

// BatchStream 批量流式（带降级）
func (r *RunnableWithFallback[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...Option) (*StreamReader[O], error) {
	stream, err := r.primary.BatchStream(ctx, inputs, opts...)
	if err == nil {
		return stream, nil
	}

	if !r.shouldFallback(err) {
		return nil, err
	}

	for i, fallback := range r.fallbacks {
		if r.config.OnFallback != nil {
			r.config.OnFallback(err, i)
		}

		stream, err = fallback.BatchStream(ctx, inputs, opts...)
		if err == nil {
			return stream, nil
		}

		if !r.shouldFallback(err) {
			return nil, err
		}
	}

	return nil, ErrAllFallbacksFailed
}

func (r *RunnableWithFallback[I, O]) shouldFallback(err error) bool {
	if len(r.config.ExceptionsToHandle) == 0 {
		return true
	}

	for _, e := range r.config.ExceptionsToHandle {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// ============== Retry Runnable ==============

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// InitialDelay 初始延迟
	InitialDelay time.Duration

	// MaxDelay 最大延迟
	MaxDelay time.Duration

	// Multiplier 延迟倍数
	Multiplier float64

	// Jitter 抖动比例 (0-1)
	Jitter float64

	// RetryOn 判断是否重试
	RetryOn func(error) bool

	// OnRetry 重试回调
	OnRetry func(attempt int, err error)
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryOn:      func(err error) bool { return err != nil },
	}
}

// RunnableWithRetry 带重试的 Runnable
type RunnableWithRetry[I, O any] struct {
	runnable Runnable[I, O]
	config   *RetryConfig
}

// WithRetry 创建带重试的 Runnable
func WithRetry[I, O any](runnable Runnable[I, O], config ...*RetryConfig) *RunnableWithRetry[I, O] {
	cfg := DefaultRetryConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	return &RunnableWithRetry[I, O]{
		runnable: runnable,
		config:   cfg,
	}
}

// Name 返回名称
func (r *RunnableWithRetry[I, O]) Name() string {
	return r.runnable.Name() + "_with_retry"
}

// Description 返回描述
func (r *RunnableWithRetry[I, O]) Description() string {
	return r.runnable.Description()
}

// InputSchema 返回输入 Schema
func (r *RunnableWithRetry[I, O]) InputSchema() *Schema {
	return r.runnable.InputSchema()
}

// OutputSchema 返回输出 Schema
func (r *RunnableWithRetry[I, O]) OutputSchema() *Schema {
	return r.runnable.OutputSchema()
}

// Invoke 执行（带重试）
func (r *RunnableWithRetry[I, O]) Invoke(ctx context.Context, input I, opts ...Option) (O, error) {
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		result, err := r.runnable.Invoke(ctx, input, opts...)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if r.config.RetryOn != nil && !r.config.RetryOn(err) {
			return result, err
		}

		if attempt < r.config.MaxRetries {
			if r.config.OnRetry != nil {
				r.config.OnRetry(attempt, err)
			}

			// 等待
			select {
			case <-ctx.Done():
				var zero O
				return zero, ctx.Err()
			case <-time.After(delay):
			}

			// 更新延迟
			delay = time.Duration(float64(delay) * r.config.Multiplier)
			if delay > r.config.MaxDelay {
				delay = r.config.MaxDelay
			}
		}
	}

	var zero O
	return zero, lastErr
}

// Stream 流式执行（带重试）
func (r *RunnableWithRetry[I, O]) Stream(ctx context.Context, input I, opts ...Option) (*StreamReader[O], error) {
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		stream, err := r.runnable.Stream(ctx, input, opts...)
		if err == nil {
			return stream, nil
		}

		lastErr = err

		if r.config.RetryOn != nil && !r.config.RetryOn(err) {
			return nil, err
		}

		if attempt < r.config.MaxRetries {
			if r.config.OnRetry != nil {
				r.config.OnRetry(attempt, err)
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * r.config.Multiplier)
			if delay > r.config.MaxDelay {
				delay = r.config.MaxDelay
			}
		}
	}

	return nil, lastErr
}

// Batch 批量执行（带重试）
func (r *RunnableWithRetry[I, O]) Batch(ctx context.Context, inputs []I, opts ...Option) ([]O, error) {
	return r.runnable.Batch(ctx, inputs, opts...)
}

// Collect 流收集
func (r *RunnableWithRetry[I, O]) Collect(ctx context.Context, input *StreamReader[I], opts ...Option) (O, error) {
	return r.runnable.Collect(ctx, input, opts...)
}

// Transform 流转换
func (r *RunnableWithRetry[I, O]) Transform(ctx context.Context, input *StreamReader[I], opts ...Option) (*StreamReader[O], error) {
	return r.runnable.Transform(ctx, input, opts...)
}

// BatchStream 批量流式
func (r *RunnableWithRetry[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...Option) (*StreamReader[O], error) {
	return r.runnable.BatchStream(ctx, inputs, opts...)
}

// ============== Circuit Breaker ==============

// CircuitState 熔断器状态
type CircuitState int

const (
	// CircuitClosed 关闭状态（正常）
	CircuitClosed CircuitState = iota
	// CircuitOpen 打开状态（熔断）
	CircuitOpen
	// CircuitHalfOpen 半开状态（尝试恢复）
	CircuitHalfOpen
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// FailureThreshold 失败阈值
	FailureThreshold int

	// SuccessThreshold 成功阈值（半开状态）
	SuccessThreshold int

	// Timeout 熔断超时
	Timeout time.Duration

	// OnStateChange 状态变化回调
	OnStateChange func(from, to CircuitState)
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	state           int32 // atomic
	failures        int32 // atomic
	successes       int32 // atomic
	lastFailureTime time.Time
	mu              sync.RWMutex
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(config ...*CircuitBreakerConfig) *CircuitBreaker {
	cfg := DefaultCircuitBreakerConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	return &CircuitBreaker{
		config: cfg,
		state:  int32(CircuitClosed),
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// Allow 检查是否允许执行
func (cb *CircuitBreaker) Allow() bool {
	state := cb.State()

	if state == CircuitClosed {
		return true
	}

	if state == CircuitOpen {
		cb.mu.RLock()
		lastFailure := cb.lastFailureTime
		cb.mu.RUnlock()

		if time.Since(lastFailure) > cb.config.Timeout {
			cb.transition(CircuitOpen, CircuitHalfOpen)
			return true
		}
		return false
	}

	// CircuitHalfOpen
	return true
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.StoreInt32(&cb.failures, 0)

	state := cb.State()
	if state == CircuitHalfOpen {
		successes := atomic.AddInt32(&cb.successes, 1)
		if int(successes) >= cb.config.SuccessThreshold {
			cb.transition(CircuitHalfOpen, CircuitClosed)
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	atomic.StoreInt32(&cb.successes, 0)

	cb.mu.Lock()
	cb.lastFailureTime = time.Now()
	cb.mu.Unlock()

	state := cb.State()
	if state == CircuitHalfOpen {
		cb.transition(CircuitHalfOpen, CircuitOpen)
		return
	}

	if state == CircuitClosed {
		failures := atomic.AddInt32(&cb.failures, 1)
		if int(failures) >= cb.config.FailureThreshold {
			cb.transition(CircuitClosed, CircuitOpen)
		}
	}
}

func (cb *CircuitBreaker) transition(from, to CircuitState) {
	if atomic.CompareAndSwapInt32(&cb.state, int32(from), int32(to)) {
		atomic.StoreInt32(&cb.failures, 0)
		atomic.StoreInt32(&cb.successes, 0)

		if cb.config.OnStateChange != nil {
			cb.config.OnStateChange(from, to)
		}
	}
}

// RunnableWithCircuitBreaker 带熔断器的 Runnable
type RunnableWithCircuitBreaker[I, O any] struct {
	runnable Runnable[I, O]
	breaker  *CircuitBreaker
}

// WithCircuitBreaker 创建带熔断器的 Runnable
func WithCircuitBreaker[I, O any](runnable Runnable[I, O], config ...*CircuitBreakerConfig) *RunnableWithCircuitBreaker[I, O] {
	return &RunnableWithCircuitBreaker[I, O]{
		runnable: runnable,
		breaker:  NewCircuitBreaker(config...),
	}
}

// Name 返回名称
func (r *RunnableWithCircuitBreaker[I, O]) Name() string {
	return r.runnable.Name() + "_with_circuit_breaker"
}

// Description 返回描述
func (r *RunnableWithCircuitBreaker[I, O]) Description() string {
	return r.runnable.Description()
}

// InputSchema 返回输入 Schema
func (r *RunnableWithCircuitBreaker[I, O]) InputSchema() *Schema {
	return r.runnable.InputSchema()
}

// OutputSchema 返回输出 Schema
func (r *RunnableWithCircuitBreaker[I, O]) OutputSchema() *Schema {
	return r.runnable.OutputSchema()
}

// Invoke 执行（带熔断）
func (r *RunnableWithCircuitBreaker[I, O]) Invoke(ctx context.Context, input I, opts ...Option) (O, error) {
	if !r.breaker.Allow() {
		var zero O
		return zero, ErrCircuitOpen
	}

	result, err := r.runnable.Invoke(ctx, input, opts...)
	if err != nil {
		r.breaker.RecordFailure()
		return result, err
	}

	r.breaker.RecordSuccess()
	return result, nil
}

// Stream 流式执行（带熔断）
func (r *RunnableWithCircuitBreaker[I, O]) Stream(ctx context.Context, input I, opts ...Option) (*StreamReader[O], error) {
	if !r.breaker.Allow() {
		return nil, ErrCircuitOpen
	}

	stream, err := r.runnable.Stream(ctx, input, opts...)
	if err != nil {
		r.breaker.RecordFailure()
		return nil, err
	}

	r.breaker.RecordSuccess()
	return stream, nil
}

// Batch 批量执行
func (r *RunnableWithCircuitBreaker[I, O]) Batch(ctx context.Context, inputs []I, opts ...Option) ([]O, error) {
	if !r.breaker.Allow() {
		return nil, ErrCircuitOpen
	}

	results, err := r.runnable.Batch(ctx, inputs, opts...)
	if err != nil {
		r.breaker.RecordFailure()
		return nil, err
	}

	r.breaker.RecordSuccess()
	return results, nil
}

// Collect 流收集
func (r *RunnableWithCircuitBreaker[I, O]) Collect(ctx context.Context, input *StreamReader[I], opts ...Option) (O, error) {
	return r.runnable.Collect(ctx, input, opts...)
}

// Transform 流转换
func (r *RunnableWithCircuitBreaker[I, O]) Transform(ctx context.Context, input *StreamReader[I], opts ...Option) (*StreamReader[O], error) {
	return r.runnable.Transform(ctx, input, opts...)
}

// BatchStream 批量流式
func (r *RunnableWithCircuitBreaker[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...Option) (*StreamReader[O], error) {
	if !r.breaker.Allow() {
		return nil, ErrCircuitOpen
	}

	stream, err := r.runnable.BatchStream(ctx, inputs, opts...)
	if err != nil {
		r.breaker.RecordFailure()
		return nil, err
	}

	r.breaker.RecordSuccess()
	return stream, nil
}
