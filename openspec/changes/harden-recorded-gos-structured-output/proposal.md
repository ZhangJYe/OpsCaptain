## Why

真实模型的 recorded GoS 回放暴露了两类问题：评测路径曾将调用超时压缩为 10 秒，且模型出现代码块、前后说明或轻微字段偏差时会被直接判为结构化输出无效。结果是有效的盲化遥测无法进入图推理，评测大量降级，无法区分模型服务不可用与协议兼容性问题。

## What Changes

- recorded GoS 保持紧凑、隔离的评测限制，但使用有效配置中的模型调用超时。
- 为模型返回的结构化入口、规划和专家评估增加受限的 JSON 提取与规范化：只接受既有 schema，拒绝缺少关键字段、未知语义或不合法的结果。
- 将“模型传输/熔断失败”和“结构化协议不兼容”分别计数并写入评测结果，避免把基础设施故障计为模型诊断能力。
- 将服务配置中的结构化认知与状态转换默认开启，使模型产生的候选、反证和细化能够进入 GoS 图状态机；保留受配置约束的调用预算和超时。
- 补充单元测试，覆盖代码块包裹、前后文本、合法最小输出和不可恢复的非法输出。

## Capabilities

### New Capabilities

- `recorded-gos-output-hardening`: recorded GoS 评测对真实模型结构化输出的受限兼容、降级归因和可审计统计。

### Modified Capabilities

- `recorded-evaluation-corpus`: 明确 recorded 回放报告必须区分模型服务失败与结构化协议失败。

## Impact

- 影响 `cmd/gos_eval` 的 recorded profile 配置与结果报告。
- 影响 `internal/ai/agent/experts` 的结构化评估解析路径及其测试。
- 不改变线上 AIOps 路由、不放宽 recorded case 隔离或标签防泄漏约束。
