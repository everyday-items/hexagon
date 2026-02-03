package metrics

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewHexagonMetrics 测试创建收集器
func TestNewHexagonMetrics(t *testing.T) {
	t.Run("默认 metrics", func(t *testing.T) {
		collector := NewHexagonMetrics(nil)
		if collector == nil {
			t.Fatal("collector 不应为 nil")
		}
		if collector.metrics == nil {
			t.Error("metrics 不应为 nil")
		}
	})

	t.Run("自定义 metrics", func(t *testing.T) {
		metrics := NewMemoryMetrics()
		collector := NewHexagonMetrics(metrics)
		if collector.metrics != metrics {
			t.Error("应该使用自定义 metrics")
		}
	})
}

// TestRecordAgentRun 测试记录 Agent 执行
func TestRecordAgentRun(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	t.Run("记录成功执行", func(t *testing.T) {
		collector.RecordAgentRun(ctx, "test-agent", 100*time.Millisecond, nil)

		stats := collector.GetAgentStats("test-agent")
		if stats == nil {
			t.Fatal("stats 不应为 nil")
		}

		if stats.TotalRuns != 1 {
			t.Errorf("TotalRuns = %d, want 1", stats.TotalRuns)
		}
		if stats.TotalErrors != 0 {
			t.Errorf("TotalErrors = %d, want 0", stats.TotalErrors)
		}
	})

	t.Run("记录失败执行", func(t *testing.T) {
		collector.RecordAgentRun(ctx, "test-agent", 50*time.Millisecond, errors.New("test error"))

		stats := collector.GetAgentStats("test-agent")
		if stats.TotalRuns != 2 {
			t.Errorf("TotalRuns = %d, want 2", stats.TotalRuns)
		}
		if stats.TotalErrors != 1 {
			t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
		}
	})

	t.Run("平均时长计算", func(t *testing.T) {
		stats := collector.GetAgentStats("test-agent")
		expectedAvg := (100*time.Millisecond + 50*time.Millisecond) / 2
		if stats.AverageDuration != expectedAvg {
			t.Errorf("AverageDuration = %v, want %v", stats.AverageDuration, expectedAvg)
		}
	})
}

// TestRecordLLMCall 测试记录 LLM 调用
func TestRecordLLMCall(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	t.Run("记录成功调用", func(t *testing.T) {
		collector.RecordLLMCall(ctx, "openai", "gpt-4", 100, 50, 500*time.Millisecond, nil)

		stats := collector.GetLLMStats("openai", "gpt-4")
		if stats == nil {
			t.Fatal("stats 不应为 nil")
		}

		if stats.TotalCalls != 1 {
			t.Errorf("TotalCalls = %d, want 1", stats.TotalCalls)
		}
		if stats.TotalPromptTokens != 100 {
			t.Errorf("TotalPromptTokens = %d, want 100", stats.TotalPromptTokens)
		}
		if stats.TotalCompletionTokens != 50 {
			t.Errorf("TotalCompletionTokens = %d, want 50", stats.TotalCompletionTokens)
		}
	})

	t.Run("记录失败调用", func(t *testing.T) {
		collector.RecordLLMCall(ctx, "openai", "gpt-4", 50, 0, 100*time.Millisecond, errors.New("rate limit"))

		stats := collector.GetLLMStats("openai", "gpt-4")
		if stats.TotalCalls != 2 {
			t.Errorf("TotalCalls = %d, want 2", stats.TotalCalls)
		}
		if stats.TotalErrors != 1 {
			t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
		}
		if stats.TotalPromptTokens != 150 {
			t.Errorf("TotalPromptTokens = %d, want 150", stats.TotalPromptTokens)
		}
	})

	t.Run("多个模型统计", func(t *testing.T) {
		collector.RecordLLMCall(ctx, "anthropic", "claude-3", 200, 100, 600*time.Millisecond, nil)

		allStats := collector.GetAllLLMStats()
		if len(allStats) != 2 {
			t.Errorf("模型数量 = %d, want 2", len(allStats))
		}
	})
}

// TestRecordToolCall 测试记录工具调用
func TestRecordToolCall(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	t.Run("记录成功调用", func(t *testing.T) {
		collector.RecordToolCall(ctx, "web_search", 200*time.Millisecond, nil)

		stats := collector.GetToolStats("web_search")
		if stats == nil {
			t.Fatal("stats 不应为 nil")
		}

		if stats.TotalCalls != 1 {
			t.Errorf("TotalCalls = %d, want 1", stats.TotalCalls)
		}
	})

	t.Run("记录失败调用", func(t *testing.T) {
		collector.RecordToolCall(ctx, "web_search", 100*time.Millisecond, errors.New("timeout"))

		stats := collector.GetToolStats("web_search")
		if stats.TotalErrors != 1 {
			t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
		}
	})

	t.Run("多个工具统计", func(t *testing.T) {
		collector.RecordToolCall(ctx, "calculator", 10*time.Millisecond, nil)

		allStats := collector.GetAllToolStats()
		if len(allStats) != 2 {
			t.Errorf("工具数量 = %d, want 2", len(allStats))
		}
	})
}

// TestRecordRetrieval 测试记录检索
func TestRecordRetrieval(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	t.Run("记录检索", func(t *testing.T) {
		collector.RecordRetrieval(ctx, "vector_search", 5, 50*time.Millisecond)
		collector.RecordRetrieval(ctx, "vector_search", 10, 100*time.Millisecond)

		stats := collector.GetRetrievalStats()
		if stats.TotalRetrievals != 2 {
			t.Errorf("TotalRetrievals = %d, want 2", stats.TotalRetrievals)
		}
		if stats.TotalDocuments != 15 {
			t.Errorf("TotalDocuments = %d, want 15", stats.TotalDocuments)
		}
		if stats.AverageDocCount != 7.5 {
			t.Errorf("AverageDocCount = %f, want 7.5", stats.AverageDocCount)
		}
	})
}

// TestAgentActive 测试活跃 Agent 计数
func TestAgentActive(t *testing.T) {
	collector := NewHexagonMetrics(nil)

	t.Run("设置活跃数量", func(t *testing.T) {
		collector.SetAgentActive(5)
		// 验证通过底层 metrics
	})

	t.Run("增减活跃数量", func(t *testing.T) {
		collector.IncrementAgentActive()
		collector.IncrementAgentActive()
		collector.DecrementAgentActive()
		// 验证通过底层 metrics
	})
}

// TestGetSummary 测试获取汇总
func TestGetSummary(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	// 记录一些数据
	collector.RecordAgentRun(ctx, "agent1", 100*time.Millisecond, nil)
	collector.RecordAgentRun(ctx, "agent2", 200*time.Millisecond, errors.New("error"))
	collector.RecordLLMCall(ctx, "openai", "gpt-4", 100, 50, 500*time.Millisecond, nil)
	collector.RecordToolCall(ctx, "tool1", 50*time.Millisecond, nil)
	collector.RecordRetrieval(ctx, "retriever1", 10, 100*time.Millisecond)

	t.Run("获取汇总", func(t *testing.T) {
		summary := collector.GetSummary()
		if summary == nil {
			t.Fatal("summary 不应为 nil")
		}

		if summary.TotalAgentRuns != 2 {
			t.Errorf("TotalAgentRuns = %d, want 2", summary.TotalAgentRuns)
		}
		if summary.TotalLLMCalls != 1 {
			t.Errorf("TotalLLMCalls = %d, want 1", summary.TotalLLMCalls)
		}
		if summary.TotalToolCalls != 1 {
			t.Errorf("TotalToolCalls = %d, want 1", summary.TotalToolCalls)
		}
		if summary.TotalRetrievals != 1 {
			t.Errorf("TotalRetrievals = %d, want 1", summary.TotalRetrievals)
		}
		if summary.TotalErrors != 1 {
			t.Errorf("TotalErrors = %d, want 1", summary.TotalErrors)
		}
		if summary.TotalTokens != 150 {
			t.Errorf("TotalTokens = %d, want 150", summary.TotalTokens)
		}
	})
}

// TestReset 测试重置
func TestReset(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	// 记录一些数据
	collector.RecordAgentRun(ctx, "agent1", 100*time.Millisecond, nil)
	collector.RecordLLMCall(ctx, "openai", "gpt-4", 100, 50, 500*time.Millisecond, nil)

	t.Run("重置后清空", func(t *testing.T) {
		collector.Reset()

		summary := collector.GetSummary()
		if summary.TotalAgentRuns != 0 {
			t.Errorf("TotalAgentRuns = %d, want 0", summary.TotalAgentRuns)
		}
		if summary.TotalLLMCalls != 0 {
			t.Errorf("TotalLLMCalls = %d, want 0", summary.TotalLLMCalls)
		}
	})
}

// TestHooks 测试 Hook 函数
func TestHooks(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	t.Run("AgentRunHook", func(t *testing.T) {
		hook := collector.AgentRunHook()
		hook(ctx, "hook-agent", 100*time.Millisecond, nil)

		stats := collector.GetAgentStats("hook-agent")
		if stats == nil || stats.TotalRuns != 1 {
			t.Error("Hook 未正确记录 Agent 执行")
		}
	})

	t.Run("LLMCallHook", func(t *testing.T) {
		hook := collector.LLMCallHook()
		hook(ctx, "openai", "gpt-4", 100, 50, 500*time.Millisecond, nil)

		stats := collector.GetLLMStats("openai", "gpt-4")
		if stats == nil || stats.TotalCalls != 1 {
			t.Error("Hook 未正确记录 LLM 调用")
		}
	})

	t.Run("ToolCallHook", func(t *testing.T) {
		hook := collector.ToolCallHook()
		hook(ctx, "hook-tool", 50*time.Millisecond, nil)

		stats := collector.GetToolStats("hook-tool")
		if stats == nil || stats.TotalCalls != 1 {
			t.Error("Hook 未正确记录 Tool 调用")
		}
	})

	t.Run("RetrievalHook", func(t *testing.T) {
		hook := collector.RetrievalHook()
		hook(ctx, "hook-retriever", 5, 100*time.Millisecond)

		stats := collector.GetRetrievalStats()
		if stats.TotalRetrievals != 1 {
			t.Error("Hook 未正确记录检索")
		}
	})
}

// TestGlobalHexagonMetrics 测试全局实例
func TestGlobalHexagonMetrics(t *testing.T) {
	t.Run("获取全局实例", func(t *testing.T) {
		m1 := GetHexagonMetrics()
		m2 := GetHexagonMetrics()

		if m1 != m2 {
			t.Error("应该返回同一个实例")
		}
	})

	t.Run("设置全局实例", func(t *testing.T) {
		custom := NewHexagonMetrics(nil)
		SetHexagonMetrics(custom)

		if GetHexagonMetrics() != custom {
			t.Error("应该返回设置的实例")
		}
	})
}

// TestGetNonExistentStats 测试获取不存在的统计
func TestGetNonExistentStats(t *testing.T) {
	collector := NewHexagonMetrics(nil)

	t.Run("不存在的 Agent", func(t *testing.T) {
		stats := collector.GetAgentStats("nonexistent")
		if stats != nil {
			t.Error("不存在的 Agent 应返回 nil")
		}
	})

	t.Run("不存在的 LLM", func(t *testing.T) {
		stats := collector.GetLLMStats("nonexistent", "model")
		if stats != nil {
			t.Error("不存在的 LLM 应返回 nil")
		}
	})

	t.Run("不存在的 Tool", func(t *testing.T) {
		stats := collector.GetToolStats("nonexistent")
		if stats != nil {
			t.Error("不存在的 Tool 应返回 nil")
		}
	})
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	collector := NewHexagonMetrics(nil)
	ctx := context.Background()

	done := make(chan bool)
	iterations := 100

	// 并发记录 Agent
	go func() {
		for i := 0; i < iterations; i++ {
			collector.RecordAgentRun(ctx, "concurrent-agent", time.Millisecond, nil)
		}
		done <- true
	}()

	// 并发记录 LLM
	go func() {
		for i := 0; i < iterations; i++ {
			collector.RecordLLMCall(ctx, "openai", "gpt-4", 10, 5, time.Millisecond, nil)
		}
		done <- true
	}()

	// 并发读取统计
	go func() {
		for i := 0; i < iterations; i++ {
			collector.GetSummary()
		}
		done <- true
	}()

	// 等待完成
	<-done
	<-done
	<-done

	// 验证结果
	summary := collector.GetSummary()
	if summary.TotalAgentRuns != int64(iterations) {
		t.Errorf("TotalAgentRuns = %d, want %d", summary.TotalAgentRuns, iterations)
	}
	if summary.TotalLLMCalls != int64(iterations) {
		t.Errorf("TotalLLMCalls = %d, want %d", summary.TotalLLMCalls, iterations)
	}
}
