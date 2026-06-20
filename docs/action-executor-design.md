# 自动处置 / Runbook 执行设计方案

> 目标：让 AI 在人工确认后，能真正执行运维操作（重启、扩缩容、查询状态等）。
> 安全约束：所有执行必须经过 Approval Gate + 二次确认，结果可追溯。

---

## 1. 架构

```
AI Agent 识别到需要执行操作
    │
    ▼
ActionRegistry 查找匹配的动作定义
    │
    ▼
ActionExecutor 验证参数
    │
    ▼
ApprovalGate 检查是否需要审批
    │
    ├── 需要审批 → 进入审批队列 → 人工确认
    │
    └── 自动通过 → 直接执行
    │
    ▼
执行适配器（HTTP / K8s / SSH）
    │
    ▼
记录执行结果到 ExecutionLog
```

## 2. 数据模型

```go
// ActionDefinition 定义一个可执行的动作
type ActionDefinition struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Category    string            `json:"category"`    // restart, scale, query, rollback
    RiskLevel   string            `json:"risk_level"`  // low, medium, high
    Parameters  []ActionParam     `json:"parameters"`
    Executor    string            `json:"executor"`    // http, k8s, ssh
    Config      map[string]string `json:"config"`      // 执行器配置
}

type ActionParam struct {
    Name        string `json:"name"`
    Type        string `json:"type"`    // string, int, bool
    Required    bool   `json:"required"`
    Description string `json:"description"`
    Default     string `json:"default,omitempty"`
}

// ActionResult 执行结果
type ActionResult struct {
    Success    bool              `json:"success"`
    ActionID   string            `json:"action_id"`
    Output     string            `json:"output"`
    Error      string            `json:"error,omitempty"`
    ExecutedAt int64             `json:"executed_at"`
    Duration   int64             `json:"duration_ms"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}
```

## 3. 执行适配器

| 适配器 | 用途 | 实现 |
|--------|------|------|
| **HTTP** | 调用内部 API / Webhook | net/http |
| **K8s** | 重启/扩缩容/回滚 | K8s REST API (HTTP) |
| **SSH** | 远程命令执行 | crypto/ssh |

MVP 阶段只实现 **HTTP 适配器**，K8s 和 SSH 后续演进。

## 4. 预定义动作

```yaml
actions:
  - id: restart_service
    name: 重启服务
    category: restart
    risk_level: medium
    executor: http
    config:
      method: POST
      url: "${K8S_API}/apis/apps/v1/namespaces/{namespace}/deployments/{service}/restart"
      
  - id: query_service_status
    name: 查询服务状态
    category: query
    risk_level: low
    executor: http
    config:
      method: GET
      url: "${K8S_API}/api/v1/namespaces/{namespace}/pods?labelSelector=app={service}"
      
  - id: scale_deployment
    name: 扩缩容
    category: scale
    risk_level: high
    executor: http
    config:
      method: PATCH
      url: "${K8S_API}/apis/apps/v1/namespaces/{namespace}/deployments/{service}"
```

## 5. 分阶段实现

### Stage 1: ActionExecutor 框架 + HTTP 适配器
- ActionDefinition / ActionResult 数据结构
- ActionRegistry 动作注册表
- HTTP 执行适配器
- 配置加载

### Stage 2: AI 工具 `execute_action`
- 结构化输入
- Approval Gate 集成
- 执行结果返回

### Stage 3: 预定义动作 + 测试

### Stage 4: 全量验证
