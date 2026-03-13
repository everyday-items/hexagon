// persistence.go 提供 Agent 状态持久化（检查点）功能
//
// 支持两种存储后端：
//   - MemoryCheckpointStore: 内存存储（适合开发和测试）
//   - FileCheckpointStore: 文件存储（JSON 文件，适合单机生产环境）
//
// 使用示例：
//
//	store := agent.NewMemoryCheckpointStore()
//	cp := &agent.Checkpoint{
//	    AgentID: "agent-1",
//	    State:   map[string]any{"step": 3},
//	}
//	store.Save(ctx, cp)
//	loaded, _ := store.Load(ctx, cp.ID)
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/toolkit/util/idgen"
)

// ============== 错误定义 ==============

var (
	// ErrCheckpointNotFound 检查点未找到
	ErrCheckpointNotFound = errors.New("agent: 检查点未找到")
)

// ============== 检查点 ==============

// Checkpoint Agent 状态检查点
type Checkpoint struct {
	// ID 检查点唯一标识
	ID string `json:"id"`

	// AgentID 所属 Agent 标识
	AgentID string `json:"agent_id"`

	// State Agent 状态快照
	State map[string]any `json:"state,omitempty"`

	// Messages 对话消息快照
	Messages []ConvMessage `json:"messages,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// Metadata 附加元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ============== 存储接口 ==============

// CheckpointStore 检查点存储接口
type CheckpointStore interface {
	// Save 保存检查点（若 ID 为空则自动生成）
	Save(ctx context.Context, cp *Checkpoint) error

	// Load 加载检查点
	Load(ctx context.Context, id string) (*Checkpoint, error)

	// List 列出指定 Agent 的所有检查点
	List(ctx context.Context, agentID string) ([]*Checkpoint, error)

	// Delete 删除检查点
	Delete(ctx context.Context, id string) error
}

// ============== 内存存储 ==============

// MemoryCheckpointStore 内存检查点存储
//
// 适合开发和测试。数据不持久化，进程退出后丢失。
// 线程安全。
type MemoryCheckpointStore struct {
	checkpoints map[string]*Checkpoint
	mu          sync.RWMutex
}

// NewMemoryCheckpointStore 创建内存检查点存储
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		checkpoints: make(map[string]*Checkpoint),
	}
}

func (s *MemoryCheckpointStore) Save(_ context.Context, cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cp.ID == "" {
		cp.ID = idgen.NanoID()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}

	// 深拷贝，防止外部修改影响已保存的检查点
	clone := &Checkpoint{
		ID:        cp.ID,
		AgentID:   cp.AgentID,
		CreatedAt: cp.CreatedAt,
	}
	if cp.State != nil {
		clone.State = make(map[string]any, len(cp.State))
		for k, v := range cp.State {
			clone.State[k] = v
		}
	}
	if cp.Metadata != nil {
		clone.Metadata = make(map[string]any, len(cp.Metadata))
		for k, v := range cp.Metadata {
			clone.Metadata[k] = v
		}
	}
	if cp.Messages != nil {
		clone.Messages = make([]ConvMessage, len(cp.Messages))
		copy(clone.Messages, cp.Messages)
	}
	s.checkpoints[clone.ID] = clone
	return nil
}

func (s *MemoryCheckpointStore) Load(_ context.Context, id string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp, exists := s.checkpoints[id]
	if !exists {
		return nil, ErrCheckpointNotFound
	}
	return cp, nil
}

func (s *MemoryCheckpointStore) List(_ context.Context, agentID string) ([]*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Checkpoint
	for _, cp := range s.checkpoints {
		if cp.AgentID == agentID {
			result = append(result, cp)
		}
	}
	return result, nil
}

func (s *MemoryCheckpointStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.checkpoints[id]; !exists {
		return ErrCheckpointNotFound
	}
	delete(s.checkpoints, id)
	return nil
}

// ============== 文件存储 ==============

// FileCheckpointStore 文件检查点存储
//
// 将每个检查点保存为独立 JSON 文件，文件名为 {id}.json。
// 适合单机生产环境。
type FileCheckpointStore struct {
	dir string // 绝对路径，所有操作限制在此目录内
}

// safePath 校验并返回安全的文件路径，防止路径穿越攻击
//
// 确保最终路径在 s.dir 目录内。
func (s *FileCheckpointStore) safePath(id string) (string, error) {
	// 禁止包含路径分隔符和特殊序列
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("检查点 ID 包含非法字符: %s", id)
	}
	path := filepath.Join(s.dir, id+".json")
	// 二次校验：确保 Clean 后仍在 dir 内
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析路径失败: %w", err)
	}
	absDir, err := filepath.Abs(s.dir)
	if err != nil {
		return "", fmt.Errorf("解析目录路径失败: %w", err)
	}
	if !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
		return "", fmt.Errorf("检查点 ID 路径穿越: %s", id)
	}
	return path, nil
}

// NewFileCheckpointStore 创建文件检查点存储
//
// dir 为存储目录，不存在时自动创建。
func NewFileCheckpointStore(dir string) (*FileCheckpointStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建检查点目录失败: %w", err)
	}
	return &FileCheckpointStore{dir: dir}, nil
}

func (s *FileCheckpointStore) Save(_ context.Context, cp *Checkpoint) error {
	if cp.ID == "" {
		cp.ID = idgen.NanoID()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}

	path, err := s.safePath(cp.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化检查点失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入检查点文件失败: %w", err)
	}
	return nil
}

func (s *FileCheckpointStore) Load(_ context.Context, id string) (*Checkpoint, error) {
	path, err := s.safePath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCheckpointNotFound
		}
		return nil, fmt.Errorf("读取检查点文件失败: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("反序列化检查点失败: %w", err)
	}
	return &cp, nil
}

func (s *FileCheckpointStore) List(_ context.Context, agentID string) ([]*Checkpoint, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("读取检查点目录失败: %w", err)
	}

	var result []*Checkpoint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}

		if cp.AgentID == agentID {
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *FileCheckpointStore) Delete(_ context.Context, id string) error {
	path, err := s.safePath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrCheckpointNotFound
		}
		return fmt.Errorf("删除检查点文件失败: %w", err)
	}
	return nil
}
