# OpsCaption 当前系统架构导览

> 更新时间：2026-05-02
> 适用范围：当前主干代码
> 结论先行：**Chat 已统一收敛为 ReAct 单链路；AIOps 保留 runtime 包装，但执行核心是 Plan-Execute-Replan。**

---

## 1. 一句话看懂项目

OpsCaption 是一个面向运维场景的智能助手后端，主要解决：

- 告警分析
- 日志 / 指标查询
- 知识库检索
- 多轮运维问答
- 复杂问题的规划式排查

它现在有两条真正生效的主链路：

1. **Chat 链路**：普通问答、日志/指标/知识库查询，走 `ReAct Agent`
2. **AIOps 链路**：复杂分析、需要先计划再执行的问题，走 `Plan-Execute-Replan`

---

## 2. 当前架构总览

```text
Client
  │
  ├── /chat
  ├── /chat_stream
  └── /ai_ops
       │
       ▼
Controller / Service
       │
       ├── 安全检查 / 降级 / 限流 / 审批
       ├── MemoryService
       ├── ContextEngine
       ├── Skills / ProgressiveDisclosure
       └── RAG / Tools / Runtime
```

### 2.1 Chat 路径

```text
chat_v1_chat.go / chat_v1_chat_stream.go
    ↓
MemoryService.BuildChatPackage
    ↓
ContextEngine 组装 history / memory / docs
    ↓
chat_pipeline.BuildChatAgentWithQuery
    ↓
Eino ReAct Agent
    ↓
Tools / RAG / 输出过滤 / 记忆持久化
```

特点：

- 单个 ReAct Agent 自主决定是否调用工具
- ProgressiveDisclosure 控制工具暴露范围
- 支持普通回答和 SSE 流式回答

### 2.2 AIOps 路径

```text
ai_ops_service.go
    ↓
Approval / Degradation / Memory
    ↓
Runtime.Dispatch(rootTask)
    ↓
aiops_plan_execute_replan
    ↓
Planner → Executor → RePlanner
    ↓
结果输出 + trace + persist
```

特点：

- 保留 runtime 外壳和 trace 能力
- 核心执行模式是 Plan-Execute-Replan
- 适合复杂运维分析，不适合简单问答

---

## 3. 工程分层模型

当前代码采用五层架构：

```text
┌─────────────────────────────────────────────────┐
│              API Layer                           │
│         internal/controller/                     │
│  参数解析 → 调用 Application → 格式化响应         │
├─────────────────────────────────────────────────┤
│           Application Layer                      │
│         internal/app/                            │
│  编排业务流程：ChatApp、AIOpsApp、KnowledgeApp    │
├─────────────────────────────────────────────────┤
│             Domain Layer                         │
│    internal/ai/{rag,contextengine,memory,...}     │
│  核心业务规则：检索、推理、证据、Agent             │
├─────────────────────────────────────────────────┤
│        Infrastructure Layer                      │
│    internal/infra/milvus/                        │
│  外部系统适配：Milvus SDK、RabbitMQ、Redis        │
├─────────────────────────────────────────────────┤
│            Common Layer                          │
│         utility/{auth,safety,health,...}          │
│  通用横切关注点：认证、限流、安全、健康检查        │
└─────────────────────────────────────────────────┘
```

### import 规则（目标态）

```text
controller/ → app/                     ✅
app/        → ai/                      ✅
app/        → infra/                   ✅（通过 interface）
ai/         → infra/                   ❌（必须通过 interface 注入）
infra/      → ai/                      ❌
utility/    → internal/                ❌
任何层      → utility/                 ✅
```

关键约束：

- `internal/ai/rag/` 不得 import `internal/infra/milvus`、`milvus-sdk-go`、`utility/client`。VectorStore 和 RetrieverFactory 通过 main.go 注入。✅ 已强制（import guard test 覆盖）
- `internal/infra/milvus/` 是 Milvus SDK 的唯一适配点。其他包不得直接 import `milvus-sdk-go`。
- `utility/` 不得 import `internal/` 下任何包。Milvus 健康检查和关闭通过 main.go 注入函数变量。

### 已知例外（待清理）

以下文件违反目标规则，已登记为例外。修改这些文件时应优先消除违规。

| 文件 | 违规 | 原因 |
|------|------|------|
| `internal/ai/retriever/retriever.go` | `ai/ → infra/milvus`、`ai/ → milvus-sdk-go` | eino-ext retriever 组件要求 raw Milvus client |
| `internal/ai/indexer/indexer.go` | `ai/ → infra/milvus` | eino-ext indexer 组件要求 raw Milvus client |
| `internal/ai/cmd/knowledge_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
| `utility/health/health.go` | `utility/ → ai/tools` | MySQL 健康检查依赖 tools.CloseMySQL |
| `utility/safety/injection_classifier.go` | `utility/ → ai/models` | 调用 models.TokenCount |
| `utility/logging/logging.go` | `utility/ → internal/consts` | 读取日志级别常量 |
| `utility/tracing/tracing.go` | `utility/ → internal/consts` | 读取 tracing 常量 |
| `utility/middleware/middleware.go` | `utility/ → internal/consts` | 读取中间件常量 |

---

## 4. 关键模块

### 4.1 Controller 层

- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`

职责：

- 接口入口
- request/session 上下文注入
- prompt guard
- 降级判断
- SSE 输出编排

### 4.2 Chat Pipeline

- `internal/ai/agent/chat_pipeline/orchestration.go`
- `internal/ai/agent/chat_pipeline/flow.go`
- `internal/ai/agent/chat_pipeline/prompt.go`

职责：

- 构建 ReAct Agent
- 配置模型与工具
- 动态裁剪工具暴露范围
- 组织 prompt 规则

### 4.3 AIOps Runtime

- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/ai_ops_runtime.go`
- `internal/ai/agent/plan_execute_replan/`

职责：

- 审批 / 降级 / session 记忆接入
- runtime dispatch
- Plan-Execute-Replan 规划分析

### 4.4 ContextEngine

- `internal/ai/contextengine/`

职责：

- 装配 `history / memory / docs / tool outputs`
- 做 token budget 控制
- 输出 context trace

### 4.5 MemoryService

- `internal/ai/service/memory_service.go`
- `internal/ai/memory/`

职责：

- 会话上下文拼装
- 结果持久化
- 长期记忆抽取与过滤

### 4.6 RAG

- `internal/ai/rag/`
- `internal/ai/retriever/`

职责：

- query rewrite
- 向量检索
- rerank
- evidence assembly

---

## 5. 已不再作为当前架构依据的内容

以下目录仍可能留在仓库中，但**不代表当前聊天主链路设计**：

- `internal/ai/agent/supervisor/`
- `internal/ai/agent/triage/`
- `internal/ai/agent/reporter/`
- `internal/ai/agent/skillspecialists/`

它们更适合作为：

- 历史实验代码
- 评测 / 合约 / harness 研究材料
- 架构演进复盘参考

**当前聊天入口已经移除了 `chat_multi_agent` 条件路由。**

---

## 6. 学习顺序建议

如果你现在要系统学习这个项目，推荐顺序是：

1. 先读 controller：`chat_v1_chat.go`、`chat_v1_chat_stream.go`
2. 再看 `MemoryService` 和 `ContextEngine`
3. 再看 `chat_pipeline/`，理解 ReAct 是怎么装起来的
4. 再看 `ai_ops_service.go` + `plan_execute_replan/`
5. 最后看 `rag/`、`skills/`、`internal/ai/memory/`

---

## 7. 面试口径

现在最稳的讲法是：

> 我们把聊天链路统一收敛到了 ReAct Agent，让模型按需调用 Prometheus、日志、知识库等工具；而复杂运维分析场景则通过 runtime 包装，执行核心收敛为 Plan-Execute-Replan，用规划、执行、复核三段式来完成多步排查。

这比说“现在还在跑 supervisor/triage/reporter 聊天编排”更准确。
