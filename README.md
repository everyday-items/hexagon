# Hexagon

Go 生态的生产级 AI Agent 框架。

[![Go Reference](https://pkg.go.dev/badge/github.com/everyday-items/hexagon.svg)](https://pkg.go.dev/github.com/everyday-items/hexagon)
[![Go Report Card](https://goreportcard.com/badge/github.com/everyday-items/hexagon)](https://goreportcard.com/report/github.com/everyday-items/hexagon)
[![CI](https://github.com/everyday-items/hexagon/workflows/CI/badge.svg)](https://github.com/everyday-items/hexagon/actions)
[![License](https://img.shields.io/github/license/everyday-items/hexagon)](LICENSE)

## 特性

- **极简入门** - 3 行代码开始，渐进式复杂度
- **类型安全** - Go 泛型支持，编译时类型检查
- **高性能** - 原生并发，支持 100k+ 并发 Agent
- **可观测** - 钩子 + 追踪 + 指标，OpenTelemetry 原生支持
- **生产就绪** - 安全防护，优雅降级，企业级稳定性

## 快速开始

### 安装

```bash
go get github.com/everyday-items/hexagon
```

### 环境配置

```bash
# OpenAI
export OPENAI_API_KEY=your-api-key

# 或 DeepSeek
export DEEPSEEK_API_KEY=your-api-key
```

### 3 行代码入门

```go
package main

import (
    "context"
    "fmt"
    "github.com/everyday-items/hexagon"
)

func main() {
    response, _ := hexagon.Chat(context.Background(), "What is Go?")
    fmt.Println(response)
}
```

### 带工具的 Agent

```go
package main

import (
    "context"
    "fmt"
    "github.com/everyday-items/hexagon"
)

func main() {
    // 定义计算器工具
    type CalcInput struct {
        A  float64 `json:"a" desc:"第一个数字" required:"true"`
        B  float64 `json:"b" desc:"第二个数字" required:"true"`
        Op string  `json:"op" desc:"运算符" required:"true" enum:"add,sub,mul,div"`
    }

    calculator := hexagon.NewTool("calculator", "执行数学计算",
        func(ctx context.Context, input CalcInput) (float64, error) {
            switch input.Op {
            case "add": return input.A + input.B, nil
            case "sub": return input.A - input.B, nil
            case "mul": return input.A * input.B, nil
            case "div": return input.A / input.B, nil
            }
            return 0, fmt.Errorf("unknown operator")
        },
    )

    // 创建带工具的 Agent
    agent := hexagon.QuickStart(
        hexagon.WithTools(calculator),
        hexagon.WithSystemPrompt("你是一个数学助手"),
    )

    output, _ := agent.Run(context.Background(), hexagon.Input{
        Query: "计算 123 * 456",
    })
    fmt.Println(output.Content)
}
```

### RAG 检索增强

```go
// 创建 RAG 引擎
engine := hexagon.NewRAGEngine(
    hexagon.WithRAGStore(hexagon.NewMemoryVectorStore()),
    hexagon.WithRAGEmbedder(hexagon.NewOpenAIEmbedder()),
)

// 索引文档
engine.Index(ctx, []hexagon.Document{
    {ID: "1", Content: "Go 支持并发编程"},
    {ID: "2", Content: "Go 有丰富的标准库"},
})

// 检索
docs, _ := engine.Retrieve(ctx, "Go 的特性", hexagon.WithTopK(2))
```

### 图编排

```go
import "github.com/everyday-items/hexagon/orchestration/graph"

// 构建工作流图
g, _ := graph.NewGraph[MyState]("workflow").
    AddNode("analyze", analyzeHandler).
    AddNode("process", processHandler).
    AddEdge(graph.START, "analyze").
    AddEdge("analyze", "process").
    AddEdge("process", graph.END).
    Build()

// 执行
result, _ := g.Run(ctx, initialState)
```

### 多 Agent 团队

```go
// 创建团队
team := hexagon.NewTeam("research-team",
    hexagon.WithAgents(researcher, writer, reviewer),
    hexagon.WithMode(hexagon.TeamModeSequential),
)

// 执行
output, _ := team.Run(ctx, hexagon.Input{Query: "写一篇技术文章"})
```

## 设计理念

1. **渐进式复杂度** - 入门 3 行代码，进阶声明式配置，专家图编排
2. **约定优于配置** - 合理默认值，零配置可运行
3. **组合优于继承** - 小而专注的组件，灵活组合
4. **显式优于隐式** - 类型安全，编译时检查
5. **生产优先** - 内置可观测性，优雅降级

## 架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Application Layer                            │
│  Chat Bot │ RAG Agent │ Workflow Engine │ Multi-Agent │ Custom Agent │
├──────────────────────────────────────────────────────────────────────┤
│                        Orchestration Layer                           │
│  Router │ Planner │ Scheduler │ Executor │ Graph │ Workflow │ State  │
├──────────────────────────────────────────────────────────────────────┤
│                          Agent Core Layer                            │
│  Agent │ Role │ Team │ Network │ Context │ State │ Lifecycle │ Msg   │
├──────────────────────────────────────────────────────────────────────┤
│                         Capability Layer                             │
│  LLM Provider │ RAG Engine │ Tools System │ Memory System │ KB       │
├──────────────────────────────────────────────────────────────────────┤
│                        Infrastructure Layer                          │
│  Tracer │ Logger │ Metrics │ Config │ Security │ Cache │ Plugin │ DI │
└──────────────────────────────────────────────────────────────────────┘
```

## 核心概念

### Component (统一执行模型)

所有组件实现相同接口，可任意组合：

```go
type Component[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
    Stream(ctx context.Context, input I) (Stream[O], error)
    Batch(ctx context.Context, inputs []I) ([]O, error)
}
```

### Agent

```go
type Agent interface {
    Component[Input, Output]
    ID() string
    Tools() []Tool
    Memory() Memory
}
```

### Tool

```go
// 函数式工具定义
calculator := hexagon.NewTool("calculator", "执行计算",
    func(ctx context.Context, input struct {
        A float64 `json:"a"`
        B float64 `json:"b"`
    }) (float64, error) {
        return input.A + input.B, nil
    },
)
```

## LLM 支持

| Provider | 状态 |
|----------|------|
| OpenAI (GPT-4, GPT-4o, o1, o3) | ✅ 已支持 |
| DeepSeek | ✅ 已支持 |
| Anthropic (Claude) | ✅ 已支持 |
| Google Gemini | ✅ 已支持 |
| 通义千问 (Qwen) | ✅ 已支持 |
| 豆包 (Ark) | ✅ 已支持 |
| Ollama (本地模型) | ✅ 已支持 |

## 项目结构

```
hexagon/
├── agent/              # Agent 核心 (ReAct/Role/Team/Handoff/State)
├── core/               # 统一接口 (Component[I,O], Stream[T])
├── orchestration/      # 编排引擎
│   ├── graph/          # 图编排 (状态图 + 检查点)
│   ├── chain/          # 链式编排
│   ├── workflow/       # 工作流引擎
│   └── planner/        # 规划器
├── rag/                # RAG 系统
│   ├── loader/         # 文档加载
│   ├── splitter/       # 文档分割
│   ├── retriever/      # 检索器 (Vector/Keyword/Hybrid)
│   ├── reranker/       # 重排序
│   └── synthesizer/    # 响应合成
├── hooks/              # 钩子系统 (Run/Tool/LLM/Retriever)
├── observe/            # 可观测性 (Tracer/Metrics/OTel)
├── security/           # 安全防护 (Guard/RBAC/Cost/Audit)
├── tool/               # 工具系统 (File/Python/Shell/Sandbox)
├── store/              # 存储 (Vector/Qdrant)
├── plugin/             # 插件系统
├── testing/            # 测试工具 (Mock/Record)
├── examples/           # 示例代码
└── hexagon.go          # 入口
```

## Dev UI

内置开发调试界面，实时查看 Agent 执行过程。

```go
import "github.com/everyday-items/hexagon/observe/devui"

// 创建 DevUI
ui := devui.New(
    devui.WithAddr(":8080"),
    devui.WithMaxEvents(1000),
)

// 启动服务
go ui.Start()

// 访问 http://localhost:8080
```

**功能特性：**

- 🔄 实时事件流 (SSE 推送)
- 📊 指标仪表板
- 🔍 事件详情查看
- 🔧 事件类型过滤
- 💬 LLM 流式输出展示

**运行示例：**

```bash
go run examples/devui/main.go
# 访问 http://localhost:8080
```

**前端开发 (hexagon-ui)：**

```bash
# 启动后端
go run examples/devui/main.go

# 启动前端 (另一个终端)
cd ../hexagon-ui
npm install
npm run dev
# 访问 http://localhost:5173
```

## 开发

```bash
make build   # 构建
make test    # 测试
make lint    # 代码检查
make fmt     # 格式化
```

## 文档

- [快速入门](docs/QUICKSTART.md)
- [架构设计](docs/DESIGN.md)
- [API 参考](docs/API.md)
- [稳定性说明](docs/STABILITY.md)
- [示例代码](examples/)

## 贡献

欢迎贡献！请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解如何参与。

## 许可证

[Apache License 2.0](LICENSE)
