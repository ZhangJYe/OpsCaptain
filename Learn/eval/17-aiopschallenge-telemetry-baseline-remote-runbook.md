# aiopschallenge2025 Telemetry Baseline Remote Runbook

这份文档对应一个已经实现好的远端脚本入口，不再需要手工串四段 `docker run`。

脚本位置：

- [run_telemetry_baseline_remote.sh](/d:/Agent/OpsCaption/scripts/aiops/run_telemetry_baseline_remote.sh)

Milvus compose：

- [docker-compose.remote.yml](/d:/Agent/OpsCaption/manifest/docker/docker-compose.remote.yml)

## 1. 这条脚本会做什么

按顺序执行：

1. 可选启动独立 Milvus
2. 运行 `aiops_rag_prep_cmd`
3. 运行 `build_telemetry_evidence.py`
4. 索引 `docs_evidence_telemetry_build`
5. 跑严格版 `holdout_related` 评测

默认 collection：

- `aiops_evidence_telemetry_build`

默认报告：

- `aiopschallenge2025/baseline/eval/report_evidence_telemetry_build_related.json`

## 2. 前提

在云端工作目录执行：

```bash
cd /opt/opscaptain/baseline-workspace
git pull origin main
```

并确保以下目录已存在：

- `aiopschallenge2025/input.json`
- `aiopschallenge2025/groundtruth.jsonl`
- `aiopschallenge2025/extracted/`

如果 `extracted/` 还没同步到云端，先从本地传：

```powershell
scp -r D:\Agent\OpsCaption\aiopschallenge2025\extracted tencent-opscaptain:/opt/opscaptain/baseline-workspace/aiopschallenge2025/
```

## 3. 一键全量运行

```bash
cd /opt/opscaptain/baseline-workspace
bash scripts/aiops/run_telemetry_baseline_remote.sh --start-milvus
```

## 4. 常用变体

只做 5 个 case 冒烟：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --start-milvus --limit 5
```

Milvus 已经在跑，只重做 telemetry + 索引 + 评测：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --skip-prep
```

只重跑评测：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --skip-prep --skip-telemetry --skip-index
```

换 collection：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --collection aiops_evidence_telemetry_build_v2
```

## 5. 关键输出

生成产物：

- `aiopschallenge2025/baseline/docs_evidence_telemetry/`
- `aiopschallenge2025/baseline/docs_evidence_telemetry_build/`
- `aiopschallenge2025/baseline/telemetry/telemetry_report.json`

评测报告：

- `aiopschallenge2025/baseline/eval/report_evidence_telemetry_build_related.json`

## 6. 最小排障

如果脚本一开始就失败，优先看这几项：

1. `aiopschallenge2025/extracted/` 是否已同步到云端
2. `docker ps` 是否正常
3. `/opt/opscaptain/.env.production` 是否存在
4. `127.0.0.1:19530` 是否被别的 Milvus 占用

如果你只想验证脚本参数是否正常：

```bash
bash scripts/aiops/run_telemetry_baseline_remote.sh --help
```
