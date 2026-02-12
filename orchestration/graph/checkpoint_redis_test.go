package graph

import (
	"context"
	"errors"
	"testing"
)

func TestMgetValueToBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []byte
		wantErr bool
	}{
		{
			name:  "string",
			input: "hello",
			want:  []byte("hello"),
		},
		{
			name:  "bytes",
			input: []byte("world"),
			want:  []byte("world"),
		},
		{
			name:    "unsupported",
			input:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mgetValueToBytes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("期望返回错误, 实际为 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("不应返回错误: %v", err)
			}
			if string(got) != string(tt.want) {
				t.Fatalf("结果不匹配: 期望 %q, 实际 %q", string(tt.want), string(got))
			}
		})
	}
}

func TestRedisCheckpointSaver_NilClientValidation(t *testing.T) {
	saver := NewRedisCheckpointSaver(nil)
	ctx := context.Background()
	cp := newTestCheckpoint("thread-1", "graph", "node_a")

	if err := saver.Save(ctx, cp); err == nil {
		t.Fatal("Save 期望返回错误")
	}
	if _, err := saver.Load(ctx, "thread-1"); err == nil {
		t.Fatal("Load 期望返回错误")
	}
	if _, err := saver.LoadByID(ctx, "cp-1"); err == nil {
		t.Fatal("LoadByID 期望返回错误")
	}
	if _, err := saver.List(ctx, "thread-1"); err == nil {
		t.Fatal("List 期望返回错误")
	}
	if _, _, err := saver.LoadByThreadIDWithWarnings(ctx, "thread-1"); err == nil {
		t.Fatal("LoadByThreadIDWithWarnings 期望返回错误")
	}
	if err := saver.Delete(ctx, "cp-1"); err == nil {
		t.Fatal("Delete 期望返回错误")
	}
	if err := saver.DeleteThread(ctx, "thread-1"); err == nil {
		t.Fatal("DeleteThread 期望返回错误")
	}
	if _, err := saver.ListThreads(ctx, "", 10); err == nil {
		t.Fatal("ListThreads 期望返回错误")
	}
	if _, err := saver.GetCheckpointCount(ctx, "thread-1"); err == nil {
		t.Fatal("GetCheckpointCount 期望返回错误")
	}
	if err := saver.Prune(ctx, "thread-1", 1); err == nil {
		t.Fatal("Prune 期望返回错误")
	}
	if err := saver.Close(); err != nil {
		t.Fatalf("Close(nil client) 不应报错: %v", err)
	}
}

func TestRedisCheckpointSaver_ContextFirst(t *testing.T) {
	saver := NewRedisCheckpointSaver(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cp := newTestCheckpoint("thread-1", "graph", "node_a")
	err := saver.Save(ctx, cp)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望返回 context.Canceled, 实际 %v", err)
	}
}
