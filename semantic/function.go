// Package semantic 提供语义函数功能
//
// 本包实现 Semantic Functions：
//   - Prompt 函数：基于 Prompt 模板定义的函数
//   - Native 函数：Go 原生函数
//   - 函数组合：函数链式调用
//   - 函数注册：统一函数管理
//
// 设计借鉴：
//   - Semantic Kernel: Semantic Functions
//   - LangChain: Prompt Templates + Chains
//   - OpenAI: Function Calling
package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"text/template"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/schema"
)

// ============== 核心类型 ==============

// Function 语义函数接口
type Function interface {
	// Name 函数名称
	Name() string

	// Description 函数描述
	Description() string

	// InputSchema 输入 Schema
	InputSchema() *schema.Schema

	// OutputSchema 输出 Schema
	OutputSchema() *schema.Schema

	// Invoke 调用函数
	Invoke(ctx context.Context, input map[string]any) (any, error)
}

// FunctionType 函数类型
type FunctionType string

const (
	// FunctionTypeSemantic 语义函数（基于 Prompt）
	FunctionTypeSemantic FunctionType = "semantic"

	// FunctionTypeNative Native 函数
	FunctionTypeNative FunctionType = "native"

	// FunctionTypeComposite 组合函数
	FunctionTypeComposite FunctionType = "composite"
)

// ============== Semantic Function ==============

// SemanticFunction 语义函数
// 使用 Prompt 模板和 LLM 实现的函数
type SemanticFunction struct {
	// name 函数名称
	name string

	// description 函数描述
	description string

	// prompt Prompt 模板
	prompt *template.Template

	// promptText 原始 Prompt 文本
	promptText string

	// inputSchema 输入 Schema
	inputSchema *schema.Schema

	// outputSchema 输出 Schema
	outputSchema *schema.Schema

	// llm LLM 提供者
	llm llm.Provider

	// model 使用的模型
	model string

	// temperature 温度参数
	temperature float64

	// maxTokens 最大 token 数
	maxTokens int

	// outputParser 输出解析器
	outputParser OutputParser

	// systemPrompt 系统提示
	systemPrompt string
}

// SemanticFunctionConfig 语义函数配置
type SemanticFunctionConfig struct {
	// Name 函数名称
	Name string

	// Description 函数描述
	Description string

	// Prompt Prompt 模板
	Prompt string

	// SystemPrompt 系统提示
	SystemPrompt string

	// LLM LLM 提供者
	LLM llm.Provider

	// Model 模型名称
	Model string

	// Temperature 温度
	Temperature float64

	// MaxTokens 最大 token 数
	MaxTokens int

	// InputSchema 输入 Schema
	InputSchema *schema.Schema

	// OutputSchema 输出 Schema
	OutputSchema *schema.Schema

	// OutputParser 输出解析器
	OutputParser OutputParser
}

// NewSemanticFunction 创建语义函数
func NewSemanticFunction(config SemanticFunctionConfig) (*SemanticFunction, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if config.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if config.LLM == nil {
		return nil, fmt.Errorf("LLM provider is required")
	}

	// 解析 Prompt 模板
	tmpl, err := template.New(config.Name).Parse(config.Prompt)
	if err != nil {
		return nil, fmt.Errorf("invalid prompt template: %w", err)
	}

	// 默认输出解析器
	if config.OutputParser == nil {
		config.OutputParser = &StringOutputParser{}
	}

	return &SemanticFunction{
		name:         config.Name,
		description:  config.Description,
		prompt:       tmpl,
		promptText:   config.Prompt,
		inputSchema:  config.InputSchema,
		outputSchema: config.OutputSchema,
		llm:          config.LLM,
		model:        config.Model,
		temperature:  config.Temperature,
		maxTokens:    config.MaxTokens,
		outputParser: config.OutputParser,
		systemPrompt: config.SystemPrompt,
	}, nil
}

// Name 返回函数名称
func (f *SemanticFunction) Name() string {
	return f.name
}

// Description 返回函数描述
func (f *SemanticFunction) Description() string {
	return f.description
}

// InputSchema 返回输入 Schema
func (f *SemanticFunction) InputSchema() *schema.Schema {
	return f.inputSchema
}

// OutputSchema 返回输出 Schema
func (f *SemanticFunction) OutputSchema() *schema.Schema {
	return f.outputSchema
}

// Invoke 调用函数
func (f *SemanticFunction) Invoke(ctx context.Context, input map[string]any) (any, error) {
	// 渲染 Prompt
	var buf strings.Builder
	if err := f.prompt.Execute(&buf, input); err != nil {
		return nil, fmt.Errorf("failed to render prompt: %w", err)
	}
	prompt := buf.String()

	// 构建消息
	messages := []llm.Message{}
	if f.systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: f.systemPrompt,
		})
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: prompt,
	})

	// 调用 LLM
	req := llm.CompletionRequest{
		Messages: messages,
	}
	if f.model != "" {
		req.Model = f.model
	}
	if f.temperature > 0 {
		req.Temperature = &f.temperature
	}
	if f.maxTokens > 0 {
		req.MaxTokens = f.maxTokens
	}

	resp, err := f.llm.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析输出
	return f.outputParser.Parse(resp.Content)
}

// ============== Native Function ==============

// NativeFunction Native 函数
// 直接封装 Go 函数
type NativeFunction struct {
	// name 函数名称
	name string

	// description 函数描述
	description string

	// fn 函数实现
	fn any

	// inputSchema 输入 Schema
	inputSchema *schema.Schema

	// outputSchema 输出 Schema
	outputSchema *schema.Schema
}

// NativeFunctionConfig Native 函数配置
type NativeFunctionConfig struct {
	Name         string
	Description  string
	Fn           any
	InputSchema  *schema.Schema
	OutputSchema *schema.Schema
}

// NewNativeFunction 创建 Native 函数
func NewNativeFunction(config NativeFunctionConfig) (*NativeFunction, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if config.Fn == nil {
		return nil, fmt.Errorf("function is required")
	}

	// 验证函数签名
	fnType := reflect.TypeOf(config.Fn)
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("fn must be a function")
	}

	return &NativeFunction{
		name:         config.Name,
		description:  config.Description,
		fn:           config.Fn,
		inputSchema:  config.InputSchema,
		outputSchema: config.OutputSchema,
	}, nil
}

// Name 返回函数名称
func (f *NativeFunction) Name() string {
	return f.name
}

// Description 返回函数描述
func (f *NativeFunction) Description() string {
	return f.description
}

// InputSchema 返回输入 Schema
func (f *NativeFunction) InputSchema() *schema.Schema {
	return f.inputSchema
}

// OutputSchema 返回输出 Schema
func (f *NativeFunction) OutputSchema() *schema.Schema {
	return f.outputSchema
}

// Invoke 调用函数
func (f *NativeFunction) Invoke(ctx context.Context, input map[string]any) (any, error) {
	fnValue := reflect.ValueOf(f.fn)
	fnType := fnValue.Type()

	// 构建参数
	args := make([]reflect.Value, fnType.NumIn())

	// 第一个参数可能是 context
	argOffset := 0
	if fnType.NumIn() > 0 && fnType.In(0) == reflect.TypeOf((*context.Context)(nil)).Elem() {
		args[0] = reflect.ValueOf(ctx)
		argOffset = 1
	}

	// 处理剩余参数
	if fnType.NumIn() > argOffset {
		inputType := fnType.In(argOffset)

		// 如果输入是 map
		if inputType.Kind() == reflect.Map {
			args[argOffset] = reflect.ValueOf(input)
		} else if inputType.Kind() == reflect.Struct || (inputType.Kind() == reflect.Ptr && inputType.Elem().Kind() == reflect.Struct) {
			// 如果输入是结构体，尝试转换
			inputData, _ := json.Marshal(input)
			inputVal := reflect.New(inputType)
			if inputType.Kind() == reflect.Ptr {
				inputVal = reflect.New(inputType.Elem())
			}
			if err := json.Unmarshal(inputData, inputVal.Interface()); err != nil {
				return nil, fmt.Errorf("failed to parse input: %w", err)
			}
			if inputType.Kind() == reflect.Ptr {
				args[argOffset] = inputVal
			} else {
				args[argOffset] = inputVal.Elem()
			}
		}
	}

	// 调用函数
	results := fnValue.Call(args)

	// 处理返回值
	if len(results) == 0 {
		return nil, nil
	}

	// 检查错误
	if len(results) == 2 {
		if !results[1].IsNil() {
			return nil, results[1].Interface().(error)
		}
		return results[0].Interface(), nil
	}

	return results[0].Interface(), nil
}

// ============== Composite Function ==============

// CompositeFunction 组合函数
// 将多个函数组合成一个
type CompositeFunction struct {
	// name 函数名称
	name string

	// description 函数描述
	description string

	// functions 函数列表
	functions []Function

	// inputSchema 输入 Schema
	inputSchema *schema.Schema

	// outputSchema 输出 Schema
	outputSchema *schema.Schema
}

// NewCompositeFunction 创建组合函数
func NewCompositeFunction(name, description string, functions ...Function) *CompositeFunction {
	var inputSchema, outputSchema *schema.Schema
	if len(functions) > 0 {
		inputSchema = functions[0].InputSchema()
		outputSchema = functions[len(functions)-1].OutputSchema()
	}

	return &CompositeFunction{
		name:         name,
		description:  description,
		functions:    functions,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
	}
}

// Name 返回函数名称
func (f *CompositeFunction) Name() string {
	return f.name
}

// Description 返回函数描述
func (f *CompositeFunction) Description() string {
	return f.description
}

// InputSchema 返回输入 Schema
func (f *CompositeFunction) InputSchema() *schema.Schema {
	return f.inputSchema
}

// OutputSchema 返回输出 Schema
func (f *CompositeFunction) OutputSchema() *schema.Schema {
	return f.outputSchema
}

// Invoke 调用函数
func (f *CompositeFunction) Invoke(ctx context.Context, input map[string]any) (any, error) {
	var result any = input

	for _, fn := range f.functions {
		// 转换输入
		inputMap, ok := result.(map[string]any)
		if !ok {
			inputMap = map[string]any{"input": result}
		}

		// 调用函数
		var err error
		result, err = fn.Invoke(ctx, inputMap)
		if err != nil {
			return nil, fmt.Errorf("function %s failed: %w", fn.Name(), err)
		}
	}

	return result, nil
}

// ============== Output Parser ==============

// OutputParser 输出解析器接口
type OutputParser interface {
	Parse(output string) (any, error)
}

// StringOutputParser 字符串输出解析器
type StringOutputParser struct{}

// Parse 解析输出
func (p *StringOutputParser) Parse(output string) (any, error) {
	return strings.TrimSpace(output), nil
}

// JSONOutputParser JSON 输出解析器
type JSONOutputParser struct {
	// TargetType 目标类型
	TargetType reflect.Type
}

// Parse 解析输出
func (p *JSONOutputParser) Parse(output string) (any, error) {
	// 提取 JSON
	output = extractJSON(output)

	if p.TargetType == nil {
		var result any
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return result, nil
	}

	result := reflect.New(p.TargetType).Interface()
	if err := json.Unmarshal([]byte(output), result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return result, nil
}

// extractJSON 从文本中提取 JSON
func extractJSON(text string) string {
	// 尝试找到 JSON 块
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")

	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}

	// 尝试数组
	start = strings.Index(text, "[")
	end = strings.LastIndex(text, "]")

	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}

	return text
}

// ListOutputParser 列表输出解析器
type ListOutputParser struct {
	// Separator 分隔符
	Separator string
}

// Parse 解析输出
func (p *ListOutputParser) Parse(output string) (any, error) {
	sep := p.Separator
	if sep == "" {
		sep = "\n"
	}

	lines := strings.Split(output, sep)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// 移除可能的编号前缀
			if len(line) > 2 && (line[0] >= '0' && line[0] <= '9') && (line[1] == '.' || line[1] == ')') {
				line = strings.TrimSpace(line[2:])
			}
			result = append(result, line)
		}
	}

	return result, nil
}

// BoolOutputParser 布尔输出解析器
type BoolOutputParser struct{}

// Parse 解析输出
func (p *BoolOutputParser) Parse(output string) (any, error) {
	output = strings.ToLower(strings.TrimSpace(output))

	trueValues := []string{"true", "yes", "是", "对", "正确", "1"}
	for _, v := range trueValues {
		if output == v || strings.HasPrefix(output, v) {
			return true, nil
		}
	}

	return false, nil
}

// ============== Function Registry ==============

// Registry 函数注册表
type Registry struct {
	functions map[string]Function
	mu        sync.RWMutex
}

// NewRegistry 创建函数注册表
func NewRegistry() *Registry {
	return &Registry{
		functions: make(map[string]Function),
	}
}

// Register 注册函数
func (r *Registry) Register(fn Function) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.functions[fn.Name()]; exists {
		return fmt.Errorf("function already exists: %s", fn.Name())
	}

	r.functions[fn.Name()] = fn
	return nil
}

// Get 获取函数
func (r *Registry) Get(name string) (Function, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fn, exists := r.functions[name]
	return fn, exists
}

// List 列出所有函数
func (r *Registry) List() []Function {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Function, 0, len(r.functions))
	for _, fn := range r.functions {
		result = append(result, fn)
	}
	return result
}

// Invoke 调用函数
func (r *Registry) Invoke(ctx context.Context, name string, input map[string]any) (any, error) {
	fn, exists := r.Get(name)
	if !exists {
		return nil, fmt.Errorf("function not found: %s", name)
	}
	return fn.Invoke(ctx, input)
}

// ============== 便捷函数 ==============

// NewFunc 从普通函数创建 Native 函数
func NewFunc[I, O any](name, description string, fn func(ctx context.Context, input I) (O, error)) *NativeFunction {
	nf, _ := NewNativeFunction(NativeFunctionConfig{
		Name:         name,
		Description:  description,
		Fn:           fn,
		InputSchema:  schema.Of[I](),
		OutputSchema: schema.Of[O](),
	})
	return nf
}

// Chain 创建函数链
func Chain(name, description string, functions ...Function) *CompositeFunction {
	return NewCompositeFunction(name, description, functions...)
}

// ============== 全局注册表 ==============

var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
)

// GlobalRegistry 获取全局注册表
func GlobalRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
	})
	return globalRegistry
}

// Register 注册到全局注册表
func Register(fn Function) error {
	return GlobalRegistry().Register(fn)
}

// Invoke 从全局注册表调用函数
func Invoke(ctx context.Context, name string, input map[string]any) (any, error) {
	return GlobalRegistry().Invoke(ctx, name, input)
}
