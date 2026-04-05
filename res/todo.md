# OnCallAI 修改记录与复盘文档

## 1. 文档目的

本文档用于系统记录最近两轮与 AI 能力相关的改动：

- 上一轮：Multi-Agent MVP 落地
- 本轮：P1 能力实现与指定问题修复

记录重点包括：

- 功能实现的详细说明和代码变更
- 每个问题的修复方案、代码变更点和测试结果
- 关键修改原因和技术选型依据
- 对性能、安全性、可维护性的影响评估

本文档同时作为后续代码审查和个人复盘材料。

---

## 2. 时间线概览

### 2.1 上一轮改动

目标：

- 按《Multi-Agent 系统实施设计稿》落地第一阶段 MVP
- 将 AI Ops 从单链路切换为单体内 Multi-Agent Runtime

结果：

- 已完成 `protocol + runtime + supervisor + triage + specialists + reporter` 的最小可运行骨架
- AI Ops 控制器已切换到新链路
- 已补最小单元测试并通过编译和测试验证

### 2.2 本轮改动

目标：

- 实现 P1 能力
- 修复用户列出的 9 类问题
- 将修改内容整理到 `res/todo.md`

结果：

- 已实现 P1 的关键基础设施
- 已修复中高优先级问题
- 已完成回归测试和构建验证

---

## 3. 上一轮改动：Multi-Agent MVP

### 3.1 功能目标

将 AI Ops 从原先的单链路/轻量代理式工作流，切换为具备统一协议和运行时的 Multi-Agent MVP。

### 3.2 主要实现内容

#### 3.2.1 协议层

新增：

- `internal/ai/protocol/types.go`

实现内容：

- `TaskEnvelope`
- `TaskResult`
- `TaskEvent`
- `ArtifactRef`
- `Artifact`
- `MemoryRef`

目的：

- 统一 agent 间消息结构
- 为后续 task ledger / artifact store / trace 提供基础对象

#### 3.2.2 Runtime 骨架

新增目录：

- `internal/ai/runtime/`

主要文件：

- `agent.go`
- `registry.go`
- `runtime.go`
- `ledger.go`
- `bus.go`
- `artifacts.go`
- `context.go`
- `runtime_test.go`

实现内容：

- Agent 注册中心
- 进程内任务分发
- 事件发布
- 内存版 ledger
- 内存版 artifact store
- runtime context 注入

技术选型原因：

- 第一阶段不引入网络和分布式复杂度
- 优先验证业务编排模型
- 保持现有 Go 单体服务结构不变

#### 3.2.3 Agent 拆分

新增：

- `internal/ai/agent/supervisor/`
- `internal/ai/agent/triage/`
- `internal/ai/agent/specialists/metrics/`
- `internal/ai/agent/specialists/logs/`
- `internal/ai/agent/specialists/knowledge/`
- `internal/ai/agent/reporter/`

角色说明：

- `Supervisor`
  - 负责 orchestrate 整个 AI Ops 任务
- `Triage`
  - 负责识别任务类型和路由域
- `Metrics Agent`
  - 负责 Prometheus 活跃告警查询
- `Log Agent`
  - 负责 MCP 日志工具发现和探测
- `Knowledge Agent`
  - 负责知识库检索
- `Reporter`
  - 负责聚合 specialist 输出并生成报告

#### 3.2.4 AI Ops 接入

修改：

- `internal/controller/chat/chat_v1_ai_ops.go`
- `internal/ai/service/ai_ops_service.go`

实现内容：

- AI Ops 控制器改为走 Multi-Agent Runtime
- 保留原有 `result + detail` 响应结构，兼容前端

### 3.3 上一轮验证结果

执行过的验证：

- `env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./...`
- `env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go build ./...`

结果：

- 通过

### 3.4 上一轮影响评估

#### 正向影响

- AI Ops 具备了明确的职责拆分
- 后续可以继续演进为真正的 P1/P2 架构
- detail 输出能反映多 agent 执行过程

#### 限制

- 仍是单体内 runtime
- `Log Agent` 仍偏保守，主要做工具发现和优雅降级
- `Ledger` 和 `ArtifactStore` 当时还是内存实现

---

## 4. 本轮改动：P1 实现

本轮目标是将 MVP 从“能跑”提升到“更稳定、可持续、可回溯”的状态。

### 4.1 已实现的 P1 能力

#### 4.1.1 持久化 Task Ledger

新增：

- `internal/ai/runtime/file_store.go`
- `internal/ai/runtime/file_store_test.go`

实现内容：

- `FileLedger`
- Task 持久化到 `var/runtime/ledger/tasks/*.json`
- Result 持久化到 `var/runtime/ledger/results/*.json`
- Event 按 trace 追加到 `var/runtime/ledger/traces/*.jsonl`

原因：

- MVP 版本 ledger 只有内存态，服务重启即丢失
- P1 需要至少有“可回看、可审计、可排障”的最小持久层

技术选型依据：

- 优先选择文件存储而非 MySQL/Redis
- 不增加额外部署依赖
- 对测试环境和本地调试更友好

影响评估：

- 可维护性显著提升
- 对性能影响很小，当前写入量低
- 未来可平滑替换成 MySQL/Redis/NATS 方案

#### 4.1.2 持久化 Artifact Store

新增：

- `internal/ai/runtime/file_store.go`

实现内容：

- `FileArtifactStore`
- Artifact 写入 `var/runtime/artifacts/*.json`
- 支持通过 `ArtifactRef.URI` 读取

原因：

- specialist 结果不应只存在于内存
- 后续做 evidence 回放和调试时需要可读产物

影响评估：

- 有助于后续 replay / audit / reporter 复用
- 对当前运行开销影响可控

#### 4.1.3 Memory Service 封装

新增：

- `internal/ai/service/memory_service.go`

实现内容：

- `ResolveSessionID`
- `InjectContext`
- `PersistOutcome`

接入点：

- `internal/ai/service/ai_ops_service.go`

原因：

- 之前 AI Ops 没有统一 memory 入口
- 直接散用 `utility/mem` 不利于后续治理

技术选型依据：

- 优先做 service 封装，不直接重写底层 memory
- 低风险复用现有 `SimpleMemory` 和 `LongTermMemory`

影响评估：

- 提高记忆接入的一致性
- 为后续 Memory Agent 打下接口基础

#### 4.1.4 审批门骨架

新增：

- `internal/ai/service/approval_gate.go`

实现内容：

- `ApprovalGate`
- `StaticApprovalGate`
- 对高风险关键词动作做拦截

接入点：

- `RunAIOpsMultiAgent`

原因：

- P1 设计中要求具备基础审批门骨架
- 即便当前 AI Ops 主要是只读分析，也应为高风险动作预留控制点

影响评估：

- 安全性提升
- 当前影响范围较小

#### 4.1.5 Artifact 产物接入 Supervisor

修改：

- `internal/ai/agent/supervisor/supervisor.go`

实现内容：

- specialist 返回结果后，自动写入 artifact
- 结果通过 `ArtifactRef` 挂回 `TaskResult`

原因：

- 让 P1 的 artifact store 不是空壳
- 为后续 replay、审计和可观测提供真实数据

---

## 5. 问题修复清单

以下是本轮要求修复的问题及处理情况。

---

## 6. 问题 1：Revoked Token 内存泄漏

### 严重度

中

### 问题说明

原实现：

- `utility/auth/jwt.go`
- `revokedTokens` 使用 `sync.Map`
- `RevokeToken` 只写入，不清理

问题：

- 撤销过的 token 会永久留在内存中
- 长期运行后存在内存泄漏风险

### 修复方案

修改文件：

- `utility/auth/jwt.go`
- `utility/auth/jwt_test.go`

实现内容：

- 吊销表不再只存储“吊销时间”，改为存 token 过期时间
- `ValidateToken` 在发现吊销 token 已过期时自动清理
- 增加后台清理协程
- 新增 `clearExpiredRevokedTokens` 用于定时清理和测试

### 代码变更点

- `RevokeToken`
  - 解析 token 的 `exp`
  - 将 exp 存入吊销表
- `ValidateToken`
  - 若吊销 token 已过期，则自动删除
- 新增：
  - `startRevokedTokenCleanup`
  - `clearExpiredRevokedTokens`
  - `loadRevokedTokenExpiry`

### 测试结果

新增测试：

- `TestClearExpiredRevokedTokens`

验证结果：

- `go test ./utility/auth` 通过

### 影响评估

- 安全性：无负面影响
- 性能：轻微增加后台清理开销，可忽略
- 可维护性：提升，吊销逻辑更完整

---

## 7. 问题 2：Long-Term Memory 无上限

### 严重度

中

### 问题说明

原实现：

- `LongTermMemory` 全局常驻
- `Store` 只增不减

问题：

- 全局条目数无上限
- 单 session 条目数也无上限
- 长期运行有内存增长风险

### 修复方案

修改文件：

- `utility/mem/long_term.go`
- `utility/mem/long_term_test.go`
- `manifest/config/config.yaml`

实现内容：

- 新增长期记忆全局上限
- 新增单 session 上限
- 当达到上限时，按 relevance + last used 进行驱逐

配置项：

- `memory.long_term_max_entries`
- `memory.long_term_max_entries_per_session`

### 代码变更点

- 新增默认值：
  - `defaultLongTermMaxEntries`
  - `defaultLongTermMaxEntriesPerSession`
- 新增：
  - `evictIfNeededLocked`
  - `evictOneLocked`
  - `removeEntryLocked`
  - `loadLongTermMaxEntries`
  - `loadLongTermMaxEntriesPerSession`

### 测试结果

新增测试：

- `TestLongTermMemory_RespectsPerSessionCapacity`

验证结果：

- `go test ./utility/mem` 通过

### 影响评估

- 性能：极小量额外排序与驱逐开销
- 内存：显著改善长期运行风险
- 可维护性：提升，memory 生命周期更明确

---

## 8. 问题 3：Triage 使用硬编码关键词匹配

### 严重度

低，MVP 可接受

### 问题说明

原实现是直接 `switch` + `strings.Contains`。

问题：

- 扩展性差
- 路由逻辑不够结构化

### 修复方案

修改文件：

- `internal/ai/agent/triage/triage.go`

实现内容：

- 从硬编码 `switch` 改为规则表 `triageRules`
- 提取 `matchesRule`
- 保留轻量关键词路由，但结构更清晰

### 影响评估

- 行为保持稳定
- 可维护性提升
- 仍然不是 LLM triage，但对当前阶段足够

---

## 9. 问题 4：Reporter 使用已废弃的 strings.Title

### 严重度

低

### 修复方案

修改文件：

- `internal/ai/agent/reporter/reporter.go`

实现内容：

- 使用自定义 `displayAgentName`
- 基于 rune 做首字母大写

### 影响评估

- 行为基本不变
- 消除废弃 API

---

## 10. 问题 5：Metrics Agent 错误处理不一致

### 严重度

低

### 问题说明

原实现：

- tool 执行失败时直接返回 error
- 解析失败或工具失败结果时又返回 degraded

问题：

- 对 AI Ops 链路来说，metrics 属于可降级依赖
- 处理风格不一致会让 supervisor 更难稳定编排

### 修复方案

修改文件：

- `internal/ai/agent/specialists/metrics/agent.go`

实现内容：

- tool 调用失败统一转为 `ResultStatusDegraded`
- 不再把 tool 失败作为整个任务 fatal error 返回

### 影响评估

- 稳定性提升
- 更符合 AI Ops “部分结果可接受”的设计

---

## 11. 问题 6：loadEnvFile 手动实现

### 严重度

低

### 问题说明

原实现：

- `main.go` 中内联了一个手写 `loadEnvFile`

问题：

- 逻辑不便复用
- scanner error 没处理
- 代码放置位置不合理

### 修复方案

修改文件：

- `utility/common/env.go`
- `utility/common/env_test.go`
- `main.go`

实现内容：

- 抽出 `common.LoadEnvFile`
- 支持：
  - 忽略不存在文件
  - 忽略注释
  - 支持 `export KEY=VALUE`
  - 支持 scanner error 返回
- `main.go` 改为调用公共方法

### 测试结果

新增测试：

- `TestLoadEnvFile`

验证结果：

- `go test ./utility/common` 通过

### 影响评估

- 可维护性提升
- 启动配置更清晰

---

## 12. 问题 7：CORS Origin 校验逻辑

### 严重度

低

### 问题说明

原实现：

- `allowed_origins` 为空时，会把任意请求头里的 `Origin` 原样回写
- SSE 直接设置 `Access-Control-Allow-Origin: *`

问题：

- CORS 默认过宽
- SSE 和普通 HTTP 逻辑不一致

### 修复方案

修改文件：

- `utility/middleware/middleware.go`
- `utility/middleware/middleware_test.go`
- `internal/logic/sse/sse.go`

实现内容：

- 抽出 `ResolveAllowedOrigin` / `matchAllowedOrigin`
- 仅当 origin 在允许名单内时才设置 CORS
- 支持 `*` 显式配置
- SSE 改为复用同一套 origin 逻辑
- 增加 `Vary: Origin`

### 测试结果

新增测试：

- `TestResolveAllowedOrigin`

验证结果：

- `go test ./utility/middleware` 通过

### 影响评估

- 安全性提升
- 默认行为更保守
- 对跨域场景要求配置更明确

---

## 13. 问题 8：知识库检索 hardcoded top 3

### 严重度

极低

### 修复方案

修改文件：

- `internal/ai/agent/specialists/knowledge/agent.go`
- `manifest/config/config.yaml`

实现内容：

- 新增 `knowledgeEvidenceLimit()` 读取配置
- 优先读取 `multi_agent.knowledge_evidence_limit`
- 否则退回 `retriever.top_k`
- 最后才默认 3

### 影响评估

- 灵活性提升
- 对性能影响可控

---

## 14. 问题 9：LongTermMemory.Retrieve 用了写锁

### 严重度

低

### 问题说明

原实现：

- `Retrieve` 全程持有写锁

问题：

- 限制并发读性能
- 不必要地放大锁竞争

### 修复方案

修改文件：

- `utility/mem/long_term.go`

实现内容：

- `Retrieve` 前半段改用 `RLock`
- 先计算候选和排序
- 最后仅在更新 `AccessCnt / LastUsed` 时短暂获取写锁

### 影响评估

- 并发读性能更好
- 锁争用减少
- 保持语义正确

---

## 15. 额外修复与优化

### 15.1 限流顺序

此前 review 提到：

- Authenticated requests 仍按 IP 限流

当前代码中 `main.go` 已经是：

- `CORSMiddleware`
- `AuthMiddleware`
- `RateLimitMiddleware`
- `ResponseMiddleware`

也就是说：

- 该问题在当前代码版本中已经不成立
- 本轮未再改动，但在复盘中确认过

### 15.2 AI Ops P1 接入

修改文件：

- `internal/ai/service/ai_ops_service.go`

新增内容：

- 使用持久化 runtime
- 引入 approval gate
- 引入 memory service
- root task 使用统一 session id 和 memory refs

### 15.3 File-backed Runtime

新增文件：

- `internal/ai/runtime/file_store.go`
- `internal/ai/runtime/file_store_test.go`

作用：

- 满足 P1 对 ledger/artifact 持久层的要求

---

## 16. 测试与验证结果

本轮执行并通过的关键命令：

```bash
env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./utility/common ./utility/auth ./utility/mem ./utility/middleware ./internal/ai/runtime ./internal/ai/service ./internal/controller/chat
env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./internal/ai/tools
env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./...
env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go build ./...
```

结果：

- 通过

新增/更新测试覆盖点：

- `utility/common/env_test.go`
- `utility/auth/jwt_test.go`
- `utility/mem/long_term_test.go`
- `utility/middleware/middleware_test.go`
- `internal/ai/runtime/file_store_test.go`
- `internal/ai/runtime/runtime_test.go`
- `internal/ai/agent/triage/triage_test.go`

---

## 17. 技术选型依据总结

### 17.1 为什么 P1 先选文件持久化而不是 MySQL / Redis

原因：

- 不增加部署依赖
- 不引入额外网络故障面
- 本地和测试环境最容易验证
- 对当前数据规模足够

未来演进：

- 可平滑替换为 DB / Redis / 对象存储

### 17.2 为什么 Memory Service 先做封装，不重写底层 memory

原因：

- 当前已有 `SimpleMemory` 和 `LongTermMemory`
- 直接重写风险高
- 先引入 service 边界，后续更容易替换

### 17.3 为什么 Metrics Agent 统一降级

原因：

- AI Ops 是多证据系统
- 某个 evidence source 失败，不应直接让整条链路失败

### 17.4 为什么 CORS 改成保守策略

原因：

- 默认允许任意 Origin 风险过高
- 更符合生产安全要求

---

## 18. 对系统影响评估

### 18.1 性能影响

正向：

- Long-term memory 读锁优化减少锁竞争
- Triage 规则表实现比之前更清晰，开销无明显变化

负向：

- File ledger / artifact store 会增加少量磁盘 IO
- JWT 清理协程增加极轻微后台成本

总体判断：

- 当前影响可接受
- 稳定性收益明显大于性能代价

### 18.2 安全性影响

提升点：

- CORS 更严格
- SSE 不再无条件 `*`
- revoked token 不再无限累积
- 审批门骨架已预留

### 18.3 可维护性影响

提升点：

- env 加载逻辑抽离
- triage 规则结构化
- reporter 去除废弃 API
- memory access 和 runtime persistence 更明确

### 18.4 生产就绪度影响

提升点：

- Multi-Agent 已从 MVP 进入更稳的 P1 阶段
- 有了基本可回溯的 ledger 和 artifact
- 有了基础 memory service 和 approval gate

仍未完成项：

- 分布式 A2A
- 真正的外部持久化存储
- 更强的 replay/eval 体系
- 更完善的 Action/SQL 安全治理

---

## 19. 后续建议

建议下一步继续推进：

1. 将 file-backed ledger/artifact 抽象成可切换后端
2. 为 `Log Agent` 做更强的 MCP 参数适配
3. 补充 AI Ops replay/eval 用例集
4. 为 Multi-Agent 增加结构化 detail 输出格式
5. 为审批门接入真正的人审/配置化策略

---

## 20. 一句话总结

最近两轮改动的核心价值是：

> 把 AI Ops 从“能跑的 Multi-Agent MVP”，推进到“具备基础持久化、记忆封装、审批骨架和关键质量修复的 P1 阶段”，同时补掉了中期运行中最容易积累风险的几类问题。

---

## 21. 下一阶段设计落地：P0 收口实现

本节记录基于《下一阶段推进与复盘手册》继续落地的第一批实现，重点对应：

- 为异步 memory extraction 增加超时保护
- 增强 `Log Agent` 的可交付证据能力
- 建立最小 replay 基线

### 21.1 异步记忆抽取超时保护

修改文件：

- `internal/ai/service/memory_service.go`
- `internal/ai/service/memory_service_test.go`
- `utility/mem/extraction.go`
- `manifest/config/config.yaml`

实现内容：

- `PersistOutcome` 不再直接 `go mem.ExtractMemories(context.Background(), ...)`
- 改为基于 `context.WithoutCancel(parent)` + `context.WithTimeout(...)` 创建有上限的后台上下文
- 新增配置项 `memory.extract_timeout_ms`
- `ExtractMemories` 本身增加 `ctx.Err()` 检查，避免超时后继续工作

原因分析：

- 之前异步记忆抽取脱离请求生命周期且没有超时边界
- 即使当前 `ExtractMemories` 实现较轻，也不应把这种模式保留下去
- 先建立统一异步保护模式，后续即使引入更重的记忆抽取链路也不需要重构调用点

测试结果：

- 新增 `TestPersistOutcomeUsesBoundedContext`
- 验证后台提取上下文包含 deadline，且会按预期超时结束

影响评估：

- 稳定性提升明显
- 对主请求无额外阻塞
- 为后续更复杂的记忆抽取实现保留安全边界

### 21.2 Log Agent 从“探测型”升级为“证据型”

修改文件：

- `internal/ai/agent/specialists/logs/agent.go`
- `internal/ai/agent/specialists/logs/agent_test.go`
- `manifest/config/config.yaml`

实现内容：

- 增加 `multi_agent.log_query_timeout_ms`
- 增加 `multi_agent.log_evidence_limit`
- `Log Agent` 不再只返回“发现了哪些 MCP 工具”
- 会优先尝试调用可 invokable 的日志工具
- 对 JSON / 文本输出做统一 evidence 提取
- 对“初始化失败 / 调用失败 / 有原始输出但无法结构化 / 无工具可用”分别返回稳定 degraded 语义

技术选型依据：

- 不假设外部 MCP 返回固定 schema
- 先做通用 JSON + 纯文本兼容抽取
- 在不引入复杂依赖的前提下，提升日志证据可读性

测试结果：

- 新增 `TestLogAgentReturnsStructuredEvidence`
- 新增 `TestLogAgentDegradesOnInvocationError`
- 新增 `TestBuildLogEvidenceFromPlainText`

影响评估：

- AI Ops 报告的日志域不再只停留在“工具发现”
- 即使日志工具返回不稳定，也能更一致地降级
- 为后续接真实 MCP replay 提供更稳定的输出形式

### 21.3 建立最小 replay 基线

新增文件：

- `internal/ai/agent/supervisor/supervisor_replay_test.go`
- `res/aiops-replay-cases.md`

实现内容：

- 为 supervisor 编排建立 3 个最小 replay case
- 覆盖：
  - 告警分析全域 fanout
  - 知识库单域路由
  - 日志排障双域路由
- 通过 stub specialist 固定输出，稳定验证 orchestration / triage / reporter 的结构行为

原因分析：

- 当前阶段最需要的是“能稳定发现编排回归”
- 先验证 orchestrator，再逐步接真实 tool specialist

影响评估：

- 回归测试能力增强
- 为后续扩展为真实 tool replay 打下基础

### 21.4 本轮验证结果

建议验证命令：

- `env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./internal/ai/service ./internal/ai/agent/specialists/logs ./internal/ai/agent/supervisor ./utility/mem`
- `env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test ./...`
- `env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go build ./...`

### 21.5 本轮结论

这一轮的价值不在于继续增加更多 agent，而在于把“下一阶段设计稿”里的第一批高收益动作真正落到代码和测试中：

- 异步记忆提取有了边界
- 日志域开始产出更稳定的证据
- Multi-Agent 编排开始具备 replay 基线

---

## 22. 后续推进：Runtime 复用、Trace 查询接口、Phase 3 规划

本节记录继续推进后的第二批动作，目标是让当前 P1 资产更可持续，并为 Chat 链路接入 Multi-Agent 做前置准备。

### 22.1 Runtime 实例复用

修改文件：

- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/ai_ops_service_test.go`

实现内容：

- 为 AI Ops runtime 增加按 `dataDir` 维度的实例复用
- 避免每次请求都重新 `NewPersistent + Register(all agents)`
- 新增：
  - `getOrCreateAIOpsRuntime`
  - `getOrCreateAIOpsRuntimeForDir`
  - `registerAIOpsAgents`

原因分析：

- 之前每次请求重复创建 runtime，属于纯初始化浪费
- 当前 runtime 已具备持久化 ledger/artifact 和 trace 能力，更适合复用而不是一次性构造

测试结果：

- 新增 `TestGetOrCreateAIOpsRuntimeReusesInstance`

影响评估：

- 性能：减少重复创建和注册开销
- 可维护性：runtime 生命周期更明确
- 风险：当前按 `dataDir` 复用，配置变更需要显式重启才能生效

### 22.2 Trace 查询接口

修改文件：

- `api/chat/v1/chat.go`
- `api/chat/chat.go`
- `internal/controller/chat/chat_v1_ai_ops.go`
- `internal/ai/service/ai_ops_service.go`
- `internal/ai/runtime/runtime.go`
- `internal/ai/runtime/file_store.go`
- `internal/ai/runtime/file_store_test.go`

实现内容：

- `AIOpsRes` 新增 `trace_id`
- 新增 `GET /api/ai_ops_trace`
- `FileLedger.EventsByTrace` 现在可从磁盘 `jsonl` 回读
- `Runtime` 新增 `TraceEvents`
- 新增 service/controller 查询链路，返回：
  - `trace_id`
  - `detail`
  - `events`

原因分析：

- 之前持久化层只有写入能力，没有业务读取入口
- 现在 trace 已经能被外部消费，ledger 才真正具备“可回溯”价值

测试结果：

- `TestFileLedgerAndArtifactStore` 扩展验证重载后的 trace 回读
- 新增 `TestGetAIOpsTraceReadsPersistedTrace`

影响评估：

- 可观测性显著提升
- 复盘与排障成本下降
- 为后续前端 trace 展示和 replay 工具打下基础

### 22.3 Phase 3：Chat 链路接入 Multi-Agent 规划

新增文档：

- `res/next-steps-todo.md`

规划结论：

- 这是合理的下一阶段方向
- 但当前阶段不应直接替换 `/chat` 主链路
- 更合理的方式是：
  - 先做 chat triage
  - 再做双路径执行
  - 再做 response shape 统一
  - 最后才考虑是否替换默认 chat path

原因分析：

- 普通聊天与 AI Ops 的风险和复杂度不同
- 当前 `chat_pipeline` 已经是稳定路径
- Multi-Agent 应先在复杂场景中证明价值，再逐步扩大

影响评估：

- 降低未来 Chat 接入 Multi-Agent 的改造风险
- 让后续阶段有更清晰的迁移路线，而不是一次性硬切

---

## 23. Review 收口修复：Approval Gate 与 Chat Memory 一致性

本节记录最近一次 review 指出的两个 P1 级问题及其修复：

- Approval Gate 拒绝被误判为“内部错误”
- Chat 主链路仍然无界后台抽取记忆

### 23.1 Approval Gate 拒绝结果修复

修改文件：

- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/ai_ops_service_test.go`

修复方案：

- `RunAIOpsMultiAgent` 在审批门拒绝时不再返回空结果
- 现在直接返回拒绝原因作为 `result`
- `detail` 保持同样的拒绝说明
- `trace_id` 保持为空，因为任务未进入 runtime

原因分析：

- 审批门拒绝属于业务结果，不应被误判为内部异常
- 当前接口形态下，最小代价的收口方式是直接返回可展示结果

测试结果：

- 新增 `TestRunAIOpsMultiAgentApprovalDenialReturnsReason`

影响评估：

- API 语义更正确
- 用户能够拿到真实拒绝原因
- 错误统计更干净

### 23.2 Chat / ChatStream 统一走有界 MemoryService

修改文件：

- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`
- `internal/ai/service/memory_service.go`

修复方案：

- `/chat` 不再直接 `go mem.ExtractMemories(context.Background(), ...)`
- `/chat_stream` 也不再直接裸起后台抽取
- 两条路径统一改为调用 `MemoryService.PersistOutcome(...)`

原因分析：

- 之前 AI Ops 已经修到 `MemoryService`，但 Chat 主链路仍保留旧路径
- 这种问题的根因不是代码不会写，而是没有把修复沿所有同类入口收口
- 把记忆持久化统一收进 service 层后，后续超时、策略和限流治理都更容易继续推进

影响评估：

- Chat / ChatStream / AI Ops 三条主要路径的 memory 行为更加一致
- 后台抽取任务现在都有 timeout 边界
- 降低未来 memory extraction 变复杂后的隐患

### 23.3 复盘文档

新增：

- `res/review-fixes-approval-and-chat-memory.md`

内容包含：

- 问题描述
- 修复方案
- 技术选型理由
- 根因分析
- 防复发建议

---

## 24. AI Ops Multi-Agent 第二轮收口：Routing / Trace Payload / Knowledge Warm Path

本节记录在 AI Ops Multi-Agent P1 基础上，对最近 3 个 review finding 的收口修复。

### 24.1 Routing 与 Memory 解耦

修改文件：

- `internal/ai/service/memory_service.go`
- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/ai_ops_service_test.go`
- `internal/ai/agent/supervisor/supervisor.go`
- `internal/ai/agent/supervisor/supervisor_context_test.go`

修复方案：

- 给 `MemoryService` 增加 `BuildContext(...)`
- AI Ops 根任务不再把 memory 直接拼进 `Goal`
- 根任务现在保持 raw query 作为 `Goal`
- memory 通过：
  - `Input["memory_context"]`
  - `MemoryRefs`
  传给下游
- `supervisor` 改成：
  - `triage` 只看 raw query
  - specialist 才在执行目标里拼接 memory context

原因分析：

- 之前的实现把“上下文增强”和“意图路由”混在了一起
- memory 一旦增长，`triage` 的路由结果就可能被历史信息污染
- 正确的职责边界应该是：
  - raw query 决定 routing
  - memory 只影响 specialist 执行时的上下文

技术选型依据：

- 没有直接引入新的 context 协议，而是先复用现有 `TaskEnvelope.Input` 与 `MemoryRefs`
- 这是当前代码基线下最小、最稳的收口方式
- 也为后续真正的 Context Engineering 预留了自然升级点

测试结果：

- 新增 `TestRunAIOpsMultiAgentKeepsRawQueryForRouting`
- 新增 `TestSupervisorRoutesOnRawQueryButPassesMemoryToSpecialists`
- 相关 `go test` 通过

影响评估：

- Routing 纯净性显著提升
- 历史记忆不再污染 domain fanout
- Multi-Agent 架构职责边界更清晰
- 为后续 Context Assembler 落地打下基础

### 24.2 Detail / Trace Payload 压缩

修改文件：

- `internal/ai/runtime/runtime.go`
- `internal/ai/runtime/runtime_test.go`

修复方案：

- `task_completed` 事件不再直接把 `result.Summary` 全量塞进 `Message`
- 对长摘要/多行 Markdown 改成短消息：
  - `任务已完成。 详细摘要已折叠。`
- `Payload` 增加：
  - `status`
  - `summary_length`
  - `summary_omitted`
- `DetailMessages(...)` 明确只保留可读的事件类型

原因分析：

- `reporter` 和 `supervisor` 都会生成完整 Markdown 报告
- 之前这些报告被重复塞进 trace 事件和 detail 数组
- 结果是：
  - 前端可读性很差
  - payload 无意义膨胀
  - trace 查询成本上升

技术选型依据：

- 没有改 API shape，只改 event message 生成规则
- 这能在不打断前端现有接入的前提下，直接消掉冗余正文
- 用 `Payload` 保留结构化元信息，兼顾后续可观测性扩展

测试结果：

- 新增 `TestRuntimeDetailMessagesOmitVerboseSummaryBodies`
- 真实 HTTP 响应已验证 `detail` 不再包含完整报告正文

影响评估：

- 前端 trace/detail 的可读性明显提升
- payload 体积下降
- trace 更适合继续做工具级耗时和结构化展示

### 24.3 Knowledge Warm Path 收口

修改文件：

- `internal/ai/tools/query_internal_docs.go`
- `internal/ai/tools/query_internal_docs_test.go`
- `manifest/config/config.yaml`

修复方案：

- `query_internal_docs` 不再每次调用都新建 retriever
- 现在会按 Milvus 地址和 `top_k` 复用已有 retriever
- 同时增加最近初始化失败的短 TTL 缓存
- 新增配置：
  - `multi_agent.knowledge_init_failure_ttl_ms`

原因分析：

- 之前知识库工具每次都会重走 retriever 初始化
- 在 Milvus 热路径较重或依赖不可用时，这会把同样的初始化开销重复打到每个请求上
- 仅靠 request timeout 只能“让它别无限卡住”，不能避免“每次都重新慢一遍”

技术选型依据：

- 当前先在 tool 层做复用，而不是大改 client/indexer 结构
- 这是最小侵入路径
- 成功缓存解决 warm path，失败 TTL 缓存解决重复失败抖动

测试结果：

- 新增 `TestQueryInternalDocsToolReusesRetriever`
- 新增 `TestQueryInternalDocsToolCachesRecentInitFailures`
- 相关 `go test` 通过

影响评估：

- 在 Milvus 可用场景下，可避免重复 retriever bootstrap
- 在 Milvus 不可用场景下，可避免短时间内重复初始化风暴
- 提升了工具层的韧性和后续可观测性价值

### 24.4 第二轮 HTTP 基线结论

新增/更新文档：

- `todo/aiops-baseline-round1-2026-04-05.md`

实测结论：

- `routing` 与 `detail` 问题已经收口
- 当前 AI Ops 主链路约 `5s` 的剩余时延，在本地环境里仍主要来自 `knowledge` 域的实际检索超时
- 也就是说，下一步最值钱的工作已经收敛成：
  1. 为 `query_internal_docs` / Milvus retrieval 补更细的 observability
  2. 区分 `retriever init` 与 `Retrieve` 的真实耗时构成
  3. 再决定先做更深的模块化 RAG 还是先做本地 fail-fast

---

## 25. Chat 链路接入 Multi-Agent（保留 Legacy Fallback）

本节记录把 `/chat` 与 `/chat_stream` 轻量接入现有 Multi-Agent runtime 的实现。

### 25.1 实现目标

- 不重写现有 `chat_pipeline`
- 不一次性把所有普通对话硬切到 Multi-Agent
- 只对明显的运维 / 知识 / 排障类问题启用 Multi-Agent
- 保留旧链路作为 fallback，降低改造风险

### 25.2 修改文件

- `internal/ai/service/chat_multi_agent.go`
- `internal/ai/service/chat_multi_agent_test.go`
- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`
- `internal/controller/chat/chat_v1_chat_test.go`
- `internal/ai/agent/reporter/reporter.go`
- `internal/ai/agent/supervisor/supervisor.go`
- `api/chat/v1/chat.go`
- `manifest/config/config.yaml`

### 25.3 功能实现说明

新增了 `ShouldUseMultiAgentForChat(...)`：

- 对 `告警 / prometheus / 日志 / log / 排查 / 故障 / 知识库 / SOP / runbook / mysql / sql / 数据库 / 指标 / 运维 / oncall / 根因` 等关键词命中时，Chat 会路由到 Multi-Agent
- 其余普通聊天继续走旧 `chat_pipeline`
- 配置项：
  - `multi_agent.chat_route_enabled: true`

新增了 `RunChatMultiAgent(...)`：

- 复用现有 AI Ops runtime、`supervisor`、specialist agents 与 trace / artifact 能力
- 使用 Chat 的原始 `sessionID`
- 复用当前 memory service
- 根任务输入增加：
  - `response_mode=chat`
  - `entrypoint=chat`

Controller 改造：

- `/chat`
  - 命中规则 -> 走 `RunChatMultiAgent`
  - 未命中 -> 保持旧 `BuildChatAgent` 流程
- `/chat_stream`
  - 命中规则 -> 先跑 Multi-Agent，再按 chunk 发送到 SSE 客户端
  - 未命中 -> 保持旧流式链路

### 25.4 Reporter 改造

原因：

- 原 `reporter` 默认输出 `# 告警分析报告`
- 这适合 AI Ops，但不适合普通 Chat

改造后：

- `response_mode=chat` 时，`reporter` 输出更接近聊天风格的结果
- 默认仍保留原 AI Ops 报告格式

技术选型依据：

- 复用现有 agent，不新增第二套 Chat specialist
- 通过 `response_mode` 做输出形态分流
- 这是最小改动、最大复用的落地方式

### 25.5 测试结果

新增测试：

- `TestShouldUseMultiAgentForChat`
- `TestRunChatMultiAgentUsesChatMode`
- `TestChatUsesMultiAgentRoute`
- `TestChatFallsBackToLegacyRoute`

验证通过：

- `go test ./internal/ai/service ./internal/controller/chat ./internal/ai/agent/supervisor`
- `go test ./...`
- `go build ./...`

### 25.6 影响评估

对系统能力的影响：

- Chat 现在已经具备“渐进式接入 Multi-Agent”的能力
- AI Ops 的 runtime、trace、memory、artifact 资产开始被 Chat 复用

对风险的影响：

- 因为保留了 legacy fallback，这次改造风险可控
- 未命中路由规则的普通对话不会受影响

对后续路线的意义：

- 这一步已经把“先实现多智能体改造”落成了真实代码，而不是设计
- 下一步可以基于实际使用反馈，再决定是：
  1. 扩大 Chat 路由范围
  2. 优化上下文工程
  3. 深化模块化 RAG

---

## 26. 前端接入 Chat Multi-Agent 可视化与效果对比

本节记录把前端正式接上 Chat Multi-Agent 元信息，以及 2026-04-05 当日的效果验证。

### 26.1 前端修改点

修改文件：

- `SuperBizAgentFrontend/app.js`
- `SuperBizAgentFrontend/styles.css`
- `internal/controller/chat/chat_v1_chat_stream.go`

实现内容：

- 普通 `/chat` 响应现在会消费：
  - `mode`
  - `trace_id`
  - `detail`
- 前端 assistant 消息新增统一 meta 渲染：
  - `Legacy`
  - `Multi-Agent`
  - `AI Ops`
- Chat Multi-Agent 可以直接查看：
  - Trace
  - 执行步骤
- 历史消息恢复时，也会恢复对应的 mode / trace / detail
- `/chat_stream` 新增 `event: meta`，前端流式模式也能显示来源与 trace

原因分析：

- 之前只有 AI Ops 有“可观测 UI”
- Chat 虽然已经接入 Multi-Agent，但前端看不出来到底走了哪条链路
- 没有 mode / trace / detail，就无法真正把 Chat Multi-Agent 用起来

### 26.2 实测结果

新增文档：

- `todo/chat-multi-agent-frontend-eval-2026-04-05.md`

关键结果：

- `/api/chat` 运维类 query：
  - `mode = multi_agent`
  - `trace_id` 已返回
  - `detail` 已返回
  - 响应约 `4.635557s`
- `/api/chat_stream` 运维类 query：
  - 已收到 `event: meta`
  - 包含 `mode / trace_id / detail`
- `/api/chat` 通用问候 query：
  - `mode = legacy`
  - 响应约 `7.806625s`

### 26.3 技术选型依据

- 没有再单独造一套 Chat 专属 trace UI
- 直接复用 AI Ops 已经跑通的 trace/detail 展示方式
- 通过统一 assistant meta 渲染，把：
  - Chat legacy
  - Chat multi-agent
  - AI Ops
  收敛到同一套前端交互模型里

### 26.4 影响评估

对可用性的影响：

- 现在你已经可以直接在前端区分 Chat 走的是哪条链路
- Chat Multi-Agent 不再是“后台已实现、前台不可见”

对调试与复盘的影响：

- Chat Multi-Agent 现在也具备 trace 可回看能力
- 效果验证不再只能依赖后端日志和 curl

对后续阶段的意义：

- 这一步把“能实现多智能体”推进到了“能在前端直接使用和观察多智能体”
- 后面再做上下文工程或模块化 RAG 时，收益会更容易被直接观察和对比

## 27. 上下文工程 P0/P1 落地：统一 Context Assembler 与记忆写入校验

本节记录 2026-04-05 针对上下文工程的第一轮真正落地，而不是继续停留在设计稿。

### 27.1 新增与改造的模块

新增包：

- `internal/ai/contextengine/types.go`
- `internal/ai/contextengine/resolver.go`
- `internal/ai/contextengine/assembler.go`
- `internal/ai/contextengine/assembler_test.go`

改造文件：

- `internal/ai/service/memory_service.go`
- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/chat_multi_agent.go`
- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`
- `utility/mem/extraction.go`

### 27.2 这轮实现了什么

1. 建立统一上下文对象：
   - `ContextRequest`
   - `ContextProfile`
   - `ContextItem`
   - `ContextPackage`
   - `ContextAssemblyTrace`
2. 建立统一 `ContextAssembler`
   - Chat 路径支持 history + memory 的 staged 注入
   - AI Ops / Chat Multi-Agent 路径支持 memory-only context
3. 把 `ContextTrace` 以 `detail` 形式接入主链路
4. 把长期记忆写入改成：
   - `ExtractMemoryCandidates`
   - `ValidateMemoryCandidate`
   - `ExtractMemoriesWithReport`

### 27.3 原因分析

这次没有继续开更大的上下文工程重构，而是先收敛到最关键的行为缺口：

- history 没有统一预算治理
- memory 没有统一装配
- memory 写入没有校验

如果这三件事不先落地，后续再谈模块化 RAG 或更复杂的 Multi-Agent 上下文都容易失焦。

### 27.4 结果与影响

对 Chat：

- legacy chat 现在也会返回显式 `context detail`
- 上下文注入不再是黑盒

对 AI Ops / Chat Multi-Agent：

- 除了 runtime trace，现在还有 context detail
- memory 选择与裁剪开始可解释

对长期记忆：

- 写入阶段增加了基础过滤
- assistant boilerplate、代码块、异常长度内容会被丢弃

### 27.5 测试结果

执行：

- `go test ./internal/ai/contextengine ./internal/ai/service ./internal/controller/chat ./utility/mem`
- `go test ./...`
- `go build ./...`

结果：

- 全部通过

## 30. 上下文工程完成度矩阵

新增文档：

- `todo/context-engineering-completion-matrix-2026-04-05.md`

核心结论：

- 上下文工程 **没有全部完成**
- 但 **P0/P1 的主体已经完成**
- 当前已经覆盖四类主来源：
  - history
  - memory
  - docs
  - tool outputs

完成度判断建议：

- 已完成：
  - `ContextProfile`
  - `ContextBudget`
  - `ContextAssembler`
  - `ContextTrace`
  - Chat / AI Ops 主链路接入
  - docs / tool outputs 进入统一上下文层
- 已有 MVP：
  - long-term memory 写入校验
- 未完成：
  - context replay / ablation eval
  - trust / poisoning / redaction 治理
  - retrieval rerank / hybrid retrieval
  - context inspection API

更合适的对外口径：

- 不说“已经全部做完”
- 说“主干已落地，治理版未完成”

### 27.6 配套文档

新增详细文档：

- `todo/context-engineering-optimization-implementation-2026-04-05.md`

## 28. 上下文工程第二轮推进：把 Documents 并入统一 ContextAssembler

本节记录第一轮上下文工程落地后的继续推进，重点是把 Chat 文档检索从 graph 黑盒迁移到统一上下文层。

### 28.1 为什么要继续这一步

第一轮之后，`history` 和 `memory` 已经进入了统一上下文装配，但 `docs` 仍然留在 `chat_pipeline` 内部作为黑盒 retriever node。

这带来的问题是：

- docs 不能作为 `ContextItem` 参与统一预算
- docs 不会进入 `ContextTrace`
- 复盘时只能看到 memory/history 的装配，看不到文档证据的进入方式

### 28.2 本次改造内容

新增：

- `internal/ai/contextengine/documents.go`

修改：

- `internal/ai/contextengine/assembler.go`
- `internal/ai/agent/chat_pipeline/types.go`
- `internal/ai/agent/chat_pipeline/lambda_func.go`
- `internal/ai/agent/chat_pipeline/orchestration.go`
- `internal/controller/chat/chat_v1_chat.go`
- `internal/controller/chat/chat_v1_chat_stream.go`

实现结果：

- Chat 文档检索现在在 `ContextAssembler` 内部完成
- documents 被转换成 `ContextItem`
- documents 进入 `ContextPackage.DocumentItems`
- documents 选择/裁剪结果进入 `ContextTrace`
- Chat pipeline 直接消费 `UserMessage.Documents`

### 28.3 技术取舍

这次没有继续做更大的 RAG 改造，只做了这一步：

- 先把文档检索拉入统一上下文控制面

没有做的部分：

- rerank
- hybrid retrieval
- retrieval eval
- tool outputs 真正接入 `ContextItem`

这是一种刻意收敛：先把 docs 纳入统一治理，再继续做更深的模块化 RAG。

### 28.4 当前状态

到目前为止，统一上下文层已覆盖：

- history
- memory
- docs

`tool outputs` 目前只完成了数据结构预留，还没有接到具体生产者。

### 28.5 测试结果

执行：

- `go test ./internal/ai/contextengine ./internal/ai/service ./internal/controller/chat ./internal/ai/agent/chat_pipeline`
- `go test ./...`
- `go build ./...`

结果：

- 全部通过

## 29. 上下文工程第三轮推进：把 AI Ops Tool Outputs 转成 ContextItem

本节记录继续推进统一上下文层，把 AI Ops specialist 的 structured evidence / summary 正式纳入上下文工程。

### 29.1 本次实现内容

新增：

- `internal/ai/contextengine/tool_items.go`
- `internal/ai/contextengine/tool_items_test.go`
- `internal/ai/agent/reporter/reporter_test.go`

修改：

- `internal/ai/contextengine/types.go`
- `internal/ai/contextengine/resolver.go`
- `internal/ai/contextengine/assembler.go`
- `internal/ai/agent/reporter/reporter.go`
- `manifest/config/config.yaml`

实现结果：

- specialist evidence / summary 会被转换成 `ContextItem`
- reporter 会通过 `ContextAssembler` 统一选择 `tool items`
- reporter 级上下文细节会进入 trace/detail
- chat 模式下的证据展示优先来自统一的 `ToolItemSnippets`

### 29.2 为什么这样做

前两轮后，统一上下文层已经覆盖：

- history
- memory
- docs

但 AI Ops 的工具证据仍然停留在 `TaskResult.Evidence` 里，没有进入统一上下文平面。

这一步收口后，统一上下文层第一次覆盖到：

- tool outputs

### 29.3 当前完成度

到目前为止，统一上下文层已覆盖四类主来源：

- history
- memory
- docs
- tool outputs

这是一个实质性的阶段点，说明项目已经不再只是“部分上下文模块化”，而是有了最小完整闭环。

### 29.4 测试结果

执行：

- `go test ./internal/ai/contextengine ./internal/ai/agent/reporter ./internal/ai/service ./internal/controller/chat`
- `go test ./...`
- `go build ./...`

结果：

- 全部通过
