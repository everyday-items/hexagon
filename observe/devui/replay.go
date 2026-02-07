package devui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/toolkit/util/idgen"
)

// ReplayManager 调试回放管理器
//
// 提供执行历史的录制和回放功能，支持：
//   - 自动录制每次图执行的完整过程
//   - 按步骤回放执行过程（前进/后退）
//   - 查看每步的状态快照
//   - 比较不同执行的差异
//   - 支持断点和条件断点
//
// 与 Collector 集成，自动从事件流中捕获执行数据。
type ReplayManager struct {
	// sessions 录制的会话列表
	sessions map[string]*ReplaySession

	// maxSessions 最大保留会话数
	maxSessions int

	mu sync.RWMutex
}

// ReplaySession 一次执行的录制会话
type ReplaySession struct {
	// ID 会话 ID
	ID string `json:"id"`

	// RunID 关联的执行 ID
	RunID string `json:"run_id"`

	// GraphID 关联的图 ID
	GraphID string `json:"graph_id,omitempty"`

	// GraphName 图名称
	GraphName string `json:"graph_name,omitempty"`

	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`

	// EndTime 结束时间
	EndTime time.Time `json:"end_time,omitempty"`

	// Status 会话状态: recording / completed / failed
	Status string `json:"status"`

	// Steps 执行步骤列表（按时间排序）
	Steps []ReplayStep `json:"steps"`

	// FinalState 最终状态
	FinalState map[string]any `json:"final_state,omitempty"`

	// Error 错误信息（如有）
	Error string `json:"error,omitempty"`

	// Metadata 额外元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ReplayStep 执行步骤
type ReplayStep struct {
	// Index 步骤序号（从 0 开始）
	Index int `json:"index"`

	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`

	// Type 步骤类型: node_start, node_end, edge, condition, error
	Type string `json:"type"`

	// NodeID 相关节点 ID
	NodeID string `json:"node_id,omitempty"`

	// NodeName 节点名称
	NodeName string `json:"node_name,omitempty"`

	// NodeType 节点类型
	NodeType string `json:"node_type,omitempty"`

	// StateSnapshot 该步骤的状态快照
	StateSnapshot map[string]any `json:"state_snapshot,omitempty"`

	// Input 节点输入
	Input map[string]any `json:"input,omitempty"`

	// Output 节点输出
	Output map[string]any `json:"output,omitempty"`

	// DurationMs 步骤耗时
	DurationMs int64 `json:"duration_ms,omitempty"`

	// Message 步骤描述
	Message string `json:"message,omitempty"`
}

// Breakpoint 断点定义
type Breakpoint struct {
	// NodeID 断点所在节点
	NodeID string `json:"node_id"`

	// Condition 条件表达式（可选，空表示无条件断点）
	Condition string `json:"condition,omitempty"`

	// Enabled 是否启用
	Enabled bool `json:"enabled"`
}

// ReplayState 回放状态
type ReplayState struct {
	// SessionID 会话 ID
	SessionID string `json:"session_id"`

	// CurrentStep 当前步骤索引
	CurrentStep int `json:"current_step"`

	// TotalSteps 总步骤数
	TotalSteps int `json:"total_steps"`

	// IsPlaying 是否正在播放
	IsPlaying bool `json:"is_playing"`

	// Step 当前步骤详情
	Step *ReplayStep `json:"step,omitempty"`

	// Breakpoints 断点列表
	Breakpoints []Breakpoint `json:"breakpoints,omitempty"`
}

// DiffResult 执行差异
type DiffResult struct {
	// SessionA 会话 A ID
	SessionA string `json:"session_a"`

	// SessionB 会话 B ID
	SessionB string `json:"session_b"`

	// StepDiffs 步骤差异
	StepDiffs []StepDiff `json:"step_diffs"`

	// Summary 差异摘要
	Summary string `json:"summary"`
}

// StepDiff 步骤差异
type StepDiff struct {
	// StepIndex 步骤索引
	StepIndex int `json:"step_index"`

	// Field 差异字段
	Field string `json:"field"`

	// ValueA A 的值
	ValueA any `json:"value_a"`

	// ValueB B 的值
	ValueB any `json:"value_b"`
}

// NewReplayManager 创建回放管理器
func NewReplayManager(maxSessions int) *ReplayManager {
	if maxSessions <= 0 {
		maxSessions = 100
	}
	return &ReplayManager{
		sessions:    make(map[string]*ReplaySession),
		maxSessions: maxSessions,
	}
}

// StartRecording 开始录制一次执行
func (m *ReplayManager) StartRecording(runID, graphID, graphName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清理过多的旧会话
	if len(m.sessions) >= m.maxSessions {
		m.evictOldest()
	}

	sessionID := "replay-" + idgen.ShortID()
	m.sessions[sessionID] = &ReplaySession{
		ID:        sessionID,
		RunID:     runID,
		GraphID:   graphID,
		GraphName: graphName,
		StartTime: time.Now(),
		Status:    "recording",
		Steps:     make([]ReplayStep, 0),
		Metadata:  make(map[string]any),
	}

	return sessionID
}

// AddStep 添加执行步骤
func (m *ReplayManager) AddStep(sessionID string, step ReplayStep) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok || session.Status != "recording" {
		return
	}

	step.Index = len(session.Steps)
	if step.Timestamp.IsZero() {
		step.Timestamp = time.Now()
	}

	session.Steps = append(session.Steps, step)
}

// EndRecording 结束录制
func (m *ReplayManager) EndRecording(sessionID string, finalState map[string]any, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	session.EndTime = time.Now()
	session.FinalState = finalState

	if err != nil {
		session.Status = "failed"
		session.Error = err.Error()
	} else {
		session.Status = "completed"
	}
}

// GetSession 获取会话详情
func (m *ReplayManager) GetSession(sessionID string) (*ReplaySession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	return session, nil
}

// ListSessions 列出所有会话
func (m *ReplayManager) ListSessions() []*ReplaySession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ReplaySession, 0, len(m.sessions))
	for _, s := range m.sessions {
		// 返回简要信息（不包含步骤详情）
		summary := &ReplaySession{
			ID:        s.ID,
			RunID:     s.RunID,
			GraphID:   s.GraphID,
			GraphName: s.GraphName,
			StartTime: s.StartTime,
			EndTime:   s.EndTime,
			Status:    s.Status,
			Error:     s.Error,
		}
		result = append(result, summary)
	}

	// 按时间倒序
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.After(result[j].StartTime)
	})

	return result
}

// GetStep 获取指定步骤
func (m *ReplayManager) GetStep(sessionID string, stepIndex int) (*ReplayStep, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if stepIndex < 0 || stepIndex >= len(session.Steps) {
		return nil, fmt.Errorf("步骤索引越界: %d (共 %d 步)", stepIndex, len(session.Steps))
	}

	return &session.Steps[stepIndex], nil
}

// GetReplayState 获取回放状态（在指定步骤）
func (m *ReplayManager) GetReplayState(sessionID string, stepIndex int) (*ReplayState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if stepIndex < 0 || stepIndex >= len(session.Steps) {
		return nil, fmt.Errorf("步骤索引越界: %d (共 %d 步)", stepIndex, len(session.Steps))
	}

	return &ReplayState{
		SessionID:   sessionID,
		CurrentStep: stepIndex,
		TotalSteps:  len(session.Steps),
		Step:        &session.Steps[stepIndex],
	}, nil
}

// CompareExecutions 比较两次执行的差异
func (m *ReplayManager) CompareExecutions(sessionA, sessionB string) (*DiffResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sA, ok := m.sessions[sessionA]
	if !ok {
		return nil, fmt.Errorf("会话 A 不存在: %s", sessionA)
	}
	sB, ok := m.sessions[sessionB]
	if !ok {
		return nil, fmt.Errorf("会话 B 不存在: %s", sessionB)
	}

	result := &DiffResult{
		SessionA:  sessionA,
		SessionB:  sessionB,
		StepDiffs: make([]StepDiff, 0),
	}

	// 比较步骤数
	maxSteps := len(sA.Steps)
	if len(sB.Steps) > maxSteps {
		maxSteps = len(sB.Steps)
	}

	diffCount := 0
	for i := 0; i < maxSteps; i++ {
		if i >= len(sA.Steps) {
			result.StepDiffs = append(result.StepDiffs, StepDiff{
				StepIndex: i,
				Field:     "存在性",
				ValueA:    nil,
				ValueB:    sB.Steps[i].Message,
			})
			diffCount++
			continue
		}
		if i >= len(sB.Steps) {
			result.StepDiffs = append(result.StepDiffs, StepDiff{
				StepIndex: i,
				Field:     "存在性",
				ValueA:    sA.Steps[i].Message,
				ValueB:    nil,
			})
			diffCount++
			continue
		}

		stepA := sA.Steps[i]
		stepB := sB.Steps[i]

		// 比较节点
		if stepA.NodeID != stepB.NodeID {
			result.StepDiffs = append(result.StepDiffs, StepDiff{
				StepIndex: i,
				Field:     "node_id",
				ValueA:    stepA.NodeID,
				ValueB:    stepB.NodeID,
			})
			diffCount++
		}

		// 比较耗时差异（超过 50% 视为显著差异）
		if stepA.DurationMs > 0 && stepB.DurationMs > 0 {
			ratio := float64(stepA.DurationMs) / float64(stepB.DurationMs)
			if ratio > 1.5 || ratio < 0.67 {
				result.StepDiffs = append(result.StepDiffs, StepDiff{
					StepIndex: i,
					Field:     "duration_ms",
					ValueA:    stepA.DurationMs,
					ValueB:    stepB.DurationMs,
				})
				diffCount++
			}
		}
	}

	result.Summary = fmt.Sprintf("会话 A: %d 步, 会话 B: %d 步, 发现 %d 处差异",
		len(sA.Steps), len(sB.Steps), diffCount)

	return result, nil
}

// DeleteSession 删除会话
func (m *ReplayManager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	delete(m.sessions, sessionID)
	return nil
}

// evictOldest 淘汰最旧的会话
func (m *ReplayManager) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, session := range m.sessions {
		if oldestID == "" || session.StartTime.Before(oldestTime) {
			oldestID = id
			oldestTime = session.StartTime
		}
	}

	if oldestID != "" {
		delete(m.sessions, oldestID)
	}
}

// ============== HTTP 处理器 ==============

// replayHandler 回放 API 处理器
type replayHandler struct {
	manager *ReplayManager
}

// newReplayHandler 创建回放处理器
func newReplayHandler(manager *ReplayManager) *replayHandler {
	return &replayHandler{manager: manager}
}

// handleSessions 会话列表
// GET /api/replay/sessions
func (h *replayHandler) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := h.manager.ListSessions()
	writeSuccess(w, map[string]any{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// handleSession 单个会话操作
// GET    /api/replay/sessions/{id}           - 获取会话详情
// DELETE /api/replay/sessions/{id}           - 删除会话
// GET    /api/replay/sessions/{id}/steps/{n} - 获取步骤
// GET    /api/replay/sessions/{id}/state/{n} - 获取回放状态
func (h *replayHandler) handleSession(w http.ResponseWriter, r *http.Request) {
	// 解析路径: /api/replay/sessions/{id}[/steps/{n}|/state/{n}|/compare/{id2}]
	path := r.URL.Path
	const prefix = "/api/replay/sessions/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusBadRequest, "无效路径")
		return
	}

	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSuffix(remainder, "/")
	parts := strings.Split(remainder, "/")

	sessionID := parts[0]

	// 子路由
	if len(parts) >= 2 {
		switch parts[1] {
		case "steps":
			if len(parts) >= 3 {
				h.handleGetStep(w, r, sessionID, parts[2])
				return
			}
		case "state":
			if len(parts) >= 3 {
				h.handleGetReplayState(w, r, sessionID, parts[2])
				return
			}
		case "compare":
			if len(parts) >= 3 {
				h.handleCompare(w, r, sessionID, parts[2])
				return
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		session, err := h.manager.GetSession(sessionID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeSuccess(w, session)

	case http.MethodDelete:
		if err := h.manager.DeleteSession(sessionID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeSuccess(w, map[string]any{"deleted": true})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGetStep 获取指定步骤
func (h *replayHandler) handleGetStep(w http.ResponseWriter, _ *http.Request, sessionID, stepStr string) {
	stepIndex, err := strconv.Atoi(stepStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的步骤索引")
		return
	}

	step, err := h.manager.GetStep(sessionID, stepIndex)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, step)
}

// handleGetReplayState 获取回放状态
func (h *replayHandler) handleGetReplayState(w http.ResponseWriter, _ *http.Request, sessionID, stepStr string) {
	stepIndex, err := strconv.Atoi(stepStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的步骤索引")
		return
	}

	state, err := h.manager.GetReplayState(sessionID, stepIndex)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, state)
}

// handleCompare 比较两次执行
func (h *replayHandler) handleCompare(w http.ResponseWriter, _ *http.Request, sessionA, sessionB string) {
	diff, err := h.manager.CompareExecutions(sessionA, sessionB)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, diff)
}

// writeJSON 写入 JSON 响应（replay 包内使用，避免与 handler.go 冲突）
func writeReplayJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
