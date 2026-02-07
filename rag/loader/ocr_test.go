package loader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ============== Mock OCR Engine ==============

// mockOCREngine 模拟 OCR 引擎
type mockOCREngine struct {
	result    *OCRResult
	err       error
	name      string
	formats   []string
	callCount int
}

func (m *mockOCREngine) ExtractText(ctx context.Context, filePath string) (*OCRResult, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockOCREngine) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock_ocr"
}

func (m *mockOCREngine) SupportedFormats() []string {
	if m.formats != nil {
		return m.formats
	}
	return []string{".png", ".jpg", ".jpeg"}
}

// ============== TesseractEngine 测试 ==============

func TestNewTesseractEngine_Defaults(t *testing.T) {
	e := NewTesseractEngine()
	if e.tesseractPath != "tesseract" {
		t.Errorf("expected default path 'tesseract', got %q", e.tesseractPath)
	}
	if e.language != "eng" {
		t.Errorf("expected default language 'eng', got %q", e.language)
	}
	if e.psm != "3" {
		t.Errorf("expected default psm '3', got %q", e.psm)
	}
	if e.oem != "3" {
		t.Errorf("expected default oem '3', got %q", e.oem)
	}
}

func TestNewTesseractEngine_WithOptions(t *testing.T) {
	e := NewTesseractEngine(
		WithTesseractPath("/usr/local/bin/tesseract"),
		WithTesseractLang("chi_sim+eng"),
		WithTesseractPSM("6"),
	)
	if e.tesseractPath != "/usr/local/bin/tesseract" {
		t.Errorf("expected custom path, got %q", e.tesseractPath)
	}
	if e.language != "chi_sim+eng" {
		t.Errorf("expected custom language, got %q", e.language)
	}
	if e.psm != "6" {
		t.Errorf("expected custom psm, got %q", e.psm)
	}
}

func TestTesseractEngine_Name(t *testing.T) {
	e := NewTesseractEngine()
	if e.Name() != "tesseract" {
		t.Errorf("expected name 'tesseract', got %q", e.Name())
	}
}

func TestTesseractEngine_SupportedFormats(t *testing.T) {
	e := NewTesseractEngine()
	formats := e.SupportedFormats()
	if len(formats) == 0 {
		t.Error("expected non-empty supported formats")
	}

	expectedFormats := []string{".png", ".jpg", ".jpeg", ".tiff", ".tif", ".bmp", ".gif", ".webp"}
	for _, expected := range expectedFormats {
		found := false
		for _, f := range formats {
			if f == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected format %q in supported formats", expected)
		}
	}
}

func TestTesseractEngine_ExtractText_NotInstalled(t *testing.T) {
	e := NewTesseractEngine(
		WithTesseractPath("nonexistent_binary_xyzzy"),
	)

	ctx := context.Background()
	_, err := e.ExtractText(ctx, "/tmp/test.png")
	if err == nil {
		t.Error("expected error for missing tesseract binary")
	}
}

// ============== VisionLLMEngine 测试 ==============

func TestNewVisionLLMEngine_Defaults(t *testing.T) {
	e := NewVisionLLMEngine(nil)
	if e.systemPrompt == "" {
		t.Error("expected non-empty default system prompt")
	}
}

func TestNewVisionLLMEngine_WithOptions(t *testing.T) {
	e := NewVisionLLMEngine(nil,
		WithVisionModel("gpt-4-vision"),
		WithVisionSystemPrompt("custom prompt"),
	)
	if e.model != "gpt-4-vision" {
		t.Errorf("expected model 'gpt-4-vision', got %q", e.model)
	}
	if e.systemPrompt != "custom prompt" {
		t.Errorf("expected custom prompt, got %q", e.systemPrompt)
	}
}

func TestVisionLLMEngine_Name(t *testing.T) {
	e := NewVisionLLMEngine(nil)
	if e.Name() != "vision_llm" {
		t.Errorf("expected name 'vision_llm', got %q", e.Name())
	}
}

func TestVisionLLMEngine_SupportedFormats(t *testing.T) {
	e := NewVisionLLMEngine(nil)
	formats := e.SupportedFormats()
	if len(formats) == 0 {
		t.Error("expected non-empty supported formats")
	}
}

func TestVisionLLMEngine_ExtractText_FileNotFound(t *testing.T) {
	e := NewVisionLLMEngine(nil)

	ctx := context.Background()
	_, err := e.ExtractText(ctx, "/nonexistent/file.png")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ============== detectMIMEType 测试 ==============

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"test.png", "image/png"},
		{"test.jpg", "image/jpeg"},
		{"test.jpeg", "image/jpeg"},
		{"test.gif", "image/gif"},
		{"test.webp", "image/webp"},
		{"test.bmp", "image/bmp"},
		{"test.tiff", "image/tiff"},
		{"test.tif", "image/tiff"},
		{"test.pdf", "application/pdf"},
		{"TEST.PNG", "image/png"}, // 大写扩展名
	}

	for _, tt := range tests {
		result := detectMIMEType(tt.path, nil)
		if result != tt.expected {
			t.Errorf("detectMIMEType(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestDetectMIMEType_Unknown(t *testing.T) {
	// 未知扩展名，使用内容嗅探
	data := []byte("<html><body>hello</body></html>")
	result := detectMIMEType("test.xyz", data)
	if result == "" {
		t.Error("expected non-empty MIME type from content detection")
	}
}

// ============== OCRLoader 测试 ==============

func TestNewOCRLoader(t *testing.T) {
	engine := &mockOCREngine{}
	loader := NewOCRLoader("/tmp/test.png", engine)

	if loader.filePath != "/tmp/test.png" {
		t.Errorf("expected filePath '/tmp/test.png', got %q", loader.filePath)
	}
	if loader.engine != engine {
		t.Error("expected engine to be set")
	}
}

func TestNewOCRLoader_WithMetadata(t *testing.T) {
	engine := &mockOCREngine{}
	loader := NewOCRLoader("/tmp/test.png", engine,
		WithOCRMetadata("key1", "value1"),
		WithOCRMetadata("key2", 42),
	)

	if loader.metadata["key1"] != "value1" {
		t.Errorf("expected metadata key1='value1', got %v", loader.metadata["key1"])
	}
	if loader.metadata["key2"] != 42 {
		t.Errorf("expected metadata key2=42, got %v", loader.metadata["key2"])
	}
}

func TestOCRLoader_Name(t *testing.T) {
	engine := &mockOCREngine{}
	loader := NewOCRLoader("/tmp/test.png", engine)
	if loader.Name() != "ocr_loader" {
		t.Errorf("expected name 'ocr_loader', got %q", loader.Name())
	}
}

func TestOCRLoader_Load_FileNotFound(t *testing.T) {
	engine := &mockOCREngine{}
	loader := NewOCRLoader("/nonexistent/file.png", engine)

	ctx := context.Background()
	_, err := loader.Load(ctx)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestOCRLoader_Load_UnsupportedFormat(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.xyz")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := &mockOCREngine{
		formats: []string{".png", ".jpg"},
	}
	loader := NewOCRLoader(tmpFile, engine)

	ctx := context.Background()
	_, err := loader.Load(ctx)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestOCRLoader_Load_EngineError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(tmpFile, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := &mockOCREngine{
		err: errors.New("OCR engine failed"),
	}
	loader := NewOCRLoader(tmpFile, engine)

	ctx := context.Background()
	_, err := loader.Load(ctx)
	if err == nil {
		t.Error("expected error from OCR engine")
	}
}

func TestOCRLoader_Load_SingleDocument(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(tmpFile, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := &mockOCREngine{
		result: &OCRResult{
			Text:       "Hello World",
			Language:   "eng",
			Confidence: 0.95,
			Metadata: map[string]any{
				"format": "mock_format",
			},
		},
	}
	loader := NewOCRLoader(tmpFile, engine,
		WithOCRMetadata("custom_key", "custom_value"),
	)

	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Content != "Hello World" {
		t.Errorf("expected content 'Hello World', got %q", doc.Content)
	}
	if doc.Source != tmpFile {
		t.Errorf("expected source %q, got %q", tmpFile, doc.Source)
	}
	if doc.ID == "" {
		t.Error("expected non-empty ID")
	}

	// 检查元数据
	if doc.Metadata["source"] != tmpFile {
		t.Errorf("expected source metadata, got %v", doc.Metadata["source"])
	}
	if doc.Metadata["ocr_engine"] != "mock_ocr" {
		t.Errorf("expected ocr_engine 'mock_ocr', got %v", doc.Metadata["ocr_engine"])
	}
	if doc.Metadata["confidence"] != 0.95 {
		t.Errorf("expected confidence 0.95, got %v", doc.Metadata["confidence"])
	}
	if doc.Metadata["language"] != "eng" {
		t.Errorf("expected language 'eng', got %v", doc.Metadata["language"])
	}
	if doc.Metadata["custom_key"] != "custom_value" {
		t.Errorf("expected custom metadata, got %v", doc.Metadata["custom_key"])
	}
	if doc.Metadata["ocr_engine_key"] != nil {
		// OCR result metadata 以 "ocr_" 前缀存储
	}
}

func TestOCRLoader_Load_MultiPage(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(tmpFile, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := &mockOCREngine{
		result: &OCRResult{
			Text:       "Page 1 content\nPage 2 content",
			Language:   "eng",
			Confidence: 0.9,
			Pages: []OCRPage{
				{PageNum: 1, Text: "Page 1 content", Confidence: 0.95},
				{PageNum: 2, Text: "Page 2 content", Confidence: 0.85},
			},
		},
	}
	loader := NewOCRLoader(tmpFile, engine)

	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents (one per page), got %d", len(docs))
	}

	// 检查第一页
	if docs[0].Content != "Page 1 content" {
		t.Errorf("expected page 1 content, got %q", docs[0].Content)
	}
	if docs[0].Metadata["page_num"] != 1 {
		t.Errorf("expected page_num 1, got %v", docs[0].Metadata["page_num"])
	}
	if docs[0].Metadata["page_confidence"] != 0.95 {
		t.Errorf("expected page_confidence 0.95, got %v", docs[0].Metadata["page_confidence"])
	}

	// 检查第二页
	if docs[1].Content != "Page 2 content" {
		t.Errorf("expected page 2 content, got %q", docs[1].Content)
	}
	if docs[1].Metadata["page_num"] != 2 {
		t.Errorf("expected page_num 2, got %v", docs[1].Metadata["page_num"])
	}
}

func TestOCRLoader_Load_NoLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(tmpFile, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := &mockOCREngine{
		result: &OCRResult{
			Text:       "Some text",
			Confidence: 0.8,
		},
	}
	loader := NewOCRLoader(tmpFile, engine)

	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	// 无语言时不应有 language 元数据
	if _, ok := docs[0].Metadata["language"]; ok {
		t.Error("expected no language metadata when Language is empty")
	}
}

// ============== OCRResult 测试 ==============

func TestOCRResult_Fields(t *testing.T) {
	result := &OCRResult{
		Text:       "test text",
		Language:   "eng",
		Confidence: 0.95,
		Pages: []OCRPage{
			{PageNum: 1, Text: "page 1", Confidence: 0.9},
		},
		Metadata: map[string]any{"key": "value"},
	}

	if result.Text != "test text" {
		t.Error("unexpected Text")
	}
	if result.Language != "eng" {
		t.Error("unexpected Language")
	}
	if result.Confidence != 0.95 {
		t.Error("unexpected Confidence")
	}
	if len(result.Pages) != 1 {
		t.Error("unexpected Pages count")
	}
}

// ============== OCRPage 测试 ==============

func TestOCRPage_Fields(t *testing.T) {
	page := OCRPage{
		PageNum:    3,
		Text:       "page text",
		Confidence: 0.88,
	}

	if page.PageNum != 3 {
		t.Error("unexpected PageNum")
	}
	if page.Text != "page text" {
		t.Error("unexpected Text")
	}
	if page.Confidence != 0.88 {
		t.Error("unexpected Confidence")
	}
}

// ============== OCREngine 接口兼容性测试 ==============

func TestTesseractEngine_ImplementsOCREngine(t *testing.T) {
	var _ OCREngine = (*TesseractEngine)(nil)
}

func TestVisionLLMEngine_ImplementsOCREngine(t *testing.T) {
	var _ OCREngine = (*VisionLLMEngine)(nil)
}
