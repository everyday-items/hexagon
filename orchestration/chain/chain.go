// Package chain 提供 Hexagon AI Agent 框架的链式编排
//
// Chain 是一种简化的编排模式，将多个组件按顺序串联执行。
// 相比 Graph，Chain 更加简单直观，适合简单的线性流程。
//
// 基本用法：
//
//	chain := NewChain[Input, Output]("my-chain").
//	    Pipe(step1).
//	    Pipe(step2).
//	    Pipe(step3).
//	    Build()
//
//	result, err := chain.Run(ctx, input)
package chain

import (
	"context"
	"fmt"

	"github.com/everyday-items/hexagon/core"
)

// Chain 链式组件
type Chain[I, O any] struct {
	name        string
	description string
	steps       []step
	middleware  []Middleware
}

// step 链中的步骤
type step struct {
	name    string
	handler func(ctx context.Context, input any) (any, error)
}

// Middleware 中间件
type Middleware func(next StepFunc) StepFunc

// StepFunc 步骤函数
type StepFunc func(ctx context.Context, input any) (any, error)

// ChainBuilder 链构建器
type ChainBuilder[I, O any] struct {
	chain *Chain[I, O]
	err   error
}

// NewChain 创建链构建器
func NewChain[I, O any](name string) *ChainBuilder[I, O] {
	return &ChainBuilder[I, O]{
		chain: &Chain[I, O]{
			name:  name,
			steps: make([]step, 0),
		},
	}
}

// WithDescription 设置描述
func (b *ChainBuilder[I, O]) WithDescription(desc string) *ChainBuilder[I, O] {
	if b.err != nil {
		return b
	}
	b.chain.description = desc
	return b
}

// Pipe 添加组件到链中
func (b *ChainBuilder[I, O]) Pipe(c core.Component[any, any]) *ChainBuilder[I, O] {
	if b.err != nil {
		return b
	}

	b.chain.steps = append(b.chain.steps, step{
		name: c.Name(),
		handler: func(ctx context.Context, input any) (any, error) {
			return c.Run(ctx, input)
		},
	})
	return b
}

// PipeFunc 添加函数到链中
func (b *ChainBuilder[I, O]) PipeFunc(name string, fn func(ctx context.Context, input any) (any, error)) *ChainBuilder[I, O] {
	if b.err != nil {
		return b
	}

	b.chain.steps = append(b.chain.steps, step{
		name:    name,
		handler: fn,
	})
	return b
}

// Use 添加中间件
func (b *ChainBuilder[I, O]) Use(middleware ...Middleware) *ChainBuilder[I, O] {
	if b.err != nil {
		return b
	}
	b.chain.middleware = append(b.chain.middleware, middleware...)
	return b
}

// Build 构建链
func (b *ChainBuilder[I, O]) Build() (*Chain[I, O], error) {
	if b.err != nil {
		return nil, b.err
	}
	if len(b.chain.steps) == 0 {
		return nil, fmt.Errorf("chain must have at least one step")
	}
	return b.chain, nil
}

// MustBuild 构建链，失败时 panic
func (b *ChainBuilder[I, O]) MustBuild() *Chain[I, O] {
	c, err := b.Build()
	if err != nil {
		panic(err)
	}
	return c
}

// Name 返回链名称
func (c *Chain[I, O]) Name() string {
	return c.name
}

// Description 返回链描述
func (c *Chain[I, O]) Description() string {
	return c.description
}

// Run 执行链
func (c *Chain[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	var current any = input

	for i, step := range c.steps {
		// 包装处理函数以应用中间件
		handler := step.handler
		for j := len(c.middleware) - 1; j >= 0; j-- {
			handler = c.middleware[j](handler)
		}

		result, err := handler(ctx, current)
		if err != nil {
			return zero, fmt.Errorf("step %d (%s) failed: %w", i, step.name, err)
		}
		current = result
	}

	// 尝试转换最终结果
	output, ok := current.(O)
	if !ok {
		return zero, fmt.Errorf("final output type mismatch: expected %T, got %T", zero, current)
	}

	return output, nil
}

// Stream 流式执行链
func (c *Chain[I, O]) Stream(ctx context.Context, input I) (core.Stream[O], error) {
	output, err := c.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	return core.NewSliceStream([]O{output}), nil
}

// Batch 批量执行链
func (c *Chain[I, O]) Batch(ctx context.Context, inputs []I) ([]O, error) {
	results := make([]O, len(inputs))
	for i, input := range inputs {
		output, err := c.Run(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("batch item %d failed: %w", i, err)
		}
		results[i] = output
	}
	return results, nil
}

// InputSchema 返回输入 Schema
func (c *Chain[I, O]) InputSchema() *core.Schema {
	return core.SchemaOf[I]()
}

// OutputSchema 返回输出 Schema
func (c *Chain[I, O]) OutputSchema() *core.Schema {
	return core.SchemaOf[O]()
}

// 确保实现了 Component 接口
var _ core.Component[any, any] = (*Chain[any, any])(nil)

// ============== 常用中间件 ==============

// LoggingMiddleware 日志中间件
func LoggingMiddleware(logger func(name string, input, output any, err error)) Middleware {
	return func(next StepFunc) StepFunc {
		return func(ctx context.Context, input any) (any, error) {
			output, err := next(ctx, input)
			logger("step", input, output, err)
			return output, err
		}
	}
}

// RetryMiddleware 重试中间件
func RetryMiddleware(maxRetries int, shouldRetry func(error) bool) Middleware {
	return func(next StepFunc) StepFunc {
		return func(ctx context.Context, input any) (any, error) {
			var lastErr error
			for i := 0; i <= maxRetries; i++ {
				output, err := next(ctx, input)
				if err == nil {
					return output, nil
				}
				lastErr = err
				if !shouldRetry(err) {
					return nil, err
				}
			}
			return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
		}
	}
}

// RecoverMiddleware panic 恢复中间件
func RecoverMiddleware() Middleware {
	return func(next StepFunc) StepFunc {
		return func(ctx context.Context, input any) (output any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()
			return next(ctx, input)
		}
	}
}

// ============== 类型安全的链式组件 ==============

// TypedStep 类型安全的步骤
type TypedStep[I, O any] struct {
	name    string
	handler func(ctx context.Context, input I) (O, error)
}

// NewTypedStep 创建类型安全的步骤
func NewTypedStep[I, O any](name string, handler func(ctx context.Context, input I) (O, error)) *TypedStep[I, O] {
	return &TypedStep[I, O]{
		name:    name,
		handler: handler,
	}
}

// ToStep 转换为通用步骤
func (s *TypedStep[I, O]) ToStep() step {
	return step{
		name: s.name,
		handler: func(ctx context.Context, input any) (any, error) {
			typedInput, ok := input.(I)
			if !ok {
				var zero I
				return nil, fmt.Errorf("input type mismatch: expected %T, got %T", zero, input)
			}
			return s.handler(ctx, typedInput)
		},
	}
}

// Then 连接另一个类型安全的步骤
func Then[I, M, O any](first *TypedStep[I, M], second *TypedStep[M, O]) *TypedStep[I, O] {
	return &TypedStep[I, O]{
		name: first.name + " -> " + second.name,
		handler: func(ctx context.Context, input I) (O, error) {
			var zero O
			mid, err := first.handler(ctx, input)
			if err != nil {
				return zero, err
			}
			return second.handler(ctx, mid)
		},
	}
}

// ============== 并行执行 ==============

// Parallel 并行执行多个组件
type Parallel[I, O any] struct {
	name     string
	handlers []func(ctx context.Context, input I) (O, error)
	merge    func(results []O) O
}

// NewParallel 创建并行执行器
func NewParallel[I, O any](name string, merge func(results []O) O) *Parallel[I, O] {
	return &Parallel[I, O]{
		name:  name,
		merge: merge,
	}
}

// Add 添加并行执行的处理函数
func (p *Parallel[I, O]) Add(handler func(ctx context.Context, input I) (O, error)) *Parallel[I, O] {
	p.handlers = append(p.handlers, handler)
	return p
}

// Run 执行并行处理
func (p *Parallel[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	if len(p.handlers) == 0 {
		return zero, fmt.Errorf("no handlers in parallel")
	}

	type result struct {
		output O
		err    error
	}

	results := make(chan result, len(p.handlers))

	for _, h := range p.handlers {
		h := h
		go func() {
			output, err := h(ctx, input)
			results <- result{output: output, err: err}
		}()
	}

	outputs := make([]O, 0, len(p.handlers))
	for range p.handlers {
		r := <-results
		if r.err != nil {
			return zero, r.err
		}
		outputs = append(outputs, r.output)
	}

	return p.merge(outputs), nil
}
