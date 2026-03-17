<div align="right">Language: <a href="rag-guide.md">中文</a> | English</div>

# RAG System User Guide

This guide explains how to build a RAG (Retrieval-Augmented Generation) system with Hexagon.

## Overview

A RAG system enhances LLM responses by retrieving relevant documents. The main steps are:

1. **Document Loading**: Load documents from various sources
2. **Document Splitting**: Split long documents into appropriately sized chunks
3. **Vectorization**: Convert documents into vector representations
4. **Index Storage**: Store vectors in a database
5. **Retrieval**: Retrieve relevant documents based on a query
6. **Generation**: Generate answers based on the retrieved results

## Quick Start

```go
import (
    "github.com/hexagon-codes/hexagon/rag"
    "github.com/hexagon-codes/hexagon/store/vector/qdrant"
)

// 1. Create vector store
store, err := qdrant.New(ctx,
    qdrant.WithCollection("knowledge"),
    qdrant.WithDimension(1536),
)

// 2. Create embedder
embedder := rag.NewOpenAIEmbedder(openaiClient)

// 3. Create retriever
retriever := rag.NewVectorRetriever(store, embedder,
    rag.WithTopK(5),
)

// 4. Create RAG pipeline
pipeline := rag.NewPipeline(
    rag.WithRetriever(retriever),
    rag.WithLLM(llm),
)

// 5. Query
response, err := pipeline.Query(ctx, "What is Hexagon?")
```

## Document Loading

### Text Files

```go
loader := rag.NewTextLoader("./docs")
docs, err := loader.Load(ctx)
```

### PDF Files

```go
loader := rag.NewPDFLoader("./documents/manual.pdf")
docs, err := loader.Load(ctx)
```

### Web Pages

```go
loader := rag.NewWebLoader([]string{
    "https://example.com/page1",
    "https://example.com/page2",
})
docs, err := loader.Load(ctx)
```

### Custom Loader

```go
type MyLoader struct{}

func (l *MyLoader) Load(ctx context.Context) ([]rag.Document, error) {
    // Custom loading logic
    return docs, nil
}
```

## Document Splitting

### Character Splitting

```go
splitter := rag.NewCharacterSplitter(
    rag.WithChunkSize(1000),
    rag.WithChunkOverlap(200),
)
chunks, err := splitter.Split(docs)
```

### Recursive Splitting

```go
splitter := rag.NewRecursiveSplitter(
    rag.WithSeparators([]string{"\n\n", "\n", " "}),
    rag.WithChunkSize(1000),
)
chunks, err := splitter.Split(docs)
```

### Semantic Splitting

```go
splitter := rag.NewSemanticSplitter(embedder,
    rag.WithBreakpointThreshold(0.3),
)
chunks, err := splitter.Split(docs)
```

## Vector Storage

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

### In-Memory Store (Development & Testing)

```go
store := vector.NewMemoryStore(1536)
```

## Retrieval Strategies

### Vector Retrieval

```go
retriever := rag.NewVectorRetriever(store, embedder,
    rag.WithTopK(5),
    rag.WithMinScore(0.7),
)
```

### Keyword Retrieval

```go
retriever := rag.NewKeywordRetriever(index,
    rag.WithTopK(10),
)
```

### Hybrid Retrieval

```go
retriever := rag.NewHybridRetriever(
    vectorRetriever,
    keywordRetriever,
    rag.WithVectorWeight(0.7),
    rag.WithKeywordWeight(0.3),
)
```

## Reranking

Improve the relevance of retrieved results:

### Score Filtering

```go
reranker := reranker.NewScoreReranker(
    reranker.WithScoreMin(0.5),
    reranker.WithScoreTopK(5),
)
```

### Cross-Encoder Reranking

```go
reranker := reranker.NewCrossEncoderReranker(
    reranker.WithCrossEncoderModel("http://localhost:8080"),
    reranker.WithCrossEncoderTopK(5),
)
```

### Cohere Reranking

```go
reranker := reranker.NewCohereReranker(apiKey,
    reranker.WithCohereModel("rerank-english-v2.0"),
    reranker.WithCohereTopK(5),
)
```

### LLM Reranking

```go
reranker := reranker.NewLLMReranker(llm,
    reranker.WithLLMRerankerTopK(5),
)
```

### RRF Fusion

Merge results from multiple retrievers:

```go
reranker := reranker.NewRRFReranker(
    reranker.WithRRFK(60),
    reranker.WithRRFTopK(10),
)

// Fuse multiple ranking lists
results := reranker.FuseRankings(ranking1, ranking2, ranking3)
```

### Chained Reranking

```go
chain := reranker.NewChainReranker(
    scoreReranker,
    crossEncoderReranker,
)
```

## Response Synthesis

### Simple Synthesis

```go
synthesizer := rag.NewSimpleSynthesizer(llm)
response, err := synthesizer.Synthesize(ctx, query, docs)
```

### Refine Synthesis

Iteratively refine the answer:

```go
synthesizer := rag.NewRefineSynthesizer(llm,
    rag.WithRefinePrompt("Improve the answer based on the following new information..."),
)
```

### Compact Synthesis

Compress multiple documents:

```go
synthesizer := rag.NewCompactSynthesizer(llm,
    rag.WithMaxTokens(4000),
)
```

## Full Pipeline

```go
// Configure the pipeline
pipeline := rag.NewPipeline(
    // Retrieval configuration
    rag.WithRetriever(hybridRetriever),

    // Reranking configuration
    rag.WithReranker(crossEncoderReranker),

    // Synthesis configuration
    rag.WithSynthesizer(refineSynthesizer),

    // LLM configuration
    rag.WithLLM(llm),

    // Other options
    rag.WithTopK(10),
    rag.WithMaxContextLength(4000),
)

// Execute query
response, err := pipeline.Query(ctx, "Your question")
```

## Indexing Pipeline

Index documents in one pass:

```go
indexer := rag.NewIndexer(
    rag.WithLoader(loader),
    rag.WithSplitter(splitter),
    rag.WithEmbedder(embedder),
    rag.WithStore(store),
    rag.WithBatchSize(100),
)

// Index documents
err := indexer.Index(ctx)
```

## Monitoring Metrics

```go
import "github.com/hexagon-codes/hexagon/observe/metrics"

collector := metrics.GetHexagonMetrics()

// Retrieval metrics are recorded automatically
// You can also record them manually
collector.RecordRetrieval(ctx, "vector_search", docCount, duration)

// View statistics
stats := collector.GetRetrievalStats()
fmt.Printf("Average retrieval time: %v\n", stats.AverageDuration)
fmt.Printf("Average document count: %.2f\n", stats.AverageDocCount)
```

## Best Practices

1. **Reasonable chunk size**: 500–1500 characters typically works well
2. **Appropriate overlap**: 10–20% overlap prevents information loss
3. **Multi-stage retrieval**: Coarse filtering followed by fine ranking
4. **Cache embeddings**: Avoid recomputing vectors redundantly
5. **Monitor recall**: Regularly evaluate retrieval quality
6. **Incremental updates**: Support adding, deleting, and modifying documents
