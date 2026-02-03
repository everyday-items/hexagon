package vector_test

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/store/vector"
)

// TestMemoryStore 测试内存向量存储
func TestMemoryStore(t *testing.T) {
	ctx := context.Background()

	// 创建存储
	store := vector.NewMemoryStore(128)
	if store == nil {
		t.Fatal("NewMemoryStore() returned nil")
	}
	defer store.Close()

	// 准备测试数据
	embedding := make([]float32, 128)
	for i := range embedding {
		embedding[i] = float32(i) * 0.01
	}

	docs := []vector.Document{
		{
			ID:        "doc1",
			Content:   "Hello World",
			Embedding: embedding,
			Metadata: map[string]interface{}{
				"category": "greeting",
			},
		},
		{
			ID:        "doc2",
			Content:   "Goodbye World",
			Embedding: embedding,
			Metadata: map[string]interface{}{
				"category": "farewell",
			},
		},
	}

	// 测试添加文档
	t.Run("Add", func(t *testing.T) {
		err := store.Add(ctx, docs)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	})

	// 测试搜索
	t.Run("Search", func(t *testing.T) {
		results, err := store.Search(ctx, embedding, 10)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Search() returned no results")
		}

		if len(results) > 2 {
			t.Errorf("Search() returned %d results, want <= 2", len(results))
		}
	})

	// 测试带过滤条件的搜索
	t.Run("SearchWithFilter", func(t *testing.T) {
		results, err := store.Search(
			ctx,
			embedding,
			10,
			vector.WithFilter(map[string]interface{}{
				"category": "greeting",
			}),
		)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Search() with filter returned no results")
		}
	})

	// 测试获取文档
	t.Run("Get", func(t *testing.T) {
		doc, err := store.Get(ctx, "doc1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if doc == nil {
			t.Fatal("Get() returned nil document")
		}

		if doc.ID != "doc1" {
			t.Errorf("Get() ID = %s, want doc1", doc.ID)
		}

		if doc.Content != "Hello World" {
			t.Errorf("Get() Content = %s, want Hello World", doc.Content)
		}
	})

	// 测试删除文档
	t.Run("Delete", func(t *testing.T) {
		err := store.Delete(ctx, []string{"doc1"})
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// 验证已删除 - Get 可能返回 nil 而不是错误
		doc, _ := store.Get(ctx, "doc1")
		if doc != nil {
			t.Error("Get() after Delete should return nil")
		}
	})

	// 测试批量操作
	t.Run("BatchOperations", func(t *testing.T) {
		// 批量添加
		batchDocs := make([]vector.Document, 10)
		for i := range batchDocs {
			batchDocs[i] = vector.Document{
				ID:        "batch_" + string(rune('0'+i)),
				Content:   "Batch document",
				Embedding: embedding,
			}
		}

		err := store.Add(ctx, batchDocs)
		if err != nil {
			t.Fatalf("Batch Add() error = %v", err)
		}

		// 批量搜索
		results, err := store.Search(ctx, embedding, 20)
		if err != nil {
			t.Fatalf("Search() after batch add error = %v", err)
		}

		if len(results) < 10 {
			t.Errorf("Search() returned %d results, want >= 10", len(results))
		}
	})

	// 测试清空
	t.Run("Clear", func(t *testing.T) {
		err := store.Clear(ctx)
		if err != nil {
			t.Fatalf("Clear() error = %v", err)
		}

		// 验证已清空
		results, err := store.Search(ctx, embedding, 10)
		if err != nil {
			t.Fatalf("Search() after Clear error = %v", err)
		}

		if len(results) != 0 {
			t.Errorf("Search() after Clear returned %d results, want 0", len(results))
		}
	})
}

// TestSearchOptions 测试搜索选项
func TestSearchOptions(t *testing.T) {
	ctx := context.Background()
	store := vector.NewMemoryStore(128)
	defer store.Close()

	embedding := make([]float32, 128)
	for i := range embedding {
		embedding[i] = 0.5
	}

	docs := []vector.Document{
		{
			ID:        "opt1",
			Content:   "Option 1",
			Embedding: embedding,
			Metadata:  map[string]interface{}{"score": 0.9},
		},
		{
			ID:        "opt2",
			Content:   "Option 2",
			Embedding: embedding,
			Metadata:  map[string]interface{}{"score": 0.5},
		},
	}

	err := store.Add(ctx, docs)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 测试 MinScore 选项
	t.Run("WithMinScore", func(t *testing.T) {
		results, err := store.Search(
			ctx,
			embedding,
			10,
			vector.WithMinScore(0.8),
		)
		if err != nil {
			t.Fatalf("Search() with MinScore error = %v", err)
		}

		for _, result := range results {
			if result.Score < 0.8 {
				t.Errorf("Result score %f < 0.8", result.Score)
			}
		}
	})

	// 测试 Embedding 选项
	t.Run("WithEmbedding", func(t *testing.T) {
		results, err := store.Search(
			ctx,
			embedding,
			10,
			vector.WithEmbedding(true),
		)
		if err != nil {
			t.Fatalf("Search() with Embedding error = %v", err)
		}

		for _, result := range results {
			if len(result.Embedding) == 0 {
				t.Error("Result should include embedding")
			}
		}
	})

	// 测试 Metadata 选项
	t.Run("WithMetadata", func(t *testing.T) {
		results, err := store.Search(
			ctx,
			embedding,
			10,
			vector.WithMetadata(true),
		)
		if err != nil {
			t.Fatalf("Search() with Metadata error = %v", err)
		}

		for _, result := range results {
			if len(result.Metadata) == 0 {
				t.Error("Result should include metadata")
			}
		}
	})
}

// TestEmbedderFunc 测试函数式 Embedder
func TestEmbedderFunc(t *testing.T) {
	ctx := context.Background()

	// 创建简单的 embedder
	embedder := vector.NewEmbedderFunc(128, func(ctx context.Context, texts []string) ([][]float32, error) {
		results := make([][]float32, len(texts))
		for i := range texts {
			// 简单的伪向量生成
			embedding := make([]float32, 128)
			for j := range embedding {
				embedding[j] = float32(len(texts[i])) * 0.01
			}
			results[i] = embedding
		}
		return results, nil
	})

	// 测试嵌入
	texts := []string{"Hello", "World", "Test"}
	embeddings, err := embedder.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(embeddings) != len(texts) {
		t.Errorf("Embed() returned %d embeddings, want %d", len(embeddings), len(texts))
	}

	for i, emb := range embeddings {
		if len(emb) != 128 {
			t.Errorf("Embedding[%d] length = %d, want 128", i, len(emb))
		}
	}
}

// TestStoreLifecycle 测试存储生命周期
func TestStoreLifecycle(t *testing.T) {
	// 创建和关闭存储
	store := vector.NewMemoryStore(128)
	if store == nil {
		t.Fatal("NewMemoryStore() returned nil")
	}

	// 关闭应该成功
	err := store.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 重复关闭不应该 panic
	err = store.Close()
	if err != nil {
		// 第二次关闭可能返回错误，这是可以接受的
		t.Logf("Second Close() returned error: %v (acceptable)", err)
	}
}

// BenchmarkMemoryStore 基准测试
func BenchmarkMemoryStore(b *testing.B) {
	ctx := context.Background()
	store := vector.NewMemoryStore(128)
	defer store.Close()

	embedding := make([]float32, 128)
	for i := range embedding {
		embedding[i] = 0.5
	}

	// 预填充一些数据
	docs := make([]vector.Document, 1000)
	for i := range docs {
		docs[i] = vector.Document{
			ID:        "doc_" + string(rune(i)),
			Embedding: embedding,
		}
	}
	store.Add(ctx, docs)

	b.Run("Search", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = store.Search(ctx, embedding, 10)
		}
	})

	b.Run("Add", func(b *testing.B) {
		doc := vector.Document{
			ID:        "benchmark",
			Embedding: embedding,
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = store.Add(ctx, []vector.Document{doc})
		}
	})

	b.Run("Get", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = store.Get(ctx, "doc_0")
		}
	})
}
