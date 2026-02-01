// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/everyday-items/hexagon/rag"
)

// Retriever Mock Retriever 实现
type Retriever struct {
	documents     []rag.Document
	mu            sync.RWMutex
	retrieveCalls []string

	// 自定义检索函数
	retrieveFn func(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error)
}

// RetrieverOption Retriever 选项
type RetrieverOption func(*Retriever)

// WithDocuments 设置文档列表
func WithDocuments(docs []rag.Document) RetrieverOption {
	return func(r *Retriever) {
		r.documents = docs
	}
}

// WithRetrieveFn 设置自定义检索函数
func WithRetrieveFn(fn func(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error)) RetrieverOption {
	return func(r *Retriever) {
		r.retrieveFn = fn
	}
}

// NewRetriever 创建 Mock Retriever
func NewRetriever(opts ...RetrieverOption) *Retriever {
	r := &Retriever{
		documents:     make([]rag.Document, 0),
		retrieveCalls: make([]string, 0),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Retrieve 检索文档
func (r *Retriever) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	r.mu.Lock()
	r.retrieveCalls = append(r.retrieveCalls, query)
	r.mu.Unlock()

	// 检查 context
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 使用自定义检索函数
	if r.retrieveFn != nil {
		return r.retrieveFn(ctx, query, opts...)
	}

	// 解析选项
	cfg := &rag.RetrieveConfig{
		TopK: 5,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 返回预设文档
	k := cfg.TopK
	if k > len(r.documents) {
		k = len(r.documents)
	}

	return r.documents[:k], nil
}

// AddDocument 添加文档
func (r *Retriever) AddDocument(doc rag.Document) *Retriever {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.documents = append(r.documents, doc)
	return r
}

// AddDocuments 批量添加文档
func (r *Retriever) AddDocuments(docs []rag.Document) *Retriever {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.documents = append(r.documents, docs...)
	return r
}

// RetrieveCalls 返回所有检索调用
func (r *Retriever) RetrieveCalls() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	calls := make([]string, len(r.retrieveCalls))
	copy(calls, r.retrieveCalls)
	return calls
}

// Reset 重置状态
func (r *Retriever) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retrieveCalls = make([]string, 0)
}

var _ rag.Retriever = (*Retriever)(nil)

// ============== 工具函数 ==============

// EmptyRetriever 创建空检索器（总是返回空结果）
func EmptyRetriever() *Retriever {
	return NewRetriever()
}

// FixedRetriever 创建固定结果检索器
func FixedRetriever(docs []rag.Document) *Retriever {
	return NewRetriever(WithDocuments(docs))
}

// ErrorRetriever 创建总是返回错误的检索器
func ErrorRetriever(err error) *Retriever {
	return NewRetriever(WithRetrieveFn(func(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
		return nil, err
	}))
}

// SimpleDocuments 创建简单文档列表
func SimpleDocuments(contents ...string) []rag.Document {
	docs := make([]rag.Document, len(contents))
	for i, content := range contents {
		docs[i] = rag.Document{
			ID:      fmt.Sprintf("doc-%d", i+1),
			Content: content,
			Score:   float32(len(contents)-i) / float32(len(contents)), // 按顺序递减分数
		}
	}
	return docs
}
