# knowledge_service_error_code_lookup

## 用途

用于根据服务错误码快速检索“错误码含义 + 常见原因 + 首轮排查建议”。

## 典型触发词

- `error code`
- `errno`
- `status code`
- `错误码`
- `错误代码`
- `返回码`

## Focus

命中后会把知识检索重点拉向：

- exact error code meaning
- common causes
- affected dependency
- first troubleshooting checks

## 主要工具

- `query_internal_docs`

## 输出预期

- 命中的知识库文档 evidence
- `knowledge_mode=service_error_code_lookup`
- `knowledge_query=原问题 + error-code focus`
- `extracted_error_codes=[...]`
- 推荐的 `next_actions`

## 适合这个 skill 的原因

- 错误码问题天然是结构化知识检索
- 适合做精确 matcher，而不是只靠模糊关键词
- 这类 skill 很适合面试里解释“为什么要给 knowledge specialist 加 skill”

## 实现位置

- `internal/ai/agent/skillspecialists/knowledge/agent.go`
- `internal/ai/agent/skillspecialists/knowledge/agent_test.go`
