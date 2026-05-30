# GoS Belief Engine 当前进度与推进闭环

日期：2026-05-18
分支：`feature/gos-belief-engine`
当前阶段：Phase 3 评测闭环收口，尚未接入 AIOps 路由

## 1. 当前结论

GoS Belief Engine 已经具备最小可运行闭环：

- `belief` 图结构和 FSM 已落地。
- `gos_engine` 主循环已落地，graph/fsm 是 per-run 局部状态。
- `experts` 已接入 ToolAdapter、RAG 注入、真实 LLM content 生成路径。
- `cmd/gos_eval` 已拆成 `gos`、`baseline`、`compare`、`smoke` 四种模式。
- `compare` 模式强制使用 `--gos-profile=real`，不允许 eval profile 自证。
- `baseline` 模式调用真实 `plan_execute_replan.BuildPlanAgent`。
- `smoke` 只作为开发回归，不作为 Phase 3 gate。
- Planner 已支持 `max_experts_per_round`，默认每轮只跑 1 个专家。
- 模型初始化已识别未解析的 `${DEEPSEEK_API_KEY}`，避免拿占位符请求远端。

当前还不能直接替换 AIOps 主链路。必须先跑通 real compare，并确认严格 gate 通过。

## 2. 已修复的关键问题

### 2.1 评测自证问题

之前 `compare` 可能用 eval-only fake tool、fake RAG、eval mapping 跑 GoS，导致 holdout 结果被规则匹配自证。

当前处理：

- `--mode=compare` 只接受 `--gos-profile=real`。
- `--mode=gos --gos-profile=eval` 只用于本地开发观察 metrics。
- `--mode=smoke` 明确标注为开发回归。

### 2.2 生产专家不是 LLM 分析的问题

之前生产 `BaseExpert.generateContent` 仍是模板路径。

当前处理：

- 生产默认走真实 chat model。
- eval/test 通过 `GenerateContentFunc` 或 `ChatModelFactory` 注入 fake。
- 工具、RAG、LLM 调用都有 timeout。
- 工具/RAG 错误走 degraded，不直接 fatal。

### 2.3 LLM 调用次数虚高

之前 GoS 把 ingest、plan、FSM decide 等确定性步骤也计入 LLM calls。

当前处理：

- 只统计专家 content generation 这类真实 LLM 调用。
- Planner 增加 `max_experts_per_round`。
- 默认每轮先跑 1 个专家，避免一上来 3 专家扇出。

本地 eval profile 结果已经从平均 `21` 次 LLM 调用降到约 `3`。

### 2.4 本地配置缺失时误打远端

本地没有有效 `DEEPSEEK_API_KEY` 时，配置中的 `${DEEPSEEK_API_KEY}` 之前会被原样传给 DeepSeek，最终表现为 401。

当前处理：

- chat model 初始化会校验 model、api_key、base_url 是否为空或未解析。
- 未解析时直接返回配置错误。
- real compare 会快速 degraded，不再等待远端 401 重试。

## 3. 评测模式说明

### 3.1 GoS metrics

只看 GoS 自身结果，不判定 gate。

```bash
go run ./cmd/gos_eval \
  --mode=gos \
  --gos-profile=eval \
  --holdout=internal/ai/agent/gos_engine/eval/testdata/holdout.json \
  --output=/tmp/gos_eval.json
```

用途：

- 本地快速回归。
- 观察 accuracy、evidence coverage、LLM calls、traceability。
- 不作为上线依据。

### 3.2 Baseline artifact

跑真实 Plan-Execute-Replan，并输出 baseline artifact。

```bash
go run ./cmd/gos_eval \
  --mode=baseline \
  --holdout=internal/ai/agent/gos_engine/eval/testdata/holdout.json \
  --output=baseline_result.json
```

要求：

- 需要真实 LLM 环境。
- 需要真实工具/RAG 配置。
- 生成的 artifact 必须和当前 holdout 对齐。

### 3.3 GoS vs Baseline compare

Phase 3 gate 只看这个模式。

```bash
go run ./cmd/gos_eval \
  --mode=compare \
  --gos-profile=real \
  --holdout=internal/ai/agent/gos_engine/eval/testdata/holdout.json \
  --baseline=baseline_result.json \
  --output=eval_compare_real_result.json
```

严格 gate：

- accuracy 不低于 baseline。
- evidence coverage 不低于 baseline。
- latency 不超过 baseline 的 1.5 倍。
- LLM calls 不超过 baseline 的 2 倍。
- degradation rate 不高于 baseline。
- traceability 必须保持完整。

## 4. 当前推进优先级

### P0：跑通 real compare

先确认真实依赖下 GoS 的实际表现，不再看 eval-only 结果做准入判断。

如果 real compare 失败，按失败项处理：

- `llm_calls` 失败：继续压缩专家扇出和 retrieval steps。
- `latency` 失败：缩短 timeout，减少工具/RAG 调用轮次。
- `accuracy` 失败：改专家 prompt 和证据聚合，不改 holdout 标签。
- `degradation_rate` 失败：优先修工具/RAG fallback。

### P1：接入 AIOps 路由但默认关闭

只有 real compare 通过后再做。

建议配置：

```yaml
aiops:
  engine: plan_execute_replan
  gos:
    enabled: false
    max_experts_per_round: 1
    max_retrieval_steps: 3
    call_timeout_ms: 5000
```

接入原则：

- 默认仍走 Plan-Execute-Replan。
- GoS 只能通过 feature flag 打开。
- GoS 返回 `protocol.TaskResult`。
- 任一工具、RAG、LLM、图更新异常都返回 degraded，不让服务崩。

### P2：线上灰度

灰度顺序：

1. 本地 eval profile smoke。
2. 服务器 baseline artifact。
3. 服务器 real compare。
4. 内部小流量打开 GoS。
5. 观察 latency、degradation、evidence、trace。

## 5. 当前风险

- `cmd/gos_eval` 仍包含 eval-only fake tool/RAG，用于 smoke，不能被误认为生产链路。
- real profile 依赖真实 LLM、Milvus/RAG、MCP log tool，环境缺失会导致 degraded。
- GoS 尚未接入 `manifest/config/config.yaml` 和 AIOps service routing。
- Planner 当前只是按配置顺序选专家，不是智能调度。
- baseline 的 LLM calls 目前用 detail steps 近似，后续最好接真实 callback 统计。

## 6. 下一步执行命令

先跑开发回归：

```bash
go test ./internal/ai/belief ./internal/ai/agent/gos_engine/... ./internal/ai/agent/experts ./internal/ai/agent/plan_execute_replan -count=1
go run ./cmd/gos_eval --mode=gos --gos-profile=eval --holdout=internal/ai/agent/gos_engine/eval/testdata/holdout.json --output=/tmp/gos_eval.json
```

再跑真实对比：

```bash
go run ./cmd/gos_eval --mode=compare --gos-profile=real --holdout=internal/ai/agent/gos_engine/eval/testdata/holdout.json --baseline=baseline_result.json --output=eval_compare_real_result.json
```

如果真实对比通过，再推进 AIOps 路由接入；如果失败，先修失败 gate，不先接路由。

## 7. 本轮实际验证结果

本地开发回归：

```text
GoS profile: eval
MaxExpertsPerRound: 1
Accuracy: 100%
Evidence coverage: 100%
Avg LLM calls: 3.0
Degradation rate: 0%
Traceability: 100%
```

本地 real compare：

```text
GoS profile: real
Baseline artifact: baseline_result.latest.json
Result: gate failed
原因: 本地没有有效 DEEPSEEK_API_KEY，GoS 全部 degraded
```

这个结果不是算法失败，而是本地真实依赖未加载。下一步应在有真实 `.env.production` 或服务器容器环境中跑：

```bash
go run ./cmd/gos_eval --mode=compare --gos-profile=real --holdout=/app/holdout.json --baseline=/app/baseline_result.json --output=/app/eval_compare_real_result.json
```
