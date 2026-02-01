// Package file 提供 AI Agent 的文件操作工具
//
// 本包实现了安全的文件读写工具，支持：
//   - 文件读取 (ReadFile)
//   - 文件写入 (WriteFile)
//   - 文件列表 (ListFiles)
//   - 文件删除 (DeleteFile)
//
// 安全特性：
//   - 路径白名单限制
//   - 文件大小限制
//   - 扩展名黑名单
//
// 使用示例：
//
//	tools := file.New(file.DefaultConfig())
//	readTool := tools.ReadFile()
//	writeTool := tools.WriteFile()
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/everyday-items/ai-core/schema"
	"github.com/everyday-items/ai-core/tool"
)

// Config 文件工具配置
type Config struct {
	// AllowedPaths 允许访问的路径前缀
	AllowedPaths []string

	// MaxFileSize 最大文件大小（字节）
	MaxFileSize int64

	// MaxReadSize 最大读取大小（字节）
	MaxReadSize int64

	// DeniedExtensions 禁止的文件扩展名
	DeniedExtensions []string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		AllowedPaths:     []string{"/tmp", os.TempDir()},
		MaxFileSize:      10 * 1024 * 1024, // 10MB
		MaxReadSize:      1 * 1024 * 1024,  // 1MB
		DeniedExtensions: []string{".exe", ".sh", ".bat", ".cmd", ".ps1"},
	}
}

// Tools 文件工具集
type Tools struct {
	config *Config
}

// New 创建文件工具集
func New(config *Config) *Tools {
	if config == nil {
		config = DefaultConfig()
	}
	return &Tools{config: config}
}

// All 返回所有文件工具
func (t *Tools) All() []tool.Tool {
	return []tool.Tool{
		t.ReadTool(),
		t.WriteTool(),
		t.ListTool(),
		t.ExistsTool(),
		t.DeleteTool(),
		t.InfoTool(),
	}
}

// ReadTool 返回读取文件工具
func (t *Tools) ReadTool() tool.Tool {
	return tool.NewFunc("file_read", "读取文件内容", t.read)
}

// WriteTool 返回写入文件工具
func (t *Tools) WriteTool() tool.Tool {
	return tool.NewFunc("file_write", "写入文件内容", t.write)
}

// ListTool 返回列出目录工具
func (t *Tools) ListTool() tool.Tool {
	return tool.NewFunc("file_list", "列出目录内容", t.list)
}

// ExistsTool 返回检查文件存在工具
func (t *Tools) ExistsTool() tool.Tool {
	return tool.NewFunc("file_exists", "检查文件或目录是否存在", t.exists)
}

// DeleteTool 返回删除文件工具
func (t *Tools) DeleteTool() tool.Tool {
	return tool.NewFunc("file_delete", "删除文件或目录", t.delete)
}

// InfoTool 返回获取文件信息工具
func (t *Tools) InfoTool() tool.Tool {
	return tool.NewFunc("file_info", "获取文件信息", t.info)
}

// ReadInput 读取文件输入
type ReadInput struct {
	Path     string `json:"path" desc:"文件路径" required:"true"`
	Encoding string `json:"encoding" desc:"编码格式，默认 utf-8"`
}

// ReadOutput 读取文件输出
type ReadOutput struct {
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

// read 读取文件
func (t *Tools) read(ctx context.Context, input ReadInput) (*ReadOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	info, err := os.Stat(input.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, use file_list instead")
	}

	if info.Size() > t.config.MaxReadSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: %d)", info.Size(), t.config.MaxReadSize)
	}

	content, err := os.ReadFile(input.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return &ReadOutput{
		Content: string(content),
		Size:    info.Size(),
	}, nil
}

// WriteInput 写入文件输入
type WriteInput struct {
	Path    string `json:"path" desc:"文件路径" required:"true"`
	Content string `json:"content" desc:"文件内容" required:"true"`
	Append  bool   `json:"append" desc:"是否追加模式"`
}

// WriteOutput 写入文件输出
type WriteOutput struct {
	Path    string `json:"path"`
	Written int    `json:"written"`
}

// write 写入文件
func (t *Tools) write(ctx context.Context, input WriteInput) (*WriteOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	if err := t.validateExtension(input.Path); err != nil {
		return nil, err
	}

	if int64(len(input.Content)) > t.config.MaxFileSize {
		return nil, fmt.Errorf("content too large: %d bytes (max: %d)", len(input.Content), t.config.MaxFileSize)
	}

	// 确保目录存在
	dir := filepath.Dir(input.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 设置写入标志
	flag := os.O_WRONLY | os.O_CREATE
	if input.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(input.Path, flag, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	n, err := f.WriteString(input.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &WriteOutput{
		Path:    input.Path,
		Written: n,
	}, nil
}

// ListInput 列出目录输入
type ListInput struct {
	Path      string `json:"path" desc:"目录路径" required:"true"`
	Recursive bool   `json:"recursive" desc:"是否递归列出"`
	Pattern   string `json:"pattern" desc:"匹配模式（如 *.txt）"`
}

// FileEntry 文件条目
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// ListOutput 列出目录输出
type ListOutput struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
	Count   int         `json:"count"`
}

// list 列出目录
func (t *Tools) list(ctx context.Context, input ListInput) (*ListOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	var entries []FileEntry

	if input.Recursive {
		err := filepath.Walk(input.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 跳过无法访问的文件
			}
			if input.Pattern != "" {
				matched, _ := filepath.Match(input.Pattern, info.Name())
				if !matched {
					return nil
				}
			}
			entries = append(entries, FileEntry{
				Name:    info.Name(),
				Path:    path,
				Size:    info.Size(),
				IsDir:   info.IsDir(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		dirEntries, err := os.ReadDir(input.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range dirEntries {
			if input.Pattern != "" {
				matched, _ := filepath.Match(input.Pattern, entry.Name())
				if !matched {
					continue
				}
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			entries = append(entries, FileEntry{
				Name:    entry.Name(),
				Path:    filepath.Join(input.Path, entry.Name()),
				Size:    info.Size(),
				IsDir:   entry.IsDir(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}

	return &ListOutput{
		Path:    input.Path,
		Entries: entries,
		Count:   len(entries),
	}, nil
}

// ExistsInput 检查存在输入
type ExistsInput struct {
	Path string `json:"path" desc:"文件或目录路径" required:"true"`
}

// ExistsOutput 检查存在输出
type ExistsOutput struct {
	Exists bool `json:"exists"`
	IsDir  bool `json:"is_dir"`
}

// exists 检查文件是否存在
func (t *Tools) exists(ctx context.Context, input ExistsInput) (*ExistsOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	info, err := os.Stat(input.Path)
	if os.IsNotExist(err) {
		return &ExistsOutput{Exists: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	return &ExistsOutput{
		Exists: true,
		IsDir:  info.IsDir(),
	}, nil
}

// DeleteInput 删除输入
type DeleteInput struct {
	Path      string `json:"path" desc:"文件或目录路径" required:"true"`
	Recursive bool   `json:"recursive" desc:"是否递归删除目录"`
}

// DeleteOutput 删除输出
type DeleteOutput struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

// delete 删除文件或目录
func (t *Tools) delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	info, err := os.Stat(input.Path)
	if os.IsNotExist(err) {
		return &DeleteOutput{Path: input.Path, Deleted: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() && !input.Recursive {
		return nil, fmt.Errorf("path is a directory, set recursive=true to delete")
	}

	if input.Recursive {
		err = os.RemoveAll(input.Path)
	} else {
		err = os.Remove(input.Path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to delete: %w", err)
	}

	return &DeleteOutput{
		Path:    input.Path,
		Deleted: true,
	}, nil
}

// InfoInput 获取文件信息输入
type InfoInput struct {
	Path string `json:"path" desc:"文件路径" required:"true"`
}

// InfoOutput 获取文件信息输出
type InfoOutput struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// info 获取文件信息
func (t *Tools) info(ctx context.Context, input InfoInput) (*InfoOutput, error) {
	if err := t.validatePath(input.Path); err != nil {
		return nil, err
	}

	info, err := os.Stat(input.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	return &InfoOutput{
		Name:    info.Name(),
		Path:    input.Path,
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
	}, nil
}

// validatePath 验证路径是否允许访问
func (t *Tools) validatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 检查是否在允许的路径下
	if len(t.config.AllowedPaths) > 0 {
		allowed := false
		for _, allowedPath := range t.config.AllowedPaths {
			absAllowed, err := filepath.Abs(allowedPath)
			if err != nil {
				continue
			}
			if strings.HasPrefix(absPath, absAllowed) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("path not allowed: %s", path)
		}
	}

	return nil
}

// validateExtension 验证文件扩展名
func (t *Tools) validateExtension(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	for _, denied := range t.config.DeniedExtensions {
		if ext == denied {
			return fmt.Errorf("file extension not allowed: %s", ext)
		}
	}
	return nil
}

// ============== 独立工具函数 ==============

// ReadFile 读取文件的便捷函数
func ReadFile(path string) tool.Tool {
	return New(nil).ReadTool()
}

// WriteFile 写入文件的便捷函数
func WriteFile(path, content string) tool.Tool {
	return New(nil).WriteTool()
}

// ListDir 列出目录的便捷函数
func ListDir(path string) tool.Tool {
	return New(nil).ListTool()
}

// 为工具定义 Schema
var _ = schema.Of[ReadInput]()
var _ = schema.Of[WriteInput]()
var _ = schema.Of[ListInput]()
var _ = schema.Of[ExistsInput]()
var _ = schema.Of[DeleteInput]()
var _ = schema.Of[InfoInput]()
