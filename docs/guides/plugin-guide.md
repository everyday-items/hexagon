# 插件开发指南

本指南介绍如何为 Hexagon 开发和使用插件。

## 概述

Hexagon 插件系统允许您：

- 扩展框架功能
- 添加新的 LLM Provider
- 集成外部服务
- 自定义中间件
- 动态加载和卸载

## 插件结构

### 插件接口

```go
type Plugin interface {
    // 插件元信息
    Name() string
    Version() string
    Description() string

    // 生命周期
    Init(ctx context.Context, config map[string]any) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // 依赖声明
    Dependencies() []string
}
```

### 基础实现

```go
type MyPlugin struct {
    config map[string]any
}

func (p *MyPlugin) Name() string        { return "my-plugin" }
func (p *MyPlugin) Version() string     { return "1.0.0" }
func (p *MyPlugin) Description() string { return "我的自定义插件" }
func (p *MyPlugin) Dependencies() []string { return nil }

func (p *MyPlugin) Init(ctx context.Context, config map[string]any) error {
    p.config = config
    return nil
}

func (p *MyPlugin) Start(ctx context.Context) error {
    // 启动逻辑
    return nil
}

func (p *MyPlugin) Stop(ctx context.Context) error {
    // 停止逻辑
    return nil
}
```

## 插件注册

### 代码注册

```go
import "github.com/everyday-items/hexagon/plugin"

// 注册插件
plugin.Register(&MyPlugin{})

// 获取插件
p, err := plugin.Get("my-plugin")
```

### 配置文件注册

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
// 从配置加载
loader := plugin.NewLoader()
err := loader.LoadFromConfig("plugins.yaml")
```

## 插件生命周期

### 生命周期管理器

```go
lifecycle := plugin.NewLifecycle()

// 添加插件
lifecycle.Add(plugin1)
lifecycle.Add(plugin2)

// 初始化所有插件
err := lifecycle.InitAll(ctx)

// 启动所有插件
err := lifecycle.StartAll(ctx)

// 停止所有插件
err := lifecycle.StopAll(ctx)
```

### 生命周期钩子

```go
lifecycle.OnInit(func(name string) {
    log.Printf("插件 %s 初始化中...", name)
})

lifecycle.OnStart(func(name string) {
    log.Printf("插件 %s 启动中...", name)
})

lifecycle.OnStop(func(name string) {
    log.Printf("插件 %s 停止中...", name)
})

lifecycle.OnError(func(name string, err error) {
    log.Printf("插件 %s 错误: %v", name, err)
})
```

## 依赖管理

### 声明依赖

```go
func (p *MyPlugin) Dependencies() []string {
    return []string{"database-plugin", "cache-plugin"}
}
```

### 版本约束

```go
func (p *MyPlugin) Dependencies() []string {
    return []string{
        "database-plugin>=1.0.0",
        "cache-plugin~1.2.0",
    }
}
```

### 依赖解析

```go
graph := plugin.NewDependencyGraph()
graph.Add(plugin1)
graph.Add(plugin2)

// 检查循环依赖
if err := graph.CheckCycle(); err != nil {
    log.Fatal("存在循环依赖:", err)
}

// 拓扑排序
sorted, err := graph.TopologicalSort()
```

## 热重载

### 启用热重载

```go
reloader := plugin.NewHotReloader(lifecycle)

// 监听插件目录
reloader.Watch("./plugins")

// 启动监听
reloader.Start(ctx)
defer reloader.Stop()
```

### 版本回滚

```go
// 获取版本历史
history := reloader.GetVersionHistory("my-plugin")

// 回滚到指定版本
err := reloader.Rollback("my-plugin", "1.0.0")
```

## 健康检查

### 实现健康检查

```go
type HealthChecker interface {
    HealthCheck(ctx context.Context) error
}

func (p *MyPlugin) HealthCheck(ctx context.Context) error {
    // 检查数据库连接
    if err := p.db.Ping(); err != nil {
        return fmt.Errorf("数据库连接失败: %w", err)
    }
    return nil
}
```

### 检查所有插件

```go
results := lifecycle.HealthCheckAll(ctx)
for name, err := range results {
    if err != nil {
        log.Printf("插件 %s 不健康: %v", name, err)
    }
}
```

## 插件类型

### LLM Provider 插件

```go
type LLMProviderPlugin struct {
    plugin.BasePlugin
    provider llm.Provider
}

func (p *LLMProviderPlugin) Init(ctx context.Context, config map[string]any) error {
    // 初始化 LLM Provider
    p.provider = newMyProvider(config)

    // 注册到全局
    llm.RegisterProvider("my-llm", p.provider)
    return nil
}
```

### 工具插件

```go
type ToolPlugin struct {
    plugin.BasePlugin
    tools []tool.Tool
}

func (p *ToolPlugin) Init(ctx context.Context, config map[string]any) error {
    // 创建工具
    p.tools = []tool.Tool{
        newSearchTool(),
        newCalculatorTool(),
    }

    // 注册工具
    for _, t := range p.tools {
        tool.Register(t)
    }
    return nil
}
```

### 中间件插件

```go
type MiddlewarePlugin struct {
    plugin.BasePlugin
}

func (p *MiddlewarePlugin) Init(ctx context.Context, config map[string]any) error {
    // 注册中间件
    middleware.Register("my-middleware", func(next agent.AgentHandler) agent.AgentHandler {
        return func(ctx context.Context, input agent.Input) (agent.Output, error) {
            // 中间件逻辑
            return next(ctx, input)
        }
    })
    return nil
}
```

## 插件清单

使用 YAML 描述插件：

```yaml
# manifest.yaml
name: my-plugin
version: 1.0.0
description: 我的自定义插件
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

## 调试插件

### 日志

```go
func (p *MyPlugin) Init(ctx context.Context, config map[string]any) error {
    log.Printf("[%s] 初始化配置: %+v", p.Name(), config)
    return nil
}
```

### 指标

```go
import "github.com/everyday-items/hexagon/observe/metrics"

func (p *MyPlugin) Start(ctx context.Context) error {
    // 记录启动指标
    metrics.IncCounter("plugin_starts_total", "plugin", p.Name())
    return nil
}
```

## 最佳实践

1. **单一职责**: 每个插件专注一个功能
2. **优雅降级**: 插件失败不影响主程序
3. **配置验证**: Init 时验证配置完整性
4. **资源清理**: Stop 时释放所有资源
5. **版本管理**: 遵循语义化版本
6. **文档完善**: 提供清晰的使用说明

## 示例项目

完整示例见 `examples/plugins/` 目录：

```
examples/plugins/
├── llm-provider/      # LLM Provider 插件示例
├── tool-plugin/       # 工具插件示例
├── middleware/        # 中间件插件示例
└── full-example/      # 完整插件示例
```
