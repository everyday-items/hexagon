package graph

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHTTPNodeExecutor_Execute 测试 HTTP 远程执行器的正常执行流程
// 使用 httptest.NewServer 模拟远程节点服务，验证请求/响应的序列化和路由
func TestHTTPNodeExecutor_Execute(t *testing.T) {
	// 模拟远程节点服务，接收执行请求并返回处理后的状态
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和路径
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST 方法，实际收到 %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/api/v1/nodes/") {
			t.Errorf("期望路径包含 /api/v1/nodes/，实际为 %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("期望 Content-Type 为 application/json，实际为 %s", r.Header.Get("Content-Type"))
		}

		// 解析请求体
		// 注意: stateData 是 []byte 类型，JSON 序列化时会变成 base64 编码字符串
		var reqBody struct {
			NodeName  string `json:"node_name"`
			StateData string `json:"state_data"` // base64 编码的 []byte
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("解析请求体失败: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// 验证节点名称
		if reqBody.NodeName != "process" {
			t.Errorf("期望节点名称为 process，实际为 %s", reqBody.NodeName)
		}

		// 解码 base64 编码的状态数据，再反序列化为 TestState
		stateBytes, err := base64.StdEncoding.DecodeString(reqBody.StateData)
		if err != nil {
			t.Errorf("base64 解码状态数据失败: %v", err)
			http.Error(w, "bad encoding", http.StatusBadRequest)
			return
		}

		var state TestState
		if err := json.Unmarshal(stateBytes, &state); err != nil {
			t.Errorf("解析状态数据失败: %v", err)
			http.Error(w, "bad state", http.StatusBadRequest)
			return
		}

		// 模拟远程处理：递增计数器
		state.Counter += 100
		state.Path += "-remote"

		stateData, _ := json.Marshal(state)
		resp := map[string]any{
			"state_data": json.RawMessage(stateData),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 创建 HTTP 执行器，附带自定义请求头
	executor := NewHTTPNodeExecutor("gpu-node", server.URL,
		WithHTTPHeader("X-Auth-Token", "test-token-123"),
	)

	// 验证执行器名称
	if executor.Name() != "gpu-node" {
		t.Errorf("期望执行器名称为 gpu-node，实际为 %s", executor.Name())
	}

	// 准备输入状态
	inputState := TestState{Counter: 1, Path: "start"}
	stateData, err := json.Marshal(inputState)
	if err != nil {
		t.Fatalf("序列化输入状态失败: %v", err)
	}

	// 执行远程调用
	ctx := context.Background()
	resultData, err := executor.Execute(ctx, "process", stateData)
	if err != nil {
		t.Fatalf("远程执行失败: %v", err)
	}

	// 验证返回结果
	var resultState TestState
	if err := json.Unmarshal(resultData, &resultState); err != nil {
		t.Fatalf("反序列化结果失败: %v", err)
	}

	if resultState.Counter != 101 {
		t.Errorf("期望 Counter 为 101，实际为 %d", resultState.Counter)
	}
	if resultState.Path != "start-remote" {
		t.Errorf("期望 Path 为 'start-remote'，实际为 '%s'", resultState.Path)
	}
}

// TestHTTPNodeExecutor_Ping 测试 HTTP 执行器的健康检查功能
// 分别测试健康和不健康两种场景
func TestHTTPNodeExecutor_Ping(t *testing.T) {
	t.Run("健康节点", func(t *testing.T) {
		// 模拟正常运行的远程节点
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/health" {
				t.Errorf("期望健康检查路径为 /api/v1/health，实际为 %s", r.URL.Path)
			}
			if r.Method != http.MethodGet {
				t.Errorf("期望 GET 方法，实际为 %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		executor := NewHTTPNodeExecutor("healthy-node", server.URL)
		err := executor.Ping(context.Background())
		if err != nil {
			t.Errorf("健康节点的 Ping 不应返回错误，实际为: %v", err)
		}
	})

	t.Run("不健康节点", func(t *testing.T) {
		// 模拟返回错误状态码的远程节点
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		executor := NewHTTPNodeExecutor("unhealthy-node", server.URL)
		err := executor.Ping(context.Background())
		if err == nil {
			t.Error("不健康节点的 Ping 应该返回错误")
		}
		if !strings.Contains(err.Error(), "健康检查失败") {
			t.Errorf("错误信息应包含'健康检查失败'，实际为: %v", err)
		}
	})

	t.Run("不可达节点", func(t *testing.T) {
		// 使用无效地址模拟不可达节点
		executor := NewHTTPNodeExecutor("unreachable-node", "http://127.0.0.1:1")
		err := executor.Ping(context.Background())
		if err == nil {
			t.Error("不可达节点的 Ping 应该返回错误")
		}
		if !strings.Contains(err.Error(), "不可达") {
			t.Errorf("错误信息应包含'不可达'，实际为: %v", err)
		}
	})
}

// TestHTTPNodeExecutor_Timeout 测试 HTTP 执行器的超时处理
// 验证当远程节点响应超时时，执行器能正确返回超时错误
func TestHTTPNodeExecutor_Timeout(t *testing.T) {
	// 模拟一个慢速的远程节点（延迟 2 秒才响应）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 设置非常短的超时时间（100 毫秒）
	executor := NewHTTPNodeExecutor("slow-node", server.URL,
		WithHTTPTimeout(100*time.Millisecond),
	)

	stateData, _ := json.Marshal(TestState{Counter: 1})
	ctx := context.Background()

	_, err := executor.Execute(ctx, "slow-task", stateData)
	if err == nil {
		t.Fatal("超时场景应该返回错误")
	}

	// 验证错误信息包含远程执行失败的描述
	if !strings.Contains(err.Error(), "远程执行失败") {
		t.Errorf("期望错误信息包含'远程执行失败'，实际为: %v", err)
	}
}

// TestHTTPNodeExecutor_ServerError 测试 HTTP 执行器对服务端错误的处理
// 验证不同 HTTP 状态码和错误响应格式的处理
func TestHTTPNodeExecutor_ServerError(t *testing.T) {
	t.Run("500内部服务器错误", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		}))
		defer server.Close()

		executor := NewHTTPNodeExecutor("error-node", server.URL)
		stateData, _ := json.Marshal(TestState{Counter: 1})

		_, err := executor.Execute(context.Background(), "failing-task", stateData)
		if err == nil {
			t.Fatal("服务端 500 错误应该返回错误")
		}
		if !strings.Contains(err.Error(), "状态码 500") {
			t.Errorf("错误信息应包含'状态码 500'，实际为: %v", err)
		}
	})

	t.Run("远程节点返回业务错误", func(t *testing.T) {
		// 模拟远程节点返回 200 但包含业务错误
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"state_data": nil,
				"error":      "节点处理失败: 内存不足",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		executor := NewHTTPNodeExecutor("biz-error-node", server.URL)
		stateData, _ := json.Marshal(TestState{Counter: 1})

		_, err := executor.Execute(context.Background(), "biz-task", stateData)
		if err == nil {
			t.Fatal("业务错误应该返回错误")
		}
		if !strings.Contains(err.Error(), "内存不足") {
			t.Errorf("错误信息应包含'内存不足'，实际为: %v", err)
		}
	})

	t.Run("无效的JSON响应", func(t *testing.T) {
		// 模拟远程节点返回非法 JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("this is not json"))
		}))
		defer server.Close()

		executor := NewHTTPNodeExecutor("bad-json-node", server.URL)
		stateData, _ := json.Marshal(TestState{Counter: 1})

		_, err := executor.Execute(context.Background(), "bad-task", stateData)
		if err == nil {
			t.Fatal("无效 JSON 响应应该返回错误")
		}
		if !strings.Contains(err.Error(), "解析响应失败") {
			t.Errorf("错误信息应包含'解析响应失败'，实际为: %v", err)
		}
	})
}

// TestRemoteRegistry 测试远程执行器注册表的注册、获取功能
// 验证并发安全的注册表操作
func TestRemoteRegistry(t *testing.T) {
	registry := NewRemoteRegistry()

	t.Run("注册和获取执行器", func(t *testing.T) {
		// 创建两个模拟的远程执行器
		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server2.Close()

		exec1 := NewHTTPNodeExecutor("gpu-node", server1.URL)
		exec2 := NewHTTPNodeExecutor("cpu-node", server2.URL)

		// 注册执行器
		registry.Register("gpu", exec1)
		registry.Register("cpu", exec2)

		// 获取已注册的执行器
		got1, ok := registry.Get("gpu")
		if !ok {
			t.Fatal("期望能获取到 gpu 执行器")
		}
		if got1.Name() != "gpu-node" {
			t.Errorf("期望执行器名称为 gpu-node，实际为 %s", got1.Name())
		}

		got2, ok := registry.Get("cpu")
		if !ok {
			t.Fatal("期望能获取到 cpu 执行器")
		}
		if got2.Name() != "cpu-node" {
			t.Errorf("期望执行器名称为 cpu-node，实际为 %s", got2.Name())
		}
	})

	t.Run("获取不存在的执行器", func(t *testing.T) {
		_, ok := registry.Get("nonexistent")
		if ok {
			t.Error("不应该获取到不存在的执行器")
		}
	})

	t.Run("覆盖注册", func(t *testing.T) {
		// 使用相同名称注册新执行器，应覆盖旧的
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		newExec := NewHTTPNodeExecutor("gpu-node-v2", server.URL)
		registry.Register("gpu", newExec)

		got, ok := registry.Get("gpu")
		if !ok {
			t.Fatal("期望能获取到覆盖后的 gpu 执行器")
		}
		if got.Name() != "gpu-node-v2" {
			t.Errorf("期望执行器名称为 gpu-node-v2，实际为 %s", got.Name())
		}
	})
}

// TestRunDistributed_LocalFallback 测试分布式执行的本地降级逻辑
// 当远程执行器不可用或未注册时，应降级到本地执行
func TestRunDistributed_LocalFallback(t *testing.T) {
	t.Run("无远程配置时本地执行", func(t *testing.T) {
		// 构建一个普通图，不配置任何节点放置
		g, err := NewGraph[TestState]("local-graph").
			AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
				s.Counter += 10
				s.Path = "local"
				return s, nil
			}).
			AddEdge(START, "step1").
			AddEdge("step1", END).
			Build()

		if err != nil {
			t.Fatalf("构建图失败: %v", err)
		}

		registry := NewRemoteRegistry()
		ctx := context.Background()

		// 没有配置任何节点放置，所有节点应本地执行
		result, err := g.RunDistributed(ctx, TestState{Counter: 0}, registry)
		if err != nil {
			t.Fatalf("分布式执行失败: %v", err)
		}

		if result.Counter != 10 {
			t.Errorf("期望 Counter 为 10，实际为 %d", result.Counter)
		}
		if result.Path != "local" {
			t.Errorf("期望 Path 为 'local'，实际为 '%s'", result.Path)
		}
	})

	t.Run("执行器未注册时降级到本地", func(t *testing.T) {
		// 配置了节点放置但执行器未注册，应降级到本地执行
		g, err := NewGraph[TestState]("fallback-graph").
			AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
				s.Counter += 5
				s.Path = "fallback-local"
				return s, nil
			}).
			AddEdge(START, "step1").
			AddEdge("step1", END).
			WithNodePlacement("step1", "missing-executor"). // 执行器不存在
			Build()

		if err != nil {
			t.Fatalf("构建图失败: %v", err)
		}

		registry := NewRemoteRegistry()
		ctx := context.Background()

		// 执行器未注册，Fallback=true 应降级到本地
		result, err := g.RunDistributed(ctx, TestState{Counter: 0}, registry)
		if err != nil {
			t.Fatalf("降级执行不应失败: %v", err)
		}

		if result.Counter != 5 {
			t.Errorf("期望 Counter 为 5，实际为 %d", result.Counter)
		}
		if result.Path != "fallback-local" {
			t.Errorf("期望 Path 为 'fallback-local'，实际为 '%s'", result.Path)
		}
	})

	t.Run("不允许降级时执行器未注册应报错", func(t *testing.T) {
		// 配置 NoFallback，执行器未注册时应报错
		g, err := NewGraph[TestState]("no-fallback-graph").
			AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
				return s, nil
			}).
			AddEdge(START, "step1").
			AddEdge("step1", END).
			WithNodePlacementNoFallback("step1", "missing-executor").
			Build()

		if err != nil {
			t.Fatalf("构建图失败: %v", err)
		}

		registry := NewRemoteRegistry()
		ctx := context.Background()

		_, err = g.RunDistributed(ctx, TestState{Counter: 0}, registry)
		if err == nil {
			t.Fatal("不允许降级且执行器未注册时应返回错误")
		}
		if !strings.Contains(err.Error(), "未注册且不允许降级") {
			t.Errorf("错误信息应包含'未注册且不允许降级'，实际为: %v", err)
		}
	})

	t.Run("远程执行失败时降级到本地", func(t *testing.T) {
		// 模拟远程执行失败（返回 500），应降级到本地 handler
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/execute") {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("remote crashed"))
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		g, err := NewGraph[TestState]("remote-fail-graph").
			AddNode("step1", func(ctx context.Context, s TestState) (TestState, error) {
				s.Counter += 42
				s.Path = "local-fallback-after-remote-fail"
				return s, nil
			}).
			AddEdge(START, "step1").
			AddEdge("step1", END).
			WithNodePlacement("step1", "failing-remote").
			Build()

		if err != nil {
			t.Fatalf("构建图失败: %v", err)
		}

		registry := NewRemoteRegistry()
		executor := NewHTTPNodeExecutor("failing-remote", server.URL)
		registry.Register("failing-remote", executor)

		ctx := context.Background()
		result, err := g.RunDistributed(ctx, TestState{Counter: 0}, registry)
		if err != nil {
			t.Fatalf("降级执行不应失败: %v", err)
		}

		// 应该降级到本地 handler，Counter 为 42
		if result.Counter != 42 {
			t.Errorf("期望 Counter 为 42（本地降级），实际为 %d", result.Counter)
		}
		if result.Path != "local-fallback-after-remote-fail" {
			t.Errorf("期望 Path 为 'local-fallback-after-remote-fail'，实际为 '%s'", result.Path)
		}
	})
}

// TestHealthCheck 测试注册表的批量健康检查功能
// 验证并发检查多个远程节点的健康状态
func TestHealthCheck(t *testing.T) {
	// 创建三个模拟服务器：两个健康，一个不健康
	healthyServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer healthyServer1.Close()

	healthyServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer healthyServer2.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthyServer.Close()

	registry := NewRemoteRegistry()
	registry.Register("gpu-1", NewHTTPNodeExecutor("gpu-1", healthyServer1.URL))
	registry.Register("gpu-2", NewHTTPNodeExecutor("gpu-2", healthyServer2.URL))
	registry.Register("cpu-1", NewHTTPNodeExecutor("cpu-1", unhealthyServer.URL))

	ctx := context.Background()
	results := registry.HealthCheck(ctx)

	// 验证返回了所有注册的执行器的检查结果
	if len(results) != 3 {
		t.Fatalf("期望 3 个健康检查结果，实际为 %d", len(results))
	}

	// 两个健康节点应该无错误
	if results["gpu-1"] != nil {
		t.Errorf("gpu-1 应该健康，实际错误: %v", results["gpu-1"])
	}
	if results["gpu-2"] != nil {
		t.Errorf("gpu-2 应该健康，实际错误: %v", results["gpu-2"])
	}

	// 不健康节点应该有错误
	if results["cpu-1"] == nil {
		t.Error("cpu-1 不健康，应该返回错误")
	}

	t.Run("空注册表的健康检查", func(t *testing.T) {
		emptyRegistry := NewRemoteRegistry()
		emptyResults := emptyRegistry.HealthCheck(ctx)
		if len(emptyResults) != 0 {
			t.Errorf("空注册表的健康检查结果应为空，实际有 %d 个", len(emptyResults))
		}
	})

	t.Run("带超时的健康检查", func(t *testing.T) {
		// 模拟慢速服务器
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer slowServer.Close()

		timeoutRegistry := NewRemoteRegistry()
		timeoutRegistry.Register("slow-node", NewHTTPNodeExecutor(
			"slow-node", slowServer.URL,
			WithHTTPTimeout(100*time.Millisecond),
		))

		// 使用带超时的 context
		timeoutCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		timeoutResults := timeoutRegistry.HealthCheck(timeoutCtx)
		if len(timeoutResults) != 1 {
			t.Fatalf("期望 1 个检查结果，实际为 %d", len(timeoutResults))
		}
		if timeoutResults["slow-node"] == nil {
			t.Error("慢速节点应该因超时返回错误")
		}
	})
}

// TestHTTPNodeExecutor_CustomHeaders 测试自定义请求头是否正确传递
func TestHTTPNodeExecutor_CustomHeaders(t *testing.T) {
	// 模拟服务器验证自定义请求头
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查自定义请求头
		authToken := r.Header.Get("X-Auth-Token")
		traceID := r.Header.Get("X-Trace-ID")

		if authToken != "bearer-abc123" {
			http.Error(w, fmt.Sprintf("无效的认证头: %s", authToken), http.StatusUnauthorized)
			return
		}
		if traceID != "trace-001" {
			http.Error(w, fmt.Sprintf("缺少追踪头: %s", traceID), http.StatusBadRequest)
			return
		}

		// 返回成功响应
		state := TestState{Counter: 99, Path: "authed"}
		stateData, _ := json.Marshal(state)
		resp := map[string]any{"state_data": json.RawMessage(stateData)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	executor := NewHTTPNodeExecutor("auth-node", server.URL,
		WithHTTPHeader("X-Auth-Token", "bearer-abc123"),
		WithHTTPHeader("X-Trace-ID", "trace-001"),
	)

	stateData, _ := json.Marshal(TestState{Counter: 0})
	resultData, err := executor.Execute(context.Background(), "secure-task", stateData)
	if err != nil {
		t.Fatalf("带自定义请求头的执行不应失败: %v", err)
	}

	var result TestState
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("反序列化结果失败: %v", err)
	}
	if result.Counter != 99 {
		t.Errorf("期望 Counter 为 99，实际为 %d", result.Counter)
	}
}

// TestRunDistributed_UncompiledGraph 测试未编译的图执行分布式应报错
func TestRunDistributed_UncompiledGraph(t *testing.T) {
	// 直接创建未编译的图（不调用 Build）
	g := &Graph[TestState]{
		compiled: false,
	}

	registry := NewRemoteRegistry()
	_, err := g.RunDistributed(context.Background(), TestState{}, registry)
	if err == nil {
		t.Fatal("未编译的图执行 RunDistributed 应返回错误")
	}
	if !strings.Contains(err.Error(), "not compiled") {
		t.Errorf("错误信息应包含 'not compiled'，实际为: %v", err)
	}
}
