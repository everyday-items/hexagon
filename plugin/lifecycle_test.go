package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockPluginForLifecycle 用于生命周期测试的模拟插件
type mockPluginForLifecycle struct {
	*BasePlugin
	initErr      error
	startErr     error
	stopErr      error
	dependencies []string
}

func newMockPluginForLifecycle(name string, deps ...string) *mockPluginForLifecycle {
	return &mockPluginForLifecycle{
		BasePlugin: NewBasePlugin(PluginInfo{
			Name:         name,
			Version:      "1.0.0",
			Type:         PluginTypeTool,
			Dependencies: deps,
		}),
		dependencies: deps,
	}
}

func (m *mockPluginForLifecycle) Info() PluginInfo {
	info := m.BasePlugin.Info()
	info.Dependencies = m.dependencies
	return info
}

func (m *mockPluginForLifecycle) Init(ctx context.Context, config map[string]any) error {
	if m.initErr != nil {
		return m.initErr
	}
	return m.BasePlugin.Init(ctx, config)
}

func (m *mockPluginForLifecycle) Start(ctx context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	return m.BasePlugin.Start(ctx)
}

func (m *mockPluginForLifecycle) Stop(ctx context.Context) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	return m.BasePlugin.Stop(ctx)
}

// TestLifecycleInit 测试插件初始化
func TestLifecycleInit(t *testing.T) {
	t.Run("成功初始化", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("test-plugin")
		if err := registry.Register(plugin); err != nil {
			t.Fatalf("注册插件失败: %v", err)
		}

		ctx := context.Background()
		config := map[string]any{"key": "value"}

		if err := lifecycle.Init(ctx, "test-plugin", config); err != nil {
			t.Fatalf("初始化失败: %v", err)
		}

		// 验证状态
		instance, _ := registry.GetInstance("test-plugin")
		if instance.State != PluginStateInitialized {
			t.Errorf("状态应为 Initialized，实际为 %s", instance.State)
		}
	})

	t.Run("初始化失败", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("fail-plugin")
		plugin.initErr = errors.New("初始化错误")
		registry.Register(plugin)

		ctx := context.Background()
		err := lifecycle.Init(ctx, "fail-plugin", nil)

		if err == nil {
			t.Error("预期返回错误")
		}

		// 验证状态为错误
		instance, _ := registry.GetInstance("fail-plugin")
		if instance.State != PluginStateError {
			t.Errorf("状态应为 Error，实际为 %s", instance.State)
		}
	})

	t.Run("插件不存在", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		ctx := context.Background()
		err := lifecycle.Init(ctx, "nonexistent", nil)

		if err == nil {
			t.Error("预期返回错误")
		}
	})
}

// TestLifecycleStart 测试插件启动
func TestLifecycleStart(t *testing.T) {
	t.Run("成功启动", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("test-plugin")
		registry.Register(plugin)

		ctx := context.Background()
		lifecycle.Init(ctx, "test-plugin", nil)

		if err := lifecycle.Start(ctx, "test-plugin"); err != nil {
			t.Fatalf("启动失败: %v", err)
		}

		// 验证状态
		instance, _ := registry.GetInstance("test-plugin")
		if instance.State != PluginStateRunning {
			t.Errorf("状态应为 Running，实际为 %s", instance.State)
		}
		if instance.StartedAt == nil {
			t.Error("StartedAt 不应为空")
		}
	})

	t.Run("依赖未启动", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		dep := newMockPluginForLifecycle("dep-plugin")
		main := newMockPluginForLifecycle("main-plugin", "dep-plugin")

		registry.Register(dep)
		registry.Register(main)

		ctx := context.Background()
		lifecycle.Init(ctx, "dep-plugin", nil)
		lifecycle.Init(ctx, "main-plugin", nil)

		// 尝试启动依赖未运行的插件
		err := lifecycle.Start(ctx, "main-plugin")
		if err == nil {
			t.Error("预期返回错误：依赖未启动")
		}
	})

	t.Run("按依赖顺序启动", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		dep := newMockPluginForLifecycle("dep-plugin")
		main := newMockPluginForLifecycle("main-plugin", "dep-plugin")

		registry.Register(dep)
		registry.Register(main)

		ctx := context.Background()
		lifecycle.Init(ctx, "dep-plugin", nil)
		lifecycle.Init(ctx, "main-plugin", nil)

		// 先启动依赖
		if err := lifecycle.Start(ctx, "dep-plugin"); err != nil {
			t.Fatalf("启动依赖失败: %v", err)
		}

		// 再启动主插件
		if err := lifecycle.Start(ctx, "main-plugin"); err != nil {
			t.Fatalf("启动主插件失败: %v", err)
		}

		// 验证启动顺序
		order := lifecycle.GetStartOrder()
		if len(order) != 2 || order[0] != "dep-plugin" || order[1] != "main-plugin" {
			t.Errorf("启动顺序不正确: %v", order)
		}
	})
}

// TestLifecycleStop 测试插件停止
func TestLifecycleStop(t *testing.T) {
	t.Run("成功停止", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("test-plugin")
		registry.Register(plugin)

		ctx := context.Background()
		lifecycle.Init(ctx, "test-plugin", nil)
		lifecycle.Start(ctx, "test-plugin")

		if err := lifecycle.Stop(ctx, "test-plugin"); err != nil {
			t.Fatalf("停止失败: %v", err)
		}

		// 验证状态
		instance, _ := registry.GetInstance("test-plugin")
		if instance.State != PluginStateStopped {
			t.Errorf("状态应为 Stopped，实际为 %s", instance.State)
		}
	})

	t.Run("有依赖者时不能停止", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		dep := newMockPluginForLifecycle("dep-plugin")
		main := newMockPluginForLifecycle("main-plugin", "dep-plugin")

		registry.Register(dep)
		registry.Register(main)

		ctx := context.Background()
		lifecycle.Init(ctx, "dep-plugin", nil)
		lifecycle.Init(ctx, "main-plugin", nil)
		lifecycle.Start(ctx, "dep-plugin")
		lifecycle.Start(ctx, "main-plugin")

		// 尝试停止被依赖的插件
		err := lifecycle.Stop(ctx, "dep-plugin")
		if err == nil {
			t.Error("预期返回错误：有其他插件依赖")
		}
	})
}

// TestLifecycleRestart 测试插件重启
func TestLifecycleRestart(t *testing.T) {
	t.Run("成功重启", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("test-plugin")
		registry.Register(plugin)

		ctx := context.Background()
		lifecycle.Init(ctx, "test-plugin", nil)
		lifecycle.Start(ctx, "test-plugin")

		if err := lifecycle.Restart(ctx, "test-plugin"); err != nil {
			t.Fatalf("重启失败: %v", err)
		}

		// 验证状态
		instance, _ := registry.GetInstance("test-plugin")
		if instance.State != PluginStateRunning {
			t.Errorf("状态应为 Running，实际为 %s", instance.State)
		}
	})
}

// TestLifecycleStartAll 测试批量启动
func TestLifecycleStartAll(t *testing.T) {
	t.Run("按依赖顺序启动所有插件", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		// 创建有依赖关系的插件
		p1 := newMockPluginForLifecycle("plugin-1")
		p2 := newMockPluginForLifecycle("plugin-2", "plugin-1")
		p3 := newMockPluginForLifecycle("plugin-3", "plugin-2")

		registry.Register(p1)
		registry.Register(p2)
		registry.Register(p3)

		ctx := context.Background()
		configs := map[string]map[string]any{
			"plugin-1": {},
			"plugin-2": {},
			"plugin-3": {},
		}
		lifecycle.InitAll(ctx, configs)

		if err := lifecycle.StartAll(ctx); err != nil {
			t.Fatalf("StartAll 失败: %v", err)
		}

		// 验证所有插件都已启动
		for _, name := range []string{"plugin-1", "plugin-2", "plugin-3"} {
			instance, _ := registry.GetInstance(name)
			if instance.State != PluginStateRunning {
				t.Errorf("插件 %s 状态应为 Running，实际为 %s", name, instance.State)
			}
		}
	})
}

// TestLifecycleStopAll 测试批量停止
func TestLifecycleStopAll(t *testing.T) {
	t.Run("逆序停止所有插件", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		p1 := newMockPluginForLifecycle("plugin-1")
		p2 := newMockPluginForLifecycle("plugin-2")

		registry.Register(p1)
		registry.Register(p2)

		ctx := context.Background()
		lifecycle.Init(ctx, "plugin-1", nil)
		lifecycle.Init(ctx, "plugin-2", nil)
		lifecycle.Start(ctx, "plugin-1")
		lifecycle.Start(ctx, "plugin-2")

		if err := lifecycle.StopAll(ctx); err != nil {
			t.Fatalf("StopAll 失败: %v", err)
		}

		// 验证所有插件都已停止
		for _, name := range []string{"plugin-1", "plugin-2"} {
			instance, _ := registry.GetInstance(name)
			if instance.State != PluginStateStopped {
				t.Errorf("插件 %s 状态应为 Stopped，实际为 %s", name, instance.State)
			}
		}
	})
}

// TestSortByDependency 测试依赖排序
func TestSortByDependency(t *testing.T) {
	t.Run("基本依赖排序", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugins := []PluginInfo{
			{Name: "c", Dependencies: []string{"a", "b"}},
			{Name: "a", Dependencies: []string{}},
			{Name: "b", Dependencies: []string{"a"}},
		}

		sorted := lifecycle.sortByDependency(plugins)

		// 验证顺序：a 应该在最前面
		if len(sorted) != 3 {
			t.Fatalf("排序结果长度错误: %d", len(sorted))
		}
		if sorted[0].Name != "a" {
			t.Errorf("第一个应该是 a，实际是 %s", sorted[0].Name)
		}
		// b 依赖 a，所以 b 应该在 a 之后
		aIdx, bIdx, cIdx := -1, -1, -1
		for i, p := range sorted {
			switch p.Name {
			case "a":
				aIdx = i
			case "b":
				bIdx = i
			case "c":
				cIdx = i
			}
		}
		if aIdx > bIdx || aIdx > cIdx || bIdx > cIdx {
			t.Errorf("排序顺序不正确: a=%d, b=%d, c=%d", aIdx, bIdx, cIdx)
		}
	})

	t.Run("无依赖的插件", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugins := []PluginInfo{
			{Name: "a", Dependencies: []string{}},
			{Name: "b", Dependencies: []string{}},
		}

		sorted := lifecycle.sortByDependency(plugins)
		if len(sorted) != 2 {
			t.Errorf("排序结果长度错误: %d", len(sorted))
		}
	})

	t.Run("空列表", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		sorted := lifecycle.sortByDependency(nil)
		if len(sorted) != 0 {
			t.Errorf("空列表排序结果应为空")
		}
	})
}

// TestHealthCheck 测试健康检查
func TestHealthCheck(t *testing.T) {
	t.Run("健康检查返回状态", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		plugin := newMockPluginForLifecycle("test-plugin")
		registry.Register(plugin)

		ctx := context.Background()
		lifecycle.Init(ctx, "test-plugin", nil)
		lifecycle.Start(ctx, "test-plugin")

		results := lifecycle.HealthCheck(ctx)

		status, exists := results["test-plugin"]
		if !exists {
			t.Fatal("应该返回 test-plugin 的健康状态")
		}
		if status.Status != HealthStateHealthy {
			t.Errorf("状态应为 Healthy，实际为 %s", status.Status)
		}
	})
}

// TestLifecycleWithHealthCheckInterval 测试健康检查间隔配置
func TestLifecycleWithHealthCheckInterval(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycle(registry, WithHealthCheckInterval(1*time.Second))

	if lifecycle.healthCheckTick != 1*time.Second {
		t.Errorf("健康检查间隔应为 1s，实际为 %v", lifecycle.healthCheckTick)
	}
}

// TestPluginManager 测试插件管理器
func TestPluginManager(t *testing.T) {
	t.Run("创建管理器", func(t *testing.T) {
		manager := NewPluginManager()
		if manager.Registry == nil {
			t.Error("Registry 不应为空")
		}
		if manager.Lifecycle == nil {
			t.Error("Lifecycle 不应为空")
		}
	})

	t.Run("加载和卸载插件", func(t *testing.T) {
		manager := NewPluginManager()

		plugin := newMockPluginForLifecycle("test-plugin")
		if err := manager.Load(plugin); err != nil {
			t.Fatalf("加载失败: %v", err)
		}

		ctx := context.Background()
		if err := manager.Unload(ctx, "test-plugin"); err != nil {
			t.Fatalf("卸载失败: %v", err)
		}
	})

	t.Run("启用和禁用插件", func(t *testing.T) {
		manager := NewPluginManager()

		plugin := newMockPluginForLifecycle("test-plugin")
		manager.Load(plugin)

		ctx := context.Background()
		config := map[string]any{"key": "value"}

		if err := manager.Enable(ctx, "test-plugin", config); err != nil {
			t.Fatalf("启用失败: %v", err)
		}

		status := manager.Status()
		if status["test-plugin"] != PluginStateRunning {
			t.Errorf("状态应为 Running")
		}

		if err := manager.Disable(ctx, "test-plugin"); err != nil {
			t.Fatalf("禁用失败: %v", err)
		}

		status = manager.Status()
		if status["test-plugin"] != PluginStateStopped {
			t.Errorf("状态应为 Stopped")
		}
	})

	t.Run("获取统计信息", func(t *testing.T) {
		manager := NewPluginManager()

		plugin := newMockPluginForLifecycle("test-plugin")
		manager.Load(plugin)

		stats := manager.Stats()
		if stats["total"].(int) != 1 {
			t.Errorf("总数应为 1")
		}
	})
}

// TestSortPluginsByPriority 测试按优先级排序
func TestSortPluginsByPriority(t *testing.T) {
	configs := []PluginConfig{
		{Name: "c", Priority: 30},
		{Name: "a", Priority: 10},
		{Name: "b", Priority: 20},
	}

	sorted := SortPluginsByPriority(configs)

	if sorted[0].Name != "a" || sorted[1].Name != "b" || sorted[2].Name != "c" {
		t.Errorf("排序不正确: %v", sorted)
	}
}
