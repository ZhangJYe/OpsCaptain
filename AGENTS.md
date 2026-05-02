# AGENTS.md — OpsCaption Agent 协作规则

本文件是所有 AI Agent（Trae、Copilot、Cursor、OpsCaption 自身 Agent 等）在本项目中工作时的唯一行为约束。
Agent 每次启动时应自动加载本文件。犯错时由人类更新本文件，形成反馈循环。
本文件优先级高于任何外部 Wiki、Slack、飞书文档。

---

## 项目概述

- 项目名称：OpsCaption / OpsCaptionAI
- 模块名：SuperBizAgent（go.mod module name）
- 定位：面向 AIOps 场景的智能运维助手，具备 ReAct 对话、Plan-Execute-Replan 运维分析、RAG 知识检索、故障诊断能力
- 当前主链路：
  - Chat：用户输入 → ContextEngine / MemoryService → Eino ReAct Agent → Tools / RAG → JSON / SSE 输出
  - AIOps：用户输入 → Approval / Degradation / Memory → Runtime → Plan-Execute-Replan → 输出分析结论
- 目标用户：内部运维团队
- 当前阶段：P1 完成，正在做 RAG baseline 评测与 Harness Engineering

## 新窗口快速背景

如果是新开的 Codex / Cursor / Claude / Trae 会话，先按下面这段理解项目，不要重新猜架构：

- 这是一个 Go 后端为主的 AIOps Agent 项目，不是纯前端 demo。
- 核心目标是让运维人员用自然语言描述告警、日志、指标异常或知识库问题，系统自动组织上下文、调用工具/RAG，并输出带证据的诊断建议。
- 当前有效主链路只有两条：
  - Chat：ReAct Agent + Tools + RAG + Memory + SSE。
  - AIOps：Plan-Execute-Replan，用于更结构化的运维分析。
- `chat_multi_agent` 已废弃并移除，不要再把 supervisor / triage / reporter 当成当前聊天入口。
- 后端能力重点：GoFrame HTTP 控制器、Eino Agent 编排、Redis 治理、RabbitMQ 异步任务、Milvus RAG、MySQL/文件持久化、Prometheus/Jaeger 可观测性、SSE 流式响应。
- 记忆系统通过 `MemoryService` 封装，聊天结果持久化和记忆抽取不能裸调用底层 `utility/mem`。
- RAG 优化必须先看 baseline / holdout 评测，不要只凭单次问答效果判断。
- 所有设计口径以本文件和 `Learn/system/` 下当前文档为准，旧的 multi-agent 学习稿只作为历史材料。

## 线上部署与验证

- 线上服务器：`124.222.57.178`
- 线上域名：`https://opscaptain.top/ai/`
- SSH 用户：`root@124.222.57.178`
- 部署目录：`/opt/opscaptain`
- 生产编排：`docker-compose.prod.yml` + `.env.production` + `release.env`
- 注意：执行 compose 命令前需要加载 `release.env`，否则 `BACKEND_IMAGE` / `FRONTEND_IMAGE` 变量会缺失。

常用只读验证命令：

```bash
ssh root@124.222.57.178
cd /opt/opscaptain
set -a; . ./release.env; set +a
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl -sS http://127.0.0.1/ai/healthz
curl -sS http://127.0.0.1/ai/readyz
docker logs --since=10m opscaptain-backend-1
docker logs --since=10m opscaptain-caddy-1
```

对外健康检查：

```bash
curl -k https://opscaptain.top/ai/healthz
curl -k https://opscaptain.top/ai/readyz
```

---

## 技术栈

- Go 1.24+，GoFrame v2 (github.com/gogf/gf/v2)
- LLM 框架：cloudwego/eino v0.7+ / cloudwego/eino-ext
- LLM 模型：DeepSeek V3（推理/rewrite/rerank）、Doubao Embedding（向量化）
- 向量数据库：Milvus (milvus-sdk-go/v2)
- 前端：React + TypeScript（SuperBizAgentFrontend/），历史静态前端代码仍在仓库中
- 数据预处理：Python 3.11+（pandas、pyarrow）
- 配置管理：manifest/config/config.yaml
- CI/CD：GitHub Actions（.github/workflows/ci.yml、cd.yml）

---

## 当前有效架构

### Chat 路径

- 控制器入口：`internal/controller/chat/chat_v1_chat.go` / `chat_v1_chat_stream.go`
- 执行核心：`internal/ai/agent/chat_pipeline/`
- 模式：Eino ReAct Agent + ProgressiveDisclosure + ContextEngine + MemoryService
- 输出：普通 JSON 响应或 SSE 流式事件

### AIOps 路径

- 服务入口：`internal/ai/service/ai_ops_service.go`
- Runtime 包装：`internal/ai/runtime/`
- 执行核心：`internal/ai/agent/plan_execute_replan/`
- 模式：Plan-Execute-Replan
- 说明：当前保留的是 AIOps runtime 壳 + Plan-Execute-Replan 执行核心，不再存在 chat multi-agent 路由

### 上下文工程

- ContextAssembler 统一装配 history / memory / docs / tool outputs
- ContextProfile 和 ContextBudget 做预算控制
- ContextTrace 记录装配细节

### RAG 链路

- Query Rewrite → RetrieverPool (Milvus) → Rerank (LLM) → Evidence Assembly
- 当前正在做 AIOps baseline 评测和 telemetry evidence 预处理

### 历史/实验代码

- `internal/ai/agent/supervisor/`
- `internal/ai/agent/triage/`
- `internal/ai/agent/reporter/`
- `internal/ai/agent/skillspecialists/`

这些目录目前不作为当前主链路的设计依据。除非用户明确要求恢复或研究历史方案，否则不要再把它们当作现行架构来描述或重新接回聊天入口。

---

## 目录结构

```
internal/ai/agent/chat_pipeline/       → Chat ReAct 执行链路
internal/ai/agent/plan_execute_replan/ → AIOps Plan-Execute-Replan
internal/ai/agent/                     → 其他 agent/历史实验代码
internal/ai/skills/                    → skill 抽象和 registry
internal/ai/protocol/                  → 统一协议
internal/ai/runtime/                   → AIOps Runtime（registry/ledger/bus/artifacts）
internal/ai/rag/                       → RAG 链路（query/rewrite/rerank/retriever_pool）
internal/ai/rag/eval/                  → RAG 评测（baseline/runner/online）
internal/ai/contextengine/             → 上下文装配和 budget 管理
internal/ai/service/                   → 服务层（memory/approval/ai_ops）
internal/ai/models/                    → 模型初始化
internal/ai/embedder/                  → embedding 管理
internal/ai/retriever/                 → Milvus retriever
internal/ai/cmd/                       → CLI 入口（knowledge_cmd/rag_eval_cmd/rag_online_eval_cmd 等）
internal/controller/                   → HTTP 控制器
utility/                               → 公共工具（auth/mem/metrics/tracing/health/client/common）
manifest/config/                       → 配置文件
res/                                   → 历史经验与复盘文档
todo/                                  → 设计文档与执行计划
Learn/                                 → 学习笔记与设计稿
scripts/                               → 数据预处理脚本（Python + PowerShell + Bash）
aiopschallenge2025/                    → AIOps 故障案例数据集
SuperBizAgentFrontend/                 → 前端
```

---

## 代码规范

- 不加注释，除非明确要求
- 错误处理统一走 `ResultStatusDegraded`，不直接 fatal
- 新增执行链路或 tool 前先补最小 replay / 回归 case
- 配置项不硬编码，走 `config.yaml`
- 不在 `main.go` 里内联工具函数
- 所有新能力都要进入配置：预算、top_k、timeouts、rerank、feature flags
- commit message 用中文
- 不主动 commit，除非用户明确要求
- 所有新增文档默认使用中文
- 不创建不必要的文件
- 优先编辑已有文件，而不是新建文件

---

## 禁止事项

- 不要把 `groundtruth` 标签直接当实时证据喂给模型
- 不要把原始 parquet 直接入向量库，必须先预处理成 serving docs
- 不要把历史标签和实时证据混在同一层，必须分层
- 不要暴露或日志记录 secrets 和 keys
- 不要提交大体积数据集、缓存目录、临时日志
- 不要删除 `res/` 或 `todo/` 下的重要历史文档
- 不要假设任何第三方库可用，先检查 `go.mod` 或 `package.json`
- 不要重新接回 `chat_multi_agent` 关键词路由
- 不要让聊天链路重新依赖 `supervisor/triage/reporter/skillspecialists`
- 不要把 memory 直接拼进 routing 判断，memory 只能作为执行上下文，不替代原始 query

---

## 历史踩坑规则

每一条规则对应一个真实发生过的失败案例。

- `Reporter` 首字母大写用自定义函数 `displayAgentName`，不用 `strings.Title`。`strings.Title` 已废弃且对 Unicode 不稳定。（问题 4，§9）
- env file 加载走 `utility/common.LoadEnvFile`，不在 `main.go` 内联写。原先逻辑不便复用且 scanner error 没处理。（问题 6，§11）
- Revoked token 吊销表必须有自动过期清理，否则长期运行内存泄漏。已补 `clearExpiredRevokedTokens` + 后台清理协程。（问题 1，§6）
- Long-term memory 必须有全局上限和 per-session 上限，否则内存无限增长。通过 `config.yaml` 配置 `memory.long_term_max_entries`。（问题 2，§7）
- `LongTermMemory.Retrieve` 应使用 `RLock` 而非写锁，只在更新 `AccessCnt/LastUsed` 时短暂获取写锁。（问题 9，§14）
- CORS 默认策略不能过宽。`allowed_origins` 为空时不应原样回写 Origin。SSE 不能无条件设 `*`。统一走 `ResolveAllowedOrigin`。（问题 7，§12）
- 知识库检索 `top_k` 不要 hardcode 3，走 config 读取 `multi_agent.knowledge_evidence_limit`。（问题 8，§13）
- AI Ops 不要每次请求重建 runtime，走 `getOrCreateAIOpsRuntime` 按 dataDir 复用。（§22.1）
- `task_completed` 事件不要把完整 Markdown 报告塞进 `Message`，长摘要折叠并把 `status/summary_length` 放 `Payload`。（§24.2）
- `query_internal_docs` 不要每次新建 retriever，按 Milvus 地址和 top_k 复用，失败走短 TTL 缓存。（§24.3）
- Chat 和 ChatStream 的 memory 持久化统一走 `MemoryService.PersistOutcome`，不裸起 `go ExtractMemories(context.Background())`。（§23.2）
- 异步记忆抽取必须有 timeout 保护，用 `context.WithoutCancel + context.WithTimeout`，配置项 `memory.extract_timeout_ms`。（§21.1）
- 记忆写入前必须做基础过滤：assistant boilerplate、代码块、异常长度内容应丢弃。走 `ExtractMemoryCandidates + ValidateMemoryCandidate`。（§27.2）
- RAG chunking 不能只按 Markdown 标题切，要支持 JSONL case 级切分。当前 `transformer.go` 偏 Markdown 结构。
- observation 不能只保留一个关键词。`context canceled` 不是完整观察，`paymentservice + error log + context canceled` 才有区分度。
- 图谱关系必须标记来源和置信度（`source_type/derivation_type/extractor_version`），否则后续无法 debug。
- `CASE_SIMILAR_TO_CASE` 边必须记录 `similarity_score`、`similarity_components`、`computed_by`。
- build split 和 eval split 必须严格分开，不能拿全量数据自证效果。
- metric score 阈值 `< 0.1 and abs(delta) < 0.5` 是当前硬编码，后续需按 metric 类型调整。
- PowerShell here-string 中 shell 变量要避免和 PS 变量名冲突。用 `probe_status` 而非 `$status`。

---

## 测试要求

- `go test ./internal/ai/...` 必须通过
- `go test ./utility/...` 必须通过
- `go test ./...` 必须通过
- `go build ./...` 必须通过
- 新增 skill 必须有对应 registry 单测
- 涉及 AIOps runtime 或 Plan-Execute-Replan 变更时，必须补对应 runtime / replay case
- RAG eval 必须有 build/holdout split
- Python 脚本运行前确认 pandas、pyarrow 已安装

---

## 当前进行中的工作

- AIOps baseline 评测：telemetry evidence 预处理脚本（`scripts/aiops/build_telemetry_evidence.py`）
- RAG 优化：待 baseline 结果出来后决定优先级
- 知识图谱设计：已完成设计稿（`Learn/graph/00.md`），待 baseline 后再决定是否实施
- Harness Engineering：P0 落地中

---

## 关键设计决策记录

- Chat 已收敛到 ReAct 单链路，不再做 `chat_multi_agent` 条件路由。
- AIOps 保留单体内 runtime，而执行核心收敛为 Plan-Execute-Replan。
- Ledger 和 Artifact Store 先用文件存储，不增加额外部署依赖。（§17.1）
- Memory Service 封装 `utility/mem`，不直接重写底层。（§17.2）
- 历史 `skillspecialists` / `supervisor` / `triage` / `reporter` 代码保留为实验或复盘材料，不再作为当前聊天架构依据。
- 图谱设计以 Case Graph 为中心，不做百科式知识图谱。（`Learn/graph/00.md`）
- 证据与结论分层：原始证据 / 归一化事实 / 历史标签 / 图谱 / Serving。
- 上下文工程覆盖四类主来源：history / memory / docs / tool outputs。（§27-29）

---

## 文档索引

- 历史修改与复盘：`res/todo.md`
- Harness Engineering 落地说明：`res/harness-engineering-for-opscaptionai.md`
- 复盘手册：`res/next-phase-retrospective-playbook.md`
- RAG 工程设计：`todo/rag-engineering-complete-design.md`
- 三阶段执行计划：`todo/three-phase-execution-plan.md`
- 知识图谱设计：`Learn/graph/00.md`
- Harness 学习指南：`Learn/harness/harness-p0-agents-md.md`
