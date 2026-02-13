// Package main 演示 Hexagon 链式编排
//
// Chain 将多个处理步骤串联，数据依次流过每个步骤。
// 相比 Graph，Chain 更加简单直观，适合线性处理管道。
// 支持中间件机制（日志、重试、异常恢复等）。
//
// 运行方式:
//
//	go run ./examples/chain/
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/orchestration/chain"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: 文本处理链 ===")
	runTextChain(ctx)

	fmt.Println("\n=== 示例 2: 带中间件的链 ===")
	runMiddlewareChain(ctx)
}

// runTextChain 文本处理链: 标准化 -> 去重 -> 格式化
func runTextChain(ctx context.Context) {
	c, err := chain.NewChain[string, string]("text-pipeline").
		WithDescription("文本处理管道").
		PipeFunc("normalize", func(ctx context.Context, input any) (any, error) {
			text := input.(string)
			fmt.Printf("  [normalize] 输入: %q\n", text)
			return strings.TrimSpace(strings.ToLower(text)), nil
		}).
		PipeFunc("deduplicate", func(ctx context.Context, input any) (any, error) {
			words := strings.Fields(input.(string))
			seen := make(map[string]bool)
			var unique []string
			for _, w := range words {
				if !seen[w] {
					seen[w] = true
					unique = append(unique, w)
				}
			}
			result := strings.Join(unique, " ")
			fmt.Printf("  [deduplicate] 去重: %q\n", result)
			return result, nil
		}).
		PipeFunc("format", func(ctx context.Context, input any) (any, error) {
			words := strings.Fields(input.(string))
			result := fmt.Sprintf("共 %d 个唯一词: [%s]", len(words), strings.Join(words, ", "))
			return result, nil
		}).
		Build()
	if err != nil {
		log.Fatalf("构建链失败: %v", err)
	}

	result, err := c.Invoke(ctx, "  Hello WORLD hello Go go GO  ")
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("  结果: %s\n", result)
}

// runMiddlewareChain 带日志和异常恢复中间件的链
func runMiddlewareChain(ctx context.Context) {
	// 日志中间件
	logging := func(next chain.StepFunc) chain.StepFunc {
		return func(ctx context.Context, input any) (any, error) {
			start := time.Now()
			fmt.Printf("  [log] 开始, 输入: %T\n", input)
			result, err := next(ctx, input)
			fmt.Printf("  [log] 完成, 耗时: %v\n", time.Since(start))
			return result, err
		}
	}

	// 异常恢复中间件
	recoverMw := func(next chain.StepFunc) chain.StepFunc {
		return func(ctx context.Context, input any) (result any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return next(ctx, input)
		}
	}

	c, _ := chain.NewChain[string, string]("safe-pipeline").
		Use(logging).
		Use(recoverMw).
		PipeFunc("upper", func(ctx context.Context, input any) (any, error) {
			return strings.ToUpper(input.(string)), nil
		}).
		PipeFunc("reverse", func(ctx context.Context, input any) (any, error) {
			runes := []rune(input.(string))
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return string(runes), nil
		}).
		Build()

	result, err := c.Invoke(ctx, "hexagon")
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("  结果: %s\n", result)
}
