## Why

现有统一评测 Harness 已具备 deterministic PR 回归和小规模 recorded fixture，但外置大语料仍只是登记信息。AIOps2025 本地离线数据包含 400 个带根因、时间窗和关键观测标签的案例，可作为首个可追溯的录制评测来源；它目前没有进入 Harness，也没有防止同类故障跨开发集与冻结集泄漏的机制。

本变更将把这份外置数据转化为可复现的开发/冻结评测资产，而不把多 GB 观测数据、原始日志或标签打进仓库和生产镜像。

## What Changes

- 新增 AIOps2025 外置语料的导入与版本清单：仅保存来源、许可、路径、输入/标签/证据指纹和生成的轻量 case 文件。
- 将 400 条已标注故障按故障家族分组，确定性切分为开发集与冻结 holdout；同一故障类型、服务目标和观测时间窗的关联样本不得跨 split。
- 为 RAG / GoS recorded adapter 生成可执行的评测 case，标签仅作为离线 expectation，不进入运行时 prompt、RAG 语料或路由规则。
- 增加 corpus validate / prepare 命令，验证来源完整性、ID 对齐、分组泄漏、split 指纹与最低规模，并产出可审计的 JSON/Markdown 摘要。
- 新增 recorded development 与 frozen holdout manifest；holdout 只允许显式离线执行，不接入 PR gate 或生产镜像。
- 将首次基线报告和运行限制记录在 Harness 文档中，明确其为离线录制证据，不代表线上或生产效果。

## Capabilities

### New Capabilities

- `recorded-evaluation-corpus`: 外置已标注 AIOps 语料的准备、版本化、防泄漏切分与 recorded 评测接入。

### Modified Capabilities

- 无。

## Impact

- `cmd/eval_harness/` 与 `internal/ai/evalharness/`：新增 corpus prepare/validate 入口及 manifest 校验。
- `evals/harness/`：新增轻量 AIOps2025 case、manifest、报告摘要和使用文档；大体积数据继续位于仓库外。
- `.gitignore` / `.dockerignore`：保持外置语料及原始观测数据不被提交或打入镜像。
- 不修改线上 Agent 路由、模型调用、Milvus 或部署配置；不上传本地数据，也不调用外部服务。
