package datetime

import (
	"context"
	"testing"
	"time"

	"github.com/everyday-items/ai-core/tool"
)

func TestTools(t *testing.T) {
	tools := Tools()
	if len(tools) == 0 {
		t.Fatal("Tools() should return non-empty slice")
	}

	// 验证所有工具都实现了 Tool 接口
	for _, tl := range tools {
		if tl.Name() == "" {
			t.Error("Tool name should not be empty")
		}
		if tl.Description() == "" {
			t.Error("Tool description should not be empty")
		}
		if tl.Schema() == nil {
			t.Error("Tool schema should not be nil")
		}
	}
}

func TestDateTimeNow(t *testing.T) {
	tools := Tools()
	var datetimeNow tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_now" {
			datetimeNow = tl
			break
		}
	}
	if datetimeNow == nil {
		t.Fatal("datetime_now tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		timezone string
		format   string
		wantErr  bool
	}{
		{
			name:     "本地时区",
			timezone: "",
			format:   "",
		},
		{
			name:     "UTC时区",
			timezone: "UTC",
			format:   "",
		},
		{
			name:     "上海时区",
			timezone: "Asia/Shanghai",
			format:   "",
		},
		{
			name:     "自定义格式",
			timezone: "",
			format:   "2006-01-02",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{}
			if tt.timezone != "" {
				args["timezone"] = tt.timezone
			}
			if tt.format != "" {
				args["format"] = tt.format
			}

			result, err := datetimeNow.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if output, ok := result.Output.(DateTimeOutput); ok {
					if output.ISO8601 == "" {
						t.Error("ISO8601 should not be empty")
					}
					if output.Unix == 0 {
						t.Error("Unix timestamp should not be 0")
					}
				}
			}
		})
	}
}

func TestDateTimeParse(t *testing.T) {
	tools := Tools()
	var datetimeParse tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_parse" {
			datetimeParse = tl
			break
		}
	}
	if datetimeParse == nil {
		t.Fatal("datetime_parse tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		datetime string
		format   string
		wantErr  bool
	}{
		{
			name:     "RFC3339格式",
			datetime: "2023-01-01T12:00:00Z",
			format:   "",
		},
		{
			name:     "日期格式",
			datetime: "2023-01-01",
			format:   "",
		},
		{
			name:     "自定义格式",
			datetime: "2023/01/01",
			format:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime": tt.datetime,
			}
			if tt.format != "" {
				args["format"] = tt.format
			}

			result, err := datetimeParse.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if output, ok := result.Output.(DateTimeOutput); ok {
					if output.ISO8601 == "" {
						t.Error("ISO8601 should not be empty")
					}
				}
			}
		})
	}
}

func TestDateTimeFormat(t *testing.T) {
	tools := Tools()
	var datetimeFormat tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_format" {
			datetimeFormat = tl
			break
		}
	}
	if datetimeFormat == nil {
		t.Fatal("datetime_format tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		datetime string
		format   string
		wantErr  bool
	}{
		{
			name:     "格式化为日期",
			datetime: "2023-01-01T12:00:00Z",
			format:   "2006-01-02",
		},
		{
			name:     "格式化为时间",
			datetime: "2023-01-01T12:00:00Z",
			format:   "15:04:05",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime": tt.datetime,
				"format":   tt.format,
			}

			result, err := datetimeFormat.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if formatted, ok := resultMap["formatted"].(string); ok {
						if formatted == "" {
							t.Error("formatted should not be empty")
						}
					}
				}
			}
		})
	}
}

func TestDateTimeAdd(t *testing.T) {
	tools := Tools()
	var datetimeAdd tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_add" {
			datetimeAdd = tl
			break
		}
	}
	if datetimeAdd == nil {
		t.Fatal("datetime_add tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		datetime string
		years    int
		months   int
		days     int
		hours    int
		minutes  int
		seconds  int
		wantErr  bool
	}{
		{
			name:     "加1天",
			datetime: "2023-01-01T12:00:00Z",
			days:     1,
		},
		{
			name:     "减1小时",
			datetime: "2023-01-01T12:00:00Z",
			hours:    -1,
		},
		{
			name:     "复杂加法",
			datetime: "2023-01-01T12:00:00Z",
			years:    1,
			months:   2,
			days:     3,
			hours:    4,
			minutes:  5,
			seconds:  6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime": tt.datetime,
			}
			if tt.years != 0 {
				args["years"] = tt.years
			}
			if tt.months != 0 {
				args["months"] = tt.months
			}
			if tt.days != 0 {
				args["days"] = tt.days
			}
			if tt.hours != 0 {
				args["hours"] = tt.hours
			}
			if tt.minutes != 0 {
				args["minutes"] = tt.minutes
			}
			if tt.seconds != 0 {
				args["seconds"] = tt.seconds
			}

			result, err := datetimeAdd.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if output, ok := result.Output.(DateTimeOutput); ok {
					if output.ISO8601 == "" {
						t.Error("ISO8601 should not be empty")
					}
				}
			}
		})
	}
}

func TestDateTimeDiff(t *testing.T) {
	tools := Tools()
	var datetimeDiff tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_diff" {
			datetimeDiff = tl
			break
		}
	}
	if datetimeDiff == nil {
		t.Fatal("datetime_diff tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		start   string
		end     string
		wantErr bool
	}{
		{
			name:  "正常差值",
			start: "2023-01-01T00:00:00Z",
			end:   "2023-01-02T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"start": tt.start,
				"end":   tt.end,
			}

			result, err := datetimeDiff.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if diff, ok := result.Output.(DateTimeDiff); ok {
					if diff.TotalSeconds == 0 {
						t.Error("TotalSeconds should not be 0")
					}
				}
			}
		})
	}
}

func TestDateTimeConvertTimezone(t *testing.T) {
	tools := Tools()
	var datetimeConvert tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_convert_timezone" {
			datetimeConvert = tl
			break
		}
	}
	if datetimeConvert == nil {
		t.Fatal("datetime_convert_timezone tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		datetime     string
		fromTimezone string
		toTimezone   string
		wantErr      bool
	}{
		{
			name:       "UTC转上海",
			datetime:   "2023-01-01T12:00:00Z",
			toTimezone: "Asia/Shanghai",
		},
		{
			name:         "上海转UTC",
			datetime:     "2023-01-01T12:00:00",
			fromTimezone: "Asia/Shanghai",
			toTimezone:   "UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime":      tt.datetime,
				"to_timezone":   tt.toTimezone,
			}
			if tt.fromTimezone != "" {
				args["from_timezone"] = tt.fromTimezone
			}

			result, err := datetimeConvert.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if output, ok := result.Output.(DateTimeOutput); ok {
					if output.ISO8601 == "" {
						t.Error("ISO8601 should not be empty")
					}
				}
			}
		})
	}
}

func TestDateTimeUnix(t *testing.T) {
	tools := Tools()
	var datetimeUnix tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_unix" {
			datetimeUnix = tl
			break
		}
	}
	if datetimeUnix == nil {
		t.Fatal("datetime_unix tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		datetime  string
		timestamp int64
		unit      string
		wantErr   bool
	}{
		{
			name:     "时间转时间戳",
			datetime: "2023-01-01T00:00:00Z",
			unit:     "seconds",
		},
		{
			name:      "时间戳转时间",
			timestamp: 1672531200,
			unit:      "seconds",
		},
		{
			name:      "毫秒时间戳",
			timestamp: 1672531200000,
			unit:      "milliseconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{}
			if tt.datetime != "" {
				args["datetime"] = tt.datetime
			}
			if tt.timestamp != 0 {
				args["timestamp"] = tt.timestamp
			}
			if tt.unit != "" {
				args["unit"] = tt.unit
			}

			result, err := datetimeUnix.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if _, ok := resultMap["datetime"].(string); !ok {
						t.Error("result should contain datetime field")
					}
					if _, ok := resultMap["timestamp"].(int64); !ok {
						t.Error("result should contain timestamp field")
					}
				}
			}
		})
	}
}

func TestDateTimeComponents(t *testing.T) {
	tools := Tools()
	var datetimeComponents tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_components" {
			datetimeComponents = tl
			break
		}
	}
	if datetimeComponents == nil {
		t.Fatal("datetime_components tool not found")
	}

	ctx := context.Background()

	args := map[string]any{
		"datetime": "2023-03-15T14:30:45Z",
	}

	result, err := datetimeComponents.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if components, ok := result.Output.(DateTimeComponents); ok {
		if components.Year != 2023 {
			t.Errorf("Year = %d, want 2023", components.Year)
		}
		if components.Month != 3 {
			t.Errorf("Month = %d, want 3", components.Month)
		}
		if components.Day != 15 {
			t.Errorf("Day = %d, want 15", components.Day)
		}
		if components.Hour != 14 {
			t.Errorf("Hour = %d, want 14", components.Hour)
		}
		if components.Quarter != 1 {
			t.Errorf("Quarter = %d, want 1", components.Quarter)
		}
	}
}

func TestDateTimeIs(t *testing.T) {
	tools := Tools()
	var datetimeIs tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_is" {
			datetimeIs = tl
			break
		}
	}
	if datetimeIs == nil {
		t.Fatal("datetime_is tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		datetime string
		check    string
		wantErr  bool
	}{
		{
			name:     "检查是否周末",
			datetime: "2023-01-07T12:00:00Z", // Saturday
			check:    "weekend",
		},
		{
			name:     "检查是否工作日",
			datetime: "2023-01-09T12:00:00Z", // Monday
			check:    "weekday",
		},
		{
			name:     "检查是否闰年",
			datetime: "2024-01-01T12:00:00Z",
			check:    "leap_year",
		},
		{
			name:     "检查是否过去",
			datetime: "2020-01-01T12:00:00Z",
			check:    "past",
		},
		{
			name:     "检查是否未来",
			datetime: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			check:    "future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime": tt.datetime,
				"check":    tt.check,
			}

			result, err := datetimeIs.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if _, ok := resultMap["result"].(bool); !ok {
						t.Error("result should contain result field")
					}
					if _, ok := resultMap["reason"].(string); !ok {
						t.Error("result should contain reason field")
					}
				}
			}
		})
	}
}

func TestDateTimeRange(t *testing.T) {
	tools := Tools()
	var datetimeRange tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_range" {
			datetimeRange = tl
			break
		}
	}
	if datetimeRange == nil {
		t.Fatal("datetime_range tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		start     string
		end       string
		step      string
		wantErr   bool
		wantCount int
	}{
		{
			name:      "天范围",
			start:     "2023-01-01",
			end:       "2023-01-05",
			step:      "day",
			wantCount: 5,
		},
		{
			name:      "周范围",
			start:     "2023-01-01",
			end:       "2023-01-31",
			step:      "week",
			wantCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"start": tt.start,
				"end":   tt.end,
				"step":  tt.step,
			}

			result, err := datetimeRange.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if count, ok := resultMap["count"].(int); ok && tt.wantCount > 0 {
						if count != tt.wantCount {
							t.Errorf("count = %d, want %d", count, tt.wantCount)
						}
					}
				}
			}
		})
	}
}

func TestDateTimeRelative(t *testing.T) {
	tools := Tools()
	var datetimeRelative tool.Tool
	for _, tl := range tools {
		if tl.Name() == "datetime_relative" {
			datetimeRelative = tl
			break
		}
	}
	if datetimeRelative == nil {
		t.Fatal("datetime_relative tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		datetime string
		language string
		wantErr  bool
	}{
		{
			name:     "英文相对时间",
			datetime: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			language: "en",
		},
		{
			name:     "中文相对时间",
			datetime: time.Now().Add(-3 * time.Hour).Format(time.RFC3339),
			language: "zh",
		},
		{
			name:     "未来时间",
			datetime: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			language: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"datetime": tt.datetime,
			}
			if tt.language != "" {
				args["language"] = tt.language
			}

			result, err := datetimeRelative.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if relative, ok := resultMap["relative"].(string); ok {
						if relative == "" {
							t.Error("relative should not be empty")
						}
					}
				}
			}
		})
	}
}
