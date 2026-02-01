package devui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/observe/tracer"
)

// statusCodeToString 将 StatusCode 转换为字符串
func statusCodeToString(code tracer.StatusCode) string {
	switch code {
	case tracer.StatusCodeOK:
		return "ok"
	case tracer.StatusCodeError:
		return "error"
	default:
		return "unset"
	}
}

//go:embed static/*
var staticFS embed.FS

// handler HTTP 请求处理器
type handler struct {
	devUI *DevUI
}

// newHandler 创建处理器
func newHandler(d *DevUI) *handler {
	return &handler{devUI: d}
}

// response 通用响应结构
type response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeSuccess 写入成功响应
func writeSuccess(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, response{
		Success: true,
		Data:    data,
	})
}

// writeError 写入错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, response{
		Success: false,
		Error:   message,
	})
}

// handleEvents 获取事件列表
// GET /api/events?limit=100&offset=0&type=agent.start
func (h *handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()

	// 解析分页参数
	limit := 100
	if l := query.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	// 获取最近的事件
	events := h.devUI.collector.Events().GetRecent(limit)

	// 过滤事件类型
	if typeFilter := query.Get("type"); typeFilter != "" {
		filtered := make([]*Event, 0, len(events))
		for _, e := range events {
			if string(e.Type) == typeFilter {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	writeSuccess(w, map[string]any{
		"events": events,
		"total":  len(events),
	})
}

// handleEventByID 获取单个事件详情
// GET /api/events/{id}
func (h *handler) handleEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析事件 ID
	path := strings.TrimPrefix(r.URL.Path, h.devUI.options.APIPrefix+"/events/")
	eventID := strings.TrimSuffix(path, "/")

	if eventID == "" {
		writeError(w, http.StatusBadRequest, "event id required")
		return
	}

	// 在缓冲区中查找事件
	events := h.devUI.collector.Events().GetAll()
	for _, e := range events {
		if e.ID == eventID {
			writeSuccess(w, e)
			return
		}
	}

	writeError(w, http.StatusNotFound, "event not found")
}

// handleTraces 获取 Trace 列表
// GET /api/traces?limit=100
func (h *handler) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()

	// 解析分页参数
	limit := 100
	if l := query.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	// 获取 Span 数据
	tracer := h.devUI.collector.Tracer()
	spans := tracer.Export()

	// 按 TraceID 分组
	traceMap := make(map[string][]*traceSpan)
	for _, s := range spans {
		traceID := s.TraceID
		if traceID == "" {
			continue
		}
		traceMap[traceID] = append(traceMap[traceID], &traceSpan{
			SpanID:   s.SpanID,
			ParentID: s.ParentID,
			Name:     s.Name,
			Kind:     s.Kind,
			Start:    s.StartTime,
			End:      s.EndTime,
			Duration: s.Duration.Milliseconds(),
			Status:   statusCodeToString(s.Status.Code),
			Input:    s.Input,
			Output:   s.Output,
			Attrs:    s.Attributes,
		})
	}

	// 转换为列表
	traces := make([]traceInfo, 0, len(traceMap))
	for traceID, ss := range traceMap {
		if len(traces) >= limit {
			break
		}

		// 计算 Trace 总时长
		var minStart, maxEnd time.Time
		for _, s := range ss {
			if minStart.IsZero() || s.Start.Before(minStart) {
				minStart = s.Start
			}
			if maxEnd.IsZero() || s.End.After(maxEnd) {
				maxEnd = s.End
			}
		}

		traces = append(traces, traceInfo{
			TraceID:   traceID,
			SpanCount: len(ss),
			Start:     minStart,
			Duration:  maxEnd.Sub(minStart).Milliseconds(),
		})
	}

	writeSuccess(w, map[string]any{
		"traces": traces,
		"total":  len(traces),
	})
}

// traceInfo Trace 信息
type traceInfo struct {
	TraceID   string    `json:"trace_id"`
	SpanCount int       `json:"span_count"`
	Start     time.Time `json:"start"`
	Duration  int64     `json:"duration_ms"`
}

// traceSpan Span 信息
type traceSpan struct {
	SpanID   string         `json:"span_id"`
	ParentID string         `json:"parent_id,omitempty"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Start    time.Time      `json:"start"`
	End      time.Time      `json:"end"`
	Duration int64          `json:"duration_ms"`
	Status   string         `json:"status"`
	Input    any            `json:"input,omitempty"`
	Output   any            `json:"output,omitempty"`
	Attrs    map[string]any `json:"attributes,omitempty"`
}

// handleTraceByID 获取单个 Trace 详情
// GET /api/traces/{traceId}
func (h *handler) handleTraceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析 Trace ID
	path := strings.TrimPrefix(r.URL.Path, h.devUI.options.APIPrefix+"/traces/")
	traceID := strings.TrimSuffix(path, "/")

	if traceID == "" {
		writeError(w, http.StatusBadRequest, "trace id required")
		return
	}

	// 获取该 Trace 的所有 Span
	tracer := h.devUI.collector.Tracer()
	spans := tracer.Export()

	traceSpans := make([]*traceSpan, 0)
	for _, s := range spans {
		if s.TraceID == traceID {
			traceSpans = append(traceSpans, &traceSpan{
				SpanID:   s.SpanID,
				ParentID: s.ParentID,
				Name:     s.Name,
				Kind:     s.Kind,
				Start:    s.StartTime,
				End:      s.EndTime,
				Duration: s.Duration.Milliseconds(),
				Status:   statusCodeToString(s.Status.Code),
				Input:    s.Input,
				Output:   s.Output,
				Attrs:    s.Attributes,
			})
		}
	}

	if len(traceSpans) == 0 {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	writeSuccess(w, map[string]any{
		"trace_id": traceID,
		"spans":    traceSpans,
	})
}

// handleMetrics 获取指标
// GET /api/metrics
func (h *handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats := h.devUI.collector.Stats()

	writeSuccess(w, map[string]any{
		"total_events":   stats.TotalEvents,
		"agent_runs":     stats.AgentRuns,
		"llm_calls":      stats.LLMCalls,
		"tool_calls":     stats.ToolCalls,
		"retriever_runs": stats.RetrieverRuns,
		"errors":         stats.Errors,
		"subscribers":    stats.Subscribers,
		"buffer_size":    stats.BufferSize,
		"uptime_seconds": int64(h.devUI.Uptime().Seconds()),
	})
}

// handleStats 获取统计信息
// GET /api/stats
func (h *handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats := h.devUI.collector.Stats()

	// 计算事件类型分布
	events := h.devUI.collector.Events().GetAll()
	typeCounts := make(map[string]int)
	for _, e := range events {
		typeCounts[string(e.Type)]++
	}

	writeSuccess(w, map[string]any{
		"collector": stats,
		"event_types": typeCounts,
		"server": map[string]any{
			"addr":    h.devUI.Addr(),
			"running": h.devUI.IsRunning(),
			"uptime":  h.devUI.Uptime().String(),
		},
	})
}

// handleHealth 健康检查
// GET /health
func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]any{
		"status":  "healthy",
		"version": "1.0.0",
		"uptime":  h.devUI.Uptime().String(),
	})
}

// handleStatic 处理静态文件
func (h *handler) handleStatic(w http.ResponseWriter, r *http.Request) {
	// 使用自定义静态目录
	if h.devUI.options.StaticDir != "" {
		http.FileServer(http.Dir(h.devUI.options.StaticDir)).ServeHTTP(w, r)
		return
	}

	// 使用内嵌静态文件
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "static files not found")
		return
	}

	// 处理根路径
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// 设置正确的 Content-Type
	switch {
	case strings.HasSuffix(path, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case strings.HasSuffix(path, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(path, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(path, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	}

	http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
}
