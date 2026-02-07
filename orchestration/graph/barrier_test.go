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

// ============== 补充边界测试 ==============

// TestBarrierNode_NilMerger 测试 nil 合并器的屏障节点
func TestBarrierNode_NilMerger(t *testing.T) {
	node := BarrierNode[TestState]("join", nil, "step_a")

	ctx := context.Background()
	state := TestState{Counter: 10}
	result, err := node.Handler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Counter != 10 {
		t.Errorf("expected counter 10, got %d", result.Counter)
	}

	// 检查元数据
	waitFor, ok := node.Metadata["__barrier_wait_for"]
	if !ok {
		t.Fatal("expected __barrier_wait_for in metadata")
	}
	wf := waitFor.([]string)
	if len(wf) != 1 || wf[0] != "step_a" {
		t.Errorf("unexpected waitFor: %v", wf)
	}
}

// TestAddBarrier_NoWaitFor 测试空 waitFor 的屏障
func TestAddBarrier_NoWaitFor(t *testing.T) {
	merger := func(original TestState, branchOutputs map[string]TestState) TestState {
		return original
	}

	_, err := NewGraph[TestState]("test").
		AddBarrier("join", merger).
		Build()

	if err == nil {
		t.Error("expected error for barrier with no waitFor nodes")
	}
}

// TestAddBarrier_PropagatesExistingError 测试已有错误时 AddBarrier 跳过
func TestAddBarrier_PropagatesExistingError(t *testing.T) {
	merger := func(original TestState, branchOutputs map[string]TestState) TestState {
		return original
	}

	// 使用保留名称触发错误
	builder := NewGraph[TestState]("test").
		AddNode(START, func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		})

	builder.AddBarrier("join", merger, "a", "b")

	_, err := builder.Build()
	if err == nil {
		t.Error("expected existing error to propagate")
	}
}

// TestMapReduceNode_EmptyShards 测试空分片
func TestMapReduceNode_EmptyShards(t *testing.T) {
	splitFn := func(s MapState) []MapState {
		return nil
	}
	mapFn := func(_ context.Context, s MapState) (MapState, error) {
		return s, nil
	}
	reduceFn := func(original MapState, results []MapState) MapState {
		return original
	}

	node := MapReduceNode[MapState]("mr", splitFn, mapFn, reduceFn, 0)

	ctx := context.Background()
	state := MapState{}
	state.Set("original", true)
	result, err := node.Handler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v, ok := result.Get("original"); !ok || v != true {
		t.Error("expected original state to be preserved with empty shards")
	}
}

// TestMapReduceNode_MapError 测试 map 阶段错误
func TestMapReduceNode_MapError(t *testing.T) {
	splitFn := func(s MapState) []MapState {
		return []MapState{MapState{}, MapState{}}
	}
	mapFn := func(_ context.Context, s MapState) (MapState, error) {
		return s, fmt.Errorf("map processing failed")
	}
	reduceFn := func(original MapState, results []MapState) MapState {
		return original
	}

	node := MapReduceNode[MapState]("mr", splitFn, mapFn, reduceFn, 0)

	ctx := context.Background()
	_, err := node.Handler(ctx, MapState{})
	if err == nil {
		t.Error("expected error from map function")
	}
}

// TestMapReduceNode_ContextCancel 测试 context 取消
func TestMapReduceNode_ContextCancel(t *testing.T) {
	splitFn := func(s MapState) []MapState {
		return make([]MapState, 5)
	}
	mapFn := func(ctx context.Context, s MapState) (MapState, error) {
		<-ctx.Done()
		return s, ctx.Err()
	}
	reduceFn := func(original MapState, results []MapState) MapState {
		return original
	}

	node := MapReduceNode[MapState]("mr", splitFn, mapFn, reduceFn, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := node.Handler(ctx, MapState{})
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

// TestFanOutFanInNode_EmptyBranches 测试空分支
func TestFanOutFanInNode_EmptyBranches(t *testing.T) {
	node := FanOutFanInNode[MapState]("fan", map[string]NodeHandler[MapState]{}, nil)

	ctx := context.Background()
	state := MapState{}
	state.Set("keep", "me")
	result, err := node.Handler(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Get("keep"); !ok || v != "me" {
		t.Error("expected original state with empty branches")
	}
}

// TestFanOutFanInNode_NilMerger 测试 nil 合并器
func TestFanOutFanInNode_NilMerger(t *testing.T) {
	branches := map[string]NodeHandler[MapState]{
		"only": func(_ context.Context, s MapState) (MapState, error) {
			s.Set("result", "done")
			return s, nil
		},
	}

	node := FanOutFanInNode[MapState]("fan", branches, nil)

	ctx := context.Background()
	result, err := node.Handler(ctx, MapState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil merger 返回任意分支结果
	if v, ok := result.Get("result"); !ok || v != "done" {
		t.Error("expected result from branch with nil merger")
	}
}

// TestFanOutFanInNode_BranchError 测试分支错误
func TestFanOutFanInNode_BranchError(t *testing.T) {
	branches := map[string]NodeHandler[MapState]{
		"good": func(_ context.Context, s MapState) (MapState, error) {
			return s, nil
		},
		"bad": func(_ context.Context, s MapState) (MapState, error) {
			return s, fmt.Errorf("branch failed")
		},
	}
	merger := func(original MapState, out map[string]MapState) MapState {
		return original
	}

	node := FanOutFanInNode[MapState]("fan", branches, merger)

	ctx := context.Background()
	_, err := node.Handler(ctx, MapState{})
	if err == nil {
		t.Error("expected error from failing branch")
	}
}

// TestAddMapReduce_PropagatesError 测试已有错误时 AddMapReduce 跳过
func TestAddMapReduce_PropagatesError(t *testing.T) {
	builder := NewGraph[TestState]("test").
		AddNode(START, func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		})

	builder.AddMapReduce("mr",
		func(s TestState) []TestState { return nil },
		func(ctx context.Context, s TestState) (TestState, error) { return s, nil },
		func(original TestState, results []TestState) TestState { return original },
		0,
	)

	_, err := builder.Build()
	if err == nil {
		t.Error("expected error to propagate")
	}
}

// TestAddFanOutFanIn_PropagatesError 测试已有错误时 AddFanOutFanIn 跳过
func TestAddFanOutFanIn_PropagatesError(t *testing.T) {
	builder := NewGraph[TestState]("test").
		AddNode(START, func(ctx context.Context, s TestState) (TestState, error) {
			return s, nil
		})

	builder.AddFanOutFanIn("fan", nil, nil)

	_, err := builder.Build()
	if err == nil {
		t.Error("expected error to propagate")
	}
}

// TestMapReduceNode_Metadata 测试 MapReduce 元数据
func TestMapReduceNode_Metadata(t *testing.T) {
	node := MapReduceNode[MapState]("mr",
		func(s MapState) []MapState { return nil },
		func(ctx context.Context, s MapState) (MapState, error) { return s, nil },
		func(original MapState, results []MapState) MapState { return original },
		4,
	)

	if node.Metadata["__map_reduce"] != true {
		t.Error("expected __map_reduce=true in metadata")
	}
	if node.Metadata["max_concurrency"] != 4 {
		t.Errorf("expected max_concurrency=4, got %v", node.Metadata["max_concurrency"])
	}
	if node.Type != NodeTypeParallel {
		t.Errorf("expected NodeTypeParallel, got %v", node.Type)
	}
}

// TestFanOutFanInNode_Metadata 测试 FanOutFanIn 元数据
func TestFanOutFanInNode_Metadata(t *testing.T) {
	branches := map[string]NodeHandler[MapState]{
		"a": func(_ context.Context, s MapState) (MapState, error) { return s, nil },
		"b": func(_ context.Context, s MapState) (MapState, error) { return s, nil },
	}

	node := FanOutFanInNode[MapState]("fan", branches, nil)

	if node.Metadata["__fan_out_fan_in"] != true {
		t.Error("expected __fan_out_fan_in=true in metadata")
	}
	if node.Metadata["branch_count"] != 2 {
		t.Errorf("expected branch_count=2, got %v", node.Metadata["branch_count"])
	}
}

// TestBarrierNode_Type 测试屏障节点类型
func TestBarrierNode_Type(t *testing.T) {
	node := BarrierNode[TestState]("join", nil, "a")
	if node.Type != NodeTypeBarrier {
		t.Errorf("expected NodeTypeBarrier(%d), got %d", NodeTypeBarrier, node.Type)
	}
}

// TestAddBarrier_EdgesCreated 测试屏障节点自动创建的边
func TestAddBarrier_EdgesCreated(t *testing.T) {
	merger := func(original MapState, branchOutputs map[string]MapState) MapState {
		return original
	}

	builder := NewGraph[MapState]("test").
		AddNode("step_a", func(_ context.Context, s MapState) (MapState, error) { return s, nil }).
		AddNode("step_b", func(_ context.Context, s MapState) (MapState, error) { return s, nil }).
		AddBarrier("join", merger, "step_a", "step_b").
		AddEdge(START, "step_a").
		AddEdge(START, "step_b").
		AddEdge("join", END)

	g, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 join 节点存在
	if _, ok := g.Nodes["join"]; !ok {
		t.Error("expected join node to exist")
	}
}
