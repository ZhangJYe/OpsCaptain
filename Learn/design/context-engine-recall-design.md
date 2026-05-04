# ContextEngine Recall Layer 设计方案

## 1. 背景

当前 ContextEngine 的组装流程是：

```
请求进来 → 直接组装（history全拿 + memory召回 + doc召回 + tools全拿）
```

其中：
- **History**：从请求中直接取最近 N 条，没有基于 query 相关性筛选
- **Memory**：有 `RetrieveScoped()` 做向量召回 ✅
- **Documents**：有 `rag.Query()` 做 RAG 召回 ✅
- **ToolItems**：从请求中直接拿，没有相关性筛选

**问题**：History 和 ToolItems 没有召回能力，只是"截取"而非"检索"。当对话历史很长或工具结果很多时，可能把不相关的内容塞进上下文，浪费 token 预算。

## 2. 问题定义

### 2.1 当前的"伪召回"

```go
// 当前 History 的处理方式：从后往前取 N 条
for i := len(history) - 1; i >= 0; i-- {
    if selectedCount >= maxMessages {
        dropped = append(dropped, ...)
        continue
    }
    // 没有任何相关性判断，纯粹按位置截取
    selectedIdx[i] = true
}
```

这种方式的问题：
1. **位置偏见**：总是取最近的，但用户可能在追问 10 轮前的一个细节
2. **无相关性**：用户问"Redis 连接超时"，但最近 5 轮聊的是"CPU 告警"，全塞进去浪费 token
3. **无语义理解**：不知道哪些历史消息和当前 query 语义相关

### 2.2 ToolItems 的问题

```go
// 当前 ToolItems 的处理方式：按顺序取前 N 个
for idx, item := range items {
    if idx >= profile.MaxToolItems {
        dropped = append(dropped, ...)
        continue
    }
    // 没有相关性判断，纯粹按顺序截取
    selected = append(selected, item)
}
```

## 3. 目标

本次 Recall Layer 的目标是：

1. **为 History 引入相关性召回**：基于 query 语义相似度筛选历史消息
2. **为 ToolItems 引入相关性召回**：基于 query 和工具结果的匹配度筛选
3. **保持向后兼容**：不破坏现有 Profile/Budget 机制
4. **可观测**：召回过程全程 Trace，便于调试

**非目标**：
- 不做 Intent Recognition（现有 Profile 机制已覆盖）
- 不做跨来源去重（优先级低，后续再做）

## 4. 核心思路

### 4.1 完整的上下文工程三阶段

```
请求进来
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 1: Multi-Source Recall（多路召回）                   │
│  - History Recall:  基于 embedding 相似度筛选历史消息       │
│  - Memory Recall:  已有 RetrieveScoped()，保持不变         │
│  - Document Recall: 已有 rag.Query()，保持不变             │
│  - Tool Recall:    基于 query 关键词匹配筛选工具结果        │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  阶段 2: Budget Assembly（预算组装）                       │
│  - 现有的 selectHistory / selectMemories / selectDocuments │
│  - 按 Profile 预算截断、排序、打包                          │
└─────────────────────────────────────────────────────────┘
    │
    ▼
ContextPackage → LLM
```

**为什么不做 Intent Recognition？**
1. 现有 Profile 机制已按 mode（chat/ai_ops）区分策略，足够用
2. 规则引擎的意图识别在运维场景下不可靠（"超时配置怎么改"会被误判为 fault_diagnosis）
3. 增加复杂度但没有明确收益——召回质量才是关键

### 4.2 History Recall 设计

#### 方案对比

| 方案 | 原理 | 优点 | 缺点 |
|------|------|------|------|
| **A. Embedding 相似度** | 把每条 history 做 embedding，和 query 计算余弦相似度 | 语义理解强 | 需要额外 embedding 调用，有延迟 |
| **B. BM25 关键词** | 对 history 做分词，用 BM25 打分 | 快，无需外部调用 | 语义理解弱，中文分词依赖 |
| **C. 混合** | A + B 加权融合 | 兼顾语义和关键词 | 复杂度高 |
| **D. LLM 摘要** | 让 LLM 判断哪些历史消息相关 | 理解最强 | 慢、贵、不稳定 |

**推荐方案：A. Embedding 相似度**

原因：
1. 当前已有 Doubao Embedding，复用成本低
2. 语义理解对运维场景很重要（"服务挂了"和"pod failure"语义相似）

#### 延迟预算

| 场景 | 条数 | 延迟估算 | 说明 |
|------|------|----------|------|
| 缓存命中 | - | ~0ms | 无额外延迟 |
| Batch Embedding（10 条） | 10 | 100-200ms | 单次 batch 调用 |
| Batch Embedding（30 条） | 30 | 200-400ms | 可能需要 2 次 batch |
| Batch Embedding（50 条） | 50 | 300-600ms | 可能需要 3 次 batch |
| 降级（embedding 失败） | - | ~0ms | 回退到位置截取 |

**关键优化**：
1. **Batch 调用**：Doubao Embedding 支持批量接口，一次传入多条文本，减少网络往返
2. **缓存**：历史消息的 embedding 按 content hash 缓存，下轮对话直接命中
3. **超时降级**：embedding 调用超过 500ms 就降级为位置截取

#### 缓存命中率分析

```
第 1 轮：history = [m1, m2, m3]
         缓存：无 → 需要 embed 3 条

第 2 轮：history = [m1, m2, m3, m4, m5]
         缓存命中：m1, m2, m3 → 只需 embed m4, m5（2 条）

第 3 轮：history = [m1, m2, m3, m4, m5, m6, m7]
         缓存命中：m1-m5 → 只需 embed m6, m7（2 条）

第 N 轮：每轮只新增 2 条消息（user + assistant）
         缓存命中率 = (N-1)*2 / N*2 ≈ 50-90%（越往后越高）
```

**结论**：缓存能显著减少 embedding 调用量。第 5 轮以后，缓存命中率 > 80%。

#### 对话结构处理

User 和 Assistant 消息是成对的，需要特殊处理：

```go
// pairAwareSelect：成对保留 user+assistant 消息
// 注意：position 是原始 history 数组的索引，不是 candidates 的索引
func pairAwareSelect(scored []scoredMessage, history []*schema.Message, topK int) []*schema.Message {
    selected := make([]*schema.Message, 0, topK*2)
    used := make(map[int]bool)
    
    for _, s := range scored {
        if len(selected) >= topK*2 {
            break
        }
        if used[s.position] {
            continue
        }
        
        // 边界检查：position 必须在 history 范围内
        if s.position < 0 || s.position >= len(history) {
            continue
        }
        
        selected = append(selected, s.message)
        used[s.position] = true
        
        // 选中 user 消息时，强制保留下一条 assistant 消息（如果存在）
        if s.message.Role == schema.User && s.position+1 < len(history) {
            next := history[s.position+1]
            if next.Role == schema.Assistant && !used[s.position+1] {
                selected = append(selected, next)
                used[s.position+1] = true
            }
        }
        
        // 选中 assistant 消息时，强制保留前一条 user 消息（如果存在）
        if s.message.Role == schema.Assistant && s.position-1 >= 0 {
            prev := history[s.position-1]
            if prev.Role == schema.User && !used[s.position-1] {
                selected = append(selected, prev)
                used[s.position-1] = true
            }
        }
    }
    
    // 按原始位置排序，保持时序
    sort.Slice(selected, func(i, j int) bool {
        return findPosition(selected[i], history) < findPosition(selected[j], history)
    })
    
    return selected
}
```

#### 最近 N 条强制保留

**即使使用 embedding 召回，最近 2-3 条消息应该强制保留**（用户刚说的话不能丢）。

```go
const minRecentMessages = 3  // 最近 3 条强制保留

func (r *HistoryRecaller) Recall(ctx context.Context, query string, history []*schema.Message, topK int) []*schema.Message {
    if len(history) <= topK {
        return history
    }
    
    // 1. 强制保留最近 N 条
    recentCount := min(minRecentMessages, len(history))
    recent := history[len(history)-recentCount:]
    candidates := history[:len(history)-recentCount]
    
    // 2. 对候选消息做 embedding 召回
    recalled := r.recallByEmbedding(ctx, query, candidates, topK-recentCount)
    
    // 3. 合并：召回结果 + 最近消息，按原始位置排序
    return mergeAndSort(recalled, recent)
}
```

#### 相似度阈值

```go
const similarityThreshold = 0.3  // 初始值，需要在实际数据上验证
// Doubao Embedding 的余弦相似度分布可能和其他模型不同
// benchmark 阶段会调整此阈值：如果召回太多不相关消息就提高，如果漏掉相关消息就降低

// 取 top-K，但如果分数低于阈值就不取
func topKWithThreshold(scored []scoredMessage, topK int, threshold float64) []scoredMessage {
    selected := make([]scoredMessage, 0, topK)
    for _, s := range scored {
        if len(selected) >= topK {
            break
        }
        if s.score < threshold {
            break  // 后面的分数更低，不用继续
        }
        selected = append(selected, s)
    }
    return selected
}
```

#### 实现思路

```go
// contextengine/history_recall.go

type HistoryRecaller struct {
    embedder    EmbeddingClient
    cache       *EmbeddingCache
    timeout     time.Duration  // 默认 500ms
    recentKeep  int            // 默认 3
    threshold   float64        // 默认 0.3
}

type HistoryRecallResult struct {
    Messages    []*schema.Message
    Scores      []float64
    CacheHits   int
    Embedded    int
    LatencyMs   int64
    Degraded    bool  // 是否降级
}

func (r *HistoryRecaller) Recall(ctx context.Context, query string, history []*schema.Message, topK int) HistoryRecallResult {
    start := time.Now()
    
    // 快速路径：历史消息不多，直接返回
    if len(history) <= topK {
        return HistoryRecallResult{Messages: history}
    }
    
    // 1. 分离最近消息和候选消息
    recentCount := min(r.recentKeep, len(history))
    recent := history[len(history)-recentCount:]
    candidates := history[:len(history)-recentCount]
    
    // 2. 带超时的 embedding 召回
    recallCtx, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()
    
    recalled, err := r.recallByEmbedding(recallCtx, query, candidates, topK-recentCount)
    if err != nil {
        // 降级：回退到位置截取
        return HistoryRecallResult{
            Messages:  fallbackPositional(history, topK),
            LatencyMs: time.Since(start).Milliseconds(),
            Degraded:  true,
        }
    }
    
    // 3. 合并并排序
    merged := mergeAndSort(recalled, recent)
    
    return HistoryRecallResult{
        Messages:  merged,
        LatencyMs: time.Since(start).Milliseconds(),
        CacheHits: recalled.CacheHits,
        Embedded:  recalled.Embedded,
    }
}

func (r *HistoryRecaller) recallByEmbedding(ctx context.Context, query string, candidates []*schema.Message, topK int) (*recallResult, error) {
    // 1. 计算 query embedding
    queryVec, err := r.embedder.Embed(ctx, query)
    if err != nil {
        return nil, err
    }
    
    // 2. 获取 candidates embeddings（带缓存 + batch）
    vecs, cacheHits, err := r.getEmbeddings(ctx, candidates)
    if err != nil {
        return nil, err
    }
    
    // 3. 计算相似度
    scored := make([]scoredMessage, len(candidates))
    for i, msg := range candidates {
        scored[i] = scoredMessage{
            message:  msg,
            score:    cosineSimilarity(queryVec, vecs[i]),
            position: i,
        }
    }
    
    // 4. 按分数排序，取 top-K（带阈值）
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].score > scored[j].score
    })
    selected := topKWithThreshold(scored, topK, r.threshold)
    
    // 5. 按原始位置排序，保持时序
    sort.Slice(selected, func(i, j int) bool {
        return selected[i].position < selected[j].position
    })
    
    return &recallResult{
        messages:  extractMessages(selected),
        scores:    extractScores(selected),
        cacheHits: cacheHits,
        embedded:  len(candidates) - cacheHits,
    }, nil
}

func (r *HistoryRecaller) getEmbeddings(ctx context.Context, messages []*schema.Message) ([][]float64, int, error) {
    vecs := make([][]float64, len(messages))
    toEmbed := make([]int, 0)      // 需要 embed 的索引
    toEmbedTexts := make([]string, 0)
    cacheHits := 0
    
    // 1. 查缓存
    for i, msg := range messages {
        hash := contentHash(msg.Content)
        if cached, ok := r.cache.Get(hash); ok {
            vecs[i] = cached
            cacheHits++
        } else {
            toEmbed = append(toEmbed, i)
            toEmbedTexts = append(toEmbedTexts, msg.Content)
        }
    }
    
    // 2. Batch embed 未缓存的
    if len(toEmbedTexts) > 0 {
        embedded, err := r.embedder.BatchEmbed(ctx, toEmbedTexts)
        if err != nil {
            return nil, 0, err
        }
        for j, idx := range toEmbed {
            vecs[idx] = embedded[j]
            r.cache.Set(contentHash(messages[idx].Content), embedded[j])
        }
    }
    
    return vecs, cacheHits, nil
}
```

#### 降级策略

```
embedding 调用成功？
    │
    ├── Yes → 使用 embedding 召回结果
    │
    └── No（超时/报错）
            │
            ▼
        降级为位置截取（现有逻辑）
```

### 4.3 ToolItems Recall 设计

#### 方案

ToolItems 的特点是：内容通常包含结构化数据（日志、指标、告警），关键词匹配比语义相似度更有效。

```go
// contextengine/tool_recall.go

type ToolRecaller struct {
    // 无状态，不需要 embedding
}

type ToolRecallResult struct {
    Items     []ContextItem
    LatencyMs int64
}

func (r *ToolRecaller) Recall(ctx context.Context, query string, items []ContextItem, topK int) ToolRecallResult {
    start := time.Now()
    
    if len(items) <= topK {
        return ToolRecallResult{Items: items, LatencyMs: time.Since(start).Milliseconds()}
    }
    
    // 1. 从 query 中提取关键词
    keywords := extractKeywords(query)
    
    // 2. 对每个 item 计算匹配分数（优先用元数据，其次用内容）
    scored := make([]scoredItem, len(items))
    for i, item := range items {
        scored[i] = scoredItem{
            item:  item,
            score: matchScore(keywords, item),
        }
    }
    
    // 3. 按分数排序，取 top-K
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].score > scored[j].score
    })
    
    selected := make([]ContextItem, 0, topK)
    for i := 0; i < min(topK, len(scored)); i++ {
        selected = append(selected, scored[i].item)
    }
    
    return ToolRecallResult{Items: selected, LatencyMs: time.Since(start).Milliseconds()}
}

func matchScore(keywords []string, item ContextItem) float64 {
    score := 0.0
    
    // 优先匹配元数据字段（Title, SourceType, SourceID）
    metaText := item.Title + " " + item.SourceType + " " + item.SourceID
    for _, kw := range keywords {
        if strings.Contains(strings.ToLower(metaText), strings.ToLower(kw)) {
            score += 2.0  // 元数据匹配权重更高
        }
    }
    
    // 其次匹配内容
    content := strings.ToLower(item.Content)
    for _, kw := range keywords {
        if strings.Contains(content, strings.ToLower(kw)) {
            score += 1.0
        }
    }
    
    return score
}
```

#### 关键词提取策略

```go
func extractKeywords(query string) []string {
    keywords := make([]string, 0)
    
    // 1. 服务名：checkoutservice, paymentservice, ...
    //    正则：[a-z]+service（英文）
    servicePattern := regexp.MustCompile(`[a-z]+service`)
    keywords = append(keywords, servicePattern.FindAllString(strings.ToLower(query), -1)...)
    
    // 2. 错误码：503, 502, 404, connection refused, timeout, ...
    errorPattern := regexp.MustCompile(`\b[45]\d{2}\b|connection refused|timeout|error|failure|down`)
    keywords = append(keywords, errorPattern.FindAllString(strings.ToLower(query), -1)...)
    
    // 3. 指标名：cpu, memory, latency, qps, ...
    metricPattern := regexp.MustCompile(`\b(cpu|memory|latency|qps|tps|error.?rate)\b`)
    keywords = append(keywords, metricPattern.FindAllString(strings.ToLower(query), -1)...)
    
    // 4. 实体词：Redis, MySQL, Kafka, Pod, ...
    entityPattern := regexp.MustCompile(`\b(redis|mysql|kafka|pod|node|cluster|namespace)\b`)
    keywords = append(keywords, entityPattern.FindAllString(strings.ToLower(query), -1)...)
    
    // 5. 中文运维术语 → 映射为英文关键词（同时保留中文）
    cnKeywords := map[string]string{
        "超时": "timeout", "连接": "connection", "拒绝": "refused",
        "延迟": "latency", "挂了": "down", "异常": "error",
        "告警": "alert", "故障": "failure", "宕机": "down",
        "慢查询": "slow query", "瓶颈": "bottleneck", "抖动": "jitter",
        "飙升": "spike", "打满": "full", "耗尽": "exhausted",
    }
    for cn, en := range cnKeywords {
        if strings.Contains(query, cn) {
            keywords = append(keywords, en, cn)
        }
    }
    
    // 6. 去重
    return unique(keywords)
}
```

**注意**：关键词提取不需要 100% 准确，漏掉一些关键词不影响大局——这只是粗筛，后面还有 Budget Assembly 做精筛。

### 4.4 Assistant 消息的 Embedding 质量

Assistant 回复通常是长篇大论（诊断报告、工具结果摘要），embedding 可能会被稀释。

**缓解方案**：
1. **截断后再 embed**：只取前 200 字符做 embedding，避免长文本稀释
2. **摘要后再 embed**（可选，后续优化）：用 LLM 对长 assistant 消息做摘要，再 embed 摘要

```go
func (r *HistoryRecaller) getMessageText(msg *schema.Message) string {
    content := msg.Content
    
    // assistant 消息截断到 200 字符
    if msg.Role == schema.Assistant && len(content) > 200 {
        content = content[:200] + "..."
    }
    
    return content
}
```

## 5. 接口设计

### 5.1 HistoryRecaller 接口

```go
// contextengine/history_recall.go

type HistoryRecallResult struct {
    Messages    []*schema.Message
    Scores      []float64         // 每条消息的相似度分数
    CacheHits   int               // 缓存命中数
    Embedded    int               // 实际 embed 数量
    LatencyMs   int64
    Degraded    bool              // 是否降级
}

type HistoryRecaller struct {
    embedder    EmbeddingClient
    cache       *EmbeddingCache
    timeout     time.Duration
    recentKeep  int
    threshold   float64
}

func NewHistoryRecaller(embedder EmbeddingClient, cache *EmbeddingCache) *HistoryRecaller
func (r *HistoryRecaller) Recall(ctx context.Context, query string, history []*schema.Message, topK int) HistoryRecallResult
```

### 5.2 ToolRecaller 接口

```go
// contextengine/tool_recall.go

type ToolRecallResult struct {
    Items     []ContextItem
    LatencyMs int64
}

type ToolRecaller struct{}

func NewToolRecaller() *ToolRecaller
func (r *ToolRecaller) Recall(ctx context.Context, query string, items []ContextItem, topK int) ToolRecallResult
```

### 5.3 修改 Assembler

```go
// contextengine/assembler.go

type Assembler struct {
    resolver    *PolicyResolver
    now         func() time.Time
    historyRec  *HistoryRecaller   // 新增
    toolRec     *ToolRecaller      // 新增
}

func NewAssembler() *Assembler {
    return &Assembler{
        resolver:   NewPolicyResolver(),
        now:        time.Now,
        historyRec: NewHistoryRecaller(sharedEmbedder(), sharedCache()),
        toolRec:    NewToolRecaller(),
    }
}

func (a *Assembler) Assemble(ctx context.Context, req ContextRequest, history []*schema.Message) (*ContextPackage, error) {
    profile := a.resolver.Resolve(ctx, req)
    pkg := &ContextPackage{...}
    
    // History：使用召回
    if profile.AllowHistory && len(history) > 0 {
        recallResult := a.historyRec.Recall(ctx, req.Query, history, profile.MaxHistoryMessages)
        // 注意：Recall() 已按 topK 截断消息数，但消息总 token 可能超过 HistoryTokens 预算
        // selectHistory 做的是 token 级截断（按 budget 砍掉超长消息），不是冗余操作
        selectedHistory, dropped, used, notes := selectHistory(recallResult.Messages, profile)
        pkg.HistoryMessages = selectedHistory
        // 记录 trace
        trace.HistoryRecall = recallResult
    }
    
    // Memory：保持不变
    if profile.AllowMemory && req.SessionID != "" {
        // 现有逻辑
    }
    
    // Documents：保持不变
    if profile.AllowDocs {
        // 现有逻辑
    }
    
    // ToolItems：使用召回
    if profile.AllowToolResults && len(req.ToolItems) > 0 {
        recallResult := a.toolRec.Recall(ctx, req.Query, req.ToolItems, profile.MaxToolItems)
        // 对召回结果做 budget 截断（现有逻辑）
        selectedTools, dropped, used, notes := selectToolItems(recallResult.Items, profile)
        pkg.ToolItems = selectedTools
        // 记录 trace
        trace.ToolRecall = recallResult
    }
}
```

## 6. 实现计划

### Phase 1: History Recall（3-4 天）

| 天数 | 任务 | 说明 |
|------|------|------|
| D1 | 实现 HistoryRecaller 骨架 | 接口定义、结构体、主流程 |
| D1 | 集成 Doubao Embedding | 复用现有 embedder |
| D2 | 实现 Batch Embedding | 支持批量调用，减少网络往返 |
| D2 | 实现 EmbeddingCache | LRU 缓存，content hash 做 key |
| D3 | 实现成对保留逻辑 | user+assistant 配对 |
| D3 | 实现最近 N 条强制保留 | recentKeep = 3 |
| D3 | 实现降级策略 | 超时/报错回退位置截取 |
| D4 | 集成到 Assembler | 修改 Assemble 函数 |
| D4 | 补充单测 + 集成测试 | 覆盖各种边界情况 |

### Phase 2: ToolItems Recall（2 天）

| 天数 | 任务 | 说明 |
|------|------|------|
| D1 | 实现 ToolRecaller | 接口、主流程 |
| D1 | 实现关键词提取 | 正则匹配服务名/错误码/指标名/实体词 |
| D2 | 实现匹配打分 | 元数据权重 > 内容权重 |
| D2 | 集成到 Assembler + 单测 | |

### Phase 3: Benchmark（1 天）

| 任务 | 说明 |
|------|------|
| 对比测试 | 在现有对话数据上对比"有召回"和"无召回"的 token 利用率 |
| 延迟测试 | 测量 P50/P99 延迟变化 |
| 效果测试 | 在 RAG 评测集上对比 answer quality |

### 7.1 召回质量对比

```
指标：Recall@5, MRR@10（Mean Reciprocal Rank）
方法：
1. 收集 100 条真实对话
2. 人工标注每条历史消息是否和当前 query 相关（二元标注：相关/不相关）
3. 分别用"位置截取"和"embedding 召回"取 top-5
4. 计算：
   - Recall@5 = 召回的相关消息数 / 总相关消息数
   - MRR@10 = 1/|Q| * Σ 1/rank_i（rank_i 是第 i 个相关结果在返回列表中的排名）
期望：
```
### 7.2 Token 利用率对比
```
指标：上下文中有用信息的 token 占比
方法：
1. 收集 100 条真实对话
2. 分别用"位置截取"和"embedding 召回"组装上下文
3. 人工标注每条消息是否和当前 query 相关
4. 计算：有用 token / 总 token

期望：
- 位置截取：~40-60% 的 token 是不相关的
- embedding 召回：~70-85% 的 token 是相关的
```

### 7.2 延迟对比

```
指标：ContextEngine 组装的 P50/P99 延迟
方法：
1. 在 staging 环境跑 1000 次请求
2. 记录每次 Assemble 的耗时
3. 计算 P50/P99

期望：
- 当前（位置截取）：P50 ~10ms, P99 ~50ms
- 新增（embedding 召回）：P50 ~50ms, P99 ~300ms
- 降级时：P50 ~10ms, P99 ~50ms（和当前一致）
```

### 7.3 端到端效果

```
指标：RAG 评测集上的 answer quality
方法：
1. 用现有的 RAG 评测集（fault/symptom/combined）
2. 分别用"位置截取"和"embedding 召回"跑评测
3. 对比 BLEU/ROUGE/人工评分

期望：answer quality 有 5-10% 的提升
```

## 8. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Embedding 调用延迟 | 增加上下文组装耗时 | Batch 调用 + 缓存 + 500ms 超时降级 |
| 召回结果不稳定 | 同样的问题召回不同的上下文 | 固定 topK + 相似度阈值 0.3 |
| 缓存内存占用 | 大量 history embedding 缓存 | LRU 淘汰 + 10 分钟过期 |
| assistant 消息 embedding 质量差 | 长文本稀释 | 截断到 200 字符后再 embed |
| 对话结构断裂 | 选中 user 但丢弃 assistant | 成对保留逻辑 |

## 9. 与现有机制的关系

```
                    现有机制                           新增机制
                    ────────                           ────────
History:     从后往前取 N 条                    →    Embedding 相似度召回
Memory:      RetrieveScoped() 向量召回          →    保持不变
Documents:   rag.Query() RAG 召回               →    保持不变
ToolItems:   按顺序取前 N 个                    →    关键词匹配召回
Profile:     按 mode 选择 profile              →    保持不变（不做 Intent Recognition）
Budget:      按 token 预算截断                  →    保持不变
```

## 10. 面试话术

> "我的 ContextEngine 分两个阶段：多路召回 → 预算组装。
>
> 多路召回是核心：History 用 embedding 相似度筛选（复用 Doubao Embedding，带缓存），Memory 和 Documents 保持现有的向量召回，ToolItems 用关键词匹配。
>
> History Recall 的关键设计：1) 最近 3 条强制保留，用户刚说的话不能丢；2) 成对保留，user 和 assistant 消息不能拆开；3) 500ms 超时降级，embedding 挂了就回退到位置截取。
>
> 延迟方面：缓存命中时零额外延迟，首次调用约 100-400ms（取决于 history 条数），通过 batch 调用和缓存控制风险。
>
> 最后按 Profile 预算组装，全程 Trace 可追溯。"
