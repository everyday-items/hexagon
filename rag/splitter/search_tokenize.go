// search_tokenize.go 搜索查询分词器
//
// 为全文检索（FTS5 / LIKE 降级）提供 CJK 友好的查询分词。
//
// 与 SimpleTokenizer.tokenize() 的区别：
//   - SimpleTokenizer: 面向 token 计数（CJK 每字一个 token），用于分块估算
//   - SearchTokenize:  面向搜索匹配（CJK bigram + 原词保留），用于构建搜索查询
//
// CJK bigram 策略参考：
//   - Elasticsearch CJK bigram token filter
//   - Lucene CJKBigramFilter
//   - SQLite FTS5 + unicode61 tokenizer 的中文检索最佳实践
//
// 示例：
//
//	SearchTokenize("杭帮菜的正确吃法")
//	→ ["杭帮", "帮菜", "菜的", "的正", "正确", "确吃", "吃法", "杭帮菜的正确吃法"]
//
//	SearchTokenize("hello world 你好世界")
//	→ ["hello", "world", "你好", "好世", "世界", "你好世界"]
package splitter

import (
	"strings"
	"unicode"
)

// SearchTokenize 将搜索查询文本分词为适合全文检索的 token 列表
//
// 策略：
//  1. 按空格和标点分割为词组
//  2. 纯 CJK 词组做 bigram 切分（保留原词用于精确匹配）
//  3. 非 CJK 词组保留原样
//  4. 过滤过短的 token（< 2 字符）
//  5. 去重
func SearchTokenize(text string) []string {
	// 标点替换为空格
	text = searchPunctReplacer.Replace(text)

	words := strings.Fields(text)
	seen := make(map[string]bool, len(words)*2)
	result := make([]string, 0, len(words)*2)

	addUnique := func(w string) {
		if !seen[w] {
			seen[w] = true
			result = append(result, w)
		}
	}

	for _, w := range words {
		w = strings.TrimSpace(w)
		if len(w) < 2 { // 字节长度 < 2，过滤单个 ASCII 字符
			continue
		}

		runes := []rune(w)

		// 检查是否包含 CJK 字符
		hasCJK := false
		for _, r := range runes {
			if IsCJK(r) {
				hasCJK = true
				break
			}
		}

		if hasCJK && len(runes) > 2 {
			// CJK bigram：相邻两字组合
			for i := 0; i < len(runes)-1; i++ {
				addUnique(string(runes[i : i+2]))
			}
			// 保留原始词用于精确匹配
			addUnique(w)
		} else {
			addUnique(w)
		}
	}

	return result
}

// IsCJK 判断 rune 是否为 CJK 统一表意文字（中日韩）
func IsCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r)
}

// searchPunctReplacer 搜索分词用标点替换器
var searchPunctReplacer = strings.NewReplacer(
	"，", " ", "。", " ", "？", " ", "！", " ",
	",", " ", ".", " ", "?", " ", "!", " ",
	"、", " ", "：", " ", "；", " ", "·", " ",
	"\"", " ", "'", " ", "(", " ", ")", " ",
	"（", " ", "）", " ", "【", " ", "】", " ",
	"《", " ", "》", " ", "\u201c", " ", "\u201d", " ",
)
