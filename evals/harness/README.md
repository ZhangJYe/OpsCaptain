# Unified Evaluation Harness

统一入口覆盖 Route、RAG、Plan、GoS、Tool 与 Evidence 六类离线评测。报告始终同时记录 `dataset_role` 与 `profile`，两者不可互相替代。

## 本地与 CI

```bash
make eval-harness-gate
```

该命令只运行 `regression + deterministic` 数据，使用固定路由、检索、诊断和工具 fixture。它不会请求模型、Milvus、Prometheus 或日志服务，结果只能作为 PR 离线回归证据。

手动工作流 `recorded-evaluation` 使用 `development + recorded`，用于复放已审阅的开发证据，不能描述为 holdout、live 或生产效果。

## Live / holdout 边界

`manifests/live-holdout.yaml` 默认禁用所有 suite，只是受控环境模板。运行前必须：

1. 获得访问真实模型、检索与观测依赖的明确授权；
2. 配置未参与当前调参的 holdout 数据集和标签来源；
3. 冻结候选配置，并记录 dataset、code、model、prompt、evaluator 与 evidence corpus 指纹；
4. 在获准环境执行并保留 JSON/Markdown 报告；
5. 依赖不可用时保留 degraded、failed 或 skipped，不回退成 deterministic 后宣称 live 通过。

本地 deterministic/recorded 结果不替代真实环境验证，也不证明线上业务收益。

## AIOps2025 外置录制语料

`eval_harness corpus prepare` 可将仓库外的 AIOps2025 目录转换为可执行的 GoS 与 Evidence recorded case。原始 metrics/logs/traces 和生成的完整 case 都保留在操作者指定的外置目录，不进入 Git、CI 默认工件或生产镜像。

```bash
go run ./cmd/eval_harness corpus prepare \
  --source=<AIOPS2025_ROOT> \
  --output=<EXTERNAL_CORPUS_ROOT> \
  --version=aiops2025-2025-06

go run ./cmd/eval_harness corpus validate \
  --manifest=<EXTERNAL_CORPUS_ROOT>/corpus-manifest.json

go run ./cmd/eval_harness run \
  --manifest=<EXTERNAL_CORPUS_ROOT>/manifests/recorded-development.yaml \
  --output-dir=evals/harness/reports
```

prepare 会校验 input 与 ground truth 的 UUID 一一对应，记录 source / license / SHA-256，并按 `fault_type + instance_type + target + observation_date` 做确定性 group split。同一故障家族不会跨 development 与 holdout；case 标签只保留在离线 expectation 和 recorded fixture，不能作为 RAG 文档、路由输入或线上提示词使用。

当前 AIOps2025 版本只有 400 条公开带标注故障：首次切分为 296 条 development、104 条 holdout。它是可复现的首批来源，不等于 registry 规划的 3000 / 700 / 2000 总规模；命令和报告会明确显示缺口，禁止用改写或合成标签填充。

该数据集为 CC BY-NC 4.0，仅适合离线研究和演示。recorded 运行验证的是语料、评测协议和 fixture contract，不验证真实模型、Milvus、观测后端或生产诊断效果。
