# Skills 候选卡片提升之后的下一步

## 这一轮已经完成

- 新增 `logs_service_offline_panic_trace`
- 新增 `logs_api_failure_rate_investigation`
- 新增 `knowledge_service_error_code_lookup`
- 给 `knowledgeSkill` 增加 matcher、错误码提取、next actions
- 给 `logSkill` 增加 matcher、场景边界控制、next actions
- 补对应单测
- 补正式 skill cards 和 Learn 复盘文档

## 下一轮建议

1. 继续把 `downstream_reconciliation_diff` 落成 `knowledge` 或 `logs+knowledge` 联动 skill
2. 把 `region_mismatch_investigation` 落成更偏配置/资源排查的 skill
3. 在 `triage` 阶段输出 `skill_hints`
4. 统计各个 skill 的命中率、成功率、degraded 率
5. 给 skill 增加离线 harness 数据集，评估命中率和误判率
6. 给 `logs` skill 增加 service name / route / status code 的结构化提取
