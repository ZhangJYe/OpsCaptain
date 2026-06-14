#!/usr/bin/env python3
"""
准备 Embedding Fine-tune 训练数据
从 eval_cases.jsonl + evidence docs 构造 ARK 平台格式的训练对
"""

import json
import glob
import os

def load_eval_cases(path):
    """加载 eval cases"""
    cases = []
    with open(path) as f:
        for line in f:
            cases.append(json.loads(line.strip()))
    return cases

def load_evidence_docs(docs_dir):
    """加载 evidence 文档"""
    docs = {}
    for path in glob.glob(os.path.join(docs_dir, "*.md")):
        doc_id = os.path.splitext(os.path.basename(path))[0]
        with open(path) as f:
            docs[doc_id] = f.read().strip()
    return docs

def build_training_pairs(cases, docs):
    """构造训练对"""
    pairs = []
    for case in cases:
        query = case["query"]
        for relevant_id in case.get("relevant_ids", []):
            if relevant_id in docs:
                pairs.append({
                    "query": query,
                    "positive": docs[relevant_id]
                })
    return pairs

def main():
    eval_cases_path = "/opt/opscaptain/baseline-workspace/aiopschallenge2025/baseline/eval/eval_cases.jsonl"
    docs_dir = "/opt/opscaptain/baseline-workspace/aiopschallenge2025/baseline/docs_evidence_build"
    output_path = "/tmp/aiops_embedding_train.jsonl"

    cases = load_eval_cases(eval_cases_path)
    docs = load_evidence_docs(docs_dir)
    pairs = build_training_pairs(cases, docs)

    print(f"Loaded {len(cases)} eval cases")
    print(f"Loaded {len(docs)} evidence docs")
    print(f"Built {len(pairs)} training pairs")

    # 转换为 ARK 平台格式
    # ARK embedding fine-tune 格式: {"query": "...", "positive": "..."}
    with open(output_path, "w") as f:
        for pair in pairs:
            f.write(json.dumps(pair, ensure_ascii=False) + "\n")

    print(f"Saved to {output_path}")

    # 输出统计
    queries = set(p["query"] for p in pairs)
    print(f"Unique queries: {len(queries)}")
    print(f"Avg pairs per query: {len(pairs)/len(queries):.1f}")

if __name__ == "__main__":
    main()
