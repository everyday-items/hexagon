// Package bench 提供 Hexagon AI Agent 框架的基准测试工具
//
// Bench 用于性能基准测试：
//   - BenchmarkRunner: 基准测试运行器
//   - BenchmarkReport: 基准测试报告
//   - 预定义基准测试
package bench

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"
)

// BenchmarkFunc 基准测试函数
type BenchmarkFunc func(ctx context.Context) error

// Benchmark 基准测试
type Benchmark struct {
	// Name 测试名称
	Name string

	// Description 描述
	Description string

	// Fn 测试函数
	Fn BenchmarkFunc

	// Setup 测试前准备函数
	Setup func() error

	// Teardown 测试后清理函数
	Teardown func() error
}

// BenchmarkResult 基准测试结果
type BenchmarkResult struct {
	// Name 测试名称
	Name string `json:"name"`

	// Iterations 迭代次数
	Iterations int `json:"iterations"`

	// TotalDuration 总耗时
	TotalDuration time.Duration `json:"total_duration"`

	// AvgDuration 平均耗时
	AvgDuration time.Duration `json:"avg_duration"`

	// MinDuration 最小耗时
	MinDuration time.Duration `json:"min_duration"`

	// MaxDuration 最大耗时
	MaxDuration time.Duration `json:"max_duration"`

	// P50 P50 延迟
	P50 time.Duration `json:"p50"`

	// P95 P95 延迟
	P95 time.Duration `json:"p95"`

	// P99 P99 延迟
	P99 time.Duration `json:"p99"`

	// OpsPerSecond 每秒操作数
	OpsPerSecond float64 `json:"ops_per_second"`

	// Errors 错误数
	Errors int `json:"errors"`

	// MemAllocs 内存分配次数
	MemAllocs uint64 `json:"mem_allocs"`

	// MemBytes 内存分配字节数
	MemBytes uint64 `json:"mem_bytes"`
}

// BenchmarkReport 基准测试报告
type BenchmarkReport struct {
	// Name 报告名称
	Name string `json:"name"`

	// Results 测试结果列表
	Results []BenchmarkResult `json:"results"`

	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`

	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`

	// TotalDuration 总耗时
	TotalDuration time.Duration `json:"total_duration"`

	// Environment 环境信息
	Environment map[string]string `json:"environment"`
}

// Runner 基准测试运行器
type Runner struct {
	benchmarks  []Benchmark
	iterations  int
	warmup      int
	concurrency int
	timeout     time.Duration
}

// RunnerOption 运行器选项
type RunnerOption func(*Runner)

// WithIterations 设置迭代次数
func WithIterations(n int) RunnerOption {
	return func(r *Runner) {
		r.iterations = n
	}
}

// WithWarmup 设置预热次数
func WithWarmup(n int) RunnerOption {
	return func(r *Runner) {
		r.warmup = n
	}
}

// WithConcurrency 设置并发数
func WithConcurrency(n int) RunnerOption {
	return func(r *Runner) {
		r.concurrency = n
	}
}

// WithTimeout 设置超时时间
func WithTimeout(d time.Duration) RunnerOption {
	return func(r *Runner) {
		r.timeout = d
	}
}

// NewRunner 创建基准测试运行器
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		benchmarks:  make([]Benchmark, 0),
		iterations:  100,
		warmup:      10,
		concurrency: 1,
		timeout:     30 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Add 添加基准测试
func (r *Runner) Add(b Benchmark) *Runner {
	r.benchmarks = append(r.benchmarks, b)
	return r
}

// Run 运行所有基准测试
func (r *Runner) Run(ctx context.Context) (*BenchmarkReport, error) {
	report := &BenchmarkReport{
		Name:      "Hexagon Benchmark",
		Results:   make([]BenchmarkResult, 0, len(r.benchmarks)),
		StartTime: time.Now(),
		Environment: map[string]string{
			"go_version":   runtime.Version(),
			"go_os":        runtime.GOOS,
			"go_arch":      runtime.GOARCH,
			"num_cpu":      fmt.Sprintf("%d", runtime.NumCPU()),
			"num_goroutine": fmt.Sprintf("%d", runtime.NumGoroutine()),
		},
	}

	for _, bench := range r.benchmarks {
		result, err := r.runBenchmark(ctx, bench)
		if err != nil {
			return nil, fmt.Errorf("benchmark %s failed: %w", bench.Name, err)
		}
		report.Results = append(report.Results, *result)
	}

	report.EndTime = time.Now()
	report.TotalDuration = report.EndTime.Sub(report.StartTime)

	return report, nil
}

func (r *Runner) runBenchmark(ctx context.Context, bench Benchmark) (*BenchmarkResult, error) {
	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// 准备
	if bench.Setup != nil {
		if err := bench.Setup(); err != nil {
			return nil, fmt.Errorf("setup failed: %w", err)
		}
	}

	defer func() {
		if bench.Teardown != nil {
			bench.Teardown()
		}
	}()

	// 预热
	for i := 0; i < r.warmup; i++ {
		_ = bench.Fn(ctx)
	}

	// 运行基准测试
	durations := make([]time.Duration, 0, r.iterations)
	var errors int
	var memStatsBefore, memStatsAfter runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&memStatsBefore)

	start := time.Now()

	if r.concurrency > 1 {
		// 并发执行
		durCh := make(chan time.Duration, r.iterations)
		errCh := make(chan struct{}, r.iterations)

		var wg sync.WaitGroup
		sem := make(chan struct{}, r.concurrency)

		for i := 0; i < r.iterations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				iterStart := time.Now()
				err := bench.Fn(ctx)
				durCh <- time.Since(iterStart)
				if err != nil {
					errCh <- struct{}{}
				}
			}()
		}

		wg.Wait()
		close(durCh)
		close(errCh)

		for d := range durCh {
			durations = append(durations, d)
		}
		errors = len(errCh)
	} else {
		// 串行执行
		for i := 0; i < r.iterations; i++ {
			iterStart := time.Now()
			err := bench.Fn(ctx)
			durations = append(durations, time.Since(iterStart))
			if err != nil {
				errors++
			}
		}
	}

	totalDuration := time.Since(start)

	runtime.ReadMemStats(&memStatsAfter)

	// 计算统计信息
	result := &BenchmarkResult{
		Name:          bench.Name,
		Iterations:    r.iterations,
		TotalDuration: totalDuration,
		Errors:        errors,
		MemAllocs:     memStatsAfter.Mallocs - memStatsBefore.Mallocs,
		MemBytes:      memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc,
	}

	// 计算延迟统计
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	if len(durations) > 0 {
		result.MinDuration = durations[0]
		result.MaxDuration = durations[len(durations)-1]
		result.P50 = percentile(durations, 0.50)
		result.P95 = percentile(durations, 0.95)
		result.P99 = percentile(durations, 0.99)

		var sum time.Duration
		for _, d := range durations {
			sum += d
		}
		result.AvgDuration = sum / time.Duration(len(durations))
		result.OpsPerSecond = float64(len(durations)) / totalDuration.Seconds()
	}

	return result, nil
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// ============== 预定义基准测试 ==============

// SimpleBenchmark 创建简单基准测试
func SimpleBenchmark(name string, fn BenchmarkFunc) Benchmark {
	return Benchmark{
		Name: name,
		Fn:   fn,
	}
}

// NopBenchmark 空操作基准测试（用于测量基线开销）
func NopBenchmark() Benchmark {
	return Benchmark{
		Name:        "nop",
		Description: "No-op benchmark for measuring baseline overhead",
		Fn: func(ctx context.Context) error {
			return nil
		},
	}
}

// SleepBenchmark 睡眠基准测试（用于验证计时）
func SleepBenchmark(d time.Duration) Benchmark {
	return Benchmark{
		Name:        fmt.Sprintf("sleep_%s", d),
		Description: fmt.Sprintf("Sleep for %s", d),
		Fn: func(ctx context.Context) error {
			time.Sleep(d)
			return nil
		},
	}
}

// ============== 报告格式化 ==============

// FormatReport 格式化报告
func FormatReport(report *BenchmarkReport) string {
	var sb fmt.Stringer = &reportFormatter{report: report}
	return sb.String()
}

type reportFormatter struct {
	report *BenchmarkReport
}

func (f *reportFormatter) String() string {
	r := f.report
	s := fmt.Sprintf("=== Benchmark Report: %s ===\n", r.Name)
	s += fmt.Sprintf("Duration: %s\n", r.TotalDuration)
	s += fmt.Sprintf("Go Version: %s\n", r.Environment["go_version"])
	s += "\n"

	for _, result := range r.Results {
		s += fmt.Sprintf("--- %s ---\n", result.Name)
		s += fmt.Sprintf("  Iterations: %d\n", result.Iterations)
		s += fmt.Sprintf("  Avg: %s\n", result.AvgDuration)
		s += fmt.Sprintf("  Min: %s\n", result.MinDuration)
		s += fmt.Sprintf("  Max: %s\n", result.MaxDuration)
		s += fmt.Sprintf("  P50: %s\n", result.P50)
		s += fmt.Sprintf("  P95: %s\n", result.P95)
		s += fmt.Sprintf("  P99: %s\n", result.P99)
		s += fmt.Sprintf("  Ops/sec: %.2f\n", result.OpsPerSecond)
		s += fmt.Sprintf("  Errors: %d\n", result.Errors)
		s += fmt.Sprintf("  MemAllocs: %d\n", result.MemAllocs)
		s += fmt.Sprintf("  MemBytes: %d\n", result.MemBytes)
		s += "\n"
	}

	return s
}
