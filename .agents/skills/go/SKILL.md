---
name: go
description: Go 开发实践。按项目 go.mod 的 go 行（优先 1.24 / 1.25 / 1.26 / 1.27）选用该版本语言与标准库 API。在编辑 .go、go.mod、go.work、*_test.go 或处理 Go 工具链时使用。
---

# Go

框架、构建工具、HTTP 库跟项目走，不要在本技能里指定。

禁止凭训练数据写 API。先定语言版本，再读对应 reference。

## 定版本

1. 读 **`go.mod` 的 `go` 行**（语言版本）。不要用本机 `go version` 当语言版本。有 `go.work` 时同样读其中的 `go` 行。
2. 有 `toolchain` 时：工具链可能更新，**语法与 std API 仍受 `go` 行约束**。`go 1.24` + `toolchain go1.26.x` 仍按 1.24 写，不要用 1.26 才有的 `new(expr)`。
3. 取 **1.y**（`go 1.25.2` → `1.25`），打开下表文件，按该文件写代码。单文件 `//go:build go1.xx` 只抬高该文件。

| 1.y | 文件 |
| --- | ---- |
| 1.24 | [references/1.24.md](references/1.24.md) |
| 1.25 | [references/1.25.md](references/1.25.md) |
| 1.26 | [references/1.26.md](references/1.26.md) |
| 1.27 | [references/1.27.md](references/1.27.md) |

未列出的更新 1.y：以已覆盖的最高档为底，再查官方 https://go.dev/doc/go1.N 。更旧：不要使用本技能里的新 API。

## 硬规则（1.24+）

- 整数循环用 `for i := range n`；遍历 iterator 用 `for x := range seq`。不要把 `for i := 0; i < n; i++` 当默认。
- 错误用 `fmt.Errorf("...: %w", err)` / `errors.Join`。不要 `err.Error()` 拼接后再 `errors.New`。
- 测试表驱动：`tests := []struct{ name string; ... }{...}` + `t.Run(tt.name, ...)`。
- 标准库已有的能力不要再拉重复实现（尤其 `crypto/*`、`testing`、`iter`）。先搜 `pkg.go.dev` 当前语言版本的包文档。
