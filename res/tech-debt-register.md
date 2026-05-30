# 技术债登记表

> 来源：2026-05-30 全量扫描
> 每条记录：现象 / 文件 / 风险 / 建议方向 / 优先级

---

## P0：线上风险，应尽快修复 ✅ 已全部完成（2026-05-30）

### TD-01 ✅ Prometheus 工具缺少 HTTP 状态码检查

- **文件**：`internal/ai/tools/query_metrics_alerts.go:81`
- **修复**：添加 resp.StatusCode 非 2xx 检查 + result.Status != "success" 检查；降级响应 json.Marshal 错误不再忽略。

### TD-02 ✅ query_log fallback tool 创建失败直接 panic

- **文件**：`internal/ai/tools/query_log.go:301`
- **修复**：panic → g.Log().Errorf + return nil。新增 `logToolOrDegraded` helper 和 `degradedLogTool` stub，所有 7 个调用点均用 wrapper 包裹，nil tool 不会进入 tool slice。`ai_ops_runtime.go` 调用点也加了 nil 检查。

### TD-03 ✅ AIOps async goroutine 无 timeout 保护

- **文件**：`internal/ai/service/ai_ops_service.go:303`
- **修复**：goroutine 内添加 `context.WithTimeout`，默认 5 分钟，配置项 `aiops.async_timeout_ms`。

### TD-04 ✅ LLM validator 静默失效

- **文件**：`internal/ai/events/llm_validator.go:122`
- **修复**：modelFactory 失败和 Generate 失败时记录 `g.Log().Warningf`。Prometheus counter 待后续补充。

---

## P1：架构债，影响后续迭代效率

### TD-05 ◐ FileStore 逻辑散落在 Application 层

- **文件**：`internal/app/knowledge_app.go:96`、`internal/ai/service/ai_ops_incident.go:81`
- **现象**：KnowledgeApp 做上传校验、隔离区写入、文件落盘、metadata、去重。FileIncidentStore 在 service 层做文件持久化、JSON 编解码、目录遍历。
- **风险**：文件系统细节侵入 Application/Domain 层，无法替换存储后端，无法独立测试。
- **建议**：抽取 `internal/infra/filestore/`，定义 FileStore interface，Application 层通过 interface 操作。
- **进展**：Knowledge 上传文件存储已抽到 `internal/infra/filestore/`，`KnowledgeApp` 仅保留校验、索引编排和返回结果。`FileIncidentStore` 暂未迁移，避免在未明确 service 边界前新增 `internal/ai/service` → `internal/infra` import 例外。

### TD-06 ✅ RabbitMQ 两个队列重复 1000 行

- **文件**：`internal/ai/service/chat_task_queue.go`（975 行）、`internal/ai/service/memory_queue.go`（986 行）
- **修复**：抽取 `internal/infra/rabbitmq/`（client.go + topology.go + ttl_set.go + config.go），两个队列文件通过 `*rabbitmq.Client` 委托连接管理、重连、拓扑声明、发布/消费。memory_queue.go 986→582 行，chat_task_queue.go 975→689 行。消除 `closeMemoryChannel`/`closeMemoryConnection` 跨文件引用 bug。publishRaw 统一采用 context-aware 重连策略（ctx deadline 收敛 dial timeout）。`OnReconnectFailed` callback 消费端重连失败可观测。chat_task retry publish 日志变量修正。

### TD-07 ✅ ChatApp 文件体量偏高

- **文件**：`internal/app/chat_app.go`（原 549 行）
- **修复**：拆分为 3 个文件，均在 `app` package，不改 import 关系：
  - `chat_app.go`（212 行）：ChatApp struct、构造函数、HandleChat、session lock
  - `chat_stream.go`（285 行）：ChatStreamInput、HandleChatStream、isOpsRelatedQuery
  - `chat_validation.go`（85 行）：ValidateChatInput、classifyError、filterOutput、enrichContext、shouldBypassCache
- `go build`、`go vet`、`go test` 全部通过，行为语义无变化。

---

## P2：质量债，影响可观测性和可维护性

### TD-08 ✅ RAG citation schema 不完整

- **文件**：`internal/ai/rag/citation.go`（新增）、`internal/ai/contextengine/documents.go`、`internal/ai/tools/query_internal_docs.go`
- **修复**：
  - 新增 `rag.Citation`/`Evidence`/`KnowledgeSearchOutput` schema + `CitationFromDocument`/`BuildCitations` helper
  - `DocumentsContent()` 输出 `[ctx-doc-N] title/source/score/content` 格式
  - `query_internal_docs` 工具成功时返回 `{success, answer, citations, evidence}` 归一化 schema
  - `chat_evidence.txt` 和 `chat_runtime_context.txt` 添加引用规则
  - 单测覆盖 citation 生成、metadata 映射、snippet 截断、工具 JSON schema

### TD-09 ✅ RAG rewrite/rerank 失败 trace 不足

- **文件**：`internal/ai/rag/query_rewrite.go`、`internal/ai/rag/rerank.go`
- **修复**：`RewriteQueryMulti` 模型初始化和 Generate 失败添加 Warningf 日志；`Rerank` Generate 失败从 Debugf 升级为 Warningf。

### TD-10 ✅ 同步 Chat 缺少 Application 级 timeout

- **文件**：`internal/app/chat_app.go`、`internal/app/aiops_app.go`
- **修复**：HandleChat/HandleAIOps 入口添加 `context.WithTimeout`，timeout 走配置 `chat.timeout_ms` / `aiops.timeout_ms`，默认 0（不启用，向后兼容）。

### TD-11 ✅ 多处工具/降级 json.Marshal 错误被忽略

- **文件**：`internal/ai/tools/query_internal_docs.go:45`
- **修复**：`query_metrics_alerts.go` 已在 TD-01 中修复；`query_internal_docs.go` 降级响应 json.Marshal 错误改为 fallback 纯文本。

### TD-12 ✅ skills/domains/metrics 工具创建无 nil 检查

- **文件**：`internal/ai/skills/domains/metrics/agent.go`
- **修复**：`runPrometheusAlertQuery` 和 `runPrometheusAlertQueryWithFocus` 两处调用点添加 nil 检查，返回 degraded TaskResult。

### TD-13 ✅ incident list 读文件失败静默跳过

- **文件**：`internal/ai/service/ai_ops_incident.go:174`
- **修复**：读取单个文件失败时记录 `g.Log().Warningf`（file name, error）。

### TD-14 ✅ MCP reconnect 用 time.Sleep 不响应 ctx

- **文件**：`internal/ai/tools/query_log.go`
- **修复**：`reconnect()` 签名改为 `reconnect(ctx context.Context)`，`time.Sleep` 替换为 `select { case <-time.After(delay): case <-ctx.Done(): return ctx.Err() }`。

---

## P3：规范债，影响 prompt 可维护性

### TD-15 ✅ Chat 主 prompt 硬编码在代码中

- **文件**：`internal/ai/agent/chat_pipeline/prompt.go`
- **修复**：创建 `internal/ai/promptreg/` 包，`//go:embed` 加载 5 个 prompt 文件（chat_base/identity/language/evidence/runtime_context）。`prompt.go` 120 行常量删除，改为引用 `promptreg.*`。prompt 文件可独立编辑，重新编译即生效。

### TD-16 ✅ rewrite/rerank prompt 硬编码

- **文件**：`internal/ai/rag/query_rewrite.go`、`internal/ai/rag/rerank.go`
- **修复**：`rewriteSystemPrompt`/`rerankSystemPrompt` 常量迁移到 `promptreg/rag_rewrite.txt` 和 `promptreg/rag_rerank.txt`，代码引用 `promptreg.RAGRewrite`/`promptreg.RAGRerank`。

### TD-17 部分完成 — 其他 prompt 硬编码热点

- **已迁移**：chat pipeline 主 prompt（5 文件）、RAG rewrite/rerank（2 文件）
- **保持内联**：`defaultAIOpsQuery`（7 行，含执行步骤编排）、`memoryAgentSystemPrompt()`（模板函数）、`buildIntentPrompt()`（模板函数）、`buildRerankPrompt()`（模板函数）、`linux_sre.go` system message（1 行）。理由：这些是短小的模板函数或运行时拼接逻辑，迁移到文件不会显著改善可维护性。

### TD-18 ✅ tiered_tools ToolNames 忽略错误

- **文件**：`internal/ai/tools/tiered_tools.go:66`
- **修复**：`ToolNames()` 添加 nil tool 跳过、`info == nil` 防御、`err != nil` Warningf + continue。

---

## RAG 质量治理（2026-05-30）

### TD-19 ✅ BM25 中文分词完全失效

- **文件**：`internal/ai/rag/bm25.go:170`
- **现象**：`bm25Tokenize()` 仅保留 `[a-z0-9_\-./:]`，所有中文字符被丢弃。中文查询无法命中任何文档。
- **修复**：新增 `isCJK()` 判断，CJK 字符（`一-鿿`、`㐀-䶿`）生成 unigram + bigram token。新增 `TestBM25Tokenize_Chinese` 和 `TestBM25Index_ChineseSearch` 测试。

### TD-20 ✅ BM25 命中文档丢失正文

- **文件**：`internal/ai/rag/bm25.go`、`internal/ai/rag/hybrid.go:249`
- **现象**：`BM25Hit` 无 Content 字段，`bm25Doc` 不存储原始内容。`lexHitToDoc()` 创建的 `schema.Document` 没有正文，导致 BM25-only 命中的文档在 citation 输出中无内容。
- **修复**：`bm25Doc` 和 `BM25Hit` 新增 `Content` 字段；`AddDocument` 存储原始内容；`Search` 返回 Content；`lexHitToDoc` 设置 `doc.Content`。新增 `TestBM25Hit_PreservesContent` 测试。

### TD-21 ✅ BM25 索引重启后为空

- **文件**：`main.go:85`
- **现象**：BM25 索引纯内存，无持久化。进程重启后 `SharedBM25Index()` 返回空索引，hybrid 检索退化为 dense-only，直到有人触发 `IndexSource()`。
- **修复**：`main.go` 启动时调用 `rag.DefaultIndexingService().SyncBM25Index(ctx)` 从 `common.FileDir` 预热索引。

---

## RAG v2 生产 rerank 接入（2026-05-30）

### TD-22 ✅ 生产 Query 接入 rewrite/rerank

- **文件**：`internal/ai/rag/query.go`、`internal/ai/rag/hybrid.go`、`manifest/config/config.yaml`
- **现象**：`rag.Query()` 不走 rewrite/rerank，hybrid 直接返回 FinalTopK（默认 5）。rerank 仅在 eval 路径可用。
- **修复**：
  - `HybridConfig` 新增 `CandidateTopK` 字段，`DefaultHybridConfig` 默认等于 `FinalTopK`。
  - `HybridRetrieveWithRetriever` 用 `CandidateTopK` 做裁剪，不再提前 trim 到 `FinalTopK`。
  - `Query()` 新增配置开关：`rag.rewrite_enabled`、`rag.rerank_enabled`。rerank 启用时自动用 `RetrieverCandidateTopK`（默认 topK*4，[20,50]）。
  - 流程：optional rewrite → hybrid candidate retrieve → optional rerank → final trim。
  - trace 记录 `RerankEnabled`、`RerankLatencyMs`、`RewriteLatencyMs`、`RawResultCount`、`ResultCount`。
  - `config.yaml` 补齐 `rag:` 默认配置块，rewrite/rerank 默认关闭。
  - 回归测试 `TestQuery_DefaultConfigDisablesRewriteAndRerank` 确认默认行为不变。

### TD-23 ✅ SyncBM25Index newLoader 失败静默

- **文件**：`internal/ai/rag/indexing_service.go:107`
- **修复**：`newLoader` 失败时添加 `g.Log().Warningf`。

### TD-24 ✅ rerank 中文截断坏 UTF-8

- **文件**：`internal/ai/rag/rerank.go:44`
- **修复**：`content[:200]` 字节截断改为 `[]rune` 截断。

---

## RAG v2 citation trace（2026-05-30）

### TD-25 ✅ 每条 citation 补检索 trace

- **文件**：`internal/ai/rag/citation.go`、`internal/ai/rag/hybrid.go`、`internal/ai/rag/query.go`
- **现象**：citation 只有 score/snippet/source，无法追溯文档在检索管线中各阶段的排名和得分。
- **修复**：
  - `CitationTrace` 结构体：`DenseRank`、`LexicalRank`、`FusionScore`、`MetadataBoost`、`RerankScore`。
  - `Citation` 新增 `Trace *CitationTrace` 字段。
  - `rrfFusion` 完成后将 `dense_rank`、`lexical_rank`、`fusion_score` 写入文档 MetaData。
  - `HybridRetrieveWithRetriever` 在 `refineRetrievedDocs` 后计算 `metadata_boost`（位置提升量）。
  - `Query()` 在 rerank 后将 `rerank_score` 写入文档 MetaData。
  - `CitationFromDocument` 从 MetaData 提取 trace，无 trace 时 `Trace` 为 nil（不增加无意义字段）。
  - 新增 3 个测试覆盖 trace 有/无/部分场景。

---

## RAG v2 评测框架（2026-05-30）

### TD-26 ✅ eval 框架补 MRR 和 per-doc trace

- **文件**：`internal/ai/rag/eval/types.go`、`runner.go`、`online.go`、`query_adapter.go`
- **现象**：eval 框架只有 Recall@K/HitRate@K/FullRecall@K，缺少 MRR；`RetrievedDoc` 无 trace 字段；无 `rag.Query()` 适配器。
- **修复**：
  - `Summary` 新增 `MRR` 字段，`runner.go` 新增 `reciprocalRank()` 计算。
  - `RetrievedDoc` 新增 `Trace *DocTrace` 字段（`DenseRank`、`LexicalRank`、`FusionScore`、`MetadataBoost`、`RerankScore`）。
  - `QuerySummary` 新增 `CitationCoverage`（非空结果比例）。
  - `SchemaDocsToRetrievedDocs` 从文档 MetaData 提取 trace。
  - 新增 `NewQueryExecutor(pool)` 适配器，包装 `rag.Query()` 为 `QueryExecutor`，自动捕获 trace 和 latency。
  - 新增 `TestRunComputesMRR`、`TestReciprocalRank` 测试。

---

## RAG v2 baseline 实验基础设施（2026-05-30）

### TD-27 ✅ eval CLI 增强与 baseline 实验脚本

- **文件**：`internal/ai/cmd/rag_online_eval_cmd/main.go`、`run_baseline.sh`、`eval/testdata/baseline_cases.jsonl`、`docs/rag-baseline-experiment.md`
- **修复**：
  - eval CLI `printSummary` 新增 MRR、CitationCoverage、EmptyRate（百分比格式）显示。
  - 新增 `rerank` eval mode（rerank=true, rewrite=false），支持单独测试 rerank 效果。
  - `-eval` 默认为空时自动使用 `eval.SampleCases()` 内置用例。
  - `warmupBM25` 支持无 evalPath 时从 `common.FileDir` 加载文档。
  - 失败 case 自动打印到 stderr。
  - `baseline_cases.jsonl`：18 条覆盖单文档/多文档/跨域/口语化/英文/同义改写场景，`relevant_ids` 对齐 `docs/knowledge/*.md` canonical ID。
  - `run_baseline.sh`：一键跑 `hybrid-retrieve` vs `hybrid-rerank` 生产路径对比实验。
  - `docs/rag-baseline-experiment.md`：实验方法论、指标说明、trace 分析指南。
- **实验结果**（2026-05-30）：hybrid-retrieve MRR=0.8148 Recall@3=1.00 160ms；hybrid-rerank MRR=0.9352 Recall@3=1.00 3895ms。Rerank 输入压缩后超时降为 1/18，质量明显提升，但延迟仍高。结论：不开全局 rerank，下一步做选择性 rerank 或继续压缩候选数/输入长度。
