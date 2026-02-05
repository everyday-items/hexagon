package mcp

import (
	"testing"

	"github.com/everyday-items/ai-core/schema"
)

// TestSchemaToMCP 测试 ai-core Schema 到 MCP JSONSchema 的转换
func TestSchemaToMCP(t *testing.T) {
	tests := []struct {
		name     string
		input    *schema.Schema
		expected *JSONSchema
	}{
		{
			name:     "nil schema",
			input:    nil,
			expected: nil,
		},
		{
			name: "simple string",
			input: &schema.Schema{
				Type:        "string",
				Description: "A string field",
			},
			expected: &JSONSchema{
				Type:        "string",
				Description: "A string field",
			},
		},
		{
			name: "object with properties",
			input: &schema.Schema{
				Type: "object",
				Properties: map[string]*schema.Schema{
					"name": {
						Type:        "string",
						Description: "用户名",
					},
					"age": {
						Type:        "integer",
						Description: "年龄",
					},
				},
				Required: []string{"name"},
			},
			expected: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"name": {
						Type:        "string",
						Description: "用户名",
					},
					"age": {
						Type:        "integer",
						Description: "年龄",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			name: "array with items",
			input: &schema.Schema{
				Type: "array",
				Items: &schema.Schema{
					Type: "string",
				},
			},
			expected: &JSONSchema{
				Type: "array",
				Items: &JSONSchema{
					Type: "string",
				},
			},
		},
		{
			name: "with enum",
			input: &schema.Schema{
				Type: "string",
				Enum: []any{"red", "green", "blue"},
			},
			expected: &JSONSchema{
				Type: "string",
				Enum: []any{"red", "green", "blue"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SchemaToMCP(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected %+v, got nil", tt.expected)
				return
			}

			// 检查类型
			if result.Type != tt.expected.Type {
				t.Errorf("type: expected %s, got %s", tt.expected.Type, result.Type)
			}

			// 检查描述
			if result.Description != tt.expected.Description {
				t.Errorf("description: expected %s, got %s", tt.expected.Description, result.Description)
			}

			// 检查 Required
			if len(result.Required) != len(tt.expected.Required) {
				t.Errorf("required length: expected %d, got %d", len(tt.expected.Required), len(result.Required))
			}

			// 检查 Properties
			if len(result.Properties) != len(tt.expected.Properties) {
				t.Errorf("properties length: expected %d, got %d", len(tt.expected.Properties), len(result.Properties))
			}

			// 检查 Enum
			if len(result.Enum) != len(tt.expected.Enum) {
				t.Errorf("enum length: expected %d, got %d", len(tt.expected.Enum), len(result.Enum))
			}
		})
	}
}

// TestSchemaFromMCP 测试 MCP JSONSchema 到 ai-core Schema 的转换
func TestSchemaFromMCP(t *testing.T) {
	tests := []struct {
		name     string
		input    *JSONSchema
		expected *schema.Schema
	}{
		{
			name:     "nil schema",
			input:    nil,
			expected: nil,
		},
		{
			name: "simple string",
			input: &JSONSchema{
				Type:        "string",
				Description: "A string field",
			},
			expected: &schema.Schema{
				Type:        "string",
				Description: "A string field",
			},
		},
		{
			name: "object with properties",
			input: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"query": {
						Type:        "string",
						Description: "搜索关键词",
					},
				},
				Required: []string{"query"},
			},
			expected: &schema.Schema{
				Type: "object",
				Properties: map[string]*schema.Schema{
					"query": {
						Type:        "string",
						Description: "搜索关键词",
					},
				},
				Required: []string{"query"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SchemaFromMCP(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected %+v, got nil", tt.expected)
				return
			}

			// 检查类型
			if result.Type != tt.expected.Type {
				t.Errorf("type: expected %s, got %s", tt.expected.Type, result.Type)
			}

			// 检查描述
			if result.Description != tt.expected.Description {
				t.Errorf("description: expected %s, got %s", tt.expected.Description, result.Description)
			}

			// 检查 Required
			if len(result.Required) != len(tt.expected.Required) {
				t.Errorf("required length: expected %d, got %d", len(tt.expected.Required), len(result.Required))
			}

			// 检查 Properties
			if len(result.Properties) != len(tt.expected.Properties) {
				t.Errorf("properties length: expected %d, got %d", len(tt.expected.Properties), len(result.Properties))
			}
		})
	}
}

// TestSchemaRoundTrip 测试双向转换的一致性
func TestSchemaRoundTrip(t *testing.T) {
	original := &schema.Schema{
		Type:        "object",
		Description: "用户输入",
		Properties: map[string]*schema.Schema{
			"name": {
				Type:        "string",
				Description: "用户名",
			},
			"tags": {
				Type: "array",
				Items: &schema.Schema{
					Type: "string",
				},
			},
		},
		Required: []string{"name"},
	}

	// ai-core -> MCP -> ai-core
	mcpSchema := SchemaToMCP(original)
	result := SchemaFromMCP(mcpSchema)

	// 验证转换结果
	if result.Type != original.Type {
		t.Errorf("type mismatch: expected %s, got %s", original.Type, result.Type)
	}

	if result.Description != original.Description {
		t.Errorf("description mismatch: expected %s, got %s", original.Description, result.Description)
	}

	if len(result.Properties) != len(original.Properties) {
		t.Errorf("properties count mismatch: expected %d, got %d", len(original.Properties), len(result.Properties))
	}

	if len(result.Required) != len(original.Required) {
		t.Errorf("required count mismatch: expected %d, got %d", len(original.Required), len(result.Required))
	}

	// 检查嵌套的 array items
	if result.Properties["tags"] == nil || result.Properties["tags"].Items == nil {
		t.Error("nested array items lost in round trip")
	}
}

// TestToolToMCPTool 测试工具转换
func TestToolToMCPTool(t *testing.T) {
	// 创建一个模拟的工具
	mockTool := &mockToolForSchema{
		name:        "search",
		description: "搜索工具",
		schema: &schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Schema{
				"query": {
					Type:        "string",
					Description: "搜索关键词",
				},
			},
			Required: []string{"query"},
		},
	}

	mcpTool := ToolToMCPTool(mockTool)

	if mcpTool.Name != "search" {
		t.Errorf("name: expected search, got %s", mcpTool.Name)
	}

	if mcpTool.Description != "搜索工具" {
		t.Errorf("description: expected 搜索工具, got %s", mcpTool.Description)
	}

	if mcpTool.InputSchema == nil {
		t.Error("inputSchema should not be nil")
	}

	if mcpTool.InputSchema.Type != "object" {
		t.Errorf("inputSchema type: expected object, got %s", mcpTool.InputSchema.Type)
	}

	if len(mcpTool.InputSchema.Required) != 1 {
		t.Errorf("required length: expected 1, got %d", len(mcpTool.InputSchema.Required))
	}
}

// mockToolForSchema 用于测试的模拟工具
type mockToolForSchema struct {
	name        string
	description string
	schema      *schema.Schema
}

func (m *mockToolForSchema) Name() string {
	return m.name
}

func (m *mockToolForSchema) Description() string {
	return m.description
}

func (m *mockToolForSchema) Schema() *schema.Schema {
	return m.schema
}
