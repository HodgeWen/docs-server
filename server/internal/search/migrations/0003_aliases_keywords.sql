-- 支持 Frontmatter 别名与关键字扩展及高权重加权：
-- 1. documents 表新增 keywords 与 aliases 字段，存储 JSON 数组
-- 2. documents_fts 表调整列为 (title, keywords, description, content)
-- 3. 平滑迁移既有索引行，keywords 初始化为空串

ALTER TABLE documents ADD COLUMN keywords TEXT NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN aliases TEXT NOT NULL DEFAULT '';

CREATE TABLE temp_fts_backup AS SELECT rowid, title, description, content FROM documents_fts;

DROP TABLE documents_fts;

CREATE VIRTUAL TABLE documents_fts USING fts5 (
    title,
    keywords,
    description,
    content
);

INSERT INTO documents_fts (rowid, title, keywords, description, content)
SELECT rowid, title, '', description, content FROM temp_fts_backup;

DROP TABLE temp_fts_backup;
