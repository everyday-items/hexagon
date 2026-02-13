// Package main 演示 Hexagon 工作流引擎
//
// 工作流引擎支持多步骤任务编排，包括：
//   - 顺序执行: 步骤按序执行，上一步输出传递给下一步
//   - 并行执行: 多个步骤并行运行，提高吞吐量
//   - 条件分支: 根据运行时条件动态选择执行路径
//
// 运行方式:
//
//	go run ./examples/workflow/
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/orchestration/workflow"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: 顺序工作流 (ETL 管道) ===")
	runSequentialWorkflow(ctx)

	fmt.Println("\n=== 示例 2: 并行工作流 ===")
	runParallelWorkflow(ctx)

	fmt.Println("\n=== 示例 3: 条件分支工作流 ===")
	runConditionalWorkflow(ctx)
}

// runSequentialWorkflow 演示顺序 ETL 管道: 提取 -> 转换 -> 加载
func runSequentialWorkflow(ctx context.Context) {
	wf, err := workflow.New("data-pipeline").
		WithDescription("数据处理管道").
		WithTimeout(30 * time.Second).
		AddFunc("extract", "提取数据", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [1/3] 提取数据...")
			data := input.Data.(string)
			return &workflow.StepOutput{
				Data:      strings.Split(data, ","),
				Variables: map[string]any{"step": "extract"},
			}, nil
		}).
		AddFunc("transform", "转换数据", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [2/3] 转换数据...")
			// PreviousOutputs 中的值保持原始类型（[]string）
			items := input.PreviousOutputs["extract"].([]string)
			var result []string
			for _, s := range items {
				result = append(result, strings.ToUpper(strings.TrimSpace(s)))
			}
			return &workflow.StepOutput{Data: result}, nil
		}).
		AddFunc("load", "加载数据", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [3/3] 加载数据...")
			items := input.PreviousOutputs["transform"].([]string)
			return &workflow.StepOutput{Data: fmt.Sprintf("成功加载 %d 条记录", len(items))}, nil
		}).
		Build()
	if err != nil {
		log.Fatalf("构建工作流失败: %v", err)
	}

	executor := workflow.NewExecutor()
	result, err := executor.Run(ctx, wf, workflow.WorkflowInput{Data: "apple, banana, cherry, date"})
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("  结果: %v\n", result.Data)
}

// runParallelWorkflow 演示并行分析: 情感、关键词、摘要同时执行
func runParallelWorkflow(ctx context.Context) {
	wf, err := workflow.New("parallel-analysis").
		AddFunc("prepare", "准备数据", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [准备] 准备数据...")
			return &workflow.StepOutput{Data: input.Data}, nil
		}).
		ParallelFuncs("analyze", "并行分析", map[string]workflow.StepFunc{
			"sentiment": func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				fmt.Println("  [并行] 情感分析...")
				time.Sleep(50 * time.Millisecond)
				return &workflow.StepOutput{Data: "positive"}, nil
			},
			"keywords": func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				fmt.Println("  [并行] 关键词提取...")
				time.Sleep(50 * time.Millisecond)
				return &workflow.StepOutput{Data: []string{"AI", "Agent", "Go"}}, nil
			},
		}).
		AddFunc("merge", "合并结果", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [合并] 合并分析结果...")
			// 并行步骤的输出存储在父步骤 ID 下，值为 map[string]any
			analyzeResult := input.PreviousOutputs["analyze"].(map[string]any)
			return &workflow.StepOutput{
				Data: fmt.Sprintf("情感=%v, 关键词=%v",
					analyzeResult["sentiment"],
					analyzeResult["keywords"]),
			}, nil
		}).
		Build()
	if err != nil {
		log.Fatalf("构建工作流失败: %v", err)
	}

	executor := workflow.NewExecutor()
	result, err := executor.Run(ctx, wf, workflow.WorkflowInput{Data: "Hexagon is powerful"})
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("  结果: %v\n", result.Data)
}

// runConditionalWorkflow 演示审核流程: 大额走经理审批，小额自动通过
func runConditionalWorkflow(ctx context.Context) {
	wf, err := workflow.New("review-process").
		AddFunc("classify", "分类请求", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			amount := input.Data.(float64)
			level := "low"
			if amount > 10000 {
				level = "high"
			}
			fmt.Printf("  [分类] 金额 %.0f → 级别: %s\n", amount, level)
			return &workflow.StepOutput{
				Data:      level,
				Variables: map[string]any{"level": level},
			}, nil
		}).
		Conditional("route", "路由审批",
			func(ctx context.Context, input workflow.StepInput) (string, error) {
				if input.Variables["level"] == "high" {
					return "true", nil
				}
				return "false", nil
			}).
			Then(workflow.NewStep("manager-review", "经理审批",
				func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
					fmt.Println("  [审批] 经理审批中...")
					return &workflow.StepOutput{Data: "经理已批准"}, nil
				})).
			Else(workflow.NewStep("auto-approve", "自动审批",
				func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
					fmt.Println("  [审批] 自动审批通过")
					return &workflow.StepOutput{Data: "自动批准"}, nil
				})).
			End().
		AddFunc("notify", "通知", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			fmt.Println("  [通知] 已发送通知")
			return &workflow.StepOutput{Data: "通知已发送"}, nil
		}).
		Build()
	if err != nil {
		log.Fatalf("构建工作流失败: %v", err)
	}

	executor := workflow.NewExecutor()

	fmt.Println("  --- 大额 (15000) ---")
	r1, _ := executor.Run(ctx, wf, workflow.WorkflowInput{Data: float64(15000)})
	fmt.Printf("  结果: %v\n", r1.Data)

	fmt.Println("  --- 小额 (500) ---")
	r2, _ := executor.Run(ctx, wf, workflow.WorkflowInput{Data: float64(500)})
	fmt.Printf("  结果: %v\n", r2.Data)
}
