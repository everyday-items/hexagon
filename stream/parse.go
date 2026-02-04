// Package stream 提供 Hexagon 框架的增强流处理能力
//
// 本文件实现流式 JSON 解析功能：
//   - ParseJSON[T]: 流式解析 JSON，支持增量解析
//   - ParseJSONArray[T]: 流式解析 JSON 数组
//   - JSONStreamParser: 增量 JSON 解析器
//
// 设计借鉴：
//   - LangChain: PydanticOutputParser
//   - OpenAI: Structured Output with streaming
//   - Jackson (Java): Streaming JSON API
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// ============== 错误定义 ==============

var (
	// ErrInvalidJSON JSON 解析错误
	ErrInvalidJSON = errors.New("invalid JSON")

	// ErrIncompleteJSON JSON 不完整
	ErrIncompleteJSON = errors.New("incomplete JSON")

	// ErrTypeMismatch 类型不匹配
	ErrTypeMismatch = errors.New("type mismatch")
)

// ============== 流式 JSON 解析 ==============

// JSONParseMode JSON 解析模式
type JSONParseMode int

const (
	// JSONParseComplete 完整解析（等待完整 JSON 再解析）
	JSONParseComplete JSONParseMode = iota
	// JSONParseIncremental 增量解析（边接收边解析）
	JSONParseIncremental
	// JSONParsePartial 部分解析（解析已接收的部分）
	JSONParsePartial
)

// JSONParseConfig JSON 解析配置
type JSONParseConfig struct {
	// Mode 解析模式
	Mode JSONParseMode
	// StrictMode 严格模式（不允许多余字段）
	StrictMode bool
	// MaxBufferSize 最大缓冲区大小（字节）
	MaxBufferSize int
	// OnPartial 部分解析回调
	OnPartial func(partial any)
}

// DefaultJSONParseConfig 默认 JSON 解析配置
func DefaultJSONParseConfig() *JSONParseConfig {
	return &JSONParseConfig{
		Mode:          JSONParseComplete,
		StrictMode:    false,
		MaxBufferSize: 1024 * 1024, // 1MB
	}
}

// ParseJSON 流式解析 JSON
// 从字符串流中解析出 T 类型的对象
//
// 示例：
//
//	type User struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//	userStream := stream.ParseJSON[User](stringStream)
//	for {
//	    user, err := userStream.Recv()
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Printf("User: %+v\n", user)
//	}
func ParseJSON[T any](sr *StreamReader[string], config ...*JSONParseConfig) *StreamReader[T] {
	cfg := DefaultJSONParseConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	parser := &jsonStreamParser[T]{
		source:  sr,
		config:  cfg,
		buffer:  strings.Builder{},
		results: make(chan T, 10),
		done:    make(chan struct{}),
	}
	go parser.parse()

	return &StreamReader[T]{
		typ: readerTypePipe,
		pipe: &pipeReader[T]{
			ch:   parser.results,
			done: parser.done,
		},
		source: sr.source,
	}
}

// jsonStreamParser 流式 JSON 解析器
type jsonStreamParser[T any] struct {
	source  *StreamReader[string]
	config  *JSONParseConfig
	buffer  strings.Builder
	results chan T
	done    chan struct{}
	mu      sync.Mutex
}

func (p *jsonStreamParser[T]) parse() {
	defer close(p.results)
	defer close(p.done)

	for {
		chunk, err := p.source.Recv()
		if err == io.EOF {
			// 尝试解析剩余内容
			p.tryParse()
			return
		}
		if err != nil {
			return
		}

		p.mu.Lock()
		p.buffer.WriteString(chunk)
		p.mu.Unlock()

		// 检查缓冲区大小
		if p.buffer.Len() > p.config.MaxBufferSize {
			return
		}

		// 根据模式处理
		switch p.config.Mode {
		case JSONParseIncremental:
			p.tryParseIncremental()
		case JSONParsePartial:
			p.tryParsePartial()
		default:
			// JSONParseComplete: 继续累积
		}
	}
}

func (p *jsonStreamParser[T]) tryParse() {
	p.mu.Lock()
	content := p.buffer.String()
	p.mu.Unlock()

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	// 尝试提取 JSON
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return
	}

	var result T
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return
	}

	select {
	case p.results <- result:
	case <-p.done:
	}
}

func (p *jsonStreamParser[T]) tryParseIncremental() {
	p.mu.Lock()
	content := p.buffer.String()
	p.mu.Unlock()

	// 查找完整的 JSON 对象
	start := strings.Index(content, "{")
	if start == -1 {
		start = strings.Index(content, "[")
	}
	if start == -1 {
		return
	}

	// 尝试找到匹配的结束符
	end := findJSONEnd(content[start:])
	if end == -1 {
		return
	}

	jsonStr := content[start : start+end+1]
	var result T
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return
	}

	// 清除已解析的内容
	p.mu.Lock()
	p.buffer.Reset()
	p.buffer.WriteString(content[start+end+1:])
	p.mu.Unlock()

	select {
	case p.results <- result:
	case <-p.done:
	}
}

func (p *jsonStreamParser[T]) tryParsePartial() {
	if p.config.OnPartial == nil {
		return
	}

	p.mu.Lock()
	content := p.buffer.String()
	p.mu.Unlock()

	// 尝试解析部分 JSON
	partial := parsePartialJSON(content)
	if partial != nil {
		p.config.OnPartial(partial)
	}
}

// ParseJSONArray 流式解析 JSON 数组
// 从字符串流中解析出 T 类型的数组元素
func ParseJSONArray[T any](sr *StreamReader[string], config ...*JSONParseConfig) *StreamReader[T] {
	cfg := DefaultJSONParseConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	parser := &jsonArrayParser[T]{
		source:  sr,
		config:  cfg,
		buffer:  strings.Builder{},
		results: make(chan T, 10),
		done:    make(chan struct{}),
	}
	go parser.parse()

	return &StreamReader[T]{
		typ: readerTypePipe,
		pipe: &pipeReader[T]{
			ch:   parser.results,
			done: parser.done,
		},
		source: sr.source,
	}
}

// jsonArrayParser JSON 数组流式解析器
type jsonArrayParser[T any] struct {
	source   *StreamReader[string]
	config   *JSONParseConfig
	buffer   strings.Builder
	results  chan T
	done     chan struct{}
	inArray  bool
	depth    int
	itemBuf  strings.Builder
	mu       sync.Mutex
}

func (p *jsonArrayParser[T]) parse() {
	defer close(p.results)
	defer close(p.done)

	for {
		chunk, err := p.source.Recv()
		if err == io.EOF {
			// 处理最后一个元素
			p.flushItem()
			return
		}
		if err != nil {
			return
		}

		p.processChunk(chunk)
	}
}

func (p *jsonArrayParser[T]) processChunk(chunk string) {
	for _, ch := range chunk {
		if !p.inArray {
			if ch == '[' {
				p.inArray = true
			}
			continue
		}

		switch ch {
		case '{', '[':
			p.depth++
			p.itemBuf.WriteRune(ch)
		case '}', ']':
			if p.depth > 0 {
				p.itemBuf.WriteRune(ch)
				p.depth--
				if p.depth == 0 && ch == '}' {
					p.flushItem()
				}
			} else if ch == ']' {
				// 数组结束
				p.flushItem()
				p.inArray = false
			}
		case ',':
			if p.depth == 0 {
				p.flushItem()
			} else {
				p.itemBuf.WriteRune(ch)
			}
		default:
			if p.depth > 0 || !unicode.IsSpace(ch) {
				if p.depth == 0 && (ch == '"' || unicode.IsDigit(ch) || ch == 't' || ch == 'f' || ch == 'n') {
					// 简单值开始
					p.itemBuf.WriteRune(ch)
					p.depth = 1
				} else if p.depth > 0 {
					p.itemBuf.WriteRune(ch)
				}
			}
		}
	}
}

func (p *jsonArrayParser[T]) flushItem() {
	itemStr := strings.TrimSpace(p.itemBuf.String())
	p.itemBuf.Reset()

	if itemStr == "" {
		return
	}

	var result T
	if err := json.Unmarshal([]byte(itemStr), &result); err != nil {
		return
	}

	select {
	case p.results <- result:
	case <-p.done:
	}
}

// ============== JSON 工具函数 ==============

// extractJSON 从文本中提取 JSON
func extractJSON(text string) string {
	// 首先尝试匹配 markdown 代码块
	codeBlockRe := regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)\\s*```")
	if matches := codeBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// 然后尝试找 JSON 对象或数组
	start := -1
	for i, ch := range text {
		if ch == '{' || ch == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := findJSONEnd(text[start:])
	if end == -1 {
		return ""
	}

	return text[start : start+end+1]
}

// findJSONEnd 找到 JSON 结束位置
func findJSONEnd(s string) int {
	if len(s) == 0 {
		return -1
	}

	opening := s[0]
	var closing byte
	if opening == '{' {
		closing = '}'
	} else if opening == '[' {
		closing = ']'
	} else {
		return -1
	}

	depth := 0
	inString := false
	escape := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escape {
			escape = false
			continue
		}

		if ch == '\\' && inString {
			escape = true
			continue
		}

		if ch == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if ch == opening {
			depth++
		} else if ch == closing {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// parsePartialJSON 解析部分 JSON
func parsePartialJSON(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return nil
	}

	// 尝试补全 JSON
	completed := completeJSON(s)

	var result map[string]any
	if err := json.Unmarshal([]byte(completed), &result); err != nil {
		return nil
	}

	return result
}

// completeJSON 尝试补全不完整的 JSON
func completeJSON(s string) string {
	// 计算需要补全的括号
	braceCount := 0
	bracketCount := 0
	inString := false
	escape := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escape {
			escape = false
			continue
		}

		if ch == '\\' && inString {
			escape = true
			continue
		}

		if ch == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch ch {
		case '{':
			braceCount++
		case '}':
			braceCount--
		case '[':
			bracketCount++
		case ']':
			bracketCount--
		}
	}

	// 如果在字符串中，先闭合字符串
	if inString {
		s += `"`
	}

	// 补全括号
	for bracketCount > 0 {
		s += "]"
		bracketCount--
	}
	for braceCount > 0 {
		s += "}"
		braceCount--
	}

	return s
}

// ============== 结构化输出流 ==============

// StructuredStreamConfig 结构化输出配置
type StructuredStreamConfig struct {
	// Schema JSON Schema 定义
	Schema any
	// Validator 验证函数
	Validator func(any) error
	// OnFieldUpdate 字段更新回调（增量解析时）
	OnFieldUpdate func(field string, value any)
}

// ParseStructured 解析结构化输出
// 支持增量解析和字段级更新通知
func ParseStructured[T any](sr *StreamReader[string], config *StructuredStreamConfig) *StreamReader[T] {
	parser := &structuredParser[T]{
		source:  sr,
		config:  config,
		buffer:  strings.Builder{},
		results: make(chan T, 1),
		done:    make(chan struct{}),
		partial: make(map[string]any),
	}
	go parser.parse()

	return &StreamReader[T]{
		typ: readerTypePipe,
		pipe: &pipeReader[T]{
			ch:   parser.results,
			done: parser.done,
		},
		source: sr.source,
	}
}

// structuredParser 结构化输出解析器
type structuredParser[T any] struct {
	source  *StreamReader[string]
	config  *StructuredStreamConfig
	buffer  strings.Builder
	results chan T
	done    chan struct{}
	partial map[string]any
	mu      sync.Mutex
}

func (p *structuredParser[T]) parse() {
	defer close(p.results)
	defer close(p.done)

	for {
		chunk, err := p.source.Recv()
		if err == io.EOF {
			p.finalize()
			return
		}
		if err != nil {
			return
		}

		p.mu.Lock()
		p.buffer.WriteString(chunk)
		p.mu.Unlock()

		// 尝试增量解析
		p.tryIncrementalParse()
	}
}

func (p *structuredParser[T]) tryIncrementalParse() {
	if p.config == nil || p.config.OnFieldUpdate == nil {
		return
	}

	p.mu.Lock()
	content := p.buffer.String()
	p.mu.Unlock()

	// 解析部分 JSON 并通知字段更新
	partial := parsePartialJSON(content)
	if partial == nil {
		return
	}

	for key, value := range partial {
		if oldValue, exists := p.partial[key]; !exists || !reflect.DeepEqual(oldValue, value) {
			p.partial[key] = value
			p.config.OnFieldUpdate(key, value)
		}
	}
}

func (p *structuredParser[T]) finalize() {
	p.mu.Lock()
	content := p.buffer.String()
	p.mu.Unlock()

	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return
	}

	var result T
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return
	}

	// 验证
	if p.config != nil && p.config.Validator != nil {
		if err := p.config.Validator(result); err != nil {
			return
		}
	}

	select {
	case p.results <- result:
	case <-p.done:
	}
}

// ============== 流式 JSON 构建器 ==============

// JSONBuilder JSON 流式构建器
// 支持边接收边构建 JSON 对象
type JSONBuilder struct {
	fields map[string]any
	mu     sync.RWMutex
}

// NewJSONBuilder 创建 JSON 构建器
func NewJSONBuilder() *JSONBuilder {
	return &JSONBuilder{
		fields: make(map[string]any),
	}
}

// Set 设置字段值
func (b *JSONBuilder) Set(key string, value any) {
	b.mu.Lock()
	b.fields[key] = value
	b.mu.Unlock()
}

// Get 获取字段值
func (b *JSONBuilder) Get(key string) (any, bool) {
	b.mu.RLock()
	v, ok := b.fields[key]
	b.mu.RUnlock()
	return v, ok
}

// Build 构建最终对象
func (b *JSONBuilder) Build() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]any, len(b.fields))
	for k, v := range b.fields {
		result[k] = v
	}
	return result
}

// ToJSON 转换为 JSON 字符串
func (b *JSONBuilder) ToJSON() (string, error) {
	data, err := json.Marshal(b.Build())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Unmarshal 反序列化到目标类型
func (b *JSONBuilder) Unmarshal(target any) error {
	data, err := json.Marshal(b.Build())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// ============== 流连接操作符 ==============

// ConcatStreams 连接多个流（顺序执行）
// 与 Merge 不同，Concat 按顺序消费每个流
func ConcatStreams[T any](readers ...*StreamReader[T]) *StreamReader[T] {
	if len(readers) == 0 {
		return FromSlice[T](nil)
	}
	if len(readers) == 1 {
		return readers[0]
	}

	reader, writer := Pipe[T](10)
	go func() {
		defer writer.Close()
		for _, r := range readers {
			for {
				item, err := r.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					writer.CloseWithError(err)
					return
				}
				if err := writer.Send(item); err != nil {
					return
				}
			}
		}
	}()

	return reader
}

// ============== 流错误恢复 ==============

// RecoveryConfig 错误恢复配置
type RecoveryConfig struct {
	// MaxRetries 最大重试次数
	MaxRetries int
	// RetryDelay 重试延迟
	RetryDelay func(attempt int) int // 返回毫秒
	// OnError 错误回调
	OnError func(err error, attempt int)
	// Fallback 降级函数
	Fallback func(err error) (any, error)
	// ShouldRetry 判断是否应该重试
	ShouldRetry func(err error) bool
}

// DefaultRecoveryConfig 默认恢复配置
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		MaxRetries: 3,
		RetryDelay: func(attempt int) int {
			return 100 * (1 << attempt) // 指数退避
		},
		ShouldRetry: func(err error) bool {
			return err != io.EOF && err != context.Canceled
		},
	}
}

// Recover 创建带错误恢复的流
func Recover[T any](sr *StreamReader[T], config *RecoveryConfig) *StreamReader[T] {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	return &StreamReader[T]{
		typ: readerTypePipe,
		pipe: &pipeReader[T]{
			ch:   make(chan T, 10),
			done: make(chan struct{}),
		},
		source: sr.source,
	}
}

// OnError 添加错误处理
func OnError[T any](sr *StreamReader[T], handler func(error) error) *StreamReader[T] {
	reader, writer := Pipe[T](10)
	go func() {
		defer writer.Close()
		for {
			item, err := sr.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				if newErr := handler(err); newErr != nil {
					writer.CloseWithError(newErr)
					return
				}
				continue
			}
			if err := writer.Send(item); err != nil {
				return
			}
		}
	}()
	return reader
}

// OnErrorResume 错误时切换到备用流
func OnErrorResume[T any](sr *StreamReader[T], fallback func(error) *StreamReader[T]) *StreamReader[T] {
	reader, writer := Pipe[T](10)
	go func() {
		defer writer.Close()
		for {
			item, err := sr.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				// 切换到备用流
				backup := fallback(err)
				if backup != nil {
					for {
						backupItem, backupErr := backup.Recv()
						if backupErr == io.EOF {
							return
						}
						if backupErr != nil {
							writer.CloseWithError(backupErr)
							return
						}
						if err := writer.Send(backupItem); err != nil {
							return
						}
					}
				}
				return
			}
			if err := writer.Send(item); err != nil {
				return
			}
		}
	}()
	return reader
}
