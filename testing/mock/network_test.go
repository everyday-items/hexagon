// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件包含 MockNetwork 的全面测试：
//   - 创建和注册: 网络创建、Agent 注册和注销
//   - 消息发送: 点对点发送、广播
//   - 消息查询: 按发送者/接收者查询消息
//   - 收件箱管理: 获取和清空收件箱
//   - 并发安全: 多 goroutine 并发操作
package mock

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestNewMockNetwork 测试创建 Mock 网络
func TestNewMockNetwork(t *testing.T) {
	n := NewMockNetwork()

	if n.AgentCount() != 0 {
		t.Errorf("期望初始 Agent 数量为 0，实际为 %d", n.AgentCount())
	}

	if n.MessageCount() != 0 {
		t.Errorf("期望初始消息数量为 0，实际为 %d", n.MessageCount())
	}

	messages := n.Messages()
	if len(messages) != 0 {
		t.Errorf("期望初始消息列表为空")
	}
}

// TestMockNetworkRegister 测试注册 Agent
func TestMockNetworkRegister(t *testing.T) {
	n := NewMockNetwork()

	// 注册两个 Agent
	if err := n.RegisterAgent("agent-1", "agent1"); err != nil {
		t.Fatalf("注册 agent1 失败: %v", err)
	}
	if err := n.RegisterAgent("agent-2", "agent2"); err != nil {
		t.Fatalf("注册 agent2 失败: %v", err)
	}

	if n.AgentCount() != 2 {
		t.Errorf("期望 2 个 Agent，实际为 %d", n.AgentCount())
	}

	// 重复注册应该返回错误
	err := n.RegisterAgent("agent-1", "agent1")
	if err == nil {
		t.Fatal("期望重复注册返回错误")
	}
}

// TestMockNetworkUnregister 测试注销 Agent
func TestMockNetworkUnregister(t *testing.T) {
	n := NewMockNetwork()

	_ = n.RegisterAgent("agent-1", "agent1")

	// 注销
	err := n.Unregister("agent-1")
	if err != nil {
		t.Fatalf("注销失败: %v", err)
	}

	if n.AgentCount() != 0 {
		t.Errorf("期望注销后 Agent 数量为 0")
	}

	// 注销不存在的 Agent
	err = n.Unregister("nonexistent")
	if err == nil {
		t.Fatal("期望注销不存在的 Agent 返回错误")
	}
}

// TestMockNetworkSend 测试发送消息
func TestMockNetworkSend(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")
	_ = n.RegisterAgent("receiver-1", "receiver")

	// 发送消息
	err := n.Send(ctx, "sender-1", "receiver-1", "hello")
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	// 验证消息被记录
	if n.MessageCount() != 1 {
		t.Errorf("期望 1 条消息，实际为 %d", n.MessageCount())
	}

	messages := n.Messages()
	if messages[0].From != "sender-1" {
		t.Errorf("期望发送者为 sender-1")
	}
	if messages[0].To != "receiver-1" {
		t.Errorf("期望接收者为 receiver-1")
	}
	if messages[0].Content != "hello" {
		t.Errorf("期望内容为 'hello'，实际为 '%v'", messages[0].Content)
	}

	// 验证收件箱
	inbox, err := n.Inbox("receiver-1")
	if err != nil {
		t.Fatalf("获取收件箱失败: %v", err)
	}
	if len(inbox) != 1 {
		t.Errorf("期望收件箱有 1 条消息，实际为 %d", len(inbox))
	}
}

// TestMockNetworkSendNotFound 测试发送给不存在的 Agent
func TestMockNetworkSendNotFound(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")

	// 目标不存在
	err := n.Send(ctx, "sender-1", "nonexistent", "hello")
	if err == nil {
		t.Fatal("期望发送给不存在的目标返回错误")
	}

	// 发送者不存在
	err = n.Send(ctx, "nonexistent", "sender-1", "hello")
	if err == nil {
		t.Fatal("期望发送者不存在时返回错误")
	}
}

// TestMockNetworkBroadcast 测试广播消息
func TestMockNetworkBroadcast(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")
	_ = n.RegisterAgent("recv-1", "receiver1")
	_ = n.RegisterAgent("recv-2", "receiver2")

	// 广播
	err := n.Broadcast(ctx, "sender-1", "broadcast msg")
	if err != nil {
		t.Fatalf("广播失败: %v", err)
	}

	// 应该有 2 条消息（发给 recv-1 和 recv-2，不包括发送者自身）
	if n.MessageCount() != 2 {
		t.Errorf("期望 2 条广播消息，实际为 %d", n.MessageCount())
	}

	// 验证发送者收件箱为空
	inbox, _ := n.Inbox("sender-1")
	if len(inbox) != 0 {
		t.Errorf("发送者不应收到自己的广播，但收件箱有 %d 条", len(inbox))
	}

	// 验证接收者收件箱
	inbox1, _ := n.Inbox("recv-1")
	inbox2, _ := n.Inbox("recv-2")
	if len(inbox1) != 1 || len(inbox2) != 1 {
		t.Errorf("期望每个接收者收到 1 条消息")
	}
}

// TestMockNetworkBroadcastNotFound 测试发送者不存在时广播
func TestMockNetworkBroadcastNotFound(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	err := n.Broadcast(ctx, "nonexistent", "msg")
	if err == nil {
		t.Fatal("期望发送者不存在时广播返回错误")
	}
}

// TestMockNetworkGetAgentName 测试获取 Agent 名称
func TestMockNetworkGetAgentName(t *testing.T) {
	n := NewMockNetwork()

	_ = n.RegisterAgent("agent-1", "agent1")

	// 获取存在的 Agent
	name, ok := n.GetAgentName("agent-1")
	if !ok {
		t.Fatal("期望找到已注册的 Agent")
	}
	if name != "agent1" {
		t.Errorf("期望名称为 'agent1'，实际为 '%s'", name)
	}

	// 获取不存在的 Agent
	_, ok = n.GetAgentName("nonexistent")
	if ok {
		t.Fatal("期望获取不存在的 Agent 返回 false")
	}
}

// TestMockNetworkListAgentIDs 测试列出所有 Agent ID
func TestMockNetworkListAgentIDs(t *testing.T) {
	n := NewMockNetwork()

	_ = n.RegisterAgent("agent-1", "agent1")
	_ = n.RegisterAgent("agent-2", "agent2")

	ids := n.ListAgentIDs()
	if len(ids) != 2 {
		t.Errorf("期望 2 个 Agent，实际为 %d", len(ids))
	}
}

// TestMockNetworkMessagesFrom 测试按发送者查询消息
func TestMockNetworkMessagesFrom(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")
	_ = n.RegisterAgent("other-1", "other")
	_ = n.RegisterAgent("recv-1", "receiver")

	_ = n.Send(ctx, "sender-1", "recv-1", "msg1")
	_ = n.Send(ctx, "sender-1", "recv-1", "msg2")
	_ = n.Send(ctx, "other-1", "recv-1", "msg3")

	fromSender := n.MessagesFrom("sender-1")
	if len(fromSender) != 2 {
		t.Errorf("期望来自 sender 的消息有 2 条，实际为 %d", len(fromSender))
	}

	fromOther := n.MessagesFrom("other-1")
	if len(fromOther) != 1 {
		t.Errorf("期望来自 other 的消息有 1 条，实际为 %d", len(fromOther))
	}
}

// TestMockNetworkMessagesTo 测试按接收者查询消息
func TestMockNetworkMessagesTo(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")
	_ = n.RegisterAgent("recv-1", "receiver1")
	_ = n.RegisterAgent("recv-2", "receiver2")

	_ = n.Send(ctx, "sender-1", "recv-1", "msg1")
	_ = n.Send(ctx, "sender-1", "recv-2", "msg2")

	toRecv1 := n.MessagesTo("recv-1")
	if len(toRecv1) != 1 {
		t.Errorf("期望发给 receiver1 的消息有 1 条，实际为 %d", len(toRecv1))
	}
}

// TestMockNetworkClearInbox 测试清空收件箱
func TestMockNetworkClearInbox(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("sender-1", "sender")
	_ = n.RegisterAgent("recv-1", "receiver")

	_ = n.Send(ctx, "sender-1", "recv-1", "msg1")
	_ = n.Send(ctx, "sender-1", "recv-1", "msg2")

	err := n.ClearInbox("recv-1")
	if err != nil {
		t.Fatalf("清空收件箱失败: %v", err)
	}

	inbox, _ := n.Inbox("recv-1")
	if len(inbox) != 0 {
		t.Errorf("期望清空后收件箱为空，实际有 %d 条", len(inbox))
	}

	// 清空不存在的 Agent 的收件箱
	err = n.ClearInbox("nonexistent")
	if err == nil {
		t.Fatal("期望清空不存在 Agent 的收件箱返回错误")
	}
}

// TestMockNetworkInboxNotFound 测试获取不存在 Agent 的收件箱
func TestMockNetworkInboxNotFound(t *testing.T) {
	n := NewMockNetwork()

	_, err := n.Inbox("nonexistent")
	if err == nil {
		t.Fatal("期望获取不存在 Agent 的收件箱返回错误")
	}
}

// TestMockNetworkReset 测试重置网络
func TestMockNetworkReset(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	_ = n.RegisterAgent("agent-1", "agent1")
	_ = n.RegisterAgent("agent-2", "agent2")

	_ = n.Send(ctx, "agent-1", "agent-2", "msg")

	n.Reset()

	// 消息被清空
	if n.MessageCount() != 0 {
		t.Errorf("期望重置后消息为空，实际有 %d 条", n.MessageCount())
	}

	// Agent 仍然存在
	if n.AgentCount() != 2 {
		t.Errorf("期望重置后 Agent 数量不变，实际为 %d", n.AgentCount())
	}

	// 收件箱被清空
	inbox, _ := n.Inbox("agent-2")
	if len(inbox) != 0 {
		t.Errorf("期望重置后收件箱为空")
	}
}

// TestMockNetworkConcurrency 测试网络并发安全
func TestMockNetworkConcurrency(t *testing.T) {
	n := NewMockNetwork()
	ctx := context.Background()

	// 注册多个 Agent
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("agent-%d", i)
		_ = n.RegisterAgent(id, fmt.Sprintf("agent%d", i))
	}

	// 并发发送消息
	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			from := fmt.Sprintf("agent-%d", idx%10)
			to := fmt.Sprintf("agent-%d", (idx+1)%10)
			_ = n.Send(ctx, from, to, fmt.Sprintf("msg-%d", idx))
		}(i)
	}

	wg.Wait()

	if n.MessageCount() != goroutines {
		t.Errorf("期望 %d 条消息，实际为 %d", goroutines, n.MessageCount())
	}
}
