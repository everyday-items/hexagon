<div align="right">Language: <a href="plugin-guide.md">中文</a> | English</div>

# Plugin Development Guide

This guide explains how to develop and use plugins for Hexagon.

## Overview

The Hexagon plugin system allows you to:

- Extend framework functionality
- Add new LLM Providers
- Integrate external services
- Customize middleware
- Dynamically load and unload components

## Plugin Structure

### Plugin Interface

```go
type Plugin interface {
    // Plugin metadata
    Name() string
    Version() string
    Description() string

    // Lifecycle
    Init(ctx context.Context, config map[string]any) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Dependency declarations
    Dependencies() []string
}
```

### Basic Implementation

```go
type MyPlugin struct {
    config map[string]any
}

func (p *MyPlugin) Name() string        { return "my-plugin" }
func (p *MyPlugin) Version() string     { return "1.0.0" }
func (p *MyPlugin) Description() string { return "My custom plugin" }
func (p *MyPlugin) Dependencies() []string { return nil }

func (p *MyPlugin) Init(ctx context.Context, config map[string]any) error {
    p.config = config
    return nil
}

func (p *MyPlugin) Start(ctx context.Context) error {
    // Startup logic
    return nil
}

func (p *MyPlugin) Stop(ctx context.Context) error {
    // Shutdown logic
    return nil
}
```

## Plugin Registration

### Code-Based Registration

```go
import "github.com/hexagon-codes/hexagon/plugin"

// Register a plugin
plugin.Register(&MyPlugin{})

// Retrieve a plugin
p, err := plugin.Get("my-plugin")
```

### Configuration File Registration

```yaml
# plugins.yaml
plugins:
  - name: my-plugin
    enabled: true
    config:
      key1: value1
      key2: value2
```

```go
// Load from configuration
loader := plugin.NewLoader()
err := loader.LoadFromConfig("plugins.yaml")
```

## Plugin Lifecycle

### Lifecycle Manager

```go
lifecycle := plugin.NewLifecycle()

// Add plugins
lifecycle.Add(plugin1)
lifecycle.Add(plugin2)

// Initialize all plugins
err := lifecycle.InitAll(ctx)

// Start all plugins
err := lifecycle.StartAll(ctx)

// Stop all plugins
err := lifecycle.StopAll(ctx)
```

### Lifecycle Hooks

```go
lifecycle.OnInit(func(name string) {
    log.Printf("Plugin %s initializing...", name)
})

lifecycle.OnStart(func(name string) {
    log.Printf("Plugin %s starting...", name)
})

lifecycle.OnStop(func(name string) {
    log.Printf("Plugin %s stopping...", name)
})

lifecycle.OnError(func(name string, err error) {
    log.Printf("Plugin %s error: %v", name, err)
})
```

## Dependency Management

### Declaring Dependencies

```go
func (p *MyPlugin) Dependencies() []string {
    return []string{"database-plugin", "cache-plugin"}
}
```

### Version Constraints

```go
func (p *MyPlugin) Dependencies() []string {
    return []string{
        "database-plugin>=1.0.0",
        "cache-plugin~1.2.0",
    }
}
```

### Dependency Resolution

```go
graph := plugin.NewDependencyGraph()
graph.Add(plugin1)
graph.Add(plugin2)

// Check for circular dependencies
if err := graph.CheckCycle(); err != nil {
    log.Fatal("Circular dependency detected:", err)
}

// Topological sort
sorted, err := graph.TopologicalSort()
```

## Hot Reload

### Enabling Hot Reload

```go
reloader := plugin.NewHotReloader(lifecycle)

// Watch a plugin directory
reloader.Watch("./plugins")

// Start watching
reloader.Start(ctx)
defer reloader.Stop()
```

### Version Rollback

```go
// Get version history
history := reloader.GetVersionHistory("my-plugin")

// Roll back to a specific version
err := reloader.Rollback("my-plugin", "1.0.0")
```

## Health Checks

### Implementing Health Checks

```go
type HealthChecker interface {
    HealthCheck(ctx context.Context) error
}

func (p *MyPlugin) HealthCheck(ctx context.Context) error {
    // Check database connection
    if err := p.db.Ping(); err != nil {
        return fmt.Errorf("database connection failed: %w", err)
    }
    return nil
}
```

### Checking All Plugins

```go
results := lifecycle.HealthCheckAll(ctx)
for name, err := range results {
    if err != nil {
        log.Printf("Plugin %s is unhealthy: %v", name, err)
    }
}
```

## Plugin Types

### LLM Provider Plugin

```go
type LLMProviderPlugin struct {
    plugin.BasePlugin
    provider llm.Provider
}

func (p *LLMProviderPlugin) Init(ctx context.Context, config map[string]any) error {
    // Initialize the LLM Provider
    p.provider = newMyProvider(config)

    // Register globally
    llm.RegisterProvider("my-llm", p.provider)
    return nil
}
```

### Tool Plugin

```go
type ToolPlugin struct {
    plugin.BasePlugin
    tools []tool.Tool
}

func (p *ToolPlugin) Init(ctx context.Context, config map[string]any) error {
    // Create tools
    p.tools = []tool.Tool{
        newSearchTool(),
        newCalculatorTool(),
    }

    // Register tools
    for _, t := range p.tools {
        tool.Register(t)
    }
    return nil
}
```

### Middleware Plugin

```go
type MiddlewarePlugin struct {
    plugin.BasePlugin
}

func (p *MiddlewarePlugin) Init(ctx context.Context, config map[string]any) error {
    // Register middleware
    middleware.Register("my-middleware", func(next agent.AgentHandler) agent.AgentHandler {
        return func(ctx context.Context, input agent.Input) (agent.Output, error) {
            // Middleware logic
            return next(ctx, input)
        }
    })
    return nil
}
```

## Plugin Manifest

Describe your plugin using YAML:

```yaml
# manifest.yaml
name: my-plugin
version: 1.0.0
description: My custom plugin
author: Your Name

dependencies:
  - name: database-plugin
    version: ">=1.0.0"

config:
  schema:
    type: object
    properties:
      apiKey:
        type: string
        required: true
      timeout:
        type: integer
        default: 30

hooks:
  init: scripts/init.sh
  start: scripts/start.sh
```

## Debugging Plugins

### Logging

```go
func (p *MyPlugin) Init(ctx context.Context, config map[string]any) error {
    log.Printf("[%s] Initializing with config: %+v", p.Name(), config)
    return nil
}
```

### Metrics

```go
import "github.com/hexagon-codes/hexagon/observe/metrics"

func (p *MyPlugin) Start(ctx context.Context) error {
    // Record startup metric
    metrics.IncCounter("plugin_starts_total", "plugin", p.Name())
    return nil
}
```

## Best Practices

1. **Single Responsibility**: Each plugin should focus on one specific function
2. **Graceful Degradation**: Plugin failures should not affect the main application
3. **Configuration Validation**: Validate configuration completeness during `Init`
4. **Resource Cleanup**: Release all resources during `Stop`
5. **Version Management**: Follow semantic versioning
6. **Clear Documentation**: Provide clear usage instructions

## Example Projects

Full examples are available in the `examples/plugins/` directory:

```
examples/plugins/
├── llm-provider/      # LLM Provider plugin example
├── tool-plugin/       # Tool plugin example
├── middleware/        # Middleware plugin example
└── full-example/      # Complete plugin example
```
