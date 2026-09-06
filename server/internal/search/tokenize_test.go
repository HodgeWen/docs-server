package search

import (
	"errors"
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

func TestSplitQueryTerms(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"自然语言整句停用词过滤", "如何在表格中进行行内编辑", []string{"表格", "行内编辑"}},
		{"虚词的与方位词过滤", "表格中的行内编辑", []string{"表格", "行内编辑"}},
		{"介词关于与用法", "关于表格编辑器的用法", []string{"表格编辑器", "用法"}},
		{"普通空格切分词", "表格 行内编辑", []string{"表格", "行内编辑"}},
		{"英文与数字混排", "UButton size", []string{"UButton", "size"}},
		{"全停用词安全回退", "如何", []string{"如何"}},
		{"全助词安全回退", "的", []string{"的"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitQueryTerms(c.query)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitQueryTerms(%q) = %v，期望 %v", c.query, got, c.want)
			}
		})
	}
}

func TestMatchQueryOR(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"2字中文词", "高亮", `"高亮"`},
		{"多词 OR 连接且长词分解二元组", "表格 行内编辑", `"表格" OR ("行内 内编 编辑" OR "行内" OR "内编" OR "编辑")`},
		{"自然语言句子经过滤后转 OR", "如何在表格中进行行内编辑", `"表格" OR ("行内 内编 编辑" OR "行内" OR "内编" OR "编辑")`},
		{"空查询", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchQueryOR(c.query)
			if got != c.want {
				t.Errorf("matchQueryOR(%q) = %q，期望 %q", c.query, got, c.want)
			}
		})
	}
}

func TestSectionChunking(t *testing.T) {
	content := `# UTable 表格

表格组件用于展示数据。

## Props

| 属性 | 说明 |
| --- | --- |
| data | 数据源 |

## Events

| 事件 | 说明 |
| --- | --- |
| change | 改变 |

## Methods

### reload
重新加载数据。
`

	t.Run("提取全部二级标题", func(t *testing.T) {
		sections := ExtractSections(content)
		want := []string{"Props", "Events", "Methods"}
		if !reflect.DeepEqual(sections, want) {
			t.Errorf("ExtractSections = %v，期望 %v", sections, want)
		}
	})

	t.Run("提取指定章节", func(t *testing.T) {
		sec, sections, err := ExtractSection(content, "Props")
		if err != nil {
			t.Fatalf("ExtractSection(Props): %v", err)
		}
		if len(sections) != 3 {
			t.Errorf("可用章节应为 3 个，实际 %d", len(sections))
		}
		wantSec := "## Props\n\n| 属性 | 说明 |\n| --- | --- |\n| data | 数据源 |"
		if strings.TrimSpace(sec) != wantSec {
			t.Errorf("提取 Props 章节不符:\nGot: %q\nWant: %q", sec, wantSec)
		}
	})

	t.Run("提取末尾章节", func(t *testing.T) {
		sec, _, err := ExtractSection(content, "Methods")
		if err != nil {
			t.Fatalf("ExtractSection(Methods): %v", err)
		}
		wantSec := "## Methods\n\n### reload\n重新加载数据。"
		if strings.TrimSpace(sec) != wantSec {
			t.Errorf("提取 Methods 章节不符:\nGot: %q\nWant: %q", sec, wantSec)
		}
	})

	t.Run("忽略大小写与##前缀", func(t *testing.T) {
		sec, _, err := ExtractSection(content, "## props")
		if err != nil {
			t.Fatalf("ExtractSection(## props): %v", err)
		}
		if !strings.HasPrefix(sec, "## Props") {
			t.Errorf("提取结果未以 ## Props 开头: %q", sec)
		}
	})

	t.Run("章节不存在返回 ErrSectionNotFound 并携带可用章节", func(t *testing.T) {
		_, _, err := ExtractSection(content, "Slots")
		if err == nil {
			t.Fatal("不存在章节应返回错误")
		}
		if !errors.Is(err, ErrSectionNotFound) {
			t.Errorf("应包含 ErrSectionNotFound，实际 %v", err)
		}
		if !strings.Contains(err.Error(), "Props, Events, Methods") {
			t.Errorf("错误信息未包含可用章节列表: %v", err)
		}
	})

	t.Run("空章节名称返回完整内容", func(t *testing.T) {
		sec, _, err := ExtractSection(content, "")
		if err != nil {
			t.Fatalf("ExtractSection empty: %v", err)
		}
		if sec != content {
			t.Errorf("空章节名称应返回完整内容")
		}
	})

	t.Run("忽略代码块内部的 ## 注释且保留完整代码块", func(t *testing.T) {
		docWithCode := `# Title

## Usage

示例代码：
` + "```bash\n## 这里是 bash 注释\ncurl http://example.com\n```\n" + `
更多说明。

## Other

其他内容。
`
		sections := ExtractSections(docWithCode)
		want := []string{"Usage", "Other"}
		if !reflect.DeepEqual(sections, want) {
			t.Errorf("ExtractSections 应该忽略代码块中的 ## 注释，实际 %v", sections)
		}

		sec, _, err := ExtractSection(docWithCode, "Usage")
		if err != nil {
			t.Fatalf("ExtractSection(Usage): %v", err)
		}
		if !strings.Contains(sec, "## 这里是 bash 注释") || !strings.Contains(sec, "更多说明。") {
			t.Errorf("代码块不应被提前截断，实际内容:\n%s", sec)
		}
		if strings.Contains(sec, "## Other") {
			t.Errorf("不应包含后续章节内容")
		}
	})
}
