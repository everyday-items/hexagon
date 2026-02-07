// Package flow 提供轻量级事件驱动编排引擎
//
// Flow 是比 Graph 更轻量的编排范式，参考 CrewAI Flows 设计：
//   - 事件驱动：通过 Start/Listen 定义事件触发关系
//   - 并行启动：多个 Start 步骤同时执行
//   - 逻辑组合：And/Or 组合多个事件的触发条件
//   - 结构化状态：类型安全的流状态管理
//
// 使用示例：
//
//	flow := NewFlow[MyState]("my-flow").
//	    Start("fetch_data", fetchDataStep).
//	    Listen("process", processStep, "fetch_data").
//	    Listen("analyze", analyzeStep, "fetch_data").
//	    ListenAll("merge", mergeStep, "process", "analyze").
//	    Build()
//
//	result, err := flow.Run(ctx, initialState)
package flow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State 流状态约束接口
type State interface {
	any
}

// StepFunc 步骤处理函数
type StepFunc[S any] func(ctx context.Context, state S) (S, error)

// StepType 步骤类型
type StepType int

const (
	// StepTypeStart 启动步骤（流开始时自动执行）
	StepTypeStart StepType = iota

	// StepTypeListen 监听步骤（某个事件触发后执行）
	StepTypeListen

	// StepTypeListenAll 全部监听步骤（所有指定事件都完成后执行）
	StepTypeListenAll

	// StepTypeListenAny 任一监听步骤（任一指定事件完成后执行）
	StepTypeListenAny
)

// Step 流步骤
type Step[S any] struct {
	Name     string
	Type     StepType
	Handler  StepFunc[S]
	ListenTo []string // 监听的事件名称列表
}

// Flow 事件驱动流
type Flow[S any] struct {
	name    string
	steps   map[string]*Step[S]
	starts  []string        // 启动步骤列表
	timeout time.Duration   // 执行超时时间（默认 5 分钟）
}

// FlowBuilder 流构建器
type FlowBuilder[S any] struct {
	flow *Flow[S]
	err  error
}

// NewFlow 创建流构建器
func NewFlow[S any](name string) *FlowBuilder[S] {
	return &FlowBuilder[S]{
		flow: &Flow[S]{
			name:   name,
			steps:  make(map[string]*Step[S]),
			starts: make([]string, 0),
		},
	}
}

// Start 添加启动步骤（流开始时并行执行）
func (b *FlowBuilder[S]) Start(name string, handler StepFunc[S]) *FlowBuilder[S] {
	if _, exists := b.flow.steps[name]; exists {
		b.err = fmt.Errorf("步骤 %q 已存在", name)
		return b
	}

	b.flow.steps[name] = &Step[S]{
		Name:    name,
		Type:    StepTypeStart,
		Handler: handler,
	}
	b.flow.starts = append(b.flow.starts, name)
	return b
}

// Listen 添加监听步骤（单个事件触发）
func (b *FlowBuilder[S]) Listen(name string, handler StepFunc[S], event string) *FlowBuilder[S] {
	if _, exists := b.flow.steps[name]; exists {
		b.err = fmt.Errorf("步骤 %q 已存在", name)
		return b
	}

	b.flow.steps[name] = &Step[S]{
		Name:     name,
		Type:     StepTypeListen,
		Handler:  handler,
		ListenTo: []string{event},
	}
	return b
}

// ListenAll 添加全部监听步骤（所有事件完成后触发）
func (b *FlowBuilder[S]) ListenAll(name string, handler StepFunc[S], events ...string) *FlowBuilder[S] {
	if _, exists := b.flow.steps[name]; exists {
		b.err = fmt.Errorf("步骤 %q 已存在", name)
		return b
	}

	b.flow.steps[name] = &Step[S]{
		Name:     name,
		Type:     StepTypeListenAll,
		Handler:  handler,
		ListenTo: events,
	}
	return b
}

// ListenAny 添加任一监听步骤（任一事件完成后触发）
func (b *FlowBuilder[S]) ListenAny(name string, handler StepFunc[S], events ...string) *FlowBuilder[S] {
	if _, exists := b.flow.steps[name]; exists {
		b.err = fmt.Errorf("步骤 %q 已存在", name)
		return b
	}

	b.flow.steps[name] = &Step[S]{
		Name:     name,
		Type:     StepTypeListenAny,
		Handler:  handler,
		ListenTo: events,
	}
	return b
}

// WithTimeout 设置流执行的超时时间
// 默认为 5 分钟。设置为 0 或负值将使用默认值。
func (b *FlowBuilder[S]) WithTimeout(d time.Duration) *FlowBuilder[S] {
	b.flow.timeout = d
	return b
}

// Build 构建流
func (b *FlowBuilder[S]) Build() (*Flow[S], error) {
	if b.err != nil {
		return nil, b.err
	}
	if len(b.flow.starts) == 0 {
		return nil, fmt.Errorf("流 %q 至少需要一个 Start 步骤", b.flow.name)
	}
	// 默认超时 5 分钟
	if b.flow.timeout <= 0 {
		b.flow.timeout = 5 * time.Minute
	}
	return b.flow, nil
}

// Run 执行流
func (f *Flow[S]) Run(ctx context.Context, state S) (S, error) {
	// 事件完成追踪
	var mu sync.Mutex
	completed := make(map[string]bool)
	currentState := state
	var stateErr error

	// 事件完成通知
	eventCh := make(chan string, len(f.steps))

	// 执行步骤的辅助函数
	executeStep := func(step *Step[S]) {
		// 在锁内拷贝当前状态，避免并发读写竞态
		mu.Lock()
		localState := currentState
		mu.Unlock()

		result, err := step.Handler(ctx, localState)

		mu.Lock()
		if err != nil {
			if stateErr == nil {
				stateErr = fmt.Errorf("步骤 %q 失败: %w", step.Name, err)
			}
		} else {
			currentState = result
		}
		completed[step.Name] = true
		mu.Unlock()

		// 通知事件完成
		select {
		case eventCh <- step.Name:
		case <-ctx.Done():
		}
	}

	// 1. 并行执行所有 Start 步骤
	var startWg sync.WaitGroup
	for _, name := range f.starts {
		step := f.steps[name]
		startWg.Add(1)
		go func(s *Step[S]) {
			defer startWg.Done()
			executeStep(s)
		}(step)
	}

	// 2. 启动事件监听循环
	go func() {
		startWg.Wait()

		// 超时保护
		timeout := time.After(f.timeout)

		for {
			select {
			case <-ctx.Done():
				return
			case <-timeout:
				return
			case completedEvent := <-eventCh:
				_ = completedEvent

				// 检查是否有新步骤可以触发
				mu.Lock()
				for _, step := range f.steps {
					if completed[step.Name] {
						continue
					}

					switch step.Type {
					case StepTypeListen:
						// 单事件触发
						if len(step.ListenTo) > 0 && completed[step.ListenTo[0]] {
							completed[step.Name] = false // 标记为进行中
							mu.Unlock()
							go executeStep(step)
							mu.Lock()
						}

					case StepTypeListenAll:
						// 所有事件都完成后触发
						allDone := true
						for _, ev := range step.ListenTo {
							if !completed[ev] {
								allDone = false
								break
							}
						}
						if allDone {
							completed[step.Name] = false
							mu.Unlock()
							go executeStep(step)
							mu.Lock()
						}

					case StepTypeListenAny:
						// 任一事件完成后触发
						for _, ev := range step.ListenTo {
							if completed[ev] {
								completed[step.Name] = false
								mu.Unlock()
								go executeStep(step)
								mu.Lock()
								break
							}
						}
					}
				}

				// 检查是否所有步骤都完成
				allCompleted := true
				for _, step := range f.steps {
					if !completed[step.Name] {
						allCompleted = false
						break
					}
				}
				mu.Unlock()

				if allCompleted {
					return
				}
			}
		}
	}()

	// 等待所有启动步骤完成
	startWg.Wait()

	// 等待所有后续步骤完成（简化：最多等待配置的超时时间）
	deadline := time.After(f.timeout)
	for {
		select {
		case <-ctx.Done():
			return currentState, ctx.Err()
		case <-deadline:
			return currentState, nil
		case <-time.After(100 * time.Millisecond):
			mu.Lock()
			allDone := true
			for _, step := range f.steps {
				if !completed[step.Name] {
					allDone = false
					break
				}
			}
			mu.Unlock()

			if allDone {
				if stateErr != nil {
					return currentState, stateErr
				}
				return currentState, nil
			}
		}
	}
}

// Name 返回流名称
func (f *Flow[S]) Name() string {
	return f.name
}
