package core

import (
	"context"
	"errors"
	"testing"
)

func TestNewSliceStream(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	stream := NewSliceStream(items)

	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
}

func TestSliceStreamNext(t *testing.T) {
	items := []string{"a", "b", "c"}
	stream := NewSliceStream(items)
	ctx := context.Background()

	// Read all items
	for i, expected := range items {
		val, ok := stream.Next(ctx)
		if !ok {
			t.Errorf("expected more items at index %d", i)
		}
		if val != expected {
			t.Errorf("expected %s, got %s", expected, val)
		}
	}

	// No more items
	_, ok := stream.Next(ctx)
	if ok {
		t.Error("expected no more items")
	}
}

func TestSliceStreamNextWithCanceledContext(t *testing.T) {
	items := []int{1, 2, 3}
	stream := NewSliceStream(items)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, ok := stream.Next(ctx)
	if ok {
		t.Error("expected false when context is canceled")
	}
}

func TestSliceStreamCollect(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	stream := NewSliceStream(items)
	ctx := context.Background()

	// Read first two items
	stream.Next(ctx)
	stream.Next(ctx)

	// Collect remaining
	remaining, err := stream.Collect(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []int{3, 4, 5}
	if len(remaining) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(remaining))
	}
	for i, v := range remaining {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestSliceStreamForEach(t *testing.T) {
	items := []int{1, 2, 3}
	stream := NewSliceStream(items)
	ctx := context.Background()

	sum := 0
	err := stream.ForEach(ctx, func(v int) error {
		sum += v
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestSliceStreamForEachWithError(t *testing.T) {
	items := []int{1, 2, 3}
	stream := NewSliceStream(items)
	ctx := context.Background()

	expectedErr := errors.New("test error")
	err := stream.ForEach(ctx, func(v int) error {
		if v == 2 {
			return expectedErr
		}
		return nil
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestSliceStreamForEachWithCanceledContext(t *testing.T) {
	items := []int{1, 2, 3}
	stream := NewSliceStream(items)

	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	err := stream.ForEach(ctx, func(v int) error {
		count++
		if count == 1 {
			cancel() // Cancel after first item
		}
		return nil
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSliceStreamErr(t *testing.T) {
	stream := NewSliceStream([]int{1, 2, 3})

	if stream.Err() != nil {
		t.Error("expected nil error")
	}
}

func TestSliceStreamClose(t *testing.T) {
	stream := NewSliceStream([]int{1, 2, 3})

	if err := stream.Close(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestBaseComponent(t *testing.T) {
	doubled := func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	}

	comp := NewBaseComponent("doubler", "Doubles a number", doubled)

	if comp.Name() != "doubler" {
		t.Errorf("expected name 'doubler', got '%s'", comp.Name())
	}

	if comp.Description() != "Doubles a number" {
		t.Errorf("expected description 'Doubles a number', got '%s'", comp.Description())
	}
}

func TestBaseComponentRun(t *testing.T) {
	doubled := func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	}

	comp := NewBaseComponent("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	result, err := comp.Run(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}
}

func TestBaseComponentRunWithError(t *testing.T) {
	expectedErr := errors.New("computation failed")
	failingFn := func(ctx context.Context, n int) (int, error) {
		return 0, expectedErr
	}

	comp := NewBaseComponent("failing", "A failing component", failingFn)
	ctx := context.Background()

	_, err := comp.Run(ctx, 5)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestBaseComponentStream(t *testing.T) {
	doubled := func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	}

	comp := NewBaseComponent("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	stream, err := comp.Stream(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := stream.Next(ctx)
	if !ok {
		t.Fatal("expected value from stream")
	}

	if val != 10 {
		t.Errorf("expected 10, got %d", val)
	}

	// No more items
	_, ok = stream.Next(ctx)
	if ok {
		t.Error("expected no more items")
	}
}

func TestBaseComponentBatch(t *testing.T) {
	doubled := func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	}

	comp := NewBaseComponent("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	results, err := comp.Batch(ctx, []int{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []int{2, 4, 6, 8, 10}
	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestBaseComponentBatchWithError(t *testing.T) {
	failAfter := 2
	count := 0
	fn := func(ctx context.Context, n int) (int, error) {
		count++
		if count > failAfter {
			return 0, errors.New("batch item failed")
		}
		return n * 2, nil
	}

	comp := NewBaseComponent("failing-batch", "Fails after 2", fn)
	ctx := context.Background()

	_, err := comp.Batch(ctx, []int{1, 2, 3, 4, 5})
	if err == nil {
		t.Error("expected error")
	}
}

func TestBaseComponentInputOutputSchema(t *testing.T) {
	type Input struct {
		Value int `json:"value"`
	}
	type Output struct {
		Result int `json:"result"`
	}

	fn := func(ctx context.Context, in Input) (Output, error) {
		return Output{Result: in.Value * 2}, nil
	}

	comp := NewBaseComponent("typed", "Typed component", fn)

	inputSchema := comp.InputSchema()
	if inputSchema == nil {
		t.Error("expected non-nil input schema")
	}

	outputSchema := comp.OutputSchema()
	if outputSchema == nil {
		t.Error("expected non-nil output schema")
	}
}

func TestSchemaOf(t *testing.T) {
	type Person struct {
		Name string `json:"name" desc:"Person's name"`
		Age  int    `json:"age" desc:"Person's age"`
	}

	schema := SchemaOf[Person]()
	if schema == nil {
		t.Error("expected non-nil schema")
	}
}
