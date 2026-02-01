// Package vector 提供向量存储抽象
//
// 本包重新导出 ai-core/store/vector 的实现，保持向后兼容性。
//
// 使用示例:
//
//	store := vector.NewMemoryStore(1536)
//	defer store.Close()
//
//	docs := []vector.Document{
//	    {ID: "1", Content: "Hello", Embedding: embedding},
//	}
//	store.Add(ctx, docs)
package vector

import (
	aicoreVector "github.com/everyday-items/ai-core/store/vector"
)

// 重新导出类型
type (
	// Store 向量存储接口
	Store = aicoreVector.Store

	// Document 文档
	Document = aicoreVector.Document

	// SearchConfig 搜索配置
	SearchConfig = aicoreVector.SearchConfig

	// SearchOption 搜索选项
	SearchOption = aicoreVector.SearchOption

	// MemoryStore 内存向量存储
	MemoryStore = aicoreVector.MemoryStore

	// Embedder 向量生成器接口
	Embedder = aicoreVector.Embedder

	// EmbedderFunc 函数式 Embedder
	EmbedderFunc = aicoreVector.EmbedderFunc
)

// 重新导出函数
var (
	// NewMemoryStore 创建内存向量存储
	NewMemoryStore = aicoreVector.NewMemoryStore

	// NewEmbedderFunc 创建函数式 Embedder
	NewEmbedderFunc = aicoreVector.NewEmbedderFunc

	// WithFilter 设置过滤条件
	WithFilter = aicoreVector.WithFilter

	// WithMinScore 设置最小分数
	WithMinScore = aicoreVector.WithMinScore

	// WithEmbedding 设置是否返回向量
	WithEmbedding = aicoreVector.WithEmbedding

	// WithMetadata 设置是否返回元数据
	WithMetadata = aicoreVector.WithMetadata
)
