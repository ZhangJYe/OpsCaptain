# logs_auth_failure_trace

## 用途

用于排查登录失败、token 过期、jwt 失效、权限拒绝等问题。

## 典型触发词

- `login`
- `auth`
- `authentication`
- `authorization`
- `token`
- `jwt`
- `unauthorized`
- `forbidden`
- `登录`
- `鉴权`

## Focus

会把日志查询重点拉向：

- login
- token
- jwt
- unauthorized
- forbidden
- permission denied
- auth middleware

## 主要工具

- 日志 MCP 工具

## 输出预期

- 结构化日志 evidence
- `skill_mode=auth_failure_trace`
- `log_focus=...jwt/auth middleware...`

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
