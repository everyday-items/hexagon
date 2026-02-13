// Package main 演示 Hexagon 配置加载
//
// 配置系统支持：
//   - YAML 文件加载: Agent、团队、工作流配置
//   - 环境变量展开: ${VAR} 语法
//   - 配置验证: 自动检查必填字段
//
// 运行方式:
//
//	go run ./examples/config/
package main

import (
	"fmt"

	"github.com/everyday-items/hexagon/config"
)

func main() {
	fmt.Println("=== 示例 1: Agent 配置结构 ===")
	runAgentConfig()

	fmt.Println("\n=== 示例 2: 团队配置结构 ===")
	runTeamConfig()

	fmt.Println("\n=== 示例 3: 配置验证 ===")
	runConfigValidation()
}

// runAgentConfig 演示 Agent 配置结构
func runAgentConfig() {
	cfg := config.AgentConfig{
		Name:        "researcher",
		Description: "一个专业的研究助手",
		Type:        "react",
		Role: config.RoleConfig{
			Name:      "研究员",
			Title:     "高级研究分析师",
			Goal:      "高效地搜索和分析信息",
			Backstory: "拥有十年研究经验的分析师",
			Expertise: []string{"数据分析", "信息检索", "报告撰写"},
		},
		LLM: config.LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o",
			Temperature: 0.7,
		},
		Tools: []config.ToolConfig{
			{Name: "web_search", Type: "builtin"},
			{Name: "file_reader", Type: "builtin"},
		},
		Memory: config.MemoryConfig{
			Type:    "buffer",
			MaxSize: 100,
		},
		MaxIterations: 10,
		Verbose:       true,
	}

	fmt.Printf("  Agent: %s (%s)\n", cfg.Name, cfg.Description)
	fmt.Printf("  类型: %s\n", cfg.Type)
	fmt.Printf("  角色: %s - %s\n", cfg.Role.Name, cfg.Role.Goal)
	fmt.Printf("  LLM: %s/%s (temperature=%.1f)\n", cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.Temperature)
	fmt.Printf("  工具: %d 个\n", len(cfg.Tools))
	fmt.Printf("  记忆: %s (max=%d)\n", cfg.Memory.Type, cfg.Memory.MaxSize)
}

// runTeamConfig 演示团队配置结构
func runTeamConfig() {
	cfg := config.TeamConfig{
		Name:        "research-team",
		Description: "研究分析团队",
		Mode:        "sequential",
		Agents: []config.AgentConfig{
			{
				Name: "searcher",
				Type: "react",
				Role: config.RoleConfig{
					Name: "搜索专家",
					Goal: "快速找到相关信息",
				},
				LLM: config.LLMConfig{Provider: "openai", Model: "gpt-4o"},
			},
			{
				Name: "analyst",
				Type: "react",
				Role: config.RoleConfig{
					Name: "分析专家",
					Goal: "深入分析数据并得出结论",
				},
				LLM: config.LLMConfig{Provider: "deepseek", Model: "deepseek-chat"},
			},
			{
				Name: "writer",
				Type: "react",
				Role: config.RoleConfig{
					Name: "写作专家",
					Goal: "撰写清晰的研究报告",
				},
				LLM: config.LLMConfig{Provider: "anthropic", Model: "claude-sonnet-4-5-20250929"},
			},
		},
	}

	fmt.Printf("  团队: %s (%s)\n", cfg.Name, cfg.Description)
	fmt.Printf("  模式: %s\n", cfg.Mode)
	fmt.Printf("  成员:\n")
	for _, a := range cfg.Agents {
		fmt.Printf("    - %s (%s/%s) → %s\n", a.Name, a.LLM.Provider, a.LLM.Model, a.Role.Goal)
	}
}

// runConfigValidation 演示配置验证
func runConfigValidation() {
	// 有效配置
	valid := config.AgentConfig{
		Name: "assistant",
		Type: "react",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o",
		},
	}

	// 缺少名称的无效配置
	invalid := config.AgentConfig{
		Type: "react",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o",
		},
	}

	fmt.Printf("  有效配置 (%s): name=%q, type=%q\n", valid.Name, valid.Name, valid.Type)
	if invalid.Name == "" {
		fmt.Println("  无效配置: 缺少 name 字段")
	}
	if valid.LLM.Provider != "" && valid.LLM.Model != "" {
		fmt.Println("  LLM 配置验证通过")
	}
}
