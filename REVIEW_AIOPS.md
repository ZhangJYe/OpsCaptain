# OpsCaptionAI (SuperBizAgent) AIOps 模块 Code Review 报告

**审查人**: AI Assistant  
**审查日期**: 2026-04-04  
**审查范围**: AIOps 全链路（前端入口 → Controller → Plan-Execute-Replan Agent → 工具层 → 外部服务）  
**代码版本**: 当前仓库最新状态（修复后）

---

## 一、思维链 (Chain of Thought)

### 1.1 审查起点：从用户截图反推

用户在前端点击 [AI Ops] 按钮后，页面显示：
- 步骤 1：LLM 生成了一个 7 步执行计划（合理）
- 步骤 2：尝试调用 `query_prometheus_alerts`，进入 reasoning → tool_calls → 但没有返回最终结果

**推断**：Agent 的 Plan（规划）能力是正常的，问题出在 Execute（执行）阶段。工具调用失败后 Agent 进入了重试死循环。

### 1.2 追踪调用链

```
前端 app.js fetchAIOps()
  → POST /api/ai_ops （空 body）
    → chat_v1_ai_ops.go:AIOps()
      → plan_execute_replan.BuildPlanAgent(ctx, hardcoded_query)
        → Planner（GLM-4.5-AIR）生成执行计划
        → Executor 按步骤调用工具：
            → GetLogMcpTool()        → mcp.log_url 为空 → 报错中断整个 Agent ❌
            → query_prometheus_alerts → http://127.0.0.1:9090 → 连接被拒 → 返回 Go error → Agent 反复重试 ❌
            → mysql_crud             → mysql.dsn 为空 → 连接失败 → 返回 Go error → Agent 反复重试 ❌
            → query_internal_docs    → Milvus ✅
            → get_current_time       → 无依赖 ✅
        → Replanner 检查结果 → 发现失败 → 重新规划 → 再次执行 → 继续失败 → 循环...
```

### 1.3 核心洞察

问题可以归为三层：

**第一层——工具层：错误返回方式不正确**

Go 的 tool handler 返回 `(string, error)`。当返回 `error != nil` 时，eino ADK 的 React Agent 会将此视为"工具执行异常"，触发重试逻辑。正确的做法是：工具内部捕获错误，将错误信息序列化为 JSON 字符串通过 `string` 返回（`error` 保持 `nil`），让 LLM 自行判断如何处理。

**第二层——编排层：缺少保护机制**

- MaxIterations=20（Plan 层）× MaxIterations=50（Execute 层）= 最坏 1000 次 LLM 调用
- 没有 context timeout，请求可以挂数十分钟
- `prints.Event()` 是 `eino-examples` 的调试函数，每个 event 都 dump 到 stdout

**第三层——配置层：外部服务地址硬编码或缺失**

- Prometheus: 硬编码 `127.0.0.1:9090`，不走配置
- MySQL: 读配置但 dsn 为空时直接 panic
- MCP Log: 为空时返回 error 中断整个流程

### 1.4 安全审查

`mysql_crud.go` 的 SQL 注入防护只检查了 `SELECT` 前缀：

```go
if !strings.HasPrefix(sqlUpper, "SELECT") {
    return "", fmt.Errorf(...)
}
db.Raw(input.SQL).Scan(&results)
```

攻击向量：`SELECT * FROM users; DROP TABLE users; --`（分号注入绕过前缀检查）

---

## 二、问题清单与修复

### [P0] 工具错误返回方式导致 Agent 死循环

| 字段 | 内容 |
|------|------|
| **文件** | `internal/ai/tools/query_metrics_alerts.go:80` |
| **原因** | `queryPrometheusAlerts` 失败时，tool handler 返回 `(json, err)` 而非 `(json, nil)`。eino React Agent 收到 `err != nil` 后触发重试逻辑，LLM 反复调用同一工具 |
| **影响** | Agent 在 Prometheus 不可用时进入死循环，最坏消耗 1000 次 API 调用 |
| **修复** | 将 `return string(jsonBytes), err` 改为 `return string(jsonBytes), nil`。错误信息通过 JSON 字符串传递给 LLM，由 LLM 自行决定跳过还是重试 |

同样问题也存在于 `mysql_crud.go`，已一并修复。

### [P0] Prometheus 地址硬编码

| 字段 | 内容 |
|------|------|
| **文件** | `internal/ai/tools/query_metrics_alerts.go:49` |
| **原因** | `baseURL` 硬编码为 `"http://127.0.0.1:9090"`，不走配置文件 |
| **影响** | 无法在不改代码的情况下连接实际的 Prometheus 实例 |
| **修复** | 改为从 `g.Cfg().Get(ctx, "prometheus.address")` 读取，`config.yaml` 新增 `prometheus.address` 配置项 |

### [P0] 无超时控制 + 过高迭代上限

| 字段 | 内容 |
|------|------|
| **文件** | `internal/ai/agent/plan_execute_replan/plan_execute_replan.go:29,45`; `executor.go:37` |
| **原因** | PlanExecute MaxIterations=20, Executor MaxIterations=50, 无 context deadline |
| **影响** | 工具持续失败时，请求可挂数十分钟，消耗大量 API 额度 |
| **修复** | 添加 `context.WithTimeout(ctx, 3*time.Minute)`; PlanExecute 降至 5 轮; Executor 降至 10 步 |

### [P1] `prints.Event()` 调试输出残留

| 字段 | 内容 |
|------|------|
| **文件** | `internal/ai/agent/plan_execute_replan/plan_execute_replan.go:45` |
| **原因** | 引用了 `eino-examples/adk/common/prints` 包的 `Event()` 函数，每个 Agent event 都 dump 到 stdout |
| **影响** | 生产环境日志被大量调试信息淹没；引入了对 examples 包的不必要依赖 |
| **修复** | 移除 `prints.Event(event)` 调用，改用 `g.Log().Debugf` 输出关键步骤信息；`go mod tidy` 移除 `eino-examples` 依赖 |

### [P1] SQL 注入风险

| 字段 | 内容 |
|------|------|
| **文件** | `internal/ai/tools/mysql_crud.go:47-58` |
| **原因** | 仅检查 SQL 是否以 `SELECT` 开头，`db.Raw(input.SQL)` 直接执行 LLM 生成的 SQL |
| **攻击向量** | `SELECT 1; DROP TABLE users; --` 通过分号注入执行任意 SQL |
| **修复** | 新增分号检查 `strings.Contains(input.SQL, ";")` 拒绝多语句；错误通过 JSON 返回而非 Go error |

### [P2] AIOps 入口不支持自定义分析指令

| 字段 | 内容 |
|------|------|
| **文件** | `api/chat/v1/chat.go`; `internal/controller/chat/chat_v1_ai_ops.go` |
| **原因** | `AIOpsReq` 是空结构体，分析 prompt 完全硬编码 |
| **影响** | 用户无法自定义分析范围（如只分析某个服务、某个时间段） |
| **修复** | `AIOpsReq` 新增 `Query string` 字段；为空时使用优化后的默认 prompt；非空时使用用户自定义指令 |

### [P2] 默认 prompt 未指示优雅降级

| 字段 | 内容 |
|------|------|
| **文件** | `internal/controller/chat/chat_v1_ai_ops.go:11-25` |
| **原因** | 原始 prompt 没有告诉 LLM "工具不可用时应跳过"，导致 LLM 反复重试失败的工具 |
| **修复** | 默认 prompt 新增 "如果某个工具不可用或返回错误，跳过该步骤并在报告中说明，不要反复重试" |

---

## 三、修复前后对比

### 3.1 行为对比

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| Prometheus 未部署 | Agent 死循环，请求挂起数十分钟 | Agent 跳过告警查询，~20秒内返回报告 |
| MySQL 未配置 | 工具抛 error → Agent 反复重试 | 工具返回 JSON 错误信息 → LLM 跳过 |
| MCP 日志未配置 | `BuildPlanAgent` 直接失败 | 日志工具不注册，其他工具正常使用 |
| 用户想自定义分析 | 不支持 | `POST /api/ai_ops {"query":"只分析 CPU 相关告警"}` |
| SQL 注入 `SELECT 1;DROP TABLE x` | 执行成功 | 被拒绝："multiple statements not allowed" |
| 请求超时 | 无限等待 | 3 分钟硬超时 |
| 最大迭代次数 | 20×50=1000 | 5×10=50 |

### 3.2 输出对比

**修复前（截图所示）**：
```
步骤 2: assistant: 我将首先调用query_prometheus_alerts工具获取所有活跃告警列表。
reasoning content: ... tool_calls: query_prometheus_alerts ...
（然后卡住，无后续输出）
```

**修复后**：
```json
{
  "result": "# 告警分析报告\n\n## 活跃告警清单\n由于 Prometheus 告警查询服务未配置...告警查询步骤已跳过。\n\n## 结论\n1. 告警查询服务当前不可用...\n2. 当前时间：2026-04-04 04:00:38\n3. 建议检查 Prometheus 服务配置...",
  "detail": ["assistant: {plan...}", "tool: {prometheus error...}", "assistant: {report...}"]
}
```

---

## 四、修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `internal/ai/tools/query_metrics_alerts.go` | Prometheus 地址改为配置读取；错误通过 JSON 返回而非 Go error |
| `internal/ai/tools/mysql_crud.go` | 新增分号注入防护；错误通过 JSON 返回；DB 连接失败优雅降级 |
| `internal/ai/agent/plan_execute_replan/plan_execute_replan.go` | 移除 `prints.Event`；添加 3 分钟超时；MaxIterations 20→5 |
| `internal/ai/agent/plan_execute_replan/executor.go` | MaxIterations 50→10 |
| `internal/controller/chat/chat_v1_ai_ops.go` | 支持自定义 query；优化默认 prompt 增加降级指令 |
| `api/chat/v1/chat.go` | `AIOpsReq` 新增 `Query` 字段 |
| `manifest/config/config.yaml` | 新增 `prometheus.address` 配置项 |
| `go.mod` / `go.sum` | 移除 `eino-examples` 依赖 |

---

## 五、验证结果

```
$ make ci
==> gofmt           ✅
==> go vet          ✅
==> go test -race   ✅ (所有包通过，无 data race)
==> go test -cover  ✅ (总覆盖率 36.8%)
==> go build        ✅
==> CI pipeline complete
```

端到端测试：
```bash
$ curl -s -m 180 -X POST http://localhost:8000/api/ai_ops \
  -H "Content-Type: application/json" -d '{}'
# 返回完整的告警分析报告，耗时 ~20 秒，正确处理了 Prometheus 不可用的情况
```

---

## 六、残留问题与建议

| 优先级 | 建议 |
|--------|------|
| **P1** | `mysql_crud` 的 SQL 防护仍然较弱（只做了分号检查），建议使用参数化查询或 SQL 白名单机制 |
| **P2** | 前端 AI Ops 按钮没有 loading 状态和取消功能，3 分钟超时期间用户无反馈 |
| **P2** | 建议在 docker-compose.yml 中添加一个带模拟数据的 Prometheus 实例，使 AIOps 开箱即可演示 |
| **P3** | `query_internal_docs` 在知识库为空时依赖 `safeRetriever` 兜底，建议在 Milvus SDK 升级后移除该 workaround |
| **P3** | AIOps 的 `detail` 字段包含完整的 tool_calls 和 reasoning 输出，数据量大且包含 API 内部信息，生产环境建议脱敏或只返回摘要 |
