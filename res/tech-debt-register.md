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

### TD-08 RAG citation schema 不完整

- **文件**：`internal/ai/contextengine/documents.go:93`、`internal/ai/tools/query_internal_docs.go:36`
- **现象**：DocumentsContent() 只输出 `[序号] title + content`，没有稳定 citation id、source uri、score。工具直接返回 `[]schema.Document` JSON，没有归一化成 `{answer, citations, evidence}`。
- **风险**：模型生成的引用无法追踪到源文档，用户无法验证。
- **建议**：定义 `Citation` 结构体（id, source, title, score, snippet），DocumentsContent 输出带 citation 标记，工具返回归一化 schema。

### TD-09 RAG rewrite/rerank 失败 trace 不足

- **文件**：`internal/ai/rag/query_rewrite.go:77`、`internal/ai/rag/rerank.go:59`
- **现象**：rewrite 失败回退原 query，rerank 失败返回原文档，行为合理但日志/trace 颗粒度弱。
- **风险**：RAG 评测时无法判断 rewrite/rerank 是否生效，影响优化决策。
- **建议**：降级时记录 structured log（original_query, rewritten_query, reason）+ trace span attribute。

### TD-10 同步 Chat 缺少 Application 级 timeout

- **文件**：`internal/app/chat_app.go:149`、`internal/app/chat_app.go:424`、`internal/app/aiops_app.go:158`
- **现象**：同步 Chat/AIOps 调用依赖外层请求 ctx 和底层模型 timeout，没有 Application 级 timeout 包裹。
- **风险**：底层超时配置变更时上层无保护。
- **建议**：在 HandleChat/HandleAIOps 入口使用 `context.WithTimeout`，timeout 走配置 `chat.timeout_ms` / `aiops.timeout_ms`。

### TD-11 多处工具/降级 json.Marshal 错误被忽略

- **文件**：`internal/ai/tools/query_metrics_alerts.go:143`、`internal/ai/tools/query_internal_docs.go:45`
- **现象**：降级响应 json.Marshal 错误被忽略。
- **风险**：概率低但静默失败不符合工具层规范。
- **建议**：检查 error，失败时返回纯文本降级消息。

### TD-12 skills/domains/metrics 工具创建无 nil 检查

- **文件**：`internal/ai/skills/domains/metrics/agent.go:144`
- **现象**：`newPrometheusAlertsQueryTool()` 返回值无 nil 检查，后续直接 `tool.InvokableRun`。
- **风险**：工具创建失败时 panic。
- **建议**：nil 检查 + 返回 error。

### TD-13 incident list 读文件失败静默跳过

- **文件**：`internal/ai/service/ai_ops_incident.go:174`
- **现象**：读取单个文件失败时 continue，无日志。
- **风险**：坏文件或权限问题被吞掉，排查困难。
- **建议**：记录 warn 日志（file path, error）。

### TD-14 MCP reconnect 用 time.Sleep 不响应 ctx

- **文件**：`internal/ai/tools/query_log.go:130`
- **现象**：MCP reconnect 用 `time.Sleep`，不响应 ctx cancel。
- **风险**：shutdown 时 reconnect 阻塞优雅退出。
- **建议**：改用 `time.After` + ctx.Done select。

---

## P3：规范债，影响 prompt 可维护性

### TD-15 Chat 主 prompt 硬编码在代码中

- **文件**：`internal/ai/agent/chat_pipeline/prompt.go:15`
- **现象**：系统提示词、证据规则、运行时上下文模板全部硬编码。
- **风险**：prompt 调整需要改代码、编译、部署。
- **建议**：迁移到 `prompts/` 目录，运行时加载，支持版本化。

### TD-16 rewrite/rerank prompt 硬编码

- **文件**：`internal/ai/rag/query_rewrite.go:17`、`internal/ai/rag/rerank.go:19`
- **现象**：prompt 写死，超时已配置化但 prompt 不可热调整。
- **建议**：同 TD-15，统一迁移到 prompt registry。

### TD-17 其他 prompt 硬编码热点

- **文件**：`internal/app/aiops_app.go:18`（defaultAIOpsQuery）、`internal/ai/memory/agent.go:383`、`internal/ai/contextengine/intent_recognizer.go:77`、`internal/ai/contextengine/tool_reranker.go:180`、`internal/ai/agent/experts/linux_sre.go:425`
- **现象**：各模块 prompt 分散硬编码。
- **建议**：统一收口到 prompt registry，按模块/用途组织。

### TD-18 tiered_tools ToolNames 忽略错误

- **文件**：`internal/ai/tools/tiered_tools.go:66`
- **现象**：`ToolNames()` 忽略 Info 错误，直接读 `info.Name`。
- **建议**：防御 nil/err，返回空名 + warn 日志。
