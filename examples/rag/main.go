// Package main demonstrates basic RAG usage with Hexagon framework.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/everyday-items/hexagon"
)

func main() {
	ctx := context.Background()

	// 1. 创建内存向量存储
	store := hexagon.NewMemoryVectorStore(384) // 384 维向量

	// 2. 创建模拟 Embedder（生产环境使用 OpenAI Embedder）
	embed := hexagon.NewMockEmbedder(384)

	// 3. 创建 RAG 引擎
	engine := hexagon.NewRAGEngine(
		hexagon.WithRAGStore(store),
		hexagon.WithRAGEmbedder(embed),
		hexagon.WithRAGTopK(3),
	)

	// 4. 准备文档
	docs := []hexagon.Document{
		{
			ID:      "doc1",
			Content: "Go 是一种静态类型、编译型语言，由 Google 开发。它具有简洁的语法和强大的并发支持。",
			Metadata: map[string]any{
				"source": "go-intro",
				"type":   "tutorial",
			},
		},
		{
			ID:      "doc2",
			Content: "Hexagon 是一个新一代 Go AI Agent 框架，支持 ReAct、图编排、多 Agent 协作等功能。",
			Metadata: map[string]any{
				"source": "hexagon-intro",
				"type":   "documentation",
			},
		},
		{
			ID:      "doc3",
			Content: "RAG（检索增强生成）是一种结合信息检索和文本生成的技术，可以让 AI 基于外部知识回答问题。",
			Metadata: map[string]any{
				"source": "rag-intro",
				"type":   "tutorial",
			},
		},
		{
			ID:      "doc4",
			Content: "向量数据库用于存储和检索高维向量，常用于语义搜索和推荐系统。常见的向量数据库有 Qdrant、Milvus、Pinecone 等。",
			Metadata: map[string]any{
				"source": "vector-db",
				"type":   "tutorial",
			},
		},
	}

	// 5. 索引文档
	fmt.Println("正在索引文档...")
	if err := engine.Index(ctx, docs); err != nil {
		log.Fatalf("索引失败: %v", err)
	}
	fmt.Printf("已索引 %d 个文档\n\n", len(docs))

	// 6. 检索相关文档
	queries := []string{
		"什么是 Hexagon 框架？",
		"Go 语言有什么特点？",
		"RAG 是什么技术？",
	}

	for _, query := range queries {
		fmt.Printf("查询: %s\n", query)
		fmt.Println(strings.Repeat("-", 50))

		results, err := engine.Retrieve(ctx, query)
		if err != nil {
			log.Printf("检索失败: %v", err)
			continue
		}

		for i, doc := range results {
			fmt.Printf("[%d] (分数: %.4f) %s\n", i+1, doc.Score, truncate(doc.Content, 60))
		}
		fmt.Println()
	}

	// 7. 使用 Query 获取格式化上下文
	fmt.Println("=== 格式化上下文示例 ===")
	context, err := engine.Query(ctx, "什么是向量数据库？")
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	fmt.Println(context)
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
