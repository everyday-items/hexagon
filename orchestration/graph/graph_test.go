package graph

import (
	"context"
	"errors"
	"testing"
)

// TestState for graph tests
type TestState struct {
	Counter int
	Path    string
	Data    map[string]string
}

func (s TestState) Clone() State {
	data := make(map[string]string)
	for k, v := range s.Data {
		data[k] = v
	}
	return TestState{
		Counter: s.Counter,
		Path:    s.Path,
		Data:    data,
	}
}

func TestNewGraph(t *testing.T) {
	builder := NewGraph[TestState]("test-graph")

	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
}

func TestGraphAddNode(t *testing.T) {
	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if g == nil {
		t.Fatal("expected non-nil graph")
	}

	if g.Name != "test-graph" {
		t.Errorf("expected name 'test-graph', got '%s'", g.Name)
	}

	// Should have 3 nodes: START, step1, END
	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
	}
}

func TestGraphAddNodeReservedName(t *testing.T) {
	_, err := NewGraph[TestState]("test-graph").
		AddNode(START, func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		Build()

	if err == nil {
		t.Error("expected error for reserved node name")
	}
}

func TestGraphAddDuplicateNode(t *testing.T) {
	_, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		Build()

	if err == nil {
		t.Error("expected error for duplicate node")
	}
}

func TestGraphRun(t *testing.T) {
	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			s.Path += "1"
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			s.Path += "2"
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	result, err := g.Run(ctx, TestState{Counter: 0, Path: ""})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Counter != 2 {
		t.Errorf("expected counter 2, got %d", result.Counter)
	}

	if result.Path != "12" {
		t.Errorf("expected path '12', got '%s'", result.Path)
	}
}

func TestGraphRunWithError(t *testing.T) {
	expectedErr := errors.New("node failed")

	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, expectedErr
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, err = g.Run(ctx, TestState{})

	if err == nil {
		t.Error("expected error")
	}
}

func TestGraphRunWithCanceledContext(t *testing.T) {
	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = g.Run(ctx, TestState{})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestGraphConditionalEdge(t *testing.T) {
	g, err := NewGraph[TestState]("conditional-graph").
		AddNode("check", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddNode("path_a", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "A"
			return s, nil
		}).
		AddNode("path_b", func(ctx context.Context, s TestState) (TestState, error) {
			s.Path = "B"
			return s, nil
		}).
		AddEdge(START, "check").
		AddConditionalEdge("check", func(s TestState) string {
			if s.Counter%2 == 0 {
				return "even"
			}
			return "odd"
		}, map[string]string{
			"even": "path_a",
			"odd":  "path_b",
		}).
		AddEdge("path_a", END).
		AddEdge("path_b", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	// Test even path
	result, err := g.Run(ctx, TestState{Counter: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Path != "A" {
		t.Errorf("expected path 'A', got '%s'", result.Path)
	}

	// Test odd path
	result, err = g.Run(ctx, TestState{Counter: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Path != "B" {
		t.Errorf("expected path 'B', got '%s'", result.Path)
	}
}

func TestGraphStream(t *testing.T) {
	g, err := NewGraph[TestState]("stream-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	events, err := g.Stream(ctx, TestState{Counter: 0})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eventCount := 0
	var lastState TestState
	for event := range events {
		eventCount++
		if event.Type == EventTypeError {
			t.Fatalf("unexpected error event: %v", event.Error)
		}
		if event.Type == EventTypeEnd {
			lastState = event.State
		}
	}

	// Should have: 2x NodeStart + 2x NodeEnd + 1x End = 5 events
	if eventCount != 5 {
		t.Errorf("expected 5 events, got %d", eventCount)
	}

	if lastState.Counter != 2 {
		t.Errorf("expected counter 2, got %d", lastState.Counter)
	}
}

func TestGraphValidationMissingNode(t *testing.T) {
	_, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "nonexistent"). // This node doesn't exist
		Build()

	if err == nil {
		t.Error("expected validation error for missing node")
	}
}

func TestGraphMetadata(t *testing.T) {
	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		WithMetadata("version", "1.0").
		WithMetadata("author", "test").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if g.Metadata["version"] != "1.0" {
		t.Errorf("expected version '1.0', got '%v'", g.Metadata["version"])
	}

	if g.Metadata["author"] != "test" {
		t.Errorf("expected author 'test', got '%v'", g.Metadata["author"])
	}
}

func TestGraphMustBuild(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Error("expected no panic for valid graph")
		}
	}()

	g := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", END).
		MustBuild()

	if g == nil {
		t.Error("expected non-nil graph")
	}
}

func TestGraphMustBuildPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid graph")
		}
	}()

	NewGraph[TestState]("test-graph").
		AddNode(START, func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		}). // Invalid: using reserved name
		MustBuild()
}

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventTypeNodeStart, "node_start"},
		{EventTypeNodeEnd, "node_end"},
		{EventTypeError, "error"},
		{EventTypeEnd, "end"},
		{EventType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.eventType.String(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestGraphWithInterrupt(t *testing.T) {
	g, err := NewGraph[TestState]("test-graph").
		AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddNode("step2", func(ctx context.Context, s TestState) (TestState, error) {
			s.Counter++
			return s, nil
		}).
		AddEdge(START, "step1").
		AddEdge("step1", "step2").
		AddEdge("step2", END).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, err = g.Run(ctx, TestState{Counter: 0}, WithInterrupt("step2"))

	if err == nil {
		t.Error("expected interrupt error")
	}
}
