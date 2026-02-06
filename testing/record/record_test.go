// Package record 提供测试的录制和回放功能
//
// 本文件包含 record 包所有组件的全面测试：
//   - Cassette: 录制会话的创建、添加、查找、保存和加载
//   - Recorder: LLM 调用录制器
//   - Replayer: LLM 调用回放器（严格模式和回退模式）
//   - ToolCassette: Tool 录制会话
//   - RAGCassette: RAG 录制会话
//   - FixtureManager: 测试固件管理
//   - ScenarioManager: 测试场景管理
//   - SessionRecorder: 完整会话录制器
//   - AssertionHelper: 断言辅助工具
//   - TestGenerator: 测试生成器
package record

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/hexagon/rag"
	"github.com/everyday-items/hexagon/testing/mock"
)

// ============================================================================
// Cassette 测试
// ============================================================================

// TestNewCassette 测试创建 Cassette
func TestNewCassette(t *testing.T) {
	c := NewCassette("test-session")

	if c.Name != "test-session" {
		t.Errorf("期望名称为 'test-session'，实际为 '%s'", c.Name)
	}

	if len(c.Interactions) != 0 {
		t.Errorf("期望初始交互列表为空，实际有 %d 条", len(c.Interactions))
	}

	if c.Metadata == nil {
		t.Error("期望 Metadata 不为 nil")
	}

	if c.CreatedAt.IsZero() {
		t.Error("期望 CreatedAt 被设置")
	}

	if c.UpdatedAt.IsZero() {
		t.Error("期望 UpdatedAt 被设置")
	}
}

// TestCassetteAddInteraction 测试添加交互
func TestCassetteAddInteraction(t *testing.T) {
	c := NewCassette("test")
	beforeUpdate := c.UpdatedAt

	// 等待一小段时间以确保时间戳不同
	time.Sleep(time.Millisecond)

	interaction := Interaction{
		ID: "int_1",
		Request: llm.CompletionRequest{
			Model:    "test-model",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		},
		Response: &llm.CompletionResponse{
			Content: "Hi there",
		},
		RequestHash: "hash123",
	}

	c.AddInteraction(interaction)

	if len(c.Interactions) != 1 {
		t.Fatalf("期望 1 条交互，实际为 %d", len(c.Interactions))
	}

	if c.Interactions[0].ID != "int_1" {
		t.Errorf("期望交互 ID 为 'int_1'，实际为 '%s'", c.Interactions[0].ID)
	}

	// UpdatedAt 应该被更新
	if !c.UpdatedAt.After(beforeUpdate) {
		t.Error("期望 UpdatedAt 在添加交互后更新")
	}
}

// TestCassetteFindByHash 测试通过哈希查找交互
func TestCassetteFindByHash(t *testing.T) {
	c := NewCassette("test")

	c.AddInteraction(Interaction{
		ID:          "int_1",
		RequestHash: "hash_aaa",
		Response:    &llm.CompletionResponse{Content: "Response A"},
	})
	c.AddInteraction(Interaction{
		ID:          "int_2",
		RequestHash: "hash_bbb",
		Response:    &llm.CompletionResponse{Content: "Response B"},
	})

	// 查找存在的哈希
	found := c.FindByHash("hash_bbb")
	if found == nil {
		t.Fatal("期望找到匹配的交互")
	}
	if found.ID != "int_2" {
		t.Errorf("期望找到 int_2，实际为 '%s'", found.ID)
	}

	// 查找不存在的哈希
	notFound := c.FindByHash("nonexistent")
	if notFound != nil {
		t.Error("期望未找到时返回 nil")
	}
}

// TestCassetteSaveAndLoad 测试保存和加载
func TestCassetteSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_cassette.json")

	// 创建并填充 Cassette
	original := NewCassette("save-test")
	original.Description = "测试保存和加载"
	original.Metadata["version"] = "1.0"
	original.AddInteraction(Interaction{
		ID: "int_1",
		Request: llm.CompletionRequest{
			Model:    "gpt-4",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "What is Go?"}},
		},
		Response: &llm.CompletionResponse{
			Content: "Go is a programming language",
			Usage:   llm.Usage{TotalTokens: 50},
		},
		Duration:    100 * time.Millisecond,
		Timestamp:   time.Now(),
		RequestHash: "hash_save_test",
	})

	// 保存
	err := original.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("期望文件存在")
	}

	// 加载
	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 验证内容
	if loaded.Name != "save-test" {
		t.Errorf("期望名称为 'save-test'，实际为 '%s'", loaded.Name)
	}
	if loaded.Description != "测试保存和加载" {
		t.Errorf("期望描述不匹配")
	}
	if len(loaded.Interactions) != 1 {
		t.Fatalf("期望 1 条交互，实际为 %d", len(loaded.Interactions))
	}
	if loaded.Interactions[0].Response.Content != "Go is a programming language" {
		t.Errorf("期望响应内容不匹配")
	}
}

// TestCassetteSaveCreatesDir 测试保存时自动创建目录
func TestCassetteSaveCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "cassette.json")

	c := NewCassette("dir-test")
	err := c.Save(path)
	if err != nil {
		t.Fatalf("保存到嵌套目录失败: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("期望文件存在")
	}
}

// TestLoadCassetteNotFound 测试加载不存在的文件
func TestLoadCassetteNotFound(t *testing.T) {
	_, err := LoadCassette("/nonexistent/path/cassette.json")
	if err == nil {
		t.Fatal("期望加载不存在的文件返回错误")
	}
}

// TestLoadCassetteInvalidJSON 测试加载无效 JSON
func TestLoadCassetteInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(path, []byte("invalid json content"), 0644)

	_, err := LoadCassette(path)
	if err == nil {
		t.Fatal("期望加载无效 JSON 返回错误")
	}
}

// TestCassetteSaveWithErrorInteraction 测试保存包含错误的交互
func TestCassetteSaveWithErrorInteraction(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error_interaction.json")

	c := NewCassette("error-test")
	c.AddInteraction(Interaction{
		ID:          "int_err",
		Error:       "connection timeout",
		RequestHash: "hash_err",
	})

	err := c.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Interactions[0].Error != "connection timeout" {
		t.Errorf("期望错误信息为 'connection timeout'，实际为 '%s'", loaded.Interactions[0].Error)
	}
}

// ============================================================================
// Recorder 测试
// ============================================================================

// TestNewRecorder 测试创建录制器
func TestNewRecorder(t *testing.T) {
	provider := mock.FixedProvider("test response")
	recorder := NewRecorder(provider, "test-recording")

	// 名称应该包含 "_recorder" 后缀
	expectedName := "fixed_recorder"
	if recorder.Name() != expectedName {
		t.Errorf("期望名称为 '%s'，实际为 '%s'", expectedName, recorder.Name())
	}

	cassette := recorder.Cassette()
	if cassette == nil {
		t.Fatal("期望 Cassette 不为 nil")
	}
	if cassette.Name != "test-recording" {
		t.Errorf("期望 Cassette 名称为 'test-recording'，实际为 '%s'", cassette.Name)
	}
}

// TestRecorderComplete 测试录制 Complete 调用
func TestRecorderComplete(t *testing.T) {
	provider := mock.FixedProvider("recorded response")
	recorder := NewRecorder(provider, "recording")

	ctx := context.Background()
	req := llm.CompletionRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	resp, err := recorder.Complete(ctx, req)
	if err != nil {
		t.Fatalf("录制调用失败: %v", err)
	}

	if resp.Content != "recorded response" {
		t.Errorf("期望 'recorded response'，实际为 '%s'", resp.Content)
	}

	// 验证交互被录制
	cassette := recorder.Cassette()
	if len(cassette.Interactions) != 1 {
		t.Fatalf("期望 1 条录制交互，实际为 %d", len(cassette.Interactions))
	}

	interaction := cassette.Interactions[0]
	if interaction.Response == nil {
		t.Fatal("期望录制的响应不为 nil")
	}
	if interaction.Response.Content != "recorded response" {
		t.Errorf("期望录制响应为 'recorded response'")
	}
	if interaction.RequestHash == "" {
		t.Error("期望请求哈希不为空")
	}
	if interaction.Duration <= 0 {
		t.Error("期望持续时间大于 0")
	}
}

// TestRecorderCompleteError 测试录制错误响应
func TestRecorderCompleteError(t *testing.T) {
	provider := mock.ErrorProvider(errors.New("provider error"))
	recorder := NewRecorder(provider, "error-recording")

	ctx := context.Background()
	_, err := recorder.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err == nil {
		t.Fatal("期望返回错误")
	}

	// 验证错误也被录制
	cassette := recorder.Cassette()
	if len(cassette.Interactions) != 1 {
		t.Fatalf("期望 1 条录制交互，实际为 %d", len(cassette.Interactions))
	}

	interaction := cassette.Interactions[0]
	if interaction.Error != "provider error" {
		t.Errorf("期望录制的错误为 'provider error'，实际为 '%s'", interaction.Error)
	}
	if interaction.Response != nil {
		t.Error("期望录制的响应为 nil")
	}
}

// TestRecorderMultipleComplete 测试录制多次调用
func TestRecorderMultipleComplete(t *testing.T) {
	provider := mock.SequenceProvider("第一", "第二", "第三")
	recorder := NewRecorder(provider, "multi-recording")

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := recorder.Complete(ctx, llm.CompletionRequest{
			Model:    "model",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf("消息%d", i)}},
		})
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
	}

	cassette := recorder.Cassette()
	if len(cassette.Interactions) != 3 {
		t.Errorf("期望 3 条录制交互，实际为 %d", len(cassette.Interactions))
	}
}

// TestRecorderStream 测试流式录制（代理到底层 provider）
func TestRecorderStream(t *testing.T) {
	provider := mock.FixedProvider("stream content")
	recorder := NewRecorder(provider, "stream-recording")

	ctx := context.Background()
	stream, err := recorder.Stream(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	if err != nil {
		t.Fatalf("流式调用失败: %v", err)
	}

	if stream == nil {
		t.Fatal("期望流对象不为 nil")
	}
}

// TestRecorderModels 测试模型列表代理
func TestRecorderModels(t *testing.T) {
	provider := mock.FixedProvider("test")
	recorder := NewRecorder(provider, "models-test")

	models := recorder.Models()
	if len(models) == 0 {
		t.Error("期望返回模型列表")
	}
}

// TestRecorderCountTokens 测试 Token 计数代理
func TestRecorderCountTokens(t *testing.T) {
	provider := mock.FixedProvider("test")
	recorder := NewRecorder(provider, "tokens-test")

	count, err := recorder.CountTokens([]llm.Message{
		{Role: llm.RoleUser, Content: "12345678"}, // 8/4 = 2
	})
	if err != nil {
		t.Fatalf("CountTokens 失败: %v", err)
	}
	if count != 2 {
		t.Errorf("期望 2 tokens，实际为 %d", count)
	}
}

// TestRecorderSave 测试保存录制
func TestRecorderSave(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "recorder_save.json")

	provider := mock.FixedProvider("save test")
	recorder := NewRecorder(provider, "save-recording")

	ctx := context.Background()
	_, _ = recorder.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	err := recorder.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证可以加载
	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(loaded.Interactions) != 1 {
		t.Errorf("期望 1 条交互，实际为 %d", len(loaded.Interactions))
	}
}

// TestRecorderConcurrency 测试录制器并发安全
func TestRecorderConcurrency(t *testing.T) {
	provider := mock.FixedProvider("concurrent")
	recorder := NewRecorder(provider, "concurrent-recording")

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = recorder.Complete(context.Background(), llm.CompletionRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf("msg-%d", idx)}},
			})
		}(i)
	}

	wg.Wait()

	cassette := recorder.Cassette()
	if len(cassette.Interactions) != goroutines {
		t.Errorf("期望 %d 条交互，实际为 %d", goroutines, len(cassette.Interactions))
	}
}

// ============================================================================
// Replayer 测试
// ============================================================================

// TestNewReplayer 测试创建回放器
func TestNewReplayer(t *testing.T) {
	c := NewCassette("replay-test")
	r := NewReplayer(c)

	if r.Name() != "replayer" {
		t.Errorf("期望名称为 'replayer'，实际为 '%s'", r.Name())
	}

	// 默认严格模式
	hits, misses := r.Stats()
	if hits != 0 || misses != 0 {
		t.Errorf("期望初始命中/未命中为 0/0，实际为 %d/%d", hits, misses)
	}
}

// TestReplayerCompleteHit 测试回放命中
func TestReplayerCompleteHit(t *testing.T) {
	// 先录制
	provider := mock.FixedProvider("replayed response")
	recorder := NewRecorder(provider, "replay")

	ctx := context.Background()
	req := llm.CompletionRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "What is Go?"}},
	}
	_, _ = recorder.Complete(ctx, req)

	// 回放
	cassette := recorder.Cassette()
	replayer := NewReplayer(cassette)

	resp, err := replayer.Complete(ctx, req)
	if err != nil {
		t.Fatalf("回放失败: %v", err)
	}

	if resp.Content != "replayed response" {
		t.Errorf("期望 'replayed response'，实际为 '%s'", resp.Content)
	}

	hits, misses := replayer.Stats()
	if hits != 1 {
		t.Errorf("期望 1 次命中，实际为 %d", hits)
	}
	if misses != 0 {
		t.Errorf("期望 0 次未命中，实际为 %d", misses)
	}
}

// TestReplayerCompleteMissStrict 测试严格模式下未匹配
func TestReplayerCompleteMissStrict(t *testing.T) {
	c := NewCassette("empty")
	r := NewReplayer(c, WithReplayMode(ReplayModeStrict))

	ctx := context.Background()
	_, err := r.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "No match"}},
	})

	if err == nil {
		t.Fatal("期望严格模式下未匹配时返回错误")
	}

	hits, misses := r.Stats()
	if hits != 0 {
		t.Errorf("期望 0 次命中，实际为 %d", hits)
	}
	if misses != 1 {
		t.Errorf("期望 1 次未命中，实际为 %d", misses)
	}
}

// TestReplayerCompleteMissFallback 测试回退模式
func TestReplayerCompleteMissFallback(t *testing.T) {
	c := NewCassette("empty")
	fallbackProvider := mock.FixedProvider("fallback response")

	r := NewReplayer(c,
		WithReplayMode(ReplayModeFallback),
		WithFallbackProvider(fallbackProvider),
	)

	ctx := context.Background()
	resp, err := r.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Fallback test"}},
	})

	if err != nil {
		t.Fatalf("回退模式不应返回错误: %v", err)
	}

	if resp.Content != "fallback response" {
		t.Errorf("期望 'fallback response'，实际为 '%s'", resp.Content)
	}

	_, misses := r.Stats()
	if misses != 1 {
		t.Errorf("期望 1 次未命中（使用回退），实际为 %d", misses)
	}
}

// TestReplayerCompleteErrorInteraction 测试回放包含错误的交互
func TestReplayerCompleteErrorInteraction(t *testing.T) {
	// 录制一个错误响应
	provider := mock.ErrorProvider(errors.New("original error"))
	recorder := NewRecorder(provider, "error-replay")

	ctx := context.Background()
	req := llm.CompletionRequest{
		Model:    "model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Error test"}},
	}
	_, _ = recorder.Complete(ctx, req)

	// 回放应该返回相同的错误
	replayer := NewReplayer(recorder.Cassette())
	_, err := replayer.Complete(ctx, req)

	if err == nil {
		t.Fatal("期望回放时也返回错误")
	}
	if err.Error() != "original error" {
		t.Errorf("期望错误为 'original error'，实际为 '%s'", err.Error())
	}
}

// TestReplayerStreamWithFallback 测试流式回放使用回退
func TestReplayerStreamWithFallback(t *testing.T) {
	c := NewCassette("test")
	fallbackProvider := mock.FixedProvider("stream fallback")

	r := NewReplayer(c, WithFallbackProvider(fallbackProvider))

	ctx := context.Background()
	stream, err := r.Stream(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Stream"}},
	})

	if err != nil {
		t.Fatalf("流式回放失败: %v", err)
	}
	if stream == nil {
		t.Fatal("期望流对象不为 nil")
	}
}

// TestReplayerStreamWithoutFallback 测试无回退时流式回放
func TestReplayerStreamWithoutFallback(t *testing.T) {
	c := NewCassette("test")
	r := NewReplayer(c)

	ctx := context.Background()
	_, err := r.Stream(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Stream"}},
	})

	if err == nil {
		t.Fatal("期望无回退时流式回放返回错误")
	}
}

// TestReplayerModels 测试回放器模型列表
func TestReplayerModels(t *testing.T) {
	c := NewCassette("test")
	r := NewReplayer(c)

	models := r.Models()
	if len(models) != 1 {
		t.Fatalf("期望 1 个模型，实际为 %d", len(models))
	}
	if models[0].ID != "replay-model" {
		t.Errorf("期望模型 ID 为 'replay-model'，实际为 '%s'", models[0].ID)
	}
}

// TestReplayerCountTokensWithFallback 测试有回退时的 Token 计数
func TestReplayerCountTokensWithFallback(t *testing.T) {
	c := NewCassette("test")
	fallback := mock.FixedProvider("test")

	r := NewReplayer(c, WithFallbackProvider(fallback))

	count, err := r.CountTokens([]llm.Message{
		{Role: llm.RoleUser, Content: "12345678"}, // 8/4 = 2
	})
	if err != nil {
		t.Fatalf("CountTokens 失败: %v", err)
	}
	if count != 2 {
		t.Errorf("期望 2 tokens，实际为 %d", count)
	}
}

// TestReplayerCountTokensWithoutFallback 测试无回退时的 Token 计数（简单估算）
func TestReplayerCountTokensWithoutFallback(t *testing.T) {
	c := NewCassette("test")
	r := NewReplayer(c)

	count, err := r.CountTokens([]llm.Message{
		{Role: llm.RoleUser, Content: "12345678"}, // 8/4 = 2
	})
	if err != nil {
		t.Fatalf("CountTokens 失败: %v", err)
	}
	if count != 2 {
		t.Errorf("期望 2 tokens，实际为 %d", count)
	}
}

// TestRecordAndReplayRoundTrip 测试完整的录制-保存-加载-回放流程
func TestRecordAndReplayRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.json")

	// 1. 录制
	provider := mock.SequenceProvider("答案A", "答案B")
	recorder := NewRecorder(provider, "roundtrip")

	ctx := context.Background()
	reqA := llm.CompletionRequest{
		Model:    "model-1",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "问题A"}},
	}
	reqB := llm.CompletionRequest{
		Model:    "model-1",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "问题B"}},
	}

	_, _ = recorder.Complete(ctx, reqA)
	_, _ = recorder.Complete(ctx, reqB)

	// 2. 保存
	err := recorder.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 3. 加载
	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 4. 回放
	replayer := NewReplayer(loaded)

	respA, err := replayer.Complete(ctx, reqA)
	if err != nil {
		t.Fatalf("回放 A 失败: %v", err)
	}
	if respA.Content != "答案A" {
		t.Errorf("期望回放响应 '答案A'，实际为 '%s'", respA.Content)
	}

	respB, err := replayer.Complete(ctx, reqB)
	if err != nil {
		t.Fatalf("回放 B 失败: %v", err)
	}
	if respB.Content != "答案B" {
		t.Errorf("期望回放响应 '答案B'，实际为 '%s'", respB.Content)
	}

	hits, _ := replayer.Stats()
	if hits != 2 {
		t.Errorf("期望 2 次命中，实际为 %d", hits)
	}
}

// TestHashRequestDeterministic 测试请求哈希的确定性
func TestHashRequestDeterministic(t *testing.T) {
	req := llm.CompletionRequest{
		Model:    "model-a",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	hash1 := hashRequest(req)
	hash2 := hashRequest(req)

	if hash1 != hash2 {
		t.Errorf("相同请求的哈希应该相同: '%s' vs '%s'", hash1, hash2)
	}

	// 不同的请求应产生不同的哈希
	reqDiff := llm.CompletionRequest{
		Model:    "model-b",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}
	hash3 := hashRequest(reqDiff)
	if hash1 == hash3 {
		t.Error("不同请求的哈希不应相同")
	}
}

// ============================================================================
// ToolCassette 测试
// ============================================================================

// TestNewToolCassette 测试创建 Tool Cassette
func TestNewToolCassette(t *testing.T) {
	c := NewToolCassette("tool-test")

	if c.Name != "tool-test" {
		t.Errorf("期望名称为 'tool-test'，实际为 '%s'", c.Name)
	}
	if len(c.Interactions) != 0 {
		t.Errorf("期望初始交互为空")
	}
	if c.CreatedAt.IsZero() {
		t.Error("期望 CreatedAt 被设置")
	}
}

// TestToolCassetteAddAndFind 测试添加和查找
func TestToolCassetteAddAndFind(t *testing.T) {
	c := NewToolCassette("test")

	c.AddInteraction(ToolInteraction{
		ID:          "ti_1",
		ToolName:    "calculator",
		Args:        map[string]any{"a": 1, "b": 2},
		Result:      "3",
		RequestHash: "tool_hash_1",
	})

	found := c.FindByHash("tool_hash_1")
	if found == nil {
		t.Fatal("期望找到匹配的交互")
	}
	if found.ToolName != "calculator" {
		t.Errorf("期望工具名为 'calculator'，实际为 '%s'", found.ToolName)
	}

	// 不存在的哈希
	notFound := c.FindByHash("nonexistent")
	if notFound != nil {
		t.Error("期望未找到时返回 nil")
	}
}

// TestToolCassetteSaveAndLoad 测试 Tool Cassette 保存和加载
func TestToolCassetteSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "tool_cassette.json")

	original := NewToolCassette("save-test")
	original.AddInteraction(ToolInteraction{
		ID:          "ti_1",
		ToolName:    "search",
		Args:        map[string]any{"query": "Go"},
		Result:      "Go is great",
		Duration:    50 * time.Millisecond,
		Timestamp:   time.Now(),
		RequestHash: "hash_tool_save",
	})

	err := original.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadToolCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("期望名称为 'save-test'")
	}
	if len(loaded.Interactions) != 1 {
		t.Fatalf("期望 1 条交互")
	}
	if loaded.Interactions[0].Result != "Go is great" {
		t.Error("期望结果匹配")
	}
}

// TestLoadToolCassetteNotFound 测试加载不存在的文件
func TestLoadToolCassetteNotFound(t *testing.T) {
	_, err := LoadToolCassette("/nonexistent/path.json")
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// TestLoadToolCassetteInvalidJSON 测试加载无效 JSON
func TestLoadToolCassetteInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(path, []byte("{invalid"), 0644)

	_, err := LoadToolCassette(path)
	if err == nil {
		t.Fatal("期望加载无效 JSON 返回错误")
	}
}

// ============================================================================
// RAGCassette 测试
// ============================================================================

// TestNewRAGCassette 测试创建 RAG Cassette
func TestNewRAGCassette(t *testing.T) {
	c := NewRAGCassette("rag-test")

	if c.Name != "rag-test" {
		t.Errorf("期望名称为 'rag-test'，实际为 '%s'", c.Name)
	}
	if len(c.Interactions) != 0 {
		t.Errorf("期望初始交互为空")
	}
}

// TestRAGCassetteAddAndFind 测试添加和查找
func TestRAGCassetteAddAndFind(t *testing.T) {
	c := NewRAGCassette("test")

	c.AddInteraction(RAGInteraction{
		ID:        "ri_1",
		Operation: "retrieve",
		Query:     "What is Go?",
		Documents: []rag.Document{
			{ID: "d1", Content: "Go is a language", Score: 0.9},
		},
		RequestHash: "rag_hash_1",
	})

	found := c.FindByHash("rag_hash_1")
	if found == nil {
		t.Fatal("期望找到匹配的交互")
	}
	if found.Operation != "retrieve" {
		t.Errorf("期望操作为 'retrieve'，实际为 '%s'", found.Operation)
	}
	if len(found.Documents) != 1 {
		t.Errorf("期望 1 个文档")
	}
}

// TestRAGCassetteSaveAndLoad 测试 RAG Cassette 保存和加载
func TestRAGCassetteSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "rag_cassette.json")

	original := NewRAGCassette("save-test")
	original.AddInteraction(RAGInteraction{
		ID:        "ri_1",
		Operation: "retrieve",
		Query:     "Go concurrency",
		Documents: []rag.Document{
			{ID: "d1", Content: "goroutines", Score: 0.95},
		},
		RequestHash: "hash_rag_save",
	})

	err := original.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadRAGCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("期望名称为 'save-test'")
	}
	if len(loaded.Interactions) != 1 {
		t.Fatalf("期望 1 条交互")
	}
	if loaded.Interactions[0].Query != "Go concurrency" {
		t.Error("期望查询匹配")
	}
}

// TestLoadRAGCassetteNotFound 测试加载不存在的文件
func TestLoadRAGCassetteNotFound(t *testing.T) {
	_, err := LoadRAGCassette("/nonexistent/path.json")
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// TestLoadRAGCassetteInvalidJSON 测试加载无效 JSON
func TestLoadRAGCassetteInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)

	_, err := LoadRAGCassette(path)
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// ============================================================================
// FixtureManager 测试
// ============================================================================

// TestFixtureManagerSaveAndLoad 测试固件保存和加载
func TestFixtureManagerSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	fixture := &Fixture{
		Name:        "test-fixture",
		Description: "测试固件",
		Data: map[string]any{
			"key1": "value1",
			"key2": float64(42),
		},
		CreatedAt: time.Now(),
	}

	err := fm.Save(fixture)
	if err != nil {
		t.Fatalf("保存固件失败: %v", err)
	}

	// 加载固件
	loaded, err := fm.Load("test-fixture")
	if err != nil {
		t.Fatalf("加载固件失败: %v", err)
	}

	if loaded.Name != "test-fixture" {
		t.Errorf("期望名称为 'test-fixture'，实际为 '%s'", loaded.Name)
	}
	if loaded.Data["key1"] != "value1" {
		t.Errorf("期望 key1 为 'value1'")
	}
}

// TestFixtureManagerLoadCached 测试固件缓存（双重检查锁）
func TestFixtureManagerLoadCached(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	fixture := &Fixture{
		Name: "cached",
		Data: map[string]any{"k": "v"},
	}
	_ = fm.Save(fixture)

	// 第一次加载（从文件）
	f1, err := fm.Load("cached")
	if err != nil {
		t.Fatalf("第一次加载失败: %v", err)
	}

	// 第二次加载（从缓存）
	f2, err := fm.Load("cached")
	if err != nil {
		t.Fatalf("第二次加载失败: %v", err)
	}

	// 两次加载应该返回相同的指针（缓存命中）
	if f1 != f2 {
		t.Error("期望第二次加载返回缓存的相同指针")
	}
}

// TestFixtureManagerLoadNotFound 测试加载不存在的固件
func TestFixtureManagerLoadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	_, err := fm.Load("nonexistent")
	if err == nil {
		t.Fatal("期望加载不存在的固件返回错误")
	}
}

// TestFixtureManagerGetAndSet 测试 Get 和 Set 操作
func TestFixtureManagerGetAndSet(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	// Set 到新固件
	err := fm.Set("my-fixture", "name", "Alice")
	if err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	// Get 值
	val, err := fm.Get("my-fixture", "name")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if val != "Alice" {
		t.Errorf("期望 'Alice'，实际为 '%v'", val)
	}

	// Get 不存在的键
	_, err = fm.Get("my-fixture", "nonexistent")
	if err == nil {
		t.Fatal("期望获取不存在的键返回错误")
	}
}

// TestFixtureManagerSetUpdate 测试 Set 更新已有固件
func TestFixtureManagerSetUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	_ = fm.Set("update-test", "key1", "original")
	_ = fm.Set("update-test", "key2", "added")

	val1, _ := fm.Get("update-test", "key1")
	val2, _ := fm.Get("update-test", "key2")

	if val1 != "original" {
		t.Errorf("期望 key1 为 'original'，实际为 '%v'", val1)
	}
	if val2 != "added" {
		t.Errorf("期望 key2 为 'added'，实际为 '%v'", val2)
	}
}

// TestFixtureManagerConcurrency 测试固件管理器并发安全
func TestFixtureManagerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFixtureManager(tmpDir)

	// 先创建固件
	_ = fm.Save(&Fixture{
		Name: "concurrent",
		Data: map[string]any{"initial": true},
	})

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = fm.Load("concurrent")
		}(i)
	}

	wg.Wait()
}

// ============================================================================
// ScenarioManager 测试
// ============================================================================

// TestScenarioManagerSaveAndLoad 测试场景保存和加载
func TestScenarioManagerSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewScenarioManager(tmpDir)

	scenario := &TestScenario{
		Name:        "login-flow",
		Description: "测试登录流程",
		Steps: []TestStep{
			{
				Name:   "发送登录请求",
				Action: "call_llm",
				Input:  map[string]any{"prompt": "用户要登录"},
			},
			{
				Name:   "验证响应",
				Action: "assert",
				Expected: map[string]any{
					"contains": "登录成功",
				},
			},
		},
		Fixtures: []string{"users", "sessions"},
	}

	err := sm.Save(scenario)
	if err != nil {
		t.Fatalf("保存场景失败: %v", err)
	}

	loaded, err := sm.Load("login-flow")
	if err != nil {
		t.Fatalf("加载场景失败: %v", err)
	}

	if loaded.Name != "login-flow" {
		t.Errorf("期望名称为 'login-flow'")
	}
	if len(loaded.Steps) != 2 {
		t.Errorf("期望 2 个步骤，实际为 %d", len(loaded.Steps))
	}
	if loaded.Steps[0].Action != "call_llm" {
		t.Errorf("期望第一步动作为 'call_llm'")
	}
}

// TestScenarioManagerLoadCached 测试场景缓存
func TestScenarioManagerLoadCached(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewScenarioManager(tmpDir)

	_ = sm.Save(&TestScenario{
		Name:  "cached-scenario",
		Steps: []TestStep{{Name: "step1", Action: "call_llm"}},
	})

	s1, _ := sm.Load("cached-scenario")
	s2, _ := sm.Load("cached-scenario")

	if s1 != s2 {
		t.Error("期望第二次加载返回缓存的指针")
	}
}

// TestScenarioManagerLoadNotFound 测试加载不存在的场景
func TestScenarioManagerLoadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewScenarioManager(tmpDir)

	_, err := sm.Load("nonexistent")
	if err == nil {
		t.Fatal("期望加载不存在的场景返回错误")
	}
}

// ============================================================================
// SessionRecorder 测试
// ============================================================================

// TestNewSessionRecorder 测试创建会话录制器
func TestNewSessionRecorder(t *testing.T) {
	sr := NewSessionRecorder("session-1")

	if sr.name != "session-1" {
		t.Errorf("期望名称为 'session-1'，实际为 '%s'", sr.name)
	}

	if sr.llmCassette == nil {
		t.Error("期望 LLM Cassette 不为 nil")
	}
	if sr.toolCassette == nil {
		t.Error("期望 Tool Cassette 不为 nil")
	}
	if sr.ragCassette == nil {
		t.Error("期望 RAG Cassette 不为 nil")
	}
}

// TestSessionRecorderRecordLLM 测试录制 LLM 交互
func TestSessionRecorderRecordLLM(t *testing.T) {
	sr := NewSessionRecorder("test")

	sr.RecordLLM(Interaction{
		ID: "llm_1",
		Request: llm.CompletionRequest{
			Model: "gpt-4",
		},
		Timestamp: time.Now(),
	})

	if len(sr.llmCassette.Interactions) != 1 {
		t.Errorf("期望 1 条 LLM 交互，实际为 %d", len(sr.llmCassette.Interactions))
	}
	if len(sr.events) != 1 {
		t.Errorf("期望 1 条事件，实际为 %d", len(sr.events))
	}
	if sr.events[0].Type != "llm" {
		t.Errorf("期望事件类型为 'llm'，实际为 '%s'", sr.events[0].Type)
	}
}

// TestSessionRecorderRecordTool 测试录制 Tool 交互
func TestSessionRecorderRecordTool(t *testing.T) {
	sr := NewSessionRecorder("test")

	sr.RecordTool(ToolInteraction{
		ID:        "tool_1",
		ToolName:  "calculator",
		Timestamp: time.Now(),
	})

	if len(sr.toolCassette.Interactions) != 1 {
		t.Errorf("期望 1 条 Tool 交互")
	}
	if len(sr.events) != 1 {
		t.Errorf("期望 1 条事件")
	}
	if sr.events[0].Type != "tool" {
		t.Errorf("期望事件类型为 'tool'")
	}
}

// TestSessionRecorderRecordRAG 测试录制 RAG 交互
func TestSessionRecorderRecordRAG(t *testing.T) {
	sr := NewSessionRecorder("test")

	sr.RecordRAG(RAGInteraction{
		ID:        "rag_1",
		Operation: "retrieve",
		Timestamp: time.Now(),
	})

	if len(sr.ragCassette.Interactions) != 1 {
		t.Errorf("期望 1 条 RAG 交互")
	}
	if len(sr.events) != 1 {
		t.Errorf("期望 1 条事件")
	}
	if sr.events[0].Type != "rag" {
		t.Errorf("期望事件类型为 'rag'")
	}
}

// TestSessionRecorderRecordEvent 测试录制自定义事件
func TestSessionRecorderRecordEvent(t *testing.T) {
	sr := NewSessionRecorder("test")

	sr.RecordEvent("custom", map[string]any{
		"action": "user_input",
		"data":   "Hello",
	})

	if len(sr.events) != 1 {
		t.Fatalf("期望 1 条事件")
	}
	if sr.events[0].Type != "custom" {
		t.Errorf("期望事件类型为 'custom'")
	}
	if sr.events[0].Data["action"] != "user_input" {
		t.Errorf("期望事件数据包含 action")
	}
}

// TestSessionRecorderSaveAll 测试保存所有录制
func TestSessionRecorderSaveAll(t *testing.T) {
	tmpDir := t.TempDir()
	sr := NewSessionRecorder("session")

	// 录制各种交互
	sr.RecordLLM(Interaction{
		ID: "llm_1",
		Request: llm.CompletionRequest{
			Model: "model",
		},
		Response:  &llm.CompletionResponse{Content: "response"},
		Timestamp: time.Now(),
	})

	sr.RecordTool(ToolInteraction{
		ID:        "tool_1",
		ToolName:  "search",
		Result:    "found",
		Timestamp: time.Now(),
	})

	sr.RecordRAG(RAGInteraction{
		ID:        "rag_1",
		Operation: "retrieve",
		Timestamp: time.Now(),
	})

	sr.RecordEvent("custom", map[string]any{"note": "test"})

	// 保存
	err := sr.SaveAll(tmpDir)
	if err != nil {
		t.Fatalf("保存所有录制失败: %v", err)
	}

	// 验证文件被创建
	expectedFiles := []string{
		"session_llm.json",
		"session_tool.json",
		"session_rag.json",
		"session_events.json",
	}
	for _, fname := range expectedFiles {
		path := filepath.Join(tmpDir, fname)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("期望文件 %s 存在", fname)
		}
	}
}

// TestSessionRecorderSaveAllEmpty 测试空录制不创建文件
func TestSessionRecorderSaveAllEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	sr := NewSessionRecorder("empty-session")

	err := sr.SaveAll(tmpDir)
	if err != nil {
		t.Fatalf("保存空录制失败: %v", err)
	}

	// 验证不应该创建文件（因为没有交互）
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("空录制不应创建文件，但发现 %d 个文件", len(entries))
	}
}

// TestSessionRecorderConcurrency 测试会话录制器并发安全
func TestSessionRecorderConcurrency(t *testing.T) {
	sr := NewSessionRecorder("concurrent")

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(3)

		go func(idx int) {
			defer wg.Done()
			sr.RecordLLM(Interaction{
				ID:        fmt.Sprintf("llm_%d", idx),
				Timestamp: time.Now(),
			})
		}(i)

		go func(idx int) {
			defer wg.Done()
			sr.RecordTool(ToolInteraction{
				ID:        fmt.Sprintf("tool_%d", idx),
				Timestamp: time.Now(),
			})
		}(i)

		go func(idx int) {
			defer wg.Done()
			sr.RecordEvent("custom", map[string]any{"idx": idx})
		}(i)
	}

	wg.Wait()

	totalEvents := len(sr.events)
	expectedEvents := goroutines * 3 // LLM + Tool + Event
	if totalEvents != expectedEvents {
		t.Errorf("期望 %d 条事件，实际为 %d", expectedEvents, totalEvents)
	}
}

// ============================================================================
// AssertionHelper 测试
// ============================================================================

// TestAssertionHelperEqual 测试相等断言
func TestAssertionHelperEqual(t *testing.T) {
	h := NewAssertionHelper()

	// 成功的断言
	result := h.AssertEqual(42, 42, "数字相等")
	if !result {
		t.Error("期望断言成功")
	}

	// 失败的断言
	result = h.AssertEqual("expected", "actual", "字符串不等")
	if result {
		t.Error("期望断言失败")
	}

	if !h.HasErrors() {
		t.Error("期望有错误")
	}

	errs := h.Errors()
	if len(errs) != 1 {
		t.Fatalf("期望 1 条错误，实际为 %d", len(errs))
	}
}

// TestAssertionHelperContains 测试包含断言
func TestAssertionHelperContains(t *testing.T) {
	h := NewAssertionHelper()

	// 成功
	result := h.AssertContains("Hello World", "World", "包含 World")
	if !result {
		t.Error("期望断言成功")
	}

	// 失败
	result = h.AssertContains("Hello", "World", "不包含 World")
	if result {
		t.Error("期望断言失败")
	}

	if len(h.Errors()) != 1 {
		t.Errorf("期望 1 条错误")
	}
}

// TestAssertionHelperNotEmpty 测试非空断言
func TestAssertionHelperNotEmpty(t *testing.T) {
	h := NewAssertionHelper()

	// 成功
	h.AssertNotEmpty("non-empty", "非空字符串")
	h.AssertNotEmpty(42, "非空数字")

	// 失败
	h.AssertNotEmpty("", "空字符串")
	h.AssertNotEmpty(nil, "nil 值")

	errs := h.Errors()
	if len(errs) != 2 {
		t.Errorf("期望 2 条错误，实际为 %d", len(errs))
	}
}

// TestAssertionHelperNoErrors 测试无错误情况
func TestAssertionHelperNoErrors(t *testing.T) {
	h := NewAssertionHelper()

	h.AssertEqual(1, 1, "ok")
	h.AssertContains("abc", "b", "ok")
	h.AssertNotEmpty("x", "ok")

	if h.HasErrors() {
		t.Error("期望无错误")
	}
	if len(h.Errors()) != 0 {
		t.Error("期望错误列表为空")
	}
}

// TestAssertionHelperConcurrency 测试断言辅助工具并发安全
func TestAssertionHelperConcurrency(t *testing.T) {
	h := NewAssertionHelper()

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// 每个 goroutine 添加一个失败的断言
			h.AssertEqual(0, idx, fmt.Sprintf("goroutine %d", idx))
		}(i)
	}

	wg.Wait()

	// goroutine 0 的断言成功（0 == 0），其余失败
	errs := h.Errors()
	if len(errs) != goroutines-1 {
		t.Errorf("期望 %d 条错误，实际为 %d", goroutines-1, len(errs))
	}
}

// ============================================================================
// TestGenerator 测试
// ============================================================================

// TestTestGenerator 测试代码生成器
func TestTestGenerator(t *testing.T) {
	gen := NewTestGenerator("mypackage")

	c := NewCassette("my-test")
	c.AddInteraction(Interaction{
		ID: "int_1",
		Request: llm.CompletionRequest{
			Model:    "gpt-4",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		},
		Response: &llm.CompletionResponse{Content: "Hi"},
	})

	code := gen.GenerateFromCassette(c)

	// 验证生成的代码包含关键内容
	if !containsStr(code, "package mypackage") {
		t.Error("生成的代码应包含 package 声明")
	}
	if !containsStr(code, "TestMy-test") || !containsStr(code, "Test") {
		// 测试函数名包含 cassette 名称
	}
	if !containsStr(code, "record.LoadCassette") {
		t.Error("生成的代码应包含 LoadCassette 调用")
	}
	if !containsStr(code, "record.NewReplayer") {
		t.Error("生成的代码应包含 NewReplayer 调用")
	}
	if !containsStr(code, "replayer.Complete") {
		t.Error("生成的代码应包含 Complete 调用")
	}
}

// TestTestGeneratorEmpty 测试空 Cassette 的代码生成
func TestTestGeneratorEmpty(t *testing.T) {
	gen := NewTestGenerator("pkg")
	c := NewCassette("empty")

	code := gen.GenerateFromCassette(c)
	if !containsStr(code, "package pkg") {
		t.Error("即使空 cassette 也应生成包含 package 的代码")
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

// TestContainsFunction 测试 contains 辅助函数
func TestContainsFunction(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"完全匹配", "hello", "hello", true},
		{"部分匹配", "hello world", "world", true},
		{"开头匹配", "hello world", "hello", true},
		{"无匹配", "hello", "world", false},
		{"空子串", "hello", "", true},
		{"空字符串", "", "hello", false},
		{"两个都空", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v，期望 %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

// TestIsEmptyFunction 测试 isEmpty 辅助函数
func TestIsEmptyFunction(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"nil", nil, true},
		{"空字符串", "", true},
		{"非空字符串", "hello", false},
		{"空切片", []any{}, true},
		{"非空切片", []any{1}, false},
		{"空 map", map[string]any{}, true},
		{"非空 map", map[string]any{"k": "v"}, false},
		{"数字（非空）", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmpty(tt.value)
			if result != tt.expected {
				t.Errorf("isEmpty(%v) = %v，期望 %v", tt.value, result, tt.expected)
			}
		})
	}
}

// containsStr 是一个用于测试的辅助函数（与 advanced.go 中的 contains 区分）
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}
