// Package reranker 提供文档重排序功能
package reranker

import (
	"context"
	"sort"

	"github.com/everyday-items/hexagon/rag"
)

// ScoreReranker 分数过滤重排序器
//
// 基于文档原有分数进行过滤和排序：
//   - 过滤低于阈值的文档
//   - 可选分数归一化
//   - 限制返回数量
//
// 使用场景：
//   - 过滤低相关性结果
//   - 统一不同检索系统的分数范围
//   - 简单快速的重排序
//
// 使用示例：
//
//	reranker := NewScoreReranker(
//	    WithScoreMin(0.5),
//	    WithScoreTopK(10),
//	    WithScoreNormalize(true),
//	)
//	result, err := reranker.Rerank(ctx, "query", docs)
type ScoreReranker struct {
	// minScore 最小分数阈值，低于此分数的文档将被过滤
	minScore float32

	// topK 返回数量
	topK int

	// normalize 是否归一化分数到 [0, 1] 范围
	normalize bool
}

// ScoreOption ScoreReranker 选项函数
type ScoreOption func(*ScoreReranker)

// WithScoreMin 设置最小分数阈值
//
// 参数：
//   - min: 最小分数，低于此分数的文档将被过滤。默认 0（不过滤）
//
// 注意：如果同时使用 normalize=true，过滤在归一化之前进行
func WithScoreMin(min float32) ScoreOption {
	return func(r *ScoreReranker) {
		r.minScore = min
	}
}

// WithScoreTopK 设置返回数量
//
// 参数：
//   - k: 返回的最相关文档数量
func WithScoreTopK(k int) ScoreOption {
	return func(r *ScoreReranker) {
		r.topK = k
	}
}

// WithScoreNormalize 设置是否归一化分数
//
// 参数：
//   - normalize: 是否将分数归一化到 [0, 1] 范围
//
// 归一化公式：
//
//	normalized = (score - min) / (max - min)
//
// 其中 min 和 max 是当前结果集中的最小和最大分数
func WithScoreNormalize(normalize bool) ScoreOption {
	return func(r *ScoreReranker) {
		r.normalize = normalize
	}
}

// NewScoreReranker 创建分数过滤重排序器
//
// 参数：
//   - opts: 可选配置项
//
// 返回：
//   - 配置好的 ScoreReranker 实例
//
// 使用示例：
//
//	r := NewScoreReranker(
//	    WithScoreMin(0.5),
//	    WithScoreTopK(10),
//	    WithScoreNormalize(true),
//	)
func NewScoreReranker(opts ...ScoreOption) *ScoreReranker {
	r := &ScoreReranker{
		minScore:  0,
		topK:      10,
		normalize: false,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Name 返回重排序器名称
func (r *ScoreReranker) Name() string {
	return "ScoreReranker"
}

// Rerank 基于分数过滤和排序文档
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串（ScoreReranker 不使用）
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表
//   - 错误信息
func (r *ScoreReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 1. 过滤低分文档
	filtered := r.filterByScore(docs)
	if len(filtered) == 0 {
		return filtered, nil
	}

	// 2. 按分数降序排序
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	// 3. 取 TopK
	k := min(r.topK, len(filtered))
	result := filtered[:k]

	// 4. 可选：归一化分数
	if r.normalize && len(result) > 0 {
		result = r.normalizeScores(result)
	}

	return result, nil
}

// filterByScore 过滤低分文档
func (r *ScoreReranker) filterByScore(docs []rag.Document) []rag.Document {
	if r.minScore <= 0 {
		// 不过滤，但需要复制以避免修改原切片
		result := make([]rag.Document, len(docs))
		copy(result, docs)
		return result
	}

	filtered := make([]rag.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.Score >= r.minScore {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// normalizeScores 归一化分数到 [0, 1] 范围
//
// 公式：normalized = (score - min) / (max - min)
func (r *ScoreReranker) normalizeScores(docs []rag.Document) []rag.Document {
	if len(docs) == 0 {
		return docs
	}

	// 找到最大和最小分数
	minScore := docs[0].Score
	maxScore := docs[0].Score
	for _, doc := range docs[1:] {
		if doc.Score < minScore {
			minScore = doc.Score
		}
		if doc.Score > maxScore {
			maxScore = doc.Score
		}
	}

	// 如果所有分数相同，全部设为 1
	if maxScore == minScore {
		result := make([]rag.Document, len(docs))
		for i, doc := range docs {
			doc.Score = 1.0
			result[i] = doc
		}
		return result
	}

	// 归一化
	scoreRange := maxScore - minScore
	result := make([]rag.Document, len(docs))
	for i, doc := range docs {
		doc.Score = (doc.Score - minScore) / scoreRange
		result[i] = doc
	}

	return result
}

// 确保实现 Reranker 接口
var _ Reranker = (*ScoreReranker)(nil)
