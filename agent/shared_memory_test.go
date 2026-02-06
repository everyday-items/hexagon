package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/everyday-items/ai-core/memory"
)

// ============== SharedMemory 基础测试 ==============

func TestSharedMemory_Basic(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	// Save
	entry := memory.Entry{
		ID:        "e1",
		Role:      "assistant",
		Content:   "hello from agent A",
		CreatedAt: time.Now(),
	}
	if err := sm.Save(ctx, entry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Get
	got, err := sm.Get(ctx, "e1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil || got.Content != "hello from agent A" {
		t.Fatalf("expected entry content 'hello from agent A', got %v", got)
	}

	// Search
	results, err := sm.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Stats
	stats := sm.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("expected 1 entry in stats, got %d", stats.EntryCount)
	}

	// Delete
	if err := sm.Delete(ctx, "e1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	got, _ = sm.Get(ctx, "e1")
	if got != nil {
		t.Errorf("expected nil after delete, got %v", got)
	}

	// SaveBatch + Clear
	err = sm.SaveBatch(ctx, []memory.Entry{
		{ID: "b1", Role: "user", Content: "batch1", CreatedAt: time.Now()},
		{ID: "b2", Role: "user", Content: "batch2", CreatedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}
	if sm.Stats().EntryCount != 2 {
		t.Errorf("expected 2 entries after batch, got %d", sm.Stats().EntryCount)
	}
	if err := sm.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if sm.Stats().EntryCount != 0 {
		t.Errorf("expected 0 entries after clear, got %d", sm.Stats().EntryCount)
	}
}

func TestSharedMemory_ImplementsInterface(t *testing.T) {
	// 编译时接口检查（在 shared_memory.go 中已有 var _ memory.Memory = ...）
	var _ memory.Memory = (*SharedMemory)(nil)
	var _ memory.Memory = (*SharedMemoryProxy)(nil)
}

func TestSharedMemory_WithOptions(t *testing.T) {
	sm := NewSharedMemory(WithShortTermCapacity(50))
	ctx := context.Background()

	// 验证容量设置生效
	for i := 0; i < 60; i++ {
		_ = sm.Save(ctx, memory.Entry{
			Role:      "user",
			Content:   "msg",
			CreatedAt: time.Now(),
		})
	}
	// BufferMemory 容量为 50，超出会 FIFO 淘汰
	if sm.Stats().EntryCount != 50 {
		t.Errorf("expected 50 entries with capacity 50, got %d", sm.Stats().EntryCount)
	}
}

// ============== SharedMemoryProxy 测试 ==============

func TestSharedMemoryProxy_SaveSyncs(t *testing.T) {
	sm := NewSharedMemory()
	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A")

	entry := memory.Entry{
		ID:        "p1",
		Role:      "assistant",
		Content:   "agent A result",
		CreatedAt: time.Now(),
	}
	if err := proxy.Save(ctx, entry); err != nil {
		t.Fatalf("Proxy Save failed: %v", err)
	}

	// 本地应该有
	localEntry, _ := local.Get(ctx, "p1")
	if localEntry == nil {
		t.Fatal("expected entry in local memory")
	}

	// 共享也应该有
	sharedEntry, _ := sm.Get(ctx, "p1")
	if sharedEntry == nil {
		t.Fatal("expected entry in shared memory")
	}
	if sharedEntry.Content != "agent A result" {
		t.Errorf("expected content 'agent A result', got '%s'", sharedEntry.Content)
	}
}

func TestSharedMemoryProxy_SearchMerges(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	// 先在共享记忆中存入 Agent A 的结果
	_ = sm.Save(ctx, memory.Entry{
		ID:        "shared-1",
		Role:      "assistant",
		Content:   "result from Agent A",
		Metadata:  map[string]any{"_agent_id": "agent-a", "_agent_name": "Agent A"},
		CreatedAt: time.Now(),
	})

	// Agent B 的本地记忆
	localB := memory.NewBuffer(100)
	_ = localB.Save(ctx, memory.Entry{
		ID:        "local-1",
		Role:      "user",
		Content:   "query from user",
		CreatedAt: time.Now(),
	})

	proxyB := NewSharedMemoryProxy(localB, sm, "agent-b", "Agent B")

	results, err := proxyB.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// 应该包含本地和共享的结果
	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 local + 1 shared), got %d", len(results))
	}

	// 检查两条内容都在
	contents := make(map[string]bool)
	for _, r := range results {
		contents[r.Content] = true
	}
	if !contents["query from user"] {
		t.Error("missing local result 'query from user'")
	}
	if !contents["result from Agent A"] {
		t.Error("missing shared result 'result from Agent A'")
	}
}

func TestSharedMemoryProxy_Dedup(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	// 同一条目同时存在于本地和共享
	entry := memory.Entry{
		ID:        "dup-1",
		Role:      "assistant",
		Content:   "duplicate content",
		CreatedAt: time.Now(),
	}

	local := memory.NewBuffer(100)
	_ = local.Save(ctx, entry)
	_ = sm.Save(ctx, entry)

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A")

	results, err := proxy.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// 应该去重，只返回 1 条
	if len(results) != 1 {
		t.Errorf("expected 1 result after dedup, got %d", len(results))
	}
}

func TestSharedMemoryProxy_WriteDisabled(t *testing.T) {
	sm := NewSharedMemory()
	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A",
		WithWriteToShared(false),
	)

	_ = proxy.Save(ctx, memory.Entry{
		ID:        "no-share",
		Role:      "assistant",
		Content:   "private",
		CreatedAt: time.Now(),
	})

	// 本地应该有
	localEntry, _ := local.Get(ctx, "no-share")
	if localEntry == nil {
		t.Fatal("expected entry in local memory")
	}

	// 共享不应该有
	sharedEntry, _ := sm.Get(ctx, "no-share")
	if sharedEntry != nil {
		t.Error("expected no entry in shared memory when WriteToShared=false")
	}
}

func TestSharedMemoryProxy_ReadDisabled(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	// 共享中有条目
	_ = sm.Save(ctx, memory.Entry{
		ID:        "shared-only",
		Role:      "assistant",
		Content:   "shared content",
		CreatedAt: time.Now(),
	})

	local := memory.NewBuffer(100)
	_ = local.Save(ctx, memory.Entry{
		ID:        "local-only",
		Role:      "user",
		Content:   "local content",
		CreatedAt: time.Now(),
	})

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A",
		WithReadFromShared(false),
	)

	results, err := proxy.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// 应该只有本地结果
	if len(results) != 1 {
		t.Fatalf("expected 1 local result, got %d", len(results))
	}
	if results[0].ID != "local-only" {
		t.Errorf("expected local-only entry, got %s", results[0].ID)
	}
}

func TestSharedMemoryProxy_SharedFailGraceful(t *testing.T) {
	// 使用一个会失败的共享记忆
	failMem := &failingMemory{err: errors.New("storage unavailable")}
	sm := &SharedMemory{shortTerm: failMem}

	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A")

	// Save 应该成功（本地写入成功，共享失败仅警告）
	err := proxy.Save(ctx, memory.Entry{
		ID:        "graceful-1",
		Role:      "user",
		Content:   "test",
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected graceful degradation on Save, got error: %v", err)
	}

	// 验证本地写入成功
	got, _ := local.Get(ctx, "graceful-1")
	if got == nil {
		t.Fatal("expected entry in local memory after graceful save")
	}

	// Search 应该成功（共享搜索失败退化为本地结果）
	results, err := proxy.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("expected graceful degradation on Search, got error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 local result on graceful degradation, got %d", len(results))
	}
}

func TestSharedMemoryProxy_AgentTagging(t *testing.T) {
	sm := NewSharedMemory()
	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-42", "Researcher")

	_ = proxy.Save(ctx, memory.Entry{
		ID:        "tagged-1",
		Role:      "assistant",
		Content:   "tagged entry",
		CreatedAt: time.Now(),
	})

	// 检查本地条目的元数据标记
	got, _ := local.Get(ctx, "tagged-1")
	if got == nil {
		t.Fatal("expected entry in local memory")
	}
	if got.Metadata["_agent_id"] != "agent-42" {
		t.Errorf("expected _agent_id='agent-42', got %v", got.Metadata["_agent_id"])
	}
	if got.Metadata["_agent_name"] != "Researcher" {
		t.Errorf("expected _agent_name='Researcher', got %v", got.Metadata["_agent_name"])
	}

	// 检查共享条目也有标记
	sharedGot, _ := sm.Get(ctx, "tagged-1")
	if sharedGot == nil {
		t.Fatal("expected entry in shared memory")
	}
	if sharedGot.Metadata["_agent_id"] != "agent-42" {
		t.Errorf("expected shared _agent_id='agent-42', got %v", sharedGot.Metadata["_agent_id"])
	}
}

func TestSharedMemoryProxy_SaveBatch(t *testing.T) {
	sm := NewSharedMemory()
	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A")

	entries := []memory.Entry{
		{ID: "batch-1", Role: "user", Content: "msg1", CreatedAt: time.Now()},
		{ID: "batch-2", Role: "assistant", Content: "msg2", CreatedAt: time.Now()},
	}
	if err := proxy.SaveBatch(ctx, entries); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	// 两条都应该在本地和共享中
	if local.Stats().EntryCount != 2 {
		t.Errorf("expected 2 entries in local, got %d", local.Stats().EntryCount)
	}
	if sm.Stats().EntryCount != 2 {
		t.Errorf("expected 2 entries in shared, got %d", sm.Stats().EntryCount)
	}
}

func TestSharedMemoryProxy_ConcurrentAccess(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	const goroutines = 10
	const entriesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			local := memory.NewBuffer(100)
			proxy := NewSharedMemoryProxy(local, sm, "agent-"+string(rune('0'+id)), "Agent")

			for i := 0; i < entriesPerGoroutine; i++ {
				_ = proxy.Save(ctx, memory.Entry{
					Role:      "assistant",
					Content:   "concurrent write",
					CreatedAt: time.Now(),
				})
			}

			// 并发搜索
			_, _ = proxy.Search(ctx, memory.SearchQuery{Limit: 5})
		}(g)
	}

	wg.Wait()

	// 所有写入都应该到达共享记忆（BufferMemory 默认 200 容量，总共 200 条）
	stats := sm.Stats()
	if stats.EntryCount != goroutines*entriesPerGoroutine {
		t.Errorf("expected %d entries in shared memory, got %d",
			goroutines*entriesPerGoroutine, stats.EntryCount)
	}
}

func TestSharedMemoryProxy_DelegatesMethods(t *testing.T) {
	sm := NewSharedMemory()
	local := memory.NewBuffer(100)
	ctx := context.Background()

	proxy := NewSharedMemoryProxy(local, sm, "agent-1", "Agent A")

	// Save then test Get (delegates to local)
	_ = proxy.Save(ctx, memory.Entry{
		ID: "del-1", Role: "user", Content: "test", CreatedAt: time.Now(),
	})

	got, err := proxy.Get(ctx, "del-1")
	if err != nil || got == nil {
		t.Fatal("Get delegation failed")
	}

	// Delete (delegates to local)
	if err := proxy.Delete(ctx, "del-1"); err != nil {
		t.Fatalf("Delete delegation failed: %v", err)
	}
	got, _ = proxy.Get(ctx, "del-1")
	if got != nil {
		t.Error("expected nil after delete")
	}

	// Clear (delegates to local)
	_ = proxy.Save(ctx, memory.Entry{
		ID: "del-2", Role: "user", Content: "test2", CreatedAt: time.Now(),
	})
	if err := proxy.Clear(ctx); err != nil {
		t.Fatalf("Clear delegation failed: %v", err)
	}
	if proxy.Stats().EntryCount != 0 {
		t.Error("expected 0 entries after Clear")
	}

	// Local accessor
	if proxy.Local() != local {
		t.Error("Local() should return original memory")
	}
}

// ============== Team 集成测试 ==============

func TestTeam_WithSharedMemory(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	agentA := NewBaseAgent(WithName("Agent A"))
	agentB := NewBaseAgent(WithName("Agent B"))

	_ = NewTeam("test-team",
		WithAgents(agentA, agentB),
		WithSharedMemory(sm),
	)

	// Agent A 的 Memory 应该被包装为 SharedMemoryProxy
	proxyA, ok := agentA.Memory().(*SharedMemoryProxy)
	if !ok {
		t.Fatal("expected Agent A memory to be SharedMemoryProxy")
	}

	// Agent A 写入一条记忆
	_ = proxyA.Save(ctx, memory.Entry{
		ID:        "a-result",
		Role:      "assistant",
		Content:   "Agent A finished task",
		CreatedAt: time.Now(),
	})

	// Agent B 应该能通过共享记忆搜索到 Agent A 的结果
	proxyB, ok := agentB.Memory().(*SharedMemoryProxy)
	if !ok {
		t.Fatal("expected Agent B memory to be SharedMemoryProxy")
	}

	results, err := proxyB.Search(ctx, memory.SearchQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Agent B search failed: %v", err)
	}

	// 应该至少包含 Agent A 的共享结果
	found := false
	for _, r := range results {
		if r.Content == "Agent A finished task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Agent B should see Agent A's result via shared memory")
	}
}

func TestTeam_WithSharedMemory_Collaborative(t *testing.T) {
	sm := NewSharedMemory()
	ctx := context.Background()

	agents := make([]*BaseAgent, 5)
	agentInterfaces := make([]Agent, 5)
	for i := range agents {
		agents[i] = NewBaseAgent(WithName("Agent"))
		agentInterfaces[i] = agents[i]
	}

	_ = NewTeam("collab-team",
		WithAgents(agentInterfaces...),
		WithMode(TeamModeCollaborative),
		WithSharedMemory(sm),
	)

	// 并发写入，模拟协作模式
	var wg sync.WaitGroup
	wg.Add(len(agents))
	for i, ag := range agents {
		go func(idx int, agent *BaseAgent) {
			defer wg.Done()
			_ = agent.Memory().(*SharedMemoryProxy).Save(ctx, memory.Entry{
				Role:      "assistant",
				Content:   "result",
				CreatedAt: time.Now(),
			})
		}(i, ag)
	}
	wg.Wait()

	// 所有结果应该在共享记忆中
	if sm.Stats().EntryCount != len(agents) {
		t.Errorf("expected %d entries in shared memory, got %d",
			len(agents), sm.Stats().EntryCount)
	}
}

func TestTeam_AddAgent_WrapsMemory(t *testing.T) {
	sm := NewSharedMemory()

	team := NewTeam("test-team",
		WithSharedMemory(sm),
	)

	// 动态添加 Agent
	agent := NewBaseAgent(WithName("Dynamic Agent"))
	team.AddAgent(agent)

	// 应该自动被包装
	if _, ok := agent.Memory().(*SharedMemoryProxy); !ok {
		t.Error("dynamically added agent should have SharedMemoryProxy")
	}
}

func TestTeam_WithoutSharedMemory(t *testing.T) {
	agentA := NewBaseAgent(WithName("Agent A"))
	agentB := NewBaseAgent(WithName("Agent B"))

	_ = NewTeam("test-team",
		WithAgents(agentA, agentB),
	)

	// 不设置共享记忆时，Agent 的 Memory 应该保持原样
	if _, ok := agentA.Memory().(*SharedMemoryProxy); ok {
		t.Error("expected original memory when no shared memory is set")
	}
	if _, ok := agentB.Memory().(*SharedMemoryProxy); ok {
		t.Error("expected original memory when no shared memory is set")
	}
}

func TestTeam_SharedMemory_Accessor(t *testing.T) {
	sm := NewSharedMemory()
	team := NewTeam("test-team", WithSharedMemory(sm))

	if team.SharedMemory() != sm {
		t.Error("SharedMemory() should return the configured shared memory")
	}

	teamNoMem := NewTeam("no-mem-team")
	if teamNoMem.SharedMemory() != nil {
		t.Error("SharedMemory() should return nil when not configured")
	}
}

// ============== mergeEntries 测试 ==============

func TestMergeEntries(t *testing.T) {
	t.Run("empty secondary", func(t *testing.T) {
		primary := []memory.Entry{{ID: "1", Content: "a"}}
		result := mergeEntries(primary, nil, 0)
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})

	t.Run("no overlap", func(t *testing.T) {
		primary := []memory.Entry{{ID: "1", Content: "a"}}
		secondary := []memory.Entry{{ID: "2", Content: "b"}}
		result := mergeEntries(primary, secondary, 0)
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
	})

	t.Run("with overlap", func(t *testing.T) {
		primary := []memory.Entry{{ID: "1", Content: "a"}}
		secondary := []memory.Entry{{ID: "1", Content: "a-copy"}, {ID: "2", Content: "b"}}
		result := mergeEntries(primary, secondary, 0)
		if len(result) != 2 {
			t.Errorf("expected 2 (dedup), got %d", len(result))
		}
		// primary 的版本应该保留
		if result[0].Content != "a" {
			t.Errorf("expected primary version 'a', got '%s'", result[0].Content)
		}
	})

	t.Run("with limit", func(t *testing.T) {
		primary := []memory.Entry{{ID: "1"}, {ID: "2"}}
		secondary := []memory.Entry{{ID: "3"}, {ID: "4"}}
		result := mergeEntries(primary, secondary, 3)
		if len(result) != 3 {
			t.Errorf("expected 3 with limit, got %d", len(result))
		}
	})

	t.Run("empty ID entries", func(t *testing.T) {
		// 无 ID 的条目不参与去重
		primary := []memory.Entry{{Content: "a"}}
		secondary := []memory.Entry{{Content: "b"}}
		result := mergeEntries(primary, secondary, 0)
		if len(result) != 2 {
			t.Errorf("expected 2 for empty ID entries, got %d", len(result))
		}
	})
}

// ============== MemorySetter 测试 ==============

func TestBaseAgent_SetMemory(t *testing.T) {
	agent := NewBaseAgent(WithName("test"))
	originalMem := agent.Memory()

	newMem := memory.NewBuffer(50)
	agent.SetMemory(newMem)

	if agent.Memory() == originalMem {
		t.Error("SetMemory should replace the memory")
	}
	if agent.Memory() != newMem {
		t.Error("SetMemory should set the new memory")
	}
}

func TestMemorySetter_Interface(t *testing.T) {
	// 验证 BaseAgent 实现了 MemorySetter 接口
	var _ MemorySetter = (*BaseAgent)(nil)
}

// ============== 测试辅助 ==============

// failingMemory 模拟失败的 Memory 实现
type failingMemory struct {
	err error
}

func (m *failingMemory) Save(_ context.Context, _ memory.Entry) error {
	return m.err
}
func (m *failingMemory) SaveBatch(_ context.Context, _ []memory.Entry) error {
	return m.err
}
func (m *failingMemory) Get(_ context.Context, _ string) (*memory.Entry, error) {
	return nil, m.err
}
func (m *failingMemory) Search(_ context.Context, _ memory.SearchQuery) ([]memory.Entry, error) {
	return nil, m.err
}
func (m *failingMemory) Delete(_ context.Context, _ string) error {
	return m.err
}
func (m *failingMemory) Clear(_ context.Context) error {
	return m.err
}
func (m *failingMemory) Stats() memory.MemoryStats {
	return memory.MemoryStats{}
}
