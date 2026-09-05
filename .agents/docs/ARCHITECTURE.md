# 架构

## 业务架构

docs-mcp 面向企业内部：库维护者把库文档推送到中心文档服务；库使用者在编辑器里通过 MCP client（Kimi Code、Claude Code 等）让 AI 直接检索这些文档。

核心域：文档采集（推送）、索引（全文检索）、消费（MCP 工具）。

主要流程：

1. 库维护者把 `scripts/push-docs.mjs` 复制到自己仓库，配环境变量（服务端地址、令牌、库标识），手动或在 CI 执行，把 Markdown 文档（含 frontmatter）经 HTTP 全量推送到文档服务。
2. 文档服务接收推送，整库替换写入 SQLite 并重建 FTS5 全文索引。
3. 使用者的 MCP client 直接连文档服务的远程 MCP 端点（streamable HTTP），AI 通过 search / get_document / list_libraries 检索文档。

## 技术架构

单仓两部分：

- **文档服务（Go，`server/`）**：唯一带状态、唯一部署的部分，单进程同时提供：
  - REST API（`/api/v1/`）：推送（单令牌 Bearer 鉴权、整库全量覆盖）、搜索、取文档、列库；读路径免鉴权。标准库 `net/http`，零框架依赖。
  - MCP 端点（`/mcp`，streamable HTTP）：官方 MCP Go SDK，暴露 search / get_document / list_libraries。
  - SQLite FTS5 全文索引：bm25 排序、标题列加权、高亮片段、按库过滤；纯 Go 驱动（modernc.org/sqlite），编译为静态二进制。
  - 部署：CGO 关闭的单文件静态二进制，拷到服务器直接运行；GitHub Actions CI 在 `v*` tag 构建 linux/darwin × amd64/arm64 发 Releases。
- **推送脚本（Node.js，`scripts/push-docs.mjs`）**：零依赖单文件脚本，随本仓库源码分发，用户复制到库仓库使用；环境变量配置（服务端地址、令牌、库 slug）；解析 frontmatter，全量推送。

进程边界：推送脚本 →（HTTP）→ 文档服务 ←（streamable HTTP MCP）← MCP client（使用者本机编辑器宿主）。

数据只存一处：文档服务的 SQLite 单文件（含 FTS5 索引）。MCP 端点与推送脚本均不存状态。

### 技术栈

| 层 | 选型 | 备注 |
| --- | --- | --- |
| 语言 / runtime | Go 最新稳定版；Node ≥ 18（推送脚本） | 推送脚本零依赖免构建 |
| 框架 | Go 标准库 net/http；官方 MCP Go SDK（streamable HTTP 传输） | 服务端零 Web 框架 |
| 数据 | SQLite FTS5（服务内嵌） | 纯 Go 驱动 modernc.org/sqlite |
| 构建 / 包管理 | Go modules（`server/`） | 单模块 |
| 测试 | go test | 核心逻辑必须有单测 |
| 部署 | 单文件静态二进制（CGO 关闭）；GitHub Actions CI 在 `v*` tag 发 Releases | 推送脚本随源码分发 |

## 未决

- 无
