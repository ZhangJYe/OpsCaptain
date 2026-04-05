# OnCallAI AI Ops Baseline Round 1

日期：`2026-04-05`  
范围：`AI Ops Multi-Agent 当前基线`  
目的：为后续优化后的回归对比提供统一基准

---

## 1. 结论摘要

当前基线可以分成两部分看：

1. **结构与单元层面是通过的**
2. **真实 HTTP 业务路径的时延表现明显不达标**

更具体地说：

- `supervisor / service / controller / build` 的自动化基线都通过
- 审批拒绝路径很快，语义正确
- 知识单域请求和全域分析请求在当前本地环境下都超过 `40s`
- 全域分析请求在 `60s` 客户端超时窗口内仍未返回
- 服务日志显示：
  - `mcp.log_url is not configured, log query tool will be disabled`
  - `querying Prometheus active alerts`
- 当前真实链路的第一优先风险更像是 `tool timeout / 外部依赖时延治理`

---

## 2. 执行环境说明

本轮测试基于当前本地仓库执行，命令均使用：

```bash
env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp ...
```

真实 HTTP 测试基于本地运行服务：

```bash
go run .
```

已观察到的本地依赖状态：

- `mcp.log_url` 未配置，日志工具会直接降级
- Prometheus 查询会被触发
- 从服务日志看，业务请求会持续执行到客户端超时之后

---

## 3. 自动化基线

## 3.1 命令

```bash
/usr/bin/time -p env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test -count=1 -v ./internal/ai/agent/supervisor
/usr/bin/time -p env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test -count=1 -v ./internal/ai/service
/usr/bin/time -p env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go test -count=1 -v ./internal/controller/chat
/usr/bin/time -p env GOCACHE=/tmp/gocache GOTMPDIR=/tmp/go-tmp go build ./...
```

## 3.2 结果

| Item | Result | go reported | real(s) | 备注 |
| --- | --- | --- | --- | --- |
| `./internal/ai/agent/supervisor` | PASS | `ok ... 2.101s` | `6.67` | replay 基线通过 |
| `./internal/ai/service` | PASS | `ok ... 2.845s` | `7.46` | service 语义通过 |
| `./internal/controller/chat` | PASS | `ok ... 3.437s` | `8.05` | controller 入口通过 |
| `go build ./...` | PASS | N/A | `5.70` | 全仓构建通过 |

## 3.3 自动化层结论

- 当前 Multi-Agent 编排基线是稳定的
- `/ai_ops` 入口语义是稳定的
- 审批拒绝路径已有测试覆盖
- 当前问题不在“跑不起来”，而在“真实依赖链路性能和降级质量”

---

## 4. HTTP 基线

## 4.1 测试命令

### 审批拒绝

```bash
curl -s -m 10 -o /tmp/aiops_approval.json \
  -w 'http_code=%{http_code} time_total=%{time_total} size=%{size_download}\n' \
  -X POST http://127.0.0.1:8000/api/ai_ops \
  -H 'Content-Type: application/json' \
  -d '{"query":"请删除生产环境中的历史记录"}'
```

### 知识单域

```bash
curl -s -m 25 -o /tmp/aiops_kb.json \
  -w 'http_code=%{http_code} time_total=%{time_total} size=%{size_download}\n' \
  -X POST http://127.0.0.1:8000/api/ai_ops \
  -H 'Content-Type: application/json' \
  -d '{"query":"请查询知识库中的 SOP 文档"}'
```

### 全域分析

```bash
curl -s -m 25 -o /tmp/aiops_full.json \
  -w 'http_code=%{http_code} time_total=%{time_total} size=%{size_download}\n' \
  -X POST http://127.0.0.1:8000/api/ai_ops \
  -H 'Content-Type: application/json' \
  -d '{"query":"请分析当前 Prometheus 告警并结合日志排查"}'
```

补充了一轮更长窗口：

```bash
curl -s -m 40 ...
curl -s -m 60 ...
```

## 4.2 结果

| Case | Query | Timeout Window | HTTP | Time(s) | Result |
| --- | --- | --- | --- | --- | --- |
| `AIOPS-B01` | 删除生产历史记录 | `10s` | `200` | `0.003682` | 快速返回审批拒绝 |
| `AIOPS-B02` | 查询知识库 SOP | `25s` | `000` | `25.006275` | 客户端超时 |
| `AIOPS-B03` | 分析 Prometheus 告警并结合日志排查 | `25s` | `000` | `25.002555` | 客户端超时 |
| `AIOPS-B04` | 查询知识库 SOP | `40s` | `000` | `40.001042` | 客户端超时 |
| `AIOPS-B05` | 分析 Prometheus 告警并结合日志排查 | `40s` | `000` | `40.005750` | 客户端超时 |
| `AIOPS-B06` | 分析 Prometheus 告警并结合日志排查 | `60s` | `000` | `60.005853` | 客户端超时 |

## 4.3 审批拒绝响应体

```json
{"message":"OK","data":{"trace_id":"","result":"检测到高风险动作，当前未获得审批。","detail":["检测到高风险动作，当前未获得审批。"]}}
```

## 4.4 服务日志关键信号

已观察到以下日志：

```text
mcp.log_url is not configured, log query tool will be disabled
querying Prometheus active alerts
```

并且在客户端超时后，服务端仍继续运行，随后出现长期记忆写入日志，这说明：

- 请求不是立即失败
- 请求在服务端会执行到超时窗口之后
- 当前慢点更像是外部依赖或超时治理问题，而不是 controller/service 直接报错

---

## 5. 当前基线判断

## 5.1 已达成

- AI Ops Multi-Agent 主链路已接入
- replay 基线通过
- trace 机制存在
- Approval Gate 行为正确
- controller/service 基础语义正确

## 5.2 未达成

- 真实业务路径响应时延不可接受
- 至少知识单域和全域分析路径已超过 `40s`
- 全域分析路径在 `60s` 窗口内仍未完成
- 当前还不能把“真实效果稳定”作为已验证结论

## 5.3 当前首要归因

本轮更偏向：

- `tool`

其次可能涉及：

- `rag`

当前不优先怀疑：

- `routing`
- `controller`

---

## 6. 优化前后的对比模板

后续每次优化后，都建议按下面这张表补第二列。

| Metric | Round 1 Baseline | After Optimization | Delta | Notes |
| --- | --- | --- | --- | --- |
| Supervisor replay | PASS |  |  |  |
| Service tests | PASS |  |  |  |
| Controller tests | PASS |  |  |  |
| `go build ./...` | PASS / `5.70s` |  |  |  |
| Approval deny latency | `0.003682s` |  |  |  |
| KB-only latency (25s window) | timeout |  |  |  |
| KB-only latency (40s window) | timeout |  |  |  |
| Full analysis latency (25s window) | timeout |  |  |  |
| Full analysis latency (40s window) | timeout |  |  |  |
| Full analysis latency (60s window) | timeout |  |  |  |
| Approval semantics | correct |  |  |  |
| Trace availability | partial validated |  |  |  |
| Primary failure type | `tool` |  |  |  |

---

## 7. 当前最值得优化的点

按这轮基线，建议优先顺序如下：

1. 明确 `Prometheus` 查询的真实耗时和超时行为
2. 补强 tool 层 timeout / degraded result / observability
3. 明确 `knowledge` 域在当前本地环境中的实际阻塞点
4. 在完成一轮工具侧治理之后，再重新评估：
   - 先做 Context
   - 还是先做 RAG

---

## 8. 下一轮建议执行项

1. 给 `metrics` 和 `knowledge` 路径补更明确的 timeout 观测
2. 为真实 HTTP replay 建最小脚本化入口
3. 继续补 8 到 12 条固定 case
4. 优化后用同一套命令重跑本文件中的全部指标

---

## 9. 修复后 Spot Check（同日复测）

在补上 `metrics / knowledge` timeout 与 degraded 语义之后，按同一批 HTTP case 复测。

### 9.1 修复内容

- `metrics` specialist 增加显式 query timeout
- `Prometheus` HTTP 请求改为真正绑定 `context`
- `knowledge` specialist 增加显式 query timeout
- `query_internal_docs` tool 增加超时上下文
- `knowledge` 超时/错误改为 `degraded`，不再把整个 specialist 升成 failed/err

### 9.2 复测结果

| Case | Before | After | Improvement |
| --- | --- | --- | --- |
| Approval deny | `200 / 0.003682s` | `200 / 0.002764s` | 基本持平 |
| KB-only | `25s timeout` / `40s timeout` | `200 / 5.011964s` | 明显改善 |
| Full analysis | `25s timeout` / `40s timeout` / `60s timeout` | `200 / 5.040247s` | 明显改善 |

### 9.3 修复后代表性响应

#### KB-only

- `trace_id` 已返回
- `result` 中明确写出：`知识库检索超时，已跳过该步骤。`
- `detail` 可完整反映 `supervisor -> triage -> knowledge -> reporter` 链路

#### Full analysis

- `trace_id` 已返回
- 三个域都以可解释方式收敛：
  - `logs`: 未配置，降级
  - `metrics`: Prometheus 不可用，降级
  - `knowledge`: 超时，降级
- `trace` 可查询，且能看到 `knowledge` 在约 `5s` 左右完成降级

### 9.4 修复后结论

这次修复说明：

- 当前 AI Ops 主链路的**最大问题确实在外部依赖超时治理**
- 一旦把 `metrics / knowledge` timeout 收口，真实 HTTP 基线会立刻改善
- 当前下一步优先级应仍然放在：
  1. 补更细的 tool observability
  2. 跑更多真实 replay
  3. 再决定先做 Context 还是先做 RAG

---

## 10. Review 收口后的第二轮复测（同日）

本轮在前一轮 timeout 修复的基础上，继续收口了 3 个 review 问题：

- `triage` 不再基于 memory-enriched query 路由
- `/ai_ops` `detail` 与 `/ai_ops_trace` 不再重复塞入完整报告正文
- `query_internal_docs` 增加 retriever 复用与短 TTL 的初始化失败缓存

### 10.1 代码层变化

- `RunAIOpsMultiAgent` 改为：
  - 根任务 `Goal` 保持原始用户 query
  - memory 以 `memory_context` 和 `memory_refs` 形式单独传递
- `supervisor` 改为：
  - `triage` 子任务只看 raw query
  - specialist 子任务按需拼接 memory context
- `runtime` 改为：
  - `task_completed` 事件只记录短消息
  - 长报告改成 `详细摘要已折叠`
- `query_internal_docs` 改为：
  - 复用已有 retriever
  - 对最近的初始化失败做短 TTL 缓存，避免不可用依赖带来的重复重建

### 10.2 自动化验证

本轮新增/扩展的回归测试包括：

- `TestRunAIOpsMultiAgentKeepsRawQueryForRouting`
- `TestSupervisorRoutesOnRawQueryButPassesMemoryToSpecialists`
- `TestRuntimeDetailMessagesOmitVerboseSummaryBodies`
- `TestQueryInternalDocsToolReusesRetriever`
- `TestQueryInternalDocsToolCachesRecentInitFailures`

并已通过：

- `go test ./internal/ai/service ./internal/ai/runtime ./internal/ai/tools ./internal/ai/agent/supervisor`
- `go test ./...`
- `go build ./...`

### 10.3 HTTP 复测结果（临时端口 `:8002`）

| Case | Result | Latency | 结论 |
| --- | --- | --- | --- |
| KB-only cold | `200` | `5.032488s` | 可解释降级，仍被 knowledge timeout 卡住 |
| KB-only warm | `200` | `5.012195s` | 与冷启动接近，当前环境瓶颈已转移到实际检索超时 |
| Full analysis | `200` | `5.010530s` | routing 与 detail 正常，整体仍受 knowledge timeout 支配 |

### 10.4 本轮最重要的结论

这轮结果很关键，因为它帮助我们把问题边界进一步收窄了：

- `routing` 纯净性问题已经收口
- `detail/trace` 体积问题已经收口
- 当前 AI Ops 主链路仍然稳定在约 `5s`
- 这 `5s` 在当前环境中主要来自 `knowledge` 域的**实际检索超时**
- 也就是说，下一步最值得做的不是继续改 `triage` 或 `reporter`，而是：
  1. 给 `query_internal_docs` / Milvus 检索增加更细的 tool-level observability
  2. 明确是 `retriever init` 慢，还是 `Retrieve` 本身慢
  3. 再决定是优化 RAG 热路径，还是在本地环境直接做更激进的 fail-fast

### 10.5 响应语义检查

本轮响应还验证了两个行为已经符合预期：

- `detail` 中不再重复出现完整 Markdown 报告正文
- `reporter` / `supervisor` 完成事件现在会折叠成短消息：
  - `任务已完成。 详细摘要已折叠。`
