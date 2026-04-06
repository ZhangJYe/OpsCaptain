# knowledge_sop_lookup

## 用途

这是 knowledge 域里的通用 SOP / checklist / 文档查询 skill。

## 典型触发词

- `sop`
- `runbook`
- `playbook`
- `doc`
- `docs`
- `知识库`
- `文档`

## Focus

会把检索重点拉向：

- SOP
- checklist
- operator steps

## 主要工具

- `query_internal_docs`

## 输出预期

- 一组知识库 evidence
- `knowledge_mode=sop_lookup`

## 失败降级

- 超时或解析失败时返回 degraded

## 实现位置

- `internal/ai/agent/skillspecialists/knowledge/agent.go`
