# Skills 下一轮迭代待办

## 已完成

- 抽通用 `Skill` 接口和 `Registry`
- 新增 `skillspecialists/knowledge`
- 新增 `skillspecialists/metrics`
- 新增 `skillspecialists/logs`
- 把 `supervisor` 和 `ai_ops_service` 切到新 specialist
- 补 skill 选择和回退相关测试

## 下一轮建议继续做

1. 给 `triage` 增加 skill hints，把 domain 路由和 skill 选择连接起来
2. 在 runtime 里统计 `skill_name -> success/degraded/failed` 分布
3. 为 skills 增加离线评测 harness，统计 skill 命中率
4. 给 knowledge / metrics / logs 再各补 2 到 3 个更细粒度 skill
5. 考虑做统一的 capability 到 skill 的查询接口
