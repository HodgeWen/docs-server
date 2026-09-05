package search

import (
	"strings"
	"unicode"
)

// FTS5 snippet() 使用的高亮标记与省略号，与 search.go 的 snippet 调用保持一致。
const (
	markOpen  = "<mark>"
	markClose = "</mark>"
	ellipsis  = "…"
)

// isCJK 判断字符是否属于 CJK 表意文字（汉字、日文假名、韩文音节）。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// scanRuns 从左到右扫描文本，把 CJK 连续段与字母/数字整词段分别回调；
// 标点、空白等其余字符仅作分隔，不产生任何段。
func scanRuns(text string, onCJK func(runes []rune), onWord func(runes []rune)) {
	var cjk, word []rune
	flushCJK := func() {
		if len(cjk) > 0 {
			onCJK(cjk)
			cjk = cjk[:0]
		}
	}
	flushWord := func() {
		if len(word) > 0 {
			onWord(word)
			word = word[:0]
		}
	}
	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, r)
		default:
			flushCJK()
			flushWord()
		}
	}
	flushCJK()
	flushWord()
}

// bigrams 产出连续段的相邻二元组；段长 1 时退化为单字。
func bigrams(rs []rune) []string {
	if len(rs) == 1 {
		return []string{string(rs[0])}
	}
	tokens := make([]string, 0, len(rs)-1)
	for i := 0; i+1 < len(rs); i++ {
		tokens = append(tokens, string(rs[i:i+2]))
	}
	return tokens
}

// tokenize 产出供 FTS5 索引的 token 序列：CJK 连续段先产全部单字、再产相邻
// 二元组（二元组彼此相邻，保证二元组短语查询可命中）；非 CJK 段保持整词。
// 调用方以空格连接后写入索引，unicode61 会把每个 token 视为独立词元。
func tokenize(text string) []string {
	var tokens []string
	scanRuns(text,
		func(rs []rune) {
			if len(rs) > 1 {
				// 单字段的单字已由 bigrams 产出，避免重复。
				for _, r := range rs {
					tokens = append(tokens, string(r))
				}
			}
			tokens = append(tokens, bigrams(rs)...)
		},
		func(rs []rune) { tokens = append(tokens, string(rs)) })
	return tokens
}

// matchQuery 把用户查询转成安全的 FTS5 MATCH 表达式：按空白分词，多词以 AND
// 连接；每个词内按 CJK 段 / 整词段拆成加引号的短语（CJK 段用相邻二元组短语，
// 与索引 token 对齐），引号按 FTS5 规则双写转义。空查询或纯标点查询返回空串。
func matchQuery(query string) string {
	var phrases []string
	for _, term := range strings.Fields(query) {
		scanRuns(term,
			func(rs []rune) { phrases = append(phrases, quote(strings.Join(bigrams(rs), " "))) },
			func(rs []rune) { phrases = append(phrases, quote(string(rs))) })
	}
	return strings.Join(phrases, " AND ")
}

// quote 把一段 token 序列包成 FTS5 短语，内部引号双写转义。
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// normalizeSnippet 后处理 FTS5 高亮片段：去掉分词引入的空格（丢弃冗余单字
// token、相邻二元组按重叠字符合并、CJK token 之间直接相连，词边界保留空格），
// 并把相邻 <mark> 合并成对查询词的连续完整包裹。去掉标记与省略号后，
// 结果为原文的连续子串。
func normalizeSnippet(snippet string) string {
	// 省略号统一成独立 token，便于作为分段边界处理。
	snippet = strings.ReplaceAll(snippet, ellipsis, " "+ellipsis+" ")

	var pieces []snippetPiece
	appendRunes := func(rs []rune, marked bool) {
		if len(rs) == 0 {
			return
		}
		if n := len(pieces); n > 0 && pieces[n-1].marked == marked {
			pieces[n-1].text += string(rs)
		} else {
			pieces = append(pieces, snippetPiece{string(rs), marked})
		}
	}
	// dropLastRune 去掉已产出文本的最后一个字符：重叠字符优先留给后到的 <mark> 完整包裹。
	dropLastRune := func() {
		for len(pieces) > 0 {
			p := &pieces[len(pieces)-1]
			rs := []rune(p.text)
			if len(rs) == 0 {
				pieces = pieces[:len(pieces)-1]
				continue
			}
			p.text = string(rs[:len(rs)-1])
			if p.text == "" {
				pieces = pieces[:len(pieces)-1]
			}
			return
		}
	}

	var prev []rune // 上一 token 的原始字符；nil 表示处于片段开头或省略号之后
	prevMarked := false
	for _, tok := range strings.Fields(snippet) {
		if tok == ellipsis {
			appendRunes([]rune(ellipsis), false)
			prev = nil
			prevMarked = false
			continue
		}
		marked := strings.HasPrefix(tok, markOpen)
		raw := []rune(strings.NewReplacer(markOpen, "", markClose, "").Replace(tok))
		if len(raw) == 0 {
			continue
		}
		// 丢弃未命中的单字 token：它们是索引里二元组的冗余副本，保留会破坏原文连续性。
		if !marked && len(raw) == 1 && isCJK(raw[0]) {
			continue
		}
		switch {
		case prev == nil:
			appendRunes(raw, marked)
		case isCJK(prev[len(prev)-1]) && isCJK(raw[0]):
			if len(raw) >= 2 && prev[len(prev)-1] == raw[0] {
				// 相邻二元组重叠（如 回高 + 高亮）：共享字只保留一份。
				if marked && !prevMarked {
					dropLastRune()
					appendRunes(raw, true)
				} else {
					appendRunes(raw[1:], marked)
				}
			} else {
				// 同一段落内的 CJK token 之间不存在原始空格。
				appendRunes(raw, marked)
			}
		default:
			// 词与词、词与 CJK 的边界保留空格（与原文有无空格无法区分，取可读形式）。
			appendRunes([]rune(" "), false)
			appendRunes(raw, marked)
		}
		prev = raw
		prevMarked = marked
	}

	var b strings.Builder
	for _, p := range pieces {
		if p.marked {
			b.WriteString(markOpen + p.text + markClose)
		} else {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// snippetPiece 是片段里一段连续文本，marked 表示是否被 <mark> 包裹；
// 相邻同 marked 的段在 appendRunes 里已合并，输出时天然形成连续包裹。
type snippetPiece struct {
	text   string
	marked bool
}
