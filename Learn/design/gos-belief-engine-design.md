# GoS Belief Engine 设计方案

> 分支：`feature/gos-belief-engine`
> 状态：可推进 POC
> 日期：2026-05-13

---

## 1. 概述

### 1.1 目标

引入 GoS（Graph of States）的因果图 + 状态机理念，增强 OpsCaption AIOps 推理能力。

### 1.2 推进策略

```
Phase 1: belief 包 + 单元测试（1-2 天）
Phase 2: gos_engine 最小实现（2-3 天）
Phase 3: 评测（1-2 天）→ 6 个 gate 通过才能进入灰度
Phase 4: 灰度接入（1-2 天）
Phase 5: 优化（持续）
```

默认关闭：`aiops.gos.enabled=false`，`aiops.engine=plan_execute_replan`

---

## 2. 数据结构

### 2.1 types.go

```go
package belief

import "time"

type NodeType string
const (
    NodeSignal     NodeType = "Signal"
    NodeEvidence   NodeType = "Evidence"
    NodeHypothesis NodeType = "Hypothesis"
)

type EdgeType string
const (
    EdgeSupport EdgeType = "support"
    EdgeRefute  EdgeType = "refute"
    EdgeRefines EdgeType = "refines"
    EdgeCausal  EdgeType = "causal"
)

type NodeStatus string
const (
    StatusActive     NodeStatus = "active"
    StatusRetracted  NodeStatus = "retracted"
    StatusSuperseded NodeStatus = "superseded"
)

type EvidenceSource struct {
    SourceType     string    `json:"source_type"`
    SourceID       string    `json:"source_id"`
    ToolName       string    `json:"tool_name,omitempty"`
    RetrievalQuery string    `json:"retrieval_query,omitempty"`
    Timestamp      time.Time `json:"timestamp"`
    SummarySnippet string    `json:"summary_snippet"`
    ArtifactRef    string    `json:"artifact_ref,omitempty"`
}

type Node struct {
    ID           string                 `json:"id"`
    Type         NodeType               `json:"type"`
    Label        string                 `json:"label"`
    Score        float64                `json:"score"`
    Status       NodeStatus             `json:"status"`
    Level        int                    `json:"level"`
    Attrs        map[string]interface{} `json:"attrs"`
    Source       *EvidenceSource        `json:"source,omitempty"`
    RetractedBy  string                 `json:"retracted_by,omitempty"`
    SupersededBy string                 `json:"superseded_by,omitempty"`
    RetractedAt  *time.Time             `json:"retracted_at,omitempty"`
    StepID       string                 `json:"step_id"`
}

type Edge struct {
    Src              string     `json:"src"`
    Dst              string     `json:"dst"`
    Type             EdgeType   `json:"type"`
    Status           NodeStatus `json:"status"`
    Confidence       float64    `json:"confidence"`
    DerivationType   string     `json:"derivation_type"`
    ExtractorVersion string     `json:"extractor_version"`
    StepID           string     `json:"step_id"`
    RetractedBy      string     `json:"retracted_by,omitempty"`
    RetractedAt      *time.Time `json:"retracted_at,omitempty"`
}

type Frontier struct {
    NodeID   string  `json:"node_id"`
    Label    string  `json:"label"`
    Why      string  `json:"why"`
    Score    float64 `json:"score"`
    Level    int     `json:"level"`
    Supports int     `json:"supports"`
    Refutes  int     `json:"refutes"`
}
```

### 2.2 graph.go（append-only，带锁）

```go
package belief

import (
    "fmt"
    "sort"
    "sync"
    "time"
)

type GraphSnapshot struct {
    StepID    string                 `json:"step_id"`
    Timestamp time.Time              `json:"timestamp"`
    Action    string                 `json:"action"`
    Nodes     map[string]*Node       `json:"nodes"`
    Edges     map[string]*Edge       `json:"edges"`
}

type GraphUpdateResult struct {
    Committed bool  `json:"committed"`
    Error     error `json:"error,omitempty"`
}

type BeliefGraph struct {
    mu            sync.RWMutex
    Nodes         map[string]*Node    `json:"nodes"`
    Edges         map[string]*Edge    `json:"edges"`
    StartSignalID string              `json:"start_signal_id"`
    Belief        string              `json:"belief"`
    LevelNodes    map[int][]LevelNode `json:"level_nodes"`
    Snapshots     []GraphSnapshot     `json:"snapshots"`
    CurrentStep   int                 `json:"current_step"`
}

type LevelNode struct {
    NodeID     string  `json:"node_id"`
    Confidence float64 `json:"confidence"`
}

func edgeKey(src, dst string) string { return src + "->" + dst }

func safeWhy(attrs map[string]interface{}) string {
    if attrs == nil { return "" }
    v, ok := attrs["why"]
    if !ok { return "" }
    s, ok := v.(string)
    if !ok { return fmt.Sprintf("%v", v) }
    return s
}

// --- 带锁的公开方法 ---

func (g *BeliefGraph) AddNode(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
    g.mu.Lock()
    defer g.mu.Unlock()
    return g.addNodeInternal(nodeType, label, score, level, attrs, source)
}

func (g *BeliefGraph) AddSignal(label string) string {
    return g.AddNode(NodeSignal, label, 1.0, 0, nil, nil)
}

func (g *BeliefGraph) AddEvidence(label string, source *EvidenceSource) string {
    return g.AddNode(NodeEvidence, label, 1.0, 0, nil, source)
}

func (g *BeliefGraph) AddHypothesis(label string, score float64, level int, why string) string {
    return g.AddNode(NodeHypothesis, label, score, level, map[string]interface{}{"why": why}, nil)
}

func (g *BeliefGraph) UpdateNode(nodeID string, newScore float64, newWhy string) {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.updateNodeInternal(nodeID, newScore, newWhy)
}

func (g *BeliefGraph) AddEdge(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.addEdgeInternal(src, dst, edgeType, confidence, derivationType)
}

func (g *BeliefGraph) ExtractFrontier(level int) *Frontier {
    g.mu.RLock()
    defer g.mu.RUnlock()
    return g.extractFrontierInternal(level)
}

// --- 内部方法（不持锁，调用者需持锁） ---

func (g *BeliefGraph) addNodeInternal(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
    g.CurrentStep++
    nodeID := fmt.Sprintf("node-%d", g.CurrentStep)
    if attrs == nil { attrs = make(map[string]interface{}) }
    g.Nodes[nodeID] = &Node{
        ID: nodeID, Type: nodeType, Label: label, Score: score,
        Status: StatusActive, Level: level, Attrs: attrs, Source: source,
        StepID: fmt.Sprintf("step-%d", g.CurrentStep),
    }
    g.updateLevelNodesInternal()
    g.takeSnapshotInternal("add_node")
    return nodeID
}

func (g *BeliefGraph) updateNodeInternal(nodeID string, newScore float64, newWhy string) {
    node, exists := g.Nodes[nodeID]
    if !exists { return }
    g.CurrentStep++
    node.Score = newScore
    if node.Attrs == nil { node.Attrs = make(map[string]interface{}) }
    node.Attrs["why"] = newWhy
    node.Attrs["updated_at"] = time.Now()
    g.updateLevelNodesInternal()
    g.takeSnapshotInternal("update_node")
}

func (g *BeliefGraph) addEdgeInternal(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
    if _, ok := g.Nodes[src]; !ok { return }
    if _, ok := g.Nodes[dst]; !ok { return }
    g.CurrentStep++
    g.Edges[edgeKey(src, dst)] = &Edge{
        Src: src, Dst: dst, Type: edgeType, Status: StatusActive,
        Confidence: confidence, DerivationType: derivationType,
        ExtractorVersion: "v1.0", StepID: fmt.Sprintf("step-%d", g.CurrentStep),
    }
    g.takeSnapshotInternal("add_edge")
}

func (g *BeliefGraph) extractFrontierInternal(level int) *Frontier {
    var candidates []*Node
    for _, n := range g.Nodes {
        if n.Type == NodeHypothesis && n.Status == StatusActive && n.Level == level {
            candidates = append(candidates, n)
        }
    }
    if len(candidates) == 0 { return nil }
    sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
    top := candidates[0]
    sup, ref := 0, 0
    for _, e := range g.Edges {
        if e.Dst == top.ID && e.Status == StatusActive {
            if e.Type == EdgeSupport { sup++ }
            if e.Type == EdgeRefute { ref++ }
        }
    }
    return &Frontier{
        NodeID: top.ID, Label: top.Label, Why: safeWhy(top.Attrs),
        Score: top.Score, Level: top.Level, Supports: sup, Refutes: ref,
    }
}

func (g *BeliefGraph) updateLevelNodesInternal() {
    g.LevelNodes = make(map[int][]LevelNode)
    for _, n := range g.Nodes {
        if n.Type == NodeHypothesis && n.Status == StatusActive {
            g.LevelNodes[n.Level] = append(g.LevelNodes[n.Level], LevelNode{NodeID: n.ID, Confidence: n.Score})
        }
    }
    for lv := range g.LevelNodes {
        sort.Slice(g.LevelNodes[lv], func(i, j int) bool { return g.LevelNodes[lv][i].Confidence > g.LevelNodes[lv][j].Confidence })
    }
}

func (g *BeliefGraph) takeSnapshotInternal(action string) {
    snap := GraphSnapshot{StepID: fmt.Sprintf("step-%d", g.CurrentStep), Timestamp: time.Now(), Action: action,
        Nodes: make(map[string]*Node, len(g.Nodes)), Edges: make(map[string]*Edge, len(g.Edges))}
    for k, v := range g.Nodes {
        c := *v
        if v.Attrs != nil { c.Attrs = make(map[string]interface{}, len(v.Attrs)); for ak, av := range v.Attrs { c.Attrs[ak] = av } }
        if v.Source != nil { sc := *v.Source; c.Source = &sc }
        snap.Nodes[k] = &c
    }
    for k, v := range g.Edges { c := *v; snap.Edges[k] = &c }
    g.Snapshots = append(g.Snapshots, snap)
}

// --- copy-on-write ---

func (g *BeliefGraph) UpdateCopyOnWrite(fn func(copy *BeliefGraph) error) *GraphUpdateResult {
    g.mu.RLock()
    cp := g.deepCopy()
    g.mu.RUnlock()

    if err := fn(cp); err != nil {
        return &GraphUpdateResult{Committed: false, Error: err}
    }

    g.mu.Lock()
    g.Nodes = cp.Nodes
    g.Edges = cp.Edges
    g.CurrentStep = cp.CurrentStep
    g.updateLevelNodesInternal()
    g.takeSnapshotInternal("copy_on_write_update")
    g.mu.Unlock()

    return &GraphUpdateResult{Committed: true}
}

func (g *BeliefGraph) deepCopy() *BeliefGraph {
    cp := &BeliefGraph{
        Nodes: make(map[string]*Node, len(g.Nodes)), Edges: make(map[string]*Edge, len(g.Edges)),
        StartSignalID: g.StartSignalID, Belief: g.Belief, CurrentStep: g.CurrentStep,
    }
    for k, v := range g.Nodes {
        n := *v
        if v.Attrs != nil { n.Attrs = make(map[string]interface{}, len(v.Attrs)); for ak, av := range v.Attrs { n.Attrs[ak] = av } }
        if v.Source != nil { sc := *v.Source; n.Source = &sc }
        cp.Nodes[k] = &n
    }
    for k, v := range g.Edges { e := *v; cp.Edges[k] = &e }
    return cp
}

// --- 副本上的无锁方法 ---

func (g *BeliefGraph) AddNodeCopy(nodeType NodeType, label string, score float64, level int, attrs map[string]interface{}, source *EvidenceSource) string {
    return g.addNodeInternal(nodeType, label, score, level, attrs, source)
}

func (g *BeliefGraph) AddEvidenceCopy(label string, source *EvidenceSource) string {
    return g.addNodeInternal(NodeEvidence, label, 1.0, 0, nil, source)
}

func (g *BeliefGraph) AddHypothesisCopy(label string, score float64, level int, why string) string {
    return g.addNodeInternal(NodeHypothesis, label, score, level, map[string]interface{}{"why": why}, nil)
}

func (g *BeliefGraph) UpdateNodeCopy(nodeID string, newScore float64, newWhy string) {
    g.updateNodeInternal(nodeID, newScore, newWhy)
}

func (g *BeliefGraph) AddEdgeCopy(src, dst string, edgeType EdgeType, confidence float64, derivationType string) {
    g.addEdgeInternal(src, dst, edgeType, confidence, derivationType)
}
```

### 2.3 fsm.go（强类型，决策与状态分离）

```go
package belief

import "fmt"

type FSMState int
const (
    StateDrilling  FSMState = iota
    StateReporting
    StateDone
)

type FSMDecision struct {
    Action string // "continue", "report", "done"
    Reason string
}

type FSMTransition struct {
    From   FSMState `json:"from"`
    To     FSMState `json:"to"`
    Reason string   `json:"reason"`
    StepID string   `json:"step_id"`
}

type FSMThresholds struct {
    GapDelta   float64 `yaml:"gap_delta"`
    MinSupport int     `yaml:"min_support"`
    MaxSteps   int     `yaml:"max_steps"`
}

type BeliefFSM struct {
    State        FSMState        `json:"state"`
    CurrentLevel int             `json:"current_level"`
    LevelSteps   map[int]int     `json:"level_steps"`
    Thresholds   FSMThresholds   `json:"thresholds"`
    History      []FSMTransition `json:"history"`
    TotalSteps   int             `json:"total_steps"`
}

func NewBeliefFSM(thresholds FSMThresholds) *BeliefFSM {
    return &BeliefFSM{State: StateDrilling, CurrentLevel: 1, LevelSteps: map[int]int{1: 0}, Thresholds: thresholds}
}

func (f *BeliefFSM) GetState() FSMState    { return f.State }
func (f *BeliefFSM) GetCurrentLevel() int  { return f.CurrentLevel }
func (f *BeliefFSM) IsFinalState() bool    { return f.State == StateDone }

func (f *BeliefFSM) TickStep(k int) {
    f.TotalSteps += k
    f.LevelSteps[f.CurrentLevel] += k
}

func (f *BeliefFSM) Decide(g *BeliefGraph) *FSMDecision {
    if f.State == StateDone { return &FSMDecision{Action: "done", Reason: "already done"} }
    cands := f.topHypos(g, f.CurrentLevel)
    if len(cands) == 0 { return &FSMDecision{Action: "report", Reason: "no candidates"} }
    gap := f.gap(cands)
    sup := f.countSupport(g, cands[0].NodeID)
    steps := f.LevelSteps[f.CurrentLevel]
    if gap >= f.Thresholds.GapDelta && sup >= f.Thresholds.MinSupport {
        return &FSMDecision{Action: "report", Reason: fmt.Sprintf("gap=%.2f sup=%d", gap, sup)}
    }
    if steps >= f.Thresholds.MaxSteps {
        return &FSMDecision{Action: "report", Reason: fmt.Sprintf("max steps %d", steps)}
    }
    return &FSMDecision{Action: "continue", Reason: "conditions not met"}
}

func (f *BeliefFSM) TransitionTo(s FSMState, reason string) {
    f.History = append(f.History, FSMTransition{From: f.State, To: s, Reason: reason, StepID: fmt.Sprintf("step-%d", f.TotalSteps)})
    f.State = s
}

func (f *BeliefFSM) DrillDown(reason string) {
    f.CurrentLevel++
    f.LevelSteps[f.CurrentLevel] = 0
    f.TransitionTo(StateDrilling, reason)
}

func (f *BeliefFSM) MarkDone(reason string) { f.TransitionTo(StateDone, reason) }

func (f *BeliefFSM) topHypos(g *BeliefGraph, level int) []LevelNode {
    g.mu.RLock()
    defer g.mu.RUnlock()
    var c []LevelNode
    for _, n := range g.Nodes {
        if n.Type == NodeHypothesis && n.Status == StatusActive && n.Level == level {
            c = append(c, LevelNode{NodeID: n.ID, Confidence: n.Score})
        }
    }
    sort.Slice(c, func(i, j int) bool { return c[i].Confidence > c[j].Confidence })
    return c
}

func (f *BeliefFSM) gap(c []LevelNode) float64 {
    if len(c) < 2 { return c[0].Confidence }
    return c[0].Confidence - c[1].Confidence
}

func (f *BeliefFSM) countSupport(g *BeliefGraph, id string) int {
    g.mu.RLock()
    defer g.mu.RUnlock()
    n := 0
    for _, e := range g.Edges {
        if e.Dst == id && e.Type == EdgeSupport && e.Status == StatusActive { n++ }
    }
    return n
}
```

---

## 3. 专家 Agent

### 3.1 registry.go

```go
package experts

import (
    "github.com/cloudwego/eino/components/tool"
    "SuperBizAgent/internal/ai/tools"
)

type ExpertAnalysis struct {
    ExpertName        string            `json:"expert_name"`
    Analysis          string            `json:"analysis"`
    Evidence          []EvidenceItem    `json:"evidence"`
    Confidence        float64           `json:"confidence"`
    Status            string            `json:"status"` // "succeeded", "degraded", "failed"
    DegradationReason string            `json:"degradation_reason,omitempty"`
    ToolErrors        []ToolError       `json:"tool_errors,omitempty"`
}

type ToolError struct {
    ToolName string `json:"tool_name"`
    Action   string `json:"action"`
    Error    string `json:"error"`
}

type EvidenceItem struct {
    SourceType string  `json:"source_type"`
    SourceID   string  `json:"source_id"`
    Title      string  `json:"title"`
    Snippet    string  `json:"snippet"`
    Score      float64 `json:"score"`
}

type ExpertAgent interface {
    Name() string
    Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *ExpertAnalysis
}

type ToolRegistry struct {
    tools map[string]tool.InvokableTool
}

func NewToolRegistry() *ToolRegistry { return &ToolRegistry{tools: make(map[string]tool.InvokableTool)} }

func (r *ToolRegistry) Register(name string, t tool.InvokableTool) { r.tools[name] = t }

func (r *ToolRegistry) Get(name string) (tool.InvokableTool, bool) {
    t, ok := r.tools[name]
    return t, ok
}

func (r *ToolRegistry) InitFromExisting() error {
    r.Register("query_internal_docs", tools.NewQueryInternalDocsTool())
    r.Register("get_current_time", tools.NewGetCurrentTimeTool())
    r.registerLogTool()
    return nil
}

func (r *ToolRegistry) registerLogTool() {
    logTools, err := tools.GetLogMcpTool()
    if err != nil {
        r.Register("query_logs", tools.NewUnavailableLogQueryTool(err.Error()))
        return
    }
    registered := false
    for _, t := range logTools {
        if it, ok := t.(tool.InvokableTool); ok {
            r.Register("query_logs", it)
            registered = true
            break
        }
    }
    if !registered {
        r.Register("query_logs", tools.NewUnavailableLogQueryTool("no invokable log tool found"))
    }
}
```

### 3.2 tool_adapter.go

```go
package experts

import (
    "context"
    einoschema "github.com/cloudwego/eino/schema"
    "github.com/cloudwego/eino/components/tool"
)

type ToolAdapter struct {
    tool      tool.InvokableTool
    toolInfo  *einoschema.ToolInfo
    name      string
    argBuilder ArgBuilder
}

func NewToolAdapter(name string, t tool.InvokableTool) (*ToolAdapter, error) {
    info, err := t.Info(context.Background())
    if err != nil { return nil, err }
    return &ToolAdapter{tool: t, toolInfo: info, name: name, argBuilder: getArgBuilder(name)}, nil
}

func (a *ToolAdapter) Run(ctx context.Context, naturalLanguageArgs string) (string, error) {
    jsonArgs, err := a.argBuilder.Build(naturalLanguageArgs)
    if err != nil { return "", err }
    return a.tool.InvokableRun(ctx, jsonArgs)
}
```

### 3.3 arg_builders.go

```go
package experts

import "encoding/json"

type ArgBuilder interface {
    Build(args string) (string, error)
}

type QueryArgBuilder struct{}

func (b *QueryArgBuilder) Build(args string) (string, error) {
    bytes, err := json.Marshal(map[string]string{"query": args})
    return string(bytes), err
}

type RawArgBuilder struct{}

func (b *RawArgBuilder) Build(args string) (string, error) { return args, nil }

func getArgBuilder(toolName string) ArgBuilder {
    switch toolName {
    case "query_internal_docs", "query_logs":
        return &QueryArgBuilder{}
    case "get_current_time":
        return &RawArgBuilder{}
    default:
        return &QueryArgBuilder{}
    }
}
```

### 3.4 linux_sre.go

```go
package experts

import (
    "context"
    "fmt"
    "SuperBizAgent/internal/ai/belief"
    "SuperBizAgent/internal/ai/rag"
)

type ExpertRuntimeConfig struct {
    Name, Description   string
    ToolNames           []string
    MaxRetrievalSteps   int
    ModelPath           string
    Temperature         float64
    MaxTokens           int
}

type LinuxSREAgent struct {
    name      string
    cfg       ExpertRuntimeConfig
    adapters  map[string]*ToolAdapter
    ragPool   *rag.RetrieverPool
    llmClient LLMClient
}

func NewLinuxSREAgent(cfg ExpertRuntimeConfig, toolReg *ToolRegistry, ragPool *rag.RetrieverPool, llm LLMClient) (*LinuxSREAgent, error) {
    adapters := make(map[string]*ToolAdapter)
    for _, tn := range cfg.ToolNames {
        if t, ok := toolReg.Get(tn); ok {
            if a, err := NewToolAdapter(tn, t); err == nil {
                adapters[tn] = a
            }
        }
    }
    return &LinuxSREAgent{name: cfg.Name, cfg: cfg, adapters: adapters, ragPool: ragPool, llmClient: llm}, nil
}

func (a *LinuxSREAgent) Name() string { return a.name }

func (a *LinuxSREAgent) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *ExpertAnalysis {
    result := &ExpertAnalysis{ExpertName: a.name, Status: "succeeded", Evidence: []EvidenceItem{}, ToolErrors: []ToolError{}}
    history := []RetrievalRecord{}

    for step := 0; step < a.cfg.MaxRetrievalSteps; step++ {
        decision, err := a.makeDecision(ctx, frontier, graph, history)
        if err != nil {
            result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: "llm", Action: "decision", Error: err.Error()})
            result.Status = "degraded"
            result.DegradationReason = fmt.Sprintf("decision_failed step %d", step)
            continue
        }

        content, err := a.generateContent(ctx, frontier, graph, history, decision)
        if err != nil {
            result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: "llm", Action: "content", Error: err.Error()})
            result.Status = "degraded"
            result.DegradationReason = fmt.Sprintf("content_failed step %d", step)
            continue
        }

        switch decision.Action {
        case "tool_call":
            adapter, ok := a.adapters[decision.ToolName]
            if !ok {
                result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: decision.ToolName, Action: "execute", Error: "not found"})
                result.Status = "degraded"
                continue
            }
            output, err := adapter.Run(ctx, content)
            if err != nil {
                result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: decision.ToolName, Action: "execute", Error: err.Error()})
                result.Status = "degraded"
                continue
            }
            history = append(history, RetrievalRecord{Query: content, Output: output, Tool: decision.ToolName})
            result.Evidence = append(result.Evidence, EvidenceItem{SourceType: "tool", SourceID: fmt.Sprintf("%s-%d", decision.ToolName, step), Title: decision.ToolName + " output", Snippet: truncateAndSanitize(output, 500), Score: 1.0})

        case "retrieve":
            docs, _, err := rag.Query(ctx, a.ragPool, content)
            if err != nil {
                result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: "rag", Action: "retrieve", Error: err.Error()})
                result.Status = "degraded"
                continue
            }
            var combined string
            for _, d := range docs { combined += d.Content + "\n" }
            history = append(history, RetrievalRecord{Query: content, Output: combined, Tool: "rag"})
            result.Evidence = append(result.Evidence, EvidenceItem{SourceType: "rag", SourceID: fmt.Sprintf("rag-%d", step), Title: "RAG", Snippet: truncateAndSanitize(combined, 500), Score: 1.0})

        case "analyze":
            result.Analysis = content
            result.Confidence = decision.Confidence
            return result
        }
    }

    if result.Analysis == "" {
        result.Analysis = "信息不足"
        result.Confidence = 0
        if result.Status == "succeeded" { result.Status = "degraded"; result.DegradationReason = "max_steps_reached" }
    }
    return result
}
```

---

## 4. 引擎

### 4.1 engine.go

```go
package gos_engine

import (
    "context"
    "fmt"
    "time"
    "github.com/google/uuid"
    "SuperBizAgent/internal/ai/belief"
    "SuperBizAgent/internal/ai/agent/experts"
    "SuperBizAgent/internal/ai/protocol"
)

type GoSEngine struct {
    graph     *belief.BeliefGraph
    fsm       *belief.BeliefFSM
    experts   map[string]experts.ExpertAgent
    ragPool   *rag.RetrieverPool
    llmClient LLMClient
    cfg       *Config
    startedAt time.Time
}

type actResult struct {
    Analyses      []*experts.ExpertAnalysis
    DegradedCount int
    FailedCount   int
}

func (e *GoSEngine) Run(ctx context.Context, symptom string) *protocol.TaskResult {
    e.startedAt = time.Now()

    if err := e.ingest(ctx, symptom); err != nil {
        return e.degradedResult("ingest_failed", err, nil, false)
    }

    for {
        if e.fsm.IsFinalState() { break }

        frontier := e.graph.ExtractFrontier(e.fsm.GetCurrentLevel())
        if frontier == nil { e.fsm.MarkDone("no frontier"); break }

        plan, err := e.plan(ctx, frontier)
        if err != nil { return e.degradedResult("plan_failed", err, nil, false) }

        actRes, err := e.act(ctx, plan, frontier)

        alreadyUpdated := false
        if actRes != nil && len(actRes.Analyses) > 0 {
            if res := e.updateGraph(ctx, actRes.Analyses, frontier); res.Committed {
                alreadyUpdated = true
            }
        }

        if err != nil { return e.degradedResult("act_failed", err, actRes, alreadyUpdated) }

        e.graph.GenerateBeliefText()
        e.fsm.TickStep(1)

        decision := e.fsm.Decide(e.graph)
        switch decision.Action {
        case "report":
            if e.shouldReport(frontier) {
                e.fsm.MarkDone("sufficient granularity")
                goto DONE
            }
            e.fsm.DrillDown(fmt.Sprintf("drill to level %d", e.fsm.CurrentLevel+1))
        case "done":
            goto DONE
        }

        if e.fsm.TotalSteps >= e.cfg.SessionMaxSteps { e.fsm.MarkDone("max steps"); break }
    }

DONE:
    return e.generateReport(ctx)
}

func (e *GoSEngine) updateGraph(ctx context.Context, analyses []*experts.ExpertAnalysis, frontier *belief.Frontier) *belief.GraphUpdateResult {
    return e.graph.UpdateCopyOnWrite(func(cp *belief.BeliefGraph) error {
        for _, a := range analyses {
            for _, ev := range a.Evidence {
                src := &belief.EvidenceSource{SourceType: ev.SourceType, SourceID: ev.SourceID, SummarySnippet: ev.Snippet}
                eid := cp.AddEvidenceCopy(ev.Title, src)
                edgeType := belief.EdgeSupport
                if a.Confidence < 0.5 { edgeType = belief.EdgeRefute }
                cp.AddEdgeCopy(eid, frontier.NodeID, edgeType, ev.Score, "expert_analysis")
            }
            if a.Confidence > 0 {
                cp.UpdateNodeCopy(frontier.NodeID, a.Confidence, a.Analysis)
            }
        }
        return nil
    })
}

func (e *GoSEngine) degradedResult(reason string, err error, actRes *actResult, alreadyUpdated bool) *protocol.TaskResult {
    if !alreadyUpdated && actRes != nil && len(actRes.Analyses) > 0 {
        if f := e.graph.ExtractFrontier(e.fsm.GetCurrentLevel()); f != nil {
            e.updateGraph(context.Background(), actRes.Analyses, f)
        }
    }
    return &protocol.TaskResult{
        TaskID: uuid.NewString(), Agent: "gos_engine",
        Status: protocol.ResultStatusDegraded, Summary: "诊断降级",
        DegradationReason: fmt.Sprintf("%s: %v", reason, err),
        Evidence: e.collectEvidence(), Metadata: map[string]any{
            "belief_graph": e.graph.ToDict(), "fsm_history": e.fsm.History, "error_phase": reason,
        },
        StartedAt: e.startedAt.UnixMilli(), FinishedAt: time.Now().UnixMilli(),
    }
}
```

---

## 5. 输出契约

严格复用 `protocol.TaskResult`，不新增字段。graph/fsm 放 `Metadata`。

```go
// 不修改 internal/ai/protocol/types.go
// graph → Metadata["belief_graph"]
// fsm  → Metadata["fsm_history"]
```

---

## 6. 配置

```yaml
aiops:
  engine: "plan_execute_replan"
  gos:
    enabled: false
    model_path: "deepseek-v3"
    temperature: 0.8
    max_tokens: 4096
    session_max_steps: 5
    max_retrieval_steps: 3
    fsm:
      gap_delta: 0.3
      min_support: 2
      max_steps: 3
    experts:
      - name: "linux_sre"
        description: "Linux SRE 专家"
        tools: ["query_logs", "query_internal_docs"]
        max_retrieval_steps: 3
      - name: "network_sre"
        description: "网络 SRE 专家"
        tools: ["query_logs", "query_internal_docs"]
      - name: "database_sre"
        description: "数据库 SRE 专家"
        tools: ["query_logs", "query_internal_docs"]
```

---

## 7. 目录结构

```
internal/ai/belief/
├── types.go
├── graph.go          # 带锁公开方法 + 内部方法 + copy-on-write + 副本方法
├── fsm.go
└── graph_test.go

internal/ai/agent/gos_engine/
├── engine.go
├── ingestor.go
├── planner.go
├── updater.go
├── reporter.go
├── prompts.go
├── config.go
└── engine_test.go

internal/ai/agent/experts/
├── registry.go       # ToolRegistry + InitFromExisting
├── tool_adapter.go   # ToolAdapter
├── arg_builders.go   # QueryArgBuilder / RawArgBuilder
├── linux_sre.go
├── network_sre.go
├── database_sre.go
└── experts_test.go
```

---

## 8. 评测 Gate

| 指标 | 条件 |
|------|------|
| 诊断质量 | >= baseline |
| 证据覆盖率 | >= baseline |
| 延迟 | <= baseline * 1.5 |
| LLM 调用次数 | <= baseline * 2 |
| 降级率 | <= baseline |
| 可追溯性 | = 100% |

全部通过才能进入灰度。

---

## 9. 设计约束

1. **单线程执行**：同一 BeliefGraph 实例的 UpdateCopyOnWrite 不会被并发调用（GoS FSM Loop 单线程）
2. **append-only**：节点/边只标记 retracted/superseded，不删除
3. **深拷贝快照**：takeSnapshot 深拷贝 Attrs map 和 Source 指针
4. **强类型 FSM**：FSMState 是 int，不是 interface{}；Decide 只返回决策不改状态
5. **安全读取**：safeWhy 避免 nil/类型断言 panic
6. **事务性更新**：UpdateCopyOnWrite 全成功或全回滚
7. **证据链接**：applyAnalysis 创建 evidence → hypothesis 的 support/refute 边
8. **降级不丢证据**：degradedResult 接收 actRes，alreadyUpdated 避免重复写图
