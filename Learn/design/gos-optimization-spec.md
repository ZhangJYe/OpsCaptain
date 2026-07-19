# GoS Belief Engine 优化 Spec

状态：In Progress
版本：v1.0
日期：2026-07-15
适用范围：`internal/ai/belief/`、`internal/ai/agent/gos_engine/`、`internal/ai/agent/experts/`、`cmd/gos_eval/`
当前主链路：`aiops.engine=plan_execute_replan`，GoS 仅作为可切换实验引擎

Agent 入口收敛、Chat/AIOps 业务路由、旧接口兼容和 Plan 退出生产的迁移设计，统一由 [统一 Agent 入口与意图路由 Spec](./unified-agent-routing-spec.md) 管理。本 Spec 只负责 GoS 的推理正确性、证据契约和真实发布 Gate；在 Phase 7 真实 Gate 通过前，不得因入口合并而提前删除 Plan 或切换生产默认引擎。

## 0. 执行状态

最后更新：2026-07-18

| 阶段 | 状态 | 当前证据 | 剩余 Gate |
|---|---|---|---|
| Phase 0 | Independent Recorded Gate Passed；Production Telemetry Unverified | development v2 与全新 blind holdout v4 均完成一次真实模型 baseline/compare；v4 的 11 项 Gate 全部通过，artifact 有代码/配置/数据/证据指纹和运行质量准入 | `recorded` 仍是 `development_only`，不能替代服务器真实生产遥测验证 |
| Phase 1 | Implemented，Independent Recorded Gate Passed | Evidence 显式 relation/target/strength/provenance；真实 Expert 结构化语义提议；支持/反驳升降分、去重、neutral、图 invariant、COW 回滚和 race 测试通过 | 生产灰度仍等待真实遥测验证 |
| Phase 2 | Implemented，Locally/Integration Verified，Feature Flag Off | StateConverter 五类决策；原子 Refine；祖先链校验与 Backtrack；Granularity；预算/边界降级；真实 BaseExpert 集成测试覆盖 L1→L2 Refine→Report | v4 compare 使用兼容路径且未开启 State Conversion；真实遥测与开关开启后的发布 Gate 仍未验证 |
| Phase 3 | Implemented，Locally Verified，Feature Flag Off | 结构化 Ingest/Plan/Graph Proposal 已实现；严格 schema、确定性 fallback、proposal 原子提交/回滚、StateConverter 复用已验证子节点；scoped tests/race/vet 通过 | 真实模型 development compare 与生产遥测仍留到后续综合 Gate |
| Phase 4 | Implemented，Locally Verified，Feature Flag Off | frontier 相关压缩图、证据目标/授权工具、专家与 session 预算、1..N 调度、稳定并发、工具级失败和两轮无进展停止均有测试；scoped tests/race/vet 通过 | 真实模型 development compare 与生产遥测仍留到后续综合 Gate |
| Phase 5 | Implemented，Controlled Development Verified，Feature Flag Off | evidence-first report 固定输出结论、图置信度、支持/反驳、冲突、缺口和下一步；结论保留专家诊断依据，证据片段受 512 字符配置上限约束，且只展示当前结论路径，避免兄弟候选证据污染报告 | 独立 holdout Gate 仍未通过，生产遥测和灰度未验证 |
| Phase 6 | Implemented，Locally Verified，Feature Flag Off | checkpoint+delta、图资源上限、phase/transition/graph metrics、取消与 panic recovery、Redis Ledger 边界、prompt/model/config version 均已实现；scoped/full tests、race/vet/build 与 deterministic Gate 通过 | 生产 P95、跨实例 Redis 与 artifact 共享存储仍需 Phase 7 真实环境验证 |
| Phase 7 | Isolated Controlled Development Complete；Frozen v1 Gate Failed | 隔离 fixture、日志适配器、Prometheus、真实模型和生产 Milvus 只读 RAG 已联通；未修改、重启或部署生产服务 | 修复模型超时稳定性与重复取证精度后，只能使用新的独立 holdout 重新 Gate；shadow run、灰度和生产写操作仍需另行授权 |

仓库目标配置保持 `plan_execute_replan`，GoS 总开关、State Conversion 与 Structured Cognition 开关均保持 `false`。2026-07-16 服务器只读审计确认在线引擎仍为 `plan_execute_replan`，但旧版在线配置中的 `aiops.gos.enabled` 为 `true`，且尚无 State Conversion、Structured Cognition 和真实遥测 profile 配置；这是待单独授权修复的配置漂移，本轮未修改。Phase 2～6 的“Locally Verified”只表示代码级测试、race、vet 和既有 regression gate 通过，不代表真实诊断效果已经验证。

2026-07-18 按“最小闭环”停止继续扩展架构：仅修复真实 development case 直接暴露的问题，包括隔离日志默认窗口、日志与当前指标互补取证、同源 ID 不再包含专家名、中性证据禁止生成 refinement、source-backed actionability、证据优势状态收敛，以及报告保留诊断依据/受限证据片段。完整 `development_v1` 一次真实运行结果为 3 条根因均匹配、evidence coverage 100%、graph validity/traceability 100%，其中 2 条 succeeded、1 条因连续三次 30 秒 Planner 超时和后续结构化评估失败而 degraded，contract compliance 66.67%，evidence precision 41.67%。随后修复报告兄弟候选污染，使用最终代码只复核 case 002：root-cause accuracy、coverage、graph validity、traceability、contract compliance 均为 100%，artifact 为 `evals/real_controlled/artifacts/development_v1/case_002_minimal_closure.json`（SHA-256 `223d83e453e91e978e7de3a59d103bf90f71384fafbf6c53e5fc667005632cf9`）。该结果仅是可检查的 development 证据，不得替代冻结 v1 holdout：v1 compare 仍以 root-cause accuracy 20%、contract compliance 0%、degradation 100% 判定 Gate Failed，禁止重跑或调参覆盖。

同日继续收敛两个已复现问题：Structured Planner 首次生成失败后，在当前 Run 内锁定为规则 fallback，避免同一请求连续触发三个 30 秒超时；`query_logs` 与 `query_prometheus_instant` 的 evidence source ID 只忽略查询文本、评估时间和响应 message 等易变外壳，仍保留真实日志时间、指标标签与数值作为观测身份。该锁定不跨 Run，也不吞掉 schema/contract 错误；未增加配置项或新架构。

本轮 deterministic Gate 显式开启仅限 `eval` profile 的 Structured Cognition 与 State Conversion，使用确定性 generator 故障触发规则 fallback，并由 source-backed expert refinement 驱动 `Refine → Report`；它不使用真实 LLM，因此不计入真实 accuracy。Gate 先发现 evaluator 在多次取证时用后一次文档结果覆盖前一次日志证据，并发现旧 baseline 的内部 config 指纹已经过期；修复 evaluator 的多源合并并恢复仅限 eval profile 的紧凑预算后，未降低任何门槛，重新生成 baseline。最终结果为 5/5，root-cause accuracy、contract compliance、evidence precision、evidence coverage、graph validity、traceability 均为 100%，premature-stop 与 degradation 均为 0%，平均 LLM/tool/RAG calls 为 6/2/0，11 项 Gate 全部 PASS。当前 baseline SHA-256 为 `5fedc48f0c738f4a953766ad8004abc9730c834f4d5c759120cdb7f2acf5dd4d`；artifact 同时校验 dataset/config/code 指纹、逐 case ID、ground truth、数量、`source-backed-signal-v2` 与 `complete-trace-profile-activity-v1` 运行质量契约。冻结 v1 holdout 不重跑；case 001 的真实 development 复核因隔离只读隧道当前未连接而保留为待办。生产 GoS 总开关、State Conversion 与 Structured Cognition 仍保持关闭，未执行 shadow、灰度、部署或生产写操作。

当前本地 LLM、Embedding、MCP、Prometheus 与 Milvus 配置已经可用，29 份知识文档已通过真实豆包 Embedding 写入 Milvus，验收查询返回 5 份文档。真实 baseline 试跑到第 4 个 holdout 时主动中止：当前 Prometheus 主要业务数据来自 `synthetic-checkout`，与 16 个 holdout 故障没有逐 case 隔离关系，继续运行会把“真实模型 + 模拟遥测”误标为全真实结果。`aiops.gos.evaluation.telemetry_profile` 因此在本地设为 `synthetic`、生产配置设为 `unverified`；只有完成真实遥测来源核验并显式设为 `real` 后，`baseline/compare` 才可生成 artifact。AIOps2025 构建器现在同时支持 `groundtruth_guided` 与 `blind`：前者产物标记为 `recorded_label_assisted/development_only`，后者完全不读取 `groundtruth.jsonl`，只解析 input 中两个 UTC 时间戳并扫描窗内全部实体，产物标记为 `recorded_blind/development_only/input_time_window_only`。blind 证据现已通过 `recorded` profile 接为逐 case 隔离的只读 Tool/RAG replay，并完成真实模型对照；该 profile 仍显式标记为 `development_only`，不能替代真实生产遥测 Gate。

## 1. 背景与依据

OpsCaptain 的 GoS 链路参考 Graph of States（GoS）论文及其开源实现，但当前仍是面向 AIOps 的初步 Go 实现，不应宣称与论文完整等价。

参考资料：

- 论文：[Graph of States: Solving Abductive Tasks with Large Language Models](https://arxiv.org/abs/2603.21250)
- 参考仓库：[gaorch85/Graph-of-States](https://github.com/gaorch85/Graph-of-States)
- 本次对照的参考提交：`1d258aa3d6dc7d8f9e5d16c50e40e1188470019f`
- 本项目已有设计：`Learn/design/gos-belief-engine-design.md`
- 本项目历史进度：`Learn/design/gos-belief-engine-progress.md`

论文的核心不是“多个专家并发”，而是双层闭环：

1. 符号到认知：`FindFocus → Plan → Expert ReAct`。
2. 认知到符号：`UpdateGraph → Backtrack → DrillDown/Report`。
3. 因果图显式保存 Signal、Evidence、Hypothesis 及 support/refute/refines/causal 关系。
4. 状态机限制合法转移，避免证据编造、上下文漂移、回溯失败和过早停止。

本 Spec 的目的，是按依赖顺序补齐上述闭环，同时保持 OpsCaptain 已有的可配置、可降级、可观测和证据约束。

## 2. 目标

### 2.1 必须实现

- 建立可复现的 GoS 与 Plan-Execute-Replan 对照基线。
- 修正图更新、置信度、support/refute 和证据 provenance 的语义。
- 实现真正可执行的 Drill-down、Backtracking 和 Granularity Check。
- 将 Ingest、Plan、Graph Proposal、Report 拆成可验证的结构化阶段。
- 为每次状态转换记录原因、输入证据、配置版本和模型/工具调用统计。
- 在真实依赖失败时返回 `ResultStatusDegraded`，保留已有有效证据。
- 通过真实 holdout compare 后，才允许进入灰度；默认主引擎不自动切换。

### 2.2 非目标

- 不逐行移植参考仓库的 Python 实现。
- 不为了论文形式恢复已废弃的 `chat_multi_agent` 路由。
- 不在没有评测证据前启用 GoS 为生产默认引擎。
- 不用增加专家数量代替图语义和状态转换正确性。
- 不把 5 条 deterministic smoke 的 100% 当作真实诊断准确率。
- 不在本轮引入自动执行故障修复；GoS 只负责分析和证据化诊断。

### 2.3 总执行顺序

| 顺序 | 阶段 | 解决的问题 | 未通过时禁止 |
|---:|---|---|---|
| 0 | 冻结基线与评测契约 | 先保证指标可信、失败可定位 | 禁止调整算法后再补基线 |
| 1 | 修正图和证据语义 | support/refute、置信度、来源、去重 | 禁止实现回溯和智能调度 |
| 2 | 补齐 State Conversion | Refine、Backtrack、Granularity、Report 决策 | 禁止扩大专家扇出 |
| 3 | 结构化认知层 | LLM Ingest/Plan/Graph Proposal 的 Schema 与 fallback | 禁止依赖自由文本修改图 |
| 4 | 专家 ReAct 与预算治理 | 定向取证、并发、循环检测、成本上限 | 禁止用更多调用掩盖推理缺陷 |
| 5 | 证据化报告与 Trace | 结论、证据、冲突、不确定性可追踪 | 禁止对用户输出“确定根因” |
| 6 | 性能、持久化与可观测性 | 图规模、快照、竞态、取消、多实例边界 | 禁止进入真实流量灰度 |
| 7 | 真实对照与灰度 | real compare、shadow、回退 | 禁止切换生产默认引擎 |

依赖关系严格按 `0 → 1 → 2 → 3 → 4 → 5 → 6 → 7` 推进。单个阶段内部可以并行补测试、实现和观测，但不能跨过该阶段 Gate。

## 3. 当前实现基线

### 3.1 已具备

- Per-run `BeliefGraph` 和 `BeliefFSM`。
- Signal、Evidence、Hypothesis 节点以及 support/refute/refines/causal 边类型。
- Copy-on-write 图更新、节点撤回/替代字段和图快照。
- Frontier 提取、专家调用、工具/RAG/LLM 统计。
- Linux、Network、Database 专家及工具注册。
- 超时、并发限制、失败降级和部分证据保留。
- deterministic smoke、baseline、real compare 等评测入口。
- AIOps Runtime 可注册 GoS 引擎，但生产配置仍选择 Plan-Execute-Replan。

### 3.2 与论文闭环的差距

| 论文阶段 | 当前状态 | 主要缺口 | 优先级 |
|---|---|---|---|
| Initialization | 初步实现 | Ingestor 使用固定三类假设；输入症状被当作同一条 Evidence 支持全部假设 | P0 |
| FindFocus | 部分实现 | 只取当前层最高分节点，没有验证其祖先链、证据缺口和探索价值 | P1 |
| Plan | 初步实现 | 关键词选择单一专家，缺少基于 frontier、missing evidence、历史调用的结构化计划 | P2 |
| Expert ReAct | 已有基础 | 专家具备 Tool/RAG/LLM，但缺少按计划声明预期证据、停止条件和调用预算 | P2 |
| UpdateGraph | 初步实现 | 用专家总体 confidence 判断 support/refute；分数只升不降；没有证据去重和冲突聚合 | P0 |
| Backtracking | 数据结构存在 | 主循环没有检查 frontier 祖先是否仍为各层最优，也没有执行可审计回溯 | P0 |
| Drill-down | 未闭环 | FSM 进入下一层后没有生成细粒度子假设，下一轮会出现 `no frontier` | P0 |
| Granularity Check | 简化实现 | 仅依赖 score/min_support，不能判断假设是否已细化到可报告根因 | P1 |
| Report | 部分实现 | 直接取 frontier/最佳专家分析，缺少面向证据、反证、不确定性和下一步的统一报告合成 | P1 |
| Evaluation | 初步实现 | smoke 仅 5 条确定性样例；真实 compare、故障注入和回溯专项数据不足 | P0 |

## 4. 核心设计约束

### D1：LLM 只能提出结构化 Proposal，不能直接修改图

Ingest、Graph Update、Refine、Report 可以由 LLM 生成候选，但必须经过：

`Schema 校验 → ID/边合法性校验 → 证据来源校验 → 置信度范围校验 → Copy-on-write commit`

任何校验失败不得产生部分图写入。

### D2：support/refute 是证据关系，不由总体 confidence 推断

每条专家 Evidence 必须显式携带：

- `relation`: `support | refute | neutral`
- `target_hypothesis_id`
- `strength`: `[0,1]`
- `source_type/source_id/tool_name/artifact_ref`
- `observation_time`

专家“对自己分析的置信度低”不等于该证据反驳当前假设。

### D3：置信度更新必须可升可降、可解释

首版不追求复杂贝叶斯模型，但必须满足：

- 输入分数和输出分数限制在 `[0,1]`。
- support 可提高分数，refute 可降低分数。
- 同一来源、同一观测不得重复累计。
- 聚合结果记录公式版本和贡献明细。
- 阈值全部进入 `manifest/config/config.yaml`。

具体聚合公式先通过 ADR 决定；在公式未固定前，不允许调参制造评测提升。

### D4：回溯不做物理删除

为保留可追溯性，Backtracking 使用 `retracted/superseded` 状态，不删除历史节点和边。回溯必须记录：

- 失效的祖先层级和节点。
- 触发回溯的新证据。
- 被撤回的后续子图。
- 新的 FSM level/frontier。

### D5：符号决策优先确定性，LLM 只处理语义判断

- Gap、support/refute 数、预算、最大步数、合法状态转换：确定性代码。
- 假设生成、证据语义关系、Granularity、最终文字报告：可使用 LLM，但必须结构化和可降级。

### D6：评测先行，生产开关最后

每一阶段先补测试和 eval case，再实现功能。阶段 Gate 未通过，不进入下一阶段；真实 compare 未通过，不改变生产默认引擎。

## 5. 分阶段实施顺序

### Phase 0：冻结基线与评测契约

目的：先建立不会自证、不会因指标口径变化而漂移的测量体系。

工作项：

- [x] Artifact 固定 git commit、配置 hash、prompt 版本、模型/工具配置和依赖 profile。
- [x] 将 deterministic smoke 明确标记为 regression，不计入真实 accuracy。
- [x] 建立 24 条 development 和 16 条独立 holdout；覆盖 CPU、内存、网络、数据库、缓存、消息队列、配置变更和依赖故障。
- [x] 新增专项 case：支持证据、反驳证据、证据冲突、错误首选假设、需要 Drill-down、需要 Backtracking、工具超时、RAG 空结果、LLM 无效 JSON。
- [x] 指标增加 root-cause accuracy、contract compliance、evidence precision/coverage、backtrack success、premature-stop rate、graph validity、degradation rate、P50/P95 latency、LLM/tool/RAG calls。
- [x] Eval artifact 记录 git commit、配置摘要、模型标识、数据集 hash 和运行时间。
- [x] real 模式启动前执行一次真实 Embedding + Milvus 检索，空集合、schema 错误、超时或空文档均拒绝生成 artifact。
- [x] artifact 记录 telemetry provenance；`synthetic/unverified` 遥测不能生成或参与 real baseline/compare。
- [x] 录制遥测增加 `blind` 提取，构建阶段不读取 ground truth，并按实体与来源族进行稳定、多样化排序。
- [x] 冻结 AIOps2025 18+18 不相交 development/holdout manifest；事后 evaluator 单独读取标签并生成带 manifest/builder/evaluator/summary hash 的 artifact。
- [x] 为 `recorded` profile 提供逐 case 构造的只读 Tool/RAG replay；缺失、越权、软链、跨 case 和标签字段均拒绝且不得回退到生产数据源。
- [x] baseline/compare 记录并复核 replay 证据语料 SHA-256，证据变化时在模型调用前拒绝比较。
- [x] 修复 Plan-Execute-Replan 将 executor 中间输出误判为终稿、降级时丢弃已有报告以及报告失败阶段误分类的问题。
- [x] Artifact 记录当前代码内容指纹；baseline 与 GoS 分别固定相关代码 scope，dirty worktree 内容变化会在模型调用前拒绝比较。

验收 Gate：

- 相同 artifact 可以复现相同的确定性指标。
- holdout 不进入 prompt、索引构建或规则映射。
- 每个失败 case 能定位到 ingest/plan/act/update/state/report 中的一个阶段。
- 现有 `go test ./internal/ai/belief ./internal/ai/agent/gos_engine/...` 通过。

本地验收证据：`dataset_test.go` 校验 schema、角色、规模、领域/场景覆盖、development/holdout 不相交及 holdout 不进入运行时代码、prompt/配置；失败/降级 case 必须声明预期阶段。`runner_test.go` 校验全部指标、状态/失败阶段契约、FSM transition、调用统计和逐 case Engine 隔离；`main_test.go` 校验 artifact 指纹、逐 case 对齐、profile/role 隔离、模式默认数据集、真实 RAG probe、telemetry provenance 和 recorded corpus 指纹门禁；`recorded_replay_test.go` 校验 schema、只读边界、跨 case/路径穿越/软链/标签字段拒绝、缺失降级和单次完整快照。`test_build_telemetry_evidence.py` 当前 18 项测试覆盖显式 case/manifest 选择、split 不相交、未知 ID 拒绝、blind 无 groundtruth 启动、UTC 窗口契约、实体分组、来源族覆盖、TiDB namespace、分钟桶与短故障观察窗、provenance/反泄漏、artifact hash 和 evaluator-only GoS dataset。Plan-Execute-Replan baseline 的 LLM/tool/RAG calls 已改为 Eino callback 计数，不再用 detail 条数近似。确定性 artifact 可复现；真实依赖已完成本地连通性验证，但真实遥测 Gate 仍待获准的真实环境执行。

2026-07-15 第二轮严格审计后验证：`go test ./...`、Phase 1/2 scoped tests、`go test -race`、`go vet ./...`、`go build ./...` 全部通过；CodeGraph 已刷新为 514 files / 8,440 nodes / 22,235 edges。`deploy/config.prod.yaml` 仅完成静态配置修改，未在云服务器真实部署环境验证。

2026-07-15 第三轮依赖复核：知识索引命令退出码为 0，真实检索返回 5 份文档；scoped tests、scoped race、`go vet ./...`、`go build ./...`、`go test ./... -count=1` 和 `frontend/npm run build` 全部通过。确定性 regression 仍为 5/5，11 项 Gate 全部 PASS。全仓测试曾暴露 `TestCorrelateAlertsTool_WithRepo` 错误假设 Prometheus 一定未启动，现已改为同时验证成功结果与显式 degraded 两条合法路径。设置 `telemetry_profile=synthetic` 后，`preflight` 与 `baseline` 都在任何真实评测调用前返回非零状态，且未创建 artifact。部分 baseline 试跑只作为 LLM/Tool/RAG 连通性证据，不计入 accuracy，也不保留为 baseline artifact。CodeGraph 已刷新为 523 files / 8,622 nodes / 22,727 edges。

真实遥测 Gate 的可用资产审计已更新：本机 `/Users/zhangjinye/workspace/dataset/agenticopseval/AIOps2025` 存在 AIOps2025 的 `input.json` 与 `groundtruth.jsonl`，并从官方 Git LFS 下载 `2025-06-09.tar.gz`、`2025-06-17.tar.gz`、`2025-06-19.tar.gz`；三者 MD5 均与仓库 `checksums.md5` 一致，解压后共 456 个 Parquet 文件、约 1.8 GiB。标签辅助构建在首批 18 case 上得到 140 metric、73 log、13 trace，18/18 非空；过程中修复 TiDB namespace 未命中和秒级故障分钟桶错分。随后冻结 `evals/aiops2025/recorded_split.json`（manifest SHA-256 `13c164ae1a46675e5983c5d19375b3038d1d4ace280b495c4d46ceeda2094b55`），development 与 holdout 各 18 case、各覆盖 18 种 fault type、ID 完全不相交且 manifest 不含标签。

2026-07-15 第四轮录制遥测复核：blind development 产生 288 metric、72 log、36 trace，holdout 产生 288 metric、92 log、96 trace，两个角色均 18/18 非空。冻结参数为 `extraction_profile=blind`、`max_metric_signals=16`；holdout 运行后未再调参。独立 evaluator 在提取完成后才读取 ground truth：development 的 exact entity recall 为 16/18（88.9%）、subsystem recall 18/18（100%）、key-metric case recall 12/18（66.7%）、key-metric coverage 24/68（35.3%）；holdout 分别为 16/18（88.9%）、18/18（100%）、13/18（72.2%）、24/70（34.3%），反泄漏契约均通过。artifact 位于 `evals/aiops2025/recorded_blind_development.json` 与 `recorded_blind_holdout.json`，包含 git commit、manifest/builder/evaluator/summary/metadata SHA-256 和逐 case 无标签指标。该结果只证明本地录制证据提取与 split 契约，不证明 GoS 真实 root-cause accuracy；下一步仍是把 blind 证据接成 case-scoped Tool/RAG replay，再采集真实模型 baseline/compare。当前目标禁止连接服务器，因此云端部署和真实生产遥测保持未验证。

2026-07-15 第五轮 recorded replay 审计：逐 case Engine factory、只读 `query_recorded_telemetry` Tool、同 case RAG 和 evidence corpus SHA-256 已接入，factory 只能看到 case ID，不能读取 evaluator label。18 条 holdout 的 Plan-Execute-Replan baseline 为 accuracy 50%、evidence precision 34.86%、coverage 50.27%、premature-stop 38.89%、P95 180.007 秒、degradation 11.11%；GoS 为 accuracy 0%、precision 5.17%、coverage 1.09%、premature-stop 100%、P95 25.674 秒、degradation 0%，因此 accuracy、precision、coverage、premature-stop 四项 Gate 失败，Phase 3～7 不得启动。失败产物为 `evals/aiops2025/recorded_plan_baseline_observed.json`（SHA-256 `7de20aa1fed001fb3f75aa998a4c41937fca78d7363bb2a2e5043c75124e998e`）和 `recorded_gos_compare_failed.json`（SHA-256 `d3d589c20ea0db02479a875bcb6054426da0f4a87e23945f595d3d0488485a28`），证据语料 SHA-256 为 `14f24e90b15d092f48c4665196662ef080c3756b930a07ee8e397d644d1fe35b`。失败分析定位到专家将约 8 KiB 的 Tool/RAG 证据硬截断为 500 字符，模型主要看到文档头部和首个 metric，18/18 结果均过早泛化为资源耗尽。此后只能在 development 上修复通用证据压缩/预算；本轮 holdout 已被观察并视为消耗，修复后必须从未使用 case 冻结新的独立 holdout 才能声称新性能。上述产物来自 dirty worktree，目前只有 git commit 与证据语料指纹、没有代码内容指纹，因此只作为观测和失败证据，不作为最终可复现 baseline。

2026-07-16 development v2 修复验证：Evidence metric contract 升级为 `source-backed-signal-v2`，premature stop 只表示错误终态或缺少必需 Refine/Backtrack，不再把“根因未命中”重复计为过早停止；baseline artifact 增加 `complete-trace-profile-activity-v1` 准入，要求逐 case 完整 trace、100% traceability 且 recorded/real 每条发生真实模型调用。稳定模型窗口内 Plan baseline 为 accuracy 50%、precision 34.34%、coverage 43.17%、premature-stop 0%、P95 180.006 秒、degradation 16.67%、traceability 100%；GoS 为 accuracy 72.22%、precision 34.34%、coverage 43.17%、premature-stop 0%、P95 13.224 秒、degradation 0%、traceability 100%，11/11 Gate PASS。baseline SHA-256 为 `272811430e533155924cd8b0e9ae432891085960b4ec63a8db12d41ed2782921`，compare SHA-256 为 `a39f32d8d984e06ecad98d43d0b53857d1adba7919c3005a470e441b1093927b`；该结果只用于 development 修复，不替代独立 holdout。

2026-07-16 holdout v3 失效记录：`recorded_split_v3.json` 从前两代 manifest 排除 54 条已用 case 后冻结 18 条，manifest SHA-256 为 `0ac03450f39a1621f3282d9e3bd21b99c7105886c213d3bb4d9dc624f364031b`。blind 提取获得 288 metric、88 log、93 trace，18/18 非空，反泄漏契约通过。唯一一次 Plan baseline 执行到第 10 条时 DeepSeek 返回 `402 Insufficient Balance`，后续 case 无模型活动；最终 traceability 仅 55.56%，被运行质量准入拒绝且没有生成 artifact。v3 已暴露部分结果并永久视为消耗，不重跑、不调参，也不把中断时的 27.78% accuracy 当作模型能力结论。

2026-07-16 独立 holdout v4 Gate：从新下载并通过官方 MD5 校验的 `2025-06-18.tar.gz`、`2025-06-20.tar.gz` 中，各冻结 9 条此前未见 case；`recorded_split_v4.json` 排除前 72 条已用 case，重复生成内容完全一致，manifest SHA-256 为 `94e24153784d775d1190012535bbf9a6bc31c52e90597481617e228bb6164e76`。blind 提取获得 288 metric、83 log、105 trace，18/18 非空；独立 evaluator 得到 exact entity 16/18、subsystem 18/18、key-metric case 12/18、key-metric coverage 23/69，反泄漏契约通过。唯一一次 Plan baseline 为 accuracy 50%、precision 50.84%、coverage 51.87%、premature-stop 0%、P95 180.006 秒、degradation 27.78%、traceability 100%，artifact SHA-256 为 `45ccb375d41ca0a993c2709f79fc29a68b6e47dadcd2d4381575eefbecb99788`；同一代码、配置和模型窗口内唯一一次 GoS compare 为 accuracy 61.11%、precision 50.84%、coverage 51.87%、premature-stop 0%、graph validity 100%、P95 31.539 秒、平均 LLM calls 3.4、degradation 5.56%、traceability 100%，11/11 Gate PASS，compare SHA-256 为 `5e696ec770c3d94abb98ea44f401f66d2b51f6707c600b715fe200b45bb03d8b`。该 Gate 解锁关闭生产开关条件下的 Phase 3～6 开发；compare 使用 `StateConversion.Enabled=false` 的兼容路径，且 `recorded` 明确是 `development_only`，因此不构成 Phase 7 的真实生产遥测或 State Conversion 灰度证据。

### Phase 1：修正图和证据语义

依赖：Phase 0。

工作项：

- [x] 扩展 `experts.EvidenceItem`，显式表达 relation、target、strength 和 provenance。
- [x] Ingest 不再把原始症状作为“支持所有假设”的证据；Signal 与 Hypothesis 使用 `refines/causal`，只有可验证 Observation 才作为 Evidence。
- [x] 为节点、边、Evidence 定义 invariant validator。
- [x] 实现证据去重键，防止同一 Tool/RAG 结果在多轮中重复加权。
- [x] 实现可升可降的 confidence aggregator，并记录 contribution trace。
- [x] neutral/无法定位目标的证据进入未决区，不强行挂到 frontier。
- [x] 保证 Copy-on-write 校验失败时图完全不变。

验收 Gate：

- refute evidence 能降低目标假设分数，并可能改变 frontier。
- 低 confidence 的 support 不会被误写为 refute。
- 重放同一证据不会重复改变分数。
- 任意 succeeded 结果中的有效 Evidence 均有非空来源；无来源信息不计入 support 数。
- 图 invariant/property tests 通过，`go test -race ./internal/ai/belief ./internal/ai/agent/gos_engine/...` 通过。

### Phase 2：补齐 State Conversion 闭环

依赖：Phase 1。

工作项：

- [x] 抽出独立 `StateConverter`，输入 Graph + FSM + Budget，输出结构化 Decision。
- [x] Decision 至少支持 `continue | refine | backtrack | report | degraded`。
- [x] 实现 `CheckBacktrack`：验证当前 frontier 的 refines 祖先链仍是各层有效最优路径。
- [x] 实现 `Backtrack`：撤回失效层之后的子图，将 FSM 返回失效层并重新选择 frontier。
- [x] 实现 `RefineHypothesis`：进入下一层前先生成和校验子假设，再进行 FSM transition。
- [x] 实现 `CheckGranularity`：判断是否已形成可操作根因，而不是只看 confidence。
- [x] 明确无候选、单候选、分数平局、最大层级、最大步数和预算耗尽的行为。
- [x] 禁止出现“FSM 已 Drill-down，但下一层没有节点”的中间状态。

验收 Gate：

- 构造“初始选错根因，后续反证触发回溯”的测试，最终 frontier 回到正确分支。
- 构造“粗粒度假设需要细化”的测试，必须生成 level+1 子假设后才进入下一层。
- Backtracking 后历史节点仍可从 snapshot/trace 查看，但不参与活跃 frontier。
- 状态转换表全部有单元测试，非法转换被拒绝并返回明确原因。

本地验证证据：`state_converter_test.go` 覆盖错误分支回溯、先生成子假设再 Drill-down、历史节点撤回状态、五类 Decision、边界行为和非法转换；Engine 集成测试覆盖 L1 Refine 到 L2 后报告。实验开关默认关闭，尚未用真实 holdout 验证开启后的准确率、回溯成功率和 premature-stop rate。

第二轮审计补强：Evidence invariant 现在校验 observation time、relation/target/strength 与 attrs/provenance 一致、dedup key 一致，以及 support/refute 必须恰有一条匹配关系边；confidence trace 保存逐证据 relation/strength/key 明细。StateConverter 在图事务前拒绝非 Drilling 状态的 Refine/Backtrack，避免 FSM 拒绝后图已提交的半事务状态。

### Phase 3：结构化认知层

依赖：Phase 2。

工作项：

- [x] 将 Ingestor 改为“规则保底 + LLM 结构化提议”，支持从告警、变更和遥测中拆分 Signal/Observation/L1 Hypothesis。
- [x] 所有 prompt 移入项目 prompt registry，不在 Go 文件内硬编码大段 prompt。
- [x] Planner 根据 frontier、missing evidence、已调用专家、失败工具和剩余预算生成 `PlanItem[]`。
- [x] PlanItem 增加预期证据、允许工具、停止条件和预算，不只包含专家名称。
- [x] Graph Updater 接收专家 proposal，校验后更新节点、边和置信度。
- [x] 为 JSON schema 错误、未知节点 ID、循环 refines、越界分数和证据来源缺失提供降级路径。
- [x] 保留确定性 fallback，LLM Planner 不可用时仍能按领域规则选择专家。

验收 Gate：

- Planner 能解释“为什么调用该专家、希望补哪类证据”。
- 同一专家不会在没有新证据目标时被无限重复调用。
- 无效 LLM 输出不污染图，运行结果为可解释 degraded 或 fallback。
- Ingest/Plan/Graph Proposal 都有 schema contract tests。

本地验证证据：`structured_cognition_test.go` 覆盖结构化 Ingest 的原子提交、未知字段 fallback、取消不写图，以及 Planner 的证据目标、授权工具、预算、无效 proposal fallback 和同目标去重；`experts_test.go` 覆盖 Graph Proposal 的严格 JSON/refinement schema 与无部分 mutation；`engine_test.go` 覆盖 source-backed refinement、未知目标、缺失 provenance、循环语义、越界分数、整批 COW 回滚及开关关闭兼容路径；`state_converter_test.go` 验证 StateConverter 复用 updater 已提交的 L2 子节点而不重复生成。`go test -race ./internal/ai/agent/gos_engine/... ./internal/ai/agent/experts ./internal/ai/belief ./internal/ai/service`、对应 scoped `go vet` 与 `git diff --check` 通过。真实模型 development compare 尚未在 Phase 3 单独重跑，生产 `structured_cognition.enabled` 继续保持 `false`。

### Phase 4：专家 ReAct 与预算治理

依赖：Phase 3。

工作项：

- [x] 专家只接收与当前 frontier 相关的压缩图、证据缺口和授权工具。
- [x] 每个专家配置 Tool/RAG/LLM 调用预算、timeout、最大 retrieval steps 和最大输出 tokens。
- [x] 调度器支持按互补证据选择 1..N 个专家，而不是固定单专家或无差别全扇出。
- [x] 并发专家结果使用稳定顺序聚合，避免 goroutine 完成顺序影响结果。
- [x] 工具失败按工具级别记录，不把“所有专家部分失败”直接等同于整轮失败。
- [x] 增加循环检测：连续两轮没有新节点、有效新证据或 frontier 变化时停止探索。

验收 Gate：

- 调用预算在成功、超时和取消路径都不会被突破。
- 专家并发结果的提交顺序可复现。
- 单个专家失败时，其余有效证据仍可进入图并形成 degraded 结果。
- 相比 Phase 0 baseline，LLM/tool calls 不因专家并发无上限增长。

本地验证证据：内置专家实现 `RunPlanned`，Engine 为其构造只包含当前 frontier、祖先、相关 observation/evidence 与起始 Signal 的只读压缩图；plan 的 expected evidence、allowed tools、stop conditions 和预算进入专家执行上下文。Planner 同时校验 session 剩余预算与专家上限，Engine 调度后预留 session budget；BaseExpert 对 LLM/Tool/RAG calls、整体 timeout、retrieval steps 和模型 `max_tokens` 执行硬限制。生产 GoS 使用的 `query_logs` 与 `query_internal_docs` 均通过 Eino `ToolInfo.ParamsOneOf` schema contract 测试；调用由 plan allowlist 授权、context timeout 限制，失败以结构化 degraded 结果进入专家状态。`experts_test.go` 覆盖成功、超时、主动取消、工具授权和 max output tokens；`structured_cognition_test.go` 覆盖互补双专家计划与超预算 proposal fallback；`engine_test.go` 覆盖并发完成顺序不影响提交顺序、压缩图隔离、全部 partial failure 保留可用证据、单专家失败时有效证据入图且最终降级，以及配置化两轮无进展停止。`git diff --check`、两份 YAML 解析、scoped `go test -race` 和 `go vet` 全部通过。生产开关继续关闭，Phase 4 尚未单独重跑真实模型 compare。

### Phase 5：证据化报告与用户可见 Trace

依赖：Phase 4。

工作项：

- [x] 报告固定包含结论、置信度、支持证据、反驳/冲突证据、未解决缺口、建议下一步。
- [x] 报告中的每个关键判断可映射到 graph node/edge 和 Evidence source。
- [x] confidence 来自图聚合结果，不直接采用单个专家自报值。
- [x] 如果只有弱证据或仍存在关键冲突，必须返回 degraded，不输出确定性根因。
- [x] API/前端展示 state transition、backtrack 和 drill-down 事件，但默认折叠内部推理细节。
- [x] 对用户输出继续通过现有 schema gate、contract gate 和 output filter。

验收 Gate：

- succeeded 结果的结论全部有 source-backed evidence。
- 报告不会引用已 retracted 的节点作为有效依据。
- 前端能区分“探索”“钻取”“回溯”“报告”“降级”五类事件。
- 报告契约和 SSE 事件顺序有集成测试。

本地验证证据：`EvidenceReport` 以当前活跃 frontier 为结论节点，从其活跃 ancestor path 收集 evidence，并在每项中保留 `node_id`、`edge_ref`、`target_hypothesis_id`、relation/strength 与 source provenance；最终 confidence 直接读取图分数。直接 source-backed support 数量不足、图置信度不足、达到配置阈值的反驳证据或专家 partial failure 均返回 `ResultStatusDegraded`，retracted evidence 不进入报告。Engine 通过统一 `state_transition/state_kind` 发出 `explore | drill_down | backtrack | report | degraded`，Runtime 仍按原有 TaskEvent ledger/API 输出，前端在 `ThinkingCollapse` 中插入独立状态项且非 streaming 时默认折叠。防遗漏审计发现同步入口过去只过滤正文/详情/执行计划，而异步 result、Evidence、NextActions、degradation reason 和用户可见 Trace 未统一过滤；新增失败测试复现后，现已在 Application 层对同步/异步输出和嵌套 Trace payload 做同一 output filter，且不修改 Ledger 原始记录。`report_test.go` 覆盖 source 映射、图置信度、强反证降级、retracted 排除和状态事件顺序（含 Drill-down→Backtrack）；`aiops_app_test.go` 覆盖四类用户可见输出的敏感内容脱敏；scoped tests/race/vet、两份 YAML 解析、`git diff --check` 与 `frontend/npm run build` 通过。生产开关继续关闭，Phase 5 尚未单独重跑真实模型 compare。

### Phase 6：性能、持久化与可观测性

依赖：Phase 5。

工作项：

- [x] 评估每次 mutation 全量 snapshot 的内存与序列化成本，改为可配置的 checkpoint + delta。
- [x] 设置最大 graph nodes/edges/depth/snapshots，超限时明确 degraded。
- [x] Trace 增加 phase latency、frontier change、backtrack count、new evidence count、confidence delta。
- [x] 对超时、取消和 panic recovery 做故障注入测试。
- [x] 明确多实例下 graph/trace/result 的存储边界；需要跨实例查询时使用 Redis Ledger，artifact 仍需单独解决共享存储。
- [x] 增加 prompt/model/config version，支持结果复盘。

验收 Gate：

- 目标规模图下不存在无界内存增长。
- `go test -race` 无竞态；context 取消后没有遗留专家 goroutine。
- P95 latency、调用次数和图规模均能从指标定位。
- 任何资源上限触发时返回 degraded，不 panic/fatal。

本地验证证据：`BeliefGraph` 首次事务写入生成完整 checkpoint，后续 mutation 只保留可重放的 node/edge upsert delta，并按 `checkpoint_interval` 周期生成新 checkpoint；`max_nodes/max_edges/max_depth/max_snapshots/max_deltas` 全部由 `aiops.gos.graph` 集中配置。Copy-on-write 在提交前校验结构与历史容量，资源超限保持原图不变并由 Engine 统一返回 `graph_resource_limit` degraded。100 节点/99 边目标规模测试中，checkpoint+delta 历史为 325,686 bytes，对照每次 mutation 全量序列化为 3,338,378 bytes，减少约 90.2%，且 checkpoint/delta 数量均受硬上限约束；该数字仅是本地结构成本，不是生产 P95。

Engine 的结果 metadata 和最终 `observability` Trace 事件均包含 ingest/plan/act/update/state-conversion/report latency、frontier change、backtrack、new evidence、confidence delta、LLM/tool/RAG calls 与图/历史规模；Prometheus 暴露 `opscaptionai_gos_run_duration_seconds`、`opscaptionai_gos_phase_duration_seconds`、`opscaptionai_gos_calls_per_run`、`opscaptionai_gos_graph_size` 和 `opscaptionai_gos_transitions_per_run`，时延桶覆盖至 300 秒。结果同时记录 GOS prompt SHA-256、解析后的 model version/model path 和不含函数/密钥的 config SHA-256。

专家 Tool/RAG/LLM 与 `aiOpsGOSAgent.Handle` 的内部超时 goroutine 已移除，取消直接沿 context 合约传入依赖；Engine 使用 `errgroup.Wait` 等待专家退出，并在专家边界和 Engine 顶层 recovery 后返回 degraded。故障注入覆盖 deadline、显式 cancel、专家 panic、structured generator panic，并验证 Engine 返回前专家 goroutine 已退出。graph 写入最终 `TaskResult.Metadata`，trace/result 由 Runtime Ledger 持久化；生产多实例配置改用 Redis Ledger，artifact 明确仍需独立共享存储。scoped/full tests、`go test -race`、`go vet`、`go build ./...`、前端 build、两份 YAML 解析、`git diff --check` 与 11 项 deterministic Gate 通过，CodeGraph 已刷新；生产默认引擎与三个 GoS 开关继续关闭，Phase 6 未执行服务器部署或真实生产遥测验证。

### Phase 7：真实对照与灰度

依赖：Phase 6。

工作项：

- [ ] 在真实服务器依赖下生成同版本 Plan-Execute-Replan baseline artifact。
- [ ] 运行 `gos-profile=real` compare，不使用 fake Tool/RAG/LLM。
- [ ] 对失败 case 做 phase breakdown，不修改 holdout 标签迎合结果。
- [ ] 先 shadow run，再内部小流量 feature flag，最后才讨论默认切换。
- [x] 配置一键回退到 Plan-Execute-Replan。

2026-07-16 真实服务器只读审计证据：生产 Compose 服务、健康检查和就绪检查正常，知识集合 schema 正常且有 3,185 条记录；服务器内 DeepSeek 最小请求返回成功。当前本地代码通过临时 SSH 转发和严格只读客户端完成真实豆包 Embedding + 生产 Milvus 检索，返回 5/5 份可用文档；探针未调用数据库/集合创建或 `LoadCollection`，未输出文档内容，结束后已关闭转发并删除临时代码。在线 Prometheus 只有 backend 与自身两个健康 target，没有逐 case 故障遥测；日志 MCP 虽可访问，但抽样命中的 7 条记录全部是读取失败信息，且缺少时间戳、namespace 与 provenance，不能证明是目标 case 的真实日志。在线后端镜像版本早于当前本地 HEAD，部署目录没有当前源码或 `gos_eval`，当前 GoS dirty worktree 也未部署。评测器对 `--mode=baseline --gos-profile=real` 与 `--mode=compare --gos-profile=real` 均在模型/工具调用前以 `telemetry_profile=synthetic` 拒绝，退出码为 1 且没有生成 artifact；不得通过手工改成 `real` 绕过证据门禁。因此本轮保持前两项未勾选，阻断原因是缺少逐 case、可核验、与 ground truth 对齐的真实遥测，以及 baseline/compare 与目标运行版本不一致，而不是服务器或模型不可连接。

2026-07-19 大样本 `recorded_blind` 评测证据：从 AIOps2025 剩余未使用样本中冻结 v5 holdout，共 30 个 case、19 个唯一 ground truth，覆盖 service 12、pod 14、node 4；提取阶段只使用时间窗，完成后 evaluator 才读取标签，反泄漏契约通过，30/30 证据非空。该 profile 使用真实 DeepSeek 模型和 case 隔离的录制遥测，但不是真实生产遥测，因此仍标记 `development_only`。评测过程中发现旧关键词规则只命中实体即可误判，以及 `fault_description` 中的关联实体被错误放入原因关键词；现已改为“至少命中一个原因词且至少命中一个实体词”，原因词只来自故障类型与显式同义词，并保留可审计的离线重评分来源。最终 GoS 严格命中 0/30，Wilson 95% 区间为 0%–11.35%；6 succeeded、24 degraded，降级率 80%，证据 precision 20.0%、coverage 2.15%，图有效率与 traceability 均为 100%，平均 5.57 次 LLM、2.60 次 RAG、0 次 Tool，平均延迟 25.12 秒、P95 48.34 秒，30 个 case 的失败阶段均落在 report。结果说明图和 Trace 合约可用，但诊断结论不可用，不能灰度。冻结结果见 `evals/aiops2025/recorded_holdout_v5_gos_batch_rescored.json`。

同一 v5 前 10 个 case 的 Plan-Execute-Replan 成本受控对照为 2/10 严格命中（20%，Wilson 95% 区间 5.67%–50.98%），9 succeeded、1 degraded，证据 precision 43.41%、coverage 50.59%，平均 25.2 次 LLM、16 次 Tool，平均延迟 83.10 秒、P95 180.01 秒。GoS 在相同 10 个 case 为 0/10；Fisher 双侧检验 `p=0.474`，样本不足以证明架构优劣。Plan 对照只用于确认“证据可被另一链路消费”，不能替代完整同规模 baseline，也不能作为生产准入结论。冻结结果见 `evals/aiops2025/recorded_holdout_v5_plan_batch_10.json`。

本轮失败链路按以下顺序修复，顺序不得颠倒：

1. [x] **P0 评测可信度**：原因/实体分离、标签污染测试、压缩 recorded 证据计数、逐 case 原子 checkpoint、身份漂移拒绝、离线重评分 provenance、LLM 调用和磁盘阈值预算。
2. [x] **P1 证据优先初始化**：`Run` 已在 ingest 前复用 `PlannedExpertAgent` 执行一次受预算、只读、case-scoped 的 evidence bootstrap；只允许显式只读工具，获得第一条来源明确的证据即停止。代码固定 observation 的 source ID，模型只能生成候选，不能伪造 provenance；取证为空或证据化 structured ingest 失败时保持空图并返回 degraded，不再落入“资源耗尽”等固定候选。生产配置与 GoS 总开关仍保持关闭。
3. [x] **P2 证据原子化**：Tool/RAG 输出已按 metric/log/trace/alert/RAG 文档拆成独立 evidence；evidence 与初始化 observation 均保留 `signal_type`、实体、观测时间和 `artifact_ref`，evidence 另保留 relation 与目标 hypothesis。`evidence_max_items` 集中限制单次取证原子数并优先覆盖不同信号类型；报告只在 Markdown 展示层截断 snippet 和限制条数，图与 evaluator 继续使用完整原子证据。
4. [ ] **P3 结构化判断可靠性**：修复 `structured_assessment_invalid`、10 秒模型超时与重试放大；分别统计 decision/content/evidence-assessment 的失败率。只有 schema-valid 的 support/refute 才进入图，失败继续 degraded，不能降级为无来源的确定性结论。
5. [ ] **P4 development 复测**：只在已消费的 v1–v4 development 数据上调试 P1–P3，使用同一严格 matcher，要求根因准确率、证据 coverage 和降级率同时改善；不得根据 v5 标签逐 case 打补丁。
6. [ ] **P5 新 holdout 与生产门禁**：v1–v5 共 120 个 AIOps2025 case 已全部被本项目使用，v5 从本轮起不再算“未见 holdout”。生产准入需要新来源的至少 100 个独立 case，并尽量保证每个主要故障类型不少于 20 个；随后再做同规模 Plan baseline、shadow run 和灰度。没有新 holdout 前保持 GoS 开关关闭。

P1 development 证据（2026-07-19）：在已消费的 v1 `recorded_blind` case `50bce1c4-311` 上，第一次真实模型 smoke 的 bootstrap 已完成 1 次只读 Tool 调用，但 structured ingest 在 30 秒边界超时，系统保持空图并以 `evidence_bootstrap_ingest_failed` 降级，未生成泛化候选。相同 case 的第二次独立重跑完成了 evidence-first ingest：从旧链路的 0 observation + 3 个固定规则候选变为 1 个受信 observation + 4 个证据驱动候选，最终候选不再固定为“资源耗尽”。该例仍因后续 `structured_assessment_invalid`、模型超时和 `no_progress_loop` 降级，耗时 133.60 秒、15 次计数内 LLM 调用、1 次 Tool、4 次 RAG；它只证明 P1 顺序和失败边界生效，不证明根因准确率提升。临时可复核 artifact 位于 `/private/tmp/opscaptain-gos-p1-dev-smoke-retry-20260719/run.json`，不纳入版本库；后续严格按 P2、P3 继续。

P2 development 证据（2026-07-19）：复用同一已消费 case `50bce1c4-311` 执行真实模型 smoke，structured ingest 写入 2 个带来源 observation；专家单次取证输出从整份 snapshot 的 1 条证据变为最多 12 条原子证据，实际最终 evaluator 收到 12 条（11 metric、1 log），报告按配置只展示 8 条但未裁剪 evaluator 的完整 snippet。运行保持 `degraded`，失败阶段仍为 report，耗时约 120.08 秒、13 次 LLM、1 次 Tool、4 次 RAG；其中仍出现 `structured_assessment_invalid` 和模型超时，明确留给 P3，不将 P2 的结构闭环误报为诊断准确率提升。临时 artifact 位于 `/private/tmp/opscaptain-gos-p2-dev-smoke-20260719/run.json`，不纳入版本库。

准入 Gate：

- Accuracy 不低于同版本 baseline。
- Evidence coverage 不低于 baseline，且 evidence precision/有效来源率满足门禁。
- P95 latency 不超过 baseline 的 1.5 倍。
- 平均 LLM calls 不超过 baseline 的 2 倍。
- Degradation rate 不高于 baseline。
- Traceability 为 100%，Backtracking/Drill-down 专项用例全部通过。
- 未满足任一 Gate 时维持 `aiops.engine=plan_execute_replan`。

本地发布准备证据：`aiops.gos.enabled` 是高于默认引擎选择的单一 kill switch；即使未来把 `aiops.engine` 配成 `gos_engine`，将该开关切为 `false` 后，非显式请求也会立即解析回 `plan_execute_replan`，显式 GoS 请求则返回可见 unavailable/degraded，避免静默伪装成 GoS。`TestSelectAIOpsAgentNameRequiresEnabledGOS` 与 `TestResolveAIOpsAgentNameRejectsExplicitDisabledGOS` 已覆盖两条回退路径；当前 manifest/production YAML 本身均为 `plan_execute_replan + gos.enabled=false`。这里只证明回退逻辑与静态配置准备完成，没有在服务器执行切换、shadow run 或小流量灰度。

## 6. 需求追踪矩阵

| ID | 需求 | 阶段 | 主要代码范围 | 必须测试 |
|---|---|---|---|---|
| GOS-001 | Eval artifact 可复现且无泄漏 | 0 | `cmd/gos_eval/`, `gos_engine/eval/` | dataset split、artifact metadata |
| GOS-002 | Evidence 显式 relation/target/provenance | 1 | `experts/registry.go`, `belief/types.go` | support/refute/neutral |
| GOS-003 | 置信度可升可降且去重 | 1 | `belief/`, `engine.updateGraph` | duplicate/refute/conflict |
| GOS-004 | 图更新原子提交 | 1 | `belief/graph.go` | invalid proposal rollback |
| GOS-005 | 真正生成细粒度子假设 | 2 | `gos_engine/`, `belief/fsm.go` | refine then transition |
| GOS-006 | 祖先链失效时回溯 | 2 | `gos_engine/`, `belief/graph.go` | wrong-path backtrack |
| GOS-007 | Granularity 决定 refine/report | 2 | `gos_engine/` | coarse vs actionable root cause |
| GOS-008 | Planner 面向证据缺口调度专家 | 3 | `gos_engine/planner.go` | plan schema/fallback/dedup |
| GOS-009 | 专家调用受预算和权限约束 | 4 | `experts/`, `gos_engine/engine.go` | timeout/budget/partial failure |
| GOS-010 | 报告可追溯且展示冲突 | 5 | `gos_engine/`, `protocol/`, frontend | contract/SSE/retracted evidence |
| GOS-011 | 图规模与快照有上限 | 6 | `belief/`, config | node/edge/snapshot limits |
| GOS-012 | 真实 compare 通过才允许灰度 | 7 | `cmd/gos_eval/`, config | real profile gate/rollback |

## 7. 每阶段固定验证命令

代码级验证：

```bash
go test ./internal/ai/belief ./internal/ai/agent/gos_engine/... ./internal/ai/agent/experts ./internal/ai/service -count=1
go test -race ./internal/ai/belief ./internal/ai/agent/gos_engine/... ./internal/ai/agent/experts -count=1
go vet ./...
```

确定性回归：

```bash
go run ./cmd/gos_eval --mode=gate --gos-profile=eval \
  --baseline=evals/baselines/gos_baseline.json \
  --output=/tmp/gos_gate.json
```

先做静态预检（不连接外部服务，不输出密钥值）：

```bash
go run ./cmd/gos_eval --mode=preflight
```

真实对照仅在有真实模型、工具、RAG 且获准连接的环境运行：

```bash
go run ./cmd/gos_eval --mode=compare --gos-profile=real \
  --holdout=<holdout.json> \
  --baseline=<plan_execute_baseline.json> \
  --output=<gos_compare.json>
```

若修改 API/前端展示：

```bash
go test ./...
cd frontend && npm run build
```

## 8. 防遗漏检查表

每个 Phase 合并前逐项确认：

- [x] 先新增失败测试，再实现代码。
- [x] 新增配置全部进入 `manifest/config/config.yaml`，没有硬编码预算或阈值。
- [x] Prompt 放入 prompt registry，并记录版本。
- [x] 所有 Tool 有 schema、timeout、权限边界和降级行为。
- [x] 用户输入经过 prompt injection guard。
- [x] 输出经过 schema/contract/output filter。
- [x] Evidence 有真实来源，不使用 Memory 代替原始 query 做 routing。
- [x] 失败返回 `ResultStatusDegraded`，不 fatal。
- [x] context cancellation 贯穿 LLM、Tool、RAG 和专家 goroutine。
- [x] 图更新通过 invariant 校验并原子提交。
- [x] 新状态转换有正向、边界、失败和回滚测试。
- [x] Eval 没有修改 holdout 或用 fake profile 自证。
- [x] 文档区分“已实现”“已验证”“已灰度”“生产默认”。
- [x] 没有恢复 `chat_multi_agent`。

## 9. 推荐的首个实施切片

第一个开发切片只做 Phase 0 + Phase 1 的最小闭环：

1. 增加 support/refute/neutral 和 target/provenance 数据契约。
2. 修复“低 confidence 等于 refute”。
3. 实现可下降、去重的置信度聚合。
4. 增加错误首选假设 + 反驳证据的 deterministic case。
5. 暂不修改 Planner、专家数量、前端和生产开关。

这个切片完成后，GoS 才具备继续实现 Backtracking 和 Drill-down 的可信基础。

## 10. 完成定义

只有同时满足以下条件，才能将本 Spec 标记为 Completed：

- GOS-001 至 GOS-012 全部实现并有测试证据。
- deterministic、fault-injection、real compare 三类评测产物齐全。
- Backtracking 和 Drill-down 不再只是数据结构或事件名称，而能在专项 case 中真实改变推理路径。
- 任何成功结论均能追溯到未撤回的 source-backed evidence。
- 灰度指标通过且具备可验证的一键回退路径。
- README 和简历表述不超过已验证范围，并保留对论文和参考仓库的归因。
