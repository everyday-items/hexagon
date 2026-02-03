# 可观测性集成指南

Hexagon 提供完整的可观测性方案，包括追踪、指标和日志。

## 追踪 (Tracing)

### 内存追踪器

```go
import "github.com/everyday-items/hexagon/observe/tracer"

tracer := tracer.NewMemoryTracer()

// Agent 使用追踪器
agent := agent.NewBaseAgent(
    agent.WithTracer(tracer),
)

// 查看追踪
spans := tracer.GetSpans()
for _, span := range spans {
    fmt.Printf("%s: %v\n", span.Name, span.Duration)
}
```

### OpenTelemetry

```go
import "github.com/everyday-items/hexagon/observe/otel"

otelTracer := otel.NewOTelTracer(
    otel.WithServiceName("my-agent"),
    otel.WithEndpoint("localhost:4317"),
)

agent := agent.NewBaseAgent(
    agent.WithTracer(otelTracer),
)
```

## 指标 (Metrics)

### Prometheus

```go
import (
    "github.com/everyday-items/hexagon/observe/prometheus"
    "net/http"
)

exporter := prometheus.NewExporter(
    prometheus.WithNamespace("hexagon"),
)

// 暴露 /metrics 端点
http.Handle("/metrics", exporter.Handler())
go http.ListenAndServe(":9090", nil)

// Agent 使用指标
agent := agent.NewBaseAgent(
    agent.WithMetrics(exporter),
)
```

## 日志 (Logging)

```go
import "github.com/everyday-items/hexagon/observe/logger"

// 配置日志级别
logger.SetLevel(logger.LevelInfo)

// Agent 会自动记录日志
agent := agent.NewBaseAgent(
    agent.WithLogger(logger.Default()),
)
```

## Dev UI

```go
import "github.com/everyday-items/hexagon/observe/devui"

ui := devui.New()
go ui.Start(":8080")

// 访问 http://localhost:8080 查看实时状态
```

更多详情参见 [DESIGN.md](../DESIGN.md#可观测性)。
