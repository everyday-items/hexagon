package interrupt

import (
	"context"
	"testing"
)

func TestResume_Basic(t *testing.T) {
	ctx := context.Background()

	// 设置恢复信息
	addr := Address{{Type: SegmentNode, ID: "step1"}}
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, nil)

	// 标记恢复
	ctx = Resume(ctx, "int-1")

	// 验证 globalResumeInfo 中有恢复数据
	gri := getGlobalResumeInfo(ctx)
	if gri == nil {
		t.Fatal("globalResumeInfo 不应为 nil")
	}

	if _, ok := gri.id2ResumeData["int-1"]; !ok {
		t.Error("int-1 应在 id2ResumeData 中")
	}
}

func TestResumeWithData(t *testing.T) {
	ctx := context.Background()

	addr := Address{{Type: SegmentNode, ID: "step1"}}
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, nil)

	type Approval struct {
		Approved bool
	}

	ctx = ResumeWithData(ctx, "int-1", Approval{Approved: true})

	gri := getGlobalResumeInfo(ctx)
	data, ok := gri.id2ResumeData["int-1"]
	if !ok {
		t.Fatal("int-1 应在 id2ResumeData 中")
	}

	approval, ok := data.(Approval)
	if !ok {
		t.Fatal("数据类型不正确")
	}
	if !approval.Approved {
		t.Error("Approved 应为 true")
	}
}

func TestBatchResumeWithData(t *testing.T) {
	ctx := context.Background()

	addr1 := Address{{Type: SegmentNode, ID: "step1"}}
	addr2 := Address{{Type: SegmentNode, ID: "step2"}}
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr1,
		"int-2": addr2,
	}, nil)

	ctx = BatchResumeWithData(ctx, map[string]any{
		"int-1": "data1",
		"int-2": "data2",
	})

	gri := getGlobalResumeInfo(ctx)
	if len(gri.id2ResumeData) != 2 {
		t.Fatalf("应有 2 个恢复数据, got %d", len(gri.id2ResumeData))
	}
}

func TestGetInterruptState(t *testing.T) {
	type Progress struct {
		LastIndex int
	}

	addr := Address{{Type: SegmentNode, ID: "batch"}}
	state := &Progress{LastIndex: 5}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, map[string]any{
		"int-1": state,
	})

	// 标记恢复
	ctx = Resume(ctx, "int-1")

	// 追加地址段（触发匹配）
	ctx = AppendAddressSegment(ctx, SegmentNode, "batch", "")

	wasInterrupted, hasState, progress := GetInterruptState[*Progress](ctx)
	if !wasInterrupted {
		t.Error("wasInterrupted 应为 true")
	}
	if !hasState {
		t.Error("hasState 应为 true")
	}
	if progress.LastIndex != 5 {
		t.Errorf("LastIndex = %d, want 5", progress.LastIndex)
	}
}

func TestGetInterruptState_TypeMismatch(t *testing.T) {
	addr := Address{{Type: SegmentNode, ID: "step1"}}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, map[string]any{
		"int-1": "string state",
	})
	ctx = Resume(ctx, "int-1")
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	wasInterrupted, hasState, _ := GetInterruptState[int](ctx)
	if !wasInterrupted {
		t.Error("wasInterrupted 应为 true（地址匹配了）")
	}
	if hasState {
		t.Error("hasState 应为 false（类型不匹配）")
	}
}

func TestGetInterruptState_NoInterrupt(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	wasInterrupted, hasState, _ := GetInterruptState[string](ctx)
	if wasInterrupted {
		t.Error("无中断时 wasInterrupted 应为 false")
	}
	if hasState {
		t.Error("无中断时 hasState 应为 false")
	}
}

func TestGetResumeContext(t *testing.T) {
	type Approval struct {
		Approved bool
		Comment  string
	}

	addr := Address{{Type: SegmentNode, ID: "review"}}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, nil)
	ctx = ResumeWithData(ctx, "int-1", Approval{Approved: true, Comment: "LGTM"})

	// 追加地址段触发匹配
	ctx = AppendAddressSegment(ctx, SegmentNode, "review", "")

	isTarget, hasData, data := GetResumeContext[Approval](ctx)
	if !isTarget {
		t.Error("isResumeTarget 应为 true")
	}
	if !hasData {
		t.Error("hasData 应为 true")
	}
	if !data.Approved {
		t.Error("Approved 应为 true")
	}
	if data.Comment != "LGTM" {
		t.Errorf("Comment = %q, want 'LGTM'", data.Comment)
	}
}

func TestGetResumeContext_NotTarget(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "other", "")

	isTarget, hasData, _ := GetResumeContext[string](ctx)
	if isTarget {
		t.Error("非恢复目标时 isResumeTarget 应为 false")
	}
	if hasData {
		t.Error("非恢复目标时 hasData 应为 false")
	}
}

func TestGetResumeContext_DescendantMatch(t *testing.T) {
	// 恢复目标是 step1 下的 tool:search
	targetAddr := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "search"},
	}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": targetAddr,
	}, nil)
	ctx = ResumeWithData(ctx, "int-1", "data")

	// 追加 step1（是目标的祖先）→ 应该标记 isResumeTarget
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	isTarget, hasData, _ := GetResumeContext[string](ctx)
	if !isTarget {
		t.Error("祖先节点应被标记为 isResumeTarget")
	}
	if hasData {
		t.Error("祖先节点不应有 hasData（数据属于后代）")
	}
}

func TestResume_WithoutPopulateInfo(t *testing.T) {
	ctx := context.Background()

	// 直接 Resume 而不先 PopulateResumeInfo
	ctx = Resume(ctx, "int-1")

	gri := getGlobalResumeInfo(ctx)
	if gri == nil {
		t.Fatal("应该自动创建 globalResumeInfo")
	}

	if _, ok := gri.id2ResumeData["int-1"]; !ok {
		t.Error("int-1 应在 id2ResumeData 中")
	}
}

func TestResumeWithData_WithoutPopulateInfo(t *testing.T) {
	ctx := context.Background()

	ctx = ResumeWithData(ctx, "int-1", "data")

	gri := getGlobalResumeInfo(ctx)
	if gri == nil {
		t.Fatal("应该自动创建 globalResumeInfo")
	}
}

func TestBatchResumeWithData_WithoutPopulateInfo(t *testing.T) {
	ctx := context.Background()

	ctx = BatchResumeWithData(ctx, map[string]any{"int-1": "data"})

	gri := getGlobalResumeInfo(ctx)
	if gri == nil {
		t.Fatal("应该自动创建 globalResumeInfo")
	}
}

func TestGetInterruptState_NilState(t *testing.T) {
	addr := Address{{Type: SegmentNode, ID: "step1"}}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, map[string]any{
		"int-1": nil,
	})
	ctx = Resume(ctx, "int-1")
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	wasInterrupted, hasState, _ := GetInterruptState[string](ctx)
	if !wasInterrupted {
		t.Error("wasInterrupted 应为 true")
	}
	if hasState {
		t.Error("nil state 时 hasState 应为 false")
	}
}

func TestGetResumeContext_NilData(t *testing.T) {
	addr := Address{{Type: SegmentNode, ID: "step1"}}

	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, nil)
	ctx = Resume(ctx, "int-1")
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	isTarget, hasData, _ := GetResumeContext[string](ctx)
	if !isTarget {
		t.Error("isResumeTarget 应为 true")
	}
	if hasData {
		t.Error("nil data 时 hasData 应为 false")
	}
}

func TestStateConsumedOnce(t *testing.T) {
	addr := Address{{Type: SegmentNode, ID: "step1"}}
	ctx := context.Background()
	ctx = PopulateResumeInfo(ctx, map[string]Address{
		"int-1": addr,
	}, map[string]any{
		"int-1": "state-data",
	})
	ctx = ResumeWithData(ctx, "int-1", "resume-data")

	// 第一次追加 → 消费状态
	ctx1 := AppendAddressSegment(ctx, SegmentNode, "step1", "")
	wasInterrupted1, hasState1, _ := GetInterruptState[string](ctx1)
	if !wasInterrupted1 || !hasState1 {
		t.Error("第一次应能获取状态")
	}

	// 第二次追加同样地址 → 状态已被消费
	ctx2 := AppendAddressSegment(ctx1, SegmentNode, "step1", "")
	_, hasState2, _ := GetInterruptState[string](ctx2)
	if hasState2 {
		t.Error("状态已被消费，第二次不应获取到")
	}
}
