# OpsCaptain - AI Ops Agent Workbench

## Project Overview

OpsCaptain is an AI-powered operations assistant that provides fault diagnosis, knowledge retrieval, and automated incident analysis. It features two AI engines: Plan-Execute-Replan (linear runbook) and GoS Belief Engine (graph-of-specialists with confidence scoring).

## Tech Stack

- **Backend**: Go 1.24, GoFrame v2, ByteDance Eino framework
- **Frontend**: React 18, TypeScript, Vite, Tailwind CSS, Framer Motion
- **AI**: Doubao LLM + Embedding, Milvus vector DB, MCP protocol
- **Infra**: Redis, RabbitMQ, MySQL, Prometheus, Docker, K8s

## Project Structure

```
OpsCaptain/
├── main.go                          # GoFrame server entry
├── api/chat/v1/                     # API request/response types (GoFrame routing)
├── internal/
│   ├── controller/chat/             # HTTP controllers
│   ├── ai/
│   │   ├── agent/
│   │   │   ├── chat_pipeline/       # Main chat orchestration + prompts
│   │   │   ├── gos_engine/          # GoS Belief Engine
│   │   │   ├── plan_execute_replan/ # Plan-Execute-Replan engine
│   │   │   ├── experts/             # Expert agent implementations
│   │   ├── skills/                  # Skill registry and progressive disclosure
│   │   │   └── domains/             # Domain skill definitions (metrics/logs/knowledge)
│   │   │   └── contracts/           # Agent behavioral contracts
│   │   ├── rag/                     # RAG: indexing, retrieval, hybrid search
│   │   ├── contextengine/           # Intent recognition, tool recall, context assembly
│   │   ├── tools/                   # Built-in tools (query_log, metrics_alerts, mysql_crud)
│   │   ├── models/                  # LLM model wrappers
│   │   ├── embedder/                # Doubao embedding client
│   │   ├── belief/                  # Belief graph + FSM
│   │   ├── runtime/                 # Task runtime, ledger, artifact store
│   │   ├── service/                 # Business service layer (AIOps, chat, memory, approval)
│   │   ├── protocol/                # Shared protocol types
│   │   ├── indexer/                 # Milvus indexer
│   │   └── events/                  # Event emission, tool wrapper, LLM validator
│   └── utility/
│       ├── auth/                    # JWT, rate limiting
│       ├── cache/                   # LLM response cache
│       ├── client/                  # Milvus client setup
│       ├── mem/                     # Memory agent
│       ├── middleware/               # HTTP middleware (CORS, auth, metrics, tracing)
│       ├── resilience/              # Semaphore, circuit breaker
│       └── safety/                  # Prompt guard, injection classifier, output filter
├── prompts/                         # Centralized prompt management
├── docs/                            # Project documentation
├── deploy/                          # Docker, Prometheus, remote deploy
├── manifest/                        # K8s manifests, config files
├── skills/                          # Skill definition markdown files
├── frontend/           # React frontend (separate Vite project)
└── Learn/                           # Learning notes and tutorials
```

## Development Rules

### Go Backend

- Use GoFrame conventions: `g.Meta` tag defines routes, `g.DB()` for database, `g.Redis()` for cache
- API types go in `api/chat/v1/chat.go`, controllers in `internal/controller/chat/`
- All prompt strings should be extracted to `prompts/` and loaded at startup
- Prefer `errgroup` for concurrent expert execution over raw goroutines
- Use `context.Context` for cancellation propagation, never ignore it
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`, log before returning

### Frontend

- Components in `src/components/`, hooks in `src/hooks/`, types in `src/types/`
- Use `useChat.ts` for all chat/AIOps state management
- Glass morphism design system: `border border-white/60 bg-white/70 backdrop-blur-2xl`
- Sky-500 as primary accent color, amber for GoS engine theme
- Asymmetric border radius: `rounded-[22px] rounded-bl-[6px]` for assistant bubbles

### Prompts

- All system prompts live in `prompts/` as markdown files
- Dynamic sections (date, documents, safety warnings) are templated with `{variable}` placeholders
- Each prompt file includes metadata: purpose, used_by, variables, version

### Safety

- Never log or expose API keys, tokens, or internal IPs
- Prompt injection guard is mandatory on all user inputs
- Output filter redacts system prompt blocks, API keys, internal IPs
- ApprovalGate must be checked before any AIOps execution (sync and async)

## Common Commands

```bash
# Backend
go build ./...                     # Build all
go test ./...                      # Run tests
go run main.go                     # Start server

# Frontend
cd frontend
npm run dev                        # Dev server
npm run build                      # Production build

# Knowledge indexing
go run internal/ai/cmd/knowledge_cmd/main.go -dir ./docs

# Docker
docker build -t opsagent .
docker compose -f deploy/docker-compose.prod.yml up -d
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
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
| GET | /api/approval_requests | Query approval requests |

## Key Design Decisions

1. **Two-engine architecture**: Plan for linear runbook-style, GoS for complex multi-hypothesis analysis
2. **Async kickoff pattern**: `/ai_ops_runs` returns trace_id immediately, frontend polls `/ai_ops_trace` for progress
3. **Hybrid RAG**: BM25 + vector search with RRF fusion for best recall
4. **Agent contracts**: Each expert has Must/MustNot/EvidencePolicy rules to prevent hallucination
5. **Safety layers**: Prompt guard -> Injection classifier -> Output filter -> ApprovalGate
