# RAG 遥测基线升级：本地预处理与云端轻量评测

这份文档不是普通 runbook，而是给面试讲解用的案例稿。

目标是把这次真实操作讲清楚：

1. 我最初想解决什么问题
2. 我做了哪些工程改造
3. 我为什么发现原方案不合理
4. 我如何通过资源诊断把方案改对
5. 最终应该怎么落地

## 1. 背景

项目是一个 AIOps/RAG 系统。前面已经有三类 baseline：

- `evidence_build`
- `history_build`
- `telemetry_evidence_build` 正在建设

原始 challenge 数据在：

- `aiopschallenge2025/input.json`
- `aiopschallenge2025/groundtruth.jsonl`
- `aiopschallenge2025/extracted/**/*.parquet`

核心问题是：

当前 RAG 的提升不明显，我需要判断到底是：

1. 语料本身不对
2. 检索策略不对
3. rewrite/rerank 在拖后腿

## 2. 我先做了什么

### 2.1 把原始 telemetry 转成证据文档

我先实现了：

- [build_telemetry_evidence.py](/d:/Agent/OpsCaption/scripts/aiops/build_telemetry_evidence.py)

作用是把：

- metric parquet
- log parquet
- trace parquet

转成当前系统可以直接索引的 markdown 证据文档。

输出包括：

- `docs_evidence_telemetry/`
- `docs_evidence_telemetry_build/`

### 2.2 给每个文档加 sidecar metadata

我又补了：

- `case.md`
- `case.metadata.json`

并让索引 loader 自动读取 sidecar：

- [loader.go](/d:/Agent/OpsCaption/internal/ai/loader/loader.go)

这样每个检索文档进入 Milvus 时，不只是正文，还会带上：

- `case_id`
- `service`
- `instance_type`
- `source`
- `destination`
- `metric_names`
- `trace_operations`

### 2.3 把评测拆成三种模式

我不再只看一个“黑盒结果”，而是把在线评测拆成三种 mode：

- `retrieve`
- `rewrite`
- `full`

对应代码：

- [query.go](/d:/Agent/OpsCaption/internal/ai/rag/query.go)
- [main.go](/d:/Agent/OpsCaption/internal/ai/cmd/rag_online_eval_cmd/main.go)

这样我就能区分：

1. 纯召回差
2. rewrite 把召回搞差
3. rerank 把排序搞差

## 3. 我最初走了一条“逻辑正确，但部署错误”的路

我当时想做的是：

1. 把完整 `extracted parquet` 上传到云端
2. 在云端做 telemetry 预处理
3. 云端直接建索引
4. 云端直接跑 baseline

这条路在逻辑上没错，但在资源上错了。

### 3.1 为什么错

云端服务器根盘只有 `40G`。

我实际查到的状态：

```text
/dev/vda2   40G   40G   0   100%
```

也就是说，服务器已经没有剩余磁盘了。

## 4. 我怎么定位出问题的

我没有继续盲传数据，而是先做磁盘归因。

我查到的大头是：

- `/opt/opscaptain`：约 `10G`
- `/opt/opscaptain/baseline-workspace/aiopschallenge2025/extracted`：约 `7.6G`
- `/var/lib/docker`：约 `9.1G`
- `/root`：约 `7.7G`

其中：

- `/var/lib/docker` 是容器层和镜像
- `/root` 里有大量历史工具缓存
- `/opt/opscaptain/baseline-workspace/aiopschallenge2025/extracted` 是我开始上传的原始 telemetry 数据

### 4.1 我学到的教训

这就是典型的部署经济学问题：

**“能在云端做”不等于“应该在云端做”。**

如果一台机器：

- 盘只有 `40G`
- 已经跑着生产容器
- Docker 层和用户缓存本来就不小

那就不应该再把 `11GB+` 的原始遥测数据全量塞进去做离线预处理。

## 5. 我最终修正成什么方案

我把架构改成：

### 正确方案：本地重预处理，云端轻 serving

本地做：

1. 读取 `extracted/**/*.parquet`
2. 生成 `docs_evidence_telemetry_build`
3. 生成 `*.metadata.json`
4. 生成 `doc_metadata_build.jsonl`

云端只做：

1. 接收已经处理好的轻量文档
2. 建索引
3. 跑评测

### 5.1 为什么这是正确的

因为两类机器适合干的事情不同：

#### 本地机器

适合：

- 大文件读取
- 原始数据预处理
- 迭代脚本
- 长时间 I/O

#### 云端机器

适合：

- 跑在线 Milvus
- 跑索引
- 跑评测
- 提供稳定环境

不适合：

- 存放完整原始遥测数据集

## 6. 我做的工程收口

为了支持这个修正后的方案，我又做了一个云端脚本：

- [run_telemetry_baseline_remote.sh](/d:/Agent/OpsCaption/scripts/aiops/run_telemetry_baseline_remote.sh)

并且把它改成了：

**即使云端没有 `extracted/`，也可以在 `--skip-telemetry` 模式下只做索引和评测。**

这点很关键。

如果不改这个脚本，文档里虽然说“本地预处理、云端轻量评测”，但脚本实际上还是强制要求云端有完整 `extracted/`，那就只是纸面方案，不是可执行方案。

## 7. 现在的标准流程

### 第一步：本地生成 telemetry build 文档

```powershell
python scripts/aiops/build_telemetry_evidence.py `
  --dataset-root aiopschallenge2025 `
  --output-root aiopschallenge2025/baseline
```

本地产物主要看：

- `aiopschallenge2025/baseline/docs_evidence_telemetry_build`
- `aiopschallenge2025/baseline/telemetry/doc_metadata_build.jsonl`

### 第二步：只上传轻量 build 产物

上传这类文件即可：

- `docs_evidence_telemetry_build/`
- 相关 `eval/*.jsonl`（如果本地也重建了 split）

不要再上传完整 `extracted/`。

### 第三步：云端只做索引和评测

如果云端已经有文档目录，那么直接：

```bash
cd /opt/opscaptain/baseline-workspace
bash scripts/aiops/run_telemetry_baseline_remote.sh \
  --start-milvus \
  --skip-prep \
  --skip-telemetry \
  --mode full
```

如果只想看纯召回：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh \
  --start-milvus \
  --skip-prep \
  --skip-telemetry \
  --mode retrieve
```

## 8. 面试时我会怎么讲这件事

可以直接按下面这个结构说。

### 8.1 先讲目标

“我当时不是盲目优化 RAG，而是在做基线分解。我想知道问题在语料、检索还是 rerank，所以我先做了 telemetry evidence builder 和多模式评测。”

### 8.2 再讲错误路径

“我一开始打算把完整 raw parquet 上传到云端做预处理，这在逻辑上能跑，但很快把 40G 根盘顶满了。这个时候我没有继续硬跑，而是先做资源归因。”

### 8.3 再讲诊断

“我把磁盘用量拆开看，发现真正的大头是 `/opt/opscaptain`、`/var/lib/docker`、`/root` 缓存，再叠加我上传的原始 telemetry 数据，最终导致根盘 100%。这说明问题不是单一目录，而是部署策略本身不合理。”

### 8.4 最后讲修正

“所以我把方案改成了本地做重预处理，云端只接收 build 产物、建索引和跑评测。与此同时我还把云端脚本改成支持 `--skip-telemetry`，保证这条轻量路径是真正可执行的，不只是纸面设计。”

## 9. 这件事能体现什么能力

这件事最能体现的不是“我会写脚本”，而是下面四点：

1. **问题分解能力**
   - 把 RAG 问题拆成语料层、召回层、rewrite 层、rerank 层

2. **工程闭环能力**
   - 不只是写 builder，还把 metadata、eval mode、remote runner 接成一条链

3. **资源意识**
   - 明白原始数据预处理和在线 serving 不该混在一台小盘机器上

4. **纠错能力**
   - 发现方案会把盘打爆后，没有硬撑，而是及时切换到本地预处理、云端轻量评测

## 10. 一句话总结

如果面试官只给你一分钟，你可以这样总结：

**“我做的不是单点调参，而是把 AIOps RAG 基线从黑盒试错改成了可解释、可评测、可落地的工程流程；并且在云端资源约束下，把方案从错误的全量上云，修正成了本地重预处理、云端轻 serving 的正确架构。”**
