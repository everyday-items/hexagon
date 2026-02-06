package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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

// ============== DefaultRetryPolicy ==============

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", p.MaxRetries)
	}
	if p.InitialInterval != time.Second {
		t.Errorf("expected InitialInterval 1s, got %v", p.InitialInterval)
	}
	if p.MaxInterval != time.Minute {
		t.Errorf("expected MaxInterval 1m, got %v", p.MaxInterval)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %f", p.Multiplier)
	}
}

// ============== WorkflowRegistry ==============

func TestWorkflowRegistry(t *testing.T) {
	reg := NewWorkflowRegistry()

	// Register
	wf := &Workflow{ID: "wf-1", Name: "W1"}
	if err := reg.Register(wf); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Register with empty ID
	if err := reg.Register(&Workflow{}); err == nil {
		t.Error("expected error for empty ID")
	}

	// Get
	got, ok := reg.Get("wf-1")
	if !ok || got.Name != "W1" {
		t.Errorf("Get: expected W1, got %v", got)
	}
	_, ok = reg.Get("missing")
	if ok {
		t.Error("expected not found")
	}

	// List
	reg.Register(&Workflow{ID: "wf-2", Name: "W2"})
	list := reg.List()
	if len(list) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(list))
	}

	// Remove
	reg.Remove("wf-1")
	_, ok = reg.Get("wf-1")
	if ok {
		t.Error("expected removed")
	}
}

// ============== NewExecution ==============

func TestNewExecution(t *testing.T) {
	wf := &Workflow{ID: "wf-test", Name: "Test"}
	exec := NewExecution(wf)
	if exec.WorkflowID != "wf-test" {
		t.Errorf("expected WorkflowID 'wf-test', got %s", exec.WorkflowID)
	}
	if exec.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", exec.Status)
	}
	if exec.StepResults == nil {
		t.Error("StepResults should be initialized")
	}
	if exec.Context == nil || exec.Context.Variables == nil {
		t.Error("Context should be initialized")
	}
}

// ============== Builder 扩展 ==============

func TestBuilder_WithID(t *testing.T) {
	wf, err := New("test").
		WithID("my-id").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if wf.ID != "my-id" {
		t.Errorf("expected ID 'my-id', got %s", wf.ID)
	}
}

func TestBuilder_WithRetryPolicy(t *testing.T) {
	policy := &RetryPolicy{MaxRetries: 5}
	wf, err := New("test").
		WithRetryPolicy(policy).
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if wf.RetryPolicy.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", wf.RetryPolicy.MaxRetries)
	}
}

func TestBuilder_WithMetadata(t *testing.T) {
	wf, err := New("test").
		WithMetadata("env", "prod").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if wf.Metadata["env"] != "prod" {
		t.Errorf("expected metadata env=prod")
	}
}

func TestBuilder_Sequential(t *testing.T) {
	s1 := NewStep("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: "a"}, nil
	})
	s2 := NewStep("s2", "S2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: "b"}, nil
	})
	wf, err := New("seq").Sequential(s1, s2).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}
}

func TestBuilder_ParallelFuncs(t *testing.T) {
	wf, err := New("pf").
		ParallelFuncs("pf-step", "ParFuncs", map[string]StepFunc{
			"a": func(ctx context.Context, input StepInput) (*StepOutput, error) {
				return &StepOutput{Data: "a"}, nil
			},
			"b": func(ctx context.Context, input StepInput) (*StepOutput, error) {
				return &StepOutput{Data: "b"}, nil
			},
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	out, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected output")
	}
}

func TestBuilder_MustBuild_Success(t *testing.T) {
	wf := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		MustBuild()
	if wf.Name != "test" {
		t.Errorf("expected name 'test'")
	}
}

func TestBuilder_MustBuild_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustBuild")
		}
	}()
	// nil step 触发 build error
	New("test").Add(nil).MustBuild()
}

func TestBuilder_AddNilStep(t *testing.T) {
	_, err := New("test").Add(nil).Build()
	if err == nil {
		t.Error("expected error for nil step")
	}
}

// ============== ConditionalBuilder.Branch ==============

func TestConditionalBuilder_Branch(t *testing.T) {
	wf, err := New("branching").
		Conditional("cond", "Cond", func(ctx context.Context, input StepInput) (string, error) {
			return input.Data.(string), nil
		}).
		Then(NewStep("t", "Then", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "then"}, nil
		})).
		Else(NewStep("e", "Else", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "else"}, nil
		})).
		Branch("custom", NewStep("c", "Custom", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "custom"}, nil
		})).
		End().
		Build()
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	out, err := executor.Run(context.Background(), wf, WorkflowInput{Data: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "custom" {
		t.Errorf("expected 'custom', got %v", out.Data)
	}
}

// ============== Pipeline / PipelineFuncs ==============

func TestPipeline(t *testing.T) {
	s1 := NewStep("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: "hello"}, nil
	})
	s2 := NewStep("s2", "S2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: input.Data.(string) + " world"}, nil
	})
	wf, err := Pipeline("pipe", s1, s2)
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	out, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "hello world" {
		t.Errorf("expected 'hello world', got %v", out.Data)
	}
}

func TestPipelineFuncs(t *testing.T) {
	wf, err := PipelineFuncs("pfuncs", []struct {
		ID   string
		Name string
		Fn   StepFunc
	}{
		{ID: "s1", Name: "S1", Fn: func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: 42}, nil
		}},
		{ID: "s2", Name: "S2", Fn: func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "done"}, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}
}

// ============== MapStep / FilterStep / ReduceStep ==============

func TestMapStep(t *testing.T) {
	step := MapStep("map", "Double", func(ctx context.Context, item any) (any, error) {
		return item.(float64) * 2, nil
	})
	out, err := step.Execute(context.Background(), StepInput{Data: []any{1.0, 2.0, 3.0}})
	if err != nil {
		t.Fatal(err)
	}
	results := out.Data.([]any)
	if results[2].(float64) != 6.0 {
		t.Errorf("expected 6.0, got %v", results[2])
	}
}

func TestMapStep_InvalidInput(t *testing.T) {
	step := MapStep("map", "Double", func(ctx context.Context, item any) (any, error) {
		return item, nil
	})
	_, err := step.Execute(context.Background(), StepInput{Data: "not array"})
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

func TestMapStep_ItemError(t *testing.T) {
	step := MapStep("map", "Fail", func(ctx context.Context, item any) (any, error) {
		return nil, errors.New("item error")
	})
	_, err := step.Execute(context.Background(), StepInput{Data: []any{1}})
	if err == nil {
		t.Error("expected error")
	}
}

func TestFilterStep(t *testing.T) {
	step := FilterStep("filter", "EvenOnly", func(item any) bool {
		return int(item.(float64))%2 == 0
	})
	out, err := step.Execute(context.Background(), StepInput{Data: []any{1.0, 2.0, 3.0, 4.0}})
	if err != nil {
		t.Fatal(err)
	}
	results := out.Data.([]any)
	if len(results) != 2 {
		t.Errorf("expected 2 items, got %d", len(results))
	}
}

func TestFilterStep_InvalidInput(t *testing.T) {
	step := FilterStep("f", "F", func(item any) bool { return true })
	_, err := step.Execute(context.Background(), StepInput{Data: 42})
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

func TestReduceStep(t *testing.T) {
	step := ReduceStep("reduce", "Sum", 0.0, func(acc, item any) any {
		return acc.(float64) + item.(float64)
	})
	out, err := step.Execute(context.Background(), StepInput{Data: []any{1.0, 2.0, 3.0}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.(float64) != 6.0 {
		t.Errorf("expected 6.0, got %v", out.Data)
	}
}

func TestReduceStep_InvalidInput(t *testing.T) {
	step := ReduceStep("r", "R", 0, func(acc, item any) any { return acc })
	_, err := step.Execute(context.Background(), StepInput{Data: "not array"})
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

// ============== RetryWrapper / TimeoutWrapper ==============

func TestRetryWrapper_BaseStep(t *testing.T) {
	callCount := 0
	step := NewStep("s", "S", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("fail")
		}
		return &StepOutput{Data: "ok"}, nil
	})
	policy := &RetryPolicy{MaxRetries: 5, InitialInterval: time.Millisecond, MaxInterval: 10 * time.Millisecond, Multiplier: 2}
	wrapped := RetryWrapper(step, policy)
	out, err := wrapped.Execute(context.Background(), StepInput{})
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if out.Data != "ok" {
		t.Errorf("expected 'ok', got %v", out.Data)
	}
}

func TestRetryWrapper_NonBaseStep(t *testing.T) {
	// ParallelStep 不是 BaseStep，应创建包装
	ps := NewParallelStep("p", "P", []Step{
		NewStep("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "done"}, nil
		}),
	})
	policy := &RetryPolicy{MaxRetries: 2, InitialInterval: time.Millisecond, MaxInterval: 10 * time.Millisecond, Multiplier: 2}
	wrapped := RetryWrapper(ps, policy)
	// 包装后是 BaseStep 类型
	if _, ok := wrapped.(*BaseStep); !ok {
		t.Error("expected wrapped to be BaseStep")
	}
}

func TestTimeoutWrapper_BaseStep(t *testing.T) {
	step := NewStep("s", "S", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return &StepOutput{Data: "done"}, nil
		}
	})
	wrapped := TimeoutWrapper(step, 10*time.Millisecond)
	_, err := wrapped.Execute(context.Background(), StepInput{})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestTimeoutWrapper_NonBaseStep(t *testing.T) {
	ps := NewParallelStep("p", "P", []Step{
		NewStep("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "done"}, nil
		}),
	})
	wrapped := TimeoutWrapper(ps, time.Second)
	if _, ok := wrapped.(*BaseStep); !ok {
		t.Error("expected wrapped to be BaseStep")
	}
}

// ============== Step Options ==============

func TestStepOptions(t *testing.T) {
	step := NewStep("s", "S",
		func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		},
		WithStepDescription("test desc"),
		WithStepRetryPolicy(&RetryPolicy{MaxRetries: 2}),
		WithStepTimeout(5*time.Second),
		WithStepDependencies("dep1", "dep2"),
		WithStepMetadata("key", "val"),
	)
	if step.description != "test desc" {
		t.Errorf("expected description")
	}
	if step.retryPolicy.MaxRetries != 2 {
		t.Errorf("expected MaxRetries 2")
	}
	if step.timeout != 5*time.Second {
		t.Errorf("expected timeout 5s")
	}
	deps := step.Dependencies()
	if len(deps) != 2 || deps[0] != "dep1" {
		t.Errorf("expected deps [dep1 dep2], got %v", deps)
	}
	if step.metadata["key"] != "val" {
		t.Errorf("expected metadata key=val")
	}
}

func TestBaseStep_Validate(t *testing.T) {
	// 空 ID
	s := &BaseStep{name: "n", executeFn: func(ctx context.Context, input StepInput) (*StepOutput, error) { return nil, nil }}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// 空 name
	s = &BaseStep{id: "id", executeFn: func(ctx context.Context, input StepInput) (*StepOutput, error) { return nil, nil }}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty name")
	}
	// nil fn
	s = &BaseStep{id: "id", name: "n"}
	if err := s.Validate(); err == nil {
		t.Error("expected error for nil fn")
	}
}

func TestBaseStep_RetryWithExponentialBackoff(t *testing.T) {
	callCount := 0
	step := NewStep("s", "S",
		func(ctx context.Context, input StepInput) (*StepOutput, error) {
			callCount++
			if callCount <= 2 {
				return nil, errors.New("retry me")
			}
			return &StepOutput{Data: "success"}, nil
		},
		WithStepRetryPolicy(&RetryPolicy{
			MaxRetries:      3,
			InitialInterval: time.Millisecond,
			MaxInterval:     5 * time.Millisecond,
			Multiplier:      2.0,
		}),
	)
	out, err := step.Execute(context.Background(), StepInput{})
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if out.Data != "success" {
		t.Errorf("expected 'success', got %v", out.Data)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestCalculateRetryInterval(t *testing.T) {
	step := NewStep("s", "S", nil, WithStepRetryPolicy(&RetryPolicy{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     500 * time.Millisecond,
		Multiplier:      2.0,
	}))

	// attempt 0: 100ms
	if d := step.calculateRetryInterval(0); d != 100*time.Millisecond {
		t.Errorf("attempt 0: expected 100ms, got %v", d)
	}
	// attempt 1: 200ms
	if d := step.calculateRetryInterval(1); d != 200*time.Millisecond {
		t.Errorf("attempt 1: expected 200ms, got %v", d)
	}
	// attempt 2: 400ms
	if d := step.calculateRetryInterval(2); d != 400*time.Millisecond {
		t.Errorf("attempt 2: expected 400ms, got %v", d)
	}
	// attempt 3: cap 到 500ms
	if d := step.calculateRetryInterval(3); d != 500*time.Millisecond {
		t.Errorf("attempt 3: expected 500ms (capped), got %v", d)
	}

	// nil retryPolicy
	s2 := &BaseStep{}
	if d := s2.calculateRetryInterval(0); d != time.Second {
		t.Errorf("nil policy: expected 1s, got %v", d)
	}
}

// ============== ParallelStep 扩展 ==============

func TestParallelStep_EmptySteps(t *testing.T) {
	ps := NewParallelStep("p", "P", nil)
	out, err := ps.Execute(context.Background(), StepInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != nil {
		t.Errorf("expected nil data for empty steps")
	}
}

func TestParallelStep_WithMaxParallel(t *testing.T) {
	var maxConcurrent int32
	var current int32

	steps := make([]Step, 5)
	for i := range steps {
		id := fmt.Sprintf("s%d", i)
		steps[i] = NewStep(id, id, func(ctx context.Context, input StepInput) (*StepOutput, error) {
			c := atomic.AddInt32(&current, 1)
			for {
				old := atomic.LoadInt32(&maxConcurrent)
				if c > old {
					if atomic.CompareAndSwapInt32(&maxConcurrent, old, c) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&current, -1)
			return &StepOutput{Data: "ok"}, nil
		})
	}

	ps := NewParallelStep("p", "P", steps, WithMaxParallel(2))
	_, err := ps.Execute(context.Background(), StepInput{})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&maxConcurrent) > 2 {
		t.Errorf("max concurrent should be <= 2, got %d", maxConcurrent)
	}
}

func TestParallelStep_FailFast(t *testing.T) {
	ps := NewParallelStep("p", "P", []Step{
		NewStep("ok", "OK", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			time.Sleep(50 * time.Millisecond)
			return &StepOutput{Data: "ok"}, nil
		}),
		NewStep("fail", "Fail", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return nil, errors.New("step error")
		}),
	}, WithFailFast(true))

	_, err := ps.Execute(context.Background(), StepInput{})
	if err == nil {
		t.Error("expected error with failFast")
	}
}

func TestParallelStep_NoFailFast(t *testing.T) {
	ps := NewParallelStep("p", "P", []Step{
		NewStep("ok", "OK", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}),
		NewStep("fail", "Fail", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return nil, errors.New("step error")
		}),
	}, WithFailFast(false))

	out, err := ps.Execute(context.Background(), StepInput{})
	// 不快速失败，仍然返回 err 但同时返回部分结果
	if err == nil {
		t.Error("expected error")
	}
	if out == nil {
		t.Error("expected partial output")
	}
}

func TestParallelStep_Validate(t *testing.T) {
	// 空 ID
	ps := &ParallelStep{steps: []Step{NewStep("s", "S", func(ctx context.Context, input StepInput) (*StepOutput, error) { return nil, nil })}}
	if err := ps.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// 空步骤列表
	ps = &ParallelStep{id: "p"}
	if err := ps.Validate(); err == nil {
		t.Error("expected error for empty steps")
	}
	// 子步骤验证失败
	ps = &ParallelStep{id: "p", steps: []Step{&BaseStep{id: "s"}}} // 无 name
	if err := ps.Validate(); err == nil {
		t.Error("expected sub-step validation error")
	}
}

// ============== ConditionalStep 扩展 ==============

func TestConditionalStep_UnknownBranch(t *testing.T) {
	cs := NewConditionalStep("c", "C", func(ctx context.Context, input StepInput) (string, error) {
		return "unknown", nil
	})
	cs.Then(NewStep("t", "T", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: "then"}, nil
	}))
	out, err := cs.Execute(context.Background(), StepInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.NextStepID != "unknown" {
		t.Errorf("expected NextStepID 'unknown', got %s", out.NextStepID)
	}
}

func TestConditionalStep_ConditionError(t *testing.T) {
	cs := NewConditionalStep("c", "C", func(ctx context.Context, input StepInput) (string, error) {
		return "", errors.New("cond err")
	})
	_, err := cs.Execute(context.Background(), StepInput{})
	if err == nil {
		t.Error("expected error")
	}
}

func TestConditionalStep_BranchExecutionError(t *testing.T) {
	cs := NewConditionalStep("c", "C", func(ctx context.Context, input StepInput) (string, error) {
		return "true", nil
	})
	cs.Then(NewStep("t", "T", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return nil, errors.New("branch error")
	}))
	_, err := cs.Execute(context.Background(), StepInput{})
	if err == nil {
		t.Error("expected branch error")
	}
}

func TestConditionalStep_Validate(t *testing.T) {
	// 空 ID
	cs := &ConditionalStep{condition: func(ctx context.Context, input StepInput) (string, error) { return "", nil }, branches: map[string]Step{"a": NewStep("a", "A", nil)}}
	if err := cs.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// nil condition
	cs = &ConditionalStep{id: "c", branches: map[string]Step{"a": NewStep("a", "A", nil)}}
	if err := cs.Validate(); err == nil {
		t.Error("expected error for nil condition")
	}
	// 空 branches
	cs = &ConditionalStep{id: "c", condition: func(ctx context.Context, input StepInput) (string, error) { return "", nil }, branches: map[string]Step{}}
	if err := cs.Validate(); err == nil {
		t.Error("expected error for empty branches")
	}
}

// ============== LoopStep 扩展 ==============

func TestLoopStep_WithCollectOutput(t *testing.T) {
	i := 0
	body := NewStep("body", "Body", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		i++
		return &StepOutput{Data: i}, nil
	})
	ls := NewLoopStep("loop", "Loop", body,
		func(ctx context.Context, input StepInput) (string, error) {
			if input.Data.(int) >= 3 {
				return "break", nil
			}
			return "continue", nil
		},
		WithMaxIterations(10),
		WithCollectOutput(false),
	)
	out, err := ls.Execute(context.Background(), StepInput{Data: 0, Variables: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	// collectOutput=false 时返回最终 data 而非数组
	if out.Data.(int) != 3 {
		t.Errorf("expected 3, got %v", out.Data)
	}
}

func TestLoopStep_ConditionError(t *testing.T) {
	body := NewStep("body", "Body", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return &StepOutput{Data: "ok"}, nil
	})
	ls := NewLoopStep("loop", "Loop", body,
		func(ctx context.Context, input StepInput) (string, error) {
			return "", errors.New("cond error")
		},
	)
	_, err := ls.Execute(context.Background(), StepInput{Data: 0, Variables: map[string]any{}})
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoopStep_BodyError(t *testing.T) {
	body := NewStep("body", "Body", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		return nil, errors.New("body error")
	})
	ls := NewLoopStep("loop", "Loop", body,
		func(ctx context.Context, input StepInput) (string, error) {
			return "continue", nil
		},
	)
	_, err := ls.Execute(context.Background(), StepInput{Data: 0, Variables: map[string]any{}})
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoopStep_Validate(t *testing.T) {
	// 空 ID
	ls := &LoopStep{step: NewStep("s", "S", nil), condition: func(ctx context.Context, input StepInput) (string, error) { return "", nil }}
	if err := ls.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// nil step
	ls = &LoopStep{id: "l", condition: func(ctx context.Context, input StepInput) (string, error) { return "", nil }}
	if err := ls.Validate(); err == nil {
		t.Error("expected error for nil step")
	}
	// nil condition
	ls = &LoopStep{id: "l", step: NewStep("s", "S", nil)}
	if err := ls.Validate(); err == nil {
		t.Error("expected error for nil condition")
	}
}

// ============== WaitUntilStep ==============

func TestWaitUntilStep(t *testing.T) {
	callCount := 0
	ws := NewWaitUntilStep("w", "Wait", func(ctx context.Context, input StepInput) (bool, error) {
		callCount++
		return callCount >= 2, nil
	})
	// 使用短 context 超时保证测试不会挂住
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := ws.Execute(ctx, StepInput{Data: "hello"})
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if out.Data != "hello" {
		t.Errorf("expected passthrough data")
	}
}

func TestWaitUntilStep_Error(t *testing.T) {
	ws := NewWaitUntilStep("w", "Wait", func(ctx context.Context, input StepInput) (bool, error) {
		return false, errors.New("check error")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ws.Execute(ctx, StepInput{})
	if err == nil {
		t.Error("expected error")
	}
}

func TestWaitStep_NoConditionNoDuration(t *testing.T) {
	ws := &WaitStep{id: "w", name: "W"}
	out, err := ws.Execute(context.Background(), StepInput{Data: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "pass" {
		t.Errorf("expected passthrough")
	}
}

func TestWaitStep_Validate(t *testing.T) {
	// 空 ID
	ws := &WaitStep{duration: time.Second}
	if err := ws.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// 无 duration 和 until
	ws = &WaitStep{id: "w", name: "W"}
	if err := ws.Validate(); err == nil {
		t.Error("expected error for no duration/until")
	}
}

// ============== SubWorkflowStep ==============

func TestSubWorkflowStep(t *testing.T) {
	subWf, _ := New("sub").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "sub-result"}, nil
		}).
		Build()
	executor := NewExecutor()

	sws := NewSubWorkflowStep("sw", "SubWF", subWf, executor)
	out, err := sws.Execute(context.Background(), StepInput{Data: "input", Variables: map[string]any{}, Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "sub-result" {
		t.Errorf("expected 'sub-result', got %v", out.Data)
	}
}

func TestSubWorkflowStep_Validate(t *testing.T) {
	// 空 ID
	sws := &SubWorkflowStep{workflow: &Workflow{}, runner: NewExecutor()}
	if err := sws.Validate(); err == nil {
		t.Error("expected error for empty id")
	}
	// nil workflow
	sws = &SubWorkflowStep{id: "s", runner: NewExecutor()}
	if err := sws.Validate(); err == nil {
		t.Error("expected error for nil workflow")
	}
	// nil runner
	sws = &SubWorkflowStep{id: "s", workflow: &Workflow{}}
	if err := sws.Validate(); err == nil {
		t.Error("expected error for nil runner")
	}
}

// ============== Executor 扩展 ==============

func TestExecutor_WithStore(t *testing.T) {
	store := NewMemoryWorkflowStore()
	executor := NewExecutor(WithStore(store))
	if !executor.config.EnablePersistence {
		t.Error("expected EnablePersistence=true")
	}

	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	out, err := executor.Run(context.Background(), wf, WorkflowInput{Data: "start"})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected output")
	}
}

func TestExecutor_WithHooks(t *testing.T) {
	var hookLog []string
	var mu sync.Mutex
	hooks := &WorkflowHooks{
		OnStart: func(ctx context.Context, wf *Workflow, input WorkflowInput) error {
			mu.Lock()
			hookLog = append(hookLog, "start")
			mu.Unlock()
			return nil
		},
		OnComplete: func(ctx context.Context, wf *Workflow, output *WorkflowOutput) error {
			mu.Lock()
			hookLog = append(hookLog, "complete")
			mu.Unlock()
			return nil
		},
		OnStepStart: func(ctx context.Context, step Step, input any) error {
			mu.Lock()
			hookLog = append(hookLog, "step-start:"+step.ID())
			mu.Unlock()
			return nil
		},
		OnStepComplete: func(ctx context.Context, step Step, output any) error {
			mu.Lock()
			hookLog = append(hookLog, "step-complete:"+step.ID())
			mu.Unlock()
			return nil
		},
	}
	executor := NewExecutor(WithHooks(hooks))

	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	_, err := executor.Run(context.Background(), wf, WorkflowInput{Data: "x"})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hookLog) < 3 {
		t.Errorf("expected at least 3 hook events, got %d: %v", len(hookLog), hookLog)
	}
	if hookLog[0] != "start" {
		t.Errorf("expected first hook to be 'start', got %s", hookLog[0])
	}
}

func TestExecutor_OnStartHookError(t *testing.T) {
	hooks := &WorkflowHooks{
		OnStart: func(ctx context.Context, wf *Workflow, input WorkflowInput) error {
			return errors.New("hook error")
		},
	}
	executor := NewExecutor(WithHooks(hooks))
	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()
	_, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err == nil {
		t.Error("expected hook error")
	}
}

func TestExecutor_OnEvent(t *testing.T) {
	var events []*WorkflowEvent
	var mu sync.Mutex
	executor := NewExecutor()
	executor.OnEvent(func(event *WorkflowEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	_, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	// 至少应有: workflow_started, step_started, step_completed, workflow_completed
	if len(events) < 4 {
		t.Errorf("expected at least 4 events, got %d", len(events))
	}
	if events[0].Type != EventWorkflowStarted {
		t.Errorf("expected first event to be WorkflowStarted, got %s", events[0].Type)
	}
}

func TestExecutor_Cancel(t *testing.T) {
	executor := NewExecutor()

	wf, _ := New("test").
		AddFunc("slow", "Slow", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &StepOutput{Data: "done"}, nil
			}
		}).
		Build()

	execID, err := executor.RunAsync(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // 让执行开始

	if err := executor.Cancel(context.Background(), execID); err != nil {
		t.Fatal(err)
	}

	exec, err := executor.WaitForCompletion(context.Background(), execID, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Cancel 触发 ctx.Done，step 返回错误后可能先设为 Failed 再设为 Cancelled
	if exec.Status != StatusCancelled && exec.Status != StatusFailed {
		t.Errorf("expected cancelled or failed, got %s", exec.Status)
	}
}

func TestExecutor_Cancel_NotFound(t *testing.T) {
	executor := NewExecutor()
	if err := executor.Cancel(context.Background(), "missing"); err == nil {
		t.Error("expected error")
	}
}

func TestExecutor_Pause_NotFound(t *testing.T) {
	executor := NewExecutor()
	if err := executor.Pause(context.Background(), "missing"); err == nil {
		t.Error("expected error")
	}
}

func TestExecutor_Resume_NotFound(t *testing.T) {
	executor := NewExecutor()
	if err := executor.Resume(context.Background(), "missing"); err == nil {
		t.Error("expected error")
	}
}

func TestExecutor_GetExecution(t *testing.T) {
	executor := NewExecutor()
	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	execID, _ := executor.RunAsync(context.Background(), wf, WorkflowInput{})
	exec, err := executor.GetExecution(context.Background(), execID)
	if err != nil {
		t.Fatal(err)
	}
	if exec.ID != execID {
		t.Errorf("expected ID %s, got %s", execID, exec.ID)
	}

	// 不存在
	_, err = executor.GetExecution(context.Background(), "missing")
	if err == nil {
		t.Error("expected error")
	}
}

func TestExecutor_GetExecution_FromStore(t *testing.T) {
	store := NewMemoryWorkflowStore()
	executor := NewExecutor(WithStore(store))

	// 直接写入 store
	testExec := &WorkflowExecution{
		ID:     "stored-exec",
		Status: StatusCompleted,
		Context: &ExecutionContext{
			Variables: make(map[string]any),
		},
		StepResults: make(map[string]*StepResult),
	}
	store.SaveExecution(context.Background(), testExec)

	exec, err := executor.GetExecution(context.Background(), "stored-exec")
	if err != nil {
		t.Fatal(err)
	}
	if exec.ID != "stored-exec" {
		t.Errorf("expected stored-exec")
	}
}

func TestExecutor_ListExecutions(t *testing.T) {
	executor := NewExecutor()
	wf, _ := New("test").WithID("wf-list").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	executor.Run(context.Background(), wf, WorkflowInput{})
	executor.Run(context.Background(), wf, WorkflowInput{})

	// 全部列出
	execs, err := executor.ListExecutions(context.Background(), "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) < 2 {
		t.Errorf("expected at least 2 executions, got %d", len(execs))
	}

	// 按 workflow ID 过滤
	execs, err = executor.ListExecutions(context.Background(), "wf-list", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) < 2 {
		t.Errorf("expected at least 2 executions for wf-list")
	}

	// 按 status 过滤
	execs, err = executor.ListExecutions(context.Background(), "", StatusCompleted, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) > 1 {
		t.Errorf("expected at most 1 execution with limit=1")
	}
}

func TestExecutor_ListExecutions_FromStore(t *testing.T) {
	store := NewMemoryWorkflowStore()
	executor := NewExecutor(WithStore(store))

	store.SaveExecution(context.Background(), &WorkflowExecution{
		ID:          "e1",
		WorkflowID:  "wf-1",
		Status:      StatusCompleted,
		StartedAt:   time.Now(),
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})

	execs, err := executor.ListExecutions(context.Background(), "wf-1", StatusCompleted, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) != 1 {
		t.Errorf("expected 1 execution, got %d", len(execs))
	}
}

func TestExecutor_CleanupCompleted(t *testing.T) {
	executor := NewExecutor()
	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	executor.Run(context.Background(), wf, WorkflowInput{})
	executor.Run(context.Background(), wf, WorkflowInput{})

	// 立即清理 (olderThan=0) 应清理所有完成的
	cleaned := executor.CleanupCompleted(0)
	if cleaned < 2 {
		t.Errorf("expected at least 2 cleaned, got %d", cleaned)
	}
}

func TestExecutor_RunAsync_WithVariables(t *testing.T) {
	executor := NewExecutor()
	wf, _ := New("test").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			v := input.Variables["key"]
			return &StepOutput{Data: v}, nil
		}).
		Build()

	out, err := executor.Run(context.Background(), wf, WorkflowInput{
		Data:      "start",
		Variables: map[string]any{"key": "value"},
		Metadata:  map[string]any{"meta": "data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "value" {
		t.Errorf("expected 'value', got %v", out.Data)
	}
}

func TestExecutor_StepErrorHook(t *testing.T) {
	var hookErr error
	hooks := &WorkflowHooks{
		OnStepError: func(ctx context.Context, step Step, err error) error {
			hookErr = err
			return nil
		},
	}
	executor := NewExecutor(WithHooks(hooks))
	wf, _ := New("test").
		AddFunc("fail", "Fail", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return nil, errors.New("step-failed")
		}).
		Build()
	executor.Run(context.Background(), wf, WorkflowInput{})
	if hookErr == nil || hookErr.Error() != "step-failed" {
		t.Errorf("expected OnStepError hook called with 'step-failed'")
	}
}

// ============== Persistence 扩展 ==============

func TestMemoryWorkflowStore_Execution_CRUD(t *testing.T) {
	store := NewMemoryWorkflowStore()
	ctx := context.Background()

	exec := &WorkflowExecution{
		ID:         "e1",
		WorkflowID: "wf-1",
		Status:     StatusRunning,
		StartedAt:  time.Now(),
		Context: &ExecutionContext{
			Variables: map[string]any{"k": "v"},
		},
		StepResults: map[string]*StepResult{
			"s1": {StepID: "s1", Status: StatusCompleted},
		},
	}

	// Save
	if err := store.SaveExecution(ctx, exec); err != nil {
		t.Fatal(err)
	}

	// Get
	got, err := store.GetExecution(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkflowID != "wf-1" {
		t.Errorf("expected WorkflowID wf-1")
	}

	// Get not found
	_, err = store.GetExecution(ctx, "missing")
	if err == nil {
		t.Error("expected error")
	}

	// List
	execs, err := store.ListExecutions(ctx, "wf-1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) != 1 {
		t.Errorf("expected 1 execution")
	}

	// List with status filter
	execs, _ = store.ListExecutions(ctx, "", StatusCompleted, 10)
	if len(execs) != 0 {
		t.Errorf("expected 0 completed executions")
	}

	// Delete
	if err := store.DeleteExecution(ctx, "e1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.GetExecution(ctx, "e1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestMemoryWorkflowStore_UpdateExecutionStatus(t *testing.T) {
	store := NewMemoryWorkflowStore()
	ctx := context.Background()

	exec := &WorkflowExecution{
		ID:          "e1",
		Status:      StatusRunning,
		StartedAt:   time.Now(),
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	}
	store.SaveExecution(ctx, exec)

	// 更新为 Completed
	if err := store.UpdateExecutionStatus(ctx, "e1", StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetExecution(ctx, "e1")
	if got.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}

	// 更新为 Paused
	store.SaveExecution(ctx, &WorkflowExecution{
		ID:          "e2",
		Status:      StatusRunning,
		StartedAt:   time.Now(),
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})
	store.UpdateExecutionStatus(ctx, "e2", StatusPaused, "")
	got, _ = store.GetExecution(ctx, "e2")
	if got.PausedAt == nil {
		t.Error("expected PausedAt to be set")
	}

	// 更新不存在的
	if err := store.UpdateExecutionStatus(ctx, "missing", StatusFailed, "err"); err == nil {
		t.Error("expected error for missing execution")
	}
}

func TestMemoryWorkflowStore_GetPendingExecutions(t *testing.T) {
	store := NewMemoryWorkflowStore()
	ctx := context.Background()

	store.SaveExecution(ctx, &WorkflowExecution{
		ID:          "e1",
		Status:      StatusRunning,
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})
	store.SaveExecution(ctx, &WorkflowExecution{
		ID:          "e2",
		Status:      StatusPaused,
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})
	store.SaveExecution(ctx, &WorkflowExecution{
		ID:          "e3",
		Status:      StatusCompleted,
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})

	pending, err := store.GetPendingExecutions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

func TestMemoryWorkflowStore_ClearAndStats(t *testing.T) {
	store := NewMemoryWorkflowStore()
	ctx := context.Background()

	store.SaveWorkflow(ctx, &Workflow{ID: "wf-1", Name: "W1"})
	store.SaveExecution(ctx, &WorkflowExecution{
		ID:          "e1",
		Context:     &ExecutionContext{Variables: make(map[string]any)},
		StepResults: make(map[string]*StepResult),
	})

	wc, ec := store.Stats()
	if wc != 1 || ec != 1 {
		t.Errorf("expected 1/1, got %d/%d", wc, ec)
	}

	store.Clear()
	wc, ec = store.Stats()
	if wc != 0 || ec != 0 {
		t.Errorf("expected 0/0 after clear, got %d/%d", wc, ec)
	}
}

// ============== Snapshot ==============

func TestSnapshot(t *testing.T) {
	exec := &WorkflowExecution{
		ID:         "e1",
		WorkflowID: "wf-1",
		Status:     StatusRunning,
		Context: &ExecutionContext{
			Variables:      map[string]any{"x": 1},
			CompletedSteps: []string{"s1"},
		},
		StepResults: map[string]*StepResult{
			"s1": {StepID: "s1", Status: StatusCompleted},
		},
		Input: json.RawMessage(`{"data":"hello"}`),
	}

	snap := CreateSnapshot(exec)
	if snap.ExecutionID != "e1" {
		t.Errorf("expected e1")
	}
	if snap.WorkflowID != "wf-1" {
		t.Errorf("expected wf-1")
	}

	restored := RestoreFromSnapshot(snap)
	if restored.ID != "e1" {
		t.Errorf("expected e1")
	}
	if restored.Status != StatusRunning {
		t.Errorf("expected StatusRunning")
	}
	if len(restored.StepResults) != 1 {
		t.Errorf("expected 1 step result")
	}
}

// ============== ExecutionRecovery ==============

func TestExecutionRecovery(t *testing.T) {
	store := NewMemoryWorkflowStore()
	reg := NewWorkflowRegistry()
	executor := NewExecutor(WithStore(store))
	ctx := context.Background()

	// 注册工作流
	wf, _ := New("test").
		WithID("wf-recover").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "recovered"}, nil
		}).
		Build()
	reg.Register(wf)
	store.SaveWorkflow(ctx, wf)

	// 模拟一个 pending 执行
	store.SaveExecution(ctx, &WorkflowExecution{
		ID:         "e-pending",
		WorkflowID: "wf-recover",
		Status:     StatusRunning,
		Input:      json.RawMessage(`{"data":"test"}`),
		Context: &ExecutionContext{
			Variables: map[string]any{},
			Metadata:  map[string]any{},
		},
		StepResults: make(map[string]*StepResult),
	})

	recovery := NewExecutionRecovery(store, reg, executor)
	count, err := recovery.RecoverPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 recovered, got %d", count)
	}
}

func TestExecutionRecovery_WorkflowFromStore(t *testing.T) {
	store := NewMemoryWorkflowStore()
	reg := NewWorkflowRegistry() // 空注册，不注册到 registry
	executor := NewExecutor(WithStore(store))
	ctx := context.Background()

	wf, _ := New("test").
		WithID("wf-store").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()
	store.SaveWorkflow(ctx, wf)

	store.SaveExecution(ctx, &WorkflowExecution{
		ID:         "e-store",
		WorkflowID: "wf-store",
		Status:     StatusRunning,
		Context: &ExecutionContext{
			Variables: map[string]any{},
			Metadata:  map[string]any{},
		},
		StepResults: make(map[string]*StepResult),
	})

	recovery := NewExecutionRecovery(store, reg, executor)
	count, _ := recovery.RecoverPending(ctx)
	if count != 1 {
		t.Errorf("expected 1 recovered from store, got %d", count)
	}
}

// ============== Builder 子工作流 ==============

func TestBuilder_SubWorkflow(t *testing.T) {
	subWf, _ := New("sub").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "sub-done"}, nil
		}).
		Build()
	executor := NewExecutor()

	wf, err := New("main").
		AddFunc("pre", "Pre", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "pre-done"}, nil
		}).
		SubWorkflow("sw", "SubWF", subWf, executor).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	out, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != "sub-done" {
		t.Errorf("expected 'sub-done', got %v", out.Data)
	}
}

func TestBuilder_Loop(t *testing.T) {
	i := 0
	body := NewStep("body", "Body", func(ctx context.Context, input StepInput) (*StepOutput, error) {
		i++
		return &StepOutput{Data: i}, nil
	})
	wf, err := New("loop").
		Loop("loop-step", "Loop", body, func(ctx context.Context, input StepInput) (string, error) {
			if input.Data.(int) >= 2 {
				return "break", nil
			}
			return "continue", nil
		}, WithMaxIterations(5)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	_, err = executor.Run(context.Background(), wf, WorkflowInput{Data: 0})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuilder_WaitUntil(t *testing.T) {
	var ready atomic.Bool
	go func() {
		time.Sleep(100 * time.Millisecond)
		ready.Store(true)
	}()
	wf, err := New("wait-until").
		WaitUntil("wu", "WaitReady", func(ctx context.Context, input StepInput) (bool, error) {
			return ready.Load(), nil
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	_, err = executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
}

// ============== Step 输出变量合并 ==============

func TestExecutor_StepOutputVariables(t *testing.T) {
	wf, _ := New("vars").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{
				Data:      "s1",
				Variables: map[string]any{"from_s1": true},
			}, nil
		}).
		AddFunc("s2", "S2", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			if input.Variables["from_s1"] != true {
				return nil, errors.New("expected from_s1 variable")
			}
			return &StepOutput{Data: "s2"}, nil
		}).
		Build()

	executor := NewExecutor()
	out, err := executor.Run(context.Background(), wf, WorkflowInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Variables["from_s1"] != true {
		t.Error("expected from_s1 in output variables")
	}
}

// ============== 并发安全 ==============

func TestExecutor_Concurrent(t *testing.T) {
	executor := NewExecutor()
	wf, _ := New("conc").
		AddFunc("s1", "S1", func(ctx context.Context, input StepInput) (*StepOutput, error) {
			return &StepOutput{Data: "ok"}, nil
		}).
		Build()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.Run(context.Background(), wf, WorkflowInput{})
			if err != nil {
				t.Errorf("concurrent run error: %v", err)
			}
		}()
	}
	wg.Wait()
}
