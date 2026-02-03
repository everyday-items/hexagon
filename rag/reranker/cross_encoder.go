// Package reranker 提供文档重排序功能
package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/everyday-items/hexagon/rag"
)

// CrossEncoderReranker 跨编码器重排序器
//
// 跨编码器（Cross-Encoder）是一种高精度的相关性评估模型：
//   - 将查询和文档同时输入模型，计算相关性分数
//   - 精度高于双塔模型（Bi-Encoder），但速度较慢
//   - 适合在检索后对少量候选文档进行精排
//
// 使用方式：
//   - 需要部署支持 Cross-Encoder 的模型服务（如 sentence-transformers/cross-encoder）
//   - 通过 HTTP API 调用外部模型服务
//
// 使用示例：
//
//	reranker := NewCrossEncoderReranker(
//	    WithCrossEncoderModel("http://localhost:9000/rerank"),
//	    WithCrossEncoderTopK(10),
//	)
//	result, err := reranker.Rerank(ctx, "query", docs)
type CrossEncoderReranker struct {
	// modelURL 模型服务地址
	modelURL string

	// batchSize 批处理大小
	batchSize int

	// topK 返回数量
	topK int

	// timeout HTTP 请求超时
	timeout time.Duration

	// client HTTP 客户端
	client *http.Client
}

// CrossEncoderOption CrossEncoderReranker 选项函数
type CrossEncoderOption func(*CrossEncoderReranker)

// WithCrossEncoderModel 设置模型服务地址
//
// 参数：
//   - url: 模型服务的完整 URL，例如 "http://localhost:9000/rerank"
func WithCrossEncoderModel(url string) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.modelURL = url
	}
}

// WithCrossEncoderBatchSize 设置批处理大小
//
// 参数：
//   - size: 每批处理的文档数量，默认 32
func WithCrossEncoderBatchSize(size int) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.batchSize = size
	}
}

// WithCrossEncoderTopK 设置返回数量
//
// 参数：
//   - k: 返回的最相关文档数量，默认 10
func WithCrossEncoderTopK(k int) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.topK = k
	}
}

// WithCrossEncoderTimeout 设置请求超时时间
//
// 参数：
//   - timeout: 超时时间，默认 30 秒
func WithCrossEncoderTimeout(timeout time.Duration) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.timeout = timeout
		r.client = &http.Client{Timeout: timeout}
	}
}

// NewCrossEncoderReranker 创建跨编码器重排序器
//
// 参数：
//   - opts: 可选配置项
//
// 返回：
//   - 配置好的 CrossEncoderReranker 实例
//
// 使用示例：
//
//	r := NewCrossEncoderReranker(
//	    WithCrossEncoderModel("http://localhost:9000/rerank"),
//	    WithCrossEncoderBatchSize(16),
//	    WithCrossEncoderTopK(5),
//	)
func NewCrossEncoderReranker(opts ...CrossEncoderOption) *CrossEncoderReranker {
	r := &CrossEncoderReranker{
		modelURL:  "http://localhost:9000/rerank",
		batchSize: 32,
		topK:      10,
		timeout:   30 * time.Second,
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
func (r *CrossEncoderReranker) Name() string {
	return "CrossEncoderReranker"
}

// Rerank 使用跨编码器对文档重排序
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表（按相关性降序）
//   - 错误信息
func (r *CrossEncoderReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 如果没有配置模型服务，使用本地降级策略
	if r.modelURL == "" || r.modelURL == "http://localhost:9000/rerank" {
		return r.localFallback(docs)
	}

	// 调用远程模型服务
	scores, err := r.scoreDocuments(ctx, query, docs)
	if err != nil {
		// 降级到本地排序
		return r.localFallback(docs)
	}

	// 根据分数排序
	return r.sortByScores(docs, scores)
}

// scoreDocuments 调用远程模型服务计算文档分数
func (r *CrossEncoderReranker) scoreDocuments(ctx context.Context, query string, docs []rag.Document) ([]float32, error) {
	scores := make([]float32, len(docs))

	// 分批处理
	for i := 0; i < len(docs); i += r.batchSize {
		end := min(i+r.batchSize, len(docs))
		batch := docs[i:end]

		batchScores, err := r.scoreBatch(ctx, query, batch)
		if err != nil {
			return nil, err
		}

		copy(scores[i:end], batchScores)
	}

	return scores, nil
}

// crossEncoderRequest 跨编码器请求结构
type crossEncoderRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

// crossEncoderResponse 跨编码器响应结构
type crossEncoderResponse struct {
	Scores []float32 `json:"scores"`
}

// scoreBatch 对一批文档计算分数
func (r *CrossEncoderReranker) scoreBatch(ctx context.Context, query string, docs []rag.Document) ([]float32, error) {
	// 准备请求
	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.Content
	}

	reqBody := crossEncoderRequest{
		Query:     query,
		Documents: docContents,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "POST", r.modelURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 不在错误消息中暴露响应体，可能包含敏感信息
		_, _ = io.Copy(io.Discard, resp.Body) // 确保读取完响应体以便连接复用
		return nil, fmt.Errorf("cross-encoder API request failed with status %d", resp.StatusCode)
	}

	// 解析响应
	var respBody crossEncoderResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return respBody.Scores, nil
}

// sortByScores 根据分数对文档排序
func (r *CrossEncoderReranker) sortByScores(docs []rag.Document, scores []float32) ([]rag.Document, error) {
	type docWithScore struct {
		doc   rag.Document
		score float32
	}

	items := make([]docWithScore, len(docs))
	for i, doc := range docs {
		items[i] = docWithScore{doc: doc, score: scores[i]}
	}

	// 按分数降序排序
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	// 取 TopK
	k := min(r.topK, len(items))
	result := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := items[i].doc
		doc.Score = items[i].score
		result[i] = doc
	}

	return result, nil
}

// localFallback 本地降级策略：按原有分数排序
func (r *CrossEncoderReranker) localFallback(docs []rag.Document) ([]rag.Document, error) {
	return localFallbackByScore(docs, r.topK)
}

// 确保实现 Reranker 接口
var _ Reranker = (*CrossEncoderReranker)(nil)
