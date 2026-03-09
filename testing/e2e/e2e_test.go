// Package e2e 提供端到端测试框架
//
// 本文件包含 E2E 测试框架自身的全面测试：
//   - Runner: 运行器创建和选项配置
//   - RunCase: 单用例执行、步骤顺序、超时、Setup/Teardown
//   - RunSuite: 套件执行、并行模式、套件级 Setup 失败
//   - TestResult/SuiteResult: 结果报告格式化
package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestNewRunner 测试创建运行器
func TestNewRunner(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("期望运行器不为 nil")
	}
	if r.verbose {
		t.Error("期望默认非 verbose 模式")
	}
	if r.parallel {
		t.Error("期望默认非 parallel 模式")
	}
}

// TestNewRunnerWithOptions 测试运行器选项
func TestNewRunnerWithOptions(t *testing.T) {
	r := NewRunner(
		WithVerbose(true),
		WithParallel(true),
		WithDefaultTimeout(10*time.Second),
	)
	if !r.verbose {
		t.Error("期望 verbose 模式")
	}
	if !r.parallel {
		t.Error("期望 parallel 模式")
	}
	if r.defaultTimeout != 10*time.Second {
		t.Errorf("期望超时为 10s，实际为 %v", r.defaultTimeout)
	}
}

// TestRunCaseSimple 测试简单用例执行
func TestRunCaseSimple(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "简单用例",
		Steps: []Step{
			{
				Name: "步骤1",
				Action: func(ctx context.Context) (any, error) {
					return "hello", nil
				},
				Validate: func(result any) error {
					if result != "hello" {
						return fmt.Errorf("期望 'hello'，实际为 '%v'", result)
					}
					return nil
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if !result.Passed {
		t.Errorf("期望用例通过，但失败了: %s", result.Error)
	}
	if len(result.Steps) != 1 {
		t.Errorf("期望 1 个步骤结果，实际为 %d", len(result.Steps))
	}
	if !result.Steps[0].Passed {
		t.Errorf("期望步骤 1 通过")
	}
	if result.Duration <= 0 {
		t.Error("期望持续时间大于 0")
	}
}

// TestRunCaseMultipleSteps 测试多步骤用例
func TestRunCaseMultipleSteps(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	stepOrder := make([]string, 0)

	tc := &TestCase{
		Name: "多步骤",
		Steps: []Step{
			{
				Name: "步骤A",
				Action: func(ctx context.Context) (any, error) {
					stepOrder = append(stepOrder, "A")
					return nil, nil
				},
			},
			{
				Name: "步骤B",
				Action: func(ctx context.Context) (any, error) {
					stepOrder = append(stepOrder, "B")
					return nil, nil
				},
			},
			{
				Name: "步骤C",
				Action: func(ctx context.Context) (any, error) {
					stepOrder = append(stepOrder, "C")
					return nil, nil
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if !result.Passed {
		t.Error("期望用例通过")
	}
	if len(result.Steps) != 3 {
		t.Errorf("期望 3 个步骤结果")
	}

	// 验证按顺序执行
	if len(stepOrder) != 3 || stepOrder[0] != "A" || stepOrder[1] != "B" || stepOrder[2] != "C" {
		t.Errorf("期望执行顺序为 A,B,C，实际为 %v", stepOrder)
	}
}

// TestRunCaseStepActionError 测试步骤 Action 失败
func TestRunCaseStepActionError(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "Action 失败",
		Steps: []Step{
			{
				Name: "成功步骤",
				Action: func(ctx context.Context) (any, error) {
					return "ok", nil
				},
			},
			{
				Name: "失败步骤",
				Action: func(ctx context.Context) (any, error) {
					return nil, errors.New("action error")
				},
			},
			{
				Name: "不应执行",
				Action: func(ctx context.Context) (any, error) {
					t.Error("此步骤不应被执行")
					return nil, nil
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if result.Passed {
		t.Error("期望用例失败")
	}

	// 只应该有 2 个步骤结果（第三个不应执行）
	if len(result.Steps) != 2 {
		t.Errorf("期望 2 个步骤结果（失败后停止），实际为 %d", len(result.Steps))
	}
	if result.Steps[0].Passed != true {
		t.Error("期望第一个步骤通过")
	}
	if result.Steps[1].Passed != false {
		t.Error("期望第二个步骤失败")
	}
}

// TestRunCaseValidateError 测试步骤验证失败
func TestRunCaseValidateError(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "验证失败",
		Steps: []Step{
			{
				Name: "验证不通过",
				Action: func(ctx context.Context) (any, error) {
					return "wrong", nil
				},
				Validate: func(result any) error {
					return errors.New("validation failed")
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if result.Passed {
		t.Error("期望用例失败")
	}
	if !strings.Contains(result.Steps[0].Error, "验证失败") {
		t.Errorf("期望错误包含 '验证失败'，实际为 '%s'", result.Steps[0].Error)
	}
}

// TestRunCaseSetupError 测试 Setup 失败
func TestRunCaseSetupError(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "Setup 失败",
		Setup: func() error {
			return errors.New("setup failed")
		},
		Steps: []Step{
			{
				Name: "不应执行",
				Action: func(ctx context.Context) (any, error) {
					t.Error("Setup 失败后不应执行步骤")
					return nil, nil
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if result.Passed {
		t.Error("期望 Setup 失败导致用例失败")
	}
	if !strings.Contains(result.Error, "Setup 失败") {
		t.Errorf("期望错误包含 'Setup 失败'，实际为 '%s'", result.Error)
	}
	if len(result.Steps) != 0 {
		t.Error("期望 Setup 失败时没有步骤结果")
	}
}

// TestRunCaseTeardown 测试 Teardown 执行
func TestRunCaseTeardown(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	teardownCalled := false
	tc := &TestCase{
		Name: "Teardown 测试",
		Steps: []Step{
			{
				Name: "正常步骤",
				Action: func(ctx context.Context) (any, error) {
					return nil, nil
				},
			},
		},
		Teardown: func() error {
			teardownCalled = true
			return nil
		},
	}

	r.RunCase(ctx, tc)
	if !teardownCalled {
		t.Error("期望 Teardown 被调用")
	}
}

// TestRunCaseTeardownAfterFailure 测试步骤失败后 Teardown 仍然执行
func TestRunCaseTeardownAfterFailure(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	teardownCalled := false
	tc := &TestCase{
		Name: "失败后 Teardown",
		Steps: []Step{
			{
				Name: "失败步骤",
				Action: func(ctx context.Context) (any, error) {
					return nil, errors.New("step failed")
				},
			},
		},
		Teardown: func() error {
			teardownCalled = true
			return nil
		},
	}

	r.RunCase(ctx, tc)
	if !teardownCalled {
		t.Error("期望步骤失败后 Teardown 仍被调用")
	}
}

// TestRunCaseTeardownError 测试 Teardown 失败
func TestRunCaseTeardownError(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "Teardown 失败",
		Steps: []Step{
			{
				Name: "正常步骤",
				Action: func(ctx context.Context) (any, error) {
					return nil, nil
				},
			},
		},
		Teardown: func() error {
			return errors.New("teardown error")
		},
	}

	result := r.RunCase(ctx, tc)
	// Teardown 失败时，如果之前没有错误，应该记录 Teardown 错误
	if result.Error == "" {
		t.Error("期望记录 Teardown 错误")
	}
}

// TestRunCaseTimeout 测试超时
func TestRunCaseTimeout(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name:    "超时用例",
		Timeout: 50 * time.Millisecond,
		Steps: []Step{
			{
				Name: "超时步骤",
				Action: func(ctx context.Context) (any, error) {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(5 * time.Second):
						return nil, nil
					}
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if result.Passed {
		t.Error("期望超时导致失败")
	}
}

// TestRunCaseStepTimeout 测试步骤级超时
func TestRunCaseStepTimeout(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "步骤超时",
		Steps: []Step{
			{
				Name:    "超时步骤",
				Timeout: 50 * time.Millisecond,
				Action: func(ctx context.Context) (any, error) {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(5 * time.Second):
						return nil, nil
					}
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if result.Passed {
		t.Error("期望步骤超时导致失败")
	}
}

// TestRunCaseNoAction 测试没有 Action 的步骤
func TestRunCaseNoAction(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	tc := &TestCase{
		Name: "无 Action",
		Steps: []Step{
			{
				Name: "仅验证",
				Validate: func(result any) error {
					if result != nil {
						return fmt.Errorf("期望 nil")
					}
					return nil
				},
			},
		},
	}

	result := r.RunCase(ctx, tc)
	if !result.Passed {
		t.Errorf("期望通过: %s", result.Steps[0].Error)
	}
}

// ============================================================================
// RunSuite 测试
// ============================================================================

// TestRunSuiteSequential 测试顺序执行套件
func TestRunSuiteSequential(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	suite := &TestSuite{
		Name: "顺序套件",
		Cases: []*TestCase{
			{
				Name: "用例1",
				Steps: []Step{
					{Name: "步骤1", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
			{
				Name: "用例2",
				Steps: []Step{
					{Name: "步骤1", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
		},
	}

	result := r.RunSuite(ctx, suite)

	if result.SuiteName != "顺序套件" {
		t.Errorf("期望套件名为 '顺序套件'")
	}
	if result.TotalCases != 2 {
		t.Errorf("期望总用例数为 2，实际为 %d", result.TotalCases)
	}
	if result.Passed != 2 {
		t.Errorf("期望通过数为 2，实际为 %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("期望失败数为 0，实际为 %d", result.Failed)
	}
	if result.Duration <= 0 {
		t.Error("期望持续时间大于 0")
	}
}

// TestRunSuiteParallel 测试并行执行套件
func TestRunSuiteParallel(t *testing.T) {
	r := NewRunner(WithParallel(true))
	ctx := context.Background()

	suite := &TestSuite{
		Name: "并行套件",
		Cases: []*TestCase{
			{
				Name: "用例1",
				Steps: []Step{
					{Name: "步骤", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
			{
				Name: "用例2",
				Steps: []Step{
					{Name: "步骤", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
			{
				Name: "用例3",
				Steps: []Step{
					{Name: "步骤", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
		},
	}

	result := r.RunSuite(ctx, suite)

	if result.TotalCases != 3 {
		t.Errorf("期望总用例数为 3")
	}
	if result.Passed != 3 {
		t.Errorf("期望全部通过，通过数=%d，失败数=%d", result.Passed, result.Failed)
	}
}

// TestRunSuiteMixed 测试混合通过/失败的套件
func TestRunSuiteMixed(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	suite := &TestSuite{
		Name: "混合套件",
		Cases: []*TestCase{
			{
				Name: "通过",
				Steps: []Step{
					{Name: "ok", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
			{
				Name: "失败",
				Steps: []Step{
					{Name: "fail", Action: func(ctx context.Context) (any, error) { return nil, errors.New("fail") }},
				},
			},
		},
	}

	result := r.RunSuite(ctx, suite)

	if result.Passed != 1 {
		t.Errorf("期望通过 1 个，实际为 %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("期望失败 1 个，实际为 %d", result.Failed)
	}
}

// TestRunSuiteSetup 测试套件 Setup
func TestRunSuiteSetup(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	setupCalled := false
	suite := &TestSuite{
		Name: "Setup 套件",
		Setup: func() error {
			setupCalled = true
			return nil
		},
		Cases: []*TestCase{
			{
				Name: "用例",
				Steps: []Step{
					{Name: "步骤", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
		},
	}

	r.RunSuite(ctx, suite)
	if !setupCalled {
		t.Error("期望套件 Setup 被调用")
	}
}

// TestRunSuiteSetupError 测试套件 Setup 失败
func TestRunSuiteSetupError(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	suite := &TestSuite{
		Name: "Setup 失败",
		Setup: func() error {
			return errors.New("suite setup failed")
		},
		Cases: []*TestCase{
			{Name: "用例1"},
			{Name: "用例2"},
		},
	}

	result := r.RunSuite(ctx, suite)

	// 所有用例都应该标记为失败
	if result.Passed != 0 {
		t.Errorf("期望通过数为 0，实际为 %d", result.Passed)
	}
	if result.Failed != 2 {
		t.Errorf("期望失败数为 2，实际为 %d", result.Failed)
	}

	for _, tr := range result.Results {
		if tr.Passed {
			t.Error("期望所有用例标记为失败")
		}
		if !strings.Contains(tr.Error, "套件 Setup 失败") {
			t.Errorf("期望错误包含 '套件 Setup 失败'")
		}
	}
}

// TestRunSuiteTeardown 测试套件 Teardown
func TestRunSuiteTeardown(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	teardownCalled := false
	suite := &TestSuite{
		Name: "Teardown 套件",
		Cases: []*TestCase{
			{
				Name: "用例",
				Steps: []Step{
					{Name: "步骤", Action: func(ctx context.Context) (any, error) { return nil, nil }},
				},
			},
		},
		Teardown: func() error {
			teardownCalled = true
			return nil
		},
	}

	r.RunSuite(ctx, suite)
	if !teardownCalled {
		t.Error("期望套件 Teardown 被调用")
	}
}

// ============================================================================
// SuiteResult 测试
// ============================================================================

// TestSuiteResultString 测试结果字符串格式化
func TestSuiteResultString(t *testing.T) {
	result := &SuiteResult{
		SuiteName:  "测试套件",
		TotalCases: 3,
		Passed:     2,
		Failed:     1,
		Duration:   100 * time.Millisecond,
		Results: []*TestResult{
			{
				CaseName: "通过用例",
				Passed:   true,
				Duration: 30 * time.Millisecond,
				Steps: []StepResult{
					{StepName: "步骤1", Passed: true, Duration: 10 * time.Millisecond},
				},
			},
			{
				CaseName: "失败用例",
				Passed:   false,
				Duration: 50 * time.Millisecond,
				Steps: []StepResult{
					{StepName: "步骤1", Passed: true, Duration: 10 * time.Millisecond},
					{StepName: "步骤2", Passed: false, Duration: 20 * time.Millisecond, Error: "验证失败: 数据不匹配"},
				},
			},
		},
	}

	output := result.String()

	// 验证输出包含关键信息
	checks := []string{
		"测试套件",
		"通过: 2",
		"失败: 1",
		"[PASS] 通过用例",
		"[FAIL] 失败用例",
		"[FAIL] 步骤2",
		"验证失败: 数据不匹配",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("期望输出包含 '%s'，完整输出:\n%s", check, output)
		}
	}
}

// TestSuiteResultStringEmpty 测试空结果的格式化
func TestSuiteResultStringEmpty(t *testing.T) {
	result := &SuiteResult{
		SuiteName: "空套件",
	}

	output := result.String()
	if !strings.Contains(output, "空套件") {
		t.Errorf("期望输出包含套件名称")
	}
}

// TestRunCaseWithSetupAndTeardown 测试完整的 Setup/Steps/Teardown 流程
func TestRunCaseWithSetupAndTeardown(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	order := make([]string, 0)

	tc := &TestCase{
		Name: "完整流程",
		Setup: func() error {
			order = append(order, "setup")
			return nil
		},
		Steps: []Step{
			{
				Name: "步骤1",
				Action: func(ctx context.Context) (any, error) {
					order = append(order, "step1")
					return nil, nil
				},
			},
			{
				Name: "步骤2",
				Action: func(ctx context.Context) (any, error) {
					order = append(order, "step2")
					return nil, nil
				},
			},
		},
		Teardown: func() error {
			order = append(order, "teardown")
			return nil
		},
	}

	result := r.RunCase(ctx, tc)
	if !result.Passed {
		t.Error("期望用例通过")
	}

	expected := []string{"setup", "step1", "step2", "teardown"}
	if len(order) != len(expected) {
		t.Fatalf("期望执行顺序长度为 %d，实际为 %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("期望第 %d 步为 '%s'，实际为 '%s'", i, v, order[i])
		}
	}
}
