# 贡献指南

感谢你对 OpsCaption 的关注！本文档帮助你快速上手开发。

## 开发环境

1. 安装 Go 1.24+ 和 Node.js 18+
2. 克隆项目：`git clone ... && cd OpsCaption`
3. 配置环境变量：`cp .env.example .env.local` 并填入 API Key
4. 启动后端：`go run main.go`
5. 启动前端：`cd frontend && npm install && npm run dev`

## 代码规范

### 架构分层

项目采用五层架构，import 规则严格：

```
Controller (参数解析) → App (业务编排) → Domain (核心规则) → Infra (外部适配) → Common (横切关注点)
```

**禁止的 import：**
- `controller/` 不能直接 import `ai/` 或 `infra/`
- `ai/` 不能直接 import `infra/`（必须通过 interface 注入）
- `utility/` 不能 import `internal/`

详见 [AGENTS.md](AGENTS.md) 第 3 节。

### 文件大小限制

- Controller 方法 < 50 行
- Application Service < 300 行
- Domain Service < 500 行
- Infrastructure Adapter < 400 行

### 命名规范

- Go 文件：snake_case.go
- 测试文件：*_test.go
- 配置项：走 `config.yaml`，不硬编码
- Commit message：**中文**

## 测试要求

提交前必须通过：

```bash
go build ./...     # 编译通过
go test ./...      # 测试通过
go vet ./...       # 静态分析
```

如有前端改动：

```bash
cd frontend && npm run build
```

## 提交流程

1. 从 main 创建分支：`git checkout -b feature/your-feature`
2. 开发并测试
3. 推送前 rebase：`git pull --rebase`
4. 提交：commit message 用中文
5. 不要主动 push，等 reviewer 确认

## 安全红线

- 不要暴露或日志记录 secrets / keys
- 不要恢复 `chat_multi_agent` 路由（已废弃）
- 新增配置必须进入 config.yaml，不硬编码
- 错误处理走降级，不直接 fatal