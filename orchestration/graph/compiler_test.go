package graph

import (
	"context"
	"testing"
	"time"
)

// TestCompile_Basic 测试基本编译功能
// 验证编译后返回的 CompiledGraph 非空，且 ExecutionPlan 和 Stats 正确初始化
func TestCompile_Basic(t *testing.T) {
	// 构建一个最简单的图: START -> step1 -> END
	g, err := NewGraph[TestState]("basic-compile").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	// 编译图
	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	// 验证 CompiledGraph 非空
	if cg == nil {
		t.Fatal("编译后的图不应为 nil")
	}

	// 验证 ExecutionPlan 非空
	if cg.ExecutionPlan == nil {
		t.Fatal("ExecutionPlan 不应为 nil")
	}

	// 验证 Stats 已初始化
	if cg.Stats == nil {
		t.Fatal("Stats 不应为 nil")
	}

	// 验证 Stats.NodeStats map 已初始化
	if cg.Stats.NodeStats == nil {
		t.Fatal("Stats.NodeStats 不应为 nil")
	}

	// 验证初始统计值为零值
	if cg.Stats.TotalExecutions != 0 {
		t.Errorf("初始 TotalExecutions 应为 0，实际为 %d", cg.Stats.TotalExecutions)
	}

	if cg.Stats.TotalDuration != 0 {
		t.Errorf("初始 TotalDuration 应为 0，实际为 %v", cg.Stats.TotalDuration)
	}

	// 验证编译后的图仍然可以正常执行
	ctx := context.Background()
	result, err := cg.Run(ctx, TestState{Counter: 0})
	if err != nil {
		t.Fatalf("执行编译后的图失败: %v", err)
	}

	if result.Counter != 1 {
		t.Errorf("期望 Counter=1，实际为 %d", result.Counter)
	}
}

// TestCompile_TopologicalOrder 测试拓扑排序的正确性
// 构建线性图 A -> B -> C，验证拓扑序满足依赖关系
func TestCompile_TopologicalOrder(t *testing.T) {
	// 构建线性图: START -> A -> B -> C -> END
	g, err := NewGraph[TestState]("topo-order").
		AddNode("A", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "A"
			return s, nil
		}).
		AddNode("B", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "B"
			return s, nil
		}).
		AddNode("C", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "C"
			return s, nil
		}).
		AddEdge(START, "A").
		AddEdge("A", "B").
		AddEdge("B", "C").
		AddEdge("C", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	order := cg.ExecutionPlan.TopologicalOrder
	if len(order) == 0 {
		t.Fatal("拓扑排序结果不应为空")
	}

	// 构建位置索引，用于验证顺序
	posMap := make(map[string]int)
	for i, name := range order {
		posMap[name] = i
	}

	// 验证所有节点都在拓扑序中（包括 START 和 END）
	expectedNodes := []string{START, "A", "B", "C", END}
	for _, name := range expectedNodes {
		if _, ok := posMap[name]; !ok {
			t.Errorf("节点 %s 未出现在拓扑排序结果中", name)
		}
	}

	// 验证依赖顺序: START 在 A 之前, A 在 B 之前, B 在 C 之前, C 在 END 之前
	pairs := [][2]string{
		{START, "A"},
		{"A", "B"},
		{"B", "C"},
		{"C", END},
	}
	for _, pair := range pairs {
		from, to := pair[0], pair[1]
		fromPos, fromOK := posMap[from]
		toPos, toOK := posMap[to]
		if !fromOK || !toOK {
			continue // 前面已经报过错
		}
		if fromPos >= toPos {
			t.Errorf("拓扑排序违反依赖: %s (位置 %d) 应在 %s (位置 %d) 之前",
				from, fromPos, to, toPos)
		}
	}
}

// TestCompile_ParallelGroups 测试并行组检测
// 构建菱形图: A -> {B, C} -> D，验证 B 和 C 在同一并行组
func TestCompile_ParallelGroups(t *testing.T) {
	// 构建菱形图: START -> A -> B, A -> C, B -> D, C -> D -> END
	g, err := NewGraph[TestState]("parallel-groups").
		AddNode("A", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "A"
			return s, nil
		}).
		AddNode("B", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "B"
			return s, nil
		}).
		AddNode("C", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "C"
			return s, nil
		}).
		AddNode("D", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path += "D"
			return s, nil
		}).
		AddEdge(START, "A").
		AddEdge("A", "B").
		AddEdge("A", "C").
		AddEdge("B", "D").
		AddEdge("C", "D").
		AddEdge("D", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	groups := cg.ExecutionPlan.ParallelGroups
	if len(groups) == 0 {
		t.Fatal("并行组不应为空")
	}

	// 检查 B 和 C 是否在同一个并行组中
	// 它们的依赖相同（都依赖 A），所以应该处于同一层级
	foundBCGroup := false
	for _, group := range groups {
		hasB := false
		hasC := false
		for _, node := range group {
			if node == "B" {
				hasB = true
			}
			if node == "C" {
				hasC = true
			}
		}
		if hasB && hasC {
			foundBCGroup = true
			break
		}
	}

	if !foundBCGroup {
		t.Error("B 和 C 应在同一并行组中，但未找到这样的组")
		t.Logf("实际并行组: %v", groups)
	}

	// 验证并行组的层级关系：A 所在组应在 B/C 所在组之前，B/C 所在组应在 D 所在组之前
	groupIndexOf := func(nodeName string) int {
		for i, group := range groups {
			for _, n := range group {
				if n == nodeName {
					return i
				}
			}
		}
		return -1
	}

	aIdx := groupIndexOf("A")
	bIdx := groupIndexOf("B")
	dIdx := groupIndexOf("D")

	if aIdx >= bIdx {
		t.Errorf("A 的并行组索引 (%d) 应小于 B 的并行组索引 (%d)", aIdx, bIdx)
	}

	if bIdx >= dIdx {
		t.Errorf("B 的并行组索引 (%d) 应小于 D 的并行组索引 (%d)", bIdx, dIdx)
	}
}

// TestCompile_CriticalPath 测试关键路径计算
// 验证从 START 到 END 的关键路径包含正确的节点序列
func TestCompile_CriticalPath(t *testing.T) {
	// 构建线性图: START -> step1 -> step2 -> END
	g, err := NewGraph[TestState]("critical-path").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	criticalPath := cg.ExecutionPlan.CriticalPath
	if len(criticalPath) == 0 {
		t.Fatal("关键路径不应为空")
	}

	// 关键路径应从 START 开始
	if criticalPath[0] != START {
		t.Errorf("关键路径应从 %s 开始，实际为 %s", START, criticalPath[0])
	}

	// 关键路径应以 END 结束
	if criticalPath[len(criticalPath)-1] != END {
		t.Errorf("关键路径应以 %s 结束，实际为 %s", END, criticalPath[len(criticalPath)-1])
	}

	// 关键路径应包含所有线性节点: START -> step1 -> step2 -> END
	if len(criticalPath) < 3 {
		t.Errorf("线性图的关键路径长度至少应为 3 (START + 节点 + END)，实际为 %d", len(criticalPath))
	}

	t.Logf("关键路径: %v", criticalPath)
}

// TestCompile_RunWithStats 测试执行统计功能
// 验证 RunWithStats 返回的 ExecutionResult 包含正确的时间和节点计时信息
func TestCompile_RunWithStats(t *testing.T) {
	// 构建图: START -> slow -> fast -> END
	// slow 节点包含短暂 sleep 以验证耗时统计
	g, err := NewGraph[TestState]("stats-graph").
		AddNode("slow", func(ctx context.Context, s TestState) (TestState, error) {
			time.Sleep(10 * time.Millisecond)
			s.Counter++
			s.Path += "slow"
			return s, nil
		}).
		AddNode("fast", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			s.Path += "fast"
			return s, nil
		}).
		AddEdge(START, "slow").
		AddEdge("slow", "fast").
		AddEdge("fast", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	ctx := context.Background()

	// 第一次执行
	result, execResult, err := cg.RunWithStats(ctx, TestState{Counter: 0, Path: ""})
	if err != nil {
		t.Fatalf("RunWithStats 执行失败: %v", err)
	}

	// 验证执行结果
	if result.Counter != 2 {
		t.Errorf("期望 Counter=2，实际为 %d", result.Counter)
	}

	if result.Path != "slowfast" {
		t.Errorf("期望 Path='slowfast'，实际为 '%s'", result.Path)
	}

	// 验证 ExecutionResult 时间字段
	if execResult.StartTime.IsZero() {
		t.Error("StartTime 不应为零值")
	}

	if execResult.EndTime.IsZero() {
		t.Error("EndTime 不应为零值")
	}

	if execResult.Duration <= 0 {
		t.Error("Duration 应大于 0")
	}

	// EndTime 应在 StartTime 之后
	if !execResult.EndTime.After(execResult.StartTime) {
		t.Error("EndTime 应在 StartTime 之后")
	}

	// 验证 Duration 至少 10ms（因为 slow 节点 sleep 了 10ms）
	if execResult.Duration < 10*time.Millisecond {
		t.Errorf("Duration 至少应为 10ms，实际为 %v", execResult.Duration)
	}

	// 验证 NodeTiming 包含执行过的节点
	if len(execResult.NodeTiming) == 0 {
		t.Error("NodeTiming 不应为空")
	}

	// 验证 slow 节点的耗时至少 10ms
	if slowDuration, ok := execResult.NodeTiming["slow"]; ok {
		if slowDuration < 10*time.Millisecond {
			t.Errorf("slow 节点耗时至少应为 10ms，实际为 %v", slowDuration)
		}
	} else {
		t.Error("NodeTiming 中应包含 'slow' 节点的计时")
	}

	// 验证 Error 字段为 nil（执行成功）
	if execResult.Error != nil {
		t.Errorf("执行成功时 Error 应为 nil，实际为 %v", execResult.Error)
	}

	// 再次通过 Run 执行，验证 Stats 累积
	_, err = cg.Run(ctx, TestState{Counter: 0})
	if err != nil {
		t.Fatalf("第二次执行失败: %v", err)
	}

	// 验证 TotalExecutions 累积（Run 方法会累加）
	if cg.Stats.TotalExecutions < 1 {
		t.Errorf("执行后 TotalExecutions 至少应为 1，实际为 %d", cg.Stats.TotalExecutions)
	}

	// 验证 TotalDuration 累积
	if cg.Stats.TotalDuration <= 0 {
		t.Error("执行后 TotalDuration 应大于 0")
	}

	// 验证 LastExecution 已更新
	if cg.Stats.LastExecution.IsZero() {
		t.Error("执行后 LastExecution 不应为零值")
	}
}

// TestCompile_Dependencies 测试依赖关系映射
// 验证 Dependencies 正确记录了每个节点的前驱节点
func TestCompile_Dependencies(t *testing.T) {
	// 构建菱形图: START -> A -> B, A -> C, B -> D, C -> D -> END
	g, err := NewGraph[TestState]("dependencies").
		AddNode("A", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("B", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("C", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("D", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddEdge(START, "A").
		AddEdge("A", "B").
		AddEdge("A", "C").
		AddEdge("B", "D").
		AddEdge("C", "D").
		AddEdge("D", END).
		Build()

	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	cg, err := Compile[TestState](g)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	deps := cg.ExecutionPlan.Dependencies
	if deps == nil {
		t.Fatal("Dependencies 不应为 nil")
	}

	// 验证 A 依赖 START
	aDeps := deps["A"]
	if !containsString(aDeps, START) {
		t.Errorf("A 的依赖应包含 %s，实际为 %v", START, aDeps)
	}

	// 验证 B 依赖 A
	bDeps := deps["B"]
	if !containsString(bDeps, "A") {
		t.Errorf("B 的依赖应包含 A，实际为 %v", bDeps)
	}

	// 验证 C 依赖 A
	cDeps := deps["C"]
	if !containsString(cDeps, "A") {
		t.Errorf("C 的依赖应包含 A，实际为 %v", cDeps)
	}

	// 验证 D 同时依赖 B 和 C
	dDeps := deps["D"]
	if !containsString(dDeps, "B") {
		t.Errorf("D 的依赖应包含 B，实际为 %v", dDeps)
	}
	if !containsString(dDeps, "C") {
		t.Errorf("D 的依赖应包含 C，实际为 %v", dDeps)
	}

	// 验证 END 依赖 D
	endDeps := deps[END]
	if !containsString(endDeps, "D") {
		t.Errorf("END 的依赖应包含 D，实际为 %v", endDeps)
	}

	// 验证 START 没有依赖（或依赖为空）
	startDeps := deps[START]
	if len(startDeps) != 0 {
		t.Errorf("START 不应有依赖，实际为 %v", startDeps)
	}
}

// containsString 检查字符串切片中是否包含指定字符串
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
