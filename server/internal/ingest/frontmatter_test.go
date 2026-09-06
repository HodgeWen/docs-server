package ingest

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	doc, err := Parse("---\ntitle: 入门指南\ndescription: 新手教程\nauthor: 某人\n---\n# 正文\n内容\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Title != "入门指南" || doc.Description != "新手教程" {
		t.Errorf("元数据不符: %+v", doc)
	}
	if doc.Content != "# 正文\n内容\n" {
		t.Errorf("正文应去掉 frontmatter，实际 %q", doc.Content)
	}
}

func TestParseIgnoresExtraFields(t *testing.T) {
	doc, err := Parse("---\ntitle: 标题\ntags:\n  - a\n  - b\nnested:\n  key: value\n---\n正文\n")
	if err != nil {
		t.Fatalf("含未知字段应解析成功: %v", err)
	}
	if doc.Title != "标题" || doc.Description != "" {
		t.Errorf("元数据不符: %+v", doc)
	}
}

func TestParseWithoutDescription(t *testing.T) {
	doc, err := Parse("---\ntitle: 仅标题\n---\n正文\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Description != "" {
		t.Errorf("description 缺省应为空串，实际 %q", doc.Description)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"无 frontmatter":   "# 只有正文\n",
		"缺结尾分隔线":          "---\ntitle: 标题\n正文\n",
		"缺 title":         "---\ndescription: 只有描述\n---\n正文\n",
		"title 为空白":       "---\ntitle: \" \"\n---\n正文\n",
		"YAML 非法":         "---\ntitle: [未闭合\n---\n正文\n",
		"frontmatter 非映射": "---\n- 只是列表\n---\n正文\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(content); err == nil {
				t.Fatal("期望解析失败，实际成功")
			} else if errors.Is(err, errNoFrontmatter) != (name == "无 frontmatter") {
				t.Errorf("errNoFrontmatter 判定不符: %v", err)
			}
		})
	}
}

func TestParseCRLF(t *testing.T) {
	doc, err := Parse("---\r\ntitle: 标题\r\n---\r\n正文\r\n")
	if err != nil {
		t.Fatalf("CRLF 文档应解析成功: %v", err)
	}
	if doc.Title != "标题" || !strings.HasPrefix(doc.Content, "正文") {
		t.Errorf("解析结果不符: %+v", doc)
	}
}

func TestParseAliasesAndKeywords(t *testing.T) {
	t.Run("YAML 列表格式", func(t *testing.T) {
		raw := `---
title: UTableEditor
description: 表格编辑器
aliases:
  - 行内编辑
  - 单元格编辑
keywords:
  - table
  - editor
---
正文内容
`
		doc, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		wantAliases := []string{"行内编辑", "单元格编辑"}
		if !reflect.DeepEqual(doc.Aliases, wantAliases) {
			t.Errorf("Aliases = %v, 期望 %v", doc.Aliases, wantAliases)
		}
		wantKeywords := []string{"table", "editor"}
		if !reflect.DeepEqual(doc.Keywords, wantKeywords) {
			t.Errorf("Keywords = %v, 期望 %v", doc.Keywords, wantKeywords)
		}
	})

	t.Run("内联数组与标量逗号格式", func(t *testing.T) {
		raw := `---
title: UTableEditor
aliases: [行内编辑, 单元格编辑]
keywords: "table，editor, grid"
---
正文内容
`
		doc, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		wantAliases := []string{"行内编辑", "单元格编辑"}
		if !reflect.DeepEqual(doc.Aliases, wantAliases) {
			t.Errorf("Aliases = %v, 期望 %v", doc.Aliases, wantAliases)
		}
		wantKeywords := []string{"table", "editor", "grid"}
		if !reflect.DeepEqual(doc.Keywords, wantKeywords) {
			t.Errorf("Keywords = %v, 期望 %v", doc.Keywords, wantKeywords)
		}
	})
}
