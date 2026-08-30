## MODIFIED Requirements

### Requirement: 标签边界与录制评测接入
系统 SHALL 把 ground truth 仅写入离线 expectation 和证据关联字段；运行时输入、RAG 检索内容、Agent 路由提示和模型配置 MUST 不包含 case 的答案标签。生成的 development 与 holdout manifest SHALL 标记 profile 为 recorded，并在报告中保留“录制证据回放、非生产验证”的 truth boundary。报告还 SHALL 分别展示模型服务失败、结构化协议失败和图推理无进展失败的 case 数，避免将不可用的模型服务批次解释为诊断质量指标。

#### Scenario: 运行开发集 recorded 评测
- **WHEN** 操作者引用 development corpus manifest 运行 recorded profile
- **THEN** 系统 SHALL 校验每个 case 的数据指纹和 provenance，执行已启用 adapter，并在 JSON/Markdown 报告中记录数据版本、split、标签来源、指纹、结果、失败样本和分类型失败计数

#### Scenario: 保护冻结集
- **WHEN** 操作者引用 frozen holdout corpus manifest
- **THEN** 系统 SHALL 拒绝将其作为 regression 或 deterministic profile 运行，并仅允许显式的 recorded 或 live holdout 评测入口使用
