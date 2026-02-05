package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTPTransport 测试 HTTP 传输层
func TestHTTPTransport(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST 方法, 实际 %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("期望 Content-Type application/json, 实际 %s", contentType)
		}

		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析请求失败: %v", err)
		}

		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"message": "hello",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 创建传输层
	transport := NewHTTPTransport(server.URL)

	ctx := context.Background()

	// 发送请求
	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  "test",
	}

	resp, err := transport.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send 失败: %v", err)
	}

	if resp.Result == nil {
		t.Error("响应结果不应为 nil")
	}

	// 测试关闭
	if err := transport.Close(); err != nil {
		t.Errorf("Close 失败: %v", err)
	}
}

// TestHTTPTransportOptions 测试传输层选项
func TestHTTPTransportOptions(t *testing.T) {
	// 测试自定义超时
	customClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	transport := NewHTTPTransport("http://localhost:8080",
		WithHTTPClient(customClient),
	)

	if transport.httpClient != customClient {
		t.Error("HTTP 客户端应该被设置为自定义客户端")
	}

	// 测试超时设置
	transport2 := NewHTTPTransport("http://localhost:8080",
		WithTimeout(60*time.Second),
	)

	if transport2.httpClient.Timeout != 60*time.Second {
		t.Errorf("超时时间应为 60s, 实际 %v", transport2.httpClient.Timeout)
	}
}

// TestHTTPTransportError 测试错误处理
func TestHTTPTransportError(t *testing.T) {
	// 测试服务器返回错误
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    ErrorCodeInternalError,
				Message: "内部错误",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx := context.Background()

	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  "test",
	}

	_, err := transport.Send(ctx, req)
	if err == nil {
		t.Error("应该返回错误")
	}
}

// TestTransportClient 测试基于 Transport 的客户端
func TestTransportClient(t *testing.T) {
	// 创建模拟服务器
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
				"capabilities": map[string]any{
					"tools": map[string]any{
						"listChanged": true,
					},
				},
				"serverInfo": map[string]any{
					"name":    "test-server",
					"version": "1.0.0",
				},
			}
		case MethodToolsList:
			resp.Result = map[string]any{
				"tools": []Tool{
					{Name: "test_tool", Description: "测试工具"},
				},
			}
		case MethodResourcesList:
			resp.Result = map[string]any{
				"resources": []Resource{
					{URI: "file:///test.txt", Name: "test.txt"},
				},
			}
		case MethodPromptsList:
			resp.Result = map[string]any{
				"prompts": []Prompt{
					{Name: "greeting", Description: "问候提示"},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	client := NewTransportClient(transport)
	defer client.Close()

	ctx := context.Background()

	// 测试初始化
	t.Run("Initialize", func(t *testing.T) {
		if err := client.Initialize(ctx); err != nil {
			t.Fatalf("Initialize 失败: %v", err)
		}

		caps := client.GetServerCapabilities()
		if caps == nil {
			t.Error("服务器能力不应为 nil")
		}

		if caps.Tools == nil {
			t.Error("工具能力不应为 nil")
		}
	})

	// 测试列出工具
	t.Run("ListTools", func(t *testing.T) {
		tools, err := client.ListTools(ctx)
		if err != nil {
			t.Fatalf("ListTools 失败: %v", err)
		}

		if len(tools) != 1 {
			t.Errorf("工具数量: 期望 1, 实际 %d", len(tools))
		}

		if tools[0].Name != "test_tool" {
			t.Errorf("工具名称: 期望 test_tool, 实际 %s", tools[0].Name)
		}
	})

	// 测试列出资源
	t.Run("ListResources", func(t *testing.T) {
		resources, err := client.ListResources(ctx)
		if err != nil {
			t.Fatalf("ListResources 失败: %v", err)
		}

		if len(resources) != 1 {
			t.Errorf("资源数量: 期望 1, 实际 %d", len(resources))
		}
	})

	// 测试列出提示
	t.Run("ListPrompts", func(t *testing.T) {
		prompts, err := client.ListPrompts(ctx)
		if err != nil {
			t.Fatalf("ListPrompts 失败: %v", err)
		}

		if len(prompts) != 1 {
			t.Errorf("提示数量: 期望 1, 实际 %d", len(prompts))
		}
	})

	// 测试获取底层传输
	t.Run("Transport", func(t *testing.T) {
		if client.Transport() != transport {
			t.Error("Transport() 应返回底层传输层")
		}
	})
}

// TestContextCancellation 测试上下文取消
func TestContextCancellation(t *testing.T) {
	// 创建一个会延迟响应的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &MCPRequest{
		JSONRPC: "2.0",
		Method:  "test",
	}

	_, err := transport.Send(ctx, req)
	if err == nil {
		t.Error("应该因为超时而返回错误")
	}
}

// TestAutoRequestID 测试自动请求 ID 分配
func TestAutoRequestID(t *testing.T) {
	var receivedIDs []int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		json.NewDecoder(r.Body).Decode(&req)

		// 记录收到的 ID
		if id, ok := req.ID.(float64); ok {
			receivedIDs = append(receivedIDs, int64(id))
		}

		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx := context.Background()

	// 发送多个请求
	for range 3 {
		req := &MCPRequest{
			JSONRPC: "2.0",
			Method:  "test",
		}
		transport.Send(ctx, req)
	}

	// 验证 ID 是递增的
	for i := 1; i < len(receivedIDs); i++ {
		if receivedIDs[i] <= receivedIDs[i-1] {
			t.Errorf("请求 ID 应该递增: %v", receivedIDs)
		}
	}
}
