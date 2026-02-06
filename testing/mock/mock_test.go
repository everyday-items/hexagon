// Package mock 提供 Hexagon AI Agent 框架测试的 Mock 实现
//
// 本文件包含 mock 包所有核心组件的全面测试：
//   - LLMProvider: Mock LLM Provider 的创建、响应、流式、调用记录等
//   - Memory: Mock Memory 的 CRUD 操作、搜索、统计等
//   - Tool: Mock Tool 的执行、调用记录、工具函数等
//   - Retriever: Mock Retriever 的检索、文档管理等
package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/ai-core/schema"
	toolpkg "github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/rag"
)

// ============================================================================
// LLMProvider 测试
// ============================================================================

// TestNewLLMProvider 测试创建 Mock LLM Provider
func TestNewLLMProvider(t *testing.T) {
	p := NewLLMProvider("test-llm")

	if p.Name() != "test-llm" {
		t.Errorf("期望名称为 'test-llm'，实际为 '%s'", p.Name())
	}

	if p.CallCount() != 0 {
		t.Errorf("期望初始调用次数为 0，实际为 %d", p.CallCount())
	}

	if p.LastCall() != nil {
		t.Error("期望初始最后调用为 nil")
	}

	calls := p.Calls()
	if len(calls) != 0 {
		t.Errorf("期望初始调用列表为空，实际有 %d 条", len(calls))
	}
}

// TestLLMProviderAddResponseAndComplete 测试添加响应并按序返回
func TestLLMProviderAddResponseAndComplete(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("第一条回复").AddResponse("第二条回复").AddResponse("第三条回复")

	ctx := context.Background()

	// 按顺序返回响应
	for i, expected := range []string{"第一条回复", "第二条回复", "第三条回复"} {
		resp, err := p.Complete(ctx, llm.CompletionRequest{
			Model:    "test-model",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf("消息%d", i+1)}},
		})
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
		if resp.Content != expected {
			t.Errorf("第 %d 次调用期望 '%s'，实际为 '%s'", i+1, expected, resp.Content)
		}
		// 验证 Usage 被正确设置
		if resp.Usage.TotalTokens != 100 {
			t.Errorf("期望 TotalTokens=100，实际为 %d", resp.Usage.TotalTokens)
		}
	}

	// 验证调用次数
	if p.CallCount() != 3 {
		t.Errorf("期望调用次数为 3，实际为 %d", p.CallCount())
	}
}

// TestLLMProviderAddToolCallResponse 测试工具调用响应
func TestLLMProviderAddToolCallResponse(t *testing.T) {
	p := NewLLMProvider("test")
	toolCalls := []llm.ToolCall{
		{
			ID:        "call_1",
			Type:      "function",
			Name:      "calculator",
			Arguments: `{"a": 1, "b": 2}`,
		},
		{
			ID:        "call_2",
			Type:      "function",
			Name:      "search",
			Arguments: `{"query": "Go language"}`,
		},
	}
	p.AddToolCallResponse(toolCalls)

	ctx := context.Background()
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "请计算"}},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("期望 2 个工具调用，实际为 %d", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].Name != "calculator" {
		t.Errorf("期望第一个工具为 'calculator'，实际为 '%s'", resp.ToolCalls[0].Name)
	}

	if resp.ToolCalls[1].Name != "search" {
		t.Errorf("期望第二个工具为 'search'，实际为 '%s'", resp.ToolCalls[1].Name)
	}

	// 验证 Usage
	if resp.Usage.TotalTokens != 150 {
		t.Errorf("期望 TotalTokens=150，实际为 %d", resp.Usage.TotalTokens)
	}
}

// TestLLMProviderAddErrorResponse 测试错误响应
func TestLLMProviderAddErrorResponse(t *testing.T) {
	p := NewLLMProvider("test")
	expectedErr := errors.New("API 限流")
	p.AddErrorResponse(expectedErr)

	ctx := context.Background()
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err == nil {
		t.Fatal("期望返回错误，但没有错误")
	}

	if err.Error() != "API 限流" {
		t.Errorf("期望错误为 'API 限流'，实际为 '%s'", err.Error())
	}

	if resp != nil {
		t.Error("期望响应为 nil")
	}
}

// TestLLMProviderWithResponseFn 测试自定义响应函数
func TestLLMProviderWithResponseFn(t *testing.T) {
	p := NewLLMProvider("test")
	p.WithResponseFn(func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		// 返回请求中消息数量作为响应
		return &llm.CompletionResponse{
			Content: fmt.Sprintf("收到 %d 条消息", len(req.Messages)),
			Usage:   llm.Usage{TotalTokens: len(req.Messages) * 10},
		}, nil
	})

	ctx := context.Background()

	// 单条消息
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if resp.Content != "收到 1 条消息" {
		t.Errorf("期望 '收到 1 条消息'，实际为 '%s'", resp.Content)
	}

	// 多条消息
	resp, err = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是助手"},
			{Role: llm.RoleUser, Content: "Hello"},
			{Role: llm.RoleAssistant, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if resp.Content != "收到 3 条消息" {
		t.Errorf("期望 '收到 3 条消息'，实际为 '%s'", resp.Content)
	}
}

// TestLLMProviderWithResponseFnOverridesResponses 测试自定义函数优先于预定义响应
func TestLLMProviderWithResponseFnOverridesResponses(t *testing.T) {
	p := NewLLMProvider("test")
	// 先添加预定义响应
	p.AddResponse("预定义响应")
	// 再设置自定义函数
	p.WithResponseFn(func(req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		return &llm.CompletionResponse{Content: "自定义响应"}, nil
	})

	ctx := context.Background()
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	// 应该返回自定义函数的响应，而非预定义响应
	if resp.Content != "自定义响应" {
		t.Errorf("期望 '自定义响应'，实际为 '%s'", resp.Content)
	}
}

// TestLLMProviderCompleteCalls 测试调用记录（Calls/LastCall/CallCount）
func TestLLMProviderCompleteCalls(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("回复1").AddResponse("回复2")

	ctx := context.Background()

	req1 := llm.CompletionRequest{
		Model:    "model-a",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "第一次"}},
	}
	req2 := llm.CompletionRequest{
		Model:    "model-b",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "第二次"}},
	}

	_, _ = p.Complete(ctx, req1)
	_, _ = p.Complete(ctx, req2)

	// 验证 Calls
	calls := p.Calls()
	if len(calls) != 2 {
		t.Fatalf("期望 2 条调用记录，实际为 %d", len(calls))
	}
	if calls[0].Model != "model-a" {
		t.Errorf("期望第一次调用模型为 'model-a'，实际为 '%s'", calls[0].Model)
	}
	if calls[1].Model != "model-b" {
		t.Errorf("期望第二次调用模型为 'model-b'，实际为 '%s'", calls[1].Model)
	}

	// 验证 LastCall
	lastCall := p.LastCall()
	if lastCall == nil {
		t.Fatal("期望最后调用不为 nil")
	}
	if lastCall.Model != "model-b" {
		t.Errorf("期望最后调用模型为 'model-b'，实际为 '%s'", lastCall.Model)
	}

	// 验证 CallCount
	if p.CallCount() != 2 {
		t.Errorf("期望调用次数为 2，实际为 %d", p.CallCount())
	}
}

// TestLLMProviderCompleteContextCancelled 测试 context 取消时的行为
func TestLLMProviderCompleteContextCancelled(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("不应该返回")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err == nil {
		t.Fatal("期望返回错误")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled 错误，实际为 '%v'", err)
	}

	if resp != nil {
		t.Error("期望响应为 nil")
	}

	// 即使 context 取消，调用也应该被记录
	if p.CallCount() != 1 {
		t.Errorf("期望调用被记录，调用次数为 1，实际为 %d", p.CallCount())
	}
}

// TestLLMProviderCompleteNoMoreResponses 测试无更多响应时报错
func TestLLMProviderCompleteNoMoreResponses(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("唯一的回复")

	ctx := context.Background()

	// 第一次调用成功
	_, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("第一次调用不应该失败: %v", err)
	}

	// 第二次调用应该失败
	_, err = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Again"}},
	})
	if err == nil {
		t.Fatal("期望无更多响应时返回错误")
	}

	if err.Error() != "no more mock responses" {
		t.Errorf("期望错误消息为 'no more mock responses'，实际为 '%s'", err.Error())
	}
}

// TestLLMProviderStream 测试流式返回
func TestLLMProviderStream(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("流式内容")

	ctx := context.Background()
	stream, err := p.Stream(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err != nil {
		t.Fatalf("流式调用失败: %v", err)
	}

	if stream == nil {
		t.Fatal("期望流对象不为 nil")
	}

	// 验证调用被记录（Stream 内部调用了 Complete）
	if p.CallCount() != 1 {
		t.Errorf("期望调用次数为 1，实际为 %d", p.CallCount())
	}
}

// TestLLMProviderStreamError 测试流式返回错误
func TestLLMProviderStreamError(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddErrorResponse(errors.New("流式错误"))

	ctx := context.Background()
	stream, err := p.Stream(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err == nil {
		t.Fatal("期望返回错误")
	}

	if stream != nil {
		t.Error("期望流对象为 nil")
	}
}

// TestLLMProviderModels 测试返回模型列表
func TestLLMProviderModels(t *testing.T) {
	p := NewLLMProvider("test")
	models := p.Models()

	if len(models) != 1 {
		t.Fatalf("期望 1 个模型，实际为 %d", len(models))
	}

	if models[0].ID != "mock-model" {
		t.Errorf("期望模型 ID 为 'mock-model'，实际为 '%s'", models[0].ID)
	}

	if models[0].Name != "Mock Model" {
		t.Errorf("期望模型名称为 'Mock Model'，实际为 '%s'", models[0].Name)
	}
}

// TestLLMProviderCountTokens 测试 Token 估算
func TestLLMProviderCountTokens(t *testing.T) {
	p := NewLLMProvider("test")

	tests := []struct {
		name     string
		messages []llm.Message
		expected int
	}{
		{
			name:     "空消息列表",
			messages: nil,
			expected: 0,
		},
		{
			name: "单条短消息",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hi!!"}, // 4 字符 = 1 token
			},
			expected: 1,
		},
		{
			name: "多条消息",
			messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "12345678"},   // 8/4 = 2
				{Role: llm.RoleUser, Content: "123456789012"}, // 12/4 = 3
			},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := p.CountTokens(tt.messages)
			if err != nil {
				t.Fatalf("CountTokens 失败: %v", err)
			}
			if count != tt.expected {
				t.Errorf("期望 %d tokens，实际为 %d", tt.expected, count)
			}
		})
	}
}

// TestLLMProviderReset 测试重置状态
func TestLLMProviderReset(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("回复1").AddResponse("回复2")

	ctx := context.Background()

	// 消耗一个响应
	_, _ = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	// 验证有调用记录
	if p.CallCount() != 1 {
		t.Fatalf("重置前期望 1 次调用，实际为 %d", p.CallCount())
	}

	// 重置
	p.Reset()

	// 验证调用记录被清空
	if p.CallCount() != 0 {
		t.Errorf("重置后期望 0 次调用，实际为 %d", p.CallCount())
	}

	// 验证响应索引回到起始位置
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("重置后调用失败: %v", err)
	}
	if resp.Content != "回复1" {
		t.Errorf("重置后期望从第一个响应开始，得到 '%s'", resp.Content)
	}
}

// TestEchoProvider 测试回声 Provider
func TestEchoProvider(t *testing.T) {
	p := EchoProvider()

	if p.Name() != "echo" {
		t.Errorf("期望名称为 'echo'，实际为 '%s'", p.Name())
	}

	ctx := context.Background()

	// 有用户消息时回声
	resp, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "系统提示"},
			{Role: llm.RoleUser, Content: "Hello World"},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if resp.Content != "Echo: Hello World" {
		t.Errorf("期望 'Echo: Hello World'，实际为 '%s'", resp.Content)
	}

	// 回声最后一条用户消息
	resp, err = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "第一条"},
			{Role: llm.RoleAssistant, Content: "回复"},
			{Role: llm.RoleUser, Content: "最后一条"},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if resp.Content != "Echo: 最后一条" {
		t.Errorf("期望 'Echo: 最后一条'，实际为 '%s'", resp.Content)
	}

	// 无用户消息时
	resp, err = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "只有系统消息"},
		},
	})
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if resp.Content != "No user message found" {
		t.Errorf("期望 'No user message found'，实际为 '%s'", resp.Content)
	}
}

// TestFixedProvider 测试固定响应 Provider
func TestFixedProvider(t *testing.T) {
	p := FixedProvider("固定回复")

	if p.Name() != "fixed" {
		t.Errorf("期望名称为 'fixed'，实际为 '%s'", p.Name())
	}

	ctx := context.Background()

	// 多次调用都应返回相同内容
	for i := 0; i < 5; i++ {
		resp, err := p.Complete(ctx, llm.CompletionRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf("消息%d", i)}},
		})
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
		if resp.Content != "固定回复" {
			t.Errorf("第 %d 次调用期望 '固定回复'，实际为 '%s'", i+1, resp.Content)
		}
	}
}

// TestErrorProvider 测试错误 Provider
func TestErrorProvider(t *testing.T) {
	expectedErr := errors.New("模型不可用")
	p := ErrorProvider(expectedErr)

	if p.Name() != "error" {
		t.Errorf("期望名称为 'error'，实际为 '%s'", p.Name())
	}

	ctx := context.Background()

	// 每次调用都应返回错误
	for i := 0; i < 3; i++ {
		resp, err := p.Complete(ctx, llm.CompletionRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		})
		if err == nil {
			t.Fatalf("第 %d 次调用期望返回错误", i+1)
		}
		if err != expectedErr {
			t.Errorf("第 %d 次调用期望原始错误对象", i+1)
		}
		if resp != nil {
			t.Errorf("第 %d 次调用期望响应为 nil", i+1)
		}
	}
}

// TestSequenceProvider 测试序列响应 Provider
func TestSequenceProvider(t *testing.T) {
	p := SequenceProvider("第一", "第二", "第三")

	if p.Name() != "sequence" {
		t.Errorf("期望名称为 'sequence'，实际为 '%s'", p.Name())
	}

	ctx := context.Background()
	expected := []string{"第一", "第二", "第三"}

	for i, exp := range expected {
		resp, err := p.Complete(ctx, llm.CompletionRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		})
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
		if resp.Content != exp {
			t.Errorf("第 %d 次调用期望 '%s'，实际为 '%s'", i+1, exp, resp.Content)
		}
	}

	// 超出序列长度应该报错
	_, err := p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("超出序列长度应该返回错误")
	}
}

// TestLLMProviderConcurrency 测试 LLM Provider 并发安全
func TestLLMProviderConcurrency(t *testing.T) {
	p := FixedProvider("并发响应")

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx := context.Background()
			resp, err := p.Complete(ctx, llm.CompletionRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf("并发消息 %d", idx)}},
			})
			if err != nil {
				errs <- fmt.Errorf("goroutine %d 调用失败: %v", idx, err)
				return
			}
			if resp.Content != "并发响应" {
				errs <- fmt.Errorf("goroutine %d 期望 '并发响应'，实际为 '%s'", idx, resp.Content)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	// 验证所有调用都被记录
	if p.CallCount() != goroutines {
		t.Errorf("期望 %d 次调用，实际为 %d", goroutines, p.CallCount())
	}
}

// TestLLMProviderMixedResponses 测试混合响应（正常+错误+工具调用）
func TestLLMProviderMixedResponses(t *testing.T) {
	p := NewLLMProvider("mixed")
	p.AddResponse("普通回复")
	p.AddErrorResponse(errors.New("中间错误"))
	p.AddToolCallResponse([]llm.ToolCall{
		{ID: "call_1", Name: "tool_a", Arguments: "{}"},
	})

	ctx := context.Background()
	req := llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
	}

	// 第一次：普通回复
	resp, err := p.Complete(ctx, req)
	if err != nil {
		t.Fatalf("第 1 次调用不应失败: %v", err)
	}
	if resp.Content != "普通回复" {
		t.Errorf("期望 '普通回复'，实际为 '%s'", resp.Content)
	}

	// 第二次：错误
	_, err = p.Complete(ctx, req)
	if err == nil {
		t.Fatal("第 2 次调用应该返回错误")
	}

	// 第三次：工具调用
	resp, err = p.Complete(ctx, req)
	if err != nil {
		t.Fatalf("第 3 次调用不应失败: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("期望 1 个工具调用，实际为 %d", len(resp.ToolCalls))
	}
}

// TestLLMProviderCallsReturnsCopy 测试 Calls 返回副本而非引用
func TestLLMProviderCallsReturnsCopy(t *testing.T) {
	p := NewLLMProvider("test")
	p.AddResponse("ok")

	ctx := context.Background()
	_, _ = p.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	calls1 := p.Calls()
	calls2 := p.Calls()

	// 修改 calls1 不应影响 calls2 或内部数据
	if len(calls1) != 1 || len(calls2) != 1 {
		t.Fatal("两次 Calls() 应该返回相同数量的记录")
	}
}

// ============================================================================
// Mock Memory 测试
// ============================================================================

// TestNewMemory 测试创建 Mock Memory
func TestNewMemory(t *testing.T) {
	m := NewMemory()

	stats := m.Stats()
	if stats.EntryCount != 0 {
		t.Errorf("期望初始条目数为 0，实际为 %d", stats.EntryCount)
	}

	entries := m.Entries()
	if len(entries) != 0 {
		t.Errorf("期望初始条目映射为空，实际有 %d 条", len(entries))
	}
}

// TestMemorySaveAndGet 测试保存和获取
func TestMemorySaveAndGet(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// 保存带 ID 的条目
	entry := memory.Entry{
		ID:      "entry-1",
		Role:    "user",
		Content: "Hello World",
	}
	err := m.Save(ctx, entry)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 获取条目
	got, err := m.Get(ctx, "entry-1")
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	if got == nil {
		t.Fatal("期望获取到条目，实际为 nil")
	}
	if got.Content != "Hello World" {
		t.Errorf("期望内容为 'Hello World'，实际为 '%s'", got.Content)
	}
	if got.Role != "user" {
		t.Errorf("期望角色为 'user'，实际为 '%s'", got.Role)
	}
}

// TestMemorySaveAutoID 测试保存时自动生成 ID
func TestMemorySaveAutoID(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// 不设置 ID
	entry := memory.Entry{
		Role:    "assistant",
		Content: "Auto ID test",
	}
	err := m.Save(ctx, entry)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证条目被保存（通过 Stats 确认）
	stats := m.Stats()
	if stats.EntryCount != 1 {
		t.Errorf("期望条目数为 1，实际为 %d", stats.EntryCount)
	}

	// 验证保存调用被记录
	saveCalls := m.SaveCalls()
	if len(saveCalls) != 1 {
		t.Fatalf("期望 1 条保存记录，实际为 %d", len(saveCalls))
	}
}

// TestMemorySaveAutoTimestamp 测试保存时自动设置时间戳
func TestMemorySaveAutoTimestamp(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	entry := memory.Entry{
		ID:      "ts-test",
		Content: "Timestamp test",
	}
	err := m.Save(ctx, entry)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	got, _ := m.Get(ctx, "ts-test")
	if got == nil {
		t.Fatal("期望获取到条目")
	}
	if got.CreatedAt.IsZero() {
		t.Error("期望 CreatedAt 被自动设置")
	}
}

// TestMemorySaveBatch 测试批量保存
func TestMemorySaveBatch(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	entries := []memory.Entry{
		{ID: "batch-1", Content: "第一条"},
		{ID: "batch-2", Content: "第二条"},
		{ID: "batch-3", Content: "第三条"},
	}

	err := m.SaveBatch(ctx, entries)
	if err != nil {
		t.Fatalf("批量保存失败: %v", err)
	}

	stats := m.Stats()
	if stats.EntryCount != 3 {
		t.Errorf("期望 3 条记录，实际为 %d", stats.EntryCount)
	}

	// 验证保存调用被记录
	if len(m.SaveCalls()) != 3 {
		t.Errorf("期望 3 条保存调用记录，实际为 %d", len(m.SaveCalls()))
	}
}

// TestMemoryGetNotFound 测试获取不存在的条目
func TestMemoryGetNotFound(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	got, err := m.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("获取不应返回错误: %v", err)
	}
	if got != nil {
		t.Error("期望不存在的条目返回 nil")
	}
}

// TestMemorySearch 测试搜索功能
func TestMemorySearch(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// 准备测试数据
	m.AddEntry(memory.Entry{ID: "s1", Content: "Go 语言入门", Role: "user"})
	m.AddEntry(memory.Entry{ID: "s2", Content: "Python 编程", Role: "user"})
	m.AddEntry(memory.Entry{ID: "s3", Content: "Go 并发编程", Role: "assistant"})

	// 按内容搜索
	results, err := m.Search(ctx, memory.SearchQuery{Query: "Go"})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("期望 2 条结果，实际为 %d", len(results))
	}
}

// TestMemorySearchIgnoreCase 测试搜索忽略大小写
func TestMemorySearchIgnoreCase(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{ID: "ic1", Content: "Hello World"})
	m.AddEntry(memory.Entry{ID: "ic2", Content: "hello world"})
	m.AddEntry(memory.Entry{ID: "ic3", Content: "HELLO WORLD"})

	results, err := m.Search(ctx, memory.SearchQuery{Query: "hello"})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("期望 3 条结果（忽略大小写），实际为 %d", len(results))
	}
}

// TestMemorySearchByRole 测试按角色搜索
func TestMemorySearchByRole(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{ID: "r1", Content: "用户消息1", Role: "user"})
	m.AddEntry(memory.Entry{ID: "r2", Content: "助手回复1", Role: "assistant"})
	m.AddEntry(memory.Entry{ID: "r3", Content: "用户消息2", Role: "user"})

	results, err := m.Search(ctx, memory.SearchQuery{Roles: []string{"user"}})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("期望 2 条 user 角色结果，实际为 %d", len(results))
	}
}

// TestMemorySearchByMetadata 测试按元数据搜索
func TestMemorySearchByMetadata(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{
		ID:       "m1",
		Content:  "带元数据的条目",
		Metadata: map[string]any{"source": "web", "lang": "zh"},
	})
	m.AddEntry(memory.Entry{
		ID:       "m2",
		Content:  "另一个条目",
		Metadata: map[string]any{"source": "api", "lang": "en"},
	})
	m.AddEntry(memory.Entry{
		ID:      "m3",
		Content: "无元数据",
	})

	results, err := m.Search(ctx, memory.SearchQuery{
		Metadata: map[string]any{"source": "web"},
	})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("期望 1 条结果，实际为 %d", len(results))
	}
}

// TestMemorySearchWithLimit 测试搜索结果数量限制
func TestMemorySearchWithLimit(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// 添加多条匹配的条目
	for i := 0; i < 10; i++ {
		m.AddEntry(memory.Entry{
			ID:      fmt.Sprintf("limit-%d", i),
			Content: "匹配内容",
		})
	}

	results, err := m.Search(ctx, memory.SearchQuery{
		Query: "匹配",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("期望最多 3 条结果，实际为 %d", len(results))
	}
}

// TestMemorySearchEmptyQuery 测试空查询返回所有
func TestMemorySearchEmptyQuery(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{ID: "eq1", Content: "条目1"})
	m.AddEntry(memory.Entry{ID: "eq2", Content: "条目2"})

	results, err := m.Search(ctx, memory.SearchQuery{})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("空查询期望返回所有 2 条记录，实际为 %d", len(results))
	}
}

// TestMemorySearchCalls 测试搜索调用记录
func TestMemorySearchCalls(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	_, _ = m.Search(ctx, memory.SearchQuery{Query: "第一次"})
	_, _ = m.Search(ctx, memory.SearchQuery{Query: "第二次"})

	calls := m.SearchCalls()
	if len(calls) != 2 {
		t.Fatalf("期望 2 条搜索记录，实际为 %d", len(calls))
	}
	if calls[0].Query != "第一次" {
		t.Errorf("期望第一次搜索为 '第一次'，实际为 '%s'", calls[0].Query)
	}
	if calls[1].Query != "第二次" {
		t.Errorf("期望第二次搜索为 '第二次'，实际为 '%s'", calls[1].Query)
	}
}

// TestMemoryDelete 测试删除
func TestMemoryDelete(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{ID: "del-1", Content: "要删除的"})
	m.AddEntry(memory.Entry{ID: "del-2", Content: "保留的"})

	err := m.Delete(ctx, "del-1")
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 验证已删除
	got, _ := m.Get(ctx, "del-1")
	if got != nil {
		t.Error("期望删除后获取为 nil")
	}

	// 验证其他条目未受影响
	got, _ = m.Get(ctx, "del-2")
	if got == nil {
		t.Error("期望 del-2 仍然存在")
	}
}

// TestMemoryDeleteNonExistent 测试删除不存在的条目不报错
func TestMemoryDeleteNonExistent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	err := m.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("删除不存在的条目不应报错，但得到: %v", err)
	}
}

// TestMemoryClear 测试清空
func TestMemoryClear(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntries([]memory.Entry{
		{ID: "c1", Content: "条目1"},
		{ID: "c2", Content: "条目2"},
		{ID: "c3", Content: "条目3"},
	})

	err := m.Clear(ctx)
	if err != nil {
		t.Fatalf("清空失败: %v", err)
	}

	stats := m.Stats()
	if stats.EntryCount != 0 {
		t.Errorf("清空后期望 0 条记录，实际为 %d", stats.EntryCount)
	}
}

// TestMemoryContextCancelled 测试 context 取消
func TestMemoryContextCancelled(t *testing.T) {
	m := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Save 应该返回 context 错误
	err := m.Save(ctx, memory.Entry{ID: "test", Content: "test"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Save 期望 context.Canceled，实际为 %v", err)
	}

	// Get 应该返回 context 错误
	_, err = m.Get(ctx, "test")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Get 期望 context.Canceled，实际为 %v", err)
	}

	// Search 应该返回 context 错误
	_, err = m.Search(ctx, memory.SearchQuery{Query: "test"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Search 期望 context.Canceled，实际为 %v", err)
	}

	// Delete 应该返回 context 错误
	err = m.Delete(ctx, "test")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Delete 期望 context.Canceled，实际为 %v", err)
	}

	// Clear 应该返回 context 错误
	err = m.Clear(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Clear 期望 context.Canceled，实际为 %v", err)
	}
}

// TestMemoryReset 测试重置
func TestMemoryReset(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	m.AddEntry(memory.Entry{ID: "r1", Content: "test"})
	_ = m.Save(ctx, memory.Entry{ID: "r2", Content: "saved"})
	_, _ = m.Search(ctx, memory.SearchQuery{Query: "test"})

	m.Reset()

	// 验证所有内容被清空
	stats := m.Stats()
	if stats.EntryCount != 0 {
		t.Errorf("重置后期望 0 条记录，实际为 %d", stats.EntryCount)
	}
	if len(m.SaveCalls()) != 0 {
		t.Errorf("重置后期望 0 条保存记录，实际为 %d", len(m.SaveCalls()))
	}
	if len(m.SearchCalls()) != 0 {
		t.Errorf("重置后期望 0 条搜索记录，实际为 %d", len(m.SearchCalls()))
	}
}

// TestMemoryAddEntries 测试批量添加条目
func TestMemoryAddEntries(t *testing.T) {
	m := NewMemory()

	entries := []memory.Entry{
		{ID: "ae1", Content: "内容1"},
		{ID: "ae2", Content: "内容2"},
	}
	m.AddEntries(entries)

	stats := m.Stats()
	if stats.EntryCount != 2 {
		t.Errorf("期望 2 条记录，实际为 %d", stats.EntryCount)
	}
}

// TestMemoryEntriesReturnsCopy 测试 Entries 返回副本
func TestMemoryEntriesReturnsCopy(t *testing.T) {
	m := NewMemory()
	m.AddEntry(memory.Entry{ID: "copy-test", Content: "test"})

	entries1 := m.Entries()
	entries2 := m.Entries()

	// 修改 entries1 不应影响内部数据
	delete(entries1, "copy-test")

	if len(entries2) != 1 {
		t.Error("修改 Entries() 返回值不应影响内部数据")
	}
}

// TestMemoryConcurrency 测试 Memory 并发安全
func TestMemoryConcurrency(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup

	// 并发保存
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = m.Save(ctx, memory.Entry{
				ID:      fmt.Sprintf("concurrent-%d", idx),
				Content: fmt.Sprintf("内容 %d", idx),
			})
		}(i)
	}

	// 并发搜索
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = m.Search(ctx, memory.SearchQuery{Query: "内容"})
		}(i)
	}

	wg.Wait()

	stats := m.Stats()
	if stats.EntryCount != goroutines {
		t.Errorf("期望 %d 条记录，实际为 %d", goroutines, stats.EntryCount)
	}
}

// ============================================================================
// Mock Tool 测试
// ============================================================================

// TestNewTool 测试创建 Mock Tool
func TestNewTool(t *testing.T) {
	tool := NewTool("test-tool")

	if tool.Name() != "test-tool" {
		t.Errorf("期望名称为 'test-tool'，实际为 '%s'", tool.Name())
	}

	if tool.Description() != "Mock tool for testing" {
		t.Errorf("期望默认描述，实际为 '%s'", tool.Description())
	}

	if tool.CallCount() != 0 {
		t.Errorf("期望初始调用次数为 0，实际为 %d", tool.CallCount())
	}
}

// TestNewToolWithOptions 测试带选项创建
func TestNewToolWithOptions(t *testing.T) {
	s := &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Schema{
			"name": {Type: "string"},
		},
	}

	tool := NewTool("custom",
		WithToolDescription("自定义工具"),
		WithToolSchema(s),
	)

	if tool.Description() != "自定义工具" {
		t.Errorf("期望描述为 '自定义工具'，实际为 '%s'", tool.Description())
	}

	gotSchema := tool.Schema()
	if gotSchema.Type != "object" {
		t.Errorf("期望 Schema 类型为 'object'，实际为 '%s'", gotSchema.Type)
	}

	if _, ok := gotSchema.Properties["name"]; !ok {
		t.Error("期望 Schema 包含 'name' 属性")
	}
}

// TestToolDefaultSchema 测试默认 Schema
func TestToolDefaultSchema(t *testing.T) {
	tool := NewTool("default-schema")
	s := tool.Schema()

	if s.Type != "object" {
		t.Errorf("期望默认 Schema 类型为 'object'，实际为 '%s'", s.Type)
	}

	if _, ok := s.Properties["input"]; !ok {
		t.Error("期望默认 Schema 包含 'input' 属性")
	}
}

// TestToolExecute 测试执行
func TestToolExecute(t *testing.T) {
	tool := NewTool("test")
	tool.AddResult("结果1").AddResult("结果2")

	ctx := context.Background()

	// 按顺序返回结果
	result, err := tool.Execute(ctx, map[string]any{"key": "val1"})
	if err != nil {
		t.Fatalf("第一次执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望结果成功")
	}

	result, err = tool.Execute(ctx, map[string]any{"key": "val2"})
	if err != nil {
		t.Fatalf("第二次执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望第二次结果也成功")
	}
}

// TestToolExecuteDefault 测试超出预定义结果时的默认返回
func TestToolExecuteDefault(t *testing.T) {
	tool := NewTool("default-result")

	ctx := context.Background()
	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望默认结果成功")
	}
}

// TestToolExecuteWithFn 测试自定义执行函数
func TestToolExecuteWithFn(t *testing.T) {
	importedTool := NewTool("fn-tool", WithToolExecuteFn(func(ctx context.Context, args map[string]any) (toolpkg.Result, error) {
		name, _ := args["name"].(string)
		return toolpkg.NewResult(fmt.Sprintf("Hello, %s!", name)), nil
	}))

	ctx := context.Background()
	result, err := importedTool.Execute(ctx, map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望结果成功")
	}
}

// TestToolExecuteRecordsCalls 测试执行记录调用
func TestToolExecuteRecordsCalls(t *testing.T) {
	tool := NewTool("record-test")

	ctx := context.Background()

	_, _ = tool.Execute(ctx, map[string]any{"a": 1})
	_, _ = tool.Execute(ctx, map[string]any{"b": 2})
	_, _ = tool.Execute(ctx, map[string]any{"c": 3})

	if tool.CallCount() != 3 {
		t.Errorf("期望 3 次调用，实际为 %d", tool.CallCount())
	}

	calls := tool.Calls()
	if len(calls) != 3 {
		t.Fatalf("期望 3 条调用记录，实际为 %d", len(calls))
	}

	// 验证参数被正确记录
	if calls[0]["a"] != 1 {
		t.Errorf("期望第一次调用参数 a=1")
	}
	if calls[1]["b"] != 2 {
		t.Errorf("期望第二次调用参数 b=2")
	}
	if calls[2]["c"] != 3 {
		t.Errorf("期望第三次调用参数 c=3")
	}

	// 验证 LastCall
	lastCall := tool.LastCall()
	if lastCall == nil {
		t.Fatal("期望最后调用不为 nil")
	}
	if lastCall["c"] != 3 {
		t.Error("期望最后调用参数 c=3")
	}
}

// TestToolLastCallEmpty 测试无调用时 LastCall 返回 nil
func TestToolLastCallEmpty(t *testing.T) {
	tool := NewTool("empty")
	if tool.LastCall() != nil {
		t.Error("期望无调用时 LastCall 返回 nil")
	}
}

// TestToolExecuteArgsCopy 测试参数被复制（修改原参数不影响记录）
func TestToolExecuteArgsCopy(t *testing.T) {
	tool := NewTool("copy-test")
	ctx := context.Background()

	args := map[string]any{"key": "original"}
	_, _ = tool.Execute(ctx, args)

	// 修改原参数
	args["key"] = "modified"

	// 验证记录中的参数未受影响
	calls := tool.Calls()
	if calls[0]["key"] != "original" {
		t.Error("期望调用记录中参数为原始值 'original'")
	}
}

// TestToolExecuteContextCancelled 测试 context 取消
func TestToolExecuteContextCancelled(t *testing.T) {
	tool := NewTool("ctx-test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tool.Execute(ctx, map[string]any{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled 错误，实际为 %v", err)
	}

	// 调用仍然应该被记录
	if tool.CallCount() != 1 {
		t.Errorf("期望调用被记录，次数为 1，实际为 %d", tool.CallCount())
	}
}

// TestToolValidate 测试验证（默认通过）
func TestToolValidate(t *testing.T) {
	tool := NewTool("validate-test")

	err := tool.Validate(map[string]any{"any": "value"})
	if err != nil {
		t.Errorf("默认验证不应返回错误: %v", err)
	}

	err = tool.Validate(nil)
	if err != nil {
		t.Errorf("nil 参数验证不应返回错误: %v", err)
	}
}

// TestToolReset 测试重置状态
func TestToolReset(t *testing.T) {
	tool := NewTool("reset-test")
	tool.AddResult("r1").AddResult("r2")

	ctx := context.Background()

	// 消耗第一个结果
	_, _ = tool.Execute(ctx, map[string]any{})

	// 重置
	tool.Reset()

	if tool.CallCount() != 0 {
		t.Errorf("重置后期望 0 次调用，实际为 %d", tool.CallCount())
	}

	// 验证结果索引回到起始位置
	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("重置后执行失败: %v", err)
	}
	if !result.Success {
		t.Error("重置后应该从第一个结果开始")
	}
}

// TestEchoTool 测试回声工具
func TestEchoTool(t *testing.T) {
	tool := EchoTool()

	if tool.Name() != "echo" {
		t.Errorf("期望名称为 'echo'，实际为 '%s'", tool.Name())
	}

	ctx := context.Background()
	args := map[string]any{"msg": "Hello", "count": 42}
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望结果成功")
	}
}

// TestFixedTool 测试固定结果工具
func TestFixedTool(t *testing.T) {
	tool := FixedTool("fixed", "固定结果")

	if tool.Name() != "fixed" {
		t.Errorf("期望名称为 'fixed'，实际为 '%s'", tool.Name())
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		result, err := tool.Execute(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("第 %d 次执行失败: %v", i+1, err)
		}
		if !result.Success {
			t.Errorf("第 %d 次期望结果成功", i+1)
		}
	}
}

// TestErrorTool 测试错误工具
func TestErrorTool(t *testing.T) {
	expectedErr := errors.New("工具错误")
	tool := ErrorTool("error-tool", expectedErr)

	if tool.Name() != "error-tool" {
		t.Errorf("期望名称为 'error-tool'，实际为 '%s'", tool.Name())
	}

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if err != expectedErr {
		t.Errorf("期望原始错误对象")
	}
}

// TestCalculatorTool 测试计算器工具
func TestCalculatorTool(t *testing.T) {
	calc := CalculatorTool()

	if calc.Name() != "calculator" {
		t.Errorf("期望名称为 'calculator'，实际为 '%s'", calc.Name())
	}

	if calc.Description() != "A simple calculator" {
		t.Errorf("期望描述为 'A simple calculator'，实际为 '%s'", calc.Description())
	}

	// 验证 Schema 有必需属性
	s := calc.Schema()
	if len(s.Required) != 3 {
		t.Errorf("期望 3 个必需属性，实际为 %d", len(s.Required))
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		args      map[string]any
		expectErr bool
	}{
		{
			name: "加法",
			args: map[string]any{"operation": "add", "a": 3.0, "b": 2.0},
		},
		{
			name: "减法",
			args: map[string]any{"operation": "subtract", "a": 10.0, "b": 4.0},
		},
		{
			name: "乘法",
			args: map[string]any{"operation": "multiply", "a": 5.0, "b": 3.0},
		},
		{
			name: "除法",
			args: map[string]any{"operation": "divide", "a": 10.0, "b": 2.0},
		},
		{
			name:      "除零",
			args:      map[string]any{"operation": "divide", "a": 10.0, "b": 0.0},
			expectErr: true,
		},
		{
			name:      "未知操作",
			args:      map[string]any{"operation": "power", "a": 2.0, "b": 3.0},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calc.Execute(ctx, tt.args)
			if tt.expectErr && err == nil {
				t.Error("期望返回错误")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("不期望错误: %v", err)
			}
		})
	}
}

// TestSearchTool 测试搜索工具
func TestSearchTool(t *testing.T) {
	results := []string{"结果1", "结果2", "结果3"}
	search := SearchTool(results)

	if search.Name() != "search" {
		t.Errorf("期望名称为 'search'，实际为 '%s'", search.Name())
	}

	ctx := context.Background()
	result, err := search.Execute(ctx, map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !result.Success {
		t.Error("期望结果成功")
	}
}

// TestToolConcurrency 测试 Tool 并发安全
func TestToolConcurrency(t *testing.T) {
	tool := FixedTool("concurrent", "ok")

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = tool.Execute(context.Background(), map[string]any{"idx": idx})
		}(i)
	}

	wg.Wait()

	if tool.CallCount() != goroutines {
		t.Errorf("期望 %d 次调用，实际为 %d", goroutines, tool.CallCount())
	}
}

// ============================================================================
// Mock Retriever 测试
// ============================================================================

// TestNewRetriever 测试创建 Mock Retriever
func TestNewRetriever(t *testing.T) {
	r := NewRetriever()

	calls := r.RetrieveCalls()
	if len(calls) != 0 {
		t.Errorf("期望初始检索调用为空，实际有 %d 条", len(calls))
	}
}

// TestNewRetrieverWithDocuments 测试带文档创建
func TestNewRetrieverWithDocuments(t *testing.T) {
	docs := SimpleDocuments("文档1", "文档2", "文档3")
	r := NewRetriever(WithDocuments(docs))

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("期望 3 条结果，实际为 %d", len(results))
	}
}

// TestRetrieverRetrieve 测试基本检索
func TestRetrieverRetrieve(t *testing.T) {
	r := NewRetriever()
	r.AddDocument(rag.Document{ID: "d1", Content: "文档1", Score: 0.9})
	r.AddDocument(rag.Document{ID: "d2", Content: "文档2", Score: 0.8})
	r.AddDocument(rag.Document{ID: "d3", Content: "文档3", Score: 0.7})

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "查询")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}

	// 默认 TopK=5，所有文档都应返回
	if len(results) != 3 {
		t.Errorf("期望 3 条结果，实际为 %d", len(results))
	}
}

// TestRetrieverRetrieveWithTopK 测试 TopK 限制
func TestRetrieverRetrieveWithTopK(t *testing.T) {
	docs := SimpleDocuments("d1", "d2", "d3", "d4", "d5")
	r := FixedRetriever(docs)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test", rag.WithTopK(2))
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("期望 2 条结果 (TopK=2)，实际为 %d", len(results))
	}
}

// TestRetrieverRetrieveTopKExceedsDocCount 测试 TopK 超出文档数量
func TestRetrieverRetrieveTopKExceedsDocCount(t *testing.T) {
	docs := SimpleDocuments("d1", "d2")
	r := FixedRetriever(docs)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test", rag.WithTopK(10))
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	// 应该返回所有可用文档
	if len(results) != 2 {
		t.Errorf("期望 2 条结果（所有可用），实际为 %d", len(results))
	}
}

// TestRetrieverWithRetrieveFn 测试自定义检索函数
func TestRetrieverWithRetrieveFn(t *testing.T) {
	r := NewRetriever(WithRetrieveFn(func(ctx context.Context, query string, opts ...rag.RetrieveOption) ([]rag.Document, error) {
		return []rag.Document{
			{ID: "custom", Content: "自定义结果: " + query},
		}, nil
	}))

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "我的查询")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际为 %d", len(results))
	}
	if results[0].Content != "自定义结果: 我的查询" {
		t.Errorf("期望 '自定义结果: 我的查询'，实际为 '%s'", results[0].Content)
	}
}

// TestRetrieverAddDocuments 测试批量添加文档
func TestRetrieverAddDocuments(t *testing.T) {
	r := NewRetriever()

	docs := []rag.Document{
		{ID: "d1", Content: "内容1"},
		{ID: "d2", Content: "内容2"},
	}
	r.AddDocuments(docs)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("期望 2 条结果，实际为 %d", len(results))
	}
}

// TestRetrieverRetrieveCalls 测试检索调用记录
func TestRetrieverRetrieveCalls(t *testing.T) {
	r := NewRetriever()

	ctx := context.Background()
	_, _ = r.Retrieve(ctx, "查询1")
	_, _ = r.Retrieve(ctx, "查询2")
	_, _ = r.Retrieve(ctx, "查询3")

	calls := r.RetrieveCalls()
	if len(calls) != 3 {
		t.Fatalf("期望 3 条调用记录，实际为 %d", len(calls))
	}
	if calls[0] != "查询1" {
		t.Errorf("期望第一次查询为 '查询1'，实际为 '%s'", calls[0])
	}
	if calls[1] != "查询2" {
		t.Errorf("期望第二次查询为 '查询2'，实际为 '%s'", calls[1])
	}
	if calls[2] != "查询3" {
		t.Errorf("期望第三次查询为 '查询3'，实际为 '%s'", calls[2])
	}
}

// TestRetrieverContextCancelled 测试 context 取消
func TestRetrieverContextCancelled(t *testing.T) {
	r := NewRetriever()
	r.AddDocument(rag.Document{ID: "d1", Content: "test"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Retrieve(ctx, "test")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled 错误，实际为 %v", err)
	}

	// 调用仍然应该被记录
	if len(r.RetrieveCalls()) != 1 {
		t.Errorf("期望调用被记录")
	}
}

// TestRetrieverReset 测试重置
func TestRetrieverReset(t *testing.T) {
	r := NewRetriever()
	r.AddDocument(rag.Document{ID: "d1", Content: "test"})

	ctx := context.Background()
	_, _ = r.Retrieve(ctx, "query1")
	_, _ = r.Retrieve(ctx, "query2")

	r.Reset()

	calls := r.RetrieveCalls()
	if len(calls) != 0 {
		t.Errorf("重置后期望 0 条调用记录，实际为 %d", len(calls))
	}
}

// TestEmptyRetriever 测试空检索器
func TestEmptyRetriever(t *testing.T) {
	r := EmptyRetriever()

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "anything")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("期望空结果，实际有 %d 条", len(results))
	}
}

// TestFixedRetriever 测试固定结果检索器
func TestFixedRetriever(t *testing.T) {
	docs := SimpleDocuments("fixed1", "fixed2")
	r := FixedRetriever(docs)

	ctx := context.Background()

	// 多次检索应该返回相同结果
	for i := 0; i < 3; i++ {
		results, err := r.Retrieve(ctx, fmt.Sprintf("query%d", i))
		if err != nil {
			t.Fatalf("第 %d 次检索失败: %v", i+1, err)
		}
		if len(results) != 2 {
			t.Errorf("第 %d 次期望 2 条结果，实际为 %d", i+1, len(results))
		}
	}
}

// TestErrorRetriever 测试错误检索器
func TestErrorRetriever(t *testing.T) {
	expectedErr := errors.New("检索失败")
	r := ErrorRetriever(expectedErr)

	ctx := context.Background()
	results, err := r.Retrieve(ctx, "test")
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if err != expectedErr {
		t.Errorf("期望原始错误对象")
	}
	if results != nil {
		t.Error("期望结果为 nil")
	}
}

// TestSimpleDocuments 测试 SimpleDocuments 辅助函数
func TestSimpleDocuments(t *testing.T) {
	docs := SimpleDocuments("A", "B", "C")

	if len(docs) != 3 {
		t.Fatalf("期望 3 个文档，实际为 %d", len(docs))
	}

	// 验证 ID 格式
	for i, doc := range docs {
		expectedID := fmt.Sprintf("doc-%d", i+1)
		if doc.ID != expectedID {
			t.Errorf("期望 ID '%s'，实际为 '%s'", expectedID, doc.ID)
		}
	}

	// 验证内容
	if docs[0].Content != "A" {
		t.Errorf("期望内容 'A'，实际为 '%s'", docs[0].Content)
	}
	if docs[1].Content != "B" {
		t.Errorf("期望内容 'B'，实际为 '%s'", docs[1].Content)
	}
	if docs[2].Content != "C" {
		t.Errorf("期望内容 'C'，实际为 '%s'", docs[2].Content)
	}

	// 验证分数递减
	if docs[0].Score <= docs[1].Score {
		t.Error("期望第一个文档分数高于第二个")
	}
	if docs[1].Score <= docs[2].Score {
		t.Error("期望第二个文档分数高于第三个")
	}
}

// TestRetrieverConcurrency 测试 Retriever 并发安全
func TestRetrieverConcurrency(t *testing.T) {
	docs := SimpleDocuments("d1", "d2", "d3")
	r := FixedRetriever(docs)

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results, err := r.Retrieve(context.Background(), fmt.Sprintf("query-%d", idx))
			if err != nil {
				t.Errorf("goroutine %d 检索失败: %v", idx, err)
				return
			}
			if len(results) != 3 {
				t.Errorf("goroutine %d 期望 3 条结果，实际为 %d", idx, len(results))
			}
		}(i)
	}

	wg.Wait()

	calls := r.RetrieveCalls()
	if len(calls) != goroutines {
		t.Errorf("期望 %d 条调用记录，实际为 %d", goroutines, len(calls))
	}
}

