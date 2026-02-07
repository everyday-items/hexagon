// Package splitter 提供 RAG 系统的文档分割器
//
// code.go 实现代码感知分割器，能够根据编程语言的语法结构进行智能分割。
// 支持的语言：Go、Python、JavaScript/TypeScript、Java、Rust、C/C++、Ruby、PHP
//
// 核心特性：
//   - 按函数/方法/类级别分割，保持代码逻辑完整性
//   - 自动检测编程语言
//   - 保留导入语句和包声明上下文
//   - 每个分块包含必要的上下文信息
//
// 使用示例：
//
//	splitter := NewCodeSplitter(
//	    WithLanguage(LangGo),
//	    WithCodeChunkSize(2000),
//	)
//	chunks, err := splitter.Split(ctx, docs)
package splitter

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// Language 编程语言类型
type Language int

const (
	// LangAuto 自动检测语言
	LangAuto Language = iota
	// LangGo Go 语言
	LangGo
	// LangPython Python 语言
	LangPython
	// LangJavaScript JavaScript 语言
	LangJavaScript
	// LangTypeScript TypeScript 语言
	LangTypeScript
	// LangJava Java 语言
	LangJava
	// LangRust Rust 语言
	LangRust
	// LangCPP C/C++ 语言
	LangCPP
	// LangRuby Ruby 语言
	LangRuby
	// LangPHP PHP 语言
	LangPHP
)

// languageSeparators 各语言的分割标记（按优先级排序）
// 借鉴 LangChain RecursiveCharacterTextSplitter.from_language 的设计
var languageSeparators = map[Language][]string{
	LangGo: {
		"\nfunc ",    // 函数定义
		"\ntype ",    // 类型定义
		"\nvar ",     // 变量声明
		"\nconst ",   // 常量声明
		"\npackage ", // 包声明
		"\n\n",       // 空行
		"\n",         // 换行
	},
	LangPython: {
		"\nclass ",   // 类定义
		"\ndef ",     // 函数定义
		"\n\tdef ",   // 方法定义
		"\n    def ",  // 方法定义（4 空格缩进）
		"\n\n",
		"\n",
	},
	LangJavaScript: {
		"\nfunction ",    // 函数声明
		"\nconst ",       // 常量（箭头函数）
		"\nlet ",         // 变量
		"\nvar ",         // 变量
		"\nclass ",       // 类
		"\nexport ",      // 导出
		"\n\n",
		"\n",
	},
	LangTypeScript: {
		"\ninterface ",   // 接口
		"\ntype ",        // 类型
		"\nfunction ",    // 函数
		"\nconst ",       // 常量
		"\nclass ",       // 类
		"\nexport ",      // 导出
		"\n\n",
		"\n",
	},
	LangJava: {
		"\npublic ",      // 公有成员
		"\nprivate ",     // 私有成员
		"\nprotected ",   // 保护成员
		"\nclass ",       // 类
		"\ninterface ",   // 接口
		"\nenum ",        // 枚举
		"\n\n",
		"\n",
	},
	LangRust: {
		"\nfn ",          // 函数
		"\npub fn ",      // 公有函数
		"\nstruct ",      // 结构体
		"\nimpl ",        // 实现块
		"\nenum ",        // 枚举
		"\ntrait ",       // 特征
		"\nmod ",         // 模块
		"\n\n",
		"\n",
	},
	LangCPP: {
		"\nclass ",       // 类
		"\nvoid ",        // void 函数
		"\nint ",         // int 函数
		"\nnamespace ",   // 命名空间
		"\ntemplate",     // 模板
		"\n#",            // 预处理器
		"\n\n",
		"\n",
	},
	LangRuby: {
		"\nclass ",       // 类
		"\ndef ",         // 方法
		"\nmodule ",      // 模块
		"\n\n",
		"\n",
	},
	LangPHP: {
		"\nfunction ",    // 函数
		"\nclass ",       // 类
		"\ninterface ",   // 接口
		"\ntrait ",       // 特征
		"\nnamespace ",   // 命名空间
		"\n\n",
		"\n",
	},
}

// 语言检测正则
var langDetectPatterns = map[Language]*regexp.Regexp{
	LangGo:         regexp.MustCompile(`(?m)^package\s+\w+`),
	LangPython:     regexp.MustCompile(`(?m)^(import\s+\w+|from\s+\w+|def\s+\w+|class\s+\w+)`),
	LangJava:       regexp.MustCompile(`(?m)^(public\s+class|import\s+java)`),
	LangRust:       regexp.MustCompile(`(?m)^(use\s+\w+|fn\s+\w+|pub\s+fn|impl\s+)`),
	LangTypeScript: regexp.MustCompile(`(?m)(interface\s+\w+|:\s*(string|number|boolean))`),
	LangJavaScript: regexp.MustCompile(`(?m)^(const\s+\w+\s*=|function\s+\w+|import\s+.*\s+from)`),
	LangRuby:       regexp.MustCompile(`(?m)^(require\s+['"]|class\s+\w+|module\s+\w+)`),
	LangPHP:        regexp.MustCompile(`(?m)^<\?php`),
	LangCPP:        regexp.MustCompile(`(?m)^(#include\s+[<"]|namespace\s+\w+)`),
}

// CodeSplitter 代码感知分割器
// 根据编程语言的语法结构进行智能分割，保持代码逻辑完整性
type CodeSplitter struct {
	language     Language
	chunkSize    int
	chunkOverlap int
	keepHeader   bool // 是否在每个分块中保留头部（package/import）
}

// CodeOption CodeSplitter 配置选项
type CodeOption func(*CodeSplitter)

// WithLanguage 设置编程语言
func WithLanguage(lang Language) CodeOption {
	return func(s *CodeSplitter) {
		s.language = lang
	}
}

// WithCodeChunkSize 设置代码分块大小
func WithCodeChunkSize(size int) CodeOption {
	return func(s *CodeSplitter) {
		s.chunkSize = size
	}
}

// WithCodeChunkOverlap 设置代码分块重叠
func WithCodeChunkOverlap(overlap int) CodeOption {
	return func(s *CodeSplitter) {
		s.chunkOverlap = overlap
	}
}

// WithKeepHeader 是否在每个分块中保留文件头部
func WithKeepHeader(keep bool) CodeOption {
	return func(s *CodeSplitter) {
		s.keepHeader = keep
	}
}

// NewCodeSplitter 创建代码感知分割器
func NewCodeSplitter(opts ...CodeOption) *CodeSplitter {
	s := &CodeSplitter{
		language:     LangAuto,
		chunkSize:    2000,
		chunkOverlap: 200,
		keepHeader:   true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Split 分割文档
// 自动检测语言并按语法结构分割
func (s *CodeSplitter) Split(ctx context.Context, docs []rag.Document) ([]rag.Document, error) {
	var result []rag.Document

	for _, doc := range docs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		lang := s.language
		if lang == LangAuto {
			lang = detectLanguage(doc.Content, doc.Source)
		}

		chunks := s.splitCode(doc.Content, lang)

		for i, chunk := range chunks {
			newDoc := rag.Document{
				ID:      util.GenerateID("code"),
				Content: chunk,
				Source:  doc.Source,
				Metadata: mergeMetadata(doc.Metadata, map[string]any{
					"splitter":    "code",
					"language":    langName(lang),
					"chunk_index": i,
					"total_chunks": len(chunks),
				}),
				CreatedAt: time.Now(),
			}
			result = append(result, newDoc)
		}
	}

	return result, nil
}

// Name 返回分割器名称
func (s *CodeSplitter) Name() string {
	return "CodeSplitter"
}

// splitCode 按语言特定分隔符分割代码
func (s *CodeSplitter) splitCode(content string, lang Language) []string {
	separators := languageSeparators[lang]
	if len(separators) == 0 {
		// 未知语言，使用通用分隔符
		separators = []string{"\n\n", "\n"}
	}

	// 提取头部（package/import）
	header := ""
	if s.keepHeader {
		header = extractHeader(content, lang)
	}

	// 递归分割
	chunks := recursiveSplit(content, separators, s.chunkSize, s.chunkOverlap)

	// 为每个非首块添加头部上下文
	if header != "" && len(chunks) > 1 {
		for i := 1; i < len(chunks); i++ {
			if !strings.HasPrefix(chunks[i], header) {
				chunks[i] = header + "\n// ... (续)\n\n" + chunks[i]
			}
		}
	}

	return chunks
}

// recursiveSplit 递归分割文本
func recursiveSplit(text string, separators []string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{strings.TrimSpace(text)}
	}

	if len(separators) == 0 {
		// 无分隔符，按大小硬切
		return hardSplit(text, chunkSize, overlap)
	}

	sep := separators[0]
	restSeps := separators[1:]

	parts := strings.Split(text, sep)

	var chunks []string
	var current strings.Builder

	for _, part := range parts {
		candidate := part
		if current.Len() > 0 {
			candidate = current.String() + sep + part
		}

		if len(candidate) > chunkSize && current.Len() > 0 {
			// 当前块已满，先保存
			chunk := strings.TrimSpace(current.String())
			if len(chunk) > chunkSize {
				// 还是太大，递归用下一级分隔符
				subChunks := recursiveSplit(chunk, restSeps, chunkSize, overlap)
				chunks = append(chunks, subChunks...)
			} else if chunk != "" {
				chunks = append(chunks, chunk)
			}
			current.Reset()
			current.WriteString(part)
		} else {
			current.Reset()
			current.WriteString(candidate)
		}
	}

	// 处理剩余
	if current.Len() > 0 {
		remaining := strings.TrimSpace(current.String())
		if len(remaining) > chunkSize {
			subChunks := recursiveSplit(remaining, restSeps, chunkSize, overlap)
			chunks = append(chunks, subChunks...)
		} else if remaining != "" {
			chunks = append(chunks, remaining)
		}
	}

	return chunks
}

// hardSplit 按固定大小分割
func hardSplit(text string, size, overlap int) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[i:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		i = end - overlap
		if i <= 0 && end == len(runes) {
			break
		}
	}
	return chunks
}

// extractHeader 提取代码文件头部（package/import 区域）
func extractHeader(content string, lang Language) string {
	lines := strings.Split(content, "\n")
	var headerLines []string

	switch lang {
	case LangGo:
		inImport := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "package ") {
				headerLines = append(headerLines, line)
				continue
			}
			if strings.HasPrefix(trimmed, "import") {
				inImport = true
				headerLines = append(headerLines, line)
				if strings.Contains(line, `"`) && !strings.Contains(line, "(") {
					break // 单行 import
				}
				continue
			}
			if inImport {
				headerLines = append(headerLines, line)
				if trimmed == ")" {
					break
				}
			}
			if trimmed == "" && len(headerLines) > 0 && !inImport {
				continue
			}
			if !inImport && len(headerLines) > 0 && !strings.HasPrefix(trimmed, "//") {
				break
			}
			if strings.HasPrefix(trimmed, "//") && len(headerLines) == 0 {
				headerLines = append(headerLines, line)
			}
		}

	case LangPython:
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
				headerLines = append(headerLines, line)
			} else if trimmed == "" && len(headerLines) > 0 {
				break
			} else if len(headerLines) > 0 {
				break
			}
		}

	case LangJavaScript, LangTypeScript:
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "const ") {
				headerLines = append(headerLines, line)
			} else if trimmed == "" && len(headerLines) > 0 {
				break
			} else if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "/*") && len(headerLines) > 0 {
				break
			}
		}

	case LangJava:
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "import ") {
				headerLines = append(headerLines, line)
			} else if trimmed == "" && len(headerLines) > 0 {
				continue
			} else if len(headerLines) > 0 {
				break
			}
		}

	default:
		// 通用：取前 5 行注释/空行
		for i, line := range lines {
			if i >= 5 {
				break
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				headerLines = append(headerLines, line)
			} else {
				break
			}
		}
	}

	return strings.Join(headerLines, "\n")
}

// detectLanguage 根据内容和文件名自动检测编程语言
func detectLanguage(content string, source string) Language {
	// 先根据文件扩展名检测
	if source != "" {
		if lang := detectByExtension(source); lang != LangAuto {
			return lang
		}
	}

	// 再根据内容检测
	for lang, pattern := range langDetectPatterns {
		if pattern.MatchString(content) {
			return lang
		}
	}

	return LangGo // 默认
}

// detectByExtension 根据文件扩展名检测语言
func detectByExtension(filename string) Language {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return LangGo
	case strings.HasSuffix(lower, ".py"):
		return LangPython
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"):
		return LangJavaScript
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return LangTypeScript
	case strings.HasSuffix(lower, ".java"):
		return LangJava
	case strings.HasSuffix(lower, ".rs"):
		return LangRust
	case strings.HasSuffix(lower, ".cpp"), strings.HasSuffix(lower, ".cc"),
		strings.HasSuffix(lower, ".c"), strings.HasSuffix(lower, ".h"):
		return LangCPP
	case strings.HasSuffix(lower, ".rb"):
		return LangRuby
	case strings.HasSuffix(lower, ".php"):
		return LangPHP
	default:
		return LangAuto
	}
}

// langName 返回语言名称
func langName(lang Language) string {
	names := map[Language]string{
		LangGo:         "go",
		LangPython:     "python",
		LangJavaScript: "javascript",
		LangTypeScript: "typescript",
		LangJava:       "java",
		LangRust:       "rust",
		LangCPP:        "cpp",
		LangRuby:       "ruby",
		LangPHP:        "php",
	}
	if name, ok := names[lang]; ok {
		return name
	}
	return "unknown"
}

// mergeMetadata 合并元数据
func mergeMetadata(base, extra map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

var _ rag.Splitter = (*CodeSplitter)(nil)
