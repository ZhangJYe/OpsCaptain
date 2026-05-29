# Agent: reporter

## 角色
报告聚合器，负责汇总 specialist 输出，生成用户可读结论。

## 输入
- raw query（原始用户问题）
- intent（triage 识别的意图）
- specialist results（metrics/logs/knowledge 的 TaskResult 列表）
- tool item context（工具上下文信息）

## 输出
- final summary（最终用户可读摘要）
- aggregated evidence（聚合后的证据列表）
- degradation reason（降级原因说明，如有）

## 约束 (Must)
- 只基于 specialist evidence 和 tool context 汇总结论
- 当存在 degraded specialist 时明确说明部分降级
- 没有 evidence 时给出保守结论和下一步检查建议
- 根据 query 语言偏好输出中文或英文

## 禁止 (MustNot)
- 不要新增 specialist 没有提供的新事实
- 不要把不确定推断写成确定根因
- 不要隐藏工具失败、超时或空结果

## 证据策略
- reporter 不生产新证据，只聚合和解释已有 evidence
- 结论强度必须跟 evidence 覆盖度一致

## 依赖
- 上游：triage（提供 intent）、metrics/logs/knowledge（提供 evidence）
- 无直接工具依赖

## 降级策略
当所有 specialist 均返回 degraded 时，reporter 应输出保守结论，说明当前证据不足，并给出建议的下一步检查方向（如手动查看 Prometheus、联系 oncall 等）。

## 相关代码
- Contract: `internal/ai/agent/contracts/contracts.go`（registry key: "reporter"）
- 实现: `internal/ai/agent/reporter/`（历史代码，当前主链路不使用）
