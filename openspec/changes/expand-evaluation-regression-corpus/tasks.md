## 1. 数据集扩充

- [x] 1.1 将 Route regression 扩充到 28 条，覆盖 chat、incident、confirm、模型、关键词、缓存、低置信度 fallback 与 Memory 不参与路由
- [x] 1.2 将 RAG regression 扩充到 28 条，覆盖 top-k 命中、排序、多相关文档和 hard negative
- [x] 1.3 将 Plan regression 扩充到 24 条、GoS regression 扩充到 28 条，覆盖成功、重规划、降级、回溯、证据关联与共享事故 case
- [x] 1.4 将 Tool regression 扩充到 28 条，覆盖成功、权限拒绝、超时、取消、外部错误、畸形返回、降级与调用预算
- [x] 1.5 将 Evidence regression 扩充到 24 条，覆盖 claim support、单/多引用、跨来源与引用追踪

## 2. 校验与预算

- [x] 2.1 增加 regression corpus 校验，检查 160 条总量、suite 最低数量、schema/suite/ID 和关键场景覆盖
- [x] 2.2 调整 PR manifest 的 run/suite `max_cases` 与必要预算，使 160 条数据可执行且保持单 case/总超时边界
- [x] 2.3 增加外置 development/holdout/replay 语料登记，并通过 `.dockerignore` 排除其内容与评测报告的镜像构建上下文
- [x] 2.4 补充数据集缩小、重复 ID、缺少场景、Docker 排除和完整 160 条通过的测试

## 3. 验证与记录

- [x] 3.1 运行 OpenSpec strict 校验、相关 evalharness/evaladapter 测试与 `make eval-harness-gate`
- [x] 3.2 运行 `go test ./...`、`go build ./...` 和 `frontend/npm run build`
- [x] 3.3 更新 implementation record，记录 160 条 suite 分布、Gate 结果、指纹、耗时、外置语料边界与 regression/holdout/production 区分
