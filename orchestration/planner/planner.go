// Package planner 提供 AI Agent 的规划能力
//
// 规划器负责将复杂任务分解为可执行的步骤序列。
//
// 支持的规划器类型：
//   - SequentialPlanner: 顺序规划器，生成线性步骤序列
//   - StepwisePlanner: 逐步规划器，边执行边规划
//   - ActionPlanner: 动作规划器，选择单一最佳动作
//
// 使用示例：
//
//	planner := planner.NewSequentialPlanner(
//	    planner.WithSequentialPlannerLLM(llmProvider),
//	    planner.WithSequentialPlannerTools(tools...),
//	)
//	plan, err := planner.Plan(ctx, "完成用户注册流程")
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/llm"
)

// Planner 规划器接口
type Planner interface {
	// Name 返回规划器名称
	Name() string

	// Plan 创建执行计划
	Plan(ctx context.Context, goal string, opts ...PlanOption) (*Plan, error)

	// Replan 根据执行结果重新规划
	Replan(ctx context.Context, plan *Plan, feedback string) (*Plan, error)
}

// Plan 执行计划
type Plan struct {
	// ID 计划唯一标识
	ID string `json:"id"`

	// Goal 目标描述
	Goal string `json:"goal"`

	// Steps 执行步骤
	Steps []*Step `json:"steps"`

	// State 计划状态
	State PlanState `json:"state"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// Step 执行步骤
type Step struct {
	// ID 步骤唯一标识
	ID string `json:"id"`

	// Index 步骤序号
	Index int `json:"index"`

	// Description 步骤描述
	Description string `json:"description"`

	// Action 执行动作
	Action *Action `json:"action"`

	// State 步骤状态
	State StepState `json:"state"`

	// Dependencies 依赖的步骤 ID
	Dependencies []string `json:"dependencies,omitempty"`

	// Result 执行结果
	Result *StepResult `json:"result,omitempty"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Action 执行动作
type Action struct {
	// Type 动作类型
	Type ActionType `json:"type"`

	// Name 动作名称 (工具名/Agent名等)
	Name string `json:"name"`

	// Parameters 动作参数
	Parameters map[string]any `json:"parameters,omitempty"`

	// Description 动作描述
	Description string `json:"description,omitempty"`
}

// ActionType 动作类型
type ActionType string

const (
	// ActionTypeTool 工具调用
	ActionTypeTool ActionType = "tool"
	// ActionTypeAgent Agent 调用
	ActionTypeAgent ActionType = "agent"
	// ActionTypeLLM LLM 调用
	ActionTypeLLM ActionType = "llm"
	// ActionTypeFunction 函数调用
	ActionTypeFunction ActionType = "function"
	// ActionTypeSubPlan 子计划
	ActionTypeSubPlan ActionType = "subplan"
)

// StepResult 步骤执行结果
type StepResult struct {
	// Success 是否成功
	Success bool `json:"success"`

	// Output 输出内容
	Output any `json:"output,omitempty"`

	// Error 错误信息
	Error string `json:"error,omitempty"`

	// Duration 执行时长 (毫秒)
	Duration int64 `json:"duration_ms"`

	// Tokens 消耗的 Token 数
	Tokens int `json:"tokens,omitempty"`
}

// PlanState 计划状态
type PlanState string

const (
	// PlanStatePending 待执行
	PlanStatePending PlanState = "pending"
	// PlanStateRunning 执行中
	PlanStateRunning PlanState = "running"
	// PlanStateCompleted 已完成
	PlanStateCompleted PlanState = "completed"
	// PlanStateFailed 失败
	PlanStateFailed PlanState = "failed"
	// PlanStateCanceled 已取消
	PlanStateCanceled PlanState = "canceled"
)

// StepState 步骤状态
type StepState string

const (
	// StepStatePending 待执行
	StepStatePending StepState = "pending"
	// StepStateRunning 执行中
	StepStateRunning StepState = "running"
	// StepStateCompleted 已完成
	StepStateCompleted StepState = "completed"
	// StepStateFailed 失败
	StepStateFailed StepState = "failed"
	// StepStateSkipped 已跳过
	StepStateSkipped StepState = "skipped"
)

// PlanOption 规划选项
type PlanOption func(*planConfig)

type planConfig struct {
	maxSteps      int
	timeout       time.Duration
	availableTools []string
	context        map[string]any
}

// WithMaxSteps 设置最大步骤数
func WithMaxSteps(n int) PlanOption {
	return func(c *planConfig) {
		c.maxSteps = n
	}
}

// WithPlanTimeout 设置规划超时时间
func WithPlanTimeout(d time.Duration) PlanOption {
	return func(c *planConfig) {
		c.timeout = d
	}
}

// WithAvailableTools 设置可用工具列表
func WithAvailableTools(tools ...string) PlanOption {
	return func(c *planConfig) {
		c.availableTools = tools
	}
}

// WithPlanContext 设置规划上下文
func WithPlanContext(ctx map[string]any) PlanOption {
	return func(c *planConfig) {
		c.context = ctx
	}
}

// ============== Executor ==============

// Executor 计划执行器
type Executor interface {
	// Execute 执行计划
	Execute(ctx context.Context, plan *Plan) error

	// ExecuteStep 执行单个步骤
	ExecuteStep(ctx context.Context, step *Step) (*StepResult, error)

	// Pause 暂停执行
	Pause(ctx context.Context, plan *Plan) error

	// Resume 恢复执行
	Resume(ctx context.Context, plan *Plan) error

	// Cancel 取消执行
	Cancel(ctx context.Context, plan *Plan) error
}

// ============== Sequential Planner ==============

// SequentialPlanner 顺序规划器
// 生成线性执行的步骤序列
type SequentialPlanner struct {
	name  string
	llm   llm.Provider // LLM Provider
	tools []ToolInfo   // 可用工具信息
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// NewSequentialPlanner 创建顺序规划器
func NewSequentialPlanner(opts ...SequentialPlannerOption) *SequentialPlanner {
	p := &SequentialPlanner{
		name:  "sequential_planner",
		tools: make([]ToolInfo, 0),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// SequentialPlannerOption 顺序规划器选项
type SequentialPlannerOption func(*SequentialPlanner)

// WithSequentialPlannerName 设置规划器名称
func WithSequentialPlannerName(name string) SequentialPlannerOption {
	return func(p *SequentialPlanner) {
		p.name = name
	}
}

// WithSequentialPlannerLLM 设置 LLM Provider
func WithSequentialPlannerLLM(provider llm.Provider) SequentialPlannerOption {
	return func(p *SequentialPlanner) {
		p.llm = provider
	}
}

// WithSequentialPlannerTools 设置可用工具
func WithSequentialPlannerTools(tools ...ToolInfo) SequentialPlannerOption {
	return func(p *SequentialPlanner) {
		p.tools = append(p.tools, tools...)
	}
}

// Name 返回规划器名称
func (p *SequentialPlanner) Name() string {
	return p.name
}

// Plan 创建执行计划
func (p *SequentialPlanner) Plan(ctx context.Context, goal string, opts ...PlanOption) (*Plan, error) {
	config := &planConfig{
		maxSteps: 10,
		timeout:  30 * time.Second,
	}
	for _, opt := range opts {
		opt(config)
	}

	plan := &Plan{
		ID:        fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Goal:      goal,
		Steps:     make([]*Step, 0),
		State:     PlanStatePending,
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 如果没有配置 LLM，返回空计划
	if p.llm == nil {
		return plan, nil
	}

	// 构建工具描述
	toolsDesc := p.buildToolsDescription()

	// 使用 LLM 生成计划
	prompt := fmt.Sprintf(`你是一个任务规划专家。请将以下目标分解为可执行的步骤序列。

目标: %s

可用工具:
%s

要求:
1. 每个步骤应该是原子操作，可以独立执行
2. 步骤之间有明确的依赖关系
3. 最多 %d 个步骤
4. 返回 JSON 格式的步骤列表

返回格式 (仅返回 JSON，不要其他内容):
{
  "steps": [
    {
      "description": "步骤描述",
      "action": {
        "type": "tool|llm|function",
        "name": "动作名称",
        "parameters": {}
      },
      "dependencies": []
    }
  ]
}`, goal, toolsDesc, config.maxSteps)

	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	// 解析 LLM 响应
	steps, err := p.parseStepsFromResponse(resp.Content)
	if err != nil {
		// 解析失败时返回空计划
		return plan, nil
	}

	plan.Steps = steps
	return plan, nil
}

// buildToolsDescription 构建工具描述文本
func (p *SequentialPlanner) buildToolsDescription() string {
	if len(p.tools) == 0 {
		return "无可用工具"
	}

	var builder strings.Builder
	for i, tool := range p.tools {
		builder.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, tool.Name, tool.Description))
	}
	return builder.String()
}

// parseStepsFromResponse 从 LLM 响应解析步骤
func (p *SequentialPlanner) parseStepsFromResponse(content string) ([]*Step, error) {
	// 尝试提取 JSON 内容
	jsonContent := extractJSON(content)
	if jsonContent == "" {
		return nil, fmt.Errorf("无法从响应中提取 JSON")
	}

	var result struct {
		Steps []struct {
			Description  string   `json:"description"`
			Action       *Action  `json:"action"`
			Dependencies []string `json:"dependencies"`
		} `json:"steps"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	steps := make([]*Step, len(result.Steps))
	for i, s := range result.Steps {
		steps[i] = &Step{
			ID:           fmt.Sprintf("step-%d", i+1),
			Index:        i,
			Description:  s.Description,
			Action:       s.Action,
			State:        StepStatePending,
			Dependencies: s.Dependencies,
		}
	}

	return steps, nil
}

// extractJSON 从文本中提取 JSON 内容
func extractJSON(content string) string {
	// 查找 JSON 开始和结束位置
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || start >= end {
		return ""
	}
	return content[start : end+1]
}

// Replan 根据执行结果重新规划
func (p *SequentialPlanner) Replan(ctx context.Context, plan *Plan, feedback string) (*Plan, error) {
	plan.UpdatedAt = time.Now()

	// 如果没有配置 LLM，直接返回
	if p.llm == nil {
		return plan, nil
	}

	// 构建当前进度描述
	var progressBuilder strings.Builder
	for _, step := range plan.Steps {
		status := "待执行"
		switch step.State {
		case StepStateCompleted:
			status = "已完成"
		case StepStateFailed:
			status = "失败"
		case StepStateSkipped:
			status = "已跳过"
		case StepStateRunning:
			status = "执行中"
		}
		progressBuilder.WriteString(fmt.Sprintf("- %s: %s\n", step.Description, status))
	}

	// 使用 LLM 重新规划
	prompt := fmt.Sprintf(`你是一个任务规划专家。请根据执行反馈调整计划。

原始目标: %s

当前进度:
%s

执行反馈: %s

可用工具:
%s

请生成调整后的步骤列表，返回 JSON 格式:
{
  "steps": [
    {
      "description": "步骤描述",
      "action": {
        "type": "tool|llm|function",
        "name": "动作名称",
        "parameters": {}
      },
      "dependencies": []
    }
  ]
}`, plan.Goal, progressBuilder.String(), feedback, p.buildToolsDescription())

	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return plan, nil // 失败时保持原计划
	}

	// 解析新步骤
	newSteps, err := p.parseStepsFromResponse(resp.Content)
	if err != nil {
		return plan, nil
	}

	// 保留已完成的步骤，添加新步骤
	completedSteps := make([]*Step, 0)
	for _, step := range plan.Steps {
		if step.State == StepStateCompleted {
			completedSteps = append(completedSteps, step)
		}
	}

	// 更新新步骤的索引
	for i, step := range newSteps {
		step.Index = len(completedSteps) + i
		step.ID = fmt.Sprintf("step-%d", step.Index+1)
	}

	plan.Steps = append(completedSteps, newSteps...)
	return plan, nil
}

// ============== Stepwise Planner ==============

// StepwisePlanner 逐步规划器
// 边执行边规划，每次只规划下一步
type StepwisePlanner struct {
	name     string
	llm      llm.Provider // LLM Provider
	maxSteps int
	tools    []ToolInfo // 可用工具信息
}

// NewStepwisePlanner 创建逐步规划器
func NewStepwisePlanner(opts ...StepwisePlannerOption) *StepwisePlanner {
	p := &StepwisePlanner{
		name:     "stepwise_planner",
		maxSteps: 20,
		tools:    make([]ToolInfo, 0),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// StepwisePlannerOption 逐步规划器选项
type StepwisePlannerOption func(*StepwisePlanner)

// WithStepwisePlannerName 设置规划器名称
func WithStepwisePlannerName(name string) StepwisePlannerOption {
	return func(p *StepwisePlanner) {
		p.name = name
	}
}

// WithStepwisePlannerMaxSteps 设置最大步骤数
func WithStepwisePlannerMaxSteps(n int) StepwisePlannerOption {
	return func(p *StepwisePlanner) {
		p.maxSteps = n
	}
}

// WithStepwisePlannerLLM 设置 LLM Provider
func WithStepwisePlannerLLM(provider llm.Provider) StepwisePlannerOption {
	return func(p *StepwisePlanner) {
		p.llm = provider
	}
}

// WithStepwisePlannerTools 设置可用工具
func WithStepwisePlannerTools(tools ...ToolInfo) StepwisePlannerOption {
	return func(p *StepwisePlanner) {
		p.tools = append(p.tools, tools...)
	}
}

// Name 返回规划器名称
func (p *StepwisePlanner) Name() string {
	return p.name
}

// Plan 创建执行计划 (初始只规划第一步)
func (p *StepwisePlanner) Plan(ctx context.Context, goal string, opts ...PlanOption) (*Plan, error) {
	plan := &Plan{
		ID:        fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Goal:      goal,
		Steps:     make([]*Step, 0),
		State:     PlanStatePending,
		Metadata:  map[string]any{"type": "stepwise"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return plan, nil
}

// PlanNextStep 规划下一步
func (p *StepwisePlanner) PlanNextStep(ctx context.Context, plan *Plan, lastResult *StepResult) (*Step, error) {
	if len(plan.Steps) >= p.maxSteps {
		return nil, fmt.Errorf("已达到最大步骤数: %d", p.maxSteps)
	}

	// 如果没有配置 LLM，返回占位步骤
	if p.llm == nil {
		return &Step{
			ID:          fmt.Sprintf("step-%d", len(plan.Steps)+1),
			Index:       len(plan.Steps),
			Description: "下一步",
			State:       StepStatePending,
		}, nil
	}

	// 构建已执行步骤的历史
	var historyBuilder strings.Builder
	for _, step := range plan.Steps {
		status := "待执行"
		result := ""
		if step.Result != nil {
			if step.Result.Success {
				status = "成功"
				if step.Result.Output != nil {
					result = fmt.Sprintf(", 输出: %v", step.Result.Output)
				}
			} else {
				status = "失败"
				if step.Result.Error != "" {
					result = fmt.Sprintf(", 错误: %s", step.Result.Error)
				}
			}
		}
		historyBuilder.WriteString(fmt.Sprintf("步骤 %d: %s [%s%s]\n", step.Index+1, step.Description, status, result))
	}

	// 构建最后一步结果描述
	lastResultDesc := "无"
	if lastResult != nil {
		if lastResult.Success {
			lastResultDesc = fmt.Sprintf("成功，输出: %v", lastResult.Output)
		} else {
			lastResultDesc = fmt.Sprintf("失败，错误: %s", lastResult.Error)
		}
	}

	// 构建工具描述
	toolsDesc := p.buildToolsDescription()

	// 使用 LLM 规划下一步
	prompt := fmt.Sprintf(`你是一个任务规划专家。请根据当前进度规划下一步操作。

目标: %s

已执行步骤:
%s

上一步结果: %s

可用工具:
%s

请规划下一步操作。如果目标已完成，返回 {"done": true}。否则返回:
{
  "done": false,
  "step": {
    "description": "步骤描述",
    "action": {
      "type": "tool|llm|function",
      "name": "动作名称",
      "parameters": {}
    }
  }
}`, plan.Goal, historyBuilder.String(), lastResultDesc, toolsDesc)

	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	// 解析响应
	jsonContent := extractJSON(resp.Content)
	if jsonContent == "" {
		return nil, fmt.Errorf("无法从响应中提取 JSON")
	}

	var result struct {
		Done bool `json:"done"`
		Step *struct {
			Description string  `json:"description"`
			Action      *Action `json:"action"`
		} `json:"step"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	if result.Done {
		return nil, nil // 返回 nil 表示目标已完成
	}

	if result.Step == nil {
		return nil, fmt.Errorf("响应中缺少步骤信息")
	}

	step := &Step{
		ID:          fmt.Sprintf("step-%d", len(plan.Steps)+1),
		Index:       len(plan.Steps),
		Description: result.Step.Description,
		Action:      result.Step.Action,
		State:       StepStatePending,
	}

	return step, nil
}

// buildToolsDescription 构建工具描述文本
func (p *StepwisePlanner) buildToolsDescription() string {
	if len(p.tools) == 0 {
		return "无可用工具"
	}

	var builder strings.Builder
	for i, tool := range p.tools {
		builder.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, tool.Name, tool.Description))
	}
	return builder.String()
}

// Replan 根据执行结果重新规划
func (p *StepwisePlanner) Replan(ctx context.Context, plan *Plan, feedback string) (*Plan, error) {
	plan.UpdatedAt = time.Now()
	return plan, nil
}

// ============== Action Planner ==============

// ActionPlanner 动作规划器
// 选择单一最佳动作
type ActionPlanner struct {
	name    string
	llm     llm.Provider // LLM Provider
	actions []ActionDefinition
}

// ActionDefinition 动作定义
type ActionDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// NewActionPlanner 创建动作规划器
func NewActionPlanner(actions []ActionDefinition, opts ...ActionPlannerOption) *ActionPlanner {
	p := &ActionPlanner{
		name:    "action_planner",
		actions: actions,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ActionPlannerOption 动作规划器选项
type ActionPlannerOption func(*ActionPlanner)

// WithActionPlannerName 设置规划器名称
func WithActionPlannerName(name string) ActionPlannerOption {
	return func(p *ActionPlanner) {
		p.name = name
	}
}

// WithActionPlannerLLM 设置 LLM Provider
func WithActionPlannerLLM(provider llm.Provider) ActionPlannerOption {
	return func(p *ActionPlanner) {
		p.llm = provider
	}
}

// Name 返回规划器名称
func (p *ActionPlanner) Name() string {
	return p.name
}

// Plan 创建执行计划 (单一动作)
func (p *ActionPlanner) Plan(ctx context.Context, goal string, opts ...PlanOption) (*Plan, error) {
	plan := &Plan{
		ID:        fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Goal:      goal,
		Steps:     make([]*Step, 0),
		State:     PlanStatePending,
		Metadata:  map[string]any{"type": "action"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 如果没有可用动作，返回空计划
	if len(p.actions) == 0 {
		return plan, nil
	}

	// 如果没有配置 LLM，选择第一个动作
	if p.llm == nil {
		action := p.actions[0]
		plan.Steps = []*Step{{
			ID:          "step-1",
			Index:       0,
			Description: action.Description,
			Action: &Action{
				Type:       ActionTypeFunction,
				Name:       action.Name,
				Parameters: action.Parameters,
			},
			State: StepStatePending,
		}}
		return plan, nil
	}

	// 构建动作描述
	var actionsBuilder strings.Builder
	for i, action := range p.actions {
		actionsBuilder.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, action.Name, action.Description))
	}

	// 使用 LLM 选择最佳动作
	prompt := fmt.Sprintf(`你是一个决策专家。请为以下目标选择最合适的单一动作。

目标: %s

可选动作:
%s

请选择最适合完成目标的动作。返回 JSON 格式:
{
  "action": {
    "name": "动作名称",
    "parameters": {},
    "reason": "选择原因"
  }
}`, goal, actionsBuilder.String())

	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		// 失败时选择第一个动作
		action := p.actions[0]
		plan.Steps = []*Step{{
			ID:          "step-1",
			Index:       0,
			Description: action.Description,
			Action: &Action{
				Type:       ActionTypeFunction,
				Name:       action.Name,
				Parameters: action.Parameters,
			},
			State: StepStatePending,
		}}
		return plan, nil
	}

	// 解析响应
	jsonContent := extractJSON(resp.Content)
	if jsonContent == "" {
		return plan, nil
	}

	var result struct {
		Action struct {
			Name       string         `json:"name"`
			Parameters map[string]any `json:"parameters"`
			Reason     string         `json:"reason"`
		} `json:"action"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return plan, nil
	}

	// 查找匹配的动作定义
	var selectedAction *ActionDefinition
	for _, action := range p.actions {
		if action.Name == result.Action.Name {
			selectedAction = &action
			break
		}
	}

	if selectedAction == nil && len(p.actions) > 0 {
		selectedAction = &p.actions[0]
	}

	if selectedAction != nil {
		// 合并参数
		params := selectedAction.Parameters
		if params == nil {
			params = make(map[string]any)
		}
		for k, v := range result.Action.Parameters {
			params[k] = v
		}

		plan.Steps = []*Step{{
			ID:          "step-1",
			Index:       0,
			Description: selectedAction.Description,
			Action: &Action{
				Type:        ActionTypeFunction,
				Name:        selectedAction.Name,
				Parameters:  params,
				Description: result.Action.Reason,
			},
			State: StepStatePending,
		}}
	}

	return plan, nil
}

// Replan 根据执行结果重新规划
func (p *ActionPlanner) Replan(ctx context.Context, plan *Plan, feedback string) (*Plan, error) {
	plan.UpdatedAt = time.Now()
	return plan, nil
}

// Ensure interfaces are implemented
var (
	_ Planner = (*SequentialPlanner)(nil)
	_ Planner = (*StepwisePlanner)(nil)
	_ Planner = (*ActionPlanner)(nil)
)
