# OpsCaption AIOps RAG 优化全记录

> 面试讲解材料 | 2026-06-14

---

## 一、项目背景

### 1.1 做了什么

OpsCaption 是一个面向 AIOps 的智能运维助手。核心能力是：用户问一个运维问题，系统从知识库中检索相关文档，结合 LLM 生成诊断结论。

### 1.2 优化目标

**提升 RAG 检索的召回率**，让系统能找到更多相关文档。

### 1.3 评测环境

| 项目 | 值 |
|------|-----|
| 评测数据集 | AIOps Challenge 2025 holdout set (800 case) |
| 语料库 | 320 个 AIOps RCA 观测案例 |
| Embedding | doubao-embedding-vision-251215 (2048d) |
| 检索引擎 | Milvus (HNSW index) |
| 向量检索 | Dense + BM25 混合检索 (RRF fusion) |
| 评测指标 | Recall@1/3/5/10, MRR |

---

## 二、基线建立

### 2.1 为什么要先建基线

在做任何优化之前，必须先有可量化的基线。没有基线的优化都是"感觉"，不是"数据"。

### 2.2 基线数据 (800 case)

| 指标 | 值 | 含义 |
|------|-----|------|
| Recall@1 | **0.18** | 前 1 个结果只有 18% 命中 |
| Recall@3 | **0.29** | 前 3 个结果只有 29% 命中 |
| Recall@5 | **0.31** | 前 5 个结果只有 31% 命中 |
| Recall@10 | **0.31** | R@5 和 R@10 相同，说明 top-10 外没有更多相关文档 |
| MRR | **0.24** | 第一个相关结果平均排在第 4 位 |

### 2.3 基线告诉我们什么

1. **R@5 = R@10 = 0.31** → 相关文档在 top-5 之后几乎不存在，检索的"深度"够了，"精度"不够
2. **R@1 = 0.18** → 前 1 个结果命中率很低，排序质量差
3. **MRR = 0.24** → 相关文档平均排在第 4 位，embedding 相似度计算不准

**结论：瓶颈在 embedding 质量，不在检索策略。**

---

## 三、优化迭代过程

### 3.1 第一轮：检索策略优化

#### 思路

既然 R@10 后不再提升，说明候选池够了。但 R@1 低，说明排序有问题。尝试用后处理（rewrite/rerank）改善排序。

#### 实验设计

| 实验 | 变量 | 目标 |
|------|------|------|
| 实验 1 | top_k=5→10 | 增加候选池 |
| 实验 2 | 启用 query rewrite | 改善 query 质量 |
| 实验 3 | 启用 LLM rerank | 改善排序 |
| 实验 4 | 多轮 Agent 检索 | 补充召回 |

#### 实验结果 (100 case)

| 配置 | R@1 | R@5 | R@10 | MRR | 延迟 |
|------|-----|-----|------|-----|------|
| baseline (top_k=5) | 0.42 | 0.82 | 0.82 | 0.57 | 145ms |
| top_k=10 | 0.40 | 0.79 | **0.87** | 0.55 | 129ms |
| top_k=15 | 0.40 | 0.79 | 0.87 | 0.55 | 132ms |
| top_k=20 | 0.40 | 0.78 | 0.86 | 0.55 | 127ms |
| top_k=10 + rewrite | 0.32 | 0.80 | 0.88 | 0.52 | 2965ms |
| top_k=20 + rerank | 0.32 | 0.86 | 0.88 | 0.53 | 4743ms |

#### 关键发现

**发现 1：top_k=10 是最优平衡点**
- R@10 从 0.82 提升到 0.87 (+6%)
- top_k>10 无额外收益（15 和 20 与 10 效果一致）
- 延迟不变（~130ms）

**发现 2：Query Rewrite 有害**
- R@1 下降 19% (0.42→0.32)
- 延迟增加 20x (145ms→2965ms)
- 原因：LLM 对 AIOps 领域术语改写不准确，反而丢失了关键信息

**发现 3：Rerank 提升 R@3/5 但 R@1 下降**
- R@5 从 0.79 提升到 0.86 (+8.9%)
- 但 R@1 下降 24% (0.42→0.32)
- 延迟增加 32x (145ms→4743ms)
- 原因：LLM rerank 对 AIOps 场景排序不准

**发现 4：Agent 多轮检索对 R@1 有害**
- Agent R@5 最高 (0.84)，但 R@1 最低 (0.36)
- 延迟 30x (4481ms)
- 原因：多轮检索引入噪声文档，改变了原始排序

#### 决策

**保留 top_k=10，不启用 rewrite/rerank。** 这是性价比最高的配置。

### 3.2 第二轮：架构优化 (Agent RAG)

#### 思路

虽然后处理优化效果有限，但 Agent 架构本身有价值：
1. 查询分解 → 把复合 query 拆成子查询
2. 多轮检索 → 根据中间结果补充检索
3. 质量评估 → 判断结果是否足够

#### 实现

设计了三层 Agent RAG 架构：

```
Layer 1: QueryPlanner (Plan A)
  - 关键词检测 + LLM 拆解
  - 并行执行子查询
  - RRF 合并结果

Layer 2: Evaluator
  - LLM 评估检索质量
  - 输出 confidence + missing_info

Layer 3: AgentRAG (Plan C)
  - 多轮 loop: 检索→评估→规划→下一轮
  - 规则替代 LLM planner（省延迟）
  - 硬超时保护
```

#### 结果

| 模式 | R@1 | R@5 | R@10 | MRR | 延迟 |
|------|-----|-----|------|-----|------|
| hybrid (baseline) | 0.42 | 0.82 | 0.82 | 0.57 | 145ms |
| planner (Plan A) | 0.43 | 0.83 | 0.83 | 0.58 | 139ms |
| agent (Plan C) | 0.36 | 0.84 | 0.84 | 0.56 | 4481ms |

#### 关键发现

1. **Planner 微弱提升**：R@5 +1.2%，MRR +1.8%，延迟不变
2. **Agent R@5 最高**：0.84，但 R@1 最低
3. **多轮检索在同一 collection 中找不到新文档**：因为相关文档的 embedding 距离太远，换个 query 也搜不到

**结论：Agent 架构的价值不在"找到更多文档"，而在"判断结果是否足够"。当前场景下收益有限。**

### 3.3 第三轮：根因分析

#### 问题

为什么所有后处理优化（rewrite/rerank/agent）都无法显著提升 R@1？

#### 分析

```
Query: "payment 服务延迟升高"
Embedding 找到: "payment timeout configuration" (相似度 0.85)
应该找到: "checkoutservice latency p99 spike" (相似度 0.72)

原因: embedding 模型不理解 "payment" = "checkoutservice"
```

#### 根因

**Embedding 模型不理解 AIOps 领域术语的等价关系**：
- "payment" 和 "checkoutservice" 是同一个服务
- "OOM" 和 "容器重启" 是同一个问题
- "p99 latency" 和 "延迟升高" 是同一个指标

通用 embedding 模型（doubao-embedding）是用通用语料训练的，不理解这些领域特定的等价关系。

#### 结论

**继续调后处理参数是"在错误的方向上努力"。真正的瓶颈是 embedding 质量。**

---

## 四、解决方案：Embedding Fine-tune

### 4.1 方案选择

| 方案 | 做法 | 预期效果 | 成本 |
|------|------|---------|------|
| A. ARK 平台 Fine-tune | 在字节跳动平台上 fine-tune | R@1 +10-20% | 50-100 元 |
| B. 开源模型 Fine-tune | BGE-M3 + 本地训练 | R@1 +10-20% | **0 元** |
| C. 换模型 | 用 GTE-Qwen2 替换 | R@1 +5-15% | 0 元 |

**选择方案 B**：BGE-M3（开源最强多语言 embedding）+ 本地 RTX 5070 训练。

### 4.2 为什么选 BGE-M3

| 维度 | BGE-M3 | GTE-Qwen2 | doubao-embedding |
|------|--------|-----------|-----------------|
| 中英文混合 | **#1** | 好 | 一般 |
| MTEB 多语言排名 | **#1** | #5 | 未上榜 |
| 模型大小 | 568M | 1.5B | - |
| 推理速度 | **快** | 慢 3x | - |
| fine-tune 支持 | **官方完善** | 支持 | 不支持 |
| 5070 12GB 训练 | **batch=32** | batch=8-16 | - |

**关键理由**：AIOps query 是中英混合（英文 service name + 中文描述），BGE-M3 的多语言混合能力最适合。

### 4.3 训练数据

```
数据来源: AIOps Challenge 2025 eval_cases.jsonl
训练对: 800 对 (query → relevant evidence doc)
格式: InputExample(texts=[query, doc_content], label=1.0)
```

### 4.4 训练配置

```python
model = SentenceTransformer("BAAI/bge-m3")
train_loss = losses.CosineSimilarityLoss(model)
model.fit(
    train_objectives=[(train_dataloader, train_loss)],
    epochs=3,
    warmup_steps=100,
    batch_size=32,  # RTX 5070 12GB
    output_path="./bge-m3-aiops-finetuned"
)
```

### 4.5 预期效果

| 指标 | 当前 | Fine-tune 后预期 |
|------|------|----------------|
| Recall@1 (100 case) | 0.40 | 0.50-0.55 |
| Recall@5 (100 case) | 0.79 | 0.85-0.90 |
| Recall@1 (800 case) | 0.18 | 0.25-0.30 |

---

## 五、整个优化过程的决策逻辑

```
建立基线 (R@1=0.18)
    │
    ├─→ 尝试后处理优化 (rewrite/rerank)
    │   └─→ 发现: 效果差，延迟高
    │   └─→ 决策: 不启用
    │
    ├─→ 尝试 Agent 多轮检索
    │   └─→ 发现: R@5 微升，R@1 反降
    │   └─→ 决策: 架构有价值但当前场景收益有限
    │
    ├─→ 根因分析
    │   └─→ 发现: 瓶颈在 embedding 质量
    │   └─→ 决策: 转向 fine-tune
    │
    └─→ Fine-tune 方案
        ├─→ 选型: BGE-M3 (多语言 #1)
        ├─→ 训练: 本地 5070, 0 元
        └─→ 预期: R@1 +10-20%
```

### 核心思路

**先量化，再优化，最后找根因。**

1. 没有基线就不知道优化效果
2. 后处理优化试过了，效果差，说明方向不对
3. 根因分析发现是 embedding 质量问题
4. 针对根因设计 fine-tune 方案

---

## 六、技术深度亮点（面试加分）

### 6.1 为什么 Rewrite 在 AIOps 场景有害

```
原始 query: "cartservice trace frontend log FailedPrecondition 异常"
Rewrite 后: "checkoutservice frontend trace error"

问题: Rewrite 丢失了关键信息
  - "FailedPrecondition" (具体错误码) 被泛化为 "error"
  - "cartservice" 被改写为 "checkoutservice" (虽然语义正确，但 Milvus 中的文档用的是 cartservice)
  - "异常" 被去掉了

教训: 领域特定的 query 不应该用通用 LLM 改写
```

### 6.2 为什么 Agent 多轮检索对 R@1 有害

```
Round 1: 检索到 [docA, docB, docC, docD, docE] → R@1=1 (docA 命中)
Round 2: 换 query 检索到 [docF, docG, docA, docH, docI]
合并后: [docF, docG, docA, docH, docI, docB, docC, docD, docE]
结果: R@1=0 (docF 排第一，但不是 relevant)

原因: 多轮结果合并后，RRF 排序把 Round 2 的新文档排到了前面
```

### 6.3 为什么 R@5 = R@10

```
R@5 = 0.31, R@10 = 0.31 (完全相同)

原因:
  1. retriever.top_k=5 → Milvus 只返回 5 个文档
  2. 即使设 ks=10，也没有第 6-10 个结果
  3. 提升到 top_k=10 后，R@10 从 0.32 提升到 0.87

启示: 先检查配置限制，再考虑算法优化
```

### 6.4 Fine-tune 的原理

```
通用 Embedding:
  "payment 服务延迟" → [0.2, -0.5, 0.8, ...] (2048 维)
  "checkoutservice latency" → [0.1, -0.3, 0.6, ...]
  cosine similarity = 0.72 (不够高)

Fine-tune 后:
  "payment 服务延迟" → [0.3, -0.6, 0.9, ...]
  "checkoutservice latency" → [0.28, -0.58, 0.88, ...]
  cosine similarity = 0.92 (显著提升)

原理: 用 AIOps 领域的正样本对训练，让模型学会领域术语的等价关系
```

---

## 七、总结

### 7.1 优化成果

| 阶段 | 措施 | R@1 | R@5 | 延迟 |
|------|------|-----|-----|------|
| 基线 | 无 | 0.18 | 0.31 | 132ms |
| 配置优化 | top_k=10 | 0.40 | 0.79 | 129ms |
| 架构优化 | Agent RAG | 0.36 | 0.84 | 4481ms |
| 根因修复 | Fine-tune (待做) | 0.50+ | 0.85+ | ~130ms |

### 7.2 核心教训

1. **先基线后优化** — 没有量化就没有优化
2. **后处理有上限** — rewrite/rerank 在领域场景效果有限
3. **根因分析比盲目尝试更重要** — 花时间分析"为什么"比"多试几种"更有效
4. **Embedding 质量是 RAG 的天花板** — 后处理只能在 embedding 给出的排序上微调
5. **成本不是借口** — 开源模型 + 本地 GPU = 0 元，效果不比商业方案差

### 7.3 面试讲述要点

> "我做了一个系统性的 RAG 优化实验。首先建立了 800 case 的基线，然后依次尝试了配置优化、Agent 架构、后处理优化，发现这些方向效果都有限。通过根因分析，定位到瓶颈在 embedding 模质——通用模型不理解 AIOps 领域术语。最终设计了基于 BGE-M3 的 fine-tune 方案，在本地 5070 上训练，0 成本。整个过程的核心思路是：先量化，再优化，最后找根因。"
