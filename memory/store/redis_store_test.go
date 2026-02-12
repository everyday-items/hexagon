package store

import (
	"context"
	"errors"
	"testing"
)

func TestRedisStore_NilClientValidation(t *testing.T) {
	s := NewRedisStore(nil)
	ctx := context.Background()

	if err := s.Put(ctx, []string{"test"}, "k1", map[string]any{"v": 1}); err == nil {
		t.Fatal("Put: nil client 应返回错误")
	}
	if _, err := s.Get(ctx, []string{"test"}, "k1"); err == nil {
		t.Fatal("Get: nil client 应返回错误")
	}
	if _, err := s.Search(ctx, []string{"test"}, &SearchQuery{Limit: 10}); err == nil {
		t.Fatal("Search: nil client 应返回错误")
	}
	if err := s.Delete(ctx, []string{"test"}, "k1"); err == nil {
		t.Fatal("Delete: nil client 应返回错误")
	}
	if _, err := s.List(ctx, []string{"test"}); err == nil {
		t.Fatal("List: nil client 应返回错误")
	}
	if err := s.DeleteNamespace(ctx, []string{"test"}); err == nil {
		t.Fatal("DeleteNamespace: nil client 应返回错误")
	}
}

func TestRedisStore_ContextCanceled(t *testing.T) {
	s := NewRedisStore(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Put(ctx, []string{"test"}, "k1", map[string]any{"v": 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Put: 期望 context.Canceled, 实际 %v", err)
	}

	_, err = s.Get(ctx, []string{"test"}, "k1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get: 期望 context.Canceled, 实际 %v", err)
	}
}

func TestRedisStore_EmptyKeyValidation(t *testing.T) {
	s := NewRedisStore(nil)
	ctx := context.Background()

	err := s.Put(ctx, []string{"test"}, "", map[string]any{"v": 1})
	if err == nil {
		t.Fatal("Put 空 key 应返回错误")
	}
}

func TestRedisStore_Options(t *testing.T) {
	s := NewRedisStore(nil,
		WithRedisPrefix("custom:"),
		WithDefaultTTL(24*60*60*1e9), // 24h in nanoseconds
	)

	if s.prefix != "custom:" {
		t.Errorf("prefix = %q, 期望 %q", s.prefix, "custom:")
	}
	if s.defaultTTL != 24*60*60*1e9 {
		t.Errorf("defaultTTL = %v, 期望 24h", s.defaultTTL)
	}
}

func TestRedisStore_KeyFormat(t *testing.T) {
	s := NewRedisStore(nil, WithRedisPrefix("hexagon:mem:"))

	// 数据键
	dataKey := s.dataKey([]string{"users", "u1"}, "prefs")
	expected := "hexagon:mem:users:u1:prefs"
	if dataKey != expected {
		t.Errorf("dataKey = %q, 期望 %q", dataKey, expected)
	}

	// 命名空间索引键
	nsKey := s.nsIndexKey([]string{"users", "u1"})
	expectedNs := "hexagon:mem:ns:users:u1"
	if nsKey != expectedNs {
		t.Errorf("nsIndexKey = %q, 期望 %q", nsKey, expectedNs)
	}
}

func TestRedisStore_Close(t *testing.T) {
	s := NewRedisStore(nil)
	if err := s.Close(); err != nil {
		t.Fatalf("Close 不应报错: %v", err)
	}
}

func TestRedisStore_SearchNilQuery(t *testing.T) {
	s := NewRedisStore(nil)
	ctx := context.Background()

	// nil query 不应报错（ctx 检查在 nil client 检查前）
	// 先测试非 nil client 但 nil query 的情况不会 panic
	// 实际上 nil client 会先返回错误，这里验证 context 取消优先
	ctx2, cancel := context.WithCancel(ctx)
	cancel()

	_, err := s.Search(ctx2, []string{"test"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search: 已取消 context 应优先返回 context.Canceled, 实际 %v", err)
	}
}
