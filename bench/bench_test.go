package bench

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkNop 测试空操作基线
func BenchmarkNop(b *testing.B) {
	ctx := context.Background()
	bench := NopBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bench.Fn(ctx)
	}
}

// BenchmarkRunner 测试运行器本身的开销
func BenchmarkRunner(b *testing.B) {
	ctx := context.Background()
	runner := NewRunner(WithIterations(100), WithWarmup(0))
	runner.Add(NopBenchmark())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runner.Run(ctx)
	}
}

// TestRunner 测试运行器
func TestRunner(t *testing.T) {
	ctx := context.Background()
	runner := NewRunner(
		WithIterations(10),
		WithWarmup(2),
	)

	runner.Add(NopBenchmark())
	runner.Add(SleepBenchmark(time.Millisecond))

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(report.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(report.Results))
	}

	// 验证 nop 基准测试
	nopResult := report.Results[0]
	if nopResult.Name != "nop" {
		t.Errorf("Expected 'nop', got %s", nopResult.Name)
	}
	if nopResult.Iterations != 10 {
		t.Errorf("Expected 10 iterations, got %d", nopResult.Iterations)
	}

	// 验证 sleep 基准测试
	sleepResult := report.Results[1]
	if sleepResult.AvgDuration < time.Millisecond {
		t.Errorf("Expected avg duration >= 1ms, got %s", sleepResult.AvgDuration)
	}

	// 打印报告
	t.Log(FormatReport(report))
}

// TestConcurrentRunner 测试并发运行器
func TestConcurrentRunner(t *testing.T) {
	ctx := context.Background()
	runner := NewRunner(
		WithIterations(20),
		WithWarmup(0),
		WithConcurrency(4),
	)

	var counter atomic.Int64
	runner.Add(Benchmark{
		Name: "counter",
		Fn: func(ctx context.Context) error {
			counter.Add(1)
			return nil
		},
	})

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(report.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(report.Results))
	}

	result := report.Results[0]
	if result.Iterations != 20 {
		t.Errorf("Expected 20 iterations, got %d", result.Iterations)
	}

	// 验证所有迭代都实际执行了
	if counter.Load() != 20 {
		t.Errorf("Expected counter=20, got %d", counter.Load())
	}
}
