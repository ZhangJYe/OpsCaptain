## 1. 公共协议与配置

- [x] 1.1 在 `internal/ai/evalharness/` 定义版本化 manifest、case envelope、dataset role、profile、suite status、公共指标、领域 payload、fingerprint 和 report schema
- [x] 1.2 实现 manifest/case 加载与校验，覆盖 schema 不兼容、suite 不匹配、指纹不一致、development/holdout/regression 角色误用和非法 role/profile 组合
- [x] 1.3 在 `manifest/config/config.yaml` 增加 suite 开关、并发、case/总超时、调用/Token/费用预算、Gate 阈值和报告脱敏配置，并补充配置加载测试
- [x] 1.4 定义 Runner、Registry 与 Adapter 生命周期契约，确保领域 payload 必须声明 schema 版本，unavailable 指标不会按零值聚合

## 2. Harness 编排、预算与报告

- [x] 2.1 实现 suite registry 和多 suite orchestrator，支持父 context 取消、suite 隔离、continue-on-error 及保留已完成结果
- [x] 2.2 实现 run/suite/case 三级预算计数与硬限制，覆盖超时、LLM/Tool/RAG 调用、Token、费用和 budget_exceeded 状态
- [x] 2.3 实现公共指标聚合与规范化失败阶段，覆盖 route、retrieve、plan、act、update、report、evidence
- [x] 2.4 实现原子 JSON 报告写入和由 JSON 渲染的 Markdown 摘要，加入 run ID、真实性声明、指纹、Gate、失败 case 和 Trace/Evidence 引用
- [x] 2.5 增加报告脱敏与长度限制测试，验证 query、工具参数、密钥、内部地址和证据原文不会越界写入产物

## 3. 复用现有 RAG、GoS 与 Plan 评测

- [x] 3.1 实现 RAG adapter，复用 `internal/ai/rag/eval/` 的数据校验、MRR/Recall/Hit、分层指标和候选 Gate，不复制评分逻辑
- [x] 3.2 使用同一 RAG fixture 对比新旧入口的关键指标和失败 case，建立 adapter 等价性回归测试
- [x] 3.3 实现 GoS adapter，复用 `internal/ai/agent/gos_engine/eval/` 与现有 profile/指纹/Trace/Evidence 口径
- [x] 3.4 使用同一 GoS fixture 对比新旧入口的根因、Evidence、图有效性、回溯和降级指标，建立 adapter 等价性回归测试
- [x] 3.5 实现 Plan adapter，将现有步骤、重规划、工具调用、Trace、Evidence 与最终状态映射到公共结果，并补充完成率、步骤成功率和重规划成功率指标测试
- [x] 3.6 增加同 case 的 Plan/GoS 对比报告，验证只比较公共指标且各自领域指标保持独立

## 4. Route、Tool 与 Evidence Suite

- [x] 4.1 实现 Route adapter，直接复用线上统一入口所依赖的 Router 接口，以原始 query 评分并记录决策来源、置信度、热点缓存命中和 fallback
- [x] 4.2 增加 Route development/regression 数据集和混淆矩阵、各类 Precision/Recall/F1、低置信度率、P95 指标测试，验证 Memory 不替代原始 query
- [x] 4.3 实现 Tool adapter，通过实际 Registry/schema 和 fake transport 覆盖成功、超时、取消、权限拒绝、畸形返回、外部错误和重试预算
- [x] 4.4 增加 Tool 契约指标与回归数据集，验证降级结果与真实成功分开计数且工具失败不触发不受控框架重试
- [x] 4.5 实现 Evidence adapter，验证 claim→Evidence→Citation→Source→Trace 的完整性、来源有效性、关键词覆盖和 unsupported claim
- [x] 4.6 增加 Evidence 回归数据集与确定性 Gate；如接入 LLM Judge，仅作为带版本指纹的 non-blocking 指标

## 5. Baseline、Gate 与命令入口

- [x] 5.1 实现 dataset、config、code scope、model、prompt、evaluator 与 evidence corpus 指纹采集，并拒绝不兼容 baseline/candidate 比较
- [x] 5.2 实现公共硬门槛和 adapter 提供的领域 Gate，输出 threshold、baseline、actual、方向、severity 与 case refs
- [x] 5.3 实现跨链路 Gate：故障路由进入 Plan/GoS 后必须有 Trace，成功诊断关键结论必须有 Evidence，权限拒绝不得变为成功执行
- [x] 5.4 在 `cmd/eval_harness/` 实现 validate、run、compare、gate 子命令及稳定退出码，并补充 CLI 参数和失败场景测试
- [x] 5.5 在 `evals/harness/` 增加 PR regression、recorded development 和 live holdout 示例 manifest，确保默认示例不包含秘密或真实内部地址

## 6. CI 迁移与验证记录

- [x] 6.1 增加 `make eval-harness-gate`，只运行无网络的 `regression + deterministic` suite，并验证普通 PR 不访问模型、Milvus、Prometheus 或日志系统
- [x] 6.2 旁路运行现有 `make eval-gate` 与新 Harness，记录 RAG/GoS 关键指标等价性、总耗时和差异原因
- [x] 6.3 在旁路验证稳定后更新 `.github/workflows/ci.yml` 使用统一 Gate，并保留旧入口至少一个迁移周期作为回滚路径
- [x] 6.4 增加手动 recorded 工作流；live/holdout 仅记录获准环境中的验证步骤，不以本地 deterministic/recorded 结果替代真实环境验证
- [x] 6.5 运行 `go test ./internal/ai/evalharness/...`、受影响的 Route/RAG/Plan/GoS/Tool/Evidence package 测试、`go test ./...`、`go build ./...` 和前端 `npm run build`
- [x] 6.6 编写 implementation record，记录运行 profile、数据集角色、指纹、Gate 结果、已知证据边界、兼容性与回滚验证
