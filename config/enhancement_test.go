package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/config"
)

// TestHotReloader 测试配置热更新
func TestHotReloader(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-agent.yaml")

	// 初始配置
	initialConfig := &config.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
	}

	// 写入初始配置
	if err := config.SaveAgentConfig(configPath, initialConfig); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	t.Run("StartAndStop", func(t *testing.T) {
		reloader := config.NewHotReloader(configPath, "agent", func(cfg any, err error) {})

		if err := reloader.Start(); err != nil {
			t.Fatalf("Failed to start reloader: %v", err)
		}

		if !reloader.IsRunning() {
			t.Error("Reloader should be running")
		}

		reloader.Stop()

		// 等待停止
		time.Sleep(100 * time.Millisecond)

		if reloader.IsRunning() {
			t.Error("Reloader should be stopped")
		}
	})

	t.Run("DetectChanges", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping slow test in short mode")
		}

		changeDetected := make(chan bool, 1)
		var receivedConfig *config.AgentConfig

		callback := func(cfg any, err error) {
			if err != nil {
				t.Logf("Reload error: %v", err)
				return
			}
			if agentConfig, ok := cfg.(*config.AgentConfig); ok {
				receivedConfig = agentConfig
				changeDetected <- true
			}
		}

		reloader := config.NewHotReloader(
			configPath,
			"agent",
			callback,
			config.WithInterval(500*time.Millisecond),
		)

		if err := reloader.Start(); err != nil {
			t.Fatalf("Failed to start reloader: %v", err)
		}
		defer reloader.Stop()

		// 等待一下确保监控已启动
		time.Sleep(600 * time.Millisecond)

		// 修改配置
		updatedConfig := &config.AgentConfig{
			Name: "test-agent",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4o", // 修改模型
			},
		}

		if err := config.SaveAgentConfig(configPath, updatedConfig); err != nil {
			t.Fatalf("Failed to save updated config: %v", err)
		}

		// 等待变更检测（最多等待 2 秒）
		select {
		case <-changeDetected:
			if receivedConfig == nil {
				t.Error("Received nil config")
			} else if receivedConfig.LLM.Model != "gpt-4o" {
				t.Errorf("Model = %s, want gpt-4o", receivedConfig.LLM.Model)
			}
		case <-time.After(2 * time.Second):
			t.Error("Change not detected within timeout")
		}
	})

	t.Run("ManualReload", func(t *testing.T) {
		reloadCalled := false

		callback := func(cfg any, err error) {
			reloadCalled = true
		}

		reloader := config.NewHotReloader(configPath, "agent", callback)

		if err := reloader.Start(); err != nil {
			t.Fatalf("Failed to start reloader: %v", err)
		}
		defer reloader.Stop()

		// 手动触发重新加载
		if err := reloader.Reload(); err != nil {
			t.Errorf("Manual reload failed: %v", err)
		}

		// 等待回调
		time.Sleep(100 * time.Millisecond)

		if !reloadCalled {
			t.Error("Reload callback was not called")
		}
	})
}

// TestHotReloadManager 测试热更新管理器
func TestHotReloadManager(t *testing.T) {
	// 创建临时配置目录
	tmpDir := t.TempDir()

	// 创建多个配置文件
	agentConfig := &config.AgentConfig{
		Name: "agent1",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
	}
	agentPath := filepath.Join(tmpDir, "agent1.yaml")
	if err := config.SaveAgentConfig(agentPath, agentConfig); err != nil {
		t.Fatalf("Failed to save agent config: %v", err)
	}

	manager := config.NewHotReloadManager()

	t.Run("WatchAndUnwatch", func(t *testing.T) {
		callback := func(cfg any, err error) {}

		// 监控配置
		if err := manager.Watch("agent1", agentPath, "agent", callback); err != nil {
			t.Fatalf("Failed to watch config: %v", err)
		}

		if manager.Count() != 1 {
			t.Errorf("Count = %d, want 1", manager.Count())
		}

		// 尝试重复监控
		if err := manager.Watch("agent1", agentPath, "agent", callback); err == nil {
			t.Error("Expected error when watching same config twice")
		}

		// 停止监控
		manager.Unwatch("agent1")

		if manager.Count() != 0 {
			t.Errorf("Count = %d, want 0", manager.Count())
		}
	})

	t.Run("StopAll", func(t *testing.T) {
		callback := func(cfg any, err error) {}

		// 监控多个配置
		manager.Watch("agent1", agentPath, "agent", callback)

		if manager.Count() != 1 {
			t.Errorf("Count = %d, want 1", manager.Count())
		}

		// 停止所有监控
		manager.StopAll()

		if manager.Count() != 0 {
			t.Errorf("Count = %d, want 0", manager.Count())
		}
	})

	t.Run("List", func(t *testing.T) {
		callback := func(cfg any, err error) {}

		manager.Watch("agent1", agentPath, "agent", callback)

		names := manager.List()
		if len(names) != 1 {
			t.Errorf("List length = %d, want 1", len(names))
		}
		if names[0] != "agent1" {
			t.Errorf("Name = %s, want agent1", names[0])
		}

		manager.StopAll()
	})
}

// TestVersionManager 测试版本管理器
func TestVersionManager(t *testing.T) {
	tmpDir := t.TempDir()

	manager, err := config.NewVersionManager(
		config.WithStorePath(filepath.Join(tmpDir, "versions")),
	)
	if err != nil {
		t.Fatalf("Failed to create version manager: %v", err)
	}

	// 测试配置
	agentConfig := &config.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
	}

	t.Run("SaveVersion", func(t *testing.T) {
		version, err := manager.SaveVersion(agentConfig, "agent", "Initial version", "test")
		if err != nil {
			t.Fatalf("Failed to save version: %v", err)
		}

		if version.ID == "" {
			t.Error("Version ID is empty")
		}
		if version.Hash == "" {
			t.Error("Version hash is empty")
		}
		if len(version.Tags) != 1 || version.Tags[0] != "test" {
			t.Errorf("Tags = %v, want [test]", version.Tags)
		}

		// 尝试保存相同配置（应该报错无变化）
		_, err = manager.SaveVersion(agentConfig, "agent", "Same config")
		if err == nil {
			t.Error("Expected error when saving unchanged config")
		}
	})

	t.Run("GetLatestVersion", func(t *testing.T) {
		version, err := manager.GetLatestVersion()
		if err != nil {
			t.Fatalf("Failed to get latest version: %v", err)
		}

		if version.ConfigType != "agent" {
			t.Errorf("ConfigType = %s, want agent", version.ConfigType)
		}
	})

	t.Run("ListVersions", func(t *testing.T) {
		// 保存另一个版本
		agentConfig.LLM.Model = "gpt-4o"
		manager.SaveVersion(agentConfig, "agent", "Updated model")

		versions := manager.ListVersions(0)
		if len(versions) < 2 {
			t.Errorf("Version count = %d, want >= 2", len(versions))
		}

		// 测试限制数量
		limitedVersions := manager.ListVersions(1)
		if len(limitedVersions) != 1 {
			t.Errorf("Limited version count = %d, want 1", len(limitedVersions))
		}
	})

	t.Run("TagVersion", func(t *testing.T) {
		versions := manager.ListVersions(1)
		if len(versions) == 0 {
			t.Fatal("No versions available")
		}

		version := versions[0]

		// 添加标签
		if err := manager.TagVersion(version.ID, "production", "stable"); err != nil {
			t.Errorf("Failed to tag version: %v", err)
		}

		// 验证标签
		tagged, err := manager.GetVersion(version.ID)
		if err != nil {
			t.Fatalf("Failed to get version: %v", err)
		}

		if len(tagged.Tags) < 2 {
			t.Errorf("Tag count = %d, want >= 2", len(tagged.Tags))
		}
	})

	t.Run("ListVersionsByTag", func(t *testing.T) {
		versions := manager.ListVersionsByTag("production")
		if len(versions) == 0 {
			t.Error("No versions with 'production' tag found")
		}
	})

	t.Run("RollbackToVersion", func(t *testing.T) {
		versions := manager.ListVersions(0)
		if len(versions) < 2 {
			t.Skip("Need at least 2 versions for rollback test")
		}

		// 回滚到旧版本
		oldVersion := versions[len(versions)-1]
		targetPath := filepath.Join(tmpDir, "rollback-test.yaml")

		restoredConfig, err := manager.RollbackToVersion(oldVersion.ID, targetPath)
		if err != nil {
			t.Fatalf("Failed to rollback: %v", err)
		}

		if restoredConfig == nil {
			t.Error("Restored config is nil")
		}

		// 验证文件已创建
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			t.Error("Rollback file was not created")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		initialCount := manager.Count()

		// 保留 1 个版本
		deleted, err := manager.Cleanup(1)
		if err != nil {
			t.Fatalf("Failed to cleanup: %v", err)
		}

		if deleted != initialCount-1 {
			t.Errorf("Deleted = %d, want %d", deleted, initialCount-1)
		}

		if manager.Count() != 1 {
			t.Errorf("Count after cleanup = %d, want 1", manager.Count())
		}
	})
}

// TestDiffConfigs 测试配置差异对比
func TestDiffConfigs(t *testing.T) {
	t.Run("AgentConfigDiff", func(t *testing.T) {
		oldConfig := &config.AgentConfig{
			Name: "test-agent",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
			MaxIterations: 10,
		}

		newConfig := &config.AgentConfig{
			Name: "test-agent",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4o", // 修改
			},
			MaxIterations: 20, // 修改
		}

		result, err := config.DiffAgentConfigs(oldConfig, newConfig)
		if err != nil {
			t.Fatalf("Failed to diff configs: %v", err)
		}

		if !result.HasChanges {
			t.Error("Should have changes")
		}

		if result.ModifiedCount != 2 {
			t.Errorf("ModifiedCount = %d, want 2", result.ModifiedCount)
		}

		// 检查具体差异
		modelChanged := false
		iterationsChanged := false

		for _, d := range result.Diffs {
			if d.Path == "llm.model" && d.Type == config.DiffTypeModified {
				modelChanged = true
			}
			if d.Path == "max_iterations" && d.Type == config.DiffTypeModified {
				iterationsChanged = true
			}
		}

		if !modelChanged {
			t.Error("Model change not detected")
		}
		if !iterationsChanged {
			t.Error("Iterations change not detected")
		}
	})

	t.Run("NoChanges", func(t *testing.T) {
		config1 := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
		}

		config2 := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
		}

		result, err := config.DiffAgentConfigs(config1, config2)
		if err != nil {
			t.Fatalf("Failed to diff configs: %v", err)
		}

		if result.HasChanges {
			t.Error("Should have no changes")
		}
	})

	t.Run("FormatDiff", func(t *testing.T) {
		oldConfig := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
		}

		newConfig := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4o",
			},
		}

		result, _ := config.DiffAgentConfigs(oldConfig, newConfig)

		formatted := result.Format()
		if formatted == "" {
			t.Error("Formatted output is empty")
		}

		compact := result.FormatCompact()
		if compact == "" {
			t.Error("Compact output is empty")
		}
	})

	t.Run("SummarizeAgentDiff", func(t *testing.T) {
		oldConfig := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
		}

		newConfig := &config.AgentConfig{
			Name: "test",
			LLM: config.LLMConfig{
				Provider: "deepseek",
				Model:    "deepseek-chat",
			},
		}

		summary, err := config.SummarizeAgentDiff(oldConfig, newConfig)
		if err != nil {
			t.Fatalf("Failed to summarize diff: %v", err)
		}

		if summary.ConfigType != "agent" {
			t.Errorf("ConfigType = %s, want agent", summary.ConfigType)
		}

		if len(summary.KeyChanges) == 0 {
			t.Error("KeyChanges is empty")
		}
	})
}

// TestEnvironmentManager 测试环境配置管理器
func TestEnvironmentManager(t *testing.T) {
	tmpDir := t.TempDir()

	manager := config.NewEnvironmentManager(
		config.WithEnvironmentBaseDir(tmpDir),
	)

	t.Run("InitializeEnvironments", func(t *testing.T) {
		if err := manager.InitializeEnvironments(); err != nil {
			t.Fatalf("Failed to initialize environments: %v", err)
		}

		// 验证目录已创建
		envs := []string{"base", "development", "test", "staging", "production"}
		for _, env := range envs {
			dir := filepath.Join(tmpDir, env)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("Environment directory %s was not created", env)
			}
		}
	})

	t.Run("ListEnvironments", func(t *testing.T) {
		envs, err := manager.ListEnvironments()
		if err != nil {
			t.Fatalf("Failed to list environments: %v", err)
		}

		if len(envs) == 0 {
			t.Error("No environments found")
		}
	})

	t.Run("SaveAndLoad", func(t *testing.T) {
		// 保存 base 配置
		baseConfig := &config.AgentConfig{
			Name: "test-agent",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
			MaxIterations: 10,
		}

		if err := manager.SaveConfig("base", "test-agent", baseConfig); err != nil {
			t.Fatalf("Failed to save base config: %v", err)
		}

		// 保存 development 环境配置
		devConfig := &config.AgentConfig{
			Name: "test-agent",
			LLM: config.LLMConfig{
				Model: "gpt-4o", // 只覆盖 model
			},
			MaxIterations: 5, // 覆盖 max_iterations
		}

		if err := manager.SaveConfig("development", "test-agent", devConfig); err != nil {
			t.Fatalf("Failed to save dev config: %v", err)
		}

		// 加载配置（应该合并 base 和 development）
		loadedConfig, err := manager.LoadAgentConfig("test-agent")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// 验证配置合并
		if loadedConfig.LLM.Provider != "openai" {
			t.Errorf("Provider = %s, want openai (from base)", loadedConfig.LLM.Provider)
		}
		if loadedConfig.LLM.Model != "gpt-4o" {
			t.Errorf("Model = %s, want gpt-4o (from dev)", loadedConfig.LLM.Model)
		}
		if loadedConfig.MaxIterations != 5 {
			t.Errorf("MaxIterations = %d, want 5 (from dev)", loadedConfig.MaxIterations)
		}
	})

	t.Run("SwitchEnvironment", func(t *testing.T) {
		if err := manager.SwitchEnvironment("test"); err != nil {
			t.Fatalf("Failed to switch environment: %v", err)
		}

		if manager.CurrentEnvironment() != "test" {
			t.Errorf("CurrentEnvironment = %s, want test", manager.CurrentEnvironment())
		}

		// 尝试切换到不存在的环境
		if err := manager.SwitchEnvironment("nonexistent"); err == nil {
			t.Error("Expected error when switching to nonexistent environment")
		}
	})

	t.Run("CopyConfig", func(t *testing.T) {
		// 创建源配置
		srcConfig := &config.AgentConfig{
			Name: "copy-test",
			LLM: config.LLMConfig{
				Provider: "openai",
				Model:    "gpt-4",
			},
		}

		manager.SaveConfig("development", "copy-test", srcConfig)

		// 复制到 test 环境
		if err := manager.CopyConfig("copy-test", "development", "test"); err != nil {
			t.Fatalf("Failed to copy config: %v", err)
		}

		// 验证复制成功
		copiedPath := filepath.Join(tmpDir, "test", "copy-test.yaml")
		if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
			t.Error("Copied config file does not exist")
		}
	})

	t.Run("ValidateEnvironmentSetup", func(t *testing.T) {
		issues, err := manager.ValidateEnvironmentSetup("development")
		if err != nil {
			t.Fatalf("Failed to validate environment: %v", err)
		}

		// 应该没有问题（或很少问题）
		if len(issues) > 0 {
			t.Logf("Validation issues: %v", issues)
		}
	})
}

// BenchmarkDiffConfigs 配置对比性能基准
func BenchmarkDiffConfigs(b *testing.B) {
	oldConfig := &config.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
		MaxIterations: 10,
		Tools: []config.ToolConfig{
			{Name: "tool1", Type: "builtin"},
			{Name: "tool2", Type: "builtin"},
		},
	}

	newConfig := &config.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o",
		},
		MaxIterations: 20,
		Tools: []config.ToolConfig{
			{Name: "tool1", Type: "builtin"},
			{Name: "tool3", Type: "builtin"},
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = config.DiffAgentConfigs(oldConfig, newConfig)
	}
}

// BenchmarkVersionManager 版本管理性能基准
func BenchmarkVersionManager(b *testing.B) {
	tmpDir := b.TempDir()

	manager, err := config.NewVersionManager(
		config.WithStorePath(filepath.Join(tmpDir, "versions")),
	)
	if err != nil {
		b.Fatalf("Failed to create version manager: %v", err)
	}

	agentConfig := &config.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// 修改配置触发版本保存
		agentConfig.MaxIterations = i
		manager.SaveVersion(agentConfig, "agent", "Benchmark version")
	}
}

// TestIntegration 集成测试：完整的配置管理流程
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// 1. 初始化环境管理器
	envManager := config.NewEnvironmentManager(
		config.WithEnvironmentBaseDir(tmpDir),
	)

	if err := envManager.InitializeEnvironments(); err != nil {
		t.Fatalf("Failed to initialize environments: %v", err)
	}

	// 2. 创建版本管理器
	versionManager, err := config.NewVersionManager(
		config.WithStorePath(filepath.Join(tmpDir, "versions")),
	)
	if err != nil {
		t.Fatalf("Failed to create version manager: %v", err)
	}

	// 3. 创建初始配置
	initialConfig := &config.AgentConfig{
		Name: "prod-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4",
		},
		MaxIterations: 10,
	}

	// 4. 保存到 production 环境
	if err := envManager.SaveConfig("production", "prod-agent", initialConfig); err != nil {
		t.Fatalf("Failed to save production config: %v", err)
	}

	// 5. 创建版本
	v1, err := versionManager.SaveVersion(initialConfig, "agent", "Initial production version", "production", "v1.0")
	if err != nil {
		t.Fatalf("Failed to save version: %v", err)
	}

	t.Logf("Created version: %s", v1.ID)

	// 6. 修改配置
	updatedConfig := &config.AgentConfig{
		Name: "prod-agent",
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o", // 升级模型
		},
		MaxIterations: 15,
	}

	// 7. 对比配置差异
	diff, err := config.DiffAgentConfigs(initialConfig, updatedConfig)
	if err != nil {
		t.Fatalf("Failed to diff configs: %v", err)
	}

	t.Logf("Diff summary: %s", diff.FormatCompact())

	// 8. 保存新版本
	v2, err := versionManager.SaveVersion(updatedConfig, "agent", "Upgraded to GPT-4o", "production", "v1.1")
	if err != nil {
		t.Fatalf("Failed to save version: %v", err)
	}

	t.Logf("Created version: %s", v2.ID)

	// 9. 列出所有版本
	versions := versionManager.ListVersions(0)
	if len(versions) != 2 {
		t.Errorf("Version count = %d, want 2", len(versions))
	}

	// 10. 回滚到 v1
	rollbackPath := filepath.Join(tmpDir, "rollback.yaml")
	if _, err := versionManager.RollbackToVersion(v1.ID, rollbackPath); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	t.Logf("Rolled back to version %s", v1.ID)

	// 11. 验证回滚结果
	rolledBackConfig, err := config.LoadAgentConfig(rollbackPath)
	if err != nil {
		t.Fatalf("Failed to load rolled back config: %v", err)
	}

	if rolledBackConfig.LLM.Model != "gpt-4" {
		t.Errorf("Rolled back model = %s, want gpt-4", rolledBackConfig.LLM.Model)
	}

	t.Log("Integration test completed successfully")
}

// testWithContext 辅助函数：测试带超时的操作
func testWithContext(t *testing.T, timeout time.Duration, fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Operation failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Operation timed out")
	}
}
