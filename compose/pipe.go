// Package compose 提供 Hexagon 框架的编排能力
//
// 本文件实现管道操作符和链式组合能力。
//
// 设计借鉴：
//   - LangChain LCEL: 管道操作符 |
//   - Eino: Chain 链式编排
//   - Spring AI: Fluent API
//
// 使用示例：
//
//	// 管道组合
//	chain := compose.Pipe(promptTemplate, chatModel, outputParser)
//
//	// 链式构建
//	chain := compose.NewChain[string, string]().
//	    Then(promptTemplate).
//	    Then(chatModel).
//	    Build()
package compose

import (
	"context"
	"sync"

	"github.com/everyday-items/hexagon/core"
	"github.com/everyday-items/hexagon/stream"
)

// ============== Pipe 管道操作符 ==============

// Pipe 将多个 Runnable 串联成管道
// 类型安全：编译时检查类型匹配
func Pipe[I, M, O any](first core.Runnable[I, M], second core.Runnable[M, O]) core.Runnable[I, O] {
	return &pipeRunnable[I, M, O]{
		first:  first,
		second: second,
	}
}

// Pipe3 三个 Runnable 串联
func Pipe3[I, M1, M2, O any](
	first core.Runnable[I, M1],
	second core.Runnable[M1, M2],
	third core.Runnable[M2, O],
) core.Runnable[I, O] {
	return Pipe(Pipe(first, second), third)
}

// Pipe4 四个 Runnable 串联
func Pipe4[I, M1, M2, M3, O any](
	first core.Runnable[I, M1],
	second core.Runnable[M1, M2],
	third core.Runnable[M2, M3],
	fourth core.Runnable[M3, O],
) core.Runnable[I, O] {
	return Pipe(Pipe3(first, second, third), fourth)
}

// Pipe5 五个 Runnable 串联
func Pipe5[I, M1, M2, M3, M4, O any](
	first core.Runnable[I, M1],
	second core.Runnable[M1, M2],
	third core.Runnable[M2, M3],
	fourth core.Runnable[M3, M4],
	fifth core.Runnable[M4, O],
) core.Runnable[I, O] {
	return Pipe(Pipe4(first, second, third, fourth), fifth)
}

type pipeRunnable[I, M, O any] struct {
	first  core.Runnable[I, M]
	second core.Runnable[M, O]
}

func (p *pipeRunnable[I, M, O]) Name() string {
	return p.first.Name() + " | " + p.second.Name()
}

func (p *pipeRunnable[I, M, O]) Description() string {
	return "Pipe: " + p.first.Description() + " -> " + p.second.Description()
}

func (p *pipeRunnable[I, M, O]) InputSchema() *core.Schema {
	return p.first.InputSchema()
}

func (p *pipeRunnable[I, M, O]) OutputSchema() *core.Schema {
	return p.second.OutputSchema()
}

func (p *pipeRunnable[I, M, O]) Invoke(ctx context.Context, input I, opts ...core.Option) (O, error) {
	mid, err := p.first.Invoke(ctx, input, opts...)
	if err != nil {
		var zero O
		return zero, err
	}
	return p.second.Invoke(ctx, mid, opts...)
}

func (p *pipeRunnable[I, M, O]) Stream(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
	// 尝试流式传递
	midStream, err := p.first.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}

	// 使用 Transform 处理流
	return p.second.Transform(ctx, midStream, opts...)
}

func (p *pipeRunnable[I, M, O]) Batch(ctx context.Context, inputs []I, opts ...core.Option) ([]O, error) {
	mids, err := p.first.Batch(ctx, inputs, opts...)
	if err != nil {
		return nil, err
	}
	return p.second.Batch(ctx, mids, opts...)
}

func (p *pipeRunnable[I, M, O]) Collect(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error) {
	// Collect -> Transform -> Collect
	midStream, err := p.first.Transform(ctx, input, opts...)
	if err != nil {
		var zero O
		return zero, err
	}
	return p.second.Collect(ctx, midStream, opts...)
}

func (p *pipeRunnable[I, M, O]) Transform(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
	midStream, err := p.first.Transform(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return p.second.Transform(ctx, midStream, opts...)
}

func (p *pipeRunnable[I, M, O]) BatchStream(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error) {
	midStream, err := p.first.BatchStream(ctx, inputs, opts...)
	if err != nil {
		return nil, err
	}
	return p.second.Transform(ctx, midStream, opts...)
}

// ============== Parallel 并行执行 ==============

// Parallel 并行执行多个 Runnable，返回结果数组
func Parallel[I, O any](runnables ...core.Runnable[I, O]) core.Runnable[I, []O] {
	return &parallelRunnable[I, O]{
		runnables: runnables,
	}
}

type parallelRunnable[I, O any] struct {
	runnables []core.Runnable[I, O]
}

func (p *parallelRunnable[I, O]) Name() string {
	return "Parallel"
}

func (p *parallelRunnable[I, O]) Description() string {
	return "Parallel execution"
}

func (p *parallelRunnable[I, O]) InputSchema() *core.Schema {
	return core.SchemaOf[I]()
}

func (p *parallelRunnable[I, O]) OutputSchema() *core.Schema {
	return core.SchemaOf[[]O]()
}

func (p *parallelRunnable[I, O]) Invoke(ctx context.Context, input I, opts ...core.Option) ([]O, error) {
	results := make([]O, len(p.runnables))
	errs := make([]error, len(p.runnables))

	var wg sync.WaitGroup
	for i, r := range p.runnables {
		wg.Add(1)
		go func(idx int, runnable core.Runnable[I, O]) {
			defer wg.Done()
			result, err := runnable.Invoke(ctx, input, opts...)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = result
		}(i, r)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (p *parallelRunnable[I, O]) Stream(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[[]O], error) {
	result, err := p.Invoke(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return stream.FromValue(result), nil
}

func (p *parallelRunnable[I, O]) Batch(ctx context.Context, inputs []I, opts ...core.Option) ([][]O, error) {
	results := make([][]O, len(inputs))
	for i, input := range inputs {
		result, err := p.Invoke(ctx, input, opts...)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

func (p *parallelRunnable[I, O]) Collect(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) ([]O, error) {
	in, err := stream.Concat(ctx, input)
	if err != nil {
		return nil, err
	}
	return p.Invoke(ctx, in, opts...)
}

func (p *parallelRunnable[I, O]) Transform(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[[]O], error) {
	reader, writer := stream.Pipe[[]O](10)
	go func() {
		defer writer.Close()
		for {
			in, err := input.Recv()
			if err != nil {
				return
			}
			result, err := p.Invoke(ctx, in, opts...)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			writer.Send(result)
		}
	}()
	return reader, nil
}

func (p *parallelRunnable[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[[]O], error) {
	results, err := p.Batch(ctx, inputs, opts...)
	if err != nil {
		return nil, err
	}
	return stream.FromSlice(results), nil
}

// ============== Branch 条件分支 ==============

// Branch 条件分支
func Branch[I, O any](
	condition func(context.Context, I) (string, error),
	branches map[string]core.Runnable[I, O],
	defaultBranch core.Runnable[I, O],
) core.Runnable[I, O] {
	return &branchRunnable[I, O]{
		condition:     condition,
		branches:      branches,
		defaultBranch: defaultBranch,
	}
}

type branchRunnable[I, O any] struct {
	condition     func(context.Context, I) (string, error)
	branches      map[string]core.Runnable[I, O]
	defaultBranch core.Runnable[I, O]
}

func (b *branchRunnable[I, O]) Name() string {
	return "Branch"
}

func (b *branchRunnable[I, O]) Description() string {
	return "Conditional branch"
}

func (b *branchRunnable[I, O]) InputSchema() *core.Schema {
	return core.SchemaOf[I]()
}

func (b *branchRunnable[I, O]) OutputSchema() *core.Schema {
	return core.SchemaOf[O]()
}

func (b *branchRunnable[I, O]) Invoke(ctx context.Context, input I, opts ...core.Option) (O, error) {
	key, err := b.condition(ctx, input)
	if err != nil {
		var zero O
		return zero, err
	}

	if runnable, ok := b.branches[key]; ok {
		return runnable.Invoke(ctx, input, opts...)
	}

	if b.defaultBranch != nil {
		return b.defaultBranch.Invoke(ctx, input, opts...)
	}

	var zero O
	return zero, nil
}

func (b *branchRunnable[I, O]) Stream(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
	key, err := b.condition(ctx, input)
	if err != nil {
		return nil, err
	}

	if runnable, ok := b.branches[key]; ok {
		return runnable.Stream(ctx, input, opts...)
	}

	if b.defaultBranch != nil {
		return b.defaultBranch.Stream(ctx, input, opts...)
	}

	return stream.FromSlice[O](nil), nil
}

func (b *branchRunnable[I, O]) Batch(ctx context.Context, inputs []I, opts ...core.Option) ([]O, error) {
	results := make([]O, len(inputs))
	for i, input := range inputs {
		result, err := b.Invoke(ctx, input, opts...)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

func (b *branchRunnable[I, O]) Collect(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error) {
	in, err := stream.Concat(ctx, input)
	if err != nil {
		var zero O
		return zero, err
	}
	return b.Invoke(ctx, in, opts...)
}

func (b *branchRunnable[I, O]) Transform(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
	reader, writer := stream.Pipe[O](10)
	go func() {
		defer writer.Close()
		for {
			in, err := input.Recv()
			if err != nil {
				return
			}
			result, err := b.Invoke(ctx, in, opts...)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			writer.Send(result)
		}
	}()
	return reader, nil
}

func (b *branchRunnable[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error) {
	results, err := b.Batch(ctx, inputs, opts...)
	if err != nil {
		return nil, err
	}
	return stream.FromSlice(results), nil
}

// ============== Chain 链式构建器 ==============

// ChainBuilder 链式构建器
type ChainBuilder[I, O any] struct {
	runnable core.Runnable[I, O]
}

// NewChain 创建链式构建器
func NewChain[I any]() *ChainBuilder[I, I] {
	return &ChainBuilder[I, I]{
		runnable: Lambda(func(i I) I { return i }),
	}
}

// Then 添加下一个节点
func Then[I, M, O any](c *ChainBuilder[I, M], next core.Runnable[M, O]) *ChainBuilder[I, O] {
	return &ChainBuilder[I, O]{
		runnable: Pipe(c.runnable, next),
	}
}

// ThenFunc 添加函数节点
func ThenFunc[I, M, O any](c *ChainBuilder[I, M], fn func(M) O) *ChainBuilder[I, O] {
	return Then(c, Lambda(fn))
}

// ThenFuncWithError 添加带错误的函数节点
func ThenFuncWithError[I, M, O any](c *ChainBuilder[I, M], fn func(M) (O, error)) *ChainBuilder[I, O] {
	return Then(c, LambdaWithError(fn))
}

// Build 构建 Runnable
func (c *ChainBuilder[I, O]) Build() core.Runnable[I, O] {
	return c.runnable
}

