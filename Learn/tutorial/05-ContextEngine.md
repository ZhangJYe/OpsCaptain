# 第 5 章：Context Engine — \"给 LLM 装上智能编辑器\"

> **本章目标**：理解 Context Engine 的设计理念、预算机制和 Profile 策略，能向面试官清晰解释\"为什么要单独做上下文装配\"以及\"四个来源如何协同工作\"。

---

## 1. 白话理解：什么是 Context Engine？

### 1.1 一句话解释

**Context Engine = 一个\"智能编辑\"**，它从多个来源（对话历史、长期记忆、RAG 文档、工具结果）中挑出最重要的信息，在预算范围内打包发给 LLM。挑什么、留什么、扔什么，全部有据可查。

### 1.2 一个类比：打包行李箱

想象你要出差 3 天，行李箱限重 **20kg**。你有一堆东西：

| 类别 | 物品 | 原始重量 |
|------|------|---------|
| 👕 换洗衣物 | 10 件衣服 | 8kg |
| 📖 工作资料 | 5 本文件夹 | 6kg |
| 🍪 零食特产 | 3 袋零食 | 4kg |
| 🔧 工具设备 | 笔记本电脑 + 充电器 | 5kg |
| **合计** | | **23kg** ❌ 超重了！|

你必须做取舍：

```
行李箱 20kg 上限
├── 👕 换洗衣物: 最多 7kg → 选最重要的 7 件
├── 📖 工作资料: 最多 5kg → 选最核心的 3 本
├── 🍪 零食特产: 最多 2kg → 选最想吃的 2 袋
└── 🔧 工具设备: 最多 5kg → 全部带上（必须的）
```

**Context Engine 做的就是这个**。LLM 有 token 上限（行李箱 20kg），你需要把对话历史、长期记忆、RAG 文档、工具结果打包进去，但不能超限。Context Engine 就是那个\"打包专家\"：

- 给每类东西定好预算上限（衣物最多 7kg，文档最多 3000 token）
- 挑最重要的放进去
- 超出的丢弃，并记录删了什么、为什么删

### 1.3 一张图看懂

```
                        Context Engine
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  HistoryMessages ──┐                                        │
│  (对话历史)         │                                        │
│                    │      ┌──────────────┐                  │
│  MemoryItems ──────┼─────▶│   Assembler  │──────▶ LLM       │
│  (长期记忆)         │      │  装配引擎     │     ContextPackage│
│                    │      │              │                  │
│  DocumentItems ────┼─────▶│  Budget 预算  │                  │
│  (RAG 检索结果)     │      │  Profile策略  │                  │
│                    │      │  Trace 可追溯  │                  │
│  ToolItems ────────┘      └──────────────┘                  │
│  (工具调用结果)                                               │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 为什么需要 Context Engine？

### 2.1 没有 Context Engine 会怎样？

| 做法 | 问题 |
|------|------|
| **全塞进去** | 超过 LLM token 上限，API 报错或静默截断 |
| **只塞最新几条消息** | 丢失历史记忆和关键上下文，回答质量下降 |
| **手动拼接** | 代码到处散落拼接逻辑，改一个参数要改十几处 |

这些问题在 Demo 阶段不明显（对话短、数据少），但在**生产系统中**会集中爆发：

- 一个运维会话可能持续几十轮对话
- 长期记忆可能积累了数百条
- RAG 可能检索出几十篇相关文档
- 工具返回的日志/告警结果可能非常长

**Context Engine 就是 Demo 和生产系统的分水岭。**

### 2.2 Context Engine 的核心价值

```
没有 Context Engine 的简单拼接:
    content := history + memory + docs + tools
    // 超限 → 截断末尾 → 用户问题可能被截掉，关键信息丢失
    // 改参数 → 要改十几处 hardcode

有 Context Engine:
    pkg := assembler.Assemble(ctx, req, history)
    // 按预算逐类处理 → 超限时按规则裁剪 → 可追溯
    // 改参数 → 改一处 config.yaml（或代码常量）
```

**三个核心价值**：

| 价值 | 说明 |
|------|------|
| **预算控制** | 每类来源独立预算，防止某一类来源\"霸占\"整个上下文窗口 |
| **策略分离** | 不同场景（chat / aiops / report）使用不同的装配策略，按需配置 |
| **完全可追溯** | ContextTrace 记录每一类的选中数、丢弃数、丢弃原因，方便排查\"为什么 LLM 没看到某条信息\" |

---

## 3. 四类上下文来源

Context Engine 管理四类信息来源，最终打包成 `ContextPackage`：

```go
// types.go - ContextPackage（上下文包裹）
type ContextPackage struct {
    Request         ContextRequest      // 原始请求信息
    Profile         ContextProfile      // 使用的策略
    Query           string              // 用户问题
    HistoryMessages []*schema.Message   // ① 对话历史（短期记忆）
    MemoryItems     []ContextItem       // ② 长期记忆（跨会话）
    DocumentItems   []ContextItem       // ③ RAG 检索结果
    ToolItems       []ContextItem       // ④ 工具调用结果
    Trace           ContextAssemblyTrace // 装配追溯
}
```

### 3.1 HistoryMessages — 对话历史（短期记忆）

- **来源**：当前会话中用户与 LLM 的对话记录
- **生命周期**：会话级，会话结束即失效
- **作用**：让 LLM 知道"刚刚聊了什么"，保持对话连贯性
- **预算**：默认 3200 token（= MaxTokens × 40%，基于公式 `HistoryReserve = maxTokens × 0.40`，默认 maxTokens=8000）
- **选择策略**：从最新消息开始逆向选取，优先保留最近的对话

```
对话历史（10 条消息）:
┌─────────────────────────────────────────────┐
│ msg1: 用户: "Redis 怎么查慢查询？"            │  ← 最旧
│ msg2: AI: "可以用 SLOWLOG GET 命令..."        │
│ msg3: 用户: "我发现有很多 KEYS * 调用"         │
│ ...                                         │
│ msg9: 用户: "帮我分析一下 CPU 飙升的原因"       │  ← 较新
│ msg10: AI: "好的，请提供时间范围..."           │  ← 最新
└─────────────────────────────────────────────┘
         │
         ▼ selectHistory（从最新开始逆向选取）
┌─────────────────────────────────────────────┐
│ msg10 ✓ (300 tokens)                        │
│ msg9  ✓ (250 tokens)                        │
│ msg8  ✓ (400 tokens)                        │
│ msg7  ✓ (350 tokens)  累计 1300 / 3200       │
│ msg6  ✓ (500 tokens)  累计 1800 / 3200       │
│ msg5  ✓ (600 tokens)  累计 2400 / 3200       │
│ msg4  ✓ (550 tokens)  累计 2950 / 3200       │
│ msg3  ✗ (400 tokens)  超预算 → 丢弃           │  ← 旧消息被裁剪
│ msg2  ✗ → 丢弃                               │
│ ...                                         │
└─────────────────────────────────────────────┘
```

> **面试要点**：从最新消息开始逆向选取。如果包含 `[对话历史摘要]` 前缀的消息（超长对话摘要），会强制保留。

### 3.2 MemoryItems — 长期记忆（跨会话）

- **来源**：`LongTermMemory`，跨会话持久化的重要信息
- **生命周期**：跨会话，持久化存储
- **作用**：让 LLM 知道\"这个用户/项目的历史背景\"
- **预算**：默认 800 token（= MaxTokens × 10%，基于公式 `MemoryReserve = maxTokens × 0.10`）
- **选择策略**：多维过滤 — 过期、Scope、置信度、安全标签、预算、数量窗口

```go
// assembler.go - selectMemories 过滤逻辑
for _, entry := range entries {
    // 1. 过期检查
    if memoryItemExpired(item, now) → 丢弃 (memory_expired)

    // 2. 作用域检查（Session / User / Project / Global）
    if !memoryScopeAllowed(item.Scope, profile.AllowedMemoryScopes) → 丢弃 (memory_scope)

    // 3. 置信度过滤（低于 MinMemoryConfidence 丢弃）
    if item.Confidence < profile.MinMemoryConfidence → 丢弃 (memory_confidence)

    // 4. 安全标签过滤（非 internal/trusted_internal/safe 丢弃）
    if !memorySafetyAllowed(item.SafetyLabel) → 丢弃 (memory_safety)

    // 5. 数量窗口（超过 MaxMemoryItems 丢弃）
    if selectedCount >= profile.MaxMemoryItems → 丢弃 (memory_window)

    // 6. Token 预算（超出 MemoryTokens 丢弃）
    if item.TokenEstimate > remaining → 丢弃 (memory_budget)

    // 全部通过 → 选中 ✓
}
```

### 3.3 DocumentItems — RAG 检索结果

- **来源**：RAG 链路（Query Rewrite → Milvus 检索 → Rerank）
- **生命周期**：请求级
- **作用**：让 LLM 参考知识库中的相关文档回答问题
- **预算**：默认 1200 token（= MaxTokens × 15%，基于公式 `DocumentReserve = maxTokens × 0.15`）
- **选择策略**：按 Rerank 分数排序，从头开始填充直到预算用完；超预算的文档会被裁剪（trim）后尝试放入

```go
// documents.go - selectDocuments
// 超预算时的处理：
if item.TokenEstimate > remaining {
    trimmed := mem.TrimToTokenBudget(item.Content, remaining)  // 裁剪到剩余预算
    if strings.TrimSpace(trimmed) == "" {
        item.DroppedReason = "document_budget"  // 裁剪后为空 → 丢弃
    }
    item.Content = trimmed                        // 裁剪后放入
    item.CompressionLevel = "trimmed"             // 标记为已裁剪
}
```

### 3.4 ToolItems — 工具调用结果

- **来源**：Agent 调用工具（查日志、查告警、查数据库等）返回的结果
- **生命周期**：请求级
- **作用**：让 LLM 基于实时工具调用结果做推理
- **预算**：默认 800 token（= MaxTokens × 10%，基于公式 `ToolTokens = int(MaxTokens × 0.10)`）
- **选择策略**：按顺序处理，支持数量窗口（MaxToolItems）+ token 预算双重限制

### 3.5 四类来源总结

| 来源 | 作用 | 生命周期 | 预算（maxTokens=8000） | 代码位置 |
|------|------|---------|------|---------|
| HistoryMessages | 对话连贯性 | 会话级 | 3200 tokens (40%) | `selectHistory()` |
| MemoryItems | 用户/项目背景 | 跨会话持久 | 800 tokens (10%) | `selectMemories()` |
| DocumentItems | 知识库参考 | 请求级 | 1200 tokens (15%) | `selectDocuments()` |
| ToolItems | 实时工具结果 | 请求级 | 800 tokens (10%) | `selectToolItems()` |

---

## 4. Budget 预算机制

### 4.1 预算结构

每类来源都有一个独立的 token 预算上限，定义在 `ContextBudget` 中：

```go
// types.go - ContextBudget
type ContextBudget struct {
    MaxTotalTokens int  // 总 token 上限（全局）
    SystemTokens   int  // System Prompt 占用
    HistoryTokens  int  // 对话历史预算
    MemoryTokens   int  // 长期记忆预算
    DocumentTokens int  // RAG 文档预算
    ToolTokens     int  // 工具结果预算
    ReservedTokens int  // 预留 token（答案输出等）
}
```

```
┌──────────────────────────────────────────────────────┐
│                 LLM Token 总预算                      │
│                 (MaxTotalTokens)                      │
├────────────┬─────┬──────┬──────┬──────┬──────┬───────┤
│  System    │History│Memory│ Docs │ Tools│Reserved│     │
│  Prompt    │ 3200  │ 800  │ 1200 │ 800  │  400   │     │
│  (固定)    │ (40%) │(10%) │(15%) │(10%) │ (5%)   │     │
└────────────┴──────┴──────┴──────┴──────┴───────┴─────┘
      ▲          ▲      ▲      ▲      ▲       ▲
      │          │      │      │      │       └─ 留给 LLM 输出
      │          │      │      │      └───────── 工具结果上限
      │          │      │      └──────────────── RAG 文档上限
      │          │      └─────────────────────── 长期记忆上限
      │          └────────────────────────────── 对话历史上限
      └───────────────────────────────────────── 系统提示词占用
```

### 4.2 预算分配公式（实际实现）

预算分两阶段计算：**第一阶段**在 `utility/mem/token_budget.go:GetTokenBudget()` 中从配置计算基础预留；**第二阶段**在 `resolver.go:Resolve()` 中由 Profile 覆盖并计算工具和预留 token。

#### 4.2.1 第一阶段：全局基础预留

```go
// utility/mem/token_budget.go - GetTokenBudget() 实际实现
maxTokens := defaultMaxContextTokens  // 默认 8000，可由 memory.max_context_tokens 配置覆盖

SystemReserve   = maxTokens × 0.20    // System Prompt 占 20%
HistoryReserve  = maxTokens × 0.40    // 对话历史占 40%
MemoryReserve   = maxTokens × 0.10    // 长期记忆占 10%
DocumentReserve = maxTokens × 0.15    // RAG 文档占 15%
```

以 **maxTokens = 8000** 为例（默认值）：

| 预留项 | 比例 | 计算公式 | 数值 |
|--------|:----:|---------|:----:|
| `SystemReserve` | 20% | `int(8000 × 0.20)` | **1600** |
| `HistoryReserve` | 40% | `int(8000 × 0.40)` | **3200** |
| `MemoryReserve` | 10% | `int(8000 × 0.10)` | **800** |
| `DocumentReserve` | 15% | `int(8000 × 0.15)` | **1200** |
| **小计（已分配）** | 85% | — | **6800** |
| **剩余池（待 Profile 分配）** | 15% | — | **1200** |

> **注**：`GetTokenBudget()` 使用 `sync.Once` 确保全局单例，配置变更需重启生效。配置项为 `memory.max_context_tokens`，从 `manifest/config/config.yaml` 读取。

#### 4.2.2 第二阶段：Profile 级分配（`resolver.go`）

在第一阶段基础值之上，`PolicyResolver.Resolve()` 完成最终分配：

```go
// resolver.go - Resolve() 实际实现
ToolTokens     = int(float64(budget.MaxTokens) × 0.10)   // 额外从剩余池分 10%
ReservedTokens = budget.MaxTokens - SystemReserve - HistoryReserve
                 - MemoryReserve - DocumentReserve - ToolTokens
// 代入 8000：= 8000 - 1600 - 3200 - 800 - 1200 - 800 = 400
```

完整计算过程（maxTokens=8000）：

| 字段 | 公式 | 数值 |
|------|------|:----:|
| `MaxTotalTokens` | `budget.MaxTokens` | **8000** |
| `SystemTokens` | `budget.SystemReserve` = `8000 × 0.20` | **1600** |
| `HistoryTokens` | `budget.HistoryReserve` = `8000 × 0.40` | **3200** |
| `MemoryTokens` | `budget.MemoryReserve` = `8000 × 0.10` | **800** |
| `DocumentTokens` | `budget.DocumentReserve` = `8000 × 0.15` | **1200** |
| `ToolTokens` | `int(8000 × 0.10)` | **800** |
| `ReservedTokens` | `8000 - 1600 - 3200 - 800 - 1200 - 800` | **400** |

> **公式总结**（一步到位）：
> ```
> ToolTokens     = MaxTokens × 10%
> ReservedTokens = MaxTokens × 5%    （因为 100% - 20% - 40% - 10% - 15% - 10% = 5%）
> ```

#### 4.2.3 Token 估算公式（`EstimateTokens`）

预算控制依赖 token 估算，`utility/mem/token_budget.go:EstimateTokens()` 的实际实现：

```go
// 不同字符类型的 token 权重
CJK 字符 (U+4E00~U+9FFF)  → 每个字符 = 1.5 tokens
ASCII 字符 (33~126)       → 每 4 个字符 = 1 token（即每个字符 0.25 tokens）
其他字符                   → 每个字符 = 0.5 tokens

total = CJK字符数 × 1.5 + ASCII字符数 ÷ 4 + 其他字符数 × 0.5
if total < 1 && len(text) > 0 { total = 1 }  // 非空文本至少 1 token
```

示例：
- `"你好世界"`（4 个 CJK）→ 4 × 1.5 = **6 tokens**
- `"hello world"`（11 个 ASCII）→ 11 ÷ 4 = **2 tokens**（取整）
- `"Redis 慢查询排查"`（5 CJK + 1 空格 + 5 ASCII）→ 5×1.5 + 0.5 + 1 = **9 tokens**

### 4.3 预算用完了怎么办？

每种来源都有独立的处理方式：

| 来源 | 超出预算时的行为 |
|------|----------------|
| HistoryMessages | 旧消息被丢弃（`history_budget`），新消息优先保留 |
| MemoryItems | 内存条目被丢弃（`memory_budget`），不裁剪 |
| DocumentItems | **先裁剪（trim），裁剪后仍空则丢弃**（`document_budget`） |
| ToolItems | 先裁剪（trim），裁剪后仍空则丢弃（`tool_budget`） |

```go
// assembler.go - selectToolItems 预算处理
if item.TokenEstimate > remaining {
    trimmed := mem.TrimToTokenBudget(item.Content, remaining)  // 尝试裁剪
    if strings.TrimSpace(trimmed) == "" {
        item.DroppedReason = "tool_budget"   // 裁剪后为空 → 丢弃
        continue
    }
    item.Content = trimmed                    // 裁剪后放入
    item.CompressionLevel = "trimmed"
}
```

> **为什么文档和工具可以裁剪，但记忆不能？** 文档和工具结果是辅助参考信息，截断部分不影响核心含义；而记忆条目通常较短且是关键背景信息，裁剪会造成信息丢失，所以超预算直接丢弃而非裁剪。

---

## 5. Profile 策略机制

### 5.1 为什么需要 Profile？

不同场景需要不同的上下文组合：

- **日常聊天（chat）**：需要对话历史保持连贯，也需要知识和记忆辅助回答
- **运维诊断（aiops）**：不需要对话历史（每次是独立故障场景），但需要记忆（用户偏好）和工具结果（实时告警/日志）
- **报告生成（report）**：只需要工具结果做素材，不需要历史和记忆

如果用一个策略覆盖所有场景，要么浪费 token（塞了不需要的东西），要么缺少关键信息。

### 5.2 三种 Profile 对比

| Profile | AllowHistory | AllowMemory | AllowDocs | AllowToolResults | Staged | 使用场景 |
|---------|:-----------:|:-----------:|:---------:|:----------------:|:------:|---------|
| **chat** | ✅ | ✅ | ✅ | ❌ | ✅ | 日常对话、知识问答 |
| **aiops** | ❌ | ✅ | ❌ | ❌ | ❌ | 运维故障排查 |
| **report** | ❌ | ❌ | ❌ | ✅ | ❌ | 报告/摘要生成 |

#### chat 模式

```
┌──────────────────────────────────────┐
│  chat 模式                           │
│                                      │
│  ✅ History  → 保留最近对话，保持连贯  │
│  ✅ Memory   → 注入用户/项目背景      │
│  ✅ Docs     → RAG 检索相关知识       │
│  ❌ Tools    → 不需要工具结果         │
│  ✅ Staged   → 记忆作为消息前置注入    │
└──────────────────────────────────────┘
```

#### aiops 模式

```
┌──────────────────────────────────────┐
│  aiops 模式                          │
│                                      │
│  ❌ History  → 每次是独立故障场景      │
│  ✅ Memory   → 用户偏好、历史经验      │
│  ✅ Docs     → 故障排查 SOP           │
│  ✅ Tools    → 实时告警、日志结果      │
│  ❌ Staged   → 记忆不进对话流         │
└──────────────────────────────────────┘
```

#### report 模式

```
┌──────────────────────────────────────┐
│  report 模式                         │
│                                      │
│  ❌ History  → 不需要对话历史         │
│  ❌ Memory   → 不需要长期记忆         │
│  ❌ Docs     → 不需要知识检索         │
│  ✅ Tools    → 只有工具结果作为素材    │
│  ❌ Staged   → 记忆不进对话流         │
└──────────────────────────────────────┘
```

### 5.3 Profile 解析逻辑

`PolicyResolver.Resolve()` 根据 `req.Mode` 决定使用哪个 Profile：

```go
// resolver.go - Profile 解析
func (r *PolicyResolver) Resolve(ctx context.Context, req ContextRequest) ContextProfile {
    base := ContextProfile{
        Name: "chat-default",               // 默认是 chat
        AllowHistory: true,
        AllowMemory:  true,
        AllowDocs:    true,
        Staged:       true,
        // ... 默认预算 ...
    }

    switch req.Mode {
    case "aiops", "specialist":
        base.Name = "aiops-default"
        base.AllowHistory = false           // ← 关掉 History
        base.AllowDocs = false              // ← 关掉 Docs
        base.AllowToolResults = false
        base.Staged = false
        // Budget: HistoryTokens = 0, ToolTokens = 0

    case "reporter":
        base.Name = "reporter-default"
        base.AllowHistory = false
        base.AllowMemory = false
        base.AllowDocs = false
        base.AllowToolResults = true        // ← 只保留 Tools
        base.Staged = false
        // Budget: HistoryTokens = 0, MemoryTokens = 0, DocumentTokens = 0

    case "chat":
        // 使用默认配置，不需要修改
    }

    return base
}
```

### 5.4 三种 Profile 预算分配对比表

以下基于 **maxTokens=8000**（默认值）展示三种 Profile 的实际预算分配。所有值均由 `resolver.go:Resolve()` 根据 `GetTokenBudget()` 基础值 + Mode 覆盖计算得出。

#### 5.4.1 预算分配明细（数值）

| 预算字段 | **chat**（默认） | **aiops**（诊断） | **reporter**（报告） | 来源 |
|---------|:---:|:---:|:---:|------|
| `MaxTotalTokens` | 8000 | 8000 | 8000 | `budget.MaxTokens` |
| `SystemTokens` | 1600 | 1600 | 1600 | `MaxTokens × 20%`（固定） |
| `HistoryTokens` | **3200** | **0** ❌ | **0** ❌ | chat: `HistoryReserve`; aiops/reporter: Profile 覆盖为 0 |
| `MemoryTokens` | **800** | **800** | **0** ❌ | chat/aiops: `MemoryReserve`; reporter: Profile 覆盖为 0 |
| `DocumentTokens` | **1200** | **0** ❌ | **0** ❌ | chat: `DocumentReserve`; aiops/reporter: Profile 覆盖为 0 |
| `ToolTokens` | 800 (禁用) | **0** ❌ | **800** | chat: 800 但 `AllowToolResults=false`; aiops: 覆盖为 0; reporter: `MaxTokens × 10%` |
| `ReservedTokens` | 400 | 400 | 400 | `MaxTokens × 5%`（固定） |
| **有效上下文预算** | **5200** | **800** | **800** | = 实际可分配给非 System 来源的 token |
| **System + Reserved** | 2000 | 2000 | 2000 | 固定开销 |

> **"有效上下文预算"** = 启用的来源预算之和（不计 System 和 Reserved）。chat 模式有 5200 token 用于 History/Memory/Docs，而 aiops 和 reporter 各自只有 800 token 用于单一来源。

#### 5.4.2 功能开关一览

| 功能开关 | **chat** | **aiops** | **reporter** | 含义 |
|---------|:---:|:---:|:---:|------|
| `AllowHistory` | ✅ | ❌ | ❌ | 是否注入对话历史 |
| `AllowMemory` | ✅ | ✅ | ❌ | 是否检索长期记忆 |
| `AllowDocs` | ✅ | ❌ | ❌ | 是否触发 RAG 检索 |
| `AllowToolResults` | ❌ | ❌ | ✅ | 是否纳入工具调用结果 |
| `Staged` | ✅ | ❌ | ❌ | 记忆是否以消息形式前置注入 |
| `MaxHistoryMessages` | 10 | 0 | 0 | 数量窗口：最大历史消息数 |
| `MaxMemoryItems` | 5 | 5 | 0 | 数量窗口：最大记忆条目数 |
| `MaxToolItems` | 0 | 0 | 8 | 数量窗口：最大工具结果数 |
| `MinMemoryConfidence` | 0.50 | 0.50 | — | 记忆置信度阈值 |
| `AllowedMemoryScopes` | session/user/project/global | session/user/project/global | — | 记忆作用域允许列表 |

#### 5.4.3 各 Profile 预算占比可视化

```
chat 模式 (maxTokens=8000):
┌──────────┬────────────┬───────┬─────────┬──────────┬──────┐
│ System   │  History   │Memory │  Docs   │ Reserved │Tool* │
│  1600    │   3200     │  800  │  1200   │   400    │ 800  │
│  (20%)   │   (40%)    │ (10%) │  (15%)  │   (5%)   │(禁用)│
└──────────┴────────────┴───────┴─────────┴──────────┴──────┘

aiops 模式 (maxTokens=8000):
┌──────────┬──────────────────────────┬───────┬──────────┐
│ System   │        (未使用)          │Memory │ Reserved │
│  1600    │   History=0, Docs=0,     │  800  │   400    │
│  (20%)   │   Tool=0                 │ (10%) │   (5%)   │
└──────────┴──────────────────────────┴───────┴──────────┘
           └─ 5200 token 留空 ─────────┘

reporter 模式 (maxTokens=8000):
┌──────────┬──────────────────────────┬──────┬──────────┐
│ System   │        (未使用)          │ Tool │ Reserved │
│  1600    │   History=0, Memory=0,   │  800 │   400    │
│  (20%)   │   Docs=0                 │ (10%)│   (5%)   │
└──────────┴──────────────────────────┴──────┴──────────┘
           └─ 5200 token 留空 ────────┘
```

> **设计理念**：aiops 和 reporter 模式下大量 token 留空，不是浪费，而是**明确拒绝了不需要的信息来源**。这些留空的 token 在实际上不会进入 LLM 上下文窗口，让 LLM 可以完全聚焦于当前任务。System Prompt 和 Reserved（答案输出）的空间始终保留。

### 5.5 Staged 模式是什么？

当 `Staged = true`（chat 模式）时，记忆不会直接放入 `MemoryItems`，而是**转换为消息前置到对话历史中**：

```go
// assembler.go - Staged 处理
if profile.Staged && len(pkg.MemoryItems) > 0 {
    pkg.HistoryMessages = append(
        memoryItemsAsMessages(pkg.MemoryItems),  // 记忆 → 消息
        pkg.HistoryMessages...,                   // 原始历史
    )
}
```

生成的记忆消息格式：

```go
// assembler.go - memoryItemsAsMessages
return []*schema.Message{
    {
        Role:    schema.User,
        Content: "[关键记忆]\n- [故障偏好] 用户更关注 Redis 相关问题\n- [历史经验] 上周处理过类似 CPU 告警",
    },
    schema.AssistantMessage("好的，我已了解这些背景信息。", nil),
}
```

**效果**：LLM 像\"看到\"两条额外的消息一样，自然地获取了记忆上下文，而不是被\"告知\"这是记忆。这种方式让记忆注入更自然，LLM 理解更好。

---

## 6. Assembler 装配流程

### 6.1 完整装配流程图

```
Assembler.Assemble(ctx, req, history)
│
├─ Step 0: 记录开始时间，用于计算装配总耗时
│
├─ Step 1: PolicyResolver.Resolve(ctx, req)
│   └─ 根据 req.Mode 选择 Profile（chat / aiops / report）
│
├─ Step 2: 初始化 ContextPackage + ContextAssemblyTrace
│   └─ 记录 BudgetBefore（装配前预算快照）
│
├─ Step 3: selectHistory() — 对话历史选择
│   ├─ 条件：profile.AllowHistory && len(history) > 0
│   ├─ 逻辑：从最新消息开始逆向选取
│   │   ├─ 按 MaxHistoryMessages 数量窗口过滤
│   │   └─ 按 HistoryTokens 预算过滤
│   ├─ 特殊处理：强制保留 "[对话历史摘要]" 前缀消息
│   └─ 输出：selectedHistory + droppedHistory + trace
│
├─ Step 4: selectMemories() — 长期记忆检索与过滤
│   ├─ 条件：profile.AllowMemory && req.SessionID != ""
│   ├─ 检索：LongTermMemory.RetrieveScoped(query, limit*3, policy)
│   │   └─ 按 Session / User / Project / Global Scope 检索
│   ├─ 过滤（6 层）：
│   │   ├─ 过期检查 (ExpiresAt)
│   │   ├─ Scope 允许列表
│   │   ├─ 置信度阈值 (MinMemoryConfidence)
│   │   ├─ 安全标签 (SafetyLabel)
│   │   ├─ 数量窗口 (MaxMemoryItems)
│   │   └─ Token 预算 (MemoryTokens)
│   └─ 输出：selectedMemory + droppedMemory + trace
│
├─ Step 5: selectDocuments() — RAG 文档检索
│   ├─ 条件：profile.AllowDocs
│   ├─ 检索：rag.Query() → Rewrite + Retrieve + Rerank
│   │   └─ 超时保护：contextDocsQueryTimeout
│   ├─ 裁剪：按 DocumentTokens 预算，超出部分 trim 或丢弃
│   └─ 输出：selectedDocs + droppedDocs + trace
│
├─ Step 6: selectToolItems() — 工具结果选择
│   ├─ 条件：profile.AllowToolResults && len(req.ToolItems) > 0
│   ├─ 过滤：先数量窗口 (MaxToolItems)，再 Token 预算 (ToolTokens)
│   ├─ 裁剪：超出预算的 item 尝试 trim
│   └─ 输出：selectedTools + droppedTools + trace
│
├─ Step 7: Staged 处理（如果启用）
│   └─ memoryItemsAsMessages() → 前置到 HistoryMessages
│
└─ Step 8: 返回 ContextPackage（含完整 Trace）
    └─ 记录 BudgetAfter + LatencyMs
```

### 6.2 代码对应

| 步骤 | 代码位置 | 核心函数 |
|------|---------|---------|
| Profile 解析 | `resolver.go:25` | `PolicyResolver.Resolve()` |
| History 选择 | `assembler.go:234` | `selectHistory()` |
| Memory 检索 | `assembler.go:65-68` | `mem.GetLongTermMemory().RetrieveScoped()` |
| Memory 过滤 | `assembler.go:292` | `selectMemories()` |
| Document 检索 | `documents.go:29` | `selectDocuments()` |
| Tool 选择 | `assembler.go:148` | `selectToolItems()` |
| Staged 注入 | `assembler.go:114-116` | `memoryItemsAsMessages()` |

---

## 7. ContextTrace — 可追溯的装配记录

### 7.1 Trace 数据结构

每步装配操作都被记录下来，形成完整的装配追溯链：

```go
// types.go - ContextAssemblyTrace
type ContextAssemblyTrace struct {
    Profile           string           // 使用的 Profile 名称（chat-default / aiops-default / reporter-default）
    Stages            []StageTrace     // 每个阶段的详细信息
    SourcesConsidered int             // 考虑的来源总数
    SourcesSelected   int             // 最终选中的来源数
    DroppedItems      []ContextItem   // 被丢弃的条目（含丢弃原因）
    BudgetBefore      BudgetSnapshot  // 装配前预算
    BudgetAfter       BudgetSnapshot  // 装配后预算使用量
    LatencyMs         int64           // 装配总耗时（毫秒）
}

type StageTrace struct {
    Name          string                // 阶段名：history / memory / documents / tool_results
    SelectedCount int                   // 选中数量
    DroppedCount  int                   // 丢弃数量
    Notes         []string              // 备注（如 tokens=1800/2000）
    Retrieval     *RetrievalStageMetrics // RAG 检索耗时详情（仅 documents 阶段）
}
```

### 7.2 丢弃原因字典

每个被丢弃的条目都带有一个 `DroppedReason`，方便排查问题：

| 丢弃原因 | 来源 | 含义 |
|---------|------|------|
| `history_window` | History | 超出 MaxHistoryMessages 数量窗口 |
| `history_budget` | History | 超出 HistoryTokens 预算 |
| `memory_expired` | Memory | 记忆已过期 |
| `memory_scope` | Memory | Scope 不在允许列表 |
| `memory_confidence` | Memory | 置信度低于阈值 |
| `memory_safety` | Memory | 安全标签不允许 |
| `memory_window` | Memory | 超出 MaxMemoryItems 数量窗口 |
| `memory_budget` | Memory | 超出 MemoryTokens 预算 |
| `document_budget` | Document | 超出 DocumentTokens 预算 |
| `tool_window` | Tool | 超出 MaxToolItems 数量窗口 |
| `tool_budget` | Tool | 超出 ToolTokens 预算 |

### 7.3 Trace 的实际用途

```go
// assembler.go - TraceDetails 格式化输出
func TraceDetails(trace ContextAssemblyTrace) []string {
    details := []string{
        fmt.Sprintf("context profile=%s", trace.Profile),
        fmt.Sprintf("context sources selected=%d/%d", trace.SourcesSelected, trace.SourcesConsidered),
    }
    // 每个阶段的详细信息
    for _, stage := range trace.Stages {
        line := fmt.Sprintf("%s selected=%d dropped=%d", stage.Name, stage.SelectedCount, stage.DroppedCount)
        // ... 附加 notes 和 retrieval metrics
    }
    // 丢弃原因的统计
    // context dropped memory_budget=3, history_window=5, memory_expired=2
    return details
}
```

**典型输出示例**：

```
context profile=chat-default
context sources selected=15/38
history selected=6 dropped=5 (tokens=2900/3200)
memory selected=3 dropped=8 (tokens=750/800; min_confidence=0.50)
documents selected=4 dropped=12 (tokens=1100/1200; retrieval cache_hit=false init_ms=12 rewrite_ms=350 retrieve_ms=120 rerank_ms=520 raw=20 final=4 rerank=true)
context dropped history_window=3, history_budget=2, memory_confidence=5, memory_budget=2, memory_expired=1, document_budget=12
latency_ms=45
```

> **面试要点**：ContextTrace 让上下文装配从\"黑盒\"变成\"白盒\"。如果 LLM 回答不理想，可以快速排查：是不是关键信息被 budget 裁掉了？是不是记忆过期了？是不是 RAG 没召回？

---

## 8. 面试问答

### Q1: Context Engine 是什么？（一句话）

<details>
<summary>点击查看答案</summary>

**一句话**：Context Engine 是一个**智能上下文装配器**，它从对话历史、长期记忆、RAG 文档、工具结果四个来源中按预算选取最重要的信息打包发给 LLM。

**展开**：
- 它解决了 LLM token 限制与信息量之间的矛盾
- 每类来源有独立的 token 预算，防止单一来源霸占窗口
- 不同场景（chat/aiops/report）使用不同的 Profile 策略
- 所有装配决策通过 ContextTrace 完全可追溯

</details>

### Q2: 为什么要单独做 Context Engine？跟简单拼接有什么区别？

<details>
<summary>点击查看答案</summary>

**核心区别**：简单拼接只解决\"怎么拼\"，Context Engine 解决\"拼什么\"和\"为什么这样拼\"。

| | 简单拼接 | Context Engine |
|---|---|---|
| 预算控制 | ❌ 无，超限就截断 | ✅ 每类独立预算，按规则裁剪 |
| 策略分离 | ❌ 全部场景一样 | ✅ chat/aiops/report 三种 Profile |
| 可追溯 | ❌ 不知道删了什么 | ✅ ContextTrace 完整记录 |
| 记忆管理 | ❌ 无/手动 | ✅ 六层过滤（过期/Scope/置信度/安全/数量/预算） |
| 配置化 | ❌ hardcode 散落各处 | ✅ config.yaml + 常量集中管理 |

**一句话**：简单拼接是 Demo 级方案，Context Engine 是生产级方案。

</details>

### Q3: Budget 怎么算的？公式和数值是什么？

<details>
<summary>点击查看答案</summary>

预算分两阶段计算，全部是比例制（基于 `MaxTokens`，默认 8000，可通过 `memory.max_context_tokens` 配置）：

**第一阶段：`GetTokenBudget()` — 全局基础预留**（`utility/mem/token_budget.go`）

```
SystemReserve   = MaxTokens × 20%    // 1600（默认）
HistoryReserve  = MaxTokens × 40%    // 3200
MemoryReserve   = MaxTokens × 10%    //  800
DocumentReserve = MaxTokens × 15%    // 1200
                 ─────────────────
                 已占用 85%          // 6800
                 剩余 15%            // 1200 → 待第二阶段分配
```

**第二阶段：`Resolve()` — Profile 级覆盖**（`resolver.go`）

```
ToolTokens     = MaxTokens × 10%     //  800（从剩余池分配）
ReservedTokens = MaxTokens × 5%      //  400（= 100%-20%-40%-10%-15%-10%）
```

然后根据 `req.Mode` 覆盖：

| 覆盖项 | **chat** | **aiops** | **reporter** |
|--------|:---:|:---:|:---:|
| `HistoryTokens` | 3200（不变） | **→ 0** | **→ 0** |
| `MemoryTokens` | 800（不变） | 800（不变） | **→ 0** |
| `DocumentTokens` | 1200（不变） | **→ 0** | **→ 0** |
| `ToolTokens` | 800（`AllowToolResults=false`） | **→ 0** | 800（不变） |
| 有效上下文预算 | **5200** | **800** | **800** |

**Token 估算**（`EstimateTokens`）：

```
CJK 字符  (U+4E00~U+9FFF) → 每个 = 1.5 tokens
ASCII 字符 (33~126)       → 每4个 = 1 token
其他字符                   → 每个 = 0.5 tokens
```

**为什么要分开设？**
- History 独占 40%（最大块），因为对话连贯性对回答质量影响最大
- 每类独立预算防止单一来源（如 RAG 返回大量文档）挤掉对话历史
- aiops 和 reporter 主动将不需要的来源预算归零，避免信息噪音

</details>

---

## 9. 自测

### 问题 1

chat 模式下，对话历史已经很长（15 条消息），用户又问了一个新问题。selectHistory 会怎么处理这些历史消息？哪些会被保留，哪些会被丢弃？

<details>
<summary>点击查看答案</summary>

**处理流程**：

1. **数量窗口过滤**：如果 `MaxHistoryMessages` = 10，超过 10 条的部分（最旧的 5 条）首先被 `history_window` 丢弃
2. **Token 预算过滤**：从最新消息（第 10 条）开始逆向累积 token
   - 如果累积到第 8 条时 token 超过 `HistoryTokens`（3200，默认值 = MaxTokens × 40%），第 7 条及更旧的被 `history_budget` 丢弃
3. **摘要优先保留**：如果第 1 条消息以 `[对话历史摘要]` 开头且未被选中，会尝试单独纳入（如果 token 预算允许）

**结果**：最近的 8-10 条消息（约 3200 token 内）被保留，更旧的消息被丢弃。

</details>

### 问题 2

为什么 DocumentItems 超预算时可以裁剪（trim），而 MemoryItems 超预算时只能丢弃？

<details>
<summary>点击查看答案</summary>

**原因**：两者的性质和使用场景不同。

- **DocumentItems（文档）**：通常是较长的参考文本，截断部分内容不影响核心含义。LLM 看到前半段仍然能获取有效信息。裁剪后标记 `CompressionLevel = "trimmed"`。

- **MemoryItems（记忆）**：通常是短文本，内容高度浓缩。裁剪会造成关键信息的不可逆丢失。例如一条记忆 \"用户上次处理 Redis 故障时发现 maxconn 配置太低，建议调整为 1000\"——裁掉后半段后变成 \"用户上次处理 Redis 故障时发现 maxconn\"，完全失去了有效信息。

**设计原则**：能裁则裁（文档、工具结果），不能裁就丢（记忆），并记录丢弃原因。

</details>

### 问题 3

aiops 模式下，为什么 AllowHistory = false，但 AllowMemory = true？这两者不都是\"以前的信息\"吗？

<details>
<summary>点击查看答案</summary>

**两者本质上完全不同**：

| | History（对话历史） | Memory（长期记忆） |
|---|---|---|
| 内容 | 本次会话的原始对话记录 | 跨会话提炼的关键信息 |
| 特点 | 冗长、包含闲聊和试探 | 精炼、经 LLM 提取和去噪 |
| 价值 | 帮助 LLM 理解当前对话上下文 | 帮助 LLM 了解用户偏好和历史经验 |
| 在 aiops 场景 | 每次是独立故障，上次的对话不相关 | 用户偏好（如\"关注 Redis\"/\"优先看日志\"）仍然相关 |

**一个例子**：

- History 包含：\"帮我查一下 Redis\" → \"好的\" → \"不对，查 MySQL\" → \"MySQL 也没问题\" → ...（对当前故障诊断无帮助）
- Memory 包含：\"用户 John 的集群是 Kubernetes 1.28，节点在 us-east-1\"（对当前故障诊断有帮助）

所以 aiops 模式保留了 Memory（有价值的长线信息），去掉了 History（无价值的会话噪音）。

</details>

---

> **下一章预告**：Memory 记忆系统 — 三层记忆架构（短期/长期/工作）如何协同，以及 LLM 异步记忆抽取的完整链路。
