# 从 Orchestrator 到真正的 Multi-Agent：架构演进设计

---

## 1. 背景

在上一轮讨论中，我们确认了一个关键事实：OpsCaption 当前并不是学术/工业界定义的 "Multi-Agent System"，而是 **Orchestrator 编排模式**。

这不是贬义。Orchestrator 模式在生产环境中更可控、更可调试、更可预测。但理解"当前是什么"和"真正的 Multi-Agent 是什么"之间的差距，对于架构演进和评审答辩都至关重要。

---

## 2. 当前系统：Orchestrator 编排模式

### 2.1 模式特征

```
Supervisor（中心控制器）
    │
    ├── Triage（路由器）── 决定派谁
    │
    ├── Specialist A ────┐
    ├── Specialist B ────┤  并行执行，互不通信
    ├── Specialist C ────┘
    │
    └── Reporter（聚合器）── 汇总结果
```

**核心特点：**

| 维度 | 当前状态 |
|------|----------|
| 控制流 | 中心化，Supervisor 决定一切 |
| Agent 间通信 | 无。Specialist 之间看不到彼此的输出 |
| 决策机制 | 规则表（Triage），不是协商或投票 |
| 执行顺序 | 确定性的：triage → parallel specialists → reporter |
| Bus 能力 | 只有 Publish，没有 Subscribe |
| Agent 自主性 | 无。Agent 不能主动发起任务、不能请求其他 Agent 帮忙 |

### 2.2 优势（为什么这样是对的）

1. **可预测**：给定同一个 query，执行路径完全确定
2. **可调试**：Ledger 记录完整生命周期，trace 链路清晰
3. **低延迟**：并行 dispatch + 固定步骤，无协商开销
4. **易测试**：mock 任意一个 Agent，其他不受影响
5. **成本可控**：LLM 调用次数确定，不会出现 Agent 无限对话

### 2.3 劣势（为什么需要演进）

1. **信息孤岛**：Metrics Specialist 发现 CPU 飙升，但 Logs Specialist 不知道，不会针对 CPU 相关日志深挖
2. **单轮推理**：每个 Specialist 只执行一次，没有"根据中间结果追问"的能力
3. **静态路由**：Triage 的规则表无法处理模糊场景（比如 query 里既像告警又像知识查询）
4. **无反思**：如果 Specialist 结果质量差，没有机制让系统自动重试或换策略
5. **无协作发现**：不同 Agent 的发现之间可能有因果关系，但当前无法联动

---

## 3. Agent 系统模式分类

在设计"真正的 Multi-Agent"之前，先建立完整的认知框架。

### 3.1 六种常见模式

```
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  模式 1：Single Agent                                            │
│  ┌─────────┐                                                    │
│  │  Agent   │ ←── 一个 Agent 做所有事                             │
│  │ (ReAct)  │     OpsCaption 的 Chat Pipeline 就是这个           │
│  └─────────┘                                                    │
│                                                                  │
│  模式 2：Orchestrator（当前）                                     │
│  ┌────────────┐                                                 │
│  │ Supervisor  │ ←── 中心控制器                                   │
│  │    ┌──┬──┐  │                                                │
│  │    A  B  C  │ ←── 并行 workers，互不通信                       │
│  │    └──┴──┘  │                                                │
│  │  Reporter   │ ←── 聚合器                                      │
│  └────────────┘                                                 │
│                                                                  │
│  模式 3：Pipeline / Chain                                        │
│  ┌───┐  ┌───┐  ┌───┐                                           │
│  │ A │─▶│ B │─▶│ C │ ←── 串行，前一个的输出是后一个的输入          │
│  └───┘  └───┘  └───┘                                           │
│                                                                  │
│  模式 4：Router（Triage 的独立形态）                               │
│  ┌──────────┐                                                   │
│  │  Router   │ ─▶ A 或 B 或 C  ←── 选一个执行                    │
│  └──────────┘                                                   │
│                                                                  │
│  模式 5：Collaborative Multi-Agent（真正的协作）                   │
│  ┌───┐     ┌───┐                                               │
│  │ A │◀───▶│ B │ ←── Agent 之间双向通信                           │
│  └─┬─┘     └─┬─┘     共享工作记忆                                │
│    │  ┌───┐  │        协商 / 投票 / 辩论                          │
│    └─▶│ C │◀─┘        自主决定是否需要其他 Agent                   │
│       └───┘                                                     │
│                                                                  │
│  模式 6：Competitive / Debate                                    │
│  ┌───┐  ┌───┐                                                  │
│  │ A │  │ B │ ←── 多个 Agent 独立给出答案                         │
│  └─┬─┘  └─┬─┘     Judge 评估选最优                               │
│    └──┬───┘                                                     │
│    ┌──┴──┐                                                      │
│    │Judge│                                                      │
│    └─────┘                                                      │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 各模式对比

| 维度 | Orchestrator | Pipeline | Collaborative | Competitive |
|------|-------------|----------|---------------|-------------|
| 控制流 | 中心化 | 线性 | 去中心化 | 并行+裁判 |
| Agent 自主性 | 低 | 低 | 高 | 中 |
| 通信成本 | 低 | 低 | 高 | 中 |
| LLM 调用数 | 确定 | 确定 | 不确定 | N+1 |
| 适合场景 | 分工明确 | 处理流水线 | 复杂推理 | 质量敏感 |
| 可调试性 | 高 | 高 | 低 | 中 |
| 生产可控性 | 高 | 高 | 低 | 中 |

---

## 4. 设计真正的 Multi-Agent：渐进式方案

核心原则：**不一步到位，渐进演进，每一步都可验证、可回退。**

### 4.1 Level 0（当前）：Orchestrator

已有。不重复。

### 4.2 Level 1：带信息共享的 Orchestrator

**改什么**：让 Specialist 在执行时能看到其他已完成 Specialist 的结果。

**为什么**：解决"信息孤岛"问题。比如 Metrics Specialist 发现 CPU 飙升 → Logs Specialist 能利用这个信息做针对性日志检索。

**怎么改**：

```
当前：
  Specialist A ──┐
  Specialist B ──┤  完全独立并行
  Specialist C ──┘

Level 1：
  Phase 1: Specialist A, B, C 并行
  Phase 2: 汇总 Phase 1 结果 → 注入到需要追问的 Specialist → 二轮执行
```

**代码层面的改动**：

```go
// supervisor.go 新增二轮执行逻辑
type phaseResult struct {
    domain  string
    result  *protocol.TaskResult
}

// Phase 1: 并行执行（和现在一样）
phase1Results := parallelDispatch(ctx, rt, task, domains)

// Phase 2: 判断是否需要二轮
needsRefinement := evaluateRefinementNeed(phase1Results)
if needsRefinement {
    sharedContext := buildSharedContext(phase1Results)
    phase2Results := parallelDispatch(ctx, rt, task, needsRefinement, sharedContext)
    mergeResults(phase1Results, phase2Results)
}
```

**需要新增的基础设施**：

- `SharedContext`：一个结构化的中间结果摘要
- Supervisor 里的 `evaluateRefinementNeed()` 函数：判断是否值得二轮

**Trade-off**：

| 收益 | 成本 |
|------|------|
| Specialist 信息互通 | 延迟增加（多一轮） |
| 结果质量提升 | LLM 调用翻倍（最坏情况） |
| 仍然中心化可控 | 需要设计"什么情况触发二轮"的规则 |

**验证方式**：构造一个 case —— Metrics 发现 CPU 高，看 Logs 二轮是否能基于此信息精准检索。

### 4.3 Level 2：Bus 订阅 + Agent 主动请求

**改什么**：给 Bus 加 Subscribe 能力，让 Agent 可以监听其他 Agent 的事件并做出反应。

**为什么**：Level 1 仍然是 Supervisor 控制二轮。Level 2 让 Agent 自己决定"我需要更多信息"。

**怎么改**：

```go
// 当前 Bus 接口
type Bus interface {
    Publish(ctx context.Context, event *protocol.TaskEvent) error
}

// Level 2 Bus 接口
type Bus interface {
    Publish(ctx context.Context, event *protocol.TaskEvent) error
    Subscribe(ctx context.Context, filter EventFilter) (<-chan *protocol.TaskEvent, func())
}

type EventFilter struct {
    AgentName string   // 只监听某个 Agent 的事件
    EventType string   // 只监听某种类型的事件
    TraceID   string   // 只监听某个 trace 的事件
}
```

**Agent 接口扩展**：

```go
// 当前 Agent 接口
type Agent interface {
    Name() string
    Capabilities() []string
    Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}

// Level 2: 新增可选接口
type ReactiveAgent interface {
    Agent
    OnEvent(ctx context.Context, event *protocol.TaskEvent) (*protocol.TaskEnvelope, error)
}
```

**执行流变化**：

```
1. Supervisor 派发 Specialist A, B, C 并行执行
2. Specialist A 完成 → Bus Publish "metrics_result" 事件
3. Specialist B（如果实现了 ReactiveAgent）收到事件
4. Specialist B 决定是否要基于 A 的结果追加查询
5. Specialist B 通过 Bus 发送 "request_refinement" 事件
6. Supervisor 收到请求，决定是否批准（保留控制权）
```

**Trade-off**：

| 收益 | 成本 |
|------|------|
| Agent 有自主反应能力 | 系统行为不再完全确定 |
| 更接近"真正"的协作 | 需要防止事件风暴 |
| Supervisor 仍有审批权 | 调试复杂度上升 |
| 可选接口，不破坏现有 Agent | Bus 需要支持并发订阅 |

### 4.4 Level 3：Shared Workspace + 协作协议

**改什么**：引入共享工作区，Agent 可以读写共享状态，基于共享状态做决策。

**为什么**：Level 2 是事件驱动的被动反应。Level 3 让 Agent 主动查看全局状态、主动发起协作。

**怎么改**：

```go
type Workspace interface {
    // 写入一条发现
    PostFinding(ctx context.Context, finding Finding) error
    // 读取所有发现
    GetFindings(ctx context.Context, filter FindingFilter) ([]Finding, error)
    // 提出一个假设
    ProposeHypothesis(ctx context.Context, hypothesis Hypothesis) error
    // 对假设投票
    VoteHypothesis(ctx context.Context, hypothesisID string, vote Vote) error
    // 获取当前共识
    GetConsensus(ctx context.Context) (*Consensus, error)
}

type Finding struct {
    Agent     string
    Domain    string
    Content   string
    Confidence float64
    Evidence  []protocol.EvidenceItem
    RelatedTo []string  // 引用其他 Finding 的 ID
}

type Hypothesis struct {
    ID          string
    Agent       string
    Description string
    SupportedBy []string  // Finding IDs
    Confidence  float64
}
```

**执行流变化**：

```
1. Supervisor 启动任务，创建 Workspace
2. Specialist A 查完指标 → PostFinding("CPU 使用率 95%")
3. Specialist B 查完日志 → PostFinding("OOM killer 触发 3 次")
4. Specialist B 看到 A 的 Finding → PostFinding("OOM 和 CPU 飙升相关")
5. Knowledge Specialist 看到两个 Finding → 搜索 "OOM + CPU" → 找到历史案例
6. Reporter 读取 Workspace → 生成有因果链的报告
```

**这就是学术意义上的 Collaborative Multi-Agent。**

**Trade-off**：

| 收益 | 成本 |
|------|------|
| 发现间有因果关系 | 系统复杂度大幅上升 |
| 结果质量显著提升 | 延迟不可预测 |
| 可以做假设验证 | 需要终止条件防止无限循环 |
| 接近人类协作模式 | LLM 调用次数不确定 |
| 评审时非常有说服力 | 需要完善的观测和调试工具 |

### 4.5 Level 4：Debate / Self-Critique

**改什么**：多个 Agent 对同一个问题给出独立判断，然后互相质疑/辩论，最终达成共识。

**为什么**：提升高风险决策的可信度。论文表明 debate 机制可以减少 LLM 幻觉。

**怎么改**：

```go
type DebateRound struct {
    RoundNumber int
    Positions   []Position
    Critiques   []Critique
    Consensus   *string  // nil 表示未达成共识
}

type Position struct {
    Agent      string
    Conclusion string
    Evidence   []protocol.EvidenceItem
    Confidence float64
}

type Critique struct {
    FromAgent  string
    ToAgent    string
    Content    string
    Type       string  // "support" / "challenge" / "refine"
}

func runDebate(ctx context.Context, agents []Agent, topic string, maxRounds int) (*Consensus, error) {
    for round := 0; round < maxRounds; round++ {
        positions := collectPositions(ctx, agents, topic, previousRound)
        critiques := collectCritiques(ctx, agents, positions)
        if consensus := checkConsensus(positions, critiques); consensus != nil {
            return consensus, nil
        }
    }
    return majorityVote(lastPositions), nil
}
```

**适用场景**：

- 根因定位（多个 Specialist 各自分析，互相质疑）
- 方案推荐（多个策略 Agent 各出方案，评估对比）
- 高风险变更审批（多角度评估风险）

**Trade-off**：

| 收益 | 成本 |
|------|------|
| 减少单 Agent 幻觉 | LLM 调用 O(N × Rounds) |
| 结论更可信 | 延迟 O(Rounds) |
| 评审高分 | 实现复杂 |
| 可解释性强 | 需要好的终止条件 |

---

## 5. 推荐演进路径

```
当前 ──▶ Level 1 ──▶ Level 2 ──▶ Level 3
 │                                  │
 │  短期（1-2 周）                    │  中期（1-2 月）
 │  改 Supervisor                    │  改 Bus + Workspace
 │  加二轮执行                        │  新增 ReactiveAgent 接口
 │  投入：小                          │  投入：中
 │  风险：低                          │  风险：中
 │  收益：信息互通                     │  收益：真正协作
 └──────────────────────────────────┘
```

### 5.1 为什么推荐先做 Level 1

1. **改动最小**：只改 supervisor.go，不动基础设施
2. **效果可度量**：构造 information-sharing case，对比有无二轮的结果质量
3. **不破坏线上**：二轮是 opt-in 的，不影响现有路径
4. **为后续铺路**：SharedContext 的设计可以复用到 Level 3 的 Workspace

### 5.2 需要的基础设施升级清单

| Level | 需要新增/修改 | 已有可复用 |
|-------|-------------|-----------|
| Level 1 | Supervisor 二轮逻辑、SharedContext 结构 | Ledger、ArtifactStore |
| Level 2 | Bus Subscribe、EventFilter、ReactiveAgent 接口 | Bus Publish、TaskEvent |
| Level 3 | Workspace 接口、Finding/Hypothesis 数据结构 | ArtifactStore（可扩展为 Workspace 后端） |
| Level 4 | Debate 引擎、Critique 数据结构、Consensus 算法 | Protocol 层的 evidence/confidence |

---

## 6. 当前系统已经具备的演进基础

这是好消息——当前架构虽然是 Orchestrator，但设计中已经埋了不少扩展点：

| 已有组件 | 为什么可以复用 |
|----------|---------------|
| Bus + Publish | 加 Subscribe 就变成完整事件系统 |
| Ledger + EventsByTrace | 已有完整事件回放能力 |
| ArtifactStore | 可作为 Workspace 的存储后端 |
| TaskEnvelope.Input | 可以注入 SharedContext |
| protocol.EvidenceItem | 已有结构化证据格式 |
| protocol.Confidence | 已有置信度，可用于投票/共识 |
| Runtime.Dispatch | 已有完整的任务生命周期管理 |
| Agent.Capabilities() | 可扩展为 capability-based 路由 |

---

## 7. 面对评审时如何解释

### Q: "你这个系统真的是 Multi-Agent 吗？"

**诚实回答**：

> 当前实现是 Orchestrator 编排模式——有一个 Supervisor 做中心调度，多个 Specialist 并行执行，最后 Reporter 汇总。Specialist 之间不直接通信。
>
> 这是一个有意的架构选择。在生产运维场景中，可预测性和可调试性比 Agent 自主性更重要。中心化控制让我们能精确追踪每个 Agent 的执行路径、控制 LLM 调用成本、保证超时降级。
>
> 但架构已经为 Multi-Agent 演进做了准备：统一的 Protocol 层、事件总线、产物存储、Agent 注册机制。下一步计划是引入 SharedContext 让 Specialist 信息互通，再逐步增加 Bus 订阅和 Workspace 协作。

### Q: "为什么不直接做真正的 Multi-Agent？"

> 三个原因：
> 1. **成本不可控**：协作式 Multi-Agent 的 LLM 调用次数不确定，我们用的 DeepSeek V3 虽然便宜，但也不能无限调用
> 2. **调试困难**：Agent 自主决策意味着执行路径不确定，出 bug 很难复现
> 3. **收益不确定**：在我们当前的评测 case 上，Orchestrator 模式已经能覆盖大部分场景。只有少数复杂故障（需要跨域因果推理）才需要真正的协作

### Q: "你怎么证明 Orchestrator 比 Multi-Agent 更好？"

> 我不会说"更好"，而是"更适合当前阶段"。我们有 baseline 评测，可以度量 Recall@K、Hit@K 等指标。当 Orchestrator 模式的指标触顶后，就是引入协作机制的时机。Level 1（二轮执行 + 信息共享）的收益可以通过构造 information-sharing case 来度量。

---

## 8. 你应该学会什么

1. **架构模式不是越复杂越好**。Orchestrator 在生产中比 Collaborative 更常用，因为它可控。
2. **准确命名很重要**。叫 "Multi-Agent" 但实际是 Orchestrator，评审会追问。诚实说"Orchestrator 编排 + 渐进式 Multi-Agent 演进"更有说服力。
3. **演进路径比最终形态更重要**。评审关心的是你的思考过程：为什么选 Orchestrator → 什么时候切换 → 怎么度量收益。
4. **现有基础设施是资产**。Bus、Ledger、ArtifactStore 不是白写的，它们是演进的基础。
5. **每个 Level 都需要验证**。不是做了就算，要有 case 证明"加了这个机制确实提升了质量"。

---

## 9. 风险和边界

| 风险 | 缓解 |
|------|------|
| Level 1 二轮执行增加延迟 | 设超时上限，快速失败则用一轮结果 |
| Level 2 事件风暴 | 限制每个 Agent 的 Subscribe 数量 + 事件去重 |
| Level 3 无限协作循环 | MaxRound 硬限制 + 总 LLM token 预算 |
| Level 4 Debate 不收敛 | 最多 3 轮 + majority vote 兜底 |
| 演进过程中线上回归 | 每个 Level 都做 feature flag，可一键关闭 |