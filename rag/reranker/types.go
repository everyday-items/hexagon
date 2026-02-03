// Package reranker 提供文档重排序功能的类型定义
package reranker

import (
	"github.com/everyday-items/hexagon/rag"
)

// RankedDocument 带排名信息的文档
//
// 扩展了 rag.Document，添加了重排序相关的元信息：
//   - RelevanceScore: 与查询的相关性得分
//   - OriginalRank: 原始排名（重排序前）
//   - NewRank: 新排名（重排序后）
//
// 使用场景：
//   - 追踪文档在重排序过程中的排名变化
//   - 调试和分析重排序效果
//   - 评估重排序算法的性能
type RankedDocument struct {
	// 嵌入原始文档
	rag.Document

	// RelevanceScore 与查询的相关性得分
	// 由重排序器计算，范围通常为 [0, 1]
	RelevanceScore float32

	// OriginalRank 原始排名（从 1 开始）
	// 表示文档在重排序前的位置
	OriginalRank int

	// NewRank 新排名（从 1 开始）
	// 表示文档在重排序后的位置
	NewRank int
}

// ToDocument 将 RankedDocument 转换为普通 Document
//
// 注意：转换后会丢失排名信息
func (rd RankedDocument) ToDocument() rag.Document {
	doc := rd.Document
	doc.Score = rd.RelevanceScore
	return doc
}

// RankedDocuments 是 RankedDocument 的切片类型
type RankedDocuments []RankedDocument

// ToDocuments 批量转换为普通文档列表
func (rds RankedDocuments) ToDocuments() []rag.Document {
	docs := make([]rag.Document, len(rds))
	for i, rd := range rds {
		docs[i] = rd.ToDocument()
	}
	return docs
}

// FromDocuments 从普通文档列表创建 RankedDocuments
//
// 使用原始索引作为 OriginalRank，使用文档 Score 作为 RelevanceScore
func FromDocuments(docs []rag.Document) RankedDocuments {
	rds := make(RankedDocuments, len(docs))
	for i, doc := range docs {
		rds[i] = RankedDocument{
			Document:       doc,
			RelevanceScore: doc.Score,
			OriginalRank:   i + 1, // 排名从 1 开始
			NewRank:        i + 1,
		}
	}
	return rds
}
