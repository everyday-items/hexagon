package interrupt

import (
	"context"
	"errors"
	"fmt"

	"github.com/everyday-items/hexagon/internal/util"
)

// ============== InterruptSignal 中断信号 ==============

// InterruptSignal 中断信号，实现 error 接口以零侵入传播
//
// 中断信号是一棵树状结构：
//   - 叶子节点（IsRoot=true）是实际触发中断的点
//   - 非叶子节点聚合了多个子中断（CompositeInterrupt）
//
// 通过实现 error 接口，信号可以通过 Go 标准错误传播机制透传，
// 无需修改任何组件的接口签名
type InterruptSignal struct {
	// ID 中断信号唯一标识
	ID string

	// Address 中断发生的层级地址
	Address Address

	// Info 面向用户的中断信息（如审核请求、确认提示等）
	Info any

	// State 组件内部状态（StatefulInterrupt 保存的进度/上下文）
	State any

	// Subs 子中断信号列表（CompositeInterrupt 的子节点）
	Subs []*InterruptSignal

	// IsRoot 是否为根因（叶子节点，即实际触发中断的点）
	IsRoot bool
}

// Error 实现 error 接口
func (s *InterruptSignal) Error() string {
	if s == nil {
		return "interrupt signal: <nil>"
	}
	return fmt.Sprintf("interrupt signal [%s] at %s: %v", s.ID, s.Address.String(), s.Info)
}

// Unwrap 支持 errors.Is/As 解包
// 如果有子中断信号，返回第一个子信号
func (s *InterruptSignal) Unwrap() error {
	if len(s.Subs) > 0 {
		return s.Subs[0]
	}
	return nil
}

// IsInterruptSignal 从 error 中提取 InterruptSignal
// 使用 errors.As 进行解包，支持嵌套错误
func IsInterruptSignal(err error) (*InterruptSignal, bool) {
	var signal *InterruptSignal
	if errors.As(err, &signal) {
		return signal, true
	}
	return nil, false
}

// ============== 三级中断函数 ==============

// InterruptSignalFunc 基础中断 — 不保存组件状态
//
// 用于简单的中断场景，如等待审批、请求用户输入等。
// 中断信号通过 error 返回值传播到调用方。
//
// 用法：
//
//	func reviewNode(ctx context.Context, state *State) (*State, error) {
//	    return state, interrupt.InterruptSignalFunc(ctx, "需要审核此内容")
//	}
func InterruptSignalFunc(ctx context.Context, info any) error {
	addr := GetCurrentAddress(ctx)
	return &InterruptSignal{
		ID:      util.GenerateID("int"),
		Address: addr,
		Info:    info,
		IsRoot:  true,
	}
}

// StatefulInterrupt 有状态中断 — 保存组件内部状态
//
// 用于需要在恢复时跳过已完成工作的场景。
// state 参数保存组件的处理进度，恢复后通过 GetInterruptState[T] 获取。
//
// 用法：
//
//	func batchNode(ctx context.Context, state *State) (*State, error) {
//	    // 恢复检查
//	    _, hasState, progress := interrupt.GetInterruptState[*Progress](ctx)
//	    start := 0
//	    if hasState { start = progress.LastIndex + 1 }
//
//	    for i := start; i < len(items); i++ {
//	        if needsReview(items[i]) {
//	            return state, interrupt.StatefulInterrupt(ctx,
//	                ReviewRequest{Item: items[i]},
//	                &Progress{LastIndex: i},
//	            )
//	        }
//	    }
//	    return state, nil
//	}
func StatefulInterrupt(ctx context.Context, info any, state any) error {
	addr := GetCurrentAddress(ctx)
	return &InterruptSignal{
		ID:      util.GenerateID("int"),
		Address: addr,
		Info:    info,
		State:   state,
		IsRoot:  true,
	}
}

// CompositeInterrupt 组合中断 — 聚合多个子中断
//
// 用于多个子组件（如多工具调用、多子图）同时触发中断的场景。
// 从 subErrors 中提取 InterruptSignal，构建树状结构。
// 非 InterruptSignal 的 error 会被忽略。
//
// 用法：
//
//	func toolsNode(ctx context.Context, state *State) (*State, error) {
//	    var errs []error
//	    for _, call := range state.ToolCalls {
//	        toolCtx := interrupt.AppendAddressSegment(ctx, interrupt.SegmentTool, call.Name, call.ID)
//	        _, err := executeTool(toolCtx, call)
//	        if err != nil {
//	            if _, ok := interrupt.IsInterruptSignal(err); ok {
//	                errs = append(errs, err)
//	                continue
//	            }
//	            return state, err
//	        }
//	    }
//	    if len(errs) > 0 {
//	        return state, interrupt.CompositeInterrupt(ctx, "多个工具需要确认", nil, errs...)
//	    }
//	    return state, nil
//	}
func CompositeInterrupt(ctx context.Context, info any, state any, subErrors ...error) error {
	addr := GetCurrentAddress(ctx)

	signal := &InterruptSignal{
		ID:      util.GenerateID("int"),
		Address: addr,
		Info:    info,
		State:   state,
		IsRoot:  false,
	}

	// 从子错误中提取 InterruptSignal
	for _, err := range subErrors {
		if sub, ok := IsInterruptSignal(err); ok {
			signal.Subs = append(signal.Subs, sub)
		}
	}

	// 如果没有有效的子中断，降级为基础中断
	if len(signal.Subs) == 0 {
		signal.IsRoot = true
	}

	return signal
}
