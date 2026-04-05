# Harness Engineering 落地说明与清单

## 1. 文档目的

本文档用于整理以下内容：

- 什么是 Harness Engineering
- 它为什么在 AI Agent 场景里变得重要
- 它和 Multi-Agent 的关系
- 它是否适合当前 OnCallAI 项目
- 如果要在当前项目中落地，应该如何分阶段实施
- 一份按 `P0 / P1 / P2` 优先级排序的落地清单

本文档偏“工程方法论 + 实施建议”，不是代码设计稿的替代品。

---

## 2. 什么是 Harness Engineering

### 2.1 定义

Harness Engineering 不是某个单独的 SDK、平台或 Agent 框架，而是一种面向 Agent 系统的工程方法。

它解决的问题不是“如何让模型输出更聪明”，而是：

- 如何让 Agent 在真实工程环境里更可控
- 如何让 Agent 的行为更稳定、更可验证
- 如何让 Agent 在出错后可追踪、可修复、可持续改进

一句话概括：

> Harness Engineering = 给 Agent 系统加一层工程化“外壳”，让非确定性的模型行为被更确定的系统约束住。

### 2.2 它通常包含什么

典型的 Harness Engineering 由以下几类要素组成：

- 上下文组织
  - 不把所有信息都塞进 prompt
  - 而是按需组织、结构化注入
- 约束与边界
  - 工具白名单
  - 权限边界
  - 预算限制
  - 迭代上限
- 验证机制
  - schema 校验
  - 单测、集成测试
  - 回放评测
  - 输出检查
- 观测与追踪
  - trace_id
  - task_id
  - tool call log
  - artifact
- 纠偏机制
  - 失败后补文档
  - 失败后补规则
  - 失败后补测试

### 2.3 它和 Multi-Agent 的关系

Harness Engineering 不是 Multi-Agent 的同义词。

关系如下：

- Multi-Agent 解决的是“如何拆角色、做协作”
- Harness Engineering 解决的是“如何让这些 Agent 的协作可控、可调试、可验证”

所以：

- 没有 Harness 的 Multi-Agent，容易失控
- 只有 Harness 没有合适的 Agent 拆分，也没有业务价值

最佳实践通常是：

- 用 Multi-Agent 做任务分工
- 用 Harness Engineering 做系统约束

---

## 3. 为什么它最近很热

最近 Agent 圈子越来越关注 Harness Engineering，本质上是因为大家都遇到了同一个问题：

- demo 很容易
- 真正能持续运行的 Agent 系统很难

尤其在以下场景里，单纯靠 prompt 很快就会撞墙：

- 工具越来越多
- 上下文越来越长
- 任务流程越来越复杂
- 不同能力之间存在权限边界
- 结果必须可审计
- 失败必须可追踪

当系统从“单轮问答”进入“多工具、多阶段、多角色协作”后，工程壳层的价值就会快速放大。

---

## 4. 当前项目适不适合引入 Harness Engineering

结论：适合，而且必要性不低。

### 4.1 为什么适合

当前项目已经具备以下特征：

- 已经不是纯聊天
- 已经有工具调用
- 已经有 RAG
- 已经有 AI Ops 分析链路
- 已经在讨论 Multi-Agent 演进

这意味着系统很快会遇到：

- 路由复杂度上升
- 工具权限边界问题
- 输出验证问题
- 失败定位困难
- 上下文污染和职责混杂问题

这些都是 Harness Engineering 典型要解决的问题。

### 4.2 为什么不是“多此一举”

如果项目只是：

- 一个简单聊天机器人
- 单模型问答
- 没有工具
- 没有工作流

那 Harness Engineering 确实可能偏重。

但你现在的项目已经不是这个阶段了。你已经在做：

- 日志分析
- 告警分析
- 文档检索
- 多轮上下文
- 准备引入 Multi-Agent

所以引入 Harness，不是为了炫技，而是为了控制复杂度。

---

## 5. 对当前项目，Harness 应该落在哪里

不建议“一刀切”给整个项目都套上重型 Harness。

建议聚焦在最复杂、最需要治理的部分。

### 5.1 优先落地范围

- AI Ops 链路
- Multi-Agent Runtime
- 工具调用链
- Memory 注入与提取链
- 结果汇总与输出验证

### 5.2 暂时不必重度落地的范围

- 普通简单聊天
- 前端轻交互逻辑
- 不涉及工具调用的纯展示层

也就是说，应该把 Harness Engineering 当作“复杂智能工作流的工程骨架”，而不是全项目统一重装。

---

## 6. 当前项目最需要的 Harness 组件

### 6.1 协议层 Harness

你项目最先要补的是统一协议，而不是 prompt。

建议建立：

- `TaskEnvelope`
- `TaskResult`
- `TaskEvent`
- `ArtifactRef`

作用：

- 统一 agent 间消息结构
- 统一任务生命周期管理
- 统一日志与追踪字段

### 6.2 Tool Boundary Harness

每个 Agent 应该只看到自己的工具集，而不是共享一个大工具池。

建议：

- `Metrics Agent` 只允许调用 `query_prometheus_alerts`
- `Log Agent` 只允许调用 `query_log`
- `Knowledge Agent` 只允许调用 `query_internal_docs`
- `SQL Agent` 单独受控

作用：

- 降低误调用风险
- 方便权限隔离
- 便于调试和审计

### 6.3 Context Harness

不要把全部上下文直接塞给每个 agent。

建议：

- 每个 specialist 只拿最小上下文
- Memory 注入统一走 service 层
- 文档、日志、告警等中间结果通过 artifact 引用传递

作用：

- 降低上下文膨胀
- 提高模型稳定性

### 6.4 Verification Harness

这是你项目当前最缺的一层。

建议建立：

- tool contract tests
- multi-agent replay tests
- structured output schema checks
- regression eval set

作用：

- 让系统输出不只“看起来像对”
- 而是“可验证地对”

### 6.5 Observability Harness

建议统一记录：

- `session_id`
- `trace_id`
- `task_id`
- `parent_task_id`
- `agent_name`
- `tool_name`
- `latency_ms`
- `status`

作用：

- 出问题时能回放
- 知道哪一层出了错

### 6.6 Correction Harness

Harness 的真正价值不在第一次跑通，而在出错后的可修复性。

建议机制：

- 每个高价值失败案例都沉淀成文档
- 每次出错后至少补一个：
  - rule
  - test
  - doc
  - guardrail

作用：

- 系统会越用越稳

---

## 7. 采用 Harness Engineering 的好处

### 7.1 好处

#### 1. 降低 Multi-Agent 失控风险

没有 Harness 的 Multi-Agent 容易变成：

- 工具乱用
- 上下文污染
- 重规划死循环
- 输出风格不一致

Harness 可以把这些问题收住。

#### 2. 更容易调试

把任务、证据、结果和工具调用结构化之后，能清楚判断：

- 路由错了
- 工具错了
- 数据缺了
- 还是汇总错了

#### 3. 更容易安全治理

你当前项目已经涉及日志、Prometheus、文档、数据库等能力。

Harness 能帮助你建立：

- 工具访问边界
- 审批门
- 风险动作开关

#### 4. 更容易迭代和扩展

当你后续增加：

- SQL Agent
- Memory Agent
- Action Agent

如果没有 Harness，系统复杂度会指数上涨。

#### 5. 更适合生产环境

生产环境要求：

- 可观测
- 可审计
- 可回滚
- 可验证

Harness 就是在补这些能力。

### 7.2 坏处

#### 1. 前期会变慢

你需要花时间定义协议、加测试、补文档、加追踪。

#### 2. 工程复杂度上升

系统不再只是“几个 prompt + 一些工具”。

#### 3. 需要纪律

团队必须接受：

- 新失败要补规则或测试
- 不要把新逻辑随便塞进总 prompt
- 工具权限需要显式管理

#### 4. 对小场景可能过重

对于普通简单聊天，重型 Harness 可能没必要。

所以关键不是“全项目都上”，而是“只在高复杂度链路上用”。

---

## 8. 当前项目的落地原则

建议遵循 5 条原则：

### 原则 1：先业务后框架

先让 AI Ops 场景受益，不要一开始全面改造整个项目。

### 原则 2：先单体内 Harness，再分布式

第一阶段只做进程内 runtime、task、artifact、trace。

### 原则 3：先约束，再自治

先让 agent 受控，再谈更高自治。

### 原则 4：先验证，再扩容

没有验证 harness 的 agent 扩张会放大风险。

### 原则 5：简单场景保持简单

普通对话链路先不重构成重型 Multi-Agent。

---

## 9. OnCallAI 项目的 Harness Engineering 落地清单

以下清单按优先级划分。

---

## 10. P0 清单

P0 表示：如果要做 Multi-Agent 或 AI Ops 生产化，这些是第一批必须补齐的。

### P0-1：统一任务协议

目标：

- 建立 agent 间统一消息结构

必须产出：

- `TaskEnvelope`
- `TaskResult`
- `TaskEvent`
- `ArtifactRef`

原因：

- 没有统一协议，就没有统一编排、追踪和验证基础

### P0-2：建立 Agent Runtime 骨架

目标：

- 建立统一注册、调度、分发机制

必须产出：

- registry
- dispatcher
- runtime
- in-memory bus

原因：

- 没有 runtime，就无法把 Harness 变成系统能力

### P0-3：建立 Tool 白名单机制

目标：

- 每个 agent 只能访问自己的工具集

必须产出：

- tool registry
- per-agent capabilities
- 拒绝未授权工具调用

原因：

- 工具边界是 Harness 最核心的约束之一

### P0-4：建立 Trace / Task ID 体系

目标：

- 所有 agent 调用都可追踪

必须产出：

- `session_id`
- `trace_id`
- `task_id`
- `parent_task_id`

原因：

- 没有统一 trace，Multi-Agent 出问题时几乎不可 debug

### P0-5：为关键工具补 contract tests

优先工具：

- `query_log`
- `query_prometheus_alerts`
- `query_internal_docs`
- `get_current_time`

目标：

- 确保输入输出契约稳定

原因：

- Tool 不稳定，agent 再聪明也不可靠

### P0-6：结果 schema 校验

目标：

- 要求每个 specialist 输出结构化结果

必须产出：

- 统一 JSON schema 或 Go struct schema
- Reporter 输入检查

原因：

- 没有结构化结果，后续聚合和验证都很脆弱

### P0-7：限制重试和重规划

目标：

- 避免死循环

必须产出：

- max_iterations
- timeout
- retry budget
- fail-fast rule

原因：

- 这是 Agent 系统最常见的失控点之一

---

## 11. P1 清单

P1 表示：P0 完成后，进入“能跑”到“好用、可维护”的阶段。

### P1-1：建立 Task Ledger

目标：

- 持久化记录任务状态和执行结果

建议内容：

- task metadata
- status history
- child tasks
- execution time
- error reason

### P1-2：建立 Artifact Store

目标：

- 中间证据不直接在 agent 间传大文本

建议内容：

- log snippets
- retrieved docs
- metrics snapshots
- intermediate reports

### P1-3：建立 Repo Knowledge 文档体系

建议目录：

- `docs/architecture/`
- `docs/agents/`
- `docs/tools/`
- `docs/runbooks/`
- `docs/policies/`

目标：

- 给 agent 和人都提供统一知识入口

### P1-4：建立 Replay / Eval 用例集

目标：

- 能回放典型 AI Ops case

建议覆盖：

- Prometheus 正常返回
- MCP 不可用
- 文档命中为空
- 多证据冲突
- 降级成功

### P1-5：建立 Memory Service 封装

目标：

- 不让每个 agent 直接读写 `utility/mem`

职责：

- memory injection
- memory extraction
- long-term retrieval
- summarization

### P1-6：建立 Approval Gate

目标：

- 为未来高风险工具调用建立审批门

适用动作：

- SQL 写操作
- 脚本执行
- 配置变更
- 自动化处置

### P1-7：建立降级策略

目标：

- 某个 specialist 失败时，系统仍可给出有限答案

例子：

- `Log Agent` 失败
  - 仍返回“基于告警和文档的分析，日志证据缺失”

---

## 12. P2 清单

P2 表示：系统进入更成熟、更规模化的阶段。

### P2-1：分布式 A2A

目标：

- specialist 独立部署

建议方式：

- gRPC 或 HTTP 控制面
- NATS / Redis Streams 事件面

### P2-2：更细粒度 Agent 拆分

可新增：

- `SQL Agent`
- `Memory Agent`
- `Action Agent`
- `SRE Reporter Agent`

### P2-3：自动化 Correction Pipeline

目标：

- 将失败案例自动沉淀为评测样本和规则建议

### P2-4：引入成本优化策略

目标：

- 降低 token 和延迟开销

建议：

- triage 用快模型
- planner 用强模型
- reporter 用轻模型
- 结果缓存

### P2-5：组织级 Harness Governance

目标：

- 多团队协作时统一规范

内容：

- agent naming
- tool ownership
- schema versioning
- review checklist

---

## 13. 推荐实施顺序

建议你按以下顺序推进：

1. P0-1 到 P0-4
   - 协议、runtime、工具白名单、trace
2. P0-5 到 P0-7
   - contract tests、schema、限制重试
3. P1-1 到 P1-4
   - ledger、artifact、repo docs、replay eval
4. P1-5 到 P1-7
   - memory service、审批门、降级
5. 最后再进入 P2

---

## 14. 对当前项目的最终建议

如果你的目标是：

- 把 AI Ops 做成可靠的复杂智能分析系统
- 后续演进到 Multi-Agent
- 希望系统可追踪、可灰度、可维护

那么 Harness Engineering 不是可选项，而是必要项。

但它应该：

- 先应用在 AI Ops 和 Multi-Agent Runtime 上
- 不要一开始铺满全项目
- 先补协议、约束、验证、观测
- 再追求更高自治

最务实的判断是：

- Multi-Agent 负责“分工”
- Harness Engineering 负责“控场”

没有分工，系统没扩展性。
没有控场，系统没生产性。

---

## 15. 一句话总结

对 OnCallAI 来说，Harness Engineering 最有价值的作用不是“让模型更聪明”，而是：

> 让即将变复杂的 Multi-Agent 系统，在工程上保持可控、可验证、可维护。

