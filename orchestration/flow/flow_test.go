package flow

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestFlowBuilder_Basic 测试创建和执行基本流
func TestFlowBuilder_Basic(t *testing.T) {
	type MyState struct {
		Counter int
		Data    string
	}

	startStep := func(ctx context.Context, state MyState) (MyState, error) {
		state.Counter = 1
		state.Data = "started"
		return state, nil
	}

	listenStep := func(ctx context.Context, state MyState) (MyState, error) {
		state.Counter++
		state.Data += " -> processed"
		return state, nil
	}

	flow, err := NewFlow[MyState]("test-flow").
		Start("start", startStep).
		Listen("process", listenStep, "start").
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	if flow.Name() != "test-flow" {
		t.Errorf("流名称不匹配: got %q, want %q", flow.Name(), "test-flow")
	}

	ctx := context.Background()
	result, err := flow.Run(ctx, MyState{})
	if err != nil {
		t.Fatalf("执行流失败: %v", err)
	}

	if result.Counter != 2 {
		t.Errorf("Counter 值不正确: got %d, want 2", result.Counter)
	}

	if result.Data != "started -> processed" {
		t.Errorf("Data 值不正确: got %q, want %q", result.Data, "started -> processed")
	}
}

// TestFlowBuilder_ListenAll 测试 ListenAll（等待所有事件完成）
func TestFlowBuilder_ListenAll(t *testing.T) {
	type State struct {
		Steps []string
	}

	var counter atomic.Int32

	start1 := func(ctx context.Context, state State) (State, error) {
		counter.Add(1)
		state.Steps = append(state.Steps, "start1")
		return state, nil
	}

	start2 := func(ctx context.Context, state State) (State, error) {
		counter.Add(1)
		state.Steps = append(state.Steps, "start2")
		return state, nil
	}

	mergeStep := func(ctx context.Context, state State) (State, error) {
		state.Steps = append(state.Steps, "merged")
		return state, nil
	}

	flow, err := NewFlow[State]("test-listen-all").
		Start("s1", start1).
		Start("s2", start2).
		ListenAll("merge", mergeStep, "s1", "s2").
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	ctx := context.Background()
	result, err := flow.Run(ctx, State{Steps: make([]string, 0)})
	if err != nil {
		t.Fatalf("执行流失败: %v", err)
	}

	// 验证 merge 步骤在所有 start 步骤完成后执行
	found := false
	for _, step := range result.Steps {
		if step == "merged" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("merge 步骤未执行: %v", result.Steps)
	}

	// 验证并发执行
	if counter.Load() != 2 {
		t.Errorf("Start 步骤执行次数不正确: got %d, want 2", counter.Load())
	}
}

// TestFlowBuilder_ListenAny 测试 ListenAny（任一事件完成触发）
func TestFlowBuilder_ListenAny(t *testing.T) {
	type State struct {
		Executed  string
		Triggered bool
	}

	fastStep := func(ctx context.Context, state State) (State, error) {
		// 快速完成
		state.Executed = "fast"
		return state, nil
	}

	slowStep := func(ctx context.Context, state State) (State, error) {
		// 延迟完成
		time.Sleep(200 * time.Millisecond)
		if state.Executed == "" {
			state.Executed = "slow"
		}
		return state, nil
	}

	anyStep := func(ctx context.Context, state State) (State, error) {
		// 只在第一次触发时记录
		if !state.Triggered {
			state.Executed += " -> any-triggered"
			state.Triggered = true
		}
		return state, nil
	}

	flow, err := NewFlow[State]("test-listen-any").
		Start("fast", fastStep).
		Start("slow", slowStep).
		ListenAny("any", anyStep, "fast", "slow").
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	ctx := context.Background()
	result, err := flow.Run(ctx, State{})
	if err != nil {
		t.Fatalf("执行流失败: %v", err)
	}

	// 验证 any 步骤被触发
	if !result.Triggered {
		t.Errorf("any 步骤应该被触发")
	}

	// 验证输出包含 any-triggered
	if result.Executed != "fast -> any-triggered" && result.Executed != "slow -> any-triggered" {
		t.Errorf("any 步骤执行结果不正确: got %q", result.Executed)
	}
}

// TestFlowBuilder_NoStart 测试无 Start 步骤时返回错误
func TestFlowBuilder_NoStart(t *testing.T) {
	type State struct{}

	step := func(ctx context.Context, state State) (State, error) {
		return state, nil
	}

	_, err := NewFlow[State]("no-start").
		Listen("listen", step, "nonexistent").
		Build()

	if err == nil {
		t.Fatal("期望构建失败，但成功了")
	}

	expectedMsg := "至少需要一个 Start 步骤"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("错误消息不匹配: got %q, want contain %q", err.Error(), expectedMsg)
	}
}

// TestFlowBuilder_DuplicateStep 测试重复步骤名称时返回错误
func TestFlowBuilder_DuplicateStep(t *testing.T) {
	type State struct{}

	step := func(ctx context.Context, state State) (State, error) {
		return state, nil
	}

	_, err := NewFlow[State]("duplicate").
		Start("step1", step).
		Start("step1", step). // 重复名称
		Build()

	if err == nil {
		t.Fatal("期望构建失败，但成功了")
	}

	expectedMsg := "已存在"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("错误消息不匹配: got %q, want contain %q", err.Error(), expectedMsg)
	}
}

// TestFlow_Name 测试流名称
func TestFlow_Name(t *testing.T) {
	type State struct{}

	step := func(ctx context.Context, state State) (State, error) {
		return state, nil
	}

	flow, err := NewFlow[State]("my-flow").
		Start("start", step).
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	if flow.Name() != "my-flow" {
		t.Errorf("流名称不匹配: got %q, want %q", flow.Name(), "my-flow")
	}
}

// TestFlow_ContextCancellation 测试上下文取消
func TestFlow_ContextCancellation(t *testing.T) {
	type State struct {
		Started bool
	}

	slowStep := func(ctx context.Context, state State) (State, error) {
		state.Started = true
		// 模拟长时间运行
		select {
		case <-time.After(5 * time.Second):
			return state, nil
		case <-ctx.Done():
			return state, ctx.Err()
		}
	}

	flow, err := NewFlow[State]("cancel-test").
		Start("slow", slowStep).
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = flow.Run(ctx, State{})
	if err == nil {
		t.Fatal("期望取消错误，但成功了")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("错误类型不匹配: got %v, want context.DeadlineExceeded", err)
	}
}

// TestFlow_StepError 测试步骤返回错误时的处理
func TestFlow_StepError(t *testing.T) {
	type State struct{}

	failStep := func(ctx context.Context, state State) (State, error) {
		return state, errors.New("步骤失败")
	}

	successStep := func(ctx context.Context, state State) (State, error) {
		return state, nil
	}

	flow, err := NewFlow[State]("error-test").
		Start("fail", failStep).
		Listen("success", successStep, "fail").
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	ctx := context.Background()
	_, err = flow.Run(ctx, State{})
	if err == nil {
		t.Fatal("期望步骤错误，但成功了")
	}

	expectedMsg := "步骤失败"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("错误消息不匹配: got %q, want contain %q", err.Error(), expectedMsg)
	}
}

// TestFlow_MultipleStarts 测试多个并行启动步骤
func TestFlow_MultipleStarts(t *testing.T) {
	type State struct {
		Count int
	}

	var mu atomic.Int32

	makeStep := func(id int) StepFunc[State] {
		return func(ctx context.Context, state State) (State, error) {
			mu.Add(1)
			state.Count++
			return state, nil
		}
	}

	flow, err := NewFlow[State]("multi-start").
		Start("s1", makeStep(1)).
		Start("s2", makeStep(2)).
		Start("s3", makeStep(3)).
		Build()

	if err != nil {
		t.Fatalf("构建流失败: %v", err)
	}

	ctx := context.Background()
	_, err = flow.Run(ctx, State{})
	if err != nil {
		t.Fatalf("执行流失败: %v", err)
	}

	// 验证所有 start 步骤都执行了
	if mu.Load() != 3 {
		t.Errorf("Start 步骤执行次数不正确: got %d, want 3", mu.Load())
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
