<div align="right">Language: <a href="agent-guide.md">中文</a> | English</div>

# Agent Development Guide

This guide covers how to develop various types of AI Agents with Hexagon.

## Agent Types

### BaseAgent

The foundational Agent that provides the simplest LLM invocation capability:

```go
agent := agent.NewBaseAgent(
    agent.WithName("assistant"),
    agent.WithLLM(llm),
    agent.WithSystemPrompt("You are a professional assistant"),
)
```

### ReActAgent

Implements the ReAct (Reasoning + Acting) pattern, supporting multi-step reasoning and tool calls:

```go
agent := agent.NewReActAgent(
    agent.WithName("researcher"),
    agent.WithLLM(llm),
    agent.WithTools(searchTool, calculatorTool),
    agent.WithMaxIterations(10),
)
```

### RAGAgent

A retrieval-augmented generation Agent that answers questions by combining a knowledge base:

```go
agent := agent.NewRAGAgent(
    agent.WithName("knowledge-bot"),
    agent.WithLLM(llm),
    agent.WithRetriever(vectorRetriever),
    agent.WithTopK(5),
)
```

## Configuration Options

### Basic Configuration

```go
agent.WithName("my-agent")           // set name
agent.WithLLM(llm)                   // set LLM
agent.WithSystemPrompt("...")        // set system prompt
agent.WithTemperature(0.7)           // set temperature
agent.WithMaxTokens(2000)            // set max tokens
```

### Tool Configuration

```go
agent.WithTools(tool1, tool2)        // add tools
agent.WithToolChoice("auto")         // tool selection strategy
```

### Memory Configuration

```go
agent.WithMemory(memory)             // add memory
agent.WithMemoryWindow(10)           // memory window size
```

## Middleware System

### Built-in Middleware

```go
// Panic recovery
agent.RecoverMiddleware()

// Logging
agent.LoggingMiddleware(logger)

// Metrics collection
agent.MetricsMiddleware(collector)

// Timeout control
agent.TimeoutMiddleware(30*time.Second)

// Retry mechanism
agent.RetryMiddleware(3, time.Second)

// Tracing
agent.TracingMiddleware("my-service")

// Rate limiting
agent.RateLimitMiddleware(limiter)
```

### Composing Middleware

```go
chain := agent.NewMiddlewareChain(
    agent.RecoverMiddleware(),
    agent.LoggingMiddleware(nil),
    agent.TimeoutMiddleware(30*time.Second),
)

// Wrap the Agent
handler := chain.WrapAgent(myAgent)

// Run
output, err := handler(ctx, input)
```

### Preset Combinations

```go
// Default middleware (Recover + Logging + Timeout)
middlewares := agent.DefaultMiddlewares()

// Production middleware
middlewares := agent.ProductionMiddlewares("my-service", metricsCollector)
```

## Custom Middleware

```go
func MyMiddleware() agent.AgentMiddleware {
    return func(next agent.AgentHandler) agent.AgentHandler {
        return func(ctx context.Context, input agent.Input) (agent.Output, error) {
            // Pre-processing
            log.Println("Starting execution")

            // Call the next handler
            output, err := next(ctx, input)

            // Post-processing
            log.Println("Execution complete")

            return output, err
        }
    }
}
```

## State Management

### Four-Layer State

```go
// Turn state: single conversation turn
state.Turn().Set("key", value)

// Session state: conversation level
state.Session().Set("user_id", "123")

// Agent state: persistent Agent state
state.Agent().Set("config", config)

// Global state: cross-Agent shared state
state.Global().Set("shared_data", data)
```

## Role System

Define the role characteristics of an Agent:

```go
agent := agent.NewBaseAgent(
    agent.WithRole(agent.Role{
        Name:      "Researcher",
        Goal:      "Collect and analyze information",
        Backstory: "You are an experienced researcher skilled at extracting key insights from complex information",
    }),
)
```

## Team Collaboration

Create a multi-Agent team:

```go
// Create team members
researcher := agent.NewReActAgent(...)
writer := agent.NewBaseAgent(...)
reviewer := agent.NewBaseAgent(...)

// Create the team
team := agent.NewTeam(
    agent.WithTeamMode(agent.SequentialMode),
    agent.WithAgents(researcher, writer, reviewer),
)

// Run the team task
output, err := team.Run(ctx, input)
```

### Team Modes

- `SequentialMode`: sequential execution
- `ParallelMode`: parallel execution
- `HierarchicalMode`: hierarchical execution
- `ConsensusMode`: consensus-based execution

## Streaming Output

```go
// Get a streaming response
stream, err := agent.Stream(ctx, input)
if err != nil {
    return err
}

// Process streaming data
for chunk := range stream.Next() {
    fmt.Print(chunk.Content)
}
```

## Error Handling

```go
output, err := agent.Run(ctx, input)
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        // handle timeout
    case errors.Is(err, agent.ErrToolFailed):
        // handle tool failure
    default:
        // handle other errors
    }
}
```

## Best Practices

1. **Use middleware**: always add `RecoverMiddleware` to guard against panics
2. **Set timeouts**: avoid indefinite blocking
3. **Add logging**: facilitates debugging and monitoring
4. **Limit iterations**: set a reasonable `MaxIterations` for `ReActAgent`
5. **Clean up resources**: use `defer` to ensure resources are released
