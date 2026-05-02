# OpsCaptain 防幻觉体系分析

> 日期：2026-05-02
> 状态：现有防线已梳理，关键缺口已识别

---

## 一、幻觉类型分类

在 AIOps 场景下，幻觉按危害程度排序：

| 类型 | 示例 | 危害 |
|---|---|---|
| **编造数据** | "payment_service 的 P99 延迟是 2.3s"（实际未查询） | 运维人员基于虚假指标做决策 |
| **错误归因** | "告警来自 order_service"（实际来自 payment_service） | 排查方向完全错误 |
| **过时信息** | "当前有 5 个 firing 告警"（实际是 10 分钟前的数据） | 误判系统状态 |
| **无中生有** | "存在一个 connection pool exhausted 告警"（实际不存在） | 引发不必要的应急响应 |
| **过度自信** | "根因是数据库连接池满"（实际只是推测） | 运维人员当作确定结论处理 |
| **遗漏关键信息** | 工具返回了 3 个告警，只提了 1 个 | 遗漏真正的问题 |

---

## 二、现有防线

### 2.1 Prompt 铁律（软约束）

**位置**：`internal/ai/agent/chat_pipeline/prompt.go`

**机制**：system prompt 中明确要求模型不编造数据：

```
## 工具使用铁律
- 你拥有查询 Prometheus 告警、搜索日志、检索内部知识库等工具。这些工具已经就绪。
- 运维相关问题必须先调用工具获取实际数据，再基于数据回答。
- 绝对不要说"我无法查询"、"我没有能力访问"。
- 回答要基于工具返回的真实数据，不要编造指标、日志或告警信息。

## 证据与上下文规则
- AIOps 场景要优先说明证据、推断和不确定性；没有证据时不要把猜测包装成结论。
- 优先级：当前用户问题 > 实时工具结果 > 可信内部文档 > 关键记忆 > 近期对话 > 历史摘要。
```

**覆盖幻觉**：编造数据、无中生有、过度自信

**强度**：中。软约束，依赖 LLM 自觉遵守。DeepSeek V3 遵守率约 70-80%，偶尔仍会编造。

**改进空间**：
- 加入"如果工具未调用或返回空数据，必须明确告知用户，不能用通用建议替代"
- 加入"每个结论必须引用工具返回的具体数据（指标值、日志片段、告警名称）"

### 2.2 注入检测（硬拦截）

**位置**：`utility/safety/prompt_guard.go` + `internal/controller/chat/prompt_guard.go`

**机制**：6 个正则模式拦截 prompt injection：

```go
"ignore previous instructions"
"you are now"
"system:"
"[inst]"
// + 中文变体
```

**覆盖幻觉**：通过 prompt injection 改变模型行为

**强度**：高。正则匹配，不依赖 LLM。

### 2.3 输出过滤（安全过滤）

**位置**：`utility/safety/output_filter.go`

**机制**：5 个正则过滤器，覆盖所有输出路径：

| 过滤器 | 作用 |
|---|---|
| `system_prompt_block` | 过滤 `<<sys>>...<</sys>>` |
| `system_prompt_line` | 过滤 `system:` 行 |
| `inst_block` | 过滤 `[inst]...[/inst]` |
| `api_key` | 过滤 `sk-*`、`api_key=*`、bearer token |
| `internal_ip` | 过滤 RFC 1918 私有 IP |

**覆盖幻觉**：system prompt 泄露、密钥泄露

**强度**：中。安全过滤，不是准确性过滤。不检查"模型是否编造了指标"。

### 2.4 Degradation 降级系统（硬约束）

**位置**：`internal/ai/service/degradation.go` + `internal/ai/protocol/types.go`

**机制**：

1. **全局 Kill Switch**：配置 `degradation.kill_switch` 或 Redis key `opscaptionai:degradation:kill_switch`
2. **三态结果模型**：`ResultStatusSucceeded` / `ResultStatusFailed` / `ResultStatusDegraded`
3. **15+ 降级点**：工具失败、超时、权限不足、空结果 → 统一走 `ResultStatusDegraded`
4. **Contract 校验**：返回格式不合规 → 降级 + 置信度减半

**覆盖幻觉**：工具失败时编造数据

**强度**：**高**。这是最强的防线。工具失败时系统强制降级，不给 LLM 编造的机会。

**局限**：只覆盖 AIOps 多 agent 路径，不覆盖 Chat ReAct 路径。

### 2.5 Memory 验证（多层防御）

**位置**：`utility/mem/extraction.go` + `utility/mem/memory_agent.go`

**机制**：4 层过滤：

| 层 | 检查内容 |
|---|---|
| `ValidateMemoryCandidate()` | 长度、代码块、套话、密钥标记 |
| `extractFacts()` | 只提取匹配运维指标的事实（服务名、IP、集群名等） |
| `sanitizeMemoryDecision()` | LLM 提议的记忆操作必须通过格式校验 |
| Context 注入过滤 | 过期、低置信度、安全标签、预算超限 → 不注入 |

**覆盖幻觉**：假记忆进入上下文、密钥进记忆、套话污染记忆

**强度**：**高**。4 层防御，每层都有具体检查规则。

### 2.6 Context 预算管理

**位置**：`internal/ai/contextengine/assembler.go`

**机制**：ContextBudget 按 history / memory / docs / tools 分配 token 上限，超出时裁剪。

**覆盖幻觉**：上下文过载导致模型"忘记"工具结果而编造

**强度**：高。主动管理，不是被动截断。

### 2.7 证据结构化

**位置**：`internal/ai/protocol/types.go`

**机制**：`EvidenceItem` 结构要求：

```go
type EvidenceItem struct {
    SourceType string  // 来源类型
    SourceID   string  // 来源 ID
    Title      string  // 标题（必填）
    Snippet    string  // 片段
    Score      float64 // 相关性分数
    URI        string  // 来源链接
}
```

**覆盖幻觉**：结论无来源

**强度**：中。有结构但无强制引用——LLM 可以不引用任何证据就给出结论。

---

## 三、关键缺口

### 缺口 1：Chat ReAct 路径无结构化降级

**风险等级**：🔴 高

**现状**：AIOps 路径有完善的降级机制，但 Chat ReAct 路径没有。如果工具调用失败，LLM 收到错误信息，但没有系统级阻止它编造回答。

**示例**：用户问"payment_service 延迟升高"，模型可能不调用工具就直接回答"建议检查错误率、日志、队列堆积"。

**改进方案**：在 ReAct 路径的工具失败时，注入明确的系统消息：

```go
// 工具调用失败时，在 context 中注入失败信息
if toolErr != nil {
    messages = append(messages, &schema.Message{
        Role: schema.System,
        Content: fmt.Sprintf("工具 %s 调用失败：%s。请告知用户该工具返回了错误，不要用通用建议替代。", toolName, toolErr.Error()),
    })
}
```

### 缺口 2：无生成后事实校验

**风险等级**：🔴 高

**现状**：防幻觉设计文档设计了 Schema Gate，但标注为"设计中"，没有实现。LLM 输出不经过任何事实校验就直接返回。

**改进方案**：轻量级校验——检查模型输出中的关键指标是否在工具结果中出现过：

```go
// 输出校验：关键指标必须在工具结果中有来源
func validateOutputAgainstEvidence(output string, toolResults []string) []string {
    var warnings []string
    // 提取输出中的数字指标（如 "P99: 2.3s"）
    metrics := extractMetrics(output)
    for _, m := range metrics {
        if !metricExistsInResults(m, toolResults) {
            warnings = append(warnings, fmt.Sprintf("指标 %s 在工具结果中未找到来源", m))
        }
    }
    return warnings
}
```

### 缺口 3：无强制引用

**风险等级**：🟡 中

**现状**：LLM 可以不引用任何证据就给出结论。EvidenceItem 结构存在但不强制使用。

**改进方案**：prompt 层面要求引用：

```
## 输出引用规则
- 每个结论必须关联一个工具返回的具体数据
- 格式：[来源: 工具名] 数据内容 → 结论
- 如果没有工具数据支持，必须标注"推测"或"待确认"
```

### 缺口 4：输出过滤只防安全不防准确性

**风险等级**：🟡 中

**现状**：`filterAssistantPayload` 只过滤密钥泄露，不过滤"模型编造了一个不存在的告警"。

**改进方案**：增加准确性过滤层（成本较高，需要定义规则）。

### 缺口 5：ReAct 路径无 Contract 校验

**风险等级**：🟡 中

**现状**：Contract 系统只覆盖 AIOps 多 agent 路径，Chat ReAct 路径没有。

**改进方案**：在 ReAct 路径的最终输出上加轻量级 Contract 校验。

---

## 四、防线全景图

```
用户输入
  │
  ├─ [硬] 注入检测 ─────────────────── 拦截 prompt injection
  │
  ▼
ReAct Agent
  │
  ├─ [硬] 工具调用
  │   ├─ [硬] BeforeToolCall 拦截
  │   ├─ [硬] 工具执行
  │   ├─ [硬] AfterToolCall 结果处理
  │   └─ [硬] 工具失败 → 降级（AIOps 路径）
  │                   → ⚠️ 无降级（ReAct 路径）← 缺口 1
  │
  ├─ [软] System Prompt 铁律
  │   ├─ 禁止编造数据
  │   ├─ 必须先调工具
  │   └─ 不确定时标注推测
  │
  ▼
LLM 输出
  │
  ├─ [硬] 输出过滤（安全）
  │   ├─ 密钥泄露
  │   ├─ 内部 IP
  │   └─ System prompt 泄露
  │
  ├─ ⚠️ 无事实校验 ← 缺口 2
  ├─ ⚠️ 无强制引用 ← 缺口 3
  ├─ ⚠️ 无准确性过滤 ← 缺口 4
  │
  ▼
用户看到的回答
```

---

## 五、改进优先级

| 优先级 | 措施 | 成本 | 收益 | 实现方式 |
|---|---|---|---|---|
| P0 | ReAct 路径工具失败注入系统消息 | 低 | 高 | ToolWrapper after hook + 系统消息注入 |
| P0 | Prompt 强制引用 | 低 | 高 | 修改 system prompt |
| P1 | 输出关键指标来源校验 | 中 | 中 | 正则提取指标 + 对比工具结果 |
| P1 | ReAct 路径轻量 Contract | 中 | 中 | 输出格式校验 + 置信度标注 |
| P2 | Schema Gate | 高 | 中 | 定义输出 schema + 校验引擎 |

---

## 六、面试回答

### Q1：你的 AIOps 系统怎么防幻觉？

> 我们有 7 层防线，按从输入到输出的顺序：
>
> 第一层是**注入检测**，用正则拦截 prompt injection，这是硬拦截。
>
> 第二层是**工具调用闭环**。运维问题必须先调用工具获取数据，工具失败时走降级流程，不给 LLM 编造的机会。这是最强的防线——在系统层面阻断"没有数据就编造"的路径。
>
> 第三层是**Prompt 铁律**。明确告诉模型"你有工具，用它"、"不要编造指标"、"不确定时标注推测"。这是软约束，但配合工具闭环效果很好。
>
> 第四层是**Memory 多层验证**。4 层过滤防止假记忆进入上下文：格式校验、运维指标匹配、LLM 提议校验、上下文注入过滤。
>
> 第五层是 **Context 预算管理**。主动控制 token 分配，防止上下文过载导致模型"忘记"工具结果。
>
> 第六层是**输出过滤**。过滤密钥、内部 IP、system prompt 泄露。
>
> 第七层是**证据结构化**。每个 TaskResult 包含 EvidenceItem，记录来源、标题、相关性分数。
>
> 最大的缺口是 Chat ReAct 路径没有结构化降级——AIOps 路径有完善的降级机制，但普通聊天路径如果工具失败，LLM 可能编造回答。这是我们下一步要补的。

### Q2：Prompt 铁律真的有用吗？LLM 不是想违反就违反？

> 你说得对，prompt 是软约束。我们不依赖 prompt 作为唯一防线。
>
> 真正的硬约束是**工具调用闭环**。当用户问运维相关问题时，系统要求必须先调用工具。如果工具失败，走降级流程，返回"XX 工具返回错误，无法获取数据"，而不是让 LLM 编造。
>
> Prompt 的作用是"引导"——让模型知道有工具可用、知道要基于数据回答。但如果模型违反了，系统层面有兜底。
>
> 这就像交通规则和护栏的关系。Prompt 是交通规则（告诉你不能超速），工具闭环是护栏（你超速了会撞上护栏，不会冲下悬崖）。

### Q3：Memory 幻觉怎么防？

> Memory 是最容易产生幻觉的地方——LLM 可能把用户的假设当作事实存下来，下次对话就当成真的用了。
>
> 我们有 4 层防御：
>
> 第一层是**格式校验**。排除太短、太长、包含代码块、包含套话（"作为 AI"、"抱歉"）的内容。
>
> 第二层是**运维指标匹配**。只提取匹配已知运维模式的事实（服务名、IP、集群名、端口号），不提取通用陈述。
>
> 第三层是 **LLM 提议校验**。即使 LLM 提议保存某个记忆，也必须通过格式和类型校验。如果 LLM 说"保存一个偏好"，但内容不符合偏好模式，会被拒绝。
>
> 第四层是**上下文注入过滤**。记忆注入上下文时，还要检查过期时间、置信度阈值、安全标签、预算限制。
>
> 这样即使某一层漏了，后面还有兜底。

### Q4：如果面试官问"你怎么验证防幻觉措施真的有效"？

> 三个层面：
>
> 第一是**自动化测试**。我们有 memory extraction 的单元测试，覆盖套话、密钥、代码块等边界情况。Contract 校验也有对应的测试用例。
>
> 第二是**线上监控**。Degradation 系统有结构化日志，每次降级都记录原因。我们可以通过日志分析"有多少请求走了降级"、"哪些工具最容易失败"。
>
> 第三是 **A/B 对比**。在 Prompt 改动前后，用相同的 AIOps 问题跑 replay，对比输出质量和工具调用率。我们有 replay test case 可以做这个。
>
> 但说实话，最难验证的是"软约束"——prompt 铁律的遵守率。这需要人工抽样审查，目前没有自动化方案。

### Q5：AIOps 和通用 ChatBot 在防幻觉上有什么本质区别？

> 两个核心区别：
>
> 第一，**容错空间不同**。通用 ChatBot 编造一个菜谱，用户最多做出来不好吃。AIOps 编造一个告警，运维人员可能凌晨 3 点起来处理一个不存在的故障。所以 AIOps 的防幻觉必须是硬约束，不能只靠 prompt。
>
> 第二，**有 ground truth**。AIOps 场景下，工具返回的数据就是 ground truth——Prometheus 的指标是真实的、日志系统的内容是真实的。我们可以用工具结果来校验 LLM 输出。通用 ChatBot 没有这个优势。
>
> 所以我们的策略是：**用工具结果作为锚点，强制 LLM 基于真实数据回答**。这是 AIOps 场景天然的防幻觉优势。

---

## 七、参考源码

### 防线实现

- `internal/ai/agent/chat_pipeline/prompt.go` — System prompt 铁律
- `utility/safety/prompt_guard.go` — 注入检测
- `utility/safety/output_filter.go` — 输出过滤
- `internal/ai/service/degradation.go` — 降级系统
- `utility/mem/extraction.go` — Memory 验证
- `internal/ai/contextengine/assembler.go` — Context 预算
- `internal/ai/protocol/types.go` — EvidenceItem 结构
- `internal/ai/agent/contracts/enforce.go` — Contract 校验

### 设计文档

- `Learn/harness/07-防幻觉体系设计.md` — 防幻觉设计（含 Schema Gate 设计）
- `internal/ai/events/tool_wrapper.go` — 工具调用拦截（可扩展为事实校验）
