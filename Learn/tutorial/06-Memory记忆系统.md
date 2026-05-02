# 第 6 章：Memory 记忆系统 — "让 AI 记住你的每一次对话"

> **本章目标**：理解 OpsCaption 的三层记忆架构（短期/长期/工作记忆），掌握记忆写入和读取的完整链路，能向面试官清晰解释异步抽取、候选验证、RLock 优化、过期清理等关键设计。

---

## 1. 白话理解：什么是三层记忆？

### 1.1 一句话解释

**记忆系统 = AI 的"大脑记忆模型"**。它不像普通 chatbot 那样每次对话都"失忆"，而是像人脑一样，把重要信息存下来、需要时回忆起来。三层的设计直接对应认知科学中"工作记忆 → 短期记忆 → 长期记忆"的经典模型。

### 1.2 一个类比：你昨天修了一台服务器

想象你是运维工程师，昨天处理了一台 Redis 服务器的故障：

```
昨天的工作记忆（当时在脑子里）:
  "现在正在看 Redis slowlog... 有很多 KEYS * 调用... 哦，是前端没加前缀"

昨天的短期记忆（当天记得）:
  用户说："Redis CPU 飙到 90% 了"
  你说："看到了，是 KEYS * 导致的，已经加了前缀过滤"
  用户说："谢谢，顺便帮我把告警阈值调到 80%"

昨天的长期记忆（现在还能想起来）:
  ✅ "用户 John 的 Redis 集群 maxconn 只有 1000，容易打满"
  ✅ "用户 John 偏好告警阈值设为 80%"
  ✅ "Redis 慢查询排查流程：先看 slowlog，再看 bigkeys"
  ❌ 具体每句话的措辞 —— 忘了（不重要）
```

**三层记忆做的事情和这个一模一样**：

| 记忆层 | 人脑类比 | OpsCaption 实现 | 生命周期 |
|--------|---------|----------------|---------|
| **短期记忆** | 当前对话的内容，你还记得刚才说了什么 | `SimpleMemory` + `HistoryMessages` | 当前会话内 |
| **长期记忆** | 跨天/跨周还记得的重要信息 | `LongTermMemory` (持久化存储) | 跨会话持久 |
| **工作记忆** | 当前手头正在做的事情 | 当前任务的 `ToolItems` | 当前请求内 |

### 1.3 一张图看懂三层协同

```
┌─────────────────────────────────────────────────────────────────┐
│                       OpsCaption 记忆系统                         │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              ③ 工作记忆 (Working Memory)                  │  │
│  │  当前任务调用的工具结果：                                      │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐                  │  │
│  │  │ 查日志结果 │ │ 查告警结果 │ │ RAG结果   │                  │  │
│  │  └──────────┘ └──────────┘ └──────────┘                  │  │
│  │  Token 预算: 2000 | 生命周期: 当前请求                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                            ▲                                     │
│                            │ 组装                                 │
│                            │                                     │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              ① 短期记忆 (Short-term Memory)                │  │
│  │  ┌─────────────────────────────────────────────────────┐ │  │
│  │  │ User: "Redis CPU 飙到 90% 了"                        │ │  │
│  │  │ AI: "看到了，是 KEYS * 导致的"                        │ │  │
│  │  │ User: "帮我调一下告警阈值到 80%"                       │ │  │
│  │  │ AI: "好的，已调整"                                    │ │  │
│  │  │ [对话历史摘要] 之前讨论了 Redis 慢查询...               │ │  │
│  │  └─────────────────────────────────────────────────────┘ │  │
│  │  Token 预算: 2000 | 生命周期: 当前会话                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                            │                                     │
│                            │ 会话结束后提取                        │
│                            ▼                                     │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              ② 长期记忆 (Long-term Memory)                │  │
│  │                                                          │  │
│  │  Session Scope:  ┌──────────────────────────────────┐    │  │
│  │  (会话级)         │ "本次会话确认 Redis maxconn=1000" │    │  │
│  │                   └──────────────────────────────────┘    │  │
│  │                                                          │  │
│  │  User Scope:     ┌──────────────────────────────────┐    │  │
│  │  (用户级)         │ "用户 John 偏好告警阈值 80%"      │    │  │
│  │                   └──────────────────────────────────┘    │  │
│  │                                                          │  │
│  │  Project Scope:  ┌──────────────────────────────────┐    │  │
│  │  (项目级)         │ "checkoutsvc 部署在 K8s 1.28"    │    │  │
│  │                   └──────────────────────────────────┘    │  │
│  │                                                          │  │
│  │  Global Scope:   ┌──────────────────────────────────┐    │  │
│  │  (全局级)         │ "SLA 要求 99.9%，P99 < 500ms"     │    │  │
│  │                   └──────────────────────────────────┘    │  │
│  │                                                          │  │
│  │  Token 预算: 500 | 生命周期: 跨会话持久                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 为什么需要记忆系统？

### 2.1 没有记忆系统会怎样？

| 场景 | 没有记忆的 AI | 有记忆的 AI |
|------|-------------|-----------|
| 用户说"帮我查一下 Redis" | "好的，Redis 是一个内存数据库..." （泛泛而谈） | "根据之前的记录，你的 Redis 集群 maxconn=1000，我先帮你查 slowlog" |
| 用户切换话题后再切回来 | "你刚才问了什么？我不记得了" | "你之前问的是 Redis 慢查询，我们排查到 KEYS * 的问题" |
| 第二天再次打开 | "你好，我能帮你什么？" （完全失忆） | "欢迎回来 John，上次我们排查了 Redis 慢查询问题，还有需要跟进的吗？" |
| 用户说"像上次那样处理" | "上次是哪次？" | "你指的是 3 天前处理 checkoutservice CPU 告警的方式，我按相同流程执行" |

### 2.2 记忆系统的核心价值

```
没有记忆: 每次对话 = 一张白纸 → 用户重复描述背景 → 效率低、体验差

有记忆:   每次对话 = 站在历史经验上 → AI 已经知道你是谁、你的环境、你的偏好
```

**三个核心价值**：

| 价值 | 说明 |
|------|------|
| **连续性** | 跨会话记住用户信息，不用每次重新自我介绍 |
| **个性化** | 记住用户偏好（"喜欢简洁回答""先给结论再解释"） |
| **效率** | 复用历史排查结论，不用每次从零开始 |

### 2.3 为什么分三层而不是一层？

如果只用一个"记忆库"存所有东西：

```
痛点 1 — 速度: 每次请求都查持久化存储 → 延迟高
痛点 2 — 容量: 对话的每句话都存 → 快速爆满
痛点 3 — 噪声: 临时闲聊和重要事实混在一起 → LLM 无法区分

三层分工:
  短期 → 快、会话内、保留对话连贯性
  长期 → 持久、跨会话、只保留提炼后的关键信息
  工作 → 即时、当前任务、用完即弃
```

---

## 3. 三层记忆详解

### 3.1 短期记忆 — SimpleMemory + HistoryMessages

**定位**：当前会话的对话记录，让 LLM 知道"刚才聊了什么"。

**数据结构**（`utility/mem/mem.go`）：

```go
type SimpleMemory struct {
    ID            string              // 会话 ID
    Messages      []*schema.Message   // 对话消息列表
    Summary       string              // 超窗口消息的摘要
    MaxWindowSize int                 // 最大消息窗口（默认 20）
    turnCount     int                 // 对话轮次计数
    createdAt     time.Time
    topics        []TopicSegment      // 话题分段
}
```

**关键机制**：

1. **滑动窗口**：`MaxWindowSize` 默认 20 条消息（配置项 `memory.max_window_size`）。超过窗口的消息被移除，但**先做摘要再移除**，不会白白丢失。

```go
// trimWindow - 超窗口处理
if len(c.Messages) > windowSize {
    excess := len(c.Messages) - windowSize
    // 先为超出部分做摘要
    newSummary := c.summarizeMessages(c.Messages[:excess])
    // 摘要追加到 c.Summary（最多保留 2000 字符）
    // 然后移除超出部分的消息
    c.Messages = c.Messages[excess:]
}
```

2. **摘要链**：随着对话变长，摘要会持续追加，形成 `[对话历史摘要]` 前缀消息：

```
摘要 v1: "[user] Redis 怎么查慢查询？; [assistant] 用 SLOWLOG GET..."
摘要 v2: "[user] 帮我调阈值; [assistant] 已调到 80%..."
→ 最终: "[对话历史摘要] ...之前的内容压缩版..."
```

3. **会话管理**：全局 `sessionMap` 管理所有会话，最多 500 个会话，每 10 分钟清理一次超过 2 小时未访问的会话。

```go
const (
    maxSessions     = 500                // 最大会话数
    sessionTTL      = 2 * time.Hour      // 会话过期时间
    cleanupInterval = 10 * time.Minute   // 清理间隔
)
```

> **面试要点**：滑动窗口 + 摘要链设计，保证内存可控的同时不丢失历史信息。

### 3.2 长期记忆 — LongTermMemory

**定位**：跨会话持久化的关键信息，让 LLM 知道"这个用户/项目的背景"。

#### 3.2.1 记忆类型（MemoryType）

| 类型 | 含义 | 默认置信度 | 示例 |
|------|------|-----------|------|
| `fact` | 事实信息 | 0.70 | "checkoutservice 部署在 K8s 1.28" |
| `preference` | 用户偏好 | 0.85 | "用户 John 喜欢简洁回答" |
| `procedure` | 操作流程 | 0.75 | "Redis 慢查询排查：先 slowlog，再看 bigkeys" |
| `episode` | 事件经历 | 0.50 | "上周三发生过一次 Redis OOM" |

```go
const (
    MemoryTypeFact       MemoryType = "fact"
    MemoryTypePreference MemoryType = "preference"
    MemoryTypeProcedure  MemoryType = "procedure"
    MemoryTypeEpisode    MemoryType = "episode"
)
```

#### 3.2.2 记忆作用域（MemoryScope）

作用域控制记忆的**可见范围**，从小到大的层级关系：

```
Session (会话级)  →  只有本会话可见
    ↓ 提升
User (用户级)     →  同一用户的所有会话可见
    ↓ 提升
Project (项目级)  →  同一项目的所有成员可见
    ↓ 提升
Global (全局级)   →  所有用户和会话可见
```

```go
const (
    MemoryScopeSession MemoryScope = "session"
    MemoryScopeUser    MemoryScope = "user"
    MemoryScopeProject MemoryScope = "project"
    MemoryScopeGlobal  MemoryScope = "global"
)
```

**作用域提升**：一条记忆可以从 session 提升到 user/project/global。比如用户 John 在一次会话中说"我习惯先看日志再看告警"，被提取为 session 级偏好。当这个偏好被多次确认后，可以 Promote 为 user 级。

```go
// Promote - 提升记忆作用域
func (ltm *LongTermMemory) Promote(ctx context.Context, id string,
    scope MemoryScope, scopeID string, confidence float64) bool {
    // 更新 scope、scopeID、confidence
    // 标记 safety_label = "internal"
}
```

#### 3.2.3 MemoryEntry 完整字段

```go
type MemoryEntry struct {
    ID            string      // 唯一标识（SHA256 前 8 位）
    SessionID     string      // 来源会话
    Type          MemoryType  // fact/preference/procedure/episode
    Content       string      // 记忆内容
    Source        string      // 来源（conversation/user_input/memory_agent）
    Scope         MemoryScope // 作用域
    ScopeID       string      // 作用域 ID
    Confidence    float64     // 置信度 (0~1)
    SafetyLabel   string      // 安全标签 (internal/trusted_internal/safe/disabled)
    Provenance    string      // 溯源标记 (extractor:rule_fact / memory_agent)
    UpdatePolicy  string      // 更新策略 (reinforce/superseded/disabled)
    ConflictGroup string      // 冲突组（同组内的记忆会互斥）
    ExpiresAt     int64       // 过期时间戳（毫秒）
    AllowTriage   bool        // 是否允许在 triage 阶段使用
    Relevance     float64     // 当前相关性分数
    AccessCnt     int         // 被检索次数
    CreatedAt     time.Time
    UpdatedAt     time.Time
    LastUsed      time.Time   // 最后一次被使用的时间
    Decay         float64     // 时间衰减系数
}
```

**每个字段都有明确的目的**：

| 字段组 | 字段 | 作用 |
|--------|------|------|
| 标识 | ID, SessionID | 唯一标识 + 来源追溯 |
| 分类 | Type, Scope, ScopeID | 类型 + 可见范围 |
| 质量 | Confidence, SafetyLabel, Provenance | 可信度 + 安全分级 + 溯源 |
| 生命周期 | ExpiresAt, UpdatePolicy, ConflictGroup | 过期 + 更新冲突处理 |
| 使用统计 | Relevance, AccessCnt, LastUsed, Decay | 相关性衰减计算 |

#### 3.2.4 相关性衰减算法

```go
func computeRelevance(entry *MemoryEntry) float64 {
    // 时间衰减：24 小时不使用，衰减到原来的 50%
    hoursSinceUse := time.Since(entry.LastUsed).Hours()
    decay := 1.0 / (1.0 + hoursSinceUse/24.0)

    // 访问频率加分：每多访问 1 次 +30%，上限 3x
    frequency := 1.0 + float64(entry.AccessCnt-1)*0.3
    if frequency > 3.0 {
        frequency = 3.0
    }

    return decay * frequency
}
```

**这个公式意味着**：

```
刚创建的记忆: decay=1.0, freq=1.0 → Relevance = 1.0
24 小时未用:  decay=0.5, freq=1.0 → Relevance = 0.5
经常使用(10次): decay=0.5, freq=3.0 → Relevance = 1.5
长期不用(7天):  decay=0.125, freq=1.0 → Relevance = 0.125 ← 接近遗忘阈值
```

> **面试要点**：这不是简单的 "最近使用"，而是同时考虑**新鲜度**（decay）和**使用频率**（frequency），是一个简化版的 spaced repetition 算法。

### 3.3 工作记忆 — ToolItems

**定位**：当前请求中 Agent 调用的工具返回结果，让 LLM 知道"刚才查到了什么"。

工作记忆不是独立的存储系统，而是 Context Engine 在装配上下文时，将工具调用结果作为 `ToolItems` 纳入 `ContextPackage`：

```go
type ContextPackage struct {
    // ...
    ToolItems []ContextItem  // ③ 工作记忆：当前工具调用结果
    // ...
}
```

| 属性 | 说明 |
|------|------|
| 生命周期 | 当前请求（用完即弃） |
| 内容 | 工具调用结果（日志查询、告警查询、RAG 检索等） |
| 预算 | 默认 2000 token |
| 超预算处理 | 先裁剪（trim），裁剪后仍空则丢弃 |

---

## 4. 记忆写入流程

### 4.1 整体链路

对话结束后，记忆写入经历以下阶段：

```
┌──────────────────────────────────────────────────────────────────┐
│                      记忆写入流程                                  │
│                                                                  │
│  对话结束（用户消息 + AI 回复）                                     │
│    │                                                             │
│    ▼                                                             │
│  ┌─────────────────────┐                                         │
│  │ PersistOutcome()     │  ← MemoryService 入口                   │
│  │ sessionID, query,    │                                         │
│  │ summary              │                                         │
│  └─────────┬───────────┘                                         │
│            │                                                     │
│            ├──▶ ① SimpleMemory.AddUserAssistantPair (同步)        │
│            │     └─ 存入短期记忆，更新对话窗口                      │
│            │                                                     │
│            ├──▶ ② MQ 异步提取（优先）                              │
│            │     └─ 消息队列投递 → 异步 worker 处理                │
│            │     └─ 投递成功 → 直接返回                            │
│            │                                                     │
│            └──▶ ③ 本地 goroutine 提取（MQ 不可用时降级）           │
│                  │                                               │
│                  ├─ acquireMemoryExtractionSlot                   │
│                  │  └─ 信号量限流（默认 max 8 并发）                │
│                  │                                               │
│                  ├─ go func() { ... }(ctx)                        │
│                  │  └─ context.WithoutCancel + WithTimeout        │
│                  │                                               │
│                  └─ processMemoryEventFunc(ctx, event)            │
│                     │                                            │
│                     ├─ ④ Agent.Decide（LLM 或 Rule）               │
│                     │  └─ 提取 MemoryAction 列表                   │
│                     │                                            │
│                     ├─ ⑤ ValidateMemoryCandidate（过滤）           │
│                     │  └─ 太短/太长/代码块/套话/密钥 → 丢弃        │
│                     │                                            │
│                     └─ ⑥ LongTermMemory.StoreWithOptions（写入）   │
│                        └─ 去重/冲突处理/驱逐/持久化                  │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 4.2 Step-by-Step 详解

#### Step 1: PersistOutcome 入口

```go
func (s *MemoryService) PersistOutcome(ctx context.Context,
    sessionID, query, summary string) {
    // 空内容直接返回
    if strings.TrimSpace(summary) == "" {
        return
    }

    // ① 同步写入短期记忆
    sessionMem := mem.GetSimpleMemory(sessionID)
    sessionMem.AddUserAssistantPair(query, summary)

    // ② 尝试 MQ 异步提取
    enqueued, err := enqueueMemoryExtraction(ctx, sessionID, query, summary)
    if enqueued {
        return  // MQ 投递成功，后续由 worker 处理
    }

    // ③ MQ 不可用，本地 goroutine 降级
    release, err := acquireMemoryExtractionSlot(ctx)
    if err != nil {
        return  // 并发满，放弃本次提取
    }

    go func(parent context.Context) {
        defer release()
        extractCtx, cancel := boundedMemoryContext(parent)
        defer cancel()
        report := processMemoryEventFunc(extractCtx,
            memoryEventFromContext(parent, sessionID, query, summary))
        // ... 日志记录丢弃的候选数量 ...
    }(ctx)
}
```

#### Step 2: 短期记忆同步写入

`SimpleMemory.AddUserAssistantPair` 做三件事：

1. 追加 `user` 和 `assistant` 两条消息
2. `turnCount++`
3. `trimWindow()`：超窗口时先摘要再移除

这一步是**同步**的，保证短期记忆立即可用。

#### Step 3: MQ 异步提取（优先路径）

`enqueueMemoryExtraction` 尝试将提取任务投递到消息队列。如果配置了 MQ 且投递成功，直接返回——后续由 MQ worker 异步处理。这是生产环境推荐的做法：**解耦请求处理和记忆提取**。

#### Step 4: 本地 goroutine 降级

MQ 不可用时，降级为本地 goroutine 处理。关键保护机制：

**a) 信号量限流**：

```go
func acquireMemoryExtractionSlot(ctx context.Context) (func(), error) {
    maxJobs := memoryExtractionMaxJobs(ctx)  // 默认 8，配置项 memory.extract_max_concurrency
    sem := getOrCreateMemoryExtractionSemaphore(maxJobs)

    select {
    case sem <- struct{}{}:
        return func() { <-sem }, nil  // 获得槽位
    case <-waitCtx.Done():
        return nil, ErrMemoryExtractionLimited  // 超时，放弃
    }
}
```

**b) context.WithoutCancel + WithTimeout**：

```go
func boundedMemoryContext(parent context.Context) (context.Context, context.CancelFunc) {
    base := context.Background()
    if parent != nil {
        base = context.WithoutCancel(parent)  // ← 关键！断掉父 context 的取消链
    }
    return context.WithTimeout(base, memoryExtractionTimeout(parent))
}
```

**为什么用 `context.WithoutCancel`？** 用户的 HTTP 请求可能已经返回了（原始 ctx 被取消），但记忆提取是后台任务，必须继续执行。断掉取消链后，只用 `WithTimeout` 控制提取本身的最大耗时（默认 1500ms，配置项 `memory.extract_timeout_ms`）。

> **面试要点**：`context.WithoutCancel` 是 Go 1.21+ 的特性，用于后台任务脱离请求生命周期。这是生产级异步处理的标准做法。

#### Step 5: Agent 决策 — LLM 还是 Rule？

```go
func processMemoryEventWithConfiguredAgent(ctx context.Context, event mem.MemoryEvent) *mem.MemoryExtractionReport {
    // 默认 Rule Agent
    agent := mem.MemoryAgent(mem.NewRuleMemoryAgent())

    // 如果配置了 LLM 模式且有有效的 API Key
    if loadMemoryAgentMode(ctx) == "llm" {
        if memoryAgentLLMConfigured(ctx) {
            chatModel, err := newMemoryChatModel(ctx)
            if err == nil {
                // 启用 LLM Agent，Rule Agent 作为 fallback
                agent = mem.NewLLMMemoryAgent(chatModel, mem.NewRuleMemoryAgent())
            }
        }
    }

    return mem.ProcessMemoryEventWithAgent(ctx, event, agent)
}
```

**两种 Agent 对比**：

| | Rule Agent | LLM Agent |
|---|---|---|
| 实现 | 关键词匹配 + 正则 | GLM Fast 模型推理 |
| 提取质量 | 精确但覆盖有限 | 理解语义，覆盖更广 |
| 延迟 | ~0ms | ~数百ms |
| 成本 | 免费 | API 调用费用 |
| 适用场景 | 开发/测试 | 生产环境 |
| 失败降级 | — | LLM 失败 → Rule Agent 兜底 |

**Rule Agent 的提取逻辑**（`extraction.go`）：

- **事实提取**：匹配"服务名""IP地址""端口""数据库名"等运维关键词 → 包含这些关键词的句子作为 fact
- **偏好提取**：匹配"我喜欢""我希望""请用""不要用""每次都"等表达 → 作为 preference
- **Scope 推断**：preference 类型 + 有 UserID → 自动提升为 User Scope

**LLM Agent 的 System Prompt 核心要求**：

```go
// 要点（翻译自代码）：
"只保存长期稳定、有复用价值的信息：用户偏好、项目约定、服务事实、排障流程"
"不要保存临时闲聊、模型套话、代码块、密钥、token、password"
"用户个人偏好用 user scope；会话事实用 session scope"
```

#### Step 6: ValidateMemoryCandidate — 候选验证

在写入长期记忆前，每一条候选都要经过严格验证：

```go
func ValidateMemoryCandidate(candidate MemoryCandidate) (bool, string) {
    // ❌ 空内容
    if content == "" → "empty"

    // ❌ 太短（< 4 字符）
    if len(content) < 4 → "too_short"

    // ❌ 太长（> 500 字符）
    if len(content) > 500 → "too_long"

    // ❌ 换行太多（> 3 行）
    if strings.Count(content, "\n") > 3 → "too_many_lines"

    // ❌ 包含代码块
    if strings.Contains(content, "```") → "contains_code_block"

    // ❌ AI 套话（"作为AI""抱歉""请提供更多信息"）
    if contains boilerplate → "assistant_boilerplate"

    // ❌ 疑似包含密钥（api_key, password, authorization:, bearer 等）
    if contains secret markers → "contains_secret_marker"

    // ✅ 通过所有检查
    return true, ""
}
```

#### Step 7: StoreWithOptions — 写入长期记忆

```go
func (ltm *LongTermMemory) StoreWithOptions(ctx context.Context, sessionID string,
    memType MemoryType, content string, source string, opts MemoryStoreOptions) string {

    ltm.mu.Lock()
    defer ltm.mu.Unlock()

    id := generateMemoryID(opts.Scope, opts.ScopeID, content)

    // 情况 A: 已存在 → 强化
    if existing, ok := ltm.entries[id]; ok {
        existing.AccessCnt++
        existing.LastUsed = now
        if opts.Confidence > existing.Confidence {
            existing.Confidence = opts.Confidence  // 置信度取高
        }
        // 处理冲突记忆（同 ConflictGroup 的旧记忆标记为 superseded）
        ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
        ltm.persistLocked(ctx)  // 持久化到文件
        return id
    }

    // 情况 B: 新记忆
    // ① 先驱逐（如果超过上限）
    ltm.evictIfNeededLocked(ctx, sessionID)
    // ② 处理冲突
    ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
    // ③ 写入
    ltm.entries[id] = entry
    ltm.index[sessionID] = append(ltm.index[sessionID], id)
    // ④ 持久化
    ltm.persistLocked(ctx)
    return id
}
```

**写入时的关键行为**：

| 场景 | 行为 |
|------|------|
| 相同 ID 已存在 | 强化（AccessCnt++, 置信度取高），不创建新条目 |
| 新记忆 | 写入，可能触发驱逐和冲突处理 |
| 同 ConflictGroup | 旧记忆标记为 `superseded`，置信度减半，立即过期 |
| 超过全局上限 | 驱逐相关性最低的记忆（`evictOneLocked`） |
| 超过会话上限 | 驱逐本会话相关性最低的记忆 |

---

## 5. 记忆读取流程

### 5.1 整体链路

```
请求到达
    │
    ▼
┌─────────────────────────────────────────────┐
│  MemoryService.BuildContextPlan()            │
│  mode, sessionID, query                      │
└─────────────────┬───────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────┐
│  Assembler.Assemble()  ← Context Engine      │
│  ┌─────────────────────────────────────────┐ │
│  │ ① selectMemories()                      │ │
│  │   └─ LongTermMemory.RetrieveScoped()    │ │
│  │      └─ 按 Scope 范围检索                │ │
│  │                                         │ │
│  │ ② 六层过滤                               │ │
│  │   ├─ 过期检查                             │ │
│  │   ├─ Scope 匹配                           │ │
│  │   ├─ 置信度阈值                           │ │
│  │   ├─ 安全标签                             │ │
│  │   ├─ 数量窗口                             │ │
│  │   └─ Token 预算                           │ │
│  │                                         │ │
│  │ ③ 组装到 ContextPackage.MemoryItems      │ │
│  └─────────────────────────────────────────┘ │
│                                               │
│  返回: contextText + MemoryRef[] + trace[]    │
└───────────────────────────────────────────────┘
```

### 5.2 RetrieveScoped — 检索 + 排序

```go
func (ltm *LongTermMemory) RetrieveScoped(ctx context.Context, query string,
    limit int, policy MemoryRetrievePolicy) []*MemoryEntry {

    ltm.mu.RLock()  // ← 读锁！
    // ... 遍历所有 entries ...
    ltm.mu.RUnlock()

    // 1. Scope 过滤：只保留 policy.ScopeRefs 中指定的 Scope
    // 2. 过期过滤：policy.IncludeExpired=false 时跳过过期记忆
    // 3. 相关性过滤：Relevance < 0.1 的直接丢弃

    // 4. 关键词匹配加分
    //    对 query 的每个词做 content 包含匹配
    //    匹配数越多，score 越高

    // 5. 按 score 降序排列

    // 6. 取 Top-K，更新选中条目的 AccessCnt + LastUsed
    //    但只短暂获取写锁：
    if len(selectedIDs) > 0 && !policy.ReadOnly {
        ltm.mu.Lock()       // ← 写锁（短暂）
        // 更新 AccessCnt、LastUsed、Relevance
        ltm.persistLocked(ctx)
        ltm.mu.Unlock()
    }

    return result
}
```

> **面试要点**：检索过程大部分时间只持有 **RLock（读锁）**，仅在最后更新使用统计时才获取 **WLock（写锁）**。这是一个经典的读写分离优化。如果整个检索过程都持有写锁，高并发时所有请求会串行化。

### 5.3 六层过滤（在 Context Engine 的 selectMemories 中）

从长期记忆检索出来的候选，还要经过 Context Engine 的六层过滤才能进入最终上下文：

```
RetrieveScoped 返回的记忆列表
    │
    ├─ ① 过期检查
    │    ExpiresAt > 0 && ExpiresAt <= now → 丢弃 (memory_expired)
    │
    ├─ ② Scope 匹配
    │    不在 Profile.AllowedMemoryScopes 内 → 丢弃 (memory_scope)
    │
    ├─ ③ 置信度过滤
    │    Confidence < MinMemoryConfidence → 丢弃 (memory_confidence)
    │
    ├─ ④ 安全标签过滤
    │    SafetyLabel 不是 internal/trusted_internal/safe → 丢弃 (memory_safety)
    │
    ├─ ⑤ 数量窗口
    │    已选数量 >= MaxMemoryItems → 丢弃 (memory_window)
    │
    └─ ⑥ Token 预算
         TokenEstimate > remaining budget → 丢弃 (memory_budget)
         （记忆不裁剪，超预算直接丢弃）
```

**每一层被丢弃的记忆都会记录在 ContextTrace 中**，方便排查"为什么 LLM 没看到某条记忆"。

### 5.4 记忆注入上下文

最终选中的记忆被格式化为 `ContextItem`，放入 `ContextPackage.MemoryItems`：

```go
type ContextItem struct {
    ID              string  // 记忆 ID
    Title           string  // 记忆类型 (fact/preference/...)
    Content         string  // 记忆内容
    Scope           string  // 作用域
    Confidence      float64 // 置信度
    SourceID        string  // 来源
    Provenance      string  // 溯源
    TokenEstimate   int
    Dropped         bool
    DroppedReason   string
    CompressionLevel string
}
```

在 `chat` 模式下，记忆以 `[关键记忆]` 前缀消息的形式注入到对话流中：

```go
// BuildEnrichedContext - 记忆注入
result = append(result, &schema.Message{
    Role:    schema.User,
    Content: fmt.Sprintf("[关键记忆]\n- [fact] checkoutservice 部署在 K8s 1.28\n- [preference] 用户喜欢简洁回答"),
})
result = append(result, schema.AssistantMessage("好的，我已了解这些背景信息。", nil))
result = append(result, shortTermMsgs...)  // 然后是实际的对话历史
```

---

## 6. 关键设计点

### 6.1 异步提取：context.WithoutCancel + WithTimeout

```
用户 HTTP 请求的生命周期:
  Request → Handler → [PersistOutcome] → Response
                           │
                           │ go func(parent) {
                           │   ctx = context.WithoutCancel(parent)  ← 断掉取消链
                           │   ctx, cancel = context.WithTimeout(ctx, 1500ms)
                           │   defer cancel()
                           │   // 提取记忆...
                           │ }()
                           │
                           ▼ (Request 已返回，但 goroutine 继续跑)
                    提取结果写入长期记忆
```

| 机制 | 作用 |
|------|------|
| `context.WithoutCancel` | 防止父 context 取消导致提取中断 |
| `context.WithTimeout` | 防止提取任务无限挂起（默认 1500ms） |
| 信号量 | 防止提取 goroutine 爆炸（默认 max 8） |
| MQ 优先 | 生产环境走消息队列，不消耗 HTTP 进程资源 |

### 6.2 并发控制：信号量限流

```go
var (
    memoryExtractSemaphoreMu sync.Mutex
    memoryExtractSemaphore   chan struct{}  // 有缓冲 channel 作为信号量
    memoryExtractSemaphoreN  int
)

func acquireMemoryExtractionSlot(ctx context.Context) (func(), error) {
    maxJobs := memoryExtractionMaxJobs(ctx)  // 默认 8
    sem := getOrCreateMemoryExtractionSemaphore(maxJobs)

    waitCtx, cancel := context.WithTimeout(ctx, memoryExtractionWait(ctx))  // 默认 50ms
    defer cancel()

    select {
    case sem <- struct{}{}:
        return func() { <-sem }, nil  // 获得槽位，返回释放函数
    case <-waitCtx.Done():
        return nil, ErrMemoryExtractionLimited  // 50ms 内没拿到槽位，放弃
    }
}
```

**设计要点**：

- 不是无限制地创建 goroutine，而是用有缓冲 channel 作为信号量控制并发
- 等待超时 50ms（配置项 `memory.extract_wait_timeout_ms`），拿不到槽位就放弃（不阻塞请求）
- 信号量大小可配置（配置项 `memory.extract_max_concurrency`）

### 6.3 Agent 模式：LLM → Rule Fallback

```
尝试 LLM 提取:
  ├─ 检查 memory.agent_mode == "llm"
  ├─ 检查 API Key 是否有效
  ├─ 创建 LLMMemoryAgent(RuleMemoryAgent 作为 fallback)
  │
  ├─ LLM.Generate() 成功 → 解析 JSON → 返回 MemoryDecision
  │
  └─ LLM.Generate() 失败 → fallback.Decide() → Rule 提取结果
       ├─ LLM 返回空
       ├─ JSON 解析失败
       ├─ Context 已取消
       └─ 任何其他错误
```

> **面试要点**：这是典型的 **graceful degradation** 模式。LLM 不可用时系统不会崩溃，而是降级到规则提取。这在运维场景中尤为重要——不能因为记忆提取失败影响主链路。

### 6.4 容量控制：多级上限

```
┌─────────────────────────────────────────────┐
│              容量控制体系                      │
│                                             │
│  全局上限: long_term_max_entries (默认 1000)  │
│  ┌───────────────────────────────────────┐  │
│  │  会话上限: per_session (默认 100)       │  │
│  │  ┌─────────────────────────────────┐  │  │
│  │  │  单条内容: 4 ~ 500 字符           │  │  │
│  │  │  单条行数: ≤ 3 行               │  │  │
│  │  └─────────────────────────────────┘  │  │
│  └───────────────────────────────────────┘  │
│                                             │
│  驱逐策略: 按 computeRelevance() 排序         │
│  淘汰相关性最低的记忆                         │
└─────────────────────────────────────────────┘
```

```go
func (ltm *LongTermMemory) evictIfNeededLocked(ctx context.Context, sessionID string) {
    // 1. 先检查 per-session 上限
    for maxPerSession > 0 && len(ltm.index[sessionID]) >= maxPerSession {
        ltm.evictOneLocked(ctx, ltm.index[sessionID])
    }
    // 2. 再检查全局上限
    for maxEntries > 0 && len(ltm.entries) >= maxEntries {
        ltm.evictOneLocked(ctx, allIDs)
    }
}
```

### 6.5 RLock 优化：读写分离

```go
func (ltm *LongTermMemory) RetrieveScoped(...) []*MemoryEntry {
    ltm.mu.RLock()              // ← 读锁，多 goroutine 可并发
    // 遍历、过滤、排序...        // ← 大部分时间在这里
    ltm.mu.RUnlock()

    // 只在更新 AccessCnt 时短暂获取写锁
    if len(selectedIDs) > 0 && !policy.ReadOnly {
        ltm.mu.Lock()            // ← 写锁，互斥
        // 更新 AccessCnt、LastUsed、Relevance
        // 持久化
        ltm.mu.Unlock()          // ← 立即释放
    }
}
```

**为什么这样设计？**

- 检索操作（读）频率远高于写入操作
- 如果整个检索过程都持写锁，高并发时所有检索请求都会串行化
- 分离后，99% 的时间在 RLock 下并发执行，只有更新统计信息的瞬间才需要写锁

### 6.6 后台清理：定时清理过期会话

```go
func startCleanup() {
    cleanupOnce.Do(func() {
        go func() {
            ticker := time.NewTicker(10 * time.Minute)
            defer ticker.Stop()
            for range ticker.C {
                sessionMu.Lock()
                now := time.Now()
                for id, entry := range sessionMap {
                    if now.Sub(entry.lastAccess) > 2*time.Hour {
                        delete(sessionMap, id)  // 清理超过 2 小时未访问的会话
                    }
                }
                sessionMu.Unlock()
            }
        }()
    })
}
```

> **面试要点**：后台 goroutine 不是针对长期记忆（LongTermMemory 通过 `ExpiresAt` + 检索时过滤 + `Forget` 主动清理），而是针对短期记忆的会话管理（`sessionMap`），防止内存泄漏。

### 6.7 冲突处理：ConflictGroup + Supersede

当新的记忆与旧记忆属于同一 `ConflictGroup`（如都是关于"Redis maxconn"的事实），旧记忆不会直接删除，而是：

```
旧记忆:
  SafetyLabel → "superseded"
  UpdatePolicy → "superseded"
  ExpiresAt → now（立即过期）
  Confidence → Confidence * 0.5（置信度减半）
```

这样做的好处：
- **可审计**：旧记忆没有物理删除，可以追溯历史
- **自动清理**：标记为过期后，下次检索时会被过滤掉
- **可恢复**：如果新记忆后来被证明是错误的，可以从审计记录恢复

---

## 7. 面试问答

### Q1: 记忆系统怎么设计？三层分别是什么？

<details>
<summary>点击查看答案</summary>

**三层记忆架构**，对应认知科学中"工作记忆 → 短期记忆 → 长期记忆"模型：

| 层级 | 实现 | 生命周期 | 作用 |
|------|------|---------|------|
| **短期记忆** | `SimpleMemory` + `HistoryMessages` | 当前会话 | 保持对话连贯性 |
| **长期记忆** | `LongTermMemory`（文件持久化） | 跨会话持久 | 记住用户偏好、项目背景、历史经验 |
| **工作记忆** | 当前 `ToolItems` | 当前请求 | 让 LLM 基于实时工具结果推理 |

**核心设计原则**：

1. **三层分离**：速度（短期）vs 持久性（长期）vs 即时性（工作）各有侧重
2. **异步提取**：对话结束后异步提取长期记忆，不阻塞主链路
3. **多级过滤**：写入有 ValidateMemoryCandidate，读取有六层过滤
4. **容量控制**：全局上限 + 会话上限 + 按相关性驱逐
5. **读写分离**：检索用 RLock，写入用 WLock，提高并发

**一句话**：三层记忆各司其职，短期保连贯、长期记关键、工作管当下，异步提取不阻塞，多级过滤保质量。

</details>

### Q2: 长期记忆怎么提取的？

<details>
<summary>点击查看答案</summary>

**提取链路**：对话结束 → MQ/本地 goroutine 异步 → Agent 决策 → 候选验证 → 写入存储。

**关键步骤**：

1. **异步触发**：`PersistOutcome` 先同步写短期记忆，然后异步提取长期记忆
   - 优先走 MQ（解耦、削峰）
   - MQ 不可用时降级为本地 goroutine（信号量限流，默认 max 8）
   - 使用 `context.WithoutCancel` 脱离请求生命周期

2. **Agent 决策**：
   - **Rule Agent**：关键词匹配（"服务名""IP地址""我喜欢""不要用"等）
   - **LLM Agent**：GLM 推理（理解语义，覆盖面更广）
   - LLM 失败时自动 fallback 到 Rule Agent

3. **候选验证**：`ValidateMemoryCandidate` 六重过滤
   - 空内容、太短（<4字符）、太长（>500字符）
   - 换行太多（>3行）、包含代码块
   - AI 套话、疑似密钥

4. **写入存储**：去重（相同 SHA256 → 强化）、冲突处理（ConflictGroup → supersede）、容量控制（超上限驱逐）

**一句话**：Rule 规则做快速提取，LLM 语义做深度理解，失败有降级，写入有验证，保证提取质量和系统稳定性。

</details>

### Q3: 怎么防止记忆无限增长？

<details>
<summary>点击查看答案</summary>

**五层防线**：

| 防线 | 机制 | 配置 |
|------|------|------|
| **① 写入过滤** | `ValidateMemoryCandidate`：太短/太长/代码块/套话/密钥直接拒绝 | 硬编码规则 |
| **② 全局上限** | `long_term_max_entries`：超过后驱逐相关性最低的记忆 | 默认 1000 |
| **③ 会话上限** | `long_term_max_entries_per_session`：单会话超过后驱逐 | 默认 100 |
| **④ 过期机制** | `ExpiresAt`：带过期时间的记忆到期自动过滤；conflict 标记 superseded 立即过期 | 可选字段 |
| **⑤ 衰减遗忘** | `computeRelevance`：时间衰减 × 使用频率。Relevance < 0.1 不再被检索；`Forget(threshold)` 主动清理 | 24h 不使用衰减到 50% |

**驱逐策略**：

```go
// 选择驱逐目标时，按相关性分数升序排列
// 相关性最低的（最久未使用 + 最少被访问）最先被驱逐
sort.Slice(candidateIDs, func(i, j int) bool {
    return computeRelevance(entries[i]) < computeRelevance(entries[j])
})
ltm.removeEntryLocked(candidateIDs[0])  // 驱逐最低分
```

**一句话**：写入有门槛、存储有上限、过期自动清、不用的自然遗忘，五层防线确保记忆库不会无限膨胀。

</details>

---

## 8. 自测

### 问题 1

用户 John 在一次会话中说"我喜欢简洁的回答，先给结论再解释"。这条偏好信息会经历怎样的旅程，从短期记忆最终成为跨会话可用的长期记忆？请描述完整链路。

<details>
<summary>点击查看答案</summary>

**旅程链路**：

1. **短期记忆阶段**：`PersistOutcome` 调用 → `SimpleMemory.AddUserAssistantPair` → 用户消息和 AI 回复存入当前会话的 `Messages` 列表

2. **异步提取阶段**：
   - MQ 投递（优先）或本地 goroutine（降级）
   - `ExtractMemoryCandidates` 被调用
   - Rule Agent 匹配到"我喜欢"关键词 → 生成 `preference` 类型候选
   - 置信度默认 0.85

3. **候选验证阶段**：
   - `ValidateMemoryCandidate` 检查：长度 > 4 ✓，不包含代码块 ✓，不是套话 ✓ → 通过

4. **Scope 推断**：
   - 因为类型是 `preference` 且 `UserID` 非空
   - `RuleMemoryAgent.Decide` 自动将 Scope 从 `session` 提升为 `user`
   - `ScopeID` 设为 John 的用户 ID

5. **写入长期记忆**：
   - 生成 SHA256 ID（基于 scope + scopeID + content）
   - `StoreWithOptions` → 写入 `LongTermMemory.entries`
   - 持久化到文件

6. **跨会话可用**：下次 John 打开新会话 → `BuildContextPlan` → `RetrieveScoped(user scope)` → 六层过滤 → 注入到 `[关键记忆]` 前缀消息 → LLM 看到"用户喜欢简洁回答，先给结论再解释"

</details>

### 问题 2

为什么检索时大部分时间用 RLock，只在更新 AccessCnt 时短暂获取 WLock？如果整个检索过程都用 WLock 会有什么后果？

<details>
<summary>点击查看答案</summary>

**原因**：读写分离，提高并发性能。

**用 RLock 的场景**：遍历所有记忆条目、计算相关性分数、关键词匹配排序。这些操作只读不改，多个 goroutine 可以**同时进行**。

**用 WLock 的场景**：更新 `AccessCnt`、`LastUsed`、`Relevance`，以及持久化。这些操作会修改数据，必须互斥。

**如果全程 WLock 会怎样？**

```
假设 10 个并发请求同时检索记忆：

用 RLock + 短暂 WLock:
  请求1: RLock(遍历排序) → WLock(毫秒级更新) → 返回
  请求2: RLock(遍历排序) → WLock(毫秒级更新) → 返回
  ...
  10 个请求几乎并行，总耗时 ≈ 单次检索时间

用全程 WLock:
  请求1: WLock → 遍历排序 → 更新 → WUnlock → 返回
  请求2: 等待... → WLock → 遍历排序 → 更新 → WUnlock → 返回
  ...
  10 个请求串行执行，总耗时 ≈ 单次检索时间 × 10
```

**设计原则**：读多写少的场景用 `sync.RWMutex` 是标准做法。检索是高频操作，写入（更新统计信息）只是检索的附属操作，所以 RLock 覆盖主流程，WLock 只覆盖必要的写入点。

</details>

### 问题 3

`ValidateMemoryCandidate` 中有一个检查是过滤 `assistant_boilerplate`（AI 套话）。为什么要过滤这些？如果不过滤会怎样？

<details>
<summary>点击查看答案</summary>

**为什么要过滤 AI 套话？**

AI 的回复中经常包含一些固定的礼貌用语和免责声明，这些内容**没有信息量**但却会被提取为记忆候选：

```
AI 回复: "作为AI助手，我无法直接操作你的服务器，但我可以提供排查建议..."
         ^^^^^^^^ 套话，没有信息量
```

被过滤的套话包括：
- "作为AI"/"作为一个AI"
- "抱歉"/"对不起"
- "请提供更多信息"
- "无法直接"

**不过滤的后果**：

1. **记忆污染**：长期记忆里充斥着"作为AI助手，我无法..."这类无意义内容
2. **浪费容量**：全局上限 1000 条，套话占了 200 条，真正有用的记忆被驱逐
3. **噪声干扰**：检索时套话记忆也会被返回，LLM 看到的上下文里混入无意义信息
4. **连锁反应**：这些记忆可能被 LLM Agent 再次提取、强化，形成恶性循环

**类似的过滤还有**：
- `contains_code_block`：代码块信息量大但不应作为"记忆"存储
- `contains_secret_marker`：防止 API Key、密码等敏感信息进入记忆库
- `too_long` / `too_short`：极端长度的内容通常不是有效的可复用记忆

**一句话**：ValidateMemoryCandidate 是记忆质量的第一道关卡，过滤的是"看起来像记忆但不是"的内容，保证长期记忆的质量和密度。

</details>

---

> **下一章预告**：Multi-Agent Runtime — 多 Agent 协作运行时，Supervisor 如何编排 Triage → Specialists → Reporter 的完整链路。
