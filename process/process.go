// Package process 提供确定性业务流程框架
//
// Process Framework 是状态机驱动的流程框架，支持：
// - 确定性的状态转换
// - 事件驱动的流程推进
// - Agent 集成（将 Agent 作为步骤）
// - 人在回路（HITL）支持
// - 流程检查点与恢复
//
// 设计理念：
// - 状态机保证流程的确定性和可预测性
// - 与 Agent 系统深度集成，实现智能与确定性的结合
// - 对标 Semantic Kernel Process Framework
//
// 使用示例：
//
//	process := NewProcess("order-processing").
//	    AddState("pending", AsInitial()).
//	    AddState("validated").
//	    AddState("completed", AsFinal()).
//	    AddTransition("pending", "validate", "validated").
//	    AddTransition("validated", "complete", "completed").
//	    OnStateEnter("validated", NewAgentStep("validator", validatorAgent)).
//	    Build()
//
//	output, err := process.Run(ctx, ProcessInput{Data: orderData})
package process

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/core"
)

// ============== 错误定义 ==============

var (
	// ErrNoInitialState 没有初始状态
	ErrNoInitialState = errors.New("process: no initial state defined")

	// ErrNoFinalState 没有终止状态
	ErrNoFinalState = errors.New("process: no final state defined")

	// ErrInvalidTransition 无效的状态转换
	ErrInvalidTransition = errors.New("process: invalid state transition")

	// ErrProcessNotStarted 流程未启动
	ErrProcessNotStarted = errors.New("process: process not started")

	// ErrProcessAlreadyStarted 流程已启动
	ErrProcessAlreadyStarted = errors.New("process: process already started")

	// ErrProcessPaused 流程已暂停
	ErrProcessPaused = errors.New("process: process is paused")

	// ErrProcessCompleted 流程已完成
	ErrProcessCompleted = errors.New("process: process already completed")

	// ErrProcessCancelled 流程已取消
	ErrProcessCancelled = errors.New("process: process cancelled")

	// ErrGuardFailed 守卫条件失败
	ErrGuardFailed = errors.New("process: transition guard failed")

	// ErrStepFailed 步骤执行失败
	ErrStepFailed = errors.New("process: step execution failed")

	// ErrStateNotFound 状态未找到
	ErrStateNotFound = errors.New("process: state not found")

	// ErrDuplicateState 重复的状态
	ErrDuplicateState = errors.New("process: duplicate state")

	// ErrDuplicateTransition 重复的转换
	ErrDuplicateTransition = errors.New("process: duplicate transition")
)

// ============== 流程状态枚举 ==============

// ProcessStatus 流程执行状态
type ProcessStatus string

const (
	// StatusPending 待启动
	StatusPending ProcessStatus = "pending"

	// StatusRunning 运行中
	StatusRunning ProcessStatus = "running"

	// StatusPaused 已暂停
	StatusPaused ProcessStatus = "paused"

	// StatusCompleted 已完成
	StatusCompleted ProcessStatus = "completed"

	// StatusFailed 已失败
	StatusFailed ProcessStatus = "failed"

	// StatusCancelled 已取消
	StatusCancelled ProcessStatus = "cancelled"
)

// IsTerminal 检查是否为终态
func (s ProcessStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// ============== 核心接口定义 ==============

// Process 业务流程接口
// 定义流程的生命周期管理方法
type Process interface {
	// 实现 Runnable 接口
	core.Runnable[ProcessInput, ProcessOutput]

	// ID 返回流程实例 ID
	ID() string

	// Name 返回流程名称
	Name() string

	// CurrentState 返回当前状态
	CurrentState() State

	// Status 返回流程执行状态
	Status() ProcessStatus

	// Start 启动流程
	Start(ctx context.Context, input ProcessInput) error

	// SendEvent 发送事件触发状态转换
	SendEvent(ctx context.Context, event Event) error

	// Pause 暂停流程
	Pause(ctx context.Context) error

	// Resume 恢复流程
	Resume(ctx context.Context) error

	// Cancel 取消流程
	Cancel(ctx context.Context) error

	// GetHistory 获取执行历史
	GetHistory() []ExecutionRecord

	// GetData 获取流程数据
	GetData() *ProcessData

	// Subscribe 订阅流程事件
	Subscribe(handler EventHandler)
}

// ProcessInput 流程输入
type ProcessInput struct {
	// Data 初始数据
	Data map[string]any

	// InitialState 初始状态名（可选，默认使用定义的初始状态）
	InitialState string

	// Metadata 元数据
	Metadata map[string]any

	// CheckpointID 从检查点恢复时使用
	CheckpointID string
}

// ProcessOutput 流程输出
type ProcessOutput struct {
	// Data 最终数据
	Data map[string]any

	// FinalState 最终状态名
	FinalState string

	// ExecutionTime 执行时长
	ExecutionTime time.Duration

	// StepResults 各步骤的执行结果
	StepResults map[string]StepResult

	// History 执行历史
	History []ExecutionRecord

	// Error 错误信息（如果失败）
	Error error

	// Metadata 元数据
	Metadata map[string]any
}

// ProcessData 流程共享数据
// 用于在步骤间传递数据
type ProcessData struct {
	// Input 原始输入数据
	Input map[string]any

	// Variables 流程变量
	Variables map[string]any

	// StepOutputs 各步骤的输出
	StepOutputs map[string]any

	// mu 保护并发访问
	mu sync.RWMutex
}

// NewProcessData 创建流程数据
func NewProcessData(input map[string]any) *ProcessData {
	return &ProcessData{
		Input:       copyMap(input),
		Variables:   make(map[string]any),
		StepOutputs: make(map[string]any),
	}
}

// Get 获取变量
func (d *ProcessData) Get(key string) (any, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 优先从变量中查找
	if v, ok := d.Variables[key]; ok {
		return v, true
	}
	// 其次从输入中查找
	if v, ok := d.Input[key]; ok {
		return v, true
	}
	return nil, false
}

// Set 设置变量
func (d *ProcessData) Set(key string, value any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Variables[key] = value
}

// GetString 获取字符串变量
func (d *ProcessData) GetString(key string) string {
	v, ok := d.Get(key)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetInt 获取整数变量
func (d *ProcessData) GetInt(key string) int {
	v, ok := d.Get(key)
	if !ok {
		return 0
	}
	switch i := v.(type) {
	case int:
		return i
	case int64:
		return int(i)
	case float64:
		return int(i)
	default:
		return 0
	}
}

// GetBool 获取布尔变量
func (d *ProcessData) GetBool(key string) bool {
	v, ok := d.Get(key)
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// SetStepOutput 设置步骤输出
func (d *ProcessData) SetStepOutput(stepID string, output any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.StepOutputs[stepID] = output
}

// GetStepOutput 获取步骤输出
func (d *ProcessData) GetStepOutput(stepID string) (any, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	v, ok := d.StepOutputs[stepID]
	return v, ok
}

// Clone 克隆流程数据
func (d *ProcessData) Clone() *ProcessData {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return &ProcessData{
		Input:       copyMap(d.Input),
		Variables:   copyMap(d.Variables),
		StepOutputs: copyMap(d.StepOutputs),
	}
}

// ============== 状态定义 ==============

// State 状态接口
type State interface {
	// Name 状态名称
	Name() string

	// IsInitial 是否为初始状态
	IsInitial() bool

	// IsFinal 是否为终止状态
	IsFinal() bool

	// OnEnter 进入状态时执行
	OnEnter(ctx context.Context, data *ProcessData) error

	// OnExit 离开状态时执行
	OnExit(ctx context.Context, data *ProcessData) error

	// GetStep 获取进入时执行的步骤
	GetStep() Step

	// SetStep 设置进入时执行的步骤
	SetStep(step Step)

	// GetMetadata 获取元数据
	GetMetadata() map[string]any
}

// StateImpl 状态实现
type StateImpl struct {
	name     string
	initial  bool
	final    bool
	step     Step
	onEnter  func(ctx context.Context, data *ProcessData) error
	onExit   func(ctx context.Context, data *ProcessData) error
	metadata map[string]any
}

// Name 返回状态名称
func (s *StateImpl) Name() string {
	return s.name
}

// IsInitial 是否为初始状态
func (s *StateImpl) IsInitial() bool {
	return s.initial
}

// IsFinal 是否为终止状态
func (s *StateImpl) IsFinal() bool {
	return s.final
}

// OnEnter 进入状态时执行
func (s *StateImpl) OnEnter(ctx context.Context, data *ProcessData) error {
	// 执行自定义进入回调
	if s.onEnter != nil {
		if err := s.onEnter(ctx, data); err != nil {
			return err
		}
	}

	// 执行关联的步骤
	if s.step != nil {
		result, err := s.step.Execute(ctx, data)
		if err != nil {
			return fmt.Errorf("步骤 %s 执行失败: %w", s.step.Name(), err)
		}
		// 保存步骤输出
		data.SetStepOutput(s.step.ID(), result.Output)
	}

	return nil
}

// OnExit 离开状态时执行
func (s *StateImpl) OnExit(ctx context.Context, data *ProcessData) error {
	if s.onExit != nil {
		return s.onExit(ctx, data)
	}
	return nil
}

// GetStep 获取步骤
func (s *StateImpl) GetStep() Step {
	return s.step
}

// SetStep 设置步骤
func (s *StateImpl) SetStep(step Step) {
	s.step = step
}

// GetMetadata 获取元数据
func (s *StateImpl) GetMetadata() map[string]any {
	return s.metadata
}

// ============== 状态转换定义 ==============

// Transition 状态转换
type Transition struct {
	// From 源状态
	From string

	// To 目标状态
	To string

	// Event 触发事件
	Event string

	// Guard 守卫条件（返回 true 才执行转换）
	Guard func(ctx context.Context, data *ProcessData) bool

	// Action 转换动作
	Action func(ctx context.Context, data *ProcessData) error

	// Priority 优先级（同一事件多个转换时使用）
	Priority int

	// Metadata 元数据
	Metadata map[string]any
}

// CanTransit 检查是否可以转换
func (t *Transition) CanTransit(ctx context.Context, data *ProcessData) bool {
	if t.Guard == nil {
		return true
	}
	return t.Guard(ctx, data)
}

// Execute 执行转换动作
func (t *Transition) Execute(ctx context.Context, data *ProcessData) error {
	if t.Action == nil {
		return nil
	}
	return t.Action(ctx, data)
}

// ============== 事件定义 ==============

// Event 事件
type Event struct {
	// Name 事件名称
	Name string

	// Data 事件数据
	Data map[string]any

	// Source 事件来源
	Source string

	// Timestamp 事件时间
	Timestamp time.Time
}

// NewEvent 创建事件
func NewEvent(name string) Event {
	return Event{
		Name:      name,
		Data:      make(map[string]any),
		Timestamp: time.Now(),
	}
}

// WithData 添加事件数据
func (e Event) WithData(key string, value any) Event {
	if e.Data == nil {
		e.Data = make(map[string]any)
	}
	e.Data[key] = value
	return e
}

// WithSource 设置事件来源
func (e Event) WithSource(source string) Event {
	e.Source = source
	return e
}

// EventHandler 事件处理器
type EventHandler func(event ProcessEvent)

// ProcessEvent 流程事件（用于订阅通知）
type ProcessEvent struct {
	// Type 事件类型
	Type ProcessEventType

	// ProcessID 流程 ID
	ProcessID string

	// StateName 状态名
	StateName string

	// EventName 触发事件名
	EventName string

	// Data 事件数据
	Data map[string]any

	// Timestamp 时间戳
	Timestamp time.Time

	// Error 错误（如果有）
	Error error
}

// ProcessEventType 流程事件类型
type ProcessEventType string

const (
	// EventTypeStateEnter 进入状态
	EventTypeStateEnter ProcessEventType = "state_enter"

	// EventTypeStateExit 离开状态
	EventTypeStateExit ProcessEventType = "state_exit"

	// EventTypeTransition 状态转换
	EventTypeTransition ProcessEventType = "transition"

	// EventTypeStepStart 步骤开始
	EventTypeStepStart ProcessEventType = "step_start"

	// EventTypeStepEnd 步骤结束
	EventTypeStepEnd ProcessEventType = "step_end"

	// EventTypeProcessStart 流程开始
	EventTypeProcessStart ProcessEventType = "process_start"

	// EventTypeProcessEnd 流程结束
	EventTypeProcessEnd ProcessEventType = "process_end"

	// EventTypeProcessPaused 流程暂停
	EventTypeProcessPaused ProcessEventType = "process_paused"

	// EventTypeProcessResumed 流程恢复
	EventTypeProcessResumed ProcessEventType = "process_resumed"

	// EventTypeError 错误
	EventTypeError ProcessEventType = "error"
)

// ============== 步骤结果 ==============

// StepResult 步骤执行结果
type StepResult struct {
	// StepID 步骤 ID
	StepID string

	// StepName 步骤名称
	StepName string

	// Success 是否成功
	Success bool

	// Output 输出数据
	Output any

	// Error 错误信息
	Error error

	// Duration 执行时长
	Duration time.Duration

	// Metadata 元数据
	Metadata map[string]any
}

// ============== 执行记录 ==============

// ExecutionRecord 执行记录
type ExecutionRecord struct {
	// ID 记录 ID
	ID string

	// Timestamp 时间戳
	Timestamp time.Time

	// Type 记录类型
	Type RecordType

	// FromState 源状态
	FromState string

	// ToState 目标状态
	ToState string

	// Event 触发事件
	Event string

	// StepID 步骤 ID
	StepID string

	// StepName 步骤名称
	StepName string

	// Duration 执行时长
	Duration time.Duration

	// Success 是否成功
	Success bool

	// Error 错误信息
	Error error

	// Data 相关数据
	Data map[string]any
}

// RecordType 记录类型
type RecordType string

const (
	// RecordTypeTransition 状态转换
	RecordTypeTransition RecordType = "transition"

	// RecordTypeStepExecution 步骤执行
	RecordTypeStepExecution RecordType = "step_execution"

	// RecordTypeStateEnter 进入状态
	RecordTypeStateEnter RecordType = "state_enter"

	// RecordTypeStateExit 离开状态
	RecordTypeStateExit RecordType = "state_exit"
)

// ============== 步骤接口 ==============

// Step 步骤接口
// 定义流程中可执行的步骤
type Step interface {
	// ID 步骤 ID
	ID() string

	// Name 步骤名称
	Name() string

	// Description 步骤描述
	Description() string

	// Execute 执行步骤
	Execute(ctx context.Context, data *ProcessData) (*StepResult, error)

	// Validate 验证步骤配置
	Validate() error
}

// ============== 辅助函数 ==============

// copyMap 复制 map
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
