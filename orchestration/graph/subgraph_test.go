package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ============== 子图节点测试 ==============

// TestSubgraphNode_Basic 测试子图作为节点的基本执行功能
//
// 验证：
//   - 子图节点能正确嵌入父图并执行
//   - 子图执行后状态正确传递回父图
func TestSubgraphNode_Basic(t *testing.T) {
	// 创建子图：两步处理，Counter +10 并追加路径
	subgraph, err := NewGraph[TestState]("sub-graph").
		AddNode("sub_step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 10
			s.Path += "S1"
			return s, nil
		}).
		AddNode("sub_step2", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 20
			s.Path += "S2"
			return s, nil
		}).
		AddEdge(START, "sub_step1").
		AddEdge("sub_step1", "sub_step2").
		AddEdge("sub_step2", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图失败: %v", err)
	}

	// 创建父图：前置节点 -> 子图节点 -> 后置节点
	parentGraph, err := NewGraph[TestState]("parent-graph").
		AddNode("pre", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 1
			s.Path += "P"
			return s, nil
		}).
		AddNodeWithBuilder(SubgraphNode("sub", subgraph)).
		AddNode("post", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 100
			s.Path += "E"
			return s, nil
		}).
		AddEdge(START, "pre").
		AddEdge("pre", "sub").
		AddEdge("sub", "post").
		AddEdge("post", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	// 执行父图
	ctx := context.Background()
	result, err := parentGraph.Run(ctx, TestState{Counter: 0, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行父图失败: %v", err)
	}

	// 验证 Counter: 1 (pre) + 10 (sub_step1) + 20 (sub_step2) + 100 (post) = 131
	if result.Counter != 131 {
		t.Errorf("期望 Counter 为 131，实际为 %d", result.Counter)
	}

	// 验证执行路径: P -> S1 -> S2 -> E
	if result.Path != "PS1S2E" {
		t.Errorf("期望 Path 为 'PS1S2E'，实际为 '%s'", result.Path)
	}
}

// TestSubgraphNode_WithStateMapper 测试带状态映射器的子图节点
//
// 验证：
//   - Input 映射器在子图执行前正确转换状态
//   - Output 映射器在子图执行后正确合并状态
func TestSubgraphNode_WithStateMapper(t *testing.T) {
	// 创建子图：对 Counter 翻倍
	subgraph, err := NewGraph[TestState]("mapper-sub").
		AddNode("double", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter *= 2
			s.Path += "D"
			return s, nil
		}).
		AddEdge(START, "double").
		AddEdge("double", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图失败: %v", err)
	}

	// 创建状态映射器
	mapper := &SubgraphStateMapper[TestState]{
		// 输入映射：将 Counter 设为 5（忽略父状态的值）
		Input: func(parentState TestState) TestState {
			return TestState{
				Counter: 5,
				Path:    parentState.Path,
				Data:    parentState.Data,
			}
		},
		// 输出映射：将子图结果的 Counter 加到父状态上
		Output: func(parentState, subOutput TestState) TestState {
			parentState.Counter += subOutput.Counter
			parentState.Path = subOutput.Path
			return parentState
		},
	}

	// 创建父图
	parentGraph, err := NewGraph[TestState]("parent-mapper").
		AddNodeWithBuilder(SubgraphNode("mapped_sub", subgraph, mapper)).
		AddEdge(START, "mapped_sub").
		AddEdge("mapped_sub", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	// 执行：父状态 Counter=100，输入映射后 Counter=5，翻倍后 Counter=10
	// 输出映射：父 Counter(100) + 子 Counter(10) = 110
	ctx := context.Background()
	result, err := parentGraph.Run(ctx, TestState{Counter: 100, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.Counter != 110 {
		t.Errorf("期望 Counter 为 110，实际为 %d", result.Counter)
	}

	if result.Path != "D" {
		t.Errorf("期望 Path 为 'D'，实际为 '%s'", result.Path)
	}
}

// TestSubgraphNode_SubgraphError 测试子图错误传播
//
// 验证：
//   - 子图内部节点的错误能正确传播到父图
//   - 错误信息包含子图名称
func TestSubgraphNode_SubgraphError(t *testing.T) {
	expectedErr := errors.New("子图内部错误")

	// 创建会失败的子图
	subgraph, err := NewGraph[TestState]("error-sub").
		AddNode("fail_step", func(ctx context.Context, s TestState) (TestState, error) {
			return s, expectedErr
		}).
		AddEdge(START, "fail_step").
		AddEdge("fail_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图失败: %v", err)
	}

	// 创建父图
	parentGraph, err := NewGraph[TestState]("parent-error").
		AddNodeWithBuilder(SubgraphNode("error_sub", subgraph)).
		AddEdge(START, "error_sub").
		AddEdge("error_sub", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	ctx := context.Background()
	_, err = parentGraph.Run(ctx, TestState{Data: make(map[string]string)})

	if err == nil {
		t.Fatal("期望子图执行返回错误，但未返回")
	}

	// 验证错误信息中包含子图名称
	if !strings.Contains(err.Error(), "error_sub") {
		t.Errorf("错误信息应包含子图名称 'error_sub'，实际为: %s", err.Error())
	}

	// 验证原始错误被包装
	if !strings.Contains(err.Error(), "子图内部错误") {
		t.Errorf("错误信息应包含原始错误，实际为: %s", err.Error())
	}
}

// ============== 动态图测试 ==============

// TestDynamicGraph_AddRemoveNode 测试动态图的节点添加和移除
//
// 验证：
//   - AddNodeDynamic 能正确添加节点
//   - RemoveNodeDynamic 能正确移除节点及其关联边
//   - 版本号在每次修改后递增
//   - 重复添加和移除保留名称会返回错误
func TestDynamicGraph_AddRemoveNode(t *testing.T) {
	dg := NewDynamicGraph[TestState]("dynamic-graph")

	// 验证初始版本号
	if v := dg.Version(); v != 0 {
		t.Errorf("期望初始版本号为 0，实际为 %d", v)
	}

	// 添加节点
	err := dg.AddNodeDynamic("node_a", func(ctx context.Context, s TestState) (TestState, error) {
		s.Counter += 1
		return s, nil
	})
	if err != nil {
		t.Fatalf("添加节点失败: %v", err)
	}
	if v := dg.Version(); v != 1 {
		t.Errorf("添加节点后期望版本号为 1，实际为 %d", v)
	}

	// 尝试添加重复节点
	err = dg.AddNodeDynamic("node_a", func(ctx context.Context, s TestState) (TestState, error) {
		return s, nil
	})
	if err == nil {
		t.Error("期望添加重复节点返回错误")
	}

	// 尝试添加保留名称节点
	err = dg.AddNodeDynamic(START, func(ctx context.Context, s TestState) (TestState, error) {
		return s, nil
	})
	if err == nil {
		t.Error("期望添加保留名称节点返回错误")
	}

	// 添加第二个节点
	err = dg.AddNodeDynamic("node_b", func(ctx context.Context, s TestState) (TestState, error) {
		s.Counter += 10
		return s, nil
	})
	if err != nil {
		t.Fatalf("添加第二个节点失败: %v", err)
	}

	// 验证节点数量（不含 START 和 END）
	if len(dg.Nodes) != 2 {
		t.Errorf("期望有 2 个节点，实际为 %d", len(dg.Nodes))
	}

	// 移除节点
	err = dg.RemoveNodeDynamic("node_a")
	if err != nil {
		t.Fatalf("移除节点失败: %v", err)
	}
	if len(dg.Nodes) != 1 {
		t.Errorf("移除后期望有 1 个节点，实际为 %d", len(dg.Nodes))
	}

	// 尝试移除不存在的节点
	err = dg.RemoveNodeDynamic("nonexistent")
	if err == nil {
		t.Error("期望移除不存在节点返回错误")
	}

	// 尝试移除保留节点
	err = dg.RemoveNodeDynamic(END)
	if err == nil {
		t.Error("期望移除保留节点返回错误")
	}

	// 验证最终版本号（添加 node_a=1, 添加 node_b=2, 移除 node_a=3）
	// 注意：添加重复节点和保留名称节点不会增加版本号
	if v := dg.Version(); v != 3 {
		t.Errorf("期望最终版本号为 3，实际为 %d", v)
	}
}

// TestDynamicGraph_AddEdge 测试动态图的边添加和移除
//
// 验证：
//   - AddEdgeDynamic 能正确添加边
//   - RemoveEdgeDynamic 能正确移除边
//   - 引用不存在节点的边会返回错误
//   - 构建后的动态图能正常执行
func TestDynamicGraph_AddEdge(t *testing.T) {
	dg := NewDynamicGraph[TestState]("edge-graph")

	// 添加节点
	_ = dg.AddNodeDynamic("step1", func(ctx context.Context, s TestState) (TestState, error) {
		s.Counter += 1
		s.Path += "1"
		return s, nil
	})
	_ = dg.AddNodeDynamic("step2", func(ctx context.Context, s TestState) (TestState, error) {
		s.Counter += 2
		s.Path += "2"
		return s, nil
	})

	// 尝试添加引用不存在节点的边
	err := dg.AddEdgeDynamic("step1", "nonexistent")
	if err == nil {
		t.Error("期望引用不存在目标节点返回错误")
	}
	err = dg.AddEdgeDynamic("nonexistent", "step1")
	if err == nil {
		t.Error("期望引用不存在源节点返回错误")
	}

	// 添加正常边
	err = dg.AddEdgeDynamic("step1", "step2")
	if err != nil {
		t.Fatalf("添加边失败: %v", err)
	}

	// 移除边
	err = dg.RemoveEdgeDynamic("step1", "step2")
	if err != nil {
		t.Fatalf("移除边失败: %v", err)
	}

	// 尝试移除不存在的边
	err = dg.RemoveEdgeDynamic("step1", "step2")
	if err == nil {
		t.Error("期望移除不存在的边返回错误")
	}

	// 重新添加边，构建并执行图
	_ = dg.AddEdgeDynamic("step1", "step2")

	// 手动添加 START->step1 和 step2->END 边
	dg.Edges = append(dg.Edges, &Edge{From: START, To: "step1", Type: EdgeTypeNormal})
	dg.adjacency[START] = append(dg.adjacency[START], "step1")
	dg.Edges = append(dg.Edges, &Edge{From: "step2", To: END, Type: EdgeTypeNormal})
	dg.adjacency["step2"] = append(dg.adjacency["step2"], END)

	built, err := dg.Build()
	if err != nil {
		t.Fatalf("构建动态图失败: %v", err)
	}

	ctx := context.Background()
	result, err := built.Run(ctx, TestState{Counter: 0, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行动态图失败: %v", err)
	}

	if result.Counter != 3 {
		t.Errorf("期望 Counter 为 3，实际为 %d", result.Counter)
	}
	if result.Path != "12" {
		t.Errorf("期望 Path 为 '12'，实际为 '%s'", result.Path)
	}
}

// ============== 图组合器测试 ==============

// TestGraphComposer_Compose 测试图组合器将多个子图组合成一个大图
//
// 验证：
//   - 多个子图能按顺序组合
//   - 组合后的图能正确执行
//   - 状态在子图之间正确传递
func TestGraphComposer_Compose(t *testing.T) {
	// 创建第一个子图：Counter +1
	graph1, err := NewGraph[TestState]("graph1").
		AddNode("g1_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 1
			s.Path += "G1"
			return s, nil
		}).
		AddEdge(START, "g1_step").
		AddEdge("g1_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建图1失败: %v", err)
	}

	// 创建第二个子图：Counter +10
	graph2, err := NewGraph[TestState]("graph2").
		AddNode("g2_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 10
			s.Path += "G2"
			return s, nil
		}).
		AddEdge(START, "g2_step").
		AddEdge("g2_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建图2失败: %v", err)
	}

	// 使用组合器顺序连接
	composer := NewGraphComposer[TestState]("composed")
	composer.AddGraph(graph1)
	composer.AddGraph(graph2)
	composed, err := composer.Sequential().Compose()
	if err != nil {
		t.Fatalf("组合图失败: %v", err)
	}

	// 执行组合后的图
	ctx := context.Background()
	result, err := composed.Run(ctx, TestState{Counter: 0, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行组合图失败: %v", err)
	}

	// 验证 Counter: 1 + 10 = 11
	if result.Counter != 11 {
		t.Errorf("期望 Counter 为 11，实际为 %d", result.Counter)
	}

	// 验证路径包含两个子图的执行标记
	if !strings.Contains(result.Path, "G1") || !strings.Contains(result.Path, "G2") {
		t.Errorf("期望 Path 包含 'G1' 和 'G2'，实际为 '%s'", result.Path)
	}
}

// ============== 并行子图测试 ==============

// TestParallelSubgraphs 测试并行子图执行
//
// 验证：
//   - 多个子图能并行执行
//   - 状态合并器正确合并所有子图的输出
//   - 每个子图独立运行（状态克隆）
func TestParallelSubgraphs(t *testing.T) {
	// 创建子图 A：Counter +10
	subA, err := NewGraph[TestState]("sub-a").
		AddNode("a_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 10
			s.Path += "A"
			return s, nil
		}).
		AddEdge(START, "a_step").
		AddEdge("a_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图A失败: %v", err)
	}

	// 创建子图 B：Counter +20
	subB, err := NewGraph[TestState]("sub-b").
		AddNode("b_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 20
			s.Path += "B"
			return s, nil
		}).
		AddEdge(START, "b_step").
		AddEdge("b_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图B失败: %v", err)
	}

	// 创建子图 C：Counter +30
	subC, err := NewGraph[TestState]("sub-c").
		AddNode("c_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter += 30
			s.Path += "C"
			return s, nil
		}).
		AddEdge(START, "c_step").
		AddEdge("c_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建子图C失败: %v", err)
	}

	// 状态合并器：累加所有子图的 Counter，拼接所有路径
	merger := func(original TestState, outputs []TestState) TestState {
		result := original
		result.Counter = 0
		result.Path = ""
		for _, out := range outputs {
			result.Counter += out.Counter
			result.Path += out.Path
		}
		return result
	}

	// 创建并行节点
	parallelNode := ParallelSubgraphs("parallel", []*Graph[TestState]{subA, subB, subC}, merger)

	// 将并行节点嵌入父图
	parentGraph, err := NewGraph[TestState]("parent-parallel").
		AddNodeWithBuilder(parallelNode).
		AddEdge(START, "parallel").
		AddEdge("parallel", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	ctx := context.Background()
	result, err := parentGraph.Run(ctx, TestState{Counter: 0, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行并行子图失败: %v", err)
	}

	// 验证 Counter: 10 + 20 + 30 = 60
	if result.Counter != 60 {
		t.Errorf("期望 Counter 为 60，实际为 %d", result.Counter)
	}

	// 验证路径包含所有子图标记（顺序不确定，因为是并行执行）
	if !strings.Contains(result.Path, "A") ||
		!strings.Contains(result.Path, "B") ||
		!strings.Contains(result.Path, "C") {
		t.Errorf("期望 Path 包含 'A'、'B'、'C'，实际为 '%s'", result.Path)
	}
}

// ============== 条件子图测试 ==============

// TestConditionalSubgraph 测试条件子图选择执行
//
// 验证：
//   - 根据选择器函数正确选择子图
//   - 不同条件走不同分支
//   - 无效索引返回错误
func TestConditionalSubgraph(t *testing.T) {
	// 创建子图 0：标记路径为 "FAST"
	fastGraph, err := NewGraph[TestState]("fast").
		AddNode("fast_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "FAST"
			s.Counter += 1
			return s, nil
		}).
		AddEdge(START, "fast_step").
		AddEdge("fast_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建快速子图失败: %v", err)
	}

	// 创建子图 1：标记路径为 "SLOW"
	slowGraph, err := NewGraph[TestState]("slow").
		AddNode("slow_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "SLOW"
			s.Counter += 100
			return s, nil
		}).
		AddEdge(START, "slow_step").
		AddEdge("slow_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建慢速子图失败: %v", err)
	}

	// 选择器：Counter < 10 走快速路径，否则走慢速路径
	selector := func(s TestState) int {
		if s.Counter < 10 {
			return 0
		}
		return 1
	}

	condNode := ConditionalSubgraph("cond", selector, fastGraph, slowGraph)

	parentGraph, err := NewGraph[TestState]("parent-cond").
		AddNodeWithBuilder(condNode).
		AddEdge(START, "cond").
		AddEdge("cond", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	ctx := context.Background()

	// 测试快速路径（Counter = 5 < 10）
	result, err := parentGraph.Run(ctx, TestState{Counter: 5, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行快速路径失败: %v", err)
	}
	if result.Path != "FAST" {
		t.Errorf("期望 Path 为 'FAST'，实际为 '%s'", result.Path)
	}
	if result.Counter != 6 {
		t.Errorf("期望 Counter 为 6，实际为 %d", result.Counter)
	}

	// 测试慢速路径（Counter = 50 >= 10）
	result, err = parentGraph.Run(ctx, TestState{Counter: 50, Path: "", Data: make(map[string]string)})
	if err != nil {
		t.Fatalf("执行慢速路径失败: %v", err)
	}
	if result.Path != "SLOW" {
		t.Errorf("期望 Path 为 'SLOW'，实际为 '%s'", result.Path)
	}
	if result.Counter != 150 {
		t.Errorf("期望 Counter 为 150，实际为 %d", result.Counter)
	}

	// 测试无效索引
	invalidSelector := func(s TestState) int { return 99 }
	invalidNode := ConditionalSubgraph("invalid_cond", invalidSelector, fastGraph, slowGraph)
	invalidGraph, err := NewGraph[TestState]("parent-invalid").
		AddNodeWithBuilder(invalidNode).
		AddEdge(START, "invalid_cond").
		AddEdge("invalid_cond", END).
		Build()
	if err != nil {
		t.Fatalf("构建无效条件图失败: %v", err)
	}

	_, err = invalidGraph.Run(ctx, TestState{Data: make(map[string]string)})
	if err == nil {
		t.Error("期望无效索引返回错误")
	}
	if !strings.Contains(err.Error(), "invalid subgraph index") {
		t.Errorf("错误信息应包含 'invalid subgraph index'，实际为: %s", err.Error())
	}
}

// ============== 分支节点测试 ==============

// TestBranchNode 测试分支节点的分支选择和执行
//
// 验证：
//   - 根据选择器返回的标签正确选择分支子图
//   - 不同标签执行不同分支
//   - 不存在的标签返回错误
func TestBranchNode(t *testing.T) {
	// 创建分支子图：审批通过
	approveGraph, err := NewGraph[TestState]("approve").
		AddNode("approve_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "APPROVED"
			s.Counter = 1
			return s, nil
		}).
		AddEdge(START, "approve_step").
		AddEdge("approve_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建审批通过子图失败: %v", err)
	}

	// 创建分支子图：审批拒绝
	rejectGraph, err := NewGraph[TestState]("reject").
		AddNode("reject_step", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "REJECTED"
			s.Counter = -1
			return s, nil
		}).
		AddEdge(START, "reject_step").
		AddEdge("reject_step", END).
		Build()
	if err != nil {
		t.Fatalf("构建审批拒绝子图失败: %v", err)
	}

	// 创建分支映射
	branches := map[string]*Graph[TestState]{
		"approve": approveGraph,
		"reject":  rejectGraph,
	}

	// 选择器：根据 Data 中的 "action" 字段决定分支
	selector := func(s TestState) string {
		if action, ok := s.Data["action"]; ok {
			return action
		}
		return "unknown"
	}

	branchNode := BranchNode("branch", branches, selector)

	parentGraph, err := NewGraph[TestState]("parent-branch").
		AddNodeWithBuilder(branchNode).
		AddEdge(START, "branch").
		AddEdge("branch", END).
		Build()
	if err != nil {
		t.Fatalf("构建父图失败: %v", err)
	}

	ctx := context.Background()

	// 测试审批通过分支
	approveState := TestState{
		Counter: 0,
		Path:    "",
		Data:    map[string]string{"action": "approve"},
	}
	result, err := parentGraph.Run(ctx, approveState)
	if err != nil {
		t.Fatalf("执行审批通过分支失败: %v", err)
	}
	if result.Path != "APPROVED" {
		t.Errorf("期望 Path 为 'APPROVED'，实际为 '%s'", result.Path)
	}
	if result.Counter != 1 {
		t.Errorf("期望 Counter 为 1，实际为 %d", result.Counter)
	}

	// 测试审批拒绝分支
	rejectState := TestState{
		Counter: 0,
		Path:    "",
		Data:    map[string]string{"action": "reject"},
	}
	result, err = parentGraph.Run(ctx, rejectState)
	if err != nil {
		t.Fatalf("执行审批拒绝分支失败: %v", err)
	}
	if result.Path != "REJECTED" {
		t.Errorf("期望 Path 为 'REJECTED'，实际为 '%s'", result.Path)
	}
	if result.Counter != -1 {
		t.Errorf("期望 Counter 为 -1，实际为 %d", result.Counter)
	}

	// 测试不存在的分支
	unknownState := TestState{
		Counter: 0,
		Path:    "",
		Data:    map[string]string{"action": "unknown_action"},
	}
	_, err = parentGraph.Run(ctx, unknownState)
	if err == nil {
		t.Error("期望不存在的分支返回错误")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("错误信息应包含 'not found'，实际为: %s", err.Error())
	}
}
