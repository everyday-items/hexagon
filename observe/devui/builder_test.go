package devui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGraphStore_CRUD 测试 GraphStore 的完整增删改查
func TestGraphStore_CRUD(t *testing.T) {
	store := NewGraphStore()

	// 创建
	def := &GraphDefinition{
		Name:        "测试图",
		Description: "用于测试的图定义",
		Nodes: []GraphNodeDef{
			{ID: "start-1", Name: "开始", Type: "start", Position: Position{X: 100, Y: 100}},
			{ID: "agent-1", Name: "Agent", Type: "agent", Position: Position{X: 300, Y: 100}},
			{ID: "end-1", Name: "结束", Type: "end", Position: Position{X: 500, Y: 100}},
		},
		Edges: []GraphEdgeDef{
			{ID: "e1", Source: "start-1", Target: "agent-1"},
			{ID: "e2", Source: "agent-1", Target: "end-1"},
		},
		EntryPoint: "start-1",
	}

	created := store.Create(def)
	if created.ID == "" {
		t.Fatal("创建后 ID 不应为空")
	}
	if created.Version != 1 {
		t.Fatalf("创建后版本号应为 1，实际为 %d", created.Version)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("创建时间不应为零值")
	}

	// 获取
	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	if got.Name != "测试图" {
		t.Fatalf("名称不匹配: %s", got.Name)
	}
	if len(got.Nodes) != 3 {
		t.Fatalf("节点数不匹配: %d", len(got.Nodes))
	}

	// 获取不存在的图
	_, err = store.Get("not-exist")
	if err == nil {
		t.Fatal("获取不存在的图应返回错误")
	}

	// 列表
	list := store.List()
	if len(list) != 1 {
		t.Fatalf("列表长度应为 1，实际为 %d", len(list))
	}

	// 更新
	def.Name = "更新后的图"
	def.Nodes = append(def.Nodes, GraphNodeDef{
		ID: "tool-1", Name: "工具", Type: "tool", Position: Position{X: 300, Y: 200},
	})
	updated, err := store.Update(created.ID, def)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("更新后版本号应为 2，实际为 %d", updated.Version)
	}
	if updated.Name != "更新后的图" {
		t.Fatalf("更新后名称不匹配: %s", updated.Name)
	}
	if len(updated.Nodes) != 4 {
		t.Fatalf("更新后节点数应为 4，实际为 %d", len(updated.Nodes))
	}

	// 更新不存在的图
	_, err = store.Update("not-exist", def)
	if err == nil {
		t.Fatal("更新不存在的图应返回错误")
	}

	// 删除
	err = store.Delete(created.ID)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 删除后获取
	_, err = store.Get(created.ID)
	if err == nil {
		t.Fatal("删除后获取应返回错误")
	}

	// 删除不存在的图
	err = store.Delete("not-exist")
	if err == nil {
		t.Fatal("删除不存在的图应返回错误")
	}
}

// TestGraphStore_Validate 测试图验证功能
func TestGraphStore_Validate(t *testing.T) {
	store := NewGraphStore()

	tests := []struct {
		name      string
		def       *GraphDefinition
		wantValid bool
		wantErrs  int // 最少错误数
	}{
		{
			name: "有效图",
			def: &GraphDefinition{
				Name: "有效图",
				Nodes: []GraphNodeDef{
					{ID: "s", Name: "开始", Type: "start"},
					{ID: "a", Name: "Agent", Type: "agent"},
					{ID: "e", Name: "结束", Type: "end"},
				},
				Edges: []GraphEdgeDef{
					{ID: "e1", Source: "s", Target: "a"},
					{ID: "e2", Source: "a", Target: "e"},
				},
				EntryPoint: "s",
			},
			wantValid: true,
		},
		{
			name: "空图",
			def: &GraphDefinition{
				Name:  "空图",
				Nodes: []GraphNodeDef{},
			},
			wantValid: false,
			wantErrs:  1,
		},
		{
			name: "缺少 start 节点",
			def: &GraphDefinition{
				Name: "无 start",
				Nodes: []GraphNodeDef{
					{ID: "a", Name: "Agent", Type: "agent"},
					{ID: "e", Name: "结束", Type: "end"},
				},
			},
			wantValid: false,
			wantErrs:  1,
		},
		{
			name: "缺少 end 节点",
			def: &GraphDefinition{
				Name: "无 end",
				Nodes: []GraphNodeDef{
					{ID: "s", Name: "开始", Type: "start"},
					{ID: "a", Name: "Agent", Type: "agent"},
				},
			},
			wantValid: false,
			wantErrs:  1,
		},
		{
			name: "节点缺少名称",
			def: &GraphDefinition{
				Name: "无名节点",
				Nodes: []GraphNodeDef{
					{ID: "s", Name: "开始", Type: "start"},
					{ID: "a", Name: "", Type: "agent"},
					{ID: "e", Name: "结束", Type: "end"},
				},
			},
			wantValid: false,
			wantErrs:  1,
		},
		{
			name: "边引用不存在的节点",
			def: &GraphDefinition{
				Name: "断边",
				Nodes: []GraphNodeDef{
					{ID: "s", Name: "开始", Type: "start"},
					{ID: "e", Name: "结束", Type: "end"},
				},
				Edges: []GraphEdgeDef{
					{ID: "e1", Source: "s", Target: "not-exist"},
				},
			},
			wantValid: false,
			wantErrs:  1,
		},
		{
			name: "多个 start 节点",
			def: &GraphDefinition{
				Name: "多 start",
				Nodes: []GraphNodeDef{
					{ID: "s1", Name: "开始1", Type: "start"},
					{ID: "s2", Name: "开始2", Type: "start"},
					{ID: "e", Name: "结束", Type: "end"},
				},
			},
			wantValid: false,
			wantErrs:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.Validate(tt.def)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v，期望 %v，错误: %v", result.Valid, tt.wantValid, result.Errors)
			}
			if !tt.wantValid && len(result.Errors) < tt.wantErrs {
				t.Errorf("错误数 = %d，期望至少 %d", len(result.Errors), tt.wantErrs)
			}
		})
	}
}

// TestBuilderHandler_CRUD 测试 Builder HTTP handler 的完整流程
func TestBuilderHandler_CRUD(t *testing.T) {
	store := NewGraphStore()
	collector := NewCollector(100)
	bh := newBuilderHandler(store, collector)

	// 1. 创建图定义
	createBody := GraphDefinition{
		Name: "测试图",
		Nodes: []GraphNodeDef{
			{ID: "s", Name: "开始", Type: "start", Position: Position{X: 100, Y: 100}},
			{ID: "a", Name: "Agent", Type: "agent", Position: Position{X: 300, Y: 100}},
			{ID: "e", Name: "结束", Type: "end", Position: Position{X: 500, Y: 100}},
		},
		Edges: []GraphEdgeDef{
			{ID: "e1", Source: "s", Target: "a"},
			{ID: "e2", Source: "a", Target: "e"},
		},
		EntryPoint: "s",
	}

	body, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/builder/graphs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	bh.handleGraphs(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("创建应返回 201，实际返回 %d: %s", w.Code, w.Body.String())
	}

	var createResp response
	if err := json.NewDecoder(w.Body).Decode(&createResp); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if !createResp.Success {
		t.Fatal("创建应成功")
	}

	// 提取创建的 ID
	dataMap, ok := createResp.Data.(map[string]any)
	if !ok {
		t.Fatal("响应数据应为 map")
	}
	graphID, ok := dataMap["id"].(string)
	if !ok || graphID == "" {
		t.Fatal("响应中应包含非空的 id")
	}

	// 2. 列出图定义
	req = httptest.NewRequest(http.MethodGet, "/api/builder/graphs", nil)
	w = httptest.NewRecorder()
	bh.handleGraphs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("列表应返回 200，实际返回 %d", w.Code)
	}

	// 3. 获取单个图定义
	req = httptest.NewRequest(http.MethodGet, "/api/builder/graphs/"+graphID, nil)
	w = httptest.NewRecorder()
	bh.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("获取应返回 200，实际返回 %d: %s", w.Code, w.Body.String())
	}

	// 4. 更新图定义
	createBody.Name = "更新后的图"
	body, _ = json.Marshal(createBody)
	req = httptest.NewRequest(http.MethodPut, "/api/builder/graphs/"+graphID, bytes.NewReader(body))
	w = httptest.NewRecorder()
	bh.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("更新应返回 200，实际返回 %d: %s", w.Code, w.Body.String())
	}

	// 5. 验证图
	req = httptest.NewRequest(http.MethodPost, "/api/builder/graphs/"+graphID+"/validate", nil)
	w = httptest.NewRecorder()
	bh.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("验证应返回 200，实际返回 %d: %s", w.Code, w.Body.String())
	}

	// 6. 获取节点类型
	req = httptest.NewRequest(http.MethodGet, "/api/builder/node-types", nil)
	w = httptest.NewRecorder()
	bh.handleNodeTypes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("节点类型应返回 200，实际返回 %d", w.Code)
	}

	// 7. 删除图定义
	req = httptest.NewRequest(http.MethodDelete, "/api/builder/graphs/"+graphID, nil)
	w = httptest.NewRecorder()
	bh.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("删除应返回 200，实际返回 %d: %s", w.Code, w.Body.String())
	}

	// 删除后获取应返回 404
	req = httptest.NewRequest(http.MethodGet, "/api/builder/graphs/"+graphID, nil)
	w = httptest.NewRecorder()
	bh.handleGraph(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("删除后获取应返回 404，实际返回 %d", w.Code)
	}
}

// TestBuilderHandler_CreateErrors 测试创建时的错误处理
func TestBuilderHandler_CreateErrors(t *testing.T) {
	store := NewGraphStore()
	collector := NewCollector(100)
	bh := newBuilderHandler(store, collector)

	// 空名称
	body, _ := json.Marshal(GraphDefinition{Name: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/builder/graphs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	bh.handleGraphs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("空名称应返回 400，实际返回 %d", w.Code)
	}

	// 无效 JSON
	req = httptest.NewRequest(http.MethodPost, "/api/builder/graphs", bytes.NewReader([]byte("invalid")))
	w = httptest.NewRecorder()
	bh.handleGraphs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("无效 JSON 应返回 400，实际返回 %d", w.Code)
	}

	// 不允许的方法
	req = httptest.NewRequest(http.MethodDelete, "/api/builder/graphs", nil)
	w = httptest.NewRecorder()
	bh.handleGraphs(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE 应返回 405，实际返回 %d", w.Code)
	}
}

// TestBuilderExecutor_Execute 测试图执行
func TestBuilderExecutor_Execute(t *testing.T) {
	collector := NewCollector(100)
	executor := NewBuilderExecutor(collector)

	def := &GraphDefinition{
		ID:   "test-graph",
		Name: "测试执行图",
		Nodes: []GraphNodeDef{
			{ID: "s", Name: "开始", Type: "start", Position: Position{X: 100, Y: 100}},
			{ID: "a1", Name: "Agent 1", Type: "agent", Position: Position{X: 300, Y: 100},
				Config: map[string]any{"agent_ref": "assistant", "system_prompt": "你是一个助手"}},
			{ID: "t1", Name: "搜索工具", Type: "tool", Position: Position{X: 500, Y: 100},
				Config: map[string]any{"tool_name": "search"}},
			{ID: "e", Name: "结束", Type: "end", Position: Position{X: 700, Y: 100}},
		},
		Edges: []GraphEdgeDef{
			{ID: "e1", Source: "s", Target: "a1"},
			{ID: "e2", Source: "a1", Target: "t1"},
			{ID: "e3", Source: "t1", Target: "e"},
		},
		EntryPoint: "s",
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, def, map[string]any{"input": "hello"})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.Status != "completed" {
		t.Fatalf("执行状态应为 completed，实际为 %s, error: %s", result.Status, result.Error)
	}
	if result.RunID == "" {
		t.Fatal("RunID 不应为空")
	}
	if result.GraphID != "test-graph" {
		t.Fatalf("GraphID 应为 test-graph，实际为 %s", result.GraphID)
	}
	if len(result.NodeResults) < 2 {
		t.Fatalf("至少应有 2 个节点执行结果，实际为 %d", len(result.NodeResults))
	}
	if result.DurationMs < 0 {
		t.Fatalf("执行耗时不应为负数: %d", result.DurationMs)
	}
}

// TestBuilderExecutor_InvalidGraph 测试执行无效图
func TestBuilderExecutor_InvalidGraph(t *testing.T) {
	collector := NewCollector(100)
	executor := NewBuilderExecutor(collector)

	// 没有出边的 start 节点
	def := &GraphDefinition{
		ID:   "invalid",
		Name: "无效图",
		Nodes: []GraphNodeDef{
			{ID: "s", Name: "开始", Type: "start"},
			{ID: "e", Name: "结束", Type: "end"},
		},
		Edges: []GraphEdgeDef{},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, def, nil)
	if err != nil {
		t.Fatalf("应返回 result 而非 error: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("无效图执行应失败，实际状态: %s", result.Status)
	}
}

// TestNodeTypes 测试节点类型列表
func TestNodeTypes(t *testing.T) {
	if len(defaultNodeTypes) == 0 {
		t.Fatal("默认节点类型列表不应为空")
	}

	types := make(map[string]bool)
	for _, nt := range defaultNodeTypes {
		if nt.Type == "" {
			t.Fatal("节点类型标识不应为空")
		}
		if nt.Name == "" {
			t.Fatalf("节点类型 %s 的名称不应为空", nt.Type)
		}
		if types[nt.Type] {
			t.Fatalf("节点类型 %s 重复定义", nt.Type)
		}
		types[nt.Type] = true
	}

	// 确保关键类型都存在
	required := []string{"start", "end", "agent", "tool", "llm", "condition"}
	for _, r := range required {
		if !types[r] {
			t.Fatalf("缺少必要的节点类型: %s", r)
		}
	}
}
