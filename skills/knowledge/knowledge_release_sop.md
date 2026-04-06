# knowledge_release_sop

## 用途

用于查询发布、部署、rollout、上线相关的 SOP / runbook。

## 典型触发词

- `release`
- `deploy`
- `deployment`
- `rollout`
- `上线`
- `发版`
- `部署`

## Focus

会把检索重点拉向：

- pre-check
- post-check
- verification
- rollback

## 主要工具

- `query_internal_docs`

## 输出预期

- 命中的知识库文档 evidence
- `knowledge_mode=release_sop`
- `knowledge_query=原始问题 + focus`

## 失败降级

- RAG 超时：返回 degraded
- 文档 payload 无法解析：返回 degraded

## 实现位置

- `internal/ai/agent/skillspecialists/knowledge/agent.go`
