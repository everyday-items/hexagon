package hooks

import (
	"context"
	"sync/atomic"
	"testing"
)

// ============== 流感知 Hook 测试 ==============

// streamAwareHook 同时实现 RunHook 和 StreamHook
type streamAwareHook struct {
	name             string
	enabled          bool
	timings          Timing
	startCount       int32
	endCount         int32
	errorCount       int32
	streamStartCount int32
	streamEndCount   int32
	lastStreamStart  *RunStreamStartEvent
	lastStreamEnd    *RunStreamEndEvent
}

func (h *streamAwareHook) Name() string  { return h.name }
func (h *streamAwareHook) Enabled() bool { return h.enabled }
func (h *streamAwareHook) Timings() Timing {
	if h.timings == 0 {
		return TimingAll
	}
	return h.timings
}

func (h *streamAwareHook) OnStart(_ context.Context, _ *RunStartEvent) error {
	atomic.AddInt32(&h.startCount, 1)
	return nil
}

func (h *streamAwareHook) OnEnd(_ context.Context, _ *RunEndEvent) error {
	atomic.AddInt32(&h.endCount, 1)
	return nil
}

func (h *streamAwareHook) OnError(_ context.Context, _ *ErrorEvent) error {
	atomic.AddInt32(&h.errorCount, 1)
	return nil
}

func (h *streamAwareHook) OnStreamStart(_ context.Context, event *RunStreamStartEvent) error {
	atomic.AddInt32(&h.streamStartCount, 1)
	h.lastStreamStart = event
	return nil
}

func (h *streamAwareHook) OnStreamEnd(_ context.Context, event *RunStreamEndEvent) error {
	atomic.AddInt32(&h.streamEndCount, 1)
	h.lastStreamEnd = event
	return nil
}

// 验证接口实现
var _ RunHook = (*streamAwareHook)(nil)
var _ StreamHook = (*streamAwareHook)(nil)
var _ TimingChecker = (*streamAwareHook)(nil)

func TestStreamHook_TriggerStreamStart(t *testing.T) {
	m := NewManager()
	hook := &streamAwareHook{name: "stream-hook", enabled: true}
	m.RegisterRunHook(hook)

	event := &RunStreamStartEvent{
		RunID:    "run-1",
		AgentID:  "agent-1",
		Input:    "hello",
		IsStream: true,
		Metadata: map[string]any{"source": "test"},
	}

	err := m.TriggerStreamStart(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hook.streamStartCount != 1 {
		t.Fatalf("expected streamStartCount=1, got %d", hook.streamStartCount)
	}
	if hook.lastStreamStart.RunID != "run-1" {
		t.Fatalf("expected RunID 'run-1', got %q", hook.lastStreamStart.RunID)
	}
	if !hook.lastStreamStart.IsStream {
		t.Fatal("expected IsStream=true")
	}
}

func TestStreamHook_TriggerStreamEnd(t *testing.T) {
	m := NewManager()
	hook := &streamAwareHook{name: "stream-hook", enabled: true}
	m.RegisterRunHook(hook)

	event := &RunStreamEndEvent{
		RunID:      "run-1",
		AgentID:    "agent-1",
		ChunkCount: 42,
		Duration:   1500,
		Metadata:   map[string]any{"tokens": 100},
	}

	err := m.TriggerStreamEnd(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hook.streamEndCount != 1 {
		t.Fatalf("expected streamEndCount=1, got %d", hook.streamEndCount)
	}
	if hook.lastStreamEnd.ChunkCount != 42 {
		t.Fatalf("expected ChunkCount=42, got %d", hook.lastStreamEnd.ChunkCount)
	}
	if hook.lastStreamEnd.Duration != 1500 {
		t.Fatalf("expected Duration=1500, got %d", hook.lastStreamEnd.Duration)
	}
}

func TestStreamHook_NonStreamHookIgnored(t *testing.T) {
	// 普通 RunHook（不实现 StreamHook）应被忽略
	m := NewManager()
	hook := &mockRunHook{name: "plain-hook", enabled: true}
	m.RegisterRunHook(hook)

	// 触发流事件不应报错
	err := m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{
		RunID:    "run-1",
		IsStream: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.TriggerStreamEnd(context.Background(), &RunStreamEndEvent{
		RunID:      "run-1",
		ChunkCount: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 普通 hook 不应被调用任何方法
	if hook.startCount != 0 || hook.endCount != 0 {
		t.Fatal("plain hook should not have been called")
	}
}

func TestStreamHook_DisabledHookIgnored(t *testing.T) {
	m := NewManager()
	hook := &streamAwareHook{name: "disabled", enabled: false}
	m.RegisterRunHook(hook)

	err := m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hook.streamStartCount != 0 {
		t.Fatal("disabled hook should not be called")
	}
}

func TestStreamHook_TimingFilter(t *testing.T) {
	m := NewManager()

	// 只关心 StreamStart，不关心 StreamEnd
	hook := &streamAwareHook{
		name:    "start-only",
		enabled: true,
		timings: TimingRunStreamStart,
	}
	m.RegisterRunHook(hook)

	err := m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.streamStartCount != 1 {
		t.Fatalf("expected streamStartCount=1, got %d", hook.streamStartCount)
	}

	err = m.TriggerStreamEnd(context.Background(), &RunStreamEndEvent{RunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.streamEndCount != 0 {
		t.Fatal("hook should not receive stream end (timing filter)")
	}
}

func TestStreamHook_TimingRunStreamAll(t *testing.T) {
	m := NewManager()
	hook := &streamAwareHook{
		name:    "all-stream",
		enabled: true,
		timings: TimingRunStreamAll,
	}
	m.RegisterRunHook(hook)

	_ = m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})
	_ = m.TriggerStreamEnd(context.Background(), &RunStreamEndEvent{RunID: "run-1"})

	if hook.streamStartCount != 1 {
		t.Fatalf("expected streamStartCount=1, got %d", hook.streamStartCount)
	}
	if hook.streamEndCount != 1 {
		t.Fatalf("expected streamEndCount=1, got %d", hook.streamEndCount)
	}
}

func TestStreamHook_MixedWithRunHook(t *testing.T) {
	// StreamHook 同时接收 Run 事件和 Stream 事件
	m := NewManager()
	hook := &streamAwareHook{name: "mixed", enabled: true}
	m.RegisterRunHook(hook)

	// 触发 Run 事件
	_ = m.TriggerRunStart(context.Background(), &RunStartEvent{RunID: "run-1"})
	_ = m.TriggerRunEnd(context.Background(), &RunEndEvent{RunID: "run-1"})

	// 触发 Stream 事件
	_ = m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})
	_ = m.TriggerStreamEnd(context.Background(), &RunStreamEndEvent{RunID: "run-1"})

	if hook.startCount != 1 {
		t.Fatalf("expected run startCount=1, got %d", hook.startCount)
	}
	if hook.endCount != 1 {
		t.Fatalf("expected run endCount=1, got %d", hook.endCount)
	}
	if hook.streamStartCount != 1 {
		t.Fatalf("expected streamStartCount=1, got %d", hook.streamStartCount)
	}
	if hook.streamEndCount != 1 {
		t.Fatalf("expected streamEndCount=1, got %d", hook.streamEndCount)
	}
}

func TestStreamHook_NoHooksRegistered(t *testing.T) {
	m := NewManager()

	// 空管理器不应报错
	err := m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.TriggerStreamEnd(context.Background(), &RunStreamEndEvent{RunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamHook_MultipleHooks(t *testing.T) {
	m := NewManager()
	hook1 := &streamAwareHook{name: "hook1", enabled: true}
	hook2 := &streamAwareHook{name: "hook2", enabled: true}
	m.RegisterRunHook(hook1)
	m.RegisterRunHook(hook2)

	_ = m.TriggerStreamStart(context.Background(), &RunStreamStartEvent{RunID: "run-1"})

	if hook1.streamStartCount != 1 || hook2.streamStartCount != 1 {
		t.Fatalf("expected both hooks called, got hook1=%d hook2=%d",
			hook1.streamStartCount, hook2.streamStartCount)
	}
}

// ============== Timing 常量测试 ==============

func TestTimingRunStreamAll(t *testing.T) {
	all := TimingRunStreamAll
	if !all.Has(TimingRunStreamStart) {
		t.Fatal("TimingRunStreamAll should include TimingRunStreamStart")
	}
	if !all.Has(TimingRunStreamEnd) {
		t.Fatal("TimingRunStreamAll should include TimingRunStreamEnd")
	}
}

func TestTimingAll_IncludesStreamTimings(t *testing.T) {
	if !TimingAll.Has(TimingRunStreamStart) {
		t.Fatal("TimingAll should include TimingRunStreamStart")
	}
	if !TimingAll.Has(TimingRunStreamEnd) {
		t.Fatal("TimingAll should include TimingRunStreamEnd")
	}
}

func TestTimingString_StreamTimings(t *testing.T) {
	s := TimingRunStreamStart.String()
	if s != "run_stream_start" {
		t.Fatalf("expected 'run_stream_start', got %q", s)
	}
	s = TimingRunStreamEnd.String()
	if s != "run_stream_end" {
		t.Fatalf("expected 'run_stream_end', got %q", s)
	}
	s = TimingRunStreamAll.String()
	if s != "run_stream_start|run_stream_end" {
		t.Fatalf("expected 'run_stream_start|run_stream_end', got %q", s)
	}
}

// ============== 事件结构体测试 ==============

func TestRunStreamStartEvent_Fields(t *testing.T) {
	event := &RunStreamStartEvent{
		RunID:    "run-1",
		AgentID:  "agent-1",
		Input:    map[string]any{"query": "test"},
		IsStream: true,
		Metadata: map[string]any{"key": "value"},
	}

	if event.RunID != "run-1" {
		t.Fatalf("expected RunID 'run-1', got %q", event.RunID)
	}
	if !event.IsStream {
		t.Fatal("expected IsStream=true")
	}
}

func TestRunStreamEndEvent_Fields(t *testing.T) {
	event := &RunStreamEndEvent{
		RunID:      "run-1",
		AgentID:    "agent-1",
		ChunkCount: 100,
		Duration:   2500,
		Metadata:   map[string]any{"model": "gpt-4"},
	}

	if event.ChunkCount != 100 {
		t.Fatalf("expected ChunkCount=100, got %d", event.ChunkCount)
	}
	if event.Duration != 2500 {
		t.Fatalf("expected Duration=2500, got %d", event.Duration)
	}
}
