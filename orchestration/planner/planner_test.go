package planner

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/testing/mock"
)

func TestSequentialPlannerCreation(t *testing.T) {
	planner := NewSequentialPlanner()

	if planner.Name() != "sequential_planner" {
		t.Errorf("expected default name 'sequential_planner', got '%s'", planner.Name())
	}
}

func TestSequentialPlannerWithOptions(t *testing.T) {
	tools := []ToolInfo{
		{Name: "search", Description: "Search tool"},
		{Name: "calculator", Description: "Calculator tool"},
	}

	mockLLM := mock.FixedProvider("mock response")

	planner := NewSequentialPlanner(
		WithSequentialPlannerName("my-planner"),
		WithSequentialPlannerLLM(mockLLM),
		WithSequentialPlannerTools(tools...),
	)

	if planner.Name() != "my-planner" {
		t.Errorf("expected name 'my-planner', got '%s'", planner.Name())
	}
}

func TestSequentialPlannerPlanWithoutLLM(t *testing.T) {
	planner := NewSequentialPlanner()

	plan, err := planner.Plan(context.Background(), "Test goal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Goal != "Test goal" {
		t.Errorf("expected goal 'Test goal', got '%s'", plan.Goal)
	}

	if plan.State != PlanStatePending {
		t.Errorf("expected state 'pending', got '%s'", plan.State)
	}

	if len(plan.Steps) != 0 {
		t.Errorf("expected 0 steps without LLM, got %d", len(plan.Steps))
	}
}

func TestSequentialPlannerPlanWithLLM(t *testing.T) {
	// 模拟 LLM 返回有效的 JSON 计划
	mockLLM := mock.FixedProvider(`{
		"steps": [
			{
				"description": "搜索相关信息",
				"action": {
					"type": "tool",
					"name": "search",
					"parameters": {"query": "test"}
				},
				"dependencies": []
			},
			{
				"description": "分析结果",
				"action": {
					"type": "llm",
					"name": "analyze"
				},
				"dependencies": ["step-1"]
			}
		]
	}`)

	planner := NewSequentialPlanner(
		WithSequentialPlannerLLM(mockLLM),
		WithSequentialPlannerTools(ToolInfo{Name: "search", Description: "Search tool"}),
	)

	plan, err := planner.Plan(context.Background(), "Find information about testing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(plan.Steps))
	}

	if len(plan.Steps) > 0 && plan.Steps[0].Description != "搜索相关信息" {
		t.Errorf("unexpected step description: %s", plan.Steps[0].Description)
	}
}

func TestSequentialPlannerReplan(t *testing.T) {
	mockLLM := mock.FixedProvider(`{"steps": []}`)

	planner := NewSequentialPlanner(
		WithSequentialPlannerLLM(mockLLM),
	)

	plan := &Plan{
		ID:   "test-plan",
		Goal: "Test goal",
		Steps: []*Step{
			{
				ID:          "step-1",
				Description: "First step",
				State:       StepStateCompleted,
			},
		},
		UpdatedAt: time.Now(),
	}

	newPlan, err := planner.Replan(context.Background(), plan, "需要调整计划")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newPlan.ID != plan.ID {
		t.Error("expected same plan ID after replan")
	}
}

func TestSequentialPlannerPlanOptions(t *testing.T) {
	planner := NewSequentialPlanner()

	plan, err := planner.Plan(context.Background(), "Test goal",
		WithMaxSteps(5),
		WithPlanTimeout(10*time.Second),
		WithAvailableTools("tool1", "tool2"),
		WithPlanContext(map[string]any{"key": "value"}),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan == nil {
		t.Error("expected non-nil plan")
	}
}

func TestStepwisePlannerCreation(t *testing.T) {
	planner := NewStepwisePlanner()

	if planner.Name() != "stepwise_planner" {
		t.Errorf("expected default name 'stepwise_planner', got '%s'", planner.Name())
	}
}

func TestStepwisePlannerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("mock response")

	planner := NewStepwisePlanner(
		WithStepwisePlannerName("my-stepwise"),
		WithStepwisePlannerMaxSteps(30),
		WithStepwisePlannerLLM(mockLLM),
		WithStepwisePlannerTools(ToolInfo{Name: "test", Description: "Test tool"}),
	)

	if planner.Name() != "my-stepwise" {
		t.Errorf("expected name 'my-stepwise', got '%s'", planner.Name())
	}

	if planner.maxSteps != 30 {
		t.Errorf("expected maxSteps 30, got %d", planner.maxSteps)
	}
}

func TestStepwisePlannerPlan(t *testing.T) {
	planner := NewStepwisePlanner()

	plan, err := planner.Plan(context.Background(), "Stepwise goal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Goal != "Stepwise goal" {
		t.Errorf("expected goal 'Stepwise goal', got '%s'", plan.Goal)
	}

	// Stepwise planner 初始计划为空
	if len(plan.Steps) != 0 {
		t.Errorf("expected 0 initial steps, got %d", len(plan.Steps))
	}

	metadata, ok := plan.Metadata["type"]
	if !ok || metadata != "stepwise" {
		t.Error("expected metadata type 'stepwise'")
	}
}

func TestStepwisePlannerPlanNextStepWithoutLLM(t *testing.T) {
	planner := NewStepwisePlanner()

	plan := &Plan{
		ID:    "test-plan",
		Goal:  "Test goal",
		Steps: make([]*Step, 0),
	}

	step, err := planner.PlanNextStep(context.Background(), plan, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if step == nil {
		t.Fatal("expected non-nil step")
	}

	if step.ID != "step-1" {
		t.Errorf("expected step ID 'step-1', got '%s'", step.ID)
	}
}

func TestStepwisePlannerMaxStepsReached(t *testing.T) {
	planner := NewStepwisePlanner(
		WithStepwisePlannerMaxSteps(2),
	)

	plan := &Plan{
		ID:   "test-plan",
		Goal: "Test goal",
		Steps: []*Step{
			{ID: "step-1"},
			{ID: "step-2"},
		},
	}

	_, err := planner.PlanNextStep(context.Background(), plan, nil)
	if err == nil {
		t.Error("expected error when max steps reached")
	}
}

func TestStepwisePlannerReplan(t *testing.T) {
	planner := NewStepwisePlanner()

	plan := &Plan{
		ID:        "test-plan",
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	oldTime := plan.UpdatedAt

	newPlan, err := planner.Replan(context.Background(), plan, "feedback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !newPlan.UpdatedAt.After(oldTime) {
		t.Error("expected UpdatedAt to be updated")
	}
}

func TestActionPlannerCreation(t *testing.T) {
	actions := []ActionDefinition{
		{Name: "action1", Description: "Action 1"},
		{Name: "action2", Description: "Action 2"},
	}

	planner := NewActionPlanner(actions)

	if planner.Name() != "action_planner" {
		t.Errorf("expected default name 'action_planner', got '%s'", planner.Name())
	}
}

func TestActionPlannerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("mock response")

	planner := NewActionPlanner(
		[]ActionDefinition{{Name: "test", Description: "Test action"}},
		WithActionPlannerName("my-action"),
		WithActionPlannerLLM(mockLLM),
	)

	if planner.Name() != "my-action" {
		t.Errorf("expected name 'my-action', got '%s'", planner.Name())
	}
}

func TestActionPlannerPlanWithoutLLM(t *testing.T) {
	actions := []ActionDefinition{
		{Name: "action1", Description: "First action", Parameters: map[string]any{"key": "value"}},
		{Name: "action2", Description: "Second action"},
	}

	planner := NewActionPlanner(actions)

	plan, err := planner.Plan(context.Background(), "Select best action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 没有 LLM 时，选择第一个动作
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}

	if plan.Steps[0].Action.Name != "action1" {
		t.Errorf("expected action 'action1', got '%s'", plan.Steps[0].Action.Name)
	}
}

func TestActionPlannerPlanNoActions(t *testing.T) {
	planner := NewActionPlanner([]ActionDefinition{})

	plan, err := planner.Plan(context.Background(), "No actions available")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Steps) != 0 {
		t.Errorf("expected 0 steps with no actions, got %d", len(plan.Steps))
	}
}

func TestActionPlannerPlanWithLLM(t *testing.T) {
	mockLLM := mock.FixedProvider(`{
		"action": {
			"name": "action2",
			"parameters": {"param": "value"},
			"reason": "最适合目标"
		}
	}`)

	actions := []ActionDefinition{
		{Name: "action1", Description: "First action"},
		{Name: "action2", Description: "Second action"},
	}

	planner := NewActionPlanner(actions, WithActionPlannerLLM(mockLLM))

	plan, err := planner.Plan(context.Background(), "Select best action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}

	if plan.Steps[0].Action.Name != "action2" {
		t.Errorf("expected action 'action2', got '%s'", plan.Steps[0].Action.Name)
	}
}

func TestActionPlannerReplan(t *testing.T) {
	planner := NewActionPlanner([]ActionDefinition{})

	plan := &Plan{
		ID:        "test-plan",
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	oldTime := plan.UpdatedAt

	newPlan, err := planner.Replan(context.Background(), plan, "feedback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !newPlan.UpdatedAt.After(oldTime) {
		t.Error("expected UpdatedAt to be updated")
	}
}

func TestPlanStates(t *testing.T) {
	states := []PlanState{
		PlanStatePending,
		PlanStateRunning,
		PlanStateCompleted,
		PlanStateFailed,
		PlanStateCanceled,
	}

	expected := []string{"pending", "running", "completed", "failed", "canceled"}

	for i, state := range states {
		if string(state) != expected[i] {
			t.Errorf("expected state '%s', got '%s'", expected[i], state)
		}
	}
}

func TestStepStates(t *testing.T) {
	states := []StepState{
		StepStatePending,
		StepStateRunning,
		StepStateCompleted,
		StepStateFailed,
		StepStateSkipped,
	}

	expected := []string{"pending", "running", "completed", "failed", "skipped"}

	for i, state := range states {
		if string(state) != expected[i] {
			t.Errorf("expected state '%s', got '%s'", expected[i], state)
		}
	}
}

func TestActionTypes(t *testing.T) {
	types := []ActionType{
		ActionTypeTool,
		ActionTypeAgent,
		ActionTypeLLM,
		ActionTypeFunction,
		ActionTypeSubPlan,
	}

	expected := []string{"tool", "agent", "llm", "function", "subplan"}

	for i, actionType := range types {
		if string(actionType) != expected[i] {
			t.Errorf("expected type '%s', got '%s'", expected[i], actionType)
		}
	}
}

func TestStepResult(t *testing.T) {
	result := &StepResult{
		Success:  true,
		Output:   "test output",
		Duration: 100,
		Tokens:   50,
	}

	if !result.Success {
		t.Error("expected success to be true")
	}

	if result.Output != "test output" {
		t.Errorf("expected output 'test output', got '%v'", result.Output)
	}

	if result.Duration != 100 {
		t.Errorf("expected duration 100, got %d", result.Duration)
	}

	// 测试错误结果
	errorResult := &StepResult{
		Success: false,
		Error:   "execution failed",
	}

	if errorResult.Success {
		t.Error("expected success to be false")
	}

	if errorResult.Error != "execution failed" {
		t.Errorf("expected error 'execution failed', got '%s'", errorResult.Error)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			input:    `Here is the JSON: {"key": "value"} and some more text`,
			expected: `{"key": "value"}`,
		},
		{
			input:    `No JSON here`,
			expected: "",
		},
		{
			input:    `{incomplete`,
			expected: "",
		},
	}

	for _, tc := range tests {
		result := extractJSON(tc.input)
		if result != tc.expected {
			t.Errorf("extractJSON(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestPlanCreation(t *testing.T) {
	plan := &Plan{
		ID:   "test-plan",
		Goal: "Test goal",
		Steps: []*Step{
			{
				ID:          "step-1",
				Index:       0,
				Description: "First step",
				Action: &Action{
					Type: ActionTypeTool,
					Name: "test_tool",
				},
				State: StepStatePending,
			},
		},
		State:     PlanStatePending,
		Metadata:  map[string]any{"key": "value"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if plan.ID != "test-plan" {
		t.Errorf("expected ID 'test-plan', got '%s'", plan.ID)
	}

	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}

	if plan.Steps[0].Action.Type != ActionTypeTool {
		t.Errorf("expected action type 'tool', got '%s'", plan.Steps[0].Action.Type)
	}
}

func TestToolInfo(t *testing.T) {
	info := ToolInfo{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: map[string]any{
			"input": "string",
		},
	}

	if info.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", info.Name)
	}

	if info.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got '%s'", info.Description)
	}
}
