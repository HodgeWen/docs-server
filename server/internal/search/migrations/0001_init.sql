-- 初始 schema：libraries / documents 与 FTS5 全文索引。
-- documents_fts 是 documents 的外置内容表，经触发器同步，title 独立成列以便 bm25 加权。

CREATE TABLE libraries (
    id INTEGER PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE
);

CREATE TABLE documents (
    id INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    UNIQUE (library_id, path)
);

CREATE VIRTUAL TABLE documents_fts USING fts5 (
    title,
    description,
    content,
    content = 'documents',
    content_rowid = 'id'
);

CREATE TRIGGER documents_fts_insert AFTER INSERT ON documents BEGIN
    INSERT INTO documents_fts (rowid, title, description, content)
    VALUES (new.id, new.title, new.description, new.content);
END;

CREATE TRIGGER documents_fts_delete AFTER DELETE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, description, content)
    VALUES ('delete', old.id, old.title, old.description, old.content);
END;

CREATE TRIGGER documents_fts_update AFTER UPDATE ON documents BEGIN
    INSERT INTO documents_fts (documents_fts, rowid, title, description, content)
    VALUES ('delete', old.id, old.title, old.description, old.content);
    INSERT INTO documents_fts (rowid, title, description, content)
    VALUES (new.id, new.title, new.description, new.content);
END;
