# MCP 工具调用链路 Review

> 日期：2026-05-03
> 范围：MCP 工具从发现、注册到调用的完整链路
> 涉及文件：query_log.go, tiered_tools.go, flow.go, tool_wrapper.go, eino_callback.go, common.go, query_metrics_alerts.go

---

## 1. 整体架构

```
用户 query
  → ProgressiveDisclosure 选择工具（按 domain 匹配）
  → eino ReAct Agent（LLM 决策调用哪个工具）
  → ToolWrapper.InvokableRun（before/after 拦截）
  → pooledToolWrapper → pooledClient.CallTool（连接池 + 超时 + 重连）
  → MCP Server / Prometheus / Milvus / MySQL 执行
  → after hook 截断 → LLM 处理结果
```

## 2. 工具清单与线上状态

| 工具 | 连接的基础设施 | config.prod.yaml | .env.production | 实际状态 |
|------|---------------|-----------------|-----------------|---------|
| get_current_time | time.Now() | — | — | ✅ 可用 |
| query_internal_docs | Milvus + SiliconFlow Embedding | `${MILVUS_ADDRESS}` | `MILVUS_ADDRESS=host.docker.internal:19530` | ⚠️ 待 CD 部署 |
| query_prometheus_alerts | Prometheus HTTP API | `${PROMETHEUS_ADDRESS}` | `PROMETHEUS_ADDRESS=http://prometheus:9090` | ⚠️ 待 CD 部署 |
| MCP 日志工具 | k3s pod logs via SSE MCP | `${MCP_LOG_URL}` | `MCP_LOG_URL=http://172.17.0.1:18088/sse` | ⚠️ 待 CD 部署 |
| mysql_crud | MySQL via GORM | `${MYSQL_DSN}` | 空 | ❌ 未部署 MySQL |

### 线上已运行的服务（docker ps 确认）

- prometheus (v2.54.1) — 10+ scrape target，含 freeexchanged 全链路
- milvus-standalone — 运行 3 周，healthy
- redis, rabbitmq, jaeger — 均正常
- opscaptain-log-mcp — systemd 服务，SSE 端点 200
- backend — healthy

## 3. 已修复的问题清单

| # | 问题 | 严重度 | 修复方案 | 状态 |
|---|------|--------|----------|------|
| 1 | `result.Content` 用 `%v` 序列化 | 🔴 | `json.Marshal(result)` | ✅ |
| 2 | reconnect 持锁 sleep 阻塞 CallTool | 🔴 | 双锁设计：mu 防并发重连，rw 保护 cli 读写 | ✅ |
| 3 | 业务超时误触发 reconnect | 🟡 | 排除 context.DeadlineExceeded，精确匹配连接错误 | ✅ |
| 4 | 工具发现结果未缓存 | 🟡 | URL 级缓存 + 错误 TTL 5 分钟 | ✅ |
| 5 | 每次调用查 Info 获取工具名 | 🟢 | 构造时缓存到 toolName 字段 | ✅ |
| 6 | 多 goroutine 并发 reconnect | 🟡 | mu Mutex 保护 | ✅ |
| 7 | 初始连接失败永久缓存 | 🟡 | 错误缓存 5 分钟 TTL 自动重试 | ✅ |
| 8 | pc.cli 并发读写无保护 | 🟡 | RWMutex 读写锁 | ✅ |
| 9 | GoFrame `${}` 环境变量替换失败 | 🔴 | os.Getenv fallback（query_log, query_metrics, common） | ✅ |
| 10 | before hook 未启用 | 🟡 | ValidateBeforeToolCall() 校验 JSON + 非空 | ✅ |

## 4. 当前评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 正确性 | ★★★★★ | MCP 协议完整，序列化正确，并发安全 |
| 可靠性 | ★★★★☆ | 超时 + 重连 + 重试 + 错误 TTL + 环境变量 fallback |
| 性能 | ★★★★☆ | 连接池 + 工具发现缓存 + ProgressiveDisclosure |
| 安全性 | ★★★★☆ | before hook 校验参数，after hook 截断结果 |
| 可观测性 | ★★★★☆ | 事件发射、耗时统计、结果摘要、重连日志 |
| 可维护性 | ★★★★☆ | 分层清晰，职责明确 |

## 5. 部署状态

### 已完成
- [x] 代码 push 到 origin/main (commit 9519942)
- [x] 服务器 .env.production 写入 MCP_LOG_URL 和 MILVUS_ADDRESS
- [x] CD 流水线已触发（GitHub Actions 自动构建）

### 待完成（明天）
- [ ] 确认 CD 部署成功（GitHub Actions 页面查看）
- [ ] 验证 MCP 日志工具可用：问 AI "查一下最近的日志"
- [ ] 验证 Prometheus 工具可用：问 AI "有什么告警"
- [ ] 验证 Milvus RAG 可用：问 AI 一个知识库问题
- [ ] **关键**：CD 会覆盖 .env.production，需要在 GitHub Secrets 的 `PROD_ENV_FILE` 中加入：
  ```
  MCP_LOG_URL=http://172.17.0.1:18088/sse
  MILVUS_ADDRESS=host.docker.internal:19530
  ```
  然后重新触发 CD

## 6. 剩余可优化项（非阻塞）

1. **连接池 metrics**：暴露连接数、重连次数、调用耗时等 Prometheus 指标
2. **Graceful shutdown**：agent 退出时关闭所有 MCP 连接
3. **ProgressiveDisclosure 多 domain**：MCP 工具绑定多个 domain，支持跨域关联
4. **重连后健康检查**：重连成功后 Ping() 确认 MCP Server 真正可用
5. **MySQL 部署**：需要额外部署 MySQL 实例并配置 DSN
