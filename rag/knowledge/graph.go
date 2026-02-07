// Package knowledge 提供知识图谱能力
//
// 实现 Property Graph 和 Text2Cypher 功能：
//   - PropertyGraph: 属性图存储和查询
//   - Entity/Relation: 实体和关系定义
//   - Text2Cypher: 自然语言转 Cypher 查询
//   - GraphRAG: 基于图的 RAG 检索
//
// 对标 LlamaIndex 的 Property Graph Index + Text2Cypher。
//
// 使用示例：
//
//	store := NewMemoryGraphStore()
//	graph := NewPropertyGraph(store)
//
//	// 添加实体和关系
//	graph.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
//	graph.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
//	graph.AddRelation(ctx, Relation{From: "Go", To: "Google", Type: "created_by"})
//
//	// 自然语言查询
//	t2c := NewText2Cypher(llmProvider, graph)
//	results, err := t2c.Query(ctx, "哪些语言是 Google 创建的？")
package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== 基础类型 ==============

// Entity 实体（图节点）
type Entity struct {
	// ID 实体唯一标识
	ID string `json:"id"`

	// Name 实体名称
	Name string `json:"name"`

	// Type 实体类型（如 Person, Company, Language）
	Type string `json:"type"`

	// Properties 属性
	Properties map[string]any `json:"properties,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Relation 关系（图边）
type Relation struct {
	// ID 关系唯一标识
	ID string `json:"id"`

	// From 源实体名称或 ID
	From string `json:"from"`

	// To 目标实体名称或 ID
	To string `json:"to"`

	// Type 关系类型（如 created_by, works_at, has_skill）
	Type string `json:"type"`

	// Properties 关系属性
	Properties map[string]any `json:"properties,omitempty"`

	// Weight 关系权重
	Weight float64 `json:"weight,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// GraphQueryResult 图查询结果
type GraphQueryResult struct {
	// Entities 匹配的实体
	Entities []Entity `json:"entities,omitempty"`

	// Relations 匹配的关系
	Relations []Relation `json:"relations,omitempty"`

	// Paths 匹配的路径
	Paths [][]string `json:"paths,omitempty"`

	// RawResult 原始查询结果
	RawResult any `json:"raw_result,omitempty"`
}

// ============== GraphStore 接口 ==============

// GraphStore 图存储接口
// 支持不同后端（内存、Neo4j、ArangoDB 等）
type GraphStore interface {
	// AddEntity 添加实体
	AddEntity(ctx context.Context, entity Entity) error

	// AddRelation 添加关系
	AddRelation(ctx context.Context, relation Relation) error

	// GetEntity 获取实体
	GetEntity(ctx context.Context, nameOrID string) (*Entity, error)

	// GetRelations 获取实体的关系
	GetRelations(ctx context.Context, entityNameOrID string, direction string) ([]Relation, error)

	// SearchEntities 搜索实体（按名称或类型）
	SearchEntities(ctx context.Context, query string, entityType string, limit int) ([]Entity, error)

	// ExecuteCypher 执行 Cypher 查询
	ExecuteCypher(ctx context.Context, cypher string, params map[string]any) (*GraphQueryResult, error)

	// GetNeighbors 获取 N 跳邻居
	GetNeighbors(ctx context.Context, entityNameOrID string, hops int) ([]Entity, []Relation, error)

	// DeleteEntity 删除实体
	DeleteEntity(ctx context.Context, nameOrID string) error

	// DeleteRelation 删除关系
	DeleteRelation(ctx context.Context, id string) error

	// Clear 清空所有数据
	Clear(ctx context.Context) error

	// Stats 统计信息
	Stats(ctx context.Context) (entityCount, relationCount int, err error)
}

// ============== 内存图存储 ==============

// MemoryGraphStore 内存图存储实现
// 适用于测试和小规模数据
type MemoryGraphStore struct {
	mu        sync.RWMutex
	entities  map[string]*Entity  // name -> entity
	relations map[string]*Relation // id -> relation
	// 邻接表
	outEdges map[string][]string // entityName -> []relationID
	inEdges  map[string][]string // entityName -> []relationID
}

// NewMemoryGraphStore 创建内存图存储
func NewMemoryGraphStore() *MemoryGraphStore {
	return &MemoryGraphStore{
		entities:  make(map[string]*Entity),
		relations: make(map[string]*Relation),
		outEdges:  make(map[string][]string),
		inEdges:   make(map[string][]string),
	}
}

func (s *MemoryGraphStore) AddEntity(_ context.Context, entity Entity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entity.ID == "" {
		entity.ID = util.GenerateID("entity")
	}
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = time.Now()
	}
	s.entities[entity.Name] = &entity
	return nil
}

func (s *MemoryGraphStore) AddRelation(_ context.Context, relation Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if relation.ID == "" {
		relation.ID = util.GenerateID("rel")
	}
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}

	s.relations[relation.ID] = &relation
	s.outEdges[relation.From] = append(s.outEdges[relation.From], relation.ID)
	s.inEdges[relation.To] = append(s.inEdges[relation.To], relation.ID)
	return nil
}

func (s *MemoryGraphStore) GetEntity(_ context.Context, nameOrID string) (*Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if e, ok := s.entities[nameOrID]; ok {
		copy := *e
		return &copy, nil
	}
	// 按 ID 查找
	for _, e := range s.entities {
		if e.ID == nameOrID {
			copy := *e
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *MemoryGraphStore) GetRelations(_ context.Context, entityNameOrID string, direction string) ([]Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Relation

	if direction == "" || direction == "out" || direction == "both" {
		for _, relID := range s.outEdges[entityNameOrID] {
			if rel, ok := s.relations[relID]; ok {
				result = append(result, *rel)
			}
		}
	}
	if direction == "in" || direction == "both" {
		for _, relID := range s.inEdges[entityNameOrID] {
			if rel, ok := s.relations[relID]; ok {
				result = append(result, *rel)
			}
		}
	}

	return result, nil
}

func (s *MemoryGraphStore) SearchEntities(_ context.Context, query string, entityType string, limit int) ([]Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Entity
	queryLower := strings.ToLower(query)

	for _, e := range s.entities {
		if entityType != "" && e.Type != entityType {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(e.Name), queryLower) {
			continue
		}
		result = append(result, *e)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

// ExecuteCypher 简化 Cypher 执行（内存存储仅支持基础查询模式）
func (s *MemoryGraphStore) ExecuteCypher(_ context.Context, cypher string, _ map[string]any) (*GraphQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 简单的模式匹配支持
	cypherLower := strings.ToLower(cypher)

	result := &GraphQueryResult{}

	// MATCH (n) RETURN n — 返回所有实体
	if strings.Contains(cypherLower, "match (n)") && strings.Contains(cypherLower, "return n") {
		for _, e := range s.entities {
			result.Entities = append(result.Entities, *e)
		}
		return result, nil
	}

	// MATCH (n)-[r]->(m) — 返回所有关系
	if strings.Contains(cypherLower, "-[r]->") {
		for _, r := range s.relations {
			result.Relations = append(result.Relations, *r)
		}
		return result, nil
	}

	return result, fmt.Errorf("内存图存储仅支持基础 Cypher 模式，完整支持请使用 Neo4j 后端")
}

func (s *MemoryGraphStore) GetNeighbors(_ context.Context, entityNameOrID string, hops int) ([]Entity, []Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if hops <= 0 {
		hops = 1
	}

	visited := make(map[string]bool)
	var entities []Entity
	var relations []Relation

	// BFS 搜索 N 跳邻居
	queue := []string{entityNameOrID}
	visited[entityNameOrID] = true

	for hop := 0; hop < hops && len(queue) > 0; hop++ {
		var nextQueue []string
		for _, name := range queue {
			// 出边
			for _, relID := range s.outEdges[name] {
				rel := s.relations[relID]
				if rel == nil {
					continue
				}
				relations = append(relations, *rel)
				if !visited[rel.To] {
					visited[rel.To] = true
					nextQueue = append(nextQueue, rel.To)
					if e, ok := s.entities[rel.To]; ok {
						entities = append(entities, *e)
					}
				}
			}
			// 入边
			for _, relID := range s.inEdges[name] {
				rel := s.relations[relID]
				if rel == nil {
					continue
				}
				relations = append(relations, *rel)
				if !visited[rel.From] {
					visited[rel.From] = true
					nextQueue = append(nextQueue, rel.From)
					if e, ok := s.entities[rel.From]; ok {
						entities = append(entities, *e)
					}
				}
			}
		}
		queue = nextQueue
	}

	return entities, relations, nil
}

func (s *MemoryGraphStore) DeleteEntity(_ context.Context, nameOrID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entities, nameOrID)
	// 清理相关关系
	for _, relID := range s.outEdges[nameOrID] {
		delete(s.relations, relID)
	}
	for _, relID := range s.inEdges[nameOrID] {
		delete(s.relations, relID)
	}
	delete(s.outEdges, nameOrID)
	delete(s.inEdges, nameOrID)
	return nil
}

func (s *MemoryGraphStore) DeleteRelation(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.relations, id)
	return nil
}

func (s *MemoryGraphStore) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entities = make(map[string]*Entity)
	s.relations = make(map[string]*Relation)
	s.outEdges = make(map[string][]string)
	s.inEdges = make(map[string][]string)
	return nil
}

func (s *MemoryGraphStore) Stats(_ context.Context) (int, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities), len(s.relations), nil
}

var _ GraphStore = (*MemoryGraphStore)(nil)

// ============== PropertyGraph ==============

// PropertyGraph 属性图
// 封装 GraphStore 提供高级操作
type PropertyGraph struct {
	store GraphStore
}

// NewPropertyGraph 创建属性图
func NewPropertyGraph(store GraphStore) *PropertyGraph {
	return &PropertyGraph{store: store}
}

// AddEntity 添加实体
func (pg *PropertyGraph) AddEntity(ctx context.Context, entity Entity) error {
	return pg.store.AddEntity(ctx, entity)
}

// AddRelation 添加关系
func (pg *PropertyGraph) AddRelation(ctx context.Context, relation Relation) error {
	return pg.store.AddRelation(ctx, relation)
}

// AddTriple 添加三元组 (subject, predicate, object)
func (pg *PropertyGraph) AddTriple(ctx context.Context, subject, predicate, object string) error {
	// 确保实体存在
	if e, _ := pg.store.GetEntity(ctx, subject); e == nil {
		if err := pg.store.AddEntity(ctx, Entity{Name: subject, Type: "entity"}); err != nil {
			return err
		}
	}
	if e, _ := pg.store.GetEntity(ctx, object); e == nil {
		if err := pg.store.AddEntity(ctx, Entity{Name: object, Type: "entity"}); err != nil {
			return err
		}
	}

	return pg.store.AddRelation(ctx, Relation{
		From: subject,
		To:   object,
		Type: predicate,
	})
}

// Query 执行 Cypher 查询
func (pg *PropertyGraph) Query(ctx context.Context, cypher string, params map[string]any) (*GraphQueryResult, error) {
	return pg.store.ExecuteCypher(ctx, cypher, params)
}

// GetSubgraph 获取以指定实体为中心的子图
func (pg *PropertyGraph) GetSubgraph(ctx context.Context, entityName string, hops int) (*GraphQueryResult, error) {
	entities, relations, err := pg.store.GetNeighbors(ctx, entityName, hops)
	if err != nil {
		return nil, err
	}

	// 加入中心实体
	center, _ := pg.store.GetEntity(ctx, entityName)
	if center != nil {
		entities = append([]Entity{*center}, entities...)
	}

	return &GraphQueryResult{
		Entities:  entities,
		Relations: relations,
	}, nil
}

// Store 返回底层存储
func (pg *PropertyGraph) Store() GraphStore {
	return pg.store
}

// ============== Text2Cypher ==============

// Text2Cypher 自然语言转 Cypher 查询
// 使用 LLM 将用户的自然语言问题转换为 Cypher 查询
type Text2Cypher struct {
	provider llm.Provider
	model    string
	graph    *PropertyGraph
}

// Text2CypherOption 选项
type Text2CypherOption func(*Text2Cypher)

// WithText2CypherModel 设置模型
func WithText2CypherModel(model string) Text2CypherOption {
	return func(t *Text2Cypher) {
		t.model = model
	}
}

// NewText2Cypher 创建 Text2Cypher
func NewText2Cypher(provider llm.Provider, graph *PropertyGraph, opts ...Text2CypherOption) *Text2Cypher {
	t := &Text2Cypher{
		provider: provider,
		graph:    graph,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Query 使用自然语言查询图
func (t *Text2Cypher) Query(ctx context.Context, question string) (*GraphQueryResult, error) {
	// 获取图的 schema 信息
	schema, err := t.getSchemaDescription(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取图 Schema 失败: %w", err)
	}

	// 使用 LLM 生成 Cypher
	prompt := fmt.Sprintf(`你是一个 Cypher 查询生成专家。根据以下图数据库的 Schema 信息和用户问题，生成对应的 Cypher 查询语句。

图 Schema:
%s

用户问题: %s

请只返回 Cypher 查询语句，不要包含任何解释。`, schema, question)

	req := llm.CompletionRequest{
		Model: t.model,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 500,
	}

	resp, err := t.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM 生成 Cypher 失败: %w", err)
	}

	// 清理生成的 Cypher
	cypher := strings.TrimSpace(resp.Content)
	cypher = strings.TrimPrefix(cypher, "```cypher")
	cypher = strings.TrimPrefix(cypher, "```")
	cypher = strings.TrimSuffix(cypher, "```")
	cypher = strings.TrimSpace(cypher)

	// 执行 Cypher
	return t.graph.Query(ctx, cypher, nil)
}

// getSchemaDescription 获取图的 Schema 描述
func (t *Text2Cypher) getSchemaDescription(ctx context.Context) (string, error) {
	entityCount, relCount, err := t.graph.Store().Stats(ctx)
	if err != nil {
		return "", err
	}

	// 获取所有实体类型
	allEntities, err := t.graph.Store().SearchEntities(ctx, "", "", 100)
	if err != nil {
		return "", err
	}

	typeSet := make(map[string]int)
	for _, e := range allEntities {
		typeSet[e.Type]++
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("实体数量: %d, 关系数量: %d\n", entityCount, relCount))
	sb.WriteString("实体类型:\n")
	for t, count := range typeSet {
		sb.WriteString(fmt.Sprintf("  - %s (%d 个)\n", t, count))
	}

	return sb.String(), nil
}

// ============== GraphRAG 检索器 ==============

// GraphRetriever 基于图的 RAG 检索器
// 结合知识图谱和传统向量检索
type GraphRetriever struct {
	graph       *PropertyGraph
	hops        int    // 子图深度
	maxEntities int    // 最大返回实体数
	includeRels bool   // 是否在结果中包含关系信息
}

// GraphRetrieverOption 选项
type GraphRetrieverOption func(*GraphRetriever)

// WithGraphHops 设置子图搜索深度
func WithGraphHops(hops int) GraphRetrieverOption {
	return func(r *GraphRetriever) {
		r.hops = hops
	}
}

// WithGraphMaxEntities 设置最大返回实体数
func WithGraphMaxEntities(max int) GraphRetrieverOption {
	return func(r *GraphRetriever) {
		r.maxEntities = max
	}
}

// WithGraphIncludeRelations 是否包含关系信息
func WithGraphIncludeRelations(include bool) GraphRetrieverOption {
	return func(r *GraphRetriever) {
		r.includeRels = include
	}
}

// NewGraphRetriever 创建图 RAG 检索器
func NewGraphRetriever(graph *PropertyGraph, opts ...GraphRetrieverOption) *GraphRetriever {
	r := &GraphRetriever{
		graph:       graph,
		hops:        2,
		maxEntities: 20,
		includeRels: true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Retrieve 检索相关文档
// 从用户查询中提取实体关键词，在图中搜索相关子图，转换为文档
func (r *GraphRetriever) Retrieve(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
	// 1. 从查询中提取关键词作为实体搜索条件
	words := strings.Fields(query)

	var allEntities []Entity
	var allRelations []Relation
	seen := make(map[string]bool)

	for _, word := range words {
		entities, err := r.graph.Store().SearchEntities(ctx, word, "", r.maxEntities)
		if err != nil {
			continue
		}

		for _, e := range entities {
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			allEntities = append(allEntities, e)

			// 获取子图
			subEntities, subRels, err := r.graph.Store().GetNeighbors(ctx, e.Name, r.hops)
			if err != nil {
				continue
			}
			for _, se := range subEntities {
				if !seen[se.Name] {
					seen[se.Name] = true
					allEntities = append(allEntities, se)
				}
			}
			allRelations = append(allRelations, subRels...)
		}
	}

	// 限制返回数量
	if len(allEntities) > r.maxEntities {
		allEntities = allEntities[:r.maxEntities]
	}

	// 2. 将图数据转换为文档
	var docs []rag.Document

	for _, e := range allEntities {
		var content strings.Builder
		content.WriteString(fmt.Sprintf("实体: %s (类型: %s)", e.Name, e.Type))
		if len(e.Properties) > 0 {
			content.WriteString("\n属性: ")
			for k, v := range e.Properties {
				content.WriteString(fmt.Sprintf("%s=%v ", k, v))
			}
		}

		// 添加关系信息
		if r.includeRels {
			for _, rel := range allRelations {
				if rel.From == e.Name {
					content.WriteString(fmt.Sprintf("\n  → %s → %s", rel.Type, rel.To))
				}
				if rel.To == e.Name {
					content.WriteString(fmt.Sprintf("\n  ← %s ← %s", rel.Type, rel.From))
				}
			}
		}

		docs = append(docs, rag.Document{
			ID:      e.ID,
			Content: content.String(),
			Metadata: map[string]any{
				"entity_name": e.Name,
				"entity_type": e.Type,
				"source":      "knowledge_graph",
			},
			Source: "knowledge_graph",
		})
	}

	return docs, nil
}
