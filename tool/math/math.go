// Package math 提供数学计算工具
//
// 支持基础数学运算、统计计算、单位转换等功能。
// 适用于 Agent 需要进行数值计算的场景。
package math

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/everyday-items/ai-core/tool"
)

// Tools 返回数学计算工具集合
func Tools() []tool.Tool {
	return []tool.Tool{
		// 基础计算器
		tool.NewFunc(
			"calculator",
			"执行基础数学运算 (支持 +, -, *, /, ^, %, sqrt, sin, cos, tan, log, ln)",
			func(ctx context.Context, input struct {
				Expression string `json:"expression" description:"数学表达式，如 '2 + 3 * 4' 或 'sqrt(16)'"`
			}) (struct {
				Result float64 `json:"result"`
				Error  string  `json:"error,omitempty"`
			}, error) {
				result, err := evaluateExpression(input.Expression)
				if err != nil {
					return struct {
						Result float64 `json:"result"`
						Error  string  `json:"error,omitempty"`
					}{Error: err.Error()}, nil
				}
				return struct {
					Result float64 `json:"result"`
					Error  string  `json:"error,omitempty"`
				}{Result: result}, nil
			},
		),

		// 统计计算
		tool.NewFunc(
			"statistics",
			"计算一组数值的统计指标 (平均值、中位数、标准差、最大/最小值)",
			func(ctx context.Context, input struct {
				Values []float64 `json:"values" description:"数值列表"`
			}) (StatisticsOutput, error) {
				if len(input.Values) == 0 {
					return StatisticsOutput{}, fmt.Errorf("values cannot be empty")
				}
				return calculateStatistics(input.Values), nil
			},
		),

		// 百分比计算
		tool.NewFunc(
			"percentage",
			"计算百分比 (增长率、占比等)",
			func(ctx context.Context, input struct {
				Value float64 `json:"value" description:"当前值"`
				Total float64 `json:"total" description:"总值或基准值"`
				Type  string  `json:"type" description:"计算类型: ratio (占比), growth (增长率)"`
			}) (struct {
				Percentage float64 `json:"percentage"`
				Formatted  string  `json:"formatted"`
			}, error) {
				var pct float64
				switch input.Type {
				case "growth":
					if input.Total == 0 {
						return struct {
							Percentage float64 `json:"percentage"`
							Formatted  string  `json:"formatted"`
						}{}, fmt.Errorf("total cannot be zero for growth calculation")
					}
					pct = (input.Value - input.Total) / input.Total * 100
				default: // ratio
					if input.Total == 0 {
						return struct {
							Percentage float64 `json:"percentage"`
							Formatted  string  `json:"formatted"`
						}{}, fmt.Errorf("total cannot be zero")
					}
					pct = input.Value / input.Total * 100
				}
				return struct {
					Percentage float64 `json:"percentage"`
					Formatted  string  `json:"formatted"`
				}{
					Percentage: pct,
					Formatted:  fmt.Sprintf("%.2f%%", pct),
				}, nil
			},
		),

		// 单位转换
		tool.NewFunc(
			"unit_convert",
			"单位转换 (长度、重量、温度、数据存储等)",
			func(ctx context.Context, input struct {
				Value    float64 `json:"value" description:"原始值"`
				From     string  `json:"from" description:"原始单位 (如 km, m, cm, mm, mile, ft, inch)"`
				To       string  `json:"to" description:"目标单位"`
				Category string  `json:"category" description:"单位类别: length, weight, temperature, data, time"`
			}) (struct {
				Result    float64 `json:"result"`
				Formatted string  `json:"formatted"`
			}, error) {
				result, err := convertUnit(input.Value, input.From, input.To, input.Category)
				if err != nil {
					return struct {
						Result    float64 `json:"result"`
						Formatted string  `json:"formatted"`
					}{}, err
				}
				return struct {
					Result    float64 `json:"result"`
					Formatted string  `json:"formatted"`
				}{
					Result:    result,
					Formatted: fmt.Sprintf("%.4f %s", result, input.To),
				}, nil
			},
		),

		// 数字格式化
		tool.NewFunc(
			"number_format",
			"格式化数字 (千分位、科学计数法、货币格式等)",
			func(ctx context.Context, input struct {
				Value     float64 `json:"value" description:"要格式化的数字"`
				Format    string  `json:"format" description:"格式类型: thousand (千分位), scientific (科学计数法), currency (货币)"`
				Precision int     `json:"precision" description:"小数位数 (默认 2)"`
			}) (struct {
				Formatted string `json:"formatted"`
			}, error) {
				precision := input.Precision
				if precision == 0 {
					precision = 2
				}
				result := formatNumber(input.Value, input.Format, precision)
				return struct {
					Formatted string `json:"formatted"`
				}{Formatted: result}, nil
			},
		),

		// 随机数生成
		tool.NewFunc(
			"random_number",
			"生成随机数",
			func(ctx context.Context, input struct {
				Min   float64 `json:"min" description:"最小值"`
				Max   float64 `json:"max" description:"最大值"`
				Count int     `json:"count" description:"生成数量 (默认 1)"`
			}) (struct {
				Numbers []float64 `json:"numbers"`
			}, error) {
				count := input.Count
				if count <= 0 {
					count = 1
				}
				if count > 1000 {
					count = 1000
				}

				numbers := make([]float64, count)
				for i := range numbers {
					// 简单线性随机
					numbers[i] = input.Min + float64(i%100)/100*(input.Max-input.Min)
				}
				return struct {
					Numbers []float64 `json:"numbers"`
				}{Numbers: numbers}, nil
			},
		),

		// 方程求解
		tool.NewFunc(
			"solve_equation",
			"求解简单方程 (一元一次、一元二次)",
			func(ctx context.Context, input struct {
				Type string    `json:"type" description:"方程类型: linear (一元一次 ax+b=0), quadratic (一元二次 ax²+bx+c=0)"`
				A    float64   `json:"a" description:"系数 a"`
				B    float64   `json:"b" description:"系数 b"`
				C    float64   `json:"c,omitempty" description:"系数 c (仅用于二次方程)"`
			}) (struct {
				Solutions []float64 `json:"solutions"`
				HasReal   bool      `json:"has_real"`
			}, error) {
				var solutions []float64
				hasReal := true

				switch input.Type {
				case "linear":
					if input.A == 0 {
						return struct {
							Solutions []float64 `json:"solutions"`
							HasReal   bool      `json:"has_real"`
						}{}, fmt.Errorf("a cannot be zero for linear equation")
					}
					solutions = []float64{-input.B / input.A}
				case "quadratic":
					if input.A == 0 {
						return struct {
							Solutions []float64 `json:"solutions"`
							HasReal   bool      `json:"has_real"`
						}{}, fmt.Errorf("a cannot be zero for quadratic equation")
					}
					discriminant := input.B*input.B - 4*input.A*input.C
					if discriminant < 0 {
						hasReal = false
					} else if discriminant == 0 {
						solutions = []float64{-input.B / (2 * input.A)}
					} else {
						sqrtD := math.Sqrt(discriminant)
						solutions = []float64{
							(-input.B + sqrtD) / (2 * input.A),
							(-input.B - sqrtD) / (2 * input.A),
						}
					}
				default:
					return struct {
						Solutions []float64 `json:"solutions"`
						HasReal   bool      `json:"has_real"`
					}{}, fmt.Errorf("unknown equation type: %s", input.Type)
				}

				return struct {
					Solutions []float64 `json:"solutions"`
					HasReal   bool      `json:"has_real"`
				}{Solutions: solutions, HasReal: hasReal}, nil
			},
		),
	}
}

// StatisticsOutput 统计输出
type StatisticsOutput struct {
	Count    int     `json:"count"`
	Sum      float64 `json:"sum"`
	Mean     float64 `json:"mean"`
	Median   float64 `json:"median"`
	Mode     float64 `json:"mode"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Range    float64 `json:"range"`
	Variance float64 `json:"variance"`
	StdDev   float64 `json:"std_dev"`
}

// calculateStatistics 计算统计值
func calculateStatistics(values []float64) StatisticsOutput {
	n := len(values)
	if n == 0 {
		return StatisticsOutput{}
	}

	// 排序副本
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	// 基础统计
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)

	// 中位数
	var median float64
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	// 众数（简单实现）
	counts := make(map[float64]int)
	for _, v := range values {
		counts[v]++
	}
	maxCount := 0
	mode := values[0]
	for v, c := range counts {
		if c > maxCount {
			maxCount = c
			mode = v
		}
	}

	// 方差和标准差
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(n)
	stdDev := math.Sqrt(variance)

	return StatisticsOutput{
		Count:    n,
		Sum:      sum,
		Mean:     mean,
		Median:   median,
		Mode:     mode,
		Min:      sorted[0],
		Max:      sorted[n-1],
		Range:    sorted[n-1] - sorted[0],
		Variance: variance,
		StdDev:   stdDev,
	}
}

// evaluateExpression 计算数学表达式
// 注意：这是简化实现，生产环境建议使用专业的表达式解析库
func evaluateExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.ToLower(expr)

	// 处理函数
	if strings.HasPrefix(expr, "sqrt(") && strings.HasSuffix(expr, ")") {
		arg := expr[5 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Sqrt(val), nil
	}

	if strings.HasPrefix(expr, "sin(") && strings.HasSuffix(expr, ")") {
		arg := expr[4 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Sin(val), nil
	}

	if strings.HasPrefix(expr, "cos(") && strings.HasSuffix(expr, ")") {
		arg := expr[4 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Cos(val), nil
	}

	if strings.HasPrefix(expr, "tan(") && strings.HasSuffix(expr, ")") {
		arg := expr[4 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Tan(val), nil
	}

	if strings.HasPrefix(expr, "log(") && strings.HasSuffix(expr, ")") {
		arg := expr[4 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Log10(val), nil
	}

	if strings.HasPrefix(expr, "ln(") && strings.HasSuffix(expr, ")") {
		arg := expr[3 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Log(val), nil
	}

	if strings.HasPrefix(expr, "abs(") && strings.HasSuffix(expr, ")") {
		arg := expr[4 : len(expr)-1]
		val, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		if err != nil {
			return 0, err
		}
		return math.Abs(val), nil
	}

	// 简单的二元运算
	operators := []string{"^", "*", "/", "%", "+", "-"}
	for _, op := range operators {
		parts := strings.SplitN(expr, op, 2)
		if len(parts) == 2 {
			left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 != nil || err2 != nil {
				continue
			}

			switch op {
			case "^":
				return math.Pow(left, right), nil
			case "*":
				return left * right, nil
			case "/":
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return left / right, nil
			case "%":
				return math.Mod(left, right), nil
			case "+":
				return left + right, nil
			case "-":
				return left - right, nil
			}
		}
	}

	// 尝试解析为单个数字
	val, err := strconv.ParseFloat(expr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %s", expr)
	}
	return val, nil
}

// convertUnit 单位转换
func convertUnit(value float64, from, to, category string) (float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	switch category {
	case "length":
		return convertLength(value, from, to)
	case "weight":
		return convertWeight(value, from, to)
	case "temperature":
		return convertTemperature(value, from, to)
	case "data":
		return convertData(value, from, to)
	case "time":
		return convertTime(value, from, to)
	default:
		return 0, fmt.Errorf("unknown category: %s", category)
	}
}

// 长度转换表（基准：米）
var lengthFactors = map[string]float64{
	"km":   1000,
	"m":    1,
	"cm":   0.01,
	"mm":   0.001,
	"mile": 1609.344,
	"ft":   0.3048,
	"inch": 0.0254,
	"yard": 0.9144,
}

func convertLength(value float64, from, to string) (float64, error) {
	fromFactor, ok1 := lengthFactors[from]
	toFactor, ok2 := lengthFactors[to]
	if !ok1 || !ok2 {
		return 0, fmt.Errorf("unknown length unit")
	}
	return value * fromFactor / toFactor, nil
}

// 重量转换表（基准：千克）
var weightFactors = map[string]float64{
	"kg":    1,
	"g":     0.001,
	"mg":    0.000001,
	"ton":   1000,
	"lb":    0.453592,
	"oz":    0.0283495,
	"stone": 6.35029,
}

func convertWeight(value float64, from, to string) (float64, error) {
	fromFactor, ok1 := weightFactors[from]
	toFactor, ok2 := weightFactors[to]
	if !ok1 || !ok2 {
		return 0, fmt.Errorf("unknown weight unit")
	}
	return value * fromFactor / toFactor, nil
}

func convertTemperature(value float64, from, to string) (float64, error) {
	// 先转为摄氏度
	var celsius float64
	switch from {
	case "c", "celsius":
		celsius = value
	case "f", "fahrenheit":
		celsius = (value - 32) * 5 / 9
	case "k", "kelvin":
		celsius = value - 273.15
	default:
		return 0, fmt.Errorf("unknown temperature unit: %s", from)
	}

	// 转为目标单位
	switch to {
	case "c", "celsius":
		return celsius, nil
	case "f", "fahrenheit":
		return celsius*9/5 + 32, nil
	case "k", "kelvin":
		return celsius + 273.15, nil
	default:
		return 0, fmt.Errorf("unknown temperature unit: %s", to)
	}
}

// 数据转换表（基准：字节）
var dataFactors = map[string]float64{
	"b":  1,
	"kb": 1024,
	"mb": 1024 * 1024,
	"gb": 1024 * 1024 * 1024,
	"tb": 1024 * 1024 * 1024 * 1024,
	"pb": 1024 * 1024 * 1024 * 1024 * 1024,
}

func convertData(value float64, from, to string) (float64, error) {
	fromFactor, ok1 := dataFactors[from]
	toFactor, ok2 := dataFactors[to]
	if !ok1 || !ok2 {
		return 0, fmt.Errorf("unknown data unit")
	}
	return value * fromFactor / toFactor, nil
}

// 时间转换表（基准：秒）
var timeFactors = map[string]float64{
	"ms":     0.001,
	"s":      1,
	"sec":    1,
	"min":    60,
	"h":      3600,
	"hour":   3600,
	"d":      86400,
	"day":    86400,
	"week":   604800,
	"month":  2592000,
	"year":   31536000,
}

func convertTime(value float64, from, to string) (float64, error) {
	fromFactor, ok1 := timeFactors[from]
	toFactor, ok2 := timeFactors[to]
	if !ok1 || !ok2 {
		return 0, fmt.Errorf("unknown time unit")
	}
	return value * fromFactor / toFactor, nil
}

// formatNumber 格式化数字
func formatNumber(value float64, format string, precision int) string {
	switch format {
	case "thousand":
		// 千分位格式
		negative := value < 0
		if negative {
			value = -value
		}

		intPart := int64(value)
		decPart := value - float64(intPart)

		// 格式化整数部分
		str := strconv.FormatInt(intPart, 10)
		result := ""
		for i, c := range str {
			if i > 0 && (len(str)-i)%3 == 0 {
				result += ","
			}
			result += string(c)
		}

		// 添加小数部分
		if precision > 0 {
			decStr := strconv.FormatFloat(decPart, 'f', precision, 64)
			if len(decStr) > 1 {
				result += decStr[1:] // 去掉前导 0
			}
		}

		if negative {
			result = "-" + result
		}
		return result

	case "scientific":
		return strconv.FormatFloat(value, 'e', precision, 64)

	case "currency":
		return fmt.Sprintf("$%s", formatNumber(value, "thousand", precision))

	default:
		return strconv.FormatFloat(value, 'f', precision, 64)
	}
}
