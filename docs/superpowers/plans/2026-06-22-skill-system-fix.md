# Skill 系统修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复前端 Skill 选择系统，使用户选择的 Skill（包括自定义 Skill）在 Chat 和 AIOps 模式下都能正确生效。

**Architecture:** 复用现有 UserSkillLoader + GenericSkill 机制注册用户 Skill 到共享 registry 实例，扩展 ResolveSelectedSkills 支持 user-skill:* 前缀兜底，AIOps 全链路透传 selectedSkillIds，前端增加即时反馈标签。

**Tech Stack:** Go (eino, gf), React/TypeScript, Framer Motion

---

## 文件结构

### 后端修改

| 文件 | 职责 |
|------|------|
| `internal/ai/skills/registry.go` | 允许空 skills 创建 registry |
| `internal/ai/skills/progressive_disclosure.go` | 扩展 ResolveSelectedSkills 支持 user-skill:* 兜底 |
| `internal/ai/skills/focus_collector.go` | 新增 ResolveSelected 方法 |
| `main.go` | 构建共享 registry 实例，初始化 UserSkillLoader |
| `internal/ai/agent/chat_pipeline/flow.go` | getChatDisclosure 使用共享 registry |
| `internal/ai/service/ai_ops_service.go` | AIOps 注入 selected skills focus hints |
| `internal/ai/service/ai_ops_incident.go` | IncidentSession 存储 selectedSkillIds |
| `internal/ai/service/ai_ops_incident_exec.go` | 执行时注入 selectedSkillIds 到 context |
| `api/chat/v1/chat.go` | AIOpsIncidentCreateReq/TurnReq 新增字段 |
| `internal/controller/chat/chat_v1_ai_ops_incident.go` | 透传 selectedSkillIds |

### 前端修改

| 文件 | 职责 |
|------|------|
| `frontend/src/lib/utils.ts` | 新增 3 个缺失 Skill，扩展 getSkillLabelById |
| `frontend/src/components/sidebar/Sidebar.tsx` | 移除 workbenchMode 条件 |
| `frontend/src/components/chat/ChatInput.tsx` | 新增 Skill 标签栏 |
| `frontend/src/hooks/useIncidents.ts` | createIncident/appendTurn 传 selectedSkillIds |
| `frontend/src/App.tsx` | AIOps 调用传 selectedSkillIds |

---

## Task 1: 修复 customReg 初始化

**Files:**
- Modify: `internal/ai/skills/registry.go:49-51`

- [ ] **Step 1: 允许空 skills 创建 registry**

修改 `NewRegistry`，移除空 registry 报错：

```go
// internal/ai/skills/registry.go
func NewRegistry(domain string, skills []Skill, opts ...RegistryOption) (*Registry, error) {
	r := &Registry{domain: strings.TrimSpace(domain)}
	for _, opt := range opts {
		opt(r)
	}
	for _, skill := range skills {
		if err := r.Register(skill); err != nil {
			return nil, err
		}
	}
	// 移除: if len(r.skills) == 0 { return nil, fmt.Errorf(...) }
	return r, nil
}
```

- [ ] **Step 2: 运行现有测试确认不破坏**

Run: `go test ./internal/ai/skills/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/skills/registry.go
git commit -m "fix: 允许空 skills 创建 registry，支持 custom domain 占位"
```

---

## Task 2: 构建共享 registry 实例 + 初始化 UserSkillLoader

**Files:**
- Modify: `main.go:220-230`

- [ ] **Step 1: 在 main.go 中构建共享 registry 实例**

```go
// main.go - 在 userSkillStore 和 dynamicMCPReg 初始化之后
logsR := logs.SkillRegistry()
metricsR := metrics.SkillRegistry()
knowledgeR := knowledge.SkillRegistry()
customReg, _ := skills.NewRegistry("custom", nil)

userSkillLoader := skills.NewUserSkillLoader(userSkillStore, dynamicMCPReg, metricsR, logsR, knowledgeR, customReg)
```

- [ ] **Step 2: 确保 UserSkillLoader.Reload 在启动时调用**

检查 main.go 中是否已有 `userSkillLoader.Reload(ctx)` 调用。如果没有，在服务启动后添加：

```go
if err := userSkillLoader.Reload(ctx); err != nil {
    g.Log().Errorf(ctx, "failed to load user skills: %v", err)
}
```

- [ ] **Step 3: 运行测试确认**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "fix: 构建共享 registry 实例并初始化 UserSkillLoader"
```

---

## Task 3: 扩展 ResolveSelectedSkills 支持 user-skill:* 兜底

**Files:**
- Modify: `internal/ai/skills/progressive_disclosure.go:88-112`

- [ ] **Step 1: 修改 ResolveSelectedSkills 增加 user-skill:* 兜底**

```go
func (pd *ProgressiveDisclosure) ResolveSelectedSkills(selectedSkillIDs []string) []SelectedSkill {
	selectedSkillIDs = normalizeSelectedSkillIDs(selectedSkillIDs)
	if len(selectedSkillIDs) == 0 {
		return nil
	}
	selected := make([]SelectedSkill, 0, len(selectedSkillIDs))
	used := make(map[string]bool)

	// 第一轮: 按原 ID 查 built-in registries
	for _, id := range selectedSkillIDs {
		for _, reg := range pd.registries {
			if reg == nil {
				continue
			}
			skill := reg.SkillByName(id)
			if skill == nil {
				continue
			}
			if used[id] {
				break
			}
			used[id] = true
			selected = append(selected, SelectedSkill{
				Name:        skill.Name(),
				Domain:      reg.Domain(),
				Description: skill.Description(),
			})
			break
		}
	}

	// 第二轮: user-skill:* 前缀兜底
	for _, id := range selectedSkillIDs {
		if used[id] {
			continue
		}
		name := id
		if strings.HasPrefix(id, "user-skill:") {
			name = strings.TrimPrefix(id, "user-skill:")
		}
		for _, reg := range pd.registries {
			if reg == nil {
				continue
			}
			skill := reg.SkillByName(name)
			if skill == nil {
				continue
			}
			selected = append(selected, SelectedSkill{
				Name:        skill.Name(),
				Domain:      reg.Domain(),
				Description: skill.Description(),
			})
			break
		}
	}

	return selected
}
```

- [ ] **Step 2: 编写测试验证 user-skill:* 兜底**

```go
// internal/ai/skills/progressive_disclosure_test.go
func TestResolveSelectedSkillsWithUserSkillPrefix(t *testing.T) {
	// 创建一个带 GenericSkill 的 custom registry
	customReg, _ := NewRegistry("custom", nil)
	gs := &testSkill{name: "my_custom_skill", description: "test"}
	customReg.Register(gs)

	pd := NewProgressiveDisclosure(nil, nil) // registries 通过构造函数传入
	// 需要修改 NewProgressiveDisclosure 支持传入 registries
	// 或者直接设置 pd.registries

	result := pd.ResolveSelectedSkills([]string{"user-skill:my_custom_skill"})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Name != "my_custom_skill" {
		t.Errorf("expected name my_custom_skill, got %s", result[0].Name)
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/ai/skills/... -v -run TestResolveSelectedSkills`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/skills/progressive_disclosure.go internal/ai/skills/progressive_disclosure_test.go
git commit -m "feat: ResolveSelectedSkills 支持 user-skill:* 前缀兜底查找"
```

---

## Task 4: 修改 getChatDisclosure 使用共享 registry

**Files:**
- Modify: `internal/ai/agent/chat_pipeline/flow.go:37-49`

- [ ] **Step 1: 修改 getChatDisclosure 接收共享 registry**

```go
var (
	chatDisclosureOnce sync.Once
	chatDisclosureIns  *skills.ProgressiveDisclosure
	sharedRegistries   []*skills.Registry
)

// SetSharedRegistries 在启动时调用，设置共享 registry 实例
func SetSharedRegistries(registries []*skills.Registry) {
	sharedRegistries = registries
}

func getChatDisclosure() *skills.ProgressiveDisclosure {
	chatDisclosureOnce.Do(func() {
		chatDisclosureIns = skills.NewProgressiveDisclosure(
			sharedRegistries,
			tools.BuildTieredTools(context.Background(), userToolStoreDeps, dynamicMCPRegDeps),
		)
	})
	return chatDisclosureIns
}
```

- [ ] **Step 2: 在 main.go 中调用 SetSharedRegistries**

```go
// main.go
chat_pipeline.SetSharedRegistries([]*skills.Registry{metricsR, logsR, knowledgeR, customReg})
```

- [ ] **Step 3: 运行测试确认**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/agent/chat_pipeline/flow.go main.go
git commit -m "fix: getChatDisclosure 使用共享 registry 实例"
```

---

## Task 5: AIOps API 层透传 selectedSkillIds

**Files:**
- Modify: `api/chat/v1/chat.go:228-238`

- [ ] **Step 1: AIOpsIncidentCreateReq 新增字段**

```go
type AIOpsIncidentCreateReq struct {
	g.Meta            `path:"/ai_ops/incidents" method:"post" summary:"创建事故排障会话"`
	Query             string   `json:"query" v:"required|max-length:8000#事故现象不能为空|事故现象长度不能超过8000"`
	Engine            string   `json:"engine,omitempty" v:"in:plan_execute_replan,gos_engine,gos#AIOps引擎不合法"`
	SelectedSkillIds  []string `json:"selected_skill_ids,omitempty"`
}
```

- [ ] **Step 2: AIOpsIncidentTurnReq 新增字段**

```go
type AIOpsIncidentTurnReq struct {
	g.Meta            `path:"/ai_ops/incidents/{incident_id}/turns" method:"post" summary:"追加事故排障轮次"`
	IncidentID        string   `json:"incident_id" v:"required|max-length:128#事故ID不能为空|事故ID长度不能超过128"`
	Query             string   `json:"query" v:"required|max-length:8000#追加现象不能为空|追加现象长度不能超过8000"`
	SelectedSkillIds  []string `json:"selected_skill_ids,omitempty"`
}
```

- [ ] **Step 3: 运行构建确认**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add api/chat/v1/chat.go
git commit -m "feat: AIOps incident API 新增 selected_skill_ids 字段"
```

---

## Task 6: 后端 IncidentSession 存储 selectedSkillIds

**Files:**
- Modify: `internal/ai/service/ai_ops_incident.go` (IncidentSession/IncidentTurn 结构体)

- [ ] **Step 1: IncidentSession 新增 SelectedSkillIds 字段**

找到 `IncidentSession` 结构体，新增字段：

```go
type IncidentSession struct {
	// ... 现有字段 ...
	SelectedSkillIds []string `json:"selected_skill_ids,omitempty"`
}
```

- [ ] **Step 2: IncidentTurn 新增 SelectedSkillIds 字段**

```go
type IncidentTurn struct {
	// ... 现有字段 ...
	SelectedSkillIds []string `json:"selected_skill_ids,omitempty"`
}
```

- [ ] **Step 3: CreateAIOpsIncident 接收 selectedSkillIds**

修改 `CreateAIOpsIncident` 函数签名和实现：

```go
func CreateAIOpsIncident(ctx context.Context, query, engine string, selectedSkillIds []string) (*IncidentSession, error) {
	// ... 现有逻辑 ...
	incident.SelectedSkillIds = selectedSkillIds
	// 首轮 turn 也存储
	incident.Turns[0].SelectedSkillIds = selectedSkillIds
	// ...
}
```

- [ ] **Step 4: AppendAIOpsIncidentTurn 接收 selectedSkillIds**

```go
func AppendAIOpsIncidentTurn(ctx context.Context, incidentID, query string, selectedSkillIds []string) (*IncidentSession, error) {
	// ... 现有逻辑 ...
	newTurn.SelectedSkillIds = selectedSkillIds
	// ...
}
```

- [ ] **Step 5: 运行测试确认**

Run: `go test ./internal/ai/service/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/service/ai_ops_incident.go
git commit -m "feat: IncidentSession/Turn 存储 selectedSkillIds"
```

---

## Task 7: 后端 Controller 透传 selectedSkillIds

**Files:**
- Modify: `internal/controller/chat/chat_v1_ai_ops_incident.go:15-38`

- [ ] **Step 1: AIOpsIncidentCreate 透传 selectedSkillIds**

```go
func (c *ControllerV1) AIOpsIncidentCreate(ctx context.Context, req *v1.AIOpsIncidentCreateReq) (res *v1.AIOpsIncidentRes, err error) {
	if ctx, _, err = checkAndGuardPrompt(ctx, req.Query); err != nil {
		return nil, err
	}
	incident, err := app.CreateAIOpsIncident(ctx, req.Query, req.Engine, req.SelectedSkillIds)
	if err != nil {
		return nil, err
	}
	return &v1.AIOpsIncidentRes{Incident: toAIOpsIncident(ctx, incident)}, nil
}
```

- [ ] **Step 2: AIOpsIncidentTurn 透传 selectedSkillIds**

```go
func (c *ControllerV1) AIOpsIncidentTurn(ctx context.Context, req *v1.AIOpsIncidentTurnReq) (res *v1.AIOpsIncidentRes, err error) {
	if ctx, _, err = checkAndGuardPrompt(ctx, req.Query); err != nil {
		return nil, err
	}
	incident, err := app.AppendAIOpsIncidentTurn(ctx, req.IncidentID, req.Query, req.SelectedSkillIds)
	if err != nil {
		if errors.Is(err, app.ErrIncidentTurnRunning) {
			return nil, errors.New("incident turn is still running")
		}
		return nil, err
	}
	return &v1.AIOpsIncidentRes{Incident: toAIOpsIncident(ctx, incident)}, nil
}
```

- [ ] **Step 3: 运行构建确认**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/controller/chat/chat_v1_ai_ops_incident.go
git commit -m "feat: Controller 透传 selectedSkillIds 到 App 层"
```

---

## Task 8: 执行层注入 selectedSkillIds 到 context

**Files:**
- Modify: `internal/ai/service/ai_ops_incident_exec.go:22-41`

- [ ] **Step 1: executeIncidentTurn 注入 selectedSkillIds**

```go
func executeIncidentTurn(ctx context.Context, incidentID, turnID string) {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return
	}
	incident, err := store.Get(ctx, incidentID)
	if err != nil {
		return
	}
	turn, ok := incidentTurnByID(incident, turnID)
	if !ok {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, incidentTurnTimeout(ctx))
	defer cancel()
	runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, incident.SessionID)
	runCtx = WithAIOpsEngine(runCtx, incident.EngineStrategy)
	runCtx = WithAIOpsIncidentContext(runCtx, incidentContext(ctx, incident, turnID))
	runCtx = withIncidentEventSink(runCtx, store, incidentID, turnID)
	// 新增: 注入 selectedSkillIds
	runCtx = skills.WithSelectedSkillIDs(runCtx, incident.SelectedSkillIds)
	response, runErr := runIncidentAIOps(runCtx, turn.UserQuery)
	// ... 后续逻辑不变 ...
}
```

- [ ] **Step 2: 运行测试确认**

Run: `go test ./internal/ai/service/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/service/ai_ops_incident_exec.go
git commit -m "feat: AIOps 执行层注入 selectedSkillIds 到 context"
```

---

## Task 9: AIOps Service 注入 selected skills focus hints

**Files:**
- Modify: `internal/ai/service/ai_ops_service.go:107-110`

- [ ] **Step 1: 在 enrichedQuery 构建后注入 selected skills**

找到 `skillFocusCollector.Collect(query)` 之后的位置，增加：

```go
// 现有: 自动匹配的 focus hints
if hints := skillFocusCollector.Collect(query); len(hints) > 0 {
	enrichedQuery = enrichedQuery + "\n\n场景分析方向（基于 Skill 匹配）：\n" + skills.FormatFocusHints(hints)
	g.Log().Infof(ctx, "[AIOps] skill focus injected: %d hints", len(hints))
}

// 新增: 用户显式选择的 skills focus hints
if selectedSkillIDs := skills.SelectedSkillIDsFromContext(ctx); len(selectedSkillIDs) > 0 {
	if selectedHints := skillFocusCollector.ResolveSelected(selectedSkillIDs); len(selectedHints) > 0 {
		enrichedQuery += "\n\n用户指定的分析方向：\n" + skills.FormatFocusHints(selectedHints)
		g.Log().Infof(ctx, "[AIOps] selected skills focus injected: %d hints", len(selectedHints))
	}
}
```

- [ ] **Step 2: FocusCollector 新增 ResolveSelected 方法**

```go
// internal/ai/skills/focus_collector.go
func (fc *FocusCollector) ResolveSelected(selectedSkillIDs []string) []FocusHint {
	if len(selectedSkillIDs) == 0 {
		return nil
	}
	var hints []FocusHint
	for _, id := range selectedSkillIDs {
		name := id
		if strings.HasPrefix(id, "user-skill:") {
			name = strings.TrimPrefix(id, "user-skill:")
		}
		for _, reg := range fc.registries {
			if reg == nil {
				continue
			}
			if skill := reg.SkillByName(name); skill != nil {
				if fp, ok := skill.(FocusProvider); ok {
					hints = append(hints, FocusHint{
						Skill:  skill.Name(),
						Focus:  fp.Focus(),
						Domain: reg.Domain(),
					})
				}
				break
			}
		}
	}
	return hints
}
```

- [ ] **Step 3: 运行测试确认**

Run: `go test ./internal/ai/service/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/service/ai_ops_service.go internal/ai/skills/focus_collector.go
git commit -m "feat: AIOps 注入用户选择的 skills focus hints"
```

---

## Task 10: 前端对齐 SKILL_GROUPS

**Files:**
- Modify: `frontend/src/lib/utils.ts:47-155`

- [ ] **Step 1: 新增 3 个缺失的 Skill 到 logs domain**

在 `SKILL_GROUPS` 的 logs skills 数组中新增：

```typescript
{
  id: 'logs_service_offline_panic_trace',
  label: '服务离线追踪',
  description: '追踪服务下线、pod 重启、crashloop 和 panic 证据。',
  domain: 'logs',
  promptFocus: '优先检查 panic、stack trace、nil pointer、restart reason、crashloop、oom 信号。',
},
{
  id: 'logs_auth_failure_trace',
  label: '鉴权失败追踪',
  description: '追踪登录、token 和鉴权失败。',
  domain: 'logs',
  promptFocus: '优先检查 login、token、jwt、forbidden、unauthorized、permission denied 信号。',
},
```

- [ ] **Step 2: 新增 1 个缺失的 Skill 到 knowledge domain**

在 `SKILL_GROUPS` 的 knowledge skills 数组中新增：

```typescript
{
  id: 'knowledge_service_error_code_lookup',
  label: '错误码查询',
  description: '检索服务错误码说明和排查步骤。',
  domain: 'knowledge',
  promptFocus: '优先查询错误码含义、常见原因、受影响依赖和初步排查步骤。',
},
```

- [ ] **Step 3: 新增 getSkillLabelById 工具函数**

```typescript
// frontend/src/lib/utils.ts
export function getSkillLabelById(id: string): string {
  const builtin = findSkillsByIds([id])
  if (builtin.length > 0) return builtin[0].label
  if (id.startsWith('user-skill:')) {
    return id.replace('user-skill:', '')
  }
  return id
}
```

- [ ] **Step 4: 运行构建确认**

Run: `cd frontend && npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/utils.ts
git commit -m "feat: 对齐前端 SKILL_GROUPS 到后端 registry，新增 getSkillLabelById"
```

---

## Task 11: SkillPanel 所有模式可见

**Files:**
- Modify: `frontend/src/components/sidebar/Sidebar.tsx:83`

- [ ] **Step 1: 移除 workbenchMode 条件**

```diff
- {workbenchMode === 'chat' && <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />}
+ <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />
```

- [ ] **Step 2: 运行构建确认**

Run: `cd frontend && npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/sidebar/Sidebar.tsx
git commit -m "feat: SkillPanel 在所有模式下可见"
```

---

## Task 12: 输入框上方显示已启用 Skill 标签

**Files:**
- Modify: `frontend/src/components/chat/ChatInput.tsx`

- [ ] **Step 1: 在输入框上方添加 Skill 标签栏**

在 `ChatInput` 组件中，找到输入框容器，在其上方添加标签栏：

```tsx
// frontend/src/components/chat/ChatInput.tsx
import { getSkillLabelById } from '../../lib/utils'

// 在组件 props 中接收 onSelectedSkillIdsChange
interface Props {
  // ... 现有 props ...
  selectedSkillIds: string[]
  onSelectedSkillIdsChange?: (ids: string[]) => void
}

// 在 return 中，输入框上方添加：
{selectedSkillIds.length > 0 && onSelectedSkillIdsChange && (
  <div className="flex flex-wrap gap-1.5 px-3 pt-2">
    {selectedSkillIds.map((id) => (
      <span
        key={id}
        className="inline-flex items-center gap-1 px-2 py-0.5 bg-sky-100 text-sky-700 rounded-full text-xs cursor-pointer hover:bg-sky-200 transition-colors"
        onClick={() => onSelectedSkillIdsChange(selectedSkillIds.filter((s) => s !== id))}
      >
        {getSkillLabelById(id)}
        <span className="text-sky-400 hover:text-sky-600">×</span>
      </span>
    ))}
  </div>
)}
```

- [ ] **Step 2: 确保 ChatInput 父组件传入 props**

检查 `AgentWorkbenchView.tsx` 或 `ChatView.tsx` 中 `ChatInput` 的调用，确保传入 `selectedSkillIds` 和 `onSelectedSkillIdsChange`。

- [ ] **Step 3: 运行构建确认**

Run: `cd frontend && npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/chat/ChatInput.tsx
git commit -m "feat: 输入框上方显示已启用 Skill 标签，支持点击取消"
```

---

## Task 13: 前端 AIOps 调用透传 selectedSkillIds

**Files:**
- Modify: `frontend/src/hooks/useIncidents.ts:153-176,178-189`
- Modify: `frontend/src/App.tsx:106-112,189-191`

- [ ] **Step 1: useIncidents createIncident 接收 selectedSkillIds**

```typescript
// frontend/src/hooks/useIncidents.ts
const createIncident = useCallback(
  async (query: string, engine: AIOpsEngine, selectedSkillIds?: string[]) => {
    setError(null)
    closeEvents()
    setIsLoading(true)
    try {
      const res = await fetch(`${getApiBaseUrl()}/ai_ops/incidents`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query, engine, selected_skill_ids: selectedSkillIds }),
      })
      // ... 后续逻辑不变 ...
    }
  },
  [closeEvents, refreshList, subscribe],
)
```

- [ ] **Step 2: useIncidents appendTurn 接收 selectedSkillIds**

```typescript
const appendTurn = useCallback(
  async (query: string, selectedSkillIds?: string[]) => {
    if (!incident) {
      throw new Error('请先创建事故。')
    }
    setError(null)
    closeEvents()
    setIsLoading(true)
    try {
      const res = await fetch(
        `${getApiBaseUrl()}/ai_ops/incidents/${encodeURIComponent(incident.incident_id)}/turns`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, selected_skill_ids: selectedSkillIds }),
        },
      )
      // ... 后续逻辑不变 ...
    }
  },
  [incident, closeEvents, subscribe],
)
```

- [ ] **Step 3: App.tsx handleStartAIOps 传 selectedSkillIds**

```typescript
const handleStartAIOps = useCallback(
  (query: string) => {
    setWorkbenchMode('aiops')
    void incidents.createIncident(query, aiOpsEngine, selectedSkillIds).catch(() => undefined)
  },
  [aiOpsEngine, incidents, selectedSkillIds],
)
```

- [ ] **Step 4: App.tsx onAppend 传 selectedSkillIds**

找到 `IncidentView` 的 `onAppend` prop：

```typescript
onAppend={(query) => {
  void incidents.appendTurn(query, selectedSkillIds).catch(() => undefined)
}}
```

- [ ] **Step 5: 运行构建确认**

Run: `cd frontend && npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/hooks/useIncidents.ts frontend/src/App.tsx
git commit -m "feat: 前端 AIOps 调用透传 selectedSkillIds"
```

---

## Task 14: 全量验证

- [ ] **Step 1: 后端全量测试**

Run: `go test ./...`
Expected: ALL PASS

- [ ] **Step 2: 前端构建**

Run: `cd frontend && npm run build`
Expected: BUILD SUCCESS

- [ ] **Step 3: 手动端到端验证清单**

- [ ] Chat 模式: 选择 built-in Skill → 发送消息 → 后端日志确认 `selected_skills` 包含选中的 Skill
- [ ] Chat 模式: 创建并审批自定义 Skill → 选择 → 发送消息 → 确认 GenericSkill 被调用
- [ ] AIOps 模式: 选择 Skill → 创建事故 → 确认 incident session 存储了 selected_skill_ids
- [ ] AIOps 模式: 追加轮次时确认 selected_skill_ids 也被传递
- [ ] AIOps 模式: 确认 enrichedQuery 中包含用户指定的分析方向
- [ ] 共享 registry: 确认 UserSkillLoader 注册的 skill 在 ProgressiveDisclosure 和 FocusCollector 中可见
- [ ] 前端标签: 输入框上方显示已启用 Skill，点击可取消
