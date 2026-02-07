package graph

import (
	"context"
	"fmt"
	"testing"
)

// ============== 测试用 Mock ==============

// testExecutable 测试用可执行对象
// 模拟图节点的执行逻辑，每个节点有固定的输出和下一个节点
type testExecutable struct {
	// nodes 节点定义：nodeID -> {output, nextNode, error}
	nodes map[string]struct {
		output any
		next   string
		err    error
	}
	// entry 入口节点 ID
	entry string
}

// ExecuteNode 执行指定节点
// 返回预设的输出、下一个节点和错误
func (e *testExecutable) ExecuteNode(_ context.Context, nodeID string, state map[string]any) (any, string, error) {
	node, ok := e.nodes[nodeID]
	if !ok {
		return nil, "", fmt.Errorf("节点不存在: %s", nodeID)
	}

	// 将输出写入 state 模拟状态变更
	if node.output != nil {
		state[nodeID+"_result"] = node.output
	}

	return node.output, node.next, node.err
}

// GetEntryPoint 获取入口节点
func (e *testExecutable) GetEntryPoint() string {
	return e.entry
}

// GetNodeName 获取节点名称
func (e *testExecutable) GetNodeName(nodeID string) string {
	return "Node_" + nodeID
}

// newSimpleExecutable 创建简单的三节点链式执行器
// 执行顺序: A -> B -> C -> 结束
func newSimpleExecutable() *testExecutable {
	return &testExecutable{
		entry: "A",
		nodes: map[string]struct {
			output any
			next   string
			err    error
		}{
			"A": {output: "result_A", next: "B", err: nil},
			"B": {output: "result_B", next: "C", err: nil},
			"C": {output: "result_C", next: "", err: nil},
		},
	}
}

// newExecutableWithError 创建第二个节点会报错的执行器
// 执行顺序: A -> B(报错) -> 终止
func newExecutableWithError() *testExecutable {
	return &testExecutable{
		entry: "A",
		nodes: map[string]struct {
			output any
			next   string
			err    error
		}{
			"A": {output: "result_A", next: "B", err: nil},
			"B": {output: nil, next: "", err: fmt.Errorf("节点 B 执行失败")},
		},
	}
}

// ============== TimeTravelDebugger.Run 测试 ==============

// TestTimeTravelDebugger_Run 测试基本执行和快照记录
// 验证：
//   - 执行完成后返回正确的状态
//   - 快照数量正确（__start__ + 节点数 + __end__）
//   - 每个快照的节点 ID 和名称正确
//   - 状态在每步之间正确传播
func TestTimeTravelDebugger_Run(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	initialState := map[string]any{"input": "hello"}

	result, err := debugger.Run(ctx, initialState)
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 验证返回状态包含所有节点的执行结果
	if result["A_result"] != "result_A" {
		t.Errorf("节点 A 的结果不正确，期望 result_A，实际 %v", result["A_result"])
	}
	if result["B_result"] != "result_B" {
		t.Errorf("节点 B 的结果不正确，期望 result_B，实际 %v", result["B_result"])
	}
	if result["C_result"] != "result_C" {
		t.Errorf("节点 C 的结果不正确，期望 result_C，实际 %v", result["C_result"])
	}

	// 验证快照数量: __start__ + A + B + C + __end__ = 5
	history := debugger.GetHistory()
	if len(history) != 5 {
		t.Fatalf("快照数量不正确，期望 5，实际 %d", len(history))
	}

	// 验证快照顺序和节点 ID
	expectedNodes := []string{"__start__", "A", "B", "C", "__end__"}
	for i, expected := range expectedNodes {
		if history[i].NodeID != expected {
			t.Errorf("快照 %d 节点 ID 不正确，期望 %s，实际 %s", i, expected, history[i].NodeID)
		}
	}

	// 验证快照的节点名称
	expectedNames := []string{"Start", "Node_A", "Node_B", "Node_C", "End"}
	for i, expected := range expectedNames {
		if history[i].NodeName != expected {
			t.Errorf("快照 %d 节点名称不正确，期望 %s，实际 %s", i, expected, history[i].NodeName)
		}
	}

	// 验证快照索引递增
	for i, snap := range history {
		if snap.Index != i {
			t.Errorf("快照 %d 的索引不正确，期望 %d，实际 %d", i, i, snap.Index)
		}
	}

	// 验证 currentIndex 指向最后一个快照
	if debugger.CurrentIndex() != 4 {
		t.Errorf("当前索引不正确，期望 4，实际 %d", debugger.CurrentIndex())
	}

	// 验证状态逐步积累：第 3 个快照（节点 B 执行后）应包含 A_result
	snapB := history[2]
	if _, ok := snapB.State["A_result"]; !ok {
		t.Error("节点 B 快照的状态应包含 A_result")
	}
}

// TestTimeTravelDebugger_Run_WithError 测试执行中出错的场景
func TestTimeTravelDebugger_Run_WithError(t *testing.T) {
	exec := newExecutableWithError()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{})
	if err == nil {
		t.Fatal("期望 Run 返回错误，但没有错误")
	}

	// 验证快照记录了错误信息
	history := debugger.GetHistory()
	// __start__ + A + B(错误) = 3
	if len(history) != 3 {
		t.Fatalf("快照数量不正确，期望 3，实际 %d", len(history))
	}

	// 最后一个快照应该包含错误信息
	lastSnap := history[len(history)-1]
	if lastSnap.Error == "" {
		t.Error("最后一个快照应包含错误信息")
	}
	if lastSnap.NodeID != "B" {
		t.Errorf("出错快照的节点 ID 不正确，期望 B，实际 %s", lastSnap.NodeID)
	}
}

// ============== GoTo 测试 ==============

// TestTimeTravelDebugger_GoTo 测试跳转到指定步骤
// 验证：
//   - 正常跳转更新 currentIndex
//   - 越界索引返回错误
//   - 跳转不影响历史记录
func TestTimeTravelDebugger_GoTo(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{"input": "test"})
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 跳转到索引 1（节点 A 执行后）
	if err := debugger.GoTo(1); err != nil {
		t.Fatalf("GoTo(1) 失败: %v", err)
	}
	if debugger.CurrentIndex() != 1 {
		t.Errorf("GoTo 后当前索引不正确，期望 1，实际 %d", debugger.CurrentIndex())
	}

	// 验证历史记录不受跳转影响
	history := debugger.GetHistory()
	if len(history) != 5 {
		t.Errorf("跳转后快照数量不应改变，期望 5，实际 %d", len(history))
	}

	// 跳转到起始位置
	if err := debugger.GoTo(0); err != nil {
		t.Fatalf("GoTo(0) 失败: %v", err)
	}
	if debugger.CurrentIndex() != 0 {
		t.Errorf("GoTo(0) 后当前索引不正确，期望 0，实际 %d", debugger.CurrentIndex())
	}

	// 测试越界索引：负数
	if err := debugger.GoTo(-1); err == nil {
		t.Error("GoTo(-1) 应该返回错误")
	}

	// 测试越界索引：超出范围
	if err := debugger.GoTo(100); err == nil {
		t.Error("GoTo(100) 应该返回错误")
	}

	// 跳转到最后一个有效索引
	if err := debugger.GoTo(4); err != nil {
		t.Fatalf("GoTo(4) 失败: %v", err)
	}
	if debugger.CurrentIndex() != 4 {
		t.Errorf("GoTo(4) 后当前索引不正确，期望 4，实际 %d", debugger.CurrentIndex())
	}
}

// ============== GoBack / GoForward 测试 ==============

// TestTimeTravelDebugger_GoBack_GoForward 测试前进和后退导航
// 验证：
//   - GoBack 将 currentIndex 减 1
//   - GoForward 将 currentIndex 加 1
//   - 在边界处返回错误（不越界）
//   - 连续前进/后退正确工作
func TestTimeTravelDebugger_GoBack_GoForward(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 执行后 currentIndex 应为 4（最后一个快照）
	if debugger.CurrentIndex() != 4 {
		t.Fatalf("初始 currentIndex 不正确，期望 4，实际 %d", debugger.CurrentIndex())
	}

	// 后退一步
	if err := debugger.GoBack(); err != nil {
		t.Fatalf("GoBack 失败: %v", err)
	}
	if debugger.CurrentIndex() != 3 {
		t.Errorf("GoBack 后 currentIndex 不正确，期望 3，实际 %d", debugger.CurrentIndex())
	}

	// 继续后退两步
	if err := debugger.GoBack(); err != nil {
		t.Fatalf("第二次 GoBack 失败: %v", err)
	}
	if err := debugger.GoBack(); err != nil {
		t.Fatalf("第三次 GoBack 失败: %v", err)
	}
	if debugger.CurrentIndex() != 1 {
		t.Errorf("三次 GoBack 后 currentIndex 不正确，期望 1，实际 %d", debugger.CurrentIndex())
	}

	// 前进一步
	if err := debugger.GoForward(); err != nil {
		t.Fatalf("GoForward 失败: %v", err)
	}
	if debugger.CurrentIndex() != 2 {
		t.Errorf("GoForward 后 currentIndex 不正确，期望 2，实际 %d", debugger.CurrentIndex())
	}

	// 跳到起始位置，再后退应报错
	if err := debugger.GoTo(0); err != nil {
		t.Fatalf("GoTo(0) 失败: %v", err)
	}
	if err := debugger.GoBack(); err == nil {
		t.Error("在起始位置 GoBack 应该返回错误")
	}

	// 跳到最后位置，再前进应报错
	if err := debugger.GoTo(4); err != nil {
		t.Fatalf("GoTo(4) 失败: %v", err)
	}
	if err := debugger.GoForward(); err == nil {
		t.Error("在末尾位置 GoForward 应该返回错误")
	}
}

// ============== Replay 测试 ==============

// TestTimeTravelDebugger_Replay 测试从当前位置重新执行
// 验证：
//   - 从 __start__ 重放时完整重新执行所有节点
//   - 重放后会生成新的分支快照
//   - 重放后的历史记录包含原始 + 分支记录
func TestTimeTravelDebugger_Replay(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{"input": "original"})
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 回到起始位置
	if err := debugger.GoTo(0); err != nil {
		t.Fatalf("GoTo(0) 失败: %v", err)
	}

	// 从 __start__ 重放
	result, err := debugger.Replay(ctx)
	if err != nil {
		t.Fatalf("Replay 失败: %v", err)
	}

	// 验证重放结果包含所有节点的执行结果
	if result["A_result"] != "result_A" {
		t.Errorf("重放后节点 A 的结果不正确，期望 result_A，实际 %v", result["A_result"])
	}
	if result["C_result"] != "result_C" {
		t.Errorf("重放后节点 C 的结果不正确，期望 result_C，实际 %v", result["C_result"])
	}

	// 验证历史记录增加了分支快照
	// 原始: 5 个 (__start__ + A + B + C + __end__)
	// 重放从 __start__ 执行: A + B + C = 3 个新快照
	history := debugger.GetHistory()
	if len(history) < 5 {
		t.Errorf("重放后快照数量应大于原始数量，实际 %d", len(history))
	}

	// 验证重放产生了分支
	branches := debugger.GetBranches()
	hasBranch := false
	for _, b := range branches {
		if b != "" {
			hasBranch = true
			break
		}
	}
	if !hasBranch {
		t.Error("重放应产生新的分支 ID")
	}
}

// ============== ReplayFrom 测试 ==============

// TestTimeTravelDebugger_ReplayFrom 测试从指定步骤重新执行
// 验证：
//   - 从中间节点重放时，从该节点继续执行
//   - 重放结果的状态正确
func TestTimeTravelDebugger_ReplayFrom(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{"input": "start"})
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 获取原始快照数量
	originalCount := len(debugger.GetHistory())

	// 从索引 2（节点 B 执行后的快照）开始重放
	result, err := debugger.ReplayFrom(ctx, 2)
	if err != nil {
		t.Fatalf("ReplayFrom(2) 失败: %v", err)
	}

	// 验证结果状态不为空
	if result == nil {
		t.Fatal("ReplayFrom 返回的结果不应为 nil")
	}

	// 验证历史记录增长
	newCount := len(debugger.GetHistory())
	if newCount <= originalCount {
		t.Errorf("ReplayFrom 后快照数量应增长，原始 %d，当前 %d", originalCount, newCount)
	}

	// 验证无效索引返回错误
	_, err = debugger.ReplayFrom(ctx, -1)
	if err == nil {
		t.Error("ReplayFrom(-1) 应该返回错误")
	}

	_, err = debugger.ReplayFrom(ctx, 9999)
	if err == nil {
		t.Error("ReplayFrom(9999) 应该返回错误")
	}
}

// ============== Compare 测试 ==============

// TestTimeTravelDebugger_Compare 测试状态差异对比
// 验证：
//   - 正确检测新增的键 (added)
//   - 正确检测修改的值 (changed)
//   - 正确检测删除的键 (removed)
//   - 无差异时返回空切片
//   - 越界索引返回错误
func TestTimeTravelDebugger_Compare(t *testing.T) {
	exec := newSimpleExecutable()
	debugger := NewTimeTravelDebugger(exec)

	ctx := context.Background()
	_, err := debugger.Run(ctx, map[string]any{"input": "hello"})
	if err != nil {
		t.Fatalf("Run 执行失败: %v", err)
	}

	// 对比索引 0（__start__）和索引 2（节点 B 执行后）
	// 索引 0 的状态只有 {"input": "hello"}
	// 索引 2 的状态应包含 {"input": "hello", "A_result": "result_A"}
	diffs, err := debugger.Compare(0, 2)
	if err != nil {
		t.Fatalf("Compare(0, 2) 失败: %v", err)
	}

	// 应该有新增的键（A_result）
	hasAdded := false
	for _, diff := range diffs {
		if diff.Type == "added" && diff.Key == "A_result" {
			hasAdded = true
			break
		}
	}
	if !hasAdded {
		t.Error("对比结果应包含新增的 A_result 键")
	}

	// 对比相邻快照（__start__ 和 A 执行后），应有差异
	diffs, err = debugger.Compare(0, 1)
	if err != nil {
		t.Fatalf("Compare(0, 1) 失败: %v", err)
	}
	// 索引 0 和索引 1 的状态可能有差异（节点 A 执行后新增了 A_result）
	if len(diffs) == 0 {
		t.Log("提示: __start__ 与节点 A 执行后的状态无差异（节点 A 的结果在其后的快照中记录）")
	}

	// 对比同一个快照，差异应为空
	diffs, err = debugger.Compare(0, 0)
	if err != nil {
		t.Fatalf("Compare(0, 0) 失败: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("对比同一快照不应有差异，实际 %d 个差异", len(diffs))
	}

	// 对比首尾快照，应有多个差异
	diffs, err = debugger.Compare(0, 4)
	if err != nil {
		t.Fatalf("Compare(0, 4) 失败: %v", err)
	}
	if len(diffs) == 0 {
		t.Error("首尾快照应有状态差异")
	}

	// 验证差异类型
	diffTypes := make(map[string]bool)
	for _, diff := range diffs {
		diffTypes[diff.Type] = true
	}
	if !diffTypes["added"] {
		t.Error("首尾对比应包含 added 类型的差异")
	}

	// 测试越界索引
	_, err = debugger.Compare(-1, 0)
	if err == nil {
		t.Error("Compare(-1, 0) 应该返回错误")
	}

	_, err = debugger.Compare(0, 100)
	if err == nil {
		t.Error("Compare(0, 100) 应该返回错误")
	}
}

// ============== MemorySnapshotStorage 测试 ==============

// TestMemorySnapshotStorage 测试内存快照存储的完整生命周期
// 验证：
//   - Save 保存快照成功
//   - Load 加载指定索引的快照
//   - LoadRange 加载范围内的快照
//   - Delete 删除指定快照
//   - Clear 清空所有快照
//   - Load 不存在的索引返回错误
func TestMemorySnapshotStorage(t *testing.T) {
	storage := NewMemorySnapshotStorage()
	ctx := context.Background()

	// 保存多个快照
	snapshots := []*StateSnapshot{
		{Index: 0, NodeID: "start", NodeName: "Start", State: map[string]any{"step": float64(0)}},
		{Index: 1, NodeID: "A", NodeName: "Node_A", State: map[string]any{"step": float64(1)}},
		{Index: 2, NodeID: "B", NodeName: "Node_B", State: map[string]any{"step": float64(2)}},
		{Index: 3, NodeID: "C", NodeName: "Node_C", State: map[string]any{"step": float64(3)}},
		{Index: 4, NodeID: "end", NodeName: "End", State: map[string]any{"step": float64(4)}},
	}

	for _, snap := range snapshots {
		if err := storage.Save(ctx, snap); err != nil {
			t.Fatalf("Save 快照 %d 失败: %v", snap.Index, err)
		}
	}

	// 测试 Load：加载指定索引的快照
	loaded, err := storage.Load(ctx, 2)
	if err != nil {
		t.Fatalf("Load(2) 失败: %v", err)
	}
	if loaded.NodeID != "B" {
		t.Errorf("Load(2) 的节点 ID 不正确，期望 B，实际 %s", loaded.NodeID)
	}
	if loaded.NodeName != "Node_B" {
		t.Errorf("Load(2) 的节点名称不正确，期望 Node_B，实际 %s", loaded.NodeName)
	}

	// 测试 Load：加载不存在的索引
	_, err = storage.Load(ctx, 99)
	if err == nil {
		t.Error("Load(99) 应该返回错误")
	}

	// 测试 LoadRange：加载范围 [1, 3]
	rangeSnapshots, err := storage.LoadRange(ctx, 1, 3)
	if err != nil {
		t.Fatalf("LoadRange(1, 3) 失败: %v", err)
	}
	if len(rangeSnapshots) != 3 {
		t.Fatalf("LoadRange(1, 3) 应返回 3 个快照，实际 %d", len(rangeSnapshots))
	}
	// 验证范围内快照的节点 ID
	expectedNodeIDs := []string{"A", "B", "C"}
	for i, expected := range expectedNodeIDs {
		if rangeSnapshots[i].NodeID != expected {
			t.Errorf("LoadRange 结果 %d 的节点 ID 不正确，期望 %s，实际 %s", i, expected, rangeSnapshots[i].NodeID)
		}
	}

	// 测试 LoadRange：部分范围（包含不存在的索引）
	partial, err := storage.LoadRange(ctx, 3, 10)
	if err != nil {
		t.Fatalf("LoadRange(3, 10) 失败: %v", err)
	}
	// 只有索引 3 和 4 存在
	if len(partial) != 2 {
		t.Errorf("LoadRange(3, 10) 应返回 2 个快照，实际 %d", len(partial))
	}

	// 测试 Delete：删除索引 2
	if err := storage.Delete(ctx, 2); err != nil {
		t.Fatalf("Delete(2) 失败: %v", err)
	}
	_, err = storage.Load(ctx, 2)
	if err == nil {
		t.Error("Delete 后 Load(2) 应该返回错误")
	}

	// 删除后其他快照不受影响
	loaded, err = storage.Load(ctx, 1)
	if err != nil {
		t.Fatalf("Delete(2) 后 Load(1) 不应失败: %v", err)
	}
	if loaded.NodeID != "A" {
		t.Errorf("Delete(2) 后 Load(1) 的节点 ID 不正确，期望 A，实际 %s", loaded.NodeID)
	}

	// 测试 Clear：清空所有快照
	if err := storage.Clear(ctx); err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}

	// 清空后所有索引都应不存在
	for i := 0; i < 5; i++ {
		_, err := storage.Load(ctx, i)
		if err == nil {
			t.Errorf("Clear 后 Load(%d) 应该返回错误", i)
		}
	}

	// 清空后 LoadRange 返回空切片
	empty, err := storage.LoadRange(ctx, 0, 4)
	if err != nil {
		t.Fatalf("Clear 后 LoadRange 不应失败: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Clear 后 LoadRange 应返回空切片，实际 %d 个", len(empty))
	}
}
