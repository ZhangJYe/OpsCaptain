## 1. 固化误差基线与隔离边界

- [x] 1.1 从 `development-v3-hybrid-retrieve.json` 生成并保存整体、category、difficulty、language 和缺失文档频次的误差分析记录
- [x] 1.2 在评测入口或比较层拒绝把上一轮 `holdout-v2-hybrid-retrieve-frozen.json` 用作本轮 Development 调参/Gate 基线，并补齐测试
- [x] 1.3 将报告 schema 升级并在兼容默认配置下重跑 Development v4 基线，确认指标口径与逐案例身份可复现

## 2. 字段化知识文档索引

- [x] 2.1 实现容错的 Markdown 标题、Provider、Tags、稳定文档 ID 与 source 解析，覆盖上游文档和现有综合文档格式
- [x] 2.2 调整 BM25 文档表示，使正文与字段 token 分离；缺失字段时保持正文检索，避免无关 metadata 污染长度归一化
- [x] 2.3 为字段解析、字段缺失、重复 AddDocument 替换和 BM25 兼容排序补齐单元测试
- [x] 2.4 让知识索引/评测预热使用同一字段解析路径，并验证 30 份 Markdown 的稳定 ID、标题和 Tags 统计

## 3. 可配置融合与可解释精排

- [x] 3.1 在 `manifest/config/config.yaml` 与 HybridConfig 增加 Dense/Lexical RRF 非负权重及校验，默认 `1.0/1.0` 保持兼容
- [x] 3.2 实现加权 RRF，并测试双通道、单通道、重复文档键、非法权重和稳定排序
- [x] 3.3 扩展知识文档 profile，加入文档 ID、标题、Tags、Provider 的精确/词项匹配信号和可配置上限
- [x] 3.4 扩展逐文档 trace，记录字段命中、字段加分、融合位置、精排位置和最终位置，并验证关闭开关时排序兼容

## 4. 有界多文档覆盖

- [x] 4.1 实现 feature-flag 控制的确定性覆盖选择，只在新增查询词项覆盖明确且位置提升不超过预算时调整 FinalTopK
- [x] 4.2 验证覆盖选择不读取 category、relevant_ids 或 distractor_ids，不扩大 FinalTopK，并在信号不足时保持原顺序
- [x] 4.3 补齐单主题、多主题、重复文档族、无新增覆盖和稳定 tie-break 测试

## 5. 分层报告与 Gate

- [x] 5.1 在评测汇总中增加 category/difficulty/language 的 cases、Recall@K、Hit Rate@K、MRR 和 failure rate
- [x] 5.2 输出 Recall@5 未完全命中案例与缺失相关文档 ID 频次，并测试空分层和失败案例分母
- [x] 5.3 将字段/融合/覆盖配置纳入有效配置快照和报告可比性校验
- [x] 5.4 在 `rag.eval_gate` 增加 multi-document 与 hard-negative Recall@5 非回归门槛，并补齐整体提升但分层回退的拒绝测试

## 6. 限定 Development 实验

- [x] 6.1 运行字段化索引 + 字段精排单独消融，保存与 v4 基线同指纹、同语料版本、同报告 schema 的候选报告
- [x] 6.2 仅在 6.1 通过 Gate 后运行最多两个预先声明的 RRF 权重候选，不追加临时网格搜索
- [x] 6.3 仅在多文档 Recall@5 仍是瓶颈时运行一个覆盖策略候选，候选总数不得超过四个
- [x] 6.4 按整体与分层 Gate 冻结单一 Development 候选；无候选通过则记录保持现状
- [x] 6.5 不重新运行或查看上一轮 sealed holdout 逐案例结果；若形成胜出候选，记录需要新一代独立 holdout，且不更新默认配置

## 7. 验证与记录

- [x] 7.1 运行 `go test ./internal/ai/rag/... ./internal/ai/cmd/rag_online_eval_cmd` 和 ContextEngine 预算测试
- [x] 7.2 运行 `go test ./...`、`go build ./...`、`git diff --check` 与 `openspec validate improve-rag-hybrid-ranking --strict`
- [x] 7.3 保存实现记录、v4 基线、候选、逐项 Gate、误差分析和冻结结论，明确 Development 离线证据边界及尚缺的新独立 holdout/线上验证
