// Package python 提供 AI Agent 的 Python 代码执行工具
//
// 本包实现了 Python REPL 工具，支持：
//   - Python 代码执行
//   - 虚拟环境支持
//   - 模块导入控制
//
// 安全特性：
//   - 模块白名单/黑名单
//   - 执行超时
//   - 输出大小限制
//
// 使用示例：
//
//	repl := python.New(python.DefaultConfig())
//	tool := repl.Tool()
//	result, _ := tool.Execute(ctx, map[string]any{
//	    "code": "print(1 + 1)",
//	})
package python

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/tool"
)

// Config Python REPL 工具配置
type Config struct {
	// PythonPath Python 解释器路径
	PythonPath string

	// Timeout 执行超时时间
	Timeout time.Duration

	// WorkDir 工作目录
	WorkDir string

	// MaxOutputSize 最大输出大小（字节）
	MaxOutputSize int

	// AllowedModules 允许导入的模块（空表示允许所有）
	AllowedModules []string

	// DeniedModules 禁止导入的模块
	DeniedModules []string

	// VirtualEnv 虚拟环境路径
	VirtualEnv string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	pythonPath := "python3"
	if runtime.GOOS == "windows" {
		pythonPath = "python"
	}

	return &Config{
		PythonPath:    pythonPath,
		Timeout:       60 * time.Second,
		MaxOutputSize: 1024 * 1024, // 1MB
		DeniedModules: []string{
			"subprocess",
			"os.system",
			"shutil.rmtree",
			"ctypes",
		},
	}
}

// Tool Python REPL 工具
type Tool struct {
	config *Config
}

// New 创建 Python REPL 工具
func New(config *Config) *Tool {
	if config == nil {
		config = DefaultConfig()
	}
	return &Tool{config: config}
}

// REPLTool 返回 Python REPL 工具
func (t *Tool) REPLTool() tool.Tool {
	return tool.NewFunc("python_repl", "执行 Python 代码", t.execute)
}

// ExecuteInput 执行代码输入
type ExecuteInput struct {
	Code    string            `json:"code" desc:"要执行的 Python 代码" required:"true"`
	Timeout int               `json:"timeout" desc:"超时时间（秒）"`
	Vars    map[string]string `json:"vars" desc:"预设变量"`
}

// ExecuteOutput 执行代码输出
type ExecuteOutput struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

// execute 执行 Python 代码
func (t *Tool) execute(ctx context.Context, input ExecuteInput) (*ExecuteOutput, error) {
	// 验证代码
	if err := t.validateCode(input.Code); err != nil {
		return nil, err
	}

	// 设置超时
	timeout := t.config.Timeout
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "hexagon_python_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// 构建完整代码
	fullCode := t.buildCode(input.Code, input.Vars)
	if _, err := tmpFile.WriteString(fullCode); err != nil {
		return nil, fmt.Errorf("failed to write code: %w", err)
	}
	tmpFile.Close()

	// 确定 Python 路径
	pythonPath := t.config.PythonPath
	if t.config.VirtualEnv != "" {
		if runtime.GOOS == "windows" {
			pythonPath = filepath.Join(t.config.VirtualEnv, "Scripts", "python.exe")
		} else {
			pythonPath = filepath.Join(t.config.VirtualEnv, "bin", "python")
		}
	}

	// 执行代码
	cmd := exec.CommandContext(ctx, pythonPath, tmpFile.Name())
	if t.config.WorkDir != "" {
		cmd.Dir = t.config.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	// 获取退出码
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("code execution timed out after %v", timeout)
		}
	}

	// 截断输出
	output := stdout.String()
	errOutput := stderr.String()
	if len(output) > t.config.MaxOutputSize {
		output = output[:t.config.MaxOutputSize] + "\n... (truncated)"
	}
	if len(errOutput) > t.config.MaxOutputSize {
		errOutput = errOutput[:t.config.MaxOutputSize] + "\n... (truncated)"
	}

	return &ExecuteOutput{
		Output:   output,
		Error:    errOutput,
		ExitCode: exitCode,
		Duration: duration.String(),
	}, nil
}

// validateCode 验证代码安全性
func (t *Tool) validateCode(code string) error {
	// 检查禁止的模块
	for _, module := range t.config.DeniedModules {
		patterns := []string{
			fmt.Sprintf("import %s", module),
			fmt.Sprintf("from %s import", module),
			fmt.Sprintf("__import__('%s')", module),
			fmt.Sprintf(`__import__("%s")`, module),
		}
		for _, pattern := range patterns {
			if strings.Contains(code, pattern) {
				return fmt.Errorf("importing '%s' is not allowed", module)
			}
		}
	}

	// 检查允许的模块（如果设置了白名单）
	if len(t.config.AllowedModules) > 0 {
		// 简单检查：查找所有 import 语句
		lines := strings.Split(code, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
				allowed := false
				for _, module := range t.config.AllowedModules {
					if strings.Contains(line, module) {
						allowed = true
						break
					}
				}
				if !allowed {
					return fmt.Errorf("import not allowed: %s", line)
				}
			}
		}
	}

	// 检查危险代码模式
	dangerousPatterns := []string{
		"eval(",
		"exec(",
		"compile(",
		"__builtins__",
		"__class__",
		"__subclasses__",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(code, pattern) {
			return fmt.Errorf("potentially dangerous code pattern detected: %s", pattern)
		}
	}

	return nil
}

// buildCode 构建完整代码
func (t *Tool) buildCode(code string, vars map[string]string) string {
	var builder strings.Builder

	// 添加预设变量
	if len(vars) > 0 {
		for k, v := range vars {
			// 简单的变量赋值（字符串类型）
			builder.WriteString(fmt.Sprintf("%s = %q\n", k, v))
		}
		builder.WriteString("\n")
	}

	// 添加用户代码
	builder.WriteString(code)

	return builder.String()
}

// ============== 便捷工具 ==============

// DataAnalysisTool 返回数据分析工具
func DataAnalysisTool() tool.Tool {
	config := DefaultConfig()
	config.AllowedModules = []string{
		"pandas",
		"numpy",
		"matplotlib",
		"seaborn",
		"json",
		"csv",
		"math",
		"statistics",
		"datetime",
	}

	t := &Tool{config: config}
	return tool.NewFunc("python_data_analysis", "使用 Python 进行数据分析", func(ctx context.Context, input struct {
		Code string `json:"code" desc:"数据分析代码" required:"true"`
	}) (*ExecuteOutput, error) {
		// 添加常用导入
		fullCode := `
import json
import math
import statistics
from datetime import datetime
try:
    import pandas as pd
    import numpy as np
except ImportError:
    pass
` + input.Code
		return t.execute(ctx, ExecuteInput{Code: fullCode})
	})
}

// CalculatorTool 返回计算器工具
func CalculatorTool() tool.Tool {
	config := DefaultConfig()
	config.Timeout = 10 * time.Second
	config.AllowedModules = []string{"math", "decimal", "fractions"}

	t := &Tool{config: config}
	return tool.NewFunc("python_calculator", "使用 Python 进行数学计算", func(ctx context.Context, input struct {
		Expression string `json:"expression" desc:"数学表达式" required:"true"`
	}) (*ExecuteOutput, error) {
		code := fmt.Sprintf(`
import math
from decimal import Decimal
from fractions import Fraction

result = %s
print(result)
`, input.Expression)
		return t.execute(ctx, ExecuteInput{Code: code})
	})
}

// JSONProcessorTool 返回 JSON 处理工具
func JSONProcessorTool() tool.Tool {
	config := DefaultConfig()
	config.Timeout = 30 * time.Second
	config.AllowedModules = []string{"json", "collections"}

	t := &Tool{config: config}
	return tool.NewFunc("python_json", "使用 Python 处理 JSON 数据", func(ctx context.Context, input struct {
		Code string `json:"code" desc:"JSON 处理代码" required:"true"`
		Data string `json:"data" desc:"输入的 JSON 数据"`
	}) (*ExecuteOutput, error) {
		fullCode := fmt.Sprintf(`
import json
from collections import defaultdict, Counter

# 输入数据
input_data = '''%s'''
if input_data.strip():
    data = json.loads(input_data)
else:
    data = None

# 用户代码
%s
`, strings.ReplaceAll(input.Data, "'", "\\'"), input.Code)
		return t.execute(ctx, ExecuteInput{Code: fullCode})
	})
}

// TextProcessorTool 返回文本处理工具
func TextProcessorTool() tool.Tool {
	config := DefaultConfig()
	config.Timeout = 30 * time.Second
	config.AllowedModules = []string{"re", "string", "textwrap", "unicodedata"}

	t := &Tool{config: config}
	return tool.NewFunc("python_text", "使用 Python 处理文本", func(ctx context.Context, input struct {
		Code string `json:"code" desc:"文本处理代码" required:"true"`
		Text string `json:"text" desc:"输入文本"`
	}) (*ExecuteOutput, error) {
		fullCode := fmt.Sprintf(`
import re
import string
import textwrap
import unicodedata

# 输入文本
text = '''%s'''

# 用户代码
%s
`, strings.ReplaceAll(input.Text, "'", "\\'"), input.Code)
		return t.execute(ctx, ExecuteInput{Code: fullCode})
	})
}
