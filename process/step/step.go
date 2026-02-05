// Package step 提供流程步骤实现
//
// 本包提供多种步骤类型：
// - ActionStep: 执行确定性动作
// - AgentStep: 将 Agent 作为步骤执行
// - ConditionStep: 条件分支步骤
// - ParallelStep: 并行执行多个步骤
package step

import (
	"context"
	"time"

	"github.com/everyday-items/hexagon/process"
	"github.com/everyday-items/toolkit/util/idgen"
)

// BaseStep 基础步骤实现
type BaseStep struct {
	id          string
	name        string
	description string
	metadata    map[string]any
}

// ID 返回步骤 ID
func (s *BaseStep) ID() string {
	return s.id
}

// Name 返回步骤名称
func (s *BaseStep) Name() string {
	return s.name
}

// Description 返回步骤描述
func (s *BaseStep) Description() string {
	return s.description
}

// Validate 验证步骤配置
func (s *BaseStep) Validate() error {
	return nil
}

// ============== ActionStep ==============

// ActionStep 动作步骤
// 执行确定性的动作函数
type ActionStep struct {
	BaseStep
	action  func(ctx context.Context, data *process.ProcessData) (any, error)
	timeout time.Duration
}

// ActionStepOption ActionStep 配置选项
type ActionStepOption func(*ActionStep)

// WithActionTimeout 设置超时
func WithActionTimeout(timeout time.Duration) ActionStepOption {
	return func(s *ActionStep) {
		s.timeout = timeout
	}
}

// WithActionDescription 设置描述
func WithActionDescription(desc string) ActionStepOption {
	return func(s *ActionStep) {
		s.description = desc
	}
}

// NewActionStep 创建动作步骤
func NewActionStep(name string, action func(ctx context.Context, data *process.ProcessData) (any, error), opts ...ActionStepOption) *ActionStep {
	s := &ActionStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		action: action,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *ActionStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	// 如果设置了超时，创建超时上下文
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// 执行动作
	output, err := s.action(ctx, data)
	duration := time.Since(start)

	result := &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  err == nil,
		Output:   output,
		Error:    err,
		Duration: duration,
	}

	return result, err
}

// ============== FuncStep ==============

// FuncStep 函数步骤（简化版 ActionStep）
type FuncStep[I, O any] struct {
	BaseStep
	fn      func(ctx context.Context, input I) (O, error)
	inputFn func(data *process.ProcessData) I
}

// NewFuncStep 创建函数步骤
func NewFuncStep[I, O any](
	name string,
	fn func(ctx context.Context, input I) (O, error),
	inputFn func(data *process.ProcessData) I,
) *FuncStep[I, O] {
	return &FuncStep[I, O]{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		fn:      fn,
		inputFn: inputFn,
	}
}

// Execute 执行步骤
func (s *FuncStep[I, O]) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	// 获取输入
	input := s.inputFn(data)

	// 执行函数
	output, err := s.fn(ctx, input)
	duration := time.Since(start)

	result := &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  err == nil,
		Output:   output,
		Error:    err,
		Duration: duration,
	}

	return result, err
}

// ============== ConditionStep ==============

// ConditionStep 条件步骤
// 根据条件选择执行不同的步骤
type ConditionStep struct {
	BaseStep
	condition func(ctx context.Context, data *process.ProcessData) bool
	trueStep  process.Step
	falseStep process.Step
}

// NewConditionStep 创建条件步骤
func NewConditionStep(
	name string,
	condition func(ctx context.Context, data *process.ProcessData) bool,
	trueStep, falseStep process.Step,
) *ConditionStep {
	return &ConditionStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		condition: condition,
		trueStep:  trueStep,
		falseStep: falseStep,
	}
}

// Execute 执行步骤
func (s *ConditionStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	// 评估条件
	result := s.condition(ctx, data)

	// 选择执行的步骤
	var step process.Step
	if result {
		step = s.trueStep
	} else {
		step = s.falseStep
	}

	// 如果选中的步骤为空，直接返回
	if step == nil {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  true,
			Output:   map[string]any{"condition": result, "executed": false},
			Duration: time.Since(start),
		}, nil
	}

	// 执行选中的步骤
	stepResult, err := step.Execute(ctx, data)

	// 包装结果
	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  stepResult.Success,
		Output: map[string]any{
			"condition":    result,
			"executed":     true,
			"step_id":      stepResult.StepID,
			"step_output":  stepResult.Output,
		},
		Error:    err,
		Duration: time.Since(start),
	}, err
}

// ============== ParallelStep ==============

// ParallelStep 并行步骤
// 并行执行多个步骤
type ParallelStep struct {
	BaseStep
	steps         []process.Step
	failFast      bool // 是否在第一个失败时停止
	maxConcurrent int  // 最大并发数，0 表示不限制
}

// ParallelStepOption ParallelStep 配置选项
type ParallelStepOption func(*ParallelStep)

// WithFailFast 设置失败时立即停止
func WithFailFast() ParallelStepOption {
	return func(s *ParallelStep) {
		s.failFast = true
	}
}

// WithMaxConcurrent 设置最大并发数
func WithMaxConcurrent(max int) ParallelStepOption {
	return func(s *ParallelStep) {
		s.maxConcurrent = max
	}
}

// NewParallelStep 创建并行步骤
func NewParallelStep(name string, steps []process.Step, opts ...ParallelStepOption) *ParallelStep {
	s := &ParallelStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		steps: steps,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *ParallelStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	if len(s.steps) == 0 {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  true,
			Output:   []any{},
			Duration: time.Since(start),
		}, nil
	}

	// 创建结果通道
	type stepResult struct {
		index  int
		result *process.StepResult
		err    error
	}
	resultCh := make(chan stepResult, len(s.steps))

	// 创建取消上下文
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 并发执行步骤
	// 如果设置了最大并发数，使用信号量控制
	var semaphore chan struct{}
	if s.maxConcurrent > 0 {
		semaphore = make(chan struct{}, s.maxConcurrent)
	}

	for i, step := range s.steps {
		if semaphore != nil {
			semaphore <- struct{}{}
		}

		go func(idx int, st process.Step) {
			if semaphore != nil {
				defer func() { <-semaphore }()
			}

			// 克隆数据以避免并发修改
			clonedData := data.Clone()
			result, err := st.Execute(ctx, clonedData)

			resultCh <- stepResult{
				index:  idx,
				result: result,
				err:    err,
			}
		}(i, step)
	}

	// 收集结果
	results := make([]*process.StepResult, len(s.steps))
	var firstErr error

	for i := 0; i < len(s.steps); i++ {
		select {
		case <-ctx.Done():
			return &process.StepResult{
				StepID:   s.id,
				StepName: s.name,
				Success:  false,
				Error:    ctx.Err(),
				Duration: time.Since(start),
			}, ctx.Err()

		case r := <-resultCh:
			results[r.index] = r.result
			if r.err != nil && firstErr == nil {
				firstErr = r.err
				if s.failFast {
					cancel()
				}
			}
		}
	}

	// 构建输出
	outputs := make([]any, len(results))
	allSuccess := true
	for i, r := range results {
		if r != nil {
			outputs[i] = r.Output
			if !r.Success {
				allSuccess = false
			}
		}
	}

	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  allSuccess,
		Output:   outputs,
		Error:    firstErr,
		Duration: time.Since(start),
	}, firstErr
}

// ============== SequenceStep ==============

// SequenceStep 顺序步骤
// 按顺序执行多个步骤
type SequenceStep struct {
	BaseStep
	steps        []process.Step
	stopOnError  bool // 是否在错误时停止
}

// SequenceStepOption SequenceStep 配置选项
type SequenceStepOption func(*SequenceStep)

// WithStopOnError 设置错误时停止
func WithStopOnError() SequenceStepOption {
	return func(s *SequenceStep) {
		s.stopOnError = true
	}
}

// NewSequenceStep 创建顺序步骤
func NewSequenceStep(name string, steps []process.Step, opts ...SequenceStepOption) *SequenceStep {
	s := &SequenceStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		steps:       steps,
		stopOnError: true, // 默认错误时停止
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *SequenceStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	outputs := make([]any, 0, len(s.steps))
	var lastErr error
	allSuccess := true

	for _, step := range s.steps {
		select {
		case <-ctx.Done():
			return &process.StepResult{
				StepID:   s.id,
				StepName: s.name,
				Success:  false,
				Output:   outputs,
				Error:    ctx.Err(),
				Duration: time.Since(start),
			}, ctx.Err()

		default:
			result, err := step.Execute(ctx, data)
			if result != nil {
				outputs = append(outputs, result.Output)
				// 将步骤输出保存到数据中
				data.SetStepOutput(step.ID(), result.Output)
			}

			if err != nil {
				lastErr = err
				allSuccess = false
				if s.stopOnError {
					return &process.StepResult{
						StepID:   s.id,
						StepName: s.name,
						Success:  false,
						Output:   outputs,
						Error:    err,
						Duration: time.Since(start),
					}, err
				}
			}
		}
	}

	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  allSuccess,
		Output:   outputs,
		Error:    lastErr,
		Duration: time.Since(start),
	}, lastErr
}

// ============== RetryStep ==============

// RetryStep 重试步骤
// 包装其他步骤，失败时自动重试
type RetryStep struct {
	BaseStep
	step          process.Step
	maxRetries    int
	retryDelay    time.Duration
	backoff       float64
	shouldRetry   func(err error) bool
}

// RetryStepOption RetryStep 配置选项
type RetryStepOption func(*RetryStep)

// WithRetryDelay 设置重试延迟
func WithRetryDelay(delay time.Duration) RetryStepOption {
	return func(s *RetryStep) {
		s.retryDelay = delay
	}
}

// WithBackoff 设置退避倍数
func WithBackoff(multiplier float64) RetryStepOption {
	return func(s *RetryStep) {
		s.backoff = multiplier
	}
}

// WithShouldRetry 设置重试条件
func WithShouldRetry(fn func(err error) bool) RetryStepOption {
	return func(s *RetryStep) {
		s.shouldRetry = fn
	}
}

// NewRetryStep 创建重试步骤
func NewRetryStep(name string, step process.Step, maxRetries int, opts ...RetryStepOption) *RetryStep {
	s := &RetryStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		step:       step,
		maxRetries: maxRetries,
		retryDelay: time.Second,
		backoff:    1.0,
		shouldRetry: func(err error) bool {
			return err != nil
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *RetryStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()
	delay := s.retryDelay

	var lastResult *process.StepResult
	var lastErr error

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		// 检查上下文
		select {
		case <-ctx.Done():
			return &process.StepResult{
				StepID:   s.id,
				StepName: s.name,
				Success:  false,
				Error:    ctx.Err(),
				Duration: time.Since(start),
				Metadata: map[string]any{"attempts": attempt},
			}, ctx.Err()
		default:
		}

		// 执行步骤
		result, err := s.step.Execute(ctx, data)
		lastResult = result
		lastErr = err

		// 成功则返回
		if err == nil {
			result.Metadata = map[string]any{"attempts": attempt + 1}
			return result, nil
		}

		// 检查是否应该重试
		if attempt < s.maxRetries && s.shouldRetry(err) {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * s.backoff)
		}
	}

	// 所有重试都失败
	if lastResult != nil {
		lastResult.Metadata = map[string]any{"attempts": s.maxRetries + 1}
	}
	return lastResult, lastErr
}

// ============== TimeoutStep ==============

// TimeoutStep 超时步骤
// 为步骤添加超时控制
type TimeoutStep struct {
	BaseStep
	step    process.Step
	timeout time.Duration
}

// NewTimeoutStep 创建超时步骤
func NewTimeoutStep(name string, step process.Step, timeout time.Duration) *TimeoutStep {
	return &TimeoutStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		step:    step,
		timeout: timeout,
	}
}

// Execute 执行步骤
func (s *TimeoutStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.step.Execute(ctx, data)
	result.Duration = time.Since(start)

	return result, err
}
