// Package agent 提供 AI Agent 核心接口和实现
//
// 本包实现了多种 Agent 类型，包括：
//   - BaseAgent: 基础 Agent，提供通用能力
//   - ReActAgent: 实现 ReAct (Reasoning + Acting) 推理模式
//   - Team: 多 Agent 协作团队，支持顺序/层级/协作/轮询四种模式
//   - SwarmRunner: 模仿 OpenAI Swarm 的 Agent 交接运行器
//
// 状态管理：
//   - TurnState: 单轮对话状态
//   - SessionState: 会话级状态
//   - AgentState: Agent 持久状态
//   - GlobalState: 跨 Agent 共享状态
//
// 使用示例：
//
//	agent := NewReAct(
//	    WithName("assistant"),
//	    WithLLM(llmProvider),
//	    WithTools(searchTool, calculatorTool),
//	)
//	output, err := agent.Run(ctx, Input{Query: "Hello"})
package agent

import (
	"context"
	"fmt"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/core"
	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/stream"
)

// Input 是 Agent 的输入
type Input struct {
	// Query 用户查询
	Query string `json:"query"`

	// Context 额外上下文
	Context map[string]any `json:"context,omitempty"`
}

// Output 是 Agent 的输出
type Output struct {
	// Content 最终回复内容
	Content string `json:"content"`

	// ToolCalls 执行的工具调用记录
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`

	// Usage Token 使用统计
	Usage llm.Usage `json:"usage,omitempty"`

	// Metadata 额外元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolCallRecord 记录工具调用
type ToolCallRecord struct {
	// Name 工具名称
	Name string `json:"name"`

	// Arguments 工具参数
	Arguments map[string]any `json:"arguments"`

	// Result 工具结果
	Result tool.Result `json:"result"`
}

// Agent 是 AI Agent 的核心接口
// 继承 Runnable 接口，添加 Agent 特有的方法
type Agent interface {
	core.Runnable[Input, Output]

	// ID 返回 Agent 唯一标识
	ID() string

	// Role 返回 Agent 的角色定义
	Role() Role

	// Tools 返回 Agent 可用的工具列表
	Tools() []tool.Tool

	// Memory 返回 Agent 的记忆系统
	Memory() memory.Memory

	// LLM 返回 Agent 使用的 LLM Provider
	LLM() llm.Provider

	// Run 执行 Agent（向后兼容方法）
	// Deprecated: 请使用 Invoke
	Run(ctx context.Context, input Input) (Output, error)
}

// Config 是 Agent 的配置
type Config struct {
	// ID Agent 唯一标识
	ID string

	// Name Agent 名称
	Name string

	// Description Agent 描述
	Description string

	// Role Agent 角色定义
	Role Role

	// SystemPrompt 系统提示词
	SystemPrompt string

	// LLM LLM 提供者
	LLM llm.Provider

	// Tools 可用工具列表
	Tools []tool.Tool

	// Memory 记忆系统
	Memory memory.Memory

	// MaxIterations 最大迭代次数（防止无限循环）
	MaxIterations int

	// Verbose 是否输出详细日志
	Verbose bool
}

// Option 是 Agent 配置选项
type Option func(*Config)

// WithID 设置 Agent ID
func WithID(id string) Option {
	return func(c *Config) {
		c.ID = id
	}
}

// WithName 设置 Agent 名称
func WithName(name string) Option {
	return func(c *Config) {
		c.Name = name
	}
}

// WithDescription 设置 Agent 描述
func WithDescription(desc string) Option {
	return func(c *Config) {
		c.Description = desc
	}
}

// WithSystemPrompt 设置系统提示词
func WithSystemPrompt(prompt string) Option {
	return func(c *Config) {
		c.SystemPrompt = prompt
	}
}

// WithLLM 设置 LLM 提供者
func WithLLM(provider llm.Provider) Option {
	return func(c *Config) {
		c.LLM = provider
	}
}

// WithTools 设置工具列表
func WithTools(tools ...tool.Tool) Option {
	return func(c *Config) {
		c.Tools = append(c.Tools, tools...)
	}
}

// WithMemory 设置记忆系统
func WithMemory(mem memory.Memory) Option {
	return func(c *Config) {
		c.Memory = mem
	}
}

// WithMaxIterations 设置最大迭代次数
func WithMaxIterations(n int) Option {
	return func(c *Config) {
		c.MaxIterations = n
	}
}

// WithVerbose 设置详细输出模式
func WithVerbose(v bool) Option {
	return func(c *Config) {
		c.Verbose = v
	}
}

// WithRole 设置 Agent 角色
func WithRole(role Role) Option {
	return func(c *Config) {
		c.Role = role
	}
}

// MemorySetter 允许外部替换 Agent 的记忆系统
//
// 用于共享记忆场景：Team 通过此接口将 Agent 原始记忆包装为 SharedMemoryProxy，
// 实现跨 Agent 记忆自动共享。BaseAgent 和 ReActAgent 均实现此接口。
type MemorySetter interface {
	SetMemory(mem memory.Memory)
}

// BaseAgent 提供 Agent 的基础实现
type BaseAgent struct {
	config Config
}

// SetMemory 替换 Agent 的记忆系统
//
// 此方法用于共享记忆代理注入，不应在常规业务代码中调用。
func (a *BaseAgent) SetMemory(mem memory.Memory) {
	a.config.Memory = mem
}

// NewBaseAgent 创建基础 Agent
func NewBaseAgent(opts ...Option) *BaseAgent {
	cfg := Config{
		ID:            generateID(),
		Name:          "Agent",
		Description:   "Base AI Agent",
		MaxIterations: 10,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.Memory == nil {
		cfg.Memory = memory.NewBuffer(100)
	}

	return &BaseAgent{config: cfg}
}

// ID 返回 Agent ID
func (a *BaseAgent) ID() string {
	return a.config.ID
}

// Name 返回 Agent 名称
func (a *BaseAgent) Name() string {
	return a.config.Name
}

// Role 返回 Agent 角色
func (a *BaseAgent) Role() Role {
	return a.config.Role
}

// Description 返回 Agent 描述
func (a *BaseAgent) Description() string {
	return a.config.Description
}

// Tools 返回工具列表
func (a *BaseAgent) Tools() []tool.Tool {
	return a.config.Tools
}

// Memory 返回记忆系统
func (a *BaseAgent) Memory() memory.Memory {
	return a.config.Memory
}

// LLM 返回 LLM 提供者
func (a *BaseAgent) LLM() llm.Provider {
	return a.config.LLM
}

// Config 返回配置（用于子类访问）
func (a *BaseAgent) Config() Config {
	return a.config
}

// Invoke 执行 Agent
// BaseAgent 提供简单的 LLM 对话实现，子类可以覆盖此方法实现更复杂的逻辑
func (a *BaseAgent) Invoke(ctx context.Context, input Input, opts ...core.Option) (Output, error) {
	if a.config.LLM == nil {
		return Output{}, fmt.Errorf("LLM provider not configured")
	}

	// 构建消息
	messages := make([]llm.Message, 0, 2)
	if a.config.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: a.config.SystemPrompt,
		})
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: input.Query,
	})

	// 调用 LLM
	resp, err := a.config.LLM.Complete(ctx, llm.CompletionRequest{
		Messages: messages,
	})
	if err != nil {
		return Output{}, fmt.Errorf("LLM completion failed: %w", err)
	}

	return Output{
		Content: resp.Content,
		Usage:   resp.Usage,
	}, nil
}

// Run 是 Invoke 的别名（向后兼容）
// Deprecated: 请使用 Invoke
func (a *BaseAgent) Run(ctx context.Context, input Input) (Output, error) {
	return a.Invoke(ctx, input)
}

// Stream 流式执行 Agent
func (a *BaseAgent) Stream(ctx context.Context, input Input, opts ...core.Option) (*stream.StreamReader[Output], error) {
	output, err := a.Invoke(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return stream.FromValue(output), nil
}

// Batch 批量执行 Agent
func (a *BaseAgent) Batch(ctx context.Context, inputs []Input, opts ...core.Option) ([]Output, error) {
	results := make([]Output, len(inputs))
	for i, input := range inputs {
		output, err := a.Invoke(ctx, input, opts...)
		if err != nil {
			return nil, err
		}
		results[i] = output
	}
	return results, nil
}

// Collect 收集流式输入并执行
func (a *BaseAgent) Collect(ctx context.Context, input *stream.StreamReader[Input], opts ...core.Option) (Output, error) {
	var zero Output
	// 收集所有输入
	collected, err := stream.Concat(ctx, input)
	if err != nil {
		return zero, err
	}
	return a.Invoke(ctx, collected, opts...)
}

// Transform 转换流
func (a *BaseAgent) Transform(ctx context.Context, input *stream.StreamReader[Input], opts ...core.Option) (*stream.StreamReader[Output], error) {
	reader, writer := stream.Pipe[Output](10)
	go func() {
		defer writer.Close()
		for {
			in, err := input.Recv()
			if err != nil {
				return
			}
			result, err := a.Invoke(ctx, in, opts...)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			writer.Send(result)
		}
	}()
	return reader, nil
}

// BatchStream 批量流式执行
func (a *BaseAgent) BatchStream(ctx context.Context, inputs []Input, opts ...core.Option) (*stream.StreamReader[Output], error) {
	results, err := a.Batch(ctx, inputs, opts...)
	if err != nil {
		return nil, err
	}
	return stream.FromSlice(results), nil
}

// InputSchema 返回输入 Schema
func (a *BaseAgent) InputSchema() *core.Schema {
	return core.SchemaOf[Input]()
}

// OutputSchema 返回输出 Schema
func (a *BaseAgent) OutputSchema() *core.Schema {
	return core.SchemaOf[Output]()
}

// generateID 生成唯一 ID
func generateID() string {
	return util.AgentID()
}
