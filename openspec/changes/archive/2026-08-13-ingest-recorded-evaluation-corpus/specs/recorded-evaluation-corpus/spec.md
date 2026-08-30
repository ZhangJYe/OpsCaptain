## Purpose

将仓库外、已标注的 AIOps 观测数据转化为可审计的录制评测语料，并在开发与冻结评测之间防止数据和故障家族泄漏。

## ADDED Requirements

### Requirement: 外置语料准备与来源证明
系统 SHALL 从用户提供的仓库外 AIOps2025 目录读取输入与 ground truth，生成可执行的 recorded evaluation case、语料元数据和 SHA-256 指纹；生成产物 MUST 位于指定的外置输出目录，且不得将原始观测文件复制到仓库或生产镜像。

#### Scenario: 准备完整外置语料
- **WHEN** 操作者提供包含 400 条 input 与 ground truth 的有效 AIOps2025 目录和外置输出目录
- **THEN** 系统生成 development、holdout、corpus manifest 与摘要，并在 manifest 中记录来源、许可、输入/标签/输出指纹、生成时间和 schema 版本

#### Scenario: 拒绝不完整来源
- **WHEN** 输入与 ground truth 缺失、不可解析或 UUID 未一一对应
- **THEN** 系统 SHALL 以非零状态退出，不生成被标记为有效的 corpus manifest，并指出不一致原因

### Requirement: 故障家族隔离的确定性切分
系统 SHALL 使用稳定的故障家族键切分 development 与 frozen holdout；家族键 MUST 至少由 fault type、实例层级、受影响服务/实例与观测日期构成，且同一键的 case 不得同时出现在两个 split。

#### Scenario: 重复执行得到相同 split
- **WHEN** 使用相同版本的来源数据和相同 split 策略重复准备语料
- **THEN** 每个 case SHALL 被分配到相同 split，且输出 manifest 的 case ID 列表和 split 指纹保持一致

#### Scenario: 检测跨 split 家族泄漏
- **WHEN** corpus validate 发现一个故障家族键同时存在于 development 与 holdout
- **THEN** 校验 SHALL 失败并列出泄漏的家族键和相关 case ID

### Requirement: 标签边界与录制评测接入
系统 SHALL 把 ground truth 仅写入离线 expectation 和证据关联字段；运行时输入、RAG 检索内容、Agent 路由提示和模型配置 MUST 不包含 case 的答案标签。生成的 development 与 holdout manifest SHALL 标记 profile 为 recorded，并在报告中保留“录制证据回放、非生产验证”的 truth boundary。

#### Scenario: 运行开发集 recorded 评测
- **WHEN** 操作者引用 development corpus manifest 运行 recorded profile
- **THEN** 系统 SHALL 校验每个 case 的数据指纹和 provenance，执行已启用 adapter，并在 JSON/Markdown 报告中记录数据版本、split、标签来源、指纹、结果和失败样本

#### Scenario: 保护冻结集
- **WHEN** 操作者引用 frozen holdout corpus manifest
- **THEN** 系统 SHALL 拒绝将其作为 regression 或 deterministic profile 运行，并仅允许显式的 recorded 或 live holdout 评测入口使用

### Requirement: 语料规模与完整性报告
系统 SHALL 在 prepare 和 validate 输出中按故障类型、实例层级、服务和证据模态统计 case 数量，并在当前来源的可用 case 少于外部语料登记目标时明确标记为“当前来源不足”，而不得用重复、改写或合成标签填充数量。

#### Scenario: 当前来源不足以满足远期规模
- **WHEN** 已准备数据的 development 或 holdout 数量低于外部语料登记的目标数量
- **THEN** 报告 SHALL 显示实际数量、缺口和来源限制，且仍允许对已准备的有效 split 执行 recorded 评测
