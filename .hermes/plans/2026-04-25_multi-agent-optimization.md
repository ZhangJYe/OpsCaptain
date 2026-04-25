# OpsCaption 多智能体优化 —— 实现计划

> **日期：** 2026-04-25
> **范围：** P0 Hybrid Triage + P1 LLM Reporter + P1 Staged Execution + 评测基础设施
> **策略：** 按优先级分波次实现，每波次完成后跑测试验证

---

## 波次 1：评测基础设施（先搭台子）

没有评测就没法证明优化有效，所以先搭。

### 1.1 新建 eval 包

```
internal/ai/agent/eval/
├── types.go           ← DiagCase, DiagScores 定义
├── runner.go          ← MultiAgentRunner（封装 Runtime）
├── judge.go           ← LLM-as-Judge（四维评分）
├── ab.go              ← A/B 对比 Runner
├── diag_test.go       ← Golden Case 回归测试
├── judge_test.go      ← Judge 校准测试
└── testdata/
    ├── diag_golden.jsonl     ← 10个 Golden Case
    └── diag_calibration.jsonl ← 5个校准 Case
```

### 1.2 验证

- `go test ./internal/ai/agent/eval/...` 通过
- Golden Case 回归在现有代码上跑通（建立基线）

---

## 波次 2：P0 Hybrid Triage

### 2.1 改造 triage.go

- 保留现有关键词匹配逻辑
- 新增 `llmTriage()` 方法：调用 DeepSeek 做语义分类
- `Handle()` 改为 Hybrid：先关键词，未命中 → LLM
- LLM 调用 1s 超时，失败 fallback 到默认规则

### 2.2 新增配置项

```yaml
# manifest/config/config.yaml
multi_agent:
  triage:
    mode: "hybrid"        # keyword | llm | hybrid
    llm_timeout_ms: 1000  # LLM 分类超时
```

### 2.3 验证

- 现有 `supervisor_replay_test.go` 不改，确保向后兼容
- 新增 `triage_accuracy_test.go`：对比关键词 vs LLM

---

## 波次 3：P1 LLM Reporter

### 3.1 改造 reporter.go

- 保留现有模板拼接逻辑（fallback）
- 新增 `llmReport()` 方法：输入所有 Specialist 结果 → LLM 合成诊断报告
- Reporter 增加 mode 选择：`template | llm`

### 3.2 Prompt 设计

- 四维输出：诊断结论 + 证据链 + 矛盾项 + 建议
- 控制 token 消耗

### 3.3 新增配置项

```yaml
multi_agent:
  reporter:
    mode: "llm"           # template | llm
```

### 3.4 验证

- 现有回归测试通过（模板模式不受影响）
- 新增矛盾检测专项测试

---

## 波次 4：P1 Staged Execution

### 4.1 改造 supervisor.go

- 新增 `execution_mode` 字段：`parallel | staged`
- Staged 模式：Triage 输出 primary_domain → 先跑 primary → 提取 focus_hints → 再跑其余

### 4.2 改造 TaskEnvelope

- 新增 `FocusHints` 字段
- Specialist 检查 FocusHints，有则聚焦检索

### 4.3 新增配置项

```yaml
multi_agent:
  execution:
    mode: "staged"        # parallel | staged
```

### 4.4 验证

- 并行模式回归测试通过
- Staged 模式 A/B 对比

---

## 文件变更总览

| 文件 | 操作 | 波次 |
|------|------|------|
| `internal/ai/agent/eval/*` | 新建 | 1 |
| `internal/ai/agent/triage/triage.go` | 修改 | 2 |
| `internal/ai/agent/reporter/reporter.go` | 修改 | 3 |
| `internal/ai/agent/supervisor/supervisor.go` | 修改 | 4 |
| `internal/ai/protocol/types.go` | 修改（加 FocusHint） | 4 |
| `manifest/config/config.yaml` | 修改（加配置项） | 2-4 |
| `internal/ai/service/chat_multi_agent.go` | 可能修改 | 2-4 |

---

## 风险

- LLM 调用增加延迟 → 所有 LLM 节点都有超时 + fallback
- 评测基础设施可能过度工程化 → 先做最小版本（10 个 Golden Case + 简单的 Judge）
- Staged Execution 可能比并行慢 → 可配置，可通过 A/B 对比决定
