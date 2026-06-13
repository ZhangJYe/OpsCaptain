# OpsCaptain 快速开始

## 项目简介

OpsCaption 是一个面向 AIOps 的智能运维助手，支持故障诊断、知识检索和自动化事件分析。

核心能力：
- 智能对话：基于 ReAct Agent 的运维问答
- RAG 检索：混合检索（Dense + BM25 + RRF）
- AIOps 分析：Plan-Execute-Replan 自动化故障诊断
- 变更事件：Webhook 接入 + 主动分析
- 安全防线：七层安全体系

## 环境要求

| 依赖 | 版本 | 用途 |
|------|------|------|
| Go | 1.24+ | 后端编译 |
| Node.js | 18+ | 前端构建 |
| Docker | 24+ | 容器化部署 |
| Docker Compose | v2+ | 本地完整环境 |

## 快速启动（3 分钟）

### 1. 克隆项目

```bash
git clone https://github.com/your-org/OpsCaption.git
cd OpsCaption
```

### 2. 配置环境变量

```bash
cp .env.example .env.local
```

编辑 `.env.local`，至少填入以下必填项：

```bash
DEEPSEEK_API_KEY=your-deepseek-api-key    # DeepSeek API 密钥
ARK_API_KEY=your-ark-api-key              # 豆包 API 密钥（用于 Embedding）
AUTH_JWT_SECRET=your-jwt-secret           # JWT 认证密钥
```

### 3. 启动后端

```bash
go run main.go
```

服务启动后访问：
- API: http://localhost:8000
- 健康检查: http://localhost:8000/healthz
- 就绪检查: http://localhost:8000/readyz
- 指标: http://localhost:8000/metrics

### 4. 启动前端（可选）

```bash
cd frontend
npm install
npm run dev
```

前端开发服务器: http://localhost:5173

## 验证

```bash
# 测试对话
curl -X POST http://localhost:8000/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"prompt": "你好"}'

# 运行全部测试
go test ./...
```

## 完整环境（Docker Compose）

如果需要完整的 Milvus + Redis + RabbitMQ 环境：

```bash
docker compose -f deploy/docker-compose.prod.yml up -d
```

这会启动：后端、前端、Redis、RabbitMQ、Milvus、Jaeger、Prometheus。

## 下一步

- [项目概览](../Learn/tutorial/01-项目概览.md) — 了解架构设计
- [main 启动流程](../Learn/tutorial/02-main启动流程.md) — 理解代码结构
- [ReAct Agent](../Learn/tutorial/03-ReAct-Agent.md) — 学习核心引擎
- [RAG 检索系统](../Learn/tutorial/04-RAG检索系统.md) — 理解知识检索
- [面试速记卡](../Learn/tutorial/09-面试速记卡.md) — 快速复习

## 常见问题

**Q: 启动报错 `redis connection refused`？**
A: Redis 是可选依赖。本地开发可以不启动 Redis，服务会降级为内存模式。

**Q: 启动报错 `milvus connect failed`？**
A: Milvus 是可选依赖。RAG 检索会不可用，但对话功能正常。

**Q: 如何配置认证？**
A: 在 `.env.local` 中设置 `AUTH_JWT_SECRET`，然后在 `config.yaml` 中设置 `auth.enabled: true`。
