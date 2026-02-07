package graph

import (
	"context"
	"fmt"
	"testing"
)

// TestBarrierNode 测试屏障节点
func TestBarrierNode(t *testing.T) {
	ctx := context.Background()

	t.Run("基本屏障等待", func(t *testing.T) {
		handler1 := func(_ context.Context, s MapState) (MapState, error) {
			s.Set("h1", "done")
			return s, nil
		}
		handler2 := func(_ context.Context, s MapState) (MapState, error) {
			s.Set("h2", "done")
			return s, nil
		}
		merge := func(original MapState, results []MapState) MapState {
			merged := original
			for _, r := range results {
				for k, v := range r {
					merged[k] = v
				}
			}
			return merged
		}

		g, err := NewGraph[MapState]("barrier-test").
			AddNodeWithBuilder(ParallelNodeWithMerger[MapState](
				"parallel", merge, handler1, handler2,
			)).
			AddNode("final", func(_ context.Context, s MapState) (MapState, error) {
				s.Set("final", true)
				return s, nil
			}).
			AddEdge(START, "parallel").
			AddEdge("parallel", "final").
			AddEdge("final", END).
			Build()
		if err != nil {
			t.Fatal(err)
		}

		result, err := g.Run(ctx, MapState{})
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := result.Get("final"); !ok {
			t.Error("final 节点未执行")
		}
	})
}

// TestMapReduceNode 测试 Map-Reduce 节点
func TestMapReduceNode(t *testing.T) {
	ctx := context.Background()

	splitFn := func(s MapState) []MapState {
		items := []string{"a", "b", "c"}
		results := make([]MapState, len(items))
		for i, item := range items {
			state := MapState{}
			state.Set("item", item)
			results[i] = state
		}
		return results
	}

	mapFn := func(_ context.Context, s MapState) (MapState, error) {
		item, _ := s.Get("item")
		s.Set("processed", fmt.Sprintf("processed_%v", item))
		return s, nil
	}

	reduceFn := func(original MapState, results []MapState) MapState {
		merged := original
		var processed []string
		for _, r := range results {
			if v, ok := r.Get("processed"); ok {
				processed = append(processed, fmt.Sprintf("%v", v))
			}
		}
		merged.Set("all_processed", processed)
		return merged
	}

	g, err := NewGraph[MapState]("mapreduce-test").
		AddNodeWithBuilder(MapReduceNode[MapState](
			"mapreduce", splitFn, mapFn, reduceFn, 0,
		)).
		AddEdge(START, "mapreduce").
		AddEdge("mapreduce", END).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	allProcessed, ok := result.Get("all_processed")
	if !ok {
		t.Fatal("缺少 all_processed 字段")
	}

	items, ok := allProcessed.([]string)
	if !ok {
		t.Fatalf("all_processed 类型错误: %T", allProcessed)
	}

	if len(items) != 3 {
		t.Errorf("期望 3 个处理结果，实际 %d", len(items))
	}
}

// TestFanOutFanInNode 测试扇入扇出节点
func TestFanOutFanInNode(t *testing.T) {
	ctx := context.Background()

	branches := map[string]NodeHandler[MapState]{
		"a": func(_ context.Context, s MapState) (MapState, error) {
			s.Set("branch_a", "done")
			return s, nil
		},
		"b": func(_ context.Context, s MapState) (MapState, error) {
			s.Set("branch_b", "done")
			return s, nil
		},
		"c": func(_ context.Context, s MapState) (MapState, error) {
			s.Set("branch_c", "done")
			return s, nil
		},
	}

	merge := func(original MapState, outputs map[string]MapState) MapState {
		merged := original
		for _, s := range outputs {
			for k, v := range s {
				merged[k] = v
			}
		}
		return merged
	}

	g, err := NewGraph[MapState]("fanout-test").
		AddNodeWithBuilder(FanOutFanInNode[MapState](
			"fanout", branches, merge,
		)).
		AddEdge(START, "fanout").
		AddEdge("fanout", END).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"branch_a", "branch_b", "branch_c"} {
		if _, ok := result.Get(key); !ok {
			t.Errorf("缺少 %s", key)
		}
	}
}

// TestMapReduceWithConcurrency 测试带并发限制的 MapReduce
func TestMapReduceWithConcurrency(t *testing.T) {
	ctx := context.Background()

	splitFn := func(s MapState) []MapState {
		results := make([]MapState, 5)
		for i := range results {
			state := MapState{}
			state.Set("index", i)
			results[i] = state
		}
		return results
	}

	mapFn := func(_ context.Context, s MapState) (MapState, error) {
		idx, _ := s.Get("index")
		s.Set("result", fmt.Sprintf("done_%v", idx))
		return s, nil
	}

	reduceFn := func(original MapState, results []MapState) MapState {
		original.Set("count", len(results))
		return original
	}

	node := MapReduceNode[MapState]("mr", splitFn, mapFn, reduceFn, 2)

	g, err := NewGraph[MapState]("mr-test").
		AddNodeWithBuilder(node).
		AddEdge(START, "mr").
		AddEdge("mr", END).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Run(ctx, MapState{})
	if err != nil {
		t.Fatal(err)
	}

	count, ok := result.Get("count")
	if !ok || count != 5 {
		t.Errorf("期望 count=5，实际 %v", count)
	}
}
