package interrupt

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestInterruptSignal_Error(t *testing.T) {
	signal := &InterruptSignal{
		ID:      "int-123",
		Address: Address{{Type: SegmentNode, ID: "step1"}},
		Info:    "需要审核",
		IsRoot:  true,
	}

	errStr := signal.Error()
	if errStr == "" {
		t.Error("Error() 不应为空")
	}

	// 确认包含关键信息
	want := "int-123"
	if !contains(errStr, want) {
		t.Errorf("Error() 应包含 ID %q, got %q", want, errStr)
	}
}

func TestInterruptSignal_NilError(t *testing.T) {
	var signal *InterruptSignal
	errStr := signal.Error()
	if errStr == "" {
		t.Error("nil 信号的 Error() 不应为空")
	}
}

func TestInterruptSignal_Unwrap(t *testing.T) {
	child := &InterruptSignal{
		ID:     "child-1",
		IsRoot: true,
	}
	parent := &InterruptSignal{
		ID:   "parent-1",
		Subs: []*InterruptSignal{child},
	}

	// 有子信号 → Unwrap 返回第一个
	unwrapped := parent.Unwrap()
	if unwrapped == nil {
		t.Fatal("Unwrap 不应返回 nil")
	}
	if unwrapped.(*InterruptSignal).ID != "child-1" {
		t.Error("Unwrap 应返回第一个子信号")
	}

	// 无子信号 → Unwrap 返回 nil
	leaf := &InterruptSignal{ID: "leaf-1", IsRoot: true}
	if leaf.Unwrap() != nil {
		t.Error("叶子节点 Unwrap 应返回 nil")
	}
}

func TestIsInterruptSignal(t *testing.T) {
	signal := &InterruptSignal{
		ID:   "int-123",
		Info: "test",
	}

	// 直接匹配
	s, ok := IsInterruptSignal(signal)
	if !ok || s.ID != "int-123" {
		t.Error("直接匹配失败")
	}

	// 包装后匹配
	wrapped := fmt.Errorf("wrapper: %w", signal)
	s, ok = IsInterruptSignal(wrapped)
	if !ok || s.ID != "int-123" {
		t.Error("包装后匹配失败")
	}

	// 普通错误不匹配
	_, ok = IsInterruptSignal(errors.New("normal error"))
	if ok {
		t.Error("普通错误不应匹配")
	}

	// nil 不匹配
	_, ok = IsInterruptSignal(nil)
	if ok {
		t.Error("nil 不应匹配")
	}
}

func TestInterruptSignalFunc(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "review", "")

	err := InterruptSignalFunc(ctx, "请审核此内容")

	signal, ok := IsInterruptSignal(err)
	if !ok {
		t.Fatal("应该能提取 InterruptSignal")
	}
	if signal.Info != "请审核此内容" {
		t.Errorf("Info = %v, want '请审核此内容'", signal.Info)
	}
	if !signal.IsRoot {
		t.Error("基础中断应为 IsRoot")
	}
	if signal.State != nil {
		t.Error("基础中断不应有 State")
	}
	if len(signal.Address) != 1 {
		t.Errorf("地址长度应为 1, got %d", len(signal.Address))
	}
	if signal.ID == "" {
		t.Error("ID 不应为空")
	}
}

func TestStatefulInterrupt(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "batch", "")

	type Progress struct {
		LastIndex int
	}

	err := StatefulInterrupt(ctx, "审核第 5 项", &Progress{LastIndex: 5})

	signal, ok := IsInterruptSignal(err)
	if !ok {
		t.Fatal("应该能提取 InterruptSignal")
	}
	if signal.Info != "审核第 5 项" {
		t.Errorf("Info = %v, want '审核第 5 项'", signal.Info)
	}
	if !signal.IsRoot {
		t.Error("有状态中断应为 IsRoot")
	}

	progress, ok := signal.State.(*Progress)
	if !ok {
		t.Fatal("State 类型应为 *Progress")
	}
	if progress.LastIndex != 5 {
		t.Errorf("LastIndex = %d, want 5", progress.LastIndex)
	}
}

func TestCompositeInterrupt(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "tools", "")

	// 创建子中断
	toolCtx1 := AppendAddressSegment(ctx, SegmentTool, "search", "call_1")
	sub1 := InterruptSignalFunc(toolCtx1, "搜索需要确认")

	toolCtx2 := AppendAddressSegment(ctx, SegmentTool, "delete", "call_2")
	sub2 := StatefulInterrupt(toolCtx2, "删除需要确认", map[string]string{"target": "file.txt"})

	// 创建组合中断（混入一个普通错误）
	normalErr := errors.New("这不是中断")
	err := CompositeInterrupt(ctx, "多个工具需要确认", nil, sub1, normalErr, sub2)

	signal, ok := IsInterruptSignal(err)
	if !ok {
		t.Fatal("应该能提取 InterruptSignal")
	}
	if signal.IsRoot {
		t.Error("组合中断不应为 IsRoot")
	}
	if len(signal.Subs) != 2 {
		t.Fatalf("应有 2 个子中断, got %d", len(signal.Subs))
	}
	if signal.Subs[0].Info != "搜索需要确认" {
		t.Error("第一个子中断 Info 不正确")
	}
	if signal.Subs[1].Info != "删除需要确认" {
		t.Error("第二个子中断 Info 不正确")
	}
}

func TestCompositeInterrupt_NoValidSubs(t *testing.T) {
	ctx := context.Background()

	// 没有有效子中断 → 降级为基础中断
	normalErr := errors.New("not an interrupt")
	err := CompositeInterrupt(ctx, "fallback", nil, normalErr)

	signal, ok := IsInterruptSignal(err)
	if !ok {
		t.Fatal("应该能提取 InterruptSignal")
	}
	if !signal.IsRoot {
		t.Error("没有有效子中断时应降级为 IsRoot")
	}
	if len(signal.Subs) != 0 {
		t.Error("不应有子中断")
	}
}

func TestInterruptSignal_ErrorsIs(t *testing.T) {
	child := &InterruptSignal{ID: "child", IsRoot: true}
	parent := &InterruptSignal{ID: "parent", Subs: []*InterruptSignal{child}}

	// errors.Is 应通过 Unwrap 链路匹配
	if !errors.Is(parent, child) {
		t.Error("errors.Is 应该能通过 Unwrap 匹配子信号")
	}
}

func TestInterruptSignal_ErrorsAs(t *testing.T) {
	signal := &InterruptSignal{ID: "test", Info: "info"}
	wrapped := fmt.Errorf("node failed: %w", signal)

	var extracted *InterruptSignal
	if !errors.As(wrapped, &extracted) {
		t.Fatal("errors.As 应该能提取 InterruptSignal")
	}
	if extracted.ID != "test" {
		t.Error("提取的 ID 不正确")
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
