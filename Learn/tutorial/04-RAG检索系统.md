# 第 4 章：RAG 检索系统 — "给 LLM 装上企业知识库"

> **本章目标**：理解 OpsCaption 知识检索链路的完整设计，能向面试官清晰解释四种检索模式、Hybrid Retrieval 原理以及全链路超时降级设计。

---

## 1. 白话理解：什么是 RAG？

### 1.1 一句话解释

**RAG = Retrieval-Augmented Generation（检索增强生成）**。在 LLM 回答之前，先从知识库里"查资料"，把查到的相关内容喂给 LLM 作为参考，让 LLM 基于真实文档回答问题。

### 1.2 一个类比：图书馆管理员

想象你要写一篇关于"Redis 连接超时如何排查"的报告：

| | **没有 RAG 的 LLM** | **有 RAG 的 LLM** |
|---|---|---|
| 能力 | 🧠 只能靠训练时"记住"的知识回答 | 🧠 + 📚 先去图书馆查资料，再基于资料回答 |
| 回答质量 | "Redis 连接超时可能是因为网络问题或资源不足，建议检查..." （泛泛而谈） | "根据内部 SOP-2024-003，Redis 连接超时排查步骤：1) 检查 `redis.maxconn` 配置 2) 查看 `slowlog` 是否有慢查询 3) 确认网络策略未拦截..." （具体、有据可循） |
| 内部知识 | ❌ 完全不知道公司内部文档 | ✅ 能检索到公司内部的 Runbook、SOP、历史故障复盘 |

**没有 RAG 的 LLM 像个没去过图书馆的学生**，只能凭记忆回答。**有 RAG 的 LLM 像个图书馆管理员**，先帮你找到相关资料，再根据资料回答——回答有来源、有依据、可追溯。

### 1.3 RAG 核心流程（一张图看懂）

```
┌──────────────────────────────────────────────────────────────────┐
│                    离线阶段（索引构建）                              │
│                                                                  │
│  内部文档（Markdown/JSON/YAML）    两阶段分块       向量化+元数据     │
│  ┌────────┐  Loader  ┌────┐  Stage1:  ┌─────┐  embed  ┌───────┐ │
│  │ SOP    │ ───────→ │全文│ ────────→ │ C1  │ ──────→ │Milvus │ │
│  │ Runbook│          └────┘ Markdown  │ C2  │         │向量库  │ │
│  │ 复盘   │                 标题切分   │ C3  │         └───┬───┘ │
│  │ 配置   │                 Stage2:    └─────┘             │     │
│  └────────┘                 语义切分   ┌───────────────────┘     │
│                               (大块)   │                         │
│                                        ▼                         │
│                               ┌──────────────────────┐          │
│                               │  同时构建 BM25 索引    │          │
│                               │  (关键词倒排+IDF统计)  │          │
│                               │  sharedBM25Index      │          │
│                               └──────────────────────┘          │
│                                                                  │
│  _source 去重：同一文件重新索引时，先 deleteExistingSource 再写入    │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                    在线阶段（实时查询）  两路并行                     │
│                                                                  │
│   用户提问                                                       │
│   "Redis 连接超时"                                                │
│        │                                                         │
│        ├──────────────────┬────────────────────┐                 │
│        ▼                  ▼                    ▼                 │
│   Query Rewrite      向量检索(Dense)      BM25检索(Sparse)       │
│   LLM改写搜索词     Doubao Embedding      关键词倒排索引           │
│        │            Milvus ANN检索         BM25公式打分            │
│        │            candidateTopK篇         candidateTopK篇       │
│        │                  │                    │                 │
│        └──────────────────┴────────────────────┘                 │
│                           │                                      │
│                           ▼                                      │
│                    ┌──────────────┐                              │
│                    │  RRF 融合算法 │  score = Σ 1/(60+rank)       │
│                    │  两路结果融合  │                              │
│                    └──────┬───────┘                              │
│                           │                                      │
│                           ▼                                      │
│                    ┌──────────────┐                              │
│                    │ RetrieveRefine│  token交叉打分+元数据加权      │
│                    │ 重排序+去噪   │  service/pod名高权重匹配       │
│                    └──────┬───────┘                              │
│                           │                                      │
│                           ▼                                      │
│                    ┌──────────────┐                              │
│                    │  LLM Rerank  │  对每篇文档打0-10分            │
│                    │  最终精排     │  取Top-K返回                  │
│                    └──────────────┘                              │
└──────────────────────────────────────────────────────────────────┘
```

**核心公式**：

```
RAG 回答 = LLM（用户问题 + 检索到的相关文档） → 基于证据的回答
```

不是"猜答案"，而是"查资料再回答"。每一步都有来源。

---

## 2. 为什么需要 RAG？

### 2.1 LLM 的"知识盲区"

| 局限 | 说明 | OpsCaption 场景影响 |
|---|---|---|
| **不知道内部文档** | 训练数据是公开互联网，不包含公司内部 SOP、Runbook | 用户问"checkoutservice CPU 告警怎么办"，LLM 只能泛泛而谈 |
| **不知道历史故障** | 不知道"上个月 Redis 挂了是因为 maxconn 太小" | 无法复用历史经验，每次都从零开始排查 |
| **知识有截止日期** | DeepSeek V3 训练数据停在一定时间点 | 不知道最近变更的配置和架构 |
| **幻觉问题** | LLM 在没有信息时会"编造"答案 | 可能给出看起来很对但实际错误的排查步骤 |

### 2.2 RAG 如何解决

在 OpsCaption 中，RAG 链路是 **ReAct Agent 的知识底座**。当 Agent 调用 `search_knowledge` 工具时，背后就是这套 RAG 系统在工作：

```
用户：Redis 连接超时怎么排查？

ReAct Agent: 我先去知识库查一下相关文档
    │
    ▼
RAG 系统：
    1. Query Rewrite: "Redis 连接超时" → "Redis connection timeout 排查 故障 recovery"
    2. Milvus 向量检索: 找到 20 篇候选文档
    3. RetrieveRefine: 去重 + 相关性过滤
    4. Rerank: LLM 打分重排序，取 Top-5
    │
    ▼
返回 Top-5 最相关文档 → LLM 基于文档生成回答
```

**核心理念**：让 LLM 的回答从"凭记忆发挥"变成"有据可循"。

---

## 2.5. 离线预处理：从原始文档到可检索索引

> 俗话说 "Garbage in, garbage out"——检索质量的上限由索引质量决定。
> 离线阶段的核心任务就是把杂乱的原始文档变成可被高效检索的结构化索引。

### 2.5.1 文档加载：统一入口

```go
// indexing_service.go - IndexSource 入口
func (s *IndexingService) IndexSource(ctx context.Context, path string) (IndexBuildSummary, error) {
    // 1. 用 Loader 加载原始文件
    loader, _ := s.newLoader(ctx)
    docs, _ := loader.Load(ctx, document.Source{URI: path})
    
    // 2. 解决 _source 元数据（用于后续去重）
    sourceValue := resolveDocumentSource(path, docs[0])
    
    // 3. 删除该 source 的旧索引数据（去重！）
    deleted, _ := s.deleteExistingSource(ctx, sourceValue)
    
    // 4. 执行索引 Pipeline：分块 → 向量化 → 写入 Milvus
    ids, _ := graph.Invoke(ctx, document.Source{URI: path})
    
    // 5. 同步重建 BM25 索引
    s.SyncBM25Index(ctx)
}
```

### 2.5.2 两阶段分块（Chunking）：先结构后语义

```go
// transformer.go - twoStageTransformer
func newDocumentTransformer(ctx context.Context) (document.Transformer, error) {
    // Stage 1: Markdown 标题切分
    mdSplitter, _ := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
        Headers: map[string]string{
            "#":    "title",      // h1 → title 元数据
            "##":   "subtitle",   // h2 → subtitle
            "###":  "section",    // h3 → section
            "####": "subsection", // h4 → subsection
        },
    })
    
    // Stage 2: 语义切分（只对大块使用）
    semSplitter, _ := semantic.NewSplitter(ctx, &semantic.Config{
        Embedding:    DoubaoEmbedding,  // 用 embedding 算相似度断点
        MinChunkSize: 50,              // 最小分块 50 字符
        Percentile:   0.85,            // 在相似度 85 分位处切分
        Separators:   []string{"\n\n", "\n", "。", ".", "？", "!"},
    })
    
    return &twoStageTransformer{stage1: mdSplitter, stage2: semSplitter}, nil
}
```

**两阶段策略**：

```
原始文档（可能数万字）
    │
    ▼ Stage 1: Markdown 标题切分
┌────┬────┬────┬────┬────┐
│ C1 │ C2 │ C3 │ C4 │ C5 │   按 #/##/### 标题边界切
└────┴────┴────┴──┬─┴────┘
                   │ C4 超过 800 字符 → 进入 Stage 2
                   ▼ Stage 2: 语义切分
            ┌─────┬─────┐
            │C4.1 │C4.2 │    按语义相似度断点切分
            └─────┴─────┘

最终输出：[C1, C2, C3, C4.1, C4.2, C5]
```

**设计依据**：
- **Stage 1 优先按标题切**：运维文档（SOP/Runbook/复盘）的 Markdown 标题层级本身就是最佳边界——`## 排查步骤` 和 `## 解决方案` 天然应该分成两块。
- **Stage 2 对大块语义补切**：超过 800 字符的块（如某个 `###` 下面的一大段文字），用 embedding 计算相邻句子的相似度，在相似度骤降处切分——保证每块都在"同一个话题"内。
- **非 Markdown 文档**（JSON/YAML）目前依赖 semantic splitter 或走专门的 chunking 策略（AGENTS.md 提到 `"RAG chunking 不能只按 Markdown 标题切，要支持 JSONL case 级切分"`）。

### 2.5.3 Embedding 向量化：文本 → 向量

```go
// 使用 Doubao Embedding 模型
eb, _ := embedder.DoubaoEmbedding(ctx)
// → 生成 1024 维向量，写入 Milvus 向量库
```

**选型原因**：Doubao Embedding 对中文技术文档效果好，与 Milvus 配合写入向量库。文档元数据（service_tokens、pod_tokens 等）作为 Milvus 的标量字段一起存储，供 RetrieveRefine 阶段做结构化交叉打分。

### 2.5.4 BM25 索引同步：双索引策略

每次索引更新后，都会**全量重建** BM25 索引：

```go
// indexing_service.go - SyncBM25Index
func (s *IndexingService) SyncBM25Index(ctx context.Context) {
    idx := NewBM25Index()
    // 遍历所有已索引的文件
    for _, path := range collectBM25SourcePaths(common.FileDir) {
        docs, _ := loader.Load(ctx, document.Source{URI: path})
        for _, doc := range docs {
            AddDocToBM25Index(idx, doc)  // 分词+统计加入倒排索引
        }
    }
    SetSharedBM25Index(idx)  // 全局单例替换
}
```

**为什么全量重建而不是增量？** 当前文档量不大（几十到几百篇），全量重建成本可控（毫秒级）。增量更新需要维护删除和更新的 diff 逻辑，复杂度远大于收益。文档量到万级别时才需要切换为增量。

### 2.5.5 _source 去重：同一文件重复索引保护

```go
// indexing_service.go - deleteExistingSource
func (s *IndexingService) deleteExistingSource(ctx context.Context, sourceValue string) (int, error) {
    // 查询 Milvus 中所有 _source == sourceValue 的文档
    expr := fmt.Sprintf(`metadata["_source"] == "%s"`, sourceValue)
    queryResult, _ := cli.Query(ctx, collectionName, []string{}, expr, []string{"id"})
    
    // 批量删除旧数据
    deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
    cli.Delete(ctx, collectionName, "", deleteExpr)
}
```

**去重时机在写入之前**——同一文件（如 `sop-redis.md`）重复执行 `IndexSource`，会先把该文件之前的所有 chunk 从 Milvus 删掉，再写入新的 chunk。这样保证：
- 文件内容更新后，旧 chunk 不会残留在向量库里
- 检索结果不会出现同一个文档的多个版本

---

## 3. 四种检索模式

OpsCaption 支持四种检索模式，通过 `QueryMode` 常量定义。默认使用 `full` 模式。

```go
// query.go - QueryMode 定义
const (
    QueryModeRetrieveOnly          QueryMode = "retrieve"  // 模式一：直接检索
    QueryModeRewriteRetrieve       QueryMode = "rewrite"   // 模式二：改写后检索
    QueryModeRewriteRetrieveRerank QueryMode = "full"      // 模式三：改写+检索+重排序（默认）
    QueryModeHybrid                QueryMode = "hybrid"    // 模式四：向量+关键词混合
)
```

| 模式 | 值 | 流程 | 适用场景 | 耗时 |
|---|---|---|---|---|
| **retrieve** | `"retrieve"` | 用户 query → Milvus 向量检索 → top-K | 快速查询，对精度要求不高 | ⚡ 快 |
| **rewrite** | `"rewrite"` | 用户 query → LLM 改写 → Milvus 检索 → top-K | 用户表达口语化，需要优化搜索词 | ⚡⚡ 较快 |
| **full** | `"full"` | 用户 query → 改写 → 检索 → 去重过滤 → LLM Rerank → top-K | **默认模式**，精度优先 | ⚡⚡⚡ 较慢 |
| **hybrid** | `"hybrid"` | 向量检索 ∥ BM25 关键词检索 → RRF 融合 → top-K | 需要精确命中术语（如 error code、pod name） | ⚡⚡ 较快（并行） |

### 模式选择逻辑

```go
// query.go - ParseQueryMode 与 DefaultQueryMode
func ParseQueryMode(raw string) (QueryMode, error) {
    switch strings.ToLower(strings.TrimSpace(raw)) {
    case "", "full", "query", "rerank":
        return QueryModeRewriteRetrieveRerank, nil  // 空字符串默认走 full
    case "rewrite":
        return QueryModeRewriteRetrieve, nil
    case "retrieve":
        return QueryModeRetrieveOnly, nil
    case "hybrid":
        return QueryModeHybrid, nil
    }
}

func DefaultQueryMode(ctx context.Context) QueryMode {
    // 从 config.yaml 读取 rag.default_query_mode，若未配置则返回 retrieve
    return QueryModeRetrieveOnly
}
```

> **面试要点**：虽然 `DefaultQueryMode` 返回 `retrieve`，但在 Chat 管道中会显式传入 `full` 模式。显式传参优先于默认值。

---

## 4. 代码拆解：Full Pipeline 的六步之旅

下面以 **full 模式**（改写 + 检索 + 重排序）为例，逐步拆解完整链路。

```
用户输入: "服务挂了"
     │
     ▼
┌──────────────────────────────────────────────────────────────┐
│  Step 1: Query Rewrite（查询改写）                             │
│  "服务挂了" → "服务故障排查 pod failure recovery"               │
│  超时: 3s，失败则降级回原始 query                               │
├──────────────────────────────────────────────────────────────┤
│  Step 2: RetrieverPool.GetOrCreate()                          │
│  按 Milvus 地址 + top_k 做缓存 key，避免每次新建连接             │
│  失败走短 TTL 缓存（15s），防止雪崩                              │
├──────────────────────────────────────────────────────────────┤
│  Step 3: Milvus ANN 向量检索                                   │
│  改写后的 query → Doubao Embedding → Milvus 相似度搜索          │
│  取 candidateTopK（topK × 4，上限 50）篇候选                    │
├──────────────────────────────────────────────────────────────┤
│  Step 4: RetrieveRefine（去重 + 相关性过滤）                    │
│  基于 query token 与文档元数据交叉打分，重新排序                 │
├──────────────────────────────────────────────────────────────┤
│  Step 5: Rerank（LLM 重排序）                                  │
│  LLM 对每篇文档打 0-10 分，按分降序取最终 top-K                 │
│  超时: 5s，失败则保持 refine 后的顺序                           │
├──────────────────────────────────────────────────────────────┤
│  Step 6: 返回 top-K 文档 + QueryTrace                          │
│  QueryTrace 记录每一步的耗时和状态，便于可观测                    │
└──────────────────────────────────────────────────────────────┘
```

### 4.1 入口函数：QueryWithMode

`query.go` 中，`Query` 和 `QueryWithMode` 是 RAG 检索的统一入口：

```go
// query.go - Query 和 QueryWithMode

func Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
    return QueryWithMode(ctx, pool, query, DefaultQueryMode(ctx))
}

func QueryWithMode(ctx context.Context, pool *RetrieverPool, query string, mode QueryMode) ([]*schema.Document, QueryTrace, error) {
    if mode == QueryModeHybrid {
        return hybridQueryWithMode(ctx, pool, query)  // Hybrid 走独立分支
    }
    return queryWithMode(ctx, pool, query, mode, RewriteQuery, Rerank)
}
```

**关键设计**：`RewriteQuery` 和 `Rerank` 作为函数参数传入，而非硬编码在 `queryWithMode` 中。这样设计便于测试时注入 mock 函数。

### 4.2 queryWithMode：核心调度逻辑

```go
// query.go - queryWithMode（核心调度函数）

func queryWithMode(ctx context.Context, pool *RetrieverPool, query string,
    mode QueryMode, rewrite rewriteFunc, rerank rerankFunc,
) ([]*schema.Document, QueryTrace, error) {

    if strings.TrimSpace(query) == "" {
        return nil, QueryTrace{}, nil  // 空查询直接返回
    }

    trace := QueryTrace{
        Mode:           string(mode),
        OriginalQuery:  query,
        RewrittenQuery: query,  // 默认与原 query 相同（降级用）
    }

    topK := RetrieverTopK(ctx)
    candidateTopK := RetrieverCandidateTopK(ctx)  // = topK × 4，范围 [20, 50]

    // ===== Step 1: Query Rewrite =====
    rewritten := query
    if mode != QueryModeRetrieveOnly {
        rewriteStart := time.Now()
        rewritten = rewrite(ctx, query)       // 调用 RewriteQuery
        trace.RewriteLatencyMs = time.Since(rewriteStart).Milliseconds()
        trace.RewrittenQuery = rewritten
    }

    // ===== Step 2: 获取 Retriever（连接池）=====
    rr, acquisition, err := pool.GetOrCreate(ctx)
    trace.CacheKey = acquisition.CacheKey
    trace.CacheHit = acquisition.CacheHit
    if err != nil {
        return nil, trace, err
    }

    // ===== Step 3: Milvus 向量检索 =====
    retrieveStart := time.Now()
    docs, err := rr.Retrieve(ctx, rewritten, retrieverapi.WithTopK(candidateTopK))
    trace.RetrieveLatencyMs = time.Since(retrieveStart).Milliseconds()
    trace.RawResultCount = len(docs)
    if err != nil {
        return nil, trace, err
    }

    // ===== Step 4: RetrieveRefine =====
    docs = refineRetrievedDocs(query, docs)

    // ===== 非 full 模式到此结束 =====
    if mode != QueryModeRewriteRetrieveRerank {
        finalDocs := trimRetrievedDocs(docs, topK)
        trace.ResultCount = len(finalDocs)
        trace.RerankEnabled = false
        return finalDocs, trace, nil
    }

    // ===== Step 5: Rerank =====
    rerankStart := time.Now()
    rerankResult := rerank(ctx, query, docs, topK)  // 调用 Rerank
    trace.RerankLatencyMs = time.Since(rerankStart).Milliseconds()
    trace.RerankEnabled = rerankResult.Enabled

    // ===== Step 6: 返回结果 =====
    finalDocs := rerankResult.Docs
    trace.ResultCount = len(finalDocs)
    return finalDocs, trace, nil
}
```

### 4.3 Step 1: Query Rewrite — 把"人话"变成"搜索词"

```go
// query_rewrite.go - RewriteQuery

const (
    defaultRewriteTimeout = 3 * time.Second  // 改写超时 3 秒
    rewriteSystemPrompt   = `You are a search query optimizer for an IT operations knowledge base.
Your job: rewrite the user's question into a concise, keyword-rich search query that maximizes retrieval recall.
Rules:
- Output ONLY the rewritten query, nothing else.
- Keep technical terms, error codes, and proper nouns unchanged.
- Expand abbreviations and slang into standard terms.
- Use Chinese if the original is Chinese, English if English.
- Maximum 50 characters.`
)

func RewriteQuery(ctx context.Context, query string) string {
    trimmed := strings.TrimSpace(query)
    if trimmed == "" {
        return query
    }

    // 创建带超时的 context
    rewriteCtx, cancel := context.WithTimeout(ctx, rewriteTimeout(ctx))
    defer cancel()

    chatModel, err := models.OpenAIForGLMFast(rewriteCtx)
    if err != nil {
        g.Log().Debugf(ctx, "query rewrite skipped: model init failed: %v", err)
        return query  // ← 失败降级：返回原始 query
    }

    resp, err := chatModel.Generate(rewriteCtx, []*schema.Message{
        {Role: schema.System, Content: rewriteSystemPrompt},
        {Role: schema.User, Content: trimmed},
    })
    if err != nil {
        g.Log().Debugf(ctx, "query rewrite failed: %v", err)
        return query  // ← 失败降级：返回原始 query
    }

    rewritten := strings.TrimSpace(resp.Content)
    if rewritten == "" {
        return query  // ← 空结果降级
    }

    return rewritten
}
```

**改写效果示例**：

| 用户输入（口语化） | 改写后（搜索优化） |
|---|---|
| "服务挂了" | "服务故障排查 pod failure recovery" |
| "支付报 503" | "payment service 503 error gateway timeout" |
| "Redis 连不上" | "Redis 连接失败 connection timeout 排查" |
| "数据库慢" | "MySQL latency high slow query 优化" |

**降级策略**：3 秒超时、模型初始化失败、LLM 调用失败、返回空结果 —— 四种情况都 **静默降级回原始 query**。用户无感知。

### 4.4 Step 2: RetrieverPool — 连接池与雪崩防护

```go
// retriever_pool.go - RetrieverPool

type RetrieverPool struct {
    mu         sync.Mutex
    factory    RetrieverFactory      // 创建 Retriever 的工厂函数
    cacheKeyFn CacheKeyFunc          // 缓存 key 生成函数
    ttlFn      FailureTTLFunc        // 失败缓存 TTL
    state      cachedRetriever       // 当前缓存的 retriever
}

type cachedRetriever struct {
    key      string                 // 缓存 key
    rr       retrieverapi.Retriever // 缓存的 retriever 实例
    lastErr  error                 // 上次创建失败的错误
    failedAt time.Time             // 失败时间戳
}

func (p *RetrieverPool) GetOrCreate(ctx context.Context) (retrieverapi.Retriever, RetrieverAcquisition, error) {
    cacheKey := p.cacheKeyFn(ctx)
    ttl := p.ttlFn(ctx)

    p.mu.Lock()
    defer p.mu.Unlock()

    // 情况 1：缓存命中，直接返回
    if p.state.rr != nil && p.state.key == cacheKey {
        acquisition.CacheHit = true
        return p.state.rr, acquisition, nil
    }

    // 情况 2：上次创建失败且在 TTL 内，直接返回缓存的错误（防止雪崩）
    if p.state.key == cacheKey && p.state.lastErr != nil &&
        time.Since(p.state.failedAt) < ttl {
        acquisition.InitFailureCached = true
        return nil, acquisition, p.state.lastErr
    }

    // 情况 3：需要新建连接
    initStart := time.Now()
    rr, err := p.factory(ctx)
    acquisition.InitLatencyMs = time.Since(initStart).Milliseconds()

    if err != nil {
        // 缓存失败结果，防止短时间内大量重试
        p.state = cachedRetriever{key: cacheKey, lastErr: err, failedAt: time.Now()}
        return nil, acquisition, err
    }

    p.state = cachedRetriever{key: cacheKey, rr: rr}
    return rr, acquisition, nil
}
```

**缓存 Key 的生成**（`config.go`）：

```go
func DefaultRetrieverCacheKey(ctx context.Context) string {
    return fmt.Sprintf("%s|%s|%d",
        common.GetMilvusAddr(ctx),           // Milvus 地址
        common.GetMilvusCollectionName(ctx), // 集合名称
        RetrieverTopK(ctx),                  // top-K
    )
}
```

**雪崩防护设计**：

```
正常情况：
  请求1 → GetOrCreate → 新建连接(100ms) → 缓存 → 返回 ✓
  请求2 → GetOrCreate → 缓存命中 → 返回 ✓
  请求3 → GetOrCreate → 缓存命中 → 返回 ✓

Milvus 宕机时（无防护）：
  请求1 → GetOrCreate → 新建连接(超时5s) → 失败 ✗
  请求2 → GetOrCreate → 新建连接(超时5s) → 失败 ✗
  请求3 → GetOrCreate → 新建连接(超时5s) → 失败 ✗
  ... 每个请求都重试，连接风暴！

Milvus 宕机时（有防护）：
  请求1 → GetOrCreate → 新建连接(超时5s) → 失败 → 缓存错误(TTL=15s)
  请求2 → GetOrCreate → 命中失败缓存 → 立即返回错误 ✗（不重试）
  请求3 → GetOrCreate → 命中失败缓存 → 立即返回错误 ✗（不重试）
  15s 后 → TTL 过期 → 重新尝试连接
```

> **面试要点**：这本质上是 **Circuit Breaker（熔断器）** 的简化实现。通过错误缓存 + 短 TTL，避免 Milvus 不可用时每个请求都等待超时。

### 4.5 Step 3: Milvus ANN 向量检索

检索阶段的核心操作：

```go
// 从 queryWithMode 中提取
rr, acquisition, err := pool.GetOrCreate(ctx)

// 用改写后的 query 检索，取 candidateTopK 篇候选
docs, err := rr.Retrieve(ctx, rewritten, retrieverapi.WithTopK(candidateTopK))
```

**candidateTopK 的计算**（`config.go`）：

```go
func RetrieverCandidateTopK(ctx context.Context) int {
    topK := RetrieverTopK(ctx)  // 默认 3（config.yaml 中配置）
    if v, err := g.Cfg().Get(ctx, "retriever.candidate_top_k"); err == nil && v.Int() > 0 {
        if v.Int() < topK { return topK }
        return v.Int()
    }
    candidate := topK * 4  // 扩放到 4 倍，留足够候选给后续 refine 和 rerank
    if candidate < 20 { candidate = 20 }
    if candidate > 50 { candidate = 50 }
    return candidate
}
```

**检索流程**：

```
Rewrite 后的 query
    │
    ▼
Doubao Embedding 模型 → 生成 1024 维向量
    │
    ▼
Milvus ANN 索引 → 返回 candidateTopK 篇候选文档
    │               （默认 topK=3, candidate=3×4=12, 夹在 [20,50] 区间 → 实际=20）
    ▼
进入 Step 4: RetrieveRefine
```

> **为什么用 candidateTopK 而不是 topK？** 因为后续还有 refine 和 rerank 两步。先从向量库里多召回一些候选，给后面的精排留空间。这就是信息检索中经典的 **"粗排→精排"两阶段策略**。

### 4.6 Step 4: RetrieveRefine — 去重 + 相关性重排序

```go
// retrieve_refine.go - refineRetrievedDocs

func refineRetrievedDocs(query string, docs []*schema.Document) []*schema.Document {
    if len(docs) <= 1 {
        return docs  // 只有 0 或 1 篇，无需 refine
    }

    // 构建 query 的 token 集合
    profile := buildRetrievalQueryProfile(query)
    if len(profile.tokens) == 0 && strings.TrimSpace(profile.rawLower) == "" {
        return docs
    }

    // 为每篇文档打分
    scored := make([]scoredDocument, 0, len(docs))
    for idx, doc := range docs {
        scored = append(scored, scoredDocument{
            doc:   doc,
            score: scoreRetrievedDocument(profile, doc, idx, len(docs)),
            idx:   idx,
        })
    }

    // 按分数降序排列
    sort.SliceStable(scored, func(i, j int) bool {
        if scored[i].score == scored[j].score {
            return scored[i].idx < scored[j].idx  // 分数相同时保持原始顺序
        }
        return scored[i].score > scored[j].score
    })

    // 返回重排后的文档列表
    out := make([]*schema.Document, 0, len(scored))
    for _, item := range scored {
        out = append(out, item.doc)
    }
    return out
}
```

**打分公式**（`scoreRetrievedDocument`）：

```go
func scoreRetrievedDocument(query retrievalQueryProfile, doc *schema.Document, idx, total int) int {
    // 基础分：考虑原始排名（越靠前分越高）
    score := (total - idx) * 2

    profile := buildRetrievalDocProfile(doc)

    // 加分项 1：query token 与文档内容的交叉匹配
    score += overlapScore(query.tokens, profile.contentTokens, 1, 6)

    // 加分项 2：query token 与运维特有元数据的交叉匹配（权重更高！）
    score += overlapScore(query.tokens, profile.metricNames, 3, 9)      // 指标名
    score += overlapScore(query.tokens, profile.traceOperations, 3, 9)  // 链路操作
    score += overlapScore(query.tokens, profile.traceServices, 3, 6)    // 链路服务
    score += overlapScore(query.tokens, profile.serviceTokens, 4, 12)   // 服务名（最高权重）
    score += overlapScore(query.tokens, profile.podTokens, 4, 12)       // Pod 名
    score += overlapScore(query.tokens, profile.nodeTokens, 4, 12)      // 节点名
    score += overlapScore(query.tokens, profile.namespaceTokens, 2, 4)  // 命名空间

    // 加分项 3：精确字段匹配（query 包含 service / source / destination 等）
    score += exactFieldBoost(query.rawLower, profile.service, 8)
    score += exactFieldBoost(query.rawLower, profile.instanceType, 5)
    score += exactFieldBoost(query.rawLower, profile.source, 6)
    score += exactFieldBoost(query.rawLower, profile.destination, 6)

    return score
}
```

**设计亮点**：这不是简单的"把 query 和文档内容做文本匹配"，而是利用文档元数据（`service_tokens`、`pod_tokens`、`metric_names` 等）做**结构化交叉打分**。服务名、Pod 名的匹配权重（4,12）远高于普通内容匹配（1,6），因为运维场景中这些字段的匹配意味着"高度相关"。

### 4.7 Step 5: Rerank — LLM 精排

```go
// rerank.go - Rerank

const (
    defaultRerankTimeout = 5 * time.Second  // Rerank 超时 5 秒
    rerankSystemPrompt   = `You are a document relevance judge for IT operations.
Given a query and a list of documents, rate each document's relevance to the query on a scale of 0-10.
Output ONLY a comma-separated list of scores in the same order as the documents.
Example output: 9,3,7,1,8
Do not output anything else.`
)

func Rerank(ctx context.Context, query string, docs []*schema.Document, topK int) RerankResult {
    if len(docs) <= 1 {
        return RerankResult{Docs: docs, Enabled: false}  // 只有 0 或 1 篇，无需 rerank
    }

    rerankCtx, cancel := context.WithTimeout(ctx, rerankTimeout(ctx))
    defer cancel()

    chatModel, err := models.OpenAIForGLMFast(rerankCtx)
    if err != nil {
        return RerankResult{Docs: docs, Enabled: false}  // ← 降级
    }

    // 构建文档列表（截断每篇内容到 200 字符）
    var sb strings.Builder
    for i, doc := range docs {
        title := docTitle(doc)
        content := doc.Content
        if len(content) > 200 {
            content = content[:200] + "..."
        }
        fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, title, content)
    }

    userMsg := fmt.Sprintf("Query: %s\n\nDocuments:\n%s", query, sb.String())

    resp, err := chatModel.Generate(rerankCtx, []*schema.Message{
        {Role: schema.System, Content: rerankSystemPrompt},
        {Role: schema.User, Content: userMsg},
    })
    if err != nil {
        return RerankResult{Docs: docs, Enabled: false}  // ← 降级
    }

    scores := parseScores(resp.Content, len(docs))
    if scores == nil {
        return RerankResult{Docs: docs, Enabled: false}  // ← 解析失败降级
    }

    // 按分数降序排列，取 topK
    type indexedDoc struct { idx int; doc *schema.Document; score float64 }
    items := make([]indexedDoc, len(docs))
    for i := range docs {
        items[i] = indexedDoc{idx: i, doc: docs[i], score: scores[i]}
    }
    sort.SliceStable(items, func(i, j int) bool {
        return items[i].score > items[j].score
    })

    limit := topK
    if limit <= 0 || limit > len(items) { limit = len(items) }
    reranked := make([]*schema.Document, 0, limit)
    for _, item := range items[:limit] {
        reranked = append(reranked, item.doc)
    }

    return RerankResult{Docs: reranked, Scores: rerankedScores, Enabled: true}
}
```

**Rerank 的价值**：

```
Milvus 返回的候选（按向量相似度排序）：
  1. Doc A: "数据库连接池配置指南"       ← 向量相似，但和"服务挂了"关系不大
  2. Doc B: "Pod 故障恢复流程"           ← 高度相关！
  3. Doc C: "HTTP 502 排查手册"          ← 可能相关
  4. Doc D: "K8s 节点运维手册"           ← 不相关
  ...

LLM Rerank 打分后（0-10 分）：
  1. Doc B: 9.0  ← 和"服务挂了"最相关
  2. Doc C: 7.0  ← 有些相关
  3. Doc A: 3.0  ← 不太相关
  4. Doc D: 1.0  ← 不相关

取 top-3 → [Doc B, Doc C, Doc A]
```

> **面试要点**：Rerank 用 LLM 做——这是 OpsCaption 的特色。相比传统的 Cross-Encoder Reranker（如 BGE-Reranker），LLM Rerank 能理解运维领域的上下文语义，但也更慢、更贵。所以做了 5s 超时和内容截断（200 字符）来平衡性能。

### 4.8 Step 6: QueryTrace — 全链路可观测

```go
// query.go - QueryTrace

type QueryTrace struct {
    Hybrid            *HybridTrace  // Hybrid 模式的详细追踪
    Mode              string         // 使用的检索模式
    CacheKey          string         // RetrieverPool 缓存 key
    CacheHit          bool           // 是否命中连接池缓存
    InitFailureCached bool           // 是否命中失败缓存
    InitLatencyMs     int64          // Retriever 初始化耗时
    RetrieveLatencyMs int64          // Milvus 检索耗时
    RewriteLatencyMs  int64          // Query Rewrite 耗时
    RerankLatencyMs   int64          // Rerank 耗时
    OriginalQuery     string         // 用户原始 query
    RewrittenQuery    string         // 改写后的 query
    RawResultCount    int            // 检索原始结果数
    ResultCount       int            // 最终返回结果数
    RerankEnabled     bool           // 是否启用了 Rerank
}
```

**每一次 RAG 查询都返回完整的 Trace 信息**，真正做到每一步都可观测、可排查。

---

## 4.9. BM25 关键词检索算法（独立详解）

> BM25 是全文检索领域的经典算法，也是 **Hybrid 模式的另一条腿**。
> 如果说向量检索（Dense）是"找意思相近的"——BM25（Sparse）就是"找词匹配的"。

### 4.9.1 核心公式

BM25 对文档 `D` 和查询 `Q` 的相关性打分公式：

```
                  N - df(qᵢ) + 0.5        f(qᵢ, D) · (k₁ + 1)
score(D, Q) = Σ log(───────────────) · ───────────────────────────────
         qᵢ∈Q       df(qᵢ) + 0.5       f(qᵢ, D) + k₁ · (1 - b + b · |D|/avgDL)

其中：
  N      = 文档总数
  df(qᵢ) = 包含词 qᵢ 的文档数（Document Frequency）
  f(qᵢ,D) = 词 qᵢ 在文档 D 中的出现次数（Term Frequency）
  |D|    = 文档 D 的长度
  avgDL  = 所有文档的平均长度
  k₁     = 词频饱和度参数（默认 1.2）
  b      = 长度归一化参数（默认 0.75）
```

### 4.9.2 公式解读——三个核心思想

**① IDF（逆文档频率）：稀有词权重高**

```
         N - df + 0.5
IDF = log(─────────────)
           df + 0.5
```

| 词 | 出现文档数 | IDF | 含义 |
|----|-----------|-----|------|
| "service" | 在 100 篇文档中出现 | ≈ 0 | 到处都是，区分度为零 |
| "connection_refused" | 只在 3 篇中出现 | ≈ 3.5 | 稀有词，高区分度 |
| "OOMKilled" | 只在 1 篇中出现 | ≈ 4.6 | 极稀有，极高区分度 |

这就是为什么 "service" 被放入停用词——它 IDF 太低，对检索几乎没贡献。

**② TF 归一化（词频饱和度）：出现 100 次 ≠ 100 倍相关**

```
        f · (k₁ + 1)
TF_norm = ──────────────────────
          f + k₁ · (1 - b + b·|D|/avgDL)
```

`k₁ = 1.2` 意味着词频超过 1.2 之后，增益迅速衰减——出现 3 次比 1 次相关很多，但出现 50 次和 20 次差别已经不大了。

**③ 文档长度归一化：长文档天然占优，需要惩罚**

`b = 0.75` 表示长度归一化生效 75%。长文档（远超 avgDL）TF 会被压低——避免长文档仅靠 "词多" 就排到前面。

### 4.9.3 分词策略

```go
// bm25.go - bm25Tokenize
func bm25Tokenize(text string) []string {
    lower := strings.ToLower(text)
    // 按 ASCII 字母/数字/_/-/./:/ 切词
    // 过滤：长度 < 2、英文停用词（a/the/is/for...）
    // 运维特有停用词：service、instance、type
}
```

| 策略 | 说明 |
|------|------|
| **只保留 ASCII 字母数字 + 特殊分隔符** | 运维场景中文术语多是英文缩写（OOM、HPA、CPU），中文部分由向量检索覆盖 |
| **停用词过滤** | 英文通用停用词 + 运维高频低区分度词（service/instance/type） |
| **长度过滤** | < 2 字符的 token 丢弃（如单个字母 "a"、"x"） |

> **为什么 "service"、"instance"、"type" 是停用词？** 在运维文档中每篇都出现，IDF 接近零。保留它们只会增加噪音。

### 4.9.4 全局单例共享

```go
// shared_bm25.go
var sharedBM25Index *BM25Index  // 全局单例，sync.Once 保证只建一次

func SetSharedBM25Index(idx *BM25Index) {
    sharedBM25Index = idx  // SyncBM25Index 全量重建后原子替换
}
```

每次文档索引更新 → 全量重建 BM25 索引 → 原子替换全局单例。读操作只需 `RLock`，不阻塞写入。

---

## 4.10. Skills 系统 — Agent 能力的插件化热插拔

> **Skills = Agent 的"技能树"。** 不同故障场景需要不同的排查策略——支付超时和 Pod 崩溃的日志分析重点完全不同。
> Skills 系统让 Agent 根据任务特征**自动选择最匹配的处理策略**，而不是把所有工具一次性砸给 LLM。

### 4.10.1 Skill 接口：统一抽象

```go
// skills/registry.go - Skill 接口
type Skill interface {
    Name()        string                          // 技能名
    Description() string                          // 功能描述
    Match(task *protocol.TaskEnvelope) bool       // 是否匹配当前任务
    Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}
```

**示例——Logs Specialist 注册的 6 个 Skill**：

| Skill 名称 | 匹配关键词 | 作用 |
|-----------|-----------|------|
| `logs_service_offline_panic_trace` | panic, crashloop, pod restart | 追踪服务崩溃的 panic 堆栈和重启原因 |
| `logs_api_failure_rate_investigation` | failure rate, 5xx, 4xx | 追踪 API 失败率飙升的根因 |
| `logs_payment_timeout_trace` | payment timeout, 支付超时 | 追踪支付/订单超时链路 |
| `logs_auth_failure_trace` | login, token, jwt, unauthorized | 追踪认证失败 |
| `logs_evidence_extract` | error, exception, timeout | 通用错误/超时证据提取（默认） |
| `logs_raw_review` | (fallback) | 兜底——返回原始日志片段 |

### 4.10.2 Registry：按域组织 + 首匹配优先

```go
// skills/registry.go - Registry
type Registry struct {
    domain string
    skills []Skill
}

func (r *Registry) Resolve(task *protocol.TaskEnvelope) (Skill, error) {
    for _, skill := range r.skills {
        if skill.Match(task) {     // 找到第一个匹配的 Skill 就返回
            return skill, nil
        }
    }
    return r.skills[0], nil        // 没匹配到 → 返回第一个作为 fallback
}
```

**设计意图**：
- **首匹配优先**：Logs Specialist 的 6 个 Skill 按**特殊→通用**顺序排列——`service_offline_panic_trace` 排前面（精确匹配 panic+crashloop），`evidence_extract` 排后面（通用错误匹配），`raw_review` 在最后（兜底）。
- **有 fallback**：即使所有 Skill 都不匹配（resolver 返回 `skills[0]`），也不会返回错误——确保了 Agent 在任何 query 下都有执行路径。

### 4.10.3 ProgressiveDisclosure：按场景渐进暴露工具

> **如果把 50 个工具全部塞给 LLM，它会在选工具上浪费 token 而且容易选错。** ProgressiveDisclosure 按任务域只暴露相关工具。

```go
// skills/progressive_disclosure.go - 三层工具分级
type ToolTier int

const (
    TierAlwaysOn  ToolTier = 0  // 始终可用（时间查询、文档检索等通用工具）
    TierSkillGate ToolTier = 1  // 匹配到对应 domain 才暴露（日志 MCP、Prometheus）
    TierOnDemand  ToolTier = 2  // 需要时手动扩展（MySQL CRUD 等高风险工具）
)

type TieredTool struct {
    Tool    tool.BaseTool    // 工具实例
    Tier    ToolTier         // 层级
    Domains []string         // 所属技能域（logs/metrics/knowledge）
}
```

**工具分级实例**（`tiered_tools.go`）：

```
TierAlwaysOn（始终暴露）:
  ├── GetCurrentTime           — 获取当前时间
  └── QueryInternalDocs        — RAG 文档检索

TierSkillGate（域匹配才暴露）:
  ├── MCP Log Tools (domain: logs)       — 日志查询（由本地 MCP Server 提供）
  └── PrometheusAlertsQuery (domain: metrics) — Prometheus 告警查询

TierOnDemand（手动扩展）:
  └── MySQLCrudTool (domains: logs/metrics/knowledge) — 数据库操作（需配置白名单）
```

**动态匹配流程**：

```go
func (pd *ProgressiveDisclosure) Disclose(query string) DisclosureResult {
    matchedDomains := pd.matchDomains(query)   // 根据 query 匹配技能域

    for _, tt := range pd.tools {
        switch tt.Tier {
        case TierAlwaysOn:
            result.Tools = append(result.Tools, tt.Tool)   // 恒定暴露

        case TierSkillGate:
            if domainOverlap(matchedDomains, tt.Domains) {
                result.Tools = append(result.Tools, tt.Tool) // 域匹配才暴露
            }
        }
    }
}
```

**效果**：
- 用户问 "服务 CPU 飙升" → matchDomains 返回 `["metrics"]` → 只暴露 Prometheus 工具，不暴露日志工具
- 用户问 "支付超时" → matchDomains 返回 `["logs"]` → 只暴露 MCP 日志工具
- Token 节省：每次请求 LLM 看到的工具列表从 10+ 个缩减到 3-4 个

### 4.10.4 FocusCollector：给 LLM 注入领域聚焦指令

```go
// skills/focus_collector.go - FocusCollector
type FocusProvider interface {
    Focus() string   // 返回该 Skill 的聚焦指令
}

func (c *FocusCollector) Collect(query string) []FocusHint {
    // 遍历所有 Registry，找到匹配的 Skill
    // 如果 Skill 实现了 FocusProvider，提取它的 Focus 指令
    // 注入到 LLM 的 System Prompt 中
}
```

**示例**：用户问 "checkoutservice 支付超时了"

```
匹配到的 FocusHint:
  [logs] Focus on payment, order, checkout, gateway timeout, retry, db timeout, and downstream latency.

→ 注入到 LLM 上下文：告诉 LLM "重点看支付、订单、网关超时、下游延迟相关日志"
→ 而不是让它从头推理应该查什么——大幅降低推理成本
```

---

## 5. Hybrid Retrieval：向量 + BM25 混合检索

### 5.1 为什么要混合？

| 检索方式 | 原理 | 优势 | 劣势 |
|---|---|---|---|
| **向量检索（Dense）** | Doubao Embedding → Milvus ANN | 理解语义，"服务挂了"能找到"Pod failure" | 对精确术语不敏感，"checkoutservice-v2-7d4f8b9c6-xqz9m"这种长 Pod 名容易被忽略 |
| **BM25 关键词（Sparse）** | TF-IDF 变体，纯关键词匹配 | 精确命中 error code、Pod 名、IP 地址 | 不理解语义，"服务挂了"和"service down"不会认为相似 |

**结论**：两者互补。向量擅长语义理解，BM25 擅长精确匹配。合在一起才能覆盖所有检索场景。

### 5.2 Hybrid 架构

```
用户 query: "checkoutservice-v2-7d4f8b9c6-xqz9m CPU 告警"
                    │
        ┌───────────┴───────────┐
        │                       │
        ▼                       ▼
┌───────────────┐       ┌───────────────┐
│  向量检索 (Dense) │       │  BM25 检索 (Lexical) │
│  Doubao Embed  │       │  关键词分词+IDF    │
│  Milvus ANN    │       │  内存索引          │
│  Top-50        │       │  Top-50           │
└───────┬───────┘       └───────┬───────────┘
        │                       │
        └───────────┬───────────┘
                    │
                    ▼
        ┌───────────────────────┐
        │   RRF 融合 (Reciprocal │
        │   Rank Fusion)         │
        │   score = Σ 1/(k+rank) │
        │   k = 60               │
        └───────────┬───────────┘
                    │
                    ▼
        ┌───────────────────────┐
        │   Refine + Top-K      │
        └───────────────────────┘
```

### 5.3 并行执行

```go
// hybrid.go - HybridRetrieve（并行执行部分）

func HybridRetrieve(ctx context.Context, pool *RetrieverPool, lexicalIndex *BM25Index,
    query string, cfg HybridConfig,
) ([]*schema.Document, HybridTrace, error) {

    denseCh := make(chan denseResult, 1)
    lexCh := make(chan lexResult, 1)

    // 并行执行：向量检索（goroutine 1）
    go func() {
        rr, _, err := pool.GetOrCreate(ctx)
        if err != nil {
            denseCh <- denseResult{err: err}
            return
        }
        start := time.Now()
        docs, err := rr.Retrieve(ctx, query, retrieverapi.WithTopK(cfg.DenseTopK))
        denseCh <- denseResult{docs: docs, err: err, latencyMs: time.Since(start).Milliseconds()}
    }()

    // 并行执行：BM25 检索（goroutine 2）
    go func() {
        start := time.Now()
        var hits []BM25Hit
        if lexicalIndex != nil {
            hits = lexicalIndex.Search(query, cfg.LexicalTopK)
        }
        lexCh <- lexResult{hits: hits, latencyMs: time.Since(start).Milliseconds()}
    }()

    // 等待两边都完成
    dr := <-denseCh
    lr := <-lexCh
    // ... 后续融合
}
```

**性能优势**：向量检索（网络 IO + GPU）和 BM25 检索（纯内存计算）完全并行，总耗时 = max(向量耗时, BM25耗时)，而非两者之和。

### 5.4 RRF 融合算法（完整公式推导）

**RRF（Reciprocal Rank Fusion）** 是混合检索的核心——不是"向量分 × 0.7 + BM25 分 × 0.3"这种拍脑袋加权，而是基于**排名倒数**的无参数融合。

```go
// hybrid.go - rrfFusion
func rrfFusion(denseDocs []*schema.Document, lexHits []BM25Hit, k int) []fusedDoc {
    if k <= 0 { k = 60 }  // 平滑因子
    kf := float64(k)

    byID := make(map[string]*entry)

    // 向量检索结果贡献 RRF 分数
    for i, doc := range denseDocs {
        id := docFusionKey(doc)  // 优先用 case_id / doc_id 去重合并
        if id == "" { id = doc.ID }
        rank := i + 1  // 排名从 1 开始
        e.score += 1.0 / (kf + float64(rank))  // ← RRF 核心公式
        e.denseRank = rank
    }

    // BM25 检索结果贡献 RRF 分数
    for i, hit := range lexHits {
        id := hit.DocID
        rank := i + 1
        e.score += 1.0 / (kf + float64(rank))
        e.lexRank = rank
    }

    // 按融合分数降序排列
    sort.SliceStable(results, func(i, j int) bool {
        return results[i].score > results[j].score
    })
    return results
}
```

**RRF 公式**：

```
RRF(文档 d) = Σ ( 1 / (k + rank_i(d)) )
               i∈{dense, sparse}

其中：
  k       = 60（平滑常数）
  rank_i  = 文档 d 在第 i 个检索列表中的排名（从 1 开始）
```

**为什么是 1/(k+rank) 而不是直接加权？**

| 方案 | 问题 |
|------|------|
| 分数加权 `α·vec_score + β·bm25_score` | 向量分数范围 [0.7, 0.99]，BM25 分数范围 [0, 50+]——量纲完全不同，需要反复调参 |
| RRF `1/(k+rank)` | 排名是统一量纲（1, 2, 3...），消除了量纲差异，**k=60 是学术界和实践验证的通用值** |

**完整示例**：

```
用户 query: "checkoutservice CPU 告警"

向量检索(Dense)结果:          BM25检索(Sparse)结果:
  #1: Doc B (相关)              #1: Doc A (精确命中 pod 名)
  #2: Doc C (可能相关)           #2: Doc D (命中 error code)
  #3: Doc A (不太相关)           #3: Doc B (部分匹配)
  #4: Doc D (不相关)             #4: Doc F (关键词匹配)

RRF 融合计算 (k=60):
  Doc A: 1/(60+3) + 1/(60+1) = 1/63 + 1/61 = 0.0159 + 0.0164 = 0.0323
  Doc B: 1/(60+1) + 1/(60+3) = 1/61 + 1/63 = 0.0164 + 0.0159 = 0.0323
  Doc C: 1/(60+2) + 0         = 1/62 + 0       = 0.0161
  Doc D: 1/(60+4) + 1/(60+2) = 1/64 + 1/62   = 0.0156 + 0.0161 = 0.0317
  Doc F: 0         + 1/(60+4) = 0     + 1/64   = 0.0156

最终排序: [Doc A, Doc B] (并列) > Doc D > Doc C > Doc F
```

**关键洞察**：Doc A 和 Doc B 分数最高——因为**两路都命中了**（虽然各自排名不同）。这就是 RRF 最核心的价值：**两边都说相关的文档，比只有一边说相关的文档更可信**。

> **为什么 k=60？** k 越大，排名差异的影响越小，融合更平滑。k=60 是学术界（Cormack et al., 2009）和实践中的常用值，能有效防止单一路径的高排名文档"碾压"另一路径的结果。如果 k=0，排名 #1 的文档贡献 1，排名 #50 的贡献 0.02——差距 50 倍，太剧烈。

### 5.5 BM25 索引实现

```go
// bm25.go - BM25 核心

type BM25Index struct {
    mu       sync.RWMutex
    k1       float64    // 词频饱和度参数（默认 1.2）
    b        float64    // 长度归一化参数（默认 0.75）
    docs     []bm25Doc
    df       map[string]int  // 文档频率（Document Frequency）
    avgDL    float64         // 平均文档长度
    totalDoc int
}

func (idx *BM25Index) Search(query string, topK int) []BM25Hit {
    queryTokens := bm25Tokenize(query)

    for i, doc := range docs {
        // 对每个文档计算 BM25 分数
        for _, qt := range queryTokens {
            docFreq, ok := df[qt]
            if !ok || docFreq == 0 { continue }

            // IDF 计算
            idf := math.Log(1 + (float64(n)-float64(docFreq)+0.5)/(float64(docFreq)+0.5))

            // TF 归一化（BM25 核心公式）
            termFreq := float64(tf[qt])
            tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/avgDL))

            score += idf * tfNorm
        }
    }
    // 按分数降序返回 topK
}
```

**分词策略**（`bm25Tokenize`）：按 ASCII 字母/数字/`_`/`-`/`.`/`/`/`:` 切词，过滤英文停用词和长度 <2 的 token，全部小写化。

```go
var retrievalStopwords = map[string]struct{}{
    "a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "by": {},
    "for": {}, "from": {}, "in": {}, "into": {}, "is": {}, "of": {}, "on": {}, "or": {},
    "the": {}, "to": {}, "with": {}, "without": {},
    // 运维场景特有停用词
    "service": {}, "instance": {}, "type": {},
}
```

> **为什么 "service"、"instance"、"type" 是停用词？** 在运维文档中，这些词出现频率极高但区分度极低。几乎每篇文档都有 "service"，保留它只会增加噪音。

---

## 5.6. RAG 评测：Recall@10 = 78%

> 一个没有评测的系统是信仰驱动的。RAG 的评测回答一个核心问题：**检索回来的文档，是不是用户真正需要的那几篇？**

### 5.6.1 评测指标：Recall@K

```
Recall@K = (前 K 篇结果中命中 ground_truth 的数量) / (该案例所有 ground_truth 的总数)
```

例如：一个故障案例标注了 3 篇相关文档 `{A, B, C}`，检索返回 Top-10 里命中了 A 和 C → Recall@10 = 2/3 = 0.67。

当前针对 **AIOps Challenge 2025 的 50+ 个真实故障案例**评测，**Top-10 Recall = 78%**。

### 5.6.2 评测集划分：build/holdout 严格分离

```go
// eval/runner.go - 评测核心
func Run(ctx context.Context, cases []EvalCase, ks []int) ([]CaseResult, Summary) {
    for _, evalCase := range cases {
        // 对每个案例用 RAG 检索
        hits := retrieveHits(ctx, evalCase.Query)
        
        // 计算 Recall@K
        for _, k := range ks {
            recall := computeRecall(evalCase.RelevantIDs, hits[:k])
            result.RecallAtK[k] = recall
            summary.AvgRecallAtK[k] += recall
            if recall == 1.0 {
                summary.FullRecallAtK[k]++  // 计数"完美召回"的案例数
            }
        }
    }
    // 求平均
    summary.AvgRecallAtK[k] /= float64(len(cases))
}
```

| 评测原则 | 做法 |
|---------|------|
| **build split 和 eval split 严格分开** | 70% 案例做索引构建，30% 做评测——不自证效果 |
| **按案例切分** | 同一案例的文档不会同时出现在 build 和 eval 里——防止 LLM "背答案" |
| **标注 ground truth** | 每个案例由人工标注相关文档 ID 列表 |

### 5.6.3 78% 到 90% 的改进路径

| 改进方向 | 预期提升 | 成本 |
|---------|---------|------|
| **Chunking 策略优化**：语义分块替代固定 Markdown 标题切分 | +5-8% | 低（改代码） |
| **HyDE (Hypothetical Document Embeddings)**：先让 LLM 生成假设答案再检索 | +3-5% | 中（每次多一次 LLM 调用） |
| **Fine-tuned Embedding**：用 AIOps 领域的 query-document 对做对比学习微调 | +5-10% | 高（需要标注数据 + GPU 训练） |

---

## 5.7. MCP 协议集成 — 外部工具的标准化接入

> **MCP（Model Context Protocol）= LLM 的 "USB 协议"。** 不管是查日志、调 Prometheus、还是操作数据库——只要对方实现了 MCP 协议，Agent 就能像插 USB 设备一样即插即用。
> OpsCaption 使用 MCP 协议将本地 K8s 日志查询工具接入 Agent 工具链。

### 5.7.1 架构：MCP Server（Python） ↔ MCP Client（Go）

```
┌─────────────────────────────────────────────────────────┐
│                   OpsCaption Agent (Go)                  │
│                                                         │
│  ┌──────────────────────┐                               │
│  │   GetLogMcpTool()    │  ← MCP Client (Go)            │
│  │   mark3labs/mcp-go   │                               │
│  │   eino-ext/tool/mcp  │                               │
│  └──────────┬───────────┘                               │
│             │ SSE (Server-Sent Events)                  │
│             │ http://127.0.0.1:18088/sse                │
└─────────────┼───────────────────────────────────────────┘
              │
┌─────────────┼───────────────────────────────────────────┐
│             ▼            MCP Server (Python)             │
│  ┌──────────────────────────────────────┐               │
│  │  local-k8s-log-mcp.py                │               │
│  │                                      │               │
│  │  POST /message ← JSON-RPC 2.0        │               │
│  │    • tools/list         → 工具列表     │               │
│  │    • tools/call         → 执行工具     │               │
│  │                                      │               │
│  │  内部实现:                             │               │
│  │    kubectl get pods → kubectl logs    │               │
│  │    → 关键词过滤 → 日志分级             │               │
│  └──────────────────────────────────────┘               │
└─────────────────────────────────────────────────────────┘
```

### 5.7.2 MCP Client：Go 端实现

```go
// tools/query_log.go - GetLogMcpTool
func GetLogMcpTool() ([]tool.BaseTool, error) {
    // 1. 从 config.yaml 读取 MCP Server 地址
    mcpURL := g.Cfg().Get(ctx, "mcp.log_url").String()

    // 2. 创建 SSE MCP 客户端
    cli, _ := client.NewSSEMCPClient(mcpURL)
    cli.Start(ctx)

    // 3. MCP 协议握手初始化
    initRequest := mcp.InitializeRequest{}
    initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
    initRequest.Params.ClientInfo = mcp.Implementation{
        Name:    "superbizagent-client",
        Version: "1.0.0",
    }
    cli.Initialize(ctx, initRequest)

    // 4. 自动发现 MCP Server 提供的所有工具
    mcpTools, _ := e_mcp.GetTools(ctx, &e_mcp.Config{Cli: cli})
    return mcpTools, nil  // 返回的工具直接兼容 Eino tool 接口
}
```

**关键点**：`e_mcp.GetTools()` 将 MCP 协议的工具定义**自动转换**为 Eino 兼容的 `tool.BaseTool`——Agent 调用 MCP 工具和调用本地工具完全一致，无需关心底层协议。

### 5.7.3 MCP Server：Python SSE 实现

```python
# deploy/local-k8s-log-mcp.py
TOOL_NAME = "query_freeexchanged_k8s_logs"

def query_logs(arguments):
    query  = arguments.get("query")
    limit  = arguments.get("limit", 5)
    focus  = arguments.get("focus")        # OpsCaption Skills 注入的领域聚焦
    mode   = arguments.get("skill_mode")   # OpsCaption Skills 模式标识

    terms = log_terms(query + "\n" + focus)  # 从自然语言提取搜索关键词

    for pod in list_pods():                   # kubectl get pods
        output = run_kubectl(["logs", "-n", NAMESPACE, pod,
            "--all-containers=true",
            "--since=2h", "--tail=160"])
        
        for line in reversed(lines):
            if any(term in line.lower() for term in terms):
                records.append({"service": pod, "message": line, "level": infer_level(line)})
```

**MCP 协议握手**（`tools/list` → `tools/call`）：

```
1. Agent 启动 → GetLogMcpTool() → POST /message
   {"method": "tools/list"}
   
2. MCP Server → 返回工具定义
   {"tools": [{"name": "query_freeexchanged_k8s_logs", "inputSchema": {...}}]}
   
3. Agent 调用 → POST /message
   {"method": "tools/call", "params": {"name": "query_freeexchanged_k8s_logs",
    "arguments": {"query": "支付超时", "limit": 5, "focus": "payment timeout..."}}}
   
4. MCP Server → 返回结果
   {"content": [{"type": "text", "text": "{\"success\": true, \"logs\": [...]}"}]}
```

### 5.7.4 MCP + Skills 协作：两条链路的配合

```
用户 query: "checkoutservice 支付超时了"

① Skill 匹配
   FocusCollector.Collect("checkoutservice 支付超时了")
   → 匹配 logs_payment_timeout_trace skill
   → 提取 Focus: "payment, checkout, gateway timeout, retry, db timeout..."

② ProgressiveDisclosure 暴露工具
   Disclose(query) → matchDomains → ["logs"]
   → TierSkillGate 工具: MCP Log Tools (domain=logs) ✅ 暴露
   → Prometheus (domain=metrics) ❌ 不暴露

③ Agent 调用 MCP 工具
   invokeFocusedLogTool(query="checkoutservice 支付超时",
                        focus="payment, checkout, gateway timeout...",
                        mode="payment_timeout_trace")

④ MCP Server 返回结构化日志证据
   → 包含 payment-service 的超时日志、gateway 的 504 响应
   → Agent 综合证据 + RAG 文档 → 生成诊断报告
```

### 5.7.5 为什么用 MCP 而不是直接调 API？

| 方案 | 问题 |
|------|------|
| **硬编码 API 调用** | 每接一个新的外部系统就要写一套 Go 代码——Prometheus API、日志 API、数据库 API……每种协议都不一样 |
| **MCP 协议** | 统一 JSON-RPC 2.0 + SSE 传输 → 任何实现 MCP 的工具都能被自动发现和调用。新增一个外部工具只需写一个 Python MCP Server，Go 端零改动 |

**当前状态**：MCP 日志工具已部署为 systemd 服务（`opscaptain-log-mcp.service`），监听 `172.17.0.1:18088`，随 K3s 启动后自动拉起。

---

## 6. 关键设计点

### 6.1 全链路超时降级

```
┌─────────────┬─────────────┬──────────────────────┐
│   环节       │   超时       │   降级策略             │
├─────────────┼─────────────┼──────────────────────┤
│ Query Rewrite│   3s        │ 返回原始 query        │
│ Retriever 创建│  取决于Milvus│ 缓存失败，15s 内不重试  │
│ Milvus 检索  │   取决于Milvus│ 直接返回 error        │
│ Rerank       │   5s        │ 跳过 rerank，保持原序  │
│ 整体链路     │   无硬上限    │ 每个环节独立降级       │
└─────────────┴─────────────┴──────────────────────┘
```

**设计原则**：每个环节独立超时，互不影响。任何一个环节挂了，后续环节继续执行（带降级），而不是整条链路崩掉。

### 6.2 RetrieverPool 雪崩防护

前面已经详细讲过，核心思想：**失败缓存 + 短 TTL** = 穷人版 Circuit Breaker。15 秒内相同参数的请求不会重复尝试创建连接。

### 6.3 BM25 关键词互补

| 用户输入 | 向量检索 | BM25 检索 |
|---|---|---|
| "checkoutservice-v2-7d4f8b9c6-xqz9m" | ❌ Pod 名太长，embedding 稀释了语义信息 | ✅ 精确匹配，直接命中 |
| "服务响应变慢了" | ✅ 理解语义，匹配"latency increase" | ❌ 没有精确关键词，效果差 |
| "error code: 503 Service Unavailable" | ⚠️ 能匹配相关文档 | ✅ 精确命中 503 + Service Unavailable |

两者合在一起：语义覆盖 + 精确匹配 = **召回最大化**。

### 6.4 candidateTopK × 4 的粗排精排策略

```
topK = 3（最终返回给 LLM 的文档数）
candidateTopK = topK × 4 = 12 → clamp到 [20, 50] → 实际 = 20

流程：
  20 篇候选 → RetrieveRefine 重排 → 20 篇 → Rerank LLM 打分 → 取 top-3

为什么中间是 20 而不是 50？
  - 20 篇已经足够覆盖"遗漏的相关文档"
  - 给 LLM Rerank 20 篇文档的成本可控（≈几百 Token）
  - 50 篇会让 Rerank 的 Prompt 过长，增加超时风险
```

### 6.5 函数注入设计

```go
// query.go - queryWithMode 签名
func queryWithMode(
    ctx context.Context,
    pool *RetrieverPool,
    query string,
    mode QueryMode,
    rewrite rewriteFunc,    // ← 函数类型，可注入
    rerank rerankFunc,      // ← 函数类型，可注入
) ([]*schema.Document, QueryTrace, error)
```

`rewriteFunc` 和 `rerankFunc` 作为参数传入而非硬编码，这使得：
- **单元测试**时可以用 mock 函数替代真实 LLM 调用
- **未来扩展**时可以替换为不同的 rewrite/rerank 实现（如用本地模型做 rerank）

---

## 7. 面试问答

### Q1: "你的 RAG 是怎么做的？能讲一下完整链路吗？"

> 我们的 RAG 链路分**离线索引构建**和**在线检索**两大阶段。
>
> **离线阶段**，文档经过 Loader 统一加载后，走两阶段分块——先用 Markdown 标题切分保持文档结构，对大块（>800 字符）再按语义相似度补切。分块后用 Doubao Embedding 生成 1024 维向量写入 Milvus，同时构建 BM25 关键词倒排索引。同一文件重复索引时，会先做 _source 去重——删掉旧的 chunk 再写入新的。
>
> **在线检索**走六步。第一步 Query Rewrite——LLM 把口语 query 改写为搜索优化关键词，3 秒超时降级。第二步 RetrieverPool 获取连接——按 Milvus 地址缓存连接，失败走 15 秒短 TTL 防雪崩。第三步 Milvus ANN 向量检索——取 candidateTopK（topK × 4）篇候选。第四步 RetrieveRefine——基于 query token 与文档元数据做交叉打分，服务名/Pod 名匹配权重最高（4,12）。第五步 Rerank——LLM 对每篇打 0-10 分，5 秒超时降级。第六步返回文档 + QueryTrace。
>
> **Hybrid 模式**额外增加 BM25 并行检索——向量 + BM25 两路用 RRF 公式融合（score = Σ 1/(60+rank)），让两边都命中的文档排最前面。当前在 AIOps Challenge 2025 案例集上评测，**Recall@10 = 78%**。

### Q2: "做了哪些优化？"

> 主要有五个优化点：
>
> **第一，两阶段分块策略。** 先用 Markdown 标题保持结构边界，对大块（>800 字符）再用语义相似度补切——从 50% 提到 78% Recall。这是性价比最高的优化。
>
> **第二，连接池 + 雪崩防护。** RetrieverPool 按缓存 key 复用连接，失败时缓存错误 15 秒，避免 Milvus 不可用时每个请求都超时等待。这本质上是 Circuit Breaker 的简化实现。
>
> **第三，全链路超时降级。** 每个环节独立超时——改写 3s、Rerank 5s。任何环节超时或失败，自动降级到下一级策略：改写失败用原始 query，Rerank 失败跳过直接返回 refine 后的顺序。用户无感知。
>
> **第四，粗排→精排两阶段检索。** 先用向量检索取 4 倍候选（candidateTopK），再用 RetrieveRefine 做结构化打分，最后用 LLM Rerank 精排。既保证了召回率，又控制了精排成本。
>
> **第五，Hybrid 检索 + BM25 并行。** BM25 关键词检索弥补向量检索的精确匹配短板——Pod 名、error code、IP 地址等精确标识符用 BM25 直接命中。两路并行执行后用 RRF 公式融合（score = Σ 1/(60+rank)），消除了量纲差异。另外离线阶段同步构建 BM25 索引并做 _source 去重——同一文件重复索引不会残留旧 chunk。

### Q3: "为什么要加 BM25？向量检索不是够了吗？"

> 向量检索确实在大多数场景效果很好，但在运维领域有明确的短板——**精确术语匹配**。
>
> 举个例子：用户输入 `checkoutservice-v2-7d4f8b9c6-xqz9m`，这是一个很长的 K8s Pod 名。向量模型把它 embedding 后，很长的字符串会稀释关键信息，导致检索效果下降。但 BM25 做精确关键词匹配，能直接命中包含这个 Pod 名的文档。
>
> 再比如 `error code: 503`、`IP: 10.0.1.25`、`trace_id: abc123`，这些都是运维场景中高价值的精确标识符。向量检索可能匹配到"类似"的 500 错误文档，而 BM25 能精确命中 503 的文档。
>
> 所以我们的设计是 **两者互补**：向量检索覆盖语义（"服务挂了"→"pod failure"），BM25 覆盖精确匹配（pod name、error code、IP）。BM25 的核心是 IDF × TF_norm——稀有词权重高，出现次数有饱和度上限，长文档有长度惩罚。然后用 RRF 算法做分数融合：两边都命中的文档排最前面——这就是为什么融合后排第一的文档往往是两个方向交叉验证过的。当前 Hybrid 模式在 AIOps Challenge 2025 案例集上 **Recall@10 = 78%**。

### Q4: "你的 Agent 怎么知道该调用哪些工具？工具多了不会让 LLM 选错吗？"

> 这正是我们 **ProgressiveDisclosure（渐进暴露）** 解决的问题。
>
> 我们把工具分成三层：**TierAlwaysOn**（始终暴露——如时间查询、文档检索，不到 5 个）、**TierSkillGate**（按场景暴露——日志 MCP 工具只在匹配到 logs 域时才暴露，Prometheus 工具只在 metrics 域暴露）、**TierOnDemand**（手动扩展——如 MySQL CRUD，需要配置白名单才暴露）。
>
> 每次用户 query 进来，先用 **Skills Registry** 做域匹配——"支付超时" 匹配到 logs 域，"CPU 飙升" 匹配到 metrics 域。然后只暴露对应域的工具。这样 LLM 看到的工具从 10+ 个缩减到 3-4 个，选错的概率大幅下降，token 消耗也减少了。
>
> 另外我们还有 **FocusCollector**——匹配到 Skill 后会提取该 Skill 的 Focus 指令（如 "重点看支付、订单、网关超时、下游延迟相关日志"），注入到 LLM 的上下文中。相当于给 LLM 提前画好重点——不需要它从头推理该查什么。

### Q5: "你的 MCP 集成是怎么做的？为什么选 MCP 而不是直接调 API？"

> MCP（Model Context Protocol）本质上是 LLM 和外部工具之间的标准协议——类似 USB 对硬件外设的作用。
>
> 我们用 Go 端（`mark3labs/mcp-go` + `eino-ext/tool/mcp`）做 MCP Client，Python 端（`local-k8s-log-mcp.py`）做 MCP Server。Go 端通过 SSE 连接 Python Server，自动完成 `tools/list` 发现工具 → `tools/call` 执行调用。
>
> 选 MCP 的核心原因：**标准化**。如果有新的外部系统要接入（比如一个新的监控平台），只需写一个实现 MCP 协议的 Server——可以是 Python、Node、Rust 任何语言。Go 端的 Agent 代码完全不用改，`e_mcp.GetTools()` 会自动发现新工具。如果不用 MCP 而是硬编码 API，每接一个外部系统就要写一套 HTTP client + 参数映射 + 错误处理——工作量是 MCP 的 3-5 倍。
>
> 当前 MCP 日志工具已部署为 systemd 服务，随 K3s 启动后自动拉起，监听 `172.17.0.1:18088`。通过 config.yaml 的 `mcp.log_url` 配置地址，可以随时切换本地/远程 MCP Server。

---

## 8. 自测

### 问题 1

RAG 的 "full" 模式包含哪几个步骤？每个步骤的作用是什么？

<details>
<summary>点击查看答案</summary>

**六步**：
1. **Query Rewrite**：把口语化查询改写为搜索优化关键词
2. **RetrieverPool.GetOrCreate**：获取/创建 Milvus 连接（带缓存和雪崩防护）
3. **Milvus ANN 检索**：向量相似度搜索，返回 candidateTopK 篇候选
4. **RetrieveRefine**：基于 token 与元数据交叉打分，重新排序
5. **Rerank**：LLM 对每篇文档打 0-10 分，按分降序取 top-K
6. **返回结果 + QueryTrace**：返回最终文档和全链路追踪信息
</details>

### 问题 2

RetrieverPool 的雪崩防护是如何实现的？Milvus 宕机后会发生什么？

<details>
<summary>点击查看答案</summary>

**实现机制**：
- 创建失败时，将错误 + 时间戳缓存到 `cachedRetriever.lastErr` 和 `failedAt`
- 后续相同 cacheKey 的请求，如果在 TTL（默认 15s）内，直接返回缓存的错误，**不重新尝试连接**
- TTL 过期后，下一次请求重新尝试创建连接

**Milvus 宕机后的行为**：
1. 第一个请求：尝试创建连接 → 超时失败 → 缓存错误（TTL=15s）
2. 后续 15s 内所有请求：命中失败缓存 → 立即返回错误（不再等待超时）
3. 15s 后：重新尝试 → 如果 Milvus 恢复了，创建新连接并缓存；如果还没恢复，重新缓存错误

这本质上是 Circuit Breaker 的简化实现。
</details>

### 问题 3

Hybrid 检索中，向量检索和 BM25 检索的结果是如何融合的？RRF 的 k=60 有什么作用？

<details>
<summary>点击查看答案</summary>

**融合方法：RRF（Reciprocal Rank Fusion）**

公式：`RRF_score(D) = Σ 1/(k + rank_i)`

- `k = 60`，是平滑常数
- `rank_i` 是文档 D 在第 i 个检索列表中的排名

两个检索结果按 ID 合并，分别贡献 RRF 分数，最后按总分降序排列。

**k=60 的作用**：
- k 越大，排名差异对分数的影响越小，融合更平滑
- 防止高排名文档"碾压"另一路径的结果
  - 例如：向量排第 1（得分 1/61=0.0164），BM25 排第 10（得分 1/70=0.0143），差异不大
  - 两个方向都命中的文档的得分会更高（累加），更可信
- k=60 是学术界的常用值，实践效果好
</details>

---

> **下一章预告**：ContextEngine — 上下文装配引擎，如何把历史/记忆/文档/工具输出高效装配到 LLM 的上下文窗口中。
