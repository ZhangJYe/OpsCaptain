## Context

统一 Harness 已具备六类 Adapter、三级预算、公共/领域/跨链路 Gate 和报告指纹，但 PR manifest 目前只加载 11 条 fixture。现有 RAG development/holdout 与 GoS 数据可作为覆盖设计参考，不能直接混入 regression 并改变其数据角色。

## Goals / Non-Goals

**Goals:**

- 构建 160 条快速、无网络、可重复的 PR 回归语料。
- 用自动校验保护 suite 数量、ID、场景覆盖和跨链路关联。
- 复用现有 Adapter payload schema 和评分逻辑。
- 建立大规模 development/holdout/replay 的外置存储契约，并从 Docker 构建上下文排除。

**Non-Goals:**

- 不声称达到生产级统计规模。
- 不接入真实模型、Milvus、Prometheus、日志系统或线上流量。
- 不修改线上 Router、Agent、RAG、Plan、GoS 或 Tool 行为。
- 不把 development/holdout 标签或真实事故内容复制进 fixture。

## Decisions

### 1. 维持三层数据集职责

PR regression 保持小而确定；development 用于调参；holdout 用于独立发布判断。相比把现有 160 条 RAG development/holdout 全塞进 PR，这一方案不会造成角色混用或显著增加 CI 成本。

### 2. 固定 160 条 suite 配额

按 Route 28、RAG 28、Plan 24、GoS 28、Tool 28、Evidence 24 分配。Route/RAG/Tool 状态空间更离散，Plan/GoS/Evidence 使用共享事故 case 形成跨链路覆盖。相比平均分配，配额更贴合当前 Adapter 的风险面。

### 3. 数据校验独立于指标 Gate

在执行指标 Gate 前校验每个 JSONL 的数量、schema、suite、唯一 ID 和场景标签。指标阈值通过不能掩盖数据集意外缩小或覆盖丢失。

### 4. 复用共享 case ID，不复制线上执行

一组事故 ID 在 Route、Plan/GoS、Evidence 中复用，Tool 权限 case 使用关联 ID。跨链路 Gate 基于报告结果关联，仍由 deterministic Adapter 执行，不触发真实依赖。

### 5. 大语料只登记，不伪造生产数据

仓库保留 `evals/harness/external-corpora.yaml`，记录离线资产的角色、预期规模、版本、SHA-256、获取方式和保留策略；实际内容放在 CI 的受控 Artifact/对象存储或获准的离线评测卷中。相比把合成 case 大量提交到仓库或复制到服务器，该方案保留可审计性并避免假称生产分布。

## Risks / Trade-offs

- [fixture 数量增加但仍可能同质化] → 为每类 case 标注场景，并校验关键场景存在；后续再接独立 holdout。
- [PR Gate 时间增长] → 保持 fixture-only、提高允许 case 上限但不放宽单 case/总超时。
- [为了全通过而构造过于理想的数据] → 通过真实可判定的权限拒绝、超时、畸形返回和降级结果覆盖负向行为；无法由当前 Adapter 表达的负向语义留在单元测试或后续 live/replay profile。
- [误把 160 条称为生产级] → 报告、外置语料登记和实施记录显式保留真实性边界。
- [大语料误入镜像或服务器磁盘] → `.dockerignore` 排除外置目录，离线资产通过 CI/对象存储按需使用。

## Migration Plan

1. 增补六类 regression JSONL，并保持现有 case ID 可用。
2. 增加数据集规模与覆盖校验测试及外置语料登记。
3. 调整 PR manifest 的 case 预算、Docker 构建排除并运行统一 Gate。
4. 失败时可恢复原数据文件、登记和预算；该变更不影响线上请求链路。
