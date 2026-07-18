GO ?= go
GOFLAGS ?= -count=1
OBSERVABILITY_COMPOSE ?= deploy/observability/docker-compose.yml

.PHONY: all fmt vet test test-race lint build clean observability-up observability-down observability-status observability-check

all: fmt vet lint test build

fmt:
	@echo "==> gofmt"
	@$(GO) fmt ./...

vet:
	@echo "==> go vet"
	@$(GO) vet ./...

lint: fmt vet

test:
	@echo "==> go test"
	@$(GO) test $(GOFLAGS) ./...

test-race:
	@echo "==> go test -race"
	@$(GO) test $(GOFLAGS) -race ./...

test-cover:
	@echo "==> go test -coverprofile"
	@$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

build:
	@echo "==> go build"
	@$(GO) build -o bin/superbizagent .

clean:
	@rm -rf bin/ coverage.out

# === Local observability ===

observability-up:
	@docker compose -f $(OBSERVABILITY_COMPOSE) up -d

observability-down:
	@docker compose -f $(OBSERVABILITY_COMPOSE) down

observability-status:
	@docker compose -f $(OBSERVABILITY_COMPOSE) ps

observability-check:
	@python3 -m unittest deploy/test_local_k8s_log_mcp.py
	@docker compose -f $(OBSERVABILITY_COMPOSE) config --quiet

# === Eval ===

eval-gate:
	@echo "==> Eval Gate (确定性回归检查)"
	@$(GO) run cmd/gos_eval/main.go --mode=gate --gos-profile=eval \
	  --baseline=evals/baselines/gos_baseline.json \
	  --output=evals/reports/gate_$$(date +%Y%m%d%H%M%S).json

eval-runs-gos:
	@echo "==> Export GoS/Diag runs"
	@$(GO) run cmd/gos_eval/main.go --mode=export-runs --gos-profile=eval \
	  --output-dir=evals/runs

eval-judge:
	@echo "==> LLM Judge 评分"
	@$(GO) run cmd/gos_eval/main.go --mode=judge \
	  --input=evals/runs --output-dir=evals/reports

eval-compression:
	@echo "==> Context Compression Eval (audit + optimize)"
	@$(GO) run ./internal/ai/cmd/context_compression_eval_cmd \
	  -input evals/context_compression/samples.jsonl \
	  -mode audit,optimize \
	  -out evals/runs/compression_eval.json

eval-compression-audit:
	@echo "==> Context Compression Audit (仅采集数据，不压缩)"
	@$(GO) run ./internal/ai/cmd/context_compression_eval_cmd \
	  -input evals/context_compression/samples.jsonl \
	  -mode audit \
	  -out evals/runs/compression_audit.json

ci: fmt vet test-race test-cover build eval-gate
	@echo "==> CI pipeline complete"
