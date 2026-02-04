// Package graph 提供图编排引擎
//
// 本文件实现增强的检查点系统，对标 LangGraph：
//   - 分支执行支持
//   - 版本管理
//   - 状态差异追踪
//   - 从任意检查点恢复
//   - 检查点配置
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// ============== 增强检查点类型 ==============

// CheckpointVersion 检查点版本
type CheckpointVersion struct {
	Major int `json:"major"` // 主版本号（不兼容变更）
	Minor int `json:"minor"` // 次版本号（向后兼容变更）
	Patch int `json:"patch"` // 补丁版本号
}

// String 返回版本字符串
func (v CheckpointVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// CheckpointStatus 检查点状态
type CheckpointStatus string

const (
	// CheckpointStatusPending 等待执行
	CheckpointStatusPending CheckpointStatus = "pending"
	// CheckpointStatusRunning 执行中
	CheckpointStatusRunning CheckpointStatus = "running"
	// CheckpointStatusCompleted 已完成
	CheckpointStatusCompleted CheckpointStatus = "completed"
	// CheckpointStatusFailed 失败
	CheckpointStatusFailed CheckpointStatus = "failed"
	// CheckpointStatusInterrupted 中断
	CheckpointStatusInterrupted CheckpointStatus = "interrupted"
)

// EnhancedCheckpoint 增强检查点
// 支持分支、版本管理、状态差异等高级特性
type EnhancedCheckpoint struct {
	// 基本信息
	ID        string            `json:"id"`
	ThreadID  string            `json:"thread_id"`
	GraphName string            `json:"graph_name"`
	Version   CheckpointVersion `json:"version"`
	Status    CheckpointStatus  `json:"status"`

	// 执行状态
	CurrentNode    string   `json:"current_node"`
	PendingNodes   []string `json:"pending_nodes"`
	CompletedNodes []string `json:"completed_nodes"`
	VisitedNodes   []string `json:"visited_nodes"` // 所有访问过的节点（含重复）

	// 状态数据
	State     json.RawMessage `json:"state"`
	StateDiff json.RawMessage `json:"state_diff,omitempty"` // 与父检查点的差异
	StateHash string          `json:"state_hash,omitempty"` // 状态哈希，用于快速比较

	// 分支支持
	ParentID   string   `json:"parent_id,omitempty"`   // 父检查点 ID
	BranchID   string   `json:"branch_id,omitempty"`   // 分支 ID
	BranchName string   `json:"branch_name,omitempty"` // 分支名称
	ChildIDs   []string `json:"child_ids,omitempty"`   // 子检查点 ID 列表

	// 元数据
	Metadata    map[string]any `json:"metadata,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Description string         `json:"description,omitempty"`

	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 执行统计
	Stats *CheckpointStats `json:"stats,omitempty"`
}

// CheckpointStats 检查点统计
type CheckpointStats struct {
	StepCount     int                      `json:"step_count"`               // 步骤计数
	TotalDuration time.Duration            `json:"total_duration"`           // 总耗时
	NodeDurations map[string]time.Duration `json:"node_durations,omitempty"` // 各节点耗时
	LLMTokens     int                      `json:"llm_tokens,omitempty"`     // LLM Token 数
	ToolCalls     int                      `json:"tool_calls,omitempty"`     // 工具调用次数
}

// ============== 增强检查点保存器接口 ==============

// EnhancedCheckpointSaver 增强检查点保存器接口
type EnhancedCheckpointSaver interface {
	CheckpointSaver // 继承基础接口

	// SaveEnhanced 保存增强检查点
	SaveEnhanced(ctx context.Context, checkpoint *EnhancedCheckpoint) error

	// LoadEnhanced 加载最新的增强检查点
	LoadEnhanced(ctx context.Context, threadID string) (*EnhancedCheckpoint, error)

	// LoadEnhancedByID 根据 ID 加载增强检查点
	LoadEnhancedByID(ctx context.Context, id string) (*EnhancedCheckpoint, error)

	// ListEnhanced 列出线程的所有增强检查点
	ListEnhanced(ctx context.Context, threadID string, opts *ListOptions) ([]*EnhancedCheckpoint, error)

	// GetHistory 获取检查点历史链
	GetHistory(ctx context.Context, checkpointID string, limit int) ([]*EnhancedCheckpoint, error)

	// GetBranches 获取分支列表
	GetBranches(ctx context.Context, threadID string) ([]*BranchInfo, error)

	// CreateBranch 从检查点创建分支
	CreateBranch(ctx context.Context, checkpointID string, branchName string) (*EnhancedCheckpoint, error)

	// MergeBranch 合并分支
	MergeBranch(ctx context.Context, sourceBranchID, targetBranchID string, strategy MergeStrategy) (*EnhancedCheckpoint, error)

	// Search 搜索检查点
	Search(ctx context.Context, query *CheckpointQuery) ([]*EnhancedCheckpoint, error)

	// Cleanup 清理旧检查点
	Cleanup(ctx context.Context, policy *CleanupPolicy) (int, error)
}

// ListOptions 列表选项
type ListOptions struct {
	Limit     int              // 限制数量
	Offset    int              // 偏移量
	Order     string           // 排序方式: "asc" 或 "desc"
	Status    CheckpointStatus // 状态过滤
	BranchID  string           // 分支过滤
	StartTime *time.Time       // 开始时间
	EndTime   *time.Time       // 结束时间
}

// BranchInfo 分支信息
type BranchInfo struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	ThreadID           string    `json:"thread_id"`
	BaseCheckpointID   string    `json:"base_checkpoint_id"`
	LatestCheckpointID string    `json:"latest_checkpoint_id"`
	CheckpointCount    int       `json:"checkpoint_count"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// MergeStrategy 合并策略
type MergeStrategy string

const (
	// MergeStrategyOverwrite 覆盖目标状态
	MergeStrategyOverwrite MergeStrategy = "overwrite"
	// MergeStrategyMerge 合并状态
	MergeStrategyMerge MergeStrategy = "merge"
	// MergeStrategyKeepBoth 保留两者
	MergeStrategyKeepBoth MergeStrategy = "keep_both"
)

// CheckpointQuery 检查点查询
type CheckpointQuery struct {
	ThreadID  string           `json:"thread_id,omitempty"`
	GraphName string           `json:"graph_name,omitempty"`
	Status    CheckpointStatus `json:"status,omitempty"`
	BranchID  string           `json:"branch_id,omitempty"`
	Tags      []string         `json:"tags,omitempty"`
	StartTime *time.Time       `json:"start_time,omitempty"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Limit     int              `json:"limit,omitempty"`
	Offset    int              `json:"offset,omitempty"`
}

// CleanupPolicy 清理策略
type CleanupPolicy struct {
	// MaxAge 最大保留时间
	MaxAge time.Duration `json:"max_age,omitempty"`
	// MaxCount 最大保留数量（每个线程）
	MaxCount int `json:"max_count,omitempty"`
	// KeepCompleted 是否保留已完成的检查点
	KeepCompleted bool `json:"keep_completed,omitempty"`
	// KeepBranchHeads 是否保留分支头
	KeepBranchHeads bool `json:"keep_branch_heads,omitempty"`
	// KeepTagged 是否保留有标签的检查点
	KeepTagged bool `json:"keep_tagged,omitempty"`
}

// ============== 内存增强检查点保存器 ==============

// MemoryEnhancedCheckpointSaver 内存增强检查点保存器
type MemoryEnhancedCheckpointSaver struct {
	*MemoryCheckpointSaver
	enhanced map[string]*EnhancedCheckpoint
	branches map[string]*BranchInfo
	mu       sync.RWMutex
}

// NewMemoryEnhancedCheckpointSaver 创建内存增强检查点保存器
func NewMemoryEnhancedCheckpointSaver() *MemoryEnhancedCheckpointSaver {
	return &MemoryEnhancedCheckpointSaver{
		MemoryCheckpointSaver: NewMemoryCheckpointSaver(),
		enhanced:              make(map[string]*EnhancedCheckpoint),
		branches:              make(map[string]*BranchInfo),
	}
}

// SaveEnhanced 保存增强检查点
func (s *MemoryEnhancedCheckpointSaver) SaveEnhanced(ctx context.Context, checkpoint *EnhancedCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if checkpoint.ID == "" {
		checkpoint.ID = util.GenerateID("ckpt")
	}
	checkpoint.UpdatedAt = time.Now()
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = checkpoint.UpdatedAt
	}

	// 计算状态哈希
	if checkpoint.State != nil {
		checkpoint.StateHash = hashState(checkpoint.State)
	}

	// 更新父检查点的子列表
	if checkpoint.ParentID != "" {
		if parent, ok := s.enhanced[checkpoint.ParentID]; ok {
			parent.ChildIDs = append(parent.ChildIDs, checkpoint.ID)
		}
	}

	s.enhanced[checkpoint.ID] = checkpoint
	s.threads[checkpoint.ThreadID] = append(s.threads[checkpoint.ThreadID], checkpoint.ID)

	// 更新分支信息
	if checkpoint.BranchID != "" {
		if branch, ok := s.branches[checkpoint.BranchID]; ok {
			branch.LatestCheckpointID = checkpoint.ID
			branch.CheckpointCount++
			branch.UpdatedAt = time.Now()
		}
	}

	return nil
}

// LoadEnhanced 加载最新的增强检查点
func (s *MemoryEnhancedCheckpointSaver) LoadEnhanced(ctx context.Context, threadID string) (*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.threads[threadID]
	if !ok || len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoint found for thread %s", threadID)
	}

	latestID := ids[len(ids)-1]
	cp, ok := s.enhanced[latestID]
	if !ok {
		return nil, fmt.Errorf("checkpoint %s not found", latestID)
	}

	return cp, nil
}

// LoadEnhancedByID 根据 ID 加载增强检查点
func (s *MemoryEnhancedCheckpointSaver) LoadEnhancedByID(ctx context.Context, id string) (*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp, ok := s.enhanced[id]
	if !ok {
		return nil, fmt.Errorf("checkpoint %s not found", id)
	}

	return cp, nil
}

// ListEnhanced 列出线程的所有增强检查点
func (s *MemoryEnhancedCheckpointSaver) ListEnhanced(ctx context.Context, threadID string, opts *ListOptions) ([]*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.threads[threadID]
	if !ok {
		return nil, nil
	}

	result := make([]*EnhancedCheckpoint, 0, len(ids))
	for _, id := range ids {
		if cp, ok := s.enhanced[id]; ok {
			// 应用过滤
			if opts != nil {
				if opts.Status != "" && cp.Status != opts.Status {
					continue
				}
				if opts.BranchID != "" && cp.BranchID != opts.BranchID {
					continue
				}
				if opts.StartTime != nil && cp.CreatedAt.Before(*opts.StartTime) {
					continue
				}
				if opts.EndTime != nil && cp.CreatedAt.After(*opts.EndTime) {
					continue
				}
			}
			result = append(result, cp)
		}
	}

	// 应用分页
	if opts != nil && opts.Limit > 0 {
		start := opts.Offset
		if start >= len(result) {
			return nil, nil
		}
		end := start + opts.Limit
		if end > len(result) {
			end = len(result)
		}
		result = result[start:end]
	}

	return result, nil
}

// GetHistory 获取检查点历史链
func (s *MemoryEnhancedCheckpointSaver) GetHistory(ctx context.Context, checkpointID string, limit int) ([]*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var history []*EnhancedCheckpoint
	currentID := checkpointID

	for currentID != "" && (limit <= 0 || len(history) < limit) {
		cp, ok := s.enhanced[currentID]
		if !ok {
			break
		}
		history = append(history, cp)
		currentID = cp.ParentID
	}

	return history, nil
}

// GetBranches 获取分支列表
func (s *MemoryEnhancedCheckpointSaver) GetBranches(ctx context.Context, threadID string) ([]*BranchInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*BranchInfo
	for _, branch := range s.branches {
		if branch.ThreadID == threadID {
			result = append(result, branch)
		}
	}

	return result, nil
}

// CreateBranch 从检查点创建分支
func (s *MemoryEnhancedCheckpointSaver) CreateBranch(ctx context.Context, checkpointID string, branchName string) (*EnhancedCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取源检查点
	source, ok := s.enhanced[checkpointID]
	if !ok {
		return nil, fmt.Errorf("checkpoint %s not found", checkpointID)
	}

	// 创建分支
	branchID := util.GenerateID("branch")
	branch := &BranchInfo{
		ID:               branchID,
		Name:             branchName,
		ThreadID:         source.ThreadID,
		BaseCheckpointID: checkpointID,
		CheckpointCount:  0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	s.branches[branchID] = branch

	// 创建新检查点
	newCheckpoint := &EnhancedCheckpoint{
		ID:             util.GenerateID("ckpt"),
		ThreadID:       source.ThreadID,
		GraphName:      source.GraphName,
		Version:        source.Version,
		Status:         CheckpointStatusPending,
		CurrentNode:    source.CurrentNode,
		PendingNodes:   append([]string{}, source.PendingNodes...),
		CompletedNodes: append([]string{}, source.CompletedNodes...),
		State:          append(json.RawMessage{}, source.State...),
		ParentID:       checkpointID,
		BranchID:       branchID,
		BranchName:     branchName,
		Metadata:       copyMetadata(source.Metadata),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	s.enhanced[newCheckpoint.ID] = newCheckpoint
	s.threads[newCheckpoint.ThreadID] = append(s.threads[newCheckpoint.ThreadID], newCheckpoint.ID)

	branch.LatestCheckpointID = newCheckpoint.ID
	branch.CheckpointCount = 1

	// 更新源检查点的子列表
	source.ChildIDs = append(source.ChildIDs, newCheckpoint.ID)

	return newCheckpoint, nil
}

// MergeBranch 合并分支
func (s *MemoryEnhancedCheckpointSaver) MergeBranch(ctx context.Context, sourceBranchID, targetBranchID string, strategy MergeStrategy) (*EnhancedCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sourceBranch, ok := s.branches[sourceBranchID]
	if !ok {
		return nil, fmt.Errorf("source branch %s not found", sourceBranchID)
	}

	targetBranch, ok := s.branches[targetBranchID]
	if !ok {
		return nil, fmt.Errorf("target branch %s not found", targetBranchID)
	}

	sourceCP, ok := s.enhanced[sourceBranch.LatestCheckpointID]
	if !ok {
		return nil, fmt.Errorf("source checkpoint not found")
	}

	targetCP, ok := s.enhanced[targetBranch.LatestCheckpointID]
	if !ok {
		return nil, fmt.Errorf("target checkpoint not found")
	}

	// 根据策略合并
	var mergedState json.RawMessage
	switch strategy {
	case MergeStrategyOverwrite:
		mergedState = append(json.RawMessage{}, sourceCP.State...)
	case MergeStrategyKeepBoth:
		mergedState = append(json.RawMessage{}, targetCP.State...)
	default:
		// 默认合并策略 - 简单使用源状态
		mergedState = append(json.RawMessage{}, sourceCP.State...)
	}

	// 创建合并检查点
	merged := &EnhancedCheckpoint{
		ID:             util.GenerateID("ckpt"),
		ThreadID:       targetCP.ThreadID,
		GraphName:      targetCP.GraphName,
		Version:        targetCP.Version,
		Status:         CheckpointStatusPending,
		CurrentNode:    sourceCP.CurrentNode,
		PendingNodes:   append([]string{}, sourceCP.PendingNodes...),
		CompletedNodes: mergeSlices(sourceCP.CompletedNodes, targetCP.CompletedNodes),
		State:          mergedState,
		ParentID:       targetCP.ID,
		BranchID:       targetBranchID,
		Metadata: map[string]any{
			"merged_from":    sourceBranchID,
			"merge_strategy": string(strategy),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.enhanced[merged.ID] = merged
	s.threads[merged.ThreadID] = append(s.threads[merged.ThreadID], merged.ID)

	targetBranch.LatestCheckpointID = merged.ID
	targetBranch.CheckpointCount++
	targetBranch.UpdatedAt = time.Now()

	return merged, nil
}

// Search 搜索检查点
func (s *MemoryEnhancedCheckpointSaver) Search(ctx context.Context, query *CheckpointQuery) ([]*EnhancedCheckpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*EnhancedCheckpoint
	for _, cp := range s.enhanced {
		if matchesQuery(cp, query) {
			result = append(result, cp)
		}
	}

	// 应用分页
	if query.Limit > 0 {
		start := query.Offset
		if start >= len(result) {
			return nil, nil
		}
		end := start + query.Limit
		if end > len(result) {
			end = len(result)
		}
		result = result[start:end]
	}

	return result, nil
}

// Cleanup 清理旧检查点
func (s *MemoryEnhancedCheckpointSaver) Cleanup(ctx context.Context, policy *CleanupPolicy) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var toDelete []string
	now := time.Now()

	for id, cp := range s.enhanced {
		// 检查是否保护
		if policy.KeepCompleted && cp.Status == CheckpointStatusCompleted {
			continue
		}
		if policy.KeepTagged && len(cp.Tags) > 0 {
			continue
		}
		if policy.KeepBranchHeads {
			isBranchHead := false
			for _, branch := range s.branches {
				if branch.LatestCheckpointID == id {
					isBranchHead = true
					break
				}
			}
			if isBranchHead {
				continue
			}
		}

		// 检查年龄
		if policy.MaxAge > 0 && now.Sub(cp.CreatedAt) > policy.MaxAge {
			toDelete = append(toDelete, id)
		}
	}

	// 删除
	for _, id := range toDelete {
		if cp, ok := s.enhanced[id]; ok {
			delete(s.enhanced, id)
			// 从线程列表移除
			if ids, ok := s.threads[cp.ThreadID]; ok {
				newIDs := make([]string, 0, len(ids)-1)
				for _, existingID := range ids {
					if existingID != id {
						newIDs = append(newIDs, existingID)
					}
				}
				s.threads[cp.ThreadID] = newIDs
			}
		}
	}

	return len(toDelete), nil
}

// ============== 辅助函数 ==============

// hashState 计算状态哈希
func hashState(state json.RawMessage) string {
	// 简化实现：使用内容长度和前几个字节
	if len(state) == 0 {
		return "empty"
	}
	return fmt.Sprintf("%d:%x", len(state), state[:min(32, len(state))])
}

// copyMetadata 复制元数据
func copyMetadata(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// mergeSlices 合并切片（去重）
func mergeSlices(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// matchesQuery 检查检查点是否匹配查询
func matchesQuery(cp *EnhancedCheckpoint, query *CheckpointQuery) bool {
	if query.ThreadID != "" && cp.ThreadID != query.ThreadID {
		return false
	}
	if query.GraphName != "" && cp.GraphName != query.GraphName {
		return false
	}
	if query.Status != "" && cp.Status != query.Status {
		return false
	}
	if query.BranchID != "" && cp.BranchID != query.BranchID {
		return false
	}
	if query.StartTime != nil && cp.CreatedAt.Before(*query.StartTime) {
		return false
	}
	if query.EndTime != nil && cp.CreatedAt.After(*query.EndTime) {
		return false
	}
	// 检查标签
	if len(query.Tags) > 0 {
		for _, tag := range query.Tags {
			found := false
			for _, cpTag := range cp.Tags {
				if cpTag == tag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// min 返回较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 确保实现了接口
var _ EnhancedCheckpointSaver = (*MemoryEnhancedCheckpointSaver)(nil)
