# Implementation Record

## 实施范围

- 将统一 Harness 的 deterministic PR regression 从 11 条扩充至 160 条：Route 28、RAG 28、Plan 24、GoS 28、Tool 28、Evidence 24。
- 新增 regression corpus 约束：总量、suite 最低数量与关键 tag 覆盖在 manifest validate 与 Harness run 前验证，防止数据集被静默缩小。
- 新增外置大语料登记 `evals/harness/external-corpora.yaml`：development 目标 3000、frozen holdout 目标 700、Replay evidence 目标 2000；仅登记位置、版本、SHA-256、访问与保留策略，不提交或生成伪造的生产数据。
- `.dockerignore` 排除 `evals/harness/external/` 与 `evals/harness/reports/`，避免离线语料和运行报告进入 Docker 构建上下文。线上请求链路未修改。

## 2026-08-13 验证记录

### PR Gate

- 命令：`make eval-harness-gate`
- 角色/Profile：`regression + deterministic`
- 结果：`succeeded`，160/160 case 执行成功。
- 分布：Route 28、RAG 28、Plan 24、GoS 28、Tool 28、Evidence 24。
- Route：Macro-F1 1.0；chat/incident/confirm 三类混淆矩阵均正确；缓存命中、低置信度 fallback 和 Memory 不参与原始 query 路由均被覆盖。
- RAG：MRR 0.8571、Recall@3/5 1.0、Hit@3/5 1.0；包含多相关文档和 hard-negative 的固定排序语料。
- Plan/GoS：完成率、步骤成功率、重规划成功率、根因/证据/图/Trace/契约指标均为 1.0。
- Tool：成功、降级和权限拒绝被覆盖；权限合规为 1.0，权限拒绝未执行跨链路 Gate 通过。
- Evidence：claim support 与 citation traceability 均为 1.0，覆盖单/多引用和跨来源 Trace。
- 跨链路 Gate：incident route→diagnosis、诊断 Trace、诊断 Evidence、权限拒绝不执行全部通过。

### 工程验证

- `go test ./internal/ai/evalharness/... ./internal/app/evaladapter/... ./cmd/eval_harness/...`：通过。
- `go test ./...`：通过（在获准的本机回环环境中运行；沙盒环境禁止 `httptest` 绑定临时端口）。
- `go build ./...`：通过。
- `frontend/npm run build`：通过；保留既有 `config.js` 非 module 与主 chunk 大于 500 kB 的 Vite warning。
- `openspec validate expand-evaluation-regression-corpus --strict`：通过。
- `.dockerignore` 已静态确认排除外置语料和报告；未在本地 Docker/Compose 代替真实部署验证，服务器部署未验证。

## 真实性边界

- 160 条是 deterministic PR regression，不是 production benchmark、独立 holdout 或线上收益证明。
- 3000 development、700 frozen holdout、2000 Replay 是已登记的离线数据目标；本次没有生成、伪造或上传其内容。
- 填充大语料前必须取得脱敏数据授权，按事故族/服务/时间窗口拆分，记录来源、标签策略与 SHA-256，并在受控 CI 或离线评测环境加载。
- 未调用真实模型、Milvus、Prometheus、日志系统或线上流量；未做服务器部署验证。
