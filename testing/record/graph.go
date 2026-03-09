// Package record 提供测试的录制和回放功能
//
// 本文件实现图执行的录制和回放：
//   - GraphCassette: 图执行交互录制会话
//   - GraphInteraction: 单次图节点执行记录
//   - GraphRecorder: 图执行录制器
//   - GraphReplayer: 图执行回放器
package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GraphInteraction 图执行交互记录
//
// 记录图中单个节点的执行信息，包括输入、输出、下一个节点等。
type GraphInteraction struct {
	// ID 交互唯一标识
	ID string `json:"id"`

	// NodeID 节点 ID
	NodeID string `json:"node_id"`

	// NodeName 节点名称
	NodeName string `json:"node_name"`

	// Input 节点输入状态
	Input map[string]any `json:"input,omitempty"`

	// Output 节点输出状态
	Output map[string]any `json:"output,omitempty"`

	// NextNode 下一个执行的节点 ID
	NextNode string `json:"next_node,omitempty"`

	// Duration 节点执行耗时
	Duration time.Duration `json:"duration"`

	// Timestamp 执行时间戳
	Timestamp time.Time `json:"timestamp"`

	// Error 执行错误信息（如果有）
	Error string `json:"error,omitempty"`
}

// GraphCassette 图执行录制会话
//
// 存储完整的图执行过程，包含所有节点的执行记录。
// 支持序列化到文件和从文件加载，便于回放和调试。
type GraphCassette struct {
	// Name 录制会话名称
	Name string `json:"name"`

	// Interactions 交互记录列表
	Interactions []*GraphInteraction `json:"interactions"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt 最后更新时间
	UpdatedAt time.Time `json:"updated_at"`

	mu sync.Mutex
}

// NewGraphCassette 创建图执行录制会话
func NewGraphCassette(name string) *GraphCassette {
	now := time.Now()
	return &GraphCassette{
		Name:         name,
		Interactions: make([]*GraphInteraction, 0),
		Metadata:     make(map[string]any),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddInteraction 添加交互记录
func (c *GraphCassette) AddInteraction(interaction *GraphInteraction) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Interactions = append(c.Interactions, interaction)
	c.UpdatedAt = time.Now()
}

// FindByNode 查找指定节点的所有交互记录
//
// 返回节点 ID 匹配的所有交互记录，按添加顺序排列。
// 同一节点可能被多次执行（例如循环图），因此可能返回多条记录。
func (c *GraphCassette) FindByNode(nodeID string) []*GraphInteraction {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*GraphInteraction, 0)
	for _, interaction := range c.Interactions {
		if interaction.NodeID == nodeID {
			result = append(result, interaction)
		}
	}
	return result
}

// FindByName 查找指定名称的所有交互记录
func (c *GraphCassette) FindByName(nodeName string) []*GraphInteraction {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*GraphInteraction, 0)
	for _, interaction := range c.Interactions {
		if interaction.NodeName == nodeName {
			result = append(result, interaction)
		}
	}
	return result
}

// Len 返回交互记录数量
func (c *GraphCassette) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Interactions)
}

// Get 获取指定索引的交互记录
func (c *GraphCassette) Get(index int) (*GraphInteraction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Interactions) {
		return nil, fmt.Errorf("index %d out of range [0, %d)", index, len(c.Interactions))
	}
	return c.Interactions[index], nil
}

// Save 保存到文件
//
// 自动创建所需目录。使用 JSON 格式，带缩进以便阅读。
func (c *GraphCassette) Save(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadGraphCassette 从文件加载图执行录制
func LoadGraphCassette(path string) (*GraphCassette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var cassette GraphCassette
	if err := json.Unmarshal(data, &cassette); err != nil {
		return nil, fmt.Errorf("反序列化失败: %w", err)
	}

	return &cassette, nil
}

// GraphRecorder 图执行录制器
//
// 用于在图执行过程中录制每个节点的执行情况。
// 线程安全：所有方法都是并发安全的。
type GraphRecorder struct {
	cassette *GraphCassette
	counter  int
	mu       sync.Mutex
}

// NewGraphRecorder 创建图执行录制器
func NewGraphRecorder(cassette *GraphCassette) *GraphRecorder {
	return &GraphRecorder{
		cassette: cassette,
	}
}

// RecordNode 录制节点执行
//
// 参数:
//   - nodeID: 节点 ID
//   - nodeName: 节点名称
//   - input: 节点输入状态
//   - output: 节点输出状态
//   - next: 下一个执行的节点 ID（空字符串表示结束）
//   - dur: 执行耗时
//   - err: 执行错误（nil 表示成功）
func (r *GraphRecorder) RecordNode(nodeID, nodeName string, input, output map[string]any, next string, dur time.Duration, err error) {
	r.mu.Lock()
	r.counter++
	id := fmt.Sprintf("graph_int_%d", r.counter)
	r.mu.Unlock()

	interaction := &GraphInteraction{
		ID:        id,
		NodeID:    nodeID,
		NodeName:  nodeName,
		Input:     input,
		Output:    output,
		NextNode:  next,
		Duration:  dur,
		Timestamp: time.Now(),
	}

	if err != nil {
		interaction.Error = err.Error()
	}

	r.cassette.AddInteraction(interaction)
}

// Cassette 返回关联的录制会话
func (r *GraphRecorder) Cassette() *GraphCassette {
	return r.cassette
}

// Save 保存录制到文件
func (r *GraphRecorder) Save(path string) error {
	return r.cassette.Save(path)
}

// GraphReplayer 图执行回放器
//
// 从录制的 GraphCassette 中按节点 ID 回放执行结果。
// 支持同一节点多次执行的场景（使用访问计数器）。
type GraphReplayer struct {
	cassette    *GraphCassette
	nodeVisits  map[string]int // 节点访问计数，支持同一节点多次执行
	mu          sync.Mutex
}

// NewGraphReplayer 创建图执行回放器
func NewGraphReplayer(cassette *GraphCassette) *GraphReplayer {
	return &GraphReplayer{
		cassette:   cassette,
		nodeVisits: make(map[string]int),
	}
}

// ReplayNode 回放节点执行结果
//
// 返回录制的输出、下一个节点和错误。
// 对于同一节点的多次调用，会按录制顺序依次返回。
func (r *GraphReplayer) ReplayNode(nodeID string) (output map[string]any, nextNode string, err error) {
	r.mu.Lock()
	visitIndex := r.nodeVisits[nodeID]
	r.nodeVisits[nodeID]++
	r.mu.Unlock()

	interactions := r.cassette.FindByNode(nodeID)
	if visitIndex >= len(interactions) {
		return nil, "", fmt.Errorf("节点 %s 没有更多的录制数据（已访问 %d 次）", nodeID, visitIndex+1)
	}

	interaction := interactions[visitIndex]
	if interaction.Error != "" {
		return interaction.Output, interaction.NextNode, fmt.Errorf("%s", interaction.Error)
	}

	return interaction.Output, interaction.NextNode, nil
}

// Reset 重置回放状态
func (r *GraphReplayer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodeVisits = make(map[string]int)
}
