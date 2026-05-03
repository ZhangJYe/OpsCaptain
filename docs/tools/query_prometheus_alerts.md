# 工具: query_prometheus_alerts

## 功能说明
查询 Prometheus 当前活跃告警列表，返回告警名称、描述、状态等信息。供 metrics specialist 使用，用于告警分诊、发布守卫、容量快照等场景。

## 输入参数
当前为无参调用（传入 `"{}"`），查询所有活跃告警。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| （无） | - | - | 查询所有活跃告警 |

## 输出格式
JSON 字符串，结构为 `PrometheusAlertsOutput`：

```json
{
  "success": true,
  "message": "",
  "error": "",
  "alerts": [
    {
      "alert_name": "HighCPUUsage",
      "description": "CPU usage exceeds 90% for 5 minutes",
      "state": "firing",
      "labels": {},
      "annotations": {}
    }
  ]
}
```

字段说明：
- `success`：查询是否成功
- `message`：查询失败时的错误消息
- `alerts`：活跃告警列表
  - `alert_name`：告警名称
  - `description`：告警描述
  - `state`：告警状态（firing/pending）

## 错误处理
- 网络不可达：返回 `success=false`，agent 降级处理（confidence 0.25）
- 超时：默认 5s，配置项 `multi_agent.metrics_query_timeout_ms`
- 响应解析失败：返回 degraded，附带原始输出（confidence 0.35）
- 无活跃告警：返回 `alerts=[]`，agent 输出"当前没有发现活跃告警"

## 配置项
| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `multi_agent.metrics_query_timeout_ms` | 5000 | 查询超时（毫秒） |

Prometheus 地址通过环境变量或配置文件注入。

## 相关代码
- 工具实现: `internal/ai/tools/query_metrics_alerts.go`
- 测试: `internal/ai/tools/query_metrics_alerts_test.go`
- 调用方: `internal/ai/agent/specialists/metrics/agent.go`
