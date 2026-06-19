# Agent RAG Query Planner 设计文档

日期: 2026-06-14
状态: Draft
目标: 通过查询分解提升复合 query 的检索召回率

## 1. 背景与动机

### 1.1 当前瓶颈

基线数据 (Learn/rag/19) 显示：
- Hit@5 = 38.8%，AvgRecall@5 = 2.25%
- rewrite/rerank 对召回率零提升
- candidate_top_k 从 20→100 无差异

核心问题：**复合 query 被当作单次检索处理**，信息密度高但检索粒度粗。

### 1.2 复合 Query 示例

| 场景 | 原始 Query | 需要的子查询 |
|------|-----------|-------------|
| 延迟诊断 | "payment 服务最近为什么延迟升高" | payment 延迟指标 + payment 日志 + payment trace |
| 告警关联 | "Redis 告警和订单失败有关系吗" | Redis 状态 + 订单日志 + 链路关系 |
| 变更影响 | "这次发布后哪些服务受影响" | 变更内容 + 依赖关系 + 历史故障 |

### 1.3 目标

| 指标 | 当前 Baseline | Plan A 目标 |
|------|-------------|------------|
| Recall@1 (复合 query) | 0.50 | >= 0.75 |
| Recall@5 (全部) | 1.00 | >= 1.00 |
| 延迟开销 | baseline | +500ms |
| LLM token 消耗 | baseline | +2000 tokens |

## 2. 架构设计

### 2.1 整体流程

```
用户 Query
    │
    ▼
QueryPlanner.Analyze()
    │
    ├── 不需要拆解 → rag.Query() → Return
    │
    └── 需要拆解
         │
         ├── LLM 生成 SubQuery[0..N]
         │
         ├── 并行执行 rag.Query() × N
         │
         └── RRF Merge + Dedup + Citation → Return
```

### 2.2 模块位置

```
internal/ai/rag/
├── planner.go           # QueryPlanner 核心逻辑
├── planner_test.go      # 单元测试
├── planner_config.go    # 配置加载
└── ... (existing files)
```

遵循 AGENTS.md 规则：
- Domain 层 (internal/ai/) 不直接依赖 infra
- 所有检索仍通过 rag.Query() 接口
- Planner 不改检索内核

### 2.3 核心类型

```go
// PlanResult 分析结果
type PlanResult struct {
    Decomposed    bool     // 是否需要拆解
    SubQueries    []string // 子查询列表
    Reason        string   // 拆解原因 (用于 trace)
    LatencyMs     int64    // 分析耗时
}

// PlannerConfig 配置
type PlannerConfig struct {
    Enabled             bool
    Model               string
    TimeoutMs           int
    MaxSubQueries       int
    MinQueryLength      int
    DecompositionKeywords []string
}

// MergedResult 合并后的结果
type MergedResult struct {
    Docs        []*schema.Document
    SubQueryMap map[string][]string // doc_id → 命中该 doc 的子查询
    Trace       PlannerTrace
}

// PlannerTrace 追踪信息
type PlannerTrace struct {
    Analyzed       bool
    SubQueryCount  int
    PlanLatencyMs  int64
    ExecLatencyMs  int64
    MergeLatencyMs int64
    FallbackReason string
}
```

## 3. 查询分析

### 3.1 两阶段分析

**阶段 1: 规则检测** (<1ms)

```go
func needsDecomposition(query string, cfg PlannerConfig) bool {
    if len([]rune(query)) < cfg.MinQueryLength {
        return false
    }
    // 检查连接词
    for _, kw := range cfg.DecompositionKeywords {
        if strings.Contains(query, kw) {
            return true
        }
    }
    return false
}
```

**阶段 2: LLM 拆解** (<200ms)

使用 `chat_model_fast`，prompt 见设计段 2。

### 3.2 Prompt 设计

```
System:
你是运维查询分析器。将用户问题拆解为独立的检索子查询。

规则：
1. 每个子查询聚焦一个具体信息点
2. 最多 {max_sub_queries} 个子查询
3. 保留原始 query 的语义关键词
4. 如果问题已经是单一主题，返回空数组 []
5. 只输出 JSON 数组，不要其他文字

User:
{query}

Output:
["sub_query_1", "sub_query_2", ...]
```

### 3.3 不拆解的情况

- 单一事实查询："Prometheus 告警先看什么"
- 闲聊类
- 拆解后子查询数 < 2
- LLM 超时或返回异常

## 4. 并行执行

### 4.1 执行策略

```go
func (p *QueryPlanner) Execute(ctx context.Context, subQueries []string) [][]*schema.Document {
    results := make([][]*schema.Document, len(subQueries))
    var wg sync.WaitGroup
    for i, q := range subQueries {
        wg.Add(1)
        go func(idx int, query string) {
            defer wg.Done()
            queryCtx, cancel := context.WithTimeout(ctx, p.execTimeout)
            defer cancel()
            docs, _, _ := rag.Query(queryCtx, rag.SharedPool(), query)
            results[idx] = docs
        }(i, q)
    }
    wg.Wait()
    return results
}
```

### 4.2 超时控制

- 单个子查询超时: `context.docs_query_timeout_ms` (默认 5s)
- 总执行超时: 取所有子查询中的最大超时
- 任一子查询超时不阻塞其他子查询

## 5. 结果合并

### 5.1 RRF 融合

```go
func MergeResults(subQueryResults [][]*schema.Document, subQueries []string, finalTopK int) MergedResult {
    const k = 60.0 // 与 hybrid.go 一致

    type entry struct {
        doc       *schema.Document
        score     float64
        subQueries []string
    }
    byID := make(map[string]*entry)

    for i, docs := range subQueryResults {
        for rank, doc := range docs {
            id := docFusionKey(doc)
            if id == "" { id = doc.ID }
            e, ok := byID[id]
            if !ok {
                e = &entry{doc: doc}
                byID[id] = e
            }
            e.score += 1.0 / (k + float64(rank+1))
            e.subQueries = append(e.subQueries, subQueries[i])
        }
    }

    // 排序 + 裁剪
    results := make([]*schema.Document, 0, finalTopK)
    for _, e := range sorted(byID) {
        results = append(results, e.doc)
        // 在 MetaData 中记录来源子查询
        e.doc.MetaData["_planner_sub_queries"] = e.subQueries
    }
    return results[:finalTopK]
}
```

### 5.2 去重策略

- 按 `docFusionKey()` 去重（与 hybrid.go 一致）
- 同一 doc 被多个子查询命中 → RRF 分数叠加 → 排名提升
- 这天然奖励"被多个角度命中"的文档

### 5.3 Citation 增强

在现有 `Citation` 结构中新增：

```go
type Citation struct {
    // ... existing fields ...
    SubQueries []string `json:"sub_queries,omitempty"` // 命中该 doc 的子查询
}
```

## 6. 配置

### 6.1 config.yaml

```yaml
rag:
  planner:
    enabled: true
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

### 6.2 配置加载

```go
func LoadPlannerConfig(ctx context.Context) PlannerConfig {
    cfg := PlannerConfig{
        Enabled:           false,
        Model:             "chat_model_fast",
        TimeoutMs:         200,
        MaxSubQueries:     4,
        MinQueryLength:    15,
        ExecTimeoutMs:     5000,
        DecompositionKeywords: []string{"和", "以及", "还有", "跟"},
    }
    // 从 g.Cfg() 加载覆盖...
    return cfg
}
```

## 7. 降级策略

| 故障场景 | 降级行为 |
|---------|---------|
| Planner 未配置/禁用 | 跳过，走现有 rag.Query() |
| 规则检测不需要拆解 | 走现有 rag.Query() |
| LLM 拆解超时 | 回退到原始 query 单次检索 |
| LLM 拆解返回空/异常 | 回退到原始 query |
| 某个子查询失败 | 用其余子查询结果合并 |
| 全部子查询失败 | 回退到原始 query |
| 合并后结果为空 | 回退到原始 query |

**关键原则**：任何故障都不影响现有检索能力，只降级到"没有 Planner"的状态。

## 8. Eval 集成

### 8.1 新增 eval mode

在 `rag_online_eval_cmd/main.go` 中新增 `planner` mode:

```go
case "planner":
    // 使用 QueryPlanner.Execute() 替代 rag.Query()
    exec = plannerExec(ctx, plannerCfg)
```

### 8.2 指标扩展

```go
type QueryMetrics struct {
    // ... existing fields ...
    Decomposed     bool   `json:"decomposed"`
    SubQueryCount  int    `json:"sub_query_count"`
    PlanLatencyMs  int64  `json:"plan_latency_ms"`
    MergeLatencyMs int64  `json:"merge_latency_ms"`
}
```

### 8.3 实验脚本

```bash
#!/bin/bash
# Agent RAG Query Planner 实验

# 1. 基线
go run ./internal/ai/cmd/rag_online_eval_cmd/ \
  -mode hybrid -ks 1,3,5 \
  -out results/baseline.json

# 2. Query Planner
go run ./internal/ai/cmd/rag_online_eval_cmd/ \
  -mode planner -ks 1,3,5 \
  -out results/planner.json

# 3. 对比
jq '{mrr:.summary.mrr, recall1:.summary.avg_recall_at_k["1"], recall5:.summary.avg_recall_at_k["5"]}' results/baseline.json results/planner.json
```

## 9. 文件清单

| 文件 | 职责 | 行数估算 |
|------|------|---------|
| `rag/planner.go` | QueryPlanner 核心 (Analyze + Execute + Merge) | ~200 |
| `rag/planner_test.go` | 单元测试 | ~150 |
| `rag/planner_config.go` | 配置加载 | ~60 |
| `rag/eval/online.go` | 扩展 eval metrics | ~20 (改动) |
| `cmd/rag_online_eval_cmd/main.go` | 新增 planner mode | ~30 (改动) |

## 10. 风险与边界

1. **不是 Multi-Hop RAG** — 子查询之间无依赖，是并行独立检索
2. **不是 Adative Retrieval** — 无检索质量评估和重试
3. **延迟敏感** — LLM 拆解 + 并行检索，总延迟需控制在 +500ms 内
4. **Token 成本** — 每次拆解消耗 ~500 tokens，需评估 ROI
5. **Eval 一致性** — 必须在同一 collection、同一 eval cases 上对比

## 11. 实施顺序

1. 实现 `planner_config.go` (配置加载)
2. 实现 `planner.go` (核心逻辑)
3. 实现 `planner_test.go` (单元测试)
4. 扩展 `eval/online.go` (metrics)
5. 扩展 `rag_online_eval_cmd` (planner mode)
6. 运行 baseline vs planner 对比实验
7. 记录实验结果到 `docs/superpowers/experiments/`
