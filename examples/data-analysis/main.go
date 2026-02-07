// Package main 演示数据分析 Agent 场景
//
// 本示例展示如何使用 Hexagon 构建一个数据分析 Agent：
//
//   - Graph 编排: 使用图编排多步数据处理流程
//   - 工具调用: SQL 查询、数据统计、图表生成
//   - 条件路由: 根据数据特征选择分析策略
//   - 流式输出: 实时输出分析进展
//
// 运行方式:
//
//	go run ./examples/data-analysis
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/everyday-items/hexagon/orchestration/graph"
)

// AnalysisState 数据分析状态
type AnalysisState struct {
	// Query 用户的分析请求
	Query string `json:"query"`

	// DataSource 数据源
	DataSource string `json:"data_source"`

	// RawData 原始数据
	RawData []map[string]any `json:"raw_data,omitempty"`

	// Statistics 统计结果
	Statistics map[string]float64 `json:"statistics,omitempty"`

	// Insights 分析洞察
	Insights []string `json:"insights,omitempty"`

	// Report 最终报告
	Report string `json:"report,omitempty"`

	// AnalysisType 分析类型
	AnalysisType string `json:"analysis_type,omitempty"`

	// Errors 错误信息
	Errors []string `json:"errors,omitempty"`
}

// Clone 克隆状态
func (s AnalysisState) Clone() graph.State {
	clone := s
	if s.RawData != nil {
		clone.RawData = make([]map[string]any, len(s.RawData))
		copy(clone.RawData, s.RawData)
	}
	if s.Statistics != nil {
		clone.Statistics = make(map[string]float64)
		for k, v := range s.Statistics {
			clone.Statistics[k] = v
		}
	}
	clone.Insights = append([]string{}, s.Insights...)
	clone.Errors = append([]string{}, s.Errors...)
	return clone
}

func main() {
	ctx := context.Background()

	// 构建数据分析图
	g, err := graph.NewGraph[AnalysisState]("data-analysis").
		// 步骤 1: 解析查询意图
		AddNode("parse_query", parseQuery).
		// 步骤 2: 加载数据
		AddNode("load_data", loadData).
		// 步骤 3: 路由到不同分析策略
		AddNode("route", routeAnalysis).
		// 步骤 4a: 描述性统计
		AddNode("descriptive", descriptiveAnalysis).
		// 步骤 4b: 趋势分析
		AddNode("trend", trendAnalysis).
		// 步骤 5: 生成洞察
		AddNode("insights", generateInsights).
		// 步骤 6: 生成报告
		AddNode("report", generateReport).
		// 边定义
		AddEdge(graph.START, "parse_query").
		AddEdge("parse_query", "load_data").
		AddEdge("load_data", "route").
		// 条件路由
		AddConditionalEdge("route", func(s AnalysisState) string {
			switch s.AnalysisType {
			case "trend":
				return "trend"
			default:
				return "descriptive"
			}
		}, map[string]string{
			"descriptive": "descriptive",
			"trend":       "trend",
		}).
		AddEdge("descriptive", "insights").
		AddEdge("trend", "insights").
		AddEdge("insights", "report").
		AddEdge("report", graph.END).
		Build()

	if err != nil {
		log.Fatalf("构建分析图失败: %v", err)
	}

	// 执行分析
	fmt.Println("=== 数据分析 Agent 演示 ===")
	fmt.Println()

	queries := []string{
		"分析最近一周的销售数据",
		"对比各产品线的趋势变化",
	}

	for _, query := range queries {
		fmt.Printf("📊 分析请求: %s\n", query)
		fmt.Println(strings.Repeat("-", 50))

		state := AnalysisState{
			Query:      query,
			DataSource: "sales_db",
		}

		// 使用流式执行
		events, err := g.Stream(ctx, state)
		if err != nil {
			log.Printf("执行失败: %v", err)
			continue
		}

		for event := range events {
			switch event.Type {
			case graph.EventTypeNodeStart:
				fmt.Printf("  ▶ 执行: %s\n", event.NodeName)
			case graph.EventTypeNodeEnd:
				// 显示中间结果
				if event.NodeName == "load_data" {
					fmt.Printf("    ✓ 加载了 %d 条数据\n", len(event.State.RawData))
				}
				if event.NodeName == "route" {
					fmt.Printf("    ✓ 分析类型: %s\n", event.State.AnalysisType)
				}
			case graph.EventTypeEnd:
				fmt.Println("\n📋 分析报告:")
				fmt.Println(event.State.Report)
			case graph.EventTypeError:
				fmt.Printf("  ❌ 错误: %v\n", event.Error)
			}
		}
		fmt.Println()
	}

	fmt.Println("=== 演示结束 ===")
}

// ============== 节点处理函数 ==============

func parseQuery(_ context.Context, state AnalysisState) (AnalysisState, error) {
	query := strings.ToLower(state.Query)

	if strings.Contains(query, "趋势") || strings.Contains(query, "变化") || strings.Contains(query, "对比") {
		state.AnalysisType = "trend"
	} else {
		state.AnalysisType = "descriptive"
	}

	return state, nil
}

func loadData(_ context.Context, state AnalysisState) (AnalysisState, error) {
	// 模拟从数据库加载数据
	state.RawData = []map[string]any{
		{"date": "2024-01-01", "product": "A", "sales": 1200, "quantity": 45},
		{"date": "2024-01-02", "product": "A", "sales": 1350, "quantity": 52},
		{"date": "2024-01-03", "product": "B", "sales": 890, "quantity": 30},
		{"date": "2024-01-04", "product": "B", "sales": 920, "quantity": 33},
		{"date": "2024-01-05", "product": "A", "sales": 1500, "quantity": 58},
		{"date": "2024-01-06", "product": "C", "sales": 2100, "quantity": 70},
		{"date": "2024-01-07", "product": "C", "sales": 2250, "quantity": 75},
	}
	return state, nil
}

func routeAnalysis(_ context.Context, state AnalysisState) (AnalysisState, error) {
	// 路由节点不修改状态，仅用于条件路由
	return state, nil
}

func descriptiveAnalysis(_ context.Context, state AnalysisState) (AnalysisState, error) {
	state.Statistics = make(map[string]float64)

	var totalSales, totalQty float64
	minSales, maxSales := math.MaxFloat64, 0.0

	for _, row := range state.RawData {
		sales, _ := toFloat64(row["sales"])
		qty, _ := toFloat64(row["quantity"])
		totalSales += sales
		totalQty += qty
		if sales < minSales {
			minSales = sales
		}
		if sales > maxSales {
			maxSales = sales
		}
	}

	count := float64(len(state.RawData))
	state.Statistics["total_sales"] = totalSales
	state.Statistics["avg_sales"] = totalSales / count
	state.Statistics["min_sales"] = minSales
	state.Statistics["max_sales"] = maxSales
	state.Statistics["total_quantity"] = totalQty
	state.Statistics["record_count"] = count

	return state, nil
}

func trendAnalysis(_ context.Context, state AnalysisState) (AnalysisState, error) {
	state.Statistics = make(map[string]float64)

	// 按产品分组统计
	productSales := make(map[string]float64)
	for _, row := range state.RawData {
		product, _ := row["product"].(string)
		sales, _ := toFloat64(row["sales"])
		productSales[product] += sales
	}

	for product, sales := range productSales {
		state.Statistics["product_"+product+"_total"] = sales
	}

	// 计算整体趋势（简单线性）
	if len(state.RawData) >= 2 {
		first, _ := toFloat64(state.RawData[0]["sales"])
		last, _ := toFloat64(state.RawData[len(state.RawData)-1]["sales"])
		state.Statistics["trend_change_pct"] = (last - first) / first * 100
	}

	return state, nil
}

func generateInsights(_ context.Context, state AnalysisState) (AnalysisState, error) {
	state.Insights = []string{}

	if avg, ok := state.Statistics["avg_sales"]; ok {
		state.Insights = append(state.Insights, fmt.Sprintf("平均日销售额为 %.0f 元", avg))
	}

	if max, ok := state.Statistics["max_sales"]; ok {
		state.Insights = append(state.Insights, fmt.Sprintf("最高单日销售额 %.0f 元", max))
	}

	if change, ok := state.Statistics["trend_change_pct"]; ok {
		if change > 0 {
			state.Insights = append(state.Insights, fmt.Sprintf("销售呈上升趋势 (+%.1f%%)", change))
		} else {
			state.Insights = append(state.Insights, fmt.Sprintf("销售呈下降趋势 (%.1f%%)", change))
		}
	}

	// 产品对比
	for key, value := range state.Statistics {
		if strings.HasPrefix(key, "product_") && strings.HasSuffix(key, "_total") {
			product := strings.TrimSuffix(strings.TrimPrefix(key, "product_"), "_total")
			state.Insights = append(state.Insights, fmt.Sprintf("产品 %s 总销售额: %.0f 元", product, value))
		}
	}

	return state, nil
}

func generateReport(_ context.Context, state AnalysisState) (AnalysisState, error) {
	var report strings.Builder

	report.WriteString(fmt.Sprintf("📊 分析报告: %s\n", state.Query))
	report.WriteString(strings.Repeat("─", 40) + "\n")

	if len(state.Statistics) > 0 {
		report.WriteString("\n📈 统计数据:\n")
		statsJSON, _ := json.MarshalIndent(state.Statistics, "  ", "  ")
		report.WriteString("  " + string(statsJSON) + "\n")
	}

	if len(state.Insights) > 0 {
		report.WriteString("\n💡 关键洞察:\n")
		for i, insight := range state.Insights {
			report.WriteString(fmt.Sprintf("  %d. %s\n", i+1, insight))
		}
	}

	state.Report = report.String()
	return state, nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}
