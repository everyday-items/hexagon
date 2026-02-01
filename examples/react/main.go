// Package main demonstrates the ReAct Agent with tools.
//
// This example shows how to create a ReAct Agent that can use tools
// to solve problems step by step.
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
)

// CalculatorInput 定义计算器工具的输入参数
type CalculatorInput struct {
	A  float64 `json:"a" desc:"第一个数字" required:"true"`
	B  float64 `json:"b" desc:"第二个数字" required:"true"`
	Op string  `json:"op" desc:"运算符: add, sub, mul, div" required:"true" enum:"add,sub,mul,div"`
}

func main() {
	// 检查 API Key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("请设置 OPENAI_API_KEY 环境变量")
	}

	ctx := context.Background()

	// 创建计算器工具
	calculatorTool := hexagon.NewTool("calculator", "执行数学计算",
		func(ctx context.Context, input CalculatorInput) (float64, error) {
			switch input.Op {
			case "add":
				return input.A + input.B, nil
			case "sub":
				return input.A - input.B, nil
			case "mul":
				return input.A * input.B, nil
			case "div":
				if input.B == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return input.A / input.B, nil
			default:
				return 0, fmt.Errorf("unknown operator: %s", input.Op)
			}
		},
	)

	// 创建带工具的 ReAct Agent
	agent := hexagon.QuickStart(
		hexagon.WithTools(calculatorTool),
		hexagon.WithSystemPrompt("你是一个数学助手，可以使用计算器工具帮助用户解决数学问题。"),
	)

	// 执行查询
	output, err := agent.Run(ctx, hexagon.Input{
		Query: "请计算 123 乘以 456，然后告诉我结果",
	})
	if err != nil {
		log.Fatalf("Agent run failed: %v", err)
	}

	// 输出结果
	fmt.Println("=== Agent Response ===")
	fmt.Println(output.Content)

	// 显示工具调用记录
	if len(output.ToolCalls) > 0 {
		fmt.Println("\n=== Tool Calls ===")
		for _, tc := range output.ToolCalls {
			fmt.Printf("- %s(%v) -> %s\n", tc.Name, tc.Arguments, tc.Result.String())
		}
	}

	// 显示 Token 使用
	fmt.Printf("\n=== Token Usage ===\n")
	fmt.Printf("Prompt: %d, Completion: %d, Total: %d\n",
		output.Usage.PromptTokens, output.Usage.CompletionTokens, output.Usage.TotalTokens)
}
