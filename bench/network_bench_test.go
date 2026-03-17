package bench

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hexagon-codes/hexagon/agent"
)

// BenchmarkNetworkCreation 测试 Agent 网络创建性能
func BenchmarkNetworkCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = agent.NewAgentNetwork("bench-network",
			agent.WithNetworkTopology(agent.TopologyMesh),
			agent.WithNetworkInboxSize(100),
		)
	}
}

// BenchmarkNetworkRegister 测试 Agent 注册性能
func BenchmarkNetworkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		network := agent.NewAgentNetwork("bench-network")
		a := agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
		_ = network.Register(a)
	}
}

// BenchmarkNetworkRegisterMultiple 测试批量注册 Agent 的性能
func BenchmarkNetworkRegisterMultiple(b *testing.B) {
	agents := make([]agent.Agent, 10)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		network := agent.NewAgentNetwork("bench-network")
		for _, a := range agents {
			_ = network.Register(a)
		}
	}
}

// BenchmarkNetworkSend 测试消息发送性能（直接投递模式）
func BenchmarkNetworkSend(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network",
		agent.WithNetworkInboxSize(b.N+100),
	)

	sender := agent.NewReAct(agent.WithName("sender"))
	receiver := agent.NewReAct(agent.WithName("receiver"))
	_ = network.Register(sender)
	_ = network.Register(receiver)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = network.SendTo(ctx, sender.ID(), receiver.ID(), "hello")
	}
}

// BenchmarkNetworkSendConcurrent 测试并发消息发送性能
func BenchmarkNetworkSendConcurrent(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network",
		agent.WithNetworkInboxSize(b.N+10000),
	)

	sender := agent.NewReAct(agent.WithName("sender"))
	receiver := agent.NewReAct(agent.WithName("receiver"))
	_ = network.Register(sender)
	_ = network.Register(receiver)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = network.SendTo(ctx, sender.ID(), receiver.ID(), "hello")
		}
	})
}

// BenchmarkNetworkBroadcast 测试广播性能
func BenchmarkNetworkBroadcast(b *testing.B) {
	for _, agentCount := range []int{5, 10, 20} {
		b.Run(fmt.Sprintf("agents_%d", agentCount), func(b *testing.B) {
			network := agent.NewAgentNetwork("bench-network",
				agent.WithNetworkInboxSize(b.N*agentCount+1000),
			)

			agents := make([]agent.Agent, agentCount)
			for i := range agents {
				agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
				_ = network.Register(agents[i])
			}

			ctx := context.Background()
			senderID := agents[0].ID()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = network.Broadcast(ctx, senderID, "broadcast message")
			}
		})
	}
}

// BenchmarkNetworkMessageCreation 测试消息创建性能
func BenchmarkNetworkMessageCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = agent.NewMessage("from", "to", agent.MessageTypeRequest, "content")
	}
}

// BenchmarkNetworkGetNode 测试节点查找性能
func BenchmarkNetworkGetNode(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network")

	agents := make([]agent.Agent, 20)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
		_ = network.Register(agents[i])
	}

	targetID := agents[10].ID()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = network.GetNode(targetID)
	}
}

// BenchmarkNetworkGetNodeConcurrent 测试并发节点查找性能
func BenchmarkNetworkGetNodeConcurrent(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network")

	agents := make([]agent.Agent, 20)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
		_ = network.Register(agents[i])
	}

	targetID := agents[10].ID()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = network.GetNode(targetID)
		}
	})
}

// BenchmarkNetworkListAgents 测试列出所有 Agent 的性能
func BenchmarkNetworkListAgents(b *testing.B) {
	for _, agentCount := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("agents_%d", agentCount), func(b *testing.B) {
			network := agent.NewAgentNetwork("bench-network")

			for i := 0; i < agentCount; i++ {
				a := agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
				_ = network.Register(a)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = network.ListAgents()
			}
		})
	}
}

// BenchmarkNetworkStats 测试网络统计信息获取性能
func BenchmarkNetworkStats(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network")

	for i := 0; i < 20; i++ {
		a := agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
		_ = network.Register(a)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = network.Stats()
	}
}

// BenchmarkNetworkTopologyMesh 测试 Mesh 拓扑构建性能
func BenchmarkNetworkTopologyMesh(b *testing.B) {
	for _, agentCount := range []int{5, 10, 20} {
		b.Run(fmt.Sprintf("agents_%d", agentCount), func(b *testing.B) {
			agents := make([]agent.Agent, agentCount)
			for i := range agents {
				agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				network := agent.NewAgentNetwork("bench-network",
					agent.WithNetworkTopology(agent.TopologyMesh),
				)
				for _, a := range agents {
					_ = network.Register(a)
				}
			}
		})
	}
}

// BenchmarkNetworkTopologyRing 测试 Ring 拓扑构建性能
func BenchmarkNetworkTopologyRing(b *testing.B) {
	agents := make([]agent.Agent, 10)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		network := agent.NewAgentNetwork("bench-network",
			agent.WithNetworkTopology(agent.TopologyRing),
		)
		for _, a := range agents {
			_ = network.Register(a)
		}
	}
}

// BenchmarkNetworkConcurrentReadWrite 测试并发读写操作性能
func BenchmarkNetworkConcurrentReadWrite(b *testing.B) {
	network := agent.NewAgentNetwork("bench-network",
		agent.WithNetworkInboxSize(100000),
	)

	agents := make([]agent.Agent, 10)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName(fmt.Sprintf("agent-%d", i)))
		_ = network.Register(agents[i])
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var wg sync.WaitGroup
	// 写操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			_ = network.SendTo(ctx, agents[0].ID(), agents[1].ID(), "msg")
		}
	}()

	// 读操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			_ = network.ListAgents()
		}
	}()

	// 统计操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			_ = network.Stats()
		}
	}()

	wg.Wait()
}
