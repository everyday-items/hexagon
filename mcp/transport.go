// Package mcp 实现 Model Context Protocol (MCP) 支持
//
// 本文件定义 MCP 传输层接口及 HTTP 实现
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Transport MCP 传输层接口
//
// Transport 定义了 MCP 客户端与服务器通信的方式。
// 支持多种实现：HTTP、Stdio 等。
type Transport interface {
	// Send 发送 MCP 请求并返回响应
	// 实现应该处理 JSON-RPC 2.0 格式的请求和响应
	Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error)

	// Close 关闭传输层
	// 释放相关资源（如进程句柄、网络连接等）
	Close() error
}

// ============== HTTP Transport ==============

// HTTPTransport HTTP 传输层实现
//
// 通过 HTTP POST 请求与 MCP 服务器通信，使用 JSON-RPC 2.0 协议。
//
// 示例：
//
//	transport := mcp.NewHTTPTransport("http://localhost:8080")
//	client := mcp.NewClientWithTransport(transport)
type HTTPTransport struct {
	endpoint   string
	httpClient *http.Client
	nextID     int64
}

// HTTPTransportOption HTTP 传输层选项
type HTTPTransportOption func(*HTTPTransport)

// WithHTTPClient 设置自定义 HTTP 客户端
func WithHTTPClient(client *http.Client) HTTPTransportOption {
	return func(t *HTTPTransport) {
		t.httpClient = client
	}
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) HTTPTransportOption {
	return func(t *HTTPTransport) {
		t.httpClient.Timeout = timeout
	}
}

// NewHTTPTransport 创建 HTTP 传输层
//
// endpoint 是 MCP 服务器的 HTTP 地址，如 "http://localhost:8080"
//
// 示例：
//
//	transport := mcp.NewHTTPTransport("http://localhost:8080",
//	    mcp.WithTimeout(60*time.Second),
//	)
func NewHTTPTransport(endpoint string, opts ...HTTPTransportOption) *HTTPTransport {
	t := &HTTPTransport{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// Send 发送 MCP 请求
func (t *HTTPTransport) Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
	// 自动分配请求 ID（如果未设置）
	if req.ID == nil {
		req.ID = atomic.AddInt64(&t.nextID, 1)
	}

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	httpResp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析响应
	var resp MCPResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查 JSON-RPC 错误
	if resp.Error != nil {
		return nil, fmt.Errorf("MCP 错误 %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}

// Close 关闭 HTTP 传输层
func (t *HTTPTransport) Close() error {
	// HTTP 客户端无需显式关闭
	return nil
}

// ============== TransportClient ==============

// TransportClient 基于 Transport 的 MCP 客户端
//
// 与 Client 类似，但使用 Transport 接口而非固定的 HTTP 通信
type TransportClient struct {
	transport Transport

	// 服务器能力（初始化后获取）
	serverCapabilities *ServerCapabilities
}

// NewTransportClient 创建基于 Transport 的客户端
//
// 示例：
//
//	// HTTP 传输
//	transport := mcp.NewHTTPTransport("http://localhost:8080")
//	client := mcp.NewTransportClient(transport)
//
//	// Stdio 传输
//	transport, _ := mcp.NewStdioTransport("npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
//	client := mcp.NewTransportClient(transport)
func NewTransportClient(transport Transport) *TransportClient {
	return &TransportClient{
		transport: transport,
	}
}

// Initialize 初始化客户端
func (c *TransportClient) Initialize(ctx context.Context) error {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodInitialize,
		Params: map[string]any{
			"protocolVersion": MCPVersion,
			"capabilities": map[string]any{
				"roots": map[string]any{
					"listChanged": true,
				},
			},
			"clientInfo": map[string]any{
				"name":    "hexagon-mcp-client",
				"version": "1.0.0",
			},
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return fmt.Errorf("初始化失败: %w", err)
	}

	// 解析服务器能力
	if result, ok := resp.Result.(map[string]any); ok {
		if caps, ok := result["capabilities"]; ok {
			capsBytes, _ := json.Marshal(caps)
			var serverCaps ServerCapabilities
			if err := json.Unmarshal(capsBytes, &serverCaps); err == nil {
				c.serverCapabilities = &serverCaps
			}
		}
	}

	return nil
}

// ListTools 列出可用工具
func (c *TransportClient) ListTools(ctx context.Context) ([]Tool, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodToolsList,
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("列出工具失败: %w", err)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式无效")
	}

	toolsRaw, ok := result["tools"]
	if !ok {
		return nil, nil
	}

	toolsBytes, _ := json.Marshal(toolsRaw)
	var tools []Tool
	if err := json.Unmarshal(toolsBytes, &tools); err != nil {
		return nil, fmt.Errorf("解析工具列表失败: %w", err)
	}

	return tools, nil
}

// CallTool 调用工具
func (c *TransportClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCallResponse, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodToolsCall,
		Params: ToolCallRequest{
			Name:      name,
			Arguments: args,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("调用工具失败: %w", err)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var toolResp ToolCallResponse
	if err := json.Unmarshal(resultBytes, &toolResp); err != nil {
		return nil, fmt.Errorf("解析工具响应失败: %w", err)
	}

	return &toolResp, nil
}

// ListResources 列出可用资源
func (c *TransportClient) ListResources(ctx context.Context) ([]Resource, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodResourcesList,
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("列出资源失败: %w", err)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式无效")
	}

	resourcesRaw, ok := result["resources"]
	if !ok {
		return nil, nil
	}

	resourcesBytes, _ := json.Marshal(resourcesRaw)
	var resources []Resource
	if err := json.Unmarshal(resourcesBytes, &resources); err != nil {
		return nil, fmt.Errorf("解析资源列表失败: %w", err)
	}

	return resources, nil
}

// ReadResource 读取资源
func (c *TransportClient) ReadResource(ctx context.Context, uri string) (*ResourceContent, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodResourcesRead,
		Params: map[string]any{
			"uri": uri,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("读取资源失败: %w", err)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式无效")
	}

	contentsRaw, ok := result["contents"]
	if !ok {
		return nil, fmt.Errorf("响应中无 contents")
	}

	contentsList, ok := contentsRaw.([]any)
	if !ok || len(contentsList) == 0 {
		return nil, fmt.Errorf("contents 为空")
	}

	contentBytes, _ := json.Marshal(contentsList[0])
	var content ResourceContent
	if err := json.Unmarshal(contentBytes, &content); err != nil {
		return nil, fmt.Errorf("解析资源内容失败: %w", err)
	}

	return &content, nil
}

// ListPrompts 列出可用提示
func (c *TransportClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodPromptsList,
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("列出提示失败: %w", err)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式无效")
	}

	promptsRaw, ok := result["prompts"]
	if !ok {
		return nil, nil
	}

	promptsBytes, _ := json.Marshal(promptsRaw)
	var prompts []Prompt
	if err := json.Unmarshal(promptsBytes, &prompts); err != nil {
		return nil, fmt.Errorf("解析提示列表失败: %w", err)
	}

	return prompts, nil
}

// GetPrompt 获取提示
func (c *TransportClient) GetPrompt(ctx context.Context, name string, args map[string]string) ([]PromptMessage, error) {
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  MethodPromptsGet,
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取提示失败: %w", err)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式无效")
	}

	messagesRaw, ok := result["messages"]
	if !ok {
		return nil, nil
	}

	messagesBytes, _ := json.Marshal(messagesRaw)
	var messages []PromptMessage
	if err := json.Unmarshal(messagesBytes, &messages); err != nil {
		return nil, fmt.Errorf("解析消息列表失败: %w", err)
	}

	return messages, nil
}

// GetServerCapabilities 获取服务器能力
func (c *TransportClient) GetServerCapabilities() *ServerCapabilities {
	return c.serverCapabilities
}

// Close 关闭客户端
func (c *TransportClient) Close() error {
	return c.transport.Close()
}

// Transport 返回底层传输层
func (c *TransportClient) Transport() Transport {
	return c.transport
}
