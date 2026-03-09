// Package e2e 提供端到端测试框架
//
// 支持构建完整的 Agent 测试场景，包括输入、预期输出和验证。
//
// 主要组件：
//   - TestCase: 单个测试用例，包含多个测试步骤
//   - TestSuite: 测试套件，组合多个测试用例
//   - Runner: 测试运行器，支持顺序和并行执行
//   - TestResult/SuiteResult: 测试结果，包含通过/失败详情
//
// 使用示例：
//
//	suite := &e2e.TestSuite{
//	    Name: "Agent 基础功能",
//	    Cases: []*e2e.TestCase{
//	        {
//	            Name: "简单对话",
//	            Steps: []e2e.Step{
//	                {
//	                    Name: "发送消息",
//	                    Action: func(ctx context.Context) (any, error) {
//	                        return agent.Run(ctx, input)
//	                    },
//	                    Validate: func(result any) error {
//	                        // 验证结果
//	                        return nil
//	                    },
//	                },
//	            },
//	        },
//	    },
//	}
//
//	runner := e2e.NewRunner()
//	result := runner.RunSuite(ctx, suite)
//	fmt.Println(result)
package e2e

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TestCase E2E 测试用例
//
// 包含一系列测试步骤，按顺序执行。
// 支持 Setup/Teardown 钩子，分别在用例开始前和结束后调用。
type TestCase struct {
	// Name 用例名称
	Name string

	// Description 用例描述
	Description string

	// Steps 测试步骤列表，按顺序执行
	Steps []Step

	// Setup 用例开始前的初始化函数（可选）
	Setup func() error

	// Teardown 用例结束后的清理函数（可选）
	Teardown func() error

	// Timeout 用例级别超时（0 表示使用 Runner 默认超时）
	Timeout time.Duration
}

// Step 测试步骤
//
// 每个步骤包含一个动作和一个验证函数。
// 动作返回结果后，验证函数检查结果是否符合预期。
type Step struct {
	// Name 步骤名称
	Name string

	// Action 步骤动作，返回执行结果和错误
	Action func(ctx context.Context) (any, error)

	// Validate 验证函数，检查 Action 的结果是否符合预期
	// 返回 nil 表示验证通过，返回 error 表示验证失败
	Validate func(result any) error

	// Timeout 步骤级别超时（0 表示使用用例级别超时）
	Timeout time.Duration
}

// TestSuite E2E 测试套件
//
// 组合多个测试用例，提供套件级别的 Setup/Teardown。
type TestSuite struct {
	// Name 套件名称
	Name string

	// Cases 测试用例列表
	Cases []*TestCase

	// Setup 套件开始前的初始化函数（可选）
	Setup func() error

	// Teardown 套件结束后的清理函数（可选）
	Teardown func() error
}

// StepResult 步骤执行结果
type StepResult struct {
	// StepName 步骤名称
	StepName string

	// Passed 是否通过
	Passed bool

	// Duration 执行耗时
	Duration time.Duration

	// Error 错误信息（如果失败）
	Error string
}

// TestResult 测试用例执行结果
type TestResult struct {
	// CaseName 用例名称
	CaseName string

	// Passed 是否通过
	Passed bool

	// Steps 各步骤结果
	Steps []StepResult

	// Duration 总执行耗时
	Duration time.Duration

	// Error 用例级别的错误信息（例如 Setup 失败）
	Error string
}

// SuiteResult 测试套件执行结果
type SuiteResult struct {
	// SuiteName 套件名称
	SuiteName string

	// Results 各用例结果
	Results []*TestResult

	// TotalCases 用例总数
	TotalCases int

	// Passed 通过的用例数
	Passed int

	// Failed 失败的用例数
	Failed int

	// Duration 总执行耗时
	Duration time.Duration
}

// String 返回套件结果的可读字符串表示
func (r *SuiteResult) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== 测试套件: %s ===\n", r.SuiteName))
	sb.WriteString(fmt.Sprintf("总耗时: %v\n", r.Duration))
	sb.WriteString(fmt.Sprintf("用例: %d 个 (通过: %d, 失败: %d)\n\n", r.TotalCases, r.Passed, r.Failed))

	for _, tr := range r.Results {
		status := "PASS"
		if !tr.Passed {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%v)\n", status, tr.CaseName, tr.Duration))

		if tr.Error != "" {
			sb.WriteString(fmt.Sprintf("  错误: %s\n", tr.Error))
		}

		for _, sr := range tr.Steps {
			stepStatus := "PASS"
			if !sr.Passed {
				stepStatus = "FAIL"
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s (%v)\n", stepStatus, sr.StepName, sr.Duration))
			if sr.Error != "" {
				sb.WriteString(fmt.Sprintf("    错误: %s\n", sr.Error))
			}
		}
	}

	return sb.String()
}

// RunnerOption Runner 配置选项
type RunnerOption func(*Runner)

// WithVerbose 设置详细输出模式
func WithVerbose(verbose bool) RunnerOption {
	return func(r *Runner) {
		r.verbose = verbose
	}
}

// WithParallel 设置并行执行模式
func WithParallel(parallel bool) RunnerOption {
	return func(r *Runner) {
		r.parallel = parallel
	}
}

// WithDefaultTimeout 设置默认超时
func WithDefaultTimeout(timeout time.Duration) RunnerOption {
	return func(r *Runner) {
		r.defaultTimeout = timeout
	}
}

// Runner E2E 测试运行器
//
// 支持顺序和并行两种执行模式。
// 并行模式下，套件中的各用例会并发执行。
type Runner struct {
	// verbose 是否输出详细信息
	verbose bool

	// parallel 是否并行执行用例
	parallel bool

	// defaultTimeout 默认超时时间
	defaultTimeout time.Duration
}

// NewRunner 创建 E2E 测试运行器
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		defaultTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// RunCase 执行单个测试用例
func (r *Runner) RunCase(ctx context.Context, tc *TestCase) *TestResult {
	start := time.Now()
	result := &TestResult{
		CaseName: tc.Name,
		Passed:   true,
		Steps:    make([]StepResult, 0, len(tc.Steps)),
	}

	// 确定超时
	timeout := r.defaultTimeout
	if tc.Timeout > 0 {
		timeout = tc.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 执行 Setup
	if tc.Setup != nil {
		if err := tc.Setup(); err != nil {
			result.Passed = false
			result.Error = fmt.Sprintf("Setup 失败: %v", err)
			result.Duration = time.Since(start)
			return result
		}
	}

	// 确保执行 Teardown
	defer func() {
		if tc.Teardown != nil {
			if err := tc.Teardown(); err != nil {
				// Teardown 失败不会覆盖已有的错误
				if result.Error == "" {
					result.Error = fmt.Sprintf("Teardown 失败: %v", err)
				}
			}
		}
	}()

	// 执行步骤
	for _, step := range tc.Steps {
		stepResult := r.runStep(ctx, step)
		result.Steps = append(result.Steps, stepResult)

		if !stepResult.Passed {
			result.Passed = false
			// 步骤失败后停止执行后续步骤
			break
		}
	}

	result.Duration = time.Since(start)
	return result
}

// runStep 执行单个测试步骤
func (r *Runner) runStep(ctx context.Context, step Step) StepResult {
	start := time.Now()
	sr := StepResult{
		StepName: step.Name,
		Passed:   true,
	}

	// 步骤级超时
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// 执行动作
	var actionResult any
	if step.Action != nil {
		var err error
		actionResult, err = step.Action(ctx)
		if err != nil {
			sr.Passed = false
			sr.Error = fmt.Sprintf("Action 执行失败: %v", err)
			sr.Duration = time.Since(start)
			return sr
		}
	}

	// 执行验证
	if step.Validate != nil {
		if err := step.Validate(actionResult); err != nil {
			sr.Passed = false
			sr.Error = fmt.Sprintf("验证失败: %v", err)
			sr.Duration = time.Since(start)
			return sr
		}
	}

	sr.Duration = time.Since(start)
	return sr
}

// RunSuite 执行测试套件
func (r *Runner) RunSuite(ctx context.Context, suite *TestSuite) *SuiteResult {
	start := time.Now()
	result := &SuiteResult{
		SuiteName:  suite.Name,
		Results:    make([]*TestResult, 0, len(suite.Cases)),
		TotalCases: len(suite.Cases),
	}

	// 执行套件 Setup
	if suite.Setup != nil {
		if err := suite.Setup(); err != nil {
			// Setup 失败，所有用例标记为失败
			for _, tc := range suite.Cases {
				result.Results = append(result.Results, &TestResult{
					CaseName: tc.Name,
					Passed:   false,
					Error:    fmt.Sprintf("套件 Setup 失败: %v", err),
				})
				result.Failed++
			}
			result.Duration = time.Since(start)
			return result
		}
	}

	// 确保执行套件 Teardown
	defer func() {
		if suite.Teardown != nil {
			_ = suite.Teardown()
		}
	}()

	if r.parallel {
		r.runCasesParallel(ctx, suite.Cases, result)
	} else {
		r.runCasesSequential(ctx, suite.Cases, result)
	}

	result.Duration = time.Since(start)
	return result
}

// runCasesSequential 顺序执行测试用例
func (r *Runner) runCasesSequential(ctx context.Context, cases []*TestCase, result *SuiteResult) {
	for _, tc := range cases {
		tr := r.RunCase(ctx, tc)
		result.Results = append(result.Results, tr)
		if tr.Passed {
			result.Passed++
		} else {
			result.Failed++
		}
	}
}

// runCasesParallel 并行执行测试用例
func (r *Runner) runCasesParallel(ctx context.Context, cases []*TestCase, result *SuiteResult) {
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tc := range cases {
		wg.Add(1)
		go func(testCase *TestCase) {
			defer wg.Done()
			tr := r.RunCase(ctx, testCase)

			mu.Lock()
			result.Results = append(result.Results, tr)
			if tr.Passed {
				result.Passed++
			} else {
				result.Failed++
			}
			mu.Unlock()
		}(tc)
	}

	wg.Wait()
}
