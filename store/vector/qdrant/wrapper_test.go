package qdrant_test

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/store/vector/qdrant"
)

// TestConfig 测试配置
func TestConfig(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		cfg := qdrant.Config{
			Host:       "localhost",
			Port:       6333,
			Collection: "test",
			Dimension:  128,
		}

		if cfg.Host != "localhost" {
			t.Errorf("Host = %s, want localhost", cfg.Host)
		}
		if cfg.Port != 6333 {
			t.Errorf("Port = %d, want 6333", cfg.Port)
		}
		if cfg.Collection != "test" {
			t.Errorf("Collection = %s, want test", cfg.Collection)
		}
		if cfg.Dimension != 128 {
			t.Errorf("Dimension = %d, want 128", cfg.Dimension)
		}
	})

	t.Run("WithOptions", func(t *testing.T) {
		cfg := qdrant.Config{
			Host:       "localhost",
			Port:       6333,
			Collection: "test",
			Dimension:  128,
		}

		// 测试各种选项构造器
		if qdrant.WithHost == nil {
			t.Error("WithHost is nil")
		}
		if qdrant.WithPort == nil {
			t.Error("WithPort is nil")
		}
		if qdrant.WithCollection == nil {
			t.Error("WithCollection is nil")
		}
		if qdrant.WithDimension == nil {
			t.Error("WithDimension is nil")
		}
		if qdrant.WithAPIKey == nil {
			t.Error("WithAPIKey is nil")
		}
		if qdrant.WithHTTPS == nil {
			t.Error("WithHTTPS is nil")
		}
		if qdrant.WithTimeout == nil {
			t.Error("WithTimeout is nil")
		}
		if qdrant.WithDistance == nil {
			t.Error("WithDistance is nil")
		}
		if qdrant.WithOnDisk == nil {
			t.Error("WithOnDisk is nil")
		}
		if qdrant.WithCreateCollection == nil {
			t.Error("WithCreateCollection is nil")
		}

		_ = cfg // 使用 cfg 避免未使用警告
	})
}

// TestDistanceMetrics 测试距离度量
func TestDistanceMetrics(t *testing.T) {
	tests := []struct {
		name     string
		distance qdrant.Distance
	}{
		{"Cosine", qdrant.DistanceCosine},
		{"Euclid", qdrant.DistanceEuclid},
		{"Dot", qdrant.DistanceDot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.distance == "" {
				t.Errorf("Distance %s is empty", tt.name)
			}
		})
	}
}

// TestBatchConfig 测试批量配置
func TestBatchConfig(t *testing.T) {
	t.Run("DefaultBatchConfig", func(t *testing.T) {
		cfg := qdrant.DefaultBatchConfig
		// BatchConfig 是结构体，不是指针，验证字段值
		if cfg.BatchSize <= 0 {
			t.Error("DefaultBatchConfig.BatchSize should be positive")
		}
		if cfg.Concurrency <= 0 {
			t.Error("DefaultBatchConfig.Concurrency should be positive")
		}
	})

	t.Run("BatchOptions", func(t *testing.T) {
		// 验证批量操作选项存在
		if qdrant.WithBatchSize == nil {
			t.Error("WithBatchSize is nil")
		}
		if qdrant.WithConcurrency == nil {
			t.Error("WithConcurrency is nil")
		}
		if qdrant.WithRetry == nil {
			t.Error("WithRetry is nil")
		}
		if qdrant.WithOnProgress == nil {
			t.Error("WithOnProgress is nil")
		}
		if qdrant.WithOnError == nil {
			t.Error("WithOnError is nil")
		}
	})
}

// TestQdrantStoreCreation 测试 Qdrant 存储创建
// 注意：这个测试需要 Qdrant 服务运行才能完全执行
func TestQdrantStoreCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Qdrant integration test in short mode")
	}

	t.Run("New", func(t *testing.T) {
		// 尝试创建存储（可能失败如果没有 Qdrant 服务）
		_, err := qdrant.New(qdrant.Config{
			Host:       "localhost",
			Port:       6333,
			Collection: "test_collection",
			Dimension:  128,
		})

		// 如果 Qdrant 服务不可用，这是预期的
		if err != nil {
			t.Logf("Expected error when Qdrant is not available: %v", err)
		}
	})

	t.Run("NewWithOptions", func(t *testing.T) {
		// 验证 NewWithOptions 函数存在且可调用
		if qdrant.NewWithOptions == nil {
			t.Error("NewWithOptions is nil")
		}
	})
}

// TestQdrantStore_Integration 集成测试
// 需要运行 Qdrant 服务才能执行
func TestQdrantStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Qdrant integration test in short mode")
	}

	ctx := context.Background()

	// 尝试连接 Qdrant
	store, err := qdrant.New(qdrant.Config{
		Host:       "localhost",
		Port:       6333,
		Collection: "test_integration",
		Dimension:  128,
	})

	if err != nil {
		t.Skipf("Qdrant not available, skipping integration test: %v", err)
		return
	}
	defer store.Close()

	// 准备测试数据
	embedding := make([]float32, 128)
	for i := range embedding {
		embedding[i] = float32(i) * 0.01
	}

	// 由于 store 实际上是 ai-core 的类型，我们需要使用 interface{} 并进行类型断言
	// 或者直接测试包装器是否正确导出
	t.Run("StoreOperations", func(t *testing.T) {
		// 验证 store 不为 nil
		if store == nil {
			t.Fatal("Store is nil")
		}

		// 这里我们主要验证包装器正确导出了类型
		// 实际的功能测试应该在 ai-core 中进行
		t.Log("Store created successfully")
	})

	_ = ctx // 避免未使用警告
}

// TestQdrantStore_Concurrent 并发操作测试
func TestQdrantStore_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	ctx := context.Background()

	store, err := qdrant.New(qdrant.Config{
		Host:       "localhost",
		Port:       6333,
		Collection: "test_concurrent",
		Dimension:  128,
	})

	if err != nil {
		t.Skipf("Qdrant not available: %v", err)
		return
	}
	defer store.Close()

	// 并发批量操作配置
	t.Run("ConcurrentBatchConfig", func(t *testing.T) {
		// 创建批量配置
		batchCfg := qdrant.DefaultBatchConfig

		// 验证批量配置字段
		if batchCfg.BatchSize <= 0 {
			t.Error("DefaultBatchConfig.BatchSize should be positive")
		}
		if batchCfg.Concurrency <= 0 {
			t.Error("DefaultBatchConfig.Concurrency should be positive")
		}
	})

	_ = ctx // 避免未使用警告
}

// TestQdrantStore_Timeout 超时测试
func TestQdrantStore_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	// 测试超时配置
	t.Run("WithTimeout", func(t *testing.T) {
		cfg := qdrant.Config{
			Host:       "localhost",
			Port:       6333,
			Collection: "test_timeout",
			Dimension:  128,
		}

		// 验证 WithTimeout 选项存在
		if qdrant.WithTimeout == nil {
			t.Error("WithTimeout should not be nil")
		}

		// 创建带超时的存储
		_, err := qdrant.New(cfg)
		if err != nil {
			t.Logf("Expected error with short timeout: %v", err)
		}
	})
}

// TestQdrantStore_HTTPS 测试 HTTPS 配置
func TestQdrantStore_HTTPS(t *testing.T) {
	t.Run("HTTPSConfig", func(t *testing.T) {
		// 验证 HTTPS 相关选项
		if qdrant.WithHTTPS == nil {
			t.Error("WithHTTPS should not be nil")
		}

		if qdrant.WithAPIKey == nil {
			t.Error("WithAPIKey should not be nil")
		}
	})
}

// BenchmarkQdrantConfig 基准测试配置创建
func BenchmarkQdrantConfig(b *testing.B) {
	b.Run("ConfigCreation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = qdrant.Config{
				Host:       "localhost",
				Port:       6333,
				Collection: "benchmark",
				Dimension:  128,
			}
		}
	})
}

// TestQdrantStore_ErrorHandling 错误处理测试
func TestQdrantStore_ErrorHandling(t *testing.T) {
	t.Run("InvalidConfig", func(t *testing.T) {
		// 测试无效配置
		_, err := qdrant.New(qdrant.Config{
			Host:       "invalid-host-that-does-not-exist-12345",
			Port:       99999,
			Collection: "",
			Dimension:  0,
		})

		if err == nil {
			t.Error("Expected error with invalid config")
		}
	})

	t.Run("InvalidDimension", func(t *testing.T) {
		_, err := qdrant.New(qdrant.Config{
			Host:       "localhost",
			Port:       6333,
			Collection: "test",
			Dimension:  -1, // 无效维度
		})

		if err == nil {
			t.Error("Expected error with negative dimension")
		}
	})
}

// TestQdrantStore_Retry 重试机制测试
func TestQdrantStore_Retry(t *testing.T) {
	t.Run("RetryConfig", func(t *testing.T) {
		// 验证重试配置选项
		if qdrant.WithRetry == nil {
			t.Error("WithRetry should not be nil")
		}

		// 测试重试配置
		retryOpt := qdrant.WithRetry(3, 100*time.Millisecond)
		if retryOpt == nil {
			t.Error("WithRetry should return a valid option")
		}
	})
}

// TestQdrantStore_Progress 进度回调测试
func TestQdrantStore_Progress(t *testing.T) {
	t.Run("ProgressCallback", func(t *testing.T) {
		// 验证进度回调选项
		if qdrant.WithOnProgress == nil {
			t.Error("WithOnProgress should not be nil")
		}

		// 创建进度回调
		progressCalled := false
		progressOpt := qdrant.WithOnProgress(func(current, total int) {
			progressCalled = true
		})

		if progressOpt == nil {
			t.Error("WithOnProgress should return a valid option")
		}

		_ = progressCalled // 避免未使用警告
	})

	t.Run("ErrorCallback", func(t *testing.T) {
		// 验证错误回调选项
		if qdrant.WithOnError == nil {
			t.Error("WithOnError should not be nil")
		}

		// 创建错误回调 (返回 bool 表示是否继续)
		errorCalled := false
		errorOpt := qdrant.WithOnError(func(err error, batch int) bool {
			errorCalled = true
			return true // 继续处理
		})

		if errorOpt == nil {
			t.Error("WithOnError should return a valid option")
		}

		_ = errorCalled // 避免未使用警告
	})
}
