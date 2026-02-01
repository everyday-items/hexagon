package agent

import (
	"testing"
)

func TestTeamCreation(t *testing.T) {
	agent1 := NewBaseAgent(WithName("agent1"))
	agent2 := NewBaseAgent(WithName("agent2"))

	team := NewTeam("test-team",
		WithAgents(agent1, agent2),
		WithTeamDescription("A test team"),
		WithMode(TeamModeSequential),
		WithMaxRounds(5),
	)

	if team.Name() != "test-team" {
		t.Errorf("expected name 'test-team', got '%s'", team.Name())
	}

	if team.Description() != "A test team" {
		t.Errorf("expected description 'A test team', got '%s'", team.Description())
	}

	if len(team.Agents()) != 2 {
		t.Errorf("expected 2 agents, got %d", len(team.Agents()))
	}

	if team.Mode() != TeamModeSequential {
		t.Errorf("expected Sequential mode, got %v", team.Mode())
	}

	if team.ID() == "" {
		t.Error("expected non-empty team ID")
	}
}

func TestTeamModeString(t *testing.T) {
	tests := []struct {
		mode     TeamMode
		expected string
	}{
		{TeamModeSequential, "sequential"},
		{TeamModeHierarchical, "hierarchical"},
		{TeamModeCollaborative, "collaborative"},
		{TeamModeRoundRobin, "round_robin"},
		{TeamMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestTeamAddRemoveAgent(t *testing.T) {
	agent1 := NewBaseAgent(WithName("agent1"))
	agent2 := NewBaseAgent(WithName("agent2"))

	team := NewTeam("test-team")

	// Add agents
	team.AddAgent(agent1)
	team.AddAgent(agent2)

	if len(team.Agents()) != 2 {
		t.Errorf("expected 2 agents after add, got %d", len(team.Agents()))
	}

	// Remove agent
	team.RemoveAgent(agent1.ID())

	if len(team.Agents()) != 1 {
		t.Errorf("expected 1 agent after remove, got %d", len(team.Agents()))
	}
}

func TestTeamWithManager(t *testing.T) {
	manager := NewBaseAgent(WithName("manager"))
	worker := NewBaseAgent(WithName("worker"))

	team := NewTeam("hierarchical-team",
		WithAgents(worker),
		WithManager(manager),
	)

	// WithManager should automatically set mode to Hierarchical
	if team.Mode() != TeamModeHierarchical {
		t.Errorf("expected Hierarchical mode with manager, got %v", team.Mode())
	}
}

func TestTeamInputOutputSchema(t *testing.T) {
	team := NewTeam("test-team")

	inputSchema := team.InputSchema()
	if inputSchema == nil {
		t.Error("expected non-nil input schema")
	}

	outputSchema := team.OutputSchema()
	if outputSchema == nil {
		t.Error("expected non-nil output schema")
	}
}
