# RAG v2 Baseline Experiment

## Date: 2026-05-30

## Objective

Establish a quantitative baseline for RAG retrieval quality before further tuning. Compare two configurations on the **production hybrid path** (`rag.Query()`: dense + BM25 + RRF):

| Mode | Rewrite | Rerank | Path |
|------|---------|--------|------|
| `hybrid-retrieve` | off | off | `rag.Query()` with explicit rewrite/rerank overrides |
| `hybrid-rerank` | off | on | `rag.Query()` with rerank override |

Both modes exercise the full production retrieval pipeline. Eval-only modes (`retrieve`, `rerank`) call `QueryForEval()` which skips BM25 — use them for ablation studies only, not as production baselines.

## Test Set

18 cases in `eval/testdata/baseline_cases.jsonl`, covering:

| Category | Cases | Example |
|----------|-------|---------|
| Single-doc hit | RAG-01, 02, 03, 05, 06, 07, 08, 09, 10, 11, 14, 15, 16 | "Prometheus 告警先看什么" |
| Multi-doc hit | RAG-04, 17 | "服务发布后怎么回滚" |
| Cross-domain | RAG-04, 17, 18 | "支付告警先怎么分诊" |
| Colloquial | RAG-13 | "容器启动就挂了日志里有 OOM" |
| English query | RAG-15 | "How to rollback a Helm release" |
| Synonym rewrite | RAG-16 | "服务可用性指标怎么定义" (SLI absent) |

## Metrics

| Metric | Meaning |
|--------|---------|
| Recall@K | Fraction of relevant docs found in top K |
| HitRate@K | Fraction of queries with at least 1 hit in top K |
| FullRecall@K | Count of queries with ALL relevant docs in top K |
| MRR | Mean Reciprocal Rank of first relevant result |
| CitationCoverage | Fraction of queries returning non-empty results |
| EmptyRate | Fraction of queries returning zero results |

## How to Run

```bash
# Ensure Milvus, Redis, and config.yaml are ready
bash internal/ai/cmd/rag_online_eval_cmd/run_baseline.sh
```

Results are saved to `internal/ai/cmd/rag_online_eval_cmd/results/`.

## Analysis Guide

After running, compare the two JSON reports. Note: `QueryCaseResult` embeds `CaseResult` anonymously, so fields are flattened in JSON (no `.case_result` nesting).

```bash
# Quick summary comparison
jq '{mrr: .summary.mrr, recall5: .summary.avg_recall_at_k["5"], empty: .summary.empty_rate}' results/*_hybrid_retrieve.json
jq '{mrr: .summary.mrr, recall5: .summary.avg_recall_at_k["5"], empty: .summary.empty_rate}' results/*_hybrid_rerank.json

# Per-case recall comparison
jq -r '.results[] | "\(.case_id) \(.metrics.total_latency_ms)ms recall@5=\(.recall_at_k["5"])"' results/*_hybrid_retrieve.json
jq -r '.results[] | "\(.case_id) \(.metrics.total_latency_ms)ms recall@5=\(.recall_at_k["5"])"' results/*_hybrid_rerank.json
```

### Trace-based failure analysis

For cases where recall@5 < 1.0, inspect the ranked IDs and metrics to identify the failure stage:

```bash
# Find failed cases
jq -r '.results[] | select(.recall_at_k["5"] < 1.0) | .case_id' results/*_hybrid_retrieve.json

# Inspect ranked IDs for a specific case
jq '.results[] | select(.case_id == "RAG-05") | .ranked_ids' results/*_hybrid_retrieve.json

# Inspect query-level metrics (latency breakdown, result count)
jq '.results[] | select(.case_id == "RAG-05") | .metrics' results/*_hybrid_retrieve.json
```

Note: The report JSON includes ranked IDs and aggregate metrics, but not per-doc trace (dense/lexical/fusion/rerank scores). To inspect per-doc trace, run the eval with `-out` and add the retrieved docs to the report, or use the rag.Query() API directly with trace logging.

Failure categories:

| Symptom | Likely Cause | Action |
|---------|-------------|--------|
| relevant doc not in ranked_ids at all | Dense+BM25 both missed | Improve chunk content, add keywords |
| relevant doc present but ranked low | RRF fusion misordered | Tune FusionK, add metadata boost |
| relevant doc ranked high without rerank, low with rerank | Rerank model issue | Check rerank prompt, try different model |
| Empty result | BM25 index empty or dense index empty | Check warmup, check Milvus collection |

## Eval-only modes (ablation)

These modes call `QueryForEval()` which does **dense-only** retrieval (no BM25, no RRF). Use for ablation studies, not as production baselines:

| Mode | Rewrite | Rerank | Path |
|------|---------|--------|------|
| `retrieve` | off | off | `QueryForEval()` dense only |
| `rerank` | off | on | `QueryForEval()` dense + rerank |
| `rewrite` | on | off | `QueryForEval()` rewrite + dense |
| `full` | on | on | `QueryForEval()` rewrite + dense + rerank |

## Next Steps (after baseline)

1. Run the experiment and record results here
2. Identify top 3 failure cases
3. For each failure, trace through dense/BM25/fusion/rerank stages
4. Decide: tune chunk, tune BM25, tune fusion, or adjust prompt/rewrite
