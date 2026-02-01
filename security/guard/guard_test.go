package guard

import (
	"context"
	"errors"
	"testing"
)

// MockGuard for testing
type MockGuard struct {
	name    string
	enabled bool
	result  *CheckResult
	err     error
}

func (g *MockGuard) Name() string                                              { return g.name }
func (g *MockGuard) Enabled() bool                                             { return g.enabled }
func (g *MockGuard) Check(ctx context.Context, input string) (*CheckResult, error) {
	return g.result, g.err
}

func TestCheckResult(t *testing.T) {
	result := &CheckResult{
		Passed:   true,
		Score:    0.5,
		Category: "test",
		Reason:   "test reason",
		Findings: []Finding{
			{Type: "test", Text: "test finding", Severity: "low"},
		},
	}

	if !result.Passed {
		t.Error("expected Passed to be true")
	}

	if result.Score != 0.5 {
		t.Errorf("expected Score 0.5, got %f", result.Score)
	}

	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

func TestNewGuardChain(t *testing.T) {
	chain := NewGuardChain(ChainModeAll)

	if chain == nil {
		t.Fatal("expected non-nil chain")
	}

	if chain.Name() != "guard_chain" {
		t.Errorf("expected name 'guard_chain', got '%s'", chain.Name())
	}

	if chain.Enabled() {
		t.Error("expected chain to be disabled with no guards")
	}
}

func TestGuardChainAdd(t *testing.T) {
	chain := NewGuardChain(ChainModeAll)
	guard := &MockGuard{name: "test", enabled: true}

	chain.Add(guard)

	if !chain.Enabled() {
		t.Error("expected chain to be enabled after adding guard")
	}
}

func TestGuardChainCheckModeAll(t *testing.T) {
	guard1 := &MockGuard{
		name:    "guard1",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.3},
	}
	guard2 := &MockGuard{
		name:    "guard2",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.5},
	}

	chain := NewGuardChain(ChainModeAll, guard1, guard2)
	ctx := context.Background()

	result, err := chain.Check(ctx, "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected all guards to pass")
	}

	if result.Score != 0.5 {
		t.Errorf("expected max score 0.5, got %f", result.Score)
	}
}

func TestGuardChainCheckModeAllWithFailure(t *testing.T) {
	guard1 := &MockGuard{
		name:    "guard1",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.3},
	}
	guard2 := &MockGuard{
		name:    "guard2",
		enabled: true,
		result:  &CheckResult{Passed: false, Score: 0.8, Reason: "failed"},
	}

	chain := NewGuardChain(ChainModeAll, guard1, guard2)
	ctx := context.Background()

	result, err := chain.Check(ctx, "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected chain to fail when one guard fails")
	}
}

func TestGuardChainCheckModeAny(t *testing.T) {
	guard1 := &MockGuard{
		name:    "guard1",
		enabled: true,
		result:  &CheckResult{Passed: false, Score: 0.8},
	}
	guard2 := &MockGuard{
		name:    "guard2",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.3},
	}

	chain := NewGuardChain(ChainModeAny, guard1, guard2)
	ctx := context.Background()

	result, err := chain.Check(ctx, "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected chain to pass when any guard passes")
	}
}

func TestGuardChainCheckModeFirst(t *testing.T) {
	guard1 := &MockGuard{
		name:    "guard1",
		enabled: true,
		result:  &CheckResult{Passed: false, Score: 0.8, Reason: "first failed"},
	}
	guard2 := &MockGuard{
		name:    "guard2",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.3},
	}

	chain := NewGuardChain(ChainModeFirst, guard1, guard2)
	ctx := context.Background()

	result, err := chain.Check(ctx, "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected chain to fail at first failure")
	}

	if result.Reason != "first failed" {
		t.Errorf("expected reason 'first failed', got '%s'", result.Reason)
	}
}

func TestGuardChainCheckDisabledGuard(t *testing.T) {
	guard1 := &MockGuard{
		name:    "guard1",
		enabled: false, // Disabled
		result:  &CheckResult{Passed: false, Score: 0.8},
	}
	guard2 := &MockGuard{
		name:    "guard2",
		enabled: true,
		result:  &CheckResult{Passed: true, Score: 0.3},
	}

	// Use ChainModeAny - if any enabled guard passes, the chain passes
	chain := NewGuardChain(ChainModeAny, guard1, guard2)
	ctx := context.Background()

	result, err := chain.Check(ctx, "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Disabled guard should be skipped, enabled guard passes
	if !result.Passed {
		t.Error("expected chain to pass (disabled guard skipped)")
	}
}

func TestGuardChainCheckWithError(t *testing.T) {
	expectedErr := errors.New("guard error")
	guard := &MockGuard{
		name:    "error-guard",
		enabled: true,
		err:     expectedErr,
	}

	chain := NewGuardChain(ChainModeAll, guard)
	ctx := context.Background()

	_, err := chain.Check(ctx, "test input")
	if err == nil {
		t.Error("expected error")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}

	if config.Threshold != 0.8 {
		t.Errorf("expected Threshold 0.8, got %f", config.Threshold)
	}

	if config.Action != ActionBlock {
		t.Errorf("expected Action 'block', got '%s'", config.Action)
	}
}

func TestToMiddleware(t *testing.T) {
	guard := &MockGuard{
		name:    "test-guard",
		enabled: true,
		result:  &CheckResult{Passed: true},
	}

	middleware := ToMiddleware(guard, ActionBlock)

	ctx := context.Background()
	next := func(ctx context.Context, input string) (string, error) {
		return "processed: " + input, nil
	}

	result, err := middleware(ctx, "test", next)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "processed: test" {
		t.Errorf("expected 'processed: test', got '%s'", result)
	}
}

func TestToMiddlewareBlock(t *testing.T) {
	guard := &MockGuard{
		name:    "block-guard",
		enabled: true,
		result:  &CheckResult{Passed: false, Reason: "blocked"},
	}

	middleware := ToMiddleware(guard, ActionBlock)

	ctx := context.Background()
	next := func(ctx context.Context, input string) (string, error) {
		return "should not reach", nil
	}

	_, err := middleware(ctx, "test", next)
	if err == nil {
		t.Error("expected block error")
	}
}

func TestFinding(t *testing.T) {
	finding := Finding{
		Type:       "pii",
		Text:       "email@example.com",
		Position:   Position{Start: 10, End: 28},
		Severity:   "high",
		Suggestion: "Redact email address",
	}

	if finding.Type != "pii" {
		t.Errorf("expected type 'pii', got '%s'", finding.Type)
	}

	if finding.Position.Start != 10 {
		t.Errorf("expected start 10, got %d", finding.Position.Start)
	}

	if finding.Position.End != 28 {
		t.Errorf("expected end 28, got %d", finding.Position.End)
	}
}

func TestGuardAction(t *testing.T) {
	tests := []struct {
		action   GuardAction
		expected string
	}{
		{ActionBlock, "block"},
		{ActionWarn, "warn"},
		{ActionLog, "log"},
		{ActionRedact, "redact"},
	}

	for _, tt := range tests {
		if string(tt.action) != tt.expected {
			t.Errorf("expected '%s', got '%s'", tt.expected, tt.action)
		}
	}
}
