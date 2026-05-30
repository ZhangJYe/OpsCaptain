# Agent 编码踩坑规则

> 每条规则对应一个真实发生过的失败案例。
> AGENTS.md 只保留最关键的 5 条摘要，完整列表在此。

---

## 基础设施与生命周期

- **env file 加载**走 `utility/common.LoadEnvFile`，不在 `main.go` 内联写。原先逻辑不便复用且 scanner error 没处理。（问题 6，§11）
- **Revoked token 吊销表**必须有自动过期清理，否则长期运行内存泄漏。已补 `clearExpiredRevokedTokens` + 后台清理协程。（问题 1，§6）
- **CORS 默认策略**不能过宽。`allowed_origins` 为空时不应原样回写 Origin。SSE 不能无条件设 `*`。统一走 `ResolveAllowedOrigin`。（问题 7，§12）
- **AI Ops runtime** 不要每次请求重建，走 `getOrCreateAIOpsRuntime` 按 dataDir 复用。（§22.1）

## Memory 与上下文

- **Long-term memory** 必须有全局上限和 per-session 上限，否则内存无限增长。通过 `config.yaml` 配置 `memory.long_term_max_entries`。（问题 2，§7）
- **`LongTermMemory.Retrieve`** 应使用 `RLock` 而非写锁，只在更新 `AccessCnt/LastUsed` 时短暂获取写锁。（问题 9，§14）
- **Chat 和 ChatStream 的 memory 持久化**统一走 `MemoryService.PersistOutcome`，不裸起 `go ExtractMemories(context.Background())`。（§23.2）
- **异步记忆抽取**必须有 timeout 保护，用 `context.WithoutCancel + context.WithTimeout`，配置项 `memory.extract_timeout_ms`。（§21.1）
- **记忆写入前**必须做基础过滤：assistant boilerplate、代码块、异常长度内容应丢弃。走 `ExtractMemoryCandidates + ValidateMemoryCandidate`。（§27.2）

## RAG 与检索

- **知识库检索 `top_k`** 不要 hardcode 3，走 config 读取 `multi_agent.knowledge_evidence_limit`。（问题 8，§13）
- **`query_internal_docs`** 不要每次新建 retriever，按 Milvus 地址和 top_k 复用，失败走短 TTL 缓存。（§24.3）
- **RAG chunking** 不能只按 Markdown 标题切，要支持 JSONL case 级切分。当前 `transformer.go` 偏 Markdown 结构。
- **build split 和 eval split** 必须严格分开，不能拿全量数据自证效果。
- **metric score 阈值** `< 0.1 and abs(delta) < 0.5` 是当前硬编码，后续需按 metric 类型调整。

## Agent 输出与事件

- **`task_completed` 事件**不要把完整 Markdown 报告塞进 `Message`，长摘要折叠并把 `status/summary_length` 放 `Payload`。（§24.2）
- **observation** 不能只保留一个关键词。`context canceled` 不是完整观察，`paymentservice + error log + context canceled` 才有区分度。

## 知识图谱

- **图谱关系**必须标记来源和置信度（`source_type/derivation_type/extractor_version`），否则后续无法 debug。
- **`CASE_SIMILAR_TO_CASE` 边**必须记录 `similarity_score`、`similarity_components`、`computed_by`。

## 编码细节

- **`Reporter` 首字母大写**用自定义函数 `displayAgentName`，不用 `strings.Title`。`strings.Title` 已废弃且对 Unicode 不稳定。（问题 4，§9）
- **PowerShell here-string** 中 shell 变量要避免和 PS 变量名冲突。用 `probe_status` 而非 `$status`。
