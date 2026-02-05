// Package step 提供流程步骤实现
//
// 本文件实现 AgentStep，将 AI Agent 作为流程步骤执行。
package step

import (
	"context"
	"time"

	"github.com/everyday-items/hexagon/agent"
	"github.com/everyday-items/hexagon/process"
	"github.com/everyday-items/toolkit/util/idgen"
)

// AgentStep Agent 步骤
// 将 Agent 作为流程中的一个步骤执行
type AgentStep struct {
	BaseStep

	// agent 执行的 Agent
	agent agent.Agent

	// inputBuilder 输入构建器
	inputBuilder func(data *process.ProcessData) agent.Input

	// outputParser 输出解析器
	outputParser func(output agent.Output, data *process.ProcessData)

	// timeout 执行超时
	timeout time.Duration

	// retries 重试次数
	retries int
}

// AgentStepOption AgentStep 配置选项
type AgentStepOption func(*AgentStep)

// WithAgentTimeout 设置超时
func WithAgentTimeout(timeout time.Duration) AgentStepOption {
	return func(s *AgentStep) {
		s.timeout = timeout
	}
}

// WithAgentRetries 设置重试次数
func WithAgentRetries(retries int) AgentStepOption {
	return func(s *AgentStep) {
		s.retries = retries
	}
}

// WithInputBuilder 设置输入构建器
func WithInputBuilder(fn func(data *process.ProcessData) agent.Input) AgentStepOption {
	return func(s *AgentStep) {
		s.inputBuilder = fn
	}
}

// WithOutputParser 设置输出解析器
func WithOutputParser(fn func(output agent.Output, data *process.ProcessData)) AgentStepOption {
	return func(s *AgentStep) {
		s.outputParser = fn
	}
}

// NewAgentStep 创建 Agent 步骤
func NewAgentStep(name string, ag agent.Agent, opts ...AgentStepOption) *AgentStep {
	s := &AgentStep{
		BaseStep: BaseStep{
			id:          idgen.NanoID(),
			name:        name,
			description: "执行 Agent: " + ag.Name(),
		},
		agent: ag,
		// 默认输入构建器：从流程数据中获取 query 字段
		inputBuilder: func(data *process.ProcessData) agent.Input {
			query, _ := data.Get("query")
			queryStr, _ := query.(string)
			return agent.Input{
				Query:   queryStr,
				Context: data.Variables,
			}
		},
		// 默认输出解析器：将输出保存到 agent_output 变量
		outputParser: func(output agent.Output, data *process.ProcessData) {
			data.Set("agent_output", output.Content)
			data.Set("agent_tool_calls", output.ToolCalls)
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *AgentStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	// 如果设置了超时
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// 构建输入
	input := s.inputBuilder(data)

	// 执行 Agent（带重试）
	var output agent.Output
	var err error
	retries := s.retries
	if retries <= 0 {
		retries = 1
	}

	for attempt := 0; attempt < retries; attempt++ {
		output, err = s.agent.Invoke(ctx, input)
		if err == nil {
			break
		}

		// 最后一次尝试不等待
		if attempt < retries-1 {
			time.Sleep(time.Second * time.Duration(attempt+1))
		}
	}

	duration := time.Since(start)

	if err != nil {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  false,
			Error:    err,
			Duration: duration,
			Metadata: map[string]any{
				"agent_name": s.agent.Name(),
				"agent_id":   s.agent.ID(),
			},
		}, err
	}

	// 解析输出
	if s.outputParser != nil {
		s.outputParser(output, data)
	}

	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  true,
		Output:   output,
		Duration: duration,
		Metadata: map[string]any{
			"agent_name": s.agent.Name(),
			"agent_id":   s.agent.ID(),
			"usage":      output.Usage,
		},
	}, nil
}

// GetAgent 获取 Agent
func (s *AgentStep) GetAgent() agent.Agent {
	return s.agent
}

// ============== AgentChainStep ==============

// AgentChainStep 多 Agent 链式执行步骤
// 按顺序执行多个 Agent，上一个 Agent 的输出作为下一个的输入
type AgentChainStep struct {
	BaseStep
	agents        []agent.Agent
	passThrough   bool // 是否将中间结果传递给下一个 Agent
}

// AgentChainOption AgentChainStep 配置选项
type AgentChainOption func(*AgentChainStep)

// WithPassThrough 启用结果传递
func WithPassThrough() AgentChainOption {
	return func(s *AgentChainStep) {
		s.passThrough = true
	}
}

// NewAgentChainStep 创建 Agent 链步骤
func NewAgentChainStep(name string, agents []agent.Agent, opts ...AgentChainOption) *AgentChainStep {
	s := &AgentChainStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		agents:      agents,
		passThrough: true,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Execute 执行步骤
func (s *AgentChainStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	if len(s.agents) == 0 {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  true,
			Output:   nil,
			Duration: time.Since(start),
		}, nil
	}

	// 获取初始查询
	query, _ := data.Get("query")
	currentQuery, _ := query.(string)

	var lastOutput agent.Output
	outputs := make([]agent.Output, 0, len(s.agents))

	for _, ag := range s.agents {
		select {
		case <-ctx.Done():
			return &process.StepResult{
				StepID:   s.id,
				StepName: s.name,
				Success:  false,
				Error:    ctx.Err(),
				Duration: time.Since(start),
			}, ctx.Err()
		default:
		}

		input := agent.Input{
			Query:   currentQuery,
			Context: data.Variables,
		}

		output, err := ag.Invoke(ctx, input)
		if err != nil {
			return &process.StepResult{
				StepID:   s.id,
				StepName: s.name,
				Success:  false,
				Output:   outputs,
				Error:    err,
				Duration: time.Since(start),
				Metadata: map[string]any{
					"failed_agent": ag.Name(),
				},
			}, err
		}

		outputs = append(outputs, output)
		lastOutput = output

		// 将输出作为下一个 Agent 的输入
		if s.passThrough {
			currentQuery = output.Content
		}
	}

	// 保存最终输出到流程数据
	data.Set("agent_output", lastOutput.Content)

	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  true,
		Output:   lastOutput,
		Duration: time.Since(start),
		Metadata: map[string]any{
			"chain_length": len(s.agents),
			"all_outputs":  outputs,
		},
	}, nil
}

// ============== ConditionalAgentStep ==============

// ConditionalAgentStep 条件 Agent 步骤
// 根据条件选择执行不同的 Agent
type ConditionalAgentStep struct {
	BaseStep
	condition    func(ctx context.Context, data *process.ProcessData) string // 返回 Agent 名称
	agents       map[string]agent.Agent
	defaultAgent agent.Agent
}

// NewConditionalAgentStep 创建条件 Agent 步骤
func NewConditionalAgentStep(
	name string,
	condition func(ctx context.Context, data *process.ProcessData) string,
	agents map[string]agent.Agent,
	defaultAgent agent.Agent,
) *ConditionalAgentStep {
	return &ConditionalAgentStep{
		BaseStep: BaseStep{
			id:   idgen.NanoID(),
			name: name,
		},
		condition:    condition,
		agents:       agents,
		defaultAgent: defaultAgent,
	}
}

// Execute 执行步骤
func (s *ConditionalAgentStep) Execute(ctx context.Context, data *process.ProcessData) (*process.StepResult, error) {
	start := time.Now()

	// 评估条件获取 Agent 名称
	agentName := s.condition(ctx, data)

	// 选择 Agent
	selectedAgent, ok := s.agents[agentName]
	if !ok {
		selectedAgent = s.defaultAgent
	}

	if selectedAgent == nil {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  false,
			Error:    process.ErrStepFailed,
			Duration: time.Since(start),
			Metadata: map[string]any{
				"error": "没有可用的 Agent",
			},
		}, process.ErrStepFailed
	}

	// 构建输入
	query, _ := data.Get("query")
	queryStr, _ := query.(string)
	input := agent.Input{
		Query:   queryStr,
		Context: data.Variables,
	}

	// 执行选中的 Agent
	output, err := selectedAgent.Invoke(ctx, input)
	duration := time.Since(start)

	if err != nil {
		return &process.StepResult{
			StepID:   s.id,
			StepName: s.name,
			Success:  false,
			Error:    err,
			Duration: duration,
			Metadata: map[string]any{
				"selected_agent": selectedAgent.Name(),
			},
		}, err
	}

	// 保存输出
	data.Set("agent_output", output.Content)

	return &process.StepResult{
		StepID:   s.id,
		StepName: s.name,
		Success:  true,
		Output:   output,
		Duration: duration,
		Metadata: map[string]any{
			"selected_agent": selectedAgent.Name(),
			"agent_id":       selectedAgent.ID(),
		},
	}, nil
}
