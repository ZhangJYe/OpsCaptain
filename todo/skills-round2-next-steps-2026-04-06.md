# Skills 第二轮之后的待办

## 这一轮已完成

- 新增 `knowledge_release_sop`
- 新增 `knowledge_rollback_runbook`
- 新增 `metrics_release_guard`
- 新增 `metrics_capacity_snapshot`
- 新增 `logs_payment_timeout_trace`
- 新增 `logs_auth_failure_trace`
- 给 knowledge / logs 增加 query focus
- 给 metrics 增加面向场景的 next actions
- 补对应测试

## 下一轮建议

1. 给 `triage` 增加 `skill_hints`
2. 在 runtime 里统计 `skill_name -> success/degraded/failed`
3. 给 logs skill 增加 service name 提取和结构化 filters
4. 给 knowledge skill 增加 skill-specific rerank
5. 给 metrics skill 增加更接近发布场景的指标基线比较
6. 为 skills 单独做离线 harness 和命中率统计
