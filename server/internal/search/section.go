package search

import (
	"errors"
	"fmt"
	"strings"
)

// ErrSectionNotFound 表示在文档中未找到指定章节。
var ErrSectionNotFound = errors.New("指定章节不存在")

// parseFenceLine 检查一行是否为 Markdown 代码块围栏行（``` 或 ~~~），
// 返回是否为围栏行、围栏字符（` 或 ~）与围栏字符数量。
func parseFenceLine(line string) (bool, rune, int) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 {
		return false, 0, 0
	}
	r := rune(trimmed[0])
	if r != '`' && r != '~' {
		return false, 0, 0
	}
	count := 0
	for _, ch := range trimmed {
		if ch == r {
			count++
		} else {
			break
		}
	}
	if count >= 3 {
		return true, r, count
	}
	return false, 0, 0
}

// ExtractSections 扫描 Markdown 内容中的全部二级标题（## ），忽略代码块内部的注释与标题，返回章节名称列表。
func ExtractSections(content string) []string {
	var sections []string
	var inCodeBlock bool
	var fenceChar rune
	var fenceLen int

	for line := range strings.Lines(content) {
		isFence, fChar, fLen := parseFenceLine(line)
		if inCodeBlock {
			if isFence && fChar == fenceChar && fLen >= fenceLen {
				inCodeBlock = false
			}
			continue
		}
		if isFence {
			inCodeBlock = true
			fenceChar = fChar
			fenceLen = fLen
			continue
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if title != "" {
				sections = append(sections, title)
			}
		}
	}
	return sections
}

// ExtractSection 从 Markdown 内容中提取指定二级标题章节（从该 ## 起到下一个 ## 或文末）。
// 自动识别代码块围栏，不会误将代码块内部的 ## 注释当作章节或提前截断代码块。
// section 参数匹配忽略大小写及前后空白。若 section 为空则返回原内容。
// 若指定章节未找到，返回 ErrSectionNotFound，并在错误信息中列出可用章节。
func ExtractSection(content, section string) (string, []string, error) {
	sections := ExtractSections(content)
	target := strings.TrimSpace(strings.TrimPrefix(section, "## "))
	if target == "" {
		return content, sections, nil
	}

	lowerTarget := strings.ToLower(target)
	var matchedTitle string
	for _, s := range sections {
		if strings.ToLower(s) == lowerTarget {
			matchedTitle = s
			break
		}
	}
	if matchedTitle == "" {
		avail := "无"
		if len(sections) > 0 {
			avail = strings.Join(sections, ", ")
		}
		return "", sections, fmt.Errorf("%w: %q，当前文档可用章节: [%s]", ErrSectionNotFound, target, avail)
	}

	var sb strings.Builder
	capturing := false
	var inCodeBlock bool
	var fenceChar rune
	var fenceLen int

	for line := range strings.Lines(content) {
		isFence, fChar, fLen := parseFenceLine(line)
		if inCodeBlock {
			if isFence && fChar == fenceChar && fLen >= fenceLen {
				inCodeBlock = false
			}
			if capturing {
				sb.WriteString(line)
			}
			continue
		}

		if isFence {
			inCodeBlock = true
			fenceChar = fChar
			fenceLen = fLen
			if capturing {
				sb.WriteString(line)
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if strings.ToLower(title) == lowerTarget {
				capturing = true
				sb.WriteString(line)
				continue
			} else if capturing {
				// 代码块外的下一个二级标题，截断
				break
			}
		}
		if capturing {
			sb.WriteString(line)
		}
	}

	return strings.TrimRight(sb.String(), "\r\n"), sections, nil
}
