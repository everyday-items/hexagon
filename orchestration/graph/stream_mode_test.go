package graph

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestStreamChannel_Basic 测试 StreamChannel 基本功能
func TestStreamChannel_Basic(t *testing.T) {
	ch := NewStreamChannel(10)
	defer ch.Close()

	// 发射事件
	ch.Emit(StreamModeEvent{
		Mode: StreamModeValues,
		Type: EventStateSnapshot,
		Node: "test-node",
		Data: "test-data",
	})

	// 接收事件
	select {
	case event := <-ch.Events():
		if event.Node != "test-node" {
			t.Errorf("expected node 'test-node', got '%s'", event.Node)
		}
		if event.Data != "test-data" {
			t.Errorf("expected data 'test-data', got '%v'", event.Data)
		}
		if event.Type != EventStateSnapshot {
			t.Errorf("expected type EventStateSnapshot, got '%s'", event.Type)
		}
		if event.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

// TestStreamChannel_Close 测试关闭后不再发射事件
func TestStreamChannel_Close(t *testing.T) {
	ch := NewStreamChannel(10)

	ch.Close()

	// 关闭后发射事件应该被忽略
	ch.Emit(StreamModeEvent{
		Mode: StreamModeValues,
		Type: EventStateSnapshot,
		Node: "test-node",
		Data: "test-data",
	})

	// 通道应该已关闭
	select {
	case _, ok := <-ch.Events():
		if ok {
			t.Error("expected closed channel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}

	// 多次关闭不应该 panic
	ch.Close()
}

// TestStreamChannel_BufferFull 测试缓冲区满时丢弃事件
func TestStreamChannel_BufferFull(t *testing.T) {
	bufferSize := 5
	ch := NewStreamChannel(bufferSize)
	defer ch.Close()

	// 填满缓冲区
	for i := 0; i < bufferSize; i++ {
		ch.Emit(StreamModeEvent{
			Mode: StreamModeValues,
			Type: EventStateSnapshot,
			Node: "test-node",
			Data: i,
		})
	}

	// 再发射一个事件，应该被丢弃（不会阻塞）
	done := make(chan struct{})
	go func() {
		ch.Emit(StreamModeEvent{
			Mode: StreamModeValues,
			Type: EventStateSnapshot,
			Node: "test-node",
			Data: "extra",
		})
		close(done)
	}()

	select {
	case <-done:
		// 成功，没有阻塞
	case <-time.After(100 * time.Millisecond):
		t.Error("Emit blocked when buffer full")
	}

	// 读取缓冲区中的事件
	receivedCount := 0
	timeout := time.After(100 * time.Millisecond)
	for i := 0; i < bufferSize; i++ {
		select {
		case <-ch.Events():
			receivedCount++
		case <-timeout:
			break
		}
	}

	if receivedCount != bufferSize {
		t.Errorf("expected %d events, got %d", bufferSize, receivedCount)
	}
}

// TestStreamChannel_Done 测试 Done 通道
func TestStreamChannel_Done(t *testing.T) {
	ch := NewStreamChannel(10)

	// Done 通道应该未关闭
	select {
	case <-ch.Done():
		t.Error("expected Done channel to be open")
	default:
		// 正常
	}

	ch.Close()

	// 关闭后 Done 通道应该关闭
	select {
	case <-ch.Done():
		// 正常
	case <-time.After(100 * time.Millisecond):
		t.Error("expected Done channel to be closed")
	}
}

// TestStreamRun_Values 测试 StreamModeValues 模式
func TestStreamRun_Values(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 1)
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 2)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{}, WithStreamMode(StreamModeValues))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snapshotCount := 0
	for event := range stream.Events() {
		if event.Type == EventStateSnapshot {
			snapshotCount++
			if event.Mode != StreamModeValues {
				t.Errorf("expected mode StreamModeValues, got %v", event.Mode)
			}
		}
	}

	// 应该有 2 个状态快照（step1 和 step2）
	if snapshotCount != 2 {
		t.Errorf("expected 2 snapshots, got %d", snapshotCount)
	}
}

// TestStreamRun_Updates 测试 StreamModeUpdates 模式
func TestStreamRun_Updates(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("key1", "value1")
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("key2", "value2")
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{}, WithStreamMode(StreamModeUpdates))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updateCount := 0
	for event := range stream.Events() {
		if event.Type == EventStateUpdate {
			updateCount++
			if event.Mode != StreamModeUpdates {
				t.Errorf("expected mode StreamModeUpdates, got %v", event.Mode)
			}
		}
	}

	// 应该有 2 个状态更新（step1 和 step2）
	if updateCount != 2 {
		t.Errorf("expected 2 updates, got %d", updateCount)
	}
}

// TestStreamRun_Debug 测试 StreamModeDebug 模式
func TestStreamRun_Debug(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 1)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{}, WithStreamMode(StreamModeDebug))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eventTypes := make(map[StreamModeEventType]int)
	for event := range stream.Events() {
		if event.Mode == StreamModeDebug {
			eventTypes[event.Type]++
		}
	}

	// 应该有 GraphStart, NodeStart, NodeEnd, GraphEnd 事件
	expectedEvents := []StreamModeEventType{
		EventGraphStart,
		EventNodeStart,
		EventNodeEnd,
		EventGraphEnd,
	}

	for _, expectedType := range expectedEvents {
		if eventTypes[expectedType] == 0 {
			t.Errorf("missing expected event type: %s", expectedType)
		}
	}

	if eventTypes[EventGraphStart] != 1 {
		t.Errorf("expected 1 GraphStart event, got %d", eventTypes[EventGraphStart])
	}

	if eventTypes[EventGraphEnd] != 1 {
		t.Errorf("expected 1 GraphEnd event, got %d", eventTypes[EventGraphEnd])
	}
}

// TestStreamRun_Filter 测试事件过滤器
func TestStreamRun_Filter(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 1)
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 2)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	// 只允许 step1 的事件通过
	filter := func(event StreamModeEvent) bool {
		return event.Node == "step1"
	}

	stream, err := g.StreamRun(ctx, MapState{},
		WithStreamMode(StreamModeValues),
		WithStreamFilter(filter),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step1Count := 0
	step2Count := 0
	for event := range stream.Events() {
		if event.Type == EventStateSnapshot {
			if event.Node == "step1" {
				step1Count++
			}
			if event.Node == "step2" {
				step2Count++
			}
		}
	}

	if step1Count != 1 {
		t.Errorf("expected 1 step1 event, got %d", step1Count)
	}

	if step2Count != 0 {
		t.Errorf("expected 0 step2 events (filtered out), got %d", step2Count)
	}
}

// TestStreamRun_MultiMode 测试同时使用多种流模式
func TestStreamRun_MultiMode(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 1)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{},
		WithStreamMode(StreamModeValues, StreamModeUpdates, StreamModeDebug),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	modes := make(map[StreamMode]bool)
	for event := range stream.Events() {
		modes[event.Mode] = true
	}

	// 应该包含所有三种模式的事件
	expectedModes := []StreamMode{StreamModeValues, StreamModeUpdates, StreamModeDebug}
	for _, mode := range expectedModes {
		if !modes[mode] {
			t.Errorf("missing events for mode %v", mode)
		}
	}
}

// TestWithStreamChannel 测试 WithStreamChannel 和 EmitCustomEvent
func TestWithStreamChannel(t *testing.T) {
	ch := NewStreamChannel(10)
	defer ch.Close()

	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			// 发射自定义事件
			EmitCustomEvent(ctx, "custom-event", map[string]any{
				"message": "hello from step1",
			})
			s.Set("counter", 1)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := WithStreamChannel(context.Background(), ch)

	// 在另一个 goroutine 中执行图
	go func() {
		_, _ = g.Run(ctx, MapState{})
	}()

	// 接收自定义事件
	timeout := time.After(1 * time.Second)
	customEventReceived := false

	for {
		select {
		case event := <-ch.Events():
			if event.Type == EventCustom {
				customEventReceived = true
				if dataMap, ok := event.Data.(map[string]any); ok {
					if name, ok := dataMap["name"].(string); ok && name == "custom-event" {
						if data, ok := dataMap["data"].(map[string]any); ok {
							if msg, ok := data["message"].(string); ok && msg == "hello from step1" {
								return
							}
						}
					}
				}
			}
		case <-timeout:
			if !customEventReceived {
				t.Error("timeout waiting for custom event")
			}
			return
		}
	}
}

// TestNewStreamChannel_DefaultBuffer 测试默认缓冲区大小
func TestNewStreamChannel_DefaultBuffer(t *testing.T) {
	// 测试 bufferSize <= 0 时使用默认值 100
	ch := NewStreamChannel(0)
	defer ch.Close()

	// 发射 100 个事件（默认缓冲区大小）
	for i := 0; i < 100; i++ {
		ch.Emit(StreamModeEvent{
			Mode: StreamModeValues,
			Type: EventStateSnapshot,
			Node: "test-node",
			Data: i,
		})
	}

	// 应该能成功发射所有事件
	receivedCount := 0
	timeout := time.After(100 * time.Millisecond)
	for i := 0; i < 100; i++ {
		select {
		case <-ch.Events():
			receivedCount++
		case <-timeout:
			break
		}
	}

	if receivedCount != 100 {
		t.Errorf("expected 100 events, got %d", receivedCount)
	}
}

// TestStreamRun_WithBufferSize 测试自定义缓冲区大小
func TestStreamRun_WithBufferSize(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			s.Set("counter", 1)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{},
		WithStreamMode(StreamModeValues),
		WithStreamBufferSize(50),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证能正常接收事件
	eventReceived := false
	for event := range stream.Events() {
		if event.Type == EventStateSnapshot {
			eventReceived = true
		}
	}

	if !eventReceived {
		t.Error("expected to receive at least one event")
	}
}

// TestEmitCustomEvent_NoChannel 测试没有 StreamChannel 的情况
func TestEmitCustomEvent_NoChannel(t *testing.T) {
	ctx := context.Background()

	// 没有 StreamChannel 时，EmitCustomEvent 应该不 panic
	EmitCustomEvent(ctx, "test-event", "test-data")

	// 如果没有 panic，测试通过
}

// TestStreamRun_ContextCancel 测试上下文取消
func TestStreamRun_ContextCancel(t *testing.T) {
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			time.Sleep(100 * time.Millisecond)
			s.Set("counter", 1)
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	stream, err := g.StreamRun(ctx, MapState{}, WithStreamMode(StreamModeDebug))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该收到错误事件
	errorEventReceived := false
	for event := range stream.Events() {
		if event.Type == EventNodeError {
			errorEventReceived = true
			break
		}
	}

	if !errorEventReceived {
		t.Error("expected error event due to context cancellation")
	}
}

// TestStreamRun_NodeError 测试节点执行错误
func TestStreamRun_NodeError(t *testing.T) {
	expectedErr := fmt.Errorf("node error")
	g, err := NewGraph[MapState]("test-graph").
		AddNode("step1", func(ctx context.Context, s MapState) (MapState, error) {
			return s, expectedErr
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	stream, err := g.StreamRun(ctx, MapState{}, WithStreamMode(StreamModeDebug))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errorEventReceived := false
	for event := range stream.Events() {
		if event.Type == EventNodeError {
			errorEventReceived = true
			if dataMap, ok := event.Data.(map[string]any); ok {
				if errMsg, ok := dataMap["error"].(string); ok {
					if errMsg != expectedErr.Error() {
						t.Errorf("expected error message '%s', got '%s'", expectedErr.Error(), errMsg)
					}
				}
			}
		}
	}

	if !errorEventReceived {
		t.Error("expected error event")
	}
}
