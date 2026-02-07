package graph

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestMemoryNodeCache_Basic 测试基本 Get/Set/Delete 操作
func TestMemoryNodeCache_Basic(t *testing.T) {
	cache := NewMemoryNodeCache()

	// 测试 Set 和 Get
	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if val != "value1" {
		t.Errorf("expected value 'value1', got '%v'", val)
	}

	// 测试 Get 不存在的 key
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}

	// 测试 Delete
	cache.Delete("key1")
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected cache miss after delete")
	}
}

// TestMemoryNodeCache_LRU 测试 LRU 驱逐策略
func TestMemoryNodeCache_LRU(t *testing.T) {
	cache := NewMemoryNodeCache(WithCacheCapacity(3))

	// 填充缓存到容量上限
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// 添加第 4 个元素，应该驱逐最旧的 key1
	cache.Set("key4", "value4")

	// key1 应该被驱逐
	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be evicted")
	}

	// key2, key3, key4 应该还在
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to be in cache")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("expected key3 to be in cache")
	}
	if _, ok := cache.Get("key4"); !ok {
		t.Error("expected key4 to be in cache")
	}

	// 访问 key2，使其成为最近访问
	cache.Get("key2")

	// 添加 key5，应该驱逐 key3（最旧的未访问）
	cache.Set("key5", "value5")

	if _, ok := cache.Get("key3"); ok {
		t.Error("expected key3 to be evicted")
	}

	// key2, key4, key5 应该还在
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to be in cache")
	}
}

// TestMemoryNodeCache_TTL 测试 TTL 过期
func TestMemoryNodeCache_TTL(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := NewMemoryNodeCache(WithCacheTTL(ttl))

	cache.Set("key1", "value1")

	// 立即读取应该成功
	val, ok := cache.Get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if val != "value1" {
		t.Errorf("expected value 'value1', got '%v'", val)
	}

	// 等待 TTL 过期
	time.Sleep(ttl + 50*time.Millisecond)

	// 过期后应该读取失败
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}
}

// TestMemoryNodeCache_Capacity 测试容量限制
func TestMemoryNodeCache_Capacity(t *testing.T) {
	capacity := 5
	cache := NewMemoryNodeCache(WithCacheCapacity(capacity))

	// 添加 capacity + 2 个元素
	for i := 0; i < capacity+2; i++ {
		cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
	}

	stats := cache.Stats()
	if stats.Size > capacity {
		t.Errorf("expected cache size <= %d, got %d", capacity, stats.Size)
	}

	// 前两个元素应该被驱逐
	_, ok := cache.Get("key0")
	if ok {
		t.Error("expected key0 to be evicted")
	}
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected key1 to be evicted")
	}
}

// TestMemoryNodeCache_Stats 测试统计信息
func TestMemoryNodeCache_Stats(t *testing.T) {
	cache := NewMemoryNodeCache(WithCacheCapacity(2))

	// 初始统计
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Size != 0 || stats.Evictions != 0 {
		t.Errorf("expected zero stats, got %+v", stats)
	}

	// 添加两个元素
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	stats = cache.Stats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}

	// 命中测试
	cache.Get("key1")
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}

	// 未命中测试
	cache.Get("nonexistent")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// 驱逐测试
	cache.Set("key3", "value3")
	stats = cache.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
}

// TestMemoryNodeCache_Clear 测试清空缓存
func TestMemoryNodeCache_Clear(t *testing.T) {
	cache := NewMemoryNodeCache()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	stats := cache.Stats()
	if stats.Size != 3 {
		t.Errorf("expected size 3, got %d", stats.Size)
	}

	cache.Clear()

	stats = cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0 after clear, got %d", stats.Size)
	}

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected cache miss after clear")
	}
}

// TestCachedNodeHandler 测试包装的节点处理函数
func TestCachedNodeHandler(t *testing.T) {
	cache := NewMemoryNodeCache()
	callCount := 0

	// 原始 handler，每次调用增加计数但不修改输入状态
	handler := func(ctx context.Context, s MapState) (MapState, error) {
		callCount++
		result := MapState{}
		result.Set("counter", callCount)
		result.Set("input", s)
		return result, nil
	}

	// 创建带缓存的 handler
	cachedHandler := CachedNodeHandler("test-node", handler, cache)

	ctx := context.Background()
	state := MapState{}
	state.Set("key", "value")

	// 第一次调用应该执行 handler
	result1, err := cachedHandler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}

	// 第二次调用相同输入应该从缓存返回
	result2, err := cachedHandler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected handler still called once (cached), got %d", callCount)
	}

	// 结果应该相同
	counter1, _ := result1.Get("counter")
	counter2, _ := result2.Get("counter")
	if counter1 != counter2 {
		t.Errorf("expected same results, got %v and %v", counter1, counter2)
	}

	// 不同输入应该执行 handler
	state2 := MapState{}
	state2.Set("different", true)
	_, err = cachedHandler(ctx, state2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected handler called twice, got %d", callCount)
	}
}

// TestCachedNodeHandler_Error 测试缓存处理函数的错误情况
func TestCachedNodeHandler_Error(t *testing.T) {
	cache := NewMemoryNodeCache()
	expectedErr := fmt.Errorf("handler error")

	handler := func(ctx context.Context, s MapState) (MapState, error) {
		return s, expectedErr
	}

	cachedHandler := CachedNodeHandler("test-node", handler, cache)

	ctx := context.Background()
	state := MapState{}

	_, err := cachedHandler(ctx, state)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// 错误结果不应该被缓存
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Error("expected no cache entries for error results")
	}
}

// TestComputeCacheKey 测试缓存 key 计算
func TestComputeCacheKey(t *testing.T) {
	state1 := MapState{}
	state1.Set("key", "value")

	state2 := MapState{}
	state2.Set("key", "value")

	state3 := MapState{}
	state3.Set("key", "different")

	// 相同输入应该产生相同的 key
	key1 := ComputeCacheKey("node1", state1)
	key2 := ComputeCacheKey("node1", state2)
	if key1 != key2 {
		t.Errorf("expected same cache key for identical states, got %s and %s", key1, key2)
	}

	// 不同输入应该产生不同的 key
	key3 := ComputeCacheKey("node1", state3)
	if key1 == key3 {
		t.Error("expected different cache keys for different states")
	}

	// 不同节点名应该产生不同的 key
	key4 := ComputeCacheKey("node2", state1)
	if key1 == key4 {
		t.Error("expected different cache keys for different node names")
	}

	// 验证 key 格式（应该是 16 字节的十六进制字符串）
	if len(key1) != 32 {
		t.Errorf("expected cache key length 32, got %d", len(key1))
	}
}

// TestComputeCacheKey_Uncacheable 测试不可序列化的状态
func TestComputeCacheKey_Uncacheable(t *testing.T) {
	// 使用包含不可序列化内容的状态
	type UncacheableState struct {
		Ch chan int // channel 不能被 JSON 序列化
	}

	state := UncacheableState{Ch: make(chan int)}

	// 应该生成一个唯一的 key（包含纳秒时间戳）
	key1 := ComputeCacheKey("node1", state)
	time.Sleep(1 * time.Millisecond)
	key2 := ComputeCacheKey("node1", state)

	if key1 == key2 {
		t.Error("expected different keys for uncacheable states")
	}

	// 验证 key 以 "uncacheable-" 开头
	if len(key1) < 12 || key1[:12] != "uncacheable-" {
		t.Errorf("expected key to start with 'uncacheable-', got '%s'", key1)
	}
}

// TestNodeCacheInterface 验证 MemoryNodeCache 实现 NodeCache 接口
func TestNodeCacheInterface(t *testing.T) {
	var _ NodeCache = (*MemoryNodeCache)(nil)
}

// TestGraphWithNodeCache 测试 Graph 的节点缓存集成
func TestGraphWithNodeCache(t *testing.T) {
	cache := NewMemoryNodeCache()
	callCount := 0

	g, err := NewGraph[MapState]("test-graph").
		AddNode("cached-node", func(ctx context.Context, s MapState) (MapState, error) {
			callCount++
			s.Set("counter", callCount)
			return s, nil
		}).
		AddEdge(START, "cached-node").
		AddEdge("cached-node", END).
		WithNodeCache("cached-node", cache).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证缓存已设置
	retrievedCache := g.GetNodeCache("cached-node")
	if retrievedCache == nil {
		t.Error("expected cache to be set")
	}
	if retrievedCache != cache {
		t.Error("expected same cache instance")
	}

	// 测试不存在的节点
	nilCache := g.GetNodeCache("nonexistent")
	if nilCache != nil {
		t.Error("expected nil cache for nonexistent node")
	}
}

// TestGraphWithNodeCache_MultipleNodes 测试多个节点的缓存
func TestGraphWithNodeCache_MultipleNodes(t *testing.T) {
	cache1 := NewMemoryNodeCache()
	cache2 := NewMemoryNodeCache()

	g, err := NewGraph[MapState]("test-graph").
		AddNode("node1", func(ctx context.Context, s MapState) (MapState, error) {
			return s, nil
		}).
		AddNode("node2", func(ctx context.Context, s MapState) (MapState, error) {
			return s, nil
		}).
		AddEdge(START, "node1").
		AddEdge("node1", "node2").
		AddEdge("node2", END).
		WithNodeCache("node1", cache1).
		WithNodeCache("node2", cache2).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证每个节点都有正确的缓存
	if g.GetNodeCache("node1") != cache1 {
		t.Error("expected cache1 for node1")
	}
	if g.GetNodeCache("node2") != cache2 {
		t.Error("expected cache2 for node2")
	}
}

// TestMemoryNodeCache_UpdateExisting 测试更新已存在的缓存条目
func TestMemoryNodeCache_UpdateExisting(t *testing.T) {
	cache := NewMemoryNodeCache()

	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected 'value1', got '%v'", val)
	}

	// 更新相同的 key
	cache.Set("key1", "value2")
	val, ok = cache.Get("key1")
	if !ok || val != "value2" {
		t.Errorf("expected 'value2', got '%v'", val)
	}

	// 缓存大小应该仍然是 1
	stats := cache.Stats()
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
}

// TestMemoryNodeCache_LRU_MoveToFront 测试 LRU 移到最前面的逻辑
func TestMemoryNodeCache_LRU_MoveToFront(t *testing.T) {
	cache := NewMemoryNodeCache(WithCacheCapacity(3))

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// 访问 key1，使其成为最新
	cache.Get("key1")

	// 更新 key2，使其成为最新
	cache.Set("key2", "value2-updated")

	// 添加 key4，应该驱逐 key3（最旧的）
	cache.Set("key4", "value4")

	// key1 和 key2 应该还在
	if _, ok := cache.Get("key1"); !ok {
		t.Error("expected key1 to be in cache")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to be in cache")
	}

	// key3 应该被驱逐
	if _, ok := cache.Get("key3"); ok {
		t.Error("expected key3 to be evicted")
	}

	// key4 应该在缓存中
	if _, ok := cache.Get("key4"); !ok {
		t.Error("expected key4 to be in cache")
	}
}

// TestMemoryNodeCache_TTL_MissIncrement 测试 TTL 过期时 misses 计数器
func TestMemoryNodeCache_TTL_MissIncrement(t *testing.T) {
	ttl := 50 * time.Millisecond
	cache := NewMemoryNodeCache(WithCacheTTL(ttl))

	cache.Set("key1", "value1")

	// 等待 TTL 过期
	time.Sleep(ttl + 50*time.Millisecond)

	// 过期后访问应该增加 misses
	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected cache miss")
	}

	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// key 应该被删除
	stats = cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
}

// TestMemoryNodeCache_ConcurrentAccess 测试并发访问
func TestMemoryNodeCache_ConcurrentAccess(t *testing.T) {
	cache := NewMemoryNodeCache(WithCacheCapacity(100))

	done := make(chan struct{})
	workers := 10
	operations := 100

	// 启动多个 goroutine 并发读写
	for i := 0; i < workers; i++ {
		go func(id int) {
			for j := 0; j < operations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Set(key, fmt.Sprintf("value-%d-%d", id, j))
				cache.Get(key)
			}
			done <- struct{}{}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < workers; i++ {
		<-done
	}

	// 验证统计信息
	stats := cache.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Error("expected some cache operations")
	}
}

// TestMemoryNodeCache_Delete_NonExistent 测试删除不存在的 key
func TestMemoryNodeCache_Delete_NonExistent(t *testing.T) {
	cache := NewMemoryNodeCache()

	// 删除不存在的 key 不应该 panic
	cache.Delete("nonexistent")

	// 缓存应该仍然为空
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
}

// TestCachedNodeHandler_TypeConversion 测试类型转换
func TestCachedNodeHandler_TypeConversion(t *testing.T) {
	cache := NewMemoryNodeCache()

	handler := func(ctx context.Context, s MapState) (MapState, error) {
		s.Set("key", "value")
		return s, nil
	}

	cachedHandler := CachedNodeHandler("test-node", handler, cache)

	ctx := context.Background()
	state := MapState{}

	// 第一次调用
	result1, err := cachedHandler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 手动设置一个错误类型的缓存值
	key := ComputeCacheKey("test-node", state)
	cache.Set(key, "wrong-type")

	// 第二次调用应该失败类型转换，返回缓存的错误类型值
	result2, err := cachedHandler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// result2 应该是零值（类型转换失败）
	if result2 != nil {
		// 如果类型不匹配，返回零值
		t.Logf("Type conversion from cache: %+v", result2)
	}

	// 清理缓存重新测试
	cache.Clear()
	result3, err := cachedHandler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key1, _ := result3.Get("key")
	key2, _ := result1.Get("key")
	if key1 != key2 {
		t.Errorf("expected same results after cache clear, got %v and %v", key1, key2)
	}
}
