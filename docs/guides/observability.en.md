<div align="right">Language: <a href="observability.md">中文</a> | English</div>

# Observability Integration Guide

Hexagon provides a complete observability solution including tracing, metrics, and logging.

## Tracing

### In-Memory Tracer

```go
import "github.com/hexagon-codes/hexagon/observe/tracer"

tracer := tracer.NewMemoryTracer()

// Use the tracer in an Agent
agent := agent.NewBaseAgent(
    agent.WithTracer(tracer),
)

// Inspect traces
spans := tracer.GetSpans()
for _, span := range spans {
    fmt.Printf("%s: %v\n", span.Name, span.Duration)
}
```

### OpenTelemetry

```go
import "github.com/hexagon-codes/hexagon/observe/otel"

otelTracer := otel.NewOTelTracer(
    otel.WithServiceName("my-agent"),
    otel.WithEndpoint("localhost:4317"),
)

agent := agent.NewBaseAgent(
    agent.WithTracer(otelTracer),
)
```

## Metrics

### Prometheus

```go
import (
    "github.com/hexagon-codes/hexagon/observe/prometheus"
    "net/http"
)

exporter := prometheus.NewExporter(
    prometheus.WithNamespace("hexagon"),
)

// Expose the /metrics endpoint
http.Handle("/metrics", exporter.Handler())
go http.ListenAndServe(":9090", nil)

// Use metrics in an Agent
agent := agent.NewBaseAgent(
    agent.WithMetrics(exporter),
)
```

## Logging

```go
import "github.com/hexagon-codes/hexagon/observe/logger"

// Configure log level
logger.SetLevel(logger.LevelInfo)

// Agent will log automatically
agent := agent.NewBaseAgent(
    agent.WithLogger(logger.Default()),
)
```

## Dev UI

```go
import "github.com/hexagon-codes/hexagon/observe/devui"

ui := devui.New()
go ui.Start(":8080")

// Visit http://localhost:8080 to view real-time status
```

For more details, see [DESIGN.md](../DESIGN.md#可观测性).
