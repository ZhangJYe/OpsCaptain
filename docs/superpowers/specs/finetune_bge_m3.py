#!/usr/bin/env python3
"""
BGE-M3 Fine-tune 训练脚本
在本地 RTX 5070 上运行
"""

import json
import glob
import os
from sentence_transformers import SentenceTransformer, InputExample, losses
from torch.utils.data import DataLoader

# ========== 1. 加载训练数据 ==========

def load_eval_cases(path):
    cases = []
    with open(path) as f:
        for line in f:
            cases.append(json.loads(line.strip()))
    return cases

def load_evidence_docs(docs_dir):
    docs = {}
    for path in glob.glob(os.path.join(docs_dir, "*.md")):
        doc_id = os.path.splitext(os.path.basename(path))[0]
        with open(path) as f:
            docs[doc_id] = f.read().strip()
    return docs

def build_training_pairs(cases, docs):
    pairs = []
    for case in cases:
        query = case["query"]
        for relevant_id in case.get("relevant_ids", []):
            if relevant_id in docs:
                pairs.append(InputExample(
                    texts=[query, docs[relevant_id]],
                    label=1.0
                ))
    return pairs

# ========== 2. 训练 ==========

def train():
    # 路径配置 - 修改为你的实际路径
    eval_cases_path = "eval_cases.jsonl"
    docs_dir = "docs_evidence_build"
    output_dir = "./bge-m3-aiops-finetuned"

    print("Loading data...")
    cases = load_eval_cases(eval_cases_path)
    docs = load_evidence_docs(docs_dir)
    train_pairs = build_training_pairs(cases, docs)
    print(f"Training pairs: {len(train_pairs)}")

    print("Loading base model...")
    model = SentenceTransformer("BAAI/bge-m3")

    train_dataloader = DataLoader(train_pairs, shuffle=True, batch_size=16)
    train_loss = losses.CosineSimilarityLoss(model)

    print("Starting training...")
    model.fit(
        train_objectives=[(train_dataloader, train_loss)],
        epochs=3,
        warmup_steps=100,
        output_path=output_dir,
        show_progress_bar=True
    )
    print(f"Model saved to {output_dir}")

    # ========== 3. 验证 ==========

    print("\nEvaluating...")
    test_queries = [
        ("payment 服务延迟升高", "checkoutservice latency p99 spike"),
        ("OOM 容器重启", "pod CrashLoopBackOff OOMKilled"),
        ("Redis 连接超时", "redis connection timeout"),
    ]

    model_finetuned = SentenceTransformer(output_dir)
    model_base = SentenceTransformer("BAAI/bge-m3")

    for query, positive in test_queries:
        emb_finetuned = model_finetuned.encode([query, positive])
        emb_base = model_base.encode([query, positive])

        cos_finetuned = cosine_similarity(emb_finetuned[0], emb_finetuned[1])
        cos_base = cosine_similarity(emb_base[0], emb_base[1])

        print(f"Query: {query}")
        print(f"  Base:     {cos_base:.4f}")
        print(f"  Finetuned: {cos_finetuned:.4f}")
        print()

def cosine_similarity(a, b):
    import numpy as np
    return float(np.dot(a, b) / (np.linalg.norm(a) * np.linalg.norm(b)))

if __name__ == "__main__":
    train()
