// Package replay 提供调用链重放功能
//
// 本包实现调用链录制和重放：
//   - 录制：记录完整的调用链
//   - 重放：重新执行已录制的调用链
//   - 比对：对比重放结果与原始结果
//   - 断点：支持断点调试
//
// 设计借鉴：
//   - LangSmith: Trace & Replay
//   - Postman: Collection Runner
//   - VCR: HTTP 录制回放
package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ============== 核心类型 ==============

// Trace 调用链追踪
type Trace struct {
	// ID 追踪 ID
	ID string `json:"id"`

	// Name 追踪名称
	Name string `json:"name,omitempty"`

	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`

	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`

	// Duration 总耗时
	Duration time.Duration `json:"duration"`

	// Status 状态
	Status TraceStatus `json:"status"`

	// Error 错误信息
	Error string `json:"error,omitempty"`

	// Spans 调用链
	Spans []*Span `json:"spans"`

	// Input 输入
	Input any `json:"input,omitempty"`

	// Output 输出
	Output any `json:"output,omitempty"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TraceStatus 追踪状态
type TraceStatus string

const (
	// StatusPending 等待中
	StatusPending TraceStatus = "pending"

	// StatusRunning 运行中
	StatusRunning TraceStatus = "running"

	// StatusCompleted 已完成
	StatusCompleted TraceStatus = "completed"

	// StatusFailed 失败
	StatusFailed TraceStatus = "failed"
)

// Span 调用跨度
type Span struct {
	// ID 跨度 ID
	ID string `json:"id"`

	// TraceID 所属追踪 ID
	TraceID string `json:"trace_id"`

	// ParentID 父跨度 ID
	ParentID string `json:"parent_id,omitempty"`

	// Name 名称
	Name string `json:"name"`

	// Type 类型
	Type SpanType `json:"type"`

	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`

	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`

	// Duration 耗时
	Duration time.Duration `json:"duration"`

	// Status 状态
	Status SpanStatus `json:"status"`

	// Input 输入
	Input any `json:"input,omitempty"`

	// Output 输出
	Output any `json:"output,omitempty"`

	// Error 错误
	Error string `json:"error,omitempty"`

	// Attributes 属性
	Attributes map[string]any `json:"attributes,omitempty"`

	// Events 事件
	Events []SpanEvent `json:"events,omitempty"`
}

// SpanType 跨度类型
type SpanType string

const (
	// SpanTypeLLM LLM 调用
	SpanTypeLLM SpanType = "llm"

	// SpanTypeTool 工具调用
	SpanTypeTool SpanType = "tool"

	// SpanTypeRetriever 检索调用
	SpanTypeRetriever SpanType = "retriever"

	// SpanTypeAgent Agent 执行
	SpanTypeAgent SpanType = "agent"

	// SpanTypeChain 链执行
	SpanTypeChain SpanType = "chain"

	// SpanTypeCustom 自定义
	SpanTypeCustom SpanType = "custom"
)

// SpanStatus 跨度状态
type SpanStatus string

const (
	// SpanStatusOK 成功
	SpanStatusOK SpanStatus = "ok"

	// SpanStatusError 错误
	SpanStatusError SpanStatus = "error"
)

// SpanEvent 跨度事件
type SpanEvent struct {
	// Name 事件名称
	Name string `json:"name"`

	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`

	// Attributes 属性
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ============== 录制器 ==============

// Recorder 调用链录制器
type Recorder struct {
	// 当前追踪
	currentTrace *Trace

	// 跨度栈
	spanStack []*Span

	// 存储
	storage TraceStorage

	// 配置
	config RecorderConfig

	mu sync.Mutex
}

// RecorderConfig 录制器配置
type RecorderConfig struct {
	// MaxSpans 最大跨度数
	MaxSpans int

	// RecordInput 是否录制输入
	RecordInput bool

	// RecordOutput 是否录制输出
	RecordOutput bool

	// SensitiveFields 敏感字段（会被脱敏）
	SensitiveFields []string

	// ErrorHandler 错误处理函数（用于处理存储失败等非致命错误）
	// 如果为 nil，错误将被静默忽略
	ErrorHandler func(err error)
}

// DefaultRecorderConfig 默认配置
var DefaultRecorderConfig = RecorderConfig{
	MaxSpans:     1000,
	RecordInput:  true,
	RecordOutput: true,
}

// NewRecorder 创建录制器
func NewRecorder(storage TraceStorage, config ...RecorderConfig) *Recorder {
	cfg := DefaultRecorderConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return &Recorder{
		storage: storage,
		config:  cfg,
	}
}

// StartTrace 开始追踪
func (r *Recorder) StartTrace(name string, input any) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := generateID("trace")
	r.currentTrace = &Trace{
		ID:        id,
		Name:      name,
		StartTime: time.Now(),
		Status:    StatusRunning,
		Spans:     make([]*Span, 0),
		Metadata:  make(map[string]any),
	}

	if r.config.RecordInput {
		r.currentTrace.Input = r.sanitize(input)
	}

	return id
}

// EndTrace 结束追踪
func (r *Recorder) EndTrace(output any, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentTrace == nil {
		return
	}

	r.currentTrace.EndTime = time.Now()
	r.currentTrace.Duration = r.currentTrace.EndTime.Sub(r.currentTrace.StartTime)

	if err != nil {
		r.currentTrace.Status = StatusFailed
		r.currentTrace.Error = err.Error()
	} else {
		r.currentTrace.Status = StatusCompleted
	}

	if r.config.RecordOutput {
		r.currentTrace.Output = r.sanitize(output)
	}

	// 保存追踪
	if r.storage != nil {
		if err := r.storage.Save(context.Background(), r.currentTrace); err != nil {
			// 使用错误处理函数处理存储失败
			if r.config.ErrorHandler != nil {
				r.config.ErrorHandler(fmt.Errorf("failed to save trace: %w", err))
			}
		}
	}

	r.currentTrace = nil
	r.spanStack = nil
}

// StartSpan 开始跨度
func (r *Recorder) StartSpan(name string, spanType SpanType, input any) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentTrace == nil {
		return ""
	}

	if len(r.currentTrace.Spans) >= r.config.MaxSpans {
		return ""
	}

	id := generateID("span")
	span := &Span{
		ID:         id,
		TraceID:    r.currentTrace.ID,
		Name:       name,
		Type:       spanType,
		StartTime:  time.Now(),
		Status:     SpanStatusOK,
		Attributes: make(map[string]any),
	}

	if len(r.spanStack) > 0 {
		span.ParentID = r.spanStack[len(r.spanStack)-1].ID
	}

	if r.config.RecordInput {
		span.Input = r.sanitize(input)
	}

	r.currentTrace.Spans = append(r.currentTrace.Spans, span)
	r.spanStack = append(r.spanStack, span)

	return id
}

// EndSpan 结束跨度
func (r *Recorder) EndSpan(output any, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.spanStack) == 0 {
		return
	}

	span := r.spanStack[len(r.spanStack)-1]
	r.spanStack = r.spanStack[:len(r.spanStack)-1]

	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)

	if err != nil {
		span.Status = SpanStatusError
		span.Error = err.Error()
	}

	if r.config.RecordOutput {
		span.Output = r.sanitize(output)
	}
}

// AddAttribute 添加属性
func (r *Recorder) AddAttribute(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.spanStack) == 0 {
		return
	}

	span := r.spanStack[len(r.spanStack)-1]
	span.Attributes[key] = value
}

// AddEvent 添加事件
func (r *Recorder) AddEvent(name string, attrs map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.spanStack) == 0 {
		return
	}

	span := r.spanStack[len(r.spanStack)-1]
	span.Events = append(span.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// GetCurrentTrace 获取当前追踪
func (r *Recorder) GetCurrentTrace() *Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentTrace
}

// sanitize 脱敏处理
func (r *Recorder) sanitize(data any) any {
	if len(r.config.SensitiveFields) == 0 {
		return data
	}

	// 转换为 map 进行处理
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var dataMap map[string]any
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		return data
	}

	// 脱敏处理
	for _, field := range r.config.SensitiveFields {
		if _, exists := dataMap[field]; exists {
			dataMap[field] = "***REDACTED***"
		}
	}

	return dataMap
}

// ============== 重放器 ==============

// Replayer 调用链重放器
type Replayer struct {
	// 存储
	storage TraceStorage

	// 执行器
	executor Executor

	// 配置
	config ReplayerConfig
}

// ReplayerConfig 重放器配置
type ReplayerConfig struct {
	// StopOnError 遇错停止
	StopOnError bool

	// Breakpoints 断点（跨度名称）
	Breakpoints []string

	// BreakpointHandler 断点处理器
	BreakpointHandler func(span *Span) bool

	// ModifyInput 修改输入
	ModifyInput func(span *Span) any

	// CompareOutput 比较输出
	CompareOutput func(expected, actual any) bool
}

// Executor 执行器接口
type Executor interface {
	// Execute 执行跨度
	Execute(ctx context.Context, span *Span) (any, error)
}

// ExecutorFunc 函数执行器
type ExecutorFunc func(ctx context.Context, span *Span) (any, error)

// Execute 实现 Executor 接口
func (f ExecutorFunc) Execute(ctx context.Context, span *Span) (any, error) {
	return f(ctx, span)
}

// NewReplayer 创建重放器
func NewReplayer(storage TraceStorage, executor Executor, config ...ReplayerConfig) *Replayer {
	cfg := ReplayerConfig{}
	if len(config) > 0 {
		cfg = config[0]
	}

	return &Replayer{
		storage:  storage,
		executor: executor,
		config:   cfg,
	}
}

// ReplayResult 重放结果
type ReplayResult struct {
	// TraceID 原始追踪 ID
	TraceID string `json:"trace_id"`

	// Success 是否成功
	Success bool `json:"success"`

	// SpanResults 各跨度结果
	SpanResults []*SpanResult `json:"span_results"`

	// Duration 总耗时
	Duration time.Duration `json:"duration"`

	// Error 错误
	Error string `json:"error,omitempty"`
}

// SpanResult 跨度重放结果
type SpanResult struct {
	// SpanID 跨度 ID
	SpanID string `json:"span_id"`

	// Name 跨度名称
	Name string `json:"name"`

	// Success 是否成功
	Success bool `json:"success"`

	// ExpectedOutput 预期输出
	ExpectedOutput any `json:"expected_output,omitempty"`

	// ActualOutput 实际输出
	ActualOutput any `json:"actual_output,omitempty"`

	// OutputMatch 输出是否匹配
	OutputMatch bool `json:"output_match"`

	// Error 错误
	Error string `json:"error,omitempty"`

	// Duration 耗时
	Duration time.Duration `json:"duration"`
}

// Replay 重放追踪
func (r *Replayer) Replay(ctx context.Context, traceID string) (*ReplayResult, error) {
	// 加载追踪
	trace, err := r.storage.Load(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load trace: %w", err)
	}

	return r.ReplayTrace(ctx, trace)
}

// ReplayTrace 重放追踪对象
func (r *Replayer) ReplayTrace(ctx context.Context, trace *Trace) (*ReplayResult, error) {
	result := &ReplayResult{
		TraceID:     trace.ID,
		Success:     true,
		SpanResults: make([]*SpanResult, 0, len(trace.Spans)),
	}

	startTime := time.Now()

	for _, span := range trace.Spans {
		// 检查断点
		if r.shouldBreak(span) {
			if r.config.BreakpointHandler != nil {
				if !r.config.BreakpointHandler(span) {
					break // 用户中断
				}
			}
		}

		// 修改输入（如果需要）
		input := span.Input
		if r.config.ModifyInput != nil {
			input = r.config.ModifyInput(span)
		}

		// 执行
		spanCopy := *span
		spanCopy.Input = input

		spanStart := time.Now()
		output, err := r.executor.Execute(ctx, &spanCopy)
		spanDuration := time.Since(spanStart)

		// 记录结果
		spanResult := &SpanResult{
			SpanID:         span.ID,
			Name:           span.Name,
			ExpectedOutput: span.Output,
			ActualOutput:   output,
			Duration:       spanDuration,
		}

		if err != nil {
			spanResult.Success = false
			spanResult.Error = err.Error()
			result.Success = false

			if r.config.StopOnError {
				result.SpanResults = append(result.SpanResults, spanResult)
				result.Error = fmt.Sprintf("span %s failed: %s", span.Name, err)
				break
			}
		} else {
			spanResult.Success = true

			// 比较输出
			if r.config.CompareOutput != nil {
				spanResult.OutputMatch = r.config.CompareOutput(span.Output, output)
			} else {
				spanResult.OutputMatch = r.defaultCompare(span.Output, output)
			}
		}

		result.SpanResults = append(result.SpanResults, spanResult)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// shouldBreak 是否应该断点
func (r *Replayer) shouldBreak(span *Span) bool {
	for _, bp := range r.config.Breakpoints {
		if span.Name == bp {
			return true
		}
	}
	return false
}

// defaultCompare 默认比较
func (r *Replayer) defaultCompare(expected, actual any) bool {
	expectedJSON, _ := json.Marshal(expected)
	actualJSON, _ := json.Marshal(actual)
	return string(expectedJSON) == string(actualJSON)
}

// ============== 存储接口 ==============

// TraceStorage 追踪存储接口
type TraceStorage interface {
	// Save 保存追踪
	Save(ctx context.Context, trace *Trace) error

	// Load 加载追踪
	Load(ctx context.Context, id string) (*Trace, error)

	// List 列出追踪
	List(ctx context.Context, filter TraceFilter) ([]*Trace, error)

	// Delete 删除追踪
	Delete(ctx context.Context, id string) error
}

// TraceFilter 追踪过滤器
type TraceFilter struct {
	// Name 名称匹配
	Name string

	// Status 状态
	Status TraceStatus

	// StartTime 开始时间范围
	StartTime *time.Time

	// EndTime 结束时间范围
	EndTime *time.Time

	// Limit 限制数量
	Limit int
}

// ============== 内存存储 ==============

// MemoryStorage 内存存储
type MemoryStorage struct {
	traces map[string]*Trace
	mu     sync.RWMutex
}

// NewMemoryStorage 创建内存存储
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		traces: make(map[string]*Trace),
	}
}

// Save 保存追踪
func (s *MemoryStorage) Save(ctx context.Context, trace *Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces[trace.ID] = trace
	return nil
}

// Load 加载追踪
func (s *MemoryStorage) Load(ctx context.Context, id string) (*Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trace, exists := s.traces[id]
	if !exists {
		return nil, fmt.Errorf("trace not found: %s", id)
	}
	return trace, nil
}

// List 列出追踪
func (s *MemoryStorage) List(ctx context.Context, filter TraceFilter) ([]*Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Trace
	for _, trace := range s.traces {
		if filter.Name != "" && trace.Name != filter.Name {
			continue
		}
		if filter.Status != "" && trace.Status != filter.Status {
			continue
		}
		if filter.StartTime != nil && trace.StartTime.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && trace.EndTime.After(*filter.EndTime) {
			continue
		}

		result = append(result, trace)

		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result, nil
}

// Delete 删除追踪
func (s *MemoryStorage) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.traces, id)
	return nil
}

// ============== 文件存储 ==============

// FileStorage 文件存储
type FileStorage struct {
	dir string
}

// NewFileStorage 创建文件存储
func NewFileStorage(dir string) *FileStorage {
	_ = os.MkdirAll(dir, 0755)
	return &FileStorage{dir: dir}
}

// Save 保存追踪
func (s *FileStorage) Save(ctx context.Context, trace *Trace) error {
	path := fmt.Sprintf("%s/%s.json", s.dir, trace.ID)
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load 加载追踪
func (s *FileStorage) Load(ctx context.Context, id string) (*Trace, error) {
	path := fmt.Sprintf("%s/%s.json", s.dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, err
	}
	return &trace, nil
}

// List 列出追踪
func (s *FileStorage) List(ctx context.Context, filter TraceFilter) ([]*Trace, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var result []*Trace
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		id := entry.Name()
		if len(id) > 5 && id[len(id)-5:] == ".json" {
			id = id[:len(id)-5]
		}

		trace, err := s.Load(ctx, id)
		if err != nil {
			continue
		}

		// 应用过滤器
		if filter.Name != "" && trace.Name != filter.Name {
			continue
		}
		if filter.Status != "" && trace.Status != filter.Status {
			continue
		}

		result = append(result, trace)

		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result, nil
}

// Delete 删除追踪
func (s *FileStorage) Delete(ctx context.Context, id string) error {
	path := fmt.Sprintf("%s/%s.json", s.dir, id)
	return os.Remove(path)
}

// ============== 导出导入 ==============

// Export 导出追踪
func Export(trace *Trace, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(trace)
}

// Import 导入追踪
func Import(r io.Reader) (*Trace, error) {
	var trace Trace
	if err := json.NewDecoder(r).Decode(&trace); err != nil {
		return nil, err
	}
	return &trace, nil
}

// ============== 辅助函数 ==============

var idCounter int64
var idMu sync.Mutex

func generateID(prefix string) string {
	idMu.Lock()
	idCounter++
	id := idCounter
	idMu.Unlock()
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), id)
}
