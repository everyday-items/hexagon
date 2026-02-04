// Package core 提供 Hexagon 框架的核心接口和类型
//
// 本包定义了框架的基础抽象：
//   - Component[I, O]: 统一组件接口，所有组件（Agent、Tool、Chain、Graph）都实现此接口
//   - Stream[T]: 泛型流接口，提供流式数据处理能力
//   - Schema: JSON Schema 类型定义
//
// 设计理念：
//   - 组合优于继承：通过接口组合实现功能扩展
//   - 类型安全：使用泛型确保编译时类型检查
//   - 统一执行模型：Run/Stream/Batch 三种执行方式
package core

import (
	"context"
	"sync"

	"github.com/everyday-items/ai-core/schema"
)

// Schema 是 ai-core/schema.Schema 的别名
type Schema = schema.Schema

// SchemaOf 从 Go 类型生成 Schema
func SchemaOf[T any]() *Schema {
	return schema.Of[T]()
}

// Component 是所有组件的统一接口
// 借鉴 LangChain Runnable + Haystack Component
// 所有组件 (Agent, Tool, Chain, Graph) 都实现此接口，可任意组合
type Component[I, O any] interface {
	// Name 返回组件名称
	Name() string

	// Description 返回组件描述
	Description() string

	// Run 执行组件（非流式）
	Run(ctx context.Context, input I) (O, error)

	// Stream 执行组件（流式）
	Stream(ctx context.Context, input I) (Stream[O], error)

	// Batch 批量执行组件
	Batch(ctx context.Context, inputs []I) ([]O, error)

	// InputSchema 返回输入参数的 Schema
	InputSchema() *Schema

	// OutputSchema 返回输出参数的 Schema
	OutputSchema() *Schema
}

// Stream 是泛型流接口
// 提供流式数据处理能力，支持管道操作
type Stream[T any] interface {
	// Next 读取下一个元素，返回值和是否还有更多元素
	Next(ctx context.Context) (T, bool)

	// Err 返回流处理中发生的错误
	Err() error

	// Close 关闭流，释放资源
	Close() error

	// Collect 收集所有元素到切片
	Collect(ctx context.Context) ([]T, error)

	// ForEach 对每个元素执行操作
	ForEach(ctx context.Context, fn func(T) error) error
}

// SliceStream 从切片创建 Stream
type SliceStream[T any] struct {
	items []T
	index int
	mu    sync.Mutex
}

// NewSliceStream 从切片创建流
func NewSliceStream[T any](items []T) *SliceStream[T] {
	return &SliceStream[T]{items: items}
}

// Next 返回流中的下一个值
func (s *SliceStream[T]) Next(ctx context.Context) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var zero T
	if ctx.Err() != nil {
		return zero, false
	}
	if s.index >= len(s.items) {
		return zero, false
	}
	v := s.items[s.index]
	s.index++
	return v, true
}

// Err 返回 nil，切片流不会产生错误
func (s *SliceStream[T]) Err() error {
	return nil
}

// Close 对于切片流是空操作
func (s *SliceStream[T]) Close() error {
	return nil
}

// Collect 返回剩余的元素
func (s *SliceStream[T]) Collect(ctx context.Context) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := s.items[s.index:]
	s.index = len(s.items)
	return remaining, nil
}

// ForEach 对每个元素执行操作
// 遍历流中的所有元素，对每个元素调用 fn 函数
// 如果 fn 返回错误或 context 被取消，则提前终止
func (s *SliceStream[T]) ForEach(ctx context.Context, fn func(T) error) error {
	for {
		// 检查 context 是否被取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 尝试获取下一个元素
		v, ok := s.Next(ctx)
		if !ok {
			// 流已结束，检查是否有错误
			if err := s.Err(); err != nil {
				return err
			}
			// 正常结束
			return nil
		}

		// 对元素执行操作
		if err := fn(v); err != nil {
			return err
		}
	}
}

// BaseComponent 提供 Component 接口的基础实现
type BaseComponent[I, O any] struct {
	name        string
	description string
	runFn       func(context.Context, I) (O, error)
	streamFn    func(context.Context, I) (Stream[O], error)
}

// NewBaseComponent 创建基础组件
func NewBaseComponent[I, O any](name, description string, runFn func(context.Context, I) (O, error)) *BaseComponent[I, O] {
	return &BaseComponent[I, O]{
		name:        name,
		description: description,
		runFn:       runFn,
	}
}

// Name 返回组件名称
func (c *BaseComponent[I, O]) Name() string {
	return c.name
}

// Description 返回组件描述
func (c *BaseComponent[I, O]) Description() string {
	return c.description
}

// Run 执行组件
func (c *BaseComponent[I, O]) Run(ctx context.Context, input I) (O, error) {
	return c.runFn(ctx, input)
}

// Stream 流式执行组件
func (c *BaseComponent[I, O]) Stream(ctx context.Context, input I) (Stream[O], error) {
	if c.streamFn != nil {
		return c.streamFn(ctx, input)
	}
	result, err := c.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	return NewSliceStream([]O{result}), nil
}

// Batch 批量执行组件
// 并发执行所有输入，返回结果切片（保持顺序）
// 如果任一执行失败，返回遇到的第一个错误
func (c *BaseComponent[I, O]) Batch(ctx context.Context, inputs []I) ([]O, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	// 单个输入直接执行，避免 goroutine 开销
	if len(inputs) == 1 {
		result, err := c.Run(ctx, inputs[0])
		if err != nil {
			return nil, err
		}
		return []O{result}, nil
	}

	results := make([]O, len(inputs))
	errs := make([]error, len(inputs))
	var wg sync.WaitGroup

	// 并发执行所有输入
	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in I) {
			defer wg.Done()
			// 检查 context 是否已取消
			if ctx.Err() != nil {
				errs[idx] = ctx.Err()
				return
			}
			result, err := c.Run(ctx, in)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = result
		}(i, input)
	}

	wg.Wait()

	// 返回遇到的第一个错误
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// InputSchema 返回输入 Schema
func (c *BaseComponent[I, O]) InputSchema() *Schema {
	return schema.Of[I]()
}

// OutputSchema 返回输出 Schema
func (c *BaseComponent[I, O]) OutputSchema() *Schema {
	return schema.Of[O]()
}

// SetStreamFn 设置流式执行函数
func (c *BaseComponent[I, O]) SetStreamFn(fn func(context.Context, I) (Stream[O], error)) *BaseComponent[I, O] {
	c.streamFn = fn
	return c
}
