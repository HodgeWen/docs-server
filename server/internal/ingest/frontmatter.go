package ingest

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

// errNoFrontmatter 表示文档缺少 YAML frontmatter 区块。
var errNoFrontmatter = errors.New("缺少 frontmatter")

// ParseError 携带解析失败的文档路径，供上层定位整批中出错的文档。
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("解析文档 %q: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// frontmatter 是可入库的元数据字段集；title/description 之外的字段忽略。
type frontmatter struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

// Parse 解析含 YAML frontmatter 的 Markdown 原文：提取 title（必填）与
// description（可选），返回去掉 frontmatter 的正文。缺 frontmatter、缺 title
// 或 YAML 解析失败均返回错误。
func Parse(content string) (search.Document, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return search.Document{}, err
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return search.Document{}, fmt.Errorf("frontmatter YAML 解析失败: %w", err)
	}
	if strings.TrimSpace(fm.Title) == "" {
		return search.Document{}, errors.New("frontmatter 缺少必填字段 title")
	}

	return search.Document{
		Title:       fm.Title,
		Description: fm.Description,
		Content:     body,
	}, nil
}

// splitFrontmatter 切出开头 `---` 与结尾 `---` 之间的 YAML 区块与剩余正文。
func splitFrontmatter(content string) (meta string, body string, err error) {
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		if rest, ok = strings.CutPrefix(content, "---\r\n"); !ok {
			return "", "", errNoFrontmatter
		}
	}
	// 结尾分隔线是独占一行的 ---；逐行扫描以兼容 \r\n。
	for len(rest) > 0 {
		line, tail, _ := strings.Cut(rest, "\n")
		if strings.TrimSuffix(line, "\r") == "---" {
			return strings.TrimSuffix(meta, "\n"), tail, nil
		}
		meta += line + "\n"
		rest = tail
	}
	return "", "", errors.New("frontmatter 缺少结尾分隔线 ---")
}
