package devui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/orchestration/graph"
	"github.com/everyday-items/toolkit/util/idgen"
)

// BuilderExecutor 可视化构建器的图执行引擎
//
// 将 GraphDefinition 转换为 graph.Graph[graph.MapState] 并执行。
// MVP 阶段节点使用 echo handler（记录执行信息到状态），
// 后续可扩展为真正调用 Agent/Tool/LLM。
type BuilderExecutor struct {
	// collector 事件收集器，用于发送执行过程的 SSE 事件
	collector *Collector
}

// NewBuilderExecutor 创建执行器
func NewBuilderExecutor(collector *Collector) *BuilderExecutor {
	return &BuilderExecutor{collector: collector}
}

// ExecutionResult 图执行结果
type ExecutionResult struct {
	// RunID 执行 ID
	RunID string `json:"run_id"`

	// GraphID 图定义 ID
	GraphID string `json:"graph_id"`

	// Status 执行状态: completed / failed
	Status string `json:"status"`

	// FinalState 最终状态
	FinalState map[string]any `json:"final_state"`

	// NodeResults 各节点的执行结果
	NodeResults []NodeResult `json:"node_results"`

	// DurationMs 总执行耗时（毫秒）
	DurationMs int64 `json:"duration_ms"`

	// Error 错误信息（如有）
	Error string `json:"error,omitempty"`
}

// NodeResult 单个节点的执行结果
type NodeResult struct {
	// NodeID 节点 ID
	NodeID string `json:"node_id"`

	// NodeName 节点名称
	NodeName string `json:"node_name"`

	// NodeType 节点类型
	NodeType string `json:"node_type"`

	// Status 执行状态: completed / skipped / failed
	Status string `json:"status"`

	// DurationMs 执行耗时（毫秒）
	DurationMs int64 `json:"duration_ms"`

	// Output 节点输出
	Output map[string]any `json:"output,omitempty"`
}

// nodeTracker 跟踪节点执行结果（线程安全）
type nodeTracker struct {
	mu      sync.Mutex
	results []NodeResult
}

func (t *nodeTracker) add(nr NodeResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.results = append(t.results, nr)
}

func (t *nodeTracker) getResults() []NodeResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]NodeResult, len(t.results))
	copy(out, t.results)
	return out
}

// Execute 执行图定义
//
// 将 GraphDefinition 构建为 graph.Graph[graph.MapState] 并使用同步 Run() 执行。
// 节点 handler 内部记录执行结果并发送 SSE 事件，避免 Stream() 的并发竞态。
func (e *BuilderExecutor) Execute(ctx context.Context, def *GraphDefinition, initialState map[string]any) (*ExecutionResult, error) {
	runID := "run-" + idgen.ShortID()
	startTime := time.Now()

	// 发送图开始事件
	e.collector.EmitGraphStart(runID, def.ID, def.Name, initialState)

	// 创建结果跟踪器
	tracker := &nodeTracker{}

	// 构建图
	g, err := e.buildGraph(def, runID, tracker)
	if err != nil {
		e.collector.EmitError(runID, "builder", "构建图失败: "+err.Error(), "")
		return &ExecutionResult{
			RunID:      runID,
			GraphID:    def.ID,
			Status:     "failed",
			DurationMs: time.Since(startTime).Milliseconds(),
			Error:      err.Error(),
		}, nil
	}

	// 准备初始状态
	state := graph.MapState{}
	for k, v := range initialState {
		state[k] = v
	}
	// 注入运行元数据
	state["__run_id"] = runID
	state["__graph_id"] = def.ID
	state["__graph_name"] = def.Name

	// 使用同步 Run() 执行，避免 Stream() 的状态并发问题
	finalState, err := g.Run(ctx, state)
	if err != nil {
		totalDuration := time.Since(startTime).Milliseconds()
		e.collector.EmitError(runID, "builder", "执行图失败: "+err.Error(), "")
		e.collector.EmitGraphEnd(runID, def.ID, map[string]any{"error": err.Error()}, totalDuration)
		return &ExecutionResult{
			RunID:       runID,
			GraphID:     def.ID,
			Status:      "failed",
			NodeResults: tracker.getResults(),
			DurationMs:  totalDuration,
			Error:       err.Error(),
		}, nil
	}

	totalDuration := time.Since(startTime).Milliseconds()
	nodeResults := tracker.getResults()

	// 发送图结束事件
	e.collector.EmitGraphEnd(runID, def.ID, map[string]any{"node_count": len(nodeResults)}, totalDuration)

	// 清理内部状态键
	resultState := make(map[string]any)
	for k, v := range finalState {
		// 过滤以 __ 开头的内部键
		if len(k) < 2 || k[:2] != "__" {
			resultState[k] = v
		}
	}

	return &ExecutionResult{
		RunID:       runID,
		GraphID:     def.ID,
		Status:      "completed",
		FinalState:  resultState,
		NodeResults: nodeResults,
		DurationMs:  totalDuration,
	}, nil
}

// buildGraph 从图定义构建 graph.Graph[graph.MapState]
//
// 将每个节点定义转换为对应的 NodeHandler[MapState]：
//   - start: 透传状态
//   - end: 透传状态
//   - agent/tool/llm: MVP 使用 echo handler
//   - condition: 根据 config.condition 做状态键判断
//   - parallel: 透传状态（MVP）
func (e *BuilderExecutor) buildGraph(def *GraphDefinition, runID string, tracker *nodeTracker) (*graph.Graph[graph.MapState], error) {
	builder := graph.NewGraph[graph.MapState](def.Name)

	// 构建节点映射，方便边处理时查找
	nodeMap := make(map[string]*GraphNodeDef, len(def.Nodes))
	for i := range def.Nodes {
		nd := &def.Nodes[i]
		nodeMap[nd.ID] = nd
	}

	// 添加节点（start/end 由 graph 包内部处理，跳过）
	for _, node := range def.Nodes {
		if node.Type == "start" || node.Type == "end" {
			continue
		}
		handler := e.createNodeHandler(node, runID, def.ID, tracker)
		builder.AddNode(node.ID, handler)
	}

	// 查找 start 和 end 节点
	var startNodeID, endNodeID string
	for _, node := range def.Nodes {
		if node.Type == "start" {
			startNodeID = node.ID
		}
		if node.Type == "end" {
			endNodeID = node.ID
		}
	}

	// 设置入口点：从 start 节点的出边确定实际入口
	entryNodeID := ""
	var finishNodeIDs []string

	for _, edge := range def.Edges {
		if edge.Source == startNodeID {
			entryNodeID = edge.Target
		}
		if edge.Target == endNodeID {
			finishNodeIDs = append(finishNodeIDs, edge.Source)
		}
	}

	if entryNodeID == "" {
		return nil, fmt.Errorf("无法确定入口节点：start 节点没有出边")
	}

	builder.SetEntryPoint(entryNodeID)

	// 添加边（排除 start/end 节点的边）
	for _, edge := range def.Edges {
		// 跳过与 start/end 节点相关的边（已通过 entry/finish 处理）
		if edge.Source == startNodeID || edge.Target == endNodeID {
			continue
		}
		builder.AddEdge(edge.Source, edge.Target)
	}

	// 设置结束点
	if len(finishNodeIDs) > 0 {
		builder.SetFinishPoint(finishNodeIDs...)
	}

	return builder.Build()
}

// createNodeHandler 为节点创建处理函数
//
// handler 内部直接记录执行结果到 tracker 并发送 SSE 事件，
// 避免通过 Stream channel 传递状态导致的并发竞态。
func (e *BuilderExecutor) createNodeHandler(node GraphNodeDef, runID, graphID string, tracker *nodeTracker) graph.NodeHandler[graph.MapState] {
	nodeID := node.ID
	nodeName := node.Name
	nodeType := node.Type
	nodeConfig := node.Config

	return func(ctx context.Context, state graph.MapState) (graph.MapState, error) {
		startTime := time.Now()

		// 发送节点开始事件
		e.collector.EmitGraphNode(runID, graphID, nodeID, nodeName,
			map[string]any{"type": nodeType, "status": "started"}, 0)

		output := map[string]any{
			"node_id":   nodeID,
			"node_name": nodeName,
			"node_type": nodeType,
			"status":    "executed",
			"timestamp": time.Now().Format(time.RFC3339),
		}

		switch nodeType {
		case "agent":
			// MVP: 记录 Agent 信息
			agentRef := ""
			systemPrompt := ""
			if nodeConfig != nil {
				if v, ok := nodeConfig["agent_ref"]; ok {
					agentRef = fmt.Sprintf("%v", v)
				}
				if v, ok := nodeConfig["system_prompt"]; ok {
					systemPrompt = fmt.Sprintf("%v", v)
				}
			}
			output["agent_ref"] = agentRef
			output["system_prompt_preview"] = truncate(systemPrompt, 100)
			output["message"] = fmt.Sprintf("Agent [%s] 已执行（MVP echo 模式）", nodeName)

		case "tool":
			// MVP: 记录工具调用信息
			toolName := ""
			if nodeConfig != nil {
				if v, ok := nodeConfig["tool_name"]; ok {
					toolName = fmt.Sprintf("%v", v)
				}
			}
			output["tool_name"] = toolName
			output["message"] = fmt.Sprintf("工具 [%s] 已执行（MVP echo 模式）", toolName)

		case "llm":
			// MVP: 记录 LLM 调用信息
			provider := ""
			model := ""
			if nodeConfig != nil {
				if v, ok := nodeConfig["provider"]; ok {
					provider = fmt.Sprintf("%v", v)
				}
				if v, ok := nodeConfig["model"]; ok {
					model = fmt.Sprintf("%v", v)
				}
			}
			output["provider"] = provider
			output["model"] = model
			output["message"] = fmt.Sprintf("LLM [%s/%s] 已调用（MVP echo 模式）", provider, model)

		case "condition":
			// MVP: 简单条件判断（检查状态中的键值）
			conditionKey := ""
			if nodeConfig != nil {
				if v, ok := nodeConfig["condition"]; ok {
					conditionKey = fmt.Sprintf("%v", v)
				}
			}
			condValue, exists := state.Get(conditionKey)
			output["condition_key"] = conditionKey
			output["condition_value"] = condValue
			output["condition_exists"] = exists
			output["message"] = fmt.Sprintf("条件检查: %s = %v", conditionKey, condValue)

		case "parallel":
			output["message"] = fmt.Sprintf("并行节点 [%s] 已执行（MVP echo 模式）", nodeName)

		default:
			output["message"] = fmt.Sprintf("节点 [%s] 已执行", nodeName)
		}

		// 将输出存入状态
		state.Set("__output_"+nodeID, output)
		state.Set("__last_node", nodeID)

		durationMs := time.Since(startTime).Milliseconds()

		// 记录节点执行结果
		tracker.add(NodeResult{
			NodeID:     nodeID,
			NodeName:   nodeName,
			NodeType:   nodeType,
			Status:     "completed",
			DurationMs: durationMs,
			Output:     output,
		})

		// 发送节点完成事件
		e.collector.EmitGraphNode(runID, graphID, nodeID, nodeName,
			map[string]any{"type": nodeType, "status": "completed"}, durationMs)

		return state, nil
	}
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
