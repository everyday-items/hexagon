package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// WorkflowStore 工作流存储接口
type WorkflowStore interface {
	// SaveWorkflow 保存工作流定义
	SaveWorkflow(ctx context.Context, wf *Workflow) error

	// GetWorkflow 获取工作流定义
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)

	// ListWorkflows 列出工作流定义
	ListWorkflows(ctx context.Context, limit int) ([]*Workflow, error)

	// DeleteWorkflow 删除工作流定义
	DeleteWorkflow(ctx context.Context, id string) error

	// SaveExecution 保存执行实例
	SaveExecution(ctx context.Context, execution *WorkflowExecution) error

	// GetExecution 获取执行实例
	GetExecution(ctx context.Context, id string) (*WorkflowExecution, error)

	// ListExecutions 列出执行实例
	ListExecutions(ctx context.Context, workflowID string, status WorkflowStatus, limit int) ([]*WorkflowExecution, error)

	// DeleteExecution 删除执行实例
	DeleteExecution(ctx context.Context, id string) error

	// UpdateExecutionStatus 更新执行状态
	UpdateExecutionStatus(ctx context.Context, id string, status WorkflowStatus, errMsg string) error

	// GetPendingExecutions 获取待恢复的执行实例
	GetPendingExecutions(ctx context.Context, limit int) ([]*WorkflowExecution, error)
}

// MemoryWorkflowStore 内存存储实现
type MemoryWorkflowStore struct {
	workflows  map[string]*Workflow
	executions map[string]*WorkflowExecution
	mu         sync.RWMutex
}

// NewMemoryWorkflowStore 创建内存存储
func NewMemoryWorkflowStore() *MemoryWorkflowStore {
	return &MemoryWorkflowStore{
		workflows:  make(map[string]*Workflow),
		executions: make(map[string]*WorkflowExecution),
	}
}

// SaveWorkflow 保存工作流定义
func (s *MemoryWorkflowStore) SaveWorkflow(ctx context.Context, wf *Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf.CreatedAt.IsZero() {
		wf.CreatedAt = time.Now()
	}
	s.workflows[wf.ID] = wf
	return nil
}

// GetWorkflow 获取工作流定义
func (s *MemoryWorkflowStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wf, ok := s.workflows[id]
	if !ok {
		return nil, fmt.Errorf("workflow %s not found", id)
	}
	return wf, nil
}

// ListWorkflows 列出工作流定义
func (s *MemoryWorkflowStore) ListWorkflows(ctx context.Context, limit int) ([]*Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Workflow, 0, len(s.workflows))
	for _, wf := range s.workflows {
		result = append(result, wf)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	// 按创建时间排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// DeleteWorkflow 删除工作流定义
func (s *MemoryWorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workflows, id)
	return nil
}

// SaveExecution 保存执行实例
func (s *MemoryWorkflowStore) SaveExecution(ctx context.Context, execution *WorkflowExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 深拷贝
	data, _ := json.Marshal(execution)
	var copy WorkflowExecution
	json.Unmarshal(data, &copy)

	s.executions[execution.ID] = &copy
	return nil
}

// GetExecution 获取执行实例
func (s *MemoryWorkflowStore) GetExecution(ctx context.Context, id string) (*WorkflowExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	execution, ok := s.executions[id]
	if !ok {
		return nil, fmt.Errorf("execution %s not found", id)
	}
	return execution, nil
}

// ListExecutions 列出执行实例
func (s *MemoryWorkflowStore) ListExecutions(ctx context.Context, workflowID string, status WorkflowStatus, limit int) ([]*WorkflowExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkflowExecution
	for _, execution := range s.executions {
		if workflowID != "" && execution.WorkflowID != workflowID {
			continue
		}
		if status != "" && execution.Status != status {
			continue
		}
		result = append(result, execution)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	// 按开始时间排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})

	return result, nil
}

// DeleteExecution 删除执行实例
func (s *MemoryWorkflowStore) DeleteExecution(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.executions, id)
	return nil
}

// UpdateExecutionStatus 更新执行状态
func (s *MemoryWorkflowStore) UpdateExecutionStatus(ctx context.Context, id string, status WorkflowStatus, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	execution, ok := s.executions[id]
	if !ok {
		return fmt.Errorf("execution %s not found", id)
	}

	execution.Status = status
	if errMsg != "" {
		execution.Error = errMsg
	}

	now := time.Now()
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		execution.CompletedAt = &now
		execution.Duration = now.Sub(execution.StartedAt)
	case StatusPaused:
		execution.PausedAt = &now
	}

	return nil
}

// GetPendingExecutions 获取待恢复的执行实例
func (s *MemoryWorkflowStore) GetPendingExecutions(ctx context.Context, limit int) ([]*WorkflowExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkflowExecution
	for _, execution := range s.executions {
		if execution.Status == StatusRunning || execution.Status == StatusPaused {
			result = append(result, execution)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// Clear 清空存储
func (s *MemoryWorkflowStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows = make(map[string]*Workflow)
	s.executions = make(map[string]*WorkflowExecution)
}

// Stats 返回存储统计
func (s *MemoryWorkflowStore) Stats() (workflowCount, executionCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.workflows), len(s.executions)
}

// ExecutionSnapshot 执行快照（用于恢复）
type ExecutionSnapshot struct {
	// ExecutionID 执行实例 ID
	ExecutionID string `json:"execution_id"`

	// WorkflowID 工作流 ID
	WorkflowID string `json:"workflow_id"`

	// Status 状态
	Status WorkflowStatus `json:"status"`

	// Context 上下文
	Context *ExecutionContext `json:"context"`

	// StepResults 步骤结果
	StepResults map[string]*StepResult `json:"step_results"`

	// Input 输入
	Input json.RawMessage `json:"input"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
}

// CreateSnapshot 创建执行快照
func CreateSnapshot(execution *WorkflowExecution) *ExecutionSnapshot {
	return &ExecutionSnapshot{
		ExecutionID: execution.ID,
		WorkflowID:  execution.WorkflowID,
		Status:      execution.Status,
		Context:     execution.Context,
		StepResults: execution.StepResults,
		Input:       execution.Input,
		CreatedAt:   time.Now(),
	}
}

// RestoreFromSnapshot 从快照恢复执行
func RestoreFromSnapshot(snapshot *ExecutionSnapshot) *WorkflowExecution {
	return &WorkflowExecution{
		ID:          snapshot.ExecutionID,
		WorkflowID:  snapshot.WorkflowID,
		Status:      snapshot.Status,
		Context:     snapshot.Context,
		StepResults: snapshot.StepResults,
		Input:       snapshot.Input,
	}
}

// ExecutionRecovery 执行恢复器
type ExecutionRecovery struct {
	store    WorkflowStore
	registry *WorkflowRegistry
	executor *Executor
}

// NewExecutionRecovery 创建执行恢复器
func NewExecutionRecovery(store WorkflowStore, registry *WorkflowRegistry, executor *Executor) *ExecutionRecovery {
	return &ExecutionRecovery{
		store:    store,
		registry: registry,
		executor: executor,
	}
}

// RecoverPending 恢复待处理的执行
func (r *ExecutionRecovery) RecoverPending(ctx context.Context) (int, error) {
	executions, err := r.store.GetPendingExecutions(ctx, 100)
	if err != nil {
		return 0, fmt.Errorf("get pending executions: %w", err)
	}

	recovered := 0
	for _, execution := range executions {
		wf, ok := r.registry.Get(execution.WorkflowID)
		if !ok {
			// 尝试从存储加载工作流
			wf, err = r.store.GetWorkflow(ctx, execution.WorkflowID)
			if err != nil {
				continue
			}
		}

		// 恢复执行
		var input WorkflowInput
		if execution.Input != nil {
			json.Unmarshal(execution.Input, &input)
		}
		input.Variables = execution.Context.Variables
		input.Metadata = execution.Context.Metadata

		if _, err := r.executor.RunAsync(ctx, wf, input); err != nil {
			continue
		}

		recovered++
	}

	return recovered, nil
}

// 确保实现了接口
var _ WorkflowStore = (*MemoryWorkflowStore)(nil)
