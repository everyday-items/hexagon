// Package search 提供搜索引擎工具
//
// 本包实现了多种搜索引擎工具：
//   - Google Search: Google 搜索 API
//   - Bing Search: Bing 搜索 API
//   - DuckDuckGo: DuckDuckGo Instant Answer API
//   - SerpAPI: SerpAPI 统一搜索接口
//   - Tavily: Tavily AI 搜索
//
// 设计借鉴：
//   - LangChain: SearchTools
//   - AutoGPT: Web Search
//
// 使用示例：
//
//	google := search.NewGoogleSearch(apiKey, cseID)
//	results, err := google.Execute(ctx, map[string]any{"query": "AI news"})
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ============== 错误定义 ==============

var (
	// ErrSearchFailed 搜索失败
	ErrSearchFailed = errors.New("search failed")

	// ErrRateLimited 被限流
	ErrRateLimited = errors.New("rate limited")

	// ErrInvalidAPIKey 无效的 API Key
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrNoResults 无搜索结果
	ErrNoResults = errors.New("no results found")
)

// ============== 搜索结果 ==============

// SearchResult 搜索结果
type SearchResult struct {
	// Title 标题
	Title string `json:"title"`

	// Link URL 链接
	Link string `json:"link"`

	// Snippet 摘要片段
	Snippet string `json:"snippet"`

	// Source 来源
	Source string `json:"source"`

	// PublishedDate 发布日期
	PublishedDate string `json:"published_date,omitempty"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	// Results 搜索结果列表
	Results []SearchResult `json:"results"`

	// TotalResults 总结果数
	TotalResults int64 `json:"total_results"`

	// SearchTime 搜索耗时（毫秒）
	SearchTime int64 `json:"search_time_ms"`

	// Query 原始查询
	Query string `json:"query"`

	// Source 搜索源
	Source string `json:"source"`
}

// SearchConfig 搜索配置
type SearchConfig struct {
	// MaxResults 最大结果数
	MaxResults int `json:"max_results"`

	// Language 语言
	Language string `json:"language"`

	// Region 地区
	Region string `json:"region"`

	// SafeSearch 安全搜索级别
	SafeSearch string `json:"safe_search"`

	// TimeRange 时间范围
	TimeRange string `json:"time_range"`

	// Timeout 超时时间
	Timeout time.Duration `json:"timeout"`
}

// DefaultSearchConfig 默认搜索配置
func DefaultSearchConfig() *SearchConfig {
	return &SearchConfig{
		MaxResults: 10,
		Language:   "en",
		SafeSearch: "moderate",
		Timeout:    30 * time.Second,
	}
}

// ============== 搜索工具接口 ==============

// SearchTool 搜索工具接口
type SearchTool interface {
	// Name 工具名称
	Name() string

	// Description 工具描述
	Description() string

	// Search 执行搜索
	Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error)
}

// ============== Google Search ==============

// GoogleSearch Google 搜索工具
type GoogleSearch struct {
	apiKey string
	cseID  string
	client *http.Client
}

// NewGoogleSearch 创建 Google 搜索工具
//
// 参数:
//   - apiKey: Google API Key
//   - cseID: Custom Search Engine ID
func NewGoogleSearch(apiKey, cseID string) *GoogleSearch {
	return &GoogleSearch{
		apiKey: apiKey,
		cseID:  cseID,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 工具名称
func (g *GoogleSearch) Name() string {
	return "google_search"
}

// Description 工具描述
func (g *GoogleSearch) Description() string {
	return "Search the web using Google Custom Search API. Returns relevant web pages, news, and information for any query."
}

// Search 执行 Google 搜索
func (g *GoogleSearch) Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error) {
	if config == nil {
		config = DefaultSearchConfig()
	}

	startTime := time.Now()

	// 构建 URL
	baseURL := "https://www.googleapis.com/customsearch/v1"
	params := url.Values{}
	params.Set("key", g.apiKey)
	params.Set("cx", g.cseID)
	params.Set("q", query)
	params.Set("num", fmt.Sprintf("%d", config.MaxResults))

	if config.Language != "" {
		params.Set("lr", "lang_"+config.Language)
	}
	if config.Region != "" {
		params.Set("gl", config.Region)
	}
	if config.SafeSearch != "" {
		params.Set("safe", config.SafeSearch)
	}

	reqURL := baseURL + "?" + params.Encode()

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: status code %d", ErrSearchFailed, resp.StatusCode)
	}

	// 解析响应
	var result googleSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	// 转换结果
	response := &SearchResponse{
		Results:      make([]SearchResult, 0, len(result.Items)),
		TotalResults: result.SearchInformation.TotalResults,
		SearchTime:   time.Since(startTime).Milliseconds(),
		Query:        query,
		Source:       "google",
	}

	for _, item := range result.Items {
		response.Results = append(response.Results, SearchResult{
			Title:   item.Title,
			Link:    item.Link,
			Snippet: item.Snippet,
			Source:  item.DisplayLink,
		})
	}

	return response, nil
}

// Schema 返回工具的 JSON Schema
func (g *GoogleSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to execute",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 10)",
				"default":     10,
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行工具（Tool 接口实现）
func (g *GoogleSearch) Execute(ctx context.Context, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, errors.New("query is required")
	}

	config := DefaultSearchConfig()
	if maxResults, ok := args["max_results"].(float64); ok {
		config.MaxResults = int(maxResults)
	}

	return g.Search(ctx, query, config)
}

// Validate 验证参数
func (g *GoogleSearch) Validate(args map[string]any) error {
	if _, ok := args["query"].(string); !ok {
		return errors.New("query must be a string")
	}
	return nil
}

type googleSearchResponse struct {
	SearchInformation struct {
		TotalResults int64 `json:"totalResults,string"`
	} `json:"searchInformation"`
	Items []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Snippet     string `json:"snippet"`
		DisplayLink string `json:"displayLink"`
	} `json:"items"`
}

// ============== Bing Search ==============

// BingSearch Bing 搜索工具
type BingSearch struct {
	apiKey string
	client *http.Client
}

// NewBingSearch 创建 Bing 搜索工具
func NewBingSearch(apiKey string) *BingSearch {
	return &BingSearch{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 工具名称
func (b *BingSearch) Name() string {
	return "bing_search"
}

// Description 工具描述
func (b *BingSearch) Description() string {
	return "Search the web using Bing Search API. Returns relevant web pages and information for any query."
}

// Search 执行 Bing 搜索
func (b *BingSearch) Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error) {
	if config == nil {
		config = DefaultSearchConfig()
	}

	startTime := time.Now()

	// 构建 URL
	baseURL := "https://api.bing.microsoft.com/v7.0/search"
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", config.MaxResults))

	if config.Language != "" {
		params.Set("mkt", config.Language+"-"+strings.ToUpper(config.Language))
	}
	if config.SafeSearch != "" {
		params.Set("safeSearch", config.SafeSearch)
	}

	reqURL := baseURL + "?" + params.Encode()

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode == 401 {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: status code %d", ErrSearchFailed, resp.StatusCode)
	}

	// 解析响应
	var result bingSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	// 转换结果
	response := &SearchResponse{
		Results:      make([]SearchResult, 0, len(result.WebPages.Value)),
		TotalResults: result.WebPages.TotalEstimatedMatches,
		SearchTime:   time.Since(startTime).Milliseconds(),
		Query:        query,
		Source:       "bing",
	}

	for _, item := range result.WebPages.Value {
		response.Results = append(response.Results, SearchResult{
			Title:   item.Name,
			Link:    item.URL,
			Snippet: item.Snippet,
			Source:  item.DisplayURL,
		})
	}

	return response, nil
}

// Schema 返回工具的 JSON Schema
func (b *BingSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to execute",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 10)",
				"default":     10,
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行工具
func (b *BingSearch) Execute(ctx context.Context, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, errors.New("query is required")
	}

	config := DefaultSearchConfig()
	if maxResults, ok := args["max_results"].(float64); ok {
		config.MaxResults = int(maxResults)
	}

	return b.Search(ctx, query, config)
}

// Validate 验证参数
func (b *BingSearch) Validate(args map[string]any) error {
	if _, ok := args["query"].(string); !ok {
		return errors.New("query must be a string")
	}
	return nil
}

type bingSearchResponse struct {
	WebPages struct {
		TotalEstimatedMatches int64 `json:"totalEstimatedMatches"`
		Value                 []struct {
			Name       string `json:"name"`
			URL        string `json:"url"`
			Snippet    string `json:"snippet"`
			DisplayURL string `json:"displayUrl"`
		} `json:"value"`
	} `json:"webPages"`
}

// ============== DuckDuckGo Search ==============

// DuckDuckGoSearch DuckDuckGo 搜索工具
type DuckDuckGoSearch struct {
	client *http.Client
}

// NewDuckDuckGoSearch 创建 DuckDuckGo 搜索工具
func NewDuckDuckGoSearch() *DuckDuckGoSearch {
	return &DuckDuckGoSearch{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 工具名称
func (d *DuckDuckGoSearch) Name() string {
	return "duckduckgo_search"
}

// Description 工具描述
func (d *DuckDuckGoSearch) Description() string {
	return "Search the web using DuckDuckGo Instant Answer API. Returns quick answers and related topics without tracking."
}

// Search 执行 DuckDuckGo 搜索
func (d *DuckDuckGoSearch) Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error) {
	if config == nil {
		config = DefaultSearchConfig()
	}

	startTime := time.Now()

	// 构建 URL (使用 Instant Answer API)
	baseURL := "https://api.duckduckgo.com/"
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("no_html", "1")
	params.Set("skip_disambig", "1")

	reqURL := baseURL + "?" + params.Encode()

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: status code %d", ErrSearchFailed, resp.StatusCode)
	}

	// 解析响应
	var result duckDuckGoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	// 转换结果
	response := &SearchResponse{
		Results:    make([]SearchResult, 0),
		SearchTime: time.Since(startTime).Milliseconds(),
		Query:      query,
		Source:     "duckduckgo",
	}

	// 添加抽象摘要
	if result.Abstract != "" {
		response.Results = append(response.Results, SearchResult{
			Title:   result.Heading,
			Link:    result.AbstractURL,
			Snippet: result.Abstract,
			Source:  result.AbstractSource,
		})
	}

	// 添加相关话题
	for _, topic := range result.RelatedTopics {
		if topic.FirstURL != "" {
			response.Results = append(response.Results, SearchResult{
				Title:   topic.Text,
				Link:    topic.FirstURL,
				Snippet: topic.Text,
				Source:  "duckduckgo",
			})
		}
		// 如果有子话题
		for _, subtopic := range topic.Topics {
			if subtopic.FirstURL != "" {
				response.Results = append(response.Results, SearchResult{
					Title:   subtopic.Text,
					Link:    subtopic.FirstURL,
					Snippet: subtopic.Text,
					Source:  "duckduckgo",
				})
			}
		}
	}

	// 限制结果数
	if len(response.Results) > config.MaxResults {
		response.Results = response.Results[:config.MaxResults]
	}

	response.TotalResults = int64(len(response.Results))
	return response, nil
}

// Schema 返回工具的 JSON Schema
func (d *DuckDuckGoSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to execute",
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行工具
func (d *DuckDuckGoSearch) Execute(ctx context.Context, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, errors.New("query is required")
	}

	config := DefaultSearchConfig()
	return d.Search(ctx, query, config)
}

// Validate 验证参数
func (d *DuckDuckGoSearch) Validate(args map[string]any) error {
	if _, ok := args["query"].(string); !ok {
		return errors.New("query must be a string")
	}
	return nil
}

type duckDuckGoResponse struct {
	Abstract       string `json:"Abstract"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Heading        string `json:"Heading"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
		Topics   []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Topics"`
	} `json:"RelatedTopics"`
}

// ============== Tavily Search ==============

// TavilySearch Tavily AI 搜索工具
type TavilySearch struct {
	apiKey string
	client *http.Client
}

// NewTavilySearch 创建 Tavily 搜索工具
func NewTavilySearch(apiKey string) *TavilySearch {
	return &TavilySearch{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 工具名称
func (t *TavilySearch) Name() string {
	return "tavily_search"
}

// Description 工具描述
func (t *TavilySearch) Description() string {
	return "Search the web using Tavily AI Search API. Optimized for AI agents with high-quality, relevant results."
}

// Search 执行 Tavily 搜索
func (t *TavilySearch) Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error) {
	if config == nil {
		config = DefaultSearchConfig()
	}

	startTime := time.Now()

	// 构建请求体
	requestBody := map[string]any{
		"api_key":            t.apiKey,
		"query":              query,
		"search_depth":       "basic",
		"include_answer":     true,
		"include_raw_content": false,
		"max_results":        config.MaxResults,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode == 401 {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status code %d, body: %s", ErrSearchFailed, resp.StatusCode, string(body))
	}

	// 解析响应
	var result tavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	// 转换结果
	response := &SearchResponse{
		Results:      make([]SearchResult, 0, len(result.Results)),
		TotalResults: int64(len(result.Results)),
		SearchTime:   time.Since(startTime).Milliseconds(),
		Query:        query,
		Source:       "tavily",
	}

	// 添加答案作为第一个结果（如果有）
	if result.Answer != "" {
		response.Results = append(response.Results, SearchResult{
			Title:   "AI Answer",
			Snippet: result.Answer,
			Source:  "tavily",
			Metadata: map[string]any{
				"is_answer": true,
			},
		})
	}

	for _, item := range result.Results {
		response.Results = append(response.Results, SearchResult{
			Title:         item.Title,
			Link:          item.URL,
			Snippet:       item.Content,
			Source:        item.URL,
			PublishedDate: item.PublishedDate,
			Metadata: map[string]any{
				"score": item.Score,
			},
		})
	}

	return response, nil
}

// Schema 返回工具的 JSON Schema
func (t *TavilySearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to execute",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 10)",
				"default":     10,
			},
		},
		"required": []string{"query"},
	}
}

// Execute 执行工具
func (t *TavilySearch) Execute(ctx context.Context, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, errors.New("query is required")
	}

	config := DefaultSearchConfig()
	if maxResults, ok := args["max_results"].(float64); ok {
		config.MaxResults = int(maxResults)
	}

	return t.Search(ctx, query, config)
}

// Validate 验证参数
func (t *TavilySearch) Validate(args map[string]any) error {
	if _, ok := args["query"].(string); !ok {
		return errors.New("query must be a string")
	}
	return nil
}

type tavilySearchResponse struct {
	Answer  string `json:"answer"`
	Results []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Content       string  `json:"content"`
		Score         float64 `json:"score"`
		PublishedDate string  `json:"published_date"`
	} `json:"results"`
}

// ============== 聚合搜索 ==============

// AggregatedSearch 聚合搜索工具
// 同时使用多个搜索引擎并合并结果
type AggregatedSearch struct {
	engines []SearchTool
}

// NewAggregatedSearch 创建聚合搜索工具
func NewAggregatedSearch(engines ...SearchTool) *AggregatedSearch {
	return &AggregatedSearch{
		engines: engines,
	}
}

// Name 工具名称
func (a *AggregatedSearch) Name() string {
	return "aggregated_search"
}

// Description 工具描述
func (a *AggregatedSearch) Description() string {
	return "Search multiple search engines simultaneously and aggregate results for comprehensive coverage."
}

// Search 执行聚合搜索
func (a *AggregatedSearch) Search(ctx context.Context, query string, config *SearchConfig) (*SearchResponse, error) {
	if config == nil {
		config = DefaultSearchConfig()
	}

	startTime := time.Now()

	// 并行搜索所有引擎
	type searchResult struct {
		response *SearchResponse
		err      error
	}

	resultCh := make(chan searchResult, len(a.engines))

	for _, engine := range a.engines {
		engine := engine
		go func() {
			resp, err := engine.Search(ctx, query, config)
			resultCh <- searchResult{response: resp, err: err}
		}()
	}

	// 收集结果
	allResults := make([]SearchResult, 0)
	var totalResults int64

	for range a.engines {
		result := <-resultCh
		if result.err != nil {
			continue // 忽略失败的搜索
		}
		if result.response != nil {
			allResults = append(allResults, result.response.Results...)
			totalResults += result.response.TotalResults
		}
	}

	// 去重和排序
	seen := make(map[string]bool)
	uniqueResults := make([]SearchResult, 0)
	for _, result := range allResults {
		if !seen[result.Link] {
			seen[result.Link] = true
			uniqueResults = append(uniqueResults, result)
		}
	}

	// 限制结果数
	if len(uniqueResults) > config.MaxResults {
		uniqueResults = uniqueResults[:config.MaxResults]
	}

	return &SearchResponse{
		Results:      uniqueResults,
		TotalResults: totalResults,
		SearchTime:   time.Since(startTime).Milliseconds(),
		Query:        query,
		Source:       "aggregated",
	}, nil
}
