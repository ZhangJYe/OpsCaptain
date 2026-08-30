## Context

参见 `proposal.md`。当前 `AgentRouterApp.Decide` 的自动链路是：以 `route mode + diagnosis strategy + query hash` 查询进程内缓存，随后匹配高置信故障关键词，最后调用 Flash 返回 `chat / incident`、推荐诊断策略、置信度与理由；低置信、结构错误或依赖失败统一返回 `confirm`。该链路只使用当前 query，能保证 Memory 不覆盖路由输入，但无法处理“那数据库呢”一类依赖活跃流程的省略表达，也没有候选间距、缺槽、上下文冲突和风险语义。

Route Adapter 已能报告 Macro-F1、每类指标、低置信、缓存和 fallback。现有证据边界如下：

| 证据 | 规模 | 已记录结果 | 能证明什么 | 不能证明什么 |
|---|---:|---:|---|---|
| `recorded-development` Route fixture | 3 条 | 覆盖 chat、incident、confirm | 路由 schema 与三条分支可重放 | 真实模型准确率、上下文收益、线上收益 |
| deterministic regression 历史成功报告 | 28 条 | Macro-F1 1.0、P95 0ms | 固定输出下协议、聚合器与 Gate 一致 | 模型质量与真实延迟 |
| deterministic regression 历史失败报告 | 28 条 | Macro-F1 0.2288 | 数据/fixture 不一致可被 Gate 捕获 | 不可作为当前 A 组模型基线，指纹不同 |
| 真实上下文路由基线 | 尚未建立 | unavailable | - | 任何优化收益 |

因此第一项实施工作不是调 Prompt，而是建立可审计的数据和 A 组真实基线。所有新配置进入 `manifest/config/config.yaml`；路由继续停留在 Application 层，不直接访问基础设施 SDK；模型或上下文依赖失败必须返回 degraded/confirm，不得 fatal。

## Goals / Non-Goals

**Goals:**

- 在保持原始 query 为主输入的前提下，处理多轮省略、缺槽、上下文冲突和高风险意图。
- 让每层决策、澄清原因、实验版本与成本可观测、可重放、可比较。
- 用冻结基线、消融、Shadow 和 A/B 证明各层是否带来净收益，而不是只展示若干成功案例。
- 支持单开关回退当前路由器，不迁移事故历史，不改变 Plan/GoS 的执行语义。

**Non-Goals:**

- 不让长期 Memory 或自由文本历史直接决定路由。
- 不在本变更中自动训练分类器、在线更新 Prompt 或从用户反馈直接改权重。
- 不将路由置信度视为工具权限；不扩大高风险操作的自动执行范围。
- 不把 Route 扩展为新的 Agent 编排器，不恢复已废弃的 `chat_multi_agent`。
- 不在规划阶段编造新的准确率、线上提升或统计显著性。

## Decisions

### 1. 缓存是 Stage 0 加速层，不计入三层语义漏斗

总体流程：

```text
Request
  -> manual mode / feature flag
  -> Stage 0: versioned cache
  -> Layer 1: deterministic guard + explicit fast path
  -> Layer 2: semantic Top-K candidates
  -> Layer 3: bounded context validation
  -> route | clarify | deny/degraded
  -> existing Chat or Incident entry
```

当前只包含 query 的最终路由缓存不能直接复用于上下文相关结果，否则“那数据库呢”会跨会话命中错误路由。B 组采用两类缓存：

- **候选缓存**：键为 query hash + classifier/prompt version，只复用 Layer 2 候选；Layer 3 每次重新校验上下文。
- **最终决策缓存**：仅缓存明确、上下文无关的结果；若使用了结构化上下文，键必须额外包含 context fingerprint、state version 与 policy version。

待澄清、拒绝、降级、高风险和过期上下文结果不缓存。替代方案是删除缓存，实现简单但会丢失当前低延迟收益；直接沿用 query-only 缓存则存在确定的串话风险，因此不采用。

### 2. Layer 1 只处理可证明的确定性边界

Layer 1 依次处理：显式 `react/diagnosis` 模式、安全与注入标记、高风险动作词、配置化强故障模式、必要实体和允许策略。规则输出 `rule_id`、匹配字段、所需槽位和风险等级；只有精度经过独立规则集验证、实体条件完整且不是高风险执行时，才能直接返回公开路由。

“错误”“慢”“数据库”等宽泛词不能单独成为事故快路径。命中高风险动作时只生成 `action_request` 候选并进入 confirm/approval，不产生执行权限。替代方案是继续使用任一关键词即 incident；延迟最低，但假阳性不可解释且会污染事故记录。

### 3. Layer 2 使用细粒度 Top-K，公开协议保持三态兼容

内部候选意图初始定义为：

- `knowledge_qa`
- `incident_diagnosis`
- `incident_followup`
- `resource_query`
- `action_request`
- `out_of_scope`

模型结构化输出：

```json
{
  "candidates": [
    {"intent": "incident_diagnosis", "confidence": 0.62, "reason_codes": ["symptom_present"]},
    {"intent": "knowledge_qa", "confidence": 0.31, "reason_codes": ["explanation_form"]}
  ],
  "entities": {"service": "payment-api"},
  "required_slots": ["time_range"],
  "risk_hint": "low"
}
```

公开映射保持：知识问答和只读资源查询进入 `chat`；故障诊断和故障续问进入 `incident`；操作、高风险、OOD、低置信与无法确认进入 `confirm`。Top-K 默认 2，所有阈值配置化。替代方案是让模型继续只给一个标签；协议更简单，但无法区分“低置信第一名”和“两个候选接近”。

### 4. Layer 3 使用 RoutingContextSnapshot，不读取长期 Memory

结构化快照只包含：

```text
session_key_hash
active_route
active_incident_id
last_confirmed_intent
confirmed_entities{service, task_id, time_range}
pending_slots[]
state_version
updated_at
```

快照由当前会话/事故状态的确定性字段生成，不包含长期记忆、模型总结、历史工具正文或完整对话。校验器检查：状态是否过期、实体是否连续、是否正在等待槽位、候选是否与活跃流程一致、当前 query 是否明确切换主题、策略是否允许。上下文只能把一个已存在的候选提升为可接受或否决冲突候选，不能凭空创造当前 query 完全没有支持的高风险意图。

替代方案是把最近 N 轮原文交给模型；能覆盖更多指代，但隐私、Token、提示注入和历史锚定风险更高，且违反当前 Memory 不替代 query 的项目口径。

### 5. 用选择性分类决策表控制直路由与澄清

初始决策参数全部配置化，并在 Validation 数据上校准：

| 条件 | 结果 |
|---|---|
| 安全拒绝或高风险动作 | `confirm/approval` |
| Top-1 达接受阈值、Top-1/Top-2 间距达标、实体完整、上下文无冲突 | 映射后直路由 |
| 候选接近、缺关键槽位、上下文冲突或 OOD | `clarify` |
| 模型/状态依赖超时、schema 无效 | `confirm` + degraded |

不预先把 `0.85/0.20` 等示例阈值写成事实。实施时先跑 A 组基线和 B 组候选的风险-覆盖曲线，按错误成本选阈值：高风险误放成本最高，其次是错误创建事故，再其次是不必要澄清。替代方案是统一最大概率阈值；无法反映类别和风险成本差异。

### 6. 澄清是带版本的短状态机

澄清记录包含 `clarification_id`、原 query hash、候选、待补槽位、state version、过期时间和问题模板。用户回答后只补充声明的槽位，再重新执行 Layer 3；如果状态版本变化或过期，则从当前 query 与新状态重新走漏斗。每次最多询问一个最能降低歧义的问题，并配置最大澄清轮次；超限后回退人工选择。

替代方案是把澄清回答拼接到原 query 后重新调用整个 Agent；实现快但无法区分用户补槽与新问题，也不利于重放。

### 7. 先建立真实 A 组基线，再评价 B 组

建议建立至少 300 个独立会话/事故簇的初始路由语料，分层建议为：清晰问答 60、清晰故障 60、易混淆 60、多轮省略 50、上下文切换/缺槽 30、高风险 20、OOD 20；数量是首轮采样目标，不是已完成数据。另维护独立的高风险与注入安全集，不以总体分布稀释。

每条样本由两人独立标注，冲突进入仲裁，字段包括 `acceptable_intents`、`expected_public_route`、`need_clarification`、`entities`、`missing_slots`、`risk_level` 和 `group_id`。按 group 划分 Development/Validation/Holdout；Holdout 在候选冻结后只运行一次。真实 query 需脱敏，报告默认只保存 hash 和受控摘要。

当前 baseline 记录如下：

```yaml
contract_baseline:
  recorded_development_cases: 3
  deterministic_regression_cases: 28
  deterministic_macro_f1: 1.0
  evidence_boundary: fixture contract only
historical_noncomparable_failure:
  cases: 28
  macro_f1: 0.228758
  reason: different dataset/fixture fingerprint; proves gate detection only
model_quality_baseline:
  status: unavailable
  required_before_ab: true
```

### 8. 离线比较包含完整 B 与两个消融

同一冻结数据、相同模型预算和评分器运行：

- A：现有 cache/keyword/single-Flash。
- B-full：三层漏斗。
- B-no-context：移除 Layer 3。
- B-no-fast-path：所有非手动请求进入 Layer 2/3。

主要离线指标：公开路由 Macro-F1、歧义消解准确率和高风险错误放行；辅助指标：Top-2 Recall、ECE/Brier、不必要澄清率、漏澄清率、澄清后正确率、上下文误用率、平均新增轮次、P95、LLM 调用和 Token。首轮候选 Gate 采用预注册的配置值，建议默认：高风险未经审批执行必须为 0；总体 Macro-F1 相对 A 不低于 1 个百分点；歧义层准确率相对 A 至少提升 5 个百分点；P95 增量不超过 50ms；单位请求 Token 增量不超过 20%。这些是进入 Shadow 的工程门槛，不是对最终效果的预言，可在看到 Holdout 前基于业务成本修订一次并冻结。

### 9. Shadow 只记录反事实，不改变服务决策

Shadow 中 A 组同步服务用户，B 组通过独立 timeout/budget 旁路运行。B 不创建事故、不调用工具、不生成面向用户的澄清。事件至少包含：experiment id、router/policy/model/prompt/context schema 版本、assignment key hash、query/context hash、A/B 候选与最终决策、差异类型、各层延迟、调用与 Token、A 实际执行结果。

Shadow 通过条件：覆盖一个完整业务周期；安全 Gate 全通过；B 完成率、分层离线一致性和延迟满足门槛；对 A/B 分歧样本完成盲审。替代方案是直接 5% A/B；能更快获得用户行为指标，但路由错误会真实创建事故，不适合作为第一步。

### 10. A/B 使用服务端稳定分桶和固定周期

分桶键为服务端生成的匿名用户或会话标识，通过 HMAC(experiment salt, assignment key) 映射，客户端参数无效。同一会话固定变体，避免上下文跨组污染。实验阶段：

1. 5% B 低风险 canary；
2. 25% B，检查分层和时段稳定性；
3. 50/50 固定周期实验；
4. 只有结论通过后才讨论默认开启，仍保留回滚开关。

主要线上指标为 `task_success_without_route_override`：下游流程完成，且用户/人工在配置窗口内没有切换为另一执行路径。辅助指标包括人工改路由率、事故误创建率、澄清完成率、平均澄清轮次、用户放弃率、无效工具调用、P95、错误率和成本。

实验开始前根据 A 组主要指标基线、最小可检测效应、显著性与功效计算样本量，并冻结最短周期；按会话聚类统计，不能把同一用户多轮请求当作独立样本。结论报告效应量与 95% 置信区间，不因每日查看结果提前宣布胜出。若必须连续监控，使用预先选择的序贯检验而不是临时多次显著性检验。

### 11. Guardrail 优先于主要指标

阻断 Guardrail：未经审批的高风险执行必须为 0；B 组错误率、超时率、P95、事故误创建率不得越过配置上限；安全/注入集不得回退。任一阻断指标触发时，服务端 feature flag 立即把所有服务决策切回 A，Shadow/Trace 可独立关闭，实验事件保留用于复盘。

### 12. 反馈只作为评测标签来源，不在线自学习

用户手动切换路径、取消事故、完成澄清、下游任务状态和人工审核形成反馈事件。反馈通过 `request_id/session_id/experiment_id` 关联并去重，先进入离线标注与评测集候选池；不得直接修改规则、Prompt 或模型参数。这样避免错误反馈立即污染路由，并保持实验代际可复现。

## Risks / Trade-offs

- [上下文让旧主题错误延续] → 只使用结构化活跃状态、TTL、版本和显式主题切换校验；报告 context misuse rate。
- [澄清提高准确率但伤害体验] → 同时 Gate 不必要澄清、平均新增轮次和放弃率，限制最大澄清轮数。
- [三层链路增加延迟和成本] → 保留规则与候选缓存，Layer 3 只在歧义或有活跃状态时运行，使用独立预算。
- [离线标签把主观意图当唯一真值] → 支持 acceptable intents 与 need_clarification，双标注和仲裁。
- [A/B 分组污染] → 按用户/会话稳定分桶，统计时按主体聚类；实验期不跨组。
- [观察到结果后调阈值造成过拟合] → Development 调试、Validation 校准、Holdout 冻结；每次版本变化创建新代际。
- [当前 fixture 1.0 造成虚假安全感] → 在所有报告中强制显示 profile、依赖和 evidence boundary，真实基线缺失时显示 unavailable。
- [Shadow B 影响主请求资源] → 独立并发、timeout、Token 与熔断预算；资源紧张时先关闭 Shadow。

## Migration Plan

1. 扩展 Route eval schema 与报告，但保持 v1 fixture 可读；录入当前契约 baseline 和 unavailable 真实基线。
2. 建立、审查并冻结分层 Development/Validation；运行 A 组，形成第一份真实模型质量 baseline。
3. 实现候选 schema、Layer 1/2/3、上下文快照和澄清状态机，默认 feature flag 关闭。
4. 在冻结数据上运行 A、B-full 与两个消融；冻结通过 Validation Gate 的唯一候选，再运行 Holdout。
5. 开启 Shadow，完成分歧盲审和资源评估；不改变任何用户路径。
6. 依次进行 5%、25%、50/50 低风险 A/B；每阶段满足最短周期、样本量和 Guardrail 后再推进。
7. 若 B 胜出，记录适用人群和证据边界后再修改默认开关；高风险执行边界保持不变。

回滚只需关闭 `agent_router.intent_funnel.enabled` 或实验服务开关，所有请求立即使用冻结 A 组路由器；新字段保持向后兼容，无需迁移或删除事故记录。

## Open Questions

- 真实路由样本能否获得用户明确的“人工改路由”事件；若暂时没有，首轮线上主要指标需使用人工盲审与下游完成状态组合代理，并明确其局限。
- 当前单实例路由缓存后续是否迁移 Redis；本变更先保持进程内缓存并把 router/policy/context 版本放入键，不把缓存迁移作为 A/B 前置条件。
