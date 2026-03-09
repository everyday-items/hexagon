// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件实现配置系统的 Mock：
//   - MockConfigProvider: 模拟配置提供者，支持多种配置类型的存取
//
// 注意：为避免与 config/agent 包的循环导入，本 Mock 使用 map[string]any
// 存储配置数据，而非直接引用 config.AgentConfig 等类型。
// 在测试中可以存储任意配置对象，取出时使用类型断言转换。
package mock

import (
	"sync"
)

// MockConfigProvider 配置提供者 Mock
//
// 提供内存中的配置存储，用于测试中模拟配置系统。
// 使用 map[string]any 存储配置，避免对 config 包的循环依赖。
// 支持按命名空间分类存储配置（如 "agent"、"team"、"workflow"）。
//
// 线程安全：所有方法都使用读写锁保护。
type MockConfigProvider struct {
	// configs 按命名空间分类存储配置
	// 键的格式为 "namespace:name"，如 "agent:assistant"
	configs map[string]any
	mu      sync.RWMutex
}

// NewMockConfigProvider 创建配置提供者 Mock
func NewMockConfigProvider() *MockConfigProvider {
	return &MockConfigProvider{
		configs: make(map[string]any),
	}
}

// Set 存储配置
//
// namespace 为配置命名空间（如 "agent"、"team"、"workflow"）
// name 为配置名称
// value 为配置值（可以是任意类型，如 *config.AgentConfig）
func (m *MockConfigProvider) Set(namespace, name string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[namespace+":"+name] = value
}

// Get 获取配置
//
// 返回配置值和是否存在的标志。
// 调用方需要使用类型断言将返回值转换为目标类型。
func (m *MockConfigProvider) Get(namespace, name string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.configs[namespace+":"+name]
	return v, ok
}

// Remove 移除配置
func (m *MockConfigProvider) Remove(namespace, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.configs, namespace+":"+name)
}

// List 列出指定命名空间下的所有配置名称
func (m *MockConfigProvider) List(namespace string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := namespace + ":"
	names := make([]string, 0)
	for key := range m.configs {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			names = append(names, key[len(prefix):])
		}
	}
	return names
}

// Count 返回配置总数
func (m *MockConfigProvider) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.configs)
}

// CountByNamespace 返回指定命名空间下的配置数量
func (m *MockConfigProvider) CountByNamespace(namespace string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := namespace + ":"
	count := 0
	for key := range m.configs {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}

// Reset 重置所有配置
func (m *MockConfigProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs = make(map[string]any)
}
