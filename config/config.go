// Package config 提供 Hexagon AI Agent 框架的配置加载能力
//
// Config 支持从 YAML 文件加载 Agent、Workflow、Team 等配置。
//
// 主要配置类型：
//   - AgentConfig: Agent 配置，包括 LLM、工具、记忆等
//   - TeamConfig: 团队配置，定义多 Agent 协作
//   - WorkflowConfig: 工作流配置，定义图编排
//
// 特性：
//   - 支持环境变量展开：${VAR} 或 $VAR
//   - 配置验证：自动验证必填字段
//   - 热更新支持（计划中）
//
// 使用示例：
//
//	config, err := LoadAgentConfig("./agents/researcher.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AgentConfig Agent 配置
type AgentConfig struct {
	// Name Agent 名称
	Name string `yaml:"name" json:"name"`

	// Description Agent 描述
	Description string `yaml:"description" json:"description"`

	// Type Agent 类型: react, plan, custom
	Type string `yaml:"type" json:"type"`

	// Role 角色配置
	Role RoleConfig `yaml:"role" json:"role"`

	// LLM LLM 配置
	LLM LLMConfig `yaml:"llm" json:"llm"`

	// Tools 工具列表
	Tools []ToolConfig `yaml:"tools" json:"tools"`

	// Memory 记忆配置
	Memory MemoryConfig `yaml:"memory" json:"memory"`

	// MaxIterations 最大迭代次数
	MaxIterations int `yaml:"max_iterations" json:"max_iterations"`

	// Verbose 详细输出
	Verbose bool `yaml:"verbose" json:"verbose"`
}

// RoleConfig 角色配置
type RoleConfig struct {
	// Name 角色名称
	Name string `yaml:"name" json:"name"`

	// Title 角色头衔
	Title string `yaml:"title" json:"title"`

	// Goal 角色目标
	Goal string `yaml:"goal" json:"goal"`

	// Backstory 背景故事
	Backstory string `yaml:"backstory" json:"backstory"`

	// Expertise 专长领域
	Expertise []string `yaml:"expertise" json:"expertise"`

	// Personality 性格特点
	Personality string `yaml:"personality" json:"personality"`

	// Constraints 行为约束
	Constraints []string `yaml:"constraints" json:"constraints"`
}

// LLMConfig LLM 配置
type LLMConfig struct {
	// Provider 提供商: openai, deepseek, anthropic, etc.
	Provider string `yaml:"provider" json:"provider"`

	// Model 模型名称
	Model string `yaml:"model" json:"model"`

	// APIKey API 密钥（可从环境变量读取）
	APIKey string `yaml:"api_key" json:"api_key"`

	// BaseURL 自定义 API 地址
	BaseURL string `yaml:"base_url" json:"base_url"`

	// Temperature 温度参数
	Temperature float64 `yaml:"temperature" json:"temperature"`

	// MaxTokens 最大 Token 数
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
}

// ToolConfig 工具配置
type ToolConfig struct {
	// Name 工具名称
	Name string `yaml:"name" json:"name"`

	// Type 工具类型: builtin, mcp, custom
	Type string `yaml:"type" json:"type"`

	// Config 工具特定配置
	Config map[string]any `yaml:"config" json:"config"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	// Type 记忆类型: buffer, vector, summary
	Type string `yaml:"type" json:"type"`

	// MaxSize 最大大小
	MaxSize int `yaml:"max_size" json:"max_size"`

	// Config 特定配置
	Config map[string]any `yaml:"config" json:"config"`
}

// TeamConfig 团队配置
type TeamConfig struct {
	// Name 团队名称
	Name string `yaml:"name" json:"name"`

	// Description 团队描述
	Description string `yaml:"description" json:"description"`

	// Mode 工作模式: sequential, hierarchical, collaborative, round_robin
	Mode string `yaml:"mode" json:"mode"`

	// Manager 管理者 Agent（仅用于 hierarchical 模式）
	Manager string `yaml:"manager" json:"manager"`

	// Agents Agent 配置列表
	Agents []AgentConfig `yaml:"agents" json:"agents"`

	// MaxRounds 最大轮次
	MaxRounds int `yaml:"max_rounds" json:"max_rounds"`
}

// WorkflowConfig 工作流配置
type WorkflowConfig struct {
	// Name 工作流名称
	Name string `yaml:"name" json:"name"`

	// Description 工作流描述
	Description string `yaml:"description" json:"description"`

	// Nodes 节点列表
	Nodes []NodeConfig `yaml:"nodes" json:"nodes"`

	// Edges 边列表
	Edges []EdgeConfig `yaml:"edges" json:"edges"`

	// EntryPoint 入口点
	EntryPoint string `yaml:"entry_point" json:"entry_point"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	// Name 节点名称
	Name string `yaml:"name" json:"name"`

	// Type 节点类型: agent, tool, condition, parallel
	Type string `yaml:"type" json:"type"`

	// Agent Agent 配置（如果是 agent 类型）
	Agent *AgentConfig `yaml:"agent" json:"agent"`

	// AgentRef Agent 引用（引用已定义的 Agent）
	AgentRef string `yaml:"agent_ref" json:"agent_ref"`

	// Condition 条件表达式（如果是 condition 类型）
	Condition string `yaml:"condition" json:"condition"`

	// Parallel 并行节点列表（如果是 parallel 类型）
	Parallel []string `yaml:"parallel" json:"parallel"`
}

// EdgeConfig 边配置
type EdgeConfig struct {
	// From 源节点
	From string `yaml:"from" json:"from"`

	// To 目标节点
	To string `yaml:"to" json:"to"`

	// Condition 条件（可选）
	Condition string `yaml:"condition" json:"condition"`

	// Label 标签
	Label string `yaml:"label" json:"label"`
}

// LoadAgentConfig 从文件加载 Agent 配置
func LoadAgentConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 处理环境变量
	config.LLM.APIKey = expandEnv(config.LLM.APIKey)
	config.LLM.BaseURL = expandEnv(config.LLM.BaseURL)

	return &config, nil
}

// LoadTeamConfig 从文件加载 Team 配置
func LoadTeamConfig(path string) (*TeamConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config TeamConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// LoadWorkflowConfig 从文件加载 Workflow 配置
func LoadWorkflowConfig(path string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config WorkflowConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// expandEnv 展开环境变量
// 支持 ${VAR} 和 $VAR 格式
func expandEnv(s string) string {
	return os.ExpandEnv(s)
}

// SaveAgentConfig 保存 Agent 配置到文件
func SaveAgentConfig(path string, config *AgentConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate 验证配置
func (c *AgentConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if err := c.LLM.Validate(); err != nil {
		return fmt.Errorf("invalid LLM config: %w", err)
	}
	return nil
}

// Validate 验证 LLM 配置
func (c *LLMConfig) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("LLM provider is required")
	}
	if c.Model == "" {
		return fmt.Errorf("LLM model is required")
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("LLM temperature must be between 0 and 2")
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("LLM max_tokens must be non-negative")
	}
	return nil
}

// Validate 验证团队配置
func (c *TeamConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("team name is required")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("team must have at least one agent")
	}
	for i, agent := range c.Agents {
		if err := agent.Validate(); err != nil {
			return fmt.Errorf("invalid agent config at index %d: %w", i, err)
		}
	}
	return nil
}

// Validate 验证工作流配置
func (c *WorkflowConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(c.Nodes) == 0 {
		return fmt.Errorf("workflow must have at least one node")
	}
	return nil
}
