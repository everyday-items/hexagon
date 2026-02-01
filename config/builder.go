package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/llm/deepseek"
	"github.com/everyday-items/ai-core/llm/openai"
	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/agent"
	"github.com/everyday-items/hexagon/orchestration/workflow"
	"gopkg.in/yaml.v3"
)

// Builder 配置构建器
type Builder struct {
	// Registry 组件注册表
	registry *Registry

	// Providers LLM Provider 工厂
	providerFactory ProviderFactory

	// ToolFactory 工具工厂
	toolFactory ToolFactory

	// Config 构建配置
	config BuilderConfig
}

// BuilderConfig 构建器配置
type BuilderConfig struct {
	// BaseDir 基础目录
	BaseDir string

	// EnvPrefix 环境变量前缀
	EnvPrefix string

	// Strict 严格模式（验证所有引用）
	Strict bool

	// DefaultProvider 默认 LLM Provider
	DefaultProvider string

	// DefaultModel 默认模型
	DefaultModel string
}

// DefaultBuilderConfig 默认构建器配置
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		BaseDir:         ".",
		EnvPrefix:       "HEXAGON_",
		Strict:          true,
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
	}
}

// BuilderOption 构建器选项
type BuilderOption func(*BuilderConfig)

// NewBuilder 创建构建器
func NewBuilder(opts ...BuilderOption) *Builder {
	config := DefaultBuilderConfig()
	for _, opt := range opts {
		opt(&config)
	}

	return &Builder{
		registry:        NewRegistry(),
		providerFactory: DefaultProviderFactory(),
		toolFactory:     DefaultToolFactory(),
		config:          config,
	}
}

// WithBaseDir 设置基础目录
func WithBaseDir(dir string) BuilderOption {
	return func(c *BuilderConfig) {
		c.BaseDir = dir
	}
}

// WithEnvPrefix 设置环境变量前缀
func WithEnvPrefix(prefix string) BuilderOption {
	return func(c *BuilderConfig) {
		c.EnvPrefix = prefix
	}
}

// WithStrict 设置严格模式
func WithStrict(strict bool) BuilderOption {
	return func(c *BuilderConfig) {
		c.Strict = strict
	}
}

// SetProviderFactory 设置 Provider 工厂
func (b *Builder) SetProviderFactory(factory ProviderFactory) {
	b.providerFactory = factory
}

// SetToolFactory 设置工具工厂
func (b *Builder) SetToolFactory(factory ToolFactory) {
	b.toolFactory = factory
}

// Registry 返回注册表
func (b *Builder) Registry() *Registry {
	return b.registry
}

// ============== Build Methods ==============

// BuildAgent 从配置构建 Agent
func (b *Builder) BuildAgent(config *AgentConfig) (agent.Agent, error) {
	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// 创建 LLM Provider
	provider, err := b.buildProvider(&config.LLM)
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}

	// 创建工具
	tools, err := b.buildTools(config.Tools)
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}

	// 创建记忆
	mem, err := b.buildMemory(&config.Memory)
	if err != nil {
		return nil, fmt.Errorf("build memory: %w", err)
	}

	// 创建角色
	role := b.buildRole(&config.Role)

	// 创建 Agent
	maxIterations := config.MaxIterations
	if maxIterations == 0 {
		maxIterations = 10
	}

	var builtAgent agent.Agent

	switch config.Type {
	case "react", "":
		builtAgent = agent.NewReAct(
			agent.WithName(config.Name),
			agent.WithDescription(config.Description),
			agent.WithLLM(provider),
			agent.WithTools(tools...),
			agent.WithMemory(mem),
			agent.WithRole(role),
			agent.WithMaxIterations(maxIterations),
			agent.WithVerbose(config.Verbose),
		)
	default:
		return nil, fmt.Errorf("unsupported agent type: %s", config.Type)
	}

	// 注册到 Registry
	b.registry.RegisterAgent(config.Name, builtAgent)

	return builtAgent, nil
}

// BuildAgentFromFile 从文件构建 Agent
func (b *Builder) BuildAgentFromFile(path string) (agent.Agent, error) {
	config, err := LoadAgentConfig(filepath.Join(b.config.BaseDir, path))
	if err != nil {
		return nil, err
	}
	return b.BuildAgent(config)
}

// BuildTeam 从配置构建 Team
func (b *Builder) BuildTeam(config *TeamConfig) (*agent.Team, error) {
	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// 构建所有 Agent
	agents := make([]agent.Agent, 0, len(config.Agents))
	for _, agentConfig := range config.Agents {
		builtAgent, err := b.BuildAgent(&agentConfig)
		if err != nil {
			return nil, fmt.Errorf("build agent %s: %w", agentConfig.Name, err)
		}
		agents = append(agents, builtAgent)
	}

	// 解析工作模式
	mode := b.parseTeamMode(config.Mode)

	// 创建选项
	teamOpts := []agent.TeamOption{
		agent.WithAgents(agents...),
		agent.WithMode(mode),
		agent.WithTeamDescription(config.Description),
	}

	if config.MaxRounds > 0 {
		teamOpts = append(teamOpts, agent.WithMaxRounds(config.MaxRounds))
	}

	// 处理 Manager
	if config.Manager != "" && mode == agent.TeamModeHierarchical {
		manager, ok := b.registry.GetAgent(config.Manager)
		if !ok {
			return nil, fmt.Errorf("manager agent %s not found", config.Manager)
		}
		teamOpts = append(teamOpts, agent.WithManager(manager))
	}

	// 创建 Team
	team := agent.NewTeam(config.Name, teamOpts...)

	// 注册到 Registry
	b.registry.RegisterTeam(config.Name, team)

	return team, nil
}

// BuildTeamFromFile 从文件构建 Team
func (b *Builder) BuildTeamFromFile(path string) (*agent.Team, error) {
	config, err := LoadTeamConfig(filepath.Join(b.config.BaseDir, path))
	if err != nil {
		return nil, err
	}
	return b.BuildTeam(config)
}

// BuildWorkflow 从配置构建 Workflow
func (b *Builder) BuildWorkflow(config *WorkflowConfig) (*workflow.Workflow, error) {
	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// 创建 workflow builder
	builder := workflow.New(config.Name).
		WithDescription(config.Description)

	// 构建节点
	nodeMap := make(map[string]workflow.Step)
	for _, nodeConfig := range config.Nodes {
		step, err := b.buildWorkflowNode(&nodeConfig)
		if err != nil {
			return nil, fmt.Errorf("build node %s: %w", nodeConfig.Name, err)
		}
		nodeMap[nodeConfig.Name] = step
	}

	// 根据入口点和边构建顺序
	visitedNodes := make(map[string]bool)
	var orderedSteps []workflow.Step

	// 简单实现：按配置顺序添加节点
	for _, nodeConfig := range config.Nodes {
		if step, ok := nodeMap[nodeConfig.Name]; ok && !visitedNodes[nodeConfig.Name] {
			orderedSteps = append(orderedSteps, step)
			visitedNodes[nodeConfig.Name] = true
		}
	}

	// 添加步骤
	for _, step := range orderedSteps {
		builder.Add(step)
	}

	// 构建 workflow
	wf, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build workflow: %w", err)
	}

	// 注册到 Registry
	b.registry.RegisterWorkflow(config.Name, wf)

	return wf, nil
}

// BuildWorkflowFromFile 从文件构建 Workflow
func (b *Builder) BuildWorkflowFromFile(path string) (*workflow.Workflow, error) {
	config, err := LoadWorkflowConfig(filepath.Join(b.config.BaseDir, path))
	if err != nil {
		return nil, err
	}
	return b.BuildWorkflow(config)
}

// ============== Helper Methods ==============

// buildProvider 构建 LLM Provider
func (b *Builder) buildProvider(config *LLMConfig) (llm.Provider, error) {
	provider := config.Provider
	if provider == "" {
		provider = b.config.DefaultProvider
	}

	model := config.Model
	if model == "" {
		model = b.config.DefaultModel
	}

	apiKey := expandEnv(config.APIKey)
	baseURL := expandEnv(config.BaseURL)

	return b.providerFactory.Create(provider, ProviderOptions{
		Model:       model,
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Temperature: config.Temperature,
		MaxTokens:   config.MaxTokens,
	})
}

// buildTools 构建工具列表
func (b *Builder) buildTools(configs []ToolConfig) ([]tool.Tool, error) {
	tools := make([]tool.Tool, 0, len(configs))

	for _, config := range configs {
		t, err := b.toolFactory.Create(config.Name, config.Type, config.Config)
		if err != nil {
			return nil, fmt.Errorf("create tool %s: %w", config.Name, err)
		}
		if t != nil {
			tools = append(tools, t)
		}
	}

	return tools, nil
}

// buildMemory 构建记忆
func (b *Builder) buildMemory(config *MemoryConfig) (memory.Memory, error) {
	memType := config.Type
	if memType == "" {
		memType = "buffer"
	}

	maxSize := config.MaxSize
	if maxSize == 0 {
		maxSize = 100
	}

	switch memType {
	case "buffer":
		return memory.NewBuffer(maxSize), nil
	case "summary":
		// 需要 Summarizer，这里返回基础 Buffer
		return memory.NewBuffer(maxSize), nil
	case "vector":
		// 需要 Embedder，这里返回基础 Buffer
		return memory.NewBuffer(maxSize), nil
	default:
		return memory.NewBuffer(maxSize), nil
	}
}

// buildRole 构建角色
func (b *Builder) buildRole(config *RoleConfig) agent.Role {
	if config.Name == "" {
		return agent.Role{}
	}

	return agent.NewRole(config.Name).
		Title(config.Title).
		Goal(config.Goal).
		Backstory(config.Backstory).
		Expertise(config.Expertise...).
		Personality(config.Personality).
		Constraints(config.Constraints...).
		Build()
}

// buildWorkflowNode 构建工作流节点
func (b *Builder) buildWorkflowNode(config *NodeConfig) (workflow.Step, error) {
	switch config.Type {
	case "agent":
		// 使用 AgentRef 或内联 Agent
		var builtAgent agent.Agent
		var err error

		if config.AgentRef != "" {
			var ok bool
			builtAgent, ok = b.registry.GetAgent(config.AgentRef)
			if !ok {
				return nil, fmt.Errorf("agent %s not found", config.AgentRef)
			}
		} else if config.Agent != nil {
			builtAgent, err = b.BuildAgent(config.Agent)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("agent node requires agent or agent_ref")
		}

		return workflow.NewStep(config.Name, config.Name, func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			// 将 workflow input 转换为 agent input
			query := ""
			if s, ok := input.Data.(string); ok {
				query = s
			} else if m, ok := input.Data.(map[string]any); ok {
				if q, ok := m["query"].(string); ok {
					query = q
				}
			}

			output, err := builtAgent.Run(ctx, agent.Input{
				Query:   query,
				Context: input.Variables,
			})
			if err != nil {
				return nil, err
			}

			return &workflow.StepOutput{
				Data: output.Content,
				Variables: map[string]any{
					"agent_output": output,
				},
			}, nil
		}), nil

	case "condition":
		// 条件节点
		return workflow.NewStep(config.Name, config.Name, func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			// 简单条件评估
			result := b.evaluateCondition(config.Condition, input)
			return &workflow.StepOutput{
				Data: result,
			}, nil
		}), nil

	case "parallel":
		// 并行节点
		steps := make([]workflow.Step, 0)
		for _, nodeName := range config.Parallel {
			if step, ok := b.registry.GetWorkflowStep(nodeName); ok {
				steps = append(steps, step)
			}
		}
		return workflow.NewParallelStep(config.Name, config.Name, steps), nil

	default:
		// 默认为函数节点
		return workflow.NewStep(config.Name, config.Name, func(ctx context.Context, input workflow.StepInput) (*workflow.StepOutput, error) {
			return &workflow.StepOutput{Data: input.Data}, nil
		}), nil
	}
}

// parseTeamMode 解析团队模式
func (b *Builder) parseTeamMode(mode string) agent.TeamMode {
	switch strings.ToLower(mode) {
	case "sequential":
		return agent.TeamModeSequential
	case "hierarchical":
		return agent.TeamModeHierarchical
	case "collaborative":
		return agent.TeamModeCollaborative
	case "round_robin":
		return agent.TeamModeRoundRobin
	default:
		return agent.TeamModeSequential
	}
}

// evaluateCondition 评估条件
func (b *Builder) evaluateCondition(condition string, input workflow.StepInput) bool {
	// 简单实现：检查变量是否存在且为真
	if strings.Contains(condition, "==") {
		parts := strings.Split(condition, "==")
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			expectedValue := strings.TrimSpace(parts[1])
			if val, ok := input.Variables[varName]; ok {
				return fmt.Sprintf("%v", val) == expectedValue
			}
		}
	}
	return false
}

// ============== Registry ==============

// Registry 组件注册表
type Registry struct {
	agents        map[string]agent.Agent
	teams         map[string]*agent.Team
	workflows     map[string]*workflow.Workflow
	workflowSteps map[string]workflow.Step
	mu            sync.RWMutex
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		agents:        make(map[string]agent.Agent),
		teams:         make(map[string]*agent.Team),
		workflows:     make(map[string]*workflow.Workflow),
		workflowSteps: make(map[string]workflow.Step),
	}
}

// RegisterAgent 注册 Agent
func (r *Registry) RegisterAgent(name string, a agent.Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = a
}

// GetAgent 获取 Agent
func (r *Registry) GetAgent(name string) (agent.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

// RegisterTeam 注册 Team
func (r *Registry) RegisterTeam(name string, t *agent.Team) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[name] = t
}

// GetTeam 获取 Team
func (r *Registry) GetTeam(name string) (*agent.Team, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.teams[name]
	return t, ok
}

// RegisterWorkflow 注册 Workflow
func (r *Registry) RegisterWorkflow(name string, w *workflow.Workflow) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflows[name] = w
}

// GetWorkflow 获取 Workflow
func (r *Registry) GetWorkflow(name string) (*workflow.Workflow, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workflows[name]
	return w, ok
}

// RegisterWorkflowStep 注册 Workflow 步骤
func (r *Registry) RegisterWorkflowStep(name string, s workflow.Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflowSteps[name] = s
}

// GetWorkflowStep 获取 Workflow 步骤
func (r *Registry) GetWorkflowStep(name string) (workflow.Step, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.workflowSteps[name]
	return s, ok
}

// ============== Provider Factory ==============

// ProviderFactory Provider 工厂接口
type ProviderFactory interface {
	Create(provider string, opts ProviderOptions) (llm.Provider, error)
}

// ProviderOptions Provider 选项
type ProviderOptions struct {
	Model       string
	APIKey      string
	BaseURL     string
	Temperature float64
	MaxTokens   int
}

// DefaultProviderFactory 默认 Provider 工厂
type defaultProviderFactory struct{}

// DefaultProviderFactory 返回默认工厂
func DefaultProviderFactory() ProviderFactory {
	return &defaultProviderFactory{}
}

// Create 创建 Provider
func (f *defaultProviderFactory) Create(provider string, opts ProviderOptions) (llm.Provider, error) {
	apiKey := opts.APIKey
	if apiKey == "" {
		// 尝试从环境变量获取
		switch provider {
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "deepseek":
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
	}

	switch strings.ToLower(provider) {
	case "openai":
		providerOpts := []openai.Option{}
		if opts.BaseURL != "" {
			providerOpts = append(providerOpts, openai.WithBaseURL(opts.BaseURL))
		}
		if opts.Model != "" {
			providerOpts = append(providerOpts, openai.WithModel(opts.Model))
		}
		return openai.New(apiKey, providerOpts...), nil

	case "deepseek":
		// DeepSeek 使用 OpenAI 兼容 API
		return deepseek.New(apiKey), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ============== Tool Factory ==============

// ToolFactory 工具工厂接口
type ToolFactory interface {
	Create(name, toolType string, config map[string]any) (tool.Tool, error)
}

// DefaultToolFactory 默认工具工厂
type defaultToolFactory struct {
	builtins map[string]tool.Tool
}

// DefaultToolFactory 返回默认工厂
func DefaultToolFactory() ToolFactory {
	return &defaultToolFactory{
		builtins: make(map[string]tool.Tool),
	}
}

// Create 创建工具
func (f *defaultToolFactory) Create(name, toolType string, config map[string]any) (tool.Tool, error) {
	switch toolType {
	case "builtin":
		if t, ok := f.builtins[name]; ok {
			return t, nil
		}
		return nil, nil // 内置工具未找到，跳过

	case "mcp":
		// MCP 工具需要额外配置
		return nil, nil

	case "custom":
		// 自定义工具需要额外实现
		return nil, nil

	default:
		return nil, nil
	}
}

// ============== Config Watcher ==============

// ConfigWatcher 配置监视器
type ConfigWatcher struct {
	// Path 监视路径
	path string

	// Builder 构建器
	builder *Builder

	// Callback 回调函数
	callback func(config any, err error)

	// Interval 检查间隔
	interval int64

	// Running 运行状态
	running bool

	mu sync.RWMutex
}

// NewConfigWatcher 创建配置监视器
func NewConfigWatcher(path string, builder *Builder) *ConfigWatcher {
	return &ConfigWatcher{
		path:     path,
		builder:  builder,
		interval: 5, // 5 秒
	}
}

// OnChange 设置变更回调
func (w *ConfigWatcher) OnChange(callback func(config any, err error)) {
	w.callback = callback
}

// ============== Validation ==============

// Validator 配置验证器
type Validator struct {
	errors []string
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{
		errors: make([]string, 0),
	}
}

// ValidateAgentConfig 验证 Agent 配置
func (v *Validator) ValidateAgentConfig(config *AgentConfig) error {
	v.errors = v.errors[:0]

	if config.Name == "" {
		v.errors = append(v.errors, "agent name is required")
	}

	if config.LLM.Provider == "" {
		v.errors = append(v.errors, "LLM provider is required")
	}

	// 验证 LLM 配置
	if config.LLM.Provider != "" {
		validProviders := map[string]bool{
			"openai": true, "deepseek": true, "anthropic": true,
			"gemini": true, "qwen": true, "ollama": true,
		}
		if !validProviders[strings.ToLower(config.LLM.Provider)] {
			v.errors = append(v.errors, fmt.Sprintf("unknown provider: %s", config.LLM.Provider))
		}
	}

	// 验证工具配置
	for i, tool := range config.Tools {
		if tool.Name == "" {
			v.errors = append(v.errors, fmt.Sprintf("tool[%d].name is required", i))
		}
	}

	if len(v.errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(v.errors, "; "))
	}

	return nil
}

// ValidateTeamConfig 验证 Team 配置
func (v *Validator) ValidateTeamConfig(config *TeamConfig) error {
	v.errors = v.errors[:0]

	if config.Name == "" {
		v.errors = append(v.errors, "team name is required")
	}

	if len(config.Agents) == 0 {
		v.errors = append(v.errors, "team must have at least one agent")
	}

	// 验证模式
	validModes := map[string]bool{
		"sequential": true, "hierarchical": true,
		"collaborative": true, "round_robin": true,
	}
	if config.Mode != "" && !validModes[strings.ToLower(config.Mode)] {
		v.errors = append(v.errors, fmt.Sprintf("unknown mode: %s", config.Mode))
	}

	// 验证 hierarchical 模式需要 manager
	if strings.ToLower(config.Mode) == "hierarchical" && config.Manager == "" {
		v.errors = append(v.errors, "hierarchical mode requires a manager")
	}

	// 验证每个 Agent
	for i, agentConfig := range config.Agents {
		if err := v.ValidateAgentConfig(&agentConfig); err != nil {
			v.errors = append(v.errors, fmt.Sprintf("agent[%d]: %v", i, err))
		}
	}

	if len(v.errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(v.errors, "; "))
	}

	return nil
}

// ValidateWorkflowConfig 验证 Workflow 配置
func (v *Validator) ValidateWorkflowConfig(config *WorkflowConfig) error {
	v.errors = v.errors[:0]

	if config.Name == "" {
		v.errors = append(v.errors, "workflow name is required")
	}

	if len(config.Nodes) == 0 {
		v.errors = append(v.errors, "workflow must have at least one node")
	}

	// 验证节点引用
	nodeNames := make(map[string]bool)
	for _, node := range config.Nodes {
		if node.Name == "" {
			v.errors = append(v.errors, "node name is required")
			continue
		}
		nodeNames[node.Name] = true
	}

	// 验证边
	for i, edge := range config.Edges {
		if edge.From == "" || edge.To == "" {
			v.errors = append(v.errors, fmt.Sprintf("edge[%d]: from and to are required", i))
			continue
		}
		if !nodeNames[edge.From] {
			v.errors = append(v.errors, fmt.Sprintf("edge[%d]: unknown node %s", i, edge.From))
		}
		if !nodeNames[edge.To] {
			v.errors = append(v.errors, fmt.Sprintf("edge[%d]: unknown node %s", i, edge.To))
		}
	}

	// 验证入口点
	if config.EntryPoint != "" && !nodeNames[config.EntryPoint] {
		v.errors = append(v.errors, fmt.Sprintf("unknown entry point: %s", config.EntryPoint))
	}

	if len(v.errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(v.errors, "; "))
	}

	return nil
}

// ============== Config Merger ==============

// MergeAgentConfigs 合并 Agent 配置
func MergeAgentConfigs(base, override *AgentConfig) *AgentConfig {
	result := *base

	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Description != "" {
		result.Description = override.Description
	}
	if override.Type != "" {
		result.Type = override.Type
	}
	if override.LLM.Provider != "" {
		result.LLM.Provider = override.LLM.Provider
	}
	if override.LLM.Model != "" {
		result.LLM.Model = override.LLM.Model
	}
	if override.LLM.APIKey != "" {
		result.LLM.APIKey = override.LLM.APIKey
	}
	if override.MaxIterations != 0 {
		result.MaxIterations = override.MaxIterations
	}
	if len(override.Tools) > 0 {
		result.Tools = append(result.Tools, override.Tools...)
	}

	return &result
}

// ============== Load Multiple Configs ==============

// LoadConfigs 加载目录下的所有配置
func LoadConfigs(dir string) (*MultiConfig, error) {
	multi := &MultiConfig{
		Agents:    make([]*AgentConfig, 0),
		Teams:     make([]*TeamConfig, 0),
		Workflows: make([]*WorkflowConfig, 0),
	}

	// 查找所有 YAML 文件
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}

	yamlFiles, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	files = append(files, yamlFiles...)

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		// 尝试解析为不同类型
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}

		// 根据字段判断类型
		if _, ok := doc["agents"]; ok {
			var teamConfig TeamConfig
			if yaml.Unmarshal(data, &teamConfig) == nil {
				multi.Teams = append(multi.Teams, &teamConfig)
			}
		} else if _, ok := doc["nodes"]; ok {
			var workflowConfig WorkflowConfig
			if yaml.Unmarshal(data, &workflowConfig) == nil {
				multi.Workflows = append(multi.Workflows, &workflowConfig)
			}
		} else if _, ok := doc["llm"]; ok {
			var agentConfig AgentConfig
			if yaml.Unmarshal(data, &agentConfig) == nil {
				multi.Agents = append(multi.Agents, &agentConfig)
			}
		}
	}

	return multi, nil
}

// MultiConfig 多配置
type MultiConfig struct {
	Agents    []*AgentConfig
	Teams     []*TeamConfig
	Workflows []*WorkflowConfig
}
