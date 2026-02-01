package audit

import (
	"context"
	"testing"
	"time"
)

func TestDefaultAuditConfig(t *testing.T) {
	cfg := DefaultAuditConfig()

	if !cfg.Enabled {
		t.Error("expected enabled by default")
	}

	if cfg.BufferSize != 1000 {
		t.Errorf("expected buffer size 1000, got %d", cfg.BufferSize)
	}

	if cfg.RetentionDays != 90 {
		t.Errorf("expected retention 90 days, got %d", cfg.RetentionDays)
	}

	if cfg.LogLevel != LevelInfo {
		t.Errorf("expected log level info, got %s", cfg.LogLevel)
	}

	if len(cfg.SensitiveFields) == 0 {
		t.Error("expected sensitive fields")
	}
}

func TestNewAuditLogger(t *testing.T) {
	logger := NewAuditLogger()

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewAuditLoggerWithOptions(t *testing.T) {
	logger := NewAuditLogger(
		WithAuditEnabled(false),
		WithAuditLevel(LevelWarning),
		WithRetentionDays(30),
	)

	if logger.config.Enabled {
		t.Error("expected disabled")
	}

	if logger.config.LogLevel != LevelWarning {
		t.Errorf("expected level warning, got %s", logger.config.LogLevel)
	}

	if logger.config.RetentionDays != 30 {
		t.Errorf("expected retention 30, got %d", logger.config.RetentionDays)
	}
}

func TestMemoryAuditStore(t *testing.T) {
	store := NewMemoryAuditStore()

	if store == nil {
		t.Fatal("expected non-nil store")
	}

	// 保存事件
	event := &AuditEvent{
		ID:        "test-1",
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Category:  CategoryAgent,
		Action:    "test",
		Result:    ResultSuccess,
	}

	err := store.Save(context.Background(), event)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// 查询事件
	events, err := store.Query(context.Background(), AuditQuery{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestMemoryAuditStoreQuery(t *testing.T) {
	store := NewMemoryAuditStore()

	// 保存多个事件
	now := time.Now()
	store.Save(context.Background(), &AuditEvent{
		ID:        "1",
		Timestamp: now,
		Level:     LevelInfo,
		Category:  CategoryAgent,
		Action:    "run",
		Result:    ResultSuccess,
	})
	store.Save(context.Background(), &AuditEvent{
		ID:        "2",
		Timestamp: now.Add(time.Hour),
		Level:     LevelError,
		Category:  CategoryTool,
		Action:    "execute",
		Result:    ResultFailure,
	})

	// 按类别查询
	events, _ := store.Query(context.Background(), AuditQuery{
		Categories: []EventCategory{CategoryAgent},
	})
	if len(events) != 1 {
		t.Errorf("expected 1 agent event, got %d", len(events))
	}

	// 按级别查询
	events, _ = store.Query(context.Background(), AuditQuery{
		Levels: []AuditLevel{LevelError},
	})
	if len(events) != 1 {
		t.Errorf("expected 1 error event, got %d", len(events))
	}

	// 按操作查询
	events, _ = store.Query(context.Background(), AuditQuery{
		Action: "run",
	})
	if len(events) != 1 {
		t.Errorf("expected 1 run event, got %d", len(events))
	}
}

func TestMemoryAuditStoreCount(t *testing.T) {
	store := NewMemoryAuditStore()

	store.Save(context.Background(), &AuditEvent{ID: "1"})
	store.Save(context.Background(), &AuditEvent{ID: "2"})
	store.Save(context.Background(), &AuditEvent{ID: "3"})

	count, err := store.Count(context.Background(), AuditQuery{})
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestMemoryAuditStoreDelete(t *testing.T) {
	store := NewMemoryAuditStore()

	store.Save(context.Background(), &AuditEvent{ID: "1"})
	store.Save(context.Background(), &AuditEvent{ID: "2"})
	store.Save(context.Background(), &AuditEvent{ID: "3"})

	err := store.Delete(context.Background(), []string{"1", "3"})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	count, _ := store.Count(context.Background(), AuditQuery{})
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}

func TestMemoryAuditStoreCleanup(t *testing.T) {
	store := NewMemoryAuditStore()

	now := time.Now()
	store.Save(context.Background(), &AuditEvent{ID: "old", Timestamp: now.Add(-48 * time.Hour)})
	store.Save(context.Background(), &AuditEvent{ID: "new", Timestamp: now})

	deleted, err := store.Cleanup(context.Background(), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	count, _ := store.Count(context.Background(), AuditQuery{})
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}

func TestAuditLevels(t *testing.T) {
	levels := []AuditLevel{
		LevelDebug,
		LevelInfo,
		LevelWarning,
		LevelError,
		LevelCritical,
	}

	expected := []string{"debug", "info", "warning", "error", "critical"}

	for i, level := range levels {
		if string(level) != expected[i] {
			t.Errorf("expected level '%s', got '%s'", expected[i], level)
		}
	}
}

func TestEventCategories(t *testing.T) {
	categories := []EventCategory{
		CategoryAuth,
		CategoryAgent,
		CategoryTool,
		CategoryLLM,
		CategoryMemory,
		CategoryWorkflow,
		CategoryNetwork,
		CategorySecurity,
		CategoryConfig,
		CategoryAdmin,
	}

	for _, cat := range categories {
		if cat == "" {
			t.Error("empty category")
		}
	}
}

func TestEventResults(t *testing.T) {
	results := []EventResult{
		ResultSuccess,
		ResultFailure,
		ResultDenied,
		ResultError,
	}

	expected := []string{"success", "failure", "denied", "error"}

	for i, result := range results {
		if string(result) != expected[i] {
			t.Errorf("expected result '%s', got '%s'", expected[i], result)
		}
	}
}

func TestActor(t *testing.T) {
	actor := &Actor{
		Type:      "user",
		ID:        "user-123",
		Name:      "Test User",
		IP:        "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		Roles:     []string{"admin", "user"},
	}

	if actor.Type != "user" {
		t.Errorf("unexpected type: %s", actor.Type)
	}

	if len(actor.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(actor.Roles))
	}
}

func TestTarget(t *testing.T) {
	target := &Target{
		Type: "agent",
		ID:   "agent-456",
		Name: "Test Agent",
		Attributes: map[string]any{
			"version": "1.0",
		},
	}

	if target.Type != "agent" {
		t.Errorf("unexpected type: %s", target.Type)
	}

	if target.Attributes["version"] != "1.0" {
		t.Error("unexpected attribute")
	}
}

func TestPredefinedActors(t *testing.T) {
	// System actor
	system := SystemActor()
	if system.Type != "system" {
		t.Errorf("expected type 'system', got '%s'", system.Type)
	}

	// User actor
	user := UserActor("u1", "User One", []string{"admin"})
	if user.Type != "user" {
		t.Errorf("expected type 'user', got '%s'", user.Type)
	}
	if user.ID != "u1" {
		t.Errorf("expected ID 'u1', got '%s'", user.ID)
	}

	// Agent actor
	agent := AgentActor("a1", "Agent One")
	if agent.Type != "agent" {
		t.Errorf("expected type 'agent', got '%s'", agent.Type)
	}
}

func TestFilterByCategory(t *testing.T) {
	filter := FilterByCategory(CategoryAgent, CategoryTool)

	// 匹配的事件
	event1 := &AuditEvent{Category: CategoryAgent}
	if !filter(event1) {
		t.Error("expected agent event to match")
	}

	event2 := &AuditEvent{Category: CategoryTool}
	if !filter(event2) {
		t.Error("expected tool event to match")
	}

	// 不匹配的事件
	event3 := &AuditEvent{Category: CategoryLLM}
	if filter(event3) {
		t.Error("expected llm event not to match")
	}
}

func TestFilterByLevel(t *testing.T) {
	filter := FilterByLevel(LevelWarning)

	// 低于阈值
	event1 := &AuditEvent{Level: LevelDebug}
	if filter(event1) {
		t.Error("expected debug event not to match")
	}

	event2 := &AuditEvent{Level: LevelInfo}
	if filter(event2) {
		t.Error("expected info event not to match")
	}

	// 等于或高于阈值
	event3 := &AuditEvent{Level: LevelWarning}
	if !filter(event3) {
		t.Error("expected warning event to match")
	}

	event4 := &AuditEvent{Level: LevelError}
	if !filter(event4) {
		t.Error("expected error event to match")
	}
}

func TestAuditEvent(t *testing.T) {
	event := &AuditEvent{
		ID:        "evt-1",
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Category:  CategoryAgent,
		Action:    "run",
		Actor:     SystemActor(),
		Target:    &Target{Type: "agent", ID: "test"},
		Result:    ResultSuccess,
		Duration:  time.Second,
		TraceID:   "trace-123",
		SpanID:    "span-456",
		Tags:      []string{"test"},
		Details:   map[string]any{"key": "value"},
		Metadata:  map[string]any{"meta": "data"},
	}

	if event.ID != "evt-1" {
		t.Errorf("unexpected ID: %s", event.ID)
	}

	if event.TraceID != "trace-123" {
		t.Errorf("unexpected trace ID: %s", event.TraceID)
	}
}

func TestContextWithAuditLogger(t *testing.T) {
	logger := NewAuditLogger()
	ctx := ContextWithAuditLogger(context.Background(), logger)

	retrieved := AuditLoggerFromContext(ctx)
	if retrieved != logger {
		t.Error("expected same logger from context")
	}
}

func TestAuditLoggerFromContextNil(t *testing.T) {
	retrieved := AuditLoggerFromContext(context.Background())
	if retrieved != nil {
		t.Error("expected nil from empty context")
	}
}

func TestTimeRange(t *testing.T) {
	tr := TimeRange{
		Start: time.Now().Add(-time.Hour),
		End:   time.Now(),
	}

	if tr.Start.After(tr.End) {
		t.Error("start should be before end")
	}
}

func TestActorStats(t *testing.T) {
	stats := ActorStats{
		ActorID:   "user-1",
		ActorName: "User One",
		Count:     100,
	}

	if stats.Count != 100 {
		t.Errorf("expected count 100, got %d", stats.Count)
	}
}

func TestActionStats(t *testing.T) {
	stats := ActionStats{
		Action: "run",
		Count:  50,
	}

	if stats.Action != "run" {
		t.Errorf("expected action 'run', got '%s'", stats.Action)
	}
}

func TestAuditQuery(t *testing.T) {
	query := AuditQuery{
		StartTime:  time.Now().Add(-time.Hour),
		EndTime:    time.Now(),
		Categories: []EventCategory{CategoryAgent},
		Levels:     []AuditLevel{LevelError},
		ActorID:    "actor-1",
		TargetID:   "target-1",
		Action:     "run",
		Result:     ResultSuccess,
		TraceID:    "trace-1",
		Tags:       []string{"test"},
		Limit:      100,
		Offset:     10,
		OrderBy:    "timestamp",
		OrderDesc:  true,
	}

	if query.Limit != 100 {
		t.Errorf("expected limit 100, got %d", query.Limit)
	}
}

func TestRequestInfo(t *testing.T) {
	info := &RequestInfo{
		Method:  "POST",
		Path:    "/api/agent/run",
		Query:   map[string]string{"id": "123"},
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    map[string]any{"input": "test"},
	}

	if info.Method != "POST" {
		t.Errorf("unexpected method: %s", info.Method)
	}
}

func TestResponseInfo(t *testing.T) {
	info := &ResponseInfo{
		StatusCode: 200,
		Body:       map[string]any{"result": "success"},
		Error:      "",
	}

	if info.StatusCode != 200 {
		t.Errorf("unexpected status: %d", info.StatusCode)
	}
}
