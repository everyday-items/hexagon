package runtime

import "context"

// Strategy controls product-neutral agent behavior.
type Strategy interface {
	Name() string
	BuildSystemPrefix(ctx context.Context, req Request) string
	BeforeTurn(ctx context.Context, state *State) error
	AfterLLM(ctx context.Context, state *State) error
	ShouldContinue(ctx context.Context, state *State) bool
	Finalize(ctx context.Context, state *State) error
}

// NoopStrategy is the default ReAct-compatible strategy.
type NoopStrategy struct{}

func (NoopStrategy) Name() string                                        { return "react" }
func (NoopStrategy) BuildSystemPrefix(context.Context, Request) string   { return "" }
func (NoopStrategy) BeforeTurn(context.Context, *State) error            { return nil }
func (NoopStrategy) AfterLLM(context.Context, *State) error              { return nil }
func (NoopStrategy) ShouldContinue(_ context.Context, state *State) bool { return !state.Final }
func (NoopStrategy) Finalize(context.Context, *State) error              { return nil }

func strategyOrDefault(s Strategy) Strategy {
	if s == nil {
		return NoopStrategy{}
	}
	return s
}
