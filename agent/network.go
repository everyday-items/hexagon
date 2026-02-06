package agent

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// NetworkTopology 网络拓扑类型
type NetworkTopology int

const (
	// TopologyMesh 全连接网格
	TopologyMesh NetworkTopology = iota

	// TopologyHub 中心辐射
	TopologyHub

	// TopologyRing 环形
	TopologyRing

	// TopologyTree 树形
	TopologyTree

	// TopologyCustom 自定义
	TopologyCustom
)

// String 返回拓扑名称
func (t NetworkTopology) String() string {
	switch t {
	case TopologyMesh:
		return "mesh"
	case TopologyHub:
		return "hub"
	case TopologyRing:
		return "ring"
	case TopologyTree:
		return "tree"
	case TopologyCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// MessageType 消息类型
type MessageType int

const (
	// MessageTypeRequest 请求消息
	MessageTypeRequest MessageType = iota

	// MessageTypeResponse 响应消息
	MessageTypeResponse

	// MessageTypeBroadcast 广播消息
	MessageTypeBroadcast

	// MessageTypeEvent 事件消息
	MessageTypeEvent

	// MessageTypeHeartbeat 心跳消息
	MessageTypeHeartbeat
)

// String 返回消息类型名称
func (t MessageType) String() string {
	switch t {
	case MessageTypeRequest:
		return "request"
	case MessageTypeResponse:
		return "response"
	case MessageTypeBroadcast:
		return "broadcast"
	case MessageTypeEvent:
		return "event"
	case MessageTypeHeartbeat:
		return "heartbeat"
	default:
		return "unknown"
	}
}

// NetworkMessage 网络消息
type NetworkMessage struct {
	// ID 消息 ID
	ID string `json:"id"`

	// Type 消息类型
	Type MessageType `json:"type"`

	// From 发送者 Agent ID
	From string `json:"from"`

	// To 接收者 Agent ID（空表示广播）
	To string `json:"to,omitempty"`

	// Topic 消息主题
	Topic string `json:"topic,omitempty"`

	// Content 消息内容
	Content any `json:"content"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`

	// ReplyTo 回复的消息 ID
	ReplyTo string `json:"reply_to,omitempty"`

	// TTL 消息生存时间
	TTL time.Duration `json:"ttl,omitempty"`

	// Priority 优先级（0-9，越大越高）
	Priority int `json:"priority,omitempty"`
}

// NewMessage 创建消息
func NewMessage(from, to string, msgType MessageType, content any) *NetworkMessage {
	return &NetworkMessage{
		ID:        util.GenerateID("msg"),
		Type:      msgType,
		From:      from,
		To:        to,
		Content:   content,
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// NetworkNode 网络节点
type NetworkNode struct {
	// Agent 关联的 Agent
	Agent Agent

	// Neighbors 相邻节点 ID
	Neighbors []string

	// Inbox 收件箱
	Inbox chan *NetworkMessage

	// Status 节点状态
	Status NodeStatus

	// LastHeartbeat 最后心跳时间
	LastHeartbeat time.Time

	// Metadata 节点元数据
	Metadata map[string]any

	// closed 标记收件箱是否已关闭
	closed bool

	// closeOnce 确保只关闭一次
	closeOnce sync.Once

	// mu 保护 closed 字段
	mu sync.RWMutex
}

// CloseInbox 安全关闭收件箱
//
// 使用 sync.Once 确保只关闭一次，避免 panic。
// 此方法是并发安全的。
func (n *NetworkNode) CloseInbox() {
	n.closeOnce.Do(func() {
		n.mu.Lock()
		n.closed = true
		n.mu.Unlock()
		close(n.Inbox)
	})
}

// IsClosed 检查收件箱是否已关闭
func (n *NetworkNode) IsClosed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.closed
}

// NodeStatus 节点状态
type NodeStatus int

const (
	// NodeStatusOnline 在线
	NodeStatusOnline NodeStatus = iota

	// NodeStatusOffline 离线
	NodeStatusOffline

	// NodeStatusBusy 忙碌
	NodeStatusBusy

	// NodeStatusError 错误
	NodeStatusError
)

// String 返回状态名称
func (s NodeStatus) String() string {
	switch s {
	case NodeStatusOnline:
		return "online"
	case NodeStatusOffline:
		return "offline"
	case NodeStatusBusy:
		return "busy"
	case NodeStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, msg *NetworkMessage) (*NetworkMessage, error)

// AgentNetwork 多 Agent 网络
type AgentNetwork struct {
	// ID 网络 ID
	id string

	// Name 网络名称
	name string

	// Topology 网络拓扑
	topology NetworkTopology

	// Nodes 网络节点
	nodes map[string]*NetworkNode

	// Router 消息路由器
	router *MessageRouter

	// Hub 中心节点 ID（用于 Hub 拓扑）
	hub string

	// Handlers 消息处理器
	handlers map[string]MessageHandler

	// GlobalState 全局状态
	globalState GlobalState

	// InboxSize 收件箱大小
	inboxSize int

	// RouterQueueSize 路由器消息队列大小（默认 10000）
	routerQueueSize int

	// HeartbeatInterval 心跳间隔
	heartbeatInterval time.Duration

	// Running 运行状态
	running bool

	mu sync.RWMutex
}

// NetworkOption 网络配置选项
type NetworkOption func(*AgentNetwork)

// NewAgentNetwork 创建 Agent 网络
func NewAgentNetwork(name string, opts ...NetworkOption) *AgentNetwork {
	n := &AgentNetwork{
		id:                util.GenerateID("network"),
		name:              name,
		topology:          TopologyMesh,
		nodes:             make(map[string]*NetworkNode),
		handlers:          make(map[string]MessageHandler),
		globalState:       NewGlobalState(),
		inboxSize:         100,
		routerQueueSize:   10000,
		heartbeatInterval: 30 * time.Second,
	}

	for _, opt := range opts {
		opt(n)
	}

	n.router = NewMessageRouter(n)

	return n
}

// WithNetworkTopology 设置拓扑
func WithNetworkTopology(topology NetworkTopology) NetworkOption {
	return func(n *AgentNetwork) {
		n.topology = topology
	}
}

// WithNetworkHub 设置中心节点
func WithNetworkHub(hubID string) NetworkOption {
	return func(n *AgentNetwork) {
		n.hub = hubID
		n.topology = TopologyHub
	}
}

// WithNetworkInboxSize 设置收件箱大小
func WithNetworkInboxSize(size int) NetworkOption {
	return func(n *AgentNetwork) {
		n.inboxSize = size
	}
}

// WithRouterQueueSize 设置路由器消息队列大小
// 默认 10000，高负载场景可适当增大。
func WithRouterQueueSize(size int) NetworkOption {
	return func(n *AgentNetwork) {
		n.routerQueueSize = size
	}
}

// WithHeartbeatInterval 设置心跳间隔
func WithHeartbeatInterval(interval time.Duration) NetworkOption {
	return func(n *AgentNetwork) {
		n.heartbeatInterval = interval
	}
}

// ID 返回网络 ID
func (n *AgentNetwork) ID() string {
	return n.id
}

// Name 返回网络名称
func (n *AgentNetwork) Name() string {
	return n.name
}

// Topology 返回网络拓扑
func (n *AgentNetwork) Topology() NetworkTopology {
	return n.topology
}

// Register 注册 Agent 到网络
func (n *AgentNetwork) Register(agent Agent) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.nodes[agent.ID()]; exists {
		return fmt.Errorf("agent %s already registered", agent.ID())
	}

	node := &NetworkNode{
		Agent:         agent,
		Neighbors:     make([]string, 0),
		Inbox:         make(chan *NetworkMessage, n.inboxSize),
		Status:        NodeStatusOnline,
		LastHeartbeat: time.Now(),
		Metadata:      make(map[string]any),
	}

	n.nodes[agent.ID()] = node
	n.globalState.RegisterAgent(agent.ID(), agent)

	// 根据拓扑建立连接
	n.updateTopology()

	return nil
}

// Unregister 从网络注销 Agent
//
// 线程安全：此方法使用安全的 channel 关闭机制，不会因为重复关闭或并发发送而 panic。
func (n *AgentNetwork) Unregister(agentID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	node, exists := n.nodes[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 安全关闭收件箱（使用 sync.Once 确保只关闭一次）
	node.CloseInbox()

	// 移除节点
	delete(n.nodes, agentID)

	// 更新其他节点的邻居列表
	for _, other := range n.nodes {
		newNeighbors := make([]string, 0)
		for _, neighbor := range other.Neighbors {
			if neighbor != agentID {
				newNeighbors = append(newNeighbors, neighbor)
			}
		}
		other.Neighbors = newNeighbors
	}

	return nil
}

// GetNode 获取节点
func (n *AgentNetwork) GetNode(agentID string) (*NetworkNode, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	node, ok := n.nodes[agentID]
	return node, ok
}

// GetAgent 获取 Agent
func (n *AgentNetwork) GetAgent(agentID string) (Agent, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if node, ok := n.nodes[agentID]; ok {
		return node.Agent, true
	}
	return nil, false
}

// ListAgents 列出所有 Agent
func (n *AgentNetwork) ListAgents() []Agent {
	n.mu.RLock()
	defer n.mu.RUnlock()

	agents := make([]Agent, 0, len(n.nodes))
	for _, node := range n.nodes {
		agents = append(agents, node.Agent)
	}
	return agents
}

// ListOnlineAgents 列出在线 Agent
func (n *AgentNetwork) ListOnlineAgents() []Agent {
	n.mu.RLock()
	defer n.mu.RUnlock()

	agents := make([]Agent, 0)
	for _, node := range n.nodes {
		if node.Status == NodeStatusOnline {
			agents = append(agents, node.Agent)
		}
	}
	return agents
}

// Send 发送消息给指定 Agent
func (n *AgentNetwork) Send(ctx context.Context, msg *NetworkMessage) error {
	return n.router.Route(ctx, msg)
}

// SendTo 发送消息给指定 Agent（便捷方法）
func (n *AgentNetwork) SendTo(ctx context.Context, from, to string, content any) error {
	msg := NewMessage(from, to, MessageTypeRequest, content)
	return n.Send(ctx, msg)
}

// Broadcast 广播消息给所有 Agent
func (n *AgentNetwork) Broadcast(ctx context.Context, from string, content any) error {
	msg := NewMessage(from, "", MessageTypeBroadcast, content)
	return n.router.Broadcast(ctx, msg)
}

// BroadcastToNeighbors 广播消息给邻居节点
func (n *AgentNetwork) BroadcastToNeighbors(ctx context.Context, from string, content any) error {
	msg := NewMessage(from, "", MessageTypeBroadcast, content)
	return n.router.BroadcastToNeighbors(ctx, msg)
}

// Multicast 多播消息给指定 Agent 列表
func (n *AgentNetwork) Multicast(ctx context.Context, from string, to []string, content any) error {
	msg := NewMessage(from, "", MessageTypeBroadcast, content)
	return n.router.Multicast(ctx, msg, to)
}

// Request 发送请求并等待响应
func (n *AgentNetwork) Request(ctx context.Context, from, to string, content any) (*NetworkMessage, error) {
	msg := NewMessage(from, to, MessageTypeRequest, content)
	return n.router.RequestResponse(ctx, msg)
}

// RegisterHandler 注册消息处理器
func (n *AgentNetwork) RegisterHandler(topic string, handler MessageHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers[topic] = handler
}

// HandleMessage 处理消息
func (n *AgentNetwork) HandleMessage(ctx context.Context, msg *NetworkMessage) (*NetworkMessage, error) {
	n.mu.RLock()
	handler, ok := n.handlers[msg.Topic]
	n.mu.RUnlock()

	if !ok {
		// 默认处理：将消息传递给目标 Agent
		return n.defaultHandler(ctx, msg)
	}

	return handler(ctx, msg)
}

// defaultHandler 默认消息处理器
func (n *AgentNetwork) defaultHandler(ctx context.Context, msg *NetworkMessage) (*NetworkMessage, error) {
	agent, ok := n.GetAgent(msg.To)
	if !ok {
		return nil, fmt.Errorf("agent %s not found", msg.To)
	}

	// 将消息内容作为输入传递给 Agent
	var query string
	switch c := msg.Content.(type) {
	case string:
		query = c
	case Input:
		query = c.Query
	default:
		query = fmt.Sprintf("%v", c)
	}

	output, err := agent.Run(ctx, Input{
		Query: query,
		Context: map[string]any{
			"from":       msg.From,
			"message_id": msg.ID,
			"topic":      msg.Topic,
			"metadata":   msg.Metadata,
		},
	})
	if err != nil {
		return nil, err
	}

	// 创建响应消息
	response := NewMessage(msg.To, msg.From, MessageTypeResponse, output.Content)
	response.ReplyTo = msg.ID
	response.Metadata["output"] = output

	return response, nil
}

// Start 启动网络
func (n *AgentNetwork) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return fmt.Errorf("network already running")
	}
	n.running = true
	n.mu.Unlock()

	// 启动消息分发器
	go n.router.Start(ctx)

	// 启动心跳检测
	go n.heartbeatLoop(ctx)

	return nil
}

// Stop 停止网络
//
// 线程安全：此方法使用安全的 channel 关闭机制，不会因为重复关闭或并发发送而 panic。
func (n *AgentNetwork) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.running = false
	n.router.Stop()

	// 安全关闭所有收件箱（使用 sync.Once 确保只关闭一次）
	for _, node := range n.nodes {
		node.CloseInbox()
	}
}

// heartbeatLoop 心跳检测循环
func (n *AgentNetwork) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(n.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查心跳
func (n *AgentNetwork) checkHeartbeats() {
	n.mu.Lock()
	defer n.mu.Unlock()

	timeout := n.heartbeatInterval * 3
	now := time.Now()

	for _, node := range n.nodes {
		if now.Sub(node.LastHeartbeat) > timeout {
			node.Status = NodeStatusOffline
		}
	}
}

// updateTopology 更新网络拓扑
func (n *AgentNetwork) updateTopology() {
	switch n.topology {
	case TopologyMesh:
		n.buildMeshTopology()
	case TopologyHub:
		n.buildHubTopology()
	case TopologyRing:
		n.buildRingTopology()
	case TopologyTree:
		n.buildTreeTopology()
	}
}

// buildMeshTopology 构建全连接网格拓扑
func (n *AgentNetwork) buildMeshTopology() {
	ids := make([]string, 0, len(n.nodes))
	for id := range n.nodes {
		ids = append(ids, id)
	}

	// 每个节点连接所有其他节点
	for _, node := range n.nodes {
		node.Neighbors = make([]string, 0, len(ids)-1)
		for _, id := range ids {
			if id != node.Agent.ID() {
				node.Neighbors = append(node.Neighbors, id)
			}
		}
	}
}

// buildHubTopology 构建中心辐射拓扑
func (n *AgentNetwork) buildHubTopology() {
	for id, node := range n.nodes {
		if id == n.hub {
			// Hub 连接所有节点
			node.Neighbors = make([]string, 0, len(n.nodes)-1)
			for otherId := range n.nodes {
				if otherId != n.hub {
					node.Neighbors = append(node.Neighbors, otherId)
				}
			}
		} else {
			// 其他节点只连接 Hub
			node.Neighbors = []string{n.hub}
		}
	}
}

// buildRingTopology 构建环形拓扑
func (n *AgentNetwork) buildRingTopology() {
	ids := make([]string, 0, len(n.nodes))
	for id := range n.nodes {
		ids = append(ids, id)
	}

	count := len(ids)
	for i, id := range ids {
		node := n.nodes[id]
		prev := ids[(i-1+count)%count]
		next := ids[(i+1)%count]
		node.Neighbors = []string{prev, next}
	}
}

// buildTreeTopology 构建树形拓扑
func (n *AgentNetwork) buildTreeTopology() {
	ids := make([]string, 0, len(n.nodes))
	for id := range n.nodes {
		ids = append(ids, id)
	}

	// 简单的二叉树结构
	for i, id := range ids {
		node := n.nodes[id]
		node.Neighbors = make([]string, 0)

		// 父节点
		if i > 0 {
			parentIdx := (i - 1) / 2
			node.Neighbors = append(node.Neighbors, ids[parentIdx])
		}

		// 子节点
		leftChildIdx := 2*i + 1
		rightChildIdx := 2*i + 2
		if leftChildIdx < len(ids) {
			node.Neighbors = append(node.Neighbors, ids[leftChildIdx])
		}
		if rightChildIdx < len(ids) {
			node.Neighbors = append(node.Neighbors, ids[rightChildIdx])
		}
	}
}

// Connect 手动连接两个节点（用于 CustomTopology）
func (n *AgentNetwork) Connect(agent1ID, agent2ID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	node1, ok1 := n.nodes[agent1ID]
	node2, ok2 := n.nodes[agent2ID]

	if !ok1 || !ok2 {
		return fmt.Errorf("one or both agents not found")
	}

	// 添加双向连接
	if !slices.Contains(node1.Neighbors, agent2ID) {
		node1.Neighbors = append(node1.Neighbors, agent2ID)
	}
	if !slices.Contains(node2.Neighbors, agent1ID) {
		node2.Neighbors = append(node2.Neighbors, agent1ID)
	}

	return nil
}

// Disconnect 断开两个节点的连接
func (n *AgentNetwork) Disconnect(agent1ID, agent2ID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	node1, ok1 := n.nodes[agent1ID]
	node2, ok2 := n.nodes[agent2ID]

	if !ok1 || !ok2 {
		return fmt.Errorf("one or both agents not found")
	}

	node1.Neighbors = removeString(node1.Neighbors, agent2ID)
	node2.Neighbors = removeString(node2.Neighbors, agent1ID)

	return nil
}

// GetNeighbors 获取邻居节点
func (n *AgentNetwork) GetNeighbors(agentID string) ([]Agent, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	node, ok := n.nodes[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	neighbors := make([]Agent, 0, len(node.Neighbors))
	for _, neighborID := range node.Neighbors {
		if neighborNode, ok := n.nodes[neighborID]; ok {
			neighbors = append(neighbors, neighborNode.Agent)
		}
	}

	return neighbors, nil
}

// Stats 返回网络统计
func (n *AgentNetwork) Stats() NetworkStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	online := 0
	offline := 0
	busy := 0
	totalEdges := 0

	for _, node := range n.nodes {
		switch node.Status {
		case NodeStatusOnline:
			online++
		case NodeStatusOffline:
			offline++
		case NodeStatusBusy:
			busy++
		}
		totalEdges += len(node.Neighbors)
	}

	return NetworkStats{
		TotalNodes:     len(n.nodes),
		OnlineNodes:    online,
		OfflineNodes:   offline,
		BusyNodes:      busy,
		TotalEdges:     totalEdges / 2, // 每条边计算了两次
		Topology:       n.topology.String(),
		MessagesSent:   n.router.messagesSent.Load(),
		MessagesRecv:   n.router.messagesRecv.Load(),
		MessagesFailed: n.router.messagesFailed.Load(),
	}
}

// NetworkStats 网络统计
type NetworkStats struct {
	TotalNodes     int    `json:"total_nodes"`
	OnlineNodes    int    `json:"online_nodes"`
	OfflineNodes   int    `json:"offline_nodes"`
	BusyNodes      int    `json:"busy_nodes"`
	TotalEdges     int    `json:"total_edges"`
	Topology       string `json:"topology"`
	MessagesSent   int64  `json:"messages_sent"`
	MessagesRecv   int64  `json:"messages_recv"`
	MessagesFailed int64  `json:"messages_failed"`
}

// ============== MessageRouter ==============

// MessageRouter 消息路由器
type MessageRouter struct {
	// Network 所属网络
	network *AgentNetwork

	// Queue 消息队列
	queue chan *NetworkMessage

	// PendingResponses 等待响应的请求
	pendingResponses sync.Map // msgID -> chan *NetworkMessage

	// Running 运行状态
	running bool

	// closed 标记队列是否已关闭
	closed bool

	// closeOnce 确保只关闭一次
	closeOnce sync.Once

	// Stats (使用原子类型确保并发安全)
	messagesSent   atomic.Int64
	messagesRecv   atomic.Int64
	messagesFailed atomic.Int64

	mu sync.RWMutex
}

// NewMessageRouter 创建消息路由器
// 队列大小从 network.routerQueueSize 获取，默认 10000。
func NewMessageRouter(network *AgentNetwork) *MessageRouter {
	queueSize := network.routerQueueSize
	if queueSize <= 0 {
		queueSize = 10000
	}
	return &MessageRouter{
		network: network,
		queue:   make(chan *NetworkMessage, queueSize),
	}
}

// Route 路由消息
//
// 如果队列未启动则直接投递，否则放入队列异步处理。
// 线程安全：会检查队列是否已关闭，避免向已关闭的 channel 发送消息。
func (r *MessageRouter) Route(ctx context.Context, msg *NetworkMessage) error {
	r.mu.RLock()
	running := r.running
	closed := r.closed
	r.mu.RUnlock()

	// 如果队列已关闭，返回错误
	if closed {
		return fmt.Errorf("message router is stopped")
	}

	if !running {
		// 直接投递
		return r.deliver(ctx, msg)
	}

	// 放入队列（使用 defer recover 防止并发关闭导致的 panic）
	defer func() {
		if rec := recover(); rec != nil {
			// channel 已关闭，忽略 panic
		}
	}()

	select {
	case r.queue <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Broadcast 广播消息
func (r *MessageRouter) Broadcast(ctx context.Context, msg *NetworkMessage) error {
	r.network.mu.RLock()
	nodes := make([]*NetworkNode, 0, len(r.network.nodes))
	for _, node := range r.network.nodes {
		if node.Agent.ID() != msg.From {
			nodes = append(nodes, node)
		}
	}
	r.network.mu.RUnlock()

	var lastErr error
	for _, node := range nodes {
		msgCopy := *msg
		msgCopy.To = node.Agent.ID()
		if err := r.deliver(ctx, &msgCopy); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// BroadcastToNeighbors 广播给邻居
func (r *MessageRouter) BroadcastToNeighbors(ctx context.Context, msg *NetworkMessage) error {
	neighbors, err := r.network.GetNeighbors(msg.From)
	if err != nil {
		return err
	}

	var lastErr error
	for _, neighbor := range neighbors {
		msgCopy := *msg
		msgCopy.To = neighbor.ID()
		if err := r.deliver(ctx, &msgCopy); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Multicast 多播消息
func (r *MessageRouter) Multicast(ctx context.Context, msg *NetworkMessage, targets []string) error {
	var lastErr error
	for _, target := range targets {
		msgCopy := *msg
		msgCopy.To = target
		if err := r.deliver(ctx, &msgCopy); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// RequestResponse 请求-响应模式
func (r *MessageRouter) RequestResponse(ctx context.Context, msg *NetworkMessage) (*NetworkMessage, error) {
	// 创建响应通道
	respCh := make(chan *NetworkMessage, 1)
	r.pendingResponses.Store(msg.ID, respCh)
	defer r.pendingResponses.Delete(msg.ID)

	// 发送请求
	if err := r.Route(ctx, msg); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// deliver 投递消息
//
// 线程安全：此方法会检查收件箱是否已关闭，避免向已关闭的 channel 发送消息导致 panic。
// 计数器使用原子操作确保并发安全。
func (r *MessageRouter) deliver(ctx context.Context, msg *NetworkMessage) error {
	node, ok := r.network.GetNode(msg.To)
	if !ok {
		r.messagesFailed.Add(1)
		return fmt.Errorf("target agent %s not found", msg.To)
	}

	// 检查收件箱是否已关闭
	if node.IsClosed() {
		r.messagesFailed.Add(1)
		return fmt.Errorf("inbox closed for agent %s", msg.To)
	}

	// 检查节点状态
	if node.Status == NodeStatusOffline {
		r.messagesFailed.Add(1)
		return fmt.Errorf("target agent %s is offline", msg.To)
	}

	// 投递到收件箱（使用 defer recover 防止并发关闭导致的 panic）
	defer func() {
		if rec := recover(); rec != nil {
			// channel 已关闭，忽略 panic
		}
	}()

	select {
	case node.Inbox <- msg:
		r.messagesSent.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		r.messagesFailed.Add(1)
		return fmt.Errorf("inbox full for agent %s", msg.To)
	}
}

// Start 启动路由器
func (r *MessageRouter) Start(ctx context.Context) {
	r.mu.Lock()
	r.running = true
	r.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.queue:
			if msg == nil {
				return
			}
			r.processMessage(ctx, msg)
		}
	}
}

// Stop 停止路由器
//
// 使用 sync.Once 确保队列只关闭一次，避免重复关闭导致 panic。
func (r *MessageRouter) Stop() {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.running = false
		r.closed = true
		r.mu.Unlock()
		close(r.queue)
	})
}

// processMessage 处理消息
//
// 计数器使用原子操作确保并发安全。
func (r *MessageRouter) processMessage(ctx context.Context, msg *NetworkMessage) {
	r.messagesRecv.Add(1)

	// 检查是否是响应消息
	if msg.Type == MessageTypeResponse && msg.ReplyTo != "" {
		if respCh, ok := r.pendingResponses.Load(msg.ReplyTo); ok {
			ch := respCh.(chan *NetworkMessage)
			select {
			case ch <- msg:
			default:
			}
			return
		}
	}

	// 调用网络的消息处理器
	response, err := r.network.HandleMessage(ctx, msg)
	if err != nil {
		r.messagesFailed.Add(1)
		return
	}

	// 如果有响应，发送回去
	if response != nil && msg.From != "" {
		r.deliver(ctx, response)
	}
}

// 辅助函数

func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
