package graph

import (
	"context"
	"encoding/json"
	"errors"
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.client == nil {
		return errors.New("redis client is nil")
	}
	if checkpoint == nil {
		return errors.New("checkpoint is nil")
	}
	if checkpoint.ThreadID == "" {
		return errors.New("checkpoint thread_id is required")
	}

	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}

	// 更新已有检查点时，如果未显式设置 CreatedAt，沿用原值
	if checkpoint.CreatedAt.IsZero() {
		existing, err := s.LoadByID(ctx, checkpoint.ID)
		if err == nil && existing != nil && !existing.CreatedAt.IsZero() {
			checkpoint.CreatedAt = existing.CreatedAt
		}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, errors.New("redis client is nil")
	}

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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, errors.New("redis client is nil")
	}
	if id == "" {
		return nil, fmt.Errorf("checkpoint id is required")
	}

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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, errors.New("redis client is nil")
	}

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
	for i, v := range values {
		if v == nil {
			continue
		}
		raw, err := mgetValueToBytes(v)
		if err != nil {
			// 跳过不可解析值，保持向后兼容
			_ = i
			continue
		}
		var checkpoint Checkpoint
		if err := json.Unmarshal(raw, &checkpoint); err != nil {
			continue
		}
		result = append(result, &checkpoint)
	}

	return result, nil
}

// LoadByThreadIDWithWarnings 加载线程的所有检查点，同时返回解析警告
func (s *RedisCheckpointSaver) LoadByThreadIDWithWarnings(ctx context.Context, threadID string) ([]*Checkpoint, []error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if s.client == nil {
		return nil, nil, errors.New("redis client is nil")
	}

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
		raw, err := mgetValueToBytes(v)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("unexpected checkpoint value type for %s: %w", ids[i], err))
			continue
		}
		var checkpoint Checkpoint
		if err := json.Unmarshal(raw, &checkpoint); err != nil {
			warnings = append(warnings, fmt.Errorf("failed to unmarshal checkpoint %s: %w", ids[i], err))
			continue
		}
		result = append(result, &checkpoint)
	}

	return result, warnings, nil
}

// Delete 删除检查点
func (s *RedisCheckpointSaver) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.client == nil {
		return errors.New("redis client is nil")
	}
	if id == "" {
		return fmt.Errorf("checkpoint id is required")
	}

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
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.client == nil {
		return errors.New("redis client is nil")
	}

	// 获取所有检查点 ID
	ids, err := s.client.ZRange(ctx, threadKey(threadID), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("get checkpoint ids: %w", err)
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, errors.New("redis client is nil")
	}

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
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s.client == nil {
		return 0, errors.New("redis client is nil")
	}

	return s.client.ZCard(ctx, threadKey(threadID)).Result()
}

// Prune 清理旧的检查点，保留最新的 n 个
func (s *RedisCheckpointSaver) Prune(ctx context.Context, threadID string, keepCount int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.client == nil {
		return errors.New("redis client is nil")
	}

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
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

// 确保实现了接口
var _ CheckpointSaver = (*RedisCheckpointSaver)(nil)

func mgetValueToBytes(v any) ([]byte, error) {
	switch val := v.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	default:
		return nil, fmt.Errorf("unsupported mget value type %T", v)
	}
}
