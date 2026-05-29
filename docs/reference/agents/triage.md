# Agent: triage

## 角色
任务分诊器，负责把原始用户问题映射为 intent、priority 和 specialist domains。

## 输入
- raw query（原始用户问题）
- triage rules（分诊规则表）

## 输出
- intent（用户意图分类）
- domains（需要调度的 specialist 列表，如 metrics、logs、knowledge）
- priority（任务优先级）

## 约束 (Must)
- 优先保持 routing 可解释
- 当无法精确识别时，选择 metrics、logs、knowledge 的安全默认组合
- 保持规则表驱动，便于扩展和 replay

## 禁止 (MustNot)
- 不要用 memory_context 改写 routing 判断
- 不要在 triage 阶段读取工具或外部文档
- 不要把 specialist 的执行职责前置到 triage

## 证据策略
- triage 不生产业务证据，只生产路由元数据

## 依赖
- 无外部工具依赖
- 输入：原始用户查询文本

## 降级策略
当无法精确识别用户意图时，返回 metrics + logs + knowledge 的安全默认组合，不做单域猜测。

## 相关代码
- Contract: `internal/ai/agent/contracts/contracts.go`（registry key: "triage"）
- 实现: `internal/ai/agent/triage/`（历史代码，当前主链路不使用）
