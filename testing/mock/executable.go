// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件实现图编排 Executable 接口的 Mock：
//   - MockExecutable: 模拟 graph.Executable 接口，用于时间旅行调试器测试
package mock

import (
	"context"
	"sync"
)

// MockExecutable 模拟 Executable 接口
// 支持自定义节点执行结果、入口节点设置和调用追踪
type MockExecutable struct {
	mu         sync.RWMutex
	entryPoint string
	nodes      map[string]*mockNode
	calls      []string
}

// mockNode 模拟节点
type mockNode struct {
	name     string
	output   any
	nextNode string
	err      error
}

// NewMockExecutable 创建 Mock Executable
func NewMockExecutable(entryPoint string) *MockExecutable {
	return &MockExecutable{
		entryPoint: entryPoint,
		nodes:      make(map[string]*mockNode),
		calls:      make([]string, 0),
	}
}

// AddNode 添加节点定义
// output: 节点执行输出
// nextNode: 下一个节点 ID（空字符串表示结束）
// err: 节点执行错误
func (e *MockExecutable) AddNode(id, name string, output any, nextNode string, err error) *MockExecutable {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nodes[id] = &mockNode{
		name:     name,
		output:   output,
		nextNode: nextNode,
		err:      err,
	}
	return e
}

// ExecuteNode 执行单个节点
func (e *MockExecutable) ExecuteNode(ctx context.Context, nodeID string, state map[string]any) (output any, nextNode string, err error) {
	e.mu.Lock()
	e.calls = append(e.calls, nodeID)
	e.mu.Unlock()

	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	e.mu.RLock()
	node, exists := e.nodes[nodeID]
	e.mu.RUnlock()

	if !exists {
		return nil, "", nil
	}

	return node.output, node.nextNode, node.err
}

// GetEntryPoint 获取入口节点
func (e *MockExecutable) GetEntryPoint() string {
	return e.entryPoint
}

// GetNodeName 获取节点名称
func (e *MockExecutable) GetNodeName(nodeID string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if node, exists := e.nodes[nodeID]; exists {
		return node.name
	}
	return nodeID
}

// Calls 返回执行调用记录
func (e *MockExecutable) Calls() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	calls := make([]string, len(e.calls))
	copy(calls, e.calls)
	return calls
}
