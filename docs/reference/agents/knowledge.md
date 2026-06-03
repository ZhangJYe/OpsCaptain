# Agent: knowledge

## 角色
知识库 specialist，负责检索 SOP、runbook、错误码解释和历史处理经验。

## 输入
- specialist query（从 triage 传入的知识检索任务）
- internal docs retriever（内部文档检索器）
- skill focus（技能焦点，如 release SOP、rollback runbook、error code lookup）

## 输出
- knowledge summary（知识库摘要）
- document evidence（文档证据列表）
- retrieval metadata（检索元数据，含 document_count）

## 约束 (Must)
- 区分 SOP、runbook、历史复盘和实时证据
- 保留 document_count、knowledge_mode、knowledge_query metadata
- 错误码任务要提取 error code 并提示确认来源服务
- 失败时返回 degraded 并保留 retrieval query

## 禁止 (MustNot)
- 不要把历史标签当实时证据
- 不要把知识库建议包装成已发生事实
- 不要在无文档命中时编造 SOP 内容

## 证据策略
- 知识库 evidence 是指导和背景，不等价于实时观测
- 涉及根因时必须和 metrics/logs 或用户提供事实交叉验证

## 依赖
- `query_internal_docs` 工具（`internal/ai/tools/query_internal_docs.go`）
- RAG 链路: Milvus 向量检索

## 降级策略
当知识库检索超时（默认 5s，配置项 `aiops.tools.knowledge_query_timeout_ms`）或文档结果解析失败时，返回 degraded 结果（confidence 0.25-0.3），附带原始查询以便排查。

配置项：
- `aiops.tools.knowledge_query_timeout_ms`：查询超时（默认 5s）
- `aiops.tools.knowledge_evidence_limit`：最大证据条数（默认跟随 `rag.RetrieverTopK`）

## 相关代码
- Contract: `internal/ai/agent/contracts/contracts.go`（registry key: "knowledge"）
- 实现: `internal/ai/agent/specialists/knowledge/agent.go`
- 工具: `internal/ai/tools/query_internal_docs.go`
