# AI Ops Replay 基线用例

## 1. 文档目的

本文档定义 AI Ops Multi-Agent 的第一版 replay 基线，用于后续回归测试、复盘和质量评估。

当前版本覆盖目标：

- 验证 supervisor 编排是否正常
- 验证 triage 路由是否符合预期
- 验证 reporter 输出结构是否稳定
- 为后续接入真实工具回放提供固定 case 基础

---

## 2. 基线原则

每个 replay case 至少定义以下内容：

- 输入问题
- 预期 intent
- 预期 domain fanout
- 关键输出检查点
- 最低验收标准

---

## 3. 基线用例

## Case 1：告警分析全域联动

### 输入

`请分析当前 Prometheus 告警并结合日志排查`

### 预期 intent

`alert_analysis`

### 预期 domains

- `metrics`
- `logs`
- `knowledge`

### 关键输出检查点

- 报告中包含 `Metrics`
- 报告中包含 `Logs`
- 报告中包含 `Knowledge`

### 最低验收标准

- supervisor 正常 fanout
- reporter 汇总三类 specialist 结果

---

## Case 2：知识库单域检索

### 输入

`请查询知识库中的 SOP 文档`

### 预期 intent

`kb_qa`

### 预期 domains

- `knowledge`

### 关键输出检查点

- 报告中包含 `Knowledge`
- 报告中不包含 `Metrics`
- 报告中不包含 `Logs`

### 最低验收标准

- triage 不应错误 fanout 到无关 specialist

---

## Case 3：日志故障排查

### 输入

`请排查支付服务日志错误`

### 预期 intent

`incident_analysis`

### 预期 domains

- `logs`
- `knowledge`

### 关键输出检查点

- 报告中包含 `Logs`
- 报告中包含 `Knowledge`
- 报告中不包含 `Metrics`

### 最低验收标准

- triage 识别为日志排障场景
- supervisor 只调用必要 specialist

---

## 4. 当前实现映射

当前这些 case 已由自动化测试覆盖：

- [supervisor_replay_test.go](/Users/agiuser/Agent/OpsCaptionAI/internal/ai/agent/supervisor/supervisor_replay_test.go)

说明：

- 当前 replay 基线仍使用 stub specialist，以保证编排逻辑稳定可测
- 后续可逐步扩展为：
  - 接真实 `Log Agent`
  - 接真实 `Metrics Agent`
  - 接真实 `Knowledge Agent`
  - 引入 artifact / detail / evidence 数量断言

---

## 5. 后续扩展建议

建议下一步增加：

1. MCP 不可用时的降级 case
2. Prometheus 空结果 case
3. 知识库空结果 case
4. 多工具部分失败但 overall succeeded 的 case
5. memory 注入命中 case

