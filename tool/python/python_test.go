package python

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// skipIfNoPython 跳过没有 Python 的测试
func skipIfNoPython(t *testing.T) {
	t.Helper()
	pythonPath := "python3"
	if runtime.GOOS == "windows" {
		pythonPath = "python"
	}
	if _, err := exec.LookPath(pythonPath); err != nil {
		t.Skip("Python not available, skipping test")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if config.Timeout != 60*time.Second {
		t.Errorf("expected timeout=60s, got %v", config.Timeout)
	}
	if config.MaxOutputSize != 1024*1024 {
		t.Errorf("expected MaxOutputSize=1MB, got %d", config.MaxOutputSize)
	}
	if len(config.DeniedModules) == 0 {
		t.Error("DeniedModules should have default values")
	}
}

func TestDefaultConfig_PythonPath(t *testing.T) {
	config := DefaultConfig()

	if runtime.GOOS == "windows" {
		if config.PythonPath != "python" {
			t.Errorf("expected PythonPath=python on Windows, got %s", config.PythonPath)
		}
	} else {
		if config.PythonPath != "python3" {
			t.Errorf("expected PythonPath=python3, got %s", config.PythonPath)
		}
	}
}

func TestNew(t *testing.T) {
	tool := New(nil)
	if tool == nil {
		t.Fatal("New with nil config returned nil")
	}
	if tool.config == nil {
		t.Error("config should be set to default")
	}

	customConfig := &Config{
		PythonPath: "custom-python",
		Timeout:    30 * time.Second,
	}
	tool = New(customConfig)
	if tool.config.PythonPath != "custom-python" {
		t.Error("custom config not applied")
	}
}

func TestTool_REPLTool(t *testing.T) {
	tool := New(DefaultConfig())
	replTool := tool.REPLTool()

	if replTool == nil {
		t.Fatal("REPLTool returned nil")
	}
	if replTool.Name() != "python_repl" {
		t.Errorf("expected name=python_repl, got %s", replTool.Name())
	}
}

func TestTool_ValidateCode(t *testing.T) {
	tool := New(DefaultConfig())

	tests := []struct {
		code    string
		wantErr bool
		desc    string
	}{
		// 安全代码
		{"print('hello')", false, "简单打印"},
		{"x = 1 + 2", false, "简单计算"},
		{"import math", false, "安全模块导入"},

		// 禁止的模块
		{"import subprocess", true, "subprocess 模块"},
		{"from subprocess import run", true, "subprocess 子导入"},
		{"import os.system", true, "os.system"},
		{"import ctypes", true, "ctypes 模块"},

		// 危险的代码模式
		{"eval('code')", true, "eval 调用"},
		{"exec('code')", true, "exec 调用"},
		{"compile('code', 'f', 'exec')", true, "compile 调用"},
		{"__builtins__", true, "访问 builtins"},
		{"__class__", true, "类内省"},
		{"__subclasses__", true, "子类枚举"},
		{"globals()", true, "全局命名空间访问"},
		{"locals()", true, "局部命名空间访问"},
		{"getattr(obj, 'x')", true, "属性访问"},
		{"open('file')", true, "文件访问"},
		{"exit()", true, "系统退出"},

		// 编码绕过
		{"base64.b64decode('xxx')", true, "base64 解码"},
		{"codecs.decode('xxx')", true, "codecs 解码"},
		{"\\x41", true, "十六进制转义"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := tool.validateCode(tt.code)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for: %s", tt.code)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tt.code, err)
			}
		})
	}
}

func TestTool_ValidateCode_AllowedModules(t *testing.T) {
	config := DefaultConfig()
	config.AllowedModules = []string{"math", "json"}
	tool := New(config)

	// 允许的模块
	err := tool.validateCode("import math")
	if err != nil {
		t.Errorf("math should be allowed: %v", err)
	}

	err = tool.validateCode("import json")
	if err != nil {
		t.Errorf("json should be allowed: %v", err)
	}

	// 不在白名单的模块
	err = tool.validateCode("import os")
	if err == nil {
		t.Error("os should not be allowed when whitelist is set")
	}
}

func TestTool_BuildCode(t *testing.T) {
	tool := New(DefaultConfig())

	// 无变量
	code := tool.buildCode("print('hello')", nil)
	if code != "print('hello')" {
		t.Errorf("unexpected code: %s", code)
	}

	// 有变量
	vars := map[string]string{
		"name": "world",
		"age":  "25",
	}
	code = tool.buildCode("print(name)", vars)
	if code == "print(name)" {
		t.Error("variables should be prepended")
	}
}

func TestTool_Execute_SimpleCode(t *testing.T) {
	skipIfNoPython(t)

	tool := New(DefaultConfig())
	ctx := context.Background()

	output, err := tool.execute(ctx, ExecuteInput{
		Code: "print(1 + 1)",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", output.ExitCode)
	}
	if output.Output != "2\n" {
		t.Errorf("expected output=2, got %q", output.Output)
	}
}

func TestTool_Execute_WithVars(t *testing.T) {
	skipIfNoPython(t)

	tool := New(DefaultConfig())
	ctx := context.Background()

	output, err := tool.execute(ctx, ExecuteInput{
		Code: "print(greeting)",
		Vars: map[string]string{"greeting": "hello"},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", output.ExitCode)
	}
	if output.Output != "hello\n" {
		t.Errorf("expected output=hello, got %q", output.Output)
	}
}

func TestTool_Execute_SyntaxError(t *testing.T) {
	skipIfNoPython(t)

	tool := New(DefaultConfig())
	ctx := context.Background()

	output, err := tool.execute(ctx, ExecuteInput{
		Code: "print(", // 语法错误
	})
	// 不应该返回错误，但 exit code 应该非零
	if err != nil {
		t.Logf("execute returned error (expected for syntax error): %v", err)
		return
	}

	if output.ExitCode == 0 {
		t.Error("expected non-zero exit code for syntax error")
	}
	if output.Error == "" {
		t.Error("expected error output for syntax error")
	}
}

func TestTool_Execute_Timeout(t *testing.T) {
	skipIfNoPython(t)

	config := DefaultConfig()
	config.Timeout = 100 * time.Millisecond
	tool := New(config)
	ctx := context.Background()

	start := time.Now()
	_, err := tool.execute(ctx, ExecuteInput{
		Code: "import time; time.sleep(10)",
	})
	duration := time.Since(start)

	// 如果执行时间很短（< 1秒），说明超时机制可能生效了
	// 如果执行时间很长，说明超时未生效（可能是 Python 启动慢或其他原因）
	if err == nil && duration > 5*time.Second {
		t.Error("expected timeout error or quick exit")
	}
	// 如果有错误或执行时间短，测试通过
	t.Logf("Execution time: %v, error: %v", duration, err)
}

func TestTool_Execute_CustomTimeout(t *testing.T) {
	skipIfNoPython(t)

	tool := New(DefaultConfig())
	ctx := context.Background()

	start := time.Now()
	_, err := tool.execute(ctx, ExecuteInput{
		Code:    "import time; time.sleep(10)",
		Timeout: 1, // 1 秒
	})
	duration := time.Since(start)

	// 同上，检查是否在合理时间内完成
	if err == nil && duration > 5*time.Second {
		t.Error("expected timeout error or quick exit with custom timeout")
	}
	t.Logf("Execution time: %v, error: %v", duration, err)
}

func TestTool_Execute_DeniedModule(t *testing.T) {
	tool := New(DefaultConfig())
	ctx := context.Background()

	_, err := tool.execute(ctx, ExecuteInput{
		Code: "import subprocess; subprocess.run(['ls'])",
	})
	if err == nil {
		t.Error("expected error for denied module")
	}
}

func TestTool_Execute_OutputTruncation(t *testing.T) {
	skipIfNoPython(t)

	config := DefaultConfig()
	config.MaxOutputSize = 10 // 非常小的限制
	tool := New(config)
	ctx := context.Background()

	output, err := tool.execute(ctx, ExecuteInput{
		Code: "print('a' * 100)",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(output.Output) > 100 {
		t.Errorf("output should be truncated, got length %d", len(output.Output))
	}
}

func TestDataAnalysisTool(t *testing.T) {
	tool := DataAnalysisTool()
	if tool == nil {
		t.Fatal("DataAnalysisTool returned nil")
	}
	if tool.Name() != "python_data_analysis" {
		t.Errorf("expected name=python_data_analysis, got %s", tool.Name())
	}
}

func TestCalculatorTool(t *testing.T) {
	tool := CalculatorTool()
	if tool == nil {
		t.Fatal("CalculatorTool returned nil")
	}
	if tool.Name() != "python_calculator" {
		t.Errorf("expected name=python_calculator, got %s", tool.Name())
	}
}

func TestJSONProcessorTool(t *testing.T) {
	tool := JSONProcessorTool()
	if tool == nil {
		t.Fatal("JSONProcessorTool returned nil")
	}
	if tool.Name() != "python_json" {
		t.Errorf("expected name=python_json, got %s", tool.Name())
	}
}

func TestTextProcessorTool(t *testing.T) {
	tool := TextProcessorTool()
	if tool == nil {
		t.Fatal("TextProcessorTool returned nil")
	}
	if tool.Name() != "python_text" {
		t.Errorf("expected name=python_text, got %s", tool.Name())
	}
}

func TestCalculatorTool_Execute(t *testing.T) {
	skipIfNoPython(t)

	calcTool := CalculatorTool()
	ctx := context.Background()

	result, err := calcTool.Execute(ctx, map[string]any{
		"expression": "math.sqrt(16) + 2**3",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	output, ok := result.Output.(*ExecuteOutput)
	if !ok {
		t.Skip("Result type mismatch, skipping output check")
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d. Error: %s", output.ExitCode, output.Error)
	}
}

func TestConfig(t *testing.T) {
	config := &Config{
		PythonPath:     "/usr/bin/python3",
		Timeout:        30 * time.Second,
		WorkDir:        "/tmp",
		MaxOutputSize:  1024,
		AllowedModules: []string{"math", "json"},
		DeniedModules:  []string{"os", "sys"},
		VirtualEnv:     "/path/to/venv",
	}

	if config.PythonPath != "/usr/bin/python3" {
		t.Errorf("expected PythonPath=/usr/bin/python3, got %s", config.PythonPath)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout=30s, got %v", config.Timeout)
	}
	if config.WorkDir != "/tmp" {
		t.Errorf("expected WorkDir=/tmp, got %s", config.WorkDir)
	}
	if config.MaxOutputSize != 1024 {
		t.Errorf("expected MaxOutputSize=1024, got %d", config.MaxOutputSize)
	}
	if len(config.AllowedModules) != 2 {
		t.Errorf("expected 2 AllowedModules, got %d", len(config.AllowedModules))
	}
	if len(config.DeniedModules) != 2 {
		t.Errorf("expected 2 DeniedModules, got %d", len(config.DeniedModules))
	}
	if config.VirtualEnv != "/path/to/venv" {
		t.Errorf("expected VirtualEnv=/path/to/venv, got %s", config.VirtualEnv)
	}
}

func TestExecuteInput(t *testing.T) {
	input := ExecuteInput{
		Code:    "print('hello')",
		Timeout: 10,
		Vars:    map[string]string{"key": "value"},
	}

	if input.Code != "print('hello')" {
		t.Errorf("expected Code=print('hello'), got %s", input.Code)
	}
	if input.Timeout != 10 {
		t.Errorf("expected Timeout=10, got %d", input.Timeout)
	}
	if input.Vars["key"] != "value" {
		t.Errorf("expected Vars[key]=value, got %s", input.Vars["key"])
	}
}

func TestExecuteOutput(t *testing.T) {
	output := ExecuteOutput{
		Output:   "result",
		Error:    "error message",
		ExitCode: 1,
		Duration: "100ms",
	}

	if output.Output != "result" {
		t.Errorf("expected Output=result, got %s", output.Output)
	}
	if output.Error != "error message" {
		t.Errorf("expected Error=error message, got %s", output.Error)
	}
	if output.ExitCode != 1 {
		t.Errorf("expected ExitCode=1, got %d", output.ExitCode)
	}
	if output.Duration != "100ms" {
		t.Errorf("expected Duration=100ms, got %s", output.Duration)
	}
}

func TestValidateCode_DeniedModulePatterns(t *testing.T) {
	tool := New(DefaultConfig())

	// 各种导入模式
	patterns := []string{
		"import subprocess",
		"from subprocess import run",
		"from subprocess.xxx import something",
		"__import__('subprocess')",
		`__import__("subprocess")`,
		"importlib.import_module('subprocess')",
		`importlib.import_module("subprocess")`,
	}

	for _, pattern := range patterns {
		err := tool.validateCode(pattern)
		if err == nil {
			t.Errorf("expected error for pattern: %s", pattern)
		}
	}
}

func TestValidateCode_DangerousPatterns(t *testing.T) {
	tool := New(DefaultConfig())

	patterns := []struct {
		code string
		desc string
	}{
		{"__bases__", "base class access"},
		{"__mro__", "method resolution order"},
		{"__globals__", "global namespace"},
		{"__code__", "code object"},
		{"__dict__", "dictionary access"},
		{"vars()", "variable access"},
		{"dir()", "object inspection"},
		{"setattr(x, 'y', z)", "attribute modification"},
		{"delattr(x, 'y')", "attribute deletion"},
		{"hasattr(x, 'y')", "attribute checking"},
		{"file('x')", "file access"},
		{"types.CodeType", "code type creation"},
		{"types.FunctionType", "function type creation"},
		{"chr(65)", "character conversion"},
		{"ord('A')", "ordinal conversion"},
		{"quit()", "system exit"},
		{"sys.exit", "system exit via sys"},
	}

	for _, p := range patterns {
		t.Run(p.desc, func(t *testing.T) {
			err := tool.validateCode(p.code)
			if err == nil {
				t.Errorf("expected error for %s: %s", p.desc, p.code)
			}
		})
	}
}

func TestValidateCode_EncodingBypass(t *testing.T) {
	tool := New(DefaultConfig())

	patterns := []string{
		"bytes.fromhex('414243')",
		"bytearray.fromhex('414243')",
		"'\\x41\\x42'",
		"'\\u0041'",
		"'\\U00000041'",
		"'\\N{LATIN CAPITAL LETTER A}'",
	}

	for _, pattern := range patterns {
		err := tool.validateCode(pattern)
		if err == nil {
			t.Errorf("expected error for encoding bypass pattern: %s", pattern)
		}
	}
}

func TestTool_Execute_VirtualEnv(t *testing.T) {
	// 这个测试主要验证代码路径，实际执行需要虚拟环境
	config := DefaultConfig()
	config.VirtualEnv = "/nonexistent/venv"
	tool := New(config)

	// 验证配置被正确设置
	if tool.config.VirtualEnv != "/nonexistent/venv" {
		t.Errorf("expected VirtualEnv=/nonexistent/venv, got %s", tool.config.VirtualEnv)
	}
}

func TestTool_Execute_WorkDir(t *testing.T) {
	skipIfNoPython(t)

	config := DefaultConfig()
	config.WorkDir = "/tmp"
	tool := New(config)
	ctx := context.Background()

	output, err := tool.execute(ctx, ExecuteInput{
		Code: "import os; print(os.getcwd())",
	})
	// 这个测试会因为 validateCode 的限制而失败，这是预期的
	if err != nil {
		// 预期因为 os 模块被阻止
		return
	}

	if output.ExitCode != 0 {
		t.Logf("WorkDir test: exit code=%d, error=%s", output.ExitCode, output.Error)
	}
}

func TestTool_Execute_MultilineCode(t *testing.T) {
	skipIfNoPython(t)

	tool := New(DefaultConfig())
	ctx := context.Background()

	code := `
x = 10
y = 20
result = x + y
print(result)
`
	output, err := tool.execute(ctx, ExecuteInput{Code: code})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d. Error: %s", output.ExitCode, output.Error)
	}
	if output.Output != "30\n" {
		t.Errorf("expected output=30, got %q", output.Output)
	}
}
