package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSpanKind_Constants(t *testing.T) {
	// 确保常量值正确
	if SpanKindInternal != 0 {
		t.Errorf("expected SpanKindInternal = 0, got %d", SpanKindInternal)
	}
	if SpanKindAgent != 1 {
		t.Errorf("expected SpanKindAgent = 1, got %d", SpanKindAgent)
	}
	if SpanKindLLM != 2 {
		t.Errorf("expected SpanKindLLM = 2, got %d", SpanKindLLM)
	}
	if SpanKindTool != 3 {
		t.Errorf("expected SpanKindTool = 3, got %d", SpanKindTool)
	}
	if SpanKindRetrieval != 4 {
		t.Errorf("expected SpanKindRetrieval = 4, got %d", SpanKindRetrieval)
	}
	if SpanKindEmbedding != 5 {
		t.Errorf("expected SpanKindEmbedding = 5, got %d", SpanKindEmbedding)
	}
}

func TestStatusCode_Constants(t *testing.T) {
	if StatusCodeUnset != 0 {
		t.Errorf("expected StatusCodeUnset = 0, got %d", StatusCodeUnset)
	}
	if StatusCodeOK != 1 {
		t.Errorf("expected StatusCodeOK = 1, got %d", StatusCodeOK)
	}
	if StatusCodeError != 2 {
		t.Errorf("expected StatusCodeError = 2, got %d", StatusCodeError)
	}
}

func TestTokenUsage(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens = 100, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("expected CompletionTokens = 50, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("expected TotalTokens = 150, got %d", usage.TotalTokens)
	}
}

func TestSpanConfig_WithOptions(t *testing.T) {
	config := &SpanConfig{}

	// 测试 WithSpanKind
	WithSpanKind(SpanKindLLM)(config)
	if config.Kind != SpanKindLLM {
		t.Errorf("expected Kind = SpanKindLLM, got %d", config.Kind)
	}

	// 测试 WithAttributes
	attrs := map[string]any{"key": "value"}
	WithAttributes(attrs)(config)
	if config.Attributes["key"] != "value" {
		t.Error("expected Attributes to contain key=value")
	}
}

func TestContextWithSpan(t *testing.T) {
	ctx := context.Background()

	// 测试从空 context 获取
	span := SpanFromContext(ctx)
	if span != nil {
		t.Error("expected nil span from empty context")
	}
}

func TestContextWithTracer(t *testing.T) {
	ctx := context.Background()

	// 测试从空 context 获取
	tracer := TracerFromContext(ctx)
	if tracer != nil {
		t.Error("expected nil tracer from empty context")
	}
}

func TestAttributeKeys(t *testing.T) {
	// 验证常用属性键
	expectedKeys := map[string]string{
		"AttrAgentID":             "agent.id",
		"AttrAgentName":           "agent.name",
		"AttrAgentRole":           "agent.role",
		"AttrLLMProvider":         "llm.provider",
		"AttrLLMModel":            "llm.model",
		"AttrLLMPromptTokens":     "llm.prompt_tokens",
		"AttrLLMCompletionTokens": "llm.completion_tokens",
		"AttrLLMTotalTokens":      "llm.total_tokens",
		"AttrToolName":            "tool.name",
		"AttrToolArguments":       "tool.arguments",
		"AttrToolResult":          "tool.result",
		"AttrRetrievalQuery":      "retrieval.query",
		"AttrErrorType":           "error.type",
		"AttrErrorMessage":        "error.message",
	}

	actualKeys := map[string]string{
		"AttrAgentID":             AttrAgentID,
		"AttrAgentName":           AttrAgentName,
		"AttrAgentRole":           AttrAgentRole,
		"AttrLLMProvider":         AttrLLMProvider,
		"AttrLLMModel":            AttrLLMModel,
		"AttrLLMPromptTokens":     AttrLLMPromptTokens,
		"AttrLLMCompletionTokens": AttrLLMCompletionTokens,
		"AttrLLMTotalTokens":      AttrLLMTotalTokens,
		"AttrToolName":            AttrToolName,
		"AttrToolArguments":       AttrToolArguments,
		"AttrToolResult":          AttrToolResult,
		"AttrRetrievalQuery":      AttrRetrievalQuery,
		"AttrErrorType":           AttrErrorType,
		"AttrErrorMessage":        AttrErrorMessage,
	}

	for name, expected := range expectedKeys {
		actual := actualKeys[name]
		if actual != expected {
			t.Errorf("%s: expected %q, got %q", name, expected, actual)
		}
	}
}

// ============================================================================
// DefaultSpan 测试
// ============================================================================

// TestNewSpan_Basic 测试基础创建
func TestNewSpan_Basic(t *testing.T) {
	span := NewSpan("test.op", "trace-123")

	if span.SpanID() == "" {
		t.Error("SpanID 不应为空")
	}
	if span.TraceID() != "trace-123" {
		t.Errorf("TraceID: 期望 %q, 得到 %q", "trace-123", span.TraceID())
	}
	if span.ParentID() != "" {
		t.Errorf("ParentID: 期望空字符串, 得到 %q", span.ParentID())
	}
	if !span.IsRecording() {
		t.Error("新创建的 Span 应处于 recording 状态")
	}
}

// TestNewSpan_WithSpanKind 测试创建时指定 SpanKind
func TestNewSpan_WithSpanKind(t *testing.T) {
	span := NewSpan("agent.run", "trace-1", WithSpanKind(SpanKindAgent))
	data := span.Export()
	if data.Kind != "agent" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "agent", data.Kind)
	}
}

// TestNewSpan_WithAttributes 测试创建时附带属性
func TestNewSpan_WithAttributes(t *testing.T) {
	attrs := map[string]any{"key1": "val1", "key2": 42}
	span := NewSpan("op", "trace-1", WithAttributes(attrs))
	got := span.Attributes()

	if got["key1"] != "val1" {
		t.Errorf("key1: 期望 %q, 得到 %v", "val1", got["key1"])
	}
	if got["key2"] != 42 {
		t.Errorf("key2: 期望 %d, 得到 %v", 42, got["key2"])
	}
}

// TestNewSpan_WithParent 测试创建时指定父 Span
func TestNewSpan_WithParent(t *testing.T) {
	parent := NewSpan("parent", "trace-1")
	child := NewSpan("child", "trace-1", WithParent(parent))

	if child.ParentID() != parent.SpanID() {
		t.Errorf("ParentID: 期望 %q, 得到 %q", parent.SpanID(), child.ParentID())
	}
}

// TestNewSpan_WithStartTime 测试创建时指定开始时间
func TestNewSpan_WithStartTime(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	span := NewSpan("op", "trace-1", WithStartTime(fixedTime))
	data := span.Export()

	if !data.StartTime.Equal(fixedTime) {
		t.Errorf("StartTime: 期望 %v, 得到 %v", fixedTime, data.StartTime)
	}
}

// TestDefaultSpan_SpanID_TraceID_ParentID 测试 ID 访问器
func TestDefaultSpan_SpanID_TraceID_ParentID(t *testing.T) {
	parent := NewSpan("parent", "trace-abc")
	child := NewSpan("child", "trace-abc", WithParent(parent))

	// SpanID 非空且唯一
	if parent.SpanID() == "" || child.SpanID() == "" {
		t.Error("SpanID 不应为空")
	}
	if parent.SpanID() == child.SpanID() {
		t.Error("不同 Span 的 SpanID 应该不同")
	}

	// TraceID 一致
	if parent.TraceID() != "trace-abc" || child.TraceID() != "trace-abc" {
		t.Error("TraceID 应与传入值一致")
	}

	// ParentID
	if parent.ParentID() != "" {
		t.Error("根 Span 的 ParentID 应为空")
	}
	if child.ParentID() != parent.SpanID() {
		t.Errorf("子 Span 的 ParentID 应为父 Span 的 SpanID")
	}
}

// TestDefaultSpan_SetName 测试修改名称
func TestDefaultSpan_SetName(t *testing.T) {
	span := NewSpan("original", "trace-1")
	span.SetName("updated")
	data := span.Export()
	if data.Name != "updated" {
		t.Errorf("Name: 期望 %q, 得到 %q", "updated", data.Name)
	}
}

// TestDefaultSpan_SetInput_SetOutput 测试设置输入输出
func TestDefaultSpan_SetInput_SetOutput(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.SetInput("hello input")
	span.SetOutput(map[string]int{"count": 5})

	data := span.Export()
	if data.Input != "hello input" {
		t.Errorf("Input: 期望 %q, 得到 %v", "hello input", data.Input)
	}
	outputMap, ok := data.Output.(map[string]int)
	if !ok {
		t.Fatalf("Output 类型错误: %T", data.Output)
	}
	if outputMap["count"] != 5 {
		t.Errorf("Output count: 期望 5, 得到 %d", outputMap["count"])
	}
}

// TestDefaultSpan_SetTokenUsage 测试设置 Token 使用量及自动属性
func TestDefaultSpan_SetTokenUsage(t *testing.T) {
	span := NewSpan("llm.call", "trace-1")
	usage := TokenUsage{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
	}
	span.SetTokenUsage(usage)

	data := span.Export()
	if data.TokenUsage != usage {
		t.Errorf("TokenUsage: 期望 %+v, 得到 %+v", usage, data.TokenUsage)
	}

	// 验证自动设置的属性
	attrs := span.Attributes()
	if attrs[AttrLLMPromptTokens] != 200 {
		t.Errorf("属性 %s: 期望 200, 得到 %v", AttrLLMPromptTokens, attrs[AttrLLMPromptTokens])
	}
	if attrs[AttrLLMCompletionTokens] != 100 {
		t.Errorf("属性 %s: 期望 100, 得到 %v", AttrLLMCompletionTokens, attrs[AttrLLMCompletionTokens])
	}
	if attrs[AttrLLMTotalTokens] != 300 {
		t.Errorf("属性 %s: 期望 300, 得到 %v", AttrLLMTotalTokens, attrs[AttrLLMTotalTokens])
	}
}

// TestDefaultSpan_SetAttribute 测试单个属性设置
func TestDefaultSpan_SetAttribute(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.SetAttribute("foo", "bar")
	span.SetAttribute("num", 42)

	attrs := span.Attributes()
	if attrs["foo"] != "bar" {
		t.Errorf("foo: 期望 %q, 得到 %v", "bar", attrs["foo"])
	}
	if attrs["num"] != 42 {
		t.Errorf("num: 期望 42, 得到 %v", attrs["num"])
	}
}

// TestDefaultSpan_SetAttributes 测试批量属性设置
func TestDefaultSpan_SetAttributes(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.SetAttribute("existing", "keep")
	span.SetAttributes(map[string]any{
		"a": 1,
		"b": "two",
	})

	attrs := span.Attributes()
	if attrs["existing"] != "keep" {
		t.Error("已有属性应被保留")
	}
	if attrs["a"] != 1 {
		t.Errorf("a: 期望 1, 得到 %v", attrs["a"])
	}
	if attrs["b"] != "two" {
		t.Errorf("b: 期望 %q, 得到 %v", "two", attrs["b"])
	}
}

// TestDefaultSpan_AddEvent_Basic 测试添加基础事件
func TestDefaultSpan_AddEvent_Basic(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.AddEvent("event1")

	events := span.Events()
	if len(events) != 1 {
		t.Fatalf("事件数量: 期望 1, 得到 %d", len(events))
	}
	if events[0].Name != "event1" {
		t.Errorf("事件名称: 期望 %q, 得到 %q", "event1", events[0].Name)
	}
	if events[0].Time.IsZero() {
		t.Error("事件时间不应为零值")
	}
}

// TestDefaultSpan_AddEvent_WithAttributes 测试添加带属性的事件
func TestDefaultSpan_AddEvent_WithAttributes(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.AddEvent("event-with-attrs", "key1", "val1", "key2", 99)

	events := span.Events()
	if len(events) != 1 {
		t.Fatalf("事件数量: 期望 1, 得到 %d", len(events))
	}
	if events[0].Attributes["key1"] != "val1" {
		t.Errorf("事件属性 key1: 期望 %q, 得到 %v", "val1", events[0].Attributes["key1"])
	}
	if events[0].Attributes["key2"] != 99 {
		t.Errorf("事件属性 key2: 期望 99, 得到 %v", events[0].Attributes["key2"])
	}
}

// TestDefaultSpan_AddEvent_Multiple 测试添加多个事件
func TestDefaultSpan_AddEvent_Multiple(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.AddEvent("first")
	span.AddEvent("second")
	span.AddEvent("third")

	events := span.Events()
	if len(events) != 3 {
		t.Fatalf("事件数量: 期望 3, 得到 %d", len(events))
	}
	if events[0].Name != "first" || events[1].Name != "second" || events[2].Name != "third" {
		t.Error("事件顺序不正确")
	}
}

// TestDefaultSpan_RecordError 测试记录错误
func TestDefaultSpan_RecordError(t *testing.T) {
	span := NewSpan("op", "trace-1")
	testErr := errors.New("test error")
	span.RecordError(testErr)

	// 验证状态设置为错误
	data := span.Export()
	if data.Status.Code != StatusCodeError {
		t.Errorf("状态码: 期望 StatusCodeError, 得到 %d", data.Status.Code)
	}
	if data.Status.Message != "test error" {
		t.Errorf("状态消息: 期望 %q, 得到 %q", "test error", data.Status.Message)
	}

	// 验证错误属性
	attrs := span.Attributes()
	if attrs[AttrErrorType] != "error" {
		t.Errorf("error.type: 期望 %q, 得到 %v", "error", attrs[AttrErrorType])
	}
	if attrs[AttrErrorMessage] != "test error" {
		t.Errorf("error.message: 期望 %q, 得到 %v", "test error", attrs[AttrErrorMessage])
	}

	// 验证异常事件
	events := span.Events()
	if len(events) != 1 {
		t.Fatalf("事件数量: 期望 1, 得到 %d", len(events))
	}
	if events[0].Name != "exception" {
		t.Errorf("事件名称: 期望 %q, 得到 %q", "exception", events[0].Name)
	}
	if events[0].Attributes["exception.message"] != "test error" {
		t.Errorf("exception.message: 期望 %q, 得到 %v", "test error", events[0].Attributes["exception.message"])
	}
}

// TestDefaultSpan_RecordError_Nil 测试记录 nil 错误不执行任何操作
func TestDefaultSpan_RecordError_Nil(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.RecordError(nil)

	data := span.Export()
	if data.Status.Code != StatusCodeUnset {
		t.Errorf("nil 错误不应改变状态码, 得到 %d", data.Status.Code)
	}
	events := span.Events()
	if len(events) != 0 {
		t.Errorf("nil 错误不应添加事件, 得到 %d 个事件", len(events))
	}
}

// TestDefaultSpan_SetStatus 测试设置状态码和消息
func TestDefaultSpan_SetStatus(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.SetStatus(StatusCodeOK, "all good")

	data := span.Export()
	if data.Status.Code != StatusCodeOK {
		t.Errorf("状态码: 期望 StatusCodeOK, 得到 %d", data.Status.Code)
	}
	if data.Status.Message != "all good" {
		t.Errorf("状态消息: 期望 %q, 得到 %q", "all good", data.Status.Message)
	}
}

// TestDefaultSpan_End 测试结束 Span
func TestDefaultSpan_End(t *testing.T) {
	span := NewSpan("op", "trace-1")
	if !span.IsRecording() {
		t.Error("结束前应处于 recording 状态")
	}

	span.End()

	if span.IsRecording() {
		t.Error("结束后 recording 应为 false")
	}

	data := span.Export()
	if data.EndTime.IsZero() {
		t.Error("结束后 EndTime 不应为零值")
	}
}

// TestDefaultSpan_EndWithError 测试结束并记录错误
func TestDefaultSpan_EndWithError(t *testing.T) {
	span := NewSpan("op", "trace-1")
	testErr := errors.New("fatal error")
	span.EndWithError(testErr)

	// 验证同时完成了结束和错误记录
	if span.IsRecording() {
		t.Error("EndWithError 后 recording 应为 false")
	}
	data := span.Export()
	if data.Status.Code != StatusCodeError {
		t.Errorf("状态码: 期望 StatusCodeError, 得到 %d", data.Status.Code)
	}
	if data.EndTime.IsZero() {
		t.Error("EndTime 不应为零值")
	}
}

// TestDefaultSpan_IsRecording 测试 recording 状态变化
func TestDefaultSpan_IsRecording(t *testing.T) {
	span := NewSpan("op", "trace-1")
	if !span.IsRecording() {
		t.Error("初始状态应为 true")
	}
	span.End()
	if span.IsRecording() {
		t.Error("End 后应为 false")
	}
}

// TestDefaultSpan_Duration_InProgress 测试进行中 Span 的持续时间
func TestDefaultSpan_Duration_InProgress(t *testing.T) {
	start := time.Now()
	span := NewSpan("op", "trace-1", WithStartTime(start))

	// 等待一小段时间确保 Duration > 0
	time.Sleep(5 * time.Millisecond)
	d := span.Duration()

	if d < 5*time.Millisecond {
		t.Errorf("进行中 Span 的 Duration 应 >= 5ms, 得到 %v", d)
	}
}

// TestDefaultSpan_Duration_Ended 测试已结束 Span 的持续时间
func TestDefaultSpan_Duration_Ended(t *testing.T) {
	start := time.Now()
	span := NewSpan("op", "trace-1", WithStartTime(start))
	time.Sleep(10 * time.Millisecond)
	span.End()

	d := span.Duration()
	if d < 10*time.Millisecond {
		t.Errorf("已结束 Span 的 Duration 应 >= 10ms, 得到 %v", d)
	}

	// 已结束的 Span，Duration 应固定不变
	time.Sleep(5 * time.Millisecond)
	d2 := span.Duration()
	if d != d2 {
		t.Errorf("已结束 Span 的 Duration 应固定, 第一次 %v, 第二次 %v", d, d2)
	}
}

// TestDefaultSpan_Attributes_ReturnsCopy 测试 Attributes 返回副本
func TestDefaultSpan_Attributes_ReturnsCopy(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.SetAttribute("key", "original")

	attrs := span.Attributes()
	attrs["key"] = "modified"

	// 修改副本不应影响原始值
	attrs2 := span.Attributes()
	if attrs2["key"] != "original" {
		t.Errorf("Attributes 应返回副本, 原始值不应被修改, 得到 %v", attrs2["key"])
	}
}

// TestDefaultSpan_Events_ReturnsCopy 测试 Events 返回副本
func TestDefaultSpan_Events_ReturnsCopy(t *testing.T) {
	span := NewSpan("op", "trace-1")
	span.AddEvent("event1")

	events := span.Events()
	if len(events) != 1 {
		t.Fatalf("事件数量: 期望 1, 得到 %d", len(events))
	}

	// 修改副本后再获取，长度不应变
	events = append(events, SpanEvent{Name: "fake"})
	events2 := span.Events()
	if len(events2) != 1 {
		t.Errorf("Events 应返回副本, 原始长度不应被修改, 得到 %d", len(events2))
	}
}

// TestDefaultSpan_Export 测试导出完整 SpanData
func TestDefaultSpan_Export(t *testing.T) {
	parent := NewSpan("parent", "trace-export")
	span := NewSpan("child.op", "trace-export",
		WithSpanKind(SpanKindLLM),
		WithParent(parent),
		WithAttributes(map[string]any{"init": true}),
	)

	span.SetInput("input-data")
	span.SetOutput("output-data")
	span.SetTokenUsage(TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
	span.SetAttribute("extra", "value")
	span.AddEvent("processing")
	span.SetStatus(StatusCodeOK, "success")
	span.End()

	data := span.Export()

	if data.SpanID == "" {
		t.Error("SpanID 不应为空")
	}
	if data.TraceID != "trace-export" {
		t.Errorf("TraceID: 期望 %q, 得到 %q", "trace-export", data.TraceID)
	}
	if data.ParentID != parent.SpanID() {
		t.Errorf("ParentID: 期望 %q, 得到 %q", parent.SpanID(), data.ParentID)
	}
	if data.Name != "child.op" {
		t.Errorf("Name: 期望 %q, 得到 %q", "child.op", data.Name)
	}
	if data.Kind != "llm" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "llm", data.Kind)
	}
	if data.Input != "input-data" {
		t.Errorf("Input: 期望 %q, 得到 %v", "input-data", data.Input)
	}
	if data.Output != "output-data" {
		t.Errorf("Output: 期望 %q, 得到 %v", "output-data", data.Output)
	}
	if data.TokenUsage.TotalTokens != 15 {
		t.Errorf("TotalTokens: 期望 15, 得到 %d", data.TokenUsage.TotalTokens)
	}
	if data.Status.Code != StatusCodeOK {
		t.Errorf("Status.Code: 期望 StatusCodeOK, 得到 %d", data.Status.Code)
	}
	if data.Duration <= 0 {
		t.Errorf("Duration 应大于 0, 得到 %v", data.Duration)
	}
	if data.EndTime.IsZero() {
		t.Error("EndTime 不应为零值")
	}
	if len(data.Events) != 1 {
		t.Errorf("Events 数量: 期望 1, 得到 %d", len(data.Events))
	}
	if data.Attributes["init"] != true {
		t.Error("Attributes 应包含初始化属性")
	}
	if data.Attributes["extra"] != "value" {
		t.Error("Attributes 应包含额外属性")
	}
}

// TestDefaultSpan_ToJSON 测试 JSON 序列化
func TestDefaultSpan_ToJSON(t *testing.T) {
	span := NewSpan("json.test", "trace-json", WithSpanKind(SpanKindTool))
	span.SetAttribute("tool", "calculator")
	span.End()

	jsonBytes, err := span.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON 失败: %v", err)
	}

	// 验证是有效的 JSON
	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	// 验证关键字段存在
	if result["span_id"] == nil || result["span_id"] == "" {
		t.Error("JSON 中 span_id 不应为空")
	}
	if result["trace_id"] != "trace-json" {
		t.Errorf("JSON 中 trace_id: 期望 %q, 得到 %v", "trace-json", result["trace_id"])
	}
	if result["name"] != "json.test" {
		t.Errorf("JSON 中 name: 期望 %q, 得到 %v", "json.test", result["name"])
	}
	if result["kind"] != "tool" {
		t.Errorf("JSON 中 kind: 期望 %q, 得到 %v", "tool", result["kind"])
	}
}

// TestDefaultSpan_kindString 测试所有 SpanKind 到字符串的映射
func TestDefaultSpan_kindString(t *testing.T) {
	tests := []struct {
		kind     SpanKind
		expected string
	}{
		{SpanKindInternal, "internal"},
		{SpanKindAgent, "agent"},
		{SpanKindLLM, "llm"},
		{SpanKindTool, "tool"},
		{SpanKindRetrieval, "retrieval"},
		{SpanKindEmbedding, "embedding"},
		{SpanKind(999), "internal"}, // 未知类型默认为 internal
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			span := NewSpan("test", "trace-1", WithSpanKind(tt.kind))
			data := span.Export()
			if data.Kind != tt.expected {
				t.Errorf("SpanKind(%d): 期望 %q, 得到 %q", tt.kind, tt.expected, data.Kind)
			}
		})
	}
}

// TestDefaultSpan_Concurrent 测试并发安全性
func TestDefaultSpan_Concurrent(t *testing.T) {
	span := NewSpan("concurrent", "trace-1")
	var wg sync.WaitGroup
	n := 100

	// 并发设置属性
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			span.SetAttribute(fmt.Sprintf("key-%d", idx), idx)
		}(i)
	}

	// 并发添加事件
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			span.AddEvent(fmt.Sprintf("event-%d", idx))
		}(i)
	}

	// 并发读取
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = span.Attributes()
			_ = span.Events()
			_ = span.IsRecording()
			_ = span.Duration()
		}()
	}

	wg.Wait()

	// 验证最终状态
	attrs := span.Attributes()
	if len(attrs) != n {
		t.Errorf("属性数量: 期望 %d, 得到 %d", n, len(attrs))
	}
	events := span.Events()
	if len(events) != n {
		t.Errorf("事件数量: 期望 %d, 得到 %d", n, len(events))
	}
}

// ============================================================================
// MemoryTracer 测试
// ============================================================================

// TestNewMemoryTracer_Default 测试默认配置
func TestNewMemoryTracer_Default(t *testing.T) {
	mt := NewMemoryTracer()
	if mt.MaxSpans() != 10000 {
		t.Errorf("默认 MaxSpans: 期望 10000, 得到 %d", mt.MaxSpans())
	}
	if mt.Size() != 0 {
		t.Errorf("初始 Size: 期望 0, 得到 %d", mt.Size())
	}
}

// TestNewMemoryTracer_WithMaxSpans 测试自定义最大数量
func TestNewMemoryTracer_WithMaxSpans(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(100))
	if mt.MaxSpans() != 100 {
		t.Errorf("MaxSpans: 期望 100, 得到 %d", mt.MaxSpans())
	}
}

// TestNewMemoryTracer_WithMaxSpans_Invalid 测试无效最大数量 (<= 0 应忽略)
func TestNewMemoryTracer_WithMaxSpans_Invalid(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(0))
	if mt.MaxSpans() != 10000 {
		t.Errorf("无效 MaxSpans 应使用默认值, 得到 %d", mt.MaxSpans())
	}

	mt2 := NewMemoryTracer(WithMaxSpans(-5))
	if mt2.MaxSpans() != 10000 {
		t.Errorf("负数 MaxSpans 应使用默认值, 得到 %d", mt2.MaxSpans())
	}
}

// TestMemoryTracer_StartSpan_Basic 测试基本的 Span 创建和存储
func TestMemoryTracer_StartSpan_Basic(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	newCtx, span := mt.StartSpan(ctx, "test.op")
	if span == nil {
		t.Fatal("StartSpan 返回的 Span 不应为 nil")
	}
	if span.SpanID() == "" {
		t.Error("SpanID 不应为空")
	}
	if mt.Size() != 1 {
		t.Errorf("Size: 期望 1, 得到 %d", mt.Size())
	}

	// 验证 context 中包含 Span
	ctxSpan := SpanFromContext(newCtx)
	if ctxSpan == nil {
		t.Error("新 context 中应包含 Span")
	}
	if ctxSpan.SpanID() != span.SpanID() {
		t.Error("context 中的 Span 应与返回的 Span 一致")
	}
}

// TestMemoryTracer_StartSpan_InheritsParent 测试自动继承父 Span
func TestMemoryTracer_StartSpan_InheritsParent(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	// 创建父 Span
	ctx, parentSpan := mt.StartSpan(ctx, "parent")

	// 从包含父 Span 的 context 创建子 Span
	_, childSpan := mt.StartSpan(ctx, "child")

	// 子 Span 应以父 Span 为 parent
	childDefault, ok := childSpan.(*DefaultSpan)
	if !ok {
		t.Fatal("Span 应为 *DefaultSpan 类型")
	}
	if childDefault.ParentID() != parentSpan.SpanID() {
		t.Errorf("子 Span 的 ParentID: 期望 %q, 得到 %q", parentSpan.SpanID(), childDefault.ParentID())
	}
}

// TestMemoryTracer_Spans 测试返回所有 Span（从旧到新）
func TestMemoryTracer_Spans(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	// 创建 3 个 Span
	mt.StartSpan(ctx, "first")
	mt.StartSpan(ctx, "second")
	mt.StartSpan(ctx, "third")

	spans := mt.Spans()
	if len(spans) != 3 {
		t.Fatalf("Spans 数量: 期望 3, 得到 %d", len(spans))
	}

	// 验证顺序（从旧到新）
	data := spans[0].Export()
	if data.Name != "first" {
		t.Errorf("第一个 Span 名称: 期望 %q, 得到 %q", "first", data.Name)
	}
	data = spans[2].Export()
	if data.Name != "third" {
		t.Errorf("第三个 Span 名称: 期望 %q, 得到 %q", "third", data.Name)
	}
}

// TestMemoryTracer_Spans_Empty 测试空追踪器返回 nil
func TestMemoryTracer_Spans_Empty(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	spans := mt.Spans()
	if spans != nil {
		t.Errorf("空追踪器的 Spans 应返回 nil, 得到 %v", spans)
	}
}

// TestMemoryTracer_RecentSpans 测试返回最近 N 个 Span（从新到旧）
func TestMemoryTracer_RecentSpans(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	mt.StartSpan(ctx, "first")
	mt.StartSpan(ctx, "second")
	mt.StartSpan(ctx, "third")

	recent := mt.RecentSpans(2)
	if len(recent) != 2 {
		t.Fatalf("RecentSpans(2) 数量: 期望 2, 得到 %d", len(recent))
	}

	// 验证顺序（从新到旧）
	data := recent[0].Export()
	if data.Name != "third" {
		t.Errorf("最新 Span: 期望 %q, 得到 %q", "third", data.Name)
	}
	data = recent[1].Export()
	if data.Name != "second" {
		t.Errorf("第二新 Span: 期望 %q, 得到 %q", "second", data.Name)
	}
}

// TestMemoryTracer_RecentSpans_Zero 测试 n=0 返回 nil
func TestMemoryTracer_RecentSpans_Zero(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()
	mt.StartSpan(ctx, "op")

	result := mt.RecentSpans(0)
	if result != nil {
		t.Error("RecentSpans(0) 应返回 nil")
	}
}

// TestMemoryTracer_RecentSpans_OverSize 测试 n > size 时返回全部
func TestMemoryTracer_RecentSpans_OverSize(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	mt.StartSpan(ctx, "first")
	mt.StartSpan(ctx, "second")

	recent := mt.RecentSpans(100)
	if len(recent) != 2 {
		t.Errorf("RecentSpans(100) 数量: 期望 2, 得到 %d", len(recent))
	}
}

// TestMemoryTracer_Size_MaxSpans 测试 Size 和 MaxSpans 返回正确值
func TestMemoryTracer_Size_MaxSpans(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(5))
	ctx := context.Background()

	if mt.MaxSpans() != 5 {
		t.Errorf("MaxSpans: 期望 5, 得到 %d", mt.MaxSpans())
	}

	for i := 0; i < 3; i++ {
		mt.StartSpan(ctx, fmt.Sprintf("op-%d", i))
	}
	if mt.Size() != 3 {
		t.Errorf("Size: 期望 3, 得到 %d", mt.Size())
	}
}

// TestMemoryTracer_Clear 测试清空所有 Span
func TestMemoryTracer_Clear(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	mt.StartSpan(ctx, "op1")
	mt.StartSpan(ctx, "op2")

	oldTraceID := mt.ExtractTraceID(context.Background())

	mt.Clear()

	if mt.Size() != 0 {
		t.Errorf("Clear 后 Size: 期望 0, 得到 %d", mt.Size())
	}
	spans := mt.Spans()
	if spans != nil {
		t.Error("Clear 后 Spans 应返回 nil")
	}

	// traceID 应重新生成
	newTraceID := mt.ExtractTraceID(context.Background())
	if newTraceID == oldTraceID {
		t.Error("Clear 后 traceID 应重新生成")
	}
}

// TestMemoryTracer_Export 测试导出 SpanData 列表
func TestMemoryTracer_Export(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	mt.StartSpan(ctx, "op1")
	mt.StartSpan(ctx, "op2")

	data := mt.Export()
	if len(data) != 2 {
		t.Fatalf("Export 数量: 期望 2, 得到 %d", len(data))
	}

	if data[0].Name != "op1" {
		t.Errorf("第一个 SpanData 名称: 期望 %q, 得到 %q", "op1", data[0].Name)
	}
	if data[1].Name != "op2" {
		t.Errorf("第二个 SpanData 名称: 期望 %q, 得到 %q", "op2", data[1].Name)
	}
}

// TestMemoryTracer_RingBufferOverflow 测试环形缓冲区溢出
func TestMemoryTracer_RingBufferOverflow(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(3))
	ctx := context.Background()

	// 添加 5 个 Span，缓冲区大小为 3
	for i := 1; i <= 5; i++ {
		mt.StartSpan(ctx, fmt.Sprintf("op-%d", i))
	}

	// 应该只保留最新的 3 个
	if mt.Size() != 3 {
		t.Errorf("环形缓冲区溢出后 Size: 期望 3, 得到 %d", mt.Size())
	}

	spans := mt.Spans()
	if len(spans) != 3 {
		t.Fatalf("Spans 数量: 期望 3, 得到 %d", len(spans))
	}

	// 验证保留的是最新的 3 个（op-3, op-4, op-5），顺序从旧到新
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Export().Name
	}
	expected := []string{"op-3", "op-4", "op-5"}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("Span[%d]: 期望 %q, 得到 %q", i, exp, names[i])
		}
	}
}

// TestMemoryTracer_ExtractTraceID 测试提取 TraceID
func TestMemoryTracer_ExtractTraceID(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	// 无 Span 的 context，返回 tracer 自身的 traceID
	traceID := mt.ExtractTraceID(ctx)
	if traceID == "" {
		t.Error("ExtractTraceID 不应返回空字符串")
	}

	// 有 Span 的 context，返回 Span 的 traceID
	ctx, span := mt.StartSpan(ctx, "op")
	extracted := mt.ExtractTraceID(ctx)
	if extracted != span.TraceID() {
		t.Errorf("ExtractTraceID: 期望 %q, 得到 %q", span.TraceID(), extracted)
	}
}

// TestMemoryTracer_InjectTraceID 测试注入 TraceID 到 context
func TestMemoryTracer_InjectTraceID(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	newCtx := mt.InjectTraceID(ctx, "injected-trace-id")
	if newCtx == nil {
		t.Fatal("InjectTraceID 返回的 context 不应为 nil")
	}

	// 验证注入的值
	val := newCtx.Value(traceIDKey{})
	if val != "injected-trace-id" {
		t.Errorf("注入的 traceID: 期望 %q, 得到 %v", "injected-trace-id", val)
	}
}

// TestMemoryTracer_Shutdown 测试关闭追踪器不报错
func TestMemoryTracer_Shutdown(t *testing.T) {
	mt := NewMemoryTracer()
	err := mt.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown 不应返回错误, 得到 %v", err)
	}
}

// TestMemoryTracer_Concurrent 测试并发安全性
func TestMemoryTracer_Concurrent(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(1000))
	ctx := context.Background()
	var wg sync.WaitGroup
	n := 100

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			_, span := mt.StartSpan(ctx, fmt.Sprintf("concurrent-%d", idx))
			span.SetAttribute("idx", idx)
			span.End()
		}(i)
	}

	// 并发读取
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = mt.Spans()
			_ = mt.RecentSpans(10)
			_ = mt.Size()
			_ = mt.Export()
		}()
	}

	wg.Wait()

	if mt.Size() != n {
		t.Errorf("并发后 Size: 期望 %d, 得到 %d", n, mt.Size())
	}
}

// ============================================================================
// NoopSpan / NoopTracer 测试
// ============================================================================

// TestNoopSpan_AllMethods 测试 NoopSpan 所有方法不 panic
func TestNoopSpan_AllMethods(t *testing.T) {
	span := &NoopSpan{}

	// ID 方法返回空字符串
	if span.SpanID() != "" {
		t.Errorf("NoopSpan.SpanID: 期望空字符串, 得到 %q", span.SpanID())
	}
	if span.TraceID() != "" {
		t.Errorf("NoopSpan.TraceID: 期望空字符串, 得到 %q", span.TraceID())
	}

	// IsRecording 返回 false
	if span.IsRecording() {
		t.Error("NoopSpan.IsRecording: 期望 false")
	}

	// 以下所有方法不应 panic
	span.SetName("test")
	span.SetInput("input")
	span.SetOutput("output")
	span.SetTokenUsage(TokenUsage{PromptTokens: 1})
	span.SetAttribute("key", "value")
	span.SetAttributes(map[string]any{"a": 1})
	span.AddEvent("event", "k", "v")
	span.RecordError(errors.New("err"))
	span.SetStatus(StatusCodeOK, "ok")
	span.End()
	span.EndWithError(errors.New("err"))
}

// TestNoopTracer_StartSpan 测试 NoopTracer 创建 NoopSpan
func TestNoopTracer_StartSpan(t *testing.T) {
	nt := &NoopTracer{}
	ctx := context.Background()

	newCtx, span := nt.StartSpan(ctx, "test")
	if span == nil {
		t.Fatal("NoopTracer.StartSpan 不应返回 nil Span")
	}

	// 应该是 NoopSpan
	if _, ok := span.(*NoopSpan); !ok {
		t.Errorf("期望 *NoopSpan, 得到 %T", span)
	}

	// context 不应被修改（NoopTracer 不注入 Span）
	if newCtx != ctx {
		t.Error("NoopTracer 不应修改 context")
	}
}

// TestNoopTracer_OtherMethods 测试 NoopTracer 其他方法不报错
func TestNoopTracer_OtherMethods(t *testing.T) {
	nt := &NoopTracer{}
	ctx := context.Background()

	traceID := nt.ExtractTraceID(ctx)
	if traceID != "" {
		t.Errorf("NoopTracer.ExtractTraceID: 期望空字符串, 得到 %q", traceID)
	}

	newCtx := nt.InjectTraceID(ctx, "trace-id")
	if newCtx != ctx {
		t.Error("NoopTracer.InjectTraceID 不应修改 context")
	}

	err := nt.Shutdown(ctx)
	if err != nil {
		t.Errorf("NoopTracer.Shutdown: 期望 nil, 得到 %v", err)
	}
}

// TestNewNoopTracer 测试 NewNoopTracer 创建
func TestNewNoopTracer(t *testing.T) {
	nt := NewNoopTracer()
	if nt == nil {
		t.Fatal("NewNoopTracer 不应返回 nil")
	}

	// 验证实现了 Tracer 接口
	var _ Tracer = nt
}

// ============================================================================
// Context 函数测试
// ============================================================================

// TestContextWithSpan_SpanFromContext_RoundTrip 测试 Span 往返
func TestContextWithSpan_SpanFromContext_RoundTrip(t *testing.T) {
	ctx := context.Background()
	span := NewSpan("test", "trace-1")

	ctx = ContextWithSpan(ctx, span)
	got := SpanFromContext(ctx)

	if got == nil {
		t.Fatal("SpanFromContext 不应返回 nil")
	}
	if got.SpanID() != span.SpanID() {
		t.Errorf("SpanID: 期望 %q, 得到 %q", span.SpanID(), got.SpanID())
	}
}

// TestContextWithTracer_TracerFromContext_RoundTrip 测试 Tracer 往返
func TestContextWithTracer_TracerFromContext_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mt := NewMemoryTracer()

	ctx = ContextWithTracer(ctx, mt)
	got := TracerFromContext(ctx)

	if got == nil {
		t.Fatal("TracerFromContext 不应返回 nil")
	}
	// 通过行为验证是同一个 tracer
	if got != mt {
		t.Error("TracerFromContext 应返回同一个 Tracer 实例")
	}
}

// TestSpanFromContext_EmptyContext 测试空 context 返回 nil
func TestSpanFromContext_EmptyContext(t *testing.T) {
	span := SpanFromContext(context.Background())
	if span != nil {
		t.Error("空 context 的 SpanFromContext 应返回 nil")
	}
}

// TestTracerFromContext_EmptyContext 测试空 context 返回 nil
func TestTracerFromContext_EmptyContext(t *testing.T) {
	tracer := TracerFromContext(context.Background())
	if tracer != nil {
		t.Error("空 context 的 TracerFromContext 应返回 nil")
	}
}

// TestContextWithSpan_Override 测试 Span 覆盖
func TestContextWithSpan_Override(t *testing.T) {
	ctx := context.Background()
	span1 := NewSpan("first", "trace-1")
	span2 := NewSpan("second", "trace-1")

	ctx = ContextWithSpan(ctx, span1)
	ctx = ContextWithSpan(ctx, span2)

	got := SpanFromContext(ctx)
	if got.SpanID() != span2.SpanID() {
		t.Error("后设置的 Span 应覆盖先前的")
	}
}

// ============================================================================
// 便捷函数测试
// ============================================================================

// TestStartSpan_NoTracer 测试无 Tracer 时返回 NoopSpan
func TestStartSpan_NoTracer(t *testing.T) {
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.op")

	if _, ok := span.(*NoopSpan); !ok {
		t.Errorf("无 Tracer 时应返回 *NoopSpan, 得到 %T", span)
	}

	// context 不应被修改
	if newCtx != ctx {
		t.Error("无 Tracer 时 context 不应被修改")
	}
}

// TestStartSpan_WithTracer 测试有 Tracer 时创建真实 Span
func TestStartSpan_WithTracer(t *testing.T) {
	mt := NewMemoryTracer()
	ctx := ContextWithTracer(context.Background(), mt)

	_, span := StartSpan(ctx, "real.op")

	if _, ok := span.(*NoopSpan); ok {
		t.Error("有 Tracer 时不应返回 NoopSpan")
	}
	if span.SpanID() == "" {
		t.Error("SpanID 不应为空")
	}
	if mt.Size() != 1 {
		t.Errorf("Tracer 中应有 1 个 Span, 得到 %d", mt.Size())
	}
}

// TestStartAgentSpan 测试 Agent Span 便捷函数
func TestStartAgentSpan(t *testing.T) {
	mt := NewMemoryTracer()
	ctx := ContextWithTracer(context.Background(), mt)

	_, span := StartAgentSpan(ctx, "agent-001", "assistant")

	ds, ok := span.(*DefaultSpan)
	if !ok {
		t.Fatalf("期望 *DefaultSpan, 得到 %T", span)
	}

	// 验证 Kind
	data := ds.Export()
	if data.Kind != "agent" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "agent", data.Kind)
	}

	// 验证属性
	attrs := ds.Attributes()
	if attrs[AttrAgentID] != "agent-001" {
		t.Errorf("agent.id: 期望 %q, 得到 %v", "agent-001", attrs[AttrAgentID])
	}
	if attrs[AttrAgentName] != "assistant" {
		t.Errorf("agent.name: 期望 %q, 得到 %v", "assistant", attrs[AttrAgentName])
	}
}

// TestStartLLMSpan 测试 LLM Span 便捷函数
func TestStartLLMSpan(t *testing.T) {
	mt := NewMemoryTracer()
	ctx := ContextWithTracer(context.Background(), mt)

	_, span := StartLLMSpan(ctx, "openai", "gpt-4o")

	ds, ok := span.(*DefaultSpan)
	if !ok {
		t.Fatalf("期望 *DefaultSpan, 得到 %T", span)
	}

	data := ds.Export()
	if data.Kind != "llm" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "llm", data.Kind)
	}

	attrs := ds.Attributes()
	if attrs[AttrLLMProvider] != "openai" {
		t.Errorf("llm.provider: 期望 %q, 得到 %v", "openai", attrs[AttrLLMProvider])
	}
	if attrs[AttrLLMModel] != "gpt-4o" {
		t.Errorf("llm.model: 期望 %q, 得到 %v", "gpt-4o", attrs[AttrLLMModel])
	}
}

// TestStartToolSpan 测试 Tool Span 便捷函数
func TestStartToolSpan(t *testing.T) {
	mt := NewMemoryTracer()
	ctx := ContextWithTracer(context.Background(), mt)

	_, span := StartToolSpan(ctx, "calculator")

	ds, ok := span.(*DefaultSpan)
	if !ok {
		t.Fatalf("期望 *DefaultSpan, 得到 %T", span)
	}

	data := ds.Export()
	if data.Kind != "tool" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "tool", data.Kind)
	}

	attrs := ds.Attributes()
	if attrs[AttrToolName] != "calculator" {
		t.Errorf("tool.name: 期望 %q, 得到 %v", "calculator", attrs[AttrToolName])
	}
}

// TestStartRetrievalSpan 测试 Retrieval Span 便捷函数
func TestStartRetrievalSpan(t *testing.T) {
	mt := NewMemoryTracer()
	ctx := ContextWithTracer(context.Background(), mt)

	_, span := StartRetrievalSpan(ctx, "how to use Go channels", 10)

	ds, ok := span.(*DefaultSpan)
	if !ok {
		t.Fatalf("期望 *DefaultSpan, 得到 %T", span)
	}

	data := ds.Export()
	if data.Kind != "retrieval" {
		t.Errorf("Kind: 期望 %q, 得到 %q", "retrieval", data.Kind)
	}

	attrs := ds.Attributes()
	if attrs[AttrRetrievalQuery] != "how to use Go channels" {
		t.Errorf("retrieval.query: 期望 %q, 得到 %v", "how to use Go channels", attrs[AttrRetrievalQuery])
	}
	if attrs[AttrRetrievalTopK] != 10 {
		t.Errorf("retrieval.top_k: 期望 10, 得到 %v", attrs[AttrRetrievalTopK])
	}
}

// TestStartAgentSpan_NoTracer 测试无 Tracer 时 Agent Span 返回 NoopSpan
func TestStartAgentSpan_NoTracer(t *testing.T) {
	ctx := context.Background()
	_, span := StartAgentSpan(ctx, "agent-001", "assistant")

	if _, ok := span.(*NoopSpan); !ok {
		t.Errorf("无 Tracer 时应返回 *NoopSpan, 得到 %T", span)
	}
}

// TestStartLLMSpan_NoTracer 测试无 Tracer 时 LLM Span 返回 NoopSpan
func TestStartLLMSpan_NoTracer(t *testing.T) {
	ctx := context.Background()
	_, span := StartLLMSpan(ctx, "openai", "gpt-4o")

	if _, ok := span.(*NoopSpan); !ok {
		t.Errorf("无 Tracer 时应返回 *NoopSpan, 得到 %T", span)
	}
}

// ============================================================================
// GlobalTracer 测试
// ============================================================================

// TestGetGlobalTracer_Default 测试默认全局 Tracer 是 NoopTracer
func TestGetGlobalTracer_Default(t *testing.T) {
	// 先保存并还原全局状态
	original := GetGlobalTracer()
	defer SetGlobalTracer(original)

	// 重置为默认值
	SetGlobalTracer(NewNoopTracer())

	gt := GetGlobalTracer()
	if _, ok := gt.(*NoopTracer); !ok {
		t.Errorf("默认全局 Tracer 应为 *NoopTracer, 得到 %T", gt)
	}
}

// TestSetGlobalTracer 测试设置全局 Tracer
func TestSetGlobalTracer(t *testing.T) {
	original := GetGlobalTracer()
	defer SetGlobalTracer(original)

	mt := NewMemoryTracer()
	SetGlobalTracer(mt)

	got := GetGlobalTracer()
	if got != mt {
		t.Error("SetGlobalTracer 后 GetGlobalTracer 应返回设置的 Tracer")
	}
}

// TestStart_UsesGlobalTracer 测试 Start 使用全局 Tracer
func TestStart_UsesGlobalTracer(t *testing.T) {
	original := GetGlobalTracer()
	defer SetGlobalTracer(original)

	mt := NewMemoryTracer()
	SetGlobalTracer(mt)

	ctx := context.Background()
	_, span := Start(ctx, "global.op")

	if span.SpanID() == "" {
		t.Error("SpanID 不应为空")
	}
	if mt.Size() != 1 {
		t.Errorf("全局 Tracer 中应有 1 个 Span, 得到 %d", mt.Size())
	}
}

// TestStart_DefaultNoopTracer 测试默认全局 Tracer (NoopTracer) 的 Start
func TestStart_DefaultNoopTracer(t *testing.T) {
	original := GetGlobalTracer()
	defer SetGlobalTracer(original)

	SetGlobalTracer(NewNoopTracer())
	ctx := context.Background()
	_, span := Start(ctx, "noop.op")

	if _, ok := span.(*NoopSpan); !ok {
		t.Errorf("默认全局 Tracer 的 Start 应返回 *NoopSpan, 得到 %T", span)
	}
}

// TestGlobalTracer_Concurrent 测试全局 Tracer 并发安全
func TestGlobalTracer_Concurrent(t *testing.T) {
	original := GetGlobalTracer()
	defer SetGlobalTracer(original)

	var wg sync.WaitGroup
	n := 50

	// 并发设置和获取全局 Tracer
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			SetGlobalTracer(NewMemoryTracer())
		}()
		go func() {
			defer wg.Done()
			gt := GetGlobalTracer()
			if gt == nil {
				t.Error("GetGlobalTracer 不应返回 nil")
			}
		}()
	}

	wg.Wait()
}

// ============================================================================
// 环形缓冲区边界测试
// ============================================================================

// TestMemoryTracer_RingBuffer_ExactFull 测试恰好填满缓冲区
func TestMemoryTracer_RingBuffer_ExactFull(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(3))
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		mt.StartSpan(ctx, fmt.Sprintf("op-%d", i))
	}

	if mt.Size() != 3 {
		t.Errorf("Size: 期望 3, 得到 %d", mt.Size())
	}

	spans := mt.Spans()
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Export().Name
	}
	expected := []string{"op-1", "op-2", "op-3"}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("Span[%d]: 期望 %q, 得到 %q", i, exp, names[i])
		}
	}
}

// TestMemoryTracer_RingBuffer_WrapAround 测试多次环绕
func TestMemoryTracer_RingBuffer_WrapAround(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(3))
	ctx := context.Background()

	// 添加 10 个 Span，环绕多次
	for i := 1; i <= 10; i++ {
		mt.StartSpan(ctx, fmt.Sprintf("op-%d", i))
	}

	if mt.Size() != 3 {
		t.Errorf("Size: 期望 3, 得到 %d", mt.Size())
	}

	// 应保留最新的 3 个: op-8, op-9, op-10
	spans := mt.Spans()
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Export().Name
	}
	expected := []string{"op-8", "op-9", "op-10"}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("Span[%d]: 期望 %q, 得到 %q", i, exp, names[i])
		}
	}

	// RecentSpans 也应正确
	recent := mt.RecentSpans(3)
	recentNames := make([]string, len(recent))
	for i, s := range recent {
		recentNames[i] = s.Export().Name
	}
	expectedRecent := []string{"op-10", "op-9", "op-8"}
	for i, exp := range expectedRecent {
		if recentNames[i] != exp {
			t.Errorf("RecentSpan[%d]: 期望 %q, 得到 %q", i, exp, recentNames[i])
		}
	}
}

// TestMemoryTracer_SingleCapacity 测试容量为 1 的环形缓冲区
func TestMemoryTracer_SingleCapacity(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(1))
	ctx := context.Background()

	mt.StartSpan(ctx, "first")
	mt.StartSpan(ctx, "second")
	mt.StartSpan(ctx, "third")

	if mt.Size() != 1 {
		t.Errorf("Size: 期望 1, 得到 %d", mt.Size())
	}

	spans := mt.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans 数量: 期望 1, 得到 %d", len(spans))
	}
	if spans[0].Export().Name != "third" {
		t.Errorf("唯一 Span: 期望 %q, 得到 %q", "third", spans[0].Export().Name)
	}
}

// TestMemoryTracer_TraceID_Consistent 测试同一 Tracer 的 Span 共享 TraceID
func TestMemoryTracer_TraceID_Consistent(t *testing.T) {
	mt := NewMemoryTracer(WithMaxSpans(10))
	ctx := context.Background()

	_, span1 := mt.StartSpan(ctx, "op1")
	_, span2 := mt.StartSpan(ctx, "op2")

	if span1.TraceID() != span2.TraceID() {
		t.Errorf("同一 Tracer 的 Span 应共享 TraceID: %q vs %q", span1.TraceID(), span2.TraceID())
	}
}
