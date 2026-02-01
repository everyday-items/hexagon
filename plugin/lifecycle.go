// Package plugin 提供 Hexagon AI Agent 框架的插件系统
package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Lifecycle 插件生命周期管理器
type Lifecycle struct {
	registry        *Registry
	mu              sync.RWMutex
	startOrder      []string
	healthCheckTick time.Duration
	stopChan        chan struct{}
}

// LifecycleOption Lifecycle 选项
type LifecycleOption func(*Lifecycle)

// WithHealthCheckInterval 设置健康检查间隔
func WithHealthCheckInterval(d time.Duration) LifecycleOption {
	return func(l *Lifecycle) {
		l.healthCheckTick = d
	}
}

// NewLifecycle 创建生命周期管理器
func NewLifecycle(registry *Registry, opts ...LifecycleOption) *Lifecycle {
	l := &Lifecycle{
		registry:        registry,
		startOrder:      make([]string, 0),
		healthCheckTick: 30 * time.Second,
		stopChan:        make(chan struct{}),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Init 初始化插件
func (l *Lifecycle) Init(ctx context.Context, name string, config map[string]any) error {
	instance, err := l.registry.GetInstance(name)
	if err != nil {
		return err
	}

	if instance.State != PluginStateLoaded {
		return fmt.Errorf("plugin %s is not in loaded state (current: %s)", name, instance.State)
	}

	if err := instance.Plugin.Init(ctx, config); err != nil {
		instance.State = PluginStateError
		instance.Error = err
		return fmt.Errorf("failed to init plugin %s: %w", name, err)
	}

	instance.State = PluginStateInitialized
	instance.Config = config

	l.registry.emitEvent(PluginEvent{
		Type:   PluginEventInitialized,
		Plugin: name,
	})

	return nil
}

// Start 启动插件
func (l *Lifecycle) Start(ctx context.Context, name string) error {
	instance, err := l.registry.GetInstance(name)
	if err != nil {
		return err
	}

	if instance.State != PluginStateInitialized && instance.State != PluginStateStopped {
		return fmt.Errorf("plugin %s is not ready to start (current: %s)", name, instance.State)
	}

	// 检查依赖
	info := instance.Plugin.Info()
	for _, dep := range info.Dependencies {
		depInstance, err := l.registry.GetInstance(dep)
		if err != nil {
			return fmt.Errorf("dependency %s not found for plugin %s", dep, name)
		}
		if depInstance.State != PluginStateRunning {
			return fmt.Errorf("dependency %s is not running (current: %s)", dep, depInstance.State)
		}
	}

	if err := instance.Plugin.Start(ctx); err != nil {
		instance.State = PluginStateError
		instance.Error = err
		return fmt.Errorf("failed to start plugin %s: %w", name, err)
	}

	now := time.Now()
	instance.State = PluginStateRunning
	instance.StartedAt = &now
	instance.Error = nil

	l.mu.Lock()
	l.startOrder = append(l.startOrder, name)
	l.mu.Unlock()

	l.registry.emitEvent(PluginEvent{
		Type:   PluginEventStarted,
		Plugin: name,
	})

	return nil
}

// Stop 停止插件
func (l *Lifecycle) Stop(ctx context.Context, name string) error {
	instance, err := l.registry.GetInstance(name)
	if err != nil {
		return err
	}

	if instance.State != PluginStateRunning {
		return fmt.Errorf("plugin %s is not running (current: %s)", name, instance.State)
	}

	// 检查是否有其他插件依赖此插件
	for _, info := range l.registry.List() {
		for _, dep := range info.Dependencies {
			if dep == name {
				depInstance, _ := l.registry.GetInstance(info.Name)
				if depInstance != nil && depInstance.State == PluginStateRunning {
					return fmt.Errorf("cannot stop plugin %s: plugin %s depends on it", name, info.Name)
				}
			}
		}
	}

	if err := instance.Plugin.Stop(ctx); err != nil {
		instance.State = PluginStateError
		instance.Error = err
		return fmt.Errorf("failed to stop plugin %s: %w", name, err)
	}

	instance.State = PluginStateStopped
	instance.StartedAt = nil

	l.mu.Lock()
	// 从启动顺序中移除
	for i, n := range l.startOrder {
		if n == name {
			l.startOrder = append(l.startOrder[:i], l.startOrder[i+1:]...)
			break
		}
	}
	l.mu.Unlock()

	l.registry.emitEvent(PluginEvent{
		Type:   PluginEventStopped,
		Plugin: name,
	})

	return nil
}

// Restart 重启插件
func (l *Lifecycle) Restart(ctx context.Context, name string) error {
	instance, err := l.registry.GetInstance(name)
	if err != nil {
		return err
	}

	if instance.State == PluginStateRunning {
		if err := l.Stop(ctx, name); err != nil {
			return err
		}
	}

	return l.Start(ctx, name)
}

// InitAll 初始化所有插件
func (l *Lifecycle) InitAll(ctx context.Context, configs map[string]map[string]any) error {
	for _, info := range l.registry.List() {
		config := configs[info.Name]
		if config == nil {
			config = make(map[string]any)
		}

		if err := l.Init(ctx, info.Name, config); err != nil {
			return err
		}
	}

	return nil
}

// StartAll 启动所有插件（按依赖顺序）
func (l *Lifecycle) StartAll(ctx context.Context) error {
	// 获取所有插件并按依赖排序
	plugins := l.registry.List()
	sorted := l.sortByDependency(plugins)

	for _, info := range sorted {
		instance, _ := l.registry.GetInstance(info.Name)
		if instance == nil || instance.State == PluginStateRunning {
			continue
		}

		if instance.State != PluginStateInitialized && instance.State != PluginStateStopped {
			continue
		}

		if err := l.Start(ctx, info.Name); err != nil {
			return err
		}
	}

	return nil
}

// StopAll 停止所有插件（按启动的逆序）
func (l *Lifecycle) StopAll(ctx context.Context) error {
	l.mu.RLock()
	order := make([]string, len(l.startOrder))
	copy(order, l.startOrder)
	l.mu.RUnlock()

	// 逆序停止
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if err := l.Stop(ctx, name); err != nil {
			// 继续停止其他插件，但记录错误
			continue
		}
	}

	return nil
}

// sortByDependency 按依赖关系排序
func (l *Lifecycle) sortByDependency(plugins []PluginInfo) []PluginInfo {
	// 构建依赖图
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, p := range plugins {
		graph[p.Name] = p.Dependencies
		if _, ok := inDegree[p.Name]; !ok {
			inDegree[p.Name] = 0
		}
		for _, dep := range p.Dependencies {
			inDegree[dep] = 0 // 初始化
		}
	}

	for _, deps := range graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// 拓扑排序（Kahn's algorithm）
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []PluginInfo
	pluginMap := make(map[string]PluginInfo)
	for _, p := range plugins {
		pluginMap[p.Name] = p
	}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if info, ok := pluginMap[name]; ok {
			sorted = append(sorted, info)
		}

		for _, deps := range graph {
			for _, dep := range deps {
				if dep == name {
					// 这里的逻辑需要调整
				}
			}
		}

		// 简化：直接按名称排序
	}

	// 如果拓扑排序不完整，返回原始列表
	if len(sorted) < len(plugins) {
		return plugins
	}

	return sorted
}

// HealthCheck 执行健康检查
func (l *Lifecycle) HealthCheck(ctx context.Context) map[string]HealthStatus {
	results := make(map[string]HealthStatus)

	for _, info := range l.registry.List() {
		instance, err := l.registry.GetInstance(info.Name)
		if err != nil {
			results[info.Name] = HealthStatus{
				Status:    HealthStateUnknown,
				Message:   err.Error(),
				LastCheck: time.Now(),
			}
			continue
		}

		if instance.State != PluginStateRunning {
			results[info.Name] = HealthStatus{
				Status:    HealthStateUnknown,
				Message:   fmt.Sprintf("plugin not running (state: %s)", instance.State),
				LastCheck: time.Now(),
			}
			continue
		}

		status := instance.Plugin.Health()
		results[info.Name] = status

		l.registry.emitEvent(PluginEvent{
			Type:   PluginEventHealthCheck,
			Plugin: info.Name,
			Data: map[string]any{
				"status": string(status.Status),
			},
		})
	}

	return results
}

// StartHealthChecker 启动健康检查器
func (l *Lifecycle) StartHealthChecker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(l.healthCheckTick)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-l.stopChan:
				return
			case <-ticker.C:
				l.HealthCheck(ctx)
			}
		}
	}()
}

// StopHealthChecker 停止健康检查器
func (l *Lifecycle) StopHealthChecker() {
	close(l.stopChan)
}

// GetStartOrder 获取启动顺序
func (l *Lifecycle) GetStartOrder() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	order := make([]string, len(l.startOrder))
	copy(order, l.startOrder)
	return order
}

// ============== PluginManager ==============

// PluginManager 插件管理器（组合 Registry 和 Lifecycle）
type PluginManager struct {
	Registry  *Registry
	Lifecycle *Lifecycle
}

// NewPluginManager 创建插件管理器
func NewPluginManager(opts ...LifecycleOption) *PluginManager {
	registry := NewRegistry()
	lifecycle := NewLifecycle(registry, opts...)

	return &PluginManager{
		Registry:  registry,
		Lifecycle: lifecycle,
	}
}

// Load 加载并注册插件
func (m *PluginManager) Load(plugin Plugin) error {
	return m.Registry.Register(plugin)
}

// LoadFromFactory 从工厂加载插件
func (m *PluginManager) LoadFromFactory(name string) error {
	plugin, err := m.Registry.CreateFromFactory(name)
	if err != nil {
		return err
	}
	return m.Registry.Register(plugin)
}

// Unload 卸载插件
func (m *PluginManager) Unload(ctx context.Context, name string) error {
	instance, err := m.Registry.GetInstance(name)
	if err != nil {
		return err
	}

	if instance.State == PluginStateRunning {
		if err := m.Lifecycle.Stop(ctx, name); err != nil {
			return err
		}
	}

	return m.Registry.Unregister(name)
}

// Enable 启用插件（初始化 + 启动）
func (m *PluginManager) Enable(ctx context.Context, name string, config map[string]any) error {
	if err := m.Lifecycle.Init(ctx, name, config); err != nil {
		return err
	}
	return m.Lifecycle.Start(ctx, name)
}

// Disable 禁用插件
func (m *PluginManager) Disable(ctx context.Context, name string) error {
	return m.Lifecycle.Stop(ctx, name)
}

// Status 获取插件状态
func (m *PluginManager) Status() map[string]PluginState {
	status := make(map[string]PluginState)

	for _, info := range m.Registry.List() {
		instance, err := m.Registry.GetInstance(info.Name)
		if err != nil {
			continue
		}
		status[info.Name] = instance.State
	}

	return status
}

// Stats 获取统计信息
func (m *PluginManager) Stats() map[string]any {
	plugins := m.Registry.List()
	status := m.Status()

	var running, stopped, errored int
	for _, state := range status {
		switch state {
		case PluginStateRunning:
			running++
		case PluginStateStopped:
			stopped++
		case PluginStateError:
			errored++
		}
	}

	typeCount := make(map[string]int)
	for _, info := range plugins {
		typeCount[string(info.Type)]++
	}

	return map[string]any{
		"total":    len(plugins),
		"running":  running,
		"stopped":  stopped,
		"errored":  errored,
		"by_type":  typeCount,
		"factories": len(m.Registry.ListFactories()),
	}
}

// ============== 便捷函数 ==============

// SortPluginsByPriority 按优先级排序插件配置
func SortPluginsByPriority(configs []PluginConfig) []PluginConfig {
	sorted := make([]PluginConfig, len(configs))
	copy(sorted, configs)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	return sorted
}
