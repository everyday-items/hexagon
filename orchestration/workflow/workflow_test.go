package workflow

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkflowBuilder(t *testing.T) {
	// 创建简单的工作流
	wf, err := New("test-workflow").
		WithDescription("测试工作流").
		WithVersion("1.0.0").
		AddFunc("step1", "Step 1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "step1 done"}, nil
		}).
		AddFunc("step2", "Step 2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: input.Data.(string) + " -> step2 done"}, nil
		}).
		Build()

	if err != nil {
		t.Fatalf("failed to build workflow: %v", err)
	}

	if wf.Name != "test-workflow" {
		t.Errorf("expected name 'test-workflow', got '%s'", wf.Name)
	}

	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}
}

func TestExecutor_Run(t *testing.T) {
	// 创建工作流
	wf, _ := New("test-workflow").
		AddFunc("step1", "Step 1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "hello"}, nil
		}).
		AddFunc("step2", "Step 2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: input.Data.(string) + " world"}, nil
		}).
		Build()

	// 创建执行器
	executor := NewExecutor()

	// 运行工作流
	ctx := context.Background()
	output, err := executor.Run(ctx, wf, WorkflowInput{Data: "start"})

	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}

	if output.Data != "hello world" {
		t.Errorf("expected 'hello world', got '%v'", output.Data)
	}
}

func TestExecutor_Parallel(t *testing.T) {
	var count int32

	// 创建并行步骤
	step1 := NewStep("p1", "Parallel 1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		atomic.AddInt32(&count, 1)
		time.Sleep(10 * time.Millisecond)
		return &StepOutput{Data: "p1"}, nil
	})
	step2 := NewStep("p2", "Parallel 2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		atomic.AddInt32(&count, 1)
		time.Sleep(10 * time.Millisecond)
		return &StepOutput{Data: "p2"}, nil
	})

	wf, _ := New("parallel-workflow").
		Parallel("parallel", "Parallel Steps", step1, step2).
		Build()

	executor := NewExecutor()
	_, err := executor.Run(context.Background(), wf, WorkflowInput{})

	if err != nil {
		t.Fatalf("parallel execution failed: %v", err)
	}

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("expected 2 parallel executions, got %d", count)
	}
}

func TestExecutor_Conditional(t *testing.T) {
	wf, _ := New("conditional-workflow").
		Conditional("branch", "Conditional Branch",
			func(ctx context.Context, input StepInput) (string, error) {
				if input.Data.(bool) {
					return "true", nil
				}
				return "false", nil
			}).
		ThenFunc("then", "Then Branch", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "then executed"}, nil
		}).
		ElseFunc("else", "Else Branch", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "else executed"}, nil
		}).
		End().
		Build()

	executor := NewExecutor()

	// Test true branch
	output, err := executor.Run(context.Background(), wf, WorkflowInput{Data: true})
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}
	if output.Data != "then executed" {
		t.Errorf("expected 'then executed', got '%v'", output.Data)
	}

	// Test false branch
	output, err = executor.Run(context.Background(), wf, WorkflowInput{Data: false})
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}
	if output.Data != "else executed" {
		t.Errorf("expected 'else executed', got '%v'", output.Data)
	}
}

func TestExecutor_Timeout(t *testing.T) {
	wf, _ := New("timeout-workflow").
		WithTimeout(50 * time.Millisecond).
		AddFunc("slow", "Slow Step", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return &StepOutput{Data: "done"}, nil
			}
		}).
		Build()

	executor := NewExecutor(WithExecutorConfig(ExecutorConfig{
		DefaultTimeout: 50 * time.Millisecond,
	}))

	_, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestExecutor_StepError(t *testing.T) {
	expectedErr := errors.New("step failed")

	wf, _ := New("error-workflow").
		AddFunc("fail", "Failing Step", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return nil, expectedErr
		}).
		Build()

	executor := NewExecutor()
	_, err := executor.Run(context.Background(), wf, WorkflowInput{})

	if err == nil {
		t.Error("expected error")
	}
}

func TestLoopStep(t *testing.T) {
	iterations := 0

	loopBody := NewStep("body", "Loop Body", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		iterations++
		return &StepOutput{Data: iterations}, nil
	})

	loopStep := NewLoopStep("loop", "Loop", loopBody,
		func(ctx context.Context, input StepInput) (string, error) {
			if input.Data.(int) >= 3 {
				return "break", nil
			}
			return "continue", nil
		},
		WithMaxIterations(10),
	)

	wf, _ := New("loop-workflow").
		Add(loopStep).
		Build()

	executor := NewExecutor()
	_, err := executor.Run(context.Background(), wf, WorkflowInput{Data: 0})

	if err != nil {
		t.Fatalf("loop execution failed: %v", err)
	}

	if iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", iterations)
	}
}

func TestWaitStep(t *testing.T) {
	start := time.Now()

	wf, _ := New("wait-workflow").
		Wait("wait", "Wait Step", 50*time.Millisecond).
		Build()

	executor := NewExecutor()
	_, err := executor.Run(context.Background(), wf, WorkflowInput{})

	if err != nil {
		t.Fatalf("wait execution failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Errorf("wait was too short: %v", elapsed)
	}
}

func TestMemoryWorkflowStore(t *testing.T) {
	store := NewMemoryWorkflowStore()
	ctx := context.Background()

	// 保存工作流
	wf := &Workflow{
		ID:   "test-wf",
		Name: "Test Workflow",
	}
	err := store.SaveWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("save workflow failed: %v", err)
	}

	// 获取工作流
	loaded, err := store.GetWorkflow(ctx, "test-wf")
	if err != nil {
		t.Fatalf("get workflow failed: %v", err)
	}
	if loaded.Name != "Test Workflow" {
		t.Errorf("expected 'Test Workflow', got '%s'", loaded.Name)
	}

	// 列出工作流
	workflows, err := store.ListWorkflows(ctx, 10)
	if err != nil {
		t.Fatalf("list workflows failed: %v", err)
	}
	if len(workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(workflows))
	}

	// 删除工作流
	err = store.DeleteWorkflow(ctx, "test-wf")
	if err != nil {
		t.Fatalf("delete workflow failed: %v", err)
	}

	workflows, _ = store.ListWorkflows(ctx, 10)
	if len(workflows) != 0 {
		t.Errorf("expected 0 workflows after delete, got %d", len(workflows))
	}
}
