# 开发规范

## 命名

- Go：遵循 Effective Go 惯例；包名小写单词；测试 `*_test.go` 与被测代码同包。
- JS 脚本：单文件 `push-docs.mjs` / `query.mjs`，函数 camelCase。
- API 路径段、库标识：小写连字符。

## 目录与代码结构

- `server/internal/`：按 `api` / `ingest` / `search` 分包；依赖方向 api → ingest/search，禁止 internal 内循环依赖。
- `scripts/push-docs.mjs` 与 `skills/docs-search/scripts/query.mjs`：保持零依赖单文件，只用 Node 内置 API（fetch、fs、path），保证复制即跑。

## 代码风格

- 注释、文档、commit message 用中文；标识符用英文。
- Go：gofmt/goimports 格式化，提交前 `go vet` 无警告。

## 测试

- 核心逻辑必须有单测：索引、检索、推送解析。
- 用 `go test ./...`；测试与被测代码同包。

## 接口

- REST API 统一 `/api/v1/` 前缀，请求/响应均为 JSON。
- 错误响应统一格式：`{ "error": { "code": "...", "message": "..." } }`，code 为机器可读的小写下划线串。
- 推送接口鉴权：`Authorization: Bearer <token>`；读路径（搜索/取文档/列库）免鉴权。

## 数据与存储

- 全部状态在服务端 SQLite 单文件；schema 变更必须走迁移文件，禁止手改库。

## 日志

- 服务端：`log/slog` 结构化 JSON 日志输出到 stdout。

## 版本与发布

- server 出静态二进制（CI 在 `v*` tag 构建发布到 GitHub Releases），与仓库 tag 对应。

## 明确禁止

- cooking `spec.md` 缺少可被 `spec-files.mjs parse` 通过的「影响文件」章节
- 查询脚本或检索技能保存文档副本或检索缓存（检索状态只在服务端 SQLite）
- 在 `server/` 里引入 HTTP 框架依赖（已定标准库 net/http）
