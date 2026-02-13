// Package loader 提供 RAG 系统的文档加载器
//
// 本文件实现 PDF 编码解码功能，包括：
//   - WinAnsiEncoding 字符映射
//   - PDF 字符串解码（括号字符串 + hex 字符串）
//   - PDF 日期格式解析

package loader

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// winAnsiEncoding WinAnsiEncoding 字符映射表
//
// 覆盖 128-255 范围中与 Latin-1 不同的码位。
// PDF 默认编码为 WinAnsiEncoding，此映射用于将 PDF 字节正确转换为 Unicode。
var winAnsiEncoding = map[byte]rune{
	0x80: '\u20AC', // €
	0x82: '\u201A', // ‚
	0x83: '\u0192', // ƒ
	0x84: '\u201E', // „
	0x85: '\u2026', // …
	0x86: '\u2020', // †
	0x87: '\u2021', // ‡
	0x88: '\u02C6', // ˆ
	0x89: '\u2030', // ‰
	0x8A: '\u0160', // Š
	0x8B: '\u2039', // ‹
	0x8C: '\u0152', // Œ
	0x8E: '\u017D', // Ž
	0x91: '\u2018', // '
	0x92: '\u2019', // '
	0x93: '\u201C', // "
	0x94: '\u201D', // "
	0x95: '\u2022', // •
	0x96: '\u2013', // –
	0x97: '\u2014', // —
	0x98: '\u02DC', // ˜
	0x99: '\u2122', // ™
	0x9A: '\u0161', // š
	0x9B: '\u203A', // ›
	0x9C: '\u0153', // œ
	0x9E: '\u017E', // ž
	0x9F: '\u0178', // Ÿ
}

// decodeWinAnsi 将 WinAnsi 编码字节序列解码为 UTF-8 字符串
//
// 对于 0x80-0x9F 范围的字节，使用 winAnsiEncoding 映射表转换；
// 其他字节直接作为 Latin-1 处理（与 Unicode 码位相同）。
func decodeWinAnsi(data []byte) string {
	var sb strings.Builder
	sb.Grow(len(data))
	for _, b := range data {
		if r, ok := winAnsiEncoding[b]; ok {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(rune(b))
		}
	}
	return sb.String()
}

// decodePDFString 统一解码 PDF 字符串
//
// PDF 字符串有两种编码形式：
//   - 括号字符串: (Hello World) — 支持反斜杠转义和八进制编码
//   - hex 字符串: <48656C6C6F> — 十六进制编码
//
// 本函数自动识别格式并解码，返回 UTF-8 字符串。
func decodePDFString(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}

	// hex 字符串: <xxxx>
	if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
		return decodeHexString(raw[1 : len(raw)-1])
	}

	// 括号字符串: (text)
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		return decodeLiteralString(raw[1 : len(raw)-1])
	}

	// 无包裹，直接返回
	return raw
}

// decodeHexString 解码 PDF hex 字符串
//
// hex 字符串由成对的十六进制数字组成，如 <48656C6C6F> 表示 "Hello"。
// 如果长度为奇数，末尾补 0。空白字符会被忽略。
func decodeHexString(hexStr string) string {
	// 移除所有空白
	hexStr = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, hexStr)

	// 奇数长度补 0
	if len(hexStr)%2 != 0 {
		hexStr += "0"
	}

	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return hexStr // 解码失败返回原始内容
	}

	return decodeWinAnsi(data)
}

// decodeLiteralString 解码 PDF 括号字符串中的转义序列
//
// 支持以下转义：
//   - \n, \r, \t, \b, \f — 标准控制字符
//   - \( , \) , \\ — 字面字符
//   - \ddd — 八进制字符编码（1-3 位）
//   - 行尾反斜杠 — 续行（忽略换行）
func decodeLiteralString(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '(':
				sb.WriteByte('(')
			case ')':
				sb.WriteByte(')')
			case '\\':
				sb.WriteByte('\\')
			case '\r':
				// 续行：\CR 或 \CRLF
				if i+1 < len(s) && s[i+1] == '\n' {
					i++
				}
			case '\n':
				// 续行：\LF
			default:
				// 八进制编码 \ddd
				if s[i] >= '0' && s[i] <= '7' {
					oct := string(s[i])
					for j := 1; j < 3 && i+j < len(s) && s[i+j] >= '0' && s[i+j] <= '7'; j++ {
						oct += string(s[i+j])
					}
					if val, err := strconv.ParseUint(oct, 8, 8); err == nil {
						sb.WriteByte(byte(val))
						i += len(oct) - 1
					} else {
						sb.WriteByte(s[i])
					}
				} else {
					sb.WriteByte(s[i])
				}
			}
		} else {
			sb.WriteByte(s[i])
		}
		i++
	}
	return decodeWinAnsi([]byte(sb.String()))
}

// decodePDFDate 解析 PDF 日期格式
//
// PDF 日期格式为: D:YYYYMMDDHHmmSSOHH'mm'
// 其中:
//   - D: 前缀标识
//   - YYYY 年份（必需）
//   - MM 月份（01-12，默认 01）
//   - DD 日期（01-31，默认 01）
//   - HH 小时（00-23，默认 00）
//   - mm 分钟（00-59，默认 00）
//   - SS 秒（00-59，默认 00）
//   - O 时区方向（+、-、Z）
//   - HH'mm' 时区偏移
//
// 示例: D:20230615120000+08'00' 表示 2023-06-15 12:00:00 UTC+8
func decodePDFDate(s string) time.Time {
	s = strings.TrimSpace(s)

	// 先移除括号（如果有），再移除 D: 前缀
	// 因为 PDF 中日期可能是 (D:20230615...) 格式
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimPrefix(s, "D:")

	if len(s) < 4 {
		return time.Time{}
	}

	// 解析各部分，使用默认值
	year := parseInt(s[0:4], 0)
	month := 1
	day := 1
	hour := 0
	minute := 0
	second := 0
	var loc *time.Location = time.UTC

	if len(s) >= 6 {
		month = parseInt(s[4:6], 1)
	}
	if len(s) >= 8 {
		day = parseInt(s[6:8], 1)
	}
	if len(s) >= 10 {
		hour = parseInt(s[8:10], 0)
	}
	if len(s) >= 12 {
		minute = parseInt(s[10:12], 0)
	}
	if len(s) >= 14 {
		second = parseInt(s[12:14], 0)
	}

	// 解析时区
	if len(s) >= 15 {
		tzStr := s[14:]
		loc = parsePDFTimezone(tzStr)
	}

	// 基本范围校验
	if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}
	}

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, loc)
}

// parsePDFTimezone 解析 PDF 时区偏移
//
// 格式: Z 或 +HH'mm' 或 -HH'mm'
func parsePDFTimezone(s string) *time.Location {
	if len(s) == 0 || s[0] == 'Z' {
		return time.UTC
	}

	sign := 1
	if s[0] == '-' {
		sign = -1
	} else if s[0] != '+' {
		return time.UTC
	}

	s = s[1:]
	// 移除引号
	s = strings.ReplaceAll(s, "'", "")

	tzHour := 0
	tzMin := 0
	if len(s) >= 2 {
		tzHour = parseInt(s[0:2], 0)
	}
	if len(s) >= 4 {
		tzMin = parseInt(s[2:4], 0)
	}

	offset := sign * (tzHour*3600 + tzMin*60)
	return time.FixedZone(fmt.Sprintf("UTC%+d", sign*tzHour), offset)
}

// parseInt 安全解析整数，失败返回默认值
func parseInt(s string, defaultVal int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}
