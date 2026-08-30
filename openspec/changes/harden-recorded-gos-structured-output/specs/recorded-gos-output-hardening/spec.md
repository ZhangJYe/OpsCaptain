## Purpose

为 recorded GoS 离线回放提供对真实模型输出的受限兼容与失败归因，使有效证据不因展示格式差异丢失，同时不放宽诊断结论的证据约束。

## ADDED Requirements

### Requirement: 受限结构化输出兼容
系统 SHALL 在 GoS 的结构化入口、规划和专家评估中接受单个 JSON 对象被 Markdown 代码块或有限前后说明包裹的模型输出，并在解析前提取该对象；提取后仍 MUST 执行既有未知字段、字段类型、置信度、证据索引、关系强度及可行动性校验。

#### Scenario: 接受代码块包裹的有效提案
- **WHEN** 模型以 Markdown JSON 代码块返回满足既有入口、规划或分析提案约束的单个对象
- **THEN** 系统 SHALL 解析该对象并将候选、执行计划、证据关系和分析结果写入 GoS 图

#### Scenario: 拒绝不可恢复的提案
- **WHEN** 模型输出缺少必填字段、包含未知字段、包含多个 JSON 对象或违反既有证据约束
- **THEN** 系统 SHALL 保持降级结果，并记录结构化协议失败而不得生成无证据诊断结论

### Requirement: recorded 评测失败归因
系统 SHALL 在 recorded GoS 评测结果中分别统计模型传输/熔断失败、结构化协议失败和图推理无进展失败，并保留每类失败的 case 数；评测结果 MUST 保留 `development_only` 资格标记。

#### Scenario: 模型服务失败不计为协议失败
- **WHEN** 模型调用返回超时、EOF、熔断或其他传输错误
- **THEN** 系统 SHALL 将该 case 归因为模型服务失败，而不得计入结构化协议失败

#### Scenario: 报告混合失败
- **WHEN** 同一批评测同时出现模型服务失败与结构化协议失败
- **THEN** 结果 SHALL 分别展示两类计数及图推理无进展计数，供操作者判断该批次是否具备比较价值

### Requirement: 结构化图推理服务默认开启
系统 SHALL 在服务 manifest 的 GoS 配置中默认启用结构化认知和状态转换。有效运行时配置 MUST 通过现有预算、超时和证据校验约束模型调用；结构化提案的受限 JSON 兼容 SHALL 在该流程启用时生效。

#### Scenario: 服务按默认 manifest 启动
- **WHEN** AIOps 服务未显式覆盖 GoS 的结构化开关
- **THEN** 有效 GoS 配置 SHALL 启用结构化认知和状态转换，并沿用 manifest 中的调用预算、超时和状态转换深度

#### Scenario: 未注入服务配置的单元装配
- **WHEN** 单元测试或显式装配直接使用 `gos_engine.DefaultConfig()`
- **THEN** 其 SHALL 保持无外部模型调用的保守基线，测试可按需显式开启结构化流程
