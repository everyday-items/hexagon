package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	coremem "github.com/everyday-items/ai-core/memory"
)

// newTestPersistentMemory 创建测试用的 PersistentMemory（基于 InMemoryStore）
func newTestPersistentMemory(namespace ...string) (*PersistentMemory, func()) {
	s := NewInMemoryStore()
	pm := NewPersistentMemory(s, namespace)
	return pm, func() { s.Close() }
}

func TestPersistentMemory_SaveAndGet(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("users", "u1")
	defer cleanup()
	ctx := context.Background()

	entry := coremem.Entry{
		ID:       "e1",
		Role:     "user",
		Content:  "你好世界",
		Metadata: map[string]any{"source": "test"},
	}
	if err := pm.Save(ctx, entry); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	got, err := pm.Get(ctx, "e1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got == nil {
		t.Fatal("Get 返回 nil，期望非 nil")
	}
	if got.ID != "e1" {
		t.Errorf("ID = %q, 期望 %q", got.ID, "e1")
	}
	if got.Role != "user" {
		t.Errorf("Role = %q, 期望 %q", got.Role, "user")
	}
	if got.Content != "你好世界" {
		t.Errorf("Content = %q, 期望 %q", got.Content, "你好世界")
	}
	if got.Metadata["source"] != "test" {
		t.Errorf("Metadata[source] = %v, 期望 %q", got.Metadata["source"], "test")
	}
}

func TestPersistentMemory_SaveAutoGenerateID(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("test")
	defer cleanup()
	ctx := context.Background()

	entry := coremem.Entry{
		Role:    "assistant",
		Content: "自动生成 ID",
	}
	if err := pm.Save(ctx, entry); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	// 验证 Stats 有一条
	stats := pm.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("EntryCount = %d, 期望 1", stats.EntryCount)
	}
}

func TestPersistentMemory_SaveBatch(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("batch")
	defer cleanup()
	ctx := context.Background()

	entries := []coremem.Entry{
		{ID: "b1", Role: "user", Content: "消息1"},
		{ID: "b2", Role: "assistant", Content: "回复1"},
		{ID: "b3", Role: "user", Content: "消息2"},
	}
	if err := pm.SaveBatch(ctx, entries); err != nil {
		t.Fatalf("SaveBatch 失败: %v", err)
	}

	stats := pm.Stats()
	if stats.EntryCount != 3 {
		t.Fatalf("EntryCount = %d, 期望 3", stats.EntryCount)
	}

	for _, e := range entries {
		got, err := pm.Get(ctx, e.ID)
		if err != nil {
			t.Fatalf("Get(%s) 失败: %v", e.ID, err)
		}
		if got == nil {
			t.Fatalf("Get(%s) 返回 nil", e.ID)
		}
		if got.Content != e.Content {
			t.Errorf("Get(%s).Content = %q, 期望 %q", e.ID, got.Content, e.Content)
		}
	}
}

func TestPersistentMemory_GetNotFound(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("test")
	defer cleanup()
	ctx := context.Background()

	got, err := pm.Get(ctx, "not-exist")
	if err != nil {
		t.Fatalf("Get 不应返回错误: %v", err)
	}
	if got != nil {
		t.Fatal("Get 应返回 nil")
	}
}

func TestPersistentMemory_Delete(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("del")
	defer cleanup()
	ctx := context.Background()

	if err := pm.Save(ctx, coremem.Entry{ID: "d1", Role: "user", Content: "待删除"}); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	if err := pm.Delete(ctx, "d1"); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	got, err := pm.Get(ctx, "d1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got != nil {
		t.Fatal("删除后 Get 应返回 nil")
	}
}

func TestPersistentMemory_Clear(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("clear")
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := pm.Save(ctx, coremem.Entry{
			ID: fmt.Sprintf("c%d", i), Role: "user", Content: "test",
		}); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	if err := pm.Clear(ctx); err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}

	stats := pm.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("Clear 后 EntryCount = %d, 期望 0", stats.EntryCount)
	}
}

func TestPersistentMemory_SearchByKeyword(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("search")
	defer cleanup()
	ctx := context.Background()

	entries := []coremem.Entry{
		{ID: "s1", Role: "user", Content: "Go 语言编程"},
		{ID: "s2", Role: "assistant", Content: "Python 数据分析"},
		{ID: "s3", Role: "user", Content: "Go 并发模型"},
	}
	for _, e := range entries {
		if err := pm.Save(ctx, e); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	// 搜索 "Go"
	results, err := pm.Search(ctx, coremem.SearchQuery{Query: "Go", Limit: 10})
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("搜索 Go 结果数 = %d, 期望 2", len(results))
	}
}

func TestPersistentMemory_SearchByRole(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("search-role")
	defer cleanup()
	ctx := context.Background()

	entries := []coremem.Entry{
		{ID: "r1", Role: "user", Content: "问题"},
		{ID: "r2", Role: "assistant", Content: "回答"},
		{ID: "r3", Role: "user", Content: "追问"},
		{ID: "r4", Role: "system", Content: "系统消息"},
	}
	for _, e := range entries {
		if err := pm.Save(ctx, e); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	// 只搜索 user 角色
	results, err := pm.Search(ctx, coremem.SearchQuery{
		Roles: []string{"user"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("搜索 user 角色结果数 = %d, 期望 2", len(results))
	}
	for _, r := range results {
		if r.Role != "user" {
			t.Errorf("结果角色 = %q, 期望 user", r.Role)
		}
	}
}

func TestPersistentMemory_SearchByTimeRange(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("search-time")
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	entries := []coremem.Entry{
		{ID: "t1", Role: "user", Content: "旧消息", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "t2", Role: "user", Content: "中间消息", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "t3", Role: "user", Content: "新消息", CreatedAt: now},
	}
	for _, e := range entries {
		e.UpdatedAt = e.CreatedAt
		if err := pm.Save(ctx, e); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	// 搜索 1.5 小时前到现在的条目
	since := now.Add(-90 * time.Minute)
	results, err := pm.Search(ctx, coremem.SearchQuery{
		Since: &since,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("时间范围搜索结果数 = %d, 期望 2", len(results))
	}
}

func TestPersistentMemory_SearchOrderDesc(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("search-order")
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	entries := []coremem.Entry{
		{ID: "o1", Role: "user", Content: "第一条", CreatedAt: now.Add(-2 * time.Second)},
		{ID: "o2", Role: "user", Content: "第二条", CreatedAt: now.Add(-1 * time.Second)},
		{ID: "o3", Role: "user", Content: "第三条", CreatedAt: now},
	}
	for _, e := range entries {
		e.UpdatedAt = e.CreatedAt
		if err := pm.Save(ctx, e); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	// 降序
	results, err := pm.Search(ctx, coremem.SearchQuery{
		Limit:     10,
		OrderDesc: true,
	})
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("降序搜索结果数 = %d, 期望 3", len(results))
	}
	if results[0].ID != "o3" {
		t.Errorf("降序第一条应为 o3, 实际 %s", results[0].ID)
	}
	if results[2].ID != "o1" {
		t.Errorf("降序最后一条应为 o1, 实际 %s", results[2].ID)
	}
}

func TestPersistentMemory_SearchPagination(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("search-page")
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	for i := 0; i < 10; i++ {
		if err := pm.Save(ctx, coremem.Entry{
			ID:        fmt.Sprintf("p%d", i),
			Role:      "user",
			Content:   fmt.Sprintf("消息 %d", i),
			CreatedAt: now.Add(time.Duration(i) * time.Second),
			UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	// 第一页，3 条
	page1, err := pm.Search(ctx, coremem.SearchQuery{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("Search 页1 失败: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("页1结果数 = %d, 期望 3", len(page1))
	}

	// 第二页，3 条
	page2, err := pm.Search(ctx, coremem.SearchQuery{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("Search 页2 失败: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("页2结果数 = %d, 期望 3", len(page2))
	}

	// 确保两页不重叠
	if page1[0].ID == page2[0].ID {
		t.Error("页1和页2不应重叠")
	}
}

func TestPersistentMemory_Stats(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("stats")
	defer cleanup()
	ctx := context.Background()

	// 空 Stats
	stats := pm.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("空 Stats EntryCount = %d", stats.EntryCount)
	}
	if stats.OldestEntry != nil || stats.NewestEntry != nil {
		t.Fatal("空 Stats 时间应为 nil")
	}

	now := time.Now()
	entries := []coremem.Entry{
		{ID: "st1", Role: "user", Content: "早", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)},
		{ID: "st2", Role: "user", Content: "中", CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute)},
		{ID: "st3", Role: "user", Content: "晚", CreatedAt: now, UpdatedAt: now},
	}
	for _, e := range entries {
		if err := pm.Save(ctx, e); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
	}

	stats = pm.Stats()
	if stats.EntryCount != 3 {
		t.Fatalf("Stats EntryCount = %d, 期望 3", stats.EntryCount)
	}
	if stats.OldestEntry == nil || stats.NewestEntry == nil {
		t.Fatal("Stats 时间不应为 nil")
	}
}

func TestPersistentMemory_EmbeddingRoundTrip(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("embed")
	defer cleanup()
	ctx := context.Background()

	entry := coremem.Entry{
		ID:        "emb1",
		Role:      "user",
		Content:   "向量测试",
		Embedding: []float32{0.1, 0.2, 0.3, 0.4},
	}
	if err := pm.Save(ctx, entry); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	got, err := pm.Get(ctx, "emb1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got == nil {
		t.Fatal("Get 返回 nil")
	}
	if len(got.Embedding) != 4 {
		t.Fatalf("Embedding 长度 = %d, 期望 4", len(got.Embedding))
	}

	// 检查精度（float32 → float64 → float32 可能有精度损失，这里用误差阈值）
	for i, expected := range entry.Embedding {
		diff := got.Embedding[i] - expected
		if diff > 0.001 || diff < -0.001 {
			t.Errorf("Embedding[%d] = %f, 期望 %f", i, got.Embedding[i], expected)
		}
	}
}

func TestPersistentMemory_NamespaceIsolation(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()

	pm1 := NewPersistentMemory(s, []string{"users", "alice"})
	pm2 := NewPersistentMemory(s, []string{"users", "bob"})
	ctx := context.Background()

	if err := pm1.Save(ctx, coremem.Entry{ID: "a1", Role: "user", Content: "Alice 的记忆"}); err != nil {
		t.Fatalf("pm1.Save 失败: %v", err)
	}
	if err := pm2.Save(ctx, coremem.Entry{ID: "b1", Role: "user", Content: "Bob 的记忆"}); err != nil {
		t.Fatalf("pm2.Save 失败: %v", err)
	}

	// Alice 只能看到自己的
	got, _ := pm1.Get(ctx, "a1")
	if got == nil || got.Content != "Alice 的记忆" {
		t.Error("Alice 应能看到自己的记忆")
	}
	got, _ = pm1.Get(ctx, "b1")
	if got != nil {
		t.Error("Alice 不应看到 Bob 的记忆")
	}

	// Bob 只能看到自己的
	got, _ = pm2.Get(ctx, "b1")
	if got == nil || got.Content != "Bob 的记忆" {
		t.Error("Bob 应能看到自己的记忆")
	}
	got, _ = pm2.Get(ctx, "a1")
	if got != nil {
		t.Error("Bob 不应看到 Alice 的记忆")
	}

	// Alice Clear 不影响 Bob
	if err := pm1.Clear(ctx); err != nil {
		t.Fatalf("pm1.Clear 失败: %v", err)
	}
	stats1 := pm1.Stats()
	stats2 := pm2.Stats()
	if stats1.EntryCount != 0 {
		t.Errorf("Alice Clear 后 EntryCount = %d", stats1.EntryCount)
	}
	if stats2.EntryCount != 1 {
		t.Errorf("Bob 应不受影响, EntryCount = %d", stats2.EntryCount)
	}
}

func TestPersistentMemory_Namespace(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("a", "b", "c")
	defer cleanup()

	ns := pm.Namespace()
	if len(ns) != 3 || ns[0] != "a" || ns[1] != "b" || ns[2] != "c" {
		t.Errorf("Namespace() = %v, 期望 [a b c]", ns)
	}

	// 修改返回值不影响内部
	ns[0] = "mutated"
	ns2 := pm.Namespace()
	if ns2[0] != "a" {
		t.Error("Namespace() 返回的应是副本")
	}
}

func TestPersistentMemory_Store(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("test")
	defer cleanup()

	if pm.Store() == nil {
		t.Error("Store() 不应返回 nil")
	}
}

func TestPersistentMemory_SavePreservesTimestamps(t *testing.T) {
	pm, cleanup := newTestPersistentMemory("ts")
	defer cleanup()
	ctx := context.Background()

	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := coremem.Entry{
		ID:        "ts1",
		Role:      "user",
		Content:   "固定时间",
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	if err := pm.Save(ctx, entry); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	got, err := pm.Get(ctx, "ts1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("CreatedAt = %v, 期望 %v", got.CreatedAt, fixedTime)
	}
	if !got.UpdatedAt.Equal(fixedTime) {
		t.Errorf("UpdatedAt = %v, 期望 %v", got.UpdatedAt, fixedTime)
	}
}
