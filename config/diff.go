// Package config 提供配置差异对比能力
package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// DiffType 差异类型
type DiffType string

const (
	// DiffTypeAdded 新增字段
	DiffTypeAdded DiffType = "added"

	// DiffTypeRemoved 删除字段
	DiffTypeRemoved DiffType = "removed"

	// DiffTypeModified 修改字段
	DiffTypeModified DiffType = "modified"

	// DiffTypeUnchanged 未变化
	DiffTypeUnchanged DiffType = "unchanged"
)

// Diff 配置差异项
//
// 表示两个配置之间的一个差异点。
type Diff struct {
	// Path 字段路径（如 "llm.model"）
	Path string `json:"path"`

	// Type 差异类型
	Type DiffType `json:"type"`

	// OldValue 旧值
	OldValue any `json:"old_value,omitempty"`

	// NewValue 新值
	NewValue any `json:"new_value,omitempty"`

	// Message 差异描述
	Message string `json:"message,omitempty"`
}

// DiffResult 差异对比结果
//
// 包含所有差异项和统计信息。
type DiffResult struct {
	// Diffs 差异列表
	Diffs []Diff `json:"diffs"`

	// HasChanges 是否有变化
	HasChanges bool `json:"has_changes"`

	// AddedCount 新增字段数量
	AddedCount int `json:"added_count"`

	// RemovedCount 删除字段数量
	RemovedCount int `json:"removed_count"`

	// ModifiedCount 修改字段数量
	ModifiedCount int `json:"modified_count"`
}

// DiffConfigs 对比两个配置
//
// 参数：
//   - old: 旧配置对象
//   - new: 新配置对象
//
// 返回值：
//   - *DiffResult: 差异对比结果
//   - error: 错误（如果有）
func DiffConfigs(old, new any) (*DiffResult, error) {
	// 转换为 map 进行对比
	oldMap, err := toMap(old)
	if err != nil {
		return nil, fmt.Errorf("failed to convert old config: %w", err)
	}

	newMap, err := toMap(new)
	if err != nil {
		return nil, fmt.Errorf("failed to convert new config: %w", err)
	}

	// 对比差异
	diffs := diffMaps("", oldMap, newMap)

	// 统计差异
	result := &DiffResult{
		Diffs: diffs,
	}

	for _, d := range diffs {
		switch d.Type {
		case DiffTypeAdded:
			result.AddedCount++
			result.HasChanges = true
		case DiffTypeRemoved:
			result.RemovedCount++
			result.HasChanges = true
		case DiffTypeModified:
			result.ModifiedCount++
			result.HasChanges = true
		}
	}

	return result, nil
}

// DiffAgentConfigs 对比两个 Agent 配置
func DiffAgentConfigs(old, new *AgentConfig) (*DiffResult, error) {
	return DiffConfigs(old, new)
}

// DiffTeamConfigs 对比两个 Team 配置
func DiffTeamConfigs(old, new *TeamConfig) (*DiffResult, error) {
	return DiffConfigs(old, new)
}

// DiffWorkflowConfigs 对比两个 Workflow 配置
func DiffWorkflowConfigs(old, new *WorkflowConfig) (*DiffResult, error) {
	return DiffConfigs(old, new)
}

// Format 格式化差异结果为可读文本
//
// 返回值：
//   - string: 格式化的差异文本
func (r *DiffResult) Format() string {
	if !r.HasChanges {
		return "No changes detected."
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Changes Summary:\n"))
	sb.WriteString(fmt.Sprintf("  Added: %d\n", r.AddedCount))
	sb.WriteString(fmt.Sprintf("  Removed: %d\n", r.RemovedCount))
	sb.WriteString(fmt.Sprintf("  Modified: %d\n", r.ModifiedCount))
	sb.WriteString("\nDetails:\n")

	for _, d := range r.Diffs {
		switch d.Type {
		case DiffTypeAdded:
			sb.WriteString(fmt.Sprintf("  + %s: %v\n", d.Path, formatValue(d.NewValue)))
		case DiffTypeRemoved:
			sb.WriteString(fmt.Sprintf("  - %s: %v\n", d.Path, formatValue(d.OldValue)))
		case DiffTypeModified:
			sb.WriteString(fmt.Sprintf("  ~ %s: %v -> %v\n", d.Path, formatValue(d.OldValue), formatValue(d.NewValue)))
		}
	}

	return sb.String()
}

// FormatCompact 紧凑格式化差异结果
func (r *DiffResult) FormatCompact() string {
	if !r.HasChanges {
		return "No changes"
	}

	return fmt.Sprintf("+%d -%d ~%d",
		r.AddedCount, r.RemovedCount, r.ModifiedCount)
}

// toMap 将配置对象转换为 map
func toMap(v any) (map[string]any, error) {
	// 先序列化为 JSON
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// 再反序列化为 map
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// diffMaps 对比两个 map
func diffMaps(prefix string, old, new map[string]any) []Diff {
	diffs := make([]Diff, 0)

	// 检查旧 map 中的键
	for key, oldValue := range old {
		path := buildPath(prefix, key)

		if newValue, exists := new[key]; exists {
			// 键存在于两个 map 中，检查值
			childDiffs := diffValues(path, oldValue, newValue)
			diffs = append(diffs, childDiffs...)
		} else {
			// 键在新 map 中不存在，表示删除
			diffs = append(diffs, Diff{
				Path:     path,
				Type:     DiffTypeRemoved,
				OldValue: oldValue,
				Message:  fmt.Sprintf("Field '%s' was removed", path),
			})
		}
	}

	// 检查新 map 中新增的键
	for key, newValue := range new {
		path := buildPath(prefix, key)

		if _, exists := old[key]; !exists {
			// 键在旧 map 中不存在，表示新增
			diffs = append(diffs, Diff{
				Path:     path,
				Type:     DiffTypeAdded,
				NewValue: newValue,
				Message:  fmt.Sprintf("Field '%s' was added", path),
			})
		}
	}

	return diffs
}

// diffValues 对比两个值
func diffValues(path string, old, new any) []Diff {
	diffs := make([]Diff, 0)

	// 处理 nil 值
	if old == nil && new == nil {
		return diffs
	}
	if old == nil {
		diffs = append(diffs, Diff{
			Path:     path,
			Type:     DiffTypeModified,
			OldValue: nil,
			NewValue: new,
			Message:  fmt.Sprintf("'%s' changed from nil to %v", path, formatValue(new)),
		})
		return diffs
	}
	if new == nil {
		diffs = append(diffs, Diff{
			Path:     path,
			Type:     DiffTypeModified,
			OldValue: old,
			NewValue: nil,
			Message:  fmt.Sprintf("'%s' changed from %v to nil", path, formatValue(old)),
		})
		return diffs
	}

	// 获取值类型
	oldValue := reflect.ValueOf(old)
	newValue := reflect.ValueOf(new)

	// 类型不同，直接标记为修改
	if oldValue.Type() != newValue.Type() {
		diffs = append(diffs, Diff{
			Path:     path,
			Type:     DiffTypeModified,
			OldValue: old,
			NewValue: new,
			Message:  fmt.Sprintf("'%s' type changed from %v to %v", path, oldValue.Type(), newValue.Type()),
		})
		return diffs
	}

	// 根据类型对比
	switch oldValue.Kind() {
	case reflect.Map:
		// 递归对比 map
		oldMap, ok1 := old.(map[string]any)
		newMap, ok2 := new.(map[string]any)
		if ok1 && ok2 {
			diffs = append(diffs, diffMaps(path, oldMap, newMap)...)
		} else {
			diffs = append(diffs, Diff{
				Path:     path,
				Type:     DiffTypeModified,
				OldValue: old,
				NewValue: new,
			})
		}

	case reflect.Slice, reflect.Array:
		// 对比切片/数组
		if !reflect.DeepEqual(old, new) {
			diffs = append(diffs, Diff{
				Path:     path,
				Type:     DiffTypeModified,
				OldValue: old,
				NewValue: new,
				Message:  fmt.Sprintf("'%s' changed", path),
			})
		}

	default:
		// 基本类型对比
		if !reflect.DeepEqual(old, new) {
			diffs = append(diffs, Diff{
				Path:     path,
				Type:     DiffTypeModified,
				OldValue: old,
				NewValue: new,
				Message:  fmt.Sprintf("'%s' changed from %v to %v", path, formatValue(old), formatValue(new)),
			})
		}
	}

	return diffs
}

// buildPath 构建字段路径
func buildPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// formatValue 格式化值为字符串
func formatValue(v any) string {
	if v == nil {
		return "<nil>"
	}

	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return fmt.Sprintf(`"%s..."`, val[:50])
		}
		return fmt.Sprintf(`"%s"`, val)
	case []any:
		if len(val) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(val))
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d fields}", len(val))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// DiffSummary 差异摘要
//
// 提供更高层次的差异摘要信息。
type DiffSummary struct {
	// ConfigType 配置类型
	ConfigType string `json:"config_type"`

	// Result 差异结果
	Result *DiffResult `json:"result"`

	// KeyChanges 关键变化（如 LLM 模型变化、工具新增/删除等）
	KeyChanges []string `json:"key_changes,omitempty"`
}

// SummarizeAgentDiff 总结 Agent 配置差异
func SummarizeAgentDiff(old, new *AgentConfig) (*DiffSummary, error) {
	result, err := DiffAgentConfigs(old, new)
	if err != nil {
		return nil, err
	}

	summary := &DiffSummary{
		ConfigType: "agent",
		Result:     result,
		KeyChanges: make([]string, 0),
	}

	// 提取关键变化
	for _, d := range result.Diffs {
		switch {
		case strings.HasPrefix(d.Path, "llm.model"):
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("LLM model changed: %v -> %v", formatValue(d.OldValue), formatValue(d.NewValue)))

		case strings.HasPrefix(d.Path, "llm.provider"):
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("LLM provider changed: %v -> %v", formatValue(d.OldValue), formatValue(d.NewValue)))

		case d.Path == "tools" && d.Type == DiffTypeModified:
			summary.KeyChanges = append(summary.KeyChanges, "Tools configuration changed")

		case d.Path == "memory.type":
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("Memory type changed: %v -> %v", formatValue(d.OldValue), formatValue(d.NewValue)))

		case d.Path == "max_iterations":
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("Max iterations changed: %v -> %v", d.OldValue, d.NewValue))
		}
	}

	return summary, nil
}

// SummarizeTeamDiff 总结 Team 配置差异
func SummarizeTeamDiff(old, new *TeamConfig) (*DiffSummary, error) {
	result, err := DiffTeamConfigs(old, new)
	if err != nil {
		return nil, err
	}

	summary := &DiffSummary{
		ConfigType: "team",
		Result:     result,
		KeyChanges: make([]string, 0),
	}

	// 提取关键变化
	for _, d := range result.Diffs {
		switch {
		case d.Path == "mode":
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("Team mode changed: %v -> %v", formatValue(d.OldValue), formatValue(d.NewValue)))

		case d.Path == "agents" && d.Type == DiffTypeModified:
			summary.KeyChanges = append(summary.KeyChanges, "Agent composition changed")

		case d.Path == "manager":
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("Manager changed: %v -> %v", formatValue(d.OldValue), formatValue(d.NewValue)))

		case d.Path == "max_rounds":
			summary.KeyChanges = append(summary.KeyChanges,
				fmt.Sprintf("Max rounds changed: %v -> %v", d.OldValue, d.NewValue))
		}
	}

	return summary, nil
}
