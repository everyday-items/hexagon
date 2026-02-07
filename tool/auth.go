// Package tool 提供工具系统的扩展功能
//
// auth.go 实现工具认证系统和 ToolContext 状态感知：
//   - Credential: 统一的认证凭据管理
//   - ToolContext: 工具执行时的上下文，可感知和修改 Session 状态
//   - AuthProvider: 认证提供者接口（支持 API Key、OAuth、JWT）
//
// 参考 Google ADK Go 的 ToolContext 设计。
//
// 使用示例：
//
//	// 注册认证提供者
//	registry := NewCredentialRegistry()
//	registry.Register("github", NewAPIKeyAuth("ghp_xxx"))
//
//	// 工具执行时获取认证
//	tool := NewAuthenticatedTool(myTool, registry, "github")
package tool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============== Credential ==============

// CredentialType 认证类型
type CredentialType int

const (
	// CredentialAPIKey API Key 认证
	CredentialAPIKey CredentialType = iota

	// CredentialOAuth OAuth 认证
	CredentialOAuth

	// CredentialJWT JWT 认证
	CredentialJWT

	// CredentialBasic Basic Auth 认证
	CredentialBasic

	// CredentialBearer Bearer Token 认证
	CredentialBearer

	// CredentialCustom 自定义认证
	CredentialCustom
)

// Credential 认证凭据
type Credential struct {
	// Type 认证类型
	Type CredentialType `json:"type"`

	// Token 认证令牌/密钥
	Token string `json:"-"` // 不序列化

	// Metadata 额外认证信息
	Metadata map[string]string `json:"metadata,omitempty"`

	// ExpiresAt 过期时间（OAuth 场景）
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsExpired 是否已过期
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*c.ExpiresAt)
}

// Header 返回 HTTP Authorization 头
func (c *Credential) Header() string {
	switch c.Type {
	case CredentialAPIKey:
		return c.Token
	case CredentialBearer, CredentialOAuth, CredentialJWT:
		return "Bearer " + c.Token
	case CredentialBasic:
		return "Basic " + c.Token
	default:
		return c.Token
	}
}

// AuthProvider 认证提供者接口
type AuthProvider interface {
	// Name 提供者名称
	Name() string

	// GetCredential 获取认证凭据
	GetCredential(ctx context.Context) (*Credential, error)

	// RefreshCredential 刷新凭据（OAuth 场景）
	RefreshCredential(ctx context.Context) (*Credential, error)
}

// ============== 内置认证提供者 ==============

// APIKeyAuth API Key 认证提供者
type APIKeyAuth struct {
	name   string
	apiKey string
}

// NewAPIKeyAuth 创建 API Key 认证
func NewAPIKeyAuth(name, apiKey string) *APIKeyAuth {
	return &APIKeyAuth{name: name, apiKey: apiKey}
}

func (a *APIKeyAuth) Name() string { return a.name }

func (a *APIKeyAuth) GetCredential(_ context.Context) (*Credential, error) {
	return &Credential{
		Type:  CredentialAPIKey,
		Token: a.apiKey,
	}, nil
}

func (a *APIKeyAuth) RefreshCredential(ctx context.Context) (*Credential, error) {
	return a.GetCredential(ctx)
}

// BearerAuth Bearer Token 认证提供者
type BearerAuth struct {
	name  string
	token string
}

// NewBearerAuth 创建 Bearer Token 认证
func NewBearerAuth(name, token string) *BearerAuth {
	return &BearerAuth{name: name, token: token}
}

func (a *BearerAuth) Name() string { return a.name }

func (a *BearerAuth) GetCredential(_ context.Context) (*Credential, error) {
	return &Credential{
		Type:  CredentialBearer,
		Token: a.token,
	}, nil
}

func (a *BearerAuth) RefreshCredential(ctx context.Context) (*Credential, error) {
	return a.GetCredential(ctx)
}

// ============== CredentialRegistry ==============

// CredentialRegistry 认证凭据注册表
// 统一管理多个认证提供者
type CredentialRegistry struct {
	mu        sync.RWMutex
	providers map[string]AuthProvider
}

// NewCredentialRegistry 创建凭据注册表
func NewCredentialRegistry() *CredentialRegistry {
	return &CredentialRegistry{
		providers: make(map[string]AuthProvider),
	}
}

// Register 注册认证提供者
func (r *CredentialRegistry) Register(name string, provider AuthProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
}

// GetCredential 获取指定名称的认证凭据
func (r *CredentialRegistry) GetCredential(ctx context.Context, name string) (*Credential, error) {
	r.mu.RLock()
	provider, ok := r.providers[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("认证提供者 %q 未注册", name)
	}

	cred, err := provider.GetCredential(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取凭据失败 (%s): %w", name, err)
	}

	// 自动刷新过期凭据
	if cred.IsExpired() {
		cred, err = provider.RefreshCredential(ctx)
		if err != nil {
			return nil, fmt.Errorf("刷新凭据失败 (%s): %w", name, err)
		}
	}

	return cred, nil
}

// ============== ToolContext ==============

// ToolContext 工具执行上下文
// 提供工具执行时的状态感知和修改能力
type ToolContext struct {
	// SessionID 当前会话 ID
	SessionID string

	// AgentID 当前 Agent ID
	AgentID string

	// State 可读写的会话状态
	State map[string]any

	// Credentials 认证注册表
	Credentials *CredentialRegistry

	// stateChanges 记录状态变更
	stateChanges map[string]any

	mu sync.Mutex
}

// NewToolContext 创建工具上下文
func NewToolContext(sessionID, agentID string, state map[string]any, creds *CredentialRegistry) *ToolContext {
	stateCopy := make(map[string]any)
	for k, v := range state {
		stateCopy[k] = v
	}

	return &ToolContext{
		SessionID:    sessionID,
		AgentID:      agentID,
		State:        stateCopy,
		Credentials:  creds,
		stateChanges: make(map[string]any),
	}
}

// GetState 获取状态值
func (tc *ToolContext) GetState(key string) (any, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	v, ok := tc.State[key]
	return v, ok
}

// SetState 设置状态值（自动追踪变更）
func (tc *ToolContext) SetState(key string, value any) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.State[key] = value
	tc.stateChanges[key] = value
}

// DeleteState 删除状态值
func (tc *ToolContext) DeleteState(key string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.State, key)
	tc.stateChanges[key] = nil
}

// StateChanges 返回所有状态变更
func (tc *ToolContext) StateChanges() map[string]any {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make(map[string]any, len(tc.stateChanges))
	for k, v := range tc.stateChanges {
		result[k] = v
	}
	return result
}

// GetCredential 获取认证凭据
func (tc *ToolContext) GetCredential(ctx context.Context, name string) (*Credential, error) {
	if tc.Credentials == nil {
		return nil, fmt.Errorf("认证注册表未初始化")
	}
	return tc.Credentials.GetCredential(ctx, name)
}

// ============== Context 传递 ==============

type toolContextKey struct{}

// WithToolContext 将 ToolContext 注入到 context 中
func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return context.WithValue(ctx, toolContextKey{}, tc)
}

// GetToolContext 从 context 中获取 ToolContext
func GetToolContext(ctx context.Context) *ToolContext {
	tc, _ := ctx.Value(toolContextKey{}).(*ToolContext)
	return tc
}
