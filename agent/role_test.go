package agent

import (
	"strings"
	"testing"
)

func TestRoleCreation(t *testing.T) {
	role := Role{
		Name:        "Researcher",
		Goal:        "Find accurate information",
		Backstory:   "An expert researcher with years of experience",
		Constraints: []string{"Be objective", "Cite sources"},
		Expertise:   []string{"Web search", "Data analysis"},
	}

	if role.Name != "Researcher" {
		t.Errorf("expected name 'Researcher', got '%s'", role.Name)
	}

	if role.Goal != "Find accurate information" {
		t.Errorf("expected goal, got '%s'", role.Goal)
	}

	if len(role.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(role.Constraints))
	}

	if len(role.Expertise) != 2 {
		t.Errorf("expected 2 expertise, got %d", len(role.Expertise))
	}
}

func TestRoleToSystemPrompt(t *testing.T) {
	role := Role{
		Name:      "Writer",
		Title:     "Content Writer",
		Goal:      "Create compelling content",
		Backstory: "A professional writer",
	}

	prompt := role.ToSystemPrompt()

	// 验证系统提示词包含角色信息
	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}

	// 系统提示词应包含角色信息
	if !strings.Contains(prompt, "Content Writer") {
		t.Error("system prompt should contain role title")
	}
}

func TestRoleWithConstraints(t *testing.T) {
	role := Role{
		Name:        "Assistant",
		Constraints: []string{"Be polite", "Stay on topic", "No harmful content"},
	}

	if len(role.Constraints) != 3 {
		t.Errorf("expected 3 constraints, got %d", len(role.Constraints))
	}
}

func TestRoleWithExpertise(t *testing.T) {
	role := Role{
		Name:      "Developer",
		Expertise: []string{"Code generation", "Code review", "Debugging"},
	}

	if len(role.Expertise) != 3 {
		t.Errorf("expected 3 expertise areas, got %d", len(role.Expertise))
	}
}

func TestEmptyRole(t *testing.T) {
	role := Role{}

	if role.Name != "" {
		t.Error("expected empty name")
	}

	if role.Goal != "" {
		t.Error("expected empty goal")
	}

	// 空角色的系统提示词应该也是空的或基础的
	prompt := role.ToSystemPrompt()
	_ = prompt // 只要不 panic 就行
}

func TestRoleInAgent(t *testing.T) {
	role := Role{
		Name:      "Analyst",
		Goal:      "Analyze data",
		Backstory: "Data science expert",
	}

	agent := NewBaseAgent(
		WithName("analyst-agent"),
		WithRole(role),
	)

	agentRole := agent.Role()

	if agentRole.Name != "Analyst" {
		t.Errorf("expected role name 'Analyst', got '%s'", agentRole.Name)
	}

	if agentRole.Goal != "Analyze data" {
		t.Errorf("expected goal 'Analyze data', got '%s'", agentRole.Goal)
	}
}

func TestRoleBuilder(t *testing.T) {
	role := NewRole("developer").
		Title("Senior Developer").
		Goal("Build great software").
		Backstory("10 years experience").
		Expertise("Go", "Python", "JavaScript").
		Constraints("Follow best practices", "Write tests").
		Personality("Detail-oriented").
		AllowDelegation(true).
		DelegateTo("junior-dev", "qa-engineer").
		Build()

	if role.Name != "developer" {
		t.Errorf("expected name 'developer', got '%s'", role.Name)
	}

	if role.Title != "Senior Developer" {
		t.Errorf("expected title 'Senior Developer', got '%s'", role.Title)
	}

	if len(role.Expertise) != 3 {
		t.Errorf("expected 3 expertise areas, got %d", len(role.Expertise))
	}

	if !role.AllowDelegation {
		t.Error("expected AllowDelegation to be true")
	}

	if len(role.DelegateTo) != 2 {
		t.Errorf("expected 2 delegate targets, got %d", len(role.DelegateTo))
	}
}

func TestPreDefinedRoles(t *testing.T) {
	// Test ResearcherRole
	if ResearcherRole.Name != "researcher" {
		t.Errorf("expected ResearcherRole name 'researcher', got '%s'", ResearcherRole.Name)
	}

	// Test WriterRole
	if WriterRole.Name != "writer" {
		t.Errorf("expected WriterRole name 'writer', got '%s'", WriterRole.Name)
	}

	// Test DeveloperRole
	if DeveloperRole.Name != "developer" {
		t.Errorf("expected DeveloperRole name 'developer', got '%s'", DeveloperRole.Name)
	}
}

func TestRoleToSystemPromptWithAllFields(t *testing.T) {
	role := NewRole("test").
		Title("Test Role").
		Goal("Test goal").
		Backstory("Test backstory").
		Expertise("Testing").
		Personality("Thorough").
		Constraints("Be precise").
		Build()

	prompt := role.ToSystemPrompt()

	// 验证所有字段都在 prompt 中
	expectedParts := []string{
		"Test Role",
		"Test goal",
		"Test backstory",
		"Testing",
		"Thorough",
		"Be precise",
	}

	for _, part := range expectedParts {
		if !strings.Contains(prompt, part) {
			t.Errorf("expected prompt to contain '%s'", part)
		}
	}
}
