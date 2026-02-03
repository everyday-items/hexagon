// Package config 提供配置热更新能力
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// HotReloader 配置热更新器
//
// 监控配置文件变化，自动重新加载并触发回调。
// 支持：
//   - 文件变化监控（基于 mtime）
//   - 自动重新加载
//   - 变更通知回调
//   - 优雅错误处理
//
// 线程安全：所有方法都是并发安全的
type HotReloader struct {
	// path 配置文件路径
	path string

	// configType 配置类型（agent/team/workflow）
	configType string

	// interval 检查间隔
	interval time.Duration

	// lastModTime 上次修改时间
	lastModTime time.Time

	// callback 变更回调
	callback ReloadCallback

	// running 运行状态
	running bool

	// ctx 上下文
	ctx context.Context

	// cancel 取消函数
	cancel context.CancelFunc

	// mu 互斥锁
	mu sync.RWMutex

	// errHandler 错误处理器
	errHandler ErrorHandler
}

// ReloadCallback 重新加载回调
//
// 参数：
//   - config: 新配置（类型为 *AgentConfig/*TeamConfig/*WorkflowConfig）
//   - err: 加载错误（如果有）
type ReloadCallback func(config any, err error)

// ErrorHandler 错误处理器
type ErrorHandler func(err error)

// HotReloadOption 热更新选项
type HotReloadOption func(*HotReloader)

// WithInterval 设置检查间隔
func WithInterval(interval time.Duration) HotReloadOption {
	return func(r *HotReloader) {
		r.interval = interval
	}
}

// WithErrorHandler 设置错误处理器
func WithErrorHandler(handler ErrorHandler) HotReloadOption {
	return func(r *HotReloader) {
		r.errHandler = handler
	}
}

// NewHotReloader 创建配置热更新器
//
// 参数：
//   - path: 配置文件路径
//   - configType: 配置类型（agent/team/workflow）
//   - callback: 变更回调
//   - opts: 可选配置
//
// 返回值：
//   - *HotReloader: 热更新器实例
func NewHotReloader(path, configType string, callback ReloadCallback, opts ...HotReloadOption) *HotReloader {
	ctx, cancel := context.WithCancel(context.Background())

	r := &HotReloader{
		path:       path,
		configType: configType,
		interval:   5 * time.Second, // 默认 5 秒检查一次
		callback:   callback,
		ctx:        ctx,
		cancel:     cancel,
		errHandler: func(err error) {
			// 默认错误处理：忽略
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Start 启动热更新监控
//
// 开始监控配置文件变化，当检测到变化时自动重新加载并触发回调。
// 非阻塞，在后台 goroutine 中运行。
func (r *HotReloader) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("hot reloader already running")
	}

	// 获取初始修改时间
	info, err := os.Stat(r.path)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}
	r.lastModTime = info.ModTime()

	r.running = true

	// 启动监控 goroutine
	go r.watch()

	return nil
}

// Stop 停止热更新监控
func (r *HotReloader) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	r.cancel()
	r.running = false
}

// IsRunning 返回运行状态
func (r *HotReloader) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// watch 监控文件变化
func (r *HotReloader) watch() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.checkAndReload()
		}
	}
}

// checkAndReload 检查文件变化并重新加载
func (r *HotReloader) checkAndReload() {
	// 检查文件修改时间
	info, err := os.Stat(r.path)
	if err != nil {
		r.errHandler(fmt.Errorf("failed to stat config file: %w", err))
		return
	}

	// 比较修改时间
	r.mu.RLock()
	lastMod := r.lastModTime
	r.mu.RUnlock()

	if info.ModTime().After(lastMod) {
		// 文件已修改，重新加载
		r.reload(info.ModTime())
	}
}

// reload 重新加载配置
func (r *HotReloader) reload(modTime time.Time) {
	// 读取配置文件
	data, err := os.ReadFile(r.path)
	if err != nil {
		r.callback(nil, fmt.Errorf("failed to read config: %w", err))
		return
	}

	// 解析配置
	var config any
	var parseErr error

	switch r.configType {
	case "agent":
		var agentConfig AgentConfig
		parseErr = yaml.Unmarshal(data, &agentConfig)
		if parseErr == nil {
			config = &agentConfig
		}

	case "team":
		var teamConfig TeamConfig
		parseErr = yaml.Unmarshal(data, &teamConfig)
		if parseErr == nil {
			config = &teamConfig
		}

	case "workflow":
		var workflowConfig WorkflowConfig
		parseErr = yaml.Unmarshal(data, &workflowConfig)
		if parseErr == nil {
			config = &workflowConfig
		}

	default:
		r.callback(nil, fmt.Errorf("unknown config type: %s", r.configType))
		return
	}

	if parseErr != nil {
		r.callback(nil, fmt.Errorf("failed to parse config: %w", parseErr))
		return
	}

	// 更新修改时间
	r.mu.Lock()
	r.lastModTime = modTime
	r.mu.Unlock()

	// 触发回调
	r.callback(config, nil)
}

// Reload 手动触发重新加载
func (r *HotReloader) Reload() error {
	info, err := os.Stat(r.path)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	r.reload(info.ModTime())
	return nil
}

// HotReloadManager 热更新管理器
//
// 管理多个配置文件的热更新。
type HotReloadManager struct {
	// reloaders 热更新器列表
	reloaders map[string]*HotReloader

	// mu 互斥锁
	mu sync.RWMutex
}

// NewHotReloadManager 创建热更新管理器
func NewHotReloadManager() *HotReloadManager {
	return &HotReloadManager{
		reloaders: make(map[string]*HotReloader),
	}
}

// Watch 监控配置文件
//
// 参数：
//   - name: 配置名称（用于标识）
//   - path: 配置文件路径
//   - configType: 配置类型
//   - callback: 变更回调
//   - opts: 可选配置
func (m *HotReloadManager) Watch(name, path, configType string, callback ReloadCallback, opts ...HotReloadOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if _, exists := m.reloaders[name]; exists {
		return fmt.Errorf("config %s already being watched", name)
	}

	// 创建热更新器
	reloader := NewHotReloader(path, configType, callback, opts...)

	// 启动监控
	if err := reloader.Start(); err != nil {
		return err
	}

	m.reloaders[name] = reloader
	return nil
}

// Unwatch 停止监控配置文件
func (m *HotReloadManager) Unwatch(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if reloader, exists := m.reloaders[name]; exists {
		reloader.Stop()
		delete(m.reloaders, name)
	}
}

// WatchDir 监控目录下的所有配置文件
//
// 参数：
//   - dir: 目录路径
//   - callback: 变更回调
//   - opts: 可选配置
//
// 返回值：
//   - []string: 已监控的配置文件名称列表
//   - error: 错误（如果有）
func (m *HotReloadManager) WatchDir(dir string, callback ReloadCallback, opts ...HotReloadOption) ([]string, error) {
	// 查找所有 YAML 文件
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}

	yamlFiles, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	files = append(files, yamlFiles...)

	watched := make([]string, 0)

	for _, file := range files {
		// 读取文件判断类型
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}

		// 判断配置类型
		var configType string
		if _, ok := doc["agents"]; ok {
			configType = "team"
		} else if _, ok := doc["nodes"]; ok {
			configType = "workflow"
		} else if _, ok := doc["llm"]; ok {
			configType = "agent"
		} else {
			continue
		}

		// 使用文件名作为标识
		name := filepath.Base(file)

		// 监控文件
		if err := m.Watch(name, file, configType, callback, opts...); err != nil {
			continue
		}

		watched = append(watched, name)
	}

	return watched, nil
}

// StopAll 停止所有监控
func (m *HotReloadManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, reloader := range m.reloaders {
		reloader.Stop()
	}

	m.reloaders = make(map[string]*HotReloader)
}

// Count 返回监控的配置数量
func (m *HotReloadManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.reloaders)
}

// List 返回所有监控的配置名称
func (m *HotReloadManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.reloaders))
	for name := range m.reloaders {
		names = append(names, name)
	}
	return names
}
