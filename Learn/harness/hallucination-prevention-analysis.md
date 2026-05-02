# OpsCaptain 防幻觉体系分析

> 日期：2026-05-02
> 状态：✅ P0-P2 全部落地，5 个缺口已修复 3 个

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

## 二、现有防线（已落地）

### 2.1 Prompt 铁律（软约束） ✅

**位置**：`internal/ai/agent/chat_pipeline/prompt.go`

**机制**：system prompt 中明确要求模型不编造数据：

```
## 工具使用铁律
- 你拥有查询 Prometheus 告警、搜索日志、检索内部知识库等工具。这些工具已经就绪。
- 运维相关问题必须先调用工具获取实际数据，再基于数据回答。
- 绝对不要说"我无法查询"、"我没有能力访问"。
- 回答要基于工具返回的真实数据，不要编造指标、日志或告警信息。

## 输出引用规则
- 每个结论必须关联一个工具返回的具体数据
- 格式：[来源: 工具名] 数据内容 → 结论
- 如果没有工具数据支持，必须标注"推测"或"待确认"

## 工具失败处理
- 工具返回错误时，必须告知用户具体的错误信息
- 不要用通用建议替代实际数据
- 格式："XX 工具返回错误：具体错误信息"
```

**覆盖幻觉**：编造数据、无中生有、过度自信

**强度**：中。软约束，依赖 LLM 自觉遵守。配合工具闭环效果好。

### 2.2 注入检测（硬拦截） ✅

**位置**：`utility/safety/prompt_guard.go` + `internal/controller/chat/prompt_guard.go`

**机制**：6 个正则模式拦截 prompt injection。

**强度**：高。正则匹配，不依赖 LLM。

### 2.3 输出过滤（安全过滤） ✅

**位置**：`utility/safety/output_filter.go`

**机制**：5 个正则过滤器（密钥、内部 IP、system prompt 泄露）。

**强度**：中。安全过滤，不是准确性过滤。

### 2.4 Degradation 降级系统（硬约束） ✅

**位置**：`internal/ai/service/degradation.go`

**机制**：全局 Kill Switch + 三态结果模型 + 15+ 降级点 + Contract 校验。

**强度**：高。只覆盖 AIOps 多 agent 路径。

### 2.5 Memory 验证（多层防御） ✅

**位置**：`utility/mem/extraction.go`

**机制**：4 层过滤（格式校验、运维指标匹配、LLM 提议校验、上下文注入过滤）。

**强度**：高。

### 2.6 Context 预算管理 ✅

**位置**：`internal/ai/contextengine/assembler.go`

**机制**：ContextBudget 按 history / memory / docs / tools 分配 token 上限。

**强度**：高。

### 2.7 证据结构化 ✅

**位置**：`internal/ai/protocol/types.go`

**机制**：`EvidenceItem` 结构记录来源、标题、相关性分数。

**强度**：中。

### 2.8 工具失败格式化 ✅ 新增

**位置**：`internal/ai/events/tool_wrapper.go`

**机制**：ToolWrapper 工具失败时返回格式化字符串而非 Go error：

```
[工具调用失败] tool_name: error
请告知用户该工具返回了错误，不要用通用建议替代实际数据。
```

**覆盖幻觉**：编造数据（工具失败后）

**强度**：高。格式化字符串让 LLM 明确知道失败，Go error 会触发 eino 重试。

### 2.9 输出指标来源校验 ✅ 新增

**位置**：`internal/ai/events/output_validator.go`

**机制**：正则提取输出中的数值指标（如 "P99: 2.3s"），对比工具结果中是否有来源。无来源时产生 warning 日志。

**覆盖幻觉**：编造数据

**强度**：中。正则提取，覆盖常见指标格式。

### 2.10 未调用工具检测 ✅ 新增

**位置**：`internal/controller/chat/chat_v1_chat_stream.go`

**机制**：`isOpsRelatedQuery` 判断是否为运维相关问题（30+ 关键词）。运维问题无工具调用时记录 hallucination risk 警告。

**覆盖幻觉**：无中生有（不调工具直接编造）

**强度**：中。关键词匹配，不覆盖同义词。

### 2.11 Contract 校验 ✅ 新增

**位置**：`internal/ai/events/contract.go`

**机制**：5 条规则校验输出合规性：

| 规则 | 等级 | 说明 |
|------|------|------|
| must_use_tools | VIOLATION | 运维问题必须有工具调用 |
| must_report_failure | VIOLATION | 工具失败必须在输出中提及 |
| should_reference_data | WARN | 有工具结果时应引用数据 |
| no_fabricated_alerts | VIOLATION | 无工具数据时不得编造告警 |
| should_hedge | WARN | 无数据支持时应使用置信度标注 |

**覆盖幻觉**：编造数据、过度自信、错误归因

**强度**：高。硬规则 + 软警告结合。

### 2.12 Schema Gate ✅ 新增

**位置**：`internal/ai/events/schema_gate.go`

**机制**：结构化输出校验引擎：

| 字段 | 必须 | 说明 |
|------|------|------|
| has_answer | ✅ | 输出必须包含实质性回答（>10 字） |
| no_contradiction | ❌ | 输出不应自相矛盾（如"正常"+"异常"） |
| actionable | ❌ | 描述问题时应提供可操作建议 |

**覆盖幻觉**：过度自信、遗漏关键信息

**强度**：中。规则引擎，可扩展。

---

## 三、关键缺口（修复状态）

### 缺口 1：Chat ReAct 路径无结构化降级

**风险等级**：🔴 高

**状态**：✅ 已修复

**修复方式**：
1. ToolWrapper 工具失败返回格式化字符串（`[工具调用失败]`）
2. Prompt 铁律要求告知用户错误
3. Contract 校验 `must_report_failure` 规则

### 缺口 2：无生成后事实校验

**风险等级**：🔴 高

**状态**：✅ 已修复

**修复方式**：
1. `ValidateOutputAgainstToolResults` — 指标来源校验
2. `ValidateContract` — 合规校验
3. `SchemaGate.Validate` — 结构化校验

### 缺口 3：无强制引用

**风险等级**：🟡 中

**状态**：✅ 已修复

**修复方式**：Prompt 输出引用规则 + Contract `should_reference_data` 规则

### 缺口 4：输出过滤只防安全不防准确性

**风险等级**：🟡 中

**状态**：⚠️ 部分修复

**已覆盖**：指标来源校验、告警编造检测

**未覆盖**：遗漏信息检测（工具返回 3 个告警只提 1 个）

### 缺口 5：ReAct 路径无 Contract 校验

**风险等级**：🟡 中

**状态**：✅ 已修复

**修复方式**：`ValidateContract` 5 条规则 + `SchemaGate` 3 条规则

---

## 四、防线全景图（更新后）

```
用户输入
  │
  ├─ [硬] 注入检测 ─────────────────── 拦截 prompt injection
  │
  ▼
ReAct Agent
  │
  ├─ [硬] 工具调用
  │   ├─ [硬] BeforeToolCall 拦截（ToolWrapper）
  │   ├─ [硬] 工具执行
  │   ├─ [硬] AfterToolCall 结果处理（ToolWrapper）
  │   └─ [硬] 工具失败 → 格式化字符串 ✅
  │       └─ "[工具调用失败] tool_name: error\n请告知用户..."
  │
  ├─ [软] System Prompt 铁律 ✅
  │   ├─ 工具使用铁律：必须先调工具
  │   ├─ 输出引用规则：结论必须关联工具数据
  │   ├─ 工具失败处理：告知用户，不用通用建议替代
  │   └─ 置信度标注：推测/待确认
  │
  ▼
LLM 输出
  │
  ├─ [硬] 输出过滤（安全）
  │   ├─ 密钥泄露
  │   ├─ 内部 IP
  │   └─ System prompt 泄露
  │
  ├─ [硬] 输出校验 ✅
  │   ├─ 指标来源校验（无来源 → warning）
  │   ├─ 未调用工具检测（运维问题无工具 → hallucination risk）
  │   └─ 置信度词检测（推测/可能 → info 日志）
  │
  ├─ [硬] Contract 校验 ✅
  │   ├─ must_use_tools：运维问题必须有工具调用
  │   ├─ must_report_failure：工具失败必须提及
  │   ├─ should_reference_data：应引用工具数据
  │   ├─ no_fabricated_alerts：不得编造告警
  │   └─ should_hedge：无数据时应标注推测
  │
  ├─ [硬] Schema Gate ✅
  │   ├─ has_answer：输出必须有实质内容
  │   ├─ no_contradiction：不应自相矛盾
  │   └─ actionable：问题描述应有建议
  │
  ▼
用户看到的回答
```

---

## 五、改进优先级（更新后）

| 优先级 | 措施 | 状态 |
|--------|------|------|
| P0 | ReAct 路径工具失败注入系统消息 | ✅ done |
| P0 | Prompt 强制引用 | ✅ done |
| P1 | 输出关键指标来源校验 | ✅ done |
| P1 | ReAct 路径轻量 Contract | ✅ done |
| P2 | Schema Gate | ✅ done |
| P2 | LLM 遗漏信息检测 | ✅ done |
| P3 | LLM 准确性过滤层 | ✅ done |

**剩余改进**：
- Contract 阻断模式（误报率确认后升级为拦截）
- 前端展示 Contract/Schema Gate 结果

---

## 六、面试回答

### Q1：你的 AIOps 系统怎么防幻觉？

> 我们有 12 层防线，按从输入到输出的顺序：
>
> 第一层是**注入检测**，用正则拦截 prompt injection，这是硬拦截。
>
> 第二层是**工具调用闭环**。运维问题必须先调用工具获取数据，工具失败时返回格式化字符串（`[工具调用失败]`），让 LLM 明确知道失败。这是最强的防线——在系统层面阻断"没有数据就编造"的路径。
>
> 第三层是**Prompt 铁律**。明确告诉模型"你有工具，用它"、"不要编造指标"、"不确定时标注推测"。
>
> 第四层是 **Contract 校验**。5 条规则检查输出合规性：必须调工具、必须报告失败、必须引用数据、不得编造告警、无数据时标注推测。
>
> 第五层是 **Schema Gate**。结构化校验输出质量：必须有实质内容、不应自相矛盾、问题描述应有建议。
>
> 第六层是**输出指标校验**。正则提取输出中的数值指标，对比工具结果中是否有来源。
>
> 第七层是**未调用工具检测**。运维问题但无工具调用时记录幻觉风险警告。
>
> 后面还有 Memory 4 层验证、Context 预算管理、输出安全过滤、证据结构化等防线。

### Q2：Contract 校验和 Schema Gate 有什么区别？

> Contract 校验关注**事实准确性**——输出中的数据是否来自工具、是否有编造。
>
> Schema Gate 关注**输出质量**——回答是否有实质内容、是否自相矛盾、是否有可操作建议。
>
> 两者互补：Contract 防止"说假话"，Schema Gate 防止"说废话"。

### Q3：这些校验会不会影响响应速度？

> 所有校验都在响应完成后异步执行，不阻塞用户看到回答。
>
> 校验结果只记录日志（warning/info），不修改输出。这是"观测不干预"的设计——先收集数据，确认误报率后再考虑是否阻断。
>
> 如果未来误报率低到可以接受，可以把 Contract 的 VIOLATION 级别升级为阻断——在输出前拦截，返回"数据不足，无法给出可靠回答"。

### Q4：Prompt 铁律真的有用吗？

> 你说得对，prompt 是软约束。我们不依赖 prompt 作为唯一防线。
>
> 真正的硬约束是**工具调用闭环 + Contract 校验**。当用户问运维相关问题时，系统要求必须先调用工具。如果工具失败，格式化字符串让 LLM 明确知道。如果 LLM 还是编造了数据，Contract 校验会在事后捕获并记录。
>
> 就像交通规则和护栏的关系。Prompt 是交通规则（告诉你不能超速），工具闭环是护栏（你超速了会撞上护栏，不会冲下悬崖），Contract 校验是事故记录仪（记录违规行为用于事后分析）。

---

## 七、参考源码

### 防线实现

| 文件 | 说明 |
|------|------|
| `internal/ai/agent/chat_pipeline/prompt.go` | System prompt 铁律 + 输出引用规则 |
| `utility/safety/prompt_guard.go` | 注入检测 |
| `utility/safety/output_filter.go` | 输出过滤 |
| `internal/ai/service/degradation.go` | 降级系统 |
| `utility/mem/extraction.go` | Memory 验证 |
| `internal/ai/contextengine/assembler.go` | Context 预算 |
| `internal/ai/protocol/types.go` | EvidenceItem 结构 |
| `internal/ai/agent/contracts/enforce.go` | Contract 校验（AIOps 路径） |
| `internal/ai/events/tool_wrapper.go` | 工具失败格式化 + before/after hook |
| `internal/ai/events/output_validator.go` | 输出指标来源校验 |
| `internal/ai/events/contract.go` | ReAct 路径 Contract 校验 |
| `internal/ai/events/schema_gate.go` | Schema Gate 结构化校验 |
| `internal/ai/events/result_collector.go` | 工具结果收集器 |
| `internal/ai/events/llm_validator.go` | LLM 高级校验（遗漏检测 + 准确性校验） |
| `internal/ai/events/hallucination_config.go` | 防幻觉配置加载 |
| `internal/controller/chat/chat_v1_chat_stream.go` | 校验集成点 |

### 测试覆盖

| 文件 | 用例数 |
|------|--------|
| `events/contract_test.go` | 11 |
| `events/schema_gate_test.go` | 6 |
| `events/output_validator_test.go` | 5 |
| `events/llm_validator_test.go` | 8 |
| `events/sequence_test.go` | 6 |
| `events/replay_test.go` | 4 |
| `events/tool_wrapper_test.go` | 10 |
| `events/health_collector_test.go` | 7 |
| `events/events_test.go` | 5 |

---

## 八、下一步

1. **Contract 阻断模式**：误报率确认后，VIOLATION 级别可升级为输出拦截
2. **前端展示**：Contract/Schema Gate 结果可通过 SSE 推送到前端，让用户看到可信度标签
3. **LLM 校验优化**：根据实际误报率调整 prompt 和阈值
