// Package mcp 实现 Model Context Protocol (MCP) 支持
//
// 本文件实现 MCP 与 ai-core tool.Tool 之间的适配
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/everyday-items/ai-core/tool"
)

// ============== Server 端适配 (ai-core -> MCP) ==============

// RegisterAICoreTool 将 ai-core tool.Tool 注册到 MCP 服务器
//
// 这允许将 Hexagon 工具暴露为 MCP 服务，供其他 MCP 客户端调用
//
// 示例：
//
//	calculator := tool.NewFunc("calc", "计算器", calcFn)
//	server.RegisterAICoreTool(calculator)
func (s *Server) RegisterAICoreTool(t tool.Tool) {
	// 转换为 MCP 工具定义
	mcpTool := ToolToMCPTool(t)

	// 创建处理函数
	handler := func(ctx context.Context, args map[string]any) (*ToolCallResponse, error) {
		// 调用 ai-core 工具
		result, err := t.Execute(ctx, args)
		if err != nil {
			return &ToolCallResponse{
				Content: []ContentBlock{
					{Type: "text", Text: err.Error()},
				},
				IsError: true,
			}, nil
		}

		// 转换结果
		if !result.Success {
			return &ToolCallResponse{
				Content: []ContentBlock{
					{Type: "text", Text: result.Error},
				},
				IsError: true,
			}, nil
		}

		return &ToolCallResponse{
			Content: []ContentBlock{
				{Type: "text", Text: result.String()},
			},
		}, nil
	}

	// 注册到服务器
	s.RegisterTool(mcpTool, handler)
}

// RegisterAICoreTools 批量注册 ai-core 工具
//
// 示例：
//
//	server.RegisterAICoreTools(calculator, searcher, fileReader)
func (s *Server) RegisterAICoreTools(tools ...tool.Tool) {
	for _, t := range tools {
		s.RegisterAICoreTool(t)
	}
}

// ServeMCPToolsFromAICore 将 ai-core 工具作为 MCP 服务暴露
//
// 这是一个便捷函数，一行代码即可启动 MCP 服务
//
// 示例：
//
//	calculator := tool.NewFunc("calc", "计算器", calcFn)
//	searcher := tool.NewFunc("search", "搜索", searchFn)
//
//	server, err := mcp.ServeMCPToolsFromAICore(":8080", calculator, searcher)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer server.Stop(context.Background())
func ServeMCPToolsFromAICore(addr string, tools ...tool.Tool) (*Server, error) {
	server := NewServer(&ServerConfig{
		Name:    "hexagon-mcp-server",
		Version: "1.0.0",
		Addr:    addr,
	})

	// 注册所有工具
	server.RegisterAICoreTools(tools...)

	// 启动服务器（非阻塞）
	go func() {
		if err := server.Start(); err != nil {
			// 启动失败时记录错误（实际应用中应使用日志）
			fmt.Printf("MCP 服务器启动失败: %v\n", err)
		}
	}()

	return server, nil
}

// ============== 兼容性适配器 (保留旧接口) ==============

// ToolDefinition 工具定义（兼容旧版本）
//
// Deprecated: 请使用 ai-core tool.Tool 接口
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]any         `json:"parameters,omitempty"`
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

// MCPToolAdapter 将 MCP 工具转换为可调用格式（兼容旧版本）
//
// Deprecated: 请使用 ConnectMCPServer 或 MCPProxyTool
type MCPToolAdapter struct {
	client   *Client
	mcpTools []Tool
}

// NewMCPToolAdapter 创建 MCP 工具适配器
//
// Deprecated: 请使用 ConnectMCPServer
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

// HexagonToolAdapter 将 Hexagon 工具转换为 MCP 工具（兼容旧版本）
//
// Deprecated: 请使用 Server.RegisterAICoreTool
type HexagonToolAdapter struct {
	server *Server
}

// NewHexagonToolAdapter 创建 Hexagon 工具适配器
//
// Deprecated: 请使用 Server.RegisterAICoreTools
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
func convertParamsToSchema(params map[string]any) *JSONSchema {
	if params == nil {
		return &JSONSchema{Type: "object"}
	}

	schema := &JSONSchema{
		Type:       "object",
		Properties: make(map[string]*JSONSchema),
	}

	for name, param := range params {
		switch v := param.(type) {
		case map[string]any:
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

// ============== 旧版便捷函数（兼容） ==============

// LoadMCPTools 从 MCP 服务器加载工具
//
// Deprecated: 请使用 ConnectMCPServer
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
//
// Deprecated: 请使用 ServeMCPToolsFromAICore
func ServeMCPTools(addr string, tools []ToolDefinition) (*Server, error) {
	server := NewServer(&ServerConfig{
		Name:    "hexagon-mcp-server",
		Version: "1.0.0",
		Addr:    addr,
	})

	adapter := NewHexagonToolAdapter(server)
	adapter.RegisterToolDefinitions(tools)

	go func() {
		_ = server.Start()
	}()

	return server, nil
}
