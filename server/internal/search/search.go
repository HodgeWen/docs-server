package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound 表示所查文档不存在，供上层映射为 404 语义。
var ErrNotFound = errors.New("文档不存在")

// defaultLimit 是 limit 非法（<= 0）时的检索条数上限。
const defaultLimit = 20

// Result 是一条检索命中，Snippet 为经 normalizeSnippet 归一化的高亮片段，
// 命中词以 <mark> 连续包裹。
type Result struct {
	Library string `json:"library"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// Search 在 FTS5 索引中检索 query；library 非空时限定单库，缺省跨库。
// query 经 matchQuery 转成 MATCH 表达式（中文词按二元组短语匹配，多词 AND）。
// 结果按 bm25 排序且 title 列加权，最多返回 limit 条。
func (s *Store) Search(ctx context.Context, query string, library string, limit int) ([]Result, error) {
	match := matchQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	q := `
		SELECT l.slug, d.path, d.title,
			snippet(documents_fts, -1, '<mark>', '</mark>', '…', 32)
		FROM documents_fts
		JOIN documents d ON d.id = documents_fts.rowid
		JOIN libraries l ON l.id = d.library_id
		WHERE documents_fts MATCH ?`
	args := []any{match}
	if library != "" {
		q += ` AND l.slug = ?`
		args = append(args, library)
	}
	// bm25 权重对应 FTS 列序：title、description、content，标题命中优先。
	q += ` ORDER BY bm25(documents_fts, 10.0, 3.0, 1.0) LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("检索 %q: %w", query, err)
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Library, &r.Path, &r.Title, &r.Snippet); err != nil {
			return nil, fmt.Errorf("读取检索结果: %w", err)
		}
		r.Snippet = normalizeSnippet(r.Snippet)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历检索结果: %w", err)
	}
	return results, nil
}

// GetDocument 按 library + path 取回文档全文与元数据；不存在时返回 ErrNotFound。
func (s *Store) GetDocument(ctx context.Context, library, path string) (Document, error) {
	var d Document
	err := s.db.QueryRowContext(ctx, `
		SELECT d.path, d.title, d.description, d.content
		FROM documents d
		JOIN libraries l ON l.id = d.library_id
		WHERE l.slug = ? AND d.path = ?`, library, path).
		Scan(&d.Path, &d.Title, &d.Description, &d.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("查询文档 %s/%s: %w", library, path, err)
	}
	return d, nil
}

// ListLibraries 返回全部库 slug，按字母序排列。
func (s *Store) ListLibraries(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug FROM libraries ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("列出库: %w", err)
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("读取库列表: %w", err)
		}
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历库列表: %w", err)
	}
	return slugs, nil
}
