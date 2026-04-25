# OpsCaptionAI 项目基线测评 STAR 复盘

> 适用场景：技术面试、项目复盘、阶段汇报。  
> 核心目标：把项目里的各类 baseline 讲成“有背景、有任务、有动作、有结果”的工程闭环，而不是只背一组指标。

## 0. 讲述边界

当前面试主线建议聚焦：

1. 工程可运行基线
2. RAG smoke baseline
3. AIOps telemetry RAG baseline
4. 线上服务 smoke baseline
5. 评测口径修正和下一步优化路径

不建议主动展开复杂 Multi-Agent 协作设计。当前更稳的讲法是：

> 项目早期探索过更复杂的协作式 Agent Runtime，但当前阶段我把重点收敛到 Chat、RAG、Context、Memory、工具治理和基线评测。面试里我更愿意讲已经跑通、可验证、能复盘的数据闭环。

## 1. 基线总览

| 基线类型 | 目的 | 评测口径 / 命令 | 当前结果 | 面试结论 |
| --- | --- | --- | --- | --- |
| 工程构建基线 | 证明项目能稳定构建和回归 | `GOTOOLCHAIN=go1.24.4 go test ./...`、`go build ./...`、`git diff --check` | 通过 | 项目不是只靠手工跑 demo，有基础自动化兜底 |
| RAG smoke baseline | 验证离线检索 harness 可复现 | `GOTOOLCHAIN=go1.24.4 go run ./internal/ai/cmd/rag_eval_cmd` | 17 docs / 20 cases；AvgRecall@1 0.82，@3 0.95，@5 0.97；HitRate@5 1.00 | 小样本基线达标，用于保护基础检索能力 |
| AIOps 早期严格 baseline | 验证 build-only holdout 泛化 | `eval_cases_holdout_related.jsonl` + build collection | evidence_build AvgRecall@5 0.15844，HitRate@5 0.71875；history_build AvgRecall@5 0.15816，HitRate@5 0.71250 | 能命中方向，但覆盖率低，历史标签未带来显著增益 |
| Telemetry 严格 Milvus baseline | 拆分 retrieve / rewrite / full | `rag_online_eval_cmd -mode retrieve/rewrite/full` | 三种模式 Hit/Recall 几乎一致；retrieve 43.64ms，rewrite 3051.20ms，full 2380.78ms | 瓶颈不在 LLM rewrite/rerank，而在召回层 |
| Metadata rerank baseline | 验证 sidecar metadata 是否有收益 | 重建带 metadata 的 collection 后复跑 retrieve | Hit@3 0.3625 -> 0.39375；Hit@5 0.3875 -> 0.39375；AvgTotal 43.64ms -> 47.69ms | metadata 确实有轻微收益，但还不足以解决召回覆盖问题 |
| 本地 BM25 archive baseline | 在无 Milvus / 无完整数据集时快速建立轻量对照 | archive build docs 320 篇 + 640 条可评测 case | Hit@1 0.0969，@3 0.2484，@5 0.3609，@10 0.5266；MRR 0.2290 | 关键词基线可做离线 sanity check，但不能替代严格 holdout |
| 线上 smoke baseline | 验证服务器存活和关键入口 | `124.222.57.178` HTTP 探测 | `/ai/`、`/ai/healthz`、`/ai/readyz`、Prometheus、Jaeger 可用；async chat 可提交；sync chat 5s 超时；RAG docs 受 Milvus/default DB 问题影响 | 服务基础可用，但 RAG 在线依赖仍需治理 |

## 2. STAR 一：工程构建基线

### Situation

OpsCaptionAI 是一个 Go 后端项目，包含 Chat、AIOps、RAG、上下文工程、记忆系统、上传索引、可观测性等多个模块。项目越往后走，风险不是“单个函数能不能跑”，而是改动后全仓还能不能稳定构建、核心模块还能不能回归。

另一个现实问题是本地默认 Go 工具链曾经触发第三方依赖兼容问题，所以必须先固定可复现的工具链口径。

### Task

我要建立一条最基础的工程 baseline：

1. 全仓测试能通过
2. 全仓构建能通过
3. 文档和代码改动没有格式级错误
4. 明确当前项目应该使用 Go 1.24.x 工具链，而不是让本地默认工具链漂移

### Action

我使用固定工具链执行：

```bash
GOTOOLCHAIN=go1.24.4 go test ./...
GOTOOLCHAIN=go1.24.4 go build ./...
git diff --check
```

在针对最近的修复回归时，也补充跑过更聚焦的模块测试：

```bash
GOTOOLCHAIN=go1.24.4 go test ./internal/ai/service ./internal/controller/chat ./utility/auth ./utility/mem
```

### Result

当前工程基线是通过的：

- `go test ./...` 通过
- `go build ./...` 通过
- `git diff --check` 通过
- 需要固定 `GOTOOLCHAIN=go1.24.4`

面试讲法：

> 我不会只说“项目能跑”，而是先把工程 baseline 固定住。当前仓库在 Go 1.24.4 口径下可以全仓测试、全仓构建通过。这个基线的价值是后续做 RAG、记忆、文件上传或服务配置改动时，可以先确认没有破坏整体工程可运行性。

## 3. STAR 二：RAG Smoke Baseline

### Situation

RAG 是项目里最容易被问的部分。只说“我接了 Milvus 和 Embedding”不够，因为面试官会追问：检索效果怎么衡量、怎么知道优化有没有收益、有没有 baseline。

所以项目里先做了一套小样本、可复现的 RAG recall harness，用来保护基础检索链路。

### Task

我要先建立一个低成本 smoke baseline：

1. 本地不用依赖 Milvus
2. 不依赖外部 LLM
3. 能快速跑出 Recall@K、HitRate@K、FullRecall
4. 能暴露“召回不到”和“召回到了但排序不稳”的差异

### Action

执行命令：

```bash
GOTOOLCHAIN=go1.24.4 go run ./internal/ai/cmd/rag_eval_cmd
```

本次口径：

- Searcher：InMemory lexical
- Corpus：17 documents
- Cases：20
- K：1、3、5

### Result

当前 smoke baseline：

| Metric | @1 | @3 | @5 |
| --- | ---: | ---: | ---: |
| Avg Recall | 0.82 | 0.95 | 0.97 |
| Hit Rate | 0.95 | 0.95 | 1.00 |
| Full Recall | 14/20 | 19/20 | 19/20 |

失败分析：

| 类型 | 数量 |
| --- | ---: |
| PASS | 19/20 |
| PARTIAL | 1/20 |
| MISS | 0/20 |

唯一明显短板是 `RAG-15`：生产环境数据库事务超时。这说明基础样例里不是完全召回失败，而是模糊表达下排序和相关文档覆盖还需要继续加强。

面试讲法：

> 我先做了一个不依赖线上组件的 RAG smoke baseline。它不是最终效果指标，而是保护基础检索链路的 sanity check。当前 20 条样例里 HitRate@5 是 1.00，AvgRecall@5 是 0.97，说明基础检索链路可用；但 RAG-15 这种模糊数据库事务超时问题只做到部分召回，提示后续还要强化同义表达和排序。

## 4. STAR 三：AIOps 早期严格 Baseline

### Situation

AIOps 场景不能只靠小样本 smoke test。真实挑战在于：用户输入是故障症状，系统要从历史案例或遥测证据里找到相似案例。早期我做过两套 build-only holdout 评测：

1. `evidence_build`：看症状和观测证据本身能召回多少
2. `history_build`：看加入历史标签知识后是否更好

### Task

我要回答两个问题：

1. 系统在严格 holdout 下是否真的能泛化
2. 历史标签知识是否比 evidence 文档更有帮助

### Action

使用 `eval_cases_holdout_related.jsonl` 作为严格评测集，只索引 build split，不把 holdout 自己放进索引，分别跑：

- `report_evidence_build_related.json`
- `report_history_build_related.json`

### Result

严格结果如下：

| Collection | Cases | AvgTotal | Hit@1 | Hit@3 | Hit@5 | AvgRecall@1 | AvgRecall@3 | AvgRecall@5 | FullRecall@5 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| evidence_build | 160 | 2932.31ms | 0.61875 | 0.70000 | 0.71875 | 0.03572 | 0.10640 | 0.15844 | 0/160 |
| history_build | 160 | 720.96ms | 0.61875 | 0.69375 | 0.71250 | 0.03572 | 0.10611 | 0.15816 | 0/160 |

这组结果说明：

- `HitRate@5` 约 0.71，说明 top5 经常能碰到至少一个相关案例
- `AvgRecall@5` 只有约 0.158，说明相关集合覆盖率很低
- `history_build` 相比 `evidence_build` 几乎没有提升

面试讲法：

> 这轮 baseline 让我发现一个关键事实：系统不是完全找不到方向，Hit@5 能到 0.71 左右；但相关集合覆盖率很低，AvgRecall@5 只有 0.158 左右。同时 history_build 没有明显优于 evidence_build，说明当时系统还没有真正利用好历史标签知识，只是在做症状相似召回。

## 5. STAR 四：Telemetry 严格 Milvus Baseline

### Situation

前面的 baseline 暴露了覆盖率低的问题，但还不能判断问题到底在哪一层。RAG 链路里有 query rewrite、retriever、rerank，如果只跑 full 模式，低分时无法判断是召回差、rewrite 差，还是 rerank 差。

### Task

我要把 RAG 在线评测拆成三种模式：

1. `retrieve`：原 query 直接检索
2. `rewrite`：先 query rewrite，再检索
3. `full`：rewrite + retrieve + rerank

目标是定位瓶颈，而不是盲目继续调 prompt。

### Action

使用 telemetry evidence build collection：

- collection：`aiops_evidence_telemetry_build_resume_20260412`
- eval：`aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl`
- mode：`retrieve`、`rewrite`、`full`

### Result

严格 telemetry baseline：

| Mode | AvgTotal | Hit@1 | Hit@3 | Hit@5 | AvgRecall@1 | AvgRecall@3 | AvgRecall@5 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| retrieve | 43.64ms | 0.20625 | 0.36250 | 0.38750 | 0.0110 | 0.0210 | 0.0225 |
| rewrite | 3051.20ms | 0.20625 | 0.36250 | 0.38750 | 0.0110 | 0.0210 | 0.0225 |
| full | 2380.78ms | 0.20625 | 0.36250 | 0.38750 | 0.0110 | 0.0210 | 0.0225 |

结论非常明确：

- 三种模式效果几乎完全一致
- rewrite 和 full 没有带来召回收益
- rewrite/full 把延迟从几十毫秒拉到 2 到 3 秒
- 当前瓶颈在召回层，不在 LLM rewrite 或 rerank

面试讲法：

> 我没有一上来就说要加 rerank 或换 prompt，而是把链路拆成 retrieve、rewrite、full 三种模式。结果发现三种模式的 Hit/Recall 完全一样，但 rewrite/full 延迟高了几十倍。这个结论很重要：它说明当前优化优先级应该回到召回层，而不是继续堆 LLM 步骤。

## 6. STAR 五：Telemetry Metadata Rerank Baseline

### Situation

Telemetry 文档不是普通 markdown，它天然带有结构化字段，比如：

- `service`
- `instance_type`
- `source`
- `destination`
- `metric_names`
- `trace_operations`

如果这些 metadata 只存在本地 sidecar 文件里，却没有进入 Milvus metadata 字段，那它们对检索没有任何价值。

### Task

我要验证两件事：

1. sidecar metadata 是否真的进入线上 collection
2. 轻量 metadata rerank 是否能带来可测收益

### Action

先做候选过召回，再做本地轻量重排：

- 扩大候选 `topK`
- 利用正文词面 overlap
- 利用 `service / metric_names / trace_operations` 等 metadata overlap
- 重建带 metadata 的新 collection：`aiops_evidence_telemetry_build_meta_opt1_20260412`

第一次复跑无提升后，我没有继续调权重，而是检查远端 collection，发现旧 collection 只有 `_source / _file_name / title / subtitle`，缺少真正的 telemetry sidecar metadata。因此修正动作是先重建索引，而不是继续改排序公式。

### Result

重建后结果：

| Variant | AvgTotal | Hit@1 | Hit@3 | Hit@5 | AvgRecall@1 | AvgRecall@3 | AvgRecall@5 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| baseline_retrieve | 43.64ms | 0.20625 | 0.36250 | 0.38750 | 0.01100 | 0.02095 | 0.02249 |
| metadata_rerank | 47.69ms | 0.19375 | 0.39375 | 0.39375 | 0.00946 | 0.02325 | 0.02359 |

解释：

- `Hit@3` 从 0.3625 提升到 0.39375
- `Hit@5` 从 0.3875 提升到 0.39375
- `AvgRecall@3/@5` 有小幅提升
- `Hit@1` 下降，说明当前重排更擅长把相关案例拉进 top5，但还不擅长抢第一

面试讲法：

> 我做 metadata rerank 时，第一次线上无提升。这个时候我没有继续盲调参数，而是检查数据面，发现旧 collection 根本没把 sidecar metadata 写进去。修正后重新建索引，Hit@3 和 Hit@5 才出现小幅提升。这件事说明 RAG 优化不能只看算法，必须确认数据特征真的进了线上索引。

## 7. STAR 六：本地 BM25 Archive Baseline

### Situation

当前本地没有完整 `aiopschallenge2025` 目录，也没有可直接复跑的全量 holdout 数据。为了在没有 Milvus 和完整数据集的情况下仍然能先测一个轻量基线，我从历史归档中提取了 build docs 和 eval cases，做了本地 BM25 对照。

### Task

我要快速回答：

1. 现有 telemetry build 文档在关键词检索下大概是什么水平
2. 是否能先得到一个不依赖 Milvus 的 baseline
3. 这组 baseline 能不能作为后续 hybrid retrieval 的参考下限

### Action

使用历史归档：

- `logs/telemetry-light-artifacts-resume-20260412-1635.tar.gz`
- build docs：320
- raw eval cases：800
- 可与 build docs 对齐的 eval cases：640

本地用 BM25 词法检索，计算 HitRate、AvgRecall、NDCG、MRR。

### Result

本地 BM25 archive baseline：

| Metric | @1 | @3 | @5 | @10 |
| --- | ---: | ---: | ---: | ---: |
| AvgRecall | 0.0969 | 0.2484 | 0.3609 | 0.5266 |
| HitRate | 0.0969 | 0.2484 | 0.3609 | 0.5266 |
| FullRecall | 62/640 | 159/640 | 231/640 | 337/640 |
| NDCG | 0.0969 | 0.1853 | 0.2315 | 0.2854 |

补充指标：

- MRR：0.2290
- `miss_or_rank_gt5`：409

解释：

- 这不是严格 holdout 的最终效果指标
- 它是一个轻量词法检索下限
- 它能帮助判断后续 hybrid retrieval 是否真的超过了关键词 baseline

面试讲法：

> 在完整数据集暂时不可用时，我没有停下来等环境，而是先从历史归档里抽取 build docs 和可对齐 eval cases，做了一个本地 BM25 baseline。它不能替代严格 Milvus holdout，但能提供一个轻量下限：如果后续 dense 或 hybrid 检索连这个词法 baseline 都打不过，就说明优化方向有问题。

## 8. STAR 七：线上服务 Smoke Baseline

### Situation

项目已经部署在服务器 `124.222.57.178` 上。面试时如果只讲本地代码，不讲线上验证，很容易被认为只是本地 demo。线上服务至少要证明：

1. 服务入口可访问
2. 健康检查可用
3. 可观测性入口可用
4. 真实业务接口是否存在依赖问题

### Task

我要做一轮轻量线上 smoke baseline，确认服务器当前状态。

### Action

探测了以下入口：

- `/ai/`
- `/ai/healthz`
- `/ai/readyz`
- Prometheus
- Jaeger
- Chat async submit
- Chat sync
- RAG docs query

### Result

当前线上 smoke 结果：

| 项目 | 结果 |
| --- | --- |
| `/ai/` | 可访问 |
| `/ai/healthz` | 可访问 |
| `/ai/readyz` | 可访问 |
| Prometheus | 可访问 |
| Jaeger | 可访问 |
| async chat submit | 可提交成功 |
| sync chat | 5s 窗口内超时 |
| RAG docs query | 受 Milvus/default DB context canceled 影响 |

解释：

- 这说明线上基础服务和观测入口是活的
- 但线上 RAG 依赖还没有达到“效果稳定”的状态
- sync chat 超时说明还需要做真实链路的 timeout、降级和依赖治理

面试讲法：

> 我做线上 smoke baseline 时，不只看服务能不能打开，而是分成健康检查、观测入口、异步提交、同步对话和 RAG 依赖几个层次。当前服务器基础入口可用，async chat 可提交，但 sync chat 和 RAG 依赖仍暴露出超时和 Milvus default database 问题，所以我不会把线上效果包装成已经完全稳定。

## 9. 面试时的一分钟总回答

可以这样讲：

> 我对项目不是只做功能 demo，而是先建立了几层 baseline。第一层是工程 baseline，Go 1.24.4 下全仓测试和构建通过；第二层是 RAG smoke baseline，17 篇文档、20 条 case 下 AvgRecall@5 是 0.97，HitRate@5 是 1.00，用来保护基础检索链路；第三层是 AIOps 严格 holdout baseline，发现 Hit@5 能到 0.71 左右，但 AvgRecall@5 只有 0.158，说明方向能命中但覆盖不足；第四层是 telemetry Milvus baseline，我把 retrieve、rewrite、full 拆开跑，发现 LLM rewrite/rerank 没有提升召回，只增加延迟，因此瓶颈在召回层；第五层是 metadata rerank，重建带 metadata 的 collection 后 Hit@3/@5 有小幅提升，但还不足以解决核心问题。最后我还做了线上 smoke，确认服务器基础入口可用，但 RAG 依赖还有 Milvus 和超时治理问题。  
> 所以这个项目的优化路线不是盲目堆 Agent 或 prompt，而是先把评测口径、数据面、召回层和线上依赖逐步基线化。

## 10. 面试追问答法

### Q1：为什么你的 RAG 指标有好几套，哪个是真的？

答：

> 小样本 RAG smoke baseline 是工程自检，证明 harness 和基础检索可用；严格 holdout baseline 才是效果评估。两者目的不同，不能混在一起解释。面试里我会明确区分 smoke、archive BM25、strict Milvus holdout 和 online smoke。

### Q2：为什么 HitRate@5 高，但 AvgRecall@5 低？

答：

> HitRate@5 只看 top5 里有没有至少一个相关文档，AvgRecall@5 看相关集合覆盖比例。AIOps holdout 里每个 query 的相关集合可能有十几个到三十几个历史案例，所以 top5 里碰到一个并不代表覆盖充分。这就是为什么 Hit@5 约 0.71，但 AvgRecall@5 只有 0.158。

### Q3：为什么 rewrite 和 rerank 没有效果？

答：

> 因为当时 retrieve、rewrite、full 三种模式的召回结果几乎完全一致，而 rewrite/full 延迟显著增加。这说明问题不在 query 改写或最终重排，而在初始候选召回质量。候选集本身不够好，后面的 LLM 步骤很难凭空补回来。

### Q4：metadata rerank 提升这么小，是否说明它没价值？

答：

> 不能这么判断。第一次无提升是因为旧 collection 根本没有 sidecar metadata。重建后 Hit@3/@5 有小幅提升，说明 metadata 有价值；但它只是 soft rerank，不能替代召回层。下一步应该做 hybrid retrieval、metadata-aware prefilter 和更细粒度 chunking。

### Q5：线上服务现在稳定吗？

答：

> 基础入口是可用的，健康检查和观测入口能访问，异步 chat 能提交。但同步 chat 有超时，RAG docs 受到 Milvus/default database 问题影响。所以我会说当前线上具备 smoke 可用性，但还不能说 RAG 在线效果已经稳定，下一步要治理依赖健康、超时和 readiness。

## 11. 下一步优化路线

按当前 baseline，下一步不建议先做复杂 Agent 协作，而是按这个顺序推进：

1. 固化本地 BM25 archive baseline，把临时脚本变成可复用命令或报告产物
2. 恢复完整 `aiopschallenge2025` 数据和 `eval_cases_holdout_related.jsonl`
3. 重建带 metadata 的 Milvus collection，并保留 collection schema/version 记录
4. 复跑 `retrieve / rewrite / full` 三模式，确认默认模式继续使用 `retrieve`
5. 引入 hybrid retrieval，用 dense + BM25 + RRF 对比当前 strict baseline
6. 增加 MRR、NDCG、按故障类型分桶的 failure breakdown
7. 治理线上 RAG 依赖：Milvus default database、readiness、timeout、degraded result

## 12. 最终项目定位

面试里可以把项目定位成：

> OpsCaptionAI 是一个面向内部运维团队的 AIOps 智能助手。我在这个项目里重点做的不是堆概念，而是把 Chat、RAG、Context、Memory 和工具治理接成可运行系统，并通过多层 baseline 把效果和风险量化出来。当前最有价值的工程结论是：RAG 优化必须先有正确评测口径，再确认数据特征真的进索引，最后再比较召回、rewrite、rerank 和线上依赖各层的收益。
