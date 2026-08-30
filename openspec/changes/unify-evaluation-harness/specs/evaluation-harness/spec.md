## Purpose

为 OpsCaptain 的 Route、RAG、Plan、GoS、Tool 与 Evidence 链路提供统一、可复现、可比较且可追溯的离线评测协议，同时保留各领域指标和失败诊断的独立语义。

## ADDED Requirements

### Requirement: 统一 suite 编排
系统 SHALL 提供一个统一评测入口，允许调用者选择 Route、RAG、Plan、GoS、Tool、Evidence 中的一个或多个 suite，并在同一 run 下生成独立结果和汇总结果。一个 suite 失败不得抹去其他已完成 suite 的报告。

#### Scenario: 执行多个 suite
- **WHEN** 调用者选择 Route、RAG 与 Evidence suite
- **THEN** 系统分别执行三个 suite，并在同一 run 报告中保存每个 suite 的状态、指标和 case 结果

#### Scenario: 单个 suite 失败
- **WHEN** 一个 suite 因配置或运行错误失败而其他 suite 已完成
- **THEN** 系统保留已完成结果，将失败 suite 标记为 failed，并使本次 run 的最终状态反映该失败

### Requirement: 版本化数据集协议
系统 SHALL 使用版本化 manifest 声明数据集角色、suite、schema 版本、case 文件、标签来源和内容指纹。case SHALL 包含稳定 ID、原始输入、预期行为和版本化领域 payload；不同 suite 不得通过无版本的任意字段共享隐式语义。

#### Scenario: 加载有效数据集
- **WHEN** manifest、case schema、suite 和数据集角色彼此兼容且内容指纹匹配
- **THEN** 系统接受数据集并在报告中记录其身份与指纹

#### Scenario: 拒绝不兼容数据集
- **WHEN** case schema 版本不受支持、suite 不匹配或内容指纹不一致
- **THEN** 系统在执行前拒绝该数据集并给出可定位的验证错误

### Requirement: Development、Holdout 与 Regression 隔离
系统 SHALL 区分 development、holdout 与 regression 三种数据集角色。调参与失败分析 SHALL 使用 development；合入回归 SHALL 使用 regression；最终候选验证 SHALL 使用未参与当前调参的 holdout。报告 SHALL 明确数据集角色，不得将 development 结果描述为 holdout 或生产效果。

#### Scenario: 调参运行 development
- **WHEN** 调用者运行用于阈值或策略选择的评测
- **THEN** 系统仅接受 development 数据集，并在报告中标记结果可用于开发比较但不代表最终验证

#### Scenario: 候选方案运行 holdout
- **WHEN** 调用者以 holdout 模式验证冻结候选方案
- **THEN** 系统拒绝任何声明为 development 或 regression 的数据集，并记录候选配置指纹

### Requirement: 运行 profile 真实性边界
系统 SHALL 支持 deterministic、recorded 与 live profile，并在报告中记录依赖状态和证据来源。deterministic 或 recorded 结果不得被标记为 live 或 production；live 依赖不可用时 SHALL 标记为 degraded、failed 或 skipped，而不是自动回退后报告通过。

#### Scenario: Recorded 评测
- **WHEN** suite 使用录制的工具响应或证据语料运行
- **THEN** 报告标记 profile 为 recorded，记录语料指纹，并禁止出现 live 或 production 验证声明

#### Scenario: Live 依赖不可用
- **WHEN** live profile 所需模型、向量库或监控系统不可用
- **THEN** 对应 suite 明确标记 degraded、failed 或 skipped，并且 Gate 不得把该状态计算为通过

### Requirement: 复用领域 evaluator
系统 SHALL 通过统一 Runner 契约适配现有领域 evaluator，并保留其原有评分口径。统一层不得用公共“准确率”替换 RAG 排序指标、GoS 图推理指标、Plan 任务指标、路由分类指标、工具契约指标或 Evidence 质量指标。

#### Scenario: 执行现有 RAG evaluator
- **WHEN** 统一入口运行 RAG suite
- **THEN** 系统复用 RAG 领域评分结果，并保留 MRR、Recall@K、Hit@K 和阶段损失等指标

#### Scenario: 汇总异构 suite
- **WHEN** 一次 run 包含多个指标语义不同的 suite
- **THEN** 系统分别展示每个 suite 的领域指标，不计算误导性的跨领域平均准确率

### Requirement: 公共指标与领域指标分层
系统 SHALL 为每个 suite 记录公共指标，包括 case 数、成功率、失败率、降级率、P95 延迟、LLM/Tool/RAG 调用数、Token 或成本信息以及 Trace 完整率；同时 SHALL 保存版本化领域指标。不可获得的指标 SHALL 标记 unavailable，不得静默填零。

#### Scenario: 指标完整
- **WHEN** Runner 能返回延迟、调用计数和领域评分
- **THEN** 报告同时包含公共指标与该 suite 的领域指标

#### Scenario: 成本数据不可获得
- **WHEN** profile 无法提供 Token 或费用数据
- **THEN** 报告将对应指标标记为 unavailable，且不把它作为数值零参与比较

### Requirement: 分层质量 Gate
系统 SHALL 按公共硬门槛、领域 Gate 和跨链路不变量三层执行质量判断。任一 blocking Gate 失败 SHALL 使整体 Gate 失败；非 blocking Gate SHALL 产生 warning。Gate 结果 SHALL 给出阈值、实际值、基线值、方向和失败 case。

#### Scenario: 领域指标回退
- **WHEN** RAG MRR 低于配置阈值或低于基线允许范围
- **THEN** RAG 领域 Gate 失败，并列出造成回退的 case，而不受其他 suite 改善抵消

#### Scenario: 跨链路不变量失败
- **WHEN** 故障请求路由到诊断链但最终结果缺少规定的 Evidence 或 Trace
- **THEN** 跨链路 Gate 失败并定位路由、执行或证据阶段

### Requirement: 路由评测使用原始 query
Route suite SHALL 使用 case 中的原始 query 作为评分输入，并记录期望路由、实际路由、置信度、决策来源和延迟。Memory 可以作为后续执行上下文，但不得替代或改写用于路由评分的原始 query。

#### Scenario: 历史上下文存在
- **WHEN** 路由 case 同时提供原始 query 和历史 Memory
- **THEN** 路由评分仍基于原始 query，报告单独记录 Memory 是否被后续执行使用

#### Scenario: 故障与日常问答区分
- **WHEN** 数据集同时包含故障诊断和日常问答样本
- **THEN** 报告输出混淆矩阵、各路由 Precision/Recall/F1、低置信度率和错误路由 case

### Requirement: Plan 与 GoS 可比但不混同
Plan 与 GoS suite SHALL 共享完成状态、延迟、调用预算、Trace 和 Evidence 等公共字段，同时分别保留 Plan 的步骤执行/重规划指标和 GoS 的根因、图有效性、反驳/回溯指标。系统 SHALL 支持在同一 case 集上比较两者，但不得将 recorded 或 deterministic 比较描述为线上优劣。

#### Scenario: 同 case 对比 Plan 与 GoS
- **WHEN** manifest 声明一组 case 同时适用于 Plan 与 GoS
- **THEN** 报告展示公共指标差异和两个 suite 各自领域指标，并明确 profile 与证据来源

### Requirement: Tool 契约与失败降级评测
Tool suite SHALL 验证工具 schema、权限边界、超时、取消传播、返回格式和失败降级行为。工具失败被转换为可读降级结果时 SHALL 与真实成功分开计数，并不得触发不受控重试。

#### Scenario: 工具超时
- **WHEN** fixture 使工具调用超过配置 timeout
- **THEN** 系统记录 timeout 阶段、取消传播、降级状态和调用次数，并验证没有超出预算的重试

#### Scenario: 权限拒绝
- **WHEN** case 请求未授权工具或资源
- **THEN** Tool suite 验证调用被拒绝、敏感参数未写入报告且整体进程不崩溃

### Requirement: Evidence 可追溯性评测
Evidence suite SHALL 验证结论到 Evidence、Citation 与 Source 的引用完整性、来源有效性、相关性和覆盖率，并区分“检索到证据”与“最终答案实际引用证据”。任何用于评分的敏感原文 SHALL 遵循脱敏与长度限制。

#### Scenario: 存在无来源结论
- **WHEN** 最终诊断包含要求证据支持的结论但未关联有效 Evidence/Citation
- **THEN** Evidence suite 将该 case 标记为 unsupported claim，并降低引用完整率或使配置的 Gate 失败

#### Scenario: 引用可回溯
- **WHEN** 最终答案引用了有效 Citation
- **THEN** 报告能够从结论定位 Citation ID、来源标识和对应 Trace，且不泄露受保护内容

### Requirement: 可复现基线与候选比较
系统 SHALL 为 baseline 和 candidate 记录 dataset、配置、代码、模型、Prompt、evaluator 与证据语料指纹，并在比较前校验兼容性。不兼容产物 SHALL 拒绝比较或明确标记为不可比较。

#### Scenario: 指纹一致的比较
- **WHEN** baseline 与 candidate 的必需指纹兼容
- **THEN** 系统按 suite 和 metric 输出绝对值、差值、变化方向及 Gate 结果

#### Scenario: 评测器版本变化
- **WHEN** candidate 使用不同 evaluator contract 且未声明迁移兼容
- **THEN** 系统拒绝直接比较，并提示重新生成基线或执行显式迁移

### Requirement: 统一报告与失败诊断
系统 SHALL 输出版本化 JSON 报告和 Markdown 摘要。报告 SHALL 包含 run 身份、时间、profile、suite 状态、公共与领域指标、Gate、失败 case、失败阶段、指纹和真实性边界；报告生成失败 SHALL 导致本次 run 失败。

#### Scenario: 生成评测报告
- **WHEN** 所选 suite 执行结束
- **THEN** 系统原子写入 JSON 报告并生成引用同一 run ID 的 Markdown 摘要

#### Scenario: 定位阶段失败
- **WHEN** 一个 case 在 route、retrieve、plan、act、update、report 或 evidence 阶段失败
- **THEN** 报告记录规范化失败阶段、可读原因和相关 Trace 标识

### Requirement: CI 与预算控制
系统 SHALL 提供适用于 PR 的 deterministic smoke/gate 配置，并通过配置限制 case 数、并发、单 case 超时、总超时、LLM/Tool/RAG 调用和 Token/费用预算。recorded 与 live profile SHALL 默认不在普通 PR CI 中运行。

#### Scenario: PR Gate 成功
- **WHEN** deterministic regression 数据集通过所有 blocking Gate 且未超预算
- **THEN** CI 命令以成功状态结束并保留摘要报告

#### Scenario: 超出预算
- **WHEN** suite 超过配置的调用、Token、费用或时间预算
- **THEN** 系统停止后续非必要执行，记录 budget_exceeded，并使 blocking Gate 失败

### Requirement: 兼容迁移与线上隔离
系统 SHALL 在迁移期间继续支持现有 RAG 和 GoS 评测入口与数据集，不要求一次性改写历史报告。Evaluation Harness SHALL 只用于离线或受控评测，不得成为线上 Agent 请求的必经依赖。

#### Scenario: 旧入口继续运行
- **WHEN** 现有脚本调用 RAG 或 GoS 原评测命令
- **THEN** 在迁移窗口内保持原有行为，并可选择额外输出统一报告而不改变原结果

#### Scenario: Harness 故障
- **WHEN** Evaluation Harness 自身不可用
- **THEN** 线上 Chat、Plan 与 GoS 请求链路不受影响
