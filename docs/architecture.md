# OpsCaptain Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (React/Vite)                    │
│  AgentWorkbenchView ─ ChatInput ─ GoSBeliefProgress ─ ResultCard│
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP / SSE
┌────────────────────────────▼────────────────────────────────────┐
│                     GoFrame HTTP Server                         │
│  CORS → Auth(JWT) → RateLimit(Redis) → Metrics → Tracing(OTel) │
└────┬───────────┬───────────┬───────────┬────────────────────────┘
     │           │           │           │
     ▼           ▼           ▼           ▼
┌─────────┐ ┌──────────┐ ┌────────┐ ┌──────────┐
│ Chat API│ │ AIOps API│ │ Upload │ │ Approval │
│ 4 modes │ │ sync+async│ │ Index  │ │  Gate    │
└────┬────┘ └────┬─────┘ └───┬────┘ └──────────┘
     │           │            │
     ▼           ▼            ▼
┌─────────────────────────────────────────────────────────────────┐
│                      AI Service Layer                           │
│  chat_pipeline / ai_ops_service / indexing_service / approval   │
└────┬───────────────────────┬────────────────────────────────────┘
     │                       │
     ▼                       ▼
┌──────────────┐    ┌──────────────────────────────────────────┐
│  RAG Pipeline │    │           AI Engines                     │
│ ┌──────────┐ │    │  ┌───────────────┐ ┌─────────────────┐  │
│ │ Embedder │ │    │  │ Plan-Execute  │ │  GoS Belief     │  │
│ │ (Doubao) │ │    │  │   -Replan     │ │    Engine        │  │
│ └──────────┘ │    │  │ (Eino-based)  │ │ (FSM + experts) │  │
│ ┌──────────┐ │    │  └───────────────┘ └─────────────────┘  │
│ │ Milvus   │ │    └──────────────────────────────────────────┘
│ │ BM25+Vec │ │
│ └──────────┘ │    ┌──────────────────────────────────────────┐
└──────────────┘    │           Tool System                     │
                    │  query_log(MCP) │ query_metrics(Prometheus)│
                    │  query_internal_docs(RAG) │ mysql_crud    │
                    └──────────────────────────────────────────┘
```

## Component Details

### 1. HTTP Layer

GoFrame v2 server. Routes defined via `g.Meta` struct tags. Middleware chain:
- **Global**: OpenTelemetry tracing, Prometheus metrics
- **Per-group**: CORS, JWT auth (optional), Redis rate limit, response formatting

### 2. Chat Pipeline

The main conversational flow, orchestrated by an Eino graph:

```
Orchestration → [Tools Node] → Prompt Construction → LLM Call → Response
```

- **Context Engine**: intent recognition → tool recall/reranking → history recall → context assembly
- **Prompt System**: base system prompt + identity rules + evidence rules + skill preferences + safety warnings
- **Modes**: quick (synchronous), stream (SSE), submit (async queue), aiops (engine-driven)

### 3. AI Engines

#### Plan-Execute-Replan

Uses Eino's `planexecute` pattern:
1. **Planner**: creates a step list from the query
2. **Executor**: runs each step (tool calls, analysis)
3. **Replanner**: adjusts the plan after each step
4. Max 5 iterations, then forces a final report

#### GoS Belief Engine

Custom graph-of-specialists engine:
1. **Ingest**: parse symptoms into candidate hypothesis nodes
2. **Plan**: select which experts to dispatch
3. **Act**: run experts concurrently (via errgroup, limit 3)
4. **Update Graph**: attach evidence to belief graph (copy-on-write)
5. **FSM Decision**: continue drilling down or report
6. Loop until FSM reaches final state or max steps

### 4. Expert System

Three domain experts, each with a behavioral contract:

| Expert | Tool | Evidence Type |
|--------|------|---------------|
| Metrics | query_prometheus_alerts | Real-time alerts, capacity signals |
| Logs | query_logs (MCP) | Structured log evidence, error traces |
| Knowledge | query_internal_docs (RAG) | SOP, runbook, historical cases |

Each expert follows Must/MustNot/EvidencePolicy rules defined in `contracts/contracts.go`.

### 5. RAG Pipeline

```
Indexing: File → MarkdownSplitter → Doubao Embedding → Milvus Insert
Retrieval: Query → BM25 + Vector Search (parallel) → RRF Fusion → Rerank → TopK
```

- Hybrid search: BM25 lexical + Milvus vector, fused with RRF (k=60)
- Query rewrite: expand user query into optimized search terms
- Retriever pool: singleton cached retriever with failure backoff

### 6. Safety Layers

```
User Input → Prompt Guard (regex) → Injection Classifier (LLM) → Agent Execution
                                                                    ↓
User Output ← Output Filter (regex) ← LLM Validator ← Agent Response
```

- **Prompt Guard**: regex blocks known injection patterns
- **Injection Classifier**: LLM-based scoring (0-1), configurable threshold
- **Output Filter**: redacts system prompts, API keys, internal IPs
- **ApprovalGate**: human-in-the-loop for high-risk operations

### 7. Memory System

- **Memory Agent**: LLM decides which conversation facts to persist
- **Scopes**: session → user → project → global
- **Operations**: skip, upsert, supersede (correct existing), promote (expand scope)
- **Storage**: Redis with TTL

### 8. Infrastructure

| Component | Role |
|-----------|------|
| Redis | Rate limiting, task state, approval queue, token audit, LLM cache, memory |
| RabbitMQ | Async chat task queue, memory extraction queue |
| MySQL | AI tool for read-only queries (GORM) |
| Milvus | Vector storage for RAG (HNSW index, IP metric) |
| Prometheus | Metrics collection + alerting |

### 9. Frontend Architecture

React + TypeScript SPA with glass morphism design:
- **State**: `useChat.ts` (sessions, messages, streaming, AIOps), `useTheme.ts`, `useFileUpload.ts`
- **Views**: ChatView, AgentWorkbenchView (with GoSBeliefProgress, EvidenceBlock, ResultCard)
- **Engine VM**: transforms backend AIOps responses into UI state (steps, confidence, evidence)
- **Persistence**: localStorage for sessions
