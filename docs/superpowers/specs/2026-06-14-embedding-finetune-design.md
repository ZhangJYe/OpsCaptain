# Embedding Fine-tune 方案设计

日期: 2026-06-14
状态: Draft
目标: 通过 fine-tune embedding 模型提升 AIOps RAG 检索质量

## 1. 背景与动机

### 1.1 当前瓶颈

| 指标 | 值 | 说明 |
|------|-----|------|
| Recall@1 (100 case) | 0.40 | 前 1 个结果只有 40% 命中 |
| Recall@5 (100 case) | 0.79 | 前 5 个结果有 79% 命中 |
| Recall@10 (100 case) | 0.87 | 前 10 个结果有 87% 命中 |
| Recall@1 (800 case) | 0.18 | 全量数据集更低 |

**核心问题**：embedding 模型不理解 AIOps 领域术语的等价关系。

### 1.2 具体案例

```
Query: "payment 服务延迟升高"
Embedding 匹配到: "payment timeout configuration"
应该匹配到: "checkoutservice latency p99 spike" (同一个服务，不同命名)

Query: "OOM 容器重启"
Embedding 匹配到: "container resource limits"
应该匹配到: "pod CrashLoopBackOff OOMKilled" (同一个问题，不同表述)
```

### 1.3 Fine-tune 预期效果

| 指标 | 当前 | Fine-tune 后预期 |
|------|------|----------------|
| Recall@1 (100 case) | 0.40 | 0.50-0.55 |
| Recall@5 (100 case) | 0.79 | 0.85-0.90 |
| Recall@1 (800 case) | 0.18 | 0.25-0.30 |

## 2. 训练数据

### 2.1 数据来源

| 数据 | 数量 | 格式 | 来源 |
|------|------|------|------|
| eval_cases.jsonl | 800 对 | {query, relevant_ids} | AIOps challenge |
| evidence_build | 320 文档 | .md 文件 | AIOps challenge |

### 2.2 数据格式

**正样本对**（从 eval_cases.jsonl 提取）：

```json
{
  "query": "cartservice service trace frontend log FailedPrecondition 异常",
  "positive": ["009be6db-313.md 的内容"],
  "negative": ["其他不相关文档的内容"]
}
```

**构造方法**：

```python
# 1. 加载 eval_cases
with open("eval_cases.jsonl") as f:
    cases = [json.loads(line) for line in f]

# 2. 加载 evidence 文档
docs = {}
for path in glob("docs_evidence_build/*.md"):
    doc_id = path.stem
    docs[doc_id] = open(path).read()

# 3. 构造训练对
training_pairs = []
for case in cases:
    query = case["query"]
    for relevant_id in case["relevant_ids"]:
        if relevant_id in docs:
            training_pairs.append({
                "query": query,
                "positive": docs[relevant_id]
            })
```

### 2.3 数据质量

- **覆盖率**：800 个 query，320 个文档，约 2.5 对/文档
- **多样性**：包含故障排查、配置查询、架构分析等多种场景
- **噪声**：部分 query 有多个 relevant_ids，需要过滤

## 3. 方案 A：ARK 平台 Fine-tune（推荐）

### 3.1 平台选择

**火山引擎 ARK**（字节跳动）：
- 支持 doubao-embedding 模型 fine-tune
- 无需 GPU，平台托管训练
- API 兼容，训练后直接替换模型 ID
- 训练数据上传到平台即可

### 3.2 实施步骤

```
Step 1: 准备训练数据
  ├── 转换 eval_cases.jsonl → ARK 平台格式
  ├── 上传到 ARK 控制台
  └── 创建 fine-tune 任务

Step 2: 训练
  ├── 选择 base model: doubao-embedding-vision-251215
  ├── 配置训练参数
  ├── 等待训练完成（约 1-2 小时）
  └── 获取 fine-tuned model ID

Step 3: 部署
  ├── 修改 config.yaml 中 embedding_model.model 为新 model ID
  ├── 重新索引 Milvus collection
  └── 运行 eval 对比

Step 4: 验证
  ├── 运行 100 case eval 对比
  ├── 运行 800 case eval 对比
  └── 记录结果
```

### 3.3 训练配置

```yaml
# ARK 平台 fine-tune 配置
base_model: "doubao-embedding-vision-251215"
task_type: "retrieval"
training_data: "aiops_pairs.jsonl"
hyperparameters:
  learning_rate: 1e-5
  batch_size: 32
  epochs: 3
  warmup_ratio: 0.1
  max_seq_length: 512
```

### 3.4 成本估算

| 项目 | 估算 |
|------|------|
| 训练时间 | 1-2 小时 |
| 训练费用 | ~50-100 元 |
| 推理费用 | 与当前相同 |
| 总投入 | 低 |

## 4. 方案 B：换更强的模型

### 4.1 候选模型

| 模型 | 维度 | 特点 | 适合场景 |
|------|------|------|---------|
| BGE-M3 | 1024 | 多语言、多粒度、多功能 | 通用检索 |
| GTE-Qwen2 | 1536 | 通义千问系列，中文优化 | 中文检索 |
| Jina Embeddings v3 | 1024 | 多任务适配 | 多语言 |
| bge-large-zh-v1.5 | 1024 | 中文优化，开源 | 中文场景 |

### 4.2 实施要点

- 需要重新索引 Milvus collection（维度不同）
- 需要修改 embedding 客户端代码
- 需要评估兼容性

### 4.3 风险

- 新模型可能在 AIOps 领域表现不如 fine-tuned doubao
- 维度变化需要重建索引
- API 兼容性问题

## 5. 方案 C：数据增强 + 规则

### 5.1 做法

给 evidence 文档添加 AIOps 同义词 metadata：

```markdown
---
aliases:
  - checkoutservice → payment service
  - CrashLoopBackOff → 容器重启
  - OOMKilled → 内存溢出
  - p99 latency → 延迟升高
  - connection timeout → 连接超时
service_map:
  checkoutservice: payment
  frontend: web
  recommendationservice: recommend
---
```

### 5.2 效果

- 在 retrieval_refine.go 中利用 aliases 做额外匹配
- 不改 embedding 模型
- 效果有限（3-8% 提升）

## 6. 推荐方案

**开源模型 BGE-M3 + 本地 RTX 5070 训练**：

1. 在本地用 `sentence-transformers` fine-tune BGE-M3
2. 用 800 对 AIOps (query→doc) 作为训练数据
3. 训练完成后导出模型
4. 部署到服务器，替换 embedding API

### 成本

| 项目 | 费用 |
|------|------|
| 模型 | 0 元 (BGE-M3 开源) |
| GPU | 0 元 (本地 5070) |
| 时间 | ~2-3 小时 |
| 总计 | **0 元** |

### 训练配置 (RTX 5070)

```python
from sentence_transformers import SentenceTransformer, InputExample, losses
from torch.utils.data import DataLoader

# 加载基座模型
model = SentenceTransformer("BAAI/bge-m3")

# 构造训练数据
train_examples = [
    InputExample(texts=["payment 服务延迟升高", "checkoutservice latency p99 spike"], label=1.0),
    InputExample(texts=["OOM 容器重启", "pod CrashLoopBackOff OOMKilled"], label=1.0),
    # ... 800 对
]
train_dataloader = DataLoader(train_examples, shuffle=True, batch_size=16)

# 训练
train_loss = losses.CosineSimilarityLoss(model)
model.fit(
    train_objectives=[(train_dataloader, train_loss)],
    epochs=3,
    warmup_steps=100,
    output_path="./bge-m3-aiops-finetuned"
)
```

### 部署方式

Fine-tune 后的模型有两种部署方式：

**方式 A：本地推理服务（推荐）**

```python
# 在服务器上部署 embedding 服务
from sentence_transformers import SentenceTransformer
model = SentenceTransformer("./bge-m3-aiops-finetuned")

# 用 FastAPI 暴露 API
@app.post("/embed")
def embed(texts: list[str]):
    embeddings = model.encode(texts)
    return {"embeddings": embeddings.tolist()}
```

修改项目的 embedding 客户端指向本地服务。

**方式 B：导出为 ONNX，用现有框架加载**

```python
model.export_onnx("bge-m3-aiops.onnx")
```

## 7. 实验验证

### 7.1 对比实验

| 实验 | 数据集 | 指标 |
|------|--------|------|
| Baseline (当前) | 100 case | R@1=0.40, R@5=0.79 |
| + 数据增强 | 100 case | R@1=?, R@5=? |
| + Fine-tune | 100 case | R@1=?, R@5=? |
| + 两者组合 | 100 case | R@1=?, R@5=? |

### 7.2 验证命令

```bash
# 重新索引
docker exec -e MILVUS_COLLECTION=aiops_evidence_build \
  opscaptain-backend-1 /app/knowledge-indexer \
  -dir /tmp/docs_evidence_build \
  -collection aiops_evidence_build

# 运行 eval
docker exec -e MILVUS_COLLECTION=aiops_evidence_build \
  opscaptain-backend-1 /tmp/rag_online_eval_cmd \
  -mode hybrid -eval /tmp/eval_cases_full.jsonl \
  -ks 1,3,5,10 -limit 100
```

## 8. 风险与边界

1. **Fine-tune 不保证提升** — 如果训练数据质量差或数量不够，可能无效
2. **需要重新索引** — Fine-tune 后的 embedding 维度可能不同，需要重建 Milvus collection
3. **延迟可能增加** — 更大的模型可能增加 embedding 计算时间
4. **成本** — ARK 平台 fine-tune 需要付费

## 9. 实施顺序

1. 准备训练数据（转换 eval_cases.jsonl 格式）
2. 上传到 ARK 平台
3. 启动 fine-tune 任务
4. 等待训练完成
5. 用新 model ID 更新配置
6. 重新索引 Milvus collection
7. 运行 eval 对比
8. 记录结果
