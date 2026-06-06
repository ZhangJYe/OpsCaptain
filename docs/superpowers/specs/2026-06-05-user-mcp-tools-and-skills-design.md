# 用户自定义 MCP 工具 & Skill 设计文档

## 概述

允许用户在前端动态添加 MCP 工具和 Skill，扩展 OpsCaptain 的能力边界。用户配置的工具和 Skill 全局共享，需管理员审批后生效。

## 设计决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 架构方案 | GenericSkill + 动态 MCP 注册 | 改动最小，复用现有 Registry/ProgressiveDisclosure 体系 |
| MCP 连接 | SSE + HTTP 双模式 | 与现有日志 MCP 接入方式一致 |
| 作用域 | 全局共享 | 简化设计，团队统一管理 |
| 安全 | 网络白名单 + 管理员审批 + 调用隔离 | 全方位安全防护 |
| 输出解析 | 模板化解析（4 种模式） | 简单可靠，无 LLM 依赖 |
| 匹配策略 | OR 关键词匹配 | 简单直接，覆盖大部分场景 |
| 持久化 | 文件 JSON | 与现有 incident/memory 模式一致 |
| UI 布局 | 标签页切换（MCP 工具 / Skill） | 简洁直观，与现有 Sidebar 风格一致 |
| 表单模式 | 分步表单 | 先测试连接再填元信息，减少无效操作 |
| 侧边栏集成 | 追加到现有域分组 | 用户 Skill 混合展示，用 🧩 图标区分 |

---

## 数据模型

### UserMCPTool

```go
type UserMCPTool struct {
    ID            string         `json:"id"`
    Name          string         `json:"name"`
    Description   string         `json:"description"`
    Transport     string         `json:"transport"`       // "sse" | "http"
    EndpointURL   string         `json:"endpoint_url"`
    HTTPURL       string         `json:"http_url,omitempty"`
    AuthToken     string         `json:"auth_token,omitempty"`
    ToolName      string         `json:"tool_name"`       // MCP server 上的具体 tool name
    InputSchema   map[string]any `json:"input_schema"`    // 自动发现的输入 schema
    TimeoutMs     int            `json:"timeout_ms"`      // 默认 5000
    Status        string         `json:"status"`          // "pending" | "approved" | "rejected" | "disabled"
    CreatedAt     time.Time      `json:"created_at"`
    CreatedBy     string         `json:"created_by"`
    ApprovedAt    *time.Time     `json:"approved_at,omitempty"`
    ApprovedBy    string         `json:"approved_by,omitempty"`
}
```

### UserSkill

```go
type UserSkill struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`            // 全局唯一
    Description   string    `json:"description"`
    Domain        string    `json:"domain"`          // "metrics" | "logs" | "knowledge" | "custom"
    ToolRefID     string    `json:"tool_ref_id"`     // 关联的 UserMCPTool ID
    Keywords      []string  `json:"keywords"`        // OR 匹配
    Focus         string    `json:"focus"`
    OutputParser  string    `json:"output_parser"`   // "json_array" | "json_nested" | "log_lines" | "raw"
    JSONPath      string    `json:"json_path"`       // json_nested 模式下的路径
    Tier          int       `json:"tier"`            // 1=SkillGate, 2=OnDemand
    Status        string    `json:"status"`          // "pending" | "approved" | "rejected" | "disabled"
    CreatedAt     time.Time `json:"created_at"`
    CreatedBy     string    `json:"created_by"`
    ApprovedAt    *time.Time `json:"approved_at,omitempty"`
    ApprovedBy    string    `json:"approved_by,omitempty"`
}
```

### 文件存储

```
var/runtime/user_tools/
└── registry.json    # { "tools": [...], "skills": [...] }
```

原子写入（写临时文件 → rename），与 `fileLongTermMemoryStore` 模式一致。

---

## 核心组件

### 1. DynamicMCPRegistry

管理多个 MCP server 连接，提供统一的工具调用接口。

**位置**：`internal/ai/tools/dynamic_mcp_registry.go`

```go
type DynamicMCPRegistry struct {
    mu          sync.RWMutex
    connections map[string]*mcpConnection  // key = toolID
}

func (r *DynamicMCPRegistry) Register(ctx context.Context, cfg UserMCPTool) error
func (r *DynamicMCPRegistry) Unregister(toolID string)
func (r *DynamicMCPRegistry) Get(toolID string) (tool.InvokableTool, bool)
func (r *DynamicMCPRegistry) Invoke(ctx context.Context, toolID, args string) (string, error)
func (r *DynamicMCPRegistry) HealthCheck(ctx context.Context) map[string]error
```

**关键行为**：
- `Register` 时连接 MCP server 并发现 tool，失败返回错误
- `Invoke` 内部处理超时、重连、降级（返回 `{"degraded": true, ...}` JSON）
- 连接池复用 `internal/ai/tools/query_log.go` 中的 `mcpClientPool`
- 懒连接，空闲 30 分钟断开，调用时重连

### 2. GenericSkill

实现 `skills.Skill` 和 `skills.FocusProvider` 接口。

**位置**：`internal/ai/skills/generic_skill.go`

```go
type GenericSkill struct {
    config UserSkill
    toolID string
    mcpReg *DynamicMCPRegistry
}

func (s *GenericSkill) Name() string        { return s.config.Name }
func (s *GenericSkill) Description() string { return s.config.Description }
func (s *GenericSkill) Focus() string       { return s.config.Focus }

func (s *GenericSkill) Match(task *protocol.TaskEnvelope) bool {
    return skills.ContainsAny(task.Goal, s.config.Keywords...)
}

func (s *GenericSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
    // 1. 调用 MCP 工具
    output, err := s.mcpReg.Invoke(ctx, s.toolID, buildGenericInput(task))
    // 2. 模板化解析输出
    evidence := s.parseOutput(output)
    // 3. 构建 TaskResult
    return buildGenericResult(task, evidence, err), nil
}
```

**输出解析模板**：

| Parser | 输入格式 | 解析逻辑 |
|--------|---------|----------|
| `json_array` | `[{"title": "...", "content": "..."}, ...]` | 遍历数组，每项构建 EvidenceItem |
| `json_nested` | `{"data": {"items": [...]}}` | 按 `json_path` 提取嵌套数组，再按 json_array 处理 |
| `log_lines` | 多行文本 | 按 `\n` 切分，每行一个 snippet |
| `raw` | 任意 | 整个输出作为一个 EvidenceItem |

**通用 Confidence 分层**：
- 成功有 evidence → `StatusSucceeded`, `Confidence: 0.70`
- 成功无 evidence → `StatusDegraded`, `Confidence: 0.40`
- 工具调用失败 → `StatusDegraded`, `Confidence: 0.25`

### 3. UserSkillStore

**位置**：`internal/ai/skills/user_skill_store.go`

```go
type UserSkillStore interface {
    Load(ctx context.Context) (*UserRegistryData, error)
    Save(ctx context.Context, data *UserRegistryData) error
}

type UserRegistryData struct {
    Tools  []UserMCPTool `json:"tools"`
    Skills []UserSkill   `json:"skills"`
}

type fileUserSkillStore struct {
    path string  // "var/runtime/user_tools/registry.json"
}
```

---

## API 端点

遵循现有 GoFrame 风格，新增端点：

| Method | Path | 说明 |
|--------|------|------|
| POST | `/api/mcp_tools` | 创建 MCP 工具配置 |
| GET | `/api/mcp_tools` | 列出所有 MCP 工具 |
| PUT | `/api/mcp_tools/{tool_id}` | 更新工具配置 |
| DELETE | `/api/mcp_tools/{tool_id}` | 删除工具 |
| POST | `/api/mcp_tools/{tool_id}/test` | 测试连接，发现工具列表 |
| POST | `/api/mcp_tools/{tool_id}/approve` | 审批工具 |
| POST | `/api/mcp_tools/{tool_id}/reject` | 拒绝工具 |
| POST | `/api/skills` | 创建 Skill |
| GET | `/api/skills` | 列出所有用户 Skill |
| PUT | `/api/skills/{skill_id}` | 更新 Skill |
| DELETE | `/api/skills/{skill_id}` | 删除 Skill |
| POST | `/api/skills/{skill_id}/approve` | 审批 Skill |
| POST | `/api/skills/{skill_id}/reject` | 拒绝 Skill |

**响应格式**：
- 列表：`{ "items": [...] }`
- 单个：`{ "tool": {...} }` 或 `{ "skill": {...} }`
- 操作：`{ "success": true }`

---

## 系统集成

### Registry 注册

启动时 + 每次 CRUD 操作后，将 approved 的用户 skill 注册到对应 domain 的 `skills.Registry`：

```go
func LoadUserSkills(ctx context.Context, store UserSkillStore, mcpReg *DynamicMCPRegistry,
    metricsReg, logsReg, knowledgeReg, customReg *skills.Registry) error {
    data, _ := store.Load(ctx)
    for _, us := range data.Skills {
        if us.Status != "approved" {
            continue
        }
        gs := NewGenericSkill(us, mcpReg)
        targetReg := resolveDomain(us.Domain, metricsReg, logsReg, knowledgeReg, customReg)
        // 先尝试注销同名旧 skill（热更新场景），再注册新的
        targetReg.Unregister(us.Name)
        targetReg.Register(gs)
    }
    return nil
}

func resolveDomain(domain string, metricsReg, logsReg, knowledgeReg, customReg *skills.Registry) *skills.Registry {
    switch domain {
    case "metrics":
        return metricsReg
    case "logs":
        return logsReg
    case "knowledge":
        return knowledgeReg
    default:
        return customReg  // "custom" 和其他未知域统一走 customReg
    }
}
```

**热更新**：CRUD 操作（approve/reject/delete）后调用 `ReloadUserSkills()`，重新加载 JSON 并同步 Registry。`Registry` 新增 `Unregister(name)` 方法支持移除已注册的用户 skill。

### ProgressiveDisclosure

用户 MCP 工具自动注册为 `TieredTool`，Tier 由 `UserSkill.Tier` 决定：

```go
for _, t := range approvedTools {
    tieredTools = append(tieredTools, skills.TieredTool{
        Tool:   t.tool,
        Tier:   skills.TierSkillGate,
        Domains: []string{t.domain},
    })
}
```

### FocusCollector

`GenericSkill` 实现 `FocusProvider`，`FocusCollector.Collect` 自动收集用户 skill 的 focus hint。

### Capabilities

用户 skill 通过 `PrefixedCapabilities` 暴露为 `skill:user_xxx`，GoS Engine expert 可引用。

---

## 安全层

### 网络白名单

```yaml
# config.yaml
user_tools:
  network_whitelist:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
    - "127.0.0.0/8"
```

`Register` 时解析 endpoint URL 的 host，检查 IP 是否在白名单内。不在白名单 → 返回 403。

### 管理员审批

新创建的 tool/skill 默认 `status: "pending"`。管理员通过 `/approve` 端点审批后才生效。审批操作记录 `ApprovedAt` 和 `ApprovedBy`。

审批采用**直接更新**模式（不复用 ApprovalQueue）：管理员调用 `/approve` 或 `/reject` 端点，后端直接更新 JSON 文件中的 status 字段，然后调用 `ReloadUserSkills()` 热更新 Registry。比 ApprovalQueue 更简单，因为工具/Skill 配置不是高频操作。

### 调用隔离

- 用户工具有独立的 `TimeoutMs`（默认 5000ms）
- `DynamicMCPRegistry.Invoke` 内部捕获所有 error，返回 degraded JSON
- 用户工具调用失败不影响内置工具

---

## 前端 UI

### 新增文件

```
frontend/src/
├── components/settings/
│   ├── ToolManager.tsx       # MCP 工具列表 + CRUD
│   ├── SkillManager.tsx      # Skill 列表 + CRUD
│   ├── MCPToolForm.tsx       # 分步表单：连接配置 → 基本信息
│   ├── UserSkillForm.tsx     # 分步表单：基本信息 → 匹配&解析
│   └── ApprovalBadge.tsx     # 状态徽章
├── hooks/
│   └── useUserTools.ts       # API 调用 hook
└── types/
    └── userTools.ts          # TypeScript 类型
```

### 页面布局

标签页切换「MCP 工具」和「Skill」两个视图，每个视图是卡片列表。入口在 Sidebar 底部的「⚙️ 工具 & Skill 管理」。

### MCP 工具表单（分步）

1. **Step 1 连接配置**：传输方式（SSE/HTTP）、Endpoint URL、Auth Token、测试连接（发现工具列表，勾选需要的）
2. **Step 2 基本信息**：名称、描述、超时

### Skill 表单（分步）

1. **Step 1 基本信息**：名称、描述、域选择（metrics/logs/knowledge/custom）、关联 MCP 工具下拉（仅列出已审批工具）
2. **Step 2 匹配 & 解析**：关键词 tag 输入（OR 语义）、Focus 提示、输出解析模式、json_nested 路径、门控层级

### 侧边栏集成

用户 Skill 追加到对应域的末尾，用 🧩 图标 + 虚线边框区分。域分组内的排序：内置 skill 在前，用户 skill 在后。

### 审批流程

```
用户提交 → pending（不显示、不可用）
    → 管理员 approve → 自动注册到 Registry，出现在 SkillPanel
    → 管理员 reject → 用户可编辑后重新提交
```

---

## 文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新增 | `internal/ai/tools/dynamic_mcp_registry.go` | 动态 MCP 连接管理 |
| 新增 | `internal/ai/skills/generic_skill.go` | 通用 Skill 执行器 |
| 新增 | `internal/ai/skills/user_skill_store.go` | 文件持久化 |
| 新增 | `internal/ai/skills/user_skill_loader.go` | 启动加载 + 热更新 |
| 修改 | `internal/ai/skills/registry.go` | 新增 Unregister(name) 方法 |
| 新增 | `internal/ai/skills/custom_registry.go` | custom domain 的 Registry 实例管理 |
| 修改 | `internal/ai/skills/progressive_disclosure.go` | 集成用户工具到 TieredTool |
| 修改 | `internal/ai/tools/tiered_tools.go` | BuildTieredTools 追加用户工具 |
| 新增 | `api/chat/v1/user_tools.go` | API 请求/响应类型 |
| 新增 | `internal/controller/chat/chat_v1_mcp_tools.go` | MCP 工具 CRUD controller |
| 新增 | `internal/controller/chat/chat_v1_user_skills.go` | Skill CRUD controller |
| 修改 | `internal/controller/chat/chat_new.go` | 注册新 controller |
| 修改 | `manifest/config/config.yaml` | 新增 user_tools 配置段 |
| 新增 | `frontend/src/components/settings/ToolManager.tsx` | 工具管理 UI |
| 新增 | `frontend/src/components/settings/SkillManager.tsx` | Skill 管理 UI |
| 新增 | `frontend/src/components/settings/MCPToolForm.tsx` | MCP 工具分步表单 |
| 新增 | `frontend/src/components/settings/UserSkillForm.tsx` | Skill 分步表单 |
| 新增 | `frontend/src/components/settings/ApprovalBadge.tsx` | 状态徽章 |
| 新增 | `frontend/src/hooks/useUserTools.ts` | API hook |
| 新增 | `frontend/src/types/userTools.ts` | TypeScript 类型 |
| 修改 | `frontend/src/App.tsx` | 新增 workbenchMode: 'settings' |
| 修改 | `frontend/src/components/sidebar/Sidebar.tsx` | 底部增加管理入口 |
| 修改 | `frontend/src/components/sidebar/SkillPanel.tsx` | 渲染用户 Skill |
| 修改 | `frontend/src/lib/utils.ts` | SKILL_GROUPS 动态合并用户 Skill |

---

## 不在范围

- 用户 Skill 的 LLM-based 匹配（仅支持 OR 关键词）
- 用户 Skill 的自定义 Runner 代码（仅支持模板化解析）
- 多租户隔离（全局共享）
- MCP server 的 WebSocket 传输
- Skill 版本管理 / 回滚
