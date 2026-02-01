// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/streamx"
)

// LLMProvider Mock LLM Provider
type LLMProvider struct {
	name      string
	responses []MockResponse
	current   int
	calls     []llm.CompletionRequest
	mu        sync.Mutex

	// 自定义响应函数
	responseFn func(req llm.CompletionRequest) (*llm.CompletionResponse, error)
}

// MockResponse 模拟响应
type MockResponse struct {
	Content   string
	ToolCalls []llm.ToolCall
	Usage     llm.Usage
	Error     error
}

// NewLLMProvider 创建 Mock LLM Provider
func NewLLMProvider(name string) *LLMProvider {
	return &LLMProvider{
		name:      name,
		responses: make([]MockResponse, 0),
		calls:     make([]llm.CompletionRequest, 0),
	}
}

// AddResponse 添加模拟响应
func (p *LLMProvider) AddResponse(content string) *LLMProvider {
	p.responses = append(p.responses, MockResponse{
		Content: content,
		Usage:   llm.Usage{TotalTokens: 100},
	})
	return p
}

// AddToolCallResponse 添加工具调用响应
func (p *LLMProvider) AddToolCallResponse(toolCalls []llm.ToolCall) *LLMProvider {
	p.responses = append(p.responses, MockResponse{
		ToolCalls: toolCalls,
		Usage:     llm.Usage{TotalTokens: 150},
	})
	return p
}

// AddErrorResponse 添加错误响应
func (p *LLMProvider) AddErrorResponse(err error) *LLMProvider {
	p.responses = append(p.responses, MockResponse{Error: err})
	return p
}

// WithResponseFn 设置自定义响应函数
func (p *LLMProvider) WithResponseFn(fn func(req llm.CompletionRequest) (*llm.CompletionResponse, error)) *LLMProvider {
	p.responseFn = fn
	return p
}

// Name 返回 Provider 名称
func (p *LLMProvider) Name() string {
	return p.name
}

// Complete 模拟完成请求
func (p *LLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 记录调用
	p.calls = append(p.calls, req)

	// 检查 context
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 使用自定义响应函数
	if p.responseFn != nil {
		return p.responseFn(req)
	}

	// 使用预定义响应
	if p.current >= len(p.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}

	resp := p.responses[p.current]
	p.current++

	if resp.Error != nil {
		return nil, resp.Error
	}

	return &llm.CompletionResponse{
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
		Usage:     resp.Usage,
	}, nil
}

// Stream 模拟流式请求
func (p *LLMProvider) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	// 简化实现：直接返回完整响应作为流
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	// 创建一个模拟的 SSE 响应
	sseData := fmt.Sprintf("data: {\"content\":\"%s\"}\n\ndata: [DONE]\n\n", resp.Content)
	reader := strings.NewReader(sseData)

	return streamx.NewStream(reader, streamx.OpenAIFormat), nil
}

// Models 返回支持的模型列表
func (p *LLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{
		{
			ID:          "mock-model",
			Name:        "Mock Model",
			Description: "A mock model for testing",
		},
	}
}

// CountTokens 模拟 Token 计数
func (p *LLMProvider) CountTokens(messages []llm.Message) (int, error) {
	total := 0
	for _, msg := range messages {
		// 简单估算：4 个字符 = 1 token
		total += len(msg.Content) / 4
	}
	return total, nil
}

// Calls 返回所有调用记录
func (p *LLMProvider) Calls() []llm.CompletionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := make([]llm.CompletionRequest, len(p.calls))
	copy(calls, p.calls)
	return calls
}

// LastCall 返回最后一次调用
func (p *LLMProvider) LastCall() *llm.CompletionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return nil
	}
	call := p.calls[len(p.calls)-1]
	return &call
}

// CallCount 返回调用次数
func (p *LLMProvider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// Reset 重置状态
func (p *LLMProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = 0
	p.calls = make([]llm.CompletionRequest, 0)
}

// 确保实现了 Provider 接口
var _ llm.Provider = (*LLMProvider)(nil)

// ============== 工具函数 ==============

// EchoProvider 创建回声 Provider（返回输入作为输出）
func EchoProvider() *LLMProvider {
	p := NewLLMProvider("echo")
	p.responseFn = func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		// 返回最后一条用户消息
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == llm.RoleUser {
				return &llm.CompletionResponse{
					Content: "Echo: " + req.Messages[i].Content,
					Usage:   llm.Usage{TotalTokens: 50},
				}, nil
			}
		}
		return &llm.CompletionResponse{
			Content: "No user message found",
			Usage:   llm.Usage{TotalTokens: 10},
		}, nil
	}
	return p
}

// FixedProvider 创建固定响应 Provider
func FixedProvider(response string) *LLMProvider {
	p := NewLLMProvider("fixed")
	p.responseFn = func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		return &llm.CompletionResponse{
			Content: response,
			Usage:   llm.Usage{TotalTokens: len(response) / 4},
		}, nil
	}
	return p
}

// ErrorProvider 创建总是返回错误的 Provider
func ErrorProvider(err error) *LLMProvider {
	p := NewLLMProvider("error")
	p.responseFn = func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		return nil, err
	}
	return p
}

// SequenceProvider 创建按顺序返回响应的 Provider
func SequenceProvider(responses ...string) *LLMProvider {
	p := NewLLMProvider("sequence")
	for _, r := range responses {
		p.AddResponse(r)
	}
	return p
}
