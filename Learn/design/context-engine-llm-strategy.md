# ContextEngine Recall Layer — 大模型使用策略

## 1. 核心原则

**先用轻量方案，效果不够再上大模型。**

大模型（LLM）是"最后手段"，不是"第一选择"。原因：
- 延迟：LLM 调用 200-2000ms，Embedding 100-400ms，关键词匹配 <10ms
- 成本：LLM 按 token 计费，Embedding 便宜 10-100 倍
- 稳定性：LLM 输出有随机性，Embedding 和关键词是确定性的

## 2. 当前实现 vs 可选方案

| 组件 | 当前方案 | 可选 LLM 方案 | 是否引入 LLM | 理由 |
|------|---------|--------------|-------------|------|
| **History Recall** | Embedding 相似度 | LLM 判断相关性 | ❌ 不引入 | 延迟太高（50条消息+LLM=1-3s），Embedding 已经够用 |
| **Memory Recall** | MemoryService 产出候选，ContextEngine 按预算选取 | LLM 判断长期记忆相关性 | ❌ 暂不引入 | 记忆先靠抽取质量、置信度、会话范围和过期策略治理，LLM 精排收益不确定 |
| **Documents Recall** | `rag.Query` 混合检索（ContextEngine 当前未启用 QueryForEval 的 rewrite/rerank） | Query Rewrite / LLM Rerank | ⚠️ 按需引入 | 先用 benchmark 判断 docs 是否是瓶颈；如需要，通过配置化开关接入 |
| **ToolItems Recall** | 关键词 + metadata 匹配 | LLM Rerank | ⚠️ 按需引入 | 只在统一 benchmark 证明 ToolItems 是瓶颈时启用 |
| **Intent Recognition** | 不做（Profile 机制覆盖） | LLM 意图识别 | ⚠️ 按需引入 | Profile 已按 mode 区分，如果需要更细粒度再加 |
| **Query Rewrite** | ContextEngine 层不做；RAG eval/full path 有能力但当前 docs 选择未默认启用 | LLM 改写 query | ⚠️ 按需引入 | 不把 eval 能力误写成线上默认能力，后续按配置接入 |

## 3. 渐进式引入策略

```
P0（先做）：统一 Recall Benchmark
├── 标注 query 需要哪些 history / memory / docs / tool outputs
├── 统计 Recall@K / MRR / NDCG / P95 latency / degraded_count
└── 判断真正拖后腿的召回源
    │
    ▼
Phase 1（当前）：纯轻量方案
├── History Recall: Embedding 相似度
├── Memory Recall: 置信度 / scope / budget 过滤
├── Documents Recall: rag.Query 混合检索
├── ToolItems Recall: 关键词 + metadata 匹配
└── Intent: 不做，用 Profile 机制
    │
    ▼ 跑 Benchmark，评估效果
    │
Phase 2（先优化确定性召回）
├── History: 加 recency / role / entity 权重
├── Memory: 加 scope / confidence / recency 权重
├── Documents: 明确是否配置化启用 rewrite/rerank
└── ToolItems: 加服务名 / 错误类型 / metadata 权重
    │
    ▼ 再跑 Benchmark，评估效果
    │
Phase 2.5（仅当某个来源仍是瓶颈）：局部 LLM Rerank
├── ToolItems: 关键词粗筛 → 脱敏裁剪 → LLM 精排
└── Documents: 如 docs 召回瓶颈，再接 RAG rewrite/rerank 配置
    │
    ▼
Phase 3（如果需要更细粒度策略）：加 LLM 意图识别
└── LLM 判断意图 → 选择对应 Profile
```

关键调整：LLM Rerank 不是默认下一阶段，而是 benchmark 证明某个召回源成为瓶颈后的局部增强。这样能避免为了"更智能"而增加热链路延迟。

## 4. Phase 2.5 详细设计：ToolItems LLM Rerank

### 4.1 触发条件

当统一 Benchmark 证明 ToolItems 是主要瓶颈时才触发：
- ToolItems Recall@5 < 0.6，且 MRR / NDCG 明显低于 history、memory、docs
- 低分不是由工具结果缺失、metadata 缺失或上游没有传入 ToolItems 导致
- 用户反馈"召回的工具结果不相关"，并能回放到具体 query / expected item
- 只在 `ai_ops`、故障诊断等高价值场景启用，普通闲聊和纯知识问答不启用

### 4.2 方案

```
用户 query
    │
    ▼
┌─────────────────────────────────────┐
│ Step 0: 配置和场景开关                │
│ - context.tool_rerank.enabled        │
│ - profile/mode 白名单                │
│ - candidate_limit / timeout / ttl    │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│ Step 1: 关键词粗筛（现有 ToolRecaller）│
│ - 快速，<10ms                        │
│ - 取 top-K*3 候选                    │
│ - 候选超过配置上限时截断              │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│ Step 2: Snippet Sanitizer            │
│ - IP / token / secret / 长日志裁剪    │
│ - 只把必要摘要和 metadata 给模型       │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│ Step 3: LLM Rerank（新增）           │
│ - 对候选打 0-10 分                   │
│ - 取 top-K 返回                      │
│ - 超时 / 解析失败降级为关键词排序      │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│ Step 4: ContextTrace 记录治理信息     │
│ - enabled / degraded / reason        │
│ - candidate_count / latency_ms       │
└─────────────────────────────────────┘
    │
    ▼
排序后的 ToolItems
```

### 4.3 实现思路

```go
// contextengine/tool_reranker.go

type ToolRerankConfig struct {
    Enabled        bool
    CandidateLimit int
    MinCandidates  int
    Timeout        time.Duration
    CacheTTL       time.Duration
    Model          string
}

type ToolReranker struct {
    model     ChatModel
    config    ToolRerankConfig
    sanitizer SnippetSanitizer
}

type RerankOutcome struct {
    Items      []ContextItem
    Enabled    bool
    Degraded   bool
    Reason     string
    LatencyMs  int64
}

func (r *ToolReranker) Rerank(ctx context.Context, query string, items []ContextItem, topK int) RerankOutcome {
    if len(items) == 0 {
        return RerankOutcome{Items: nil, Reason: "empty_candidates"}
    }
    if !r.config.Enabled || len(items) < r.config.MinCandidates {
        return RerankOutcome{Items: takeTopK(items, topK), Reason: "disabled_or_too_few_candidates"}
    }

    start := time.Now()
    rerankCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
    defer cancel()

    candidates := items
    if len(candidates) > r.config.CandidateLimit {
        candidates = candidates[:r.config.CandidateLimit]
    }

    safeCandidates := r.sanitizer.Sanitize(candidates)
    scores, err := r.scoreCandidates(rerankCtx, query, safeCandidates)
    if err != nil {
        return RerankOutcome{
            Items:     takeTopK(items, topK),
            Enabled:   true,
            Degraded:  true,
            Reason:    err.Error(),
            LatencyMs: time.Since(start).Milliseconds(),
        }
    }

    return RerankOutcome{
        Items:     r.selectByScore(candidates, scores, topK),
        Enabled:   true,
        LatencyMs: time.Since(start).Milliseconds(),
    }
}

func (r *ToolReranker) scoreCandidates(ctx context.Context, query string, items []ContextItem) ([]float64, error) {
    prompt := buildRerankPrompt(query, items)
    resp, err := r.model.Generate(ctx, prompt)
    if err != nil {
        return nil, err
    }
    scores, ok := parseRerankScores(resp, len(items))
    if !ok {
        return nil, fmt.Errorf("failed to parse rerank scores")
    }
    return scores, nil
}

func buildRerankPrompt(query string, items []ContextItem) string {
    var sb strings.Builder
    sb.WriteString("你是运维专家。判断以下工具结果和用户问题的相关性。\n\n")
    sb.WriteString(fmt.Sprintf("用户问题：%s\n\n", query))
    sb.WriteString("工具结果（已脱敏和裁剪）：\n")
    for i, item := range items {
        text := item.Content
        if len(text) > 200 {
            text = text[:200] + "..."
        }
        sb.WriteString(fmt.Sprintf("[%d] source=%s title=%s content=%s\n", i+1, item.SourceType, item.Title, text))
    }
    sb.WriteString("\n严格按以下 JSON 格式输出，不要添加任何其他文字：\n")
    sb.WriteString(`{"scores": [{"id": 1, "score": 9}, {"id": 2, "score": 2}]}`)
    return sb.String()
}

func parseRerankScores(resp string, expectedCount int) ([]float64, bool) {
    type scoreEntry struct {
        ID    int     `json:"id"`
        Score float64 `json:"score"`
    }
    type rerankResult struct {
        Scores []scoreEntry `json:"scores"`
    }

    var result rerankResult
    if err := json.Unmarshal([]byte(resp), &result); err != nil {
        return parseRerankScoresRegex(resp, expectedCount)
    }

    scores := make([]float64, expectedCount)
    for _, entry := range result.Scores {
        idx := entry.ID - 1
        if idx >= 0 && idx < expectedCount {
            scores[idx] = math.Min(10, math.Max(0, entry.Score))
        }
    }
    return scores, true
}

func parseRerankScoresRegex(resp string, expectedCount int) ([]float64, bool) {
    re := regexp.MustCompile(`\[(\d+)\]\s*(\d+(?:\.\d+)?)`)
    matches := re.FindAllStringSubmatch(resp, -1)
    if len(matches) == 0 {
        return nil, false
    }
    scores := make([]float64, expectedCount)
    for _, m := range matches {
        id, _ := strconv.Atoi(m[1])
        score, _ := strconv.ParseFloat(m[2], 64)
        idx := id - 1
        if idx >= 0 && idx < expectedCount {
            scores[idx] = math.Min(10, math.Max(0, score))
        }
    }
    return scores, true
}
```

### 4.4 Prompt 设计

```
你是运维专家。判断以下工具结果和用户问题的相关性。

用户问题：Redis 连接超时怎么排查

工具结果：
[1] connection timeout to Redis server at [private-ip]:6379
[2] CPU usage 87% on checkoutservice
[3] Redis maxclients reached, rejecting new connections
[4] payment service latency p99=500ms
[5] normal log entry: health check passed

严格按以下 JSON 格式输出，不要添加任何其他文字：
{"scores": [{"id": 1, "score": 9}, {"id": 2, "score": 2}]}
```

**预期输出：**
```json
{"scores": [{"id": 1, "score": 9}, {"id": 2, "score": 2}, {"id": 3, "score": 10}, {"id": 4, "score": 3}, {"id": 5, "score": 0}]}
```

**输出解析策略**：优先 JSON 解析，失败时用正则 `\[(\d+)\]\s*(\d+)` 兜底，两次都失败则降级。

### 4.5 配置项

所有 LLM rerank 能力必须走配置，不能把候选数、超时、模型和开关写死在代码里。

```yaml
context:
  tool_rerank:
    enabled: false
    profiles: ["ai_ops"]
    min_candidates: 6
    candidate_limit: 20
    timeout_ms: 2000
    cache_ttl_seconds: 300
    model: "glm_fast"
```

默认关闭。只有 benchmark 证明 ToolItems 是瓶颈，并且 replay case 通过后再打开。

### 4.6 降级策略

```
LLM Rerank 成功？
    │
    ├── Yes → 使用 LLM 排序结果
    │
    └── No（超时/报错/解析失败）
            │
            ▼
        降级为关键词排序结果（现有逻辑）
```

降级时必须写入 ContextTrace：
- `tool_rerank.enabled=true`
- `tool_rerank.degraded=true`
- `tool_rerank.reason=timeout|model_error|parse_error|sanitizer_error`
- `tool_rerank.candidate_count`
- `tool_rerank.latency_ms`

这样 replay 和线上 trace 能解释"为什么没有用 LLM 精排"，而不是静默退回。

### 4.7 延迟预算

| 场景 | 延迟 | 说明 |
|------|------|------|
| 关键词粗筛 | <10ms | |
| Snippet Sanitizer | <10ms | 只做规则脱敏和裁剪 |
| LLM Rerank（成功） | 200-1000ms | 这是目标预算，不是保证值，取决于远端模型和候选数量 |
| LLM Rerank（降级） | <10ms | |
| **总计（正常）** | **210-1020ms** | 只在高价值场景启用 |
| **总计（降级）** | **<30ms** | 不阻塞主链路 |

**控制延迟的关键**：
1. 候选超过配置上限时截断，避免 prompt 过长导致延迟不可控。
2. 只对粗筛后的候选精排，不让 LLM 面对全量工具结果。
3. 设置 timeout，失败后保留关键词排序结果。
4. 用 query + candidate ids 做短 TTL 缓存，减少重复请求。

## 5. Phase 3 详细设计：LLM 意图识别

### 5.1 触发条件

当 Benchmark 发现以下情况时触发：
- 不同类型的问题需要明显不同的召回策略
- Profile 机制的 mode 粒度不够细

**具体判断方法**：
1. 人工标注 50 条真实 query 为 fault_diagnosis / knowledge_query / chat
2. 分组跑 Phase 1 召回，人工评估召回质量（每组打 1-5 分）
3. 如果各组平均分差异 > 1.5，说明需要差异化策略

### 5.2 方案：独立 LLM 调用

意图识别必须发生在 ContextEngine 组装上下文之前，且不依赖 Agent 的运行时输出。
因此必须用独立的 LLM 调用，不能复用 Chat Agent 的 system prompt。

```go
// contextengine/intent_recognizer.go

const intentTimeout = 500 * time.Millisecond

type IntentRecognizer struct {
    model   ChatModel
    timeout time.Duration
}

type IntentType string

const (
    IntentFaultDiagnosis IntentType = "fault_diagnosis"
    IntentKnowledgeQuery IntentType = "knowledge_query"
    IntentChat           IntentType = "chat"
)

type IntentResult struct {
    Type     IntentType
    Degraded bool
}

func (r *IntentRecognizer) Recognize(ctx context.Context, query string) IntentResult {
    intentCtx, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()

    prompt := fmt.Sprintf(`判断以下问题的类型，只输出 JSON，不要添加其他文字：
{"type": "fault_diagnosis"}

可选类型：
- fault_diagnosis：故障排查（用户遇到了问题，需要诊断）
- knowledge_query：知识查询（用户想了解某个概念或配置）
- chat：闲聊（用户在闲聊或问候）

问题：%s`, query)

    resp, err := r.model.Generate(intentCtx, prompt)
    if err != nil {
        return IntentResult{Type: IntentKnowledgeQuery, Degraded: true}
    }

    intent, ok := parseInt(resp)
    if !ok {
        return IntentResult{Type: IntentKnowledgeQuery, Degraded: true}
    }

    return IntentResult{Type: intent}
}

func parseInt(resp string) (IntentType, bool) {
    type intentResp struct {
        Type string `json:"type"`
    }
    var r intentResp
    if err := json.Unmarshal([]byte(resp), &r); err == nil {
        switch IntentType(r.Type) {
        case IntentFaultDiagnosis, IntentKnowledgeQuery, IntentChat:
            return IntentType(r.Type), true
        }
    }

    lower := strings.ToLower(resp)
    if strings.Contains(lower, "fault_diagnosis") || strings.Contains(lower, "故障") {
        return IntentFaultDiagnosis, true
    }
    if strings.Contains(lower, "knowledge_query") || strings.Contains(lower, "知识") {
        return IntentKnowledgeQuery, true
    }
    if strings.Contains(lower, "chat") || strings.Contains(lower, "闲聊") {
        return IntentChat, true
    }
    return "", false
}
```

**为什么不用 system prompt 方案**：
1. 时序问题：意图识别发生在上下文组装之前，不能依赖 Agent 运行时输出
2. 可靠性问题：GLM-4.5-AIR 连 function calling 都不稳定，不保证输出 `[INTENT: xxx]` 格式
3. 解耦问题：ContextEngine 不应该依赖 Agent 的运行时行为

### 5.3 意图 → Profile 映射

意图不直接控制权重，而是选择对应的 Profile。Profile 内部的 token 预算分配由现有的 Budget 机制决定。

```go
var intentProfileMap = map[IntentType]string{
    IntentFaultDiagnosis: "ai_ops",    // 偏重工具结果和文档
    IntentKnowledgeQuery: "ai_ops",    // 偏重文档
    IntentChat:           "chat",      // 偏重历史消息
}
```

**为什么不直接改权重**：
1. 现有 Profile 机制已经管理了 token 预算分配，再加一层权重会和 Budget 冲突
2. 意图和来源权重不是线性关系——同样是 fault_diagnosis，"Redis 超时"需要 doc + tool，"Pod Crash"需要 tool + history
3. 通过 Profile 切换，可以复用已有的 selectHistory / selectDocuments / selectToolItems 逻辑，不需要额外的权重乘法

**未来扩展**：如果 ai_ops profile 内部还需要差异化（比如故障排查时加大 ToolItems 的 topK），可以在 Profile 内加意图相关的覆盖配置，而不是在外面加权重矩阵。

### 5.4 降级策略

```
意图识别成功？
    │
    ├── Yes → 使用识别到的意图选择 Profile
    │
    └── No（超时/报错/解析失败）
            │
            ▼
        降级为默认 Profile（现有 mode 路由逻辑）
```

## 6. Benchmark 指标

### 6.1 统一 Recall Benchmark

```
目标：先判断哪个召回源影响最大，再决定是否引入 LLM。

召回源：
- history：历史对话
- memory：长期记忆
- docs：知识库文档
- tool_outputs：工具执行结果

方法：
1. 收集 50-100 条真实对话或 replay case
2. 对每条 query 标注 expected item ids 和 source_type
3. 跑当前 ContextEngine，记录 selected / dropped / degraded / latency
4. 计算 Recall@K、MRR、NDCG、P95 latency、degraded_count
5. 对低分 case 做人工复盘，区分"召回算法问题"和"上游没有候选"
```

### 6.2 相关性标注口径

一条 item 被标注为相关，必须满足至少一个条件：
- 能直接回答用户问题
- 能支撑故障定位中的关键判断
- 能提供必要的服务名、错误类型、时间窗口或历史处置经验
- 能排除一个高风险误判

不标注为相关的情况：
- 只是主题相近，但不能用于当前问题
- 只包含泛化话术或助手自我介绍
- 是过期、低置信度或跨会话无关记忆
- 是工具原始噪声，没有可解释诊断价值

### 6.3 何时触发 Phase 2.5（LLM Rerank）

```
触发条件：
1. 某一召回源 Recall@5 < 0.6 或 MRR 明显低于其他来源
2. 低分 case 主要来自排序不准，而不是候选缺失
3. 确定性优化后仍无法达到目标
4. replay case 证明 LLM rerank 能提升召回质量，且 P95 延迟在预算内
```

### 6.4 何时触发 Phase 3（LLM 意图识别）

```
指标：不同意图分组的召回质量评分（1-5 分）
方法：
1. 人工标注 50 条真实 query 的意图（fault_diagnosis / knowledge_query / chat）
2. 分组跑 Phase 1 召回，人工评估每条的召回质量
3. 计算各组平均分
触发条件：各组平均分差异 > 1.5
```

## 7. 决策树

```
开始
│
├── P0：统一 Recall Benchmark
│   ├── 标注 history / memory / docs / tool_outputs
│   └── 计算 Recall@K / MRR / NDCG / P95
│
├── Phase 1：纯轻量方案（当前）
│   ├── History: Embedding
│   ├── Memory: scope / confidence / budget
│   ├── Docs: hybrid retrieval
│   ├── Tool: 关键词 + metadata
│   └── Intent: Profile 机制
│
│   ▼ 跑 Benchmark
│
├── 某个召回源低分？
│   ├── Yes → Phase 2：先做确定性优化
│   └── No → 保持 Phase 1
│
│   ▼ 再跑 Benchmark
│
├── 仍是排序问题且收益明确？
│   ├── Yes → Phase 2.5：局部 LLM Rerank
│   └── No → 不引入 LLM，继续修候选质量
│
│   ▼ 再跑 Benchmark
│
├── 不同意图分组召回质量差异 > 1.5 分？
│   ├── Yes → Phase 3：加 LLM 意图识别 → Profile 切换
│   └── No → 保持当前
│
│   ▼ 持续监控
│
└── 结束
```

## 8. 面试话术

> "ContextEngine 的召回层我不是简单把历史、记忆和文档拼进 prompt，而是把它当成一个多源召回和预算治理层来做。
>
> 第一阶段我先用确定性方案保证稳定：History 用 Embedding 相似度，Memory 走 scope、置信度和预算过滤，Docs 走 RAG 混合检索，Tool Outputs 用关键词和 metadata 匹配，Profile 负责不同场景的 token budget。
>
> 在引入 LLM 之前，我会先做 Recall Benchmark，标注每条 query 应该命中的 history、memory、docs 和 tool outputs，统计 Recall@K、MRR、NDCG、P95 延迟和降级次数。这样能判断到底是文档召回差、记忆污染，还是工具结果排序不准。
>
> 如果 benchmark 证明某个来源确实是排序问题，我再局部引入 LLM Rerank。比如 Tool Outputs 会先关键词粗筛，再脱敏裁剪，然后让 LLM 对少量候选打分；整个能力有配置开关、超时、缓存和降级，失败时回到关键词排序，并把原因写进 ContextTrace。
>
> 对我来说，大模型不是第一选择，而是最后一层增强。ContextEngine 的重点是可控、可观测、可降级，先把召回质量和 token budget 管住，再决定是否需要模型精排。"
