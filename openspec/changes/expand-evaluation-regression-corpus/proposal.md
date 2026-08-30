## Why

统一 Evaluation Harness 当前只有 11 条确定性 fixture，只足以验证入口、Adapter 契约和少量跨链路不变量，无法覆盖常见边界与失败模式。需要建立分层且可维护的评测语料：PR Gate 负责快速回归，development/holdout/replay 负责离线评估，且大语料不进入生产镜像或服务器磁盘。

## What Changes

- 将 deterministic PR regression 从 11 条扩充到 160 条：Route 28、RAG 28、Plan 24、GoS 28、Tool 28、Evidence 24。
- 覆盖正常路径、低置信度与缓存、检索排序与 hard negative、重规划与降级、证据冲突、工具超时/权限/畸形返回以及引用完整性。
- 保留跨 suite 的共享 case ID，用于验证 incident route、Plan/GoS、Trace、Evidence 和权限拒绝等跨链路约束。
- 增加数据集规模、suite 分布、ID 唯一性和关键场景覆盖校验，防止后续回归集意外缩小或失衡。
- 增加外置大语料目录约定：development、frozen holdout 与 replay 原始证据通过受控 CI/离线任务按需挂载或下载，仓库只保存 manifest、摘要与指纹。
- 排除外置语料目录和评测运行产物的 Docker 构建上下文；不修改线上 Controller、Agent 路由、RAG、Plan 或 GoS 执行链路。

## Capabilities

### New Capabilities

- `evaluation-regression-corpus`: 定义统一 Harness 的分层确定性回归语料、最低规模、覆盖约束和真实性边界。

### Modified Capabilities

无。

## Impact

- 主要影响 `evals/harness/datasets/`、`evals/harness/manifests/pr-regression.yaml` 及对应数据校验测试。
- 继续复用现有六类 Adapter 和领域 evaluator，不新增外部依赖，不访问模型、Milvus、Prometheus 或日志系统。
- PR Gate 运行 case 数增加，但仍受现有并发、单 case 超时和总预算约束。
