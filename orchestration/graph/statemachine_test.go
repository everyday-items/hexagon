package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestStateMachine_BasicRun 测试基本的状态机流转：init -> process -> done
func TestStateMachine_BasicRun(t *testing.T) {
	sm := NewStateMachine[*TestState]("basic")

	// 添加状态：init 处理器将 Counter 加 1，并追加路径
	sm.AddState("init", func(ctx context.Context, s *TestState) (string, error) {
		s.Counter++
		s.Path += "init"
		return "", nil // 使用转换条件决定下一状态
	})

	// 添加状态：process 处理器将 Counter 加 1，并追加路径
	sm.AddState("process", func(ctx context.Context, s *TestState) (string, error) {
		s.Counter++
		s.Path += "->process"
		return "", nil
	})

	// 添加转换条件
	sm.AddTransition("init", "process", Always[*TestState]())
	sm.AddTransition("process", "done", Always[*TestState]())

	// 设置初始状态和终态
	sm.SetInitial("init")
	sm.AddFinal("done")

	// 执行
	ctx := context.Background()
	state := &TestState{Counter: 0, Path: "", Data: make(map[string]string)}
	result, err := sm.Run(ctx, state)

	if err != nil {
		t.Fatalf("期望无错误，实际得到: %v", err)
	}

	if result.Counter != 2 {
		t.Errorf("期望 Counter=2，实际得到 Counter=%d", result.Counter)
	}

	if result.Path != "init->process" {
		t.Errorf("期望 Path='init->process'，实际得到 Path='%s'", result.Path)
	}
}

// TestStateMachine_NoInitialState 测试未设置初始状态时返回 ErrNoInitialState
func TestStateMachine_NoInitialState(t *testing.T) {
	sm := NewStateMachine[*TestState]("no-initial")

	sm.AddState("step1", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil
	})

	// 故意不调用 SetInitial
	ctx := context.Background()
	state := &TestState{}
	_, err := sm.Run(ctx, state)

	if !errors.Is(err, ErrNoInitialState) {
		t.Errorf("期望 ErrNoInitialState，实际得到: %v", err)
	}
}

// TestStateMachine_StateNotFound 测试转换到不存在的状态时返回 ErrStateNotFound
func TestStateMachine_StateNotFound(t *testing.T) {
	sm := NewStateMachine[*TestState]("state-not-found")

	// 只添加 init 状态，转换目标 process 不存在
	sm.AddState("init", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil
	})
	sm.AddTransition("init", "process", Always[*TestState]())
	sm.SetInitial("init")

	ctx := context.Background()
	state := &TestState{}
	_, err := sm.Run(ctx, state)

	if !errors.Is(err, ErrStateNotFound) {
		t.Errorf("期望 ErrStateNotFound，实际得到: %v", err)
	}

	// 验证错误消息中包含状态名
	if err != nil && !strings.Contains(err.Error(), "process") {
		t.Errorf("期望错误消息中包含状态名 'process'，实际得到: %v", err)
	}
}

// TestStateMachine_NoTransition 测试当前状态没有可用转换时返回 ErrNoTransition
func TestStateMachine_NoTransition(t *testing.T) {
	sm := NewStateMachine[*TestState]("no-transition")

	// init 状态的 handler 返回空字符串，表示使用转换条件
	sm.AddState("init", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil
	})
	sm.SetInitial("init")
	// 不添加任何转换，也不添加终态

	ctx := context.Background()
	state := &TestState{}
	_, err := sm.Run(ctx, state)

	if !errors.Is(err, ErrNoTransition) {
		t.Errorf("期望 ErrNoTransition，实际得到: %v", err)
	}
}

// TestStateMachine_NoTransitionConditionNotMatched 测试所有转换条件都不满足时返回 ErrNoTransition
func TestStateMachine_NoTransitionConditionNotMatched(t *testing.T) {
	sm := NewStateMachine[*TestState]("no-match")

	sm.AddState("init", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil
	})
	sm.AddState("unreachable", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil
	})

	// 添加一个永远不满足的转换条件
	sm.AddTransition("init", "unreachable", Never[*TestState]())
	sm.SetInitial("init")

	ctx := context.Background()
	state := &TestState{}
	_, err := sm.Run(ctx, state)

	if !errors.Is(err, ErrNoTransition) {
		t.Errorf("期望 ErrNoTransition（无条件匹配），实际得到: %v", err)
	}
}

// TestStateMachine_MaxStepsExceeded 测试超过最大步数时返回 ErrMaxStepsExceeded
func TestStateMachine_MaxStepsExceeded(t *testing.T) {
	sm := NewStateMachine[*TestState]("max-steps")

	// 创建一个无限循环：loop -> loop
	sm.AddState("loop", func(ctx context.Context, s *TestState) (string, error) {
		s.Counter++
		return "", nil
	})
	sm.AddTransition("loop", "loop", Always[*TestState]())
	sm.SetInitial("loop")
	sm.SetMaxSteps(5) // 最多 5 步

	ctx := context.Background()
	state := &TestState{Counter: 0}
	result, err := sm.Run(ctx, state)

	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Errorf("期望 ErrMaxStepsExceeded，实际得到: %v", err)
	}

	// 验证确实执行了 5 步
	if result.Counter != 5 {
		t.Errorf("期望 Counter=5（执行了5步），实际得到 Counter=%d", result.Counter)
	}
}

// TestStateMachine_HandlerReturnsNext 测试 handler 直接返回下一个状态名（跳过转换条件）
func TestStateMachine_HandlerReturnsNext(t *testing.T) {
	sm := NewStateMachine[*TestState]("handler-next")

	// init 的 handler 直接返回 "finish"，跳过转换条件
	sm.AddState("init", func(ctx context.Context, s *TestState) (string, error) {
		s.Path += "init"
		return "finish", nil // 直接指定下一状态
	})

	// 即使添加了到 process 的转换，也不会走这条路
	sm.AddState("process", func(ctx context.Context, s *TestState) (string, error) {
		s.Path += "->process"
		return "", nil
	})

	sm.AddState("finish", func(ctx context.Context, s *TestState) (string, error) {
		s.Path += "->finish"
		return "", nil
	})

	sm.AddTransition("init", "process", Always[*TestState]())
	sm.SetInitial("init")
	sm.AddFinal("done")
	sm.AddTransition("finish", "done", Always[*TestState]())

	ctx := context.Background()
	state := &TestState{Path: ""}
	result, err := sm.Run(ctx, state)

	if err != nil {
		t.Fatalf("期望无错误，实际得到: %v", err)
	}

	// 验证跳过了 process，直接到 finish
	if result.Path != "init->finish" {
		t.Errorf("期望 Path='init->finish'，实际得到 Path='%s'", result.Path)
	}
}

// TestStateMachine_OnEnterOnExit 测试 OnEnter 和 OnExit 钩子的执行顺序
func TestStateMachine_OnEnterOnExit(t *testing.T) {
	// 使用切片记录钩子执行顺序
	var hookOrder []string

	sm := NewStateMachine[*TestState]("hooks")

	// step1 带 OnEnter 和 OnExit 钩子
	sm.AddStateWithHooks("step1",
		func(ctx context.Context, s *TestState) (string, error) {
			hookOrder = append(hookOrder, "step1:handler")
			return "", nil
		},
		func(ctx context.Context, s *TestState) error {
			hookOrder = append(hookOrder, "step1:enter")
			return nil
		},
		func(ctx context.Context, s *TestState) error {
			hookOrder = append(hookOrder, "step1:exit")
			return nil
		},
	)

	// step2 带 OnEnter 和 OnExit 钩子
	sm.AddStateWithHooks("step2",
		func(ctx context.Context, s *TestState) (string, error) {
			hookOrder = append(hookOrder, "step2:handler")
			return "", nil
		},
		func(ctx context.Context, s *TestState) error {
			hookOrder = append(hookOrder, "step2:enter")
			return nil
		},
		func(ctx context.Context, s *TestState) error {
			hookOrder = append(hookOrder, "step2:exit")
			return nil
		},
	)

	sm.AddTransition("step1", "step2", Always[*TestState]())
	sm.AddTransition("step2", "done", Always[*TestState]())
	sm.SetInitial("step1")
	sm.AddFinal("done")

	ctx := context.Background()
	state := &TestState{}
	_, err := sm.Run(ctx, state)

	if err != nil {
		t.Fatalf("期望无错误，实际得到: %v", err)
	}

	// 验证钩子执行顺序：enter -> handler -> exit，依次对每个状态
	expected := []string{
		"step1:enter", "step1:handler", "step1:exit",
		"step2:enter", "step2:handler", "step2:exit",
	}

	if len(hookOrder) != len(expected) {
		t.Fatalf("期望 %d 个钩子调用，实际得到 %d 个: %v", len(expected), len(hookOrder), hookOrder)
	}

	for i, exp := range expected {
		if hookOrder[i] != exp {
			t.Errorf("钩子执行顺序[%d]: 期望 '%s'，实际得到 '%s'", i, exp, hookOrder[i])
		}
	}
}

// TestStateMachine_OnEnterError 测试 OnEnter 钩子返回错误时中止执行
func TestStateMachine_OnEnterError(t *testing.T) {
	expectedErr := errors.New("enter hook failed")

	sm := NewStateMachine[*TestState]("enter-error")
	sm.AddStateWithHooks("step1",
		func(ctx context.Context, s *TestState) (string, error) {
			t.Error("handler 不应在 OnEnter 失败后执行")
			return "", nil
		},
		func(ctx context.Context, s *TestState) error {
			return expectedErr
		},
		nil,
	)
	sm.SetInitial("step1")

	ctx := context.Background()
	_, err := sm.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("期望 OnEnter 错误，实际无错误")
	}

	if !strings.Contains(err.Error(), "OnEnter") {
		t.Errorf("期望错误消息包含 'OnEnter'，实际得到: %v", err)
	}
}

// TestStateMachine_OnExitError 测试 OnExit 钩子返回错误时中止执行
func TestStateMachine_OnExitError(t *testing.T) {
	expectedErr := errors.New("exit hook failed")

	sm := NewStateMachine[*TestState]("exit-error")
	sm.AddStateWithHooks("step1",
		func(ctx context.Context, s *TestState) (string, error) {
			return "", nil
		},
		nil,
		func(ctx context.Context, s *TestState) error {
			return expectedErr
		},
	)
	sm.AddTransition("step1", "done", Always[*TestState]())
	sm.SetInitial("step1")
	sm.AddFinal("done")

	ctx := context.Background()
	_, err := sm.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("期望 OnExit 错误，实际无错误")
	}

	if !strings.Contains(err.Error(), "OnExit") {
		t.Errorf("期望错误消息包含 'OnExit'，实际得到: %v", err)
	}
}

// TestStateMachine_Builder 测试 StateMachineBuilder 的流式 API
func TestStateMachine_Builder(t *testing.T) {
	sm := NewBuilder[*TestState]("builder-test").
		State("init", func(ctx context.Context, s *TestState) (string, error) {
			s.Counter++
			s.Path += "init"
			return "", nil
		}).
		State("process", func(ctx context.Context, s *TestState) (string, error) {
			s.Counter += 10
			s.Path += "->process"
			return "", nil
		}).
		Transition("init", "process", Always[*TestState]()).
		Transition("process", "done", Always[*TestState]()).
		Initial("init").
		Final("done").
		MaxSteps(50).
		Build()

	ctx := context.Background()
	state := &TestState{Counter: 0, Path: "", Data: make(map[string]string)}
	result, err := sm.Run(ctx, state)

	if err != nil {
		t.Fatalf("期望无错误，实际得到: %v", err)
	}

	if result.Counter != 11 {
		t.Errorf("期望 Counter=11 (1+10)，实际得到 Counter=%d", result.Counter)
	}

	if result.Path != "init->process" {
		t.Errorf("期望 Path='init->process'，实际得到 Path='%s'", result.Path)
	}
}

// TestStateMachine_BuilderMaxSteps 测试 Builder 的 MaxSteps 设置生效
func TestStateMachine_BuilderMaxSteps(t *testing.T) {
	sm := NewBuilder[*TestState]("builder-maxsteps").
		State("loop", func(ctx context.Context, s *TestState) (string, error) {
			s.Counter++
			return "", nil
		}).
		Transition("loop", "loop", Always[*TestState]()).
		Initial("loop").
		MaxSteps(3).
		Build()

	ctx := context.Background()
	result, err := sm.Run(ctx, &TestState{Counter: 0})

	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Errorf("期望 ErrMaxStepsExceeded，实际得到: %v", err)
	}

	if result.Counter != 3 {
		t.Errorf("期望 Counter=3，实际得到 Counter=%d", result.Counter)
	}
}

// TestStateMachine_ConditionalTransition 测试多条件转换的优先级
func TestStateMachine_ConditionalTransition(t *testing.T) {
	sm := NewStateMachine[*TestState]("conditional")

	// 路由状态：根据 Counter 值决定走哪条路
	sm.AddState("router", func(ctx context.Context, s *TestState) (string, error) {
		return "", nil // 使用转换条件
	})
	sm.AddState("path_high", func(ctx context.Context, s *TestState) (string, error) {
		s.Path = "high"
		return "", nil
	})
	sm.AddState("path_low", func(ctx context.Context, s *TestState) (string, error) {
		s.Path = "low"
		return "", nil
	})
	sm.AddState("path_default", func(ctx context.Context, s *TestState) (string, error) {
		s.Path = "default"
		return "", nil
	})

	// 添加带优先级的转换条件
	// 优先级 0（最高）：Counter >= 10 -> path_high
	sm.AddTransitionWithPriority("router", "path_high",
		When[*TestState](func(s *TestState) bool { return s.Counter >= 10 }), 0)
	// 优先级 1：Counter >= 5 -> path_low
	sm.AddTransitionWithPriority("router", "path_low",
		When[*TestState](func(s *TestState) bool { return s.Counter >= 5 }), 1)
	// 优先级 2（最低）：默认走 path_default
	sm.AddTransitionWithPriority("router", "path_default", Always[*TestState](), 2)

	sm.AddTransition("path_high", "done", Always[*TestState]())
	sm.AddTransition("path_low", "done", Always[*TestState]())
	sm.AddTransition("path_default", "done", Always[*TestState]())
	sm.SetInitial("router")
	sm.AddFinal("done")

	tests := []struct {
		name     string
		counter  int
		wantPath string
	}{
		{"高值路径", 15, "high"},
		{"低值路径", 7, "low"},
		{"默认路径", 2, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			state := &TestState{Counter: tt.counter}
			result, err := sm.Run(ctx, state)

			if err != nil {
				t.Fatalf("期望无错误，实际得到: %v", err)
			}

			if result.Path != tt.wantPath {
				t.Errorf("Counter=%d 时期望 Path='%s'，实际得到 Path='%s'",
					tt.counter, tt.wantPath, result.Path)
			}
		})
	}
}

// TestStateMachine_HelperFunctions 测试 Always/Never/When/PassThrough/End/Conditional 辅助函数
func TestStateMachine_HelperFunctions(t *testing.T) {
	ctx := context.Background()
	state := &TestState{Counter: 5, Path: "test"}

	// 测试 Always：总是返回 true
	t.Run("Always", func(t *testing.T) {
		cond := Always[*TestState]()
		if !cond(ctx, state) {
			t.Error("Always 应返回 true")
		}
	})

	// 测试 Never：总是返回 false
	t.Run("Never", func(t *testing.T) {
		cond := Never[*TestState]()
		if cond(ctx, state) {
			t.Error("Never 应返回 false")
		}
	})

	// 测试 When：根据谓词函数返回结果
	t.Run("When_True", func(t *testing.T) {
		cond := When[*TestState](func(s *TestState) bool { return s.Counter > 3 })
		if !cond(ctx, state) {
			t.Error("When(Counter>3) 对 Counter=5 应返回 true")
		}
	})

	t.Run("When_False", func(t *testing.T) {
		cond := When[*TestState](func(s *TestState) bool { return s.Counter > 10 })
		if cond(ctx, state) {
			t.Error("When(Counter>10) 对 Counter=5 应返回 false")
		}
	})

	// 测试 PassThrough：返回空字符串，让转换条件决定下一状态
	t.Run("PassThrough", func(t *testing.T) {
		handler := PassThrough[*TestState]()
		next, err := handler(ctx, state)
		if err != nil {
			t.Fatalf("PassThrough 不应返回错误: %v", err)
		}
		if next != "" {
			t.Errorf("PassThrough 应返回空字符串，实际得到 '%s'", next)
		}
	})

	// 测试 End：返回指定的下一状态名
	t.Run("End", func(t *testing.T) {
		handler := End[*TestState]("final")
		next, err := handler(ctx, state)
		if err != nil {
			t.Fatalf("End 不应返回错误: %v", err)
		}
		if next != "final" {
			t.Errorf("End('final') 应返回 'final'，实际得到 '%s'", next)
		}
	})

	// 测试 Conditional：根据条件返回不同的状态名
	t.Run("Conditional_True", func(t *testing.T) {
		handler := Conditional[*TestState](
			func(s *TestState) bool { return s.Counter > 3 },
			"yes_path", "no_path",
		)
		next, err := handler(ctx, state)
		if err != nil {
			t.Fatalf("Conditional 不应返回错误: %v", err)
		}
		if next != "yes_path" {
			t.Errorf("Conditional 条件为真时应返回 'yes_path'，实际得到 '%s'", next)
		}
	})

	t.Run("Conditional_False", func(t *testing.T) {
		handler := Conditional[*TestState](
			func(s *TestState) bool { return s.Counter > 10 },
			"yes_path", "no_path",
		)
		next, err := handler(ctx, state)
		if err != nil {
			t.Fatalf("Conditional 不应返回错误: %v", err)
		}
		if next != "no_path" {
			t.Errorf("Conditional 条件为假时应返回 'no_path'，实际得到 '%s'", next)
		}
	})
}

// TestStateMachine_HandlerError 测试 handler 返回错误时中止执行
func TestStateMachine_HandlerError(t *testing.T) {
	expectedErr := errors.New("handler error")

	sm := NewStateMachine[*TestState]("handler-error")
	sm.AddState("step1", func(ctx context.Context, s *TestState) (string, error) {
		return "", expectedErr
	})
	sm.SetInitial("step1")

	ctx := context.Background()
	_, err := sm.Run(ctx, &TestState{})

	if err == nil {
		t.Fatal("期望 handler 错误，实际无错误")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("期望错误链中包含原始错误，实际得到: %v", err)
	}
}

// TestStateMachine_MultipleFinalStates 测试多个终态
func TestStateMachine_MultipleFinalStates(t *testing.T) {
	sm := NewStateMachine[*TestState]("multi-final")

	sm.AddState("router", Conditional[*TestState](
		func(s *TestState) bool { return s.Counter > 0 },
		"success", "failure",
	))

	// success 和 failure 都是终态
	sm.SetInitial("router")
	sm.AddFinal("success", "failure")

	tests := []struct {
		name    string
		counter int
	}{
		{"到达 success 终态", 1},
		{"到达 failure 终态", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			state := &TestState{Counter: tt.counter}
			_, err := sm.Run(ctx, state)

			if err != nil {
				t.Fatalf("期望无错误，实际得到: %v", err)
			}
		})
	}
}

// TestStateMachine_InitialStateIsFinal 测试初始状态同时是终态时立即结束
func TestStateMachine_InitialStateIsFinal(t *testing.T) {
	sm := NewStateMachine[*TestState]("init-final")

	// 不需要添加 handler，因为终态会在执行前被检测
	sm.SetInitial("done")
	sm.AddFinal("done")

	ctx := context.Background()
	state := &TestState{Counter: 42}
	result, err := sm.Run(ctx, state)

	if err != nil {
		t.Fatalf("期望无错误，实际得到: %v", err)
	}

	// Counter 应保持不变，因为没有执行任何 handler
	if result.Counter != 42 {
		t.Errorf("期望 Counter=42（未被修改），实际得到 Counter=%d", result.Counter)
	}
}
