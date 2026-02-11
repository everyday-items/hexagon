package interrupt

import (
	"context"
)

// ============== 信号树持久化 ==============

// SignalToPersistenceMaps 将信号树扁平化为两个 map
//
// 遍历信号树的所有节点，提取每个中断点的地址和状态。
// 返回的 map 可以直接 JSON/gob 序列化保存。
//
// 参数：
//   - signal: 中断信号树的根节点
//
// 返回：
//   - id2Addr: 中断 ID → 层级地址
//   - id2State: 中断 ID → 组件内部状态（仅包含有 State 的节点）
func SignalToPersistenceMaps(signal *InterruptSignal) (
	id2Addr map[string]Address,
	id2State map[string]any,
) {
	id2Addr = make(map[string]Address)
	id2State = make(map[string]any)

	if signal == nil {
		return
	}

	flattenSignal(signal, id2Addr, id2State)
	return
}

// flattenSignal 递归遍历信号树并扁平化
func flattenSignal(signal *InterruptSignal, id2Addr map[string]Address, id2State map[string]any) {
	id2Addr[signal.ID] = signal.Address
	if signal.State != nil {
		id2State[signal.ID] = signal.State
	}

	for _, sub := range signal.Subs {
		flattenSignal(sub, id2Addr, id2State)
	}
}

// PopulateResumeInfo 从持久化数据恢复全局恢复信息到 context
//
// 在恢复执行前调用，将之前保存的中断点信息注入 context。
// 之后再调用 Resume/ResumeWithData 标记要恢复的中断点。
//
// 用法：
//
//	// 从存储加载中断信息
//	ctx = interrupt.PopulateResumeInfo(ctx, savedAddrs, savedStates)
//	// 恢复指定中断点
//	ctx = interrupt.ResumeWithData(ctx, interruptID, approvalData)
//	// 重新执行图
//	state, err = graph.Run(ctx, restoredState)
func PopulateResumeInfo(ctx context.Context,
	id2Addr map[string]Address,
	id2State map[string]any,
) context.Context {
	gri := &globalResumeInfo{
		id2Addr:       make(map[string]Address),
		id2State:      make(map[string]any),
		id2ResumeData: make(map[string]any),
		id2StateUsed:  make(map[string]bool),
		id2DataUsed:   make(map[string]bool),
	}

	for id, addr := range id2Addr {
		// 复制地址以防外部修改
		copied := make(Address, len(addr))
		copy(copied, addr)
		gri.id2Addr[id] = copied
	}

	for id, state := range id2State {
		gri.id2State[id] = state
	}

	return setGlobalResumeInfo(ctx, gri)
}

// ============== 用户面向的中断上下文 ==============

// InterruptContext 用户面向的中断上下文
//
// 从信号树提取的平面视图，便于用户列举和处理所有中断点。
// 每个 InterruptContext 对应信号树中的一个节点。
type InterruptContext struct {
	// ID 中断点唯一 ID（对应 InterruptSignal.ID）
	ID string

	// Address 层级地址
	Address Address

	// Info 中断信息
	Info any

	// IsRoot 是否为根因（叶子节点）
	IsRoot bool

	// Parent 父中断（CompositeInterrupt 的父节点）
	Parent *InterruptContext
}

// ToInterruptContexts 将信号树转换为用户面向的平面列表
//
// 递归遍历信号树，将每个节点转换为 InterruptContext。
// 可通过 filterTypes 只保留指定地址段类型的节点，
// 例如只看 agent+tool 层，隐藏 node/subgraph 的实现细节。
//
// 参数：
//   - signal: 中断信号树
//   - filterTypes: 地址段类型过滤器（为空则保留所有节点）
//
// 返回所有匹配的 InterruptContext 列表
func ToInterruptContexts(signal *InterruptSignal, filterTypes ...AddressSegmentType) []*InterruptContext {
	if signal == nil {
		return nil
	}

	var result []*InterruptContext
	collectContexts(signal, nil, filterTypes, &result)
	return result
}

// collectContexts 递归收集中断上下文
func collectContexts(signal *InterruptSignal, parent *InterruptContext, filterTypes []AddressSegmentType, result *[]*InterruptContext) {
	ic := &InterruptContext{
		ID:      signal.ID,
		Address: signal.Address,
		Info:    signal.Info,
		IsRoot:  signal.IsRoot,
		Parent:  parent,
	}

	// 应用过滤
	if matchesFilter(signal.Address, filterTypes) {
		*result = append(*result, ic)
	}

	// 递归处理子中断
	for _, sub := range signal.Subs {
		collectContexts(sub, ic, filterTypes, result)
	}
}

// matchesFilter 检查地址是否匹配过滤条件
// 如果 filterTypes 为空，则始终匹配
func matchesFilter(addr Address, filterTypes []AddressSegmentType) bool {
	if len(filterTypes) == 0 {
		return true
	}

	// 检查地址的最后一段是否匹配任一过滤类型
	if len(addr) == 0 {
		return false
	}

	lastSeg := addr[len(addr)-1]
	for _, ft := range filterTypes {
		if lastSeg.Type == ft {
			return true
		}
	}
	return false
}
