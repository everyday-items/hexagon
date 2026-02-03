package plugin_test

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/plugin"
)

// TestPluginInfo 测试插件信息
func TestPluginInfo(t *testing.T) {
	t.Run("BasicInfo", func(t *testing.T) {
		info := plugin.PluginInfo{
			Name:        "test-plugin",
			Version:     "1.0.0",
			Type:        plugin.PluginTypeTool,
			Description: "Test plugin",
			Author:      "Test Author",
			License:     "MIT",
			Homepage:    "https://example.com",
			Dependencies: []string{"dep1", "dep2"},
			Tags:        []string{"tag1", "tag2"},
			Metadata: map[string]any{
				"key": "value",
			},
		}

		if info.Name != "test-plugin" {
			t.Errorf("Name = %s, want test-plugin", info.Name)
		}
		if info.Version != "1.0.0" {
			t.Errorf("Version = %s, want 1.0.0", info.Version)
		}
		if info.Type != plugin.PluginTypeTool {
			t.Errorf("Type = %s, want %s", info.Type, plugin.PluginTypeTool)
		}
		if len(info.Dependencies) != 2 {
			t.Errorf("Dependencies count = %d, want 2", len(info.Dependencies))
		}
		if len(info.Tags) != 2 {
			t.Errorf("Tags count = %d, want 2", len(info.Tags))
		}
	})
}

// TestPluginTypes 测试插件类型常量
func TestPluginTypes(t *testing.T) {
	types := []plugin.PluginType{
		plugin.PluginTypeProvider,
		plugin.PluginTypeTool,
		plugin.PluginTypeMemory,
		plugin.PluginTypeRetriever,
		plugin.PluginTypeEvaluator,
		plugin.PluginTypeAgent,
		plugin.PluginTypeMiddleware,
		plugin.PluginTypeExtension,
	}

	for _, pluginType := range types {
		if pluginType == "" {
			t.Error("Plugin type should not be empty")
		}
	}
}

// TestHealthStatus 测试健康状态
func TestHealthStatus(t *testing.T) {
	t.Run("HealthStates", func(t *testing.T) {
		states := []plugin.HealthState{
			plugin.HealthStateHealthy,
			plugin.HealthStateDegraded,
			plugin.HealthStateUnhealthy,
			plugin.HealthStateUnknown,
		}

		for _, state := range states {
			if state == "" {
				t.Error("Health state should not be empty")
			}
		}
	})

	t.Run("HealthStatus", func(t *testing.T) {
		now := time.Now()
		status := plugin.HealthStatus{
			Status:  plugin.HealthStateHealthy,
			Message: "All good",
			Details: map[string]any{
				"uptime": "1h",
			},
			LastCheck: now,
		}

		if status.Status != plugin.HealthStateHealthy {
			t.Errorf("Status = %s, want %s", status.Status, plugin.HealthStateHealthy)
		}
		if status.Message != "All good" {
			t.Errorf("Message = %s, want All good", status.Message)
		}
		if !status.LastCheck.Equal(now) {
			t.Error("LastCheck time mismatch")
		}
	})
}

// TestPluginStates 测试插件状态
func TestPluginStates(t *testing.T) {
	states := []plugin.PluginState{
		plugin.PluginStateUnloaded,
		plugin.PluginStateLoaded,
		plugin.PluginStateInitialized,
		plugin.PluginStateRunning,
		plugin.PluginStateStopped,
		plugin.PluginStateError,
	}

	for _, state := range states {
		if state == "" {
			t.Error("Plugin state should not be empty")
		}
	}
}

// TestBasePlugin 测试基础插件
func TestBasePlugin(t *testing.T) {
	info := plugin.PluginInfo{
		Name:        "base-plugin",
		Version:     "1.0.0",
		Type:        plugin.PluginTypeTool,
		Description: "Base plugin test",
	}

	p := plugin.NewBasePlugin(info)
	if p == nil {
		t.Fatal("NewBasePlugin() returned nil")
	}

	t.Run("Info", func(t *testing.T) {
		got := p.Info()
		if got.Name != info.Name {
			t.Errorf("Info().Name = %s, want %s", got.Name, info.Name)
		}
		if got.Version != info.Version {
			t.Errorf("Info().Version = %s, want %s", got.Version, info.Version)
		}
	})

	t.Run("Init", func(t *testing.T) {
		ctx := context.Background()
		config := map[string]any{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		}

		err := p.Init(ctx, config)
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		// 验证配置已保存
		if p.Config()["key1"] != "value1" {
			t.Error("Config not saved properly")
		}
	})

	t.Run("Start", func(t *testing.T) {
		ctx := context.Background()
		err := p.Start(ctx)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// 验证健康状态
		health := p.Health()
		if health.Status != plugin.HealthStateHealthy {
			t.Errorf("Health().Status = %s, want %s", health.Status, plugin.HealthStateHealthy)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		ctx := context.Background()
		err := p.Stop(ctx)
		if err != nil {
			t.Fatalf("Stop() error = %v", err)
		}

		// 验证健康状态变为未知
		health := p.Health()
		if health.Status != plugin.HealthStateUnknown {
			t.Errorf("Health().Status after Stop = %s, want %s", health.Status, plugin.HealthStateUnknown)
		}
	})

	t.Run("GetConfigString", func(t *testing.T) {
		val := p.GetConfigString("key1", "default")
		if val != "value1" {
			t.Errorf("GetConfigString() = %s, want value1", val)
		}

		val = p.GetConfigString("nonexistent", "default")
		if val != "default" {
			t.Errorf("GetConfigString() for nonexistent = %s, want default", val)
		}
	})

	t.Run("GetConfigInt", func(t *testing.T) {
		val := p.GetConfigInt("key2", 0)
		if val != 42 {
			t.Errorf("GetConfigInt() = %d, want 42", val)
		}

		val = p.GetConfigInt("nonexistent", 99)
		if val != 99 {
			t.Errorf("GetConfigInt() for nonexistent = %d, want 99", val)
		}
	})

	t.Run("GetConfigBool", func(t *testing.T) {
		val := p.GetConfigBool("key3", false)
		if !val {
			t.Error("GetConfigBool() = false, want true")
		}

		val = p.GetConfigBool("nonexistent", false)
		if val {
			t.Error("GetConfigBool() for nonexistent = true, want false")
		}
	})
}

// TestPluginInstance 测试插件实例
func TestPluginInstance(t *testing.T) {
	info := plugin.PluginInfo{
		Name:    "instance-test",
		Version: "1.0.0",
		Type:    plugin.PluginTypeTool,
	}

	p := plugin.NewBasePlugin(info)
	now := time.Now()

	instance := plugin.PluginInstance{
		Plugin:   p,
		State:    plugin.PluginStateLoaded,
		Config:   map[string]any{"key": "value"},
		LoadedAt: now,
	}

	if instance.Plugin == nil {
		t.Error("PluginInstance.Plugin should not be nil")
	}
	if instance.State != plugin.PluginStateLoaded {
		t.Errorf("PluginInstance.State = %s, want %s", instance.State, plugin.PluginStateLoaded)
	}
	if !instance.LoadedAt.Equal(now) {
		t.Error("PluginInstance.LoadedAt mismatch")
	}
}

// TestPluginConfig 测试插件配置
func TestPluginConfig(t *testing.T) {
	config := plugin.PluginConfig{
		Name:    "test-plugin",
		Enabled: true,
		Config: map[string]any{
			"setting1": "value1",
		},
		Priority: 10,
	}

	if config.Name != "test-plugin" {
		t.Errorf("PluginConfig.Name = %s, want test-plugin", config.Name)
	}
	if !config.Enabled {
		t.Error("PluginConfig.Enabled should be true")
	}
	if config.Priority != 10 {
		t.Errorf("PluginConfig.Priority = %d, want 10", config.Priority)
	}
}

// TestPluginFactory 测试插件工厂
func TestPluginFactory(t *testing.T) {
	factory := func() plugin.Plugin {
		return plugin.NewBasePlugin(plugin.PluginInfo{
			Name:    "factory-plugin",
			Version: "1.0.0",
			Type:    plugin.PluginTypeTool,
		})
	}

	p := factory()
	if p == nil {
		t.Fatal("Factory returned nil plugin")
	}

	info := p.Info()
	if info.Name != "factory-plugin" {
		t.Errorf("Factory plugin name = %s, want factory-plugin", info.Name)
	}
}

// TestPluginEvent 测试插件事件
func TestPluginEvent(t *testing.T) {
	t.Run("EventTypes", func(t *testing.T) {
		types := []plugin.PluginEventType{
			plugin.PluginEventLoaded,
			plugin.PluginEventUnloaded,
			plugin.PluginEventInitialized,
			plugin.PluginEventStarted,
			plugin.PluginEventStopped,
			plugin.PluginEventError,
			plugin.PluginEventHealthCheck,
		}

		for _, eventType := range types {
			if eventType == "" {
				t.Error("Event type should not be empty")
			}
		}
	})

	t.Run("Event", func(t *testing.T) {
		now := time.Now()
		event := plugin.PluginEvent{
			Type:      plugin.PluginEventLoaded,
			Plugin:    "test-plugin",
			Timestamp: now,
			Data: map[string]any{
				"version": "1.0.0",
			},
		}

		if event.Type != plugin.PluginEventLoaded {
			t.Errorf("Event.Type = %s, want %s", event.Type, plugin.PluginEventLoaded)
		}
		if event.Plugin != "test-plugin" {
			t.Errorf("Event.Plugin = %s, want test-plugin", event.Plugin)
		}
		if !event.Timestamp.Equal(now) {
			t.Error("Event.Timestamp mismatch")
		}
	})

	t.Run("EventHandler", func(t *testing.T) {
		handlerCalled := false
		handler := func(event plugin.PluginEvent) {
			handlerCalled = true
		}

		event := plugin.PluginEvent{
			Type:   plugin.PluginEventLoaded,
			Plugin: "test",
		}

		handler(event)

		if !handlerCalled {
			t.Error("Event handler was not called")
		}
	})
}

// MockPlugin 用于测试的模拟插件
type MockPlugin struct {
	*plugin.BasePlugin
	initCalled  bool
	startCalled bool
	stopCalled  bool
	initError   error
	startError  error
	stopError   error
}

func NewMockPlugin(name string) *MockPlugin {
	return &MockPlugin{
		BasePlugin: plugin.NewBasePlugin(plugin.PluginInfo{
			Name:    name,
			Version: "1.0.0",
			Type:    plugin.PluginTypeTool,
		}),
	}
}

func (m *MockPlugin) Init(ctx context.Context, config map[string]any) error {
	m.initCalled = true
	if m.initError != nil {
		return m.initError
	}
	return m.BasePlugin.Init(ctx, config)
}

func (m *MockPlugin) Start(ctx context.Context) error {
	m.startCalled = true
	if m.startError != nil {
		return m.startError
	}
	return m.BasePlugin.Start(ctx)
}

func (m *MockPlugin) Stop(ctx context.Context) error {
	m.stopCalled = true
	if m.stopError != nil {
		return m.stopError
	}
	return m.BasePlugin.Stop(ctx)
}

// TestMockPlugin 测试模拟插件
func TestMockPlugin(t *testing.T) {
	ctx := context.Background()
	p := NewMockPlugin("mock-plugin")

	t.Run("Init", func(t *testing.T) {
		err := p.Init(ctx, map[string]any{"key": "value"})
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		if !p.initCalled {
			t.Error("Init was not called")
		}
	})

	t.Run("Start", func(t *testing.T) {
		err := p.Start(ctx)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if !p.startCalled {
			t.Error("Start was not called")
		}
	})

	t.Run("Stop", func(t *testing.T) {
		err := p.Stop(ctx)
		if err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
		if !p.stopCalled {
			t.Error("Stop was not called")
		}
	})
}

// BenchmarkBasePlugin 基准测试
func BenchmarkBasePlugin(b *testing.B) {
	ctx := context.Background()
	info := plugin.PluginInfo{
		Name:    "bench-plugin",
		Version: "1.0.0",
		Type:    plugin.PluginTypeTool,
	}

	b.Run("NewBasePlugin", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = plugin.NewBasePlugin(info)
		}
	})

	b.Run("Info", func(b *testing.B) {
		p := plugin.NewBasePlugin(info)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = p.Info()
		}
	})

	b.Run("Init", func(b *testing.B) {
		p := plugin.NewBasePlugin(info)
		config := map[string]any{"key": "value"}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = p.Init(ctx, config)
		}
	})

	b.Run("Health", func(b *testing.B) {
		p := plugin.NewBasePlugin(info)
		p.Start(ctx)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = p.Health()
		}
	})
}
