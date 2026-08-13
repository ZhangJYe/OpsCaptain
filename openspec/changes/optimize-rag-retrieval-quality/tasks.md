## 1. 固定可比评测入口

- [x] 1.1 完成并保留现有生产 Hybrid 路径消融修正，使四种基础模式只通过 Rewrite/Rerank override 区分
- [x] 1.2 将未知评测模式改为显式校验错误，并补齐模式解析与生产路径选择单元测试
- [x] 1.3 为评测入口增加 dataset role、corpus version 和报告输出参数校验

## 2. 补齐 Trace 与指标

- [x] 2.1 为 Hybrid trace 增加完整墙钟耗时、Dense/Lexical/Fusion 细分耗时和各阶段候选计数
- [x] 2.2 将 Query 与在线评测指标映射到正确的 Hybrid 总耗时，并保留 Rewrite/Rerank/端到端耗时
- [x] 2.3 在评测汇总中增加失败率以及端到端和各阶段延迟的 Avg/P50/P95，并补齐边界测试
- [x] 2.4 验证失败案例不会被质量指标静默吞掉，成功案例指标与全量可靠性指标分母清晰

## 3. 建立可复现报告与 Gate

- [x] 3.1 扩展报告元数据，记录数据角色、数据集 SHA-256、语料版本、有效 RAG 配置、代码 revision、生成时间和证据环境
- [x] 3.2 在 `manifest/config/config.yaml` 增加 `rag.eval_gate` 的主指标、最小改进、允许回退、失败率、空结果率和 P95 延迟门槛
- [x] 3.3 实现基线/候选报告比较与逐门槛结果输出，并测试质量提升但延迟超限等拒绝场景
- [x] 3.4 校验两份报告的数据指纹、语料版本和关键基础配置可比，旧报告缺少必要元数据时拒绝进入新 gate

## 4. 数据与检索参数优化

- [x] 4.1 建立初版 development/holdout JSONL 与标签隔离规则；当前 12/6 条仅作为评测链路冒烟集，不作为正式配置选择证据
- [x] 4.2 盘点当前语料，建立版本化可评测文档 ID 清单并排除 README、索引和导入说明等非答案文档
- [x] 4.3 扩展评测样本 schema 与校验器，校验必填字段、标签 ID、类别枚举、困难样本干扰 ID、数据规模和语料覆盖，并补齐测试
- [x] 4.4 在不查看 holdout 检索结果的前提下，将 development 扩充到至少 60 条并满足五类查询目标分布
- [x] 4.5 将 sealed holdout 扩充到至少 100 条，完成标签、覆盖和跨 split 近重复校验后保存版本与 SHA-256 指纹
- [x] 4.6 在 `manifest/config/config.yaml` 增加跨 split 近重复阈值，并验证阈值由配置读取而非硬编码
- [x] 4.7 固化扩展后 development 的当前配置基线，并运行 hybrid-retrieve、hybrid-rewrite、hybrid-rerank、hybrid-full 消融报告
- [x] 4.8 在原 12 条冒烟开发集上完成一轮 CandidateTopK 参数试验并保留报告；该结果不作为扩展数据集的最终候选结论
- [x] 4.9 仅对扩展后 development 的胜出组合小范围调整 DenseTopK、LexicalTopK、FusionK、CandidateTopK、FinalTopK 和超时预算
- [x] 4.10 验证 CandidateTopK 与 FinalTopK 分离，最终文档数受 FinalTopK 限制且下游 ContextEngine token budget 仍生效
- [x] 4.11 冻结 development 胜出的单一候选后运行指纹一致的 sealed holdout；只有 holdout gate 通过才更新默认 RAG 配置，否则保留当前配置

## 5. 降级与验证

- [x] 5.1 补齐 Rewrite/Rerank 超时回退和基础检索错误透传测试，确认不引入 fatal 行为
- [x] 5.2 运行 `go test ./internal/ai/rag/...` 与 `go test ./internal/ai/cmd/rag_online_eval_cmd` 验证 RAG 局部改动
- [x] 5.3 运行 `go test ./...` 和 `go build ./...`；如真实 Milvus/模型服务不可用，明确记录真实依赖评测未完成且不以 mock 结果替代
- [x] 5.4 保存扩展数据集的基线、候选、gate 和 sealed holdout 报告，明确标注数据版本、指纹及离线证据边界
