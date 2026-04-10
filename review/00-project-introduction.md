# OpsCaptain 项目介绍 — 评审汇报稿

---

## 开场 (30 秒)

各位好，我今天给大家介绍的项目是 **OpsCaptain**，一个 AI 驱动的智能运维平台。

项目核心解决的问题是：**当生产环境出现告警或故障时，运维人员需要在多个系统之间来回切换查数据、翻文档、查日志，效率低且容易遗漏。** OpsCaptain 用 AI Agent 将这些操作串联起来，自动完成 "查告警 → 查知识库 → 查日志 → 根因分析 → 输出报告" 的完整流程。

---

## 一、项目定位与功能 (2 分钟)

### 1.1 项目是什么

OpsCaptain 是一个面向运维团队的 AI 助手，提供三个核心能力：

| 能力 | 用户场景 | AI 范式 |
|------|---------|--------|
| **智能对话 (Chat)** | 运维人员通过自然语言提问，系统自动调用工具获取数据并回答 | ReAct Agent (推理-行动循环) |
| **深度分析 (AIOps)** | 一键触发告警分析，系统自动规划分析步骤、执行查询、评估结果 | Plan-Execute-Replan (规划-执行-重规划) |
| **知识管理** | 上传运维文档，自动向量化索引，支持语义检索 | RAG (检索增强生成) |

### 1.2 一个典型使用场景

> 凌晨 3 点收到 CPU 告警，值班工程师打开 OpsCaptain：
> 1. 点击 **AIOps 分析** → 系统自动查 Prometheus 告警、检索知识库 SOP、查日志
> 2. 30 秒后输出完整分析报告：告警详情 + 根因分析 + 处理建议
> 3. 如果需要进一步确认，在 **Chat** 中追问："这个告警之前出现过吗？上次怎么处理的？"
> 4. 系统结合之前的分析记忆 + 知识库文档回答

---

## 二、技术架构 (3 分钟)

### 2.1 技术栈

| 层级 | 技术选型 | 选型理由 |
|------|---------|---------|
| 语言 | **Go 1.24** | 高性能、强类型、适合后端服务 |
| Web 框架 | **GoFrame v2** | 国产成熟框架，自动路由绑定、参数校验 |
| AI Agent 框架 | **Eino (字节跳动)** | 原生 Go 支持、内置 ReAct/Plan-Execute-Replan 范式 |
| LLM | **GLM-4.5-AIR (智谱)** | OpenAI 兼容接口，成本可控 |
| 向量数据库 | **Milvus** | 开源、高性能向量检索 |
| 缓存/限流 | **Redis** | 响应缓存、Token 审计、降级开关、分布式限流 |
| 业务数据 | **MySQL** (GORM) | 工具查询用 |
| 可观测性 | **OpenTelemetry + Prometheus + Jaeger** | 全链路追踪、指标监控 |
| 部署 | **Docker Compose + Caddy** | 一键部署，自动 HTTPS |
| CI/CD | **GitHub Actions** | CI (lint/test/build) + CD (Docker 远程部署) |

### 2.2 整体架构

```
┌────────────────────────────────────────────────────────────┐
│                     用户 (浏览器)                            │
└──────────────────────┬─────────────────────────────────────┘
                       │
                       ▼
┌────────────────────────────────────────────────────────────┐
│  Gateway 层: Caddy (HTTPS) → GoFrame Server                │
│  中间件链: Tracing → Metrics → CORS → JWT Auth → RateLimit │
└──────────────────────┬─────────────────────────────────────┘
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   ┌─────────┐   ┌──────────┐   ┌──────────┐
   │  Chat   │   │  AIOps   │   │  Upload  │
   │ ReAct   │   │ Plan-Exe │   │  知识索引 │
   │ Agent   │   │ -Replan  │   │ Pipeline │
   └────┬────┘   └────┬─────┘   └────┬─────┘
        │              │              │
        └──────┬───────┘              │
               ▼                      ▼
        ┌──────────────┐      ┌──────────────┐
        │  Tools 工具集 │      │  Milvus 索引 │
        │ Prometheus   │      └──────────────┘
        │ MySQL / Docs │
        │ Logs (MCP)   │
        └──────────────┘
               │
               ▼
        ┌──────────────────────────┐
        │  LLM: GLM-4.5-AIR (智谱) │
        └──────────────────────────┘
```

### 2.3 两种 AI 范式的区别

**Chat — ReAct Agent:**
```
用户问题 → LLM 思考 → 需要调工具吗？
  ├── 是 → 调工具 → 拿到结果 → 继续思考 (最多 25 轮)
  └── 否 → 直接回复
```
- 适合：日常问答、单次查询
- 特点：LLM 自主决策调哪个工具

**AIOps — Plan-Execute-Replan:**
```
用户问题 → Planner 制定计划 → Executor 逐步执行 → Replanner 评估
  ├── 数据够了 → 输出最终报告
  └── 不够 → 调整计划，继续执行 (最多 5 轮迭代)
```
- 适合：复杂故障排查、需要多步分析
- 特点：先规划后执行，结果可追溯

---

## 三、项目规模与质量 (1 分钟)

### 3.1 代码规模

| 指标 | 数值 |
|------|------|
| Go 源文件 | **155 个** |
| 代码行数 | **约 17,700 行** |
| 测试文件 | **44 个** |
| 测试通过 | **24 个包全部 PASS** |
| 内部模块 | 20+ 模块 (在用 14 个 + 保留 6 个) |
| 外部依赖 | 8 个核心组件 (LLM / Milvus / Redis / MySQL / Prometheus / MCP / Jaeger / MinIO) |

### 3.2 质量保障

- **安全多层防护**: Prompt Guard (6 种注入检测) → Output Filter → JWT Auth → RBAC → Rate Limit
- **优雅降级**: Redis/Config 双源 Kill Switch，任何时刻可一键切断 AI 调用
- **成本控制**: Token 审计 + 每日限额，防止 LLM 调用失控
- **审批流**: 含 delete/rollback/restart 等关键词的操作自动拦截，需人工审批
- **可观测性**: OpenTelemetry 全链路追踪 + Prometheus 指标 + 结构化日志

---

## 四、核心设计亮点 (2 分钟)

### 4.1 Context Engine — 四阶段上下文组装

AI 回答质量 = 上下文质量。系统独立设计了四阶段上下文组装引擎：

```
Stage 1: History (对话历史)     Token Budget: 2000
Stage 2: Memory (长期记忆)      Token Budget: 800
Stage 3: Documents (RAG 文档)   Token Budget: 1200
Stage 4: Tool Results (工具结果) Token Budget: 600
```

- 每阶段独立 Token 预算，不会互相挤占
- 按 Mode 切换不同 Profile (Chat 侧重历史，AIOps 侧重文档和记忆)
- 单阶段失败不影响其他阶段

### 4.2 双层记忆系统

```
短期记忆 (SimpleMemory): 当前会话的对话历史
长期记忆 (LongTermMemory): 跨会话的关键信息 (异步提取)
```

- Chat 和 AIOps 共享记忆池
- 先做 Chat 对话 → 切到 AIOps 分析 → 分析结果写入记忆 → 再 Chat 时能引用分析历史

### 4.3 高危操作审批门控

```
用户请求 "帮我 rollback 服务 A"
  → Approval Gate 检测到高危关键词
  → 请求入队等待审批
  → 管理员审批后自动执行
```

### 4.4 Knowledge Index Agent 化

知识索引 Pipeline (FileLoader → MarkdownSplitter → MilvusIndexer) 既可以作为独立 ETL 直接调用，也实现了 `runtime.Agent` 接口，可被 Runtime 调度——为后续多 Agent 协同预留了扩展点。

---

## 五、架构演进与保留设计 (1 分钟)

项目内保留了一套完整的 **Multi-Agent Runtime** 基础设施（但当前未连接到 Chat 链路）：

| 保留组件 | 说明 |
|---------|------|
| Runtime | Agent 注册、Task 分发、Ledger 事件追踪、持久化存储 |
| Supervisor | 编排器: Triage → 并行 Specialists → Reporter |
| Triage | 意图分类 (规则匹配) |
| Specialists | 三个专家 Agent: Metrics / Logs / Knowledge |
| Reporter | 结果聚合报告 |
| Skills | Skill 注册表和匹配框架 |

**为什么保留？**
- 当前 Chat 用 ReAct 足够，AIOps 用 Plan-Execute-Replan 效果更好
- Multi-Agent 体系代码完整、有测试，随时可重新接入
- 后续计划集成 LLM 到 Multi-Agent 中，作为 Chat 的增强模式 (用户可在前端切换)

---

## 六、部署与 CI/CD (30 秒)

```
开发 → Push → GitHub Actions CI (fmt/vet/test/build)
                   │
                   ▼ (main 分支)
            GitHub Actions CD
                   │
                   ▼
          Docker Build → Push → 远程部署 (docker-compose)
                                   │
                                   ▼
                   Caddy (HTTPS) + Backend + Frontend + Milvus + Redis
```

- 一个 `docker-compose.prod.yml` 管理所有服务
- Caddy 自动 HTTPS 证书
- Prometheus + AlertManager 配置已包含在 `deploy/` 目录

---

## 七、项目目录结构总览 (1 分钟)

```
OpsCaptain/
├── api/chat/v1/                     # API 请求/响应定义
├── internal/
│   ├── controller/chat/             # Controller 层 (HTTP 入口)
│   ├── ai/
│   │   ├── agent/
│   │   │   ├── chat_pipeline/       # Chat ReAct Agent
│   │   │   ├── plan_execute_replan/ # AIOps Plan-Execute-Replan
│   │   │   ├── knowledge_index_pipeline/ # 知识索引 + Agent 壳
│   │   │   ├── supervisor/          # (保留) 编排器
│   │   │   ├── triage/              # (保留) 意图分类
│   │   │   ├── skillspecialists/    # (保留) 三个专家 Agent
│   │   │   └── reporter/           # (保留) 报告聚合
│   │   ├── service/                 # Service 层 (业务编排)
│   │   ├── contextengine/           # 上下文组装引擎
│   │   ├── rag/                     # RAG 检索 + 索引
│   │   ├── runtime/                 # (保留) Multi-Agent Runtime
│   │   ├── tools/                   # LLM 工具 (Prometheus/MySQL/Docs/Logs/Time)
│   │   └── protocol/               # Agent 通信协议
│   └── consts/                      # 常量定义
├── utility/
│   ├── auth/                        # JWT + RBAC + Rate Limiter
│   ├── safety/                      # Prompt Guard + Output Filter
│   ├── mem/                         # 双层记忆系统
│   ├── cache/                       # LLM 响应缓存
│   ├── resilience/                  # 断路器 + 信号量
│   ├── metrics/                     # Prometheus 指标
│   ├── tracing/                     # OpenTelemetry 追踪
│   └── health/                      # 健康检查探针
├── deploy/                          # 部署配置 (Docker Compose / Caddy / Prometheus)
├── .github/workflows/               # CI/CD (ci.yml + cd.yml)
└── review/                          # 评审文档
```

---

## 八、总结 (30 秒)

| 维度 | 总结 |
|------|------|
| **解决的问题** | 运维故障排查效率低 → AI 自动化查数据 + 分析 + 出报告 |
| **技术方案** | Go + Eino Agent 框架，Chat (ReAct) + AIOps (Plan-Execute-Replan) 双范式 |
| **工程质量** | 17,700 行代码，44 个测试文件，24 个包全 PASS，多层安全防护 |
| **设计亮点** | 四阶段上下文引擎、双层记忆、高危审批、优雅降级、Agent 化设计 |
| **扩展性** | 完整 Multi-Agent Runtime 保留，随时可升级为多智能体增强模式 |

---

## 附录: 预备 Q&A

**Q: 为什么选 Go 而不是 Python？**
A: Go 编译型语言，运行时性能好、部署简单 (单二进制)；Eino 框架原生 Go 支持 ReAct 和 Plan-Execute-Replan；GoFrame 是成熟的企业级 Web 框架。

**Q: 为什么用 GLM 而不是 GPT/Claude？**
A: GLM-4.5-AIR 提供 OpenAI 兼容接口，成本更低，国内网络环境友好。通过 Eino 的 OpenAI 适配层，切换模型只需改配置。

**Q: 为什么 Chat 和 AIOps 用不同的 AI 范式？**
A: Chat 是交互式对话，用户问一句答一句，ReAct 的"边想边做"更自然。AIOps 是一次性深度分析，需要先规划再执行，Plan-Execute-Replan 的分步迭代更可靠、结果更完整。

**Q: Multi-Agent 为什么保留但不用？**
A: 当前规则驱动的 Multi-Agent (零 LLM) 效果不如 LLM 驱动的 ReAct/Plan-Execute-Replan。保留代码是因为后续计划集成 LLM 到各个 Agent 中，作为 Chat 的增强模式。

**Q: 安全层面怎么考虑的？**
A: 五层防护：(1) Prompt Guard 检测注入攻击 (2) Output Filter 过滤敏感信息 (3) JWT Auth 身份验证 (4) RBAC 权限控制 (5) Rate Limit 防滥用。加上 Approval Gate 拦截高危操作。

**Q: 如果 LLM 挂了怎么办？**
A: 双源降级开关 (Redis + Config)，一键切断所有 AI 调用返回降级消息。同时有断路器模式防止雪崩。
