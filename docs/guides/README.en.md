<div align="right">Language: <a href="README.md">中文</a> | English</div>

# Hexagon Guides

Welcome to the Hexagon AI Agent framework! This directory contains detailed usage guides and best practices.

## Ecosystem

Hexagon is a complete AI Agent development ecosystem:

| Repository | Description |
|-----|------|
| **hexagon** | AI Agent framework core (orchestration, RAG, Graph, Hooks) |
| **ai-core** | AI capability library (LLM/Tool/Memory/Schema) |
| **toolkit** | Go general-purpose toolkit (lang/crypto/net/cache/util) |
| **hexagon-ui** | Dev UI frontend (Vue 3 + TypeScript) |

## Guide Index

### Getting Started

1. [**Agent Development Guide**](./agent-development.en.md)
   - Creating your first Agent
   - Choosing the right Agent type
   - Adding tools and memory
   - Configuration management
   - Best practices

2. [**RAG System Integration Guide**](./rag-integration.en.md)
   - Document loading and splitting
   - Embedding generation and storage
   - Retrieval strategies
   - Answer synthesis
   - Performance optimization

### Advanced Guides

3. [**Multi-Agent Collaboration Guide**](./multi-agent.en.md)
   - Role system
   - Team working modes
   - Agent handoff
   - Network communication
   - Consensus mechanism

4. [**Graph Orchestration Best Practices**](./graph-orchestration.en.md)
   - Basic graph structure
   - Conditional branching and loops
   - Parallel execution
   - Interrupt and resume
   - Checkpoint mechanism

### Deployment & Operations

5. [**Deployment Guide**](../../deploy/README.md)
   - Docker Compose full mode (one-click startup)
   - Docker Compose development mode (connect to docker-dev-env)
   - Kubernetes / Helm Chart
   - Infrastructure switching (built-in / external)

### Operations Guides

6. [**Observability Integration Guide**](./observability.en.md)
   - Distributed tracing (OpenTelemetry)
   - Metrics monitoring (Prometheus)
   - Logging
   - Dev UI usage

7. [**Security Configuration Guide**](./security.en.md)
   - Input validation and prompt injection detection
   - PII detection and redaction
   - RBAC access control
   - Cost control
   - Audit logging

8. [**Performance Optimization Guide**](./performance-optimization.en.md)
   - Agent optimization
   - RAG optimization
   - Multi-Agent optimization
   - System optimization
   - Benchmarking

## Quick Navigation

### I want to...

- **Create a simple conversational Agent** → [Agent Development Guide](./agent-development.en.md#quick-start)
- **Build a knowledge Q&A system** → [RAG System Integration Guide](./rag-integration.en.md#quick-start)
- **Have multiple agents collaborate on a task** → [Multi-Agent Collaboration Guide](./multi-agent.en.md#team-collaboration)
- **Implement a complex workflow** → [Graph Orchestration Best Practices](./graph-orchestration.en.md)
- **Monitor Agent performance** → [Observability Integration Guide](./observability.en.md)
- **Deploy to Docker / K8s** → [Deployment Guide](../../deploy/README.md)
- **Secure the system** → [Security Configuration Guide](./security.en.md)
- **Improve system performance** → [Performance Optimization Guide](./performance-optimization.en.md)

## Other Resources

- [Quick Start](../QUICKSTART.en.md) - Get up and running in 5 minutes
- [API Documentation](../API.en.md) - Complete API reference
- [Design Document](../DESIGN.en.md) - Architecture and design philosophy
- [Framework Comparison](../comparison.en.md) - How Hexagon compares to mainstream frameworks
- [Example Code](../../examples/) - Runnable examples

## Learning Paths

### Beginner Path

1. Read [Quick Start](../QUICKSTART.en.md)
2. Study [Agent Development Guide](./agent-development.en.md)
3. Try [examples/quickstart](../../examples/quickstart/)
4. Explore [RAG System Integration Guide](./rag-integration.en.md)

### Advanced Path

1. Master [Multi-Agent Collaboration Guide](./multi-agent.en.md)
2. Study [Graph Orchestration Best Practices](./graph-orchestration.en.md)
3. Configure [Observability](./observability.en.md)
4. Apply [Performance Optimization](./performance-optimization.en.md)

### Production-Ready Path

1. Implement [Security](./security.en.md)
2. Configure [Observability](./observability.en.md)
3. Optimize [Performance](./performance-optimization.en.md)
4. Establish monitoring and alerting

## Get Help

- [GitHub Issues](https://github.com/hexagon-codes/hexagon/issues)
- [Discussions](https://github.com/hexagon-codes/hexagon/discussions)
- Email: support@hexagon-codes.com

## License

Hexagon is open-sourced under the Apache License 2.0.
