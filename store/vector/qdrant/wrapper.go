// Package qdrant 提供 Qdrant 向量数据库适配器
//
// 本包重新导出 ai-core/store/vector/qdrant 的实现，保持向后兼容性。
//
// 使用示例:
//
//	store, err := qdrant.New(qdrant.Config{
//	    Host:       "localhost",
//	    Port:       6333,
//	    Collection: "documents",
//	    Dimension:  1536,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
package qdrant

import (
	aicoreQdrant "github.com/everyday-items/ai-core/store/vector/qdrant"
)

// 重新导出类型
type (
	// Store Qdrant 向量存储
	Store = aicoreQdrant.Store

	// Config Qdrant 配置
	Config = aicoreQdrant.Config

	// Distance 距离度量方式
	Distance = aicoreQdrant.Distance

	// Option Qdrant 存储选项
	Option = aicoreQdrant.Option

	// BatchConfig 批量操作配置
	BatchConfig = aicoreQdrant.BatchConfig

	// BatchOption 批量操作选项
	BatchOption = aicoreQdrant.BatchOption
)

// 重新导出常量
const (
	// DistanceCosine 余弦距离
	DistanceCosine = aicoreQdrant.DistanceCosine

	// DistanceEuclid 欧几里得距离
	DistanceEuclid = aicoreQdrant.DistanceEuclid

	// DistanceDot 点积
	DistanceDot = aicoreQdrant.DistanceDot
)

// 重新导出函数
var (
	// New 创建 Qdrant 存储
	New = aicoreQdrant.New

	// NewWithOptions 使用选项创建 Qdrant 存储
	NewWithOptions = aicoreQdrant.NewWithOptions

	// WithHost 设置服务器地址
	WithHost = aicoreQdrant.WithHost

	// WithPort 设置服务器端口
	WithPort = aicoreQdrant.WithPort

	// WithCollection 设置集合名称
	WithCollection = aicoreQdrant.WithCollection

	// WithDimension 设置向量维度
	WithDimension = aicoreQdrant.WithDimension

	// WithAPIKey 设置 API 密钥
	WithAPIKey = aicoreQdrant.WithAPIKey

	// WithHTTPS 设置是否使用 HTTPS
	WithHTTPS = aicoreQdrant.WithHTTPS

	// WithTimeout 设置请求超时时间
	WithTimeout = aicoreQdrant.WithTimeout

	// WithDistance 设置距离度量方式
	WithDistance = aicoreQdrant.WithDistance

	// WithOnDisk 设置是否将向量存储在磁盘上
	WithOnDisk = aicoreQdrant.WithOnDisk

	// WithCreateCollection 设置是否自动创建集合
	WithCreateCollection = aicoreQdrant.WithCreateCollection

	// WithBatchSize 设置批量大小
	WithBatchSize = aicoreQdrant.WithBatchSize

	// WithConcurrency 设置并发数
	WithConcurrency = aicoreQdrant.WithConcurrency

	// WithRetry 设置重试次数和延迟
	WithRetry = aicoreQdrant.WithRetry

	// WithOnProgress 设置进度回调
	WithOnProgress = aicoreQdrant.WithOnProgress

	// WithOnError 设置错误回调
	WithOnError = aicoreQdrant.WithOnError
)

// 重新导出变量
var (
	// DefaultBatchConfig 默认批量配置
	DefaultBatchConfig = aicoreQdrant.DefaultBatchConfig
)
