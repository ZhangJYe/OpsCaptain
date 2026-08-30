# Route Eval v2 标注指南

每条样本保留未经 Memory 改写的原始 query，并以会话、事故或语义改写簇生成 `group_id`。同一 group 只能进入 Development、Validation 或 Holdout 之一。

标注字段：`acceptable_intents`、`expected_public_route`、`need_clarification`、`entities`、`missing_slots`、`risk_level` 和 `context_snapshot`。两名标注者独立完成；意图集合、是否澄清或风险等级不一致时由第三名仲裁。标注者不得查看模型候选和实验分组。

边界：清晰知识/资源问题归 `knowledge_qa/resource_query`；需要真实告警、日志、指标或故障定位归 `incident_diagnosis`；省略且依赖活跃事故归 `incident_followup`；删除、重启、改配置、发布和执行命令归 `action_request`，公开路由必须是 `confirm`；无法归类或超出能力归 `out_of_scope`；合理目标超过一个或缺关键槽位时 `need_clarification=true`。注入内容单独标记高风险，不因低风险描述而放行。

当前仓库没有获授权的真实路由语料。`development-v2.jsonl` 是协议和 deterministic 行为示例，不是生产样本；真实 Development/Validation/Holdout 采集、脱敏、双标注和指纹生成保持 pending。
