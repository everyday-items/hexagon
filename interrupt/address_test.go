package interrupt

import (
	"context"
	"testing"
)

func TestAddressSegment_String(t *testing.T) {
	tests := []struct {
		name string
		seg  AddressSegment
		want string
	}{
		{
			name: "无 SubID",
			seg:  AddressSegment{Type: SegmentNode, ID: "step1"},
			want: "node:step1",
		},
		{
			name: "有 SubID",
			seg:  AddressSegment{Type: SegmentTool, ID: "search", SubID: "call_1"},
			want: "tool:search:call_1",
		},
		{
			name: "subgraph 类型",
			seg:  AddressSegment{Type: SegmentSubgraph, ID: "inner"},
			want: "subgraph:inner",
		},
		{
			name: "agent 类型",
			seg:  AddressSegment{Type: SegmentAgent, ID: "reviewer"},
			want: "agent:reviewer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.seg.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddressSegment_Equals(t *testing.T) {
	seg1 := AddressSegment{Type: SegmentNode, ID: "step1"}
	seg2 := AddressSegment{Type: SegmentNode, ID: "step1"}
	seg3 := AddressSegment{Type: SegmentNode, ID: "step2"}
	seg4 := AddressSegment{Type: SegmentTool, ID: "step1"}

	if !seg1.Equals(seg2) {
		t.Error("相同段应该相等")
	}
	if seg1.Equals(seg3) {
		t.Error("不同 ID 不应该相等")
	}
	if seg1.Equals(seg4) {
		t.Error("不同类型不应该相等")
	}
}

func TestAddress_String(t *testing.T) {
	tests := []struct {
		name string
		addr Address
		want string
	}{
		{
			name: "空地址",
			addr: Address{},
			want: "",
		},
		{
			name: "单段",
			addr: Address{{Type: SegmentNode, ID: "step1"}},
			want: "node:step1",
		},
		{
			name: "多段",
			addr: Address{
				{Type: SegmentNode, ID: "step1"},
				{Type: SegmentTool, ID: "search", SubID: "call_1"},
			},
			want: "node:step1;tool:search:call_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddress_Equals(t *testing.T) {
	addr1 := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "search"},
	}
	addr2 := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "search"},
	}
	addr3 := Address{
		{Type: SegmentNode, ID: "step1"},
	}
	addr4 := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "other"},
	}

	if !addr1.Equals(addr2) {
		t.Error("相同地址应该相等")
	}
	if addr1.Equals(addr3) {
		t.Error("不同长度的地址不应该相等")
	}
	if addr1.Equals(addr4) {
		t.Error("不同内容的地址不应该相等")
	}
	if addr1.Equals(nil) {
		t.Error("与 nil 比较不应该相等")
	}
}

func TestAddress_IsDescendantOf(t *testing.T) {
	parent := Address{
		{Type: SegmentNode, ID: "step1"},
	}
	child := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "search"},
	}
	grandchild := Address{
		{Type: SegmentNode, ID: "step1"},
		{Type: SegmentTool, ID: "search"},
		{Type: SegmentAgent, ID: "reviewer"},
	}
	unrelated := Address{
		{Type: SegmentNode, ID: "step2"},
		{Type: SegmentTool, ID: "search"},
	}

	if !child.IsDescendantOf(parent) {
		t.Error("child 应该是 parent 的后代")
	}
	if !grandchild.IsDescendantOf(parent) {
		t.Error("grandchild 应该是 parent 的后代")
	}
	if !grandchild.IsDescendantOf(child) {
		t.Error("grandchild 应该是 child 的后代")
	}
	if parent.IsDescendantOf(child) {
		t.Error("parent 不应该是 child 的后代")
	}
	if parent.IsDescendantOf(parent) {
		t.Error("相同地址不是后代关系")
	}
	if unrelated.IsDescendantOf(parent) {
		t.Error("不相关地址不是后代关系")
	}
}

func TestAddress_Append(t *testing.T) {
	original := Address{
		{Type: SegmentNode, ID: "step1"},
	}
	newSeg := AddressSegment{Type: SegmentTool, ID: "search"}
	result := original.Append(newSeg)

	// 验证结果
	if len(result) != 2 {
		t.Fatalf("Append 后长度应为 2, got %d", len(result))
	}
	if !result[1].Equals(newSeg) {
		t.Error("新追加的段不正确")
	}

	// 验证原地址未被修改
	if len(original) != 1 {
		t.Error("Append 不应修改原地址")
	}
}

func TestContext_AddressOperations(t *testing.T) {
	ctx := context.Background()

	// 初始状态：空地址
	addr := GetCurrentAddress(ctx)
	if len(addr) != 0 {
		t.Error("初始地址应为空")
	}

	// 追加第一段
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")
	addr = GetCurrentAddress(ctx)
	if len(addr) != 1 {
		t.Fatalf("第一次追加后应有 1 段, got %d", len(addr))
	}
	if addr[0].Type != SegmentNode || addr[0].ID != "step1" {
		t.Error("第一段不正确")
	}

	// 追加第二段
	ctx = AppendAddressSegment(ctx, SegmentTool, "search", "call_1")
	addr = GetCurrentAddress(ctx)
	if len(addr) != 2 {
		t.Fatalf("第二次追加后应有 2 段, got %d", len(addr))
	}
	if addr[1].Type != SegmentTool || addr[1].ID != "search" || addr[1].SubID != "call_1" {
		t.Error("第二段不正确")
	}

	// 地址字符串
	want := "node:step1;tool:search:call_1"
	if addr.String() != want {
		t.Errorf("地址字符串 = %q, want %q", addr.String(), want)
	}
}

func TestContext_AddressIsolation(t *testing.T) {
	ctx := context.Background()
	ctx = AppendAddressSegment(ctx, SegmentNode, "step1", "")

	// 分支1
	ctx1 := AppendAddressSegment(ctx, SegmentTool, "tool1", "")
	// 分支2
	ctx2 := AppendAddressSegment(ctx, SegmentTool, "tool2", "")

	addr1 := GetCurrentAddress(ctx1)
	addr2 := GetCurrentAddress(ctx2)

	if addr1.Equals(addr2) {
		t.Error("不同分支的地址不应该相等")
	}
	if addr1[1].ID != "tool1" {
		t.Error("分支1的工具 ID 不正确")
	}
	if addr2[1].ID != "tool2" {
		t.Error("分支2的工具 ID 不正确")
	}

	// 原始 context 的地址不受影响
	origAddr := GetCurrentAddress(ctx)
	if len(origAddr) != 1 {
		t.Error("原始 context 的地址不应被修改")
	}
}
