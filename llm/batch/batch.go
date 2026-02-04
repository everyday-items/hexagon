// Package batch 提供 LLM 请求批处理功能
//
// 本包实现了 LLM 请求的批量处理和优化：
//   - 请求合并：将多个相似请求合并处理
//   - 请求队列：管理并发请求队列
//   - 速率限制：控制 API 调用频率
//   - 自动重试：处理临时错误
//
// 设计借鉴：
//   - OpenAI Batch API: 批量处理
//   - LangChain: 请求批处理
//   - gRPC: 请求流和批处理
//
// 使用示例：
//
//	batcher := batch.NewBatcher(provider, batch.DefaultConfig())
//	results := batcher.BatchComplete(ctx, requests)
package batch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ============== 错误定义 ==============

var (
	// ErrBatchFailed 批处理失败
	ErrBatchFailed = errors.New("batch processing failed")

	// ErrQueueFull 队列已满
	ErrQueueFull = errors.New("request queue is full")

	// ErrRateLimited 被限流
	ErrRateLimited = errors.New("rate limited")

	// ErrTimeout 超时
	ErrTimeout = errors.New("timeout")

	// ErrProviderNotSet 未设置 Provider
	ErrProviderNotSet = errors.New("provider not set")
)

// ============== 请求和响应 ==============

// Request 批处理请求
type Request struct {
	// ID 请求 ID
	ID string

	// Messages 消息列表
	Messages []Message

	// Model 模型名称（可选，使用默认）
	Model string

	// MaxTokens 最大 token 数
	MaxTokens int

	// Temperature 温度
	Temperature float64

	// Metadata 元数据
	Metadata map[string]any
}

// Message 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response 批处理响应
type Response struct {
	// ID 请求 ID
	ID string

	// Content 响应内容
	Content string

	// TokensUsed 使用的 token 数
	TokensUsed int

	// FinishReason 完成原因
	FinishReason string

	// Error 错误（如果有）
	Error error

	// Latency 延迟（毫秒）
	Latency int64

	// Metadata 元数据
	Metadata map[string]any
}

// ============== Provider 接口 ==============

// Provider LLM 提供者接口（简化版）
type Provider interface {
	// Complete 执行补全
	Complete(ctx context.Context, req *Request) (*Response, error)
}

// ============== 批处理配置 ==============

// Config 批处理配置
type Config struct {
	// MaxBatchSize 最大批量大小
	MaxBatchSize int

	// MaxConcurrent 最大并发数
	MaxConcurrent int

	// QueueSize 队列大小
	QueueSize int

	// FlushInterval 刷新间隔
	FlushInterval time.Duration

	// Timeout 请求超时
	Timeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryDelay 重试延迟
	RetryDelay time.Duration

	// RateLimit 速率限制（每秒请求数）
	RateLimit int

	// OnBatchStart 批次开始回调
	OnBatchStart func(batchID string, count int)

	// OnBatchComplete 批次完成回调
	OnBatchComplete func(batchID string, count int, duration time.Duration)

	// OnRequestComplete 请求完成回调
	OnRequestComplete func(req *Request, resp *Response)

	// OnError 错误回调
	OnError func(req *Request, err error)
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxBatchSize:  10,
		MaxConcurrent: 5,
		QueueSize:     1000,
		FlushInterval: 100 * time.Millisecond,
		Timeout:       60 * time.Second,
		MaxRetries:    3,
		RetryDelay:    time.Second,
		RateLimit:     100, // 100 请求/秒
	}
}

// ============== 批处理器 ==============

// Batcher 批处理器
type Batcher struct {
	provider Provider
	config   *Config

	// 请求队列
	queue     chan *pendingRequest
	batchChan chan []*pendingRequest

	// 状态
	running  int32
	wg       sync.WaitGroup
	stopChan chan struct{}

	// 统计
	stats *Stats

	// 速率限制
	limiter *rateLimiter
}

// pendingRequest 待处理请求
type pendingRequest struct {
	request  *Request
	response chan *Response
}

// Stats 统计信息
type Stats struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessRequests int64         `json:"success_requests"`
	FailedRequests  int64         `json:"failed_requests"`
	TotalBatches    int64         `json:"total_batches"`
	TotalTokens     int64         `json:"total_tokens"`
	TotalLatency    time.Duration `json:"total_latency"`
	mu              sync.RWMutex
}

// NewBatcher 创建批处理器
func NewBatcher(provider Provider, config ...*Config) *Batcher {
	cfg := DefaultConfig()
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	}

	return &Batcher{
		provider:  provider,
		config:    cfg,
		queue:     make(chan *pendingRequest, cfg.QueueSize),
		batchChan: make(chan []*pendingRequest, cfg.MaxConcurrent),
		stopChan:  make(chan struct{}),
		stats:     &Stats{},
		limiter:   newRateLimiter(cfg.RateLimit),
	}
}

// Start 启动批处理器
func (b *Batcher) Start() {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return
	}

	// 启动收集器
	b.wg.Add(1)
	go b.collector()

	// 启动工作者
	for i := 0; i < b.config.MaxConcurrent; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}
}

// Stop 停止批处理器
func (b *Batcher) Stop() {
	if !atomic.CompareAndSwapInt32(&b.running, 1, 0) {
		return
	}
	close(b.stopChan)
	b.wg.Wait()
}

// Submit 提交请求
func (b *Batcher) Submit(ctx context.Context, req *Request) (*Response, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 创建待处理请求
	pending := &pendingRequest{
		request:  req,
		response: make(chan *Response, 1),
	}

	// 提交到队列
	select {
	case b.queue <- pending:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrQueueFull
	}

	// 等待响应
	select {
	case resp := <-pending.response:
		return resp, resp.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BatchSubmit 批量提交请求
func (b *Batcher) BatchSubmit(ctx context.Context, requests []*Request) []*Response {
	results := make([]*Response, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		i, req := i, req
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := b.Submit(ctx, req)
			if err != nil && resp == nil {
				resp = &Response{
					ID:    req.ID,
					Error: err,
				}
			}
			results[i] = resp
		}()
	}

	wg.Wait()
	return results
}

// collector 收集请求并组成批次
func (b *Batcher) collector() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*pendingRequest, 0, b.config.MaxBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// 发送批次
		batchCopy := make([]*pendingRequest, len(batch))
		copy(batchCopy, batch)

		select {
		case b.batchChan <- batchCopy:
		case <-b.stopChan:
			return
		}

		batch = batch[:0]
	}

	for {
		select {
		case <-b.stopChan:
			flush()
			close(b.batchChan)
			return

		case req := <-b.queue:
			batch = append(batch, req)
			if len(batch) >= b.config.MaxBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// worker 工作者处理批次
func (b *Batcher) worker(id int) {
	defer b.wg.Done()

	for batch := range b.batchChan {
		b.processBatch(batch)
	}
}

// processBatch 处理批次
func (b *Batcher) processBatch(batch []*pendingRequest) {
	startTime := time.Now()
	batchID := generateBatchID()

	atomic.AddInt64(&b.stats.TotalBatches, 1)

	if b.config.OnBatchStart != nil {
		b.config.OnBatchStart(batchID, len(batch))
	}

	// 并发处理每个请求
	var wg sync.WaitGroup
	for _, pending := range batch {
		pending := pending
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.processRequest(pending)
		}()
	}
	wg.Wait()

	if b.config.OnBatchComplete != nil {
		b.config.OnBatchComplete(batchID, len(batch), time.Since(startTime))
	}
}

// processRequest 处理单个请求
func (b *Batcher) processRequest(pending *pendingRequest) {
	atomic.AddInt64(&b.stats.TotalRequests, 1)
	startTime := time.Now()

	// 速率限制
	b.limiter.wait()

	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), b.config.Timeout)
	defer cancel()

	var resp *Response
	var err error

	// 重试逻辑
	for attempt := 0; attempt <= b.config.MaxRetries; attempt++ {
		resp, err = b.provider.Complete(ctx, pending.request)
		if err == nil {
			break
		}

		// 检查是否应该重试
		if !isRetryableError(err) {
			break
		}

		// 等待重试
		if attempt < b.config.MaxRetries {
			time.Sleep(b.config.RetryDelay * time.Duration(attempt+1))
		}
	}

	latency := time.Since(startTime)

	if resp == nil {
		resp = &Response{
			ID:    pending.request.ID,
			Error: err,
		}
	}
	resp.Latency = latency.Milliseconds()

	// 更新统计
	if err != nil {
		atomic.AddInt64(&b.stats.FailedRequests, 1)
		if b.config.OnError != nil {
			b.config.OnError(pending.request, err)
		}
	} else {
		atomic.AddInt64(&b.stats.SuccessRequests, 1)
		atomic.AddInt64(&b.stats.TotalTokens, int64(resp.TokensUsed))
	}

	b.stats.mu.Lock()
	b.stats.TotalLatency += latency
	b.stats.mu.Unlock()

	if b.config.OnRequestComplete != nil {
		b.config.OnRequestComplete(pending.request, resp)
	}

	// 发送响应
	pending.response <- resp
}

// Stats 获取统计信息
func (b *Batcher) GetStats() Stats {
	b.stats.mu.RLock()
	defer b.stats.mu.RUnlock()

	return Stats{
		TotalRequests:   atomic.LoadInt64(&b.stats.TotalRequests),
		SuccessRequests: atomic.LoadInt64(&b.stats.SuccessRequests),
		FailedRequests:  atomic.LoadInt64(&b.stats.FailedRequests),
		TotalBatches:    atomic.LoadInt64(&b.stats.TotalBatches),
		TotalTokens:     atomic.LoadInt64(&b.stats.TotalTokens),
		TotalLatency:    b.stats.TotalLatency,
	}
}

// ============== 速率限制器 ==============

type rateLimiter struct {
	rate     int
	tokens   int
	lastTime time.Time
	mu       sync.Mutex
}

func newRateLimiter(rate int) *rateLimiter {
	return &rateLimiter{
		rate:     rate,
		tokens:   rate,
		lastTime: time.Now(),
	}
}

func (l *rateLimiter) wait() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 补充令牌
	now := time.Now()
	elapsed := now.Sub(l.lastTime)
	l.tokens += int(elapsed.Seconds() * float64(l.rate))
	if l.tokens > l.rate {
		l.tokens = l.rate
	}
	l.lastTime = now

	// 等待令牌
	if l.tokens <= 0 {
		waitTime := time.Second / time.Duration(l.rate)
		time.Sleep(waitTime)
		l.tokens = 1
	}

	l.tokens--
}

// ============== 辅助函数 ==============

var batchCounter int64

func generateBatchID() string {
	id := atomic.AddInt64(&batchCounter, 1)
	return string(rune(id))
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// 可重试的错误类型
	errStr := err.Error()
	return errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrTimeout) ||
		containsAny(errStr, "timeout", "rate limit", "429", "503", "temporarily")
}

func containsAny(s string, substrs ...string) bool {
	sLower := string([]byte(s))
	for _, sub := range substrs {
		if len(sub) > 0 && len(sLower) >= len(sub) {
			for i := 0; i <= len(sLower)-len(sub); i++ {
				match := true
				for j := 0; j < len(sub); j++ {
					c := sLower[i+j]
					if c >= 'A' && c <= 'Z' {
						c += 32
					}
					sc := sub[j]
					if sc >= 'A' && sc <= 'Z' {
						sc += 32
					}
					if c != sc {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}
	return false
}

// ============== 批量执行辅助函数 ==============

// BatchComplete 批量补全（简化接口）
func BatchComplete(ctx context.Context, provider Provider, requests []*Request, config ...*Config) []*Response {
	batcher := NewBatcher(provider, config...)
	batcher.Start()
	defer batcher.Stop()

	return batcher.BatchSubmit(ctx, requests)
}

// ParallelComplete 并行补全（不使用批处理器）
func ParallelComplete(ctx context.Context, provider Provider, requests []*Request, maxConcurrent int) []*Response {
	results := make([]*Response, len(requests))

	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, req := range requests {
		i, req := i, req
		wg.Add(1)
		go func() {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			startTime := time.Now()
			resp, err := provider.Complete(ctx, req)
			if resp == nil {
				resp = &Response{
					ID:    req.ID,
					Error: err,
				}
			}
			resp.Latency = time.Since(startTime).Milliseconds()
			results[i] = resp
		}()
	}

	wg.Wait()
	return results
}

// ============== 请求构建器 ==============

// RequestBuilder 请求构建器
type RequestBuilder struct {
	request *Request
}

// NewRequestBuilder 创建请求构建器
func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		request: &Request{
			Metadata: make(map[string]any),
		},
	}
}

// WithID 设置 ID
func (rb *RequestBuilder) WithID(id string) *RequestBuilder {
	rb.request.ID = id
	return rb
}

// WithModel 设置模型
func (rb *RequestBuilder) WithModel(model string) *RequestBuilder {
	rb.request.Model = model
	return rb
}

// WithMaxTokens 设置最大 token 数
func (rb *RequestBuilder) WithMaxTokens(max int) *RequestBuilder {
	rb.request.MaxTokens = max
	return rb
}

// WithTemperature 设置温度
func (rb *RequestBuilder) WithTemperature(temp float64) *RequestBuilder {
	rb.request.Temperature = temp
	return rb
}

// AddSystemMessage 添加系统消息
func (rb *RequestBuilder) AddSystemMessage(content string) *RequestBuilder {
	rb.request.Messages = append(rb.request.Messages, Message{
		Role:    "system",
		Content: content,
	})
	return rb
}

// AddUserMessage 添加用户消息
func (rb *RequestBuilder) AddUserMessage(content string) *RequestBuilder {
	rb.request.Messages = append(rb.request.Messages, Message{
		Role:    "user",
		Content: content,
	})
	return rb
}

// AddAssistantMessage 添加助手消息
func (rb *RequestBuilder) AddAssistantMessage(content string) *RequestBuilder {
	rb.request.Messages = append(rb.request.Messages, Message{
		Role:    "assistant",
		Content: content,
	})
	return rb
}

// WithMetadata 设置元数据
func (rb *RequestBuilder) WithMetadata(key string, value any) *RequestBuilder {
	rb.request.Metadata[key] = value
	return rb
}

// Build 构建请求
func (rb *RequestBuilder) Build() *Request {
	return rb.request
}
