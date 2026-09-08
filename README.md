# docs-server

企业内部库文档检索系统：库维护者把文档推送到中心服务建全文索引，库使用者在编辑器里让 AI 通过 Agent Skill 调用 REST 检索这些文档。

```
库维护者                          中心服务                          库使用者
push-docs.mjs ──HTTP PUT──▶ docs-server（Go 单二进制） ◀──REST── Agent Skill（docs-search 查询脚本）
（零依赖 Node 脚本）          SQLite + FTS5 全文索引              （npx skills add 安装）
```

单仓四部分：

| 部分 | 路径 | 说明 |
| --- | --- | --- |
| 文档服务 | `server/` | Go 单进程：REST API，SQLite FTS5 全文检索，编译为 CGO 关闭的单文件静态二进制 |
| 推送脚本 | `scripts/push-docs.mjs` | 零依赖单文件 Node 脚本（Node ≥ 18），复制到库仓库使用，整库全量推送 |
| 检索技能 | `skills/docs-search/` | 通用检索技能：内嵌查询脚本调 REST（list / search / get） |
| 文档生成技能 | `skills/docs-gen/` | 只服务库：文档标准（检索优化）、安装推送脚本、库代码改动后同步文档、执行推送 |

## 部署

### 1. 获取二进制

从 [GitHub Releases](https://github.com/HodgeWen/docs-server/releases) 下载对应平台的静态二进制（linux/darwin × amd64/arm64），放到服务器任意目录：

```bash
# 例：Linux x64
curl -LO https://github.com/HodgeWen/docs-server/releases/latest/download/docs-mcp-linux-x64
chmod +x docs-mcp-linux-x64
```

### 2. 配置

配置来源两选一（或混用，优先级：环境变量 > 配置文件 > 默认值）。

**方式 A：环境变量**

| 变量 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- |
| `DOCS_DB_PATH` | 是 | — | SQLite 数据库文件路径（自动建库建索引） |
| `DOCS_PUSH_TOKEN` | 是 | — | 推送令牌，推送接口 Bearer 鉴权用；读路径免鉴权 |
| `DOCS_ADDR` | 否 | `:8080` | HTTP 监听地址 |

**方式 B：YAML 配置文件**

```yaml
# /etc/docs-server.yaml
addr: ":8080"
db_path: /var/lib/docs-server/docs.db
push_token: <openssl rand -hex 32 生成的令牌>
```

用 `-config` 参数或 `DOCS_CONFIG` 环境变量指定文件路径：

```bash
./docs-mcp-linux-x64 -config /etc/docs-server.yaml
```

### 3. 运行

```bash
DOCS_DB_PATH=/var/lib/docs-server/docs.db \
DOCS_PUSH_TOKEN=$(openssl rand -hex 32) \
DOCS_ADDR=:8080 \
./docs-mcp-linux-x64
```

### 4. systemd 常驻（可选）

```ini
# /etc/systemd/system/docs-server.service
[Unit]
After=network.target

[Service]
ExecStart=/usr/local/bin/docs-mcp-linux-x64
Environment=DOCS_DB_PATH=/var/lib/docs-server/docs.db
EnvironmentFile=/etc/docs-server.env   # 其中放 DOCS_PUSH_TOKEN=...
Restart=on-failure
StateDirectory=docs-server

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now docs-server
```

### 从源码构建

需要 Go 1.26+（在 `server/` 目录下）：

```bash
cd server
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o docs-mcp ./cmd/server
```

## 推送脚本使用

### 1. 准备文档

在你的库仓库里放 Markdown 文档（默认扫描 `agent-docs/` 目录，可换目录）。每个文件需要 YAML frontmatter，`title` 必填：

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
DOCS_SERVER_URL=http://docs-server.internal:8080 \
DOCS_TOKEN=<推送令牌> \
DOCS_LIBRARY=my-lib \
node scripts/push-docs.mjs [文档目录，默认 agent-docs/]
```

| 环境变量 | 说明 |
| --- | --- |
| `DOCS_SERVER_URL` | 服务端地址（结尾斜杠会自动去掉） |
| `DOCS_TOKEN` | 与服务端 `DOCS_PUSH_TOKEN` 一致的推送令牌 |
| `DOCS_LIBRARY` | 库标识（slug）：仅小写字母、数字与连字符 |

行为说明：

- **整库覆盖**：每次推送全量替换该库的全部文档，删掉服务端旧文档。
- **本地校验**：任一文档缺 frontmatter 或缺 `title` 会直接失败，不发出请求。
- **CI 集成**：在库仓库 CI 里配上述三个环境变量后执行脚本即可，例如 GitHub Actions：

```yaml
- run: node scripts/push-docs.mjs
  env:
    DOCS_SERVER_URL: ${{ vars.DOCS_SERVER_URL }}
    DOCS_TOKEN: ${{ secrets.DOCS_TOKEN }}
    DOCS_LIBRARY: my-lib
```

## REST API

服务端所有接口挂 `/api/v1/` 前缀；推送需 Bearer 鉴权，读路径免鉴权。错误统一为 `{"error":{"code","message"}}`。

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| PUT | `/api/v1/libraries/{slug}/documents` | Bearer | 整库覆盖推送，body 为 `[{"path","content"}]` 数组 |
| GET | `/api/v1/libraries` | 免 | 列出全部库 slug：`{"libraries":[...]}` |
| GET | `/api/v1/libraries/{slug}/documents/{path}` | 免 | 取文档；可选 `?section=` 提取 `## ` 章节；返回 `{library,path,title,description,keywords,aliases,sections,content}` |
| GET | `/api/v1/search?q=关键词&library=slug` | 免 | 全文检索，`library` 可选；返回 `{"results":[{library,path,title,description,snippet}]}`，命中词以 `<mark>` 包裹 |

```bash
# 搜索示例
curl 'http://localhost:8080/api/v1/search?q=如何分页'
```

## 检索接入（库使用者）

消费侧是 Agent Skill + REST：安装本仓库技能、设置 `DOCS_SERVER_URL`，由 AI 运行 `docs-search` 查询脚本。读路径免鉴权。

```bash
# 安装检索技能（也可不加 --skill，同时装上库维护者用的 docs-gen）
npx skills add HodgeWen/docs-server --skill docs-search

# 已安装则更新
npx skills update
```

设置服务地址（结尾斜杠会自动去掉）：

```bash
export DOCS_SERVER_URL=http://docs-server.internal:8080
```

需要检索内部库文档时，AI 从技能目录运行内嵌脚本（Node ≥ 26）：

```bash
node scripts/query.mjs libraries
node scripts/query.mjs search --q <关键词> [--library <slug>]
node scripts/query.mjs get --library <slug> --path <path> [--section <章节>]
```

### 从旧接入方式迁移

若你曾把本服务配成各宿主的远程 MCP 端点：

1. **删除各宿主 MCP 配置**：去掉 Claude Code 的 `claude mcp add ... /mcp`、配置文件里的 `mcpServers.docs-mcp` 等条目。
2. **改环境变量名**（服务端不再读取任何 `DOCS_MCP_*`）：
   - `DOCS_MCP_ADDR` → `DOCS_ADDR`
   - `DOCS_MCP_DB_PATH` → `DOCS_DB_PATH`
   - `DOCS_MCP_PUSH_TOKEN` → `DOCS_PUSH_TOKEN`
   - `DOCS_MCP_CONFIG` → `DOCS_CONFIG`
   - `DOCS_MCP_SERVER_URL` → `DOCS_SERVER_URL`
   - `DOCS_MCP_TOKEN` → `DOCS_TOKEN`
   - `DOCS_MCP_LIBRARY` → `DOCS_LIBRARY`
3. **安装检索技能**：按上一节用 `npx skills add` / `npx skills update` 安装 `docs-search`，并设置 `DOCS_SERVER_URL`。

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
git tag v0.1.0-beta.2
git push origin main v0.1.0-beta.2
```

## License

[MIT](LICENSE)
