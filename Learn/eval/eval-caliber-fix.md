# RAG 评测口径修复与默认查询模式调整

## 1. 背景

telemetry baseline 跑完之后，`retrieve / rewrite / full` 三组结果几乎完全一致，但分数整体偏低。  
继续调 `rewrite/rerank` 没意义，先把两个基础问题修正：

1. 评测 query 是按症状生成的，但相关集按 `fault_type / fault_category` 定义
2. `Query()` 默认还是 `full`，线上会平白多走一层不稳定的 LLM 路径

## 2. 这次修了什么

### 2.1 评测口径

在 [aiops_baseline.go](D:/Agent/OpsCaption/internal/ai/rag/eval/aiops_baseline.go) 里并行生成三套 holdout 评测文件：

- `eval_cases_holdout_related.jsonl`
  - 旧口径
  - 相关集来自 `fault_type / fault_category`
- `eval_cases_holdout_symptom.jsonl`
  - 新口径
  - 相关集来自 `service / instance_type / source-destination / observation keywords`
- `eval_cases_holdout_combined.jsonl`
  - 并集口径
  - 相关集是 fault 口径和 symptom 口径的并集

这样后续评测时可以直接区分：

- 检索器找“同根因案例”的能力
- 检索器找“同症状案例”的能力

### 2.2 symptom 相关集规则

新增 `relatedBuildCaseIDsBySymptom(...)`，规则是：

- 同 `service`：`+3`
- 同 `instance_type`：`+1`
- 同 `source`：`+1`
- 同 `destination`：`+1`
- `observation keyword` 每重合一个：`+1`
- 仅在非精确 service 命中时，service 子串包含：`+1`

收口条件：

- 总分至少 `>= 3`
- 且必须满足以下至少一条：
  - keyword overlap > 0
  - `source/destination` 有命中
- 最终只保留分数最高的前 `20` 个 build case，避免热门服务把相关集撑得过大

同时去掉了把通用类型词 `metric / trace / log` 直接算进 keyword overlap 的做法，避免相关集被放得过宽。

### 2.3 默认查询模式

在 [query.go](D:/Agent/OpsCaption/internal/ai/rag/query.go) 和 [config.go](D:/Agent/OpsCaption/internal/ai/rag/config.go) 里做了调整：

- `Query()` 不再硬编码默认 `full`
- 改为读取 `DefaultQueryMode(ctx)`
- 如果配置里没有 `rag.default_query_mode`，默认走 `retrieve`
- [rag_online_eval_cmd/main.go](D:/Agent/OpsCaption/internal/ai/cmd/rag_online_eval_cmd/main.go) 的 `-mode` 默认值也改成了 `retrieve`

也就是说：

- 默认线上路径现在更稳定
- 如果后面要重新打开 `rewrite` 或 `full`，只需要改配置，不用改代码

## 3. 为什么这样改

### 3.1 先修评分标准

如果 query 是“症状驱动”，相关集却是“根因驱动”，那低分不一定说明检索器差，可能只是评分标准在惩罚产品真实目标。  
所以先把 fault/symptom/combined 三套口径并行保留下来，后续所有优化才有意义。

### 3.2 先关掉默认 full

前面的基线已经说明：

- `rewrite` 没带来增益
- `rerank` 没带来增益
- 但它们显著增加延迟和 429 风险

在召回层还没做好之前，默认 `full` 只会制造噪声。

## 4. 具体代码改动

### 4.1 [aiops_baseline.go](D:/Agent/OpsCaption/internal/ai/rag/eval/aiops_baseline.go)

- `AIOPSPrepSummary` 新增：
  - `holdout_symptom_eval_cases`
  - `holdout_combined_eval_cases`
- holdout 生成流程现在同时产出：
  - `fault`
  - `symptom`
  - `combined`
- 新增：
  - `unionIDs(...)`
  - `appendEvalNotes(...)`

### 4.2 [aiops_baseline_test.go](D:/Agent/OpsCaption/internal/ai/rag/eval/aiops_baseline_test.go)

补了两类测试：

- 端到端测试
  - 验证三套 JSONL 文件确实生成
  - 验证 `fault / symptom / combined` 的 `RelevantIDs` 分别正确
- 症状规则测试
  - 验证同 `service + keyword overlap` 会命中
  - 验证无关 build case 不会误入

### 4.3 [query.go](D:/Agent/OpsCaption/internal/ai/rag/query.go)

- `Query()` 默认模式改为 `DefaultQueryMode(ctx)`
- `queryWithMode(..., mode="")` 也走 `DefaultQueryMode(ctx)`

### 4.4 [config.go](D:/Agent/OpsCaption/internal/ai/rag/config.go)

新增 `DefaultQueryMode(ctx)`：

- 读取 `rag.default_query_mode`
- 没配置时默认 `retrieve`

## 5. 验证

执行：

```powershell
go test ./internal/ai/rag/eval ./internal/ai/rag
```

验证目标：

1. 新评测文件真实生成
2. 旧 `holdout_related` 仍然保留
3. symptom/combined 逻辑有测试保护
4. 默认查询模式已切到 `retrieve`

## 6. 结果和边界

### 已解决

- 新口径不再是死代码
- fault/symptom/combined 三套评测口径并行存在
- 默认查询模式不再硬编码 `full`

### 还没解决

1. 这次没有引入 hybrid retrieval
2. 这次没有做 telemetry-aware chunking
3. 这次没有做 Milvus schema/version 治理
4. symptom 规则当前还是启发式打分，不是学习得到的

## 7. 给评审员怎么讲

可以直接这样说：

> 我先修的不是召回算法，而是评测口径。因为原来 query 是按症状构造的，但相关集却按 fault_type 来定义，这会天然压低产品真实检索目标的分数。  
> 我把评测拆成 fault、symptom、combined 三套口径并行输出，再把默认查询模式从 full 改成 retrieve，先把基线收敛成稳定、可解释、可复现的状态。后面再做 hybrid retrieval，结论才可信。

## 8. 我应该学会什么

1. 评测口径错了，后面的优化几乎都会被带偏
2. 先把默认路径改成稳定版本，再谈高级策略
3. RAG 优化不能只看“分数低”，要先看“你到底在测什么”
