// Package main 演示 Hexagon Dev UI 的使用
//
// 运行示例：
//
//	go run examples/devui/main.go
//
// 然后在浏览器中访问 http://localhost:8080 查看 Dev UI
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hexagon-codes/hexagon/hooks"
	"github.com/hexagon-codes/hexagon/observe/devui"
)

func main() {
	fmt.Println("🔮 Hexagon Dev UI 示例")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 创建 DevUI
	ui := devui.New(
		devui.WithAddr(":8080"),
		devui.WithMaxEvents(1000),
		devui.WithSSE(true),
		devui.WithMetrics(true),
	)

	// 获取 Hook Manager 用于模拟事件
	hookMgr := ui.HookManager()

	// 启动 DevUI 服务器
	go func() {
		if err := ui.Start(); err != nil {
			fmt.Printf("❌ DevUI 启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	fmt.Println()
	fmt.Printf("✅ Dev UI 已启动: %s\n", ui.URL())
	fmt.Println()
	fmt.Println("📋 功能说明:")
	fmt.Println("   - 实时事件流 (SSE 推送)")
	fmt.Println("   - 事件详情查看")
	fmt.Println("   - 指标仪表板")
	fmt.Println("   - 事件类型过滤")
	fmt.Println()
	fmt.Println("🎯 正在模拟 Agent 执行事件...")
	fmt.Println("   按 Ctrl+C 退出")
	fmt.Println()

	// 启动模拟器
	ctx, cancel := context.WithCancel(context.Background())
	go runSimulator(ctx, hookMgr)

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n⏹️  正在停止...")
	cancel()

	// 停止 DevUI
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := ui.Stop(shutdownCtx); err != nil {
		fmt.Printf("❌ 停止失败: %v\n", err)
	}

	fmt.Println("👋 已停止")
}

// runSimulator 模拟 Agent 执行过程
func runSimulator(ctx context.Context, hookMgr *hooks.Manager) {
	runID := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
			runID++
			simulateAgentRun(ctx, hookMgr, fmt.Sprintf("run-%d", runID))
			time.Sleep(time.Duration(2+rand.Intn(3)) * time.Second)
		}
	}
}

// simulateAgentRun 模拟一次 Agent 运行
func simulateAgentRun(ctx context.Context, hookMgr *hooks.Manager, runID string) {
	agentID := fmt.Sprintf("agent-%d", rand.Intn(3)+1)
	queries := []string{
		"今天北京天气怎么样？",
		"帮我计算 123 * 456",
		"查找关于 AI 的最新新闻",
		"翻译这段文字到英文",
		"分析这篇文章的主题",
	}
	query := queries[rand.Intn(len(queries))]

	// Agent 开始
	hookMgr.TriggerRunStart(ctx, &hooks.RunStartEvent{
		RunID:   runID,
		AgentID: agentID,
		Input:   query,
		Metadata: map[string]any{
			"source": "simulator",
		},
	})

	// 模拟思考时间
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

	// 模拟 LLM 调用
	models := []string{"gpt-4o", "gpt-4o-mini", "claude-3-opus", "deepseek-chat"}
	model := models[rand.Intn(len(models))]

	hookMgr.TriggerLLMStart(ctx, &hooks.LLMStartEvent{
		RunID:       runID,
		Provider:    "openai",
		Model:       model,
		Messages:    []any{map[string]string{"role": "user", "content": query}},
		Temperature: 0.7,
	})

	// 模拟流式输出
	responses := []string{
		"好的，让我来帮您处理这个请求。",
		"根据我的分析，",
		"这是一个很有趣的问题。",
		"我需要调用一些工具来完成这个任务。",
	}
	response := responses[rand.Intn(len(responses))]

	for i, char := range response {
		hookMgr.TriggerLLMStream(ctx, &hooks.LLMStreamEvent{
			RunID:      runID,
			Model:      model,
			Content:    string(char),
			ChunkIndex: i,
		})
		time.Sleep(20 * time.Millisecond)
	}

	// LLM 响应完成
	promptTokens := 50 + rand.Intn(100)
	completionTokens := 100 + rand.Intn(200)
	hookMgr.TriggerLLMEnd(ctx, &hooks.LLMEndEvent{
		RunID:            runID,
		Model:            model,
		Response:         response,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Duration:         int64(200 + rand.Intn(500)),
	})

	// 随机决定是否调用工具
	if rand.Float32() < 0.7 {
		simulateToolCall(ctx, hookMgr, runID)
	}

	// 随机决定是否进行检索
	if rand.Float32() < 0.5 {
		simulateRetrieval(ctx, hookMgr, runID, query)
	}

	// 随机模拟错误
	if rand.Float32() < 0.1 {
		hookMgr.TriggerError(ctx, &hooks.ErrorEvent{
			RunID:   runID,
			AgentID: agentID,
			Error:   fmt.Errorf("模拟错误: 处理超时"),
		})
	}

	// Agent 结束
	hookMgr.TriggerRunEnd(ctx, &hooks.RunEndEvent{
		RunID:    runID,
		AgentID:  agentID,
		Output:   "任务已完成：" + response,
		Duration: int64(1000 + rand.Intn(2000)),
	})

	fmt.Printf("📤 完成模拟运行: %s (Agent: %s)\n", runID, agentID)
}

// simulateToolCall 模拟工具调用
func simulateToolCall(ctx context.Context, hookMgr *hooks.Manager, runID string) {
	tools := []struct {
		name   string
		input  map[string]any
		output any
	}{
		{
			name:   "calculator",
			input:  map[string]any{"operation": "multiply", "a": 123, "b": 456},
			output: 56088,
		},
		{
			name:   "weather",
			input:  map[string]any{"city": "北京", "date": "today"},
			output: map[string]any{"temperature": 25, "condition": "晴朗"},
		},
		{
			name:   "search",
			input:  map[string]any{"query": "AI 新闻", "limit": 5},
			output: []string{"新闻1", "新闻2", "新闻3"},
		},
		{
			name:   "translator",
			input:  map[string]any{"text": "你好", "target": "en"},
			output: "Hello",
		},
	}

	tool := tools[rand.Intn(len(tools))]
	toolID := fmt.Sprintf("tool-%d", rand.Intn(1000))

	// 工具开始
	hookMgr.TriggerToolStart(ctx, &hooks.ToolStartEvent{
		RunID:    runID,
		ToolName: tool.name,
		ToolID:   toolID,
		Input:    tool.input,
	})

	// 模拟执行时间
	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	// 工具结束
	hookMgr.TriggerToolEnd(ctx, &hooks.ToolEndEvent{
		RunID:    runID,
		ToolName: tool.name,
		ToolID:   toolID,
		Output:   tool.output,
		Duration: int64(50 + rand.Intn(150)),
	})
}

// simulateRetrieval 模拟检索
func simulateRetrieval(ctx context.Context, hookMgr *hooks.Manager, runID, query string) {
	topK := 3 + rand.Intn(5)

	// 检索开始
	hookMgr.TriggerRetrieverStart(ctx, &hooks.RetrieverStartEvent{
		RunID: runID,
		Query: query,
		TopK:  topK,
	})

	// 模拟检索时间
	time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)

	// 生成模拟文档
	docs := make([]any, topK)
	for i := 0; i < topK; i++ {
		docs[i] = map[string]any{
			"id":      fmt.Sprintf("doc-%d", i+1),
			"content": fmt.Sprintf("这是与查询 '%s' 相关的文档 %d", query, i+1),
			"score":   0.9 - float64(i)*0.1,
		}
	}

	// 检索结束
	hookMgr.TriggerRetrieverEnd(ctx, &hooks.RetrieverEndEvent{
		RunID:     runID,
		Query:     query,
		Documents: docs,
		Duration:  int64(30 + rand.Intn(70)),
	})
}
