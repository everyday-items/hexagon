// Package cache 提供 RAG 语义缓存功能
//
// 通过缓存相似查询的结果，避免重复检索和生成，提升性能。
// 与传统精确字符串匹配不同，语义缓存基于查询的语义相似度进行匹配：
//   - 使用向量嵌入表示查询语义
//   - 通过余弦相似度判断查询是否命中缓存
//   - 支持 TTL 过期、LRU 淘汰和容量限制
//
// 使用示例：
//
//	sc := cache.New(embedder,
//	    cache.WithMaxSize(500),
//	    cache.WithTTL(30 * time.Minute),
//	    cache.WithThreshold(0.9),
//	)
//
//	// 查找缓存
//	result, hit, err := sc.Get(ctx, "什么是 Go 语言？")
//	if !hit {
//	    // 缓存未命中，执行检索后写入缓存
//	    sc.Put(ctx, "什么是 Go 语言？", result)
//	}
package cache

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Embedder 嵌入器接口（简化版，不依赖 ai-core）
// 将文本转换为向量表示，用于语义相似度计算
type Embedder interface {
	// Embed 将文本转换为向量
	Embed(ctx context.Context, text string) ([]float64, error)
}

// CacheEntry 缓存条目
// 存储查询的向量表示和对应的检索结果
type CacheEntry struct {
	// Query 原始查询文本
	Query string

	// Embedding 查询的向量表示
	Embedding []float64

	// Result 缓存的检索结果
	Result *CacheResult

	// CreatedAt 创建时间
	CreatedAt time.Time

	// HitCount 命中次数（用于统计和 LRU 淘汰参考）
	HitCount int64
}

// CacheResult 缓存结果
// 包含检索到的文档和生成的答案
type CacheResult struct {
	// Documents 缓存的文档列表
	Documents []CachedDocument

	// Answer 缓存的生成答案
	Answer string

	// Metadata 附加元数据
	Metadata map[string]any
}

// CachedDocument 缓存文档
type CachedDocument struct {
	// Content 文档内容
	Content string

	// Score 相关性分数
	Score float64

	// Metadata 文档元数据
	Metadata map[string]any
}

// CacheStats 缓存统计信息
type CacheStats struct {
	// Size 当前缓存条目数
	Size int

	// MaxSize 最大缓存容量
	MaxSize int

	// Hits 命中次数
	Hits int64

	// Misses 未命中次数
	Misses int64

	// HitRate 命中率 (0-1)
	HitRate float64

	// Evictions 淘汰次数
	Evictions int64
}

// SemanticCache 语义缓存
// 基于查询语义相似度进行缓存匹配，而非精确字符串匹配。
// 通过向量嵌入计算余弦相似度，当相似度超过阈值时视为命中。
//
// 线程安全：所有方法都是并发安全的
type SemanticCache struct {
	entries   []*CacheEntry // 缓存条目列表
	embedder  Embedder      // 向量嵌入器
	maxSize   int           // 最大缓存条目数
	ttl       time.Duration // 缓存过期时间
	threshold float64       // 相似度阈值 (0-1)
	mu        sync.RWMutex
	hits      atomic.Int64 // 命中次数
	misses    atomic.Int64 // 未命中次数
	evictions atomic.Int64 // 淘汰次数
}

// Option 语义缓存配置选项
type Option func(*SemanticCache)

// WithMaxSize 设置最大缓存条目数
// 当缓存满时，最早创建的条目会被淘汰
// 默认值: 1000
func WithMaxSize(size int) Option {
	return func(c *SemanticCache) {
		if size > 0 {
			c.maxSize = size
		}
	}
}

// WithTTL 设置缓存过期时间
// 超过 TTL 的条目在查询时会被跳过并清理
// 默认值: 1 小时
func WithTTL(ttl time.Duration) Option {
	return func(c *SemanticCache) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithThreshold 设置相似度阈值
// 只有余弦相似度 >= threshold 的缓存才会被视为命中
// 值范围: 0.0 ~ 1.0，越高越严格
// 默认值: 0.85
func WithThreshold(t float64) Option {
	return func(c *SemanticCache) {
		if t > 0 && t <= 1.0 {
			c.threshold = t
		}
	}
}

// New 创建语义缓存
// embedder 用于将查询文本转换为向量表示
func New(embedder Embedder, opts ...Option) *SemanticCache {
	c := &SemanticCache{
		entries:   make([]*CacheEntry, 0),
		embedder:  embedder,
		maxSize:   1000,
		ttl:       time.Hour,
		threshold: 0.85,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get 查找缓存（语义匹配）
// 将查询文本向量化后，与所有缓存条目计算余弦相似度，
// 返回相似度最高且超过阈值的结果。
//
// 返回值:
//   - result: 缓存结果，未命中时为 nil
//   - hit: 是否命中
//   - err: 嵌入计算错误
func (c *SemanticCache) Get(ctx context.Context, query string) (*CacheResult, bool, error) {
	// 生成查询向量
	embedding, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return nil, false, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	var bestEntry *CacheEntry
	bestSim := 0.0

	for _, entry := range c.entries {
		// 检查是否过期
		if now.Sub(entry.CreatedAt) > c.ttl {
			continue
		}

		// 计算余弦相似度
		sim := cosineSimilarity(embedding, entry.Embedding)
		if sim >= c.threshold && sim > bestSim {
			bestSim = sim
			bestEntry = entry
		}
	}

	if bestEntry != nil {
		atomic.AddInt64(&bestEntry.HitCount, 1)
		c.hits.Add(1)
		return bestEntry.Result, true, nil
	}

	c.misses.Add(1)
	return nil, false, nil
}

// Put 写入缓存
// 将查询和结果写入缓存。如果缓存已满，淘汰最早创建的条目。
func (c *SemanticCache) Put(ctx context.Context, query string, result *CacheResult) error {
	// 生成查询向量
	embedding, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 先清理过期条目，释放空间
	c.cleanExpiredLocked(now)

	// 检查是否已存在相同查询（精确匹配），更新而非重复添加
	for i, entry := range c.entries {
		if entry.Query == query {
			c.entries[i] = &CacheEntry{
				Query:     query,
				Embedding: embedding,
				Result:    result,
				CreatedAt: now,
			}
			return nil
		}
	}

	// 容量检查，淘汰最旧的条目
	for len(c.entries) >= c.maxSize {
		c.entries = c.entries[1:]
		c.evictions.Add(1)
	}

	// 添加新条目
	c.entries = append(c.entries, &CacheEntry{
		Query:     query,
		Embedding: embedding,
		Result:    result,
		CreatedAt: now,
	})

	return nil
}

// cleanExpiredLocked 清理过期缓存条目（调用方必须持有写锁）
func (c *SemanticCache) cleanExpiredLocked(now time.Time) {
	n := 0
	for _, entry := range c.entries {
		if now.Sub(entry.CreatedAt) <= c.ttl {
			c.entries[n] = entry
			n++
		} else {
			c.evictions.Add(1)
		}
	}
	c.entries = c.entries[:n]
}

// Invalidate 使特定缓存失效
// 通过精确匹配查询文本来删除对应的缓存条目
func (c *SemanticCache) Invalidate(query string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, entry := range c.entries {
		if entry.Query == query {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return
		}
	}
}

// Clear 清空所有缓存
func (c *SemanticCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make([]*CacheEntry, 0)
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
}

// Stats 获取缓存统计信息
func (c *SemanticCache) Stats() CacheStats {
	c.mu.RLock()
	size := len(c.entries)
	c.mu.RUnlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Size:      size,
		MaxSize:   c.maxSize,
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Evictions: c.evictions.Load(),
	}
}

// Size 返回当前缓存条目数
func (c *SemanticCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// cosineSimilarity 计算两个向量的余弦相似度
// 公式: cos(θ) = (A·B) / (|A| * |B|)
// 返回值范围: [-1, 1]，值越大表示越相似
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
