package rag

import (
	"testing"
	"time"
)

func TestDocument(t *testing.T) {
	now := time.Now()
	doc := Document{
		ID:      "doc-1",
		Content: "This is a test document.",
		Metadata: map[string]any{
			"source": "test",
			"page":   1,
		},
		Score:     0.95,
		CreatedAt: now,
	}

	if doc.ID != "doc-1" {
		t.Errorf("expected ID 'doc-1', got '%s'", doc.ID)
	}
	if doc.Content != "This is a test document." {
		t.Errorf("expected content 'This is a test document.', got '%s'", doc.Content)
	}
	if doc.Metadata["source"] != "test" {
		t.Error("expected metadata source to be 'test'")
	}
	if doc.Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", doc.Score)
	}
}

func TestRetrieveConfig(t *testing.T) {
	config := &RetrieveConfig{
		TopK:     10,
		MinScore: 0.5,
		Filter: map[string]any{
			"category": "tech",
		},
	}

	if config.TopK != 10 {
		t.Errorf("expected TopK 10, got %d", config.TopK)
	}
	if config.MinScore != 0.5 {
		t.Errorf("expected MinScore 0.5, got %f", config.MinScore)
	}
	if config.Filter["category"] != "tech" {
		t.Error("expected filter category to be 'tech'")
	}
}

func TestRetrieveOptions(t *testing.T) {
	config := &RetrieveConfig{}

	// 测试 WithTopK
	WithTopK(20)(config)
	if config.TopK != 20 {
		t.Errorf("expected TopK 20, got %d", config.TopK)
	}

	// 测试 WithMinScore
	WithMinScore(0.7)(config)
	if config.MinScore != 0.7 {
		t.Errorf("expected MinScore 0.7, got %f", config.MinScore)
	}

	// 测试 WithFilter
	filter := map[string]any{"type": "article"}
	WithFilter(filter)(config)
	if config.Filter["type"] != "article" {
		t.Error("expected filter type to be 'article'")
	}
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine()

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	// 检查默认值
	if engine.topK != 5 {
		t.Errorf("expected default topK 5, got %d", engine.topK)
	}
	if engine.minScore != 0.0 {
		t.Errorf("expected default minScore 0.0, got %f", engine.minScore)
	}
}

func TestNewEngine_WithOptions(t *testing.T) {
	engine := NewEngine(
		WithEngineTopK(10),
		WithEngineMinScore(0.3),
	)

	if engine.topK != 10 {
		t.Errorf("expected topK 10, got %d", engine.topK)
	}
	if engine.minScore != 0.3 {
		t.Errorf("expected minScore 0.3, got %f", engine.minScore)
	}
}

func TestEngine_Ingest_NoLoader(t *testing.T) {
	engine := NewEngine()

	err := engine.Ingest(nil)
	if err == nil {
		t.Error("expected error for missing loader")
	}
}

func TestEngine_Retrieve_NoStore(t *testing.T) {
	engine := NewEngine()

	_, err := engine.Retrieve(nil, "test query")
	if err == nil {
		t.Error("expected error for missing store")
	}
}

func TestEngine_Delete_NoStore(t *testing.T) {
	engine := NewEngine()

	err := engine.Delete(nil, []string{"doc-1"})
	if err == nil {
		t.Error("expected error for missing store")
	}
}

func TestEngine_Clear_NoStore(t *testing.T) {
	engine := NewEngine()

	err := engine.Clear(nil)
	if err == nil {
		t.Error("expected error for missing store")
	}
}

func TestEngine_Count_NoStore(t *testing.T) {
	engine := NewEngine()

	_, err := engine.Count(nil)
	if err == nil {
		t.Error("expected error for missing store")
	}
}

func TestPipeline(t *testing.T) {
	// Pipeline 需要各个组件，这里只测试结构创建
	// 完整的集成测试在 testing/integration 包中

	// 测试 nil 组件场景
	pipeline := NewPipeline(nil, nil, nil, nil)
	if pipeline == nil {
		t.Fatal("expected non-nil pipeline")
	}
}
