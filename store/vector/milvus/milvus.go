// Package milvus 提供 Milvus 向量数据库集成
//
// ⚠️ 警告: 当前为实验性内存模拟实现，不适用于生产环境！
//
// 当前状态: EXPERIMENTAL (实验性)
// - 所有操作使用内存模拟，不会连接真实的 Milvus 服务器
// - 数据不会持久化，重启后丢失
// - 仅用于开发测试和功能验证
//
// 生产环境使用说明:
// 如需在生产环境使用 Milvus，请：
// 1. 引入 milvus-sdk-go: go get github.com/milvus-io/milvus-sdk-go/v2
// 2. 实现真实的 Milvus 客户端连接
// 3. 或使用已经完整实现的 Qdrant/Chroma 向量存储
//
// Milvus 是一个开源的云原生向量数据库，专为海量向量数据的存储、索引和管理而设计。
//
// 特性（完整实现后）：
//   - 支持十亿级向量检索
//   - 多种索引类型 (FLAT, IVF_FLAT, HNSW)
//   - 分布式架构
//   - 云原生设计
//
// 使用示例:
//
//	store, err := milvus.NewStore(ctx,
//	    milvus.WithAddress("localhost:19530"),
//	    milvus.WithCollection("documents"),
//	    milvus.WithDimension(1536),
//	)
//	defer store.Close()
package milvus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/store/vector"
)

// Store Milvus 向量存储
//
// ⚠️ 警告: 当前为实验性内存模拟实现！
//
// 封装 Milvus 客户端，实现 vector.Store 接口
//
// 当前状态: EXPERIMENTAL
// - 使用内存模拟，不连接真实 Milvus
// - 生产环境请使用 Qdrant 或 Chroma
type Store struct {
	address    string
	collection string
	dimension  int
	username   string
	password   string

	// 连接状态
	connected bool
	mu        sync.RWMutex

	// 索引配置
	indexType   string // FLAT, IVF_FLAT, HNSW
	metricType  string // L2, IP, COSINE
	indexParams map[string]any

	// 内存存储（模拟实现）
	// 生产环境应该替换为真实的 Milvus 客户端
	documents map[string]*vector.Document
}

// Option Store 配置选项
type Option func(*Store)

// WithAddress 设置 Milvus 地址
func WithAddress(address string) Option {
	return func(s *Store) {
		s.address = address
	}
}

// WithCollection 设置集合名称
func WithCollection(collection string) Option {
	return func(s *Store) {
		s.collection = collection
	}
}

// WithDimension 设置向量维度
func WithDimension(dim int) Option {
	return func(s *Store) {
		s.dimension = dim
	}
}

// WithAuth 设置认证信息
func WithAuth(username, password string) Option {
	return func(s *Store) {
		s.username = username
		s.password = password
	}
}

// WithIndexType 设置索引类型
func WithIndexType(indexType string) Option {
	return func(s *Store) {
		s.indexType = indexType
	}
}

// WithMetricType 设置距离度量类型
func WithMetricType(metricType string) Option {
	return func(s *Store) {
		s.metricType = metricType
	}
}

// DefaultDimension 默认向量维度
const DefaultDimension = 1536

// DefaultAddress 默认 Milvus 地址
const DefaultAddress = "localhost:19530"

// DefaultCollection 默认集合名称
const DefaultCollection = "hexagon_documents"

// DefaultIndexType 默认索引类型
const DefaultIndexType = "HNSW"

// DefaultMetricType 默认距离度量类型
const DefaultMetricType = "COSINE"

// NewStore 创建 Milvus 向量存储
//
// ⚠️ 警告: 当前为实验性内存模拟实现，不连接真实 Milvus 服务器！
// 生产环境请使用 Qdrant 或 Chroma 向量存储。
func NewStore(ctx context.Context, opts ...Option) (*Store, error) {
	s := &Store{
		address:    DefaultAddress,
		collection: DefaultCollection,
		dimension:  DefaultDimension,
		indexType:  DefaultIndexType,
		metricType: DefaultMetricType,
		indexParams: map[string]any{
			"M":              16,
			"efConstruction": 200,
		},
		documents: make(map[string]*vector.Document),
	}

	for _, opt := range opts {
		opt(s)
	}

	// ⚠️ 实验性实现：使用内存模拟
	// 生产环境应该：
	// 1. 引入 milvus-sdk-go: go get github.com/milvus-io/milvus-sdk-go/v2
	// 2. 连接到 Milvus 服务器
	// 3. 确保集合存在
	//
	// 示例代码：
	// import "github.com/milvus-io/milvus-sdk-go/v2/client"
	// s.client, err = client.NewGrpcClient(ctx, s.address)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to connect: %w", err)
	// }

	s.connected = true

	return s, nil
}

// IsExperimental 返回当前实现是否为实验性
func (s *Store) IsExperimental() bool {
	return true // 当前为内存模拟实现
}

// Add 添加文档
func (s *Store) Add(ctx context.Context, docs []vector.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	if len(docs) == 0 {
		return nil
	}

	// 验证向量维度
	for i, doc := range docs {
		if len(doc.Embedding) != s.dimension {
			return fmt.Errorf("document %d has invalid embedding dimension: expected %d, got %d",
				i, s.dimension, len(doc.Embedding))
		}

		// 生成 ID（如果未提供）
		if doc.ID == "" {
			docs[i].ID = generateID()
		}

		// 存储到内存（模拟）
		docCopy := doc
		s.documents[docs[i].ID] = &docCopy
	}

	// TODO: 生产环境实现
	// 1. 准备数据为 Milvus 格式
	// 2. 调用 Insert API
	// 3. 刷新索引（可选）
	//
	// 示例代码：
	// columns := []entity.Column{
	//     entity.NewColumnVarChar("id", ids),
	//     entity.NewColumnFloatVector("embedding", dim, embeddings),
	//     entity.NewColumnVarChar("content", contents),
	// }
	// _, err := s.client.Insert(ctx, s.collection, "", columns...)

	return nil
}

// Search 搜索相似文档
func (s *Store) Search(ctx context.Context, embedding []float32, limit int, opts ...vector.SearchOption) ([]vector.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	// 验证向量维度
	if len(embedding) != s.dimension {
		return nil, fmt.Errorf("invalid embedding dimension: expected %d, got %d",
			s.dimension, len(embedding))
	}

	// 应用搜索选项
	cfg := &vector.SearchConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// 内存模拟：简单的暴力搜索
	type result struct {
		doc   *vector.Document
		score float32
	}

	var results []result
	for _, doc := range s.documents {
		// 计算相似度（余弦相似度）
		score := s.cosineSimilarity(embedding, doc.Embedding)

		// 应用最小分数过滤
		if score >= cfg.MinScore {
			results = append(results, result{doc: doc, score: score})
		}
	}

	// 排序（降序）
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// 限制数量
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	// 构建返回结果
	docs := make([]vector.Document, len(results))
	for i, r := range results {
		doc := *r.doc
		doc.Score = r.score
		if !cfg.IncludeEmbedding {
			doc.Embedding = nil
		}
		docs[i] = doc
	}

	// TODO: 生产环境实现
	// 使用 Milvus 的向量搜索 API
	//
	// 示例代码：
	// searchParams, _ := entity.NewIndexHNSWSearchParam(ef)
	// results, err := s.client.Search(ctx, s.collection,
	//     []string{},
	//     "", // filter expression
	//     []string{"id", "content"},
	//     []entity.Vector{entity.FloatVector(embedding)},
	//     "embedding",
	//     metricType,
	//     limit,
	//     searchParams,
	// )

	return docs, nil
}

// cosineSimilarity 计算余弦相似度
func (s *Store) cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
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

	return dotProduct / (float32(sqrt(float64(normA))) * float32(sqrt(float64(normB))))
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	// 牛顿迭代法
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// Get 获取文档
func (s *Store) Get(ctx context.Context, id string) (*vector.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	// 从内存获取（模拟）
	doc, ok := s.documents[id]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", id)
	}

	// 返回副本
	result := *doc
	return &result, nil

	// TODO: 生产环境实现
	// 使用 Milvus 的 Query API
	//
	// 示例代码：
	// expr := fmt.Sprintf("id == '%s'", id)
	// results, err := s.client.Query(ctx, s.collection,
	//     []string{},
	//     expr,
	//     []string{"id", "content", "embedding"},
	// )
}

// Delete 删除文档
func (s *Store) Delete(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// 从内存删除（模拟）
	for _, id := range ids {
		delete(s.documents, id)
	}

	// TODO: 生产环境实现
	// 使用 Milvus 的 Delete API
	//
	// 示例代码：
	// expr := fmt.Sprintf("id in [%s]", strings.Join(quoted(ids), ","))
	// err := s.client.Delete(ctx, s.collection, "", expr)

	return nil
}

// Update 更新文档
func (s *Store) Update(ctx context.Context, docs []vector.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// Milvus 不支持直接更新，需要删除后重新插入
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
		// 更新内存（模拟）
		docCopy := doc
		s.documents[doc.ID] = &docCopy
	}

	// TODO: 生产环境实现
	// 1. 先删除旧文档
	// 2. 再插入新文档
	//
	// 示例代码：
	// // Delete
	// expr := fmt.Sprintf("id in [%s]", strings.Join(quoted(ids), ","))
	// err := s.client.Delete(ctx, s.collection, "", expr)
	// if err != nil {
	//     return err
	// }
	// // Insert
	// return s.Add(ctx, docs)

	return nil
}

// Count 统计文档数量
func (s *Store) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return 0, fmt.Errorf("not connected to Milvus")
	}

	// 返回内存中的文档数量（模拟）
	return len(s.documents), nil

	// TODO: 生产环境实现
	// 使用 Milvus 的 GetCollectionStatistics API
	//
	// 示例代码：
	// stats, err := s.client.GetCollectionStatistics(ctx, s.collection)
	// if err != nil {
	//     return 0, err
	// }
	// count, err := strconv.Atoi(stats["row_count"])
	// return count, err
}

// Close 关闭连接
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return nil
	}

	// TODO: 实际实现
	// 1. 关闭客户端连接

	s.connected = false
	return nil
}

// CreateIndex 创建索引
func (s *Store) CreateIndex(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	// 1. 创建向量字段索引
	// 2. 等待索引构建完成

	return nil
}

// DropIndex 删除索引
func (s *Store) DropIndex(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	// 1. 删除索引

	return nil
}

// LoadCollection 加载集合到内存
func (s *Store) LoadCollection(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	// 1. 加载集合到内存
	// 2. 等待加载完成

	return nil
}

// ReleaseCollection 释放集合
func (s *Store) ReleaseCollection(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	// 1. 释放集合

	return nil
}

// Flush 刷新数据到磁盘
func (s *Store) Flush(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	// 1. 刷新集合

	return nil
}

// Clear 清空所有文档
func (s *Store) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// 清空内存（模拟）
	s.documents = make(map[string]*vector.Document)

	// TODO: 生产环境实现
	// 删除并重建集合
	//
	// 示例代码：
	// // Drop collection
	// err := s.client.DropCollection(ctx, s.collection)
	// if err != nil {
	//     return err
	// }
	// // Recreate collection
	// return s.createCollection(ctx)

	return nil
}

// Stats 获取统计信息
func (s *Store) Stats(ctx context.Context) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	// TODO: 实际实现
	stats := map[string]any{
		"collection": s.collection,
		"dimension":  s.dimension,
		"index_type": s.indexType,
		"metric_type": s.metricType,
		"connected":  s.connected,
		"timestamp":  time.Now(),
	}

	return stats, nil
}

// 确保实现了 vector.Store 接口
var _ vector.Store = (*Store)(nil)
