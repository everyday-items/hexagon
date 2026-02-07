// Package artifact 提供 Agent 生成文件的版本化管理
//
// Artifact 管理系统参考 Google ADK Go 设计，用于：
//   - 保存 Agent 执行过程中生成的文件和二进制数据
//   - 版本化管理，支持按版本回溯
//   - 与 Session 关联，支持按会话查询
//
// 使用示例：
//
//	store := NewMemoryArtifactStore()
//	svc := NewService(store)
//
//	// 保存 artifact
//	id, err := svc.Save(ctx, Artifact{
//	    SessionID: "session-123",
//	    Name:      "report.pdf",
//	    MimeType:  "application/pdf",
//	    Data:      pdfBytes,
//	})
//
//	// 获取最新版本
//	art, err := svc.GetLatest(ctx, "session-123", "report.pdf")
//
//	// 列出所有版本
//	versions, err := svc.ListVersions(ctx, "session-123", "report.pdf")
package artifact

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Artifact 代表 Agent 生成的一个文件或数据对象
type Artifact struct {
	// ID 唯一标识
	ID string `json:"id"`

	// SessionID 关联的会话 ID
	SessionID string `json:"session_id"`

	// Name 文件名称（同名同 session 的多个 artifact 视为不同版本）
	Name string `json:"name"`

	// MimeType MIME 类型
	MimeType string `json:"mime_type"`

	// Data 文件内容
	Data []byte `json:"data,omitempty"`

	// Version 版本号（从 1 开始自增）
	Version int `json:"version"`

	// Size 文件大小（字节）
	Size int64 `json:"size"`

	// Metadata 额外元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy 创建者（Agent ID）
	CreatedBy string `json:"created_by,omitempty"`
}

// Store artifact 存储接口
type Store interface {
	// Save 保存 artifact，返回 ID
	Save(ctx context.Context, art Artifact) (string, error)

	// Get 按 ID 获取 artifact
	Get(ctx context.Context, id string) (*Artifact, error)

	// GetByVersion 按名称和版本获取
	GetByVersion(ctx context.Context, sessionID, name string, version int) (*Artifact, error)

	// GetLatest 获取最新版本
	GetLatest(ctx context.Context, sessionID, name string) (*Artifact, error)

	// List 列出会话下所有 artifact（最新版本）
	List(ctx context.Context, sessionID string) ([]Artifact, error)

	// ListVersions 列出指定名称的所有版本
	ListVersions(ctx context.Context, sessionID, name string) ([]Artifact, error)

	// Delete 删除指定 artifact
	Delete(ctx context.Context, id string) error

	// DeleteAll 删除会话下所有 artifact
	DeleteAll(ctx context.Context, sessionID string) error
}

// Service artifact 管理服务
type Service struct {
	store Store
}

// NewService 创建 artifact 管理服务
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Save 保存 artifact，自动生成 ID 和版本号
func (s *Service) Save(ctx context.Context, art Artifact) (string, error) {
	if art.SessionID == "" {
		return "", fmt.Errorf("SessionID 不能为空")
	}
	if art.Name == "" {
		return "", fmt.Errorf("Name 不能为空")
	}

	// 获取当前最大版本号
	latest, _ := s.store.GetLatest(ctx, art.SessionID, art.Name)
	if latest != nil {
		art.Version = latest.Version + 1
	} else {
		art.Version = 1
	}

	if art.ID == "" {
		art.ID = fmt.Sprintf("%s/%s/v%d", art.SessionID, art.Name, art.Version)
	}
	art.Size = int64(len(art.Data))
	if art.CreatedAt.IsZero() {
		art.CreatedAt = time.Now()
	}

	return s.store.Save(ctx, art)
}

// Get 获取 artifact
func (s *Service) Get(ctx context.Context, id string) (*Artifact, error) {
	return s.store.Get(ctx, id)
}

// GetLatest 获取最新版本
func (s *Service) GetLatest(ctx context.Context, sessionID, name string) (*Artifact, error) {
	return s.store.GetLatest(ctx, sessionID, name)
}

// ListVersions 列出所有版本
func (s *Service) ListVersions(ctx context.Context, sessionID, name string) ([]Artifact, error) {
	return s.store.ListVersions(ctx, sessionID, name)
}

// List 列出会话下所有 artifact
func (s *Service) List(ctx context.Context, sessionID string) ([]Artifact, error) {
	return s.store.List(ctx, sessionID)
}

// Delete 删除 artifact
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// DeleteAll 删除会话下所有 artifact
func (s *Service) DeleteAll(ctx context.Context, sessionID string) error {
	return s.store.DeleteAll(ctx, sessionID)
}

// ============== MemoryStore 内存存储实现 ==============

// MemoryStore 内存存储
// 适用于开发和测试环境
type MemoryStore struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact // ID -> Artifact
}

// NewMemoryStore 创建内存存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		artifacts: make(map[string]*Artifact),
	}
}

func (s *MemoryStore) Save(_ context.Context, art Artifact) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	artCopy := art
	s.artifacts[art.ID] = &artCopy
	return art.ID, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	art, ok := s.artifacts[id]
	if !ok {
		return nil, fmt.Errorf("artifact 不存在: %s", id)
	}
	return art, nil
}

func (s *MemoryStore) GetByVersion(_ context.Context, sessionID, name string, version int) (*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, art := range s.artifacts {
		if art.SessionID == sessionID && art.Name == name && art.Version == version {
			return art, nil
		}
	}
	return nil, fmt.Errorf("artifact 不存在: %s/%s v%d", sessionID, name, version)
}

func (s *MemoryStore) GetLatest(_ context.Context, sessionID, name string) (*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *Artifact
	for _, art := range s.artifacts {
		if art.SessionID == sessionID && art.Name == name {
			if latest == nil || art.Version > latest.Version {
				latest = art
			}
		}
	}

	if latest == nil {
		return nil, nil
	}
	return latest, nil
}

func (s *MemoryStore) List(_ context.Context, sessionID string) ([]Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 按名称聚合，取最新版本
	latestByName := make(map[string]*Artifact)
	for _, art := range s.artifacts {
		if art.SessionID != sessionID {
			continue
		}
		if existing, ok := latestByName[art.Name]; !ok || art.Version > existing.Version {
			latestByName[art.Name] = art
		}
	}

	var result []Artifact
	for _, art := range latestByName {
		result = append(result, *art)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (s *MemoryStore) ListVersions(_ context.Context, sessionID, name string) ([]Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var versions []Artifact
	for _, art := range s.artifacts {
		if art.SessionID == sessionID && art.Name == name {
			versions = append(versions, *art)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	return versions, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.artifacts, id)
	return nil
}

func (s *MemoryStore) DeleteAll(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, art := range s.artifacts {
		if art.SessionID == sessionID {
			delete(s.artifacts, id)
		}
	}
	return nil
}

var _ Store = (*MemoryStore)(nil)
