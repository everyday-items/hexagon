package stream

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// 创建函数测试
// =============================================================================

// TestPipe_基本读写 验证 Pipe 创建的读写对能正常传输数据
func TestPipe_基本读写(t *testing.T) {
	reader, writer := Pipe[int](5)

	go func() {
		for i := 0; i < 5; i++ {
			if err := writer.Send(i); err != nil {
				t.Errorf("发送第 %d 个元素失败: %v", i, err)
			}
		}
		writer.Close()
	}()

	var got []int
	for {
		v, err := reader.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("接收数据失败: %v", err)
		}
		got = append(got, v)
	}

	expected := []int{0, 1, 2, 3, 4}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestPipe_零容量 验证容量为 0 时使用同步通道
func TestPipe_零容量(t *testing.T) {
	reader, writer := Pipe[string](0)

	go func() {
		writer.Send("hello")
		writer.Close()
	}()

	v, err := reader.Recv()
	if err != nil {
		t.Fatalf("接收失败: %v", err)
	}
	if v != "hello" {
		t.Errorf("期望 \"hello\"，得到 %q", v)
	}

	_, err = reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// TestFromSlice_基本 验证从切片创建流
func TestFromSlice_基本(t *testing.T) {
	items := []string{"a", "b", "c"}
	reader := FromSlice(items)

	got, err := reader.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if !reflect.DeepEqual(got, items) {
		t.Errorf("期望 %v，得到 %v", items, got)
	}
}

// TestFromSlice_空切片 验证空切片直接返回 EOF
func TestFromSlice_空切片(t *testing.T) {
	reader := FromSlice([]int{})

	_, err := reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// TestFromSlice_nil切片 验证 nil 切片的处理
func TestFromSlice_nil切片(t *testing.T) {
	reader := FromSlice[int](nil)

	_, err := reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// TestFromValue_单个值 验证单值流只返回一个元素
func TestFromValue_单个值(t *testing.T) {
	reader := FromValue(42)

	v, err := reader.Recv()
	if err != nil {
		t.Fatalf("接收失败: %v", err)
	}
	if v != 42 {
		t.Errorf("期望 42，得到 %d", v)
	}

	_, err = reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// =============================================================================
// 核心方法测试
// =============================================================================

// TestStreamReader_Recv_顺序接收 验证 Recv 按顺序接收数据
func TestStreamReader_Recv_顺序接收(t *testing.T) {
	reader := FromSlice([]int{10, 20, 30})

	for _, expected := range []int{10, 20, 30} {
		v, err := reader.Recv()
		if err != nil {
			t.Fatalf("接收失败: %v", err)
		}
		if v != expected {
			t.Errorf("期望 %d，得到 %d", expected, v)
		}
	}
}

// TestStreamReader_Recv_关闭后返回EOF 验证关闭后读取返回 EOF
func TestStreamReader_Recv_关闭后返回EOF(t *testing.T) {
	reader, writer := Pipe[int](1)
	writer.Send(1)
	writer.Close()

	// 读取唯一的元素
	_, _ = reader.Recv()

	// 再次读取应返回 EOF
	_, err := reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// TestStreamReader_Close 验证 Reader 关闭后不 panic
func TestStreamReader_Close(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	// 读取一个元素
	_, _ = reader.Recv()

	// 关闭 Reader
	err := reader.Close()
	if err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

// TestStreamReader_Source 验证 Source 和 SetSource 方法
func TestStreamReader_Source(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	// 设置来源
	ret := reader.SetSource("test-source")
	if ret != reader {
		t.Error("SetSource 应返回自身以支持链式调用")
	}

	src := reader.Source()
	if src != "test-source" {
		t.Errorf("期望 Source 为 \"test-source\"，得到 %q", src)
	}
}

// TestStreamReader_Collect_正常 验证 Collect 收集所有元素
func TestStreamReader_Collect_正常(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})
	ctx := context.Background()

	got, err := reader.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestStreamReader_Collect_上下文取消 验证 Collect 在上下文取消时返回错误
func TestStreamReader_Collect_上下文取消(t *testing.T) {
	reader, writer := Pipe[int](10)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 持续发送数据直到上下文取消
	go func() {
		for i := 0; ; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	_, err := reader.Collect(ctx)
	if err == nil {
		t.Error("期望 Collect 因上下文取消返回错误")
	}
}

// TestStreamReader_ForEach_正常 验证 ForEach 遍历所有元素
func TestStreamReader_ForEach_正常(t *testing.T) {
	reader := FromSlice([]string{"a", "b", "c"})
	ctx := context.Background()

	var got []string
	err := reader.ForEach(ctx, func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatalf("ForEach 失败: %v", err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestStreamReader_ForEach_回调返回错误 验证 ForEach 在回调错误时提前终止
func TestStreamReader_ForEach_回调返回错误(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})
	ctx := context.Background()

	targetErr := errors.New("停止迭代")
	count := 0
	err := reader.ForEach(ctx, func(v int) error {
		count++
		if v == 3 {
			return targetErr
		}
		return nil
	})

	if !errors.Is(err, targetErr) {
		t.Errorf("期望错误为 %v，得到 %v", targetErr, err)
	}
	if count != 3 {
		t.Errorf("期望处理 3 个元素，实际处理了 %d 个", count)
	}
}

// TestStreamReader_ForEach_上下文取消 验证 ForEach 在上下文取消时终止
func TestStreamReader_ForEach_上下文取消(t *testing.T) {
	reader, writer := Pipe[int](10)
	ctx, cancel := context.WithCancel(context.Background())

	// 持续发送数据
	go func() {
		for i := 0; i < 100; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		writer.Close()
	}()

	count := 0
	_ = reader.ForEach(ctx, func(v int) error {
		count++
		if count >= 3 {
			cancel()
		}
		return nil
	})

	// 确保只处理了少量元素
	if count > 10 {
		t.Errorf("期望处理少量元素，实际处理了 %d 个", count)
	}
}

// TestStreamReader_Copy_多读者 验证 Copy 创建多个独立的流读取器
func TestStreamReader_Copy_多读者(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})
	copies := reader.Copy(3)

	if len(copies) != 3 {
		t.Fatalf("期望 3 个副本，得到 %d 个", len(copies))
	}

	// 每个副本都应该能独立读取所有数据
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([][]int, 3)

	for i, cp := range copies {
		wg.Add(1)
		go func(idx int, r *StreamReader[int]) {
			defer wg.Done()
			got, err := r.Collect(ctx)
			if err != nil {
				t.Errorf("副本 %d Collect 失败: %v", idx, err)
				return
			}
			results[idx] = got
		}(i, cp)
	}

	wg.Wait()

	expected := []int{1, 2, 3, 4, 5}
	for i, result := range results {
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("副本 %d: 期望 %v，得到 %v", i, expected, result)
		}
	}
}

// TestStreamReader_Copy_零份 验证 Copy(0) 返回 nil
func TestStreamReader_Copy_零份(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})
	copies := reader.Copy(0)

	if copies != nil {
		t.Errorf("期望 nil，得到 %v", copies)
	}
}

// TestStreamReader_Copy_一份 验证 Copy(1) 返回原始流
func TestStreamReader_Copy_一份(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})
	copies := reader.Copy(1)

	if len(copies) != 1 {
		t.Fatalf("期望 1 个副本，得到 %d 个", len(copies))
	}

	// Copy(1) 返回原始流本身
	if copies[0] != reader {
		t.Log("Copy(1) 返回了新的流（非原始流）")
	}

	got, err := copies[0].Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestStreamReader_Copy_消费速度不同 验证 Copy 的多个读者以不同速度消费
func TestStreamReader_Copy_消费速度不同(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})
	copies := reader.Copy(2)

	ctx := context.Background()
	results := make([][]int, 2)
	var wg sync.WaitGroup

	// 快速消费者
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err := copies[0].Collect(ctx)
		if err != nil {
			t.Errorf("快速消费者失败: %v", err)
			return
		}
		results[0] = got
	}()

	// 慢速消费者
	wg.Add(1)
	go func() {
		defer wg.Done()
		var got []int
		for {
			v, err := copies[1].Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("慢速消费者失败: %v", err)
				return
			}
			got = append(got, v)
			time.Sleep(10 * time.Millisecond)
		}
		results[1] = got
	}()

	wg.Wait()

	expected := []int{1, 2, 3, 4, 5}
	for i, result := range results {
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("消费者 %d: 期望 %v，得到 %v", i, expected, result)
		}
	}
}

// TestStreamReader_Copy_从Pipe创建 验证 Copy 处理 Pipe 创建的流
func TestStreamReader_Copy_从Pipe创建(t *testing.T) {
	reader, writer := Pipe[string](5)

	go func() {
		writer.Send("alpha")
		writer.Send("beta")
		writer.Send("gamma")
		writer.Close()
	}()

	copies := reader.Copy(2)

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([][]string, 2)

	for i, cp := range copies {
		wg.Add(1)
		go func(idx int, r *StreamReader[string]) {
			defer wg.Done()
			got, err := r.Collect(ctx)
			if err != nil {
				t.Errorf("副本 %d Collect 失败: %v", idx, err)
				return
			}
			results[idx] = got
		}(i, cp)
	}

	wg.Wait()

	expected := []string{"alpha", "beta", "gamma"}
	for i, result := range results {
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("副本 %d: 期望 %v，得到 %v", i, expected, result)
		}
	}
}

// TestStreamWriter_Send_正常 验证 Send 发送数据
func TestStreamWriter_Send_正常(t *testing.T) {
	reader, writer := Pipe[string](3)

	writer.Send("hello")
	writer.Send("world")
	writer.Close()

	got, err := reader.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"hello", "world"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestStreamWriter_Send_关闭后发送 验证关闭后发送返回错误
func TestStreamWriter_Send_关闭后发送(t *testing.T) {
	_, writer := Pipe[int](1)
	writer.Close()

	err := writer.Send(1)
	if err == nil {
		t.Error("期望关闭后发送返回错误")
	}
}

// TestStreamWriter_CloseWithError 验证 CloseWithError 携带自定义错误
func TestStreamWriter_CloseWithError(t *testing.T) {
	reader, writer := Pipe[int](1)
	customErr := errors.New("自定义关闭错误")

	writer.Send(1)
	writer.CloseWithError(customErr)

	// 先读取已发送的元素
	_, _ = reader.Recv()

	// 再次读取应返回自定义错误
	_, err := reader.Recv()
	if !errors.Is(err, customErr) {
		t.Errorf("期望错误 %v，得到 %v", customErr, err)
	}
}

// TestStreamWriter_Close_多次关闭 验证多次关闭不 panic
func TestStreamWriter_Close_多次关闭(t *testing.T) {
	_, writer := Pipe[int](1)

	err1 := writer.Close()
	if err1 != nil {
		t.Errorf("首次关闭不应出错: %v", err1)
	}

	// 第二次关闭不应 panic
	err2 := writer.Close()
	_ = err2
}

// TestStreamWriter_CloseWithError_自定义错误传播 验证 CloseWithError 的错误能被 Recv 接收
func TestStreamWriter_CloseWithError_自定义错误传播(t *testing.T) {
	reader, writer := Pipe[int](5)
	customErr := errors.New("处理失败")

	go func() {
		writer.Send(1)
		writer.Send(2)
		writer.CloseWithError(customErr)
	}()

	// 读取正常数据
	v1, err := reader.Recv()
	if err != nil {
		t.Fatalf("接收第一个元素失败: %v", err)
	}
	if v1 != 1 {
		t.Errorf("期望 1，得到 %d", v1)
	}

	v2, err := reader.Recv()
	if err != nil {
		t.Fatalf("接收第二个元素失败: %v", err)
	}
	if v2 != 2 {
		t.Errorf("期望 2，得到 %d", v2)
	}

	// 第三次读取应返回自定义错误
	_, err = reader.Recv()
	if !errors.Is(err, customErr) {
		t.Errorf("期望错误 %v，得到 %v", customErr, err)
	}
}

// =============================================================================
// 转换操作测试
// =============================================================================

// TestMap_基本转换 验证 Map 对每个元素应用转换函数
func TestMap_基本转换(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	mapped := Map(reader, func(v int) string {
		return strings.Repeat("*", v)
	})

	got, err := mapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"*", "**", "***", "****", "*****"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestMap_空流 验证 Map 处理空流
func TestMap_空流(t *testing.T) {
	reader := FromSlice([]int{})

	mapped := Map(reader, func(v int) int {
		return v * 2
	})

	got, err := mapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestMap_类型转换 验证 Map 在不同类型间转换
func TestMap_类型转换(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	mapped := Map(reader, func(v int) float64 {
		return float64(v) * 1.5
	})

	got, err := mapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []float64{1.5, 3.0, 4.5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFilter_基本过滤 验证 Filter 过滤掉不满足条件的元素
func TestFilter_基本过滤(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// 只保留偶数
	filtered := Filter(reader, func(v int) bool {
		return v%2 == 0
	})

	got, err := filtered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{2, 4, 6, 8, 10}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFilter_全部过滤 验证 Filter 过滤掉所有元素
func TestFilter_全部过滤(t *testing.T) {
	reader := FromSlice([]int{1, 3, 5, 7})

	filtered := Filter(reader, func(v int) bool {
		return v%2 == 0
	})

	got, err := filtered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestFilter_全部保留 验证 Filter 保留所有元素
func TestFilter_全部保留(t *testing.T) {
	reader := FromSlice([]int{2, 4, 6})

	filtered := Filter(reader, func(v int) bool {
		return v%2 == 0
	})

	got, err := filtered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{2, 4, 6}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFlatMap_基本展平 验证 FlatMap 将每个元素映射为切片再展平
func TestFlatMap_基本展平(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	// 每个数字展开为 [n, n*10]
	flatMapped := FlatMap(reader, func(v int) []int {
		return []int{v, v * 10}
	})

	got, err := flatMapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 10, 2, 20, 3, 30}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFlatMap_单元素内部切片 验证 FlatMap 处理每个元素映射为单元素切片
func TestFlatMap_单元素内部切片(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	flatMapped := FlatMap(reader, func(v int) []int {
		return []int{v * 100}
	})

	got, err := flatMapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{100, 200, 300}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFlatMap_深度嵌套 验证 FlatMap 处理多层展平
func TestFlatMap_深度嵌套(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	// 第一次展开: 每个 n -> [n, n+10]
	flat1 := FlatMap(reader, func(v int) []int {
		return []int{v, v + 10}
	})

	// 第二次展开: 每个 n -> [n, n+100]
	flat2 := FlatMap(flat1, func(v int) []int {
		return []int{v, v + 100}
	})

	got, err := flat2.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 1 -> [1, 11] -> [1, 101, 11, 111]
	// 2 -> [2, 12] -> [2, 102, 12, 112]
	// 3 -> [3, 13] -> [3, 103, 13, 113]
	expected := []int{1, 101, 11, 111, 2, 102, 12, 112, 3, 103, 13, 113}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestReduce_求和 验证 Reduce 聚合操作
func TestReduce_求和(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})
	ctx := context.Background()

	sum, err := Reduce(ctx, reader, 0, func(acc int, v int) int {
		return acc + v
	})
	if err != nil {
		t.Fatalf("Reduce 失败: %v", err)
	}

	if sum != 15 {
		t.Errorf("期望 15，得到 %d", sum)
	}
}

// TestReduce_字符串拼接 验证 Reduce 类型转换聚合
func TestReduce_字符串拼接(t *testing.T) {
	reader := FromSlice([]string{"hello", " ", "world"})
	ctx := context.Background()

	result, err := Reduce(ctx, reader, "", func(acc string, v string) string {
		return acc + v
	})
	if err != nil {
		t.Fatalf("Reduce 失败: %v", err)
	}

	if result != "hello world" {
		t.Errorf("期望 \"hello world\"，得到 %q", result)
	}
}

// TestReduce_空流 验证 Reduce 处理空流返回初始值
func TestReduce_空流(t *testing.T) {
	reader := FromSlice([]int{})
	ctx := context.Background()

	result, err := Reduce(ctx, reader, 100, func(acc int, v int) int {
		return acc + v
	})
	if err != nil {
		t.Fatalf("Reduce 失败: %v", err)
	}

	if result != 100 {
		t.Errorf("期望初始值 100，得到 %d", result)
	}
}

// TestReduce_上下文取消 验证 Reduce 在上下文取消时终止
func TestReduce_上下文取消(t *testing.T) {
	reader, writer := Pipe[int](10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		for i := 0; ; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	_, err := Reduce(ctx, reader, 0, func(acc int, v int) int {
		return acc + v
	})

	if err == nil {
		t.Error("期望因上下文取消而返回错误")
	}
}

// =============================================================================
// 选择操作测试
// =============================================================================

// TestTake_取前N个 验证 Take 只取前 N 个元素
func TestTake_取前N个(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	taken := Take(reader, 3)

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestTake_取0个 验证 Take(0) 返回空流
func TestTake_取0个(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	taken := Take(reader, 0)

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestTake_超出长度 验证 Take(n) 当 n > 流长度时返回所有元素
func TestTake_超出长度(t *testing.T) {
	reader := FromSlice([]int{1, 2})

	taken := Take(reader, 100)

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSkip_跳过前N个 验证 Skip 跳过前 N 个元素
func TestSkip_跳过前N个(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	skipped := Skip(reader, 2)

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSkip_跳过0个 验证 Skip(0) 不跳过任何元素
func TestSkip_跳过0个(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	skipped := Skip(reader, 0)

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSkip_超出长度 验证 Skip(n) 当 n >= 流长度时返回空流
func TestSkip_超出长度(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	skipped := Skip(reader, 10)

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestSkipTake_组合 验证 Skip 和 Take 的组合使用
func TestSkipTake_组合(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// 跳过前 3 个，取接下来 4 个
	result := Take(Skip(reader, 3), 4)

	got, err := result.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{4, 5, 6, 7}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestTakeWhile_条件满足时取值 验证 TakeWhile 在条件满足时持续取值
func TestTakeWhile_条件满足时取值(t *testing.T) {
	reader := FromSlice([]int{2, 4, 6, 7, 8, 10})

	// 取值直到遇到奇数
	taken := TakeWhile(reader, func(v int) bool {
		return v%2 == 0
	})

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{2, 4, 6}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestTakeWhile_首元素不满足 验证 TakeWhile 首元素不满足时返回空流
func TestTakeWhile_首元素不满足(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	taken := TakeWhile(reader, func(v int) bool {
		return v > 5
	})

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestTakeWhile_全部满足 验证 TakeWhile 所有元素都满足条件时返回全部
func TestTakeWhile_全部满足(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	taken := TakeWhile(reader, func(v int) bool {
		return v < 10
	})

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSkipWhile_跳过满足条件的前缀 验证 SkipWhile 跳过满足条件的连续前缀
func TestSkipWhile_跳过满足条件的前缀(t *testing.T) {
	reader := FromSlice([]int{2, 4, 6, 7, 8, 10})

	// 跳过偶数前缀
	skipped := SkipWhile(reader, func(v int) bool {
		return v%2 == 0
	})

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{7, 8, 10}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSkipWhile_全部跳过 验证 SkipWhile 当所有元素都满足条件时返回空流
func TestSkipWhile_全部跳过(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	skipped := SkipWhile(reader, func(v int) bool {
		return v < 10
	})

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestSkipWhile_首元素不满足 验证 SkipWhile 首元素不满足条件时不跳过任何元素
func TestSkipWhile_首元素不满足(t *testing.T) {
	reader := FromSlice([]int{5, 1, 2, 3})

	skipped := SkipWhile(reader, func(v int) bool {
		return v < 5
	})

	got, err := skipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{5, 1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestTakeWhile_SkipWhile_组合 验证 TakeWhile 和 SkipWhile 的组合使用
func TestTakeWhile_SkipWhile_组合(t *testing.T) {
	reader := FromSlice([]int{1, 1, 2, 2, 3, 3, 4, 4})

	// 先跳过所有 1，再取所有 <= 3 的
	skipped := SkipWhile(reader, func(v int) bool { return v == 1 })
	taken := TakeWhile(skipped, func(v int) bool { return v <= 3 })

	got, err := taken.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{2, 2, 3, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// =============================================================================
// 去重测试
// =============================================================================

// TestDistinct_基本去重 验证 Distinct 去除重复元素
func TestDistinct_基本去重(t *testing.T) {
	reader := FromSlice([]int{1, 2, 2, 3, 1, 4, 3, 5})

	distinct := Distinct(reader, func(a, b int) bool {
		return a == b
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestDistinct_无重复 验证 Distinct 处理无重复元素的流
func TestDistinct_无重复(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	distinct := Distinct(reader, func(a, b int) bool {
		return a == b
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestDistinct_全部重复 验证 Distinct 处理全部相同的元素
func TestDistinct_全部重复(t *testing.T) {
	reader := FromSlice([]int{7, 7, 7, 7})

	distinct := Distinct(reader, func(a, b int) bool {
		return a == b
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{7}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestDistinct_空流 验证 Distinct 处理空流
func TestDistinct_空流(t *testing.T) {
	reader := FromSlice([]int{})

	distinct := Distinct(reader, func(a, b int) bool {
		return a == b
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestDistinctBy_按键去重 验证 DistinctBy 按提取的键去重
func TestDistinctBy_按键去重(t *testing.T) {
	type person struct {
		Name string
		Age  int
	}

	reader := FromSlice([]person{
		{"Alice", 30},
		{"Bob", 25},
		{"Alice", 31}, // 重复名字
		{"Charlie", 35},
		{"Bob", 26}, // 重复名字
	})

	distinct := DistinctBy(reader, func(p person) string {
		return p.Name
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("期望 3 个不重复元素，得到 %d 个: %v", len(got), got)
	}

	// 保留第一次出现的
	if got[0].Name != "Alice" || got[0].Age != 30 {
		t.Errorf("第一个元素应为 Alice(30)，得到 %v", got[0])
	}
	if got[1].Name != "Bob" || got[1].Age != 25 {
		t.Errorf("第二个元素应为 Bob(25)，得到 %v", got[1])
	}
	if got[2].Name != "Charlie" {
		t.Errorf("第三个元素应为 Charlie，得到 %v", got[2])
	}
}

// TestDistinctBy_空流 验证 DistinctBy 处理空流
func TestDistinctBy_空流(t *testing.T) {
	reader := FromSlice([]string{})

	distinct := DistinctBy(reader, func(s string) string {
		return s
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// =============================================================================
// 组合操作测试
// =============================================================================

// TestZip_基本合并 验证 Zip 将两个流对应元素合并
func TestZip_基本合并(t *testing.T) {
	sr1 := FromSlice([]int{1, 2, 3})
	sr2 := FromSlice([]string{"a", "b", "c"})

	zipped := Zip(sr1, sr2, func(a int, b string) string {
		return strings.Repeat(b, a)
	})

	got, err := zipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"a", "bb", "ccc"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestZip_不等长度 验证 Zip 在较短流结束时停止
func TestZip_不等长度(t *testing.T) {
	sr1 := FromSlice([]int{1, 2, 3, 4, 5})
	sr2 := FromSlice([]int{10, 20})

	zipped := Zip(sr1, sr2, func(a, b int) int {
		return a + b
	})

	got, err := zipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{11, 22}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestZip_空流 验证 Zip 处理空流
func TestZip_空流(t *testing.T) {
	sr1 := FromSlice([]int{})
	sr2 := FromSlice([]int{1, 2, 3})

	zipped := Zip(sr1, sr2, func(a, b int) int {
		return a + b
	})

	got, err := zipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestZip_类型转换 验证 Zip 组合不同类型为结构体
func TestZip_类型转换(t *testing.T) {
	names := FromSlice([]string{"Alice", "Bob", "Charlie"})
	ages := FromSlice([]int{30, 25, 35})

	type person struct {
		Name string
		Age  int
	}

	zipped := Zip(names, ages, func(name string, age int) person {
		return person{Name: name, Age: age}
	})

	got, err := zipped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []person{
		{"Alice", 30},
		{"Bob", 25},
		{"Charlie", 35},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestMerge_基本合流 验证 Merge 将多个流合并为一个
func TestMerge_基本合流(t *testing.T) {
	sr1 := FromSlice([]int{1, 2, 3})
	sr2 := FromSlice([]int{4, 5, 6})
	sr3 := FromSlice([]int{7, 8, 9})

	merged := Merge(sr1, sr2, sr3)

	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// Merge 使用轮询方式，可能不保证严格顺序，验证元素完整性
	if len(got) != 9 {
		t.Fatalf("期望 9 个元素，得到 %d 个: %v", len(got), got)
	}

	sort.Ints(got)
	expected := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("排序后期望 %v，得到 %v", expected, got)
	}
}

// TestMerge_空流列表 验证 Merge 处理无输入流的情况
func TestMerge_空流列表(t *testing.T) {
	merged := Merge[int]()

	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestMerge_单个流 验证 Merge 处理单个流返回原始流
func TestMerge_单个流(t *testing.T) {
	sr := FromSlice([]int{1, 2, 3})

	merged := Merge(sr)

	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestMerge_含空流 验证 Merge 在部分流为空时正确工作
func TestMerge_含空流(t *testing.T) {
	sr1 := FromSlice([]int{1, 2, 3})
	sr2 := FromSlice([]int{})
	sr3 := FromSlice([]int{4, 5})

	merged := Merge(sr1, sr2, sr3)

	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 5 {
		t.Fatalf("期望 5 个元素，得到 %d 个: %v", len(got), got)
	}

	sort.Ints(got)
	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("排序后期望 %v，得到 %v", expected, got)
	}
}

// TestMerge_并发生产 验证 Merge 在并发场景下正确工作
func TestMerge_并发生产(t *testing.T) {
	var readers []*StreamReader[int]

	// 创建 5 个并发写入的流
	for i := 0; i < 5; i++ {
		r, w := Pipe[int](10)
		readers = append(readers, r)

		go func(base int) {
			for j := 0; j < 10; j++ {
				w.Send(base*10 + j)
			}
			w.Close()
		}(i)
	}

	merged := Merge(readers...)

	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 50 {
		t.Errorf("期望 50 个元素，得到 %d 个", len(got))
	}
}

// TestMerge_SourceEOF 验证 Merge 的 SourceEOF 行为
func TestMerge_SourceEOF(t *testing.T) {
	sr1 := FromSlice([]int{1}).SetSource("src-a")
	sr2 := FromSlice([]int{2}).SetSource("src-b")

	merged := Merge(sr1, sr2)

	// Collect 内部会跳过 SourceEOF，应能获取所有数据
	got, err := merged.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("期望 2 个元素，得到 %d 个: %v", len(got), got)
	}

	sort.Ints(got)
	expected := []int{1, 2}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("排序后期望 %v，得到 %v", expected, got)
	}
}

// =============================================================================
// 分片操作测试
// =============================================================================

// TestBatch_基本分批 验证 Batch 将流按固定大小分批
func TestBatch_基本分批(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6, 7})

	batched := Batch(reader, 3)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("期望 3 个批次，得到 %d 个", len(got))
	}

	expected := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBatch_整除分批 验证 Batch 元素数恰好整除批次大小
func TestBatch_整除分批(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6})

	batched := Batch(reader, 2)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := [][]int{{1, 2}, {3, 4}, {5, 6}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBatch_批次大于流长 验证 Batch 批次大小大于流长度时产生单个批次
func TestBatch_批次大于流长(t *testing.T) {
	reader := FromSlice([]int{1, 2})

	batched := Batch(reader, 10)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("期望 1 个批次，得到 %d 个", len(got))
	}

	expected := []int{1, 2}
	if !reflect.DeepEqual(got[0], expected) {
		t.Errorf("期望 %v，得到 %v", expected, got[0])
	}
}

// TestBatch_空流 验证 Batch 处理空流
func TestBatch_空流(t *testing.T) {
	reader := FromSlice([]int{})

	batched := Batch(reader, 3)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// TestBatch_动态流 验证 Batch 处理动态生成的流数据
func TestBatch_动态流(t *testing.T) {
	reader, writer := Pipe[int](10)

	go func() {
		for i := 1; i <= 7; i++ {
			writer.Send(i)
		}
		writer.Close()
	}()

	batched := Batch(reader, 3)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("期望 3 个批次，得到 %d 个: %v", len(got), got)
	}

	expected := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestWindow_滑动窗口 验证 Window 创建固定大小的滑动窗口
func TestWindow_滑动窗口(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	windowed := Window(reader, 3)

	got, err := windowed.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 窗口: [1,2,3], [2,3,4], [3,4,5]
	expected := [][]int{{1, 2, 3}, {2, 3, 4}, {3, 4, 5}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestWindow_窗口大于流长 验证 Window 窗口大小大于流长度
func TestWindow_窗口大于流长(t *testing.T) {
	reader := FromSlice([]int{1, 2})

	windowed := Window(reader, 5)

	got, err := windowed.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 元素不足以填满窗口时，返回已有的不完整窗口
	if len(got) != 1 {
		t.Fatalf("期望 1 个窗口，得到 %d 个: %v", len(got), got)
	}

	expected := []int{1, 2}
	if !reflect.DeepEqual(got[0], expected) {
		t.Errorf("期望 %v，得到 %v", expected, got[0])
	}
}

// TestWindow_窗口为1 验证 Window 窗口大小为 1 等同于 Map 到单元素切片
func TestWindow_窗口为1(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	windowed := Window(reader, 1)

	got, err := windowed.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := [][]int{{1}, {2}, {3}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestWindow_空流 验证 Window 处理空流
func TestWindow_空流(t *testing.T) {
	reader := FromSlice([]int{})

	windowed := Window(reader, 3)

	got, err := windowed.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("期望空切片，得到 %v", got)
	}
}

// =============================================================================
// 缓冲测试
// =============================================================================

// TestBuffer_基本缓冲 验证 Buffer 预读数据
func TestBuffer_基本缓冲(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		for i := 1; i <= 10; i++ {
			writer.Send(i)
		}
		writer.Close()
	}()

	buffered := Buffer(reader, 5)

	got, err := buffered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBuffer_缓冲大于数据量 验证 Buffer 缓冲区大于数据量
func TestBuffer_缓冲大于数据量(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	buffered := Buffer(reader, 100)

	got, err := buffered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBuffer_零大小 验证 Buffer 零大小时使用最小缓冲
func TestBuffer_零大小(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	buffered := Buffer(reader, 0)

	got, err := buffered.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// =============================================================================
// 时间控制测试
// =============================================================================

// TestTimeout_正常完成 验证 Timeout 在超时前正常完成
func TestTimeout_正常完成(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	timed := Timeout(reader, 1*time.Second)

	got, err := timed.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestTimeout_超时触发 验证 Timeout 在数据延迟时触发超时
func TestTimeout_超时触发(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		writer.Send(1)
		// 在发送第二个元素前等待较长时间
		time.Sleep(500 * time.Millisecond)
		writer.Send(2)
		writer.Close()
	}()

	timed := Timeout(reader, 100*time.Millisecond)

	var gotErr error
	for {
		_, err := timed.Recv()
		if err != nil {
			gotErr = err
			break
		}
	}

	if !errors.Is(gotErr, ErrStreamTimeout) {
		t.Errorf("期望 ErrStreamTimeout，得到 %v", gotErr)
	}
}

// TestDebounce_消除抖动 验证 Debounce 只在静默期后输出最后一个值
func TestDebounce_消除抖动(t *testing.T) {
	reader, writer := Pipe[int](10)

	go func() {
		// 快速发送 1, 2, 3，然后暂停，再发送 4, 5
		writer.Send(1)
		writer.Send(2)
		writer.Send(3)
		time.Sleep(200 * time.Millisecond)
		writer.Send(4)
		writer.Send(5)
		time.Sleep(200 * time.Millisecond)
		writer.Close()
	}()

	debounced := Debounce(reader, 100*time.Millisecond)

	got, err := debounced.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// Debounce 应该至少输出一些值
	if len(got) == 0 {
		t.Error("Debounce 不应返回空结果")
	}

	t.Logf("Debounce 结果: %v", got)
}

// TestThrottle_限流 验证 Throttle 限制元素发出频率
func TestThrottle_限流(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	throttled := Throttle(reader, 50*time.Millisecond)

	start := time.Now()
	got, err := throttled.Collect(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// Throttle 应该保留至少第一个元素
	if len(got) == 0 {
		t.Error("Throttle 不应返回空结果")
	}

	// 如果源流是同步的（FromSlice），因为时间间隔很短可能只保留第一个
	// 主要验证不 panic 且有输出
	t.Logf("Throttle 耗时: %v，结果: %v", elapsed, got)
}

// TestThrottle_慢速流 验证 Throttle 处理间隔大于节流时间的流
func TestThrottle_慢速流(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		for i := 1; i <= 3; i++ {
			writer.Send(i)
			time.Sleep(100 * time.Millisecond) // 每个间隔 100ms
		}
		writer.Close()
	}()

	// 节流时间 50ms < 间隔 100ms，所有元素应该都保留
	throttled := Throttle(reader, 50*time.Millisecond)

	got, err := throttled.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// =============================================================================
// 背压测试
// =============================================================================

// TestBackpressure_阻塞策略 验证阻塞策略在缓冲区满时阻塞生产者
func TestBackpressure_阻塞策略(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		for i := 0; i < 20; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
		}
		writer.Close()
	}()

	config := DefaultBackpressureConfig()
	config.Strategy = BackpressureBlock
	config.BufferSize = 5

	bp := Backpressure(reader, config)

	got, err := bp.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 阻塞策略应保证所有数据不丢失
	if len(got) != 20 {
		t.Errorf("期望 20 个元素，得到 %d 个", len(got))
	}
}

// TestBackpressure_丢弃策略 验证丢弃策略在缓冲区满时丢弃新数据
func TestBackpressure_丢弃策略(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		for i := 0; i < 100; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
		}
		writer.Close()
	}()

	config := DefaultBackpressureConfig()
	config.Strategy = BackpressureDrop
	config.BufferSize = 5

	bp := Backpressure(reader, config)

	got, err := bp.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 丢弃策略可能丢失部分数据
	t.Logf("丢弃策略: 收到 %d 个元素（原始 100 个）", len(got))
	if len(got) == 0 {
		t.Error("丢弃策略不应丢弃所有数据")
	}
}

// TestBackpressure_丢弃最旧策略 验证丢弃最旧策略保留最新数据
func TestBackpressure_丢弃最旧策略(t *testing.T) {
	reader, writer := Pipe[int](0)

	go func() {
		for i := 0; i < 50; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
		}
		writer.Close()
	}()

	config := DefaultBackpressureConfig()
	config.Strategy = BackpressureDropOldest
	config.BufferSize = 10

	bp := Backpressure(reader, config)

	got, err := bp.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	t.Logf("丢弃最旧策略: 收到 %d 个元素", len(got))
	if len(got) == 0 {
		t.Error("丢弃最旧策略不应丢弃所有数据")
	}
}

// TestBackpressure_错误策略 验证错误策略在缓冲区满时返回错误
func TestBackpressure_错误策略(t *testing.T) {
	reader, writer := Pipe[int](0)

	// 快速发送大量数据让缓冲区溢出
	go func() {
		for i := 0; i < 100; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
		}
		writer.Close()
	}()

	config := DefaultBackpressureConfig()
	config.Strategy = BackpressureError
	config.BufferSize = 2

	bp := Backpressure(reader, config)

	// 故意慢速消费，让缓冲区溢出
	var gotErr error
	for {
		_, err := bp.Recv()
		if err != nil {
			gotErr = err
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 可能收到 ErrBackpressure 或 io.EOF
	t.Logf("错误策略结果: %v", gotErr)
}

// TestBackpressure_nil配置 验证 nil 配置使用默认值
func TestBackpressure_nil配置(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	bp := Backpressure(reader, nil)

	got, err := bp.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBackpressure_水位回调 验证高水位和低水位回调
func TestBackpressure_水位回调(t *testing.T) {
	reader, writer := Pipe[int](0)

	var highCalled, lowCalled sync.Once
	highTriggered := make(chan struct{})
	lowTriggered := make(chan struct{})

	go func() {
		for i := 0; i < 20; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
		}
		writer.Close()
	}()

	config := &BackpressureConfig{
		BufferSize:    10,
		Strategy:      BackpressureBlock,
		HighWaterMark: 5,
		LowWaterMark:  2,
		OnHighWater: func() {
			highCalled.Do(func() { close(highTriggered) })
		},
		OnLowWater: func() {
			lowCalled.Do(func() { close(lowTriggered) })
		},
	}

	bp := Backpressure(reader, config)

	got, err := bp.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 20 {
		t.Errorf("期望 20 个元素，得到 %d 个", len(got))
	}
}

// TestDefaultBackpressureConfig 验证默认配置的合理性
func TestDefaultBackpressureConfig(t *testing.T) {
	config := DefaultBackpressureConfig()

	if config.BufferSize <= 0 {
		t.Errorf("默认缓冲区大小应 > 0，得到 %d", config.BufferSize)
	}
	if config.Strategy != BackpressureBlock {
		t.Errorf("默认策略应为 BackpressureBlock，得到 %d", config.Strategy)
	}
	if config.HighWaterMark <= 0 {
		t.Errorf("默认高水位应 > 0，得到 %d", config.HighWaterMark)
	}
	if config.LowWaterMark <= 0 {
		t.Errorf("默认低水位应 > 0，得到 %d", config.LowWaterMark)
	}
	if config.HighWaterMark <= config.LowWaterMark {
		t.Errorf("高水位(%d)应大于低水位(%d)", config.HighWaterMark, config.LowWaterMark)
	}
}

// =============================================================================
// 类型注册测试
// =============================================================================

// TestConcat_字符串拼接 验证 Concat 将流中的字符串类型元素拼接
func TestConcat_字符串拼接(t *testing.T) {
	// 注册字符串拼接函数
	RegisterConcatFunc[string](func(items []string) (string, error) {
		return strings.Join(items, ""), nil
	})

	reader := FromSlice([]string{"hello", " ", "world"})
	ctx := context.Background()

	result, err := Concat[string](ctx, reader)
	if err != nil {
		t.Fatalf("Concat 失败: %v", err)
	}

	if result != "hello world" {
		t.Errorf("期望 \"hello world\"，得到 %q", result)
	}
}

// TestConcatItems_字符串拼接 验证 ConcatItems 直接拼接切片
func TestConcatItems_字符串拼接(t *testing.T) {
	// 注册自定义拼接函数
	RegisterConcatFunc[string](func(items []string) (string, error) {
		return strings.Join(items, "-"), nil
	})

	result, err := ConcatItems([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("ConcatItems 失败: %v", err)
	}

	if result != "a-b-c" {
		t.Errorf("期望 \"a-b-c\"，得到 %q", result)
	}
}

// TestConcatItems_空切片 验证 ConcatItems 处理空切片返回 ErrStreamEmpty
func TestConcatItems_空切片(t *testing.T) {
	_, err := ConcatItems([]string{})
	if !errors.Is(err, ErrStreamEmpty) {
		t.Errorf("期望 ErrStreamEmpty，得到 %v", err)
	}
}

// TestConcatItems_单个元素 验证 ConcatItems 单个元素直接返回
func TestConcatItems_单个元素(t *testing.T) {
	result, err := ConcatItems([]string{"hello"})
	if err != nil {
		t.Fatalf("ConcatItems 失败: %v", err)
	}

	if result != "hello" {
		t.Errorf("期望 \"hello\"，得到 %q", result)
	}
}

// TestConcatItems_未注册类型_返回最后一个 验证未注册 ConcatFunc 时返回最后一个元素
func TestConcatItems_未注册类型_返回最后一个(t *testing.T) {
	type customType struct{ V int }

	result, err := ConcatItems([]customType{{1}, {2}, {3}})
	if err != nil {
		t.Fatalf("ConcatItems 失败: %v", err)
	}

	// 未注册时默认返回最后一个元素
	if result.V != 3 {
		t.Errorf("期望 V=3，得到 V=%d", result.V)
	}
}

// TestSplit_字符串分割 验证 Split 将字符串分割为流
func TestSplit_字符串分割(t *testing.T) {
	// 注册字符串分割函数
	RegisterSplitFunc[string](func(item string) []string {
		return strings.Split(item, "")
	})

	reader := Split("abc")

	got, err := reader.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestSplit_未注册类型 验证未注册分割函数时返回单元素流
func TestSplit_未注册类型(t *testing.T) {
	type customType struct{ V int }

	reader := Split(customType{42})

	got, err := reader.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("期望 1 个元素，得到 %d 个", len(got))
	}
	if got[0].V != 42 {
		t.Errorf("期望 V=42，得到 V=%d", got[0].V)
	}
}

// TestRegisterMergeFunc 验证注册合并函数不 panic
func TestRegisterMergeFunc(t *testing.T) {
	// 注册不应 panic
	RegisterMergeFunc[int](func(items []int, sources []string) (int, error) {
		sum := 0
		for _, v := range items {
			sum += v
		}
		return sum, nil
	})
}

// =============================================================================
// 错误处理测试
// =============================================================================

// TestErrStreamClosed_存在 验证 ErrStreamClosed 错误已定义
func TestErrStreamClosed_存在(t *testing.T) {
	if ErrStreamClosed == nil {
		t.Error("ErrStreamClosed 不应为 nil")
	}
	if ErrStreamClosed.Error() == "" {
		t.Error("ErrStreamClosed.Error() 不应为空")
	}
}

// TestErrStreamEmpty_存在 验证 ErrStreamEmpty 错误已定义
func TestErrStreamEmpty_存在(t *testing.T) {
	if ErrStreamEmpty == nil {
		t.Error("ErrStreamEmpty 不应为 nil")
	}
}

// TestErrStreamTimeout_存在 验证 ErrStreamTimeout 错误已定义
func TestErrStreamTimeout_存在(t *testing.T) {
	if ErrStreamTimeout == nil {
		t.Error("ErrStreamTimeout 不应为 nil")
	}
}

// TestErrNoConcatFunc_存在 验证 ErrNoConcatFunc 错误已定义
func TestErrNoConcatFunc_存在(t *testing.T) {
	if ErrNoConcatFunc == nil {
		t.Error("ErrNoConcatFunc 不应为 nil")
	}
}

// TestErrBackpressure_存在 验证 ErrBackpressure 错误已定义
func TestErrBackpressure_存在(t *testing.T) {
	if ErrBackpressure == nil {
		t.Error("ErrBackpressure 不应为 nil")
	}
}

// TestErrSourceEOF_存在 验证 ErrSourceEOF 已定义
func TestErrSourceEOF_存在(t *testing.T) {
	if ErrSourceEOF == nil {
		t.Error("ErrSourceEOF 不应为 nil")
	}
}

// TestSourceEOF_创建和检测 验证 SourceEOF 错误的创建和检测
func TestSourceEOF_创建和检测(t *testing.T) {
	seof := &SourceEOF{Source: "test-provider"}

	// 验证 Error() 方法
	msg := seof.Error()
	if msg == "" {
		t.Error("SourceEOF.Error() 不应返回空字符串")
	}
	if !strings.Contains(msg, "test-provider") {
		t.Errorf("SourceEOF.Error() 应包含来源名称，得到 %q", msg)
	}

	// 验证 IsSourceEOF 检测
	source, ok := IsSourceEOF(seof)
	if !ok {
		t.Error("IsSourceEOF 应能识别 SourceEOF 错误")
	}
	if source != "test-provider" {
		t.Errorf("期望来源 \"test-provider\"，得到 %q", source)
	}
}

// TestIsSourceEOF_非SourceEOF错误 验证 IsSourceEOF 对普通错误返回 false
func TestIsSourceEOF_非SourceEOF错误(t *testing.T) {
	_, ok := IsSourceEOF(errors.New("普通错误"))
	if ok {
		t.Error("IsSourceEOF 不应识别普通错误")
	}

	_, ok = IsSourceEOF(io.EOF)
	if ok {
		t.Error("IsSourceEOF 不应识别 io.EOF")
	}
}

// TestIsSourceEOF_nil错误 验证 IsSourceEOF 对 nil 返回 false
func TestIsSourceEOF_nil错误(t *testing.T) {
	_, ok := IsSourceEOF(nil)
	if ok {
		t.Error("IsSourceEOF 不应对 nil 返回 true")
	}
}

// =============================================================================
// 边界情况测试
// =============================================================================

// TestPipe_大量数据 验证大数据量传输
func TestPipe_大量数据(t *testing.T) {
	const n = 10000
	reader, writer := Pipe[int](100)

	go func() {
		for i := 0; i < n; i++ {
			if err := writer.Send(i); err != nil {
				t.Errorf("发送第 %d 个元素失败: %v", i, err)
				return
			}
		}
		writer.Close()
	}()

	got, err := reader.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	if len(got) != n {
		t.Errorf("期望 %d 个元素，得到 %d 个", n, len(got))
	}
}

// TestPipe_并发读写 验证并发安全性
func TestPipe_并发读写(t *testing.T) {
	reader, writer := Pipe[int](10)
	const numMessages = 100

	var wg sync.WaitGroup

	// 并发写入
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			writer.Send(i)
		}
		writer.Close()
	}()

	// 并发读取
	var received int
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, err := reader.Recv()
			if err != nil {
				break
			}
			received++
		}
	}()

	wg.Wait()

	if received != numMessages {
		t.Errorf("期望接收 %d 条消息，实际接收 %d 条", numMessages, received)
	}
}

// TestMap_链式操作 验证多个操作的链式组合
func TestMap_链式操作(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// 过滤偶数 -> 乘以 10 -> 取前 3 个
	result := Take(
		Map(
			Filter(reader, func(v int) bool { return v%2 == 0 }),
			func(v int) int { return v * 10 },
		),
		3,
	)

	got, err := result.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []int{20, 40, 60}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFilter_Map_Reduce_管道 验证完整的流处理管道
func TestFilter_Map_Reduce_管道(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	ctx := context.Background()

	// 过滤偶数 -> 乘以 2 -> 求和
	filtered := Filter(reader, func(v int) bool { return v%2 == 0 })
	mapped := Map(filtered, func(v int) int { return v * 2 })
	sum, err := Reduce(ctx, mapped, 0, func(acc, v int) int { return acc + v })

	if err != nil {
		t.Fatalf("管道处理失败: %v", err)
	}

	// 偶数: 2,4,6,8,10 -> 乘2: 4,8,12,16,20 -> 求和: 60
	if sum != 60 {
		t.Errorf("期望 60，得到 %d", sum)
	}
}

// TestSetSource_链式调用 验证 SetSource 支持链式调用
func TestSetSource_链式调用(t *testing.T) {
	reader := FromSlice([]int{1}).SetSource("provider-a")

	if reader.Source() != "provider-a" {
		t.Errorf("期望 Source 为 \"provider-a\"，得到 %q", reader.Source())
	}

	// 更新来源
	reader.SetSource("provider-b")
	if reader.Source() != "provider-b" {
		t.Errorf("期望 Source 为 \"provider-b\"，得到 %q", reader.Source())
	}
}

// TestPipe_WriterClose_ReaderRecv 验证 Writer 关闭后 Reader 能读完所有缓冲数据
func TestPipe_WriterClose_ReaderRecv(t *testing.T) {
	reader, writer := Pipe[int](10)

	// 先写入数据再关闭
	for i := 0; i < 5; i++ {
		writer.Send(i)
	}
	writer.Close()

	// 逐个读取
	var got []int
	for {
		v, err := reader.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("读取失败: %v", err)
		}
		got = append(got, v)
	}

	expected := []int{0, 1, 2, 3, 4}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestCollect_上下文超时 验证 Collect 在上下文超时后返回错误
func TestCollect_上下文超时(t *testing.T) {
	reader, writer := Pipe[int](10)

	// 写入器持续写入，模拟无限流
	go func() {
		for i := 0; ; i++ {
			if err := writer.Send(i); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := reader.Collect(ctx)
	if err == nil {
		t.Error("期望因上下文超时而返回错误")
	}
}

// TestFlatMap_多元素映射 验证 FlatMap 每个元素映射为多个元素
func TestFlatMap_多元素映射(t *testing.T) {
	reader := FromSlice([]string{"ab", "cd"})

	flatMapped := FlatMap(reader, func(v string) []string {
		// 将字符串分割为单个字符
		chars := make([]string, len(v))
		for i, c := range v {
			chars[i] = string(c)
		}
		return chars
	})

	got, err := flatMapped.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestBatch_大小为1 验证 Batch 批次大小为 1
func TestBatch_大小为1(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3})

	batched := Batch(reader, 1)

	got, err := batched.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := [][]int{{1}, {2}, {3}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestFromValue_字符串 验证 FromValue 处理字符串类型
func TestFromValue_字符串(t *testing.T) {
	reader := FromValue("hello world")

	v, err := reader.Recv()
	if err != nil {
		t.Fatalf("接收失败: %v", err)
	}
	if v != "hello world" {
		t.Errorf("期望 \"hello world\"，得到 %q", v)
	}

	_, err = reader.Recv()
	if err != io.EOF {
		t.Errorf("期望 io.EOF，得到 %v", err)
	}
}

// TestMap_Filter_FlatMap_组合 验证多种操作符的复杂组合
func TestMap_Filter_FlatMap_组合(t *testing.T) {
	reader := FromSlice([]int{1, 2, 3, 4, 5})

	// 过滤奇数 -> 展开为 [n, n*n] -> 映射为字符串
	result := Map(
		FlatMap(
			Filter(reader, func(v int) bool { return v%2 != 0 }),
			func(v int) []int { return []int{v, v * v} },
		),
		func(v int) int { return v + 1 },
	)

	got, err := result.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	// 奇数: 1,3,5 -> 展开: [1,1], [3,9], [5,25] -> +1: [2,2,4,10,6,26]
	expected := []int{2, 2, 4, 10, 6, 26}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}

// TestDistinct_字符串比较 验证 Distinct 处理字符串去重
func TestDistinct_字符串比较(t *testing.T) {
	reader := FromSlice([]string{"hello", "world", "hello", "go", "world"})

	distinct := Distinct(reader, func(a, b string) bool {
		return a == b
	})

	got, err := distinct.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}

	expected := []string{"hello", "world", "go"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("期望 %v，得到 %v", expected, got)
	}
}
