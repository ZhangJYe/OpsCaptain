# 多智能体 P0 Hybrid Triage 实施记录

> 日期：2026-04-26
> 目标：按 `多智能体架构优化分析.md` 的 P0 方案，把 Triage 从纯关键词路由升级为“规则快路径 + LLM 语义兜底”的可灰度实现，并保留可复现评测口径。

---

## 一、为什么先做 Hybrid Triage

当前 Multi-Agent 主链路是：

```text
用户输入 -> Supervisor -> Triage -> Specialists 并行执行 -> Reporter 聚合
```

Triage 是整个链路的入口，入口判断错了，后续 Metrics / Logs / Knowledge 再强也会被错误 fan-out 拖累。基线评测显示：

| 指标 | rule baseline |
|---|---:|
| Case 数 | 20 |
| Intent Accuracy | 0.40 |
| Domain Precision | 0.77 |
| Domain Recall | 0.88 |
| Domain F1 | 0.77 |
| Fallback Rate | 0.70 |

关键问题不是 recall 低，而是 fallback 默认打开 `metrics + logs + knowledge`，导致 recall 看起来高，但成本和噪声都高。

---

## 二、实现策略

P0 只做低风险 Hybrid，不直接替换现有路由：

```text
rule 模式：
  只使用原有规则表，作为可复现 baseline。

hybrid 模式：
  规则命中 -> 直接返回，保持低延迟和稳定性。
  规则未命中 -> 调 DeepSeek quick LLM 输出结构化 JSON。
  LLM 超时 / 初始化失败 / JSON 解析失败 -> 回退到原默认规则。

llm 模式：
  始终走 LLM 分类，主要用于手动评测和对照实验。
```

新增配置：

```yaml
multi_agent:
  triage_mode: "rule"          # rule | hybrid | llm
  triage_llm_timeout_ms: 1000
```

当前默认仍保持 `rule`，原因是：

1. CI 和 baseline 不应依赖外部 LLM。
2. 线上切换需要先确认 DeepSeek 配置、超时和调用量。
3. A/B 时可以只改配置到 `hybrid`，用同一套 golden cases 对比。

---

## 三、代码变更

| 文件 | 变更 |
|---|---|
| `internal/ai/agent/triage/triage.go` | 增加 `rule / hybrid / llm` 三种模式；LLM JSON 分类；超时兜底；补充高频运维规则；输出 `triage_source`、`triage_fallback`、`use_multi_agent` 等 metadata |
| `internal/ai/agent/triage/triage_test.go` | 覆盖规则快路径、规则未命中走 LLM、LLM 失败回退、LLM 输出归一化、高频规则命中 |
| `internal/ai/agent/supervisor/supervisor.go` | 尊重 `use_multi_agent=false`，避免空 domains 被错误默认全量 fan-out；向最终结果透传 Triage metadata |
| `internal/ai/agent/supervisor/supervisor_context_test.go` | 增加 Supervisor 不 fan-out 的回归测试 |
| `manifest/config/config.yaml` | 增加 Hybrid Triage 配置项 |

---

## 四、验收结果

已运行 baseline 命令，确认默认 `rule` 模式仍可复现原始结果：

```powershell
go run ./internal/ai/cmd/agent_eval_cmd -mode routing -runner triage -format json -name current-triage-baseline
```

结果摘要：

```text
cases=20
intent_accuracy=0.40
domain_precision=0.77
domain_recall=0.88
domain_f1=0.77
fallback_rate=0.70
failed=0
```

已运行局部回归：

```powershell
go test ./internal/ai/agent/triage ./internal/ai/agent/supervisor ./internal/ai/agent/eval
```

### 4.1 小步推进：补齐高频规则

为避免过度设计，第二步没有直接推进 Staged Execution 或 LLM Reporter，而是先补齐当前 golden cases 中暴露出的高频规则缺口：

- 告警类：`CPU`、`使用率`、`连接数`、`p95`、`内存`、`慢查询`、`健康状态`
- 故障类：`5xx`、`CrashLoopBackOff`、`504`、`connection refused`、`context deadline exceeded`、`unauthorized`
- 知识类：`什么是`、`怎么配置`、`错误码`、`readinessProbe`、`HPA`、`服务降级`、`熔断`
- 非多智能体类：问候、天气等不需要 specialist fan-out 的输入

复测命令：

```powershell
go run ./internal/ai/cmd/agent_eval_cmd -mode routing -runner triage -format json -name rule-triage-expanded-rules
```

复测结果：

```text
cases=20
intent_accuracy=1.00
domain_precision=1.00
domain_recall=1.00
domain_f1=1.00
fallback_rate=0.00
failed=0
```

这个结果只说明当前 20 条 baseline case 的规则覆盖已补齐，不代表规则路由具备完整语义泛化能力。后续仍需要扩大 holdout case，再判断是否把默认模式从 `rule` 切到 `hybrid`。

---

## 五、如何做 Hybrid A/B

对照组保持：

```yaml
multi_agent:
  triage_mode: "rule"
```

实验组改为：

```yaml
multi_agent:
  triage_mode: "hybrid"
```

然后使用同一条命令评测：

```powershell
go run ./internal/ai/cmd/agent_eval_cmd -mode routing -runner triage -format json -name hybrid-triage
```

重点看四个指标：

1. `intent_accuracy` 是否从 0.40 提升。
2. `fallback_rate` 是否明显下降。
3. `domain_precision` 是否提升，说明少调了不必要 specialist。
4. `domain_recall` 是否不低于 baseline，避免漏查关键证据。

---

## 六、面试讲解版

可以按 STAR 讲：

**Situation**
项目已经有 Supervisor-Triage-Specialists-Reporter 多智能体链路，但 Triage 仍是关键词规则。基线评测发现 Intent Accuracy 只有 0.40，Fallback Rate 达到 0.70。

**Task**
我没有直接把路由改成全 LLM，而是设计 Hybrid Triage：保留规则快路径，只有规则未命中时才调用轻量 LLM 分类，同时必须可配置、可回退、可评测。

**Action**
我先补了 routing baseline harness，用 20 条 golden case 量化 intent、domain precision/recall 和 fallback rate。然后实现 `rule | hybrid | llm` 三种模式，LLM 输出结构化 JSON，超时或解析失败回退到原规则。Supervisor 层也补了一处关键修复：当 Triage 判断 `use_multi_agent=false` 时，不再默认展开所有 specialist。

**Result**
当前默认 rule 模式保持 baseline 可复现，Hybrid 通过单测验证了规则命中不调 LLM、未命中走 LLM、LLM 失败安全回退。后续只需切配置到 `hybrid`，即可用同一套 golden cases 做 A/B，对比路由质量和成本变化。

---

## 七、边界与下一步

- 当前实现只把 LLM 用在 Triage，不把 memory 拼进 routing goal，避免历史上下文污染路由。
- Hybrid 的真实收益需要在 DeepSeek 配置可用的环境跑 A/B，不能只看单测。
- 如果 Hybrid 指标稳定提升，再考虑把 `triage_mode` 默认值从 `rule` 切到 `hybrid`。
- 后续 P1 可以继续做 Staged Execution 或 LLM Reporter，但要沿用这套 baseline 方法先测再改。

---

## 八、框架推进：P1/P2 骨架先落地

> 日期：2026-04-26
> 原则：先把大体框架打通，不抠策略细节；所有新能力默认关闭或保持原行为。

### 8.1 新增配置

```yaml
multi_agent:
  execution_mode: "parallel"        # parallel | staged
  reporter_mode: "template"         # template | llm | auto
  reporter_llm_timeout_ms: 10000
  self_reflect_enabled: false
```

### 8.2 Staged Execution 骨架

Supervisor 现在支持两种执行模式：

```text
parallel:
  保持原有并行 fan-out，默认模式。

staged:
  按 Triage 返回的 domains 顺序串行执行 specialist。
  后执行的 specialist 会收到 prior_results 和追加到 goal 的“上游专业代理结果”。
```

当前只实现信息传递框架，不做复杂动态重排。后续如果要优化质量，可以让 Triage 输出 `primary_domain/execution_order`，或让前一个 specialist 的结构化结果生成更精细的 focus hints。

### 8.3 LLM Reporter 骨架

Reporter 现在支持三种模式：

```text
template:
  保持原模板拼接，默认模式。

llm:
  调 DeepSeek quick 把 specialist 结果合成为 Markdown 诊断报告。
  LLM 初始化、超时或调用失败时回退 template。

auto:
  当前简单规则：当 specialist 结果数 >= 2 时走 LLM，否则走 template。
```

这一步只解决“框架可切换 + 失败可回退”，不在 prompt 上继续细调。

### 8.4 Self-Reflect 骨架

Self-Reflect 目前是 audit-only：

```text
self_reflect_enabled=false:
  不产生 reflection metadata。

self_reflect_enabled=true:
  在最终 metadata 中写入 reflection_status / reflection_reason / reflection_missing_domains。
```

当前不会自动二次调度，避免过早引入重试循环导致成本和状态管理复杂化。后续可以基于这些 metadata 再加一次补查。

### 8.5 当前验收

已覆盖单测：

- Staged Execution 会把前序 specialist summary 传给后续 specialist。
- LLM Reporter 成功时使用合成报告。
- LLM Reporter 失败时回退模板。
- Self-Reflect 打开后输出 audit metadata。

已运行：

```powershell
go test ./internal/ai/agent/supervisor ./internal/ai/agent/reporter ./internal/ai/agent/triage ./internal/ai/agent/eval
```

面试讲解时可以这样说：

> 我没有一次性把多智能体链路改成复杂的 ReAct/Replan 系统，而是先把三个关键扩展点用 feature flag 打出来：执行模式、报告模式、自省状态。默认值完全保持原行为，所以风险可控；后续每个点都可以独立 A/B 和灰度。
