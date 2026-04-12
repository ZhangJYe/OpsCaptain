# OpsCaption 数据流详解

本文档详细讲解系统中每一类数据是怎么流转的。

---

## 第一章：用户请求的数据流

### 1.1 完整数据流

```
浏览器 POST /api/v1/chat
│
│  body: { "prompt": "checkoutservice CPU 告警", "session_id": "abc123" }
│
▼
Controller (chat_v1_chat.go)
│
│  提取 query, session_id
│  JWT 验证
│  Rate Limit 检查
│
▼
ShouldUseMultiAgentForChat(query)
│
│  query 包含 "告警" → true
│
▼
RunChatMultiAgent(ctx, sessionID, query)
│
│  ┌─ 检查降级开关 (degradation.go)
│  │  如果降级 → 直接返回降级响应
│  │
│  ├─ 获取/创建 Runtime (getOrCreateChatRuntime)
│  │  按 dataDir 复用 Runtime，不每次重建
│  │
│  ├─ 构建记忆上下文 (memory_service.go)
│  │  │
│  │  │  ContextAssembler.Assemble()
│  │  │  │
│  │  │  ├─ PolicyResolver 选择 Profile
│  │  │  ├─ 按 Budget 选择 History
│  │  │  ├─ 从 LongTermMemory 检索记忆
│  │  │  └─ 返回 ContextPackage
│  │  │
│  │  └─ 输出: memory_context (文本) + memory_refs (引用)
│  │
│  ├─ 构建 TaskEnvelope
│  │  │
│  │  │  goal = "checkoutservice CPU 告警"
│  │  │  assignee = "supervisor"
│  │  │  input = {
│  │  │    raw_query: "checkoutservice CPU 告警",
│  │  │    memory_context: "上次讨论过 checkoutservice 的部署配置...",
│  │  │    response_mode: "chat",
│  │  │    entrypoint: "chat"
│  │  │  }
│  │  │
│  │  └─ 输出: TaskEnvelope
│  │
│  └─ Runtime.Dispatch(task)
│     │
│     ▼
│  Supervisor.Handle(ctx, task)
│     │
│     ├─ 1. Dispatch → Triage
│     │     │
│     │     │  输入: goal = "checkoutservice CPU 告警"
│     │     │  匹配规则: "告警" → alert_analysis
│     │     │  输出: intent = "alert_analysis"
│     │     │        domains = ["metrics", "logs", "knowledge"]
│     │     │
│     │     └─ TaskResult { metadata: { intent, domains } }
│     │
│     ├─ 2. 并行 Dispatch → 3 个 Specialists
│     │     │
│     │     │  每个 specialist 收到的 task:
│     │     │    goal = "checkoutservice CPU 告警\n\n可参考的历史上下文：..."
│     │     │    input.raw_query = "checkoutservice CPU 告警"
│     │     │    input.memory_context = "..."
│     │     │    input.intent = "alert_analysis"
│     │     │
│     │     ├─ [goroutine 1] Metrics Specialist
│     │     │     │
│     │     │     │  调用 query_prometheus_alerts
│     │     │     │  返回: 活跃告警列表
│     │     │     │
│     │     │     └─ TaskResult {
│     │     │          status: succeeded,
│     │     │          summary: "发现 2 条活跃告警: CPUThrottling...",
│     │     │          evidence: [{ source_type: "prometheus", ... }]
│     │     │        }
│     │     │
│     │     ├─ [goroutine 2] Logs Specialist
│     │     │     │
│     │     │     │  调用 query_log
│     │     │     │  返回: 相关日志条目
│     │     │     │
│     │     │     └─ TaskResult {
│     │     │          status: succeeded,
│     │     │          summary: "发现 context canceled 错误日志...",
│     │     │          evidence: [{ source_type: "log", ... }]
│     │     │        }
│     │     │
│     │     └─ [goroutine 3] Knowledge Specialist
│     │           │
│     │           │  Skills Registry 选择 skill
│     │           │  调用 query_internal_docs
│     │           │  │
│     │           │  │  RAG 链路:
│     │           │  │  query → Rewrite → Retrieve (Milvus) → Rerank
│     │           │  │
│     │           │  返回: 相关知识文档
│     │           │
│     │           └─ TaskResult {
│     │                status: succeeded,
│     │                summary: "找到 3 篇相关文档...",
│     │                evidence: [{ source_type: "knowledge", ... }]
│     │              }
│     │
│     ├─ 3. wg.Wait() — 等待所有 specialist 完成
│     │
│     ├─ 4. Dispatch → Reporter
│     │     │
│     │     │  输入: query + intent + 3 个 specialist 的 results
│     │     │
│     │     │  Reporter 做的事:
│     │     │  ├─ 装配上下文 (ContextAssembler)
│     │     │  ├─ 如果 chat 模式 → 调 LLM 生成自然语言回答
│     │     │  └─ 如果 report 模式 → 生成 Markdown 报告
│     │     │
│     │     └─ TaskResult {
│     │          status: succeeded,
│     │          summary: "综合分析: checkoutservice CPU 使用率过高...",
│     │          evidence: [合并所有 specialist 的 evidence]
│     │        }
│     │
│     └─ 返回最终 TaskResult
│
▼
RunChatMultiAgent 收到结果
│
│  提取 summary, evidence, detail, trace_id
│  异步持久化记忆 (PersistOutcome)
│
▼
Controller 返回 HTTP 响应给浏览器
│
│  body: {
│    "content": "综合分析: checkoutservice CPU 使用率过高...",
│    "detail": ["triage: alert_analysis", "metrics: 2 alerts", ...],
│    "trace_id": "xxx"
│  }
```

---

## 第二章：RAG 检索的数据流

### 2.1 知识入库流程

```
用户上传文件 (chat_v1_file_upload.go)
│
▼
Knowledge Index Pipeline (knowledge_index_pipeline/)
│
├─ Loader: 读取文件内容 (Markdown/PDF/...)
├─ Transformer: 切分文档 (按标题/段落/固定长度)
├─ Embedder: 调 Doubao Embedding 生成向量
└─ Indexer: 写入 Milvus
│
▼
Milvus Collection
│
│  每条记录:
│  ├─ id: 文档片段 ID
│  ├─ content: 文档文本
│  ├─ metadata: 来源信息
│  └─ vector: 1536 维向量
```

### 2.2 知识检索流程

```
Knowledge Specialist 调用 query_internal_docs(query)
│
▼
query_internal_docs 工具
│
├─ 获取 SharedRetrieverPool（全局复用）
│
▼
rag.Query(ctx, pool, query)
│
├─ 1. Query Rewrite
│     │
│     │  调用 DeepSeek V3
│     │  prompt: "你是搜索优化器，把问题改写成关键词丰富的搜索查询"
│     │  "checkoutservice CPU 告警" → "checkoutservice CPU 高使用率 告警 pod throttling"
│     │  超时 3 秒，失败用原始 query
│     │
│     └─ 输出: rewritten_query
│
├─ 2. Retrieve
│     │
│     │  RetrieverPool.GetOrCreate()
│     │  │  按 Milvus 地址 + top_k 做缓存
│     │  │  如果缓存命中 → 复用
│     │  │  如果缓存未命中 → 新建 Milvus Retriever
│     │  │
│     │  Milvus Retriever:
│     │  │  把 rewritten_query 用 Doubao Embedding 转成向量
│     │  │  在 Milvus 里做 ANN (近似最近邻) 搜索
│     │  │  返回 top_k 个最相似的文档片段
│     │  │
│     │  └─ 输出: []*schema.Document (raw results)
│
├─ 3. Rerank
│     │
│     │  调用 DeepSeek V3
│     │  prompt: "给每个文档打 0-10 的相关性分"
│     │  对每个文档评分，然后按分数重排
│     │  超时 5 秒，失败用原始排序
│     │
│     └─ 输出: []*schema.Document (reranked, top_k)
│
└─ 输出: docs + QueryTrace
```

---

## 第三章：记忆系统的数据流

### 3.1 记忆写入

```
对话结束后
│
▼
MemoryService.PersistOutcome()
│
├─ 构建要保存的对话内容
├─ 用 LLM 提取记忆候选 (ExtractMemoryCandidates)
│     │
│     │  prompt: "从对话中提取值得记住的关键信息"
│     │  超时保护 (configurable, 默认 1.5 秒)
│     │
│     └─ 输出: 记忆候选列表
│
├─ 校验每条记忆 (ValidateMemoryCandidate)
│     │
│     │  过滤掉:
│     │  ├─ 太短的
│     │  ├─ 太长的
│     │  ├─ 纯代码块的
│     │  └─ assistant boilerplate 的
│     │
│     └─ 输出: 有效记忆列表
│
└─ 写入 LongTermMemory
      │
      │  每条记忆:
      │  ├─ SessionID
      │  ├─ Content (文本)
      │  ├─ AccessCnt (访问次数)
      │  └─ LastUsed (最后使用时间)
```

### 3.2 记忆读取

```
新请求进来
│
▼
MemoryService.BuildContextPlan()
│
├─ ContextAssembler.Assemble()
│     │
│     ├─ PolicyResolver 选择 Profile
│     │     根据 mode (chat/aiops) 决定是否允许 memory
│     │
│     ├─ LongTermMemory.Retrieve(sessionID, query, limit)
│     │     │
│     │     │  按 query 相似度排序
│     │     │  返回最相关的 N 条记忆
│     │     │
│     │     └─ 输出: []Entry
│     │
│     └─ selectMemories(entries, profile)
│           │
│           │  按 budget 裁剪
│           │  按 freshness 排序
│           │  超出预算的丢弃
│           │
│           └─ 输出: MemoryItems
│
└─ 输出: memory_context (文本) + memory_refs (引用)
```

---

## 第四章：评测数据的流转

### 4.1 Baseline 生成

```
GenerateAIOPSBaselineArtifacts()
│
├─ 读取 input.json + groundtruth.jsonl
│
├─ 划分 build / holdout split (70% / 30%)
│
├─ 为 build cases 生成:
│     ├─ Evidence Docs (只含观测信号，不含标签)
│     └─ History Docs (含历史标签和结论)
│
├─ 为 holdout cases 生成:
│     └─ Eval Cases (query + expected results)
│
└─ 输出:
      ├─ baseline/docs_evidence_build/
      ├─ baseline/docs_history_build/
      ├─ baseline/eval/eval_cases_holdout_related.jsonl
      └─ baseline/eval/build_split.json
```

### 4.2 Telemetry 预处理

```
build_telemetry_evidence.py
│
├─ 读取 input.json + groundtruth.jsonl
├─ 读取 build_split.json
│
├─ 对每个 case:
│     │
│     ├─ 确定时间窗口
│     ├─ 定位相关 parquet 文件
│     │
│     ├─ 提取 Metric Signals
│     │     baseline 窗口 vs incident 窗口对比
│     │
│     ├─ 提取 Log Signals
│     │     过滤 → 归一化 → 聚合 → 评分
│     │
│     ├─ 提取 Trace Signals
│     │     过滤 → 分组 → 统计 → 评分
│     │
│     └─ 渲染 Markdown Evidence Doc + metadata
│
└─ 输出:
      ├─ baseline/docs_evidence_telemetry_build/  (build split)
      ├─ baseline/docs_evidence_telemetry/  (全量)
      └─ baseline/telemetry/  (汇总 JSONL + report)
```

### 4.3 远程评测

```
run_telemetry_baseline_remote.sh
│
├─ 启动 Milvus (Docker)
├─ Go prep: 生成 eval cases
├─ Go indexing: 把 evidence docs 索引进 Milvus
├─ Go eval: RunQueryEval
│     │
│     │  对每个 eval case:
│     │  ├─ 发送 query 到 RAG
│     │  ├─ 获取 top-K 结果
│     │  ├─ 与 expected 对比
│     │  └─ 计算 Recall@K, Hit@K
│     │
│     └─ 输出: report JSON
│
└─ 输出:
      └─ baseline/eval/report_evidence_telemetry_build_related.json
```

---

## 第五章：关键数据结构速查

### TaskEnvelope（任务信封）

```
task_id        → 任务唯一 ID
parent_task_id → 父任务 ID（子任务才有）
session_id     → 用户会话 ID
trace_id       → 追踪 ID
goal           → 任务目标
assignee       → 执行者（agent 名称）
input          → 附加参数
status         → 当前状态
memory_refs    → 记忆引用
```

### TaskResult（任务结果）

```
task_id            → 任务 ID
agent              → 执行者
status             → succeeded / failed / degraded
summary            → 摘要文本
confidence         → 置信度 0~1
evidence           → 证据列表
degradation_reason → 降级原因
```

### ContextPackage（上下文包）

```
HistoryMessages → 对话历史
MemoryItems     → 长期记忆
DocumentItems   → RAG 检索结果
ToolItems       → 工具调用结果
Trace           → 装配过程追踪
```

### QueryTrace（RAG 查询追踪）

```
Mode              → retrieve / rewrite / full
OriginalQuery     → 原始查询
RewrittenQuery    → 改写后查询
CacheHit          → RetrieverPool 是否命中缓存
RetrieveLatencyMs → 检索耗时
RewriteLatencyMs  → 改写耗时
RerankLatencyMs   → 重排耗时
RawResultCount    → 原始结果数
ResultCount       → 最终结果数
RerankEnabled     → 是否启用了重排
```