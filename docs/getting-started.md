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

编辑项目根目录唯一的 `.env`，至少填入以下必填项：

```bash
DEEPSEEK_API_KEY=your-deepseek-api-key    # DeepSeek API 密钥
ARK_API_KEY=your-ark-api-key              # 豆包 API 密钥（用于 Embedding）
AUTH_JWT_SECRET=your-jwt-secret           # JWT 认证密钥
```

模型方案使用 DeepSeek Chat + 豆包 Embedding。`.env` 是唯一的本地密钥入口；provider、模型名、Base URL 和向量维度统一维护在 `manifest/config/config.yaml`。

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

## 本地可观测性环境

Prometheus、Loki、模拟指标和模拟日志使用独立的本地环境，不复用生产 Compose：

```bash
make observability-up
```

它会启动：

- Prometheus：`http://127.0.0.1:9090`
- Loki：`http://127.0.0.1:3100`
- Prometheus Avalanche：持续生成模拟指标并周期制造 series spike
- 轻量日志生成器：持续生成 checkout/payment/gateway/catalog 模拟故障日志
- 日志适配器：`http://127.0.0.1:18088/tools/query_logs`

其中 checkout 指标、`CheckoutSyntheticSeriesSpike` 告警和 payment-timeout 日志共享 `service=checkout`、`incident=payment-timeout` 标签，可以直接用于 GoS 的告警、指标、日志证据关联。

本地端点已经集中配置在 `manifest/config/config.yaml` 的 `prometheus.address` 和 `mcp.log_http_url`；`.env` 仍然只保存 API key、密码和 webhook secret。

验证数据链路：

```bash
curl 'http://127.0.0.1:9090/api/v1/query?query=up'

curl -X POST 'http://127.0.0.1:18088/tools/query_logs' \
  -H 'Content-Type: application/json' \
  -d '{"query":"timeout","service":"checkout","window":"30m"}'
```

查看状态或停止：

```bash
make observability-status
make observability-down
```

这套环境用于本地协议联调和 GoS smoke test。需要更真实的跨服务故障时，可以把 OpenTelemetry Astronomy Shop 的指标接入同一个 Prometheus、日志接入同一个 Loki，OpsCaptain 侧不需要再修改工具协议。

`deploy/docker-compose.prod.yml` 仅用于部署手册定义的服务器生产环境，不作为本地验证入口。

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
A: 在 `.env` 中设置 `AUTH_JWT_SECRET`，然后在 `config.yaml` 中设置 `auth.enabled: true`。
