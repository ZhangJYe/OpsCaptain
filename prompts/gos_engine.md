---
purpose: GoS Belief Engine expert prompts and degraded result template
used_by: internal/ai/agent/experts/linux_sre.go, internal/ai/agent/gos_engine/engine.go
variables:
  - {name}: expert name
  - {frontier.Label}: current hypothesis label
  - {frontier.Why}: hypothesis reasoning
  - {symptom}: original user symptom
  - {tool}: evidence source tool
  - {query}: tool query used
  - {output}: tool output
  - {gap}: missing evidence description
version: "1.0"
---

# GoS Belief Engine Prompts

## Expert System Prompt

```
你是 AIOps SRE 专家。只输出当前步骤需要的内容，不要输出解释、Markdown 或多余前后缀。
```

## Expert Content Prompts (buildContentPrompt)

### tool_call 动作 — 生成工具查询语句

```
专家：{name}
动作：tool_call
假设：{frontier.Label}
依据：{frontier.Why}
症状：{symptom}
已获得证据：
- 来源={tool} 查询={query} 输出={output}
请生成一句适合传给日志或知识库工具的中文查询语句。
```

### retrieve 动作 — 生成 RAG 检索语句

```
专家：{name}
动作：retrieve
假设：{frontier.Label}
依据：{frontier.Why}
症状：{symptom}
已获得证据：
- 来源={tool} 查询={query} 输出={output}
请生成一句适合传给 RAG 的检索查询语句。
```

### analyze 动作 — 生成诊断结论

```
专家：{name}
动作：analyze
假设：{frontier.Label}
依据：{frontier.Why}
症状：{symptom}
已获得证据：
- 来源={tool} 查询={query} 输出={output}
请基于证据输出最终诊断结论和简短建议。
```

## GoS 降级结果模板

当 GoS 引擎未能获得足够证据时输出：

```
GoS 未获得足够可用证据，无法形成可信根因。

缺少或不可用的证据：
- {gap}

下一步建议：
- 补充服务名、告警时间窗、关键日志片段和指标快照。
- 确认知识库 collection 已导入 runbook/历史案例，并检查 Milvus schema 与文档数。
- 确认日志 MCP `/healthz` 和 `/tools/query_logs` 可用后重试。
```

## GoS 阶段事件映射

GoS 引擎通过 EventEmitter 发射以下阶段事件，前端用于实时进度展示：

| 阶段事件 | 含义 | 前端步骤映射 |
|----------|------|-------------|
| `ingest` | 解析症状并建立候选假设 | gos:hypothesis |
| `ingest_done` | 候选假设已建立 | gos:hypothesis (done) |
| `frontier_selected` | 选中 frontier 节点 | gos:experts |
| `expert_planned` | 调度专家计划 | gos:experts |
| `evidence_attached` | 挂载证据到信念图 | gos:evidence |
| `confidence_updated` | 置信度更新 | gos:confidence |
| `fsm_decision` | FSM 状态机决策 | gos:confidence |
| `report` | 生成最终报告 | gos:confidence (done) |
