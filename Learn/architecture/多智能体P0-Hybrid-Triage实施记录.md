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
| `internal/ai/agent/triage/triage.go` | 增加 `rule / hybrid / llm` 三种模式；LLM JSON 分类；超时兜底；输出 `triage_source`、`triage_fallback`、`use_multi_agent` 等 metadata |
| `internal/ai/agent/triage/triage_test.go` | 覆盖规则快路径、规则未命中走 LLM、LLM 失败回退、LLM 输出归一化 |
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
