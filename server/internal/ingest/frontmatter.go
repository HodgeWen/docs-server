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

// stringList 支持从 YAML 序列（[a, b]）或标量（"a, b"）解析字符串列表。
type stringList []string

func (s *stringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		str := strings.TrimSpace(value.Value)
		if str == "" {
			*s = nil
			return nil
		}
		parts := strings.FieldsFunc(str, func(r rune) bool {
			return r == ',' || r == '，'
		})
		var list []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				list = append(list, trimmed)
			}
		}
		*s = list
		return nil
	case yaml.SequenceNode:
		var list []string
		for _, item := range value.Content {
			if item.Kind == yaml.ScalarNode && strings.TrimSpace(item.Value) != "" {
				list = append(list, strings.TrimSpace(item.Value))
			}
		}
		*s = list
		return nil
	default:
		return nil
	}
}

// frontmatter 是可入库的元数据字段集；支持 title、description、aliases 与 keywords。
type frontmatter struct {
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Aliases     stringList `yaml:"aliases"`
	Keywords    stringList `yaml:"keywords"`
}

// Parse 解析含 YAML frontmatter 的 Markdown 原文：提取 title（必填）与
// description、aliases、keywords（可选），返回去掉 frontmatter 的正文。缺 frontmatter、缺 title
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
		Keywords:    []string(fm.Keywords),
		Aliases:     []string(fm.Aliases),
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
