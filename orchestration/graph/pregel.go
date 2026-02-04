// pregel.go - Pregel 循环图编排模式
//
// Pregel 是 Google 的图计算模型，适合需要迭代计算的循环图。
// 借鉴 Eino 框架的精细实现，支持：
//   - 循环图执行（有环图）
//   - AnyPredecessor 触发模式
//   - 超级步（Superstep）并行执行
//   - 最大迭代限制
//
// 使用场景：
//   - 多轮对话推理
//   - 迭代优化算法
//   - 自我修正循环
//   - 复杂决策树

package graph

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// ============== Pregel 触发模式 ==============

// TriggerMode 节点触发模式
type TriggerMode int

const (
	// TriggerAllPredecessors 所有前驱完成后触发（默认 DAG 模式）
	// 节点等待所有入边的前驱节点都执行完成后才开始执行
	TriggerAllPredecessors TriggerMode = iota

	// TriggerAnyPredecessor 任意前驱完成后触发（Pregel 模式）
	// 节点在任意一个前驱节点完成后就可以开始执行
	// 适用于循环图，允许节点在不同轮次多次执行
	TriggerAnyPredecessor
)

// String 返回触发模式的字符串表示
func (m TriggerMode) String() string {
	switch m {
	case TriggerAllPredecessors:
		return "all_predecessors"
	case TriggerAnyPredecessor:
		return "any_predecessor"
	default:
		return "unknown"
	}
}

// ============== Pregel 配置 ==============

// PregelStateMerger 状态合并器接口
// 用于在 Pregel 并行执行时合并多个节点的输出状态
type PregelStateMerger[S State] interface {
	// Merge 合并多个状态为一个
	// base 是执行前的基础状态
	// states 是各节点执行后的状态列表
	Merge(base S, states []S) S
}

// DefaultPregelMerger 默认状态合并器
// 使用最后一个状态（向后兼容）
type DefaultPregelMerger[S State] struct{}

// Merge 使用最后一个状态
func (m *DefaultPregelMerger[S]) Merge(base S, states []S) S {
	if len(states) == 0 {
		return base
	}
	return states[len(states)-1]
}

// PregelConfig Pregel 执行配置
type PregelConfig struct {
	// MaxSupersteps 最大超级步数量（防止无限循环）
	// 默认 100
	MaxSupersteps int

	// TriggerMode 默认触发模式
	// 默认 TriggerAnyPredecessor
	TriggerMode TriggerMode

	// ParallelExecution 是否并行执行同一超级步内的节点
	// 默认 true
	ParallelExecution bool

	// TerminationCheck 终止检查函数
	// 返回 true 表示应该终止迭代
	// 如果为 nil，则只检查是否到达 END 节点
	TerminationCheck func(step int, activeNodes []string) bool

	// Debug 调试模式
	Debug bool
}

// DefaultPregelConfig 返回默认的 Pregel 配置
func DefaultPregelConfig() *PregelConfig {
	return &PregelConfig{
		MaxSupersteps:     100,
		TriggerMode:       TriggerAnyPredecessor,
		ParallelExecution: true,
		TerminationCheck:  nil,
		Debug:             false,
	}
}

// PregelOption Pregel 配置选项
type PregelOption func(*PregelConfig)

// WithMaxSupersteps 设置最大超级步数量
func WithMaxSupersteps(n int) PregelOption {
	return func(c *PregelConfig) {
		if n > 0 {
			c.MaxSupersteps = n
		}
	}
}

// WithPregelTriggerMode 设置触发模式
func WithPregelTriggerMode(mode TriggerMode) PregelOption {
	return func(c *PregelConfig) {
		c.TriggerMode = mode
	}
}

// WithParallelExecution 设置是否并行执行
func WithParallelExecution(parallel bool) PregelOption {
	return func(c *PregelConfig) {
		c.ParallelExecution = parallel
	}
}

// WithTerminationCheck 设置终止检查函数
func WithTerminationCheck(fn func(step int, activeNodes []string) bool) PregelOption {
	return func(c *PregelConfig) {
		c.TerminationCheck = fn
	}
}

// WithPregelDebug 设置调试模式
func WithPregelDebug(debug bool) PregelOption {
	return func(c *PregelConfig) {
		c.Debug = debug
	}
}

// PregelExecutorOption PregelExecutor 配置选项
type PregelExecutorOption[S State] func(*PregelExecutor[S])

// WithPregelMerger 设置状态合并器
// 用于在并行执行时正确合并多个节点的输出状态
func WithPregelMerger[S State](merger PregelStateMerger[S]) PregelExecutorOption[S] {
	return func(pe *PregelExecutor[S]) {
		pe.merger = merger
	}
}

// ============== Pregel 执行器 ==============

// PregelExecutor Pregel 图执行器
// 支持循环图的迭代执行
type PregelExecutor[S State] struct {
	graph  *Graph[S]
	config *PregelConfig

	// 状态合并器
	merger PregelStateMerger[S]

	// 运行时状态
	state       S
	superstep   int32 // 当前超级步
	activeNodes map[string]bool
	nodeInputs  map[string][]S // 节点收到的输入消息

	// 前驱统计
	predecessors   map[string][]string // node -> predecessors
	completedPreds map[string]int      // node -> completed predecessor count
	predMu         sync.RWMutex

	mu sync.Mutex
}

// NewPregelExecutor 创建 Pregel 执行器
func NewPregelExecutor[S State](g *Graph[S], opts ...PregelOption) *PregelExecutor[S] {
	config := DefaultPregelConfig()
	for _, opt := range opts {
		opt(config)
	}

	pe := &PregelExecutor[S]{
		graph:          g,
		config:         config,
		merger:         &DefaultPregelMerger[S]{}, // 默认合并器
		activeNodes:    make(map[string]bool),
		nodeInputs:     make(map[string][]S),
		predecessors:   make(map[string][]string),
		completedPreds: make(map[string]int),
	}

	// 构建前驱关系
	pe.buildPredecessors()

	return pe
}

// NewPregelExecutorWithMerger 创建带状态合并器的 Pregel 执行器
func NewPregelExecutorWithMerger[S State](g *Graph[S], merger PregelStateMerger[S], opts ...PregelOption) *PregelExecutor[S] {
	pe := NewPregelExecutor(g, opts...)
	if merger != nil {
		pe.merger = merger
	}
	return pe
}

// buildPredecessors 构建前驱关系映射
func (pe *PregelExecutor[S]) buildPredecessors() {
	// 从边信息构建前驱映射
	for _, edge := range pe.graph.Edges {
		pe.predecessors[edge.To] = append(pe.predecessors[edge.To], edge.From)
	}

	// 从条件边构建前驱映射
	for from, condEdges := range pe.graph.conditionalEdges {
		for _, ce := range condEdges {
			for _, to := range ce.edges {
				pe.predecessors[to] = append(pe.predecessors[to], from)
			}
		}
	}
}

// Run 执行 Pregel 图
// 返回最终状态和执行的超级步数量
func (pe *PregelExecutor[S]) Run(ctx context.Context, initialState S) (S, int, error) {
	if !pe.graph.compiled {
		return initialState, 0, fmt.Errorf("graph not compiled")
	}

	pe.mu.Lock()
	pe.state = initialState
	pe.superstep = 0
	pe.mu.Unlock()

	// 初始化：激活入口节点
	entryPoint := pe.graph.EntryPoint
	if entryPoint == "" {
		entryPoint = START
	}
	pe.activeNodes[entryPoint] = true

	// 执行超级步循环
	for {
		select {
		case <-ctx.Done():
			return pe.state, int(pe.superstep), ctx.Err()
		default:
		}

		// 检查最大迭代
		currentStep := int(atomic.LoadInt32(&pe.superstep))
		if currentStep >= pe.config.MaxSupersteps {
			return pe.state, currentStep, fmt.Errorf("max supersteps (%d) exceeded", pe.config.MaxSupersteps)
		}

		// 获取活跃节点列表
		activeList := pe.getActiveNodes()
		if len(activeList) == 0 {
			// 没有活跃节点，执行完成
			return pe.state, currentStep, nil
		}

		// 检查自定义终止条件
		if pe.config.TerminationCheck != nil && pe.config.TerminationCheck(currentStep, activeList) {
			return pe.state, currentStep, nil
		}

		// 检查是否只有 END 节点活跃
		if len(activeList) == 1 && activeList[0] == END {
			return pe.state, currentStep, nil
		}

		// 执行当前超级步
		if err := pe.executeSuperstep(ctx, activeList); err != nil {
			return pe.state, currentStep, err
		}

		atomic.AddInt32(&pe.superstep, 1)
	}
}

// getActiveNodes 获取活跃节点列表
func (pe *PregelExecutor[S]) getActiveNodes() []string {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	nodes := make([]string, 0, len(pe.activeNodes))
	for node, active := range pe.activeNodes {
		if active {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// executeSuperstep 执行单个超级步
func (pe *PregelExecutor[S]) executeSuperstep(ctx context.Context, activeNodes []string) error {
	// 过滤掉 START 和 END 节点
	nodesToExecute := make([]string, 0, len(activeNodes))
	for _, node := range activeNodes {
		if node != START && node != END {
			nodesToExecute = append(nodesToExecute, node)
		}
	}

	// 清除活跃节点（执行后重新激活后继节点）
	pe.mu.Lock()
	pe.activeNodes = make(map[string]bool)
	pe.mu.Unlock()

	if len(nodesToExecute) == 0 {
		// 如果只有 START 活跃，激活其后继节点
		for _, node := range activeNodes {
			if node == START {
				pe.activateSuccessors(node)
			}
		}
		return nil
	}

	if pe.config.ParallelExecution {
		return pe.executeNodesParallel(ctx, nodesToExecute)
	}
	return pe.executeNodesSequential(ctx, nodesToExecute)
}

// executeNodesParallel 并行执行节点
func (pe *PregelExecutor[S]) executeNodesParallel(ctx context.Context, nodes []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(nodes))
	resultCh := make(chan nodeResult[S], len(nodes))

	// 保存执行前的基础状态
	pe.mu.Lock()
	baseState := pe.state
	pe.mu.Unlock()

	for _, nodeName := range nodes {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			node, ok := pe.graph.Nodes[name]
			if !ok {
				errCh <- fmt.Errorf("node %s not found", name)
				return
			}

			// 执行节点（使用基础状态的副本）
			newState, err := node.Handler(ctx, baseState)
			if err != nil {
				errCh <- fmt.Errorf("node %s failed: %w", name, err)
				return
			}

			resultCh <- nodeResult[S]{name: name, state: newState}
		}(nodeName)
	}

	// 等待所有节点完成
	go func() {
		wg.Wait()
		close(errCh)
		close(resultCh)
	}()

	// 收集错误
	var firstError error
	for err := range errCh {
		if firstError == nil {
			firstError = err
		}
	}
	if firstError != nil {
		return firstError
	}

	// 收集所有结果并使用状态合并器合并
	var results []nodeResult[S]
	for result := range resultCh {
		results = append(results, result)
	}

	// 提取状态列表
	states := make([]S, len(results))
	for i, r := range results {
		states[i] = r.state
	}

	// 使用合并器合并状态
	pe.mu.Lock()
	pe.state = pe.merger.Merge(baseState, states)
	pe.mu.Unlock()

	// 激活后继节点
	for _, result := range results {
		pe.activateSuccessors(result.name)
	}

	return nil
}

// executeNodesSequential 顺序执行节点
func (pe *PregelExecutor[S]) executeNodesSequential(ctx context.Context, nodes []string) error {
	for _, nodeName := range nodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, ok := pe.graph.Nodes[nodeName]
		if !ok {
			return fmt.Errorf("node %s not found", nodeName)
		}

		// 执行节点
		newState, err := node.Handler(ctx, pe.state)
		if err != nil {
			return fmt.Errorf("node %s failed: %w", nodeName, err)
		}

		pe.mu.Lock()
		pe.state = newState
		pe.mu.Unlock()

		pe.activateSuccessors(nodeName)
	}

	return nil
}

// activateSuccessors 激活后继节点
func (pe *PregelExecutor[S]) activateSuccessors(nodeName string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// 检查条件边
	if condEdges, ok := pe.graph.conditionalEdges[nodeName]; ok {
		for _, ce := range condEdges {
			label := ce.router(pe.state)
			if target, ok := ce.edges[label]; ok {
				if pe.shouldActivate(target, nodeName) {
					pe.activeNodes[target] = true
				}
			}
		}
	}

	// 检查普通边
	if targets, ok := pe.graph.adjacency[nodeName]; ok {
		for _, target := range targets {
			if pe.shouldActivate(target, nodeName) {
				pe.activeNodes[target] = true
			}
		}
	}
}

// shouldActivate 检查节点是否应该被激活
// 根据 TriggerMode 决定是否激活
// 注意：调用此方法时，外层已持有 pe.mu 锁
func (pe *PregelExecutor[S]) shouldActivate(node, completedPred string) bool {
	switch pe.config.TriggerMode {
	case TriggerAnyPredecessor:
		// 任意前驱完成即可激活
		return true

	case TriggerAllPredecessors:
		// 所有前驱完成才激活
		pe.predMu.Lock()
		defer pe.predMu.Unlock()

		preds := pe.predecessors[node]
		if len(preds) == 0 {
			return true
		}

		// 原子地增加并检查计数
		pe.completedPreds[node]++
		completed := pe.completedPreds[node]
		required := len(preds)

		// 如果达到要求，重置计数以便下一轮
		if completed >= required {
			pe.completedPreds[node] = 0
			return true
		}
		return false

	default:
		return true
	}
}

// nodeResult 节点执行结果
type nodeResult[S State] struct {
	name  string
	state S
}

// ============== Graph 扩展方法 ==============

// RunPregelMode 以 Pregel 模式执行图
// 支持循环图和迭代执行
func (g *Graph[S]) RunPregelMode(ctx context.Context, initialState S, opts ...PregelOption) (S, int, error) {
	executor := NewPregelExecutor(g, opts...)
	return executor.Run(ctx, initialState)
}

// StreamPregelMode 以 Pregel 模式流式执行图
// 每个超级步发送一个事件
func (g *Graph[S]) StreamPregelMode(ctx context.Context, initialState S, opts ...PregelOption) (<-chan PregelEvent[S], error) {
	if !g.compiled {
		return nil, fmt.Errorf("graph not compiled")
	}

	events := make(chan PregelEvent[S], 10)
	executor := NewPregelExecutor(g, opts...)

	go func() {
		defer close(events)

		executor.mu.Lock()
		executor.state = initialState
		executor.superstep = 0
		executor.mu.Unlock()

		// 初始化
		entryPoint := g.EntryPoint
		if entryPoint == "" {
			entryPoint = START
		}
		executor.activeNodes[entryPoint] = true

		// sendEvent 发送事件
		sendEvent := func(evt PregelEvent[S]) bool {
			select {
			case <-ctx.Done():
				return false
			case events <- evt:
				return true
			}
		}

		// 执行超级步循环
		for {
			select {
			case <-ctx.Done():
				sendEvent(PregelEvent[S]{
					Type:  PregelEventError,
					Error: ctx.Err(),
				})
				return
			default:
			}

			currentStep := int(atomic.LoadInt32(&executor.superstep))
			if currentStep >= executor.config.MaxSupersteps {
				sendEvent(PregelEvent[S]{
					Type:  PregelEventError,
					Error: fmt.Errorf("max supersteps exceeded"),
				})
				return
			}

			activeList := executor.getActiveNodes()
			if len(activeList) == 0 || (len(activeList) == 1 && activeList[0] == END) {
				sendEvent(PregelEvent[S]{
					Type:      PregelEventComplete,
					State:     executor.state,
					Superstep: currentStep,
				})
				return
			}

			// 发送超级步开始事件
			if !sendEvent(PregelEvent[S]{
				Type:        PregelEventSuperstepStart,
				Superstep:   currentStep,
				ActiveNodes: activeList,
			}) {
				return
			}

			// 执行超级步
			if err := executor.executeSuperstep(ctx, activeList); err != nil {
				sendEvent(PregelEvent[S]{
					Type:  PregelEventError,
					Error: err,
				})
				return
			}

			// 发送超级步完成事件
			if !sendEvent(PregelEvent[S]{
				Type:      PregelEventSuperstepEnd,
				State:     executor.state,
				Superstep: currentStep,
			}) {
				return
			}

			atomic.AddInt32(&executor.superstep, 1)
		}
	}()

	return events, nil
}

// PregelEvent Pregel 执行事件
type PregelEvent[S State] struct {
	// Type 事件类型
	Type PregelEventType

	// Superstep 当前超级步编号
	Superstep int

	// ActiveNodes 活跃节点列表
	ActiveNodes []string

	// State 当前状态
	State S

	// Error 错误信息
	Error error
}

// PregelEventType Pregel 事件类型
type PregelEventType int

const (
	// PregelEventSuperstepStart 超级步开始
	PregelEventSuperstepStart PregelEventType = iota
	// PregelEventSuperstepEnd 超级步结束
	PregelEventSuperstepEnd
	// PregelEventComplete 执行完成
	PregelEventComplete
	// PregelEventError 错误
	PregelEventError
)

// String 返回事件类型的字符串表示
func (t PregelEventType) String() string {
	switch t {
	case PregelEventSuperstepStart:
		return "superstep_start"
	case PregelEventSuperstepEnd:
		return "superstep_end"
	case PregelEventComplete:
		return "complete"
	case PregelEventError:
		return "error"
	default:
		return "unknown"
	}
}
