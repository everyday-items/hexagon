// Package main demonstrates the multi-Agent Team collaboration.
//
// This example shows how to create Agent teams with different work modes:
// - Sequential: Agents execute one after another
// - Hierarchical: A manager coordinates the team
// - Collaborative: Agents work in parallel
// - RoundRobin: Agents take turns until done
//
// Usage:
//
//	export OPENAI_API_KEY=your-api-key
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/everyday-items/hexagon"
	"github.com/everyday-items/hexagon/agent"
)

func main() {
	ctx := context.Background()

	// 检查 API Key（真实 LLM 调用时需要）
	hasAPIKey := os.Getenv("OPENAI_API_KEY") != ""

	if !hasAPIKey {
		fmt.Println("注意: 未设置 OPENAI_API_KEY，将使用模拟 Agent 演示")
	}

	// ========================================
	// 示例 1: 顺序执行模式
	// ========================================
	fmt.Println("=== 示例 1: 顺序执行模式 (Sequential) ===")
	runSequentialTeam(ctx, hasAPIKey)

	// ========================================
	// 示例 2: 层级模式
	// ========================================
	fmt.Println("\n=== 示例 2: 层级模式 (Hierarchical) ===")
	runHierarchicalTeam(ctx, hasAPIKey)

	// ========================================
	// 示例 3: 协作模式
	// ========================================
	fmt.Println("\n=== 示例 3: 协作模式 (Collaborative) ===")
	runCollaborativeTeam(ctx, hasAPIKey)

	// ========================================
	// 示例 4: 轮询模式
	// ========================================
	fmt.Println("\n=== 示例 4: 轮询模式 (RoundRobin) ===")
	runRoundRobinTeam(ctx, hasAPIKey)
}

// createMockAgent 创建模拟 Agent（用于演示，不需要真实 LLM）
func createMockAgent(name, role string) *agent.ReActAgent {
	return agent.NewReAct(
		agent.WithName(name),
		agent.WithDescription(role),
		agent.WithSystemPrompt(fmt.Sprintf("你是%s，负责%s", name, role)),
	)
}

// runSequentialTeam 演示顺序执行模式
// 适用于：流水线式的任务，前一步的输出是后一步的输入
func runSequentialTeam(ctx context.Context, hasAPIKey bool) {
	fmt.Println("  顺序模式: Agent 按顺序依次执行，前一个的输出作为后一个的输入")
	fmt.Println("  适用场景: 内容生成流水线 (研究 -> 写作 -> 审核)")

	// 创建 Agent 团队
	researcher := createMockAgent("研究员", "收集和分析信息")
	writer := createMockAgent("作家", "撰写内容")
	reviewer := createMockAgent("审核员", "审核和改进内容")

	team := hexagon.NewTeam("content-team",
		hexagon.WithAgents(researcher, writer, reviewer),
		hexagon.WithMode(hexagon.TeamModeSequential),
		hexagon.WithTeamDescription("内容创作团队"),
	)

	fmt.Printf("  团队成员: %d 个\n", len(team.Agents()))
	fmt.Printf("  工作模式: %s\n", team.Mode())

	// 在真实场景中执行
	if hasAPIKey {
		output, err := team.Run(ctx, hexagon.Input{
			Query: "写一篇关于 Go 语言并发特性的短文",
		})
		if err != nil {
			log.Printf("  执行失败: %v\n", err)
			return
		}
		fmt.Printf("  输出: %s\n", output.Content)
	} else {
		fmt.Println("  (跳过执行，需要设置 OPENAI_API_KEY)")
	}
}

// runHierarchicalTeam 演示层级模式
// 适用于：需要协调和分配任务的复杂场景
func runHierarchicalTeam(ctx context.Context, hasAPIKey bool) {
	fmt.Println("  层级模式: Manager Agent 协调和分配任务给其他 Agent")
	fmt.Println("  适用场景: 项目管理、复杂任务分解")

	// 创建 Manager 和团队成员
	manager := createMockAgent("项目经理", "协调团队工作，分配任务")
	frontend := createMockAgent("前端开发", "处理 UI 和用户交互")
	backend := createMockAgent("后端开发", "处理 API 和数据库")
	tester := createMockAgent("测试工程师", "编写和执行测试")

	team := hexagon.NewTeam("dev-team",
		hexagon.WithAgents(frontend, backend, tester),
		hexagon.WithManager(manager), // 设置 Manager 会自动切换到 Hierarchical 模式
		hexagon.WithTeamDescription("软件开发团队"),
	)

	fmt.Printf("  团队成员: %d 个 (+ 1 Manager)\n", len(team.Agents()))
	fmt.Printf("  工作模式: %s\n", team.Mode())

	if hasAPIKey {
		output, err := team.Run(ctx, hexagon.Input{
			Query: "实现一个用户登录功能",
		})
		if err != nil {
			log.Printf("  执行失败: %v\n", err)
			return
		}
		fmt.Printf("  输出: %s\n", output.Content)
	} else {
		fmt.Println("  (跳过执行，需要设置 OPENAI_API_KEY)")
	}
}

// runCollaborativeTeam 演示协作模式
// 适用于：多个 Agent 可以并行工作的场景
func runCollaborativeTeam(ctx context.Context, hasAPIKey bool) {
	fmt.Println("  协作模式: 所有 Agent 并行工作，通过消息传递协作")
	fmt.Println("  适用场景: 多角度分析、头脑风暴")

	// 创建多个分析师
	marketAnalyst := createMockAgent("市场分析师", "分析市场趋势和竞争态势")
	techAnalyst := createMockAgent("技术分析师", "评估技术可行性和风险")
	financeAnalyst := createMockAgent("财务分析师", "进行成本效益分析")

	team := hexagon.NewTeam("analysis-team",
		hexagon.WithAgents(marketAnalyst, techAnalyst, financeAnalyst),
		hexagon.WithMode(hexagon.TeamModeCollaborative),
		hexagon.WithTeamDescription("多维度分析团队"),
	)

	fmt.Printf("  团队成员: %d 个\n", len(team.Agents()))
	fmt.Printf("  工作模式: %s\n", team.Mode())

	if hasAPIKey {
		output, err := team.Run(ctx, hexagon.Input{
			Query: "评估进入 AI Agent 市场的可行性",
		})
		if err != nil {
			log.Printf("  执行失败: %v\n", err)
			return
		}
		fmt.Printf("  输出: %s\n", output.Content)
	} else {
		fmt.Println("  (跳过执行，需要设置 OPENAI_API_KEY)")
	}
}

// runRoundRobinTeam 演示轮询模式
// 适用于：需要迭代改进的场景
func runRoundRobinTeam(ctx context.Context, hasAPIKey bool) {
	fmt.Println("  轮询模式: Agent 轮流执行，直到达到目标或最大轮次")
	fmt.Println("  适用场景: 代码审查、迭代改进")

	// 创建代码审查团队
	coder := createMockAgent("开发者", "编写和修改代码")
	reviewer1 := createMockAgent("审查者A", "进行代码审查")
	reviewer2 := createMockAgent("审查者B", "进行安全审查")

	team := hexagon.NewTeam("review-team",
		hexagon.WithAgents(coder, reviewer1, reviewer2),
		hexagon.WithMode(hexagon.TeamModeRoundRobin),
		hexagon.WithMaxRounds(3), // 最多 3 轮
		hexagon.WithTeamDescription("代码审查团队"),
	)

	fmt.Printf("  团队成员: %d 个\n", len(team.Agents()))
	fmt.Printf("  工作模式: %s\n", team.Mode())
	fmt.Println("  最大轮次: 3")

	if hasAPIKey {
		output, err := team.Run(ctx, hexagon.Input{
			Query: "审查这段代码: func add(a, b int) int { return a + b }",
		})
		if err != nil {
			log.Printf("  执行失败: %v\n", err)
			return
		}
		fmt.Printf("  输出: %s\n", output.Content)
	} else {
		fmt.Println("  (跳过执行，需要设置 OPENAI_API_KEY)")
	}
}
