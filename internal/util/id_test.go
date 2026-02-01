package util

import (
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	id := GenerateID("test")

	if !strings.HasPrefix(id, "test-") {
		t.Errorf("expected ID to start with 'test-', got '%s'", id)
	}

	// ID 应该有合理的长度
	if len(id) < 10 {
		t.Errorf("expected ID length >= 10, got %d", len(id))
	}
}

func TestGenerateIDLong(t *testing.T) {
	id := GenerateIDLong("trace")

	if !strings.HasPrefix(id, "trace-") {
		t.Errorf("expected ID to start with 'trace-', got '%s'", id)
	}

	// 长 ID 应该比普通 ID 更长
	shortID := GenerateID("trace")
	if len(id) <= len(shortID) {
		t.Errorf("expected long ID to be longer than short ID")
	}
}

func TestAgentID(t *testing.T) {
	id := AgentID()

	if !strings.HasPrefix(id, "agent-") {
		t.Errorf("expected AgentID to start with 'agent-', got '%s'", id)
	}
}

func TestSessionID(t *testing.T) {
	id := SessionID()

	if !strings.HasPrefix(id, "session-") {
		t.Errorf("expected SessionID to start with 'session-', got '%s'", id)
	}
}

func TestTraceID(t *testing.T) {
	id := TraceID()

	if !strings.HasPrefix(id, "trace-") {
		t.Errorf("expected TraceID to start with 'trace-', got '%s'", id)
	}
}

func TestSpanID(t *testing.T) {
	id := SpanID()

	// SpanID 不应该有前缀
	if strings.Contains(id, "-") && strings.HasPrefix(id, "span") {
		t.Errorf("SpanID should not have 'span-' prefix, got '%s'", id)
	}

	// 应该有合理的长度
	if len(id) < 5 {
		t.Errorf("expected SpanID length >= 5, got %d", len(id))
	}
}

func TestRandomString(t *testing.T) {
	lengths := []int{8, 16, 32}

	for _, length := range lengths {
		str := RandomString(length)
		if len(str) != length {
			t.Errorf("expected length %d, got %d", length, len(str))
		}
	}
}

func TestRandomHex(t *testing.T) {
	str := RandomHex(8)

	// 长度应该是 16 (8 * 2)
	if len(str) != 16 {
		t.Errorf("expected length 16, got %d", len(str))
	}

	// 应该只包含十六进制字符
	for _, c := range str {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("expected hex character, got '%c'", c)
		}
	}
}

func TestUniqueIDs(t *testing.T) {
	// 生成多个 ID，确保它们都是唯一的
	ids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id := GenerateID("unique")
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}
