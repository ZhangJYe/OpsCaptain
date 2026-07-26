# Skill 系统修复设计

## 背景

前端 Skill 选择系统存在以下问题，导致用户在前端选择 Skill 后无任何反应：

1. **自定义 Skill 后端不识别**: `user-skill:*` 前缀的用户 Skill 被 `ResolveSelectedSkills` 静默丢弃，因为只查 built-in registries
2. **SkillPanel 仅 chat 模式可见**: 默认 aiops 模式下看不到 Skill 面板
3. **选中后无即时反馈**: 用户不知道选择是否生效
4. **前后端 Skill 名称不一致**: 前端 `SKILL_GROUPS` 中部分 ID 在后端 registry 不存在
5. **AIOps 链路缺失**: AIOps 创建事故时不传 `selectedSkillIds`，选中的 Skill 对 AIOps 诊断无影响
6. **customReg 初始化失败**: `main.go:223` 中 `NewRegistry("custom", nil)` 因空 registry 报错返回 nil

## 修改范围

### 1. 修复 customReg 初始化 + 复用 UserSkillLoader 注册自定义 Skill

**问题根因**: 不应在 `ResolveSelectedSkills` 中直接读 store，应复用已有的 `UserSkillLoader` + `GenericSkill` 机制。

**文件**:
- `main.go` - 修复 customReg 初始化
- `internal/ai/skills/registry.go` - 允许空 skills 创建 registry（仅用于占位）

**修改**:
- `registry.go`: 移除 `len(r.skills) == 0` 的报错，允许空 registry 创建
- `main.go:223`: 确保 `customReg` 正常创建，传入 `UserSkillLoader`
- `UserSkillLoader.Reload()` 已经将 approved user skill 注册为 `GenericSkill` 到对应 domain registry（包括 custom），无需重复逻辑

**效果**: User skill 通过 `GenericSkill` 注册到 built-in registries 后，`ResolveSelectedSkills` 的现有逻辑自然能查找到它们，无需特殊处理 `user-skill:` 前缀。

### 2. 扩展 ResolveSelectedSkills 支持 user-skill:* 前缀兜底

**文件**: `internal/ai/skills/progressive_disclosure.go`

即使 UserSkillLoader 已注册，仍需处理前端直接传 `user-skill:*` ID 的情况（如 loader 未及时 reload）。

```go
func (pd *ProgressiveDisclosure) ResolveSelectedSkills(selectedSkillIDs []string) []SelectedSkill {
    // 现有 built-in 查找逻辑不变
    
    // 新增: 兜底处理 user-skill:* 前缀
    // 仅当 built-in registry 未找到时，从 registries 中查找
    // 不直接读 store，而是检查所有 registries（包括 custom）
}
```

**关键约束**: 不直接引入 `UserSkillStore` 依赖，而是通过已注册的 registries 查找。

### 3. AIOps 全链路透传 selectedSkillIds

**问题**: AIOps 创建事故时不传 `selectedSkillIds`，导致选中的 Skill 对诊断无影响。

**修改链路**:

#### 3.1 前端 API 层

**文件**: `api/chat/v1/chat.go`

```go
type AIOpsIncidentCreateReq struct {
    Query           string   `json:"query" v:"required"`
    Engine          string   `json:"engine,omitempty"`
    SelectedSkillIds []string `json:"selected_skill_ids,omitempty"` // 新增
}

type AIOpsIncidentTurnReq struct {
    IncidentID      string   `json:"incident_id" v:"required"`
    Query           string   `json:"query" v:"required"`
    SelectedSkillIds []string `json:"selected_skill_ids,omitempty"` // 新增
}
```

#### 3.2 前端 Hook 层

**文件**: `frontend/src/hooks/useIncidents.ts`

```typescript
const createIncident = useCallback(
  async (query: string, engine: AIOpsEngine, selectedSkillIds?: string[]) => {
    // ...
    body: JSON.stringify({ query, engine, selected_skill_ids: selectedSkillIds }),
    // ...
  },
  [closeEvents, refreshList, subscribe],
)
```

**文件**: `frontend/src/App.tsx`

```typescript
const handleStartAIOps = useCallback(
  (query: string) => {
    setWorkbenchMode('aiops')
    void incidents.createIncident(query, aiOpsEngine, selectedSkillIds).catch(() => undefined)
  },
  [aiOpsEngine, incidents, selectedSkillIds],
)
```

#### 3.3 后端 Controller 层

**文件**: `internal/controller/chat/chat_v1_ai_ops_incident.go`

```go
func (c *ControllerV1) AIOpsIncidentCreate(ctx context.Context, req *v1.AIOpsIncidentCreateReq) (res *v1.AIOpsIncidentRes, err error) {
    // ... 现有逻辑 ...
    incident, err := app.CreateAIOpsIncident(ctx, req.Query, req.Engine, req.SelectedSkillIds)
    // ...
}
```

#### 3.4 后端 App 层

**文件**: `internal/app/ai_ops_incident.go` (或相关文件)

- `CreateAIOpsIncident` 接收 `selectedSkillIds` 并存储到 incident session
- `IncidentSession` 结构体新增 `SelectedSkillIds []string` 字段
- `IncidentTurn` 结构体新增 `SelectedSkillIds []string` 字段

#### 3.5 后端执行层

**文件**: `internal/ai/service/ai_ops_incident_exec.go`

```go
func executeIncidentTurn(ctx context.Context, incidentID, turnID string) {
    // ... 现有逻辑 ...
    runCtx = skills.WithSelectedSkillIDs(runCtx, incident.SelectedSkillIds) // 新增
    // ...
}
```

#### 3.6 AIOps Service 层

**文件**: `internal/ai/service/ai_ops_service.go`

在 `RunAIOpsMultiAgent` 中，除了现有的 `skillFocusCollector.Collect(query)` 自动匹配外，增加对 selectedSkillIds 的处理：

```go
// 在 enrichedQuery 构建后，注入 selected skills 的 focus hints
if selectedSkillIDs := skills.SelectedSkillIDsFromContext(ctx); len(selectedSkillIDs) > 0 {
    selectedHints := skillFocusCollector.ResolveSelected(selectedSkillIDs)
    if len(selectedHints) > 0 {
        enrichedQuery += "\n\n用户指定的分析方向：\n" + skills.FormatFocusHints(selectedHints)
    }
}
```

### 4. SkillPanel 所有模式可见

**文件**: `frontend/src/components/sidebar/Sidebar.tsx`

移除第 83 行的条件判断：

```diff
- {workbenchMode === 'chat' && <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />}
+ <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />
```

### 5. 输入框上方显示已启用 Skill 标签（支持自定义 Skill）

**文件**: `frontend/src/components/chat/ChatInput.tsx`

**问题**: `findSkillsByIds` 只查静态 `SKILL_GROUPS`，不支持 `user-skill:*` 前缀。

**解决方案**: 扩展 `findSkillsByIds` 支持 user-skill 前缀，或在 ChatInput 中直接使用 `useUserTools` 获取 user skill 名称。

```typescript
// 扩展 findSkillsByIds 或新增 getSkillLabelById
function getSkillLabelById(id: string): string {
    // 先查 SKILL_GROUPS
    const builtin = findSkillsByIds([id])
    if (builtin.length > 0) return builtin[0].label
    // 再查 user-skill:* 前缀
    if (id.startsWith('user-skill:')) {
        return id.replace('user-skill:', '')
    }
    return id
}
```

布局：`[告警分诊] [证据提取] [自定义Skill]` 横向排列，位于输入框正上方，点击可取消。

### 6. 对齐前端 SKILL_GROUPS 到后端 Registry

**文件**: `frontend/src/lib/utils.ts`

**实际需要新增的 Skill**（已有则跳过）：

| 新增 Skill ID | Domain | 描述 | 后端来源 |
|--------------|--------|------|---------|
| `logs_service_offline_panic_trace` | logs | 追踪服务下线、pod 重启、crashloop 和 panic 证据 | `log_skills.go:108` |
| `logs_auth_failure_trace` | logs | 追踪登录、token 和鉴权失败 | `log_skills.go:137` |
| `knowledge_service_error_code_lookup` | knowledge | 检索服务错误码说明和排查步骤 | `knowledge_skills.go:136` |

**已存在无需新增**（用户反馈纠正）：
- `knowledge_release_sop` - 已在 `utils.ts:138`
- `knowledge_rollback_runbook` - 已在 `utils.ts`

## 实施约束（必须遵守）

### 约束 1: 共享同一组 registry 实例

**问题**: `logs.SkillRegistry()` / `metrics.SkillRegistry()` 每次调用都新建 registry。如果 `UserSkillLoader` 注册到实例 A，而 `ProgressiveDisclosure` / `FocusCollector` 用实例 B，user skill 仍然查不到。

**要求**: 在应用启动时构建一次 `logsR` / `metricsR` / `knowledgeR` / `customR`，同时传给：
- `UserSkillLoader`（注册 user skill）
- `ProgressiveDisclosure`（解析 selected skills）
- `FocusCollector`（AIOps focus hints）

**涉及文件**:
- `main.go` - 构建共享 registry 实例
- `internal/ai/agent/chat_pipeline/flow.go` - `getChatDisclosure()` 使用共享实例
- `internal/ai/service/ai_ops_service.go` - `skillFocusCollector` 使用共享实例

### 约束 2: AIOps append turn 也要透传 selectedSkillIds

**问题**: spec 只展示了 `createIncident` 的改法，但追加轮次 `App.tsx:189` → `incidents.appendTurn(query)` 也要带 `selectedSkillIds`，否则第二轮起选择不生效。

**修改**:

**文件**: `frontend/src/hooks/useIncidents.ts`

```typescript
const appendTurn = useCallback(
  async (query: string, selectedSkillIds?: string[]) => {
    // ...
    body: JSON.stringify({ query, selected_skill_ids: selectedSkillIds }),
    // ...
  },
  [closeEvents, subscribe],
)
```

**文件**: `frontend/src/App.tsx`

```typescript
onAppend={(query) => {
  void incidents.appendTurn(query, selectedSkillIds).catch(() => undefined)
}}
```

**文件**: `api/chat/v1/chat.go` - `AIOpsIncidentTurnReq` 已在修改 3.1 中新增 `SelectedSkillIds` 字段。

### 约束 3: ResolveSelectedSkills 兜底必须 trim user-skill: 前缀

**问题**: `SkillByName` 查的是 skill name；如果直接拿 `user-skill:xxx` 查 registry 会失败。

**要求**: 在 `ResolveSelectedSkills` 中：
1. 先按原 ID 查所有 registries
2. 失败且有 `user-skill:` 前缀时，用 `strings.TrimPrefix(id, "user-skill:")` 后的 name 再查所有 registries（包括 custom）
3. 确保 `SkillByName` 使用 `strings.EqualFold` 匹配（已有）

```go
func (pd *ProgressiveDisclosure) ResolveSelectedSkills(selectedSkillIDs []string) []SelectedSkill {
    // ... 现有 built-in 查找逻辑 ...
    
    // 兜底: user-skill:* 前缀处理
    for _, id := range remaining {
        name := id
        if strings.HasPrefix(id, "user-skill:") {
            name = strings.TrimPrefix(id, "user-skill:")
        }
        for _, reg := range pd.registries {
            if skill := reg.SkillByName(name); skill != nil {
                selected = append(selected, SelectedSkill{
                    Name:   skill.Name(),
                    Domain: reg.Domain(),
                })
                break
            }
        }
    }
}
```

## 不修改

- 后端 Skill registry 的注册逻辑（UserSkillLoader + GenericSkill 机制不变）
- 前端 `useChat` hook 的 `send` 函数（已经正确传递 `selectedSkillIds`）
- `App.tsx` 的状态管理逻辑
- `localStorage` 持久化逻辑

## 验证方式

1. `go test ./internal/ai/skills/...` - 确保 ResolveSelectedSkills 测试通过
2. `go test ./internal/ai/service/...` - 确保 AIOps service 和 incident 执行链路测试通过
3. `go test ./internal/app/...` - 确保 AIOps incident 相关测试通过
4. `npm run build` - 确保前端构建通过
5. 手动端到端验证：
   - **Chat 模式**: 选择 built-in Skill → 发送消息 → 后端日志确认 `selected_skills` 包含选中的 Skill
   - **Chat 模式**: 创建并审批自定义 Skill → 选择 → 发送消息 → 确认 GenericSkill 被调用
   - **AIOps 模式**: 选择 Skill → 创建事故 → 确认 incident session 存储了 selected_skill_ids
   - **AIOps 模式**: 追加轮次时确认 selected_skill_ids 也被传递
   - **AIOps 模式**: 确认 enrichedQuery 中包含用户指定的分析方向
   - **共享 registry**: 确认 UserSkillLoader 注册的 skill 在 ProgressiveDisclosure 和 FocusCollector 中可见
