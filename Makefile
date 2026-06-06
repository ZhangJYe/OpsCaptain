GO ?= go
GOFLAGS ?= -count=1

.PHONY: all fmt vet test test-race lint build clean

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

ci: fmt vet test-race test-cover build eval-gate
	@echo "==> CI pipeline complete"
