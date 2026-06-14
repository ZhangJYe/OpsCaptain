# 火山引擎 ARK Embedding Fine-tune 操作指南

> 基于火山引擎官方文档整理，2026-06-14

---

## 前提条件

1. 注册火山引擎账号：https://console.volcengine.com
2. 开通「火山方舟」服务
3. 准备 ARK API Key（你已有：ark-997035b9...）

---

## Step 1：准备训练数据

### 1.1 数据格式

ARK embedding fine-tune 需要 **query-document 正样本对**，JSONL 格式：

```jsonl
{"query": "payment 服务延迟升高", "document": "checkoutservice latency p99 spike..."}
{"query": "OOM 容器重启", "document": "pod CrashLoopBackOff OOMKilled..."}
```

### 1.2 生成训练数据

在你的 5070 主机上运行（或在服务器上生成后下载）：

```python
import json

# 从 eval_cases.jsonl 生成
cases = []
with open("eval_cases.jsonl") as f:
    for line in f:
        cases.append(json.loads(line.strip()))

docs = {}
for path in glob.glob("docs_evidence_build/*.md"):
    doc_id = path.stem
    with open(path) as f:
        docs[doc_id] = f.read().strip()

# 转换为 ARK 格式
with open("ark_train_data.jsonl", "w") as f:
    for case in cases:
        query = case["query"]
        for relevant_id in case.get("relevant_ids", []):
            if relevant_id in docs:
                f.write(json.dumps({
                    "query": query,
                    "document": docs[relevant_id]
                }, ensure_ascii=False) + "\n")

print(f"Generated {count} training pairs")
```

### 1.3 数据量要求

| 要求 | 最低 | 推荐 |
|------|------|------|
| 训练对数量 | 100+ | 500+ |
| 每条 document 长度 | 50-2000 字 | 100-500 字 |
| query 长度 | 5-100 字 | 10-50 字 |

你有 800 对，完全满足要求。

---

## Step 2：上传数据到 ARK

### 2.1 登录控制台

访问：https://console.volcengine.com/ark

### 2.2 创建数据集

1. 左侧菜单 → **模型精调** → **数据集管理**
2. 点击 **创建数据集**
3. 填写：
   - 数据集名称：`aiops-rag-finetune`
   - 数据类型：**Embedding 微调**
   - 上传 `ark_train_data.jsonl`
4. 等待数据校验完成

---

## Step 3：创建精调任务

### 3.1 进入精调页面

1. 左侧菜单 → **模型精调** → **创建精调任务**
2. 选择任务类型：**Embedding 微调**

### 3.2 配置任务

| 配置项 | 值 | 说明 |
|--------|-----|------|
| 基座模型 | `doubao-embedding-vision-251215` | 你当前使用的模型 |
| 数据集 | `aiops-rag-finetune` | Step 2 创建的 |
| 训练轮数 | 3 | 通常 2-5 轮 |
| 学习率 | 1e-5 | 默认即可 |
| Batch Size | 32 | 根据数据量调整 |

### 3.3 启动训练

1. 确认配置无误
2. 点击 **提交训练**
3. 等待训练完成（通常 1-2 小时）

---

## Step 4：获取精调模型 ID

训练完成后：

1. 在 **模型精调** → **任务列表** 中找到已完成的任务
2. 点击任务详情
3. 复制 **精调后模型 ID**（类似：`ep-xxxxxxxxxxxx`）

---

## Step 5：更新项目配置

### 5.1 修改 config.yaml

```yaml
embedding_model:
  model: "ep-xxxxxxxxxxxx"  # 精调后的模型 ID
  api_key: "${ARK_API_KEY}"
  base_url: "https://ark.cn-beijing.volces.com/api/v3"
  dimension: 2048  # 维度不变
```

### 5.2 重新索引 Milvus

```bash
# 在服务器上
docker exec -e MILVUS_ADDRESS=milvus:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_build \
  opscaptain-backend-1 /app/knowledge-indexer \
  -dir /tmp/docs_evidence_build \
  -collection aiops_evidence_build
```

### 5.3 重启服务

```bash
cd /opt/opscaptain
set -a && . ./release.env && set +a
docker compose --env-file .env.production -f docker-compose.prod.yml restart backend
```

---

## Step 6：验证效果

```bash
# 运行 eval 对比
docker exec -e MILVUS_ADDRESS=milvus:19530 \
  -e MILVUS_COLLECTION=aiops_evidence_build \
  opscaptain-backend-1 /tmp/rag_online_eval_cmd \
  -mode hybrid -eval /tmp/eval_cases_full.jsonl \
  -ks 1,3,5,10 -limit 100
```

对比 fine-tune 前后的 Recall@1/3/5/10。

---

## 注意事项

1. **精调后模型只能在 ARK 平台调用**，不能导出到本地
2. **维度不变**：doubao-embedding 精调后仍是 2048 维，Milvus collection 不需要重建索引
3. **API 兼容**：精调后的模型 ID 直接替换原 model ID，其他代码不用改
4. **费用**：精调训练 ~50-100 元，推理费用与原模型相同

---

## 常见问题

### Q: 精调后效果没有提升？
- 检查训练数据质量（正样本对是否准确）
- 增加训练轮数（epochs=5）
- 检查 eval 时是否用了正确的 collection

### Q: 精调任务失败？
- 检查数据格式是否符合要求
- 检查 API Key 权限
- 联系火山引擎技术支持

### Q: 能不能同时用 fine-tuned 和原始模型？
- 可以，部署两个 embedding endpoint，A/B 测试
