package bench

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/agent"
	"github.com/everyday-items/hexagon/core"
)

// BenchmarkAgentCreation 测试 Agent 创建性能
func BenchmarkAgentCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = agent.NewReAct(
			agent.WithName("test-agent"),
			agent.WithSystemPrompt("You are a test agent"),
			agent.WithMaxIterations(10),
		)
	}
}

// BenchmarkStateManager 测试状态管理器性能
func BenchmarkStateManager(b *testing.B) {
	sm := agent.NewStateManager("session-1", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sm.Turn().Set("key", i)
		_, _ = sm.Turn().Get("key")
		sm.Session().Set("user", "test")
		_, _ = sm.Session().Get("user")
	}
}

// BenchmarkStateManagerConcurrent 测试状态管理器并发性能
func BenchmarkStateManagerConcurrent(b *testing.B) {
	sm := agent.NewStateManager("session-1", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sm.Turn().Set("key", i)
			_, _ = sm.Turn().Get("key")
			i++
		}
	})
}

// BenchmarkGlobalState 测试全局状态性能
func BenchmarkGlobalState(b *testing.B) {
	gs := agent.NewGlobalState()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		gs.Set("key", i)
		_, _ = gs.Get("key")
	}
}

// BenchmarkGlobalStateConcurrent 测试全局状态并发性能
func BenchmarkGlobalStateConcurrent(b *testing.B) {
	gs := agent.NewGlobalState()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			gs.Set("key", i)
			_, _ = gs.Get("key")
			i++
		}
	})
}

// BenchmarkTeamCreation 测试 Team 创建性能
func BenchmarkTeamCreation(b *testing.B) {
	agents := make([]agent.Agent, 3)
	for i := range agents {
		agents[i] = agent.NewReAct(agent.WithName("agent"))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = agent.NewTeam("test-team",
			agent.WithAgents(agents...),
			agent.WithMode(agent.TeamModeSequential),
		)
	}
}

// BenchmarkSchemaGeneration 测试 Schema 生成性能
func BenchmarkSchemaGeneration(b *testing.B) {
	type TestInput struct {
		Name    string  `json:"name" desc:"Name field" required:"true"`
		Age     int     `json:"age" desc:"Age field"`
		Score   float64 `json:"score"`
		Enabled bool    `json:"enabled"`
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = core.SchemaOf[TestInput]()
	}
}

// BenchmarkSliceStream 测试 SliceStream 性能
func BenchmarkSliceStream(b *testing.B) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	b.ReportAllocs()
	b.ResetTimer()

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		stream := core.NewSliceStream(items)
		_, _ = stream.Collect(ctx)
	}
}

// BenchmarkSliceStreamForEach 测试 SliceStream ForEach 性能
func BenchmarkSliceStreamForEach(b *testing.B) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	b.ReportAllocs()
	b.ResetTimer()

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		stream := core.NewSliceStream(items)
		_ = stream.ForEach(ctx, func(v int) error {
			return nil
		})
	}
}
