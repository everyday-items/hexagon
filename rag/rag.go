// Package rag 提供 Hexagon AI Agent 框架的检索增强生成 (RAG) 系统
//
// RAG 是一种将检索与生成结合的技术，让 AI Agent 能够基于外部知识库回答问题。
// 借鉴 LlamaIndex 的设计理念，提供完整的文档处理管道。
//
// 核心组件：
//   - Document: 文档数据结构
//   - Loader: 文档加载器（从文件、URL 等加载）
//   - Splitter: 文档分割器（将长文档分割成小块）
//   - Embedder: 向量生成器（将文本转换为向量）
//   - Indexer: 索引器（将文档向量化并存储）
//   - Retriever: 检索器（根据查询检索相关文档）
//   - Reranker: 重排序器（对检索结果重新排序）
//   - Synthesizer: 合成器（将检索结果与 LLM 结合生成答案）
//
// 使用示例：
//
//	engine := NewEngine(
//	    WithStore(vectorStore),
//	    WithEngineEmbedder(embedder),
//	)
//	docs, err := engine.Retrieve(ctx, "What is Go?")
package rag

import (
	"context"
	"time"
)

// Document 表示一个文档或文档片段
type Document struct {
	// ID 文档唯一标识
	ID string `json:"id"`

	// Content 文档内容
	Content string `json:"content"`

	// Metadata 文档元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// Embedding 文档向量（如果已生成）
	Embedding []float32 `json:"embedding,omitempty"`

	// Score 检索相关性分数（仅在检索结果中有效）
	Score float32 `json:"score,omitempty"`

	// Source 文档来源（文件路径、URL 等）
	Source string `json:"source,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Loader 是文档加载器接口
// 负责从各种来源加载文档
type Loader interface {
	// Load 加载文档
	Load(ctx context.Context) ([]Document, error)

	// Name 返回加载器名称
	Name() string
}

// Splitter 是文档分割器接口
// 负责将长文档分割成适合向量化的小块
type Splitter interface {
	// Split 分割文档
	Split(ctx context.Context, docs []Document) ([]Document, error)

	// Name 返回分割器名称
	Name() string
}

// Indexer 是索引器接口
// 负责将文档向量化并存储
type Indexer interface {
	// Index 索引文档
	Index(ctx context.Context, docs []Document) error

	// Delete 删除文档
	Delete(ctx context.Context, ids []string) error

	// Clear 清空索引
	Clear(ctx context.Context) error

	// Count 返回文档数量
	Count(ctx context.Context) (int, error)
}

// Retriever 是检索器接口
// 负责根据查询检索相关文档
type Retriever interface {
	// Retrieve 检索相关文档
	Retrieve(ctx context.Context, query string, opts ...RetrieveOption) ([]Document, error)
}

// RetrieveConfig 是检索配置
type RetrieveConfig struct {
	// TopK 返回的文档数量
	TopK int

	// MinScore 最小相关性分数
	MinScore float32

	// Filter 元数据过滤条件
	Filter map[string]any
}

// RetrieveOption 是检索选项
type RetrieveOption func(*RetrieveConfig)

// WithTopK 设置返回文档数量
func WithTopK(k int) RetrieveOption {
	return func(c *RetrieveConfig) {
		c.TopK = k
	}
}

// WithMinScore 设置最小分数阈值
func WithMinScore(score float32) RetrieveOption {
	return func(c *RetrieveConfig) {
		c.MinScore = score
	}
}

// WithFilter 设置元数据过滤
func WithFilter(filter map[string]any) RetrieveOption {
	return func(c *RetrieveConfig) {
		c.Filter = filter
	}
}

// Embedder 是向量生成器接口
type Embedder interface {
	// Embed 将文本转换为向量
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension 返回向量维度
	Dimension() int
}

// VectorStore 是向量存储接口
type VectorStore interface {
	// Add 添加向量
	Add(ctx context.Context, docs []Document) error

	// Search 相似性搜索
	Search(ctx context.Context, embedding []float32, topK int, filter map[string]any) ([]Document, error)

	// Delete 删除向量
	Delete(ctx context.Context, ids []string) error

	// Clear 清空存储
	Clear(ctx context.Context) error

	// Count 返回文档数量
	Count(ctx context.Context) (int, error)
}

// RAG 是 RAG 系统的核心接口
// 组合了 Indexer 和 Retriever 的功能
type RAG interface {
	Indexer
	Retriever
}

// Pipeline 是 RAG 处理管道
type Pipeline struct {
	loader    Loader
	splitter  Splitter
	indexer   Indexer
	retriever Retriever
}

// NewPipeline 创建 RAG 管道
func NewPipeline(loader Loader, splitter Splitter, indexer Indexer, retriever Retriever) *Pipeline {
	return &Pipeline{
		loader:    loader,
		splitter:  splitter,
		indexer:   indexer,
		retriever: retriever,
	}
}

// Ingest 执行完整的文档摄取流程
func (p *Pipeline) Ingest(ctx context.Context) error {
	// 1. 加载文档
	docs, err := p.loader.Load(ctx)
	if err != nil {
		return err
	}

	// 2. 分割文档
	if p.splitter != nil {
		docs, err = p.splitter.Split(ctx, docs)
		if err != nil {
			return err
		}
	}

	// 3. 索引文档
	return p.indexer.Index(ctx, docs)
}

// Query 执行查询
func (p *Pipeline) Query(ctx context.Context, query string, opts ...RetrieveOption) ([]Document, error) {
	return p.retriever.Retrieve(ctx, query, opts...)
}
