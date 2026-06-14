# Agent RAG 优化实验报告

日期: 2026-06-14
实验人: AI Agent
目标: 系统优化 RAG 检索质量，找到最优配置

## 1. 实验环境

- 服务器: 124.222.57.178
- Milvus collection: `aiops_evidence_build` (320 个 AIOps RCA 案例)
- Embedding: doubao-embedding-vision-251215 (2048d)
- LLM: deepseek-v4-flash
- 评测数据集: `eval_cases.jsonl` (800 case)

## 2. 基线结果 (800 case)

| 指标 | 值 |
|------|-----|
| Recall@1 | 0.18 |
| Recall@3 | 0.29 |
| Recall@5 | 0.31 |
| Recall@10 | 0.31 |
| MRR | 0.24 |
| 延迟 | 132ms |

## 3. 配置优化实验 (100 case 采样)

### 3.1 top_k 调优

| top_k | R@1 | R@3 | R@5 | R@10 | MRR | 延迟 |
|-------|-----|-----|-----|------|-----|------|
| 5 (原配置) | 0.42 | 0.71 | 0.82 | 0.82 | 0.57 | 145ms |
| 10 | 0.40 | 0.70 | 0.79 | **0.87** | 0.55 | 129ms |
| 15 | 0.40 | 0.70 | 0.79 | 0.87 | 0.55 | 132ms |
| 20 | 0.40 | 0.70 | 0.78 | 0.86 | 0.55 | 127ms |

**结论**: top_k=10 是最优平衡点，R@10 提升 6%。

### 3.2 Query Rewrite

| 配置 | R@1 | R@5 | R@10 | MRR | 延迟 |
|------|-----|-----|------|-----|------|
| top_k=10 + rewrite=on | 0.32 | 0.80 | 0.88 | 0.52 | 2965ms |
| top_k=10 + rewrite=off | 0.40 | 0.79 | 0.87 | 0.55 | 129ms |

**结论**: Rewrite 有害。LLM 改写后的 query 对 AIOps 领域术语改写不准确，R@1 下降 19%。

### 3.3 Rerank

| 配置 | R@1 | R@5 | R@10 | MRR | 延迟 |
|------|-----|-----|------|-----|------|
| top_k=20 + rerank=on | 0.32 | 0.86 | 0.88 | 0.53 | 4743ms |
| top_k=10 + rerank=off | 0.40 | 0.79 | 0.87 | 0.55 | 129ms |

**结论**: Rerank 提升 R@3/5 但 R@1 下降，延迟 +4.5s。性价比低。

### 3.4 Agent 多轮检索

| 配置 | R@1 | R@5 | R@10 | MRR | 延迟 |
|------|-----|-----|------|-----|------|
| agent (100 case) | 0.36 | 0.84 | 0.84 | 0.56 | 4481ms |
| hybrid (100 case) | 0.42 | 0.82 | 0.82 | 0.57 | 145ms |

**结论**: Agent R@5 最高 (0.84)，但 R@1 最低 (0.36)，延迟 30x。适合对延迟不敏感的场景。

## 4. 最优配置

```yaml
retriever:
  top_k: 10

rag:
  rewrite_enabled: false
  rerank_enabled: false
  hybrid_dense_top_k: 50
  hybrid_lexical_top_k: 50
  hybrid_candidate_top_k: 20
  hybrid_final_top_k: 10
```

## 5. 瓶颈分析

当前 R@1=0.40 的瓶颈不在检索策略，而在：

1. **Embedding 质量** — doubao-embedding 对 AIOps 领域术语（service name、metric name、trace operation）理解不够
2. **候选池深度** — top_k>10 无额外收益，说明相关文档在 embedding 空间中距离较远
3. **RRF 融合** — Dense + Lexical 的融合策略对 AIOps 查询效果有限

## 6. 下一步优化方向

| 方向 | 预期效果 | 难度 |
|------|---------|------|
| Fine-tune embedding model | R@1 提升 10-20% | 中 |
| 增加 AIOps 领域语料 | R@5 提升 5-10% | 低 |
| Metadata-aware prefilter | R@1 提升 5-10% | 中 |
| Multi-vector indexing | R@10 提升 5-10% | 高 |
