package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 确保 FileCheckpointSaver 实现了 CheckpointSaver 接口
var _ CheckpointSaver = (*FileCheckpointSaver)(nil)

// FileCheckpointSaver 基于文件系统的检查点保存器
//
// 将检查点以 JSON 文件形式存储在文件系统中。
// 目录结构：
//
//	baseDir/threads/{threadID}/          - 线程目录
//	baseDir/threads/{threadID}/index.json - 索引文件，记录检查点 ID 列表
//	baseDir/threads/{threadID}/{id}.json  - 检查点 JSON 文件
//
// 线程安全：使用 sync.RWMutex 保护所有读写操作
type FileCheckpointSaver struct {
	// baseDir 基础目录路径
	baseDir string
	// mu 读写锁，保证并发安全
	mu sync.RWMutex
}

// threadIndex 线程索引
// 记录一个线程下所有检查点 ID 的有序列表
type threadIndex struct {
	// CheckpointIDs 检查点 ID 列表，按保存顺序排列
	CheckpointIDs []string `json:"checkpoint_ids"`
}

// NewFileCheckpointSaver 创建基于文件系统的检查点保存器
//
// 参数：
//   - baseDir: 基础目录路径，如果不存在会自动创建
//
// 返回：
//   - *FileCheckpointSaver: 文件检查点保存器实例
//   - error: 创建目录失败时返回错误
func NewFileCheckpointSaver(baseDir string) (*FileCheckpointSaver, error) {
	// 创建基础目录（包含 threads 子目录）
	threadsDir := filepath.Join(baseDir, "threads")
	if err := os.MkdirAll(threadsDir, 0755); err != nil {
		return nil, fmt.Errorf("创建检查点基础目录失败: %w", err)
	}

	return &FileCheckpointSaver{
		baseDir: baseDir,
	}, nil
}

// Save 保存检查点到文件系统
//
// 如果检查点 ID 为空，会自动生成。
// 自动设置 UpdatedAt 时间戳，如果 CreatedAt 为零值则同时设置。
// 检查点数据以 JSON 格式写入文件，同时更新索引文件。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - checkpoint: 要保存的检查点
//
// 返回：
//   - error: 保存失败时返回错误
func (s *FileCheckpointSaver) Save(ctx context.Context, checkpoint *Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if checkpoint.ThreadID == "" {
		return fmt.Errorf("thread_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 自动生成 ID
	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}

	// 更新已存在检查点时，如果未显式设置 CreatedAt，沿用原值
	cpPath := s.checkpointPath(checkpoint.ThreadID, checkpoint.ID)
	if checkpoint.CreatedAt.IsZero() {
		if _, err := os.Stat(cpPath); err == nil {
			existing, err := s.readCheckpoint(checkpoint.ThreadID, checkpoint.ID)
			if err != nil {
				return fmt.Errorf("读取已有检查点失败: %w", err)
			}
			checkpoint.CreatedAt = existing.CreatedAt
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("检查检查点文件失败: %w", err)
		}
	}

	// 设置时间戳
	now := time.Now()
	checkpoint.UpdatedAt = now
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = now
	}

	// 确保线程目录存在
	dir := s.threadDir(checkpoint.ThreadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建线程目录失败: %w", err)
	}

	// 序列化检查点为 JSON
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化检查点失败: %w", err)
	}

	// 原子写入检查点文件，避免写入中断导致文件损坏
	if err := writeFileAtomic(cpPath, data, 0644); err != nil {
		return fmt.Errorf("写入检查点文件失败: %w", err)
	}

	// 更新索引文件
	idx, err := s.loadIndex(checkpoint.ThreadID)
	if err != nil {
		return fmt.Errorf("加载索引文件失败: %w", err)
	}

	// 同一个检查点 ID 只记录一次，避免重复索引
	exists := false
	for _, id := range idx.CheckpointIDs {
		if id == checkpoint.ID {
			exists = true
			break
		}
	}
	if !exists {
		idx.CheckpointIDs = append(idx.CheckpointIDs, checkpoint.ID)
	}

	if err := s.saveIndex(checkpoint.ThreadID, idx); err != nil {
		return fmt.Errorf("保存索引文件失败: %w", err)
	}

	return nil
}

// Load 加载线程的最新检查点
//
// 返回线程中最后保存的检查点。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - threadID: 线程 ID
//
// 返回：
//   - *Checkpoint: 最新的检查点
//   - error: 线程不存在或没有检查点时返回错误
func (s *FileCheckpointSaver) Load(ctx context.Context, threadID string) (*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 加载索引
	idx, err := s.loadIndex(threadID)
	if err != nil {
		return nil, fmt.Errorf("no checkpoint found for thread %s", threadID)
	}
	if len(idx.CheckpointIDs) == 0 {
		return nil, fmt.Errorf("no checkpoint found for thread %s", threadID)
	}

	// 返回最新的检查点（列表最后一个）
	latestID := idx.CheckpointIDs[len(idx.CheckpointIDs)-1]
	return s.readCheckpoint(threadID, latestID)
}

// LoadByID 根据 ID 加载检查点
//
// 遍历所有线程目录查找指定 ID 的检查点。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - id: 检查点 ID
//
// 返回：
//   - *Checkpoint: 找到的检查点
//   - error: 检查点不存在时返回错误
func (s *FileCheckpointSaver) LoadByID(ctx context.Context, id string) (*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, fmt.Errorf("checkpoint id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 遍历所有线程目录查找检查点
	threadsDir := filepath.Join(s.baseDir, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		return nil, fmt.Errorf("checkpoint %s not found", id)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := entry.Name()
		cp, err := s.readCheckpoint(threadID, id)
		if err == nil {
			return cp, nil
		}
	}

	return nil, fmt.Errorf("checkpoint %s not found", id)
}

// List 列出线程的所有检查点
//
// 按保存顺序返回线程中的所有检查点。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - threadID: 线程 ID
//
// 返回：
//   - []*Checkpoint: 检查点列表，如果线程不存在则返回 nil
//   - error: 读取错误时返回错误
func (s *FileCheckpointSaver) List(ctx context.Context, threadID string) ([]*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 加载索引
	idx, err := s.loadIndex(threadID)
	if err != nil {
		// 线程不存在时返回空列表
		return nil, nil
	}

	result := make([]*Checkpoint, 0, len(idx.CheckpointIDs))
	for _, id := range idx.CheckpointIDs {
		cp, err := s.readCheckpoint(threadID, id)
		if err != nil {
			continue
		}
		result = append(result, cp)
	}

	return result, nil
}

// Delete 删除指定的检查点
//
// 删除检查点文件并从索引中移除。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - id: 要删除的检查点 ID
//
// 返回：
//   - error: 删除失败时返回错误
func (s *FileCheckpointSaver) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("checkpoint id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 遍历所有线程目录查找并删除检查点
	threadsDir := filepath.Join(s.baseDir, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		// 目录不存在时静默返回
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := entry.Name()

		// 检查该线程下是否存在该检查点文件
		cpPath := s.checkpointPath(threadID, id)
		if _, err := os.Stat(cpPath); os.IsNotExist(err) {
			continue
		}

		// 删除检查点文件
		if err := os.Remove(cpPath); err != nil {
			return fmt.Errorf("删除检查点文件失败: %w", err)
		}

		// 从索引中移除
		idx, err := s.loadIndex(threadID)
		if err != nil {
			return fmt.Errorf("加载索引文件失败: %w", err)
		}
		newIDs := make([]string, 0, len(idx.CheckpointIDs))
		for _, existingID := range idx.CheckpointIDs {
			if existingID != id {
				newIDs = append(newIDs, existingID)
			}
		}
		idx.CheckpointIDs = newIDs
		if err := s.saveIndex(threadID, idx); err != nil {
			return fmt.Errorf("更新索引文件失败: %w", err)
		}

		return nil
	}

	return nil
}

// DeleteThread 删除线程的所有检查点
//
// 删除整个线程目录及其中所有检查点文件。
//
// 参数：
//   - ctx: 上下文（预留，当前未使用）
//   - threadID: 要删除的线程 ID
//
// 返回：
//   - error: 删除失败时返回错误
func (s *FileCheckpointSaver) DeleteThread(ctx context.Context, threadID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.threadDir(threadID)
	// 如果目录不存在，静默返回
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("删除线程目录失败: %w", err)
	}

	return nil
}

// threadDir 返回线程目录路径
//
// 格式：baseDir/threads/{threadID}/
func (s *FileCheckpointSaver) threadDir(threadID string) string {
	return filepath.Join(s.baseDir, "threads", threadID)
}

// checkpointPath 返回检查点文件路径
//
// 格式：baseDir/threads/{threadID}/{checkpointID}.json
func (s *FileCheckpointSaver) checkpointPath(threadID, checkpointID string) string {
	return filepath.Join(s.threadDir(threadID), checkpointID+".json")
}

// loadIndex 加载线程的索引文件
//
// 如果索引文件不存在，返回一个空的索引结构。
//
// 参数：
//   - threadID: 线程 ID
//
// 返回：
//   - *threadIndex: 索引结构
//   - error: 读取或解析失败时返回错误
func (s *FileCheckpointSaver) loadIndex(threadID string) (*threadIndex, error) {
	indexPath := filepath.Join(s.threadDir(threadID), "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &threadIndex{CheckpointIDs: []string{}}, nil
		}
		return nil, fmt.Errorf("读取索引文件失败: %w", err)
	}

	var idx threadIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析索引文件失败: %w", err)
	}

	return &idx, nil
}

// saveIndex 保存线程的索引文件
//
// 参数：
//   - threadID: 线程 ID
//   - idx: 要保存的索引结构
//
// 返回：
//   - error: 序列化或写入失败时返回错误
func (s *FileCheckpointSaver) saveIndex(threadID string, idx *threadIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化索引文件失败: %w", err)
	}

	indexPath := filepath.Join(s.threadDir(threadID), "index.json")
	if err := writeFileAtomic(indexPath, data, 0644); err != nil {
		return fmt.Errorf("写入索引文件失败: %w", err)
	}

	return nil
}

// readCheckpoint 从文件读取检查点
//
// 参数：
//   - threadID: 线程 ID
//   - checkpointID: 检查点 ID
//
// 返回：
//   - *Checkpoint: 读取到的检查点
//   - error: 文件不存在或解析失败时返回错误
func (s *FileCheckpointSaver) readCheckpoint(threadID, checkpointID string) (*Checkpoint, error) {
	cpPath := s.checkpointPath(threadID, checkpointID)
	data, err := os.ReadFile(cpPath)
	if err != nil {
		return nil, fmt.Errorf("读取检查点文件失败: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("解析检查点文件失败: %w", err)
	}

	return &cp, nil
}

// writeFileAtomic 原子写入文件：先写临时文件，再 rename 覆盖目标文件
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
