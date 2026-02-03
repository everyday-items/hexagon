// Package reranker 提供文档重排序功能
package reranker

import (
	"context"
	"strings"

	"github.com/everyday-items/hexagon/rag"
)

// ChainReranker 链式组合重排序器
//
// 将多个重排序器串联执行，每个重排序器的输出作为下一个的输入。
// 常用于组合多种重排序策略：
//   - 先过滤低分文档
//   - 再用高质量模型精排
//   - 最后限制返回数量
//
// 使用示例：
//
//	reranker := NewChainReranker(
//	    NewScoreReranker(WithScoreMin(0.5)),  // 先过滤低分
//	    NewCohereReranker("api-key"),          // 再用 Cohere 精排
//	    NewScoreReranker(WithScoreTopK(5)),    // 最后取 Top 5
//	)
//	result, err := reranker.Rerank(ctx, "query", docs)
type ChainReranker struct {
	// rerankers 重排序器列表，按顺序执行
	rerankers []Reranker
}

// ChainOption ChainReranker 选项函数
type ChainOption func(*ChainReranker)

// WithChainRerankers 添加重排序器
//
// 参数：
//   - rerankers: 要添加的重排序器列表
func WithChainRerankers(rerankers ...Reranker) ChainOption {
	return func(c *ChainReranker) {
		c.rerankers = append(c.rerankers, rerankers...)
	}
}

// NewChainReranker 创建链式重排序器
//
// 参数：
//   - rerankers: 按执行顺序排列的重排序器
//
// 返回：
//   - 配置好的 ChainReranker 实例
//
// 使用示例：
//
//	r := NewChainReranker(
//	    NewScoreReranker(WithScoreMin(0.3)),
//	    NewLLMReranker(llm, WithLLMRerankerTopK(10)),
//	)
func NewChainReranker(rerankers ...Reranker) *ChainReranker {
	return &ChainReranker{
		rerankers: rerankers,
	}
}

// Name 返回重排序器名称
//
// 返回格式：ChainReranker(A -> B -> C)
func (c *ChainReranker) Name() string {
	if len(c.rerankers) == 0 {
		return "ChainReranker"
	}

	names := make([]string, len(c.rerankers))
	for i, r := range c.rerankers {
		names[i] = r.Name()
	}

	return "ChainReranker(" + strings.Join(names, " -> ") + ")"
}

// Rerank 按顺序执行所有重排序器
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表
//   - 错误信息（遇到第一个错误即返回）
func (c *ChainReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	// 空链直接返回原文档
	if len(c.rerankers) == 0 {
		return docs, nil
	}

	// 依次执行每个重排序器
	result := docs
	var err error

	for _, reranker := range c.rerankers {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err = reranker.Rerank(ctx, query, result)
		if err != nil {
			return nil, err
		}

		// 如果中间结果为空，提前返回
		if len(result) == 0 {
			return result, nil
		}
	}

	return result, nil
}

// Add 向链中添加重排序器
//
// 参数：
//   - reranker: 要添加的重排序器
//
// 返回：
//   - 返回自身，支持链式调用
func (c *ChainReranker) Add(reranker Reranker) *ChainReranker {
	c.rerankers = append(c.rerankers, reranker)
	return c
}

// Prepend 在链头部添加重排序器
//
// 参数：
//   - reranker: 要添加的重排序器
//
// 返回：
//   - 返回自身，支持链式调用
func (c *ChainReranker) Prepend(reranker Reranker) *ChainReranker {
	c.rerankers = append([]Reranker{reranker}, c.rerankers...)
	return c
}

// Len 返回链中重排序器的数量
func (c *ChainReranker) Len() int {
	return len(c.rerankers)
}

// Rerankers 返回链中所有重排序器的副本
func (c *ChainReranker) Rerankers() []Reranker {
	result := make([]Reranker, len(c.rerankers))
	copy(result, c.rerankers)
	return result
}

// 确保实现 Reranker 接口
var _ Reranker = (*ChainReranker)(nil)
