package multimodal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============== ContentType 测试 ==============

func TestContentType_Constants(t *testing.T) {
	if ContentTypeText != "text" {
		t.Errorf("期望 ContentTypeText=text，实际 %s", ContentTypeText)
	}
	if ContentTypeImage != "image" {
		t.Errorf("期望 ContentTypeImage=image，实际 %s", ContentTypeImage)
	}
	if ContentTypeAudio != "audio" {
		t.Errorf("期望 ContentTypeAudio=audio，实际 %s", ContentTypeAudio)
	}
	if ContentTypeVideo != "video" {
		t.Errorf("期望 ContentTypeVideo=video，实际 %s", ContentTypeVideo)
	}
}

// ============== Content 类型判断测试 ==============

func TestContent_IsText(t *testing.T) {
	c := &Content{Type: ContentTypeText, Text: "hello"}
	if !c.IsText() {
		t.Error("期望 IsText() 返回 true")
	}
	if c.IsImage() || c.IsAudio() || c.IsVideo() {
		t.Error("文本内容不应匹配其他类型")
	}
}

func TestContent_IsImage(t *testing.T) {
	c := &Content{Type: ContentTypeImage, Data: []byte{0x89, 0x50}}
	if !c.IsImage() {
		t.Error("期望 IsImage() 返回 true")
	}
	if c.IsText() || c.IsAudio() || c.IsVideo() {
		t.Error("图像内容不应匹配其他类型")
	}
}

func TestContent_IsAudio(t *testing.T) {
	c := &Content{Type: ContentTypeAudio}
	if !c.IsAudio() {
		t.Error("期望 IsAudio() 返回 true")
	}
}

func TestContent_IsVideo(t *testing.T) {
	c := &Content{Type: ContentTypeVideo}
	if !c.IsVideo() {
		t.Error("期望 IsVideo() 返回 true")
	}
}

// ============== Content 创建函数测试 ==============

func TestNewTextContent(t *testing.T) {
	c := NewTextContent("这是文本")
	if c.Type != ContentTypeText {
		t.Errorf("期望类型为 text，实际 %s", c.Type)
	}
	if c.Text != "这是文本" {
		t.Errorf("期望文本为 '这是文本'，实际 '%s'", c.Text)
	}
}

func TestNewImageContent(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG 魔数
	c := NewImageContent(data, ImageFormatPNG)

	if c.Type != ContentTypeImage {
		t.Errorf("期望类型为 image，实际 %s", c.Type)
	}
	if !bytes.Equal(c.Data, data) {
		t.Error("数据不匹配")
	}
	if c.Format != string(ImageFormatPNG) {
		t.Errorf("期望格式为 png，实际 %s", c.Format)
	}
}

func TestNewImageContentFromURL(t *testing.T) {
	c := NewImageContentFromURL("https://example.com/image.png")

	if c.Type != ContentTypeImage {
		t.Errorf("期望类型为 image，实际 %s", c.Type)
	}
	if c.URL != "https://example.com/image.png" {
		t.Errorf("URL 不匹配: %s", c.URL)
	}
}

func TestNewAudioContent(t *testing.T) {
	data := []byte{0xFF, 0xFB} // MP3 帧头
	c := NewAudioContent(data, AudioFormatMP3)

	if c.Type != ContentTypeAudio {
		t.Errorf("期望类型为 audio，实际 %s", c.Type)
	}
	if c.Format != string(AudioFormatMP3) {
		t.Errorf("期望格式为 mp3，实际 %s", c.Format)
	}
}

func TestNewVideoContent(t *testing.T) {
	data := []byte{0x00, 0x00}
	c := NewVideoContent(data, VideoFormatMP4)

	if c.Type != ContentTypeVideo {
		t.Errorf("期望类型为 video，实际 %s", c.Type)
	}
	if c.Format != string(VideoFormatMP4) {
		t.Errorf("期望格式为 mp4，实际 %s", c.Format)
	}
}

// ============== Content ToBase64 和 MIME 类型测试 ==============

func TestContent_ToBase64_WithData(t *testing.T) {
	data := []byte("test data")
	c := NewImageContent(data, ImageFormatPNG)

	b64 := c.ToBase64()
	if b64 == "" {
		t.Error("期望非空 base64 字符串")
	}
	if !strings.HasPrefix(b64, "data:image/png;base64,") {
		t.Errorf("期望以 data:image/png;base64, 开头，实际 %s", b64[:30])
	}
}

func TestContent_ToBase64_WithDataURL(t *testing.T) {
	c := &Content{
		Type:    ContentTypeImage,
		DataURL: "data:image/png;base64,abc123",
	}

	b64 := c.ToBase64()
	if b64 != "data:image/png;base64,abc123" {
		t.Errorf("应直接返回 DataURL，实际 %s", b64)
	}
}

func TestContent_ToBase64_Empty(t *testing.T) {
	c := &Content{Type: ContentTypeImage}
	b64 := c.ToBase64()
	if b64 != "" {
		t.Errorf("无数据时应返回空字符串，实际 %s", b64)
	}
}

func TestContent_getMimeType(t *testing.T) {
	tests := []struct {
		contentType ContentType
		format      string
		expected    string
	}{
		{ContentTypeImage, string(ImageFormatPNG), "image/png"},
		{ContentTypeImage, string(ImageFormatJPEG), "image/jpeg"},
		{ContentTypeImage, string(ImageFormatGIF), "image/gif"},
		{ContentTypeImage, string(ImageFormatWebP), "image/webp"},
		{ContentTypeImage, "unknown", "image/png"}, // 默认 PNG
		{ContentTypeAudio, string(AudioFormatMP3), "audio/mpeg"},
		{ContentTypeAudio, string(AudioFormatWAV), "audio/wav"},
		{ContentTypeAudio, string(AudioFormatOGG), "audio/ogg"},
		{ContentTypeAudio, "unknown", "audio/mpeg"}, // 默认 MP3
		{ContentTypeVideo, string(VideoFormatMP4), "video/mp4"},
		{ContentTypeVideo, string(VideoFormatWebM), "video/webm"},
		{ContentTypeVideo, "unknown", "video/mp4"}, // 默认 MP4
		{ContentTypeText, "", "text/plain"},
	}

	for _, tt := range tests {
		c := &Content{Type: tt.contentType, Format: tt.format}
		mime := c.getMimeType()
		if mime != tt.expected {
			t.Errorf("getMimeType(type=%s, format=%s) = %s，期望 %s",
				tt.contentType, tt.format, mime, tt.expected)
		}
	}
}

// ============== MultimodalDocument 测试 ==============

func TestNewMultimodalDocument(t *testing.T) {
	text := NewTextContent("hello")
	img := NewImageContent([]byte{1, 2, 3}, ImageFormatPNG)

	doc := NewMultimodalDocument(text, img)

	if doc.ID == "" {
		t.Error("期望自动生成 ID")
	}
	if len(doc.Contents) != 2 {
		t.Errorf("期望 2 个内容，实际 %d", len(doc.Contents))
	}
	if doc.CreatedAt.IsZero() {
		t.Error("期望自动设置 CreatedAt")
	}
}

func TestMultimodalDocument_AddContent(t *testing.T) {
	doc := NewMultimodalDocument()
	doc.AddContent(NewTextContent("文本1"))
	doc.AddContent(NewImageContent([]byte{1}, ImageFormatJPEG))

	if len(doc.Contents) != 2 {
		t.Errorf("期望 2 个内容，实际 %d", len(doc.Contents))
	}
}

func TestMultimodalDocument_GetText(t *testing.T) {
	doc := NewMultimodalDocument(
		NewTextContent("第一段"),
		NewImageContent([]byte{1}, ImageFormatPNG),
		NewTextContent("第二段"),
	)
	doc.TextDescription = "描述"

	text := doc.GetText()
	if !strings.Contains(text, "第一段") {
		t.Error("期望包含 '第一段'")
	}
	if !strings.Contains(text, "第二段") {
		t.Error("期望包含 '第二段'")
	}
	if !strings.Contains(text, "描述") {
		t.Error("期望包含 TextDescription")
	}
}

func TestMultimodalDocument_GetImages(t *testing.T) {
	doc := NewMultimodalDocument(
		NewTextContent("text"),
		NewImageContent([]byte{1}, ImageFormatPNG),
		NewImageContent([]byte{2}, ImageFormatJPEG),
		NewAudioContent([]byte{3}, AudioFormatMP3),
	)

	images := doc.GetImages()
	if len(images) != 2 {
		t.Errorf("期望 2 张图片，实际 %d", len(images))
	}
}

func TestMultimodalDocument_HasImages(t *testing.T) {
	doc1 := NewMultimodalDocument(NewTextContent("text"))
	if doc1.HasImages() {
		t.Error("纯文本文档不应有图片")
	}

	doc2 := NewMultimodalDocument(NewImageContent([]byte{1}, ImageFormatPNG))
	if !doc2.HasImages() {
		t.Error("包含图片的文档应返回 true")
	}
}

func TestMultimodalDocument_HasAudio(t *testing.T) {
	doc1 := NewMultimodalDocument(NewTextContent("text"))
	if doc1.HasAudio() {
		t.Error("纯文本文档不应有音频")
	}

	doc2 := NewMultimodalDocument(NewAudioContent([]byte{1}, AudioFormatMP3))
	if !doc2.HasAudio() {
		t.Error("包含音频的文档应返回 true")
	}
}

func TestMultimodalDocument_HasVideo(t *testing.T) {
	doc1 := NewMultimodalDocument(NewTextContent("text"))
	if doc1.HasVideo() {
		t.Error("纯文本文档不应有视频")
	}

	doc2 := NewMultimodalDocument(NewVideoContent([]byte{1}, VideoFormatMP4))
	if !doc2.HasVideo() {
		t.Error("包含视频的文档应返回 true")
	}
}

func TestMultimodalDocument_ToRAGDocument(t *testing.T) {
	doc := NewMultimodalDocument(NewTextContent("内容"))
	doc.Metadata = map[string]any{"key": "value"}

	ragDoc := doc.ToRAGDocument()
	if ragDoc.ID != doc.ID {
		t.Errorf("ID 不匹配: %s vs %s", ragDoc.ID, doc.ID)
	}
	if ragDoc.Content != "内容" {
		t.Errorf("内容不匹配: %s", ragDoc.Content)
	}
	if ragDoc.Metadata["key"] != "value" {
		t.Error("元数据不匹配")
	}
}

// ============== Processor 接口测试 ==============

func TestImageProcessor_SupportedTypes(t *testing.T) {
	// 传 nil provider，仅测试 SupportedTypes
	p := NewImageProcessor(nil)
	types := p.SupportedTypes()

	if len(types) != 1 || types[0] != ContentTypeImage {
		t.Errorf("期望支持 [image]，实际 %v", types)
	}
}

func TestImageProcessor_ProcessNonImage(t *testing.T) {
	p := NewImageProcessor(nil)
	ctx := context.Background()

	_, err := p.Process(ctx, NewTextContent("not an image"))
	if err == nil {
		t.Error("处理非图像内容应返回错误")
	}
}

func TestAudioProcessor_SupportedTypes(t *testing.T) {
	p := NewAudioProcessor(nil)
	types := p.SupportedTypes()

	if len(types) != 1 || types[0] != ContentTypeAudio {
		t.Errorf("期望支持 [audio]，实际 %v", types)
	}
}

func TestAudioProcessor_ProcessNonAudio(t *testing.T) {
	p := NewAudioProcessor(nil)
	ctx := context.Background()

	_, err := p.Process(ctx, NewTextContent("not audio"))
	if err == nil {
		t.Error("处理非音频内容应返回错误")
	}
}

func TestAudioProcessor_Process_NoTranscriber(t *testing.T) {
	p := NewAudioProcessor(nil) // 无转录器
	ctx := context.Background()

	result, err := p.Process(ctx, NewAudioContent([]byte{1, 2}, AudioFormatMP3))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if result.TextDescription != "" {
		t.Error("无转录器时不应有文本描述")
	}
	if result.Metadata["content_type"] != "audio" {
		t.Error("元数据 content_type 应为 audio")
	}
	if result.Metadata["format"] != string(AudioFormatMP3) {
		t.Error("元数据 format 应为 mp3")
	}
}

func TestVideoProcessor_SupportedTypes(t *testing.T) {
	p := NewVideoProcessor(nil, nil, nil)
	types := p.SupportedTypes()

	if len(types) != 1 || types[0] != ContentTypeVideo {
		t.Errorf("期望支持 [video]，实际 %v", types)
	}
}

func TestVideoProcessor_ProcessNonVideo(t *testing.T) {
	p := NewVideoProcessor(nil, nil, nil)
	ctx := context.Background()

	_, err := p.Process(ctx, NewTextContent("not video"))
	if err == nil {
		t.Error("处理非视频内容应返回错误")
	}
}

func TestVideoProcessor_Process_NoExtractor(t *testing.T) {
	p := NewVideoProcessor(nil, nil, nil) // 无帧提取器
	ctx := context.Background()

	result, err := p.Process(ctx, NewVideoContent([]byte{0, 0}, VideoFormatMP4))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if result.Metadata["content_type"] != "video" {
		t.Error("元数据 content_type 应为 video")
	}
}

// ============== ImageProcessor 选项测试 ==============

func TestImageProcessor_Options(t *testing.T) {
	p := NewImageProcessor(nil,
		WithImageModel("gpt-4-vision"),
		WithImagePrompt("描述这张图"),
	)

	if p.model != "gpt-4-vision" {
		t.Errorf("期望 model=gpt-4-vision，实际 %s", p.model)
	}
	if p.defaultPrompt != "描述这张图" {
		t.Errorf("期望自定义 prompt，实际 %s", p.defaultPrompt)
	}
}

// ============== CLIPEmbedder 测试 ==============

func TestCLIPEmbedder_NotImplemented(t *testing.T) {
	clip := NewCLIPEmbedder("http://localhost:8080", "key")

	if clip.IsImplemented() {
		t.Error("CLIP 当前未实现，应返回 false")
	}

	ctx := context.Background()

	_, err := clip.EmbedText(ctx, []string{"test"})
	if err != ErrCLIPNotImplemented {
		t.Errorf("期望 ErrCLIPNotImplemented，实际 %v", err)
	}

	_, err = clip.EmbedImage(ctx, NewImageContent([]byte{1}, ImageFormatPNG))
	if err != ErrCLIPNotImplemented {
		t.Errorf("期望 ErrCLIPNotImplemented，实际 %v", err)
	}

	_, err = clip.EmbedImages(ctx, []*Content{NewImageContent([]byte{1}, ImageFormatPNG)})
	if err != ErrCLIPNotImplemented {
		t.Errorf("期望 ErrCLIPNotImplemented，实际 %v", err)
	}
}

// ============== MultimodalLoader 测试 ==============

func TestMultimodalLoader_New(t *testing.T) {
	loader := NewMultimodalLoader()
	if loader == nil {
		t.Fatal("NewMultimodalLoader 返回 nil")
	}
	if len(loader.supportedExtensions) == 0 {
		t.Error("期望有支持的扩展名")
	}
}

func TestMultimodalLoader_LoadFile_Text(t *testing.T) {
	// 创建临时文本文件
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	loader := NewMultimodalLoader()
	ctx := context.Background()

	doc, err := loader.LoadFile(ctx, path)
	if err != nil {
		t.Fatalf("LoadFile 失败: %v", err)
	}

	if len(doc.Contents) != 1 {
		t.Fatalf("期望 1 个内容，实际 %d", len(doc.Contents))
	}
	if !doc.Contents[0].IsText() {
		t.Error("期望内容类型为文本")
	}
	if doc.Contents[0].Text != "hello world" {
		t.Errorf("内容不匹配: %s", doc.Contents[0].Text)
	}
	if doc.Metadata["source"] != path {
		t.Errorf("source 元数据不匹配: %v", doc.Metadata["source"])
	}
}

func TestMultimodalLoader_LoadFile_Image(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	// 写入 PNG 魔数作为数据
	if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4E, 0x47}, 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	loader := NewMultimodalLoader()
	ctx := context.Background()

	doc, err := loader.LoadFile(ctx, path)
	if err != nil {
		t.Fatalf("LoadFile 失败: %v", err)
	}

	if !doc.Contents[0].IsImage() {
		t.Error("期望内容类型为图像")
	}
	if doc.Contents[0].Format != string(ImageFormatPNG) {
		t.Errorf("期望格式为 png，实际 %s", doc.Contents[0].Format)
	}
}

func TestMultimodalLoader_LoadFile_UnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xyz")
	_ = os.WriteFile(path, []byte("data"), 0644)

	loader := NewMultimodalLoader()
	ctx := context.Background()

	_, err := loader.LoadFile(ctx, path)
	if err == nil {
		t.Error("不支持的扩展名应返回错误")
	}
}

func TestMultimodalLoader_LoadFile_NotExists(t *testing.T) {
	loader := NewMultimodalLoader()
	ctx := context.Background()

	_, err := loader.LoadFile(ctx, "/nonexistent/file.txt")
	if err == nil {
		t.Error("不存在的文件应返回错误")
	}
}

func TestMultimodalLoader_LoadDirectory(t *testing.T) {
	dir := t.TempDir()

	// 创建多种类型的文件
	_ = os.WriteFile(filepath.Join(dir, "doc.txt"), []byte("text"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "img.png"), []byte{0x89, 0x50}, 0644)
	_ = os.WriteFile(filepath.Join(dir, "skip.xyz"), []byte("skip"), 0644) // 不支持的格式

	loader := NewMultimodalLoader()
	ctx := context.Background()

	docs, err := loader.LoadDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("LoadDirectory 失败: %v", err)
	}

	// 应加载 2 个文件（跳过 .xyz）
	if len(docs) != 2 {
		t.Errorf("期望 2 个文档，实际 %d", len(docs))
	}
}

func TestMultimodalLoader_LoadFromReader(t *testing.T) {
	loader := NewMultimodalLoader()
	ctx := context.Background()

	reader := strings.NewReader("从 reader 读取的内容")
	doc, err := loader.LoadFromReader(ctx, reader, ContentTypeText, "")
	if err != nil {
		t.Fatalf("LoadFromReader 失败: %v", err)
	}

	if len(doc.Contents) != 1 {
		t.Fatalf("期望 1 个内容，实际 %d", len(doc.Contents))
	}
	if doc.Contents[0].Text != "从 reader 读取的内容" {
		t.Errorf("内容不匹配: %s", doc.Contents[0].Text)
	}
}

func TestMultimodalLoader_LoadFromReader_Image(t *testing.T) {
	loader := NewMultimodalLoader()
	ctx := context.Background()

	reader := bytes.NewReader([]byte{0x89, 0x50})
	doc, err := loader.LoadFromReader(ctx, reader, ContentTypeImage, string(ImageFormatPNG))
	if err != nil {
		t.Fatalf("LoadFromReader 失败: %v", err)
	}

	if !doc.Contents[0].IsImage() {
		t.Error("期望图像类型")
	}
}

func TestMultimodalLoader_LoadFromReader_UnsupportedType(t *testing.T) {
	loader := NewMultimodalLoader()
	ctx := context.Background()

	reader := strings.NewReader("data")
	_, err := loader.LoadFromReader(ctx, reader, ContentType("unknown"), "")
	if err == nil {
		t.Error("不支持的内容类型应返回错误")
	}
}

// ============== MultimodalLoader 各种图像格式测试 ==============

func TestMultimodalLoader_LoadFile_JPEG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	_ = os.WriteFile(path, []byte{0xFF, 0xD8}, 0644)

	loader := NewMultimodalLoader()
	doc, err := loader.LoadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadFile 失败: %v", err)
	}
	if doc.Contents[0].Format != string(ImageFormatJPEG) {
		t.Errorf("期望 JPEG 格式，实际 %s", doc.Contents[0].Format)
	}
}

func TestMultimodalLoader_LoadFile_Audio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sound.mp3")
	_ = os.WriteFile(path, []byte{0xFF, 0xFB}, 0644)

	loader := NewMultimodalLoader()
	doc, err := loader.LoadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadFile 失败: %v", err)
	}
	if !doc.Contents[0].IsAudio() {
		t.Error("期望音频类型")
	}
	if doc.Contents[0].Format != string(AudioFormatMP3) {
		t.Errorf("期望 MP3 格式，实际 %s", doc.Contents[0].Format)
	}
}

func TestMultimodalLoader_LoadFile_Video(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	_ = os.WriteFile(path, []byte{0x00, 0x00}, 0644)

	loader := NewMultimodalLoader()
	doc, err := loader.LoadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadFile 失败: %v", err)
	}
	if !doc.Contents[0].IsVideo() {
		t.Error("期望视频类型")
	}
	if doc.Contents[0].Format != string(VideoFormatMP4) {
		t.Errorf("期望 MP4 格式，实际 %s", doc.Contents[0].Format)
	}
}

// ============== MultimodalIndexer 接口符合性测试 ==============

func TestMultimodalIndexer_ImplementsIndexer(t *testing.T) {
	// 编译时检查已在 multimodal.go 中通过 var _ rag.Indexer = ... 完成
	// 此处仅确认 NewMultimodalIndexer 不会 panic
	idx := NewMultimodalIndexer(nil, nil)
	if idx == nil {
		t.Fatal("NewMultimodalIndexer 返回 nil")
	}
	if idx.batchSize != 10 {
		t.Errorf("默认 batchSize 应为 10，实际 %d", idx.batchSize)
	}
}

func TestMultimodalIndexer_WithBatchSize(t *testing.T) {
	idx := NewMultimodalIndexer(nil, nil, WithMultimodalBatchSize(50))
	if idx.batchSize != 50 {
		t.Errorf("期望 batchSize=50，实际 %d", idx.batchSize)
	}
}

// ============== MultimodalRetriever 选项测试 ==============

func TestMultimodalRetriever_New(t *testing.T) {
	r := NewMultimodalRetriever(nil, nil)
	if r == nil {
		t.Fatal("NewMultimodalRetriever 返回 nil")
	}
	if r.topK != 10 {
		t.Errorf("默认 topK 应为 10，实际 %d", r.topK)
	}
}

func TestMultimodalRetriever_WithOptions(t *testing.T) {
	r := NewMultimodalRetriever(nil, nil,
		WithMultimodalTopK(20),
	)
	if r.topK != 20 {
		t.Errorf("期望 topK=20，实际 %d", r.topK)
	}
}
