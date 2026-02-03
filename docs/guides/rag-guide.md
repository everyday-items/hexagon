# RAG 系统使用指南

本指南介绍如何使用 Hexagon 构建 RAG（检索增强生成）系统。

## 概述

RAG 系统通过检索相关文档来增强 LLM 的回答能力，主要包含以下步骤：

1. **文档加载**: 从各种来源加载文档
2. **文档分割**: 将长文档切分为适当大小的块
3. **向量化**: 将文档转换为向量表示
4. **索引存储**: 将向量存储到数据库
5. **检索**: 根据查询检索相关文档
6. **生成**: 基于检索结果生成回答

## 快速开始

```go
import (
    "github.com/everyday-items/hexagon/rag"
    "github.com/everyday-items/hexagon/store/vector/qdrant"
)

// 1. 创建向量存储
store, err := qdrant.New(ctx,
    qdrant.WithCollection("knowledge"),
    qdrant.WithDimension(1536),
)

// 2. 创建嵌入器
embedder := rag.NewOpenAIEmbedder(openaiClient)

// 3. 创建检索器
retriever := rag.NewVectorRetriever(store, embedder,
    rag.WithTopK(5),
)

// 4. 创建 RAG 管道
pipeline := rag.NewPipeline(
    rag.WithRetriever(retriever),
    rag.WithLLM(llm),
)

// 5. 查询
response, err := pipeline.Query(ctx, "什么是 Hexagon？")
```

## 文档加载

### 文本文件

```go
loader := rag.NewTextLoader("./docs")
docs, err := loader.Load(ctx)
```

### PDF 文件

```go
loader := rag.NewPDFLoader("./documents/manual.pdf")
docs, err := loader.Load(ctx)
```

### 网页

```go
loader := rag.NewWebLoader([]string{
    "https://example.com/page1",
    "https://example.com/page2",
})
docs, err := loader.Load(ctx)
```

### 自定义加载器

```go
type MyLoader struct{}

func (l *MyLoader) Load(ctx context.Context) ([]rag.Document, error) {
    // 自定义加载逻辑
    return docs, nil
}
```

## 文档分割

### 字符分割

```go
splitter := rag.NewCharacterSplitter(
    rag.WithChunkSize(1000),
    rag.WithChunkOverlap(200),
)
chunks, err := splitter.Split(docs)
```

### 递归分割

```go
splitter := rag.NewRecursiveSplitter(
    rag.WithSeparators([]string{"\n\n", "\n", " "}),
    rag.WithChunkSize(1000),
)
chunks, err := splitter.Split(docs)
```

### 语义分割

```go
splitter := rag.NewSemanticSplitter(embedder,
    rag.WithBreakpointThreshold(0.3),
)
chunks, err := splitter.Split(docs)
```

## 向量存储

### Qdrant

```go
store, err := qdrant.New(ctx,
    qdrant.WithHost("localhost"),
    qdrant.WithPort(6333),
    qdrant.WithCollection("documents"),
    qdrant.WithDimension(1536),
)
```

### Chroma

```go
store, err := chroma.NewStore(ctx,
    chroma.WithHost("localhost"),
    chroma.WithPort(8000),
    chroma.WithCollection("documents"),
)
```

### Milvus

```go
store, err := milvus.NewStore(ctx,
    milvus.WithAddress("localhost:19530"),
    milvus.WithCollection("documents"),
    milvus.WithDimension(1536),
)
```

### 内存存储（开发测试）

```go
store := vector.NewMemoryStore(1536)
```

## 检索策略

### 向量检索

```go
retriever := rag.NewVectorRetriever(store, embedder,
    rag.WithTopK(5),
    rag.WithMinScore(0.7),
)
```

### 关键词检索

```go
retriever := rag.NewKeywordRetriever(index,
    rag.WithTopK(10),
)
```

### 混合检索

```go
retriever := rag.NewHybridRetriever(
    vectorRetriever,
    keywordRetriever,
    rag.WithVectorWeight(0.7),
    rag.WithKeywordWeight(0.3),
)
```

## 重排序

提高检索结果的相关性：

### 分数过滤

```go
reranker := reranker.NewScoreReranker(
    reranker.WithScoreMin(0.5),
    reranker.WithScoreTopK(5),
)
```

### 跨编码器重排序

```go
reranker := reranker.NewCrossEncoderReranker(
    reranker.WithCrossEncoderModel("http://localhost:8080"),
    reranker.WithCrossEncoderTopK(5),
)
```

### Cohere 重排序

```go
reranker := reranker.NewCohereReranker(apiKey,
    reranker.WithCohereModel("rerank-english-v2.0"),
    reranker.WithCohereTopK(5),
)
```

### LLM 重排序

```go
reranker := reranker.NewLLMReranker(llm,
    reranker.WithLLMRerankerTopK(5),
)
```

### RRF 融合

合并多个检索结果：

```go
reranker := reranker.NewRRFReranker(
    reranker.WithRRFK(60),
    reranker.WithRRFTopK(10),
)

// 融合多个排名列表
results := reranker.FuseRankings(ranking1, ranking2, ranking3)
```

### 链式重排序

```go
chain := reranker.NewChainReranker(
    scoreReranker,
    crossEncoderReranker,
)
```

## 响应合成

### 简单合成

```go
synthesizer := rag.NewSimpleSynthesizer(llm)
response, err := synthesizer.Synthesize(ctx, query, docs)
```

### 精炼合成

迭代优化回答：

```go
synthesizer := rag.NewRefineSynthesizer(llm,
    rag.WithRefinePrompt("基于以下新信息完善回答..."),
)
```

### 压缩合成

压缩多个文档：

```go
synthesizer := rag.NewCompactSynthesizer(llm,
    rag.WithMaxTokens(4000),
)
```

## 完整管道

```go
// 配置管道
pipeline := rag.NewPipeline(
    // 检索配置
    rag.WithRetriever(hybridRetriever),

    // 重排序配置
    rag.WithReranker(crossEncoderReranker),

    // 合成配置
    rag.WithSynthesizer(refineSynthesizer),

    // LLM 配置
    rag.WithLLM(llm),

    // 其他配置
    rag.WithTopK(10),
    rag.WithMaxContextLength(4000),
)

// 执行查询
response, err := pipeline.Query(ctx, "你的问题")
```

## 索引管道

一次性完成文档索引：

```go
indexer := rag.NewIndexer(
    rag.WithLoader(loader),
    rag.WithSplitter(splitter),
    rag.WithEmbedder(embedder),
    rag.WithStore(store),
    rag.WithBatchSize(100),
)

// 索引文档
err := indexer.Index(ctx)
```

## 监控指标

```go
import "github.com/everyday-items/hexagon/observe/metrics"

collector := metrics.GetHexagonMetrics()

// 检索指标自动记录
// 也可以手动记录
collector.RecordRetrieval(ctx, "vector_search", docCount, duration)

// 查看统计
stats := collector.GetRetrievalStats()
fmt.Printf("平均检索时间: %v\n", stats.AverageDuration)
fmt.Printf("平均文档数: %.2f\n", stats.AverageDocCount)
```

## 最佳实践

1. **合理的分块大小**: 通常 500-1500 字符效果较好
2. **适当的重叠**: 10-20% 的重叠避免信息丢失
3. **多级检索**: 先粗筛后精排
4. **缓存嵌入**: 避免重复计算向量
5. **监控召回率**: 定期评估检索质量
6. **增量更新**: 支持文档的增删改
