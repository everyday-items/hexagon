package graph

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestHITLNode_Approval 测试审批模式：批准和拒绝两种场景
func TestHITLNode_Approval(t *testing.T) {
	t.Run("批准", func(t *testing.T) {
		// 构造一个自动批准的回调处理器
		handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
			if req.Type != HITLApproval {
				t.Errorf("期望请求类型为 approval，实际为 %s", req.Type)
			}
			return &HITLResponse{
				RequestID:  req.ID,
				Approved:   true,
				Feedback:   "已审批通过",
				RespondedBy: "admin",
				RespondedAt: time.Now(),
			}, nil
		})

		node := &HITLNode[TestState]{
			ID:      "approve-node",
			Name:    "审批节点",
			Type:    HITLApproval,
			Handler: handler,
			RequestBuilder: func(state TestState) *HITLRequest {
				return &HITLRequest{
					Title:    "需要审批操作",
					Priority: PriorityHigh,
				}
			},
			ResponseHandler: func(state TestState, resp *HITLResponse) TestState {
				if resp.Approved {
					state.Path += "approved"
				} else {
					state.Path += "rejected"
				}
				return state
			},
		}

		ctx := context.Background()
		state := TestState{Counter: 0, Path: "", Data: map[string]string{}}
		result, err := node.Execute(ctx, state)
		if err != nil {
			t.Fatalf("执行审批节点失败: %v", err)
		}
		if result.Path != "approved" {
			t.Errorf("期望路径为 'approved'，实际为 '%s'", result.Path)
		}
	})

	t.Run("拒绝", func(t *testing.T) {
		// 构造一个自动拒绝的回调处理器
		handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
			return &HITLResponse{
				RequestID:  req.ID,
				Approved:   false,
				Feedback:   "不符合要求",
				RespondedAt: time.Now(),
			}, nil
		})

		node := &HITLNode[TestState]{
			ID:      "reject-node",
			Name:    "拒绝节点",
			Type:    HITLApproval,
			Handler: handler,
			RequestBuilder: func(state TestState) *HITLRequest {
				return &HITLRequest{
					Title:    "需要审批",
					Priority: PriorityNormal,
				}
			},
			ResponseHandler: func(state TestState, resp *HITLResponse) TestState {
				if resp.Approved {
					state.Path += "approved"
				} else {
					state.Path += "rejected"
				}
				return state
			},
		}

		ctx := context.Background()
		state := TestState{Counter: 0, Path: "", Data: map[string]string{}}
		result, err := node.Execute(ctx, state)
		if err != nil {
			t.Fatalf("执行拒绝节点失败: %v", err)
		}
		if result.Path != "rejected" {
			t.Errorf("期望路径为 'rejected'，实际为 '%s'", result.Path)
		}
	})
}

// TestHITLNode_Input 测试输入模式：人工提供额外输入
func TestHITLNode_Input(t *testing.T) {
	// 模拟人工提供姓名和邮箱输入
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		if req.Type != HITLInput {
			t.Errorf("期望请求类型为 input，实际为 %s", req.Type)
		}
		// 校验请求中携带了 InputSchema
		if req.InputSchema == nil {
			t.Error("期望 InputSchema 不为空")
		}
		return &HITLResponse{
			RequestID: req.ID,
			Input: map[string]any{
				"name":  "张三",
				"email": "zhangsan@example.com",
			},
			RespondedBy: "operator",
			RespondedAt: time.Now(),
		}, nil
	})

	node := &HITLNode[TestState]{
		ID:      "input-node",
		Name:    "输入节点",
		Type:    HITLInput,
		Handler: handler,
		RequestBuilder: func(state TestState) *HITLRequest {
			return &HITLRequest{
				Title: "请提供用户信息",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  map[string]any{"type": "string"},
						"email": map[string]any{"type": "string"},
					},
				},
				Priority: PriorityNormal,
			}
		},
		ResponseHandler: func(state TestState, resp *HITLResponse) TestState {
			// 将人工输入存入 Data 字段
			if name, ok := resp.Input["name"].(string); ok {
				state.Data["name"] = name
			}
			if email, ok := resp.Input["email"].(string); ok {
				state.Data["email"] = email
			}
			state.Counter++
			return state
		},
	}

	ctx := context.Background()
	state := TestState{Counter: 0, Path: "", Data: map[string]string{}}
	result, err := node.Execute(ctx, state)
	if err != nil {
		t.Fatalf("执行输入节点失败: %v", err)
	}
	if result.Data["name"] != "张三" {
		t.Errorf("期望 name 为 '张三'，实际为 '%s'", result.Data["name"])
	}
	if result.Data["email"] != "zhangsan@example.com" {
		t.Errorf("期望 email 为 'zhangsan@example.com'，实际为 '%s'", result.Data["email"])
	}
	if result.Counter != 1 {
		t.Errorf("期望 Counter 为 1，实际为 %d", result.Counter)
	}
}

// TestHITLNode_Timeout 测试超时处理
func TestHITLNode_Timeout(t *testing.T) {
	// 使用 ChannelHITLHandler，不提交响应来触发超时
	ch := NewChannelHITLHandler(1)

	node := &HITLNode[TestState]{
		ID:      "timeout-node",
		Name:    "超时节点",
		Type:    HITLApproval,
		Handler: ch,
		RequestBuilder: func(state TestState) *HITLRequest {
			return &HITLRequest{
				Title:   "即将超时的请求",
				Timeout: 50 * time.Millisecond, // 设置较短的超时时间
			}
		},
	}

	ctx := context.Background()
	state := TestState{Counter: 0, Path: "", Data: map[string]string{}}

	// 不提交响应，等待超时
	_, err := node.Execute(ctx, state)
	if err == nil {
		t.Fatal("期望超时错误，但未返回错误")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("期望错误包含 'timeout'，实际为: %v", err)
	}
}

// TestChannelHITLHandler 测试基于 channel 的处理器收发
func TestChannelHITLHandler(t *testing.T) {
	ch := NewChannelHITLHandler(10)

	// 在后台 goroutine 中发送请求并等待响应
	type result struct {
		resp *HITLResponse
		err  error
	}
	resultCh := make(chan result, 1)

	req := &HITLRequest{
		ID:       "test-req-1",
		Type:     HITLApproval,
		Title:    "测试请求",
		Timeout:  5 * time.Second,
		Priority: PriorityNormal,
	}

	go func() {
		resp, err := ch.Handle(context.Background(), req)
		resultCh <- result{resp: resp, err: err}
	}()

	// 从请求 channel 中读取请求
	select {
	case received := <-ch.GetRequests():
		if received.ID != "test-req-1" {
			t.Errorf("期望请求 ID 为 'test-req-1'，实际为 '%s'", received.ID)
		}
		if received.Title != "测试请求" {
			t.Errorf("期望请求标题为 '测试请求'，实际为 '%s'", received.Title)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("获取请求超时")
	}

	// 提交响应
	err := ch.SubmitResponse(&HITLResponse{
		RequestID:  "test-req-1",
		Approved:   true,
		Feedback:   "通过",
		RespondedBy: "tester",
	})
	if err != nil {
		t.Fatalf("提交响应失败: %v", err)
	}

	// 等待处理结果
	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("处理请求失败: %v", r.err)
		}
		if !r.resp.Approved {
			t.Error("期望响应为批准")
		}
		if r.resp.Feedback != "通过" {
			t.Errorf("期望反馈为 '通过'，实际为 '%s'", r.resp.Feedback)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待处理结果超时")
	}

	// 测试对不存在的请求提交响应应返回错误
	err = ch.SubmitResponse(&HITLResponse{
		RequestID: "nonexistent",
	})
	if err == nil {
		t.Error("期望对不存在的请求返回错误")
	}
}

// TestHITLManager_Submit 测试管理器提交请求和接收响应
func TestHITLManager_Submit(t *testing.T) {
	// 使用回调处理器自动批准
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		return &HITLResponse{
			RequestID:  req.ID,
			Approved:   true,
			Feedback:   "自动批准",
			RespondedAt: time.Now(),
		}, nil
	})

	manager := NewHITLManager(handler, HITLManagerConfig{
		MaxPendingRequests: 10,
		DefaultTimeout:     5 * time.Second,
		HistoryLimit:       100,
	})

	ctx := context.Background()
	req := &HITLRequest{
		ID:       "mgr-req-1",
		Type:     HITLApproval,
		Title:    "管理器测试请求",
		Priority: PriorityHigh,
	}

	resp, err := manager.Submit(ctx, req)
	if err != nil {
		t.Fatalf("提交请求失败: %v", err)
	}
	if !resp.Approved {
		t.Error("期望响应为批准")
	}

	// 验证统计信息
	stats := manager.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("期望总请求数为 1，实际为 %d", stats.TotalRequests)
	}
	if stats.CompletedCount != 1 {
		t.Errorf("期望已完成数为 1，实际为 %d", stats.CompletedCount)
	}
	if stats.ByType[HITLApproval] != 1 {
		t.Errorf("期望审批类型计数为 1，实际为 %d", stats.ByType[HITLApproval])
	}
	if stats.ByPriority[PriorityHigh] != 1 {
		t.Errorf("期望高优先级计数为 1，实际为 %d", stats.ByPriority[PriorityHigh])
	}
}

// TestHITLManager_ActiveRequests 测试活跃请求查询
func TestHITLManager_ActiveRequests(t *testing.T) {
	// 使用阻塞处理器，请求在处理期间保持活跃状态
	blockCh := make(chan struct{})
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		// 阻塞直到接收到信号
		<-blockCh
		return &HITLResponse{
			RequestID:  req.ID,
			Approved:   true,
			RespondedAt: time.Now(),
		}, nil
	})

	manager := NewHITLManager(handler)

	// 在后台提交请求
	go func() {
		_, _ = manager.Submit(context.Background(), &HITLRequest{
			ID:       "active-req-1",
			Type:     HITLInput,
			Title:    "活跃请求1",
			Priority: PriorityNormal,
		})
	}()

	// 等待请求进入活跃状态
	time.Sleep(50 * time.Millisecond)

	activeReqs := manager.GetActiveRequests()
	if len(activeReqs) != 1 {
		t.Errorf("期望活跃请求数为 1，实际为 %d", len(activeReqs))
	}
	if len(activeReqs) > 0 && activeReqs[0].ID != "active-req-1" {
		t.Errorf("期望活跃请求 ID 为 'active-req-1'，实际为 '%s'", activeReqs[0].ID)
	}

	// 解除阻塞，让请求完成
	close(blockCh)

	// 等待请求完成
	time.Sleep(50 * time.Millisecond)

	activeReqs = manager.GetActiveRequests()
	if len(activeReqs) != 0 {
		t.Errorf("期望完成后活跃请求数为 0，实际为 %d", len(activeReqs))
	}
}

// TestHITLManager_History 测试历史记录功能
func TestHITLManager_History(t *testing.T) {
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		return &HITLResponse{
			RequestID:  req.ID,
			Approved:   req.Type == HITLApproval,
			RespondedAt: time.Now(),
		}, nil
	})

	manager := NewHITLManager(handler, HITLManagerConfig{
		MaxPendingRequests: 10,
		DefaultTimeout:     5 * time.Second,
		HistoryLimit:       5, // 设置较小的历史记录上限便于测试
	})

	ctx := context.Background()

	// 提交多个请求
	for i := 0; i < 3; i++ {
		_, _ = manager.Submit(ctx, &HITLRequest{
			ID:       "hist-" + string(rune('a'+i)),
			Type:     HITLApproval,
			Title:    "历史记录测试",
			Priority: PriorityNormal,
		})
	}

	// 获取全部历史记录
	history := manager.GetHistory(0)
	if len(history) != 3 {
		t.Errorf("期望历史记录数为 3，实际为 %d", len(history))
	}

	// 获取指定数量的历史记录
	history = manager.GetHistory(2)
	if len(history) != 2 {
		t.Errorf("期望历史记录数为 2，实际为 %d", len(history))
	}

	// 验证记录状态为 completed
	for _, record := range history {
		if record.Status != "completed" {
			t.Errorf("期望记录状态为 'completed'，实际为 '%s'", record.Status)
		}
		if record.Response == nil {
			t.Error("期望记录包含响应")
		}
		if record.EndedAt == nil {
			t.Error("期望记录包含结束时间")
		}
	}

	// 测试导出功能
	data, err := manager.Export()
	if err != nil {
		t.Fatalf("导出历史记录失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("期望导出数据不为空")
	}
}

// TestNewApprovalNode 测试审批节点便捷构造函数
func TestNewApprovalNode(t *testing.T) {
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		// 验证默认请求构建器生成的字段
		if req.Type != HITLApproval {
			t.Errorf("期望类型为 approval，实际为 %s", req.Type)
		}
		if req.Priority != PriorityUrgent {
			t.Errorf("期望优先级为 urgent，实际为 %s", req.Priority)
		}
		if req.Title != "自定义审批标题" {
			t.Errorf("期望标题为 '自定义审批标题'，实际为 '%s'", req.Title)
		}
		if req.Description != "审批描述" {
			t.Errorf("期望描述为 '审批描述'，实际为 '%s'", req.Description)
		}
		// 验证默认 Options 中包含批准和拒绝选项
		if len(req.Options) != 2 {
			t.Errorf("期望 2 个选项，实际为 %d", len(req.Options))
		}
		return &HITLResponse{
			RequestID:  req.ID,
			Approved:   true,
			RespondedAt: time.Now(),
		}, nil
	})

	// 使用选项自定义审批节点
	node := NewApprovalNode[TestState](
		"approval-1",
		handler,
		WithHITLTitle[TestState]("自定义审批标题"),
		WithHITLDescription[TestState]("审批描述"),
		WithHITLPriority[TestState](PriorityUrgent),
	)

	// 验证节点基本属性
	if node.ID != "approval-1" {
		t.Errorf("期望节点 ID 为 'approval-1'，实际为 '%s'", node.ID)
	}
	if node.Name != "Approval: approval-1" {
		t.Errorf("期望节点名称为 'Approval: approval-1'，实际为 '%s'", node.Name)
	}
	if node.Type != HITLApproval {
		t.Errorf("期望节点类型为 approval，实际为 %s", node.Type)
	}

	// 执行节点以验证请求构建和响应处理
	ctx := context.Background()
	state := TestState{Counter: 0, Path: "", Data: map[string]string{}}
	_, err := node.Execute(ctx, state)
	if err != nil {
		t.Fatalf("执行审批节点失败: %v", err)
	}
}

// TestNewInputNode 测试输入节点便捷构造函数
func TestNewInputNode(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{"type": "string"},
		},
	}

	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		// 验证请求类型和 Schema
		if req.Type != HITLInput {
			t.Errorf("期望类型为 input，实际为 %s", req.Type)
		}
		if req.InputSchema == nil {
			t.Error("期望 InputSchema 不为空")
		}
		if req.Title != "请输入原因" {
			t.Errorf("期望标题为 '请输入原因'，实际为 '%s'", req.Title)
		}
		return &HITLResponse{
			RequestID: req.ID,
			Input: map[string]any{
				"reason": "测试原因",
			},
			RespondedAt: time.Now(),
		}, nil
	})

	node := NewInputNode[TestState](
		"input-1",
		handler,
		schema,
		WithHITLTitle[TestState]("请输入原因"),
		WithHITLTimeout[TestState](10*time.Second),
	)

	// 验证节点基本属性
	if node.ID != "input-1" {
		t.Errorf("期望节点 ID 为 'input-1'，实际为 '%s'", node.ID)
	}
	if node.Name != "Input: input-1" {
		t.Errorf("期望节点名称为 'Input: input-1'，实际为 '%s'", node.Name)
	}
	if node.Type != HITLInput {
		t.Errorf("期望节点类型为 input，实际为 %s", node.Type)
	}

	// 执行节点
	ctx := context.Background()
	state := TestState{Counter: 0, Path: "", Data: map[string]string{}}
	result, err := node.Execute(ctx, state)
	if err != nil {
		t.Fatalf("执行输入节点失败: %v", err)
	}

	// TestState 不是 map[string]any，所以默认 ResponseHandler 不会修改状态
	// 验证节点确实正常执行完成
	if result.Counter != 0 {
		t.Errorf("期望 Counter 保持为 0（默认处理器不适用于 TestState），实际为 %d", result.Counter)
	}
}

// TestHITLNode_Condition 测试 HITL 节点的条件触发功能
func TestHITLNode_Condition(t *testing.T) {
	callCount := 0
	handler := HITLCallback(func(ctx context.Context, req *HITLRequest) (*HITLResponse, error) {
		callCount++
		return &HITLResponse{
			RequestID:  req.ID,
			Approved:   true,
			RespondedAt: time.Now(),
		}, nil
	})

	// 创建带条件的审批节点：仅当 Counter > 5 时触发
	node := NewApprovalNode[TestState](
		"conditional",
		handler,
		WithHITLCondition[TestState](func(state TestState) bool {
			return state.Counter > 5
		}),
	)

	ctx := context.Background()

	// Counter = 3，条件不满足，应跳过
	state := TestState{Counter: 3, Path: "", Data: map[string]string{}}
	_, err := node.Execute(ctx, state)
	if err != nil {
		t.Fatalf("条件不满足时执行失败: %v", err)
	}
	if callCount != 0 {
		t.Errorf("期望处理器未被调用，实际调用了 %d 次", callCount)
	}

	// Counter = 10，条件满足，应触发
	state = TestState{Counter: 10, Path: "", Data: map[string]string{}}
	_, err = node.Execute(ctx, state)
	if err != nil {
		t.Fatalf("条件满足时执行失败: %v", err)
	}
	if callCount != 1 {
		t.Errorf("期望处理器被调用 1 次，实际调用了 %d 次", callCount)
	}
}
