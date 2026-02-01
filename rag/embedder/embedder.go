// Package embedder 提供 RAG 系统的文本嵌入生成器
//
// Embedder 用于将文本转换为向量：
//   - OpenAIEmbedder: 使用 OpenAI Embedding API
//   - CachedEmbedder: 带缓存的 Embedder 包装器
//   - BatchEmbedder: 批量处理的 Embedder 包装器
package embedder

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/everyday-items/hexagon/store/vector"
)

// ============== OpenAIEmbedder ==============

// EmbeddingProvider 嵌入提供者接口
// 通常由 ai-core 的 llm.Provider 实现
type EmbeddingProvider interface {
	// Embed 将文本转换为向量
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// OpenAIEmbedder OpenAI Embedding 生成器
type OpenAIEmbedder struct {
	provider  EmbeddingProvider
	model     string
	dimension int
	batchSize int
}

// OpenAIOption OpenAIEmbedder 选项
type OpenAIOption func(*OpenAIEmbedder)

// WithModel 设置模型
func WithModel(model string) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.model = model
	}
}

// WithDimension 设置向量维度
func WithDimension(dim int) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.dimension = dim
	}
}

// WithBatchSize 设置批量大小
func WithBatchSize(size int) OpenAIOption {
	return func(e *OpenAIEmbedder) {
		e.batchSize = size
	}
}

// NewOpenAIEmbedder 创建 OpenAI Embedder
func NewOpenAIEmbedder(provider EmbeddingProvider, opts ...OpenAIOption) *OpenAIEmbedder {
	e := &OpenAIEmbedder{
		provider:  provider,
		model:     "text-embedding-3-small",
		dimension: 1536,
		batchSize: 100,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Embed 将文本列表转换为向量
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// 分批处理
	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += e.batchSize {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		end := i + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		embeddings, err := e.provider.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to embed batch %d-%d: %w", i, end, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)
	}

	return allEmbeddings, nil
}

// EmbedOne 将单个文本转换为向量
func (e *OpenAIEmbedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// Dimension 返回向量维度
func (e *OpenAIEmbedder) Dimension() int {
	return e.dimension
}

var _ vector.Embedder = (*OpenAIEmbedder)(nil)

// ============== CachedEmbedder ==============

// CachedEmbedder 带缓存的 Embedder
type CachedEmbedder struct {
	embedder vector.Embedder
	cache    map[string][]float32
	mu       sync.RWMutex
	maxSize  int
}

// CacheOption CachedEmbedder 选项
type CacheOption func(*CachedEmbedder)

// WithMaxCacheSize 设置最大缓存大小
func WithMaxCacheSize(size int) CacheOption {
	return func(e *CachedEmbedder) {
		e.maxSize = size
	}
}

// NewCachedEmbedder 创建带缓存的 Embedder
func NewCachedEmbedder(embedder vector.Embedder, opts ...CacheOption) *CachedEmbedder {
	e := &CachedEmbedder{
		embedder: embedder,
		cache:    make(map[string][]float32),
		maxSize:  10000,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Embed 将文本列表转换为向量（带缓存）
func (e *CachedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	var toEmbed []string
	var toEmbedIdx []int

	e.mu.RLock()
	for i, text := range texts {
		key := hashText(text)
		if cached, ok := e.cache[key]; ok {
			result[i] = cached
		} else {
			toEmbed = append(toEmbed, text)
			toEmbedIdx = append(toEmbedIdx, i)
		}
	}
	e.mu.RUnlock()

	if len(toEmbed) > 0 {
		embeddings, err := e.embedder.Embed(ctx, toEmbed)
		if err != nil {
			return nil, err
		}

		e.mu.Lock()
		for i, embedding := range embeddings {
			idx := toEmbedIdx[i]
			result[idx] = embedding

			// 添加到缓存
			if len(e.cache) < e.maxSize {
				e.cache[hashText(toEmbed[i])] = embedding
			}
		}
		e.mu.Unlock()
	}

	return result, nil
}

// EmbedOne 将单个文本转换为向量
func (e *CachedEmbedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// Dimension 返回向量维度
func (e *CachedEmbedder) Dimension() int {
	return e.embedder.Dimension()
}

// CacheSize 返回缓存大小
func (e *CachedEmbedder) CacheSize() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.cache)
}

// ClearCache 清空缓存
func (e *CachedEmbedder) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string][]float32)
}

var _ vector.Embedder = (*CachedEmbedder)(nil)

// ============== MockEmbedder ==============

// MockEmbedder 模拟 Embedder（用于测试）
type MockEmbedder struct {
	dimension int
	fixedVec  []float32
}

// NewMockEmbedder 创建模拟 Embedder
func NewMockEmbedder(dimension int) *MockEmbedder {
	// 创建一个固定向量
	vec := make([]float32, dimension)
	for i := range vec {
		vec[i] = float32(i) / float32(dimension)
	}
	return &MockEmbedder{
		dimension: dimension,
		fixedVec:  vec,
	}
}

// Embed 模拟嵌入
func (e *MockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		// 基于文本内容生成不同的向量
		vec := make([]float32, e.dimension)
		hash := hashText(text)
		for j := 0; j < e.dimension && j < len(hash); j++ {
			vec[j] = float32(hash[j]) / 255.0
		}
		// 填充剩余维度
		for j := len(hash); j < e.dimension; j++ {
			vec[j] = e.fixedVec[j]
		}
		result[i] = vec
	}
	return result, nil
}

// EmbedOne 模拟嵌入单个文本
func (e *MockEmbedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// Dimension 返回向量维度
func (e *MockEmbedder) Dimension() int {
	return e.dimension
}

var _ vector.Embedder = (*MockEmbedder)(nil)

// ============== FuncEmbedder ==============

// FuncEmbedder 函数式 Embedder
type FuncEmbedder struct {
	embedFn   func(ctx context.Context, texts []string) ([][]float32, error)
	dimension int
}

// NewFuncEmbedder 创建函数式 Embedder
func NewFuncEmbedder(dimension int, fn func(ctx context.Context, texts []string) ([][]float32, error)) *FuncEmbedder {
	return &FuncEmbedder{
		embedFn:   fn,
		dimension: dimension,
	}
}

// Embed 调用函数生成向量
func (e *FuncEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embedFn(ctx, texts)
}

// EmbedOne 嵌入单个文本
func (e *FuncEmbedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// Dimension 返回向量维度
func (e *FuncEmbedder) Dimension() int {
	return e.dimension
}

var _ vector.Embedder = (*FuncEmbedder)(nil)

// ============== 辅助函数 ==============

// hashText 计算文本的 MD5 哈希
func hashText(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
