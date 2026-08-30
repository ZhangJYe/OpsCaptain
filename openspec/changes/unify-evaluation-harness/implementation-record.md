# Implementation Record

## 实施范围

- 新增统一 `eval_harness` CLI，支持 `validate`、`run`、`gate`、`compare`。
- 新增 Route、RAG、Plan、GoS、Tool、Evidence 六类 adapter；RAG 与 GoS 直接复用现有 evaluator 评分逻辑。
- 新增版本化 manifest/case/report、三层预算、公共/领域/跨链路 Gate、指纹、脱敏 JSON 与 Markdown 报告。
- CI 先运行新统一 Gate，再保留旧 `make eval-gate` 一个迁移周期；新增手动 recorded development 工作流。
- 未修改 Controller 或线上 Chat/AIOps 编排；未恢复 `chat_multi_agent`；Memory 仅被记录为存在，不参与原始 query 路由评分。

## 2026-08-13 验证记录

### 统一 PR Gate

- 命令：`make eval-harness-gate`
- 角色/Profile：`regression + deterministic`
- 结果：`succeeded`
- 运行：Route 3、RAG 2、Plan 1、GoS 1、Tool 3、Evidence 1，共 11 个固定 fixture case。
- 关键指标：Route Macro-F1 1.0、缓存命中率 0.3333、低置信度率 0.3333；RAG MRR 0.75、Recall@5 1.0、Hit@5 1.0；Plan 完成/步骤/重规划成功率均为 1.0；GoS 根因、Evidence precision/coverage、图有效性、Trace 与契约均为 1.0；Tool 契约/权限/降级符合率均为 1.0；Evidence claim support 与 citation traceability 均为 1.0。
- 跨链路 Gate：incident route 可关联 Plan/GoS、诊断 Trace、诊断 Evidence、权限拒绝不执行，全部通过。
- 最终指纹：dataset `6be143b58d25589f281f0315432cb92a2fccb2a3e45966dab5ae2f2507fa17f3`；evidence corpus `51018a75935f63e96a68a3f28035ec4cd8df9df15ce71575048f1a857318050b`。code/config 指纹随候选代码和 manifest 变化，报告逐次记录，不作为候选实现可变项的兼容键。
- CLI 冷/热缓存计时会波动；最终一次旁路计时为 `real 1.37s`，报告内实际执行约 8ms。普通 PR 依赖状态明确记录为 model fixture，Milvus/Prometheus/logs 均 `not_used`。

### 旧入口旁路验证

- 命令：`make eval-gate`
- 结果：通过；5/5 deterministic GoS case，根因准确率、Evidence precision/coverage、图有效性、Trace、契约均为 1.0，降级率与过早停止率均为 0。
- 计时：`real 3.49s`，旧报告内部执行约 4ms。
- 等价性：单元测试用同一 RAG/GoS fixture 对比 adapter 与原 evaluator，RAG MRR/Recall/Hit 和 GoS 根因、Evidence、图、回溯、降级指标保持一致。
- 差异：旧 Gate 使用 5 条旧 GoS smoke case；新 Harness 的 PR manifest 使用 1 条共享 Plan/GoS case，并额外覆盖其他五类 suite。两次命令的数据集不同，因此旁路结果只能证明兼容与评分复用，不能当作线上效果对比。

### Recorded 与 live 边界

- `recorded-development.yaml` 手动运行通过，使用 development 标签和已录制 fixture，不能描述为 holdout、live 或 production。
- `live-holdout.yaml` 默认禁用；现有 replay adapter 会拒绝 live profile。启用前必须在获准环境配置真实依赖、独立 holdout、标签来源与完整指纹。
- 未执行真实模型、Milvus、Prometheus、日志系统或线上流量验证；没有生产收益结论。

### 工程验证

- `go test ./internal/ai/evalharness/... ./internal/app/evaladapter/... ./internal/app/... ./cmd/eval_harness/...`：通过。
- `go test ./...`：通过。
- `go build ./...`：通过。
- `frontend/npm run build`：通过；保留既有 `config.js` 非 module 和主 chunk 大于 500 kB 的 Vite warning。
- `openspec validate unify-evaluation-harness --strict`：通过。
- GitHub Actions 两个 YAML 已完成本地解析；远端 Actions 尚未运行。

## 兼容与回滚

- 旧 `cmd/gos_eval`、RAG evaluator 和数据集未删除，`make eval-gate` 继续运行。
- 新 CI step 或 `make eval-harness-gate` 可独立移除，不影响线上请求链路。
- 新 Harness 报告目录已忽略，不把运行产物提交进仓库；CI 以 artifact 方式保存 JSON/Markdown。
