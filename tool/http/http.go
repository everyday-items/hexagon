// Package http 提供 HTTP API 调用工具
//
// 支持 REST API、GraphQL 等 HTTP 调用场景。
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/everyday-items/ai-core/tool"
)

// HTTPTool HTTP API 调用工具
type HTTPTool struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// Option HTTP 工具选项
type Option func(*HTTPTool)

// WithBaseURL 设置基础 URL
func WithBaseURL(url string) Option {
	return func(t *HTTPTool) {
		t.baseURL = url
	}
}

// WithHeaders 设置默认请求头
func WithHeaders(headers map[string]string) Option {
	return func(t *HTTPTool) {
		t.headers = headers
	}
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(t *HTTPTool) {
		t.client.Timeout = timeout
	}
}

// NewHTTPTool 创建 HTTP 工具
func NewHTTPTool(opts ...Option) *HTTPTool {
	t := &HTTPTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers: make(map[string]string),
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// RequestInput GET 请求输入
type RequestInput struct {
	URL     string            `json:"url" description:"请求 URL"`
	Method  string            `json:"method" description:"HTTP 方法 (GET/POST/PUT/DELETE)"`
	Headers map[string]string `json:"headers,omitempty" description:"请求头"`
	Body    string            `json:"body,omitempty" description:"请求体 (JSON 字符串)"`
}

// RequestOutput 请求输出
type RequestOutput struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// Tools 返回 HTTP 相关工具集合
func Tools(opts ...Option) []tool.Tool {
	t := NewHTTPTool(opts...)

	return []tool.Tool{
		// GET 请求
		tool.NewFunc(
			"http_get",
			"发送 HTTP GET 请求获取数据",
			func(ctx context.Context, input struct {
				URL     string            `json:"url" description:"请求 URL"`
				Headers map[string]string `json:"headers,omitempty" description:"请求头"`
			}) (RequestOutput, error) {
				return t.request(ctx, "GET", input.URL, input.Headers, nil)
			},
		),

		// POST 请求
		tool.NewFunc(
			"http_post",
			"发送 HTTP POST 请求提交数据",
			func(ctx context.Context, input struct {
				URL     string            `json:"url" description:"请求 URL"`
				Headers map[string]string `json:"headers,omitempty" description:"请求头"`
				Body    string            `json:"body" description:"请求体 (JSON 字符串)"`
			}) (RequestOutput, error) {
				var body io.Reader
				if input.Body != "" {
					body = bytes.NewBufferString(input.Body)
				}
				return t.request(ctx, "POST", input.URL, input.Headers, body)
			},
		),

		// 通用请求
		tool.NewFunc(
			"http_request",
			"发送自定义 HTTP 请求",
			func(ctx context.Context, input RequestInput) (RequestOutput, error) {
				var body io.Reader
				if input.Body != "" {
					body = bytes.NewBufferString(input.Body)
				}
				return t.request(ctx, input.Method, input.URL, input.Headers, body)
			},
		),
	}
}

// request 执行 HTTP 请求
func (t *HTTPTool) request(ctx context.Context, method, url string, headers map[string]string, body io.Reader) (RequestOutput, error) {
	// 构建完整 URL
	if t.baseURL != "" && url[0] == '/' {
		url = t.baseURL + url
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return RequestOutput{}, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置默认请求头
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	// 设置自定义请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 如果有 body 且没有 Content-Type，设置为 JSON
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	resp, err := t.client.Do(req)
	if err != nil {
		return RequestOutput{}, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return RequestOutput{}, fmt.Errorf("读取响应失败: %w", err)
	}

	// 构建输出
	output := RequestOutput{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
		Body:       string(respBody),
	}

	// 复制响应头
	for k := range resp.Header {
		output.Headers[k] = resp.Header.Get(k)
	}

	return output, nil
}

// GraphQLTool GraphQL 工具
type GraphQLTool struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

// NewGraphQLTool 创建 GraphQL 工具
func NewGraphQLTool(endpoint string, opts ...Option) *GraphQLTool {
	t := &GraphQLTool{
		endpoint: endpoint,
		headers:  make(map[string]string),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// 应用选项
	httpTool := &HTTPTool{headers: t.headers, client: t.client}
	for _, opt := range opts {
		opt(httpTool)
	}

	return t
}

// GraphQLInput GraphQL 查询输入
type GraphQLInput struct {
	Query     string         `json:"query" description:"GraphQL 查询语句"`
	Variables map[string]any `json:"variables,omitempty" description:"查询变量"`
}

// GraphQLOutput GraphQL 查询输出
type GraphQLOutput struct {
	Data   any      `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

// Tool 返回 GraphQL 工具
func (t *GraphQLTool) Tool() tool.Tool {
	return tool.NewFunc(
		"graphql_query",
		"执行 GraphQL 查询",
		func(ctx context.Context, input GraphQLInput) (GraphQLOutput, error) {
			// 构建请求体
			reqBody, err := json.Marshal(map[string]any{
				"query":     input.Query,
				"variables": input.Variables,
			})
			if err != nil {
				return GraphQLOutput{}, fmt.Errorf("序列化请求失败: %w", err)
			}

			// 创建请求
			req, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, bytes.NewReader(reqBody))
			if err != nil {
				return GraphQLOutput{}, fmt.Errorf("创建请求失败: %w", err)
			}

			// 设置请求头
			req.Header.Set("Content-Type", "application/json")
			for k, v := range t.headers {
				req.Header.Set(k, v)
			}

			// 发送请求
			resp, err := t.client.Do(req)
			if err != nil {
				return GraphQLOutput{}, fmt.Errorf("请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 解析响应
			var result struct {
				Data   any `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return GraphQLOutput{}, fmt.Errorf("解析响应失败: %w", err)
			}

			// 构建输出
			output := GraphQLOutput{
				Data: result.Data,
			}

			if len(result.Errors) > 0 {
				output.Errors = make([]string, len(result.Errors))
				for i, e := range result.Errors {
					output.Errors[i] = e.Message
				}
			}

			return output, nil
		},
	)
}
