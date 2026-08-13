#!/usr/bin/env bash
# RAG retrieval development ablations.
# All modes exercise the production rag.Query() path (dense + BM25 + RRF).
# Requires: Milvus, Redis, and config.yaml with rag: block configured.
set -euo pipefail

OUT_DIR="internal/ai/cmd/rag_online_eval_cmd/results"
mkdir -p "$OUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
EVAL_PATH="evals/rag/retrieval_development.jsonl"
CORPUS_VERSION="${RAG_CORPUS_VERSION:-knowledge-seed-v1}"

echo "=== RAG Baseline Experiment: $TIMESTAMP ==="
echo ""

MODES=(hybrid-retrieve hybrid-rewrite hybrid-rerank hybrid-full)
for INDEX in "${!MODES[@]}"; do
  MODE="${MODES[$INDEX]}"
  echo "--- [$((INDEX + 1))/${#MODES[@]}] $MODE ---"
  go run ./internal/ai/cmd/rag_online_eval_cmd/ \
    -eval "$EVAL_PATH" \
    -dataset-role development \
    -corpus-version "$CORPUS_VERSION" \
    -mode "$MODE" \
    -ks 1,3,5 \
    -timeout-ms 15000 \
    -out "$OUT_DIR/${TIMESTAMP}_${MODE}.json" \
    2>&1 | tee "$OUT_DIR/${TIMESTAMP}_${MODE}.txt"
  echo ""
done

echo ""
echo "=== Results saved to $OUT_DIR/${TIMESTAMP}_*.json ==="
