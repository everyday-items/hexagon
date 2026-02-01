// Package splitter 提供 RAG 系统的语义文本分割器

package splitter

import (
	"context"
	"math"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// Embedder 是向量生成器接口（简化版，避免循环依赖）
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ============== SemanticSplitter ==============

// SemanticSplitter 语义分割器
// 基于 embedding 相似度进行智能分割，在语义边界处分割文档
type SemanticSplitter struct {
	embedder           Embedder
	bufferSize         int     // 句子缓冲区大小（用于计算相似度）
	breakpointThreshold float64 // 断点阈值（低于此值视为语义边界）
	minChunkSize       int     // 最小块大小
	maxChunkSize       int     // 最大块大小
	sentenceEnds       []string
}

// SemanticOption SemanticSplitter 选项
type SemanticOption func(*SemanticSplitter)

// WithSemanticBufferSize 设置句子缓冲区大小
func WithSemanticBufferSize(size int) SemanticOption {
	return func(s *SemanticSplitter) {
		s.bufferSize = size
	}
}

// WithSemanticBreakpointThreshold 设置断点阈值
func WithSemanticBreakpointThreshold(threshold float64) SemanticOption {
	return func(s *SemanticSplitter) {
		s.breakpointThreshold = threshold
	}
}

// WithSemanticMinChunkSize 设置最小块大小
func WithSemanticMinChunkSize(size int) SemanticOption {
	return func(s *SemanticSplitter) {
		s.minChunkSize = size
	}
}

// WithSemanticMaxChunkSize 设置最大块大小
func WithSemanticMaxChunkSize(size int) SemanticOption {
	return func(s *SemanticSplitter) {
		s.maxChunkSize = size
	}
}

// WithSemanticSentenceEnds 设置句子结束符
func WithSemanticSentenceEnds(ends []string) SemanticOption {
	return func(s *SemanticSplitter) {
		s.sentenceEnds = ends
	}
}

// NewSemanticSplitter 创建语义分割器
func NewSemanticSplitter(embedder Embedder, opts ...SemanticOption) *SemanticSplitter {
	s := &SemanticSplitter{
		embedder:           embedder,
		bufferSize:         3,   // 前后各考虑 3 个句子
		breakpointThreshold: 0.3, // 相似度低于 0.3 视为边界
		minChunkSize:       100,
		maxChunkSize:       2000,
		sentenceEnds:       []string{"。", "！", "？", ".", "!", "?"},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 语义分割文档
func (s *SemanticSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chunks, err := s.splitDocument(ctx, doc)
		if err != nil {
			return nil, err
		}
		result = append(result, chunks...)
	}

	return result, nil
}

func (s *SemanticSplitter) splitDocument(ctx context.Context, doc rag.Document) ([]rag.Document, error) {
	// 1. 先按句子分割
	sentences := s.splitToSentences(doc.Content)
	if len(sentences) <= 1 {
		return []rag.Document{doc}, nil
	}

	// 2. 计算每个句子的 embedding
	embeddings, err := s.embedder.Embed(ctx, sentences)
	if err != nil {
		return nil, err
	}

	// 3. 计算相邻句子的相似度
	similarities := s.calculateSimilarities(embeddings)

	// 4. 找到语义边界（相似度低于阈值的位置）
	breakpoints := s.findBreakpoints(similarities)

	// 5. 根据边界分割
	chunks := s.splitByBreakpoints(sentences, breakpoints)

	// 6. 合并过小的块，分割过大的块
	chunks = s.adjustChunks(chunks)

	// 7. 创建文档
	result := make([]rag.Document, len(chunks))
	for i, chunk := range chunks {
		result[i] = rag.Document{
			ID:      util.GenerateID("chunk"),
			Content: chunk,
			Source:  doc.Source,
			Metadata: copyMetadata(doc.Metadata, map[string]any{
				"chunk_index":     i,
				"chunk_total":     len(chunks),
				"parent_id":       doc.ID,
				"splitter":        "semantic",
				"sentence_count":  countSentences(chunk, s.sentenceEnds),
			}),
			CreatedAt: time.Now(),
		}
	}

	return result, nil
}

func (s *SemanticSplitter) splitToSentences(text string) []string {
	var sentences []string
	var current []rune
	runes := []rune(text)

	for i, r := range runes {
		current = append(current, r)

		// 检查是否是句子结束
		if s.isSentenceEnd(r) {
			// 检查是否真的是句子结束（排除缩写等情况）
			if s.isRealSentenceEnd(runes, i) {
				sentence := string(current)
				if len(sentence) > 0 {
					sentences = append(sentences, sentence)
				}
				current = nil
			}
		}
	}

	// 添加最后一个句子
	if len(current) > 0 {
		sentences = append(sentences, string(current))
	}

	return sentences
}

func (s *SemanticSplitter) isSentenceEnd(r rune) bool {
	for _, end := range s.sentenceEnds {
		if string(r) == end {
			return true
		}
	}
	return false
}

func (s *SemanticSplitter) isRealSentenceEnd(runes []rune, pos int) bool {
	// 简单启发式：如果后面是空格或换行，或者到达末尾，认为是真正的句子结束
	if pos >= len(runes)-1 {
		return true
	}

	next := runes[pos+1]
	return next == ' ' || next == '\n' || next == '\r' || next == '\t'
}

func (s *SemanticSplitter) calculateSimilarities(embeddings [][]float32) []float64 {
	if len(embeddings) < 2 {
		return nil
	}

	similarities := make([]float64, len(embeddings)-1)

	for i := 0; i < len(embeddings)-1; i++ {
		// 计算当前句子和下一句子的相似度
		// 使用缓冲区：比较前后 bufferSize 个句子的平均 embedding
		leftEmb := s.averageEmbedding(embeddings, max(0, i-s.bufferSize+1), i+1)
		rightEmb := s.averageEmbedding(embeddings, i+1, min(len(embeddings), i+1+s.bufferSize))

		similarities[i] = cosineSimilarity(leftEmb, rightEmb)
	}

	return similarities
}

func (s *SemanticSplitter) averageEmbedding(embeddings [][]float32, start, end int) []float32 {
	if start >= end || len(embeddings) == 0 {
		return embeddings[0]
	}

	dim := len(embeddings[0])
	avg := make([]float32, dim)
	count := float32(end - start)

	for i := start; i < end; i++ {
		for j := 0; j < dim; j++ {
			avg[j] += embeddings[i][j]
		}
	}

	for j := 0; j < dim; j++ {
		avg[j] /= count
	}

	return avg
}

func (s *SemanticSplitter) findBreakpoints(similarities []float64) []int {
	if len(similarities) == 0 {
		return nil
	}

	// 计算统计信息
	mean, stdDev := calculateStats(similarities)

	// 动态阈值：均值 - 标准差，但不低于固定阈值
	threshold := math.Max(mean-stdDev, s.breakpointThreshold)

	var breakpoints []int
	for i, sim := range similarities {
		if sim < threshold {
			breakpoints = append(breakpoints, i+1) // i+1 是分割点位置
		}
	}

	return breakpoints
}

func (s *SemanticSplitter) splitByBreakpoints(sentences []string, breakpoints []int) []string {
	if len(breakpoints) == 0 {
		// 没有断点，合并所有句子
		return []string{joinSentences(sentences)}
	}

	var chunks []string
	start := 0

	for _, bp := range breakpoints {
		if bp > start && bp <= len(sentences) {
			chunk := joinSentences(sentences[start:bp])
			if len(chunk) > 0 {
				chunks = append(chunks, chunk)
			}
			start = bp
		}
	}

	// 添加最后一个块
	if start < len(sentences) {
		chunk := joinSentences(sentences[start:])
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

func (s *SemanticSplitter) adjustChunks(chunks []string) []string {
	var result []string

	for _, chunk := range chunks {
		chunkLen := len([]rune(chunk))

		if chunkLen < s.minChunkSize && len(result) > 0 {
			// 合并到前一个块
			result[len(result)-1] += " " + chunk
		} else if chunkLen > s.maxChunkSize {
			// 分割过大的块（使用递归分割）
			subSplitter := NewRecursiveSplitter(
				WithRecursiveChunkSize(s.maxChunkSize),
				WithRecursiveChunkOverlap(0),
			)
			subChunks := subSplitter.splitTextRecursive(chunk, []string{"\n\n", "\n", "。", ".", " "})
			result = append(result, subChunks...)
		} else {
			result = append(result, chunk)
		}
	}

	return result
}

// Name 返回分割器名称
func (s *SemanticSplitter) Name() string {
	return "SemanticSplitter"
}

var _ rag.Splitter = (*SemanticSplitter)(nil)

// ============== 辅助函数 ==============

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// calculateStats 计算均值和标准差
func calculateStats(values []float64) (mean, stdDev float64) {
	if len(values) == 0 {
		return 0, 0
	}

	// 计算均值
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	// 计算标准差
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	stdDev = math.Sqrt(sumSquares / float64(len(values)))

	return mean, stdDev
}

// joinSentences 连接句子
func joinSentences(sentences []string) string {
	result := ""
	for i, s := range sentences {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}

// countSentences 统计句子数量
func countSentences(text string, ends []string) int {
	count := 0
	for _, r := range text {
		for _, end := range ends {
			if string(r) == end {
				count++
				break
			}
		}
	}
	if count == 0 {
		count = 1 // 至少一个句子
	}
	return count
}

// max 返回较大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min 返回较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
