package strategy

import (
	"context"

	hruntime "github.com/hexagon-codes/hexagon/runtime"
)

// ReAct is the default reasoning/action strategy.
type ReAct struct{}

func (ReAct) Name() string                                               { return "react" }
func (ReAct) BuildSystemPrefix(context.Context, hruntime.Request) string { return "" }
func (ReAct) BeforeTurn(context.Context, *hruntime.State) error          { return nil }
func (ReAct) AfterLLM(context.Context, *hruntime.State) error            { return nil }
func (ReAct) ShouldContinue(context.Context, *hruntime.State) bool       { return true }
func (ReAct) Finalize(context.Context, *hruntime.State) error            { return nil }
