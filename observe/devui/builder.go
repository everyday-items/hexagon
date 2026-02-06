package devui

import (
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/toolkit/util/idgen"
)

// GraphDefinition 可视化图定义
//
// 存储构建器画布上的图定义数据，包括节点、边和元数据。
// 该结构用于前后端之间的图定义序列化和持久化。
type GraphDefinition struct {
	// ID 图定义的唯一标识
	ID string `json:"id"`

	// Name 图名称
	Name string `json:"name"`

	// Description 图描述（可选）
	Description string `json:"description,omitempty"`

	// Version 版本号，每次更新自增
	Version int `json:"version"`

	// Nodes 节点列表
	Nodes []GraphNodeDef `json:"nodes"`

	// Edges 边列表
	Edges []GraphEdgeDef `json:"edges"`

	// EntryPoint 入口节点 ID
	EntryPoint string `json:"entry_point"`

	// Metadata 元数据（可选）
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt 最后更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// GraphNodeDef 图节点定义
//
// 描述画布上的一个节点，包含节点类型、位置和配置。
// 支持的节点类型：
//   - start: 开始节点
//   - end: 结束节点
//   - agent: Agent 节点
//   - tool: 工具节点
//   - condition: 条件分支节点
//   - parallel: 并行节点
//   - llm: LLM 调用节点
type GraphNodeDef struct {
	// ID 节点唯一标识
	ID string `json:"id"`

	// Name 节点显示名称
	Name string `json:"name"`

	// Type 节点类型
	Type string `json:"type"`

	// Position 画布上的坐标位置
	Position Position `json:"position"`

	// Description 节点描述（可选）
	Description string `json:"description,omitempty"`

	// Config 节点配置（可选，按类型不同配置不同）
	Config map[string]any `json:"config,omitempty"`
}

// GraphEdgeDef 图边定义
//
// 描述两个节点之间的连接关系，支持条件路由。
type GraphEdgeDef struct {
	// ID 边的唯一标识
	ID string `json:"id"`

	// Source 源节点 ID
	Source string `json:"source"`

	// Target 目标节点 ID
	Target string `json:"target"`

	// Label 边的标签（可选，用于条件路由显示）
	Label string `json:"label,omitempty"`

	// Condition 条件表达式（可选，用于条件路由）
	Condition string `json:"condition,omitempty"`
}

// Position 画布坐标
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeTypeInfo 节点类型信息
//
// 描述可用的节点类型，供前端节点面板使用。
type NodeTypeInfo struct {
	// Type 节点类型标识
	Type string `json:"type"`

	// Name 显示名称
	Name string `json:"name"`

	// Description 类型描述
	Description string `json:"description"`

	// Icon 图标（emoji）
	Icon string `json:"icon"`

	// Color 主题颜色
	Color string `json:"color"`

	// Category 分类
	Category string `json:"category"`
}

// validationResult 图验证结果
type validationResult struct {
	// Valid 是否验证通过
	Valid bool `json:"valid"`

	// Errors 验证错误列表
	Errors []string `json:"errors"`
}

// defaultNodeTypes 默认可用的节点类型列表
var defaultNodeTypes = []NodeTypeInfo{
	{Type: "start", Name: "开始", Description: "流程开始节点，每个图必须有一个", Icon: "▶️", Color: "#58a6ff", Category: "control"},
	{Type: "end", Name: "结束", Description: "流程结束节点，每个图必须有一个", Icon: "⏹️", Color: "#58a6ff", Category: "control"},
	{Type: "agent", Name: "Agent", Description: "AI Agent 节点，执行智能任务", Icon: "🤖", Color: "#a855f7", Category: "ai"},
	{Type: "tool", Name: "工具", Description: "工具调用节点，执行具体操作", Icon: "🔧", Color: "#3fb950", Category: "action"},
	{Type: "llm", Name: "LLM", Description: "LLM 调用节点，调用大语言模型", Icon: "🧠", Color: "#f97316", Category: "ai"},
	{Type: "condition", Name: "条件", Description: "条件分支节点，根据条件路由", Icon: "🔀", Color: "#d29922", Category: "control"},
	{Type: "parallel", Name: "并行", Description: "并行执行节点，同时运行多个分支", Icon: "⚡", Color: "#f85149", Category: "control"},
}

// GraphStore 图定义的内存存储
//
// 线程安全的内存存储，使用 sync.RWMutex 保护并发访问。
// 适用于 MVP 阶段，后续可替换为持久化存储。
type GraphStore struct {
	// graphs 图定义映射表，key 为图 ID
	graphs map[string]*GraphDefinition

	// mu 读写锁，保护 graphs 的并发访问
	mu sync.RWMutex
}

// NewGraphStore 创建图定义存储
func NewGraphStore() *GraphStore {
	return &GraphStore{
		graphs: make(map[string]*GraphDefinition),
	}
}

// Create 创建新的图定义
//
// 自动生成 ID、设置版本号和时间戳。
// 返回创建后的图定义（包含生成的 ID）。
func (s *GraphStore) Create(def *GraphDefinition) *GraphDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	def.ID = idgen.ShortID()
	def.Version = 1
	def.CreatedAt = now
	def.UpdatedAt = now

	// 确保 Nodes 和 Edges 不为 nil
	if def.Nodes == nil {
		def.Nodes = []GraphNodeDef{}
	}
	if def.Edges == nil {
		def.Edges = []GraphEdgeDef{}
	}

	s.graphs[def.ID] = def
	return def
}

// Get 根据 ID 获取图定义
//
// 如果未找到返回 nil 和错误。
func (s *GraphStore) Get(id string) (*GraphDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	def, ok := s.graphs[id]
	if !ok {
		return nil, fmt.Errorf("图定义不存在: %s", id)
	}
	return def, nil
}

// List 列出所有图定义
//
// 返回所有图定义的切片，按创建时间倒序排列。
func (s *GraphStore) List() []*GraphDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*GraphDefinition, 0, len(s.graphs))
	for _, def := range s.graphs {
		result = append(result, def)
	}
	return result
}

// Update 更新图定义
//
// 自增版本号并更新时间戳。
// 如果图不存在返回错误。
func (s *GraphStore) Update(id string, def *GraphDefinition) (*GraphDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.graphs[id]
	if !ok {
		return nil, fmt.Errorf("图定义不存在: %s", id)
	}

	// 保留原始 ID 和创建时间，自增版本
	def.ID = id
	def.CreatedAt = existing.CreatedAt
	def.Version = existing.Version + 1
	def.UpdatedAt = time.Now()

	// 确保 Nodes 和 Edges 不为 nil
	if def.Nodes == nil {
		def.Nodes = []GraphNodeDef{}
	}
	if def.Edges == nil {
		def.Edges = []GraphEdgeDef{}
	}

	s.graphs[id] = def
	return def, nil
}

// Delete 删除图定义
//
// 如果图不存在返回错误。
func (s *GraphStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.graphs[id]; !ok {
		return fmt.Errorf("图定义不存在: %s", id)
	}
	delete(s.graphs, id)
	return nil
}

// Validate 验证图定义
//
// 执行以下校验：
//   - 必须有 start 节点
//   - 必须有 end 节点
//   - 所有节点必须有 name
//   - 边引用的节点必须存在
//   - BFS 从入口点检查可达性
//   - 检查孤立节点
func (s *GraphStore) Validate(def *GraphDefinition) *validationResult {
	result := &validationResult{Valid: true, Errors: []string{}}

	if len(def.Nodes) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "图中没有节点")
		return result
	}

	// 构建节点 ID 集合和类型统计
	nodeIDs := make(map[string]bool, len(def.Nodes))
	typeCount := make(map[string]int)
	for _, node := range def.Nodes {
		nodeIDs[node.ID] = true
		typeCount[node.Type]++

		if node.Name == "" {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("节点 %s 缺少名称", node.ID))
		}
	}

	// 检查必须有 start 节点
	if typeCount["start"] == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "缺少开始节点 (start)")
	}
	if typeCount["start"] > 1 {
		result.Valid = false
		result.Errors = append(result.Errors, "开始节点 (start) 只能有一个")
	}

	// 检查必须有 end 节点
	if typeCount["end"] == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "缺少结束节点 (end)")
	}

	// 检查边引用的节点是否存在
	adjacency := make(map[string][]string)
	for _, edge := range def.Edges {
		if !nodeIDs[edge.Source] {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("边 %s 的源节点 %s 不存在", edge.ID, edge.Source))
		}
		if !nodeIDs[edge.Target] {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("边 %s 的目标节点 %s 不存在", edge.ID, edge.Target))
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}

	// BFS 可达性检查（从 entry_point 或 start 节点出发）
	entryPoint := def.EntryPoint
	if entryPoint == "" {
		// 自动查找 start 节点
		for _, node := range def.Nodes {
			if node.Type == "start" {
				entryPoint = node.ID
				break
			}
		}
	}

	if entryPoint != "" && nodeIDs[entryPoint] {
		visited := make(map[string]bool)
		queue := []string{entryPoint}
		visited[entryPoint] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, neighbor := range adjacency[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}

		// 检查孤立节点（不从入口点可达）
		for _, node := range def.Nodes {
			if !visited[node.ID] {
				result.Errors = append(result.Errors, fmt.Sprintf("节点 %s (%s) 从入口点不可达", node.Name, node.ID))
			}
		}
	}

	return result
}
