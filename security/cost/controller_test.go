package cost

import (
	"context"
	"testing"
	"time"
)

func TestNewController(t *testing.T) {
	c := NewController()
	if c == nil {
		t.Fatal("NewController returned nil")
	}

	// 检查默认值
	if c.requestsPerMinute != 60 {
		t.Errorf("expected requestsPerMinute=60, got %d", c.requestsPerMinute)
	}
	if c.maxTokensPerRequest != 8000 {
		t.Errorf("expected maxTokensPerRequest=8000, got %d", c.maxTokensPerRequest)
	}
	if c.maxTokensPerSession != 100000 {
		t.Errorf("expected maxTokensPerSession=100000, got %d", c.maxTokensPerSession)
	}
	if c.maxTokensTotal != 1000000 {
		t.Errorf("expected maxTokensTotal=1000000, got %d", c.maxTokensTotal)
	}
	if c.rateLimiter == nil {
		t.Error("rateLimiter should be initialized")
	}
}

func TestNewController_WithOptions(t *testing.T) {
	c := NewController(
		WithBudget(100.0),
		WithMaxTokensPerRequest(4000),
		WithMaxTokensPerSession(50000),
		WithMaxTokensTotal(500000),
		WithRequestsPerMinute(30),
	)

	if c.budget != 100.0 {
		t.Errorf("expected budget=100.0, got %f", c.budget)
	}
	if c.remaining != 100.0 {
		t.Errorf("expected remaining=100.0, got %f", c.remaining)
	}
	if c.maxTokensPerRequest != 4000 {
		t.Errorf("expected maxTokensPerRequest=4000, got %d", c.maxTokensPerRequest)
	}
	if c.maxTokensPerSession != 50000 {
		t.Errorf("expected maxTokensPerSession=50000, got %d", c.maxTokensPerSession)
	}
	if c.maxTokensTotal != 500000 {
		t.Errorf("expected maxTokensTotal=500000, got %d", c.maxTokensTotal)
	}
	if c.requestsPerMinute != 30 {
		t.Errorf("expected requestsPerMinute=30, got %d", c.requestsPerMinute)
	}
}

func TestController_WithPricing(t *testing.T) {
	customPricing := map[string]ModelPricing{
		"custom-model": {PromptPrice: 0.01, CompletionPrice: 0.02},
	}

	c := NewController(WithPricing(customPricing))

	if pricing, ok := c.pricing["custom-model"]; !ok {
		t.Error("custom pricing not added")
	} else {
		if pricing.PromptPrice != 0.01 {
			t.Errorf("expected PromptPrice=0.01, got %f", pricing.PromptPrice)
		}
	}
}

func TestController_CheckRequest(t *testing.T) {
	c := NewController(
		WithMaxTokensPerRequest(1000),
		WithMaxTokensTotal(5000),
		WithRequestsPerMinute(100), // 设置高一点避免速率限制影响测试
	)
	ctx := context.Background()

	// 正常请求
	err := c.CheckRequest(ctx, 500)
	if err != nil {
		t.Fatalf("CheckRequest failed for valid request: %v", err)
	}

	// 超过单次请求限制
	err = c.CheckRequest(ctx, 1500)
	if err == nil {
		t.Error("expected error for exceeding per-request limit")
	}

	// 累计超过总限制
	c.usedTokens = 4800
	err = c.CheckRequest(ctx, 300)
	if err == nil {
		t.Error("expected error for exceeding total limit")
	}
}

func TestController_CheckRequest_RateLimit(t *testing.T) {
	c := NewController(
		WithRequestsPerMinute(2), // 非常低的限制
	)
	ctx := context.Background()

	// 前两个请求应该成功
	err := c.CheckRequest(ctx, 100)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	err = c.CheckRequest(ctx, 100)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	// 第三个请求应该被限流
	err = c.CheckRequest(ctx, 100)
	if err == nil {
		t.Error("expected rate limit error")
	}
}

func TestController_RecordUsage(t *testing.T) {
	c := NewController(WithBudget(10.0))

	usage := TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}

	err := c.RecordUsage("gpt-4", usage)
	if err != nil {
		t.Fatalf("RecordUsage failed: %v", err)
	}

	// 检查 Token 累计
	if c.usedTokens != 1500 {
		t.Errorf("expected usedTokens=1500, got %d", c.usedTokens)
	}

	// 检查成本计算
	// gpt-4: prompt $0.03/1K, completion $0.06/1K
	// cost = 1000/1000 * 0.03 + 500/1000 * 0.06 = 0.03 + 0.03 = 0.06
	expectedCost := 0.06
	if c.used < expectedCost-0.001 || c.used > expectedCost+0.001 {
		t.Errorf("expected used=~%f, got %f", expectedCost, c.used)
	}
}

func TestController_RecordUsage_BudgetExceeded(t *testing.T) {
	var callbackCalled bool
	c := NewController(
		WithBudget(0.01), // 非常低的预算
		OnBudgetExceeded(func(used, budget float64) {
			callbackCalled = true
		}),
	)

	usage := TokenUsage{
		PromptTokens:     10000,
		CompletionTokens: 10000,
		TotalTokens:      20000,
	}

	err := c.RecordUsage("gpt-4", usage)
	if err == nil {
		t.Error("expected budget exceeded error")
	}

	if !callbackCalled {
		t.Error("onBudgetExceeded callback should be called")
	}
}

func TestController_RecordUsage_UnknownModel(t *testing.T) {
	c := NewController()

	usage := TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 1000,
		TotalTokens:      2000,
	}

	// 未知模型应该使用默认定价
	err := c.RecordUsage("unknown-model", usage)
	if err != nil {
		t.Fatalf("RecordUsage failed: %v", err)
	}

	// 默认定价: prompt $0.001/1K, completion $0.002/1K
	// cost = 1000/1000 * 0.001 + 1000/1000 * 0.002 = 0.003
	expectedCost := 0.003
	if c.used < expectedCost-0.0001 || c.used > expectedCost+0.0001 {
		t.Errorf("expected used=~%f, got %f", expectedCost, c.used)
	}
}

func TestController_Stats(t *testing.T) {
	c := NewController(
		WithBudget(100.0),
		WithMaxTokensTotal(10000),
		WithRequestsPerMinute(60),
	)

	// 记录一些使用量
	c.RecordUsage("gpt-4o-mini", TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	})

	stats := c.Stats()

	if stats.Budget != 100.0 {
		t.Errorf("expected budget=100.0, got %f", stats.Budget)
	}
	if stats.UsedTokens != 150 {
		t.Errorf("expected usedTokens=150, got %d", stats.UsedTokens)
	}
	if stats.MaxTokensTotal != 10000 {
		t.Errorf("expected maxTokensTotal=10000, got %d", stats.MaxTokensTotal)
	}
	if stats.RequestsPerMin != 60 {
		t.Errorf("expected requestsPerMin=60, got %d", stats.RequestsPerMin)
	}
}

func TestController_Reset(t *testing.T) {
	c := NewController(WithBudget(100.0))

	// 记录一些使用量
	c.RecordUsage("gpt-4", TokenUsage{TotalTokens: 1000, PromptTokens: 500, CompletionTokens: 500})

	// 重置
	c.Reset()

	if c.used != 0 {
		t.Errorf("expected used=0 after reset, got %f", c.used)
	}
	if c.usedTokens != 0 {
		t.Errorf("expected usedTokens=0 after reset, got %d", c.usedTokens)
	}
	if c.remaining != 100.0 {
		t.Errorf("expected remaining=100.0 after reset, got %f", c.remaining)
	}
}

func TestController_EstimateCost(t *testing.T) {
	c := NewController()

	// GPT-4: prompt $0.03/1K, completion $0.06/1K
	cost := c.EstimateCost("gpt-4", 1000, 1000)
	expected := 0.03 + 0.06 // 0.09
	if cost < expected-0.001 || cost > expected+0.001 {
		t.Errorf("expected cost=~%f, got %f", expected, cost)
	}

	// GPT-4o-mini: prompt $0.00015/1K, completion $0.0006/1K
	cost = c.EstimateCost("gpt-4o-mini", 10000, 5000)
	expected = 10*0.00015 + 5*0.0006 // 0.0015 + 0.003 = 0.0045
	if cost < expected-0.0001 || cost > expected+0.0001 {
		t.Errorf("expected cost=~%f, got %f", expected, cost)
	}
}

func TestController_RemainingBudget(t *testing.T) {
	c := NewController(WithBudget(100.0))

	// 记录一些使用量
	c.RecordUsage("gpt-4o-mini", TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 1000,
		TotalTokens:      2000,
	})

	remaining := c.RemainingBudget()
	if remaining >= 100.0 {
		t.Error("remaining should be less than initial budget")
	}
	if remaining <= 0 {
		t.Error("remaining should be positive")
	}
}

func TestController_RemainingTokens(t *testing.T) {
	c := NewController(WithMaxTokensTotal(10000))

	// 初始应该是全部
	if c.RemainingTokens() != 10000 {
		t.Errorf("expected remainingTokens=10000, got %d", c.RemainingTokens())
	}

	// 记录一些使用量
	c.usedTokens = 3000

	if c.RemainingTokens() != 7000 {
		t.Errorf("expected remainingTokens=7000, got %d", c.RemainingTokens())
	}
}

func TestController_CanAfford(t *testing.T) {
	c := NewController(WithBudget(1.0))

	// 可以负担
	if !c.CanAfford(0.5) {
		t.Error("should be able to afford 0.5")
	}

	// 不能负担
	if c.CanAfford(2.0) {
		t.Error("should not be able to afford 2.0")
	}

	// 无预算限制
	c2 := NewController() // budget = 0
	if !c2.CanAfford(1000.0) {
		t.Error("should be able to afford anything with no budget limit")
	}
}

func TestContextWithController(t *testing.T) {
	c := NewController()
	ctx := context.Background()

	// 添加控制器到 context
	ctx = ContextWithController(ctx, c)

	// 从 context 获取控制器
	got := ControllerFromContext(ctx)
	if got != c {
		t.Error("controller not retrieved correctly from context")
	}

	// 没有控制器的 context
	got = ControllerFromContext(context.Background())
	if got != nil {
		t.Error("expected nil for context without controller")
	}
}

func TestCheckAndRecord(t *testing.T) {
	c := NewController(
		WithBudget(10.0),
		WithRequestsPerMinute(100),
	)
	ctx := ContextWithController(context.Background(), c)

	usage := TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	err := CheckAndRecord(ctx, "gpt-4o-mini", usage)
	if err != nil {
		t.Fatalf("CheckAndRecord failed: %v", err)
	}

	// 检查是否记录
	if c.usedTokens != 150 {
		t.Errorf("expected usedTokens=150, got %d", c.usedTokens)
	}
}

func TestCheckAndRecord_NoController(t *testing.T) {
	// 没有控制器的 context 应该跳过检查
	ctx := context.Background()
	usage := TokenUsage{TotalTokens: 1000}

	err := CheckAndRecord(ctx, "gpt-4", usage)
	if err != nil {
		t.Errorf("CheckAndRecord should skip when no controller: %v", err)
	}
}

func TestCallbacks(t *testing.T) {
	var tokenCallbackCalled bool
	var rateCallbackCalled bool

	c := NewController(
		WithMaxTokensTotal(100),
		WithRequestsPerMinute(1),
		OnTokensExceeded(func(used, limit int64) {
			tokenCallbackCalled = true
		}),
		OnRateExceeded(func(requests, limit int) {
			rateCallbackCalled = true
		}),
	)
	ctx := context.Background()

	// 触发速率限制回调
	c.CheckRequest(ctx, 10) // 第一次
	c.CheckRequest(ctx, 10) // 第二次应该触发限流

	if !rateCallbackCalled {
		t.Error("onRateExceeded callback should be called")
	}

	// 重置以测试 Token 回调
	c.rateLimiter.Reset()
	c.usedTokens = 90

	c.CheckRequest(ctx, 20) // 超过总限制
	if !tokenCallbackCalled {
		t.Error("onTokensExceeded callback should be called")
	}
}

func TestDefaultPricing(t *testing.T) {
	// 验证默认定价表包含预期的模型
	expectedModels := []string{
		"gpt-4", "gpt-4-turbo", "gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo",
		"claude-3-opus", "claude-3-sonnet", "claude-3-haiku",
		"deepseek-chat", "deepseek-reasoner",
		"default",
	}

	for _, model := range expectedModels {
		if _, ok := DefaultPricing[model]; !ok {
			t.Errorf("expected model %s in DefaultPricing", model)
		}
	}
}

func TestModelPricing(t *testing.T) {
	// 测试定价结构
	pricing := ModelPricing{
		PromptPrice:     0.01,
		CompletionPrice: 0.02,
	}

	if pricing.PromptPrice != 0.01 {
		t.Errorf("expected PromptPrice=0.01, got %f", pricing.PromptPrice)
	}
	if pricing.CompletionPrice != 0.02 {
		t.Errorf("expected CompletionPrice=0.02, got %f", pricing.CompletionPrice)
	}
}

func TestTokenUsage(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens=100, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("expected CompletionTokens=50, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("expected TotalTokens=150, got %d", usage.TotalTokens)
	}
}

func TestControllerStats(t *testing.T) {
	stats := ControllerStats{
		Budget:          100.0,
		Used:            25.0,
		Remaining:       75.0,
		UsedTokens:      5000,
		MaxTokensTotal:  100000,
		RequestsLastMin: 10,
		RequestsPerMin:  60,
	}

	if stats.Budget != 100.0 {
		t.Errorf("expected Budget=100.0, got %f", stats.Budget)
	}
	if stats.Remaining != 75.0 {
		t.Errorf("expected Remaining=75.0, got %f", stats.Remaining)
	}
}

func TestController_ConcurrentAccess(t *testing.T) {
	c := NewController(
		WithBudget(1000.0),
		WithMaxTokensTotal(1000000),
		WithRequestsPerMinute(1000),
	)
	ctx := context.Background()

	// 并发访问测试
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.CheckRequest(ctx, 100)
				c.RecordUsage("gpt-4o-mini", TokenUsage{
					PromptTokens:     10,
					CompletionTokens: 10,
					TotalTokens:      20,
				})
				c.Stats()
				c.RemainingBudget()
				c.RemainingTokens()
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent test timeout")
		}
	}
}
