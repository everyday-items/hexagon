package strategy

import (
	"context"

	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

// Reflection asks the model to self-check before finalizing.
type Reflection struct{}

func (Reflection) Name() string { return "reflection" }
func (Reflection) BuildSystemPrefix(context.Context, hruntime.Request) string {
	return "After producing an answer, perform a concise self-check for missing constraints, reasoning mistakes, and contradictions. Correct the answer if needed."
}
func (Reflection) BeforeTurn(context.Context, *hruntime.State) error    { return nil }
func (Reflection) AfterLLM(context.Context, *hruntime.State) error      { return nil }
func (Reflection) ShouldContinue(context.Context, *hruntime.State) bool { return true }
func (Reflection) Finalize(context.Context, *hruntime.State) error      { return nil }
