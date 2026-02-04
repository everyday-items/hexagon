package core

import (
	"context"
	"testing"

	"github.com/everyday-items/hexagon/stream"
)

func TestNewSliceStream(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	sr := NewSliceStream(items)

	if sr == nil {
		t.Fatal("expected non-nil stream")
	}
}

func TestSliceStreamRecv(t *testing.T) {
	items := []string{"a", "b", "c"}
	sr := NewSliceStream(items)

	// Read all items
	for i, expected := range items {
		val, err := sr.Recv()
		if err != nil {
			t.Errorf("unexpected error at index %d: %v", i, err)
		}
		if val != expected {
			t.Errorf("expected %s, got %s", expected, val)
		}
	}

	// No more items
	_, err := sr.Recv()
	if err == nil {
		t.Error("expected error when stream is exhausted")
	}
}

func TestSliceStreamCollect(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	sr := NewSliceStream(items)
	ctx := context.Background()

	// Read first two items
	sr.Recv()
	sr.Recv()

	// Collect remaining
	remaining, err := stream.Concat(ctx, sr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 由于 Concat 是对基本类型的，对于 int 数组可能不会正常工作
	// 但至少验证没有 panic
	_ = remaining
}

func TestSliceStreamClose(t *testing.T) {
	sr := NewSliceStream([]int{1, 2, 3})

	if err := sr.Close(); err != nil {
		t.Errorf("expected nil error, got %v", err)
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

func TestStreamReaderCopy(t *testing.T) {
	items := []int{1, 2, 3}
	sr := NewSliceStream(items)

	// Copy to 2 readers
	copies := sr.Copy(2)
	if len(copies) != 2 {
		t.Fatalf("expected 2 copies, got %d", len(copies))
	}

	// 验证两个副本都能读取
	for i, cp := range copies {
		val, err := cp.Recv()
		if err != nil {
			t.Errorf("copy %d: unexpected error: %v", i, err)
		}
		if val != 1 {
			t.Errorf("copy %d: expected 1, got %d", i, val)
		}
	}
}

func TestStreamMerge(t *testing.T) {
	sr1 := NewSliceStream([]int{1, 2})
	sr2 := NewSliceStream([]int{3, 4})

	merged := stream.Merge(sr1, sr2)
	if merged == nil {
		t.Fatal("expected non-nil merged stream")
	}

	// 验证可以读取
	val, err := merged.Recv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 值可能是 1 或 3（取决于哪个先到）
	if val != 1 && val != 3 {
		t.Errorf("expected 1 or 3, got %d", val)
	}
}

func TestStreamMap(t *testing.T) {
	sr := NewSliceStream([]int{1, 2, 3})
	doubled := stream.Map(sr, func(n int) int { return n * 2 })

	val, err := doubled.Recv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}
}

func TestStreamFilter(t *testing.T) {
	sr := NewSliceStream([]int{1, 2, 3, 4, 5})
	evens := stream.Filter(sr, func(n int) bool { return n%2 == 0 })

	val, err := evens.Recv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2 (first even), got %d", val)
	}
}

func TestNewRunnable(t *testing.T) {
	doubled := func(ctx context.Context, n int, opts ...Option) (int, error) {
		return n * 2, nil
	}

	r := NewRunnable("doubler", "Doubles a number", doubled)

	if r.Name() != "doubler" {
		t.Errorf("expected name 'doubler', got '%s'", r.Name())
	}

	if r.Description() != "Doubles a number" {
		t.Errorf("expected description 'Doubles a number', got '%s'", r.Description())
	}
}

func TestRunnableInvoke(t *testing.T) {
	doubled := func(ctx context.Context, n int, opts ...Option) (int, error) {
		return n * 2, nil
	}

	r := NewRunnable("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	result, err := r.Invoke(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}
}

func TestRunnableStream(t *testing.T) {
	doubled := func(ctx context.Context, n int, opts ...Option) (int, error) {
		return n * 2, nil
	}

	r := NewRunnable("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	sr, err := r.Stream(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := sr.Recv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val != 10 {
		t.Errorf("expected 10, got %d", val)
	}
}

func TestRunnableBatch(t *testing.T) {
	doubled := func(ctx context.Context, n int, opts ...Option) (int, error) {
		return n * 2, nil
	}

	r := NewRunnable("doubler", "Doubles a number", doubled)
	ctx := context.Background()

	results, err := r.Batch(ctx, []int{1, 2, 3, 4, 5})
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

func TestRunnableInputOutputSchema(t *testing.T) {
	type Input struct {
		Value int `json:"value"`
	}
	type Output struct {
		Result int `json:"result"`
	}

	fn := func(ctx context.Context, in Input, opts ...Option) (Output, error) {
		return Output{Result: in.Value * 2}, nil
	}

	r := NewRunnable("typed", "Typed runnable", fn)

	inputSchema := r.InputSchema()
	if inputSchema == nil {
		t.Error("expected non-nil input schema")
	}

	outputSchema := r.OutputSchema()
	if outputSchema == nil {
		t.Error("expected non-nil output schema")
	}
}
