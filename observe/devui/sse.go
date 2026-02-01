package devui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/everyday-items/toolkit/util/poolx"
)

// sseBufferPool SSE 消息缓冲区对象池
// 用于减少 JSON 编码时的内存分配
var sseBufferPool = poolx.NewBufferPool(1024)

// handleSSE 处理 SSE 事件流
// GET /events
//
// SSE 事件格式：
//
//	event: agent.start
//	data: {"id":"evt-1","type":"agent.start","data":{...}}
//
//	event: llm.stream
//	data: {"id":"evt-2","type":"llm.stream","data":{"content":"Hello"}}
func (h *handler) handleSSE(w http.ResponseWriter, r *http.Request) {
	// 检查是否支持 SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	// 订阅事件流
	eventCh, unsubscribe := h.devUI.collector.Subscribe()
	defer unsubscribe()

	// 创建取消上下文
	ctx := r.Context()

	// 发送初始连接消息
	h.sendSSEEvent(w, "connected", map[string]any{
		"message": "Connected to Hexagon Dev UI",
		"time":    time.Now().Format(time.RFC3339),
	})
	flusher.Flush()

	// 心跳定时器，保持连接活跃
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			// 客户端断开连接
			return

		case event, ok := <-eventCh:
			if !ok {
				// 通道已关闭
				return
			}

			// 发送事件
			if err := h.sendSSEEvent(w, string(event.Type), event); err != nil {
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			// 发送心跳
			if err := h.sendSSEComment(w, "heartbeat"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// sendSSEEvent 发送 SSE 事件
func (h *handler) sendSSEEvent(w http.ResponseWriter, eventType string, data any) error {
	// 从对象池获取缓冲区
	buf := sseBufferPool.Get()
	defer sseBufferPool.Put(buf)

	// 序列化数据
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 构建 SSE 消息
	// event: <eventType>
	// data: <jsonData>
	//
	buf = append(buf, "event: "...)
	buf = append(buf, eventType...)
	buf = append(buf, '\n')
	buf = append(buf, "data: "...)
	buf = append(buf, jsonData...)
	buf = append(buf, '\n', '\n')

	_, err = w.Write(buf)
	return err
}

// sendSSEComment 发送 SSE 注释（用于心跳）
func (h *handler) sendSSEComment(w http.ResponseWriter, comment string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	return err
}

// SSEClient SSE 客户端
// 用于测试和程序化订阅
type SSEClient struct {
	url       string
	eventCh   chan *Event
	errorCh   chan error
	cancel    context.CancelFunc
	connected bool
}

// NewSSEClient 创建 SSE 客户端
func NewSSEClient(url string) *SSEClient {
	return &SSEClient{
		url:     url,
		eventCh: make(chan *Event, 100),
		errorCh: make(chan error, 1),
	}
}

// Connect 连接到 SSE 服务器
func (c *SSEClient) Connect(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	c.connected = true

	// 启动读取协程
	go c.readLoop(ctx, resp)

	return nil
}

// readLoop 读取 SSE 事件
func (c *SSEClient) readLoop(ctx context.Context, resp *http.Response) {
	defer resp.Body.Close()
	defer close(c.eventCh)

	buf := make([]byte, 4096)
	var eventType string
	var dataLine string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := resp.Body.Read(buf)
		if err != nil {
			c.errorCh <- err
			return
		}

		// 简单解析 SSE 格式
		lines := string(buf[:n])
		for _, line := range splitLines(lines) {
			if len(line) == 0 {
				// 空行表示事件结束
				if dataLine != "" {
					event := &Event{}
					if err := json.Unmarshal([]byte(dataLine), event); err == nil {
						if eventType != "" && eventType != "connected" {
							event.Type = EventType(eventType)
						}
						select {
						case c.eventCh <- event:
						default:
							// 通道满了，丢弃事件
						}
					}
				}
				eventType = ""
				dataLine = ""
			} else if len(line) > 7 && line[:7] == "event: " {
				eventType = line[7:]
			} else if len(line) > 6 && line[:6] == "data: " {
				dataLine = line[6:]
			}
		}
	}
}

// splitLines 分割行
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Events 返回事件通道
func (c *SSEClient) Events() <-chan *Event {
	return c.eventCh
}

// Errors 返回错误通道
func (c *SSEClient) Errors() <-chan error {
	return c.errorCh
}

// Close 关闭连接
func (c *SSEClient) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	c.connected = false
}

// IsConnected 返回是否已连接
func (c *SSEClient) IsConnected() bool {
	return c.connected
}
