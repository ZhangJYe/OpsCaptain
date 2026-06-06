# Agent Eval 系统设计

> 日期：2026-06-06
> 方案：三阶段渐进式（Go 确定性 gate → Go LLM Judge → deepeval）

---

## 一、背景

项目已有完整的 Go 原生评估体系（RAG 召回率/MRR、Agent 准确率/证据覆盖率、Gate 回归检查），但缺少：
- LLM-as-Judge 多维度评分（`DiagScores` 类型已定义，runner 未实现）
- 端到端答案质量评估（Faithfulness、Answer Relevancy）
- 评估结果持久化和趋势追踪

## 二、核心设计原则

1. **CI 只跑确定性 gate**，不依赖 LLM API（无 secrets）
2. **eval_cases 和 eval_runs 分离**：cases 是输入+期望，runs 是实际执行结果
3. **GoS/RAG 各自保留原有格式**，不硬合一个 schema
4. **三阶段渐进**：每阶段独立可用，不依赖后续阶段

## 三、Artifact 体系

### eval_cases（输入 + 期望）

评估用例，描述"应该怎样"。由人工编写或从现有数据迁移。

```
evals/cases/
├── gos_holdout.json          # GoS 格式：{symptom, ground_truth, expected_keywords}
├── rag_cases.jsonl           # RAG 格式：{query, relevant_ids}
└── diag_cases.jsonl          # 通用格式：{query, expected_intent, must_mention, severity}
```

三个文件各自保持原有 schema，不做统一。

### eval_runs（实际执行结果）

Agent/RAG 的真实运行输出。由运行命令生成。

```
evals/runs/
├── gos_{timestamp}.json      # GoS 运行结果：{case_id, prediction, matched, latency, ...}
├── rag_{timestamp}.jsonl     # RAG 运行结果：{query, retrieved_ids, recall, mrr, ...}
└── diag_{timestamp}.jsonl    # Agent 运行结果：{query, actual_output, tools_called, ...}
```

### eval_reports（评估报告）

评分和指标汇总。

```
evals/reports/
├── gate_{timestamp}.json     # 确定性 gate 结果（pass/fail）
├── judge_{timestamp}.json    # LLM Judge 评分结果
└── deepeval_{timestamp}.json # deepeval 评估结果
```

## 四、第一阶段：Go 确定性 Gate（CI 自动）

### 目标

CI 跑确定性回归检查，不依赖 LLM API，PR 阻断。

### 改动

1. `cmd/gos_eval` 新增 `--mode=export-runs`：运行 GoS/RAG，输出 `evals/runs/` artifact
2. `cmd/gos_eval` 新增 `--mode=gate`：从 `evals/runs/` 读取结果，跑 CheckGate
3. Makefile 新增：

```makefile
# CI 自动（确定性，无 LLM 依赖）
eval-gate:
	go run cmd/gos_eval/main.go --mode=compare --gos-profile=eval

# 生成 run artifact（需要 LLM API，手动）
eval-runs:
	go run cmd/gos_eval/main.go --mode=export-runs --gos-profile=eval \
	  --output=evals/runs/
```

4. CI 配置（`.github/workflows/ci.yml`）新增：

```yaml
- name: Eval Gate
  run: make eval-gate
```

### 不改

- 现有 GoS eval 框架不变
- 现有 RAG eval 框架不变
- 不引入 LLM Judge

## 五、第二阶段：Go LLM Judge（手动/Nightly）

### 目标

对 Agent 输出做多维度质量评分，手动或 nightly 运行。

### 改动

1. `internal/ai/agent/eval/judge_runner.go`：实现 JudgeRunner

```go
// 复用现有 DiagScores 定义（types.go），不改字段
type JudgeRunner struct {
    model   model.ChatModel
    timeout time.Duration
}

func (j *JudgeRunner) Score(ctx, query, answer string, toolData []string) (*DiagScores, error)
```

2. 评分 Prompt（中文，4 维度，与现有 DiagScores 对齐）：
   - Correctness（正确性）：诊断结论是否基于工具数据
   - Completeness（完整性）：是否覆盖关键发现
   - Coherence（连贯性）：逻辑是否清晰
   - Actionability（可操作性）：是否给出可执行建议

3. `cmd/gos_eval` 新增 `--mode=judge`：从 `evals/runs/` 读取结果，调用 JudgeRunner 评分，输出 `evals/reports/judge_*.json`

4. Makefile 新增：

```makefile
# LLM Judge（需要 API key，手动/nightly）
eval-judge:
	go run cmd/gos_eval/main.go --mode=judge \
	  --input=evals/runs/ \
	  --output=evals/reports/
```

### 不改

- CI 不跑 judge（需要 LLM secrets）
- 现有 DiagScores 类型不变（int + Comments，不改为 float64）

## 六、第三阶段：deepeval 离线评估（手动）

### 目标

用 deepeval 的 Faithfulness/Answer Relevancy 做深度离线评估。

### 前置条件

- 第二阶段的 `eval_runs` artifact 已存在
- Python 环境已配置

### 改动

```
evals/deepeval/
├── requirements.txt
├── runner.py
├── metrics/
│   └── ops_metrics.py
└── datasets/
    └── .gitkeep              # 由 eval-runs 生成
```

### runner.py 输入格式

读取 `evals/runs/diag_*.jsonl`，不直接读 cases：

```python
# 输入：eval_runs（Agent 实际输出）
{"case_id": "DIAG-001", "query": "...", "actual_output": "...", "tools_called": [...]}

# 转换为 deepeval test case
LLMTestCase(
    input=run["query"],
    actual_output=run["actual_output"],
    retrieval_context=run["tools_called"]  # 工具数据作为检索上下文
)
```

### deepeval 环境配置

```bash
export OPENAI_API_KEY="${ARK_API_KEY}"
export OPENAI_MODEL_NAME="deepseek-v4"
# 如需自定义 base URL，通过 LiteLLM 或 deepeval 自定义模型适配
```

### Makefile 新增

```makefile
# 导出 runs 给 deepeval 用
eval-export-deep:
	go run cmd/gos_eval/main.go --mode=export-runs \
	  --output=evals/deepeval/datasets/

# deepeval 深度评估（需要 Python + API key）
eval-deep:
	cd evals/deepeval && python runner.py
```

## 七、DiagScores 兼容

严格复用现有 `agent/eval/types.go` 定义，不改字段：

```go
// 现有定义（types.go:21-28），不改
type DiagScores struct {
    Correctness   int    `json:"correctness"`
    Completeness  int    `json:"completeness"`
    Coherence     int    `json:"coherence"`
    Actionability int    `json:"actionability"`
    Overall       int    `json:"overall"`
    Comments      string `json:"comments,omitempty"`
}
```

JudgeRunner 输出对齐此格式（int 1-5 分）。

## 八、Makefile 汇总

```makefile
# 第一阶段：CI 自动（确定性）
eval-gate:
	go run cmd/gos_eval/main.go --mode=compare --gos-profile=eval

# 第一阶段：生成 run artifact（需要 LLM，手动）
eval-runs:
	go run cmd/gos_eval/main.go --mode=export-runs --gos-profile=eval \
	  --output=evals/runs/

# 第二阶段：LLM Judge（需要 API key，手动/nightly）
eval-judge:
	go run cmd/gos_eval/main.go --mode=judge \
	  --input=evals/runs/ --output=evals/reports/

# 第三阶段：deepeval（需要 Python + API key）
eval-export-deep:
	go run cmd/gos_eval/main.go --mode=export-runs \
	  --output=evals/deepeval/datasets/

eval-deep:
	cd evals/deepeval && python runner.py
```

## 九、改动文件清单

### 第一阶段
| 文件 | 改动 |
|------|------|
| `cmd/gos_eval/main.go` | 新增 `--mode=export-runs` 和 `--mode=gate` |
| `Makefile` | 新增 eval-gate / eval-runs |
| `.github/workflows/ci.yml` | 新增 eval-gate 步骤 |

### 第二阶段
| 文件 | 改动 |
|------|------|
| `internal/ai/agent/eval/judge_runner.go` | 新文件，Go LLM-as-Judge |
| `internal/ai/agent/eval/judge_runner_test.go` | 新文件 |
| `cmd/gos_eval/main.go` | 新增 `--mode=judge` |
| `Makefile` | 新增 eval-judge |

### 第三阶段
| 文件 | 改动 |
|------|------|
| `evals/deepeval/requirements.txt` | 新文件 |
| `evals/deepeval/runner.py` | 新文件 |
| `evals/deepeval/metrics/ops_metrics.py` | 新文件 |
| `Makefile` | 新增 eval-export-deep / eval-deep |

## 十、不改的部分

- 现有 GoS eval 框架（types.go, runner.go）不变
- 现有 RAG eval 框架不变
- 现有 DiagScores 类型定义不变
- 现有 LLM Validator / HealthCollector / Contract 不变
- 不引入 deepeval 到 Go 依赖
- CI 不引入 LLM API 依赖
