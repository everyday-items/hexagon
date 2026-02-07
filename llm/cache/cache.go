// Package cache 提供 LLM 响应缓存功能
//
// 本包实现了多层 LLM 响应缓存机制，支持：
//   - 内存缓存（本地快速访问）
//   - Redis 缓存（分布式共享）
//   - 混合缓存（多级缓存）
//   - 语义缓存（相似查询复用）
//
// 设计借鉴：
//   - LangChain: LLMCache
//   - GPTCache: 语义缓存
//   - Redis: 分布式缓存模式
//
// 使用示例：
//
//	cache := cache.NewMemoryCache(cache.DefaultCacheConfig())
//	cachedProvider := cache.Wrap(provider, cache)
//	response, err := cachedProvider.Complete(ctx, request)
package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"
)

// ============== 错误定义 ==============

var (
	// ErrCacheMiss 缓存未命中
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheExpired 缓存已过期
	ErrCacheExpired = errors.New("cache expired")

	// ErrCacheDisabled 缓存已禁用
	ErrCacheDisabled = errors.New("cache disabled")

	// ErrInvalidCacheKey 无效的缓存键
	ErrInvalidCacheKey = errors.New("invalid cache key")
)

// ============== 缓存接口 ==============

// Cache LLM 响应缓存接口
type Cache interface {
	// Get 获取缓存
	Get(ctx context.Context, key string) (*CacheEntry, error)

	// Set 设置缓存
	Set(ctx context.Context, key string, entry *CacheEntry) error

	// Delete 删除缓存
	Delete(ctx context.Context, key string) error

	// Clear 清空所有缓存
	Clear(ctx context.Context) error

	// Stats 获取缓存统计
	Stats() *CacheStats

	// Close 关闭缓存
	Close() error
}

// CacheEntry 缓存条目
type CacheEntry struct {
	// Key 缓存键
	Key string `json:"key"`

	// Response 响应内容
	Response []byte `json:"response"`

	// Model 模型名称
	Model string `json:"model"`

	// TokensUsed 使用的 token 数
	TokensUsed int `json:"tokens_used"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt 过期时间
	ExpiresAt time.Time `json:"expires_at"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// HitCount 命中次数
	HitCount int64 `json:"hit_count"`
}

// IsExpired 检查是否过期
func (e *CacheEntry) IsExpired() bool {
	return !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)
}

// CacheStats 缓存统计
type CacheStats struct {
	// Hits 命中次数
	Hits int64 `json:"hits"`

	// Misses 未命中次数
	Misses int64 `json:"misses"`

	// Size 缓存条目数
	Size int64 `json:"size"`

	// BytesUsed 使用的字节数
	BytesUsed int64 `json:"bytes_used"`

	// TokensSaved 节省的 token 数
	TokensSaved int64 `json:"tokens_saved"`

	// CostSaved 节省的成本（美元）
	CostSaved float64 `json:"cost_saved"`

	// HitRate 命中率
	HitRate float64 `json:"hit_rate"`

	// AvgLatencyMs 平均延迟（毫秒）
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// CacheConfig 缓存配置
type CacheConfig struct {
	// Enabled 是否启用缓存
	Enabled bool `json:"enabled"`

	// TTL 缓存过期时间
	TTL time.Duration `json:"ttl"`

	// MaxEntries 最大条目数
	MaxEntries int `json:"max_entries"`

	// MaxSize 最大缓存大小（字节）
	MaxSize int64 `json:"max_size"`

	// KeyPrefix 缓存键前缀
	KeyPrefix string `json:"key_prefix"`

	// IncludeModel 缓存键是否包含模型名
	IncludeModel bool `json:"include_model"`

	// IncludeTemperature 缓存键是否包含温度
	IncludeTemperature bool `json:"include_temperature"`

	// IgnoreSystemMessage 缓存键是否忽略系统消息
	IgnoreSystemMessage bool `json:"ignore_system_message"`

	// CostPerToken token 成本（用于计算节省）
	CostPerToken float64 `json:"cost_per_token"`
}

// DefaultCacheConfig 默认缓存配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:            true,
		TTL:                24 * time.Hour,
		MaxEntries:         10000,
		MaxSize:            100 * 1024 * 1024, // 100MB
		KeyPrefix:          "llm_cache:",
		IncludeModel:       true,
		IncludeTemperature: true,
		CostPerToken:       0.00003, // GPT-4 估算
	}
}

// ============== 内存缓存 ==============

// MemoryCache 内存缓存实现
// 使用 container/list 双向链表实现 O(1) 的 LRU 操作
type MemoryCache struct {
	config  *CacheConfig
	data    map[string]*CacheEntry
	list    *list.List                 // LRU 双向链表，Front 为最近使用，Back 为最久未使用
	listMap map[string]*list.Element   // key -> 链表节点的映射，用于 O(1) 查找
	mu      sync.RWMutex

	// 统计
	hits      int64
	misses    int64
	bytesUsed int64
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache(config *CacheConfig) *MemoryCache {
	if config == nil {
		config = DefaultCacheConfig()
	}
	return &MemoryCache{
		config:  config,
		data:    make(map[string]*CacheEntry),
		list:    list.New(),
		listMap: make(map[string]*list.Element),
	}
}

// Get 获取缓存
// 全程使用写锁，避免 TOCTOU 竞态：在同一临界区内完成查找、过期检查、统计更新和 LRU 调整
func (c *MemoryCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	if !c.config.Enabled {
		return nil, ErrCacheDisabled
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.data[key]
	if !exists {
		c.misses++
		return nil, ErrCacheMiss
	}

	// 在同一临界区内检查过期，避免释放锁后 entry 被其他 goroutine 修改
	if entry.IsExpired() {
		// 直接在锁内删除，不调用 Delete 方法（Delete 会重复加锁导致死锁）
		c.bytesUsed -= int64(len(entry.Response))
		delete(c.data, key)
		c.removeFromOrder(key)
		c.misses++
		return nil, ErrCacheExpired
	}

	// 更新命中统计和 LRU
	c.hits++
	entry.HitCount++
	c.moveToFront(key)

	return entry, nil
}

// Set 设置缓存
func (c *MemoryCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	if !c.config.Enabled {
		return ErrCacheDisabled
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查大小限制
	entrySize := int64(len(entry.Response))
	if c.config.MaxSize > 0 && c.bytesUsed+entrySize > c.config.MaxSize {
		c.evict(entrySize)
	}

	// 检查条目数限制
	if c.config.MaxEntries > 0 && len(c.data) >= c.config.MaxEntries {
		c.evictOne()
	}

	// 设置过期时间
	if entry.ExpiresAt.IsZero() && c.config.TTL > 0 {
		entry.ExpiresAt = time.Now().Add(c.config.TTL)
	}

	// 存储
	oldEntry, exists := c.data[key]
	if exists {
		c.bytesUsed -= int64(len(oldEntry.Response))
	}
	c.data[key] = entry
	c.bytesUsed += entrySize

	// 更新 LRU
	if !exists {
		elem := c.list.PushFront(key)
		c.listMap[key] = elem
	} else {
		c.moveToFront(key)
	}

	return nil
}

// Delete 删除缓存
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.data[key]; exists {
		c.bytesUsed -= int64(len(entry.Response))
		delete(c.data, key)
		c.removeFromOrder(key)
	}
	return nil
}

// Clear 清空缓存
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*CacheEntry)
	c.list.Init()
	c.listMap = make(map[string]*list.Element)
	c.bytesUsed = 0
	return nil
}

// Stats 获取统计
func (c *MemoryCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	var tokensSaved int64
	for _, entry := range c.data {
		tokensSaved += int64(entry.TokensUsed) * entry.HitCount
	}

	return &CacheStats{
		Hits:        c.hits,
		Misses:      c.misses,
		Size:        int64(len(c.data)),
		BytesUsed:   c.bytesUsed,
		TokensSaved: tokensSaved,
		CostSaved:   float64(tokensSaved) * c.config.CostPerToken,
		HitRate:     hitRate,
	}
}

// Close 关闭缓存
func (c *MemoryCache) Close() error {
	return c.Clear(context.Background())
}

// moveToFront 移动到链表最前面（标记为最近使用）— O(1)
func (c *MemoryCache) moveToFront(key string) {
	if elem, ok := c.listMap[key]; ok {
		c.list.MoveToFront(elem)
	}
}

// removeFromOrder 从 LRU 链表中移除 — O(1)
func (c *MemoryCache) removeFromOrder(key string) {
	if elem, ok := c.listMap[key]; ok {
		c.list.Remove(elem)
		delete(c.listMap, key)
	}
}

// evict 驱逐到指定大小
func (c *MemoryCache) evict(needed int64) {
	for c.bytesUsed+needed > c.config.MaxSize && c.list.Len() > 0 {
		c.evictOne()
	}
}

// evictOne 驱逐最久未使用的一个条目（链表尾部）— O(1)
func (c *MemoryCache) evictOne() {
	back := c.list.Back()
	if back == nil {
		return
	}
	key := back.Value.(string)
	c.list.Remove(back)
	delete(c.listMap, key)
	if entry, exists := c.data[key]; exists {
		c.bytesUsed -= int64(len(entry.Response))
		delete(c.data, key)
	}
}

// ============== 缓存键生成 ==============

// CacheKeyGenerator 缓存键生成器
type CacheKeyGenerator struct {
	config *CacheConfig
}

// NewCacheKeyGenerator 创建缓存键生成器
func NewCacheKeyGenerator(config *CacheConfig) *CacheKeyGenerator {
	return &CacheKeyGenerator{config: config}
}

// GenerateKey 生成缓存键
func (g *CacheKeyGenerator) GenerateKey(request *CacheRequest) string {
	// 构建键内容
	keyParts := make(map[string]any)

	// 消息内容
	if g.config.IgnoreSystemMessage {
		messages := make([]map[string]string, 0)
		for _, msg := range request.Messages {
			if msg.Role != "system" {
				messages = append(messages, map[string]string{
					"role":    msg.Role,
					"content": msg.Content,
				})
			}
		}
		keyParts["messages"] = messages
	} else {
		keyParts["messages"] = request.Messages
	}

	// 模型
	if g.config.IncludeModel {
		keyParts["model"] = request.Model
	}

	// 温度
	if g.config.IncludeTemperature {
		keyParts["temperature"] = request.Temperature
	}

	// 其他参数
	if request.MaxTokens > 0 {
		keyParts["max_tokens"] = request.MaxTokens
	}

	// 序列化
	data, _ := json.Marshal(keyParts)

	// 计算哈希
	hash := sha256.Sum256(data)
	key := g.config.KeyPrefix + hex.EncodeToString(hash[:])

	return key
}

// CacheRequest 缓存请求
type CacheRequest struct {
	// Model 模型名称
	Model string

	// Messages 消息列表
	Messages []CacheMessage

	// Temperature 温度
	Temperature float64

	// MaxTokens 最大 token 数
	MaxTokens int
}

// CacheMessage 缓存消息
type CacheMessage struct {
	// Role 角色
	Role string `json:"role"`

	// Content 内容
	Content string `json:"content"`
}

// ============== 分层缓存 ==============

// TieredCache 分层缓存
// 支持多级缓存，按层级查找
type TieredCache struct {
	tiers  []Cache
	config *CacheConfig
}

// NewTieredCache 创建分层缓存
func NewTieredCache(config *CacheConfig, tiers ...Cache) *TieredCache {
	return &TieredCache{
		tiers:  tiers,
		config: config,
	}
}

// Get 获取缓存（按层级查找）
func (c *TieredCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	for i, tier := range c.tiers {
		entry, err := tier.Get(ctx, key)
		if err == nil {
			// 填充上层缓存
			for j := 0; j < i; j++ {
				c.tiers[j].Set(ctx, key, entry)
			}
			return entry, nil
		}
	}
	return nil, ErrCacheMiss
}

// Set 设置缓存（写入所有层级）
func (c *TieredCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	var lastErr error
	for _, tier := range c.tiers {
		if err := tier.Set(ctx, key, entry); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Delete 删除缓存（从所有层级删除）
func (c *TieredCache) Delete(ctx context.Context, key string) error {
	var lastErr error
	for _, tier := range c.tiers {
		if err := tier.Delete(ctx, key); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Clear 清空缓存
func (c *TieredCache) Clear(ctx context.Context) error {
	var lastErr error
	for _, tier := range c.tiers {
		if err := tier.Clear(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Stats 获取统计
func (c *TieredCache) Stats() *CacheStats {
	if len(c.tiers) > 0 {
		return c.tiers[0].Stats()
	}
	return &CacheStats{}
}

// Close 关闭缓存
func (c *TieredCache) Close() error {
	var lastErr error
	for _, tier := range c.tiers {
		if err := tier.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ============== 语义缓存 ==============

// SemanticCacheConfig 语义缓存配置
type SemanticCacheConfig struct {
	// CacheConfig 基础缓存配置
	*CacheConfig

	// SimilarityThreshold 相似度阈值（0-1）
	SimilarityThreshold float64

	// EmbeddingModel 嵌入模型
	EmbeddingModel string

	// MaxCandidates 最大候选数
	MaxCandidates int
}

// DefaultSemanticCacheConfig 默认语义缓存配置
func DefaultSemanticCacheConfig() *SemanticCacheConfig {
	return &SemanticCacheConfig{
		CacheConfig:         DefaultCacheConfig(),
		SimilarityThreshold: 0.95,
		EmbeddingModel:      "text-embedding-ada-002",
		MaxCandidates:       10,
	}
}

// SemanticCache 语义缓存
// 基于语义相似度匹配缓存
type SemanticCache struct {
	config     *SemanticCacheConfig
	baseCache  Cache
	embeddings map[string][]float32
	mu         sync.RWMutex
}

// NewSemanticCache 创建语义缓存
func NewSemanticCache(config *SemanticCacheConfig, embedder Embedder) *SemanticCache {
	if config == nil {
		config = DefaultSemanticCacheConfig()
	}
	return &SemanticCache{
		config:     config,
		baseCache:  NewMemoryCache(config.CacheConfig),
		embeddings: make(map[string][]float32),
	}
}

// Embedder 嵌入接口
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Get 获取缓存（精确匹配）
func (c *SemanticCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	return c.baseCache.Get(ctx, key)
}

// GetSemantic 语义查询缓存
func (c *SemanticCache) GetSemantic(ctx context.Context, queryEmbedding []float32) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var bestKey string
	var bestSimilarity float64

	for key, embedding := range c.embeddings {
		similarity := cosineSimilarity(queryEmbedding, embedding)
		if similarity > bestSimilarity && similarity >= c.config.SimilarityThreshold {
			bestSimilarity = similarity
			bestKey = key
		}
	}

	if bestKey == "" {
		return nil, ErrCacheMiss
	}

	return c.baseCache.Get(ctx, bestKey)
}

// Set 设置缓存
func (c *SemanticCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	return c.baseCache.Set(ctx, key, entry)
}

// SetWithEmbedding 设置缓存（带嵌入向量）
func (c *SemanticCache) SetWithEmbedding(ctx context.Context, key string, entry *CacheEntry, embedding []float32) error {
	c.mu.Lock()
	c.embeddings[key] = embedding
	c.mu.Unlock()

	return c.baseCache.Set(ctx, key, entry)
}

// Delete 删除缓存
func (c *SemanticCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.embeddings, key)
	c.mu.Unlock()
	return c.baseCache.Delete(ctx, key)
}

// Clear 清空缓存
func (c *SemanticCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	c.embeddings = make(map[string][]float32)
	c.mu.Unlock()
	return c.baseCache.Clear(ctx)
}

// Stats 获取统计
func (c *SemanticCache) Stats() *CacheStats {
	return c.baseCache.Stats()
}

// Close 关闭缓存
func (c *SemanticCache) Close() error {
	return c.baseCache.Close()
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
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

// ============== 缓存中间件 ==============

// CacheMiddleware 缓存中间件选项
type CacheMiddleware struct {
	// Cache 缓存实例
	Cache Cache

	// KeyGenerator 键生成器
	KeyGenerator *CacheKeyGenerator

	// OnHit 命中回调
	OnHit func(key string, entry *CacheEntry)

	// OnMiss 未命中回调
	OnMiss func(key string)

	// OnSet 设置回调
	OnSet func(key string, entry *CacheEntry)
}

// NewCacheMiddleware 创建缓存中间件
func NewCacheMiddleware(cache Cache, config *CacheConfig) *CacheMiddleware {
	return &CacheMiddleware{
		Cache:        cache,
		KeyGenerator: NewCacheKeyGenerator(config),
	}
}

// ============== 缓存预热 ==============

// Warmer 缓存预热器
type Warmer struct {
	cache  Cache
	config *CacheConfig
}

// NewWarmer 创建缓存预热器
func NewWarmer(cache Cache, config *CacheConfig) *Warmer {
	return &Warmer{
		cache:  cache,
		config: config,
	}
}

// WarmFromFile 从文件预热
func (w *Warmer) WarmFromFile(ctx context.Context, path string) error {
	// 实现从文件加载缓存数据
	return nil
}

// WarmFromEntries 从条目列表预热
func (w *Warmer) WarmFromEntries(ctx context.Context, entries []*CacheEntry) error {
	for _, entry := range entries {
		if err := w.cache.Set(ctx, entry.Key, entry); err != nil {
			return err
		}
	}
	return nil
}

// ExportToFile 导出到文件
func (w *Warmer) ExportToFile(ctx context.Context, path string) error {
	// 实现导出缓存数据到文件
	return nil
}
