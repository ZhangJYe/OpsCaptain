## Context

现有 Harness 已统一了 Route、RAG、Plan、GoS、Tool 与 Evidence 的评测报告和指纹，但 recorded development 只使用小型 fixture。`evals/harness/external-corpora.yaml` 已约定大语料不进仓库和镜像。AIOps2025 在本机仓库外，含 400 条 anomaly input、400 条 fault ground truth 和多 GB 指标/日志/调用链观测文件，许可为 CC BY-NC 4.0。

## Goals / Non-Goals

**Goals:**

- 将已标注 source 转成可运行、可复查的外置 recorded corpus。
- 以 case ID、源文件 SHA-256、稳定 split key 和 corpus manifest 建立可重复的数据链路。
- 对 source ID 对齐、schema、split 泄漏、fingerprint 与输出完整性提供本地可运行的校验。
- 生成 development / holdout 的运行 manifest，并令 Harness 报告保留 corpus provenance。

**Non-Goals:**

- 不将原始 parquet/log/trace 复制到仓库、镜像或 CI 默认工件。
- 不伪造 3000/700/2000 条标签，不把同一事故的改写版本当作独立真实 case。
- 不将 recorded 结果描述为 live、线上或生产收益。
- 不修改线上 Agent 推理、路由或检索链路；live adapter 的真实依赖接入另行设计。

## Decisions

### 1. 外置 corpus 包由 prepare 命令生成

新增 `eval_harness corpus prepare`，接收 `--source` 和 `--output`。source 是 AIOps2025 根目录；output 必须由操作者显式提供，默认不写入仓库受版本控制的目录。命令读取 `input.json` 与 `groundtruth.jsonl`，产生 JSONL case 和 `corpus-manifest.json`。

选择生成轻量 JSONL，而不是在 Harness 中直接读取 parquet：Harness 只需要输入、离线 expectation 和证据关键词；解析数 GB telemetry 不适合 PR/录制 contract runner。真实观测重放仍通过 source 路径和指纹被引用，不被复制。

### 2. 首期只接入 GoS 与 Evidence recorded suites

每个 AIOps2025 case 能产生：

- GoS case：原 anomaly description 作为用户输入；fault type、受影响实体、关键观测关键词、稳定 evidence ID 作为离线 expectation / 录制 TaskResult。
- Evidence case：key observations 转为跨 metric/log/trace 的 expected evidence source 与关键词验证。

不将 ground truth 转成 RAG 文档再用同一标签评测 RAG，避免答案泄漏。RAG 的独立 development / holdout 仍使用已有文档-查询语料或后续接入独立知识库。Plan 与 Tool 也不从该 source 人工推断，以免把测试 fixture 误称真实回放。

### 3. 按故障家族键做不可跨 split 的确定性切分

family key 使用 `fault_type | instance_type | canonical affected target | observation_date`。同日、同类、同目标的注入通常共享观测模式，放入同一 split 能降低泄漏。家族采用 SHA-256 的稳定排序，约 75% 家族进入 development，其余进入 frozen holdout；case 数量偏离时在报告中说明，不强行打散家族凑比例。

选择 group split 而不是随机 case split，因为随机切分会让同类同服务同时间的样本同时出现于调参与评测；代价是各分类比例无法完全均匀，报告会呈现实际分布。

### 4. Corpus manifest 是运行前置条件

新增 versioned `corpus-manifest.json`，包含 source / license、input 与 label SHA、split strategy、split fingerprint、统计、case 文件 SHA 与创建时间。开发和冻结 Harness manifest 引用该文件；运行前校验 source manifest、case fingerprint、dataset role 和 profile。

把 provenance 放在外置 corpus manifest 而不是只放 YAML 注释中，可以允许不同 runner 生成可比较的报告，并能检测 source/label 换了但 case 文件未更新的情况。

### 5. 第一批规模以现有 400 个有标签 case 为上限

AIOps2025 的可验证标签为 400 条，prepare 输出实际 development/holdout 数量，不试图满足远期 3000/700/2000 目标。外部 registry 的目标仍保留为容量规划；报告将该差距标为 source coverage gap。后续扩容必须引入额外脱敏录制事故或新的公开基准，并走相同 prepare/validate 过程。

## Risks / Trade-offs

- [AIOps2025 许可为非商业且数据域偏向 HipsterShop/TiDB] → manifest 记录许可证与来源；仅用作离线研究/面试演示，生产数据评测需独立授权。
- [按 family group split 后类别不均衡] → 输出分布和缺口，不为配平跨 split 拆分同族数据。
- [录制 TaskResult 不覆盖真实模型、Milvus 或 telemetry 查询] → 报告固定标为 recorded，不允许拿来宣称 live 诊断准确率。
- [冻结标签位于 runner 可读输出目录] → holdout manifest 使用显式路径和只读权限约定；本地开发仅验证 workflow，发布评测需由受限 runner 管理。

## Migration Plan

1. 增加 corpus 数据结构、prepare/validate 命令与单元测试。
2. 使用本机已有 AIOps2025 目录生成一次外置 corpus，检查统计、指纹和 split 泄漏。
3. 生成 development/holdout recorded manifest，运行 development 基线并保留报告。
4. 把运行命令、许可边界和扩容缺口写入 Harness README；外置目录保持 `.gitignore` 与 `.dockerignore` 排除。
5. 回滚只需移除新 corpus manifest/命令；外置 source 与输出未被线上服务或镜像引用。
