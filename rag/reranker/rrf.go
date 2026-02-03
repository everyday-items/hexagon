// Package reranker 提供文档重排序功能
package reranker

import (
	"context"
	"sort"

	"github.com/everyday-items/hexagon/rag"
)

// RRFReranker 倒数排名融合（Reciprocal Rank Fusion）重排序器
//
// RRF 是一种简单有效的排名融合算法，常用于混合检索场景：
//   - 融合来自不同检索系统的结果（如向量检索 + 关键词检索）
//   - 不依赖原始分数，只使用排名位置
//   - 对异常分数不敏感
//
// 算法公式：
//
//	RRF(d) = Σ 1/(k + rank(d))
//
// 其中 k 是常数（通常为 60），rank(d) 是文档 d 在某个排名列表中的位置。
//
// 使用示例：
//
//	reranker := NewRRFReranker(
//	    WithRRFK(60),
//	    WithRRFTopK(10),
//	)
//	// 融合多个排名列表
//	result := reranker.FuseRankings(vectorResults, keywordResults)
type RRFReranker struct {
	// k RRF 参数，控制排名的平滑程度
	// 较大的 k 值使排名差异更平滑
	k float64

	// topK 返回数量
	topK int
}

// RRFOption RRFReranker 选项函数
type RRFOption func(*RRFReranker)

// WithRRFK 设置 RRF 参数 k
//
// 参数：
//   - k: RRF 参数，默认 60。较大的值使排名差异更平滑
//
// 推荐值：
//   - k=60: 经典设置，适合大多数场景
//   - k=1-10: 更强调头部排名
//   - k=100+: 更平滑，减少头部优势
func WithRRFK(k float64) RRFOption {
	return func(r *RRFReranker) {
		r.k = k
	}
}

// WithRRFTopK 设置返回数量
//
// 参数：
//   - topK: 返回的最相关文档数量
func WithRRFTopK(topK int) RRFOption {
	return func(r *RRFReranker) {
		r.topK = topK
	}
}

// NewRRFReranker 创建 RRF 重排序器
//
// 参数：
//   - opts: 可选配置项
//
// 返回：
//   - 配置好的 RRFReranker 实例
//
// 使用示例：
//
//	r := NewRRFReranker(
//	    WithRRFK(60),
//	    WithRRFTopK(10),
//	)
func NewRRFReranker(opts ...RRFOption) *RRFReranker {
	r := &RRFReranker{
		k:    60, // 经典 RRF 参数
		topK: 10,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Name 返回重排序器名称
func (r *RRFReranker) Name() string {
	return "RRFReranker"
}

// Rerank 对单个文档列表进行 RRF 重排序
//
// 注意：单个列表的 RRF 重排序实际上就是按原始分数排序后应用 RRF 分数。
// 对于融合多个列表，请使用 FuseRankings 方法。
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串（RRF 不使用）
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表
//   - 错误信息
func (r *RRFReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 对单个列表，先按分数排序
	sorted := make([]rag.Document, len(docs))
	copy(sorted, docs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// 应用 RRF 分数
	for i := range sorted {
		// RRF 分数 = 1 / (k + rank)
		// rank 从 1 开始
		sorted[i].Score = float32(1.0 / (r.k + float64(i+1)))
	}

	// 取 TopK
	k := min(r.topK, len(sorted))
	return sorted[:k], nil
}

// FuseRankings 融合多个排名列表
//
// 这是 RRF 的主要用途：融合来自不同检索系统的结果。
//
// 参数：
//   - rankings: 多个排名列表，每个列表应该已按相关性降序排列
//
// 返回：
//   - 融合后的文档列表，按 RRF 分数降序排列
//
// 使用示例：
//
//	vectorResults := vectorRetriever.Retrieve(ctx, query)
//	keywordResults := keywordRetriever.Retrieve(ctx, query)
//	fusedResults := reranker.FuseRankings(vectorResults, keywordResults)
func (r *RRFReranker) FuseRankings(rankings ...[]rag.Document) []rag.Document {
	if len(rankings) == 0 {
		return nil
	}

	// 用于存储每个文档的 RRF 分数
	// key: 文档 ID, value: 累计 RRF 分数
	rrfScores := make(map[string]float64)

	// 存储文档内容（用于构建结果）
	docMap := make(map[string]rag.Document)

	// 计算每个文档的 RRF 分数
	for _, ranking := range rankings {
		for rank, doc := range ranking {
			// 跳过没有 ID 的文档，避免所有无 ID 文档被错误合并
			if doc.ID == "" {
				continue
			}

			// RRF 分数 = 1 / (k + rank)
			// rank 从 1 开始
			score := 1.0 / (r.k + float64(rank+1))
			rrfScores[doc.ID] += score

			// 保存文档（如果还没有）
			if _, exists := docMap[doc.ID]; !exists {
				docMap[doc.ID] = doc
			}
		}
	}

	// 构建结果列表
	type docWithRRF struct {
		doc   rag.Document
		score float64
	}

	results := make([]docWithRRF, 0, len(rrfScores))
	for id, score := range rrfScores {
		doc := docMap[id]
		results = append(results, docWithRRF{doc: doc, score: score})
	}

	// 按 RRF 分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// 取 TopK
	k := min(r.topK, len(results))
	output := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := results[i].doc
		doc.Score = float32(results[i].score)
		output[i] = doc
	}

	return output
}

// 确保实现 Reranker 接口
var _ Reranker = (*RRFReranker)(nil)
