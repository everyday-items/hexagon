// Package loader 提供 RAG 系统的文档加载器
//
// ocr.go 实现高级文档解析 (OCR) 能力：
//   - OCRLoader: 通用 OCR 加载器，支持图片和扫描 PDF
//   - OCREngine: OCR 引擎接口，可对接 Tesseract、PaddleOCR 等
//   - TesseractEngine: Tesseract OCR 引擎实现
//   - VisionLLMEngine: 基于多模态 LLM 的 OCR（如 GPT-4V）
//
// 对标 LlamaIndex 的 LlamaParse 高级文档解析能力。
//
// 使用示例：
//
//	// 方式 1: 使用 Tesseract
//	engine := NewTesseractEngine(WithTesseractLang("chi_sim+eng"))
//	loader := NewOCRLoader("scan.pdf", engine)
//	docs, err := loader.Load(ctx)
//
//	// 方式 2: 使用多模态 LLM
//	engine := NewVisionLLMEngine(llmProvider, "gpt-4-vision-preview")
//	loader := NewOCRLoader("photo.png", engine)
//	docs, err := loader.Load(ctx)
package loader

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/llm"
	"github.com/everyday-items/ai-core/template"
	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== OCR 引擎接口 ==============

// OCREngine OCR 引擎接口
// 实现此接口以对接不同的 OCR 后端
type OCREngine interface {
	// ExtractText 从文件中提取文字
	// filePath: 图片或 PDF 文件路径
	// 返回提取的文字内容和可能的元数据
	ExtractText(ctx context.Context, filePath string) (*OCRResult, error)

	// Name 返回引擎名称
	Name() string

	// SupportedFormats 返回支持的文件格式
	SupportedFormats() []string
}

// OCRResult OCR 提取结果
type OCRResult struct {
	// Text 提取的完整文本
	Text string

	// Pages 分页结果（PDF 场景）
	Pages []OCRPage

	// Language 检测到的语言
	Language string

	// Confidence 整体置信度 (0-1)
	Confidence float64

	// Metadata 额外元数据
	Metadata map[string]any
}

// OCRPage 单页 OCR 结果
type OCRPage struct {
	// PageNum 页码（从 1 开始）
	PageNum int

	// Text 该页文本
	Text string

	// Confidence 该页置信度
	Confidence float64
}

// ============== Tesseract OCR 引擎 ==============

// TesseractEngine Tesseract OCR 引擎
// 需要系统安装 tesseract 命令行工具
type TesseractEngine struct {
	// tesseractPath tesseract 可执行文件路径
	tesseractPath string

	// language 识别语言（如 "chi_sim+eng"）
	language string

	// psm 页面分割模式
	psm string

	// oem OCR 引擎模式
	oem string
}

// TesseractOption Tesseract 选项
type TesseractOption func(*TesseractEngine)

// WithTesseractPath 设置 tesseract 路径
func WithTesseractPath(path string) TesseractOption {
	return func(e *TesseractEngine) {
		e.tesseractPath = path
	}
}

// WithTesseractLang 设置识别语言
func WithTesseractLang(lang string) TesseractOption {
	return func(e *TesseractEngine) {
		e.language = lang
	}
}

// WithTesseractPSM 设置页面分割模式
func WithTesseractPSM(psm string) TesseractOption {
	return func(e *TesseractEngine) {
		e.psm = psm
	}
}

// NewTesseractEngine 创建 Tesseract OCR 引擎
func NewTesseractEngine(opts ...TesseractOption) *TesseractEngine {
	e := &TesseractEngine{
		tesseractPath: "tesseract",
		language:      "eng",
		psm:           "3",
		oem:           "3",
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExtractText 使用 Tesseract 提取文字
func (e *TesseractEngine) ExtractText(ctx context.Context, filePath string) (*OCRResult, error) {
	// 检查 tesseract 是否可用
	if _, err := exec.LookPath(e.tesseractPath); err != nil {
		return nil, fmt.Errorf("tesseract 未安装或不在 PATH 中: %w (请安装: brew install tesseract 或 apt install tesseract-ocr)", err)
	}

	// 构建命令参数
	args := []string{
		filePath,
		"stdout", // 输出到标准输出
		"-l", e.language,
		"--psm", e.psm,
		"--oem", e.oem,
	}

	cmd := exec.CommandContext(ctx, e.tesseractPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tesseract 执行失败: %w", err)
	}

	text := strings.TrimSpace(string(output))

	return &OCRResult{
		Text:       text,
		Language:   e.language,
		Confidence: 0.8, // tesseract 不直接返回置信度，使用估计值
		Metadata: map[string]any{
			"engine": "tesseract",
			"psm":    e.psm,
			"oem":    e.oem,
		},
	}, nil
}

func (e *TesseractEngine) Name() string { return "tesseract" }

func (e *TesseractEngine) SupportedFormats() []string {
	return []string{".png", ".jpg", ".jpeg", ".tiff", ".tif", ".bmp", ".gif", ".webp"}
}

// ============== Vision LLM OCR 引擎 ==============

// VisionLLMEngine 基于多模态 LLM 的 OCR 引擎
// 使用 GPT-4V、Gemini Vision 等多模态模型提取文字
type VisionLLMEngine struct {
	provider    llm.Provider
	model       string
	systemPrompt string
}

// VisionLLMOption Vision LLM 选项
type VisionLLMOption func(*VisionLLMEngine)

// WithVisionModel 设置视觉模型
func WithVisionModel(model string) VisionLLMOption {
	return func(e *VisionLLMEngine) {
		e.model = model
	}
}

// WithVisionSystemPrompt 设置系统提示词
func WithVisionSystemPrompt(prompt string) VisionLLMOption {
	return func(e *VisionLLMEngine) {
		e.systemPrompt = prompt
	}
}

// NewVisionLLMEngine 创建 Vision LLM OCR 引擎
func NewVisionLLMEngine(provider llm.Provider, opts ...VisionLLMOption) *VisionLLMEngine {
	e := &VisionLLMEngine{
		provider: provider,
		systemPrompt: `你是一个专业的文档OCR助手。请仔细识别图片中的所有文字内容，保持原有的格式和结构。
如果包含表格，请用 Markdown 表格格式输出。
如果包含公式，请用 LaTeX 格式输出。
只输出识别到的文字内容，不要添加任何解释。`,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExtractText 使用多模态 LLM 提取文字
// 将图片文件读取后 base64 编码，通过 MultiContent 消息发送给 LLM
func (e *VisionLLMEngine) ExtractText(ctx context.Context, filePath string) (*OCRResult, error) {
	// 读取图片文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 检测 MIME 类型并构建 base64 data URI
	mimeType := detectMIMEType(filePath, data)
	b64Data := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data)

	// 构建多模态消息：文本提示 + 图片
	req := llm.CompletionRequest{
		Model: e.model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: e.systemPrompt},
			{
				Role: llm.RoleUser,
				MultiContent: []template.ContentPart{
					template.NewTextPart(fmt.Sprintf("请识别以下图片中的所有文字内容（文件名: %s）", filepath.Base(filePath))),
					template.NewImageURLPart(dataURI, "high"),
				},
			},
		},
		MaxTokens: 4000,
	}

	resp, err := e.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM OCR 失败: %w", err)
	}

	return &OCRResult{
		Text:       resp.Content,
		Confidence: 0.9,
		Metadata: map[string]any{
			"engine":    "vision_llm",
			"model":     e.model,
			"mime_type": mimeType,
			"file_size": len(data),
		},
	}, nil
}

// detectMIMEType 检测文件的 MIME 类型
// 优先根据文件扩展名判断，其次使用 http.DetectContentType 嗅探
func detectMIMEType(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".pdf":
		return "application/pdf"
	default:
		// 降级：使用内容嗅探
		return http.DetectContentType(data)
	}
}

func (e *VisionLLMEngine) Name() string { return "vision_llm" }

func (e *VisionLLMEngine) SupportedFormats() []string {
	return []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".pdf"}
}

// ============== OCR 加载器 ==============

// OCRLoader OCR 文档加载器
// 使用 OCR 引擎从图片或扫描文档中提取文字
type OCRLoader struct {
	filePath string
	engine   OCREngine
	metadata map[string]any
}

// OCRLoaderOption OCR 加载器选项
type OCRLoaderOption func(*OCRLoader)

// WithOCRMetadata 设置加载元数据
func WithOCRMetadata(key string, value any) OCRLoaderOption {
	return func(l *OCRLoader) {
		l.metadata[key] = value
	}
}

// NewOCRLoader 创建 OCR 加载器
func NewOCRLoader(filePath string, engine OCREngine, opts ...OCRLoaderOption) *OCRLoader {
	l := &OCRLoader{
		filePath: filePath,
		engine:   engine,
		metadata: make(map[string]any),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 使用 OCR 加载文档
func (l *OCRLoader) Load(ctx context.Context) ([]rag.Document, error) {
	// 检查文件是否存在
	info, err := os.Stat(l.filePath)
	if err != nil {
		return nil, fmt.Errorf("文件不存在: %w", err)
	}

	// 检查格式是否支持
	ext := strings.ToLower(filepath.Ext(l.filePath))
	supported := false
	for _, f := range l.engine.SupportedFormats() {
		if f == ext {
			supported = true
			break
		}
	}
	if !supported {
		return nil, fmt.Errorf("OCR 引擎 %q 不支持格式 %q，支持的格式: %v",
			l.engine.Name(), ext, l.engine.SupportedFormats())
	}

	// 执行 OCR
	result, err := l.engine.ExtractText(ctx, l.filePath)
	if err != nil {
		return nil, fmt.Errorf("OCR 提取失败: %w", err)
	}

	// 构建文档元数据
	metadata := map[string]any{
		"source":      l.filePath,
		"ocr_engine":  l.engine.Name(),
		"confidence":  result.Confidence,
		"file_size":   info.Size(),
	}
	if result.Language != "" {
		metadata["language"] = result.Language
	}
	for k, v := range result.Metadata {
		metadata["ocr_"+k] = v
	}
	for k, v := range l.metadata {
		metadata[k] = v
	}

	// 如果有分页结果，每页一个文档
	if len(result.Pages) > 0 {
		var docs []rag.Document
		for _, page := range result.Pages {
			pageMetadata := make(map[string]any, len(metadata)+2)
			for k, v := range metadata {
				pageMetadata[k] = v
			}
			pageMetadata["page_num"] = page.PageNum
			pageMetadata["page_confidence"] = page.Confidence

			docs = append(docs, rag.Document{
				ID:        util.GenerateID("ocr"),
				Content:   page.Text,
				Metadata:  pageMetadata,
				Source:    l.filePath,
				CreatedAt: time.Now(),
			})
		}
		return docs, nil
	}

	// 单文档结果
	return []rag.Document{
		{
			ID:        util.GenerateID("ocr"),
			Content:   result.Text,
			Metadata:  metadata,
			Source:    l.filePath,
			CreatedAt: time.Now(),
		},
	}, nil
}

// Name 返回加载器名称
func (l *OCRLoader) Name() string {
	return "ocr_loader"
}
