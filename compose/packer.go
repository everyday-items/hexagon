// Package compose 提供 Hexagon 框架的编排能力
//
// 本文件实现自动范式转换器 RunnablePacker，支持 6 种范式间的 30 种自动转换。
//
// 转换矩阵：
//   - Invoke: 优先用户提供 > Stream+Concat > Collect+Single > Transform+Concat+Single
//   - Stream: 优先用户提供 > Invoke+Wrap > Transform+Single > Collect+Wrap
//   - Batch:  优先用户提供 > Invoke+Parallel > Stream+Parallel+Concat
//   - Collect: 优先用户提供 > Invoke+Concat > Transform+Concat
//   - Transform: 优先用户提供 > Stream+Map > Collect+Wrap > Invoke+Map+Wrap
//   - BatchStream: 优先用户提供 > Stream+Merge > Batch+Wrap
//
// 设计借鉴：
//   - Eino: runnablePacker 四范式转换
//   - Hexagon: 扩展到六范式，30 种转换
package compose

import (
	"context"
	"sync"

	"github.com/everyday-items/hexagon/core"
	"github.com/everyday-items/hexagon/stream"
)

// ============== 六范式函数类型 ==============

// InvokeFunc 同步调用函数类型
type InvokeFunc[I, O any] func(ctx context.Context, input I, opts ...core.Option) (O, error)

// StreamFunc 流式输出函数类型
type StreamFunc[I, O any] func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error)

// BatchFunc 批量调用函数类型
type BatchFunc[I, O any] func(ctx context.Context, inputs []I, opts ...core.Option) ([]O, error)

// CollectFunc 流收集函数类型
type CollectFunc[I, O any] func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error)

// TransformFunc 流转换函数类型
type TransformFunc[I, O any] func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error)

// BatchStreamFunc 批量流式函数类型
type BatchStreamFunc[I, O any] func(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error)

// ============== RunnablePacker ==============

// RunnablePacker 自动范式转换器
// 支持 6 种范式间的 30 种自动转换
type RunnablePacker[I, O any] struct {
	name        string
	description string

	// 用户提供的实现
	invoke      InvokeFunc[I, O]
	stream      StreamFunc[I, O]
	batch       BatchFunc[I, O]
	collect     CollectFunc[I, O]
	transform   TransformFunc[I, O]
	batchStream BatchStreamFunc[I, O]

	// 派生的实现（自动生成）
	derivedInvoke      InvokeFunc[I, O]
	derivedStream      StreamFunc[I, O]
	derivedBatch       BatchFunc[I, O]
	derivedCollect     CollectFunc[I, O]
	derivedTransform   TransformFunc[I, O]
	derivedBatchStream BatchStreamFunc[I, O]

	// 是否已完成派生
	derived bool
	mu      sync.Mutex
}

// NewPacker 创建范式转换器
func NewPacker[I, O any](name string) *RunnablePacker[I, O] {
	return &RunnablePacker[I, O]{
		name: name,
	}
}

// WithDescription 设置描述
func (p *RunnablePacker[I, O]) WithDescription(desc string) *RunnablePacker[I, O] {
	p.description = desc
	return p
}

// WithInvoke 设置 Invoke 实现
func (p *RunnablePacker[I, O]) WithInvoke(fn InvokeFunc[I, O]) *RunnablePacker[I, O] {
	p.invoke = fn
	p.derived = false
	return p
}

// WithStream 设置 Stream 实现
func (p *RunnablePacker[I, O]) WithStream(fn StreamFunc[I, O]) *RunnablePacker[I, O] {
	p.stream = fn
	p.derived = false
	return p
}

// WithBatch 设置 Batch 实现
func (p *RunnablePacker[I, O]) WithBatch(fn BatchFunc[I, O]) *RunnablePacker[I, O] {
	p.batch = fn
	p.derived = false
	return p
}

// WithCollect 设置 Collect 实现
func (p *RunnablePacker[I, O]) WithCollect(fn CollectFunc[I, O]) *RunnablePacker[I, O] {
	p.collect = fn
	p.derived = false
	return p
}

// WithTransform 设置 Transform 实现
func (p *RunnablePacker[I, O]) WithTransform(fn TransformFunc[I, O]) *RunnablePacker[I, O] {
	p.transform = fn
	p.derived = false
	return p
}

// WithBatchStream 设置 BatchStream 实现
func (p *RunnablePacker[I, O]) WithBatchStream(fn BatchStreamFunc[I, O]) *RunnablePacker[I, O] {
	p.batchStream = fn
	p.derived = false
	return p
}

// derive 执行自动派生
func (p *RunnablePacker[I, O]) derive() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.derived {
		return
	}

	// 派生 Invoke
	p.derivedInvoke = p.deriveInvoke()

	// 派生 Stream
	p.derivedStream = p.deriveStream()

	// 派生 Batch
	p.derivedBatch = p.deriveBatch()

	// 派生 Collect
	p.derivedCollect = p.deriveCollect()

	// 派生 Transform
	p.derivedTransform = p.deriveTransform()

	// 派生 BatchStream
	p.derivedBatchStream = p.deriveBatchStream()

	p.derived = true
}

// deriveInvoke 派生 Invoke 实现
// 优先级：用户提供 > Stream+Concat > Collect+Single > Transform+Concat+Single
func (p *RunnablePacker[I, O]) deriveInvoke() InvokeFunc[I, O] {
	if p.invoke != nil {
		return p.invoke
	}

	// 从 Stream 派生
	if p.stream != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (O, error) {
			sr, err := p.stream(ctx, input, opts...)
			if err != nil {
				var zero O
				return zero, err
			}
			return stream.Concat(ctx, sr)
		}
	}

	// 从 Collect 派生
	if p.collect != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (O, error) {
			sr := stream.FromValue(input)
			return p.collect(ctx, sr, opts...)
		}
	}

	// 从 Transform 派生
	if p.transform != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (O, error) {
			sr := stream.FromValue(input)
			outSr, err := p.transform(ctx, sr, opts...)
			if err != nil {
				var zero O
				return zero, err
			}
			return stream.Concat(ctx, outSr)
		}
	}

	// 从 Batch 派生
	if p.batch != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (O, error) {
			results, err := p.batch(ctx, []I{input}, opts...)
			if err != nil {
				var zero O
				return zero, err
			}
			if len(results) == 0 {
				var zero O
				return zero, nil
			}
			return results[0], nil
		}
	}

	// 从 BatchStream 派生
	if p.batchStream != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (O, error) {
			sr, err := p.batchStream(ctx, []I{input}, opts...)
			if err != nil {
				var zero O
				return zero, err
			}
			return stream.Concat(ctx, sr)
		}
	}

	return nil
}

// deriveStream 派生 Stream 实现
// 优先级：用户提供 > Invoke+Wrap > Transform+Single > Collect+Wrap
func (p *RunnablePacker[I, O]) deriveStream() StreamFunc[I, O] {
	if p.stream != nil {
		return p.stream
	}

	// 从 Invoke 派生
	if p.invoke != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
			result, err := p.invoke(ctx, input, opts...)
			if err != nil {
				return nil, err
			}
			return stream.FromValue(result), nil
		}
	}

	// 从 Transform 派生
	if p.transform != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
			sr := stream.FromValue(input)
			return p.transform(ctx, sr, opts...)
		}
	}

	// 从 Collect 派生
	if p.collect != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
			sr := stream.FromValue(input)
			result, err := p.collect(ctx, sr, opts...)
			if err != nil {
				return nil, err
			}
			return stream.FromValue(result), nil
		}
	}

	// 从 Batch 派生
	if p.batch != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
			results, err := p.batch(ctx, []I{input}, opts...)
			if err != nil {
				return nil, err
			}
			return stream.FromSlice(results), nil
		}
	}

	// 从 BatchStream 派生
	if p.batchStream != nil {
		return func(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
			return p.batchStream(ctx, []I{input}, opts...)
		}
	}

	return nil
}

// deriveBatch 派生 Batch 实现
// 优先级：用户提供 > Invoke+Parallel > Stream+Parallel+Concat
func (p *RunnablePacker[I, O]) deriveBatch() BatchFunc[I, O] {
	if p.batch != nil {
		return p.batch
	}

	// 从 Invoke 或其他派生 Invoke 派生
	invokeFunc := p.deriveInvoke()
	if invokeFunc != nil {
		return func(ctx context.Context, inputs []I, opts ...core.Option) ([]O, error) {
			if len(inputs) == 0 {
				return nil, nil
			}

			if len(inputs) == 1 {
				result, err := invokeFunc(ctx, inputs[0], opts...)
				if err != nil {
					return nil, err
				}
				return []O{result}, nil
			}

			results := make([]O, len(inputs))
			errs := make([]error, len(inputs))
			var wg sync.WaitGroup

			for i, input := range inputs {
				wg.Add(1)
				go func(idx int, in I) {
					defer wg.Done()
					if ctx.Err() != nil {
						errs[idx] = ctx.Err()
						return
					}
					result, err := invokeFunc(ctx, in, opts...)
					if err != nil {
						errs[idx] = err
						return
					}
					results[idx] = result
				}(i, input)
			}

			wg.Wait()

			for _, err := range errs {
				if err != nil {
					return nil, err
				}
			}

			return results, nil
		}
	}

	return nil
}

// deriveCollect 派生 Collect 实现
// 优先级：用户提供 > Invoke+Concat > Transform+Concat
func (p *RunnablePacker[I, O]) deriveCollect() CollectFunc[I, O] {
	if p.collect != nil {
		return p.collect
	}

	// 从 Invoke 派生
	invokeFunc := p.deriveInvoke()
	if invokeFunc != nil {
		return func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error) {
			in, err := stream.Concat(ctx, input)
			if err != nil {
				var zero O
				return zero, err
			}
			return invokeFunc(ctx, in, opts...)
		}
	}

	// 从 Transform 派生
	if p.transform != nil {
		return func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error) {
			outSr, err := p.transform(ctx, input, opts...)
			if err != nil {
				var zero O
				return zero, err
			}
			return stream.Concat(ctx, outSr)
		}
	}

	return nil
}

// deriveTransform 派生 Transform 实现
// 优先级：用户提供 > Stream+Map > Collect+Wrap > Invoke+Map+Wrap
func (p *RunnablePacker[I, O]) deriveTransform() TransformFunc[I, O] {
	if p.transform != nil {
		return p.transform
	}

	// 从 Stream 派生
	if p.stream != nil {
		return func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
			// 先收集输入流
			in, err := stream.Concat(ctx, input)
			if err != nil {
				return nil, err
			}
			return p.stream(ctx, in, opts...)
		}
	}

	// 从 Invoke 派生（对每个输入元素调用 Invoke）
	invokeFunc := p.deriveInvoke()
	if invokeFunc != nil {
		return func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
			reader, writer := stream.Pipe[O](10)
			go func() {
				defer writer.Close()
				for {
					in, err := input.Recv()
					if err != nil {
						return
					}
					out, err := invokeFunc(ctx, in, opts...)
					if err != nil {
						writer.CloseWithError(err)
						return
					}
					writer.Send(out)
				}
			}()
			return reader, nil
		}
	}

	// 从 Collect 派生
	if p.collect != nil {
		return func(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
			result, err := p.collect(ctx, input, opts...)
			if err != nil {
				return nil, err
			}
			return stream.FromValue(result), nil
		}
	}

	return nil
}

// deriveBatchStream 派生 BatchStream 实现
// 优先级：用户提供 > Stream+Merge > Batch+Wrap
func (p *RunnablePacker[I, O]) deriveBatchStream() BatchStreamFunc[I, O] {
	if p.batchStream != nil {
		return p.batchStream
	}

	// 从 Stream 派生
	streamFunc := p.deriveStream()
	if streamFunc != nil {
		return func(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error) {
			if len(inputs) == 0 {
				return stream.FromSlice[O](nil), nil
			}

			readers := make([]*stream.StreamReader[O], len(inputs))
			var mu sync.Mutex
			var wg sync.WaitGroup
			var firstErr error

			for i, input := range inputs {
				wg.Add(1)
				go func(idx int, in I) {
					defer wg.Done()
					sr, err := streamFunc(ctx, in, opts...)
					mu.Lock()
					defer mu.Unlock()
					if err != nil && firstErr == nil {
						firstErr = err
						return
					}
					if sr != nil {
						sr.SetSource(string(rune('A' + idx))) // 设置源标识
					}
					readers[idx] = sr
				}(i, input)
			}

			wg.Wait()

			if firstErr != nil {
				return nil, firstErr
			}

			// 过滤掉 nil
			validReaders := make([]*stream.StreamReader[O], 0, len(readers))
			for _, r := range readers {
				if r != nil {
					validReaders = append(validReaders, r)
				}
			}

			return stream.Merge(validReaders...), nil
		}
	}

	// 从 Batch 派生
	batchFunc := p.deriveBatch()
	if batchFunc != nil {
		return func(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error) {
			results, err := batchFunc(ctx, inputs, opts...)
			if err != nil {
				return nil, err
			}
			return stream.FromSlice(results), nil
		}
	}

	return nil
}

// Build 构建 Runnable
func (p *RunnablePacker[I, O]) Build() core.Runnable[I, O] {
	p.derive()
	return &packedRunnable[I, O]{
		packer: p,
	}
}

// ============== packedRunnable ==============

type packedRunnable[I, O any] struct {
	packer *RunnablePacker[I, O]
}

func (r *packedRunnable[I, O]) Name() string {
	return r.packer.name
}

func (r *packedRunnable[I, O]) Description() string {
	return r.packer.description
}

func (r *packedRunnable[I, O]) InputSchema() *core.Schema {
	return core.SchemaOf[I]()
}

func (r *packedRunnable[I, O]) OutputSchema() *core.Schema {
	return core.SchemaOf[O]()
}

func (r *packedRunnable[I, O]) Invoke(ctx context.Context, input I, opts ...core.Option) (O, error) {
	if r.packer.derivedInvoke != nil {
		return r.packer.derivedInvoke(ctx, input, opts...)
	}
	var zero O
	return zero, nil
}

func (r *packedRunnable[I, O]) Stream(ctx context.Context, input I, opts ...core.Option) (*stream.StreamReader[O], error) {
	if r.packer.derivedStream != nil {
		return r.packer.derivedStream(ctx, input, opts...)
	}
	return stream.FromSlice[O](nil), nil
}

func (r *packedRunnable[I, O]) Batch(ctx context.Context, inputs []I, opts ...core.Option) ([]O, error) {
	if r.packer.derivedBatch != nil {
		return r.packer.derivedBatch(ctx, inputs, opts...)
	}
	return nil, nil
}

func (r *packedRunnable[I, O]) Collect(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (O, error) {
	if r.packer.derivedCollect != nil {
		return r.packer.derivedCollect(ctx, input, opts...)
	}
	var zero O
	return zero, nil
}

func (r *packedRunnable[I, O]) Transform(ctx context.Context, input *stream.StreamReader[I], opts ...core.Option) (*stream.StreamReader[O], error) {
	if r.packer.derivedTransform != nil {
		return r.packer.derivedTransform(ctx, input, opts...)
	}
	return stream.FromSlice[O](nil), nil
}

func (r *packedRunnable[I, O]) BatchStream(ctx context.Context, inputs []I, opts ...core.Option) (*stream.StreamReader[O], error) {
	if r.packer.derivedBatchStream != nil {
		return r.packer.derivedBatchStream(ctx, inputs, opts...)
	}
	return stream.FromSlice[O](nil), nil
}

// ============== 便捷构造函数 ==============

// FromInvoke 从 Invoke 函数创建 Runnable
func FromInvoke[I, O any](name string, fn InvokeFunc[I, O]) core.Runnable[I, O] {
	return NewPacker[I, O](name).WithInvoke(fn).Build()
}

// FromStream 从 Stream 函数创建 Runnable
func FromStream[I, O any](name string, fn StreamFunc[I, O]) core.Runnable[I, O] {
	return NewPacker[I, O](name).WithStream(fn).Build()
}

// FromTransform 从 Transform 函数创建 Runnable
func FromTransform[I, O any](name string, fn TransformFunc[I, O]) core.Runnable[I, O] {
	return NewPacker[I, O](name).WithTransform(fn).Build()
}

// FromCollect 从 Collect 函数创建 Runnable
func FromCollect[I, O any](name string, fn CollectFunc[I, O]) core.Runnable[I, O] {
	return NewPacker[I, O](name).WithCollect(fn).Build()
}

// Lambda 从简单函数创建 Runnable
func Lambda[I, O any](fn func(I) O) core.Runnable[I, O] {
	return FromInvoke("lambda", func(ctx context.Context, input I, opts ...core.Option) (O, error) {
		return fn(input), nil
	})
}

// LambdaWithError 从带错误的函数创建 Runnable
func LambdaWithError[I, O any](fn func(I) (O, error)) core.Runnable[I, O] {
	return FromInvoke("lambda", func(ctx context.Context, input I, opts ...core.Option) (O, error) {
		return fn(input)
	})
}
