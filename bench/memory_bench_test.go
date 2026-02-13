package bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	memstore "github.com/everyday-items/hexagon/memory/store"
)

// BenchmarkMemoryStoreCreation 测试内存存储创建性能
func BenchmarkMemoryStoreCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = memstore.NewInMemoryStore()
	}
}

// BenchmarkMemoryStorePut 测试写入性能
func BenchmarkMemoryStorePut(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Put(ctx, []string{"bench", "users"}, fmt.Sprintf("key-%d", i), map[string]any{
			"name":  "test",
			"value": i,
		})
	}
}

// BenchmarkMemoryStoreGet 测试读取性能
func BenchmarkMemoryStoreGet(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	// 预填充数据
	for i := 0; i < 1000; i++ {
		store.Put(ctx, []string{"bench"}, fmt.Sprintf("key-%d", i), map[string]any{"value": i})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Get(ctx, []string{"bench"}, fmt.Sprintf("key-%d", i%1000))
	}
}

// BenchmarkMemoryStoreSearch 测试搜索性能
func BenchmarkMemoryStoreSearch(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	// 预填充数据
	for i := 0; i < 100; i++ {
		store.Put(ctx, []string{"bench", "search"}, fmt.Sprintf("key-%d", i), map[string]any{
			"value": i,
			"text":  fmt.Sprintf("document %d", i),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Search(ctx, []string{"bench", "search"}, &memstore.SearchQuery{
			Limit: 10,
		})
	}
}

// BenchmarkMemoryStorePutWithTTL 测试带 TTL 写入性能
func BenchmarkMemoryStorePutWithTTL(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Put(ctx, []string{"bench"}, fmt.Sprintf("ttl-%d", i), map[string]any{
			"value": i,
		}, memstore.WithTTL(1*time.Hour))
	}
}

// BenchmarkMemoryStoreDelete 测试删除性能
func BenchmarkMemoryStoreDelete(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	// 预填充数据
	for i := 0; i < b.N; i++ {
		store.Put(ctx, []string{"bench"}, fmt.Sprintf("del-%d", i), map[string]any{"value": i})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Delete(ctx, []string{"bench"}, fmt.Sprintf("del-%d", i))
	}
}

// BenchmarkMemoryStoreConcurrentPut 测试并发写入性能
func BenchmarkMemoryStoreConcurrentPut(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			store.Put(ctx, []string{"bench"}, fmt.Sprintf("key-%d", i), map[string]any{"value": i})
			i++
		}
	})
}

// BenchmarkMemoryStoreConcurrentGet 测试并发读取性能
func BenchmarkMemoryStoreConcurrentGet(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	// 预填充数据
	for i := 0; i < 1000; i++ {
		store.Put(ctx, []string{"bench"}, fmt.Sprintf("key-%d", i), map[string]any{"value": i})
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			store.Get(ctx, []string{"bench"}, fmt.Sprintf("key-%d", i%1000))
			i++
		}
	})
}

// BenchmarkMemoryStoreList 测试列表查询性能
func BenchmarkMemoryStoreList(b *testing.B) {
	store := memstore.NewInMemoryStore()
	ctx := context.Background()

	// 预填充数据
	for i := 0; i < 100; i++ {
		store.Put(ctx, []string{"bench", "list"}, fmt.Sprintf("key-%d", i), map[string]any{"value": i})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.List(ctx, []string{"bench", "list"}, memstore.WithListLimit(20))
	}
}
