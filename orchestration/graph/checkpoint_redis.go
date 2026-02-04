package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes
	checkpointKeyPrefix = "hexagon:checkpoint:"
	threadKeyPrefix     = "hexagon:thread:"

	// Default TTL for checkpoints (7 days)
	defaultCheckpointTTL = 7 * 24 * time.Hour
)

// RedisCheckpointSaver 基于 Redis 的检查点保存器
type RedisCheckpointSaver struct {
	client *redis.Client
	ttl    time.Duration
}

// RedisCheckpointOption 是 RedisCheckpointSaver 的配置选项
type RedisCheckpointOption func(*RedisCheckpointSaver)

// WithTTL 设置检查点过期时间
func WithTTL(ttl time.Duration) RedisCheckpointOption {
	return func(s *RedisCheckpointSaver) {
		s.ttl = ttl
	}
}

// NewRedisCheckpointSaver 创建基于 Redis 的检查点保存器
func NewRedisCheckpointSaver(client *redis.Client, opts ...RedisCheckpointOption) *RedisCheckpointSaver {
	s := &RedisCheckpointSaver{
		client: client,
		ttl:    defaultCheckpointTTL,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewRedisCheckpointSaverFromURL 从 URL 创建 Redis 检查点保存器
func NewRedisCheckpointSaverFromURL(redisURL string, opts ...RedisCheckpointOption) (*RedisCheckpointSaver, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opt)
	return NewRedisCheckpointSaver(client, opts...), nil
}

// checkpointKey 返回检查点的 Redis key
func checkpointKey(id string) string {
	return checkpointKeyPrefix + id
}

// threadKey 返回线程的 Redis key
func threadKey(threadID string) string {
	return threadKeyPrefix + threadID
}

// Save 保存检查点
func (s *RedisCheckpointSaver) Save(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}
	checkpoint.UpdatedAt = time.Now()
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = checkpoint.UpdatedAt
	}

	// 序列化检查点
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	// 使用 Pipeline 批量执行
	pipe := s.client.Pipeline()

	// 保存检查点
	pipe.Set(ctx, checkpointKey(checkpoint.ID), data, s.ttl)

	// 将检查点 ID 添加到线程的有序集合中（按时间戳排序）
	pipe.ZAdd(ctx, threadKey(checkpoint.ThreadID), redis.Z{
		Score:  float64(checkpoint.CreatedAt.UnixNano()),
		Member: checkpoint.ID,
	})

	// 设置线程 key 的过期时间
	pipe.Expire(ctx, threadKey(checkpoint.ThreadID), s.ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save checkpoint to redis: %w", err)
	}

	return nil
}

// Load 加载最新的检查点
func (s *RedisCheckpointSaver) Load(ctx context.Context, threadID string) (*Checkpoint, error) {
	// 从有序集合中获取最新的检查点 ID
	ids, err := s.client.ZRevRange(ctx, threadKey(threadID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("get latest checkpoint id: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoint found for thread %s", threadID)
	}

	return s.LoadByID(ctx, ids[0])
}

// LoadByID 根据 ID 加载检查点
func (s *RedisCheckpointSaver) LoadByID(ctx context.Context, id string) (*Checkpoint, error) {
	data, err := s.client.Get(ctx, checkpointKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("checkpoint %s not found", id)
		}
		return nil, fmt.Errorf("get checkpoint from redis: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// List 列出线程的所有检查点
func (s *RedisCheckpointSaver) List(ctx context.Context, threadID string) ([]*Checkpoint, error) {
	// 获取所有检查点 ID（按时间戳升序）
	ids, err := s.client.ZRange(ctx, threadKey(threadID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("list checkpoint ids: %w", err)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	// 批量获取检查点
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = checkpointKey(id)
	}

	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("batch get checkpoints: %w", err)
	}

	result := make([]*Checkpoint, 0, len(values))
	var unmarshalErrors []error
	for i, v := range values {
		if v == nil {
			continue
		}
		var checkpoint Checkpoint
		if err := json.Unmarshal([]byte(v.(string)), &checkpoint); err != nil {
			// 记录解析错误，但继续处理其他检查点
			unmarshalErrors = append(unmarshalErrors, fmt.Errorf("checkpoint %d: %w", i, err))
			continue
		}
		result = append(result, &checkpoint)
	}

	// 如果有解析错误，返回部分结果和合并的错误信息
	// 但不影响已成功解析的检查点
	if len(unmarshalErrors) > 0 {
		// 返回结果，但同时记录警告（不返回错误，以保持向后兼容）
		// 调用方可以通过 LoadByThreadIDWithWarnings 获取详细错误
		return result, nil
	}

	return result, nil
}

// LoadByThreadIDWithWarnings 加载线程的所有检查点，同时返回解析警告
func (s *RedisCheckpointSaver) LoadByThreadIDWithWarnings(ctx context.Context, threadID string) ([]*Checkpoint, []error, error) {
	// 获取线程的所有检查点 ID
	ids, err := s.client.ZRange(ctx, threadKey(threadID), 0, -1).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("get thread checkpoint ids: %w", err)
	}

	if len(ids) == 0 {
		return nil, nil, nil
	}

	// 批量获取检查点
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = checkpointKey(id)
	}

	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("batch get checkpoints: %w", err)
	}

	result := make([]*Checkpoint, 0, len(values))
	var warnings []error
	for i, v := range values {
		if v == nil {
			continue
		}
		var checkpoint Checkpoint
		if err := json.Unmarshal([]byte(v.(string)), &checkpoint); err != nil {
			warnings = append(warnings, fmt.Errorf("failed to unmarshal checkpoint %s: %w", ids[i], err))
			continue
		}
		result = append(result, &checkpoint)
	}

	return result, warnings, nil
}

// Delete 删除检查点
func (s *RedisCheckpointSaver) Delete(ctx context.Context, id string) error {
	// 先获取检查点以获取 threadID
	checkpoint, err := s.LoadByID(ctx, id)
	if err != nil {
		// 如果检查点不存在，视为删除成功
		return nil
	}

	pipe := s.client.Pipeline()

	// 删除检查点
	pipe.Del(ctx, checkpointKey(id))

	// 从线程的有序集合中移除
	pipe.ZRem(ctx, threadKey(checkpoint.ThreadID), id)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete checkpoint from redis: %w", err)
	}

	return nil
}

// DeleteThread 删除线程的所有检查点
func (s *RedisCheckpointSaver) DeleteThread(ctx context.Context, threadID string) error {
	// 获取所有检查点 ID
	ids, err := s.client.ZRange(ctx, threadKey(threadID), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("get checkpoint ids: %w", err)
	}

	if len(ids) == 0 {
		return nil
	}

	// 构建要删除的 keys
	keys := make([]string, len(ids)+1)
	for i, id := range ids {
		keys[i] = checkpointKey(id)
	}
	keys[len(ids)] = threadKey(threadID)

	// 批量删除
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete checkpoints from redis: %w", err)
	}

	return nil
}

// ListThreads 列出所有线程 ID
func (s *RedisCheckpointSaver) ListThreads(ctx context.Context, pattern string, limit int64) ([]string, error) {
	if pattern == "" {
		pattern = "*"
	}
	if limit <= 0 {
		limit = 100
	}

	cursor := uint64(0)
	var threads []string

	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, threadKeyPrefix+pattern, limit).Result()
		if err != nil {
			return nil, fmt.Errorf("scan threads: %w", err)
		}

		for _, key := range keys {
			// 提取 threadID
			threadID := key[len(threadKeyPrefix):]
			threads = append(threads, threadID)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return threads, nil
}

// GetCheckpointCount 获取线程的检查点数量
func (s *RedisCheckpointSaver) GetCheckpointCount(ctx context.Context, threadID string) (int64, error) {
	return s.client.ZCard(ctx, threadKey(threadID)).Result()
}

// Prune 清理旧的检查点，保留最新的 n 个
func (s *RedisCheckpointSaver) Prune(ctx context.Context, threadID string, keepCount int64) error {
	// 获取要删除的检查点 ID（保留最新的 keepCount 个）
	// ZRemRangeByRank 删除排名从 0 到 -(keepCount+1) 的元素
	removeCount := -keepCount - 1

	// 先获取要删除的 ID
	ids, err := s.client.ZRange(ctx, threadKey(threadID), 0, removeCount).Result()
	if err != nil {
		return fmt.Errorf("get checkpoint ids to prune: %w", err)
	}

	if len(ids) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()

	// 删除检查点数据
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = checkpointKey(id)
	}
	pipe.Del(ctx, keys...)

	// 从有序集合中移除
	pipe.ZRemRangeByRank(ctx, threadKey(threadID), 0, removeCount)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("prune checkpoints: %w", err)
	}

	return nil
}

// Close 关闭 Redis 连接
func (s *RedisCheckpointSaver) Close() error {
	return s.client.Close()
}

// 确保实现了接口
var _ CheckpointSaver = (*RedisCheckpointSaver)(nil)
