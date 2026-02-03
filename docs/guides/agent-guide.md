# Agent 开发指南

本指南详细介绍如何使用 Hexagon 开发各类 AI Agent。

## Agent 类型

### BaseAgent

基础 Agent，提供最简单的 LLM 调用能力：

```go
agent := agent.NewBaseAgent(
    agent.WithName("assistant"),
    agent.WithLLM(llm),
    agent.WithSystemPrompt("你是一个专业的助手"),
)
```

### ReActAgent

实现 ReAct (Reasoning + Acting) 模式，支持多步推理和工具调用：

```go
agent := agent.NewReActAgent(
    agent.WithName("researcher"),
    agent.WithLLM(llm),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithMaxIterations(10),
)
```

### RAGAgent

检索增强生成 Agent，结合知识库回答问题：

```go
agent := agent.NewRAGAgent(
    agent.WithName("knowledge-bot"),
    agent.WithLLM(llm),
    agent.WithRetriever(vectorRetriever),
    agent.WithTopK(5),
)
```

## 配置选项

### 基础配置

```go
agent.WithName("my-agent")           // 设置名称
agent.WithLLM(llm)                   // 设置 LLM
agent.WithSystemPrompt("...")        // 设置系统提示
agent.WithTemperature(0.7)           // 设置温度
agent.WithMaxTokens(2000)            // 设置最大 token
```

### 工具配置

```go
agent.WithTools(tool1, tool2)        // 添加工具
agent.WithToolChoice("auto")         // 工具选择策略
```

### 记忆配置

```go
agent.WithMemory(memory)             // 添加记忆
agent.WithMemoryWindow(10)           // 记忆窗口大小
```

## 中间件系统

### 内置中间件

```go
// Panic 恢复
agent.RecoverMiddleware()

// 日志记录
agent.LoggingMiddleware(logger)

// 指标采集
agent.MetricsMiddleware(collector)

// 超时控制
agent.TimeoutMiddleware(30*time.Second)

// 重试机制
agent.RetryMiddleware(3, time.Second)

// 追踪
agent.TracingMiddleware("my-service")

// 限流
agent.RateLimitMiddleware(limiter)
```

### 组合中间件

```go
chain := agent.NewMiddlewareChain(
    agent.RecoverMiddleware(),
    agent.LoggingMiddleware(nil),
    agent.TimeoutMiddleware(30*time.Second),
)

// 包装 Agent
handler := chain.WrapAgent(myAgent)

// 执行
output, err := handler(ctx, input)
```

### 预设组合

```go
// 默认中间件（Recover + Logging + Timeout）
middlewares := agent.DefaultMiddlewares()

// 生产环境中间件
middlewares := agent.ProductionMiddlewares("my-service", metricsCollector)
```

## 自定义中间件

```go
func MyMiddleware() agent.AgentMiddleware {
    return func(next agent.AgentHandler) agent.AgentHandler {
        return func(ctx context.Context, input agent.Input) (agent.Output, error) {
            // 前置处理
            log.Println("开始执行")

            // 调用下一个处理器
            output, err := next(ctx, input)

            // 后置处理
            log.Println("执行完成")

            return output, err
        }
    }
}
```

## 状态管理

### 四层状态

```go
// Turn 状态：单轮对话
state.Turn().Set("key", value)

// Session 状态：会话级别
state.Session().Set("user_id", "123")

// Agent 状态：Agent 持久状态
state.Agent().Set("config", config)

// Global 状态：全局共享
state.Global().Set("shared_data", data)
```

## 角色系统

定义 Agent 的角色特征：

```go
agent := agent.NewBaseAgent(
    agent.WithRole(agent.Role{
        Name:      "研究员",
        Goal:      "收集和分析信息",
        Backstory: "你是一位经验丰富的研究员，擅长从复杂信息中提取关键内容",
    }),
)
```

## 团队协作

创建多 Agent 团队：

```go
// 创建团队成员
researcher := agent.NewReActAgent(...)
writer := agent.NewBaseAgent(...)
reviewer := agent.NewBaseAgent(...)

// 创建团队
team := agent.NewTeam(
    agent.WithTeamMode(agent.SequentialMode),
    agent.WithAgents(researcher, writer, reviewer),
)

// 执行团队任务
output, err := team.Run(ctx, input)
```

### 团队模式

- `SequentialMode`: 顺序执行
- `ParallelMode`: 并行执行
- `HierarchicalMode`: 层级执行
- `ConsensusMode`: 共识执行

## 流式输出

```go
// 获取流式响应
stream, err := agent.Stream(ctx, input)
if err != nil {
    return err
}

// 处理流式数据
for chunk := range stream.Next() {
    fmt.Print(chunk.Content)
}
```

## 错误处理

```go
output, err := agent.Run(ctx, input)
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        // 处理超时
    case errors.Is(err, agent.ErrToolFailed):
        // 处理工具失败
    default:
        // 处理其他错误
    }
}
```

## 最佳实践

1. **使用中间件**: 始终添加 RecoverMiddleware 防止 panic
2. **设置超时**: 避免无限等待
3. **添加日志**: 便于调试和监控
4. **限制迭代**: ReActAgent 设置合理的 MaxIterations
5. **清理资源**: 使用 defer 确保资源释放
