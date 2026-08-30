## Context

参见 `proposal.md` 的动机。当前仓库存在三套可复用基础：`internal/ai/rag/eval/` 已覆盖检索数据校验、分层指标和候选 Gate；`internal/ai/agent/gos_engine/eval/` 与 `cmd/gos_eval/` 已覆盖数据集角色、真实/录制/确定性 profile、指纹、Trace、Evidence 和基线比较；`internal/ai/agent/eval/` 提供诊断 A/B Judge 基础。Plan 已产生步骤、工具调用和 Trace 结果，Tool 具有契约测试，Citation/Evidence 已有规范化结构，但这些能力尚未共享控制面。

设计必须遵守以下约束：不进入 Controller，不成为线上请求依赖；不恢复 `chat_multi_agent`；Memory 不替代原始 query 参与路由；真实外部依赖不可用时使用 degraded/skipped/failed 语义；所有预算与 Gate 必须配置化；RAG 领域层不得直接依赖 Milvus 实现。

## Goals / Non-Goals

**Goals:**

- 用一个 CLI 和一个 run manifest 编排六类 suite，生成统一且可比较的报告。
- 最大程度复用 RAG、GoS 与 Plan 现有执行/评分逻辑，统一层只负责协议、调度、汇总和 Gate。
- 同时支持快速确定性 PR Gate、可复现 recorded 回归和受控 live 验证。
- 将失败定位到 route、retrieve、plan、act、update、report、evidence 等规范化阶段。
- 保持数据集、基线、评测器和证据语料的可追溯性，避免开发集自证和失效基线。

**Non-Goals:**

- 不实现线上自动反馈学习、自动 Prompt 优化或自动修改 Gate。
- 不构建新的 Agent 编排框架，也不统一 Plan 与 GoS 的内部状态机。
- 第一阶段不提供 Web 管理页面，不新增对外 API。
- 不宣称 deterministic、recorded 或小规模 holdout 等同于生产效果。
- 不在第一阶段替换历史 RAG/GoS 报告格式或删除旧命令。

## Decisions

### 1. 采用“控制面 + 领域 Adapter”，不做通用大一统 Evaluator

新增 `internal/ai/evalharness/`，负责 manifest 校验、suite 注册、执行预算、报告聚合、指纹和 Gate；每个 suite 通过薄 adapter 实现统一 Runner 契约：

```text
cmd/eval_harness
    -> Harness Orchestrator
        -> Route Adapter
        -> RAG Adapter  -> existing rag/eval
        -> Plan Adapter -> existing Plan run artifacts/runner
        -> GoS Adapter  -> existing gos_engine/eval
        -> Tool Adapter -> tool registry + fixtures
        -> Evidence Adapter -> normalized answer/citation/evidence trace
    -> Gate Engine -> JSON + Markdown
```

Runner 只统一生命周期，不统一领域语义：`Validate -> RunCase -> Aggregate -> DomainGate`。领域输入、结果与指标使用带 `schema_version` 的 JSON payload，公共层只读取明确约定的公共字段。

选择原因：RAG 的排序指标和 GoS 的图推理指标无法用同一“准确率”正确表达。薄 adapter 能复用已经测试过的逻辑，减少评分口径漂移。

替代方案：重写一个泛型 evaluator。否决，因为会复制现有代码、丢失领域信息，并形成难维护的 `map[string]any` 巨型协议。

### 2. 使用 manifest 管理 run，case envelope 管理稳定身份

建议目录：

```text
evals/harness/
  manifests/
    pr-regression.yaml
    recorded-development.yaml
    live-holdout.yaml
  datasets/<suite>/
  baselines/<profile>/<suite>/
  reports/<run-id>/
```

Manifest 声明：

- `schema_version`、`run_name`、`dataset_role`、`profile`
- suite 列表及各自 dataset、baseline、adapter config
- 并发、超时、case 上限、调用/Token/费用预算
- 公共 Gate、领域 Gate、跨链路 Gate
- 报告目录与是否继续执行非阻塞 suite

Case envelope 仅放稳定公共字段：`id`、`suite`、`input`、`expectation`、`tags`、`payload_schema_version`、`payload`。原始 query 必须原样保存；敏感 fixture 只保存引用和哈希，不直接复制到报告。

选择原因：manifest 让一次运行可以冻结配置，并允许同一 case ID 在 Plan/GoS 之间关联；envelope 避免强迫所有 suite 使用同一字段集合。

替代方案：让每个命令继续读取自己的 flags。保留作为兼容入口，但不适合作为统一、可重放的评测声明。

### 3. 三类数据角色与三类运行 profile 正交

数据角色回答“这批标签怎么使用”：

- `development`：调参与失败分析，可重复查看。
- `regression`：稳定、小规模、确定性，服务 PR Gate。
- `holdout`：候选冻结后验证，不参与本轮调参。

Profile 回答“依赖从哪里来”：

- `deterministic`：fake/fixture，不调用真实外部服务。
- `recorded`：使用经过来源校验和指纹固定的录制证据。
- `live`：调用获准环境中的真实模型/检索/工具依赖。

合法组合由 adapter 声明。例如 PR 默认 `regression + deterministic`；GoS 可以运行 `development + recorded`；最终验证使用 `holdout + recorded/live`。报告同时展示两个维度，避免把 recorded holdout 误说成线上验证。

### 4. 报告采用公共 envelope + 领域 payload

统一 JSON 报告建议包含：

- `schema_version`、`run_id`、开始/结束时间、最终状态、真实性声明
- `fingerprints`：dataset、config、code scope、model、prompt、evaluator、evidence corpus
- `budget`：限制、已用量、是否超限
- `suites[]`：suite 状态、公共指标、领域指标、Gate、case 结果
- `cross_suite_gates[]`：跨链路不变量
- `failures[]`：case、阶段、状态、原因、trace/evidence 引用

公共状态固定为 `succeeded | degraded | failed | skipped | budget_exceeded`。公共指标不可获得时使用显式 availability，而不是零值。Markdown 由 JSON 渲染，JSON 是唯一事实来源。

替代方案：直接拼接现有报告。否决，因为字段同名不同义、缺少版本和 unavailable 语义，无法安全比较。

### 5. Gate 分三层，禁止跨领域平均分

Gate Engine 按顺序判断：

1. **公共硬门槛**：schema/指纹/Trace 完整性、失败率、降级率、预算、P95。
2. **领域 Gate**：由 adapter 提供，例如 Route Macro-F1、RAG MRR/Recall、Plan 完成率/重规划成功率、GoS 根因准确率/图有效性、Tool 契约符合率、Evidence 引用完整率。
3. **跨链路不变量**：例如故障路由进入 Plan/GoS 后必须有 Trace；成功诊断的关键结论必须有 Evidence；工具权限拒绝不得变成成功执行。

每个 Gate 记录 metric、operator、threshold、baseline、actual、severity 和 case refs。Blocking 失败决定退出码；warning 不阻断但进入摘要。不存在“六类 suite 平均准确率”。

### 6. 路由评测直接调用路由器并冻结原始 query

Route adapter 不通过完整 Chat 链路，也不读取 Memory 重写后的 query，而是使用统一入口所依赖的实际 Router 接口。case 可以携带历史上下文用于验证后续执行，但 `routing_input_hash` 必须对应原始 query。输出混淆矩阵、每类 Precision/Recall/F1、低置信度率、缓存命中来源、模型 fallback 和 P95。

选择原因：这能隔离“路由错”与“执行错”，同时验证当前设计约束：Memory 是执行上下文，不是路由输入替代品。

### 7. Tool 与 Evidence 先做契约/可追溯评测，不引入 LLM Judge 作为硬 Gate

Tool adapter 通过 Registry 获取实际 schema 和 wrapper，使用 fake transport 注入成功、超时、取消、权限拒绝、畸形返回与外部错误。硬指标包括 schema 合法率、权限边界符合率、超时/取消符合率、降级符合率和重试预算。

Evidence adapter 对规范化结果做确定性检查：claim 是否引用 Citation、Citation 是否有 Source、Evidence 是否回指 Citation、Trace 是否可定位、关键词/来源规则是否满足。语义相关性可选使用 Judge，但第一阶段只作为 non-blocking 指标，并记录 Judge 版本与不确定性。

选择原因：结构和契约可以稳定回归；直接把 LLM Judge 作为 PR 硬 Gate 会引入漂移、成本和不可复现性。

### 8. 执行预算由 Harness 统一兜底，adapter 继续执行领域超时

Harness 使用父 `context.Context` 控制 run 总超时和取消，suite 受独立 timeout、并发和 case 数限制；adapter 内部继续保留现有单调用 timeout。计数器聚合 LLM、Tool、RAG、Token 和估算费用，触发硬预算后取消未开始或非必要任务，已完成结果仍写报告。

默认 CI 使用并发 1 或小并发，保证确定性；recorded/live 可配置并发，但必须尊重依赖限流。所有新增值进入 `manifest/config/config.yaml`，禁止硬编码。

### 9. 分阶段接入 CI，普通 PR 只跑确定性 Gate

新增 Make target，例如 `make eval-harness-gate`，执行 `regression + deterministic`，只包含无网络的 Route、RAG 小语料、Plan、GoS、Tool、Evidence smoke。普通 PR 不调用真实模型、Milvus、Prometheus 或日志系统。

recorded development 可由手动工作流运行；holdout recorded/live 需要候选冻结和显式授权；live 结果只能标记为受控环境验证。CI 上传 JSON/Markdown artifact，但报告中的 query、工具参数、证据片段需经过脱敏。

## Risks / Trade-offs

- **[统一协议可能变成巨型抽象]** → 公共 envelope 只保留稳定生命周期字段，领域指标和 payload 由版本化 adapter 管理。
- **[现有 RAG/GoS 指标在适配时发生漂移]** → adapter 必须对同一 fixture 证明新旧报告关键指标一致，旧 evaluator 继续作为事实来源。
- **[Holdout 被反复查看造成污染]** → manifest 记录角色与候选指纹；本地开发命令默认拒绝 holdout，并在文档中要求重新生成下一轮 holdout。
- **[LLM Judge 不稳定导致 PR 抖动]** → 第一阶段 Judge 仅作为非阻塞指标；硬 Gate 只使用确定性契约或冻结录制结果。
- **[Recorded 数据被误解为真实线上数据]** → 报告顶层强制真实性声明，profile 不允许由 adapter 自行覆盖。
- **[评测时间过长]** → PR 只跑小规模 regression；suite 并行受配置控制；超预算立即取消非必要任务。
- **[报告泄露 query、工具参数或证据]** → 默认只保存哈希、截断摘要和允许字段；复用项目输出过滤规则并增加敏感字段测试。
- **[一次性迁移风险高]** → 旧命令和数据集保持可用，先做旁路统一报告，再迁 CI，不删除历史入口。

## Migration Plan

1. 建立公共 schema、manifest loader、registry、预算与报告单元测试，不接入 CI。
2. 先适配 RAG 和 GoS，以同一 fixture 校验新旧指标等价；随后适配 Plan。
3. 增加 Route、Tool、Evidence deterministic suite 与最小 regression 数据集。
4. 增加公共/领域/跨链路 Gate，生成 JSON 与 Markdown 报告。
5. 旁路运行现有 `make eval-gate` 与新 Harness，比较稳定后将 PR CI 切换到统一入口；旧入口继续保留至少一个迁移周期。
6. 在获准环境中手动验证 recorded/live profile，并明确记录“受控验证”而非生产验证。

回滚时仅移除新的 CI step/Make target；旧 RAG、GoS 与 Plan 评测入口仍可独立运行，线上链路不受影响。

## Open Questions

- 第一阶段 Markdown 摘要是否只作为 CI artifact，还是后续在前端增加内部评测页；该选择不影响当前协议与 Runner 设计。
- Token 费用是否按模型配置中的静态单价估算，还是只记录 token 数；第一阶段可将费用标记 unavailable，不影响预算中的调用数与时间限制。
