// Package rbac 提供 Hexagon AI Agent 框架的基于角色的访问控制
//
// 支持角色定义、权限管理、资源访问控制等功能。
package rbac

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// RBAC 角色权限控制系统
type RBAC struct {
	// Roles 角色表
	roles map[string]*Role

	// Users 用户表
	users map[string]*User

	// Policies 策略表
	policies map[string]*Policy

	// RoleHierarchy 角色层级
	roleHierarchy map[string][]string

	// Store 持久化存储
	store RBACStore

	// regexCache 缓存编译后的正则表达式，避免 OpMatches 每次重新编译
	regexCache sync.Map // pattern -> *regexp.Regexp

	mu sync.RWMutex
}

// RBACStore RBAC 存储接口
type RBACStore interface {
	SaveRole(ctx context.Context, role *Role) error
	GetRole(ctx context.Context, name string) (*Role, error)
	DeleteRole(ctx context.Context, name string) error
	ListRoles(ctx context.Context) ([]*Role, error)

	SaveUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	DeleteUser(ctx context.Context, id string) error

	SavePolicy(ctx context.Context, policy *Policy) error
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	DeletePolicy(ctx context.Context, id string) error
	ListPolicies(ctx context.Context) ([]*Policy, error)
}

// NewRBAC 创建 RBAC 系统
func NewRBAC() *RBAC {
	rbac := &RBAC{
		roles:         make(map[string]*Role),
		users:         make(map[string]*User),
		policies:      make(map[string]*Policy),
		roleHierarchy: make(map[string][]string),
	}

	// 初始化默认角色
	rbac.initDefaultRoles()

	return rbac
}

// SetStore 设置存储
func (r *RBAC) SetStore(store RBACStore) {
	r.store = store
}

// initDefaultRoles 初始化默认角色
func (r *RBAC) initDefaultRoles() {
	// 超级管理员
	r.AddRole(&Role{
		Name:        "admin",
		DisplayName: "Administrator",
		Description: "Full system access",
		Permissions: []Permission{
			{Resource: "*", Action: "*"},
		},
	})

	// 普通用户
	r.AddRole(&Role{
		Name:        "user",
		DisplayName: "User",
		Description: "Standard user access",
		Permissions: []Permission{
			{Resource: "agent", Action: "run"},
			{Resource: "agent", Action: "read"},
			{Resource: "tool", Action: "execute"},
			{Resource: "memory", Action: "read"},
			{Resource: "memory", Action: "write"},
		},
	})

	// 访客
	r.AddRole(&Role{
		Name:        "guest",
		DisplayName: "Guest",
		Description: "Read-only access",
		Permissions: []Permission{
			{Resource: "agent", Action: "read"},
			{Resource: "tool", Action: "read"},
		},
	})

	// Agent 角色
	r.AddRole(&Role{
		Name:        "agent",
		DisplayName: "AI Agent",
		Description: "Agent execution permissions",
		Permissions: []Permission{
			{Resource: "llm", Action: "call"},
			{Resource: "tool", Action: "execute"},
			{Resource: "memory", Action: "read"},
			{Resource: "memory", Action: "write"},
		},
	})

	// 设置角色层级
	r.SetRoleHierarchy("admin", []string{"user", "guest", "agent"})
	r.SetRoleHierarchy("user", []string{"guest"})
}

// ============== Role Management ==============

// Role 角色
type Role struct {
	// Name 角色名称（唯一标识）
	Name string `json:"name" yaml:"name"`

	// DisplayName 显示名称
	DisplayName string `json:"display_name" yaml:"display_name"`

	// Description 描述
	Description string `json:"description" yaml:"description"`

	// Permissions 权限列表
	Permissions []Permission `json:"permissions" yaml:"permissions"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// Permission 权限
type Permission struct {
	// Resource 资源（支持通配符 *）
	Resource string `json:"resource" yaml:"resource"`

	// Action 操作（支持通配符 *）
	Action string `json:"action" yaml:"action"`

	// Conditions 条件
	Conditions map[string]any `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// AddRole 添加角色
// 先在内存中更新，然后释放锁后再持久化到 store，避免持锁调用外部函数导致死锁。
func (r *RBAC) AddRole(role *Role) error {
	r.mu.Lock()

	if _, exists := r.roles[role.Name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("role %s already exists", role.Name)
	}

	now := time.Now()
	role.CreatedAt = now
	role.UpdatedAt = now

	r.roles[role.Name] = role
	r.mu.Unlock()

	// 在锁外调用 store，避免持锁期间阻塞在外部 I/O 上
	if r.store != nil {
		if err := r.store.SaveRole(context.Background(), role); err != nil {
			// store 持久化失败，回滚内存状态
			r.mu.Lock()
			delete(r.roles, role.Name)
			r.mu.Unlock()
			return err
		}
	}

	return nil
}

// UpdateRole 更新角色
// 先在内存中更新，然后释放锁后再持久化到 store，避免持锁调用外部函数导致死锁。
func (r *RBAC) UpdateRole(role *Role) error {
	r.mu.Lock()

	oldRole, exists := r.roles[role.Name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("role %s not found", role.Name)
	}

	role.UpdatedAt = time.Now()
	r.roles[role.Name] = role
	r.mu.Unlock()

	// 在锁外调用 store
	if r.store != nil {
		if err := r.store.SaveRole(context.Background(), role); err != nil {
			// store 持久化失败，回滚内存状态
			r.mu.Lock()
			r.roles[role.Name] = oldRole
			r.mu.Unlock()
			return err
		}
	}

	return nil
}

// DeleteRole 删除角色
// 先在内存中删除，然后释放锁后再持久化到 store，避免持锁调用外部函数导致死锁。
func (r *RBAC) DeleteRole(name string) error {
	r.mu.Lock()

	oldRole, exists := r.roles[name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("role %s not found", name)
	}

	oldHierarchy := r.roleHierarchy[name]
	delete(r.roles, name)
	delete(r.roleHierarchy, name)
	r.mu.Unlock()

	// 在锁外调用 store
	if r.store != nil {
		if err := r.store.DeleteRole(context.Background(), name); err != nil {
			// store 持久化失败，回滚内存状态
			r.mu.Lock()
			r.roles[name] = oldRole
			if oldHierarchy != nil {
				r.roleHierarchy[name] = oldHierarchy
			}
			r.mu.Unlock()
			return err
		}
	}

	return nil
}

// GetRole 获取角色
func (r *RBAC) GetRole(name string) (*Role, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	role, ok := r.roles[name]
	return role, ok
}

// ListRoles 列出所有角色
func (r *RBAC) ListRoles() []*Role {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]*Role, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, role)
	}
	return roles
}

// SetRoleHierarchy 设置角色层级
func (r *RBAC) SetRoleHierarchy(parent string, children []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roleHierarchy[parent] = children
}

// GetInheritedRoles 获取继承的角色
//
// 线程安全：此方法会获取读锁。
// 注意：不要在已持有读锁的情况下调用此方法，否则可能导致死锁。
// 如果已持有锁，请使用 getInheritedRolesLocked。
func (r *RBAC) GetInheritedRoles(roleName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getInheritedRolesLocked(roleName)
}

// getInheritedRolesLocked 获取继承的角色（调用者必须已持有读锁）
func (r *RBAC) getInheritedRolesLocked(roleName string) []string {
	visited := make(map[string]bool)
	return r.collectInheritedRoles(roleName, visited)
}

func (r *RBAC) collectInheritedRoles(roleName string, visited map[string]bool) []string {
	if visited[roleName] {
		return nil
	}
	visited[roleName] = true

	result := []string{roleName}
	if children, ok := r.roleHierarchy[roleName]; ok {
		for _, child := range children {
			result = append(result, r.collectInheritedRoles(child, visited)...)
		}
	}
	return result
}

// ============== User Management ==============

// User 用户
type User struct {
	// ID 用户 ID
	ID string `json:"id" yaml:"id"`

	// Name 用户名
	Name string `json:"name" yaml:"name"`

	// Roles 角色列表
	Roles []string `json:"roles" yaml:"roles"`

	// Attributes 用户属性
	Attributes map[string]any `json:"attributes,omitempty" yaml:"attributes,omitempty"`

	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// LastActiveAt 最后活跃时间
	LastActiveAt time.Time `json:"last_active_at" yaml:"last_active_at"`
}

// AddUser 添加用户
func (r *RBAC) AddUser(user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if user.ID == "" {
		user.ID = util.GenerateID("user")
	}

	if _, exists := r.users[user.ID]; exists {
		return fmt.Errorf("user %s already exists", user.ID)
	}

	user.CreatedAt = time.Now()
	user.Enabled = true
	r.users[user.ID] = user

	if r.store != nil {
		return r.store.SaveUser(context.Background(), user)
	}

	return nil
}

// UpdateUser 更新用户
func (r *RBAC) UpdateUser(user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.ID]; !exists {
		return fmt.Errorf("user %s not found", user.ID)
	}

	r.users[user.ID] = user

	if r.store != nil {
		return r.store.SaveUser(context.Background(), user)
	}

	return nil
}

// DeleteUser 删除用户
func (r *RBAC) DeleteUser(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[id]; !exists {
		return fmt.Errorf("user %s not found", id)
	}

	delete(r.users, id)

	if r.store != nil {
		return r.store.DeleteUser(context.Background(), id)
	}

	return nil
}

// GetUser 获取用户
func (r *RBAC) GetUser(id string) (*User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	return user, ok
}

// AssignRole 分配角色给用户
func (r *RBAC) AssignRole(userID, roleName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, ok := r.users[userID]
	if !ok {
		return fmt.Errorf("user %s not found", userID)
	}

	if _, ok := r.roles[roleName]; !ok {
		return fmt.Errorf("role %s not found", roleName)
	}

	// 检查是否已有该角色
	if slices.Contains(user.Roles, roleName) {
		return nil
	}

	user.Roles = append(user.Roles, roleName)

	if r.store != nil {
		return r.store.SaveUser(context.Background(), user)
	}

	return nil
}

// RevokeRole 撤销用户角色
func (r *RBAC) RevokeRole(userID, roleName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, ok := r.users[userID]
	if !ok {
		return fmt.Errorf("user %s not found", userID)
	}

	newRoles := make([]string, 0)
	for _, role := range user.Roles {
		if role != roleName {
			newRoles = append(newRoles, role)
		}
	}
	user.Roles = newRoles

	if r.store != nil {
		return r.store.SaveUser(context.Background(), user)
	}

	return nil
}

// GetUserRoles 获取用户的所有角色（包括继承的）
//
// 线程安全：此方法会获取读锁，并使用内部方法避免嵌套锁导致的死锁。
func (r *RBAC) GetUserRoles(userID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getUserRolesLocked(userID)
}

// getUserRolesLocked 获取用户的所有角色（调用者必须已持有读锁）
func (r *RBAC) getUserRolesLocked(userID string) []string {
	user, ok := r.users[userID]
	if !ok {
		return nil
	}

	allRoles := make(map[string]bool)
	for _, roleName := range user.Roles {
		// 使用不加锁的内部方法，因为我们已经持有读锁
		for _, inherited := range r.getInheritedRolesLocked(roleName) {
			allRoles[inherited] = true
		}
	}

	result := make([]string, 0, len(allRoles))
	for role := range allRoles {
		result = append(result, role)
	}
	return result
}

// ============== Authorization ==============

// AccessRequest 访问请求
type AccessRequest struct {
	// Subject 主体（用户/Agent ID）
	Subject string

	// Resource 资源
	Resource string

	// Action 操作
	Action string

	// Context 上下文
	Context map[string]any
}

// AccessResult 访问结果
type AccessResult struct {
	// Allowed 是否允许
	Allowed bool

	// Reason 原因
	Reason string

	// MatchedPolicy 匹配的策略
	MatchedPolicy string

	// MatchedPermission 匹配的权限
	MatchedPermission *Permission
}

// Authorize 授权检查
//
// 安全说明：此方法要求 Subject 必须是已验证的用户 ID。
// 调用者应该通过 AuthorizeFromContext 方法进行授权检查，
// 该方法会从 context 中获取已验证的用户身份。
func (r *RBAC) Authorize(req AccessRequest) AccessResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := AccessResult{
		Allowed: false,
		Reason:  "no matching permission",
	}

	// 获取用户
	user, ok := r.users[req.Subject]
	if !ok {
		result.Reason = "user not found"
		return result
	}

	// 检查用户是否启用
	if !user.Enabled {
		result.Reason = "user disabled"
		return result
	}

	// 获取用户的所有角色（使用内部方法避免死锁）
	allRoles := r.getUserRolesLocked(req.Subject)

	// 检查每个角色的权限
	for _, roleName := range allRoles {
		role, ok := r.roles[roleName]
		if !ok {
			continue
		}

		for _, perm := range role.Permissions {
			if r.matchPermission(perm, req) {
				result.Allowed = true
				result.Reason = fmt.Sprintf("permitted by role %s", roleName)
				result.MatchedPermission = &perm
				return result
			}
		}
	}

	// 检查策略
	for _, policy := range r.policies {
		if policy.Enabled && r.matchPolicy(policy, req) {
			result.Allowed = policy.Effect == EffectAllow
			result.Reason = fmt.Sprintf("matched policy %s", policy.ID)
			result.MatchedPolicy = policy.ID
			return result
		}
	}

	return result
}

// matchPermission 匹配权限
func (r *RBAC) matchPermission(perm Permission, req AccessRequest) bool {
	// 检查资源
	if !matchWildcard(perm.Resource, req.Resource) {
		return false
	}

	// 检查操作
	if !matchWildcard(perm.Action, req.Action) {
		return false
	}

	// 检查条件
	if len(perm.Conditions) > 0 {
		for key, value := range perm.Conditions {
			if reqValue, ok := req.Context[key]; !ok || reqValue != value {
				return false
			}
		}
	}

	return true
}

// matchWildcard 通配符匹配
func matchWildcard(pattern, value string) bool {
	if pattern == "*" {
		return true
	}

	// 简单的前缀匹配（如 "agent:*" 匹配 "agent:run"）
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}

	return pattern == value
}

// ============== Policy ==============

// Policy 策略
type Policy struct {
	// ID 策略 ID
	ID string `json:"id" yaml:"id"`

	// Name 策略名称
	Name string `json:"name" yaml:"name"`

	// Description 描述
	Description string `json:"description" yaml:"description"`

	// Effect 效果
	Effect PolicyEffect `json:"effect" yaml:"effect"`

	// Subjects 主体
	Subjects []string `json:"subjects" yaml:"subjects"`

	// Resources 资源
	Resources []string `json:"resources" yaml:"resources"`

	// Actions 操作
	Actions []string `json:"actions" yaml:"actions"`

	// Conditions 条件
	Conditions []PolicyCondition `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	// Priority 优先级（越大越高）
	Priority int `json:"priority" yaml:"priority"`

	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// PolicyEffect 策略效果
type PolicyEffect string

const (
	EffectAllow PolicyEffect = "allow"
	EffectDeny  PolicyEffect = "deny"
)

// PolicyCondition 策略条件
type PolicyCondition struct {
	// Key 条件键
	Key string `json:"key" yaml:"key"`

	// Operator 操作符
	Operator ConditionOperator `json:"operator" yaml:"operator"`

	// Value 值
	Value any `json:"value" yaml:"value"`
}

// ConditionOperator 条件操作符
type ConditionOperator string

const (
	OpEquals     ConditionOperator = "equals"
	OpNotEquals  ConditionOperator = "not_equals"
	OpContains   ConditionOperator = "contains"
	OpStartsWith ConditionOperator = "starts_with"
	OpEndsWith   ConditionOperator = "ends_with"
	OpMatches    ConditionOperator = "matches" // 正则匹配
	OpIn         ConditionOperator = "in"
	OpNotIn      ConditionOperator = "not_in"
	OpGreaterThan ConditionOperator = "greater_than"
	OpLessThan   ConditionOperator = "less_than"
)

// AddPolicy 添加策略
func (r *RBAC) AddPolicy(policy *Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if policy.ID == "" {
		policy.ID = util.GenerateID("policy")
	}

	policy.CreatedAt = time.Now()
	policy.Enabled = true
	r.policies[policy.ID] = policy

	if r.store != nil {
		return r.store.SavePolicy(context.Background(), policy)
	}

	return nil
}

// UpdatePolicy 更新策略
func (r *RBAC) UpdatePolicy(policy *Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.policies[policy.ID]; !exists {
		return fmt.Errorf("policy %s not found", policy.ID)
	}

	r.policies[policy.ID] = policy

	if r.store != nil {
		return r.store.SavePolicy(context.Background(), policy)
	}

	return nil
}

// DeletePolicy 删除策略
func (r *RBAC) DeletePolicy(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.policies[id]; !exists {
		return fmt.Errorf("policy %s not found", id)
	}

	delete(r.policies, id)

	if r.store != nil {
		return r.store.DeletePolicy(context.Background(), id)
	}

	return nil
}

// GetPolicy 获取策略
func (r *RBAC) GetPolicy(id string) (*Policy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy, ok := r.policies[id]
	return policy, ok
}

// ListPolicies 列出所有策略
func (r *RBAC) ListPolicies() []*Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policies := make([]*Policy, 0, len(r.policies))
	for _, policy := range r.policies {
		policies = append(policies, policy)
	}
	return policies
}

// matchPolicy 匹配策略
func (r *RBAC) matchPolicy(policy *Policy, req AccessRequest) bool {
	// 匹配主体
	subjectMatched := false
	for _, subject := range policy.Subjects {
		if matchWildcard(subject, req.Subject) {
			subjectMatched = true
			break
		}
	}
	if !subjectMatched {
		return false
	}

	// 匹配资源
	resourceMatched := false
	for _, resource := range policy.Resources {
		if matchWildcard(resource, req.Resource) {
			resourceMatched = true
			break
		}
	}
	if !resourceMatched {
		return false
	}

	// 匹配操作
	actionMatched := false
	for _, action := range policy.Actions {
		if matchWildcard(action, req.Action) {
			actionMatched = true
			break
		}
	}
	if !actionMatched {
		return false
	}

	// 检查条件
	for _, cond := range policy.Conditions {
		if !r.evaluateCondition(cond, req.Context) {
			return false
		}
	}

	return true
}

// evaluateCondition 评估条件
func (r *RBAC) evaluateCondition(cond PolicyCondition, ctx map[string]any) bool {
	value, ok := ctx[cond.Key]
	if !ok {
		return false
	}

	switch cond.Operator {
	case OpEquals:
		return value == cond.Value
	case OpNotEquals:
		return value != cond.Value
	case OpContains:
		if str, ok := value.(string); ok {
			if condStr, ok := cond.Value.(string); ok {
				return strings.Contains(str, condStr)
			}
		}
	case OpStartsWith:
		if str, ok := value.(string); ok {
			if condStr, ok := cond.Value.(string); ok {
				return len(str) >= len(condStr) && str[:len(condStr)] == condStr
			}
		}
	case OpEndsWith:
		if str, ok := value.(string); ok {
			if condStr, ok := cond.Value.(string); ok {
				return len(str) >= len(condStr) && str[len(str)-len(condStr):] == condStr
			}
		}
	case OpMatches:
		if str, ok := value.(string); ok {
			if pattern, ok := cond.Value.(string); ok {
				// 限制输入字符串长度防止 ReDoS
				if len(str) > 10000 {
					return false
				}
				// 限制正则表达式长度
				if len(pattern) > 1000 {
					return false
				}
				// 使用缓存的编译后正则，避免每次重新编译
				re := r.getOrCompileRegex(pattern)
				if re != nil {
					return re.MatchString(str)
				}
			}
		}
	case OpIn:
		if list, ok := cond.Value.([]any); ok {
			return slices.Contains(list, value)
		}
	case OpNotIn:
		if list, ok := cond.Value.([]any); ok {
			return !slices.Contains(list, value)
		}
	}

	return false
}

// getOrCompileRegex 获取或编译正则表达式（带缓存）
// 编译后的正则会缓存在 sync.Map 中，避免重复编译开销。
func (r *RBAC) getOrCompileRegex(pattern string) *regexp.Regexp {
	if cached, ok := r.regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	r.regexCache.Store(pattern, re)
	return re
}

// ============== Context Helpers ==============

// contextKey context key
type contextKey struct{}

// ContextWithUser 将用户添加到 context
func ContextWithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

// UserFromContext 从 context 获取用户
func UserFromContext(ctx context.Context) *User {
	if user, ok := ctx.Value(contextKey{}).(*User); ok {
		return user
	}
	return nil
}

// AuthorizeFromContext 从 context 授权检查
func (r *RBAC) AuthorizeFromContext(ctx context.Context, resource, action string) AccessResult {
	user := UserFromContext(ctx)
	if user == nil {
		return AccessResult{
			Allowed: false,
			Reason:  "no user in context",
		}
	}

	return r.Authorize(AccessRequest{
		Subject:  user.ID,
		Resource: resource,
		Action:   action,
	})
}

// ============== Memory Store ==============

// MemoryRBACStore 内存存储
type MemoryRBACStore struct {
	roles    map[string]*Role
	users    map[string]*User
	policies map[string]*Policy
	mu       sync.RWMutex
}

// NewMemoryRBACStore 创建内存存储
func NewMemoryRBACStore() *MemoryRBACStore {
	return &MemoryRBACStore{
		roles:    make(map[string]*Role),
		users:    make(map[string]*User),
		policies: make(map[string]*Policy),
	}
}

func (s *MemoryRBACStore) SaveRole(ctx context.Context, role *Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roles[role.Name] = role
	return nil
}

func (s *MemoryRBACStore) GetRole(ctx context.Context, name string) (*Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if role, ok := s.roles[name]; ok {
		return role, nil
	}
	return nil, fmt.Errorf("role %s not found", name)
}

func (s *MemoryRBACStore) DeleteRole(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.roles, name)
	return nil
}

func (s *MemoryRBACStore) ListRoles(ctx context.Context) ([]*Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roles := make([]*Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *MemoryRBACStore) SaveUser(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.ID] = user
	return nil
}

func (s *MemoryRBACStore) GetUser(ctx context.Context, id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if user, ok := s.users[id]; ok {
		return user, nil
	}
	return nil, fmt.Errorf("user %s not found", id)
}

func (s *MemoryRBACStore) DeleteUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return nil
}

func (s *MemoryRBACStore) SavePolicy(ctx context.Context, policy *Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[policy.ID] = policy
	return nil
}

func (s *MemoryRBACStore) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if policy, ok := s.policies[id]; ok {
		return policy, nil
	}
	return nil, fmt.Errorf("policy %s not found", id)
}

func (s *MemoryRBACStore) DeletePolicy(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.policies, id)
	return nil
}

func (s *MemoryRBACStore) ListPolicies(ctx context.Context) ([]*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policies := make([]*Policy, 0, len(s.policies))
	for _, policy := range s.policies {
		policies = append(policies, policy)
	}
	return policies, nil
}
