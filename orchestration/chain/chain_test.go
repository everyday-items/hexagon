package chain

import (
	"context"
	"errors"
	"testing"
)

func TestNewChain(t *testing.T) {
	builder := NewChain[string, string]("test-chain")

	if builder == nil {
		t.Fatal("expected non-nil builder")
	}

	if builder.chain.name != "test-chain" {
		t.Errorf("expected name 'test-chain', got '%s'", builder.chain.name)
	}
}

func TestChainBuilderWithDescription(t *testing.T) {
	builder := NewChain[string, string]("test-chain").
		WithDescription("A test chain")

	if builder.chain.description != "A test chain" {
		t.Errorf("expected description 'A test chain', got '%s'", builder.chain.description)
	}
}

func TestChainBuilderPipeFunc(t *testing.T) {
	builder := NewChain[string, string]("test-chain").
		PipeFunc("step1", func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-processed", nil
		})

	if len(builder.chain.steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(builder.chain.steps))
	}

	if builder.chain.steps[0].name != "step1" {
		t.Errorf("expected step name 'step1', got '%s'", builder.chain.steps[0].name)
	}
}

func TestChainBuilderBuild(t *testing.T) {
	chain, err := NewChain[string, string]("test-chain").
		PipeFunc("step1", func(ctx context.Context, input any) (any, error) {
			return input, nil
		}).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestChainBuilderBuildNoSteps(t *testing.T) {
	_, err := NewChain[string, string]("empty-chain").Build()

	if err == nil {
		t.Error("expected error for chain with no steps")
	}
}

func TestChainBuilderMustBuild(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Log("MustBuild panicked as expected for empty chain")
		}
	}()

	// 有步骤时不应该 panic
	chain := NewChain[string, string]("test-chain").
		PipeFunc("step1", func(ctx context.Context, input any) (any, error) {
			return input, nil
		}).
		MustBuild()

	if chain == nil {
		t.Error("expected non-nil chain from MustBuild")
	}
}

func TestChainBuilderMustBuildPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for chain with no steps")
		}
	}()

	_ = NewChain[string, string]("empty-chain").MustBuild()
}

func TestChainNameAndDescription(t *testing.T) {
	chain, _ := NewChain[string, string]("my-chain").
		WithDescription("My chain description").
		PipeFunc("step", func(ctx context.Context, input any) (any, error) {
			return input, nil
		}).
		Build()

	if chain.Name() != "my-chain" {
		t.Errorf("expected name 'my-chain', got '%s'", chain.Name())
	}

	if chain.Description() != "My chain description" {
		t.Errorf("expected description 'My chain description', got '%s'", chain.Description())
	}
}

func TestChainRun(t *testing.T) {
	chain, _ := NewChain[string, string]("process-chain").
		PipeFunc("uppercase", func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-upper", nil
		}).
		PipeFunc("suffix", func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-done", nil
		}).
		Build()

	result, err := chain.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "test-upper-done" {
		t.Errorf("expected 'test-upper-done', got '%s'", result)
	}
}

func TestChainRunWithError(t *testing.T) {
	expectedErr := errors.New("step failed")

	chain, _ := NewChain[string, string]("error-chain").
		PipeFunc("failing-step", func(ctx context.Context, input any) (any, error) {
			return nil, expectedErr
		}).
		Build()

	_, err := chain.Run(context.Background(), "test")
	if err == nil {
		t.Error("expected error from chain")
	}
}

func TestChainRunTypeMismatch(t *testing.T) {
	// 创建一个返回错误类型的链
	chain, _ := NewChain[string, string]("type-mismatch").
		PipeFunc("returns-int", func(ctx context.Context, input any) (any, error) {
			return 123, nil // 返回 int 而不是 string
		}).
		Build()

	_, err := chain.Run(context.Background(), "test")
	if err == nil {
		t.Error("expected type mismatch error")
	}
}

func TestChainStream(t *testing.T) {
	chain, _ := NewChain[string, string]("stream-chain").
		PipeFunc("identity", func(ctx context.Context, input any) (any, error) {
			return input, nil
		}).
		Build()

	stream, err := chain.Stream(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stream == nil {
		t.Fatal("expected non-nil stream")
	}

	// 收集结果
	var results []string
	for {
		item, ok := stream.Next(context.Background())
		if !ok {
			break
		}
		results = append(results, item)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0] != "test" {
		t.Errorf("expected 'test', got '%s'", results[0])
	}
}

func TestChainBatch(t *testing.T) {
	chain, _ := NewChain[string, string]("batch-chain").
		PipeFunc("process", func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-processed", nil
		}).
		Build()

	inputs := []string{"a", "b", "c"}
	results, err := chain.Batch(context.Background(), inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	expected := []string{"a-processed", "b-processed", "c-processed"}
	for i, result := range results {
		if result != expected[i] {
			t.Errorf("result[%d]: expected '%s', got '%s'", i, expected[i], result)
		}
	}
}

func TestChainBatchWithError(t *testing.T) {
	chain, _ := NewChain[string, string]("batch-error").
		PipeFunc("failing", func(ctx context.Context, input any) (any, error) {
			if input.(string) == "fail" {
				return nil, errors.New("failed on 'fail'")
			}
			return input, nil
		}).
		Build()

	inputs := []string{"ok", "fail", "ok"}
	_, err := chain.Batch(context.Background(), inputs)
	if err == nil {
		t.Error("expected batch error")
	}
}

func TestChainSchemas(t *testing.T) {
	chain, _ := NewChain[string, int]("schema-chain").
		PipeFunc("convert", func(ctx context.Context, input any) (any, error) {
			return len(input.(string)), nil
		}).
		Build()

	inputSchema := chain.InputSchema()
	if inputSchema == nil {
		t.Error("expected non-nil input schema")
	}

	outputSchema := chain.OutputSchema()
	if outputSchema == nil {
		t.Error("expected non-nil output schema")
	}
}

func TestChainWithMiddleware(t *testing.T) {
	var logs []string

	loggingMiddleware := func(next StepFunc) StepFunc {
		return func(ctx context.Context, input any) (any, error) {
			logs = append(logs, "before")
			result, err := next(ctx, input)
			logs = append(logs, "after")
			return result, err
		}
	}

	chain, _ := NewChain[string, string]("middleware-chain").
		Use(loggingMiddleware).
		PipeFunc("step", func(ctx context.Context, input any) (any, error) {
			logs = append(logs, "step")
			return input, nil
		}).
		Build()

	_, _ = chain.Run(context.Background(), "test")

	expected := []string{"before", "step", "after"}
	if len(logs) != len(expected) {
		t.Errorf("expected %d log entries, got %d", len(expected), len(logs))
	}

	for i, log := range logs {
		if log != expected[i] {
			t.Errorf("log[%d]: expected '%s', got '%s'", i, expected[i], log)
		}
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var loggedName string
	var loggedInput, loggedOutput any
	var loggedErr error

	logger := func(name string, input, output any, err error) {
		loggedName = name
		loggedInput = input
		loggedOutput = output
		loggedErr = err
	}

	middleware := LoggingMiddleware(logger)

	next := func(ctx context.Context, input any) (any, error) {
		return "output", nil
	}

	wrapped := middleware(next)
	_, _ = wrapped(context.Background(), "input")

	if loggedName != "step" {
		t.Errorf("expected logged name 'step', got '%s'", loggedName)
	}

	if loggedInput != "input" {
		t.Errorf("expected logged input 'input', got '%v'", loggedInput)
	}

	if loggedOutput != "output" {
		t.Errorf("expected logged output 'output', got '%v'", loggedOutput)
	}

	if loggedErr != nil {
		t.Errorf("expected nil error, got %v", loggedErr)
	}
}

func TestRetryMiddleware(t *testing.T) {
	attempts := 0

	middleware := RetryMiddleware(3, func(err error) bool {
		return err.Error() == "retry"
	})

	// 测试成功重试
	next := func(ctx context.Context, input any) (any, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("retry")
		}
		return "success", nil
	}

	wrapped := middleware(next)
	result, err := wrapped(context.Background(), "input")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result != "success" {
		t.Errorf("expected 'success', got '%v'", result)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryMiddlewareMaxExceeded(t *testing.T) {
	middleware := RetryMiddleware(2, func(err error) bool {
		return true // 总是重试
	})

	next := func(ctx context.Context, input any) (any, error) {
		return nil, errors.New("always fails")
	}

	wrapped := middleware(next)
	_, err := wrapped(context.Background(), "input")

	if err == nil {
		t.Error("expected max retries exceeded error")
	}
}

func TestRetryMiddlewareNoRetry(t *testing.T) {
	middleware := RetryMiddleware(3, func(err error) bool {
		return false // 不重试
	})

	next := func(ctx context.Context, input any) (any, error) {
		return nil, errors.New("no retry")
	}

	wrapped := middleware(next)
	_, err := wrapped(context.Background(), "input")

	if err == nil {
		t.Error("expected error without retry")
	}
}

func TestRecoverMiddleware(t *testing.T) {
	middleware := RecoverMiddleware()

	next := func(ctx context.Context, input any) (any, error) {
		panic("test panic")
	}

	wrapped := middleware(next)
	_, err := wrapped(context.Background(), "input")

	if err == nil {
		t.Error("expected recovered error")
	}

	if err.Error() != "panic recovered: test panic" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRecoverMiddlewareNoPanic(t *testing.T) {
	middleware := RecoverMiddleware()

	next := func(ctx context.Context, input any) (any, error) {
		return "success", nil
	}

	wrapped := middleware(next)
	result, err := wrapped(context.Background(), "input")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result != "success" {
		t.Errorf("expected 'success', got '%v'", result)
	}
}

func TestTypedStep(t *testing.T) {
	step := NewTypedStep("double", func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	if step.name != "double" {
		t.Errorf("expected name 'double', got '%s'", step.name)
	}

	// 测试 ToStep
	genericStep := step.ToStep()
	result, err := genericStep.handler(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != 10 {
		t.Errorf("expected 10, got %v", result)
	}
}

func TestTypedStepTypeMismatch(t *testing.T) {
	step := NewTypedStep("int-step", func(ctx context.Context, input int) (int, error) {
		return input, nil
	})

	genericStep := step.ToStep()
	_, err := genericStep.handler(context.Background(), "not an int")

	if err == nil {
		t.Error("expected type mismatch error")
	}
}

func TestThen(t *testing.T) {
	first := NewTypedStep("add-one", func(ctx context.Context, input int) (int, error) {
		return input + 1, nil
	})

	second := NewTypedStep("double", func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	combined := Then(first, second)

	if combined.name != "add-one -> double" {
		t.Errorf("expected combined name, got '%s'", combined.name)
	}

	result, err := combined.handler(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (5 + 1) * 2 = 12
	if result != 12 {
		t.Errorf("expected 12, got %d", result)
	}
}

func TestThenWithError(t *testing.T) {
	first := NewTypedStep("fail", func(ctx context.Context, input int) (int, error) {
		return 0, errors.New("first failed")
	})

	second := NewTypedStep("never-called", func(ctx context.Context, input int) (int, error) {
		return input, nil
	})

	combined := Then(first, second)

	_, err := combined.handler(context.Background(), 5)
	if err == nil {
		t.Error("expected error from first step")
	}
}

func TestParallelCreation(t *testing.T) {
	parallel := NewParallel[int, int]("test-parallel", func(results []int) int {
		sum := 0
		for _, r := range results {
			sum += r
		}
		return sum
	})

	if parallel.name != "test-parallel" {
		t.Errorf("expected name 'test-parallel', got '%s'", parallel.name)
	}
}

func TestParallelRun(t *testing.T) {
	parallel := NewParallel[int, int]("sum-parallel", func(results []int) int {
		sum := 0
		for _, r := range results {
			sum += r
		}
		return sum
	})

	parallel.Add(func(ctx context.Context, input int) (int, error) {
		return input * 1, nil
	})
	parallel.Add(func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})
	parallel.Add(func(ctx context.Context, input int) (int, error) {
		return input * 3, nil
	})

	result, err := parallel.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10*1 + 10*2 + 10*3 = 60
	if result != 60 {
		t.Errorf("expected 60, got %d", result)
	}
}

func TestParallelRunNoHandlers(t *testing.T) {
	parallel := NewParallel[int, int]("empty-parallel", func(results []int) int {
		return 0
	})

	_, err := parallel.Run(context.Background(), 10)
	if err == nil {
		t.Error("expected error for parallel with no handlers")
	}
}

func TestParallelRunWithError(t *testing.T) {
	parallel := NewParallel[int, int]("error-parallel", func(results []int) int {
		return 0
	})

	parallel.Add(func(ctx context.Context, input int) (int, error) {
		return 0, errors.New("handler failed")
	})

	_, err := parallel.Run(context.Background(), 10)
	if err == nil {
		t.Error("expected error from parallel handler")
	}
}
