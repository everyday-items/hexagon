package plugin

import (
	"testing"
)

// TestParseVersionConstraint 测试版本约束解析
func TestParseVersionConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		wantOp     string
		wantVer    string
		wantErr    bool
	}{
		{
			name:       "精确版本",
			constraint: "1.0.0",
			wantOp:     "=",
			wantVer:    "1.0.0",
			wantErr:    false,
		},
		{
			name:       "大于等于",
			constraint: ">=1.2.0",
			wantOp:     ">=",
			wantVer:    "1.2.0",
			wantErr:    false,
		},
		{
			name:       "小于等于",
			constraint: "<=2.0.0",
			wantOp:     "<=",
			wantVer:    "2.0.0",
			wantErr:    false,
		},
		{
			name:       "大于",
			constraint: ">1.0.0",
			wantOp:     ">",
			wantVer:    "1.0.0",
			wantErr:    false,
		},
		{
			name:       "小于",
			constraint: "<3.0.0",
			wantOp:     "<",
			wantVer:    "3.0.0",
			wantErr:    false,
		},
		{
			name:       "兼容版本",
			constraint: "~>1.0",
			wantOp:     "~>",
			wantVer:    "1.0",
			wantErr:    false,
		},
		{
			name:       "语义版本兼容",
			constraint: "^1.2.3",
			wantOp:     "^",
			wantVer:    "1.2.3",
			wantErr:    false,
		},
		{
			name:       "带空格",
			constraint: ">= 1.0.0",
			wantOp:     ">=",
			wantVer:    "1.0.0",
			wantErr:    false,
		},
		{
			name:       "空约束",
			constraint: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersionConstraint(tt.constraint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersionConstraint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Operator != tt.wantOp {
				t.Errorf("Operator = %q, want %q", got.Operator, tt.wantOp)
			}
			if got.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", got.Version, tt.wantVer)
			}
		})
	}
}

// TestVersionConstraintCheckVersion 测试版本约束检查
func TestVersionConstraintCheckVersion(t *testing.T) {
	tests := []struct {
		name       string
		constraint VersionConstraint
		version    string
		want       bool
	}{
		{
			name:       "精确匹配-成功",
			constraint: VersionConstraint{Operator: "=", Version: "1.0.0"},
			version:    "1.0.0",
			want:       true,
		},
		{
			name:       "精确匹配-失败",
			constraint: VersionConstraint{Operator: "=", Version: "1.0.0"},
			version:    "1.0.1",
			want:       false,
		},
		{
			name:       "大于-成功",
			constraint: VersionConstraint{Operator: ">", Version: "1.0.0"},
			version:    "1.0.1",
			want:       true,
		},
		{
			name:       "大于-失败",
			constraint: VersionConstraint{Operator: ">", Version: "1.0.0"},
			version:    "1.0.0",
			want:       false,
		},
		{
			name:       "大于等于-成功(等于)",
			constraint: VersionConstraint{Operator: ">=", Version: "1.0.0"},
			version:    "1.0.0",
			want:       true,
		},
		{
			name:       "大于等于-成功(大于)",
			constraint: VersionConstraint{Operator: ">=", Version: "1.0.0"},
			version:    "2.0.0",
			want:       true,
		},
		{
			name:       "小于-成功",
			constraint: VersionConstraint{Operator: "<", Version: "2.0.0"},
			version:    "1.9.9",
			want:       true,
		},
		{
			name:       "小于-失败",
			constraint: VersionConstraint{Operator: "<", Version: "2.0.0"},
			version:    "2.0.0",
			want:       false,
		},
		{
			name:       "小于等于-成功",
			constraint: VersionConstraint{Operator: "<=", Version: "2.0.0"},
			version:    "2.0.0",
			want:       true,
		},
		{
			name:       "兼容版本-成功",
			constraint: VersionConstraint{Operator: "~>", Version: "1.0"},
			version:    "1.0.5",
			want:       true,
		},
		{
			name:       "兼容版本-失败",
			constraint: VersionConstraint{Operator: "~>", Version: "1.0"},
			version:    "2.0.0",
			want:       false,
		},
		{
			name:       "语义兼容-成功",
			constraint: VersionConstraint{Operator: "^", Version: "1.2.3"},
			version:    "1.9.9",
			want:       true,
		},
		{
			name:       "语义兼容-失败(主版本不同)",
			constraint: VersionConstraint{Operator: "^", Version: "1.2.3"},
			version:    "2.0.0",
			want:       false,
		},
		{
			name:       "语义兼容-失败(版本过低)",
			constraint: VersionConstraint{Operator: "^", Version: "1.2.3"},
			version:    "1.2.0",
			want:       false,
		},
		{
			name:       "未知操作符",
			constraint: VersionConstraint{Operator: "??", Version: "1.0.0"},
			version:    "1.0.0",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.constraint.CheckVersion(tt.version)
			if got != tt.want {
				t.Errorf("CheckVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

// TestCompareVersions 测试版本比较
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"1.0", "1.0.0", 0},
		{"1.0.0", "1.0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

// TestDependencyGraph 测试依赖图
func TestDependencyGraph(t *testing.T) {
	t.Run("添加节点", func(t *testing.T) {
		graph := NewDependencyGraph()

		deps := []Dependency{
			{Name: "dep1"},
			{Name: "dep2"},
		}
		graph.AddNode("plugin", "1.0.0", deps)

		if _, exists := graph.nodes["plugin"]; !exists {
			t.Error("节点应该存在")
		}
		if len(graph.edges["plugin"]) != 2 {
			t.Errorf("边数量应为 2，实际为 %d", len(graph.edges["plugin"]))
		}
	})

	t.Run("检测循环依赖", func(t *testing.T) {
		graph := NewDependencyGraph()

		// 创建循环: a -> b -> c -> a
		graph.AddNode("a", "1.0.0", []Dependency{{Name: "b"}})
		graph.AddNode("b", "1.0.0", []Dependency{{Name: "c"}})
		graph.AddNode("c", "1.0.0", []Dependency{{Name: "a"}})

		cycle := graph.DetectCycle()
		if cycle == nil {
			t.Error("应该检测到循环依赖")
		}
	})

	t.Run("无循环依赖", func(t *testing.T) {
		graph := NewDependencyGraph()

		// 线性依赖: a -> b -> c
		graph.AddNode("a", "1.0.0", []Dependency{})
		graph.AddNode("b", "1.0.0", []Dependency{{Name: "a"}})
		graph.AddNode("c", "1.0.0", []Dependency{{Name: "b"}})

		cycle := graph.DetectCycle()
		if cycle != nil {
			t.Errorf("不应该检测到循环依赖: %v", cycle)
		}
	})
}

// TestDependencyGraphTopologicalSort 测试拓扑排序
func TestDependencyGraphTopologicalSort(t *testing.T) {
	t.Run("正常排序", func(t *testing.T) {
		graph := NewDependencyGraph()

		// 依赖图实现：edges[A] = [B] 表示 A 依赖 B
		// 拓扑排序结果：依赖者排在被依赖者之前（因为入度计算的是被指向的次数）
		// c 依赖 b，b 依赖 a，所以顺序应该是 c, b, a（从依赖者到被依赖者）
		graph.AddNode("c", "1.0.0", []Dependency{{Name: "b"}})
		graph.AddNode("b", "1.0.0", []Dependency{{Name: "a"}})
		graph.AddNode("a", "1.0.0", []Dependency{})

		sorted, err := graph.TopologicalSort()
		if err != nil {
			t.Fatalf("拓扑排序失败: %v", err)
		}

		if len(sorted) != 3 {
			t.Fatalf("排序结果长度应为 3，实际为 %d", len(sorted))
		}

		// 验证所有节点都在结果中
		nodeSet := make(map[string]bool)
		for _, name := range sorted {
			nodeSet[name] = true
		}
		if !nodeSet["a"] || !nodeSet["b"] || !nodeSet["c"] {
			t.Errorf("排序结果缺少节点: %v", sorted)
		}
	})

	t.Run("循环依赖返回错误", func(t *testing.T) {
		graph := NewDependencyGraph()

		graph.AddNode("a", "1.0.0", []Dependency{{Name: "b"}})
		graph.AddNode("b", "1.0.0", []Dependency{{Name: "a"}})

		_, err := graph.TopologicalSort()
		if err == nil {
			t.Error("循环依赖应该返回错误")
		}
	})
}

// TestDependencyResolver 测试依赖解析器
func TestDependencyResolver(t *testing.T) {
	t.Run("解析依赖顺序", func(t *testing.T) {
		registry := NewRegistry()
		resolver := NewDependencyResolver(registry)

		plugins := []PluginInfo{
			{Name: "c", Version: "1.0.0", Dependencies: []string{"b"}},
			{Name: "b", Version: "1.0.0", Dependencies: []string{"a"}},
			{Name: "a", Version: "1.0.0", Dependencies: []string{}},
		}

		order, err := resolver.Resolve(plugins)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}

		if len(order) != 3 {
			t.Errorf("顺序长度应为 3，实际为 %d", len(order))
		}
	})

	t.Run("验证依赖", func(t *testing.T) {
		registry := NewRegistry()
		resolver := NewDependencyResolver(registry)

		plugins := []PluginInfo{
			{Name: "a", Version: "1.0.0", Dependencies: []string{}},
			{Name: "b", Version: "1.0.0", Dependencies: []string{"a"}},
		}

		err := resolver.ValidateDependencies(plugins)
		if err != nil {
			t.Errorf("验证不应失败: %v", err)
		}
	})

	t.Run("验证依赖-缺失依赖", func(t *testing.T) {
		registry := NewRegistry()
		resolver := NewDependencyResolver(registry)

		plugins := []PluginInfo{
			{Name: "b", Version: "1.0.0", Dependencies: []string{"nonexistent"}},
		}

		err := resolver.ValidateDependencies(plugins)
		if err == nil {
			t.Error("应该返回缺失依赖错误")
		}
	})

	t.Run("获取加载顺序", func(t *testing.T) {
		registry := NewRegistry()
		resolver := NewDependencyResolver(registry)

		plugins := []PluginInfo{
			{Name: "c", Version: "1.0.0", Dependencies: []string{"b"}},
			{Name: "a", Version: "1.0.0", Dependencies: []string{}},
			{Name: "b", Version: "1.0.0", Dependencies: []string{"a"}},
		}

		ordered, err := resolver.GetLoadOrder(plugins)
		if err != nil {
			t.Fatalf("获取加载顺序失败: %v", err)
		}

		if len(ordered) != 3 {
			t.Errorf("顺序长度应为 3，实际为 %d", len(ordered))
		}

		// 验证所有插件都在结果中
		names := make(map[string]bool)
		for _, p := range ordered {
			names[p.Name] = true
		}
		if !names["a"] || !names["b"] || !names["c"] {
			t.Errorf("结果缺少插件: %v", ordered)
		}
	})
}

// TestDependencyTree 测试依赖树
func TestDependencyTree(t *testing.T) {
	tree := &DependencyTree{
		Plugin: PluginInfo{Name: "root", Version: "1.0.0"},
		Dependencies: []*DependencyTree{
			{
				Plugin:       PluginInfo{Name: "child1", Version: "1.0.0"},
				Dependencies: nil,
			},
			{
				Plugin:       PluginInfo{Name: "child2", Version: "1.0.0"},
				Dependencies: nil,
			},
		},
	}

	str := tree.String()
	if str == "" {
		t.Error("String() 不应返回空")
	}
	if len(str) < 10 {
		t.Errorf("String() 结果太短: %s", str)
	}
}

// TestDependency 测试依赖项
func TestDependency(t *testing.T) {
	dep := Dependency{
		Name:     "test-dep",
		Version:  ">=1.0.0",
		Optional: true,
	}

	if dep.Name != "test-dep" {
		t.Errorf("Name = %s, want test-dep", dep.Name)
	}
	if dep.Version != ">=1.0.0" {
		t.Errorf("Version = %s, want >=1.0.0", dep.Version)
	}
	if !dep.Optional {
		t.Error("Optional should be true")
	}
}
