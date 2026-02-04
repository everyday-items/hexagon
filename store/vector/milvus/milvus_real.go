// Package milvus 提供 Milvus 向量数据库集成
//
// 本文件实现真实的 Milvus 客户端连接
// 需要安装依赖: go get github.com/milvus-io/milvus-sdk-go/v2
//
// 使用示例:
//
//	store, err := milvus.NewRealStore(ctx,
//	    milvus.WithAddress("localhost:19530"),
//	    milvus.WithCollection("documents"),
//	    milvus.WithDimension(1536),
//	)
//	defer store.Close()

//go:build milvus
// +build milvus

package milvus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/store/vector"
	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// RealStore 真实 Milvus 向量存储
// 使用 milvus-sdk-go 连接真实的 Milvus 服务器
type RealStore struct {
	client     milvusclient.Client
	address    string
	collection string
	dimension  int
	username   string
	password   string

	// 索引配置
	indexType   string
	metricType  entity.MetricType
	indexParams map[string]any

	// 连接状态
	connected bool
	mu        sync.RWMutex
}

// RealOption RealStore 配置选项
type RealOption func(*RealStore)

// WithRealAddress 设置 Milvus 地址
func WithRealAddress(address string) RealOption {
	return func(s *RealStore) {
		s.address = address
	}
}

// WithRealCollection 设置集合名称
func WithRealCollection(collection string) RealOption {
	return func(s *RealStore) {
		s.collection = collection
	}
}

// WithRealDimension 设置向量维度
func WithRealDimension(dim int) RealOption {
	return func(s *RealStore) {
		s.dimension = dim
	}
}

// WithRealAuth 设置认证信息
func WithRealAuth(username, password string) RealOption {
	return func(s *RealStore) {
		s.username = username
		s.password = password
	}
}

// WithRealIndexType 设置索引类型
func WithRealIndexType(indexType string) RealOption {
	return func(s *RealStore) {
		s.indexType = indexType
	}
}

// WithRealMetricType 设置距离度量类型
func WithRealMetricType(metricType string) RealOption {
	return func(s *RealStore) {
		switch metricType {
		case "L2":
			s.metricType = entity.L2
		case "IP":
			s.metricType = entity.IP
		case "COSINE":
			s.metricType = entity.COSINE
		default:
			s.metricType = entity.COSINE
		}
	}
}

// NewRealStore 创建真实 Milvus 向量存储
func NewRealStore(ctx context.Context, opts ...RealOption) (*RealStore, error) {
	s := &RealStore{
		address:    DefaultAddress,
		collection: DefaultCollection,
		dimension:  DefaultDimension,
		indexType:  DefaultIndexType,
		metricType: entity.COSINE,
		indexParams: map[string]any{
			"M":              16,
			"efConstruction": 200,
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	// 连接 Milvus
	var err error
	if s.username != "" {
		s.client, err = milvusclient.NewGrpcClient(ctx, s.address,
			milvusclient.WithGrpcOption(),
		)
	} else {
		s.client, err = milvusclient.NewGrpcClient(ctx, s.address)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Milvus at %s: %w", s.address, err)
	}

	s.connected = true

	// 确保集合存在
	if err := s.ensureCollection(ctx); err != nil {
		s.client.Close()
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	return s, nil
}

// ensureCollection 确保集合存在
func (s *RealStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.HasCollection(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}

	if exists {
		// 加载集合到内存
		err = s.client.LoadCollection(ctx, s.collection, false)
		if err != nil {
			return fmt.Errorf("load collection: %w", err)
		}
		return nil
	}

	// 创建集合
	schema := &entity.Schema{
		CollectionName: s.collection,
		Description:    "Hexagon document vectors",
		AutoID:         false,
		Fields: []*entity.Field{
			{
				Name:       "id",
				DataType:   entity.FieldTypeVarChar,
				PrimaryKey: true,
				TypeParams: map[string]string{"max_length": "256"},
			},
			{
				Name:     "embedding",
				DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{
					"dim": fmt.Sprintf("%d", s.dimension),
				},
			},
			{
				Name:       "content",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "65535"},
			},
			{
				Name:       "source",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "1024"},
			},
			{
				Name:     "metadata",
				DataType: entity.FieldTypeJSON,
			},
		},
	}

	err = s.client.CreateCollection(ctx, schema, entity.DefaultShardNumber)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// 创建索引
	idx, err := entity.NewIndexHNSW(s.metricType, 16, 200)
	if err != nil {
		return fmt.Errorf("create index params: %w", err)
	}

	err = s.client.CreateIndex(ctx, s.collection, "embedding", idx, false)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// 加载集合到内存
	err = s.client.LoadCollection(ctx, s.collection, false)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	return nil
}

// Add 添加文档
func (s *RealStore) Add(ctx context.Context, docs []vector.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	if len(docs) == 0 {
		return nil
	}

	// 准备数据
	ids := make([]string, len(docs))
	embeddings := make([][]float32, len(docs))
	contents := make([]string, len(docs))
	sources := make([]string, len(docs))
	metadatas := make([][]byte, len(docs))

	for i, doc := range docs {
		if doc.ID == "" {
			ids[i] = generateID()
		} else {
			ids[i] = doc.ID
		}

		if len(doc.Embedding) != s.dimension {
			return fmt.Errorf("document %d has invalid embedding dimension: expected %d, got %d",
				i, s.dimension, len(doc.Embedding))
		}
		embeddings[i] = doc.Embedding
		contents[i] = doc.Content
		sources[i] = doc.Source

		// 序列化元数据
		if doc.Metadata != nil {
			metadataBytes, err := json.Marshal(doc.Metadata)
			if err != nil {
				return fmt.Errorf("marshal metadata for doc %d: %w", i, err)
			}
			metadatas[i] = metadataBytes
		} else {
			metadatas[i] = []byte("{}")
		}
	}

	// 插入数据
	idColumn := entity.NewColumnVarChar("id", ids)
	embeddingColumn := entity.NewColumnFloatVector("embedding", s.dimension, embeddings)
	contentColumn := entity.NewColumnVarChar("content", contents)
	sourceColumn := entity.NewColumnVarChar("source", sources)
	metadataColumn := entity.NewColumnJSONBytes("metadata", metadatas)

	_, err := s.client.Insert(ctx, s.collection, "",
		idColumn, embeddingColumn, contentColumn, sourceColumn, metadataColumn)
	if err != nil {
		return fmt.Errorf("insert documents: %w", err)
	}

	// 刷新数据
	err = s.client.Flush(ctx, s.collection, false)
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

// Search 搜索相似文档
func (s *RealStore) Search(ctx context.Context, embedding []float32, limit int, opts ...vector.SearchOption) ([]vector.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	if len(embedding) != s.dimension {
		return nil, fmt.Errorf("invalid embedding dimension: expected %d, got %d",
			s.dimension, len(embedding))
	}

	// 应用搜索选项
	cfg := &vector.SearchConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// 构建搜索参数
	sp, err := entity.NewIndexHNSWSearchParam(64)
	if err != nil {
		return nil, fmt.Errorf("create search params: %w", err)
	}

	// 执行搜索
	searchVectors := []entity.Vector{entity.FloatVector(embedding)}
	results, err := s.client.Search(ctx, s.collection, nil, "",
		[]string{"id", "content", "source", "metadata"},
		searchVectors, "embedding", s.metricType, limit, sp)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// 解析结果
	var docs []vector.Document
	for _, result := range results {
		for i := 0; i < result.ResultCount; i++ {
			score := result.Scores[i]
			
			// 应用最小分数过滤
			if score < cfg.MinScore {
				continue
			}

			doc := vector.Document{
				Score: score,
			}

			// 获取字段值
			if idCol, ok := result.Fields.GetColumn("id").(*entity.ColumnVarChar); ok {
				doc.ID, _ = idCol.ValueByIdx(i)
			}
			if contentCol, ok := result.Fields.GetColumn("content").(*entity.ColumnVarChar); ok {
				doc.Content, _ = contentCol.ValueByIdx(i)
			}
			if sourceCol, ok := result.Fields.GetColumn("source").(*entity.ColumnVarChar); ok {
				doc.Source, _ = sourceCol.ValueByIdx(i)
			}
			if metadataCol, ok := result.Fields.GetColumn("metadata").(*entity.ColumnJSONBytes); ok {
				metadataBytes, _ := metadataCol.ValueByIdx(i)
				var metadata map[string]any
				if err := json.Unmarshal(metadataBytes, &metadata); err == nil {
					doc.Metadata = metadata
				}
			}

			// 是否包含向量
			if cfg.IncludeEmbedding {
				doc.Embedding = embedding // 注意：Milvus 搜索不返回向量
			}

			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// Get 获取文档
func (s *RealStore) Get(ctx context.Context, id string) (*vector.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	expr := fmt.Sprintf("id == \"%s\"", id)
	results, err := s.client.Query(ctx, s.collection, nil, expr,
		[]string{"id", "content", "source", "metadata"})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("document not found: %s", id)
	}

	doc := &vector.Document{}
	
	// 解析结果
	for _, col := range results {
		switch c := col.(type) {
		case *entity.ColumnVarChar:
			switch c.Name() {
			case "id":
				doc.ID, _ = c.ValueByIdx(0)
			case "content":
				doc.Content, _ = c.ValueByIdx(0)
			case "source":
				doc.Source, _ = c.ValueByIdx(0)
			}
		case *entity.ColumnJSONBytes:
			if c.Name() == "metadata" {
				metadataBytes, _ := c.ValueByIdx(0)
				var metadata map[string]any
				if err := json.Unmarshal(metadataBytes, &metadata); err == nil {
					doc.Metadata = metadata
				}
			}
		}
	}

	return doc, nil
}

// Delete 删除文档
func (s *RealStore) Delete(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	if len(ids) == 0 {
		return nil
	}

	// 构建表达式
	expr := "id in ["
	for i, id := range ids {
		if i > 0 {
			expr += ","
		}
		expr += fmt.Sprintf("\"%s\"", id)
	}
	expr += "]"

	err := s.client.Delete(ctx, s.collection, "", expr)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

// Update 更新文档
func (s *RealStore) Update(ctx context.Context, docs []vector.Document) error {
	// Milvus 不支持直接更新，需要删除后重新插入
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
	}

	if err := s.Delete(ctx, ids); err != nil {
		return fmt.Errorf("delete for update: %w", err)
	}

	return s.Add(ctx, docs)
}

// Count 统计文档数量
func (s *RealStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return 0, fmt.Errorf("not connected to Milvus")
	}

	stats, err := s.client.GetCollectionStatistics(ctx, s.collection)
	if err != nil {
		return 0, fmt.Errorf("get statistics: %w", err)
	}

	for _, stat := range stats {
		if stat.Key == "row_count" {
			var count int
			fmt.Sscanf(stat.Value, "%d", &count)
			return count, nil
		}
	}

	return 0, nil
}

// Clear 清空所有文档
func (s *RealStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return fmt.Errorf("not connected to Milvus")
	}

	// 删除并重建集合
	err := s.client.DropCollection(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("drop collection: %w", err)
	}

	return s.ensureCollection(ctx)
}

// Close 关闭连接
func (s *RealStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return nil
	}

	s.connected = false
	return s.client.Close()
}

// Stats 获取统计信息
func (s *RealStore) Stats(ctx context.Context) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return nil, fmt.Errorf("not connected to Milvus")
	}

	count, _ := s.Count(ctx)

	return map[string]any{
		"collection":  s.collection,
		"dimension":   s.dimension,
		"index_type":  s.indexType,
		"metric_type": s.metricType.String(),
		"connected":   s.connected,
		"count":       count,
		"timestamp":   time.Now(),
	}, nil
}

// IsExperimental 返回当前实现是否为实验性
func (s *RealStore) IsExperimental() bool {
	return false // 这是真实实现
}

// 确保实现了 vector.Store 接口
var _ vector.Store = (*RealStore)(nil)
