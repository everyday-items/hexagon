// Package reranker 提供 RAG 系统的文档重排序
//
// Reranker 用于对初始检索结果进行重排序，提高最终结果的相关性：
//   - CrossEncoderReranker: 使用交叉编码器模型
//   - CohereReranker: 使用 Cohere Rerank API
//   - LLMReranker: 使用 LLM 进行排序
//   - RRFReranker: 使用 Reciprocal Rank Fusion
//   - ScoreReranker: 基于分数阈值过滤
package reranker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/everyday-items/hexagon/rag"
)

// Reranker 是重排序器接口
type Reranker interface {
	// Rerank 对文档进行重排序
	Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error)

	// Name 返回重排序器名称
	Name() string
}

// RankedDocument 带排名的文档
type RankedDocument struct {
	rag.Document
	RelevanceScore float32 `json:"relevance_score"`
	OriginalRank   int     `json:"original_rank"`
	NewRank        int     `json:"new_rank"`
}

// ============== CrossEncoderReranker ==============

// CrossEncoderReranker 使用交叉编码器模型进行重排序
// 支持本地模型或远程 API
type CrossEncoderReranker struct {
	modelURL   string
	httpClient *http.Client
	batchSize  int
	topK       int
}

// CrossEncoderOption CrossEncoderReranker 选项
type CrossEncoderOption func(*CrossEncoderReranker)

// WithCrossEncoderModel 设置模型 URL
func WithCrossEncoderModel(url string) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.modelURL = url
	}
}

// WithCrossEncoderBatchSize 设置批处理大小
func WithCrossEncoderBatchSize(size int) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.batchSize = size
	}
}

// WithCrossEncoderTopK 设置返回数量
func WithCrossEncoderTopK(k int) CrossEncoderOption {
	return func(r *CrossEncoderReranker) {
		r.topK = k
	}
}

// NewCrossEncoderReranker 创建交叉编码器重排序器
func NewCrossEncoderReranker(opts ...CrossEncoderOption) *CrossEncoderReranker {
	r := &CrossEncoderReranker{
		modelURL:   "http://localhost:8000/rerank",
		httpClient: &http.Client{},
		batchSize:  32,
		topK:       10,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type crossEncoderRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type crossEncoderResponse struct {
	Scores []float32 `json:"scores"`
}

// Rerank 使用交叉编码器重排序
func (r *CrossEncoderReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 准备请求
	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	// 批量处理
	var allScores []float32
	for i := 0; i < len(contents); i += r.batchSize {
		end := i + r.batchSize
		if end > len(contents) {
			end = len(contents)
		}

		batch := contents[i:end]
		scores, err := r.scoreBatch(ctx, query, batch)
		if err != nil {
			return nil, fmt.Errorf("cross encoder rerank failed: %w", err)
		}
		allScores = append(allScores, scores...)
	}

	// 组合并排序
	type scored struct {
		doc   rag.Document
		score float32
		rank  int
	}

	scoredDocs := make([]scored, len(docs))
	for i, doc := range docs {
		scoredDocs[i] = scored{doc: doc, score: allScores[i], rank: i}
	}

	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// 返回 TopK
	k := r.topK
	if k > len(scoredDocs) {
		k = len(scoredDocs)
	}

	result := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := scoredDocs[i].doc
		doc.Score = scoredDocs[i].score
		result[i] = doc
	}

	return result, nil
}

func (r *CrossEncoderReranker) scoreBatch(ctx context.Context, query string, docs []string) ([]float32, error) {
	reqBody := crossEncoderRequest{
		Query:     query,
		Documents: docs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.modelURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cross encoder returned status %d", resp.StatusCode)
	}

	var result crossEncoderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Scores, nil
}

// Name 返回重排序器名称
func (r *CrossEncoderReranker) Name() string {
	return "CrossEncoderReranker"
}

var _ Reranker = (*CrossEncoderReranker)(nil)

// ============== CohereReranker ==============

// CohereReranker 使用 Cohere Rerank API
type CohereReranker struct {
	apiKey     string
	model      string
	httpClient *http.Client
	topK       int
}

// CohereOption CohereReranker 选项
type CohereOption func(*CohereReranker)

// WithCohereAPIKey 设置 API Key
func WithCohereAPIKey(key string) CohereOption {
	return func(r *CohereReranker) {
		r.apiKey = key
	}
}

// WithCohereModel 设置模型
func WithCohereModel(model string) CohereOption {
	return func(r *CohereReranker) {
		r.model = model
	}
}

// WithCohereTopK 设置返回数量
func WithCohereTopK(k int) CohereOption {
	return func(r *CohereReranker) {
		r.topK = k
	}
}

// NewCohereReranker 创建 Cohere 重排序器
func NewCohereReranker(apiKey string, opts ...CohereOption) *CohereReranker {
	r := &CohereReranker{
		apiKey:     apiKey,
		model:      "rerank-english-v3.0",
		httpClient: &http.Client{},
		topK:       10,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type cohereRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	Model     string   `json:"model"`
	TopN      int      `json:"top_n,omitempty"`
}

type cohereRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank 使用 Cohere API 重排序
func (r *CohereReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 准备请求
	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	reqBody := cohereRerankRequest{
		Query:     query,
		Documents: contents,
		Model:     r.model,
		TopN:      r.topK,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.cohere.ai/v1/rerank", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cohere rerank returned status %d", resp.StatusCode)
	}

	var result cohereRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 构建结果
	reranked := make([]rag.Document, len(result.Results))
	for i, res := range result.Results {
		doc := docs[res.Index]
		doc.Score = float32(res.RelevanceScore)
		reranked[i] = doc
	}

	return reranked, nil
}

// Name 返回重排序器名称
func (r *CohereReranker) Name() string {
	return "CohereReranker"
}

var _ Reranker = (*CohereReranker)(nil)

// ============== LLMReranker ==============

// LLMProvider 是 LLM 提供者接口（简化版，避免循环依赖）
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMReranker 使用 LLM 进行重排序
type LLMReranker struct {
	llm         LLMProvider
	topK        int
	concurrency int
}

// LLMRerankerOption LLMReranker 选项
type LLMRerankerOption func(*LLMReranker)

// WithLLMRerankerTopK 设置返回数量
func WithLLMRerankerTopK(k int) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.topK = k
	}
}

// WithLLMRerankerConcurrency 设置并发数
func WithLLMRerankerConcurrency(c int) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.concurrency = c
	}
}

// NewLLMReranker 创建 LLM 重排序器
func NewLLMReranker(llm LLMProvider, opts ...LLMRerankerOption) *LLMReranker {
	r := &LLMReranker{
		llm:         llm,
		topK:        10,
		concurrency: 5,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 使用 LLM 重排序
func (r *LLMReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 并发评分
	type scored struct {
		idx   int
		doc   rag.Document
		score float32
		err   error
	}

	scoreCh := make(chan scored, len(docs))
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup

	for i, doc := range docs {
		wg.Add(1)
		go func(idx int, d rag.Document) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			score, err := r.scoreDocument(ctx, query, d.Content)
			scoreCh <- scored{idx: idx, doc: d, score: score, err: err}
		}(i, doc)
	}

	go func() {
		wg.Wait()
		close(scoreCh)
	}()

	// 收集结果
	scoredDocs := make([]scored, 0, len(docs))
	for s := range scoreCh {
		if s.err != nil {
			continue // 跳过失败的
		}
		scoredDocs = append(scoredDocs, s)
	}

	// 排序
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// 返回 TopK
	k := r.topK
	if k > len(scoredDocs) {
		k = len(scoredDocs)
	}

	result := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := scoredDocs[i].doc
		doc.Score = scoredDocs[i].score
		result[i] = doc
	}

	return result, nil
}

func (r *LLMReranker) scoreDocument(ctx context.Context, query, content string) (float32, error) {
	prompt := fmt.Sprintf(`Rate the relevance of the following document to the query on a scale of 0-10.
Only respond with a single number.

Query: %s

Document: %s

Relevance score (0-10):`, query, truncate(content, 1000))

	resp, err := r.llm.Complete(ctx, prompt)
	if err != nil {
		return 0, err
	}

	// 解析分数
	resp = strings.TrimSpace(resp)
	var score float32
	_, err = fmt.Sscanf(resp, "%f", &score)
	if err != nil {
		return 0, fmt.Errorf("failed to parse score: %s", resp)
	}

	return score / 10.0, nil // 归一化到 0-1
}

// Name 返回重排序器名称
func (r *LLMReranker) Name() string {
	return "LLMReranker"
}

var _ Reranker = (*LLMReranker)(nil)

// ============== RRFReranker ==============

// RRFReranker 使用 Reciprocal Rank Fusion 融合多个排名列表
type RRFReranker struct {
	k    float32 // RRF 常数，通常为 60
	topK int
}

// RRFOption RRFReranker 选项
type RRFOption func(*RRFReranker)

// WithRRFK 设置 RRF 常数
func WithRRFK(k float32) RRFOption {
	return func(r *RRFReranker) {
		r.k = k
	}
}

// WithRRFTopK 设置返回数量
func WithRRFTopK(k int) RRFOption {
	return func(r *RRFReranker) {
		r.topK = k
	}
}

// NewRRFReranker 创建 RRF 重排序器
func NewRRFReranker(opts ...RRFOption) *RRFReranker {
	r := &RRFReranker{
		k:    60,
		topK: 10,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// FuseRankings 融合多个排名列表
func (r *RRFReranker) FuseRankings(rankings ...[]rag.Document) []rag.Document {
	scoreMap := make(map[string]float32)
	docMap := make(map[string]rag.Document)

	for _, ranking := range rankings {
		for rank, doc := range ranking {
			// RRF 公式: 1 / (k + rank)
			rrf := 1.0 / (r.k + float32(rank+1))
			scoreMap[doc.ID] += rrf
			if _, exists := docMap[doc.ID]; !exists {
				docMap[doc.ID] = doc
			}
		}
	}

	// 排序
	type scored struct {
		id    string
		score float32
	}
	var scoredList []scored
	for id, score := range scoreMap {
		scoredList = append(scoredList, scored{id: id, score: score})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	// 返回 TopK
	k := r.topK
	if k > len(scoredList) {
		k = len(scoredList)
	}

	result := make([]rag.Document, k)
	for i := 0; i < k; i++ {
		doc := docMap[scoredList[i].id]
		doc.Score = scoredList[i].score
		result[i] = doc
	}

	return result
}

// Rerank 对单个列表基于原始分数重排序（实际上是原样返回 TopK）
func (r *RRFReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	// 单个列表时，RRF 退化为按原始分数排序
	sorted := make([]rag.Document, len(docs))
	copy(sorted, docs)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	k := r.topK
	if k > len(sorted) {
		k = len(sorted)
	}

	return sorted[:k], nil
}

// Name 返回重排序器名称
func (r *RRFReranker) Name() string {
	return "RRFReranker"
}

var _ Reranker = (*RRFReranker)(nil)

// ============== ScoreReranker ==============

// ScoreReranker 基于分数阈值过滤和重排序
type ScoreReranker struct {
	minScore float32
	topK     int
	normalize bool
}

// ScoreOption ScoreReranker 选项
type ScoreOption func(*ScoreReranker)

// WithScoreMin 设置最小分数
func WithScoreMin(score float32) ScoreOption {
	return func(r *ScoreReranker) {
		r.minScore = score
	}
}

// WithScoreTopK 设置返回数量
func WithScoreTopK(k int) ScoreOption {
	return func(r *ScoreReranker) {
		r.topK = k
	}
}

// WithScoreNormalize 设置是否归一化分数
func WithScoreNormalize(normalize bool) ScoreOption {
	return func(r *ScoreReranker) {
		r.normalize = normalize
	}
}

// NewScoreReranker 创建分数重排序器
func NewScoreReranker(opts ...ScoreOption) *ScoreReranker {
	r := &ScoreReranker{
		minScore:  0.0,
		topK:      10,
		normalize: false,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rerank 基于分数过滤和排序
func (r *ScoreReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 过滤低分文档
	filtered := make([]rag.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.Score >= r.minScore {
			filtered = append(filtered, doc)
		}
	}

	// 排序
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	// 归一化分数
	if r.normalize && len(filtered) > 0 {
		maxScore := filtered[0].Score
		minScore := filtered[len(filtered)-1].Score
		scoreRange := maxScore - minScore

		if scoreRange > 0 {
			for i := range filtered {
				filtered[i].Score = (filtered[i].Score - minScore) / scoreRange
			}
		}
	}

	// 返回 TopK
	k := r.topK
	if k > len(filtered) {
		k = len(filtered)
	}

	return filtered[:k], nil
}

// Name 返回重排序器名称
func (r *ScoreReranker) Name() string {
	return "ScoreReranker"
}

var _ Reranker = (*ScoreReranker)(nil)

// ============== ChainReranker ==============

// ChainReranker 链式重排序器，依次应用多个重排序器
type ChainReranker struct {
	rerankers []Reranker
}

// NewChainReranker 创建链式重排序器
func NewChainReranker(rerankers ...Reranker) *ChainReranker {
	return &ChainReranker{
		rerankers: rerankers,
	}
}

// Rerank 依次应用所有重排序器
func (r *ChainReranker) Rerank(ctx context.Context, query string, docs []rag.Document) ([]rag.Document, error) {
	result := docs

	for _, reranker := range r.rerankers {
		var err error
		result, err = reranker.Rerank(ctx, query, result)
		if err != nil {
			return nil, fmt.Errorf("%s failed: %w", reranker.Name(), err)
		}
	}

	return result, nil
}

// Name 返回重排序器名称
func (r *ChainReranker) Name() string {
	return "ChainReranker"
}

var _ Reranker = (*ChainReranker)(nil)

// ============== 辅助函数 ==============

// truncate 截断文本
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
