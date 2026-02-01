// Package prometheus 提供 Prometheus 指标导出
//
// 本包重新导出 toolkit/infra/prometheus 的实现，保持向后兼容性。
//
// 使用示例:
//
//	exporter := prometheus.NewExporter(
//	    prometheus.WithNamespace("myapp"),
//	)
//	http.Handle("/metrics", exporter.Handler())
package prometheus

import (
	toolkitProm "github.com/everyday-items/toolkit/infra/prometheus"
)

// 重新导出类型
type (
	// Exporter Prometheus 导出器
	Exporter = toolkitProm.Exporter

	// ExporterOption 导出器选项
	ExporterOption = toolkitProm.ExporterOption

	// Registry 指标注册表
	Registry = toolkitProm.Registry

	// Collector 指标收集器
	Collector = toolkitProm.Collector

	// PrometheusCounter Prometheus Counter
	PrometheusCounter = toolkitProm.PrometheusCounter

	// PrometheusGauge Prometheus Gauge
	PrometheusGauge = toolkitProm.PrometheusGauge

	// PrometheusHistogram Prometheus Histogram
	PrometheusHistogram = toolkitProm.PrometheusHistogram

	// PrometheusSummary Prometheus Summary
	PrometheusSummary = toolkitProm.PrometheusSummary

	// MetricsAdapter 指标适配器
	MetricsAdapter = toolkitProm.MetricsAdapter
)

// 重新导出函数
var (
	// NewExporter 创建 Prometheus 导出器
	NewExporter = toolkitProm.NewExporter

	// WithNamespace 设置命名空间
	WithNamespace = toolkitProm.WithNamespace

	// WithSubsystem 设置子系统
	WithSubsystem = toolkitProm.WithSubsystem

	// NewRegistry 创建注册表
	NewRegistry = toolkitProm.NewRegistry

	// NewCollector 创建收集器
	NewCollector = toolkitProm.NewCollector

	// NewMetricsAdapter 创建指标适配器
	NewMetricsAdapter = toolkitProm.NewMetricsAdapter
)

// 重新导出变量
var (
	// DefaultBuckets 默认桶
	DefaultBuckets = toolkitProm.DefaultBuckets

	// DefaultQuantiles 默认分位数
	DefaultQuantiles = toolkitProm.DefaultQuantiles
)
