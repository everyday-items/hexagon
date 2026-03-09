// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件包含 MockConfigProvider 的全面测试：
//   - 基本操作: Set、Get、Remove、List
//   - 命名空间: 按命名空间分类存储和查询
//   - 计数: 总数和按命名空间计数
//   - 并发安全: 多 goroutine 并发操作
package mock

import (
	"fmt"
	"sync"
	"testing"
)

// TestNewMockConfigProvider 测试创建配置提供者
func TestNewMockConfigProvider(t *testing.T) {
	p := NewMockConfigProvider()

	if p.Count() != 0 {
		t.Errorf("期望初始配置数量为 0，实际为 %d", p.Count())
	}
}

// TestMockConfigProviderSetAndGet 测试设置和获取配置
func TestMockConfigProviderSetAndGet(t *testing.T) {
	p := NewMockConfigProvider()

	type testConfig struct {
		Name        string
		Description string
	}

	cfg := &testConfig{
		Name:        "researcher",
		Description: "研究助手",
	}

	p.Set("agent", "researcher", cfg)

	// 获取存在的配置
	got, ok := p.Get("agent", "researcher")
	if !ok {
		t.Fatal("期望找到已添加的配置")
	}
	tc, ok := got.(*testConfig)
	if !ok {
		t.Fatal("类型断言失败")
	}
	if tc.Name != "researcher" {
		t.Errorf("期望名称为 'researcher'，实际为 '%s'", tc.Name)
	}

	// 获取不存在的配置
	_, ok = p.Get("agent", "nonexistent")
	if ok {
		t.Fatal("期望获取不存在的配置返回 false")
	}
}

// TestMockConfigProviderRemove 测试移除配置
func TestMockConfigProviderRemove(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "test", map[string]string{"name": "test"})

	// 移除
	p.Remove("agent", "test")

	_, ok := p.Get("agent", "test")
	if ok {
		t.Fatal("期望移除后获取返回 false")
	}

	if p.Count() != 0 {
		t.Errorf("期望移除后数量为 0，实际为 %d", p.Count())
	}
}

// TestMockConfigProviderList 测试列出配置名称
func TestMockConfigProviderList(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "agent1", "config1")
	p.Set("agent", "agent2", "config2")
	p.Set("agent", "agent3", "config3")
	p.Set("team", "team1", "teamconfig1")

	names := p.List("agent")
	if len(names) != 3 {
		t.Errorf("期望 3 个 agent 配置，实际为 %d", len(names))
	}

	// 验证所有名称都在
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}
	for _, expected := range []string{"agent1", "agent2", "agent3"} {
		if !nameSet[expected] {
			t.Errorf("期望列表中包含 '%s'", expected)
		}
	}

	// team 命名空间应该只有 1 个
	teamNames := p.List("team")
	if len(teamNames) != 1 {
		t.Errorf("期望 1 个 team 配置，实际为 %d", len(teamNames))
	}
}

// TestMockConfigProviderNamespaces 测试多命名空间
func TestMockConfigProviderNamespaces(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "a1", "agent-config")
	p.Set("team", "t1", "team-config")
	p.Set("workflow", "w1", "workflow-config")

	if p.CountByNamespace("agent") != 1 {
		t.Errorf("期望 agent 命名空间有 1 个配置")
	}
	if p.CountByNamespace("team") != 1 {
		t.Errorf("期望 team 命名空间有 1 个配置")
	}
	if p.CountByNamespace("workflow") != 1 {
		t.Errorf("期望 workflow 命名空间有 1 个配置")
	}
	if p.Count() != 3 {
		t.Errorf("期望总共 3 个配置，实际为 %d", p.Count())
	}
}

// TestMockConfigProviderCountByNamespace 测试按命名空间计数
func TestMockConfigProviderCountByNamespace(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "a1", "v1")
	p.Set("agent", "a2", "v2")
	p.Set("team", "t1", "v3")

	if p.CountByNamespace("agent") != 2 {
		t.Errorf("期望 2 个 agent 配置，实际为 %d", p.CountByNamespace("agent"))
	}
	if p.CountByNamespace("team") != 1 {
		t.Errorf("期望 1 个 team 配置，实际为 %d", p.CountByNamespace("team"))
	}
	if p.CountByNamespace("nonexistent") != 0 {
		t.Errorf("期望不存在的命名空间有 0 个配置")
	}
}

// TestMockConfigProviderReset 测试重置
func TestMockConfigProviderReset(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "a1", "v1")
	p.Set("team", "t1", "v2")
	p.Set("workflow", "w1", "v3")

	p.Reset()

	if p.Count() != 0 {
		t.Errorf("期望重置后配置数为 0，实际为 %d", p.Count())
	}
}

// TestMockConfigProviderOverwrite 测试覆盖配置
func TestMockConfigProviderOverwrite(t *testing.T) {
	p := NewMockConfigProvider()

	p.Set("agent", "test", "v1")
	p.Set("agent", "test", "v2")

	got, _ := p.Get("agent", "test")
	if got != "v2" {
		t.Errorf("期望覆盖后值为 'v2'，实际为 '%v'", got)
	}

	if p.Count() != 1 {
		t.Errorf("期望覆盖后仍为 1 个配置，实际为 %d", p.Count())
	}
}

// TestMockConfigProviderConcurrency 测试并发安全
func TestMockConfigProviderConcurrency(t *testing.T) {
	p := NewMockConfigProvider()

	const goroutines = 20
	var wg sync.WaitGroup

	// 并发写入
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("agent-%d", idx)
			p.Set("agent", name, map[string]int{"id": idx})
		}(i)
	}

	wg.Wait()

	if p.CountByNamespace("agent") != goroutines {
		t.Errorf("期望 %d 个配置，实际为 %d", goroutines, p.CountByNamespace("agent"))
	}

	// 并发读取
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("agent-%d", idx)
			_, ok := p.Get("agent", name)
			if !ok {
				t.Errorf("期望能找到配置 '%s'", name)
			}
		}(i)
	}

	wg.Wait()
}
