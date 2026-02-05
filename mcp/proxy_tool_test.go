package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMCPProxyTool 测试 MCP 代理工具
func TestMCPProxyTool(t *testing.T) {
	// 创建一个模拟的 MCP 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析请求失败: %v", err)
		}

		var resp MCPResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case MethodInitialize:
			resp.Result = map[string]any{
				"protocolVersion": MCPVersion,
				"capabilities":    map[string]any{},
				"serverInfo": map[string]any{
					"name":    "test-server",
					"version": "1.0.0",
				},
			}
		case MethodToolsList:
			resp.Result = map[string]any{
				"tools": []Tool{
					{
						Name:        "calculator",
						Description: "计算器工具",
						InputSchema: &JSONSchema{
							Type: "object",
							Properties: map[string]*JSONSchema{
								"a": {Type: "number", Description: "第一个数"},
								"b": {Type: "number", Description: "第二个数"},
							},
							Required: []string{"a", "b"},
						},
					},
				},
			}
		case MethodToolsCall:
			resp.Result = &ToolCallResponse{
				Content: []ContentBlock{
					{Type: "text", Text: "42"},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()

	// 测试 ConnectMCPServer
	t.Run("ConnectMCPServer", func(t *testing.T) {
		tools, err := ConnectMCPServer(ctx, server.URL)
		if err != nil {
			t.Fatalf("ConnectMCPServer 失败: %v", err)
		}

		if len(tools) != 1 {
			t.Fatalf("期望 1 个工具，实际 %d 个", len(tools))
		}

		tool := tools[0]
		if tool.Name() != "calculator" {
			t.Errorf("工具名称: 期望 calculator, 实际 %s", tool.Name())
		}

		if tool.Description() != "计算器工具" {
			t.Errorf("工具描述: 期望 计算器工具, 实际 %s", tool.Description())
		}

		// 测试 Schema
		schema := tool.Schema()
		if schema == nil {
			t.Fatal("Schema 不应为 nil")
		}

		if schema.Type != "object" {
			t.Errorf("Schema 类型: 期望 object, 实际 %s", schema.Type)
		}

		if len(schema.Required) != 2 {
			t.Errorf("Required 长度: 期望 2, 实际 %d", len(schema.Required))
		}
	})

	// 测试工具执行
	t.Run("Execute", func(t *testing.T) {
		tools, err := ConnectMCPServer(ctx, server.URL)
		if err != nil {
			t.Fatalf("ConnectMCPServer 失败: %v", err)
		}

		result, err := tools[0].Execute(ctx, map[string]any{
			"a": 20,
			"b": 22,
		})
		if err != nil {
			t.Fatalf("Execute 失败: %v", err)
		}

		if !result.Success {
			t.Errorf("执行应该成功: %s", result.Error)
		}

		// 输出应该是 "42"
		if result.Output != "42" {
			t.Errorf("输出: 期望 42, 实际 %v", result.Output)
		}
	})

	// 测试参数验证
	t.Run("Validate", func(t *testing.T) {
		tools, err := ConnectMCPServer(ctx, server.URL)
		if err != nil {
			t.Fatalf("ConnectMCPServer 失败: %v", err)
		}

		// 缺少必填字段
		err = tools[0].Validate(map[string]any{
			"a": 10,
			// 缺少 "b"
		})
		if err == nil {
			t.Error("缺少必填字段应该返回错误")
		}

		// 所有字段都存在
		err = tools[0].Validate(map[string]any{
			"a": 10,
			"b": 20,
		})
		if err != nil {
			t.Errorf("验证应该通过: %v", err)
		}
	})
}

// TestMCPToolSet 测试 MCP 工具集合
func TestMCPToolSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp MCPResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case MethodInitialize:
			resp.Result = map[string]any{
				"protocolVersion": MCPVersion,
				"capabilities":    map[string]any{},
			}
		case MethodToolsList:
			resp.Result = map[string]any{
				"tools": []Tool{
					{Name: "tool1", Description: "工具1"},
					{Name: "tool2", Description: "工具2"},
					{Name: "tool3", Description: "工具3"},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()

	// 测试 ConnectMCPServerWithToolSet
	toolSet, err := ConnectMCPServerWithToolSet(ctx, server.URL)
	if err != nil {
		t.Fatalf("ConnectMCPServerWithToolSet 失败: %v", err)
	}
	defer toolSet.Close()

	// 测试 Tools()
	tools := toolSet.Tools()
	if len(tools) != 3 {
		t.Errorf("工具数量: 期望 3, 实际 %d", len(tools))
	}

	// 测试 Get()
	tool, ok := toolSet.Get("tool2")
	if !ok {
		t.Error("应该能找到 tool2")
	}
	if tool.Name() != "tool2" {
		t.Errorf("工具名称: 期望 tool2, 实际 %s", tool.Name())
	}

	// 测试不存在的工具
	_, ok = toolSet.Get("nonexistent")
	if ok {
		t.Error("不应该找到不存在的工具")
	}
}

// TestMCPProxyToolErrorHandling 测试错误处理
func TestMCPProxyToolErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp MCPResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case MethodInitialize:
			resp.Result = map[string]any{
				"protocolVersion": MCPVersion,
				"capabilities":    map[string]any{},
			}
		case MethodToolsList:
			resp.Result = map[string]any{
				"tools": []Tool{
					{Name: "failing_tool", Description: "会失败的工具"},
				},
			}
		case MethodToolsCall:
			// 返回错误响应
			resp.Result = &ToolCallResponse{
				Content: []ContentBlock{
					{Type: "text", Text: "执行失败: 参数无效"},
				},
				IsError: true,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()

	tools, err := ConnectMCPServer(ctx, server.URL)
	if err != nil {
		t.Fatalf("ConnectMCPServer 失败: %v", err)
	}

	result, err := tools[0].Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Execute 不应返回错误: %v", err)
	}

	// 工具执行失败应该通过 result.Success 表示
	if result.Success {
		t.Error("执行应该失败")
	}

	if result.Error == "" {
		t.Error("应该有错误信息")
	}
}
