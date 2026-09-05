package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokenizeCJKRun(t *testing.T) {
	// 长度 ≥2 的 CJK 连续段：先产全部单字，再产相邻二元组。
	got := tokenize("返回高亮片段")
	want := []string{"返", "回", "高", "亮", "片", "段", "返回", "回高", "高亮", "亮片", "片段"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize(CJK 连续段) = %v，期望 %v", got, want)
	}

	// 单字段只产单字，没有二元组。
	if got, want := tokenize("的"), []string{"的"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize(单字段) = %v，期望 %v", got, want)
	}
}

func TestTokenizeMixedAndWords(t *testing.T) {
	// 中英混排：非 CJK 段保持整词，CJK 段单字 + 二元组。
	got := tokenize("SQLite 数据库")
	want := []string{"SQLite", "数", "据", "库", "数据", "据库"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize(中英混排) = %v，期望 %v", got, want)
	}

	// 标点与空白仅作分隔。
	got = tokenize("full-text search!")
	want = []string{"full", "text", "search"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize(含标点) = %v，期望 %v", got, want)
	}
}

func TestMatchQuery(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"2 字中文词转二元组短语", "高亮", `"高亮"`},
		{"中文整词转二元组短语", "快速开始", `"快速 速开 开始"`},
		{"单字中文词", "数", `"数"`},
		{"多词保持 AND", "高亮 片段", `"高亮" AND "片段"`},
		{"英文整词", "sqlite", `"sqlite"`},
		{"中英混排多词", "SQLite 数据库", `"SQLite" AND "数据 据库"`},
		{"中英混排单词", "SQLite数据库", `"SQLite" AND "数据 据库"`},
		{"空查询", "", ""},
		{"纯空白查询", "  \t ", ""},
		{"纯标点查询", "（）", ""},
		{"含引号括号", `full "text" (search)`, `"full" AND "text" AND "search"`},
	}
	for _, c := range cases {
		if got := matchQuery(c.query); got != c.want {
			t.Errorf("%s: matchQuery(%q) = %q，期望 %q", c.name, c.query, got, c.want)
		}
	}
}

// stripSnippet 去掉标记与省略号，用于校验片段是原文的连续子串。
func stripSnippet(s string) string {
	return strings.NewReplacer(markOpen, "", markClose, "", ellipsis, "").Replace(s)
}

func TestNormalizeSnippet(t *testing.T) {
	cases := []struct {
		name     string
		original string // 被索引的原文
		snippet  string // FTS5 在分词文本上产出的片段
		want     string
	}{
		{
			"合并相邻 mark 并去除分词空格",
			"返回高亮片段",
			"返 回 高 亮 片 段 返回 回高 <mark>高亮</mark> 亮片 片段",
			"返回<mark>高亮</mark>片段",
		},
		{
			"多个相邻 mark 合并为连续完整包裹",
			"返回高亮片段",
			"回高 <mark>高亮</mark> <mark>亮片</mark> <mark>片段</mark>",
			"回<mark>高亮片段</mark>",
		},
		{
			"首尾省略号保留",
			"返回高亮片段",
			"…回高 <mark>高亮</mark> 亮片…",
			"…回<mark>高亮</mark>片…",
		},
		{
			"英文词边界保留空格",
			"SQLite 数据库",
			"<mark>SQLite</mark> 数 据 库 数据 据库",
			"<mark>SQLite</mark> 数据库",
		},
		{
			"命中词后的重叠二元组保留尾部",
			"返回高亮",
			"<mark>回高</mark> 高亮",
			"<mark>回高</mark>亮",
		},
		{
			"无标记片段仅去空格",
			"返回高亮",
			"返回 回高 高亮",
			"返回高亮",
		},
		{
			"空片段",
			"",
			"",
			"",
		},
	}
	for _, c := range cases {
		got := normalizeSnippet(c.snippet)
		if got != c.want {
			t.Errorf("%s: normalizeSnippet(%q) = %q，期望 %q", c.name, c.snippet, got, c.want)
			continue
		}
		if stripped := stripSnippet(got); !strings.Contains(c.original, stripped) {
			t.Errorf("%s: 去掉标记与省略号后 %q 不是原文 %q 的连续子串", c.name, stripped, c.original)
		}
	}
}
