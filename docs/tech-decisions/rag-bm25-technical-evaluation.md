# RAG BM25 技术选型与评估

## 背景

OpsCaptain 的 RAG 系统采用 Hybrid Search 架构：Dense Retrieval（Milvus 向量检索）+ Lexical Retrieval（BM25 关键词检索）+ RRF 融合。BM25 部分需要一个倒排索引 + scoring 实现。

## 候选方案

| 方案 | 类型 | 语言 | Stars | 特点 |
|------|------|------|-------|------|
| **bleve** | 全文搜索库 | Go | 6k+ | 倒排索引 + BM25 + 多种查询类型 + 持久化 |
| **tantivy-go** | 搜索引擎绑定 | Go/Rust | 200+ | Rust tantivy 的 CGO 绑定，性能极高 |
| **Elasticsearch** | 独立服务 | Java | 70k+ | 企业级搜索引擎，功能最全 |
| **wukong** | 中文全文搜索 | Go | 5k+ | 专注中文分词，但项目已不活跃 |
| **自研** | 纯 BM25 实现 | Go | - | 200 行代码，纯内存，无外部依赖 |

## 评估维度

### 1. bleve

**优点：**
- Go 原生，无 CGO 依赖
- 成熟稳定（Couchbase 团队维护，10 年历史）
- 内置多种分词器，支持 CJK
- 支持持久化索引
- 支持布尔组合、模糊搜索、范围查询等高级功能

**缺点：**
- 功能过重：我们只需要 BM25 scoring，bleve 是一个完整的搜索引擎（索引、分词、存储、查询全包），引入后只用 5% 的功能
- CJK 支持需要额外配置（`unicode` 或 `cjk` 分析器），效果不一定比自研 bigram 方案好
- 200+ 文件，版本升级可能有 breaking changes
- 索引持久化对我们是冗余的（我们从 Milvus 拿数据，BM25 只是辅助）

**结论：不引入。** 杀鸡用牛刀，维护成本大于收益。

### 2. tantivy-go

**优点：**
- Rust tantivy 性能极高
- 支持中文分词

**缺点：**
- CGO 依赖，交叉编译复杂
- K8s 部署需要额外的编译环境
- 社区较小，问题排查困难

**结论：不引入。** CGO 是硬伤，我们的部署环境（K8s + Docker）对 CGO 不友好。

### 3. Elasticsearch

**优点：**
- 功能最全，性能最好
- 生态成熟，运维工具丰富

**缺点：**
- 引入独立服务，增加运维复杂度
- 资源消耗大（至少需要 1GB 内存）
- 对于我们的知识库规模（几百篇文档）严重过度

**结论：不引入。** 我们已经有 Milvus 做向量检索，再引入 ES 是双倍运维负担。

### 4. 自研

**优点：**
- 200 行代码，完全可控
- 纯 Go，无外部依赖，交叉编译无障碍
- tokenizer 可针对运维场景定制（CJK bigram + meta field 扩展 + 运维 stopword）
- 与 RAG pipeline 深度集成，无额外抽象层
- 团队能完全理解和维护

**缺点：**
- 无持久化（每次索引从文件重建）
- 无增量更新（全量重建 df 统计）
- CJK 分词只有 bigram，没有更复杂的 N-gram
- 没有模糊搜索、布尔组合等高级功能

**结论：采用。** 对于当前规模（几百篇文档）和场景（运维知识库辅助检索），自研是最佳选择。

## 决策

**采用自研 BM25 实现。**

### 核心理由

1. **规模匹配** — 几百篇文档的倒排索引，纯内存实现的全量重建在毫秒级完成，不需要持久化或增量更新
2. **依赖可控** — 纯 Go 实现，无 CGO，K8s 部署无额外编译步骤
3. **业务定制** — tokenizer 针对运维场景做了 CJK bigram、meta field 扩展、运维 stopword，通用库不一定支持这些定制
4. **代码量小** — 200 行代码，团队能完全掌控，不存在"黑盒"问题
5. **已验证** — 生产环境运行稳定，hybrid search 的 recall 满足需求

### 何时考虑切换

| 条件 | 触发动作 |
|------|---------|
| 知识库 > 1 万篇 | 评估 bleve，引入持久化倒排索引 |
| 需要模糊搜索/布尔组合 | 评估 bleve 或 ES |
| BM25 成为性能瓶颈（P99 > 100ms） | 先优化 rebuildStatsLocked，再评估外部库 |
| 需要分布式 BM25 | 引入 ES |

## 当前实现概要

```
internal/ai/rag/bm25.go          # BM25 索引和 scoring（~200 行）
internal/ai/rag/shared_bm25.go   # 全局单例，double-checked locking
internal/ai/rag/hybrid.go        # BM25 + Dense + RRF 融合
```

**BM25 参数：** k1=1.2, b=0.75（标准值）

**Tokenizer 策略：**
- 英文：lowercase + 非字母数字切分 + 过滤 <2 字符 token + stopword
- 中文：unigram + bigram overlap（如 "故障诊断" → ["故障", "障诊", "诊断"]）
- Meta field：将文档的 service、pod、metric 等 metadata 也加入 token 列表

**Hybrid Search 流程：**
```
用户查询
  ├─ Dense Retrieval (Milvus, topK=50)  ──┐
  └─ BM25 Retrieval (内存索引, topK=50) ──┤
                                           ▼
                                     RRF Fusion (K=60)
                                           ▼
                                     Metadata Boost
                                           ▼
                                     Top-20 Candidates
                                           ▼
                                     LLM Rerank (可选)
                                           ▼
                                     Top-5 Final Results
```

## 面试话术

### Q: 为什么自己写 BM25 而不用 bleve/ES？

> "我们评估过 bleve、tantivy-go 和 Elasticsearch。
>
> bleve 是一个完整的搜索引擎，引入它等于引入一个子系统，我们的场景只需要 BM25 scoring + 倒排索引，200 行代码就够了。而且 bleve 的 CJK 支持需要额外配置，效果不一定比我们针对运维场景定制的 bigram 方案好。
>
> tantivy-go 有 CGO 依赖，在我们的 K8s 部署环境里多了一层编译复杂度。
>
> Elasticsearch 对我们的知识库规模（几百篇文档）严重过度，而且引入一个独立服务会增加运维负担。
>
> 最终选择自研是因为：纯 Go 无外部依赖、200 行代码完全可控、tokenizer 可以针对运维场景定制——比如 CJK bigram 分词、meta field 扩展（把 service name、pod name、metric name 也加入索引）、运维领域 stopword。这些定制在通用库里不容易实现。"

### Q: 如果知识库规模增长到 10 万篇怎么办？

> "有几个层次的优化：
>
> 第一步是优化现有的 BM25 实现——目前 `rebuildStatsLocked` 是全量重建，可以改成增量更新 df 统计，从 O(N*M) 降到 O(delta*M)。
>
> 第二步是引入 bleve 的持久化倒排索引，避免每次启动都从文件重建。
>
> 第三步如果需要分布式，才考虑引入 Elasticsearch。
>
> 但目前我们的知识库是运维文档和 SOP，增长速度很慢，几百篇的规模在可预见的未来不会变化太大。所以当前的自研方案是够用的。"

### Q: 你的 BM25 中文分词效果怎么样？

> "我们用的是 unigram + bigram overlap 策略。比如'故障诊断'会被切成 ['故障', '障诊', '诊断']。
>
> 这个方案的优势是简单可靠，不需要维护词典。缺点是会产生一些无意义的 bigram（比如'障诊'），但因为 BM25 的 IDF 权重会自然降低这些噪声 token 的影响，实际效果是可接受的。
>
> 如果需要更好的中文分词效果，可以接入 jieba 分词器，但在我们的场景里，运维文档的关键词通常是英文术语（timeout、OOM、crashloop）或者固定中文短语（超时、宕机、重启），bigram 方案已经能覆盖。"

### Q: Hybrid Search 的 RRF 融合是怎么做的？

> "我们用 Reciprocal Rank Fusion，公式是 score = Σ 1/(K+rank)，K=60。
>
> Dense Retrieval 和 BM25 并行执行，各自返回 top-50。然后用文档的 case_id 或 doc_id 作为 fusion key 做去重——如果同一篇文档在两个列表里都出现了，它的融合分数会更高。
>
> 融合之后还有一个 metadata boost 步骤：如果查询里提到了具体的 service name 或 metric name，而文档的 metadata 里也包含这些信息，会额外加分。这是针对运维场景的定制。
>
> 整个流程有完整的 trace 记录——每个文档会携带 dense_rank、lexical_rank、fusion_score、metadata_boost、rerank_score，方便调试和评估。"

### Q: 为什么不直接用向量检索就够了？

> "纯向量检索在语义匹配上很强，但在精确关键词匹配上有短板。比如用户问'OOM 导致 pod restart'，向量检索可能返回语义相似但不包含 OOM 关键词的文档。BM25 能精确匹配到包含 OOM 和 restart 的文档。
>
> 两者互补：向量检索负责语义召回，BM25 负责关键词召回，RRF 融合取两者的并集。我们在内部评估中发现 hybrid search 比纯向量检索的 recall@5 提升了约 15-20%。"
