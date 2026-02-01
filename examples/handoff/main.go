// Package main demonstrates the Agent Handoff pattern (inspired by OpenAI Swarm).
//
// This example shows how to create Agents that can transfer conversations
// to other specialized Agents based on the context.
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

	// 检查 API Key
	if os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Println("注意: 未设置 OPENAI_API_KEY，将展示 Handoff 模式概念")
		runConceptDemo()
		return
	}

	// ========================================
	// 示例: 客服 Swarm
	// ========================================
	fmt.Println("=== 客服 Swarm 示例 ===")
	runCustomerServiceSwarm(ctx)
}

// runConceptDemo 展示 Handoff 概念（不需要真实 LLM）
func runConceptDemo() {
	fmt.Println("\n=== Handoff 模式概念演示 ===")
	fmt.Println(`
Handoff (Agent 交接) 模式借鉴自 OpenAI Swarm:

1. 每个 Agent 专注于特定领域
2. Agent 可以将对话转交给更合适的 Agent
3. 转交通过特殊的 "transfer_to_xxx" 工具实现
4. SwarmRunner 自动处理转交逻辑

典型场景: 客服系统
┌─────────────┐
│   Triage    │  ← 入口 Agent，负责分类
│   Agent     │
└─────┬───────┘
      │
      ├──────────────┬──────────────┐
      ▼              ▼              ▼
┌───────────┐ ┌───────────┐ ┌───────────┐
│   Sales   │ │  Support  │ │  Billing  │
│   Agent   │ │   Agent   │ │   Agent   │
└───────────┘ └───────────┘ └───────────┘

代码示例:

  // 创建专业 Agent
  salesAgent := hexagon.QuickStart(
      hexagon.WithSystemPrompt("你是销售客服..."),
  )
  supportAgent := hexagon.QuickStart(
      hexagon.WithSystemPrompt("你是技术支持..."),
  )

  // Triage Agent 可以转交给其他 Agent
  triageAgent := hexagon.QuickStart(
      hexagon.WithSystemPrompt("你是入口客服，根据用户问题转交给合适的 Agent"),
      hexagon.WithTools(
          hexagon.TransferTo(salesAgent),   // 转交工具
          hexagon.TransferTo(supportAgent),
      ),
  )

  // 创建 Swarm 运行器
  runner := agent.NewSwarmRunner(triageAgent)
  runner.MaxHandoffs = 5  // 最多转交 5 次

  // 运行
  output, _ := runner.Run(ctx, hexagon.Input{
      Query: "我想了解产品价格",
  })
  // triageAgent 会自动转交给 salesAgent

TransferTo 工具会生成如下的工具定义:
  - Name: "transfer_to_销售客服"
  - Description: "Transfer the conversation to 销售客服. ..."

当 LLM 决定调用这个工具时，SwarmRunner 会:
  1. 检测到转交请求
  2. 切换到目标 Agent
  3. 将对话历史传递给新 Agent
  4. 继续执行直到完成或再次转交`)

	// 展示上下文变量
	fmt.Println("\n=== 上下文变量 ===")
	fmt.Println(`
上下文变量用于在 Agent 之间传递状态:

  // 创建上下文变量
  vars := agent.ContextVariables{
      "user_id":   "12345",
      "user_name": "张三",
      "vip_level": "gold",
  }

  // 添加到 context
  ctx = agent.ContextWithVariables(ctx, vars)

  // 在 Agent 中获取
  vars := agent.VariablesFromContext(ctx)
  userID, _ := vars.Get("user_id")

  // 更新变量
  ctx = agent.UpdateContextVariables(ctx, agent.ContextVariables{
      "last_topic": "pricing",
  })`)
}

// runCustomerServiceSwarm 运行客服 Swarm 示例
func runCustomerServiceSwarm(ctx context.Context) {
	// ========================================
	// 步骤 1: 创建专业 Agent
	// ========================================

	// 销售 Agent
	salesAgent := hexagon.QuickStart(
		hexagon.WithSystemPrompt(`你是销售客服。
你的职责是:
- 回答产品价格问题
- 介绍产品功能
- 处理购买咨询
- 提供优惠信息

如果用户问题不属于销售范畴，使用 transfer 工具转交给其他 Agent。`),
	)

	// 技术支持 Agent
	supportAgent := hexagon.QuickStart(
		hexagon.WithSystemPrompt(`你是技术支持客服。
你的职责是:
- 解决技术问题
- 提供使用指导
- 处理 Bug 报告
- 解释技术概念

如果用户问题不属于技术支持范畴，使用 transfer 工具转交给其他 Agent。`),
	)

	// 账单 Agent
	billingAgent := hexagon.QuickStart(
		hexagon.WithSystemPrompt(`你是账单客服。
你的职责是:
- 处理付款问题
- 解释账单明细
- 处理退款请求
- 更新支付方式

如果用户问题不属于账单范畴，使用 transfer 工具转交给其他 Agent。`),
	)

	// ========================================
	// 步骤 2: 创建 Triage Agent (带转交工具)
	// ========================================

	// 为 Triage Agent 添加转交工具
	triageAgent := hexagon.QuickStart(
		hexagon.WithSystemPrompt(`你是客服入口 Agent。
你的职责是:
1. 理解用户问题
2. 判断问题类型
3. 转交给合适的专业 Agent:
   - 产品、价格、购买问题 -> 销售客服
   - 技术、使用、Bug 问题 -> 技术支持
   - 付款、账单、退款问题 -> 账单客服

收到用户消息后，分析问题类型并立即转交，不要自己回答专业问题。`),
		hexagon.WithTools(
			hexagon.TransferTo(salesAgent),
			hexagon.TransferTo(supportAgent),
			hexagon.TransferTo(billingAgent),
		),
	)

	// 为专业 Agent 也添加转交能力（可以互相转交）
	// 注：在真实场景中，你可能需要重新创建 Agent 来添加工具

	// ========================================
	// 步骤 3: 创建 Swarm 运行器
	// ========================================

	runner := agent.NewSwarmRunner(triageAgent)
	runner.MaxHandoffs = 5 // 最多 5 次转交
	runner.Verbose = true  // 显示转交过程

	// ========================================
	// 步骤 4: 运行不同类型的查询
	// ========================================

	queries := []string{
		"你们的产品多少钱？",
		"软件安装后打不开怎么办？",
		"我想申请退款",
	}

	for _, query := range queries {
		fmt.Printf("\n用户: %s\n", query)
		fmt.Println("---")

		output, err := runner.Run(ctx, hexagon.Input{
			Query: query,
		})

		if err != nil {
			log.Printf("执行失败: %v\n", err)
			continue
		}

		fmt.Printf("回复: %s\n", output.Content)
	}
}
