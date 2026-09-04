// Package ingest 接收文档推送：解析 frontmatter、校验后整库替换写入存储。
package ingest

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

// slugPattern 是库标识的合法形式：小写字母、数字与连字符。
var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ErrInvalidSlug 表示库标识不匹配 slugPattern。
var ErrInvalidSlug = fmt.Errorf("库标识不匹配 %s", slugPattern)

// RawDocument 是推送请求里的一篇原始文档，Content 为含 frontmatter 的 Markdown 原文。
type RawDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Push 校验并整批推送：slug 非法返回 ErrInvalidSlug；先解析全部文档的 frontmatter，
// 任一失败返回 *ParseError 且不写库（推送原子）；全部通过后整库替换 slug 库。
func Push(ctx context.Context, store *search.Store, slug string, raws []RawDocument) error {
	if !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}

	docs := make([]search.Document, 0, len(raws))
	for _, raw := range raws {
		doc, err := Parse(raw.Content)
		if err != nil {
			return &ParseError{Path: raw.Path, Err: err}
		}
		doc.Path = raw.Path
		docs = append(docs, doc)
	}

	if err := store.ReplaceLibrary(ctx, slug, docs); err != nil {
		return fmt.Errorf("整库替换 %q: %w", slug, err)
	}
	return nil
}
