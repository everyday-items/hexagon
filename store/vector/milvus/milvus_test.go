package milvus

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/store/vector"
)

// TestNewStore 测试创建存储
func TestNewStore(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		ctx := context.Background()
		store, err := NewStore(ctx)

		if err != nil {
			t.Fatalf("NewStore 错误: %v", err)
		}
		defer store.Close()

		if store.address != "localhost:19530" {
			t.Errorf("address = %s, want localhost:19530", store.address)
		}
		if store.dimension != 1536 {
			t.Errorf("dimension = %d, want 1536", store.dimension)
		}
		if store.indexType != "HNSW" {
			t.Errorf("indexType = %s, want HNSW", store.indexType)
		}
	})

	t.Run("自定义配置", func(t *testing.T) {
		ctx := context.Background()
		store, err := NewStore(ctx,
			WithAddress("custom:19530"),
			WithCollection("my_collection"),
			WithDimension(768),
			WithAuth("user", "pass"),
			WithIndexType("IVF_FLAT"),
			WithMetricType("L2"),
		)

		if err != nil {
			t.Fatalf("NewStore 错误: %v", err)
		}
		defer store.Close()

		if store.address != "custom:19530" {
			t.Errorf("address = %s, want custom:19530", store.address)
		}
		if store.collection != "my_collection" {
			t.Errorf("collection = %s, want my_collection", store.collection)
		}
		if store.dimension != 768 {
			t.Errorf("dimension = %d, want 768", store.dimension)
		}
		if store.username != "user" {
			t.Errorf("username = %s, want user", store.username)
		}
		if store.indexType != "IVF_FLAT" {
			t.Errorf("indexType = %s, want IVF_FLAT", store.indexType)
		}
		if store.metricType != "L2" {
			t.Errorf("metricType = %s, want L2", store.metricType)
		}
	})
}

// TestStoreAdd 测试添加文档
func TestStoreAdd(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	t.Run("添加单个文档", func(t *testing.T) {
		docs := []vector.Document{
			{
				ID:        "doc1",
				Content:   "test content",
				Embedding: []float32{0.1, 0.2, 0.3},
				Metadata: map[string]any{
					"source": "test",
				},
			},
		}

		err := store.Add(ctx, docs)
		if err != nil {
			t.Errorf("Add 错误: %v", err)
		}
	})

	t.Run("添加多个文档", func(t *testing.T) {
		docs := []vector.Document{
			{ID: "doc2", Content: "content 2", Embedding: []float32{0.4, 0.5, 0.6}},
			{ID: "doc3", Content: "content 3", Embedding: []float32{0.7, 0.8, 0.9}},
		}

		err := store.Add(ctx, docs)
		if err != nil {
			t.Errorf("Add 错误: %v", err)
		}
	})

	t.Run("空文档列表", func(t *testing.T) {
		err := store.Add(ctx, []vector.Document{})
		if err != nil {
			t.Errorf("空列表不应报错: %v", err)
		}
	})

	t.Run("无 ID 文档", func(t *testing.T) {
		docs := []vector.Document{
			{Content: "auto id content", Embedding: []float32{0.1, 0.2, 0.3}},
		}

		err := store.Add(ctx, docs)
		if err != nil {
			t.Errorf("Add 错误: %v", err)
		}
	})

	t.Run("维度不匹配", func(t *testing.T) {
		docs := []vector.Document{
			{ID: "bad", Content: "wrong dim", Embedding: []float32{0.1, 0.2}}, // 只有 2 维
		}

		err := store.Add(ctx, docs)
		if err == nil {
			t.Error("维度不匹配应该报错")
		}
	})
}

// TestStoreSearch 测试搜索
func TestStoreSearch(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	// 添加测试数据
	docs := []vector.Document{
		{ID: "doc1", Content: "content 1", Embedding: []float32{1.0, 0.0, 0.0}},
		{ID: "doc2", Content: "content 2", Embedding: []float32{0.0, 1.0, 0.0}},
		{ID: "doc3", Content: "content 3", Embedding: []float32{0.0, 0.0, 1.0}},
		{ID: "doc4", Content: "content 4", Embedding: []float32{0.5, 0.5, 0.0}},
	}
	if err := store.Add(ctx, docs); err != nil {
		t.Fatalf("添加测试数据错误: %v", err)
	}

	t.Run("基本搜索", func(t *testing.T) {
		embedding := []float32{1.0, 0.0, 0.0}
		results, err := store.Search(ctx, embedding, 2)

		if err != nil {
			t.Errorf("Search 错误: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("结果数量 = %d, want 2", len(results))
		}

		// 第一个结果应该是 doc1（完全匹配）
		if results[0].ID != "doc1" {
			t.Errorf("最相似文档 = %s, want doc1", results[0].ID)
		}

		// 验证分数降序
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("结果未按分数降序排列")
			}
		}
	})

	t.Run("带最小分数过滤", func(t *testing.T) {
		embedding := []float32{1.0, 0.0, 0.0}
		results, err := store.Search(ctx, embedding, 10,
			vector.WithMinScore(0.9),
		)

		if err != nil {
			t.Errorf("Search 错误: %v", err)
		}

		// 只有 doc1 应该满足 0.9+ 分数
		if len(results) != 1 {
			t.Errorf("结果数量 = %d, want 1", len(results))
		}
	})

	t.Run("不包含嵌入向量", func(t *testing.T) {
		embedding := []float32{1.0, 0.0, 0.0}
		results, err := store.Search(ctx, embedding, 1)

		if err != nil {
			t.Errorf("Search 错误: %v", err)
		}

		if len(results) > 0 && results[0].Embedding != nil {
			t.Error("默认不应包含嵌入向量")
		}
	})

	t.Run("包含嵌入向量", func(t *testing.T) {
		embedding := []float32{1.0, 0.0, 0.0}
		results, err := store.Search(ctx, embedding, 1,
			vector.WithEmbedding(true),
		)

		if err != nil {
			t.Errorf("Search 错误: %v", err)
		}

		if len(results) > 0 && results[0].Embedding == nil {
			t.Error("应该包含嵌入向量")
		}
	})

	t.Run("维度不匹配", func(t *testing.T) {
		embedding := []float32{1.0, 0.0} // 只有 2 维
		_, err := store.Search(ctx, embedding, 2)

		if err == nil {
			t.Error("维度不匹配应该报错")
		}
	})
}

// TestStoreGet 测试获取文档
func TestStoreGet(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	// 添加测试数据
	docs := []vector.Document{
		{ID: "doc1", Content: "test content", Embedding: []float32{0.1, 0.2, 0.3}},
	}
	if err := store.Add(ctx, docs); err != nil {
		t.Fatalf("添加测试数据错误: %v", err)
	}

	t.Run("获取存在的文档", func(t *testing.T) {
		doc, err := store.Get(ctx, "doc1")
		if err != nil {
			t.Errorf("Get 错误: %v", err)
		}

		if doc == nil {
			t.Fatal("doc 不应为 nil")
		}

		if doc.ID != "doc1" {
			t.Errorf("ID = %s, want doc1", doc.ID)
		}
		if doc.Content != "test content" {
			t.Errorf("Content = %s, want test content", doc.Content)
		}
	})

	t.Run("获取不存在的文档", func(t *testing.T) {
		_, err := store.Get(ctx, "nonexistent")
		if err == nil {
			t.Error("获取不存在的文档应该报错")
		}
	})
}

// TestStoreDelete 测试删除文档
func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	// 添加测试数据
	docs := []vector.Document{
		{ID: "doc1", Content: "content 1", Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "doc2", Content: "content 2", Embedding: []float32{0.4, 0.5, 0.6}},
	}
	if err := store.Add(ctx, docs); err != nil {
		t.Fatalf("添加测试数据错误: %v", err)
	}

	t.Run("删除单个文档", func(t *testing.T) {
		err := store.Delete(ctx, []string{"doc1"})
		if err != nil {
			t.Errorf("Delete 错误: %v", err)
		}

		// 验证已删除
		_, err = store.Get(ctx, "doc1")
		if err == nil {
			t.Error("已删除的文档不应该能获取")
		}
	})

	t.Run("删除多个文档", func(t *testing.T) {
		// 先添加更多数据
		store.Add(ctx, []vector.Document{
			{ID: "doc3", Content: "content 3", Embedding: []float32{0.1, 0.2, 0.3}},
			{ID: "doc4", Content: "content 4", Embedding: []float32{0.4, 0.5, 0.6}},
		})

		err := store.Delete(ctx, []string{"doc3", "doc4"})
		if err != nil {
			t.Errorf("Delete 错误: %v", err)
		}
	})
}

// TestStoreUpdate 测试更新文档
func TestStoreUpdate(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	// 添加测试数据
	docs := []vector.Document{
		{ID: "doc1", Content: "original content", Embedding: []float32{0.1, 0.2, 0.3}},
	}
	if err := store.Add(ctx, docs); err != nil {
		t.Fatalf("添加测试数据错误: %v", err)
	}

	t.Run("更新文档", func(t *testing.T) {
		updates := []vector.Document{
			{ID: "doc1", Content: "updated content", Embedding: []float32{0.9, 0.8, 0.7}},
		}

		err := store.Update(ctx, updates)
		if err != nil {
			t.Errorf("Update 错误: %v", err)
		}

		// 验证更新
		doc, err := store.Get(ctx, "doc1")
		if err != nil {
			t.Errorf("Get 错误: %v", err)
		}

		if doc.Content != "updated content" {
			t.Errorf("Content = %s, want updated content", doc.Content)
		}
	})
}

// TestStoreCount 测试统计数量
func TestStoreCount(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	t.Run("空存储", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Errorf("Count 错误: %v", err)
		}

		if count != 0 {
			t.Errorf("count = %d, want 0", count)
		}
	})

	t.Run("有数据", func(t *testing.T) {
		docs := []vector.Document{
			{ID: "doc1", Content: "content 1", Embedding: []float32{0.1, 0.2, 0.3}},
			{ID: "doc2", Content: "content 2", Embedding: []float32{0.4, 0.5, 0.6}},
		}
		store.Add(ctx, docs)

		count, err := store.Count(ctx)
		if err != nil {
			t.Errorf("Count 错误: %v", err)
		}

		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}
	})
}

// TestStoreClear 测试清空
func TestStoreClear(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	// 添加测试数据
	docs := []vector.Document{
		{ID: "doc1", Content: "content 1", Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "doc2", Content: "content 2", Embedding: []float32{0.4, 0.5, 0.6}},
	}
	store.Add(ctx, docs)

	t.Run("清空存储", func(t *testing.T) {
		err := store.Clear(ctx)
		if err != nil {
			t.Errorf("Clear 错误: %v", err)
		}

		count, _ := store.Count(ctx)
		if count != 0 {
			t.Errorf("清空后 count = %d, want 0", count)
		}
	})
}

// TestStoreStats 测试统计信息
func TestStoreStats(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx,
		WithCollection("stats_test"),
		WithDimension(512),
		WithIndexType("HNSW"),
		WithMetricType("COSINE"),
	)
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	t.Run("获取统计信息", func(t *testing.T) {
		stats, err := store.Stats(ctx)
		if err != nil {
			t.Errorf("Stats 错误: %v", err)
		}

		if stats["collection"] != "stats_test" {
			t.Errorf("collection = %v, want stats_test", stats["collection"])
		}
		if stats["dimension"] != 512 {
			t.Errorf("dimension = %v, want 512", stats["dimension"])
		}
		if stats["index_type"] != "HNSW" {
			t.Errorf("index_type = %v, want HNSW", stats["index_type"])
		}
		if stats["connected"] != true {
			t.Error("connected 应为 true")
		}
	})
}

// TestStoreClose 测试关闭连接
func TestStoreClose(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx)
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}

	t.Run("关闭连接", func(t *testing.T) {
		err := store.Close()
		if err != nil {
			t.Errorf("Close 错误: %v", err)
		}

		// 再次关闭不应报错
		err = store.Close()
		if err != nil {
			t.Errorf("重复 Close 错误: %v", err)
		}
	})

	t.Run("关闭后操作应失败", func(t *testing.T) {
		_, err := store.Get(ctx, "doc1")
		if err == nil {
			t.Error("关闭后 Get 应该返回错误")
		}

		_, err = store.Search(ctx, []float32{0.1, 0.2, 0.3}, 1)
		if err == nil {
			t.Error("关闭后 Search 应该返回错误")
		}
	})
}

// TestStoreIndexOperations 测试索引操作
func TestStoreIndexOperations(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx)
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	t.Run("创建索引", func(t *testing.T) {
		err := store.CreateIndex(ctx)
		if err != nil {
			t.Errorf("CreateIndex 错误: %v", err)
		}
	})

	t.Run("删除索引", func(t *testing.T) {
		err := store.DropIndex(ctx)
		if err != nil {
			t.Errorf("DropIndex 错误: %v", err)
		}
	})
}

// TestStoreCollectionOperations 测试集合操作
func TestStoreCollectionOperations(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx)
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	t.Run("加载集合", func(t *testing.T) {
		err := store.LoadCollection(ctx)
		if err != nil {
			t.Errorf("LoadCollection 错误: %v", err)
		}
	})

	t.Run("释放集合", func(t *testing.T) {
		err := store.ReleaseCollection(ctx)
		if err != nil {
			t.Errorf("ReleaseCollection 错误: %v", err)
		}
	})

	t.Run("刷新数据", func(t *testing.T) {
		err := store.Flush(ctx)
		if err != nil {
			t.Errorf("Flush 错误: %v", err)
		}
	})
}

// TestCosineSimilarity 测试余弦相似度计算
func TestCosineSimilarity(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(ctx, WithDimension(3))
	if err != nil {
		t.Fatalf("NewStore 错误: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "完全相同",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "完全正交",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "不同长度",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0, // 长度不同返回 0
		},
		{
			name:     "零向量",
			a:        []float32{0.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0, // 包含零向量返回 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.cosineSimilarity(tt.a, tt.b)
			// 由于浮点数精度，允许小误差
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestGenerateID 测试 ID 生成
func TestGenerateID(t *testing.T) {
	t.Run("生成唯一 ID", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := generateID()
			if ids[id] {
				t.Errorf("生成了重复 ID: %s", id)
			}
			ids[id] = true
		}
	})

	t.Run("ID 长度", func(t *testing.T) {
		id := generateID()
		if len(id) < 10 {
			t.Errorf("ID 太短: %s", id)
		}
	})
}
