# OpsCaption 代码导读地图

本文档列出系统中每个关键文件的作用，方便你快速定位。

---

## Agent 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| supervisor/supervisor.go | 总指挥 | 调 triage 分类 → 并行派 specialists → 汇总给 reporter |
| triage/triage.go | 路由器 | 用规则表匹配关键词，决定走哪些 domain |
| skillspecialists/metrics/agent.go | 指标 Agent | 查 Prometheus 告警 |
| skillspecialists/logs/agent.go | 日志 Agent | 查 MCP 日志 |
| skillspecialists/knowledge/agent.go | 知识 Agent | 用 Skills Registry 选 skill，查知识库 |
| reporter/reporter.go | 报告生成器 | 汇总所有 specialist 结果，生成 Markdown 或自然语言回答 |

## 协议层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| protocol/types.go | 统一数据结构 | TaskEnvelope / TaskResult / TaskEvent / EvidenceItem / ArtifactRef |

## Runtime 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| runtime/runtime.go | 任务分发引擎 | Dispatch: 找 Agent → 执行 → 记录 |
| runtime/registry.go | Agent 注册表 | 记录谁能做什么 |
| runtime/ledger.go | 任务账本接口 | 记录任务创建/更新/完成 |
| runtime/bus.go | 事件总线 | Agent 之间发事件通知 |
| runtime/artifacts.go | 产物存储接口 | 存储 Agent 执行产物 |
| runtime/file_store.go | 持久化实现 | Ledger + ArtifactStore 的文件存储版 |
| runtime/context.go | Runtime 上下文 | 通过 ctx 传递 Runtime 引用 |
| runtime/agent.go | Agent 接口定义 | Name() + Capabilities() + Handle() |

## Service 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| service/chat_multi_agent.go | Chat Multi-Agent 入口 | 判断是否走 Multi-Agent + 执行 |
| service/ai_ops_service.go | AIOps 入口 | AIOps 专用的 Multi-Agent 执行 |
| service/ai_ops_runtime.go | Runtime 工厂 | 按 dataDir 复用 Runtime |
| service/memory_service.go | 记忆管理 | 记忆构建 + 记忆持久化 |
| service/degradation.go | 降级开关 | 手动关闭 AI 能力 |
| service/approval_gate.go | 审批门 | 高风险操作拦截 |
| service/token_audit.go | Token 审计 | 用量追踪和限制 |

## RAG 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| rag/query.go | RAG 查询主入口 | Query → Rewrite → Retrieve → Rerank |
| rag/query_rewrite.go | 查询改写 | 用 LLM 把口语改成搜索词 |
| rag/rerank.go | 结果重排 | 用 LLM 给文档打分重排 |
| rag/retriever_pool.go | Retriever 连接池 | 按地址+top_k 复用 Milvus retriever |
| rag/shared_pool.go | 全局共享池 | 进程级单例 RetrieverPool |
| rag/config.go | RAG 配置 | 从 config.yaml 读取 RAG 参数 |
| rag/indexing_service.go | 知识入库 | 文档 → 切分 → embedding → 写入 Milvus |

## RAG Eval 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| rag/eval/types.go | 评测数据结构 | EvalCase / EvalResult / EvalReport |
| rag/eval/runner.go | 评测执行器 | 跑 eval cases，算 Recall@K |
| rag/eval/aiops_baseline.go | AIOps baseline 生成 | 从 groundtruth 生成 docs + eval cases |
| rag/eval/inmemory.go | 内存 Retriever | 评测用的内存向量库 |
| rag/eval/io.go | 文件读写 | 读写 JSONL / JSON 文件 |
| rag/eval/adapter.go | 适配器 | 把 Milvus retriever 适配成 eval 接口 |
| rag/eval/samples.go | 样本数据 | 内置的小规模评测样本 |
| rag/eval/online.go | 在线评测 | 线上 RAG 质量监控 |

## Context Engine 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| contextengine/types.go | 数据结构 | ContextRequest / Profile / Budget / Package |
| contextengine/assembler.go | 上下文装配器 | 按 Profile 和 Budget 组装四类上下文 |
| contextengine/resolver.go | 策略解析器 | 根据 mode/intent 选择 Profile |
| contextengine/documents.go | 文档注入 | RAG 检索结果注入上下文 |
| contextengine/tool_items.go | 工具结果注入 | Specialist 结果注入上下文 |

## Skills 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| skills/registry.go | Skill 注册表 | Register + Resolve + Match |
| skills/focus_collector.go | Focus 收集 | 从 task 提取 focus 信息 |
| skills/progressive_disclosure.go | 渐进展示 | 控制 skill 输出详细度 |

## Tools 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| tools/query_metrics_alerts.go | Prometheus 告警查询 | 查活跃告警 |
| tools/query_log.go | MCP 日志查询 | 查日志条目 |
| tools/query_internal_docs.go | 知识库检索 | 走 RAG 链路查文档 |
| tools/mysql_crud.go | MySQL 操作 | 数据库查询 |
| tools/get_current_time.go | 时间工具 | 获取当前时间 |
| tools/tiered_tools.go | 分层工具 | 工具能力分级 |

## Memory 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| utility/mem/long_term.go | 长期记忆存储 | 按 session 存取记忆 |
| utility/mem/extraction.go | 记忆提取 | 用 LLM 从对话中提取记忆 |
| utility/mem/token_budget.go | Token 预算 | 记忆 token 计算 |
| utility/mem/mem.go | 记忆工具函数 | SessionID 生成等 |

## 基础设施层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| utility/auth/jwt.go | JWT 认证 | Token 签发/验证/吊销 |
| utility/auth/rate_limiter.go | 本地限流 | 令牌桶算法 |
| utility/metrics/metrics.go | Prometheus 指标 | 系统指标收集 |
| utility/tracing/tracing.go | OpenTelemetry 追踪 | 分布式追踪 |
| utility/health/health.go | 健康检查 | /healthz /readyz |
| utility/safety/prompt_guard.go | 输入安全 | 过滤危险 prompt |
| utility/safety/output_filter.go | 输出安全 | 过滤敏感输出 |
| utility/resilience/resilience.go | 重试/熔断 | 韧性工具 |
| utility/cache/llm_cache.go | LLM 缓存 | 缓存 LLM 调用结果 |
| utility/common/common.go | 公共配置 | Milvus 地址、top_k 等读取 |
| utility/common/env.go | 环境变量 | .env 文件加载 |

## Controller 层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| controller/chat/chat_v1_chat.go | 普通聊天 | POST /api/v1/chat |
| controller/chat/chat_v1_chat_stream.go | 流式聊天 | SSE 流式响应 |
| controller/chat/chat_v1_ai_ops.go | AIOps 接口 | POST /api/v1/aiops |
| controller/chat/chat_v1_admin.go | 管理接口 | memory/degradation/approval |
| controller/chat/chat_v1_file_upload.go | 文件上传 | 知识库文件入库 |
| controller/chat/prompt_guard.go | 请求安全检查 | Controller 层的安全拦截 |
| controller/chat/response_helpers.go | 响应工具 | 统一响应格式 |

## 脚本层

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| scripts/aiops/build_telemetry_evidence.py | 本地预处理 | parquet → 异常信号摘要 |
| scripts/aiops/run_telemetry_local_then_remote.ps1 | 全链路编排 | 本地 → 上传 → 远程索引 → 评测 |
| scripts/aiops/run_telemetry_baseline_remote.sh | 远程评测 | Milvus + 索引 + eval |

## CLI 入口

| 文件 | 作用 | 一句话说明 |
|---|---|---|
| cmd/knowledge_cmd/main.go | 知识库 CLI | 本地文档入库 |
| cmd/rag_eval_cmd/main.go | RAG 评测 CLI | 离线评测 |
| cmd/rag_online_eval_cmd/main.go | 在线评测 CLI | 线上质量监控 |
| cmd/aiops_rag_prep_cmd/main.go | AIOps 预处理 CLI | baseline 生成 |
| cmd/ai_ops_cmd/main.go | AIOps CLI | 命令行 AIOps |
| cmd/chat_cmd/main.go | Chat CLI | 命令行聊天 |
| cmd/recall_cmd/main.go | 召回测试 CLI | RAG 召回调试 |