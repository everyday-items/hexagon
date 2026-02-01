package graph

// EdgeType 边类型
type EdgeType int

const (
	// EdgeTypeNormal 普通边
	EdgeTypeNormal EdgeType = iota
	// EdgeTypeConditional 条件边
	EdgeTypeConditional
)

// Edge 图的边
type Edge struct {
	// From 源节点名称
	From string

	// To 目标节点名称
	To string

	// Type 边类型
	Type EdgeType

	// Condition 条件（仅用于条件边）
	// 返回 true 时边才有效
	Condition func(state State) bool

	// Label 边标签（用于条件路由的匹配）
	Label string

	// Priority 优先级（数值越小优先级越高）
	Priority int
}

// EdgeBuilder 边构建器
type EdgeBuilder struct {
	edge *Edge
}

// NewEdge 创建边构建器
func NewEdge(from, to string) *EdgeBuilder {
	return &EdgeBuilder{
		edge: &Edge{
			From: from,
			To:   to,
			Type: EdgeTypeNormal,
		},
	}
}

// WithCondition 设置条件
func (b *EdgeBuilder) WithCondition(cond func(state State) bool) *EdgeBuilder {
	b.edge.Type = EdgeTypeConditional
	b.edge.Condition = cond
	return b
}

// WithLabel 设置标签
func (b *EdgeBuilder) WithLabel(label string) *EdgeBuilder {
	b.edge.Label = label
	return b
}

// WithPriority 设置优先级
func (b *EdgeBuilder) WithPriority(priority int) *EdgeBuilder {
	b.edge.Priority = priority
	return b
}

// Build 构建边
func (b *EdgeBuilder) Build() *Edge {
	return b.edge
}

// Router 路由器接口
// 用于条件路由
type Router[S State] interface {
	// Route 根据状态决定下一个节点
	Route(state S) string
}

// RouterFunc 路由函数类型
type RouterFunc[S State] func(state S) string

// Route 实现 Router 接口
func (f RouterFunc[S]) Route(state S) string {
	return f(state)
}

// MultiRouter 多路由器
// 返回多个可能的下一节点
type MultiRouter[S State] interface {
	// RouteMulti 根据状态决定多个下一节点
	RouteMulti(state S) []string
}

// MultiRouterFunc 多路由函数类型
type MultiRouterFunc[S State] func(state S) []string

// RouteMulti 实现 MultiRouter 接口
func (f MultiRouterFunc[S]) RouteMulti(state S) []string {
	return f(state)
}

// BranchConfig 分支配置
type BranchConfig[S State] struct {
	// Condition 分支条件
	Condition func(S) bool

	// Target 目标节点
	Target string
}

// Branch 创建分支配置
func Branch[S State](condition func(S) bool, target string) BranchConfig[S] {
	return BranchConfig[S]{
		Condition: condition,
		Target:    target,
	}
}

// SelectFirst 选择第一个满足条件的分支
func SelectFirst[S State](state S, branches ...BranchConfig[S]) string {
	for _, branch := range branches {
		if branch.Condition(state) {
			return branch.Target
		}
	}
	return END
}

// Send 标记消息发送节点
type Send struct {
	// Node 目标节点名称
	Node string

	// Data 发送的数据
	Data any
}

// NewSend 创建 Send
func NewSend(node string, data any) Send {
	return Send{
		Node: node,
		Data: data,
	}
}
