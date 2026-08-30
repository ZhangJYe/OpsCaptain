## Why

OpsCaptain 已分别具备 RAG、GoS 和部分 Plan 评测能力，但数据集角色、运行入口、报告结构、基线指纹和质量 Gate 分散在不同命令中，路由、Tool 与 Evidence 也缺少可横向比较的评测协议。现在需要统一评测控制面，让一次变更能够回答“影响了哪条链路、在哪个阶段退化、是否允许合入”，同时继续保留各领域指标的专业性。

## What Changes

- 新增统一 Evaluation Harness 命令入口，按 suite 选择并编排 Route、RAG、Plan、GoS、Tool、Evidence 六类评测，不改变线上请求链路。
- 定义统一的数据集清单、case envelope、运行 profile、结果状态、执行元数据和报告 envelope；领域特有输入、输出与指标通过版本化 payload 扩展。
- 通过 adapter 复用现有 RAG 与 GoS evaluator，并为 Route、Plan、Tool、Evidence 提供同一 Runner 契约，禁止重写已有领域评分逻辑。
- 定义 common metrics 与 domain metrics 两层指标：公共层衡量完成率、降级率、P95 延迟、调用成本和 trace 完整性，领域层保留路由准确率、检索指标、根因命中、工具契约和证据质量等口径。
- 统一 development、holdout、regression 数据集角色及 deterministic、recorded、live 运行 profile，记录 dataset、配置、代码、模型、Prompt 与证据语料指纹。
- 新增统一 baseline comparison 与分层 Gate：公共硬门槛、领域 Gate、跨链路不变量分别判断，禁止用单一“总体准确率”掩盖局部退化。
- 输出机器可读 JSON 报告和面向开发者的 Markdown 摘要，提供失败 case、失败阶段、指标差异和证据定位信息。
- 将确定性 smoke/gate 接入 PR CI；recorded 与 live 评测仅通过手动或定时任务运行，外部依赖不可用时明确标记 degraded/skipped，不伪装为通过。

## Capabilities

### New Capabilities

- `evaluation-harness`: 统一定义跨 Route、RAG、Plan、GoS、Tool、Evidence 的评测编排、数据协议、报告、基线对比与质量 Gate。

### Modified Capabilities

无。现有主规格目录尚未定义需要修改的评测能力；本变更通过新增能力整合现有 evaluator。

## Impact

- 预计新增 `internal/ai/evalharness/` 领域模块和 `cmd/eval_harness/` CLI 入口。
- 需要为现有 `internal/ai/rag/eval/`、`internal/ai/agent/gos_engine/eval/`、Plan 运行产物、Agent Router、Tool Registry 和 Evidence schema 增加薄 adapter。
- 评测数据与报告统一收敛到 `evals/harness/`，但保留现有 RAG/GoS 数据集和报告的兼容读取能力。
- `manifest/config/config.yaml` 增加评测 suite、预算、超时、并发和 Gate 配置；默认不触发真实外部模型或基础设施调用。
- `.github/workflows/ci.yml` 增加确定性评测 Gate；live/recorded 工作流不得被描述为生产验证。
- 不新增线上 API，不恢复已废弃的 `chat_multi_agent`，不允许 Memory 替代原始 query 参与路由评分。
