FROM golang:1.24-alpine AS builder

ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/superbizagent .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /build/superbizagent .
COPY --from=builder /build/manifest/config/config.yaml manifest/config/config.yaml

RUN mkdir -p /app/docs /app/docs/quarantine /app/var/runtime && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:8000/healthz || exit 1

ENTRYPOINT ["./superbizagent"]
