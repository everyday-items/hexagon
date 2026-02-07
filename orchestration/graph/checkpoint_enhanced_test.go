package graph

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// ============== 辅助函数 ==============

// newTestEnhancedCheckpoint 创建一个用于测试的增强检查点
// 包含完整的字段数据，便于验证各个方法的正确性
func newTestEnhancedCheckpoint(threadID, graphName, currentNode string, status CheckpointStatus) *EnhancedCheckpoint {
	state := map[string]any{
		"counter": 42,
		"message": "增强检查点测试状态",
	}
	stateJSON, _ := json.Marshal(state)

	return &EnhancedCheckpoint{
		ThreadID:       threadID,
		GraphName:      graphName,
		Version:        CheckpointVersion{Major: 1, Minor: 0, Patch: 0},
		Status:         status,
		CurrentNode:    currentNode,
		PendingNodes:   []string{"node_b", "node_c"},
		CompletedNodes: []string{"node_a"},
		State:          json.RawMessage(stateJSON),
		Metadata: map[string]any{
			"step":    1,
			"trigger": "manual",
		},
	}
}

// ============== MemoryEnhancedCheckpointSaver 测试 ==============

// TestMemoryEnhancedCheckpointSaver_SaveAndLoad 测试增强检查点的基本保存和加载功能
// 验证：自动生成 ID、时间戳设置、状态哈希计算、字段完整性
func TestMemoryEnhancedCheckpointSaver_SaveAndLoad(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 创建并保存增强检查点
	cp := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusRunning)
	err := saver.SaveEnhanced(ctx, cp)
	if err != nil {
		t.Fatalf("保存增强检查点失败: %v", err)
	}

	// 验证 ID 已自动生成
	if cp.ID == "" {
		t.Fatal("增强检查点 ID 未自动生成")
	}

	// 验证时间戳已设置
	if cp.CreatedAt.IsZero() {
		t.Fatal("CreatedAt 未设置")
	}
	if cp.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt 未设置")
	}

	// 验证状态哈希已计算
	if cp.StateHash == "" {
		t.Fatal("StateHash 未计算")
	}

	// 加载最新的增强检查点
	loaded, err := saver.LoadEnhanced(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载增强检查点失败: %v", err)
	}

	// 验证所有关键字段
	if loaded.ID != cp.ID {
		t.Errorf("ID 不匹配: 期望 %s, 实际 %s", cp.ID, loaded.ID)
	}
	if loaded.ThreadID != "thread-1" {
		t.Errorf("ThreadID 不匹配: 期望 thread-1, 实际 %s", loaded.ThreadID)
	}
	if loaded.GraphName != "test-graph" {
		t.Errorf("GraphName 不匹配: 期望 test-graph, 实际 %s", loaded.GraphName)
	}
	if loaded.Status != CheckpointStatusRunning {
		t.Errorf("Status 不匹配: 期望 %s, 实际 %s", CheckpointStatusRunning, loaded.Status)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}
	if loaded.Version.Major != 1 || loaded.Version.Minor != 0 || loaded.Version.Patch != 0 {
		t.Errorf("Version 不匹配: 期望 1.0.0, 实际 %s", loaded.Version.String())
	}
	if len(loaded.PendingNodes) != 2 {
		t.Errorf("PendingNodes 长度不匹配: 期望 2, 实际 %d", len(loaded.PendingNodes))
	}
	if len(loaded.CompletedNodes) != 1 {
		t.Errorf("CompletedNodes 长度不匹配: 期望 1, 实际 %d", len(loaded.CompletedNodes))
	}

	// 验证 State 字段的内容
	var stateMap map[string]any
	if err := json.Unmarshal(loaded.State, &stateMap); err != nil {
		t.Fatalf("解析 State 失败: %v", err)
	}
	if stateMap["message"] != "增强检查点测试状态" {
		t.Errorf("State.message 不匹配: 期望 '增强检查点测试状态', 实际 '%v'", stateMap["message"])
	}

	// 测试多个检查点保存后加载最新的
	cp2 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_b", CheckpointStatusCompleted)
	err = saver.SaveEnhanced(ctx, cp2)
	if err != nil {
		t.Fatalf("保存第二个增强检查点失败: %v", err)
	}

	latest, err := saver.LoadEnhanced(ctx, "thread-1")
	if err != nil {
		t.Fatalf("加载最新增强检查点失败: %v", err)
	}
	if latest.CurrentNode != "node_b" {
		t.Errorf("期望最新检查点的 CurrentNode 为 node_b, 实际 %s", latest.CurrentNode)
	}

	// 测试加载不存在的线程
	_, err = saver.LoadEnhanced(ctx, "nonexistent-thread")
	if err == nil {
		t.Fatal("加载不存在的线程应该返回错误")
	}
}

// TestMemoryEnhancedCheckpointSaver_SaveWithParent 测试带有父检查点关系的保存
// 验证：父检查点的 ChildIDs 会被正确更新
func TestMemoryEnhancedCheckpointSaver_SaveWithParent(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 保存父检查点
	parent := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	err := saver.SaveEnhanced(ctx, parent)
	if err != nil {
		t.Fatalf("保存父检查点失败: %v", err)
	}

	// 保存子检查点，指定父 ID
	child := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_b", CheckpointStatusRunning)
	child.ParentID = parent.ID
	err = saver.SaveEnhanced(ctx, child)
	if err != nil {
		t.Fatalf("保存子检查点失败: %v", err)
	}

	// 验证父检查点的 ChildIDs 已更新
	loadedParent, err := saver.LoadEnhancedByID(ctx, parent.ID)
	if err != nil {
		t.Fatalf("加载父检查点失败: %v", err)
	}
	if len(loadedParent.ChildIDs) != 1 {
		t.Fatalf("父检查点 ChildIDs 长度不匹配: 期望 1, 实际 %d", len(loadedParent.ChildIDs))
	}
	if loadedParent.ChildIDs[0] != child.ID {
		t.Errorf("父检查点 ChildIDs[0] 不匹配: 期望 %s, 实际 %s", child.ID, loadedParent.ChildIDs[0])
	}
}

// TestMemoryEnhancedCheckpointSaver_LoadByID 测试根据 ID 加载增强检查点
// 验证：能够准确按 ID 获取特定检查点，不存在时返回错误
func TestMemoryEnhancedCheckpointSaver_LoadByID(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 保存多个检查点
	cp1 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	cp2 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_b", CheckpointStatusRunning)
	cp3 := newTestEnhancedCheckpoint("thread-2", "other-graph", "node_x", CheckpointStatusPending)
	_ = saver.SaveEnhanced(ctx, cp1)
	_ = saver.SaveEnhanced(ctx, cp2)
	_ = saver.SaveEnhanced(ctx, cp3)

	// 通过 ID 加载第一个检查点
	loaded, err := saver.LoadEnhancedByID(ctx, cp1.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_a" {
		t.Errorf("CurrentNode 不匹配: 期望 node_a, 实际 %s", loaded.CurrentNode)
	}
	if loaded.Status != CheckpointStatusCompleted {
		t.Errorf("Status 不匹配: 期望 %s, 实际 %s", CheckpointStatusCompleted, loaded.Status)
	}

	// 通过 ID 加载第三个检查点（不同线程）
	loaded, err = saver.LoadEnhancedByID(ctx, cp3.ID)
	if err != nil {
		t.Fatalf("根据 ID 加载检查点失败: %v", err)
	}
	if loaded.CurrentNode != "node_x" {
		t.Errorf("CurrentNode 不匹配: 期望 node_x, 实际 %s", loaded.CurrentNode)
	}
	if loaded.GraphName != "other-graph" {
		t.Errorf("GraphName 不匹配: 期望 other-graph, 实际 %s", loaded.GraphName)
	}

	// 加载不存在的 ID
	_, err = saver.LoadEnhancedByID(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("加载不存在的检查点应该返回错误")
	}
}

// TestMemoryEnhancedCheckpointSaver_List 测试列出增强检查点
// 验证：基本列出、状态过滤、分支过滤、时间范围过滤、分页功能
func TestMemoryEnhancedCheckpointSaver_List(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 保存多个状态不同的检查点
	cp1 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	cp2 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_b", CheckpointStatusRunning)
	cp3 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_c", CheckpointStatusFailed)
	cp4 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_d", CheckpointStatusCompleted)
	cp4.BranchID = "branch-1"
	_ = saver.SaveEnhanced(ctx, cp1)
	_ = saver.SaveEnhanced(ctx, cp2)
	_ = saver.SaveEnhanced(ctx, cp3)
	_ = saver.SaveEnhanced(ctx, cp4)

	// 不带过滤条件列出所有
	all, err := saver.ListEnhanced(ctx, "thread-1", nil)
	if err != nil {
		t.Fatalf("列出增强检查点失败: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("检查点数量不匹配: 期望 4, 实际 %d", len(all))
	}

	// 按状态过滤
	opts := &ListOptions{Status: CheckpointStatusCompleted}
	completed, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("按状态过滤检查点失败: %v", err)
	}
	if len(completed) != 2 {
		t.Errorf("已完成检查点数量不匹配: 期望 2, 实际 %d", len(completed))
	}

	// 按分支过滤
	opts = &ListOptions{BranchID: "branch-1"}
	branched, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("按分支过滤检查点失败: %v", err)
	}
	if len(branched) != 1 {
		t.Errorf("分支检查点数量不匹配: 期望 1, 实际 %d", len(branched))
	}

	// 分页测试
	opts = &ListOptions{Limit: 2, Offset: 0}
	page1, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("分页列出检查点失败: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("第一页检查点数量不匹配: 期望 2, 实际 %d", len(page1))
	}

	opts = &ListOptions{Limit: 2, Offset: 2}
	page2, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("分页列出检查点失败: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("第二页检查点数量不匹配: 期望 2, 实际 %d", len(page2))
	}

	// 超出范围的偏移量应返回空
	opts = &ListOptions{Limit: 10, Offset: 100}
	empty, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("超出范围分页不应返回错误: %v", err)
	}
	if empty != nil {
		t.Errorf("超出范围的偏移量应返回 nil, 实际 %d 个", len(empty))
	}

	// 列出不存在的线程
	none, err := saver.ListEnhanced(ctx, "nonexistent-thread", nil)
	if err != nil {
		t.Fatalf("列出不存在线程不应返回错误: %v", err)
	}
	if none != nil {
		t.Errorf("不存在的线程应返回 nil, 实际 %d 个", len(none))
	}
}

// TestMemoryEnhancedCheckpointSaver_ListWithTimeFilter 测试带时间范围过滤的列表功能
func TestMemoryEnhancedCheckpointSaver_ListWithTimeFilter(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 手动设定 CreatedAt 来控制时间
	now := time.Now()
	cp1 := newTestEnhancedCheckpoint("thread-1", "test-graph", "old_node", CheckpointStatusCompleted)
	cp1.CreatedAt = now.Add(-2 * time.Hour)
	_ = saver.SaveEnhanced(ctx, cp1)

	cp2 := newTestEnhancedCheckpoint("thread-1", "test-graph", "new_node", CheckpointStatusRunning)
	cp2.CreatedAt = now.Add(-30 * time.Minute)
	_ = saver.SaveEnhanced(ctx, cp2)

	// 过滤最近1小时的检查点
	startTime := now.Add(-1 * time.Hour)
	opts := &ListOptions{StartTime: &startTime}
	recent, err := saver.ListEnhanced(ctx, "thread-1", opts)
	if err != nil {
		t.Fatalf("按时间过滤检查点失败: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("最近1小时检查点数量不匹配: 期望 1, 实际 %d", len(recent))
	}
	if len(recent) > 0 && recent[0].CurrentNode != "new_node" {
		t.Errorf("过滤后的检查点 CurrentNode 不匹配: 期望 new_node, 实际 %s", recent[0].CurrentNode)
	}
}

// TestMemoryEnhancedCheckpointSaver_GetHistory 测试获取检查点历史链
// 验证：沿 ParentID 回溯历史，limit 参数限制返回数量
func TestMemoryEnhancedCheckpointSaver_GetHistory(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 构建三级检查点链: cp1 <- cp2 <- cp3
	cp1 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	_ = saver.SaveEnhanced(ctx, cp1)

	cp2 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_b", CheckpointStatusCompleted)
	cp2.ParentID = cp1.ID
	_ = saver.SaveEnhanced(ctx, cp2)

	cp3 := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_c", CheckpointStatusRunning)
	cp3.ParentID = cp2.ID
	_ = saver.SaveEnhanced(ctx, cp3)

	// 从 cp3 获取完整历史（limit=0 表示不限制）
	history, err := saver.GetHistory(ctx, cp3.ID, 0)
	if err != nil {
		t.Fatalf("获取历史链失败: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("历史链长度不匹配: 期望 3, 实际 %d", len(history))
	}

	// 验证顺序：从当前到最早
	if history[0].CurrentNode != "node_c" {
		t.Errorf("历史第1项 CurrentNode 不匹配: 期望 node_c, 实际 %s", history[0].CurrentNode)
	}
	if history[1].CurrentNode != "node_b" {
		t.Errorf("历史第2项 CurrentNode 不匹配: 期望 node_b, 实际 %s", history[1].CurrentNode)
	}
	if history[2].CurrentNode != "node_a" {
		t.Errorf("历史第3项 CurrentNode 不匹配: 期望 node_a, 实际 %s", history[2].CurrentNode)
	}

	// 带 limit 的历史获取
	limited, err := saver.GetHistory(ctx, cp3.ID, 2)
	if err != nil {
		t.Fatalf("获取有限历史链失败: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("有限历史链长度不匹配: 期望 2, 实际 %d", len(limited))
	}

	// 从根节点获取历史，应该只有一项
	rootHistory, err := saver.GetHistory(ctx, cp1.ID, 0)
	if err != nil {
		t.Fatalf("获取根节点历史失败: %v", err)
	}
	if len(rootHistory) != 1 {
		t.Errorf("根节点历史长度不匹配: 期望 1, 实际 %d", len(rootHistory))
	}

	// 获取不存在的检查点历史，应返回空列表
	empty, err := saver.GetHistory(ctx, "nonexistent-id", 0)
	if err != nil {
		t.Fatalf("获取不存在检查点历史不应返回错误: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("不存在检查点历史长度应为 0, 实际 %d", len(empty))
	}
}

// TestMemoryEnhancedCheckpointSaver_CreateBranch 测试从检查点创建分支
// 验证：分支信息创建、新检查点生成、源检查点的 ChildIDs 更新、状态复制
func TestMemoryEnhancedCheckpointSaver_CreateBranch(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 保存一个基础检查点
	base := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	base.Tags = []string{"important"}
	_ = saver.SaveEnhanced(ctx, base)

	// 从基础检查点创建分支
	branchCP, err := saver.CreateBranch(ctx, base.ID, "experimental-branch")
	if err != nil {
		t.Fatalf("创建分支失败: %v", err)
	}

	// 验证分支检查点的属性
	if branchCP.ID == "" {
		t.Fatal("分支检查点 ID 未生成")
	}
	if branchCP.ID == base.ID {
		t.Error("分支检查点 ID 不应与源检查点相同")
	}
	if branchCP.ParentID != base.ID {
		t.Errorf("分支检查点 ParentID 不匹配: 期望 %s, 实际 %s", base.ID, branchCP.ParentID)
	}
	if branchCP.BranchName != "experimental-branch" {
		t.Errorf("分支名称不匹配: 期望 experimental-branch, 实际 %s", branchCP.BranchName)
	}
	if branchCP.BranchID == "" {
		t.Fatal("分支 ID 未生成")
	}
	if branchCP.Status != CheckpointStatusPending {
		t.Errorf("分支检查点状态不匹配: 期望 %s, 实际 %s", CheckpointStatusPending, branchCP.Status)
	}
	if branchCP.ThreadID != base.ThreadID {
		t.Errorf("分支检查点 ThreadID 不匹配: 期望 %s, 实际 %s", base.ThreadID, branchCP.ThreadID)
	}
	if branchCP.GraphName != base.GraphName {
		t.Errorf("分支检查点 GraphName 不匹配: 期望 %s, 实际 %s", base.GraphName, branchCP.GraphName)
	}
	if branchCP.CurrentNode != base.CurrentNode {
		t.Errorf("分支检查点 CurrentNode 不匹配: 期望 %s, 实际 %s", base.CurrentNode, branchCP.CurrentNode)
	}

	// 验证状态被正确复制
	var branchState map[string]any
	if err := json.Unmarshal(branchCP.State, &branchState); err != nil {
		t.Fatalf("解析分支状态失败: %v", err)
	}
	if branchState["message"] != "增强检查点测试状态" {
		t.Errorf("分支状态内容不匹配: 期望 '增强检查点测试状态', 实际 '%v'", branchState["message"])
	}

	// 验证源检查点的 ChildIDs 已更新
	loadedBase, _ := saver.LoadEnhancedByID(ctx, base.ID)
	if len(loadedBase.ChildIDs) != 1 || loadedBase.ChildIDs[0] != branchCP.ID {
		t.Errorf("源检查点 ChildIDs 未正确更新: %v", loadedBase.ChildIDs)
	}

	// 验证分支信息已创建
	branches, err := saver.GetBranches(ctx, "thread-1")
	if err != nil {
		t.Fatalf("获取分支列表失败: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("分支数量不匹配: 期望 1, 实际 %d", len(branches))
	}
	if branches[0].Name != "experimental-branch" {
		t.Errorf("分支名称不匹配: 期望 experimental-branch, 实际 %s", branches[0].Name)
	}
	if branches[0].BaseCheckpointID != base.ID {
		t.Errorf("分支基础检查点 ID 不匹配: 期望 %s, 实际 %s", base.ID, branches[0].BaseCheckpointID)
	}
	if branches[0].CheckpointCount != 1 {
		t.Errorf("分支检查点计数不匹配: 期望 1, 实际 %d", branches[0].CheckpointCount)
	}

	// 从不存在的检查点创建分支应返回错误
	_, err = saver.CreateBranch(ctx, "nonexistent-id", "bad-branch")
	if err == nil {
		t.Fatal("从不存在的检查点创建分支应该返回错误")
	}
}

// TestMemoryEnhancedCheckpointSaver_MergeBranch 测试分支合并功能
// 验证：覆盖策略、保留两者策略、合并后的检查点属性、分支信息更新
func TestMemoryEnhancedCheckpointSaver_MergeBranch(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 准备：创建基础检查点和两个分支
	base := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	_ = saver.SaveEnhanced(ctx, base)

	// 创建源分支
	sourceBranchCP, err := saver.CreateBranch(ctx, base.ID, "source-branch")
	if err != nil {
		t.Fatalf("创建源分支失败: %v", err)
	}
	sourceBranchID := sourceBranchCP.BranchID

	// 在源分支上修改状态
	sourceState := map[string]any{"source_key": "source_value", "counter": 100}
	sourceStateJSON, _ := json.Marshal(sourceState)
	sourceBranchCP.State = json.RawMessage(sourceStateJSON)
	sourceBranchCP.CurrentNode = "node_source_end"
	sourceBranchCP.CompletedNodes = []string{"node_a", "node_source_1"}
	_ = saver.SaveEnhanced(ctx, sourceBranchCP)

	// 创建目标分支
	targetBranchCP, err := saver.CreateBranch(ctx, base.ID, "target-branch")
	if err != nil {
		t.Fatalf("创建目标分支失败: %v", err)
	}
	targetBranchID := targetBranchCP.BranchID

	// 在目标分支上修改状态
	targetState := map[string]any{"target_key": "target_value", "counter": 200}
	targetStateJSON, _ := json.Marshal(targetState)
	targetBranchCP.State = json.RawMessage(targetStateJSON)
	targetBranchCP.CompletedNodes = []string{"node_a", "node_target_1"}
	_ = saver.SaveEnhanced(ctx, targetBranchCP)

	// 测试覆盖策略合并（源覆盖目标）
	merged, err := saver.MergeBranch(ctx, sourceBranchID, targetBranchID, MergeStrategyOverwrite)
	if err != nil {
		t.Fatalf("合并分支失败: %v", err)
	}

	// 验证合并检查点的属性
	if merged.ID == "" {
		t.Fatal("合并检查点 ID 未生成")
	}
	if merged.ParentID != targetBranchCP.ID {
		t.Errorf("合并检查点 ParentID 不匹配: 期望 %s, 实际 %s", targetBranchCP.ID, merged.ParentID)
	}
	if merged.BranchID != targetBranchID {
		t.Errorf("合并检查点 BranchID 不匹配: 期望 %s, 实际 %s", targetBranchID, merged.BranchID)
	}
	if merged.Status != CheckpointStatusPending {
		t.Errorf("合并检查点状态不匹配: 期望 %s, 实际 %s", CheckpointStatusPending, merged.Status)
	}

	// 覆盖策略下，State 应该是源的状态
	var mergedState map[string]any
	if err := json.Unmarshal(merged.State, &mergedState); err != nil {
		t.Fatalf("解析合并状态失败: %v", err)
	}
	if mergedState["source_key"] != "source_value" {
		t.Errorf("覆盖合并后状态不匹配: 期望 source_key=source_value, 实际 %v", mergedState["source_key"])
	}

	// 验证 CompletedNodes 合并去重
	if len(merged.CompletedNodes) < 2 {
		t.Errorf("合并后 CompletedNodes 长度不足: 期望 >= 2, 实际 %d", len(merged.CompletedNodes))
	}

	// 验证元数据中记录了合并信息
	if merged.Metadata["merged_from"] != sourceBranchID {
		t.Errorf("合并元数据 merged_from 不匹配: 期望 %s, 实际 %v", sourceBranchID, merged.Metadata["merged_from"])
	}
	if merged.Metadata["merge_strategy"] != string(MergeStrategyOverwrite) {
		t.Errorf("合并元数据 merge_strategy 不匹配: 期望 %s, 实际 %v", MergeStrategyOverwrite, merged.Metadata["merge_strategy"])
	}

	// 测试不存在的分支合并应返回错误
	_, err = saver.MergeBranch(ctx, "nonexistent-branch", targetBranchID, MergeStrategyOverwrite)
	if err == nil {
		t.Fatal("合并不存在的源分支应该返回错误")
	}

	_, err = saver.MergeBranch(ctx, sourceBranchID, "nonexistent-branch", MergeStrategyOverwrite)
	if err == nil {
		t.Fatal("合并到不存在的目标分支应该返回错误")
	}
}

// TestMemoryEnhancedCheckpointSaver_MergeBranchKeepBoth 测试 KeepBoth 合并策略
// 验证：KeepBoth 策略保留目标分支的状态
func TestMemoryEnhancedCheckpointSaver_MergeBranchKeepBoth(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	base := newTestEnhancedCheckpoint("thread-1", "test-graph", "node_a", CheckpointStatusCompleted)
	_ = saver.SaveEnhanced(ctx, base)

	sourceBranchCP, _ := saver.CreateBranch(ctx, base.ID, "source")
	targetBranchCP, _ := saver.CreateBranch(ctx, base.ID, "target")

	// 修改目标分支状态
	targetState := map[string]any{"target_data": "keep_me"}
	targetStateJSON, _ := json.Marshal(targetState)
	targetBranchCP.State = json.RawMessage(targetStateJSON)
	_ = saver.SaveEnhanced(ctx, targetBranchCP)

	merged, err := saver.MergeBranch(ctx, sourceBranchCP.BranchID, targetBranchCP.BranchID, MergeStrategyKeepBoth)
	if err != nil {
		t.Fatalf("KeepBoth 合并失败: %v", err)
	}

	// KeepBoth 策略应保留目标状态
	var mergedState map[string]any
	if err := json.Unmarshal(merged.State, &mergedState); err != nil {
		t.Fatalf("解析合并状态失败: %v", err)
	}
	if mergedState["target_data"] != "keep_me" {
		t.Errorf("KeepBoth 策略应保留目标状态, 实际 %v", mergedState)
	}
}

// TestMemoryEnhancedCheckpointSaver_Search 测试检查点搜索功能
// 验证：按线程、状态、标签、图名称等多维度搜索，分页功能
func TestMemoryEnhancedCheckpointSaver_Search(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 准备多种检查点数据
	cp1 := newTestEnhancedCheckpoint("thread-1", "graph-a", "node_1", CheckpointStatusCompleted)
	cp1.Tags = []string{"release", "v1"}
	_ = saver.SaveEnhanced(ctx, cp1)

	cp2 := newTestEnhancedCheckpoint("thread-1", "graph-a", "node_2", CheckpointStatusFailed)
	cp2.Tags = []string{"debug"}
	_ = saver.SaveEnhanced(ctx, cp2)

	cp3 := newTestEnhancedCheckpoint("thread-2", "graph-b", "node_3", CheckpointStatusCompleted)
	cp3.Tags = []string{"release", "v2"}
	_ = saver.SaveEnhanced(ctx, cp3)

	cp4 := newTestEnhancedCheckpoint("thread-2", "graph-b", "node_4", CheckpointStatusRunning)
	_ = saver.SaveEnhanced(ctx, cp4)

	// 按线程 ID 搜索
	results, err := saver.Search(ctx, &CheckpointQuery{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("按线程搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("按线程搜索结果数量不匹配: 期望 2, 实际 %d", len(results))
	}

	// 按状态搜索
	results, err = saver.Search(ctx, &CheckpointQuery{Status: CheckpointStatusCompleted})
	if err != nil {
		t.Fatalf("按状态搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("按状态搜索结果数量不匹配: 期望 2, 实际 %d", len(results))
	}

	// 按图名称搜索
	results, err = saver.Search(ctx, &CheckpointQuery{GraphName: "graph-b"})
	if err != nil {
		t.Fatalf("按图名称搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("按图名称搜索结果数量不匹配: 期望 2, 实际 %d", len(results))
	}

	// 按标签搜索
	results, err = saver.Search(ctx, &CheckpointQuery{Tags: []string{"release"}})
	if err != nil {
		t.Fatalf("按标签搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("按标签搜索结果数量不匹配: 期望 2, 实际 %d", len(results))
	}

	// 组合条件搜索：线程 + 状态
	results, err = saver.Search(ctx, &CheckpointQuery{
		ThreadID: "thread-1",
		Status:   CheckpointStatusCompleted,
	})
	if err != nil {
		t.Fatalf("组合搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("组合搜索结果数量不匹配: 期望 1, 实际 %d", len(results))
	}

	// 搜索分页
	results, err = saver.Search(ctx, &CheckpointQuery{Limit: 2})
	if err != nil {
		t.Fatalf("分页搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("分页搜索结果数量不匹配: 期望 2, 实际 %d", len(results))
	}

	// 无匹配结果的搜索
	results, err = saver.Search(ctx, &CheckpointQuery{ThreadID: "nonexistent-thread"})
	if err != nil {
		t.Fatalf("无匹配搜索不应返回错误: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("无匹配搜索结果应为空, 实际 %d 个", len(results))
	}
}

// TestMemoryEnhancedCheckpointSaver_Cleanup 测试清理旧检查点功能
// 验证：基于过期时间清理、KeepCompleted 保护、KeepTagged 保护、KeepBranchHeads 保护
func TestMemoryEnhancedCheckpointSaver_Cleanup(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	now := time.Now()

	// 创建各种类型的旧检查点
	// 旧的已完成检查点
	cpOldCompleted := newTestEnhancedCheckpoint("thread-1", "test-graph", "old_completed", CheckpointStatusCompleted)
	cpOldCompleted.CreatedAt = now.Add(-48 * time.Hour)
	_ = saver.SaveEnhanced(ctx, cpOldCompleted)

	// 旧的失败检查点
	cpOldFailed := newTestEnhancedCheckpoint("thread-1", "test-graph", "old_failed", CheckpointStatusFailed)
	cpOldFailed.CreatedAt = now.Add(-48 * time.Hour)
	_ = saver.SaveEnhanced(ctx, cpOldFailed)

	// 旧的带标签检查点
	cpOldTagged := newTestEnhancedCheckpoint("thread-1", "test-graph", "old_tagged", CheckpointStatusFailed)
	cpOldTagged.CreatedAt = now.Add(-48 * time.Hour)
	cpOldTagged.Tags = []string{"important"}
	_ = saver.SaveEnhanced(ctx, cpOldTagged)

	// 新的检查点（不应被清理）
	cpNew := newTestEnhancedCheckpoint("thread-1", "test-graph", "new_node", CheckpointStatusRunning)
	_ = saver.SaveEnhanced(ctx, cpNew)

	// 场景1：仅按过期时间清理，不保护任何类型
	cleaned, err := saver.Cleanup(ctx, &CleanupPolicy{
		MaxAge: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("清理检查点失败: %v", err)
	}
	if cleaned != 3 {
		t.Errorf("清理数量不匹配: 期望 3, 实际 %d", cleaned)
	}

	// 验证新检查点仍然存在
	remaining, err := saver.ListEnhanced(ctx, "thread-1", nil)
	if err != nil {
		t.Fatalf("列出剩余检查点失败: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("剩余检查点数量不匹配: 期望 1, 实际 %d", len(remaining))
	}
}

// TestMemoryEnhancedCheckpointSaver_CleanupWithProtection 测试带保护策略的清理功能
func TestMemoryEnhancedCheckpointSaver_CleanupWithProtection(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	now := time.Now()

	// 旧的已完成检查点（应被 KeepCompleted 保护）
	cpCompleted := newTestEnhancedCheckpoint("thread-1", "test-graph", "completed", CheckpointStatusCompleted)
	cpCompleted.CreatedAt = now.Add(-48 * time.Hour)
	_ = saver.SaveEnhanced(ctx, cpCompleted)

	// 旧的带标签检查点（应被 KeepTagged 保护）
	cpTagged := newTestEnhancedCheckpoint("thread-1", "test-graph", "tagged", CheckpointStatusFailed)
	cpTagged.CreatedAt = now.Add(-48 * time.Hour)
	cpTagged.Tags = []string{"important"}
	_ = saver.SaveEnhanced(ctx, cpTagged)

	// 旧的无保护检查点（应被清理）
	cpOld := newTestEnhancedCheckpoint("thread-1", "test-graph", "unprotected", CheckpointStatusFailed)
	cpOld.CreatedAt = now.Add(-48 * time.Hour)
	_ = saver.SaveEnhanced(ctx, cpOld)

	// 使用保护策略清理
	cleaned, err := saver.Cleanup(ctx, &CleanupPolicy{
		MaxAge:        24 * time.Hour,
		KeepCompleted: true,
		KeepTagged:    true,
	})
	if err != nil {
		t.Fatalf("带保护策略清理失败: %v", err)
	}
	// 只有无保护的旧检查点被清理
	if cleaned != 1 {
		t.Errorf("清理数量不匹配: 期望 1, 实际 %d", cleaned)
	}

	// 验证被保护的检查点仍然存在
	loadedCompleted, err := saver.LoadEnhancedByID(ctx, cpCompleted.ID)
	if err != nil {
		t.Fatalf("已完成检查点应被保护: %v", err)
	}
	if loadedCompleted.CurrentNode != "completed" {
		t.Errorf("已完成检查点 CurrentNode 不匹配: 期望 completed, 实际 %s", loadedCompleted.CurrentNode)
	}

	loadedTagged, err := saver.LoadEnhancedByID(ctx, cpTagged.ID)
	if err != nil {
		t.Fatalf("带标签检查点应被保护: %v", err)
	}
	if loadedTagged.CurrentNode != "tagged" {
		t.Errorf("带标签检查点 CurrentNode 不匹配: 期望 tagged, 实际 %s", loadedTagged.CurrentNode)
	}

	// 被清理的检查点不应存在
	_, err = saver.LoadEnhancedByID(ctx, cpOld.ID)
	if err == nil {
		t.Fatal("无保护的旧检查点应该已被清理")
	}
}

// TestMemoryEnhancedCheckpointSaver_CleanupKeepBranchHeads 测试分支头保护
func TestMemoryEnhancedCheckpointSaver_CleanupKeepBranchHeads(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	now := time.Now()

	// 创建基础检查点
	base := newTestEnhancedCheckpoint("thread-1", "test-graph", "base", CheckpointStatusCompleted)
	base.CreatedAt = now.Add(-48 * time.Hour)
	_ = saver.SaveEnhanced(ctx, base)

	// 从基础检查点创建分支（分支头检查点也是旧的）
	branchCP, err := saver.CreateBranch(ctx, base.ID, "my-branch")
	if err != nil {
		t.Fatalf("创建分支失败: %v", err)
	}
	// 手动设置分支头检查点为旧时间
	branchCP.CreatedAt = now.Add(-48 * time.Hour)

	// 使用 KeepBranchHeads 策略清理
	cleaned, err := saver.Cleanup(ctx, &CleanupPolicy{
		MaxAge:          24 * time.Hour,
		KeepBranchHeads: true,
	})
	if err != nil {
		t.Fatalf("带分支头保护清理失败: %v", err)
	}

	// 分支头不应被清理，但基础检查点（非分支头）应被清理
	// 注意：base 是 completed 但没有 KeepCompleted 保护
	if cleaned < 1 {
		t.Errorf("至少应清理 1 个检查点, 实际清理 %d 个", cleaned)
	}

	// 分支头检查点应该仍然存在
	_, err = saver.LoadEnhancedByID(ctx, branchCP.ID)
	if err != nil {
		t.Errorf("分支头检查点应被保护不被清理: %v", err)
	}
}

// TestMemoryEnhancedCheckpointSaver_CleanupNothingToClean 测试无需清理的场景
func TestMemoryEnhancedCheckpointSaver_CleanupNothingToClean(t *testing.T) {
	saver := NewMemoryEnhancedCheckpointSaver()
	ctx := context.Background()

	// 仅保存新的检查点
	cp := newTestEnhancedCheckpoint("thread-1", "test-graph", "new_node", CheckpointStatusRunning)
	_ = saver.SaveEnhanced(ctx, cp)

	// 清理，不应删除任何检查点
	cleaned, err := saver.Cleanup(ctx, &CleanupPolicy{
		MaxAge: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("清理不应返回错误: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("无需清理时应返回 0, 实际 %d", cleaned)
	}
}
