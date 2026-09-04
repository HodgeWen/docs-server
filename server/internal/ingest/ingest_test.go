package ingest

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

func openTestStore(t *testing.T) *search.Store {
	t.Helper()
	s, err := search.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func docWithTitle(title string) RawDocument {
	return RawDocument{Path: "doc.md", Content: "---\ntitle: " + title + "\n---\nsqlite 正文\n"}
}

func TestPushInvalidSlug(t *testing.T) {
	s := openTestStore(t)
	err := Push(context.Background(), s, "Bad_Slug", []RawDocument{docWithTitle("标题")})
	if !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("期望 ErrInvalidSlug，实际 %v", err)
	}
	slugs, err := s.ListLibraries(context.Background())
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(slugs) != 0 {
		t.Errorf("slug 非法不应建库，实际 %v", slugs)
	}
}

func TestPushParsesThenReplaces(t *testing.T) {
	s := openTestStore(t)
	err := Push(context.Background(), s, "alpha", []RawDocument{
		{Path: "a.md", Content: "---\ntitle: 文档 A\ndescription: 描述\n---\nsqlite 内容\n"},
		{Path: "dir/b.md", Content: "---\ntitle: 文档 B\n---\ngrep 内容\n"},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	doc, err := s.GetDocument(context.Background(), "alpha", "dir/b.md")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.Title != "文档 B" || doc.Content != "grep 内容\n" {
		t.Errorf("入库文档不符: %+v", doc)
	}
}

func TestPushRejectsWholeBatch(t *testing.T) {
	s := openTestStore(t)
	if err := Push(context.Background(), s, "alpha", []RawDocument{
		{Path: "old.md", Content: "---\ntitle: 旧文档\n---\nstable 内容\n"},
	}); err != nil {
		t.Fatalf("首次 Push: %v", err)
	}

	err := Push(context.Background(), s, "alpha", []RawDocument{
		{Path: "ok.md", Content: "---\ntitle: 新文档\n---\nfresh 内容\n"},
		{Path: "bad.md", Content: "# 没有 frontmatter\n"},
	})
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("期望 *ParseError，实际 %v", err)
	}
	if parseErr.Path != "bad.md" {
		t.Errorf("ParseError 应带出错路径 bad.md，实际 %q", parseErr.Path)
	}

	doc, err := s.GetDocument(context.Background(), "alpha", "old.md")
	if err != nil {
		t.Fatalf("整批拒绝后旧文档应保持不变: %v", err)
	}
	if doc.Title != "旧文档" {
		t.Errorf("旧文档内容被改动: %+v", doc)
	}
	if _, err := s.GetDocument(context.Background(), "alpha", "ok.md"); !errors.Is(err, search.ErrNotFound) {
		t.Errorf("整批拒绝后新文档不应入库，实际 err=%v", err)
	}
}
