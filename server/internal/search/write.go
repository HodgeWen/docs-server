package search

import (
	"context"
	"fmt"
)

// Document 是一篇待索引的文档，Path 在库内唯一。
type Document struct {
	Path        string
	Title       string
	Description string
	Content     string
}

// ReplaceLibrary 以 docs 整库替换 slug 库：事务内删除该库旧文档（触发器同步清索引）、
// 写入新文档并更新 FTS 索引；任一步失败整体回滚，库内容不变。
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
		`DELETE FROM documents WHERE library_id = ?`, libraryID); err != nil {
		return fmt.Errorf("清除库 %q 旧文档: %w", slug, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO documents (library_id, path, title, description, content)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备写入语句: %w", err)
	}
	defer stmt.Close()

	for _, d := range docs {
		if _, err := stmt.ExecContext(ctx, libraryID, d.Path, d.Title, d.Description, d.Content); err != nil {
			return fmt.Errorf("写入文档 %q: %w", d.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交整库替换 %q: %w", slug, err)
	}
	return nil
}
