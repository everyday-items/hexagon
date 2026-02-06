package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  AgentConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: AgentConfig{
				Name: "test-agent",
				LLM:  LLMConfig{Provider: "openai", Model: "gpt-4"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: AgentConfig{
				LLM: LLMConfig{Provider: "openai", Model: "gpt-4"},
			},
			wantErr: true,
		},
		{
			name: "missing provider",
			config: AgentConfig{
				Name: "test-agent",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTeamConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TeamConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: TeamConfig{
				Name: "test-team",
				Agents: []AgentConfig{
					{Name: "agent-1", LLM: LLMConfig{Provider: "openai", Model: "gpt-4"}},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: TeamConfig{
				Agents: []AgentConfig{
					{Name: "agent-1"},
				},
			},
			wantErr: true,
		},
		{
			name: "no agents",
			config: TeamConfig{
				Name:   "test-team",
				Agents: []AgentConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkflowConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WorkflowConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: WorkflowConfig{
				Name: "test-workflow",
				Nodes: []NodeConfig{
					{Name: "step1", Type: "agent"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: WorkflowConfig{
				Nodes: []NodeConfig{
					{Name: "step1"},
				},
			},
			wantErr: true,
		},
		{
			name: "no nodes",
			config: WorkflowConfig{
				Name:  "test-workflow",
				Nodes: []NodeConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadAgentConfig(t *testing.T) {
	// 创建临时 YAML 文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	yamlContent := `
name: test-agent
description: A test agent
type: react
llm:
  provider: openai
  model: gpt-4
  api_key: ${OPENAI_API_KEY}
  temperature: 0.7
max_iterations: 10
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// 设置环境变量
	os.Setenv("OPENAI_API_KEY", "test-key-123")
	defer os.Unsetenv("OPENAI_API_KEY")

	// 加载配置
	config, err := LoadAgentConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// 验证配置
	if config.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", config.Name)
	}
	if config.LLM.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", config.LLM.Provider)
	}
	if config.LLM.APIKey != "test-key-123" {
		t.Errorf("expected api_key 'test-key-123', got '%s'", config.LLM.APIKey)
	}
	if config.MaxIterations != 10 {
		t.Errorf("expected max_iterations 10, got %d", config.MaxIterations)
	}
}

func TestLoadAgentConfig_FileNotFound(t *testing.T) {
	_, err := LoadAgentConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSaveAgentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	config := &AgentConfig{
		Name:        "saved-agent",
		Description: "A saved agent",
		Type:        "react",
		LLM: LLMConfig{
			Provider: "deepseek",
			Model:    "deepseek-chat",
		},
		MaxIterations: 5,
	}

	// 保存配置
	if err := SaveAgentConfig(configPath, config); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// 重新加载验证
	loaded, err := LoadAgentConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Name != config.Name {
		t.Errorf("expected name '%s', got '%s'", config.Name, loaded.Name)
	}
	if loaded.LLM.Provider != config.LLM.Provider {
		t.Errorf("expected provider '%s', got '%s'", config.LLM.Provider, loaded.LLM.Provider)
	}
}

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test-value")
	defer os.Unsetenv("TEST_VAR")

	tests := []struct {
		input    string
		expected string
	}{
		{"${TEST_VAR}", "test-value"},
		{"$TEST_VAR", "test-value"},
		{"prefix-${TEST_VAR}-suffix", "prefix-test-value-suffix"},
		{"no-var", "no-var"},
		{"${NONEXISTENT}", ""},
	}

	for _, tt := range tests {
		result := expandEnv(tt.input)
		if result != tt.expected {
			t.Errorf("expandEnv(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestRoleConfig(t *testing.T) {
	role := RoleConfig{
		Name:        "Researcher",
		Title:       "Senior Researcher",
		Goal:        "Find accurate information",
		Backstory:   "Expert in research",
		Expertise:   []string{"search", "analysis"},
		Personality: "thorough",
		Constraints: []string{"be accurate", "cite sources"},
	}

	if role.Name != "Researcher" {
		t.Errorf("expected name 'Researcher', got '%s'", role.Name)
	}
	if len(role.Expertise) != 2 {
		t.Errorf("expected 2 expertise items, got %d", len(role.Expertise))
	}
}

func TestMemoryConfig(t *testing.T) {
	memory := MemoryConfig{
		Type:    "buffer",
		MaxSize: 100,
		Config: map[string]any{
			"ttl": 3600,
		},
	}

	if memory.Type != "buffer" {
		t.Errorf("expected type 'buffer', got '%s'", memory.Type)
	}
	if memory.MaxSize != 100 {
		t.Errorf("expected max_size 100, got %d", memory.MaxSize)
	}
}
