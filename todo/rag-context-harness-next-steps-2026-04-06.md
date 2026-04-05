# RAG / Context / Harness 下一步待办

## 已完成

- 模块化 RAG runtime
- 统一 retriever cache / failure TTL
- 统一 indexing service
- documents 阶段结构化 retrieval trace
- 离线 eval harness
- 样例 `Recall@K` 基线

## 下一步优先级

### P0

- 把 `internal/ai/rag/eval` 的 sample cases 扩充到 10 条以上
- 给每条 case 增加更清晰的业务标签
- 让真实 Milvus retriever 也能接入同一套 runner

### P1

- 增加 `MRR`
- 增加 duplicate rate
- 增加 retrieval latency 汇总

### P1

- 给 `query_internal_docs` 增加 tool-level retrieval trace
- 把 query rewrite 作为可切换模块接进 harness
- 为 rerank 预留单独评测入口

### P2

- 把 offline eval 接进 CI
- 为真实知识库建立 golden cases
- 做 answer grounding 评测
