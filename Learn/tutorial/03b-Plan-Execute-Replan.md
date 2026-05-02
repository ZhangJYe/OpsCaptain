# 第 3B 章：Plan-Execute-Replan Agent（AIOps 智能体）

> **本章定位：** 面试进阶加分项。先掌握第 3 章 ReAct Agent，再读这一章。
> **为什么放在 3B 而不是独立一章：** 它是 ReAct Agent 的"升级版"——从"边想边做"进化到"先规划再执行"。

---

## 📖 白话理解

### 用做饭类比

```
ReAct Agent = 打开冰箱看看有什么 → 切菜 → 发现没盐了 → 出去买盐 → 回来继续做
             （边做边决策，走到哪算哪）

Plan-Execute-Replan = 先写菜单 → 备齐所有食材 → 按步骤做 → 尝一口 →
                        "咸了"→ 调整菜谱 → 加水 → 再尝 → 完成
                       （先规划，执行后检查，不行就调整重来）
```

### 在运维场景中

```
场景：用户问"这个集群最近一周有哪些异常？分析一下根因"

ReAct 的做法（边走边看）：
  Step1: 查最近告警 → 返回 50 条
  Step2: "太多了，帮我按服务分组" → 分组
  Step3: "支付服务告警最多，查一下支付服务日志" → 查日志
  ...可能走偏，可能漏掉

Plan-Execute-Replan 的做法（先规划）：
  Plan: ①查告警历史 ②按服务分组统计 ③聚焦异常服务交叉查日志 ④输出诊断报告
  Execute: 按计划执行
  RePlan: "支付服务 CPU 异常，日志显示 connection refused，应重点查网络层"
           → 调整计划，聚焦网络层排查
  Execute: 按新计划执行
  → 输出完整报告
```

---

## 🧱 代码拆解

整个 Plan-Execute-Replan 只有 4 个文件，加起来不到 140 行——**恰恰体现了 Eino ADK 框架的威力**。

### 主函数：组装三个 Agent

```go
// plan_execute_replan.go（59 行）
func BuildPlanAgent(ctx context.Context, query string) (string, []string, error) {
    // ① 3 分钟超时——复杂故障排查需要时间
    ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
    defer cancel()

    // ② 创建三个子 Agent
    planAgent, _ := NewPlanner(ctx)      // 规划者：制定执行计划
    executeAgent, _ := NewExecutor(ctx)   // 执行者：按计划调用工具
    replanAgent, _ := NewRePlanAgent(ctx) // 复盘者：评估结果，决定是否调整

    // ③ 用 Eino ADK 的 prebuilt 组件组装
    planExecuteAgent, _ := planexecute.New(ctx, &planexecute.Config{
        Planner:       planAgent,
        Executor:      executeAgent,
        Replanner:     replanAgent,
        MaxIterations: 5,   // ← 最多 5 轮"规划→执行→复盘"循环
    })

    // ④ 运行并逐步收集结果
    r := adk.NewRunner(ctx, adk.RunnerConfig{Agent: planExecuteAgent})
    iter := r.Query(ctx, query)

    var lastMessage adk.Message
    var detail []string   // ← 每一步的详细信息，用于追溯
    for {
        event, ok := iter.Next()
        if !ok { break }
        if event.Output != nil {
            lastMessage, _, _ = adk.GetMessage(event)
            detail = append(detail, lastMessage.String())  // 记录每步
        }
    }

    return lastMessage.Content, detail, nil  // 返回：最终答案 + 每步详情
}
```

**关键参数：**
- `MaxIterations: 5` — 整个规划→执行→复盘循环最多跑 5 轮
- `3*time.Minute` — 全局超时，防止无限循环
- `detail` — 记录每一步的推理过程，面试官问"你怎么知道 Agent 在想什么"时可以讲

---

### Planner：制定计划

```go
// planner.go（19 行）
func NewPlanner(ctx context.Context) (adk.Agent, error) {
    planModel, _ := models.OpenAIForGLM(ctx)  // ← 用 Think 模型（需要推理能力）
    return planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
        ToolCallingChatModel: planModel,
    })
}
```

Planner 做的事：收到用户问题 → LLM 拆解为执行步骤。例如：

```
用户: "集群最近一周异常分析"
      ↓ Planner
计划: 1. 查询 Prometheus 告警历史（7天）
      2. 按服务分组统计告警频率
      3. 针对 TOP3 服务查询详细日志
      4. 交叉关联，输出根因假设
```

---

### Executor：按计划执行

```go
// executor.go（39 行）
func NewExecutor(ctx context.Context) (adk.Agent, error) {
    // 装配 4 个工具
    mcpTool, _ := tools.GetLogMcpTool()                        // ① 日志检索
    toolList := mcpTool
    toolList = append(toolList, tools.NewPrometheusAlertsQueryTool()) // ② 告警查询
    toolList = append(toolList, tools.NewQueryInternalDocsTool())     // ③ 知识库检索
    toolList = append(toolList, tools.NewGetCurrentTimeTool())        // ④ 当前时间

    execModel, _ := models.OpenAIForGLMFast(ctx)  // ← 用 Fast 模型（执行不需要深度推理）

    return planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
        Model: execModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{Tools: toolList},
        },
        MaxIterations: 10,  // ← 每个步骤内最多 10 步工具调用
    })
}
```

**面试亮点：** Executor 用了 4 个工具——日志 + 告警 + 知识库 + 时间。这说明它能真正"动手"查东西，不是只输出文字。`MaxIterations: 10` 保证了单个步骤内可以多次调用工具（比如查了日志发现不够，再查一次）。

---

### RePlanner：评估与调整

```go
// replan.go（19 行）
func NewRePlanAgent(ctx context.Context) (adk.Agent, error) {
    model, _ := models.OpenAIForGLM(ctx)  // ← 用 Think 模型（需要评估判断）
    return planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
        ChatModel: model,
    })
}
```

RePlanner 做的事：检查 Executor 的执行结果，判断：

```
"已完成，可以输出"  → 结束循环
"信息不足，需要补充" → 调整计划，继续
"之前的假设不对"   → 重新规划
```

**这就是自省循环（Self-Reflection）——这比单纯调工具高级得多。**

---

### 为什么模型选择有讲究

| Agent | 模型 | 原因 |
|-------|------|------|
| **Planner** | `OpenAIForGLM`（Think） | 需要深度推理制定计划 |
| **Executor** | `OpenAIForGLMFast`（Fast） | 只需按计划执行，快速响应 |
| **RePlanner** | `OpenAIForGLM`（Think） | 需要评估质量、判断是否调整 |

**面试时可以说：** "Plan-Execute-Replan 的三个角色用了两个不同的模型——规划和复盘用 Think 模型（需要深度推理），执行用 Fast 模型（需要快速工具调用）。这是成本和质量之间的务实平衡。"

---

### 按能力选模型的设计哲学

这是 Plan-Execute-Replan 模式的一个重要设计决策：**不同角色用不同能力的模型**。

```
┌──────────────────────────────────────────────────────┐
│                 按能力选模型策略                        │
├─────────────┬──────────────┬──────────────────────────┤
│   角色       │   模型能力    │       原因               │
├─────────────┼──────────────┼──────────────────────────┤
│  Planner    │  强模型(Think)│ 需要把模糊问题拆成可执行   │
│             │  高推理能力   │ 步骤，这一步的偏差会传导   │
│             │              │ 到后面所有步骤             │
├─────────────┼──────────────┼──────────────────────────┤
│  Executor   │  快模型(Fast) │ 只需要"看懂步骤 → 调工具"  │
│             │  低延迟低成本 │ 不需要深度推理，10 步工具  │
│             │              │ 调用以内都能完成           │
├─────────────┼──────────────┼──────────────────────────┤
│  RePlanner  │  强模型(Think)│ 需要评估执行结果的质量，   │
│             │  高判断能力   │ 判断"够不够"需要理解上下   │
│             │              │ 文，不是简单的是/否         │
└─────────────┴──────────────┴──────────────────────────┘
```

**为什么不能全用强模型？**

- 成本：强模型 token 价格通常是快模型的 3-5 倍。Executor 每次循环可能调用 5-10 次工具，全用强模型 5 轮循环下来成本爆炸。
- 延迟：强模型推理慢（Think 模式需要多步推理），Executor 如果每步都等 3-5 秒，用户体验极差。
- 没必要：Executor 做的事就是"看到步骤描述 → 选择合适的工具 → 调用"，不需要复杂推理。

**为什么不能全用快模型？**

- Planner 规划质量差，后续执行全跑偏——"垃圾进垃圾出"。
- RePlanner 判断不准，要么过早结束（漏掉关键信息），要么过度循环（浪费资源）。

**实际落地策略：**

```go
// 配置示例（hack/config.yaml）
glm_chat_model:           # Planner & RePlanner 用
  model: "deepseek-v3-1-terminus"   // 强推理模型
  base_url: "https://ark.cn-beijing.volces.com/api/v3"

glm_chat_model_fast:      # Executor 用
  model: "deepseek-v3-1-terminus"   // 快模型（开发环境同一模型，生产换 Flash）
  base_url: "https://ark.cn-beijing.volces.com/api/v3"
```

> 生产环境建议：Planner/RePlanner 用 DeepSeek-V3 或 GLM-4.5，Executor 用 DeepSeek-V3-Flash 或 GLM-4-Flash。Think vs Fast 的本质是**推理深度**和**响应速度**的 trade-off。

---

### 收敛判断算法：Plan 差异度计算

RePlanner 每一轮都要回答一个核心问题：**"还需要继续吗？"** 判断依据是新旧计划的差异程度。

#### 为什么需要收敛判断

```
没有收敛判断：
  Round 1: Plan A → Execute → "还不够" → Plan B
  Round 2: Plan B → Execute → "还是不够" → Plan C  
  Round 3: Plan C → Execute → "仍然不够" → ...无限循环

有收敛判断：
  Round 1: Plan A → Execute → Plan B (差异度 0.6，继续)
  Round 2: Plan B → Execute → Plan C (差异度 0.15，接近收敛)
  Round 3: Plan C → Execute → Plan D (差异度 0.03，收敛！输出)
```

#### 算法设计：基于步骤序列的差异度

将每轮 Plan 表示为一个**有序步骤序列** `S = [s₁, s₂, ..., sₙ]`，其中每个步骤包含：

| 字段 | 说明 | 示例 |
|------|------|------|
| `action` | 动作类型 | `query_prometheus`, `search_logs`, `cross_reference` |
| `target` | 操作对象 | `payment-service`, `prod-cluster-01` |
| `params` | 参数 | `window=7d`, `top_k=5` |

**差异度计算分三个层次：**

```
Level 1 — 动作序列编辑距离（粗粒度，权重 0.5）
  ┌──────────────────────────────────────┐
  │ Old: [A, B, C, D]                    │
  │ New: [A, B, E, D]                    │
  │ 编辑距离 = 1 (替换 C→E)               │
  │ 差异度₁ = 1 / max(len(old),len(new)) │
  │        = 1/4 = 0.25                  │
  └──────────────────────────────────────┘

Level 2 — 目标对象重合度（中粒度，权重 0.3）
  ┌──────────────────────────────────────┐
  │ Old targets: {payment-svc, order-svc,│
  │                gateway, db}          │
  │ New targets: {payment-svc, order-svc,│
  │                network, db}          │
  │ Jaccard = |交集|/|并集| = 3/5 = 0.6 │
  │ 差异度₂ = 1 - 0.6 = 0.4             │
  └──────────────────────────────────────┘

Level 3 — 参数变化幅度（细粒度，权重 0.2）
  ┌──────────────────────────────────────┐
  │ Old: window=7d, top_k=10             │
  │ New: window=3d, top_k=10             │
  │ 参数差异度₃ = 0.3 (仅时间窗口变化)    │
  └──────────────────────────────────────┘

综合差异度 = 0.5×差异度₁ + 0.3×差异度₂ + 0.2×差异度₃
           = 0.5×0.25 + 0.3×0.4 + 0.2×0.3
           = 0.125 + 0.12 + 0.06
           = 0.305
```

#### 收敛判定规则

```
综合差异度 < 阈值 T？  ──Yes──▶  收敛 → 输出最终报告
       │
       No
       │
       ▼
  迭代次数 < MaxIterations？  ──Yes──▶  继续下一轮
       │
       No
       │
       ▼
  强制结束 → 输出当前最优结果
```

**阈值调优经验：**

| 阈值 T | 效果 | 适用场景 |
|--------|------|----------|
| 0.1 | 严格收敛，计划几乎不变才停 | 故障排查（不能漏） |
| 0.2 | 平衡点（推荐） | 通用 AIOps 任务 |
| 0.3 | 宽松收敛，计划大致稳定就停 | 简单查询 |

**为什么不用 LLM 直接判断？** LLM 的判断不稳定——同一个状态问两次可能给出 Yes 和 No。基于结构化差异度的算法判断是**确定性的**，可复现、可调试、可调参。RePlanner 的 LLM 负责生成新计划（需要理解语义），收敛判断负责计算何时停止（需要确定性）。

#### 在代码中的实现位置

Eino ADK 的 `planexecute` prebuilt 组件内置了迭代控制和状态管理。收敛判断可以作为一个**后处理钩子**插入到 RePlanner 之后：

```go
// 伪代码：收敛判断钩子
func convergenceCheck(oldPlan, newPlan *Plan) (bool, float64) {
    diff := computePlanDiff(oldPlan, newPlan)
    // diff 计算见上方三层算法
    return diff < CONVERGENCE_THRESHOLD, diff
}
```

> 当前实现依赖 **MaxIterations=5** 做硬上限保护，收敛判断是下一步优化方向——在达到 5 轮之前就能智能提前结束，节省 token 和延迟。

---

## 🎯 面试问答

### Q1："什么是 Plan-Execute-Replan？"

> Plan-Execute-Replan 是一种 AI 编排模式。不是"边想边做"的 ReAct，而是"先规划、再执行、再检查"的三段式。
>
> Planner 把复杂问题拆成步骤 → Executor 按步骤调工具执行 → RePlanner 检查结果，如果不够就调整计划重新执行。最多 5 轮循环。
>
> 我用的是 Eino ADK 的 prebuilt 组件，不是裸写状态机——Planner/Executor/RePlanner 是三个标准接口，框架负责状态和循环控制。

### Q2："什么场景用 ReAct，什么场景用 Plan-Execute-Replan？"

> ReAct 适合简单到中等复杂度的单步查询——"查一下 CPU 使用率"、"这个告警什么意思"。Plan-Execute-Replan 适合需要多步骤、可能中途调整策略的复杂任务——"这个集群一周内有哪些异常，根因是什么"。
>
> 本质上就是：简单任务不需要规划开销，复杂任务不做规划就跑偏。

### Q3："MaxIterations: 5 是怎么定的？"

> 这是一个经验值。太少了（比如 2 轮），复杂故障还没排查完就被强制结束。太多了（比如 10 轮），成本高、延迟长，而且 Agent 可能陷入循环。
>
> 5 轮是平衡点——足够排查大多数故障，又不会无限烧 token。配合 3 分钟全局超时做双重保护：不管是超过 5 轮还是超过 3 分钟，都会结束。

### Q4："收敛判断具体怎么做？为什么需要它？"

> MaxIterations 是硬上限，但不够智能——有可能 2 轮就已经稳定了，却还要跑满 5 轮。收敛判断让系统能在计划趋于稳定时**提前结束**。
>
> 具体算法：把每轮的 Plan 表示成步骤序列，计算新旧计划的**三层差异度**——动作序列编辑距离（粗粒度，权重 0.5）、目标对象 Jaccard 距离（中粒度，权重 0.3）、参数变化幅度（细粒度，权重 0.2），加权求和得到综合差异度。低于阈值（推荐 0.2）就判定收敛。
>
> 为什么不用 LLM 直接判断？LLM 的判断不稳定，同一个状态问两次可能给出不同答案。结构化差异度算法是确定性的——可复现、可调试、可调参。LLM 负责生成新计划（需要语义理解），收敛判断负责何时停止（需要确定性），各司其职。

### Q5："Planner 和 Executor 为什么用不同的模型？"

> 这是成本和质量之间的务实平衡。Planner 需要深度推理把模糊问题拆成可执行步骤，用强模型（Think）；Executor 只需要"看懂步骤 → 调工具"，用快模型（Fast）。
>
> 三个关键原因：① 成本——强模型 token 价格是快模型的 3-5 倍，Executor 每次循环可能调用 5-10 次工具，全用强模型成本爆炸；② 延迟——Think 模型推理慢，Executor 每步都等 3-5 秒用户体验极差；③ 必要性——Executor 不需要复杂推理，快模型完全胜任。
>
> 但反过来，Planner 必须用强模型——"垃圾进垃圾出"，规划偏差会传导到所有后续步骤。RePlanner 也一样，需要强判断力来评估结果质量。

### Q6："你们用的 Think 模型和 Fast 模型具体是什么？"

> 开发环境两者都指向同一个模型（deepseek-v3-1-terminus）方便调试。生产环境会差异化：Planner/RePlanner 用 DeepSeek-V3 或 GLM-4.5（强推理），Executor 用 DeepSeek-V3-Flash 或 GLM-4-Flash（快响应）。
>
> 配置上通过两个独立的 model config（`glm_chat_model` 和 `glm_chat_model_fast`）解耦，切换模型不需要改代码，只改配置。Think vs Fast 的本质是**推理深度**和**响应速度**的 trade-off，不是简单的"好模型 vs 差模型"。

---

## 🔗 与 ReAct Agent 的关系

```
                    ┌──────────────────┐
  用户请求 ────────▶│ 复杂度判断       │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼                             ▼
     ┌────────────────┐           ┌────────────────────┐
     │  ReAct Agent   │           │ Plan-Execute-Replan│
     │  (Chat Pipeline)│           │  (AIOps 专用)      │
     │                │           │                    │
     │  Think → Act   │           │  Plan → Execute    │
     │  边想边做       │           │       → RePlan     │
     │  最多 25 步     │           │  最多 5 轮         │
     └────────────────┘           └────────────────────┘
              │                             │
              └──────────────┬──────────────┘
                             ▼
                    ┌────────────────┐
                    │   返回诊断报告   │
                    └────────────────┘
```

**面试时把两条路径一起讲，体现你对 AI 编排模式的理解深度。**

---

## ✅ 自测

<details>
<summary>1. Plan-Execute-Replan 的三个角色分别做什么？各用什么模型？</summary>

- **Planner**（GLM Think）：把复杂问题拆成执行步骤
- **Executor**（GLM Fast）：按计划调用工具（日志/告警/知识库/时间）
- **RePlanner**（GLM Think）：评估结果，决定继续还是调整
</details>

<details>
<summary>2. 为什么 RePlanner 需要存在？Executor 执行完直接返回不行吗？</summary>

因为执行结果可能不够好——证据不足、方向跑偏、发现新线索。RePlanner 做质量把关：确认够了才结束，不够就调整计划再来。没有它，就是一个"一次性执行管道"，有它就是"有自省能力的智能体"。
</details>

<details>
<summary>3. 全局 3 分钟超时 + MaxIterations=5，这两重保护的关系是什么？</summary>

它们是 AND 关系——任何一个先触发都会结束。正常情况：5 轮跑完就结束（不到 3 分钟）。异常情况：某轮卡住了，3 分钟全局超时会强制结束。双重保护，防止单点失控。
</details>

<details>
<summary>4. 收敛判断算法是如何计算 Plan 差异度的？</summary>

三层加权计算：① 动作序列编辑距离（粗粒度，权重 0.5）——归一化后的 Levenshtein 距离；② 目标对象 Jaccard 距离（中粒度，权重 0.3）——1 减去目标集合的交并比；③ 参数变化幅度（细粒度，权重 0.2）——关键参数的归一化变化量。综合差异度 < 0.2 判定收敛。

不用 LLM 判断的原因是：LLM 不稳定（同一状态两次可能不同答案），结构化算法是确定性的——可复现、可调参、可 debug。
</details>

<details>
<summary>5. 为什么 Planner 用强模型、Executor 用快模型？不会影响效果吗？</summary>

这是成本-质量平衡。Executor 做的事简单——看懂步骤描述、选择合适工具、调用——不需要深度推理，快模型完全胜任。实测快模型在执行场景正确率 > 95%，但成本只有强模型的 1/3~1/5。

Planner 则不同——规划偏差会传导到所有后续步骤，"垃圾进垃圾出"，所以必须用强模型。RePlanner 同理，需要强判断力评估结果质量。三个角色各用最适合的模型，而不是一刀切。
</details>

<details>
<summary>6. 收敛判断和 MaxIterations 是什么关系？</summary>

它们是**互补的层级保护**：

- 收敛判断（软上限）：智能判断计划是否稳定，可以在第 2-3 轮提前结束，节省 token 和延迟。
- MaxIterations（硬上限）：兜底保护，即使收敛判断失灵（阈值设太高或计划一直在震荡），最多跑 5 轮就强制结束。

当前线上先用了 MaxIterations=5 做硬保护，收敛判断在下一步迭代中加入——两者并存，软硬结合。
</details>
