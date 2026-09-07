# 库文档编写与生成规范指南（面向 docs-mcp）

本指南面向**库维护者**以及**负责自动生成库文档的 AI Agent / 自动化脚本**，指导如何编写和组织库文档，使文档在被推送至 `docs-mcp` 服务后，能够被使用者的 AI 编程助手（如 Claude Code、Kimi Code、Cursor 等）**更准确、更全面、更快速**地检索与消费。

---

## 一、docs-mcp 核心机制揭秘：文档如何被检索与消费

了解服务端的检索与切片机制，有助于针对性地优化文档结构：

```
                    ┌───────────────────────── docs-mcp 服务端 ────────────────────────┐
                    │                                                                  │
【库项目文档】       │   1. 解析 Frontmatter 提取 title, description, keywords, aliases  │
.md 源码 (docs/)    │   2. 预分词处理：                                                 │
   │                │      - 中文：单字 + 相邻二元组（Bigram）                          │
   │ push-docs.mjs  │      - 英文/数字：完整词元保留                                    │
   ▼                │   3. 写入 SQLite FTS5 虚拟表，建立 BM25 索引：                    │
[整库全量推送] ────▶ │      - title (权重 10.0)                                         │
                    │      - keywords & aliases (权重 10.0)                            │
                    │      - description (权重 3.0)                                    │
                    │      - content (权重 1.0)                                        │
                    └─────────────────────────────────┬────────────────────────────────┘
                                                      │
                    ┌─────────────────────────────────┴────────────────────────────────┐
                    │              消费：REST + docs-search 查询脚本                    │
                    │                                                                  │
                    │  search（GET /api/v1/search）                                    │
                    │   - 自动过滤中文停用词（"如何"、"怎么"、"使用" 等）              │
                    │   - 优先 AND 短语匹配；无结果时自动降级 OR 宽容匹配               │
                    │   - 返回 title, description, path, snippet(高亮命中片段)         │
                    │                                                                  │
                    │  get_document（GET .../documents/{path}，可选 section）          │
                    │   - 传入 section 可按二级标题（## ）精准提取局部章节             │
                    │   - 提取失败时返回文档全部可用二级标题列表供 AI 重新决策         │
                    │   - 技能侧：query.mjs get --library --path [--section]           │
                    └──────────────────────────────────────────────────────────────────┘
```

---

## 二、Frontmatter 元数据规范（核心提效手段）

每篇 Markdown 必须以 YAML Frontmatter 开头。`docs-mcp` 对各字段赋予了不同的检索权重与消费逻辑：

### 1. 字段定义与权重

| 字段 | 必填 | 搜索权重 | 消费用途 | 规范建议 |
| :--- | :---: | :---: | :--- | :--- |
| `title` | **是** | **10.0** | 列表展示、精准定级 | 包含模块/组件名 + 中文全称，如 `Table 表格组件` |
| `description` | 否 | **3.0** | AI 检索摘要、决策依据 | 1~2 句话讲清定位与核心解决场景，杜绝空话 |
| `keywords` | 否 | **10.0** | 核心意图召回、场景词补充 | 补充开发者的提问词、功能动宾短语、核心参数名 |
| `aliases` | 否 | **10.0** | 别名匹配、同义词对齐 | 补充中英文别称、缩写、旧接口名、类似开源库命名 |

> [!TIP]
> `keywords` 与 `aliases` 在服务端共享 **10.0 的最高 BM25 权重**（与 `title` 等权）。善用这两个字段是消除“开发者提问用词与文档用词不一致”最立竿见影的手段。

### 2. 标准 Frontmatter 模板

支持 YAML 数组（`[...]`）或逗号分隔的标量字符串：

```markdown
---
title: Table 表格组件
description: 用于展示多行结构化数据，支持客户端/服务端分页、行内编辑、列冻结及虚拟滚动。
aliases:
  - DataTable
  - Grid
  - 数据表格
keywords:
  - 分页
  - 排序
  - 虚拟列表
  - 大数据量渲染
  - 行展开
  - pagination
  - sorting
---
```

### 3. Frontmatter 填写反模式与正例

- ❌ **坏味道**：
  ```markdown
  ---
  title: 表格
  description: 表格组件的使用方法
  ---
  ```
  *后果*：AI 搜索 `DataTable`、`虚拟滚动` 或 `大数据量分页` 时均无法通过高权重匹配召回。
-  **好味道**：
  ```markdown
  ---
  title: Table 表格组件
  description: 基础数据展示组件，支持复杂表头、单选多选、排序筛选、自定义单元格渲染与懒加载。
  aliases: [Grid, DataGrid, 表格, 行列表]
  keywords: [columns, dataSource, rowKey, 分页查询, 树形数据, 勾选行]
  ---
  ```

---

## 三、目录结构与文档划分粒度

合理的物理组织能让 AI 既不会在单篇超长文档中迷失，也不会在碎片化小文件中频繁发起网络请求。

### 1. 推荐目录布局

库项目的文档默认统一置于根目录 `docs/` 下：

```text
docs/
├── index.md                   # 库概览、安装指南、整体架构与模块速查表
├── guide/                     # 概念指南与进阶指引
│   ├── quick-start.md         # 快速开始
│   ├── configuration.md       # 全局配置与环境变量
│   └── best-practices.md      # 最佳实践与避坑指南
├── components/ 或 apis/       # 按模块/组件划分的详细 API 手册
│   ├── button.md
│   ├── table.md
│   └── select.md
├── recipes/                   # 常见典型场景解决方案（Cookbook）
│   ├── custom-rendering.md    # 高频复杂业务场景 1
│   └── form-table-binding.md  # 高频复杂业务场景 2
└── troubleshooting/           # 排障与常见错误
    ├── common-errors.md       # 报错代码、原因与解决方案
    └── migration.md           # 破坏性变更与迁移指南
```

### 2. 划分粒度黄金准则

- **单篇文档行数控制在 100 ~ 500 行**：
  - **避免大包揽**：不要将全库 API 塞入一个 `README.md` 或 `all.md` 中。文档过长会导致上下文膨胀、Token 浪费，且不同 API 的关键词会在全文中互相稀释权重。
  - **避免过度粉碎**：不要为每个只有 2 行类型定义的微小函数单独建一个 `.md`（如 `isString.md`、`isNumber.md`）。应按**逻辑模块**合并（如 `type-guards.md`）。
- **按“开发者意图”而非仅仅按“代码文件”组织**：
  - 一个具有独立生命周期的组件、一个核心服务 Class、或一组高度协同的工具函数，应对应一篇独立文档。

---

## 四、二级标题（`## `）设计规范：开启精准切片提取

REST 取文档接口（`GET /api/v1/libraries/{slug}/documents/{path}`）与 `docs-search` 查询脚本的 `get --section` 均支持 `section` 参数：**直接根据二级标题（`## `）截取该段内容并返回**。

如果 AI 仅需查询参数列表，只需提取 `## Props`，不仅响应速度提升数倍，而且节省了 80% 以上的上下文空间。

### 1. 标题命名规范

为保障跨文档的一致性，推荐统一采用下列语义明确的二级标题命名：

| 推荐二级标题 | 适用内容 |
| :--- | :--- |
| `## 快速上手` 或 `## Quick Start` | 最简引入与基础运行示例（几行代码讲清用法） |
| `## API 签名` 或 `## 类型定义` | TypeScript 接口、Go Struct、函数签名等强类型代码块 |
| `## 参数说明` 或 `## Props` | 属性、入参、配置项的含义、类型、默认值与必填项 |
| `## 方法与事件` 或 `## Methods & Events` | 暴露的实例方法、回调事件、Hook 返回值 |
| `## 典型示例` 或 `## Examples` | 2~3 个覆盖 80% 核心场景的高质量代码范式 |
| `## 注意事项` 或 `## Gotchas` | 容易踩坑的隐式行为、副作用、性能瓶颈、禁止写法 |
| `## 常见问题` 或 `## FAQ` | 经典报错、排查流程、替代用法 |

### 2. 规范要点

1. **必须且仅使用 `## ` 作为主要分段标志**：
   - 服务端切片提取算法以行开头的 `## ` 为边界（直到遇到下一个 `## ` 或文档末尾）。
   - 三级标题 `### `、四级标题 `#### ` 会被完整包含在所属的二级标题章节中。
2. **切片容错机制**：
   - 服务端匹配 `section` 参数时已忽略大小写，并支持传 `## Props` 或 `Props`。
   - 若 AI 请求了不存在的章节，服务端会返回该文档**当前所有的可用章节名称列表**。因此规范命名的二级标题能引导 AI 快速自动纠偏。
3. **避免在代码块内破坏分段**：
   - 服务端算法会自动识别 ` ``` ` 和 `~~~` 围栏，跳过代码块内部的 `## 注释`，但建议在文档代码块中保持干净规范。

---

## 五、面向 AI 消费的内容编写原则

AI 读取文档并据此写代码，不同于人类翻阅教程。以下原则能大幅降低 AI 的“幻觉”率：

### 1. 类型定义优先于纯文字描述

AI 对编程语言原生类型的理解能力，远高于长篇大论的自然语言表格。

-  **推荐（提供紧凑的类型定义块）**：
  ````markdown
  ## API 签名

  ```typescript
  export interface TableProps<T = any> {
    /** 数据源数组，必填 */
    dataSource: T[];
    /** 列配置清单，必填 */
    columns: ColumnItem<T>[];
    /** 唯一行主键，默认 'id' */
    rowKey?: string | ((record: T) => string);
    /** 加载态 */
    loading?: boolean;
    /** 分页变更回调 */
    onPageChange?: (page: number, pageSize: number) => void;
  }
  ```
  ````

- ⚠️ **次优（单纯只有 Markdown 表格）**：
  表格占用更多字符，且复杂泛型、联合类型难以表达精确。若有表格，建议作为类型声明的辅助补充。

### 2. 提供最小可运行示例（MRE）

AI 极易把不同版本、不同库的调用方式混淆。文档中的示例必须**自闭环、包含确切引用路径**：

- 给出完整的 `import` 语句（使用企业私有包真实导出的路径，不要用相对路径 `../../src`）。
- 示例尽量精炼，只展示核心模式，避免为了“真实业务”而引入大量无关的 UI 布局或业务逻辑。

```typescript
import { Table } from '@company/ui-core';

export function SimpleDemo() {
  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '年龄', dataIndex: 'age', key: 'age' },
  ];
  const data = [{ id: '1', name: '张三', age: 28 }];

  return <Table rowKey="id" columns={columns} dataSource={data} />;
}
```

### 3. 显式列出“禁止用法”与“版本差异”

AI 常常从开源大模型基座知识库中“脑补”一些通用的 API。例如，开源库是 `onChange`，而你司内部库是 `onValueChange`：

````markdown
## 注意事项

> [!WARNING]
> - **属性命名**：数值变更事件为 `onValueChange`，而非 `onChange`。
> - **严禁直接修改 dataSource**：必须通过 `setDataSource` 进行不可变更新。
> - **废弃说明**：`isStriped` 属性已在 v2.0 废弃，请统一改用 `striped={true}`。
````

---

## 六、标准文档范例（Golden Sample）

以下是一份完全契合 `docs-mcp` 最佳实践的组件文档范例：

````markdown
---
title: Request HTTP 请求客户端
description: 基于 Axios 封装的企业级统一 HTTP 请求库，内置鉴权拦截、网关重试、TraceId 链路追踪与统一错误弹窗。
aliases:
  - httpClient
  - AxiosWrapper
  - 网络请求
keywords:
  - interceptor
  - token 刷新
  - 401 重试
  - traceId
  - 统一错误处理
---

# Request HTTP 请求客户端

统一网络请求客户端，预置内部网关鉴权协议与通用熔断策略。

## 快速上手

```typescript
import { request } from '@company/request';

interface UserInfo {
  id: string;
  name: string;
}

// 发起标准 GET 请求
const user = await request.get<UserInfo>('/api/v1/user/profile');
console.log(user.name);
```

## API 签名

```typescript
export interface RequestConfig extends AxiosRequestConfig {
  /** 是否跳过全局统一错误弹窗，默认 false */
  skipErrorHandler?: boolean;
  /** 失败自动重试次数，默认 0 */
  retryTimes?: number;
  /** 自定义业务错误码捕获规则 */
  customValidateStatus?: (status: number, data: any) => boolean;
}

export interface HttpClient {
  get<T = any>(url: string, config?: RequestConfig): Promise<T>;
  post<T = any>(url: string, data?: any, config?: RequestConfig): Promise<T>;
  put<T = any>(url: string, data?: any, config?: RequestConfig): Promise<T>;
  delete<T = any>(url: string, config?: RequestConfig): Promise<T>;
}
```

## 参数说明

| 参数名 | 类型 | 默认值 | 必填 | 说明 |
| :--- | :--- | :--- | :---: | :--- |
| `skipErrorHandler` | `boolean` | `false` | 否 | 为 `true` 时发生 4xx/5xx 或业务非 0 码不弹出全局 Toast |
| `retryTimes` | `number` | `0` | 否 | 遇到网络超时或 502/503 时的最大重试次数 |

## 典型示例

### 场景一：静默提交并自行处理错误

```typescript
import { request, BusinessError } from '@company/request';

try {
  await request.post('/api/v1/order/submit', orderData, {
    skipErrorHandler: true,
  });
} catch (err) {
  if (err instanceof BusinessError && err.code === 10042) {
    // 针对特定业务码进行前端特殊流转
  }
}
```

## 常见问题

> [!NOTE]
> **Q: 为什么本地联调时报 401 Token Expired 且没有自动重试？**  
> A: 确保在项目入口调用了 `initAuthBridge()` 完成了与单点登录 SDK 的桥接绑定。
````

---

## 七、自动化生成策略（给 AI Agent / 提取脚本）

如果你正在使用 AI Agent 或 AST 解析脚本为库自动生成文档，请遵循以下自动化提取逻辑：

1. **Frontmatter 自动提纯**：
   - 提取导出的组件名/类名/函数名作为 `title`。
   - 读取符号的 JSDoc / Docstring 的首段概述作为 `description`。
   - 将所有导出的公开属性名、方法名、枚举值、以及相关场景词聚合为 `keywords`。
   - 将该符号在历史版本或同类库中的常见叫法提取为 `aliases`。
2. **规范二级标题约束**：
   - 提示词中强制生成模型使用固定的二级标题结构：
     `## 快速上手`、`## API 签名`、`## 参数说明`、`## 典型示例`、`## 注意事项`。
3. **过滤内部符号**：
   - 严格排除 `@internal`、`@private`、未导出的 Helper 工具。

---

## 八、CI/CD 自动化校验与推送

在库项目的持续集成流程中集成 `push-docs.mjs`，确保代码变更后文档自动同步到 `docs-mcp` 服务端。

### 1. 本地前置校验

`push-docs.mjs` 在推送前会对所有 Markdown 进行强校验：
- 文件必须含有 `---\n...\n---` 的 Frontmatter 区块。
- Frontmatter 中必须包含非空的 `title` 字段。
- 只要有任意一篇文档校验失败，推送会立即中止并退出，避免脏数据污染索引。

### 2. CI 配置示例（GitHub Actions）

在库仓库的 `.github/workflows/docs.yml` 中配置：

```yaml
name: Sync Documentation to docs-mcp

on:
  push:
    branches: [ main ]
    paths:
      - 'docs/**'
      - 'scripts/push-docs.mjs'
  workflow_dispatch:

jobs:
  push-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Push docs to docs-mcp
        env:
          DOCS_SERVER_URL: ${{ vars.DOCS_SERVER_URL }}
          DOCS_TOKEN: ${{ secrets.DOCS_TOKEN }}
          DOCS_LIBRARY: my-library-slug
        run: |
          node scripts/push-docs.mjs docs
```

### 3. 检查清单（Checklist）

在将库文档正式推送到 `docs-mcp` 之前，请核对：

- [ ] 每个 Markdown 文件开头都有符合规范的 Frontmatter，包含 `title` 与 `description`。
- [ ] 针对易混淆或提问频率高的场景，已在 `keywords` 和 `aliases` 中补充了同义词与参数名。
- [ ] 核心内容均归类在规范命名的二级标题（`## `）下，便于 `docs-search` 按 `section` 切片提取。
- [ ] 代码示例包含完整的公共导出路径，无内部私有路径引用。
- [ ] 标注了关键的避坑指南与与通用框架/开源库的用法差异。
- [ ] 库标识符 `DOCS_LIBRARY` 仅包含小写英文字母、数字与连字符（如 `my-sdk`、`ui-core`）。
