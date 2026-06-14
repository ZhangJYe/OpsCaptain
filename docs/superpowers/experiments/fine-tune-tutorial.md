# BGE-M3 Fine-tune 完整教程

> 在你的 RTX 5070 主机上操作，0 元成本

---

## Step 1：在服务器上准备训练数据

SSH 到你的服务器 124.222.57.178，运行：

```bash
# 创建训练数据目录
mkdir -p /opt/opscaptain/finetune

# 用 Python 生成训练数据
python3 << 'EOF'
import json, glob, os

# 加载 eval cases (query → relevant_ids)
cases = []
with open("/opt/opscaptain/baseline-workspace/aiopschallenge2025/baseline/eval/eval_cases.jsonl") as f:
    for line in f:
        cases.append(json.loads(line.strip()))

# 加载 evidence 文档
docs = {}
for path in glob.glob("/opt/opscaptain/baseline-workspace/aiopschallenge2025/baseline/docs_evidence_build/*.md"):
    doc_id = os.path.splitext(os.path.basename(path))[0]
    with open(path) as f:
        docs[doc_id] = f.read().strip()

# 构造训练对 (sentence-transformers 格式)
pairs = []
for case in cases:
    query = case["query"]
    for relevant_id in case.get("relevant_ids", []):
        if relevant_id in docs:
            pairs.append({"texts": [query, docs[relevant_id]], "label": 1.0})

# 保存
with open("/opt/opscaptain/finetune/train_pairs.jsonl", "w") as f:
    for p in pairs:
        f.write(json.dumps(p, ensure_ascii=False) + "\n")

print(f"Done! {len(pairs)} training pairs saved")
EOF

# 打包训练数据和 evidence 文档
cd /opt/opscaptain
tar czf finetune_data.tar.gz \
  finetune/train_pairs.jsonl \
  baseline-workspace/aiopschallenge2025/baseline/docs_evidence_build/
```

---

## Step 2：在你的 5070 主机上操作

### 2.1 安装 Python 环境

如果你还没有 Python 环境：

```bash
# 安装 conda (如果没有的话)
wget https://repo.anaconda.com/miniconda/Miniconda3-latest-Linux-x86_64.sh
bash Miniconda3-latest-Linux-x86_64.sh -b
source ~/miniconda3/bin/activate

# 创建虚拟环境
conda create -n rag-finetune python=3.11 -y
conda activate rag-finetune
```

### 2.2 安装依赖

```bash
pip install sentence-transformers torch torchvision --index-url https://download.pytorch.org/whl/cu121
pip install numpy
```

### 2.3 从服务器下载训练数据

```bash
# 在你的 5070 主机上运行
scp root@124.222.57.178:/opt/opscaptain/finetune_data.tar.gz .

# 解压
tar xzf finetune_data.tar.gz
```

---

## Step 3：运行训练

创建训练脚本：

```bash
cat > train.py << 'PYEOF'
import json
import numpy as np
from sentence_transformers import SentenceTransformer, InputExample, losses
from torch.utils.data import DataLoader

print("=" * 50)
print("BGE-M3 AIOps Fine-tune")
print("=" * 50)

# 1. 加载训练数据
print("\n[1/4] Loading training pairs...")
pairs = []
with open("finetune/train_pairs.jsonl") as f:
    for line in f:
        data = json.loads(line.strip())
        pairs.append(InputExample(texts=data["texts"], label=data["label"]))
print(f"  Loaded {len(pairs)} pairs")

# 2. 加载基座模型
print("\n[2/4] Loading BGE-M3 model...")
model = SentenceTransformer("BAAI/bge-m3")
print(f"  Model loaded: {model.get_sentence_embedding_dimension()}d")

# 3. 训练
print("\n[3/4] Training (RTX 5070, batch=32, epochs=3)...")
train_dataloader = DataLoader(pairs, shuffle=True, batch_size=32)
train_loss = losses.CosineSimilarityLoss(model)

model.fit(
    train_objectives=[(train_dataloader, train_loss)],
    epochs=3,
    warmup_steps=100,
    output_path="./bge-m3-aiops-finetuned",
    show_progress_bar=True,
    checkpoint_path="./checkpoints",
    checkpoint_save_steps=500,
)
print("  Training complete!")

# 4. 验证
print("\n[4/4] Evaluating...")
test_pairs = [
    ("payment 服务延迟升高", "checkoutservice latency p99 spike"),
    ("OOM 容器重启", "pod CrashLoopBackOff OOMKilled"),
    ("Redis 连接超时", "redis connection timeout"),
    ("MySQL 死锁", "innodb deadlock detected"),
    ("Prometheus 告警", "alert rule firing"),
]

model_ft = SentenceTransformer("./bge-m3-aiops-finetuned")
model_base = SentenceTransformer("BAAI/bge-m3")

print(f"\n{'Query':<30} {'Base':>8} {'Fine-tuned':>12} {'Delta':>8}")
print("-" * 60)
for query, positive in test_pairs:
    emb_ft = model_ft.encode([query, positive])
    emb_base = model_base.encode([query, positive])
    cos_ft = float(np.dot(emb_ft[0], emb_ft[1]) / (np.linalg.norm(emb_ft[0]) * np.linalg.norm(emb_ft[1])))
    cos_base = float(np.dot(emb_base[0], emb_base[1]) / (np.linalg.norm(emb_base[0]) * np.linalg.norm(emb_base[1])))
    delta = cos_ft - cos_base
    print(f"{query:<30} {cos_base:>8.4f} {cos_ft:>12.4f} {delta:>+8.4f}")

print("\n" + "=" * 50)
print("Model saved to: ./bge-m3-aiops-finetuned/")
print("=" * 50)
PYEOF

# 开始训练
python train.py
```

**训练时间预估**：
- 800 对 × 3 epochs ÷ 32 batch = ~75 steps
- RTX 5070: ~2-3 分钟
- 加上模型加载和评估: ~5-10 分钟

---

## Step 4：把模型部署到服务器

### 4.1 打包模型

```bash
# 在 5070 主机上
cd bge-m3-aiops-finetuned
tar czf ../bge-m3-aiops-finetuned.tar.gz .
cd ..
```

### 4.2 上传到服务器

```bash
scp bge-m3-aiops-finetuned.tar.gz root@124.222.57.178:/opt/opscaptain/
```

### 4.3 在服务器上解压

```bash
ssh root@124.222.57.178 "
  mkdir -p /opt/opscaptain/bge-m3-aiops-finetuned
  cd /opt/opscaptain
  tar xzf bge-m3-aiops-finetuned.tar.gz -C bge-m3-aiops-finetuned
  ls -la bge-m3-aiops-finetuned/
"
```

---

## Step 5：部署 embedding 服务

### 5.1 安装 Python 环境（服务器上）

```bash
ssh root@124.222.57.178 "
  apt update && apt install -y python3-pip
  pip3 install sentence-transformers fastapi uvicorn
"
```

### 5.2 创建 embedding 服务

```bash
ssh root@124.222.57.178 "cat > /opt/opscaptain/embedding_server.py << 'PYEOF'
from fastapi import FastAPI
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer
import numpy as np

app = FastAPI()
model = SentenceTransformer(\"/opt/opscaptain/bge-m3-aiops-finetuned\")

class EmbedRequest(BaseModel):
    texts: list[str]

@app.post(\"/embed\")
def embed(req: EmbedRequest):
    embeddings = model.encode(req.texts, normalize_embeddings=True)
    return {\"embeddings\": embeddings.tolist()}

@app.get(\"/health\")
def health():
    return {\"status\": \"ok\", \"dimension\": model.get_sentence_embedding_dimension()}

if __name__ == \"__main__\":
    import uvicorn
    uvicorn.run(app, host=\"0.0.0.0\", port=6333)
PYEOF
"
```

### 5.3 启动服务

```bash
ssh root@124.222.57.178 "
  nohup python3 /opt/opscaptain/embedding_server.py > /opt/opscaptain/embedding_server.log 2>&1 &
  echo \"Embedding server started on port 6333\"
  sleep 3
  curl http://localhost:6333/health
"
```

---

## Step 6：验证效果

```bash
# 测试 embedding 服务
ssh root@124.222.57.178 "
  curl -s http://localhost:6333/embed \\
    -H 'Content-Type: application/json' \\
    -d '{\"texts\": [\"payment 服务延迟升高\", \"checkoutservice latency p99 spike\"]}' \\
  | python3 -c 'import json,sys;d=json.load(sys.stdin);print(f\"Dimension: {len(d[\"embeddings\"][0])}\")'
"
```

---

## 常见问题

### Q: 训练时 CUDA out of memory
```bash
# 降低 batch_size
# 在 train.py 中把 batch_size=32 改为 batch_size=16
```

### Q: sentence-transformers 安装失败
```bash
# 确保 PyTorch 先安装
pip install torch torchvision --index-url https://download.pytorch.org/whl/cu121
pip install sentence-transformers
```

### Q: 服务器没有 Python 环境
```bash
# 用 Docker
docker run -d -p 6333:6333 --name embedding \\
  -v /opt/opscaptain/bge-m3-aiops-finetuned:/model \\
  python:3.11-slim \\
  bash -c "pip install sentence-transformers fastapi uvicorn && python -c 'from sentence_transformers import SentenceTransformer; m=SentenceTransformer(\"/model\")'"
```

---

## 完整流程图

```
你的 5070 主机                        服务器 (124.222.57.178)
    │                                      │
    │  ← scp 下载训练数据                    │
    │    finetune_data.tar.gz              │
    │                                      │
    │  训练 BGE-M3 (~5-10 min)             │
    │  batch=32, epochs=3                  │
    │                                      │
    │  → scp 上传模型                       │
    │    bge-m3-aiops-finetuned.tar.gz     │
    │                                      │
    │           ┌──────────────────────────┤
    │           │ 部署 embedding 服务       │
    │           │ port 6333                │
    │           │                          │
    │           │ 重新索引 Milvus          │
    │           │ 运行 eval 对比           │
    │           └──────────────────────────┘
```
