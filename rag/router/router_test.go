package router

import (
	"context"
	"testing"

	"github.com/hexagon-codes/hexagon/rag"
)

// mockRetriever 模拟检索器
type mockRetriever struct {
	name string
	docs []rag.Document
}

func (r *mockRetriever) Retrieve(_ context.Context, _ string, _ ...rag.RetrieveOption) ([]rag.Document, error) {
	return r.docs, nil
}

func newMockRetriever(name string, docs ...rag.Document) *mockRetriever {
	return &mockRetriever{name: name, docs: docs}
}

// ============== Route 配置创建测试 ==============

func TestWithRoute(t *testing.T) {
	retriever := newMockRetriever("vector")
	router := NewQueryRouter(
		WithRoute("vector", retriever, "语义检索"),
	)

	if len(router.routes) != 1 {
		t.Fatalf("期望 1 条路由，实际 %d", len(router.routes))
	}
	if router.routes[0].Name != "vector" {
		t.Errorf("期望路由名称 'vector'，实际 '%s'", router.routes[0].Name)
	}
	if router.routes[0].Description != "语义检索" {
		t.Errorf("期望描述 '语义检索'，实际 '%s'", router.routes[0].Description)
	}
}

func TestWithRouteKeywords(t *testing.T) {
	retriever := newMockRetriever("keyword")
	router := NewQueryRouter(
		WithRouteKeywords("keyword", retriever, "关键词搜索", []string{"搜索", "查找"}),
	)

	if len(router.routes) != 1 {
		t.Fatalf("期望 1 条路由，实际 %d", len(router.routes))
	}
	if len(router.routes[0].Keywords) != 2 {
		t.Errorf("期望 2 个关键词，实际 %d", len(router.routes[0].Keywords))
	}
}

func TestWithRoutePatterns(t *testing.T) {
	retriever := newMockRetriever("sql")
	router := NewQueryRouter(
		WithRoutePatterns("sql", retriever, "数据查询", []string{`\d{4}-\d{2}-\d{2}`, `SELECT|INSERT`}),
	)

	if len(router.routes) != 1 {
		t.Fatalf("期望 1 条路由，实际 %d", len(router.routes))
	}
	if len(router.routes[0].Patterns) != 2 {
		t.Errorf("期望 2 个模式，实际 %d", len(router.routes[0].Patterns))
	}
}

func TestWithRoutePatterns_InvalidRegex(t *testing.T) {
	retriever := newMockRetriever("test")
	router := NewQueryRouter(
		WithRoutePatterns("test", retriever, "测试", []string{`[invalid`, `valid.*`}),
	)

	// 无效正则应被跳过
	if len(router.routes[0].Patterns) != 1 {
		t.Errorf("期望 1 个有效模式（跳过无效），实际 %d", len(router.routes[0].Patterns))
	}
}

// ============== 路由器创建测试 ==============

func TestNewQueryRouter_Defaults(t *testing.T) {
	router := NewQueryRouter()

	if router.mode != ModeRule {
		t.Errorf("默认模式应为 ModeRule，实际 %d", router.mode)
	}
	if len(router.routes) != 0 {
		t.Errorf("默认应有 0 条路由，实际 %d", len(router.routes))
	}
}

func TestWithRouterMode(t *testing.T) {
	router := NewQueryRouter(WithRouterMode(ModeLLM))
	if router.mode != ModeLLM {
		t.Errorf("期望 ModeLLM，实际 %d", router.mode)
	}
}

func TestWithFallback(t *testing.T) {
	fallback := newMockRetriever("fallback")
	router := NewQueryRouter(WithFallback(fallback))

	if router.fallback == nil {
		t.Error("期望设置 fallback 检索器")
	}
}

// ============== 关键词匹配测试 ==============

func TestRuleRoute_KeywordMatch(t *testing.T) {
	vectorRetriever := newMockRetriever("vector",
		rag.Document{ID: "v1", Content: "向量结果"},
	)
	keywordRetriever := newMockRetriever("keyword",
		rag.Document{ID: "k1", Content: "关键词结果"},
	)

	router := NewQueryRouter(
		WithRouteKeywords("vector", vectorRetriever, "语义", []string{"类似", "相关"}),
		WithRouteKeywords("keyword", keywordRetriever, "精确", []string{"搜索", "查找", "精确"}),
	)

	ctx := context.Background()

	// "搜索" 关键词应匹配 keyword 路由
	docs, err := router.Retrieve(ctx, "搜索 Go 语言文档")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "k1" {
		t.Errorf("期望匹配 keyword 路由，实际 docs=%v", docs)
	}
}

func TestRuleRoute_KeywordMatch_CaseInsensitive(t *testing.T) {
	retriever := newMockRetriever("test",
		rag.Document{ID: "t1", Content: "结果"},
	)

	router := NewQueryRouter(
		WithRouteKeywords("test", retriever, "测试", []string{"SELECT"}),
	)

	ctx := context.Background()

	// 大小写不敏感匹配
	docs, err := router.Retrieve(ctx, "select * from table")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("期望匹配（大小写不敏感），实际 %d 个文档", len(docs))
	}
}

func TestRuleRoute_MultipleKeywordsScore(t *testing.T) {
	ret1 := newMockRetriever("r1", rag.Document{ID: "1"})
	ret2 := newMockRetriever("r2", rag.Document{ID: "2"})

	router := NewQueryRouter(
		WithRouteKeywords("r1", ret1, "路由1", []string{"数据"}),
		WithRouteKeywords("r2", ret2, "路由2", []string{"数据", "分析"}),
	)

	ctx := context.Background()

	// "数据分析" 匹配 r2 的两个关键词，分数更高
	docs, err := router.Retrieve(ctx, "数据分析报告")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "2" {
		t.Errorf("期望匹配 r2（分数更高），实际 docs=%v", docs)
	}
}

// ============== 正则匹配测试 ==============

func TestRuleRoute_PatternMatch(t *testing.T) {
	sqlRetriever := newMockRetriever("sql",
		rag.Document{ID: "sql1", Content: "SQL 结果"},
	)

	router := NewQueryRouter(
		WithRoutePatterns("sql", sqlRetriever, "SQL 查询", []string{`\d{4}-\d{2}-\d{2}`, `(?i)select\s+`}),
	)

	ctx := context.Background()

	// 包含日期格式，应匹配 sql 路由
	docs, err := router.Retrieve(ctx, "查询 2024-01-15 的数据")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "sql1" {
		t.Errorf("期望匹配 sql 路由，实际 docs=%v", docs)
	}
}

func TestRuleRoute_PatternMatch_HigherScore(t *testing.T) {
	kwRetriever := newMockRetriever("kw", rag.Document{ID: "kw1"})
	patRetriever := newMockRetriever("pat", rag.Document{ID: "pat1"})

	router := NewQueryRouter(
		WithRouteKeywords("kw", kwRetriever, "关键词", []string{"数据"}),
		WithRoutePatterns("pat", patRetriever, "正则", []string{`数据`}),
	)

	ctx := context.Background()

	// 正则匹配分数(20) > 关键词匹配分数(10)
	docs, err := router.Retrieve(ctx, "查看数据")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "pat1" {
		t.Errorf("期望正则路由优先（分数更高），实际 docs=%v", docs)
	}
}

// ============== SelectRoute 测试 ==============

func TestSelectRoute(t *testing.T) {
	ret := newMockRetriever("test", rag.Document{ID: "1"})
	router := NewQueryRouter(
		WithRouteKeywords("test", ret, "测试", []string{"测试"}),
	)

	ctx := context.Background()
	name, err := router.SelectRoute(ctx, "运行测试")
	if err != nil {
		t.Fatalf("SelectRoute 失败: %v", err)
	}
	if name != "test" {
		t.Errorf("期望路由名称 'test'，实际 '%s'", name)
	}
}

func TestSelectRoute_NoMatch(t *testing.T) {
	ret := newMockRetriever("test")
	router := NewQueryRouter(
		WithRouteKeywords("test", ret, "测试", []string{"特定关键词"}),
	)

	ctx := context.Background()
	name, err := router.SelectRoute(ctx, "完全不相关的查询")
	if err != nil {
		t.Fatalf("SelectRoute 失败: %v", err)
	}
	if name != "" {
		t.Errorf("无匹配时期望空字符串，实际 '%s'", name)
	}
}

// ============== Fallback 测试 ==============

func TestRetrieve_NoMatch_WithFallback(t *testing.T) {
	fallbackRetriever := newMockRetriever("fallback",
		rag.Document{ID: "fb1", Content: "兜底结果"},
	)

	router := NewQueryRouter(
		WithRouteKeywords("specific", newMockRetriever("spec"), "特定", []string{"特定"}),
		WithFallback(fallbackRetriever),
	)

	ctx := context.Background()
	docs, err := router.Retrieve(ctx, "随机查询")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "fb1" {
		t.Errorf("无匹配时应使用 fallback，实际 docs=%v", docs)
	}
}

func TestRetrieve_NoMatch_NoFallback(t *testing.T) {
	router := NewQueryRouter(
		WithRouteKeywords("specific", newMockRetriever("spec"), "特定", []string{"特定"}),
	)

	ctx := context.Background()
	_, err := router.Retrieve(ctx, "随机查询")
	if err == nil {
		t.Error("无匹配且无 fallback 时应返回错误")
	}
}

// ============== LLM 路由模式测试 ==============

func TestLLMRoute_NoProvider(t *testing.T) {
	router := NewQueryRouter(WithRouterMode(ModeLLM))

	ctx := context.Background()
	_, err := router.Retrieve(ctx, "测试")
	if err == nil {
		t.Error("无 LLM provider 时应返回错误")
	}
}

// ============== Hybrid 模式测试 ==============

func TestHybridRoute_RuleFirst(t *testing.T) {
	ret := newMockRetriever("kw", rag.Document{ID: "kw1"})

	router := NewQueryRouter(
		WithRouterMode(ModeHybrid),
		WithRouteKeywords("kw", ret, "关键词", []string{"搜索"}),
	)

	ctx := context.Background()
	docs, err := router.Retrieve(ctx, "搜索文档")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}
	// Hybrid 模式下，规则先匹配成功
	if len(docs) != 1 || docs[0].ID != "kw1" {
		t.Errorf("Hybrid 模式规则优先，期望 kw1，实际 docs=%v", docs)
	}
}

func TestHybridRoute_FallbackToLLM_NoProvider(t *testing.T) {
	router := NewQueryRouter(
		WithRouterMode(ModeHybrid),
		WithRouteKeywords("kw", newMockRetriever("kw"), "关键词", []string{"特定"}),
		// 无 LLM provider，无 fallback
	)

	ctx := context.Background()
	_, err := router.Retrieve(ctx, "随机查询")
	// 规则未匹配，LLM 无 provider，应返回错误
	if err == nil {
		t.Error("Hybrid 模式规则未匹配且无 LLM 时应返回错误")
	}
}

// ============== MultiRouter 测试 ==============

func TestMultiRouter_Retrieve(t *testing.T) {
	r1 := NewQueryRouter(
		WithRouteKeywords("kw", newMockRetriever("kw",
			rag.Document{ID: "d1", Content: "文档1", Score: 0.9},
		), "关键词", []string{"测试"}),
	)

	r2 := NewQueryRouter(
		WithRouteKeywords("vec", newMockRetriever("vec",
			rag.Document{ID: "d2", Content: "文档2", Score: 0.8},
			rag.Document{ID: "d1", Content: "重复", Score: 0.7}, // 重复 ID
		), "向量", []string{"测试"}),
	)

	multi := NewMultiRouter(10, r1, r2)
	ctx := context.Background()

	docs, err := multi.Retrieve(ctx, "测试查询")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	// 应去重，d1 只出现一次
	idSet := make(map[string]bool)
	for _, doc := range docs {
		if idSet[doc.ID] {
			t.Errorf("文档 %s 重复", doc.ID)
		}
		idSet[doc.ID] = true
	}

	if len(docs) != 2 {
		t.Errorf("去重后期望 2 个文档，实际 %d", len(docs))
	}
}

func TestMultiRouter_MergeTop(t *testing.T) {
	r1 := NewQueryRouter(
		WithRouteKeywords("kw", newMockRetriever("kw",
			rag.Document{ID: "d1", Score: 0.9},
			rag.Document{ID: "d2", Score: 0.8},
			rag.Document{ID: "d3", Score: 0.7},
		), "关键词", []string{"测试"}),
	)

	multi := NewMultiRouter(2, r1)
	ctx := context.Background()

	docs, err := multi.Retrieve(ctx, "测试")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("mergeTop=2 时期望 2 个文档，实际 %d", len(docs))
	}
}

// ============== sortDocsByScore 测试 ==============

func TestSortDocsByScore(t *testing.T) {
	docs := []rag.Document{
		{ID: "low", Score: 0.3},
		{ID: "high", Score: 0.9},
		{ID: "mid", Score: 0.6},
	}

	sortDocsByScore(docs)

	if docs[0].ID != "high" {
		t.Errorf("期望第一个为 high，实际 %s", docs[0].ID)
	}
	if docs[1].ID != "mid" {
		t.Errorf("期望第二个为 mid，实际 %s", docs[1].ID)
	}
	if docs[2].ID != "low" {
		t.Errorf("期望第三个为 low，实际 %s", docs[2].ID)
	}
}

func TestSortDocsByScore_Empty(t *testing.T) {
	var docs []rag.Document
	sortDocsByScore(docs) // 不应 panic
}

func TestSortDocsByScore_Single(t *testing.T) {
	docs := []rag.Document{{ID: "only", Score: 0.5}}
	sortDocsByScore(docs)
	if docs[0].ID != "only" {
		t.Error("单元素排序不应改变")
	}
}

// ============== RouterMode 常量测试 ==============

func TestRouterMode_Constants(t *testing.T) {
	if ModeRule != 0 {
		t.Errorf("ModeRule 应为 0，实际 %d", ModeRule)
	}
	if ModeLLM != 1 {
		t.Errorf("ModeLLM 应为 1，实际 %d", ModeLLM)
	}
	if ModeHybrid != 2 {
		t.Errorf("ModeHybrid 应为 2，实际 %d", ModeHybrid)
	}
}
