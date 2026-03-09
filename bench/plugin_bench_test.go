package bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/plugin"
)

// mockPlugin 基准测试用的 mock 插件
type mockPlugin struct {
	info plugin.PluginInfo
}

func newMockPlugin(name string, pluginType plugin.PluginType) *mockPlugin {
	return &mockPlugin{
		info: plugin.PluginInfo{
			Name:        name,
			Version:     "1.0.0",
			Type:        pluginType,
			Description: "benchmark mock plugin",
		},
	}
}

func (p *mockPlugin) Info() plugin.PluginInfo                           { return p.info }
func (p *mockPlugin) Init(_ context.Context, _ map[string]any) error    { return nil }
func (p *mockPlugin) Start(_ context.Context) error                     { return nil }
func (p *mockPlugin) Stop(_ context.Context) error                      { return nil }
func (p *mockPlugin) Health() plugin.HealthStatus {
	return plugin.HealthStatus{
		Status:    plugin.HealthStateHealthy,
		LastCheck: time.Now(),
	}
}

// BenchmarkPluginCreation 测试插件创建性能
func BenchmarkPluginCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = plugin.NewBasePlugin(plugin.PluginInfo{
			Name:        "bench-plugin",
			Version:     "1.0.0",
			Type:        plugin.PluginTypeTool,
			Description: "benchmark plugin",
		})
	}
}

// BenchmarkPluginInit 测试插件初始化性能
func BenchmarkPluginInit(b *testing.B) {
	p := plugin.NewBasePlugin(plugin.PluginInfo{
		Name:    "bench-plugin",
		Version: "1.0.0",
		Type:    plugin.PluginTypeTool,
	})

	config := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = p.Init(ctx, config)
	}
}

// BenchmarkRegistryCreation 测试 Registry 创建性能
func BenchmarkRegistryCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = plugin.NewRegistry()
	}
}

// BenchmarkRegistryRegister 测试插件注册性能
func BenchmarkRegistryRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		registry := plugin.NewRegistry()
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}
}

// BenchmarkRegistryRegisterMultiple 测试批量注册性能
func BenchmarkRegistryRegisterMultiple(b *testing.B) {
	for _, count := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("plugins_%d", count), func(b *testing.B) {
			plugins := make([]*mockPlugin, count)
			for i := range plugins {
				plugins[i] = newMockPlugin(
					fmt.Sprintf("plugin-%d", i),
					plugin.PluginTypeTool,
				)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				registry := plugin.NewRegistry()
				for _, p := range plugins {
					_ = registry.Register(p)
				}
			}
		})
	}
}

// BenchmarkRegistryGet 测试插件查找性能
func BenchmarkRegistryGet(b *testing.B) {
	registry := plugin.NewRegistry()

	// 注册 100 个插件
	for i := 0; i < 100; i++ {
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = registry.Get("plugin-50")
	}
}

// BenchmarkRegistryGetConcurrent 测试并发查找性能
func BenchmarkRegistryGetConcurrent(b *testing.B) {
	registry := plugin.NewRegistry()

	for i := 0; i < 100; i++ {
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = registry.Get("plugin-50")
		}
	})
}

// BenchmarkRegistryHas 测试插件存在性检查性能
func BenchmarkRegistryHas(b *testing.B) {
	registry := plugin.NewRegistry()

	for i := 0; i < 100; i++ {
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = registry.Has("plugin-50")
	}
}

// BenchmarkRegistryList 测试列出所有插件性能
func BenchmarkRegistryList(b *testing.B) {
	for _, count := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("plugins_%d", count), func(b *testing.B) {
			registry := plugin.NewRegistry()

			for i := 0; i < count; i++ {
				p := newMockPlugin(
					fmt.Sprintf("plugin-%d", i),
					plugin.PluginTypeTool,
				)
				_ = registry.Register(p)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = registry.List()
			}
		})
	}
}

// BenchmarkRegistryListByType 测试按类型列出插件性能
func BenchmarkRegistryListByType(b *testing.B) {
	registry := plugin.NewRegistry()

	// 注册不同类型的插件
	types := []plugin.PluginType{
		plugin.PluginTypeTool,
		plugin.PluginTypeProvider,
		plugin.PluginTypeMemory,
		plugin.PluginTypeAgent,
	}
	for i := 0; i < 100; i++ {
		pt := types[i%len(types)]
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), pt)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = registry.ListByType(plugin.PluginTypeTool)
	}
}

// BenchmarkRegistryRegisterFactory 测试工厂注册性能
func BenchmarkRegistryRegisterFactory(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		registry := plugin.NewRegistry()
		_ = registry.RegisterFactory(fmt.Sprintf("factory-%d", i), func() plugin.Plugin {
			return newMockPlugin("created", plugin.PluginTypeTool)
		})
	}
}

// BenchmarkRegistryCreateFromFactory 测试从工厂创建插件性能
func BenchmarkRegistryCreateFromFactory(b *testing.B) {
	registry := plugin.NewRegistry()
	_ = registry.RegisterFactory("bench-factory", func() plugin.Plugin {
		return newMockPlugin("created", plugin.PluginTypeTool)
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = registry.CreateFromFactory("bench-factory")
	}
}

// BenchmarkRegistryConcurrentRegisterAndGet 测试并发注册和查找性能
func BenchmarkRegistryConcurrentRegisterAndGet(b *testing.B) {
	registry := plugin.NewRegistry()

	// 预先注册一些插件
	for i := 0; i < 50; i++ {
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%3 == 0 {
				// 查找操作（读多写少）
				_, _ = registry.Get("plugin-25")
			} else if i%3 == 1 {
				_ = registry.Has("plugin-25")
			} else {
				_ = registry.List()
			}
			i++
		}
	})
}

// BenchmarkRegistryCount 测试插件计数性能
func BenchmarkRegistryCount(b *testing.B) {
	registry := plugin.NewRegistry()

	for i := 0; i < 100; i++ {
		p := newMockPlugin(fmt.Sprintf("plugin-%d", i), plugin.PluginTypeTool)
		_ = registry.Register(p)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = registry.Count()
	}
}

// BenchmarkPluginHealth 测试插件健康检查性能
func BenchmarkPluginHealth(b *testing.B) {
	p := plugin.NewBasePlugin(plugin.PluginInfo{
		Name:    "health-plugin",
		Version: "1.0.0",
		Type:    plugin.PluginTypeTool,
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = p.Health()
	}
}

// BenchmarkPluginConfigAccess 测试插件配置访问性能
func BenchmarkPluginConfigAccess(b *testing.B) {
	p := plugin.NewBasePlugin(plugin.PluginInfo{
		Name: "config-plugin",
	})
	_ = p.Init(context.Background(), map[string]any{
		"string_key": "value",
		"int_key":    42,
		"bool_key":   true,
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = p.GetConfigString("string_key", "")
		_ = p.GetConfigInt("int_key", 0)
		_ = p.GetConfigBool("bool_key", false)
	}
}
