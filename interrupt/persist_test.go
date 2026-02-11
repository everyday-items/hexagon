package interrupt

import (
	"context"
	"testing"
)

func TestSignalToPersistenceMaps_Nil(t *testing.T) {
	addrs, states := SignalToPersistenceMaps(nil)
	if len(addrs) != 0 || len(states) != 0 {
		t.Error("nil 信号应返回空 map")
	}
}

func TestSignalToPersistenceMaps_Simple(t *testing.T) {
	signal := &InterruptSignal{
		ID:      "int-1",
		Address: Address{{Type: SegmentNode, ID: "step1"}},
		Info:    "test",
		State:   "progress",
		IsRoot:  true,
	}

	addrs, states := SignalToPersistenceMaps(signal)

	if len(addrs) != 1 {
		t.Fatalf("应有 1 个地址, got %d", len(addrs))
	}
	if !addrs["int-1"].Equals(signal.Address) {
		t.Error("地址不匹配")
	}

	if len(states) != 1 {
		t.Fatalf("应有 1 个状态, got %d", len(states))
	}
	if states["int-1"] != "progress" {
		t.Error("状态不匹配")
	}
}

func TestSignalToPersistenceMaps_Composite(t *testing.T) {
	child1 := &InterruptSignal{
		ID:      "child-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}, {Type: SegmentTool, ID: "search"}},
		Info:    "搜索确认",
		State:   "search-state",
		IsRoot:  true,
	}
	child2 := &InterruptSignal{
		ID:      "child-2",
		Address: Address{{Type: SegmentNode, ID: "tools"}, {Type: SegmentTool, ID: "delete"}},
		Info:    "删除确认",
		IsRoot:  true,
	}
	parent := &InterruptSignal{
		ID:      "parent-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}},
		Info:    "多工具中断",
		Subs:    []*InterruptSignal{child1, child2},
	}

	addrs, states := SignalToPersistenceMaps(parent)

	// 3 个地址（parent + 2 children）
	if len(addrs) != 3 {
		t.Fatalf("应有 3 个地址, got %d", len(addrs))
	}
	if _, ok := addrs["parent-1"]; !ok {
		t.Error("缺少 parent-1")
	}
	if _, ok := addrs["child-1"]; !ok {
		t.Error("缺少 child-1")
	}
	if _, ok := addrs["child-2"]; !ok {
		t.Error("缺少 child-2")
	}

	// 只有 child-1 有 State
	if len(states) != 1 {
		t.Fatalf("应有 1 个状态, got %d", len(states))
	}
	if states["child-1"] != "search-state" {
		t.Error("child-1 状态不匹配")
	}
}

func TestSignalToPersistenceMaps_DeepTree(t *testing.T) {
	leaf := &InterruptSignal{
		ID:      "leaf",
		Address: Address{{Type: SegmentNode, ID: "a"}, {Type: SegmentSubgraph, ID: "b"}, {Type: SegmentNode, ID: "c"}},
		Info:    "叶子",
		State:   42,
		IsRoot:  true,
	}
	mid := &InterruptSignal{
		ID:      "mid",
		Address: Address{{Type: SegmentNode, ID: "a"}, {Type: SegmentSubgraph, ID: "b"}},
		Info:    "中间",
		Subs:    []*InterruptSignal{leaf},
	}
	root := &InterruptSignal{
		ID:      "root",
		Address: Address{{Type: SegmentNode, ID: "a"}},
		Info:    "根",
		Subs:    []*InterruptSignal{mid},
	}

	addrs, states := SignalToPersistenceMaps(root)

	if len(addrs) != 3 {
		t.Fatalf("应有 3 个地址, got %d", len(addrs))
	}
	if len(states) != 1 {
		t.Fatalf("只有 leaf 有 State, 应有 1 个, got %d", len(states))
	}
	if states["leaf"] != 42 {
		t.Error("leaf 状态不匹配")
	}
}

func TestPopulateResumeInfo(t *testing.T) {
	addr1 := Address{{Type: SegmentNode, ID: "step1"}}
	addr2 := Address{{Type: SegmentNode, ID: "step2"}}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr1,
		"int-2": addr2,
	}, map[string]any{
		"int-1": "state1",
	})

	gri := getGlobalResumeInfo(ctx)
	if gri == nil {
		t.Fatal("globalResumeInfo 不应为 nil")
	}
	if len(gri.id2Addr) != 2 {
		t.Fatalf("应有 2 个地址, got %d", len(gri.id2Addr))
	}
	if len(gri.id2State) != 1 {
		t.Fatalf("应有 1 个状态, got %d", len(gri.id2State))
	}
}

func TestPopulateResumeInfo_AddressCopied(t *testing.T) {
	original := Address{{Type: SegmentNode, ID: "step1"}}
	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": original,
	}, nil)

	// 修改原始地址
	original[0].ID = "modified"

	// 验证 context 中的地址未被修改
	gri := getGlobalResumeInfo(ctx)
	if gri.id2Addr["int-1"][0].ID != "step1" {
		t.Error("PopulateResumeInfo 应复制地址以防外部修改")
	}
}

func TestToInterruptContexts_Nil(t *testing.T) {
	result := ToInterruptContexts(nil)
	if len(result) != 0 {
		t.Error("nil 信号应返回空列表")
	}
}

func TestToInterruptContexts_Simple(t *testing.T) {
	signal := &InterruptSignal{
		ID:      "int-1",
		Address: Address{{Type: SegmentNode, ID: "review"}},
		Info:    "需要审核",
		IsRoot:  true,
	}

	contexts := ToInterruptContexts(signal)
	if len(contexts) != 1 {
		t.Fatalf("应有 1 个上下文, got %d", len(contexts))
	}
	if contexts[0].ID != "int-1" {
		t.Error("ID 不匹配")
	}
	if contexts[0].IsRoot != true {
		t.Error("应为 IsRoot")
	}
	if contexts[0].Parent != nil {
		t.Error("根节点无 Parent")
	}
}

func TestToInterruptContexts_Composite(t *testing.T) {
	child1 := &InterruptSignal{
		ID:      "child-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}, {Type: SegmentTool, ID: "search"}},
		Info:    "搜索确认",
		IsRoot:  true,
	}
	child2 := &InterruptSignal{
		ID:      "child-2",
		Address: Address{{Type: SegmentNode, ID: "tools"}, {Type: SegmentTool, ID: "delete"}},
		Info:    "删除确认",
		IsRoot:  true,
	}
	parent := &InterruptSignal{
		ID:      "parent-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}},
		Info:    "多工具中断",
		Subs:    []*InterruptSignal{child1, child2},
	}

	contexts := ToInterruptContexts(parent)
	if len(contexts) != 3 {
		t.Fatalf("应有 3 个上下文, got %d", len(contexts))
	}

	// 验证父子关系
	if contexts[0].Parent != nil {
		t.Error("parent 不应有 Parent")
	}
	if contexts[1].Parent != contexts[0] {
		t.Error("child-1 的 Parent 应为 parent")
	}
	if contexts[2].Parent != contexts[0] {
		t.Error("child-2 的 Parent 应为 parent")
	}
}

func TestToInterruptContexts_Filter(t *testing.T) {
	child := &InterruptSignal{
		ID:      "child-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}, {Type: SegmentTool, ID: "search"}},
		Info:    "搜索确认",
		IsRoot:  true,
	}
	parent := &InterruptSignal{
		ID:      "parent-1",
		Address: Address{{Type: SegmentNode, ID: "tools"}},
		Info:    "多工具中断",
		Subs:    []*InterruptSignal{child},
	}

	// 只保留 tool 类型
	contexts := ToInterruptContexts(parent, SegmentTool)
	if len(contexts) != 1 {
		t.Fatalf("过滤后应有 1 个上下文, got %d", len(contexts))
	}
	if contexts[0].ID != "child-1" {
		t.Error("应只保留 tool 类型的中断")
	}

	// 只保留 node 类型
	contexts = ToInterruptContexts(parent, SegmentNode)
	if len(contexts) != 1 {
		t.Fatalf("过滤后应有 1 个上下文, got %d", len(contexts))
	}
	if contexts[0].ID != "parent-1" {
		t.Error("应只保留 node 类型的中断")
	}

	// 多类型过滤
	contexts = ToInterruptContexts(parent, SegmentNode, SegmentTool)
	if len(contexts) != 2 {
		t.Fatalf("过滤后应有 2 个上下文, got %d", len(contexts))
	}
}

func TestToInterruptContexts_FilterEmptyAddress(t *testing.T) {
	signal := &InterruptSignal{
		ID:      "int-1",
		Address: Address{}, // 空地址
		Info:    "test",
		IsRoot:  true,
	}

	// 有过滤条件 + 空地址 → 不匹配
	contexts := ToInterruptContexts(signal, SegmentNode)
	if len(contexts) != 0 {
		t.Error("空地址不应匹配任何过滤条件")
	}

	// 无过滤条件 → 始终匹配
	contexts = ToInterruptContexts(signal)
	if len(contexts) != 1 {
		t.Error("无过滤条件应始终匹配")
	}
}

func TestRoundTrip_PersistAndRestore(t *testing.T) {
	// 模拟完整的持久化/恢复流程

	// 1. 创建中断信号
	signal := &InterruptSignal{
		ID:      "int-1",
		Address: Address{{Type: SegmentNode, ID: "review"}},
		Info:    "需要审核",
		State:   map[string]int{"progress": 3},
		IsRoot:  true,
	}

	// 2. 扁平化
	addrs, states := SignalToPersistenceMaps(signal)

	// 3. 恢复到 context
	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, addrs, states)

	// 4. 标记恢复
	ctx = ResumeWithData(ctx, "int-1", map[string]bool{"approved": true})

	// 5. 执行时追加地址段 → 触发匹配
	ctx = AppendAddressSegment(ctx, SegmentNode, "review", "")

	// 6. 验证中断状态
	wasInterrupted, hasState, state := GetInterruptState[map[string]int](ctx)
	if !wasInterrupted {
		t.Error("应能检测到中断")
	}
	if !hasState {
		t.Error("应有保存的状态")
	}
	if state["progress"] != 3 {
		t.Errorf("progress = %d, want 3", state["progress"])
	}

	// 7. 验证恢复上下文
	isTarget, hasData, data := GetResumeContext[map[string]bool](ctx)
	if !isTarget {
		t.Error("应为恢复目标")
	}
	if !hasData {
		t.Error("应有恢复数据")
	}
	if !data["approved"] {
		t.Error("approved 应为 true")
	}
}
