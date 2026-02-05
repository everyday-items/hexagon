// Package process 提供确定性业务流程框架
//
// 本文件实现流程构建器，提供流畅的 API 来定义流程。
package process

import (
	"context"
	"fmt"
)

// ProcessBuilder 流程构建器
// 提供流畅的 API 来构建流程定义
type ProcessBuilder struct {
	// 流程名称
	name string

	// 流程描述
	description string

	// 状态定义
	states map[string]*StateImpl

	// 状态顺序（用于保持定义顺序）
	stateOrder []string

	// 状态转换
	transitions []*Transition

	// 状态进入步骤
	stateSteps map[string]Step

	// 验证错误
	errors []error

	// 元数据
	metadata map[string]any

	// 配置选项
	options *ProcessOptions
}

// ProcessOptions 流程配置选项
type ProcessOptions struct {
	// MaxExecutionTime 最大执行时间（秒）
	MaxExecutionTime int

	// MaxSteps 最大步骤数
	MaxSteps int

	// EnableCheckpoint 是否启用检查点
	EnableCheckpoint bool

	// CheckpointInterval 检查点间隔（状态转换次数）
	CheckpointInterval int

	// EnableHistory 是否记录历史
	EnableHistory bool

	// MaxHistorySize 最大历史记录数
	MaxHistorySize int

	// OnError 错误处理策略
	OnError ErrorStrategy

	// RetryPolicy 重试策略
	RetryPolicy *RetryPolicy
}

// ErrorStrategy 错误处理策略
type ErrorStrategy string

const (
	// ErrorStrategyFail 失败时停止流程
	ErrorStrategyFail ErrorStrategy = "fail"

	// ErrorStrategyIgnore 忽略错误继续执行
	ErrorStrategyIgnore ErrorStrategy = "ignore"

	// ErrorStrategyRetry 重试
	ErrorStrategyRetry ErrorStrategy = "retry"

	// ErrorStrategyFallback 降级到指定状态
	ErrorStrategyFallback ErrorStrategy = "fallback"
)

// RetryPolicy 重试策略
type RetryPolicy struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryDelay 重试延迟（毫秒）
	RetryDelay int

	// BackoffMultiplier 退避倍数
	BackoffMultiplier float64
}

// DefaultProcessOptions 默认流程选项
func DefaultProcessOptions() *ProcessOptions {
	return &ProcessOptions{
		MaxExecutionTime:   3600, // 1 小时
		MaxSteps:           1000,
		EnableCheckpoint:   false,
		CheckpointInterval: 10,
		EnableHistory:      true,
		MaxHistorySize:     100,
		OnError:            ErrorStrategyFail,
	}
}

// NewProcess 创建流程构建器
func NewProcess(name string) *ProcessBuilder {
	return &ProcessBuilder{
		name:        name,
		states:      make(map[string]*StateImpl),
		stateOrder:  make([]string, 0),
		transitions: make([]*Transition, 0),
		stateSteps:  make(map[string]Step),
		errors:      make([]error, 0),
		metadata:    make(map[string]any),
		options:     DefaultProcessOptions(),
	}
}

// WithDescription 设置流程描述
func (b *ProcessBuilder) WithDescription(desc string) *ProcessBuilder {
	b.description = desc
	return b
}

// WithMetadata 设置元数据
func (b *ProcessBuilder) WithMetadata(key string, value any) *ProcessBuilder {
	b.metadata[key] = value
	return b
}

// WithOptions 设置流程选项
func (b *ProcessBuilder) WithOptions(opts *ProcessOptions) *ProcessBuilder {
	b.options = opts
	return b
}

// WithMaxExecutionTime 设置最大执行时间
func (b *ProcessBuilder) WithMaxExecutionTime(seconds int) *ProcessBuilder {
	b.options.MaxExecutionTime = seconds
	return b
}

// WithCheckpoint 启用检查点
func (b *ProcessBuilder) WithCheckpoint(interval int) *ProcessBuilder {
	b.options.EnableCheckpoint = true
	b.options.CheckpointInterval = interval
	return b
}

// WithErrorStrategy 设置错误处理策略
func (b *ProcessBuilder) WithErrorStrategy(strategy ErrorStrategy) *ProcessBuilder {
	b.options.OnError = strategy
	return b
}

// WithRetryPolicy 设置重试策略
func (b *ProcessBuilder) WithRetryPolicy(maxRetries, retryDelay int, backoff float64) *ProcessBuilder {
	b.options.RetryPolicy = &RetryPolicy{
		MaxRetries:        maxRetries,
		RetryDelay:        retryDelay,
		BackoffMultiplier: backoff,
	}
	b.options.OnError = ErrorStrategyRetry
	return b
}

// ============== 状态定义 ==============

// StateOption 状态配置选项
type StateOption func(*StateImpl)

// AsInitial 标记为初始状态
func AsInitial() StateOption {
	return func(s *StateImpl) {
		s.initial = true
	}
}

// AsFinal 标记为终止状态
func AsFinal() StateOption {
	return func(s *StateImpl) {
		s.final = true
	}
}

// WithStateMetadata 设置状态元数据
func WithStateMetadata(key string, value any) StateOption {
	return func(s *StateImpl) {
		if s.metadata == nil {
			s.metadata = make(map[string]any)
		}
		s.metadata[key] = value
	}
}

// WithOnEnter 设置进入回调
func WithOnEnter(fn func(ctx context.Context, data *ProcessData) error) StateOption {
	return func(s *StateImpl) {
		s.onEnter = fn
	}
}

// WithOnExit 设置离开回调
func WithOnExit(fn func(ctx context.Context, data *ProcessData) error) StateOption {
	return func(s *StateImpl) {
		s.onExit = fn
	}
}

// AddState 添加状态
func (b *ProcessBuilder) AddState(name string, opts ...StateOption) *ProcessBuilder {
	// 检查重复
	if _, exists := b.states[name]; exists {
		b.errors = append(b.errors, fmt.Errorf("%w: %s", ErrDuplicateState, name))
		return b
	}

	state := &StateImpl{
		name:     name,
		metadata: make(map[string]any),
	}

	for _, opt := range opts {
		opt(state)
	}

	b.states[name] = state
	b.stateOrder = append(b.stateOrder, name)

	return b
}

// ============== 状态转换定义 ==============

// TransitionOption 转换配置选项
type TransitionOption func(*Transition)

// WithGuard 设置守卫条件
func WithGuard(fn func(ctx context.Context, data *ProcessData) bool) TransitionOption {
	return func(t *Transition) {
		t.Guard = fn
	}
}

// WithAction 设置转换动作
func WithAction(fn func(ctx context.Context, data *ProcessData) error) TransitionOption {
	return func(t *Transition) {
		t.Action = fn
	}
}

// WithTransitionPriority 设置转换优先级
func WithTransitionPriority(priority int) TransitionOption {
	return func(t *Transition) {
		t.Priority = priority
	}
}

// WithTransitionMetadata 设置转换元数据
func WithTransitionMetadata(key string, value any) TransitionOption {
	return func(t *Transition) {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		t.Metadata[key] = value
	}
}

// AddTransition 添加状态转换
func (b *ProcessBuilder) AddTransition(from, event, to string, opts ...TransitionOption) *ProcessBuilder {
	transition := &Transition{
		From:  from,
		Event: event,
		To:    to,
	}

	for _, opt := range opts {
		opt(transition)
	}

	b.transitions = append(b.transitions, transition)

	return b
}

// AddAutoTransition 添加自动转换（无需事件触发）
// 当进入源状态并满足条件时自动转换到目标状态
func (b *ProcessBuilder) AddAutoTransition(from, to string, guard func(ctx context.Context, data *ProcessData) bool) *ProcessBuilder {
	return b.AddTransition(from, "_auto_", to, WithGuard(guard))
}

// ============== 步骤绑定 ==============

// OnStateEnter 绑定状态进入步骤
func (b *ProcessBuilder) OnStateEnter(stateName string, step Step) *ProcessBuilder {
	b.stateSteps[stateName] = step
	return b
}

// OnStateEnterFunc 使用函数绑定状态进入步骤
func (b *ProcessBuilder) OnStateEnterFunc(stateName string, fn func(ctx context.Context, data *ProcessData) (any, error)) *ProcessBuilder {
	step := newInlineActionStep(stateName+"_step", fn)
	return b.OnStateEnter(stateName, step)
}

// inlineActionStep 内联动作步骤（用于 OnStateEnterFunc）
type inlineActionStep struct {
	id     string
	name   string
	action func(ctx context.Context, data *ProcessData) (any, error)
}

func newInlineActionStep(name string, action func(ctx context.Context, data *ProcessData) (any, error)) *inlineActionStep {
	return &inlineActionStep{
		id:     name,
		name:   name,
		action: action,
	}
}

func (s *inlineActionStep) ID() string          { return s.id }
func (s *inlineActionStep) Name() string        { return s.name }
func (s *inlineActionStep) Description() string { return "" }
func (s *inlineActionStep) Validate() error     { return nil }

func (s *inlineActionStep) Execute(ctx context.Context, data *ProcessData) (*StepResult, error) {
	output, err := s.action(ctx, data)
	return &StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  err == nil,
		Output:   output,
		Error:    err,
	}, err
}

// ============== 构建 ==============

// Build 构建流程定义
func (b *ProcessBuilder) Build() (Process, error) {
	// 检查构建过程中的错误
	if len(b.errors) > 0 {
		return nil, b.errors[0]
	}

	// 验证流程定义
	if err := b.validate(); err != nil {
		return nil, err
	}

	// 绑定步骤到状态
	for stateName, step := range b.stateSteps {
		if state, ok := b.states[stateName]; ok {
			state.SetStep(step)
		}
	}

	// 创建流程定义
	def := &ProcessDefinition{
		name:        b.name,
		description: b.description,
		states:      b.states,
		stateOrder:  b.stateOrder,
		transitions: b.transitions,
		metadata:    b.metadata,
		options:     b.options,
	}

	// 创建流程实例
	return NewProcessInstance(def), nil
}

// MustBuild 构建流程定义，失败时 panic
func (b *ProcessBuilder) MustBuild() Process {
	p, err := b.Build()
	if err != nil {
		panic(err)
	}
	return p
}

// validate 验证流程定义
func (b *ProcessBuilder) validate() error {
	// 检查是否有状态
	if len(b.states) == 0 {
		return fmt.Errorf("流程必须至少有一个状态")
	}

	// 检查初始状态
	hasInitial := false
	for _, state := range b.states {
		if state.IsInitial() {
			if hasInitial {
				return fmt.Errorf("只能有一个初始状态")
			}
			hasInitial = true
		}
	}
	if !hasInitial {
		return ErrNoInitialState
	}

	// 检查终止状态
	hasFinal := false
	for _, state := range b.states {
		if state.IsFinal() {
			hasFinal = true
			break
		}
	}
	if !hasFinal {
		return ErrNoFinalState
	}

	// 验证转换引用的状态存在
	for _, t := range b.transitions {
		if _, ok := b.states[t.From]; !ok {
			return fmt.Errorf("转换引用了不存在的源状态: %s", t.From)
		}
		if _, ok := b.states[t.To]; !ok {
			return fmt.Errorf("转换引用了不存在的目标状态: %s", t.To)
		}
	}

	return nil
}

// ============== 流程定义 ==============

// ProcessDefinition 流程定义
// 包含流程的完整定义，可以创建多个流程实例
type ProcessDefinition struct {
	name        string
	description string
	states      map[string]*StateImpl
	stateOrder  []string
	transitions []*Transition
	metadata    map[string]any
	options     *ProcessOptions
}

// Name 返回流程名称
func (d *ProcessDefinition) Name() string {
	return d.name
}

// Description 返回流程描述
func (d *ProcessDefinition) Description() string {
	return d.description
}

// GetInitialState 获取初始状态
func (d *ProcessDefinition) GetInitialState() *StateImpl {
	for _, state := range d.states {
		if state.IsInitial() {
			return state
		}
	}
	return nil
}

// GetState 获取状态
func (d *ProcessDefinition) GetState(name string) (*StateImpl, bool) {
	state, ok := d.states[name]
	return state, ok
}

// GetTransitions 获取从指定状态出发的转换
func (d *ProcessDefinition) GetTransitions(fromState string) []*Transition {
	var result []*Transition
	for _, t := range d.transitions {
		if t.From == fromState {
			result = append(result, t)
		}
	}
	return result
}

// GetTransitionByEvent 获取指定状态和事件的转换
func (d *ProcessDefinition) GetTransitionByEvent(fromState, event string) *Transition {
	for _, t := range d.transitions {
		if t.From == fromState && t.Event == event {
			return t
		}
	}
	return nil
}

