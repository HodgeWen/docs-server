---
name: docs-mcp
description: >
  库维护者接入企业内部文档检索服务：在库仓库安装推送脚本、按文档标准撰写 Markdown、
  判断并引导创建推送所需 .env、在合适时机执行全量推送。本技能只服务库维护者。
  在用户要求推送/同步/撰写库文档、配置推送环境变量时使用。
  需要检索内部库文档时改用 docs-search 技能。
---

# docs-mcp 库维护者接入

本技能只服务库维护者：把本仓库文档推送到中心服务建全文索引。需要检索内部库文档时使用 `docs-search` 技能（安装与查询步骤不在本技能展开）。

```
库维护者 ──push-docs.mjs（HTTP PUT）──▶ 文档服务（SQLite FTS5）
```

## 库维护者流程

### 1. 安装推送脚本

- 检查仓库内 `scripts/push-docs.mjs` 是否存在。
- 不存在：从本技能目录把 `scripts/push-docs.mjs` 复制到仓库 `scripts/` 下。脚本零依赖免构建，Node ≥ 18 直接运行。

### 2. 判断是否引导创建 .env

检查时机（满足其一即检查）：

- 刚完成脚本安装（首次接入）
- 即将执行推送
- 推送报「缺少必填环境变量」

检查方法：读仓库根目录 `.env` 与当前 shell 环境，确认三个变量齐全：

| 变量 | 说明 |
| --- | --- |
| `DOCS_SERVER_URL` | 文档服务地址，如 `http://docs.internal:8080`（结尾斜杠脚本会自动去掉） |
| `DOCS_TOKEN` | 推送令牌，与服务端 `DOCS_PUSH_TOKEN` 一致，向服务管理员索取 |
| `DOCS_LIBRARY` | 库 slug：仅小写字母、数字与连字符（`^[a-z0-9-]+$`），通常取库名 |

缺失时引导创建：

1. 用提问工具向用户逐项询问缺失的值；禁止编造 token 或服务地址。
2. 写入 `.env`（已存在则只补缺失行，不覆盖已有值）。格式：`KEY=value`，`=` 两侧不留空格。
3. 确认 `.gitignore` 已含 `.env`；没有则追加。token 属敏感信息，严禁提交入库。

### 3. 按标准撰写文档

文档默认放 `docs/`（脚本参数可指定其他目录）。硬性要求：每个 `.md` 必须有 YAML frontmatter 且 `title` 非空，否则推送直接失败。撰写或更新文档时遵循 [references/doc-standards.md](references/doc-standards.md)。

### 4. 执行推送

执行时机：

- 用户明确要求推送/同步文档：直接执行
- 文档发生实质性变更（新增/修改/删除 `.md`）后：主动询问用户是否同步推送
- 推送失败修复后：修复完成即重推

命令（脚本只认进程环境变量，`.env` 需先加载）：

```bash
set -a && source .env && set +a && node scripts/push-docs.mjs
```

Node ≥ 20.6 可用 `node --env-file=.env scripts/push-docs.mjs`。

行为须知：

- 整库覆盖：每次推送全量替换服务端该库全部文档，服务端旧文档会被删除
- 本地校验：任一文档缺 frontmatter 或缺 `title` 直接失败，不会发出请求
- 失败时把脚本错误原文转述给用户，修复后重推，禁止盲目重试

可选：接入 CI（GitHub Actions）让文档随主干自动同步：

```yaml
- run: node scripts/push-docs.mjs
  env:
    DOCS_SERVER_URL: ${{ vars.DOCS_SERVER_URL }}
    DOCS_TOKEN: ${{ secrets.DOCS_TOKEN }}
    DOCS_LIBRARY: my-lib
```

## 检查清单

- [ ] 推送前：三个环境变量齐全，`.env` 已加入 `.gitignore`
- [ ] 每篇文档有 frontmatter 且 `title` 非空
- [ ] 新增文档符合 [references/doc-standards.md](references/doc-standards.md)
- [ ] 推送成功后向用户报告推送篇数

## 反模式

- 编造或猜测 `DOCS_TOKEN`、服务地址、库 slug
- 把 token 写进代码、示例或提交到 git
- 推送失败后不看报错盲目重试
- 未经用户确认擅自提交 `.env`
- 文档塞满内部实现细节与敏感信息
- 在本技能内展开检索安装或查询步骤（检索交给 `docs-search`）
