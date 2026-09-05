# 代码地图

`server/`、`scripts/` 与部署产物（CI 二进制发布）均已落地。

## 树

```
docs-mcp/
├── server/                   # Go 模块，文档检索 + MCP 单服务
│   ├── cmd/server/           # main：装配配置、存储、REST 与 MCP handler，起 HTTP 服务
│   └── internal/
│       ├── api/              # REST handler 与路由（/api/v1/），推送 Bearer 鉴权
│       ├── mcp/              # MCP streamable HTTP 端点与 tools 定义
│       ├── ingest/           # 接收推送：frontmatter 解析、整批校验、整库替换写库
│       └── search/           # SQLite FTS5 索引与全文检索
├── scripts/
│   └── push-docs.mjs         # 零依赖 Node 推送脚本，用户复制到库仓库使用
├── skills/
│   └── docs-mcp/             # 随库分发的接入技能：维护者推送/文档标准 + 使用者接 MCP；内含脚本副本
├── .github/workflows/        # GitHub Actions：测试 + v* tag 构建四平台二进制发 Releases
└── .agents/                  # 工程协作（docs/scripts 入库，cooking 忽略）
```

## 模块

| 模块 | 路径 | 职责 | 主要入口 |
| --- | --- | --- | --- |
| 服务端入口 | `server/cmd/server/` | 装配配置、存储、REST 与 MCP handler，起服务 | `server/cmd/server/main.go` |
| REST API 层 | `server/internal/api/` | 路由、推送 Bearer 鉴权、请求/响应、错误格式 | `server/internal/api/` |
| MCP 端点 | `server/internal/mcp/` | streamable HTTP 端点，search / get_document / list_libraries tools | `server/internal/mcp/` |
| 推送接收 | `server/internal/ingest/` | frontmatter 解析、整批校验、整库替换写入 | `server/internal/ingest/` |
| 索引与检索 | `server/internal/search/` | SQLite FTS5 建索引、bm25 + 标题加权、高亮片段 | `server/internal/search/` |
| 推送脚本 | `scripts/push-docs.mjs` | 扫描库内文档，HTTP 全量推送到服务端 | `scripts/push-docs.mjs` |
| docs-mcp 技能 | `skills/docs-mcp/` | 双视角接入技能：维护者装脚本/引导 .env/文档标准/执行推送，使用者检测并引导接入 MCP；脚本副本须与 `scripts/push-docs.mjs` 同步 | `skills/docs-mcp/SKILL.md` |

## 依赖

```mermaid
graph TD
    push["scripts/push-docs.mjs"] --> api["server/internal/api"]
    mcpclient["MCP client（外部）"] --> mcp["server/internal/mcp"]
    api --> ingest["server/internal/ingest"]
    api --> search["server/internal/search"]
    mcp --> search
    ingest --> search
```

## 关键路径

- 推送：库仓库执行 `push-docs.mjs` → PUT 服务端 `api`（Bearer 鉴权）→ `ingest`（frontmatter 解析 + 整批校验）→ 整库替换写 SQLite 并重建 FTS5 索引。
- 检索：MCP client 连服务端 MCP 端点 → AI 调 search tool → `mcp` → `search` 查 FTS5 → 高亮片段经 MCP 返回；取全文走 get_document。
