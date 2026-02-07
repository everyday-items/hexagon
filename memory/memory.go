// Package memory 提供 Hexagon 框架的高级记忆系统
//
// 在 ai-core 的基础 Memory 接口上，扩展多种记忆实现：
//   - WindowMemory: 滑动窗口记忆，只保留最近 N 轮对话
//   - SummaryMemory: 摘要记忆，定期将旧记忆压缩为摘要
//   - VectorMemory: 向量记忆，支持语义搜索
//   - EntityMemory: 实体记忆，提取和记录实体信息
//
// 使用示例：
//
//	mem := NewWindowMemory(10) // 保留最近 10 条
//	mem := NewSummaryMemory(llmProvider, "model", 20) // 超过 20 条时摘要
//	mem := NewVectorMemory(embedder, vectorStore, 100)
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/ai-core/llm"
	coremem "github.com/everyday-items/ai-core/memory"
)

// ============== WindowMemory ==============

// WindowMemory 滑动窗口记忆
// 只保留最近 N 条对话记录，超出时自动移除最旧的
type WindowMemory struct {
	mu       sync.RWMutex
	entries  []coremem.Entry
	windowSize int
}

// NewWindowMemory 创建滑动窗口记忆
// windowSize: 窗口大小（保留的最大条目数）
func NewWindowMemory(windowSize int) *WindowMemory {
	if windowSize <= 0 {
		windowSize = 10
	}
	return &WindowMemory{
		entries:    make([]coremem.Entry, 0, windowSize),
		windowSize: windowSize,
	}
}

func (m *WindowMemory) Save(_ context.Context, entry coremem.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	m.entries = append(m.entries, entry)

	// 滑动窗口：超出时移除最旧的
	if len(m.entries) > m.windowSize {
		m.entries = m.entries[len(m.entries)-m.windowSize:]
	}

	return nil
}

func (m *WindowMemory) SaveBatch(ctx context.Context, entries []coremem.Entry) error {
	for _, e := range entries {
		if err := m.Save(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (m *WindowMemory) Get(_ context.Context, id string) (*coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.entries {
		if m.entries[i].ID == id {
			entry := m.entries[i]
			return &entry, nil
		}
	}
	return nil, nil
}

func (m *WindowMemory) Search(_ context.Context, query coremem.SearchQuery) ([]coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]coremem.Entry, len(m.entries))
	copy(result, m.entries)

	if query.Limit > 0 && len(result) > query.Limit {
		result = result[len(result)-query.Limit:]
	}

	return result, nil
}

func (m *WindowMemory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.entries {
		if e.ID == id {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *WindowMemory) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = m.entries[:0]
	return nil
}

func (m *WindowMemory) Stats() coremem.MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return coremem.MemoryStats{EntryCount: len(m.entries)}
}

var _ coremem.Memory = (*WindowMemory)(nil)

// ============== SummaryMemory ==============

// SummaryMemory 摘要记忆
// 当记忆条目超过阈值时，使用 LLM 将旧记忆压缩为摘要
type SummaryMemory struct {
	mu            sync.RWMutex
	entries       []coremem.Entry
	summary       string // 当前摘要
	maxEntries    int    // 触发摘要的最大条目数
	provider      llm.Provider
	model         string
	summarizing   bool // 防止并发摘要
}

// NewSummaryMemory 创建摘要记忆
// maxEntries: 超过此数量时触发摘要压缩
func NewSummaryMemory(provider llm.Provider, model string, maxEntries int) *SummaryMemory {
	if maxEntries <= 0 {
		maxEntries = 20
	}
	return &SummaryMemory{
		entries:    make([]coremem.Entry, 0),
		maxEntries: maxEntries,
		provider:   provider,
		model:      model,
	}
}

func (m *SummaryMemory) Save(ctx context.Context, entry coremem.Entry) error {
	m.mu.Lock()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	m.entries = append(m.entries, entry)

	needSummarize := len(m.entries) > m.maxEntries && !m.summarizing
	if needSummarize {
		m.summarizing = true
	}
	m.mu.Unlock()

	// 触发摘要（在锁外执行避免持锁调 LLM）
	if needSummarize {
		m.summarize(ctx)
	}

	return nil
}

// summarize 将旧记忆压缩为摘要
func (m *SummaryMemory) summarize(ctx context.Context) {
	m.mu.RLock()
	// 取前半部分做摘要
	half := len(m.entries) / 2
	toSummarize := make([]coremem.Entry, half)
	copy(toSummarize, m.entries[:half])
	currentSummary := m.summary
	m.mu.RUnlock()

	// 构建摘要 prompt
	var conversation strings.Builder
	if currentSummary != "" {
		conversation.WriteString("之前的摘要:\n" + currentSummary + "\n\n")
	}
	conversation.WriteString("新的对话:\n")
	for _, e := range toSummarize {
		conversation.WriteString(fmt.Sprintf("%s: %s\n", e.Role, e.Content))
	}

	req := llm.CompletionRequest{
		Model: m.model,
		Messages: []llm.Message{
			{Role: "system", Content: "请将以下对话历史压缩为简洁的摘要，保留关键信息。用中文回复。"},
			{Role: "user", Content: conversation.String()},
		},
		MaxTokens: 500,
	}

	resp, err := m.provider.Complete(ctx, req)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.summarizing = false

	if err != nil {
		return // 摘要失败，保留原数据
	}

	m.summary = resp.Content
	// 移除已摘要的条目
	m.entries = m.entries[half:]
}

func (m *SummaryMemory) SaveBatch(ctx context.Context, entries []coremem.Entry) error {
	for _, e := range entries {
		if err := m.Save(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (m *SummaryMemory) Get(_ context.Context, id string) (*coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.entries {
		if m.entries[i].ID == id {
			entry := m.entries[i]
			return &entry, nil
		}
	}
	return nil, nil
}

// Search 搜索时会将摘要作为第一条结果返回
func (m *SummaryMemory) Search(_ context.Context, query coremem.SearchQuery) ([]coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []coremem.Entry

	// 如果有摘要，作为系统消息插入
	if m.summary != "" {
		result = append(result, coremem.Entry{
			ID:        "summary",
			Role:      "system",
			Content:   "对话历史摘要: " + m.summary,
			CreatedAt: time.Now(),
		})
	}

	result = append(result, m.entries...)

	if query.Limit > 0 && len(result) > query.Limit {
		result = result[len(result)-query.Limit:]
	}

	return result, nil
}

func (m *SummaryMemory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.entries {
		if e.ID == id {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *SummaryMemory) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = m.entries[:0]
	m.summary = ""
	return nil
}

func (m *SummaryMemory) Stats() coremem.MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return coremem.MemoryStats{EntryCount: len(m.entries)}
}

// Summary 返回当前摘要内容
func (m *SummaryMemory) Summary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.summary
}

var _ coremem.Memory = (*SummaryMemory)(nil)

// ============== EntityMemory ==============

// EntityMemory 实体记忆
// 自动提取对话中提到的实体（人名、地名、组织等）并记忆
type EntityMemory struct {
	mu       sync.RWMutex
	entries  []coremem.Entry
	entities map[string]*Entity // 实体名称 -> 实体信息
	capacity int
}

// Entity 实体信息
type Entity struct {
	// Name 实体名称
	Name string `json:"name"`

	// Type 实体类型（person/place/org/other）
	Type string `json:"type"`

	// Description 实体描述
	Description string `json:"description"`

	// Mentions 提及次数
	Mentions int `json:"mentions"`

	// LastMentioned 最后提及时间
	LastMentioned time.Time `json:"last_mentioned"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewEntityMemory 创建实体记忆
func NewEntityMemory(capacity int) *EntityMemory {
	if capacity <= 0 {
		capacity = 100
	}
	return &EntityMemory{
		entries:  make([]coremem.Entry, 0),
		entities: make(map[string]*Entity),
		capacity: capacity,
	}
}

func (m *EntityMemory) Save(_ context.Context, entry coremem.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// FIFO
	if len(m.entries) >= m.capacity {
		m.entries = m.entries[1:]
	}
	m.entries = append(m.entries, entry)

	// 提取实体（简单实现：从元数据中获取）
	if entities, ok := entry.Metadata["entities"].([]map[string]string); ok {
		for _, e := range entities {
			name := e["name"]
			if name == "" {
				continue
			}
			if existing, exists := m.entities[name]; exists {
				existing.Mentions++
				existing.LastMentioned = time.Now()
			} else {
				m.entities[name] = &Entity{
					Name:          name,
					Type:          e["type"],
					Description:   e["description"],
					Mentions:      1,
					LastMentioned: time.Now(),
				}
			}
		}
	}

	return nil
}

func (m *EntityMemory) SaveBatch(ctx context.Context, entries []coremem.Entry) error {
	for _, e := range entries {
		if err := m.Save(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (m *EntityMemory) Get(_ context.Context, id string) (*coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.entries {
		if m.entries[i].ID == id {
			entry := m.entries[i]
			return &entry, nil
		}
	}
	return nil, nil
}

func (m *EntityMemory) Search(_ context.Context, query coremem.SearchQuery) ([]coremem.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]coremem.Entry, len(m.entries))
	copy(result, m.entries)

	if query.Limit > 0 && len(result) > query.Limit {
		result = result[len(result)-query.Limit:]
	}

	return result, nil
}

func (m *EntityMemory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.entries {
		if e.ID == id {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *EntityMemory) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = m.entries[:0]
	m.entities = make(map[string]*Entity)
	return nil
}

func (m *EntityMemory) Stats() coremem.MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return coremem.MemoryStats{EntryCount: len(m.entries)}
}

// GetEntity 获取实体信息
func (m *EntityMemory) GetEntity(name string) *Entity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entities[name]
}

// AllEntities 返回所有实体
func (m *EntityMemory) AllEntities() map[string]*Entity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Entity, len(m.entities))
	for k, v := range m.entities {
		result[k] = v
	}
	return result
}

var _ coremem.Memory = (*EntityMemory)(nil)
