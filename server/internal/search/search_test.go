package search

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func mustReplace(t *testing.T, s *Store, slug string, docs []Document) {
	t.Helper()
	if err := s.ReplaceLibrary(context.Background(), slug, docs); err != nil {
		t.Fatalf("ReplaceLibrary(%q): %v", slug, err)
	}
}

func mustSearch(t *testing.T, s *Store, query, library string) []Result {
	t.Helper()
	results, err := s.Search(context.Background(), query, library, 0)
	if err != nil {
		t.Fatalf("Search(%q, %q): %v", query, library, err)
	}
	return results
}

func TestReplaceAndSearch(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "guide.md", Title: "入门指南", Description: "新手教程", Content: "SQLite 是一个嵌入式数据库"},
	})

	results := mustSearch(t, s, "sqlite", "")
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(results))
	}
	r := results[0]
	if r.Library != "alpha" || r.Path != "guide.md" || r.Title != "入门指南" {
		t.Errorf("结果元数据不符: %+v", r)
	}
	if !strings.Contains(r.Snippet, "<mark>SQLite</mark>") {
		t.Errorf("片段缺少高亮标记: %q", r.Snippet)
	}
}

func TestSearchRanksTitleMatchFirst(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "content-hit.md", Title: "数据库笔记", Content: "sqlite sqlite sqlite 反复出现"},
		{Path: "title-hit.md", Title: "SQLite 教程", Content: "数据库入门内容"},
	})

	results := mustSearch(t, s, "sqlite", "")
	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，实际 %d", len(results))
	}
	if results[0].Path != "title-hit.md" {
		t.Errorf("标题命中应排第一，实际 %q", results[0].Path)
	}
}

func TestSearchRanksByFrequency(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "less.md", Title: "较少", Content: "token 出现一次"},
		{Path: "more.md", Title: "较多", Content: "token token token token token 出现多次"},
	})

	results := mustSearch(t, s, "token", "")
	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，实际 %d", len(results))
	}
	if results[0].Path != "more.md" {
		t.Errorf("词频高者应排第一，实际 %q", results[0].Path)
	}
}

func TestSearchLibraryFilter(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "a.md", Title: "Alpha 文档", Content: "grep 使用说明"},
	})
	mustReplace(t, s, "beta", []Document{
		{Path: "b.md", Title: "Beta 文档", Content: "grep 进阶技巧"},
	})

	if all := mustSearch(t, s, "grep", ""); len(all) != 2 {
		t.Fatalf("跨库检索期望 2 条，实际 %d", len(all))
	}
	filtered := mustSearch(t, s, "grep", "alpha")
	if len(filtered) != 1 || filtered[0].Library != "alpha" {
		t.Fatalf("按库过滤结果不符: %+v", filtered)
	}
}

func TestReplaceRemovesOldDocuments(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "old.md", Title: "旧文档", Content: "obsolete 内容"},
	})
	mustReplace(t, s, "beta", []Document{
		{Path: "keep.md", Title: "保留", Content: "obsolete 但属于 beta"},
	})

	mustReplace(t, s, "alpha", []Document{
		{Path: "new.md", Title: "新文档", Content: "fresh 内容"},
	})

	results := mustSearch(t, s, "obsolete", "")
	if len(results) != 1 || results[0].Library != "beta" {
		t.Fatalf("替换后旧文档仍可检索: %+v", results)
	}
	if _, err := s.GetDocument(context.Background(), "alpha", "old.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("替换后旧文档应不可取回，实际 err=%v", err)
	}
}

func TestReplaceIsAtomic(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "old.md", Title: "旧文档", Content: "stable 内容"},
	})

	err := s.ReplaceLibrary(context.Background(), "alpha", []Document{
		{Path: "dup.md", Title: "一", Content: "x"},
		{Path: "dup.md", Title: "二", Content: "y"},
	})
	if err == nil {
		t.Fatal("同批重复 path 应报错")
	}
	results := mustSearch(t, s, "stable", "")
	if len(results) != 1 || results[0].Path != "old.md" {
		t.Fatalf("替换失败后旧文档应保持不变: %+v", results)
	}
}

func TestGetDocument(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "guide.md", Title: "入门指南", Description: "新手教程", Content: "完整正文内容"},
	})

	d, err := s.GetDocument(context.Background(), "alpha", "guide.md")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if d.Path != "guide.md" || d.Title != "入门指南" || d.Description != "新手教程" || d.Content != "完整正文内容" {
		t.Errorf("取回文档不符: %+v", d)
	}
	if _, err := s.GetDocument(context.Background(), "alpha", "missing.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在文档应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := s.GetDocument(context.Background(), "ghost", "guide.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在库的文档应返回 ErrNotFound，实际 %v", err)
	}
}

func TestListLibraries(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "beta", nil)
	mustReplace(t, s, "alpha", []Document{{Path: "a.md", Title: "A", Content: "内容"}})

	slugs, err := s.ListLibraries(context.Background())
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "alpha" || slugs[1] != "beta" {
		t.Errorf("库列表不符: %v", slugs)
	}
}

func TestSearchQuerySanitization(t *testing.T) {
	s, _ := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{
		{Path: "a.md", Title: "文档", Content: "full text search 支持"},
	})

	if results := mustSearch(t, s, "   ", ""); len(results) != 0 {
		t.Errorf("空查询应无结果，实际 %+v", results)
	}
	if results := mustSearch(t, s, `full "text" (search)`, ""); len(results) != 1 {
		t.Errorf("含特殊字符查询不应报错且按词匹配，实际 %+v", results)
	}
}

func TestSearchChinese(t *testing.T) {
	s, _ := openTestStore(t)
	content := "本教程帮助快速开始并返回高亮片段"
	mustReplace(t, s, "alpha", []Document{
		{Path: "guide.md", Title: "入门指南", Description: "新手教程", Content: content},
		{Path: "other.md", Title: "进阶参考", Content: "只提到高亮二字"},
	})

	t.Run("中文子串命中", func(t *testing.T) {
		results := mustSearch(t, s, "高亮", "")
		if len(results) != 2 {
			t.Fatalf("q=高亮 期望 2 条结果，实际 %d", len(results))
		}
	})

	t.Run("中文整词仍命中", func(t *testing.T) {
		results := mustSearch(t, s, "快速开始", "")
		if len(results) != 1 || results[0].Path != "guide.md" {
			t.Fatalf("q=快速开始 结果不符: %+v", results)
		}
	})

	t.Run("查询词以连续 mark 完整包裹", func(t *testing.T) {
		results := mustSearch(t, s, "高亮", "alpha")
		var snippet string
		for _, r := range results {
			if r.Path == "guide.md" {
				snippet = r.Snippet
			}
		}
		if !strings.Contains(snippet, "<mark>高亮</mark>") {
			t.Errorf("片段未完整包裹查询词: %q", snippet)
		}
		if strings.Contains(snippet, "<mark>高</mark>") || strings.Contains(snippet, "<mark>亮</mark>") {
			t.Errorf("查询词被拆成单字标记: %q", snippet)
		}
	})

	t.Run("片段去标记与省略号后为原文连续子串", func(t *testing.T) {
		results := mustSearch(t, s, "高亮", "alpha")
		for _, r := range results {
			if r.Path != "guide.md" {
				continue
			}
			if stripped := stripSnippet(r.Snippet); !strings.Contains(content, stripped) {
				t.Errorf("去标记后 %q 不是原文 %q 的连续子串", stripped, content)
			}
		}
	})

	t.Run("多词 AND", func(t *testing.T) {
		results := mustSearch(t, s, "高亮 片段", "")
		if len(results) != 1 || results[0].Path != "guide.md" {
			t.Fatalf("q=高亮 片段 应只命中 guide.md，实际 %+v", results)
		}
	})

	t.Run("特殊字符查询不报错", func(t *testing.T) {
		if results := mustSearch(t, s, `高亮 "片段"`, ""); len(results) != 1 {
			t.Errorf("含引号查询应命中 1 条，实际 %+v", results)
		}
		if results := mustSearch(t, s, "（）", ""); len(results) != 0 {
			t.Errorf("纯标点查询应无结果，实际 %+v", results)
		}
	})
}

func TestUpgradeFrom0001(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// 以 0001 schema 建库并写入含中文的文档，模拟旧版库。
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("打开旧版库: %v", err)
	}
	schema0001, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatalf("读取 0001 迁移: %v", err)
	}
	stmts := []string{
		string(schema0001),
		`CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`INSERT INTO schema_migrations (version) VALUES ('0001_init.sql')`,
		`INSERT INTO libraries (slug) VALUES ('alpha')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("建旧版库: %v", err)
		}
	}
	content := "旧版写入的正文包含高亮片段四个字"
	if _, err := db.Exec(
		`INSERT INTO documents (library_id, path, title, description, content)
		 VALUES (1, 'guide.md', '入门指南', '新手教程', ?)`, content); err != nil {
		t.Fatalf("写入旧版文档: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧版库: %v", err)
	}

	// 新版 Open 应用 0002 迁移并重建索引，中文子串查询直接命中既有文档。
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("升级 Open: %v", err)
	}
	results := mustSearch(t, s, "高亮", "")
	if len(results) != 1 || results[0].Path != "guide.md" {
		t.Fatalf("升级后 q=高亮 应命中既有文档，实际 %+v", results)
	}
	if !strings.Contains(results[0].Snippet, "<mark>高亮</mark>") {
		t.Errorf("升级后片段未完整包裹查询词: %q", results[0].Snippet)
	}

	// 迁移重建的索引文本与 Go 写入路径的预分词结果一致（空白归一后相同）。
	var indexed string
	if err := s.db.QueryRow(`SELECT content FROM documents_fts`).Scan(&indexed); err != nil {
		t.Fatalf("读取重建索引: %v", err)
	}
	if got, want := strings.Join(strings.Fields(indexed), " "), indexText(content); got != want {
		t.Errorf("重建索引文本 = %q，期望 %q", got, want)
	}

	// get_document 取回的全文与元数据不变。
	d, err := s.GetDocument(context.Background(), "alpha", "guide.md")
	if err != nil {
		t.Fatalf("升级后取文档: %v", err)
	}
	if d.Path != "guide.md" || d.Title != "入门指南" || d.Description != "新手教程" || d.Content != content {
		t.Errorf("升级后文档不符: %+v", d)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 重复 Open 幂等：迁移记录不重复，检索与取文档结果不变。
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("重复 Open: %v", err)
	}
	defer reopened.Close()
	var migrations int
	if err := reopened.db.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = '0002_fts5_zh_tokenizer.sql'`).Scan(&migrations); err != nil {
		t.Fatalf("查询迁移记录: %v", err)
	}
	if migrations != 1 {
		t.Errorf("0002 迁移记录期望 1 条，实际 %d", migrations)
	}
	if results := mustSearch(t, reopened, "高亮", ""); len(results) != 1 {
		t.Errorf("重开后 q=高亮 期望 1 条，实际 %+v", results)
	}
	d2, err := reopened.GetDocument(context.Background(), "alpha", "guide.md")
	if err != nil || d2 != d {
		t.Errorf("重开后文档不符: %+v, err=%v", d2, err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s, path := openTestStore(t)
	mustReplace(t, s, "alpha", []Document{{Path: "a.md", Title: "A", Content: "内容"}})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("重复 Open（重跑迁移）: %v", err)
	}
	defer reopened.Close()
	d, err := reopened.GetDocument(context.Background(), "alpha", "a.md")
	if err != nil {
		t.Fatalf("重开后取文档: %v", err)
	}
	if d.Title != "A" {
		t.Errorf("重开后数据不符: %+v", d)
	}
}
