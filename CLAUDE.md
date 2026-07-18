# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpsCaptain is an AI-powered operations assistant for fault diagnosis, knowledge retrieval, and automated incident analysis. It features two AI engines: Plan-Execute-Replan (linear runbook) and GoS Belief Engine (graph-of-specialists with confidence scoring).

## Tech Stack

- **Backend**: Go 1.24, GoFrame v2, ByteDance Eino framework
- **Frontend**: React 18, TypeScript, Vite, Tailwind CSS, GSAP
- **AI**: DeepSeek V4 Pro/Flash, Doubao Embedding Vision (2048dim), Milvus vector DB, MCP protocol
- **Infra**: Redis, RabbitMQ, MySQL, Prometheus, Jaeger, Docker

## Common Commands

```bash
# Backend
go build ./...                          # Build all
go test ./...                           # Run all tests
go test -run TestFunctionName ./path/   # Run single test
go run main.go                          # Start server

# Makefile targets
make test                               # Run tests with -count=1
make test-race                          # Run tests with race detector
make build                              # Build binary to bin/superbizagent
make eval-compression                   # Run context compression eval
make ci                                 # Full CI pipeline (fmt, vet, test, build, eval)

# Frontend
cd frontend
npm run dev                             # Dev server
npm run build                           # Production build

# Knowledge indexing (default: indexes /app/knowledge_seed)
docker compose -f deploy/docker-compose.prod.yml run --rm knowledge-indexer

# Docker
docker build -t opsagent .
docker compose -f deploy/docker-compose.prod.yml up -d
```

## Architecture

Five-layer architecture with strict import rules:

```
Controller Layer    internal/controller/    Parameter parsing → call Application → format response
Application Layer   internal/app/           Business orchestration: ChatApp, AIOpsApp, KnowledgeApp
Domain Layer        internal/ai/            Core business: retrieval, reasoning, evidence, Agent
Infrastructure      internal/infra/         External system adapters: Milvus, RabbitMQ, Redis
Common Layer        utility/                Cross-cutting concerns: auth, rate limiting, safety, health
```

Import rules (target state, no new exceptions allowed):
- `controller/ → app/` ✅
- `app/ → ai/` ✅
- `app/ → infra/` ✅ (via interface only)
- `ai/ → infra/` ❌ (must inject via interface)
- `infra/ → ai/` ❌
- `utility/ → internal/` ❌
- `RAG package must not import infra/milvus directly` (has import guard test)

See AGENTS.md section 3 for registered exceptions; do not add new ones.

### Key Modules

| Module | Path | Purpose |
|--------|------|---------|
| Chat Pipeline | `internal/ai/agent/chat_pipeline/` | ReAct Agent orchestration, prompt assembly |
| GoS Engine | `internal/ai/agent/gos_engine/` | Belief graph + FSM diagnosis engine |
| Plan-Execute-Replan | `internal/ai/agent/plan_execute_replan/` | Linear runbook execution |
| Experts | `internal/ai/agent/experts/` | LinuxSRE, NetworkSRE, DatabaseSRE agents |
| Skills | `internal/ai/skills/` | Skill registry, progressive disclosure (AlwaysOn/SkillGate/OnDemand) |
| RAG | `internal/ai/rag/` | Hybrid retrieval (Dense + BM25 + RRF), indexing |
| Context Engine | `internal/ai/contextengine/` | Context assembly with token budget, intent recognition |
| Memory | `internal/ai/memory/` | Short-term (sliding window), long-term (persistent), extraction |
| Context Compression | `internal/ai/contextcompression/` | JSON/log compression with audit/optimize modes |
| Tools | `internal/ai/tools/` | Built-in tools + MCP integration with 3-level fallback |
| Events | `internal/ai/events/` | Tool wrapper, anti-hallucination pipeline, schema gate |
| Models | `internal/ai/models/` | LLM wrappers with instrumentation, circuit breaker, retry |

### Request Flow

```
User → Controller → App → Service (Memory + Context Assembly)
                            → Agent (ReAct or Plan-Execute-Replan)
                              → Tools (Prometheus/Logs/RAG/MySQL)
                            → Response + Memory Persistence
```

### Progressive Disclosure (Tool Tiers)

- **TierAlwaysOn**: get_current_time, query_internal_docs, MCP log tools, prometheus_alerts
- **TierSkillGate**: prometheus_metrics_discovery, prometheus_range/instant_query, user MCP tools
- **TierOnDemand**: mysql_crud (requires whitelist config)

When injection risk is detected, only AlwaysOn tools are exposed.

## Development Rules

### Go Backend

- Use GoFrame conventions: `g.Meta` tag defines routes
- **Controller must not directly access DB/Redis/Milvus** — only parse params and map responses; access infra through app/service/infra layers
- API types go in `api/chat/v1/`, controllers in `internal/controller/chat/`
- All prompt strings should be extracted to `prompts/` and loaded at startup
- Prefer `errgroup` for concurrent expert execution over raw goroutines
- Use `context.Context` for cancellation propagation, never ignore it
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`, log before returning
- Error degradation: use `ResultStatusDegraded` instead of fatal/crash
- Tool failures return formatted strings (not Go error) to avoid Eino framework retry loops
- New tools must have schema, timeout, permission bounds, degradation behavior, and tests
- New capabilities must be configurable (budget / top_k / timeout / feature flag)
- RAG/Agent/ContextEngine changes must run corresponding package tests

### Frontend

- Components in `src/components/`, hooks in `src/hooks/`, types in `src/types/`
- Use `useChat.ts` for all chat/AIOps state management
- GSAP for complex animations and scroll effects, Framer Motion for simple transitions
- Glass morphism design system: `border border-white/60 bg-white/70 backdrop-blur-2xl`
- Sky-500 as primary accent color, amber for GoS engine theme

### Safety

- Prompt injection guard is mandatory on all user inputs (regex + optional LLM classifier)
- Output filter redacts system prompt blocks, API keys, internal IPs
- ApprovalGate distinguishes "analysis" (allowed) from "execution" (requires approval)
- MCP tools have CIDR whitelist validation
- SQL injection protection: regex blacklist + table whitelist + subquery禁用 + auto LIMIT

### Commit Rules

- Commit messages must be in Chinese
- Never commit/push unless explicitly requested
- Run `go test ./...` and `npm run build` before pushing
- Always `git pull --rebase` before pushing
- Do not restore `chat_multi_agent` route (deprecated architecture)

### Memory Capture

- When stable project knowledge is discovered, update Claude Code project memory with a concise note
- Only write memory when it is useful for future coding/review/debugging and backed by source paths or command results
- Prefer updating existing memory over creating new files; deduplicate before writing
- Each memory entry must include `last_updated`, summary, authoritative source paths, usage guidance, and freshness caveat when needed
- Store only indexes and high-density summaries for large docs; keep detailed content in repository docs
- Do not store secrets, credentials, private IPs, long document bodies, temporary TODOs, speculative conclusions, or routine test output

## API Endpoints (Key)

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/agent | Unified Agent entry (`auto` / `chat` / `aiops_diagnosis`, feature flag controlled; enabled in current local config) |
| POST | /api/chat | Synchronous chat |
| POST | /api/chat_stream | SSE streaming chat |
| POST | /api/chat_submit | Submit async chat task |
| GET | /api/chat_task | Query async task status |
| POST | /api/ai_ops | Synchronous AIOps analysis |
| POST | /api/ai_ops_runs | Async AIOps kickoff |
| GET | /api/ai_ops_result | Get AIOps result by trace_id |
| GET | /api/ai_ops_trace | Get AIOps trace events |
| POST | /api/upload | File upload for knowledge indexing |
| GET | /api/memories | Query memories |
| POST | /api/memories/action | Memory actions (delete, etc.) |
| POST | /api/memories/promote | Promote memory scope |
| GET | /api/approval_requests | Query approval requests |
| POST | /api/approval_requests/approve | Approve request |
| POST | /api/approval_requests/reject | Reject request |
| GET | /api/token_audit | Token usage audit |

Full route definitions in `api/chat/v1/*.go` (chat.go, user_tools.go, etc.).

## Config

All configuration in `manifest/config/config.yaml`. Key sections:
- `chat_model` / `chat_model_fast`: LLM model config
- `embedding_model`: Doubao embedding config
- `context_compression`: Compression settings (enabled/mode/min_tokens)
- `rag`: Retrieval config (top_k, rewrite_enabled, rerank_enabled)
- `memory`: Token budget, extraction settings
- `safety`: Prompt guard, output filter, injection classifier

## Multi-Instance Deployment

OpsCaptain 默认按单实例配置。扩展到多副本（K8s Deployment N=2+）必须显式开启以下配置项，
否则会出现状态分裂（trace 404、SSE 静默、对话历史丢失、token 翻倍等）：

### 必开（CRITICAL）

| 配置项 | 默认 | 多实例必须 | 作用 |
|--------|------|-----------|------|
| `aiops.ledger.backend` | `file` | `redis` | AIOps task/result/event 共享，避免 `submit on A → query on B → 404` |
| `memory.session_store.redis_enabled` | `false` | `true` | 短期对话跨实例共享，避免轮间换 Pod 丢上下文 |
| `change_events.cluster_broadcast.enabled` | `true` | `true` | SSE 通知跨实例 fan-out，确保所有客户端实时收到事件 |

RabbitMQ 单消费者（`chat-task` / `memory-extract`）默认通过 **Redis SETNX 去重**保护，
无需额外配置；只需保证 Redis 可达即可。

### 强烈建议（HIGH）

- **上传文件**：`filestore.UploadStore` 是接口，默认 `LocalUploadStore` 写本地磁盘。
  多副本必须挂同一个 PVC（ReadWriteMany NFS/EFS）到 `file_dir` 目录，或自行实现
  S3 适配（后续会提供）。否则上传到 A 的文件，B 上查询/索引会 404。
- **MCP 用户工具**（`internal/ai/skills/user_skill_store.go`）：当前用本地 JSON
  持久化，多实例下注册的工具只对一个实例生效。临时方案：挂共享卷；长期：迁 Redis（H2）。
- **BM25 索引**（`internal/ai/rag/shared_bm25.go`）：每个 pod 启动时扫本地
  `file_dir` 重建。多实例必须确保 `file_dir` 是共享卷，或把新增文档走外部 indexer job。

### Redis 是硬依赖

多实例部署下，下列功能 **全部依赖** Redis 共享后端，Redis 不可达时会：
- AIOps Ledger 回退到本地 file → 跨实例 404 重新出现
- Session store 回退到 in-process → 对话历史不跨实例
- Dedup 回退到 in-memory TTLSet → 同条 RabbitMQ delivery 可能被双消费
- SSE 跨实例广播失效 → 订阅者只能收到本实例 ingest 的事件
- Rate limiter 回退到 in-process → 总限流值 = 配置值 × 实例数（限制被放大）

建议在 K8s 健康探针中加 Redis 连通性检查，Redis 故障期间标记 Pod 不健康。

### 已知限制

- **变更事件 ringBuffer**（`internal/ai/changeevent/bus.go`）仅作本实例性能缓存；
  Query/Get 走 Redis（共享），但 `RecentByService`/`RecentAll` 只看本实例最近 200 条。
- **AIOps Artifact**（`internal/ai/runtime/artifacts.go`）仍走 file backend；
  artifact_id 跨实例不可见（H6 待迁 Redis）。
- **限流降级路径**（`utility/auth/rate_limiter.go`）：Redis 抖动时回退 in-memory，
  限制按实例独立计数。生产建议加监控告警。
