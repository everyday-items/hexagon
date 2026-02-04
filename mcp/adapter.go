// Package mcp 实现 Model Context Protocol (MCP) 支持
//
// 本文件实现 MCP 工具与 Hexagon 工具之间的适配器
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ============== MCP 工具适配器 ==============

// MCPToolAdapter 将 MCP 工具转换为 Hexagon 兼容格式
type MCPToolAdapter struct {
	client   *Client
	mcpTools []Tool
}

// NewMCPToolAdapter 创建 MCP 工具适配器
func NewMCPToolAdapter(client *Client) *MCPToolAdapter {
	return &MCPToolAdapter{
		client: client,
	}
}

// LoadTools 从 MCP 服务器加载工具
func (a *MCPToolAdapter) LoadTools(ctx context.Context) error {
	tools, err := a.client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	a.mcpTools = tools
	return nil
}

// GetMCPTools 获取 MCP 工具列表
func (a *MCPToolAdapter) GetMCPTools() []Tool {
	return a.mcpTools
}

// CallTool 调用 MCP 工具
func (a *MCPToolAdapter) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := a.client.CallTool(ctx, name, args)
	if err != nil {
		return "", err
	}

	if resp.IsError {
		if len(resp.Content) > 0 {
			return "", fmt.Errorf("%s", resp.Content[0].Text)
		}
		return "", fmt.Errorf("tool call failed")
	}

	// 合并所有文本内容
	var result strings.Builder
	for _, content := range resp.Content {
		if content.Type == "text" {
			result.WriteString(content.Text)
		}
	}

	return result.String(), nil
}

// ============== Hexagon 工具到 MCP 适配器 ==============

// ToolDefinition 工具定义（简化版，不依赖外部包）
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

// HexagonToolAdapter 将 Hexagon 工具转换为 MCP 工具
type HexagonToolAdapter struct {
	server *Server
}

// NewHexagonToolAdapter 创建 Hexagon 工具适配器
func NewHexagonToolAdapter(server *Server) *HexagonToolAdapter {
	return &HexagonToolAdapter{
		server: server,
	}
}

// RegisterToolDefinition 注册工具定义为 MCP 工具
func (a *HexagonToolAdapter) RegisterToolDefinition(def ToolDefinition) {
	mcpTool := Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: convertParamsToSchema(def.Parameters),
	}

	handler := func(ctx context.Context, args map[string]any) (*ToolCallResponse, error) {
		result, err := def.Handler(ctx, args)
		if err != nil {
			return &ToolCallResponse{
				Content: []ContentBlock{
					{Type: "text", Text: err.Error()},
				},
				IsError: true,
			}, nil
		}

		return &ToolCallResponse{
			Content: []ContentBlock{
				{Type: "text", Text: result},
			},
		}, nil
	}

	a.server.RegisterTool(mcpTool, handler)
}

// RegisterToolDefinitions 批量注册工具定义
func (a *HexagonToolAdapter) RegisterToolDefinitions(defs []ToolDefinition) {
	for _, def := range defs {
		a.RegisterToolDefinition(def)
	}
}

// convertParamsToSchema 将参数 map 转换为 JSON Schema
func convertParamsToSchema(params map[string]interface{}) *JSONSchema {
	if params == nil {
		return &JSONSchema{Type: "object"}
	}

	schema := &JSONSchema{
		Type:       "object",
		Properties: make(map[string]*JSONSchema),
	}

	for name, param := range params {
		switch v := param.(type) {
		case map[string]interface{}:
			propSchema := &JSONSchema{}
			if t, ok := v["type"].(string); ok {
				propSchema.Type = t
			}
			if desc, ok := v["description"].(string); ok {
				propSchema.Description = desc
			}
			schema.Properties[name] = propSchema
		case string:
			schema.Properties[name] = &JSONSchema{Type: v}
		default:
			schema.Properties[name] = &JSONSchema{Type: "string"}
		}
	}

	return schema
}

// ============== 便捷函数 ==============

// LoadMCPTools 从 MCP 服务器加载工具
func LoadMCPTools(ctx context.Context, endpoint string) (*MCPToolAdapter, error) {
	client := NewClient(endpoint)

	if err := client.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize client: %w", err)
	}

	adapter := NewMCPToolAdapter(client)
	if err := adapter.LoadTools(ctx); err != nil {
		return nil, fmt.Errorf("load tools: %w", err)
	}

	return adapter, nil
}

// ServeMCPTools 将工具定义作为 MCP 服务器暴露
func ServeMCPTools(addr string, tools []ToolDefinition) (*Server, error) {
	server := NewServer(&ServerConfig{
		Name:    "hexagon-mcp-server",
		Version: "1.0.0",
		Addr:    addr,
	})

	adapter := NewHexagonToolAdapter(server)
	adapter.RegisterToolDefinitions(tools)

	go server.Start()

	return server, nil
}

// MCPToolCallResult MCP 工具调用结果
type MCPToolCallResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// ToJSON 将结果转换为 JSON
func (r *MCPToolCallResult) ToJSON() string {
	bytes, _ := json.Marshal(r)
	return string(bytes)
}
