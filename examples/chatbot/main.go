// Package main 演示完整客服 Bot 场景
//
// 本示例展示如何使用 Hexagon 构建一个完整的智能客服系统：
//
//   - 多轮对话: 使用记忆维护上下文
//   - 知识库检索: 从 FAQ 中检索答案
//   - 意图识别: 根据用户消息路由到不同处理流程
//   - Agent Handoff: 复杂问题转交人工客服
//
// 运行方式:
//
//	export OPENAI_API_KEY=your-key
//	go run ./examples/chatbot
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/everyday-items/ai-core/llm/openai"
	coretool "github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/agent"
)

func main() {
	ctx := context.Background()

	// 1. 初始化 LLM
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("请设置 OPENAI_API_KEY 环境变量")
	}
	provider := openai.New(apiKey)

	// 2. 定义客服工具
	queryFAQ := coretool.NewFunc("query_faq", "查询 FAQ 知识库",
		func(_ context.Context, input struct {
			Question string `json:"question" description:"用户问题"`
		}) (string, error) {
			// 模拟 FAQ 检索
			faqs := map[string]string{
				"退款":  "退款申请将在 3-5 个工作日内处理，退款将原路返回。",
				"配送":  "标准配送 3-5 天，快速配送 1-2 天。免费配送门槛为 99 元。",
				"账号":  "您可以在设置页面修改密码、绑定手机、更换邮箱。",
				"优惠":  "新用户首单享 8 折优惠，VIP 会员全场 9 折。",
				"投诉":  "我们非常重视您的反馈，投诉将在 24 小时内由专人跟进处理。",
			}

			question := strings.ToLower(input.Question)
			for keyword, answer := range faqs {
				if strings.Contains(question, keyword) {
					return answer, nil
				}
			}
			return "抱歉，未找到相关 FAQ，我将为您转接人工客服。", nil
		},
	)

	checkOrder := coretool.NewFunc("check_order", "查询订单状态",
		func(_ context.Context, input struct {
			OrderID string `json:"order_id" description:"订单号"`
		}) (string, error) {
			return fmt.Sprintf("订单 %s 状态：已发货，预计明天送达。", input.OrderID), nil
		},
	)

	transferHuman := coretool.NewFunc("transfer_human", "转接人工客服",
		func(_ context.Context, input struct {
			Reason string `json:"reason" description:"转接原因"`
		}) (string, error) {
			return fmt.Sprintf("已为您创建人工客服工单，原因：%s。客服将在 5 分钟内联系您。", input.Reason), nil
		},
	)

	// 3. 创建客服 Agent
	bot := agent.NewBaseAgent(
		agent.WithName("小六客服"),
		agent.WithLLM(provider),
		agent.WithSystemPrompt(`你是"小六客服"，一个专业友好的智能客服助手。

职责：
1. 回答用户关于产品、订单、配送、退款的问题
2. 使用 query_faq 工具查询知识库获取标准答案
3. 使用 check_order 工具查询订单状态
4. 遇到无法处理的问题，使用 transfer_human 转接人工

回复要求：
- 友好、专业、简洁
- 先理解用户意图，再选择合适的工具
- 如果用户情绪不好，先表示理解和歉意`),
		agent.WithTools(queryFAQ, checkOrder, transferHuman),
	)

	// 4. 模拟对话
	conversations := []string{
		"你好，我想问一下配送要多久？",
		"我有个订单 ORD-20240101 想查一下状态",
		"退款需要多长时间？",
		"你们的服务太差了，我要投诉！",
	}

	fmt.Println("=== 小六客服 - 智能客服演示 ===")
	fmt.Println()

	for _, msg := range conversations {
		fmt.Printf("用户: %s\n", msg)

		input := agent.Input{
			Query: msg,
		}

		output, err := bot.Run(ctx, input)
		if err != nil {
			fmt.Printf("错误: %v\n\n", err)
			continue
		}

		fmt.Printf("小六: %s\n\n", output.Content)
	}

	fmt.Println("=== 演示结束 ===")
}
