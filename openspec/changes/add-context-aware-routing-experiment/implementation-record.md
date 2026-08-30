# Implementation Record: add-context-aware-routing-experiment

## 实际完成范围

- 三层漏斗：Layer 1 手动/注入/高风险/强故障边界；Layer 2 Top-K 候选、实体、缺槽、风险和 schema 校验；Layer 3 TTL、状态版本、活跃事故续问、主题切换、实体冲突、澄清上限和槽位回填。
- 旧 `chat / incident / confirm` 路由、旧 v1 Route Adapter、旧客户端字段保持兼容；`intent_funnel.enabled` 默认关闭。
- 路由响应增加可选候选、实体、风险、澄清和脱敏 Trace；Trace 只含 query hash、context fingerprint、版本和层级原因，不含原始敏感文本。
- Stage 0 最终缓存加入策略/上下文指纹，新增候选缓存；澄清、降级、高风险和过期上下文不缓存。
- 服务端实验控制组件支持稳定 HMAC 分桶、Shadow 超时旁路、5%/25%/50% 配置、风险/OOD 排除、Guardrail 停 B 回退和反馈去重。
- Route Adapter 支持 `route-eval/v2` 并保留 v1；v2 输出 Macro-F1、Top-2 Recall、ECE/Brier、歧义/澄清、上下文误用、高风险、OOD、P95、LLM 调用和成本可用性字段。`Variant` 支持 A、B-full、B-no-context、B-no-fast-path 的 deterministic 回放。

## 证据与实验结果

| 证据 | 样本量 | 结果 | 边界 |
|---|---:|---|---|
| recorded route fixture | 3 | 协议回放通过 | 只能证明 fixture contract，不是模型质量或生产效果 |
| deterministic route regression | 28 | 历史 Macro-F1=1.0 | 固定 fixture 回归，不是生产效果 |
| route-eval/v2 deterministic 示例 | 4 | 路由安全、澄清和 v2 指标测试通过 | schema/代码契约，不是标注真实数据 |
| 真实 A 基线 | unavailable | 未运行 | 缺真实冻结语料、模型凭证和授权环境 |
| B-full / 消融真实离线 A/B | unavailable | 未运行 | 不编造收益、显著性或置信区间 |
| Shadow / canary | pending | 未运行 | 无受控流量、观测和部署授权 |

## 指纹与配置

- dataset：现有 route development/regression fixture + `development-v2.jsonl`，运行时由 Harness 计算 SHA-256。
- code/config/prompt/evaluator/context：由 Harness manifest 指纹机制计算；真实实验代际必须冻结所有指纹。
- 默认开关：`agent_router.intent_funnel.enabled=false`，`agent_router.experimentation.enabled=false`，Shadow=false，rollout_stage=off。

## 未完成项

真实分层语料采集与双标注、Validation 校准和 Holdout 单次运行、真实 A/B/消融报告、Shadow 受控环境、低风险 canary 和线上反馈导出均保持 `unavailable/pending`。当前没有把 deterministic、recorded、Shadow 或 canary 结果外推为全量生产效果。

## 回滚

关闭 `agent_router.intent_funnel.enabled` 或 `agent_router.experimentation.enabled` 即立即回到 A 组缓存/关键词/Flash 路由；新字段均为可选，不需要迁移事故数据。Guardrail 触发时服务端停止 B、保留审计事件并继续 A。

## 测试

已运行并通过：

```text
GOCACHE=/tmp/opscaptain-go-cache go test ./internal/app/...
GOCACHE=/tmp/opscaptain-go-cache go test ./internal/app/evaladapter/...
GOCACHE=/tmp/opscaptain-go-cache go test ./internal/ai/evalharness/...
GOCACHE=/tmp/opscaptain-go-cache go build ./...
openspec validate add-context-aware-routing-experiment --strict
cd frontend && npm run build
```

`go test ./...` 已运行；本次受限沙箱中与本变更无关的 `httptest.NewServer` 测试因绑定 `[::1]:0` 被拒绝而失败（`internal/ai/actionexecutor`、`internal/ai/chatops`、`internal/ai/tools`、`internal/infra/jaeger`、`internal/infra/notifier`），其余包通过。默认 Go 缓存目录也曾出现 operation not permitted，因此验证使用仓库外临时缓存目录。Redis、模型凭证和真实 Shadow 未启用，相关日志只作为 degraded 环境证据，不视为代码失败。
