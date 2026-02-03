// Package reranker 提供文档重排序功能
package reranker

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/everyday-items/hexagon/rag"
)

// defaultMaxDocuments LLM 重排序的默认最大文档数
// 超过此数量将截断，避免过多 LLM 调用导致成本过高
const defaultMaxDocuments = 50

// LLMReranker 使用 LLM 进行重排序
//
// 通过让 LLM 评估查询与文档的相关性来实现重排序：
//   - 灵活性高，可以理解复杂的语义关系
//   - 成本较高，适合少量候选文档的精排
//   - 支持并发处理提高效率
//
// 注意：为避免过多 LLM 调用，默认最多处理 50 个文档。
// 可通过 WithLLMRerankerMaxDocuments 调整。
//
// 使用示例：
//
//	reranker := NewLLMReranker(llm,
//	    WithLLMRerankerTopK(10),
//	    WithLLMRerankerConcurrency(5),
//	)
//	result, err := reranker.Rerank(ctx, "query", docs)
type LLMReranker struct {
	// llm LLM 提供者
	llm LLMProvider

	// topK 返回数量
	topK int

	// concurrency 并发数
	concurrency int

	// maxDocuments 最大处理文档数
	maxDocuments int

	// promptTemplate 评分提示词模板
	promptTemplate string
}

// LLMRerankerOption LLMReranker 选项函数
type LLMRerankerOption func(*LLMReranker)

// WithLLMRerankerTopK 设置返回数量
//
// 参数：
//   - k: 返回的最相关文档数量
func WithLLMRerankerTopK(k int) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.topK = k
	}
}

// WithLLMRerankerConcurrency 设置并发数
//
// 参数：
//   - c: 并发处理的文档数量
func WithLLMRerankerConcurrency(c int) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.concurrency = c
	}
}

// WithLLMRerankerPromptTemplate 设置评分提示词模板
//
// 模板中可使用的占位符：
//   - {{query}}: 查询字符串
//   - {{document}}: 文档内容
//
// 参数：
//   - template: 提示词模板
func WithLLMRerankerPromptTemplate(template string) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.promptTemplate = template
	}
}

// WithLLMRerankerMaxDocuments 设置最大处理文档数
//
// 超过此数量的文档将被截断（保留前 N 个）。
// 这是为了避免过多 LLM 调用导致成本过高。
//
// 参数：
//   - max: 最大文档数，默认 50
func WithLLMRerankerMaxDocuments(max int) LLMRerankerOption {
	return func(r *LLMReranker) {
		if max > 0 {
			r.maxDocuments = max
		}
	}
}

// defaultLLMRerankerPrompt 默认的评分提示词
const defaultLLMRerankerPrompt = `Rate the relevance of the following document to the query on a scale of 0-10.
Only respond with a single number, no explanation needed.

Query: {{query}}

Document:
{{document}}

Relevance score (0-10):`

// NewLLMReranker 创建 LLM 重排序器
//
// 参数：
//   - llm: LLM 提供者实例
//   - opts: 可选配置项
//
// 返回：
//   - 配置好的 LLMReranker 实例
//
// 使用示例：
//
//	r := NewLLMReranker(llm,
//	    WithLLMRerankerTopK(5),
//	    WithLLMRerankerConcurrency(3),
//	)
func NewLLMReranker(llm LLMProvider, opts ...LLMRerankerOption) *LLMReranker {
	r := &LLMReranker{
		llm:            llm,
		topK:           10,
		concurrency:    5,
		maxDocuments:   defaultMaxDocuments,
		promptTemplate: defaultLLMRerankerPrompt,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Name 返回重排序器名称
func (r *LLMReranker) Name() string {
	return "LLMReranker"
}

// Rerank 使用 LLM 对文档重排序
//
// 参数：
//   - ctx: 上下文
//   - query: 查询字符串
//   - docs: 待重排序的文档列表
//
// 返回：
//   - 重排序后的文档列表（按相关性降序）
//   - 错误信息
func (r *LLMReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 限制处理的文档数量，避免过多 LLM 调用
	docsToScore := docs
	if len(docs) > r.maxDocuments {
		docsToScore = docs[:r.maxDocuments]
	}

	// 并发评分
	scores, err := r.scoreDocumentsConcurrently(ctx, query, docsToScore)
	if err != nil {
		return nil, err
	}

	// 按分数排序
	return r.sortByScores(docsToScore, scores), nil
}

// scoreDocumentsConcurrently 并发计算文档分数
func (r *LLMReranker) scoreDocumentsConcurrently(ctx context.Context, query string, docs []rag.Document) ([]float32, error) {
	scores := make([]float32, len(docs))
	errors := make([]error, len(docs))

	// 使用 semaphore 控制并发
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup

	for i, doc := range docs {
		wg.Add(1)
		go func(idx int, d rag.Document) {
			defer wg.Done()

			// 获取信号量
			sem <- struct{}{}
			defer func() { <-sem }()

			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				errors[idx] = ctx.Err()
				return
			default:
			}

			// 评分
			score, err := r.scoreDocument(ctx, query, d)
			if err != nil {
				errors[idx] = err
				scores[idx] = 0 // 出错时分数为 0
				return
			}
			scores[idx] = score
		}(i, doc)
	}

	wg.Wait()

	// 检查是否有错误（忽略单个文档的评分错误）
	var firstErr error
	failCount := 0
	for _, err := range errors {
		if err != nil {
			failCount++
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// 如果所有文档都评分失败，返回错误
	if failCount == len(docs) {
		return nil, fmt.Errorf("all document scoring failed: %w", firstErr)
	}

	return scores, nil
}

// scoreDocument 对单个文档评分
func (r *LLMReranker) scoreDocument(ctx context.Context, query string, doc rag.Document) (float32, error) {
	// 构建提示词
	prompt := r.buildPrompt(query, doc.Content)

	// 调用 LLM
	response, err := r.llm.Complete(ctx, prompt)
	if err != nil {
		return 0, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析分数
	score, err := r.parseScore(response)
	if err != nil {
		return 0, fmt.Errorf("parse score failed: %w", err)
	}

	return score, nil
}

// buildPrompt 构建评分提示词
func (r *LLMReranker) buildPrompt(query, document string) string {
	prompt := r.promptTemplate
	prompt = strings.ReplaceAll(prompt, "{{query}}", query)
	prompt = strings.ReplaceAll(prompt, "{{document}}", truncateText(document, 2000))
	return prompt
}

// parseScore 解析 LLM 返回的分数
//
// 期望 LLM 返回 0-10 的整数或浮点数
// 归一化到 0-1 范围
func (r *LLMReranker) parseScore(response string) (float32, error) {
	// 清理响应
	response = strings.TrimSpace(response)

	// 尝试提取数字
	re := regexp.MustCompile(`(\d+\.?\d*)`)
	matches := re.FindStringSubmatch(response)
	if len(matches) < 2 {
		return 0, fmt.Errorf("no number found in response: %s", response)
	}

	// 解析数字
	score, err := strconv.ParseFloat(matches[1], 32)
	if err != nil {
		return 0, fmt.Errorf("parse number failed: %w", err)
	}

	// 归一化到 0-1
	if score > 10 {
		score = 10
	}
	if score < 0 {
		score = 0
	}

	return float32(score / 10.0), nil
}

// sortByScores 根据分数对文档排序
func (r *LLMReranker) sortByScores(docs []rag.Document, scores []float32) []rag.Document {
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

	return result
}

// 确保实现 Reranker 接口
var _ Reranker = (*LLMReranker)(nil)
