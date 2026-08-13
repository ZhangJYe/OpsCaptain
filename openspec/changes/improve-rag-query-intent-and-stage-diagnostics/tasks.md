## 1. 固化范围与基线边界

- [x] 1.1 保存字段增强冻结候选的 14 个 Recall@5 不完整案例、category 分布与缺失文档频次，作为本轮只读误差输入
- [x] 1.2 在评测比较层继续拒绝 holdout 作为 Development 基线，并记录本轮禁止案例专属词表、RRF 网格搜索和生产默认更新

## 2. 阶段级检索诊断

- [x] 2.1 扩展 Hybrid trace，按运行顺序记录 Dense、Lexical、Fusion、Candidate 与 Final 阶段的规范化文档 ID
- [x] 2.2 将阶段 trace 接入评测适配层，输出每案例相关文档的阶段排名、最后出现阶段与召回缺口分类
- [x] 2.3 汇总各阶段 Recall@1/3/5/20，并为未召回、Fusion 丢失、Candidate 截断和 Final 丢失补齐单元测试
- [x] 2.4 控制 trace 体积，只保存现有 TopK 边界内的 ID，不复制正文、向量或评测标签

## 3. 确定性查询意图

- [x] 3.1 在 `manifest/config/config.yaml` 增加意图解析/精排 feature flag、连接词、最大词项数、正向加分和排除惩罚上限，默认关闭
- [x] 3.2 实现 `QueryIntent` 解析，覆盖“只看 A，不要 B”“A 而不是 B”“A，不执行 B”等明确结构
- [x] 3.3 对无连接词、任一侧为空、双重否定或歧义表达安全降级为无排除意图，并补齐中英文/混合查询测试
- [x] 3.4 验证解析只使用原始 query，不读取 Memory、category、relevant_ids、distractor_ids 或案例 ID

## 4. 有界意图精排与 Trace

- [x] 4.1 在 CandidateTopK 截断前加入配置化正向加分与排除降权，仅在文档缺少正向匹配时施加有上限的 penalty
- [x] 4.2 文档同时命中正向/排除词项时保留候选并计算净分，不过滤文档，不改变 CandidateTopK/FinalTopK
- [x] 4.3 扩展逐文档 trace，记录 intent rule、positive hits、excluded hits、bonus、penalty、net score 与精排位置
- [x] 4.4 补齐开关关闭兼容、hard-negative、综合文档、稳定 tie-break 和 FinalTopK 边界测试

## 5. 报告、Gate 与限定实验

- [x] 5.1 升级报告 schema，将阶段指标、缺口分类、意图解析覆盖率/应用率/降权次数和有效配置写入报告
- [x] 5.2 关闭意图能力生成同指纹、同语料版本、同检索预算的 Development v5 兼容基线
- [x] 5.3 只运行一个意图精排候选，与 v5 基线比较整体 MRR/Recall@5、P95、failure/empty、multi-document 与 hard-negative Recall@5
- [x] 5.4 按全部 Gate 冻结或拒绝候选，不追加连接词、案例专属规则、权重候选或网格搜索
- [x] 5.5 不运行或查看任何 sealed holdout 逐案例结果；候选通过也保持生产默认关闭并记录需要新一代独立 holdout

## 6. 验证与记录

- [x] 6.1 运行 `go test ./internal/ai/rag/... ./internal/ai/cmd/rag_online_eval_cmd` 和 ContextEngine 预算测试
- [x] 6.2 运行 `go test ./...`、`go build ./...`、前端 `npm run build`、`git diff --check` 与 OpenSpec strict 校验
- [x] 6.3 保存实现记录、v5 基线、唯一候选、阶段误差分析与冻结结论，明确 Development 离线证据边界
