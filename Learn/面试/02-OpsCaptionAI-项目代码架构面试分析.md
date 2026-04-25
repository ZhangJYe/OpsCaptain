# OpsCaptionAI 项目代码架构面试分析

> 适用场景：技术面试、项目复盘、架构讲解。  
> 说明：当前版本聚焦已经更稳定的 Chat、AIOps、RAG、Context、记忆、安全治理和工程化能力；暂不展开尚未完善的复杂协作设计。

## 1. 项目整体概述

OpsCaptionAI，也就是当前仓库中的 OpsCaptain / SuperBizAgent，是一个面向内部运维团队的 AIOps 智能运维助手。它解决的核心问题是：生产环境出现告警或故障时，值班工程师通常需要在 Prometheus、日志平台、知识库、数据库和历史处理经验之间反复切换，人工整合证据成本高、速度慢，也容易遗漏关键信息。

项目把这些能力整合到一个统一入口里：用户用自然语言描述问题，系统根据场景进行对话回答、知识检索、告警分析、日志查询、上下文补全和报告生成，最终输出可解释的运维建议。

可以用一句话概括：

> 这是一个以 Go 后端为主体、基于 Eino 和 RAG 构建的智能运维助手，用 LLM 工具调用、知识库检索、上下文工程和记忆系统，把自然语言问题转成可追踪、可治理、带证据的运维分析结果。

当前代码中最适合面试讲的能力包括：

| 能力 | 入口 | 解决的问题 |
| --- | --- | --- |
| 智能对话 | `POST /api/chat`、`POST /api/chat_stream` | 支持自然语言问答、工具辅助查询、知识库问答 |
| AIOps 分析 | `POST /api/ai_ops` | 对告警、日志、文档等证据做更完整的运维分析 |
| 知识库检索 | `query_internal_docs`、上传索引流程 | 把内部文档、SOP、runbook 转成可检索知识 |
| 上下文工程 | `contextengine.Assembler` | 控制历史、记忆、文档和工具结果进入模型的方式 |
| 记忆系统 | `MemoryService`、`utility/mem` | 保存会话历史，并异步沉淀可复用的长期记忆 |
| 安全治理 | Prompt Guard、Output Filter、Approval、Token Audit | 控制注入风险、敏感信息泄露、高危操作和调用成本 |

## 2. 技术栈列表

| 分类 | 技术 |
| --- | --- |
| 后端语言 | Go 1.24 |
| Web 框架 | GoFrame v2 |
| LLM 编排 | CloudWeGo Eino、Eino ADK、ReAct、Plan-Execute-Replan |
| LLM 接入 | OpenAI-compatible ChatModel，配置项为 `ds_think_chat_model`、`ds_quick_chat_model`；当前示例配置指向 GLM-4.5-AIR，部署时可替换为 DeepSeek 等兼容模型 |
| Embedding | OpenAI-compatible Embedding，配置项为 `doubao_embedding_model`；当前示例配置使用 BGE-M3，函数名保留了 DoubaoEmbedding 的历史命名 |
| 向量数据库 | Milvus，`milvus-sdk-go/v2` |
| 检索增强 | Dense retrieval、BM25、Hybrid RRF、Query Rewrite、LLM Rerank |
| 缓存 / 队列 / 审计 | Redis、RabbitMQ |
| 指标系统 | Prometheus HTTP API `/api/v1/alerts` |
| 日志接入 | MCP SSE client |
| 数据库访问 | MySQL + GORM，工具层只允许安全 SELECT |
| 可观测性 | Prometheus metrics、OpenTelemetry、Jaeger、pprof |
| 安全治理 | JWT、RBAC、RateLimit、Prompt Guard、Output Filter、审批流 |
| 前端 | 原生 HTML / CSS / JavaScript |
| 部署 | Docker、Docker Compose、Caddy、Nginx |

## 3. 模块划分与核心类/函数

### 3.1 `main.go`

`main()` 是服务启动入口，负责初始化运行环境和 HTTP 服务。

关键职责：

1. 加载环境变量和配置。
2. 初始化 Redis、日志、Tracing。
3. 注入模型层的 Token Audit hook。
4. 校验启动密钥、记忆抽取配置、异步对话任务配置。
5. 启动记忆抽取管道和异步对话任务管道。
6. 挂载 `/healthz`、`/readyz`、`/metrics`。
7. 在 `/api` 下绑定 CORS、Auth、RateLimit、Response 中间件和业务 Controller。
8. 处理优雅停机，关闭 HTTP、pprof、Tracing、队列和依赖资源。

面试讲法：

> 启动逻辑不是只起一个 HTTP server，而是把认证、日志、追踪、队列、成本审计、健康检查和优雅停机都纳入启动生命周期。这说明项目已经按生产服务的方式做基础设施治理。

### 3.2 `ControllerV1.Chat`

路径：`internal/controller/chat/chat_v1_chat.go`

`Chat` 是同步对话入口。它的核心流程是：

1. 校验 session ID。
2. 写入 session ID、request ID 到 context。
3. 执行 Prompt Guard，拦截明显的提示注入。
4. 判断全局降级开关。
5. 对同一 session 加锁，保证并发请求串行处理。
6. 查询缓存，命中时直接返回。
7. 组装上下文。
8. 构建并执行 Eino ReAct 对话链路。
9. 输出过滤。
10. 持久化对话结果到短期记忆和长期记忆抽取流程。
11. 写入响应缓存。

面试讲法：

> Controller 层做的是请求生命周期治理，不直接写复杂业务。真正的模型调用、上下文装配、缓存、记忆沉淀都委托给专门模块，这样职责比较清晰。

### 3.3 `ControllerV1.ChatStream`

路径：`internal/controller/chat/chat_v1_chat_stream.go`

`ChatStream` 是 SSE 流式对话入口，整体流程和 `Chat` 类似，但输出方式不同：

1. 创建 SSE client。
2. 发送 meta 事件，包含 mode、trace、detail、degraded 等信息。
3. 把上下文组装 detail 作为 thought 事件推给前端。
4. 调用 Eino 的 Stream。
5. 按 chunk 推送 message。
6. 流结束后统一写入记忆。

面试讲法：

> 同步接口和流式接口复用了同一套上下文与模型链路，只是在输出层做了 SSE 分块，这样能保证两种交互方式行为一致。

### 3.4 `ControllerV1.AIOps`

路径：`internal/controller/chat/chat_v1_ai_ops.go`

`AIOps` 是深度运维分析入口。它比普通 Chat 多了两个关键能力：

1. 空 query 时注入默认分析指令，要求按告警、文档、证据、结论的结构输出。
2. 调用 AIOps 分析服务完成审批、上下文、工具执行和报告生成。

虽然函数名里保留了历史命名，但面试时可以按“深度运维分析服务”来讲，不需要展开尚未稳定的内部协作模型。

### 3.5 AIOps 分析服务

路径：`internal/ai/service/ai_ops_service.go`

这是 AIOps 分析的服务层入口。核心步骤：

1. `ApprovalGate.Check` 检查是否涉及高危操作。
2. `GetDegradationDecision` 判断是否进入全局降级。
3. `MemoryService.ResolveSessionID` 生成或获取会话 ID。
4. `MemoryService.BuildContextPlan` 获取可参考的历史记忆。
5. 通过场景提示增强，把查询补充成更适合运维分析的输入。
6. 调用 Plan-Execute-Replan 分析链路。
7. 收集执行 detail 和 trace ID。
8. `PersistOutcome` 写入记忆。
9. 返回统一的 `ExecutionResponse`。

面试讲法：

> AIOps 入口强调的是工程治理：高危请求先审批，系统不可用时先降级，有历史上下文就补充，没有证据就不要强行编造结论，最后把分析结果沉淀到记忆中。

### 3.6 `plan_execute_replan.BuildPlanAgent`

路径：`internal/ai/agent/plan_execute_replan/`

这是复杂运维分析的核心执行链路。它基于 Eino ADK 的 Plan-Execute-Replan 模式：

| 阶段 | 作用 |
| --- | --- |
| Planner | 根据用户问题生成执行计划 |
| Executor | 按计划调用工具并整理中间结果 |
| Replanner | 判断已有结果是否足够，不足时调整计划 |

关键参数：

- 整体超时：3 分钟。
- 最大重规划轮数：5。
- Executor 最大执行轮数：10。
- 可用工具包括 Prometheus 告警查询、内部文档检索、日志 MCP、当前时间等。

面试讲法：

> 普通问答可以直接让模型回答，但复杂故障排查通常需要先计划、再查证据、再判断是否补充查询。Plan-Execute-Replan 的价值就是把“想一步答一步”变成“有计划、有执行、有复核”的流程。

### 3.7 `chat_pipeline.BuildChatAgentWithQuery`

路径：`internal/ai/agent/chat_pipeline/`

这是普通对话的 Eino Graph 构建入口：

```text
UserMessage
  -> InputToChat
  -> ChatTemplate
  -> ReAct
  -> schema.Message
```

关键设计：

| 模块 | 作用 |
| --- | --- |
| `newInputToChatLambda` | 把业务请求转成 prompt 模板变量 |
| `newChatTemplate` | 注入系统提示词、日期、文档、历史消息 |
| `newReactAgentLambdaWithQuery` | 构建 ReAct 执行器 |
| Progressive Disclosure | 根据问题选择暴露哪些工具，降低噪声和误调用 |

面试讲法：

> 普通对话走 ReAct，是因为它适合“模型判断是否需要工具”的交互。比如用户问当前告警，模型可以调用 Prometheus；用户问文档步骤，模型可以调用知识库；简单问题则直接回答。

### 3.8 `tools.BuildTieredTools`

路径：`internal/ai/tools/tiered_tools.go`

工具不是一次性全部暴露给模型，而是按层级控制：

| 层级 | 说明 | 典型工具 |
| --- | --- | --- |
| AlwaysOn | 总是可用 | 当前时间、内部文档检索 |
| SkillGate | 只有问题匹配相关领域时暴露 | Prometheus、日志 MCP |
| OnDemand | 更谨慎暴露 | MySQL 查询工具 |

这样做的意义：

- 减少无关工具对模型决策的干扰。
- 降低误调用风险。
- 控制 prompt 中工具描述的长度。
- 对 MySQL 这类高风险能力做额外开关控制。

### 3.9 `contextengine.Assembler.Assemble`

路径：`internal/ai/contextengine/assembler.go`

Context Engine 是上下文工程核心，用来控制哪些信息可以进入模型。

它支持四类来源：

| Stage | 内容 |
| --- | --- |
| history | 当前会话历史 |
| memory | 长期记忆 |
| documents | RAG 检索文档 |
| tool_results | 工具结果 |

每个 Stage 都会记录：

- 考虑了多少来源。
- 选中了多少来源。
- 丢弃了什么内容。
- 丢弃原因是什么。
- 使用了多少 token 预算。
- 检索耗时和命中情况。

面试讲法：

> LLM 应用的稳定性很大程度取决于上下文质量。这个模块把上下文拼接从“临时字符串拼接”升级成了“按模式、按预算、可追踪、可裁剪”的工程能力。

### 3.10 `MemoryService`

路径：`internal/ai/service/memory_service.go`

`MemoryService` 负责两个方向：

| 能力 | 说明 |
| --- | --- |
| 构建上下文 | 调用 Context Engine，把短期历史、长期记忆、文档组装成模型输入 |
| 持久化结果 | 对话完成后写入短期记忆，并触发长期记忆抽取 |

`PersistOutcome` 的流程：

```text
回答完成
  -> SimpleMemory.AddUserAssistantPair
  -> enqueueMemoryExtraction
       -> RabbitMQ 可用：发布到队列
       -> RabbitMQ 不可用：本地有界异步执行
  -> MemoryAgent 抽取候选记忆
  -> ValidateMemoryCandidate 过滤
  -> LongTermMemory.StoreWithOptions 保存
```

面试讲法：

> 记忆不是简单保存所有对话，而是先保存短期窗口，再异步抽取可复用的事实、偏好或流程。长期记忆还带 scope、confidence、safety label、provenance 和过期时间，避免长期污染上下文。

### 3.11 `rag.Query` 与 `IndexingService`

路径：`internal/ai/rag/`

RAG 分为检索和索引两条链路。

检索侧：

| 函数 / 模块 | 作用 |
| --- | --- |
| `Query` / `QueryWithMode` | 执行不同检索模式 |
| `RetrieverPool.GetOrCreate` | 复用 Milvus Retriever，并缓存初始化失败 |
| `RewriteQuery` | 用 LLM 优化检索 query |
| `Rerank` | 用 LLM 对候选文档重排 |
| `HybridRetrieve` | Dense retrieval + BM25 + RRF 融合 |
| `refineRetrievedDocs` | 根据 service、metric、trace、pod 等 metadata 做 boost |

索引侧：

| 函数 / 模块 | 作用 |
| --- | --- |
| `IndexingService.IndexSource` | 单文件索引入口 |
| `metadataSidecarLoader` | 加载文档旁路 metadata |
| Markdown Header Splitter | 先按 Markdown 结构切分 |
| Semantic Splitter | 大 chunk 再语义切分 |
| Milvus Indexer | 写入向量库 |
| BM25 sync | 同步词法检索索引 |

面试讲法：

> RAG 不是只接 Milvus topK。这个项目里已经把初始化复用、失败缓存、query rewrite、候选召回、元数据增强、重排和混合检索都留好了扩展点，后续可以通过评测数据持续调优。

## 4. 核心业务流程

### 4.1 普通 Chat 流程

```text
POST /api/chat
  -> ValidateSessionID
  -> enrichRequestContext
  -> Prompt Guard
  -> Degradation Check
  -> session lock
  -> cache lookup
  -> MemoryService.BuildChatPackage
  -> contextengine.Assembler
  -> chat_pipeline.BuildChatAgentWithQuery
  -> Eino ReAct
  -> LLM 按需调用工具
  -> Output Filter
  -> PersistOutcome
  -> cache store
  -> ChatRes
```

这条链路的关键点：

- 同一 session 串行，避免上下文并发写入错乱。
- 缓存命中直接返回，减少重复 LLM 成本。
- Context Engine 控制进入 prompt 的历史、记忆和文档。
- 工具分层暴露，降低误调用。
- 输出经过敏感信息过滤。
- 回答结果进入记忆沉淀流程。

### 4.2 流式 Chat 流程

```text
POST /api/chat_stream
  -> 和 Chat 相同的安全与上下文处理
  -> 创建 SSE client
  -> 发送 meta
  -> 发送 context detail
  -> runner.Stream
  -> chunk -> message event
  -> EOF -> done event
  -> PersistOutcome
```

这条链路适合前端实时展示模型输出，同时保留完整的记忆持久化和安全过滤。

### 4.3 AIOps 分析流程

```text
POST /api/ai_ops
  -> Prompt Guard
  -> Degradation Check
  -> 默认 query 注入
  -> ApprovalGate.Check
       -> 高危：进入审批队列
       -> 非高危：继续执行
  -> MemoryService.BuildContextPlan
  -> 场景提示增强
  -> Plan-Execute-Replan
       -> Planner 生成计划
       -> Executor 调工具
       -> Replanner 判断是否补充
  -> PersistOutcome
  -> AIOpsRes
```

这条链路适合讲清楚项目的“深度分析能力”：

- 先做安全和降级，保证入口可控。
- 复杂问题先生成计划，而不是直接回答。
- 工具结果作为分析依据。
- 结果写入记忆，后续对话可以延续上下文。

### 4.4 RAG 检索流程

默认检索流程：

```text
query_internal_docs
  -> rag.Query
  -> SharedPool.GetOrCreate
  -> Milvus Retriever
  -> retrieve candidate docs
  -> refineRetrievedDocs
  -> top_k docs
```

完整增强流程：

```text
原始问题
  -> RewriteQuery
  -> Milvus Retrieve candidate_top_k
  -> metadata boost
  -> LLM Rerank
  -> top_k docs
```

混合检索流程：

```text
Dense retrieval + BM25 lexical search
  -> RRF fusion
  -> metadata boost
  -> final top_k docs
```

### 4.5 知识索引流程

```text
上传文档或 CLI 指定文件
  -> IndexingService.IndexSource
  -> FileLoader
  -> metadata sidecar merge
  -> deleteExistingSource
  -> Markdown Header Splitter
  -> Semantic Splitter
  -> Milvus Indexer
  -> sync BM25 index
  -> IndexBuildSummary
```

索引流程的关键点是幂等：同一个 source 重建时先删除旧 chunk，再写入新 chunk，避免重复召回。

### 4.6 记忆抽取流程

```text
对话或分析结束
  -> PersistOutcome
  -> 写入 SimpleMemory
  -> 尝试发布 RabbitMQ 记忆抽取事件
  -> 不可用则本地有界异步执行
  -> LLM 记忆抽取可用则使用 LLM
  -> 不可用 fallback rule extractor
  -> ValidateMemoryCandidate
  -> LongTermMemory 保存
```

记忆的治理字段：

| 字段 | 作用 |
| --- | --- |
| scope | session / user / project / global |
| confidence | 控制进入上下文的可信度 |
| safety_label | 过滤 disabled、superseded 等不安全内容 |
| provenance | 记录来源 |
| expires_at | 控制过期 |
| conflict_group | 支持同类记忆替换 |

## 5. 亮点与难点

### 5.1 亮点

**1. 上下文工程做成独立模块**

项目没有在 Controller 或 prompt 里随手拼历史，而是通过 Context Engine 统一装配 history、memory、documents、tool results，并记录 token 预算和裁剪原因。这是 LLM 应用从 Demo 走向工程化的重要标志。

**2. RAG 链路具备持续优化空间**

检索不只是向量 topK，还支持查询改写、候选集扩大、元数据增强、LLM 重排、BM25 和混合检索。后续可以用评测集持续比较不同模式的 Recall@K 和 HitRate@K。

**3. 记忆系统考虑了长期运行风险**

短期记忆有窗口和摘要，长期记忆有全局上限、单会话上限、置信度、作用域、过期和安全过滤。避免了“什么都记、越跑越脏”的问题。

**4. 工具调用有安全边界**

Prometheus、日志、文档、MySQL 等工具都被包装成明确输入输出。MySQL 只允许 SELECT，要求表白名单，并自动加 LIMIT。日志和知识库工具失败时也不会让整个请求不可用。

**5. 生产治理比较完整**

项目包含 Prompt Guard、Output Filter、JWT、RBAC、RateLimit、审批流、降级开关、Token Audit、OpenTelemetry、Prometheus metrics 和 pprof。这些能力能体现系统面向真实环境的考虑。

**6. AIOps 分析采用计划式流程**

复杂故障分析不是一次性让模型给答案，而是先规划、再执行、再复核。这样更适合需要多步查证的运维问题。

### 5.2 难点

**1. LLM 输出稳定性**

模型可能受上下文污染、工具返回异常、检索不准影响。项目通过 Prompt Guard、上下文优先级、输出过滤和证据约束降低风险，但仍需要评测和回放来持续验证。

**2. RAG 质量依赖数据工程**

RAG 效果不只取决于 Milvus，还取决于 chunking、metadata、query rewrite、topK、rerank、hybrid 配置和评测集。当前代码已经提供了基础能力，真正上线还需要持续评测。

**3. 工具依赖不稳定**

Prometheus、MCP、Milvus、Redis、LLM 都可能超时或不可用。项目需要在局部失败时给出降级说明，而不是直接让用户请求失败。

**4. 记忆可能污染上下文**

长期记忆有价值，但也可能过期或与当前问题无关。项目通过 scope、confidence、safety label、expires_at 和 token budget 做过滤，面试时可以强调“记忆只作为参考，不等同实时证据”。

**5. 成本和并发控制**

LLM 调用有成本和延迟。项目通过 token audit、daily limit、并发信号量、超时、重试和缓存来控制成本，但仍需要线上指标持续观察。

### 5.3 可以主动提的改进方向

| 方向 | 说明 |
| --- | --- |
| RAG 评测闭环 | 固化 build/holdout split，持续跟踪 Recall@K、HitRate@K |
| 证据置信度 | 根据来源、时间、结构化程度、检索分数计算统一 confidence |
| 日志工具连接复用 | 对 MCP 工具 discovery 和 client 做缓存，降低冷启动成本 |
| 更强审批策略 | 从关键词升级为基于角色、环境、资源范围和操作类型的策略 |
| 记忆审计 | 对新增、替换、提升作用域的记忆做可查询审计 |
| 故障回放 | 建立端到端案例回放，覆盖 Chat、AIOps、RAG 和记忆链路 |

## 6. 可能的面试问题与回答要点

### Q1：这个项目是做什么的？

回答要点：

- 面向内部运维团队的 AIOps 智能助手。
- 用户用自然语言提问，系统自动结合告警、日志、文档和历史记忆。
- 输出排障建议、处理步骤或分析报告。
- 核心价值是减少跨系统查询和人工整合证据的成本。

### Q2：为什么使用 Go？

回答要点：

- 运维平台通常需要稳定、并发性能好、部署简单的后端。
- Go 的 goroutine、context、标准库和静态编译适合服务端。
- 当前项目还要处理 HTTP、队列、Redis、Milvus、Prometheus、MCP 等多种外部依赖，Go 的工程性比较合适。

### Q3：Chat 请求完整链路是什么？

回答要点：

- Controller 校验 session、做安全检查和降级判断。
- 同一 session 加锁，避免历史上下文并发错乱。
- 先查缓存。
- 未命中则通过 MemoryService 和 Context Engine 组装上下文。
- Eino ReAct 执行器按需调用工具。
- 输出过滤后返回，并写入记忆和缓存。

### Q4：流式接口和普通接口有什么区别？

回答要点：

- 核心模型链路和上下文逻辑一致。
- 流式接口通过 SSE 推送 meta、thought、message、done。
- 适合前端实时展示长回答。
- 流结束后仍会统一写入记忆。

### Q5：AIOps 分析和普通 Chat 有什么区别？

回答要点：

- 普通 Chat 更偏即时问答。
- AIOps 更偏复杂运维分析。
- AIOps 会先经过高危操作审批。
- 执行上采用计划、执行、复核的流程。
- 输出更强调证据、结论和处理建议。

### Q6：Plan-Execute-Replan 是怎么工作的？

回答要点：

- Planner 先根据问题生成计划。
- Executor 按计划调用工具并整理结果。
- Replanner 判断结果是否足够，不足则补充计划。
- 适合复杂排障，因为它比单轮回答更有结构。

### Q7：RAG 检索链路是怎么设计的？

回答要点：

- 查询入口是 `rag.Query`。
- 通过 `RetrieverPool` 复用 Milvus Retriever。
- 默认可以走向量检索。
- 增强模式支持 query rewrite 和 rerank。
- 混合模式支持 dense retrieval + BM25 + RRF。
- 检索后还会基于 metadata 做二次排序增强。

### Q8：为什么需要 `RetrieverPool`？

回答要点：

- Milvus Retriever 初始化成本较高，并依赖配置、client 和 embedding。
- 每次请求都新建会增加延迟和依赖压力。
- Pool 复用成功初始化的 retriever。
- 初始化失败也会短时间缓存错误，避免高并发下反复冲击依赖。

### Q9：文档索引流程如何保证幂等？

回答要点：

- `IndexingService.IndexSource` 会先根据 source 删除旧数据。
- 再执行 FileLoader、Splitter、Milvus Indexer。
- 最后同步 BM25。
- 同一文件重复索引不会产生重复 chunk。

### Q10：Context Engine 解决了什么问题？

回答要点：

- 解决 LLM 上下文不可控的问题。
- 它统一处理历史、长期记忆、文档和工具结果。
- 每类上下文都有 token budget 和窗口限制。
- 会记录 selected、dropped、reason 和检索耗时。
- 便于排查为什么模型没看到某些资料。

### Q11：记忆系统怎么设计？

回答要点：

- SimpleMemory 管当前会话短期窗口和摘要。
- LongTermMemory 管跨会话的长期信息。
- 对话结束后异步抽取记忆候选。
- 候选会经过长度、代码块、敏感信息、模板话术等过滤。
- 长期记忆有 scope、confidence、safety label、provenance 和 expires_at。

### Q12：如何避免记忆污染模型？

回答要点：

- 检索长期记忆时按 scope 过滤。
- 低 confidence 不进入上下文。
- disabled、superseded、过期或不安全 label 会被过滤。
- Context Engine 有 token budget，避免记忆挤占主要问题上下文。
- 记忆只作为参考，不当作实时证据。

### Q13：Prompt 注入如何防？

回答要点：

- 输入侧有 Prompt Guard。
- 检测英文 ignore previous instructions、system prefix、`[inst]` 等模式。
- 也检测中文“忽略之前指令”“你现在是”等模式。
- 文档和工具结果在提示词中被声明为参考资料，不具备系统指令优先级。

### Q14：输出侧如何防止泄露？

回答要点：

- Output Filter 会过滤 system prompt block、system line、`[inst]` block。
- 会替换 API key、Bearer token、内网 IP 等敏感内容。
- Chat 和 AIOps 返回前都会经过过滤。

### Q15：高危操作审批怎么做？

回答要点：

- AIOps 请求先进入 ApprovalGate。
- 检测 delete、drop、rollback、restart、删除、修改、回滚、重启等关键词。
- 命中后写入审批队列，返回 `approval_required`。
- 审批通过后带 bypass context 再执行原请求。

### Q16：MySQL 工具如何保证安全？

回答要点：

- 只允许单条 SELECT。
- 禁止 DROP、DELETE、UPDATE、INSERT、ALTER、TRUNCATE、SLEEP、BENCHMARK、FOR UPDATE 等关键词。
- 去掉注释和字符串后再做检查。
- 表必须在 `mysql.allowed_tables` 白名单中。
- 最终包装为子查询并加 LIMIT。

### Q17：系统如何做降级？

回答要点：

- 支持配置项 `degradation.kill_switch`。
- 支持 Redis key 动态降级。
- 服务入口先检查降级，开启时返回统一降级消息。
- 外部工具失败时尽量返回部分结果和明确原因。

### Q18：如何控制 LLM 成本？

回答要点：

- 模型层统一 wrapper。
- 调用前检查 daily token limit。
- 调用后记录 prompt、completion、total token。
- Redis 保存 token audit。
- 响应缓存减少重复请求。
- 并发信号量控制同时调用数量。

### Q19：项目可观测性有哪些？

回答要点：

- HTTP 层有 tracing 和 metrics middleware。
- 模型层记录模型名、耗时、状态、token usage。
- `/metrics` 暴露 Prometheus 指标。
- `/readyz` 做依赖就绪检查。
- pprof 用于性能诊断。
- 日志里包含 session 和 request ID，便于排查。

### Q20：如果 Milvus 不可用会怎样？

回答要点：

- Retriever 初始化失败会被短 TTL 缓存。
- 文档检索阶段会记录 documents unavailable。
- 知识库相关能力降级，但普通对话和其他工具仍可继续。
- 返回时应该明确说明知识检索不可用，避免编造文档依据。

### Q21：这个项目最大的技术亮点是什么？

回答要点：

- 上下文工程独立化。
- RAG 链路可评测、可扩展。
- AIOps 采用计划式分析流程。
- 记忆系统有治理字段和异步抽取。
- 安全、审批、成本和可观测性贯穿主链路。

### Q22：如果继续优化，你会做什么？

回答要点：

- 建立 RAG 评测集和回归指标。
- 增加端到端故障案例回放。
- 强化证据置信度计算。
- 优化日志 MCP 连接复用和失败缓存。
- 将审批策略从关键词升级为规则引擎。
- 增加记忆审计和人工管理能力。

## 7. 面试时的项目讲述模板

可以按 4 段讲：

**第一段：背景**

> 这个项目是给内部运维团队用的智能运维助手。排障时指标、日志、知识库、数据库和历史经验分散在多个系统里，值班人员需要手动拼证据，所以我们做了一个通过 LLM、RAG 和工具调用整合这些能力的平台。

**第二段：架构**

> 后端用 GoFrame，LLM 编排用 Eino。普通对话走 ReAct，可以按需调用 Prometheus、日志、知识库和时间等工具。复杂运维分析走 Plan-Execute-Replan，先规划、再执行、再复核，同时接入审批、记忆、降级和输出过滤。

**第三段：亮点**

> 我重点会讲上下文工程和 RAG。Context Engine 会按 history、memory、documents、tool results 分阶段裁剪，并记录 dropped reason；RAG 支持 retriever 复用、query rewrite、rerank、BM25 和混合检索，后续可以用评测集持续优化。

**第四段：治理**

> 因为这是运维场景，安全和稳定性很重要。项目加了 Prompt Guard、Output Filter、高危操作审批、Token Audit、全局降级开关、Prometheus metrics 和 tracing，目标不是只跑通 Demo，而是具备向生产可用演进的基础。
