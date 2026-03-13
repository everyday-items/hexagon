package eventstream

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestStream_SubscribeAndPublish 测试订阅和发布
func TestStream_SubscribeAndPublish(t *testing.T) {
	s := New()
	defer s.Close()

	ch, unsub := s.Subscribe()
	defer unsub()

	s.Emit(EventAgentStart, "agent-1", map[string]any{"model": "gpt-4"})

	select {
	case event := <-ch:
		if event.Type != EventAgentStart {
			t.Errorf("事件类型错误: got %s, want %s", event.Type, EventAgentStart)
		}
		if event.AgentID != "agent-1" {
			t.Errorf("AgentID 错误: got %s", event.AgentID)
		}
		if event.Data["model"] != "gpt-4" {
			t.Errorf("Data 错误: %v", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("超时等待事件")
	}
}

// TestStream_MultipleSubscribers 测试多订阅者
func TestStream_MultipleSubscribers(t *testing.T) {
	s := New()
	defer s.Close()

	ch1, unsub1 := s.Subscribe()
	defer unsub1()
	ch2, unsub2 := s.Subscribe()
	defer unsub2()

	s.Emit(EventToolCall, "agent-1", nil)

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			if event.Type != EventToolCall {
				t.Errorf("事件类型错误: %s", event.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("超时等待事件")
		}
	}
}

// TestStream_Unsubscribe 测试取消订阅
func TestStream_Unsubscribe(t *testing.T) {
	s := New()
	defer s.Close()

	ch, unsub := s.Subscribe()
	if s.SubscriberCount() != 1 {
		t.Errorf("订阅者数量应为 1, got %d", s.SubscriberCount())
	}

	unsub()
	if s.SubscriberCount() != 0 {
		t.Errorf("取消订阅后数量应为 0, got %d", s.SubscriberCount())
	}

	// 通道应已关闭
	_, ok := <-ch
	if ok {
		t.Error("取消订阅后通道应已关闭")
	}
}

// TestStream_PublishSync 测试同步发布
func TestStream_PublishSync(t *testing.T) {
	s := New(WithBufferSize(1))
	defer s.Close()

	ch, unsub := s.Subscribe()
	defer unsub()

	ctx := context.Background()
	err := s.PublishSync(ctx, Event{Type: EventLLMRequest, AgentID: "a1"})
	if err != nil {
		t.Fatalf("同步发布失败: %v", err)
	}

	event := <-ch
	if event.Type != EventLLMRequest {
		t.Errorf("事件类型错误: %s", event.Type)
	}
}

// TestStream_PublishSyncCancelled 测试同步发布取消
func TestStream_PublishSyncCancelled(t *testing.T) {
	s := New(WithBufferSize(0)) // 无缓冲
	defer s.Close()

	_, unsub := s.Subscribe()
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := s.PublishSync(ctx, Event{Type: EventAgentEnd})
	if err != context.Canceled {
		t.Errorf("期望 context.Canceled, got %v", err)
	}
}

// TestStream_NonBlockingPublish 测试非阻塞发布（缓冲区满时不阻塞）
func TestStream_NonBlockingPublish(t *testing.T) {
	s := New(WithBufferSize(1))
	defer s.Close()

	_, unsub := s.Subscribe()
	defer unsub()

	// 发布超过缓冲区大小的事件，不应阻塞
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.Publish(Event{Type: EventMessage})
		}
		close(done)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("非阻塞发布不应阻塞")
	}
}

// TestStream_Close 测试关闭
func TestStream_Close(t *testing.T) {
	s := New()
	ch, _ := s.Subscribe()

	s.Close()

	// 关闭后通道应已关闭
	_, ok := <-ch
	if ok {
		t.Error("关闭后通道应已关闭")
	}

	// 关闭后发布不应 panic
	s.Publish(Event{Type: EventAgentStart})

	// 重复关闭不应 panic
	s.Close()
}

// TestStream_Concurrent 测试并发安全
func TestStream_Concurrent(t *testing.T) {
	s := New(WithBufferSize(100))
	defer s.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := s.Subscribe()
			defer unsub()

			for j := 0; j < 50; j++ {
				s.Emit(EventToolCall, "agent", nil)
			}

			// 读取一些事件
			for j := 0; j < 10; j++ {
				select {
				case <-ch:
				case <-time.After(100 * time.Millisecond):
				}
			}
		}()
	}
	wg.Wait()
}

// TestStream_EmitWithTrace 测试带追踪信息发布
func TestStream_EmitWithTrace(t *testing.T) {
	s := New()
	defer s.Close()

	ch, unsub := s.Subscribe()
	defer unsub()

	s.EmitWithTrace(EventLLMResponse, "agent-1", "trace-123", "span-456", nil)

	event := <-ch
	if event.TraceID != "trace-123" {
		t.Errorf("TraceID 错误: %s", event.TraceID)
	}
	if event.SpanID != "span-456" {
		t.Errorf("SpanID 错误: %s", event.SpanID)
	}
}
