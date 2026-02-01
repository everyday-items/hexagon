// Package main demonstrates the quickstart API of Hexagon framework.
//
// This example shows how to use Hexagon with just 3 lines of code.
//
// Usage:
//
//	export OPENAI_API_KEY=your-api-key
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/everyday-items/hexagon"
)

func main() {
	// 检查 API Key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("请设置 OPENAI_API_KEY 环境变量")
	}

	ctx := context.Background()

	// 3 行代码入门
	response, err := hexagon.Chat(ctx, "What is Go programming language?")
	if err != nil {
		log.Fatalf("Chat failed: %v", err)
	}

	fmt.Println("Response:")
	fmt.Println(response)
}
