# OpsCaption 代码导读地图

> 当前统一口径（2026-05）
> - Chat 主链路：`ContextEngine / MemoryService -> Eino ReAct Agent -> Tools / RAG -> JSON / SSE`
> - AIOps 主链路：`Approval / Degradation / Memory -> Runtime -> Plan-Execute-Replan`
> - `supervisor / triage / reporter / skillspecialists / chat_multi_agent` 相关内容，如果仍出现在仓库其他文档里，应理解为历史实验或演进讨论，不再代表当前聊天主链路。

本文档只列出**当前最值得读、且会直接影响线上行为**的代码入口，方便你快速建立阅读顺序。

---

## 1. 先看入口

| 文件 | 作用 | 为什么先看 |
|---|---|---|
| `main.go` | 服务启动、配置加载、中间件、路由注册 | 先搞清楚系统怎么启动 |
| `manifest/config/config.yaml` | 全局配置 | 看 feature flags、模型、RAG、memory、runtime 参数 |
| `internal/controller/chat/chat_v1_chat.go` | 普通聊天入口 | 当前 Chat 主链路 |
| `internal/controller/chat/chat_v1_chat_stream.go` | 流式聊天入口 | 当前 SSE 主链路 |
| `internal/controller/chat/chat_v1_ai_ops.go` | AIOps 入口 | 复杂分析链路入口 |

---

## 2. Chat 主链路

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `internal/controller/chat/chat_v1_chat.go` | Chat Controller | 校验、降级、缓存、session lock、构建上下文、执行 ReAct、持久化结果 |
| `internal/controller/chat/chat_v1_chat_stream.go` | ChatStream Controller | 流式版本的 Chat，负责 SSE 事件输出和 fallback |
| `internal/ai/service/memory_service.go` | 记忆服务 | 构建 ChatPackage、持久化对话结果、异步触发记忆抽取 |
| `internal/ai/contextengine/assembler.go` | 上下文装配器 | 把 history / memory / docs / tool outputs 组装成可供模型消费的上下文包 |
| `internal/ai/contextengine/resolver.go` | Profile 解析 | 根据 mode 选择 ContextProfile 和预算策略 |
| `internal/ai/agent/chat_pipeline/orchestration.go` | ReAct 执行装配 | 构建 Chat ReAct 执行器 |
| `internal/ai/agent/chat_pipeline/flow.go` | ReAct 行为配置 | 绑定工具、技能披露、回调等 |
| `internal/ai/agent/chat_pipeline/prompt.go` | Chat Prompt | 系统规则、上下文提示、轻社交约束 |

---

## 3. AIOps 主链路

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `internal/controller/chat/chat_v1_ai_ops.go` | AIOps Controller | AIOps 请求入口，负责参数校验和响应封装 |
| `internal/ai/service/ai_ops_service.go` | AIOps Service | AIOps 主服务，负责审批、降级、runtime 调度 |
| `internal/ai/service/ai_ops_runtime.go` | Runtime 复用 | 按 dataDir 复用 AIOps runtime，避免每次重建 |
| `internal/ai/agent/plan_execute_replan/plan_execute_replan.go` | AIOps 执行骨架 | Plan-Execute-Replan 的主入口 |
| `internal/ai/agent/plan_execute_replan/planner.go` | Planner | 拆计划 |
| `internal/ai/agent/plan_execute_replan/executor.go` | Executor | 按计划调工具 |
| `internal/ai/agent/plan_execute_replan/replan.go` | RePlanner | 结果不够时调整计划 |

---

## 4. Context / RAG / Skills

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `internal/ai/contextengine/types.go` | 上下文协议 | ContextRequest / ContextPackage / Budget / Profile |
| `internal/ai/contextengine/documents.go` | 文档上下文 | 把 RAG 检索结果转成上下文项 |
| `internal/ai/contextengine/tool_items.go` | 工具结果上下文 | 把 tool outputs 转成上下文项 |
| `internal/ai/rag/query.go` | RAG 主入口 | Query -> Rewrite -> Retrieve -> Rerank |
| `internal/ai/rag/query_rewrite.go` | Query Rewrite | 口语问题转检索式表达 |
| `internal/ai/rag/rerank.go` | Rerank | 结果重排 |
| `internal/ai/rag/retriever_pool.go` | Milvus 连接池 | 复用 retriever，避免每次重建 |
| `internal/ai/skills/registry.go` | Skills 注册表 | 注册、解析、归一化 skill |
| `internal/ai/skills/progressive_disclosure.go` | 渐进披露 | 根据 query 和 selected skills 决定暴露哪些工具 |

---

## 5. Tools 与外部依赖

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `internal/ai/tools/query_metrics_alerts.go` | 告警查询 | Prometheus 告警/指标 |
| `internal/ai/tools/query_log.go` | 日志查询 | 日志平台/MCP 日志 |
| `internal/ai/tools/query_internal_docs.go` | 知识检索工具 | 走 RAG 查内部文档 |
| `internal/ai/tools/mysql_crud.go` | 数据库查询 | 只读 SQL 能力 |
| `internal/ai/tools/get_current_time.go` | 时间工具 | 获取当前时间 |
| `internal/ai/tools/tiered_tools.go` | 工具分层 | 管理工具分级和暴露边界 |

---

## 6. Memory / Cache / MQ

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `utility/mem/mem.go` | 短期记忆 | 会话窗口、摘要、working state |
| `utility/mem/long_term.go` | 长期记忆 | scope、衰减、检索、上限控制 |
| `utility/mem/extraction.go` | 记忆提取 | 从用户明确陈述中抽候选记忆并做过滤 |
| `utility/cache/llm_cache.go` | 回答缓存 | 按 session + query + skill scope 做缓存 |
| `internal/ai/service/chat_task_queue.go` | Chat 异步任务 | 聊天异步执行 |
| `internal/ai/service/memory_queue.go` | 记忆异步任务 | 记忆抽取异步执行 |

---

## 7. 安全与稳定性

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| `internal/ai/service/degradation.go` | 降级决策 | 系统降级开关 |
| `internal/ai/service/approval_gate.go` | 审批门 | 高风险请求审批 |
| `internal/ai/service/token_audit.go` | Token 审计 | 记录成本与配额 |
| `utility/safety/prompt_guard.go` | 输入安全 | 拦截危险 prompt |
| `utility/safety/output_filter.go` | 输出安全 | 过滤敏感或违规输出 |
| `utility/auth/rate_limiter.go` | 限流 | 本地令牌桶 |
| `internal/logic/sse/sse.go` | SSE 封装 | 统一流式响应头和事件写出 |

---

## 8. 读代码的建议顺序

1. `main.go`
2. `internal/controller/chat/chat_v1_chat.go`
3. `internal/ai/service/memory_service.go`
4. `internal/ai/contextengine/assembler.go`
5. `internal/ai/agent/chat_pipeline/`
6. `internal/controller/chat/chat_v1_ai_ops.go`
7. `internal/ai/service/ai_ops_service.go`
8. `internal/ai/agent/plan_execute_replan/`
9. `internal/ai/rag/`
10. `utility/mem/`、`utility/cache/`、`internal/ai/service/*queue.go`

---

## 9. 哪些目录属于历史/实验材料

下面这些目录仍然有参考价值，但**不要再把它们当成当前聊天主链路**：

- `internal/ai/agent/supervisor/`
- `internal/ai/agent/triage/`
- `internal/ai/agent/reporter/`
- `internal/ai/agent/skillspecialists/`

它们更适合用来理解：

- 你们曾经尝试过什么 Orchestrator / Multi-Agent 方案
- 为什么后来把 Chat 收敛到 ReAct 单链路
- 为什么 AIOps 最终保留 runtime 外壳，但执行核心切到 Plan-Execute-Replan
