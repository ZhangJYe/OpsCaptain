# AGENTS.md — OpsCaption 工程护栏

本文件是所有 AI Agent 在本项目中工作时的行为约束。每次启动自动加载。
犯错时由人类更新本文件，形成反馈循环。优先级高于外部 Wiki、Slack、飞书文档。

---

## 1. 行为红线

| # | 规则 | 违反后果 |
|---|------|---------|
| 1 | **允许必要注释**，用于解释复杂逻辑、边界条件或外部协议 | 降低维护成本 |
| 2 | **不主动 commit / push**，除非用户明确要求 | 用户可能想先 review |
| 3 | **推送前必须跑 `go test ./...` 和 `npm run build`** | CI 会挂 |
| 4 | commit message **用中文** | 团队规范 |
| 5 | **优先编辑已有文件**，不创建不必要的文件 | 仓库膨胀 |
| 6 | 配置项**不硬编码**，走 `config.yaml` | 后续改不了 |
| 7 | 新增能力必须**可配置**（budget / top_k / timeout / feature flag） | 无法调参 |
| 8 | 错误处理走 `ResultStatusDegraded`，**不直接 fatal** | 服务崩溃 |
| 9 | **不要暴露或日志记录 secrets / keys** | 安全事故 |
| 10 | **不要重新接回 `chat_multi_agent`** 路由 | 已废弃架构 |
| 11 | 修改 RAG / Agent / ContextEngine 后，**必须跑对应 package 测试** | 局部改动可能破坏上游 |
| 12 | **每次 push 前必须先 pull 最新远端代码** | 避免推送失败 |

---

## 2. AI Coding Rules v1

### 2.1 硬规则（违反即 revert）

- **Controller 不直连 DB / infra SDK**。Controller 只做参数解析、鉴权上下文、响应映射。
- **`internal/ai/rag/` 不 import `internal/infra/milvus`、`milvus-sdk-go`、`utility/client`**。✅ 已有 import guard test 自动检查。
- **`utility/` 不新增 `internal/` 依赖**。现有例外已登记，不允许新增同类违规。
- **新增配置不得 hardcode**，必须进入 `manifest/config/config.yaml`。
- **不允许恢复 `chat_multi_agent` 路由**和 `supervisor/triage/reporter/skillspecialists` 编排。

### 2.2 分层规则

```
API Layer            internal/controller/       参数解析 → 调用 Application → 格式化响应
Application Layer    internal/app/              编排业务流程：ChatApp、AIOpsApp、KnowledgeApp
Domain Layer         internal/ai/               核心业务规则：检索、推理、证据、Agent
Infrastructure Layer internal/infra/            外部系统适配：Milvus、RabbitMQ、Redis
Common Layer         utility/                   通用横切关注点：认证、限流、安全、健康检查
```

各层职责边界详见 [01-system-architecture-guide.md](./Learn/system/01-system-architecture-guide.md) 第 3 节。

文件大小警戒线：
- Controller 方法 < 50 行
- Application Service < 300 行
- Domain Service < 500 行
- Infrastructure Adapter < 400 行

### 2.3 Agent / RAG / Tool 规则

- **Agent 输出诊断结论时必须带 evidence**；没有证据时输出 `ResultStatusDegraded` 或明确说明证据不足。
- **RAG 检索默认必须经过 rewrite / retrieve / rerank / token budget 控制**，除非测试或显式配置关闭。
- **检索结果进入上下文前必须经过 ContextEngine budget 裁剪**。
- **Memory 只能作为执行上下文，不能替代原始 query 做 routing**。
- **新增 tool 必须补充 schema、timeout、权限边界、失败降级行为、最小测试**。
- **工具调用逻辑不能散落在 Controller 或随意业务代码里**，应进入 tool registry / skill / app 编排层。
- **RAG 优化必须先看 baseline / holdout 评测**，不要只凭单次问答效果判断。

---

## 3. import 规则（目标态）

```
controller/ → app/                     ✅
app/        → ai/                      ✅
app/        → infra/                   ✅（通过 interface）
ai/         → infra/                   ❌（必须通过 interface 注入）
infra/      → ai/                      ❌
utility/    → internal/                ❌
任何层      → utility/                 ✅
```

### 已知例外（待清理，不允许新增同类违规）

| 文件 | 违规 | 原因 |
|------|------|------|
| `internal/ai/retriever/retriever.go` | `ai/ → infra/milvus`、`ai/ → milvus-sdk-go` | eino-ext retriever 组件要求 raw Milvus client |
| `internal/ai/indexer/indexer.go` | `ai/ → infra/milvus` | eino-ext indexer 组件要求 raw Milvus client |
| `internal/ai/cmd/knowledge_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
| `internal/ai/cmd/rag_online_eval_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
| `internal/ai/cmd/recall_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
| `utility/logging/logging.go` | `utility/ → internal/consts` | 读取日志级别常量 |
| `utility/tracing/tracing.go` | `utility/ → internal/consts` | 读取 tracing 常量 |
| `utility/middleware/middleware.go` | `utility/ → internal/consts` | 读取中间件常量 |

---

## 4. 当前项目口径

- **项目**：OpsCaption / OpsCaptionAI，模块名 `SuperBizAgent`（go.mod）
- **定位**：面向 AIOps 的智能运维助手，Go 后端为主
- **当前有效主链路**：
  - Chat：`ContextEngine / MemoryService → Eino ReAct Agent → Tools / RAG → JSON / SSE`
  - AIOps：`Approval / Degradation / Memory → Runtime → Plan-Execute-Replan`
- **`chat_multi_agent` 已废弃**，`supervisor/triage/reporter/skillspecialists` 不再作为当前架构依据
- **MemoryService** 封装 `internal/ai/memory`，不直接裸调底层
- **设计口径**以本文件和 `Learn/system/` 下当前文档为准

---

## 5. 提交与验证流程

每次提交前：

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 通过
- [ ] `npm run build` 通过（如有前端改动）
- [ ] 注释聚焦复杂逻辑、边界条件或外部协议
- [ ] 没有创建不必要的新文件
- [ ] commit message 是中文
- [ ] 配置项走 config.yaml，没有硬编码
- [ ] push 前已 `git pull --rebase`

---

## 6. 关键踩坑摘要

完整列表见 [res/agent-coding-retrospective.md](./res/agent-coding-retrospective.md)。

- **不恢复 `chat_multi_agent`** — 已废弃的架构，不要重新接回。
- **Memory 不参与 routing** — 只作为执行上下文，不替代原始 query。
- **RAG 先看 baseline / holdout** — 不能拿全量数据自证效果。
- **不暴露 secrets** — 不要日志记录 API keys、tokens、内部 IP。
- **push 前必须测试 + pull rebase** — CI 会挂，远端更新会冲突。

---

## 7. 必读文档索引

| 文档 | 内容 |
|------|------|
| [系统架构导览](./Learn/system/01-system-architecture-guide.md) | 五层模型、import 规则、主链路、关键模块、设计决策 |
| [代码导读地图](./Learn/system/03-code-map.md) | 技术栈、目录结构、按文件的阅读顺序 |
| [部署手册](./Learn/system/deployment-runbook.md) | 线上环境信息、验证命令 |
| [踩坑规则全集](./res/agent-coding-retrospective.md) | 20 条真实失败案例及对应规则 |
| [历史修改与复盘](./res/todo.md) | 修改记录 |
| [Harness Engineering](./res/harness-engineering-for-opscaptionai.md) | Harness 落地说明 |
| [知识图谱设计](./Learn/graph/00.md) | Case Graph 设计稿 |
