# 第 15 章：Harness Engineering — "给 AI Agent 套上工程化外壳"

> **本章目标**：理解什么是 Harness Engineering，掌握六层模型和当前落地进度，能向面试官清晰解释"为什么需要 Harness"以及"你的项目做到了哪一步"。

---

## 1. 白话理解：什么是 Harness Engineering？

### 1.1 一句话解释

**Harness Engineering = 给 AI Agent 系统套一层工程化"外壳"，用确定性约束去管理非确定性的模型行为。**

它不是某个 SDK、平台或框架，而是一种**工程方法**。

### 1.2 一个类比：发动机 vs 底盘

```
Agent 系统 = 发动机
  → 产生驱动力（模型推理、工具调用、任务执行）
  → 越强越好，但没有刹车的发动机 = 灾难

Harness = 底盘 + 刹车 + 仪表盘 + 安全带
  → 确保发动机不会把车开进沟里
  → 不让车跑得更快，但让车跑得安全
```

### 1.3 为什么需要它？

```
没有 Harness 的 Agent 系统：
  ❌ Agent 调了不该调的工具（Logs Agent 去改数据库）
  ❌ 上下文塞太多导致幻觉（50 篇文档全塞给 LLM）
  ❌ 出了问题不知道哪个环节出错（没有 trace_id）
  ❌ 同一个错误反复犯（没人记得上次怎么修的）
  ❌ 改了 Prompt 不知道有没有变差（没有评测体系）

有 Harness 的 Agent 系统：
  ✅ 每个 Agent 有明确的工具白名单
  ✅ 上下文按预算裁剪，超限自动丢弃
  ✅ 全链路 trace_id 追踪，精确定位
  ✅ 犯错 → 更新 AGENTS.md 规则 → 补测试 → 下次不犯
  ✅ 改了 Prompt → 跑 Golden Case → 看 delta → 确认没退化
```

---

## 2. 核心概念：六层模型

Harness Engineering 由六层组成，每层解决一个特定问题：

```
┌──────────────────────────────────────────────────┐
│                  Harness 六层模型                    │
├────────────┬─────────────────────────────────────┤
│  协议层     │ TaskEnvelope / TaskResult / Event    │  ← "说什么话"
│  Protocol   │ 统一消息结构、生命周期、序列化           │
├────────────┼─────────────────────────────────────┤
│  边界层     │ Agent Contract / Tool 白名单          │  ← "能做什么"
│  Boundary   │ Must / MustNot / Inputs / Outputs     │
├────────────┼─────────────────────────────────────┤
│  上下文层    │ ContextEngine / Budget / Profile      │  ← "看什么信息"
│  Context    │ 按角色装配、按预算裁剪、按引用传递       │
├────────────┼─────────────────────────────────────┤
│  验证层     │ Schema Gate / Contract Enforce        │  ← "做得对不对"
│  Verification│ 输入输出契约、结构化校验                │
├────────────┼─────────────────────────────────────┤
│  观测层     │ trace_id / task_id / session_id       │  ← "出了什么错"
│  Observability│ 全链路追踪、性能度量、错误定位          │
├────────────┼─────────────────────────────────────┤
│  纠偏层     │ AGENTS.md / correction pipeline       │  ← "怎么不再犯"
│  Correction │ 失败→规则→测试→文档 的闭环              │
└────────────┴─────────────────────────────────────┘
```

### 2.1 协议层 (Protocol)

**一句话**：定义 Agent 之间说什么"话"。

```go
// internal/ai/protocol/types.go

// TaskEnvelope = 任务信封，Agent 之间传递的任务描述
type TaskEnvelope struct {
    TaskID        string            // 唯一任务 ID
    ParentTaskID  string            // 父任务 ID（支持子任务链）
    SessionID     string            // 用户会话 ID
    TraceID       string            // 全链路追踪 ID
    Goal          string            // 任务目标描述
    Assignee      string            // 指派给哪个 Agent
    Status        TaskStatus        // pending/running/succeeded/failed/degraded
    DeadlineAt    *time.Time        // 截止时间
    Metadata      map[string]string // 自定义元数据
}

// TaskResult = 任务结果，Agent 执行完毕后返回
type TaskResult struct {
    TaskID             string          // 对应哪个任务
    Agent              string          // 哪个 Agent 执行的
    Status             TaskStatus      // succeeded/failed/degraded
    Summary            string          // 结论摘要
    Confidence         float64         // 置信度 0~1
    DegradationReason  string          // 降级原因（status=degraded 时必填）
    Evidence           []EvidenceItem  // 证据列表
    Error              *TaskError      // 错误信息（status=failed 时必填）
}
```

> **面试要点**：协议层是 Harness 的基础。没有统一协议，Agent 之间说不同"方言"，编排器无法调度。

### 2.2 边界层 (Boundary)

**一句话**：限制每个 Agent 能做什么、不能做什么。

```go
// internal/ai/agent/contracts/contracts.go

// 每个 Agent 有一份 Contract（合约）
type Contract struct {
    AgentName       string   // "metrics", "logs", "knowledge"
    Role            string   // 一句话定位
    Responsibilities []string // 职责范围
    Inputs          []string // 能接收什么
    Outputs         []string // 能产出什么
    Must            []string // 必须遵守的行为
    MustNot         []string // 禁止的行为
    EvidencePolicy  string   // 证据策略
}
```

**示例：Metrics Agent 的合约**：

```go
{
    AgentName: "metrics",
    Role: "监控指标分析专家",
    Responsibilities: []string{"查询 Prometheus 告警", "分析指标趋势"},
    Must: []string{
        "必须调用 query_metrics_alerts 工具获取真实数据",
        "结论必须引用工具返回的具体数值",
    },
    MustNot: []string{
        "不得编造未查询到的告警",
        "不得尝试调用日志或知识库工具",
    },
}
```

> **面试要点**：Contract 是"文档性质"的约束——它定义了 Agent 应该遵守的规则。运行时通过 `EnforceContract()` 强制校验（见验证层）。

### 2.3 上下文层 (Context)

**一句话**：控制每个 Agent 能看到什么信息。

这就是第 5 章 ContextEngine 的内容。Harness 的上下文层强调的是**按 Agent 角色差异化装配**：

```
Metrics Agent → 需要看 Prometheus 工具结果，不需要看日志
Logs Agent    → 需要看日志工具结果，不需要看指标
Reporter      → 需要看所有 Specialist 的结果，不需要看原始工具输出
```

### 2.4 验证层 (Verification)

**一句话**：检查 Agent 的输出是否符合合约。**这是当前最薄弱的一层。**

```go
// internal/ai/protocol/validate.go — Schema Gate

func ValidateTaskResult(r *TaskResult) error {
    if r == nil                    { return ErrNilResult }
    if r.TaskID == ""              { return ErrEmptyTaskID }
    if r.Agent == ""               { return ErrEmptyAgent }
    if !validStatus(r.Status)      { return ErrInvalidStatus }
    if r.Summary == ""             { return ErrEmptySummary }
    if len(r.Summary) > 4096       { return ErrSummaryTooLong }
    if r.Status == StatusDegraded && r.DegradationReason == "" {
        return ErrDegradedNoReason
    }
    if r.Status == StatusFailed && r.Error == nil {
        return ErrFailedNoError
    }
    if r.Confidence < 0 || r.Confidence > 1 {
        return ErrInvalidConfidence
    }
    return nil
}
```

```go
// internal/ai/agent/contracts/enforce.go — Contract Enforce

func EnforceContract(result *TaskResult) *TaskResult {
    // ① 结构完整性校验
    if err := ValidateTaskResult(result); err != nil {
        return degrade(result, fmt.Sprintf("schema validation failed: %v", err))
    }
    // ② 契约符合性校验
    if err := ValidateAgainstContract(result); err != nil {
        return degrade(result, fmt.Sprintf("contract violation: %v", err))
    }
    return result  // 校验通过，原样返回
}

func degrade(result *TaskResult, reason string) *TaskResult {
    result.Status = StatusDegraded
    result.Confidence *= 0.5  // 置信度折半
    if result.DegradationReason != "" {
        result.DegradationReason += "; " + reason
    } else {
        result.DegradationReason = reason
    }
    return result
}
```

> **面试要点**：校验失败不是直接拒绝，而是**自动降级**——status 改为 degraded，置信度折半，reason 记录原因。这保证了系统不会因为校验失败而完全不可用。

### 2.5 观测层 (Observability)

**一句话**：出问题时知道"哪一个环节出了什么错"。

```go
// 全链路 ID 体系
session_id       → 用户会话
  └─ trace_id    → 一次完整请求链路
       └─ task_id → 一个具体任务
            └─ parent_task_id → 父子任务关系
```

这 4 级 ID 贯穿整个系统：Controller → Service → Agent → Tool，每一层都记录。

### 2.6 纠偏层 (Correction)

**一句话**：失败不是终点，是系统进化的起点。

```
Agent 犯错
  → 人类分析根因
  → 更新 AGENTS.md 规则（如："工具失败返回格式化字符串，不要返回 Go error"）
  → 补测试用例
  → Agent 下次不犯
```

**AGENTS.md 中的真实例子**：

```markdown
## 历史踩坑规则

- `LongTermMemory.Retrieve` 应使用 `RLock` 而非写锁，只在更新时短暂获取写锁。（问题 9，§14）
- 异步记忆抽取必须有 timeout 保护，用 `context.WithoutCancel + context.WithTimeout`。（§21.1）
- 记忆写入前必须做基础过滤：assistant boilerplate、代码块、异常长度内容应丢弃。（§27.2）
```

每一条规则都对应一个真实发生的失败案例。这就是纠偏层的价值。

---

## 3. 当前落地进度

### 3.1 总览

```
层       P0(已完成)  P1(进行中)  P2(规划中)
───────────────────────────────────────
协议层    ✅          -           -
边界层    ✅          -           -
上下文层   ✅          ✅          -
验证层    ⚠️          ⚠️          ❌
观测层    ✅          ✅          -
纠偏层    ✅          ⚠️          ❌
───────────────────────────────────────
完成率    83%        57%          0%
```

### 3.2 各层详细状态

| 层 | 状态 | 已完成 | 未完成 |
|---|---|---|---|
| **协议层** | ✅ 已完成 | TaskEnvelope/TaskResult/TaskEvent/ArtifactRef 结构完整 | — |
| **边界层** | ✅ 已完成 | 5 个 Agent 有 Contract（triage/metrics/logs/knowledge/reporter） | Contract 是文档性质，无运行时 enforce |
| **上下文层** | ✅ 已完成 | ContextEngine 7 文件，Budget/Profile/Trace 完整 | — |
| **验证层** | ⚠️ 部分完成 | Schema Gate（14 测试）+ Contract Enforce（13 测试） | 工具契约测试缺失、Replay 用例不足 |
| **观测层** | ✅ 已完成 | 31 处 trace_id 引用，覆盖全部链路 | — |
| **纠偏层** | ✅ 已完成 | AGENTS.md 20+ 条踩坑规则 | 自动化 Correction Pipeline 未开始 |

### 3.3 Harness MVP 交付物（2026-04-29）

| 文件 | 行数 | 职责 |
|---|---|---|
| `internal/ai/protocol/validate.go` | 55 | Schema Gate — 结构完整性校验 |
| `internal/ai/protocol/validate_test.go` | 157 | Schema Gate 测试 — 14 个 case |
| `internal/ai/agent/contracts/enforce.go` | 57 | Contract Enforce — 契约校验 + 自动降级 |
| `internal/ai/agent/contracts/enforce_test.go` | 114 | Enforce 测试 — 13 个 case |
| `manifest/config/config.yaml` | +10 | 新增 `harness` 配置段 |

**测试结果**：27/27 PASS。

### 3.4 配置化

```yaml
# manifest/config/config.yaml
harness:
  max_iterations: 10          # Agent 最大迭代次数
  task_timeout_ms: 300000     # 单任务超时 5min
  retry_budget: 3             # 失败重试次数
  fail_fast: false            # false: 其他 specialist 继续执行
  validation:
    enabled: true             # Schema Gate 总开关
    contract_enforce: true    # Contract 校验开关
    strict_mode: false        # false: 校验失败降级; true: 拒绝
```

---

## 4. Harness vs 防幻觉体系 vs 评测体系

这三个概念容易混淆，它们的关系是：

```
┌─────────────────────────────────────────────────┐
│                Harness Engineering               │
│                (六层工程化外壳)                     │
│                                                  │
│  ┌──────────────┐  ┌──────────────┐             │
│  │  防幻觉体系    │  │  评测体系     │             │
│  │  (第14章)     │  │  (第13章)     │             │
│  │              │  │              │             │
│  │ 工具调用闭环   │  │ Golden Case  │             │
│  │ Contract校验  │  │ LLM-as-Judge│             │
│  │ Schema Gate   │  │ 行为指标监控  │             │
│  │ 输出过滤      │  │ 人工评测     │             │
│  └──────────────┘  └──────────────┘             │
│         ↑                    ↑                    │
│         │                    │                    │
│    属于验证层            属于验证层+纠偏层           │
└─────────────────────────────────────────────────┘
```

- **Harness Engineering** 是总体框架（六层模型）
- **防幻觉体系** 是验证层的具体落地（防止 LLM 编造数据）
- **评测体系** 是验证层+纠偏层的具体落地（验证 Agent 做得对不对）

面试时可以说："Harness Engineering 是我的工程方法论，防幻觉和评测是它在验证层的两个具体实践。"

---

## 5. 面试问答

### Q1: "什么是 Harness Engineering？"

> Harness Engineering 是给 AI Agent 系统套一层工程化外壳，用确定性约束管理非确定性的模型行为。
>
> 它由六层组成：协议层（统一消息格式）、边界层（Agent 合约+工具白名单）、上下文层（按预算裁剪）、验证层（Schema Gate+Contract Enforce）、观测层（全链路追踪）、纠偏层（失败→规则→测试闭环）。
>
> 核心思想是：不追求让模型更聪明，而是让模型的行为可控、可调试、可验证。

### Q2: "你的项目 Harness 做到了什么程度？"

> P0 阶段完成率 83%。具体来说：
>
> **已完成的**：协议层（TaskEnvelope/TaskResult 结构完整）、边界层（5 个 Agent 有 Contract）、上下文层（ContextEngine 预算控制）、观测层（全链路 trace_id）、纠偏层（AGENTS.md 20+ 条踩坑规则）。
>
> **部分完成的**：验证层——Schema Gate（14 测试）和 Contract Enforce（13 测试）已落地，但工具契约测试和 Replay 用例还不够。
>
> **未开始的**：P2 阶段的自动化 Correction Pipeline、成本优化、分布式 A2A。
>
> 最大的收获是纠偏层的反馈循环——每次犯错都更新 AGENTS.md 并补测试，20+ 条规则每条都对应一个真实失败案例。

### Q3: "Contract Enforce 校验失败会怎样？"

> 不是直接拒绝，而是**自动降级**。
>
> 具体行为：status 改为 degraded，置信度折半（confidence × 0.5），DegradationReason 追加失败原因。
>
> 为什么降级而不拒绝？因为 AIOps 场景下，"有瑕疵的回答"比"没有回答"好。凌晨 3 点运维人员等着诊断建议，你因为校验失败就返回空——这比返回一个置信度 0.4 的建议更危险。
>
> 但如果配置了 `strict_mode: true`，校验失败会直接拒绝返回。

### Q4: "Harness Engineering 和 Multi-Agent 是什么关系？"

> **正交且互补**。
>
> Multi-Agent 解决的是"怎么分工"——Supervisor、Triage、Specialists 各司其职。
> Harness 解决的是"怎么控场"——分工之后，怎么保证每个 Agent 不越界、不出错、可追溯。
>
> 没有 Harness 的 Multi-Agent = 一群没有交通规则的司机。
> 只有 Harness 没有 Agent 拆分 = 空有交通规则但没车在跑。

---

## 6. 自测

### 问题 1

Harness Engineering 的六层模型分别是什么？每层的一句话职责是什么？

<details>
<summary>点击查看答案</summary>

- 协议层：统一消息格式（说什么话）
- 边界层：Agent 合约+工具白名单（能做什么）
- 上下文层：按预算裁剪上下文（看什么信息）
- 验证层：Schema Gate + Contract Enforce（做得对不对）
- 观测层：全链路追踪（出了什么错）
- 纠偏层：失败→规则→测试闭环（怎么不再犯）

</details>

### 问题 2

Contract Enforce 校验失败时，为什么不直接拒绝而是降级？

<details>
<summary>点击查看答案</summary>

AIOps 场景下，"有瑕疵的回答"比"没有回答"好。凌晨 3 点运维人员等着诊断建议，校验失败直接返回空比返回一个置信度 0.4 的建议更危险。降级保证了系统不会因为校验失败而完全不可用，同时通过置信度折半和 DegradationReason 让下游知道这个结果的可信度降低了。

</details>

### 问题 3

纠偏层的核心循环是什么？举一个你项目中的真实例子。

<details>
<summary>点击查看答案</summary>

核心循环：Agent 犯错 → 人类分析 → 更新 AGENTS.md 规则 → 补测试 → Agent 下次不犯。

真实例子：项目早期工具失败时返回 Go error，导致 eino 框架触发重试，Agent 进入死循环。修复后在 AGENTS.md 记录规则："工具失败返回格式化字符串（`[工具调用失败]`），不要返回 Go error。"并补了对应的测试用例。

</details>

---

## 相关文档

| 文档 | 内容 | 什么时候看 |
|------|------|-----------|
| [01-概览与概念](../harness/01-概览与概念.md) | Harness 六层模型详细教学 | 想深入理解时 |
| [02-当前状态审计](../harness/02-当前状态审计.md) | 逐项对照代码检查落地情况 | 面试前快速了解进度 |
| [09-MVP实施记录](../harness/09-MVP实施记录.md) | Schema Gate + Contract Enforce 实施细节 | 被问到验证层代码时 |
| [14-防幻觉体系](./14-防幻觉体系.md) | 防幻觉十四层防线 | 被问到"怎么防幻觉"时 |
| [13-Agent评测体系](./13-Agent评测体系.md) | 评测方法论 | 被问到"怎么知道做得对"时 |

---

> 📌 **学完本章，你应该能做到：**
> - 用一句话解释 Harness Engineering
> - 画出六层模型并说出每层职责
> - 讲清楚当前落地进度和未完成项
> - 解释 Contract Enforce 的降级策略
>
> 下一章 → 回顾全部章节，对着镜子讲一遍，闭眼能画出架构图。
