# aiopschallenge2025 云端 RAG Baseline 实战记录

这份文档记录的是一套严格的、可复现的云端基线流程，不是在本地开发机上自测。

目标有四个：

1. 把 `aiopschallenge2025` 数据预处理成当前项目可直接索引的 RAG 文档。
2. 在云端拉起独立 Milvus，避免污染现网知识库。
3. 用当前项目真实的 `rag.Query(...)` 链路跑 baseline。
4. 把中间踩到的设计和实现问题收口成可复盘的结论。

## 1. 运行环境

- 云机：`124.222.57.178`
- SSH 别名：`tencent-opscaptain`
- 线上部署目录：`/opt/opscaptain`
- 评测工作目录：`/opt/opscaptain/baseline-workspace`
- 独立 Milvus Compose：`/opt/opscaptain/baseline-workspace/manifest/docker/docker-compose.remote.yml`

说明：

- 正式站点的 `backend/frontend/caddy` 容器继续跑，不动现网站点。
- baseline 使用单独的 Milvus 容器和单独的 collection 名，不复用现网 collection。
- 数据预处理、索引、评测都在云端执行。

## 2. 数据输入

这次只把最小必要文件传到云端：

- `aiopschallenge2025/input.json`
- `aiopschallenge2025/groundtruth.jsonl`

没有把 `11.9GB` 的 parquet 全量同步到这次 baseline 工作目录。

当前这一版 baseline 的定位是：

- 先验证当前项目的 RAG 基线链路。
- 文档内容来自 `input.json + groundtruth` 派生产物。
- 还不是“纯原始遥测证据基线”。

如果后面要更严格，下一步再补：

- `parquet -> evidence summary` 提取器
- 图谱增强召回
- 不依赖 `groundtruth.key_observations` 的证据构建

## 3. 预处理产物

预处理命令：

```bash
go run ./internal/ai/cmd/aiops_rag_prep_cmd \
  -dataset-root ./aiopschallenge2025 \
  -output-root ./aiopschallenge2025/baseline
```

当前命令会生成以下目录：

- `aiopschallenge2025/baseline/docs_evidence`
- `aiopschallenge2025/baseline/docs_history`
- `aiopschallenge2025/baseline/docs_evidence_build`
- `aiopschallenge2025/baseline/docs_history_build`
- `aiopschallenge2025/baseline/eval/eval_cases.jsonl`
- `aiopschallenge2025/baseline/eval/eval_cases_holdout.jsonl`
- `aiopschallenge2025/baseline/eval/build_split.json`
- `aiopschallenge2025/baseline/eval/eval_split.json`

含义：

- `docs_evidence` / `docs_history`：全量 400 case 的文档投影。
- `docs_evidence_build` / `docs_history_build`：只包含 build split 的 320 case。
- `eval_cases_holdout.jsonl`：只评 holdout split 的 160 条 query。

## 4. 为什么必须加 build-only 目录

这是这次流程里最重要的修正点。

如果直接：

1. 把 `docs_evidence` 全量 400 case 全部建索引
2. 再用 `eval_cases_holdout.jsonl` 去评

那么 holdout case 自己仍然在索引里，会产生 case 自检索泄漏。

这会让结果看起来更好，但不是真正的 holdout baseline。

所以严格流程必须是：

1. 只索引 `docs_*_build`
2. 只评 `eval_cases_holdout.jsonl`

## 5. 云端执行顺序

### 步骤 1：启动独立 Milvus

目录：

```bash
cd /opt/opscaptain/baseline-workspace/manifest/docker
docker compose -p opscaptain-rag -f docker-compose.remote.yml up -d etcd standalone
```

说明：

- 这套 compose 使用独立的 `milvus-etcd/minio/standalone`
- 不依赖现网站点容器
- 访问地址使用 `127.0.0.1:19530`

### 步骤 2：重新生成 baseline 产物

```bash
cd /opt/opscaptain/baseline-workspace
docker run --rm --network host \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go run ./internal/ai/cmd/aiops_rag_prep_cmd -dataset-root ./aiopschallenge2025 -output-root ./aiopschallenge2025/baseline'
```

### 步骤 3：索引 build-only evidence 集合

```bash
cd /opt/opscaptain/baseline-workspace
docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_build \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go run ./internal/ai/cmd/knowledge_cmd -dir ./aiopschallenge2025/baseline/docs_evidence_build'
```

### 步骤 4：评估 holdout evidence baseline

```bash
cd /opt/opscaptain/baseline-workspace
docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_build \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go run ./internal/ai/cmd/rag_online_eval_cmd -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout.jsonl -ks 1,3,5 -out ./aiopschallenge2025/baseline/eval/report_evidence_build.json'
```

### 步骤 5：索引 build-only history 集合

```bash
cd /opt/opscaptain/baseline-workspace
docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_history_build \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go run ./internal/ai/cmd/knowledge_cmd -dir ./aiopschallenge2025/baseline/docs_history_build'
```

### 步骤 6：评估 holdout history baseline

```bash
cd /opt/opscaptain/baseline-workspace
docker run --rm --network host \
  --env-file /opt/opscaptain/.env.production \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=sum.golang.google.cn \
  -e MILVUS_ADDRESS=127.0.0.1:19530 \
  -e MILVUS_COLLECTION=aiops_history_build \
  -v /opt/opscaptain/baseline-workspace:/work \
  -v /opt/opscaptain/go-cache/mod:/go/pkg/mod \
  -v /opt/opscaptain/go-cache/build:/root/.cache/go-build \
  -w /work golang:1.24 \
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go run ./internal/ai/cmd/rag_online_eval_cmd -eval ./aiopschallenge2025/baseline/eval/eval_cases_holdout.jsonl -ks 1,3,5 -out ./aiopschallenge2025/baseline/eval/report_history_build.json'
```

## 6. 这次实际修掉的问题

这次不是单纯跑命令，过程中修了几处会直接把 baseline 跑偏或跑挂的代码问题。

### 6.1 Indexer 向量类型错误

现象：

- Milvus collection 是 `FloatVector`
- indexer 默认把 embedding 转成 `[]byte`
- 插入时报 `expected []float32, got []uint8`

修复：

- 在 [indexer.go](/d:/Agent/OpsCaption/internal/ai/indexer/indexer.go) 里显式提供 `DocumentConverter`
- 把 `[][]float64` 转成 `[]float32`

### 6.2 Retriever 向量类型错误

现象：

- collection 是 `VECTOR_FLOAT`
- retriever 默认按 `VECTOR_BINARY` 查询

修复：

- 在 [retriever.go](/d:/Agent/OpsCaption/internal/ai/retriever/retriever.go) 里显式提供 `VectorConverter`
- 检索时改成 `entity.FloatVector`

### 6.3 Retriever MetricType/SearchParam 与索引不一致

现象：

- 索引侧是 `IP + HNSW`
- 检索侧默认还是依赖包的 `HAMMING + AUTOINDEXSearchParam(radius/range_filter)`
- Milvus 会直接拒绝搜索参数

修复：

- 在 [retriever.go](/d:/Agent/OpsCaption/internal/ai/retriever/retriever.go) 里显式设置：
  - `MetricType`
  - `SearchParam`
- `HNSW` 使用 `entity.NewIndexHNSWSearchParam(...)`

### 6.4 Eval ID 规范化不支持 Windows 路径

现象：

- 文档 `_source` 可能是 `D:\...`
- 在 Linux 上 `filepath.Base` 不会按反斜杠截断
- 会把整条 Windows 路径当成 doc id

修复：

- 在 [online.go](/d:/Agent/OpsCaption/internal/ai/rag/eval/online.go) 里先把 `\` 归一成 `/`
- 再取 basename

### 6.5 Holdout 流程本身有泄漏

现象：

- 预处理最初只生成全量 `docs_evidence/docs_history`
- 拿它去评 holdout 会泄漏

修复：

- 在 [aiops_baseline.go](/d:/Agent/OpsCaption/internal/ai/rag/eval/aiops_baseline.go) 里新增：
  - `docs_evidence_build`
  - `docs_history_build`

## 7. 验证方式

本次至少做了三层验证：

1. 本地相关包单测
2. 云端 Go 容器回归测试
3. 云端真实数据预处理 + 索引 + 在线评测

云端回归重点包：

- `./internal/ai/indexer`
- `./internal/ai/retriever`
- `./internal/ai/rag`
- `./internal/ai/rag/eval`
- `./utility/common`

## 8. 结果记录

### 8.1 废弃结果：全量 evidence 自检索

这一轮结果已经跑出来，但**不作为最终 baseline**，因为存在 holdout 泄漏。

当时结果是：

- `Cases: 160`
- `Avg Recall@1: 0.29`
- `Avg Recall@3: 0.59`
- `Avg Recall@5: 0.67`
- `Avg Total ms: 5157.68`

这组结果只能说明：

- 当前链路在全量 case 自检索条件下能跑通
- 不能说明严格 holdout 场景下的泛化效果

### 8.2 严格结果：build-only evidence

待本轮云端任务完成后补充。

### 8.3 严格结果：build-only history

待本轮云端任务完成后补充。

## 9. 如何解读这两套 baseline

如果后面两套严格结果都跑完，解读方式应该是：

- `evidence_build`：看当前系统靠“症状和观测信息”本身能召回到什么程度
- `history_build`：看加入历史标签知识后，召回和排序能提升多少

如果 `history_build` 明显优于 `evidence_build`，说明：

- 你的系统更像“历史案例辅助 RCA”
- 不是“纯证据检索型 RCA”

如果两者都低，优先怀疑：

- 文档投影质量
- query 构造质量
- chunk 方式
- embedding 模型
- rewrite/rerank 稳定性

## 10. 下一步建议

如果这轮 strict baseline 跑完，下一步最值得做的是：

1. 接 `parquet -> evidence summary`，把 evidence 文档从标签投影升级成真实遥测摘要。
2. 把 `report_*.json` 再加工成 Markdown/CSV 对比报告。
3. 加 `per-case failure analysis`，把 top miss 的 query 归因到 rewrite / retrieve / rerank 哪一层。
4. 再引入图谱过滤或 Graph-Enhanced RAG，对比 strict baseline 是否继续提升。
