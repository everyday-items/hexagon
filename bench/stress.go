package bench

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StressTest 压力测试
//
// 在高并发和持续负载下测试系统行为，支持以下特性：
//   - 可配置并发数和持续时间
//   - 升温阶段（ramp-up）逐步增加并发
//   - 延迟统计（P50/P95/P99/Max）
//   - 内存使用和 goroutine 泄漏检测
//   - 错误率统计
//
// 使用示例：
//
//	st := NewStressTest("my_test", func(ctx context.Context) error {
//	    // 执行被测操作
//	    return nil
//	}, WithStressConcurrency(100), WithStressDuration(10*time.Second))
//	result, _ := st.Run(context.Background())
//	fmt.Println(result)
type StressTest struct {
	// name 测试名称
	name string

	// fn 被测函数
	fn func(ctx context.Context) error

	// concurrency 并发数
	concurrency int

	// duration 持续时间
	duration time.Duration

	// rampUp 升温时间，在此期间逐步增加并发数
	rampUp time.Duration
}

// StressResult 压力测试结果
//
// 包含性能指标、内存使用和 goroutine 信息。
type StressResult struct {
	// Name 测试名称
	Name string `json:"name"`

	// TotalOps 总操作数
	TotalOps int64 `json:"total_ops"`

	// SuccessOps 成功操作数
	SuccessOps int64 `json:"success_ops"`

	// FailedOps 失败操作数
	FailedOps int64 `json:"failed_ops"`

	// OpsPerSecond 每秒操作数
	OpsPerSecond float64 `json:"ops_per_second"`

	// AvgLatency 平均延迟
	AvgLatency time.Duration `json:"avg_latency"`

	// P50Latency P50 延迟
	P50Latency time.Duration `json:"p50_latency"`

	// P95Latency P95 延迟
	P95Latency time.Duration `json:"p95_latency"`

	// P99Latency P99 延迟
	P99Latency time.Duration `json:"p99_latency"`

	// MaxLatency 最大延迟
	MaxLatency time.Duration `json:"max_latency"`

	// ErrorRate 错误率（0.0 - 1.0）
	ErrorRate float64 `json:"error_rate"`

	// Duration 实际运行时长
	Duration time.Duration `json:"duration"`

	// MemoryStart 起始内存使用（字节）
	MemoryStart uint64 `json:"memory_start"`

	// MemoryEnd 结束内存使用（字节）
	MemoryEnd uint64 `json:"memory_end"`

	// MemoryGrowth 内存增长（字节，可为负数）
	MemoryGrowth int64 `json:"memory_growth"`

	// GoroutineStart 起始 goroutine 数
	GoroutineStart int `json:"goroutine_start"`

	// GoroutineEnd 结束 goroutine 数
	GoroutineEnd int `json:"goroutine_end"`
}

// StressOption 压力测试配置选项
type StressOption func(*StressTest)

// WithStressConcurrency 设置并发数
func WithStressConcurrency(n int) StressOption {
	return func(s *StressTest) {
		if n > 0 {
			s.concurrency = n
		}
	}
}

// WithStressDuration 设置持续时间
func WithStressDuration(d time.Duration) StressOption {
	return func(s *StressTest) {
		if d > 0 {
			s.duration = d
		}
	}
}

// WithStressRampUp 设置升温时间
//
// 在升温阶段，并发数会从 1 逐步增加到目标并发数。
// 升温时间不应超过总持续时间。
func WithStressRampUp(d time.Duration) StressOption {
	return func(s *StressTest) {
		if d > 0 {
			s.rampUp = d
		}
	}
}

// NewStressTest 创建压力测试
//
// 默认配置：10 并发、5 秒持续时间、无升温。
func NewStressTest(name string, fn func(ctx context.Context) error, opts ...StressOption) *StressTest {
	s := &StressTest{
		name:        name,
		fn:          fn,
		concurrency: 10,
		duration:    5 * time.Second,
		rampUp:      0,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run 执行压力测试
//
// 创建指定数量的 goroutine，在持续时间内反复执行被测函数。
// 收集延迟数据并计算统计信息。
// 如果设置了升温时间，会逐步启动 goroutine。
func (s *StressTest) Run(ctx context.Context) (*StressResult, error) {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(ctx, s.duration)
	defer cancel()

	// 记录起始状态
	runtime.GC()
	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)
	goroutineStart := runtime.NumGoroutine()

	// 操作计数器（原子操作确保并发安全）
	var totalOps atomic.Int64
	var successOps atomic.Int64
	var failedOps atomic.Int64

	// 延迟收集（使用 mutex 保护切片）
	var latencyMu sync.Mutex
	latencies := make([]time.Duration, 0, 10000)

	// 等待所有 goroutine 完成
	var wg sync.WaitGroup

	start := time.Now()

	if s.rampUp > 0 && s.rampUp < s.duration {
		// 升温模式：逐步启动 goroutine
		rampInterval := s.rampUp / time.Duration(s.concurrency)
		rampInterval = max(rampInterval, time.Millisecond)

	rampLoop:
		for i := 0; i < s.concurrency; i++ {
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				break rampLoop
			default:
			}

			wg.Add(1)
			go s.worker(ctx, &wg, &totalOps, &successOps, &failedOps, &latencyMu, &latencies)

			if i < s.concurrency-1 {
				time.Sleep(rampInterval)
			}
		}
	} else {
		// 立即启动所有 goroutine
		for i := 0; i < s.concurrency; i++ {
			wg.Add(1)
			go s.worker(ctx, &wg, &totalOps, &successOps, &failedOps, &latencyMu, &latencies)
		}
	}

	wg.Wait()
	actualDuration := time.Since(start)

	// 记录结束状态
	var memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsAfter)
	goroutineEnd := runtime.NumGoroutine()

	// 计算延迟统计
	result := &StressResult{
		Name:           s.name,
		TotalOps:       totalOps.Load(),
		SuccessOps:     successOps.Load(),
		FailedOps:      failedOps.Load(),
		Duration:       actualDuration,
		MemoryStart:    memStatsBefore.Alloc,
		MemoryEnd:      memStatsAfter.Alloc,
		MemoryGrowth:   int64(memStatsAfter.Alloc) - int64(memStatsBefore.Alloc),
		GoroutineStart: goroutineStart,
		GoroutineEnd:   goroutineEnd,
	}

	total := result.TotalOps
	if total > 0 {
		result.OpsPerSecond = float64(total) / actualDuration.Seconds()
		result.ErrorRate = float64(result.FailedOps) / float64(total)
	}

	// 计算延迟百分位数
	latencyMu.Lock()
	defer latencyMu.Unlock()

	if len(latencies) > 0 {
		slices.Sort(latencies)

		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		result.AvgLatency = sum / time.Duration(len(latencies))
		result.P50Latency = percentile(latencies, 0.50)
		result.P95Latency = percentile(latencies, 0.95)
		result.P99Latency = percentile(latencies, 0.99)
		result.MaxLatency = latencies[len(latencies)-1]
	}

	return result, nil
}

// worker 单个压力测试工作协程
//
// 在上下文有效期内反复执行被测函数，记录延迟和错误。
func (s *StressTest) worker(
	ctx context.Context,
	wg *sync.WaitGroup,
	totalOps, successOps, failedOps *atomic.Int64,
	latencyMu *sync.Mutex,
	latencies *[]time.Duration,
) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		opStart := time.Now()
		err := s.fn(ctx)
		elapsed := time.Since(opStart)

		totalOps.Add(1)
		if err != nil {
			failedOps.Add(1)
		} else {
			successOps.Add(1)
		}

		latencyMu.Lock()
		*latencies = append(*latencies, elapsed)
		latencyMu.Unlock()
	}
}

// String 返回压力测试结果的可读字符串
func (r *StressResult) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "=== 压力测试结果: %s ===\n", r.Name)
	fmt.Fprintf(&sb, "持续时间:    %s\n", r.Duration)
	fmt.Fprintf(&sb, "总操作数:    %d\n", r.TotalOps)
	fmt.Fprintf(&sb, "成功:        %d\n", r.SuccessOps)
	fmt.Fprintf(&sb, "失败:        %d\n", r.FailedOps)
	fmt.Fprintf(&sb, "每秒操作:    %.2f ops/s\n", r.OpsPerSecond)
	fmt.Fprintf(&sb, "错误率:      %.2f%%\n", r.ErrorRate*100)
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "平均延迟:    %s\n", r.AvgLatency)
	fmt.Fprintf(&sb, "P50 延迟:    %s\n", r.P50Latency)
	fmt.Fprintf(&sb, "P95 延迟:    %s\n", r.P95Latency)
	fmt.Fprintf(&sb, "P99 延迟:    %s\n", r.P99Latency)
	fmt.Fprintf(&sb, "最大延迟:    %s\n", r.MaxLatency)
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "起始内存:    %s\n", formatBytes(r.MemoryStart))
	fmt.Fprintf(&sb, "结束内存:    %s\n", formatBytes(r.MemoryEnd))
	fmt.Fprintf(&sb, "内存增长:    %s\n", formatSignedBytes(r.MemoryGrowth))
	fmt.Fprintf(&sb, "起始协程:    %d\n", r.GoroutineStart)
	fmt.Fprintf(&sb, "结束协程:    %d\n", r.GoroutineEnd)

	return sb.String()
}

// formatBytes 格式化字节数为可读字符串
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatSignedBytes 格式化有符号字节数
func formatSignedBytes(b int64) string {
	sign := ""
	absB := uint64(b)
	if b < 0 {
		sign = "-"
		absB = uint64(math.Abs(float64(b)))
	} else {
		sign = "+"
	}
	return sign + formatBytes(absB)
}
