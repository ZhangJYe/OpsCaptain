# Agent Eval 系统设计

> 日期：2026-06-06
> 方案：混合方案（Go 原生 CI + Python deepeval 离线）

---

## 一、背景

项目已有完整的 Go 原生评估体系（RAG 召回率/MRR、Agent 准确率/证据覆盖率、Gate 回归检查、LLM 遗漏/准确性验证），但缺少：
- LLM-as-Judge 多维度评分器（`DiagScores` 类型已定义，runner 未实现）
- 语义相似度评估（当前只有关键词匹配）
- 端到端答案质量评估（Faithfulness、Answer Relevancy）
- CI 自动化集成

## 二、技术选型

### 方案对比

| 维度 | A: Go 原生 | B: deepeval | C: 混合（选择） |
|------|-----------|------------|----------------|
| 额外依赖 | 无 | Python + pip + deepeval | Python 仅离线用 |
| 维护成本 | 低 | 高（双语言栈） | 中 |
| 指标丰富度 | 中（自实现） | 高（开箱即用） | 高 |
| CI 速度 | 快（秒级） | 慢（分钟级） | 快（Go 部分） |
| 架构侵入 | 无 | 需桥接层 | 最小 |
| 工作量 | 3-5 天 | 5-7 天 | 5-7 天 |

### 选择方案 C：混合方案

- **Go 原生**：日常 CI 回归（快速、轻量、单语言）
- **deepeval**：深度离线评估（指标丰富、社区生态）

## 三、整体架构

```
eval_cases.jsonl（共享数据集）
├── GoS holdout cases (5)
├── RAG cases (18)
└── Agent DiagCases (新增)
        │
        ├──→ Go JudgeRunner (CI, 秒级)
        │    ├── Correctness (1-5)
        │    ├── Completeness (1-5)
        │    ├── Coherence (1-5)
        │    ├── Actionability (1-5)
        │    └── Overall (1-5)
        │
        └──→ Python deepeval (离线, 分钟级)
             ├── Faithfulness
             ├── Answer Relevancy
             └── Contextual Precision/Recall
```

## 四、Go 原生 JudgeRunner

### 接口

```go
// 复用 agent/eval/types.go 已有定义
type DiagScores struct {
    Correctness   float64 `json:"correctness"`    // 1-5
    Completeness  float64 `json:"completeness"`   // 1-5
    Coherence     float64 `json:"coherence"`      // 1-5
    Actionability float64 `json:"actionability"`  // 1-5
    Overall       float64 `json:"overall"`        // 1-5
    Reasoning     string  `json:"reasoning"`
}

type JudgeRunner struct {
    model   model.ChatModel
    timeout time.Duration
}

func NewJudgeRunner() *JudgeRunner
func (j *JudgeRunner) Score(ctx, query, answer string, toolData []string) (*DiagScores, error)
func (j *JudgeRunner) ScoreBatch(ctx, cases []DiagCase) ([]*DiagScores, error)
```

### 评分 Prompt

中文，5 维度，输出 JSON。复用 `models.OpenAIForGLMFast` 模型。

### 集成点

`cmd/gos_eval` 新增 `--mode=judge` 模式：
1. 加载 eval cases
2. 运行 Agent 获取输出
3. 调用 JudgeRunner 评分
4. 输出评分报告（JSON）

## 五、deepeval 离线评估

### 目录结构

```
evals/deepeval/
├── requirements.txt          # deepeval + openai
├── runner.py                 # 主入口
├── metrics/
│   ├── __init__.py
│   └── ops_metrics.py        # 自定义运维指标
├── datasets/
│   └── shared_cases.jsonl    # 共享数据集
└── reports/
    └── .gitkeep
```

### 核心指标

- **FaithfulnessMetric**：答案是否基于检索到的上下文（防幻觉）
- **AnswerRelevancyMetric**：答案是否回答了用户问题
- **ContextualPrecision/Recall**：检索上下文的质量

### runner.py 用法

```python
from deepeval import evaluate
from deepeval.metrics import FaithfulnessMetric, AnswerRelevancyMetric
from deepeval.test_case import LLMTestCase

cases = load_jsonl("datasets/shared_cases.jsonl")
test_cases = [
    LLMTestCase(
        input=case["query"],
        actual_output=case["answer"],
        retrieval_context=case["tool_data"]
    )
    for case in cases
]

evaluate(
    test_cases,
    metrics=[
        FaithfulnessMetric(threshold=0.7),
        AnswerRelevancyMetric(threshold=0.7),
    ]
)
```

### LLM 配置

deepeval 通过环境变量配置 LLM：
```bash
export OPENAI_API_KEY="${ARK_API_KEY}"
export OPENAI_API_BASE="https://ark.cn-beijing.volces.com/api/v3"
export OPENAI_MODEL="deepseek-v4"
```

## 六、共享数据集格式

### shared_cases.jsonl

```json
{
  "id": "DIAG-001",
  "query": "payment-service 响应延迟升高，排查一下",
  "answer": "诊断结果全文...",
  "tool_data": [
    "prometheus: p99=500ms, error_rate=5%",
    "log: connection timeout to mysql-001"
  ],
  "expected_keywords": ["超时", "数据库", "连接池"],
  "expected_intent": "diagnose",
  "severity": "high",
  "source": "gos_holdout"
}
```

### 导出

`cmd/gos_eval --mode=export` 从现有 eval cases 生成 `shared_cases.jsonl`。

## 七、CI 集成

### Makefile

```makefile
# Go 原生评估（CI 自动，~30s）
eval-go:
	go run cmd/gos_eval/main.go --mode=compare --gos-profile=eval
	go run cmd/gos_eval/main.go --mode=judge

# deepeval 深度评估（手动触发，~5min）
eval-deep:
	cd evals/deepeval && python runner.py

# 导出共享数据集
eval-export:
	go run cmd/gos_eval/main.go --mode=export \
	  --output=evals/deepeval/datasets/shared_cases.jsonl
```

### CI 流程

```
PR 提交 → make eval-go（Go 快速回归）
        → 失败则阻断合并
        → 通过则合并

定期手动 → make eval-export && make eval-deep
        → 生成深度评估报告
```

## 八、改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/ai/agent/eval/judge_runner.go` | 新文件，Go LLM-as-Judge 评分器 |
| `internal/ai/agent/eval/judge_runner_test.go` | 新文件，测试 |
| `cmd/gos_eval/main.go` | 新增 `--mode=judge` 和 `--mode=export` |
| `evals/deepeval/requirements.txt` | 新文件 |
| `evals/deepeval/runner.py` | 新文件 |
| `evals/deepeval/metrics/ops_metrics.py` | 新文件 |
| `evals/deepeval/datasets/shared_cases.jsonl` | 新文件（由 export 生成） |
| `Makefile` | 新增 eval-go / eval-deep / eval-export |

## 九、不改的部分

- 现有 RAG eval 框架不变
- 现有 GoS eval 框架不变
- 现有 LLM Validator（events/llm_validator.go）不变
- 现有 HealthCollector 不变
- 现有 Contract/SchemaGate 不变
- 不引入 deepeval 到 Go 依赖（Python 独立运行）
