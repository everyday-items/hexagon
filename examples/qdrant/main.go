// Package main demonstrates Qdrant vector store usage with Hexagon framework.
//
// 运行前需要先启动 Qdrant:
//
//	docker run -p 6333:6333 qdrant/qdrant
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/everyday-items/hexagon"
)

func main() {
	ctx := context.Background()

	// 1. 创建 Qdrant 向量存储
	fmt.Println("正在连接 Qdrant...")
	store, err := hexagon.NewQdrantStore(hexagon.QdrantConfig{
		Host:             "localhost",
		Port:             6333,
		Collection:       "hexagon_demo",
		Dimension:        384,
		Distance:         hexagon.QdrantDistanceCosine,
		CreateCollection: true,
		Timeout:          10 * time.Second,
	})
	if err != nil {
		log.Fatalf("无法连接 Qdrant: %v\n请确保 Qdrant 正在运行: docker run -p 6333:6333 qdrant/qdrant", err)
	}
	defer store.Close()
	fmt.Println("已连接 Qdrant")

	// 2. 创建模拟 Embedder（生产环境使用 OpenAI Embedder）
	embed := hexagon.NewMockEmbedder(384)

	// 3. 创建 RAG 引擎
	engine := hexagon.NewRAGEngine(
		hexagon.WithRAGStore(store),
		hexagon.WithRAGEmbedder(embed),
		hexagon.WithRAGTopK(3),
	)

	// 4. 清空旧数据
	fmt.Println("清空旧数据...")
	if err := engine.Clear(ctx); err != nil {
		log.Fatalf("清空失败: %v", err)
	}

	// 5. 准备文档
	docs := []hexagon.Document{
		{
			ID:      "doc1",
			Content: "Go 是一种静态类型、编译型语言，由 Google 开发。它具有简洁的语法和强大的并发支持。",
			Metadata: map[string]any{
				"source":   "go-intro",
				"category": "programming",
				"language": "chinese",
			},
		},
		{
			ID:      "doc2",
			Content: "Hexagon 是一个新一代 Go AI Agent 框架，支持 ReAct、图编排、多 Agent 协作等功能。",
			Metadata: map[string]any{
				"source":   "hexagon-intro",
				"category": "framework",
				"language": "chinese",
			},
		},
		{
			ID:      "doc3",
			Content: "Qdrant 是一个高性能的开源向量数据库，支持向量相似度搜索、元数据过滤和分布式部署。",
			Metadata: map[string]any{
				"source":   "qdrant-intro",
				"category": "database",
				"language": "chinese",
			},
		},
		{
			ID:      "doc4",
			Content: "RAG（检索增强生成）是一种结合信息检索和文本生成的技术，可以让 AI 基于外部知识回答问题。",
			Metadata: map[string]any{
				"source":   "rag-intro",
				"category": "technique",
				"language": "chinese",
			},
		},
		{
			ID:      "doc5",
			Content: "Python is a high-level programming language known for its simplicity and readability.",
			Metadata: map[string]any{
				"source":   "python-intro",
				"category": "programming",
				"language": "english",
			},
		},
	}

	// 6. 索引文档
	fmt.Println("正在索引文档...")
	if err := engine.Index(ctx, docs); err != nil {
		log.Fatalf("索引失败: %v", err)
	}

	// 等待索引完成
	time.Sleep(500 * time.Millisecond)

	count, _ := engine.Count(ctx)
	fmt.Printf("已索引 %d 个文档\n\n", count)

	// 7. 基本检索
	fmt.Println("=== 基本检索 ===")
	queries := []string{
		"什么是 Qdrant？",
		"Go 语言有什么特点？",
		"什么是 RAG 技术？",
	}

	for _, query := range queries {
		fmt.Printf("\n查询: %s\n", query)
		fmt.Println(strings.Repeat("-", 50))

		results, err := engine.Retrieve(ctx, query)
		if err != nil {
			log.Printf("检索失败: %v", err)
			continue
		}

		for i, doc := range results {
			fmt.Printf("[%d] (分数: %.4f) %s\n", i+1, doc.Score, truncate(doc.Content, 50))
		}
	}

	// 8. 带过滤的检索
	fmt.Println("\n\n=== 带过滤的检索 ===")
	fmt.Println("过滤条件: category=programming")
	fmt.Println(strings.Repeat("-", 50))

	results, err := engine.Retrieve(ctx, "编程语言",
		hexagon.WithFilter(map[string]any{"category": "programming"}),
	)
	if err != nil {
		log.Printf("检索失败: %v", err)
	} else {
		for i, doc := range results {
			fmt.Printf("[%d] (分数: %.4f) %s\n", i+1, doc.Score, truncate(doc.Content, 50))
			fmt.Printf("    元数据: %v\n", doc.Metadata)
		}
	}

	// 9. 获取单个文档
	fmt.Println("\n\n=== 获取单个文档 ===")
	doc, err := store.Get(ctx, "doc2")
	if err != nil {
		log.Printf("获取失败: %v", err)
	} else if doc != nil {
		fmt.Printf("ID: %s\n", doc.ID)
		fmt.Printf("内容: %s\n", doc.Content)
		fmt.Printf("元数据: %v\n", doc.Metadata)
	}

	// 10. 删除文档
	fmt.Println("\n\n=== 删除文档 ===")
	if err := store.Delete(ctx, []string{"doc5"}); err != nil {
		log.Printf("删除失败: %v", err)
	} else {
		fmt.Println("已删除 doc5")
	}

	// 等待删除完成
	time.Sleep(500 * time.Millisecond)

	count, _ = engine.Count(ctx)
	fmt.Printf("剩余文档数: %d\n", count)

	fmt.Println("\n完成！")
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
