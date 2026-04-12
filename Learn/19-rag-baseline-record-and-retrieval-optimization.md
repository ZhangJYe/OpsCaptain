# 19-RAG 基线记录与召回优化（2026-04-12）

## 1. 背景

当前项目已经完成 `aiopschallenge2025` 的 telemetry evidence 预处理，并在云端按同一口径跑通了 `retrieve / rewrite / full` 三种评测模式。现在要先把真实基线固定下来，再针对真正的瓶颈做优化。

这一步的目标不是继续调 prompt，也不是继续追 `rerank`，而是把“当前系统到底卡在哪一层”说清楚。

## 2. 基线口径

本次基线使用的是严格版 holdout 评测口径：

- 只索引 `build split`
- 只评估 `holdout` query
- `RelevantIDs` 是 build split 里的相关历史案例，不是 holdout 自身
- telemetry 语料来自本地重预处理后的 `docs_evidence_telemetry_build`

评测文件：

- `aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl`

collection：

- `aiops_evidence_telemetry_build_resume_20260412`

## 3. 当前真实基线

telemetry evidence 基线结果如下：

| mode | Avg Total ms | Hit@1 | Hit@3 | Hit@5 | AvgRecall@1 | AvgRecall@3 | AvgRecall@5 |
|---|---:|---:|---:|---:|---:|---:|---:|
| `retrieve` | `43.64` | `0.20625` | `0.3625` | `0.3875` | `0.0110` | `0.0210` | `0.0225` |
| `rewrite` | `3051.20` | `0.20625` | `0.3625` | `0.3875` | `0.0110` | `0.0210` | `0.0225` |
| `full` | `2380.78` | `0.20625` | `0.3625` | `0.3875` | `0.0110` | `0.0210` | `0.0225` |

## 4. 这组基线说明了什么

结论很硬：

1. `retrieve / rewrite / full` 三组效果完全一样。
2. `rewrite` 和 `full` 只增加了延迟，没有增加任何命中率或召回率。
3. 当前瓶颈不在 `LLM rewrite`，也不在 `rerank`，而在检索层本身。
4. 当前系统的主要问题不是“查不到文档”，而是“前 5 个结果里相关案例比例仍然偏低”。

补充判断：

- `Empty Rate = 0`，说明检索链路是通的。
- `Cache Hit` 很高，说明缓存和基础设施没坏。
- `Hit@5 = 0.3875` 说明前 5 个结果里只有约 39% 的 query 能碰到至少 1 个相关案例。
- `AvgRecall@5 = 0.0225` 说明相关集合覆盖率很低。

因此，下一步不应该继续把时间花在 `rewrite/rerank` 上，而应该优先提升候选召回质量。

## 5. 为什么这次优化要改召回层

原始检索链路有两个问题：

1. Milvus 只按配置直接取 `top_k`
2. 取回结果后没有利用 telemetry sidecar metadata 做二次整理

对于当前 telemetry 文档，这两个问题会直接限制效果：

- 向量检索只拿很少的候选，容易把真正相关但初始相似度稍低的 case 截掉
- telemetry doc 已经包含 `service / instance_type / source / destination / metric_names / trace_operations` 等信息，但原始链路完全没利用这些结构化线索

所以这次优化采用的思路是：

- 先过召回更多候选
- 再用轻量 metadata + lexical overlap 做本地重排
- 最后再切回最终 `top_k`

这一步不依赖 LLM，因此不会引入额外的 429 风险。

## 6. 本次代码优化内容

本次修改落在 `internal/ai/rag/`：

1. 增加候选召回数量
   - 新增 `RetrieverCandidateTopK()`
   - 默认策略：最终 `top_k` 的 4 倍，最少 `20`，最多 `50`

2. 增加本地轻量重排器
   - 新增 `retrieve_refine.go`
   - 评分依据：
     - query 与文档正文的词面重合
     - query 与 `service_tokens / pod_tokens / node_tokens / namespace_tokens` 的重合
     - query 与 `metric_names / trace_services / trace_operations` 的重合
     - `service / instance_type / source / destination` 的显式命中

3. 调整 `QueryWithMode()` 行为
   - retrieve 阶段不再只拿最终 `top_k`
   - 先用扩大的 `candidate_top_k` 做检索
   - 本地重排后：
     - `retrieve / rewrite` 模式裁回最终 `top_k`
     - `full` 模式把重排后的候选交给 rerank

## 7. 第一次远端复跑为什么没有提升

第一次把这套逻辑直接跑到云端旧 collection 上时，结果完全没变化：

- `Hit@1/3/5` 不变
- `AvgRecall@1/3/5` 不变
- 只是延迟从 `43.64ms` 上升到 `58.49ms`

原因后来定位清楚了：**旧 telemetry collection 根本没有 sidecar metadata**。

远端实际取回的文档 `MetaData` 只有：

- `_source`
- `_file_name`
- `title`
- `subtitle`

没有：

- `case_id`
- `service`
- `instance_type`
- `metric_names`
- `trace_operations`

这意味着我新增的 metadata 重排逻辑在旧 collection 上没有任何输入，自然不会产生效果。

这个定位很关键，因为它说明当“优化无效”时，先要确认数据面是不是具备了算法需要的特征，而不是盲目继续调参数。

## 8. 修正动作：重建带 metadata 的 telemetry collection

确认问题后，修正动作不是继续改排序权重，而是先重建 collection：

- 重新使用带 `metadataSidecarLoader` 的最新索引链路
- 将 `docs_evidence_telemetry_build/*.metadata.json` 真正写进 Milvus `metadata` 字段
- 在新 collection 上再跑同口径 `retrieve` 评测

新 collection：

- `aiops_evidence_telemetry_build_meta_opt1_20260412`

重建后，远端抽样验证的文档 `MetaData` 已经包含：

- `case_id`
- `doc_id`
- `service`
- `instance_type`
- `metric_names`
- `trace_services`
- `trace_operations`
- `service_tokens / pod_tokens / node_tokens / namespace_tokens`

## 9. 修正后的优化结果

在真正带 metadata 的新 collection 上，`retrieve` 结果变成：

| variant | Avg Total ms | Hit@1 | Hit@3 | Hit@5 | AvgRecall@1 | AvgRecall@3 | AvgRecall@5 |
|---|---:|---:|---:|---:|---:|---:|---:|
| `baseline_retrieve` | `43.64` | `0.20625` | `0.3625` | `0.3875` | `0.0110` | `0.02095` | `0.02249` |
| `metadata_rerank` | `47.69` | `0.19375` | `0.39375` | `0.39375` | `0.00946` | `0.02325` | `0.02359` |

这说明：

1. 优化不是完全无效，真正生效后在 `@3/@5` 上有小幅提升。
2. `Hit@1` 和 `Recall@1` 下降，说明当前重排策略更像“把相关案例从深层拉进 top5”，但还不够擅长抢第一。
3. 这一步已经证明 sidecar metadata 不是装饰字段，确实能给检索带来收益。

我还额外测试了把 `candidate_top_k` 提到 `100`，结果与 `20` 基本一致，说明当前瓶颈已经不是“候选池太浅”，而是更深层的召回策略本身。

## 10. 这次怎么验证的

本地验证：

```powershell
go test ./internal/ai/rag
go test ./internal/ai/contextengine ./internal/ai/cmd/rag_online_eval_cmd
```

新增覆盖点：

- `retrieve-only` 确认会请求扩大的候选 `topK`
- 本地重排器确认会把 metadata 和词面更匹配的 telemetry doc 排到前面

云端验证：

- 先在旧 collection 上复跑，确认“无变化”
- 再检查远端文档 metadata，定位旧 collection 缺少 sidecar metadata
- 重建带 metadata 的新 collection
- 在新 collection 上继续跑 `retrieve` 模式进行同口径对比

## 11. 风险与边界

这次优化有几个边界要明确：

1. 这不是 `hybrid retrieval`
   - 还没有真正引入 BM25 或倒排检索

2. 这不是 metadata hard filter
   - 这里只做 soft rerank，不做硬过滤
   - 原因是当前 holdout 相关案例不一定来自同一个 service，硬过滤会伤 recall

3. 这不是最终形态
   - 如果这次优化仍然提升有限，下一步应该进入：
     - hybrid retrieval
     - metadata-aware prefilter
     - 更细粒度的 failure breakdown

## 12. 面对评审员应该怎么讲

可以直接这样讲：

“我先把 telemetry RAG 的基线拆成 `retrieve / rewrite / full` 三组，结果发现三组效果完全一致，但 `rewrite/full` 延迟高了几十倍。这说明当前瓶颈不在 LLM，而在召回层本身。随后我做了过召回和 metadata 重排，但第一次线上结果完全没变。我没有继续盲调，而是去检查远端 collection，发现旧索引里根本没把 sidecar metadata 写进去。修正方式不是继续改排序公式，而是先重建带 metadata 的 collection，再重新评测。重建后 `Hit@3/@5` 和 `Recall@3/@5` 才开始出现真实提升。”

## 13. 我作为项目负责人应该学会什么

这一步最重要的不是“多写一个重排器”，而是学会下面三件事：

1. 不要把 `full` 路径当作默认真相，要先拆模式看基线。
2. 当 `rewrite/rerank` 没有收益时，应该回到召回层，而不是继续堆 LLM 技巧。
3. telemetry 文档一旦有结构化 sidecar metadata，就应该尽早把这些信息转化为检索增益，而且要确认这些字段真的被索引进线上 collection，而不是停留在本地文件里。
