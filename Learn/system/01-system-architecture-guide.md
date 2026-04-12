# OpsCaption 系统学习指南

本文档面向项目负责人，系统性讲解 OpsCaption 的完整架构、数据流、每个模块的职责和它们之间的关系。

---

## 第一章：系统全景图

### 1.1 你的系统是什么

OpsCaption 是一个智能运维助手。它能：

- 接收用户的运维问题（比如"某服务告警了怎么办"）
- 自动判断问题类型
- 派多个 AI Agent 同时去查指标、查日志、查知识库
- 把查到的结果汇总成一份结构化报告

它不是一个简单的聊天机器人。它有两条执行路径：

- **Orchestrator 编排路径**：Supervisor 集中调度多个 Specialist 并行执行，最终由 Reporter 汇总（当前生产模式）
- **ReAct Agent 路径**：单个 Agent 通过 Reasoning + Acting 循环，自主选择工具、多步推理

当前系统采用的是 **Orchestrator 编排模式**，不是学术意义上的 Multi-Agent 协作。Specialist 之间不直接通信，所有控制流由 Supervisor 集中管理。

### 1.2 总体架构图

```
┌──────────────────────────────────────────────────────────────────┐
│                         用户 (浏览器)                             │
│                              │                                   │
│                         HTTP 请求                                │
└──────────────────────────────┬───────────────────────────────────┘
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                      Controller 层                               │
│                                                                  │
│  chat_v1_chat.go ─── 普通聊天                                     │
│  chat_v1_chat_stream.go ─── 流式聊天 (SSE)                        │
│  chat_v1_ai_ops.go ─── AIOps 故障分析                             │
│  chat_v1_admin.go ─── 管理接口                                    │
│  chat_v1_file_upload.go ─── 文件上传（知识库入库）                   │
│                              │                                   │
│                    判断是否走 Orchestrator 编排                      │
└──────────────────────────────┬───────────────────────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │  走 Orchestrator？    │
                    │                     │
                    │  是 ──────────────── ▶ Service 层 (Orchestrator 编排)
                    │  否 ──────────────── ▶ Chat Pipeline (ReAct Agent)
                    └─────────────────────┘

─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─

Orchestrator 编排路径：

┌──────────────────────────────────────────────────────────────────┐
│                      Service 层                                  │
│                                                                  │
│  chat_multi_agent.go ─── Chat 入口                               │
│  ai_ops_service.go ─── AIOps 入口                                │
│  memory_service.go ─── 记忆管理                                   │
│  degradation.go ─── 降级开关                                      │
│  approval_gate.go ─── 审批拦截                                    │
│                              │                                   │
│                    构建 TaskEnvelope                               │
│                    注入 memory_context                             │
│                    调用 Runtime.Dispatch                           │
└──────────────────────────────┬───────────────────────────────────┘
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                      Runtime 层                                  │
│                                                                  │
│  runtime.go ─── 任务分发引擎                                      │
│  registry.go ─── Agent 注册表                                     │
│  ledger.go ─── 任务生命周期记录                                    │
│  bus.go ─── 事件总线                                              │
│  artifacts.go ─── 证据/产物存储                                    │
│                              │                                   │
│                    Dispatch(task) → 找到对应 Agent → 执行           │
└──────────────────────────────┬───────────────────────────────────┘
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                      Agent 层                                    │
│                                                                  │
│  ┌───────────┐    ┌──────────┐    ┌──────────────────────┐       │
│  │ Supervisor │───▶│  Triage  │    │                      │       │
│  │            │    │          │    │    判断任务类型        │       │
│  │  编排者     │    │  路由器   │    │    返回 domains       │       │
│  └─────┬─────┘    └──────────┘    └──────────────────────┘       │
│        │                                                         │
│        │  根据 domains 并行派发                                    │
│        ▼                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │   Metrics     │  │   Logs       │  │  Knowledge   │           │
│  │   Specialist  │  │   Specialist │  │  Specialist  │           │
│  │              │  │              │  │              │           │
│  │ 查 Prometheus │  │ 查 MCP 日志   │  │ 查知识库 RAG  │           │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │
│         │                 │                 │                    │
│         └────────┬────────┘─────────────────┘                    │
│                  ▼                                                │
│  ┌──────────────────────────────────────────────┐                │
│  │               Reporter                       │                │
│  │                                              │                │
│  │  汇总所有 specialist 的结果                    │                │
│  │  生成结构化报告                                │                │
│  │  返回给用户                                    │                │
│  └──────────────────────────────────────────────┘                │
└──────────────────────────────────────────────────────────────────┘

─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─

支撑层：

┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────────┐    │
│  │ RAG 链路     │  │ Context      │  │ Memory 系统           │    │
│  │              │  │ Engine       │  │                      │    │
│  │ Query Rewrite│  │              │  │ Long-term Memory     │    │
│  │ Retriever    │  │ Assembler    │  │ Extraction           │    │
│  │ Rerank       │  │ Budget       │  │ Token Budget         │    │
│  │ (Milvus)     │  │ Policy       │  │                      │    │
│  └─────────────┘  └──────────────┘  └──────────────────────┘    │
│                                                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────────┐    │
│  │ Skills 系统  │  │ Tools 系统    │  │ 基础设施              │    │
│  │              │  │              │  │                      │    │
│  │ Registry     │  │ query_docs   │  │ auth / metrics       │    │
│  │ Match/Run    │  │ query_log    │  │ tracing / health     │    │
│  │ Progressive  │  │ query_alerts │  │ safety / resilience  │    │
│  │ Disclosure   │  │ mysql_crud   │  │ cache / logging      │    │
│  └─────────────┘  └──────────────┘  └──────────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
```

---

## 第二章：一个请求从头到尾怎么走

用最具体的例子讲。

假设用户在浏览器输入：

> "checkoutservice 告警了，CPU 使用率很高，帮我排查一下"

### 步骤 1：请求进入 Controller

浏览器发 HTTP 请求到后端。Controller 收到后：

- 验证 JWT token
- 提取 session_id
- 拿到用户输入的 query

代码位置：internal/controller/chat/chat_v1_chat.go

### 步骤 2：判断走哪条路径

系统看 query 里有没有运维关键词（告警、log、prometheus、故障...）。

如果有 → 走 Orchestrator 编排路径（RunChatMultiAgent → Supervisor 调度多个 Specialist）
如果没有 → 走 ReAct Agent 路径（Chat Pipeline，单 Agent 带工具调用，最多 25 步推理循环）

代码位置：internal/ai/service/chat_multi_agent.go 的 ShouldUseMultiAgentForChat

### 步骤 3：构建 TaskEnvelope

TaskEnvelope 是系统里所有 Agent 通信的"信封"。

它包含：

- task_id：唯一任务 ID
- session_id：用户会话 ID
- trace_id：追踪 ID
- goal：用户的问题
- assignee：指定谁来处理（一开始是 supervisor）
- input：附加信息（memory_context、response_mode 等）
- memory_refs：记忆引用

代码位置：internal/ai/protocol/types.go 的 TaskEnvelope

### 步骤 4：注入记忆上下文

在发给 supervisor 之前，系统先去 Memory Service 查这个用户的历史记忆。

MemoryService 做的事：

- 通过 ContextAssembler 装配上下文
- 按 budget 控制记忆注入量
- 返回 memory_context 文本 + memory_refs 引用

代码位置：internal/ai/service/memory_service.go 的 BuildContextPlan

### 步骤 5：Runtime 分发任务给 Supervisor

Runtime 是整个系统的"任务分发引擎"。

它做的事：

- 在 Ledger 里记录任务
- 根据 task.Assignee 找到对应 Agent
- 调用 Agent.Handle(ctx, task)
- 记录结果

代码位置：internal/ai/runtime/runtime.go 的 Dispatch

### 步骤 6：Supervisor 编排

Supervisor 是"总指挥"。它做三件事：

**第一件：调 Triage 做分类**

把用户 query 发给 Triage Agent。Triage 用规则表匹配关键词，判断这个问题属于什么类型：

- 告警分析 → domains: [metrics, logs, knowledge]
- 知识检索 → domains: [knowledge]
- 故障排查 → domains: [logs, knowledge]

代码位置：internal/ai/agent/triage/triage.go

**第二件：并行派发 Specialists**

根据 Triage 返回的 domains，Supervisor 同时派出对应的 Specialist：

- metrics domain → Metrics Specialist → 查 Prometheus 告警
- logs domain → Logs Specialist → 查 MCP 日志
- knowledge domain → Knowledge Specialist → 查知识库 (RAG)

三个 Specialist 用 goroutine 并行执行。

代码位置：internal/ai/agent/supervisor/supervisor.go 第 74-112 行

**第三件：汇总结果给 Reporter**

所有 Specialist 执行完后，Supervisor 把所有结果打包发给 Reporter。

### 步骤 7：Specialists 各自执行

**Metrics Specialist**

- 调用 query_prometheus_alerts 工具
- 查当前活跃的 Prometheus 告警
- 返回告警摘要

代码位置：internal/ai/agent/skillspecialists/metrics/agent.go

**Logs Specialist**

- 调用 query_log 工具
- 查 MCP 日志
- 返回相关日志条目

代码位置：internal/ai/agent/skillspecialists/logs/agent.go

**Knowledge Specialist**

- 使用 Skills Registry 选择合适的 skill
- 调用 query_internal_docs 工具
- 通过 RAG 链路查知识库
- 返回相关文档片段

代码位置：internal/ai/agent/skillspecialists/knowledge/agent.go

### 步骤 8：Reporter 生成报告

Reporter 拿到所有 Specialist 的结果后：

- 通过 ContextAssembler 装配完整上下文
- 生成结构化 Markdown 报告（包含各 Specialist 的摘要、证据、结论）
- 如果是 chat 模式，调用 LLM 生成自然语言回答

代码位置：internal/ai/agent/reporter/reporter.go

### 步骤 9：结果返回用户

最终结果通过 HTTP 或 SSE 返回给浏览器。

---

## 第三章：统一协议 — 所有 Agent 说同一种语言

系统里所有 Agent 之间通信用的都是同一套数据结构。

### 3.1 TaskEnvelope — 任务信封

就像邮件一样：

- task_id：这封信的编号
- parent_task_id：这封信是回复哪封信的
- session_id：这次对话的编号
- trace_id：追踪链 ID
- goal：任务目标（用户的问题）
- assignee：交给谁处理
- input：附带的额外信息
- status：当前状态（pending → running → succeeded/failed）

### 3.2 TaskResult — 任务结果

Agent 执行完后返回的结果：

- status：成功 / 失败 / 降级
- summary：摘要文本
- confidence：置信度（0~1）
- evidence：证据列表
- degradation_reason：如果降级了，原因是什么

### 3.3 TaskEvent — 事件

Agent 执行过程中发出的事件，比如：

- "supervisor 开始编排"
- "triage 判定为告警分析"
- "knowledge specialist 检索到 3 条文档"

### 3.4 EvidenceItem — 证据

每条证据包含：

- source_type：来源类型（prometheus / log / knowledge）
- title：标题
- snippet：内容片段
- score：相关性分数

### 3.5 ArtifactRef — 产物引用

Agent 执行产生的产物（比如 specialist 的完整结果），存在 ArtifactStore 里，用 ref 引用。

代码位置：internal/ai/protocol/types.go

---

## 第四章：Runtime — 任务分发引擎

Runtime 是整个系统的"骨架"。所有 Agent 的执行都通过 Runtime 调度。

### 4.1 核心组件

```
Runtime
├── Registry — Agent 注册表（记录谁能做什么）
├── Ledger — 任务账本（记录任务生命周期）
├── Bus — 事件总线（Agent 之间的事件通知）
└── ArtifactStore — 产物仓库（存储 Agent 执行产物）
```

### 4.2 Dispatch 流程

```
Dispatch(task)
│
├── 1. 给 task 分配 ID（如果没有）
├── 2. 在 Ledger 里创建任务记录
├── 3. 在 Registry 里找到 task.Assignee 对应的 Agent
├── 4. 更新状态为 running
├── 5. 调用 Agent.Handle(ctx, task)
├── 6. 更新状态为 succeeded / failed
├── 7. 在 Ledger 里记录结果
└── 8. 返回 TaskResult
```

### 4.3 持久化

Runtime 支持两种存储模式：

- InMemory（默认）— 数据在内存里，进程退出就没了
- Persistent（可选）— 数据写文件，可以回放

代码位置：internal/ai/runtime/

---

## 第五章：RAG 链路 — 知识检索的核心

RAG = Retrieval-Augmented Generation（检索增强生成）

### 5.1 RAG 做什么

当 Knowledge Specialist 需要查知识库时，走的就是 RAG 链路：

```
用户 query
    │
    ▼
Query Rewrite（重写查询，让搜索更精准）
    │
    ▼
Retriever（去 Milvus 向量库搜索相似文档）
    │
    ▼
Rerank（用 LLM 对结果重新排序）
    │
    ▼
返回 top-K 个最相关文档
```

### 5.2 Query Rewrite

把用户口语化的问题改写成更适合搜索的形式。

比如：

- 用户说："服务挂了怎么办"
- 改写成："服务故障排查方法 pod failure recovery"

用 LLM 做改写，有 3 秒超时保护，失败则用原始 query。

代码位置：internal/ai/rag/query_rewrite.go

### 5.3 RetrieverPool

不是每次查询都新建一个 Milvus 连接，而是复用。

RetrieverPool 按 Milvus 地址 + top_k 做缓存 key，失败也会短暂缓存避免雪崩。

代码位置：internal/ai/rag/retriever_pool.go

### 5.4 Rerank

Milvus 返回的结果可能排序不够好。用 LLM 对每个文档打相关性分（0-10），然后重新排序。

有 5 秒超时保护，失败则用原始排序。

代码位置：internal/ai/rag/rerank.go

### 5.5 三种查询模式

- retrieve：只检索，不改写不重排
- rewrite：改写 + 检索，不重排
- full：改写 + 检索 + 重排（默认）

代码位置：internal/ai/rag/query.go

---

## 第六章：Context Engine — 上下文工程

### 6.1 为什么需要 Context Engine

LLM 有 token 上限。不能把所有信息都塞进去。

Context Engine 的职责是：按需、按预算，组装最合适的上下文。

### 6.2 四类上下文来源

```
ContextPackage
├── HistoryMessages — 对话历史
├── MemoryItems — 长期记忆
├── DocumentItems — RAG 检索结果
└── ToolItems — 工具调用结果
```

### 6.3 Budget 机制

每类来源都有 token 预算：

```
ContextBudget
├── HistoryTokens: 2000
├── MemoryTokens: 500
├── DocumentTokens: 3000
├── ToolTokens: 2000
└── ReservedTokens: 500
```

如果某类来源超预算，会自动裁剪。

### 6.4 Profile 机制

不同场景用不同的上下文策略：

- chat 模式：允许 history + memory + docs
- aiops 模式：允许 memory + docs + tools
- report 模式：允许 tools，不允许 history

代码位置：internal/ai/contextengine/

---

## 第七章：Skills 系统 — 让 Specialist 更灵活

### 7.1 Skill 是什么

Skill 是一个"可插拔的能力单元"。

每个 Skill 实现三个方法：

- Name() → 我叫什么
- Match(task) → 这个任务我能不能处理
- Run(ctx, task) → 执行

### 7.2 Registry 是什么

Registry 管理一组 Skill。当任务来了，Registry 按顺序匹配：

- 第一个匹配成功的 Skill 来执行
- 如果都不匹配，返回错误

### 7.3 Knowledge Specialist 的 Skills

Knowledge Specialist 内置了多个 skill，比如：

- 告警知识检索
- SOP/Runbook 检索
- 通用知识检索

根据任务类型自动选择最合适的 skill。

代码位置：internal/ai/skills/

---

## 第八章：Memory 系统 — 让系统有记忆

### 8.1 三层记忆

```
Memory
├── 短期记忆 — 当前对话历史（HistoryMessages）
├── 长期记忆 — 跨会话的重要信息（LongTermMemory）
└── 工作记忆 — 当前任务的工具输出（ToolItems）
```

### 8.2 长期记忆怎么产生

每次对话结束后，系统用 LLM 从对话内容中提取值得记住的信息。

比如：

- 用户偏好
- 已确认的系统配置
- 历史排查结论

提取过程有超时保护和内容校验。

### 8.3 长期记忆怎么用

下次用户来问时，系统先检索相关的长期记忆，注入到上下文里。

这样系统就能"记住"上次的对话。

代码位置：utility/mem/ + internal/ai/service/memory_service.go

---

## 第九章：Tools — Agent 的工具箱

### 9.1 可用工具

| 工具 | 功能 | 代码位置 |
|---|---|---|
| query_prometheus_alerts | 查 Prometheus 活跃告警 | tools/query_metrics_alerts.go |
| query_log | 查 MCP 日志 | tools/query_log.go |
| query_internal_docs | 查知识库（RAG） | tools/query_internal_docs.go |
| mysql_crud | 查 MySQL 数据 | tools/mysql_crud.go |
| get_current_time | 获取当前时间 | tools/get_current_time.go |

### 9.2 工具白名单

每个 Specialist 只能用自己的工具：

- Metrics Specialist → query_prometheus_alerts
- Logs Specialist → query_log
- Knowledge Specialist → query_internal_docs

不是所有 Agent 共享一个大工具池。

代码位置：internal/ai/tools/

---

## 第十章：安全与韧性

### 10.1 认证与授权

- JWT Token 认证
- Rate Limiter（本地 + Redis 两种）
- Token 审计

代码位置：utility/auth/

### 10.2 输入安全

- Prompt Guard：过滤危险输入
- Output Filter：过滤敏感输出

代码位置：utility/safety/

### 10.3 降级机制

系统支持手动降级开关。当 AI 服务不可用时，可以通过配置关闭 AI 能力，返回友好提示。

代码位置：internal/ai/service/degradation.go

### 10.4 韧性

- Resilience 工具包：重试、限流
- Semaphore：并发控制

代码位置：utility/resilience/

### 10.5 审批门

高风险操作（比如写数据库）可以走审批流程。

代码位置：internal/ai/service/approval_gate.go

---

## 第十一章：可观测性

### 11.1 Metrics

Prometheus 指标收集。

代码位置：utility/metrics/metrics.go

### 11.2 Tracing

OpenTelemetry 分布式追踪。

代码位置：utility/tracing/tracing.go

### 11.3 Health Check

/healthz 和 /readyz 端点。

代码位置：utility/health/health.go

### 11.4 Logging

结构化日志。

代码位置：utility/logging/logging.go

---

## 第十二章：RAG Eval — 评测系统

### 12.1 评测做什么

验证 RAG 检索能力：给一个问题，系统能不能找到正确的文档。

### 12.2 核心概念

- EvalCase：一个评测样本（包含 query + 期望结果）
- Recall@K：前 K 个结果中，正确文档被找到的比例
- Hit@K：前 K 个结果中，是否包含至少一个正确文档
- Build Split / Holdout Split：训练集 / 测试集

### 12.3 AIOps Baseline

专门为 AIOps 故障数据集设计的 baseline 生成器：

- 读取 groundtruth.jsonl
- 生成 evidence docs 和 history docs
- 划分 build / holdout split
- 生成 eval cases

代码位置：internal/ai/rag/eval/

---

## 第十三章：数据预处理 Pipeline

### 13.1 本地预处理

Python 脚本从 parquet 中提取异常信号摘要。

- 指标异常：基线对比法
- 日志异常：模式归一化 + 信号评分
- 调用链异常：错误率 + 延迟统计

代码位置：scripts/aiops/build_telemetry_evidence.py

### 13.2 本地 → 远程 Pipeline

PowerShell 脚本编排：

本地预处理 → 打包 → SSH 上传 → 远程索引 → 远程评测

代码位置：scripts/aiops/run_telemetry_local_then_remote.ps1

### 13.3 远程评测

Bash 脚本在云端 Docker 里运行：

启动 Milvus → Go prep → 索引入库 → 运行 eval runner

代码位置：scripts/aiops/run_telemetry_baseline_remote.sh

---

## 第十四章：模块依赖关系图

```
main.go
  │
  ├── Controller 层
  │     └── chat_v1_*.go
  │
  ├── Service 层
  │     ├── chat_multi_agent.go ──▶ Runtime + Supervisor
  │     ├── ai_ops_service.go ──▶ Runtime + Supervisor
  │     ├── memory_service.go ──▶ ContextEngine + mem
  │     ├── degradation.go
  │     └── approval_gate.go
  │
  ├── Agent 层
  │     ├── supervisor ──▶ triage + specialists + reporter
  │     ├── triage ──▶ 规则表
  │     ├── skillspecialists/metrics ──▶ tools/query_metrics_alerts
  │     ├── skillspecialists/logs ──▶ tools/query_log
  │     ├── skillspecialists/knowledge ──▶ skills/registry + tools/query_internal_docs
  │     └── reporter ──▶ contextengine
  │
  ├── 支撑层
  │     ├── runtime ──▶ protocol + ledger + bus + artifacts
  │     ├── rag ──▶ query_rewrite + retriever_pool + rerank + milvus
  │     ├── contextengine ──▶ assembler + resolver + budget
  │     ├── skills ──▶ registry + progressive_disclosure
  │     └── tools ──▶ prometheus + mcp + milvus + mysql
  │
  └── 基础设施层
        ├── auth (jwt + rate_limiter)
        ├── mem (long_term + extraction + token_budget)
        ├── metrics + tracing + health + logging
        ├── safety (prompt_guard + output_filter)
        ├── resilience (retry + semaphore)
        └── cache (llm_cache)
```

---

## 第十五章：学习路线建议

### 第一周：理解主链路

1. 读 protocol/types.go — 理解 TaskEnvelope 和 TaskResult
2. 读 runtime/runtime.go — 理解 Dispatch 流程
3. 读 supervisor/supervisor.go — 理解编排流程
4. 读 triage/triage.go — 理解路由规则
5. 读 reporter/reporter.go — 理解报告生成

### 第二周：理解支撑系统

1. 读 rag/query.go — 理解 RAG 链路
2. 读 contextengine/assembler.go — 理解上下文工程
3. 读 service/memory_service.go — 理解记忆系统
4. 读 skills/registry.go — 理解 Skills 系统

### 第三周：理解评测和预处理

1. 读 rag/eval/runner.go — 理解评测流程
2. 读 rag/eval/aiops_baseline.go — 理解 baseline 生成
3. 读 scripts/aiops/build_telemetry_evidence.py — 理解数据预处理
4. 读 scripts/aiops/run_telemetry_local_then_remote.ps1 — 理解全链路 pipeline

### 第四周：理解安全和韧性

1. 读 utility/auth/ — 认证和限流
2. 读 utility/safety/ — 输入输出安全
3. 读 service/degradation.go — 降级机制
4. 读 service/approval_gate.go — 审批机制