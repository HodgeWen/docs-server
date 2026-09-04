// Package search 提供基于 SQLite FTS5 的文档存储与全文检索。
package search

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store 是文档检索存储，全部状态落在一个 SQLite 文件中。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）path 处的 SQLite 数据库，并按序执行内嵌迁移。
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %q: %w", path, err)
	}
	// SQLite 单写者：限制单连接，避免 database/sql 并发连接触发 SQLITE_BUSY。
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate 按文件名顺序执行 migrations/ 下尚未应用的迁移，逐个事务提交并记录版本。
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("创建迁移记录表: %w", err)
	}

	// embed.FS 的 ReadDir 已按文件名排序。
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取迁移目录: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if err := s.applyMigration(ctx, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version string) error {
	var applied int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
		return fmt.Errorf("查询迁移 %s 状态: %w", version, err)
	}
	if applied > 0 {
		return nil
	}

	content, err := migrationsFS.ReadFile("migrations/" + version)
	if err != nil {
		return fmt.Errorf("读取迁移 %s: %w", version, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启迁移 %s 事务: %w", version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("执行迁移 %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("记录迁移 %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移 %s: %w", version, err)
	}
	return nil
}
