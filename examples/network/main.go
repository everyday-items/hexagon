// Package main 演示 Hexagon Agent 网络
//
// Agent 网络支持多种拓扑和通信模式：
//   - 拓扑类型: Mesh/Hub/Ring/Tree/Custom
//   - 消息类型: Request/Response/Broadcast/Event
//   - 消息路由: 点对点/广播/主题订阅
//
// 本示例展示网络数据结构和消息传递。
//
// 运行方式:
//
//	go run ./examples/network/
package main

import (
	"fmt"
	"time"

	"github.com/everyday-items/hexagon/agent"
)

func main() {
	fmt.Println("=== 示例 1: 网络拓扑类型 ===")
	runTopologies()

	fmt.Println("\n=== 示例 2: 消息类型和创建 ===")
	runMessages()

	fmt.Println("\n=== 示例 3: 节点状态 ===")
	runNodeStatus()
}

// runTopologies 演示网络拓扑类型
func runTopologies() {
	topologies := []struct {
		topo agent.NetworkTopology
		desc string
	}{
		{agent.TopologyMesh, "全连接，每个节点可以与所有其他节点通信"},
		{agent.TopologyHub, "中心辐射，所有通信经过中心节点"},
		{agent.TopologyRing, "环形，消息沿环传递"},
		{agent.TopologyTree, "树形，层级通信"},
		{agent.TopologyCustom, "自定义拓扑"},
	}

	for _, t := range topologies {
		fmt.Printf("  %s: %s\n", t.topo.String(), t.desc)
	}
}

// runMessages 演示消息创建和类型
func runMessages() {
	// 创建不同类型的消息
	messages := []*agent.NetworkMessage{
		agent.NewMessage("agent-1", "agent-2", agent.MessageTypeRequest, "请帮我分析数据"),
		agent.NewMessage("agent-2", "agent-1", agent.MessageTypeResponse, "分析结果：增长 15%"),
		agent.NewMessage("agent-1", "", agent.MessageTypeBroadcast, "通知：分析完成"),
		agent.NewMessage("system", "", agent.MessageTypeEvent, map[string]any{
			"event": "agent_joined",
			"agent": "agent-3",
		}),
	}

	for _, msg := range messages {
		to := msg.To
		if to == "" {
			to = "(广播)"
		}
		fmt.Printf("  [%s] %s → %s: %v\n",
			msg.Type.String(), msg.From, to, msg.Content)
	}

	// 消息元数据
	msg := agent.NewMessage("agent-1", "agent-2", agent.MessageTypeRequest, "重要任务")
	msg.Topic = "task-assignment"
	msg.Priority = 9
	msg.TTL = 5 * time.Minute
	msg.ReplyTo = "msg-prev-001"
	fmt.Printf("\n  高优先级消息:\n")
	fmt.Printf("    主题: %s\n", msg.Topic)
	fmt.Printf("    优先级: %d\n", msg.Priority)
	fmt.Printf("    TTL: %v\n", msg.TTL)
	fmt.Printf("    回复: %s\n", msg.ReplyTo)
}

// runNodeStatus 演示节点状态
func runNodeStatus() {
	statuses := []struct {
		status agent.NodeStatus
		desc   string
	}{
		{agent.NodeStatusOnline, "节点在线，可以接收消息"},
		{agent.NodeStatusOffline, "节点离线，消息将排队"},
		{agent.NodeStatusBusy, "节点忙碌，可能延迟处理"},
		{agent.NodeStatusError, "节点异常，需要排查"},
	}

	for _, s := range statuses {
		fmt.Printf("  %s: %s\n", s.status.String(), s.desc)
	}
}
