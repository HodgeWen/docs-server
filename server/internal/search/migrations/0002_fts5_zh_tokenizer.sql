-- 中文分词升级：FTS 索引改存应用层预分词文本（CJK 连续段展开为单字 + 相邻二元组，
-- 非 CJK 词保持整词，token 以空格分隔），unicode61 即可把每个 token 视为独立词元，
-- 中文子串查询经二元组短语命中。旧索引与同步触发器废弃，索引按同一分词方案从
-- documents 全量重建；documents 原文不动，数据不丢。

DROP TRIGGER documents_fts_insert;
DROP TRIGGER documents_fts_delete;
DROP TRIGGER documents_fts_update;
DROP TABLE documents_fts;

-- 新索引不再外置内容：写入路径（write.go）直接写入预分词文本；snippet 取自索引文本，
-- 由 normalizeSnippet 归一化为原文片段。
CREATE VIRTUAL TABLE documents_fts USING fts5 (
    title,
    description,
    content
);

-- 从 documents 重建索引。字符级状态机与 tokenize.go 的 tokenize() 对齐：CJK 段产出
-- 全部单字 + 相邻二元组，词字符段保留整词，其余字符视为分隔；输出按
-- （CJK 段序号, 段内序：单字/词 < 二元组 < 段后词, 字符位置）排序，与 tokenize 的
-- token 顺序一致。CJK 码点区间与 Go unicode.Han / Hiragana / Katakana / Hangul 相同；
-- 词字符对 ASCII 精确，非 ASCII 仅排除常见标点块，个别罕见符号的分类可能与 Go 版
-- 不同（只影响存量行的索引文本，整库重写后即以 Go 版为准）。
WITH RECURSIVE
src(id, col, txt) AS (
    SELECT id, 1, title FROM documents WHERE length(title) > 0
    UNION ALL SELECT id, 2, description FROM documents WHERE length(description) > 0
    UNION ALL SELECT id, 3, content FROM documents WHERE length(content) > 0
),
walk(id, col, pos, n, txt) AS (
    SELECT id, col, 1, length(txt), txt FROM src
    UNION ALL
    SELECT id, col, pos + 1, n, txt FROM walk WHERE pos < n
),
chars(id, col, pos, ch, cp) AS (
    SELECT id, col, pos, substr(txt, pos, 1), unicode(substr(txt, pos, 1)) FROM walk
),
flagged(id, col, pos, ch, cp, is_cjk) AS (
    SELECT id, col, pos, ch, cp, (
        cp BETWEEN 0x2E80 AND 0x2E99 OR cp BETWEEN 0x2E9B AND 0x2EF3 OR
        cp BETWEEN 0x2F00 AND 0x2FD5 OR cp IN (0x3005, 0x3007) OR
        cp BETWEEN 0x3021 AND 0x3029 OR cp BETWEEN 0x3038 AND 0x303B OR
        cp BETWEEN 0x3400 AND 0x4DBF OR cp BETWEEN 0x4E00 AND 0x9FFF OR
        cp BETWEEN 0xF900 AND 0xFA6D OR cp BETWEEN 0xFA70 AND 0xFAD9 OR
        cp BETWEEN 0x16FE2 AND 0x16FE3 OR cp BETWEEN 0x16FF0 AND 0x16FF1 OR
        cp BETWEEN 0x20000 AND 0x2A6DF OR cp BETWEEN 0x2A700 AND 0x2B739 OR
        cp BETWEEN 0x2B740 AND 0x2B81D OR cp BETWEEN 0x2B820 AND 0x2CEA1 OR
        cp BETWEEN 0x2CEB0 AND 0x2EBE0 OR cp BETWEEN 0x2F800 AND 0x2FA1D OR
        cp BETWEEN 0x30000 AND 0x3134A OR cp BETWEEN 0x31350 AND 0x323AF OR
        cp BETWEEN 0x3041 AND 0x3096 OR cp BETWEEN 0x309D AND 0x309F OR
        cp BETWEEN 0x1B001 AND 0x1B11F OR
        cp IN (0x1B132, 0x1B150, 0x1B151, 0x1B152, 0x1F200) OR
        cp BETWEEN 0x30A1 AND 0x30FA OR cp BETWEEN 0x30FD AND 0x30FF OR
        cp BETWEEN 0x31F0 AND 0x31FF OR cp BETWEEN 0x32D0 AND 0x32FE OR
        cp BETWEEN 0x3300 AND 0x3357 OR cp BETWEEN 0xFF66 AND 0xFF6F OR
        cp BETWEEN 0xFF71 AND 0xFF9D OR
        cp BETWEEN 0x1AFF0 AND 0x1AFF3 OR cp BETWEEN 0x1AFF5 AND 0x1AFFB OR
        cp BETWEEN 0x1AFFD AND 0x1AFFE OR
        cp IN (0x1B000, 0x1B120, 0x1B121, 0x1B122, 0x1B155, 0x1B164,
               0x1B165, 0x1B166, 0x1B167) OR
        cp BETWEEN 0x1100 AND 0x11FF OR cp IN (0x302E, 0x302F) OR
        cp BETWEEN 0x3131 AND 0x318E OR cp BETWEEN 0x3200 AND 0x321E OR
        cp BETWEEN 0x3260 AND 0x327E OR cp BETWEEN 0xA960 AND 0xA97C OR
        cp BETWEEN 0xAC00 AND 0xD7A3 OR cp BETWEEN 0xD7B0 AND 0xD7C6 OR
        cp BETWEEN 0xD7CB AND 0xD7FB OR cp BETWEEN 0xFFA0 AND 0xFFBE OR
        cp BETWEEN 0xFFC2 AND 0xFFC7 OR cp BETWEEN 0xFFCA AND 0xFFCF OR
        cp BETWEEN 0xFFD2 AND 0xFFD7 OR cp BETWEEN 0xFFDA AND 0xFFDC
    ) FROM chars
),
classified(id, col, pos, ch, is_cjk, is_word) AS (
    SELECT id, col, pos, ch, is_cjk, (
        NOT is_cjk AND (
            cp BETWEEN 0x30 AND 0x39 OR cp BETWEEN 0x41 AND 0x5A OR
            cp BETWEEN 0x61 AND 0x7A OR (
                cp >= 0x80 AND
                cp NOT BETWEEN 0x300 AND 0x36F AND
                cp NOT BETWEEN 0xA0 AND 0xB4 AND
                (cp NOT BETWEEN 0xB6 AND 0xBF OR cp = 0xBA) AND
                cp NOT IN (0xD7, 0xF7) AND
                cp NOT BETWEEN 0x2000 AND 0x20FF AND
                cp NOT BETWEEN 0x2100 AND 0x2BFF AND
                cp NOT BETWEEN 0x2E00 AND 0x2E7F AND
                cp NOT BETWEEN 0x3000 AND 0x303F AND
                cp NOT BETWEEN 0xFE00 AND 0xFE0F AND
                cp NOT BETWEEN 0xFE30 AND 0xFE4F AND
                cp NOT BETWEEN 0xFF00 AND 0xFF0F AND
                cp NOT BETWEEN 0xFF1A AND 0xFF20 AND
                cp NOT BETWEEN 0xFF3B AND 0xFF40 AND
                cp NOT BETWEEN 0xFF5B AND 0xFF65 AND
                cp NOT BETWEEN 0xE000 AND 0xF8FF AND
                cp NOT BETWEEN 0x1F000 AND 0x1FAFF
            )
        )
    ) FROM flagged
),
sequenced(id, col, pos, ch, is_cjk, is_word, prev_ch, prev_cjk, prev_word) AS (
    SELECT id, col, pos, ch, is_cjk, is_word,
        LAG(ch)     OVER (PARTITION BY id, col ORDER BY pos),
        LAG(is_cjk) OVER (PARTITION BY id, col ORDER BY pos),
        LAG(is_word) OVER (PARTITION BY id, col ORDER BY pos)
    FROM classified
),
grouped(id, col, pos, ch, is_cjk, is_word, prev_ch, prev_cjk, prev_word, run_id) AS (
    SELECT *,
        SUM(CASE WHEN is_cjk = 1 AND (prev_cjk IS NULL OR prev_cjk = 0) THEN 1 ELSE 0 END)
            OVER (PARTITION BY id, col ORDER BY pos)
    FROM sequenced
),
pieces(id, col, run_id, sec, ord, piece) AS (
    SELECT id, col, run_id,
        CASE WHEN is_cjk = 1 THEN 0 ELSE 2 END,
        pos,
        CASE
            WHEN is_cjk = 1 THEN ' ' || ch
            WHEN is_word = 1 THEN CASE WHEN prev_word = 1 THEN '' ELSE ' ' END || ch
            ELSE ' '
        END
    FROM grouped
    UNION ALL
    SELECT id, col, run_id, 1, pos, ' ' || prev_ch || ch
    FROM grouped WHERE is_cjk = 1 AND prev_cjk = 1
),
assembled(id, col, txt) AS (
    SELECT id, col, trim(group_concat(piece, '' ORDER BY run_id, sec, ord))
    FROM pieces
    GROUP BY id, col
)
INSERT INTO documents_fts (rowid, title, description, content)
SELECT id,
    MAX(CASE WHEN col = 1 THEN txt END),
    MAX(CASE WHEN col = 2 THEN txt END),
    MAX(CASE WHEN col = 3 THEN txt END)
FROM assembled
GROUP BY id;
