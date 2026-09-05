package search

import (
	"context"
	"fmt"
)

// Document 是一篇待索引的文档，Path 在库内唯一。
type Document struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// ReplaceLibrary 以 docs 整库替换 slug 库：事务内删除该库旧文档及其 FTS 索引、
// 写入新文档并把经 indexText 预分词的文本写入索引；任一步失败整体回滚，库内容不变。
func (s *Store) ReplaceLibrary(ctx context.Context, slug string, docs []Document) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO libraries (slug) VALUES (?)`, slug); err != nil {
		return fmt.Errorf("创建库 %q: %w", slug, err)
	}
	var libraryID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM libraries WHERE slug = ?`, slug).Scan(&libraryID); err != nil {
		return fmt.Errorf("查询库 %q: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM documents_fts WHERE rowid IN (SELECT id FROM documents WHERE library_id = ?)`, libraryID); err != nil {
		return fmt.Errorf("清除库 %q 旧索引: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM documents WHERE library_id = ?`, libraryID); err != nil {
		return fmt.Errorf("清除库 %q 旧文档: %w", slug, err)
	}

	docStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO documents (library_id, path, title, description, content)
		VALUES (?, ?, ?, ?, ?) RETURNING id`)
	if err != nil {
		return fmt.Errorf("准备写入语句: %w", err)
	}
	defer docStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO documents_fts (rowid, title, description, content)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备索引语句: %w", err)
	}
	defer ftsStmt.Close()

	for _, d := range docs {
		var docID int64
		if err := docStmt.QueryRowContext(ctx, libraryID, d.Path, d.Title, d.Description, d.Content).Scan(&docID); err != nil {
			return fmt.Errorf("写入文档 %q: %w", d.Path, err)
		}
		if _, err := ftsStmt.ExecContext(ctx, docID, indexText(d.Title), indexText(d.Description), indexText(d.Content)); err != nil {
			return fmt.Errorf("写入文档 %q 索引: %w", d.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交整库替换 %q: %w", slug, err)
	}
	return nil
}
