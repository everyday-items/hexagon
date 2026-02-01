// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/everyday-items/ai-core/schema"
	"github.com/everyday-items/ai-core/tool"
)

// Tool Mock Tool 实现
type Tool struct {
	name        string
	description string
	toolSchema  *schema.Schema
	results     []tool.Result
	current     int
	calls       []map[string]any
	mu          sync.Mutex

	// 自定义执行函数
	executeFn func(ctx context.Context, args map[string]any) (tool.Result, error)
}

// ToolOption Tool 选项
type ToolOption func(*Tool)

// WithToolDescription 设置描述
func WithToolDescription(desc string) ToolOption {
	return func(t *Tool) {
		t.description = desc
	}
}

// WithToolSchema 设置 Schema
func WithToolSchema(s *schema.Schema) ToolOption {
	return func(t *Tool) {
		t.toolSchema = s
	}
}

// WithToolExecuteFn 设置执行函数
func WithToolExecuteFn(fn func(ctx context.Context, args map[string]any) (tool.Result, error)) ToolOption {
	return func(t *Tool) {
		t.executeFn = fn
	}
}

// NewTool 创建 Mock Tool
func NewTool(name string, opts ...ToolOption) *Tool {
	t := &Tool{
		name:        name,
		description: "Mock tool for testing",
		results:     make([]tool.Result, 0),
		calls:       make([]map[string]any, 0),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Name 返回工具名称
func (t *Tool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *Tool) Description() string {
	return t.description
}

// Schema 返回工具 Schema
func (t *Tool) Schema() *schema.Schema {
	if t.toolSchema != nil {
		return t.toolSchema
	}
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Schema{
			"input": {
				Type:        "string",
				Description: "Input parameter",
			},
		},
	}
}

// Execute 执行工具
func (t *Tool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 记录调用
	argsCopy := make(map[string]any)
	for k, v := range args {
		argsCopy[k] = v
	}
	t.calls = append(t.calls, argsCopy)

	// 检查 context
	if ctx.Err() != nil {
		return tool.Result{}, ctx.Err()
	}

	// 使用自定义执行函数
	if t.executeFn != nil {
		return t.executeFn(ctx, args)
	}

	// 使用预定义结果
	if t.current >= len(t.results) {
		return tool.NewResult(fmt.Sprintf("Mock result for %s", t.name)), nil
	}

	result := t.results[t.current]
	t.current++
	return result, nil
}

// Validate 验证参数
func (t *Tool) Validate(args map[string]any) error {
	// 简单验证：不做任何检查
	return nil
}

// AddResult 添加预定义结果
func (t *Tool) AddResult(data any) *Tool {
	t.results = append(t.results, tool.NewResult(data))
	return t
}

// AddErrorResult 添加错误结果
func (t *Tool) AddErrorResult(err error) *Tool {
	t.results = append(t.results, tool.NewErrorResult(err))
	return t
}

// Calls 返回所有调用记录
func (t *Tool) Calls() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	calls := make([]map[string]any, len(t.calls))
	copy(calls, t.calls)
	return calls
}

// LastCall 返回最后一次调用
func (t *Tool) LastCall() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.calls) == 0 {
		return nil
	}
	return t.calls[len(t.calls)-1]
}

// CallCount 返回调用次数
func (t *Tool) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

// Reset 重置状态
func (t *Tool) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = 0
	t.calls = make([]map[string]any, 0)
}

var _ tool.Tool = (*Tool)(nil)

// ============== 工具函数 ==============

// EchoTool 创建回声工具（返回输入作为输出）
func EchoTool() *Tool {
	return NewTool("echo", WithToolExecuteFn(func(ctx context.Context, args map[string]any) (tool.Result, error) {
		return tool.NewResult(args), nil
	}))
}

// FixedTool 创建固定结果工具
func FixedTool(name string, result any) *Tool {
	return NewTool(name, WithToolExecuteFn(func(ctx context.Context, args map[string]any) (tool.Result, error) {
		return tool.NewResult(result), nil
	}))
}

// ErrorTool 创建总是返回错误的工具
func ErrorTool(name string, err error) *Tool {
	return NewTool(name, WithToolExecuteFn(func(ctx context.Context, args map[string]any) (tool.Result, error) {
		return tool.Result{}, err
	}))
}

// CalculatorTool 创建简单计算器工具
func CalculatorTool() *Tool {
	return NewTool("calculator",
		WithToolDescription("A simple calculator"),
		WithToolSchema(&schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Schema{
				"operation": {
					Type:        "string",
					Description: "The operation to perform (add, subtract, multiply, divide)",
				},
				"a": {
					Type:        "number",
					Description: "First operand",
				},
				"b": {
					Type:        "number",
					Description: "Second operand",
				},
			},
			Required: []string{"operation", "a", "b"},
		}),
		WithToolExecuteFn(func(ctx context.Context, args map[string]any) (tool.Result, error) {
			op, _ := args["operation"].(string)
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)

			var result float64
			switch op {
			case "add":
				result = a + b
			case "subtract":
				result = a - b
			case "multiply":
				result = a * b
			case "divide":
				if b == 0 {
					return tool.Result{}, fmt.Errorf("division by zero")
				}
				result = a / b
			default:
				return tool.Result{}, fmt.Errorf("unknown operation: %s", op)
			}

			return tool.NewResult(result), nil
		}),
	)
}

// SearchTool 创建模拟搜索工具
func SearchTool(results []string) *Tool {
	return NewTool("search",
		WithToolDescription("Search for information"),
		WithToolSchema(&schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Schema{
				"query": {
					Type:        "string",
					Description: "Search query",
				},
			},
			Required: []string{"query"},
		}),
		WithToolExecuteFn(func(ctx context.Context, args map[string]any) (tool.Result, error) {
			return tool.NewResult(results), nil
		}),
	)
}
