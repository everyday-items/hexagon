package bench

import (
	"context"
	"strings"
	"testing"

	"github.com/everyday-items/hexagon/orchestration/chain"
)

// BenchmarkChainCreation 测试链创建和构建性能
func BenchmarkChainCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c, _ := chain.NewChain[string, string]("bench-chain").
			PipeFunc("step1", func(ctx context.Context, input any) (any, error) {
				return input, nil
			}).
			PipeFunc("step2", func(ctx context.Context, input any) (any, error) {
				return input, nil
			}).
			Build()
		_ = c
	}
}

// BenchmarkChainInvoke 测试链执行性能（3 步）
func BenchmarkChainInvoke(b *testing.B) {
	c, _ := chain.NewChain[string, string]("bench-chain").
		PipeFunc("lower", func(ctx context.Context, input any) (any, error) {
			return strings.ToLower(input.(string)), nil
		}).
		PipeFunc("trim", func(ctx context.Context, input any) (any, error) {
			return strings.TrimSpace(input.(string)), nil
		}).
		PipeFunc("upper", func(ctx context.Context, input any) (any, error) {
			return strings.ToUpper(input.(string)), nil
		}).
		Build()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Invoke(ctx, "  Hello World  ")
	}
}

// BenchmarkChainInvokeLong 测试长链执行性能（10 步）
func BenchmarkChainInvokeLong(b *testing.B) {
	builder := chain.NewChain[string, string]("bench-chain-10")
	for j := 0; j < 10; j++ {
		builder = builder.PipeFunc("step", func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
	}
	c, _ := builder.Build()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Invoke(ctx, "input")
	}
}

// BenchmarkChainWithMiddleware 测试带中间件的链性能
func BenchmarkChainWithMiddleware(b *testing.B) {
	nopMiddleware := func(next chain.StepFunc) chain.StepFunc {
		return func(ctx context.Context, input any) (any, error) {
			return next(ctx, input)
		}
	}

	c, _ := chain.NewChain[string, string]("bench-mw").
		Use(nopMiddleware).
		Use(nopMiddleware).
		PipeFunc("process", func(ctx context.Context, input any) (any, error) {
			return strings.ToUpper(input.(string)), nil
		}).
		Build()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Invoke(ctx, "hello")
	}
}

// BenchmarkChainInvokeConcurrent 测试链并发执行性能
func BenchmarkChainInvokeConcurrent(b *testing.B) {
	c, _ := chain.NewChain[string, string]("bench-concurrent").
		PipeFunc("process", func(ctx context.Context, input any) (any, error) {
			return strings.ToUpper(input.(string)), nil
		}).
		Build()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Invoke(ctx, "hello")
		}
	})
}
