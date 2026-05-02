# 第 13 章：Agent 评测体系 — "你怎么证明你的 Agent 真的变好了？"

> 当前统一口径（2026-05）
> - 当前线上回答链路以 `Chat ReAct` 和 `AIOps Plan-Execute-Replan` 为核心。
> - 文中如果出现 `triage / reporter / multi-agent pipeline` 等对象，应理解为 Harness 评测讨论中的历史样本，不再代表当前聊天主链路。

> **一个没有评测的系统是信仰驱动的。**
> 本章帮你建立完整的 Agent 评测方法论——从单元测试到 LLM-as-Judge 到业界 Benchmark，面试时能讲清楚"你怎么知道你的 Agent 做得对？"

---

## 1. 白话理解：Agent 评测为什么比传统软件难？

### 1.1 一句话解释

**传统软件：输入确定，输出确定，对不对一目了然。Agent：同一个问题可以有多种正确回答，甚至流程不同但结果都对。**

### 1.2 一个类比：给厨师考试打分

```
传统软件评测 = 烤面包机测试
  输入：插电 + 放面包 + 按下按钮
  输出：弹出烤好的面包
  对不对：面包金黄 = 对 / 面包焦黑 = 错
  ✅ 对错明确，自动化容易

Agent 评测 = 厨师考试
  输入："做一个四菜一汤"
  输出：四道菜 + 一碗汤
  对不对：
    - 菜做熟了 → 基本要求 ✓
    - 味道怎么样 → 主观判断
    - 用的什么步骤 → 步骤A对，步骤B也对，都合理
    - 浪费了多少食材 → 效率问题
    - 厨房炸了没有 → 安全性
  ❌ 没有唯一正确答案，需要多维度评分
```

### 1.3 Agent 评测的五个难点

| 难点 | 说明 | 举例 |
|------|------|------|
| **非确定性** | 同一问题，LLM 两次回答可能不同 | "MySQL 连接数高了怎么办？"——第一次推荐查 slowlog，第二次先推荐扩连接池，都合理 |
| **多路径正确** | Plan A 和 Plan B 都能解决问题 | ReAct 6 步解决 vs Plan-Execute 3 轮解决——都对了，怎么比？ |
| **开放式输出** | 诊断报告没有标准答案 | "checkoutservice CPU 高"——有人写 200 字，有人写 800 字，谁更好？ |
| **工具依赖** | Agent 调用工具可能失败，需要降级 | Prometheus 挂了，Agent 用历史数据做了近似分析——算对还是算错？ |
| **成本与延迟** | 质量不是唯一维度 | Rerank 能提召回率但多花 2 秒——值不值？ |

---

## 2. 五维评测框架

> **评测一个 Agent，至少从这五个维度看。**

### 2.1 五个维度

```
                    ┌──────────────┐
                    │  任务完成度   │ ← 问题解决了吗？
                    │  (Success)    │
                    ├──────────────┤
                    │  过程质量     │ ← 推理正确吗？工具用对了吗？
                    │  (Process)    │
                    ├──────────────┤
                    │  鲁棒性       │ ← 工具挂了他扛得住吗？
                    │  (Robustness) │
                    ├──────────────┤
                    │  效率/成本    │ ← token 花了多少？几步完成的？
                    │  (Efficiency) │
                    ├──────────────┤
                    │  安全性       │ ← 有没有乱调危险的 API？
                    │  (Safety)     │
                    └──────────────┘
```

### 2.2 每个维度的量化指标

| 维度 | 指标 | 怎么算 | 目标 |
|------|------|--------|------|
| **任务完成度** | Task Success Rate | 正确完成的 case 数 / 总 case 数 | > 85% |
| **过程质量** | Intent Accuracy | 意图识别正确的比例 | > 90% |
| **过程质量** | Tool Selection Correctness | 选了正确工具的次数 / 总工具调用次数 | > 80% |
| **鲁棒性** | Degradation Rate | 触发了降级但任务仍完成的比例 | 越低越好 |
| **效率/成本** | Avg Steps per Task | 平均完成任务需要的 Agent 步数 | < 10 |
| **效率/成本** | Avg Token per Task | 平均完成任务消耗的 token | < 5000 |
| **安全性** | Safety Violations | 触发了安全规则的次数 | 0 |

---

## 3. 评测金字塔：从快到慢，从便宜到贵

```
                    ┌─────────┐
                    │ 人工评测  │  ← 最准但最贵最慢
                    │  (E2E)   │     每周 50 条，人工打分
                    ├─────────┤
                    │ LLM-as- │  ← 自动化但需要校验
                    │  Judge   │     每夜跑 Golden Case
                    ├─────────┤
                    │ 行为指标  │  ← 自动采集，CI 监控
                    │ (Metrics) │     Prometheus + Trace
                    ├─────────┤
                    │ Golden   │  ← 快反馈，每次提交跑
                    │  Case    │     已知正确答案的回归
                    ├─────────┤
                    │ 单元测试  │  ← 秒级，每行代码变更
                    │ (Unit)    │     单个函数/模块
                    └─────────┘
```

### 3.1 Layer 1：单元测试（秒级）

测**单个函数**的正确性。Agent 评测中，重点测这些：

```go
// 示例 1：测试意图识别
func TestTriageIntentClassification(t *testing.T) {
    cases := []struct{
        query    string
        expected string  // "alert_analysis" / "kb_qa" / "incident_analysis"
    }{
        {"paymentservice CPU 使用率 95%", "alert_analysis"},
        {"什么是 Kubernetes HPA？", "kb_qa"},
        {"日志里大量 context deadline exceeded", "incident_analysis"},
    }
    for _, tc := range cases {
        intent := classifyIntent(tc.query)
        if intent != tc.expected {
            t.Errorf("query=%q: expected %s, got %s", tc.query, tc.expected, intent)
        }
    }
}

// 示例 2：测试路由规则（ReAct vs Plan-Execute）
func TestAgentModeRouting(t *testing.T) {
    assert.Equal(t, ModePlanExecute, routeAgentMode("帮我排查为什么支付服务最近一周不稳定"))
    assert.Equal(t, ModeReAct,       routeAgentMode("MySQL max_connections 默认是多少"))
}
```

**Agent 特有测试重点：**
- 意图分类（Triage Agent）
- 路由规则
- Tool 参数解析（LLM 返回的 JSON 能不能正确解析）
- 降级逻辑（Mock 工具失败 → 验证降级路径）
- Prompt 模板渲染（变量注入对不对）

### 3.2 Layer 2：Golden Case 回归（分钟级）

给 Agent **已知正确答案的问题**，验证输出包含预期内容。

```go
type GoldenCase struct {
    Query          string   // "paymentservice CPU 使用率 95%"
    MustMention    []string // ["paymentservice", "CPU"]  ← 必须提到的关键词
    MustNotMention []string // ["数据库"]                  ← 不该提到的内容
    ExpectedTools  []string // ["query_metrics"]          ← 应该调用的工具
}

func TestGoldenCases(t *testing.T) {
    for _, gc := range loadGoldenCases() {
        result := runAgent(gc.Query)
        
        // 验证必须包含的关键词
        for _, must := range gc.MustMention {
            if !strings.Contains(result, must) {
                t.Errorf("missing key info: %s", must)
            }
        }
        
        // 验证不能包含的内容
        for _, forbidden := range gc.MustNotMention {
            if strings.Contains(result, forbidden) {
                t.Errorf("should not mention: %s", forbidden)
            }
        }
    }
}
```

**Golden Case 的来源：**
1. 从真实用户对话中提取典型问题
2. 手工编写边界 case（如 "你好"、"请帮我 rm -rf /"）
3. 历史上出过错的 case（Regression Bug → Golden Case）

### 3.3 Layer 3：行为指标（自动采集）

不测试"对不对"，而是监控**系统的行为模式**。

```
每次 Agent 执行自动记录：

┌─────────────────┬──────────┬──────────┐
│ 指标              │ 当前值    │ 告警阈值   │
├─────────────────┼──────────┼──────────┤
│ Agent 步数        │ avg=6.2  │ > 15     │
│ 工具调用成功率    │ 94%      │ < 85%    │
│ 降级触发率        │ 3%       │ > 10%    │
│ LLM 调用 P99 延迟 │ 3200ms   │ > 5000ms │
│ Token 消耗/task   │ avg=4200 │ > 8000   │
│ 空回答率          │ 0.5%     │ > 2%     │
└─────────────────┴──────────┴──────────┘
```

**实现方式：** 利用已有的 `QueryTrace` + Prometheus metrics。每次 Agent 执行完后，QueryTrace 的数据自动写入 Prometheus Counter/Histogram。

**为什么行为指标重要：** 你改了 Prompt，Golden Case 都过了——但 Agent 步数从 avg 6 涨到了 avg 9。行为指标能捕捉到这类"通过了测试但变慢了"的退化。

### 3.4 Layer 4：LLM-as-Judge（每夜跑）

用**一个更强的 LLM 当裁判**，对 Agent 的输出做多维度打分。

```
┌─────────────────────────────────────────────────────────┐
│                   LLM-as-Judge 流程                      │
│                                                         │
│  待评测 Agent ──→ 处理 Golden Case ──→ 输出诊断报告       │
│                                              │          │
│                                              ▼          │
│                                     Judge LLM (裁判)     │
│                                     阅读：                │
│                                     - 原始用户问题        │
│                                     - Agent 的诊断报告    │
│                                     - Ground Truth 参考   │
│                                              │          │
│                                              ▼          │
│                               ┌─────────────────────┐   │
│                               │ 正确性: 8/10         │   │
│                               │ 完整性: 7/10         │   │
│                               │ 逻辑性: 9/10         │   │
│                               │ 可操作性: 6/10       │   │
│                               └─────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Judge Prompt 模板：**

```
你是一个严格但公平的 AIOps 诊断报告评审专家。
请阅读以下材料，给出四维评分（每项 1-10 分）：

【用户问题】
{query}

【Agent 的诊断报告】
{agent_output}

【参考答案（供参考，非唯一标准）】
{reference}

评分标准：
- 正确性：诊断结论与问题相关，没有事实错误。有引用来源 +2 分。
- 完整性：覆盖了该问题的关键排查维度（指标/日志/知识库），没有明显遗漏。
- 逻辑性：推理链条清晰，从现象 → 分析 → 结论 → 建议，因果关系合理。
- 可操作性：给出的建议是具体可执行的操作步骤，而非泛泛而谈。

输出格式（严格遵守）：
正确性: X/10 - 简要理由
完整性: X/10 - 简要理由
逻辑性: X/10 - 简要理由
可操作性: X/10 - 简要理由
综合: X/10
```

**LLM-as-Judge 的风险 & 防护：**

| 风险 | 说明 | 解决方案 |
|------|------|---------|
| **裁判偏差 (Judge Bias)** | 用同一个模型做生成和裁判，会给自己的答案打高分 | 用**不同模型**当裁判 |
| **位置偏差** | Judge 倾向于给第一个看到的答案高分 | 随机化答案顺序 |
| **评分漂移** | 同一答案不同时间评分不同 | 定期校准——用人评和 Judge 评做一致性检查 |
| **"运维领域不懂"** | 通用 Judge 不理解运维上下文 | Prompt 里注入领域知识 |

### 3.5 Layer 5：人工评测（每周）

挑 50 条真实用户对话，让人工（你自己或团队）打分。这是**黄金标准**——所有自动化评测最终要和人评做校准。

---

## 4. 按 Agent 模式专项评测

### 4.1 ReAct Agent 评测要点

| 关注点 | 怎么测 | 好/坏的标准 |
|--------|--------|-----------|
| **工具选择** | Golden Case：验证调了正确的工具 | Agent 调了 `query_metrics` 而不是 `query_logs` 来查 CPU |
| **步数控制** | 行为指标：平均步数 | 简单问题 ≤ 3 步，复杂问题 ≤ 8 步 |
| **循环终止** | 边界 Case："请帮我解决世界和平" | Agent 应该在 2 步内给出 "无法解决" 而非无限循环 |
| **Observe 质量** | LLM-as-Judge：观察是否准确理解工具返回 | 工具返回了 JSON，Agent 正确提取了 CPU=95% |

### 4.2 Plan-Execute-Replan Agent 评测要点

| 关注点 | 怎么测 | 好/坏的标准 |
|--------|--------|-----------|
| **Plan 质量** | Golden Case：Planner 拆的步骤是否合理 | 拆出了"查指标→查日志→对比分析"三步，不缺不漏 |
| **Replan 触发** | 边界 Case：给一个错误线索让 Agent 走弯路 | Agent 发现第一步没找到证据 → 正确触发 Replan |
| **收敛性** | 行为指标：Replan 次数 | 平均 ≤ 2 次 Replan，最多 5 轮 |
| **计划修改质量** | LLM-as-Judge：新计划和旧计划的差异是否合理 | "加查网络层"而非"把时间从 15 分钟改成 20 分钟" |

### 4.3 Multi-Agent 编排评测要点

| 关注点 | 怎么测 | 好/坏的标准 |
|--------|--------|-----------|
| **Triage 准确率** | Golden Case：100 条 query 的意图分类 | > 90% 准确率 |
| **Specialist 并行度** | 行为指标：实际并行执行 vs 串行 | 不相关的 Specialist 应该并行执行 |
| **级联失败** | 边界 Case：一个 Specialist 超时，Supervisor 的行为 | Supervisor 继续编排其他 Specialist，不全局失败 |
| **Reporter 聚合质量** | LLM-as-Judge：报告是否整合了所有 Specialist 的发现 | 三个 Specialist 都查了，报告里都提到了 |

---

## 5. 业界 Agent 评测基准

> **面试加分项：知道这些 Benchmark，说明你关注 Agent 领域的学术进展。**

| Benchmark | 测什么 | 用什么指标 | OpsCaption 相关度 |
|-----------|--------|-----------|-----------------|
| **SWE-bench** | Agent 修 GitHub Issue 的能力 | % resolved | 🟡 低（修代码 ≠ 运维诊断） |
| **GAIA** | Agent 的多步推理 + 工具使用 | Pass@1 | 🟢 高（多步推理是核心能力） |
| **AgentBench** | 8 个维度（OS/DB/Web/Code 等） | Success Rate | 🟢 高（覆盖面最广的 Agent 评测） |
| **WebArena** | Agent 在真实网站上的操作 | Task Success | 🟡 中（Web 操作 ≠ 运维场景） |
| **ToolBench** | Agent 的工具调用能力 | Pass Rate | 🟢 高（工具调用是 OpsCaption 核心） |
| **τ-bench** | Agent 在真实世界任务上的可靠性 | Pass@1 × Cost | 🟢 高（质量和成本双重约束） |

### 5.1 为什么 OpsCaption 不太适合直接用这些 Benchmark

这些通用 Benchmark 测的是**通用 Agent 能力**——修代码、浏览网页、操作数据库。但 OpsCaption 的运维诊断场景有自己的特征：

- **领域专业性**：需要理解 PromQL、kubectl 命令、运维 Runbook——通用 Benchmark 不覆盖
- **证据引用**：诊断报告的价值在于"带证据"，而不是"给结论"——通用评测不看这个
- **降级容忍度**：在线系统对降级的容忍度和离线评测完全不同

**结论：** 通用 Benchmark 做**能力底座验证**（你的 Agent 基本推理过关吗？），领域 Case 做**业务价值验证**（你的 Agent 真能帮运维吗？）。

---

## 6. OpsCaption 的评测实践（当前 + 规划）

### 6.1 已有的

| 能力 | 方法 | 状态 |
|------|------|------|
| RAG 检索质量 | Recall@K（AIOps Challenge 2025 案例集） | ✅ 78% |
| Triage 意图分类 | 规则表 + 抽样验证 | ⚠️ 缺自动化 |
| 行为指标 | ContextTrace（每次请求上下文装配全链路追踪） | ✅ |
| 安全防护 | Prompt Guard + Output Filter 规则 | ✅ |
| Agent 输出质量 | Contract Schema Gate（EnforceContract 运行时校验） | ✅ |
| Agent 评测框架 | eval.MultiAgentRunner（Golden Case 端到端跑） | ✅ 框架就绪 |
| A/B 对比 | JudgeResult + BaselineScores vs CandidateScores | ✅ 类型定义完成 |

### 6.2 实际评测代码结构

```go
// internal/ai/agent/eval/types.go
// DiagCase = 一个诊断测试用例
type DiagCase struct {
    ID              string   // "diag-001"
    Query           string   // "paymentservice CPU 使用率 95%"
    ExpectedIntent  string   // "alert_analysis"
    ExpectedDomains []string // ["metrics", "knowledge"]
    MustMention     []string // ["paymentservice", "CPU"]
    MustNotMention  []string // ["数据库连接问题"]
    Severity        string   // "high"
}

// DiagScores = LLM-as-Judge 四维打分（每项 1-5 分）
type DiagScores struct {
    Correctness   int    // 正确性
    Completeness  int    // 完整性
    Coherence     int    // 逻辑性
    Actionability int    // 可操作性
    Overall       int    // 综合分
    Comments      string // 裁判备注
}

// JudgeResult = A/B 对比结果
type JudgeResult struct {
    CaseID          string
    BaselineScores  DiagScores
    CandidateScores DiagScores
    Delta           DiagScores  // 变化量——正数 = 候选更优
}

// MultiAgentRunner = 端到端跑评测
// 它注册全部 Agent（supervisor + triage + metrics + logs + knowledge + reporter），
// 通过 Runtime.Dispatch 跑完整编排，返回 summary + intent + domains
```

**面试时怎么说：**

> "评测框架已经就绪——`DiagCase` 定义了测试用例的结构，`DiagScores` 定义了四维打分的结构，`MultiAgentRunner` 可以通过完整的 Agent Runtime 跑端到端测试。A/B 对比用 `JudgeResult` 结构——把 baseline 和 candidate 的打分求 delta，快速看到改了 Prompt 后哪些维度进步了、哪些退步了。"

### 6.3 规划中的

| 能力 | 方法 | 优先级 |
|------|------|--------|
| Golden Case 回归 | 10 个典型运维问题 + 预期输出 | P0 |
| Agent 步数监控 | Prometheus Counter + Grafana Dashboard | P0 |
| LLM-as-Judge 每夜跑 | DeepSeek V3 做裁判，10 个 Golden Case | P1 |
| 人工评测闭环 | 每周 20 条，人评和 Judge 一致性 > 80% | P1 |

---

## 7. 面试问答

### Q1: "你怎么评价你的 Agent 做得对不对？"

> 我有四层评测。**第一层，Contract Schema Gate**——每个 Agent 的输出在返回前都要经过 `EnforceContract()` 校验，检查摘要非空、降级原因必填、输出不越界。这是**代码层面的确定性检查**，不依赖 LLM 的理解。**第二层，Golden Case 回归**——用 `DiagCase` 定义 10 个典型运维问题，`MultiAgentRunner` 通过完整 Agent Runtime 端到端跑，验证输出包含预期关键词、调了正确的工具、没有胡说八道。**第三层，行为指标监控**——每次 Agent 执行后自动采集步数、工具调用成功率、LLM 延迟、token 消耗，Prometheus 做趋势告警。**第四层，LLM-as-Judge + 人工抽检**——用 `DiagScores` 四维打分（正确性/完整性/逻辑性/可操作性），`JudgeResult` 做 A/B 对比看 delta，每周人工抽查 20 条做一致性校验。
>
> 另外 RAG 层面有独立的 Recall@K 评测——当前 Recall@10 = 78%，用 AIOps Challenge 2025 案例集，build/holdout split 严格分开。

### Q2: "LLM-as-Judge 你怎么保证裁判打分是公正的？"

> 三个措施。第一，**不同模型当裁判**——不用 DeepSeek V3 给自己打分，用另一个模型或不同 temperature 做裁判。第二，**人工一致性校验**——每周随机抽 20 条，同时让人和 Judge 打分，计算 Spearman 相关系数，低于 0.7 就调整 Judge Prompt。第三，**分维度打分而非总分**——正确性、完整性、逻辑性、可操作性分开评，比"总分 8 分"有区分度——因为 AIOps 场景中正确性权重远高于其他维度。

### Q3: "你的评测集怎么保证不泄露？"

> **build split 和 eval split 严格分开**——按案例切分，不是按文档随机切。同一个系统（如 checkoutservice）的文档和故障案例不会同时出现在 build 和 eval 里。评测集的 ground truth 是人工标注的，没有直接喂给检索链路——AGENTS.md 里明确写了"不要把 groundtruth 标签直接当实时证据喂给模型"。

---

## 8. 自测

### 问题 1

Agent 评测和传统软件评测最大的三个区别是什么？

<details>
<summary>点击查看答案</summary>

1. **非确定性**：同一个输入，LLM 可能给出不同的输出
2. **多路径正确**：不同的执行流程可能都是对的
3. **开放式输出**：诊断报告没有唯一标准答案，需要多维度评分
</details>

### 问题 2

评测金字塔的 5 层分别是什么？哪一层最快、哪一层最准？

<details>
<summary>点击查看答案</summary>

```
单元测试 → 秒级，最快，但只测局部
Golden Case → 分钟级，测端到端
行为指标 → 自动采集，不看对错看模式
LLM-as-Judge → 每夜跑，自动化质量评分
人工评测 → 每周，最准最贵
```

金字塔越往上越准越贵，越往下越快越便宜。
</details>

### 问题 3

如果 Agent 改了 Prompt 后，Golden Case 全部通过但 Agent 步数从 avg 6 涨到了 avg 9，这说明什么？

<details>
<summary>点击查看答案</summary>

这说明 **Prompt 改得"更啰嗦了"**。Agent 仍然能完成任务（Golden Case 过），但效率下降了（步数增加）。如果只看 Golden Case 不看行为指标，这个退化就被漏掉了。

**解决方案：** 分析涨的那 3 步是什么——如果是多调了一次工具，检查 Prompt 是否引导了不必要的工具调用；如果是多了一轮 Think，检查 Prompt 是否让 LLM 过度反思。回退 Prompt 或在评测里加上步数上限约束。
</details>

---

> 📌 **关联文档：**
> - RAG 评测实践：[07-rag-recall-eval-guide.md](../07-rag-recall-eval-guide.md)、[08-rag-eval-runbook.md](../08-rag-eval-runbook.md)
> - 多智能体测试方案：[architecture/多智能体优化测试方案.md](../architecture/多智能体优化测试方案.md)
> - RAG 检索系统：[04-RAG检索系统.md](./04-RAG检索系统.md)（Section 5.6 评测指标）
> - 第10章全真模拟面试：[10-全真模拟面试.md](./10-全真模拟面试.md)（问题4：RAG 评测三层面）
