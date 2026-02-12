package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/everyday-items/ai-core/schema"
	"github.com/everyday-items/ai-core/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockTool 测试用 ai-core 工具
type mockTool struct {
	name        string
	description string
	schema      *schema.Schema
	executeFn   func(ctx context.Context, args map[string]any) (tool.Result, error)
}

func (t *mockTool) Name() string                                       { return t.name }
func (t *mockTool) Description() string                                { return t.description }
func (t *mockTool) Schema() *schema.Schema                             { return t.schema }
func (t *mockTool) Validate(args map[string]any) error                 { return nil }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	if t.executeFn != nil {
		return t.executeFn(ctx, args)
	}
	return tool.Result{Output: "ok"}, nil
}

var _ tool.Tool = (*mockTool)(nil)

// connectInMemory 创建 in-memory 的 client-server 连接用于测试
func connectInMemory(ctx context.Context, server *sdkmcp.Server) (*sdkmcp.ClientSession, error) {
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		return nil, err
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	return client.Connect(ctx, t2, nil)
}

// ============== Schema 转换测试 ==============

func TestSdkSchemaToAICore_Nil(t *testing.T) {
	result := sdkSchemaToAICore(nil)
	if result != nil {
		t.Fatal("期望 nil schema 转换为 nil")
	}
}

func TestSdkSchemaToAICore_MapInput(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "用户姓名",
			},
			"age": map[string]any{
				"type": "integer",
			},
		},
		"required": []any{"name"},
	}

	result := sdkSchemaToAICore(input)
	if result == nil {
		t.Fatal("转换结果不应为 nil")
	}
	if result.Type != "object" {
		t.Errorf("Type = %q, 期望 %q", result.Type, "object")
	}
	if len(result.Properties) != 2 {
		t.Errorf("Properties 数量 = %d, 期望 2", len(result.Properties))
	}
	nameProp := result.Properties["name"]
	if nameProp == nil || nameProp.Type != "string" || nameProp.Description != "用户姓名" {
		t.Errorf("name 属性解析错误: %+v", nameProp)
	}
	if len(result.Required) != 1 || result.Required[0] != "name" {
		t.Errorf("Required = %v, 期望 [name]", result.Required)
	}
}

func TestSdkSchemaToAICore_JSONRoundTrip(t *testing.T) {
	// 测试非 map 类型通过 JSON 往返转换
	type testSchema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	input := testSchema{
		Type: "object",
		Properties: map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}

	result := sdkSchemaToAICore(input)
	if result == nil {
		t.Fatal("JSON 往返转换结果不应为 nil")
	}
	if result.Type != "object" {
		t.Errorf("Type = %q, 期望 %q", result.Type, "object")
	}
}

func TestSdkSchemaToAICore_Items(t *testing.T) {
	input := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
		},
	}

	result := sdkSchemaToAICore(input)
	if result == nil || result.Items == nil {
		t.Fatal("items 解析失败")
	}
	if result.Items.Type != "string" {
		t.Errorf("Items.Type = %q, 期望 %q", result.Items.Type, "string")
	}
}

func TestSdkSchemaToAICore_Enum(t *testing.T) {
	input := map[string]any{
		"type": "string",
		"enum": []any{"red", "green", "blue"},
	}

	result := sdkSchemaToAICore(input)
	if result == nil {
		t.Fatal("enum 解析失败")
	}
	if len(result.Enum) != 3 {
		t.Errorf("Enum 数量 = %d, 期望 3", len(result.Enum))
	}
}

func TestAicoreSchemaToSDK_Nil(t *testing.T) {
	result := aicoreSchemaToSDK(nil)
	if result["type"] != "object" {
		t.Errorf("nil schema 应转换为 object 类型, 实际: %v", result["type"])
	}
	props, ok := result["properties"].(map[string]any)
	if !ok || len(props) != 0 {
		t.Errorf("nil schema 应有空 properties")
	}
}

func TestAicoreSchemaToSDK_Full(t *testing.T) {
	s := &schema.Schema{
		Type:        "object",
		Description: "测试工具",
		Properties: map[string]*schema.Schema{
			"input": {Type: "string", Description: "输入"},
		},
		Required: []string{"input"},
	}

	result := aicoreSchemaToSDK(s)
	if result["type"] != "object" {
		t.Errorf("type = %v, 期望 object", result["type"])
	}
	if result["description"] != "测试工具" {
		t.Errorf("description = %v, 期望 测试工具", result["description"])
	}
	props := result["properties"].(map[string]any)
	inputProp := props["input"].(map[string]any)
	if inputProp["type"] != "string" {
		t.Errorf("input.type = %v, 期望 string", inputProp["type"])
	}
	req := result["required"].([]string)
	if len(req) != 1 || req[0] != "input" {
		t.Errorf("required = %v, 期望 [input]", req)
	}
}

func TestSchemaRoundTrip_V2(t *testing.T) {
	// ai-core -> SDK -> ai-core 往返转换
	original := &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Schema{
			"query":  {Type: "string", Description: "搜索查询"},
			"limit":  {Type: "integer"},
			"tags":   {Type: "array", Items: &schema.Schema{Type: "string"}},
			"status": {Type: "string", Enum: []any{"active", "inactive"}},
		},
		Required: []string{"query"},
	}

	sdkSchema := aicoreSchemaToSDK(original)
	// 模拟通过 JSON 传输（服务端发送 → 客户端接收会变成 map[string]any）
	data, err := json.Marshal(sdkSchema)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	var received map[string]any
	if err := json.Unmarshal(data, &received); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	result := sdkSchemaToAICore(received)
	if result.Type != original.Type {
		t.Errorf("往返 Type = %q, 期望 %q", result.Type, original.Type)
	}
	if len(result.Properties) != len(original.Properties) {
		t.Errorf("往返 Properties 数量 = %d, 期望 %d", len(result.Properties), len(original.Properties))
	}
	if result.Properties["query"].Description != "搜索查询" {
		t.Errorf("往返 query.Description = %q, 期望 %q", result.Properties["query"].Description, "搜索查询")
	}
	if result.Items != nil {
		t.Error("顶层不应有 Items")
	}
	if result.Properties["tags"].Items == nil || result.Properties["tags"].Items.Type != "string" {
		t.Error("tags.items 往返失败")
	}
	if len(result.Required) != 1 || result.Required[0] != "query" {
		t.Errorf("往返 Required = %v, 期望 [query]", result.Required)
	}
}

// ============== ServerV2 测试 ==============

func TestNewMCPServerV2(t *testing.T) {
	s := NewMCPServerV2("test-server", "0.1.0")
	if s == nil {
		t.Fatal("NewMCPServerV2 不应返回 nil")
	}
	if s.Server() == nil {
		t.Fatal("Server() 不应返回 nil")
	}
}

func TestServerV2_RegisterAndCallTool(t *testing.T) {
	ctx := context.Background()

	// 创建服务器并注册工具
	s := NewMCPServerV2("test", "0.1.0")
	s.RegisterTool(&mockTool{
		name:        "greet",
		description: "打招呼",
		schema: &schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Schema{
				"name": {Type: "string", Description: "姓名"},
			},
			Required: []string{"name"},
		},
		executeFn: func(ctx context.Context, args map[string]any) (tool.Result, error) {
			name := args["name"].(string)
			return tool.Result{Output: fmt.Sprintf("你好, %s!", name)}, nil
		},
	})

	// 通过 in-memory transport 连接
	session, err := connectInMemory(ctx, s.Server())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer session.Close()

	// 列举工具
	toolsResult, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(toolsResult.Tools) != 1 {
		t.Fatalf("工具数量 = %d, 期望 1", len(toolsResult.Tools))
	}
	if toolsResult.Tools[0].Name != "greet" {
		t.Errorf("工具名称 = %q, 期望 %q", toolsResult.Tools[0].Name, "greet")
	}

	// 调用工具
	callResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "greet",
		Arguments: map[string]any{"name": "世界"},
	})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if callResult.IsError {
		t.Fatal("CallTool 不应返回错误")
	}
	if len(callResult.Content) != 1 {
		t.Fatalf("Content 数量 = %d, 期望 1", len(callResult.Content))
	}
	tc, ok := callResult.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatal("Content 不是 TextContent 类型")
	}
	if tc.Text != "你好, 世界!" {
		t.Errorf("结果 = %q, 期望 %q", tc.Text, "你好, 世界!")
	}
}

func TestServerV2_RegisterTools(t *testing.T) {
	ctx := context.Background()

	s := NewMCPServerV2("test", "0.1.0")
	s.RegisterTools(
		&mockTool{name: "tool1", description: "工具1", schema: &schema.Schema{Type: "object"}},
		&mockTool{name: "tool2", description: "工具2", schema: &schema.Schema{Type: "object"}},
	)

	session, err := connectInMemory(ctx, s.Server())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer session.Close()

	toolsResult, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(toolsResult.Tools) != 2 {
		t.Errorf("工具数量 = %d, 期望 2", len(toolsResult.Tools))
	}
}

func TestServerV2_ToolExecutionError(t *testing.T) {
	ctx := context.Background()

	s := NewMCPServerV2("test", "0.1.0")
	s.RegisterTool(&mockTool{
		name:   "fail",
		schema: &schema.Schema{Type: "object"},
		executeFn: func(ctx context.Context, args map[string]any) (tool.Result, error) {
			return tool.Result{}, fmt.Errorf("执行失败: 内部错误")
		},
	})

	session, err := connectInMemory(ctx, s.Server())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer session.Close()

	callResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "fail",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool 不应返回 transport 错误: %v", err)
	}
	if !callResult.IsError {
		t.Fatal("期望 IsError = true")
	}
	tc := callResult.Content[0].(*sdkmcp.TextContent)
	if tc.Text != "执行失败: 内部错误" {
		t.Errorf("错误消息 = %q, 期望包含执行失败", tc.Text)
	}
}

func TestServerV2_NilSchema(t *testing.T) {
	ctx := context.Background()

	// nil schema 应该被转换为空 object schema
	s := NewMCPServerV2("test", "0.1.0")
	s.RegisterTool(&mockTool{
		name:   "no-schema",
		schema: nil,
	})

	session, err := connectInMemory(ctx, s.Server())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer session.Close()

	toolsResult, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(toolsResult.Tools) != 1 {
		t.Fatalf("工具数量 = %d, 期望 1", len(toolsResult.Tools))
	}

	// nil schema 转换后应该是 object 类型
	inputSchema := toolsResult.Tools[0].InputSchema
	schemaMap, ok := inputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema 类型 = %T, 期望 map[string]any", inputSchema)
	}
	if schemaMap["type"] != "object" {
		t.Errorf("InputSchema.type = %v, 期望 object", schemaMap["type"])
	}
}

// ============== 端到端 Client/Server 测试 ==============

func TestEndToEnd_ConnectAndCall(t *testing.T) {
	ctx := context.Background()

	// 创建服务器并注册工具
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "e2e-server", Version: "0.1.0"}, nil)
	server.AddTool(&sdkmcp.Tool{
		Name:        "add",
		Description: "加法运算",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []any{"a", "b"},
		},
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{
				&sdkmcp.TextContent{Text: fmt.Sprintf("%.0f", args.A+args.B)},
			},
		}, nil
	})

	// 使用 ConnectMCPServerV2 连接
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("Server 连接失败: %v", err)
	}

	tools, closer, err := ConnectMCPServerV2(ctx, t2)
	if err != nil {
		t.Fatalf("ConnectMCPServerV2 失败: %v", err)
	}
	defer closer.Close()

	// 验证工具列表
	if len(tools) != 1 {
		t.Fatalf("工具数量 = %d, 期望 1", len(tools))
	}

	addTool := tools[0]
	if addTool.Name() != "add" {
		t.Errorf("工具名 = %q, 期望 %q", addTool.Name(), "add")
	}
	if addTool.Description() != "加法运算" {
		t.Errorf("工具描述 = %q, 期望 %q", addTool.Description(), "加法运算")
	}

	// 验证 Schema 转换
	s := addTool.Schema()
	if s == nil {
		t.Fatal("Schema 不应为 nil")
	}
	if s.Type != "object" {
		t.Errorf("Schema.Type = %q, 期望 %q", s.Type, "object")
	}
	if len(s.Properties) != 2 {
		t.Errorf("Schema.Properties 数量 = %d, 期望 2", len(s.Properties))
	}
	if len(s.Required) != 2 {
		t.Errorf("Schema.Required 数量 = %d, 期望 2", len(s.Required))
	}

	// 执行工具
	result, err := addTool.Execute(ctx, map[string]any{"a": 3.0, "b": 4.0})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result.Output != "7" {
		t.Errorf("Output = %v, 期望 %q", result.Output, "7")
	}
}

func TestEndToEnd_HexagonServerAndClient(t *testing.T) {
	ctx := context.Background()

	// 使用 Hexagon ServerV2 创建 MCP 服务器
	hexServer := NewMCPServerV2("hexagon-test", "0.1.0")
	hexServer.RegisterTool(&mockTool{
		name:        "concat",
		description: "字符串拼接",
		schema: &schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Schema{
				"a": {Type: "string"},
				"b": {Type: "string"},
			},
			Required: []string{"a", "b"},
		},
		executeFn: func(ctx context.Context, args map[string]any) (tool.Result, error) {
			a, _ := args["a"].(string)
			b, _ := args["b"].(string)
			return tool.Result{Output: a + b}, nil
		},
	})

	// 使用 ConnectMCPServerV2 作为客户端连接
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := hexServer.Server().Connect(ctx, t1, nil); err != nil {
		t.Fatalf("Server 连接失败: %v", err)
	}

	tools, closer, err := ConnectMCPServerV2(ctx, t2)
	if err != nil {
		t.Fatalf("ConnectMCPServerV2 失败: %v", err)
	}
	defer closer.Close()

	if len(tools) != 1 {
		t.Fatalf("工具数量 = %d, 期望 1", len(tools))
	}

	// 调用拼接工具
	result, err := tools[0].Execute(ctx, map[string]any{"a": "Hello", "b": "World"})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result.Output != "HelloWorld" {
		t.Errorf("Output = %v, 期望 %q", result.Output, "HelloWorld")
	}
}

func TestEndToEnd_ToolWithError(t *testing.T) {
	ctx := context.Background()

	hexServer := NewMCPServerV2("test", "0.1.0")
	hexServer.RegisterTool(&mockTool{
		name:   "error-tool",
		schema: &schema.Schema{Type: "object"},
		executeFn: func(ctx context.Context, args map[string]any) (tool.Result, error) {
			return tool.Result{}, fmt.Errorf("something went wrong")
		},
	})

	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := hexServer.Server().Connect(ctx, t1, nil); err != nil {
		t.Fatalf("Server 连接失败: %v", err)
	}

	tools, closer, err := ConnectMCPServerV2(ctx, t2)
	if err != nil {
		t.Fatalf("ConnectMCPServerV2 失败: %v", err)
	}
	defer closer.Close()

	// 调用会失败的工具
	_, err = tools[0].Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// ============== MCPProxyToolV2 验证测试 ==============

func TestMCPProxyToolV2_Validate(t *testing.T) {
	proxy := &MCPProxyToolV2{
		mcpTool: &sdkmcp.Tool{Name: "test"},
		cachedSchema: &schema.Schema{
			Type:     "object",
			Required: []string{"name", "age"},
		},
	}

	// 缺少必需参数
	err := proxy.Validate(map[string]any{"name": "test"})
	if err == nil {
		t.Fatal("缺少 age 参数应返回错误")
	}

	// 完整参数
	err = proxy.Validate(map[string]any{"name": "test", "age": 20})
	if err != nil {
		t.Fatalf("完整参数不应返回错误: %v", err)
	}
}

func TestMCPProxyToolV2_ValidateNilSchema(t *testing.T) {
	proxy := &MCPProxyToolV2{
		mcpTool:      &sdkmcp.Tool{Name: "test"},
		cachedSchema: nil,
	}

	// nil schema 应该总是通过校验
	err := proxy.Validate(map[string]any{"anything": "goes"})
	if err != nil {
		t.Fatalf("nil schema 不应返回错误: %v", err)
	}
}

func TestSessionCloser_Close(t *testing.T) {
	ctx := context.Background()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	server.AddTool(&sdkmcp.Tool{
		Name:        "dummy",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{}, nil
	})

	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	tools, closer, err := ConnectMCPServerV2(ctx, t2)
	if err != nil {
		t.Fatalf("ConnectMCPServerV2 失败: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("工具数量 = %d, 期望 1", len(tools))
	}

	// 正常关闭
	if err := closer.Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
}
