package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============== 辅助函数 ==============

// newTestCheckpoint 创建一个用于测试的检查点
// 包含完整的字段数据，便于验证序列化/反序列化的正确性
func newTestCheckpoint(threadID, graphName, currentNode string) *Checkpoint {
	state := map[string]any{
		"counter": 42,
		"message": "测试状态",
		"nested": map[string]any{
			"key": "value",
		},
	}
	stateJSON, _ := json.Marshal(state)

	return &Checkpoint{
		ThreadID:       threadID,
		GraphName:      graphName,
		CurrentNode:    currentNode,
		State:          json.RawMessage(stateJSON),
		PendingNodes:   []string{"node_b", "node_c"},
		CompletedNodes: []string{"node_a"},
		Metadata: map[string]any{
			"step":    1,
			"trigger": "manual",
		},
		ParentID: "",
	}
}

// newTestCheckpointWithID 创建一个带有指定 ID 的测试检查点
func newTestCheckpointWithID(id, threadID, graphName string) *Checkpoint {
	cp := newTestCheckpoint(threadID, graphName, "node_start")
	cp.ID = id
	return cp
}

// ============== MemoryCheckpointSaver 测试 ==============

// TestMemoryCheckpointSaver_SaveAndLoad 测试内存保存器的保存和加载功能
func TestMemoryCheckpointSaver_SaveAndLoad(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 创建并保存检查点
	cp := newTestCheckpoint("thread-1", "test-graph", "node_a")
	err := saver.Save(ctx, cp)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	// 验证 ID 已自动生成
	if cp.ID == "" {
		t.Fatal("检查点 ID 未自动生成")
	}

	// 验证时间戳已设置
	if cp.CreatedAt.IsZero() {
		t.Fatal("CreatedAt 未设置")
	}
	if cp.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt 未设置")
	}

	// 加载最新的检查点
	loaded, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}

	// 验证所有字段
	if loaded.ID != cp.ID {
		t.Errorf("ID 不匹配: 期望 %s, 实际 %s", cp.ID, loaded.ID)
	}
	if loaded.ThreadID != "thread-1" {
		t.Errorf("ThreadID 不匹配: 期望 thread-1, 实际 %s", loaded.ThreadID)
	}
	if loaded.GraphName != "test-graph" {
		t.Errorf("GraphName 不匹配: 期望 test-graph, 实际 %s", loaded.GraphName)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}
	if len(loaded.PendingNodes) != 2 {
		t.Errorf("PendingNodes 长度不匹配: 期望 2, 实际 %d", len(loaded.PendingNodes))
	}
	if len(loaded.CompletedNodes) != 1 {
		t.Errorf("CompletedNodes 长度不匹配: 期望 1, 实际 %d", len(loaded.CompletedNodes))
	}
	if loaded.Metadata["step"] != 1 {
		t.Errorf("Metadata[step] 不匹配: 期望 1, 实际 %v", loaded.Metadata["step"])
	}

	// 验证 State 字段的内容
	var stateMap map[string]any
	if err := json.Unmarshal(loaded.State, &stateMap); err != nil {
		t.Fatalf("解析 State 失败: %v", err)
	}
	if stateMap["message"] != "测试状态" {
		t.Errorf("State.message 不匹配: 期望 '测试状态', 实际 '%v'", stateMap["message"])
	}
}

// TestMemoryCheckpointSaver_SaveAndLoad_Latest 测试多次保存后加载最新的检查点
func TestMemoryCheckpointSaver_SaveAndLoad_Latest(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 保存多个检查点
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-1", "test-graph", "node_c")

	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 加载最新的检查点应该是第三个
	loaded, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_c" {
		t.Errorf("期望最新检查点的 CurrentNode 为 node_c, 实际 %s", loaded.CurrentNode)
	}
}

// TestMemoryCheckpointSaver_Load_NotFound 测试加载不存在的线程
func TestMemoryCheckpointSaver_Load_NotFound(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	_, err := saver.Load(ctx, "nonexistent-thread")
	if err == nil {
		t.Fatal("加载不存在的线程应该返回错误")
	}
}

// TestMemoryCheckpointSaver_LoadByID 测试根据 ID 加载检查点
func TestMemoryCheckpointSaver_LoadByID(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 保存多个检查点
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)

	// 通过 ID 加载第一个检查点
	loaded, err := saver.LoadByID(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}

	// 通过 ID 加载第二个检查点
	loaded, err = saver.LoadByID(ctx, cp2.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}
}

// TestMemoryCheckpointSaver_LoadByID_NotFound 测试根据不存在的 ID 加载
func TestMemoryCheckpointSaver_LoadByID_NotFound(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	_, err := saver.LoadByID(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("加载不存在的检查点应该返回错误")
	}
}

// TestMemoryCheckpointSaver_List 测试列出线程的所有检查点
func TestMemoryCheckpointSaver_List(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 保存多个检查点到同一线程
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-1", "test-graph", "node_c")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 保存一个到不同线程
	cp4 := newTestCheckpoint("thread-2", "test-graph", "node_x")
	_ = saver.Save(ctx, cp4)

	// 列出 thread-1 的检查点
	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("检查点数量不匹配: 期望 3, 实际 %d", len(list))
	}

	// 验证顺序（按保存顺序）
	if list[0].CurrentNode != "node_a" {
		t.Errorf("第一个检查点的 CurrentNode 不匹配: 期望 node_a, 实际 %s", list[0].CurrentNode)
	}
	if list[2].CurrentNode != "node_c" {
		t.Errorf("第三个检查点的 CurrentNode 不匹配: 期望 node_c, 实际 %s", list[2].CurrentNode)
	}

	// 列出 thread-2 的检查点
	list2, err := saver.List(ctx, "thread-2")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("检查点数量不匹配: 期望 1, 实际 %d", len(list2))
	}
}

// TestMemoryCheckpointSaver_List_Empty 测试列出不存在线程的检查点
func TestMemoryCheckpointSaver_List_Empty(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	list, err := saver.List(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("列出不存在线程的检查点不应返回错误: %v", err)
	}
	if list != nil {
		t.Errorf("不存在的线程应返回 nil, 实际 %v", list)
	}
}

// TestMemoryCheckpointSaver_Delete 测试删除检查点
func TestMemoryCheckpointSaver_Delete(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 保存两个检查点
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)

	// 删除第一个检查点
	err := saver.Delete(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("删除检查点失败: %v", err)
	}

	// 验证第一个检查点已被删除
	_, err = saver.LoadByID(ctx, cp1.ID)
	if err == nil {
		t.Fatal("已删除的检查点不应被找到")
	}

	// 验证第二个检查点仍然存在
	loaded, err := saver.LoadByID(ctx, cp2.ID)
	if err != nil {
		t.Fatalf("第二个检查点应该仍然存在: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}

	// 验证列表中只剩一个检查点
	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("检查点数量不匹配: 期望 1, 实际 %d", len(list))
	}
}

// TestMemoryCheckpointSaver_Delete_NotFound 测试删除不存在的检查点
func TestMemoryCheckpointSaver_Delete_NotFound(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 删除不存在的检查点应该不返回错误
	err := saver.Delete(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("删除不存在的检查点不应返回错误: %v", err)
	}
}

// TestMemoryCheckpointSaver_DeleteThread 测试删除线程的所有检查点
func TestMemoryCheckpointSaver_DeleteThread(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 保存多个检查点到两个线程
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-2", "test-graph", "node_x")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 删除 thread-1 的所有检查点
	err := saver.DeleteThread(ctx, "thread-1")
	if err != nil {
		t.Fatalf("删除线程检查点失败: %v", err)
	}

	// 验证 thread-1 的检查点已被删除
	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if list != nil {
		t.Errorf("thread-1 的检查点应该全部被删除, 但仍有 %d 个", len(list))
	}

	// 验证 thread-2 的检查点仍然存在
	list2, err := saver.List(ctx, "thread-2")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("thread-2 的检查点数量不匹配: 期望 1, 实际 %d", len(list2))
	}
}

// TestMemoryCheckpointSaver_DeleteThread_NotFound 测试删除不存在的线程
func TestMemoryCheckpointSaver_DeleteThread_NotFound(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 删除不存在的线程应该不返回错误
	err := saver.DeleteThread(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("删除不存在的线程不应返回错误: %v", err)
	}
}

// TestMemoryCheckpointSaver_SaveStoresCopy 测试保存时会复制检查点，避免外部修改污染已保存数据
func TestMemoryCheckpointSaver_SaveStoresCopy(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	cp := newTestCheckpoint("thread-copy", "test-graph", "node_a")
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	// 修改原始对象
	cp.CurrentNode = "node_mutated"
	cp.PendingNodes[0] = "node_mutated"
	cp.Metadata["step"] = 999

	loaded, err := saver.Load(ctx, "thread-copy")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}

	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不应受外部修改影响: got %s", loaded.CurrentNode)
	}
	if len(loaded.PendingNodes) == 0 || loaded.PendingNodes[0] != "node_b" {
		t.Errorf("PendingNodes 不应受外部修改影响: got %v", loaded.PendingNodes)
	}
	if loaded.Metadata["step"] != 1 {
		t.Errorf("Metadata 不应受外部修改影响: got %v", loaded.Metadata["step"])
	}
}

// TestMemoryCheckpointSaver_LoadReturnsCopy 测试加载返回副本，避免调用方修改内部存储
func TestMemoryCheckpointSaver_LoadReturnsCopy(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	cp := newTestCheckpoint("thread-load-copy", "test-graph", "node_a")
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	loaded1, err := saver.Load(ctx, "thread-load-copy")
	if err != nil {
		t.Fatalf("首次加载检查点失败: %v", err)
	}

	// 修改首次加载结果
	loaded1.CurrentNode = "node_changed"
	loaded1.Metadata["step"] = 100

	loaded2, err := saver.Load(ctx, "thread-load-copy")
	if err != nil {
		t.Fatalf("二次加载检查点失败: %v", err)
	}

	if loaded2.CurrentNode != "node_a" {
		t.Errorf("内部存储不应被调用方修改: got %s", loaded2.CurrentNode)
	}
	if loaded2.Metadata["step"] != 1 {
		t.Errorf("内部 Metadata 不应被调用方修改: got %v", loaded2.Metadata["step"])
	}
}

// TestMemoryCheckpointSaver_SaveSameIDNoDuplicate 测试同一 ID 重复保存不会导致线程索引重复
func TestMemoryCheckpointSaver_SaveSameIDNoDuplicate(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	cp := newTestCheckpointWithID("fixed-id", "thread-fixed", "test-graph")
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}

	cp.CurrentNode = "node_updated"
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}

	list, err := saver.List(ctx, "thread-fixed")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("同一 ID 不应重复入索引: got %d", len(list))
	}
	if list[0].CurrentNode != "node_updated" {
		t.Errorf("应保留最新数据: got %s", list[0].CurrentNode)
	}
}

// TestMemoryCheckpointSaver_AutoGenerateID 测试 ID 自动生成
func TestMemoryCheckpointSaver_AutoGenerateID(t *testing.T) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	// 不设置 ID 的检查点
	cp := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp.ID = "" // 确保 ID 为空
	err := saver.Save(ctx, cp)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	if cp.ID == "" {
		t.Fatal("检查点 ID 应该被自动生成")
	}

	// 预设 ID 的检查点应该保留原 ID
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp2.ID = "custom-id-123"
	err = saver.Save(ctx, cp2)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	if cp2.ID != "custom-id-123" {
		t.Errorf("检查点 ID 不应被修改: 期望 custom-id-123, 实际 %s", cp2.ID)
	}

	// 通过自定义 ID 加载
	loaded, err := saver.LoadByID(ctx, "custom-id-123")
	if err != nil {
		t.Fatalf("通过自定义 ID 加载失败: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}
}

// ============== FileCheckpointSaver 测试 ==============

// TestFileCheckpointSaver_SaveAndLoad 测试文件保存器的保存和加载功能
func TestFileCheckpointSaver_SaveAndLoad(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 创建并保存检查点
	cp := newTestCheckpoint("thread-1", "test-graph", "node_a")
	err = saver.Save(ctx, cp)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	// 验证 ID 已自动生成
	if cp.ID == "" {
		t.Fatal("检查点 ID 未自动生成")
	}

	// 验证时间戳已设置
	if cp.CreatedAt.IsZero() {
		t.Fatal("CreatedAt 未设置")
	}
	if cp.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt 未设置")
	}

	// 加载最新的检查点
	loaded, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}

	// 验证所有字段
	if loaded.ID != cp.ID {
		t.Errorf("ID 不匹配: 期望 %s, 实际 %s", cp.ID, loaded.ID)
	}
	if loaded.ThreadID != "thread-1" {
		t.Errorf("ThreadID 不匹配: 期望 thread-1, 实际 %s", loaded.ThreadID)
	}
	if loaded.GraphName != "test-graph" {
		t.Errorf("GraphName 不匹配: 期望 test-graph, 实际 %s", loaded.GraphName)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}
	if len(loaded.PendingNodes) != 2 {
		t.Errorf("PendingNodes 长度不匹配: 期望 2, 实际 %d", len(loaded.PendingNodes))
	}
	if len(loaded.CompletedNodes) != 1 {
		t.Errorf("CompletedNodes 长度不匹配: 期望 1, 实际 %d", len(loaded.CompletedNodes))
	}

	// 验证 Metadata
	if loaded.Metadata == nil {
		t.Fatal("Metadata 不应为 nil")
	}
	// JSON 反序列化后数字可能变为 float64
	step, ok := loaded.Metadata["step"].(float64)
	if !ok {
		t.Fatalf("Metadata[step] 类型不正确: %T", loaded.Metadata["step"])
	}
	if step != 1 {
		t.Errorf("Metadata[step] 不匹配: 期望 1, 实际 %v", step)
	}

	// 验证 State 字段的内容
	var stateMap map[string]any
	if err := json.Unmarshal(loaded.State, &stateMap); err != nil {
		t.Fatalf("解析 State 失败: %v", err)
	}
	if stateMap["message"] != "测试状态" {
		t.Errorf("State.message 不匹配: 期望 '测试状态', 实际 '%v'", stateMap["message"])
	}
}

// TestFileCheckpointSaver_SaveAndLoad_Latest 测试多次保存后加载最新的检查点
func TestFileCheckpointSaver_SaveAndLoad_Latest(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-1", "test-graph", "node_c")

	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	loaded, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_c" {
		t.Errorf("期望最新检查点的 CurrentNode 为 node_c, 实际 %s", loaded.CurrentNode)
	}
}

// TestFileCheckpointSaver_Load_NotFound 测试加载不存在的线程
func TestFileCheckpointSaver_Load_NotFound(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	_, err = saver.Load(ctx, "nonexistent-thread")
	if err == nil {
		t.Fatal("加载不存在的线程应该返回错误")
	}
}

// TestFileCheckpointSaver_LoadByID 测试根据 ID 加载检查点
func TestFileCheckpointSaver_LoadByID(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 保存多个检查点到不同线程
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-2", "test-graph", "node_x")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 通过 ID 加载第一个检查点
	loaded, err := saver.LoadByID(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}

	// 通过 ID 加载第三个检查点（不同线程）
	loaded, err = saver.LoadByID(ctx, cp3.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_x" {
		t.Errorf("CurrentNode 不匹配: 期望 node_x, 实际 %s", loaded.CurrentNode)
	}
}

// TestFileCheckpointSaver_LoadByID_NotFound 测试根据不存在的 ID 加载
func TestFileCheckpointSaver_LoadByID_NotFound(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	_, err = saver.LoadByID(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("加载不存在的检查点应该返回错误")
	}
}

// TestFileCheckpointSaver_List 测试列出线程的所有检查点
func TestFileCheckpointSaver_List(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 保存多个检查点到同一线程
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-1", "test-graph", "node_c")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 保存一个到不同线程
	cp4 := newTestCheckpoint("thread-2", "test-graph", "node_x")
	_ = saver.Save(ctx, cp4)

	// 列出 thread-1 的检查点
	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("检查点数量不匹配: 期望 3, 实际 %d", len(list))
	}

	// 验证顺序
	if list[0].CurrentNode != "node_a" {
		t.Errorf("第一个检查点的 CurrentNode 不匹配: 期望 node_a, 实际 %s", list[0].CurrentNode)
	}
	if list[2].CurrentNode != "node_c" {
		t.Errorf("第三个检查点的 CurrentNode 不匹配: 期望 node_c, 实际 %s", list[2].CurrentNode)
	}

	// 列出 thread-2 的检查点
	list2, err := saver.List(ctx, "thread-2")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("检查点数量不匹配: 期望 1, 实际 %d", len(list2))
	}
}

// TestFileCheckpointSaver_List_Empty 测试列出不存在线程的检查点
func TestFileCheckpointSaver_List_Empty(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	list, err := saver.List(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("列出不存在线程的检查点不应返回错误: %v", err)
	}
	if list != nil && len(list) != 0 {
		t.Errorf("不存在的线程应返回空列表, 实际 %d 个", len(list))
	}
}

// TestFileCheckpointSaver_Delete 测试删除检查点
func TestFileCheckpointSaver_Delete(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 保存两个检查点
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)

	// 删除第一个检查点
	err = saver.Delete(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("删除检查点失败: %v", err)
	}

	// 验证第一个检查点已被删除
	_, err = saver.LoadByID(ctx, cp1.ID)
	if err == nil {
		t.Fatal("已删除的检查点不应被找到")
	}

	// 验证第二个检查点仍然存在
	loaded, err := saver.LoadByID(ctx, cp2.ID)
	if err != nil {
		t.Fatalf("第二个检查点应该仍然存在: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}

	// 验证列表中只剩一个检查点
	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("检查点数量不匹配: 期望 1, 实际 %d", len(list))
	}
}

// TestFileCheckpointSaver_Delete_NotFound 测试删除不存在的检查点
func TestFileCheckpointSaver_Delete_NotFound(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 删除不存在的检查点应该不返回错误
	err = saver.Delete(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("删除不存在的检查点不应返回错误: %v", err)
	}
}

// TestFileCheckpointSaver_DeleteThread 测试删除线程的所有检查点
func TestFileCheckpointSaver_DeleteThread(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 保存多个检查点到两个线程
	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-2", "test-graph", "node_x")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 删除 thread-1 的所有检查点
	err = saver.DeleteThread(ctx, "thread-1")
	if err != nil {
		t.Fatalf("删除线程检查点失败: %v", err)
	}

	// 验证 thread-1 的检查点已被删除
	_, err = saver.Load(ctx, "thread-1")
	if err == nil {
		t.Fatal("thread-1 的检查点应该全部被删除")
	}

	list, err := saver.List(ctx, "thread-1")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("thread-1 的检查点应该全部被删除, 但仍有 %d 个", len(list))
	}

	// 验证 thread-2 的检查点仍然存在
	list2, err := saver.List(ctx, "thread-2")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("thread-2 的检查点数量不匹配: 期望 1, 实际 %d", len(list2))
	}
}

// TestFileCheckpointSaver_DeleteThread_NotFound 测试删除不存在的线程
func TestFileCheckpointSaver_DeleteThread_NotFound(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 删除不存在的线程应该不返回错误
	err = saver.DeleteThread(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("删除不存在的线程不应返回错误: %v", err)
	}
}

// TestFileCheckpointSaver_Persistence 测试文件持久化
// 创建一个保存器写入数据，然后创建新的保存器实例读取，验证数据持久性
func TestFileCheckpointSaver_Persistence(t *testing.T) {
	baseDir := t.TempDir()
	ctx := context.Background()

	// 使用第一个保存器实例写入数据
	saver1, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建第一个保存器失败: %v", err)
	}

	cp1 := newTestCheckpoint("thread-persist", "persist-graph", "node_a")
	cp1.ParentID = "parent-001"
	err = saver1.Save(ctx, cp1)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	cp2 := newTestCheckpoint("thread-persist", "persist-graph", "node_b")
	cp2.ParentID = cp1.ID
	err = saver1.Save(ctx, cp2)
	if err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	savedID1 := cp1.ID
	savedID2 := cp2.ID

	// 创建新的保存器实例，使用相同的目录
	saver2, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建第二个保存器失败: %v", err)
	}

	// 通过新保存器加载最新的检查点
	loaded, err := saver2.Load(ctx, "thread-persist")
	if err != nil {
		t.Fatalf("新保存器加载检查点失败: %v", err)
	}
	if loaded.ID != savedID2 {
		t.Errorf("ID 不匹配: 期望 %s, 实际 %s", savedID2, loaded.ID)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}
	if loaded.ParentID != savedID1 {
		t.Errorf("ParentID 不匹配: 期望 %s, 实际 %s", savedID1, loaded.ParentID)
	}

	// 通过新保存器根据 ID 加载第一个检查点
	loaded1, err := saver2.LoadByID(ctx, savedID1)
	if err != nil {
		t.Fatalf("新保存器根据 ID 加载检查点失败: %v", err)
	}
	if loaded1.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded1.CurrentNode)
	}
	if loaded1.GraphName != "persist-graph" {
		t.Errorf("GraphName 不匹配: 期望 persist-graph, 实际 %s", loaded1.GraphName)
	}

	// 验证 State 数据完整性
	var stateMap map[string]any
	if err := json.Unmarshal(loaded1.State, &stateMap); err != nil {
		t.Fatalf("解析 State 失败: %v", err)
	}
	if stateMap["message"] != "测试状态" {
		t.Errorf("State.message 不匹配: 期望 '测试状态', 实际 '%v'", stateMap["message"])
	}
	counter, ok := stateMap["counter"].(float64)
	if !ok || counter != 42 {
		t.Errorf("State.counter 不匹配: 期望 42, 实际 %v", stateMap["counter"])
	}

	// 通过新保存器列出所有检查点
	list, err := saver2.List(ctx, "thread-persist")
	if err != nil {
		t.Fatalf("新保存器列出检查点失败: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("检查点数量不匹配: 期望 2, 实际 %d", len(list))
	}

	// 验证 Metadata 的持久化
	if loaded1.Metadata == nil {
		t.Fatal("持久化后 Metadata 不应为 nil")
	}
	trigger, ok := loaded1.Metadata["trigger"].(string)
	if !ok || trigger != "manual" {
		t.Errorf("Metadata[trigger] 不匹配: 期望 'manual', 实际 '%v'", loaded1.Metadata["trigger"])
	}

	// 验证时间戳的持久化
	if loaded1.CreatedAt.IsZero() {
		t.Error("持久化后 CreatedAt 不应为零值")
	}
	if loaded1.UpdatedAt.IsZero() {
		t.Error("持久化后 UpdatedAt 不应为零值")
	}
}

// TestFileCheckpointSaver_ConcurrentAccess 测试文件保存器的并发安全性
func TestFileCheckpointSaver_ConcurrentAccess(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	// 并发写入和读取
	const goroutines = 20
	const checkpointsPerGoroutine = 5
	var wg sync.WaitGroup
	errChan := make(chan error, goroutines*checkpointsPerGoroutine)

	// 并发写入
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(threadNum int) {
			defer wg.Done()
			threadID := fmt.Sprintf("thread-%d", threadNum)
			for j := 0; j < checkpointsPerGoroutine; j++ {
				cp := newTestCheckpoint(threadID, "concurrent-graph", fmt.Sprintf("node_%d_%d", threadNum, j))
				if err := saver.Save(ctx, cp); err != nil {
					errChan <- fmt.Errorf("线程 %d 保存检查点 %d 失败: %w", threadNum, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		t.Error(err)
	}

	// 验证所有检查点都已正确保存
	for i := 0; i < goroutines; i++ {
		threadID := fmt.Sprintf("thread-%d", i)
		list, err := saver.List(ctx, threadID)
		if err != nil {
			t.Errorf("列出线程 %s 的检查点失败: %v", threadID, err)
			continue
		}
		if len(list) != checkpointsPerGoroutine {
			t.Errorf("线程 %s 的检查点数量不匹配: 期望 %d, 实际 %d", threadID, checkpointsPerGoroutine, len(list))
		}
	}

	// 并发读写混合测试
	var wg2 sync.WaitGroup

	// 并发写入
	for i := 0; i < 10; i++ {
		wg2.Add(1)
		go func(n int) {
			defer wg2.Done()
			cp := newTestCheckpoint("thread-mixed", "concurrent-graph", fmt.Sprintf("mixed_write_%d", n))
			_ = saver.Save(ctx, cp)
		}(i)
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			// 读取可能返回错误（线程可能还未创建），忽略错误
			_, _ = saver.List(ctx, "thread-mixed")
		}()
	}

	wg2.Wait()
}

// TestFileCheckpointSaver_SaveSameIDNoDuplicate 测试同 ID 重复保存不会重复写入线程索引，且会保留原 CreatedAt
func TestFileCheckpointSaver_SaveSameIDNoDuplicate(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	cp1 := newTestCheckpoint("thread-file-dup", "dup-graph", "node_a")
	if err := saver.Save(ctx, cp1); err != nil {
		t.Fatalf("首次保存检查点失败: %v", err)
	}
	createdAt := cp1.CreatedAt

	cp2 := newTestCheckpoint("thread-file-dup", "dup-graph", "node_b")
	cp2.ID = cp1.ID
	cp2.CreatedAt = time.Time{} // 验证会沿用已存在的 CreatedAt
	if err := saver.Save(ctx, cp2); err != nil {
		t.Fatalf("同 ID 再次保存检查点失败: %v", err)
	}

	list, err := saver.List(ctx, "thread-file-dup")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("同 ID 重复保存不应产生重复索引, 实际 %d", len(list))
	}

	loaded, err := saver.LoadByID(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("按 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("检查点内容未更新: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}
	if !loaded.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt 未沿用旧值: 期望 %v, 实际 %v", createdAt, loaded.CreatedAt)
	}
}

// TestFileCheckpointSaver_SaveValidation 测试文件保存器参数校验行为
func TestFileCheckpointSaver_SaveValidation(t *testing.T) {
	baseDir := t.TempDir()
	saver, err := NewFileCheckpointSaver(baseDir)
	if err != nil {
		t.Fatalf("创建文件检查点保存器失败: %v", err)
	}
	ctx := context.Background()

	if err := saver.Save(ctx, nil); err == nil {
		t.Fatal("保存 nil 检查点应返回错误")
	}

	cp := newTestCheckpoint("", "graph", "node_a")
	if err := saver.Save(ctx, cp); err == nil {
		t.Fatal("保存缺少 thread_id 的检查点应返回错误")
	}

	if _, err := saver.LoadByID(ctx, ""); err == nil {
		t.Fatal("LoadByID 空 ID 应返回错误")
	}

	if err := saver.Delete(ctx, ""); err == nil {
		t.Fatal("Delete 空 ID 应返回错误")
	}
}

// ============== CheckpointSaver 接口统一测试 ==============

// TestCheckpointSaver_Interface 使用相同的测试用例测试所有 CheckpointSaver 实现
// 确保所有实现的行为一致
func TestCheckpointSaver_Interface(t *testing.T) {
	// 构建测试用的保存器工厂
	saverFactories := map[string]func(t *testing.T) CheckpointSaver{
		"Memory": func(t *testing.T) CheckpointSaver {
			return NewMemoryCheckpointSaver()
		},
		"File": func(t *testing.T) CheckpointSaver {
			baseDir := t.TempDir()
			saver, err := NewFileCheckpointSaver(baseDir)
			if err != nil {
				t.Fatalf("创建文件检查点保存器失败: %v", err)
			}
			return saver
		},
	}

	for name, factory := range saverFactories {
		t.Run(name, func(t *testing.T) {
			t.Run("SaveAndLoad", func(t *testing.T) {
				saver := factory(t)
				testSaverSaveAndLoad(t, saver)
			})

			t.Run("LoadByID", func(t *testing.T) {
				saver := factory(t)
				testSaverLoadByID(t, saver)
			})

			t.Run("List", func(t *testing.T) {
				saver := factory(t)
				testSaverList(t, saver)
			})

			t.Run("Delete", func(t *testing.T) {
				saver := factory(t)
				testSaverDelete(t, saver)
			})

			t.Run("DeleteThread", func(t *testing.T) {
				saver := factory(t)
				testSaverDeleteThread(t, saver)
			})

			t.Run("NotFound", func(t *testing.T) {
				saver := factory(t)
				testSaverNotFound(t, saver)
			})

			t.Run("MultipleThreads", func(t *testing.T) {
				saver := factory(t)
				testSaverMultipleThreads(t, saver)
			})

			t.Run("StatePreservation", func(t *testing.T) {
				saver := factory(t)
				testSaverStatePreservation(t, saver)
			})

			t.Run("TimestampBehavior", func(t *testing.T) {
				saver := factory(t)
				testSaverTimestampBehavior(t, saver)
			})
		})
	}
}

// testSaverSaveAndLoad 测试保存和加载功能
func testSaverSaveAndLoad(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	cp := newTestCheckpoint("thread-1", "test-graph", "node_a")
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}
	if cp.ID == "" {
		t.Fatal("检查点 ID 未自动生成")
	}

	loaded, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}
	if loaded.ID != cp.ID {
		t.Errorf("ID 不匹配: 期望 %s, 实际 %s", cp.ID, loaded.ID)
	}
	if loaded.ThreadID != "thread-1" {
		t.Errorf("ThreadID 不匹配: 期望 thread-1, 实际 %s", loaded.ThreadID)
	}
	if loaded.GraphName != "test-graph" {
		t.Errorf("GraphName 不匹配: 期望 test-graph, 实际 %s", loaded.GraphName)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}
}

// testSaverLoadByID 测试根据 ID 加载
func testSaverLoadByID(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	cp1 := newTestCheckpoint("thread-1", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-1", "test-graph", "node_b")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)

	// 根据 ID 加载第一个
	loaded, err := saver.LoadByID(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}

	// Load 返回最新的
	latest, err := saver.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载最新检查点失败: %v", err)
	}
	if latest.CurrentNode != "node_b" {
		t.Errorf("最新检查点 CurrentNode 不匹配: 期望 node_b, 实际 %s", latest.CurrentNode)
	}
}

// testSaverList 测试列出功能
func testSaverList(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 保存 3 个检查点
	for i, node := range []string{"node_a", "node_b", "node_c"} {
		cp := newTestCheckpoint("thread-list", "test-graph", node)
		cp.ID = fmt.Sprintf("cp-%d", i)
		if err := saver.Save(ctx, cp); err != nil {
			t.Fatalf("保存检查点 %s 失败: %v", node, err)
		}
	}

	list, err := saver.List(ctx, "thread-list")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("检查点数量不匹配: 期望 3, 实际 %d", len(list))
	}

	// 验证顺序
	expectedNodes := []string{"node_a", "node_b", "node_c"}
	for i, cp := range list {
		if cp.CurrentNode != expectedNodes[i] {
			t.Errorf("第 %d 个检查点 CurrentNode 不匹配: 期望 %s, 实际 %s", i, expectedNodes[i], cp.CurrentNode)
		}
	}
}

// testSaverDelete 测试删除功能
func testSaverDelete(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	cp1 := newTestCheckpoint("thread-del", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-del", "test-graph", "node_b")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)

	// 删除第一个
	if err := saver.Delete(ctx, cp1.ID); err != nil {
		t.Fatalf("删除检查点失败: %v", err)
	}

	// 验证已删除
	_, err := saver.LoadByID(ctx, cp1.ID)
	if err == nil {
		t.Fatal("已删除的检查点不应被找到")
	}

	// 第二个仍然存在
	loaded, err := saver.LoadByID(ctx, cp2.ID)
	if err != nil {
		t.Fatalf("第二个检查点应该仍然存在: %v", err)
	}
	if loaded.CurrentNode != "node_b" {
		t.Errorf("CurrentNode 不匹配: 期望 node_b, 实际 %s", loaded.CurrentNode)
	}

	// 列表只有一个
	list, err := saver.List(ctx, "thread-del")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("检查点数量不匹配: 期望 1, 实际 %d", len(list))
	}
}

// testSaverDeleteThread 测试删除线程功能
func testSaverDeleteThread(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 保存到两个线程
	cp1 := newTestCheckpoint("thread-A", "test-graph", "node_a")
	cp2 := newTestCheckpoint("thread-A", "test-graph", "node_b")
	cp3 := newTestCheckpoint("thread-B", "test-graph", "node_x")
	_ = saver.Save(ctx, cp1)
	_ = saver.Save(ctx, cp2)
	_ = saver.Save(ctx, cp3)

	// 删除 thread-A
	if err := saver.DeleteThread(ctx, "thread-A"); err != nil {
		t.Fatalf("删除线程检查点失败: %v", err)
	}

	// thread-A 不应有检查点
	_, err := saver.Load(ctx, "thread-A")
	if err == nil {
		t.Fatal("thread-A 的检查点应该全部被删除")
	}

	// thread-B 仍然存在
	loaded, err := saver.Load(ctx, "thread-B")
	if err != nil {
		t.Fatalf("thread-B 的检查点应该仍然存在: %v", err)
	}
	if loaded.CurrentNode != "node_x" {
		t.Errorf("CurrentNode 不匹配: 期望 node_x, 实际 %s", loaded.CurrentNode)
	}
}

// testSaverNotFound 测试不存在情况的处理
func testSaverNotFound(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 加载不存在的线程
	_, err := saver.Load(ctx, "nonexistent-thread")
	if err == nil {
		t.Fatal("加载不存在的线程应该返回错误")
	}

	// 加载不存在的 ID
	_, err = saver.LoadByID(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("加载不存在的检查点应该返回错误")
	}

	// 删除不存在的检查点不应报错
	err = saver.Delete(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("删除不存在的检查点不应返回错误: %v", err)
	}

	// 删除不存在的线程不应报错
	err = saver.DeleteThread(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("删除不存在的线程不应返回错误: %v", err)
	}

	// 列出不存在的线程应返回空列表
	list, err := saver.List(ctx, "nonexistent-thread")
	if err != nil {
		t.Fatalf("列出不存在线程的检查点不应返回错误: %v", err)
	}
	if list != nil && len(list) > 0 {
		t.Errorf("不存在的线程应返回空列表, 实际 %d 个", len(list))
	}
}

// testSaverMultipleThreads 测试多线程隔离
func testSaverMultipleThreads(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 保存到多个线程
	threads := []string{"thread-alpha", "thread-beta", "thread-gamma"}
	for i, threadID := range threads {
		for j := 0; j < 3; j++ {
			cp := newTestCheckpoint(threadID, "multi-graph", fmt.Sprintf("node_%d_%d", i, j))
			if err := saver.Save(ctx, cp); err != nil {
				t.Fatalf("保存检查点失败: %v", err)
			}
		}
	}

	// 验证各线程的检查点数量
	for _, threadID := range threads {
		list, err := saver.List(ctx, threadID)
		if err != nil {
			t.Fatalf("列出 %s 的检查点失败: %v", threadID, err)
		}
		if len(list) != 3 {
			t.Errorf("%s 的检查点数量不匹配: 期望 3, 实际 %d", threadID, len(list))
		}
	}

	// 删除一个线程不影响其他线程
	if err := saver.DeleteThread(ctx, "thread-beta"); err != nil {
		t.Fatalf("删除线程检查点失败: %v", err)
	}

	// thread-beta 已被删除
	list, err := saver.List(ctx, "thread-beta")
	if err != nil {
		t.Fatalf("列出检查点失败: %v", err)
	}
	if list != nil && len(list) > 0 {
		t.Errorf("thread-beta 应该已被删除, 但仍有 %d 个检查点", len(list))
	}

	// 其他线程不受影响
	for _, threadID := range []string{"thread-alpha", "thread-gamma"} {
		list, err := saver.List(ctx, threadID)
		if err != nil {
			t.Fatalf("列出 %s 的检查点失败: %v", threadID, err)
		}
		if len(list) != 3 {
			t.Errorf("%s 的检查点数量不匹配: 期望 3, 实际 %d", threadID, len(list))
		}
	}
}

// testSaverStatePreservation 测试 State (json.RawMessage) 字段的完整性
func testSaverStatePreservation(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 构造复杂的 State 数据
	complexState := map[string]any{
		"string_field": "你好世界",
		"int_field":    42,
		"float_field":  3.14,
		"bool_field":   true,
		"null_field":   nil,
		"array_field":  []any{1, "two", 3.0},
		"nested_object": map[string]any{
			"deep_key": "deep_value",
			"deep_array": []any{
				map[string]any{"id": 1},
				map[string]any{"id": 2},
			},
		},
	}
	stateJSON, _ := json.Marshal(complexState)

	cp := &Checkpoint{
		ThreadID:       "thread-state",
		GraphName:      "state-graph",
		CurrentNode:    "node_state",
		State:          json.RawMessage(stateJSON),
		PendingNodes:   []string{},
		CompletedNodes: []string{"start"},
	}
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	loaded, err := saver.Load(ctx, "thread-state")
	if err != nil {
		t.Fatalf("加载检查点失败: %v", err)
	}

	// 解析加载的 State 并验证
	var loadedState map[string]any
	if err := json.Unmarshal(loaded.State, &loadedState); err != nil {
		t.Fatalf("解析加载的 State 失败: %v", err)
	}

	if loadedState["string_field"] != "你好世界" {
		t.Errorf("string_field 不匹配: 期望 '你好世界', 实际 '%v'", loadedState["string_field"])
	}
	if loadedState["bool_field"] != true {
		t.Errorf("bool_field 不匹配: 期望 true, 实际 %v", loadedState["bool_field"])
	}
	if loadedState["null_field"] != nil {
		t.Errorf("null_field 不匹配: 期望 nil, 实际 %v", loadedState["null_field"])
	}

	// 验证嵌套对象
	nested, ok := loadedState["nested_object"].(map[string]any)
	if !ok {
		t.Fatal("nested_object 类型不正确")
	}
	if nested["deep_key"] != "deep_value" {
		t.Errorf("nested.deep_key 不匹配: 期望 'deep_value', 实际 '%v'", nested["deep_key"])
	}
}

// testSaverTimestampBehavior 测试时间戳行为
func testSaverTimestampBehavior(t *testing.T, saver CheckpointSaver) {
	t.Helper()
	ctx := context.Background()

	// 测试自动设置时间戳
	cp := newTestCheckpoint("thread-time", "time-graph", "node_time")
	beforeSave := time.Now()
	if err := saver.Save(ctx, cp); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}
	afterSave := time.Now()

	if cp.CreatedAt.Before(beforeSave.Add(-time.Second)) || cp.CreatedAt.After(afterSave.Add(time.Second)) {
		t.Errorf("CreatedAt 超出预期范围: %v 不在 [%v, %v] 之间", cp.CreatedAt, beforeSave, afterSave)
	}
	if cp.UpdatedAt.Before(beforeSave.Add(-time.Second)) || cp.UpdatedAt.After(afterSave.Add(time.Second)) {
		t.Errorf("UpdatedAt 超出预期范围: %v 不在 [%v, %v] 之间", cp.UpdatedAt, beforeSave, afterSave)
	}

	// 测试预设 CreatedAt 时不被覆盖
	presetTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	cp2 := newTestCheckpoint("thread-time", "time-graph", "node_time2")
	cp2.CreatedAt = presetTime
	if err := saver.Save(ctx, cp2); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}
	if !cp2.CreatedAt.Equal(presetTime) {
		t.Errorf("预设的 CreatedAt 被覆盖: 期望 %v, 实际 %v", presetTime, cp2.CreatedAt)
	}
}
