#!/usr/bin/env bash
set -euo pipefail
mkdir -p /opt/opscaptain/go-cache/mod /opt/opscaptain/go-cache/build
cd /opt/opscaptain/baseline-workspace

docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_telemetry_build_resume_20260412 \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  /usr/local/go/bin/go run ./internal/ai/cmd/knowledge_cmd -dir ./aiopschallenge2025/baseline/docs_evidence_telemetry_build

docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_telemetry_build_resume_20260412 \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  /usr/local/go/bin/go run ./internal/ai/cmd/rag_online_eval_cmd -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl -ks 1,3,5 -timeout-ms 30000 -out ./aiopschallenge2025/baseline/eval/report_evidence_telemetry_build_related.json
