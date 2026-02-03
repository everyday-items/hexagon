package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNewLoader 测试创建加载器
func TestNewLoader(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle)

		if loader.registry != registry {
			t.Error("registry 应该正确设置")
		}
		if loader.lifecycle != lifecycle {
			t.Error("lifecycle 应该正确设置")
		}
		if len(loader.searchPaths) != 3 {
			t.Errorf("默认搜索路径数量应为 3，实际为 %d", len(loader.searchPaths))
		}
	})

	t.Run("自定义搜索路径", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle, WithSearchPaths("/custom/path"))

		if len(loader.searchPaths) != 1 {
			t.Errorf("搜索路径数量应为 1，实际为 %d", len(loader.searchPaths))
		}
		if loader.searchPaths[0] != "/custom/path" {
			t.Errorf("搜索路径应为 /custom/path，实际为 %s", loader.searchPaths[0])
		}
	})
}

// TestLoaderLoadFromConfig 测试从配置文件加载
func TestLoaderLoadFromConfig(t *testing.T) {
	t.Run("配置文件不存在", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle)

		ctx := context.Background()
		err := loader.LoadFromConfig(ctx, "/nonexistent/config.yaml")

		if err == nil {
			t.Error("应该返回错误")
		}
	})

	t.Run("有效配置文件", func(t *testing.T) {
		// 创建临时配置文件
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "plugins.yaml")

		configContent := `
plugins:
  - name: test-plugin
    enabled: true
    config:
      key: value
    priority: 10
`
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("创建配置文件失败: %v", err)
		}

		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		// 注册工厂
		registry.RegisterFactory("test-plugin", func() Plugin {
			return NewBasePlugin(PluginInfo{
				Name:    "test-plugin",
				Version: "1.0.0",
				Type:    PluginTypeTool,
			})
		})

		loader := NewLoader(registry, lifecycle)
		ctx := context.Background()

		if err := loader.LoadFromConfig(ctx, configPath); err != nil {
			t.Fatalf("加载配置失败: %v", err)
		}

		// 验证插件已加载
		_, err := registry.Get("test-plugin")
		if err != nil {
			t.Error("插件应该已注册")
		}
	})

	t.Run("无效 YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yaml")

		if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
			t.Fatalf("创建配置文件失败: %v", err)
		}

		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle)

		ctx := context.Background()
		err := loader.LoadFromConfig(ctx, configPath)

		if err == nil {
			t.Error("应该返回解析错误")
		}
	})
}

// TestLoaderLoadFromDirectory 测试从目录加载
func TestLoaderLoadFromDirectory(t *testing.T) {
	t.Run("目录不存在", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle)

		ctx := context.Background()
		err := loader.LoadFromDirectory(ctx, "/nonexistent/dir")

		if err == nil {
			t.Error("应该返回错误")
		}
	})

	t.Run("空目录", func(t *testing.T) {
		tmpDir := t.TempDir()

		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle)

		ctx := context.Background()
		err := loader.LoadFromDirectory(ctx, tmpDir)

		if err != nil {
			t.Errorf("空目录不应返回错误: %v", err)
		}
	})

	t.Run("有配置文件的目录", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 创建配置文件
		configContent := `
plugins:
  - name: dir-plugin
    enabled: true
    config: {}
`
		configPath := filepath.Join(tmpDir, "plugins.yaml")
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("创建配置文件失败: %v", err)
		}

		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)

		// 注册工厂
		registry.RegisterFactory("dir-plugin", func() Plugin {
			return NewBasePlugin(PluginInfo{
				Name:    "dir-plugin",
				Version: "1.0.0",
				Type:    PluginTypeTool,
			})
		})

		loader := NewLoader(registry, lifecycle)
		ctx := context.Background()

		if err := loader.LoadFromDirectory(ctx, tmpDir); err != nil {
			t.Fatalf("加载目录失败: %v", err)
		}
	})
}

// TestLoaderDiscover 测试发现插件
func TestLoaderDiscover(t *testing.T) {
	t.Run("空搜索路径", func(t *testing.T) {
		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle, WithSearchPaths())

		discovered, err := loader.Discover()
		if err != nil {
			t.Errorf("不应返回错误: %v", err)
		}
		if len(discovered) != 0 {
			t.Errorf("应该没有发现插件")
		}
	})

	t.Run("发现插件", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 创建插件目录和清单
		pluginDir := filepath.Join(tmpDir, "my-plugin")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("创建插件目录失败: %v", err)
		}

		manifestContent := `
name: my-plugin
version: 1.0.0
type: tool
description: Test plugin
`
		manifestPath := filepath.Join(pluginDir, "plugin.yaml")
		if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
			t.Fatalf("创建清单文件失败: %v", err)
		}

		registry := NewRegistry()
		lifecycle := NewLifecycle(registry)
		loader := NewLoader(registry, lifecycle, WithSearchPaths(tmpDir))

		discovered, err := loader.Discover()
		if err != nil {
			t.Fatalf("发现插件失败: %v", err)
		}

		if len(discovered) != 1 {
			t.Errorf("应该发现 1 个插件，实际发现 %d 个", len(discovered))
		}

		if len(discovered) > 0 && discovered[0].Name != "my-plugin" {
			t.Errorf("插件名应为 my-plugin，实际为 %s", discovered[0].Name)
		}
	})
}

// TestPluginManifest 测试插件清单
func TestPluginManifest(t *testing.T) {
	t.Run("解析清单", func(t *testing.T) {
		data := []byte(`
info:
  name: test-plugin
  version: 1.0.0
  type: tool
  description: Test plugin
config:
  key: value
hooks:
  on_load: echo "loaded"
`)

		manifest, err := ParseManifest(data)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}

		if manifest.Info.Name != "test-plugin" {
			t.Errorf("Name = %s, want test-plugin", manifest.Info.Name)
		}
		if manifest.Info.Version != "1.0.0" {
			t.Errorf("Version = %s, want 1.0.0", manifest.Info.Version)
		}
		if manifest.Config["key"] != "value" {
			t.Error("Config 解析错误")
		}
	})

	t.Run("无效清单", func(t *testing.T) {
		data := []byte("invalid yaml: :")

		_, err := ParseManifest(data)
		if err == nil {
			t.Error("应该返回解析错误")
		}
	})

	t.Run("加载清单文件", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "plugin.yaml")

		content := `
info:
  name: file-plugin
  version: 2.0.0
  type: provider
`
		if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}

		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}

		if manifest.Info.Name != "file-plugin" {
			t.Errorf("Name = %s, want file-plugin", manifest.Info.Name)
		}
	})

	t.Run("加载不存在的文件", func(t *testing.T) {
		_, err := LoadManifest("/nonexistent/plugin.yaml")
		if err == nil {
			t.Error("应该返回错误")
		}
	})
}

// TestBuiltinPlugins 测试内置插件
func TestBuiltinPlugins(t *testing.T) {
	t.Run("创建和添加", func(t *testing.T) {
		builtins := NewBuiltinPlugins()

		builtins.Add("plugin-a", func() Plugin {
			return NewBasePlugin(PluginInfo{Name: "plugin-a", Version: "1.0.0"})
		})
		builtins.Add("plugin-b", func() Plugin {
			return NewBasePlugin(PluginInfo{Name: "plugin-b", Version: "1.0.0"})
		})

		list := builtins.List()
		if len(list) != 2 {
			t.Errorf("应该有 2 个插件，实际有 %d 个", len(list))
		}
	})

	t.Run("注册到注册表", func(t *testing.T) {
		builtins := NewBuiltinPlugins()
		builtins.Add("builtin-plugin", func() Plugin {
			return NewBasePlugin(PluginInfo{Name: "builtin-plugin", Version: "1.0.0"})
		})

		registry := NewRegistry()
		if err := builtins.RegisterAll(registry); err != nil {
			t.Fatalf("注册失败: %v", err)
		}

		factories := registry.ListFactories()
		found := false
		for _, name := range factories {
			if name == "builtin-plugin" {
				found = true
				break
			}
		}
		if !found {
			t.Error("工厂应该已注册")
		}
	})

	t.Run("链式调用", func(t *testing.T) {
		builtins := NewBuiltinPlugins().
			Add("plugin-1", func() Plugin { return nil }).
			Add("plugin-2", func() Plugin { return nil })

		if len(builtins.List()) != 2 {
			t.Error("链式调用应该工作")
		}
	})
}

// TestPluginsConfig 测试插件配置结构
func TestPluginsConfig(t *testing.T) {
	config := PluginsConfig{
		Plugins: []PluginConfig{
			{Name: "a", Enabled: true, Priority: 1},
			{Name: "b", Enabled: false, Priority: 2},
		},
	}

	if len(config.Plugins) != 2 {
		t.Errorf("应该有 2 个插件配置")
	}
	if config.Plugins[0].Name != "a" {
		t.Errorf("第一个插件名应为 a")
	}
}
