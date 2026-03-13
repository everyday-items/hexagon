// alert.go 提供指标告警功能
//
// 基于指标规则的告警系统：
//   - 支持多种比较操作符
//   - 三级告警严重度
//   - 告警处理器回调
//   - 周期性评估
//
// 使用示例：
//
//	mgr := metrics.NewAlertManager(m)
//	mgr.AddRule(&metrics.AlertRule{
//	    Name:     "high-latency",
//	    Severity: metrics.SeverityWarning,
//	    Condition: metrics.AlertCondition{
//	        MetricName: "llm.latency",
//	        Operator:   ">",
//	        Threshold:  5.0,
//	    },
//	})
//	mgr.OnAlert(func(a *metrics.Alert) { log.Println(a) })
//	mgr.Start(10 * time.Second)
package metrics

import (
	"fmt"
	"sync"
	"time"
)

// ============== 严重度 ==============

// Severity 告警严重度
type Severity int

const (
	// SeverityInfo 信息级别
	SeverityInfo Severity = iota

	// SeverityWarning 警告级别
	SeverityWarning

	// SeverityCritical 严重级别
	SeverityCritical
)

// String 返回严重度名称
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ============== 告警条件 ==============

// AlertCondition 告警触发条件
type AlertCondition struct {
	// MetricName 指标名称
	MetricName string

	// Operator 比较操作符 (">", "<", ">=", "<=", "==", "!=")
	Operator string

	// Threshold 阈值
	Threshold float64

	// Duration 持续时间（条件必须持续满足多久才触发，0 表示立即）
	Duration time.Duration
}

// floatEpsilon 浮点数比较精度
const floatEpsilon = 1e-9

// evaluate 评估条件是否满足
func (c *AlertCondition) evaluate(value float64) bool {
	diff := value - c.Threshold
	switch c.Operator {
	case ">":
		return value > c.Threshold
	case "<":
		return value < c.Threshold
	case ">=":
		return value >= c.Threshold
	case "<=":
		return value <= c.Threshold
	case "==":
		return diff > -floatEpsilon && diff < floatEpsilon
	case "!=":
		return diff <= -floatEpsilon || diff >= floatEpsilon
	default:
		return false
	}
}

// ============== 告警规则 ==============

// AlertRule 告警规则
type AlertRule struct {
	// Name 规则名称（唯一标识）
	Name string

	// Description 规则描述
	Description string

	// Condition 触发条件
	Condition AlertCondition

	// Severity 严重度
	Severity Severity

	// Labels 标签（用于分类和路由）
	Labels map[string]string

	// Enabled 是否启用
	Enabled bool

	// 内部状态
	triggeredAt time.Time // 条件首次满足的时间
	active      bool      // 当前是否处于告警状态
}

// ============== 告警 ==============

// Alert 告警实例
type Alert struct {
	// Rule 触发的规则
	Rule *AlertRule

	// Value 触发时的指标值
	Value float64

	// TriggeredAt 触发时间
	TriggeredAt time.Time

	// ResolvedAt 恢复时间（nil 表示未恢复）
	ResolvedAt *time.Time
}

// String 返回告警描述
func (a *Alert) String() string {
	status := "FIRING"
	if a.ResolvedAt != nil {
		status = "RESOLVED"
	}
	return fmt.Sprintf("[%s] %s: %s %s %.2f (当前值: %.2f)",
		status, a.Rule.Severity, a.Rule.Name,
		a.Rule.Condition.Operator, a.Rule.Condition.Threshold, a.Value)
}

// ============== 告警处理器 ==============

// AlertHandler 告警处理回调
type AlertHandler func(alert *Alert)

// ============== 告警管理器 ==============

// AlertManager 指标告警管理器
//
// 周期性评估所有告警规则，触发和恢复告警。
// 线程安全。
type AlertManager struct {
	rules    []*AlertRule
	handlers []AlertHandler
	active   []*Alert
	metrics  Metrics
	mu       sync.RWMutex
	stopCh   chan struct{}
	running  bool
}

// NewAlertManager 创建告警管理器
func NewAlertManager(m Metrics) *AlertManager {
	return &AlertManager{
		metrics: m,
	}
}

// AddRule 添加告警规则
func (am *AlertManager) AddRule(rule *AlertRule) error {
	if rule.Name == "" {
		return fmt.Errorf("告警规则名称不能为空")
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// 检查重复
	for _, r := range am.rules {
		if r.Name == rule.Name {
			return fmt.Errorf("告警规则 %s 已存在", rule.Name)
		}
	}

	if !rule.Enabled {
		rule.Enabled = true
	}
	am.rules = append(am.rules, rule)
	return nil
}

// RemoveRule 移除告警规则
func (am *AlertManager) RemoveRule(name string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	for i, r := range am.rules {
		if r.Name == name {
			am.rules = append(am.rules[:i], am.rules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("告警规则 %s 未找到", name)
}

// OnAlert 注册告警处理回调
func (am *AlertManager) OnAlert(handler AlertHandler) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.handlers = append(am.handlers, handler)
}

// Start 启动周期性评估
func (am *AlertManager) Start(interval time.Duration) {
	am.mu.Lock()
	if am.running {
		am.mu.Unlock()
		return
	}
	am.running = true
	am.stopCh = make(chan struct{})
	am.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				am.Evaluate()
			case <-am.stopCh:
				return
			}
		}
	}()
}

// Stop 停止周期性评估
func (am *AlertManager) Stop() {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.running {
		close(am.stopCh)
		am.running = false
	}
}

// Evaluate 手动评估所有规则
//
// 两阶段评估：锁内读取状态和计算，锁外调用 handlers。
// 同时清理已恢复的告警，防止 active 切片无限增长。
func (am *AlertManager) Evaluate() {
	// 阶段 1：在锁内读取指标、评估条件、生成告警
	am.mu.Lock()

	now := time.Now()
	var pendingAlerts []*Alert

	for _, rule := range am.rules {
		if !rule.Enabled {
			continue
		}

		// 获取指标当前值（使用 Gauge 接口获取）
		gauge := am.metrics.Gauge(rule.Condition.MetricName)
		value := gauge.Value()

		conditionMet := rule.Condition.evaluate(value)

		if conditionMet {
			if rule.triggeredAt.IsZero() {
				rule.triggeredAt = now
			}

			// 检查是否满足持续时间要求
			elapsed := now.Sub(rule.triggeredAt)
			if elapsed >= rule.Condition.Duration && !rule.active {
				// 触发告警
				rule.active = true
				alert := &Alert{
					Rule:        rule,
					Value:       value,
					TriggeredAt: now,
				}
				am.active = append(am.active, alert)
				pendingAlerts = append(pendingAlerts, alert)
			}
		} else {
			if rule.active {
				// 恢复告警
				rule.active = false
				resolvedAt := now
				for _, alert := range am.active {
					if alert.Rule.Name == rule.Name && alert.ResolvedAt == nil {
						alert.ResolvedAt = &resolvedAt
						alert.Value = value
						pendingAlerts = append(pendingAlerts, alert)
					}
				}
			}
			rule.triggeredAt = time.Time{} // 重置
		}
	}

	// 清理已恢复的告警，防止 active 切片无限增长
	n := 0
	for _, alert := range am.active {
		if alert.ResolvedAt == nil {
			am.active[n] = alert
			n++
		}
	}
	// 清除尾部引用，帮助 GC
	for i := n; i < len(am.active); i++ {
		am.active[i] = nil
	}
	am.active = am.active[:n]

	// 复制 handlers 快照
	handlers := make([]AlertHandler, len(am.handlers))
	copy(handlers, am.handlers)

	am.mu.Unlock()

	// 阶段 2：在锁外通知 handlers
	for _, alert := range pendingAlerts {
		for _, handler := range handlers {
			func() {
				defer func() { recover() }()
				handler(alert)
			}()
		}
	}
}

// ActiveAlerts 返回当前活跃的告警
func (am *AlertManager) ActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*Alert, 0, len(am.active))
	for _, alert := range am.active {
		if alert.ResolvedAt == nil {
			result = append(result, alert)
		}
	}
	return result
}
