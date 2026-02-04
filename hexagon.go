// Package hexagon 提供 Hexagon AI Agent 框架的顶层 API
//
// Hexagon 是一个新一代 Go AI Agent 框架，设计目标是：
//   - 极简入门：3 行代码即可开始
//   - 类型安全：编译时检查，零运行时类型错误
//   - 高性能：原生并发，100k+ 并发 Agent
//   - 可观测：100% 覆盖率
//   - 生产就绪：优雅降级，运维友好
//
// # 快速开始
//
// 最简单的使用方式（3 行代码）：
//
//	response, _ := hexagon.Chat(ctx, "What is Go?")
//	fmt.Println(response)
//
// 带工具的 Agent：
//
//	agent := hexagon.QuickStart(
//	    hexagon.WithTools(calculatorTool),
//	    hexagon.WithSystemPrompt("You are a math assistant."),
//	)
//	output, _ := agent.Run(ctx, hexagon.Input{Query: "What is 123 * 456?"})
package hexagon

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/llm/openai"
	"github.com/everyday-items/ai-core/memory"
	"github.com/everyday-items/ai-core/tool"
	"github.com/everyday-items/hexagon/agent"
	"github.com/everyday-items/hexagon/core"
	"github.com/everyday-items/hexagon/observe/metrics"
	"github.com/everyday-items/hexagon/observe/tracer"
	"github.com/everyday-items/hexagon/orchestration/chain"
	"github.com/everyday-items/hexagon/orchestration/graph"
	"github.com/everyday-items/hexagon/rag"
	"github.com/everyday-items/hexagon/rag/embedder"
	"github.com/everyday-items/hexagon/rag/indexer"
	"github.com/everyday-items/hexagon/rag/loader"
	"github.com/everyday-items/hexagon/rag/retriever"
	"github.com/everyday-items/hexagon/rag/splitter"
	"github.com/everyday-items/hexagon/security/cost"
	"github.com/everyday-items/hexagon/security/guard"
	"github.com/everyday-items/hexagon/store/vector"
	"github.com/everyday-items/hexagon/store/vector/qdrant"
)

// Version information for the Hexagon framework.
const (
	// Version is the current version of the Hexagon framework.
	// Format: MAJOR.MINOR.PATCH[-PRERELEASE]
	Version = "0.3.0-beta"

	// VersionMajor is the major version number.
	VersionMajor = 0

	// VersionMinor is the minor version number.
	VersionMinor = 3

	// VersionPatch is the patch version number.
	VersionPatch = 0

	// VersionPrerelease is the pre-release identifier (empty for stable releases).
	VersionPrerelease = "beta"
)

// 重新导出常用类型，简化使用
type (
	// Input 是 Agent 输入
	Input = agent.Input

	// Output 是 Agent 输出
	Output = agent.Output

	// Tool 是工具接口
	Tool = tool.Tool

	// Memory 是记忆接口
	Memory = memory.Memory

	// Message 是聊天消息
	Message = llm.Message

	// Schema 是 JSON Schema
	Schema = core.Schema

	// Component 是组件接口
	Component[I, O any] = core.Component[I, O]

	// Stream 是泛型流接口
	Stream[T any] = core.Stream[T]
)

// ============== QuickStart API ==============

// defaultProvider 默认 LLM Provider（延迟初始化）
var (
	defaultProvider     llm.Provider
	defaultProviderOnce sync.Once
	defaultProviderMu   sync.RWMutex
)

// ErrNoProvider 表示没有配置 LLM Provider
var ErrNoProvider = errors.New("no LLM provider configured: set OPENAI_API_KEY environment variable or use WithProvider() option")

// getDefaultProvider 获取默认 Provider（并发安全）
func getDefaultProvider() llm.Provider {
	// 使用 sync.Once 确保只初始化一次
	defaultProviderOnce.Do(func() {
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			defaultProviderMu.Lock()
			defaultProvider = openai.New(key)
			defaultProviderMu.Unlock()
		}
	})

	defaultProviderMu.RLock()
	defer defaultProviderMu.RUnlock()
	return defaultProvider
}

// SetDefaultProvider 设置默认 LLM Provider（并发安全）
func SetDefaultProvider(p llm.Provider) {
	defaultProviderMu.Lock()
	defer defaultProviderMu.Unlock()
	defaultProvider = p
}

// QuickStartOption 是 QuickStart 的配置选项
type QuickStartOption func(*quickStartConfig)

type quickStartConfig struct {
	provider     llm.Provider
	tools        []tool.Tool
	systemPrompt string
	memory       memory.Memory
}

// WithProvider 设置 LLM Provider
func WithProvider(p llm.Provider) QuickStartOption {
	return func(c *quickStartConfig) {
		c.provider = p
	}
}

// WithTools 设置工具
func WithTools(tools ...tool.Tool) QuickStartOption {
	return func(c *quickStartConfig) {
		c.tools = append(c.tools, tools...)
	}
}

// WithSystemPrompt 设置系统提示词
func WithSystemPrompt(prompt string) QuickStartOption {
	return func(c *quickStartConfig) {
		c.systemPrompt = prompt
	}
}

// WithMemory 设置记忆系统
func WithMemory(m memory.Memory) QuickStartOption {
	return func(c *quickStartConfig) {
		c.memory = m
	}
}

// QuickStart 快速创建一个 ReAct Agent
//
// 注意：需要配置 LLM Provider，可以通过以下方式之一：
//   - 设置 OPENAI_API_KEY 环境变量
//   - 使用 WithProvider() 选项
//   - 调用 SetDefaultProvider()
//
// 如果没有配置 Provider，将会 panic。
//
// 示例：
//
//	agent := hexagon.QuickStart(
//	    hexagon.WithTools(searchTool, calculatorTool),
//	    hexagon.WithSystemPrompt("You are a helpful assistant."),
//	)
//	output, err := agent.Run(ctx, hexagon.Input{Query: "What is 2+2?"})
func QuickStart(opts ...QuickStartOption) *agent.ReActAgent {
	cfg := &quickStartConfig{
		provider: getDefaultProvider(),
		memory:   memory.NewBuffer(100),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// 检查 provider 是否配置
	if cfg.provider == nil {
		panic(ErrNoProvider)
	}

	agentOpts := []agent.Option{
		agent.WithLLM(cfg.provider),
		agent.WithMemory(cfg.memory),
	}

	if len(cfg.tools) > 0 {
		agentOpts = append(agentOpts, agent.WithTools(cfg.tools...))
	}
	if cfg.systemPrompt != "" {
		agentOpts = append(agentOpts, agent.WithSystemPrompt(cfg.systemPrompt))
	}

	return agent.NewReAct(agentOpts...)
}

// ============== 便捷函数 ==============

// Chat 执行简单对话（最简 API）
//
// 示例：
//
//	response, err := hexagon.Chat(ctx, "What is Go?")
//	fmt.Println(response)
func Chat(ctx context.Context, query string, opts ...QuickStartOption) (string, error) {
	a := QuickStart(opts...)
	output, err := a.Run(ctx, Input{Query: query})
	if err != nil {
		return "", err
	}
	return output.Content, nil
}

// ChatWithTools 带工具的对话
//
// 示例：
//
//	result, err := hexagon.ChatWithTools(ctx, "What is 123 * 456?", calculatorTool)
func ChatWithTools(ctx context.Context, query string, tools ...tool.Tool) (string, error) {
	return Chat(ctx, query, WithTools(tools...))
}

// Run 执行 Agent 并返回完整输出
//
// 示例：
//
//	output, err := hexagon.Run(ctx, hexagon.Input{Query: "Hello"})
func Run(ctx context.Context, input Input, opts ...QuickStartOption) (Output, error) {
	a := QuickStart(opts...)
	return a.Run(ctx, input)
}

// ============== 工具创建便捷函数 ==============

// NewTool 从函数创建工具
//
// 示例：
//
//	type CalcInput struct {
//	    A float64 `json:"a" desc:"第一个数字" required:"true"`
//	    B float64 `json:"b" desc:"第二个数字" required:"true"`
//	}
//
//	calculator := hexagon.NewTool("calculator", "执行加法计算",
//	    func(ctx context.Context, input CalcInput) (float64, error) {
//	        return input.A + input.B, nil
//	    },
//	)
func NewTool[I, O any](name, description string, fn func(context.Context, I) (O, error)) *tool.FuncTool[I, O] {
	return tool.NewFunc(name, description, fn)
}

// ============== 重新导出子包便捷函数 ==============

// NewReActAgent 创建 ReAct Agent
var NewReActAgent = agent.NewReAct

// NewBufferMemory 创建缓冲记忆
var NewBufferMemory = memory.NewBuffer

// NewOpenAI 创建 OpenAI Provider
var NewOpenAI = openai.New

// ============== 编排引擎 ==============

// NewGraph 创建图编排构建器
//
// 示例：
//
//	g, _ := hexagon.NewGraph[MyState]("my-graph").
//	    AddNode("step1", handler1).
//	    AddNode("step2", handler2).
//	    AddEdge(hexagon.START, "step1").
//	    AddEdge("step1", "step2").
//	    AddEdge("step2", hexagon.END).
//	    Build()
func NewGraph[S graph.State](name string) *graph.GraphBuilder[S] {
	return graph.NewGraph[S](name)
}

// NewChain 创建链式编排构建器
//
// 示例：
//
//	c, _ := hexagon.NewChain[Input, Output]("my-chain").
//	    Pipe(step1).
//	    Pipe(step2).
//	    Build()
func NewChain[I, O any](name string) *chain.ChainBuilder[I, O] {
	return chain.NewChain[I, O](name)
}

// 图编排常量
const (
	// START 起始节点
	START = graph.START
	// END 结束节点
	END = graph.END
)

// ============== 多 Agent 协作 ==============

// NewTeam 创建 Agent 团队
//
// 示例：
//
//	team := hexagon.NewTeam("research-team",
//	    hexagon.WithAgents(researcher, writer),
//	    hexagon.WithMode(hexagon.TeamModeSequential),
//	)
var NewTeam = agent.NewTeam

// TransferTo 创建 Agent 交接工具（借鉴 OpenAI Swarm）
//
// 示例：
//
//	tools := []hexagon.Tool{
//	    hexagon.TransferTo(salesAgent),
//	    hexagon.TransferTo(supportAgent),
//	}
var TransferTo = agent.TransferTo

// 团队模式常量
const (
	TeamModeSequential    = agent.TeamModeSequential
	TeamModeHierarchical  = agent.TeamModeHierarchical
	TeamModeCollaborative = agent.TeamModeCollaborative
	TeamModeRoundRobin    = agent.TeamModeRoundRobin
)

// 团队选项
var (
	WithAgents           = agent.WithAgents
	WithMode             = agent.WithMode
	WithManager          = agent.WithManager
	WithMaxRounds        = agent.WithMaxRounds
	WithTeamDescription  = agent.WithTeamDescription
)

// ============== 可观测性 ==============

// NewTracer 创建内存追踪器
//
// 示例：
//
//	tracer := hexagon.NewTracer()
//	ctx := hexagon.ContextWithTracer(ctx, tracer)
var NewTracer = tracer.NewMemoryTracer

// NewNoopTracer 创建空追踪器（禁用追踪）
var NewNoopTracer = tracer.NewNoopTracer

// ContextWithTracer 将追踪器添加到 context
var ContextWithTracer = tracer.ContextWithTracer

// StartSpan 开始新的追踪 Span
var StartSpan = tracer.StartSpan

// NewMetrics 创建内存指标收集器
//
// 示例：
//
//	m := hexagon.NewMetrics()
//	m.Counter("agent_calls", "agent", "react").Inc()
var NewMetrics = metrics.NewMemoryMetrics

// ============== 安全防护 ==============

// NewPromptInjectionGuard 创建 Prompt 注入检测守卫
//
// 示例：
//
//	guard := hexagon.NewPromptInjectionGuard()
//	result, _ := guard.Check(ctx, userInput)
//	if !result.Passed {
//	    // 处理潜在的注入攻击
//	}
var NewPromptInjectionGuard = guard.NewPromptInjectionGuard

// NewPIIGuard 创建 PII 检测守卫
var NewPIIGuard = guard.NewPIIGuard

// NewGuardChain 创建守卫链
var NewGuardChain = guard.NewGuardChain

// 守卫链模式
const (
	ChainModeAll   = guard.ChainModeAll
	ChainModeAny   = guard.ChainModeAny
	ChainModeFirst = guard.ChainModeFirst
)

// NewCostController 创建成本控制器
//
// 示例：
//
//	controller := hexagon.NewCostController(
//	    hexagon.WithBudget(10.0),  // $10 预算
//	    hexagon.WithMaxTokensTotal(100000),
//	)
var NewCostController = cost.NewController

// 成本控制选项
var (
	WithBudget              = cost.WithBudget
	WithMaxTokensPerRequest = cost.WithMaxTokensPerRequest
	WithMaxTokensPerSession = cost.WithMaxTokensPerSession
	WithMaxTokensTotal      = cost.WithMaxTokensTotal
	WithRequestsPerMinute   = cost.WithRequestsPerMinute
)

// ============== 状态管理 ==============

// NewStateManager 创建状态管理器
//
// 示例：
//
//	sm := hexagon.NewStateManager("session-123", nil)
//	sm.Turn().Set("key", "value")
//	sm.Session().Set("user_id", 123)
var NewStateManager = agent.NewStateManager

// NewGlobalState 创建全局状态
var NewGlobalState = agent.NewGlobalState

// ============== 类型重新导出 ==============

// Agent 相关类型
type (
	// Agent 是 Agent 接口
	Agent = agent.Agent

	// Role 是角色定义
	Role = agent.Role

	// Team 是团队
	Team = agent.Team

	// StateManager 是状态管理器接口
	StateManager = agent.StateManager
)

// 图编排相关类型
type (
	// Graph 是编译后的图
	Graph[S graph.State] = graph.Graph[S]

	// Chain 是链式组件
	Chain[I, O any] = chain.Chain[I, O]

	// State 是图状态接口
	State = graph.State

	// MapState 是通用 map 状态
	MapState = graph.MapState
)

// 可观测性相关类型
type (
	// Tracer 是追踪器接口
	Tracer = tracer.Tracer

	// Span 是追踪 Span 接口
	Span = tracer.Span

	// Metrics 是指标接口
	Metrics = metrics.Metrics
)

// 安全相关类型
type (
	// Guard 是守卫接口
	Guard = guard.Guard

	// CostController 是成本控制器
	CostController = cost.Controller
)

// ============== RAG 系统 ==============

// NewRAGEngine 创建 RAG 引擎
//
// 示例：
//
//	engine := hexagon.NewRAGEngine(
//	    hexagon.WithRAGStore(vectorStore),
//	    hexagon.WithRAGEmbedder(embedder),
//	)
//	docs, _ := engine.Retrieve(ctx, "What is Go?")
var NewRAGEngine = rag.NewEngine

// RAG 引擎选项
var (
	WithRAGStore     = rag.WithStore
	WithRAGEmbedder  = rag.WithEngineEmbedder
	WithRAGLoader    = rag.WithLoader
	WithRAGSplitter  = rag.WithEngineSplitter
	WithRAGTopK      = rag.WithEngineTopK
	WithRAGMinScore  = rag.WithEngineMinScore
)

// RAG 检索选项
var (
	// WithFilter 设置元数据过滤条件
	WithFilter = rag.WithFilter

	// WithTopK 设置返回文档数量
	WithTopK = rag.WithTopK

	// WithMinScore 设置最小相关性分数
	WithMinScore = rag.WithMinScore
)

// NewRAGPipeline 创建 RAG 管道
var NewRAGPipeline = rag.NewPipeline

// ============== RAG 组件 ==============

// 文档加载器
var (
	// NewTextLoader 创建文本文件加载器
	NewTextLoader = loader.NewTextLoader

	// NewMarkdownLoader 创建 Markdown 文件加载器
	NewMarkdownLoader = loader.NewMarkdownLoader

	// NewDirectoryLoader 创建目录批量加载器
	NewDirectoryLoader = loader.NewDirectoryLoader

	// NewURLLoader 创建 URL 加载器
	NewURLLoader = loader.NewURLLoader

	// NewStringLoader 创建字符串加载器
	NewStringLoader = loader.NewStringLoader
)

// 文档分割器
var (
	// NewCharacterSplitter 创建字符分割器
	NewCharacterSplitter = splitter.NewCharacterSplitter

	// NewRecursiveSplitter 创建递归分割器
	NewRecursiveSplitter = splitter.NewRecursiveSplitter

	// NewMarkdownSplitter 创建 Markdown 分割器
	NewMarkdownSplitter = splitter.NewMarkdownSplitter

	// NewSentenceSplitter 创建句子分割器
	NewSentenceSplitter = splitter.NewSentenceSplitter
)

// 文档检索器
var (
	// NewVectorRetriever 创建向量检索器
	NewVectorRetriever = retriever.NewVectorRetriever

	// NewKeywordRetriever 创建关键词检索器
	NewKeywordRetriever = retriever.NewKeywordRetriever

	// NewHybridRetriever 创建混合检索器
	NewHybridRetriever = retriever.NewHybridRetriever

	// NewMultiRetriever 创建多源检索器
	NewMultiRetriever = retriever.NewMultiRetriever
)

// 文档索引器
var (
	// NewVectorIndexer 创建向量索引器
	NewVectorIndexer = indexer.NewVectorIndexer

	// NewConcurrentIndexer 创建并发索引器
	NewConcurrentIndexer = indexer.NewConcurrentIndexer

	// NewIncrementalIndexer 创建增量索引器
	NewIncrementalIndexer = indexer.NewIncrementalIndexer
)

// 向量生成器
var (
	// NewOpenAIEmbedder 创建 OpenAI Embedder
	NewOpenAIEmbedder = embedder.NewOpenAIEmbedder

	// NewCachedEmbedder 创建带缓存的 Embedder
	NewCachedEmbedder = embedder.NewCachedEmbedder

	// NewMockEmbedder 创建模拟 Embedder（用于测试）
	NewMockEmbedder = embedder.NewMockEmbedder
)

// 向量存储
var (
	// NewMemoryVectorStore 创建内存向量存储
	NewMemoryVectorStore = vector.NewMemoryStore

	// NewQdrantStore 创建 Qdrant 向量存储
	//
	// 示例：
	//
	//	store, err := hexagon.NewQdrantStore(hexagon.QdrantConfig{
	//	    Host:             "localhost",
	//	    Port:             6333,
	//	    Collection:       "documents",
	//	    Dimension:        1536,
	//	    CreateCollection: true,
	//	})
	NewQdrantStore = qdrant.New
)

// QdrantConfig 是 Qdrant 配置
type QdrantConfig = qdrant.Config

// Qdrant 距离度量方式
const (
	QdrantDistanceCosine = qdrant.DistanceCosine
	QdrantDistanceEuclid = qdrant.DistanceEuclid
	QdrantDistanceDot    = qdrant.DistanceDot
)

// Qdrant 选项式创建
var NewQdrantStoreWithOptions = qdrant.NewWithOptions

// Qdrant 配置选项
var (
	QdrantWithHost             = qdrant.WithHost
	QdrantWithPort             = qdrant.WithPort
	QdrantWithCollection       = qdrant.WithCollection
	QdrantWithDimension        = qdrant.WithDimension
	QdrantWithAPIKey           = qdrant.WithAPIKey
	QdrantWithHTTPS            = qdrant.WithHTTPS
	QdrantWithTimeout          = qdrant.WithTimeout
	QdrantWithDistance         = qdrant.WithDistance
	QdrantWithOnDisk           = qdrant.WithOnDisk
	QdrantWithCreateCollection = qdrant.WithCreateCollection
)

// RAG 相关类型
type (
	// Document 是 RAG 文档
	Document = rag.Document

	// Loader 是文档加载器接口
	Loader = rag.Loader

	// Splitter 是文档分割器接口
	Splitter = rag.Splitter

	// Indexer 是文档索引器接口
	Indexer = rag.Indexer

	// Retriever 是文档检索器接口
	Retriever = rag.Retriever

	// Embedder 是向量生成器接口
	Embedder = rag.Embedder

	// VectorStore 是向量存储接口
	VectorStore = rag.VectorStore

	// RAGEngine 是 RAG 引擎
	RAGEngine = rag.Engine
)
