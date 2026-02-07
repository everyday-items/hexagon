// Package retriever 提供 RAG 系统的文档检索器
//
// adaptive.go 实现自适应检索 (Adaptive Retrieval)：
//   - AdaptiveRetriever: 根据查询复杂度自动调整检索策略和参数
//   - QueryClassifier: 查询分类器，判断查询类型和复杂度
//   - StrategySelector: 策略选择器，根据分类结果选择最优检索方案
//
// 对标 LangChain/LlamaIndex 的自适应检索能力。
//
// 使用示例：
//
//	adaptive := NewAdaptiveRetriever(
//	    WithBaseRetriever(vectorRetriever),
//	    WithFallbackRetriever(keywordRetriever),
//	    WithClassifier(NewRuleClassifier()),
//	)
//	docs, err := adaptive.Retrieve(ctx, "复杂的多实体关联查询")
package retriever

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/everyday-items/hexagon/rag"
)

// ============== 查询复杂度与分类 ==============

// QueryComplexity 查询复杂度
type QueryComplexity int

const (
	// ComplexitySimple 简单查询（关键词、短问题）
	ComplexitySimple QueryComplexity = iota
	// ComplexityModerate 中等复杂度（需要一些推理）
	ComplexityModerate
	// ComplexityComplex 复杂查询（多实体、多步推理）
	ComplexityComplex
)

// QueryType 查询类型
type QueryType int

const (
	// QueryTypeFactual 事实型查询（"X 是什么"）
	QueryTypeFactual QueryType = iota
	// QueryTypeAnalytical 分析型查询（"为什么"、"怎样"）
	QueryTypeAnalytical
	// QueryTypeComparative 比较型查询（"A 和 B 的区别"）
	QueryTypeComparative
	// QueryTypeAggregation 聚合型查询（"列出所有"）
	QueryTypeAggregation
)

// QueryClassification 查询分类结果
type QueryClassification struct {
	// Complexity 查询复杂度
	Complexity QueryComplexity

	// Type 查询类型
	Type QueryType

	// Keywords 提取的关键词
	Keywords []string

	// Score 置信度 (0-1)
	Score float64
}

// ============== 查询分类器 ==============

// QueryClassifier 查询分类器接口
type QueryClassifier interface {
	// Classify 对查询进行分类
	Classify(ctx context.Context, query string) (*QueryClassification, error)
}

// RuleClassifier 基于规则的查询分类器
type RuleClassifier struct {
	// 复杂度指标的词汇
	complexWords []string
	compareWords []string
	aggrWords    []string
}

// NewRuleClassifier 创建基于规则的分类器
func NewRuleClassifier() *RuleClassifier {
	return &RuleClassifier{
		complexWords: []string{"为什么", "如何", "怎样", "影响", "原因", "关系", "区别",
			"why", "how", "impact", "cause", "relationship", "difference"},
		compareWords: []string{"比较", "区别", "不同", "对比", "优缺点", "vs",
			"compare", "difference", "versus", "pros and cons"},
		aggrWords: []string{"列出", "所有", "哪些", "多少", "统计",
			"list", "all", "which", "how many", "count"},
	}
}

// Classify 使用规则分类查询
func (c *RuleClassifier) Classify(_ context.Context, query string) (*QueryClassification, error) {
	queryLower := strings.ToLower(query)
	result := &QueryClassification{
		Complexity: ComplexitySimple,
		Type:       QueryTypeFactual,
		Score:      0.8,
	}

	// 提取关键词（简单按空格分词）
	result.Keywords = strings.Fields(query)

	// 判断复杂度
	wordCount := utf8.RuneCountInString(query)
	complexScore := 0.0

	// 长度因子
	if wordCount > 50 {
		complexScore += 0.4
	} else if wordCount > 20 {
		complexScore += 0.2
	}

	// 关键词因子
	for _, w := range c.complexWords {
		if strings.Contains(queryLower, w) {
			complexScore += 0.3
			break
		}
	}

	// 问号数量
	if strings.Count(query, "?") + strings.Count(query, "？") > 1 {
		complexScore += 0.2
	}

	if complexScore >= 0.6 {
		result.Complexity = ComplexityComplex
	} else if complexScore >= 0.3 {
		result.Complexity = ComplexityModerate
	}

	// 判断查询类型
	for _, w := range c.compareWords {
		if strings.Contains(queryLower, w) {
			result.Type = QueryTypeComparative
			return result, nil
		}
	}
	for _, w := range c.aggrWords {
		if strings.Contains(queryLower, w) {
			result.Type = QueryTypeAggregation
			return result, nil
		}
	}
	if result.Complexity >= ComplexityModerate {
		result.Type = QueryTypeAnalytical
	}

	return result, nil
}

// ============== 检索策略 ==============

// RetrievalStrategy 检索策略
type RetrievalStrategy struct {
	// TopK 返回文档数量
	TopK int

	// MinScore 最低相关性分数
	MinScore float32

	// UseReranker 是否启用重排序
	UseReranker bool

	// MultiQuery 是否启用多查询扩展
	MultiQuery bool

	// RetrieverName 使用的检索器名称
	RetrieverName string
}

// ============== 自适应检索器 ==============

// AdaptiveRetriever 自适应检索器
// 根据查询的复杂度和类型，自动调整检索策略
type AdaptiveRetriever struct {
	// 基础检索器
	baseRetriever rag.Retriever

	// 备选检索器映射
	retrievers map[string]rag.Retriever

	// 查询分类器
	classifier QueryClassifier

	// 策略映射（复杂度 -> 策略）
	strategies map[QueryComplexity]*RetrievalStrategy

	// 默认参数
	defaultTopK     int
	defaultMinScore float32
}

// AdaptiveOption 自适应检索器选项
type AdaptiveOption func(*AdaptiveRetriever)

// WithBaseRetriever 设置基础检索器
func WithBaseRetriever(r rag.Retriever) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.baseRetriever = r
	}
}

// WithNamedRetriever 添加命名检索器
func WithNamedRetriever(name string, r rag.Retriever) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.retrievers[name] = r
	}
}

// WithClassifier 设置查询分类器
func WithClassifier(c QueryClassifier) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.classifier = c
	}
}

// WithDefaultTopK 设置默认 TopK
func WithDefaultTopK(k int) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.defaultTopK = k
	}
}

// WithDefaultMinScore 设置默认最小分数
func WithDefaultMinScore(score float32) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.defaultMinScore = score
	}
}

// WithComplexityStrategy 设置特定复杂度的策略
func WithComplexityStrategy(complexity QueryComplexity, strategy *RetrievalStrategy) AdaptiveOption {
	return func(a *AdaptiveRetriever) {
		a.strategies[complexity] = strategy
	}
}

// NewAdaptiveRetriever 创建自适应检索器
func NewAdaptiveRetriever(opts ...AdaptiveOption) *AdaptiveRetriever {
	r := &AdaptiveRetriever{
		retrievers:      make(map[string]rag.Retriever),
		defaultTopK:     5,
		defaultMinScore: 0.0,
		strategies: map[QueryComplexity]*RetrievalStrategy{
			ComplexitySimple: {
				TopK:        3,
				MinScore:    0.7,
				UseReranker: false,
				MultiQuery:  false,
			},
			ComplexityModerate: {
				TopK:        5,
				MinScore:    0.5,
				UseReranker: true,
				MultiQuery:  false,
			},
			ComplexityComplex: {
				TopK:        10,
				MinScore:    0.3,
				UseReranker: true,
				MultiQuery:  true,
			},
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	// 默认分类器
	if r.classifier == nil {
		r.classifier = NewRuleClassifier()
	}

	return r
}

// Retrieve 自适应检索
func (r *AdaptiveRetriever) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	// 1. 分类查询
	classification, err := r.classifier.Classify(ctx, query)
	if err != nil {
		// 分类失败，使用默认策略
		return r.retrieveWithStrategy(ctx, query, &RetrievalStrategy{
			TopK:     r.defaultTopK,
			MinScore: r.defaultMinScore,
		}, opts...)
	}

	// 2. 获取对应策略
	strategy, ok := r.strategies[classification.Complexity]
	if !ok {
		strategy = &RetrievalStrategy{
			TopK:     r.defaultTopK,
			MinScore: r.defaultMinScore,
		}
	}

	// 3. 根据查询类型微调策略
	strategy = r.adjustForQueryType(strategy, classification.Type)

	// 4. 执行检索
	return r.retrieveWithStrategy(ctx, query, strategy, opts...)
}

// adjustForQueryType 根据查询类型微调策略
func (r *AdaptiveRetriever) adjustForQueryType(base *RetrievalStrategy, queryType QueryType) *RetrievalStrategy {
	// 创建副本
	adjusted := *base

	switch queryType {
	case QueryTypeComparative:
		// 比较型需要更多文档
		adjusted.TopK = int(math.Max(float64(adjusted.TopK), 8))
		adjusted.UseReranker = true
	case QueryTypeAggregation:
		// 聚合型需要更多文档，降低阈值
		adjusted.TopK = int(math.Max(float64(adjusted.TopK), 10))
		adjusted.MinScore = float32(math.Min(float64(adjusted.MinScore), 0.3))
	case QueryTypeAnalytical:
		// 分析型启用重排序
		adjusted.UseReranker = true
	}

	return &adjusted
}

// retrieveWithStrategy 使用指定策略执行检索
func (r *AdaptiveRetriever) retrieveWithStrategy(ctx context.Context, query string, strategy *RetrievalStrategy, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	// 选择检索器
	retriever := r.baseRetriever
	if strategy.RetrieverName != "" {
		if named, ok := r.retrievers[strategy.RetrieverName]; ok {
			retriever = named
		}
	}

	if retriever == nil {
		return nil, nil
	}

	// 构建检索选项
	retrieveOpts := make([]rag.RetrieveOption, 0, len(opts)+2)
	retrieveOpts = append(retrieveOpts, rag.WithTopK(strategy.TopK))
	if strategy.MinScore > 0 {
		retrieveOpts = append(retrieveOpts, rag.WithMinScore(strategy.MinScore))
	}
	retrieveOpts = append(retrieveOpts, opts...)

	return retriever.Retrieve(ctx, query, retrieveOpts...)
}
