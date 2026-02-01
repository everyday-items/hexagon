// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
package mock

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/hexagon/internal/util"
)

// Memory Mock Memory 实现
type Memory struct {
	entries     map[string]memory.Entry
	mu          sync.RWMutex
	saveCalls   []memory.Entry
	searchCalls []memory.SearchQuery
}

// NewMemory 创建 Mock Memory
func NewMemory() *Memory {
	return &Memory{
		entries:     make(map[string]memory.Entry),
		saveCalls:   make([]memory.Entry, 0),
		searchCalls: make([]memory.SearchQuery, 0),
	}
}

// Save 保存记忆条目
func (m *Memory) Save(ctx context.Context, entry memory.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 记录调用
	m.saveCalls = append(m.saveCalls, entry)

	// 保存条目
	if entry.ID == "" {
		entry.ID = util.GenerateID("mock")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	m.entries[entry.ID] = entry

	return nil
}

// SaveBatch 批量保存
func (m *Memory) SaveBatch(ctx context.Context, entries []memory.Entry) error {
	for _, entry := range entries {
		if err := m.Save(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// Get 获取记忆条目
func (m *Memory) Get(ctx context.Context, id string) (*memory.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	entry, ok := m.entries[id]
	if !ok {
		return nil, nil
	}
	return &entry, nil
}

// Search 搜索记忆
func (m *Memory) Search(ctx context.Context, query memory.SearchQuery) ([]memory.Entry, error) {
	m.mu.Lock()
	m.searchCalls = append(m.searchCalls, query)
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var results []memory.Entry
	for _, entry := range m.entries {
		// 简单匹配：检查内容是否包含查询文本
		if query.Query != "" && !containsIgnoreCase(entry.Content, query.Query) {
			continue
		}

		// 检查角色
		if len(query.Roles) > 0 && !containsString(query.Roles, entry.Role) {
			continue
		}

		// 检查元数据
		if !matchMetadata(entry.Metadata, query.Metadata) {
			continue
		}

		results = append(results, entry)

		// 限制结果数量
		if query.Limit > 0 && len(results) >= query.Limit {
			break
		}
	}

	return results, nil
}

// Delete 删除记忆条目
func (m *Memory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	delete(m.entries, id)
	return nil
}

// Clear 清空记忆
func (m *Memory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	m.entries = make(map[string]memory.Entry)
	return nil
}

// Stats 返回统计信息
func (m *Memory) Stats() memory.MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return memory.MemoryStats{
		EntryCount: len(m.entries),
	}
}

// ============== 测试辅助方法 ==============

// Entries 返回所有条目
func (m *Memory) Entries() map[string]memory.Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make(map[string]memory.Entry)
	for k, v := range m.entries {
		entries[k] = v
	}
	return entries
}

// SaveCalls 返回所有保存调用
func (m *Memory) SaveCalls() []memory.Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]memory.Entry, len(m.saveCalls))
	copy(calls, m.saveCalls)
	return calls
}

// SearchCalls 返回所有搜索调用
func (m *Memory) SearchCalls() []memory.SearchQuery {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]memory.SearchQuery, len(m.searchCalls))
	copy(calls, m.searchCalls)
	return calls
}

// Reset 重置状态
func (m *Memory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]memory.Entry)
	m.saveCalls = make([]memory.Entry, 0)
	m.searchCalls = make([]memory.SearchQuery, 0)
}

// AddEntry 直接添加条目（用于测试准备）
func (m *Memory) AddEntry(entry memory.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = util.GenerateID("mock")
	}
	m.entries[entry.ID] = entry
}

// AddEntries 批量添加条目
func (m *Memory) AddEntries(entries []memory.Entry) {
	for _, entry := range entries {
		m.AddEntry(entry)
	}
}

var _ memory.Memory = (*Memory)(nil)

// ============== 辅助函数 ==============

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func matchMetadata(entryMeta, queryMeta map[string]any) bool {
	if queryMeta == nil {
		return true
	}

	for k, v := range queryMeta {
		if entryMeta == nil {
			return false
		}
		if entryMeta[k] != v {
			return false
		}
	}
	return true
}
