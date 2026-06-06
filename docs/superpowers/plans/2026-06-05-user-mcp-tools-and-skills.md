# 用户自定义 MCP 工具 & Skill 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 允许用户在前端动态添加 MCP 工具和 Skill，扩展 OpsCaptain 的能力边界。

**Architecture:** GenericSkill 实现 Skill 接口，内部调用 DynamicMCPRegistry 管理的 MCP 工具，用模板化解析输出。用户配置存储为 JSON 文件，启动时加载 + CRUD 后热更新到现有 Registry 体系。

**Tech Stack:** Go 1.24, GoFrame v2, eino MCP client, React 18, TypeScript, Tailwind CSS

---

## 文件结构

### 后端新增
| 文件 | 职责 |
|------|------|
| `internal/ai/skills/user_types.go` | UserMCPTool, UserSkill, UserRegistryData 类型定义 |
| `internal/ai/skills/user_skill_store.go` | UserSkillStore 接口 + fileUserSkillStore 实现 |
| `internal/ai/skills/user_skill_store_test.go` | Store 单元测试 |
| `internal/ai/skills/generic_skill.go` | GenericSkill 实现 Skill + FocusProvider |
| `internal/ai/skills/generic_skill_test.go` | GenericSkill 单元测试 |
| `internal/ai/skills/user_skill_loader.go` | 启动加载 + 热更新逻辑 |
| `internal/ai/tools/dynamic_mcp_registry.go` | DynamicMCPRegistry 管理多个 MCP 连接 |
| `internal/ai/tools/dynamic_mcp_registry_test.go` | DynamicMCPRegistry 单元测试 |
| `api/chat/v1/user_tools.go` | API 请求/响应类型 |
| `internal/controller/chat/chat_v1_mcp_tools.go` | MCP 工具 CRUD controller |
| `internal/controller/chat/chat_v1_user_skills.go` | Skill CRUD controller |

### 后端修改
| 文件 | 变更 |
|------|------|
| `internal/ai/skills/registry.go` | 新增 Unregister(name) 方法 |
| `internal/ai/tools/tiered_tools.go` | BuildTieredTools 追加用户工具 |
| `internal/controller/chat/chat_new.go` | 注册新 controller |
| `manifest/config/config.yaml` | 新增 user_tools 配置段 |

### 前端新增
| 文件 | 职责 |
|------|------|
| `frontend/src/types/userTools.ts` | TypeScript 类型 |
| `frontend/src/hooks/useUserTools.ts` | API 调用 hook |
| `frontend/src/components/settings/ToolManager.tsx` | MCP 工具列表 + CRUD |
| `frontend/src/components/settings/SkillManager.tsx` | Skill 列表 + CRUD |
| `frontend/src/components/settings/MCPToolForm.tsx` | MCP 工具分步表单 |
| `frontend/src/components/settings/UserSkillForm.tsx` | Skill 分步表单 |
| `frontend/src/components/settings/ApprovalBadge.tsx` | 状态徽章 |
| `frontend/src/components/settings/SettingsView.tsx` | 设置页面容器（标签页切换） |

### 前端修改
| 文件 | 变更 |
|------|------|
| `frontend/src/types/chat.ts` | WorkbenchMode 新增 'settings' |
| `frontend/src/App.tsx` | 新增 settings 视图分支 |
| `frontend/src/components/sidebar/Sidebar.tsx` | 底部增加管理入口 |
| `frontend/src/components/sidebar/SkillPanel.tsx` | 渲染用户 Skill（🧩 图标） |
| `frontend/src/lib/utils.ts` | SKILL_GROUPS 动态合并用户 Skill |

---

## Task 1: 数据模型与持久化

**Files:**
- Create: `internal/ai/skills/user_types.go`
- Create: `internal/ai/skills/user_skill_store.go`
- Create: `internal/ai/skills/user_skill_store_test.go`

### Step 1: 创建 user_types.go

```go
// internal/ai/skills/user_types.go
package skills

import "time"

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusDisabled = "disabled"

	TransportSSE  = "sse"
	TransportHTTP = "http"

	ParserJSONArray   = "json_array"
	ParserJSONNested  = "json_nested"
	ParserLogLines    = "log_lines"
	ParserRaw         = "raw"

	DomainMetrics   = "metrics"
	DomainLogs      = "logs"
	DomainKnowledge = "knowledge"
	DomainCustom    = "custom"
)

type UserMCPTool struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Transport   string         `json:"transport"`
	EndpointURL string         `json:"endpoint_url"`
	HTTPURL     string         `json:"http_url,omitempty"`
	AuthToken   string         `json:"auth_token,omitempty"`
	ToolName    string         `json:"tool_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	TimeoutMs   int            `json:"timeout_ms"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	CreatedBy   string         `json:"created_by"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	ApprovedBy  string         `json:"approved_by,omitempty"`
}

type UserSkill struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Domain       string    `json:"domain"`
	ToolRefID    string    `json:"tool_ref_id"`
	Keywords     []string  `json:"keywords"`
	Focus        string    `json:"focus,omitempty"`
	OutputParser string    `json:"output_parser"`
	JSONPath     string    `json:"json_path,omitempty"`
	Tier         int       `json:"tier"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	ApprovedBy   string    `json:"approved_by,omitempty"`
}

type UserRegistryData struct {
	Tools  []UserMCPTool `json:"tools"`
	Skills []UserSkill   `json:"skills"`
}
```

### Step 2: 创建 user_skill_store.go

```go
// internal/ai/skills/user_skill_store.go
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type UserSkillStore interface {
	Load(ctx context.Context) (*UserRegistryData, error)
	Save(ctx context.Context, data *UserRegistryData) error
}

type fileUserSkillStore struct {
	mu   sync.Mutex
	path string
}

func NewFileUserSkillStore(path string) UserSkillStore {
	return &fileUserSkillStore{path: path}
}

func (s *fileUserSkillStore) Load(ctx context.Context) (*UserRegistryData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UserRegistryData{}, nil
		}
		return nil, fmt.Errorf("read store: %w", err)
	}

	var result UserRegistryData
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode store: %w", err)
	}
	return &result, nil
}

func (s *fileUserSkillStore) Save(ctx context.Context, data *UserRegistryData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
```

### Step 3: 创建测试

```go
// internal/ai/skills/user_skill_store_test.go
package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileUserSkillStoreLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileUserSkillStore(filepath.Join(dir, "registry.json"))
	data, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(data.Tools) != 0 || len(data.Skills) != 0 {
		t.Fatalf("expected empty data, got tools=%d skills=%d", len(data.Tools), len(data.Skills))
	}
}

func TestFileUserSkillStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewFileUserSkillStore(filepath.Join(dir, "registry.json"))
	ctx := context.Background()

	original := &UserRegistryData{
		Tools: []UserMCPTool{
			{ID: "t1", Name: "test-tool", Transport: "sse", EndpointURL: "http://localhost:8080/sse", Status: StatusPending},
		},
		Skills: []UserSkill{
			{ID: "s1", Name: "test-skill", Domain: "metrics", ToolRefID: "t1", Keywords: []string{"test"}, OutputParser: ParserRaw, Status: StatusApproved},
		},
	}
	if err := store.Save(ctx, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Tools) != 1 || loaded.Tools[0].ID != "t1" {
		t.Fatalf("tool mismatch: %+v", loaded.Tools)
	}
	if len(loaded.Skills) != 1 || loaded.Skills[0].Name != "test-skill" {
		t.Fatalf("skill mismatch: %+v", loaded.Skills)
	}
}

func TestFileUserSkillStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store := NewFileUserSkillStore(path)
	ctx := context.Background()

	if err := store.Save(ctx, &UserRegistryData{
		Tools: []UserMCPTool{{ID: "t1", Name: "tool1"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify no .tmp file left behind
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not exist")
	}
}
```

### Step 4: 运行测试

```bash
go test ./internal/ai/skills/ -run TestFileUserSkillStore -v
```

### Step 5: 提交

```bash
git add internal/ai/skills/user_types.go internal/ai/skills/user_skill_store.go internal/ai/skills/user_skill_store_test.go
git commit -m "feat: add user tool/skill data models and file store"
```

---

## Task 2: Registry.Unregister 方法

**Files:**
- Modify: `internal/ai/skills/registry.go`
- Modify: `internal/ai/skills/registry_test.go`

### Step 1: 添加 Unregister 方法

在 `registry.go` 的 `Register` 方法之后添加：

```go
// Unregister removes a skill by name. Returns true if the skill was found and removed.
func (r *Registry) Unregister(name string) bool {
	if r == nil || len(r.skills) == 0 {
		return false
	}
	target := strings.TrimSpace(name)
	if target == "" {
		return false
	}
	for i, skill := range r.skills {
		if strings.EqualFold(skill.Name(), target) {
			r.skills = append(r.skills[:i], r.skills[i+1:]...)
			return true
		}
	}
	return false
}
```

### Step 2: 添加测试

在 `registry_test.go` 中添加：

```go
func TestRegistryUnregister(t *testing.T) {
	registry, err := NewRegistry("test",
		[]Skill{
			&fakeSkill{name: "skill_a", match: false},
			&fakeSkill{name: "skill_b", match: true},
		},
	)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	removed := registry.Unregister("skill_a")
	if !removed {
		t.Fatal("expected skill_a to be removed")
	}
	if len(registry.SkillNames()) != 1 || registry.SkillNames()[0] != "skill_b" {
		t.Fatalf("expected only skill_b, got %v", registry.SkillNames())
	}

	// Removing non-existent skill returns false
	removed = registry.Unregister("nonexistent")
	if removed {
		t.Fatal("expected false for non-existent skill")
	}
}
```

### Step 3: 运行测试

```bash
go test ./internal/ai/skills/ -run TestRegistryUnregister -v
```

### Step 4: 提交

```bash
git add internal/ai/skills/registry.go internal/ai/skills/registry_test.go
git commit -m "feat: add Registry.Unregister method for dynamic skill removal"
```

---

## Task 3: DynamicMCPRegistry

**Files:**
- Create: `internal/ai/tools/dynamic_mcp_registry.go`
- Create: `internal/ai/tools/dynamic_mcp_registry_test.go`

### Step 1: 创建 DynamicMCPRegistry

```go
// internal/ai/tools/dynamic_mcp_registry.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/skills"

	toolapi "github.com/cloudwego/eino/components/tool"
	e_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/mark3labs/mcp-go/client"
)

type mcpConn struct {
	tool      toolapi.InvokableTool
	config    skills.UserMCPTool
	createdAt time.Time
}

type DynamicMCPRegistry struct {
	mu          sync.RWMutex
	connections map[string]*mcpConn // key = toolID
	whitelist   []*net.IPNet
	timeoutMs   int
}

func NewDynamicMCPRegistry(whitelist []string, timeoutMs int) (*DynamicMCPRegistry, error) {
	nets := make([]*net.IPNet, 0, len(whitelist))
	for _, cidr := range whitelist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipNet)
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &DynamicMCPRegistry{
		connections: make(map[string]*mcpConn),
		whitelist:   nets,
		timeoutMs:   timeoutMs,
	}, nil
}

func (r *DynamicMCPRegistry) Register(ctx context.Context, cfg skills.UserMCPTool) error {
	if err := r.checkWhitelist(cfg.EndpointURL); err != nil {
		return err
	}
	if cfg.HTTPURL != "" {
		if err := r.checkWhitelist(cfg.HTTPURL); err != nil {
			return err
		}
	}

	timeout := r.timeoutMs
	if cfg.TimeoutMs > 0 {
		timeout = cfg.TimeoutMs
	}

	tool, err := r.connectAndDiscover(ctx, cfg, timeout)
	if err != nil {
		return fmt.Errorf("connect to MCP server: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[cfg.ID] = &mcpConn{
		tool:      tool,
		config:    cfg,
		createdAt: time.Now(),
	}
	return nil
}

func (r *DynamicMCPRegistry) Unregister(toolID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, toolID)
}

func (r *DynamicMCPRegistry) Get(toolID string) (toolapi.InvokableTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.connections[toolID]
	if !ok {
		return nil, false
	}
	return conn.tool, true
}

func (r *DynamicMCPRegistry) Invoke(ctx context.Context, toolID, args string) (string, error) {
	tool, ok := r.Get(toolID)
	if !ok {
		return degradedJSON("tool not found: " + toolID), nil
	}

	timeout := r.timeoutMs
	r.mu.RLock()
	if conn, exists := r.connections[toolID]; exists && conn.config.TimeoutMs > 0 {
		timeout = conn.config.TimeoutMs
	}
	r.mu.RUnlock()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	output, err := tool.InvokableRun(callCtx, args)
	if err != nil {
		return degradedJSON(err.Error()), nil
	}
	return output, nil
}

func (r *DynamicMCPRegistry) ListConfigs() []skills.UserMCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]skills.UserMCPTool, 0, len(r.connections))
	for _, conn := range r.connections {
		result = append(result, conn.config)
	}
	return result
}

func (r *DynamicMCPRegistry) checkWhitelist(endpointURL string) error {
	if len(r.whitelist) == 0 {
		return nil // no whitelist configured = allow all
	}
	u, err := url.Parse(endpointURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil {
		// Resolve hostname
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("resolve host %q: %w", host, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IP found for host %q", host)
		}
		ip = ips[0]
	}
	for _, ipNet := range r.whitelist {
		if ipNet.Contains(ip) {
			return nil
		}
	}
	return fmt.Errorf("endpoint %q (IP: %s) is not in network whitelist", endpointURL, ip)
}

func (r *DynamicMCPRegistry) connectAndDiscover(ctx context.Context, cfg skills.UserMCPTool, timeoutMs int) (toolapi.InvokableTool, error) {
	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var mcpClient client.MCPClient
	var err error

	if cfg.Transport == skills.TransportHTTP {
		mcpClient, err = client.NewStreamableHTTPClient(cfg.EndpointURL)
	} else {
		mcpClient, err = client.NewSSEMCPClient(cfg.EndpointURL)
	}
	if err != nil {
		return nil, fmt.Errorf("create MCP client: %w", err)
	}

	if err := mcpClient.Start(connectCtx); err != nil {
		return nil, fmt.Errorf("start MCP client: %w", err)
	}

	initReq := &client.InitializeRequest{}
	initReq.Params.ProtocolVersion = "2024-11-05"
	initReq.Params.ClientInfo = map[string]any{"name": "opscaptain", "version": "1.0"}
	_, err = mcpClient.Initialize(connectCtx, initReq)
	if err != nil {
		return nil, fmt.Errorf("initialize MCP: %w", err)
	}

	tools, err := e_mcp.GetTools(ctx, &e_mcp.Config{Cli: mcpClient})
	if err != nil {
		return nil, fmt.Errorf("discover tools: %w", err)
	}

	// Find the specific tool by name
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == cfg.ToolName {
			if invokable, ok := t.(toolapi.InvokableTool); ok {
				return invokable, nil
			}
		}
	}

	return nil, fmt.Errorf("tool %q not found on MCP server (available tools: %d)", cfg.ToolName, len(tools))
}

func degradedJSON(reason string) string {
	msg := map[string]any{
		"degraded": true,
		"reason":   reason,
	}
	raw, _ := json.Marshal(msg)
	return string(raw)
}
```

### Step 2: 创建测试

```go
// internal/ai/tools/dynamic_mcp_registry_test.go
package tools

import (
	"testing"

	"SuperBizAgent/internal/ai/skills"
)

func TestDynamicMCPRegistryWhitelist(t *testing.T) {
	reg, err := NewDynamicMCPRegistry([]string{"10.0.0.0/8", "192.168.0.0/16"}, 5000)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	// Should pass - private IP
	err = reg.checkWhitelist("http://10.1.2.3:8080/sse")
	if err != nil {
		t.Fatalf("expected 10.x to pass whitelist: %v", err)
	}

	// Should fail - public IP
	err = reg.checkWhitelist("http://8.8.8.8:8080/sse")
	if err == nil {
		t.Fatal("expected 8.8.8.8 to fail whitelist")
	}
}

func TestDynamicMCPRegistryNoWhitelist(t *testing.T) {
	reg, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	// No whitelist = allow all
	err = reg.checkWhitelist("http://8.8.8.8:8080/sse")
	if err != nil {
		t.Fatalf("expected no whitelist to allow all: %v", err)
	}
}

func TestDynamicMCPRegistryGetNotFound(t *testing.T) {
	reg, _ := NewDynamicMCPRegistry(nil, 5000)
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("expected false for non-existent tool")
	}
}

func TestDynamicMCPRegistryInvokeNotFound(t *testing.T) {
	reg, _ := NewDynamicMCPRegistry(nil, 5000)
	output, err := reg.Invoke(nil, "nonexistent", "{}")
	if err != nil {
		t.Fatalf("invoke should not return error, got: %v", err)
	}
	if output == "" {
		t.Fatal("expected degraded JSON output")
	}
}

func TestDynamicMCPRegistryUnregister(t *testing.T) {
	reg, _ := NewDynamicMCPRegistry(nil, 5000)
	reg.mu.Lock()
	reg.connections["t1"] = &mcpConn{
		config: skills.UserMCPTool{ID: "t1", Name: "test"},
	}
	reg.mu.Unlock()

	reg.Unregister("t1")
	_, ok := reg.Get("t1")
	if ok {
		t.Fatal("expected tool to be removed after unregister")
	}
}
```

### Step 3: 运行测试

```bash
go test ./internal/ai/tools/ -run TestDynamicMCPRegistry -v
```

### Step 4: 提交

```bash
git add internal/ai/tools/dynamic_mcp_registry.go internal/ai/tools/dynamic_mcp_registry_test.go
git commit -m "feat: add DynamicMCPRegistry for user-defined MCP tools"
```

---

## Task 4: GenericSkill

**Files:**
- Create: `internal/ai/skills/generic_skill.go`
- Create: `internal/ai/skills/generic_skill_test.go`

### Step 1: 创建 GenericSkill

```go
// internal/ai/skills/generic_skill.go
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

// MCPInvoker is the interface GenericSkill uses to call MCP tools.
// DynamicMCPRegistry implements this.
type MCPInvoker interface {
	Invoke(ctx context.Context, toolID, args string) (string, error)
}

type GenericSkill struct {
	config UserSkill
	invoker MCPInvoker
}

func NewGenericSkill(config UserSkill, invoker MCPInvoker) *GenericSkill {
	return &GenericSkill{config: config, invoker: invoker}
}

func (s *GenericSkill) Name() string        { return s.config.Name }
func (s *GenericSkill) Description() string { return s.config.Description }
func (s *GenericSkill) Focus() string       { return s.config.Focus }

func (s *GenericSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil || len(s.config.Keywords) == 0 {
		return false
	}
	return ContainsAny(task.Goal, s.config.Keywords...)
}

func (s *GenericSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	args := buildGenericInput(task)
	output, err := s.invoker.Invoke(ctx, s.config.ToolRefID, args)
	if err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      "user_skill",
			Status:     protocol.ResultStatusDegraded,
			Summary:    fmt.Sprintf("user tool invoke failed: %v", err),
			Confidence: 0.25,
			Metadata: map[string]any{
				"user_skill": true,
				"skill_name": s.config.Name,
				"tool_id":    s.config.ToolRefID,
				"error":      err.Error(),
			},
		}, nil
	}

	evidence := s.parseOutput(output)
	status := protocol.ResultStatusSucceeded
	confidence := 0.70
	summary := fmt.Sprintf("user skill %s returned %d evidence items", s.config.Name, len(evidence))

	if len(evidence) == 0 {
		status = protocol.ResultStatusDegraded
		confidence = 0.40
		summary = fmt.Sprintf("user skill %s returned no extractable evidence", s.config.Name)
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      "user_skill",
		Status:     status,
		Summary:    summary,
		Confidence: confidence,
		Evidence:   evidence,
		Metadata: map[string]any{
			"user_skill":    true,
			"skill_name":    s.config.Name,
			"tool_id":       s.config.ToolRefID,
			"parser_mode":   s.config.OutputParser,
			"output_length": len(output),
		},
	}, nil
}

func buildGenericInput(task *protocol.TaskEnvelope) string {
	args := map[string]any{
		"query": task.Goal,
	}
	raw, _ := json.Marshal(args)
	return string(raw)
}

func (s *GenericSkill) parseOutput(output string) []protocol.EvidenceItem {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	switch s.config.OutputParser {
	case ParserJSONArray:
		return parseJSONArray(trimmed)
	case ParserJSONNested:
		return parseJSONNested(trimmed, s.config.JSONPath)
	case ParserLogLines:
		return parseLogLines(trimmed)
	case ParserRaw:
		return parseRaw(trimmed)
	default:
		return parseRaw(trimmed)
	}
}

func parseJSONArray(output string) []protocol.EvidenceItem {
	var items []map[string]any
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return parseRaw(output)
	}
	result := make([]protocol.EvidenceItem, 0, len(items))
	for i, item := range items {
		title := firstStringFromMap(item, "title", "name", "id")
		if title == "" {
			title = fmt.Sprintf("item-%d", i+1)
		}
		content := firstStringFromMap(item, "content", "text", "message", "description", "snippet")
		if content == "" {
			raw, _ := json.Marshal(item)
			content = string(raw)
		}
		result = append(result, protocol.EvidenceItem{
			SourceType: "user_tool",
			SourceID:   title,
			Title:      title,
			Snippet:    truncate(content, 200),
			Score:      0.70 - float64(i)*0.05,
		})
	}
	return result
}

func parseJSONNested(output, jsonPath string) []protocol.EvidenceItem {
	var root map[string]any
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		return parseRaw(output)
	}
	parts := strings.Split(jsonPath, ".")
	var current any = root
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return parseRaw(output)
		}
		current = m[part]
	}
	arr, ok := current.([]any)
	if !ok {
		return parseRaw(output)
	}
	arrJSON, _ := json.Marshal(arr)
	return parseJSONArray(string(arrJSON))
}

func parseLogLines(output string) []protocol.EvidenceItem {
	lines := strings.Split(output, "\n")
	result := make([]protocol.EvidenceItem, 0, len(lines))
	idx := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx++
		result = append(result, protocol.EvidenceItem{
			SourceType: "user_tool",
			SourceID:   fmt.Sprintf("line-%d", idx),
			Title:      fmt.Sprintf("log line %d", idx),
			Snippet:    truncate(line, 200),
			Score:      0.65 - float64(idx)*0.05,
		})
	}
	return result
}

func parseRaw(output string) []protocol.EvidenceItem {
	return []protocol.EvidenceItem{
		{
			SourceType: "user_tool",
			SourceID:   "raw-output",
			Title:      "raw tool output",
			Snippet:    truncate(output, 200),
			Score:      0.50,
		},
	}
}

func firstStringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
```

### Step 2: 创建测试

```go
// internal/ai/skills/generic_skill_test.go
package skills

import (
	"context"
	"encoding/json"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

type mockInvoker struct {
	output string
	err    error
}

func (m *mockInvoker) Invoke(ctx context.Context, toolID, args string) (string, error) {
	return m.output, m.err
}

func TestGenericSkillMatch(t *testing.T) {
	s := NewGenericSkill(UserSkill{
		Keywords: []string{"test", "query"},
	}, &mockInvoker{})

	task := &protocol.TaskEnvelope{Goal: "please test this query"}
	if !s.Match(task) {
		t.Fatal("expected match")
	}

	task2 := &protocol.TaskEnvelope{Goal: "unrelated topic"}
	if s.Match(task2) {
		t.Fatal("expected no match")
	}
}

func TestGenericSkillRunJSONArray(t *testing.T) {
	items := []map[string]any{
		{"title": "alert-1", "content": "CPU high"},
		{"title": "alert-2", "content": "Memory full"},
	}
	output, _ := json.Marshal(items)

	s := NewGenericSkill(UserSkill{
		Name:         "test-skill",
		OutputParser: ParserJSONArray,
	}, &mockInvoker{output: string(output)})

	result, err := s.Run(context.Background(), &protocol.TaskEnvelope{TaskID: "t1", Goal: "test"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", result.Status)
	}
	if len(result.Evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(result.Evidence))
	}
	if result.Evidence[0].Title != "alert-1" {
		t.Fatalf("expected title alert-1, got %s", result.Evidence[0].Title)
	}
}

func TestGenericSkillRunJSONNested(t *testing.T) {
	root := map[string]any{
		"data": map[string]any{
			"items": []map[string]any{
				{"name": "pod-1", "message": "OOMKilled"},
			},
		},
	}
	output, _ := json.Marshal(root)

	s := NewGenericSkill(UserSkill{
		Name:         "test-nested",
		OutputParser: ParserJSONNested,
		JSONPath:     "data.items",
	}, &mockInvoker{output: string(output)})

	result, err := s.Run(context.Background(), &protocol.TaskEnvelope{TaskID: "t1", Goal: "test"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(result.Evidence))
	}
	if result.Evidence[0].Title != "pod-1" {
		t.Fatalf("expected pod-1, got %s", result.Evidence[0].Title)
	}
}

func TestGenericSkillRunLogLines(t *testing.T) {
	s := NewGenericSkill(UserSkill{
		Name:         "test-logs",
		OutputParser: ParserLogLines,
	}, &mockInvoker{output: "line1\nline2\n\nline3"})

	result, err := s.Run(context.Background(), &protocol.TaskEnvelope{TaskID: "t1", Goal: "test"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Evidence) != 3 {
		t.Fatalf("expected 3 evidence, got %d", len(result.Evidence))
	}
}

func TestGenericSkillRunEmptyOutput(t *testing.T) {
	s := NewGenericSkill(UserSkill{
		Name:         "test-empty",
		OutputParser: ParserRaw,
	}, &mockInvoker{output: ""})

	result, err := s.Run(context.Background(), &protocol.TaskEnvelope{TaskID: "t1", Goal: "test"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded for empty output, got %s", result.Status)
	}
	if result.Confidence != 0.40 {
		t.Fatalf("expected confidence 0.40, got %f", result.Confidence)
	}
}

func TestGenericSkillRunInvokeError(t *testing.T) {
	s := NewGenericSkill(UserSkill{
		Name: "test-err",
	}, &mockInvoker{err: context.DeadlineExceeded})

	result, err := s.Run(context.Background(), &protocol.TaskEnvelope{TaskID: "t1", Goal: "test"})
	if err != nil {
		t.Fatalf("run should not return error, got: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded, got %s", result.Status)
	}
	if result.Confidence != 0.25 {
		t.Fatalf("expected confidence 0.25, got %f", result.Confidence)
	}
}
```

### Step 3: 运行测试

```bash
go test ./internal/ai/skills/ -run TestGenericSkill -v
```

### Step 4: 提交

```bash
git add internal/ai/skills/generic_skill.go internal/ai/skills/generic_skill_test.go
git commit -m "feat: add GenericSkill with template-based output parsing"
```

---

## Task 5: UserSkillLoader（启动加载 + 热更新）

**Files:**
- Create: `internal/ai/skills/user_skill_loader.go`

### Step 1: 创建 loader

```go
// internal/ai/skills/user_skill_loader.go
package skills

import (
	"context"
	"fmt"
	"log"
)

// UserSkillLoader loads approved user skills from store and registers them into domain registries.
type UserSkillLoader struct {
	store     UserSkillStore
	mcpReg    MCPInvoker
	metricsR  *Registry
	logsR     *Registry
	knowledgeR *Registry
	customR   *Registry
}

func NewUserSkillLoader(store UserSkillStore, mcpReg MCPInvoker,
	metricsR, logsR, knowledgeR, customR *Registry) *UserSkillLoader {
	return &UserSkillLoader{
		store:      store,
		mcpReg:     mcpReg,
		metricsR:   metricsR,
		logsR:      logsR,
		knowledgeR: knowledgeR,
		customR:    customR,
	}
}

// Reload clears all user skills from registries and re-loads approved ones from store.
func (l *UserSkillLoader) Reload(ctx context.Context) error {
	data, err := l.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("load user skills: %w", err)
	}

	// Clear existing user skills from all registries
	l.clearUserSkills()

	// Register approved skills
	for _, us := range data.Skills {
		if us.Status != StatusApproved {
			continue
		}
		gs := NewGenericSkill(us, l.mcpReg)
		reg := l.resolveDomain(us.Domain)
		if err := reg.Register(gs); err != nil {
			log.Printf("[user_skill_loader] failed to register skill %q: %v", us.Name, err)
		}
	}
	return nil
}

func (l *UserSkillLoader) clearUserSkills() {
	registries := []*Registry{l.metricsR, l.logsR, l.knowledgeR, l.customR}
	for _, reg := range registries {
		if reg == nil {
			continue
		}
		for _, name := range reg.SkillNames() {
			// Only remove skills that look like user skills (not built-in)
			// We identify user skills by checking if they implement GenericSkill
			if skill := reg.SkillByName(name); skill != nil {
				if _, ok := skill.(*GenericSkill); ok {
					reg.Unregister(name)
				}
			}
		}
	}
}

func (l *UserSkillLoader) resolveDomain(domain string) *Registry {
	switch domain {
	case DomainMetrics:
		return l.metricsR
	case DomainLogs:
		return l.logsR
	case DomainKnowledge:
		return l.knowledgeR
	default:
		if l.customR != nil {
			return l.customR
		}
		// Fallback: create a throwaway registry (should not happen in practice)
		r, _ := NewRegistry("custom", nil)
		return r
	}
}
```

### Step 2: 提交

```bash
git add internal/ai/skills/user_skill_loader.go
git commit -m "feat: add UserSkillLoader for startup load and hot reload"
```

---

## Task 6: 配置变更

**Files:**
- Modify: `manifest/config/config.yaml`

### Step 1: 添加 user_tools 配置段

在 `mcp:` 段之后添加：

```yaml
user_tools:
  store_path: "var/runtime/user_tools/registry.json"
  network_whitelist:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
    - "127.0.0.0/8"
  default_timeout_ms: 5000
```

### Step 2: 提交

```bash
git add manifest/config/config.yaml
git commit -m "feat: add user_tools config section for MCP whitelist and store path"
```

---

## Task 7: API 类型定义

**Files:**
- Create: `api/chat/v1/user_tools.go`

### Step 1: 创建 API 类型

```go
// api/chat/v1/user_tools.go
package chat

import "github.com/gogf/gf/v2/frame/g"

// MCP Tools CRUD
type MCPToolCreateReq struct {
	g.Meta      `path:"/mcp_tools" method:"post" summary:"创建 MCP 工具"`
	Name        string `json:"Name" v:"required|max-length:128#名称不能为空|名称长度不能超过128"`
	Description string `json:"Description" v:"max-length:500"`
	Transport   string `json:"Transport" v:"required|in:sse,http#传输方式不能为空|传输方式必须为 sse 或 http"`
	EndpointURL string `json:"EndpointUrl" v:"required#Endpoint URL 不能为空"`
	HTTPURL     string `json:"HttpUrl"`
	AuthToken   string `json:"AuthToken"`
	ToolName    string `json:"ToolName" v:"required#工具名称不能为空"`
	TimeoutMs   int    `json:"TimeoutMs"`
}

type MCPToolCreateRes struct {
	Tool interface{} `json:"tool"`
}

type MCPToolListReq struct {
	g.Meta `path:"/mcp_tools" method:"get" summary:"列出 MCP 工具"`
}

type MCPToolListRes struct {
	Items []interface{} `json:"items"`
}

type MCPToolUpdateReq struct {
	g.Meta      `path:"/mcp_tools/{ToolId}" method:"put" summary:"更新 MCP 工具"`
	ToolId      string `json:"ToolId" v:"required#工具ID不能为空"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	TimeoutMs   int    `json:"TimeoutMs"`
}

type MCPToolUpdateRes struct {
	Tool interface{} `json:"tool"`
}

type MCPToolDeleteReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}" method:"delete" summary:"删除 MCP 工具"`
	ToolId string `json:"ToolId" v:"required#工具ID不能为空"`
}

type MCPToolDeleteRes struct {
	Success bool `json:"success"`
}

type MCPToolTestReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/test" method:"post" summary:"测试 MCP 工具连接"`
	ToolId string `json:"ToolId" v:"required#工具ID不能为空"`
}

type MCPToolTestRes struct {
	Success bool     `json:"success"`
	Tools   []string `json:"tools,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type MCPToolApproveReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/approve" method:"post" summary:"审批 MCP 工具"`
	ToolId string `json:"ToolId" v:"required#工具ID不能为空"`
}

type MCPToolApproveRes struct {
	Success bool `json:"success"`
}

type MCPToolRejectReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/reject" method:"post" summary:"拒绝 MCP 工具"`
	ToolId string `json:"ToolId" v:"required#工具ID不能为空"`
}

type MCPToolRejectRes struct {
	Success bool `json:"success"`
}

// Skills CRUD
type UserSkillCreateReq struct {
	g.Meta       string   `path:"/skills" method:"post" summary:"创建 Skill"`
	Name         string   `json:"Name" v:"required|max-length:128#名称不能为空|名称长度不能超过128"`
	Description  string   `json:"Description" v:"max-length:500"`
	Domain       string   `json:"Domain" v:"required|in:metrics,logs,knowledge,custom#域不能为空|域必须为 metrics/logs/knowledge/custom"`
	ToolRefId    string   `json:"ToolRefId" v:"required#关联工具不能为空"`
	Keywords     []string `json:"Keywords" v:"required#关键词不能为空"`
	Focus        string   `json:"Focus"`
	OutputParser string   `json:"OutputParser" v:"required|in:json_array,json_nested,log_lines,raw#解析模式不能为空"`
	JSONPath     string   `json:"JsonPath"`
	Tier         int      `json:"Tier"`
}

type UserSkillCreateRes struct {
	Skill interface{} `json:"skill"`
}

type UserSkillListReq struct {
	g.Meta `path:"/skills" method:"get" summary:"列出 Skill"`
}

type UserSkillListRes struct {
	Items []interface{} `json:"items"`
}

type UserSkillUpdateReq struct {
	g.Meta       string   `path:"/skills/{SkillId}" method:"put" summary:"更新 Skill"`
	SkillId      string   `json:"SkillId" v:"required#SkillID不能为空"`
	Name         string   `json:"Name"`
	Description  string   `json:"Description"`
	Domain       string   `json:"Domain"`
	ToolRefId    string   `json:"ToolRefId"`
	Keywords     []string `json:"Keywords"`
	Focus        string   `json:"Focus"`
	OutputParser string   `json:"OutputParser"`
	JSONPath     string   `json:"JsonPath"`
	Tier         int      `json:"Tier"`
}

type UserSkillUpdateRes struct {
	Skill interface{} `json:"skill"`
}

type UserSkillDeleteReq struct {
	g.Meta  `path:"/skills/{SkillId}" method:"delete" summary:"删除 Skill"`
	SkillId string `json:"SkillId" v:"required#SkillID不能为空"`
}

type UserSkillDeleteRes struct {
	Success bool `json:"success"`
}

type UserSkillApproveReq struct {
	g.Meta  `path:"/skills/{SkillId}/approve" method:"post" summary:"审批 Skill"`
	SkillId string `json:"SkillId" v:"required#SkillID不能为空"`
}

type UserSkillApproveRes struct {
	Success bool `json:"success"`
}

type UserSkillRejectReq struct {
	g.Meta  `path:"/skills/{SkillId}/reject" method:"post" summary:"拒绝 Skill"`
	SkillId string `json:"SkillId" v:"required#SkillID不能为空"`
}

type UserSkillRejectRes struct {
	Success bool `json:"success"`
}
```

### Step 2: 提交

```bash
git add api/chat/v1/user_tools.go
git commit -m "feat: add API types for user MCP tools and skills CRUD"
```

---

## Task 8: MCP 工具 Controller

**Files:**
- Create: `internal/controller/chat/chat_v1_mcp_tools.go`

### Step 1: 创建 controller

```go
// internal/controller/chat/chat_v1_mcp_tools.go
package chat

import (
	"context"
	"time"

	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/skills"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

func (c *ControllerV1) MCPToolCreate(ctx context.Context, req *v1.MCPToolCreateReq) (res *v1.MCPToolCreateRes, err error) {
	store := c.userSkillStore
	data, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}

	tool := skills.UserMCPTool{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Transport:   req.Transport,
		EndpointURL: req.EndpointURL,
		HTTPURL:     req.HTTPURL,
		AuthToken:   req.AuthToken,
		ToolName:    req.ToolName,
		TimeoutMs:   req.TimeoutMs,
		Status:      skills.StatusPending,
		CreatedAt:   time.Now(),
		CreatedBy:   g.Request(ctx).GetClientIp(),
	}
	if tool.TimeoutMs == 0 {
		tool.TimeoutMs = 5000
	}

	data.Tools = append(data.Tools, tool)
	if err := store.Save(ctx, data); err != nil {
		return nil, err
	}

	return &v1.MCPToolCreateRes{Tool: tool}, nil
}

func (c *ControllerV1) MCPToolList(ctx context.Context, req *v1.MCPToolListReq) (res *v1.MCPToolListRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]interface{}, len(data.Tools))
	for i, t := range data.Tools {
		items[i] = t
	}
	return &v1.MCPToolListRes{Items: items}, nil
}

func (c *ControllerV1) MCPToolUpdate(ctx context.Context, req *v1.MCPToolUpdateReq) (res *v1.MCPToolUpdateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, t := range data.Tools {
		if t.ID == req.ToolId {
			if req.Name != "" {
				data.Tools[i].Name = req.Name
			}
			if req.Description != "" {
				data.Tools[i].Description = req.Description
			}
			if req.TimeoutMs > 0 {
				data.Tools[i].TimeoutMs = req.TimeoutMs
			}
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			return &v1.MCPToolUpdateRes{Tool: data.Tools[i]}, nil
		}
	}
	return nil, g.NewErrorf("tool %s not found", req.ToolId)
}

func (c *ControllerV1) MCPToolDelete(ctx context.Context, req *v1.MCPToolDeleteReq) (res *v1.MCPToolDeleteRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, t := range data.Tools {
		if t.ID == req.ToolId {
			data.Tools = append(data.Tools[:i], data.Tools[i+1:]...)
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			c.dynamicMCPReg.Unregister(req.ToolId)
			return &v1.MCPToolDeleteRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("tool %s not found", req.ToolId)
}

func (c *ControllerV1) MCPToolTest(ctx context.Context, req *v1.MCPToolTestReq) (res *v1.MCPToolTestRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range data.Tools {
		if t.ID == req.ToolId {
			testCfg := t
			testCfg.Status = skills.StatusApproved //临时标记为 approved 以便连接
			connErr := c.dynamicMCPReg.Register(ctx, testCfg)
			if connErr != nil {
				return &v1.MCPToolTestRes{Success: false, Error: connErr.Error()}, nil
			}
			// 获取发现的工具信息
			configs := c.dynamicMCPReg.ListConfigs()
			var toolNames []string
			for _, cfg := range configs {
				if cfg.ID == req.ToolId {
					toolNames = append(toolNames, cfg.ToolName)
				}
			}
			c.dynamicMCPReg.Unregister(req.ToolId) // 测试完移除
			return &v1.MCPToolTestRes{Success: true, Tools: toolNames}, nil
		}
	}
	return nil, g.NewErrorf("tool %s not found", req.ToolId)
}

func (c *ControllerV1) MCPToolApprove(ctx context.Context, req *v1.MCPToolApproveReq) (res *v1.MCPToolApproveRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i, t := range data.Tools {
		if t.ID == req.ToolId {
			data.Tools[i].Status = skills.StatusApproved
			data.Tools[i].ApprovedAt = &now
			data.Tools[i].ApprovedBy = g.Request(ctx).GetClientIp()
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			// 注册到动态 MCP 注册表
			if regErr := c.dynamicMCPReg.Register(ctx, data.Tools[i]); regErr != nil {
				// 记录但不阻断审批
				g.Log().Warningf(ctx, "MCP register after approve failed: %v", regErr)
			}
			return &v1.MCPToolApproveRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("tool %s not found", req.ToolId)
}

func (c *ControllerV1) MCPToolReject(ctx context.Context, req *v1.MCPToolRejectReq) (res *v1.MCPToolRejectRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, t := range data.Tools {
		if t.ID == req.ToolId {
			data.Tools[i].Status = skills.StatusRejected
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			return &v1.MCPToolRejectRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("tool %s not found", req.ToolId)
}
```

### Step 2: 提交

```bash
git add internal/controller/chat/chat_v1_mcp_tools.go
git commit -m "feat: add MCP tools CRUD controller"
```

---

## Task 9: Skill Controller

**Files:**
- Create: `internal/controller/chat/chat_v1_user_skills.go`

### Step 1: 创建 controller

```go
// internal/controller/chat/chat_v1_user_skills.go
package chat

import (
	"context"
	"time"

	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/skills"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

func (c *ControllerV1) UserSkillCreate(ctx context.Context, req *v1.UserSkillCreateReq) (res *v1.UserSkillCreateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	// Check name uniqueness
	for _, s := range data.Skills {
		if s.Name == req.Name {
			return nil, g.NewErrorf("skill name %q already exists", req.Name)
		}
	}

	tier := req.Tier
	if tier == 0 {
		tier = int(skills.TierSkillGate)
	}

	skill := skills.UserSkill{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Description:  req.Description,
		Domain:       req.Domain,
		ToolRefId:    req.ToolRefId,
		Keywords:     req.Keywords,
		Focus:        req.Focus,
		OutputParser: req.OutputParser,
		JSONPath:     req.JSONPath,
		Tier:         tier,
		Status:       skills.StatusPending,
		CreatedAt:    time.Now(),
		CreatedBy:    g.Request(ctx).GetClientIp(),
	}

	data.Skills = append(data.Skills, skill)
	if err := c.userSkillStore.Save(ctx, data); err != nil {
		return nil, err
	}

	return &v1.UserSkillCreateRes{Skill: skill}, nil
}

func (c *ControllerV1) UserSkillList(ctx context.Context, req *v1.UserSkillListReq) (res *v1.UserSkillListRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]interface{}, len(data.Skills))
	for i, s := range data.Skills {
		items[i] = s
	}
	return &v1.UserSkillListRes{Items: items}, nil
}

func (c *ControllerV1) UserSkillUpdate(ctx context.Context, req *v1.UserSkillUpdateReq) (res *v1.UserSkillUpdateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, s := range data.Skills {
		if s.ID == req.SkillId {
			if req.Name != "" {
				data.Skills[i].Name = req.Name
			}
			if req.Description != "" {
				data.Skills[i].Description = req.Description
			}
			if req.Domain != "" {
				data.Skills[i].Domain = req.Domain
			}
			if req.ToolRefId != "" {
				data.Skills[i].ToolRefId = req.ToolRefId
			}
			if req.Keywords != nil {
				data.Skills[i].Keywords = req.Keywords
			}
			if req.Focus != "" {
				data.Skills[i].Focus = req.Focus
			}
			if req.OutputParser != "" {
				data.Skills[i].OutputParser = req.OutputParser
			}
			if req.JSONPath != "" {
				data.Skills[i].JSONPath = req.JSONPath
			}
			if req.Tier > 0 {
				data.Skills[i].Tier = req.Tier
			}
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			return &v1.UserSkillUpdateRes{Skill: data.Skills[i]}, nil
		}
	}
	return nil, g.NewErrorf("skill %s not found", req.SkillId)
}

func (c *ControllerV1) UserSkillDelete(ctx context.Context, req *v1.UserSkillDeleteReq) (res *v1.UserSkillDeleteRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, s := range data.Skills {
		if s.ID == req.SkillId {
			data.Skills = append(data.Skills[:i], data.Skills[i+1:]...)
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			// 热更新
			if c.userSkillLoader != nil {
				_ = c.userSkillLoader.Reload(ctx)
			}
			return &v1.UserSkillDeleteRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("skill %s not found", req.SkillId)
}

func (c *ControllerV1) UserSkillApprove(ctx context.Context, req *v1.UserSkillApproveReq) (res *v1.UserSkillApproveRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i, s := range data.Skills {
		if s.ID == req.SkillId {
			data.Skills[i].Status = skills.StatusApproved
			data.Skills[i].ApprovedAt = &now
			data.Skills[i].ApprovedBy = g.Request(ctx).GetClientIp()
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			// 热更新
			if c.userSkillLoader != nil {
				_ = c.userSkillLoader.Reload(ctx)
			}
			return &v1.UserSkillApproveRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("skill %s not found", req.SkillId)
}

func (c *ControllerV1) UserSkillReject(ctx context.Context, req *v1.UserSkillRejectReq) (res *v1.UserSkillRejectRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	for i, s := range data.Skills {
		if s.ID == req.SkillId {
			data.Skills[i].Status = skills.StatusRejected
			if err := c.userSkillStore.Save(ctx, data); err != nil {
				return nil, err
			}
			return &v1.UserSkillRejectRes{Success: true}, nil
		}
	}
	return nil, g.NewErrorf("skill %s not found", req.SkillId)
}
```

### Step 2: 提交

```bash
git add internal/controller/chat/chat_v1_user_skills.go
git commit -m "feat: add user skills CRUD controller with hot reload"
```

---

## Task 10: Controller 注册与依赖注入

**Files:**
- Modify: `internal/controller/chat/chat_new.go`
- Modify: `internal/ai/tools/tiered_tools.go`

### Step 1: 修改 chat_new.go

在 `ControllerV1` 结构体中添加新字段：

```go
type ControllerV1 struct {
	service          *sse.Service
	chatApp          *app.ChatApp
	knowledgeApp     *app.KnowledgeApp
	aiopsApp         *app.AIOpsApp
	userSkillStore   skills.UserSkillStore
	dynamicMCPReg    *tools.DynamicMCPRegistry
	userSkillLoader  *skills.UserSkillLoader
}
```

更新构造函数：

```go
func NewV1(chatApp *app.ChatApp, knowledgeApp *app.KnowledgeApp, aiopsApp *app.AIOpsApp,
	userSkillStore skills.UserSkillStore, dynamicMCPReg *tools.DynamicMCPRegistry,
	userSkillLoader *skills.UserSkillLoader) chat.IChatV1 {
	return &ControllerV1{
		service:         sse.New(),
		chatApp:         chatApp,
		knowledgeApp:    knowledgeApp,
		aiopsApp:        aiopsApp,
		userSkillStore:  userSkillStore,
		dynamicMCPReg:   dynamicMCPReg,
		userSkillLoader: userSkillLoader,
	}
}
```

### Step 2: 修改 main.go 初始化

在 `main.go` 中添加初始化逻辑（在现有 `chat.NewV1` 调用处）：

```go
// 初始化用户工具系统
userSkillStore := skills.NewFileUserSkillStore(g.Cfg().MustGet(ctx, "user_tools.store_path").String())
whitelist := g.Cfg().MustGet(ctx, "user_tools.network_whitelist").Strings()
defaultTimeout := g.Cfg().MustGet(ctx, "user_tools.default_timeout_ms").Int()
dynamicMCPReg, err := tools.NewDynamicMCPRegistry(whitelist, defaultTimeout)
if err != nil {
    g.Log().Fatalf(ctx, "init dynamic MCP registry: %v", err)
}

// 创建 custom registry
customReg, _ := skills.NewRegistry("custom", nil)

// 创建 loader
userSkillLoader := skills.NewUserSkillLoader(userSkillStore, dynamicMCPReg,
    metricsAgent.SkillRegistry(), logsAgent.SkillRegistry(), knowledgeAgent.SkillRegistry(), customReg)
if err := userSkillLoader.Reload(ctx); err != nil {
    g.Log().Warningf(ctx, "load user skills: %v", err)
}
```

### Step 3: 修改 tiered_tools.go

在 `BuildTieredTools` 函数末尾追加用户工具：

```go
// 在函数末尾、return 之前添加
if userToolStore != nil {
    data, err := userToolStore.Load(ctx)
    if err == nil {
        for _, t := range data.Tools {
            if t.Status != skills.StatusApproved {
                continue
            }
            if tool, ok := dynamicMCPReg.Get(t.ID); ok {
                tieredTools = append(tieredTools, skills.TieredTool{
                    Tool:    tool,
                    Tier:    skills.TierSkillGate,
                    Domains: []string{skills.DomainCustom},
                })
            }
        }
    }
}
```

### Step 4: 提交

```bash
git add internal/controller/chat/chat_new.go internal/ai/tools/tiered_tools.go main.go
git commit -m "feat: wire up user tool/skill system with dependency injection"
```

---

## Task 11: 前端类型与 Hook

**Files:**
- Create: `frontend/src/types/userTools.ts`
- Create: `frontend/src/hooks/useUserTools.ts`

### Step 1: 创建 TypeScript 类型

```typescript
// frontend/src/types/userTools.ts
export interface UserMCPTool {
  id: string
  name: string
  description: string
  transport: 'sse' | 'http'
  endpoint_url: string
  http_url?: string
  auth_token?: string
  tool_name: string
  input_schema?: Record<string, unknown>
  timeout_ms: number
  status: 'pending' | 'approved' | 'rejected' | 'disabled'
  created_at: string
  created_by: string
  approved_at?: string
  approved_by?: string
}

export interface UserSkill {
  id: string
  name: string
  description: string
  domain: 'metrics' | 'logs' | 'knowledge' | 'custom'
  tool_ref_id: string
  keywords: string[]
  focus?: string
  output_parser: 'json_array' | 'json_nested' | 'log_lines' | 'raw'
  json_path?: string
  tier: number
  status: 'pending' | 'approved' | 'rejected' | 'disabled'
  created_at: string
  created_by: string
  approved_at?: string
  approved_by?: string
}
```

### Step 2: 创建 useUserTools hook

```typescript
// frontend/src/hooks/useUserTools.ts
import { useState, useEffect, useCallback } from 'react'
import { UserMCPTool, UserSkill } from '../types/userTools'

const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export function useUserTools() {
  const [tools, setTools] = useState<UserMCPTool[]>([])
  const [skills, setSkills] = useState<UserSkill[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchTools = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/mcp_tools`)
      const data = await res.json()
      setTools(data.items || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch tools')
    }
  }, [])

  const fetchSkills = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/skills`)
      const data = await res.json()
      setSkills(data.items || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch skills')
    }
  }, [])

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    await Promise.all([fetchTools(), fetchSkills()])
    setLoading(false)
  }, [fetchTools, fetchSkills])

  useEffect(() => { refresh() }, [refresh])

  const createTool = useCallback(async (tool: Partial<UserMCPTool>) => {
    const res = await fetch(`${API_BASE}/mcp_tools`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(tool),
    })
    if (!res.ok) throw new Error((await res.json()).message || 'Create failed')
    await refresh()
  }, [refresh])

  const updateTool = useCallback(async (id: string, updates: Partial<UserMCPTool>) => {
    const res = await fetch(`${API_BASE}/mcp_tools/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates),
    })
    if (!res.ok) throw new Error((await res.json()).message || 'Update failed')
    await refresh()
  }, [refresh])

  const deleteTool = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/mcp_tools/${id}`, { method: 'DELETE' })
    if (!res.ok) throw new Error((await res.json()).message || 'Delete failed')
    await refresh()
  }, [refresh])

  const testTool = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/mcp_tools/${id}/test`, { method: 'POST' })
    return res.json()
  }, [])

  const approveTool = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/mcp_tools/${id}/approve`, { method: 'POST' })
    if (!res.ok) throw new Error('Approve failed')
    await refresh()
  }, [refresh])

  const rejectTool = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/mcp_tools/${id}/reject`, { method: 'POST' })
    if (!res.ok) throw new Error('Reject failed')
    await refresh()
  }, [refresh])

  const createSkill = useCallback(async (skill: Partial<UserSkill>) => {
    const res = await fetch(`${API_BASE}/skills`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(skill),
    })
    if (!res.ok) throw new Error((await res.json()).message || 'Create failed')
    await refresh()
  }, [refresh])

  const updateSkill = useCallback(async (id: string, updates: Partial<UserSkill>) => {
    const res = await fetch(`${API_BASE}/skills/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates),
    })
    if (!res.ok) throw new Error((await res.json()).message || 'Update failed')
    await refresh()
  }, [refresh])

  const deleteSkill = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/skills/${id}`, { method: 'DELETE' })
    if (!res.ok) throw new Error((await res.json()).message || 'Delete failed')
    await refresh()
  }, [refresh])

  const approveSkill = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/skills/${id}/approve`, { method: 'POST' })
    if (!res.ok) throw new Error('Approve failed')
    await refresh()
  }, [refresh])

  const rejectSkill = useCallback(async (id: string) => {
    const res = await fetch(`${API_BASE}/skills/${id}/reject`, { method: 'POST' })
    if (!res.ok) throw new Error('Reject failed')
    await refresh()
  }, [refresh])

  return {
    tools, skills, loading, error,
    createTool, updateTool, deleteTool, testTool, approveTool, rejectTool,
    createSkill, updateSkill, deleteSkill, approveSkill, rejectSkill,
    refresh,
  }
}
```

### Step 3: 提交

```bash
git add frontend/src/types/userTools.ts frontend/src/hooks/useUserTools.ts
git commit -m "feat: add frontend types and API hook for user tools/skills"
```

---

## Task 12: 前端 ApprovalBadge 组件

**Files:**
- Create: `frontend/src/components/settings/ApprovalBadge.tsx`

### Step 1: 创建组件

```tsx
// frontend/src/components/settings/ApprovalBadge.tsx
import React from 'react'

interface Props {
  status: string
}

const statusConfig: Record<string, { label: string; className: string }> = {
  pending: { label: '待审批', className: 'bg-amber-100 text-amber-700' },
  approved: { label: '已审批', className: 'bg-green-100 text-green-700' },
  rejected: { label: '已拒绝', className: 'bg-red-100 text-red-700' },
  disabled: { label: '已禁用', className: 'bg-gray-100 text-gray-500' },
}

export const ApprovalBadge: React.FC<Props> = ({ status }) => {
  const config = statusConfig[status] || statusConfig.pending
  return (
    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${config.className}`}>
      {config.label}
    </span>
  )
}
```

### Step 2: 提交

```bash
git add frontend/src/components/settings/ApprovalBadge.tsx
git commit -m "feat: add ApprovalBadge component for status display"
```

---

## Task 13: 前端 ToolManager 组件

**Files:**
- Create: `frontend/src/components/settings/ToolManager.tsx`

### Step 1: 创建组件

```tsx
// frontend/src/components/settings/ToolManager.tsx
import React, { useState } from 'react'
import { UserMCPTool } from '../../types/userTools'
import { ApprovalBadge } from './ApprovalBadge'
import { MCPToolForm } from './MCPToolForm'

interface Props {
  tools: UserMCPTool[]
  onCreate: (tool: Partial<UserMCPTool>) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onTest: (id: string) => Promise<{ success: boolean; tools?: string[]; error?: string }>
  onApprove: (id: string) => Promise<void>
  onReject: (id: string) => Promise<void>
}

export const ToolManager: React.FC<Props> = ({ tools, onCreate, onDelete, onTest, onApprove, onReject }) => {
  const [showForm, setShowForm] = useState(false)
  const [testResult, setTestResult] = useState<Record<string, { success: boolean; tools?: string[]; error?: string }>>({})

  const handleTest = async (id: string) => {
    const result = await onTest(id)
    setTestResult(prev => ({ ...prev, [id]: result }))
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-700">MCP 工具</h3>
        <button
          onClick={() => setShowForm(!showForm)}
          className="text-sm text-sky-500 hover:text-sky-600 font-medium"
        >
          + 添加 MCP 工具
        </button>
      </div>

      {showForm && (
        <MCPToolForm
          onSubmit={async (tool) => { await onCreate(tool); setShowForm(false) }}
          onCancel={() => setShowForm(false)}
        />
      )}

      <div className="space-y-2">
        {tools.map(tool => (
          <div
            key={tool.id}
            className="rounded-xl border border-white/60 bg-white/70 backdrop-blur-sm p-3"
          >
            <div className="flex items-center justify-between mb-1">
              <div className="font-medium text-sm">{tool.name}</div>
              <ApprovalBadge status={tool.status} />
            </div>
            <div className="text-xs text-gray-500 mb-2">
              {tool.transport.toUpperCase()}: {tool.endpoint_url} · 工具: {tool.tool_name}
            </div>
            <div className="flex gap-2">
              <button onClick={() => handleTest(tool.id)} className="text-xs text-sky-500 hover:text-sky-600">测试</button>
              {tool.status === 'pending' && (
                <>
                  <button onClick={() => onApprove(tool.id)} className="text-xs text-green-500 hover:text-green-600">审批</button>
                  <button onClick={() => onReject(tool.id)} className="text-xs text-red-500 hover:text-red-600">拒绝</button>
                </>
              )}
              <button onClick={() => onDelete(tool.id)} className="text-xs text-red-400 hover:text-red-500">删除</button>
            </div>
            {testResult[tool.id] && (
              <div className={`mt-2 text-xs ${testResult[tool.id].success ? 'text-green-600' : 'text-red-600'}`}>
                {testResult[tool.id].success
                  ? `✅ 连接成功，发现工具: ${testResult[tool.id].tools?.join(', ') || tool.tool_name}`
                  : `❌ ${testResult[tool.id].error}`}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
```

### Step 2: 提交

```bash
git add frontend/src/components/settings/ToolManager.tsx
git commit -m "feat: add ToolManager component for MCP tool management"
```

---

## Task 14: 前端 MCPToolForm 分步表单

**Files:**
- Create: `frontend/src/components/settings/MCPToolForm.tsx`

### Step 1: 创建组件

```tsx
// frontend/src/components/settings/MCPToolForm.tsx
import React, { useState } from 'react'
import { UserMCPTool } from '../../types/userTools'

interface Props {
  onSubmit: (tool: Partial<UserMCPTool>) => Promise<void>
  onCancel: () => void
}

export const MCPToolForm: React.FC<Props> = ({ onSubmit, onCancel }) => {
  const [step, setStep] = useState(1)
  const [form, setForm] = useState({
    transport: 'sse' as 'sse' | 'http',
    endpoint_url: '',
    http_url: '',
    auth_token: '',
    name: '',
    description: '',
    tool_name: '',
    timeout_ms: 5000,
  })

  const update = (field: string, value: string | number) => setForm(prev => ({ ...prev, [field]: value }))

  const handleSubmit = async () => {
    await onSubmit({
      ...form,
      tool_name: form.tool_name || form.name,
    })
  }

  return (
    <div className="rounded-xl border border-white/60 bg-white/70 backdrop-blur-sm p-4 space-y-4">
      {/* Step indicator */}
      <div className="flex items-center gap-2 text-xs">
        <div className={`w-6 h-6 rounded-full flex items-center justify-center font-semibold ${step === 1 ? 'bg-sky-500 text-white' : 'bg-gray-200 text-gray-500'}`}>1</div>
        <span className={step === 1 ? 'text-sky-500 font-medium' : 'text-gray-400'}>连接配置</span>
        <div className="w-8 border-t border-gray-200"></div>
        <div className={`w-6 h-6 rounded-full flex items-center justify-center font-semibold ${step === 2 ? 'bg-sky-500 text-white' : 'bg-gray-200 text-gray-500'}`}>2</div>
        <span className={step === 2 ? 'text-sky-500 font-medium' : 'text-gray-400'}>基本信息</span>
      </div>

      {step === 1 && (
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">传输方式 *</label>
            <div className="flex gap-2">
              {(['sse', 'http'] as const).map(t => (
                <button
                  key={t}
                  onClick={() => update('transport', t)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium border ${form.transport === t ? 'bg-sky-50 border-sky-300 text-sky-600' : 'bg-white border-gray-200 text-gray-600'}`}
                >
                  {t.toUpperCase()}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">Endpoint URL *</label>
            <input
              value={form.endpoint_url}
              onChange={e => update('endpoint_url', e.target.value)}
              placeholder="http://mcp-server:8080/sse"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">HTTP Fallback</label>
            <input
              value={form.http_url}
              onChange={e => update('http_url', e.target.value)}
              placeholder="http://mcp-server:8080/http"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">Auth Token</label>
            <input
              type="password"
              value={form.auth_token}
              onChange={e => update('auth_token', e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">名称 *</label>
            <input
              value={form.name}
              onChange={e => update('name', e.target.value)}
              placeholder="日志查询 MCP"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">描述</label>
            <input
              value={form.description}
              onChange={e => update('description', e.target.value)}
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">工具名称 *</label>
            <input
              value={form.tool_name}
              onChange={e => update('tool_name', e.target.value)}
              placeholder="query_logs"
              className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">超时 (ms)</label>
            <input
              type="number"
              value={form.timeout_ms}
              onChange={e => update('timeout_ms', parseInt(e.target.value) || 5000)}
              className="w-32 px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300"
            />
          </div>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button onClick={onCancel} className="px-4 py-1.5 rounded-lg border border-gray-200 text-sm text-gray-600 hover:bg-gray-50">
          取消
        </button>
        {step === 1 ? (
          <button
            onClick={() => setStep(2)}
            disabled={!form.endpoint_url}
            className="px-4 py-1.5 rounded-lg bg-sky-500 text-white text-sm font-medium hover:bg-sky-600 disabled:opacity-50"
          >
            下一步 →
          </button>
        ) : (
          <button
            onClick={handleSubmit}
            disabled={!form.name}
            className="px-4 py-1.5 rounded-lg bg-amber-500 text-white text-sm font-medium hover:bg-amber-600 disabled:opacity-50"
          >
            提交审批
          </button>
        )}
      </div>
    </div>
  )
}
```

### Step 2: 提交

```bash
git add frontend/src/components/settings/MCPToolForm.tsx
git commit -m "feat: add MCPToolForm step wizard component"
```

---

## Task 15: 前端 SkillManager + UserSkillForm

**Files:**
- Create: `frontend/src/components/settings/SkillManager.tsx`
- Create: `frontend/src/components/settings/UserSkillForm.tsx`
- Create: `frontend/src/components/settings/SettingsView.tsx`

### Step 1: 创建 UserSkillForm

```tsx
// frontend/src/components/settings/UserSkillForm.tsx
import React, { useState } from 'react'
import { UserSkill, UserMCPTool } from '../../types/userTools'

interface Props {
  tools: UserMCPTool[]
  onSubmit: (skill: Partial<UserSkill>) => Promise<void>
  onCancel: () => void
}

export const UserSkillForm: React.FC<Props> = ({ tools, onSubmit, onCancel }) => {
  const [step, setStep] = useState(1)
  const [keywordInput, setKeywordInput] = useState('')
  const [form, setForm] = useState({
    name: '',
    description: '',
    domain: 'custom' as string,
    tool_ref_id: '',
    keywords: [] as string[],
    focus: '',
    output_parser: 'json_array' as string,
    json_path: '',
    tier: 1,
  })

  const update = (field: string, value: unknown) => setForm(prev => ({ ...prev, [field]: value }))

  const addKeyword = () => {
    const kw = keywordInput.trim()
    if (kw && !form.keywords.includes(kw)) {
      update('keywords', [...form.keywords, kw])
      setKeywordInput('')
    }
  }

  const removeKeyword = (kw: string) => {
    update('keywords', form.keywords.filter(k => k !== kw))
  }

  const approvedTools = tools.filter(t => t.status === 'approved')

  return (
    <div className="rounded-xl border border-white/60 bg-white/70 backdrop-blur-sm p-4 space-y-4">
      <div className="flex items-center gap-2 text-xs">
        <div className={`w-6 h-6 rounded-full flex items-center justify-center font-semibold ${step === 1 ? 'bg-sky-500 text-white' : 'bg-gray-200 text-gray-500'}`}>1</div>
        <span className={step === 1 ? 'text-sky-500 font-medium' : 'text-gray-400'}>基本信息</span>
        <div className="w-8 border-t border-gray-200"></div>
        <div className={`w-6 h-6 rounded-full flex items-center justify-center font-semibold ${step === 2 ? 'bg-sky-500 text-white' : 'bg-gray-200 text-gray-500'}`}>2</div>
        <span className={step === 2 ? 'text-sky-500 font-medium' : 'text-gray-400'}>匹配 & 解析</span>
      </div>

      {step === 1 && (
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">名称 *</label>
            <input value={form.name} onChange={e => update('name', e.target.value)} placeholder="user_custom_query" className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300" />
            <div className="text-xs text-gray-400 mt-0.5">全局唯一，建议 user_ 前缀</div>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">描述</label>
            <input value={form.description} onChange={e => update('description', e.target.value)} className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300" />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">域 *</label>
            <div className="flex gap-2">
              {['metrics', 'logs', 'knowledge', 'custom'].map(d => (
                <button key={d} onClick={() => update('domain', d)} className={`px-3 py-1.5 rounded-lg text-xs font-medium border ${form.domain === d ? 'bg-sky-50 border-sky-300 text-sky-600' : 'bg-white border-gray-200 text-gray-600'}`}>{d}</button>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">关联工具 *</label>
            <select value={form.tool_ref_id} onChange={e => update('tool_ref_id', e.target.value)} className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300">
              <option value="">-- 选择已审批的工具 --</option>
              {approvedTools.map(t => <option key={t.id} value={t.id}>{t.name} / {t.tool_name}</option>)}
            </select>
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">关键词 * <span className="text-gray-400 font-normal">(任意一个命中即匹配)</span></label>
            <div className="flex flex-wrap gap-1.5 p-2 rounded-lg border border-gray-200 bg-white/70 min-h-[36px]">
              {form.keywords.map(kw => (
                <span key={kw} className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-sky-50 border border-sky-200 text-sky-600 text-xs">
                  {kw}
                  <button onClick={() => removeKeyword(kw)} className="text-sky-400 hover:text-sky-600">×</button>
                </span>
              ))}
              <input
                value={keywordInput}
                onChange={e => setKeywordInput(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addKeyword() } }}
                placeholder="输入关键词后回车"
                className="flex-1 min-w-[100px] text-sm bg-transparent focus:outline-none"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">Focus 提示</label>
            <input value={form.focus} onChange={e => update('focus', e.target.value)} placeholder="Focus on ..." className="w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300" />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">输出解析 *</label>
            <div className="flex flex-wrap gap-2">
              {['json_array', 'json_nested', 'log_lines', 'raw'].map(p => (
                <button key={p} onClick={() => update('output_parser', p)} className={`px-3 py-1.5 rounded-lg text-xs font-medium border ${form.output_parser === p ? 'bg-sky-50 border-sky-300 text-sky-600' : 'bg-white border-gray-200 text-gray-600'}`}>{p}</button>
              ))}
            </div>
            {form.output_parser === 'json_nested' && (
              <input value={form.json_path} onChange={e => update('json_path', e.target.value)} placeholder="data.items" className="mt-2 w-full px-3 py-2 rounded-lg border border-gray-200 text-sm bg-white/70 focus:outline-none focus:border-sky-300" />
            )}
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">门控层级</label>
            <div className="flex gap-2">
              <button onClick={() => update('tier', 1)} className={`px-3 py-1.5 rounded-lg text-xs font-medium border ${form.tier === 1 ? 'bg-sky-50 border-sky-300 text-sky-600' : 'bg-white border-gray-200 text-gray-600'}`}>SkillGate</button>
              <button onClick={() => update('tier', 2)} className={`px-3 py-1.5 rounded-lg text-xs font-medium border ${form.tier === 2 ? 'bg-sky-50 border-sky-300 text-sky-600' : 'bg-white border-gray-200 text-gray-600'}`}>OnDemand</button>
            </div>
          </div>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button onClick={onCancel} className="px-4 py-1.5 rounded-lg border border-gray-200 text-sm text-gray-600 hover:bg-gray-50">取消</button>
        {step === 1 ? (
          <button onClick={() => setStep(2)} disabled={!form.name || !form.tool_ref_id} className="px-4 py-1.5 rounded-lg bg-sky-500 text-white text-sm font-medium hover:bg-sky-600 disabled:opacity-50">下一步 →</button>
        ) : (
          <button onClick={() => onSubmit(form)} disabled={form.keywords.length === 0} className="px-4 py-1.5 rounded-lg bg-amber-500 text-white text-sm font-medium hover:bg-amber-600 disabled:opacity-50">提交审批</button>
        )}
      </div>
    </div>
  )
}
```

### Step 2: 创建 SkillManager

```tsx
// frontend/src/components/settings/SkillManager.tsx
import React, { useState } from 'react'
import { UserSkill, UserMCPTool } from '../../types/userTools'
import { ApprovalBadge } from './ApprovalBadge'
import { UserSkillForm } from './UserSkillForm'

interface Props {
  skills: UserSkill[]
  tools: UserMCPTool[]
  onCreate: (skill: Partial<UserSkill>) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onApprove: (id: string) => Promise<void>
  onReject: (id: string) => Promise<void>
}

export const SkillManager: React.FC<Props> = ({ skills, tools, onCreate, onDelete, onApprove, onReject }) => {
  const [showForm, setShowForm] = useState(false)

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-700">Skill</h3>
        <button onClick={() => setShowForm(!showForm)} className="text-sm text-sky-500 hover:text-sky-600 font-medium">+ 添加 Skill</button>
      </div>

      {showForm && (
        <UserSkillForm
          tools={tools}
          onSubmit={async (skill) => { await onCreate(skill); setShowForm(false) }}
          onCancel={() => setShowForm(false)}
        />
      )}

      <div className="space-y-2">
        {skills.map(skill => (
          <div key={skill.id} className="rounded-xl border border-white/60 bg-white/70 backdrop-blur-sm p-3">
            <div className="flex items-center justify-between mb-1">
              <div className="font-medium text-sm">🧩 {skill.name}</div>
              <ApprovalBadge status={skill.status} />
            </div>
            <div className="text-xs text-gray-500 mb-2">
              {skill.domain} · 关键词: {skill.keywords.join(', ')} · 解析: {skill.output_parser}
            </div>
            <div className="flex gap-2">
              {skill.status === 'pending' && (
                <>
                  <button onClick={() => onApprove(skill.id)} className="text-xs text-green-500 hover:text-green-600">审批</button>
                  <button onClick={() => onReject(skill.id)} className="text-xs text-red-500 hover:text-red-600">拒绝</button>
                </>
              )}
              <button onClick={() => onDelete(skill.id)} className="text-xs text-red-400 hover:text-red-500">删除</button>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
```

### Step 3: 创建 SettingsView

```tsx
// frontend/src/components/settings/SettingsView.tsx
import React, { useState } from 'react'
import { useUserTools } from '../../hooks/useUserTools'
import { ToolManager } from './ToolManager'
import { SkillManager } from './SkillManager'

interface Props {
  onBack: () => void
}

export const SettingsView: React.FC<Props> = ({ onBack }) => {
  const [tab, setTab] = useState<'tools' | 'skills'>('tools')
  const {
    tools, skills, loading, error,
    createTool, deleteTool, testTool, approveTool, rejectTool,
    createSkill, deleteSkill, approveSkill, rejectSkill,
  } = useUserTools()

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-3 px-4 py-3 border-b border-white/30">
        <button onClick={onBack} className="text-gray-500 hover:text-gray-700">← 返回</button>
        <h2 className="text-lg font-semibold">工具 & Skill 管理</h2>
      </div>

      <div className="flex border-b border-white/30">
        <button
          onClick={() => setTab('tools')}
          className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'tools' ? 'border-sky-500 text-sky-600' : 'border-transparent text-gray-500'}`}
        >
          MCP 工具
        </button>
        <button
          onClick={() => setTab('skills')}
          className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'skills' ? 'border-sky-500 text-sky-600' : 'border-transparent text-gray-500'}`}
        >
          Skill
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {loading && <div className="text-sm text-gray-500">加载中...</div>}
        {error && <div className="text-sm text-red-500">{error}</div>}
        {tab === 'tools' && (
          <ToolManager
            tools={tools}
            onCreate={createTool}
            onDelete={deleteTool}
            onTest={testTool}
            onApprove={approveTool}
            onReject={rejectTool}
          />
        )}
        {tab === 'skills' && (
          <SkillManager
            skills={skills}
            tools={tools}
            onCreate={createSkill}
            onDelete={deleteSkill}
            onApprove={approveSkill}
            onReject={rejectSkill}
          />
        )}
      </div>
    </div>
  )
}
```

### Step 4: 提交

```bash
git add frontend/src/components/settings/
git commit -m "feat: add SkillManager, UserSkillForm, and SettingsView components"
```

---

## Task 16: 前端集成（App.tsx, Sidebar, SkillPanel）

**Files:**
- Modify: `frontend/src/types/chat.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/sidebar/Sidebar.tsx`
- Modify: `frontend/src/components/sidebar/SkillPanel.tsx`
- Modify: `frontend/src/lib/utils.ts`

### Step 1: 修改 WorkbenchMode 类型

在 `frontend/src/types/chat.ts` 中：

```typescript
// 将
export type WorkbenchMode = 'chat' | 'aiops'
// 改为
export type WorkbenchMode = 'chat' | 'aiops' | 'settings'
```

### Step 2: 修改 App.tsx

在视图切换处添加 settings 分支：

```tsx
// 在现有 workbenchMode === 'aiops' 三元表达式处改为：
{workbenchMode === 'settings' ? (
    <SettingsView onBack={() => setWorkbenchMode('chat')} />
) : workbenchMode === 'aiops' ? (
    <IncidentView ... />
) : (
    <AgentWorkbenchView ... />
)}
```

添加 import：
```tsx
import { SettingsView } from './components/settings/SettingsView'
```

### Step 3: 修改 Sidebar.tsx

在 Sidebar 底部添加管理入口：

```tsx
// 在 Sidebar 组件的 JSX 末尾、关闭 div 之前添加
<div className="mt-auto pt-3 border-t border-white/20">
  <button
    onClick={() => onWorkbenchModeChange?.('settings')}
    className="flex items-center gap-2 w-full px-3 py-2 rounded-lg text-sm text-gray-500 hover:text-gray-700 hover:bg-white/30 transition-colors"
  >
    ⚙️ 工具 & Skill 管理
  </button>
</div>
```

确保 `onWorkbenchModeChange` 在 Sidebar 的 Props 中定义。

### Step 4: 修改 SkillPanel.tsx

在每个域的 skill 列表末尾追加该域的用户 skill：

```tsx
// 在 SkillPanel 中，遍历 skills 时追加用户 skill
// 用户 skill 用虚线边框 + 🧩 图标区分
{userSkills
  .filter(s => s.domain === activeDomain && s.status === 'approved')
  .map(skill => (
    <div
      key={skill.id}
      className="flex items-center justify-between px-3 py-2 rounded-lg border border-dashed border-sky-200 bg-sky-50/30"
    >
      <span className="text-sm flex items-center gap-1.5">
        🧩 {skill.name}
      </span>
      <button
        onClick={() => toggleSkill(skill.name)}
        className={`... toggle styles ...`}
      >
        {/* toggle switch */}
      </button>
    </div>
  ))
}
```

需要从 `useUserTools` hook 获取 approved 的用户 skill 列表。

### Step 5: 提交

```bash
git add frontend/src/types/chat.ts frontend/src/App.tsx frontend/src/components/sidebar/Sidebar.tsx frontend/src/components/sidebar/SkillPanel.tsx frontend/src/lib/utils.ts
git commit -m "feat: integrate user tools/skills into sidebar and skill panel"
```

---

## 自检清单

- [ ] Spec 中的每个 API 端点都有对应的 controller 方法
- [ ] GenericSkill 实现了 Skill 和 FocusProvider 两个接口
- [ ] DynamicMCPRegistry 的 Invoke 返回 degraded JSON 而非 error
- [ ] 网络白名单检查在 Register 时执行
- [ ] 审批后自动调用 Reload 热更新
- [ ] 前端表单的必填字段校验与后端 v: 标签一致
- [ ] SkillPanel 中用户 skill 用 🧩 + 虚线边框区分
- [ ] SettingsView 的 workbenchMode 为 'settings'
- [ ] 所有新文件都有对应的测试（后端）
