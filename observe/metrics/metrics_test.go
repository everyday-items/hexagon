package metrics

import (
	"testing"
	"time"
)

func TestMemoryMetricsCreation(t *testing.T) {
	m := NewMemoryMetrics()

	if m == nil {
		t.Fatal("expected non-nil MemoryMetrics")
	}
}

func TestCounterOperations(t *testing.T) {
	m := NewMemoryMetrics()
	counter := m.Counter("test_counter")

	// 初始值应该是 0
	if counter.Value() != 0 {
		t.Errorf("expected initial value 0, got %f", counter.Value())
	}

	// 测试 Inc
	counter.Inc()
	if counter.Value() != 1 {
		t.Errorf("expected value 1 after Inc, got %f", counter.Value())
	}

	// 测试 Add
	counter.Add(5)
	if counter.Value() != 6 {
		t.Errorf("expected value 6 after Add(5), got %f", counter.Value())
	}

	// 测试浮点数 Add
	counter.Add(0.5)
	if counter.Value() != 6.5 {
		t.Errorf("expected value 6.5 after Add(0.5), got %f", counter.Value())
	}
}

func TestCounterWithTags(t *testing.T) {
	m := NewMemoryMetrics()

	// 不同 tags 应该创建不同的 counter
	counter1 := m.Counter("requests", "method", "GET")
	counter2 := m.Counter("requests", "method", "POST")
	counter3 := m.Counter("requests", "method", "GET") // 相同 tags

	counter1.Inc()
	counter2.Add(2)

	if counter1.Value() != 1 {
		t.Errorf("expected counter1 value 1, got %f", counter1.Value())
	}

	if counter2.Value() != 2 {
		t.Errorf("expected counter2 value 2, got %f", counter2.Value())
	}

	// counter3 应该和 counter1 是同一个
	if counter3.Value() != 1 {
		t.Errorf("expected counter3 (same as counter1) value 1, got %f", counter3.Value())
	}
}

func TestHistogramOperations(t *testing.T) {
	m := NewMemoryMetrics()
	histogram := m.Histogram("latency")

	// 初始值
	if histogram.Count() != 0 {
		t.Errorf("expected initial count 0, got %d", histogram.Count())
	}

	if histogram.Sum() != 0 {
		t.Errorf("expected initial sum 0, got %f", histogram.Sum())
	}

	// 测试 Observe
	histogram.Observe(10)
	histogram.Observe(20)
	histogram.Observe(30)

	if histogram.Count() != 3 {
		t.Errorf("expected count 3, got %d", histogram.Count())
	}

	if histogram.Sum() != 60 {
		t.Errorf("expected sum 60, got %f", histogram.Sum())
	}
}

func TestGaugeOperations(t *testing.T) {
	m := NewMemoryMetrics()
	gauge := m.Gauge("active_agents")

	// 初始值
	if gauge.Value() != 0 {
		t.Errorf("expected initial value 0, got %f", gauge.Value())
	}

	// 测试 Set
	gauge.Set(10)
	if gauge.Value() != 10 {
		t.Errorf("expected value 10 after Set, got %f", gauge.Value())
	}

	// 测试 Inc
	gauge.Inc()
	if gauge.Value() != 11 {
		t.Errorf("expected value 11 after Inc, got %f", gauge.Value())
	}

	// 测试 Dec
	gauge.Dec()
	if gauge.Value() != 10 {
		t.Errorf("expected value 10 after Dec, got %f", gauge.Value())
	}

	// 测试 Add
	gauge.Add(5)
	if gauge.Value() != 15 {
		t.Errorf("expected value 15 after Add(5), got %f", gauge.Value())
	}

	// 测试负数 Add (应该不会低于 0)
	gauge.Add(-20)
	if gauge.Value() != 0 {
		t.Errorf("expected value 0 after Add(-20), got %f", gauge.Value())
	}
}

func TestTimerOperations(t *testing.T) {
	m := NewMemoryMetrics()
	timer := m.Timer("request_duration")

	// 测试 ObserveDuration
	timer.ObserveDuration(100 * time.Millisecond)
	timer.ObserveDuration(200 * time.Millisecond)

	// 创建新计时器并停止
	tc := timer.NewTimer()
	time.Sleep(10 * time.Millisecond)
	tc.Stop()

	// 测试 Time 函数
	timer.Time(func() {
		time.Sleep(5 * time.Millisecond)
	})
}

func TestTimerContext(t *testing.T) {
	m := NewMemoryMetrics()
	timer := m.Timer("test_timer")

	tc := timer.NewTimer()
	time.Sleep(10 * time.Millisecond)
	duration := tc.Stop()

	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestMemoryMetricsSnapshot(t *testing.T) {
	m := NewMemoryMetrics()

	// 添加一些数据
	m.Counter("counter1").Add(10)
	m.Counter("counter2", "tag", "value").Add(5)
	m.Gauge("gauge1").Set(100)
	m.Histogram("histogram1").Observe(50)
	m.Timer("timer1").ObserveDuration(time.Second)

	snapshot := m.Snapshot()

	// 验证 counters
	if len(snapshot.Counters) != 2 {
		t.Errorf("expected 2 counters, got %d", len(snapshot.Counters))
	}

	if snapshot.Counters["counter1"] != 10 {
		t.Errorf("expected counter1 value 10, got %f", snapshot.Counters["counter1"])
	}

	// 验证 gauges
	if snapshot.Gauges["gauge1"] != 100 {
		t.Errorf("expected gauge1 value 100, got %f", snapshot.Gauges["gauge1"])
	}

	// 验证 histograms
	if snapshot.Histograms["histogram1"].Count != 1 {
		t.Errorf("expected histogram1 count 1, got %d", snapshot.Histograms["histogram1"].Count)
	}

	// 验证 timers
	if snapshot.Timers["timer1"].Count != 1 {
		t.Errorf("expected timer1 count 1, got %d", snapshot.Timers["timer1"].Count)
	}
}

func TestGlobalMetrics(t *testing.T) {
	// 保存原始全局指标
	original := GetGlobalMetrics()
	defer SetGlobalMetrics(original)

	// 设置新的全局指标
	newMetrics := NewMemoryMetrics()
	SetGlobalMetrics(newMetrics)

	// 验证
	if GetGlobalMetrics() != newMetrics {
		t.Error("expected new global metrics")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	// 保存原始全局指标
	original := GetGlobalMetrics()
	defer SetGlobalMetrics(original)

	// 设置新的全局指标
	m := NewMemoryMetrics()
	SetGlobalMetrics(m)

	// 测试便捷函数
	IncCounter("test_inc")
	AddCounter("test_add", 5)
	ObserveHistogram("test_hist", 100)
	SetGauge("test_gauge", 50)
	TimeDuration("test_time", time.Second)

	snapshot := m.Snapshot()

	if snapshot.Counters["test_inc"] != 1 {
		t.Errorf("expected test_inc 1, got %f", snapshot.Counters["test_inc"])
	}

	if snapshot.Counters["test_add"] != 5 {
		t.Errorf("expected test_add 5, got %f", snapshot.Counters["test_add"])
	}

	if snapshot.Gauges["test_gauge"] != 50 {
		t.Errorf("expected test_gauge 50, got %f", snapshot.Gauges["test_gauge"])
	}
}

func TestStartTimer(t *testing.T) {
	// 保存原始全局指标
	original := GetGlobalMetrics()
	defer SetGlobalMetrics(original)

	m := NewMemoryMetrics()
	SetGlobalMetrics(m)

	tc := StartTimer("test_start_timer")
	time.Sleep(5 * time.Millisecond)
	tc.Stop()

	snapshot := m.Snapshot()
	if snapshot.Timers["test_start_timer"].Count != 1 {
		t.Errorf("expected timer count 1, got %d", snapshot.Timers["test_start_timer"].Count)
	}
}

func TestBuildKey(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		expected string
	}{
		{"metric", nil, "metric"},
		{"metric", []string{}, "metric"},
		{"metric", []string{"a", "1"}, "metric,a=1"},
		{"metric", []string{"a", "1", "b", "2"}, "metric,a=1,b=2"},
	}

	for _, tc := range tests {
		result := buildKey(tc.name, tc.tags)
		if result != tc.expected {
			t.Errorf("buildKey(%q, %v) = %q, expected %q", tc.name, tc.tags, result, tc.expected)
		}
	}
}

func TestMetricConstants(t *testing.T) {
	// 验证预定义指标名称
	metrics := []string{
		MetricAgentRunsTotal,
		MetricAgentRunDuration,
		MetricAgentRunErrors,
		MetricAgentActiveCount,
		MetricLLMCallsTotal,
		MetricLLMCallDuration,
		MetricLLMCallErrors,
		MetricLLMPromptTokens,
		MetricLLMCompletionTokens,
		MetricToolCallsTotal,
		MetricToolCallDuration,
		MetricToolCallErrors,
		MetricRetrievalTotal,
		MetricRetrievalDuration,
		MetricRetrievalDocCount,
	}

	for _, m := range metrics {
		if m == "" {
			t.Errorf("metric constant should not be empty")
		}
	}
}

func TestHistogramSnapshot(t *testing.T) {
	snapshot := HistogramSnapshot{
		Count: 10,
		Sum:   100.5,
	}

	if snapshot.Count != 10 {
		t.Errorf("expected count 10, got %d", snapshot.Count)
	}

	if snapshot.Sum != 100.5 {
		t.Errorf("expected sum 100.5, got %f", snapshot.Sum)
	}
}

func TestMetricsSnapshot(t *testing.T) {
	snapshot := MetricsSnapshot{
		Counters:   map[string]float64{"c1": 1},
		Gauges:     map[string]float64{"g1": 2},
		Histograms: map[string]HistogramSnapshot{"h1": {Count: 3, Sum: 30}},
		Timers:     map[string]HistogramSnapshot{"t1": {Count: 4, Sum: 40}},
	}

	if snapshot.Counters["c1"] != 1 {
		t.Error("unexpected counter value")
	}

	if snapshot.Gauges["g1"] != 2 {
		t.Error("unexpected gauge value")
	}

	if snapshot.Histograms["h1"].Count != 3 {
		t.Error("unexpected histogram count")
	}

	if snapshot.Timers["t1"].Count != 4 {
		t.Error("unexpected timer count")
	}
}
