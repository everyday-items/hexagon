package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/hexagon-codes/ai-core/llm"
	"github.com/hexagon-codes/hexagon/hooks"
	"github.com/hexagon-codes/hexagon/internal/util"
	agentruntime "github.com/hexagon-codes/hexagon/runtime"
)

func runCompletionWithRuntime(ctx context.Context, provider llm.Provider, runID string, messages []llm.Message, sink agentruntime.EventSink) (*llm.CompletionResponse, error) {
	if provider == nil {
		return nil, fmt.Errorf("LLM provider not configured")
	}
	if runID == "" {
		runID = util.GenerateID("run")
	}

	runner := agentruntime.NewRunner(agentruntime.Config{
		ProviderSelector: agentruntime.StaticProviderSelector{
			Provider: provider,
			Name:     provider.Name(),
		},
		DefaultMaxTurns: 1,
	})
	result, err := runner.RunWithSink(ctx, agentruntime.Request{
		ID:         runID,
		Messages:   append([]llm.Message(nil), messages...),
		Limits:     agentruntime.Limits{MaxTurns: 1},
		StreamMode: agentruntime.StreamModeEvents,
	}, sink)
	if err != nil {
		return nil, err
	}
	resp := &llm.CompletionResponse{}
	if result != nil {
		resp.Content = result.Content
		resp.Usage = result.Usage
	}
	return resp, nil
}

func runtimeLLMHookSink(runID, providerName string, hookManager *hooks.Manager) agentruntime.EventSink {
	if hookManager == nil {
		return nil
	}
	var llmStart time.Time
	return agentruntime.EventSinkFunc(func(ctx context.Context, event agentruntime.Event) error {
		switch event.Type {
		case agentruntime.EventLLMStarted:
			llmStart = time.Now()
			if v, ok := event.Metadata["provider"].(string); ok && v != "" {
				providerName = v
			}
			var messages []any
			if event.State != nil {
				messages = convertMessagesToAny(event.State.Messages)
			}
			return hookManager.TriggerLLMStart(ctx, &hooks.LLMStartEvent{
				RunID:    runID,
				Provider: providerName,
				Messages: messages,
			})
		case agentruntime.EventLLMCompleted:
			if event.Response == nil {
				return nil
			}
			return hookManager.TriggerLLMEnd(ctx, &hooks.LLMEndEvent{
				RunID:            runID,
				Response:         event.Response.Content,
				PromptTokens:     event.Response.Usage.PromptTokens,
				CompletionTokens: event.Response.Usage.CompletionTokens,
				Duration:         time.Since(llmStart).Milliseconds(),
			})
		}
		return nil
	})
}
