# knowledge_incident_guidance

## 用途

knowledge 域的默认回退 skill。  
当 query 不明显属于 release / rollback / SOP，但仍然需要知识库支持时使用。

## 典型触发场景

- 故障分析
- 故障缓解
- 经验检索
- 泛化排障问题

## Focus

会把检索重点拉向：

- troubleshooting guidance
- mitigation steps
- related incident runbooks

## 主要工具

- `query_internal_docs`

## 输出预期

- 一组通用知识 evidence
- `knowledge_mode=incident_guidance`

## 实现位置

- `internal/ai/agent/skillspecialists/knowledge/agent.go`
