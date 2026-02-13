// Package main 演示 Hexagon 共识机制
//
// 共识系统支持多种投票策略：
//   - Majority: 多数投票
//   - Unanimous: 全票通过
//   - Weighted: 加权投票
//   - Borda: Borda 计数法
//
// 本示例展示共识策略和投票数据结构的使用。
//
// 运行方式:
//
//	go run ./examples/consensus/
package main

import (
	"fmt"
	"time"

	"github.com/everyday-items/hexagon/agent"
)

func main() {
	fmt.Println("=== 示例 1: 共识策略类型 ===")
	runStrategies()

	fmt.Println("\n=== 示例 2: 投票数据结构 ===")
	runVoting()

	fmt.Println("\n=== 示例 3: 共识结果分析 ===")
	runResultAnalysis()
}

// runStrategies 演示共识策略类型
func runStrategies() {
	strategies := []agent.ConsensusStrategy{
		agent.ConsensusMajority,
		agent.ConsensusUnanimous,
		agent.ConsensusWeighted,
		agent.ConsensusAverage,
		agent.ConsensusBorda,
		agent.ConsensusFirst,
		agent.ConsensusBest,
	}

	for _, s := range strategies {
		fmt.Printf("  策略: %s\n", s.String())
	}
}

// runVoting 演示投票数据结构
func runVoting() {
	// 模拟三个 Agent 的投票
	votes := []agent.Vote{
		{
			AgentID:   "agent-1",
			AgentName: "分析师",
			Value:     "方案A",
			Weight:    1.0,
			Score:     0.85,
			Reason:    "方案A成本低，实施简单",
			Timestamp: time.Now(),
		},
		{
			AgentID:   "agent-2",
			AgentName: "架构师",
			Value:     "方案B",
			Weight:    1.5,
			Score:     0.92,
			Reason:    "方案B可扩展性更好",
			Timestamp: time.Now(),
		},
		{
			AgentID:   "agent-3",
			AgentName: "产品经理",
			Value:     "方案A",
			Weight:    1.0,
			Score:     0.78,
			Reason:    "方案A更符合用户需求",
			Timestamp: time.Now(),
		},
	}

	fmt.Printf("  共 %d 票:\n", len(votes))
	for _, v := range votes {
		fmt.Printf("    %s (%s): %v (权重=%.1f, 分数=%.2f)\n",
			v.AgentName, v.AgentID, v.Value, v.Weight, v.Score)
		fmt.Printf("      理由: %s\n", v.Reason)
	}
}

// runResultAnalysis 演示共识结果分析
func runResultAnalysis() {
	// 构造模拟结果
	result := &agent.ConsensusResult{
		Strategy: agent.ConsensusMajority,
		Decision: "方案A",
		Reached:  true,
		Reason:   "方案A获得 2/3 多数票",
		Votes: []agent.Vote{
			{AgentID: "agent-1", Value: "方案A", Weight: 1.0},
			{AgentID: "agent-2", Value: "方案B", Weight: 1.5},
			{AgentID: "agent-3", Value: "方案A", Weight: 1.0},
		},
		VoteCount: map[string]int{
			"方案A": 2,
			"方案B": 1,
		},
		Participation: 1.0,
		Duration:      500 * time.Millisecond,
	}

	fmt.Printf("  策略: %s\n", result.Strategy.String())
	fmt.Printf("  达成共识: %v\n", result.Reached)
	fmt.Printf("  最终决策: %v\n", result.Decision)
	fmt.Printf("  理由: %s\n", result.Reason)
	fmt.Printf("  参与率: %.0f%%\n", result.Participation*100)
	fmt.Printf("  投票统计:\n")
	for option, count := range result.VoteCount {
		fmt.Printf("    %s: %d 票\n", option, count)
	}
	fmt.Printf("  耗时: %v\n", result.Duration)
}
