// Package graph 提供图编排引擎
//
// 本文件实现检查点恢复执行器：
//   - 从任意检查点恢复执行
//   - 支持断点续跑
//   - 支持分支执行
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CheckpointRunner 检查点恢复执行器
// 支持从任意检查点恢复图执行
type CheckpointRunner[S State] struct {
	graph  *Graph[S]
	saver  EnhancedCheckpointSaver
	config *CheckpointRunnerConfig

	// 当前执行状态
	currentCheckpoint *EnhancedCheckpoint
	threadID          string
	branchID          string
}

// CheckpointRunnerConfig 检查点执行器配置
type CheckpointRunnerConfig struct {
	// AutoSave 是否自动保存检查点
	AutoSave bool
	// SaveInterval 保存间隔（每执行多少个节点保存一次）
	SaveInterval int
	// SaveOnError 错误时是否保存检查点
	SaveOnError bool
	// SaveOnInterrupt 中断时是否保存检查点
	SaveOnInterrupt bool
	// MaxRetries 最大重试次数
	MaxRetries int
	// RetryDelay 重试延迟
	RetryDelay time.Duration
}

// DefaultCheckpointRunnerConfig 默认配置
func DefaultCheckpointRunnerConfig() *CheckpointRunnerConfig {
	return &CheckpointRunnerConfig{
		AutoSave:        true,
		SaveInterval:    1,
		SaveOnError:     true,
		SaveOnInterrupt: true,
		MaxRetries:      3,
		RetryDelay:      time.Second,
	}
}

// NewCheckpointRunner 创建检查点执行器
func NewCheckpointRunner[S State](g *Graph[S], saver EnhancedCheckpointSaver, config *CheckpointRunnerConfig) *CheckpointRunner[S] {
	if config == nil {
		config = DefaultCheckpointRunnerConfig()
	}
	return &CheckpointRunner[S]{
		graph:  g,
		saver:  saver,
		config: config,
	}
}

// Run 从初始状态运行
func (r *CheckpointRunner[S]) Run(ctx context.Context, threadID string, initialState S) (S, error) {
	r.threadID = threadID

	// 创建初始检查点
	stateBytes, err := json.Marshal(initialState)
	if err != nil {
		return initialState, fmt.Errorf("marshal initial state: %w", err)
	}

	r.currentCheckpoint = &EnhancedCheckpoint{
		ThreadID:       threadID,
		GraphName:      r.graph.Name,
		Version:        CheckpointVersion{Major: 1, Minor: 0, Patch: 0},
		Status:         CheckpointStatusRunning,
		CurrentNode:    START,
		PendingNodes:   r.getInitialPendingNodes(),
		CompletedNodes: []string{},
		VisitedNodes:   []string{START},
		State:          stateBytes,
		Stats:          &CheckpointStats{StepCount: 0},
		Metadata:       make(map[string]any),
	}

	if err := r.saver.SaveEnhanced(ctx, r.currentCheckpoint); err != nil {
		return initialState, fmt.Errorf("save initial checkpoint: %w", err)
	}

	return r.executeFromCheckpoint(ctx, initialState)
}

// Resume 从检查点恢复执行
func (r *CheckpointRunner[S]) Resume(ctx context.Context, checkpointID string) (S, error) {
	var zero S

	// 加载检查点
	checkpoint, err := r.saver.LoadEnhancedByID(ctx, checkpointID)
	if err != nil {
		return zero, fmt.Errorf("load checkpoint: %w", err)
	}

	r.currentCheckpoint = checkpoint
	r.threadID = checkpoint.ThreadID
	r.branchID = checkpoint.BranchID

	// 反序列化状态
	var state S
	if err := json.Unmarshal(checkpoint.State, &state); err != nil {
		return zero, fmt.Errorf("unmarshal state: %w", err)
	}

	// 更新检查点状态
	checkpoint.Status = CheckpointStatusRunning
	if err := r.saver.SaveEnhanced(ctx, checkpoint); err != nil {
		return zero, fmt.Errorf("save checkpoint: %w", err)
	}

	return r.executeFromCheckpoint(ctx, state)
}

// ResumeFromLatest 从最新检查点恢复
func (r *CheckpointRunner[S]) ResumeFromLatest(ctx context.Context, threadID string) (S, error) {
	var zero S

	checkpoint, err := r.saver.LoadEnhanced(ctx, threadID)
	if err != nil {
		return zero, fmt.Errorf("load latest checkpoint: %w", err)
	}

	return r.Resume(ctx, checkpoint.ID)
}

// Fork 从检查点创建分支并执行
func (r *CheckpointRunner[S]) Fork(ctx context.Context, checkpointID string, branchName string, modifyState func(S) S) (S, error) {
	var zero S

	// 创建分支
	newCheckpoint, err := r.saver.CreateBranch(ctx, checkpointID, branchName)
	if err != nil {
		return zero, fmt.Errorf("create branch: %w", err)
	}

	// 反序列化状态
	var state S
	if err := json.Unmarshal(newCheckpoint.State, &state); err != nil {
		return zero, fmt.Errorf("unmarshal state: %w", err)
	}

	// 修改状态（如果提供了修改函数）
	if modifyState != nil {
		state = modifyState(state)
		stateBytes, err := json.Marshal(state)
		if err != nil {
			return zero, fmt.Errorf("marshal modified state: %w", err)
		}
		newCheckpoint.State = stateBytes
	}

	r.currentCheckpoint = newCheckpoint
	r.threadID = newCheckpoint.ThreadID
	r.branchID = newCheckpoint.BranchID

	return r.executeFromCheckpoint(ctx, state)
}

// executeFromCheckpoint 从检查点执行
func (r *CheckpointRunner[S]) executeFromCheckpoint(ctx context.Context, state S) (S, error) {
	startTime := time.Now()
	stepCount := 0

	// 确保 Stats 和 Metadata 已初始化
	if r.currentCheckpoint.Stats == nil {
		r.currentCheckpoint.Stats = &CheckpointStats{}
	}
	if r.currentCheckpoint.Metadata == nil {
		r.currentCheckpoint.Metadata = make(map[string]any)
	}

	// 获取待执行节点
	pendingNodes := r.currentCheckpoint.PendingNodes
	if len(pendingNodes) == 0 {
		// 如果没有待执行节点，从当前节点获取后继
		pendingNodes = r.getSuccessors(r.currentCheckpoint.CurrentNode, state)
	}

	for len(pendingNodes) > 0 {
		select {
		case <-ctx.Done():
			// 上下文取消，保存中断检查点
			if r.config.SaveOnInterrupt {
				r.saveInterruptCheckpoint(ctx, state, pendingNodes)
			}
			return state, ctx.Err()
		default:
		}

		// 获取下一个节点
		currentNode := pendingNodes[0]
		pendingNodes = pendingNodes[1:]

		// 检查是否到达终点
		if currentNode == END {
			r.currentCheckpoint.Status = CheckpointStatusCompleted
			r.currentCheckpoint.CurrentNode = END
			r.currentCheckpoint.PendingNodes = nil
			r.currentCheckpoint.Stats.TotalDuration = time.Since(startTime)
			r.saver.SaveEnhanced(ctx, r.currentCheckpoint)
			return state, nil
		}

		// 执行节点
		nodeStart := time.Now()
		var err error
		state, err = r.executeNode(ctx, currentNode, state)
		nodeDuration := time.Since(nodeStart)

		if err != nil {
			// 错误处理
			if r.config.SaveOnError {
				r.saveErrorCheckpoint(ctx, state, currentNode, pendingNodes, err)
			}
			return state, fmt.Errorf("execute node %s: %w", currentNode, err)
		}

		stepCount++

		// 更新检查点
		r.currentCheckpoint.CurrentNode = currentNode
		r.currentCheckpoint.CompletedNodes = append(r.currentCheckpoint.CompletedNodes, currentNode)
		r.currentCheckpoint.VisitedNodes = append(r.currentCheckpoint.VisitedNodes, currentNode)
		r.currentCheckpoint.Stats.StepCount = stepCount
		if r.currentCheckpoint.Stats.NodeDurations == nil {
			r.currentCheckpoint.Stats.NodeDurations = make(map[string]time.Duration)
		}
		r.currentCheckpoint.Stats.NodeDurations[currentNode] = nodeDuration

		// 获取后继节点
		successors := r.getSuccessors(currentNode, state)
		pendingNodes = append(successors, pendingNodes...)
		r.currentCheckpoint.PendingNodes = pendingNodes

		// 序列化状态
		stateBytes, err := json.Marshal(state)
		if err == nil {
			r.currentCheckpoint.State = stateBytes
		}

		// 自动保存检查点
		if r.config.AutoSave && stepCount%r.config.SaveInterval == 0 {
			r.saver.SaveEnhanced(ctx, r.currentCheckpoint)
		}
	}

	// 完成
	r.currentCheckpoint.Status = CheckpointStatusCompleted
	r.currentCheckpoint.Stats.TotalDuration = time.Since(startTime)
	r.saver.SaveEnhanced(ctx, r.currentCheckpoint)

	return state, nil
}

// executeNode 执行单个节点
func (r *CheckpointRunner[S]) executeNode(ctx context.Context, nodeName string, state S) (S, error) {
	node, ok := r.graph.Nodes[nodeName]
	if !ok {
		return state, fmt.Errorf("node %s not found", nodeName)
	}

	// 支持重试
	var lastErr error
	for i := 0; i <= r.config.MaxRetries; i++ {
		newState, err := node.Handler(ctx, state)
		if err == nil {
			return newState, nil
		}
		lastErr = err

		if i < r.config.MaxRetries {
			select {
			case <-ctx.Done():
				return state, ctx.Err()
			case <-time.After(r.config.RetryDelay):
			}
		}
	}

	return state, lastErr
}

// getSuccessors 获取后继节点
func (r *CheckpointRunner[S]) getSuccessors(nodeName string, state S) []string {
	var successors []string

	// 检查条件边
	if condEdges, ok := r.graph.conditionalEdges[nodeName]; ok {
		for _, ce := range condEdges {
			label := ce.router(state)
			if target, ok := ce.edges[label]; ok {
				successors = append(successors, target)
			}
		}
	}

	// 检查普通边
	if targets, ok := r.graph.adjacency[nodeName]; ok {
		successors = append(successors, targets...)
	}

	return successors
}

// getInitialPendingNodes 获取初始待执行节点
func (r *CheckpointRunner[S]) getInitialPendingNodes() []string {
	if targets, ok := r.graph.adjacency[START]; ok {
		return targets
	}
	if r.graph.EntryPoint != "" {
		return []string{r.graph.EntryPoint}
	}
	return nil
}

// saveInterruptCheckpoint 保存中断检查点
func (r *CheckpointRunner[S]) saveInterruptCheckpoint(ctx context.Context, state S, pendingNodes []string) {
	stateBytes, _ := json.Marshal(state)
	r.currentCheckpoint.State = stateBytes
	r.currentCheckpoint.PendingNodes = pendingNodes
	r.currentCheckpoint.Status = CheckpointStatusInterrupted
	r.saver.SaveEnhanced(context.Background(), r.currentCheckpoint)
}

// saveErrorCheckpoint 保存错误检查点
func (r *CheckpointRunner[S]) saveErrorCheckpoint(ctx context.Context, state S, failedNode string, pendingNodes []string, err error) {
	stateBytes, _ := json.Marshal(state)
	r.currentCheckpoint.State = stateBytes
	r.currentCheckpoint.PendingNodes = append([]string{failedNode}, pendingNodes...)
	r.currentCheckpoint.Status = CheckpointStatusFailed
	r.currentCheckpoint.Metadata["error"] = err.Error()
	r.currentCheckpoint.Metadata["failed_node"] = failedNode
	r.saver.SaveEnhanced(context.Background(), r.currentCheckpoint)
}

// GetCurrentCheckpoint 获取当前检查点
func (r *CheckpointRunner[S]) GetCurrentCheckpoint() *EnhancedCheckpoint {
	return r.currentCheckpoint
}

// GetHistory 获取执行历史
func (r *CheckpointRunner[S]) GetHistory(ctx context.Context, limit int) ([]*EnhancedCheckpoint, error) {
	if r.currentCheckpoint == nil {
		return nil, nil
	}
	return r.saver.GetHistory(ctx, r.currentCheckpoint.ID, limit)
}
