#!/usr/bin/env bash
# RAG v2 Baseline Experiment
# Compares: hybrid-retrieve (production default) vs hybrid-rerank (rerank on)
# Both modes exercise the production rag.Query() path (dense + BM25 + RRF).
# Requires: Milvus, Redis, and config.yaml with rag: block configured.
set -euo pipefail

OUT_DIR="internal/ai/cmd/rag_online_eval_cmd/results"
mkdir -p "$OUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== RAG Baseline Experiment: $TIMESTAMP ==="
echo ""

# Experiment 1: hybrid-retrieve (production default, rerank off)
echo "--- [1/2] hybrid-retrieve (production path, rerank=false) ---"
go run ./internal/ai/cmd/rag_online_eval_cmd/ \
  -mode hybrid-retrieve \
  -ks 1,3,5 \
  -timeout-ms 15000 \
  -out "$OUT_DIR/${TIMESTAMP}_hybrid_retrieve.json" \
  2>&1 | tee "$OUT_DIR/${TIMESTAMP}_hybrid_retrieve.txt"

echo ""
echo "--- [2/2] hybrid-rerank (production path, rerank=true) ---"
go run ./internal/ai/cmd/rag_online_eval_cmd/ \
  -mode hybrid-rerank \
  -ks 1,3,5 \
  -timeout-ms 15000 \
  -out "$OUT_DIR/${TIMESTAMP}_hybrid_rerank.json" \
  2>&1 | tee "$OUT_DIR/${TIMESTAMP}_hybrid_rerank.txt"

echo ""
echo "=== Results saved to $OUT_DIR/${TIMESTAMP}_*.json ==="
echo ""
echo "To compare summaries:"
echo "  jq '{mrr:.summary.mrr, recall5:.summary.avg_recall_at_k[\"5\"], empty:.summary.empty_rate}' $OUT_DIR/${TIMESTAMP}_hybrid_retrieve.json"
echo "  jq '{mrr:.summary.mrr, recall5:.summary.avg_recall_at_k[\"5\"], empty:.summary.empty_rate}' $OUT_DIR/${TIMESTAMP}_hybrid_rerank.json"
