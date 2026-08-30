## Why

当前自动路由使用“缓存 → 高置信故障关键词 → 单次 Flash 分类”，能够完成 `chat / incident / confirm` 基础分流，但无法系统处理多轮省略、候选意图接近、上下文切换、缺失关键实体和高风险操作等歧义。现有 Route 评测主要是 3 条 recorded fixture 与 28 条 deterministic fixture，只能证明协议和回归链路可用，不能作为真实模型路由质量或生产效果证明。

## What Changes

- 在现有缓存加速层之后，引入三层漏斗式意图路由：确定性边界与快路径、Top-K 语义候选、结构化上下文消歧与校验。
- 明确上下文使用边界：原始 query 始终是路由主输入；只允许使用有界、可审计的会话状态、已确认实体、待补槽位和当前流程状态，Memory 不得重写或替代原始 query。
- 将路由结果扩展为候选意图、置信度、候选间距、实体、缺失槽位、校验理由与 `route / clarify / deny` 决策；高风险请求继续由权限、审批和确定性执行边界控制。
- 扩展 Route 评测协议，建立分层标注的真实离线基线，覆盖清晰请求、易混淆请求、多轮省略、上下文切换、缺失参数、高风险和 OOD；记录 Macro-F1、Top-2 Recall、校准误差、澄清质量、上下文误用率、安全指标、P95 与成本。
- 设计 A/B 实验控制面：A 组保持当前缓存/关键词/Flash 路由，B 组启用三层漏斗；先离线回放，再 Shadow，仅在通过安全 Gate 后灰度低风险流量。
- 使用稳定匿名主体进行确定性分桶，记录实验版本、候选决策和最终采用决策；Shadow 不改变用户路径，高风险请求不得因实验自动执行。
- 定义逐级放量、停止、回滚与结论冻结规则；不把 deterministic、recorded、Shadow 或小流量实验误写成生产全量效果。

## Capabilities

### New Capabilities

- `agent-intent-routing-funnel`: 定义三层漏斗、结构化上下文校验、主动澄清、高风险边界和可观测路由结果。
- `agent-routing-experimentation`: 定义真实基线、离线消融、Shadow、A/B 分桶、指标、Guardrail、放量与回滚协议。

### Modified Capabilities

- 无。

## Impact

- 路由应用层：`internal/app/agent_router.go` 及其测试，新增候选、上下文校验、澄清和实验 Trace 契约。
- API 与前端：统一 Agent 路由请求可选携带结构化会话状态；响应增加澄清与实验元数据，保持旧客户端兼容。
- Prompt 与配置：`prompts/` 和 `manifest/config/config.yaml` 增加候选数、置信阈值、候选间距、上下文预算、feature flag、Shadow/A/B 配置与安全 Gate。
- 评测：`internal/app/evaladapter/route.go`、`internal/ai/evalharness/`、`evals/harness/` 增加新 schema、分层数据集、A/B 报告和冻结基线记录。
- 可观测性：新增不含原始敏感文本的 route trace、实验分桶、人工改路由与任务完成反馈；不新增外部运行时依赖。
