# OnCallAI 生产级加固设计规划与评审

## CoT 思维链

### 第一步：理解当前系统现状

当前 OnCallAI 是一个 AIOps Multi-Agent 系统，技术栈如下：

- 语言：Go 1.24 + GoFrame v2
- AI 框架：cloudwego/eino（chat pipeline、embedding、retriever）
- 向量数据库：Milvus
- 关系数据库：MySQL（GORM）
- 缓存：Redis（GoFrame redis adapter）
- 前端：独立 SPA（SuperBizAgentFrontend）
- 部署：Docker Compose + Caddy + 阿里云 ECS 单机
- CI/CD：GitHub Actions → ACR → ECS SSH 部署
- LLM 后端：GLM-4.5-AIR（智谱 AI）、SiliconFlow embedding

系统已有能力：

| 能力 | 现状 |
| --- | --- |
| 熔断器 | `utility/resilience/resilience.go`，带 CircuitBreaker + 超时 + 重试 |
| 限流 | `utility/auth/rate_limiter.go`，支持 Redis 和 in-memory 双模 |
| 审批门控 | `internal/ai/service/approval_gate.go`，静态关键词拦截 |
| Token 预算 | `utility/mem/token_budget.go` + `contextengine/assembler.go` |
| Context 装配 | `internal/ai/contextengine/`，含 history/memory/docs/tools 四阶段选择 |
| 健康检查 | `/healthz` + `/readyz` + Dockerfile HEALTHCHECK |
| Trace 回溯 | Ledger + Artifact Store + `/api/ai_ops_trace` |
| JWT 认证 | `utility/auth/jwt.go` + middleware |
| CORS | middleware 支持，配置可控 |

### 第二步：逐项评估差距

针对用户提出的每个维度，我需要回答三个问题：

1. 当前有什么
2. 缺什么
3. 该怎么补

### 第三步：按优先级排列

生产上线的优先级排列原则：

- P0：不做就不能上线（安全、数据不丢、服务可恢复）
- P1：上线后一个月内必须补（可观测性、成本控制）
- P2：持续迭代（性能优化、高级安全）

### 第四步：输出可执行的 workstream

每个 workstream 要说清楚：改哪里、怎么改、验收标准。

---

## 1. 高可用架构设计

### 1.1 当前现状

- 单 ECS 实例 + Docker Compose
- 单 backend 实例，无水平扩展
- Caddy 做反向代理和自动 HTTPS
- 无负载均衡，无健康检查联动

### 1.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 单点故障 | P0 | 后端只有一个实例，挂了就全挂 |
| 无 graceful shutdown | P1 | 进程收到 SIGTERM 后未等待 in-flight 请求完成 |
| 无 readiness 联动 | P1 | `/readyz` 存在但始终返回 ready，未检查依赖健康 |
| 无滚动更新 | P1 | `docker compose up -d` 有短暂不可用窗口 |
| 无多副本 | P2 | 当前阶段可接受，但需要为后续扩展做准备 |

### 1.3 设计要求

#### WS-HA-1：增强 readiness 探针（P0）

改动文件：`main.go`

要求：

- `/readyz` 检查 Redis 连通性
- `/readyz` 检查 Milvus 连通性（如果配置了 MILVUS_ADDRESS）
- 返回 `{"ready": true/false, "checks": {...}}` 结构

验收标准：

- 当 Redis 不可达时返回 503
- 当 Milvus 不可达时返回 503
- 当所有依赖正常时返回 200

#### WS-HA-2：Graceful Shutdown（P0）

改动文件：`main.go`

要求：

- 监听 SIGTERM / SIGINT
- 先停止接收新请求
- 等待 in-flight 请求完成（最长 30s）
- 关闭 Redis / MySQL / Milvus 连接

验收标准：

- 在 `docker stop` 期间，已接收请求不被截断
- 日志输出 shutdown 进度

#### WS-HA-3：部署零停机（P1）

改动文件：`deploy/docker-compose.prod.yml`、`deploy/remote-deploy.sh`

要求：

- docker compose 使用 `--wait` 等待新容器 healthy 后再销毁旧容器
- 或引入简单的蓝绿切换脚本
- health check interval 和 start-period 调优

验收标准：

- 部署期间 curl 不出现 5xx

---

## 2. 缓存策略

### 2.1 当前现状

- RetrieverPool 有初始化失败缓存（failure TTL）
- Rate limiter 支持 Redis 和 in-memory 双模降级
- 无 LLM 响应缓存
- 无 Semantic Cache

### 2.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 无 LLM 响应缓存 | P1 | 相同问题重复调用 LLM，浪费 token |
| 无 KV Cache for context | P2 | context assembler 每次从头装配 |
| 无 Semantic Cache | P2 | 语义相似问题无法复用 |

### 2.3 设计要求

#### WS-CACHE-1：LLM 响应 KV 缓存（P1）

新增文件：`utility/cache/llm_cache.go`

要求：

- 对非流式 chat 请求，按 (session_id + query_hash) 做 Redis 缓存
- TTL 可配置（建议默认 5 分钟）
- 缓存命中时直接返回，不调 LLM
- 流式请求不缓存
- 配置开关 `cache.llm_response.enabled`

验收标准：

- 相同问题 5 分钟内第二次请求不产生 LLM 调用
- 缓存命中/未命中有日志和计数

#### WS-CACHE-2：Semantic Cache 设计（P2）

新增文件：`internal/ai/cache/semantic_cache.go`

要求：

- 使用现有 embedding model 对 query 做向量化
- 在 Milvus 中维护一个 `semantic_cache` collection
- 余弦相似度 > 0.95 视为命中
- 命中时返回缓存的 answer
- TTL 24h
- 配置开关 `cache.semantic.enabled`

验收标准：

- 语义相近问题能命中缓存
- 缓存过期后自动失效
- 不影响正常请求延迟

---

## 3. 异步任务队列与重试机制

### 3.1 当前现状

- `resilience.Execute` 已支持重试 + 超时 + 熔断
- Memory extraction 已使用 bounded async context
- 无持久化任务队列
- Multi-Agent dispatch 是同步调用链

### 3.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| LLM 调用未接入 resilience | P0 | specialist 直接调 LLM，无重试和超时保护 |
| 无持久化任务队列 | P2 | 当前 Ledger 做了持久化，但不是真正的队列 |
| 重试策略未统一 | P1 | 各 specialist 各自处理超时，策略不一致 |

### 3.3 设计要求

#### WS-ASYNC-1：LLM 调用统一接入 resilience（P0）

改动文件：

- `internal/ai/agent/chat_pipeline/orchestration.go`
- `internal/ai/agent/skillspecialists/*/agent.go`
- `internal/ai/agent/reporter/reporter.go`
- `internal/ai/agent/triage/triage.go`

要求：

- 所有 LLM 调用包裹在 `resilience.Execute` 中
- 默认配置：timeout=30s, maxRetries=2, retryDelay=1s
- 每个 agent 使用自己的 breaker name

验收标准：

- LLM 超时时自动重试
- 连续失败后触发熔断，返回 degraded 结果
- 有日志记录重试和熔断状态

#### WS-ASYNC-2：统一 Agent 调用超时（P1）

改动文件：`internal/ai/runtime/runtime.go`

要求：

- `Dispatch` 增加可配置的 per-agent timeout
- 超时后标记 task 为 failed 并 emit event
- supervisor 并发调用 specialist 时各自独立超时

验收标准：

- 单个 specialist 超时不阻塞整体流程
- 超时事件在 trace 中可见

---

## 4. 降级与熔断

### 4.1 当前现状

- `utility/resilience/` 已实现 CircuitBreaker（Closed / Open / HalfOpen）
- 有 `GetBreaker(name)` 全局 breaker 注册
- metrics specialist 已有 degraded 返回逻辑
- logs specialist 有 MCP 不可用时的 degraded 逻辑

### 4.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| breaker 未接入 LLM 调用路径 | P0 | 见 WS-ASYNC-1 |
| 无全局降级开关 | P1 | 无法在紧急情况下一键关闭 AI 能力 |
| 降级响应缺少统一格式 | P1 | 各 specialist 降级返回格式不一致 |
| 无 LLM Provider 切换 | P2 | 当 GLM 不可用时无法切到备用模型 |

### 4.3 设计要求

#### WS-DEGRADE-1：全局降级开关（P1）

改动文件：`manifest/config/config.yaml`、`internal/ai/service/ai_ops_service.go`

要求：

- 新增配置 `degradation.kill_switch: false`
- 当 kill_switch=true 时，所有 AI 请求返回预设兜底消息
- 支持通过 Redis key 动态切换，无需重启

验收标准：

- 设置 Redis key 后立即生效
- 所有入口（chat/chat_stream/ai_ops）统一降级

#### WS-DEGRADE-2：统一降级响应格式（P1）

改动文件：`internal/ai/protocol/types.go`

要求：

- `TaskResult` 的 `Status=degraded` 时必须携带 `DegradationReason`
- reporter 在汇总时保留所有 degraded 原因
- 前端展示降级提示

验收标准：

- degraded 结果在 API response 中明确标注
- trace 中可查到降级原因

#### WS-DEGRADE-3：LLM Provider Fallback（P2）

新增文件：`internal/ai/models/fallback.go`

要求：

- 配置多个 LLM provider（primary + fallback）
- primary 连续失败后自动切到 fallback
- fallback 响应中标记 `provider=fallback`

验收标准：

- primary 不可用时自动切换
- 切换事件有日志和 metric

---

## 5. 可观测性（Observability）

### 5.1 当前现状

- go.mod 中已引入 `go.opentelemetry.io/otel`（作为间接依赖）
- 有 trace 概念（TaskEvent + TraceID + Ledger）但不是 OTel trace
- 有 `g.Log()` 日志但无结构化 metric export
- 无 Prometheus 自身指标暴露（只查询外部 Prometheus）
- 无 Jaeger 链路追踪

### 5.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 无应用级 Prometheus metrics | P0 | 无法监控请求量、延迟、错误率 |
| 无 OTel 链路追踪 | P1 | 无法可视化跨 agent 调用链路 |
| 无结构化日志 | P1 | 当前日志是 printf 风格，不便于索引 |
| 无 SLI/SLO 定义 | P1 | 无法量化服务质量 |

### 5.3 设计要求

#### WS-OBS-1：Prometheus 自身指标暴露（P0）

新增文件：`utility/metrics/metrics.go`

改动文件：`main.go`

要求：

- 暴露 `/metrics` endpoint
- 核心指标：
  - `oncallai_http_requests_total{method, path, status}`
  - `oncallai_http_request_duration_seconds{method, path}`
  - `oncallai_llm_calls_total{agent, model, status}`
  - `oncallai_llm_call_duration_seconds{agent, model}`
  - `oncallai_llm_tokens_total{agent, model, type}` (prompt/completion)
  - `oncallai_agent_dispatch_total{agent, status}`
  - `oncallai_agent_dispatch_duration_seconds{agent}`
  - `oncallai_circuit_breaker_state{name}`
  - `oncallai_cache_hits_total{type}`
  - `oncallai_cache_misses_total{type}`

验收标准：

- curl /metrics 返回 Prometheus 格式文本
- Grafana dashboard 可展示核心指标

#### WS-OBS-2：OpenTelemetry + Jaeger 链路追踪（P1）

新增文件：`utility/tracing/tracing.go`

改动文件：`main.go`、`internal/ai/runtime/runtime.go`

要求：

- 初始化 OTel TracerProvider，export 到 Jaeger
- HTTP middleware 自动创建 root span
- `runtime.Dispatch` 为每个 agent 调用创建 child span
- LLM 调用创建 child span（带 model name、token count）
- span attributes 包含 session_id、agent_name、task_id
- 配置项 `tracing.enabled`、`tracing.jaeger_endpoint`

验收标准：

- 一次 AI Ops 请求在 Jaeger 中可看到完整调用链
- span 包含 agent 名称、耗时、状态
- 可以按 session_id 搜索

#### WS-OBS-3：结构化日志（P1）

改动文件：所有使用 `g.Log()` 的文件

要求：

- 日志切换到 JSON 格式输出
- 每条日志包含 trace_id、session_id、agent（如适用）
- 日志级别：Error/Warn 用于异常，Info 用于关键步骤，Debug 用于详细信息
- 生产环境默认 Info 级别

验收标准：

- 日志可被 ELK/Loki 解析
- 每条日志可通过 trace_id 关联

#### WS-OBS-4：SLI/SLO 定义（P1）

新增文件：`docs/sli_slo.md`（仅在团队需要时创建）

定义：

| SLI | 目标 SLO | 测量方式 |
| --- | --- | --- |
| 请求可用性 | 99.5% | 200 响应 / 总请求 |
| P95 延迟（Chat） | < 5s | histogram |
| P95 延迟（AI Ops） | < 30s | histogram |
| LLM 调用成功率 | > 95% | counter |
| 降级率 | < 10% | degraded / total |

---

## 6. 成本监控与 Token 审计

### 6.1 当前现状

- `utility/mem/token_budget.go` 有 token 估算能力
- context assembler trace 记录了各阶段 token 使用
- 无 LLM 实际 token 消耗追踪
- 无成本聚合和告警

### 6.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| LLM 响应中 usage 字段未采集 | P0 | 无法知道实际消耗了多少 token |
| 无 per-session/per-user token 审计 | P1 | 无法做成本归因 |
| 无成本告警 | P1 | 无法发现异常消耗 |
| 无 token 用量 dashboard | P1 | 无法监控趋势 |

### 6.3 设计要求

#### WS-COST-1：LLM Usage 采集（P0）

改动文件：

- `internal/ai/models/open_ai.go`
- `internal/ai/agent/chat_pipeline/orchestration.go`
- `internal/ai/agent/triage/triage.go`
- `internal/ai/agent/reporter/reporter.go`

要求：

- 从 LLM 响应中提取 `usage.prompt_tokens` 和 `usage.completion_tokens`
- 写入 Prometheus metric `oncallai_llm_tokens_total`
- 写入 TaskEvent payload，使 trace 可回溯 token 消耗

验收标准：

- 每次 LLM 调用后 metric 计数增加
- trace 事件中可看到 token 消耗

#### WS-COST-2：Per-Session Token 审计（P1）

新增文件：`internal/ai/service/token_audit.go`

要求：

- 维护 Redis hash：`token_audit:{date}:{session_id}`
- 记录每次 LLM 调用的 prompt_tokens、completion_tokens
- 提供 API 查询单个 session 的累计消耗
- 配置 `cost.daily_limit_tokens` 做硬限制

验收标准：

- 超过日限额时返回 429 + 原因
- 可查询任意 session 的 token 消耗历史

#### WS-COST-3：成本告警（P1）

要求：

- Prometheus alert rule：单小时 token 消耗超过阈值
- Prometheus alert rule：单用户日消耗超过阈值
- 告警发到 webhook（钉钉/飞书/Slack）

验收标准：

- 模拟高消耗场景触发告警
- 告警消息包含 user_id、session_id、token_count

---

## 7. 性能分析与优化

### 7.1 当前现状

- supervisor 已并发调用 specialist（goroutine + WaitGroup）
- RetrieverPool 有初始化缓存，避免重复创建
- Runtime 按 dataDir 复用实例
- Context assembler 有 token budget 控制

### 7.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 无 pprof 暴露 | P1 | 无法在线分析性能瓶颈 |
| embedding 调用无缓存 | P1 | 相同文档重复 embedding |
| specialist 无并发度限制 | P1 | 并发请求过多时可能打爆 LLM quota |
| 无连接池管理 | P2 | HTTP client 未复用 |

### 7.3 设计要求

#### WS-PERF-1：pprof 暴露（P1）

改动文件：`main.go`

要求：

- 非生产环境暴露 `/debug/pprof/`
- 生产环境通过配置 `debug.pprof_enabled` 控制
- 建议绑定到 `127.0.0.1` 的独立端口

验收标准：

- `go tool pprof` 可远程采样

#### WS-PERF-2：LLM 并发度限制（P1）

新增文件：`utility/resilience/semaphore.go`

要求：

- 全局 LLM 调用信号量，限制并发 LLM 请求数
- 配置项 `llm.max_concurrent_calls`（建议默认 10）
- 超过限制时排队等待，超时返回 degraded

验收标准：

- 并发 20 个请求时，只有 10 个同时调 LLM
- 剩余请求排队或超时降级

---

## 8. 安全性（Security）

### 8.1 Prompt 注入防御

#### 8.1.1 当前现状

- ApprovalGate 对高危关键词做静态拦截
- Context assembler 有 `SafetyLabel` 字段但未实际使用
- 无 prompt sanitization

#### 8.1.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 无输入 sanitization | P0 | 用户可注入 system prompt 覆盖指令 |
| 无输出 sanitization | P1 | LLM 可能在输出中泄露 system prompt |
| SafetyLabel 未生效 | P2 | context item 的 safety 标记未做过滤 |

#### 8.1.3 设计要求

##### WS-SEC-1：Prompt 注入检测与过滤（P0）

新增文件：`utility/safety/prompt_guard.go`

改动文件：

- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`
- `internal/controller/chat/chat_v1_ai_ops.go`

要求：

- 在 controller 层对用户输入做 sanitization
- 检测常见注入模式：
  - `ignore previous instructions`
  - `you are now`
  - `system:`
  - `[INST]`、`<<SYS>>`
  - 中文变体：`忽略之前的指令`、`你现在是`
- 检测到注入时记录告警日志并返回拒绝响应
- 配置开关 `safety.prompt_guard.enabled`

验收标准：

- 典型注入 payload 被拦截
- 正常业务问题不受影响
- 有单测覆盖

##### WS-SEC-2：输出脱敏（P1）

新增文件：`utility/safety/output_filter.go`

要求：

- LLM 输出经过 output filter 再返回给用户
- 过滤 system prompt 片段泄露
- 过滤敏感信息模式（API key pattern、内部 IP 等）

验收标准：

- LLM 输出中不会泄露 system prompt
- 有单测

### 8.2 权限控制与沙盒隔离

#### 8.2.1 当前现状

- JWT 认证已实现但默认关闭（`auth.enabled: false`）
- 有 `consts.CtxKeyUserRole` 但未做 RBAC
- MySQL CRUD tool 对所有认证用户可用
- 无 query 沙盒（SQL 注入风险）

#### 8.2.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| auth 默认关闭 | P0 | 生产必须启用认证 |
| 无 RBAC | P1 | 无法区分 admin / operator / viewer |
| MySQL tool 无沙盒 | P0 | 用户可通过 AI 执行任意 SQL |
| 无 API key 管理 | P2 | 当前只有 JWT |

#### 8.2.3 设计要求

##### WS-SEC-3：生产环境强制认证（P0）

改动文件：`deploy/config.prod.yaml`

要求：

- 生产配置 `auth.enabled: true`
- 部署脚本校验 `AUTH_JWT_SECRET` 是否为强密钥

验收标准：

- 未带 token 请求返回 401
- CI/CD 部署校验通过

##### WS-SEC-4：MySQL Tool 沙盒（P0）

改动文件：`internal/ai/tools/mysql_crud.go`

要求：

- MySQL tool 只允许 SELECT 语句
- 禁止 DROP / DELETE / UPDATE / INSERT / ALTER / TRUNCATE
- 配置可选的允许查询的表白名单
- 查询结果行数限制（默认 100 行）
- 查询超时（默认 10s）

验收标准：

- 尝试执行 `DROP TABLE` 被拒绝
- 有单测覆盖各种绕过尝试

##### WS-SEC-5：RBAC 基础（P1）

改动文件：`utility/auth/jwt.go`、`utility/middleware/middleware.go`

要求：

- JWT claims 包含 `role` 字段
- middleware 校验 role
- 定义 3 个角色：
  - `admin`：所有权限
  - `operator`：chat + ai_ops + trace 查询
  - `viewer`：chat only
- ai_ops 端点需要 operator 或 admin
- trace 查询端点需要 operator 或 admin

验收标准：

- viewer 访问 ai_ops 返回 403
- 有单测

### 8.3 人机协作边界（Human-in-the-loop）

#### 8.3.1 当前现状

- `StaticApprovalGate` 做静态关键词拦截
- 拦截后返回"未获得审批"消息
- 无异步审批流程
- 无人工确认环节

#### 8.3.2 缺口分析

| 缺口 | 严重度 | 说明 |
| --- | --- | --- |
| 审批只有拒绝无批准 | P1 | 无法让人工确认后继续执行 |
| 无审批队列 | P2 | 被拦截的请求无法恢复 |
| 无分级审批 | P2 | 所有高危操作同一策略 |

#### 8.3.3 设计要求

##### WS-HITL-1：审批流程增强（P1）

改动文件：`internal/ai/service/approval_gate.go`

新增文件：`internal/ai/service/approval_queue.go`

要求：

- 高危操作（含 delete/drop/update 等）进入待审批队列
- 审批记录持久化到 Redis
- 提供 API 查询待审批列表、批准、拒绝
- 批准后继续执行原请求

验收标准：

- 高危操作不直接执行
- 管理员可在界面审批
- 审批记录可追溯

##### WS-HITL-2：操作确认机制（P1）

要求：

- 对 MySQL 查询结果超过一定复杂度时，先返回执行计划让用户确认
- 对多步骤 AI Ops 操作（如回滚建议），先展示方案再执行
- 前端增加"确认执行"按钮

验收标准：

- 用户能看到执行计划后决定是否继续
- 未确认的操作不会执行

---

## 9. .env 安全审查

### 9.1 当前发现的问题

**严重：`.env` 文件中包含真实 API Key**

当前 `.env` 文件包含：

- `GLM_API_KEY=db5d2808...`（真实 key）
- `SILICONFLOW_API_KEY=sk-jkfok...`（真实 key）

### 9.2 设计要求

##### WS-SEC-ENV：密钥安全（P0）

要求：

- `.env` 文件加入 `.gitignore`（已有，确认生效）
- 确认 `.env` 未被 commit 到 git 历史，如有需执行 `git filter-branch` 清理
- 生产环境使用 GitHub Secrets + `PROD_ENV_FILE` 注入（已实现）
- 所有 API key 使用环境变量注入，不硬编码
- 增加启动时 key 格式校验（非 placeholder）

验收标准：

- `git log --all --full-history -- .env` 无真实 key
- 生产环境从 `.env.production` 读取而非 `.env`

---

## 10. 评审总结：优先级矩阵

| 优先级 | Workstream ID | 名称 | 预估工时 |
| --- | --- | --- | --- |
| **P0** | WS-HA-1 | Readiness 探针增强 | 2h |
| **P0** | WS-HA-2 | Graceful Shutdown | 3h |
| **P0** | WS-ASYNC-1 | LLM 调用接入 resilience | 4h |
| **P0** | WS-OBS-1 | Prometheus 自身指标暴露 | 4h |
| **P0** | WS-COST-1 | LLM Usage 采集 | 3h |
| **P0** | WS-SEC-1 | Prompt 注入检测与过滤 | 4h |
| **P0** | WS-SEC-3 | 生产环境强制认证 | 1h |
| **P0** | WS-SEC-4 | MySQL Tool 沙盒 | 3h |
| **P0** | WS-SEC-ENV | 密钥安全 | 1h |
| **P1** | WS-HA-3 | 部署零停机 | 3h |
| **P1** | WS-CACHE-1 | LLM 响应 KV 缓存 | 4h |
| **P1** | WS-ASYNC-2 | 统一 Agent 调用超时 | 3h |
| **P1** | WS-DEGRADE-1 | 全局降级开关 | 2h |
| **P1** | WS-DEGRADE-2 | 统一降级响应格式 | 2h |
| **P1** | WS-OBS-2 | OTel + Jaeger 链路追踪 | 6h |
| **P1** | WS-OBS-3 | 结构化日志 | 4h |
| **P1** | WS-OBS-4 | SLI/SLO 定义 | 2h |
| **P1** | WS-COST-2 | Per-Session Token 审计 | 4h |
| **P1** | WS-COST-3 | 成本告警 | 3h |
| **P1** | WS-PERF-1 | pprof 暴露 | 1h |
| **P1** | WS-PERF-2 | LLM 并发度限制 | 2h |
| **P1** | WS-SEC-2 | 输出脱敏 | 3h |
| **P1** | WS-SEC-5 | RBAC 基础 | 4h |
| **P1** | WS-HITL-1 | 审批流程增强 | 6h |
| **P1** | WS-HITL-2 | 操作确认机制 | 4h |
| **P2** | WS-CACHE-2 | Semantic Cache | 6h |
| **P2** | WS-DEGRADE-3 | LLM Provider Fallback | 4h |

---

## 11. 评审考虑

### 11.1 风险评审

| 风险 | 影响 | 缓解措施 |
| --- | --- | --- |
| `.env` 中真实 API Key 已泄露到 git 历史 | API Key 被盗用 | 立即轮换 key + 清理 git 历史 |
| MySQL Tool 无沙盒 | 用户通过 AI 执行破坏性 SQL | P0 优先加沙盒 |
| 无 Prompt 注入防御 | 攻击者操控 AI 输出 | P0 优先加过滤 |
| 单实例无 graceful shutdown | 部署时请求中断 | P0 优先加 shutdown |
| LLM 调用无重试保护 | LLM 抖动导致全量失败 | P0 优先接入 resilience |
| 无 token 消耗监控 | 成本失控 | P0 优先加 usage 采集 |

### 11.2 架构评审

当前架构的优势：

1. Multi-Agent runtime 设计干净，Ledger + Bus + Artifact 分层明确
2. Context Engine 有完整的 budget 控制和 trace 能力
3. Skills 化设计使能力扩展不需要修改核心 agent
4. Resilience 基础设施已存在（CircuitBreaker + retry + timeout）

当前架构的短板：

1. 可观测性几乎为零——没有 Prometheus metrics、没有 OTel tracing、日志不结构化
2. 安全边界不完整——auth 默认关闭、无 prompt guard、SQL tool 无沙盒
3. 成本不可控——没有 token usage 采集、没有 per-user 限制
4. 单实例部署——无法水平扩展，但当前阶段可接受

### 11.3 执行建议

**Phase 1（上线前必须完成，约 25h）：**

执行所有 P0 workstream：
- WS-HA-1 + WS-HA-2（可用性基础）
- WS-ASYNC-1（调用保护）
- WS-OBS-1（最小可观测）
- WS-COST-1（成本可视）
- WS-SEC-1 + WS-SEC-3 + WS-SEC-4 + WS-SEC-ENV（安全底线）

**Phase 2（上线后两周内，约 35h）：**

执行高优 P1 workstream：
- WS-OBS-2 + WS-OBS-3（可观测完善）
- WS-DEGRADE-1 + WS-DEGRADE-2（降级体系）
- WS-COST-2 + WS-COST-3（成本管控）
- WS-SEC-5 + WS-HITL-1（安全增强）

**Phase 3（上线后一个月内，约 20h）：**

执行剩余 P1 和 P2 workstream。

### 11.4 依赖说明

| Workstream | 依赖 |
| --- | --- |
| WS-OBS-1 | 需引入 `prometheus/client_golang` |
| WS-OBS-2 | 需引入 `go.opentelemetry.io/otel` 直接依赖 + Jaeger exporter |
| WS-CACHE-1 | 依赖 Redis |
| WS-CACHE-2 | 依赖 Milvus + embedding model |
| WS-COST-2 | 依赖 WS-COST-1 |
| WS-COST-3 | 依赖 WS-OBS-1 |
| WS-DEGRADE-3 | 需配置备用 LLM provider |
| WS-HITL-1 | 依赖 Redis |

### 11.5 不做的决定及原因

| 不做的事 | 原因 |
| --- | --- |
| K8s 迁移 | 当前单机 Docker Compose 足够，K8s 引入过多复杂度 |
| 消息队列（Kafka/RabbitMQ） | 当前 Ledger + goroutine 满足需求，无需引入新中间件 |
| 多 region 部署 | 用户量不到多 region 门槛 |
| GPU 推理服务 | 使用外部 LLM API，不自建推理 |
| 数据库分库分表 | 当前数据量不需要 |