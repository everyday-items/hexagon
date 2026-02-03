package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httptool "github.com/everyday-items/hexagon/tool/http"
)

// TestHTTPTools 测试 HTTP 工具
func TestHTTPTools(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/test":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"message": "success",
			})
		case "/api/echo":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	tools := httptool.Tools(httptool.WithBaseURL(server.URL))

	t.Run("HTTPGet", func(t *testing.T) {
		getTool := tools[0] // http_get
		if getTool.Name() != "http_get" {
			t.Errorf("Tool name = %s, want http_get", getTool.Name())
		}

		// 执行 GET 请求
		result, err := getTool.Execute(ctx, map[string]any{
			"url": "/api/test",
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		output, ok := result.Output.(httptool.RequestOutput)
		if !ok {
			t.Fatal("Output type mismatch")
		}

		if output.StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", output.StatusCode)
		}
	})

	t.Run("HTTPPost", func(t *testing.T) {
		postTool := tools[1] // http_post
		if postTool.Name() != "http_post" {
			t.Errorf("Tool name = %s, want http_post", postTool.Name())
		}

		result, err := postTool.Execute(ctx, map[string]any{
			"url":  "/api/echo",
			"body": `{"test": "data"}`,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		output, ok := result.Output.(httptool.RequestOutput)
		if !ok {
			t.Fatal("Output type mismatch")
		}

		if output.StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", output.StatusCode)
		}
	})
}

// TestGraphQLTool 测试 GraphQL 工具
func TestGraphQLTool(t *testing.T) {
	// 创建 GraphQL 测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}

		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"name": "test",
				},
			},
		})
	}))
	defer server.Close()

	ctx := context.Background()
	gqlTool := httptool.NewGraphQLTool(server.URL)
	tool := gqlTool.Tool()

	result, err := tool.Execute(ctx, map[string]any{
		"query": "{ user { name } }",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, ok := result.Output.(httptool.GraphQLOutput)
	if !ok {
		t.Fatal("Output type mismatch")
	}

	if output.Data == nil {
		t.Error("Data should not be nil")
	}
}
