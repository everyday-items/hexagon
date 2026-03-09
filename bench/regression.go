package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RegressionDetector 性能回归检测器
//
// 对比当前基准测试结果与历史基线，检测性能退化。
// 支持多维度对比（耗时、分配次数、分配字节），超过阈值即判定为回归。
//
// 使用示例：
//
//	detector := NewRegressionDetector("baselines.json", WithThreshold(0.1))
//	_ = detector.LoadBaseline()
//	results, _ := detector.Check(benchResults)
//	fmt.Println(detector.Report(results))
type RegressionDetector struct {
	// baselinePath 基线文件路径
	baselinePath string

	// threshold 回归阈值百分比（如 0.1 表示 10%）
	threshold float64

	// baselines 历史基线数据，键为测试名称
	baselines map[string]*Baseline

	// allocsThreshold 内存分配次数回归阈值（独立配置，默认与 threshold 相同）
	allocsThreshold float64

	// bytesThreshold 内存分配字节回归阈值（独立配置，默认与 threshold 相同）
	bytesThreshold float64
}

// Baseline 性能基线
//
// 记录某个基准测试在某一时间点的性能数据，用于后续的回归对比。
type Baseline struct {
	// Name 测试名称
	Name string `json:"name"`

	// AvgNs 平均耗时（纳秒）
	AvgNs float64 `json:"avg_ns"`

	// P99Ns P99 耗时（纳秒）
	P99Ns float64 `json:"p99_ns"`

	// AllocsOp 每次操作分配次数
	AllocsOp int64 `json:"allocs_op"`

	// BytesOp 每次操作分配字节
	BytesOp int64 `json:"bytes_op"`

	// Timestamp 基线记录时间
	Timestamp time.Time `json:"timestamp"`
}

// RegressionResult 回归检测结果
//
// 描述单个基准测试的回归检测结果，包括是否发生回归以及详细的变化信息。
type RegressionResult struct {
	// Name 测试名称
	Name string `json:"name"`

	// Regressed 是否发生回归
	Regressed bool `json:"regressed"`

	// Current 当前测试结果
	Current *Baseline `json:"current"`

	// Previous 历史基线
	Previous *Baseline `json:"previous"`

	// DeltaPct 平均耗时变化百分比（正数表示变慢，负数表示变快）
	DeltaPct float64 `json:"delta_pct"`

	// Detail 详细说明
	Detail string `json:"detail"`
}

// RegressionOption 回归检测器配置选项
type RegressionOption func(*RegressionDetector)

// WithThreshold 设置回归阈值百分比
//
// threshold 为百分比，如 0.1 表示 10%，即当性能下降超过 10% 时判定为回归。
// 同时会设置 allocsThreshold 和 bytesThreshold 为相同值。
func WithThreshold(threshold float64) RegressionOption {
	return func(d *RegressionDetector) {
		d.threshold = threshold
		d.allocsThreshold = threshold
		d.bytesThreshold = threshold
	}
}

// WithAllocsThreshold 单独设置内存分配次数的回归阈值
func WithAllocsThreshold(threshold float64) RegressionOption {
	return func(d *RegressionDetector) {
		d.allocsThreshold = threshold
	}
}

// WithBytesThreshold 单独设置内存分配字节数的回归阈值
func WithBytesThreshold(threshold float64) RegressionOption {
	return func(d *RegressionDetector) {
		d.bytesThreshold = threshold
	}
}

// NewRegressionDetector 创建性能回归检测器
//
// baselinePath 为基线文件的存储路径（JSON 格式）。
// 默认回归阈值为 10%（0.1），可通过选项覆盖。
func NewRegressionDetector(baselinePath string, opts ...RegressionOption) *RegressionDetector {
	d := &RegressionDetector{
		baselinePath:    baselinePath,
		threshold:       0.1,
		baselines:       make(map[string]*Baseline),
		allocsThreshold: 0.1,
		bytesThreshold:  0.1,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// SaveBaseline 保存当前基准测试结果为基线
//
// 将 BenchmarkResult 转换为 Baseline 并以 JSON 格式保存到文件。
// 如果目标目录不存在，会自动创建。
func (d *RegressionDetector) SaveBaseline(results []*BenchmarkResult) error {
	baselines := make(map[string]*Baseline, len(results))
	now := time.Now()

	for _, r := range results {
		if r == nil {
			continue
		}
		avgNs := float64(r.AvgDuration.Nanoseconds())
		p99Ns := float64(r.P99.Nanoseconds())
		var allocsOp, bytesOp int64
		if r.Iterations > 0 {
			allocsOp = int64(r.MemAllocs) / int64(r.Iterations)
			bytesOp = int64(r.MemBytes) / int64(r.Iterations)
		}

		baselines[r.Name] = &Baseline{
			Name:      r.Name,
			AvgNs:     avgNs,
			P99Ns:     p99Ns,
			AllocsOp:  allocsOp,
			BytesOp:   bytesOp,
			Timestamp: now,
		}
	}

	// 确保目录存在
	dir := filepath.Dir(d.baselinePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建基线目录失败: %w", err)
		}
	}

	data, err := json.MarshalIndent(baselines, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化基线数据失败: %w", err)
	}

	if err := os.WriteFile(d.baselinePath, data, 0644); err != nil {
		return fmt.Errorf("写入基线文件失败: %w", err)
	}

	d.baselines = baselines
	return nil
}

// LoadBaseline 从文件加载历史基线
//
// 如果文件不存在，返回 nil（不视为错误，因为首次运行时没有基线）。
func (d *RegressionDetector) LoadBaseline() error {
	data, err := os.ReadFile(d.baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 首次运行没有基线，不视为错误
			return nil
		}
		return fmt.Errorf("读取基线文件失败: %w", err)
	}

	baselines := make(map[string]*Baseline)
	if err := json.Unmarshal(data, &baselines); err != nil {
		return fmt.Errorf("解析基线数据失败: %w", err)
	}

	d.baselines = baselines
	return nil
}

// Check 对比当前结果与历史基线，检测性能回归
//
// 对每个基准测试结果，分别检查平均耗时、P99 耗时、分配次数和分配字节数。
// 任意一项超过阈值即判定为回归。
// 如果没有对应的历史基线，跳过该测试（不视为回归）。
func (d *RegressionDetector) Check(results []*BenchmarkResult) ([]RegressionResult, error) {
	regressions := make([]RegressionResult, 0, len(results))

	for _, r := range results {
		if r == nil {
			continue
		}

		current := &Baseline{
			Name:  r.Name,
			AvgNs: float64(r.AvgDuration.Nanoseconds()),
			P99Ns: float64(r.P99.Nanoseconds()),
		}
		if r.Iterations > 0 {
			current.AllocsOp = int64(r.MemAllocs) / int64(r.Iterations)
			current.BytesOp = int64(r.MemBytes) / int64(r.Iterations)
		}
		current.Timestamp = time.Now()

		prev, ok := d.baselines[r.Name]
		if !ok {
			// 没有历史基线，跳过
			regressions = append(regressions, RegressionResult{
				Name:    r.Name,
				Current: current,
				Detail:  "无历史基线，跳过检测",
			})
			continue
		}

		// 计算变化百分比
		result := RegressionResult{
			Name:     r.Name,
			Current:  current,
			Previous: prev,
		}

		var details []string
		regressed := false

		// 检查平均耗时
		if prev.AvgNs > 0 {
			deltaPct := (current.AvgNs - prev.AvgNs) / prev.AvgNs
			result.DeltaPct = deltaPct
			if deltaPct > d.threshold {
				regressed = true
				details = append(details, fmt.Sprintf("平均耗时回归 %.1f%% (%.0fns -> %.0fns)",
					deltaPct*100, prev.AvgNs, current.AvgNs))
			}
		}

		// 检查 P99 耗时
		if prev.P99Ns > 0 {
			p99DeltaPct := (current.P99Ns - prev.P99Ns) / prev.P99Ns
			if p99DeltaPct > d.threshold {
				regressed = true
				details = append(details, fmt.Sprintf("P99 耗时回归 %.1f%% (%.0fns -> %.0fns)",
					p99DeltaPct*100, prev.P99Ns, current.P99Ns))
			}
		}

		// 检查内存分配次数
		if prev.AllocsOp > 0 {
			allocsDelta := float64(current.AllocsOp-prev.AllocsOp) / float64(prev.AllocsOp)
			if allocsDelta > d.allocsThreshold {
				regressed = true
				details = append(details, fmt.Sprintf("分配次数回归 %.1f%% (%d -> %d)",
					allocsDelta*100, prev.AllocsOp, current.AllocsOp))
			}
		}

		// 检查内存分配字节数
		if prev.BytesOp > 0 {
			bytesDelta := float64(current.BytesOp-prev.BytesOp) / float64(prev.BytesOp)
			if bytesDelta > d.bytesThreshold {
				regressed = true
				details = append(details, fmt.Sprintf("分配字节回归 %.1f%% (%d -> %d)",
					bytesDelta*100, prev.BytesOp, current.BytesOp))
			}
		}

		result.Regressed = regressed
		if len(details) > 0 {
			result.Detail = strings.Join(details, "; ")
		} else {
			// 计算改善的情况
			if result.DeltaPct < 0 {
				result.Detail = fmt.Sprintf("性能提升 %.1f%%", math.Abs(result.DeltaPct)*100)
			} else {
				result.Detail = "性能稳定"
			}
		}

		regressions = append(regressions, result)
	}

	return regressions, nil
}

// Report 生成人类可读的回归检测报告
func (d *RegressionDetector) Report(results []RegressionResult) string {
	var sb strings.Builder

	sb.WriteString("=== 性能回归检测报告 ===\n")
	fmt.Fprintf(&sb, "阈值: %.0f%%\n", d.threshold*100)
	fmt.Fprintf(&sb, "基线文件: %s\n\n", d.baselinePath)

	regressedCount := 0
	for _, r := range results {
		if r.Regressed {
			regressedCount++
		}
	}

	fmt.Fprintf(&sb, "总计: %d 项, 回归: %d 项\n\n", len(results), regressedCount)

	for _, r := range results {
		status := "PASS"
		if r.Regressed {
			status = "FAIL"
		}
		fmt.Fprintf(&sb, "[%s] %s\n", status, r.Name)

		if r.Previous != nil {
			fmt.Fprintf(&sb, "  变化: %+.1f%%\n", r.DeltaPct*100)
		}
		fmt.Fprintf(&sb, "  详情: %s\n", r.Detail)
		sb.WriteString("\n")
	}

	if regressedCount > 0 {
		sb.WriteString(fmt.Sprintf(">>> 检测到 %d 项性能回归! <<<\n", regressedCount))
	} else {
		sb.WriteString(">>> 未检测到性能回归 <<<\n")
	}

	return sb.String()
}

// HasRegression 检查结果中是否有回归
func HasRegression(results []RegressionResult) bool {
	for _, r := range results {
		if r.Regressed {
			return true
		}
	}
	return false
}
