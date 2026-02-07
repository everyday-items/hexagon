// Package graph 提供 Hexagon AI Agent 框架的图编排引擎
//
// visualize.go 实现图结构的可视化导出功能，支持：
//   - Mermaid: 适用于 Markdown 文档和在线渲染
//   - DOT (Graphviz): 适用于生成高质量图片
//   - ASCII: 适用于终端输出
package graph

import (
	"fmt"
	"strings"
)

// ExportFormat 导出格式
type ExportFormat int

const (
	// FormatMermaid Mermaid 格式（适合嵌入 Markdown）
	FormatMermaid ExportFormat = iota

	// FormatDOT Graphviz DOT 格式（适合生成图片）
	FormatDOT

	// FormatASCII ASCII 文本格式（适合终端输出）
	FormatASCII
)

// ExportOption 导出选项
type ExportOption func(*exportConfig)

type exportConfig struct {
	// title 图标题
	title string

	// direction 方向：TB(上下)、LR(左右)
	direction string

	// showConditions 是否显示条件边标签
	showConditions bool

	// highlightNodes 高亮的节点列表
	highlightNodes map[string]bool

	// nodeStyles 节点自定义样式
	nodeStyles map[string]string
}

// WithExportTitle 设置导出标题
func WithExportTitle(title string) ExportOption {
	return func(c *exportConfig) {
		c.title = title
	}
}

// WithDirection 设置图方向（TB=上下, LR=左右）
func WithDirection(dir string) ExportOption {
	return func(c *exportConfig) {
		c.direction = dir
	}
}

// WithShowConditions 是否显示条件边标签
func WithShowConditions(show bool) ExportOption {
	return func(c *exportConfig) {
		c.showConditions = show
	}
}

// WithHighlightNodes 设置高亮节点
func WithHighlightNodes(nodes ...string) ExportOption {
	return func(c *exportConfig) {
		for _, n := range nodes {
			c.highlightNodes[n] = true
		}
	}
}

// Export 将图导出为指定格式的字符串
//
// 支持 Mermaid、DOT、ASCII 三种格式：
//
//	mermaid := graph.Export(FormatMermaid)
//	dot := graph.Export(FormatDOT, WithExportTitle("我的工作流"))
func (g *Graph[S]) Export(format ExportFormat, opts ...ExportOption) string {
	cfg := &exportConfig{
		direction:      "TB",
		showConditions: true,
		highlightNodes: make(map[string]bool),
		nodeStyles:     make(map[string]string),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	switch format {
	case FormatMermaid:
		return g.exportMermaid(cfg)
	case FormatDOT:
		return g.exportDOT(cfg)
	case FormatASCII:
		return g.exportASCII(cfg)
	default:
		return g.exportMermaid(cfg)
	}
}

// exportMermaid 导出 Mermaid 格式
func (g *Graph[S]) exportMermaid(cfg *exportConfig) string {
	var b strings.Builder

	// 图声明
	b.WriteString(fmt.Sprintf("graph %s\n", cfg.direction))

	// 节点定义
	for name, node := range g.Nodes {
		label := name
		if node.Name != "" {
			label = node.Name
		}

		shape := mermaidNodeShape(name, cfg)
		b.WriteString(fmt.Sprintf("    %s%s\n", sanitizeMermaidID(name), shape(label)))
	}

	// 添加 START 和 END 虚拟节点
	if g.EntryPoint != "" {
		b.WriteString(fmt.Sprintf("    %s((%s))\n", sanitizeMermaidID(START), "开始"))
		b.WriteString(fmt.Sprintf("    %s((%s))\n", sanitizeMermaidID(END), "结束"))
	}

	b.WriteString("\n")

	// 普通边
	for _, edge := range g.Edges {
		from := sanitizeMermaidID(edge.From)
		to := sanitizeMermaidID(edge.To)
		b.WriteString(fmt.Sprintf("    %s --> %s\n", from, to))
	}

	// 条件边
	if cfg.showConditions {
		for from, conds := range g.conditionalEdges {
			fromID := sanitizeMermaidID(from)
			for _, cond := range conds {
				for label, target := range cond.edges {
					toID := sanitizeMermaidID(target)
					b.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", fromID, label, toID))
				}
			}
		}
	}

	// 高亮样式
	for name := range cfg.highlightNodes {
		id := sanitizeMermaidID(name)
		b.WriteString(fmt.Sprintf("    style %s fill:#f96,stroke:#333,stroke-width:2px\n", id))
	}

	return b.String()
}

// exportDOT 导出 Graphviz DOT 格式
func (g *Graph[S]) exportDOT(cfg *exportConfig) string {
	var b strings.Builder

	title := cfg.title
	if title == "" {
		title = g.Name
	}

	b.WriteString(fmt.Sprintf("digraph %q {\n", title))
	b.WriteString("    rankdir=" + cfg.direction + ";\n")
	b.WriteString("    node [shape=box, style=rounded, fontname=\"sans-serif\"];\n")
	b.WriteString("    edge [fontname=\"sans-serif\", fontsize=10];\n\n")

	// START/END 节点
	b.WriteString("    __START__ [shape=circle, label=\"\", width=0.3, style=filled, fillcolor=black];\n")
	b.WriteString("    __END__ [shape=doublecircle, label=\"\", width=0.3, style=filled, fillcolor=black];\n\n")

	// 普通节点
	for name, node := range g.Nodes {
		label := name
		if node.Name != "" {
			label = node.Name
		}

		attrs := fmt.Sprintf("label=%q", label)
		if cfg.highlightNodes[name] {
			attrs += ", style=\"rounded,filled\", fillcolor=\"#ffcccc\""
		}
		b.WriteString(fmt.Sprintf("    %q [%s];\n", name, attrs))
	}

	b.WriteString("\n")

	// 普通边
	for _, edge := range g.Edges {
		b.WriteString(fmt.Sprintf("    %q -> %q;\n", edge.From, edge.To))
	}

	// 条件边
	if cfg.showConditions {
		for from, conds := range g.conditionalEdges {
			for _, cond := range conds {
				for label, target := range cond.edges {
					b.WriteString(fmt.Sprintf("    %q -> %q [label=%q, style=dashed];\n", from, target, label))
				}
			}
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// exportASCII 导出 ASCII 格式
func (g *Graph[S]) exportASCII(cfg *exportConfig) string {
	var b strings.Builder

	title := cfg.title
	if title == "" {
		title = g.Name
	}

	b.WriteString(fmt.Sprintf("=== %s ===\n\n", title))

	// 列出节点
	b.WriteString("节点:\n")
	for name, node := range g.Nodes {
		marker := "  "
		if cfg.highlightNodes[name] {
			marker = "* "
		}
		desc := ""
		if node.Name != "" && node.Name != name {
			desc = fmt.Sprintf(" (%s)", node.Name)
		}
		_ = node // 避免未使用警告
		b.WriteString(fmt.Sprintf("  %s[%s]%s\n", marker, name, desc))
	}

	b.WriteString("\n边:\n")

	// 普通边
	for _, edge := range g.Edges {
		b.WriteString(fmt.Sprintf("  %s --> %s\n", edge.From, edge.To))
	}

	// 条件边
	for from, conds := range g.conditionalEdges {
		for _, cond := range conds {
			for label, target := range cond.edges {
				b.WriteString(fmt.Sprintf("  %s --%s--> %s\n", from, label, target))
			}
		}
	}

	return b.String()
}

// sanitizeMermaidID 将节点名转换为合法的 Mermaid ID
func sanitizeMermaidID(name string) string {
	// 替换特殊字符
	r := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
		":", "_",
	)
	id := r.Replace(name)
	if id == START {
		return "__START__"
	}
	if id == END {
		return "__END__"
	}
	return id
}

// mermaidNodeShape 返回 Mermaid 节点形状格式化函数
func mermaidNodeShape(name string, cfg *exportConfig) func(string) string {
	if cfg.highlightNodes[name] {
		// 高亮节点使用六边形
		return func(label string) string {
			return fmt.Sprintf("{{%s}}", label)
		}
	}
	// 默认使用圆角矩形
	return func(label string) string {
		return fmt.Sprintf("(%s)", label)
	}
}
