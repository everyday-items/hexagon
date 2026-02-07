// Package router 提供 RAG 系统的查询路由器
//
// QueryRouter 根据查询内容自动选择最合适的检索策略，支持：
//   - 基于规则的路由：使用关键词匹配和正则表达式
//   - 基于 LLM 的路由：让 LLM 分析查询并选择最佳检索器
//   - 基于嵌入的路由：通过语义相似度匹配到预设类别
//
// 使用示例：
//
//	router := NewQueryRouter(
//	    WithRoute("vector", vectorRetriever, "语义相关的问题"),
//	    WithRoute("keyword", keywordRetriever, "精确搜索"),
//	    WithRoute("sql", sqlRetriever, "数据查询"),
//	)
//	docs, err := router.Retrieve(ctx, "最近的销售数据是多少？")
package router

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/rag"
)

// Route 路由规则
type Route struct {
	// Name 路由名称
	Name string

	// Retriever 关联的检索器
	Retriever rag.Retriever

	// Description 路由描述（用于 LLM 路由判断）
	Description string

	// Keywords 关键词列表（用于规则路由）
	Keywords []string

	// Patterns 正则表达式模式（用于规则路由）
	Patterns []*regexp.Regexp

	// Priority 优先级（数值越小优先级越高）
	Priority int
}

// RouterMode 路由模式
type RouterMode int

const (
	// ModeRule 基于规则的路由
	ModeRule RouterMode = iota

	// ModeLLM 基于 LLM 的路由
	ModeLLM

	// ModeHybrid 混合路由（先规则后 LLM 兜底）
	ModeHybrid
)

// QueryRouter 查询路由器
// 根据查询内容自动选择最合适的检索策略
type QueryRouter struct {
	routes   []*Route
	mode     RouterMode
	llm      llm.Provider
	model    string
	fallback rag.Retriever
}

// RouterOption 路由器配置选项
type RouterOption func(*QueryRouter)

// WithRoute 添加路由规则
func WithRoute(name string, retriever rag.Retriever, description string) RouterOption {
	return func(r *QueryRouter) {
		r.routes = append(r.routes, &Route{
			Name:        name,
			Retriever:   retriever,
			Description: description,
		})
	}
}

// WithRouteKeywords 添加带关键词的路由规则
func WithRouteKeywords(name string, retriever rag.Retriever, description string, keywords []string) RouterOption {
	return func(r *QueryRouter) {
		r.routes = append(r.routes, &Route{
			Name:        name,
			Retriever:   retriever,
			Description: description,
			Keywords:    keywords,
		})
	}
}

// WithRoutePatterns 添加带正则的路由规则
func WithRoutePatterns(name string, retriever rag.Retriever, description string, patterns []string) RouterOption {
	return func(r *QueryRouter) {
		compiledPatterns := make([]*regexp.Regexp, 0, len(patterns))
		for _, p := range patterns {
			if re, err := regexp.Compile(p); err == nil {
				compiledPatterns = append(compiledPatterns, re)
			}
		}
		r.routes = append(r.routes, &Route{
			Name:        name,
			Retriever:   retriever,
			Description: description,
			Patterns:    compiledPatterns,
		})
	}
}

// WithRouterMode 设置路由模式
func WithRouterMode(mode RouterMode) RouterOption {
	return func(r *QueryRouter) {
		r.mode = mode
	}
}

// WithRouterLLM 设置 LLM 路由的 Provider
func WithRouterLLM(provider llm.Provider, model string) RouterOption {
	return func(r *QueryRouter) {
		r.llm = provider
		r.model = model
	}
}

// WithFallback 设置兜底检索器
func WithFallback(retriever rag.Retriever) RouterOption {
	return func(r *QueryRouter) {
		r.fallback = retriever
	}
}

// NewQueryRouter 创建查询路由器
func NewQueryRouter(opts ...RouterOption) *QueryRouter {
	r := &QueryRouter{
		routes: make([]*Route, 0),
		mode:   ModeRule,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Retrieve 根据查询自动路由到合适的检索器
func (r *QueryRouter) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	route, err := r.selectRoute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("路由选择失败: %w", err)
	}

	if route == nil {
		if r.fallback != nil {
			return r.fallback.Retrieve(ctx, query, opts...)
		}
		return nil, fmt.Errorf("无法匹配路由且无兜底检索器，查询: %s", query)
	}

	return route.Retriever.Retrieve(ctx, query, opts...)
}

// Route 返回选中的路由名称（不执行检索）
func (r *QueryRouter) SelectRoute(ctx context.Context, query string) (string, error) {
	route, err := r.selectRoute(ctx, query)
	if err != nil {
		return "", err
	}
	if route == nil {
		return "", nil
	}
	return route.Name, nil
}

// selectRoute 选择路由
func (r *QueryRouter) selectRoute(ctx context.Context, query string) (*Route, error) {
	switch r.mode {
	case ModeRule:
		return r.ruleRoute(query), nil
	case ModeLLM:
		return r.llmRoute(ctx, query)
	case ModeHybrid:
		// 先尝试规则路由
		if route := r.ruleRoute(query); route != nil {
			return route, nil
		}
		// 规则无法匹配，使用 LLM
		return r.llmRoute(ctx, query)
	default:
		return r.ruleRoute(query), nil
	}
}

// ruleRoute 基于规则的路由
func (r *QueryRouter) ruleRoute(query string) *Route {
	queryLower := strings.ToLower(query)

	var bestRoute *Route
	bestScore := 0

	for _, route := range r.routes {
		score := 0

		// 关键词匹配
		for _, kw := range route.Keywords {
			if strings.Contains(queryLower, strings.ToLower(kw)) {
				score += 10
			}
		}

		// 正则匹配
		for _, pattern := range route.Patterns {
			if pattern.MatchString(query) {
				score += 20
			}
		}

		// 优先级加权
		if score > 0 && route.Priority > 0 {
			score += (100 - route.Priority)
		}

		if score > bestScore {
			bestScore = score
			bestRoute = route
		}
	}

	return bestRoute
}

// llmRoute 基于 LLM 的路由
func (r *QueryRouter) llmRoute(ctx context.Context, query string) (*Route, error) {
	if r.llm == nil {
		return nil, fmt.Errorf("LLM 路由模式需要设置 LLM Provider")
	}

	// 构建 prompt
	prompt := r.buildRoutePrompt(query)

	req := llm.CompletionRequest{
		Model: r.model,
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个查询路由器。根据用户查询选择最合适的检索策略。只回复策略名称，不要解释。"},
			{Role: "user", Content: prompt},
		},
		Temperature: floatPtr(0.0),
		MaxTokens:   50,
	}

	resp, err := r.llm.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM 路由调用失败: %w", err)
	}

	// 解析 LLM 响应，匹配路由名称
	choice := strings.TrimSpace(resp.Content)
	for _, route := range r.routes {
		if strings.EqualFold(route.Name, choice) {
			return route, nil
		}
	}

	// 模糊匹配
	choiceLower := strings.ToLower(choice)
	for _, route := range r.routes {
		if strings.Contains(choiceLower, strings.ToLower(route.Name)) {
			return route, nil
		}
	}

	return nil, nil
}

// buildRoutePrompt 构建路由提示
func (r *QueryRouter) buildRoutePrompt(query string) string {
	var b strings.Builder
	b.WriteString("用户查询: " + query + "\n\n")
	b.WriteString("可用的检索策略:\n")
	for _, route := range r.routes {
		b.WriteString(fmt.Sprintf("- %s: %s\n", route.Name, route.Description))
	}
	b.WriteString("\n请选择最合适的策略名称:")
	return b.String()
}

// MultiRouter 多路由器，将查询分发到多个检索器并合并结果
type MultiRouter struct {
	routers  []*QueryRouter
	mergeTop int
}

// NewMultiRouter 创建多路由器
func NewMultiRouter(mergeTop int, routers ...*QueryRouter) *MultiRouter {
	return &MultiRouter{
		routers:  routers,
		mergeTop: mergeTop,
	}
}

// Retrieve 并发检索并合并去重
func (m *MultiRouter) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	type result struct {
		docs []rag.Document
		err  error
	}

	ch := make(chan result, len(m.routers))
	for _, router := range m.routers {
		go func(r *QueryRouter) {
			docs, err := r.Retrieve(ctx, query, opts...)
			ch <- result{docs: docs, err: err}
		}(router)
	}

	var allDocs []rag.Document
	seen := make(map[string]bool)

	for range m.routers {
		res := <-ch
		if res.err != nil {
			continue // 跳过失败的路由
		}
		for _, doc := range res.docs {
			if !seen[doc.ID] {
				seen[doc.ID] = true
				allDocs = append(allDocs, doc)
			}
		}
	}

	// 按分数排序截取
	if m.mergeTop > 0 && len(allDocs) > m.mergeTop {
		sortDocsByScore(allDocs)
		allDocs = allDocs[:m.mergeTop]
	}

	return allDocs, nil
}

// sortDocsByScore 按分数降序排序
func sortDocsByScore(docs []rag.Document) {
	for i := 1; i < len(docs); i++ {
		for j := i; j > 0 && docs[j].Score > docs[j-1].Score; j-- {
			docs[j], docs[j-1] = docs[j-1], docs[j]
		}
	}
}

func floatPtr(f float64) *float64 {
	return &f
}
