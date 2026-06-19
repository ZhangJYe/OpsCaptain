# CMDB MVP 设计方案：本地服务注册表

> 目标：让 AI Agent 具备"查资产"能力，知道每个服务是谁负责的、部署在哪、依赖谁。
> 约束：零额外部署依赖，不增加容器/磁盘开销，数据存本地 YAML 文件，只读不写。

---

## 1. 背景与目标

### 1.1 问题

当前 AI Agent 能查监控、查日志、查知识库，但不知道"这个服务是谁的、在哪、依赖谁"。用户问"paymentservice 是谁负责的"，AI 只能回答"我不知道"。

### 1.2 目标

在 OpsCaption 内部实现一个轻量服务注册表，让 AI Agent 能：

1. 查询某个服务的元数据（Owner、团队、集群、环境）
2. 查询某个服务的依赖关系（上游依赖 + 反向计算下游）
3. 查询某个集群/团队下有哪些服务
4. 根据关键词模糊搜索服务

### 1.3 非目标（MVP 阶段明确不做）

- 不做写 API — YAML 只读，手动维护，避免并发/原子写/多实例一致性问题
- 不做主机/实例管理（后续演进）
- 不做 Web UI（后续演进）
- 不做调用拓扑可视化（后续演进）
- 不接入外部 CMDB 系统（后续演进）
- ContextEngine 不直接依赖 CMDB 实现 — 通过 Agent 工具返回自然语言结果，进入现有 ToolItems 流程

---

## 2. 数据模型

### 2.1 Service 实体

```yaml
# docs/assets/services.yaml
services:
  - name: paymentservice          # 服务名称（唯一标识）
    display_name: "支付服务"       # 中文展示名
    owner: "张三"                 # 负责人
    team: "支付团队"              # 所属团队
    cluster: "prod-cluster-01"   # 部署集群
    env: "production"            # 环境：production / staging / development
    region: "cn-shanghai"        # 地域/机房
    language: "go"               # 技术栈
    port: 8080                   # 服务端口
    dependencies:                # 上游依赖（本服务调用谁）
      - userservice
      - cartservice
    description: "处理支付、退款、对账"  # 服务描述
    on_call: "张三"              # 当前 On-Call 人
    last_deploy: "2026-06-17"    # 最近部署时间
    tags:                        # 自定义标签
      - critical
      - payment
      - core
```

### 2.2 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 服务唯一标识，小写英文 |
| `display_name` | string | ❌ | 中文展示名 |
| `owner` | string | ✅ | 负责人姓名 |
| `team` | string | ✅ | 所属团队 |
| `cluster` | string | ✅ | 部署集群 |
| `env` | string | ✅ | 环境 |
| `region` | string | ❌ | 地域/机房 |
| `language` | string | ❌ | 技术栈 |
| `port` | int | ❌ | 服务端口 |
| `dependencies` | []string | ❌ | 上游依赖（单向存储） |
| `description` | string | ❌ | 服务描述 |
| `on_call` | string | ❌ | 当前 On-Call |
| `last_deploy` | string | ❌ | 最近部署时间 |
| `tags` | []string | ❌ | 自定义标签 |

### 2.3 依赖关系设计

**只存 `dependencies`（上游依赖），`dependents`（下游依赖）通过反向计算得出。**

理由：
- 双写必然漂移，长期维护成本高
- 下游关系可以从上游关系反向推导
- MVP 阶段数据量小，反向计算开销可忽略

```go
// GetDependents 返回谁依赖该服务（反向计算）
func (s *ServiceStore) GetDependents(serviceName string) []string {
    var dependents []string
    for _, svc := range s.services {
        for _, dep := range svc.Dependencies {
            if dep == serviceName {
                dependents = append(dependents, svc.Name)
                break
            }
        }
    }
    return dependents
}
```

---

## 3. 架构设计

### 3.1 分层与依赖关系（解决 AI → Infra 违规问题）

**核心约束：infra/ 不 import ai/，ai/ 不 import infra/。** 通过 adapter 模式实现。

```
┌──────────────────────────────────────────────────────────────┐
│                    工厂层（组装）                              │
│                                                              │
│  main.go ──→ 创建 YAMLLoader (infra)                         │
│          ──→ 创建 CMDBAdapter (infra，实现 ai interface)      │
│          ──→ 注入到 CMDBService / Tool / App                  │
└──────────────────────────────────────────────────────────────┘
        │                    │                    │
        ▼                    ▼                    ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────┐
│ infra/cmdb/      │ │  ai/cmdb/        │ │  app/cmdb/   │
│                  │ │                  │ │              │
│ YAMLLoader       │ │ ServiceRepository│ │ HTTP API     │
│   (读 YAML)      │ │   (定义 interface)│ │ (只读查询)   │
│ CMDBServiceDTO   │ │ ServiceInfo      │ │              │
│   (infra 自有 DTO)│ │   (domain model) │ │              │
│ CMDBAdapter      │ │ 查询/搜索/依赖    │ │              │
│   (adapter,      │ │                  │ │              │
│    DTO→Info 转换) │ │                  │ │              │
└──────────────────┘ └──────────────────┘ └──────────────┘
   不 import ai/          定义 interface        调用 interface
```

**关键规则：**
- `infra/cmdb/` **不 import ai/** — 只有自己的 DTO 和 YAML 读写
- `ai/cmdb/` **定义** `ServiceRepository` interface + `ServiceInfo` domain model
- `infra/cmdb/CMDBAdapter` **实现** `ServiceRepository`，内部做 DTO → Info 转换
- `main.go` 负责**组装注入**：创建 adapter，传给 tool / app
- `ai/tools/query_cmdb.go` 依赖 `ai/cmdb.ServiceRepository`（合法）

### 3.2 Interface 定义

```go
// internal/ai/cmdb/repository.go
package cmdb

// ServiceInfo domain model — ai/ 层使用的数据结构
type ServiceInfo struct {
    Name         string   `json:"name"`
    DisplayName  string   `json:"display_name,omitempty"`
    Owner        string   `json:"owner"`
    Team         string   `json:"team"`
    Cluster      string   `json:"cluster"`
    Env          string   `json:"env"`
    Region       string   `json:"region,omitempty"`
    Language     string   `json:"language,omitempty"`
    Port         int      `json:"port,omitempty"`
    Dependencies []string `json:"dependencies,omitempty"`
    Dependents   []string `json:"dependents,omitempty"` // 反向计算，不存储
    Description  string   `json:"description,omitempty"`
    OnCall       string   `json:"on_call,omitempty"`
    LastDeploy   string   `json:"last_deploy,omitempty"`
    Tags         []string `json:"tags,omitempty"`
}

// ServiceRepository CMDB 服务仓库接口
// ai/ 层只依赖此接口，不依赖 infra 实现
type ServiceRepository interface {
    GetService(name string) (*ServiceInfo, bool)
    SearchServices(keyword string, limit int) []ServiceInfo
    ListServicesByCluster(cluster string) []ServiceInfo
    ListServicesByTeam(team string) []ServiceInfo
    GetDependents(name string) []string
    ListAll() []ServiceInfo
}
```

```go
// internal/infra/cmdb/store.go — infra 自有 DTO，不 import ai/cmdb
type cmdbServiceDTO struct {
    Name         string   `yaml:"name"`
    DisplayName  string   `yaml:"display_name,omitempty"`
    Owner        string   `yaml:"owner"`
    Team         string   `yaml:"team"`
    Cluster      string   `yaml:"cluster"`
    Env          string   `yaml:"env"`
    Region       string   `yaml:"region,omitempty"`
    Language     string   `yaml:"language,omitempty"`
    Port         int      `yaml:"port,omitempty"`
    Dependencies []string `yaml:"dependencies,omitempty"`
    Description  string   `yaml:"description,omitempty"`
    OnCall       string   `yaml:"on_call,omitempty"`
    LastDeploy   string   `yaml:"last_deploy,omitempty"`
    Tags         []string `yaml:"tags,omitempty"`
}

type cmdbFile struct {
    Services []cmdbServiceDTO `yaml:"services"`
}
```

```go
// internal/infra/cmdb/adapter.go — adapter，实现 ai/cmdb.ServiceRepository
// 内部做 DTO → Info 转换，不泄露 infra 细节
type CMDBAdapter struct {
    loader *YAMLLoader
}

func (a *CMDBAdapter) GetService(name string) (*cmdb.ServiceInfo, bool) {
    dto, ok := a.loader.getService(name)
    if !ok {
        return nil, false
    }
    info := dtoToInfo(dto, a.loader)
    return &info, true
}

func dtoToInfo(dto cmdbServiceDTO, loader *YAMLLoader) cmdb.ServiceInfo {
    return cmdb.ServiceInfo{
        Name:         dto.Name,
        DisplayName:  dto.DisplayName,
        Owner:        dto.Owner,
        Team:         dto.Team,
        Cluster:      dto.Cluster,
        Env:          dto.Env,
        Region:       dto.Region,
        Language:     dto.Language,
        Port:         dto.Port,
        Dependencies: dto.Dependencies,
        Dependents:   loader.getDependents(dto.Name), // 反向计算
        Description:  dto.Description,
        OnCall:       dto.OnCall,
        LastDeploy:   dto.LastDeploy,
        Tags:         dto.Tags,
    }
}
```

**import 规则验证：**
- `infra/cmdb/` → 只 import 标准库 + yaml 包，**不 import ai/**
- `ai/cmdb/` → 只定义 interface + domain model，**不 import infra/**
- `main.go` → import 两者，做组装注入
```

### 3.3 模块划分

```
internal/
├── infra/cmdb/
│   ├── store.go              # YAML 文件读写 + 内存缓存
│   ├── store_test.go
│   └── types.go              # yamlService / yamlFile 数据结构
├── ai/cmdb/
│   ├── repository.go         # ServiceRepository interface + ServiceInfo
│   ├── service.go            # 查询逻辑、模糊匹配、依赖分析
│   └── service_test.go
├── ai/tools/
│   └── query_cmdb.go         # AI Agent 工具
│   └── query_cmdb_test.go
├── app/
│   └── cmdb_app.go           # 只读 HTTP API
api/chat/v1/
│   └── cmdb.go               # API 类型定义
```

### 3.4 分层职责

| 层 | 文件 | 职责 |
|----|------|------|
| Infrastructure | `infra/cmdb/store.go` | YAML 文件读写、内存缓存、反向依赖计算 |
| Domain Interface | `ai/cmdb/repository.go` | 定义 ServiceRepository interface |
| Domain Logic | `ai/cmdb/service.go` | 模糊匹配、搜索策略、结果格式化 |
| Tool | `ai/tools/query_cmdb.go` | AI Agent 工具定义 + 结构化输入 |
| Application | `app/cmdb_app.go` | 只读 HTTP API（GET） |

---

## 4. AI Agent 工具设计

### 4.1 结构化输入（替代 DSL 字符串）

```go
// internal/ai/tools/query_cmdb.go

type QueryCMDBInput struct {
    Action      string `json:"action" jsonschema:"description=查询动作，可选值：
      'get_service' — 查询单个服务详情（需配合 service_name）
      'search' — 按关键词模糊搜索（需配合 keyword）
      'list_by_cluster' — 查询集群下所有服务（需配合 cluster）
      'list_by_team' — 查询团队下所有服务（需配合 team）
      'get_dependencies' — 查询服务依赖关系（需配合 service_name）
      'list_all' — 列出所有服务"`
    ServiceName string `json:"service_name,omitempty" jsonschema:"description=服务名称，用于 get_service / get_dependencies"`
    Cluster     string `json:"cluster,omitempty" jsonschema:"description=集群名称，用于 list_by_cluster"`
    Team        string `json:"team,omitempty" jsonschema:"description=团队名称，用于 list_by_team"`
    Keyword     string `json:"keyword,omitempty" jsonschema:"description=搜索关键词，用于 search（支持服务名/描述/标签模糊匹配）"`
    Limit       int    `json:"limit,omitempty" jsonschema:"description=返回结果数量上限，默认 10，最大 50"`
}
```

### 4.2 工具行为

| action | 必填参数 | 返回内容 |
|--------|---------|---------|
| `get_service` | service_name | 服务完整信息（含反向 dependents） |
| `search` | keyword | 模糊匹配的服务列表 |
| `list_by_cluster` | cluster | 集群下所有服务 |
| `list_by_team` | team | 团队下所有服务 |
| `get_dependencies` | service_name | 该服务的上游依赖 + 下游被依赖 |
| `list_all` | 无 | 所有服务列表 |

### 4.3 降级输出规范

工具在任何异常情况下都返回 JSON 格式的 degraded 结果，**不返回 Go error**（避免 Eino 重试）：

```go
type QueryCMDBOutput struct {
    Success  bool          `json:"success"`
    Degraded bool          `json:"degraded,omitempty"`
    Action   string        `json:"action"`
    Services []ServiceInfo `json:"services,omitempty"`
    Message  string        `json:"message,omitempty"`
    Error    string        `json:"error,omitempty"`
}
```

| 异常场景 | 返回 |
|---------|------|
| CMDB 未启用 | `success:true, services:[], message:"CMDB 未启用"` |
| YAML 文件不存在 | `success:false, degraded:true, error:"services.yaml not found"` |
| YAML 解析失败 | `success:false, degraded:true, error:"services.yaml parse error"` |
| 查询超时 | `success:false, degraded:true, error:"query timeout"` |
| 无匹配结果 | `success:true, services:[], message:"未找到匹配的服务"` |

### 4.4 工具 Tier

**TierSkillGate**（匹配到运维域时暴露）

理由：
- CMDB 查询不是每次对话都需要
- 只有当用户问"谁负责"、"部署在哪"、"依赖谁"等资产相关问题时才需要
- 通过 Skills 的 domain 匹配自动暴露

---

## 5. ContextEngine 集成策略

### 5.1 MVP 策略：不直接注入，走工具链路

**MVP 阶段 ContextEngine 不直接依赖 CMDB 实现。** 理由：

1. 当前 Assembler 是纯上下文编排（history/memory/docs/tool_results），职责边界清晰
2. 直接在 Assembler 里做正则提取 + CMDB 查询 = 隐式 IO，增加复杂度
3. Agent 工具返回的自然语言结果会自动进入 `ToolItems`，已有的 ContextEngine 流程会处理

**数据流：**
```
用户 query → Agent 调用 query_cmdb 工具
           → 工具返回结构化 JSON 服务元数据
           → Agent 基于 JSON 数据分析回答
           → 结果进入 ToolItems
           → ContextAssembler 统一编排
```

### 5.2 Phase 2 演进：CMDBEnricher interface

后续如果需要"自动注入"（不依赖 Agent 主动调用工具），再定义：

```go
// internal/ai/contextengine/cmdb_enricher.go

type CMDBEnricher interface {
    // Enrich 根据 query 自动提取服务名并注入元数据
    Enrich(ctx context.Context, query string) []ContextItem
}
```

由 main.go 注入实现，Assembler 只依赖 interface。带 timeout、trace stage、degraded 标记。

---

## 6. API 设计（只读）

### 6.1 只读 API

| Method | Path | 说明 |
|--------|------|------|
| GET | `/api/cmdb/services` | 列出所有服务 |
| GET | `/api/cmdb/services/search?q=关键词` | 模糊搜索 |
| GET | `/api/cmdb/services/{name}` | 查询单个服务 |
| GET | `/api/cmdb/services/{name}/dependencies` | 查询依赖关系（含反向） |
| GET | `/api/cmdb/services/cluster/{cluster}` | 查询集群下服务 |
| GET | `/api/cmdb/services/team/{team}` | 查询团队下服务 |

### 6.2 写 API（Phase 1.5，当前不做）

写 API 放到 Phase 1.5，需要解决：
- YAML 并发写保护（mutex + temp file + rename）
- Schema validation
- 多 Pod 部署一致性（标注"不保证跨实例一致"）
- 写失败降级

### 6.3 响应示例

**查询服务**
```json
GET /api/cmdb/services/paymentservice

Response:
{
  "success": true,
  "service": {
    "name": "paymentservice",
    "display_name": "支付服务",
    "owner": "张三",
    "team": "支付团队",
    "cluster": "prod-cluster-01",
    "env": "production",
    "dependencies": ["userservice", "cartservice"],
    "dependents": ["gateway", "order-service"],
    "tags": ["critical", "payment"],
    "last_deploy": "2026-06-17"
  }
}
```

**查询依赖**
```json
GET /api/cmdb/services/paymentservice/dependencies

Response:
{
  "success": true,
  "service": "paymentservice",
  "upstream": ["userservice", "cartservice"],
  "downstream": ["gateway", "order-service"]
}
```

---

## 7. 配置项

在 `manifest/config/config.yaml` 中新增：

```yaml
cmdb:
  enabled: true                              # 主开关
  store_path: "docs/assets/services.yaml"    # YAML 文件路径
  cache_ttl_seconds: 60                      # 内存缓存 TTL（秒）
  search:
    max_results: 10                          # 搜索最大返回数
  tool:
    timeout_ms: 3000                         # 工具查询超时
```

---

## 8. 工作量估算

| 任务 | 工作量 | 说明 |
|------|--------|------|
| types.go + repository.go | 1h | 数据结构 + interface 定义 |
| YAMLStore (infra) | 2h | 文件读写 + 缓存 + 反向依赖 + 测试 |
| CMDBService (ai) | 2h | 搜索逻辑 + 结果格式化 + 测试 |
| query_cmdb 工具 | 2h | 结构化输入 + 降级输出 + 集成 tiered_tools |
| 只读 HTTP API | 1.5h | 6 个 GET 端点 + 测试 |
| 示例数据 | 0.5h | services.yaml 示例 |
| 单元测试 | 1h | 覆盖核心逻辑 |
| **总计** | **10h (约 1.5 天)** | |

---

## 9. 文件清单

新增文件：
```
internal/infra/cmdb/types.go              # cmdbServiceDTO / cmdbFile 数据结构
internal/infra/cmdb/store.go              # YAMLLoader：文件读写 + 内存缓存 + 反向依赖
internal/infra/cmdb/store_test.go
internal/infra/cmdb/adapter.go            # CMDBAdapter：DTO→Info 转换，实现 ServiceRepository
internal/ai/cmdb/repository.go            # ServiceRepository interface + ServiceInfo domain model
internal/ai/cmdb/service.go               # 搜索/查询逻辑
internal/ai/cmdb/service_test.go
internal/ai/tools/query_cmdb.go           # AI Agent 工具（结构化输入）
internal/ai/tools/query_cmdb_test.go
internal/app/cmdb_app.go                  # 只读 HTTP API
internal/controller/chat/chat_v1_cmdb.go  # CMDB 只读路由（独立文件）
api/chat/v1/cmdb.go                       # API 类型定义
docs/assets/services.yaml                 # 示例数据
docs/cmdb-mvp-design.md                   # 本文档
```

修改文件：
```
internal/ai/tools/tiered_tools.go              # 注册 query_cmdb 工具
main.go                                        # 组装注入 ServiceRepository + 注册路由
manifest/config/config.yaml                    # 新增 cmdb 配置段
```

新增 controller 文件（不塞进 chat_v1_chat.go）：
```
internal/controller/chat/chat_v1_cmdb.go       # CMDB 只读路由，独立文件
```

---

## 10. 演进路线

| 阶段 | 内容 | 前置条件 |
|------|------|---------|
| **MVP (当前)** | 只读 YAML + Agent 工具 + 只读 API | 无 |
| **Phase 1.5** | 写 API（CRUD）+ 并发保护 + schema validation | MVP 上线 |
| **Phase 2** | 主机/实例管理 + CMDBEnricher interface | Phase 1.5 上线 |
| **Phase 3** | 调用拓扑 + 可视化 | Phase 2 上线 |
| **Phase 4** | 接入蓝鲸 CMDB | 换大服务器 |
| **Phase 5** | 变更关联 + 自动发现 | Phase 3 上线 |

---

## 11. 验收标准

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/infra/cmdb ./internal/ai/cmdb ./internal/ai/tools ./internal/app ./internal/ai/contextengine` 通过
- [ ] `curl -s http://127.0.0.1:8000/api/cmdb/services` 返回示例服务列表
- [ ] `curl -s http://127.0.0.1:8000/api/cmdb/services/paymentservice` 返回服务详情
- [ ] `curl -s http://127.0.0.1:8000/api/cmdb/services/paymentservice/dependencies` 返回依赖关系
- [ ] AI Agent 能通过 `query_cmdb` 工具的 `get_service` action 查询到服务信息
- [ ] AI Agent 能通过 `query_cmdb` 工具的 `search` action 模糊搜索服务
- [ ] 工具在 CMDB 未启用 / 文件不存在 / 无匹配时返回 degraded 而非 error
- [ ] 无 `ai/ → infra/` 违规 import（import guard test 覆盖）
- [ ] 无 `infra/ → ai/` 违规 import

---

## 12. 风险与缓解

| 风险 | 缓解 |
|------|------|
| YAML 文件手动维护成本高 | Phase 1.5 加写 API；或从 Prometheus label 自动发现服务 |
| 多 Pod 部署 YAML 不一致 | MVP 只读，单实例挂载同一文件即可 |
| 模糊匹配不准 | 后续可接入 embedding 做语义搜索 |
| 服务名与告警标签不匹配 | 支持 `alert_label` 字段做映射 |
