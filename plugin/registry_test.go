package plugin_test

import (
	"sync"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/plugin"
)

// TestRegistry 测试插件注册表
func TestRegistry(t *testing.T) {
	t.Run("NewRegistry", func(t *testing.T) {
		registry := plugin.NewRegistry()
		if registry == nil {
			t.Fatal("NewRegistry() returned nil")
		}

		if registry.Count() != 0 {
			t.Errorf("Count() = %d, want 0", registry.Count())
		}
	})

	t.Run("Register", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")

		err := registry.Register(p)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		if registry.Count() != 1 {
			t.Errorf("Count() after Register = %d, want 1", registry.Count())
		}

		// 重复注册应该失败
		err = registry.Register(p)
		if err == nil {
			t.Error("Register() duplicate should return error")
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")
		registry.Register(p)

		err := registry.Unregister("plugin1")
		if err != nil {
			t.Fatalf("Unregister() error = %v", err)
		}

		if registry.Count() != 0 {
			t.Errorf("Count() after Unregister = %d, want 0", registry.Count())
		}

		// 注销不存在的插件应该失败
		err = registry.Unregister("nonexistent")
		if err == nil {
			t.Error("Unregister() nonexistent should return error")
		}
	})

	t.Run("UnregisterRunning", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("running-plugin")
		registry.Register(p)

		// 设置为运行状态
		registry.SetState("running-plugin", plugin.PluginStateRunning)

		// 尝试注销运行中的插件应该失败
		err := registry.Unregister("running-plugin")
		if err == nil {
			t.Error("Unregister() running plugin should return error")
		}
	})

	t.Run("Get", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")
		registry.Register(p)

		got, err := registry.Get("plugin1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got == nil {
			t.Fatal("Get() returned nil")
		}

		info := got.Info()
		if info.Name != "plugin1" {
			t.Errorf("Get() Name = %s, want plugin1", info.Name)
		}

		// 获取不存在的插件应该失败
		_, err = registry.Get("nonexistent")
		if err == nil {
			t.Error("Get() nonexistent should return error")
		}
	})

	t.Run("GetInstance", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")
		registry.Register(p)

		instance, err := registry.GetInstance("plugin1")
		if err != nil {
			t.Fatalf("GetInstance() error = %v", err)
		}
		if instance == nil {
			t.Fatal("GetInstance() returned nil")
		}

		if instance.State != plugin.PluginStateLoaded {
			t.Errorf("GetInstance().State = %s, want %s", instance.State, plugin.PluginStateLoaded)
		}
	})

	t.Run("Has", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")
		registry.Register(p)

		if !registry.Has("plugin1") {
			t.Error("Has() = false, want true")
		}

		if registry.Has("nonexistent") {
			t.Error("Has() for nonexistent = true, want false")
		}
	})

	t.Run("List", func(t *testing.T) {
		registry := plugin.NewRegistry()

		// 注册多个插件
		for i := 0; i < 3; i++ {
			p := NewMockPlugin("plugin" + string(rune('1'+i)))
			registry.Register(p)
		}

		list := registry.List()
		if len(list) != 3 {
			t.Errorf("List() length = %d, want 3", len(list))
		}

		// 验证列表已排序（按名称）
		if list[0].Name != "plugin1" {
			t.Errorf("List()[0].Name = %s, want plugin1", list[0].Name)
		}
	})

	t.Run("ListByType", func(t *testing.T) {
		registry := plugin.NewRegistry()

		// 注册不同类型的插件
		p1 := plugin.NewBasePlugin(plugin.PluginInfo{
			Name: "tool1",
			Type: plugin.PluginTypeTool,
		})
		p2 := plugin.NewBasePlugin(plugin.PluginInfo{
			Name: "memory1",
			Type: plugin.PluginTypeMemory,
		})
		p3 := plugin.NewBasePlugin(plugin.PluginInfo{
			Name: "tool2",
			Type: plugin.PluginTypeTool,
		})

		registry.Register(p1)
		registry.Register(p2)
		registry.Register(p3)

		tools := registry.ListByType(plugin.PluginTypeTool)
		if len(tools) != 2 {
			t.Errorf("ListByType(Tool) length = %d, want 2", len(tools))
		}

		memories := registry.ListByType(plugin.PluginTypeMemory)
		if len(memories) != 1 {
			t.Errorf("ListByType(Memory) length = %d, want 1", len(memories))
		}
	})

	t.Run("ListByState", func(t *testing.T) {
		registry := plugin.NewRegistry()

		p1 := NewMockPlugin("plugin1")
		p2 := NewMockPlugin("plugin2")

		registry.Register(p1)
		registry.Register(p2)

		// 设置不同状态
		registry.SetState("plugin1", plugin.PluginStateRunning)
		registry.SetState("plugin2", plugin.PluginStateLoaded)

		running := registry.ListByState(plugin.PluginStateRunning)
		if len(running) != 1 {
			t.Errorf("ListByState(Running) length = %d, want 1", len(running))
		}

		loaded := registry.ListByState(plugin.PluginStateLoaded)
		if len(loaded) != 1 {
			t.Errorf("ListByState(Loaded) length = %d, want 1", len(loaded))
		}
	})

	t.Run("SetState", func(t *testing.T) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin1")
		registry.Register(p)

		err := registry.SetState("plugin1", plugin.PluginStateRunning)
		if err != nil {
			t.Fatalf("SetState() error = %v", err)
		}

		instance, _ := registry.GetInstance("plugin1")
		if instance.State != plugin.PluginStateRunning {
			t.Errorf("State after SetState = %s, want %s", instance.State, plugin.PluginStateRunning)
		}

		// 设置不存在插件的状态应该失败
		err = registry.SetState("nonexistent", plugin.PluginStateRunning)
		if err == nil {
			t.Error("SetState() for nonexistent should return error")
		}
	})
}

// TestRegistryFactory 测试工厂注册
func TestRegistryFactory(t *testing.T) {
	registry := plugin.NewRegistry()

	t.Run("RegisterFactory", func(t *testing.T) {
		factory := func() plugin.Plugin {
			return NewMockPlugin("factory-plugin")
		}

		err := registry.RegisterFactory("test-factory", factory)
		if err != nil {
			t.Fatalf("RegisterFactory() error = %v", err)
		}

		// 重复注册应该失败
		err = registry.RegisterFactory("test-factory", factory)
		if err == nil {
			t.Error("RegisterFactory() duplicate should return error")
		}
	})

	t.Run("UnregisterFactory", func(t *testing.T) {
		factory := func() plugin.Plugin {
			return NewMockPlugin("factory-plugin")
		}

		registry.RegisterFactory("temp-factory", factory)
		registry.UnregisterFactory("temp-factory")

		// 注销后应该可以再次注册
		err := registry.RegisterFactory("temp-factory", factory)
		if err != nil {
			t.Errorf("RegisterFactory() after Unregister error = %v", err)
		}
	})

	t.Run("ListFactories", func(t *testing.T) {
		registry := plugin.NewRegistry()

		factory := func() plugin.Plugin {
			return NewMockPlugin("factory-plugin")
		}

		registry.RegisterFactory("factory1", factory)
		registry.RegisterFactory("factory2", factory)

		factories := registry.ListFactories()
		if len(factories) != 2 {
			t.Errorf("ListFactories() length = %d, want 2", len(factories))
		}

		// 验证已排序
		if factories[0] != "factory1" {
			t.Errorf("ListFactories()[0] = %s, want factory1", factories[0])
		}
	})

	t.Run("CreateFromFactory", func(t *testing.T) {
		registry := plugin.NewRegistry()

		factory := func() plugin.Plugin {
			return NewMockPlugin("created-plugin")
		}

		registry.RegisterFactory("test-factory", factory)

		p, err := registry.CreateFromFactory("test-factory")
		if err != nil {
			t.Fatalf("CreateFromFactory() error = %v", err)
		}
		if p == nil {
			t.Fatal("CreateFromFactory() returned nil")
		}

		info := p.Info()
		if info.Name != "created-plugin" {
			t.Errorf("Created plugin name = %s, want created-plugin", info.Name)
		}

		// 从不存在的工厂创建应该失败
		_, err = registry.CreateFromFactory("nonexistent")
		if err == nil {
			t.Error("CreateFromFactory() nonexistent should return error")
		}
	})
}

// TestRegistryEvents 测试事件系统
func TestRegistryEvents(t *testing.T) {
	t.Run("OnEvent", func(t *testing.T) {
		registry := plugin.NewRegistry()

		var mu sync.Mutex
		events := make([]plugin.PluginEvent, 0)

		registry.OnEvent(func(event plugin.PluginEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		})

		p := NewMockPlugin("plugin1")
		registry.Register(p)

		// 等待事件处理（异步）
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		count := len(events)
		mu.Unlock()

		if count == 0 {
			t.Error("No events received")
		}

		mu.Lock()
		firstEvent := events[0]
		mu.Unlock()

		if firstEvent.Type != plugin.PluginEventLoaded {
			t.Errorf("Event type = %s, want %s", firstEvent.Type, plugin.PluginEventLoaded)
		}
		if firstEvent.Plugin != "plugin1" {
			t.Errorf("Event plugin = %s, want plugin1", firstEvent.Plugin)
		}
	})

	t.Run("MultipleHandlers", func(t *testing.T) {
		registry := plugin.NewRegistry()

		var mu sync.Mutex
		count1 := 0
		count2 := 0

		registry.OnEvent(func(event plugin.PluginEvent) {
			mu.Lock()
			count1++
			mu.Unlock()
		})

		registry.OnEvent(func(event plugin.PluginEvent) {
			mu.Lock()
			count2++
			mu.Unlock()
		})

		p := NewMockPlugin("plugin1")
		registry.Register(p)

		// 等待事件处理
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		c1 := count1
		c2 := count2
		mu.Unlock()

		if c1 == 0 {
			t.Error("Handler 1 not called")
		}
		if c2 == 0 {
			t.Error("Handler 2 not called")
		}
	})
}

// TestRegistryConcurrency 并发测试
func TestRegistryConcurrency(t *testing.T) {
	registry := plugin.NewRegistry()

	const numGoroutines = 10
	var wg sync.WaitGroup

	t.Run("ConcurrentRegister", func(t *testing.T) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				p := NewMockPlugin("plugin-" + string(rune('0'+id)))
				_ = registry.Register(p)
			}(i)
		}

		wg.Wait()

		if registry.Count() != numGoroutines {
			t.Errorf("Count() after concurrent Register = %d, want %d", registry.Count(), numGoroutines)
		}
	})

	t.Run("ConcurrentRead", func(t *testing.T) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = registry.List()
				_ = registry.Count()
			}()
		}

		wg.Wait()
	})

	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		// 读
		for i := 0; i < numGoroutines/2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = registry.List()
			}()
		}

		// 写
		for i := 0; i < numGoroutines/2; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				p := NewMockPlugin("new-plugin-" + string(rune('0'+id)))
				_ = registry.Register(p)
			}(i)
		}

		wg.Wait()
	})
}

// TestGlobalRegistry 测试全局注册表
func TestGlobalRegistry(t *testing.T) {
	t.Run("Global", func(t *testing.T) {
		g1 := plugin.Global()
		g2 := plugin.Global()

		// 应该返回同一个实例
		if g1 != g2 {
			t.Error("Global() should return the same instance")
		}
	})

	t.Run("RegisterPlugin", func(t *testing.T) {
		p := NewMockPlugin("global-plugin")
		err := plugin.RegisterPlugin(p)
		if err != nil && err.Error() != "plugin global-plugin already registered" {
			t.Fatalf("RegisterPlugin() error = %v", err)
		}
	})

	t.Run("RegisterFactoryGlobal", func(t *testing.T) {
		factory := func() plugin.Plugin {
			return NewMockPlugin("factory-plugin")
		}

		err := plugin.RegisterFactoryGlobal("global-factory", factory)
		if err != nil && err.Error() != "plugin factory global-factory already registered" {
			t.Fatalf("RegisterFactoryGlobal() error = %v", err)
		}
	})

	t.Run("GetPlugin", func(t *testing.T) {
		p := NewMockPlugin("get-test-plugin")
		plugin.RegisterPlugin(p)

		got, err := plugin.GetPlugin("get-test-plugin")
		if err != nil {
			t.Fatalf("GetPlugin() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetPlugin() returned nil")
		}
	})

	t.Run("ListPlugins", func(t *testing.T) {
		list := plugin.ListPlugins()
		if len(list) == 0 {
			t.Error("ListPlugins() returned empty list")
		}
	})
}

// BenchmarkRegistry 基准测试
func BenchmarkRegistry(b *testing.B) {
	b.Run("Register", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			registry := plugin.NewRegistry()
			p := NewMockPlugin("plugin")
			b.StartTimer()

			_ = registry.Register(p)
		}
	})

	b.Run("Get", func(b *testing.B) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin")
		registry.Register(p)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.Get("plugin")
		}
	})

	b.Run("List", func(b *testing.B) {
		registry := plugin.NewRegistry()
		for i := 0; i < 10; i++ {
			p := NewMockPlugin("plugin-" + string(rune('0'+i)))
			registry.Register(p)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = registry.List()
		}
	})

	b.Run("ConcurrentGet", func(b *testing.B) {
		registry := plugin.NewRegistry()
		p := NewMockPlugin("plugin")
		registry.Register(p)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = registry.Get("plugin")
			}
		})
	})
}
