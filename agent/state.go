package agent

import (
	"context"
	"sync"
	"time"
)

// StateManager 分层状态管理器接口
// 提供四层状态管理: Turn -> Session -> Agent -> Global
type StateManager interface {
	// Turn 获取单轮对话状态（生命周期：单次 Run 调用）
	Turn() TurnState

	// Session 获取会话级状态（生命周期：一次会话/对话）
	Session() SessionState

	// Agent 获取 Agent 持久状态（生命周期：Agent 实例）
	Agent() AgentState

	// Global 获取全局共享状态（生命周期：应用程序）
	Global() GlobalState

	// NewTurn 创建新的轮次，重置 TurnState
	NewTurn() TurnState

	// Snapshot 创建当前状态的快照
	Snapshot() StateSnapshot

	// Restore 从快照恢复状态
	Restore(snapshot StateSnapshot) error
}

// TurnState 单轮对话状态
// 生命周期：单次 Run 调用
type TurnState interface {
	// Get 获取值
	Get(key string) (any, bool)

	// Set 设置值
	Set(key string, value any)

	// Delete 删除值
	Delete(key string)

	// Clear 清空所有值
	Clear()

	// All 获取所有键值对
	All() map[string]any

	// Iteration 获取当前迭代次数（ReAct 循环）
	Iteration() int

	// SetIteration 设置迭代次数
	SetIteration(n int)

	// Messages 获取本轮消息
	Messages() []Message

	// AddMessage 添加消息
	AddMessage(msg Message)
}

// SessionState 会话级状态
// 生命周期：一次会话/对话
type SessionState interface {
	// ID 获取会话 ID
	ID() string

	// Get 获取值
	Get(key string) (any, bool)

	// Set 设置值
	Set(key string, value any)

	// Delete 删除值
	Delete(key string)

	// All 获取所有键值对
	All() map[string]any

	// CreatedAt 获取创建时间
	CreatedAt() time.Time

	// UpdatedAt 获取最后更新时间
	UpdatedAt() time.Time

	// TurnCount 获取轮次计数
	TurnCount() int

	// IncrementTurnCount 增加轮次计数
	IncrementTurnCount()
}

// AgentState Agent 持久状态
// 生命周期：Agent 实例
type AgentState interface {
	// Get 获取值
	Get(key string) (any, bool)

	// Set 设置值
	Set(key string, value any)

	// Delete 删除值
	Delete(key string)

	// All 获取所有键值对
	All() map[string]any

	// Stats 获取 Agent 统计信息
	Stats() AgentStats

	// UpdateStats 更新统计信息
	UpdateStats(fn func(*AgentStats))
}

// GlobalState 全局共享状态
// 生命周期：应用程序
// 多个 Agent 之间共享
type GlobalState interface {
	// Get 获取值
	Get(key string) (any, bool)

	// Set 设置值
	Set(key string, value any)

	// Delete 删除值
	Delete(key string)

	// All 获取所有键值对
	All() map[string]any

	// RegisterAgent 注册 Agent
	RegisterAgent(agentID string, agent Agent)

	// GetAgent 获取已注册的 Agent
	GetAgent(agentID string) (Agent, bool)

	// ListAgents 列出所有已注册的 Agent
	ListAgents() []string
}

// Message 消息结构（用于状态存储）
type Message struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []ToolCall     `json:"tool_calls,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ToolCall 工具调用记录
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Result    string         `json:"result,omitempty"`
}

// AgentStats Agent 统计信息
type AgentStats struct {
	TotalRuns        int64         `json:"total_runs"`
	SuccessfulRuns   int64         `json:"successful_runs"`
	FailedRuns       int64         `json:"failed_runs"`
	TotalTokens      int64         `json:"total_tokens"`
	PromptTokens     int64         `json:"prompt_tokens"`
	CompletionTokens int64         `json:"completion_tokens"`
	TotalToolCalls   int64         `json:"total_tool_calls"`
	TotalDuration    time.Duration `json:"total_duration"`
	LastRunAt        time.Time     `json:"last_run_at"`
}

// StateSnapshot 状态快照
type StateSnapshot struct {
	Timestamp    time.Time      `json:"timestamp"`
	SessionID    string         `json:"session_id"`
	TurnData     map[string]any `json:"turn_data"`
	SessionData  map[string]any `json:"session_data"`
	AgentData    map[string]any `json:"agent_data"`
	TurnCount    int            `json:"turn_count"`
	Iteration    int            `json:"iteration"`
	Messages     []Message      `json:"messages"`
}

// ============== 默认实现 ==============

// DefaultStateManager 默认状态管理器实现
type DefaultStateManager struct {
	turn    *defaultTurnState
	session *defaultSessionState
	agent   *defaultAgentState
	global  GlobalState
	mu      sync.RWMutex
}

// NewStateManager 创建默认状态管理器
func NewStateManager(sessionID string, global GlobalState) *DefaultStateManager {
	if global == nil {
		global = NewGlobalState()
	}
	return &DefaultStateManager{
		turn:    newDefaultTurnState(),
		session: newDefaultSessionState(sessionID),
		agent:   newDefaultAgentState(),
		global:  global,
	}
}

func (m *DefaultStateManager) Turn() TurnState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.turn
}

func (m *DefaultStateManager) Session() SessionState {
	return m.session
}

func (m *DefaultStateManager) Agent() AgentState {
	return m.agent
}

func (m *DefaultStateManager) Global() GlobalState {
	return m.global
}

func (m *DefaultStateManager) NewTurn() TurnState {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session.IncrementTurnCount()
	m.turn = newDefaultTurnState()
	return m.turn
}

func (m *DefaultStateManager) Snapshot() StateSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return StateSnapshot{
		Timestamp:   time.Now(),
		SessionID:   m.session.ID(),
		TurnData:    copyMap(m.turn.All()),
		SessionData: copyMap(m.session.All()),
		AgentData:   copyMap(m.agent.All()),
		TurnCount:   m.session.TurnCount(),
		Iteration:   m.turn.Iteration(),
		Messages:    append([]Message(nil), m.turn.Messages()...),
	}
}

func (m *DefaultStateManager) Restore(snapshot StateSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 恢复 Turn 状态
	m.turn = newDefaultTurnState()
	for k, v := range snapshot.TurnData {
		m.turn.Set(k, v)
	}
	m.turn.SetIteration(snapshot.Iteration)
	for _, msg := range snapshot.Messages {
		m.turn.AddMessage(msg)
	}

	// 恢复 Session 状态
	for k, v := range snapshot.SessionData {
		m.session.Set(k, v)
	}
	m.session.setTurnCount(snapshot.TurnCount)

	// 恢复 Agent 状态
	for k, v := range snapshot.AgentData {
		m.agent.Set(k, v)
	}

	return nil
}

// ============== TurnState 默认实现 ==============

type defaultTurnState struct {
	data      map[string]any
	iteration int
	messages  []Message
	mu        sync.RWMutex
}

func newDefaultTurnState() *defaultTurnState {
	return &defaultTurnState{
		data:     make(map[string]any),
		messages: make([]Message, 0),
	}
}

func (s *defaultTurnState) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *defaultTurnState) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *defaultTurnState) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *defaultTurnState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]any)
	s.messages = make([]Message, 0)
	s.iteration = 0
}

func (s *defaultTurnState) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyMap(s.data)
}

func (s *defaultTurnState) Iteration() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.iteration
}

func (s *defaultTurnState) SetIteration(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.iteration = n
}

func (s *defaultTurnState) Messages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Message(nil), s.messages...)
}

func (s *defaultTurnState) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

// ============== SessionState 默认实现 ==============

type defaultSessionState struct {
	id        string
	data      map[string]any
	createdAt time.Time
	updatedAt time.Time
	turnCount int
	mu        sync.RWMutex
}

func newDefaultSessionState(id string) *defaultSessionState {
	now := time.Now()
	return &defaultSessionState{
		id:        id,
		data:      make(map[string]any),
		createdAt: now,
		updatedAt: now,
	}
}

func (s *defaultSessionState) ID() string {
	return s.id
}

func (s *defaultSessionState) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *defaultSessionState) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	s.updatedAt = time.Now()
}

func (s *defaultSessionState) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	s.updatedAt = time.Now()
}

func (s *defaultSessionState) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyMap(s.data)
}

func (s *defaultSessionState) CreatedAt() time.Time {
	return s.createdAt
}

func (s *defaultSessionState) UpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

func (s *defaultSessionState) TurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnCount
}

func (s *defaultSessionState) IncrementTurnCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnCount++
	s.updatedAt = time.Now()
}

func (s *defaultSessionState) setTurnCount(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnCount = n
}

// ============== AgentState 默认实现 ==============

type defaultAgentState struct {
	data  map[string]any
	stats AgentStats
	mu    sync.RWMutex
}

func newDefaultAgentState() *defaultAgentState {
	return &defaultAgentState{
		data: make(map[string]any),
	}
}

func (s *defaultAgentState) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *defaultAgentState) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *defaultAgentState) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *defaultAgentState) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyMap(s.data)
}

func (s *defaultAgentState) Stats() AgentStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *defaultAgentState) UpdateStats(fn func(*AgentStats)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.stats)
}

// ============== GlobalState 默认实现 ==============

type defaultGlobalState struct {
	data   map[string]any
	agents map[string]Agent
	mu     sync.RWMutex
}

// NewGlobalState 创建全局状态
func NewGlobalState() *defaultGlobalState {
	return &defaultGlobalState{
		data:   make(map[string]any),
		agents: make(map[string]Agent),
	}
}

func (s *defaultGlobalState) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *defaultGlobalState) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *defaultGlobalState) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *defaultGlobalState) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyMap(s.data)
}

func (s *defaultGlobalState) RegisterAgent(agentID string, agent Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agentID] = agent
}

func (s *defaultGlobalState) GetAgent(agentID string) (Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[agentID]
	return a, ok
}

func (s *defaultGlobalState) ListAgents() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// ============== Context 辅助函数 ==============

type stateManagerKey struct{}

// ContextWithStateManager 将 StateManager 添加到 context
func ContextWithStateManager(ctx context.Context, sm StateManager) context.Context {
	return context.WithValue(ctx, stateManagerKey{}, sm)
}

// StateManagerFromContext 从 context 中获取 StateManager
func StateManagerFromContext(ctx context.Context) StateManager {
	if sm, ok := ctx.Value(stateManagerKey{}).(StateManager); ok {
		return sm
	}
	return nil
}

// ============== 工具函数 ==============

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
