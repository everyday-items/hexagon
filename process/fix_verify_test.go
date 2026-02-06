package process

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============== 修复 #1 验证：死锁修复 ==============

// TestFix1_PauseResumeNoDeadlockWithSubscriber 验证 Pause/Resume 在有事件订阅者时不会死锁
// 修复前：handler 中调用 p.Status() 需要读锁，而 Pause 持有写锁调用 handler → 死锁
// 修复后：Pause 在释放写锁后再调用 publishEvent，handler 可安全获取读锁
func TestFix1_PauseResumeNoDeadlockWithSubscriber(t *testing.T) {
	p, err := NewProcess("deadlock-test").
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

	// 订阅者在回调中读取流程状态（需要读锁）
	var eventStatuses []ProcessStatus
	var mu sync.Mutex
	p.Subscribe(func(event ProcessEvent) {
		status := p.Status()
		mu.Lock()
		eventStatuses = append(eventStatuses, status)
		mu.Unlock()
	})

	p.Start(ctx, ProcessInput{})
	p.SendEvent(ctx, NewEvent("go"))

	done := make(chan struct{})
	go func() {
		p.Pause(ctx)
		p.Resume(ctx)
		close(done)
	}()

	select {
	case <-done:
		// 验证 handler 确实被调用了
		mu.Lock()
		count := len(eventStatuses)
		mu.Unlock()
		if count == 0 {
			t.Error("事件处理器未被调用")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Pause/Resume 死锁！修复无效")
	}
}

// TestFix1_CancelNoDeadlockWithSubscriber 验证 Cancel 在有事件订阅者时不会死锁
func TestFix1_CancelNoDeadlockWithSubscriber(t *testing.T) {
	p, err := NewProcess("cancel-deadlock-test").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end").
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()

	// 订阅者在回调中读取流程数据（需要读锁）
	p.Subscribe(func(event ProcessEvent) {
		_ = p.GetData()
		_ = p.GetHistory()
	})

	p.Start(ctx, ProcessInput{Data: map[string]any{"key": "value"}})

	done := make(chan struct{})
	go func() {
		p.Cancel(ctx)
		close(done)
	}()

	select {
	case <-done:
		if p.Status() != StatusCancelled {
			t.Errorf("Status() = %v, want cancelled", p.Status())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Cancel 死锁！修复无效")
	}
}

// TestFix1_CompleteNoDeadlockWithSubscriber 验证流程完成时 handler 中读取状态不死锁
func TestFix1_CompleteNoDeadlockWithSubscriber(t *testing.T) {
	p, err := NewProcess("complete-deadlock-test").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end").
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()

	var completedStatus ProcessStatus
	p.Subscribe(func(event ProcessEvent) {
		if event.Type == EventTypeProcessEnd {
			completedStatus = p.Status()
		}
	})

	p.Start(ctx, ProcessInput{})

	done := make(chan struct{})
	go func() {
		p.SendEvent(ctx, NewEvent("go"))
		close(done)
	}()

	select {
	case <-done:
		if completedStatus != StatusCompleted {
			t.Errorf("handler 中读到的 Status = %v, want completed", completedStatus)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("complete() 死锁！修复无效")
	}
}

// ============== 修复 #2 验证：并发 SendEvent 竞争 ==============

// TestFix2_ConcurrentSendEventSerializes 验证并发 SendEvent 被正确序列化
// 修复前：两个并发 SendEvent 都能从同一状态转出 → 状态机语义被破坏
// 修复后：transitionMu 序列化所有 SendEvent，只有一个能成功
func TestFix2_ConcurrentSendEventSerializes(t *testing.T) {
	for trial := 0; trial < 100; trial++ {
		p, _ := NewProcess("race-test").
			AddState("start", AsInitial()).
			AddState("a").
			AddState("b").
			AddState("end_a", AsFinal()).
			AddState("end_b", AsFinal()).
			AddTransition("start", "go_a", "a").
			AddTransition("start", "go_b", "b").
			AddTransition("a", "finish", "end_a").
			AddTransition("b", "finish", "end_b").
			Build()

		ctx := context.Background()
		p.Start(ctx, ProcessInput{})

		var wg sync.WaitGroup
		wg.Add(2)

		var successCount int32

		go func() {
			defer wg.Done()
			if p.SendEvent(ctx, NewEvent("go_a")) == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
		go func() {
			defer wg.Done()
			if p.SendEvent(ctx, NewEvent("go_b")) == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()

		wg.Wait()

		count := atomic.LoadInt32(&successCount)
		if count != 1 {
			t.Fatalf("trial %d: 期望恰好 1 个 SendEvent 成功，实际 %d 个成功（状态: %s）",
				trial, count, p.CurrentState().Name())
		}
	}
}

// ============== 修复 #3 验证：Guard 双重检查 TOCTOU ==============

// TestFix3_GuardCalledOnce 验证 Guard 只被调用一次
// 修复前：Guard 在 SendEvent 和 executeTransition 各调用一次 → TOCTOU
// 修复后：executeTransition 不再检查 Guard，只在 SendEvent 中检查一次
func TestFix3_GuardCalledOnce(t *testing.T) {
	var callCount int32

	p, err := NewProcess("double-check").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end",
			WithGuard(func(ctx context.Context, data *ProcessData) bool {
				atomic.AddInt32(&callCount, 1)
				return true
			})).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	p.Start(ctx, ProcessInput{})
	err = p.SendEvent(ctx, NewEvent("go"))
	if err != nil {
		t.Fatalf("SendEvent() error = %v", err)
	}

	count := atomic.LoadInt32(&callCount)
	if count != 1 {
		t.Errorf("Guard 被调用了 %d 次，期望只调用 1 次", count)
	}

	if p.CurrentState().Name() != "end" {
		t.Errorf("CurrentState() = %v, want end", p.CurrentState().Name())
	}
}

// TestFix3_GuardTOCTOUFixed 验证 TOCTOU 场景被修复
// 模拟 Guard 第一次返回 true，第二次返回 false 的情况
func TestFix3_GuardTOCTOUFixed(t *testing.T) {
	var callCount int32

	p, err := NewProcess("toctou-fix").
		AddState("start", AsInitial()).
		AddState("end", AsFinal()).
		AddTransition("start", "go", "end",
			WithGuard(func(ctx context.Context, data *ProcessData) bool {
				count := atomic.AddInt32(&callCount, 1)
				return count == 1 // 只有第一次返回 true
			})).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	p.Start(ctx, ProcessInput{})
	err = p.SendEvent(ctx, NewEvent("go"))

	// 修复后只调用一次 Guard，第一次返回 true → 转换成功
	if err != nil {
		t.Errorf("SendEvent() error = %v, 修复后应成功", err)
	}
	if p.CurrentState().Name() != "end" {
		t.Errorf("CurrentState() = %v, want end", p.CurrentState().Name())
	}
}

// ============== 修复 #4 验证：SendEvent 按优先级排序 ==============

// TestFix4_SendEventRespectsPriority 验证 SendEvent 按优先级选择转换
// 修复前：按定义顺序选择（优先级无效）
// 修复后：与 processAutoTransitions 一致，高优先级优先
func TestFix4_SendEventRespectsPriority(t *testing.T) {
	p, err := NewProcess("priority-guard-test").
		AddState("start", AsInitial()).
		AddState("high", AsFinal()).
		AddState("low", AsFinal()).
		// 低优先级先定义
		AddTransition("start", "check", "low",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(1)).
		// 高优先级后定义
		AddTransition("start", "check", "high",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(10)).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx := context.Background()
	p.Start(ctx, ProcessInput{})
	p.SendEvent(ctx, NewEvent("check"))

	// 修复后应该选择高优先级的转换
	if p.CurrentState().Name() != "high" {
		t.Errorf("CurrentState() = %v, want high（高优先级应优先）", p.CurrentState().Name())
	}
}

// TestFix4_PriorityConsistentWithAutoTransition 验证 SendEvent 和 autoTransition 的优先级行为一致
func TestFix4_PriorityConsistentWithAutoTransition(t *testing.T) {
	// 手动事件触发的优先级
	p1, _ := NewProcess("manual-priority").
		AddState("start", AsInitial()).
		AddState("high", AsFinal()).
		AddState("low", AsFinal()).
		AddTransition("start", "go", "low",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(1)).
		AddTransition("start", "go", "high",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(10)).
		Build()

	ctx := context.Background()
	p1.Start(ctx, ProcessInput{})
	p1.SendEvent(ctx, NewEvent("go"))
	manualResult := p1.CurrentState().Name()

	// 自动转换的优先级（通过 AddTransition 直接设置优先级）
	p2Auto, _ := NewProcess("auto-priority-2").
		AddState("start", AsInitial()).
		AddState("high", AsFinal()).
		AddState("low", AsFinal()).
		AddTransition("start", "_auto_", "low",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(1)).
		AddTransition("start", "_auto_", "high",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true }),
			WithTransitionPriority(10)).
		Build()

	p2Auto.Start(ctx, ProcessInput{})
	// Start 内部会调用 processAutoTransitions
	autoResult := p2Auto.CurrentState().Name()

	if manualResult != autoResult {
		t.Errorf("行为不一致: SendEvent 选了 %s, autoTransition 选了 %s", manualResult, autoResult)
	}
	if manualResult != "high" {
		t.Errorf("期望选择高优先级 high, 实际 %s", manualResult)
	}
}

// ============== 修复 #5 验证：handler panic 保护 ==============

// TestFix5_HandlerPanicDoesNotCrashProcess 验证 handler panic 不会崩溃流程
func TestFix5_HandlerPanicDoesNotCrashProcess(t *testing.T) {
	p, err := NewProcess("panic-test").
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
	panicCount := 0
	normalCount := 0

	// 第一个 handler: 会 panic
	p.Subscribe(func(event ProcessEvent) {
		if event.Type == EventTypeTransition {
			panicCount++
			panic("handler panic!")
		}
	})

	// 第二个 handler: 正常执行
	p.Subscribe(func(event ProcessEvent) {
		if event.Type == EventTypeTransition {
			normalCount++
		}
	})

	p.Start(ctx, ProcessInput{})
	p.SendEvent(ctx, NewEvent("go"))
	p.SendEvent(ctx, NewEvent("finish"))

	// 验证流程正常完成
	if p.Status() != StatusCompleted {
		t.Errorf("Status() = %v, want completed", p.Status())
	}

	// 验证第二个 handler 也被调用了（panic 不影响后续 handler）
	if normalCount == 0 {
		t.Error("第二个 handler 未被调用，panic 不应影响后续 handler")
	}
}

// TestFix5_HandlerPanicInPauseDoesNotCorruptState 验证 Pause 时 handler panic 不会破坏状态
func TestFix5_HandlerPanicInPauseDoesNotCorruptState(t *testing.T) {
	p, err := NewProcess("pause-panic-test").
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

	p.Subscribe(func(event ProcessEvent) {
		if event.Type == EventTypeProcessPaused {
			panic("pause handler panic!")
		}
	})

	p.Start(ctx, ProcessInput{})
	p.SendEvent(ctx, NewEvent("go"))

	// Pause 时 handler panic
	err = p.Pause(ctx)
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	// 状态应该正确设置为 paused
	if p.Status() != StatusPaused {
		t.Errorf("Status() = %v, want paused", p.Status())
	}

	// 应该还能正常 Resume
	err = p.Resume(ctx)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if p.Status() != StatusRunning {
		t.Errorf("Status() = %v, want running", p.Status())
	}
}

// ============== 修复 #6 验证：移除 GetTransitionByEvent ==============

// TestFix6_GetTransitionsByEventWorks 验证 GetTransitionsByEvent 正常工作（替代旧方法）
func TestFix6_GetTransitionsByEventWorks(t *testing.T) {
	builder := NewProcess("transitions-test").
		AddState("start", AsInitial()).
		AddState("a", AsFinal()).
		AddState("b", AsFinal()).
		AddTransition("start", "go", "a",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return true })).
		AddTransition("start", "go", "b",
			WithGuard(func(ctx context.Context, data *ProcessData) bool { return false }))

	p, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// 使用 GetTransitionsByEvent 获取所有转换
	pi := p.(*ProcessInstance)
	transitions := pi.definition.GetTransitionsByEvent("start", "go")
	if len(transitions) != 2 {
		t.Errorf("GetTransitionsByEvent 返回 %d 个转换, 期望 2 个", len(transitions))
	}
}
