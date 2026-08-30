## Purpose

为统一 Evaluation Harness 提供规模明确、场景分层且完全确定性的 PR 回归语料，用于发现多链路契约与关键不变量回归，并明确区分回归验证与生产效果评估。

## ADDED Requirements

### Requirement: 分层回归语料规模
系统 SHALL 为 deterministic regression profile 提供至少 160 条 case，并包含 Route 28、RAG 28、Plan 24、GoS 28、Tool 28、Evidence 24 条。

#### Scenario: PR Gate 加载完整语料
- **WHEN** CI 使用 regression + deterministic manifest 执行统一 Gate
- **THEN** 六类 suite SHALL 按声明的最低数量加载 case，且总数不得低于 160

### Requirement: 关键场景覆盖
回归语料 MUST 覆盖各 suite 的正常路径、核心边界与 Adapter 可确定性判定的受控结果，且场景标签 SHALL 可被确定性校验。

#### Scenario: 场景覆盖校验
- **WHEN** 校验回归数据集
- **THEN** Route SHALL 覆盖 chat、incident、confirm、cache 和 low-confidence，RAG SHALL 覆盖命中、排序、多相关文档与 hard-negative，Plan/GoS SHALL 覆盖成功、重规划、降级、回溯与证据关联，Tool SHALL 覆盖成功、权限、超时、取消、错误、畸形和降级，Evidence SHALL 覆盖单/多引用、跨来源与引用追踪

### Requirement: 跨链路关联
系统 SHALL 通过稳定 case ID 将故障路由、Plan/GoS、Trace、Evidence 与权限拒绝场景关联，并继续执行已有跨链路 Gate。

#### Scenario: 共享事故 case
- **WHEN** 同一事故 case 出现在 Route 与 Plan 或 GoS suite
- **THEN** 报告 SHALL 能关联其诊断 Trace 与 Evidence，并验证权限拒绝未产生实际工具执行

### Requirement: 回归语料完整性
系统 MUST 校验 case schema、suite 匹配、ID 唯一性、最低规模和关键场景分布；任一约束不满足时 PR Gate SHALL 失败。

#### Scenario: 数据集意外缩小
- **WHEN** 任一 suite 的 case 数低于声明的最低值
- **THEN** 校验 SHALL 返回明确错误并阻止 Gate 通过

### Requirement: 真实性边界
回归报告 MUST 声明 deterministic fixture 的依赖状态，且不得将结果表述为 holdout、live 或 production 效果。

#### Scenario: 离线回归报告
- **WHEN** 160 条 deterministic regression case 全部通过
- **THEN** 报告 SHALL 仅证明回归契约与固定场景结果，并继续标记真实模型、Milvus、Prometheus、日志系统和线上流量为未验证

### Requirement: 大语料外置与镜像隔离
系统 MUST 将 development、frozen holdout 和 Replay 原始语料视为离线资产，并通过版本、位置、SHA-256、数据角色和保留策略进行登记；这些资产不得被复制进生产镜像或作为线上服务运行时依赖。

#### Scenario: 构建生产镜像
- **WHEN** 构建 OpsCaptain 生产镜像
- **THEN** Docker 构建上下文 SHALL 排除外置评测语料和评测报告，最终运行镜像 SHALL 不包含其原始 case 或 evidence 内容
