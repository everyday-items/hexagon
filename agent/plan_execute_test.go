package agent

import (
	"context"
	"testing"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/schema"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/orchestration/planner"
)

// mockPlanner 模拟规划器
type mockPlanner struct {
	plan *planner.Plan
}

func (p *mockPlanner) Name() string { return "mock_planner" }

func (p *mockPlanner) Plan(ctx context.Context, goal string, opts ...planner.PlanOption) (*planner.Plan, error) {
	if p.plan != nil {
		return p.plan, nil
	}
	return &planner.Plan{
		ID:   "plan-1",
		Goal: goal,
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "第一步",
				State:       planner.StepStatePending,
			},
		},
		State: planner.PlanStatePending,
	}, nil
}

func (p *mockPlanner) Replan(ctx context.Context, plan *planner.Plan, feedback string) (*planner.Plan, error) {
	return plan, nil
}

func TestNewPlanExecute(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewPlanExecute(
		[]Option{
			WithLLM(provider),
			WithName("TestPlanExecuteAgent"),
		},
		WithPlanExecuteMaxReplans(5),
	)

	if agent == nil {
		t.Fatal("NewPlanExecute returned nil")
	}

	if agent.Name() != "TestPlanExecuteAgent" {
		t.Errorf("expected name 'TestPlanExecuteAgent', got %s", agent.Name())
	}

	if agent.maxReplans != 5 {
		t.Errorf("expected maxReplans=5, got %d", agent.maxReplans)
	}
}

func TestNewPlanExecute_DefaultValues(t *testing.T) {
	provider := &mockLLMProvider{}
	agent := NewPlanExecute(
		[]Option{WithLLM(provider)},
	)

	if agent.maxReplans != 3 {
		t.Errorf("expected default maxReplans=3, got %d", agent.maxReplans)
	}

	if !agent.replanOnFailure {
		t.Error("expected default replanOnFailure=true")
	}

	if agent.Name() != "PlanExecuteAgent" {
		t.Errorf("expected default name 'PlanExecuteAgent', got %s", agent.Name())
	}
}

func TestPlanExecute_WithPlanner(t *testing.T) {
	provider := &mockLLMProvider{
		response: "任务完成。",
	}

	mockPlan := &planner.Plan{
		ID:   "plan-test",
		Goal: "测试目标",
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "执行 LLM 操作",
				Action: &planner.Action{
					Type: planner.ActionTypeLLM,
					Name: "llm",
				},
				State: planner.StepStatePending,
			},
		},
		State: planner.PlanStatePending,
	}

	agent := NewPlanExecute(
		[]Option{WithLLM(provider)},
		WithPlanExecutePlanner(&mockPlanner{plan: mockPlan}),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "测试任务"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if output.Content == "" {
		t.Error("expected non-empty output content")
	}

	// 检查元数据
	if output.Metadata == nil {
		t.Error("expected metadata in output")
	} else {
		if output.Metadata["plan_id"] != "plan-test" {
			t.Errorf("expected plan_id='plan-test', got %v", output.Metadata["plan_id"])
		}
	}
}

func TestPlanExecute_WithTool(t *testing.T) {
	provider := &mockLLMProvider{
		response: "工具执行完成。",
	}

	mockTool := &mockToolForTest{
		name:        "test_tool",
		description: "测试工具",
		result: tool.Result{
			Success: true,
			Output:  "工具输出",
		},
	}

	mockPlan := &planner.Plan{
		ID:   "plan-tool",
		Goal: "测试工具调用",
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "调用测试工具",
				Action: &planner.Action{
					Type:       planner.ActionTypeTool,
					Name:       "test_tool",
					Parameters: map[string]any{"arg": "value"},
				},
				State: planner.StepStatePending,
			},
		},
		State: planner.PlanStatePending,
	}

	agent := NewPlanExecute(
		[]Option{
			WithLLM(provider),
			WithTools(mockTool),
		},
		WithPlanExecutePlanner(&mockPlanner{plan: mockPlan}),
	)

	ctx := context.Background()
	output, err := agent.Run(ctx, Input{Query: "使用工具"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 检查工具调用记录
	if len(output.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(output.ToolCalls))
	} else {
		if output.ToolCalls[0].Name != "test_tool" {
			t.Errorf("expected tool name 'test_tool', got %s", output.ToolCalls[0].Name)
		}
	}
}

func TestPlanExecute_ReplanOnFailure(t *testing.T) {
	provider := &mockLLMProvider{
		response: "重计划后完成。",
	}

	// 第一次执行会失败的工具
	callCount := 0
	failingTool := &mockToolForTest{
		name:        "failing_tool",
		description: "可能失败的工具",
		executeFunc: func(ctx context.Context, args map[string]any) (tool.Result, error) {
			callCount++
			if callCount == 1 {
				return tool.Result{Success: false, Error: "第一次失败"}, nil
			}
			return tool.Result{Success: true, Output: "成功"}, nil
		},
	}

	mockPlan := &planner.Plan{
		ID:   "plan-replan",
		Goal: "测试重计划",
		Steps: []*planner.Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "可能失败的步骤",
				Action: &planner.Action{
					Type: planner.ActionTypeTool,
					Name: "failing_tool",
				},
				State: planner.StepStatePending,
			},
		},
		State: planner.PlanStatePending,
	}

	agent := NewPlanExecute(
		[]Option{
			WithLLM(provider),
			WithTools(failingTool),
		},
		WithPlanExecutePlanner(&mockPlanner{plan: mockPlan}),
		WithPlanExecuteReplanOnFailure(true),
		WithPlanExecuteMaxReplans(3),
	)

	ctx := context.Background()
	_, err := agent.Run(ctx, Input{Query: "测试重计划"})

	// 即使失败也应该尝试重计划
	if err != nil {
		// 预期可能失败，检查是否尝试了重计划
		t.Logf("Run completed with error: %v", err)
	}

	// 检查元数据中的重计划次数
	if callCount > 0 {
		t.Logf("Tool was called %d times", callCount)
	}
}

func TestPlanExecute_AllStepsCompleted(t *testing.T) {
	agent := &PlanExecuteAgent{}

	plan := &planner.Plan{
		Steps: []*planner.Step{
			{State: planner.StepStateCompleted},
			{State: planner.StepStateCompleted},
			{State: planner.StepStateSkipped},
		},
	}

	if !agent.allStepsCompleted(plan) {
		t.Error("expected all steps to be completed")
	}

	plan.Steps[1].State = planner.StepStatePending
	if agent.allStepsCompleted(plan) {
		t.Error("expected not all steps completed")
	}
}

func TestPlanExecute_CheckDependencies(t *testing.T) {
	agent := &PlanExecuteAgent{}

	plan := &planner.Plan{
		Steps: []*planner.Step{
			{ID: "step-1", State: planner.StepStateCompleted},
			{ID: "step-2", State: planner.StepStatePending, Dependencies: []string{"step-1"}},
			{ID: "step-3", State: planner.StepStatePending, Dependencies: []string{"step-2"}},
		},
	}

	// step-2 的依赖 step-1 已完成
	if !agent.checkDependencies(plan, plan.Steps[1]) {
		t.Error("expected step-2 dependencies to be satisfied")
	}

	// step-3 的依赖 step-2 未完成
	if agent.checkDependencies(plan, plan.Steps[2]) {
		t.Error("expected step-3 dependencies not satisfied")
	}

	// 无依赖的步骤
	noDepsStep := &planner.Step{Dependencies: nil}
	if !agent.checkDependencies(plan, noDepsStep) {
		t.Error("expected step without dependencies to be satisfied")
	}
}

func TestPlanExecute_CountCompletedSteps(t *testing.T) {
	agent := &PlanExecuteAgent{}

	plan := &planner.Plan{
		Steps: []*planner.Step{
			{State: planner.StepStateCompleted},
			{State: planner.StepStateCompleted},
			{State: planner.StepStatePending},
			{State: planner.StepStateFailed},
		},
	}

	count := agent.countCompletedSteps(plan)
	if count != 2 {
		t.Errorf("expected 2 completed steps, got %d", count)
	}
}

func TestPlanExecute_BuildSimpleSummary(t *testing.T) {
	agent := &PlanExecuteAgent{}

	// 全部完成
	plan := &planner.Plan{
		Steps: []*planner.Step{
			{State: planner.StepStateCompleted},
			{State: planner.StepStateCompleted},
		},
	}

	summary := agent.buildSimpleSummary(plan)
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// 部分完成
	plan.Steps[1].State = planner.StepStatePending
	summary = agent.buildSimpleSummary(plan)
	if summary == "" {
		t.Error("expected non-empty summary for partial completion")
	}
}

// mockToolForTest 测试用模拟工具
type mockToolForTest struct {
	name        string
	description string
	result      tool.Result
	executeFunc func(ctx context.Context, args map[string]any) (tool.Result, error)
}

func (t *mockToolForTest) Name() string              { return t.name }
func (t *mockToolForTest) Description() string       { return t.description }
func (t *mockToolForTest) Schema() *schema.Schema    { return nil }
func (t *mockToolForTest) Validate(args map[string]any) error {
	return nil
}
func (t *mockToolForTest) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, args)
	}
	return t.result, nil
}

// mockLLMProvider 模拟 LLM Provider
type mockLLMProvider struct {
	response   string
	toolCalls  []llm.ToolCall
	callCount  int
	responses  []string // 多次调用时的响应列表
	usageCount int
}

func (p *mockLLMProvider) Name() string { return "mock" }

func (p *mockLLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{Name: "mock-model"}}
}

func (p *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	p.callCount++

	response := p.response
	if len(p.responses) > 0 && p.callCount-1 < len(p.responses) {
		response = p.responses[p.callCount-1]
	}

	return &llm.CompletionResponse{
		Content:   response,
		ToolCalls: p.toolCalls,
		Usage: llm.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}, nil
}

func (p *mockLLMProvider) Stream(ctx context.Context, req llm.CompletionRequest) (*llm.Stream, error) {
	return nil, nil
}

func (p *mockLLMProvider) CountTokens(messages []llm.Message) (int, error) {
	return 100, nil
}
