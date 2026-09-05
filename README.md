# docs-mcp

企业内部库文档检索系统：库维护者把文档推送到中心服务建全文索引，库使用者在编辑器里让 AI 通过远程 MCP 直接检索这些文档。

```
库维护者                          中心服务                          库使用者
push-docs.mjs ──HTTP PUT──▶ docs-mcp（Go 单二进制） ◀──streamable HTTP MCP── 编辑器 MCP client
（零依赖 Node 脚本）          SQLite + FTS5 全文索引              （Kimi Code、Claude Code 等）
```

单仓两部分：

| 部分 | 路径 | 说明 |
| --- | --- | --- |
| 文档服务 | `server/` | Go 单进程：REST API + MCP 端点同端口，SQLite FTS5 全文检索，编译为 CGO 关闭的单文件静态二进制 |
| 推送脚本 | `scripts/push-docs.mjs` | 零依赖单文件 Node 脚本（Node ≥ 18），复制到库仓库使用，整库全量推送 |

## 部署

### 1. 获取二进制

从 [GitHub Releases](https://github.com/HodgeWen/docs-mcp/releases) 下载对应平台的静态二进制（linux/darwin × amd64/arm64），放到服务器任意目录：

```bash
# 例：Linux amd64
curl -LO https://github.com/HodgeWen/docs-mcp/releases/latest/download/docs-mcp-linux-amd64
chmod +x docs-mcp-linux-amd64
```

### 2. 配置环境变量

| 变量 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- |
| `DOCS_MCP_DB_PATH` | 是 | — | SQLite 数据库文件路径（自动建库建索引） |
| `DOCS_MCP_PUSH_TOKEN` | 是 | — | 推送令牌，推送接口 Bearer 鉴权用；读路径免鉴权 |
| `DOCS_MCP_ADDR` | 否 | `:8080` | HTTP 监听地址 |

### 3. 运行

```bash
DOCS_MCP_DB_PATH=/var/lib/docs-mcp/docs.db \
DOCS_MCP_PUSH_TOKEN=$(openssl rand -hex 32) \
DOCS_MCP_ADDR=:8080 \
./docs-mcp-linux-amd64
```

### 4. systemd 常驻（可选）

```ini
# /etc/systemd/system/docs-mcp.service
[Unit]
After=network.target

[Service]
ExecStart=/usr/local/bin/docs-mcp-linux-amd64
Environment=DOCS_MCP_DB_PATH=/var/lib/docs-mcp/docs.db
EnvironmentFile=/etc/docs-mcp.env   # 其中放 DOCS_MCP_PUSH_TOKEN=...
Restart=on-failure
StateDirectory=docs-mcp

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now docs-mcp
```

### 从源码构建

需要 Go 1.26+（在 `server/` 目录下）：

```bash
cd server
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o docs-mcp ./cmd/server
```

## 推送脚本使用

### 1. 准备文档

在你的库仓库里放 Markdown 文档（默认扫描 `docs/` 目录，可换目录）。每个文件需要 YAML frontmatter，`title` 必填：

```markdown
---
title: 快速开始
description: 五分钟上手指南
---

正文……
```

### 2. 安装脚本

把 [`scripts/push-docs.mjs`](scripts/push-docs.mjs) 复制到自己仓库（如 `scripts/push-docs.mjs`）。脚本零依赖、免构建，Node ≥ 18 直接运行。

### 3. 执行推送

```bash
DOCS_MCP_SERVER_URL=http://docs-mcp.internal:8080 \
DOCS_MCP_TOKEN=<推送令牌> \
DOCS_MCP_LIBRARY=my-lib \
node scripts/push-docs.mjs [文档目录，默认 docs/]
```

| 环境变量 | 说明 |
| --- | --- |
| `DOCS_MCP_SERVER_URL` | 服务端地址（结尾斜杠会自动去掉） |
| `DOCS_MCP_TOKEN` | 与服务端 `DOCS_MCP_PUSH_TOKEN` 一致的推送令牌 |
| `DOCS_MCP_LIBRARY` | 库标识（slug）：仅小写字母、数字与连字符 |

行为说明：

- **整库覆盖**：每次推送全量替换该库的全部文档，删掉服务端旧文档。
- **本地校验**：任一文档缺 frontmatter 或缺 `title` 会直接失败，不发出请求。
- **CI 集成**：在库仓库 CI 里配上述三个环境变量后执行脚本即可，例如 GitHub Actions：

```yaml
- run: node scripts/push-docs.mjs
  env:
    DOCS_MCP_SERVER_URL: ${{ vars.DOCS_MCP_SERVER_URL }}
    DOCS_MCP_TOKEN: ${{ secrets.DOCS_MCP_TOKEN }}
    DOCS_MCP_LIBRARY: my-lib
```

## REST API

服务端所有接口挂 `/api/v1/` 前缀；推送需 Bearer 鉴权，读路径免鉴权。错误统一为 `{"error":{"code","message"}}`。

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| PUT | `/api/v1/libraries/{slug}/documents` | Bearer | 整库覆盖推送，body 为 `[{"path","content"}]` 数组 |
| GET | `/api/v1/libraries` | 免 | 列出全部库 slug：`{"libraries":[...]}` |
| GET | `/api/v1/libraries/{slug}/documents/{path}` | 免 | 取文档全文与元数据 |
| GET | `/api/v1/search?q=关键词&library=slug` | 免 | 全文检索，`library` 可选；返回 `[{library,path,title,snippet}]`，命中词以 `<mark>` 包裹 |

```bash
# 搜索示例
curl 'http://localhost:8080/api/v1/search?q=如何分页'
```

## MCP 接入（库使用者）

MCP 端点为 `http://<服务地址>/mcp`（streamable HTTP），免鉴权，暴露三个 tools：

| Tool | 参数 | 说明 |
| --- | --- | --- |
| `search` | `query`（必填）、`library`（可选，限定单库） | 全文检索，bm25 排序、标题加权、高亮片段 |
| `get_document` | `library`、`path`（均必填） | 取回文档全文与元数据 |
| `list_libraries` | 无 | 列出全部库 slug |

Claude Code 接入：

```bash
claude mcp add --transport http docs-mcp http://docs-mcp.internal:8080/mcp
```

其他支持远程 HTTP MCP 的客户端（Kimi Code 等），在 MCP 配置中添加：

```json
{
  "mcpServers": {
    "docs-mcp": { "type": "http", "url": "http://docs-mcp.internal:8080/mcp" }
  }
}
```

## 开发

```bash
cd server
go vet ./...   # 静态检查
go test ./...  # 单元测试
```

CI（[.github/workflows/ci.yml](.github/workflows/ci.yml)）：

- push main / PR：跑 `go vet` + `go test`
- 打 `v*` tag：构建 linux/darwin × amd64/arm64 静态二进制并发布 GitHub Releases

发版流程：合并代码到 main 后，打 tag 并推送即可自动发版：

```bash
git tag v0.1.0-beta.1
git push origin main v0.1.0-beta.1
```

## License

[MIT](LICENSE)
