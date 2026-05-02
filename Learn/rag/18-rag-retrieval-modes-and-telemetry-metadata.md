# RAG 下一步改造：Retrieval Modes 与 Telemetry Metadata

这次改动做了两件事，目标都很明确：

1. 让 `telemetry evidence` 文档真正带上结构化 metadata，并且索引链路能读到这些 metadata
2. 让在线评测支持 `retriever-only` / `rewrite+retrieve` / `full` 三种模式，拆开看问题到底出在哪一层

## 1. 为什么先做这两件事

前面的 baseline 已经暴露出一个核心问题：

- `history_build` 对 `evidence_build` 几乎没有提升

这说明现在不能再只盯着“文档内容要不要更丰富”，而要分清楚：

1. 问题在语料
2. 问题在检索
3. 问题在 rewrite/rerank

这次改造就是为这个判断服务的。

## 2. Telemetry metadata 改了什么

### 2.1 新增 sidecar metadata

每个 telemetry markdown 文档旁边，现在会多一个同名 sidecar：

- `case-id.md`
- `case-id.metadata.json`

示例路径：

- `aiopschallenge2025/baseline/docs_evidence_telemetry/009be6db-313.md`
- `aiopschallenge2025/baseline/docs_evidence_telemetry/009be6db-313.metadata.json`

### 2.2 sidecar 里包含什么

核心字段：

- `case_id`
- `doc_id`
- `doc_kind`
- `split`
- `service`
- `instance_type`
- `instance`
- `source`
- `destination`
- `start_time`
- `end_time`
- `service_tokens`
- `pod_tokens`
- `node_tokens`
- `namespace_tokens`
- `metric_signal_count`
- `log_signal_count`
- `trace_signal_count`
- `metric_names`
- `trace_services`
- `trace_operations`

### 2.3 额外输出的清单

除了每个文档自己的 sidecar，还新增了汇总：

- `aiopschallenge2025/baseline/telemetry/doc_metadata.jsonl`
- `aiopschallenge2025/baseline/telemetry/doc_metadata_build.jsonl`

### 2.4 为什么 sidecar 要接进索引

如果 metadata 只落盘、不进索引，那它对当前 RAG 没价值。

所以这次一起改了 loader：

- [loader.go](/d:/Agent/OpsCaption/internal/ai/loader/loader.go)

现在当 `knowledge_cmd` 索引 `foo.md` 时，会自动查找同目录的：

- `foo.metadata.json`

并把 sidecar 字段合并进 `schema.Document.MetaData`。

这意味着：

1. Milvus 里的 `metadata` 字段现在真的会带上 `case_id/service/instance_type/...`
2. 评测里的 doc id 规范化会优先吃 `case_id`
3. 后面做 metadata filter 时，不需要再重做文档格式

## 3. Retrieval modes 改了什么

### 3.1 新增 QueryMode

位置：

- [query.go](/d:/Agent/OpsCaption/internal/ai/rag/query.go)

现在支持三种模式：

1. `retrieve`
   - 原 query 直接检索
   - 不 rewrite
   - 不 rerank

2. `rewrite`
   - 先 rewrite
   - 再 retrieve
   - 不 rerank

3. `full`
   - rewrite
   - retrieve
   - rerank

原来的 `rag.Query(...)` 仍然保留，默认等价于 `full`。

### 3.2 在线评测命令新增 `--mode`

位置：

- [main.go](/d:/Agent/OpsCaption/internal/ai/cmd/rag_online_eval_cmd/main.go)

现在可以这样跑：

```bash
go run ./internal/ai/cmd/rag_online_eval_cmd \
  -mode retrieve \
  -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl
```

也可以跑：

```bash
go run ./internal/ai/cmd/rag_online_eval_cmd \
  -mode rewrite \
  -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl
```

默认仍是：

```bash
go run ./internal/ai/cmd/rag_online_eval_cmd \
  -mode full \
  -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout_related.jsonl
```

### 3.3 为什么这一步重要

有了 `--mode` 之后，你才能把问题拆开看：

1. `retrieve` 很差  
说明问题主要在召回层

2. `retrieve` 还可以，`rewrite` 变差  
说明 query rewrite 在伤害召回

3. `rewrite` 还可以，`full` 变差  
说明 rerank 在伤害排序

这个判断在做 hybrid retrieval 之前是必要的。

## 4. 云端脚本也同步了 mode

位置：

- [run_telemetry_baseline_remote.sh](/d:/Agent/OpsCaption/scripts/aiops/run_telemetry_baseline_remote.sh)

现在远端脚本支持：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --mode retrieve
```

或者：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --mode full
```

## 5. 这次怎么验证的

Go 侧：

```powershell
go test ./internal/ai/loader ./internal/ai/rag ./internal/ai/agent/knowledge_index_pipeline ./internal/ai/cmd/rag_online_eval_cmd
```

Python 侧：

```powershell
python -m unittest scripts.aiops.test_build_telemetry_evidence
python scripts/aiops/build_telemetry_evidence.py --dataset-root aiopschallenge2025 --output-root aiopschallenge2025/baseline --limit 1
```

实际看到的 sidecar 示例已经包含：

- `case_id`
- `service`
- `instance_type`
- `metric_names`
- `trace_operations`

## 6. 当前边界

这次还没做：

1. hybrid retrieval
2. metadata filter 真正参与召回
3. 基于图谱的 pre-filter

这次做的是“把下一步要用的基础能力先接好”。

## 7. 现在最合适的下一步

在 telemetry baseline 跑完之后，直接做两组对比：

1. `retrieve`
2. `rewrite`
3. `full`

对比对象：

- `evidence_build`
- `history_build`
- `telemetry_evidence_build`

只有这三组都跑完，才值得继续做 `hybrid retrieval`。
