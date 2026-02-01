// Package plugin 提供 Hexagon AI Agent 框架的插件系统
package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// Registry 插件注册表
type Registry struct {
	mu        sync.RWMutex
	plugins   map[string]*PluginInstance
	factories map[string]PluginFactory
	handlers  []PluginEventHandler
}

// NewRegistry 创建插件注册表
func NewRegistry() *Registry {
	return &Registry{
		plugins:   make(map[string]*PluginInstance),
		factories: make(map[string]PluginFactory),
		handlers:  make([]PluginEventHandler, 0),
	}
}

// RegisterFactory 注册插件工厂
func (r *Registry) RegisterFactory(name string, factory PluginFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("plugin factory %s already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// UnregisterFactory 注销插件工厂
func (r *Registry) UnregisterFactory(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, name)
}

// Register 注册插件实例
func (r *Registry) Register(plugin Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := plugin.Info()
	if _, exists := r.plugins[info.Name]; exists {
		return fmt.Errorf("plugin %s already registered", info.Name)
	}

	r.plugins[info.Name] = &PluginInstance{
		Plugin: plugin,
		State:  PluginStateLoaded,
	}

	r.emitEvent(PluginEvent{
		Type:   PluginEventLoaded,
		Plugin: info.Name,
	})

	return nil
}

// Unregister 注销插件
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// 如果插件正在运行，不允许注销
	if instance.State == PluginStateRunning {
		return fmt.Errorf("cannot unregister running plugin %s", name)
	}

	delete(r.plugins, name)

	r.emitEvent(PluginEvent{
		Type:   PluginEventUnloaded,
		Plugin: name,
	})

	return nil
}

// Get 获取插件
func (r *Registry) Get(name string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return instance.Plugin, nil
}

// GetInstance 获取插件实例
func (r *Registry) GetInstance(name string) (*PluginInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return instance, nil
}

// Has 检查插件是否存在
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.plugins[name]
	return exists
}

// List 列出所有插件
func (r *Registry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(r.plugins))
	for _, instance := range r.plugins {
		infos = append(infos, instance.Plugin.Info())
	}

	// 按名称排序
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos
}

// ListByType 按类型列出插件
func (r *Registry) ListByType(pluginType PluginType) []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0)
	for _, instance := range r.plugins {
		info := instance.Plugin.Info()
		if info.Type == pluginType {
			infos = append(infos, info)
		}
	}

	return infos
}

// ListByState 按状态列出插件
func (r *Registry) ListByState(state PluginState) []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0)
	for _, instance := range r.plugins {
		if instance.State == state {
			infos = append(infos, instance.Plugin.Info())
		}
	}

	return infos
}

// ListFactories 列出所有工厂
func (r *Registry) ListFactories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// CreateFromFactory 从工厂创建插件
func (r *Registry) CreateFromFactory(name string) (Plugin, error) {
	r.mu.RLock()
	factory, exists := r.factories[name]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin factory %s not found", name)
	}

	return factory(), nil
}

// Count 返回插件数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.plugins)
}

// OnEvent 注册事件处理器
func (r *Registry) OnEvent(handler PluginEventHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers = append(r.handlers, handler)
}

func (r *Registry) emitEvent(event PluginEvent) {
	for _, handler := range r.handlers {
		go handler(event)
	}
}

// SetState 设置插件状态
func (r *Registry) SetState(name string, state PluginState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	instance.State = state
	return nil
}

// ============== Global Registry ==============

var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
)

// Global 返回全局插件注册表
func Global() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
	})
	return globalRegistry
}

// Register 注册插件到全局注册表
func RegisterPlugin(plugin Plugin) error {
	return Global().Register(plugin)
}

// RegisterFactory 注册插件工厂到全局注册表
func RegisterFactoryGlobal(name string, factory PluginFactory) error {
	return Global().RegisterFactory(name, factory)
}

// GetPlugin 从全局注册表获取插件
func GetPlugin(name string) (Plugin, error) {
	return Global().Get(name)
}

// ListPlugins 列出全局注册表中的所有插件
func ListPlugins() []PluginInfo {
	return Global().List()
}
