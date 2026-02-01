package graph

import (
	"context"
	"fmt"
	"time"
)

// CompiledGraph 编译后的图
// 提供更高效的执行和更多运行时特性
type CompiledGraph[S State] struct {
	*Graph[S]

	// ExecutionPlan 执行计划
	ExecutionPlan *ExecutionPlan

	// Stats 执行统计
	Stats *ExecutionStats
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
	// TopologicalOrder 拓扑排序后的节点顺序
	TopologicalOrder []string

	// ParallelGroups 可并行执行的节点组
	ParallelGroups [][]string

	// CriticalPath 关键路径
	CriticalPath []string

	// Dependencies 节点依赖关系
	Dependencies map[string][]string
}

// ExecutionStats 执行统计
type ExecutionStats struct {
	// TotalExecutions 总执行次数
	TotalExecutions int64

	// TotalDuration 总执行时间
	TotalDuration time.Duration

	// NodeStats 节点统计
	NodeStats map[string]*NodeStats

	// LastExecution 最后一次执行时间
	LastExecution time.Time
}

// NodeStats 节点统计
type NodeStats struct {
	// Executions 执行次数
	Executions int64

	// TotalDuration 总执行时间
	TotalDuration time.Duration

	// AverageDuration 平均执行时间
	AverageDuration time.Duration

	// Errors 错误次数
	Errors int64

	// LastError 最后一次错误
	LastError error

	// LastExecution 最后一次执行时间
	LastExecution time.Time
}

// Compile 编译图
func Compile[S State](g *Graph[S]) (*CompiledGraph[S], error) {
	if !g.compiled {
		if err := g.compile(); err != nil {
			return nil, err
		}
	}

	// 计算执行计划
	plan, err := computeExecutionPlan(g)
	if err != nil {
		return nil, err
	}

	return &CompiledGraph[S]{
		Graph:         g,
		ExecutionPlan: plan,
		Stats: &ExecutionStats{
			NodeStats: make(map[string]*NodeStats),
		},
	}, nil
}

// computeExecutionPlan 计算执行计划
func computeExecutionPlan[S State](g *Graph[S]) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{
		Dependencies: make(map[string][]string),
	}

	// 计算节点依赖
	for _, edge := range g.Edges {
		plan.Dependencies[edge.To] = append(plan.Dependencies[edge.To], edge.From)
	}

	// 拓扑排序
	order, err := topologicalSort(g.Nodes, plan.Dependencies)
	if err != nil {
		return nil, err
	}
	plan.TopologicalOrder = order

	// 计算可并行执行的组
	plan.ParallelGroups = computeParallelGroups(order, plan.Dependencies)

	// 计算关键路径（简化版：最长路径）
	plan.CriticalPath = computeCriticalPath(g, plan.Dependencies)

	return plan, nil
}

// topologicalSort 拓扑排序
func topologicalSort[S State](nodes map[string]*Node[S], deps map[string][]string) ([]string, error) {
	// 计算入度
	inDegree := make(map[string]int)
	for name := range nodes {
		inDegree[name] = 0
	}
	for _, dependencies := range deps {
		for _, dep := range dependencies {
			if _, exists := nodes[dep]; exists {
				inDegree[dep]++
			}
		}
	}

	// 找到所有入度为 0 的节点
	queue := make([]string, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Kahn 算法
	result := make([]string, 0, len(nodes))
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// 减少后继节点的入度
		for successor, dependencies := range deps {
			for _, dep := range dependencies {
				if dep == node {
					inDegree[successor]--
					if inDegree[successor] == 0 {
						queue = append(queue, successor)
					}
				}
			}
		}
	}

	if len(result) != len(nodes) {
		return nil, fmt.Errorf("graph contains a cycle")
	}

	return result, nil
}

// computeParallelGroups 计算可并行执行的节点组
func computeParallelGroups(order []string, deps map[string][]string) [][]string {
	if len(order) == 0 {
		return nil
	}

	// 计算每个节点的层级
	levels := make(map[string]int)
	maxLevel := 0

	for _, node := range order {
		level := 0
		for _, dep := range deps[node] {
			if depLevel, ok := levels[dep]; ok && depLevel >= level {
				level = depLevel + 1
			}
		}
		levels[node] = level
		if level > maxLevel {
			maxLevel = level
		}
	}

	// 按层级分组
	groups := make([][]string, maxLevel+1)
	for i := range groups {
		groups[i] = make([]string, 0)
	}

	for node, level := range levels {
		groups[level] = append(groups[level], node)
	}

	return groups
}

// computeCriticalPath 计算关键路径
func computeCriticalPath[S State](g *Graph[S], deps map[string][]string) []string {
	// 简化实现：找到从 START 到 END 的最长路径
	path := make([]string, 0)

	current := START
	visited := make(map[string]bool)

	for current != END {
		if visited[current] {
			break // 避免循环
		}
		visited[current] = true
		path = append(path, current)

		// 找下一个节点（简化：选择第一个）
		if targets, ok := g.adjacency[current]; ok && len(targets) > 0 {
			current = targets[0]
		} else {
			break
		}
	}

	if current == END {
		path = append(path, END)
	}

	return path
}

// Run 执行编译后的图
func (cg *CompiledGraph[S]) Run(ctx context.Context, initialState S, opts ...RunOption) (S, error) {
	startTime := time.Now()
	defer func() {
		cg.Stats.TotalExecutions++
		cg.Stats.TotalDuration += time.Since(startTime)
		cg.Stats.LastExecution = time.Now()
	}()

	return cg.Graph.Run(ctx, initialState, opts...)
}

// RunWithStats 执行并返回详细统计
func (cg *CompiledGraph[S]) RunWithStats(ctx context.Context, initialState S, opts ...RunOption) (S, *ExecutionResult, error) {
	result := &ExecutionResult{
		StartTime:  time.Now(),
		NodeTiming: make(map[string]time.Duration),
	}

	// 包装节点处理函数以收集统计
	originalNodes := make(map[string]NodeHandler[S])
	for name, node := range cg.Nodes {
		originalNodes[name] = node.Handler
		nodeName := name
		node.Handler = func(ctx context.Context, state S) (S, error) {
			nodeStart := time.Now()
			defer func() {
				result.NodeTiming[nodeName] = time.Since(nodeStart)
			}()
			return originalNodes[nodeName](ctx, state)
		}
	}

	// 执行
	finalState, err := cg.Graph.Run(ctx, initialState, opts...)

	// 恢复原始处理函数
	for name, handler := range originalNodes {
		cg.Nodes[name].Handler = handler
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Error = err

	return finalState, result, err
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	// StartTime 开始时间
	StartTime time.Time

	// EndTime 结束时间
	EndTime time.Time

	// Duration 总耗时
	Duration time.Duration

	// NodeTiming 每个节点的耗时
	NodeTiming map[string]time.Duration

	// Error 错误
	Error error
}

// Visualize 可视化图（返回 Mermaid 格式）
func (cg *CompiledGraph[S]) Visualize() string {
	var result string
	result += "graph TD\n"

	// 添加节点
	for name, node := range cg.Nodes {
		label := name
		switch node.Type {
		case NodeTypeStart:
			result += fmt.Sprintf("    %s((%s))\n", name, label)
		case NodeTypeEnd:
			result += fmt.Sprintf("    %s((%s))\n", name, label)
		case NodeTypeConditional:
			result += fmt.Sprintf("    %s{%s}\n", name, label)
		case NodeTypeParallel:
			result += fmt.Sprintf("    %s[[%s]]\n", name, label)
		default:
			result += fmt.Sprintf("    %s[%s]\n", name, label)
		}
	}

	// 添加边
	for _, edge := range cg.Edges {
		if edge.Label != "" {
			result += fmt.Sprintf("    %s -->|%s| %s\n", edge.From, edge.Label, edge.To)
		} else {
			result += fmt.Sprintf("    %s --> %s\n", edge.From, edge.To)
		}
	}

	return result
}
