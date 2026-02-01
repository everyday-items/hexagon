package synthesizer

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/hexagon/rag"
	"github.com/everyday-items/hexagon/testing/mock"
)

func TestRefineSynthesizerCreation(t *testing.T) {
	synth := NewRefineSynthesizer()

	if synth.Name() != "refine_synthesizer" {
		t.Errorf("expected name 'refine_synthesizer', got '%s'", synth.Name())
	}
}

func TestRefineSynthesizerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("test response")

	synth := NewRefineSynthesizer(
		WithRefineSynthesizerName("custom-refine"),
		WithRefineSynthesizerLLM(mockLLM),
		WithRefinePrompt("Custom prompt: {context}"),
	)

	if synth.Name() != "custom-refine" {
		t.Errorf("expected name 'custom-refine', got '%s'", synth.Name())
	}
}

func TestRefineSynthesizerEmptyDocs(t *testing.T) {
	synth := NewRefineSynthesizer()

	resp, err := synth.Synthesize(context.Background(), "test query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "没有找到相关信息来回答您的问题。" {
		t.Errorf("unexpected content for empty docs: %s", resp.Content)
	}

	if resp.Metadata["doc_count"] != 0 {
		t.Error("expected doc_count to be 0")
	}
}

func TestRefineSynthesizerWithoutLLM(t *testing.T) {
	synth := NewRefineSynthesizer()

	docs := []rag.Document{
		{ID: "1", Content: "First document content"},
		{ID: "2", Content: "Second document content"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Metadata["strategy"] != "refine" {
		t.Error("expected strategy 'refine'")
	}

	if len(resp.SourceDocuments) != 2 {
		t.Errorf("expected 2 source documents, got %d", len(resp.SourceDocuments))
	}
}

func TestRefineSynthesizerWithLLM(t *testing.T) {
	mockLLM := mock.NewLLMProvider("refine-test")
	mockLLM.AddResponse("Initial answer")
	mockLLM.AddResponse("Refined answer")

	synth := NewRefineSynthesizer(
		WithRefineSynthesizerLLM(mockLLM),
	)

	docs := []rag.Document{
		{ID: "1", Content: "First document"},
		{ID: "2", Content: "Second document"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Refined answer" {
		t.Errorf("expected 'Refined answer', got '%s'", resp.Content)
	}
}

func TestCompactSynthesizerCreation(t *testing.T) {
	synth := NewCompactSynthesizer()

	if synth.Name() != "compact_synthesizer" {
		t.Errorf("expected name 'compact_synthesizer', got '%s'", synth.Name())
	}
}

func TestCompactSynthesizerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("test response")

	synth := NewCompactSynthesizer(
		WithCompactSynthesizerName("custom-compact"),
		WithCompactSynthesizerMaxContext(2048),
		WithCompactSynthesizerLLM(mockLLM),
	)

	if synth.Name() != "custom-compact" {
		t.Errorf("expected name 'custom-compact', got '%s'", synth.Name())
	}

	if synth.maxContextLength != 2048 {
		t.Errorf("expected maxContextLength 2048, got %d", synth.maxContextLength)
	}
}

func TestCompactSynthesizerEmptyDocs(t *testing.T) {
	synth := NewCompactSynthesizer()

	resp, err := synth.Synthesize(context.Background(), "test query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Metadata["strategy"] != "compact" {
		t.Error("expected strategy 'compact'")
	}
}

func TestCompactSynthesizerWithoutLLM(t *testing.T) {
	synth := NewCompactSynthesizer()

	docs := []rag.Document{
		{ID: "1", Content: "Document one"},
		{ID: "2", Content: "Document two"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Metadata["doc_count"] != len(docs) {
		t.Errorf("expected doc_count %d, got %v", len(docs), resp.Metadata["doc_count"])
	}
}

func TestCompactSynthesizerWithLLM(t *testing.T) {
	mockLLM := mock.FixedProvider("Compact answer")

	synth := NewCompactSynthesizer(
		WithCompactSynthesizerLLM(mockLLM),
	)

	docs := []rag.Document{
		{ID: "1", Content: "Document one"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Compact answer" {
		t.Errorf("expected 'Compact answer', got '%s'", resp.Content)
	}
}

func TestTreeSummarizeSynthesizerCreation(t *testing.T) {
	synth := NewTreeSummarizeSynthesizer()

	if synth.Name() != "tree_summarize_synthesizer" {
		t.Errorf("expected name 'tree_summarize_synthesizer', got '%s'", synth.Name())
	}
}

func TestTreeSummarizeSynthesizerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("test response")

	synth := NewTreeSummarizeSynthesizer(
		WithTreeSynthesizerName("custom-tree"),
		WithTreeSynthesizerChunkSize(3),
		WithTreeSynthesizerLLM(mockLLM),
	)

	if synth.Name() != "custom-tree" {
		t.Errorf("expected name 'custom-tree', got '%s'", synth.Name())
	}

	if synth.chunkSize != 3 {
		t.Errorf("expected chunkSize 3, got %d", synth.chunkSize)
	}
}

func TestTreeSummarizeSynthesizerEmptyDocs(t *testing.T) {
	synth := NewTreeSummarizeSynthesizer()

	resp, err := synth.Synthesize(context.Background(), "test query", []rag.Document{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Metadata["strategy"] != "tree_summarize" {
		t.Error("expected strategy 'tree_summarize'")
	}
}

func TestTreeSummarizeSynthesizerWithoutLLM(t *testing.T) {
	synth := NewTreeSummarizeSynthesizer()

	docs := []rag.Document{
		{ID: "1", Content: "Document one"},
		{ID: "2", Content: "Document two"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.SourceDocuments) != 2 {
		t.Errorf("expected 2 source documents, got %d", len(resp.SourceDocuments))
	}
}

func TestSimpleSummarizeSynthesizerCreation(t *testing.T) {
	synth := NewSimpleSummarizeSynthesizer()

	if synth.Name() != "simple_summarize_synthesizer" {
		t.Errorf("expected name 'simple_summarize_synthesizer', got '%s'", synth.Name())
	}
}

func TestSimpleSummarizeSynthesizerWithOptions(t *testing.T) {
	mockLLM := mock.FixedProvider("test response")

	synth := NewSimpleSummarizeSynthesizer(
		WithSimpleSynthesizerName("custom-simple"),
		WithSimpleSynthesizerLLM(mockLLM),
	)

	if synth.Name() != "custom-simple" {
		t.Errorf("expected name 'custom-simple', got '%s'", synth.Name())
	}
}

func TestSimpleSummarizeSynthesizerWithLLM(t *testing.T) {
	mockLLM := mock.FixedProvider("Simple answer")

	synth := NewSimpleSummarizeSynthesizer(
		WithSimpleSynthesizerLLM(mockLLM),
	)

	docs := []rag.Document{
		{ID: "1", Content: "Document one"},
	}

	resp, err := synth.Synthesize(context.Background(), "test query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Simple answer" {
		t.Errorf("expected 'Simple answer', got '%s'", resp.Content)
	}
}

func TestSynthesizerFactory(t *testing.T) {
	tests := []struct {
		synthType    SynthesizerType
		expectedName string
	}{
		{TypeRefine, "refine_synthesizer"},
		{TypeCompact, "compact_synthesizer"},
		{TypeTreeSummarize, "tree_summarize_synthesizer"},
		{TypeSimpleSummarize, "simple_summarize_synthesizer"},
		{"unknown", "compact_synthesizer"}, // default
	}

	for _, tc := range tests {
		synth := New(tc.synthType)
		if synth.Name() != tc.expectedName {
			t.Errorf("New(%s): expected name '%s', got '%s'", tc.synthType, tc.expectedName, synth.Name())
		}
	}
}

func TestSynthesizerTypes(t *testing.T) {
	types := []SynthesizerType{
		TypeRefine,
		TypeCompact,
		TypeTreeSummarize,
		TypeSimpleSummarize,
	}

	expected := []string{"refine", "compact", "tree_summarize", "simple_summarize"}

	for i, synthType := range types {
		if string(synthType) != expected[i] {
			t.Errorf("expected type '%s', got '%s'", expected[i], synthType)
		}
	}
}

func TestSynthesizeOptions(t *testing.T) {
	synth := NewCompactSynthesizer()

	docs := []rag.Document{
		{ID: "1", Content: "Test document"},
	}

	// 测试各种选项
	resp, err := synth.Synthesize(context.Background(), "test query", docs,
		WithMaxTokens(512),
		WithTemperature(0.5),
		WithPromptTemplate("Custom template"),
		WithSourceDocuments(false),
		WithSynthesizeTimeout(5*time.Second),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestResponse(t *testing.T) {
	resp := &Response{
		Content: "Test content",
		SourceDocuments: []rag.Document{
			{ID: "1", Content: "Doc 1"},
		},
		Metadata: map[string]any{
			"strategy":     "compact",
			"total_tokens": 100,
		},
	}

	if resp.Content != "Test content" {
		t.Errorf("unexpected content: %s", resp.Content)
	}

	if len(resp.SourceDocuments) != 1 {
		t.Errorf("expected 1 source document, got %d", len(resp.SourceDocuments))
	}

	if resp.Metadata["strategy"] != "compact" {
		t.Error("expected strategy 'compact'")
	}
}
