// Package graph 提供 Hexagon AI Agent 框架的图编排引擎
//
// distributed.go 实现分布式图执行支持：
//   - RemoteNodeExecutor: 远程节点执行器接口
//   - HTTPNodeExecutor: 基于 HTTP 的远程节点执行
//   - DistributedGraph: 支持节点分布在不同机器上执行
//   - NodePlacement: 节点放置策略
//
// 对标 LangGraph Cloud 的分布式调度能力。
//
// 使用示例：
//
//	// 注册远程执行器
//	registry := NewRemoteRegistry()
//	registry.Register("gpu-node", NewHTTPNodeExecutor("http://gpu-server:8080"))
//
//	// 配置节点分布
//	graph := NewGraph[MyState]("distributed-flow").
//	    AddNode("preprocess", preprocessHandler).
//	    AddNode("inference", inferenceHandler).
//	    WithNodePlacement("inference", "gpu-node").
//	    Build()
//
//	result, err := graph.RunDistributed(ctx, state, registry)
package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// RemoteNodeExecutor 远程节点执行器接口
// 实现此接口以支持不同的远程执行方式（HTTP、gRPC、消息队列等）
type RemoteNodeExecutor interface {
	// Execute 远程执行节点
	// nodeName 节点名称
	// stateData 序列化后的状态数据
	// 返回序列化后的结果状态数据
	Execute(ctx context.Context, nodeName string, stateData []byte) ([]byte, error)

	// Ping 检查远程节点是否可用
	Ping(ctx context.Context) error

	// Name 返回执行器名称
	Name() string
}

// ============== HTTP 远程执行器 ==============

// HTTPNodeExecutor 基于 HTTP 的远程节点执行器
type HTTPNodeExecutor struct {
	name       string
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// HTTPExecutorOption HTTP 执行器选项
type HTTPExecutorOption func(*HTTPNodeExecutor)

// WithHTTPTimeout 设置 HTTP 超时
func WithHTTPTimeout(timeout time.Duration) HTTPExecutorOption {
	return func(e *HTTPNodeExecutor) {
		e.httpClient.Timeout = timeout
	}
}

// WithHTTPHeader 添加自定义请求头
func WithHTTPHeader(key, value string) HTTPExecutorOption {
	return func(e *HTTPNodeExecutor) {
		e.headers[key] = value
	}
}

// NewHTTPNodeExecutor 创建 HTTP 远程节点执行器
func NewHTTPNodeExecutor(name, baseURL string, opts ...HTTPExecutorOption) *HTTPNodeExecutor {
	e := &HTTPNodeExecutor{
		name:    name,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers: make(map[string]string),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute 通过 HTTP 远程执行节点
func (e *HTTPNodeExecutor) Execute(ctx context.Context, nodeName string, stateData []byte) ([]byte, error) {
	// 构建请求体
	reqBody := map[string]any{
		"node_name":  nodeName,
		"state_data": stateData,
	}
	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/nodes/%s/execute", e.baseURL, nodeName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("远程执行失败: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("远程执行返回错误 (状态码 %d): %s", resp.StatusCode, string(respData))
	}

	// 解析响应
	var result struct {
		StateData json.RawMessage `json:"state_data"`
		Error     string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("远程节点执行错误: %s", result.Error)
	}

	return result.StateData, nil
}

// Ping 检查远程节点是否可用
func (e *HTTPNodeExecutor) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/health", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("远程节点不可达: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("远程节点健康检查失败: 状态码 %d", resp.StatusCode)
	}
	return nil
}

// Name 返回执行器名称
func (e *HTTPNodeExecutor) Name() string {
	return e.name
}

// ============== 远程注册表 ==============

// RemoteRegistry 远程执行器注册表
type RemoteRegistry struct {
	mu        sync.RWMutex
	executors map[string]RemoteNodeExecutor
}

// NewRemoteRegistry 创建远程注册表
func NewRemoteRegistry() *RemoteRegistry {
	return &RemoteRegistry{
		executors: make(map[string]RemoteNodeExecutor),
	}
}

// Register 注册远程执行器
func (r *RemoteRegistry) Register(name string, executor RemoteNodeExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[name] = executor
}

// Get 获取远程执行器
func (r *RemoteRegistry) Get(name string) (RemoteNodeExecutor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[name]
	return e, ok
}

// HealthCheck 检查所有远程节点的健康状态
func (r *RemoteRegistry) HealthCheck(ctx context.Context) map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]error, len(r.executors))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, executor := range r.executors {
		wg.Add(1)
		go func(n string, e RemoteNodeExecutor) {
			defer wg.Done()
			err := e.Ping(ctx)
			mu.Lock()
			results[n] = err
			mu.Unlock()
		}(name, executor)
	}

	wg.Wait()
	return results
}

// ============== 节点放置 ==============

// NodePlacement 节点放置配置
type NodePlacement struct {
	// NodeName 节点名称
	NodeName string

	// ExecutorName 远程执行器名称
	ExecutorName string

	// Fallback 是否在远程不可用时降级到本地执行
	Fallback bool
}

// WithNodePlacement 配置节点的远程执行位置
func (b *GraphBuilder[S]) WithNodePlacement(nodeName, executorName string) *GraphBuilder[S] {
	if b.err != nil {
		return b
	}

	placements, _ := b.graph.Metadata["__node_placements"].([]NodePlacement)
	placements = append(placements, NodePlacement{
		NodeName:     nodeName,
		ExecutorName: executorName,
		Fallback:     true,
	})
	b.graph.Metadata["__node_placements"] = placements

	return b
}

// WithNodePlacementNoFallback 配置节点的远程执行位置（不降级）
func (b *GraphBuilder[S]) WithNodePlacementNoFallback(nodeName, executorName string) *GraphBuilder[S] {
	if b.err != nil {
		return b
	}

	placements, _ := b.graph.Metadata["__node_placements"].([]NodePlacement)
	placements = append(placements, NodePlacement{
		NodeName:     nodeName,
		ExecutorName: executorName,
		Fallback:     false,
	})
	b.graph.Metadata["__node_placements"] = placements

	return b
}

// GetNodePlacements 获取所有节点放置配置
func (g *Graph[S]) GetNodePlacements() []NodePlacement {
	placements, _ := g.Metadata["__node_placements"].([]NodePlacement)
	return placements
}

// RunDistributed 分布式执行图
// 支持将节点分发到远程机器执行
//
// 实现采用 save/restore handler 模式：在执行前保存原始 handler，
// 替换为远程执行包装器，执行完毕后恢复原始 handler。
// 这样避免永久修改节点的 Handler，确保同一个 Graph 可以安全地
// 多次在本地和分布式模式间切换。
func (g *Graph[S]) RunDistributed(ctx context.Context, initialState S, registry *RemoteRegistry, opts ...RunOption) (S, error) {
	if !g.compiled {
		return initialState, fmt.Errorf("graph not compiled")
	}

	// 构建节点放置映射
	placementMap := make(map[string]NodePlacement)
	for _, p := range g.GetNodePlacements() {
		placementMap[p.NodeName] = p
	}

	// 保存原始 handler 并替换为远程执行包装器
	// 使用 defer 确保即使 Run 出错也能恢复
	type savedHandler struct {
		nodeName string
		handler  func(context.Context, S) (S, error)
	}
	var saved []savedHandler

	defer func() {
		// 恢复所有被替换的原始 handler
		for _, s := range saved {
			if node, ok := g.Nodes[s.nodeName]; ok {
				node.Handler = s.handler
			}
		}
	}()

	for nodeName, placement := range placementMap {
		node, ok := g.Nodes[nodeName]
		if !ok {
			continue
		}

		executor, hasExecutor := registry.Get(placement.ExecutorName)
		if !hasExecutor {
			if !placement.Fallback {
				return initialState, fmt.Errorf("远程执行器 %q 未注册且不允许降级", placement.ExecutorName)
			}
			continue // 降级到本地执行
		}

		// 保存原始 handler
		originalHandler := node.Handler
		saved = append(saved, savedHandler{nodeName: nodeName, handler: originalHandler})

		// 捕获循环变量
		capturedNodeName := nodeName
		capturedPlacement := placement

		// 替换为远程执行包装器
		node.Handler = func(ctx context.Context, state S) (S, error) {
			// 序列化状态
			stateData, err := json.Marshal(state)
			if err != nil {
				if capturedPlacement.Fallback {
					return originalHandler(ctx, state)
				}
				return state, fmt.Errorf("序列化状态失败: %w", err)
			}

			// 远程执行
			resultData, err := executor.Execute(ctx, capturedNodeName, stateData)
			if err != nil {
				if capturedPlacement.Fallback {
					return originalHandler(ctx, state)
				}
				return state, fmt.Errorf("远程执行节点 %q 失败: %w", capturedNodeName, err)
			}

			// 反序列化结果
			var resultState S
			if err := json.Unmarshal(resultData, &resultState); err != nil {
				if capturedPlacement.Fallback {
					return originalHandler(ctx, state)
				}
				return state, fmt.Errorf("反序列化远程结果失败: %w", err)
			}

			return resultState, nil
		}
	}

	// 使用标准执行器执行
	return g.Run(ctx, initialState, opts...)
}
