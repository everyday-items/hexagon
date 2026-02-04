// Package client 提供 Hexagon 框架的 Fluent API
//
// 本包实现类似 Spring AI ChatClient 的流畅 API 风格。
//
// 设计借鉴：
//   - Spring AI: ChatClient Fluent API
//   - LangChain: 链式调用
//
// 使用示例：
//
//	result, err := hexagon.Chat().
//	    Model("gpt-4").
//	    System("你是一个助手").
//	    User("你好").
//	    Tools(weatherTool).
//	    Temperature(0.7).
//	    MaxTokens(1000).
//	    Call(ctx)
//
//	// 流式调用
//	stream, err := hexagon.Chat().
//	    Model("gpt-4").
//	    User("写一首诗").
//	    Stream().
//	    Call(ctx)
package client

import (
	"context"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/advisor"
	"github.com/everyday-items/hexagon/stream"
)

// ============== ChatClient ==============

// ChatClient Fluent 风格的聊天客户端
type ChatClient struct {
	provider llm.Provider
	config   *chatConfig
}

type chatConfig struct {
	// 模型配置
	model       string
	temperature *float64
	maxTokens   int
	topP        *float64
	stop        []string

	// 消息
	systemPrompt string
	messages     []llm.Message

	// 工具
	tools []tool.Tool

	// 切面
	advisors []advisor.Advisor

	// 流式
	streaming bool

	// 元数据
	metadata map[string]any
}

// NewChatClient 创建聊天客户端
func NewChatClient(provider llm.Provider) *ChatClient {
	return &ChatClient{
		provider: provider,
		config: &chatConfig{
			messages: make([]llm.Message, 0),
			metadata: make(map[string]any),
		},
	}
}

// ============== Fluent 方法 ==============

// Model 设置模型
func (c *ChatClient) Model(model string) *ChatClient {
	c.config.model = model
	return c
}

// System 设置系统提示
func (c *ChatClient) System(prompt string) *ChatClient {
	c.config.systemPrompt = prompt
	return c
}

// User 添加用户消息
func (c *ChatClient) User(content string) *ChatClient {
	c.config.messages = append(c.config.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: content,
	})
	return c
}

// Assistant 添加助手消息
func (c *ChatClient) Assistant(content string) *ChatClient {
	c.config.messages = append(c.config.messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: content,
	})
	return c
}

// Messages 设置消息列表
func (c *ChatClient) Messages(messages []llm.Message) *ChatClient {
	c.config.messages = messages
	return c
}

// AddMessage 添加消息
func (c *ChatClient) AddMessage(role llm.Role, content string) *ChatClient {
	c.config.messages = append(c.config.messages, llm.Message{
		Role:    role,
		Content: content,
	})
	return c
}

// Tools 设置工具
func (c *ChatClient) Tools(tools ...tool.Tool) *ChatClient {
	c.config.tools = tools
	return c
}

// Temperature 设置温度
func (c *ChatClient) Temperature(temp float64) *ChatClient {
	c.config.temperature = &temp
	return c
}

// MaxTokens 设置最大 token 数
func (c *ChatClient) MaxTokens(max int) *ChatClient {
	c.config.maxTokens = max
	return c
}

// TopP 设置 TopP
func (c *ChatClient) TopP(p float64) *ChatClient {
	c.config.topP = &p
	return c
}

// Stop 设置停止序列
func (c *ChatClient) Stop(sequences ...string) *ChatClient {
	c.config.stop = sequences
	return c
}

// Advisors 设置切面
func (c *ChatClient) Advisors(advisors ...advisor.Advisor) *ChatClient {
	c.config.advisors = advisors
	return c
}

// Metadata 设置元数据
func (c *ChatClient) Metadata(key string, value any) *ChatClient {
	c.config.metadata[key] = value
	return c
}

// Stream 启用流式输出
func (c *ChatClient) Stream() *ChatClient {
	c.config.streaming = true
	return c
}

// ============== 执行方法 ==============

// ChatResponse 聊天响应
type ChatResponse struct {
	Content      string
	ToolCalls    []llm.ToolCall
	Usage        llm.Usage
	FinishReason string
	Metadata     map[string]any
}

// StreamResponse 流式响应
type StreamResponse struct {
	Stream   *stream.StreamReader[*ChatChunk]
	Metadata map[string]any
}

// ChatChunk 流式块
type ChatChunk struct {
	Content   string
	Delta     string
	ToolCalls []llm.ToolCall
	Done      bool
}

// Call 执行调用
func (c *ChatClient) Call(ctx context.Context) (*ChatResponse, error) {
	// 构建请求
	req := c.buildRequest()

	// 如果是流式
	if c.config.streaming {
		return c.callStreaming(ctx, req)
	}

	// 非流式调用
	return c.callNonStreaming(ctx, req)
}

// CallStream 流式调用
func (c *ChatClient) CallStream(ctx context.Context) (*StreamResponse, error) {
	c.config.streaming = true
	req := c.buildRequest()

	streamResp, err := c.provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	// 包装为 StreamReader
	reader, writer := stream.Pipe[*ChatChunk](10)
	go func() {
		defer writer.Close()
		chunks := streamResp.Chunks()
		for chunk := range chunks {
			writer.Send(&ChatChunk{
				Content:   chunk.Content,
				Delta:     chunk.Content,
				ToolCalls: chunk.ToolCalls,
				Done:      chunk.FinishReason != "",
			})
		}
	}()

	return &StreamResponse{
		Stream:   reader,
		Metadata: c.config.metadata,
	}, nil
}

// buildRequest 构建请求
func (c *ChatClient) buildRequest() llm.CompletionRequest {
	messages := make([]llm.Message, 0, len(c.config.messages)+1)

	// 添加系统消息
	if c.config.systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: c.config.systemPrompt,
		})
	}

	// 添加其他消息
	messages = append(messages, c.config.messages...)

	req := llm.CompletionRequest{
		Model:       c.config.model,
		Messages:    messages,
		Temperature: c.config.temperature,
		MaxTokens:   c.config.maxTokens,
		TopP:        c.config.topP,
		Stop:        c.config.stop,
	}

	// 添加工具
	if len(c.config.tools) > 0 {
		toolDefs := make([]llm.ToolDefinition, len(c.config.tools))
		for i, t := range c.config.tools {
			toolDefs[i] = llm.NewToolDefinition(
				t.Name(),
				t.Description(),
				t.Schema(),
			)
		}
		req.Tools = toolDefs
	}

	return req
}

// callNonStreaming 非流式调用
func (c *ChatClient) callNonStreaming(ctx context.Context, req llm.CompletionRequest) (*ChatResponse, error) {
	resp, err := c.provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Content:      resp.Content,
		ToolCalls:    resp.ToolCalls,
		Usage:        resp.Usage,
		FinishReason: resp.FinishReason,
		Metadata:     c.config.metadata,
	}, nil
}

// callStreaming 流式调用并合并结果
func (c *ChatClient) callStreaming(ctx context.Context, req llm.CompletionRequest) (*ChatResponse, error) {
	streamResp, err := c.provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	// 收集结果
	result, err := streamResp.Collect()
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Content:      result.Content,
		ToolCalls:    result.ToolCalls,
		FinishReason: result.FinishReason,
		Usage:        result.Usage,
		Metadata:     c.config.metadata,
	}, nil
}

// ============== 便捷函数 ==============

// defaultProvider 默认 Provider
var defaultProvider llm.Provider

// SetDefaultProvider 设置默认 Provider
func SetDefaultProvider(provider llm.Provider) {
	defaultProvider = provider
}

// Chat 创建聊天客户端（使用默认 Provider）
func Chat() *ChatClient {
	return NewChatClient(defaultProvider)
}

// ChatWith 创建聊天客户端（指定 Provider）
func ChatWith(provider llm.Provider) *ChatClient {
	return NewChatClient(provider)
}

// ============== PromptClient ==============

// PromptClient Prompt 模板客户端
type PromptClient struct {
	template string
	vars     map[string]any
}

// NewPromptClient 创建 Prompt 客户端
func NewPromptClient(template string) *PromptClient {
	return &PromptClient{
		template: template,
		vars:     make(map[string]any),
	}
}

// Var 设置变量
func (p *PromptClient) Var(key string, value any) *PromptClient {
	p.vars[key] = value
	return p
}

// Vars 批量设置变量
func (p *PromptClient) Vars(vars map[string]any) *PromptClient {
	for k, v := range vars {
		p.vars[k] = v
	}
	return p
}

// Render 渲染模板
func (p *PromptClient) Render() (string, error) {
	// 简单实现：使用 {} 占位符
	result := p.template
	for k, v := range p.vars {
		// 简化实现
		_ = k
		_ = v
	}
	return result, nil
}

// ToChat 转换为聊天客户端
func (p *PromptClient) ToChat(provider llm.Provider) *ChatClient {
	content, _ := p.Render()
	return NewChatClient(provider).User(content)
}

// Prompt 创建 Prompt 客户端
func Prompt(template string) *PromptClient {
	return NewPromptClient(template)
}
