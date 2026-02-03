// Package record 提供高级测试录制和回放功能
package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/rag"
)

// ============== Tool Recording ==============

// ToolInteraction Tool 交互记录
type ToolInteraction struct {
	ID          string         `json:"id"`
	ToolName    string         `json:"tool_name"`
	Args        map[string]any `json:"args"`
	Result      string         `json:"result"`
	Error       string         `json:"error,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Timestamp   time.Time      `json:"timestamp"`
	RequestHash string         `json:"request_hash"`
}

// ToolCassette Tool 录制会话
type ToolCassette struct {
	Name         string            `json:"name"`
	Interactions []ToolInteraction `json:"interactions"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// NewToolCassette 创建 Tool 录制会话
func NewToolCassette(name string) *ToolCassette {
	now := time.Now()
	return &ToolCassette{
		Name:         name,
		Interactions: make([]ToolInteraction, 0),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddInteraction 添加交互
func (c *ToolCassette) AddInteraction(interaction ToolInteraction) {
	c.Interactions = append(c.Interactions, interaction)
	c.UpdatedAt = time.Now()
}

// FindByHash 查找交互
func (c *ToolCassette) FindByHash(hash string) *ToolInteraction {
	for i := range c.Interactions {
		if c.Interactions[i].RequestHash == hash {
			return &c.Interactions[i]
		}
	}
	return nil
}

// Save 保存到文件
func (c *ToolCassette) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cassette: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadToolCassette 从文件加载
func LoadToolCassette(path string) (*ToolCassette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cassette file: %w", err)
	}

	var cassette ToolCassette
	if err := json.Unmarshal(data, &cassette); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cassette: %w", err)
	}

	return &cassette, nil
}

// ============== RAG Recording ==============

// RAGInteraction RAG 交互记录
type RAGInteraction struct {
	ID          string         `json:"id"`
	Operation   string         `json:"operation"` // retrieve, rerank, synthesize
	Query       string         `json:"query"`
	Documents   []rag.Document `json:"documents,omitempty"`
	Result      string         `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Timestamp   time.Time      `json:"timestamp"`
	RequestHash string         `json:"request_hash"`
}

// RAGCassette RAG 录制会话
type RAGCassette struct {
	Name         string           `json:"name"`
	Interactions []RAGInteraction `json:"interactions"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// NewRAGCassette 创建 RAG 录制会话
func NewRAGCassette(name string) *RAGCassette {
	now := time.Now()
	return &RAGCassette{
		Name:         name,
		Interactions: make([]RAGInteraction, 0),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddInteraction 添加交互
func (c *RAGCassette) AddInteraction(interaction RAGInteraction) {
	c.Interactions = append(c.Interactions, interaction)
	c.UpdatedAt = time.Now()
}

// FindByHash 查找交互
func (c *RAGCassette) FindByHash(hash string) *RAGInteraction {
	for i := range c.Interactions {
		if c.Interactions[i].RequestHash == hash {
			return &c.Interactions[i]
		}
	}
	return nil
}

// Save 保存到文件
func (c *RAGCassette) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cassette: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadRAGCassette 从文件加载
func LoadRAGCassette(path string) (*RAGCassette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cassette file: %w", err)
	}

	var cassette RAGCassette
	if err := json.Unmarshal(data, &cassette); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cassette: %w", err)
	}

	return &cassette, nil
}

// ============== Fixture Management ==============

// Fixture 测试固件
type Fixture struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Data        map[string]any `json:"data"`
	CreatedAt   time.Time      `json:"created_at"`
}

// FixtureManager 固件管理器
type FixtureManager struct {
	basePath string
	fixtures map[string]*Fixture
	mu       sync.RWMutex
}

// NewFixtureManager 创建固件管理器
func NewFixtureManager(basePath string) *FixtureManager {
	return &FixtureManager{
		basePath: basePath,
		fixtures: make(map[string]*Fixture),
	}
}

// Load 加载固件
//
// 优化版本：使用双重检查锁保证线程安全和性能
func (m *FixtureManager) Load(name string) (*Fixture, error) {
	// 第一次检查（快速路径）
	m.mu.RLock()
	if fixture, ok := m.fixtures[name]; ok {
		m.mu.RUnlock()
		return fixture, nil
	}
	m.mu.RUnlock()

	// 获取写锁
	m.mu.Lock()
	defer m.mu.Unlock()

	// 第二次检查（避免重复加载）
	if fixture, ok := m.fixtures[name]; ok {
		return fixture, nil
	}

	// 从文件加载
	path := filepath.Join(m.basePath, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture %s: %w", name, err)
	}

	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fixture %s: %w", name, err)
	}

	m.fixtures[name] = &fixture
	return &fixture, nil
}

// Save 保存固件
func (m *FixtureManager) Save(fixture *Fixture) error {
	if err := os.MkdirAll(m.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	path := filepath.Join(m.basePath, fixture.Name+".json")
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal fixture: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write fixture: %w", err)
	}

	m.mu.Lock()
	m.fixtures[fixture.Name] = fixture
	m.mu.Unlock()

	return nil
}

// Get 获取固件数据
func (m *FixtureManager) Get(name, key string) (any, error) {
	fixture, err := m.Load(name)
	if err != nil {
		return nil, err
	}

	value, ok := fixture.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in fixture %s", key, name)
	}

	return value, nil
}

// Set 设置固件数据
func (m *FixtureManager) Set(name, key string, value any) error {
	fixture, err := m.Load(name)
	if err != nil {
		// 创建新固件
		fixture = &Fixture{
			Name:      name,
			Data:      make(map[string]any),
			CreatedAt: time.Now(),
		}
	}

	fixture.Data[key] = value
	return m.Save(fixture)
}

// ============== Test Scenario ==============

// TestScenario 测试场景
type TestScenario struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Steps       []TestStep          `json:"steps"`
	Fixtures    []string            `json:"fixtures"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
}

// TestStep 测试步骤
type TestStep struct {
	Name        string         `json:"name"`
	Action      string         `json:"action"`      // call_llm, call_tool, assert, etc
	Input       map[string]any `json:"input,omitempty"`
	Expected    map[string]any `json:"expected,omitempty"`
	Cassette    string         `json:"cassette,omitempty"`
	Description string         `json:"description,omitempty"`
}

// ScenarioManager 场景管理器
type ScenarioManager struct {
	basePath  string
	scenarios map[string]*TestScenario
	mu        sync.RWMutex
}

// NewScenarioManager 创建场景管理器
func NewScenarioManager(basePath string) *ScenarioManager {
	return &ScenarioManager{
		basePath:  basePath,
		scenarios: make(map[string]*TestScenario),
	}
}

// Load 加载场景
func (m *ScenarioManager) Load(name string) (*TestScenario, error) {
	m.mu.RLock()
	if scenario, ok := m.scenarios[name]; ok {
		m.mu.RUnlock()
		return scenario, nil
	}
	m.mu.RUnlock()

	path := filepath.Join(m.basePath, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario: %w", err)
	}

	var scenario TestScenario
	if err := json.Unmarshal(data, &scenario); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scenario: %w", err)
	}

	m.mu.Lock()
	m.scenarios[name] = &scenario
	m.mu.Unlock()

	return &scenario, nil
}

// Save 保存场景
func (m *ScenarioManager) Save(scenario *TestScenario) error {
	if err := os.MkdirAll(m.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	path := filepath.Join(m.basePath, scenario.Name+".json")
	data, err := json.MarshalIndent(scenario, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scenario: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write scenario: %w", err)
	}

	m.mu.Lock()
	m.scenarios[scenario.Name] = scenario
	m.mu.Unlock()

	return nil
}

// ============== Session Recording ==============

// SessionRecorder 会话录制器
//
// 录制完整的测试会话，包括所有类型的交互
type SessionRecorder struct {
	name            string
	llmCassette     *Cassette
	toolCassette    *ToolCassette
	ragCassette     *RAGCassette
	events          []SessionEvent
	startTime       time.Time
	mu              sync.Mutex
}

// SessionEvent 会话事件
type SessionEvent struct {
	Type      string         `json:"type"`      // llm, tool, rag, custom
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

// NewSessionRecorder 创建会话录制器
func NewSessionRecorder(name string) *SessionRecorder {
	return &SessionRecorder{
		name:         name,
		llmCassette:  NewCassette(name + "_llm"),
		toolCassette: NewToolCassette(name + "_tool"),
		ragCassette:  NewRAGCassette(name + "_rag"),
		events:       make([]SessionEvent, 0),
		startTime:    time.Now(),
	}
}

// RecordLLM 录制 LLM 交互
func (r *SessionRecorder) RecordLLM(interaction Interaction) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.llmCassette.AddInteraction(interaction)
	r.events = append(r.events, SessionEvent{
		Type:      "llm",
		Timestamp: interaction.Timestamp,
		Data: map[string]any{
			"id":    interaction.ID,
			"model": interaction.Request.Model,
		},
	})
}

// RecordTool 录制 Tool 交互
func (r *SessionRecorder) RecordTool(interaction ToolInteraction) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.toolCassette.AddInteraction(interaction)
	r.events = append(r.events, SessionEvent{
		Type:      "tool",
		Timestamp: interaction.Timestamp,
		Data: map[string]any{
			"id":   interaction.ID,
			"tool": interaction.ToolName,
		},
	})
}

// RecordRAG 录制 RAG 交互
func (r *SessionRecorder) RecordRAG(interaction RAGInteraction) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ragCassette.AddInteraction(interaction)
	r.events = append(r.events, SessionEvent{
		Type:      "rag",
		Timestamp: interaction.Timestamp,
		Data: map[string]any{
			"id":        interaction.ID,
			"operation": interaction.Operation,
		},
	})
}

// RecordEvent 录制自定义事件
func (r *SessionRecorder) RecordEvent(eventType string, data map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, SessionEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// SaveAll 保存所有录制
func (r *SessionRecorder) SaveAll(basePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 保存 LLM 录制
	if len(r.llmCassette.Interactions) > 0 {
		llmPath := filepath.Join(basePath, r.name+"_llm.json")
		if err := r.llmCassette.Save(llmPath); err != nil {
			return fmt.Errorf("failed to save LLM cassette: %w", err)
		}
	}

	// 保存 Tool 录制
	if len(r.toolCassette.Interactions) > 0 {
		toolPath := filepath.Join(basePath, r.name+"_tool.json")
		if err := r.toolCassette.Save(toolPath); err != nil {
			return fmt.Errorf("failed to save Tool cassette: %w", err)
		}
	}

	// 保存 RAG 录制
	if len(r.ragCassette.Interactions) > 0 {
		ragPath := filepath.Join(basePath, r.name+"_rag.json")
		if err := r.ragCassette.Save(ragPath); err != nil {
			return fmt.Errorf("failed to save RAG cassette: %w", err)
		}
	}

	// 保存事件时间线
	if len(r.events) > 0 {
		eventsPath := filepath.Join(basePath, r.name+"_events.json")
		data, err := json.MarshalIndent(r.events, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal events: %w", err)
		}
		if err := os.WriteFile(eventsPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write events: %w", err)
		}
	}

	return nil
}

// ============== Assertion Helpers ==============

// AssertionHelper 断言辅助工具
type AssertionHelper struct {
	errors []string
	mu     sync.Mutex
}

// NewAssertionHelper 创建断言辅助工具
func NewAssertionHelper() *AssertionHelper {
	return &AssertionHelper{
		errors: make([]string, 0),
	}
}

// AssertEqual 断言相等
func (h *AssertionHelper) AssertEqual(expected, actual any, message string) bool {
	if expected != actual {
		h.mu.Lock()
		h.errors = append(h.errors, fmt.Sprintf("%s: expected %v, got %v", message, expected, actual))
		h.mu.Unlock()
		return false
	}
	return true
}

// AssertContains 断言包含
func (h *AssertionHelper) AssertContains(haystack, needle string, message string) bool {
	if !contains(haystack, needle) {
		h.mu.Lock()
		h.errors = append(h.errors, fmt.Sprintf("%s: %q not found in %q", message, needle, haystack))
		h.mu.Unlock()
		return false
	}
	return true
}

// AssertNotEmpty 断言非空
func (h *AssertionHelper) AssertNotEmpty(value any, message string) bool {
	if isEmpty(value) {
		h.mu.Lock()
		h.errors = append(h.errors, fmt.Sprintf("%s: value is empty", message))
		h.mu.Unlock()
		return false
	}
	return true
}

// HasErrors 是否有错误
func (h *AssertionHelper) HasErrors() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.errors) > 0
}

// Errors 返回所有错误
func (h *AssertionHelper) Errors() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.errors...)
}

// ============== Test Generator ==============

// TestGenerator 测试生成器
//
// 从录制生成测试代码
type TestGenerator struct {
	packageName string
}

// NewTestGenerator 创建测试生成器
func NewTestGenerator(packageName string) *TestGenerator {
	return &TestGenerator{
		packageName: packageName,
	}
}

// GenerateFromCassette 从录制生成测试代码
func (g *TestGenerator) GenerateFromCassette(cassette *Cassette) string {
	code := fmt.Sprintf(`package %s

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/testing/record"
	"github.com/everyday-items/ai-core/llm"
)

func Test%s(t *testing.T) {
	// 加载录制
	cassette, err := record.LoadCassette("testdata/%s.json")
	if err != nil {
		t.Fatalf("Failed to load cassette: %%v", err)
	}

	// 创建回放器
	replayer := record.NewReplayer(cassette, record.WithReplayMode(record.ReplayModeStrict))

	// 执行测试
	ctx := context.Background()

`, g.packageName, cassette.Name, cassette.Name)

	for i, interaction := range cassette.Interactions {
		code += fmt.Sprintf(`
	// Test interaction %d
	resp%d, err := replayer.Complete(ctx, llm.CompletionRequest{
		Model: "%s",
		Messages: []llm.Message{
			// ... 配置消息
		},
	})
	if err != nil {
		t.Errorf("Interaction %d failed: %%v", err)
	}
	if resp%d == nil {
		t.Error("Expected response, got nil")
	}

`, i+1, i+1, interaction.Request.Model, i+1, i+1)
	}

	code += "}\n"

	return code
}

// ============== 辅助函数 ==============

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}
