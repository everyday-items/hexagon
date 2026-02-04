package graph

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewMemoryInterruptHandler(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	if handler == nil {
		t.Fatal("NewMemoryInterruptHandler returned nil")
	}
	if handler.interrupts == nil {
		t.Error("interrupts map not initialized")
	}
	if handler.waiters == nil {
		t.Error("waiters map not initialized")
	}
}

func TestMemoryInterruptHandler_Create(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	interrupt := &Interrupt{
		NodeName: "test_node",
		Type:     InterruptTypeApproval,
		Title:    "Test Approval",
		Message:  "Please approve this action",
	}

	err := handler.Create(ctx, interrupt)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 检查 ID 是否已生成
	if interrupt.ID == "" {
		t.Error("ID should be generated")
	}

	// 检查状态是否设置为 pending
	if interrupt.Status != InterruptStatusPending {
		t.Errorf("expected status=pending, got %s", interrupt.Status)
	}

	// 检查时间是否设置
	if interrupt.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestMemoryInterruptHandler_Get(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	// 创建中断
	interrupt := &Interrupt{
		NodeName: "test_node",
		Type:     InterruptTypeInput,
		Title:    "Test Input",
	}
	handler.Create(ctx, interrupt)

	// 获取存在的中断
	got, err := handler.Get(ctx, interrupt.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != interrupt.ID {
		t.Errorf("expected ID=%s, got %s", interrupt.ID, got.ID)
	}

	// 获取不存在的中断
	_, err = handler.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent interrupt")
	}
}

func TestMemoryInterruptHandler_Resolve(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	// 创建中断
	interrupt := &Interrupt{
		NodeName: "test_node",
		Type:     InterruptTypeApproval,
		Title:    "Test",
	}
	handler.Create(ctx, interrupt)

	// 解决中断
	response := &InterruptResponse{
		Action:  "approve",
		Comment: "Looks good",
	}
	err := handler.Resolve(ctx, interrupt.ID, response, "admin")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 检查状态
	got, _ := handler.Get(ctx, interrupt.ID)
	if got.Status != InterruptStatusApproved {
		t.Errorf("expected status=approved, got %s", got.Status)
	}
	if got.ResolvedBy != "admin" {
		t.Errorf("expected resolvedBy=admin, got %s", got.ResolvedBy)
	}
	if got.Response == nil {
		t.Error("response should be set")
	}

	// 尝试再次解决已解决的中断
	err = handler.Resolve(ctx, interrupt.ID, response, "admin")
	if err == nil {
		t.Error("expected error when resolving non-pending interrupt")
	}
}

func TestMemoryInterruptHandler_ResolveActions(t *testing.T) {
	tests := []struct {
		action         string
		expectedStatus InterruptStatus
	}{
		{"approve", InterruptStatusApproved},
		{"reject", InterruptStatusRejected},
		{"submit", InterruptStatusCompleted},
		{"cancel", InterruptStatusCancelled},
		{"unknown", InterruptStatusCompleted}, // 默认
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			handler := NewMemoryInterruptHandler()
			ctx := context.Background()

			interrupt := &Interrupt{NodeName: "test", Type: InterruptTypeApproval}
			handler.Create(ctx, interrupt)

			response := &InterruptResponse{Action: tt.action}
			handler.Resolve(ctx, interrupt.ID, response, "user")

			got, _ := handler.Get(ctx, interrupt.ID)
			if got.Status != tt.expectedStatus {
				t.Errorf("action=%s: expected status=%s, got %s",
					tt.action, tt.expectedStatus, got.Status)
			}
		})
	}
}

func TestMemoryInterruptHandler_Cancel(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	interrupt := &Interrupt{NodeName: "test", Type: InterruptTypeApproval}
	handler.Create(ctx, interrupt)

	err := handler.Cancel(ctx, interrupt.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	got, _ := handler.Get(ctx, interrupt.ID)
	if got.Status != InterruptStatusCancelled {
		t.Errorf("expected status=cancelled, got %s", got.Status)
	}
}

func TestMemoryInterruptHandler_List(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	// 创建多个中断 (使用唯一 ID 避免冲突)
	thread1 := "thread-1"
	thread2 := "thread-2"

	handler.Create(ctx, &Interrupt{ID: "int-1", ThreadID: thread1, NodeName: "n1"})
	handler.Create(ctx, &Interrupt{ID: "int-2", ThreadID: thread1, NodeName: "n2"})
	handler.Create(ctx, &Interrupt{ID: "int-3", ThreadID: thread2, NodeName: "n3"})

	// 列出 thread1 的中断
	list, err := handler.List(ctx, thread1)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 interrupts for thread1, got %d", len(list))
	}

	// 列出 thread2 的中断
	list, err = handler.List(ctx, thread2)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 interrupt for thread2, got %d", len(list))
	}
}

func TestMemoryInterruptHandler_ListPending(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	// 创建中断 (使用唯一 ID 避免冲突)
	int1 := &Interrupt{ID: "pending-1", NodeName: "n1"}
	int2 := &Interrupt{ID: "pending-2", NodeName: "n2"}
	int3 := &Interrupt{ID: "pending-3", NodeName: "n3"}
	handler.Create(ctx, int1)
	handler.Create(ctx, int2)
	handler.Create(ctx, int3)

	// 解决一个
	handler.Resolve(ctx, int1.ID, &InterruptResponse{Action: "approve"}, "user")

	// 列出待处理的
	pending, err := handler.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending interrupts, got %d", len(pending))
	}
}

func TestMemoryInterruptHandler_Wait(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	interrupt := &Interrupt{NodeName: "test"}
	handler.Create(ctx, interrupt)

	// 在后台解决中断
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		handler.Resolve(ctx, interrupt.ID, &InterruptResponse{Action: "approve"}, "user")
	}()

	// 等待中断解决
	result, err := handler.Wait(ctx, interrupt.ID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != InterruptStatusApproved {
		t.Errorf("expected status=approved, got %s", result.Status)
	}

	wg.Wait()
}

func TestMemoryInterruptHandler_WaitWithTimeout(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	interrupt := &Interrupt{NodeName: "test"}
	handler.Create(ctx, interrupt)

	// 超时测试
	_, err := handler.WaitWithTimeout(ctx, interrupt.ID, 50*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestMemoryInterruptHandler_WaitAlreadyResolved(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	ctx := context.Background()

	interrupt := &Interrupt{NodeName: "test"}
	handler.Create(ctx, interrupt)
	handler.Resolve(ctx, interrupt.ID, &InterruptResponse{Action: "approve"}, "user")

	// 已解决的中断应立即返回
	result, err := handler.Wait(ctx, interrupt.ID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != InterruptStatusApproved {
		t.Errorf("expected status=approved, got %s", result.Status)
	}
}

func TestMemoryInterruptHandler_WaitContextCancelled(t *testing.T) {
	handler := NewMemoryInterruptHandler()

	interrupt := &Interrupt{NodeName: "test"}
	ctx := context.Background()
	handler.Create(ctx, interrupt)

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())

	// 在后台取消 context
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := handler.Wait(ctx, interrupt.ID)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

func TestInterruptBuilder(t *testing.T) {
	interrupt := NewInterrupt("test_node").
		WithType(InterruptTypeInput).
		WithTitle("Test Title").
		WithMessage("Test Message").
		WithData("key1", "value1").
		WithTimeout(5 * time.Minute).
		Build()

	if interrupt.NodeName != "test_node" {
		t.Errorf("expected nodeID=test_node, got %s", interrupt.NodeName)
	}
	if interrupt.Type != InterruptTypeInput {
		t.Errorf("expected type=input, got %s", interrupt.Type)
	}
	if interrupt.Title != "Test Title" {
		t.Errorf("expected title=Test Title, got %s", interrupt.Title)
	}
	if interrupt.Message != "Test Message" {
		t.Errorf("expected message=Test Message, got %s", interrupt.Message)
	}
	if interrupt.Data["key1"] != "value1" {
		t.Errorf("expected data[key1]=value1, got %v", interrupt.Data["key1"])
	}
	if interrupt.Timeout != 5*time.Minute {
		t.Errorf("expected timeout=5m, got %v", interrupt.Timeout)
	}
}

func TestInterruptBuilder_WithOptions(t *testing.T) {
	options := []InterruptOption{
		{Value: "yes", Label: "Yes", Style: "primary"},
		{Value: "no", Label: "No", Style: "danger"},
	}

	interrupt := NewInterrupt("test").
		WithOptions(options...).
		Build()

	if len(interrupt.Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(interrupt.Options))
	}
	if interrupt.Options[0].Value != "yes" {
		t.Errorf("expected first option=yes, got %s", interrupt.Options[0].Value)
	}
}

func TestInterruptBuilder_WithInputSchema(t *testing.T) {
	schema := &InputSchema{
		Fields: []InputField{
			{Name: "name", Type: "text", Label: "Name", Required: true},
			{Name: "age", Type: "number", Label: "Age"},
		},
	}

	interrupt := NewInterrupt("test").
		WithInputSchema(schema).
		Build()

	if interrupt.InputSchema == nil {
		t.Fatal("InputSchema should be set")
	}
	if len(interrupt.InputSchema.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(interrupt.InputSchema.Fields))
	}
}

func TestApprovalInterrupt(t *testing.T) {
	interrupt := ApprovalInterrupt("node1", "Approve Action", "Do you approve?")

	if interrupt.Type != InterruptTypeApproval {
		t.Errorf("expected type=approval, got %s", interrupt.Type)
	}
	if interrupt.NodeName != "node1" {
		t.Errorf("expected nodeID=node1, got %s", interrupt.NodeName)
	}
	if len(interrupt.Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(interrupt.Options))
	}
}

func TestInputInterrupt(t *testing.T) {
	fields := []InputField{
		{Name: "email", Type: "text", Label: "Email", Required: true},
	}

	interrupt := InputInterrupt("node1", "Input Required", "Please provide", fields...)

	if interrupt.Type != InterruptTypeInput {
		t.Errorf("expected type=input, got %s", interrupt.Type)
	}
	if interrupt.InputSchema == nil {
		t.Fatal("InputSchema should be set")
	}
	if len(interrupt.InputSchema.Fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(interrupt.InputSchema.Fields))
	}
}

func TestReviewInterrupt(t *testing.T) {
	data := map[string]any{
		"amount": 1000,
		"reason": "purchase",
	}

	interrupt := ReviewInterrupt("node1", "Review Required", "Please review", data)

	if interrupt.Type != InterruptTypeReview {
		t.Errorf("expected type=review, got %s", interrupt.Type)
	}
	if interrupt.Data["amount"] != 1000 {
		t.Errorf("expected data[amount]=1000, got %v", interrupt.Data["amount"])
	}
	if len(interrupt.Options) != 3 {
		t.Errorf("expected 3 options (approve/reject/modify), got %d", len(interrupt.Options))
	}
}

func TestInterruptError(t *testing.T) {
	interrupt := &Interrupt{
		NodeName: "test_node",
		Title:    "Test Error",
	}

	err := &InterruptError{Interrupt: interrupt}

	expectedMsg := "graph interrupted at node test_node: Test Error"
	if err.Error() != expectedMsg {
		t.Errorf("expected error=%q, got %q", expectedMsg, err.Error())
	}
}

func TestIsInterruptError(t *testing.T) {
	// 测试是中断错误的情况
	interrupt := &Interrupt{NodeName: "test"}
	err := &InterruptError{Interrupt: interrupt}

	got, ok := IsInterruptError(err)
	if !ok {
		t.Error("expected IsInterruptError to return true")
	}
	if got != interrupt {
		t.Error("expected to return the original interrupt")
	}

	// 测试不是中断错误的情况
	regularErr := context.DeadlineExceeded
	_, ok = IsInterruptError(regularErr)
	if ok {
		t.Error("expected IsInterruptError to return false for regular error")
	}
}

func TestInterruptConfig(t *testing.T) {
	handler := NewMemoryInterruptHandler()
	config := NewInterruptConfig(handler).
		WithDefaultTimeout(1 * time.Hour).
		WithAutoResume(true)

	if config.Handler != handler {
		t.Error("handler not set correctly")
	}
	if config.DefaultTimeout != 1*time.Hour {
		t.Errorf("expected timeout=1h, got %v", config.DefaultTimeout)
	}
	if !config.AutoResume {
		t.Error("expected autoResume=true")
	}
}

func TestGenerateInterruptID(t *testing.T) {
	id1 := generateInterruptID()
	time.Sleep(time.Nanosecond)
	id2 := generateInterruptID()

	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}

	if len(id1) == 0 {
		t.Error("generated ID should not be empty")
	}
}

func TestInputField(t *testing.T) {
	minVal := 0.0
	maxVal := 100.0
	minLen := 1
	maxLen := 255

	field := InputField{
		Name:        "age",
		Type:        "number",
		Label:       "Age",
		Description: "Your age",
		Required:    true,
		Default:     18,
		Options:     []string{"18", "25", "35", "45"},
		Validation: &FieldValidation{
			Min:       &minVal,
			Max:       &maxVal,
			MinLength: &minLen,
			MaxLength: &maxLen,
			Pattern:   "^[0-9]+$",
		},
	}

	if field.Name != "age" {
		t.Errorf("expected name=age, got %s", field.Name)
	}
	if field.Validation.Min == nil || *field.Validation.Min != 0.0 {
		t.Error("validation min not set correctly")
	}
	if field.Validation.Pattern != "^[0-9]+$" {
		t.Errorf("expected pattern=^[0-9]+$, got %s", field.Validation.Pattern)
	}
}

func TestInterruptResponse(t *testing.T) {
	response := &InterruptResponse{
		Action:  "approve",
		Data:    map[string]any{"note": "approved with changes"},
		Comment: "Looks good",
	}

	if response.Action != "approve" {
		t.Errorf("expected action=approve, got %s", response.Action)
	}
	if response.Data["note"] != "approved with changes" {
		t.Error("data not set correctly")
	}
}

func TestInterruptOption(t *testing.T) {
	option := InterruptOption{
		Value:       "approve",
		Label:       "Approve",
		Description: "Approve the request",
		Style:       "primary",
	}

	if option.Value != "approve" {
		t.Errorf("expected value=approve, got %s", option.Value)
	}
	if option.Style != "primary" {
		t.Errorf("expected style=primary, got %s", option.Style)
	}
}

// 确保实现了接口
func TestInterfaceImplementation(t *testing.T) {
	var _ InterruptHandler = (*MemoryInterruptHandler)(nil)
}
