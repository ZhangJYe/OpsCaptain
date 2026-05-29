# 工具: query_log

## 功能说明
通过 MCP（Model Context Protocol）日志工具查询应用日志，支持结构化日志检索和原始日志抽取。供 logs specialist 使用，用于提取错误、超时、panic、依赖失败等日志证据。

## 输入参数
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| query | string | 是 | 日志查询文本（通常为任务目标） |
| limit | int | 否 | 最大返回条数（默认 3） |

输入示例：
```json
{"query": "payment service timeout error", "limit": 3}
```

## 输出格式
MCP 工具返回格式不固定，agent 会尝试从以下结构中提取日志片段：

1. JSON 结构化响应（优先）：
```json
{
  "logs": [
    {
      "timestamp": "2026-04-21T10:30:00Z",
      "level": "ERROR",
      "service": "payment-service",
      "message": "connection timeout after 30s"
    }
  ]
}
```

支持的嵌套 key：`logs`、`items`、`results`、`data`、`entries`、`records`
支持的字段 key：`message`/`msg`/`content`/`text`、`timestamp`/`time`/`ts`、`level`/`severity`、`service`/`app`/`source`/`host`

2. 原始文本（fallback）：
按行拆分，每行截取前 200 字符作为 snippet。

## 错误处理
- MCP 工具初始化失败：返回 degraded（confidence 0.28）
- 未配置日志工具：返回 degraded，提示"日志查询能力未配置"
- 单个工具失败：记录错误，继续尝试其他工具
- 所有工具失败：返回 degraded，附带工具错误列表
- 仅获取 raw output：返回 degraded（confidence 0.42），建议检查 MCP 工具的结构化返回格式
- 超时：默认 3s，配置项 `multi_agent.log_query_timeout_ms`

## 配置项
| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `multi_agent.log_query_timeout_ms` | 3000 | 单次查询超时（毫秒） |
| `multi_agent.log_evidence_limit` | 3 | 最大证据条数 |

MCP 日志工具通过 `tools.GetLogMcpTool` 动态发现。

## 相关代码
- 工具实现: `internal/ai/tools/query_log.go`
- 测试: `internal/ai/tools/query_log_test.go`
- 调用方: `internal/ai/agent/specialists/logs/agent.go`
