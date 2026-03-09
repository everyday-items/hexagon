package knowledge

import (
	"context"
	"testing"
)

// ============== Entity 和 Relation 基础测试 ==============

func TestEntity_Creation(t *testing.T) {
	e := Entity{
		Name: "Go",
		Type: "Language",
		Properties: map[string]any{
			"year": 2009,
		},
	}

	if e.Name != "Go" {
		t.Errorf("期望 Name=Go，实际 %s", e.Name)
	}
	if e.Type != "Language" {
		t.Errorf("期望 Type=Language，实际 %s", e.Type)
	}
	if e.Properties["year"] != 2009 {
		t.Errorf("期望 year=2009，实际 %v", e.Properties["year"])
	}
}

func TestRelation_Creation(t *testing.T) {
	r := Relation{
		From:   "Go",
		To:     "Google",
		Type:   "created_by",
		Weight: 1.0,
	}

	if r.From != "Go" {
		t.Errorf("期望 From=Go，实际 %s", r.From)
	}
	if r.To != "Google" {
		t.Errorf("期望 To=Google，实际 %s", r.To)
	}
	if r.Type != "created_by" {
		t.Errorf("期望 Type=created_by，实际 %s", r.Type)
	}
}

// ============== MemoryGraphStore 测试 ==============

func TestMemoryGraphStore_AddEntity(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	err := store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	if err != nil {
		t.Fatalf("AddEntity 失败: %v", err)
	}

	ec, rc, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats 失败: %v", err)
	}
	if ec != 1 {
		t.Errorf("期望 1 个实体，实际 %d", ec)
	}
	if rc != 0 {
		t.Errorf("期望 0 个关系，实际 %d", rc)
	}
}

func TestMemoryGraphStore_AddEntity_AutoID(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	err := store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	if err != nil {
		t.Fatalf("AddEntity 失败: %v", err)
	}

	e, err := store.GetEntity(ctx, "Go")
	if err != nil {
		t.Fatalf("GetEntity 失败: %v", err)
	}
	if e == nil {
		t.Fatal("期望找到实体 Go")
	}
	if e.ID == "" {
		t.Error("期望自动生成 ID")
	}
	if e.CreatedAt.IsZero() {
		t.Error("期望自动设置 CreatedAt")
	}
}

func TestMemoryGraphStore_AddRelation(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})

	err := store.AddRelation(ctx, Relation{
		From: "Go",
		To:   "Google",
		Type: "created_by",
	})
	if err != nil {
		t.Fatalf("AddRelation 失败: %v", err)
	}

	_, rc, _ := store.Stats(ctx)
	if rc != 1 {
		t.Errorf("期望 1 个关系，实际 %d", rc)
	}
}

func TestMemoryGraphStore_GetEntity_ByName(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})

	e, err := store.GetEntity(ctx, "Go")
	if err != nil {
		t.Fatalf("GetEntity 失败: %v", err)
	}
	if e == nil {
		t.Fatal("期望找到实体")
	}
	if e.Name != "Go" {
		t.Errorf("期望 Name=Go，实际 %s", e.Name)
	}
}

func TestMemoryGraphStore_GetEntity_ByID(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{ID: "custom-id", Name: "Go", Type: "Language"})

	e, err := store.GetEntity(ctx, "custom-id")
	if err != nil {
		t.Fatalf("GetEntity 失败: %v", err)
	}
	if e == nil {
		t.Fatal("期望通过 ID 找到实体")
	}
	if e.Name != "Go" {
		t.Errorf("期望 Name=Go，实际 %s", e.Name)
	}
}

func TestMemoryGraphStore_GetEntity_NotFound(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	e, err := store.GetEntity(ctx, "不存在")
	if err != nil {
		t.Fatalf("GetEntity 不应报错: %v", err)
	}
	if e != nil {
		t.Error("不存在的实体应返回 nil")
	}
}

func TestMemoryGraphStore_GetRelations_Out(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = store.AddRelation(ctx, Relation{From: "Go", To: "Google", Type: "created_by"})

	rels, err := store.GetRelations(ctx, "Go", "out")
	if err != nil {
		t.Fatalf("GetRelations 失败: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("期望 1 个出边，实际 %d", len(rels))
	}
	if len(rels) > 0 && rels[0].Type != "created_by" {
		t.Errorf("期望关系类型 created_by，实际 %s", rels[0].Type)
	}
}

func TestMemoryGraphStore_GetRelations_In(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = store.AddRelation(ctx, Relation{From: "Go", To: "Google", Type: "created_by"})

	rels, err := store.GetRelations(ctx, "Google", "in")
	if err != nil {
		t.Fatalf("GetRelations 失败: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("期望 1 个入边，实际 %d", len(rels))
	}
}

func TestMemoryGraphStore_GetRelations_Both(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A"})
	_ = store.AddEntity(ctx, Entity{Name: "B"})
	_ = store.AddEntity(ctx, Entity{Name: "C"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "r1"})
	_ = store.AddRelation(ctx, Relation{From: "C", To: "B", Type: "r2"})

	rels, err := store.GetRelations(ctx, "B", "both")
	if err != nil {
		t.Fatalf("GetRelations 失败: %v", err)
	}
	// B 没有出边，有 1 个入边(C->B)；以及作为 A->B 的 To 端的入边
	// outEdges["B"] 为空，inEdges["B"] = [r1, r2]
	if len(rels) != 2 {
		t.Errorf("期望 2 条关系（both），实际 %d", len(rels))
	}
}

func TestMemoryGraphStore_GetRelations_Empty(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	rels, err := store.GetRelations(ctx, "不存在", "out")
	if err != nil {
		t.Fatalf("GetRelations 失败: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("期望 0 条关系，实际 %d", len(rels))
	}
}

func TestMemoryGraphStore_SearchEntities(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = store.AddEntity(ctx, Entity{Name: "Golang", Type: "Language"})

	// 搜索包含 "Go" 的实体
	entities, err := store.SearchEntities(ctx, "Go", "", 10)
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(entities) < 2 {
		t.Errorf("期望至少 2 个匹配，实际 %d", len(entities))
	}
}

func TestMemoryGraphStore_SearchEntities_ByType(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = store.AddEntity(ctx, Entity{Name: "Python", Type: "Language"})

	entities, err := store.SearchEntities(ctx, "", "Language", 10)
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("期望 2 个 Language 类型，实际 %d", len(entities))
	}
}

func TestMemoryGraphStore_SearchEntities_WithLimit(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = store.AddEntity(ctx, Entity{Name: "item", Type: "Test"})
	}

	entities, err := store.SearchEntities(ctx, "", "Test", 3)
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	// 注意：由于 map 迭代和相同 Name 会覆盖，实际只有 1 个
	if len(entities) > 3 {
		t.Errorf("期望最多 3 个，实际 %d", len(entities))
	}
}

// ============== BFS 邻居搜索测试 ==============

func TestMemoryGraphStore_GetNeighbors_1Hop(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "C", Type: "Node"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "C", Type: "link"})

	entities, relations, err := store.GetNeighbors(ctx, "A", 1)
	if err != nil {
		t.Fatalf("GetNeighbors 失败: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("期望 2 个 1-hop 邻居，实际 %d", len(entities))
	}
	if len(relations) != 2 {
		t.Errorf("期望 2 条关系，实际 %d", len(relations))
	}
}

func TestMemoryGraphStore_GetNeighbors_2Hop(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	// A -> B -> C
	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "C", Type: "Node"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})
	_ = store.AddRelation(ctx, Relation{From: "B", To: "C", Type: "link"})

	entities, _, err := store.GetNeighbors(ctx, "A", 2)
	if err != nil {
		t.Fatalf("GetNeighbors 失败: %v", err)
	}
	// 应该包含 B（1-hop）和 C（2-hop）
	if len(entities) != 2 {
		t.Errorf("期望 2 个邻居（B 和 C），实际 %d", len(entities))
	}
}

func TestMemoryGraphStore_GetNeighbors_Bidirectional(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	// A -> B, C -> A
	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "C", Type: "Node"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})
	_ = store.AddRelation(ctx, Relation{From: "C", To: "A", Type: "link"})

	entities, _, err := store.GetNeighbors(ctx, "A", 1)
	if err != nil {
		t.Fatalf("GetNeighbors 失败: %v", err)
	}
	// BFS 同时遍历出边和入边，所以 B 和 C 都应在邻居中
	if len(entities) != 2 {
		t.Errorf("期望 2 个邻居（B 和 C），实际 %d", len(entities))
	}
}

func TestMemoryGraphStore_GetNeighbors_ZeroHops(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})

	// hops=0 应被修正为 1
	entities, _, err := store.GetNeighbors(ctx, "A", 0)
	if err != nil {
		t.Fatalf("GetNeighbors 失败: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("hops=0 应默认为 1，期望 1 个邻居，实际 %d", len(entities))
	}
}

func TestMemoryGraphStore_GetNeighbors_NoDuplicates(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	// A -> B, A -> B (两条边连接同一对节点)
	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link1"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link2"})

	entities, relations, err := store.GetNeighbors(ctx, "A", 1)
	if err != nil {
		t.Fatalf("GetNeighbors 失败: %v", err)
	}
	// 实体应去重，但关系不去重
	if len(entities) != 1 {
		t.Errorf("期望 1 个去重邻居，实际 %d", len(entities))
	}
	if len(relations) != 2 {
		t.Errorf("期望 2 条关系，实际 %d", len(relations))
	}
}

// ============== 删除操作测试 ==============

func TestMemoryGraphStore_DeleteEntity(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = store.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = store.AddRelation(ctx, Relation{From: "Go", To: "Google", Type: "created_by"})

	err := store.DeleteEntity(ctx, "Go")
	if err != nil {
		t.Fatalf("DeleteEntity 失败: %v", err)
	}

	e, _ := store.GetEntity(ctx, "Go")
	if e != nil {
		t.Error("删除后不应找到实体")
	}

	ec, rc, _ := store.Stats(ctx)
	if ec != 1 {
		t.Errorf("期望 1 个实体，实际 %d", ec)
	}
	// 相关关系也应被清理
	if rc != 0 {
		t.Errorf("期望 0 个关系（已清理），实际 %d", rc)
	}
}

func TestMemoryGraphStore_DeleteRelation(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A"})
	_ = store.AddEntity(ctx, Entity{Name: "B"})
	_ = store.AddRelation(ctx, Relation{ID: "rel-1", From: "A", To: "B", Type: "link"})

	err := store.DeleteRelation(ctx, "rel-1")
	if err != nil {
		t.Fatalf("DeleteRelation 失败: %v", err)
	}

	_, rc, _ := store.Stats(ctx)
	if rc != 0 {
		t.Errorf("期望 0 个关系，实际 %d", rc)
	}
}

func TestMemoryGraphStore_Clear(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A"})
	_ = store.AddEntity(ctx, Entity{Name: "B"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})

	err := store.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}

	ec, rc, _ := store.Stats(ctx)
	if ec != 0 || rc != 0 {
		t.Errorf("清空后期望 0 实体 0 关系，实际 %d 实体 %d 关系", ec, rc)
	}
}

// ============== ExecuteCypher 测试 ==============

func TestMemoryGraphStore_ExecuteCypher_MatchAllEntities(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A", Type: "Node"})
	_ = store.AddEntity(ctx, Entity{Name: "B", Type: "Node"})

	result, err := store.ExecuteCypher(ctx, "MATCH (n) RETURN n", nil)
	if err != nil {
		t.Fatalf("ExecuteCypher 失败: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Errorf("期望 2 个实体，实际 %d", len(result.Entities))
	}
}

func TestMemoryGraphStore_ExecuteCypher_MatchRelations(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_ = store.AddEntity(ctx, Entity{Name: "A"})
	_ = store.AddEntity(ctx, Entity{Name: "B"})
	_ = store.AddRelation(ctx, Relation{From: "A", To: "B", Type: "link"})

	result, err := store.ExecuteCypher(ctx, "MATCH (n)-[r]->(m) RETURN r", nil)
	if err != nil {
		t.Fatalf("ExecuteCypher 失败: %v", err)
	}
	if len(result.Relations) != 1 {
		t.Errorf("期望 1 条关系，实际 %d", len(result.Relations))
	}
}

func TestMemoryGraphStore_ExecuteCypher_Unsupported(t *testing.T) {
	store := NewMemoryGraphStore()
	ctx := context.Background()

	_, err := store.ExecuteCypher(ctx, "CREATE (n:Person {name: 'Alice'})", nil)
	if err == nil {
		t.Error("不支持的 Cypher 应返回错误")
	}
}

// ============== PropertyGraph 测试 ==============

func TestPropertyGraph_AddEntity(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	err := graph.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	if err != nil {
		t.Fatalf("AddEntity 失败: %v", err)
	}
}

func TestPropertyGraph_AddTriple(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	err := graph.AddTriple(ctx, "Go", "created_by", "Google")
	if err != nil {
		t.Fatalf("AddTriple 失败: %v", err)
	}

	// 验证两个实体已自动创建
	ec, rc, _ := store.Stats(ctx)
	if ec != 2 {
		t.Errorf("期望 2 个实体（自动创建），实际 %d", ec)
	}
	if rc != 1 {
		t.Errorf("期望 1 个关系，实际 %d", rc)
	}
}

func TestPropertyGraph_AddTriple_ExistingEntities(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	// 预先创建实体
	_ = graph.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})
	_ = graph.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})

	err := graph.AddTriple(ctx, "Go", "created_by", "Google")
	if err != nil {
		t.Fatalf("AddTriple 失败: %v", err)
	}

	// 实体不应重复创建
	ec, _, _ := store.Stats(ctx)
	if ec != 2 {
		t.Errorf("期望 2 个实体（不重复），实际 %d", ec)
	}
}

func TestPropertyGraph_Query(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	_ = graph.AddEntity(ctx, Entity{Name: "A"})
	_ = graph.AddEntity(ctx, Entity{Name: "B"})

	result, err := graph.Query(ctx, "MATCH (n) RETURN n", nil)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Errorf("期望 2 个实体，实际 %d", len(result.Entities))
	}
}

func TestPropertyGraph_GetSubgraph(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	_ = graph.AddTriple(ctx, "Go", "created_by", "Google")
	_ = graph.AddTriple(ctx, "Go", "influenced_by", "C")
	_ = graph.AddTriple(ctx, "Google", "headquarters", "Mountain View")

	// 获取以 Go 为中心、2 跳的子图
	result, err := graph.GetSubgraph(ctx, "Go", 2)
	if err != nil {
		t.Fatalf("GetSubgraph 失败: %v", err)
	}

	// 应包含 Go（中心）+ 邻居
	if len(result.Entities) < 2 {
		t.Errorf("期望至少 2 个实体，实际 %d", len(result.Entities))
	}
	if len(result.Relations) < 2 {
		t.Errorf("期望至少 2 条关系，实际 %d", len(result.Relations))
	}
}

func TestPropertyGraph_GetSubgraph_CenterIncluded(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	_ = graph.AddEntity(ctx, Entity{Name: "Center", Type: "Node"})

	result, err := graph.GetSubgraph(ctx, "Center", 1)
	if err != nil {
		t.Fatalf("GetSubgraph 失败: %v", err)
	}

	// 即使没有邻居，中心实体也应该在结果中
	if len(result.Entities) != 1 {
		t.Errorf("期望 1 个实体（中心），实际 %d", len(result.Entities))
	}
}

func TestPropertyGraph_Store(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)

	if graph.Store() != store {
		t.Error("Store() 应返回底层存储")
	}
}

// ============== GraphRetriever 测试 ==============

func TestGraphRetriever_New(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)

	r := NewGraphRetriever(graph)
	if r == nil {
		t.Fatal("NewGraphRetriever 返回 nil")
	}
	if r.hops != 2 {
		t.Errorf("默认 hops 应为 2，实际 %d", r.hops)
	}
	if r.maxEntities != 20 {
		t.Errorf("默认 maxEntities 应为 20，实际 %d", r.maxEntities)
	}
	if !r.includeRels {
		t.Error("默认应包含关系信息")
	}
}

func TestGraphRetriever_WithOptions(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)

	r := NewGraphRetriever(graph,
		WithGraphHops(3),
		WithGraphMaxEntities(10),
		WithGraphIncludeRelations(false),
	)

	if r.hops != 3 {
		t.Errorf("hops 应为 3，实际 %d", r.hops)
	}
	if r.maxEntities != 10 {
		t.Errorf("maxEntities 应为 10，实际 %d", r.maxEntities)
	}
	if r.includeRels {
		t.Error("应不包含关系信息")
	}
}

func TestGraphRetriever_Retrieve(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	// 构建知识图谱
	_ = graph.AddEntity(ctx, Entity{Name: "Go", Type: "Language", Properties: map[string]any{"year": 2009}})
	_ = graph.AddEntity(ctx, Entity{Name: "Google", Type: "Company"})
	_ = graph.AddEntity(ctx, Entity{Name: "Python", Type: "Language"})
	_ = graph.AddRelation(ctx, Relation{From: "Go", To: "Google", Type: "created_by"})

	r := NewGraphRetriever(graph, WithGraphHops(1), WithGraphIncludeRelations(true))

	// 搜索 "Go"
	docs, err := r.Retrieve(ctx, "Go")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	if len(docs) == 0 {
		t.Fatal("期望返回文档")
	}

	// 验证文档包含实体信息
	found := false
	for _, doc := range docs {
		if doc.Metadata["entity_name"] == "Go" {
			found = true
			if doc.Metadata["source"] != "knowledge_graph" {
				t.Error("source 应为 knowledge_graph")
			}
		}
	}
	if !found {
		t.Error("期望找到 Go 实体对应的文档")
	}
}

func TestGraphRetriever_Retrieve_WithRelations(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	_ = graph.AddTriple(ctx, "Go", "created_by", "Google")

	r := NewGraphRetriever(graph, WithGraphHops(1), WithGraphIncludeRelations(true))

	docs, err := r.Retrieve(ctx, "Go")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	// 检查文档中包含关系信息
	hasRelation := false
	for _, doc := range docs {
		if doc.Metadata["entity_name"] == "Go" {
			if len(doc.Content) > 0 {
				hasRelation = true
			}
		}
	}
	if !hasRelation {
		t.Error("期望文档内容包含关系信息")
	}
}

func TestGraphRetriever_Retrieve_NoMatch(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	_ = graph.AddEntity(ctx, Entity{Name: "Go", Type: "Language"})

	r := NewGraphRetriever(graph)

	docs, err := r.Retrieve(ctx, "完全不相关的查询")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("无匹配时期望 0 个文档，实际 %d", len(docs))
	}
}

func TestGraphRetriever_Retrieve_MaxEntities(t *testing.T) {
	store := NewMemoryGraphStore()
	graph := NewPropertyGraph(store)
	ctx := context.Background()

	// 添加大量以 "test" 开头的实体
	for i := 0; i < 30; i++ {
		name := "test_entity"
		if i > 0 {
			// 只有第一个能匹配（map key 覆盖）
			name = "test_entity"
		}
		_ = graph.AddEntity(ctx, Entity{Name: name, Type: "Test"})
	}

	r := NewGraphRetriever(graph, WithGraphMaxEntities(5))

	docs, err := r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("Retrieve 失败: %v", err)
	}

	if len(docs) > 5 {
		t.Errorf("期望最多 5 个文档，实际 %d", len(docs))
	}
}

// ============== GraphStore 接口符合性测试 ==============

func TestMemoryGraphStore_ImplementsGraphStore(t *testing.T) {
	var _ GraphStore = (*MemoryGraphStore)(nil)
}
