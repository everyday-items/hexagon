package graph

import (
	"context"
	"fmt"
)

// NodeType 节点类型
type NodeType int

const (
	// NodeTypeNormal 普通节点
	NodeTypeNormal NodeType = iota
	// NodeTypeStart 起始节点
	NodeTypeStart
	// NodeTypeEnd 结束节点
	NodeTypeEnd
	// NodeTypeConditional 条件节点
	NodeTypeConditional
	// NodeTypeParallel 并行节点
	NodeTypeParallel
	// NodeTypeSubgraph 子图节点
	NodeTypeSubgraph
)

// Node 图节点
type Node[S State] struct {
	// Name 节点名称
	Name string

	// Type 节点类型
	Type NodeType

	// Handler 节点处理函数
	Handler NodeHandler[S]

	// Metadata 节点元数据
	Metadata map[string]any

	// RetryPolicy 重试策略
	RetryPolicy *RetryPolicy

	// Timeout 超时时间（毫秒）
	Timeout int64
}

// NodeHandler 节点处理函数类型
type NodeHandler[S State] func(ctx context.Context, state S) (S, error)

// NodeResult 节点执行结果
type NodeResult[S State] struct {
	// State 更新后的状态
	State S

	// NextNodes 下一个要执行的节点（用于条件路由）
	NextNodes []string

	// Error 执行错误
	Error error

	// Metadata 执行元数据
	Metadata map[string]any
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// InitialDelay 初始延迟（毫秒）
	InitialDelay int64

	// MaxDelay 最大延迟（毫秒）
	MaxDelay int64

	// Multiplier 延迟倍数
	Multiplier float64

	// RetryOn 重试条件（返回 true 表示需要重试）
	RetryOn func(error) bool
}

// DefaultRetryPolicy 默认重试策略
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 100,
		MaxDelay:     5000,
		Multiplier:   2.0,
		RetryOn: func(err error) bool {
			return err != nil
		},
	}
}

// NodeBuilder 节点构建器
type NodeBuilder[S State] struct {
	node *Node[S]
}

// NewNode 创建节点构建器
func NewNode[S State](name string, handler NodeHandler[S]) *NodeBuilder[S] {
	return &NodeBuilder[S]{
		node: &Node[S]{
			Name:     name,
			Type:     NodeTypeNormal,
			Handler:  handler,
			Metadata: make(map[string]any),
		},
	}
}

// WithType 设置节点类型
func (b *NodeBuilder[S]) WithType(t NodeType) *NodeBuilder[S] {
	b.node.Type = t
	return b
}

// WithMetadata 设置元数据
func (b *NodeBuilder[S]) WithMetadata(key string, value any) *NodeBuilder[S] {
	b.node.Metadata[key] = value
	return b
}

// WithRetry 设置重试策略
func (b *NodeBuilder[S]) WithRetry(policy *RetryPolicy) *NodeBuilder[S] {
	b.node.RetryPolicy = policy
	return b
}

// WithTimeout 设置超时时间（毫秒）
func (b *NodeBuilder[S]) WithTimeout(ms int64) *NodeBuilder[S] {
	b.node.Timeout = ms
	return b
}

// Build 构建节点
func (b *NodeBuilder[S]) Build() *Node[S] {
	return b.node
}

// 特殊节点名称常量
const (
	// START 起始节点名称
	START = "__start__"
	// END 结束节点名称
	END = "__end__"
)

// StartNode 创建起始节点
func StartNode[S State]() *Node[S] {
	return &Node[S]{
		Name: START,
		Type: NodeTypeStart,
		Handler: func(ctx context.Context, state S) (S, error) {
			return state, nil
		},
	}
}

// EndNode 创建结束节点
func EndNode[S State]() *Node[S] {
	return &Node[S]{
		Name: END,
		Type: NodeTypeEnd,
		Handler: func(ctx context.Context, state S) (S, error) {
			return state, nil
		},
	}
}

// ParallelNode 创建并行执行节点
// 将多个节点的处理函数并行执行，并合并结果
func ParallelNode[S State](name string, handlers ...NodeHandler[S]) *Node[S] {
	return &Node[S]{
		Name: name,
		Type: NodeTypeParallel,
		Handler: func(ctx context.Context, state S) (S, error) {
			// 并行执行所有处理函数
			type result struct {
				state S
				err   error
			}
			results := make(chan result, len(handlers))

			for _, h := range handlers {
				h := h // 捕获变量
				go func() {
					s, err := h(ctx, state.Clone().(S))
					results <- result{state: s, err: err}
				}()
			}

			// 收集结果
			var finalState S = state
			for i := 0; i < len(handlers); i++ {
				r := <-results
				if r.err != nil {
					return finalState, fmt.Errorf("parallel execution failed: %w", r.err)
				}
				// 这里简单使用最后一个结果，实际应该合并
				finalState = r.state
			}
			return finalState, nil
		},
	}
}

// ConditionalHandler 条件处理函数
// 返回下一个要执行的节点名称
type ConditionalHandler[S State] func(ctx context.Context, state S) string

// ConditionalNode 创建条件节点
func ConditionalNode[S State](name string, router ConditionalHandler[S]) *Node[S] {
	return &Node[S]{
		Name: name,
		Type: NodeTypeConditional,
		Handler: func(ctx context.Context, state S) (S, error) {
			// 条件节点不修改状态，只决定路由
			return state, nil
		},
		Metadata: map[string]any{
			"router": router,
		},
	}
}
