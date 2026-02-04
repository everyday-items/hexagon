// Package datetime 提供日期时间处理工具
//
// 支持日期时间格式化、解析、计算、时区转换等功能。
// 适用于 Agent 需要处理时间数据的场景。
package datetime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/tool"
)

// Tools 返回日期时间工具集合
func Tools() []tool.Tool {
	return []tool.Tool{
		// 获取当前时间
		tool.NewFunc(
			"datetime_now",
			"获取当前日期时间",
			func(ctx context.Context, input struct {
				Timezone string `json:"timezone,omitempty" description:"时区 (如 Asia/Shanghai, UTC, America/New_York)，默认本地时区"`
				Format   string `json:"format,omitempty" description:"输出格式 (如 2006-01-02 15:04:05)，默认 RFC3339"`
			}) (DateTimeOutput, error) {
				now := time.Now()

				if input.Timezone != "" {
					loc, err := time.LoadLocation(input.Timezone)
					if err != nil {
						return DateTimeOutput{}, fmt.Errorf("invalid timezone: %s", input.Timezone)
					}
					now = now.In(loc)
				}

				format := input.Format
				if format == "" {
					format = time.RFC3339
				}

				return formatDateTime(now, format), nil
			},
		),

		// 解析日期时间
		tool.NewFunc(
			"datetime_parse",
			"解析日期时间字符串",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"日期时间字符串"`
				Format   string `json:"format,omitempty" description:"输入格式 (如 2006-01-02 15:04:05)，留空则自动检测"`
			}) (DateTimeOutput, error) {
				t, err := parseDateTime(input.DateTime, input.Format)
				if err != nil {
					return DateTimeOutput{}, err
				}
				return formatDateTime(t, time.RFC3339), nil
			},
		),

		// 格式化日期时间
		tool.NewFunc(
			"datetime_format",
			"格式化日期时间",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"日期时间字符串"`
				Format   string `json:"format" description:"输出格式 (如 2006-01-02, Monday January 2, 2006)"`
			}) (struct {
				Formatted string `json:"formatted"`
			}, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return struct {
						Formatted string `json:"formatted"`
					}{}, err
				}
				return struct {
					Formatted string `json:"formatted"`
				}{Formatted: t.Format(input.Format)}, nil
			},
		),

		// 日期计算
		tool.NewFunc(
			"datetime_add",
			"日期时间加减计算",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"基准日期时间"`
				Years    int    `json:"years,omitempty" description:"增加的年数 (负数表示减少)"`
				Months   int    `json:"months,omitempty" description:"增加的月数"`
				Days     int    `json:"days,omitempty" description:"增加的天数"`
				Hours    int    `json:"hours,omitempty" description:"增加的小时数"`
				Minutes  int    `json:"minutes,omitempty" description:"增加的分钟数"`
				Seconds  int    `json:"seconds,omitempty" description:"增加的秒数"`
			}) (DateTimeOutput, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return DateTimeOutput{}, err
				}

				t = t.AddDate(input.Years, input.Months, input.Days)
				t = t.Add(time.Duration(input.Hours) * time.Hour)
				t = t.Add(time.Duration(input.Minutes) * time.Minute)
				t = t.Add(time.Duration(input.Seconds) * time.Second)

				return formatDateTime(t, time.RFC3339), nil
			},
		),

		// 日期差值
		tool.NewFunc(
			"datetime_diff",
			"计算两个日期时间的差值",
			func(ctx context.Context, input struct {
				Start string `json:"start" description:"开始日期时间"`
				End   string `json:"end" description:"结束日期时间"`
			}) (DateTimeDiff, error) {
				t1, err := parseDateTime(input.Start, "")
				if err != nil {
					return DateTimeDiff{}, fmt.Errorf("invalid start datetime: %w", err)
				}
				t2, err := parseDateTime(input.End, "")
				if err != nil {
					return DateTimeDiff{}, fmt.Errorf("invalid end datetime: %w", err)
				}

				return calculateDiff(t1, t2), nil
			},
		),

		// 时区转换
		tool.NewFunc(
			"datetime_convert_timezone",
			"转换时区",
			func(ctx context.Context, input struct {
				DateTime     string `json:"datetime" description:"日期时间字符串"`
				FromTimezone string `json:"from_timezone,omitempty" description:"源时区 (默认 UTC)"`
				ToTimezone   string `json:"to_timezone" description:"目标时区"`
			}) (DateTimeOutput, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return DateTimeOutput{}, err
				}

				// 设置源时区
				if input.FromTimezone != "" {
					loc, err := time.LoadLocation(input.FromTimezone)
					if err != nil {
						return DateTimeOutput{}, fmt.Errorf("invalid from_timezone: %s", input.FromTimezone)
					}
					t = time.Date(
						t.Year(), t.Month(), t.Day(),
						t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
						loc,
					)
				}

				// 转换到目标时区
				loc, err := time.LoadLocation(input.ToTimezone)
				if err != nil {
					return DateTimeOutput{}, fmt.Errorf("invalid to_timezone: %s", input.ToTimezone)
				}
				t = t.In(loc)

				return formatDateTime(t, time.RFC3339), nil
			},
		),

		// Unix 时间戳转换
		tool.NewFunc(
			"datetime_unix",
			"Unix 时间戳转换",
			func(ctx context.Context, input struct {
				DateTime  string `json:"datetime,omitempty" description:"日期时间字符串 (转为时间戳)"`
				Timestamp int64  `json:"timestamp,omitempty" description:"Unix 时间戳 (转为日期时间)"`
				Unit      string `json:"unit,omitempty" description:"时间戳单位: seconds (默认), milliseconds"`
			}) (struct {
				DateTime  string `json:"datetime"`
				Timestamp int64  `json:"timestamp"`
			}, error) {
				if input.DateTime != "" {
					t, err := parseDateTime(input.DateTime, "")
					if err != nil {
						return struct {
							DateTime  string `json:"datetime"`
							Timestamp int64  `json:"timestamp"`
						}{}, err
					}

					ts := t.Unix()
					if input.Unit == "milliseconds" {
						ts = t.UnixMilli()
					}

					return struct {
						DateTime  string `json:"datetime"`
						Timestamp int64  `json:"timestamp"`
					}{
						DateTime:  t.Format(time.RFC3339),
						Timestamp: ts,
					}, nil
				}

				ts := input.Timestamp
				if input.Unit == "milliseconds" {
					ts = ts / 1000
				}
				t := time.Unix(ts, 0)

				return struct {
					DateTime  string `json:"datetime"`
					Timestamp int64  `json:"timestamp"`
				}{
					DateTime:  t.Format(time.RFC3339),
					Timestamp: t.Unix(),
				}, nil
			},
		),

		// 获取日期组件
		tool.NewFunc(
			"datetime_components",
			"获取日期时间的各个组件 (年、月、日、时、分、秒、星期等)",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"日期时间字符串"`
			}) (DateTimeComponents, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return DateTimeComponents{}, err
				}
				return extractComponents(t), nil
			},
		),

		// 判断日期
		tool.NewFunc(
			"datetime_is",
			"判断日期属性 (是否周末、闰年、工作日等)",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"日期时间字符串"`
				Check    string `json:"check" description:"检查类型: weekend, weekday, leap_year, today, past, future"`
			}) (struct {
				Result bool   `json:"result"`
				Reason string `json:"reason"`
			}, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return struct {
						Result bool   `json:"result"`
						Reason string `json:"reason"`
					}{}, err
				}

				var result bool
				var reason string

				switch input.Check {
				case "weekend":
					wd := t.Weekday()
					result = wd == time.Saturday || wd == time.Sunday
					reason = fmt.Sprintf("%s is a %s", t.Format("2006-01-02"), wd.String())
				case "weekday":
					wd := t.Weekday()
					result = wd != time.Saturday && wd != time.Sunday
					reason = fmt.Sprintf("%s is a %s", t.Format("2006-01-02"), wd.String())
				case "leap_year":
					year := t.Year()
					result = (year%4 == 0 && year%100 != 0) || (year%400 == 0)
					if result {
						reason = fmt.Sprintf("%d is a leap year", year)
					} else {
						reason = fmt.Sprintf("%d is not a leap year", year)
					}
				case "today":
					now := time.Now()
					result = t.Year() == now.Year() && t.YearDay() == now.YearDay()
					reason = t.Format("2006-01-02")
				case "past":
					result = t.Before(time.Now())
					if result {
						reason = "date is in the past"
					} else {
						reason = "date is not in the past"
					}
				case "future":
					result = t.After(time.Now())
					if result {
						reason = "date is in the future"
					} else {
						reason = "date is not in the future"
					}
				default:
					return struct {
						Result bool   `json:"result"`
						Reason string `json:"reason"`
					}{}, fmt.Errorf("unknown check type: %s", input.Check)
				}

				return struct {
					Result bool   `json:"result"`
					Reason string `json:"reason"`
				}{Result: result, Reason: reason}, nil
			},
		),

		// 生成日期范围
		tool.NewFunc(
			"datetime_range",
			"生成日期范围",
			func(ctx context.Context, input struct {
				Start string `json:"start" description:"开始日期"`
				End   string `json:"end" description:"结束日期"`
				Step  string `json:"step" description:"步长: day, week, month, year"`
			}) (struct {
				Dates []string `json:"dates"`
				Count int      `json:"count"`
			}, error) {
				t1, err := parseDateTime(input.Start, "")
				if err != nil {
					return struct {
						Dates []string `json:"dates"`
						Count int      `json:"count"`
					}{}, fmt.Errorf("invalid start date: %w", err)
				}
				t2, err := parseDateTime(input.End, "")
				if err != nil {
					return struct {
						Dates []string `json:"dates"`
						Count int      `json:"count"`
					}{}, fmt.Errorf("invalid end date: %w", err)
				}

				var dates []string
				current := t1
				maxDates := 366 // 防止无限循环

				for !current.After(t2) && len(dates) < maxDates {
					dates = append(dates, current.Format("2006-01-02"))
					switch input.Step {
					case "day":
						current = current.AddDate(0, 0, 1)
					case "week":
						current = current.AddDate(0, 0, 7)
					case "month":
						current = current.AddDate(0, 1, 0)
					case "year":
						current = current.AddDate(1, 0, 0)
					default:
						current = current.AddDate(0, 0, 1)
					}
				}

				return struct {
					Dates []string `json:"dates"`
					Count int      `json:"count"`
				}{Dates: dates, Count: len(dates)}, nil
			},
		),

		// 相对时间描述
		tool.NewFunc(
			"datetime_relative",
			"生成相对时间描述 (如 '3 天前', '2 小时后')",
			func(ctx context.Context, input struct {
				DateTime string `json:"datetime" description:"日期时间字符串"`
				Language string `json:"language,omitempty" description:"语言: en (默认), zh"`
			}) (struct {
				Relative string `json:"relative"`
			}, error) {
				t, err := parseDateTime(input.DateTime, "")
				if err != nil {
					return struct {
						Relative string `json:"relative"`
					}{}, err
				}

				relative := getRelativeTime(t, input.Language)
				return struct {
					Relative string `json:"relative"`
				}{Relative: relative}, nil
			},
		),
	}
}

// DateTimeOutput 日期时间输出
type DateTimeOutput struct {
	ISO8601   string `json:"iso8601"`
	Unix      int64  `json:"unix"`
	Year      int    `json:"year"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	Hour      int    `json:"hour"`
	Minute    int    `json:"minute"`
	Second    int    `json:"second"`
	Weekday   string `json:"weekday"`
	DayOfYear int    `json:"day_of_year"`
	WeekOfYear int   `json:"week_of_year"`
	Timezone  string `json:"timezone"`
}

// DateTimeDiff 日期差值
type DateTimeDiff struct {
	Years        int     `json:"years"`
	Months       int     `json:"months"`
	Days         int     `json:"days"`
	Hours        int     `json:"hours"`
	Minutes      int     `json:"minutes"`
	Seconds      int     `json:"seconds"`
	TotalDays    float64 `json:"total_days"`
	TotalHours   float64 `json:"total_hours"`
	TotalMinutes float64 `json:"total_minutes"`
	TotalSeconds float64 `json:"total_seconds"`
	Positive     bool    `json:"positive"`
}

// DateTimeComponents 日期时间组件
type DateTimeComponents struct {
	Year       int    `json:"year"`
	Month      int    `json:"month"`
	MonthName  string `json:"month_name"`
	Day        int    `json:"day"`
	Hour       int    `json:"hour"`
	Minute     int    `json:"minute"`
	Second     int    `json:"second"`
	Weekday    int    `json:"weekday"`
	WeekdayName string `json:"weekday_name"`
	YearDay    int    `json:"year_day"`
	ISOWeek    int    `json:"iso_week"`
	Quarter    int    `json:"quarter"`
	IsLeapYear bool   `json:"is_leap_year"`
}

// formatDateTime 格式化时间
func formatDateTime(t time.Time, format string) DateTimeOutput {
	_, week := t.ISOWeek()
	return DateTimeOutput{
		ISO8601:    t.Format(format),
		Unix:       t.Unix(),
		Year:       t.Year(),
		Month:      int(t.Month()),
		Day:        t.Day(),
		Hour:       t.Hour(),
		Minute:     t.Minute(),
		Second:     t.Second(),
		Weekday:    t.Weekday().String(),
		DayOfYear:  t.YearDay(),
		WeekOfYear: week,
		Timezone:   t.Location().String(),
	}
}

// parseDateTime 解析时间
func parseDateTime(s string, format string) (time.Time, error) {
	if format != "" {
		return time.Parse(format, s)
	}

	// 尝试常见格式
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		"02/01/2006",
		"01/02/2006",
		"Jan 2, 2006",
		"January 2, 2006",
		"2006年01月02日",
		"15:04:05",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", s)
}

// calculateDiff 计算时间差
func calculateDiff(t1, t2 time.Time) DateTimeDiff {
	diff := t2.Sub(t1)
	positive := diff >= 0
	if !positive {
		diff = -diff
	}

	totalSeconds := diff.Seconds()
	totalMinutes := diff.Minutes()
	totalHours := diff.Hours()
	totalDays := totalHours / 24

	// 计算年月日（近似）
	years := int(totalDays / 365.25)
	months := int((totalDays - float64(years)*365.25) / 30.44)
	days := int(totalDays - float64(years)*365.25 - float64(months)*30.44)
	hours := int(totalHours) % 24
	minutes := int(totalMinutes) % 60
	seconds := int(totalSeconds) % 60

	return DateTimeDiff{
		Years:        years,
		Months:       months,
		Days:         days,
		Hours:        hours,
		Minutes:      minutes,
		Seconds:      seconds,
		TotalDays:    totalDays,
		TotalHours:   totalHours,
		TotalMinutes: totalMinutes,
		TotalSeconds: totalSeconds,
		Positive:     positive,
	}
}

// extractComponents 提取日期组件
func extractComponents(t time.Time) DateTimeComponents {
	_, week := t.ISOWeek()
	year := t.Year()
	isLeap := (year%4 == 0 && year%100 != 0) || (year%400 == 0)

	return DateTimeComponents{
		Year:        year,
		Month:       int(t.Month()),
		MonthName:   t.Month().String(),
		Day:         t.Day(),
		Hour:        t.Hour(),
		Minute:      t.Minute(),
		Second:      t.Second(),
		Weekday:     int(t.Weekday()),
		WeekdayName: t.Weekday().String(),
		YearDay:     t.YearDay(),
		ISOWeek:     week,
		Quarter:     (int(t.Month())-1)/3 + 1,
		IsLeapYear:  isLeap,
	}
}

// getRelativeTime 获取相对时间描述
func getRelativeTime(t time.Time, language string) string {
	now := time.Now()
	diff := now.Sub(t)
	past := diff > 0
	if !past {
		diff = -diff
	}

	zh := strings.ToLower(language) == "zh"

	var result string
	seconds := int(diff.Seconds())
	minutes := int(diff.Minutes())
	hours := int(diff.Hours())
	days := hours / 24
	weeks := days / 7
	months := days / 30
	years := days / 365

	switch {
	case years > 0:
		if zh {
			result = fmt.Sprintf("%d 年", years)
		} else {
			result = fmt.Sprintf("%d year(s)", years)
		}
	case months > 0:
		if zh {
			result = fmt.Sprintf("%d 个月", months)
		} else {
			result = fmt.Sprintf("%d month(s)", months)
		}
	case weeks > 0:
		if zh {
			result = fmt.Sprintf("%d 周", weeks)
		} else {
			result = fmt.Sprintf("%d week(s)", weeks)
		}
	case days > 0:
		if zh {
			result = fmt.Sprintf("%d 天", days)
		} else {
			result = fmt.Sprintf("%d day(s)", days)
		}
	case hours > 0:
		if zh {
			result = fmt.Sprintf("%d 小时", hours)
		} else {
			result = fmt.Sprintf("%d hour(s)", hours)
		}
	case minutes > 0:
		if zh {
			result = fmt.Sprintf("%d 分钟", minutes)
		} else {
			result = fmt.Sprintf("%d minute(s)", minutes)
		}
	case seconds > 0:
		if zh {
			result = fmt.Sprintf("%d 秒", seconds)
		} else {
			result = fmt.Sprintf("%d second(s)", seconds)
		}
	default:
		if zh {
			return "刚刚"
		}
		return "just now"
	}

	if past {
		if zh {
			return result + "前"
		}
		return result + " ago"
	}
	if zh {
		return result + "后"
	}
	return "in " + result
}
