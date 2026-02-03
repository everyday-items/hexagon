// Package query 提供 RAG 系统的查询增强功能
//
// Query Enhancement 用于提升检索质量：
//   - QueryExpander: 查询扩展，添加相关术语
//   - QueryRewriter: 查询重写，改进查询表达
//   - MultiQueryGenerator: 多查询生成，从不同角度检索
//   - HyDEGenerator: 假设文档生成，基于假设答案检索
package query

import (
	"context"
	"fmt"
	"strings"
)

// LLMProvider 是 LLM 提供者接口（简化版）
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// ============== QueryExpander ==============

// QueryExpander 查询扩展器
//
// 通过添加同义词、相关术语等方式扩展查询，提高召回率。
// 可以使用 LLM 或词典方式进行扩展。
type QueryExpander struct {
	llm           LLMProvider
	maxExpansions int    // 最大扩展数量
	strategy      string // 扩展策略: llm, synonym, both
}

// ExpanderOption QueryExpander 选项
type ExpanderOption func(*QueryExpander)

// WithExpanderMaxExpansions 设置最大扩展数量
func WithExpanderMaxExpansions(max int) ExpanderOption {
	return func(e *QueryExpander) {
		e.maxExpansions = max
	}
}

// WithExpanderStrategy 设置扩展策略
func WithExpanderStrategy(strategy string) ExpanderOption {
	return func(e *QueryExpander) {
		e.strategy = strategy
	}
}

// NewQueryExpander 创建查询扩展器
func NewQueryExpander(llm LLMProvider, opts ...ExpanderOption) *QueryExpander {
	e := &QueryExpander{
		llm:           llm,
		maxExpansions: 3,
		strategy:      "llm",
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Expand 扩展查询
//
// 输入原始查询，返回扩展后的查询列表（包含原始查询）
func (e *QueryExpander) Expand(ctx context.Context, query string) ([]string, error) {
	switch e.strategy {
	case "llm":
		return e.expandWithLLM(ctx, query)
	case "synonym":
		return e.expandWithSynonym(query), nil
	case "both":
		llmQueries, err := e.expandWithLLM(ctx, query)
		if err != nil {
			// 降级到同义词扩展
			return e.expandWithSynonym(query), nil
		}
		synQueries := e.expandWithSynonym(query)
		// 合并去重
		seen := make(map[string]bool)
		var result []string
		for _, q := range append(llmQueries, synQueries...) {
			if !seen[q] {
				seen[q] = true
				result = append(result, q)
			}
		}
		return result, nil
	default:
		return []string{query}, nil
	}
}

func (e *QueryExpander) expandWithLLM(ctx context.Context, query string) ([]string, error) {
	prompt := fmt.Sprintf(`Expand the following query by providing %d alternative phrasings that capture the same intent.
Each phrasing should be on a new line.

Original query: %s

Alternative phrasings:`, e.maxExpansions, query)

	resp, err := e.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("query expansion failed: %w", err)
	}

	// 解析响应
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	expansions := []string{query} // 始终包含原始查询

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 移除编号前缀 (1., 2., -, * 等)
		line = strings.TrimLeft(line, "0123456789.-*• ")
		if line != "" && line != query {
			expansions = append(expansions, line)
			if len(expansions) > e.maxExpansions {
				break
			}
		}
	}

	return expansions, nil
}

func (e *QueryExpander) expandWithSynonym(query string) []string {
	// 简单的同义词扩展（可以接入外部词典）
	// 这里提供基础实现
	expansions := []string{query}

	// 简单的术语替换示例
	synonyms := map[string][]string{
		"AI":              {"artificial intelligence", "machine learning", "人工智能"},
		"machine learning": {"ML", "AI", "机器学习"},
		"database":        {"DB", "data store", "数据库"},
		"search":          {"query", "find", "lookup", "搜索", "查询"},
		"document":        {"doc", "file", "文档"},
	}

	queryLower := strings.ToLower(query)
	for term, syns := range synonyms {
		if strings.Contains(queryLower, strings.ToLower(term)) {
			for _, syn := range syns {
				expanded := strings.ReplaceAll(queryLower, strings.ToLower(term), syn)
				if expanded != queryLower {
					expansions = append(expansions, expanded)
				}
			}
		}
	}

	// 限制数量
	if len(expansions) > e.maxExpansions+1 {
		expansions = expansions[:e.maxExpansions+1]
	}

	return expansions
}

// ============== QueryRewriter ==============

// QueryRewriter 查询重写器
//
// 使用 LLM 重写查询，使其更清晰、更具体、更易检索。
type QueryRewriter struct {
	llm  LLMProvider
	mode string // 重写模式: clarify, simplify, specify
}

// RewriterOption QueryRewriter 选项
type RewriterOption func(*QueryRewriter)

// WithRewriterMode 设置重写模式
func WithRewriterMode(mode string) RewriterOption {
	return func(r *QueryRewriter) {
		r.mode = mode
	}
}

// NewQueryRewriter 创建查询重写器
func NewQueryRewriter(llm LLMProvider, opts ...RewriterOption) *QueryRewriter {
	r := &QueryRewriter{
		llm:  llm,
		mode: "clarify",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rewrite 重写查询
func (r *QueryRewriter) Rewrite(ctx context.Context, query string) (string, error) {
	var prompt string

	switch r.mode {
	case "clarify":
		prompt = fmt.Sprintf(`Rewrite the following query to make it clearer and more specific for document retrieval.
Keep it concise but unambiguous.

Original query: %s

Rewritten query:`, query)

	case "simplify":
		prompt = fmt.Sprintf(`Simplify the following query while preserving its core intent.
Remove unnecessary words and make it more direct.

Original query: %s

Simplified query:`, query)

	case "specify":
		prompt = fmt.Sprintf(`Make the following query more specific by adding relevant context or details that would improve retrieval.

Original query: %s

More specific query:`, query)

	default:
		return query, nil
	}

	resp, err := r.llm.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("query rewriting failed: %w", err)
	}

	rewritten := strings.TrimSpace(resp)
	if rewritten == "" {
		return query, nil
	}

	return rewritten, nil
}

// ============== MultiQueryGenerator ==============

// MultiQueryGenerator 多查询生成器
//
// 从不同角度生成多个查询，提高检索覆盖率。
// 这对于复杂问题特别有用，可以从多个侧面检索相关信息。
type MultiQueryGenerator struct {
	llm            LLMProvider
	numQueries     int  // 生成查询数量
	includeSelf    bool // 是否包含原始查询
	diversityBoost bool // 是否增强多样性
}

// MultiQueryOption MultiQueryGenerator 选项
type MultiQueryOption func(*MultiQueryGenerator)

// WithNumQueries 设置生成查询数量
func WithNumQueries(num int) MultiQueryOption {
	return func(g *MultiQueryGenerator) {
		g.numQueries = num
	}
}

// WithIncludeSelf 设置是否包含原始查询
func WithIncludeSelf(include bool) MultiQueryOption {
	return func(g *MultiQueryGenerator) {
		g.includeSelf = include
	}
}

// WithDiversityBoost 设置是否增强多样性
func WithDiversityBoost(boost bool) MultiQueryOption {
	return func(g *MultiQueryGenerator) {
		g.diversityBoost = boost
	}
}

// NewMultiQueryGenerator 创建多查询生成器
func NewMultiQueryGenerator(llm LLMProvider, opts ...MultiQueryOption) *MultiQueryGenerator {
	g := &MultiQueryGenerator{
		llm:            llm,
		numQueries:     3,
		includeSelf:    true,
		diversityBoost: false,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Generate 生成多个查询
func (g *MultiQueryGenerator) Generate(ctx context.Context, query string) ([]string, error) {
	var prompt string

	if g.diversityBoost {
		prompt = fmt.Sprintf(`Generate %d diverse search queries that approach the following question from different angles.
Each query should capture a different aspect or perspective of the original question.
Provide one query per line.

Original question: %s

Diverse queries:`, g.numQueries, query)
	} else {
		prompt = fmt.Sprintf(`Generate %d alternative search queries that would help answer the following question.
Provide one query per line.

Original question: %s

Alternative queries:`, g.numQueries, query)
	}

	resp, err := g.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("multi-query generation failed: %w", err)
	}

	// 解析响应
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	var queries []string

	if g.includeSelf {
		queries = append(queries, query)
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 移除编号前缀
		line = strings.TrimLeft(line, "0123456789.-*• ")
		if line != "" && line != query {
			queries = append(queries, line)
			if len(queries) >= g.numQueries+(func() int {
				if g.includeSelf {
					return 1
				}
				return 0
			}()) {
				break
			}
		}
	}

	// 确保至少有原始查询
	if len(queries) == 0 {
		queries = append(queries, query)
	}

	return queries, nil
}

// ============== HyDEGenerator ==============

// HyDEGenerator 假设文档嵌入生成器
//
// HyDE (Hypothetical Document Embeddings) 是一种创新的检索策略：
// 1. 根据查询生成一个假设的答案文档
// 2. 使用假设文档的嵌入进行检索，而不是查询的嵌入
// 3. 因为文档-文档相似度通常高于查询-文档相似度，可以提高检索质量
type HyDEGenerator struct {
	llm         LLMProvider
	docLength   int    // 生成文档长度
	temperature float32 // LLM 温度参数
}

// HyDEOption HyDEGenerator 选项
type HyDEOption func(*HyDEGenerator)

// WithHyDEDocLength 设置生成文档长度
func WithHyDEDocLength(length int) HyDEOption {
	return func(g *HyDEGenerator) {
		g.docLength = length
	}
}

// WithHyDETemperature 设置 LLM 温度
func WithHyDETemperature(temp float32) HyDEOption {
	return func(g *HyDEGenerator) {
		g.temperature = temp
	}
}

// NewHyDEGenerator 创建 HyDE 生成器
func NewHyDEGenerator(llm LLMProvider, opts ...HyDEOption) *HyDEGenerator {
	g := &HyDEGenerator{
		llm:         llm,
		docLength:   200,
		temperature: 0.7,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Generate 生成假设文档
func (g *HyDEGenerator) Generate(ctx context.Context, query string) (string, error) {
	prompt := fmt.Sprintf(`Write a passage that would answer the following question.
The passage should be factual, informative, and approximately %d words.

Question: %s

Answer passage:`, g.docLength, query)

	resp, err := g.llm.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("HyDE generation failed: %w", err)
	}

	hypotheticalDoc := strings.TrimSpace(resp)
	if hypotheticalDoc == "" {
		return query, nil // 降级到原始查询
	}

	return hypotheticalDoc, nil
}

// GenerateMultiple 生成多个假设文档
//
// 通过生成多个假设文档并检索，可以进一步提高覆盖率
func (g *HyDEGenerator) GenerateMultiple(ctx context.Context, query string, count int) ([]string, error) {
	docs := make([]string, 0, count)

	for i := 0; i < count; i++ {
		doc, err := g.Generate(ctx, query)
		if err != nil {
			continue // 跳过失败的生成
		}
		docs = append(docs, doc)
	}

	// 确保至少有一个结果
	if len(docs) == 0 {
		docs = append(docs, query)
	}

	return docs, nil
}

// ============== QueryProcessor ==============

// QueryProcessor 查询处理器
//
// 组合多种查询增强技术，提供统一的查询处理接口
type QueryProcessor struct {
	expander  *QueryExpander
	rewriter  *QueryRewriter
	multiGen  *MultiQueryGenerator
	hydeGen   *HyDEGenerator
	enableAll bool // 是否启用所有增强
}

// ProcessorOption QueryProcessor 选项
type ProcessorOption func(*QueryProcessor)

// WithExpander 设置查询扩展器
func WithExpander(expander *QueryExpander) ProcessorOption {
	return func(p *QueryProcessor) {
		p.expander = expander
	}
}

// WithRewriter 设置查询重写器
func WithRewriter(rewriter *QueryRewriter) ProcessorOption {
	return func(p *QueryProcessor) {
		p.rewriter = rewriter
	}
}

// WithMultiGen 设置多查询生成器
func WithMultiGen(multiGen *MultiQueryGenerator) ProcessorOption {
	return func(p *QueryProcessor) {
		p.multiGen = multiGen
	}
}

// WithHyDEGen 设置 HyDE 生成器
func WithHyDEGen(hydeGen *HyDEGenerator) ProcessorOption {
	return func(p *QueryProcessor) {
		p.hydeGen = hydeGen
	}
}

// WithEnableAll 设置是否启用所有增强
func WithEnableAll(enable bool) ProcessorOption {
	return func(p *QueryProcessor) {
		p.enableAll = enable
	}
}

// NewQueryProcessor 创建查询处理器
func NewQueryProcessor(opts ...ProcessorOption) *QueryProcessor {
	p := &QueryProcessor{
		enableAll: false,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ProcessResult 查询处理结果
type ProcessResult struct {
	Original       string   // 原始查询
	Rewritten      string   // 重写后的查询
	Expanded       []string // 扩展查询列表
	MultiQueries   []string // 多角度查询列表
	HypotheticalDoc string   // 假设文档
}

// Process 处理查询
//
// 根据配置应用各种查询增强技术
func (p *QueryProcessor) Process(ctx context.Context, query string) (*ProcessResult, error) {
	result := &ProcessResult{
		Original: query,
	}

	// 查询重写
	if p.rewriter != nil {
		rewritten, err := p.rewriter.Rewrite(ctx, query)
		if err == nil {
			result.Rewritten = rewritten
			query = rewritten // 后续使用重写后的查询
		}
	}

	// 查询扩展
	if p.expander != nil {
		expanded, err := p.expander.Expand(ctx, query)
		if err == nil {
			result.Expanded = expanded
		}
	}

	// 多查询生成
	if p.multiGen != nil {
		multiQueries, err := p.multiGen.Generate(ctx, query)
		if err == nil {
			result.MultiQueries = multiQueries
		}
	}

	// HyDE 生成
	if p.hydeGen != nil {
		hydeDoc, err := p.hydeGen.Generate(ctx, query)
		if err == nil {
			result.HypotheticalDoc = hydeDoc
		}
	}

	return result, nil
}

// GetAllQueries 获取所有生成的查询
//
// 返回所有查询的去重列表，用于多路检索
func (p *ProcessResult) GetAllQueries() []string {
	seen := make(map[string]bool)
	var queries []string

	add := func(q string) {
		if q != "" && !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}

	// 顺序很重要：原始 -> 重写 -> 扩展 -> 多查询
	add(p.Original)
	add(p.Rewritten)
	for _, q := range p.Expanded {
		add(q)
	}
	for _, q := range p.MultiQueries {
		add(q)
	}

	return queries
}
