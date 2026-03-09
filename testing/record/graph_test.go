// Package record 提供测试的录制和回放功能
//
// 本文件包含图执行录制和回放的全面测试：
//   - GraphCassette: 创建、添加、查找、保存和加载
//   - GraphRecorder: 节点执行录制
//   - GraphReplayer: 节点执行回放（包括多次执行同一节点的场景）
package record

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// GraphCassette 测试
// ============================================================================

// TestNewGraphCassette 测试创建图执行录制
func TestNewGraphCassette(t *testing.T) {
	c := NewGraphCassette("graph-test")

	if c.Name != "graph-test" {
		t.Errorf("期望名称为 'graph-test'，实际为 '%s'", c.Name)
	}

	if c.Len() != 0 {
		t.Errorf("期望初始交互数量为 0，实际为 %d", c.Len())
	}

	if c.Metadata == nil {
		t.Error("期望 Metadata 不为 nil")
	}

	if c.CreatedAt.IsZero() {
		t.Error("期望 CreatedAt 被设置")
	}
}

// TestGraphCassetteAddInteraction 测试添加交互
func TestGraphCassetteAddInteraction(t *testing.T) {
	c := NewGraphCassette("test")

	interaction := &GraphInteraction{
		ID:       "gi_1",
		NodeID:   "node_1",
		NodeName: "入口节点",
		Input:    map[string]any{"query": "hello"},
		Output:   map[string]any{"result": "world"},
		NextNode: "node_2",
		Duration: 50 * time.Millisecond,
	}

	c.AddInteraction(interaction)

	if c.Len() != 1 {
		t.Errorf("期望 1 条交互，实际为 %d", c.Len())
	}

	got, err := c.Get(0)
	if err != nil {
		t.Fatalf("获取交互失败: %v", err)
	}
	if got.NodeID != "node_1" {
		t.Errorf("期望节点 ID 为 'node_1'，实际为 '%s'", got.NodeID)
	}
	if got.NodeName != "入口节点" {
		t.Errorf("期望节点名为 '入口节点'")
	}
	if got.NextNode != "node_2" {
		t.Errorf("期望下一节点为 'node_2'")
	}
}

// TestGraphCassetteFindByNode 测试按节点 ID 查找
func TestGraphCassetteFindByNode(t *testing.T) {
	c := NewGraphCassette("test")

	c.AddInteraction(&GraphInteraction{NodeID: "node_1", NodeName: "A"})
	c.AddInteraction(&GraphInteraction{NodeID: "node_2", NodeName: "B"})
	c.AddInteraction(&GraphInteraction{NodeID: "node_1", NodeName: "A"}) // 同节点第二次执行

	// 查找 node_1 应该返回 2 条
	found := c.FindByNode("node_1")
	if len(found) != 2 {
		t.Errorf("期望找到 2 条 node_1 的记录，实际为 %d", len(found))
	}

	// 查找 node_2 应该返回 1 条
	found = c.FindByNode("node_2")
	if len(found) != 1 {
		t.Errorf("期望找到 1 条 node_2 的记录")
	}

	// 查找不存在的节点
	found = c.FindByNode("nonexistent")
	if len(found) != 0 {
		t.Errorf("期望找不到不存在的节点")
	}
}

// TestGraphCassetteFindByName 测试按节点名称查找
func TestGraphCassetteFindByName(t *testing.T) {
	c := NewGraphCassette("test")

	c.AddInteraction(&GraphInteraction{NodeID: "n1", NodeName: "处理器"})
	c.AddInteraction(&GraphInteraction{NodeID: "n2", NodeName: "输出器"})
	c.AddInteraction(&GraphInteraction{NodeID: "n3", NodeName: "处理器"})

	found := c.FindByName("处理器")
	if len(found) != 2 {
		t.Errorf("期望找到 2 条 '处理器' 的记录，实际为 %d", len(found))
	}
}

// TestGraphCassetteGetOutOfRange 测试越界访问
func TestGraphCassetteGetOutOfRange(t *testing.T) {
	c := NewGraphCassette("test")

	// 空 cassette 访问
	_, err := c.Get(0)
	if err == nil {
		t.Fatal("期望越界访问返回错误")
	}

	c.AddInteraction(&GraphInteraction{NodeID: "n1"})

	// 负索引
	_, err = c.Get(-1)
	if err == nil {
		t.Fatal("期望负索引返回错误")
	}

	// 超出范围
	_, err = c.Get(1)
	if err == nil {
		t.Fatal("期望超出范围返回错误")
	}

	// 正常索引
	_, err = c.Get(0)
	if err != nil {
		t.Fatalf("期望正常索引成功: %v", err)
	}
}

// TestGraphCassetteSaveAndLoad 测试保存和加载
func TestGraphCassetteSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "graph_cassette.json")

	original := NewGraphCassette("save-test")
	original.Metadata["version"] = "1.0"
	original.AddInteraction(&GraphInteraction{
		ID:        "gi_1",
		NodeID:    "start",
		NodeName:  "开始",
		Input:     map[string]any{"query": "test"},
		Output:    map[string]any{"result": "ok"},
		NextNode:  "end",
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
	})
	original.AddInteraction(&GraphInteraction{
		ID:       "gi_2",
		NodeID:   "end",
		NodeName: "结束",
		Output:   map[string]any{"final": true},
	})

	// 保存
	err := original.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("期望文件存在")
	}

	// 加载
	loaded, err := LoadGraphCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("期望名称为 'save-test'，实际为 '%s'", loaded.Name)
	}
	if len(loaded.Interactions) != 2 {
		t.Fatalf("期望 2 条交互，实际为 %d", len(loaded.Interactions))
	}
	if loaded.Interactions[0].NodeID != "start" {
		t.Errorf("期望第一条节点 ID 为 'start'")
	}
	if loaded.Interactions[1].NodeName != "结束" {
		t.Errorf("期望第二条节点名为 '结束'")
	}
}

// TestGraphCassetteSaveCreatesDir 测试保存时自动创建目录
func TestGraphCassetteSaveCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sub", "dir", "graph.json")

	c := NewGraphCassette("dir-test")
	err := c.Save(path)
	if err != nil {
		t.Fatalf("保存到嵌套目录失败: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("期望文件存在")
	}
}

// TestLoadGraphCassetteNotFound 测试加载不存在的文件
func TestLoadGraphCassetteNotFound(t *testing.T) {
	_, err := LoadGraphCassette("/nonexistent/path/graph.json")
	if err == nil {
		t.Fatal("期望加载不存在的文件返回错误")
	}
}

// TestLoadGraphCassetteInvalidJSON 测试加载无效 JSON
func TestLoadGraphCassetteInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)

	_, err := LoadGraphCassette(path)
	if err == nil {
		t.Fatal("期望加载无效 JSON 返回错误")
	}
}

// TestGraphCassetteWithError 测试带错误的交互记录
func TestGraphCassetteWithError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error_graph.json")

	c := NewGraphCassette("error-test")
	c.AddInteraction(&GraphInteraction{
		ID:       "gi_err",
		NodeID:   "failing_node",
		NodeName: "失败节点",
		Error:    "node execution failed",
	})

	err := c.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadGraphCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Interactions[0].Error != "node execution failed" {
		t.Errorf("期望错误信息为 'node execution failed'，实际为 '%s'", loaded.Interactions[0].Error)
	}
}

// ============================================================================
// GraphRecorder 测试
// ============================================================================

// TestNewGraphRecorder 测试创建图执行录制器
func TestNewGraphRecorder(t *testing.T) {
	c := NewGraphCassette("recorder-test")
	r := NewGraphRecorder(c)

	if r.Cassette() != c {
		t.Error("期望返回关联的 Cassette")
	}

	if c.Len() != 0 {
		t.Error("期望初始无交互记录")
	}
}

// TestGraphRecorderRecordNode 测试录制节点执行
func TestGraphRecorderRecordNode(t *testing.T) {
	c := NewGraphCassette("test")
	r := NewGraphRecorder(c)

	input := map[string]any{"query": "hello"}
	output := map[string]any{"result": "world"}

	r.RecordNode("node_1", "处理节点", input, output, "node_2", 50*time.Millisecond, nil)

	if c.Len() != 1 {
		t.Fatalf("期望 1 条交互，实际为 %d", c.Len())
	}

	got, _ := c.Get(0)
	if got.NodeID != "node_1" {
		t.Errorf("期望节点 ID 为 'node_1'")
	}
	if got.NodeName != "处理节点" {
		t.Errorf("期望节点名为 '处理节点'")
	}
	if got.NextNode != "node_2" {
		t.Errorf("期望下一节点为 'node_2'")
	}
	if got.Duration != 50*time.Millisecond {
		t.Errorf("期望耗时为 50ms")
	}
	if got.Error != "" {
		t.Errorf("期望无错误")
	}
	if got.ID == "" {
		t.Error("期望 ID 被自动生成")
	}
}

// TestGraphRecorderRecordNodeError 测试录制错误节点
func TestGraphRecorderRecordNodeError(t *testing.T) {
	c := NewGraphCassette("test")
	r := NewGraphRecorder(c)

	r.RecordNode("node_err", "失败节点", nil, nil, "", 10*time.Millisecond,
		errors.New("execution failed"))

	got, _ := c.Get(0)
	if got.Error != "execution failed" {
		t.Errorf("期望错误为 'execution failed'，实际为 '%s'", got.Error)
	}
}

// TestGraphRecorderMultipleRecords 测试录制多个节点
func TestGraphRecorderMultipleRecords(t *testing.T) {
	c := NewGraphCassette("test")
	r := NewGraphRecorder(c)

	r.RecordNode("n1", "A", nil, nil, "n2", time.Millisecond, nil)
	r.RecordNode("n2", "B", nil, nil, "n3", time.Millisecond, nil)
	r.RecordNode("n3", "C", nil, nil, "", time.Millisecond, nil)

	if c.Len() != 3 {
		t.Errorf("期望 3 条交互，实际为 %d", c.Len())
	}

	// 验证 ID 自增
	g0, _ := c.Get(0)
	g1, _ := c.Get(1)
	g2, _ := c.Get(2)
	if g0.ID == g1.ID || g1.ID == g2.ID {
		t.Error("期望每条交互有唯一 ID")
	}
}

// TestGraphRecorderSave 测试录制器保存
func TestGraphRecorderSave(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "recorder_save.json")

	c := NewGraphCassette("save")
	r := NewGraphRecorder(c)

	r.RecordNode("n1", "test", nil, nil, "", time.Millisecond, nil)

	err := r.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadGraphCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(loaded.Interactions) != 1 {
		t.Errorf("期望 1 条交互")
	}
}

// TestGraphRecorderConcurrency 测试录制器并发安全
func TestGraphRecorderConcurrency(t *testing.T) {
	c := NewGraphCassette("concurrent")
	r := NewGraphRecorder(c)

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.RecordNode("node", "test", nil, nil, "", time.Millisecond, nil)
		}(i)
	}

	wg.Wait()

	if c.Len() != goroutines {
		t.Errorf("期望 %d 条交互，实际为 %d", goroutines, c.Len())
	}
}

// ============================================================================
// GraphReplayer 测试
// ============================================================================

// TestNewGraphReplayer 测试创建回放器
func TestNewGraphReplayer(t *testing.T) {
	c := NewGraphCassette("test")
	c.AddInteraction(&GraphInteraction{
		NodeID: "n1",
		Output: map[string]any{"v": 1},
	})

	rp := NewGraphReplayer(c)
	if rp == nil {
		t.Fatal("期望回放器不为 nil")
	}
}

// TestGraphReplayerReplayNode 测试回放节点
func TestGraphReplayerReplayNode(t *testing.T) {
	c := NewGraphCassette("test")
	c.AddInteraction(&GraphInteraction{
		NodeID:   "n1",
		Output:   map[string]any{"result": "ok"},
		NextNode: "n2",
	})
	c.AddInteraction(&GraphInteraction{
		NodeID: "n2",
		Output: map[string]any{"final": true},
	})

	rp := NewGraphReplayer(c)

	// 回放 n1
	output, next, err := rp.ReplayNode("n1")
	if err != nil {
		t.Fatalf("回放 n1 失败: %v", err)
	}
	if output["result"] != "ok" {
		t.Errorf("期望输出 result=ok")
	}
	if next != "n2" {
		t.Errorf("期望下一节点为 'n2'，实际为 '%s'", next)
	}

	// 回放 n2
	output, next, err = rp.ReplayNode("n2")
	if err != nil {
		t.Fatalf("回放 n2 失败: %v", err)
	}
	if output["final"] != true {
		t.Errorf("期望输出 final=true")
	}
	if next != "" {
		t.Errorf("期望下一节点为空")
	}
}

// TestGraphReplayerMultipleVisits 测试同一节点多次回放
func TestGraphReplayerMultipleVisits(t *testing.T) {
	c := NewGraphCassette("test")

	// 同一节点执行两次（循环场景）
	c.AddInteraction(&GraphInteraction{
		NodeID: "loop",
		Output: map[string]any{"iteration": 1},
	})
	c.AddInteraction(&GraphInteraction{
		NodeID: "loop",
		Output: map[string]any{"iteration": 2},
	})

	rp := NewGraphReplayer(c)

	// 第一次回放
	output1, _, _ := rp.ReplayNode("loop")
	if output1["iteration"] != float64(1) && output1["iteration"] != 1 {
		t.Errorf("期望第一次迭代为 1，实际为 %v", output1["iteration"])
	}

	// 第二次回放
	output2, _, _ := rp.ReplayNode("loop")
	if output2["iteration"] != float64(2) && output2["iteration"] != 2 {
		t.Errorf("期望第二次迭代为 2，实际为 %v", output2["iteration"])
	}

	// 第三次应该报错
	_, _, err := rp.ReplayNode("loop")
	if err == nil {
		t.Fatal("期望超出录制次数时返回错误")
	}
}

// TestGraphReplayerNotFound 测试回放不存在的节点
func TestGraphReplayerNotFound(t *testing.T) {
	c := NewGraphCassette("test")
	rp := NewGraphReplayer(c)

	_, _, err := rp.ReplayNode("nonexistent")
	if err == nil {
		t.Fatal("期望回放不存在的节点返回错误")
	}
}

// TestGraphReplayerWithError 测试回放带错误的节点
func TestGraphReplayerWithError(t *testing.T) {
	c := NewGraphCassette("test")
	c.AddInteraction(&GraphInteraction{
		NodeID: "err_node",
		Error:  "something went wrong",
		Output: map[string]any{"partial": true},
	})

	rp := NewGraphReplayer(c)

	output, _, err := rp.ReplayNode("err_node")
	if err == nil {
		t.Fatal("期望回放带错误的节点返回错误")
	}
	if err.Error() != "something went wrong" {
		t.Errorf("期望错误信息为 'something went wrong'，实际为 '%s'", err.Error())
	}
	// 即使有错误，输出也应该存在
	if output["partial"] != true {
		t.Errorf("期望即使有错误也返回部分输出")
	}
}

// TestGraphReplayerReset 测试重置回放状态
func TestGraphReplayerReset(t *testing.T) {
	c := NewGraphCassette("test")
	c.AddInteraction(&GraphInteraction{
		NodeID: "n1",
		Output: map[string]any{"v": 1},
	})

	rp := NewGraphReplayer(c)

	// 第一次回放
	_, _, _ = rp.ReplayNode("n1")

	// 重置
	rp.Reset()

	// 重置后应该可以重新回放
	output, _, err := rp.ReplayNode("n1")
	if err != nil {
		t.Fatalf("重置后回放失败: %v", err)
	}
	if output["v"] != 1 {
		t.Errorf("期望重置后输出正确")
	}
}

// TestGraphRecorderAndReplayerRoundTrip 测试完整的录制-保存-加载-回放流程
func TestGraphRecorderAndReplayerRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.json")

	// 1. 录制
	c := NewGraphCassette("roundtrip")
	recorder := NewGraphRecorder(c)

	recorder.RecordNode("start", "开始", map[string]any{"q": "hello"},
		map[string]any{"r": "world"}, "process", 10*time.Millisecond, nil)
	recorder.RecordNode("process", "处理", map[string]any{"data": "world"},
		map[string]any{"result": "done"}, "", 20*time.Millisecond, nil)

	// 2. 保存
	err := recorder.Save(path)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 3. 加载
	loaded, err := LoadGraphCassette(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 4. 回放
	replayer := NewGraphReplayer(loaded)

	output1, next1, err := replayer.ReplayNode("start")
	if err != nil {
		t.Fatalf("回放 start 失败: %v", err)
	}
	if output1["r"] != "world" {
		t.Errorf("期望输出 r=world")
	}
	if next1 != "process" {
		t.Errorf("期望下一节点为 'process'")
	}

	output2, next2, err := replayer.ReplayNode("process")
	if err != nil {
		t.Fatalf("回放 process 失败: %v", err)
	}
	if output2["result"] != "done" {
		t.Errorf("期望输出 result=done")
	}
	if next2 != "" {
		t.Errorf("期望最后节点的下一节点为空")
	}
}
