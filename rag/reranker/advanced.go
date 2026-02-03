// Package reranker 提供高级重排序算法
package reranker

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/everyday-items/hexagon/rag"
)

// ============== MMRReranker ==============

// MMRReranker 最大边际相关性重排序器
//
// MMR (Maximal Marginal Relevance) 是一种平衡相关性和多样性的算法：
//   - 选择与查询最相关的文档
//   - 同时确保文档之间具有多样性（避免冗余）
//   - 使用 lambda 参数控制相关性和多样性的权重
//
// 算法：
//  1. 选择相关性最高的文档加入结果
//  2. 对于剩余文档，计算 MMR 分数：
//     MMR = λ × Sim(Doc, Query) - (1-λ) × max(Sim(Doc, Selected))
//  3. 选择 MMR 分数最高的文档加入结果
//  4. 重复直到达到 TopK
type MMRReranker struct {
	lambda    float32 // 相关性权重 (0-1)，越大越注重相关性，越小越注重多样性
	topK      int     // 返回数量
	embedder  Embedder // 向量嵌入器，用于计算文档相似度
	useCache  bool    // 是否缓存嵌入向量

	// 相似度缓存（优化性能）
	simCache map[string]float32
	mu       sync.RWMutex
}

// Embedder 向量嵌入器接口
type Embedder interface {
	EmbedOne(ctx context.Context, text string) ([]float32, error)
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// MMROption MMRReranker 选项
type MMROption func(*MMRReranker)

// WithMMRLambda 设置相关性权重
func WithMMRLambda(lambda float32) MMROption {
	return func(r *MMRReranker) {
		r.lambda = lambda
	}
}

// WithMMRTopK 设置返回数量
func WithMMRTopK(k int) MMROption {
	return func(r *MMRReranker) {
		r.topK = k
	}
}

// WithMMREmbedder 设置嵌入器
func WithMMREmbedder(embedder Embedder) MMROption {
	return func(r *MMRReranker) {
		r.embedder = embedder
	}
}

// WithMMRCache 设置是否缓存嵌入向量
func WithMMRCache(cache bool) MMROption {
	return func(r *MMRReranker) {
		r.useCache = cache
	}
}

// NewMMRReranker 创建 MMR 重排序器
func NewMMRReranker(embedder Embedder, opts ...MMROption) *MMRReranker {
	r := &MMRReranker{
		lambda:   0.5, // 默认平衡相关性和多样性
		topK:     10,
		embedder: embedder,
		useCache: true,
		simCache: make(map[string]float32),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 使用 MMR 算法重排序
func (r *MMRReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 获取查询向量
	queryEmbed, err := r.embedder.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// 获取所有文档的向量
	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.Content
	}

	docEmbeds, err := r.embedder.Embed(ctx, docContents)
	if err != nil {
		return nil, fmt.Errorf("failed to embed documents: %w", err)
	}

	// 计算所有文档与查询的相似度
	querySims := make([]float32, len(docs))
	for i, docEmbed := range docEmbeds {
		querySims[i] = cosineSimilarity(queryEmbed, docEmbed)
	}

	// MMR 选择
	selected := make([]int, 0, r.topK)
	remaining := make(map[int]bool)
	for i := range docs {
		remaining[i] = true
	}

	// 第一步：选择相关性最高的文档
	maxIdx := 0
	maxSim := querySims[0]
	for i := 1; i < len(docs); i++ {
		if querySims[i] > maxSim {
			maxSim = querySims[i]
			maxIdx = i
		}
	}
	selected = append(selected, maxIdx)
	delete(remaining, maxIdx)

	// 迭代选择剩余文档
	for len(selected) < r.topK && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := float32(-1.0)

		for idx := range remaining {
			// 计算相关性部分: λ × Sim(Doc, Query)
			relevance := r.lambda * querySims[idx]

			// 计算多样性部分: (1-λ) × max(Sim(Doc, Selected))
			maxSelectedSim := float32(0.0)
			for _, selIdx := range selected {
				// 使用缓存的相似度（如果启用）
				sim := r.getCachedSimilarity(idx, selIdx, docEmbeds)
				if sim > maxSelectedSim {
					maxSelectedSim = sim
				}
			}
			diversity := (1.0 - r.lambda) * maxSelectedSim

			// MMR 分数
			mmr := relevance - diversity

			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = idx
			}
		}

		if bestIdx != -1 {
			selected = append(selected, bestIdx)
			delete(remaining, bestIdx)
		} else {
			break
		}
	}

	// 构建结果
	result := make([]rag.Document, len(selected))
	for i, idx := range selected {
		doc := docs[idx]
		doc.Score = querySims[idx] // 使用查询相似度作为分数
		result[i] = doc
	}

	return result, nil
}

// Name 返回重排序器名称
func (r *MMRReranker) Name() string {
	return "MMRReranker"
}

// getCachedSimilarity 获取缓存的相似度
func (r *MMRReranker) getCachedSimilarity(i, j int, embeds [][]float32) float32 {
	if !r.useCache {
		return cosineSimilarity(embeds[i], embeds[j])
	}

	// 确保 i < j（规范化缓存键）
	if i > j {
		i, j = j, i
	}

	cacheKey := fmt.Sprintf("%d-%d", i, j)

	// 尝试从缓存读取
	r.mu.RLock()
	if sim, ok := r.simCache[cacheKey]; ok {
		r.mu.RUnlock()
		return sim
	}
	r.mu.RUnlock()

	// 计算并缓存
	sim := cosineSimilarity(embeds[i], embeds[j])

	r.mu.Lock()
	r.simCache[cacheKey] = sim
	r.mu.Unlock()

	return sim
}

var _ Reranker = (*MMRReranker)(nil)

// ============== ContextualCompressionReranker ==============

// ContextualCompressionReranker 上下文压缩重排序器
//
// 通过 LLM 提取文档中与查询最相关的部分，压缩上下文：
//   - 减少 token 使用
//   - 提高信息密度
//   - 去除无关内容
type ContextualCompressionReranker struct {
	llm             LLMProvider
	topK            int
	compressionRate float32 // 压缩率 (0-1)，0.5 表示压缩到 50%
	minLength       int     // 最小保留长度
}

// CompressionOption ContextualCompressionReranker 选项
type CompressionOption func(*ContextualCompressionReranker)

// WithCompressionTopK 设置返回数量
func WithCompressionTopK(k int) CompressionOption {
	return func(r *ContextualCompressionReranker) {
		r.topK = k
	}
}

// WithCompressionRate 设置压缩率
func WithCompressionRate(rate float32) CompressionOption {
	return func(r *ContextualCompressionReranker) {
		r.compressionRate = rate
	}
}

// WithCompressionMinLength 设置最小保留长度
func WithCompressionMinLength(length int) CompressionOption {
	return func(r *ContextualCompressionReranker) {
		r.minLength = length
	}
}

// NewContextualCompressionReranker 创建上下文压缩重排序器
func NewContextualCompressionReranker(llm LLMProvider, opts ...CompressionOption) *ContextualCompressionReranker {
	r := &ContextualCompressionReranker{
		llm:             llm,
		topK:            10,
		compressionRate: 0.5,
		minLength:       100,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 压缩并重排序文档
func (r *ContextualCompressionReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 限制处理数量
	k := r.topK
	if k > len(docs) {
		k = len(docs)
	}

	// 只处理前 k 个文档（假设输入已排序）
	result := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := docs[i]

		// 计算目标长度
		targetLength := int(float32(len(doc.Content)) * r.compressionRate)
		if targetLength < r.minLength {
			targetLength = r.minLength
		}

		// 如果文档已经很短，跳过压缩
		if len(doc.Content) <= targetLength {
			result[i] = doc
			continue
		}

		// 使用 LLM 压缩
		compressed, err := r.compressDocument(ctx, query, doc.Content, targetLength)
		if err != nil {
			// 压缩失败，保留原文档
			result[i] = doc
			continue
		}

		// 更新文档内容
		doc.Content = compressed
		result[i] = doc
	}

	return result, nil
}

func (r *ContextualCompressionReranker) compressDocument(ctx context.Context, query, content string, targetLength int) (string, error) {
	prompt := fmt.Sprintf(`Extract the most relevant information from the following document that helps answer the query.
Keep only the essential information and aim for approximately %d characters.

Query: %s

Document:
%s

Extracted relevant information:`, targetLength, query, truncateText(content, 2000))

	resp, err := r.llm.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	compressed := strings.TrimSpace(resp)
	if compressed == "" {
		return content, nil
	}

	return compressed, nil
}

// Name 返回重排序器名称
func (r *ContextualCompressionReranker) Name() string {
	return "ContextualCompressionReranker"
}

var _ Reranker = (*ContextualCompressionReranker)(nil)

// ============== LostInTheMiddleReranker ==============

// LostInTheMiddleReranker "中间丢失"重排序器
//
// 研究表明，LLM 在处理长上下文时，对开头和结尾的信息关注度更高，
// 而中间部分的信息容易被忽略（Lost in the Middle）。
//
// 该重排序器将最重要的文档放在开头和结尾，次重要的放在中间：
//   位置分布: [最重要, 次重要, ..., 最不重要, ..., 次重要, 重要]
//
// 例如，输入 [1, 2, 3, 4, 5, 6] (按相关性降序)
// 输出: [1, 3, 5, 6, 4, 2]
type LostInTheMiddleReranker struct {
	topK     int
	strategy string // 重排策略: alternate（交替）, bookend（首尾加强）
}

// LostInTheMiddleOption LostInTheMiddleReranker 选项
type LostInTheMiddleOption func(*LostInTheMiddleReranker)

// WithLostInTheMiddleTopK 设置返回数量
func WithLostInTheMiddleTopK(k int) LostInTheMiddleOption {
	return func(r *LostInTheMiddleReranker) {
		r.topK = k
	}
}

// WithLostInTheMiddleStrategy 设置重排策略
func WithLostInTheMiddleStrategy(strategy string) LostInTheMiddleOption {
	return func(r *LostInTheMiddleReranker) {
		r.strategy = strategy
	}
}

// NewLostInTheMiddleReranker 创建"中间丢失"重排序器
func NewLostInTheMiddleReranker(opts ...LostInTheMiddleOption) *LostInTheMiddleReranker {
	r := &LostInTheMiddleReranker{
		topK:     10,
		strategy: "alternate",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 重排序以对抗"中间丢失"问题
func (r *LostInTheMiddleReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 确保输入按相关性排序（降序）
	sorted := make([]rag.Document, len(docs))
	copy(sorted, docs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// 限制数量
	k := r.topK
	if k > len(sorted) {
		k = len(sorted)
	}
	sorted = sorted[:k]

	// 根据策略重排
	var reranked []rag.Document
	switch r.strategy {
	case "alternate":
		reranked = r.alternateRerank(sorted)
	case "bookend":
		reranked = r.bookendRerank(sorted)
	default:
		reranked = sorted
	}

	return reranked, nil
}

// alternateRerank 交替重排：重要文档交替放在首尾
//
// 输入: [1, 2, 3, 4, 5, 6] (相关性降序)
// 输出: [1, 3, 5, 6, 4, 2] (首尾加强，中间减弱)
func (r *LostInTheMiddleReranker) alternateRerank(docs []rag.Document) []rag.Document {
	n := len(docs)
	if n <= 2 {
		return docs
	}

	result := make([]rag.Document, n)
	left := 0
	right := n - 1
	toFront := true

	for i := 0; i < n; i++ {
		if toFront {
			result[left] = docs[i]
			left++
		} else {
			result[right] = docs[i]
			right--
		}
		toFront = !toFront
	}

	return result
}

// bookendRerank 首尾加强：最重要的文档在首尾
//
// 输入: [1, 2, 3, 4, 5, 6] (相关性降序)
// 输出: [1, 2, 4, 5, 3, 6]
//
// 策略：
//   - 前 1/3 最重要文档放在开头
//   - 中间 1/3 不重要文档放在中间
//   - 后 1/3 次重要文档放在结尾
func (r *LostInTheMiddleReranker) bookendRerank(docs []rag.Document) []rag.Document {
	n := len(docs)
	if n <= 3 {
		return docs
	}

	// 分成三部分
	third := n / 3
	front := docs[:third]           // 最重要 - 放开头
	middle := docs[third : 2*third] // 中等 - 放中间
	back := docs[2*third:]          // 次重要 - 放结尾

	// 重组: 开头 + 中间 + 结尾
	result := make([]rag.Document, 0, n)
	result = append(result, front...)
	result = append(result, middle...)
	result = append(result, back...)

	return result
}

// Name 返回重排序器名称
func (r *LostInTheMiddleReranker) Name() string {
	return "LostInTheMiddleReranker"
}

var _ Reranker = (*LostInTheMiddleReranker)(nil)

// ============== DiversityReranker ==============

// DiversityReranker 多样性重排序器
//
// 确保结果集具有多样性，避免返回过于相似的文档。
// 使用贪心算法选择：
//   1. 与查询相关
//   2. 与已选文档不太相似
type DiversityReranker struct {
	topK          int
	embedder      Embedder
	threshold     float32 // 相似度阈值，超过则认为过于相似
	balanceFactor float32 // 相关性与多样性的平衡因子
}

// DiversityOption DiversityReranker 选项
type DiversityOption func(*DiversityReranker)

// WithDiversityTopK 设置返回数量
func WithDiversityTopK(k int) DiversityOption {
	return func(r *DiversityReranker) {
		r.topK = k
	}
}

// WithDiversityThreshold 设置相似度阈值
func WithDiversityThreshold(threshold float32) DiversityOption {
	return func(r *DiversityReranker) {
		r.threshold = threshold
	}
}

// WithDiversityBalance 设置平衡因子
func WithDiversityBalance(factor float32) DiversityOption {
	return func(r *DiversityReranker) {
		r.balanceFactor = factor
	}
}

// NewDiversityReranker 创建多样性重排序器
func NewDiversityReranker(embedder Embedder, opts ...DiversityOption) *DiversityReranker {
	r := &DiversityReranker{
		topK:          10,
		embedder:      embedder,
		threshold:     0.9, // 相似度超过 0.9 视为过于相似
		balanceFactor: 0.5,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 多样性重排序
func (r *DiversityReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 获取文档向量
	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.Content
	}

	docEmbeds, err := r.embedder.Embed(ctx, docContents)
	if err != nil {
		return nil, fmt.Errorf("failed to embed documents: %w", err)
	}

	// 贪心选择
	selected := make([]int, 0, r.topK)
	remaining := make(map[int]bool)
	for i := range docs {
		remaining[i] = true
	}

	// 第一步：选择相关性最高的文档
	maxIdx := 0
	maxScore := docs[0].Score
	for i := 1; i < len(docs); i++ {
		if docs[i].Score > maxScore {
			maxScore = docs[i].Score
			maxIdx = i
		}
	}
	selected = append(selected, maxIdx)
	delete(remaining, maxIdx)

	// 迭代选择
	for len(selected) < r.topK && len(remaining) > 0 {
		bestIdx := -1
		bestScore := float32(-1.0)

		for idx := range remaining {
			// 计算与已选文档的最大相似度
			maxSim := float32(0.0)
			for _, selIdx := range selected {
				sim := cosineSimilarity(docEmbeds[idx], docEmbeds[selIdx])
				if sim > maxSim {
					maxSim = sim
				}
			}

			// 如果过于相似，跳过
			if maxSim > r.threshold {
				continue
			}

			// 综合得分：相关性 - 相似度惩罚
			score := docs[idx].Score - r.balanceFactor*maxSim

			if score > bestScore {
				bestScore = score
				bestIdx = idx
			}
		}

		if bestIdx != -1 {
			selected = append(selected, bestIdx)
			delete(remaining, bestIdx)
		} else {
			// 没有找到不相似的文档，放松阈值
			for idx := range remaining {
				selected = append(selected, idx)
				if len(selected) >= r.topK {
					break
				}
			}
			break
		}
	}

	// 构建结果
	result := make([]rag.Document, len(selected))
	for i, idx := range selected {
		result[i] = docs[idx]
	}

	return result, nil
}

// Name 返回重排序器名称
func (r *DiversityReranker) Name() string {
	return "DiversityReranker"
}

var _ Reranker = (*DiversityReranker)(nil)

// ============== 辅助函数 ==============

// cosineSimilarity 计算余弦相似度
//
// 优化版本：减少平方根计算次数
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	// 使用 float32(math.Sqrt) 只调用一次
	denominator := float32(math.Sqrt(float64(normA * normB)))
	if denominator == 0 {
		return 0
	}

	return dotProduct / denominator
}
