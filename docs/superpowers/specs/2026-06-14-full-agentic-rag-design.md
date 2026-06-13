# Full Agentic RAG 设计文档

日期: 2026-06-14
状态: Draft
目标: 在 Plan A (Query Planner) 基础上，增加检索质量评估 + 多轮自适应检索

## 1. 背景

Plan A 已实现并上线：
- QueryPlanner: 查询分解 + 并行检索 + RRF 合并
- 线上效果: Recall@1 0.36→0.78 (+117%), 延迟 149ms→706ms
- 拆解率: 2/18 (11.1%)

**局限**：
- 只做一次检索，不评估结果质量
- 拆解率低（仅关键词触发）
- 无法处理"检索结果不够好"的情况

## 2. 架构设计

### 2.1 整体流程

```
用户 Query
    │
    ▼
Round 1: QueryPlanner (Plan A)
    │
    ▼
Evaluator: 结果够不够？
    │
    ├── 够了 → Synthesize → 输出
    │
    └── 不够 → RetrieverAgent 规划下一轮
                │
                ▼
          Round 2: Retrieve (新 query/新策略)
                │
                ▼
          回到 Evaluator (最多 max_rounds 轮)
```

### 2.2 模块位置

```
internal/ai/rag/
├── agent_rag.go           # AgentRAG 主入口
├── agent_rag_test.go      # 单元测试
├── agent_config.go        # Agent 配置
├── evaluator.go           # 检索质量评估器
├── retrieval_planner.go   # 多轮检索规划器
├── planner.go             # Plan A (已有)
├── planner_config.go      # Plan A 配置 (已有)
└── ...
```

### 2.3 核心类型

```go
// AgentRAG 主入口
// 复用现有包级函数：QueryWithPlanner()、Execute()、MergeResults()
// 不引入额外 wrapper，仅通过 AgentConfig 控制行为
type AgentRAG struct {
    evaluator *Evaluator
    planner2  *RetrievalPlanner
    cfg       AgentConfig
}

// QueryWithPlannerFunc 用于测试注入，默认指向 rag.QueryWithPlanner
type QueryWithPlannerFunc func(ctx context.Context, pool *RetrieverPool, query string, cfg PlannerConfig) ([]*schema.Document, MergedResult, error)

// AgentConfig 配置
type AgentConfig struct {
    Enabled              bool
    Model                string
    MaxRounds            int
    ConfidenceThreshold  float64
    EvalTimeoutMs        int
    PlanTimeoutMs        int
    TotalTimeoutMs       int  // 外层总超时，防止多轮累积超时
    MaxTotalTokens       int
}

// EvalResult 评估结果
type EvalResult struct {
    Confidence   float64
    Sufficient   bool
    MissingInfo  []string
    NextStrategy string
    Reason       string
}

// RetrievalPlan 检索规划
type RetrievalPlan struct {
    SubQueries []string
    Strategy   string
    Reason     string
}

// AgentTrace 追踪
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
    Confidence    float64
    Strategy      string
    LatencyMs     int64
}
```

## 3. 评估器 (Evaluator)

### 3.1 职责

判断检索结果是否足够回答用户问题。

### 3.2 评估维度

1. 覆盖度: 是否覆盖了问题的所有方面
2. 相关性: 结果与问题的相关程度
3. 证据强度: 是否有明确证据支持结论

### 3.3 LLM 评估

```
System: 你是运维知识检索质量评估器。

User: 给定用户问题和检索结果，判断是否足够回答。

用户问题: {query}
已检索文档: {docs}
已尝试轮数: {round}/{max_rounds}

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
}
```

### 3.4 停止条件

满足任一则停止：
- `confidence >= threshold` (默认 0.7)
- `sufficient == true`
- 达到 `max_rounds`

## 4. 检索规划器 (RetrievalPlanner)

### 4.1 职责

根据评估结果，规划下一轮检索策略。

### 4.2 四种策略

| 策略 | 触发 | 行为 |
|------|------|------|
| `expand_scope` | 缺少多个方面 | 增加子查询覆盖缺失点 |
| `refine_query` | 相关性低 | 换关键词重新检索 |
| `add_angle` | 缺少特定角度 | 添加补充检索角度 |
| `none` | 已足够 | 停止 |

### 4.3 LLM 规划

```
System: 你是运维检索策略规划器。

User: 根据评估结果，规划下一轮检索。

用户问题: {query}
评估结果: {eval_result}
已检索文档 ID: {retrieved_ids}

规则:
1. 针对 missing_info 制定检索子查询
2. 最多 3 个子查询
3. 不要重复已检索的文档 ID（程序会自动过滤，但请尽量避免）

输出 JSON:
{
  "sub_queries": ["子查询1", "子查询2"],
  "strategy": "expand_scope|refine_query|add_angle",
  "reason": "规划理由"
}
```

## 5. AgentRAG 主流程

```go
func (a *AgentRAG) Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, AgentTrace, error) {
    trace := AgentTrace{}
    allDocs := make([]*schema.Document, 0)
    seenDocIDs := make(map[string]struct{}) // [P2] 已见文档 ID 集合
    currentQuery := query
    plannerCfg := LoadPlannerConfig(ctx)

    for round := 0; round < a.cfg.MaxRounds; round++ {
        // [P2] 检查外层总超时
        if ctx.Err() != nil {
            break
        }

        roundTrace := RoundTrace{Round: round + 1}

        // 1. 检索 — 复用现有 QueryWithPlanner 包级函数
        docs, merged, err := QueryWithPlanner(ctx, pool, currentQuery, plannerCfg)
        if err != nil {
            break
        }

        // [P2] 按 canonical ID 过滤已见文档，只保留新增
        newDocs := filterNewDocs(docs, seenDocIDs)
        roundTrace.DocCount = len(newDocs)
        roundTrace.SubQueryCount = merged.Trace.SubQueryCount

        // [P2] 新增文档为 0 → 停止（避免无效轮次）
        if len(newDocs) == 0 && round > 0 {
            break
        }

        allDocs = append(allDocs, newDocs...)

        // 2. 去重合并到累计候选集
        candidateDocs := mergeAndDedup(allDocs)

        // 3. 评估累计候选集（不是仅当前轮 docs）
        evalStart := time.Now()
        evalResult := a.evaluator.Evaluate(ctx, query, candidateDocs, round+1, a.cfg.MaxRounds)
        roundTrace.Confidence = evalResult.Confidence
        roundTrace.LatencyMs = time.Since(evalStart).Milliseconds()

        trace.RoundTraces = append(trace.RoundTraces, roundTrace)
        trace.Rounds = round + 1
        trace.FinalConfidence = evalResult.Confidence

        // 4. 停止条件
        if evalResult.Sufficient || evalResult.Confidence >= a.cfg.ConfidenceThreshold {
            break
        }

        // 5. 规划下一轮
        if round < a.cfg.MaxRounds-1 {
            plan := a.planner2.Plan(ctx, query, evalResult, candidateDocs)
            if plan.Strategy == "none" || len(plan.SubQueries) == 0 {
                break
            }
            currentQuery = strings.Join(plan.SubQueries, " ")
        }
    }

    // 最终去重排序
    finalDocs := mergeAndDedup(allDocs)
    return finalDocs, trace, nil
}

// filterNewDocs 过滤已见文档，返回新增文档
// 使用 canonical ID（复用 eval/online.go 的逻辑）
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

// canonicalDocID 提取文档的规范 ID（与 eval/online.go CanonicalSchemaDocID 一致）
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
```

## 6. 配置

```yaml
rag:
  agent:
    enabled: true
    model: "chat_model_fast"
    max_rounds: 3
    confidence_threshold: 0.7
    eval_timeout_ms: 3000
    plan_timeout_ms: 2000
    total_timeout_ms: 30000
    max_total_tokens: 8000
```

## 7. 降级策略

| 故障 | 行为 |
|------|------|
| Agent 未启用 | 走 Plan A (QueryPlanner) |
| 评估 LLM 超时 | 用当前结果直接输出 |
| 规划 LLM 超时 | 不再重试 |
| 达到 max_rounds | 用当前最佳结果 |
| 全部失败 | 回退到 Plan A |

## 8. Eval 集成

新增 eval mode `agent`：

```bash
go run ./internal/ai/cmd/rag_online_eval_cmd/ -mode agent -ks 1,3,5 -timeout-ms 60000
```

注意：agent 模式需要更大的 per-query timeout（默认 60s），因为多轮检索 + LLM 评估/规划的总延迟可能超过 15s。eval 命令的 `-timeout-ms` 参数应传入 agent 的 `total_timeout_ms` + 余量。

新增指标：
```go
type QueryMetrics struct {
    // ... existing ...
    AgentRounds     int     `json:"agent_rounds"`
    FinalConfidence float64 `json:"final_confidence"`
    AgentLatencyMs  int64   `json:"agent_latency_ms"`
}
```

## 9. 延迟与成本

| 组件 | 超时 | Token |
|------|------|-------|
| 评估 LLM | 3s | ~1500/轮 |
| 规划 LLM | 2s | ~1000/轮 |
| 单轮检索 | 5s | - |
| 外层总超时 | 30s | - |
| 3 轮理论最大 | ~25s | ~7500 |

## 10. 文件清单

| 文件 | 职责 | 行数估算 |
|------|------|---------|
| `rag/agent_rag.go` | AgentRAG 主入口 | ~150 |
| `rag/agent_config.go` | 配置加载 | ~60 |
| `rag/evaluator.go` | 检索质量评估 | ~120 |
| `rag/retrieval_planner.go` | 多轮检索规划 | ~100 |
| `rag/agent_rag_test.go` | 单元测试 | ~150 |
| `rag/eval/online.go` | 扩展 metrics | ~20 (改动) |
| `cmd/rag_online_eval_cmd/main.go` | 新增 agent mode | ~30 (改动) |
| `manifest/config/config.yaml` | 新增 agent 配置块 | ~15 (改动) |
| `deploy/config.prod.yaml` | 新增 agent 配置块 | ~15 (改动) |
