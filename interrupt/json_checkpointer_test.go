package interrupt

import (
	"context"
	"testing"
)

func TestJSONCheckpointer_SaveLoadDelete(t *testing.T) {
	cp := NewJSONCheckpointer(t.TempDir())
	ctx := context.Background()

	err := cp.Save(ctx, &Checkpoint{
		ThreadID: "thread-1",
		NodeID:   "node-1",
		Payload:  map[string]any{"value": "x"},
		Status:   StatusInterrupted,
	})
	if err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	loaded, err := cp.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded checkpoint")
	}
	if loaded.ThreadID != "thread-1" {
		t.Fatalf("thread_id = %q, want %q", loaded.ThreadID, "thread-1")
	}
	if loaded.NodeID != "node-1" {
		t.Fatalf("node_id = %q, want %q", loaded.NodeID, "node-1")
	}
	if loaded.Version != 1 {
		t.Fatalf("version = %d, want 1", loaded.Version)
	}

	list, err := cp.List(ctx, "thread-1", 10)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}

	if err := cp.Delete(ctx, "thread-1"); err != nil {
		t.Fatalf("delete checkpoint: %v", err)
	}

	loaded, err = cp.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("load checkpoint after delete: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil checkpoint after delete")
	}
}

func TestJSONCheckpointer_SaveIncrementsVersion(t *testing.T) {
	cp := NewJSONCheckpointer(t.TempDir())
	ctx := context.Background()

	first := &Checkpoint{
		ThreadID: "thread-2",
		NodeID:   "node-1",
		Status:   StatusInterrupted,
	}
	if err := cp.Save(ctx, first); err != nil {
		t.Fatalf("save first checkpoint: %v", err)
	}

	second := &Checkpoint{
		ThreadID: "thread-2",
		NodeID:   "node-2",
		Status:   StatusResumed,
	}
	if err := cp.Save(ctx, second); err != nil {
		t.Fatalf("save second checkpoint: %v", err)
	}

	if second.Version != 2 {
		t.Fatalf("second version = %d, want 2", second.Version)
	}

	loaded, err := cp.Load(ctx, "thread-2")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded checkpoint")
	}
	if loaded.Version != 2 {
		t.Fatalf("loaded version = %d, want 2", loaded.Version)
	}
	if loaded.NodeID != "node-2" {
		t.Fatalf("loaded node_id = %q, want %q", loaded.NodeID, "node-2")
	}
}

func TestJSONCheckpointer_SaveValidation(t *testing.T) {
	cp := NewJSONCheckpointer(t.TempDir())
	ctx := context.Background()

	if err := cp.Save(ctx, nil); err == nil {
		t.Fatal("expected error when saving nil checkpoint")
	}

	err := cp.Save(ctx, &Checkpoint{
		NodeID: "node-1",
		Status: StatusInterrupted,
	})
	if err == nil {
		t.Fatal("expected error when thread_id is missing")
	}
}
