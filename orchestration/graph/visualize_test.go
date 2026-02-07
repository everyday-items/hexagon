package graph

import (
	"context"
	"strings"
	"testing"
)

// buildSimpleGraph 构建一个简单的线性图: START → A → B → END
// 用于可视化导出测试的通用辅助函数
func buildSimpleGraph(t *testing.T) *Graph[TestState] {
	t.Helper()

	g, err := NewGraph[TestState]("test-viz").
		AddNode("A", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddNode("B", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "A").
		AddEdge("A", "B").
		AddEdge("B", END).
		Build()

	if err != nil {
		t.Fatalf("构建简单图失败: %v", err)
	}
	return g
}

// buildConditionalGraph 构建一个带条件边的图: START → check → (path_a | path_b) → END
// 用于测试条件边在各种导出格式中的渲染
func buildConditionalGraph(t *testing.T) *Graph[TestState] {
	t.Helper()

	g, err := NewGraph[TestState]("conditional-viz").
		AddNode("check", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("path_a", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "A"
			return s, nil
		}).
		AddNode("path_b", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "B"
			return s, nil
		}).
		AddEdge(START, "check").
		AddConditionalEdge("check", func(s TestState) string {
			if s.Counter > 0 {
				return "yes"
			}
			return "no"
		}, map[string]string{
			"yes": "path_a",
			"no":  "path_b",
		}).
		AddEdge("path_a", END).
		AddEdge("path_b", END).
		Build()

	if err != nil {
		t.Fatalf("构建条件图失败: %v", err)
	}
	return g
}

// TestExport_Mermaid 测试 Mermaid 格式导出
// 验证输出包含正确的图声明、节点和边
func TestExport_Mermaid(t *testing.T) {
	g := buildSimpleGraph(t)
	output := g.Export(FormatMermaid)

	// 验证图声明（默认方向为 TB）
	if !strings.Contains(output, "graph TB") {
		t.Errorf("Mermaid 输出应包含 'graph TB' 图声明，实际输出:\n%s", output)
	}

	// 验证节点 A 和 B 存在
	if !strings.Contains(output, "A") {
		t.Errorf("Mermaid 输出应包含节点 A，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "B") {
		t.Errorf("Mermaid 输出应包含节点 B，实际输出:\n%s", output)
	}

	// 验证 START 和 END 虚拟节点
	if !strings.Contains(output, "__START__") {
		t.Errorf("Mermaid 输出应包含 __START__ 虚拟节点，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "__END__") {
		t.Errorf("Mermaid 输出应包含 __END__ 虚拟节点，实际输出:\n%s", output)
	}

	// 验证边连接（普通边使用 --> 语法）
	if !strings.Contains(output, "-->") {
		t.Errorf("Mermaid 输出应包含 '-->' 边连接，实际输出:\n%s", output)
	}

	// 验证 START → A 和 A → B 和 B → END 的边
	if !strings.Contains(output, "__START__ --> A") {
		t.Errorf("Mermaid 输出应包含 '__START__ --> A' 边，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "A --> B") {
		t.Errorf("Mermaid 输出应包含 'A --> B' 边，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "B --> __END__") {
		t.Errorf("Mermaid 输出应包含 'B --> __END__' 边，实际输出:\n%s", output)
	}
}

// TestExport_Mermaid_Conditional 测试 Mermaid 格式导出条件边
// 验证条件边使用 -->|label| 语法渲染
func TestExport_Mermaid_Conditional(t *testing.T) {
	g := buildConditionalGraph(t)
	output := g.Export(FormatMermaid, WithShowConditions(true))

	// 验证条件边标签语法 -->|label|
	if !strings.Contains(output, "-->|") {
		t.Errorf("Mermaid 条件边输出应包含 '-->|' 语法，实际输出:\n%s", output)
	}

	// 验证条件标签 "yes" 和 "no" 存在
	if !strings.Contains(output, "yes") {
		t.Errorf("Mermaid 输出应包含条件标签 'yes'，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "no") {
		t.Errorf("Mermaid 输出应包含条件标签 'no'，实际输出:\n%s", output)
	}
}

// TestExport_DOT 测试 DOT (Graphviz) 格式导出
// 验证输出包含正确的 digraph 声明、节点和边
func TestExport_DOT(t *testing.T) {
	g := buildSimpleGraph(t)
	output := g.Export(FormatDOT)

	// 验证 digraph 声明
	if !strings.Contains(output, "digraph") {
		t.Errorf("DOT 输出应包含 'digraph' 声明，实际输出:\n%s", output)
	}

	// 验证图名称
	if !strings.Contains(output, "test-viz") {
		t.Errorf("DOT 输出应包含图名称 'test-viz'，实际输出:\n%s", output)
	}

	// 验证 rankdir 设置（默认 TB）
	if !strings.Contains(output, "rankdir=TB") {
		t.Errorf("DOT 输出应包含 'rankdir=TB'，实际输出:\n%s", output)
	}

	// 验证 START 和 END 特殊节点
	if !strings.Contains(output, "__START__") {
		t.Errorf("DOT 输出应包含 __START__ 节点，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "__END__") {
		t.Errorf("DOT 输出应包含 __END__ 节点，实际输出:\n%s", output)
	}

	// 验证普通节点 A 和 B 存在
	if !strings.Contains(output, `"A"`) {
		t.Errorf("DOT 输出应包含节点 A，实际输出:\n%s", output)
	}
	if !strings.Contains(output, `"B"`) {
		t.Errorf("DOT 输出应包含节点 B，实际输出:\n%s", output)
	}

	// 验证边使用 -> 语法
	if !strings.Contains(output, "->") {
		t.Errorf("DOT 输出应包含 '->' 边连接，实际输出:\n%s", output)
	}

	// 验证图以闭合花括号结尾
	if !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Errorf("DOT 输出应以 '}' 结尾，实际输出:\n%s", output)
	}
}

// TestExport_DOT_Conditional 测试 DOT 格式导出条件边
// 验证条件边使用虚线样式渲染
func TestExport_DOT_Conditional(t *testing.T) {
	g := buildConditionalGraph(t)
	output := g.Export(FormatDOT, WithShowConditions(true))

	// 验证条件边使用 dashed 样式
	if !strings.Contains(output, "style=dashed") {
		t.Errorf("DOT 条件边输出应包含 'style=dashed'，实际输出:\n%s", output)
	}

	// 验证条件标签
	if !strings.Contains(output, "yes") {
		t.Errorf("DOT 输出应包含条件标签 'yes'，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "no") {
		t.Errorf("DOT 输出应包含条件标签 'no'，实际输出:\n%s", output)
	}
}

// TestExport_ASCII 测试 ASCII 格式导出
// 验证输出包含标题、节点列表和边列表
func TestExport_ASCII(t *testing.T) {
	g := buildSimpleGraph(t)
	output := g.Export(FormatASCII)

	// 验证标题（默认使用图名称）
	if !strings.Contains(output, "=== test-viz ===") {
		t.Errorf("ASCII 输出应包含标题 '=== test-viz ==='，实际输出:\n%s", output)
	}

	// 验证节点部分标记
	if !strings.Contains(output, "节点:") {
		t.Errorf("ASCII 输出应包含 '节点:' 部分标记，实际输出:\n%s", output)
	}

	// 验证边部分标记
	if !strings.Contains(output, "边:") {
		t.Errorf("ASCII 输出应包含 '边:' 部分标记，实际输出:\n%s", output)
	}

	// 验证节点 A 和 B
	if !strings.Contains(output, "[A]") {
		t.Errorf("ASCII 输出应包含节点 '[A]'，实际输出:\n%s", output)
	}
	if !strings.Contains(output, "[B]") {
		t.Errorf("ASCII 输出应包含节点 '[B]'，实际输出:\n%s", output)
	}

	// 验证边使用 --> 语法
	if !strings.Contains(output, "-->") {
		t.Errorf("ASCII 输出应包含 '-->' 边连接，实际输出:\n%s", output)
	}
}

// TestExport_ASCII_Conditional 测试 ASCII 格式导出条件边
// 验证条件边使用 --label--> 语法渲染
func TestExport_ASCII_Conditional(t *testing.T) {
	g := buildConditionalGraph(t)
	output := g.Export(FormatASCII)

	// 验证条件边语法: --label-->
	if !strings.Contains(output, "--yes-->") || !strings.Contains(output, "--no-->") {
		t.Errorf("ASCII 条件边输出应包含 '--yes-->' 和 '--no-->' 语法，实际输出:\n%s", output)
	}
}

// TestExport_WithOptions 测试导出选项的正确应用
func TestExport_WithOptions(t *testing.T) {
	g := buildSimpleGraph(t)

	// 子测试: 自定义标题
	t.Run("自定义标题", func(t *testing.T) {
		// DOT 格式使用 cfg.title 覆盖图名称
		output := g.Export(FormatDOT, WithExportTitle("我的工作流"))
		if !strings.Contains(output, "我的工作流") {
			t.Errorf("DOT 输出应包含自定义标题 '我的工作流'，实际输出:\n%s", output)
		}

		// ASCII 格式同样使用 cfg.title
		outputASCII := g.Export(FormatASCII, WithExportTitle("我的工作流"))
		if !strings.Contains(outputASCII, "=== 我的工作流 ===") {
			t.Errorf("ASCII 输出应包含自定义标题 '=== 我的工作流 ==='，实际输出:\n%s", outputASCII)
		}
	})

	// 子测试: 方向设置
	t.Run("LR方向", func(t *testing.T) {
		// Mermaid 格式使用 graph LR
		output := g.Export(FormatMermaid, WithDirection("LR"))
		if !strings.Contains(output, "graph LR") {
			t.Errorf("Mermaid 输出应包含 'graph LR' 方向声明，实际输出:\n%s", output)
		}

		// DOT 格式使用 rankdir=LR
		outputDOT := g.Export(FormatDOT, WithDirection("LR"))
		if !strings.Contains(outputDOT, "rankdir=LR") {
			t.Errorf("DOT 输出应包含 'rankdir=LR'，实际输出:\n%s", outputDOT)
		}
	})

	// 子测试: 高亮节点
	t.Run("高亮节点", func(t *testing.T) {
		// Mermaid 格式为高亮节点添加 style 规则
		output := g.Export(FormatMermaid, WithHighlightNodes("A"))
		if !strings.Contains(output, "style A fill:#f96") {
			t.Errorf("Mermaid 输出应包含节点 A 的高亮样式，实际输出:\n%s", output)
		}

		// DOT 格式为高亮节点添加 fillcolor
		outputDOT := g.Export(FormatDOT, WithHighlightNodes("A"))
		if !strings.Contains(outputDOT, "fillcolor") {
			t.Errorf("DOT 输出应包含高亮节点的 fillcolor 属性，实际输出:\n%s", outputDOT)
		}

		// ASCII 格式为高亮节点添加 * 标记
		outputASCII := g.Export(FormatASCII, WithHighlightNodes("A"))
		if !strings.Contains(outputASCII, "* [A]") {
			t.Errorf("ASCII 输出应包含高亮节点的 '* [A]' 标记，实际输出:\n%s", outputASCII)
		}
	})

	// 子测试: 隐藏条件标签
	t.Run("隐藏条件标签", func(t *testing.T) {
		cg := buildConditionalGraph(t)

		// showConditions=false 时不应显示条件边
		output := cg.Export(FormatMermaid, WithShowConditions(false))
		if strings.Contains(output, "-->|yes|") {
			t.Errorf("Mermaid 输出在隐藏条件时不应包含 '-->|yes|'，实际输出:\n%s", output)
		}

		outputDOT := cg.Export(FormatDOT, WithShowConditions(false))
		if strings.Contains(outputDOT, "style=dashed") {
			t.Errorf("DOT 输出在隐藏条件时不应包含 'style=dashed'，实际输出:\n%s", outputDOT)
		}
	})

	// 子测试: 多个选项组合
	t.Run("多选项组合", func(t *testing.T) {
		output := g.Export(FormatDOT,
			WithExportTitle("组合测试"),
			WithDirection("LR"),
			WithHighlightNodes("A", "B"),
		)

		// 验证标题生效
		if !strings.Contains(output, "组合测试") {
			t.Errorf("组合选项输出应包含自定义标题，实际输出:\n%s", output)
		}
		// 验证方向生效
		if !strings.Contains(output, "rankdir=LR") {
			t.Errorf("组合选项输出应包含 LR 方向，实际输出:\n%s", output)
		}
		// 验证两个节点都高亮
		if !strings.Contains(output, "fillcolor") {
			t.Errorf("组合选项输出应包含高亮样式，实际输出:\n%s", output)
		}
	})
}

// TestExport_DefaultFormat 测试未知格式回退到 Mermaid
func TestExport_DefaultFormat(t *testing.T) {
	g := buildSimpleGraph(t)

	// 使用一个未定义的格式值，应回退到 Mermaid 格式
	output := g.Export(ExportFormat(99))
	if !strings.Contains(output, "graph TB") {
		t.Errorf("未知格式应回退到 Mermaid，输出应包含 'graph TB'，实际输出:\n%s", output)
	}
}

// TestSanitizeMermaidID 测试 Mermaid ID 清理函数
// 验证特殊字符被替换、START/END 被转换
func TestSanitizeMermaidID(t *testing.T) {
	tests := []struct {
		name     string // 测试用例名称
		input    string // 输入节点名
		expected string // 期望的 Mermaid ID
	}{
		{"普通名称不变", "nodeA", "nodeA"},
		{"空格替换为下划线", "node A", "node_A"},
		{"连字符替换为下划线", "node-B", "node_B"},
		{"点号替换为下划线", "node.C", "node_C"},
		{"斜杠替换为下划线", "path/to", "path_to"},
		{"冒号替换为下划线", "ns:name", "ns_name"},
		{"START常量转为__START__", START, "__START__"},
		{"END常量转为__END__", END, "__END__"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMermaidID(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeMermaidID(%q) = %q，期望 %q", tt.input, got, tt.expected)
			}
		})
	}
}
