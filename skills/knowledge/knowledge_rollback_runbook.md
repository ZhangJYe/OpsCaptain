# knowledge_rollback_runbook

## 用途

用于查询回滚、恢复、止损、恢复验证相关 runbook。

## 典型触发词

- `rollback`
- `revert`
- `recover`
- `restore`
- `回滚`
- `恢复`
- `止损`

## Focus

会把检索重点拉向：

- rollback triggers
- mitigation actions
- recovery steps
- validation checklist

## 主要工具

- `query_internal_docs`

## 输出预期

- 命中的 runbook evidence
- `knowledge_mode=rollback_runbook`
- `knowledge_query=原始问题 + focus`

## 设计备注

这个 skill 在 registry 里要排在 `knowledge_release_sop` 前面，否则回滚问题容易先被 release 关键词抢走。

## 实现位置

- `internal/ai/agent/skillspecialists/knowledge/agent.go`
