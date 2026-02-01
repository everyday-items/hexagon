package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/hooks"
	"github.com/everyday-items/hexagon/internal/util"
)

const defaultReActPrompt = `You are a helpful AI assistant with access to tools.

When you need to use a tool, respond with a tool call. After receiving the tool result, analyze it and continue reasoning.

Think step by step:
1. Understand the user's question
2. Decide if you need to use any tools
3. If yes, call the appropriate tool
4. Analyze the tool result
5. Continue until you have enough information to answer
6. Provide a final answer

Always be helpful, accurate, and concise.`

// ReActAgent 实现 ReAct (Reasoning + Acting) 模式的 Agent
// ReAct 模式让 Agent 交替进行推理和行动，直到完成任务
type ReActAgent struct {
	*BaseAgent
}

// NewReAct 创建 ReAct Agent
func NewReAct(opts ...Option) *ReActAgent {
	base := NewBaseAgent(opts...)

	// 设置默认系统提示词
	if base.config.SystemPrompt == "" {
		base.config.SystemPrompt = defaultReActPrompt
	}

	// 设置默认名称
	if base.config.Name == "Agent" {
		base.config.Name = "ReActAgent"
	}

	return &ReActAgent{BaseAgent: base}
}

// Run 执行 ReAct Agent
func (a *ReActAgent) Run(ctx context.Context, input Input) (Output, error) {
	if a.config.LLM == nil {
		return Output{}, fmt.Errorf("LLM provider not configured")
	}

	// 生成运行 ID
	runID := util.GenerateID("run")
	startTime := time.Now()

	// 获取钩子管理器
	hookManager := hooks.ManagerFromContext(ctx)

	// 触发运行开始钩子
	if hookManager != nil {
		if err := hookManager.TriggerRunStart(ctx, &hooks.RunStartEvent{
			RunID:   runID,
			AgentID: a.ID(),
			Input:   input,
		}); err != nil {
			return Output{}, fmt.Errorf("run start hook failed: %w", err)
		}
	}

	// 构建消息历史
	messages := a.buildInitialMessages(input)

	// 构建工具定义
	toolDefs := a.buildToolDefinitions()

	var output Output
	var totalUsage llm.Usage
	var runErr error

	// ReAct 循环
	for i := 0; i < a.config.MaxIterations; i++ {
		// 调用 LLM
		req := llm.CompletionRequest{
			Messages: messages,
			Tools:    toolDefs,
		}

		// 触发 LLM 开始钩子
		if hookManager != nil {
			hookManager.TriggerLLMStart(ctx, &hooks.LLMStartEvent{
				RunID:    runID,
				Provider: a.config.LLM.Name(),
				Messages: convertMessagesToAny(messages),
			})
		}

		llmStartTime := time.Now()
		resp, err := a.config.LLM.Complete(ctx, req)
		llmDuration := time.Since(llmStartTime).Milliseconds()

		if err != nil {
			runErr = fmt.Errorf("LLM call failed: %w", err)
			// 触发错误钩子
			if hookManager != nil {
				hookManager.TriggerError(ctx, &hooks.ErrorEvent{
					RunID:   runID,
					AgentID: a.ID(),
					Error:   runErr,
					Phase:   "llm_call",
				})
			}
			break
		}

		// 触发 LLM 结束钩子
		if hookManager != nil {
			hookManager.TriggerLLMEnd(ctx, &hooks.LLMEndEvent{
				RunID:            runID,
				Response:         resp.Content,
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				Duration:         llmDuration,
			})
		}

		// 累计 Token 使用
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		// 检查是否需要调用工具
		if len(resp.ToolCalls) == 0 {
			// 没有工具调用，返回最终结果
			output.Content = resp.Content
			output.Usage = totalUsage
			break
		}

		// 处理工具调用（带钩子）
		toolResults, toolRecords, err := a.executeToolCallsWithHooks(ctx, runID, resp.ToolCalls, hookManager)
		if err != nil {
			runErr = fmt.Errorf("tool execution failed: %w", err)
			if hookManager != nil {
				hookManager.TriggerError(ctx, &hooks.ErrorEvent{
					RunID:   runID,
					AgentID: a.ID(),
					Error:   runErr,
					Phase:   "tool_execution",
				})
			}
			break
		}

		output.ToolCalls = append(output.ToolCalls, toolRecords...)

		// 将工具调用和结果添加到消息历史
		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

		for _, result := range toolResults {
			messages = append(messages, llm.Message{
				Role:    llm.RoleTool,
				Content: result,
			})
		}
	}

	// 触发运行结束钩子
	if hookManager != nil {
		if runErr != nil {
			hookManager.TriggerError(ctx, &hooks.ErrorEvent{
				RunID:   runID,
				AgentID: a.ID(),
				Error:   runErr,
				Phase:   "run",
			})
		} else {
			hookManager.TriggerRunEnd(ctx, &hooks.RunEndEvent{
				RunID:    runID,
				AgentID:  a.ID(),
				Output:   output,
				Duration: time.Since(startTime).Milliseconds(),
			})
		}
	}

	if runErr != nil {
		return Output{}, runErr
	}

	// 保存到记忆
	if a.config.Memory != nil {
		a.saveToMemory(ctx, input, output)
	}

	return output, nil
}

// executeToolCallsWithHooks 执行工具调用（带钩子）
func (a *ReActAgent) executeToolCallsWithHooks(ctx context.Context, runID string, calls []llm.ToolCall, hookManager *hooks.Manager) ([]string, []ToolCallRecord, error) {
	results := make([]string, 0, len(calls))
	records := make([]ToolCallRecord, 0, len(calls))

	for _, call := range calls {
		// 查找工具
		var targetTool tool.Tool
		for _, t := range a.config.Tools {
			if t.Name() == call.Name {
				targetTool = t
				break
			}
		}

		if targetTool == nil {
			result := fmt.Sprintf("Error: tool '%s' not found", call.Name)
			results = append(results, result)
			continue
		}

		// 解析参数
		args, err := tool.ParseArgs(call.Arguments)
		if err != nil {
			result := fmt.Sprintf("Error: failed to parse arguments: %v", err)
			results = append(results, result)
			continue
		}

		toolID := util.GenerateID("tool")

		// 触发工具开始钩子
		if hookManager != nil {
			hookManager.TriggerToolStart(ctx, &hooks.ToolStartEvent{
				RunID:    runID,
				ToolName: call.Name,
				ToolID:   toolID,
				Input:    args,
			})
		}

		// 执行工具
		toolStartTime := time.Now()
		toolResult, err := targetTool.Execute(ctx, args)
		toolDuration := time.Since(toolStartTime).Milliseconds()

		// 触发工具结束钩子
		if hookManager != nil {
			hookManager.TriggerToolEnd(ctx, &hooks.ToolEndEvent{
				RunID:    runID,
				ToolName: call.Name,
				ToolID:   toolID,
				Output:   toolResult,
				Duration: toolDuration,
				Error:    err,
			})
		}

		if err != nil {
			result := fmt.Sprintf("Error: tool execution failed: %v", err)
			results = append(results, result)
			continue
		}

		// 记录工具调用
		records = append(records, ToolCallRecord{
			Name:      call.Name,
			Arguments: args,
			Result:    toolResult,
		})

		// 格式化结果
		resultStr := formatToolResult(toolResult)
		results = append(results, resultStr)
	}

	return results, records, nil
}

// convertMessagesToAny 将消息列表转换为 any 切片
func convertMessagesToAny(messages []llm.Message) []any {
	result := make([]any, len(messages))
	for i, m := range messages {
		result[i] = m
	}
	return result
}

// buildInitialMessages 构建初始消息
func (a *ReActAgent) buildInitialMessages(input Input) []llm.Message {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: a.config.SystemPrompt},
	}

	// 从记忆中获取历史上下文
	if a.config.Memory != nil {
		entries, _ := a.config.Memory.Search(context.Background(), memory.SearchQuery{
			Limit:     10,
			OrderDesc: true,
		})
		// 按时间正序添加
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			messages = append(messages, llm.Message{
				Role:    llm.Role(entry.Role),
				Content: entry.Content,
			})
		}
	}

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: input.Query,
	})

	return messages
}

// buildToolDefinitions 构建工具定义
func (a *ReActAgent) buildToolDefinitions() []llm.ToolDefinition {
	if len(a.config.Tools) == 0 {
		return nil
	}

	defs := make([]llm.ToolDefinition, len(a.config.Tools))
	for i, t := range a.config.Tools {
		defs[i] = llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		}
	}
	return defs
}


// formatToolResult 格式化工具结果
func formatToolResult(result tool.Result) string {
	if !result.Success {
		return fmt.Sprintf("Error: %s", result.Error)
	}

	switch v := result.Output.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// saveToMemory 保存到记忆
func (a *ReActAgent) saveToMemory(ctx context.Context, input Input, output Output) {
	// 保存用户输入
	a.config.Memory.Save(ctx, memory.Entry{
		Role:    "user",
		Content: input.Query,
	})

	// 保存工具调用记录
	if len(output.ToolCalls) > 0 {
		var toolSummary strings.Builder
		for _, tc := range output.ToolCalls {
			toolSummary.WriteString(fmt.Sprintf("Called %s: %s\n", tc.Name, tc.Result.String()))
		}
		a.config.Memory.Save(ctx, memory.Entry{
			Role:    "tool",
			Content: toolSummary.String(),
		})
	}

	// 保存 Agent 回复
	a.config.Memory.Save(ctx, memory.Entry{
		Role:    "assistant",
		Content: output.Content,
	})
}

// 确保实现了 Agent 接口
var _ Agent = (*ReActAgent)(nil)
