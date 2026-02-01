package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestDefault(t *testing.T) {
	logger := Default()

	if logger == nil {
		t.Fatal("expected non-nil default logger")
	}
}

func TestContextWithAgentID(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithAgentID(ctx, "agent-123")

	agentID := AgentIDFromContext(ctx)
	if agentID != "agent-123" {
		t.Errorf("expected agent ID 'agent-123', got '%s'", agentID)
	}
}

func TestContextWithSessionID(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithSessionID(ctx, "session-456")

	sessionID := SessionIDFromContext(ctx)
	if sessionID != "session-456" {
		t.Errorf("expected session ID 'session-456', got '%s'", sessionID)
	}
}

func TestContextWithTraceID(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithTraceID(ctx, "trace-789")

	traceID := TraceIDFromContext(ctx)
	if traceID != "trace-789" {
		t.Errorf("expected trace ID 'trace-789', got '%s'", traceID)
	}
}

func TestContextWithSpanID(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithSpanID(ctx, "span-abc")

	spanID := SpanIDFromContext(ctx)
	if spanID != "span-abc" {
		t.Errorf("expected span ID 'span-abc', got '%s'", spanID)
	}
}

func TestContextWithComponent(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithComponent(ctx, "ReActAgent")

	component := ComponentFromContext(ctx)
	if component != "ReActAgent" {
		t.Errorf("expected component 'ReActAgent', got '%s'", component)
	}
}

func TestContextFromEmpty(t *testing.T) {
	ctx := context.Background()

	// 从空 context 提取应该返回空字符串
	if AgentIDFromContext(ctx) != "" {
		t.Error("expected empty agent ID from empty context")
	}

	if SessionIDFromContext(ctx) != "" {
		t.Error("expected empty session ID from empty context")
	}

	if TraceIDFromContext(ctx) != "" {
		t.Error("expected empty trace ID from empty context")
	}

	if SpanIDFromContext(ctx) != "" {
		t.Error("expected empty span ID from empty context")
	}

	if ComponentFromContext(ctx) != "" {
		t.Error("expected empty component from empty context")
	}
}

func TestAgentIDAttr(t *testing.T) {
	attr := AgentID("test-agent")

	if attr.Key != "agent_id" {
		t.Errorf("expected key 'agent_id', got '%s'", attr.Key)
	}

	if attr.Value.String() != "test-agent" {
		t.Errorf("expected value 'test-agent', got '%s'", attr.Value.String())
	}
}

func TestSessionIDAttr(t *testing.T) {
	attr := SessionID("test-session")

	if attr.Key != "session_id" {
		t.Errorf("expected key 'session_id', got '%s'", attr.Key)
	}

	if attr.Value.String() != "test-session" {
		t.Errorf("expected value 'test-session', got '%s'", attr.Value.String())
	}
}

func TestSpanIDAttr(t *testing.T) {
	attr := SpanID("test-span")

	if attr.Key != "span_id" {
		t.Errorf("expected key 'span_id', got '%s'", attr.Key)
	}

	if attr.Value.String() != "test-span" {
		t.Errorf("expected value 'test-span', got '%s'", attr.Value.String())
	}
}

func TestModelAttr(t *testing.T) {
	attr := Model("gpt-4")

	if attr.Key != "model" {
		t.Errorf("expected key 'model', got '%s'", attr.Key)
	}

	if attr.Value.String() != "gpt-4" {
		t.Errorf("expected value 'gpt-4', got '%s'", attr.Value.String())
	}
}

func TestToolNameAttr(t *testing.T) {
	attr := ToolName("calculator")

	if attr.Key != "tool_name" {
		t.Errorf("expected key 'tool_name', got '%s'", attr.Key)
	}

	if attr.Value.String() != "calculator" {
		t.Errorf("expected value 'calculator', got '%s'", attr.Value.String())
	}
}

func TestTokensAttr(t *testing.T) {
	attr := Tokens(1500)

	if attr.Key != "tokens" {
		t.Errorf("expected key 'tokens', got '%s'", attr.Key)
	}

	if attr.Value.Int64() != 1500 {
		t.Errorf("expected value 1500, got %d", attr.Value.Int64())
	}
}

func TestPromptTokensAttr(t *testing.T) {
	attr := PromptTokens(500)

	if attr.Key != "prompt_tokens" {
		t.Errorf("expected key 'prompt_tokens', got '%s'", attr.Key)
	}

	if attr.Value.Int64() != 500 {
		t.Errorf("expected value 500, got %d", attr.Value.Int64())
	}
}

func TestCompletionTokensAttr(t *testing.T) {
	attr := CompletionTokens(1000)

	if attr.Key != "completion_tokens" {
		t.Errorf("expected key 'completion_tokens', got '%s'", attr.Key)
	}

	if attr.Value.Int64() != 1000 {
		t.Errorf("expected value 1000, got %d", attr.Value.Int64())
	}
}

func TestIterationAttr(t *testing.T) {
	attr := Iteration(5)

	if attr.Key != "iteration" {
		t.Errorf("expected key 'iteration', got '%s'", attr.Key)
	}

	if attr.Value.Int64() != 5 {
		t.Errorf("expected value 5, got %d", attr.Value.Int64())
	}
}

func TestReexportedFunctions(t *testing.T) {
	// 测试重新导出的函数是否可用
	_ = String("key", "value")
	_ = Int("key", 123)
	_ = Int64("key", int64(456))
	_ = Uint64("key", uint64(789))
	_ = Float64("key", 3.14)
	_ = Bool("key", true)
	// 其他函数类似
}

func TestSetDefault(t *testing.T) {
	// 保存原始默认 logger
	original := Default()
	defer SetDefault(original)

	// 创建并设置新的默认 logger
	// 使用一个简单的实现来测试
	mockLogger := &mockLoggerImpl{}
	SetDefault(mockLogger)

	if Default() != mockLogger {
		t.Error("expected new default logger")
	}
}

// mockLoggerImpl 模拟 Logger 实现
type mockLoggerImpl struct{}

func (m *mockLoggerImpl) Debug(msg string, args ...any)                               {}
func (m *mockLoggerImpl) Info(msg string, args ...any)                                {}
func (m *mockLoggerImpl) Warn(msg string, args ...any)                                {}
func (m *mockLoggerImpl) Error(msg string, args ...any)                               {}
func (m *mockLoggerImpl) DebugContext(ctx context.Context, msg string, args ...any)   {}
func (m *mockLoggerImpl) InfoContext(ctx context.Context, msg string, args ...any)    {}
func (m *mockLoggerImpl) WarnContext(ctx context.Context, msg string, args ...any)    {}
func (m *mockLoggerImpl) ErrorContext(ctx context.Context, msg string, args ...any)   {}
func (m *mockLoggerImpl) With(args ...any) Logger                                     { return m }
func (m *mockLoggerImpl) WithContext(ctx context.Context) Logger                      { return m }
func (m *mockLoggerImpl) SetLevel(level string)                                       {}

var _ Logger = (*mockLoggerImpl)(nil)

func TestMultipleContextValues(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithAgentID(ctx, "agent-1")
	ctx = ContextWithSessionID(ctx, "session-1")
	ctx = ContextWithTraceID(ctx, "trace-1")
	ctx = ContextWithSpanID(ctx, "span-1")
	ctx = ContextWithComponent(ctx, "TestComponent")

	// 验证所有值都能正确提取
	if AgentIDFromContext(ctx) != "agent-1" {
		t.Error("agent ID mismatch")
	}

	if SessionIDFromContext(ctx) != "session-1" {
		t.Error("session ID mismatch")
	}

	if TraceIDFromContext(ctx) != "trace-1" {
		t.Error("trace ID mismatch")
	}

	if SpanIDFromContext(ctx) != "span-1" {
		t.Error("span ID mismatch")
	}

	if ComponentFromContext(ctx) != "TestComponent" {
		t.Error("component mismatch")
	}
}

func TestLoggerInterface(t *testing.T) {
	// 确保 Logger 接口定义正确
	var _ Logger = (*AgentLogger)(nil)
}

func TestAttrTypes(t *testing.T) {
	// 测试各种属性创建是否返回正确的 slog.Attr 类型
	attrs := []slog.Attr{
		AgentID("a"),
		SessionID("s"),
		SpanID("sp"),
		Model("m"),
		ToolName("t"),
		Tokens(1),
		PromptTokens(2),
		CompletionTokens(3),
		Iteration(4),
	}

	for i, attr := range attrs {
		if attr.Key == "" {
			t.Errorf("attr[%d] has empty key", i)
		}
	}
}

func TestConvenienceLoggingFunctions(t *testing.T) {
	// 保存原始默认 logger
	original := Default()
	defer SetDefault(original)

	// 设置 mock logger 来捕获调用
	mock := &mockLoggerImpl{}
	SetDefault(mock)

	// 这些调用不应该 panic
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	ctx := context.Background()
	DebugContext(ctx, "debug with context")
	InfoContext(ctx, "info with context")
	WarnContext(ctx, "warn with context")
	ErrorContext(ctx, "error with context")

	_ = With("key", "value")
	SetLevel("debug")
}
