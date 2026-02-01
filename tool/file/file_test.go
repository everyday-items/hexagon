package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if len(cfg.AllowedPaths) == 0 {
		t.Error("expected default allowed paths")
	}

	if cfg.MaxFileSize <= 0 {
		t.Error("expected positive max file size")
	}

	if len(cfg.DeniedExtensions) == 0 {
		t.Error("expected denied extensions")
	}
}

func TestNewTools(t *testing.T) {
	tools := New(nil)

	if tools == nil {
		t.Fatal("expected non-nil tools")
	}

	all := tools.All()
	if len(all) != 6 {
		t.Errorf("expected 6 tools, got %d", len(all))
	}
}

func TestNewToolsWithConfig(t *testing.T) {
	cfg := &Config{
		AllowedPaths:     []string{"/custom"},
		MaxFileSize:      1024,
		MaxReadSize:      512,
		DeniedExtensions: []string{".exe"},
	}

	tools := New(cfg)

	if tools.config.MaxFileSize != 1024 {
		t.Errorf("expected MaxFileSize 1024, got %d", tools.config.MaxFileSize)
	}
}

func TestReadTool(t *testing.T) {
	tools := New(nil)
	tool := tools.ReadTool()

	if tool.Name() != "file_read" {
		t.Errorf("expected name 'file_read', got '%s'", tool.Name())
	}

	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestWriteTool(t *testing.T) {
	tools := New(nil)
	tool := tools.WriteTool()

	if tool.Name() != "file_write" {
		t.Errorf("expected name 'file_write', got '%s'", tool.Name())
	}
}

func TestListTool(t *testing.T) {
	tools := New(nil)
	tool := tools.ListTool()

	if tool.Name() != "file_list" {
		t.Errorf("expected name 'file_list', got '%s'", tool.Name())
	}
}

func TestExistsTool(t *testing.T) {
	tools := New(nil)
	tool := tools.ExistsTool()

	if tool.Name() != "file_exists" {
		t.Errorf("expected name 'file_exists', got '%s'", tool.Name())
	}
}

func TestDeleteTool(t *testing.T) {
	tools := New(nil)
	tool := tools.DeleteTool()

	if tool.Name() != "file_delete" {
		t.Errorf("expected name 'file_delete', got '%s'", tool.Name())
	}
}

func TestInfoTool(t *testing.T) {
	tools := New(nil)
	tool := tools.InfoTool()

	if tool.Name() != "file_info" {
		t.Errorf("expected name 'file_info', got '%s'", tool.Name())
	}
}

func TestReadWriteFile(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
		MaxFileSize:  1024 * 1024,
		MaxReadSize:  1024 * 1024,
	}
	tools := New(cfg)

	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"

	// 测试写入
	writeOutput, err := tools.write(context.Background(), WriteInput{
		Path:    testFile,
		Content: testContent,
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if writeOutput.Written != len(testContent) {
		t.Errorf("expected written %d, got %d", len(testContent), writeOutput.Written)
	}

	// 测试读取
	readOutput, err := tools.read(context.Background(), ReadInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if readOutput.Content != testContent {
		t.Errorf("expected content '%s', got '%s'", testContent, readOutput.Content)
	}
}

func TestWriteAppend(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
		MaxFileSize:  1024 * 1024,
		MaxReadSize:  1024 * 1024,
	}
	tools := New(cfg)

	testFile := filepath.Join(tmpDir, "append.txt")

	// 第一次写入
	_, err := tools.write(context.Background(), WriteInput{
		Path:    testFile,
		Content: "Hello",
	})
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// 追加写入
	_, err = tools.write(context.Background(), WriteInput{
		Path:    testFile,
		Content: " World",
		Append:  true,
	})
	if err != nil {
		t.Fatalf("append write failed: %v", err)
	}

	// 读取验证
	readOutput, err := tools.read(context.Background(), ReadInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if readOutput.Content != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", readOutput.Content)
	}
}

func TestListDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 测试列出目录
	listOutput, err := tools.list(context.Background(), ListInput{
		Path: tmpDir,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if listOutput.Count != 3 {
		t.Errorf("expected 3 entries, got %d", listOutput.Count)
	}
}

func TestListDirectoryWithPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "image.png"), []byte("image"), 0644)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 只列出 .txt 文件
	listOutput, err := tools.list(context.Background(), ListInput{
		Path:    tmpDir,
		Pattern: "*.txt",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if listOutput.Count != 2 {
		t.Errorf("expected 2 .txt files, got %d", listOutput.Count)
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "exists.txt")

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 文件不存在
	existsOutput, err := tools.exists(context.Background(), ExistsInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("exists failed: %v", err)
	}

	if existsOutput.Exists {
		t.Error("expected file not to exist")
	}

	// 创建文件
	os.WriteFile(testFile, []byte("content"), 0644)

	// 文件存在
	existsOutput, err = tools.exists(context.Background(), ExistsInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("exists failed: %v", err)
	}

	if !existsOutput.Exists {
		t.Error("expected file to exist")
	}

	if existsOutput.IsDir {
		t.Error("expected not to be a directory")
	}
}

func TestDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "delete.txt")

	// 创建文件
	os.WriteFile(testFile, []byte("content"), 0644)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 删除文件
	deleteOutput, err := tools.delete(context.Background(), DeleteInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if !deleteOutput.Deleted {
		t.Error("expected file to be deleted")
	}

	// 验证文件不存在
	_, err = os.Stat(testFile)
	if !os.IsNotExist(err) {
		t.Error("expected file to not exist after deletion")
	}
}

func TestDeleteNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 删除不存在的文件
	deleteOutput, err := tools.delete(context.Background(), DeleteInput{
		Path: filepath.Join(tmpDir, "nonexistent.txt"),
	})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if deleteOutput.Deleted {
		t.Error("expected Deleted to be false for non-existent file")
	}
}

func TestFileInfo(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "info.txt")
	content := "test content"

	os.WriteFile(testFile, []byte(content), 0644)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	infoOutput, err := tools.info(context.Background(), InfoInput{
		Path: testFile,
	})
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}

	if infoOutput.Name != "info.txt" {
		t.Errorf("expected name 'info.txt', got '%s'", infoOutput.Name)
	}

	if infoOutput.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), infoOutput.Size)
	}

	if infoOutput.IsDir {
		t.Error("expected not to be a directory")
	}
}

func TestPathValidation(t *testing.T) {
	cfg := &Config{
		AllowedPaths: []string{"/tmp"},
	}
	tools := New(cfg)

	// 尝试读取不允许的路径
	_, err := tools.read(context.Background(), ReadInput{
		Path: "/etc/passwd",
	})
	if err == nil {
		t.Error("expected error for disallowed path")
	}
}

func TestExtensionValidation(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths:     []string{tmpDir},
		DeniedExtensions: []string{".exe"},
	}
	tools := New(cfg)

	// 尝试写入禁止的扩展名
	_, err := tools.write(context.Background(), WriteInput{
		Path:    filepath.Join(tmpDir, "test.exe"),
		Content: "content",
	})
	if err == nil {
		t.Error("expected error for denied extension")
	}
}

func TestFileSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
		MaxFileSize:  10, // 只允许 10 字节
	}
	tools := New(cfg)

	// 尝试写入过大的内容
	_, err := tools.write(context.Background(), WriteInput{
		Path:    filepath.Join(tmpDir, "large.txt"),
		Content: "This content is too large",
	})
	if err == nil {
		t.Error("expected error for oversized content")
	}
}

func TestReadSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	// 创建一个大文件
	largeContent := make([]byte, 1000)
	os.WriteFile(testFile, largeContent, 0644)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
		MaxReadSize:  100, // 只允许读取 100 字节
	}
	tools := New(cfg)

	// 尝试读取过大的文件
	_, err := tools.read(context.Background(), ReadInput{
		Path: testFile,
	})
	if err == nil {
		t.Error("expected error for oversized file")
	}
}

func TestReadDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 尝试读取目录
	_, err := tools.read(context.Background(), ReadInput{
		Path: tmpDir,
	})
	if err == nil {
		t.Error("expected error when reading directory")
	}
}

func TestDeleteDirectoryWithoutRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 尝试删除目录而不指定 recursive
	_, err := tools.delete(context.Background(), DeleteInput{
		Path: subDir,
	})
	if err == nil {
		t.Error("expected error when deleting directory without recursive")
	}
}

func TestDeleteDirectoryRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0644)

	cfg := &Config{
		AllowedPaths: []string{tmpDir},
	}
	tools := New(cfg)

	// 递归删除目录
	deleteOutput, err := tools.delete(context.Background(), DeleteInput{
		Path:      subDir,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("recursive delete failed: %v", err)
	}

	if !deleteOutput.Deleted {
		t.Error("expected directory to be deleted")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	// 测试便捷函数返回工具
	readTool := ReadFile("test")
	if readTool == nil {
		t.Error("expected non-nil read tool")
	}

	writeTool := WriteFile("test", "content")
	if writeTool == nil {
		t.Error("expected non-nil write tool")
	}

	listTool := ListDir("test")
	if listTool == nil {
		t.Error("expected non-nil list tool")
	}
}

func TestFileEntry(t *testing.T) {
	entry := FileEntry{
		Name:    "test.txt",
		Path:    "/tmp/test.txt",
		Size:    100,
		IsDir:   false,
		ModTime: "2024-01-01 12:00:00",
	}

	if entry.Name != "test.txt" {
		t.Errorf("unexpected name: %s", entry.Name)
	}

	if entry.IsDir {
		t.Error("expected not to be a directory")
	}
}
