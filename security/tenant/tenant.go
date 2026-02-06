// Package tenant 提供多租户支持
//
// 本包实现多租户隔离和管理：
//   - 租户隔离：数据和资源隔离
//   - 租户配额：资源使用限制
//   - 租户上下文：请求级租户识别
//   - 租户路由：基于租户的请求路由
//
// 设计借鉴：
//   - PostgreSQL: Row Level Security
//   - Kubernetes: Namespace 隔离
//   - AWS: Multi-tenant SaaS
package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============== 租户定义 ==============

// Tenant 租户
type Tenant struct {
	// ID 租户唯一标识
	ID string `json:"id"`

	// Name 租户名称
	Name string `json:"name"`

	// DisplayName 显示名称
	DisplayName string `json:"display_name,omitempty"`

	// Type 租户类型
	Type TenantType `json:"type"`

	// Status 租户状态
	Status TenantStatus `json:"status"`

	// Plan 订阅计划
	Plan string `json:"plan,omitempty"`

	// Quota 配额
	Quota *TenantQuota `json:"quota,omitempty"`

	// Settings 租户设置
	Settings map[string]any `json:"settings,omitempty"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`

	// ExpiresAt 过期时间（可选）
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// TenantType 租户类型
type TenantType string

const (
	// TenantTypeFree 免费租户
	TenantTypeFree TenantType = "free"

	// TenantTypeStandard 标准租户
	TenantTypeStandard TenantType = "standard"

	// TenantTypePremium 高级租户
	TenantTypePremium TenantType = "premium"

	// TenantTypeEnterprise 企业租户
	TenantTypeEnterprise TenantType = "enterprise"
)

// TenantStatus 租户状态
type TenantStatus string

const (
	// TenantStatusActive 活跃
	TenantStatusActive TenantStatus = "active"

	// TenantStatusSuspended 暂停
	TenantStatusSuspended TenantStatus = "suspended"

	// TenantStatusInactive 不活跃
	TenantStatusInactive TenantStatus = "inactive"

	// TenantStatusDeleted 已删除
	TenantStatusDeleted TenantStatus = "deleted"
)

// TenantQuota 租户配额
type TenantQuota struct {
	// MaxRequests 最大请求数/月
	MaxRequests int64 `json:"max_requests"`

	// MaxTokens 最大 Token 数/月
	MaxTokens int64 `json:"max_tokens"`

	// MaxAgents 最大 Agent 数
	MaxAgents int `json:"max_agents"`

	// MaxConcurrency 最大并发数
	MaxConcurrency int `json:"max_concurrency"`

	// MaxStorage 最大存储（字节）
	MaxStorage int64 `json:"max_storage"`

	// RateLimitPerMinute 每分钟请求限制
	RateLimitPerMinute int `json:"rate_limit_per_minute"`

	// CustomQuotas 自定义配额
	CustomQuotas map[string]int64 `json:"custom_quotas,omitempty"`
}

// ============== 租户上下文 ==============

type tenantContextKey struct{}

// WithTenant 将租户信息添加到上下文
func WithTenant(ctx context.Context, tenant *Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenant)
}

// FromContext 从上下文获取租户
func FromContext(ctx context.Context) (*Tenant, bool) {
	tenant, ok := ctx.Value(tenantContextKey{}).(*Tenant)
	return tenant, ok
}

// MustFromContext 从上下文获取租户（必须存在）
//
// ⚠️ 警告：如果上下文中不存在租户，此函数会 panic。
// 仅在确定租户一定存在时使用（如中间件已验证后）。
// 推荐使用 FromContext 进行安全检查，或使用 FromContextOrDefault 提供默认值。
//
// 使用场景：
//   - 在租户中间件之后的处理器中
//   - 在已确认租户存在的业务逻辑中
func MustFromContext(ctx context.Context) *Tenant {
	tenant, ok := FromContext(ctx)
	if !ok {
		panic("tenant not found in context: ensure tenant middleware is applied")
	}
	return tenant
}

// FromContextOrDefault 从上下文获取租户，如果不存在则返回默认租户
// 这是 MustFromContext 的安全替代方案
func FromContextOrDefault(ctx context.Context, defaultTenant *Tenant) *Tenant {
	tenant, ok := FromContext(ctx)
	if !ok {
		return defaultTenant
	}
	return tenant
}

// FromContextOrError 从上下文获取租户，如果不存在则返回错误
// 这是 MustFromContext 的安全替代方案
func FromContextOrError(ctx context.Context) (*Tenant, error) {
	tenant, ok := FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant not found in context")
	}
	return tenant, nil
}

// TenantID 从上下文获取租户 ID
func TenantID(ctx context.Context) string {
	if tenant, ok := FromContext(ctx); ok {
		return tenant.ID
	}
	return ""
}

// ============== 租户管理器 ==============

// Manager 租户管理器
type Manager struct {
	// storage 租户存储
	storage TenantStorage

	// cache 租户缓存
	cache map[string]*Tenant

	// usageTracker 使用量追踪
	usageTracker *UsageTracker

	// quotaEnforcer 配额执行器
	quotaEnforcer *QuotaEnforcer

	mu sync.RWMutex
}

// TenantStorage 租户存储接口
type TenantStorage interface {
	// Get 获取租户
	Get(ctx context.Context, id string) (*Tenant, error)

	// Save 保存租户
	Save(ctx context.Context, tenant *Tenant) error

	// Delete 删除租户
	Delete(ctx context.Context, id string) error

	// List 列出租户
	List(ctx context.Context, filter TenantFilter) ([]*Tenant, error)
}

// TenantFilter 租户过滤器
type TenantFilter struct {
	// Type 类型过滤
	Type TenantType

	// Status 状态过滤
	Status TenantStatus

	// Plan 计划过滤
	Plan string

	// Limit 限制数量
	Limit int

	// Offset 偏移量
	Offset int
}

// NewManager 创建租户管理器
func NewManager(storage TenantStorage) *Manager {
	m := &Manager{
		storage:      storage,
		cache:        make(map[string]*Tenant),
		usageTracker: NewUsageTracker(),
	}
	m.quotaEnforcer = NewQuotaEnforcer(m)
	return m
}

// Create 创建租户
func (m *Manager) Create(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()

	if tenant.Status == "" {
		tenant.Status = TenantStatusActive
	}

	if tenant.Type == "" {
		tenant.Type = TenantTypeFree
	}

	// 设置默认配额
	if tenant.Quota == nil {
		tenant.Quota = DefaultQuotaForType(tenant.Type)
	}

	if err := m.storage.Save(ctx, tenant); err != nil {
		return err
	}

	m.mu.Lock()
	m.cache[tenant.ID] = tenant
	m.mu.Unlock()

	return nil
}

// Get 获取租户
func (m *Manager) Get(ctx context.Context, id string) (*Tenant, error) {
	// 先检查缓存
	m.mu.RLock()
	if tenant, ok := m.cache[id]; ok {
		m.mu.RUnlock()
		return tenant, nil
	}
	m.mu.RUnlock()

	// 从存储加载
	tenant, err := m.storage.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	m.mu.Lock()
	m.cache[id] = tenant
	m.mu.Unlock()

	return tenant, nil
}

// Update 更新租户
func (m *Manager) Update(ctx context.Context, tenant *Tenant) error {
	tenant.UpdatedAt = time.Now()

	if err := m.storage.Save(ctx, tenant); err != nil {
		return err
	}

	m.mu.Lock()
	m.cache[tenant.ID] = tenant
	m.mu.Unlock()

	return nil
}

// Delete 删除租户
func (m *Manager) Delete(ctx context.Context, id string) error {
	if err := m.storage.Delete(ctx, id); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.cache, id)
	m.mu.Unlock()

	return nil
}

// Suspend 暂停租户
func (m *Manager) Suspend(ctx context.Context, id string) error {
	tenant, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	tenant.Status = TenantStatusSuspended
	return m.Update(ctx, tenant)
}

// Activate 激活租户
func (m *Manager) Activate(ctx context.Context, id string) error {
	tenant, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	tenant.Status = TenantStatusActive
	return m.Update(ctx, tenant)
}

// List 列出租户
func (m *Manager) List(ctx context.Context, filter TenantFilter) ([]*Tenant, error) {
	return m.storage.List(ctx, filter)
}

// GetUsageTracker 获取使用量追踪器
func (m *Manager) GetUsageTracker() *UsageTracker {
	return m.usageTracker
}

// GetQuotaEnforcer 获取配额执行器
func (m *Manager) GetQuotaEnforcer() *QuotaEnforcer {
	return m.quotaEnforcer
}

// DefaultQuotaForType 根据类型获取默认配额
func DefaultQuotaForType(tenantType TenantType) *TenantQuota {
	switch tenantType {
	case TenantTypeFree:
		return &TenantQuota{
			MaxRequests:        1000,
			MaxTokens:          100000,
			MaxAgents:          1,
			MaxConcurrency:     1,
			MaxStorage:         100 * 1024 * 1024, // 100MB
			RateLimitPerMinute: 10,
		}
	case TenantTypeStandard:
		return &TenantQuota{
			MaxRequests:        10000,
			MaxTokens:          1000000,
			MaxAgents:          5,
			MaxConcurrency:     5,
			MaxStorage:         1024 * 1024 * 1024, // 1GB
			RateLimitPerMinute: 60,
		}
	case TenantTypePremium:
		return &TenantQuota{
			MaxRequests:        100000,
			MaxTokens:          10000000,
			MaxAgents:          20,
			MaxConcurrency:     20,
			MaxStorage:         10 * 1024 * 1024 * 1024, // 10GB
			RateLimitPerMinute: 300,
		}
	case TenantTypeEnterprise:
		return &TenantQuota{
			MaxRequests:        -1, // 无限制
			MaxTokens:          -1,
			MaxAgents:          -1,
			MaxConcurrency:     100,
			MaxStorage:         -1,
			RateLimitPerMinute: 1000,
		}
	default:
		return DefaultQuotaForType(TenantTypeFree)
	}
}

// ============== 使用量追踪 ==============

// UsageTracker 使用量追踪器
type UsageTracker struct {
	// usage 使用量数据（按租户ID和资源类型）
	usage map[string]map[string]*UsageData

	mu sync.RWMutex
}

// UsageData 使用量数据
type UsageData struct {
	// Current 当前使用量
	Current int64 `json:"current"`

	// Total 累计使用量
	Total int64 `json:"total"`

	// LastUpdated 最后更新时间
	LastUpdated time.Time `json:"last_updated"`

	// PeriodStart 计费周期开始
	PeriodStart time.Time `json:"period_start"`
}

// NewUsageTracker 创建使用量追踪器
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		usage: make(map[string]map[string]*UsageData),
	}
}

// Track 追踪使用量
func (t *UsageTracker) Track(tenantID, resource string, amount int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.usage[tenantID]; !ok {
		t.usage[tenantID] = make(map[string]*UsageData)
	}

	if _, ok := t.usage[tenantID][resource]; !ok {
		now := time.Now()
		t.usage[tenantID][resource] = &UsageData{
			PeriodStart: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), // 当月1号
		}
	}

	data := t.usage[tenantID][resource]
	data.Current += amount
	data.Total += amount
	data.LastUpdated = time.Now()
}

// GetUsage 获取使用量
func (t *UsageTracker) GetUsage(tenantID, resource string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if tenantUsage, ok := t.usage[tenantID]; ok {
		if data, ok := tenantUsage[resource]; ok {
			return data.Current
		}
	}
	return 0
}

// GetAllUsage 获取租户所有使用量
func (t *UsageTracker) GetAllUsage(tenantID string) map[string]*UsageData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*UsageData)
	if tenantUsage, ok := t.usage[tenantID]; ok {
		for k, v := range tenantUsage {
			result[k] = &UsageData{
				Current:     v.Current,
				Total:       v.Total,
				LastUpdated: v.LastUpdated,
				PeriodStart: v.PeriodStart,
			}
		}
	}
	return result
}

// ResetPeriod 重置计费周期
func (t *UsageTracker) ResetPeriod(tenantID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if tenantUsage, ok := t.usage[tenantID]; ok {
		now := time.Now()
		for _, data := range tenantUsage {
			data.Current = 0
			data.PeriodStart = now
		}
	}
}

// ============== 配额执行 ==============

// QuotaEnforcer 配额执行器
type QuotaEnforcer struct {
	manager *Manager
}

// NewQuotaEnforcer 创建配额执行器
func NewQuotaEnforcer(manager *Manager) *QuotaEnforcer {
	return &QuotaEnforcer{manager: manager}
}

// QuotaError 配额错误
type QuotaError struct {
	TenantID string
	Resource string
	Limit    int64
	Current  int64
	Message  string
}

func (e *QuotaError) Error() string {
	return e.Message
}

// Check 检查配额
func (e *QuotaEnforcer) Check(ctx context.Context, resource string, amount int64) error {
	tenant, ok := FromContext(ctx)
	if !ok {
		return nil // 无租户上下文，跳过检查
	}

	if tenant.Quota == nil {
		return nil
	}

	current := e.manager.usageTracker.GetUsage(tenant.ID, resource)
	limit := e.getLimit(tenant.Quota, resource)

	if limit > 0 && current+amount > limit {
		return &QuotaError{
			TenantID: tenant.ID,
			Resource: resource,
			Limit:    limit,
			Current:  current,
			Message:  fmt.Sprintf("quota exceeded for %s: %d/%d", resource, current+amount, limit),
		}
	}

	return nil
}

// CheckAndTrack 检查并追踪
func (e *QuotaEnforcer) CheckAndTrack(ctx context.Context, resource string, amount int64) error {
	if err := e.Check(ctx, resource, amount); err != nil {
		return err
	}

	tenant, ok := FromContext(ctx)
	if ok {
		e.manager.usageTracker.Track(tenant.ID, resource, amount)
	}

	return nil
}

// getLimit 获取资源限制
func (e *QuotaEnforcer) getLimit(quota *TenantQuota, resource string) int64 {
	switch resource {
	case "requests":
		return quota.MaxRequests
	case "tokens":
		return quota.MaxTokens
	case "storage":
		return quota.MaxStorage
	default:
		if quota.CustomQuotas != nil {
			if limit, ok := quota.CustomQuotas[resource]; ok {
				return limit
			}
		}
		return -1 // 无限制
	}
}

// ============== 内存存储实现 ==============

// MemoryStorage 内存存储
type MemoryStorage struct {
	tenants map[string]*Tenant
	mu      sync.RWMutex
}

// NewMemoryStorage 创建内存存储
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		tenants: make(map[string]*Tenant),
	}
}

// Get 获取租户
func (s *MemoryStorage) Get(ctx context.Context, id string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenant, ok := s.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant not found: %s", id)
	}
	return tenant, nil
}

// Save 保存租户
func (s *MemoryStorage) Save(ctx context.Context, tenant *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[tenant.ID] = tenant
	return nil
}

// Delete 删除租户
func (s *MemoryStorage) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tenants, id)
	return nil
}

// List 列出租户
func (s *MemoryStorage) List(ctx context.Context, filter TenantFilter) ([]*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Tenant
	for _, tenant := range s.tenants {
		if filter.Type != "" && tenant.Type != filter.Type {
			continue
		}
		if filter.Status != "" && tenant.Status != filter.Status {
			continue
		}
		if filter.Plan != "" && tenant.Plan != filter.Plan {
			continue
		}

		result = append(result, tenant)

		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result, nil
}

// ============== 租户中间件 ==============

// Middleware 租户中间件
type Middleware struct {
	manager   *Manager
	extractor TenantExtractor
}

// TenantExtractor 租户提取器接口
type TenantExtractor interface {
	Extract(ctx context.Context) (string, error)
}

// TenantExtractorFunc 函数提取器
type TenantExtractorFunc func(ctx context.Context) (string, error)

// Extract 实现 TenantExtractor
func (f TenantExtractorFunc) Extract(ctx context.Context) (string, error) {
	return f(ctx)
}

// NewMiddleware 创建中间件
func NewMiddleware(manager *Manager, extractor TenantExtractor) *Middleware {
	return &Middleware{
		manager:   manager,
		extractor: extractor,
	}
}

// Wrap 包装上下文
func (m *Middleware) Wrap(ctx context.Context) (context.Context, error) {
	tenantID, err := m.extractor.Extract(ctx)
	if err != nil {
		return ctx, err
	}

	if tenantID == "" {
		return ctx, nil
	}

	tenant, err := m.manager.Get(ctx, tenantID)
	if err != nil {
		return ctx, err
	}

	// 检查租户状态
	if tenant.Status != TenantStatusActive {
		return ctx, fmt.Errorf("tenant is not active: %s", tenant.Status)
	}

	// 检查是否过期
	if tenant.ExpiresAt != nil && time.Now().After(*tenant.ExpiresAt) {
		return ctx, fmt.Errorf("tenant has expired")
	}

	return WithTenant(ctx, tenant), nil
}
