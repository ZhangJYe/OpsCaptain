# OnCallAI 下一步 TODO 与执行记录

## 1. 结论

当前阶段最合理的下一步，确实是以下三项：

1. Runtime 实例复用，避免每次 AI Ops 请求都重复创建和注册整套 runtime
2. 增加 trace 查询接口，让持久化 ledger 真正可回读、可追溯
3. 开始规划 Phase 3：Chat 链路接入 Multi-Agent，但先做方案和切换策略，不直接硬切主链路

其中：

- 第 1、2 项本轮已执行
- 第 3 项本轮完成规划和 TODO 拆解，暂不直接切换生产主路径

---

## 2. 已执行项

## 2.1 Runtime 实例复用

### 问题

此前 `RunAIOpsMultiAgent` 每次请求都会：

- 创建新的 persistent runtime
- 重新注册 supervisor / triage / specialists / reporter

这会带来：

- 重复初始化开销
- 不必要的对象分配
- 同一进程内难以共享 runtime 状态

### 本轮实现

修改文件：

- `internal/ai/service/ai_ops_service.go`
- `internal/ai/service/ai_ops_service_test.go`

实现内容：

- 引入按 `dataDir` 维度缓存 runtime 的复用机制
- 新增：
  - `getOrCreateAIOpsRuntime`
  - `getOrCreateAIOpsRuntimeForDir`
  - `registerAIOpsAgents`
- runtime 现在只在首次请求时创建并注册

### 收益

- AI Ops 请求路径更稳定
- 避免重复 agent 注册和 runtime 构造
- 后续更容易扩展为共享 trace 查询和状态查询入口

### 测试

- `TestGetOrCreateAIOpsRuntimeReusesInstance`

---

## 2.2 Trace 查询接口

### 问题

此前 ledger 虽然已经把 trace event 写入磁盘，但业务层没有查询接口，持久化层只能“写”，很难说“真正可回溯”。

### 本轮实现

修改文件：

- `api/chat/v1/chat.go`
- `api/chat/chat.go`
- `internal/controller/chat/chat_v1_ai_ops.go`
- `internal/ai/service/ai_ops_service.go`
- `internal/ai/runtime/runtime.go`
- `internal/ai/runtime/file_store.go`
- `internal/ai/runtime/file_store_test.go`

实现内容：

- `AIOps` 响应新增 `trace_id`
- 新增 `GET /api/ai_ops_trace`
- 新增响应结构：
  - `trace_id`
  - `detail`
  - `events`
- `FileLedger.EventsByTrace` 改为支持从磁盘 jsonl 文件回读
- `Runtime` 新增 `TraceEvents`

### 收益

- 现在 AI Ops 请求可以把 `trace_id` 直接返回给前端或调试人员
- 后续可以基于 trace 查询接口做复盘、排障和回放
- ledger 的持久化能力开始真正被业务层消费

### 测试

- `TestFileLedgerAndArtifactStore` 已扩展验证 trace 回读
- `TestGetAIOpsTraceReadsPersistedTrace`

---

## 3. 已规划项：Phase 3 Chat 链路接入 Multi-Agent

## 3.1 为什么现在不直接切

虽然这是合理的下一阶段方向，但当前不建议直接把 `/chat` 主链路整体切到 Multi-Agent，原因是：

- 普通聊天链路和 AI Ops 的复杂度不同
- Chat 现在仍然依赖成熟的 `chat_pipeline`
- 若直接硬切，风险会显著高于收益

更稳妥的方式是：

- 先保留现有 chat pipeline
- 给 Chat 增加受控入口或灰度开关
- 先在部分场景走 Multi-Agent 路由

---

## 3.2 Phase 3 目标

目标不是“替换所有 Chat 能力”，而是：

- 为 Chat 增加可控的 Multi-Agent 路由能力
- 让普通问答、知识检索、复杂运维分析走不同处理路径
- 保持现有 chat pipeline 作为 fallback

---

## 3.3 建议拆分顺序

### Phase 3.1：Chat Triage 层

目标：

- 判断请求是普通问答、知识检索、还是复杂 AI Ops / 调查类任务

建议：

- 先复用现有 triage 模型
- 增加 chat 场景专用规则

### Phase 3.2：双路径执行

目标：

- 简单问题仍走 `chat_pipeline`
- 复杂问题走 `supervisor + specialists`

建议：

- 通过 feature flag 或配置开关控制
- 保留快速回退能力

### Phase 3.3：Chat 结果统一包装

目标：

- 让前端不需要区分是单 Agent 还是 Multi-Agent 结果

建议：

- 统一 response shape
- 增加 `mode` / `trace_id` / `detail` 等字段

### Phase 3.4：Chat replay / eval

目标：

- 防止引入 Multi-Agent 后普通聊天质量下降

建议：

- 建立 chat replay case：
  - 普通 FAQ
  - 纯知识库问答
  - 复杂分析请求
  - 歧义请求

---

## 3.4 Phase 3 TODO 清单

| ID | Action | Priority | Exit Criteria |
| --- | --- | --- | --- |
| P3-01 | 设计 Chat triage 规则 | P1 | 能区分普通问答/知识问答/复杂分析 |
| P3-02 | 给 Chat 增加灰度开关 | P1 | 可按配置切换 old/new path |
| P3-03 | 定义统一 chat response shape | P1 | 前端无需分别适配 |
| P3-04 | 建立 Chat replay 基线 | P1 | 至少 5 个 case 可执行 |
| P3-05 | 在非默认路径接入 Multi-Agent Chat | P2 | 不影响现有主链路 |
| P3-06 | 评估是否替换默认 Chat 主路径 | P2 | replay + 线上验证通过 |

---

## 4. 当前推荐顺序

建议你接下来按这个顺序推进：

1. 先消费新的 trace 查询接口，验证回溯链路是否满足复盘需求
2. 再补 trace / detail 的前端展示或内部调试页
3. 然后再进入 Chat Phase 3 的 triage 和灰度接入

一句话总结：

> 现在不是“直接把 Chat 全量切到 Multi-Agent”，而是“先把 AI Ops 的 runtime 与 trace 基础设施做扎实，再为 Chat 接入铺一条低风险迁移路径”。

