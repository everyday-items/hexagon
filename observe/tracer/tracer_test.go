package tracer

import (
	"context"
	"testing"
)

func TestSpanKind_Constants(t *testing.T) {
	// 确保常量值正确
	if SpanKindInternal != 0 {
		t.Errorf("expected SpanKindInternal = 0, got %d", SpanKindInternal)
	}
	if SpanKindAgent != 1 {
		t.Errorf("expected SpanKindAgent = 1, got %d", SpanKindAgent)
	}
	if SpanKindLLM != 2 {
		t.Errorf("expected SpanKindLLM = 2, got %d", SpanKindLLM)
	}
	if SpanKindTool != 3 {
		t.Errorf("expected SpanKindTool = 3, got %d", SpanKindTool)
	}
	if SpanKindRetrieval != 4 {
		t.Errorf("expected SpanKindRetrieval = 4, got %d", SpanKindRetrieval)
	}
	if SpanKindEmbedding != 5 {
		t.Errorf("expected SpanKindEmbedding = 5, got %d", SpanKindEmbedding)
	}
}

func TestStatusCode_Constants(t *testing.T) {
	if StatusCodeUnset != 0 {
		t.Errorf("expected StatusCodeUnset = 0, got %d", StatusCodeUnset)
	}
	if StatusCodeOK != 1 {
		t.Errorf("expected StatusCodeOK = 1, got %d", StatusCodeOK)
	}
	if StatusCodeError != 2 {
		t.Errorf("expected StatusCodeError = 2, got %d", StatusCodeError)
	}
}

func TestTokenUsage(t *testing.T) {
	usage := TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens = 100, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("expected CompletionTokens = 50, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("expected TotalTokens = 150, got %d", usage.TotalTokens)
	}
}

func TestSpanConfig_WithOptions(t *testing.T) {
	config := &SpanConfig{}

	// 测试 WithSpanKind
	WithSpanKind(SpanKindLLM)(config)
	if config.Kind != SpanKindLLM {
		t.Errorf("expected Kind = SpanKindLLM, got %d", config.Kind)
	}

	// 测试 WithAttributes
	attrs := map[string]any{"key": "value"}
	WithAttributes(attrs)(config)
	if config.Attributes["key"] != "value" {
		t.Error("expected Attributes to contain key=value")
	}
}

func TestContextWithSpan(t *testing.T) {
	ctx := context.Background()

	// 测试从空 context 获取
	span := SpanFromContext(ctx)
	if span != nil {
		t.Error("expected nil span from empty context")
	}
}

func TestContextWithTracer(t *testing.T) {
	ctx := context.Background()

	// 测试从空 context 获取
	tracer := TracerFromContext(ctx)
	if tracer != nil {
		t.Error("expected nil tracer from empty context")
	}
}

func TestAttributeKeys(t *testing.T) {
	// 验证常用属性键
	expectedKeys := map[string]string{
		"AttrAgentID":             "agent.id",
		"AttrAgentName":           "agent.name",
		"AttrAgentRole":           "agent.role",
		"AttrLLMProvider":         "llm.provider",
		"AttrLLMModel":            "llm.model",
		"AttrLLMPromptTokens":     "llm.prompt_tokens",
		"AttrLLMCompletionTokens": "llm.completion_tokens",
		"AttrLLMTotalTokens":      "llm.total_tokens",
		"AttrToolName":            "tool.name",
		"AttrToolArguments":       "tool.arguments",
		"AttrToolResult":          "tool.result",
		"AttrRetrievalQuery":      "retrieval.query",
		"AttrErrorType":           "error.type",
		"AttrErrorMessage":        "error.message",
	}

	actualKeys := map[string]string{
		"AttrAgentID":             AttrAgentID,
		"AttrAgentName":           AttrAgentName,
		"AttrAgentRole":           AttrAgentRole,
		"AttrLLMProvider":         AttrLLMProvider,
		"AttrLLMModel":            AttrLLMModel,
		"AttrLLMPromptTokens":     AttrLLMPromptTokens,
		"AttrLLMCompletionTokens": AttrLLMCompletionTokens,
		"AttrLLMTotalTokens":      AttrLLMTotalTokens,
		"AttrToolName":            AttrToolName,
		"AttrToolArguments":       AttrToolArguments,
		"AttrToolResult":          AttrToolResult,
		"AttrRetrievalQuery":      AttrRetrievalQuery,
		"AttrErrorType":           AttrErrorType,
		"AttrErrorMessage":        AttrErrorMessage,
	}

	for name, expected := range expectedKeys {
		actual := actualKeys[name]
		if actual != expected {
			t.Errorf("%s: expected %q, got %q", name, expected, actual)
		}
	}
}
