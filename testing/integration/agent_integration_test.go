// Package integration 提供 Hexagon 框架的集成测试
package integration

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/agent"
)

// TestAgentStateManagerIntegration tests the complete StateManager workflow
func TestAgentStateManagerIntegration(t *testing.T) {
	// Create a global state shared across agents
	globalState := agent.NewGlobalState()

	// Create multiple agents sharing the same global state
	sm1 := agent.NewStateManager("session-1", globalState)
	sm2 := agent.NewStateManager("session-2", globalState)

	// Agent 1 sets global state
	globalState.Set("shared_key", "shared_value")

	// Agent 2 should see it
	val, ok := sm2.Global().Get("shared_key")
	if !ok || val != "shared_value" {
		t.Error("expected global state to be shared between sessions")
	}

	// Test session isolation
	sm1.Session().Set("private", "session1_data")
	sm2.Session().Set("private", "session2_data")

	val1, _ := sm1.Session().Get("private")
	val2, _ := sm2.Session().Get("private")

	if val1 == val2 {
		t.Error("session state should be isolated between sessions")
	}

	// Test turn state lifecycle
	sm1.Turn().Set("turn_data", "turn1")
	sm1.NewTurn() // Create new turn

	_, ok = sm1.Turn().Get("turn_data")
	if ok {
		t.Error("turn state should be reset after NewTurn")
	}
}

// TestAgentTeamIntegration tests agent team functionality
func TestAgentTeamIntegration(t *testing.T) {
	// Create specialized agents
	researcher := agent.NewBaseAgent(
		agent.WithName("researcher"),
		agent.WithDescription("Researches topics"),
	)

	writer := agent.NewBaseAgent(
		agent.WithName("writer"),
		agent.WithDescription("Writes content"),
	)

	reviewer := agent.NewBaseAgent(
		agent.WithName("reviewer"),
		agent.WithDescription("Reviews content"),
	)

	// Create a sequential team
	team := agent.NewTeam("content-team",
		agent.WithAgents(researcher, writer, reviewer),
		agent.WithMode(agent.TeamModeSequential),
		agent.WithTeamDescription("Content creation team"),
	)

	if len(team.Agents()) != 3 {
		t.Errorf("expected 3 agents, got %d", len(team.Agents()))
	}

	if team.Mode() != agent.TeamModeSequential {
		t.Errorf("expected Sequential mode, got %v", team.Mode())
	}

	// Test adding/removing agents dynamically
	editor := agent.NewBaseAgent(agent.WithName("editor"))
	team.AddAgent(editor)

	if len(team.Agents()) != 4 {
		t.Error("expected 4 agents after add")
	}

	team.RemoveAgent(editor.ID())
	if len(team.Agents()) != 3 {
		t.Error("expected 3 agents after remove")
	}
}

// TestAgentContextVariablesIntegration tests context variables flow
func TestAgentContextVariablesIntegration(t *testing.T) {
	ctx := context.Background()

	// Initialize with user info
	vars := agent.ContextVariables{
		"user_id":   "123",
		"user_name": "Alice",
		"role":      "admin",
	}

	ctx = agent.ContextWithVariables(ctx, vars)

	// Simulate agent processing that updates variables
	ctx = agent.UpdateContextVariables(ctx, agent.ContextVariables{
		"action":       "query",
		"query_result": "success",
	})

	// Verify all variables are present
	finalVars := agent.VariablesFromContext(ctx)

	expectedKeys := []string{"user_id", "user_name", "role", "action", "query_result"}
	for _, key := range expectedKeys {
		_, ok := finalVars.Get(key)
		if !ok {
			t.Errorf("expected key '%s' to be present", key)
		}
	}

	// Test clone isolation
	cloned := finalVars.Clone()
	cloned.Set("new_key", "new_value")

	_, ok := finalVars.Get("new_key")
	if ok {
		t.Error("original should not be affected by clone modification")
	}
}

// TestAgentRoleIntegration tests role-based agent configuration
func TestAgentRoleIntegration(t *testing.T) {
	role := agent.Role{
		Name:        "Data Analyst",
		Goal:        "Analyze data and provide insights",
		Backstory:   "Expert in statistical analysis and data visualization",
		Constraints: []string{"Use only approved data sources", "Maintain data privacy"},
	}

	analyst := agent.NewBaseAgent(
		agent.WithName("analyst"),
		agent.WithRole(role),
		agent.WithMaxIterations(10),
	)

	if analyst.Role().Name != "Data Analyst" {
		t.Errorf("expected role name 'Data Analyst', got '%s'", analyst.Role().Name)
	}

	if analyst.Role().Goal != "Analyze data and provide insights" {
		t.Error("expected correct role goal")
	}

	if len(analyst.Role().Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(analyst.Role().Constraints))
	}
}

// TestAgentGlobalStateAgentRegistry tests agent registration
func TestAgentGlobalStateAgentRegistry(t *testing.T) {
	globalState := agent.NewGlobalState()

	agent1 := agent.NewBaseAgent(agent.WithName("agent1"))
	agent2 := agent.NewBaseAgent(agent.WithName("agent2"))

	// Register agents
	globalState.RegisterAgent("agent-1", agent1)
	globalState.RegisterAgent("agent-2", agent2)

	// List agents
	agentIDs := globalState.ListAgents()
	if len(agentIDs) != 2 {
		t.Errorf("expected 2 registered agents, got %d", len(agentIDs))
	}

	// Retrieve agents
	retrieved, ok := globalState.GetAgent("agent-1")
	if !ok {
		t.Error("expected agent-1 to be retrievable")
	}

	if retrieved.Name() != "agent1" {
		t.Errorf("expected name 'agent1', got '%s'", retrieved.Name())
	}

	// Non-existent agent
	_, ok = globalState.GetAgent("nonexistent")
	if ok {
		t.Error("expected non-existent agent to return false")
	}
}
