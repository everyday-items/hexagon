package bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/orchestration/workflow"
)

// BenchmarkWorkflowCreation 测试工作流创建性能
func BenchmarkWorkflowCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		workflow.New("bench-wf").
			AddFunc("step1", "步骤1", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				return &workflow.StepOutput{Data: input.Data}, nil
			}).
			AddFunc("step2", "步骤2", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				return &workflow.StepOutput{Data: input.Data}, nil
			}).
			Build()
	}
}

// BenchmarkWorkflowRun 测试工作流执行性能（3 步顺序）
func BenchmarkWorkflowRun(b *testing.B) {
	wf, _ := workflow.New("bench-wf").
		AddFunc("step1", "步骤1", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		}).
		AddFunc("step2", "步骤2", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		}).
		AddFunc("step3", "步骤3", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		}).
		Build()
	executor := workflow.NewExecutor()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		executor.Run(ctx, wf, workflow.WorkflowInput{Data: "input"})
	}
}

// BenchmarkWorkflowRunLong 测试长工作流执行性能（10 步）
func BenchmarkWorkflowRunLong(b *testing.B) {
	builder := workflow.New("bench-wf-10").WithTimeout(30 * time.Second)
	for j := 0; j < 10; j++ {
		id := fmt.Sprintf("step-%d", j)
		builder = builder.AddFunc(id, id, func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		})
	}
	wf, _ := builder.Build()
	executor := workflow.NewExecutor()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		executor.Run(ctx, wf, workflow.WorkflowInput{Data: "input"})
	}
}

// BenchmarkWorkflowConditional 测试条件分支工作流性能
func BenchmarkWorkflowConditional(b *testing.B) {
	wf, _ := workflow.New("bench-cond").
		AddFunc("check", "检查", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{
				Data:      input.Data,
				Variables: map[string]any{"branch": "true"},
			}, nil
		}).
		Conditional("route", "路由",
			func(ctx context.Context, input workflow.StepInput) (string, error) {
				if input.Variables["branch"] == "true" {
					return "true", nil
				}
				return "false", nil
			}).
			ThenFunc("yes", "是", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				return &workflow.StepOutput{Data: "yes"}, nil
			}).
			ElseFunc("no", "否", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
				return &workflow.StepOutput{Data: "no"}, nil
			}).
			End().
		Build()
	executor := workflow.NewExecutor()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		executor.Run(ctx, wf, workflow.WorkflowInput{Data: "input"})
	}
}

// BenchmarkWorkflowRunConcurrent 测试并发工作流执行性能
func BenchmarkWorkflowRunConcurrent(b *testing.B) {
	wf, _ := workflow.New("bench-concurrent").
		AddFunc("process", "处理", func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		}).
		Build()
	executor := workflow.NewExecutor()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			executor.Run(ctx, wf, workflow.WorkflowInput{Data: "input"})
		}
	})
}
