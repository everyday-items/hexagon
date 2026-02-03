// Package milvus 提供 Milvus 向量数据库集成
package milvus

import (
	"github.com/everyday-items/hexagon/internal/util"
)

// generateID 生成唯一 ID
//
// 使用 toolkit 提供的 NanoID 实现，确保一致性和安全性。
func generateID() string {
	return util.GenerateID("milvus")
}
