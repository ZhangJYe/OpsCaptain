# logs_raw_review

## 用途

logs 域的默认回退 skill。  
当 query 不明显属于 payment timeout / auth failure / generic error，或者日志结构化提取不足时使用。

## 典型触发场景

- 广义日志查看
- 不带明显故障关键词的日志请求
- 只能拿到 raw output 的场景

## 行为特点

- 如果能提结构化 evidence，就提 evidence
- 如果不能，就至少保留 raw snippet

## 主要工具

- 日志 MCP 工具

## 输出预期

- `skill_mode=raw_review`
- 如果结构化失败，返回 `log-raw` evidence

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
