# Agent RAG Query Planner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Query Planner，通过 LLM 查询分解 + 并行检索 + RRF 合并，提升复合 query 的检索召回率。

**Architecture:** 在 `rag.Query()` 上层加 `QueryPlanner`，职责：判断是否拆解 → LLM 生成子查询 → 并行执行 → RRF 合并。不改检索内核，任何故障降级到现有 pipeline。

**Tech Stack:** Go 1.24, GoFrame v2, Eino schema, DeepSeek chat_model_fast

---

## 文件结构

| 文件 | 职责 | 操作 |
|------|------|------|
| `internal/ai/rag/planner_config.go` | 配置加载 | 新建 |
| `internal/ai/rag/planner.go` | QueryPlanner 核心 (Analyze + Execute + Merge) | 新建 |
| `internal/ai/rag/planner_test.go` | 单元测试 | 新建 |
| `internal/ai/rag/eval/online.go` | 扩展 QueryMetrics | 修改 |
| `internal/ai/cmd/rag_online_eval_cmd/main.go` | 新增 planner eval mode | 修改 |
| `manifest/config/config.yaml` | 新增 rag.planner 配置块 | 修改 |

---

### Task 1: 配置加载

**Files:**
- Create: `internal/ai/rag/planner_config.go`

- [ ] **Step 1: 创建 planner_config.go**

```go
package rag

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type PlannerConfig struct {
	Enabled               bool
	Model                 string
	TimeoutMs             int
	MaxSubQueries         int
	MinQueryLength        int
	ExecTimeoutMs         int
	DecompositionKeywords []string
}

func DefaultPlannerConfig() PlannerConfig {
	return PlannerConfig{
		Enabled:           false,
		Model:             "chat_model_fast",
		TimeoutMs:         200,
		MaxSubQueries:     4,
		MinQueryLength:    15,
		ExecTimeoutMs:     5000,
		DecompositionKeywords: []string{"和", "以及", "还有", "跟", "为什么", "怎么回事", "导致", "关系"},
	}
}

func LoadPlannerConfig(ctx context.Context) PlannerConfig {
	cfg := DefaultPlannerConfig()

	v, err := g.Cfg().Get(ctx, "rag.planner.enabled")
	if err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.model")
	if err == nil && v.String() != "" {
		cfg.Model = v.String()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.TimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.max_sub_queries")
	if err == nil && v.Int() > 0 {
		cfg.MaxSubQueries = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.min_query_length")
	if err == nil && v.Int() > 0 {
		cfg.MinQueryLength = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.exec_timeout_ms")
	if err == nil && v.Int() > 0 {
		cfg.ExecTimeoutMs = v.Int()
	}
	v, err = g.Cfg().Get(ctx, "rag.planner.decomposition_keywords")
	if err == nil && v.Strings() != nil && len(v.Strings()) > 0 {
		cfg.DecompositionKeywords = v.Strings()
	}

	return cfg
}

func plannerTimeout(ctx context.Context, cfg PlannerConfig) time.Duration {
	return time.Duration(cfg.TimeoutMs) * time.Millisecond
}

func plannerExecTimeout(ctx context.Context, cfg PlannerConfig) time.Duration {
	return time.Duration(cfg.ExecTimeoutMs) * time.Millisecond
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS (no output)

- [ ] **Step 3: Commit**

```bash
git add internal/ai/rag/planner_config.go
git commit -m "feat(rag): 新增 QueryPlanner 配置加载"
```

---

### Task 2: QueryPlanner 核心逻辑

**Files:**
- Create: `internal/ai/rag/planner.go`

- [ ] **Step 1: 创建 planner.go — PlanResult 和辅助函数**

```go
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/promptreg"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type PlanResult struct {
	Decomposed bool
	SubQueries []string
	Reason     string
	LatencyMs  int64
}

type PlannerTrace struct {
	Analyzed       bool
	SubQueryCount  int
	PlanLatencyMs  int64
	ExecLatencyMs  int64
	MergeLatencyMs int64
	FallbackReason string
}

type MergedResult struct {
	Docs    []*schema.Document
	Trace   PlannerTrace
}

type plannerSubQueryMap map[string][]string
```

- [ ] **Step 2: 添加 Analyze 方法**

```go
// needsDecomposition 规则检测：是否需要拆解
func needsDecomposition(query string, cfg PlannerConfig) bool {
	if len([]rune(query)) < cfg.MinQueryLength {
		return false
	}
	for _, kw := range cfg.DecompositionKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	return false
}

const plannerDecomposePrompt = `你是运维查询分析器。将用户问题拆解为独立的检索子查询。

规则：
1. 每个子查询聚焦一个具体信息点
2. 最多 %d 个子查询
3. 保留原始 query 的语义关键词
4. 如果问题已经是单一主题，返回空数组 []
5. 只输出 JSON 数组，不要其他文字

示例：
用户: "payment 服务最近为什么延迟升高，跟 Redis 有没有关系？"
输出: ["payment 服务延迟升高指标", "payment 服务错误日志", "Redis 性能异常", "payment Redis 连接"]

用户: "Prometheus 告警先看什么"
输出: []

用户: `%s`
输出:`

// decomposeQuery 使用 LLM 拆解查询
func decomposeQuery(ctx context.Context, query string, cfg PlannerConfig) ([]string, error) {
	planCtx, cancel := context.WithTimeout(ctx, plannerTimeout(ctx, cfg))
	defer cancel()

	chatModel, err := models.OpenAIForGLMByPath(planCtx, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("planner model init failed: %w", err)
	}

	prompt := fmt.Sprintf(plannerDecomposePrompt, cfg.MaxSubQueries, query)
	resp, err := chatModel.Generate(planCtx, []*schema.Message{
		{Role: schema.System, Content: promptreg.RAGPlannerSystem},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("planner LLM call failed: %w", err)
	}

	var subQueries []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &subQueries); err != nil {
		return nil, fmt.Errorf("planner parse failed: %w", err)
	}

	// 去重 + 过滤空串
	seen := make(map[string]struct{})
	unique := make([]string, 0, len(subQueries))
	for _, q := range subQueries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		unique = append(unique, q)
	}

	if len(unique) < 2 {
		return nil, nil // 拆解结果不足，不使用
	}

	if len(unique) > cfg.MaxSubQueries {
		unique = unique[:cfg.MaxSubQueries]
	}

	return unique, nil
}

// Analyze 分析 query 是否需要拆解
func Analyze(ctx context.Context, query string, cfg PlannerConfig) PlanResult {
	start := time.Now()
	result := PlanResult{}

	if !cfg.Enabled {
		result.Reason = "planner_disabled"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if !needsDecomposition(query, cfg) {
		result.Reason = "no_decomposition_needed"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	subQueries, err := decomposeQuery(ctx, query, cfg)
	if err != nil {
		g.Log().Debugf(ctx, "query planner decompose failed: %v", err)
		result.Reason = fmt.Sprintf("decompose_error: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if len(subQueries) == 0 {
		result.Reason = "decompose_returned_empty"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.Decomposed = true
	result.SubQueries = subQueries
	result.Reason = "decomposed"
	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}
```

- [ ] **Step 3: 添加 Execute 和 Merge 方法**

```go
// Execute 并行执行子查询
func Execute(ctx context.Context, subQueries []string, cfg PlannerConfig) ([][]*schema.Document, int64) {
	start := time.Now()
	results := make([][]*schema.Document, len(subQueries))
	var wg sync.WaitGroup

	for i, q := range subQueries {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			queryCtx, cancel := context.WithTimeout(ctx, plannerExecTimeout(ctx, cfg))
			defer cancel()
			docs, _, err := Query(queryCtx, SharedPool(), query)
			if err != nil {
				g.Log().Debugf(ctx, "planner sub-query %d failed: %v", idx, err)
				return
			}
			results[idx] = docs
		}(i, q)
	}
	wg.Wait()

	return results, time.Since(start).Milliseconds()
}

// MergeResults 使用 RRF 融合子查询结果
func MergeResults(subQueryResults [][]*schema.Document, subQueries []string, finalTopK int) ([]*schema.Document, plannerSubQueryMap) {
	const k = 60.0

	type entry struct {
		doc        *schema.Document
		score      float64
		subQueries []string
	}
	byID := make(map[string]*entry)

	for i, docs := range subQueryResults {
		if i >= len(subQueries) {
			break
		}
		for rank, doc := range docs {
			if doc == nil {
				continue
			}
			id := docFusionKey(doc)
			if id == "" {
				id = doc.ID
			}
			if id == "" {
				continue
			}
			e, ok := byID[id]
			if !ok {
				e = &entry{doc: doc}
				byID[id] = e
			}
			e.score += 1.0 / (k + float64(rank+1))
			// 记录来源子查询
			found := false
			for _, sq := range e.subQueries {
				if sq == subQueries[i] {
					found = true
					break
				}
			}
			if !found {
				e.subQueries = append(e.subQueries, subQueries[i])
			}
		}
	}

	entries := make([]*entry, 0, len(byID))
	for _, e := range byID {
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	limit := finalTopK
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	docs := make([]*schema.Document, 0, limit)
	subQueryMap := make(plannerSubQueryMap)
	for _, e := range entries[:limit] {
		docs = append(docs, e.doc)
		id := docFusionKey(e.doc)
		if id == "" {
			id = e.doc.ID
		}
		subQueryMap[id] = e.subQueries
	}

	return docs, subQueryMap
}
```

- [ ] **Step 4: 添加 QueryWithPlanner 主入口**

```go
// QueryWithPlanner 带 Query Planner 的检索入口
func QueryWithPlanner(ctx context.Context, pool *RetrieverPool, query string, cfg PlannerConfig) ([]*schema.Document, MergedResult, error) {
	trace := PlannerTrace{Analyzed: true}

	if strings.TrimSpace(query) == "" {
		return nil, MergedResult{Trace: trace}, nil
	}

	plan := Analyze(ctx, query, cfg)
	trace.PlanLatencyMs = plan.LatencyMs

	if !plan.Decomposed {
		// 不拆解，走现有 pipeline
		docs, _, err := Query(ctx, pool, query)
		trace.FallbackReason = plan.Reason
		return docs, MergedResult{Trace: trace}, err
	}

	trace.SubQueryCount = len(plan.SubQueries)

	// 并行执行子查询
	subResults, execLatency := Execute(ctx, plan.SubQueries, cfg)
	trace.ExecLatencyMs = execLatency

	// 合并结果
	mergeStart := time.Now()
	docs, subQueryMap := MergeResults(subResults, plan.SubQueries, RetrieverTopK(ctx))
	trace.MergeLatencyMs = time.Since(mergeStart).Milliseconds()

	// 将 subQuery 信息写入 doc MetaData
	for _, doc := range docs {
		id := docFusionKey(doc)
		if id == "" {
			id = doc.ID
		}
		if sqs, ok := subQueryMap[id]; ok {
			if doc.MetaData == nil {
				doc.MetaData = make(map[string]any)
			}
			doc.MetaData["_planner_sub_queries"] = sqs
		}
	}

	return docs, MergedResult{Trace: trace}, nil
}
```

- [ ] **Step 5: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/rag/planner.go
git commit -m "feat(rag): 实现 QueryPlanner 核心逻辑 (Analyze + Execute + Merge)"
```

---

### Task 3: Prompt 注册

**Files:**
- Modify: `internal/ai/promptreg/` (找到 RAG 相关 prompt 文件)

- [ ] **Step 1: 添加 RAGPlannerSystem prompt**

在 `internal/ai/promptreg/` 中找到 prompt 注册文件，添加：

```go
const RAGPlannerSystem = `你是一个运维查询分析器。你的任务是将用户的运维问题拆解为多个独立的检索子查询，以便更精准地从知识库中检索相关信息。`
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/ai/rag/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ai/promptreg/
git commit -m "feat(rag): 新增 QueryPlanner system prompt"
```

---

### Task 4: 单元测试

**Files:**
- Create: `internal/ai/rag/planner_test.go`

- [ ] **Step 1: 编写 needsDecomposition 测试**

```go
package rag

import (
	"testing"
)

func TestNeedsDecomposition(t *testing.T) {
	cfg := DefaultPlannerConfig()

	tests := []struct {
		query    string
		expected bool
	}{
		{"Prometheus 告警先看什么", false},                          // 太短，单一主题
		{"payment 服务最近为什么延迟升高，跟 Redis 有没有关系？", true}, // 包含 "跟"、"为什么"
		{"MySQL 锁等待怎么定位", false},                              // 单一主题
		{"K8s 发布观察和 Helm 回滚的完整流程", true},                   // 包含 "和"
		{"Redis 分布式锁主从切换会丢锁吗", false},                      // 单一主题
		{"服务发布后怎么回滚", false},                                  // 太短
		{"告警规则里 for 字段是干嘛的", false},                         // 单一主题
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := needsDecomposition(tt.query, cfg)
			if result != tt.expected {
				t.Errorf("needsDecomposition(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestDefaultPlannerConfig(t *testing.T) {
	cfg := DefaultPlannerConfig()
	if cfg.Enabled {
		t.Error("default config should be disabled")
	}
	if cfg.MaxSubQueries != 4 {
		t.Errorf("MaxSubQueries = %d, want 4", cfg.MaxSubQueries)
	}
	if cfg.TimeoutMs != 200 {
		t.Errorf("TimeoutMs = %d, want 200", cfg.TimeoutMs)
	}
}
```

- [ ] **Step 2: 编写 MergeResults 测试**

```go
func TestMergeResults_DedupAndRRF(t *testing.T) {
	// 两个子查询，doc A 被两个都命中，doc B 只被一个命中
	docs1 := []*schema.Document{
		{ID: "doc-a", Content: "content A"},
		{ID: "doc-b", Content: "content B"},
	}
	docs2 := []*schema.Document{
		{ID: "doc-a", Content: "content A"},
		{ID: "doc-c", Content: "content C"},
	}

	subQueries := []string{"query 1", "query 2"}
	results, subQueryMap := MergeResults(
		[][]*schema.Document{docs1, docs2},
		subQueries,
		10,
	)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// doc-a 应该排第一（被两个子查询命中）
	if results[0].ID != "doc-a" {
		t.Errorf("first result should be doc-a, got %s", results[0].ID)
	}

	// doc-a 的 subQueries 应该包含两个
	sqs := subQueryMap["doc-a"]
	if len(sqs) != 2 {
		t.Errorf("doc-a should be in 2 sub-queries, got %d", len(sqs))
	}
}

func TestMergeResults_Empty(t *testing.T) {
	results, _ := MergeResults(
		[][]*schema.Document{nil, nil},
		[]string{"q1", "q2"},
		10,
	)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./internal/ai/rag/ -run "TestNeedsDecomposition|TestDefaultPlannerConfig|TestMergeResults" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/rag/planner_test.go
git commit -m "test(rag): 新增 QueryPlanner 单元测试"
```

---

### Task 5: Eval Metrics 扩展

**Files:**
- Modify: `internal/ai/rag/eval/online.go`

- [ ] **Step 1: 在 QueryMetrics 中新增字段**

在 `internal/ai/rag/eval/online.go` 的 `QueryMetrics` 结构体中添加：

```go
type QueryMetrics struct {
	// ... existing fields ...
	Decomposed     bool   `json:"decomposed"`
	SubQueryCount  int    `json:"sub_query_count"`
	PlanLatencyMs  int64  `json:"plan_latency_ms"`
	MergeLatencyMs int64  `json:"merge_latency_ms"`
}
```

- [ ] **Step 2: 在 printSummary 中输出新指标**

在 `printSummary` 函数中添加输出：

```go
fmt.Printf("  Decomposed    : %.2f%%\n", decompRate*100)
fmt.Printf("  Avg SubQueries: %.2f\n", avgSubQueries)
```

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/ai/rag/eval/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ai/rag/eval/online.go
git commit -m "feat(rag): 扩展 eval metrics 支持 planner 指标"
```

---

### Task 6: Eval Mode 集成

**Files:**
- Modify: `internal/ai/cmd/rag_online_eval_cmd/main.go`

- [ ] **Step 1: 在 parseEvalMode 中添加 planner case**

```go
case "planner":
    return false, false, false, true // 需要特殊处理
```

- [ ] **Step 2: 在 main 函数中添加 planner 执行路径**

在 `exec` 函数定义之后，添加 planner 特殊处理：

```go
if *modeRaw == "planner" {
    plannerCfg := rag.LoadPlannerConfig(ctx)
    plannerCfg.Enabled = true // eval 时强制启用
    exec = func(ctx context.Context, query string) ([]eval.RetrievedDoc, eval.QueryMetrics, error) {
        start := time.Now()
        queryCtx, cancel := context.WithTimeout(ctx, time.Duration(*perQueryTimeoutMs)*time.Millisecond)
        defer cancel()

        docs, merged, err := rag.QueryWithPlanner(queryCtx, rag.SharedPool(), query, plannerCfg)
        if err != nil {
            return nil, eval.QueryMetrics{}, err
        }

        metrics := eval.QueryMetrics{
            TotalLatencyMs: time.Since(start).Milliseconds(),
            ResultCount:    len(docs),
            Decomposed:     merged.Trace.Analyzed && merged.Trace.SubQueryCount > 0,
            SubQueryCount:  merged.Trace.SubQueryCount,
            PlanLatencyMs:  merged.Trace.PlanLatencyMs,
            MergeLatencyMs: merged.Trace.MergeLatencyMs,
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
git commit -m "feat(rag): eval 支持 planner mode"
```

---

### Task 7: 配置文件

**Files:**
- Modify: `manifest/config/config.yaml`

- [ ] **Step 1: 在 rag 块下新增 planner 配置**

在 `config.yaml` 的 `rag:` 部分添加：

```yaml
  planner:
    enabled: false
    model: "chat_model_fast"
    timeout_ms: 200
    max_sub_queries: 4
    min_query_length: 15
    exec_timeout_ms: 5000
    decomposition_keywords:
      - "和"
      - "以及"
      - "还有"
      - "跟"
      - "为什么"
      - "怎么回事"
      - "导致"
      - "关系"
```

- [ ] **Step 2: 验证配置格式**

Run: `go run ./internal/ai/cmd/chat_cmd/ 2>&1 | head -5` (验证 config 加载不报错)
Expected: 服务正常启动或因缺少依赖报错（但不报 config 解析错误）

- [ ] **Step 3: Commit**

```bash
git add manifest/config/config.yaml
git commit -m "config(rag): 新增 QueryPlanner 配置项"
```

---

### Task 8: 全量测试验证

- [ ] **Step 1: 运行 RAG 全量测试**

Run: `go test ./internal/ai/rag/... -v`
Expected: PASS

- [ ] **Step 2: 运行 ContextEngine 测试**

Run: `go test ./internal/ai/contextengine/... -v`
Expected: PASS

- [ ] **Step 3: 运行 go vet**

Run: `go vet ./internal/ai/rag/...`
Expected: PASS

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: 运行 in-memory eval 基线对比**

Run: `go run ./internal/ai/cmd/rag_eval_cmd/ -ks 1,3,5`
Expected: 与之前基线一致 (Recall@1=0.89, Recall@5=1.00)

---

### Task 9: 实验记录

**Files:**
- Modify: `docs/superpowers/experiments/2026-06-14-agent-rag-baseline.md`

- [ ] **Step 1: 追加实验结果**

在实验记录文档末尾追加：

```markdown
## 7. Plan A 实现后对比 (待运行)

### 本地 In-Memory Eval

| 指标 | Baseline | Planner | 变化 |
|------|---------|---------|------|
| Recall@1 | 0.89 | TBD | - |
| Recall@3 | 1.00 | TBD | - |
| Recall@5 | 1.00 | TBD | - |

### 远端 Milvus Eval (需要线上环境)

| 指标 | Baseline | Planner | 变化 |
|------|---------|---------|------|
| Hit@5 | 38.8% | TBD | - |
| AvgRecall@5 | 2.25% | TBD | - |
| Avg Total ms | 43.64 | TBD | - |
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/experiments/2026-06-14-agent-rag-baseline.md
git commit -m "docs(rag): 追加 Query Planner 实验记录模板"
```

---

## 实施顺序

1. Task 1: 配置加载
2. Task 2: 核心逻辑
3. Task 3: Prompt 注册
4. Task 4: 单元测试
5. Task 5: Eval 扩展
6. Task 6: Eval Mode 集成
7. Task 7: 配置文件
8. Task 8: 全量测试
9. Task 9: 实验记录

## 验证清单

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/ai/rag/...` 通过
- [ ] `go test ./internal/ai/contextengine/...` 通过
- [ ] `go vet ./internal/ai/rag/...` 通过
- [ ] 无 import 规则违规
- [ ] 配置项走 config.yaml，无硬编码
- [ ] commit message 用中文
