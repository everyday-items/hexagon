package process

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewProcess 测试创建流程
func TestNewProcess(t *testing.T) {
	process, err := NewProcess("test-process").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "finish", "end").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if process.Name() != "test-process" {
		t.Errorf("Name() = %v, want test-process", process.Name())
	}
}

// TestProcessStateMachine 测试状态机基本流转
func TestProcessStateMachine(t *testing.T) {
	process, err := NewProcess("order-process").
		AddState("pending", AsInitial()).
		AddState("processing").
		AddState("completed", AsFinal()).
		AddTransition("pending", "process", "processing").
		AddTransition("processing", "complete", "completed").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()

	// 启动流程
	err = process.Start(ctx, ProcessInput{
		Data: map[string]any{"order_id": "123"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 检查初始状态
	if process.CurrentState().Name() != "pending" {
		t.Errorf("CurrentState() = %v, want pending", process.CurrentState().Name())
	}

	// 发送事件转换到 processing
	err = process.SendEvent(ctx, NewEvent("process"))
	if err != nil {
		t.Fatalf("SendEvent(process) error = %v", err)
	}

	if process.CurrentState().Name() != "processing" {
		t.Errorf("CurrentState() = %v, want processing", process.CurrentState().Name())
	}

	// 发送事件转换到 completed
	err = process.SendEvent(ctx, NewEvent("complete"))
	if err != nil {
		t.Fatalf("SendEvent(complete) error = %v", err)
	}

	if process.CurrentState().Name() != "completed" {
		t.Errorf("CurrentState() = %v, want completed", process.CurrentState().Name())
	}

	// 检查流程状态
	if process.Status() != StatusCompleted {
		t.Errorf("Status() = %v, want completed", process.Status())
	}
}

// TestProcessWithSteps 测试带步骤的流程
func TestProcessWithSteps(t *testing.T) {
	stepExecuted := false

	process, err := NewProcess("step-process").
		AddState("start", AsInitial()).
		AddState("processing").
		AddState("end", AsFinal()).
		AddTransition("start", "go", "processing").
		AddTransition("processing", "done", "end").
		OnStateEnterFunc("processing", func(ctx context.Context, data *ProcessData) (any, error) {
			stepExecuted = true
			data.Set("step_result", "processed")
			return "step output", nil
		}).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()

	// 启动并执行
	err = process.Start(ctx, ProcessInput{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 触发转换
	err = process.SendEvent(ctx, NewEvent("go"))
	if err != nil {
		t.Fatalf("SendEvent() error = %v", err)
	}

	// 验证步骤被执行
	if !stepExecuted {
		t.Error("步骤未被执行")
	}

	// 验证数据被设置
	result, ok := process.GetData().Get("step_result")
	if !ok || result != "processed" {
		t.Errorf("step_result = %v, want processed", result)
	}
}

// TestProcessWithGuard 测试带守卫条件的转换
func TestProcessWithGuard(t *testing.T) {
	_, err := NewProcess("guard-process").
		AddState("start", AsInitial()).
		AddState("success", AsFinal()).
		AddState("failure", AsFinal()).
		AddTransition("start", "check", "success",
			WithGuard(func(ctx context.Context, data *ProcessData) bool {
				return data.GetBool("valid")
			})).
		AddTransition("start", "check", "failure",
			WithGuard(func(ctx context.Context, data *ProcessData) bool {
				return !data.GetBool("valid")
			})).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()

	// 测试 valid = true
	t.Run("valid=true", func(t *testing.T) {
		p, _ := NewProcess("guard-test").
			AddState("start", AsInitial()).
			AddState("success", AsFinal()).
			AddState("failure", AsFinal()).
			AddTransition("start", "check", "success",
				WithGuard(func(ctx context.Context, data *ProcessData) bool {
					return data.GetBool("valid")
				})).
			AddTransition("start", "check", "failure",
				WithGuard(func(ctx context.Context, data *ProcessData) bool {
					return !data.GetBool("valid")
				})).
			Build()

		p.Start(ctx, ProcessInput{Data: map[string]any{"valid": true}})
		p.SendEvent(ctx, NewEvent("check"))

		if p.CurrentState().Name() != "success" {
			t.Errorf("CurrentState() = %v, want success", p.CurrentState().Name())
		}
	})

	// 测试 valid = false
	t.Run("valid=false", func(t *testing.T) {
		p, _ := NewProcess("guard-test").
			AddState("start", AsInitial()).
			AddState("success", AsFinal()).
			AddState("failure", AsFinal()).
			AddTransition("start", "check", "success",
				WithGuard(func(ctx context.Context, data *ProcessData) bool {
					return data.GetBool("valid")
				})).
			AddTransition("start", "check", "failure",
				WithGuard(func(ctx context.Context, data *ProcessData) bool {
					return !data.GetBool("valid")
				})).
			Build()

		p.Start(ctx, ProcessInput{Data: map[string]any{"valid": false}})
		p.SendEvent(ctx, NewEvent("check"))

		if p.CurrentState().Name() != "failure" {
			t.Errorf("CurrentState() = %v, want failure", p.CurrentState().Name())
		}
	})
}

// TestProcessWithAction 测试带动作的转换
func TestProcessWithAction(t *testing.T) {
	actionExecuted := false

	process, err := NewProcess("action-process").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end",
			WithAction(func(ctx context.Context, data *ProcessData) error {
				actionExecuted = true
				data.Set("action_result", "done")
				return nil
			})).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})
	process.SendEvent(ctx, NewEvent("go"))

	if !actionExecuted {
		t.Error("转换动作未被执行")
	}

	result, _ := process.GetData().Get("action_result")
	if result != "done" {
		t.Errorf("action_result = %v, want done", result)
	}
}

// TestProcessAutoTransition 测试自动转换
func TestProcessAutoTransition(t *testing.T) {
	process, err := NewProcess("auto-process").
		AddState("start", AsInitial()).
		AddState("auto_state").
		AddState("end", AsFinal()).
		AddTransition("start", "go", "auto_state").
		AddAutoTransition("auto_state", "end", func(ctx context.Context, data *ProcessData) bool {
			return true // 总是自动转换
		}).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})
	process.SendEvent(ctx, NewEvent("go"))

	// 应该自动转换到 end
	if process.CurrentState().Name() != "end" {
		t.Errorf("CurrentState() = %v, want end", process.CurrentState().Name())
	}

	if process.Status() != StatusCompleted {
		t.Errorf("Status() = %v, want completed", process.Status())
	}
}

// TestProcessInvoke 测试 Invoke 方法
func TestProcessInvoke(t *testing.T) {
	process, err := NewProcess("invoke-process").
		AddState("start", AsInitial()).
		AddState("processing").
		AddState("end", AsFinal()).
		AddTransition("start", "_auto_", "processing",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
		AddTransition("processing", "_auto_", "end",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
		OnStateEnterFunc("processing", func(ctx context.Context, data *ProcessData) (any, error) {
			data.Set("result", "processed")
			return "output", nil
		}).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	output, err := process.Invoke(ctx, ProcessInput{Data: map[string]any{"input": "test"}})

	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if output.FinalState != "end" {
		t.Errorf("FinalState = %v, want end", output.FinalState)
	}

	if output.ExecutionTime <= 0 {
		t.Error("ExecutionTime should be > 0")
	}
}

// TestProcessHistory 测试执行历史
func TestProcessHistory(t *testing.T) {
	process, err := NewProcess("history-process").
		AddState("a", AsInitial()).
		AddState("b").
		AddState("c", AsFinal()).
		AddTransition("a", "to_b", "b").
		AddTransition("b", "to_c", "c").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})
	process.SendEvent(ctx, NewEvent("to_b"))
	process.SendEvent(ctx, NewEvent("to_c"))

	history := process.GetHistory()

	// 应该有多条记录
	if len(history) < 3 {
		t.Errorf("历史记录数量 = %d, 期望至少 3 条", len(history))
	}

	// 检查有状态转换记录
	hasTransition := false
	for _, record := range history {
		if record.Type == RecordTypeTransition {
			hasTransition = true
			break
		}
	}

	if !hasTransition {
		t.Error("历史记录中没有状态转换记录")
	}
}

// TestProcessEventSubscription 测试事件订阅
func TestProcessEventSubscription(t *testing.T) {
	events := make([]ProcessEvent, 0)

	process, err := NewProcess("event-process").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// 订阅事件
	process.Subscribe(func(event ProcessEvent) {
		events = append(events, event)
	})

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})
	process.SendEvent(ctx, NewEvent("go"))

	// 应该收到多个事件
	if len(events) < 3 {
		t.Errorf("收到事件数量 = %d, 期望至少 3 个", len(events))
	}

	// 检查事件类型
	hasProcessStart := false
	hasTransition := false
	hasProcessEnd := false

	for _, event := range events {
		switch event.Type {
		case EventTypeProcessStart:
			hasProcessStart = true
		case EventTypeTransition:
			hasTransition = true
		case EventTypeProcessEnd:
			hasProcessEnd = true
		}
	}

	if !hasProcessStart {
		t.Error("没有收到 ProcessStart 事件")
	}
	if !hasTransition {
		t.Error("没有收到 Transition 事件")
	}
	if !hasProcessEnd {
		t.Error("没有收到 ProcessEnd 事件")
	}
}

// TestProcessPauseResume 测试暂停和恢复
func TestProcessPauseResume(t *testing.T) {
	process, err := NewProcess("pause-process").
		AddState("start", AsInitial()).
		AddState("middle").
		AddState("end", AsFinal()).
		AddTransition("start", "go", "middle").
		AddTransition("middle", "finish", "end").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})
	process.SendEvent(ctx, NewEvent("go"))

	// 暂停
	err = process.Pause(ctx)
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	if process.Status() != StatusPaused {
		t.Errorf("Status() = %v, want paused", process.Status())
	}

	// 暂停时发送事件应该失败
	err = process.SendEvent(ctx, NewEvent("finish"))
	if err == nil {
		t.Error("暂停时发送事件应该失败")
	}

	// 恢复
	err = process.Resume(ctx)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if process.Status() != StatusRunning {
		t.Errorf("Status() = %v, want running", process.Status())
	}

	// 恢复后可以继续
	err = process.SendEvent(ctx, NewEvent("finish"))
	if err != nil {
		t.Fatalf("恢复后 SendEvent() error = %v", err)
	}
}

// TestProcessCancel 测试取消
func TestProcessCancel(t *testing.T) {
	process, err := NewProcess("cancel-process").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	process.Start(ctx, ProcessInput{})

	// 取消
	err = process.Cancel(ctx)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	if process.Status() != StatusCancelled {
		t.Errorf("Status() = %v, want cancelled", process.Status())
	}

	// 取消后发送事件应该失败
	err = process.SendEvent(ctx, NewEvent("go"))
	if err == nil {
		t.Error("取消后发送事件应该失败")
	}
}

// TestProcessData 测试流程数据操作
func TestProcessData(t *testing.T) {
	data := NewProcessData(map[string]any{
		"input_string": "hello",
		"input_int":    42,
		"input_bool":   true,
	})

	// 测试获取输入数据
	if data.GetString("input_string") != "hello" {
		t.Errorf("GetString() = %v, want hello", data.GetString("input_string"))
	}

	if data.GetInt("input_int") != 42 {
		t.Errorf("GetInt() = %v, want 42", data.GetInt("input_int"))
	}

	if !data.GetBool("input_bool") {
		t.Error("GetBool() should return true")
	}

	// 测试设置变量
	data.Set("var1", "value1")
	v, ok := data.Get("var1")
	if !ok || v != "value1" {
		t.Errorf("Get(var1) = %v, want value1", v)
	}

	// 测试步骤输出
	data.SetStepOutput("step1", map[string]any{"result": "ok"})
	output, ok := data.GetStepOutput("step1")
	if !ok {
		t.Error("GetStepOutput() 失败")
	}
	if m, ok := output.(map[string]any); !ok || m["result"] != "ok" {
		t.Errorf("GetStepOutput() = %v, 格式不对", output)
	}

	// 测试克隆
	clone := data.Clone()
	clone.Set("new_var", "new_value")

	// 原始数据不应该受影响
	_, ok = data.Get("new_var")
	if ok {
		t.Error("克隆后修改不应该影响原始数据")
	}
}

// TestProcessValidation 测试流程定义验证
func TestProcessValidation(t *testing.T) {
	// 测试没有初始状态
	t.Run("no initial state", func(t *testing.T) {
		_, err := NewProcess("invalid").
			AddState("a").
			AddState("b", AsFinal()).
			AddTransition("a", "go", "b").
			Build()

		if !errors.Is(err, ErrNoInitialState) {
			t.Errorf("期望 ErrNoInitialState, got %v", err)
		}
	})

	// 测试没有终止状态
	t.Run("no final state", func(t *testing.T) {
		_, err := NewProcess("invalid").
			AddState("a", AsInitial()).
			AddState("b").
			AddTransition("a", "go", "b").
			Build()

		if !errors.Is(err, ErrNoFinalState) {
			t.Errorf("期望 ErrNoFinalState, got %v", err)
		}
	})

	// 测试转换引用不存在的状态
	t.Run("invalid transition", func(t *testing.T) {
		_, err := NewProcess("invalid").
			AddState("a", AsInitial()).
			AddState("b", AsFinal()).
			AddTransition("a", "go", "nonexistent").
			Build()

		if err == nil {
			t.Error("期望验证失败")
		}
	})
}

// TestProcessStatus 测试状态方法
func TestProcessStatus(t *testing.T) {
	tests := []struct {
		status     ProcessStatus
		isTerminal bool
	}{
		{StatusPending, false},
		{StatusRunning, false},
		{StatusPaused, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if tt.status.IsTerminal() != tt.isTerminal {
				t.Errorf("%s.IsTerminal() = %v, want %v", tt.status, tt.status.IsTerminal(), tt.isTerminal)
			}
		})
	}
}

// TestEvent 测试事件
func TestEvent(t *testing.T) {
	event := NewEvent("test").
		WithData("key1", "value1").
		WithData("key2", 42).
		WithSource("test_source")

	if event.Name != "test" {
		t.Errorf("Name = %v, want test", event.Name)
	}

	if event.Data["key1"] != "value1" {
		t.Errorf("Data[key1] = %v, want value1", event.Data["key1"])
	}

	if event.Data["key2"] != 42 {
		t.Errorf("Data[key2] = %v, want 42", event.Data["key2"])
	}

	if event.Source != "test_source" {
		t.Errorf("Source = %v, want test_source", event.Source)
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

// BenchmarkProcessExecution 基准测试流程执行
func BenchmarkProcessExecution(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		process, _ := NewProcess("bench-process").
			AddState("start", AsInitial()).
			AddState("end", AsFinal()).
			AddTransition("start", "_auto_", "end",
				WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
			Build()

		process.Invoke(ctx, ProcessInput{})
	}
}

// BenchmarkProcessWithSteps 带步骤的流程基准测试
func BenchmarkProcessWithSteps(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		process, _ := NewProcess("bench-step-process").
			AddState("start", AsInitial()).
			AddState("step1").
			AddState("step2").
			AddState("end", AsFinal()).
			AddTransition("start", "_auto_", "step1",
				WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
			AddTransition("step1", "_auto_", "step2",
				WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
			AddTransition("step2", "_auto_", "end",
				WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
			OnStateEnterFunc("step1", func(ctx context.Context, data *ProcessData) (any, error) {
				return "step1 output", nil
			}).
			OnStateEnterFunc("step2", func(ctx context.Context, data *ProcessData) (any, error) {
				return "step2 output", nil
			}).
			Build()

		process.Invoke(ctx, ProcessInput{})
	}
}

// TestProcessConcurrency 测试并发安全
func TestProcessConcurrency(t *testing.T) {
	ctx := context.Background()

	// 创建 100 个流程并发执行
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			process, _ := NewProcess("concurrent-process").
				AddState("start", AsInitial()).
				AddState("end", AsFinal()).
				AddTransition("start", "_auto_", "end",
					WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
				Build()

			_, _ = process.Invoke(ctx, ProcessInput{})
			done <- true
		}()
	}

	// 等待所有完成
	timeout := time.After(10 * time.Second)
	for i := 0; i < 100; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("并发测试超时")
		}
	}
}
