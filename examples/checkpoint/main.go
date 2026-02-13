// Package main 演示 Hexagon 检查点保存和恢复
//
// 检查点系统用于保存图执行的中间状态，支持：
//   - 内存检查点: 保存/加载/列出检查点
//   - 线程隔离: 不同执行实例独立存储
//   - 历史追溯: 通过 ParentID 链接检查点历史
//
// 运行方式:
//
//	go run ./examples/checkpoint/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/everyday-items/hexagon/orchestration/graph"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: 检查点保存和加载 ===")
	runSaveAndLoad(ctx)

	fmt.Println("\n=== 示例 2: 检查点历史链 ===")
	runCheckpointHistory(ctx)

	fmt.Println("\n=== 示例 3: 多线程检查点隔离 ===")
	runMultiThread(ctx)
}

// runSaveAndLoad 演示检查点保存和加载
func runSaveAndLoad(ctx context.Context) {
	saver := graph.NewMemoryCheckpointSaver()

	// 构造状态
	state := map[string]any{
		"messages": []string{"hello", "world"},
		"counter":  42,
	}
	stateJSON, _ := json.Marshal(state)

	// 保存检查点
	cp := &graph.Checkpoint{
		ID:             "cp-001",
		ThreadID:       "thread-main",
		GraphName:      "demo-graph",
		CurrentNode:    "process",
		State:          stateJSON,
		PendingNodes:   []string{"output"},
		CompletedNodes: []string{"input", "process"},
	}

	err := saver.Save(ctx, cp)
	if err != nil {
		log.Fatalf("保存失败: %v", err)
	}
	fmt.Println("  已保存检查点 cp-001")

	// 加载检查点
	loaded, err := saver.Load(ctx, "thread-main")
	if err != nil {
		log.Fatalf("加载失败: %v", err)
	}
	if loaded != nil {
		fmt.Printf("  已加载: ID=%s, 当前节点=%s\n", loaded.ID, loaded.CurrentNode)
		fmt.Printf("  已完成节点: %v\n", loaded.CompletedNodes)
		fmt.Printf("  待执行节点: %v\n", loaded.PendingNodes)

		// 还原状态
		var restoredState map[string]any
		json.Unmarshal(loaded.State, &restoredState)
		fmt.Printf("  恢复的状态: %v\n", restoredState)
	}
}

// runCheckpointHistory 演示检查点历史链
func runCheckpointHistory(ctx context.Context) {
	saver := graph.NewMemoryCheckpointSaver()

	threadID := "thread-history"

	// 模拟多步执行，每步保存检查点
	steps := []struct {
		id, node string
		data     map[string]any
	}{
		{"cp-h1", "input", map[string]any{"step": 1, "data": "raw"}},
		{"cp-h2", "transform", map[string]any{"step": 2, "data": "cleaned"}},
		{"cp-h3", "output", map[string]any{"step": 3, "data": "result"}},
	}

	parentID := ""
	for _, s := range steps {
		stateJSON, _ := json.Marshal(s.data)
		cp := &graph.Checkpoint{
			ID:          s.id,
			ThreadID:    threadID,
			GraphName:   "pipeline",
			CurrentNode: s.node,
			State:       stateJSON,
			ParentID:    parentID,
		}
		saver.Save(ctx, cp)
		parentID = s.id
	}
	fmt.Println("  已保存 3 个检查点")

	// 列出所有检查点
	all, _ := saver.List(ctx, threadID)
	fmt.Printf("  线程 %s 共有 %d 个检查点:\n", threadID, len(all))
	for _, cp := range all {
		fmt.Printf("    %s → 节点: %s (父: %s)\n", cp.ID, cp.CurrentNode, cp.ParentID)
	}

	// 按 ID 加载特定检查点
	cp2, _ := saver.LoadByID(ctx, "cp-h2")
	if cp2 != nil {
		fmt.Printf("  回溯到 %s: 节点=%s\n", cp2.ID, cp2.CurrentNode)
	}
}

// runMultiThread 演示多线程检查点隔离
func runMultiThread(ctx context.Context) {
	saver := graph.NewMemoryCheckpointSaver()

	// 线程 A
	stateA, _ := json.Marshal(map[string]any{"user": "Alice"})
	saver.Save(ctx, &graph.Checkpoint{
		ID: "cp-a1", ThreadID: "thread-A", GraphName: "chat",
		CurrentNode: "respond", State: stateA,
	})

	// 线程 B
	stateB, _ := json.Marshal(map[string]any{"user": "Bob"})
	saver.Save(ctx, &graph.Checkpoint{
		ID: "cp-b1", ThreadID: "thread-B", GraphName: "chat",
		CurrentNode: "respond", State: stateB,
	})

	// 各线程独立加载
	cpA, _ := saver.Load(ctx, "thread-A")
	cpB, _ := saver.Load(ctx, "thread-B")

	var stateAMap, stateBMap map[string]any
	json.Unmarshal(cpA.State, &stateAMap)
	json.Unmarshal(cpB.State, &stateBMap)

	fmt.Printf("  线程 A: user=%s\n", stateAMap["user"])
	fmt.Printf("  线程 B: user=%s\n", stateBMap["user"])

	// 清理线程 A
	saver.DeleteThread(ctx, "thread-A")
	allA, _ := saver.List(ctx, "thread-A")
	allB, _ := saver.List(ctx, "thread-B")
	fmt.Printf("  清理后: 线程 A=%d 条, 线程 B=%d 条\n", len(allA), len(allB))
}
