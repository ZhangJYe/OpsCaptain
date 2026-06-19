# CMDB Phase 2 设计方案：主机管理 + CMDBEnricher

> 目标：1) 让 AI 能查"服务部署在哪些机器上"；2) ContextEngine 自动注入服务元数据，不需要 Agent 主动调工具。
> 约束：Host 数据存在同一个 YAML 文件，CMDBEnricher 通过 interface 注入 ContextEngine。

---

## 1. 数据模型

### 1.1 Host 实体

在 `docs/assets/services.yaml` 中新增 `hosts` 段：

```yaml
services:
  - name: paymentservice
    ...

hosts:
  - name: paymentservice-pod-01
    service: paymentservice        # 关联服务
    ip: "10.0.1.100"
    node: "node-03"
    cluster: "prod-cluster-01"
    env: "production"
    status: "running"
    last_restart: "2026-06-18"
    tags:
      - primary
```

### 1.2 Host 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 实例名称（唯一标识） |
| `service` | string | ✅ | 关联服务名 |
| `ip` | string | ✅ | IP 地址 |
| `node` | string | ❌ | 所在 Node |
| `cluster` | string | ✅ | 集群 |
| `env` | string | ✅ | 环境 |
| `status` | string | ✅ | running / stopped / unhealthy |
| `last_restart` | string | ❌ | 最近重启时间 |
| `tags` | []string | ❌ | 自定义标签 |

---

## 2. 架构设计

### 2.1 模块划分

```
新增/修改文件：
  internal/infra/cmdb/types.go           # 新增 HostDTO
  internal/infra/cmdb/store.go           # 新增 Host CRUD 方法
  internal/infra/cmdb/store_test.go      # Host 测试
  internal/ai/cmdb/repository.go         # 新增 HostRepository interface + HostInfo
  internal/ai/cmdb/service.go            # Host 格式化函数
  internal/ai/cmdb/service_test.go       # Host 测试
  internal/app/cmdb_adapter.go           # Host adapter 方法
  internal/app/cmdb_app.go               # Host app 方法
  internal/app/cmdb_validation.go        # Host 校验
  internal/ai/tools/query_cmdb_host.go   # Host AI 工具
  internal/ai/tools/query_cmdb_host_test.go
  internal/ai/tools/tiered_tools.go      # 注册 Host 工具
  api/chat/v1/cmdb.go                    # Host API 类型
  internal/controller/chat/chat_v1_cmdb.go  # Host controller
  docs/assets/services.yaml              # 示例 host 数据

  internal/ai/contextengine/cmdb_enricher.go  # CMDBEnricher interface
  internal/ai/contextengine/assembler.go      # 集成 CMDBEnricher
  main.go                                      # 注入 CMDBEnricher
```

### 2.2 Host Repository Interface

```go
// internal/ai/cmdb/repository.go 新增

type HostInfo struct {
    Name        string   `json:"name"`
    Service     string   `json:"service"`
    IP          string   `json:"ip"`
    Node        string   `json:"node,omitempty"`
    Cluster     string   `json:"cluster"`
    Env         string   `json:"env"`
    Status      string   `json:"status"`
    LastRestart string   `json:"last_restart,omitempty"`
    Tags        []string `json:"tags,omitempty"`
}

type HostRepository interface {
    GetHost(name string) (*HostInfo, bool)
    ListHostsByService(service string) []HostInfo
    ListHostsByCluster(cluster string) []HostInfo
    ListAllHosts() []HostInfo
}
```

### 2.3 CMDBEnricher Interface

```go
// internal/ai/contextengine/cmdb_enricher.go

package contextengine

import "context"

type CMDBEnricher interface {
    // Enrich 根据 query 自动提取服务名并返回元数据作为 ContextItem
    Enrich(ctx context.Context, query string) []ContextItem
}
```

由 main.go 注入实现，Assembler 只依赖 interface。

### 2.4 ContextEngine 集成

在 Assembler 的装配流程中新增一个 stage：

```
history → memory → cmdb → docs → tool_results
```

CMDB stage：
1. 从 query 中提取可能的服务名（正则匹配已知服务名列表）
2. 调用 CMDBEnricher.Enrich()
3. 结果转换为 ContextItem 注入
4. 记录到 ContextTrace

---

## 3. AI 工具设计

### 3.1 query_cmdb_host 工具

```go
type QueryCMDBHostInput struct {
    Action    string `json:"action" jsonschema:"description=查询动作：get_host(查单个实例), list_by_service(按服务查), list_by_cluster(按集群查), list_all(列全部)"`
    HostName  string `json:"host_name,omitempty"`
    Service   string `json:"service,omitempty"`
    Cluster   string `json:"cluster,omitempty"`
    Limit     int    `json:"limit,omitempty"`
}
```

### 3.2 query_cmdb 工具增强

在现有 `query_cmdb` 工具中新增一个 action：`get_service_hosts` — 查询某服务的所有实例。

---

## 4. 配置项

```yaml
cmdb:
  enricher:
    enabled: true              # ContextEngine 自动注入开关
    max_tokens: 500            # 最大注入 token 数
    fuzzy_match: true          # 是否模糊匹配服务名
```

---

## 5. 演进路线更新

| 阶段 | 内容 | 状态 |
|------|------|------|
| MVP | 只读 YAML + Agent 工具 + 只读 API | ✅ |
| Phase 1.5 | 写 API + 原子写 + schema 校验 | ✅ |
| **Phase 2** | 主机管理 + CMDBEnricher | 🔄 当前 |
| Phase 3 | 调用拓扑 + 可视化 | 待定 |
