## 1. Corpus contract and validation

- [x] 1.1 定义外置 recorded corpus manifest、来源指纹、split 元数据和统计结构，并为其添加序列化/校验测试。
- [x] 1.2 实现 source input / ground truth 的 UUID 对齐、schema 和完整性校验；缺失或不一致时返回可审计错误。
- [x] 1.3 实现稳定 fault-family key、确定性 group split 与跨 split 泄漏校验，并覆盖重复运行和泄漏失败测试。

## 2. AIOps2025 preparation

- [x] 2.1 实现 `eval_harness corpus prepare`，从外置 AIOps2025 source 生成 development / holdout 的 GoS 与 Evidence JSONL 及 corpus manifest。
- [x] 2.2 实现 `eval_harness corpus validate`，验证已生成的 corpus、指纹、case schema、split 与按分类/模态统计；在远期目标不足时输出 coverage gap。
- [x] 2.3 用本机现有 AIOps2025 source 生成一次外置 corpus，保留仅含元数据的可复查摘要，不提交原始 telemetry 或完整外置 case 数据。

## 3. Harness integration and evidence

- [x] 3.1 增加 development 和 frozen holdout 的 recorded manifest 模板，使其引用外置 corpus 并校验 provenance / split / role。
- [x] 3.2 在 Harness 报告中输出 external corpus provenance、split fingerprint 与 coverage gap，并在 recorded truth boundary 中明确离线限制。
- [x] 3.3 为 prepare/validate/manifest 集成和报告字段补充单元测试，运行相关 Go package 测试和 Harness gate。

## 4. Documentation and verification

- [x] 4.1 更新 Harness README 与 external-corpora registry，记录 AIOps2025 许可、运行命令、外置存储边界、实际规模和扩容方法。
- [x] 4.2 执行一次 development recorded 基线，保留 JSON/Markdown 报告摘要；确认其不作为 PR、live 或生产结论。
- [x] 4.3 运行 `openspec validate ingest-recorded-evaluation-corpus --strict`、受影响 Go 测试和 `npm run build`，记录验证结果。
