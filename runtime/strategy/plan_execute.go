package strategy

import (
	"context"

	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

// PlanExecute asks the model to plan before executing.
type PlanExecute struct{}

func (PlanExecute) Name() string { return "plan-execute" }
func (PlanExecute) BuildSystemPrefix(context.Context, hruntime.Request) string {
	return "Plan before acting. For multi-step tasks, first produce a concise plan, then execute each step, and finish with a final answer."
}
func (PlanExecute) BeforeTurn(context.Context, *hruntime.State) error    { return nil }
func (PlanExecute) AfterLLM(context.Context, *hruntime.State) error      { return nil }
func (PlanExecute) ShouldContinue(context.Context, *hruntime.State) bool { return true }
func (PlanExecute) Finalize(context.Context, *hruntime.State) error      { return nil }
