package math

import (
	"context"
	"testing"

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

func TestCalculator(t *testing.T) {
	tools := Tools()
	var calculator tool.Tool
	for _, tl := range tools {
		if tl.Name() == "calculator" {
			calculator = tl
			break
		}
	}
	if calculator == nil {
		t.Fatal("calculator tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name       string
		expression string
		wantErr    bool
		checkValue bool
		wantValue  float64
	}{
		{
			name:       "基础加法",
			expression: "2 + 3",
			checkValue: true,
			wantValue:  5.0,
		},
		{
			name:       "基础减法",
			expression: "10 - 4",
			checkValue: true,
			wantValue:  6.0,
		},
		{
			name:       "基础乘法",
			expression: "3 * 4",
			checkValue: true,
			wantValue:  12.0,
		},
		{
			name:       "基础除法",
			expression: "20 / 5",
			checkValue: true,
			wantValue:  4.0,
		},
		{
			name:       "平方根",
			expression: "sqrt(16)",
			checkValue: true,
			wantValue:  4.0,
		},
		{
			name:       "幂运算",
			expression: "2 ^ 3",
			checkValue: true,
			wantValue:  8.0,
		},
		{
			name:       "除以零",
			expression: "10 / 0",
			wantErr:    false, // 返回的是 Error 字段，不是 error
		},
		{
			name:       "无效表达式",
			expression: "invalid",
			wantErr:    false, // 返回的是 Error 字段，不是 error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"expression": tt.expression,
			}

			// 验证参数
			if err := calculator.Validate(args); err != nil {
				if !tt.wantErr {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}

			result, err := calculator.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkValue {
				// 检查结果
				if !result.Success {
					t.Errorf("result should be successful, got error: %s", result.Error)
				}
				// 结果是结构体，有 Result 和 Error 字段
				if resultMap, ok := result.Output.(map[string]any); ok {
					if errStr, ok := resultMap["error"].(string); ok && errStr != "" {
						t.Errorf("unexpected error in result: %s", errStr)
					}
					if val, ok := resultMap["result"].(float64); ok && tt.checkValue {
						if val != tt.wantValue {
							t.Errorf("got result = %v, want %v", val, tt.wantValue)
						}
					}
				}
			}
		})
	}
}

func TestStatistics(t *testing.T) {
	tools := Tools()
	var statistics tool.Tool
	for _, tl := range tools {
		if tl.Name() == "statistics" {
			statistics = tl
			break
		}
	}
	if statistics == nil {
		t.Fatal("statistics tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		values  []float64
		wantErr bool
		check   func(*testing.T, any)
	}{
		{
			name:   "正常统计",
			values: []float64{1, 2, 3, 4, 5},
			check: func(t *testing.T, content any) {
				stats, ok := content.(StatisticsOutput)
				if !ok {
					t.Fatal("result is not StatisticsOutput")
				}
				if stats.Count != 5 {
					t.Errorf("Count = %d, want 5", stats.Count)
				}
				if stats.Mean != 3.0 {
					t.Errorf("Mean = %f, want 3.0", stats.Mean)
				}
				if stats.Median != 3.0 {
					t.Errorf("Median = %f, want 3.0", stats.Median)
				}
				if stats.Min != 1.0 {
					t.Errorf("Min = %f, want 1.0", stats.Min)
				}
				if stats.Max != 5.0 {
					t.Errorf("Max = %f, want 5.0", stats.Max)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"values": tt.values,
			}

			result, err := statistics.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, result.Output)
			}
		})
	}
}

func TestPercentage(t *testing.T) {
	tools := Tools()
	var percentage tool.Tool
	for _, tl := range tools {
		if tl.Name() == "percentage" {
			percentage = tl
			break
		}
	}
	if percentage == nil {
		t.Fatal("percentage tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		value   float64
		total   float64
		typ     string
		wantErr bool
		wantPct float64 // 期望的百分比
	}{
		{
			name:    "占比计算",
			value:   25,
			total:   100,
			typ:     "ratio",
			wantPct: 25.0,
		},
		{
			name:    "增长率计算",
			value:   150,
			total:   100,
			typ:     "growth",
			wantPct: 50.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"value": tt.value,
				"total": tt.total,
				"type":  tt.typ,
			}

			result, err := percentage.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if pct, ok := resultMap["percentage"].(float64); ok {
						if pct != tt.wantPct {
							t.Errorf("percentage = %v, want %v", pct, tt.wantPct)
						}
					}
				}
			}
		})
	}
}

func TestUnitConvert(t *testing.T) {
	tools := Tools()
	var unitConvert tool.Tool
	for _, tl := range tools {
		if tl.Name() == "unit_convert" {
			unitConvert = tl
			break
		}
	}
	if unitConvert == nil {
		t.Fatal("unit_convert tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name       string
		value      float64
		from       string
		to         string
		category   string
		wantErr    bool
		checkValue bool
		wantValue  float64
	}{
		{
			name:       "千米转米",
			value:      1,
			from:       "km",
			to:         "m",
			category:   "length",
			checkValue: true,
			wantValue:  1000,
		},
		{
			name:       "摄氏度转华氏度",
			value:      0,
			from:       "c",
			to:         "f",
			category:   "temperature",
			checkValue: true,
			wantValue:  32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"value":    tt.value,
				"from":     tt.from,
				"to":       tt.to,
				"category": tt.category,
			}

			result, err := unitConvert.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkValue {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if val, ok := resultMap["result"].(float64); ok {
						if val != tt.wantValue {
							t.Errorf("result = %v, want %v", val, tt.wantValue)
						}
					}
				}
			}
		})
	}
}

func TestNumberFormat(t *testing.T) {
	tools := Tools()
	var numberFormat tool.Tool
	for _, tl := range tools {
		if tl.Name() == "number_format" {
			numberFormat = tl
			break
		}
	}
	if numberFormat == nil {
		t.Fatal("number_format tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		value     float64
		format    string
		precision int
	}{
		{
			name:      "千分位格式",
			value:     1234567.89,
			format:    "thousand",
			precision: 2,
		},
		{
			name:      "科学计数法",
			value:     1234567.89,
			format:    "scientific",
			precision: 2,
		},
		{
			name:      "货币格式",
			value:     1234.56,
			format:    "currency",
			precision: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"value":     tt.value,
				"format":    tt.format,
				"precision": tt.precision,
			}

			result, err := numberFormat.Execute(ctx, args)
			if err != nil {
				t.Errorf("Execute() error = %v", err)
				return
			}

			if resultMap, ok := result.Output.(map[string]any); ok {
				if formatted, ok := resultMap["formatted"].(string); ok {
					if formatted == "" {
						t.Error("formatted result is empty")
					}
				}
			}
		})
	}
}

func TestSolveEquation(t *testing.T) {
	tools := Tools()
	var solveEquation tool.Tool
	for _, tl := range tools {
		if tl.Name() == "solve_equation" {
			solveEquation = tl
			break
		}
	}
	if solveEquation == nil {
		t.Fatal("solve_equation tool not found")
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		typ     string
		a       float64
		b       float64
		c       float64
		wantErr bool
	}{
		{
			name: "一元一次方程",
			typ:  "linear",
			a:    2,
			b:    -4,
		},
		{
			name: "一元二次方程(有解)",
			typ:  "quadratic",
			a:    1,
			b:    -3,
			c:    2,
		},
		{
			name: "一元二次方程(无实数解)",
			typ:  "quadratic",
			a:    1,
			b:    0,
			c:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"type": tt.typ,
				"a":    tt.a,
				"b":    tt.b,
				"c":    tt.c,
			}

			result, err := solveEquation.Execute(ctx, args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resultMap, ok := result.Output.(map[string]any); ok {
					if _, ok := resultMap["solutions"]; !ok {
						t.Error("result should contain solutions field")
					}
					if _, ok := resultMap["has_real"]; !ok {
						t.Error("result should contain has_real field")
					}
				}
			}
		})
	}
}

func TestRandomNumber(t *testing.T) {
	tools := Tools()
	var randomNumber tool.Tool
	for _, tl := range tools {
		if tl.Name() == "random_number" {
			randomNumber = tl
			break
		}
	}
	if randomNumber == nil {
		t.Fatal("random_number tool not found")
	}

	ctx := context.Background()

	args := map[string]any{
		"min":   0.0,
		"max":   100.0,
		"count": 10,
	}

	result, err := randomNumber.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultMap, ok := result.Output.(map[string]any); ok {
		if numbers, ok := resultMap["numbers"].([]float64); ok {
			if len(numbers) != 10 {
				t.Errorf("got %d numbers, want 10", len(numbers))
			}
		}
	}
}
