# Harness MVP 实施记录

> 当前统一口径（2026-05）
> - 本文是历史 Harness MVP 落地记录，保留当时的角色命名和测试输出。
> - 当前实现请以 `Chat ReAct` 与 `AIOps Runtime + Plan-Execute-Replan` 为准。

> 实施日期：2026-04-29
> 范围：第一优先级验证闭环 + 统一配置

---

## 1. 交付清单

### 新增文件

| 文件 | 行数 | 职责 |
|---|---|---|
| `internal/ai/protocol/validate.go` | 55 | TaskResult Schema Gate — 结构完整性校验 |
| `internal/ai/protocol/validate_test.go` | 157 | Schema Gate 测试 — 14 个 case |
| `internal/ai/agent/contracts/enforce.go` | 57 | 运行时 Contract Enforce — 契约校验 + 自动降级 |
| `internal/ai/agent/contracts/enforce_test.go` | 114 | Enforce 测试 — 13 个 case |

### 修改文件

| 文件 | 变更 |
|---|---|
| `manifest/config/config.yaml` | 新增 `harness` 配置段 |

---

## 2. Schema Gate（protocol/validate.go）

**作用**：运行时检查 TaskResult 的结构完整性。

**校验规则**：

| 检查项 | 规则 |
|---|---|
| 空指针 | result 不能为 nil |
| task_id | 必须非空 |
| agent | 必须非空 |
| status | 必须是 succeeded/failed/degraded |
| summary | 必须非空，≤ 4096 字符 |
| degraded | 必须有 DegradationReason |
| failed | 必须有 Error |
| confidence | 必须在 [0.0, 1.0] |
| evidence | 每条必须有 source_type 和 title |
| error | 必须有 code 或 message |

**API**：
```go
func ValidateTaskResult(r *TaskResult) error
```

**测试覆盖**：14 个 case，覆盖正常/异常/边界/空值。

---

## 3. Contract Enforce（contracts/enforce.go）

**作用**：在 Schema Gate 基础上，进一步按 Agent Contract 校验。

**两层校验**：
1. `ValidateTaskResult` — 结构完整性
2. `ValidateAgainstContract` — 契约符合性（匹配 contracts.go 定义的 Must/MustNot）

**EnforceContract 行为**：
- 校验通过 → 原样返回
- 校验失败 → 自动降级为 degraded，置信度折半，reason 说明原因
- 已有 DegradationReason → 追加 enforcement 错误信息

**API**：
```go
func ValidateAgainstContract(result *TaskResult) error
func EnforceContract(result *TaskResult) *TaskResult
```

**测试覆盖**：13 个 case，覆盖正常通过/缺少字段/非法状态/置信度折半/nil 输入/reason 追加。

---

## 4. 统一配置

`manifest/config/config.yaml` 新增：

```yaml
harness:
  max_iterations: 10          # Agent 最大迭代次数
  task_timeout_ms: 300000     # 单任务超时 5min
  retry_budget: 3             # 失败重试次数
  fail_fast: false            # false: 其他 specialist 继续执行
  validation:
    enabled: true             # Schema Gate 总开关
    contract_enforce: true    # Contract 校验开关
    strict_mode: false        # false: 校验失败降级; true: 拒绝
```

---

## 5. 调用链路

```
Agent 执行完毕 → TaskResult 返回
    │
    ▼
EnforceContract(result)
    ├── ValidateTaskResult(result)    ← 结构完整性
    ├── ValidateAgainstContract(result) ← 契约符合性
    └── 失败 → status=degraded, confidence×0.5
    │
    ▼
Reporter 接收（可能已降级的）result
```

---

## 6. 测试结果

```
=== protocol (14 tests) ===
TestValidateTaskResult_Nil                    PASS
TestValidateTaskResult_ValidSucceeded         PASS
TestValidateTaskResult_ValidDegraded          PASS
TestValidateTaskResult_ValidFailed            PASS
TestValidateTaskResult_EmptyTaskID            PASS
TestValidateTaskResult_EmptyAgent             PASS
TestValidateTaskResult_InvalidStatus          PASS
TestValidateTaskResult_EmptySummary           PASS
TestValidateTaskResult_SummaryTooLong         PASS
TestValidateTaskResult_DegradedNoReason       PASS
TestValidateTaskResult_FailedNoError          PASS
TestValidateTaskResult_InvalidConfidence      PASS
TestValidateTaskResult_EvidenceMissing*       PASS (×2)
TestValidateTaskResult_ErrorEmptyCodeAndMsg   PASS
TestValidateTaskResult_EmptyEvidenceOK        PASS

=== contracts (13 tests) ===
TestValidateAgainstContract_Nil               PASS
TestValidateAgainstContract_UnknownAgent      PASS
TestValidateAgainstContract_ValidTriage       PASS
TestValidateAgainstContract_ValidMetrics      PASS
TestValidateAgainstContract_DegradedMissing*  PASS
TestEnforceContract_NormalPasses              PASS
TestEnforceContract_DegradesOnInvalid         PASS
TestEnforceContract_DegradesInvalidStatus     PASS
TestEnforceContract_ConfidenceHalved          PASS
TestEnforceContract_NilReturnsNil             PASS
TestEnforceContract_AppendsDegradationReason  PASS
TestAllContracts_RegisteredAgents             PASS (contracts_test.go)
TestContract_ValidPrompt                      PASS (contracts_test.go)

总计: 27/27 PASS
```

---

## 7. 未完成项

| 事项 | 原因 | 后续计划 |
|---|---|---|
| `tools/contract_test.go` | `bytedance/sonic v1.14.1` 与 Go 1.26 不兼容，整个 tools 包无法编译 | 升级 sonic 后补上 |
| Runtime 集成 EnforceContract | 需要在线服务环境验证 | 后续集成测试时补上 |
| Replay 用例扩充 | 第二批优先级 | 按 05-运维加固设计执行 |

---

## 8. 兼容性

- ✅ 不改任何现有代码逻辑
- ✅ 不改 go.mod
- ✅ 不改 TaskResult 结构体
- ✅ 新增配置字段有合理默认值，不设也能跑
- ⚠️ sonic 兼容性问题为环境预置问题，不影响本次交付的正确性
