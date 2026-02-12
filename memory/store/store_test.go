package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestInMemoryStore_PutAndGet 测试基本的存储和获取
func TestInMemoryStore_PutAndGet(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"users", "u123"}
	value := map[string]any{
		"name":  "张三",
		"theme": "dark",
	}

	// 存储
	if err := s.Put(ctx, ns, "preferences", value); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	// 获取
	item, err := s.Get(ctx, ns, "preferences")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if item == nil {
		t.Fatal("Get 返回 nil")
	}
	if item.Key != "preferences" {
		t.Errorf("期望 key=preferences, 实际=%s", item.Key)
	}
	if item.Value["name"] != "张三" {
		t.Errorf("期望 name=张三, 实际=%v", item.Value["name"])
	}
	if item.Value["theme"] != "dark" {
		t.Errorf("期望 theme=dark, 实际=%v", item.Value["theme"])
	}
}

// TestInMemoryStore_GetNonExistent 测试获取不存在的记忆
func TestInMemoryStore_GetNonExistent(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	item, err := s.Get(ctx, []string{"ns"}, "nonexistent")
	if err != nil {
		t.Fatalf("Get 不应返回错误: %v", err)
	}
	if item != nil {
		t.Fatal("Get 不存在的 key 应返回 nil")
	}
}

// TestInMemoryStore_PutUpdate 测试更新已存在的记忆
func TestInMemoryStore_PutUpdate(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"users", "u1"}

	// 首次存储
	if err := s.Put(ctx, ns, "profile", map[string]any{"name": "Alice"}); err != nil {
		t.Fatal(err)
	}

	item1, _ := s.Get(ctx, ns, "profile")
	createdAt := item1.CreatedAt

	// 短暂等待以确保时间差异
	time.Sleep(time.Millisecond)

	// 更新
	if err := s.Put(ctx, ns, "profile", map[string]any{"name": "Alice Updated"}); err != nil {
		t.Fatal(err)
	}

	item2, _ := s.Get(ctx, ns, "profile")

	// 创建时间应保持不变
	if !item2.CreatedAt.Equal(createdAt) {
		t.Error("更新后创建时间不应改变")
	}
	// 更新时间应变化
	if !item2.UpdatedAt.After(createdAt) {
		t.Error("更新时间应在创建时间之后")
	}
	// 值应更新
	if item2.Value["name"] != "Alice Updated" {
		t.Errorf("值未更新: %v", item2.Value["name"])
	}
}

// TestInMemoryStore_PutEmptyKey 测试空 key
func TestInMemoryStore_PutEmptyKey(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	err := s.Put(ctx, []string{"ns"}, "", map[string]any{"data": 1})
	if err == nil {
		t.Error("空 key 应返回错误")
	}
}

// TestInMemoryStore_Delete 测试删除
func TestInMemoryStore_Delete(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"test"}
	s.Put(ctx, ns, "key1", map[string]any{"v": 1})

	// 删除
	if err := s.Delete(ctx, ns, "key1"); err != nil {
		t.Fatal(err)
	}

	// 确认已删除
	item, _ := s.Get(ctx, ns, "key1")
	if item != nil {
		t.Error("删除后应返回 nil")
	}

	// 删除不存在的 key 不应报错
	if err := s.Delete(ctx, ns, "nonexistent"); err != nil {
		t.Errorf("删除不存在的 key 不应报错: %v", err)
	}
}

// TestInMemoryStore_List 测试列表
func TestInMemoryStore_List(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"users", "u1"}

	// 添加多条记忆
	s.Put(ctx, ns, "a", map[string]any{"order": 1})
	time.Sleep(time.Millisecond)
	s.Put(ctx, ns, "b", map[string]any{"order": 2})
	time.Sleep(time.Millisecond)
	s.Put(ctx, ns, "c", map[string]any{"order": 3})

	// 添加不同命名空间的记忆
	s.Put(ctx, []string{"users", "u2"}, "x", map[string]any{"order": 4})

	// 列出 u1 的记忆
	items, err := s.List(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("期望 3 条, 实际 %d 条", len(items))
	}

	// 降序
	items, _ = s.List(ctx, ns, WithOrderDesc())
	if len(items) > 1 && items[0].UpdatedAt.Before(items[1].UpdatedAt) {
		t.Error("降序排列不正确")
	}

	// 限制数量
	items, _ = s.List(ctx, ns, WithListLimit(2))
	if len(items) != 2 {
		t.Errorf("期望 2 条, 实际 %d 条", len(items))
	}

	// 偏移量
	items, _ = s.List(ctx, ns, WithListOffset(1), WithListLimit(10))
	if len(items) != 2 {
		t.Errorf("期望 2 条（偏移1）, 实际 %d 条", len(items))
	}

	// 键前缀过滤
	items, _ = s.List(ctx, ns, WithKeyPrefix("a"))
	if len(items) != 1 {
		t.Errorf("期望 1 条（前缀 a）, 实际 %d 条", len(items))
	}
}

// TestInMemoryStore_DeleteNamespace 测试删除命名空间
func TestInMemoryStore_DeleteNamespace(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	// 添加多个命名空间的记忆
	s.Put(ctx, []string{"users", "u1"}, "a", map[string]any{"v": 1})
	s.Put(ctx, []string{"users", "u1"}, "b", map[string]any{"v": 2})
	s.Put(ctx, []string{"users", "u2"}, "c", map[string]any{"v": 3})
	s.Put(ctx, []string{"other"}, "d", map[string]any{"v": 4})

	// 删除 users:u1 命名空间
	if err := s.DeleteNamespace(ctx, []string{"users", "u1"}); err != nil {
		t.Fatal(err)
	}

	// u1 的记忆应被删除
	items, _ := s.List(ctx, []string{"users", "u1"})
	if len(items) != 0 {
		t.Errorf("u1 命名空间应已清空, 实际 %d 条", len(items))
	}

	// u2 的记忆应保留
	item, _ := s.Get(ctx, []string{"users", "u2"}, "c")
	if item == nil {
		t.Error("u2 的记忆不应被删除")
	}

	// other 的记忆应保留
	item, _ = s.Get(ctx, []string{"other"}, "d")
	if item == nil {
		t.Error("other 的记忆不应被删除")
	}
}

// TestInMemoryStore_TTL 测试 TTL 过期
func TestInMemoryStore_TTL(t *testing.T) {
	s := NewInMemoryStore(WithCleanupInterval(50 * time.Millisecond))
	defer s.Close()
	ctx := context.Background()

	ns := []string{"test"}

	// 添加一条 100ms 后过期的记忆
	s.Put(ctx, ns, "expiring", map[string]any{"v": 1}, WithTTL(100*time.Millisecond))

	// 添加一条永不过期的记忆
	s.Put(ctx, ns, "permanent", map[string]any{"v": 2})

	// 立即可以获取
	item, _ := s.Get(ctx, ns, "expiring")
	if item == nil {
		t.Fatal("TTL 未过期时应能获取")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 惰性清理：Get 时检测到过期
	item, _ = s.Get(ctx, ns, "expiring")
	if item != nil {
		t.Error("TTL 过期后不应能获取")
	}

	// 永不过期的记忆仍然存在
	item, _ = s.Get(ctx, ns, "permanent")
	if item == nil {
		t.Error("永不过期的记忆应仍然存在")
	}
}

// TestInMemoryStore_Search 测试搜索
func TestInMemoryStore_Search(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"knowledge"}

	s.Put(ctx, ns, "doc1", map[string]any{
		"title":   "Go 并发编程",
		"content": "goroutine 是 Go 语言的核心特性",
		"type":    "article",
	})
	s.Put(ctx, ns, "doc2", map[string]any{
		"title":   "Python 入门",
		"content": "Python 是一门简单易学的语言",
		"type":    "article",
	})
	s.Put(ctx, ns, "doc3", map[string]any{
		"title":   "Go 错误处理",
		"content": "error 接口是 Go 错误处理的基础",
		"type":    "tutorial",
	})

	// 关键词搜索
	results, err := s.Search(ctx, ns, &SearchQuery{Query: "Go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("期望 2 条关于 Go 的结果, 实际 %d 条", len(results))
	}

	// 元数据过滤
	results, _ = s.Search(ctx, ns, &SearchQuery{
		Filter: map[string]any{"type": "tutorial"},
	})
	if len(results) != 1 {
		t.Errorf("期望 1 条 tutorial, 实际 %d 条", len(results))
	}

	// 关键词 + 过滤
	results, _ = s.Search(ctx, ns, &SearchQuery{
		Query:  "Go",
		Filter: map[string]any{"type": "article"},
	})
	if len(results) != 1 {
		t.Errorf("期望 1 条 Go article, 实际 %d 条", len(results))
	}

	// 无结果
	results, _ = s.Search(ctx, ns, &SearchQuery{Query: "Rust"})
	if len(results) != 0 {
		t.Errorf("期望 0 条关于 Rust 的结果, 实际 %d 条", len(results))
	}

	// 分页
	results, _ = s.Search(ctx, ns, &SearchQuery{Limit: 1})
	if len(results) != 1 {
		t.Errorf("期望 1 条（limit=1）, 实际 %d 条", len(results))
	}
}

// TestInMemoryStore_SearchWithOffset 测试搜索分页
func TestInMemoryStore_SearchWithOffset(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"data"}
	for i := 0; i < 5; i++ {
		s.Put(ctx, ns, fmt.Sprintf("item%d", i), map[string]any{
			"content": "测试数据",
		})
	}

	// 第一页
	results, _ := s.Search(ctx, ns, &SearchQuery{
		Query: "测试",
		Limit: 2,
	})
	if len(results) != 2 {
		t.Errorf("第一页期望 2 条, 实际 %d 条", len(results))
	}

	// 第二页
	results, _ = s.Search(ctx, ns, &SearchQuery{
		Query:  "测试",
		Limit:  2,
		Offset: 2,
	})
	if len(results) != 2 {
		t.Errorf("第二页期望 2 条, 实际 %d 条", len(results))
	}
}

// TestInMemoryStore_NamespaceIsolation 测试命名空间隔离
func TestInMemoryStore_NamespaceIsolation(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	// 不同命名空间可以有相同的 key
	s.Put(ctx, []string{"ns1"}, "key", map[string]any{"v": "ns1"})
	s.Put(ctx, []string{"ns2"}, "key", map[string]any{"v": "ns2"})

	item1, _ := s.Get(ctx, []string{"ns1"}, "key")
	item2, _ := s.Get(ctx, []string{"ns2"}, "key")

	if item1.Value["v"] != "ns1" {
		t.Errorf("ns1 值错误: %v", item1.Value["v"])
	}
	if item2.Value["v"] != "ns2" {
		t.Errorf("ns2 值错误: %v", item2.Value["v"])
	}
}

// TestInMemoryStore_ConcurrentAccess 测试并发安全
func TestInMemoryStore_ConcurrentAccess(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	ns := []string{"concurrent"}
	done := make(chan struct{})

	// 并发写入
	for i := 0; i < 100; i++ {
		go func(i int) {
			s.Put(ctx, ns, fmt.Sprintf("key%d", i), map[string]any{"v": i})
			done <- struct{}{}
		}(i)
	}

	// 等待所有写入完成
	for i := 0; i < 100; i++ {
		<-done
	}

	// 验证所有条目都存在
	items, _ := s.List(ctx, ns)
	if len(items) != 100 {
		t.Errorf("期望 100 条, 实际 %d 条", len(items))
	}
}

// TestInMemoryStore_SearchNilQuery 测试 nil 查询
func TestInMemoryStore_SearchNilQuery(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()

	results, err := s.Search(context.Background(), []string{"ns"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Error("nil 查询应返回 nil")
	}
}

// TestInMemoryStore_ListEmptyNamespace 测试空命名空间列表
func TestInMemoryStore_ListEmptyNamespace(t *testing.T) {
	s := NewInMemoryStore()
	defer s.Close()
	ctx := context.Background()

	s.Put(ctx, []string{"a"}, "k1", map[string]any{"v": 1})
	s.Put(ctx, []string{"b"}, "k2", map[string]any{"v": 2})

	// 空命名空间应列出所有
	items, _ := s.List(ctx, nil)
	if len(items) != 2 {
		t.Errorf("空命名空间应列出所有, 期望 2 条, 实际 %d 条", len(items))
	}
}

// TestItem_IsExpired 测试 Item 过期检查
func TestItem_IsExpired(t *testing.T) {
	// 未设置过期时间
	item1 := &Item{ExpiresAt: nil}
	if item1.IsExpired() {
		t.Error("nil ExpiresAt 不应过期")
	}

	// 未来时间
	future := time.Now().Add(time.Hour)
	item2 := &Item{ExpiresAt: &future}
	if item2.IsExpired() {
		t.Error("未来时间不应过期")
	}

	// 过去时间
	past := time.Now().Add(-time.Hour)
	item3 := &Item{ExpiresAt: &past}
	if !item3.IsExpired() {
		t.Error("过去时间应过期")
	}
}

// 确保 fmt 被使用
var _ = fmt.Sprintf
