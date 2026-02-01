// Package evaluate 提供 AI Agent 系统的评估框架
package evaluate

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
)

// Reporter 报告生成器接口
type Reporter interface {
	// Generate 生成报告
	Generate(report *EvalReport, w io.Writer) error

	// Format 返回报告格式
	Format() string
}

// ============== JSONReporter ==============

// JSONReporter JSON 格式报告生成器
type JSONReporter struct {
	pretty bool
}

// NewJSONReporter 创建 JSON 报告生成器
func NewJSONReporter(pretty bool) *JSONReporter {
	return &JSONReporter{pretty: pretty}
}

// Generate 生成 JSON 报告
func (r *JSONReporter) Generate(report *EvalReport, w io.Writer) error {
	var data []byte
	var err error

	if r.pretty {
		data, err = json.MarshalIndent(report, "", "  ")
	} else {
		data, err = json.Marshal(report)
	}

	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// Format 返回报告格式
func (r *JSONReporter) Format() string {
	return "json"
}

var _ Reporter = (*JSONReporter)(nil)

// ============== MarkdownReporter ==============

// MarkdownReporter Markdown 格式报告生成器
type MarkdownReporter struct {
	includeDetails bool
}

// NewMarkdownReporter 创建 Markdown 报告生成器
func NewMarkdownReporter(includeDetails bool) *MarkdownReporter {
	return &MarkdownReporter{includeDetails: includeDetails}
}

// Generate 生成 Markdown 报告
func (r *MarkdownReporter) Generate(report *EvalReport, w io.Writer) error {
	var sb strings.Builder

	// 标题
	sb.WriteString(fmt.Sprintf("# %s\n\n", report.Name))

	// 概要
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Dataset**: %s\n", report.Dataset))
	sb.WriteString(fmt.Sprintf("- **Start Time**: %s\n", report.StartTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- **Duration**: %s\n", report.Duration))
	sb.WriteString(fmt.Sprintf("- **Total Samples**: %d\n", report.TotalSamples))
	sb.WriteString(fmt.Sprintf("- **Success**: %d\n", report.SuccessSamples))
	sb.WriteString(fmt.Sprintf("- **Failed**: %d\n", report.FailedSamples))
	sb.WriteString("\n")

	// 指标汇总表格
	sb.WriteString("## Metrics Summary\n\n")
	sb.WriteString("| Metric | Mean | Min | Max | Pass Rate |\n")
	sb.WriteString("|--------|------|-----|-----|----------|\n")

	// 排序指标名称
	metricNames := make([]string, 0, len(report.Summary))
	for name := range report.Summary {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)

	for _, name := range metricNames {
		summary := report.Summary[name]
		passRate := "N/A"
		if summary.PassRate != nil {
			passRate = fmt.Sprintf("%.1f%%", *summary.PassRate*100)
		}
		sb.WriteString(fmt.Sprintf("| %s | %.3f | %.3f | %.3f | %s |\n",
			name, summary.Mean, summary.Min, summary.Max, passRate))
	}
	sb.WriteString("\n")

	// 分数分布
	sb.WriteString("## Score Distribution\n\n")
	for _, name := range metricNames {
		summary := report.Summary[name]
		if len(summary.Distribution) > 0 {
			sb.WriteString(fmt.Sprintf("### %s\n\n", name))
			sb.WriteString("| Level | Count |\n")
			sb.WriteString("|-------|-------|\n")

			levels := []string{"excellent", "good", "fair", "poor", "bad"}
			for _, level := range levels {
				count := summary.Distribution[level]
				sb.WriteString(fmt.Sprintf("| %s | %d |\n", level, count))
			}
			sb.WriteString("\n")
		}
	}

	// 详细结果
	if r.includeDetails && len(report.Results) > 0 {
		sb.WriteString("## Detailed Results\n\n")

		for i, result := range report.Results {
			sb.WriteString(fmt.Sprintf("### Sample %d: %s\n\n", i+1, result.SampleID))
			sb.WriteString(fmt.Sprintf("**Query**: %s\n\n", truncate(result.Query, 200)))

			if result.Response != "" {
				sb.WriteString(fmt.Sprintf("**Response**: %s\n\n", truncate(result.Response, 500)))
			}

			if result.Error != "" {
				sb.WriteString(fmt.Sprintf("**Error**: %s\n\n", result.Error))
			}

			sb.WriteString("| Metric | Score | Passed | Reason |\n")
			sb.WriteString("|--------|-------|--------|--------|\n")

			for metricName, evalResult := range result.Results {
				passed := "N/A"
				if evalResult.Passed != nil {
					if *evalResult.Passed {
						passed = "✓"
					} else {
						passed = "✗"
					}
				}
				reason := truncate(evalResult.Reason, 100)
				sb.WriteString(fmt.Sprintf("| %s | %.3f | %s | %s |\n",
					metricName, evalResult.Score, passed, reason))
			}
			sb.WriteString("\n---\n\n")
		}
	}

	_, err := w.Write([]byte(sb.String()))
	return err
}

// Format 返回报告格式
func (r *MarkdownReporter) Format() string {
	return "markdown"
}

var _ Reporter = (*MarkdownReporter)(nil)

// ============== HTMLReporter ==============

// HTMLReporter HTML 格式报告生成器
type HTMLReporter struct {
	includeDetails bool
}

// NewHTMLReporter 创建 HTML 报告生成器
func NewHTMLReporter(includeDetails bool) *HTMLReporter {
	return &HTMLReporter{includeDetails: includeDetails}
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Name}}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 20px; }
        h1, h2, h3 { color: #333; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #f5f5f5; }
        tr:nth-child(even) { background-color: #fafafa; }
        .summary-box { display: flex; gap: 20px; flex-wrap: wrap; }
        .summary-item { background: #f0f0f0; padding: 15px; border-radius: 8px; min-width: 150px; }
        .summary-item .value { font-size: 24px; font-weight: bold; color: #2563eb; }
        .summary-item .label { color: #666; font-size: 14px; }
        .pass { color: #22c55e; }
        .fail { color: #ef4444; }
        .score-excellent { background-color: #dcfce7; }
        .score-good { background-color: #fef9c3; }
        .score-fair { background-color: #fed7aa; }
        .score-poor { background-color: #fecaca; }
        .score-bad { background-color: #fee2e2; }
        .chart-container { margin: 20px 0; }
    </style>
</head>
<body>
    <h1>{{.Name}}</h1>

    <h2>Summary</h2>
    <div class="summary-box">
        <div class="summary-item">
            <div class="value">{{.TotalSamples}}</div>
            <div class="label">Total Samples</div>
        </div>
        <div class="summary-item">
            <div class="value pass">{{.SuccessSamples}}</div>
            <div class="label">Success</div>
        </div>
        <div class="summary-item">
            <div class="value fail">{{.FailedSamples}}</div>
            <div class="label">Failed</div>
        </div>
        <div class="summary-item">
            <div class="value">{{.Duration}}</div>
            <div class="label">Duration</div>
        </div>
    </div>

    <h2>Metrics Summary</h2>
    <table>
        <thead>
            <tr>
                <th>Metric</th>
                <th>Mean</th>
                <th>Min</th>
                <th>Max</th>
                <th>Count</th>
            </tr>
        </thead>
        <tbody>
            {{range $name, $summary := .Summary}}
            <tr>
                <td>{{$name}}</td>
                <td>{{printf "%.3f" $summary.Mean}}</td>
                <td>{{printf "%.3f" $summary.Min}}</td>
                <td>{{printf "%.3f" $summary.Max}}</td>
                <td>{{$summary.Count}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

    {{if .Results}}
    <h2>Detailed Results</h2>
    {{range $i, $result := .Results}}
    <div style="margin-bottom: 30px; padding: 15px; border: 1px solid #ddd; border-radius: 8px;">
        <h3>Sample {{$i}}: {{$result.SampleID}}</h3>
        <p><strong>Query:</strong> {{$result.Query}}</p>
        {{if $result.Response}}
        <p><strong>Response:</strong> {{$result.Response}}</p>
        {{end}}
        {{if $result.Error}}
        <p class="fail"><strong>Error:</strong> {{$result.Error}}</p>
        {{end}}
        <table>
            <thead>
                <tr>
                    <th>Metric</th>
                    <th>Score</th>
                    <th>Reason</th>
                </tr>
            </thead>
            <tbody>
                {{range $metric, $eval := $result.Results}}
                <tr>
                    <td>{{$metric}}</td>
                    <td>{{printf "%.3f" $eval.Score}}</td>
                    <td>{{$eval.Reason}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}
    {{end}}

    <footer style="margin-top: 40px; color: #666; font-size: 12px;">
        Generated at {{.EndTime.Format "2006-01-02 15:04:05"}}
    </footer>
</body>
</html>`

// Generate 生成 HTML 报告
func (r *HTMLReporter) Generate(report *EvalReport, w io.Writer) error {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// 如果不包含详细信息，清空结果
	data := *report
	if !r.includeDetails {
		data.Results = nil
	}

	return tmpl.Execute(w, data)
}

// Format 返回报告格式
func (r *HTMLReporter) Format() string {
	return "html"
}

var _ Reporter = (*HTMLReporter)(nil)

// ============== ConsoleReporter ==============

// ConsoleReporter 控制台格式报告生成器
type ConsoleReporter struct {
	colored bool
}

// NewConsoleReporter 创建控制台报告生成器
func NewConsoleReporter(colored bool) *ConsoleReporter {
	return &ConsoleReporter{colored: colored}
}

// Generate 生成控制台报告
func (r *ConsoleReporter) Generate(report *EvalReport, w io.Writer) error {
	var sb strings.Builder

	// 标题
	sb.WriteString("\n")
	sb.WriteString(r.colorize("=== EVALUATION REPORT ===\n", "bold"))
	sb.WriteString("\n")

	// 概要
	sb.WriteString(fmt.Sprintf("Dataset:    %s\n", report.Dataset))
	sb.WriteString(fmt.Sprintf("Duration:   %s\n", report.Duration))
	sb.WriteString(fmt.Sprintf("Samples:    %d total, %s success, %s failed\n",
		report.TotalSamples,
		r.colorize(fmt.Sprintf("%d", report.SuccessSamples), "green"),
		r.colorize(fmt.Sprintf("%d", report.FailedSamples), "red")))
	sb.WriteString("\n")

	// 指标汇总
	sb.WriteString(r.colorize("--- Metrics Summary ---\n", "bold"))

	// 排序指标名称
	metricNames := make([]string, 0, len(report.Summary))
	for name := range report.Summary {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)

	for _, name := range metricNames {
		summary := report.Summary[name]
		scoreColor := r.getScoreColor(summary.Mean)
		sb.WriteString(fmt.Sprintf("  %-20s %s (min: %.3f, max: %.3f)\n",
			name+":",
			r.colorize(fmt.Sprintf("%.3f", summary.Mean), scoreColor),
			summary.Min, summary.Max))
	}

	sb.WriteString("\n")
	sb.WriteString(r.colorize("=========================\n", "bold"))

	_, err := w.Write([]byte(sb.String()))
	return err
}

func (r *ConsoleReporter) colorize(text, color string) string {
	if !r.colored {
		return text
	}

	colors := map[string]string{
		"bold":   "\033[1m",
		"red":    "\033[31m",
		"green":  "\033[32m",
		"yellow": "\033[33m",
		"blue":   "\033[34m",
		"reset":  "\033[0m",
	}

	if code, ok := colors[color]; ok {
		return code + text + colors["reset"]
	}
	return text
}

func (r *ConsoleReporter) getScoreColor(score float64) string {
	switch {
	case score >= 0.8:
		return "green"
	case score >= 0.6:
		return "yellow"
	default:
		return "red"
	}
}

// Format 返回报告格式
func (r *ConsoleReporter) Format() string {
	return "console"
}

var _ Reporter = (*ConsoleReporter)(nil)

// ============== 辅助函数 ==============

// truncate 截断文本
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// GenerateReport 生成报告的便捷函数
func GenerateReport(report *EvalReport, format string, w io.Writer) error {
	var reporter Reporter

	switch format {
	case "json":
		reporter = NewJSONReporter(true)
	case "markdown", "md":
		reporter = NewMarkdownReporter(true)
	case "html":
		reporter = NewHTMLReporter(true)
	case "console":
		reporter = NewConsoleReporter(true)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	return reporter.Generate(report, w)
}
