# OpsCaptain 记忆系统评审报告

> 评审日期：2026-06-05
> 评审范围：`internal/ai/memory/`、`internal/ai/service/memory_service.go`、`internal/ai/service/memory_queue.go`、`internal/ai/contextengine/assembler.go`
> 结论：系统设计合理，但有 3 个严重问题直接影响效果，需要优先修复

---

## 一、架构概览

### 三层记忆体系

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户对话                                  │
│                           ↓                                     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Session Memory（短期，内存）                              │   │
│  │  • 滑动窗口，默认 20 条消息                                │   │
│  │  • 溢出时自动摘要（2000 字符上限）                          │   │
│  │  • 2 小时 TTL，LRU 淘汰（最多 500 会话）                   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                           ↓ PersistOutcome                       │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Memory Agent（提取决策）                                  │   │
│  │  • RuleAgent：关键词匹配提取事实                           │   │
│  │  • LLMAgent：大模型判断（skip/upsert/supersede/promote）   │   │
│  │  • 异步执行：RabbitMQ + 本地 goroutine 双通道              │   │
│  └──────────────────────────────────────────────────────────┘   │
│                           ↓ Store                                │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Long-Term Memory（持久化）                                │   │
│  │  • 默认纯内存，可选文件 JSON 持久化（需配置 store_path）    │   │
│  │  • 内容寻址 ID：SHA256(scope:scopeID:content)              │   │
│  │  • 4 级作用域：session / user / project / global           │   │
│  │  • 容量：全局 1000 条，每会话 100 条                        │   │
│  │  • 淘汰：relevance = decay × frequency                    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                           ↓ Retrieve + Rank                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Context Assembler（上下文注入）                            │   │
│  │  • 综合评分：confidence×0.5 + freshness×0.3 + scope×0.2   │   │
│  │  • Token 预算：memory 占 10%（800 tokens）                 │   │
│  │  • 注入方式：[关键记忆] user/assistant 消息对               │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 数据流

```
对话结束 → PersistOutcome
  ├── 1. Session Memory: AddUserAssistantPair（滑动窗口）
  ├── 2. RabbitMQ Enqueue（异步提取）
  │     └── Consumer → MemoryAgent → LTM.Store（内存，可选文件持久化）
  │           ├── 成功 → ACK
  │           ├── 失败 → Retry Queue（2s TTL）→ 重试 3 次
  │           └── 耗尽 → DLQ（人工处理）
  └── 3. 本地降级：goroutine + semaphore（RabbitMQ 不可用时）

对话开始 → BuildContext
  ├── 1. Session Memory → GetContextMessages（历史 + 摘要）
  ├── 2. LTM.RetrieveScoped → 按综合评分排序 → Top 5
  ├── 3. Token 预算裁剪
  └── 4. 注入为 [关键记忆] 消息对
```

### 关键配置项

| 配置键 | 默认值 | 说明 |
|--------|--------|------|
| `memory.max_window_size` | 20 | 会话滑动窗口大小 |
| `memory.max_context_tokens` | 8000 | 上下文 token 总预算 |
| `memory.agent_mode` | "rule" | 提取模式：rule / llm |
| `memory.long_term_max_entries` | 1000 | 全局记忆容量上限 |
| `memory.long_term_max_entries_per_session` | 100 | 每会话记忆容量上限 |
| `memory.long_term_store_path` | (空) | 可选持久化文件路径，空则纯内存 |
| `memory.extract_timeout_ms` | 1500 | 提取超时 |
| `memory.extract_max_concurrency` | 8 | 本地提取并发上限 |
| `context.chat_max_memory_items` | 5 | 最多注入几条记忆 |
| `context.min_memory_confidence` | 0.50 | 最低置信度阈值 |

---

## 二、问题清单

### 🔴 严重问题（直接影响效果）

#### 问题 1：Token 预算严重不足

**位置**：`token_budget.go:38`（预算分配）、`extraction.go:88`（内容长度校验）

**现状**：
```
总预算：8000 tokens
分配：System 20% (1600) | Memory 10% (800) | History 40% (3200) | Documents 15% (1200) | 剩余 15%
```

**问题**：
- Memory 只有 800 tokens
- 每条记忆内容上限 500 字节（`len(content) > 500`），中文 UTF-8 每字约 3 字节 → 约 166 个汉字 ≈ 249 tokens
- 800 tokens 理论上能放约 3 条中文记忆，英文记忆可放更多
- `MaxMemoryItems=5` 在中文长记忆场景下通常无法放满
- 记忆系统投入了大量工程（异步提取、冲突检测、衰减淘汰），但中文长记忆场景下通常无法放满 5 条，ROI 打折

**影响**：记忆系统实际可用性很低，大部分记忆永远不会被注入上下文。

**建议**：
- 方案 A：将 memory 预算从 10% 提升到 20%（1600 tokens，约 1000 字符，可放 3-4 条）
- 方案 B：对长记忆做截断摘要（按 rune 或 token budget 截断，复用 `TrimToTokenBudget` 或新增 UTF-8 安全 helper），而非整条丢弃
- 方案 C：A + B 组合

---

#### 问题 2：记忆检索无语义能力

**位置**：`long_term.go:218-300`（`RetrieveScoped`）

**现状**：
```go
// 检索评分 = 关键词匹配 + 相关性衰减
matchScore := float64(matchCount) / float64(totalWords) * 2.0
relevance := computeRelevance(entry)
score := matchScore + relevance
```

**问题**：
- 纯关键词匹配，没有向量语义搜索
- "Redis 连接超时" 搜不到 "Redis timeout"（中英文不匹配）
- "服务挂了" 搜不到 "service down"（同义词不匹配）
- 与 RAG 系统形成矛盾：RAG 用了 Dense + BM25 + RRF 混合检索，记忆系统却只有关键词

**影响**：记忆召回率低，用户换个说法就找不到相关记忆。

**建议**：
- 方案 A（轻量）：复用现有 BM25 索引，对记忆内容做分词检索
- 方案 B（完整）：为记忆建立独立的小型向量索引（Milvus collection 或内存 HNSW）
- 方案 C（最小）：在 RetrieveScoped 里加一个 fuzzy match（编辑距离/子串匹配）

---

#### 问题 3：文件持久化每次全量写入（条件性问题）

**位置**：`long_term.go:743-810`（`fileLongTermMemoryStore`）

**现状**：
```go
// 持久化是可选的，只有配置了 store_path 才启用
func loadLongTermMemoryStore() LongTermMemoryStore {
    v, err := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
    if err != nil || strings.TrimSpace(v.String()) == "" {
        return nil  // 默认：纯内存，不持久化
    }
    return NewFileLongTermMemoryStore(v.String())
}

// 启用后，每次变更全量写入
func (s *fileLongTermMemoryStore) Save(ctx context.Context, entries []*MemoryEntry) error {
    data, _ := json.MarshalIndent(entries, "", "  ")
    os.WriteFile(tmpPath, data, 0o600)  // 0600 权限
    os.Rename(tmpPath, s.path)  // 原子替换
}
```

**问题**：
- **前提**：只有配置了 `memory.long_term_store_path` 才会触发文件持久化
- 默认纯内存模式，重启丢失所有记忆，但无 I/O 问题
- 启用后：每次记忆变更都序列化整个 map 并重写文件
- 1000 条记忆 ≈ 100KB+ JSON → 每次对话结束写 100KB

**影响**：仅在启用文件持久化时生效。高频对话场景下 I/O 开销大。

**建议**：
- 方案 A（增量写入）：用 append-only JSONL + 定期 compaction
- 方案 B（数据库）：SQLite 或 Redis 作为后端（`LongTermMemoryStore` 接口已支持扩展）
- 方案 C（写合并）：debounce 持久化，N 秒内的多次变更合并为一次写入

---

### 🟡 中等问题（影响质量）

#### 问题 4：摘要截断粗暴

**位置**：`session.go:224-229`

**现状**：
```go
if len(summary) > maxSummaryLen {
    summary = summary[len(summary)-maxSummaryLen:]  // 截后 2000 字符
    if idx := strings.IndexByte(summary, '\n'); idx >= 0 {
        summary = summary[idx+1:]  // 跳到下一个换行
    }
}
```

**问题**：
- 截取末尾 2000 字符，丢失早期对话的全部上下文
- 从换行处开始，可能切断段落产生语义断裂
- 没有智能摘要能力（应该让 LLM 压缩，而非机械截断）

**影响**：长时间对话中，早期重要信息（如故障现象描述）被截断丢失。

**建议**：
- 方案 A（简单）：改为截取开头 + 结尾，中间用 `[...省略 N 轮对话...]` 连接
- 方案 B（智能）：调用 LLM 对超长摘要做压缩（`请将以下对话摘要压缩到 200 字以内`）

---

#### 问题 5：没有记忆压缩/合并机制

**位置**：整个 `long_term.go`

**现状**：
- 内容完全相同的记忆 → 同一 ID → reinforce（AccessCnt++）
- 措辞略有不同 → 不同 ID → 两条独立记忆
- 例："payment 端口是 8080" 和 "payment-service port:8080" 会创建两条记忆

**问题**：
- 随着对话增多，相似记忆不断累积
- 占用宝贵的容量配额（全局 1000 条）
- 检索时返回多条相似结果，降低信息密度

**建议**：
- 方案 A：LLM 提取时做去重（已有 `supersede` 机制，但需要 TargetID 精确匹配）
- 方案 B：定期运行记忆合并任务（相似度 > 0.8 的记忆合并为一条）
- 方案 C：检索后做去重（Top-K 结果中相似度 > 0.9 的只保留最高分）

---

#### 问题 6：EventID 包含时间戳，去重效果打折

**位置**：`memory_queue.go:467`

**现状**：
```go
func buildMemoryExtractionEventID(e memoryExtractionEvent) string {
    raw := fmt.Sprintf("%s:%s:%s:%d", e.SessionID, e.Query, e.Summary, e.RequestedAt.UnixMilli())
    return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))[:16]
}
```

**问题**：
- `RequestedAt` 是毫秒精度 → 同一查询在不同时间产生不同 EventID
- 如果用户重复发送相同消息，不会被去重

**建议**：
- 去掉 `RequestedAt`，改为 `SessionID:Query:Summary` 的哈希
- 或者用 `SessionID:Query:Summary:Attempt`（重试时 Attempt 递增，首次提交可去重）

---

#### 问题 7：RetrieveScoped 的访问计数更新时序

**位置**：`long_term.go:286-298`

**现状**：
```go
ltm.mu.RLock()
// ... 克隆 entries，用于排序和选择 ...
ltm.mu.RUnlock()

// 窗口期：其他 goroutine 可能修改 entries

ltm.mu.Lock()
entry.AccessCnt++   // 仅更新当前 map entry 的计数
entry.LastUsed = now
ltm.mu.Unlock()
```

**问题**：
- 候选排序基于快照，访问计数更新可能略滞后
- 写回阶段只更新 AccessCnt/LastUsed，不会用旧快照覆盖整个 entry
- 实际影响很小（排序可能基于略微过时的访问计数）

**建议**：
- 改为 `Lock`（写锁）贯穿整个操作，或用 `atomic` 操作更新 AccessCnt/LastUsed

---

### 🟢 低等问题（代码质量）

#### 问题 8：6 处死代码

| 死代码 | 位置 | 说明 |
|--------|------|------|
| `MemoryEntry.Decay` 字段 | `long_term.go:56` | 创建时写入 1.0，clone 保留，但未参与 `computeRelevance()` 计算（动态计算） |
| `MemoryEntry.UpdatePolicy` 字段 | `long_term.go:53` | 设为 "reinforce" 但无逻辑读取 |
| `MemoryEntry.AllowTriage` 字段 | `long_term.go:50` | 从未设为 true |
| `TopicSegment` + `AddTopic`/`GetTopics` | `session.go:166` | 未使用的主题分段功能 |
| `truncate()` 函数 | `extraction.go:233` | 定义并测试但从未调用 |
| `BuildEnrichedContext()` | `extraction.go:116` | 旧版注入路径，已被 Assembler 替代 |

**建议**：全部删除。如果未来需要 TopicSegment，可以从 git 恢复。

---

#### 问题 9：Bubble Sort

**位置**：`long_term.go:264-270`

**现状**：用 O(n²) 选择排序替代 `sort.Slice`。

**影响**：`n ≤ 15`（`retrieveLimit*3`），性能影响可忽略。

**建议**：改为 `sort.Slice`，提升代码可读性。

---

#### ~~问题 10：Superseded 记忆不主动清理~~ ✅ 已实现

**位置**：`long_term.go:631-634`

**现状**：冲突检测时已设置 `ExpiresAt = now.UnixMilli()`，superseded 记忆会立即过期：
```go
entry.SafetyLabel = "superseded"
entry.UpdatePolicy = "superseded"
entry.ExpiresAt = now.UnixMilli()  // 立即过期
entry.Confidence = entry.Confidence * 0.5
```

**建议**：补充测试断言，验证 superseded 记忆的 `ExpiresAt` 确实被设置。

---

## 三、优化优先级

```
优先级    问题                    工作量    效果
──────────────────────────────────────────────────
P0       Token 预算不足           小        高（直接提升记忆可用性）
P0       记忆检索无语义           中        高（提升召回率）
P1       文件全量写入（条件性）    中        中（仅启用持久化时生效）
P1       摘要截断粗暴             小        中（保留重要上下文）
P1       记忆压缩/合并            大        中（提升信息密度）
P2       EventID 时间戳           小        低（去重优化）
P2       访问计数更新时序         小        低（排序略滞后）
P2       死代码清理               小        低（代码质量）
P2       Bubble Sort             小        低（可读性）
✅       Superseded 清理          —         已实现（ExpiresAt = now）
```

---

## 四、与 RAG 系统的关系

### 记忆 vs RAG：定位不同

| 维度 | 记忆系统 | RAG 知识库 |
|------|---------|-----------|
| 写入方式 | Agent 自动提取 | 人工批量索引 |
| 内容 | 对话中的事实/偏好 | 预构建的运维文档 |
| 时效性 | 实时更新，有衰减 | 相对静态 |
| 检索方式 | 关键词匹配 | Dense + BM25 + RRF |
| 作用域 | session/user/project/global | 全局共享 |
| 注入位置 | `[关键记忆]` 消息 | `[参考文档]` 消息 |

### 矛盾点

RAG 系统用了最先进的混合检索（向量 + BM25 + RRF + 重排），而记忆系统只有关键词匹配。两者都是"从知识库中检索相关信息注入上下文"，但检索质量差距很大。

**理想状态**：记忆系统复用 RAG 的检索能力（至少 BM25），或者为记忆建立轻量级向量索引。

---

## 五、面试话术

### Q: 你的记忆系统怎么设计的？

> 我设计了三层记忆体系：
>
> **短期记忆**是会话级的滑动窗口，默认 20 条消息，溢出时自动摘要。
> 用内存 map 存储，2 小时 TTL，最多 500 个并发会话。
>
> **提取层**是异步的，对话结束后通过 RabbitMQ 发消息到提取队列。
> 两种 Agent：规则提取（关键词匹配，快速确定）和 LLM 提取
> （大模型判断，支持 supersedes/promote 操作）。RabbitMQ 不可用时
> 降级到本地 goroutine + 信号量。
>
> **长期记忆**默认纯内存存储，可选文件持久化。每条记忆有内容寻址 ID（SHA256 哈希），
> 4 级作用域（session/user/project/global），衰减淘汰策略
> （relevance = decay × frequency，24 小时半衰期）。
>
> 上下文注入时，按综合评分（置信度 50% + 新鲜度 30% + 作用域优先级 20%）
> 排序，取 Top-5 注入为 `[关键记忆]` 消息对。

### Q: 记忆系统有什么问题？怎么优化？

> 目前有三个主要问题：
>
> **第一，Token 预算太紧**。Memory 只分配了 10%（800 tokens），
> 最多塞 1-2 条记忆。计划提升到 20%，并对长记忆做截断摘要。
>
> **第二，检索没有语义能力**。目前是纯关键词匹配，"Redis 连接超时"
> 搜不到 "Redis timeout"。计划复用 BM25 做分词检索，或者加轻量向量索引。
>
> **第三，启用文件持久化后每次全量写入**。默认是纯内存，配置了
> `memory.long_term_store_path` 才启用文件持久化。启用后 1000 条记忆
> 每次写 100KB+。计划改为增量 JSONL + 定期 compaction，或者迁移到 SQLite。

### Q: 记忆和 RAG 怎么配合？

> 两者定位不同但互补。RAG 是预构建的运维知识库，所有人查到的结果一样；
> 记忆是从对话中实时提取的用户/项目特定事实，有时效衰减。
>
> 上下文注入时，记忆在前（`[关键记忆]`），RAG 文档在后（`[参考文档]`）。
> 记忆提供个性化背景，RAG 提供通用知识，两者不重叠。
>
> 目前的短板是记忆检索能力远弱于 RAG。RAG 用了 Dense + BM25 + RRF
> 混合检索，记忆只有关键词匹配。这是下一步优化重点。

### Q: 记忆怎么保证不产生错误信息？

> 四层保障：
>
> 1. **提取时**：LLM Agent 有 confidence 评分，低于 0.50 的不注入
> 2. **冲突检测**：同一 conflict group 的新记忆会 supersede 旧记忆
> 3. **作用域隔离**：session 级记忆不影响其他会话
> 4. **衰减淘汰**：24 小时半衰期，长期不用的记忆自动降低优先级
>
> 但目前没有"事实验证"机制——如果 LLM 提取了错误事实
> （如错误的端口号），会一直保留直到被新记忆 supersede。
> 这是一个已知的改进方向。
