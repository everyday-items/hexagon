package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hexagon-codes/ai-core/llm"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// PreservedToolResult 保留完整协议结果，并提供不含媒体编码的文本投影。
type PreservedToolResult struct{ *sdkmcp.CallToolResult }

func (r *PreservedToolResult) ToolText() string {
	var text []string
	for _, content := range r.Content {
		switch item := content.(type) {
		case *sdkmcp.TextContent:
			text = append(text, item.Text)
		case *sdkmcp.ImageContent:
			text = append(text, "[MCP image content]")
		case *sdkmcp.ResourceLink:
			text = append(text, item.URI)
		}
	}
	if r.StructuredContent != nil {
		if encoded, err := json.Marshal(r.StructuredContent); err == nil {
			text = append(text, string(encoded))
		}
	}
	return strings.Join(text, "\n")
}

// ToolMessageContent 只承载上游实际返回的图片，不附加解题或批改指令。
func (r *PreservedToolResult) ToolMessageContent() []llm.ContentPart {
	var parts []llm.ContentPart
	for _, content := range r.Content {
		if item, ok := content.(*sdkmcp.ImageContent); ok {
			parts = append(parts, llm.NewImageURLPart(fmt.Sprintf("data:%s;base64,%s", item.MIMEType, base64.StdEncoding.EncodeToString(item.Data)), "auto"))
		}
	}
	return parts
}
