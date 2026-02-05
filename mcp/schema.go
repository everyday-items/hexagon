// Package mcp 实现 Model Context Protocol (MCP) 支持
//
// 本文件实现 ai-core Schema 与 MCP JSONSchema 之间的双向转换
package mcp

import (
	"github.com/everyday-items/ai-core/schema"
)

// SchemaToMCP 将 ai-core Schema 转换为 MCP JSONSchema
//
// 支持的字段映射：
//   - Type -> Type
//   - Description -> Description
//   - Properties -> Properties (递归转换)
//   - Required -> Required
//   - Items -> Items (数组元素)
//   - Enum -> Enum
//
// 示例：
//
//	aiSchema := schema.Of[MyInput]()
//	mcpSchema := mcp.SchemaToMCP(aiSchema)
func SchemaToMCP(s *schema.Schema) *JSONSchema {
	if s == nil {
		return nil
	}

	js := &JSONSchema{
		Type:        s.Type,
		Description: s.Description,
	}

	// 转换 Properties
	if len(s.Properties) > 0 {
		js.Properties = make(map[string]*JSONSchema)
		for name, prop := range s.Properties {
			js.Properties[name] = SchemaToMCP(prop)
		}
	}

	// 转换 Required
	if len(s.Required) > 0 {
		js.Required = make([]string, len(s.Required))
		copy(js.Required, s.Required)
	}

	// 转换 Items (数组元素 Schema)
	if s.Items != nil {
		js.Items = SchemaToMCP(s.Items)
	}

	// 转换 Enum
	if len(s.Enum) > 0 {
		js.Enum = make([]any, len(s.Enum))
		copy(js.Enum, s.Enum)
	}

	return js
}

// SchemaFromMCP 将 MCP JSONSchema 转换为 ai-core Schema
//
// 支持的字段映射：
//   - Type -> Type
//   - Description -> Description
//   - Properties -> Properties (递归转换)
//   - Required -> Required
//   - Items -> Items (数组元素)
//   - Enum -> Enum
//
// 示例：
//
//	mcpTools, _ := client.ListTools(ctx)
//	for _, t := range mcpTools {
//	    aiSchema := mcp.SchemaFromMCP(t.InputSchema)
//	}
func SchemaFromMCP(js *JSONSchema) *schema.Schema {
	if js == nil {
		return nil
	}

	s := &schema.Schema{
		Type:        js.Type,
		Description: js.Description,
	}

	// 转换 Properties
	if len(js.Properties) > 0 {
		s.Properties = make(map[string]*schema.Schema)
		for name, prop := range js.Properties {
			s.Properties[name] = SchemaFromMCP(prop)
		}
	}

	// 转换 Required
	if len(js.Required) > 0 {
		s.Required = make([]string, len(js.Required))
		copy(s.Required, js.Required)
	}

	// 转换 Items (数组元素 Schema)
	if js.Items != nil {
		s.Items = SchemaFromMCP(js.Items)
	}

	// 转换 Enum
	if len(js.Enum) > 0 {
		s.Enum = make([]any, len(js.Enum))
		copy(s.Enum, js.Enum)
	}

	return s
}

// ToolToMCPTool 将 ai-core tool.Tool 转换为 MCP Tool 定义
//
// 这是一个便捷函数，用于将 Hexagon 工具暴露为 MCP 服务时使用
//
// 示例：
//
//	calculator := tool.NewFunc("calc", "计算器", calcFn)
//	mcpTool := mcp.ToolToMCPTool(calculator)
func ToolToMCPTool(t interface {
	Name() string
	Description() string
	Schema() *schema.Schema
}) Tool {
	return Tool{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: SchemaToMCP(t.Schema()),
	}
}
