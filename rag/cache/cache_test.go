package cache

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

// mockEmbedder 模拟嵌入器
// 简单地将文本转换为固定维度的向量，相同文本返回相同向量
type mockEmbedder struct {
	mu         sync.Mutex
	callCount  int
	embeddings map[string][]float64
	err        error
}

func newMockEmbedder() *mockEmbedder {
	return &mockEmbedder{
		embeddings: make(map[string][]float64),
	}
}

// setEmbedding 预设文本对应的向量
func (e *mockEmbedder) setEmbedding(text string, emb []float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.embeddings[text] = emb
}

func (e *mockEmbedder) Embed(_ context.Context, text string) ([]float64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callCount++

	if e.err != nil {
		return nil, e.err
	}

	if emb, ok := e.embeddings[text]; ok {
		return emb, nil
	}

	// 默认返回基于文本长度的简单向量
	return []float64{float64(len(text)), 0.5, 0.3}, nil
}

// ============== 基础创建测试 ==============

func TestNew(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder)

	if sc == nil {
		t.Fatal("New 返回 nil")
	}

	if sc.maxSize != 1000 {
		t.Errorf("默认 maxSize 应为 1000，实际 %d", sc.maxSize)
	}

	if sc.ttl != time.Hour {
		t.Errorf("默认 ttl 应为 1 小时，实际 %v", sc.ttl)
	}

	if sc.threshold != 0.85 {
		t.Errorf("默认 threshold 应为 0.85，实际 %f", sc.threshold)
	}
}

func TestNew_WithOptions(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder,
		WithMaxSize(500),
		WithTTL(30*time.Minute),
		WithThreshold(0.9),
	)

	if sc.maxSize != 500 {
		t.Errorf("maxSize 应为 500，实际 %d", sc.maxSize)
	}

	if sc.ttl != 30*time.Minute {
		t.Errorf("ttl 应为 30 分钟，实际 %v", sc.ttl)
	}

	if sc.threshold != 0.9 {
		t.Errorf("threshold 应为 0.9，实际 %f", sc.threshold)
	}
}

func TestNew_InvalidOptions(t *testing.T) {
	embedder := newMockEmbedder()

	// 无效参数不应改变默认值
	sc := New(embedder,
		WithMaxSize(-1),
		WithTTL(-time.Second),
		WithThreshold(1.5),
	)

	if sc.maxSize != 1000 {
		t.Errorf("无效 maxSize 应保持默认值 1000，实际 %d", sc.maxSize)
	}

	if sc.ttl != time.Hour {
		t.Errorf("无效 ttl 应保持默认值 1 小时，实际 %v", sc.ttl)
	}

	if sc.threshold != 0.85 {
		t.Errorf("无效 threshold 应保持默认值 0.85，实际 %f", sc.threshold)
	}
}

// ============== Get/Put 测试 ==============

func TestPutAndGet_ExactMatch(t *testing.T) {
	embedder := newMockEmbedder()
	// 为查询文本预设相同的向量（模拟完全相同的语义）
	embedder.setEmbedding("什么是 Go", []float64{1.0, 0.0, 0.0})

	sc := New(embedder, WithThreshold(0.85))

	ctx := context.Background()
	result := &CacheResult{
		Answer:    "Go 是一种编程语言",
		Documents: []CachedDocument{{Content: "Go lang doc", Score: 0.95}},
	}

	// 写入缓存
	err := sc.Put(ctx, "什么是 Go", result)
	if err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	// 查找缓存（相同查询，相同向量，相似度=1.0）
	got, hit, err := sc.Get(ctx, "什么是 Go")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !hit {
		t.Fatal("期望命中缓存")
	}
	if got.Answer != "Go 是一种编程语言" {
		t.Errorf("答案不匹配，got: %s", got.Answer)
	}
}

func TestGet_SemanticMatch(t *testing.T) {
	embedder := newMockEmbedder()
	// 设置语义相近的向量
	embedder.setEmbedding("什么是 Go 语言", []float64{0.9, 0.1, 0.0})
	embedder.setEmbedding("Go 语言是什么", []float64{0.88, 0.12, 0.01})

	sc := New(embedder, WithThreshold(0.95))

	ctx := context.Background()
	result := &CacheResult{Answer: "Go 是编程语言"}

	err := sc.Put(ctx, "什么是 Go 语言", result)
	if err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	// 语义相近的查询应该命中
	got, hit, err := sc.Get(ctx, "Go 语言是什么")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}

	// 计算两个向量的余弦相似度
	sim := cosineSimilarity([]float64{0.9, 0.1, 0.0}, []float64{0.88, 0.12, 0.01})
	if sim >= 0.95 {
		if !hit {
			t.Error("相似度超过阈值，应该命中")
		}
		if got.Answer != "Go 是编程语言" {
			t.Errorf("答案不匹配: %s", got.Answer)
		}
	} else {
		if hit {
			t.Error("相似度低于阈值，不应命中")
		}
	}
}

func TestGet_Miss(t *testing.T) {
	embedder := newMockEmbedder()
	// 设置语义差异较大的向量
	embedder.setEmbedding("什么是 Go", []float64{1.0, 0.0, 0.0})
	embedder.setEmbedding("天气怎么样", []float64{0.0, 0.0, 1.0})

	sc := New(embedder, WithThreshold(0.85))

	ctx := context.Background()
	_ = sc.Put(ctx, "什么是 Go", &CacheResult{Answer: "Go 是编程语言"})

	// 不相关的查询不应命中
	_, hit, err := sc.Get(ctx, "天气怎么样")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if hit {
		t.Error("不相关查询不应命中缓存")
	}
}

func TestGet_EmptyCache(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder)

	ctx := context.Background()
	_, hit, err := sc.Get(ctx, "任意查询")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if hit {
		t.Error("空缓存不应命中")
	}
}

func TestGet_EmbedderError(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.err = fmt.Errorf("嵌入服务不可用")
	sc := New(embedder)

	ctx := context.Background()
	_, _, err := sc.Get(ctx, "测试")
	if err == nil {
		t.Error("嵌入器报错时 Get 应返回错误")
	}
}

func TestPut_EmbedderError(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.err = fmt.Errorf("嵌入服务不可用")
	sc := New(embedder)

	ctx := context.Background()
	err := sc.Put(ctx, "测试", &CacheResult{})
	if err == nil {
		t.Error("嵌入器报错时 Put 应返回错误")
	}
}

func TestPut_UpdateExisting(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.setEmbedding("测试查询", []float64{1.0, 0.0, 0.0})
	sc := New(embedder)

	ctx := context.Background()

	// 第一次写入
	_ = sc.Put(ctx, "测试查询", &CacheResult{Answer: "答案1"})
	if sc.Size() != 1 {
		t.Errorf("期望 1 个条目，实际 %d", sc.Size())
	}

	// 再次写入相同查询（精确匹配），应该更新而非新增
	_ = sc.Put(ctx, "测试查询", &CacheResult{Answer: "答案2"})
	if sc.Size() != 1 {
		t.Errorf("更新后应仍为 1 个条目，实际 %d", sc.Size())
	}

	// 验证内容已更新
	got, hit, _ := sc.Get(ctx, "测试查询")
	if !hit {
		t.Fatal("期望命中")
	}
	if got.Answer != "答案2" {
		t.Errorf("期望更新后答案为'答案2'，实际 %s", got.Answer)
	}
}

// ============== TTL 过期测试 ==============

func TestGet_TTLExpiry(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.setEmbedding("测试", []float64{1.0, 0.0, 0.0})

	// 设置极短的 TTL
	sc := New(embedder, WithTTL(50*time.Millisecond), WithThreshold(0.99))

	ctx := context.Background()
	_ = sc.Put(ctx, "测试", &CacheResult{Answer: "已过期"})

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	_, hit, err := sc.Get(ctx, "测试")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if hit {
		t.Error("过期条目不应命中")
	}
}

// ============== 容量淘汰测试 ==============

func TestPut_Eviction(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder, WithMaxSize(3))

	ctx := context.Background()

	// 写入 3 个条目（达到容量上限）
	for i := 0; i < 3; i++ {
		query := fmt.Sprintf("查询%d", i)
		embedder.setEmbedding(query, []float64{float64(i), 0, 0})
		_ = sc.Put(ctx, query, &CacheResult{Answer: fmt.Sprintf("答案%d", i)})
	}

	if sc.Size() != 3 {
		t.Errorf("期望 3 个条目，实际 %d", sc.Size())
	}

	// 写入第 4 个，应淘汰最早的
	embedder.setEmbedding("查询3", []float64{3, 0, 0})
	_ = sc.Put(ctx, "查询3", &CacheResult{Answer: "答案3"})

	if sc.Size() != 3 {
		t.Errorf("淘汰后应仍为 3 个条目，实际 %d", sc.Size())
	}

	stats := sc.Stats()
	if stats.Evictions != 1 {
		t.Errorf("期望 1 次淘汰，实际 %d", stats.Evictions)
	}
}

// ============== Invalidate 测试 ==============

func TestInvalidate(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.setEmbedding("查询A", []float64{1, 0, 0})
	embedder.setEmbedding("查询B", []float64{0, 1, 0})
	sc := New(embedder)

	ctx := context.Background()
	_ = sc.Put(ctx, "查询A", &CacheResult{Answer: "A"})
	_ = sc.Put(ctx, "查询B", &CacheResult{Answer: "B"})

	if sc.Size() != 2 {
		t.Fatalf("期望 2 个条目，实际 %d", sc.Size())
	}

	// 失效查询A
	sc.Invalidate("查询A")

	if sc.Size() != 1 {
		t.Errorf("失效后期望 1 个条目，实际 %d", sc.Size())
	}
}

func TestInvalidate_NotFound(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder)

	// 失效不存在的查询不应 panic
	sc.Invalidate("不存在")
}

// ============== Clear 测试 ==============

func TestClear(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder)

	ctx := context.Background()
	_ = sc.Put(ctx, "查询1", &CacheResult{Answer: "A"})
	_ = sc.Put(ctx, "查询2", &CacheResult{Answer: "B"})

	// 记录一些命中/未命中
	_, _, _ = sc.Get(ctx, "查询1")
	_, _, _ = sc.Get(ctx, "不存在")

	sc.Clear()

	if sc.Size() != 0 {
		t.Errorf("清空后期望 0 个条目，实际 %d", sc.Size())
	}

	stats := sc.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("清空后统计应重置")
	}
}

// ============== Stats 测试 ==============

func TestStats(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.setEmbedding("命中查询", []float64{1.0, 0.0, 0.0})
	embedder.setEmbedding("未命中查询", []float64{0.0, 0.0, 1.0})

	sc := New(embedder, WithMaxSize(100), WithThreshold(0.85))

	ctx := context.Background()
	_ = sc.Put(ctx, "命中查询", &CacheResult{Answer: "OK"})

	// 命中
	_, _, _ = sc.Get(ctx, "命中查询")
	// 未命中
	_, _, _ = sc.Get(ctx, "未命中查询")
	_, _, _ = sc.Get(ctx, "未命中查询")

	stats := sc.Stats()

	if stats.Size != 1 {
		t.Errorf("期望 Size=1，实际 %d", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Errorf("期望 MaxSize=100，实际 %d", stats.MaxSize)
	}
	if stats.Hits != 1 {
		t.Errorf("期望 Hits=1，实际 %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("期望 Misses=2，实际 %d", stats.Misses)
	}

	expectedHitRate := 1.0 / 3.0
	if math.Abs(stats.HitRate-expectedHitRate) > 0.01 {
		t.Errorf("期望 HitRate=%.2f，实际 %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestStats_NoRequests(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder)

	stats := sc.Stats()
	if stats.HitRate != 0 {
		t.Errorf("无请求时 HitRate 应为 0，实际 %f", stats.HitRate)
	}
}

// ============== 余弦相似度测试 ==============

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float64
		expected float64
	}{
		{
			name:     "完全相同的向量",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "正交向量",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "反向向量",
			a:        []float64{1, 0, 0},
			b:        []float64{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "空向量",
			a:        []float64{},
			b:        []float64{},
			expected: 0.0,
		},
		{
			name:     "长度不一致",
			a:        []float64{1, 0},
			b:        []float64{1, 0, 0},
			expected: 0.0,
		},
		{
			name:     "零向量",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 0.0,
		},
		{
			name:     "部分相似",
			a:        []float64{1, 1, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0 / math.Sqrt(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("cosineSimilarity(%v, %v) = %f，期望 %f", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// ============== 并发安全测试 ==============

func TestConcurrentAccess(t *testing.T) {
	embedder := newMockEmbedder()
	for i := 0; i < 100; i++ {
		query := fmt.Sprintf("并发查询%d", i)
		embedder.setEmbedding(query, []float64{float64(i), float64(i % 10), 0})
	}

	sc := New(embedder, WithMaxSize(50))
	ctx := context.Background()

	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			query := fmt.Sprintf("并发查询%d", idx)
			_ = sc.Put(ctx, query, &CacheResult{Answer: fmt.Sprintf("答案%d", idx)})
		}(i)
	}

	// 并发读取
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			query := fmt.Sprintf("并发查询%d", idx)
			_, _, _ = sc.Get(ctx, query)
		}(i)
	}

	// 并发统计和清理
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = sc.Stats()
	}()
	go func() {
		defer wg.Done()
		sc.Invalidate("并发查询0")
	}()

	wg.Wait()

	// 不应 panic，缓存大小应在合理范围内
	if sc.Size() > 50 {
		t.Errorf("缓存大小 %d 超过最大限制 50", sc.Size())
	}
}

// ============== CacheResult 完整性测试 ==============

func TestCacheResult_WithDocuments(t *testing.T) {
	embedder := newMockEmbedder()
	embedder.setEmbedding("测试", []float64{1.0, 0.0, 0.0})

	sc := New(embedder, WithThreshold(0.99))
	ctx := context.Background()

	result := &CacheResult{
		Answer: "综合答案",
		Documents: []CachedDocument{
			{Content: "文档1", Score: 0.9, Metadata: map[string]any{"source": "wiki"}},
			{Content: "文档2", Score: 0.8, Metadata: map[string]any{"source": "book"}},
		},
		Metadata: map[string]any{"model": "gpt-4"},
	}

	_ = sc.Put(ctx, "测试", result)

	got, hit, _ := sc.Get(ctx, "测试")
	if !hit {
		t.Fatal("期望命中")
	}
	if len(got.Documents) != 2 {
		t.Errorf("期望 2 个文档，实际 %d", len(got.Documents))
	}
	if got.Documents[0].Content != "文档1" {
		t.Errorf("文档内容不匹配: %s", got.Documents[0].Content)
	}
	if got.Metadata["model"] != "gpt-4" {
		t.Errorf("元数据不匹配")
	}
}

// ============== 返回最佳匹配测试 ==============

func TestGet_ReturnsBestMatch(t *testing.T) {
	embedder := newMockEmbedder()
	// 设置三个查询的向量
	embedder.setEmbedding("Go 编程", []float64{0.9, 0.1, 0.0})
	embedder.setEmbedding("Java 编程", []float64{0.5, 0.5, 0.0})
	embedder.setEmbedding("Go 语言编程", []float64{0.89, 0.11, 0.01})

	sc := New(embedder, WithThreshold(0.5))
	ctx := context.Background()

	_ = sc.Put(ctx, "Go 编程", &CacheResult{Answer: "Go"})
	_ = sc.Put(ctx, "Java 编程", &CacheResult{Answer: "Java"})

	// "Go 语言编程" 应该更接近 "Go 编程"
	got, hit, _ := sc.Get(ctx, "Go 语言编程")
	if !hit {
		t.Fatal("期望命中")
	}
	if got.Answer != "Go" {
		t.Errorf("期望匹配 'Go'，实际 '%s'", got.Answer)
	}
}

// ============== 边界情况 ==============

func TestWithThreshold_ZeroAcceptsAll(t *testing.T) {
	embedder := newMockEmbedder()
	// threshold=0 无效，保持默认 0.85
	sc := New(embedder, WithThreshold(0))

	if sc.threshold != 0.85 {
		t.Errorf("threshold=0 应保持默认 0.85，实际 %f", sc.threshold)
	}
}

func TestWithThreshold_OneExact(t *testing.T) {
	embedder := newMockEmbedder()
	sc := New(embedder, WithThreshold(1.0))

	if sc.threshold != 1.0 {
		t.Errorf("threshold 应为 1.0，实际 %f", sc.threshold)
	}
}
