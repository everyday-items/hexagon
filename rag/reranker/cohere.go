// Package reranker 提供文档重排序功能
package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/everyday-items/hexagon/rag"
)

// CohereReranker 使用 Cohere API 进行重排序
//
// Cohere Rerank API 是一个高质量的商业重排序服务：
//   - 支持多语言（100+ 语言）
//   - 高精度的语义相关性评估
//   - 简单易用的 REST API
//
// API 文档：https://docs.cohere.com/reference/rerank
//
// 使用示例：
//
//	reranker := NewCohereReranker("your-api-key",
//	    WithCohereModel("rerank-multilingual-v3.0"),
//	    WithCohereTopK(10),
//	)
//	result, err := reranker.Rerank(ctx, "query", docs)
type CohereReranker struct {
	// apiKey Cohere API 密钥
	apiKey string

	// model 使用的模型名称
	model string

	// topK 返回数量
	topK int

	// maxDocuments 单次请求最大文档数
	maxDocuments int

	// timeout 请求超时时间
	timeout time.Duration

	// baseURL API 基础地址
	baseURL string

	// client HTTP 客户端
	client *http.Client
}

// CohereOption CohereReranker 选项函数
type CohereOption func(*CohereReranker)

// WithCohereAPIKey 设置 API 密钥（可覆盖构造函数中的密钥）
//
// 参数：
//   - key: Cohere API 密钥
func WithCohereAPIKey(key string) CohereOption {
	return func(r *CohereReranker) {
		r.apiKey = key
	}
}

// WithCohereModel 设置使用的模型
//
// 参数：
//   - model: 模型名称，如 "rerank-english-v3.0" 或 "rerank-multilingual-v3.0"
//
// 可选模型：
//   - rerank-english-v3.0: 英文专用模型
//   - rerank-multilingual-v3.0: 多语言模型
//   - rerank-english-v2.0: 旧版英文模型
//   - rerank-multilingual-v2.0: 旧版多语言模型
func WithCohereModel(model string) CohereOption {
	return func(r *CohereReranker) {
		r.model = model
	}
}

// WithCohereTopK 设置返回数量
//
// 参数：
//   - k: 返回的最相关文档数量
func WithCohereTopK(k int) CohereOption {
	return func(r *CohereReranker) {
		r.topK = k
	}
}

// WithCohereTimeout 设置请求超时时间
//
// 参数：
//   - timeout: 超时时间
func WithCohereTimeout(timeout time.Duration) CohereOption {
	return func(r *CohereReranker) {
		r.timeout = timeout
		r.client = &http.Client{Timeout: timeout}
	}
}

// WithCohereBaseURL 设置 API 基础地址（用于测试或私有部署）
//
// 参数：
//   - url: API 基础地址
func WithCohereBaseURL(url string) CohereOption {
	return func(r *CohereReranker) {
		r.baseURL = url
	}
}

// NewCohereReranker 创建 Cohere 重排序器
//
// 参数：
//   - apiKey: Cohere API 密钥
//   - opts: 可选配置项
//
// 返回：
//   - 配置好的 CohereReranker 实例
//
// 使用示例：
//
//	r := NewCohereReranker("your-api-key",
//	    WithCohereModel("rerank-multilingual-v3.0"),
//	    WithCohereTopK(10),
//	)
func NewCohereReranker(apiKey string, opts ...CohereOption) *CohereReranker {
	r := &CohereReranker{
		apiKey:       apiKey,
		model:        "rerank-english-v3.0",
		topK:         10,
		maxDocuments: 1000, // Cohere API 限制
		timeout:      30 * time.Second,
		baseURL:      "https://api.cohere.ai",
	}

	// 配置带连接池的 HTTP 客户端
	r.client = &http.Client{
		Timeout: r.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Name 返回重排序器名称
func (r *CohereReranker) Name() string {
	return "CohereReranker"
}

// Rerank 使用 Cohere API 对文档重排序
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表（按相关性降序）
//   - 错误信息
func (r *CohereReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 如果没有 API Key，降级到本地排序
	if r.apiKey == "" {
		return r.localFallback(docs)
	}

	// 限制文档数量
	docsToRerank := docs
	if len(docs) > r.maxDocuments {
		docsToRerank = docs[:r.maxDocuments]
	}

	// 调用 Cohere API
	results, err := r.callCohereAPI(ctx, query, docsToRerank)
	if err != nil {
		// API 调用失败时降级到本地排序
		return r.localFallback(docs)
	}

	return results, nil
}

// cohereRerankRequest Cohere Rerank API 请求结构
type cohereRerankRequest struct {
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Model           string   `json:"model,omitempty"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
}

// cohereRerankResponse Cohere Rerank API 响应结构
type cohereRerankResponse struct {
	Results []cohereRerankResult `json:"results"`
}

// cohereRerankResult 单个重排序结果
type cohereRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// callCohereAPI 调用 Cohere Rerank API
func (r *CohereReranker) callCohereAPI(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	// 准备请求
	docContents := make([]string, len(docs))
	for i, doc := range docs {
		// Cohere 要求每个文档不超过 10k tokens，这里简单截断
		content := doc.Content
		if len(content) > 10000 {
			content = content[:10000]
		}
		docContents[i] = content
	}

	reqBody := cohereRerankRequest{
		Query:           query,
		Documents:       docContents,
		Model:           r.model,
		TopN:            r.topK,
		ReturnDocuments: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 发送请求
	url := r.baseURL + "/v1/rerank"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 不在错误消息中暴露响应体，可能包含敏感信息
		_, _ = io.Copy(io.Discard, resp.Body) // 确保读取完响应体以便连接复用
		return nil, fmt.Errorf("cohere API request failed with status %d", resp.StatusCode)
	}

	// 解析响应
	var respBody cohereRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	// 根据结果重建文档列表
	// 只添加有效索引的文档，过滤掉无效响应
	result := make([]rag.Document, 0, len(respBody.Results))
	for _, res := range respBody.Results {
		if res.Index >= 0 && res.Index < len(docs) {
			doc := docs[res.Index]
			doc.Score = float32(res.RelevanceScore)
			result = append(result, doc)
		}
		// 跳过无效索引，不会留下空文档
	}

	return result, nil
}

// localFallback 本地降级策略：按原有分数排序
func (r *CohereReranker) localFallback(docs []rag.Document) ([]rag.Document, error) {
	return localFallbackByScore(docs, r.topK)
}

// 确保实现 Reranker 接口
var _ Reranker = (*CohereReranker)(nil)
