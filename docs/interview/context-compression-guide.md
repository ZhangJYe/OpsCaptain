# 上下文压缩系统 — 实验记录与面试准备

> 主题：在 OpsCaptain 中评估 Headroom-like 上下文压缩，降低工具输出和 RAG 文档的 token 消耗，同时不损害 AIOps 诊断证据。

## 1. 背景与问题

OpsCaptain 的主链路是：

```text
用户问题 -> ContextEngine / MemoryService -> Eino ReAct Agent -> Tools / RAG -> JSON / SSE
```

在 AIOps 场景里，LLM 经常收到大块上下文：

- Prometheus 查询结果：大量正常时序或聚合值，只有少量异常项有价值。
- 日志查询结果：大多数是 INFO/DEBUG，真正关键的是 ERROR/WARN/timeout/OOM 附近几行。
- RAG 文档：文档正文可能很长，但诊断只需要 source/title/score 和相关 evidence snippet。

原来的保护主要是 `events.tool_summary_max_len=4000` 的长度截断，以及 ContextEngine 的 token budget 裁剪。它们能防止上下文爆炸，但问题是：

- 暴力截断可能切掉中间位置的关键错误。
- token 浪费在大量正常行上。
- 截断没有 evidence 保留率指标，难以评估优化是否伤害诊断。

所以这次实验的目标不是“压得越短越好”，而是：

```text
在 evidence 不下降、诊断不退化的前提下，减少 prompt token。
```

## 2. Headroom 调研结论

调研对象：[chopratejas/headroom](https://github.com/chopratejas/headroom)。

Headroom 的关键思想值得借鉴：

- SmartCrusher：结构化压缩 JSON / logs / tool outputs，保留异常、错误、首尾样本和 query 相关项。
- CCR：压缩后保留原文 hash，必要时可回溯检索原始内容。
- Cache alignment：尽量稳定 prompt 前缀，减少 provider cache bust。

但没有直接把 Headroom 作为生产核心依赖，原因是：

- OpsCaptain 是 Go + Eino + GoFrame，Headroom 主体是 Python/Rust/TypeScript，直接引入会增加部署和排障复杂度。
- AIOps 诊断要求 evidence 可追溯，外部 proxy 一旦压缩或回取失败，可能影响主链路。
- 项目规则要求新增能力可配置、失败降级、tool 走 registry/timeout/权限边界；直接接入外部 MCP/proxy 不够贴合。

面试表达：

> 我没有把 Headroom 当成“拿来就接”的依赖，而是把它作为设计参考。对 OpsCaptain 来说，核心场景是运维日志、Prometheus JSON、RAG 片段，我先用 Go 原生做轻量压缩实验，保留证据和降级语义，避免把主链路绑到跨语言 proxy 上。

## 3. 实现设计

新增模块：

```text
internal/ai/contextcompression/
├── compressor.go           # Compress 入口、Request/Result/Report
├── json_compressor.go      # JSON 数组/对象压缩
├── log_compressor.go       # 日志/文本压缩
├── config.go               # config.yaml 配置加载
├── *_test.go               # 单元测试
```

新增评测命令：

```text
internal/ai/cmd/context_compression_eval_cmd
```

配置位于 `manifest/config/config.yaml`：

```yaml
context_compression:
  enabled: false
  mode: "audit"              # off | audit | optimize
  min_tokens: 300
  preserve_first: 3
  preserve_last: 2
  log_context_lines: 1
  source_types: ["tool", "rag"]
```

默认关闭，生产行为不变。

### 3.1 两种模式

`audit` 模式：

- 执行压缩候选分析。
- 记录 candidate token / evidence。
- 不改变 runtime 内容。
- 用于无风险观察收益潜力。

`optimize` 模式：

- 只有压缩后 token 确实变少时才替换内容。
- 如果压缩无收益或失败，保留原文。
- 后续仍经过 ContextEngine budget 裁剪。

这次 review 后专门修正了一个重要口径：audit 的“候选收益”和“实际 runtime 收益”必须分开记录。

### 3.2 压缩策略

JSON 数组：

- 保留前 `preserve_first` 项。
- 保留后 `preserve_last` 项。
- 保留包含错误关键词的项：`error / warning / timeout / 503 / OOM / 告警 / 超时 / 异常`。
- 保留命中 query term 的项。

日志/文本：

- 保留 ERROR/FATAL/PANIC/OOM/5xx/中文异常关键词所在行。
- 保留上下文窗口。
- 保留 WARN/ALERT 行。
- 保留 query 命中行。
- 保留首尾行。
- 去重连续重复行。

JSON 对象：

- 提取 `error / message / status / code / reason / detail / data / result` 等关键字段。
- 只在有关键字段时产生候选压缩。

### 3.3 RAG 文档压缩：不等于检索压缩

RAG 文档压缩经常被误解为"压缩检索过程"或"压缩召回率"。实际上，压缩发生在检索之后，不影响召回率。

完整流程：

```text
用户问题
    ↓
RAG 检索（Milvus 向量搜索）← 这里决定召回率
    ↓
检索到的文档（比如 7 个，共 10K tokens）
    ↓
压缩文档（10K → 2K tokens）← 压缩在这里
    ↓
发送给 LLM 生成答案
```

压缩什么？

压缩的是检索到的文档内容，不是检索过程：

| 阶段 | 压缩前 | 压缩后 |
|------|--------|--------|
| 检索 | 从 2,104 个文档中找到 7 个 | 不变 |
| 文档内容 | 7 个文档完整内容（10K tokens） | 保留关键证据，去掉冗余（2K tokens） |
| 召回率 | 7/7 相关文档 | 仍然是 7/7 |

举例说明：

假设用户问："Director 如何部署？"

压缩前（10K tokens）：

```text
[doc-1] 部署指南.md（5K tokens）
  完整的部署步骤、注意事项、截图说明...

[doc-2] 性能计数器.md（3K tokens）
  C100050001 物理机总量
  C100050002 物理机总量最大值
  ...（200 个指标定义）

[doc-3] 安全配置.md（2K tokens）
  用户管理、角色配置、密码策略...
```

压缩后（2K tokens）：

```text
[doc-1] 部署指南.md（1.5K tokens）
  保留：部署步骤、关键命令、常见错误
  去掉：截图说明、重复的注意事项

[doc-2] 性能计数器.md（200 tokens）
  保留：与部署相关的指标（CPU、内存使用率）
  去掉：其他 190 个无关指标

[doc-3] 安全配置.md（300 tokens）
  保留：部署需要的权限配置
  去掉：详细的密码策略、角色管理
```

关键点：

1. **召回率不变**：检索到的文档数量和质量不变
2. **证据保留率 100%**：压缩后仍保留所有关键证据
3. **节省 token**：减少发送给 LLM 的内容，降低成本
4. **可能提升质量**：去掉冗余后，LLM 更容易找到关键信息

面试表达：

> "RAG 文档压缩是在检索之后、发送给 LLM 之前进行的。它不影响召回率，因为压缩发生在检索完成之后。压缩的目的是减少 token 消耗，同时保留所有关键证据。我们的测试显示，真实文档的压缩率能达到 78.81%，但证据保留率仍然是 100%。"

## 4. 接入点

这次接入了三层：

1. Tool wrapper 层  
   `internal/ai/agent/chat_pipeline/flow.go` 已从 `SummaryAfterToolCall` 改为 `CompressAfterToolCall`。

2. ContextEngine tool items  
   `selectToolItems` 对 tool evidence / summary 做 audit 或 optimize，然后再走 token budget。

3. RAG documents  
   RAG doc 转成 `ContextItem` 后先做 audit/optimize，再进入 document budget 裁剪。

修复后的关键行为：

- 关闭压缩时，仍保留旧的 `SummaryAfterToolCall(maxLen)` 截断行为。
- audit 模式不改变 runtime 内容。
- optimize 模式只在 `candidate_tokens_after < tokens_before` 时替换。
- 不再用 `context.Background()` 丢失请求上下文。
- 不在服务主路径用 `fmt.Printf` 打压缩日志，改成框架 logger。

## 5. 实验设计

样本文件：

```text
evals/context_compression/samples.jsonl
```

评测命令：

```bash
go run ./internal/ai/cmd/context_compression_eval_cmd \
  -input evals/context_compression/samples.jsonl \
  -mode audit,optimize \
  -out evals/runs/compression_eval.json
```

Makefile：

```bash
make eval-compression-audit
make eval-compression
```

报告区分两类收益：

| 字段 | 含义 |
|------|------|
| Candidate Save | 压缩器候选输出能省多少 token |
| Runtime Save | 当前模式真正送入下游的内容省了多少 token |
| Candidate Evidence Recall | 候选压缩内容保留了多少 required evidence |
| Runtime Evidence Recall | 实际 runtime 内容保留了多少 required evidence |

这个区分很关键：audit 模式应该有 candidate saving，但 runtime saving 必须是 0，因为 audit 不改变内容。

### 5.1 数据集扩充记录

初版只有 11 条 smoke 样本，覆盖 Prometheus JSON、日志、MySQL、RAG 文档、重复 ERROR 日志、短内容。这能验证代码能跑通，但不够支撑“运维场景有效”的面试结论。

这次把样本扩展到 32 条，新增 21 条合成运维样本。数据构造原则：

- 不使用生产真实 secret、内网 IP、真实 token。
- 场景参考公开运维文档和常见排障形态：Kubernetes Pod debug / 镜像拉取 / probe、Prometheus Alertmanager、OpenTelemetry logs、etcd metrics。
- 内容为合成 payload，但保留真实工具输出的结构特征，例如 `kubectl describe pod` events、Prometheus alert JSON、OpenTelemetry structured log、RAG runbook 段落。
- 每条样本都标注 `required_evidence`，评测不靠主观读结果，而是检查关键证据是否仍在压缩后内容里。

参考材料：

- Kubernetes Debug Running Pods: https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/
- Kubernetes Images: https://kubernetes.io/docs/concepts/containers/images/
- Prometheus Alerting Overview: https://prometheus.io/docs/alerting/latest/overview/
- OpenTelemetry Logs: https://opentelemetry.io/docs/concepts/signals/logs/
- etcd Metrics: https://etcd.io/docs/v3.5/metrics/

### 5.2 样本覆盖矩阵

| 类别 | 样本数 | 代表样本 | 主要验证点 |
|------|--------|----------|------------|
| Prometheus / Alert JSON | 6 | `prom-001`, `alertmanager-001`, `slo-burn-001` | JSON 数组、告警状态、实例标签、SLO burn rate |
| Kubernetes Pod / Events | 4 | `k8s-crash-001`, `k8s-imagepull-001`, `k8s-probe-001` | CrashLoopBackOff、ImagePullBackOff、probe failed、OOMKilled |
| Control Plane / etcd | 2 | `etcd-001`, `etcd-log-001` | leader change、fsync latency、proposal pending |
| Application Logs | 7 | `log-001`, `log-004`, `otel-log-001`, `nginx-504-001` | ERROR/WARN、5xx、trace_id、timeout、重复日志 |
| 数据库 / 中间件 | 4 | `mysql-deadlock-001`, `redis-lock-001`, `rabbitmq-001` | deadlock、锁等待、队列积压、内存水位 |
| 发布系统 | 3 | `argocd-sync-001`, `helm-rollback-001`, `deployment-rollout-001` | sync failed、rollback、rollout unavailable |
| LLM / Agent 运行态 | 1 | `llm-rate-001` | rate limit、retry_after、degraded |
| RAG Runbook 文档 | 5 | `rag-001`, `rag-k8s-001`, `rag-etcd-001`, `rag-argocd-001` | 长文档中保留与 query 相关证据 |

### 5.3 门禁口径

当前阶段是离线压缩器评测，不声称已经证明线上端到端效果。门禁分两层：

离线压缩器门禁：

```text
required evidence recall >= 95%
degraded = 0
compression p95 <= 100ms
optimize 只在 tokens_after < tokens_before 时替换
```

上线前端到端门禁：

```text
RAG Recall@K / MRR 不低于 baseline
answer accuracy 不低于 baseline
evidence coverage 不低于 baseline
ResultStatusDegraded 不上升
prompt tokens 有可观下降
```

面试表达：

> 我先用 11 条 smoke 样本把压缩链路、report 和测试跑通。后来意识到这不够支撑效果结论，就扩到 32 条，覆盖 metrics、logs、Kubernetes events、release、middleware、RAG runbook。扩容后平均 saving 从 8.17% 降到 4.65%，但这是更可信的结果，因为它包含了大量低收益和无收益样本。

## 6. 复跑结果

### 6.1 第一版 11 样本结果

第一版用于验证最小闭环：

```text
Cases          : 11
Optimize Runtime Save: 8.17%
Runtime Evidence Recall: 100.00%
Degraded       : 0
```

问题：样本数量太少，且高收益样本占比偏高，不能代表真实运维分布。

### 6.2 扩展到 32 样本后的结果

复跑命令：

```bash
go test ./internal/ai/contextcompression

go run ./internal/ai/cmd/context_compression_eval_cmd \
  -input evals/context_compression/samples.jsonl \
  -mode audit,optimize \
  -out evals/runs/compression_eval.json
```

Audit 结果：

```text
Cases          : 32
Avg Before     : 244 tokens
Candidate After: 231 tokens
Runtime After  : 244 tokens
Candidate Save : 4.65%
Runtime Save   : 0.00%
P95 Latency    : 0 ms
Candidate Evidence Recall: 100.00%
Runtime Evidence Recall  : 100.00%
Degraded       : 0
Strategies     : map[below_min_tokens:3 json_array:9 log:19 log_fallback:1]
```

Optimize 结果：

```text
Cases          : 32
Avg Before     : 244 tokens
Candidate After: 231 tokens
Runtime After  : 231 tokens
Candidate Save : 4.65%
Runtime Save   : 4.65%
P95 Latency    : 0 ms
Candidate Evidence Recall: 100.00%
Runtime Evidence Recall  : 100.00%
Degraded       : 0
Strategies     : map[below_min_tokens:3 json_array_no_savings:9 log:9 log_fallback_no_savings:1 log_no_savings:10]
```

### 6.3 关键样本观察

```text
rag-001: 583 -> 413 tokens，saving 29%，evidence recall 100%
log-005: 116 -> 55 tokens，saving 53%，evidence recall 100%
k8s-crash-001: 282 -> 199 tokens，saving 29%，evidence recall 100%
k8s-imagepull-001: 268 -> 255 tokens，saving 5%，evidence recall 100%
deployment-rollout-001: 216 -> 195 tokens，saving 10%，evidence recall 100%
```

解释口径：

- 32 样本的平均收益 4.65% 低于 11 样本的 8.17%，这是合理的，因为扩展样本里包含更多短内容、结构化告警、低冗余日志和无收益 JSON。
- `json_array_no_savings` 和 `log_no_savings` 是预期行为，不是失败。它说明 optimize 模式会识别“压缩无收益”，并保留原文。
- evidence recall 保持 100%，比 saving 更重要。AIOps 场景中压错比少省 token 更危险。
- 当前结论应表述为“离线压缩策略在扩展合成运维样本上没有丢 required evidence，并能在部分长日志 / RAG 场景节省 token”，不要夸大为“线上一定节省 4.65%”。

### 6.4 扩样本带来的真实发现

扩容后第一次复跑时，`k8s-imagepull-001` 的 evidence recall 只有 75%。原因是初版日志关键词偏传统应用日志，能识别 ERROR/WARN/timeout/OOM，但没有覆盖 Kubernetes events 里的证据词：

```text
ImagePullBackOff
ErrImagePull
manifest unknown
imagePullSecrets
```

修复动作：

- 把 `crashloopbackoff / imagepullbackoff / errimagepull / oomkilled / failedscheduling / failedmount / unhealthy / manifest unknown / imagepullsecrets` 加入运维证据关键词。
- 新增 `TestCompressLog_KubernetesEventEvidence`，防止后续回归。
- 复跑 32 样本，candidate/runtime evidence recall 回到 100%。

面试表达：

> 扩样本的价值不只是让数字更好看，反而是把问题暴露出来。第一次扩到 32 条时，ImagePullBackOff 样本掉了证据召回，我发现压缩器只懂传统 ERROR 日志，不懂 Kubernetes events。于是补了 K8s 证据关键词和回归测试，复跑后 recall 回到 100%。这说明实验不是自证成功，而是在驱动策略迭代。

### 6.5 真实文档压缩：CCF AIOps 数据集

为了验证压缩在真实运维文档上的效果，使用 2024 CCF 国际 AIOps 挑战赛数据集（ZTE 电信 Director 云管平台文档）进行测试。

数据来源：

- CCF AIOps 2024 挑战赛 director 文档包（3,192 个 HTML 页面）
- 经过清洗（去空文件、去导航页、去重）后保留 2,104 个有效文档
- 文档类型：部署指南、性能计数器定义、安全配置、对接指南、告警规范等

测试样本构造：

模拟 RAG 返回场景，每个样本包含 7 个文档拼接（3 个相关 + 4 个不相关），查询与文档通过关键词匹配关联。

```bash
go run ./internal/ai/cmd/context_compression_eval_cmd \
  -input evals/context_compression/ccf_rag_samples.jsonl \
  -mode optimize
```

Optimize 结果：

```text
Cases          : 5
Avg Before     : 10,108 tokens
Candidate After: 2,081 tokens
Runtime After  : 2,081 tokens
Candidate Save : 78.81%
Runtime Save   : 78.81%
P95 Latency    : 0 ms
Candidate Evidence Recall: 100.00%
Runtime Evidence Recall  : 100.00%
Degraded       : 0
Strategies     : map[log:5]
```

各样本详情：

| 样本 | 原始 tokens | 压缩后 tokens | 压缩率 | 相关文档数 |
|------|-------------|---------------|--------|-----------|
| ccf-deployment | 11,029 | 3,952 | 64% | 3/7 |
| ccf-metrics | 12,279 | 483 | 96% | 3/7 |
| ccf-security | 12,927 | 2,837 | 78% | 3/7 |
| ccf-integration | 10,305 | 2,233 | 78% | 3/7 |
| ccf-alerting | 4,001 | 903 | 77% | 3/7 |

关键观察：

1. **真实文档压缩率远高于合成样本**：78.81% vs 4.65%，因为真实文档内容更长、冗余更多
2. **100% 证据保留**：所有压缩后的文档仍包含查询相关的关键词和证据
3. **log 策略通用性好**：所有样本都使用 log 策略成功压缩，说明该策略对结构化文档同样有效
4. **ccf-metrics 样本压缩率 96%**：因为性能计数器定义文档格式高度重复，压缩效果极佳

与合成样本的差异：

| 指标 | 合成样本 (32条) | 真实文档 (5条) |
|------|----------------|----------------|
| 平均原始 tokens | 244 | 10,108 |
| 压缩率 | 4.65% | 78.81% |
| 证据保留率 | 100% | 100% |
| 主要策略 | log/json_array | log |

结论：

- 合成样本的低压缩率是因为样本本身就很短（平均 244 tokens），压缩空间有限
- 真实 RAG 文档通常很长（平均 10K tokens），压缩收益显著
- 压缩系统在两种场景下都能保持 100% 证据保留率

面试表达：

> 我用 CCF AIOps 挑战赛的真实电信文档做了进一步验证。这些是 ZTE Director 云管平台的部署指南、性能指标定义、安全配置等文档，平均长度 10K tokens。测试结果显示压缩率 78.81%，远高于合成样本的 4.65%，因为真实文档内容更长、冗余更多。更重要的是，证据保留率仍然是 100%。这说明压缩系统在真实运维场景下确实有显著价值。

## 7. Review 中发现并修复的问题

初版实现后做了一轮 review，发现几个会让结论偏乐观的问题：

1. Tool wrapper 没有真正接入  
   新增了 `CompressAfterToolCall`，但 chat pipeline 还在调用旧的 `SummaryAfterToolCall`。

2. audit 和 optimize 报告相同  
   初版报告把“候选压缩 after tokens”当作 audit 实际收益，导致 audit 看起来也省 token。

3. 只在超 budget 时压缩  
   ContextEngine 初版只把压缩当作超预算救火，和 eval 中“所有样本直接压缩”的口径不一致。

4. 压缩可能无收益还被替换  
   初版 optimize 对部分路径没有统一检查 `CompressionRatio < 1.0`。

5. 请求上下文丢失  
   初版在 tool items 压缩中用了 `context.Background()`。

修复结果：

- Tool wrapper 已接线。
- eval 报告拆分 candidate/runtime。
- ContextEngine 对符合配置的 item 先做压缩候选，再进入 budget。
- optimize 只在真正省 token 时替换。
- 所有接入点使用请求 ctx。

面试表达：

> 我不是只实现完就说成功，而是复核了实验口径。初版最大的问题是 audit 报告容易误导，因为它展示的是候选压缩收益，不是实际 runtime 收益。我把报告拆成 candidate 和 runtime 两套指标，audit 的 runtime saving 回到 0%，这样结论才可信。

## 8. 面试 Q&A

### Q1：为什么做上下文压缩？

因为 AIOps 工具输出天然很长，但关键信息很稀疏。简单截断会把证据切掉，直接影响诊断可信度。上下文压缩的目标是保留错误、告警、query 命中项，同时减少无关正常内容。

### Q2：你的核心指标是什么？

我不只看 token saving，主要看四类指标：

- Candidate / Runtime token saving。
- Candidate / Runtime evidence recall。
- P95 compression latency。
- 端到端 accuracy / evidence coverage / degradation rate。

当前扩展样本结果是 optimize runtime saving 4.65%、evidence recall 100%、degraded 0。这个结果比 11 样本的 8.17% 更可信，因为覆盖了更多低收益和无收益运维输出。

### Q3：为什么 audit 和 optimize 要分开？

audit 用来无风险观察“如果压缩会怎样”，不能改变送入 LLM 的内容；optimize 才真正替换上下文。分开后可以先在线上采集压缩潜力和 evidence recall，再决定是否打开实际压缩。

### Q4：怎么防止压缩丢证据？

三层保护：

- 策略上保留 error/warning/timeout/503/OOM/中文异常关键词和 query 命中项。
- 评测上计算 required evidence recall。
- 工程上 optimize 失败或无收益时 passthrough，不替换原文。

### Q5：为什么不用 LLM 做摘要？

V1 选择确定性规则，因为：

- 低延迟，无外部调用。
- 可预测，便于测试。
- 不会生成不存在的结论。
- 更符合 evidence-first 的 AIOps 诊断要求。

LLM 摘要可以作为未来增强，但不能替代原始 evidence 保留。

### Q6：这个实验的不足是什么？

当前不足很明确：

- 32 条仍是合成运维样本，不是脱敏生产 holdout。
- 还没跑 RAG Recall@K / MRR A/B。
- 还没跑 GoS/AIOps end-to-end accuracy A/B。
- 暂时没有 CCR 原文回取能力。

所以我会把它讲成“完成了可验证的 V1 离线实验闭环”，而不是“已经证明线上收益”。

### Q7：下一步怎么验证到生产可用？

三步：

1. 扩大样本：继续收集脱敏真实 Prometheus、日志、RAG 输出各 30-50 条，形成 holdout。
2. RAG A/B：对比 compression off/optimize 下 Recall@K、MRR、citation coverage。
3. GoS/AIOps A/B：对比 accuracy、evidence coverage、degradation rate、prompt tokens。

上线前门槛：

```text
evidence recall >= 95%
accuracy 不低于 baseline
degradation rate 不上升
prompt tokens 有明显下降
compression p95 <= 100ms
```

### Q8：为什么扩样本后 saving 下降了，还是好事？

因为初版 11 条里高收益样本比例偏高，扩到 32 条后加入了更多真实会遇到的低冗余 payload，例如结构化 Alertmanager JSON、短事件、RAG runbook 中本来就相关的段落。平均 saving 从 8.17% 降到 4.65%，说明评测更接近真实分布。更重要的是 evidence recall 仍然是 100%，这比追求漂亮 saving 更关键。

### Q9：为什么有很多 no_savings？

`no_savings` 是故意设计的安全阀。压缩候选如果没有变短，optimize 不会替换 runtime 内容。这样能避免为了“压缩而压缩”导致 prompt 变长或结构变差。

## 9. 实验结果解读

### 9.1 结果数据

```text
Cases          : 32
Audit Runtime Save: 0.00%
Optimize Runtime Save: 4.65%
Runtime Evidence Recall: 100.00%
Degraded       : 0
P95 Latency    : 0 ms
```

### 9.2 逐项解释

**Cases: 32**

测试了 32 个样本，覆盖 Prometheus JSON、Kubernetes events、应用日志、数据库中间件、发布系统、RAG 文档等场景。初版只有 11 个样本，后来扩到 32 个，因为样本太少结论不可信。

**Audit Runtime Save: 0.00%**

audit 模式下，runtime 内容没有变化，所以 runtime saving 是 0%。这是正确的行为——audit 的意义是"观察如果压缩会怎样"，但不真的改变送入 LLM 的内容。如果 audit 显示有 saving，说明代码有 bug（把候选压缩结果当成了实际结果）。

**Optimize Runtime Save: 4.65%**

optimize 模式下，实际送入 LLM 的内容平均减少了 4.65%。这个数字不大，原因是：

- 32 个样本里有很多短内容（<300 tokens），压缩器直接跳过
- 结构化告警 JSON（如 Alertmanager）本身就不太冗余
- 只有长日志和 RAG 文档有明显收益（29%-53%）

**Runtime Evidence Recall: 100.00%**

压缩后的内容保留了所有 `required_evidence`。这是最重要的指标——宁可不压缩，也不能丢证据。比如 `k8s-crash-001` 样本，原始内容有 `CrashLoopBackOff`、`OOMKilled`、`exit code 137`，压缩后这些全在。

**Degraded: 0**

没有压缩失败的情况。压缩器遇到异常会 passthrough（返回原文），degraded 记录的是 passthrough 次数。0 次说明所有样本都正常处理。

**P95 Latency: 0 ms**

压缩是纯 CPU 操作（字符串匹配 + JSON 解析），42K 文档级别也是毫秒级。不涉及网络调用，所以延迟可以忽略。

### 9.3 关键样本观察

```text
rag-001:      583 → 413 tokens，saving 29%，保留了告警相关段落
log-005:      116 → 55 tokens，saving 53%，5 行重复 ERROR 去重为 1 行
k8s-crash-001: 282 → 199 tokens，saving 29%，保留了 CrashLoopBackOff 事件
```

低收益或无收益的样本（如短 JSON、结构化告警）不会被压缩，这是设计预期。

## 10. 压缩的真实价值：诚实评估

### 10.1 生产价值有限

压缩解决的问题是"工具返回太多数据，浪费 token"。但更好的解决办法是在工具层加 filter，从源头减少无关数据：

```text
原始查询: http_requests_total{job="paymentservice"}
优化查询: http_requests_total{job="paymentservice", status=~"5.."}
→ 直接只返回异常 metric，根本不需要压缩
```

压缩作为兜底策略有价值，但不是首选方案。

### 10.2 面试价值更大

| 维度 | 展示的能力 |
|------|-----------|
| 技术深度 | 分层架构设计（audit/optimize）、评测框架搭建 |
| 工程思维 | 关注 evidence 保留率、不盲目追求压缩率 |
| 问题拆解 | 把"降 token"拆成压缩、rerank、budget 三个子问题 |
| 量化验证 | 用数据说话，区分 candidate/runtime 指标 |
| 诚实态度 | 承认样本局限、不夸大生产收益 |

### 10.3 真正能提升 OpsCaptain 生产价值的是什么

1. **灌入更多知识库**（如 CCF AIOps 数据集 3,192 文档）— 让 RAG 有东西可搜
2. **工具输出优化** — 在 Prometheus/日志工具层加 filter，从源头减少无关数据
3. **Prompt 优化** — 更好的 system prompt，让 LLM 更会用工具
4. **RAG 文档压缩** — 真实文档平均 10K tokens，压缩率 78.81%，收益显著

压缩实验的价值是**学会怎么设计和评测一个系统优化**，而不是这个优化本身有多大的生产收益。

### 10.4 面试怎么说

> "压缩实验在合成样本上的线上 saving 是 4.65%，不大。但我用 CCF AIOps 真实文档测试后，发现真实 RAG 文档的压缩率能达到 78.81%，因为真实文档更长、冗余更多。这说明压缩在特定场景下确实有价值。更重要的是，我学会了怎么建立评测口径、怎么区分候选收益和实际收益、怎么用数据驱动迭代。"

## 11. STAR 讲法

Situation：

> OpsCaptain 是 AIOps 助手，工具和 RAG 输出经常很长。旧逻辑靠长度截断控制 token，但可能切掉关键错误和证据。

Task：

> 我要设计一个可配置、可回滚、可评测的上下文压缩实验，目标是在不损害 evidence 的前提下降低 prompt token。

Action：

> 我先调研 Headroom，抽取 SmartCrusher/CCR 思想，但没有直接引入外部 proxy。然后实现 Go 原生压缩器，支持 JSON 和日志两类高频 payload，接入 tool wrapper、ContextEngine tool items 和 RAG docs。为了避免实验结论失真，我把报告拆成 candidate/runtime 两套指标，并补了 audit/optimize 模式。

Result：

> 第一版 11 条 smoke 样本上，optimize runtime token saving 是 8.17%，required evidence recall 是 100%。后来我主动把样本扩到 32 条，覆盖 Kubernetes、Prometheus、etcd、OpenTelemetry、发布系统、中间件和 RAG runbook。扩样本后平均 saving 变成 4.65%，但 evidence recall 仍是 100%，degraded 为 0。过程中还发现并修复了 ImagePullBackOff 事件证据漏保留的问题，补了回归测试。
>
> 后来我用 CCF AIOps 挑战赛的真实电信文档做了进一步验证。这些是 ZTE Director 云管平台的部署指南、性能指标定义等文档，平均长度 10K tokens。测试结果显示压缩率 78.81%，远高于合成样本的 4.65%，因为真实文档更长、冗余更多。证据保留率仍然是 100%。

面试加分点：

> 这件事的重点不是写了一个压缩函数，而是建立了”优化不能伤证据”的评测口径。尤其 audit 模式必须区分候选收益和实际 runtime 收益，否则实验会看起来很好但没有工程意义。另外，用真实文档验证让我发现合成样本和真实场景的差异很大——合成样本平均 244 tokens，压缩空间有限；真实文档平均 10K tokens，压缩收益显著。

### 11.1 面试追问：这个压缩上线了吗？

诚实回答：

> “没有上线。合成样本的平均 saving 只有 4.65%，收益不大。但我用 CCF AIOps 真实文档测试后，发现真实 RAG 文档的压缩率能达到 78.81%，因为真实文档更长、冗余更多。不过我发现生产环境更好的做法是在工具层加 filter，比如 Prometheus 查询加 `status=~'5..'`，从源头减少无关数据，而不是在输出层压缩。压缩作为兜底策略有价值，但不是首选。这个实验的价值是让我学会了怎么建立评测口径、怎么区分候选收益和实际收益、怎么用数据驱动迭代。”

不要说”上线了省了 X% token”，面试官一追问细节就会露馅。诚实说”没上线，但学到了什么”反而加分。
