# OpsCaptain 企业级加固实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复架构违规、补全配置、完善文档，使 OpsCaptain 成为可真正运行的企业级项目，并为 Go 初学者提供完整教程。

**Architecture:** 五层架构（Controller → App → Domain → Infra → Common），严格 import 规则。本次修复重点：消除 controller→ai/ 直连违规、修复 prod 配置违规、补全 .env.example、完善文档体系。

**Tech Stack:** Go 1.24, GoFrame v2, Eino framework, React 18, Docker, Redis, RabbitMQ, Milvus

---

## 审计结果摘要

| 类别 | 状态 | 数量 |
|------|------|------|
| ai/rag/ → infra/milvus (Rule 1) | ✅ 干净 | 0 |
| utility/ → internal/ (Rule 2) | ✅ 干净（仅已知例外） | 0 new |
| infra/ → ai/ (Rule 3) | ✅ 干净 | 0 |
| controller/ → ai/ (Rule 4) | ❌ 违规 | 16 (9 生产 + 7 测试) |
| ai/ → infra/ (interface 规则) | ❌ 违规 | 5 (3 生产 + 2 CLI) |
| prod config multi_agent | ❌ 违反 AGENTS.md | 1 |
| .env.example | ❌ 缺失 | 1 |
| getting-started.md | ❌ 缺失 | 1 |

---

## Task 1: 修复 prod 配置 multi_agent 违规

**Files:**
- Modify: `deploy/config.prod.yaml:136-139`

- [ ] **Step 1: 修复 multi_agent 配置**

将 `deploy/config.prod.yaml` 中 `multi_agent.enabled: true` 改为 `false`，`ai_ops_enabled: true` 改为 `false`，与 dev config 和 AGENTS.md 保持一致。

```yaml
# Before (line 136-139):
multi_agent:
  enabled: true
  data_dir: "/app/var/runtime"
  ai_ops_enabled: true

# After:
multi_agent:
  enabled: false
  data_dir: "/app/var/runtime"
  ai_ops_enabled: false
```

- [ ] **Step 2: 验证**

```bash
grep -A3 "multi_agent:" deploy/config.prod.yaml
```
Expected: `enabled: false` and `ai_ops_enabled: false`

- [ ] **Step 3: 提交**

```bash
git add deploy/config.prod.yaml
git commit -m "修复: prod 配置 multi_agent.enabled 改为 false，与 AGENTS.md 一致"
```

---

## Task 2: 创建 .env.example

**Files:**
- Create: `.env.example`

- [ ] **Step 1: 创建 .env.example**

从 `deploy/config.prod.yaml` 和 `manifest/config/config.yaml` 中提取所有 `${ENV_VAR}` 引用，创建完整的环境变量模板。

```bash
# ============================================
# OpsCaptain 环境变量配置模板
# 复制为 .env.local 并填入实际值
# ============================================

# --- 必填：AI 模型密钥 ---
DEEPSEEK_API_KEY=your-deepseek-api-key
ARK_API_KEY=your-ark-api-key

# --- 必填：认证 ---
AUTH_JWT_SECRET=your-jwt-secret-min-32-chars

# --- 可选：Redis（多实例部署必填） ---
REDIS_ADDRESS=127.0.0.1:6379
REDIS_PASSWORD=

# --- 可选：RabbitMQ ---
RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
RABBITMQ_USERNAME=guest
RABBITMQ_PASSWORD=guest

# --- 可选：Milvus 向量数据库 ---
MILVUS_ADDRESS=127.0.0.1:19530
MILVUS_COLLECTION=opscaption_knowledge

# --- 可选：MySQL ---
MYSQL_DSN=

# --- 可选：Prometheus ---
PROMETHEUS_ADDRESS=

# --- 可选：Jaeger 链路追踪 ---
JAEGER_ENDPOINT=

# --- 可选：MCP 日志集成 ---
MCP_LOG_URL=
MCP_LOG_HTTP_URL=

# --- 可选：日志主题（火山引擎） ---
LOG_TOPIC_REGION=
LOG_TOPIC_ID=

# --- 可选：CORS ---
CORS_ALLOWED_ORIGIN=

# --- 可选：成本控制 ---
DAILY_LIMIT_TOKENS=1000000
COST_HOURLY_ALERT_TOKENS=100000
COST_USER_DAILY_ALERT_TOKENS=50000

# --- 可选：Webhook 密钥 ---
OPSCAPTION_GITHUB_WEBHOOK_SECRET=
OPSCAPTION_GITLAB_WEBHOOK_SECRET=
OPSCAPTION_ARGOCD_WEBHOOK_SECRET=
```

- [ ] **Step 2: 确保 .gitignore 排除 .env.example 不被忽略**

检查 `.gitignore` 中 `.env.*` 规则不会排除 `.env.example`。当前 `.gitignore` 有 `.env.*`，需要添加排除规则。

在 `.gitignore` 的 `# Env / secrets` 部分添加：

```
# Env / secrets
.env
.env.*
!.env.example
```

- [ ] **Step 3: 验证**

```bash
git status .env.example  # 应显示 untracked，不是 ignored
```

- [ ] **Step 4: 提交**

```bash
git add .env.example .gitignore
git commit -m "新增: .env.example 环境变量模板，方便新开发者快速配置"
```

---

## Task 3: 修复 controller → ai/ import 违规（9 个生产文件）

**Files:**
- Modify: `internal/controller/chat/chat_new.go`
- Modify: `internal/controller/chat/chat_v1_change_events.go`
- Modify: `internal/controller/chat/chat_v1_mcp_tools.go`
- Modify: `internal/controller/chat/chat_v1_admin.go`
- Modify: `internal/controller/chat/chat_v1_user_skills.go`
- Modify: `internal/controller/chat/chat_v1_chat_task.go`
- Modify: `internal/controller/chat/response_helpers.go`
- Modify: `internal/controller/chat/chat_v1_ai_ops_incident.go`
- Modify: `internal/app/chat_app.go` (新增接口方法)

**策略：** 在 `internal/app/` 层新增接口方法，Controller 通过 App 层间接访问 ai/ 类型。不改变现有功能，只调整调用路径。

- [ ] **Step 1: 分析每个违规的 usage**

逐个读取 9 个违规文件，理解它们使用了 ai/ 包的哪些类型和函数：

| Controller 文件 | 违规 import | 使用的类型/函数 |
|----------------|-------------|----------------|
| `chat_new.go` | `ai/skills`, `ai/tools` | `skills.UserSkillStore`, `tools.DynamicMCPRegistry` |
| `chat_v1_change_events.go` | `ai/protocol` | `protocol.ChangeEvent` |
| `chat_v1_mcp_tools.go` | `ai/skills` | `skills.UserSkillStore`, `skills.MCPToolConfig` |
| `chat_v1_admin.go` | `ai/service` | `service.TokenAuditRecord` |
| `chat_v1_user_skills.go` | `ai/skills` | `skills.UserSkillStore`, `skills.SkillDefinition` |
| `chat_v1_chat_task.go` | `ai/service` | `service.ChatTaskStatus` |
| `response_helpers.go` | `ai/service` | `service.TokenAuditRecord` |
| `chat_v1_ai_ops_incident.go` | `ai/service` | `service.IncidentEvent` |

- [ ] **Step 2: 在 App 层新增类型别名和接口方法**

在 `internal/app/` 中新增或扩展接口，将 ai/ 类型重新导出给 Controller 使用。

```go
// internal/app/types.go (已存在，扩展)
// 将 ai/ 包的类型通过 app 层重新导出

// 从 ai/service 重新导出
type TokenAuditRecord = aiservice.TokenAuditRecord
type ChatTaskStatus = aiservice.ChatTaskStatus
type IncidentEvent = aiservice.IncidentEvent

// 从 ai/skills 重新导出
type UserSkillStore = skills.UserSkillStore
type MCPToolConfig = skills.MCPToolConfig
type SkillDefinition = skills.SkillDefinition

// 从 ai/protocol 重新导出
type ChangeEvent = protocol.ChangeEvent
```

- [ ] **Step 3: 修改 Controller 文件的 import**

将每个 Controller 文件的 import 从 `SuperBizAgent/internal/ai/...` 改为 `SuperBizAgent/internal/app`，使用 app 层的类型别名。

例如 `chat_new.go`:

```go
// Before:
import (
    "SuperBizAgent/internal/ai/skills"
    "SuperBizAgent/internal/ai/tools"
)

// After:
import (
    "SuperBizAgent/internal/app"
)
// 使用 app.UserSkillStore, app.DynamicMCPRegistry
```

- [ ] **Step 4: 运行测试验证**

```bash
go build ./...
go test ./internal/controller/chat/...
go test ./internal/app/...
```

- [ ] **Step 5: 提交**

```bash
git add internal/controller/ internal/app/
git commit -m "重构: controller 层通过 app 层间接访问 ai/ 类型，消除 import 违规"
```

---

## Task 4: 修复 ai/ → infra/ import 违规（3 个生产文件）

**Files:**
- Modify: `internal/ai/service/memory_queue.go`
- Modify: `internal/ai/service/chat_task_queue.go`
- Modify: `internal/app/` (新增 Queue 接口)

**策略：** 定义 Queue 接口在 ai/ 层，infra/rabbitmq 实现该接口，通过依赖注入传入。

- [ ] **Step 1: 在 ai/service 层定义 Queue 接口**

```go
// internal/ai/service/queue.go (新建)
package service

import "context"

// MessageQueue 定义消息队列的抽象接口
type MessageQueue interface {
    Publish(ctx context.Context, queueName string, body []byte) error
    Consume(ctx context.Context, queueName string) (<-chan []byte, error)
    Close() error
}
```

- [ ] **Step 2: 在 infra/rabbitmq 层实现接口**

检查 `internal/infra/rabbitmq/` 是否已有符合接口的方法，如有则无需修改。

- [ ] **Step 3: 修改 memory_queue.go 和 chat_task_queue.go**

将直接 import `internal/infra/rabbitmq` 改为使用 `MessageQueue` 接口，通过构造函数注入。

- [ ] **Step 4: 在 main.go 中注入实现**

```go
// main.go 中
var mq service.MessageQueue = rabbitmq.NewClient(...)
```

- [ ] **Step 5: 运行测试验证**

```bash
go build ./...
go test ./internal/ai/service/...
```

- [ ] **Step 6: 提交**

```bash
git add internal/ai/service/ internal/infra/rabbitmq/ main.go
git commit -m "重构: ai/service 层通过接口抽象消息队列，消除 infra 直连违规"
```

---

## Task 5: 修复 CLI 入口的 ai/ → infra/ 违规（2 个文件）

**Files:**
- Modify: `internal/ai/cmd/rag_online_eval_cmd/main.go`
- Modify: `internal/ai/cmd/recall_cmd/main.go`

**策略：** CLI 入口属于组装层，允许直接依赖 infra。将这 2 个文件加入 AGENTS.md 已知例外列表。

- [ ] **Step 1: 在 AGENTS.md 已知例外表中添加**

```markdown
| `internal/ai/cmd/rag_online_eval_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
| `internal/ai/cmd/recall_cmd/main.go` | `ai/ → infra/milvus` | CLI 入口，组装时需要 infra 适配 |
```

- [ ] **Step 2: 同时更新已清理的例外**

移除已不再存在的违规：
- `utility/health/health.go` — 已通过函数变量注入，不再直接 import ai/tools
- `utility/safety/injection_classifier.go` — 已通过工厂函数注入，不再直接 import ai/models

- [ ] **Step 3: 提交**

```bash
git add AGENTS.md
git commit -m "文档: 更新 AGENTS.md 已知例外列表，反映当前实际 import 状态"
```

---

## Task 6: 完善 Dockerfile 安全性

**Files:**
- Modify: `Dockerfile`
- Modify: `frontend/Dockerfile`

- [ ] **Step 1: 优化后端 Dockerfile 缓存层**

```dockerfile
# Before:
COPY . .

# After (更好的缓存):
COPY go.mod go.sum ./
RUN go mod download
COPY . .
```

- [ ] **Step 2: 前端 Dockerfile 添加非 root 用户和 healthcheck**

```dockerfile
# frontend/Dockerfile 添加：
USER nginx
HEALTHCHECK --interval=30s --timeout=3s CMD wget --spider http://localhost/ || exit 1
```

- [ ] **Step 3: 验证构建**

```bash
docker build -t opsagent-test .
docker build -t opsagent-frontend-test -f frontend/Dockerfile frontend/
```

- [ ] **Step 4: 提交**

```bash
git add Dockerfile frontend/Dockerfile
git commit -m "安全: 优化 Dockerfile 缓存层，前端添加非 root 用户和 healthcheck"
```

---

## Task 7: 添加 Docker Compose 日志轮转

**Files:**
- Modify: `deploy/docker-compose.prod.yml`

- [ ] **Step 1: 为所有服务添加日志轮转配置**

```yaml
# 在每个服务下添加：
logging:
  driver: "json-file"
  options:
    max-size: "50m"
    max-file: "3"
```

- [ ] **Step 2: 提交**

```bash
git add deploy/docker-compose.prod.yml
git commit -m "运维: Docker Compose 添加日志轮转配置，防止磁盘占满"
```

---

## Task 8: 创建 getting-started.md

**Files:**
- Create: `docs/getting-started.md`

- [ ] **Step 1: 编写 Getting Started 教程**

内容结构：
1. 项目简介（一句话概括）
2. 环境要求（Go 1.24+, Node 18+, Docker）
3. 快速启动（3 步：clone → 配置 → 运行）
4. 核心概念（五层架构简述）
5. 第一次对话（API 示例）
6. 常见问题
7. 下一步（链接到 tutorial/ 系列）

```markdown
# OpsCaptain 快速开始

## 项目简介

OpsCaption 是一个面向 AIOps 的智能运维助手，支持故障诊断、知识检索和自动化事件分析。

## 环境要求

- Go 1.24+
- Node.js 18+
- Docker & Docker Compose
- （可选）Redis, RabbitMQ, Milvus — 用于完整功能

## 快速启动

### 1. 克隆项目

git clone https://github.com/your-org/OpsCaption.git
cd OpsCaption

### 2. 配置环境变量

cp .env.example .env.local
# 编辑 .env.local，填入你的 API Key

### 3. 启动后端

go run main.go

### 4. 启动前端（可选）

cd frontend && npm install && npm run dev

## 验证

curl http://localhost:8000/healthz
# 返回 {"ok": true}

## 下一步

- 阅读 [项目概览](../Learn/tutorial/01-项目概览.md) 了解架构
- 阅读 [main 启动流程](../Learn/tutorial/02-main启动流程.md) 理解代码
- 阅读 [ReAct Agent](../Learn/tutorial/03-ReAct-Agent.md) 学习核心引擎
```

- [ ] **Step 2: 提交**

```bash
git add docs/getting-started.md
git commit -m "文档: 新增 getting-started.md 快速开始教程"
```

---

## Task 9: 更新 README.md 文档导航

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在 README 文档导航表中补充缺失链接**

添加以下链接：
- `docs/getting-started.md` — 快速开始
- `docs/interview/` — 面试准备
- `docs/superpowers/plans/` — 实施计划
- `docs/tech-decisions/` — 技术决策

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "文档: README 补充 getting-started、面试、计划、技术决策文档链接"
```

---

## Task 10: 创建 CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: 编写贡献指南**

内容：
1. 开发环境搭建
2. 代码规范（引用 AGENTS.md）
3. 提交规范（中文 commit message）
4. 测试要求（go test ./... + npm run build）
5. PR 流程
6. 架构规则（import 规则简述）

- [ ] **Step 2: 提交**

```bash
git add CONTRIBUTING.md
git commit -m "文档: 新增 CONTRIBUTING.md 贡献指南"
```

---

## Task 11: 全量验证

- [ ] **Step 1: 构建验证**

```bash
go build ./...
go vet ./...
```

- [ ] **Step 2: 测试验证**

```bash
go test ./...
```

- [ ] **Step 3: Import 规则验证**

```bash
# 检查 controller 层不再直接 import ai/
grep -r "SuperBizAgent/internal/ai" internal/controller/ --include="*.go" | grep -v "_test.go"
# 应返回空
```

- [ ] **Step 4: 文档完整性检查**

```bash
ls docs/getting-started.md CONTRIBUTING.md .env.example
# 三个文件都应存在
```

- [ ] **Step 5: 最终提交**

```bash
git add -A
git commit -m "验证: 全量构建、测试、import 规则检查通过"
```

---

## 执行顺序

```
Task 1 (prod config)     ← 最快修复，5 分钟
Task 2 (.env.example)    ← 关键缺失，10 分钟
Task 5 (CLI exceptions)  ← 文档更新，5 分钟
Task 8 (getting-started) ← 文档，20 分钟
Task 9 (README update)   ← 文档，10 分钟
Task 10 (CONTRIBUTING)   ← 文档，15 分钟
Task 3 (controller import) ← 核心重构，60 分钟
Task 4 (ai/infra import)  ← 核心重构，40 分钟
Task 6 (Dockerfile)       ← 安全加固，15 分钟
Task 7 (log rotation)     ← 运维，5 分钟
Task 11 (全量验证)         ← 收尾，10 分钟
```

**预计总时间：~3 小时**（比原估 6 小时更紧凑，因为 build/test 已通过）
