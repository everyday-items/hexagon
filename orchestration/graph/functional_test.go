package graph

import (
	"context"
	"testing"
)

// TestWorkflowBasic 测试基本工作流
func TestWorkflowBasic(t *testing.T) {
	ctx := context.Background()

	wf := NewWorkflow[MapState]("basic-workflow")

	// 定义任务
	task1 := DefineTask(wf, "task1", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("step1", "done")
		return s, nil
	})

	task2 := DefineTask(wf, "task2", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("step2", "done")
		return s, nil
	})

	// 定义入口点
	DefineEntrypoint(wf, func(ctx context.Context, state MapState) (MapState, error) {
		state, err := task1.Run(ctx, state)
		if err != nil {
			return state, err
		}
		return task2.Run(ctx, state)
	})

	// 执行工作流
	result, err := wf.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := result.Get("step1"); !ok || v != "done" {
		t.Error("step1 未执行")
	}
	if v, ok := result.Get("step2"); !ok || v != "done" {
		t.Error("step2 未执行")
	}
}

// TestWorkflowToGraph 测试工作流转换为图
func TestWorkflowToGraph(t *testing.T) {
	ctx := context.Background()

	wf := NewWorkflow[MapState]("to-graph-test")

	task := DefineTask(wf, "process", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("processed", true)
		return s, nil
	})

	DefineEntrypoint(wf, func(ctx context.Context, state MapState) (MapState, error) {
		return task.Run(ctx, state)
	})

	g, err := wf.ToGraph()
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := result.Get("processed"); !ok || v != true {
		t.Error("processed 未执行")
	}
}

// TestDefineTask 测试任务定义
func TestDefineTask(t *testing.T) {
	wf := NewWorkflow[MapState]("task-test")
	task := DefineTask(wf, "test-task", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("executed", true)
		return s, nil
	})

	if task.Name != "test-task" {
		t.Errorf("期望名称 test-task，实际 %s", task.Name)
	}

	// 测试执行计数
	if task.ExecCount() != 0 {
		t.Errorf("期望执行次数 0，实际 %d", task.ExecCount())
	}

	ctx := context.Background()
	_, err := task.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	if task.ExecCount() != 1 {
		t.Errorf("期望执行次数 1，实际 %d", task.ExecCount())
	}
}

// TestRunParallel 测试并行执行
func TestRunParallel(t *testing.T) {
	ctx := context.Background()

	wf := NewWorkflow[MapState]("parallel-test")

	taskA := DefineTask(wf, "task_a", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("a_done", true)
		return s, nil
	})
	taskB := DefineTask(wf, "task_b", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("b_done", true)
		return s, nil
	})

	merge := func(original MapState, results []MapState) MapState {
		for _, r := range results {
			for k, v := range r {
				original[k] = v
			}
		}
		return original
	}

	initialState := MapState{}
	initialState.Set("input", "hello")

	result, err := RunParallel(ctx, initialState, merge, taskA, taskB)
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := result.Get("a_done"); !ok || v != true {
		t.Error("任务 A 未执行")
	}
	if v, ok := result.Get("b_done"); !ok || v != true {
		t.Error("任务 B 未执行")
	}
}

// TestRunConditional 测试条件执行
func TestRunConditional(t *testing.T) {
	ctx := context.Background()

	wf := NewWorkflow[MapState]("conditional-test")

	branchHigh := DefineTask(wf, "high", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("branch", "high")
		return s, nil
	})
	branchLow := DefineTask(wf, "low", func(_ context.Context, s MapState) (MapState, error) {
		s.Set("branch", "low")
		return s, nil
	})

	routes := map[string]*TaskDef[MapState]{
		"high": branchHigh,
		"low":  branchLow,
	}

	condition := func(s MapState) string {
		v, _ := s.Get("value")
		if val, ok := v.(int); ok && val > 5 {
			return "high"
		}
		return "low"
	}

	t.Run("选择 high 分支", func(t *testing.T) {
		state := MapState{}
		state.Set("value", 10)

		result, err := RunConditional(ctx, state, condition, routes)
		if err != nil {
			t.Fatal(err)
		}

		if v, _ := result.Get("branch"); v != "high" {
			t.Errorf("期望分支 high，实际 %v", v)
		}
	})

	t.Run("选择 low 分支", func(t *testing.T) {
		state := MapState{}
		state.Set("value", 3)

		result, err := RunConditional(ctx, state, condition, routes)
		if err != nil {
			t.Fatal(err)
		}

		if v, _ := result.Get("branch"); v != "low" {
			t.Errorf("期望分支 low，实际 %v", v)
		}
	})
}

// TestWorkflowNoEntrypoint 测试未设置入口点
func TestWorkflowNoEntrypoint(t *testing.T) {
	wf := NewWorkflow[MapState]("no-entry")

	_, err := wf.Run(context.Background(), MapState{})
	if err == nil {
		t.Error("期望错误（未定义入口点）")
	}
}

// TestWorkflowName 测试工作流名称
func TestWorkflowName(t *testing.T) {
	wf := NewWorkflow[MapState]("my-workflow")
	if wf.Name() != "my-workflow" {
		t.Errorf("期望名称 my-workflow，实际 %s", wf.Name())
	}
}

// TestWorkflowTasks 测试任务列表
func TestWorkflowTasks(t *testing.T) {
	wf := NewWorkflow[MapState]("tasks-test")
	DefineTask(wf, "t1", func(_ context.Context, s MapState) (MapState, error) { return s, nil })
	DefineTask(wf, "t2", func(_ context.Context, s MapState) (MapState, error) { return s, nil })

	tasks := wf.Tasks()
	if len(tasks) != 2 {
		t.Errorf("期望 2 个任务，实际 %d", len(tasks))
	}
}
