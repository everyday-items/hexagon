// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件实现 Agent 网络通信的 Mock：
//   - MockNetwork: 模拟 Agent 网络通信，支持消息发送、广播和记录
//   - MockMessage: 记录网络中传递的消息
//
// 注意：为避免与 agent 包的循环导入，本 Mock 使用字符串 ID 标识 Agent，
// 不直接引用 agent.Agent 接口。
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MessageType 消息类型（与 agent.MessageType 对应，但独立定义以避免循环导入）
type MessageType string

const (
	// MessageTypeRequest 请求消息
	MessageTypeRequest MessageType = "request"
	// MessageTypeResponse 响应消息
	MessageTypeResponse MessageType = "response"
	// MessageTypeBroadcast 广播消息
	MessageTypeBroadcast MessageType = "broadcast"
)

// MockMessage 记录网络中传递的消息
type MockMessage struct {
	// From 发送者 Agent ID
	From string

	// To 接收者 Agent ID（空字符串表示广播）
	To string

	// Content 消息内容
	Content any

	// Type 消息类型
	Type MessageType

	// Time 发送时间
	Time time.Time
}

// MockNetwork Agent 网络通信 Mock
//
// 提供简化的网络通信模拟能力，不需要启动真正的消息路由器。
// 使用字符串 ID 标识 Agent，不依赖 agent 包以避免循环导入。
// 所有消息操作都会被记录到 messages 列表中，供测试断言使用。
//
// 线程安全：所有方法都使用读写锁保护。
type MockNetwork struct {
	agents   map[string]*mockNetworkAgent
	messages []MockMessage
	mu       sync.RWMutex
}

// mockNetworkAgent 内部 Agent 包装
type mockNetworkAgent struct {
	id    string
	name  string
	inbox []MockMessage
}

// NewMockNetwork 创建 Mock 网络
func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		agents:   make(map[string]*mockNetworkAgent),
		messages: make([]MockMessage, 0),
	}
}

// RegisterAgent 注册 Agent 到网络（使用字符串 ID 和名称）
func (n *MockNetwork) RegisterAgent(id, name string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.agents[id]; exists {
		return fmt.Errorf("agent %s already registered", id)
	}

	n.agents[id] = &mockNetworkAgent{
		id:    id,
		name:  name,
		inbox: make([]MockMessage, 0),
	}
	return nil
}

// Unregister 从网络注销 Agent
func (n *MockNetwork) Unregister(agentID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.agents[agentID]; !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	delete(n.agents, agentID)
	return nil
}

// Send 发送消息给指定 Agent
//
// 消息会被记录到 messages 列表中，同时投递到目标 Agent 的收件箱。
func (n *MockNetwork) Send(_ context.Context, from, to string, content any) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.agents[from]; !ok {
		return fmt.Errorf("sender agent %s not found", from)
	}

	target, ok := n.agents[to]
	if !ok {
		return fmt.Errorf("target agent %s not found", to)
	}

	msg := MockMessage{
		From:    from,
		To:      to,
		Content: content,
		Type:    MessageTypeRequest,
		Time:    time.Now(),
	}

	target.inbox = append(target.inbox, msg)
	n.messages = append(n.messages, msg)

	return nil
}

// Broadcast 广播消息给所有 Agent（除发送者自身外）
func (n *MockNetwork) Broadcast(_ context.Context, from string, content any) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.agents[from]; !ok {
		return fmt.Errorf("sender agent %s not found", from)
	}

	for id, a := range n.agents {
		if id == from {
			continue
		}
		msg := MockMessage{
			From:    from,
			To:      id,
			Content: content,
			Type:    MessageTypeBroadcast,
			Time:    time.Now(),
		}
		a.inbox = append(a.inbox, msg)
		n.messages = append(n.messages, msg)
	}

	return nil
}

// GetAgentName 获取已注册 Agent 的名称
func (n *MockNetwork) GetAgentName(agentID string) (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if a, ok := n.agents[agentID]; ok {
		return a.name, true
	}
	return "", false
}

// ListAgentIDs 列出所有已注册的 Agent ID
func (n *MockNetwork) ListAgentIDs() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ids := make([]string, 0, len(n.agents))
	for id := range n.agents {
		ids = append(ids, id)
	}
	return ids
}

// Inbox 获取指定 Agent 的收件箱消息
func (n *MockNetwork) Inbox(agentID string) ([]MockMessage, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	a, ok := n.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	msgs := make([]MockMessage, len(a.inbox))
	copy(msgs, a.inbox)
	return msgs, nil
}

// Messages 返回所有已记录的消息
func (n *MockNetwork) Messages() []MockMessage {
	n.mu.RLock()
	defer n.mu.RUnlock()

	msgs := make([]MockMessage, len(n.messages))
	copy(msgs, n.messages)
	return msgs
}

// MessageCount 返回已记录的消息数
func (n *MockNetwork) MessageCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.messages)
}

// MessagesFrom 返回指定发送者的消息
func (n *MockNetwork) MessagesFrom(agentID string) []MockMessage {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]MockMessage, 0)
	for _, msg := range n.messages {
		if msg.From == agentID {
			result = append(result, msg)
		}
	}
	return result
}

// MessagesTo 返回发送给指定接收者的消息
func (n *MockNetwork) MessagesTo(agentID string) []MockMessage {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]MockMessage, 0)
	for _, msg := range n.messages {
		if msg.To == agentID {
			result = append(result, msg)
		}
	}
	return result
}

// ClearInbox 清空指定 Agent 的收件箱
func (n *MockNetwork) ClearInbox(agentID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	a, ok := n.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	a.inbox = make([]MockMessage, 0)
	return nil
}

// Reset 重置网络状态（清空所有消息记录和收件箱）
func (n *MockNetwork) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.messages = make([]MockMessage, 0)
	for _, a := range n.agents {
		a.inbox = make([]MockMessage, 0)
	}
}

// AgentCount 返回已注册的 Agent 数量
func (n *MockNetwork) AgentCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.agents)
}
