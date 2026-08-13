## 实施记录

- 日期：2026-08-13
- 基准代码：`cc73ae805865`，实施时工作区包含未提交修改
- OpenSpec 进度：26/26
- 证据边界：本记录包含代码级、确定性测试、扩展数据集静态校验，以及真实本地 Milvus + 外部 Embedding/模型服务的离线 Development 与 sealed holdout 评测。首次 60 条扩展 Development 运行发现 BM25 只加载顶层 15 份文档，相关报告仅作为缺陷定位证据；递归修复后的 v3 Development 报告用于模式与参数选择，单次 sealed holdout 用于验证冻结配置。所有数据仍是离线证据，不代表线上收益。

## 已完成

- 四种检索消融统一调用生产 Hybrid Query 路径，未知模式返回错误。
- 评测入口强制声明 dataset role、外部语料版本和报告路径，并支持有限 Hybrid 参数 override。
- Hybrid Trace 记录完整墙钟耗时、Dense/Lexical/Fusion 细分耗时和候选数量；Query 不再用 Dense 耗时代替完整检索耗时。
- 评测汇总增加失败率及各阶段 Avg/P50/P95，失败案例与成功案例质量指标使用不同且明确的分母。
- 报告记录数据指纹、语料版本、有效配置、代码 revision、生成时间和离线证据类型。
- 非回归 Gate 比较 MRR/Recall@5、失败率、空结果率和 P95 延迟，并拒绝旧报告或数据/语料不一致的报告。
- CandidateTopK 与 FinalTopK 保持分离；最终结果受 FinalTopK 限制，下游 ContextEngine 文档 token budget 测试通过。
- Rewrite/Rerank 失败回退和基础检索错误透传已有测试覆盖。
- development 与 holdout 固定数据已拆分，相关 ID 只用于评测打分，不进入 Query 或运行时提示。
- 建立 `knowledge-seed-v1` 可评测语料清单：30 份 Markdown 中纳入 27 份答案文档，排除目录索引、知识导入说明和 README。
- 新增数据集静态校验器和独立命令，校验样本 schema、标签 ID、类别分布、语料覆盖、跨 split 重复/近重复与封存指纹。
- development 扩充为 60 条，holdout 扩充为 100 条；两者均精确满足五类目标分布并覆盖全部 27 份可评测文档。
- 修复在线评测 BM25 预热只读取顶层目录的问题，改为递归加载 Markdown；测试和 1 条真实 Development 冒烟均确认索引数由 15 变为 30。
- Rewrite/Rerank trace 与评测报告新增 attempted、applied、degraded 和安全失败原因，能够区分“功能已开启”和“本次实际生效”。
- 修复父 context 已过期后仍输出下一次重试提示的问题，避免把一次超时误读为继续重试；超时仍按原有降级路径返回。

## 数据记录

- `evals/rag/retrieval_development.jsonl`：60 个案例，封存指纹 `35eec6f8c266156e96ea7ec1f2eed48452da16e7458dad415690b2454bfe0d3d`
- `evals/rag/retrieval_holdout.jsonl`：100 个案例，封存指纹 `5e8eee0b1d9e4abe1cbbfdf659c63eac5adf396b3ec3114bb199997099849b9a`
- 静态校验：两套数据规模达标、语料覆盖率均为 `100%`、跨 split 近重复为 `0`、问题数为 `0`；报告位于 `evals/rag/reports/dataset-validation.json`。
- 原 12 条 development 冒烟集基线：Hybrid 50/50/20/5，MRR `0.9583`、Recall@5 `1.0`、失败率 `0`、P95 `213ms`。
- 原 12 条 development 冒烟集候选：Hybrid 20/20/10/5，MRR `0.9583`、Recall@5 `1.0`、失败率 `0`、P95 `218ms`，通过非回归 Gate 但没有稳定收益；该结论不用于扩展数据集的配置选择。
- 最终保留基线和候选报告；早期 `code_revision=unknown` 的探索报告已移除。
- 默认 `rewrite_enabled` 与 `rerank_enabled` 保持关闭，Hybrid 默认参数保持不变。Development 没有新候选通过 Gate，sealed holdout 因此验证的是现有冻结配置，不存在待晋级的新配置，也没有修改生产默认参数。
- 递归修复前的 60 条 Development 消融结果：纯 Hybrid MRR `0.6681` / P95 `198ms`；Rewrite MRR `0.6736` / P95 `3217ms`；Rerank MRR `0.7306` / P95 `5222ms`；Full MRR `0.6611` / P95 `8234ms`。后三者均因延迟或质量 Gate 失败。
- 递归修复前的有限调参中，Hybrid 30/30/15/5 的 MRR `0.6708`、Recall@5 `0.7778`、P95 `188ms` 并通过 Gate；由于 BM25 语料不完整，该候选已撤销冻结，必须在修复后重新评测。
- 递归修复后的 v3 Development 基线（Hybrid 50/50/20/5）：MRR `0.7061`、Recall@5 `0.7944`、P95 `172ms`、失败率 `0`，报告为 `development-v3-hybrid-retrieve.json`。
- v3 Rewrite：MRR `0.6667`、Recall@5 `0.7750`、P95 `3228ms`；实际生效 `12/60`、超时降级 `48/60`，Gate 未通过。
- v3 Rerank：MRR `0.7014`、Recall@5 `0.7861`、P95 `5239ms`；实际生效 `15/60`、超时降级 `45/60`，Gate 未通过。
- v3 Full：MRR `0.7528`、Recall@5 `0.7583`、P95 `8225ms`；Rewrite 实际生效 `8/60`、降级 `52/60`，Rerank 实际生效 `19/60`、降级 `41/60`。虽然 MRR 上升，但 Recall@5 与延迟门槛失败，Gate 未通过。
- v3 有限调参只运行两个预先限定的纯 Hybrid 候选：20/20/10/5 的 MRR `0.6381`、Recall@5 `0.7528`、P95 `164ms`；30/30/15/5 的 MRR `0.6797`、Recall@5 `0.7778`、P95 `196ms`。两者 Gate 均未通过，因此冻结当前 50/50/20/5 纯 Hybrid 配置，不修改默认参数。
- 单次 sealed holdout 使用同一 `knowledge-seed-v1` 语料和冻结的 Hybrid 50/50/60/20/5 配置，100/100 条成功：MRR `0.6708`、Recall@5 `0.7775`、Hit Rate@5 `0.8500`、P95 `180ms`、失败率 `0`、空结果率 `0`、引用覆盖率 `100%`。报告为 `evals/rag/reports/holdout-v2-hybrid-retrieve-frozen.json`，数据指纹为 `5e8eee0b1d9e4abe1cbbfdf659c63eac5adf396b3ec3114bb199997099849b9a`。
- Holdout 只运行冻结配置，没有用于搜索参数。由于 Development 阶段没有新候选通过 Gate，本轮不存在“新候选对旧基线”的 holdout 差分 Gate；配置晋级规则的实际结果是“不晋级、不改默认值”。Holdout 指标只作为现有配置的独立泛化证据。

## 验证记录

- `go test ./internal/ai/rag/... ./internal/ai/cmd/rag_online_eval_cmd`：通过
- `go test ./internal/ai/contextengine`：通过
- `go test ./...`：通过；沙箱内首次运行因既有 `httptest` 无法绑定回环端口而失败，允许本机回环绑定后复跑通过
- `go build ./...`：通过
- `openspec validate optimize-rag-retrieval-quality --strict`：通过
- `go test ./internal/ai/rag/... ./internal/ai/cmd/rag_dataset_validate_cmd ./internal/ai/cmd/rag_online_eval_cmd`：通过
- `go run ./internal/ai/cmd/rag_dataset_validate_cmd`：通过，读取 `rag.eval_dataset.near_duplicate_threshold=0.8`
- 在 v3 Development 消融、阶段结果观测与重试日志修复完成后重新运行 `go test ./...`：通过
- 在同一代码状态重新运行 `go build ./...`：通过
- 在实施记录更新后重新运行 `openspec validate optimize-rag-retrieval-quality --strict`：通过
- `go run ./internal/ai/cmd/rag_dataset_validate_cmd` 在 holdout 运行前再次通过，封存指纹未变化
- `go run ./internal/ai/cmd/rag_online_eval_cmd ... -dataset-role holdout ...`：单次 100 条 sealed holdout 运行通过，100 条逐案例结果已保存
- 完成 holdout 与记录更新后，RAG/ContextEngine/Resilience 局部测试、`go test ./...`、`go build ./...` 再次通过
- 最终审计确认 holdout 报告包含 100 个唯一案例，报告指纹与 manifest 一致，冻结配置与默认运行参数一致，五份 Development 候选报告的 Gate 均为未通过
- `openspec instructions apply --change optimize-rag-retrieval-quality --json` 返回 `26/26`、`state=all_done`，严格校验再次通过

上述 Go 命令使用 `/private/tmp/opscaptain-rag-gocache`，原因是默认 Go 缓存目录在当前沙箱中不可写；这不改变测试内容。

## 历史阻塞记录（Milvus 连接已解决）

- 原 12 条 development 的基础 Hybrid 基线与有限参数调优已完成；扩展 Development 在修复 BM25 递归加载后需要重跑，holdout 评测和最终报告尚未完成。
- 首次基线命令启动后约 90 秒无输出，随后主动终止；当时检查 `127.0.0.1:19530` 返回 `Connection refused`，本地 Milvus 未运行。
- 已生成 `evals/rag/reports/development-hybrid-retrieve.json` 和 `development-hybrid-retrieve-candidate-20.json`。没有用内存检索、mock 或历史报告替代真实依赖结果。

## 最终评测结论

1. 默认采用 Dense + BM25 + RRF + Metadata Boost 的纯 Hybrid 链路，参数为 DenseTopK 50、LexicalTopK 50、FusionK 60、CandidateTopK 20、FinalTopK 5。
2. Rewrite/Rerank 保留可配置能力和失败回退，但当前外部模型阶段超时率高、P95 延迟显著增加，默认关闭。
3. 两个缩小候选池的参数组合均未通过 Development Gate；不以更低延迟交换 MRR 和 Recall@5 回退。
4. 独立 sealed holdout 上 100/100 成功、失败率与空结果率为 0，证明当前冻结配置在该离线语料和依赖环境下可稳定复现；不外推为生产流量效果。

## 面试口述版（STAR）

**S（情境）**：项目原来已经有 Dense、BM25、查询改写和重排，但只有 12 条冒烟样本，评测路径和生产路径也存在偏差，无法证明某个增强模块真的提升了检索质量。

**T（任务）**：我的目标是把 RAG 优化变成一套可复现、可拒绝错误候选的工程闭环：既要提升召回，也不能牺牲失败率和尾延迟，并且要严格隔离 Development 与 sealed holdout。

**A（行动）**：我先把评测统一到生产 Hybrid Query 链路，补齐数据指纹、语料版本、有效配置和分阶段延迟；再把数据扩展到 60 条 Development 和 100 条 holdout，覆盖 27 份知识文档，并校验跨集合近重复为 0。随后我做四组消融，发现 BM25 预热只加载了顶层文档，于是改成递归加载，索引文档从 15 份恢复到 30 份。最后用 MRR、Recall@5、失败率、空结果率和 P95 组成 Gate，只在 Development 上做两组有限调参，同时给 Rewrite/Rerank 增加实际生效与降级观测。

**R（结果）**：Development 上当前纯 Hybrid 的 MRR 是 `0.7061`、Recall@5 是 `0.7944`、P95 是 `172ms`，60 条请求全部成功。Full 模式虽然把 MRR 提到 `0.7528`，但 Recall@5 降到 `0.7583`、P95 增至 `8225ms`，所以被 Gate 拒绝；两组参数候选也没有超过基线。最终冻结纯 Hybrid 配置，在 100 条 sealed holdout 上得到 MRR `0.6708`、Recall@5 `0.7775`、P95 `180ms`，100 条全部成功且空结果率为 0。我的结论不是“增强模块越多越好”，而是用独立数据和质量、可靠性、延迟三类门槛拒绝不合格方案。以上仍属于离线评测，不能包装成线上收益。

## 连接故障修复记录

- 日期：2026-08-12
- 根因：Docker Desktop 未运行，Docker socket 不存在，已有 `opscaptain-observability` 依赖栈因此全部停止，`127.0.0.1:19530` 返回 `Connection refused`。
- 恢复：启动 Docker Desktop 后，已有 `opscaptain-observability-milvus-1` 按 restart policy 自动恢复；容器健康，19530 端口可连接。
- 清理：曾尝试启动 `manifest/docker/docker-compose.yml` 的重复 Milvus 栈，但其 MinIO 与已有 9000 端口冲突；已精确执行该 Compose 的 `down`，未删除卷，未影响 `opscaptain-observability` 栈。
- 本地数据：`agent/opscaption_knowledge_v2` 集合已加载，schema 为 2048 维 FloatVector，统计为 840 行。
- 代码修复：在线评测改为使用只读 `OpenExistingMilvusClient`，连接受 `milvus.startup_timeout_ms` 控制；同时复用主服务的 `.env` 安全加载逻辑，避免评测进程缺少模型凭据。
- 验证：空 Query 的 connectivity-only 评测成功生成 `/private/tmp/opscaptain-rag-connectivity-report.json`，没有调用外部 Embedding 服务。
- development 的 12 条查询已在明确授权后发送到配置的豆包 Embedding 服务，基础 Hybrid 评测完成。
- 最终状态：修复后的 Development 消融和单次 sealed holdout 均已在明确授权后完成；外部依赖真实可用，但仍未进行生产流量或线上 A/B 验证。
