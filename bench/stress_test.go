package bench

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestStressTest_Basic 测试基本压力测试功能
func TestStressTest_Basic(t *testing.T) {
	var counter atomic.Int64

	st := NewStressTest("basic_stress",
		func(ctx context.Context) error {
			counter.Add(1)
			return nil
		},
		WithStressConcurrency(4),
		WithStressDuration(500*time.Millisecond),
	)

	result, err := st.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	if result.Name != "basic_stress" {
		t.Errorf("名称期望 basic_stress, 实际 %s", result.Name)
	}

	if result.TotalOps == 0 {
		t.Error("总操作数不应为 0")
	}

	if result.SuccessOps != result.TotalOps {
		t.Errorf("全部操作应成功: success=%d, total=%d", result.SuccessOps, result.TotalOps)
	}

	if result.FailedOps != 0 {
		t.Errorf("不应有失败操作: %d", result.FailedOps)
	}

	if result.ErrorRate != 0 {
		t.Errorf("错误率应为 0, 实际 %f", result.ErrorRate)
	}

	if result.OpsPerSecond <= 0 {
		t.Error("每秒操作数应大于 0")
	}

	// 验证延迟统计
	if result.AvgLatency <= 0 {
		t.Error("平均延迟应大于 0")
	}
	if result.P50Latency <= 0 {
		t.Error("P50 延迟应大于 0")
	}
	if result.P99Latency < result.P50Latency {
		t.Error("P99 延迟应 >= P50 延迟")
	}
	if result.MaxLatency < result.P99Latency {
		t.Error("最大延迟应 >= P99 延迟")
	}

	t.Log(result.String())
}

// TestStressTest_WithErrors 测试有错误的压力测试
func TestStressTest_WithErrors(t *testing.T) {
	var counter atomic.Int64
	testErr := errors.New("test error")

	st := NewStressTest("error_stress",
		func(ctx context.Context) error {
			n := counter.Add(1)
			// 每 3 次操作返回一次错误
			if n%3 == 0 {
				return testErr
			}
			return nil
		},
		WithStressConcurrency(2),
		WithStressDuration(500*time.Millisecond),
	)

	result, err := st.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	if result.FailedOps == 0 {
		t.Error("应有失败操作")
	}

	if result.ErrorRate <= 0 {
		t.Error("错误率应大于 0")
	}

	if result.SuccessOps+result.FailedOps != result.TotalOps {
		t.Errorf("成功+失败应等于总数: %d + %d != %d",
			result.SuccessOps, result.FailedOps, result.TotalOps)
	}
}

// TestStressTest_WithRampUp 测试升温模式
func TestStressTest_WithRampUp(t *testing.T) {
	st := NewStressTest("rampup_stress",
		func(ctx context.Context) error {
			return nil
		},
		WithStressConcurrency(4),
		WithStressDuration(1*time.Second),
		WithStressRampUp(200*time.Millisecond),
	)

	result, err := st.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	if result.TotalOps == 0 {
		t.Error("总操作数不应为 0")
	}

	// 升温模式应该也能正常完成
	if result.Duration < 500*time.Millisecond {
		t.Errorf("持续时间应至少 500ms, 实际 %s", result.Duration)
	}
}

// TestStressTest_ContextCancellation 测试上下文取消
func TestStressTest_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	st := NewStressTest("cancel_stress",
		func(ctx context.Context) error {
			time.Sleep(time.Millisecond)
			return nil
		},
		WithStressConcurrency(2),
		WithStressDuration(10*time.Second), // 设置很长的持续时间
	)

	result, err := st.Run(ctx)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 应在 200ms 左右完成（由外部 context 控制）
	if result.Duration > 1*time.Second {
		t.Errorf("应被外部上下文取消, 实际运行 %s", result.Duration)
	}
}

// TestStressTest_MemoryTracking 测试内存追踪
func TestStressTest_MemoryTracking(t *testing.T) {
	st := NewStressTest("memory_stress",
		func(ctx context.Context) error {
			// 分配一些内存
			_ = make([]byte, 1024)
			return nil
		},
		WithStressConcurrency(2),
		WithStressDuration(300*time.Millisecond),
	)

	result, err := st.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 起始内存应该有值
	if result.MemoryStart == 0 {
		t.Error("起始内存不应为 0")
	}
}

// TestStressTest_GoroutineTracking 测试 goroutine 追踪
func TestStressTest_GoroutineTracking(t *testing.T) {
	st := NewStressTest("goroutine_stress",
		func(ctx context.Context) error {
			return nil
		},
		WithStressConcurrency(2),
		WithStressDuration(300*time.Millisecond),
	)

	result, err := st.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	if result.GoroutineStart == 0 {
		t.Error("起始 goroutine 数不应为 0")
	}
}

// TestStressTest_DefaultOptions 测试默认配置
func TestStressTest_DefaultOptions(t *testing.T) {
	st := NewStressTest("default_test",
		func(ctx context.Context) error {
			return nil
		},
	)

	if st.concurrency != 10 {
		t.Errorf("默认并发数应为 10, 实际 %d", st.concurrency)
	}
	if st.duration != 5*time.Second {
		t.Errorf("默认持续时间应为 5s, 实际 %s", st.duration)
	}
	if st.rampUp != 0 {
		t.Errorf("默认升温时间应为 0, 实际 %s", st.rampUp)
	}
}

// TestStressTest_InvalidOptions 测试无效配置选项（应保留默认值）
func TestStressTest_InvalidOptions(t *testing.T) {
	st := NewStressTest("invalid_test",
		func(ctx context.Context) error {
			return nil
		},
		WithStressConcurrency(-1),
		WithStressDuration(-1*time.Second),
		WithStressRampUp(-1*time.Second),
	)

	if st.concurrency != 10 {
		t.Errorf("无效并发数应保留默认值 10, 实际 %d", st.concurrency)
	}
	if st.duration != 5*time.Second {
		t.Errorf("无效持续时间应保留默认值 5s, 实际 %s", st.duration)
	}
}

// TestStressResult_String 测试结果字符串格式化
func TestStressResult_String(t *testing.T) {
	result := &StressResult{
		Name:           "format_test",
		TotalOps:       1000,
		SuccessOps:     990,
		FailedOps:      10,
		OpsPerSecond:   200.0,
		AvgLatency:     5 * time.Millisecond,
		P50Latency:     4 * time.Millisecond,
		P95Latency:     10 * time.Millisecond,
		P99Latency:     20 * time.Millisecond,
		MaxLatency:     50 * time.Millisecond,
		ErrorRate:      0.01,
		Duration:       5 * time.Second,
		MemoryStart:    1024 * 1024,
		MemoryEnd:      2 * 1024 * 1024,
		MemoryGrowth:   1024 * 1024,
		GoroutineStart: 5,
		GoroutineEnd:   5,
	}

	s := result.String()
	if s == "" {
		t.Fatal("String() 不应返回空字符串")
	}

	checks := []string{
		"format_test",
		"1000",
		"990",
		"10",
		"200.00",
	}
	for _, check := range checks {
		if !containsSubstr(s, check) {
			t.Errorf("结果字符串应包含 %q", check)
		}
	}

	t.Log(s)
}

// TestFormatBytes 测试字节格式化
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, 期望 %s", tt.input, result, tt.expected)
		}
	}
}

// BenchmarkStressTest 基准测试压力测试框架本身的开销
func BenchmarkStressTest(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		st := NewStressTest("bench",
			func(ctx context.Context) error {
				return nil
			},
			WithStressConcurrency(2),
			WithStressDuration(100*time.Millisecond),
		)
		_, _ = st.Run(context.Background())
	}
}
