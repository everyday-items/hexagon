// Package reranker 提供辅助函数
package reranker

import (
	"sort"

	"github.com/everyday-items/hexagon/rag"
)

// truncateText 截断文本到指定长度
//
// 如果文本长度超过 maxLen，则截断并添加省略号
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

// truncate 是 truncateText 的别名，用于测试兼容
//
// 如果文本长度超过 maxLen，则截断并添加省略号
func truncate(text string, maxLen int) string {
	return truncateText(text, maxLen)
}

// min 返回两个整数中较小的一个
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 返回两个整数中较大的一个
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// localFallbackByScore 本地降级策略：按原有分数排序
//
// 当远程服务不可用时，使用此函数作为降级策略。
// 复制文档列表以避免修改原切片，然后按分数降序排序并返回 TopK。
//
// 参数：
//   - docs: 原始文档列表
//   - topK: 返回的最大文档数
//
// 返回：
//   - 按分数降序排列的文档列表（最多 topK 个）
//   - 错误信息（始终为 nil）
func localFallbackByScore(docs []rag.Document, topK int) ([]rag.Document, error) {
	// 复制文档避免修改原切片
	result := make([]rag.Document, len(docs))
	copy(result, docs)

	// 按现有分数降序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	// 取 TopK
	k := min(topK, len(result))
	return result[:k], nil
}
