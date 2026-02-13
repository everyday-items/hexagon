// Package main 演示 Hexagon 规划器
//
// 规划器负责将复杂任务分解为可执行的步骤序列。
// 本示例展示如何手动构建执行计划并检查其结构。
//
// 运行方式:
//
//	go run ./examples/planner/
package main

import (
	"fmt"
	"time"

	"github.com/everyday-items/hexagon/orchestration/planner"
)

func main() {
	fmt.Println("=== 示例 1: 构建执行计划 ===")
	runBuildPlan()

	fmt.Println("\n=== 示例 2: 带依赖的计划 ===")
	runDependencyPlan()
}

// runBuildPlan 演示手动构建执行计划
func runBuildPlan() {
	plan := &planner.Plan{
		ID:        "plan-001",
		Goal:      "完成用户注册流程",
		State:     planner.PlanStatePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "验证用户输入参数",
				Action: &planner.Action{
					Type:       planner.ActionTypeTool,
					Name:       "validator",
					Parameters: map[string]any{"fields": []string{"email", "password"}},
				},
				State: planner.StepStatePending,
			},
			{
				ID:          "step-2",
				Index:       1,
				Description: "创建用户账号",
				Action: &planner.Action{
					Type:       planner.ActionTypeTool,
					Name:       "user_service",
					Parameters: map[string]any{"action": "create"},
				},
				State: planner.StepStatePending,
			},
			{
				ID:          "step-3",
				Index:       2,
				Description: "发送欢迎邮件",
				Action: &planner.Action{
					Type:       planner.ActionTypeTool,
					Name:       "email_service",
					Parameters: map[string]any{"template": "welcome"},
				},
				State: planner.StepStatePending,
			},
		},
	}

	fmt.Printf("  计划: %s\n", plan.Goal)
	fmt.Printf("  状态: %s\n", plan.State)
	fmt.Printf("  步骤数: %d\n", len(plan.Steps))
	for _, step := range plan.Steps {
		fmt.Printf("    [%d] %s → 工具: %s\n", step.Index, step.Description, step.Action.Name)
	}
}

// runDependencyPlan 演示带依赖关系的计划
func runDependencyPlan() {
	plan := &planner.Plan{
		ID:        "plan-002",
		Goal:      "部署微服务应用",
		State:     planner.PlanStatePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "构建 Docker 镜像",
				Action: &planner.Action{
					Type: planner.ActionTypeTool,
					Name: "docker_build",
				},
				State: planner.StepStatePending,
			},
			{
				ID:           "step-2",
				Index:        1,
				Description:  "运行单元测试",
				Dependencies: []string{"step-1"},
				Action: &planner.Action{
					Type: planner.ActionTypeTool,
					Name: "test_runner",
				},
				State: planner.StepStatePending,
			},
			{
				ID:           "step-3",
				Index:        2,
				Description:  "推送镜像到仓库",
				Dependencies: []string{"step-1", "step-2"},
				Action: &planner.Action{
					Type: planner.ActionTypeTool,
					Name: "docker_push",
				},
				State: planner.StepStatePending,
			},
			{
				ID:           "step-4",
				Index:        3,
				Description:  "部署到 Kubernetes",
				Dependencies: []string{"step-3"},
				Action: &planner.Action{
					Type: planner.ActionTypeTool,
					Name: "k8s_deploy",
				},
				State: planner.StepStatePending,
			},
		},
	}

	fmt.Printf("  计划: %s\n", plan.Goal)
	for _, step := range plan.Steps {
		deps := "无"
		if len(step.Dependencies) > 0 {
			deps = fmt.Sprintf("%v", step.Dependencies)
		}
		fmt.Printf("    [%d] %s (依赖: %s)\n", step.Index, step.Description, deps)
	}
}
