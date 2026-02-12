// Package mcp 实现 Model Context Protocol (MCP) 支持
//
// 本文件实现 Stdio 传输层，用于与本地 MCP 进程通信
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/everyday-items/ai-core/tool"
)

// StdioTransport 标准输入输出传输层
//
// 通过 stdin/stdout 与本地 MCP 进程通信。
// 适用于 npx、uvx 等工具启动的本地 MCP 服务。
//
// 通信协议：
//   - 每行一个 JSON-RPC 消息（以换行符分隔）
//   - 请求发送到进程的 stdin
//   - 响应从进程的 stdout 读取
//
// 示例：
//
//	// 连接 filesystem MCP 服务
//	transport, cleanup, err := mcp.NewStdioTransport("npx", "-y",
//	    "@modelcontextprotocol/server-filesystem", "/tmp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
//
//	client := mcp.NewTransportClient(transport)
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	// 请求 ID 生成器
	nextID int64

	// 互斥锁保护 stdin 写入
	writeMu sync.Mutex

	// 响应通道映射（支持并发请求）
	pending   map[int64]chan *MCPResponse
	pendingMu sync.Mutex

	// 关闭标志
	closed atomic.Bool

	// 错误通道
	errChan chan error
}

// NewStdioTransport 创建 Stdio 传输层
//
// command 是要执行的命令，args 是命令参数
//
// 返回值：
//   - transport: Stdio 传输层实例
//   - cleanup: 清理函数，用于关闭进程和释放资源
//   - error: 错误信息
//
// 示例：
//
//	// 使用 npx 启动 MCP 服务
//	transport, cleanup, err := mcp.NewStdioTransport("npx", "-y",
//	    "@modelcontextprotocol/server-filesystem", "/tmp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
//
//	// 使用 uvx 启动 Python MCP 服务
//	transport, cleanup, err := mcp.NewStdioTransport("uvx", "mcp-server-fetch")
func NewStdioTransport(command string, args ...string) (*StdioTransport, func(), error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("创建 stdin 管道失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, nil, fmt.Errorf("启动进程失败: %w", err)
	}

	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
		pending: make(map[int64]chan *MCPResponse),
		errChan: make(chan error, 1),
	}

	// 启动响应读取协程
	go t.readResponses()

	// 启动 stderr 读取协程（用于调试）
	go t.readStderr()

	cleanup := func() {
		_ = t.Close()
	}

	return t, cleanup, nil
}

// Send 发送 MCP 请求
func (t *StdioTransport) Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("传输层已关闭")
	}

	// 分配请求 ID
	id := atomic.AddInt64(&t.nextID, 1)
	req.ID = id

	// 创建响应通道
	respChan := make(chan *MCPResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = respChan
	t.pendingMu.Unlock()

	// 确保清理响应通道
	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	// 序列化请求
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 发送请求（加换行符）
	t.writeMu.Lock()
	_, err = t.stdin.Write(append(reqBytes, '\n'))
	t.writeMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 等待响应
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-t.errChan:
		return nil, fmt.Errorf("读取响应失败: %w", err)
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP 错误 %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

// Close 关闭 Stdio 传输层
func (t *StdioTransport) Close() error {
	if t.closed.Swap(true) {
		return nil // 已经关闭
	}

	// 关闭 stdin（通知进程退出）
	if err := t.stdin.Close(); err != nil {
		// 忽略关闭错误
	}

	// 等待进程退出
	if err := t.cmd.Wait(); err != nil {
		// 进程可能被强制终止，忽略错误
	}

	return nil
}

// readResponses 读取响应协程
func (t *StdioTransport) readResponses() {
	for {
		if t.closed.Load() {
			return
		}

		// 读取一行
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			if !t.closed.Load() {
				select {
				case t.errChan <- err:
				default:
				}
			}
			return
		}

		// 解析响应
		var resp MCPResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// 忽略无效的 JSON 行（可能是日志输出）
			continue
		}

		// 查找对应的请求通道
		id, ok := resp.ID.(float64) // JSON 数字默认解析为 float64
		if !ok {
			continue
		}

		t.pendingMu.Lock()
		ch, exists := t.pending[int64(id)]
		t.pendingMu.Unlock()

		if exists {
			select {
			case ch <- &resp:
			default:
			}
		}
	}
}

// readStderr 读取 stderr 协程（用于调试）
func (t *StdioTransport) readStderr() {
	reader := bufio.NewReader(t.stderr)
	for {
		if t.closed.Load() {
			return
		}

		_, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		// 可以在这里记录 stderr 输出用于调试
		// fmt.Fprintf(os.Stderr, "[MCP stderr] %s", line)
	}
}

// ============== 便捷函数 ==============

// ConnectStdioServer 连接本地 MCP 进程并返回工具列表
//
// Deprecated: 请使用 ConnectStdioServerV2，基于官方 Go SDK 实现。
//
// 这是连接本地 MCP 服务的便捷函数。
// 返回的 cleanup 函数用于关闭进程和释放资源。
//
// 示例：
//
//	// 连接 filesystem MCP 服务
//	tools, cleanup, err := mcp.ConnectStdioServer(ctx, "npx", "-y",
//	    "@modelcontextprotocol/server-filesystem", "/tmp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
//
//	for _, t := range tools {
//	    fmt.Printf("Tool: %s - %s\n", t.Name(), t.Description())
//	}
func ConnectStdioServer(ctx context.Context, command string, args ...string) ([]tool.Tool, func(), error) {
	// 创建 Stdio 传输
	transport, cleanup, err := NewStdioTransport(command, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 Stdio 传输失败: %w", err)
	}

	// 创建客户端
	client := NewTransportClient(transport)

	// 初始化连接
	if err := client.Initialize(ctx); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("初始化 MCP 客户端失败: %w", err)
	}

	// 获取工具列表
	mcpTools, err := client.ListTools(ctx)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("获取工具列表失败: %w", err)
	}

	// 包装为 ai-core 工具
	tools := WrapMCPTools(client, mcpTools)

	// 增强 cleanup 函数，同时关闭 client
	enhancedCleanup := func() {
		_ = client.Close()
		cleanup()
	}

	return tools, enhancedCleanup, nil
}

// ConnectStdioServerWithToolSet 连接本地 MCP 进程并返回工具集合
//
// Deprecated: 请使用 ConnectStdioServerV2，基于官方 Go SDK 实现。
//
// 与 ConnectStdioServer 类似，但返回 MCPToolSet 以便管理生命周期
//
// 示例：
//
//	toolSet, cleanup, err := mcp.ConnectStdioServerWithToolSet(ctx,
//	    "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
//
//	agent := agent.New(
//	    agent.WithTools(toolSet.Tools()...),
//	)
func ConnectStdioServerWithToolSet(ctx context.Context, command string, args ...string) (*MCPToolSet, func(), error) {
	// 创建 Stdio 传输
	transport, cleanup, err := NewStdioTransport(command, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 Stdio 传输失败: %w", err)
	}

	// 创建客户端
	client := NewTransportClient(transport)

	// 初始化连接
	if err := client.Initialize(ctx); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("初始化 MCP 客户端失败: %w", err)
	}

	// 获取工具列表
	mcpTools, err := client.ListTools(ctx)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("获取工具列表失败: %w", err)
	}

	// 创建工具集合
	toolSet := NewMCPToolSet(client, mcpTools)

	// 增强 cleanup 函数
	enhancedCleanup := func() {
		_ = toolSet.Close()
		cleanup()
	}

	return toolSet, enhancedCleanup, nil
}
