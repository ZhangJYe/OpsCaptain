# aiopschallenge2025 Step 1: Telemetry Evidence Builder

这一步的目标不是直接升级到 GraphRAG，而是先把 `aiopschallenge2025` 的原始遥测数据转成当前项目可索引、可评测的证据文档。

## 1. 为什么先做这一步

当前 baseline 的 `docs_evidence` 主要来自 `input.json + groundtruth.key_observations` 的派生内容。它能跑通链路，但不是真正的“原始遥测证据”。

第一步要解决的是：

1. 从 `parquet` 中抽取真实的 metric/log/trace 信号
2. 生成能直接进入 RAG 的 markdown 文档
3. 保持和现有 `build/holdout` baseline 流程兼容

## 2. 新增脚本

脚本位置：

- [build_telemetry_evidence.py](/d:/Agent/OpsCaption/scripts/aiops/build_telemetry_evidence.py)

测试位置：

- [test_build_telemetry_evidence.py](/d:/Agent/OpsCaption/scripts/aiops/test_build_telemetry_evidence.py)

## 3. 输入和输出

输入：

- `aiopschallenge2025/input.json`
- `aiopschallenge2025/groundtruth.jsonl`
- `aiopschallenge2025/extracted/**/*.parquet`
- 可选 `aiopschallenge2025/baseline/eval/build_split.json`

输出：

- `aiopschallenge2025/baseline/docs_evidence_telemetry/`
- `aiopschallenge2025/baseline/docs_evidence_telemetry_build/`
- `aiopschallenge2025/baseline/telemetry/case_evidence_summary.jsonl`
- `aiopschallenge2025/baseline/telemetry/case_evidence_summary_build.jsonl`
- `aiopschallenge2025/baseline/telemetry/telemetry_report.json`

## 4. 抽取逻辑

### Metric

优先抽取：

1. `apm/service`
2. `apm/pod`
3. `apm/pod_ns_*`
4. `infra_node` 或 `infra_pod`
5. `infra_tidb` / `other`（仅在 case 看起来属于 TiDB 类场景时）

核心做法：

- 以故障窗口前 `30` 分钟作为 baseline
- 计算故障窗口内的 `incident_mean / incident_max`
- 用 `delta / baseline_mean` 或绝对值变化做粗粒度评分
- 只保留分数靠前的 metric signal

### Log

核心做法：

- 只扫故障时间窗口覆盖到的小时级 parquet
- 用 `pod/node/message` 与 case token 做匹配
- 去掉 ANSI 控制符、UUID、IP、长数字等高噪声片段
- 对 `error/timeout/failed/canceled` 等模式加分
- 避免把 `5009ms` 这种耗时数字误判成 `HTTP 500`

### Trace

核心做法：

- 只扫故障时间窗口覆盖到的 trace parquet
- 按 `service + operation + peer` 聚合
- 统计 `count / error_count / avg_duration_ms / p95_duration_ms`
- 按 `error_count + duration` 的组合分数排序

## 5. 已修掉的质量问题

实现过程中实际踩到的坑：

1. namespace token 会把同命名空间的无关 log/trace 一起吸进来  
修复：namespace 只用于 metric 聚合，不再参与 log/trace relevance。

2. 日志里 ANSI 转义序列没有清理  
修复：加入 ANSI strip。

3. `pandas` 空 `Series` 做按位或时会把真实匹配抹掉  
修复：`series_contains_any(...)` 统一按目标索引返回布尔向量。

4. `5009ms` 这种耗时值会被误判成 `500` 错误码  
修复：`500/503` 改成边界匹配，不再做裸字符串包含。

5. generic node log 会污染 retrieval keywords  
修复：只有 `signal_score > 0` 的 log pattern 才进入 retrieval keywords。

## 6. 本地验证命令

单测：

```powershell
python -m unittest scripts.aiops.test_build_telemetry_evidence
```

真实数据抽样：

```powershell
python scripts/aiops/build_telemetry_evidence.py `
  --dataset-root aiopschallenge2025 `
  --output-root aiopschallenge2025/baseline `
  --limit 3
```

本次抽样结果：

- cases: `3`
- build_cases: `3`
- metric_signals: `15`
- log_signals: `7`
- trace_signals: `6`
- empty_cases: `1`

## 7. 当前边界

这一步已经能生成“真实遥测摘要文档”，但还不是最终版：

1. 已经有远端一键脚本可以把这个目录接到 `knowledge_cmd` 和 `rag_online_eval_cmd`
   入口见 [run_telemetry_baseline_remote.sh](/d:/Agent/OpsCaption/scripts/aiops/run_telemetry_baseline_remote.sh)
2. 还没有在云端对完整 `11.9 GB parquet` 做全量运行
3. metric 评分仍然是启发式，不是 RCA 特化 scoring
4. log pattern 还是基于规则，不是模板挖掘

## 8. 下一步怎么接到 baseline

推荐顺序：

1. 在云端全量生成 `docs_evidence_telemetry_build`
2. 建 collection，例如 `aiops_evidence_telemetry_build`
3. 用现有 [rag_online_eval_cmd](/d:/Agent/OpsCaption/internal/ai/cmd/rag_online_eval_cmd/main.go) 跑同一套 holdout 评测
4. 把结果和当前 `evidence_build/history_build` 做三方对比

目标不是马上替换旧 baseline，而是先回答一个更关键的问题：

`真实遥测摘要文档` 能不能把 `AvgRecall@5` 从现在的 `~0.158` 往上推。
