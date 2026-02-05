// Package template 提供 LLM 提示词模板引擎
//
// 本包实现了完整的提示词模板系统：
//   - 变量替换：{{ variable }}
//   - 条件语句：{% if condition %}...{% endif %}
//   - 循环语句：{% for item in items %}...{% endfor %}
//   - 过滤器：{{ value | filter }}
//   - 模板继承和包含
//   - 提示词版本控制
//
// 设计借鉴：
//   - Jinja2: 模板语法
//   - LangChain: PromptTemplate
//   - Semantic Kernel: 语义函数模板
//
// 使用示例：
//
//	tpl := template.New("greeting", "Hello, {{ name }}!")
//	result, err := tpl.Execute(map[string]any{"name": "World"})
//	// result: "Hello, World!"
package template

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/everyday-items/toolkit/lang/conv"
)

// ============== 错误定义 ==============

var (
	// ErrTemplateNotFound 模板未找到
	ErrTemplateNotFound = errors.New("template not found")

	// ErrInvalidTemplate 无效模板
	ErrInvalidTemplate = errors.New("invalid template")

	// ErrMissingVariable 缺少变量
	ErrMissingVariable = errors.New("missing required variable")

	// ErrExecutionFailed 执行失败
	ErrExecutionFailed = errors.New("template execution failed")
)

// ============== 模板接口 ==============

// Template 提示词模板接口
type Template interface {
	// Name 模板名称
	Name() string

	// Execute 执行模板
	Execute(variables map[string]any) (string, error)

	// ExecuteContext 带上下文执行
	ExecuteContext(ctx context.Context, variables map[string]any) (string, error)

	// Variables 获取所有变量
	Variables() []Variable

	// Validate 验证变量
	Validate(variables map[string]any) error
}

// Variable 模板变量定义
type Variable struct {
	// Name 变量名
	Name string `json:"name"`

	// Type 变量类型
	Type string `json:"type"`

	// Description 描述
	Description string `json:"description"`

	// Required 是否必需
	Required bool `json:"required"`

	// Default 默认值
	Default any `json:"default,omitempty"`

	// Examples 示例值
	Examples []any `json:"examples,omitempty"`
}

// ============== 基础模板 ==============

// PromptTemplate 基础提示词模板
type PromptTemplate struct {
	name      string
	template  string
	variables []Variable
	parsed    *template.Template
	mu        sync.RWMutex
}

// New 创建新模板
func New(name, tmpl string, variables ...Variable) *PromptTemplate {
	pt := &PromptTemplate{
		name:      name,
		template:  tmpl,
		variables: variables,
	}

	// 解析变量（如果未提供）
	if len(variables) == 0 {
		pt.variables = pt.extractVariables()
	}

	return pt
}

// Name 返回模板名称
func (pt *PromptTemplate) Name() string {
	return pt.name
}

// Execute 执行模板
func (pt *PromptTemplate) Execute(variables map[string]any) (string, error) {
	return pt.ExecuteContext(context.Background(), variables)
}

// ExecuteContext 带上下文执行模板
func (pt *PromptTemplate) ExecuteContext(ctx context.Context, variables map[string]any) (string, error) {
	// 验证变量
	if err := pt.Validate(variables); err != nil {
		return "", err
	}

	// 应用默认值
	vars := pt.applyDefaults(variables)

	// 获取或解析模板
	tmpl, err := pt.getParsedTemplate()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}

	// 执行模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}

	return buf.String(), nil
}

// Variables 获取变量列表
func (pt *PromptTemplate) Variables() []Variable {
	return pt.variables
}

// Validate 验证变量
func (pt *PromptTemplate) Validate(variables map[string]any) error {
	for _, v := range pt.variables {
		if v.Required {
			if _, ok := variables[v.Name]; !ok {
				if v.Default == nil {
					return fmt.Errorf("%w: %s", ErrMissingVariable, v.Name)
				}
			}
		}
	}
	return nil
}

// getParsedTemplate 获取解析后的模板
func (pt *PromptTemplate) getParsedTemplate() (*template.Template, error) {
	pt.mu.RLock()
	if pt.parsed != nil {
		pt.mu.RUnlock()
		return pt.parsed, nil
	}
	pt.mu.RUnlock()

	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.parsed != nil {
		return pt.parsed, nil
	}

	// 转换模板语法
	goTemplate := convertToGoTemplate(pt.template)

	// 解析模板
	tmpl, err := template.New(pt.name).Funcs(defaultFuncMap()).Parse(goTemplate)
	if err != nil {
		return nil, err
	}

	pt.parsed = tmpl
	return pt.parsed, nil
}

// applyDefaults 应用默认值
func (pt *PromptTemplate) applyDefaults(variables map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range variables {
		result[k] = v
	}

	for _, v := range pt.variables {
		if _, ok := result[v.Name]; !ok && v.Default != nil {
			result[v.Name] = v.Default
		}
	}

	return result
}

// extractVariables 从模板中提取变量
func (pt *PromptTemplate) extractVariables() []Variable {
	// 匹配 {{ variable }} 和 {{ variable | filter }}
	re := regexp.MustCompile(`\{\{\s*(\w+)(?:\s*\|[^}]*)?\s*\}\}`)
	matches := re.FindAllStringSubmatch(pt.template, -1)

	seen := make(map[string]bool)
	variables := make([]Variable, 0)

	for _, match := range matches {
		if len(match) > 1 {
			name := match[1]
			if !seen[name] {
				seen[name] = true
				variables = append(variables, Variable{
					Name:     name,
					Type:     "string",
					Required: true,
				})
			}
		}
	}

	return variables
}

// convertToGoTemplate 将 Jinja2 风格转换为 Go 模板语法
func convertToGoTemplate(tmpl string) string {
	// 变量: {{ var }} -> {{.var}}
	result := regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`).ReplaceAllString(tmpl, "{{.${1}}}")

	// 带过滤器的变量: {{ var | filter }} -> {{.var | filter}}
	result = regexp.MustCompile(`\{\{\s*(\w+)\s*\|\s*(\w+)\s*\}\}`).ReplaceAllString(result, "{{.${1} | ${2}}}")

	// if 语句: {% if condition %} -> {{if condition}}
	result = regexp.MustCompile(`\{%\s*if\s+(.+?)\s*%\}`).ReplaceAllString(result, "{{if ${1}}}")

	// elif 语句: {% elif condition %} -> {{else if condition}}
	result = regexp.MustCompile(`\{%\s*elif\s+(.+?)\s*%\}`).ReplaceAllString(result, "{{else if ${1}}}")

	// else 语句: {% else %} -> {{else}}
	result = regexp.MustCompile(`\{%\s*else\s*%\}`).ReplaceAllString(result, "{{else}}")

	// endif 语句: {% endif %} -> {{end}}
	result = regexp.MustCompile(`\{%\s*endif\s*%\}`).ReplaceAllString(result, "{{end}}")

	// for 循环: {% for item in items %} -> {{range $item := .items}}
	result = regexp.MustCompile(`\{%\s*for\s+(\w+)\s+in\s+(\w+)\s*%\}`).ReplaceAllString(result, "{{range $$${1} := .${2}}}")

	// endfor 语句: {% endfor %} -> {{end}}
	result = regexp.MustCompile(`\{%\s*endfor\s*%\}`).ReplaceAllString(result, "{{end}}")

	return result
}

// defaultFuncMap 默认函数映射
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// 字符串处理
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      strings.Title,
		"trim":       strings.TrimSpace,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"repeat":     strings.Repeat,
		"truncate":   truncate,
		"default":    defaultValue,
		"quote":      strconv.Quote,

		// 数字处理
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int { return a / b },
		"mod": func(a, b int) int { return a % b },

		// 类型转换
		"toString": toString,
		"toInt":    toInt,
		"toFloat":  toFloat,
		"toBool":   toBool,

		// 日期时间
		"now":        time.Now,
		"formatDate": formatDate,

		// JSON
		"toJSON":   toJSON,
		"fromJSON": fromJSON,

		// 列表
		"first": first,
		"last":  last,
		"len":   length,
		"index": index,
		"slice": sliceList,

		// 条件
		"ternary": ternary,
		"empty":   empty,
		"coalesce": coalesce,
	}
}

// 辅助函数实现

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

func defaultValue(defaultVal, val any) any {
	if empty(val) {
		return defaultVal
	}
	return val
}

// 使用 toolkit/lang/conv 的类型转换函数
// 这些函数比原有实现更健壮，支持更多类型
var (
	toString = conv.String
	toInt    = conv.Int
	toFloat  = conv.Float64
	toBool   = conv.Bool
)

func formatDate(t time.Time, layout string) string {
	return t.Format(layout)
}

func toJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func fromJSON(s string) any {
	var result any
	json.Unmarshal([]byte(s), &result)
	return result
}

func first(list any) any {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice || v.Len() == 0 {
		return nil
	}
	return v.Index(0).Interface()
}

func last(list any) any {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice || v.Len() == 0 {
		return nil
	}
	return v.Index(v.Len() - 1).Interface()
}

func length(v any) int {
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return val.Len()
	default:
		return 0
	}
}

func index(list any, i int) any {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice || i < 0 || i >= v.Len() {
		return nil
	}
	return v.Index(i).Interface()
}

func sliceList(list any, start, end int) any {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice {
		return list
	}
	if start < 0 {
		start = 0
	}
	if end > v.Len() {
		end = v.Len()
	}
	return v.Slice(start, end).Interface()
}

func ternary(condition bool, trueVal, falseVal any) any {
	if condition {
		return trueVal
	}
	return falseVal
}

func empty(v any) bool {
	if v == nil {
		return true
	}
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return val.Len() == 0
	case reflect.Bool:
		return !val.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() == 0
	case reflect.Float32, reflect.Float64:
		return val.Float() == 0
	case reflect.Ptr, reflect.Interface:
		return val.IsNil()
	default:
		return false
	}
}

func coalesce(values ...any) any {
	for _, v := range values {
		if !empty(v) {
			return v
		}
	}
	return nil
}

// ============== 消息模板 ==============

// MessageTemplate 消息模板
type MessageTemplate struct {
	// Role 消息角色
	Role string `json:"role"`

	// Template 内容模板
	Template *PromptTemplate `json:"-"`

	// Content 静态内容（如果不使用模板）
	Content string `json:"content,omitempty"`
}

// Execute 执行消息模板
func (mt *MessageTemplate) Execute(variables map[string]any) (string, string, error) {
	if mt.Template != nil {
		content, err := mt.Template.Execute(variables)
		if err != nil {
			return "", "", err
		}
		return mt.Role, content, nil
	}
	return mt.Role, mt.Content, nil
}

// ChatTemplate 对话模板
type ChatTemplate struct {
	name      string
	messages  []*MessageTemplate
	variables []Variable
}

// NewChatTemplate 创建对话模板
func NewChatTemplate(name string) *ChatTemplate {
	return &ChatTemplate{
		name:     name,
		messages: make([]*MessageTemplate, 0),
	}
}

// AddSystemMessage 添加系统消息
func (ct *ChatTemplate) AddSystemMessage(template string) *ChatTemplate {
	ct.messages = append(ct.messages, &MessageTemplate{
		Role:     "system",
		Template: New("system", template),
	})
	return ct
}

// AddUserMessage 添加用户消息
func (ct *ChatTemplate) AddUserMessage(template string) *ChatTemplate {
	ct.messages = append(ct.messages, &MessageTemplate{
		Role:     "user",
		Template: New("user", template),
	})
	return ct
}

// AddAssistantMessage 添加助手消息
func (ct *ChatTemplate) AddAssistantMessage(template string) *ChatTemplate {
	ct.messages = append(ct.messages, &MessageTemplate{
		Role:     "assistant",
		Template: New("assistant", template),
	})
	return ct
}

// Execute 执行对话模板
func (ct *ChatTemplate) Execute(variables map[string]any) ([]Message, error) {
	messages := make([]Message, 0, len(ct.messages))

	for _, mt := range ct.messages {
		role, content, err := mt.Execute(variables)
		if err != nil {
			return nil, err
		}
		messages = append(messages, Message{
			Role:    role,
			Content: content,
		})
	}

	return messages, nil
}

// Message 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ============== 模板库 ==============

// TemplateLibrary 模板库
type TemplateLibrary struct {
	templates map[string]Template
	mu        sync.RWMutex
}

// NewTemplateLibrary 创建模板库
func NewTemplateLibrary() *TemplateLibrary {
	return &TemplateLibrary{
		templates: make(map[string]Template),
	}
}

// Register 注册模板
func (lib *TemplateLibrary) Register(tmpl Template) {
	lib.mu.Lock()
	lib.templates[tmpl.Name()] = tmpl
	lib.mu.Unlock()
}

// Get 获取模板
func (lib *TemplateLibrary) Get(name string) (Template, error) {
	lib.mu.RLock()
	tmpl, ok := lib.templates[name]
	lib.mu.RUnlock()

	if !ok {
		return nil, ErrTemplateNotFound
	}
	return tmpl, nil
}

// Execute 执行指定模板
func (lib *TemplateLibrary) Execute(name string, variables map[string]any) (string, error) {
	tmpl, err := lib.Get(name)
	if err != nil {
		return "", err
	}
	return tmpl.Execute(variables)
}

// List 列出所有模板
func (lib *TemplateLibrary) List() []string {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	names := make([]string, 0, len(lib.templates))
	for name := range lib.templates {
		names = append(names, name)
	}
	return names
}

// ============== 预置模板 ==============

// 常用提示词模板
var (
	// SummarizeTemplate 摘要模板
	SummarizeTemplate = New("summarize", `Please summarize the following text in {{ language | default "English" }}:

{{ text }}

Requirements:
- Keep it concise ({{ max_length | default "200" }} words max)
- Preserve key information
- Use clear and professional language`)

	// TranslateTemplate 翻译模板
	TranslateTemplate = New("translate", `Please translate the following text to {{ target_language }}:

{{ text }}

Note: Maintain the original meaning and tone.`)

	// QATemplate 问答模板
	QATemplate = New("qa", `Based on the following context, answer the question.

Context:
{{ context }}

Question: {{ question }}

Please provide a clear and accurate answer based only on the given context.`)

	// CodeReviewTemplate 代码审查模板
	CodeReviewTemplate = New("code_review", "Please review the following {{ language | default \"code\" }} and provide feedback:\n\n```{{ language }}\n{{ code }}\n```\n\nFocus on:\n- Code quality and best practices\n- Potential bugs or issues\n- Performance considerations\n- Suggestions for improvement")

	// ExtractTemplate 信息提取模板
	ExtractTemplate = New("extract", `Extract the following information from the text:
{{ fields | join ", " }}

Text:
{{ text }}

Respond in JSON format.`)
)

// RegisterBuiltinTemplates 注册内置模板
func RegisterBuiltinTemplates(lib *TemplateLibrary) {
	lib.Register(SummarizeTemplate)
	lib.Register(TranslateTemplate)
	lib.Register(QATemplate)
	lib.Register(CodeReviewTemplate)
	lib.Register(ExtractTemplate)
}

// ============== Few-Shot 模板 ==============

// FewShotTemplate Few-Shot 学习模板
type FewShotTemplate struct {
	name       string
	prefix     string
	examples   []Example
	suffix     string
	separator  string
	exampleTpl *PromptTemplate
}

// Example 示例
type Example struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// NewFewShotTemplate 创建 Few-Shot 模板
func NewFewShotTemplate(name string) *FewShotTemplate {
	return &FewShotTemplate{
		name:      name,
		separator: "\n\n",
	}
}

// WithPrefix 设置前缀
func (ft *FewShotTemplate) WithPrefix(prefix string) *FewShotTemplate {
	ft.prefix = prefix
	return ft
}

// WithSuffix 设置后缀
func (ft *FewShotTemplate) WithSuffix(suffix string) *FewShotTemplate {
	ft.suffix = suffix
	return ft
}

// WithExamples 设置示例
func (ft *FewShotTemplate) WithExamples(examples []Example) *FewShotTemplate {
	ft.examples = examples
	return ft
}

// WithExampleTemplate 设置示例模板
func (ft *FewShotTemplate) WithExampleTemplate(template string) *FewShotTemplate {
	ft.exampleTpl = New("example", template)
	return ft
}

// WithSeparator 设置分隔符
func (ft *FewShotTemplate) WithSeparator(sep string) *FewShotTemplate {
	ft.separator = sep
	return ft
}

// Name 返回模板名称
func (ft *FewShotTemplate) Name() string {
	return ft.name
}

// Execute 执行模板
func (ft *FewShotTemplate) Execute(variables map[string]any) (string, error) {
	var parts []string

	// 前缀
	if ft.prefix != "" {
		parts = append(parts, ft.prefix)
	}

	// 示例
	for _, example := range ft.examples {
		if ft.exampleTpl != nil {
			exampleStr, err := ft.exampleTpl.Execute(map[string]any{
				"input":  example.Input,
				"output": example.Output,
			})
			if err != nil {
				return "", err
			}
			parts = append(parts, exampleStr)
		} else {
			parts = append(parts, fmt.Sprintf("Input: %s\nOutput: %s", example.Input, example.Output))
		}
	}

	// 后缀（包含用户输入）
	if ft.suffix != "" {
		suffixTpl := New("suffix", ft.suffix)
		suffixStr, err := suffixTpl.Execute(variables)
		if err != nil {
			return "", err
		}
		parts = append(parts, suffixStr)
	}

	return strings.Join(parts, ft.separator), nil
}

// Variables 获取变量列表
func (ft *FewShotTemplate) Variables() []Variable {
	if ft.suffix == "" {
		return nil
	}
	suffixTpl := New("suffix", ft.suffix)
	return suffixTpl.Variables()
}

// Validate 验证变量
func (ft *FewShotTemplate) Validate(variables map[string]any) error {
	if ft.suffix == "" {
		return nil
	}
	suffixTpl := New("suffix", ft.suffix)
	return suffixTpl.Validate(variables)
}
