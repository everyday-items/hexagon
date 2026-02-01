// Package indexer 提供 RAG 系统的文档索引器
//
// Indexer 用于将文档向量化并存储到向量数据库：
//   - VectorIndexer: 基于向量存储的索引器
//   - BatchIndexer: 批量索引器
package indexer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
	"github.com/everyday-items/hexagon/store/vector"
)

// ============== VectorIndexer ==============

// VectorIndexer 向量索引器
type VectorIndexer struct {
	store     vector.Store
	embedder  vector.Embedder
	batchSize int
}

// VectorIndexerOption VectorIndexer 选项
type VectorIndexerOption func(*VectorIndexer)

// WithBatchSize 设置批量大小
func WithBatchSize(size int) VectorIndexerOption {
	return func(i *VectorIndexer) {
		i.batchSize = size
	}
}

// NewVectorIndexer 创建向量索引器
func NewVectorIndexer(store vector.Store, embedder vector.Embedder, opts ...VectorIndexerOption) *VectorIndexer {
	idx := &VectorIndexer{
		store:     store,
		embedder:  embedder,
		batchSize: 100,
	}
	for _, opt := range opts {
		opt(idx)
	}
	return idx
}

// Index 索引文档
func (i *VectorIndexer) Index(ctx context.Context, docs []rag.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 分批处理
	for start := 0; start < len(docs); start += i.batchSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		end := start + i.batchSize
		if end > len(docs) {
			end = len(docs)
		}

		batch := docs[start:end]

		// 提取需要嵌入的文本
		texts := make([]string, len(batch))
		for j, doc := range batch {
			texts[j] = doc.Content
		}

		// 生成向量
		embeddings, err := i.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("failed to embed documents: %w", err)
		}

		// 转换为 vector.Document
		vectorDocs := make([]vector.Document, len(batch))
		for j, doc := range batch {
			id := doc.ID
			if id == "" {
				id = util.GenerateID("doc")
			}
			vectorDocs[j] = vector.Document{
				ID:        id,
				Content:   doc.Content,
				Embedding: embeddings[j],
				Metadata:  doc.Metadata,
				CreatedAt: time.Now(),
			}
		}

		// 存储到向量数据库
		if err := i.store.Add(ctx, vectorDocs); err != nil {
			return fmt.Errorf("failed to add documents to store: %w", err)
		}
	}

	return nil
}

// Delete 删除文档
func (i *VectorIndexer) Delete(ctx context.Context, ids []string) error {
	return i.store.Delete(ctx, ids)
}

// Clear 清空索引
func (i *VectorIndexer) Clear(ctx context.Context) error {
	return i.store.Clear(ctx)
}

// Count 返回文档数量
func (i *VectorIndexer) Count(ctx context.Context) (int, error) {
	return i.store.Count(ctx)
}

var _ rag.Indexer = (*VectorIndexer)(nil)

// ============== ConcurrentIndexer ==============

// ConcurrentIndexer 并发索引器
type ConcurrentIndexer struct {
	store       vector.Store
	embedder    vector.Embedder
	batchSize   int
	concurrency int
}

// ConcurrentIndexerOption ConcurrentIndexer 选项
type ConcurrentIndexerOption func(*ConcurrentIndexer)

// WithConcurrentBatchSize 设置批量大小
func WithConcurrentBatchSize(size int) ConcurrentIndexerOption {
	return func(i *ConcurrentIndexer) {
		i.batchSize = size
	}
}

// WithConcurrency 设置并发数
func WithConcurrency(n int) ConcurrentIndexerOption {
	return func(i *ConcurrentIndexer) {
		i.concurrency = n
	}
}

// NewConcurrentIndexer 创建并发索引器
func NewConcurrentIndexer(store vector.Store, embedder vector.Embedder, opts ...ConcurrentIndexerOption) *ConcurrentIndexer {
	idx := &ConcurrentIndexer{
		store:       store,
		embedder:    embedder,
		batchSize:   100,
		concurrency: 4,
	}
	for _, opt := range opts {
		opt(idx)
	}
	return idx
}

// Index 并发索引文档
func (i *ConcurrentIndexer) Index(ctx context.Context, docs []rag.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 分批
	var batches [][]rag.Document
	for start := 0; start < len(docs); start += i.batchSize {
		end := start + i.batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batches = append(batches, docs[start:end])
	}

	// 并发处理
	errCh := make(chan error, len(batches))
	sem := make(chan struct{}, i.concurrency)

	var wg sync.WaitGroup
	for _, batch := range batches {
		wg.Add(1)
		go func(batch []rag.Document) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			// 生成向量
			texts := make([]string, len(batch))
			for j, doc := range batch {
				texts[j] = doc.Content
			}

			embeddings, err := i.embedder.Embed(ctx, texts)
			if err != nil {
				errCh <- err
				return
			}

			// 转换并存储
			vectorDocs := make([]vector.Document, len(batch))
			for j, doc := range batch {
				id := doc.ID
				if id == "" {
					id = util.GenerateID("doc")
				}
				vectorDocs[j] = vector.Document{
					ID:        id,
					Content:   doc.Content,
					Embedding: embeddings[j],
					Metadata:  doc.Metadata,
					CreatedAt: time.Now(),
				}
			}

			if err := i.store.Add(ctx, vectorDocs); err != nil {
				errCh <- err
				return
			}
		}(batch)
	}

	wg.Wait()
	close(errCh)

	// 收集错误
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete 删除文档
func (i *ConcurrentIndexer) Delete(ctx context.Context, ids []string) error {
	return i.store.Delete(ctx, ids)
}

// Clear 清空索引
func (i *ConcurrentIndexer) Clear(ctx context.Context) error {
	return i.store.Clear(ctx)
}

// Count 返回文档数量
func (i *ConcurrentIndexer) Count(ctx context.Context) (int, error) {
	return i.store.Count(ctx)
}

var _ rag.Indexer = (*ConcurrentIndexer)(nil)

// ============== IncrementalIndexer ==============

// IncrementalIndexer 增量索引器
// 只索引新增或变更的文档
type IncrementalIndexer struct {
	store        vector.Store
	embedder     vector.Embedder
	checksums    map[string]string // ID -> checksum
	mu           sync.RWMutex
	batchSize    int
}

// IncrementalIndexerOption IncrementalIndexer 选项
type IncrementalIndexerOption func(*IncrementalIndexer)

// WithIncrementalBatchSize 设置批量大小
func WithIncrementalBatchSize(size int) IncrementalIndexerOption {
	return func(i *IncrementalIndexer) {
		i.batchSize = size
	}
}

// NewIncrementalIndexer 创建增量索引器
func NewIncrementalIndexer(store vector.Store, embedder vector.Embedder, opts ...IncrementalIndexerOption) *IncrementalIndexer {
	idx := &IncrementalIndexer{
		store:     store,
		embedder:  embedder,
		checksums: make(map[string]string),
		batchSize: 100,
	}
	for _, opt := range opts {
		opt(idx)
	}
	return idx
}

// Index 增量索引文档
func (i *IncrementalIndexer) Index(ctx context.Context, docs []rag.Document) error {
	// 过滤出需要更新的文档
	var toIndex []rag.Document

	i.mu.RLock()
	for _, doc := range docs {
		checksum := computeChecksum(doc.Content)
		if existing, ok := i.checksums[doc.ID]; !ok || existing != checksum {
			toIndex = append(toIndex, doc)
		}
	}
	i.mu.RUnlock()

	if len(toIndex) == 0 {
		return nil
	}

	// 使用基础索引器索引
	baseIndexer := NewVectorIndexer(i.store, i.embedder, WithBatchSize(i.batchSize))
	if err := baseIndexer.Index(ctx, toIndex); err != nil {
		return err
	}

	// 更新校验和
	i.mu.Lock()
	for _, doc := range toIndex {
		i.checksums[doc.ID] = computeChecksum(doc.Content)
	}
	i.mu.Unlock()

	return nil
}

// Delete 删除文档
func (i *IncrementalIndexer) Delete(ctx context.Context, ids []string) error {
	if err := i.store.Delete(ctx, ids); err != nil {
		return err
	}

	i.mu.Lock()
	for _, id := range ids {
		delete(i.checksums, id)
	}
	i.mu.Unlock()

	return nil
}

// Clear 清空索引
func (i *IncrementalIndexer) Clear(ctx context.Context) error {
	if err := i.store.Clear(ctx); err != nil {
		return err
	}

	i.mu.Lock()
	i.checksums = make(map[string]string)
	i.mu.Unlock()

	return nil
}

// Count 返回文档数量
func (i *IncrementalIndexer) Count(ctx context.Context) (int, error) {
	return i.store.Count(ctx)
}

var _ rag.Indexer = (*IncrementalIndexer)(nil)

// ============== 辅助函数 ==============

// computeChecksum 计算内容校验和
func computeChecksum(content string) string {
	// 简单使用内容长度和首尾字符作为快速校验
	// 生产环境应使用 MD5 或 SHA256
	if len(content) == 0 {
		return "empty"
	}
	return fmt.Sprintf("%d:%c:%c", len(content), content[0], content[len(content)-1])
}
