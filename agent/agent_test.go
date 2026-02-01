package agent

import (
	"context"
	"testing"
)

func TestBaseAgentCreation(t *testing.T) {
	agent := NewBaseAgent(
		WithName("test-agent"),
		WithDescription("A test agent"),
		WithSystemPrompt("You are a test assistant"),
		WithMaxIterations(5),
	)

	if agent.Name() != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", agent.Name())
	}

	if agent.Description() != "A test agent" {
		t.Errorf("expected description 'A test agent', got '%s'", agent.Description())
	}

	if agent.ID() == "" {
		t.Error("expected non-empty ID")
	}

	if agent.Memory() == nil {
		t.Error("expected default memory to be set")
	}
}

func TestBaseAgentWithRole(t *testing.T) {
	role := Role{
		Name:      "Researcher",
		Goal:      "Find information",
		Backstory: "Expert in research",
	}

	agent := NewBaseAgent(
		WithRole(role),
	)

	if agent.Role().Name != "Researcher" {
		t.Errorf("expected role name 'Researcher', got '%s'", agent.Role().Name)
	}
}

func TestInputOutput(t *testing.T) {
	input := Input{
		Query: "Hello",
		Context: map[string]any{
			"user": "test",
		},
	}

	if input.Query != "Hello" {
		t.Errorf("expected query 'Hello', got '%s'", input.Query)
	}

	if input.Context["user"] != "test" {
		t.Errorf("expected context user 'test', got '%v'", input.Context["user"])
	}

	output := Output{
		Content: "Hi there",
		Metadata: map[string]any{
			"tokens": 10,
		},
	}

	if output.Content != "Hi there" {
		t.Errorf("expected content 'Hi there', got '%s'", output.Content)
	}
}

func TestStateManager(t *testing.T) {
	global := NewGlobalState()
	sm := NewStateManager("session-1", global)

	// Test Turn State
	sm.Turn().Set("turn_key", "turn_value")
	val, ok := sm.Turn().Get("turn_key")
	if !ok || val != "turn_value" {
		t.Errorf("expected turn_value, got %v", val)
	}

	// Test Session State
	sm.Session().Set("session_key", 123)
	val, ok = sm.Session().Get("session_key")
	if !ok || val != 123 {
		t.Errorf("expected 123, got %v", val)
	}

	// Test Agent State
	sm.Agent().Set("agent_key", true)
	val, ok = sm.Agent().Get("agent_key")
	if !ok || val != true {
		t.Errorf("expected true, got %v", val)
	}

	// Test Clear Turn
	sm.Turn().Set("temp", "data")
	sm.Turn().Clear()
	_, ok = sm.Turn().Get("temp")
	if ok {
		t.Error("expected turn state to be cleared")
	}
}

func TestGlobalState(t *testing.T) {
	gs := NewGlobalState()

	// Test Set and Get
	gs.Set("key1", "value1")
	val, ok := gs.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}

	// Test Delete
	gs.Delete("key1")
	_, ok = gs.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}

	// Test RegisterAgent
	agent := NewBaseAgent(WithName("test"))
	gs.RegisterAgent("agent-1", agent)

	retrieved, found := gs.GetAgent("agent-1")
	if !found || retrieved == nil {
		t.Error("expected agent to be registered")
	}
}

func TestContextVariables(t *testing.T) {
	vars := ContextVariables{
		"user_id": "123",
		"name":    "Test",
	}

	// Test Get
	val, ok := vars.Get("user_id")
	if !ok || val != "123" {
		t.Errorf("expected '123', got %v", val)
	}

	// Test Set
	vars.Set("new_key", "new_value")
	val, ok = vars.Get("new_key")
	if !ok || val != "new_value" {
		t.Errorf("expected 'new_value', got %v", val)
	}

	// Test Clone
	cloned := vars.Clone()
	cloned.Set("cloned_key", "cloned_value")

	_, ok = vars.Get("cloned_key")
	if ok {
		t.Error("clone should not affect original")
	}

	// Test Merge
	other := ContextVariables{"merged": "value"}
	vars.Merge(other)
	val, ok = vars.Get("merged")
	if !ok || val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestContextWithVariables(t *testing.T) {
	ctx := context.Background()
	vars := ContextVariables{"key": "value"}

	ctx = ContextWithVariables(ctx, vars)

	retrieved := VariablesFromContext(ctx)
	if retrieved == nil {
		t.Fatal("expected variables in context")
	}

	val, ok := retrieved.Get("key")
	if !ok || val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestUpdateContextVariables(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithVariables(ctx, ContextVariables{"initial": "value"})

	ctx = UpdateContextVariables(ctx, ContextVariables{"new": "data"})

	vars := VariablesFromContext(ctx)

	// Should have both keys
	val1, _ := vars.Get("initial")
	val2, _ := vars.Get("new")

	if val1 != "value" {
		t.Errorf("expected initial value preserved")
	}
	if val2 != "data" {
		t.Errorf("expected new value added")
	}
}
