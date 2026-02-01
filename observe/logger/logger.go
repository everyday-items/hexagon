// Package logger 提供 Hexagon AI Agent 框架的日志工具
//
// Logger 封装了 toolkit/util/logger，并添加了 Agent 相关的上下文字段支持。
// 支持自动从 context 中提取 agent_id, session_id, trace_id, span_id 等字段。
package logger

import (
	"context"
	"log/slog"
	"sync"

	tklogger "github.com/everyday-items/toolkit/util/logger"
)

// Logger 是 Hexagon 框架的日志接口
type Logger interface {
	// Debug 记录调试日志
	Debug(msg string, args ...any)

	// Info 记录信息日志
	Info(msg string, args ...any)

	// Warn 记录警告日志
	Warn(msg string, args ...any)

	// Error 记录错误日志
	Error(msg string, args ...any)

	// DebugContext 记录带 context 的调试日志
	DebugContext(ctx context.Context, msg string, args ...any)

	// InfoContext 记录带 context 的信息日志
	InfoContext(ctx context.Context, msg string, args ...any)

	// WarnContext 记录带 context 的警告日志
	WarnContext(ctx context.Context, msg string, args ...any)

	// ErrorContext 记录带 context 的错误日志
	ErrorContext(ctx context.Context, msg string, args ...any)

	// With 创建带有额外字段的子 Logger
	With(args ...any) Logger

	// WithContext 从 context 中提取上下文信息创建子 Logger
	WithContext(ctx context.Context) Logger

	// SetLevel 动态设置日志级别
	SetLevel(level string)
}

// AgentLogger 基于 toolkit logger 的 Agent 日志实现
type AgentLogger struct {
	inner *tklogger.Logger
}

// 确保实现了 Logger 接口
var _ Logger = (*AgentLogger)(nil)

// Config 日志配置
type Config = tklogger.Config

// FileConfig 文件配置
type FileConfig = tklogger.FileConfig

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return tklogger.DefaultConfig()
}

// New 创建新的 AgentLogger
func New(cfg *Config) (*AgentLogger, error) {
	inner, err := tklogger.New(cfg)
	if err != nil {
		return nil, err
	}
	return &AgentLogger{inner: inner}, nil
}

// NewWithLogger 从已有的 toolkit logger 创建 AgentLogger
func NewWithLogger(l *tklogger.Logger) *AgentLogger {
	return &AgentLogger{inner: l}
}

// Debug 记录调试日志
func (l *AgentLogger) Debug(msg string, args ...any) {
	l.inner.Debug(msg, args...)
}

// Info 记录信息日志
func (l *AgentLogger) Info(msg string, args ...any) {
	l.inner.Info(msg, args...)
}

// Warn 记录警告日志
func (l *AgentLogger) Warn(msg string, args ...any) {
	l.inner.Warn(msg, args...)
}

// Error 记录错误日志
func (l *AgentLogger) Error(msg string, args ...any) {
	l.inner.Error(msg, args...)
}

// DebugContext 记录带 context 的调试日志
func (l *AgentLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.inner.DebugContext(ctx, msg, l.appendContextAttrs(ctx, args)...)
}

// InfoContext 记录带 context 的信息日志
func (l *AgentLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.inner.InfoContext(ctx, msg, l.appendContextAttrs(ctx, args)...)
}

// WarnContext 记录带 context 的警告日志
func (l *AgentLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.inner.WarnContext(ctx, msg, l.appendContextAttrs(ctx, args)...)
}

// ErrorContext 记录带 context 的错误日志
func (l *AgentLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.inner.ErrorContext(ctx, msg, l.appendContextAttrs(ctx, args)...)
}

// With 创建带有额外字段的子 Logger
func (l *AgentLogger) With(args ...any) Logger {
	return &AgentLogger{inner: l.inner.With(args...)}
}

// WithContext 从 context 中提取上下文信息创建子 Logger
func (l *AgentLogger) WithContext(ctx context.Context) Logger {
	attrs := l.extractContextAttrs(ctx)
	if len(attrs) == 0 {
		return l
	}
	return l.With(attrs...)
}

// SetLevel 动态设置日志级别
func (l *AgentLogger) SetLevel(level string) {
	l.inner.SetLevel(level)
}

// Inner 返回底层的 toolkit logger
func (l *AgentLogger) Inner() *tklogger.Logger {
	return l.inner
}

// Close 关闭日志记录器
func (l *AgentLogger) Close() error {
	return l.inner.Close()
}

// extractContextAttrs 从 context 中提取 Agent 相关的属性
func (l *AgentLogger) extractContextAttrs(ctx context.Context) []any {
	var attrs []any

	// 提取 Agent ID
	if agentID := AgentIDFromContext(ctx); agentID != "" {
		attrs = append(attrs, "agent_id", agentID)
	}

	// 提取 Session ID
	if sessionID := SessionIDFromContext(ctx); sessionID != "" {
		attrs = append(attrs, "session_id", sessionID)
	}

	// 提取 Trace ID
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, "trace_id", traceID)
	}

	// 提取 Span ID
	if spanID := SpanIDFromContext(ctx); spanID != "" {
		attrs = append(attrs, "span_id", spanID)
	}

	// 提取 Component 名称
	if component := ComponentFromContext(ctx); component != "" {
		attrs = append(attrs, "component", component)
	}

	return attrs
}

// appendContextAttrs 将 context 属性附加到参数中
func (l *AgentLogger) appendContextAttrs(ctx context.Context, args []any) []any {
	attrs := l.extractContextAttrs(ctx)
	if len(attrs) == 0 {
		return args
	}
	return append(args, attrs...)
}

// ============== Context keys ==============

type (
	agentIDKey   struct{}
	sessionIDKey struct{}
	traceIDKey   struct{}
	spanIDKey    struct{}
	componentKey struct{}
)

// ContextWithAgentID 将 Agent ID 添加到 context
func ContextWithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDKey{}, agentID)
}

// ContextWithSessionID 将 Session ID 添加到 context
func ContextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

// ContextWithTraceID 将 Trace ID 添加到 context
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// ContextWithSpanID 将 Span ID 添加到 context
func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey{}, spanID)
}

// ContextWithComponent 将组件名称添加到 context
func ContextWithComponent(ctx context.Context, component string) context.Context {
	return context.WithValue(ctx, componentKey{}, component)
}

// AgentIDFromContext 从 context 中获取 Agent ID
func AgentIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(agentIDKey{}).(string); ok {
		return v
	}
	return ""
}

// SessionIDFromContext 从 context 中获取 Session ID
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// TraceIDFromContext 从 context 中获取 Trace ID
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// SpanIDFromContext 从 context 中获取 Span ID
func SpanIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(spanIDKey{}).(string); ok {
		return v
	}
	return ""
}

// ComponentFromContext 从 context 中获取组件名称
func ComponentFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(componentKey{}).(string); ok {
		return v
	}
	return ""
}

// ============== Agent-specific attributes ==============

// AgentID 创建 agent_id 属性
func AgentID(id string) slog.Attr {
	return slog.String("agent_id", id)
}

// SessionID 创建 session_id 属性
func SessionID(id string) slog.Attr {
	return slog.String("session_id", id)
}

// SpanID 创建 span_id 属性
func SpanID(id string) slog.Attr {
	return slog.String("span_id", id)
}

// Model 创建 model 属性
func Model(name string) slog.Attr {
	return slog.String("model", name)
}

// ToolName 创建 tool_name 属性
func ToolName(name string) slog.Attr {
	return slog.String("tool_name", name)
}

// Tokens 创建 tokens 属性
func Tokens(count int) slog.Attr {
	return slog.Int("tokens", count)
}

// PromptTokens 创建 prompt_tokens 属性
func PromptTokens(count int) slog.Attr {
	return slog.Int("prompt_tokens", count)
}

// CompletionTokens 创建 completion_tokens 属性
func CompletionTokens(count int) slog.Attr {
	return slog.Int("completion_tokens", count)
}

// Iteration 创建 iteration 属性
func Iteration(n int) slog.Attr {
	return slog.Int("iteration", n)
}

// 重新导出 toolkit logger 的常用属性函数
var (
	String   = tklogger.String
	Int      = tklogger.Int
	Int64    = tklogger.Int64
	Uint64   = tklogger.Uint64
	Float64  = tklogger.Float64
	Bool     = tklogger.Bool
	Time     = tklogger.Time
	Duration = tklogger.Duration
	Any      = tklogger.Any
	Group    = tklogger.Group
	Err      = tklogger.Err
	TraceID  = tklogger.TraceID
	Latency  = tklogger.Latency
)

// ============== 全局默认 Logger ==============

var (
	defaultLogger     Logger
	defaultLoggerOnce sync.Once
)

// Default 返回默认 Logger
func Default() Logger {
	defaultLoggerOnce.Do(func() {
		inner := tklogger.Default()
		defaultLogger = &AgentLogger{inner: inner}
	})
	return defaultLogger
}

// SetDefault 设置默认 Logger
func SetDefault(l Logger) {
	defaultLogger = l
}

// Init 初始化全局日志记录器
func Init(cfg *Config) error {
	if err := tklogger.Init(cfg); err != nil {
		return err
	}
	defaultLogger = &AgentLogger{inner: tklogger.Default()}
	return nil
}

// ============== 便捷函数 ==============

// Debug 使用默认 Logger 记录调试日志
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info 使用默认 Logger 记录信息日志
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn 使用默认 Logger 记录警告日志
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error 使用默认 Logger 记录错误日志
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// DebugContext 使用默认 Logger 记录带 context 的调试日志
func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

// InfoContext 使用默认 Logger 记录带 context 的信息日志
func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

// WarnContext 使用默认 Logger 记录带 context 的警告日志
func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

// ErrorContext 使用默认 Logger 记录带 context 的错误日志
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}

// With 创建带有固定字段的子记录器
func With(args ...any) Logger {
	return Default().With(args...)
}

// SetLevel 设置全局日志级别
func SetLevel(level string) {
	Default().SetLevel(level)
}
