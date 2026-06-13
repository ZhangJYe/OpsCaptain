# Full Agentic RAG Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Full Agentic RAG — 在 Plan A (QueryPlanner) 基础上增加检索质量评估 + 多轮自适应检索。

**Architecture:** AgentRAG 循环：检索(复用 QueryWithPlanner) → 评估(Evaluator LLM) → 规划(RetrievalPlanner LLM) → 下一轮。评估对象为累计候选集，去重用 seenDocIDs，外层有 TotalTimeoutMs 总超时。

**Tech Stack:** Go 1.24, GoFrame v2, Eino schema, DeepSeek chat_model_fast

---

## 文件结构

| 文件 | 职责 | 操作 |
|------|------|------|
| `internal/ai/rag/agent_config.go` | Agent 配置加载 | 新建 |
| `internal/ai/rag/evaluator.go` | 检索质量评估器 (LLM) | 新建 |
| `internal/ai/rag/retrieval_planner.go` | 多轮检索规划器 (LLM) | 新建 |
| `internal/ai/rag/agent_rag.go` | AgentRAG 主入口 + 辅助函数 | 新建 |
| `internal/ai/rag/agent_rag_test.go` | 单元测试 | 新建 |
| `internal/ai/rag/eval/online.go` | 扩展 QueryMetrics + QuerySummary | 修改 |
| `internal/ai/cmd/rag_online_eval_cmd/main.go` | 新增 agent eval mode | 修改 |
| `internal/ai/promptreg/rag_evaluator.txt` | Evaluator system prompt | 新建 |
| `internal/ai/promptreg/rag_planner_agent.txt` | Planner system prompt | 新建 |
| `internal/ai/promptreg/promptreg.go` | 注册新 prompt | 修改 |
| `manifest/config/config.yaml` | 新增 rag.agent 配置块 | 修改 |
| `deploy/config.prod.yaml` | 新增 rag.agent 配置块 | 修改 |

---

### Task 1: Agent 配置加载

**Files:**
- Create: `internal/ai/rag/agent_config.go`

- [ ] **Step 1: 创建 agent_config.go**

```go
package rag

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type AgentConfig struct {
	Enabled             bool
	Model               string
	MaxRounds           int
	ConfidenceThreshold float64
	EvalTimeoutMs       int
	PlanTimeoutMs       int
	TotalTimeoutMs      int
	MaxTotalTokens      int
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled:             false,
		Model:               "chat_model_fast",
		MaxRounds:           3,
		ConfidenceThreshold: 0.7,
		EvalTimeoutMs:       3000,
		PlanTimeoutMs:       2000,
		TotalTimeoutMs:      30000,
		MaxTotalTokens:      8000,
	}
}

func LoadAgentConfig(ctx context.Context) AgentConfig {
	cfg := DefaultAgentConfig()

	v, err := g.Cfg().Get(ctx, "rag.agent.enabled")
	if err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.model")
	if err == nil && v.String() != "" {
		cfg.Model = v.String()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.max_rounds")
	if err == nil && v.Int() > 0 {
		cfg.MaxRounds = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.confidence_threshold")
	if err == nil && v.Float64() > 0 {
		cfg.ConfidenceThreshold = v.Float64()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.eval_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.EvalTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.plan_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.PlanTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.total_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.TotalTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.agent.max_total_tokens")
	if err == nil && v.Int() > 0 {
		cfg.MaxTotalTokens = v.Int()
	}

	return cfg
}

func agentEvalTimeout(cfg AgentConfig) time.Duration {
	return time.Duration(cfg.EvalTimeoutMs) * time.Millisecond
}

func agentPlanTimeout(cfg AgentConfig) time.Duration {
	return time.Duration(cfg.PlanTimeoutMs) * time.Millisecond
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/agent_config.go
git commit -m "feat(rag): 新增 AgentRAG 配置加载"
```

---

### Task 2: Prompt 注册

**Files:**
- Create: `internal/ai/promptreg/rag_evaluator.txt`
- Create: `internal/ai/promptreg/rag_planner_agent.txt`
- Modify: `internal/ai/promptreg/promptreg.go`

- [ ] **Step 1: 创建 rag_evaluator.txt**

```
你是一个运维知识检索质量评估器。给定用户问题和检索结果，判断是否足够回答。
评估维度：覆盖度、相关性、证据强度。输出 JSON。
```

- [ ] **Step 2: 创建 rag_planner_agent.txt**

```
你是一个运维检索策略规划器。根据评估结果和已检索文档，规划下一轮检索。
针对缺失信息制定子查询，避免重复已检索内容。
```

- [ ] **Step 3: 修改 promptreg.go 注册新 prompt**

在 `promptreg.go` 中添加：

```go
//go:embed rag_evaluator.txt
var RAGEvaluator string

//go:embed rag_planner_agent.txt
var RAGPlannerAgent string
```

在 `init()` 中添加：

```go
RAGEvaluator = strings.TrimSpace(RAGEvaluator)
RAGPlannerAgent = strings.TrimSpace(RAGPlannerAgent)
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./internal/ai/promptreg/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/promptreg/rag_evaluator.txt internal/ai/promptreg/rag_planner_agent.txt internal/ai/promptreg/promptreg.go
git commit -m "feat(rag): 注册 AgentRAG evaluator 与 planner prompt"
```

---

### Task 3: 评估器 (Evaluator)

**Files:**
- Create: `internal/ai/rag/evaluator.go`

- [ ] **Step 1: 创建 evaluator.go**

```go
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/promptreg"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type EvalResult struct {
	Confidence   float64
	Sufficient   bool
	MissingInfo  []string
	NextStrategy string
	Reason       string
}

type Evaluator struct {
	model string
}

func NewEvaluator(model string) *Evaluator {
	return &Evaluator{model: model}
}

const evaluatorPrompt = `你是运维知识检索质量评估器。给定用户问题和检索结果，判断是否足够回答。

用户问题: %s
已检索文档:
%s
已尝试轮数: %d/%d

评估维度:
1. 是否覆盖了问题的所有方面？
2. 结果与问题的相关性如何？
3. 是否有明确的证据支持结论？

输出 JSON:
{
  "confidence": 0.0-1.0,
  "sufficient": true/false,
  "missing_info": ["缺失点1"],
  "next_strategy": "expand_scope|refine_query|add_angle|none",
  "reason": "评估理由"
}`

func (e *Evaluator) Evaluate(ctx context.Context, query string, docs []*schema.Document, round, maxRounds int) EvalResult {
	evalCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(evalCtx)
	if err != nil {
		g.Log().Debugf(ctx, "evaluator model init failed: %v", err)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "model init failed"}
	}

	docSummaries := formatDocsForEval(docs)
	prompt := fmt.Sprintf(evaluatorPrompt, query, docSummaries, round, maxRounds)

	resp, err := chatModel.Generate(evalCtx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		g.Log().Debugf(ctx, "evaluator LLM call failed: %v", err)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "LLM call failed"}
	}

	result := parseEvalResult(resp.Content)
	return result
}

func formatDocsForEval(docs []*schema.Document) string {
	var sb strings.Builder
	for i, doc := range docs {
		if doc == nil {
			continue
		}
		title := ""
		if doc.MetaData != nil {
			for _, key := range []string{"title", "file_name", "source"} {
				if v, ok := doc.MetaData[key].(string); ok && v != "" {
					title = v
					break
				}
			}
		}
		if title == "" {
			title = doc.ID
		}
		content := doc.Content
		runes := []rune(content)
		if len(runes) > 200 {
			content = string(runes[:200]) + "..."
		}
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, title, content)
	}
	return sb.String()
}

func parseEvalResult(raw string) EvalResult {
	type evalResponse struct {
		Confidence   float64  `json:"confidence"`
		Sufficient   bool     `json:"sufficient"`
		MissingInfo  []string `json:"missing_info"`
		NextStrategy string   `json:"next_strategy"`
		Reason       string   `json:"reason"`
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var r evalResponse
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		g.Log().Debugf(context.Background(), "evaluator parse failed: %v, raw=%q", err, raw)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "parse failed"}
	}

	if r.Confidence < 0 || r.Confidence > 1 {
		r.Confidence = 0.5
	}
	if r.NextStrategy == "" {
		r.NextStrategy = "none"
	}

	return EvalResult{
		Confidence:   r.Confidence,
		Sufficient:   r.Sufficient,
		MissingInfo:  r.MissingInfo,
		NextStrategy: r.NextStrategy,
		Reason:       r.Reason,
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/evaluator.go
git commit -m "feat(rag): 实现检索质量评估器 (Evaluator)"
```

---

### Task 4: 检索规划器 (RetrievalPlanner)

**Files:**
- Create: `internal/ai/rag/retrieval_planner.go`

- [ ] **Step 1: 创建 retrieval_planner.go**

```go
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type RetrievalPlan struct {
	SubQueries []string
	Strategy   string
	Reason     string
}

type RetrievalPlanner struct {
	model string
}

func NewRetrievalPlanner(model string) *RetrievalPlanner {
	return &RetrievalPlanner{model: model}
}

const retrievalPlannerPrompt = `你是运维检索策略规划器。根据评估结果和已检索文档，规划下一轮检索。

用户问题: %s
评估结果:
- 置信度: %.2f
- 是否充分: %v
- 缺失信息: %s
- 建议策略: %s

已检索文档 ID: %s

规则:
1. 针对 missing_info 制定检索子查询
2. 最多 3 个子查询
3. 不要重复已检索的文档 ID（程序会自动过滤，但请尽量避免）

输出 JSON:
{
  "sub_queries": ["子查询1", "子查询2"],
  "strategy": "expand_scope|refine_query|add_angle",
  "reason": "规划理由"
}`

func (rp *RetrievalPlanner) Plan(ctx context.Context, query string, evalResult EvalResult, candidateDocs []*schema.Document) RetrievalPlan {
	planCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(planCtx)
	if err != nil {
		g.Log().Debugf(ctx, "retrieval planner model init failed: %v", err)
		return RetrievalPlan{Strategy: "none", Reason: "model init failed"}
	}

	missingInfoStr := "无"
	if len(evalResult.MissingInfo) > 0 {
		missingInfoStr = strings.Join(evalResult.MissingInfo, ", ")
	}

	docIDs := extractDocIDs(candidateDocs)
	prompt := fmt.Sprintf(retrievalPlannerPrompt,
		query, evalResult.Confidence, evalResult.Sufficient,
		missingInfoStr, evalResult.NextStrategy, docIDs)

	resp, err := chatModel.Generate(planCtx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		g.Log().Debugf(ctx, "retrieval planner LLM call failed: %v", err)
		return RetrievalPlan{Strategy: "none", Reason: "LLM call failed"}
	}

	return parseRetrievalPlan(resp.Content)
}

func extractDocIDs(docs []*schema.Document) string {
	ids := make([]string, 0, len(docs))
	seen := make(map[string]struct{})
	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return strings.Join(ids, ", ")
}

func parseRetrievalPlan(raw string) RetrievalPlan {
	type planResponse struct {
		SubQueries []string `json:"sub_queries"`
		Strategy   string   `json:"strategy"`
		Reason     string   `json:"reason"`
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var r planResponse
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		g.Log().Debugf(context.Background(), "retrieval planner parse failed: %v, raw=%q", err, raw)
		return RetrievalPlan{Strategy: "none", Reason: "parse failed"}
	}

	if r.Strategy == "" {
		r.Strategy = "none"
	}

	return RetrievalPlan{
		SubQueries: r.SubQueries,
		Strategy:   r.Strategy,
		Reason:     r.Reason,
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/retrieval_planner.go
git commit -m "feat(rag): 实现多轮检索规划器 (RetrievalPlanner)"
```

---

### Task 5: AgentRAG 主入口 + 辅助函数

**Files:**
- Create: `internal/ai/rag/agent_rag.go`

- [ ] **Step 1: 创建 agent_rag.go**

```go
package rag

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type AgentTrace struct {
	Rounds          int
	FinalConfidence float64
	TotalLatencyMs  int64
	RoundTraces     []RoundTrace
}

type RoundTrace struct {
	Round         int
	SubQueryCount int
	DocCount      int
	NewDocCount   int
	Confidence    float64
	Strategy      string
	LatencyMs     int64
}

type AgentRAG struct {
	evaluator *Evaluator
	planner   *RetrievalPlanner
	cfg       AgentConfig
}

func NewAgentRAG(cfg AgentConfig) *AgentRAG {
	return &AgentRAG{
		evaluator: NewEvaluator(cfg.Model),
		planner:   NewRetrievalPlanner(cfg.Model),
		cfg:       cfg,
	}
}

func (a *AgentRAG) Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, AgentTrace, error) {
	start := time.Now()
	trace := AgentTrace{}

	if !a.cfg.Enabled {
		docs, _, err := QueryWithPlanner(ctx, pool, query, LoadPlannerConfig(ctx))
		return docs, trace, err
	}

	totalTimeout := time.Duration(a.cfg.TotalTimeoutMs) * time.Millisecond
	agentCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	allDocs := make([]*schema.Document, 0)
	seenDocIDs := make(map[string]struct{})
	currentQuery := query
	plannerCfg := LoadPlannerConfig(ctx)

	for round := 0; round < a.cfg.MaxRounds; round++ {
		if agentCtx.Err() != nil {
			g.Log().Debugf(ctx, "agent rag: total timeout at round %d", round+1)
			break
		}

		roundTrace := RoundTrace{Round: round + 1}
		roundStart := time.Now()

		docs, merged, err := QueryWithPlanner(agentCtx, pool, currentQuery, plannerCfg)
		if err != nil {
			g.Log().Debugf(ctx, "agent rag: retrieval failed at round %d: %v", round+1, err)
			break
		}

		newDocs := filterNewDocs(docs, seenDocIDs)
		roundTrace.DocCount = len(docs)
		roundTrace.NewDocCount = len(newDocs)
		roundTrace.SubQueryCount = merged.Trace.SubQueryCount

		if len(newDocs) == 0 && round > 0 {
			g.Log().Debugf(ctx, "agent rag: no new docs at round %d, stopping", round+1)
			break
		}

		allDocs = append(allDocs, newDocs...)
		candidateDocs := mergeAndDedup(allDocs)

		evalResult := a.evaluator.Evaluate(agentCtx, query, candidateDocs, round+1, a.cfg.MaxRounds)
		roundTrace.Confidence = evalResult.Confidence
		roundTrace.Strategy = evalResult.NextStrategy

		trace.RoundTraces = append(trace.RoundTraces, roundTrace)
		trace.Rounds = round + 1
		trace.FinalConfidence = evalResult.Confidence

		g.Log().Debugf(ctx, "agent rag round %d: docs=%d new=%d confidence=%.2f sufficient=%v",
			round+1, len(docs), len(newDocs), evalResult.Confidence, evalResult.Sufficient)

		if evalResult.Sufficient || evalResult.Confidence >= a.cfg.ConfidenceThreshold {
			break
		}

		if round < a.cfg.MaxRounds-1 {
			plan := a.planner.Plan(agentCtx, query, evalResult, candidateDocs)
			roundTrace.Strategy = plan.Strategy
			if plan.Strategy == "none" || len(plan.SubQueries) == 0 {
				break
			}
			currentQuery = strings.Join(plan.SubQueries, " ")
		}
	}

	finalDocs := mergeAndDedup(allDocs)
	trace.TotalLatencyMs = time.Since(start).Milliseconds()
	return finalDocs, trace, nil
}

func filterNewDocs(docs []*schema.Document, seen map[string]struct{}) []*schema.Document {
	var out []*schema.Document
	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, doc)
	}
	return out
}

func canonicalDocID(doc *schema.Document) string {
	if doc == nil {
		return ""
	}
	if doc.MetaData != nil {
		for _, key := range []string{"case_id", "caseid", "doc_id"} {
			if v, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return strings.TrimSpace(doc.ID)
}

func mergeAndDedup(docs []*schema.Document) []*schema.Document {
	seen := make(map[string]*schema.Document, len(docs))
	order := make([]string, 0, len(docs))

	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			id = doc.ID
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = doc
		order = append(order, id)
	}

	out := make([]*schema.Document, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/agent_rag.go
git commit -m "feat(rag): 实现 AgentRAG 主入口 (多轮检索 + 评估 + 规划)"
```

---

### Task 6: 单元测试

**Files:**
- Create: `internal/ai/rag/agent_rag_test.go`

- [ ] **Step 1: 创建 agent_rag_test.go**

```go
package rag

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestDefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()
	if cfg.Enabled {
		t.Error("default config should be disabled")
	}
	if cfg.MaxRounds != 3 {
		t.Errorf("MaxRounds = %d, want 3", cfg.MaxRounds)
	}
	if cfg.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7", cfg.ConfidenceThreshold)
	}
	if cfg.TotalTimeoutMs != 30000 {
		t.Errorf("TotalTimeoutMs = %d, want 30000", cfg.TotalTimeoutMs)
	}
}

func TestFilterNewDocs(t *testing.T) {
	seen := map[string]struct{}{
		"doc-a": {},
		"doc-b": {},
	}
	docs := []*schema.Document{
		{ID: "doc-a", Content: "old"},
		{ID: "doc-b", Content: "old"},
		{ID: "doc-c", Content: "new"},
		{ID: "doc-d", Content: "new"},
	}

	newDocs := filterNewDocs(docs, seen)
	if len(newDocs) != 2 {
		t.Fatalf("expected 2 new docs, got %d", len(newDocs))
	}
	if newDocs[0].ID != "doc-c" {
		t.Errorf("first new doc should be doc-c, got %s", newDocs[0].ID)
	}
	if newDocs[1].ID != "doc-d" {
		t.Errorf("second new doc should be doc-d, got %s", newDocs[1].ID)
	}
}

func TestFilterNewDocs_Empty(t *testing.T) {
	seen := map[string]struct{}{"doc-a": {}}
	docs := []*schema.Document{{ID: "doc-a"}}
	newDocs := filterNewDocs(docs, seen)
	if len(newDocs) != 0 {
		t.Errorf("expected 0 new docs, got %d", len(newDocs))
	}
}

func TestMergeAndDedup(t *testing.T) {
	docs := []*schema.Document{
		{ID: "doc-a", Content: "first"},
		{ID: "doc-b", Content: "second"},
		{ID: "doc-a", Content: "duplicate"},
		{ID: "doc-c", Content: "third"},
	}

	result := mergeAndDedup(docs)
	if len(result) != 3 {
		t.Fatalf("expected 3 docs after dedup, got %d", len(result))
	}
	if result[0].Content != "first" {
		t.Errorf("first doc should keep original, got %s", result[0].Content)
	}
}

func TestMergeAndDedup_Empty(t *testing.T) {
	result := mergeAndDedup(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 docs, got %d", len(result))
	}
}

func TestCanonicalDocID(t *testing.T) {
	tests := []struct {
		name     string
		doc      *schema.Document
		expected string
	}{
		{
			name:     "nil doc",
			doc:      nil,
			expected: "",
		},
		{
			name:     "case_id in meta",
			doc:      &schema.Document{ID: "fallback", MetaData: map[string]any{"case_id": "case-123"}},
			expected: "case-123",
		},
		{
			name:     "doc_id in meta",
			doc:      &schema.Document{ID: "fallback", MetaData: map[string]any{"doc_id": "doc-456"}},
			expected: "doc-456",
		},
		{
			name:     "fallback to ID",
			doc:      &schema.Document{ID: "doc-789"},
			expected: "doc-789",
		},
		{
			name:     "empty ID",
			doc:      &schema.Document{ID: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canonicalDocID(tt.doc)
			if result != tt.expected {
				t.Errorf("canonicalDocID() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseEvalResult(t *testing.T) {
	raw := `{"confidence": 0.85, "sufficient": true, "missing_info": [], "next_strategy": "none", "reason": "覆盖充分"}`
	result := parseEvalResult(raw)
	if result.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85", result.Confidence)
	}
	if !result.Sufficient {
		t.Error("Sufficient should be true")
	}
	if result.NextStrategy != "none" {
		t.Errorf("NextStrategy = %s, want none", result.NextStrategy)
	}
}

func TestParseEvalResult_Invalid(t *testing.T) {
	result := parseEvalResult("not json")
	if result.Confidence != 0.5 {
		t.Errorf("fallback Confidence = %f, want 0.5", result.Confidence)
	}
	if result.Sufficient {
		t.Error("fallback Sufficient should be false")
	}
}

func TestParseRetrievalPlan(t *testing.T) {
	raw := `{"sub_queries": ["Redis 状态", "Redis 连接"], "strategy": "expand_scope", "reason": "补充 Redis 信息"}`
	result := parseRetrievalPlan(raw)
	if len(result.SubQueries) != 2 {
		t.Fatalf("expected 2 sub queries, got %d", len(result.SubQueries))
	}
	if result.Strategy != "expand_scope" {
		t.Errorf("Strategy = %s, want expand_scope", result.Strategy)
	}
}

func TestParseRetrievalPlan_Invalid(t *testing.T) {
	result := parseRetrievalPlan("bad")
	if result.Strategy != "none" {
		t.Errorf("fallback Strategy = %s, want none", result.Strategy)
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/ai/rag/ -run "TestDefaultAgentConfig|TestFilterNewDocs|TestMergeAndDedup|TestCanonicalDocID|TestParseEvalResult|TestParseRetrievalPlan" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/agent_rag_test.go
git commit -m "test(rag): 新增 AgentRAG 单元测试"
```

---

### Task 7: Eval Metrics 扩展

**Files:**
- Modify: `internal/ai/rag/eval/online.go`

- [ ] **Step 1: 在 QueryMetrics 中添加 agent 字段**

```go
type QueryMetrics struct {
	// ... existing fields ...
	AgentRounds     int     `json:"agent_rounds"`
	FinalConfidence float64 `json:"final_confidence"`
	AgentLatencyMs  int64   `json:"agent_latency_ms"`
}
```

- [ ] **Step 2: 在 QuerySummary 中添加 agent 字段**

```go
type QuerySummary struct {
	Summary
	// ... existing fields ...
	AvgAgentRounds    float64 `json:"avg_agent_rounds"`
	AvgFinalConfidence float64 `json:"avg_final_confidence"`
	AvgAgentLatencyMs float64 `json:"avg_agent_latency_ms"`
}
```

- [ ] **Step 3: 在 accumulateQueryMetrics 中累加新字段**

在 `accumulateQueryMetrics` 函数末尾添加：

```go
	if metrics.AgentRounds > 0 {
		qSummary.AvgAgentRounds += float64(metrics.AgentRounds)
		qSummary.AvgFinalConfidence += metrics.FinalConfidence
		qSummary.AvgAgentLatencyMs += float64(metrics.AgentLatencyMs)
	}
```

- [ ] **Step 4: 在 finalizeQuerySummary 中计算平均值**

在 `finalizeQuerySummary` 中添加（在 caseCount 检查之后）：

```go
	if qSummary.AvgAgentRounds > 0 {
		agentCases := float64(0)
		for _, r := range results {
			if r.Metrics.AgentRounds > 0 {
				agentCases++
			}
		}
		if agentCases > 0 {
			qSummary.AvgAgentRounds /= agentCases
			qSummary.AvgFinalConfidence /= agentCases
			qSummary.AvgAgentLatencyMs /= agentCases
		}
	}
```

- [ ] **Step 5: 验证编译通过**

Run: `go build ./internal/ai/rag/eval/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/rag/eval/online.go
git commit -m "feat(rag): 扩展 eval metrics 支持 agent mode 指标"
```

---

### Task 8: Agent Eval Mode 集成

**Files:**
- Modify: `internal/ai/cmd/rag_online_eval_cmd/main.go`

- [ ] **Step 1: 在 parseEvalMode 中添加 agent case**

```go
case "agent":
    return false, false, false, true
```

- [ ] **Step 2: 在 main 函数中添加 agent 执行路径**

在现有的 `if *modeRaw == "planner"` 块之后，添加：

```go
if *modeRaw == "agent" {
    agentCfg := rag.LoadAgentConfig(context.Background())
    agentCfg.Enabled = true
    agent := rag.NewAgentRAG(agentCfg)
    exec = func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
        start := time.Now()
        queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
        defer cancel()

        docs, agentTrace, err := agent.Query(queryCtx, rag.SharedPool(), query)
        if err != nil {
            return nil, eval.QueryMetrics{}, err
        }

        metrics := eval.QueryMetrics{
            TotalLatencyMs:  time.Since(start).Milliseconds(),
            ResultCount:     len(docs),
            AgentRounds:     agentTrace.Rounds,
            FinalConfidence: agentTrace.FinalConfidence,
            AgentLatencyMs:  agentTrace.TotalLatencyMs,
        }
        return eval.SchemaDocsToRetrievedDocs(docs), metrics, nil
    }
}
```

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/ai/cmd/rag_online_eval_cmd/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/cmd/rag_online_eval_cmd/main.go
git commit -m "feat(rag): eval 支持 agent mode"
```

---

### Task 9: 配置文件

**Files:**
- Modify: `manifest/config/config.yaml`
- Modify: `deploy/config.prod.yaml`

- [ ] **Step 1: 在 config.yaml 的 rag 块下添加 agent 配置**

在 `rag.planner` 块之后添加：

```yaml
  agent:
    enabled: false
    model: "chat_model_fast"
    max_rounds: 3
    confidence_threshold: 0.7
    eval_timeout_ms: 3000
    plan_timeout_ms: 2000
    total_timeout_ms: 30000
    max_total_tokens: 8000
```

- [ ] **Step 2: 在 deploy/config.prod.yaml 的 rag 块下添加 agent 配置**

在 `rag.planner` 块之后添加：

```yaml
  agent:
    enabled: false
    model: "chat_model_fast"
    max_rounds: 3
    confidence_threshold: 0.7
    eval_timeout_ms: 3000
    plan_timeout_ms: 2000
    total_timeout_ms: 30000
    max_total_tokens: 8000
```

- [ ] **Step 3: Commit**

```bash
git add manifest/config/config.yaml deploy/config.prod.yaml
git commit -m "config(rag): 新增 AgentRAG 配置项"
```

---

### Task 10: 全量测试验证

- [ ] **Step 1: 运行全部测试**

Run: `go test ./internal/ai/rag/... -v`
Expected: PASS

- [ ] **Step 2: 运行 go vet**

Run: `go vet ./internal/ai/rag/...`
Expected: PASS

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 4: 运行 in-memory eval**

Run: `go run ./internal/ai/cmd/rag_eval_cmd/ -ks 1,3,5`
Expected: 基线不变 (Recall@1=0.89, Recall@5=1.00)

---

## 实施顺序

1. Task 1: Agent 配置加载
2. Task 2: Prompt 注册
3. Task 3: 评估器 (Evaluator)
4. Task 4: 检索规划器 (RetrievalPlanner)
5. Task 5: AgentRAG 主入口
6. Task 6: 单元测试
7. Task 7: Eval Metrics 扩展
8. Task 8: Agent Eval Mode 集成
9. Task 9: 配置文件
10. Task 10: 全量测试

## 验证清单

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/ai/rag/...` 通过
- [ ] `go test ./internal/ai/contextengine/...` 通过
- [ ] `go vet ./internal/ai/rag/...` 通过
- [ ] 无 import 规则违规
- [ ] 配置项走 config.yaml，无硬编码
- [ ] commit message 用中文
