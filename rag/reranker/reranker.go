// Package reranker 提供文档重排序功能
//
// Reranker 用于对检索到的文档进行二次排序，提高结果质量：
//   - 提升最相关文档的排名
//   - 增加结果多样性
//   - 优化上下文利用
package reranker

import (
	"context"

	"github.com/everyday-items/hexagon/rag"
)

// Reranker 文档重排序器接口
//
// 所有重排序器都实现此接口，提供统一的重排序能力
type Reranker interface {
	// Name 返回重排序器名称
	Name() string

	// Rerank 重排序文档
	//
	// 参数：
	//   - ctx: 上下文
	//   - query: 查询字符串
	//   - docs: 待重排序的文档列表
	//
	// 返回：
	//   - 重排序后的文档列表
	//   - 错误信息
	Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error)
}

// LLMProvider LLM 提供者接口（简化版）
//
// 用于重排序器中需要调用 LLM 的场景
type LLMProvider interface {
	// Complete 完成文本生成
	//
	// 参数：
	//   - ctx: 上下文
	//   - prompt: 提示词
	//
	// 返回：
	//   - 生成的文本
	//   - 错误信息
	Complete(ctx context.Context, prompt string) (string, error)
}
