package bench

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRegressionDetector_SaveAndLoad 测试基线的保存和加载
func TestRegressionDetector_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baselines.json")

	detector := NewRegressionDetector(baselinePath)

	// 创建测试结果
	results := []*BenchmarkResult{
		{
			Name:        "test_fast",
			Iterations:  100,
			AvgDuration: 10 * time.Microsecond,
			P99:         50 * time.Microsecond,
			MemAllocs:   500,
			MemBytes:    10000,
		},
		{
			Name:        "test_slow",
			Iterations:  100,
			AvgDuration: 100 * time.Millisecond,
			P99:         200 * time.Millisecond,
			MemAllocs:   2000,
			MemBytes:    50000,
		},
	}

	// 保存基线
	if err := detector.SaveBaseline(results); err != nil {
		t.Fatalf("SaveBaseline 失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("基线文件不存在: %v", err)
	}

	// 加载基线
	detector2 := NewRegressionDetector(baselinePath)
	if err := detector2.LoadBaseline(); err != nil {
		t.Fatalf("LoadBaseline 失败: %v", err)
	}

	// 验证基线数据
	if len(detector2.baselines) != 2 {
		t.Errorf("期望 2 条基线, 实际 %d 条", len(detector2.baselines))
	}

	fast, ok := detector2.baselines["test_fast"]
	if !ok {
		t.Fatal("缺少 test_fast 基线")
	}
	if fast.AvgNs != float64(10*time.Microsecond) {
		t.Errorf("test_fast AvgNs 期望 %f, 实际 %f", float64(10*time.Microsecond), fast.AvgNs)
	}
	if fast.AllocsOp != 5 {
		t.Errorf("test_fast AllocsOp 期望 5, 实际 %d", fast.AllocsOp)
	}
}

// TestRegressionDetector_LoadNonExistent 测试加载不存在的基线文件
func TestRegressionDetector_LoadNonExistent(t *testing.T) {
	detector := NewRegressionDetector("/tmp/nonexistent_baseline_xyz.json")
	err := detector.LoadBaseline()
	if err != nil {
		t.Errorf("加载不存在的基线文件应该返回 nil, 实际返回: %v", err)
	}
	if len(detector.baselines) != 0 {
		t.Errorf("基线应为空, 实际 %d 条", len(detector.baselines))
	}
}

// TestRegressionDetector_CheckNoRegression 测试无回归的情况
func TestRegressionDetector_CheckNoRegression(t *testing.T) {
	detector := NewRegressionDetector("", WithThreshold(0.1))

	// 设置基线
	detector.baselines["test_op"] = &Baseline{
		Name:      "test_op",
		AvgNs:     10000,
		P99Ns:     50000,
		AllocsOp:  10,
		BytesOp:   1000,
		Timestamp: time.Now().Add(-24 * time.Hour),
	}

	// 当前结果略有波动但未超过阈值
	results := []*BenchmarkResult{
		{
			Name:        "test_op",
			Iterations:  100,
			AvgDuration: 10500 * time.Nanosecond, // 5% 变慢，低于 10% 阈值
			P99:         52000 * time.Nanosecond,
			MemAllocs:   1050, // 5% 增加
			MemBytes:    105000,
		},
	}

	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	if len(regressions) != 1 {
		t.Fatalf("期望 1 条结果, 实际 %d 条", len(regressions))
	}

	if regressions[0].Regressed {
		t.Errorf("5%% 变化不应判定为回归 (阈值 10%%)")
	}
}

// TestRegressionDetector_CheckWithRegression 测试发生回归的情况
func TestRegressionDetector_CheckWithRegression(t *testing.T) {
	detector := NewRegressionDetector("", WithThreshold(0.1))

	// 设置基线
	detector.baselines["test_slow"] = &Baseline{
		Name:      "test_slow",
		AvgNs:     10000,
		P99Ns:     50000,
		AllocsOp:  10,
		BytesOp:   1000,
		Timestamp: time.Now().Add(-24 * time.Hour),
	}

	// 当前结果超过阈值
	results := []*BenchmarkResult{
		{
			Name:        "test_slow",
			Iterations:  100,
			AvgDuration: 15000 * time.Nanosecond, // 50% 变慢
			P99:         80000 * time.Nanosecond, // 60% 变慢
			MemAllocs:   1500,                    // 50% 增加
			MemBytes:    200000,
		},
	}

	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	if len(regressions) != 1 {
		t.Fatalf("期望 1 条结果, 实际 %d 条", len(regressions))
	}

	r := regressions[0]
	if !r.Regressed {
		t.Error("50%% 变化应判定为回归")
	}
	if r.DeltaPct < 0.4 || r.DeltaPct > 0.6 {
		t.Errorf("DeltaPct 期望约 0.5, 实际 %f", r.DeltaPct)
	}
	if r.Detail == "" {
		t.Error("回归详情不应为空")
	}
}

// TestRegressionDetector_CheckNoBaseline 测试没有基线时的情况
func TestRegressionDetector_CheckNoBaseline(t *testing.T) {
	detector := NewRegressionDetector("", WithThreshold(0.1))

	results := []*BenchmarkResult{
		{
			Name:        "new_test",
			Iterations:  100,
			AvgDuration: 10 * time.Microsecond,
		},
	}

	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	if len(regressions) != 1 {
		t.Fatalf("期望 1 条结果, 实际 %d 条", len(regressions))
	}

	if regressions[0].Regressed {
		t.Error("没有基线时不应判定为回归")
	}
	if regressions[0].Previous != nil {
		t.Error("没有基线时 Previous 应为 nil")
	}
}

// TestRegressionDetector_CheckPerformanceImproved 测试性能提升的情况
func TestRegressionDetector_CheckPerformanceImproved(t *testing.T) {
	detector := NewRegressionDetector("", WithThreshold(0.1))

	detector.baselines["test_improved"] = &Baseline{
		Name:      "test_improved",
		AvgNs:     10000,
		P99Ns:     50000,
		AllocsOp:  10,
		BytesOp:   1000,
		Timestamp: time.Now().Add(-24 * time.Hour),
	}

	// 性能提升 30%
	results := []*BenchmarkResult{
		{
			Name:        "test_improved",
			Iterations:  100,
			AvgDuration: 7000 * time.Nanosecond,
			P99:         35000 * time.Nanosecond,
			MemAllocs:   700,
			MemBytes:    70000,
		},
	}

	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	r := regressions[0]
	if r.Regressed {
		t.Error("性能提升不应判定为回归")
	}
	if r.DeltaPct >= 0 {
		t.Errorf("性能提升的 DeltaPct 应为负数, 实际 %f", r.DeltaPct)
	}
}

// TestRegressionDetector_Report 测试报告生成
func TestRegressionDetector_Report(t *testing.T) {
	detector := NewRegressionDetector("baselines.json", WithThreshold(0.1))

	results := []RegressionResult{
		{
			Name:      "test_pass",
			Regressed: false,
			Current:   &Baseline{AvgNs: 10000},
			Previous:  &Baseline{AvgNs: 10000},
			DeltaPct:  0.0,
			Detail:    "性能稳定",
		},
		{
			Name:      "test_fail",
			Regressed: true,
			Current:   &Baseline{AvgNs: 20000},
			Previous:  &Baseline{AvgNs: 10000},
			DeltaPct:  1.0,
			Detail:    "平均耗时回归 100.0%",
		},
	}

	report := detector.Report(results)

	// 验证报告包含关键信息
	if report == "" {
		t.Fatal("报告不应为空")
	}

	checks := []string{
		"性能回归检测报告",
		"10%",
		"test_pass",
		"test_fail",
		"PASS",
		"FAIL",
		"回归: 1 项",
	}
	for _, check := range checks {
		if !contains(report, check) {
			t.Errorf("报告应包含 %q", check)
		}
	}
}

// TestRegressionDetector_HasRegression 测试 HasRegression 辅助函数
func TestRegressionDetector_HasRegression(t *testing.T) {
	noRegression := []RegressionResult{
		{Regressed: false},
		{Regressed: false},
	}
	if HasRegression(noRegression) {
		t.Error("全部通过时 HasRegression 应返回 false")
	}

	withRegression := []RegressionResult{
		{Regressed: false},
		{Regressed: true},
	}
	if !HasRegression(withRegression) {
		t.Error("存在回归时 HasRegression 应返回 true")
	}
}

// TestRegressionDetector_CustomThresholds 测试自定义分配阈值
func TestRegressionDetector_CustomThresholds(t *testing.T) {
	detector := NewRegressionDetector("",
		WithThreshold(0.5),        // 耗时回归阈值 50%
		WithAllocsThreshold(0.05), // 分配次数阈值 5%
	)

	detector.baselines["test_allocs"] = &Baseline{
		Name:     "test_allocs",
		AvgNs:    10000,
		AllocsOp: 100,
		BytesOp:  10000,
	}

	// 耗时只增加 10%（低于 50% 阈值），但分配增加 10%（高于 5% 阈值）
	results := []*BenchmarkResult{
		{
			Name:        "test_allocs",
			Iterations:  100,
			AvgDuration: 11000 * time.Nanosecond,
			MemAllocs:   11000, // 110 per op, 10% 增加
			MemBytes:    1000000,
		},
	}

	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	if !regressions[0].Regressed {
		t.Error("分配次数超过阈值应判定为回归")
	}
}

// TestRegressionDetector_NilResults 测试空结果的处理
func TestRegressionDetector_NilResults(t *testing.T) {
	detector := NewRegressionDetector("")

	results := []*BenchmarkResult{nil, nil}
	regressions, err := detector.Check(results)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}
	if len(regressions) != 0 {
		t.Errorf("nil 结果应被跳过, 实际 %d 条", len(regressions))
	}
}

// TestRegressionDetector_SaveCreatesDir 测试保存基线时自动创建目录
func TestRegressionDetector_SaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "sub", "dir", "baselines.json")

	detector := NewRegressionDetector(baselinePath)

	results := []*BenchmarkResult{
		{
			Name:        "test",
			Iterations:  10,
			AvgDuration: time.Microsecond,
		},
	}

	if err := detector.SaveBaseline(results); err != nil {
		t.Fatalf("SaveBaseline 失败: %v", err)
	}

	if _, err := os.Stat(baselinePath); err != nil {
		t.Errorf("基线文件应该被创建: %v", err)
	}
}

// TestRegressionDetector_SaveAndCheckRoundtrip 测试保存-加载-检查的完整流程
func TestRegressionDetector_SaveAndCheckRoundtrip(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baselines.json")

	// 第一次运行：保存基线
	detector1 := NewRegressionDetector(baselinePath, WithThreshold(0.1))
	baseline := []*BenchmarkResult{
		{
			Name:        "roundtrip_test",
			Iterations:  100,
			AvgDuration: 10 * time.Microsecond,
			P99:         50 * time.Microsecond,
			MemAllocs:   1000,
			MemBytes:    50000,
		},
	}
	if err := detector1.SaveBaseline(baseline); err != nil {
		t.Fatalf("SaveBaseline 失败: %v", err)
	}

	// 第二次运行：加载基线并检查（性能变差 20%）
	detector2 := NewRegressionDetector(baselinePath, WithThreshold(0.1))
	if err := detector2.LoadBaseline(); err != nil {
		t.Fatalf("LoadBaseline 失败: %v", err)
	}

	current := []*BenchmarkResult{
		{
			Name:        "roundtrip_test",
			Iterations:  100,
			AvgDuration: 12 * time.Microsecond, // 20% 变慢
			P99:         60 * time.Microsecond,
			MemAllocs:   1200,
			MemBytes:    60000,
		},
	}

	regressions, err := detector2.Check(current)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}

	if len(regressions) != 1 {
		t.Fatalf("期望 1 条结果, 实际 %d 条", len(regressions))
	}

	if !regressions[0].Regressed {
		t.Error("20%% 变慢应判定为回归 (阈值 10%%)")
	}
}

// contains 检查字符串中是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
