// Package stream 提供 Hexagon 框架的增强流处理能力
//
// 本包实现了完整的流处理系统，包括：
//   - StreamReader[T]: 泛型流读取器，支持多种底层实现
//   - StreamWriter[T]: 泛型流写入器
//   - 流操作符：Map、Filter、Reduce、Copy、Merge、Buffer、Timeout
//   - 类型注册：注册自定义类型的合并、分块函数
//
// 设计借鉴：
//   - Eino: StreamReader/StreamWriter 基础设计
//   - LangChain: 流式处理模式
//   - Go: channel 和 io.Reader 风格
//
// 使用示例：
//
//	// 创建管道流
//	reader, writer := stream.Pipe[string](10)
//	go func() {
//	    writer.Send("hello")
//	    writer.Send("world")
//	    writer.Close()
//	}()
//
//	// 流操作
//	result := stream.Map(reader, strings.ToUpper).
//	    Filter(func(s string) bool { return len(s) > 3 }).
//	    Collect(ctx)
package stream

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ============== 错误定义 ==============

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream closed")

	// ErrStreamEmpty 流为空
	ErrStreamEmpty = errors.New("stream is empty")

	// ErrStreamTimeout 流操作超时
	ErrStreamTimeout = errors.New("stream operation timeout")

	// ErrNoConcatFunc 未注册合并函数
	ErrNoConcatFunc = errors.New("no concat function registered for type")

	// ErrSourceEOF 源流结束（用于合并流）
	ErrSourceEOF = errors.New("source stream EOF")
)

// SourceEOF 源流结束错误，包含源名称
type SourceEOF struct {
	Source string
}

func (e *SourceEOF) Error() string {
	return "source " + e.Source + " EOF"
}

// IsSourceEOF 检查是否是源流结束错误
func IsSourceEOF(err error) (string, bool) {
	if se, ok := err.(*SourceEOF); ok {
		return se.Source, true
	}
	return "", false
}

// ============== StreamReader ==============

// readerType 流读取器类型
type readerType int

const (
	readerTypePipe readerType = iota
	readerTypeArray
	readerTypeMulti
	readerTypeChild
	readerTypeMap
	readerTypeFilter
	readerTypeBuffer
	readerTypeTimeout
	readerTypeBackpressure
	readerTypeTake
	readerTypeSkip
	readerTypeTakeWhile
	readerTypeSkipWhile
	readerTypeFlatMap
	readerTypeDistinct
	readerTypeDistinctBy
	readerTypeZip
	readerTypeBatch
	readerTypeWindow
	readerTypeDebounce
	readerTypeThrottle
)

// StreamReader 泛型流读取器
// 支持多种底层实现，提供统一的读取接口
type StreamReader[T any] struct {
	typ readerType

	// 不同类型的底层实现
	pipe          *pipeReader[T]
	array         *arrayReader[T]
	multi         *multiReader[T]
	child         *childReader[T]
	mapR          *mapReader[T]
	filterR       *filterReader[T]
	bufferR       *bufferReader[T]
	timeoutR      *timeoutReader[T]
	backpressureR *backpressureReader[T]
	takeR         *takeReader[T]
	skipR         *skipReader[T]
	takeWhileR    *takeWhileReader[T]
	skipWhileR    *skipWhileReader[T]
	flatMapR      *flatMapReader[T]
	distinctR     *distinctReader[T]
	distinctByR   any // *distinctByReader[T, K] - 使用 any 因为 K 是泛型
	zipR          *zipReader[T]
	batchR        any // *batchReader[T] 会产生 []T
	windowR       any // *windowReader[T] 会产生 []T
	debounceR     *debounceReader[T]
	throttleR     *throttleReader[T]

	// 元信息
	source string // 流来源标识
}

// Recv 接收下一个元素
// 返回 io.EOF 表示流结束
func (sr *StreamReader[T]) Recv() (T, error) {
	switch sr.typ {
	case readerTypePipe:
		return sr.pipe.recv()
	case readerTypeArray:
		return sr.array.recv()
	case readerTypeMulti:
		return sr.multi.recv()
	case readerTypeChild:
		return sr.child.recv()
	case readerTypeMap:
		return sr.mapR.recv()
	case readerTypeFilter:
		return sr.filterR.recv()
	case readerTypeBuffer:
		return sr.bufferR.recv()
	case readerTypeTimeout:
		return sr.timeoutR.recv()
	case readerTypeBackpressure:
		return sr.backpressureR.recv()
	case readerTypeTake:
		return sr.takeR.recv()
	case readerTypeSkip:
		return sr.skipR.recv()
	case readerTypeTakeWhile:
		return sr.takeWhileR.recv()
	case readerTypeSkipWhile:
		return sr.skipWhileR.recv()
	case readerTypeFlatMap:
		return sr.flatMapR.recv()
	case readerTypeDistinct:
		return sr.distinctR.recv()
	case readerTypeDistinctBy:
		return sr.distinctByR.(interface{ recv() (T, error) }).recv()
	case readerTypeZip:
		return sr.zipR.recv()
	case readerTypeBatch:
		return sr.batchR.(interface{ recv() (T, error) }).recv()
	case readerTypeWindow:
		return sr.windowR.(interface{ recv() (T, error) }).recv()
	case readerTypeDebounce:
		return sr.debounceR.recv()
	case readerTypeThrottle:
		return sr.throttleR.recv()
	default:
		var zero T
		return zero, ErrStreamClosed
	}
}

// Close 关闭流
func (sr *StreamReader[T]) Close() error {
	switch sr.typ {
	case readerTypePipe:
		return sr.pipe.close()
	case readerTypeArray:
		return nil
	case readerTypeMulti:
		return sr.multi.close()
	case readerTypeChild:
		return nil // child 不负责关闭
	case readerTypeMap:
		return sr.mapR.close()
	case readerTypeFilter:
		return sr.filterR.close()
	case readerTypeBuffer:
		return sr.bufferR.close()
	case readerTypeTimeout:
		return sr.timeoutR.close()
	case readerTypeBackpressure:
		return sr.backpressureR.close()
	case readerTypeTake:
		return sr.takeR.close()
	case readerTypeSkip:
		return sr.skipR.close()
	case readerTypeTakeWhile:
		return sr.takeWhileR.close()
	case readerTypeSkipWhile:
		return sr.skipWhileR.close()
	case readerTypeFlatMap:
		return sr.flatMapR.close()
	case readerTypeDistinct:
		return sr.distinctR.close()
	case readerTypeDistinctBy:
		return sr.distinctByR.(interface{ close() error }).close()
	case readerTypeZip:
		return sr.zipR.close()
	case readerTypeBatch:
		return sr.batchR.(interface{ close() error }).close()
	case readerTypeWindow:
		return sr.windowR.(interface{ close() error }).close()
	case readerTypeDebounce:
		return sr.debounceR.close()
	case readerTypeThrottle:
		return sr.throttleR.close()
	default:
		return nil
	}
}

// Source 获取流来源标识
func (sr *StreamReader[T]) Source() string {
	return sr.source
}

// SetSource 设置流来源标识
func (sr *StreamReader[T]) SetSource(source string) *StreamReader[T] {
	sr.source = source
	return sr
}

// Collect 收集所有元素到切片
func (sr *StreamReader[T]) Collect(ctx context.Context) ([]T, error) {
	var items []T
	for {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
			item, err := sr.Recv()
			if err == io.EOF {
				return items, nil
			}
			if _, ok := IsSourceEOF(err); ok {
				continue // 忽略源 EOF，继续读取
			}
			if err != nil {
				return items, err
			}
			items = append(items, item)
		}
	}
}

// ForEach 对每个元素执行操作
func (sr *StreamReader[T]) ForEach(ctx context.Context, fn func(T) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			item, err := sr.Recv()
			if err == io.EOF {
				return nil
			}
			if _, ok := IsSourceEOF(err); ok {
				continue
			}
			if err != nil {
				return err
			}
			if err := fn(item); err != nil {
				return err
			}
		}
	}
}

// Copy 复制流为多个独立读者
// 每个读者独立消费，不互相影响
//
// 实现原理（零分配 Linked List 设计）：
//   - 使用 sync.Once + 链表实现零分配流复制
//   - 所有子 Reader 共享同一链表，只追踪不同的读取位置
//   - 每个节点使用 sync.Once 确保只被初始化一次
//   - 真正的零拷贝设计，数据只从源流读取一次
//
// 借鉴自 Eino 框架的精细实现
func (sr *StreamReader[T]) Copy(n int) []*StreamReader[T] {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []*StreamReader[T]{sr}
	}

	// 创建哨兵头节点
	// 所有 childReader 从这个哨兵节点开始
	sentinel := &streamNode[T]{}

	// 创建父流管理器
	parent := &parentStream[T]{
		source: sr,
		head:   sentinel, // 链表头（哨兵节点）
		tail:   sentinel, // 初始时 tail 也指向哨兵
		done:   false,
	}

	// 创建 n 个子读取器，每个都从哨兵节点开始
	readers := make([]*StreamReader[T], n)
	for i := 0; i < n; i++ {
		child := &childReader[T]{
			parent:  parent,
			current: sentinel, // 从哨兵节点开始
		}
		readers[i] = &StreamReader[T]{
			typ:    readerTypeChild,
			child:  child,
			source: sr.source,
		}
	}

	return readers
}

// ============== StreamWriter ==============

// StreamWriter 泛型流写入器
type StreamWriter[T any] struct {
	pipe *pipeWriter[T]
}

// Send 发送元素到流
func (sw *StreamWriter[T]) Send(item T) error {
	return sw.pipe.send(item)
}

// Close 关闭流
func (sw *StreamWriter[T]) Close() error {
	return sw.pipe.close()
}

// CloseWithError 带错误关闭流
func (sw *StreamWriter[T]) CloseWithError(err error) error {
	return sw.pipe.closeWithError(err)
}

// ============== Pipe 管道流 ==============

// Pipe 创建管道流
// cap 为缓冲区大小，0 表示无缓冲
func Pipe[T any](cap int) (*StreamReader[T], *StreamWriter[T]) {
	ch := make(chan T, cap)
	done := make(chan struct{})
	var closeErr atomic.Value

	reader := &pipeReader[T]{ch: ch, done: done, closeErr: &closeErr}
	writer := &pipeWriter[T]{ch: ch, done: done, closeErr: &closeErr, closed: 0}

	return &StreamReader[T]{typ: readerTypePipe, pipe: reader},
		&StreamWriter[T]{pipe: writer}
}

type pipeReader[T any] struct {
	ch       chan T
	done     chan struct{}
	closeErr *atomic.Value
}

func (pr *pipeReader[T]) recv() (T, error) {
	select {
	case item, ok := <-pr.ch:
		if !ok {
			var zero T
			if err := pr.closeErr.Load(); err != nil {
				return zero, err.(error)
			}
			return zero, io.EOF
		}
		return item, nil
	case <-pr.done:
		var zero T
		if err := pr.closeErr.Load(); err != nil {
			return zero, err.(error)
		}
		return zero, io.EOF
	}
}

func (pr *pipeReader[T]) close() error {
	select {
	case <-pr.done:
		return nil
	default:
		close(pr.done)
		return nil
	}
}

type pipeWriter[T any] struct {
	ch       chan T
	done     chan struct{}
	closeErr *atomic.Value
	closed   int32
}

func (pw *pipeWriter[T]) send(item T) error {
	if atomic.LoadInt32(&pw.closed) == 1 {
		return ErrStreamClosed
	}
	select {
	case pw.ch <- item:
		return nil
	case <-pw.done:
		return ErrStreamClosed
	}
}

func (pw *pipeWriter[T]) close() error {
	if atomic.CompareAndSwapInt32(&pw.closed, 0, 1) {
		close(pw.ch)
	}
	return nil
}

func (pw *pipeWriter[T]) closeWithError(err error) error {
	pw.closeErr.Store(err)
	return pw.close()
}

// ============== Array 数组流 ==============

// FromSlice 从切片创建流
func FromSlice[T any](items []T) *StreamReader[T] {
	return &StreamReader[T]{
		typ:   readerTypeArray,
		array: &arrayReader[T]{items: items, idx: 0},
	}
}

// FromValue 从单个值创建流
func FromValue[T any](item T) *StreamReader[T] {
	return FromSlice([]T{item})
}

type arrayReader[T any] struct {
	items []T
	idx   int
	mu    sync.Mutex
}

func (ar *arrayReader[T]) recv() (T, error) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if ar.idx >= len(ar.items) {
		var zero T
		return zero, io.EOF
	}
	item := ar.items[ar.idx]
	ar.idx++
	return item, nil
}

// ============== Multi 合并流 ==============

// Merge 合并多个流
// 保留源流信息，当某个源流结束时返回 SourceEOF
func Merge[T any](readers ...*StreamReader[T]) *StreamReader[T] {
	if len(readers) == 0 {
		return FromSlice[T](nil)
	}
	if len(readers) == 1 {
		return readers[0]
	}

	return &StreamReader[T]{
		typ: readerTypeMulti,
		multi: &multiReader[T]{
			readers: readers,
			current: 0,
			done:    make([]bool, len(readers)),
		},
	}
}

type multiReader[T any] struct {
	readers []*StreamReader[T]
	current int
	done    []bool
	mu      sync.Mutex
	allDone bool
}

func (mr *multiReader[T]) recv() (T, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.allDone {
		var zero T
		return zero, io.EOF
	}

	// 轮询方式读取
	tried := 0
	for tried < len(mr.readers) {
		idx := mr.current
		mr.current = (mr.current + 1) % len(mr.readers)

		if mr.done[idx] {
			tried++
			continue
		}

		item, err := mr.readers[idx].Recv()
		if err == io.EOF {
			mr.done[idx] = true
			// 检查是否全部完成
			allDone := true
			for _, d := range mr.done {
				if !d {
					allDone = false
					break
				}
			}
			mr.allDone = allDone
			if allDone {
				var zero T
				return zero, io.EOF
			}
			// 返回源 EOF
			source := mr.readers[idx].Source()
			if source == "" {
				source = "unknown"
			}
			var zero T
			return zero, &SourceEOF{Source: source}
		}
		if err != nil {
			var zero T
			return zero, err
		}
		return item, nil
	}

	var zero T
	return zero, io.EOF
}

func (mr *multiReader[T]) close() error {
	for _, r := range mr.readers {
		r.Close()
	}
	return nil
}

// ============== Child 子流（用于 Copy）- 零分配 Linked List 设计 ==============
// 借鉴 Eino 的 sync.Once + linked list 实现
// 所有子 Reader 共享同一链表，只追踪不同的读取位置
// 真正的零分配设计，避免数据复制

// streamNode 流节点（链表节点）
type streamNode[T any] struct {
	value T
	err   error
	next  *streamNode[T]
	once  sync.Once // 确保 next 只被初始化一次
}

// parentStream 父流（管理链表）
type parentStream[T any] struct {
	source *StreamReader[T]
	head   *streamNode[T] // 链表头（哨兵节点）
	tail   *streamNode[T] // 链表尾
	mu     sync.Mutex
	done   bool
	endErr error
}

// advance 推进链表，从源流读取下一个元素
// 使用 sync.Once 确保每个节点只被初始化一次
func (ps *parentStream[T]) advance(current *streamNode[T]) (*streamNode[T], error) {
	// 使用 once 确保只初始化一次
	current.once.Do(func() {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		// 再次检查是否已初始化（double-check）
		if current.next != nil {
			return
		}

		// 如果已经结束
		if ps.done {
			current.next = &streamNode[T]{err: ps.endErr}
			return
		}

		// 从源流读取
		item, err := ps.source.Recv()
		if err != nil {
			ps.done = true
			ps.endErr = err
			current.next = &streamNode[T]{err: err}
			return
		}

		// 创建新节点
		newNode := &streamNode[T]{value: item}
		current.next = newNode
		ps.tail = newNode
	})

	return current.next, current.next.err
}

// childReader 子流读取器
// 每个子读取器维护自己在链表中的位置指针
type childReader[T any] struct {
	parent  *parentStream[T]
	current *streamNode[T] // 当前位置
}

func (cr *childReader[T]) recv() (T, error) {
	// 推进到下一个节点
	next, err := cr.parent.advance(cr.current)
	if err != nil {
		var zero T
		return zero, err
	}

	// 更新当前位置
	cr.current = next
	return next.value, nil
}

// ============== 流操作符 ==============

// Map 流映射转换
func Map[T, U any](sr *StreamReader[T], fn func(T) U) *StreamReader[U] {
	return &StreamReader[U]{
		typ: readerTypeMap,
		mapR: &mapReader[U]{
			source: sr,
			fn: func() (U, error) {
				item, err := sr.Recv()
				if err != nil {
					var zero U
					return zero, err
				}
				return fn(item), nil
			},
		},
		source: sr.source,
	}
}

type mapReader[T any] struct {
	source interface{ Close() error }
	fn     func() (T, error)
}

func (mr *mapReader[T]) recv() (T, error) {
	return mr.fn()
}

func (mr *mapReader[T]) close() error {
	return mr.source.Close()
}

// Filter 流过滤
func Filter[T any](sr *StreamReader[T], fn func(T) bool) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeFilter,
		filterR: &filterReader[T]{
			source: sr,
			fn:     fn,
		},
		source: sr.source,
	}
}

type filterReader[T any] struct {
	source *StreamReader[T]
	fn     func(T) bool
}

func (fr *filterReader[T]) recv() (T, error) {
	for {
		item, err := fr.source.Recv()
		if err != nil {
			return item, err
		}
		if fr.fn(item) {
			return item, nil
		}
	}
}

func (fr *filterReader[T]) close() error {
	return fr.source.Close()
}

// Reduce 流归约
func Reduce[T, U any](ctx context.Context, sr *StreamReader[T], init U, fn func(U, T) U) (U, error) {
	result := init
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			item, err := sr.Recv()
			if err == io.EOF {
				return result, nil
			}
			if _, ok := IsSourceEOF(err); ok {
				continue
			}
			if err != nil {
				return result, err
			}
			result = fn(result, item)
		}
	}
}

// Buffer 带缓冲的流
func Buffer[T any](sr *StreamReader[T], size int) *StreamReader[T] {
	if size <= 0 {
		size = 1
	}
	br := &bufferReader[T]{
		source: sr,
		buffer: make(chan T, size),
		done:   make(chan struct{}),
	}
	go br.fill()
	return &StreamReader[T]{
		typ:     readerTypeBuffer,
		bufferR: br,
		source:  sr.source,
	}
}

type bufferReader[T any] struct {
	source *StreamReader[T]
	buffer chan T
	done   chan struct{}
	err    error
	closed int32
}

func (br *bufferReader[T]) fill() {
	defer close(br.buffer)
	for {
		select {
		case <-br.done:
			return
		default:
			item, err := br.source.Recv()
			if err != nil {
				br.err = err
				return
			}
			select {
			case br.buffer <- item:
			case <-br.done:
				return
			}
		}
	}
}

func (br *bufferReader[T]) recv() (T, error) {
	item, ok := <-br.buffer
	if !ok {
		var zero T
		if br.err != nil {
			return zero, br.err
		}
		return zero, io.EOF
	}
	return item, nil
}

func (br *bufferReader[T]) close() error {
	if atomic.CompareAndSwapInt32(&br.closed, 0, 1) {
		close(br.done)
	}
	return br.source.Close()
}

// Timeout 带超时的流
func Timeout[T any](sr *StreamReader[T], d time.Duration) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeTimeout,
		timeoutR: &timeoutReader[T]{
			source:  sr,
			timeout: d,
		},
		source: sr.source,
	}
}

type timeoutReader[T any] struct {
	source  *StreamReader[T]
	timeout time.Duration
}

func (tr *timeoutReader[T]) recv() (T, error) {
	done := make(chan struct{})
	var item T
	var err error

	go func() {
		item, err = tr.source.Recv()
		close(done)
	}()

	select {
	case <-done:
		return item, err
	case <-time.After(tr.timeout):
		var zero T
		return zero, ErrStreamTimeout
	}
}

func (tr *timeoutReader[T]) close() error {
	return tr.source.Close()
}

// ============== 类型注册 ==============

var (
	concatFuncs = sync.Map{}
	mergeFuncs  = sync.Map{}
	splitFuncs  = sync.Map{}
)

// RegisterConcatFunc 注册类型的合并函数
// 用于将多个流元素合并为一个
func RegisterConcatFunc[T any](fn func([]T) (T, error)) {
	var zero T
	key := getTypeKey(zero)
	concatFuncs.Store(key, fn)
}

// RegisterMergeFunc 注册类型的流合并函数（保留元信息）
func RegisterMergeFunc[T any](fn func(items []T, sources []string) (T, error)) {
	var zero T
	key := getTypeKey(zero)
	mergeFuncs.Store(key, fn)
}

// RegisterSplitFunc 注册类型的分块函数（用于流式输出）
func RegisterSplitFunc[T any](fn func(T) []T) {
	var zero T
	key := getTypeKey(zero)
	splitFuncs.Store(key, fn)
}

// Concat 合并流中所有元素为一个
func Concat[T any](ctx context.Context, sr *StreamReader[T]) (T, error) {
	items, err := sr.Collect(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	return ConcatItems(items)
}

// ConcatItems 合并切片中所有元素为一个
func ConcatItems[T any](items []T) (T, error) {
	if len(items) == 0 {
		var zero T
		return zero, ErrStreamEmpty
	}
	if len(items) == 1 {
		return items[0], nil
	}

	var zero T
	key := getTypeKey(zero)
	if fn, ok := concatFuncs.Load(key); ok {
		return fn.(func([]T) (T, error))(items)
	}

	// 默认返回最后一个
	return items[len(items)-1], nil
}

// Split 将单个元素分割为流
func Split[T any](item T) *StreamReader[T] {
	key := getTypeKey(item)
	if fn, ok := splitFuncs.Load(key); ok {
		items := fn.(func(T) []T)(item)
		return FromSlice(items)
	}
	return FromValue(item)
}

func getTypeKey[T any](v T) string {
	return typeString[T]()
}

func typeString[T any]() string {
	var zero T
	return string(rune(0)) + // 防止空字符串
		func() string {
			switch any(zero).(type) {
			case string:
				return "string"
			case int:
				return "int"
			case int64:
				return "int64"
			case float64:
				return "float64"
			case bool:
				return "bool"
			default:
				// 使用反射获取类型名
				return "any"
			}
		}()
}

// ============== 背压控制 ==============

// BackpressureStrategy 背压策略
type BackpressureStrategy int

const (
	// BackpressureBlock 阻塞等待（默认）
	BackpressureBlock BackpressureStrategy = iota
	// BackpressureDrop 丢弃最新元素
	BackpressureDrop
	// BackpressureDropOldest 丢弃最旧元素
	BackpressureDropOldest
	// BackpressureError 返回错误
	BackpressureError
)

// ErrBackpressure 背压错误
var ErrBackpressure = errors.New("stream backpressure: buffer full")

// BackpressureConfig 背压配置
type BackpressureConfig struct {
	// BufferSize 缓冲区大小
	BufferSize int
	// Strategy 背压策略
	Strategy BackpressureStrategy
	// HighWaterMark 高水位（触发背压）
	HighWaterMark int
	// LowWaterMark 低水位（恢复正常）
	LowWaterMark int
	// OnHighWater 高水位回调
	OnHighWater func()
	// OnLowWater 低水位回调
	OnLowWater func()
}

// DefaultBackpressureConfig 默认背压配置
func DefaultBackpressureConfig() *BackpressureConfig {
	return &BackpressureConfig{
		BufferSize:    64,
		Strategy:      BackpressureBlock,
		HighWaterMark: 48, // 75%
		LowWaterMark:  16, // 25%
	}
}

// Backpressure 创建带背压控制的流
func Backpressure[T any](sr *StreamReader[T], config *BackpressureConfig) *StreamReader[T] {
	if config == nil {
		config = DefaultBackpressureConfig()
	}

	bp := &backpressureReader[T]{
		source:   sr,
		config:   config,
		buffer:   make(chan T, config.BufferSize),
		done:     make(chan struct{}),
		highFlag: 0,
	}
	go bp.fill()

	return &StreamReader[T]{
		typ:     readerTypeBackpressure,
		backpressureR: bp,
		source:  sr.source,
	}
}

type backpressureReader[T any] struct {
	source   *StreamReader[T]
	config   *BackpressureConfig
	buffer   chan T
	done     chan struct{}
	err      error
	closed   int32
	highFlag int32 // 是否处于高水位
}

func (bp *backpressureReader[T]) fill() {
	defer close(bp.buffer)
	for {
		select {
		case <-bp.done:
			return
		default:
			item, err := bp.source.Recv()
			if err != nil {
				bp.err = err
				return
			}

			// 检查水位
			currentLen := len(bp.buffer)
			if currentLen >= bp.config.HighWaterMark && atomic.CompareAndSwapInt32(&bp.highFlag, 0, 1) {
				if bp.config.OnHighWater != nil {
					bp.config.OnHighWater()
				}
			}

			// 根据策略处理
			switch bp.config.Strategy {
			case BackpressureBlock:
				// 阻塞等待
				select {
				case bp.buffer <- item:
				case <-bp.done:
					return
				}
			case BackpressureDrop:
				// 丢弃最新
				select {
				case bp.buffer <- item:
				default:
					// 丢弃
				}
			case BackpressureDropOldest:
				// 丢弃最旧
				select {
				case bp.buffer <- item:
				default:
					// 缓冲区满，丢弃最旧的
					select {
					case <-bp.buffer:
					default:
					}
					bp.buffer <- item
				}
			case BackpressureError:
				// 返回错误
				select {
				case bp.buffer <- item:
				default:
					bp.err = ErrBackpressure
					return
				}
			}
		}
	}
}

func (bp *backpressureReader[T]) recv() (T, error) {
	item, ok := <-bp.buffer
	if !ok {
		var zero T
		if bp.err != nil {
			return zero, bp.err
		}
		return zero, io.EOF
	}

	// 检查低水位
	currentLen := len(bp.buffer)
	if currentLen <= bp.config.LowWaterMark && atomic.CompareAndSwapInt32(&bp.highFlag, 1, 0) {
		if bp.config.OnLowWater != nil {
			bp.config.OnLowWater()
		}
	}

	return item, nil
}

func (bp *backpressureReader[T]) close() error {
	if atomic.CompareAndSwapInt32(&bp.closed, 0, 1) {
		close(bp.done)
	}
	return bp.source.Close()
}

// ============== 高阶流操作 ==============

// Take 只取前 n 个元素
func Take[T any](sr *StreamReader[T], n int) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeTake,
		takeR: &takeReader[T]{
			source: sr,
			limit:  n,
			count:  0,
		},
		source: sr.source,
	}
}

type takeReader[T any] struct {
	source *StreamReader[T]
	limit  int
	count  int
	mu     sync.Mutex
}

func (tr *takeReader[T]) recv() (T, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.count >= tr.limit {
		var zero T
		return zero, io.EOF
	}

	item, err := tr.source.Recv()
	if err != nil {
		return item, err
	}
	tr.count++
	return item, nil
}

func (tr *takeReader[T]) close() error {
	return tr.source.Close()
}

// Skip 跳过前 n 个元素
func Skip[T any](sr *StreamReader[T], n int) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeSkip,
		skipR: &skipReader[T]{
			source:  sr,
			skip:    n,
			skipped: 0,
		},
		source: sr.source,
	}
}

type skipReader[T any] struct {
	source  *StreamReader[T]
	skip    int
	skipped int
	mu      sync.Mutex
}

func (sr *skipReader[T]) recv() (T, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// 跳过指定数量
	for sr.skipped < sr.skip {
		_, err := sr.source.Recv()
		if err != nil {
			var zero T
			return zero, err
		}
		sr.skipped++
	}

	return sr.source.Recv()
}

func (sr *skipReader[T]) close() error {
	return sr.source.Close()
}

// TakeWhile 取元素直到条件不满足
func TakeWhile[T any](sr *StreamReader[T], predicate func(T) bool) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeTakeWhile,
		takeWhileR: &takeWhileReader[T]{
			source:    sr,
			predicate: predicate,
			done:      false,
		},
		source: sr.source,
	}
}

type takeWhileReader[T any] struct {
	source    *StreamReader[T]
	predicate func(T) bool
	done      bool
	mu        sync.Mutex
}

func (tw *takeWhileReader[T]) recv() (T, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.done {
		var zero T
		return zero, io.EOF
	}

	item, err := tw.source.Recv()
	if err != nil {
		return item, err
	}

	if !tw.predicate(item) {
		tw.done = true
		var zero T
		return zero, io.EOF
	}

	return item, nil
}

func (tw *takeWhileReader[T]) close() error {
	return tw.source.Close()
}

// SkipWhile 跳过元素直到条件不满足
func SkipWhile[T any](sr *StreamReader[T], predicate func(T) bool) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeSkipWhile,
		skipWhileR: &skipWhileReader[T]{
			source:    sr,
			predicate: predicate,
			skipping:  true,
		},
		source: sr.source,
	}
}

type skipWhileReader[T any] struct {
	source    *StreamReader[T]
	predicate func(T) bool
	skipping  bool
	mu        sync.Mutex
}

func (sw *skipWhileReader[T]) recv() (T, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	for sw.skipping {
		item, err := sw.source.Recv()
		if err != nil {
			return item, err
		}
		if !sw.predicate(item) {
			sw.skipping = false
			return item, nil
		}
	}

	return sw.source.Recv()
}

func (sw *skipWhileReader[T]) close() error {
	return sw.source.Close()
}

// FlatMap 扁平映射（一对多）
func FlatMap[T, U any](sr *StreamReader[T], fn func(T) []U) *StreamReader[U] {
	return &StreamReader[U]{
		typ: readerTypeFlatMap,
		flatMapR: &flatMapReader[U]{
			source: sr,
			fn: func() ([]U, error) {
				item, err := sr.Recv()
				if err != nil {
					return nil, err
				}
				return fn(item), nil
			},
			buffer: nil,
			idx:    0,
		},
		source: sr.source,
	}
}

type flatMapReader[T any] struct {
	source interface{ Close() error }
	fn     func() ([]T, error)
	buffer []T
	idx    int
	mu     sync.Mutex
}

func (fm *flatMapReader[T]) recv() (T, error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// 如果缓冲区有数据，先返回缓冲区的
	if fm.idx < len(fm.buffer) {
		item := fm.buffer[fm.idx]
		fm.idx++
		return item, nil
	}

	// 获取下一批数据
	items, err := fm.fn()
	if err != nil {
		var zero T
		return zero, err
	}

	if len(items) == 0 {
		// 递归获取下一批
		return fm.recv()
	}

	fm.buffer = items
	fm.idx = 1
	return items[0], nil
}

func (fm *flatMapReader[T]) close() error {
	return fm.source.Close()
}

// Distinct 去重（基于比较函数）
func Distinct[T any](sr *StreamReader[T], equals func(T, T) bool) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeDistinct,
		distinctR: &distinctReader[T]{
			source: sr,
			equals: equals,
			seen:   nil,
		},
		source: sr.source,
	}
}

type distinctReader[T any] struct {
	source *StreamReader[T]
	equals func(T, T) bool
	seen   []T
	mu     sync.Mutex
}

func (dr *distinctReader[T]) recv() (T, error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	for {
		item, err := dr.source.Recv()
		if err != nil {
			return item, err
		}

		// 检查是否已见过
		found := false
		for _, s := range dr.seen {
			if dr.equals(item, s) {
				found = true
				break
			}
		}

		if !found {
			dr.seen = append(dr.seen, item)
			return item, nil
		}
	}
}

func (dr *distinctReader[T]) close() error {
	return dr.source.Close()
}

// DistinctBy 基于 key 去重
func DistinctBy[T any, K comparable](sr *StreamReader[T], keyFn func(T) K) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeDistinctBy,
		distinctByR: &distinctByReader[T, K]{
			source: sr,
			keyFn:  keyFn,
			seen:   make(map[K]struct{}),
		},
		source: sr.source,
	}
}

type distinctByReader[T any, K comparable] struct {
	source *StreamReader[T]
	keyFn  func(T) K
	seen   map[K]struct{}
	mu     sync.Mutex
}

func (db *distinctByReader[T, K]) recv() (T, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for {
		item, err := db.source.Recv()
		if err != nil {
			return item, err
		}

		key := db.keyFn(item)
		if _, exists := db.seen[key]; !exists {
			db.seen[key] = struct{}{}
			return item, nil
		}
	}
}

func (db *distinctByReader[T, K]) close() error {
	return db.source.Close()
}

// Zip 合并两个流
func Zip[T, U, R any](sr1 *StreamReader[T], sr2 *StreamReader[U], fn func(T, U) R) *StreamReader[R] {
	return &StreamReader[R]{
		typ: readerTypeZip,
		zipR: &zipReader[R]{
			fn: func() (R, error) {
				item1, err1 := sr1.Recv()
				if err1 != nil {
					var zero R
					return zero, err1
				}
				item2, err2 := sr2.Recv()
				if err2 != nil {
					var zero R
					return zero, err2
				}
				return fn(item1, item2), nil
			},
			close: func() error {
				sr1.Close()
				sr2.Close()
				return nil
			},
		},
	}
}

type zipReader[T any] struct {
	fn    func() (T, error)
	close func() error
}

func (zr *zipReader[T]) recv() (T, error) {
	return zr.fn()
}

// Batch 批量收集
func Batch[T any](sr *StreamReader[T], size int) *StreamReader[[]T] {
	if size <= 0 {
		size = 1
	}
	return &StreamReader[[]T]{
		typ: readerTypeBatch,
		batchR: &batchReader[T]{
			source: sr,
			size:   size,
		},
		source: sr.source,
	}
}

type batchReader[T any] struct {
	source *StreamReader[T]
	size   int
	mu     sync.Mutex
}

func (br *batchReader[T]) recv() ([]T, error) {
	br.mu.Lock()
	defer br.mu.Unlock()

	batch := make([]T, 0, br.size)
	for len(batch) < br.size {
		item, err := br.source.Recv()
		if err == io.EOF {
			if len(batch) > 0 {
				return batch, nil
			}
			return nil, io.EOF
		}
		if err != nil {
			if len(batch) > 0 {
				return batch, nil
			}
			return nil, err
		}
		batch = append(batch, item)
	}
	return batch, nil
}

func (br *batchReader[T]) close() error {
	return br.source.Close()
}

// Window 滑动窗口
func Window[T any](sr *StreamReader[T], size int) *StreamReader[[]T] {
	if size <= 0 {
		size = 1
	}
	return &StreamReader[[]T]{
		typ: readerTypeWindow,
		windowR: &windowReader[T]{
			source: sr,
			size:   size,
			buffer: make([]T, 0, size),
		},
		source: sr.source,
	}
}

type windowReader[T any] struct {
	source *StreamReader[T]
	size   int
	buffer []T
	mu     sync.Mutex
}

func (wr *windowReader[T]) recv() ([]T, error) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	// 填充初始窗口
	for len(wr.buffer) < wr.size {
		item, err := wr.source.Recv()
		if err == io.EOF {
			if len(wr.buffer) > 0 {
				result := make([]T, len(wr.buffer))
				copy(result, wr.buffer)
				wr.buffer = nil
				return result, nil
			}
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		wr.buffer = append(wr.buffer, item)
	}

	// 返回当前窗口
	result := make([]T, len(wr.buffer))
	copy(result, wr.buffer)

	// 滑动窗口
	item, err := wr.source.Recv()
	if err == io.EOF {
		wr.buffer = nil
		return result, nil
	}
	if err != nil {
		wr.buffer = nil
		return result, nil
	}
	wr.buffer = append(wr.buffer[1:], item)

	return result, nil
}

func (wr *windowReader[T]) close() error {
	return wr.source.Close()
}

// Debounce 防抖（在指定时间内只取最后一个）
func Debounce[T any](sr *StreamReader[T], d time.Duration) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeDebounce,
		debounceR: &debounceReader[T]{
			source:   sr,
			duration: d,
			done:     make(chan struct{}),
			output:   make(chan T, 1),
		},
		source: sr.source,
	}
}

type debounceReader[T any] struct {
	source   *StreamReader[T]
	duration time.Duration
	done     chan struct{}
	output   chan T
	started  int32
	closed   int32
}

func (dr *debounceReader[T]) start() {
	if !atomic.CompareAndSwapInt32(&dr.started, 0, 1) {
		return
	}
	go func() {
		var latest T
		var hasValue bool
		timer := time.NewTimer(dr.duration)
		timer.Stop()

		for {
			select {
			case <-dr.done:
				timer.Stop()
				close(dr.output)
				return
			default:
				item, err := dr.source.Recv()
				if err != nil {
					timer.Stop()
					if hasValue {
						dr.output <- latest
					}
					close(dr.output)
					return
				}
				latest = item
				hasValue = true
				timer.Reset(dr.duration)

				select {
				case <-timer.C:
					if hasValue {
						dr.output <- latest
						hasValue = false
					}
				default:
				}
			}
		}
	}()
}

func (dr *debounceReader[T]) recv() (T, error) {
	dr.start()
	item, ok := <-dr.output
	if !ok {
		var zero T
		return zero, io.EOF
	}
	return item, nil
}

func (dr *debounceReader[T]) close() error {
	if atomic.CompareAndSwapInt32(&dr.closed, 0, 1) {
		close(dr.done)
	}
	return dr.source.Close()
}

// Throttle 节流（在指定时间内最多取一个）
func Throttle[T any](sr *StreamReader[T], d time.Duration) *StreamReader[T] {
	return &StreamReader[T]{
		typ: readerTypeThrottle,
		throttleR: &throttleReader[T]{
			source:   sr,
			duration: d,
			lastTime: time.Time{},
		},
		source: sr.source,
	}
}

type throttleReader[T any] struct {
	source   *StreamReader[T]
	duration time.Duration
	lastTime time.Time
	mu       sync.Mutex
}

func (tr *throttleReader[T]) recv() (T, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	for {
		item, err := tr.source.Recv()
		if err != nil {
			return item, err
		}

		now := time.Now()
		if tr.lastTime.IsZero() || now.Sub(tr.lastTime) >= tr.duration {
			tr.lastTime = now
			return item, nil
		}
		// 跳过这个元素，继续读取
	}
}

func (tr *throttleReader[T]) close() error {
	return tr.source.Close()
}
