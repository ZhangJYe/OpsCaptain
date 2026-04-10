# OpsCaptain (SuperBizAgent) 项目架构评审

> 评审日期: 2026-04-08 (更新)
> 评审方法: SART (Situation-Action-Result-Test)

---

## 一、项目总览

OpsCaptain (模块名 `SuperBizAgent`) 是一个 **AI 驱动的智能运维 (AIOps) 平台**，核心功能包括：
- 智能对话 (Chat) —— 基于 ReAct Agent + LLM 的运维问答
- AI 运维分析 (AIOps) —— 基于 Plan-Execute-Replan 范式的告警分析与故障排查
- 知识管理 —— RAG 知识检索 + 知识索引 Pipeline (支持 Agent 化调度)
- 审批流 —— 高危操作的人工审批门控

技术栈: **Go 1.24** + GoFrame v2 + Eino (字节跳动 AI Agent 框架) + Milvus (向量库) + Redis + MySQL + Prometheus + OpenTelemetry + Docker Compose

---

## 二、整体架构图 (SART)

### Situation (现状)
系统采用经典三层架构，前后端分离部署。Chat 和 AIOps 使用不同的 AI 范式。

### Action (架构设计)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        External Layer                                   │
│  ┌──────────┐   ┌───────┐   ┌───────────┐   ┌───────────────────────┐  │
│  │ Browser  │──▶│ Caddy │──▶│ Frontend  │   │  Prometheus / Jaeger  │  │
│  │ / Client │   │(HTTPS)│   │ (Nginx)   │   │  (Observability)      │  │
│  └──────────┘   └───┬───┘   └───────────┘   └───────────────────────┘  │
│                     │                                                   │
└─────────────────────┼───────────────────────────────────────────────────┘
                      │ /api/*
┌─────────────────────┼───────────────────────────────────────────────────┐
│                     ▼           API Gateway Layer                       │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                   GoFrame HTTP Server                            │   │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────┐ ┌───────────────────┐  │   │
│  │  │ Tracing  │ │ Metrics  │ │   CORS    │ │   Auth (JWT)      │  │   │
│  │  │Middleware│ │Middleware│ │ Middleware │ │   + Rate Limit    │  │   │
│  │  └──────────┘ └──────────┘ └───────────┘ └───────────────────┘  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                  Controller Layer (chat.ControllerV1)            │   │
│  │  ┌─────────┐ ┌──────────────┐ ┌──────────┐ ┌──────────────┐    │   │
│  │  │  Chat   │ │  ChatStream  │ │  AIOps   │ │  Admin APIs  │    │   │
│  │  │ (POST)  │ │   (SSE)      │ │  (POST)  │ │ (Audit/Appr.)│    │   │
│  │  └────┬────┘ └──────┬───────┘ └────┬─────┘ └──────┬───────┘    │   │
│  │       │              │              │              │             │   │
│  │  ┌────▼──────────────▼──────────────▼──────────────▼─────────┐  │   │
│  │  │             Prompt Guard / Output Filter                  │  │   │
│  │  │             (Safety Module)                               │  │   │
│  │  └──────────────────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                      │
┌─────────────────────┼───────────────────────────────────────────────────┐
│                     ▼        Service / AI Layer                         │
│                                                                         │
│  ┌─────────────────────────┐    ┌────────────────────────────────────┐  │
│  │   Chat (ReAct Agent)    │    │   AIOps (Plan-Execute-Replan)      │  │
│  │                         │    │                                    │  │
│  │  ┌───────────────────┐  │    │  ┌──────────────────────────────┐  │  │
│  │  │ Eino Graph:       │  │    │  │ Planner (LLM: GLM-4.5-AIR)  │  │  │
│  │  │ UserMessage       │  │    │  │   ▼                          │  │  │
│  │  │  ─▶ Template      │  │    │  │ Executor (LLM + Tools)      │  │  │
│  │  │  ─▶ ReAct Agent   │  │    │  │   ▼                          │  │  │
│  │  │     (LLM + Tools) │  │    │  │ Replanner (LLM)             │  │  │
│  │  │     最多 25 轮     │  │    │  │   最多 5 轮迭代             │  │  │
│  │  └───────────────────┘  │    │  └──────────────────────────────┘  │  │
│  └─────────────────────────┘    └────────────────────────────────────┘  │
│                                                                         │
│  ┌─────────────────────────┐    ┌────────────────────────────────────┐  │
│  │  Memory Service         │    │  Knowledge Index Pipeline          │  │
│  │  ┌───────────────────┐  │    │  (ETL + Agent 壳)                  │  │
│  │  │ Context Engine    │  │    │                                    │  │
│  │  │ (Assembler)       │  │    │  FileLoader → MDSplitter           │  │
│  │  │ - History         │  │    │  → MilvusIndexer                   │  │
│  │  │ - Memory          │  │    │                                    │  │
│  │  │ - Documents       │  │    │  实现 runtime.Agent 接口           │  │
│  │  │ - Tool Results    │  │    │  可被 Runtime 调度                 │  │
│  │  └───────────────────┘  │    └────────────────────────────────────┘  │
│  │  ┌───────────────────┐  │                                           │
│  │  │ Token Budget      │  │    ┌────────────────────────────────────┐  │
│  │  │ Management        │  │    │  Cross-cutting                     │  │
│  │  └───────────────────┘  │    │  ┌──────────────────────────────┐  │  │
│  └─────────────────────────┘    │  │ Degradation (Kill Switch)   │  │  │
│                                 │  ├──────────────────────────────┤  │  │
│  ┌─────────────────────────┐    │  │ Approval Gate (Human Review)│  │  │
│  │  Multi-Agent Runtime    │    │  ├──────────────────────────────┤  │  │
│  │  (保留未连接 Chat)      │    │  │ Token Audit (Cost Control)  │  │  │
│  │  Supervisor / Triage    │    │  └──────────────────────────────┘  │  │
│  │  Specialists / Reporter │    └────────────────────────────────────┘  │
│  └─────────────────────────┘                                           │
└─────────────────────────────────────────────────────────────────────────┘
                      │
┌─────────────────────┼───────────────────────────────────────────────────┐
│                     ▼      Infrastructure / Tool Layer                  │
│                                                                         │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐   │
│  │  GLM-4.5-AIR │ │  Prometheus  │ │   Milvus     │ │    Redis     │   │
│  │  (LLM)      │ │  (Alerts)    │ │  (Vector DB) │ │  (Cache/Mem) │   │
│  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐                    │
│  │  MySQL       │ │  MCP Server  │ │  Jaeger      │                    │
│  │  (CRUD)      │ │  (Log Query) │ │  (Tracing)   │                    │
│  └──────────────┘ └──────────────┘ └──────────────┘                    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Result (架构特征)

| 架构特征 | 描述 |
|---------|------|
| **前后端分离** | 前端 (HTML/JS/CSS) 通过 Nginx 服务，后端 Go API 通过 Caddy 反向代理 |
| **Chat 单 Agent** | Chat 统一走 ReAct Agent (Eino)，LLM 自主决策工具调用 |
| **AIOps Plan-Execute-Replan** | AIOps 使用 LLM 规划→执行→重规划的迭代范式 |
| **知识索引 Agent 化** | 知识索引 Pipeline 实现 `runtime.Agent` 接口，可被 Runtime 调度 |
| **优雅降级** | 全局 Kill Switch (Redis/Config) + 单步超时降级 |
| **安全多层** | Prompt Guard + Output Filter + JWT Auth + Rate Limit + RBAC |
| **可观测性** | OpenTelemetry + Prometheus Metrics + Jaeger Tracing + pprof |
| **Multi-Agent Runtime** | 保留完整的运行时基础设施 (Ledger/Event/Artifact)，当前未连接 Chat 链路 |

### Test (验证点)

- [ ] `GET /healthz` 返回 200 且 `{"ok": true}`
- [ ] `GET /readyz` 在 Milvus/Redis 就绪时返回 200
- [ ] `POST /api/chat` 统一走 ReAct Agent，返回 `mode: "chat"`
- [ ] `POST /api/chat_stream` 走 SSE 流式 ReAct Agent，返回 `mode: "chat"`
- [ ] `POST /api/ai_ops` 走 Plan-Execute-Replan，返回分析报告
- [ ] `POST /api/ai_ops` 包含高危关键词时返回 `approval_required: true`
- [ ] 设置 `degradation.kill_switch: true` 后所有 AI 接口返回降级响应
- [ ] Prometheus `/metrics` 端点暴露 HTTP 请求延迟

---

## 三、组件清单

### 3.1 外部组件 (Infrastructure)

| 组件 | 用途 | 接入方式 |
|------|------|---------|
| GLM-4.5-AIR (智谱) | LLM 推理 | OpenAI 兼容 HTTP API via Eino SDK |
| Milvus | 向量存储与检索 (RAG) | gRPC via milvus-sdk-go |
| Redis | 缓存、Token 审计、降级开关、限流 | GoFrame Redis 插件 |
| MySQL | 业务数据 CRUD | GORM + go-sql-driver |
| Prometheus | 告警查询 | HTTP API `/api/v1/alerts` |
| MCP Server | 日志查询 | MCP 协议 (eino-ext/tool/mcp) |
| Jaeger | 分布式追踪 | OpenTelemetry Exporter |
| MinIO / etcd | Milvus 依赖存储 | Docker Compose 内部 |

### 3.2 内部模块

| 模块 | 路径 | 职责 | 状态 |
|------|------|------|------|
| API 定义 | `api/chat/v1/` | GoFrame 请求/响应结构体 | 在用 |
| Controller | `internal/controller/chat/` | HTTP 路由处理、安全检查、输出过滤 | 在用 |
| AIOps Service | `internal/ai/service/ai_ops_service.go` | AIOps 编排 (Plan-Execute-Replan) | 在用 |
| Plan-Execute-Replan | `internal/ai/agent/plan_execute_replan/` | LLM 规划→执行→重规划 Agent | 在用 (AIOps) |
| Chat Pipeline | `internal/ai/agent/chat_pipeline/` | ReAct Agent (Eino Graph) | 在用 (Chat) |
| Knowledge Index Pipeline | `internal/ai/agent/knowledge_index_pipeline/` | 知识索引 ETL + Agent 壳 | 在用 |
| Context Engine | `internal/ai/contextengine/` | 上下文组装 (History + Memory + Documents) | 在用 |
| RAG | `internal/ai/rag/` | 检索增强生成 (Retriever Pool / Query / Indexing) | 在用 |
| Tools | `internal/ai/tools/` | LLM 可调用工具 (Prometheus / MySQL / 文档 / 日志 / 时间) | 在用 |
| Memory Service | `internal/ai/service/memory_service.go` | 上下文组装 + 记忆持久化 | 在用 |
| Memory | `utility/mem/` | 会话记忆 (Short-term + Long-term) | 在用 |
| Auth | `utility/auth/` | JWT + RBAC + Rate Limiter | 在用 |
| Safety | `utility/safety/` | Prompt Guard + Output Filter | 在用 |
| Observability | `utility/metrics/`, `utility/tracing/`, `utility/logging/` | 指标、追踪、日志 | 在用 |
| Resilience | `utility/resilience/` | 断路器、信号量 | 在用 |
| Health | `utility/health/` | 就绪/存活探针 | 在用 |
| Cache | `utility/cache/` | LLM 响应缓存 | 在用 |
| Multi-Agent Runtime | `internal/ai/runtime/` | Agent 注册、Task 分发、Ledger 追踪 | **保留，未连接 Chat** |
| Multi-Agent Chat Service | `internal/ai/service/chat_multi_agent.go` | Multi-Agent Chat 编排 | **保留，未连接 Chat** |
| Supervisor / Triage | `internal/ai/agent/supervisor/`, `triage/` | 多智能体编排和意图分类 | **保留，未连接 Chat** |
| Skill Specialists | `internal/ai/agent/skillspecialists/` | Metrics/Logs/Knowledge 专家 | **保留，未连接 Chat** |
| Reporter | `internal/ai/agent/reporter/` | 结果聚合报告 | **保留，未连接 Chat** |
| Skills | `internal/ai/skills/` | Skill 注册表和匹配框架 | **保留，未连接 Chat** |

---

## 四、部署拓扑

```
┌─────────────────────────────────────────────────────┐
│                 Production (Docker Compose)          │
│                                                      │
│  ┌──────────┐      ┌──────────┐      ┌──────────┐   │
│  │  Caddy   │─443──▶│ Frontend │      │  Jaeger  │   │
│  │ (HTTPS/  │      │ (Nginx)  │      │  :16686  │   │
│  │  Proxy)  │─/api─▶│          │      └──────────┘   │
│  │ :80/:443 │      └──────────┘                      │
│  └────┬─────┘                                        │
│       │ /api/*                                       │
│       ▼                                              │
│  ┌──────────┐                                        │
│  │ Backend  │─────▶ GLM-4.5-AIR (External)           │
│  │  :8000   │─────▶ Prometheus (External/Internal)   │
│  │          │─────▶ Redis (External)                 │
│  │          │─────▶ Milvus (Internal/External)       │
│  │          │─────▶ MySQL (External)                 │
│  │          │─────▶ MCP Server (External)            │
│  └──────────┘                                        │
│                                                      │
│  Volumes: docs-data, runtime-data, caddy-data/config │
└─────────────────────────────────────────────────────┘
```

**CI/CD**: GitHub Actions (`.github/workflows/ci.yml` + `cd.yml`)
- CI: fmt → vet → test-race → coverage → build
- CD: Docker 镜像构建 + 远程部署 (`deploy/remote-deploy.sh`)

---

## 五、数据流总览

```
User Request
     │
     ▼
[Caddy Reverse Proxy]
     │
     ▼
[GoFrame Server] ──▶ Middleware Chain:
     │                Tracing → Metrics → CORS → Auth → RateLimit → Response
     │
     ▼
[Controller] ──▶ Prompt Guard ──▶ Degradation Check
     │
     ├──(/api/chat)──▶ MemoryService.BuildChatPackage()
     │                       │
     │                       ▼
     │                  Context Engine (Assembler)
     │                       │
     │                       ▼
     │                  Eino Graph: InputToChat → Template → ReAct Agent
     │                       │
     │                       ▼
     │                  LLM (GLM-4.5-AIR) + Tool Calls:
     │                  Prometheus / MySQL / Docs / Logs / Time
     │                       │
     │                       ▼
     │                  Cache Store + PersistOutcome
     │
     ├──(/api/ai_ops)──▶ ApprovalGate.Check()
     │                       │
     │                       ▼
     │                  MemoryService.BuildContextPlan()
     │                       │
     │                       ▼
     │                  Plan-Execute-Replan:
     │                  Planner (LLM) → Executor (LLM + Tools)
     │                  → Replanner (LLM) → 最多 5 轮迭代
     │                       │
     │                       ▼
     │                  PersistOutcome
     │
     ├──(/api/upload)──▶ Knowledge Index Pipeline:
     │                  FileLoader → MDSplitter → MilvusIndexer
     │
     ▼
[Controller] ──▶ Output Filter
     │
     ▼
HTTP Response (JSON / SSE)
```
