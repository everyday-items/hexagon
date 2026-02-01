
# Hexagon API 稳定性说明

本文档说明 Hexagon 框架各模块的 API 稳定性等级和兼容性承诺。

## 版本规范

Hexagon 遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范：

```
MAJOR.MINOR.PATCH[-PRERELEASE]
```

- **MAJOR**: 不兼容的 API 更改
- **MINOR**: 向后兼容的功能新增
- **PATCH**: 向后兼容的 Bug 修复
- **PRERELEASE**: 预发布标识 (alpha, beta, GA)

## 稳定性等级

| 等级 | 说明 | 兼容性承诺 |
|:---:|------|-----------|
| **Stable** | 稳定 API，可用于生产 | MAJOR 版本内保持兼容 |
| **Beta** | 功能完整，API 可能微调 | MINOR 版本内尽量保持兼容 |
| **Alpha** | 实验性功能，API 可能大改 | 无兼容性承诺 |
| **Deprecated** | 已弃用，将在未来版本移除 | 至少保留 1 个 MINOR 版本 |

## 模块稳定性

### Stable (稳定)

以下 API 在 v1.x 版本内保持向后兼容：

**顶层 API** (`github.com/everyday-items/hexagon`)
- `Chat()`, `ChatWithTools()`, `Run()`
- `QuickStart()` 及其选项函数
- `NewTool()`
- 类型导出 (`Input`, `Output`, `Tool`, `Memory`, `Message`, `Schema`)

**核心接口** (`github.com/everyday-items/hexagon/core`)
- `Component[I, O]` 接口
- `Stream[T]` 接口
- `Schema` 类型

**Agent** (`github.com/everyday-items/hexagon/agent`)
- `Agent` 接口
- `Input`, `Output` 类型
- `NewReAct()` 及其选项函数
- `Role` 类型

**图编排** (`github.com/everyday-items/hexagon/orchestration/graph`)
- `State` 接口
- `MapState` 类型
- `NewGraph[S]()` 构建器
- `Graph[S].Run()`, `Graph[S].Stream()`
- `START`, `END` 常量

### Beta (测试)

以下 API 功能完整，但可能在 MINOR 版本中微调：

**多 Agent** (`github.com/everyday-items/hexagon/agent`)
- `Team` 及其选项函数
- `TeamMode` 常量
- `TransferTo()`, `SwarmRunner`
- `StateManager` 接口

**RAG** (`github.com/everyday-items/hexagon/rag`)
- `Engine` 及其选项函数
- `Document`, `Loader`, `Splitter`, `Retriever`, `Indexer`, `Embedder` 接口
- 内置加载器、分割器、检索器实现

**安全** (`github.com/everyday-items/hexagon/security`)
- `Guard` 接口
- `NewPromptInjectionGuard()`, `NewPIIGuard()`
- `GuardChain` 及其模式
- `CostController` 及其选项函数

**可观测性** (`github.com/everyday-items/hexagon/observe`)
- `Tracer`, `Span` 接口
- `Metrics` 接口
- `NewTracer()`, `NewMetrics()`

### Alpha (实验)

以下 API 处于实验阶段，可能有较大改动：

**工作流** (`github.com/everyday-items/hexagon/orchestration/workflow`)
- `Workflow`, `Step` 类型
- 持久化接口

**检查点** (`github.com/everyday-items/hexagon/orchestration/graph`)
- `CheckpointSaver` 接口
- Redis 检查点实现
- 中断和恢复功能

**向量存储** (`github.com/everyday-items/hexagon/store/vector`)
- `VectorStore` 接口
- Qdrant 实现

### Deprecated (已弃用)

当前无已弃用的 API。

## 兼容性策略

### 向后兼容的更改

以下更改被视为向后兼容：

- 添加新的导出函数、类型、常量
- 为现有函数添加可选参数（通过选项函数模式）
- 为接口添加具有默认实现的方法
- 改进错误消息
- 修复 Bug

### 不兼容的更改

以下更改被视为不兼容（需要增加 MAJOR 版本）：

- 删除或重命名导出的函数、类型、常量
- 更改函数签名（参数或返回值）
- 更改接口定义
- 更改已有行为的语义

## 弃用流程

1. **标记弃用**: 在文档和代码注释中标记 `Deprecated`
2. **迁移指南**: 提供迁移到新 API 的说明
3. **警告日志**: 运行时输出弃用警告
4. **保留期**: 至少保留 1 个 MINOR 版本周期
5. **移除**: 在下一个 MAJOR 版本中移除

## 导入路径稳定性

以下导入路径是稳定的：

```go
import "github.com/everyday-items/hexagon"                  // 顶层 API
import "github.com/everyday-items/hexagon/agent"            // Agent
import "github.com/everyday-items/hexagon/core"             // 核心接口
import "github.com/everyday-items/hexagon/orchestration/graph" // 图编排
import "github.com/everyday-items/hexagon/rag"              // RAG
import "github.com/everyday-items/hexagon/security/guard"   // 安全守卫
import "github.com/everyday-items/hexagon/observe/tracer"   // 追踪
import "github.com/everyday-items/hexagon/observe/metrics"  // 指标
```

`internal/` 包不对外公开，可能随时更改。

## 依赖稳定性

Hexagon 依赖以下外部库：

| 依赖 | 版本 | 说明 |
|-----|------|------|
| `github.com/everyday-items/ai-core` | v1.x | AI 基础能力库 |
| `github.com/everyday-items/toolkit` | v1.x | Go 通用工具库 |

这些依赖的公开 API 变更会同步反映在 Hexagon 的版本号中。

## 反馈

如果您对 API 稳定性有任何问题或建议：

- 提交 [GitHub Issue](https://github.com/everyday-items/hexagon/issues)
- 参与 [GitHub Discussions](https://github.com/everyday-items/hexagon/discussions)
