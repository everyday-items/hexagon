# 快速入门指南

本指南将帮助您快速上手 Hexagon AI Agent 框架。

## 安装

```bash
go get github.com/everyday-items/hexagon
```

## 最简示例

只需 3 行代码即可创建一个 AI Agent：

```go
package main

import (
    "context"
    "fmt"

    "github.com/everyday-items/hexagon/agent"
    "github.com/everyday-items/ai-core/llm/openai"
)

func main() {
    // 创建 LLM Provider
    llm, _ := openai.New(openai.WithAPIKey("your-api-key"))

    // 创建 Agent
    myAgent := agent.NewBaseAgent(
        agent.WithLLM(llm),
        agent.WithSystemPrompt("你是一个有帮助的助手"),
    )

    // 执行
    output, _ := myAgent.Run(context.Background(), agent.Input{
        Query: "你好，请介绍一下你自己",
    })

    fmt.Println(output.Content)
}
```

## 核心概念

### Agent

Agent 是 Hexagon 的核心概念，代表一个可执行的 AI 实体。每个 Agent 可以：

- 接收用户输入
- 调用 LLM 进行推理
- 使用工具执行任务
- 返回处理结果

### Component

所有组件（Agent、Tool、Chain、Graph）都实现统一的 `Component[I, O]` 接口：

```go
type Component[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
    Stream(ctx context.Context, input I) (Stream[O], error)
    Batch(ctx context.Context, inputs []I) ([]O, error)
}
```

### 中间件

使用中间件扩展 Agent 功能：

```go
chain := agent.NewMiddlewareChain(
    agent.RecoverMiddleware(),    // panic 恢复
    agent.LoggingMiddleware(nil), // 日志记录
    agent.TimeoutMiddleware(30*time.Second), // 超时控制
)

handler := chain.WrapAgent(myAgent)
output, err := handler(ctx, input)
```

## 添加工具

让 Agent 能够执行具体任务：

```go
import "github.com/everyday-items/ai-core/tool"

// 定义搜索工具
searchTool := tool.NewFunc("web_search",
    "搜索网页信息",
    func(ctx context.Context, input struct {
        Query string `json:"query" description:"搜索关键词"`
    }) (string, error) {
        // 实现搜索逻辑
        return "搜索结果...", nil
    },
)

// 创建带工具的 Agent
myAgent := agent.NewReActAgent(
    agent.WithLLM(llm),
    agent.WithTools(searchTool),
)
```

## 使用记忆

为 Agent 添加对话记忆：

```go
import "github.com/everyday-items/ai-core/memory"

// 创建记忆
mem := memory.NewConversationMemory(10) // 保留最近 10 轮对话

// 创建带记忆的 Agent
myAgent := agent.NewBaseAgent(
    agent.WithLLM(llm),
    agent.WithMemory(mem),
)
```

## RAG 检索增强

集成知识库增强 Agent 能力：

```go
import (
    "github.com/everyday-items/hexagon/rag"
    "github.com/everyday-items/hexagon/store/vector/qdrant"
)

// 创建向量存储
store, _ := qdrant.New(ctx, qdrant.WithCollection("docs"))

// 创建检索器
retriever := rag.NewVectorRetriever(store, embedder)

// 创建 RAG Agent
ragAgent := agent.NewRAGAgent(
    agent.WithLLM(llm),
    agent.WithRetriever(retriever),
)
```

## 可观测性

添加指标和追踪：

```go
import "github.com/everyday-items/hexagon/observe/metrics"

// 获取指标收集器
collector := metrics.GetHexagonMetrics()

// 记录 Agent 执行
collector.RecordAgentRun(ctx, "my-agent", duration, err)

// 获取统计汇总
summary := collector.GetSummary()
fmt.Printf("总执行次数: %d\n", summary.TotalAgentRuns)
```

## 下一步

- [Agent 开发指南](agent-guide.md) - 深入了解 Agent 开发
- [RAG 系统使用](rag-guide.md) - 构建知识增强应用
- [插件开发指南](plugin-guide.md) - 扩展框架功能
