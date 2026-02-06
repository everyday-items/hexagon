// Package process 提供确定性业务流程框架
//
// 本文件实现流程执行器，负责流程实例的执行和状态管理。
package process

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/core"
	"github.com/everyday-items/hexagon/stream"
	"github.com/everyday-items/toolkit/util/idgen"
)

// ProcessInstance 流程实例
// 实现 Process 接口，表示一个正在执行的流程
type ProcessInstance struct {
	// 流程定义
	definition *ProcessDefinition

	// 流程实例 ID
	id string

	// 当前状态
	currentState *StateImpl

	// 流程执行状态
	status ProcessStatus

	// 流程数据
	data *ProcessData

	// 执行历史
	history []ExecutionRecord

	// 事件订阅者
	handlers []EventHandler

	// 开始时间
	startTime time.Time

	// 结束时间
	endTime time.Time

	// 步骤结果
	stepResults map[string]StepResult

	// 错误信息
	lastError error

	// mu 保护字段的并发读写，仅用于短时间持有
	// 重要：持有 mu 时绝不能调用事件处理器（handler），否则会因 Go RWMutex 不可重入而死锁
	mu sync.RWMutex

	// transitionMu 序列化所有状态变更操作（Start/SendEvent/Pause/Resume/Cancel）
	// 确保同一时刻只有一个状态变更在执行，防止并发 SendEvent 导致状态机语义被破坏
	transitionMu sync.Mutex

	// 暂停通道
	pauseCh chan struct{}

	// 恢复通道
	resumeCh chan struct{}

	// 取消函数
	cancelFunc context.CancelFunc
}

// NewProcessInstance 创建流程实例
func NewProcessInstance(def *ProcessDefinition) *ProcessInstance {
	return &ProcessInstance{
		definition:  def,
		id:          idgen.NanoID(),
		status:      StatusPending,
		history:     make([]ExecutionRecord, 0),
		handlers:    make([]EventHandler, 0),
		stepResults: make(map[string]StepResult),
		pauseCh:     make(chan struct{}),
		resumeCh:    make(chan struct{}),
	}
}

// ============== Process 接口实现 ==============

// ID 返回流程实例 ID
func (p *ProcessInstance) ID() string {
	return p.id
}

// Name 返回流程名称
func (p *ProcessInstance) Name() string {
	return p.definition.Name()
}

// Description 返回流程描述
func (p *ProcessInstance) Description() string {
	return p.definition.Description()
}

// CurrentState 返回当前状态
func (p *ProcessInstance) CurrentState() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentState
}

// Status 返回流程执行状态
func (p *ProcessInstance) Status() ProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// GetData 获取流程数据
func (p *ProcessInstance) GetData() *ProcessData {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.data
}

// GetHistory 获取执行历史
func (p *ProcessInstance) GetHistory() []ExecutionRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]ExecutionRecord, len(p.history))
	copy(result, p.history)
	return result
}

// Subscribe 订阅流程事件
func (p *ProcessInstance) Subscribe(handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers = append(p.handlers, handler)
}

// Start 启动流程
func (p *ProcessInstance) Start(ctx context.Context, input ProcessInput) error {
	p.transitionMu.Lock()
	defer p.transitionMu.Unlock()

	p.mu.Lock()

	// 检查状态
	if p.status != StatusPending {
		p.mu.Unlock()
		return ErrProcessAlreadyStarted
	}

	// 初始化流程数据
	p.data = NewProcessData(input.Data)
	p.startTime = time.Now()
	p.status = StatusRunning

	// 确定初始状态
	var initialState *StateImpl
	if input.InitialState != "" {
		state, ok := p.definition.GetState(input.InitialState)
		if !ok {
			// 回滚状态：启动失败不应留下 StatusRunning
			p.status = StatusPending
			p.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrStateNotFound, input.InitialState)
		}
		initialState = state
	} else {
		initialState = p.definition.GetInitialState()
	}

	if initialState == nil {
		// 回滚状态：启动失败不应留下 StatusRunning
		p.status = StatusPending
		p.mu.Unlock()
		return ErrNoInitialState
	}

	p.currentState = initialState
	p.mu.Unlock()

	// 发布流程开始事件（无锁状态下调用，handler 可安全读取流程状态）
	p.publishEvent(ProcessEvent{
		Type:      EventTypeProcessStart,
		ProcessID: p.id,
		StateName: initialState.Name(),
		Timestamp: time.Now(),
	})

	// 进入初始状态
	if err := p.enterState(ctx, initialState); err != nil {
		p.setError(err)
		return err
	}

	// 检查是否有自动转换
	p.processAutoTransitions(ctx)

	return nil
}

// SendEvent 发送事件触发状态转换
func (p *ProcessInstance) SendEvent(ctx context.Context, event Event) error {
	// 序列化所有状态变更，防止并发 SendEvent 导致同一状态发生多次转换
	p.transitionMu.Lock()
	defer p.transitionMu.Unlock()

	// 读取当前状态（只需读锁，transitionMu 已保证无并发写入）
	p.mu.RLock()
	status := p.status
	currentStateName := p.currentState.Name()
	p.mu.RUnlock()

	// 检查状态
	if status == StatusPending {
		return ErrProcessNotStarted
	}
	if status == StatusPaused {
		return ErrProcessPaused
	}
	if status.IsTerminal() {
		return ErrProcessCompleted
	}

	// 查找所有匹配的转换
	transitions := p.definition.GetTransitionsByEvent(currentStateName, event.Name)
	if len(transitions) == 0 {
		return fmt.Errorf("%w: 状态 %s 没有事件 %s 的转换", ErrInvalidTransition, currentStateName, event.Name)
	}

	// 按优先级排序（高优先级优先），与 processAutoTransitions 行为一致
	if len(transitions) > 1 {
		sort.Slice(transitions, func(i, j int) bool {
			return transitions[i].Priority > transitions[j].Priority
		})
	}

	// 遍历所有转换，找到第一个守卫条件通过的
	var matchedTransition *Transition
	for _, t := range transitions {
		if t.Guard == nil {
			// 无守卫条件，直接匹配
			matchedTransition = t
			break
		}
		// 评估守卫条件
		if t.Guard(ctx, p.data) {
			matchedTransition = t
			break
		}
	}

	if matchedTransition == nil {
		return fmt.Errorf("%w: 状态 %s 的事件 %s 没有满足条件的转换", ErrInvalidTransition, currentStateName, event.Name)
	}

	// 执行转换（Guard 已在此处检查，executeTransition 不再重复检查）
	return p.executeTransition(ctx, matchedTransition, event)
}

// Pause 暂停流程
func (p *ProcessInstance) Pause(ctx context.Context) error {
	p.transitionMu.Lock()
	defer p.transitionMu.Unlock()

	p.mu.Lock()
	if p.status != StatusRunning {
		p.mu.Unlock()
		return fmt.Errorf("流程不在运行状态，无法暂停")
	}

	p.status = StatusPaused
	stateName := p.currentState.Name()
	p.mu.Unlock()

	// 先释放 mu 再发布事件，handler 可安全调用 Status()/GetData() 等读方法
	p.publishEvent(ProcessEvent{
		Type:      EventTypeProcessPaused,
		ProcessID: p.id,
		StateName: stateName,
		Timestamp: time.Now(),
	})

	return nil
}

// Resume 恢复流程
func (p *ProcessInstance) Resume(ctx context.Context) error {
	p.transitionMu.Lock()
	defer p.transitionMu.Unlock()

	p.mu.Lock()
	if p.status != StatusPaused {
		p.mu.Unlock()
		return fmt.Errorf("流程不在暂停状态，无法恢复")
	}

	p.status = StatusRunning
	stateName := p.currentState.Name()
	p.mu.Unlock()

	// 先释放 mu 再发布事件
	p.publishEvent(ProcessEvent{
		Type:      EventTypeProcessResumed,
		ProcessID: p.id,
		StateName: stateName,
		Timestamp: time.Now(),
	})

	return nil
}

// Cancel 取消流程
func (p *ProcessInstance) Cancel(ctx context.Context) error {
	p.transitionMu.Lock()
	defer p.transitionMu.Unlock()

	p.mu.Lock()
	if p.status.IsTerminal() {
		p.mu.Unlock()
		return fmt.Errorf("流程已结束，无法取消")
	}

	p.status = StatusCancelled
	p.endTime = time.Now()
	stateName := p.currentState.Name()

	if p.cancelFunc != nil {
		p.cancelFunc()
	}
	p.mu.Unlock()

	// 先释放 mu 再发布事件
	p.publishEvent(ProcessEvent{
		Type:      EventTypeProcessEnd,
		ProcessID: p.id,
		StateName: stateName,
		Timestamp: time.Now(),
		Data:      map[string]any{"reason": "cancelled"},
	})

	return nil
}

// ============== Runnable 接口实现 ==============

// Invoke 同步执行流程
func (p *ProcessInstance) Invoke(ctx context.Context, input ProcessInput, opts ...core.Option) (ProcessOutput, error) {
	// 启动流程
	if err := p.Start(ctx, input); err != nil {
		return ProcessOutput{Error: err}, err
	}

	// 等待流程完成
	return p.waitForCompletion(ctx)
}

// Stream 流式执行（流程不支持真正的流式，返回单个结果）
func (p *ProcessInstance) Stream(ctx context.Context, input ProcessInput, opts ...core.Option) (*core.StreamReader[ProcessOutput], error) {
	output, err := p.Invoke(ctx, input, opts...)

	reader, writer := stream.Pipe[ProcessOutput](1)

	go func() {
		defer writer.Close()
		writer.Send(output)
	}()

	return reader, err
}

// Batch 批量执行
func (p *ProcessInstance) Batch(ctx context.Context, inputs []ProcessInput, opts ...core.Option) ([]ProcessOutput, error) {
	results := make([]ProcessOutput, len(inputs))

	for i, input := range inputs {
		// 每个输入创建新的流程实例
		instance := NewProcessInstance(p.definition)
		output, err := instance.Invoke(ctx, input, opts...)
		if err != nil {
			results[i] = ProcessOutput{Error: err}
		} else {
			results[i] = output
		}
	}

	return results, nil
}

// Collect 流输入转单输出
func (p *ProcessInstance) Collect(ctx context.Context, input *core.StreamReader[ProcessInput], opts ...core.Option) (ProcessOutput, error) {
	// 收集第一个输入
	processInput, err := input.Recv()
	if err != nil {
		return ProcessOutput{Error: err}, err
	}
	return p.Invoke(ctx, processInput, opts...)
}

// Transform 流输入转流输出
func (p *ProcessInstance) Transform(ctx context.Context, input *core.StreamReader[ProcessInput], opts ...core.Option) (*core.StreamReader[ProcessOutput], error) {
	reader, writer := stream.Pipe[ProcessOutput](1)

	go func() {
		defer writer.Close()

		for {
			processInput, err := input.Recv()
			if err != nil {
				break
			}

			// 每个输入创建新的流程实例
			instance := NewProcessInstance(p.definition)
			output, _ := instance.Invoke(ctx, processInput, opts...)
			writer.Send(output)
		}
	}()

	return reader, nil
}

// BatchStream 批量流式执行
func (p *ProcessInstance) BatchStream(ctx context.Context, inputs []ProcessInput, opts ...core.Option) (*core.StreamReader[ProcessOutput], error) {
	reader, writer := stream.Pipe[ProcessOutput](1)

	go func() {
		defer writer.Close()

		for _, input := range inputs {
			instance := NewProcessInstance(p.definition)
			output, _ := instance.Invoke(ctx, input, opts...)
			writer.Send(output)
		}
	}()

	return reader, nil
}

// InputSchema 返回输入 Schema
func (p *ProcessInstance) InputSchema() *core.Schema {
	return nil // 动态 Schema
}

// OutputSchema 返回输出 Schema
func (p *ProcessInstance) OutputSchema() *core.Schema {
	return nil // 动态 Schema
}

// ============== 内部方法 ==============

// enterState 进入状态
// 注意：调用方必须已持有 transitionMu，本方法内不再获取 transitionMu
func (p *ProcessInstance) enterState(ctx context.Context, state *StateImpl) error {
	// 发布进入事件
	p.publishEvent(ProcessEvent{
		Type:      EventTypeStateEnter,
		ProcessID: p.id,
		StateName: state.Name(),
		Timestamp: time.Now(),
	})

	// 记录历史
	p.addRecord(ExecutionRecord{
		ID:        idgen.NanoID(),
		Timestamp: time.Now(),
		Type:      RecordTypeStateEnter,
		ToState:   state.Name(),
		Success:   true,
	})

	// 执行 OnEnter
	start := time.Now()
	if err := state.OnEnter(ctx, p.data); err != nil {
		p.publishEvent(ProcessEvent{
			Type:      EventTypeError,
			ProcessID: p.id,
			StateName: state.Name(),
			Timestamp: time.Now(),
			Error:     err,
		})
		return err
	}

	// 如果有步骤，记录步骤执行
	if state.GetStep() != nil {
		step := state.GetStep()
		p.mu.Lock()
		p.stepResults[step.ID()] = StepResult{
			StepID:   step.ID(),
			StepName: step.Name(),
			Success:  true,
			Duration: time.Since(start),
		}
		p.mu.Unlock()
	}

	// 检查是否为终止状态
	if state.IsFinal() {
		p.complete()
	}

	return nil
}

// exitState 离开状态
// 注意：调用方必须已持有 transitionMu，本方法内不再获取 transitionMu
func (p *ProcessInstance) exitState(ctx context.Context, state *StateImpl) error {
	// 发布离开事件
	p.publishEvent(ProcessEvent{
		Type:      EventTypeStateExit,
		ProcessID: p.id,
		StateName: state.Name(),
		Timestamp: time.Now(),
	})

	// 记录历史
	p.addRecord(ExecutionRecord{
		ID:        idgen.NanoID(),
		Timestamp: time.Now(),
		Type:      RecordTypeStateExit,
		FromState: state.Name(),
		Success:   true,
	})

	// 执行 OnExit
	if err := state.OnExit(ctx, p.data); err != nil {
		return err
	}

	return nil
}

// executeTransition 执行状态转换
// 注意：调用方（SendEvent/processAutoTransitions）已保证 Guard 通过，本方法不再重复检查
// 注意：调用方必须已持有 transitionMu，本方法内不再获取 transitionMu
func (p *ProcessInstance) executeTransition(ctx context.Context, t *Transition, event Event) error {
	p.mu.RLock()
	currentState := p.currentState
	p.mu.RUnlock()

	// 离开当前状态
	if err := p.exitState(ctx, currentState); err != nil {
		return err
	}

	// 执行转换动作
	if err := t.Execute(ctx, p.data); err != nil {
		return err
	}

	// 获取目标状态
	targetState, ok := p.definition.GetState(t.To)
	if !ok {
		return fmt.Errorf("%w: %s", ErrStateNotFound, t.To)
	}

	// 发布转换事件
	p.publishEvent(ProcessEvent{
		Type:      EventTypeTransition,
		ProcessID: p.id,
		StateName: targetState.Name(),
		EventName: event.Name,
		Timestamp: time.Now(),
		Data:      event.Data,
	})

	// 记录历史
	p.addRecord(ExecutionRecord{
		ID:        idgen.NanoID(),
		Timestamp: time.Now(),
		Type:      RecordTypeTransition,
		FromState: currentState.Name(),
		ToState:   targetState.Name(),
		Event:     event.Name,
		Success:   true,
	})

	// 更新当前状态
	p.mu.Lock()
	p.currentState = targetState
	p.mu.Unlock()

	// 进入新状态
	if err := p.enterState(ctx, targetState); err != nil {
		return err
	}

	// 检查是否有自动转换
	p.processAutoTransitions(ctx)

	return nil
}

// processAutoTransitions 处理自动转换
// 注意：调用方必须已持有 transitionMu，本方法内不再获取 transitionMu
func (p *ProcessInstance) processAutoTransitions(ctx context.Context) {
	p.mu.RLock()
	if p.status.IsTerminal() {
		p.mu.RUnlock()
		return
	}
	currentStateName := p.currentState.Name()
	p.mu.RUnlock()

	// 获取当前状态的所有转换
	transitions := p.definition.GetTransitions(currentStateName)

	// 过滤出自动转换
	var autoTransitions []*Transition
	for _, t := range transitions {
		if t.Event == "_auto_" {
			autoTransitions = append(autoTransitions, t)
		}
	}

	if len(autoTransitions) == 0 {
		return
	}

	// 按优先级排序
	sort.Slice(autoTransitions, func(i, j int) bool {
		return autoTransitions[i].Priority > autoTransitions[j].Priority
	})

	// 尝试执行第一个满足条件的自动转换
	for _, t := range autoTransitions {
		if t.CanTransit(ctx, p.data) {
			if err := p.executeTransition(ctx, t, NewEvent("_auto_")); err == nil {
				return
			}
		}
	}
}

// complete 完成流程
func (p *ProcessInstance) complete() {
	p.mu.Lock()
	p.status = StatusCompleted
	p.endTime = time.Now()
	stateName := p.currentState.Name()
	p.mu.Unlock()

	// 先释放 mu 再发布事件，防止 handler 获取读锁时死锁
	p.publishEvent(ProcessEvent{
		Type:      EventTypeProcessEnd,
		ProcessID: p.id,
		StateName: stateName,
		Timestamp: time.Now(),
		Data:      map[string]any{"status": "completed"},
	})
}

// setError 设置错误
func (p *ProcessInstance) setError(err error) {
	p.mu.Lock()
	p.lastError = err
	p.status = StatusFailed
	p.endTime = time.Now()
	stateName := p.currentState.Name()
	p.mu.Unlock()

	// 先释放 mu 再发布事件，防止 handler 获取读锁时死锁
	p.publishEvent(ProcessEvent{
		Type:      EventTypeError,
		ProcessID: p.id,
		StateName: stateName,
		Timestamp: time.Now(),
		Error:     err,
	})
}

// waitForCompletion 等待流程完成
func (p *ProcessInstance) waitForCompletion(ctx context.Context) (ProcessOutput, error) {
	// 简单实现：流程是同步执行的，直接返回结果
	p.mu.RLock()
	defer p.mu.RUnlock()

	output := ProcessOutput{
		Data:          p.data.Variables,
		FinalState:    p.currentState.Name(),
		ExecutionTime: p.endTime.Sub(p.startTime),
		StepResults:   p.stepResults,
		History:       p.history,
		Error:         p.lastError,
		Metadata:      make(map[string]any),
	}

	return output, p.lastError
}

// addRecord 添加执行记录
func (p *ProcessInstance) addRecord(record ExecutionRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.history = append(p.history, record)

	// 限制历史记录大小（默认最多保留 1000 条，防止无限增长）
	maxHistory := p.definition.options.MaxHistorySize
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	if len(p.history) > maxHistory {
		p.history = p.history[len(p.history)-maxHistory:]
	}
}

// publishEvent 发布事件
// 重要：调用此方法时不能持有 p.mu，否则 handler 调用 Status()/GetData() 等方法时会死锁
func (p *ProcessInstance) publishEvent(event ProcessEvent) {
	p.mu.RLock()
	handlers := make([]EventHandler, len(p.handlers))
	copy(handlers, p.handlers)
	p.mu.RUnlock()

	for _, handler := range handlers {
		// 捕获 handler panic，防止单个 handler 异常中断整个流程执行
		func() {
			defer func() {
				if r := recover(); r != nil {
					// 事件处理器 panic 不应中断流程执行
					// TODO: 集成 observe 包后添加日志记录
				}
			}()
			handler(event)
		}()
	}
}

// ============== 便捷函数 ==============

// Run 运行流程（便捷方法）
func Run(ctx context.Context, process Process, data map[string]any) (ProcessOutput, error) {
	input := ProcessInput{
		Data: data,
	}
	return process.Invoke(ctx, input)
}

// RunWithEvents 运行流程并处理事件
func RunWithEvents(ctx context.Context, process Process, data map[string]any, handler EventHandler) (ProcessOutput, error) {
	process.Subscribe(handler)
	return Run(ctx, process, data)
}
