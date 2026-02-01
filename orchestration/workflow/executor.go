package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Executor 工作流执行器
type Executor struct {
	// 执行中的工作流
	executions sync.Map // executionID -> *executionState

	// 持久化存储
	store WorkflowStore

	// 事件处理器
	eventHandlers []WorkflowEventHandler

	// 钩子
	hooks *WorkflowHooks

	// 配置
	config ExecutorConfig

	mu sync.RWMutex
}

// ExecutorConfig 执行器配置
type ExecutorConfig struct {
	// DefaultTimeout 默认超时时间
	DefaultTimeout time.Duration

	// MaxConcurrentExecutions 最大并发执行数
	MaxConcurrentExecutions int

	// EnablePersistence 启用持久化
	EnablePersistence bool
}

// DefaultExecutorConfig 返回默认配置
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		DefaultTimeout:          time.Hour,
		MaxConcurrentExecutions: 100,
		EnablePersistence:       false,
	}
}

// executionState 执行状态
type executionState struct {
	execution *WorkflowExecution
	workflow  *Workflow
	cancel    context.CancelFunc
	pauseCh   chan struct{}
	resumeCh  chan struct{}
	doneCh    chan struct{}
	mu        sync.Mutex
}

// ExecutorOption 执行器选项
type ExecutorOption func(*Executor)

// WithStore 设置存储
func WithStore(store WorkflowStore) ExecutorOption {
	return func(e *Executor) {
		e.store = store
		e.config.EnablePersistence = true
	}
}

// WithHooks 设置钩子
func WithHooks(hooks *WorkflowHooks) ExecutorOption {
	return func(e *Executor) {
		e.hooks = hooks
	}
}

// WithExecutorConfig 设置配置
func WithExecutorConfig(config ExecutorConfig) ExecutorOption {
	return func(e *Executor) {
		e.config = config
	}
}

// NewExecutor 创建执行器
func NewExecutor(opts ...ExecutorOption) *Executor {
	e := &Executor{
		config:        DefaultExecutorConfig(),
		eventHandlers: make([]WorkflowEventHandler, 0),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// OnEvent 注册事件处理器
func (e *Executor) OnEvent(handler WorkflowEventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventHandlers = append(e.eventHandlers, handler)
}

// emitEvent 发送事件
func (e *Executor) emitEvent(event *WorkflowEvent) {
	e.mu.RLock()
	handlers := make([]WorkflowEventHandler, len(e.eventHandlers))
	copy(handlers, e.eventHandlers)
	e.mu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}

// Run 同步运行工作流
func (e *Executor) Run(ctx context.Context, wf *Workflow, input WorkflowInput) (*WorkflowOutput, error) {
	executionID, err := e.RunAsync(ctx, wf, input)
	if err != nil {
		return nil, err
	}

	timeout := wf.Timeout
	if timeout == 0 {
		timeout = e.config.DefaultTimeout
	}

	execution, err := e.WaitForCompletion(ctx, executionID, timeout)
	if err != nil {
		return nil, err
	}

	if execution.Status == StatusFailed {
		return nil, fmt.Errorf("workflow failed: %s", execution.Error)
	}

	if execution.Status == StatusCancelled {
		return nil, fmt.Errorf("workflow cancelled")
	}

	var output WorkflowOutput
	if execution.Output != nil {
		// 尝试直接反序列化为 WorkflowOutput
		if err := json.Unmarshal(execution.Output, &output); err != nil {
			// 如果失败，尝试反序列化为 Data 字段
			var data any
			if err := json.Unmarshal(execution.Output, &data); err != nil {
				// 如果仍然失败，使用原始字节
				output.Data = execution.Output
			} else {
				output.Data = data
			}
		}
	}
	output.Variables = execution.Context.Variables
	output.StepOutputs = make(map[string]any)
	for stepID, result := range execution.StepResults {
		output.StepOutputs[stepID] = result.Output
	}

	return &output, nil
}

// RunAsync 异步运行工作流
func (e *Executor) RunAsync(ctx context.Context, wf *Workflow, input WorkflowInput) (string, error) {
	// 创建执行实例
	execution := NewExecution(wf)
	execution.StartedAt = time.Now()
	execution.Status = StatusRunning

	// 序列化输入
	if input.Data != nil {
		inputData, _ := json.Marshal(input.Data)
		execution.Input = inputData
	}

	// 初始化上下文
	if input.Variables != nil {
		execution.Context.Variables = input.Variables
	}
	if input.Metadata != nil {
		execution.Context.Metadata = input.Metadata
	}

	// 创建执行状态
	execCtx, cancel := context.WithCancel(ctx)
	state := &executionState{
		execution: execution,
		workflow:  wf,
		cancel:    cancel,
		pauseCh:   make(chan struct{}),
		resumeCh:  make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	e.executions.Store(execution.ID, state)

	// 持久化
	if e.config.EnablePersistence && e.store != nil {
		if err := e.store.SaveExecution(ctx, execution); err != nil {
			return "", fmt.Errorf("save execution: %w", err)
		}
	}

	// 触发开始钩子
	if e.hooks != nil && e.hooks.OnStart != nil {
		if err := e.hooks.OnStart(ctx, wf, input); err != nil {
			return "", fmt.Errorf("start hook failed: %w", err)
		}
	}

	// 发送开始事件
	e.emitEvent(&WorkflowEvent{
		Type:        EventWorkflowStarted,
		ExecutionID: execution.ID,
		Status:      StatusRunning,
		Timestamp:   time.Now(),
	})

	// 异步执行
	go e.executeWorkflow(execCtx, state, input)

	return execution.ID, nil
}

// executeWorkflow 执行工作流
func (e *Executor) executeWorkflow(ctx context.Context, state *executionState, input WorkflowInput) {
	defer close(state.doneCh)

	wf := state.workflow
	execution := state.execution

	// 准备步骤输入
	stepInput := StepInput{
		Data:            input.Data,
		Variables:       execution.Context.Variables,
		PreviousOutputs: make(map[string]any),
		Metadata:        execution.Context.Metadata,
	}

	// 顺序执行步骤
	for _, step := range wf.Steps {
		select {
		case <-ctx.Done():
			e.setExecutionStatus(state, StatusCancelled, ctx.Err().Error())
			return
		case <-state.pauseCh:
			e.setExecutionStatus(state, StatusPaused, "")
			// 等待恢复
			select {
			case <-ctx.Done():
				e.setExecutionStatus(state, StatusCancelled, ctx.Err().Error())
				return
			case <-state.resumeCh:
				e.setExecutionStatus(state, StatusRunning, "")
			}
		default:
		}

		// 检查依赖
		if baseStep, ok := step.(*BaseStep); ok {
			for _, dep := range baseStep.Dependencies() {
				if _, completed := execution.StepResults[dep]; !completed {
					e.setExecutionStatus(state, StatusFailed, fmt.Sprintf("dependency %s not completed", dep))
					return
				}
			}
		}

		// 执行步骤
		execution.Context.CurrentStepID = step.ID()

		// 触发步骤开始钩子
		if e.hooks != nil && e.hooks.OnStepStart != nil {
			e.hooks.OnStepStart(ctx, step, stepInput.Data)
		}

		e.emitEvent(&WorkflowEvent{
			Type:        EventStepStarted,
			ExecutionID: execution.ID,
			StepID:      step.ID(),
			Status:      StatusRunning,
			Timestamp:   time.Now(),
		})

		stepResult := &StepResult{
			StepID:    step.ID(),
			Status:    StatusRunning,
			StartedAt: time.Now(),
		}
		execution.StepResults[step.ID()] = stepResult

		output, err := step.Execute(ctx, stepInput)

		completedAt := time.Now()
		stepResult.CompletedAt = &completedAt
		stepResult.Duration = completedAt.Sub(stepResult.StartedAt)

		if err != nil {
			stepResult.Status = StatusFailed
			stepResult.Error = err.Error()

			// 触发步骤错误钩子
			if e.hooks != nil && e.hooks.OnStepError != nil {
				e.hooks.OnStepError(ctx, step, err)
			}

			e.emitEvent(&WorkflowEvent{
				Type:        EventStepFailed,
				ExecutionID: execution.ID,
				StepID:      step.ID(),
				Status:      StatusFailed,
				Error:       err.Error(),
				Timestamp:   time.Now(),
			})

			e.setExecutionStatus(state, StatusFailed, fmt.Sprintf("step %s failed: %s", step.ID(), err.Error()))
			return
		}

		stepResult.Status = StatusCompleted
		if output != nil {
			stepResult.Output = output.Data

			// 更新输入
			stepInput.Data = output.Data
			stepInput.PreviousOutputs[step.ID()] = output.Data

			// 合并变量
			for k, v := range output.Variables {
				stepInput.Variables[k] = v
				execution.Context.Variables[k] = v
			}
		}

		execution.Context.CompletedSteps = append(execution.Context.CompletedSteps, step.ID())

		// 触发步骤完成钩子
		if e.hooks != nil && e.hooks.OnStepComplete != nil {
			e.hooks.OnStepComplete(ctx, step, output)
		}

		e.emitEvent(&WorkflowEvent{
			Type:        EventStepCompleted,
			ExecutionID: execution.ID,
			StepID:      step.ID(),
			Status:      StatusCompleted,
			Data:        output,
			Timestamp:   time.Now(),
		})

		// 持久化
		if e.config.EnablePersistence && e.store != nil {
			e.store.SaveExecution(ctx, execution)
		}
	}

	// 工作流完成
	outputData, _ := json.Marshal(stepInput.Data)
	execution.Output = outputData
	e.setExecutionStatus(state, StatusCompleted, "")

	// 触发完成钩子
	if e.hooks != nil && e.hooks.OnComplete != nil {
		e.hooks.OnComplete(ctx, wf, &WorkflowOutput{
			Data:      stepInput.Data,
			Variables: stepInput.Variables,
		})
	}
}

// setExecutionStatus 设置执行状态
func (e *Executor) setExecutionStatus(state *executionState, status WorkflowStatus, errMsg string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	execution := state.execution
	execution.Status = status
	if errMsg != "" {
		execution.Error = errMsg
	}

	now := time.Now()
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		execution.CompletedAt = &now
		execution.Duration = now.Sub(execution.StartedAt)
	case StatusPaused:
		execution.PausedAt = &now
	}

	// 持久化
	if e.config.EnablePersistence && e.store != nil {
		e.store.SaveExecution(context.Background(), execution)
	}

	// 发送事件
	var eventType WorkflowEventType
	switch status {
	case StatusCompleted:
		eventType = EventWorkflowCompleted
	case StatusFailed:
		eventType = EventWorkflowFailed
	case StatusPaused:
		eventType = EventWorkflowPaused
	case StatusCancelled:
		eventType = EventWorkflowCancelled
	case StatusRunning:
		eventType = EventWorkflowResumed
	}

	if eventType != "" {
		e.emitEvent(&WorkflowEvent{
			Type:        eventType,
			ExecutionID: execution.ID,
			Status:      status,
			Error:       errMsg,
			Timestamp:   now,
		})
	}
}

// Pause 暂停执行
func (e *Executor) Pause(ctx context.Context, executionID string) error {
	stateVal, ok := e.executions.Load(executionID)
	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	state := stateVal.(*executionState)
	if state.execution.Status != StatusRunning {
		return fmt.Errorf("execution is not running")
	}

	select {
	case state.pauseCh <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("already pausing")
	}
}

// Resume 恢复执行
func (e *Executor) Resume(ctx context.Context, executionID string) error {
	stateVal, ok := e.executions.Load(executionID)
	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	state := stateVal.(*executionState)
	if state.execution.Status != StatusPaused {
		return fmt.Errorf("execution is not paused")
	}

	select {
	case state.resumeCh <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("already resuming")
	}
}

// Cancel 取消执行
func (e *Executor) Cancel(ctx context.Context, executionID string) error {
	stateVal, ok := e.executions.Load(executionID)
	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	state := stateVal.(*executionState)
	state.cancel()
	return nil
}

// GetExecution 获取执行实例
func (e *Executor) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	// 先从内存查找
	if stateVal, ok := e.executions.Load(executionID); ok {
		state := stateVal.(*executionState)
		return state.execution, nil
	}

	// 从存储查找
	if e.config.EnablePersistence && e.store != nil {
		return e.store.GetExecution(ctx, executionID)
	}

	return nil, fmt.Errorf("execution %s not found", executionID)
}

// WaitForCompletion 等待执行完成
func (e *Executor) WaitForCompletion(ctx context.Context, executionID string, timeout time.Duration) (*WorkflowExecution, error) {
	stateVal, ok := e.executions.Load(executionID)
	if !ok {
		return nil, fmt.Errorf("execution %s not found", executionID)
	}

	state := stateVal.(*executionState)

	if timeout > 0 {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		select {
		case <-state.doneCh:
			return state.execution, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	<-state.doneCh
	return state.execution, nil
}

// ListExecutions 列出执行实例
func (e *Executor) ListExecutions(ctx context.Context, workflowID string, status WorkflowStatus, limit int) ([]*WorkflowExecution, error) {
	if e.config.EnablePersistence && e.store != nil {
		return e.store.ListExecutions(ctx, workflowID, status, limit)
	}

	// 从内存获取
	var executions []*WorkflowExecution
	e.executions.Range(func(_, value any) bool {
		state := value.(*executionState)
		if workflowID != "" && state.workflow.ID != workflowID {
			return true
		}
		if status != "" && state.execution.Status != status {
			return true
		}
		executions = append(executions, state.execution)
		if limit > 0 && len(executions) >= limit {
			return false
		}
		return true
	})

	return executions, nil
}

// CleanupCompleted 清理已完成的执行
func (e *Executor) CleanupCompleted(olderThan time.Duration) int {
	cutoff := time.Now().Add(-olderThan)
	cleaned := 0

	e.executions.Range(func(key, value any) bool {
		state := value.(*executionState)
		execution := state.execution

		if execution.Status == StatusCompleted || execution.Status == StatusFailed || execution.Status == StatusCancelled {
			if execution.CompletedAt != nil && execution.CompletedAt.Before(cutoff) {
				e.executions.Delete(key)
				cleaned++
			}
		}
		return true
	})

	return cleaned
}

// 确保实现了接口
var _ WorkflowRunner = (*Executor)(nil)
