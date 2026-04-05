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

ci: fmt vet test-race test-cover build
	@echo "==> CI pipeline complete"
