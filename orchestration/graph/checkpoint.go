package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// Checkpoint 检查点
// 用于保存图执行的中间状态
type Checkpoint struct {
	// ID 检查点 ID
	ID string `json:"id"`

	// ThreadID 线程 ID（用于区分不同的执行实例）
	ThreadID string `json:"thread_id"`

	// GraphName 图名称
	GraphName string `json:"graph_name"`

	// CurrentNode 当前节点
	CurrentNode string `json:"current_node"`

	// State 状态快照
	State json.RawMessage `json:"state"`

	// PendingNodes 待执行的节点
	PendingNodes []string `json:"pending_nodes"`

	// CompletedNodes 已完成的节点
	CompletedNodes []string `json:"completed_nodes"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// InterruptAddrs 中断点地址映射（中断 ID -> 序列化的地址）
	// 用于持久化 InterruptSignal 树中各中断点的层级地址
	InterruptAddrs map[string]json.RawMessage `json:"interrupt_addrs,omitempty"`

	// InterruptStates 中断点状态映射（中断 ID -> 序列化的组件状态）
	// 用于持久化 StatefulInterrupt 保存的组件内部状态
	InterruptStates map[string]json.RawMessage `json:"interrupt_states,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`

	// ParentID 父检查点 ID（用于构建历史链）
	ParentID string `json:"parent_id,omitempty"`
}

// CheckpointSaver 检查点保存器接口
type CheckpointSaver interface {
	// Save 保存检查点
	Save(ctx context.Context, checkpoint *Checkpoint) error

	// Load 加载检查点
	Load(ctx context.Context, threadID string) (*Checkpoint, error)

	// LoadByID 根据 ID 加载检查点
	LoadByID(ctx context.Context, id string) (*Checkpoint, error)

	// List 列出线程的所有检查点
	List(ctx context.Context, threadID string) ([]*Checkpoint, error)

	// Delete 删除检查点
	Delete(ctx context.Context, id string) error

	// DeleteThread 删除线程的所有检查点
	DeleteThread(ctx context.Context, threadID string) error
}

// MemoryCheckpointSaver 内存检查点保存器
type MemoryCheckpointSaver struct {
	checkpoints map[string]*Checkpoint // id -> checkpoint
	threads     map[string][]string    // threadID -> []checkpointID
	mu          sync.RWMutex
}

// NewMemoryCheckpointSaver 创建内存检查点保存器
func NewMemoryCheckpointSaver() *MemoryCheckpointSaver {
	return &MemoryCheckpointSaver{
		checkpoints: make(map[string]*Checkpoint),
		threads:     make(map[string][]string),
	}
}

// Save 保存检查点
func (s *MemoryCheckpointSaver) Save(ctx context.Context, checkpoint *Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if checkpoint.ThreadID == "" {
		return fmt.Errorf("checkpoint thread_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}
	if existing, ok := s.checkpoints[checkpoint.ID]; ok && checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = existing.CreatedAt
	}
	checkpoint.UpdatedAt = time.Now()
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = checkpoint.UpdatedAt
	}

	// 保存副本，避免外部修改引用导致检查点污染
	s.checkpoints[checkpoint.ID] = cloneCheckpoint(checkpoint)

	// 同一个检查点 ID 只记录一次线程索引，避免重复列表项
	ids := s.threads[checkpoint.ThreadID]
	exists := false
	for _, id := range ids {
		if id == checkpoint.ID {
			exists = true
			break
		}
	}
	if !exists {
		s.threads[checkpoint.ThreadID] = append(ids, checkpoint.ID)
	}

	return nil
}

// Load 加载最新的检查点
func (s *MemoryCheckpointSaver) Load(ctx context.Context, threadID string) (*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.threads[threadID]
	if !ok || len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoint found for thread %s", threadID)
	}

	// 返回最新的检查点
	latestID := ids[len(ids)-1]
	cp, ok := s.checkpoints[latestID]
	if !ok {
		return nil, fmt.Errorf("checkpoint %s not found", latestID)
	}

	return cloneCheckpoint(cp), nil
}

// LoadByID 根据 ID 加载检查点
func (s *MemoryCheckpointSaver) LoadByID(ctx context.Context, id string) (*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	cp, ok := s.checkpoints[id]
	if !ok {
		return nil, fmt.Errorf("checkpoint %s not found", id)
	}

	return cloneCheckpoint(cp), nil
}

// List 列出线程的所有检查点
func (s *MemoryCheckpointSaver) List(ctx context.Context, threadID string) ([]*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.threads[threadID]
	if !ok {
		return nil, nil
	}

	result := make([]*Checkpoint, 0, len(ids))
	for _, id := range ids {
		if cp, ok := s.checkpoints[id]; ok {
			result = append(result, cloneCheckpoint(cp))
		}
	}

	return result, nil
}

// Delete 删除检查点
func (s *MemoryCheckpointSaver) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cp, ok := s.checkpoints[id]
	if !ok {
		return nil
	}

	delete(s.checkpoints, id)

	// 从线程列表中移除
	if ids, ok := s.threads[cp.ThreadID]; ok {
		newIDs := make([]string, 0, len(ids)-1)
		for _, existingID := range ids {
			if existingID != id {
				newIDs = append(newIDs, existingID)
			}
		}
		s.threads[cp.ThreadID] = newIDs
	}

	return nil
}

// DeleteThread 删除线程的所有检查点
func (s *MemoryCheckpointSaver) DeleteThread(ctx context.Context, threadID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ids, ok := s.threads[threadID]
	if !ok {
		return nil
	}

	for _, id := range ids {
		delete(s.checkpoints, id)
	}
	delete(s.threads, threadID)

	return nil
}

func cloneCheckpoint(cp *Checkpoint) *Checkpoint {
	if cp == nil {
		return nil
	}

	cloned := &Checkpoint{
		ID:              cp.ID,
		ThreadID:        cp.ThreadID,
		GraphName:       cp.GraphName,
		CurrentNode:     cp.CurrentNode,
		State:           append(json.RawMessage(nil), cp.State...),
		PendingNodes:    append([]string(nil), cp.PendingNodes...),
		CompletedNodes:  append([]string(nil), cp.CompletedNodes...),
		Metadata:        cloneMapAny(cp.Metadata),
		InterruptAddrs:  cloneMapRawMessage(cp.InterruptAddrs),
		InterruptStates: cloneMapRawMessage(cp.InterruptStates),
		CreatedAt:       cp.CreatedAt,
		UpdatedAt:       cp.UpdatedAt,
		ParentID:        cp.ParentID,
	}
	return cloned
}

func cloneMapRawMessage(src map[string]json.RawMessage) map[string]json.RawMessage {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]json.RawMessage, len(src))
	for k, v := range src {
		dst[k] = append(json.RawMessage(nil), v...)
	}
	return dst
}

func cloneMapAny(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ThreadConfig 线程配置
type ThreadConfig struct {
	// ThreadID 线程 ID
	ThreadID string

	// CheckpointSaver 检查点保存器
	CheckpointSaver CheckpointSaver

	// ResumeFromCheckpoint 是否从检查点恢复
	ResumeFromCheckpoint bool

	// CheckpointID 要恢复的检查点 ID（可选）
	CheckpointID string
}

// NewThreadConfig 创建线程配置
func NewThreadConfig(threadID string) *ThreadConfig {
	return &ThreadConfig{
		ThreadID:        threadID,
		CheckpointSaver: NewMemoryCheckpointSaver(),
	}
}

// WithCheckpointSaver 设置检查点保存器
func (c *ThreadConfig) WithCheckpointSaver(saver CheckpointSaver) *ThreadConfig {
	c.CheckpointSaver = saver
	return c
}

// WithResume 设置从检查点恢复
func (c *ThreadConfig) WithResume(checkpointID string) *ThreadConfig {
	c.ResumeFromCheckpoint = true
	c.CheckpointID = checkpointID
	return c
}

// generateCheckpointID 生成检查点 ID
func generateCheckpointID() string {
	return util.GenerateID("checkpoint")
}

// 确保实现了接口
var _ CheckpointSaver = (*MemoryCheckpointSaver)(nil)
