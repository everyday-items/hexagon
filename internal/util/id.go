// Package util 提供 Hexagon 框架的内部工具函数
//
// 本包是内部实现，不对外暴露。主要功能：
//   - ID 生成：使用 toolkit/util/idgen 生成各类唯一标识
//   - 随机字符串：生成随机字符串和十六进制字符串
//
// 注意：本包遵循基础设施复用原则，所有 ID 生成都使用 toolkit 提供的实现
package util

import (
	"fmt"

	"github.com/everyday-items/toolkit/util/idgen"
)

// GenerateID generates a unique ID with the given prefix.
// Format: {prefix}-{nanoid}
// Example: agent-Uakgb_J5m9g
func GenerateID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, idgen.ShortID())
}

// GenerateIDLong generates a longer unique ID with the given prefix.
// Format: {prefix}-{nanoid_16}
// Example: agent-Uakgb_J5m9g-abcd
func GenerateIDLong(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, idgen.MediumID())
}

// AgentID generates a unique Agent ID
func AgentID() string {
	return GenerateID("agent")
}

// SessionID generates a unique Session ID
func SessionID() string {
	return GenerateID("session")
}

// TraceID generates a unique Trace ID (longer for distributed tracing)
func TraceID() string {
	return GenerateIDLong("trace")
}

// SpanID generates a unique Span ID
func SpanID() string {
	return idgen.ShortID()
}

// RandomString generates a random string of the given length.
// Delegates to toolkit's NanoID implementation.
func RandomString(n int) string {
	return idgen.NanoIDSize(n)
}

// RandomHex generates a random hex string.
// For compatibility, this uses NanoID with hex alphabet.
func RandomHex(n int) string {
	return idgen.NanoIDCustom("0123456789abcdef", n*2)[:n*2]
}
