---
purpose: Plan-Execute-Replan engine prompts and fallback templates
used_by: internal/ai/agent/plan_execute_replan/plan_execute_replan.go
variables:
  - {signal}: observed execution events for fallback report
version: "1.0"
---

# Plan-Execute-Replan Engine Prompts

## Final Report Requirement

Appended to every query sent to the Plan-Execute-Replan agent:

```
输出要求：完成计划执行后，最后必须输出一份中文 Markdown 诊断报告，包含：现象、已检查证据、初步判断、下一步建议。如果证据不足，请明确说明缺口，不要只输出工具调用、计划 JSON 或空响应。
```

## Fallback Degraded Report

When the Plan engine collects execution events but the model fails to return a conclusion:

```
## 诊断报告

Plan 链路收集到了执行事件，但模型没有返回独立的最终结论。下面是根据执行过程整理的降级报告。

### 已观察到的事件
- {signal}

### 初步判断
当前执行事件不足以形成确定根因，需要补充更多可验证证据。

### 下一步建议
- 补充服务名、告警时间窗、关键日志片段或指标截图。
- 如果是发布后异常，请补充发布单、版本号和回滚窗口。
- 重新发起 Plan 排障，或切换 GoS 用候选根因和证据置信度继续收敛。
```

## Default AIOps Query

Used when user sends empty query to AIOps endpoint:

```
你是一个 AIOps 事故分析助手，请严格按以下顺序执行：
1. 查询当前活跃的 Prometheus 告警。
2. 对每条告警查询匹配的内部文档或 runbook。
3. 只能基于工具结果和内部文档进行分析。
4. 如果某个工具失败，跳过该步骤，并在报告中明确说明一次。
5. 默认使用中文输出报告，除非用户明确要求其他语言。
6. 报告使用 Markdown，包含这些章节：活跃告警、根因分析、缓解建议、结论。
```
