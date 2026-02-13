// Package main 演示 Hexagon 跨会话持久记忆
//
// 记忆存储系统支持：
//   - 内存存储: 基本 CRUD 操作
//   - TTL 过期: 数据自动过期，适用于缓存
//   - 命名空间隔离: 不同命名空间数据互不干扰
//
// 运行方式:
//
//	go run ./examples/memory/
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	memstore "github.com/everyday-items/hexagon/memory/store"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: 内存存储基本操作 ===")
	runBasicStore(ctx)

	fmt.Println("\n=== 示例 2: 带 TTL 的存储 ===")
	runTTLStore(ctx)

	fmt.Println("\n=== 示例 3: 命名空间隔离 ===")
	runNamespaceStore(ctx)
}

// runBasicStore 演示 Put/Get/Search/Delete 基本操作
func runBasicStore(ctx context.Context) {
	store := memstore.NewInMemoryStore()

	// 保存
	err := store.Put(ctx, []string{"users", "u1"}, "profile", map[string]any{
		"name": "Alice", "email": "alice@example.com", "role": "admin",
	})
	if err != nil {
		log.Fatalf("保存失败: %v", err)
	}
	fmt.Println("  已保存用户 profile")

	// 获取
	item, err := store.Get(ctx, []string{"users", "u1"}, "profile")
	if err != nil {
		log.Fatalf("获取失败: %v", err)
	}
	if item != nil {
		fmt.Printf("  获取用户: name=%s, role=%s\n", item.Value["name"], item.Value["role"])
	}

	// 搜索
	results, _ := store.Search(ctx, []string{"users"}, &memstore.SearchQuery{Limit: 10})
	fmt.Printf("  搜索到 %d 条记录\n", len(results))

	// 删除
	store.Delete(ctx, []string{"users", "u1"}, "profile")
	fmt.Println("  已删除用户 profile")
}

// runTTLStore 演示数据自动过期
func runTTLStore(ctx context.Context) {
	store := memstore.NewInMemoryStore()

	store.Put(ctx, []string{"cache"}, "temp", map[string]any{
		"message": "这条数据会在 1 秒后过期",
	}, memstore.WithTTL(1*time.Second))

	item, _ := store.Get(ctx, []string{"cache"}, "temp")
	if item != nil {
		fmt.Printf("  立即获取: %s\n", item.Value["message"])
	}

	fmt.Println("  等待过期...")
	time.Sleep(1100 * time.Millisecond)

	item, _ = store.Get(ctx, []string{"cache"}, "temp")
	if item == nil {
		fmt.Println("  数据已过期 (nil)")
	}
}

// runNamespaceStore 演示命名空间隔离
func runNamespaceStore(ctx context.Context) {
	store := memstore.NewInMemoryStore()

	store.Put(ctx, []string{"app", "settings"}, "theme", map[string]any{"value": "dark"})
	store.Put(ctx, []string{"app", "settings"}, "lang", map[string]any{"value": "zh-CN"})
	store.Put(ctx, []string{"app", "users"}, "count", map[string]any{"value": 42})

	// settings 命名空间
	results, _ := store.Search(ctx, []string{"app", "settings"}, &memstore.SearchQuery{Limit: 10})
	fmt.Printf("  settings: %d 条记录\n", len(results))
	for _, r := range results {
		fmt.Printf("    %s = %v\n", r.Item.Key, r.Item.Value["value"])
	}

	// users 命名空间
	results, _ = store.Search(ctx, []string{"app", "users"}, &memstore.SearchQuery{Limit: 10})
	fmt.Printf("  users: %d 条记录\n", len(results))
}
