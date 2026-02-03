# Hexagon

**六边形战士** - Go 生态的生产级 AI Agent 框架

[![Go Reference](https://pkg.go.dev/badge/github.com/everyday-items/hexagon.svg)](https://pkg.go.dev/github.com/everyday-items/hexagon)
[![Go Report Card](https://goreportcard.com/badge/github.com/everyday-items/hexagon)](https://goreportcard.com/report/github.com/everyday-items/hexagon)
[![CI](https://github.com/everyday-items/hexagon/workflows/CI/badge.svg)](https://github.com/everyday-items/hexagon/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

命名源自网络热词"六边形战士"，寓意各维度能力均衡强大、没有短板。框架在易用性、性能、扩展性、编排、可观测、安全六个维度追求均衡卓越。

```
      易用性 ⬡
        ╱ ╲
   安全 ⬡   ⬡ 性能
      │     │
 可观测 ⬡   ⬡ 扩展性
        ╲ ╱
      编排 ⬡
```

## 特性

- **极简入门** - 3 行代码开始，渐进式复杂度
- **类型安全** - Go 泛型支持，编译时类型检查
- **高性能** - 原生并发，支持 100k+ 并发 Agent
- **可观测** - 钩子 + 追踪 + 指标，OpenTelemetry 原生支持
- **生产就绪** - 安全防护，优雅降级，企业级稳定性

## 生态系统

Hexagon 是一个完整的 AI Agent 开发生态，由多个仓库组成：

| 仓库 | 说明 | 链接 |
|-----|------|------|
| **hexagon** | AI Agent 框架核心 (编排、RAG、Graph、Hooks) | [github.com/everyday-items/hexagon](https://github.com/everyday-items/hexagon) |
| **ai-core** | AI 基础能力库 (LLM/Tool/Memory/Schema) | [github.com/everyday-items/ai-core](https://github.com/everyday-items/ai-core) |
| **toolkit** | Go 通用工具库 (lang/crypto/net/cache/util) | [github.com/everyday-items/toolkit](https://github.com/everyday-items/toolkit) |
| **hexagon-ui** | Dev UI 前端 (Vue 3 + TypeScript) | [github.com/everyday-items/hexagon-ui](https://github.com/everyday-items/hexagon-ui) |

### ai-core - AI 基础能力库

提供 LLM、Tool、Memory、Schema 等核心抽象，支持多种 LLM Provider：

```go
import "github.com/everyday-items/ai-core/llm"
import "github.com/everyday-items/ai-core/llm/openai"
import "github.com/everyday-items/ai-core/tool"
import "github.com/everyday-items/ai-core/memory"
```

**主要模块：**
- `llm/` - LLM Provider 接口 + 实现 (OpenAI, DeepSeek, Anthropic, Gemini, 通义, 豆包, Ollama)
- `tool/` - 工具系统，支持函数式定义
- `memory/` - 记忆系统，支持向量存储
- `schema/` - JSON Schema 自动生成
- `streamx/` - 流式响应处理
- `template/` - Prompt 模板引擎

### toolkit - Go 通用工具库

生产级 Go 通用工具包，提供语言增强、加密、网络、缓存、协程池等基础能力：

```go
import "github.com/everyday-items/toolkit/lang/conv"      // 类型转换
import "github.com/everyday-items/toolkit/lang/stringx"   // 字符串工具
import "github.com/everyday-items/toolkit/lang/syncx"     // 并发工具
import "github.com/everyday-items/toolkit/net/httpx"      // HTTP 客户端
import "github.com/everyday-items/toolkit/net/sse"        // SSE 客户端
import "github.com/everyday-items/toolkit/util/retry"     // 重试机制
import "github.com/everyday-items/toolkit/util/idgen"     // ID 生成
import "github.com/everyday-items/toolkit/pool"           // 协程池
import "github.com/everyday-items/toolkit/cache/local"    // 本地缓存
```

**主要模块：**
- `lang/` - 语言增强 (conv, stringx, slicex, mapx, timex, contextx, errorx, syncx)
- `pool/` - 协程池 (高性能 goroutine 池，支持任务队列、动态扩缩容、优雅关闭)
- `crypto/` - 加密 (aes, rsa, sign)
- `net/` - 网络 (httpx, sse, ip)
- `cache/` - 缓存 (local, redis, multi)
- `util/` - 工具 (retry, rate, idgen, logger, validator)
- `collection/` - 数据结构 (set, list, queue, stack)

### hexagon-ui - Dev UI 前端

基于 Vue 3 + TypeScript 的开发调试界面：

```bash
cd hexagon-ui
npm install
npm run dev
# 访问 http://localhost:5173
```

**功能特性：**
- 实时事件流 (SSE 推送)
- 指标仪表板
- 事件详情查看
- LLM 流式输出展示

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
│  LLM Provider │ RAG Engine │ Tools System │ Memory System │ KB      │
├──────────────────────────────────────────────────────────────────────┤
│                        Infrastructure Layer                          │
│  Tracer │ Logger │ Metrics │ Config │ Security │ Cache │ Plugin │ DI │
└──────────────────────────────────────────────────────────────────────┘
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
├── store/              # 存储 (Vector/Qdrant/Milvus/Chroma)
├── plugin/             # 插件系统
├── config/             # 配置管理
├── evaluate/           # 评估系统
├── testing/            # 测试工具 (Mock/Record)
├── examples/           # 示例代码
└── hexagon.go          # 入口
```

## 文档

### 核心文档

| 文档 | 说明 |
|-----|------|
| [快速入门](docs/QUICKSTART.md) | 5 分钟上手 Hexagon |
| [架构设计](docs/DESIGN.md) | 框架设计理念和架构 |
| [API 参考](docs/API.md) | 完整 API 文档 |
| [稳定性说明](docs/STABILITY.md) | API 稳定性和版本策略 |
| [框架对比](docs/comparison.md) | 与主流框架的对比分析 |

### 使用指南

| 指南 | 说明 |
|-----|------|
| [快速开始](docs/guides/getting-started.md) | 从零开始构建第一个 Agent |
| [Agent 开发](docs/guides/agent-guide.md) | Agent 开发完整指南 |
| [Agent 进阶](docs/guides/agent-development.md) | 高级 Agent 开发模式 |
| [RAG 系统](docs/guides/rag-guide.md) | 检索增强生成入门 |
| [RAG 集成](docs/guides/rag-integration.md) | RAG 系统深度集成 |
| [图编排](docs/guides/graph-orchestration.md) | 复杂工作流编排 |
| [多 Agent](docs/guides/multi-agent.md) | 多 Agent 协作系统 |
| [插件开发](docs/guides/plugin-guide.md) | 插件系统使用指南 |
| [可观测性](docs/guides/observability.md) | 追踪、指标、日志集成 |
| [安全防护](docs/guides/security.md) | 安全最佳实践 |
| [性能优化](docs/guides/performance-optimization.md) | 性能调优指南 |

### 示例代码

| 示例 | 说明 |
|-----|------|
| [examples/quickstart](examples/quickstart) | 快速入门示例 |
| [examples/react](examples/react) | ReAct Agent 示例 |
| [examples/rag](examples/rag) | RAG 检索示例 |
| [examples/graph](examples/graph) | 图编排示例 |
| [examples/team](examples/team) | 多 Agent 团队示例 |
| [examples/handoff](examples/handoff) | Agent 交接示例 |
| [examples/devui](examples/devui) | Dev UI 示例 |

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

**运行示例：**

```bash
# 启动后端
go run examples/devui/main.go

# 启动前端 (hexagon-ui)
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

## 贡献

欢迎贡献！请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解如何参与。

## 许可证

[MIT License](LICENSE)

```
MIT License

Copyright (c) 2024 everyday-items

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
