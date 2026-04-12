#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bash scripts/aiops/run_telemetry_baseline_remote.sh [options]

Purpose:
  Run the cloud telemetry-evidence baseline end-to-end:
  1. optional Milvus startup
  2. baseline split/eval manifest generation
  3. telemetry evidence doc generation from parquet
  4. indexing build-only telemetry docs
  5. strict holdout evaluation

Options:
  --start-milvus          Start dedicated Milvus via manifest/docker/docker-compose.remote.yml
  --skip-prep             Skip aiops_rag_prep_cmd
  --skip-telemetry        Skip build_telemetry_evidence.py
  --skip-index            Skip knowledge_cmd indexing
  --skip-eval             Skip rag_online_eval_cmd
  --limit N               Optional case limit passed to build_telemetry_evidence.py
  --ks CSV                K list for evaluation, default: 1,3,5
  --timeout-ms N          Per-query eval timeout, default: 30000
  --collection NAME       Milvus collection, default: aiops_evidence_telemetry_build
  --dataset-root PATH     Dataset root under workspace, default: ./aiopschallenge2025
  --output-root PATH      Output root under workspace, default: <dataset-root>/baseline
  --env-file PATH         Env file for Go indexing/eval containers, default: /opt/opscaptain/.env.production
  --help                  Show this help

Environment overrides:
  WORKSPACE_ROOT
  GO_IMAGE
  PYTHON_IMAGE
  GO_MOD_CACHE
  GO_BUILD_CACHE
  PIP_CACHE
  MILVUS_ADDRESS
  COMPOSE_PROJECT_NAME
  COMPOSE_FILE
  EVAL_PATH
  REPORT_PATH
EOF
}

die() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[$(date '+%F %T')] $*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

START_MILVUS=0
SKIP_PREP=0
SKIP_TELEMETRY=0
SKIP_INDEX=0
SKIP_EVAL=0
LIMIT=0
KS="1,3,5"
TIMEOUT_MS=30000
COLLECTION=""
DATASET_ROOT_RAW=""
OUTPUT_ROOT_RAW=""
ENV_FILE_RAW=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start-milvus)
      START_MILVUS=1
      ;;
    --skip-prep)
      SKIP_PREP=1
      ;;
    --skip-telemetry)
      SKIP_TELEMETRY=1
      ;;
    --skip-index)
      SKIP_INDEX=1
      ;;
    --skip-eval)
      SKIP_EVAL=1
      ;;
    --limit)
      shift
      [[ $# -gt 0 ]] || die "--limit requires a value"
      LIMIT="$1"
      ;;
    --ks)
      shift
      [[ $# -gt 0 ]] || die "--ks requires a value"
      KS="$1"
      ;;
    --timeout-ms)
      shift
      [[ $# -gt 0 ]] || die "--timeout-ms requires a value"
      TIMEOUT_MS="$1"
      ;;
    --collection)
      shift
      [[ $# -gt 0 ]] || die "--collection requires a value"
      COLLECTION="$1"
      ;;
    --dataset-root)
      shift
      [[ $# -gt 0 ]] || die "--dataset-root requires a value"
      DATASET_ROOT_RAW="$1"
      ;;
    --output-root)
      shift
      [[ $# -gt 0 ]] || die "--output-root requires a value"
      OUTPUT_ROOT_RAW="$1"
      ;;
    --env-file)
      shift
      [[ $# -gt 0 ]] || die "--env-file requires a value"
      ENV_FILE_RAW="$1"
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
  shift
done

require_cmd docker
require_cmd realpath

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(realpath -m "$SCRIPT_DIR/../..")}"
GO_IMAGE="${GO_IMAGE:-golang:1.24}"
PYTHON_IMAGE="${PYTHON_IMAGE:-python:3.12-slim}"
GO_MOD_CACHE="${GO_MOD_CACHE:-/opt/opscaptain/go-cache/mod}"
GO_BUILD_CACHE="${GO_BUILD_CACHE:-/opt/opscaptain/go-cache/build}"
PIP_CACHE="${PIP_CACHE:-/opt/opscaptain/pip-cache}"
MILVUS_ADDRESS="${MILVUS_ADDRESS:-127.0.0.1:19530}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-opscaptain-rag}"
COMPOSE_FILE="${COMPOSE_FILE:-$WORKSPACE_ROOT/manifest/docker/docker-compose.remote.yml}"

DATASET_ROOT="${DATASET_ROOT_RAW:-$WORKSPACE_ROOT/aiopschallenge2025}"
OUTPUT_ROOT="${OUTPUT_ROOT_RAW:-$DATASET_ROOT/baseline}"
ENV_FILE="${ENV_FILE_RAW:-/opt/opscaptain/.env.production}"
COLLECTION="${COLLECTION:-${MILVUS_COLLECTION:-aiops_evidence_telemetry_build}}"
EVAL_PATH="${EVAL_PATH:-$OUTPUT_ROOT/eval/eval_cases_holdout_related.jsonl}"
REPORT_PATH="${REPORT_PATH:-$OUTPUT_ROOT/eval/report_evidence_telemetry_build_related.json}"

container_path() {
  local raw="$1"
  local abs
  abs="$(realpath -m "$raw")"
  case "$abs" in
    "$WORKSPACE_ROOT")
      printf '.'
      ;;
    "$WORKSPACE_ROOT"/*)
      printf './%s' "${abs#"$WORKSPACE_ROOT"/}"
      ;;
    *)
      die "path must stay under workspace root: $abs"
      ;;
  esac
}

mkdir -p "$OUTPUT_ROOT" "$GO_MOD_CACHE" "$GO_BUILD_CACHE" "$PIP_CACHE"

[[ -d "$WORKSPACE_ROOT" ]] || die "workspace root not found: $WORKSPACE_ROOT"
[[ -f "$DATASET_ROOT/input.json" ]] || die "missing dataset file: $DATASET_ROOT/input.json"
[[ -f "$DATASET_ROOT/groundtruth.jsonl" ]] || die "missing dataset file: $DATASET_ROOT/groundtruth.jsonl"
[[ -d "$DATASET_ROOT/extracted" ]] || die "missing dataset directory: $DATASET_ROOT/extracted"

if [[ "$SKIP_INDEX" -eq 0 || "$SKIP_EVAL" -eq 0 ]]; then
  [[ -f "$ENV_FILE" ]] || die "missing env file: $ENV_FILE"
fi

REL_DATASET_ROOT="$(container_path "$DATASET_ROOT")"
REL_OUTPUT_ROOT="$(container_path "$OUTPUT_ROOT")"
REL_EVAL_PATH="$(container_path "$EVAL_PATH")"
REL_REPORT_PATH="$(container_path "$REPORT_PATH")"
REL_DOCS_BUILD_DIR="$(container_path "$OUTPUT_ROOT/docs_evidence_telemetry_build")"

run_go() {
  local inner="$1"
  local docker_args=(
    run --rm --network host
    -e GOPROXY=https://goproxy.cn,direct
    -e GOSUMDB=sum.golang.google.cn
    -e MILVUS_ADDRESS="$MILVUS_ADDRESS"
    -e MILVUS_COLLECTION="$COLLECTION"
    -v "$WORKSPACE_ROOT:/work"
    -v "$GO_MOD_CACHE:/go/pkg/mod"
    -v "$GO_BUILD_CACHE:/root/.cache/go-build"
    -w /work
  )
  if [[ -n "$ENV_FILE" ]]; then
    docker_args+=(--env-file "$ENV_FILE")
  fi
  docker "${docker_args[@]}" "$GO_IMAGE" sh -lc "$inner"
}

run_python() {
  local inner="$1"
  docker run --rm --network host \
    -v "$WORKSPACE_ROOT:/work" \
    -v "$PIP_CACHE:/root/.cache/pip" \
    -w /work \
    "$PYTHON_IMAGE" \
    sh -lc "python -m pip install --no-input -q pandas pyarrow && $inner"
}

wait_for_health() {
  local container_name="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-5}"
  local status=""
  for _ in $(seq 1 "$attempts"); do
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_name" 2>/dev/null || true)"
    if [[ "$status" == "healthy" || "$status" == "running" ]]; then
      return 0
    fi
    sleep "$sleep_seconds"
  done
  docker ps --filter "name=$container_name" || true
  die "container did not become healthy: $container_name (last status: ${status:-unknown})"
}

if [[ "$START_MILVUS" -eq 1 ]]; then
  [[ -f "$COMPOSE_FILE" ]] || die "missing compose file: $COMPOSE_FILE"
  log "starting dedicated Milvus"
  (
    cd "$(dirname "$COMPOSE_FILE")"
    BASELINE_WORKSPACE_ROOT="$WORKSPACE_ROOT" \
      docker compose -p "$COMPOSE_PROJECT_NAME" -f "$(basename "$COMPOSE_FILE")" up -d etcd minio standalone
  )
  wait_for_health milvus-etcd 30 3
  wait_for_health milvus-minio 30 3
  wait_for_health milvus-standalone 60 5
fi

if [[ "$SKIP_PREP" -eq 0 ]]; then
  log "running aiops_rag_prep_cmd"
  run_go "go run ./internal/ai/cmd/aiops_rag_prep_cmd -dataset-root $REL_DATASET_ROOT -output-root $REL_OUTPUT_ROOT"
fi

if [[ "$SKIP_TELEMETRY" -eq 0 ]]; then
  log "running telemetry evidence builder"
  PY_CMD="python scripts/aiops/build_telemetry_evidence.py --dataset-root $REL_DATASET_ROOT --output-root $REL_OUTPUT_ROOT"
  if [[ "$LIMIT" != "0" ]]; then
    PY_CMD="$PY_CMD --limit $LIMIT"
  fi
  run_python "$PY_CMD"
fi

if [[ "$SKIP_INDEX" -eq 0 ]]; then
  log "indexing telemetry build docs into collection $COLLECTION"
  run_go "go run ./internal/ai/cmd/knowledge_cmd -dir $REL_DOCS_BUILD_DIR"
fi

if [[ "$SKIP_EVAL" -eq 0 ]]; then
  log "running strict holdout evaluation"
  run_go "go run ./internal/ai/cmd/rag_online_eval_cmd -eval $REL_EVAL_PATH -ks $KS -timeout-ms $TIMEOUT_MS -out $REL_REPORT_PATH"
fi

log "final telemetry report"
if [[ -f "$OUTPUT_ROOT/telemetry/telemetry_report.json" ]]; then
  cat "$OUTPUT_ROOT/telemetry/telemetry_report.json"
fi

if [[ -f "$REPORT_PATH" ]]; then
  log "evaluation summary"
  python3 - "$REPORT_PATH" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
data = json.loads(path.read_text(encoding="utf-8"))
summary = data["summary"]
print("cases =", summary["cases"])
print("avg_total_ms =", summary["avg_total_latency_ms"])
print("empty_rate =", summary["empty_rate"])
print("hit@1/3/5 =", summary["hit_rate_at_k"]["1"], summary["hit_rate_at_k"]["3"], summary["hit_rate_at_k"]["5"])
print("recall@1/3/5 =", summary["avg_recall_at_k"]["1"], summary["avg_recall_at_k"]["3"], summary["avg_recall_at_k"]["5"])
PY
fi

log "done"
