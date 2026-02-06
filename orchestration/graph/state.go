// Package graph 提供 Hexagon AI Agent 框架的图编排引擎
package graph

import (
	"encoding/json"
	"fmt"
	"os"
)

// State 是图状态的约束接口
// 所有状态类型都必须实现此接口
type State interface {
	// Clone 创建状态的深拷贝
	Clone() State
}

// Reducer 状态合并器
// 用于合并多个节点的输出到状态中
type Reducer[S State, V any] interface {
	// Reduce 将新值合并到状态中
	Reduce(state S, key string, value V) S
}

// OverwriteReducer 覆盖式合并器
// 新值直接覆盖旧值
type OverwriteReducer[S State, V any] struct{}

func (r OverwriteReducer[S, V]) Reduce(state S, key string, value V) S {
	// 由具体状态类型处理
	return state
}

// AppendReducer 追加式合并器
// 将新值追加到切片中
type AppendReducer[S State, V any] struct{}

func (r AppendReducer[S, V]) Reduce(state S, key string, value V) S {
	// 由具体状态类型处理
	return state
}

// MapState 通用的 map 状态实现
type MapState map[string]any

// Clone 创建状态的深拷贝
func (s MapState) Clone() State {
	if s == nil {
		return MapState{}
	}
	clone := make(MapState, len(s))
	for k, v := range s {
		clone[k] = deepCopy(v)
	}
	return clone
}

// Get 获取值
func (s MapState) Get(key string) (any, bool) {
	v, ok := s[key]
	return v, ok
}

// Set 设置值
func (s MapState) Set(key string, value any) {
	s[key] = value
}

// Delete 删除值
func (s MapState) Delete(key string) {
	delete(s, key)
}

// Merge 合并另一个状态
func (s MapState) Merge(other MapState) {
	for k, v := range other {
		s[k] = v
	}
}

// Channel 通道定义
// 用于定义状态中的特定字段如何被更新
type Channel[V any] struct {
	// Name 通道名称（对应状态字段）
	Name string

	// Default 默认值
	Default V

	// Reducer 合并函数
	Reducer func(current V, new V) V
}

// NewChannel 创建新通道
func NewChannel[V any](name string, defaultValue V) *Channel[V] {
	return &Channel[V]{
		Name:    name,
		Default: defaultValue,
		Reducer: func(current V, new V) V {
			return new // 默认覆盖
		},
	}
}

// WithReducer 设置自定义合并函数
func (c *Channel[V]) WithReducer(reducer func(current V, new V) V) *Channel[V] {
	c.Reducer = reducer
	return c
}

// MessageChannel 消息通道（追加模式）
func MessageChannel[V any](name string) *Channel[[]V] {
	return &Channel[[]V]{
		Name:    name,
		Default: nil,
		Reducer: func(current []V, new []V) []V {
			return append(current, new...)
		},
	}
}

// StateAnnotation 状态注解
// 用于定义状态结构和通道
type StateAnnotation struct {
	Channels map[string]ChannelConfig
}

// ChannelConfig 通道配置
type ChannelConfig struct {
	// Type 通道类型: "overwrite", "append", "custom"
	Type string `json:"type"`

	// Default 默认值
	Default any `json:"default,omitempty"`
}

// deepCopy 深拷贝值
func deepCopy(v any) any {
	if v == nil {
		return nil
	}

	// 对于基本类型，直接返回
	switch val := v.(type) {
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		return val
	case []any:
		clone := make([]any, len(val))
		for i, item := range val {
			clone[i] = deepCopy(item)
		}
		return clone
	case map[string]any:
		clone := make(map[string]any, len(val))
		for k, item := range val {
			clone[k] = deepCopy(item)
		}
		return clone
	default:
		// 对于复杂类型，使用 JSON 序列化/反序列化
		data, err := json.Marshal(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] deepCopy: JSON marshal failed for type %T, returning original reference: %v\n", v, err)
			return v
		}
		var clone any
		if err := json.Unmarshal(data, &clone); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] deepCopy: JSON unmarshal failed for type %T, returning original reference: %v\n", v, err)
			return v
		}
		return clone
	}
}
