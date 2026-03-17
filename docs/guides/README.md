<div align="right">语言: 中文 | <a href="README.en.md">English</a></div>

# Hexagon 使用指南

欢迎使用 Hexagon AI Agent 框架！本目录包含详细的使用指南和最佳实践。

## 📦 生态系统

Hexagon 是一个完整的 AI Agent 开发生态：

| 仓库 | 说明 |
|-----|------|
| **hexagon** | AI Agent 框架核心 (编排、RAG、Graph、Hooks) |
| **ai-core** | AI 基础能力库 (LLM/Tool/Memory/Schema) |
| **toolkit** | Go 通用工具库 (lang/crypto/net/cache/util) |
| **hexagon-ui** | Dev UI 前端 (Vue 3 + TypeScript) |

## 📚 指南列表

### 入门指南

1. [**Agent 开发指南**](./agent-development.md)
   - 创建第一个 Agent
   - Agent 类型选择
   - 添加工具和记忆
   - 配置管理
   - 最佳实践

2. [**RAG 系统使用指南**](./rag-integration.md)
   - 文档加载和分割
   - 向量生成和存储
   - 检索策略
   - 答案合成
   - 性能优化

### 进阶指南

3. [**多 Agent 协作指南**](./multi-agent.md)
   - 角色系统
   - Team 工作模式
   - Agent 交接 (Handoff)
   - 网络通信
   - 共识机制

4. [**图编排最佳实践**](./graph-orchestration.md)
   - 基本图结构
   - 条件分支和循环
   - 并行执行
   - 中断和恢复
   - 检查点机制

### 部署运维

5. [**部署指南**](../../deploy/README.md)
   - Docker Compose 完整模式（一键启动）
   - Docker Compose 开发模式（连接 docker-dev-env）
   - Kubernetes / Helm Chart
   - 基础设施切换（内置/外部）

### 运维指南

6. [**可观测性集成指南**](./observability.md)
   - 分布式追踪 (OpenTelemetry)
   - 指标监控 (Prometheus)
   - 日志记录
   - Dev UI 使用

7. [**安全防护配置指南**](./security.md)
   - 输入验证和 Prompt 注入检测
   - PII 检测和脱敏
   - RBAC 访问控制
   - 成本控制
   - 审计日志

8. [**性能优化指南**](./performance-optimization.md)
   - Agent 优化
   - RAG 优化
   - 多 Agent 优化
   - 系统优化
   - 基准测试

## 🚀 快速导航

### 我想要...

- **创建一个简单的对话 Agent** → [Agent 开发指南](./agent-development.md#快速开始)
- **构建知识问答系统** → [RAG 系统使用指南](./rag-integration.md#快速开始)
- **多个 Agent 协作完成任务** → [多 Agent 协作指南](./multi-agent.md#team-协作)
- **实现复杂的工作流** → [图编排最佳实践](./graph-orchestration.md)
- **监控 Agent 性能** → [可观测性集成指南](./observability.md)
- **部署到 Docker/K8s** → [部署指南](../../deploy/README.md)
- **保护系统安全** → [安全防护配置指南](./security.md)
- **提升系统性能** → [性能优化指南](./performance-optimization.md)

## 📖 其他资源

- [快速开始](../QUICKSTART.md) - 5分钟快速入门
- [API 文档](../API.md) - 完整 API 参考
- [设计文档](../DESIGN.md) - 架构和设计理念
- [框架对比](../comparison.md) - 与主流框架的对比分析
- [示例代码](../../examples/) - 可运行的示例

## 💡 学习路径

### 初学者路径

1. 阅读 [快速开始](../QUICKSTART.md)
2. 学习 [Agent 开发指南](./agent-development.md)
3. 尝试 [examples/quickstart](../../examples/quickstart/)
4. 深入 [RAG 系统使用指南](./rag-integration.md)

### 进阶路径

1. 掌握 [多 Agent 协作指南](./multi-agent.md)
2. 学习 [图编排最佳实践](./graph-orchestration.md)
3. 配置 [可观测性](./observability.md)
4. 应用 [性能优化](./performance-optimization.md)

### 生产就绪路径

1. 实施 [安全防护](./security.md)
2. 配置 [可观测性](./observability.md)
3. 优化 [性能](./performance-optimization.md)
4. 建立监控和告警

## 🤝 获取帮助

- 📝 [GitHub Issues](https://github.com/hexagon-codes/hexagon/issues)
- 💬 [Discussions](https://github.com/hexagon-codes/hexagon/discussions)
- 📧 Email: support@hexagon-codes.com

## 📄 许可证

Hexagon 采用 Apache License 2.0 许可证开源。
