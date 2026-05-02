# OpsCaptain RAG Eval 样例结果

这是我在当前仓库里实际运行下面这条命令得到的结果：

```powershell
go run ./internal/ai/cmd/rag_eval_cmd
```

命令入口在这里：

- [main.go](D:/Agent/OpsCaptionAI/internal/ai/cmd/rag_eval_cmd/main.go)

## 本次结果

```text
RAG Recall Harness
cases=5
avg_recall@1=0.90 hit_rate@1=1.00 full_recall_cases@1=4
avg_recall@3=1.00 hit_rate@3=1.00 full_recall_cases@3=5
```

## 逐条结果

```text
RAG-01 recall@1=1.00 recall@3=1.00 ranked=[payment-timeout-sop mysql-lock-troubleshooting milvus-connection-playbook] relevant=[payment-timeout-sop]
RAG-02 recall@1=1.00 recall@3=1.00 ranked=[prometheus-alert-triage payment-timeout-sop mysql-lock-troubleshooting] relevant=[prometheus-alert-triage]
RAG-03 recall@1=1.00 recall@3=1.00 ranked=[milvus-connection-playbook payment-timeout-sop login-rate-limit-guide] relevant=[milvus-connection-playbook]
RAG-04 recall@1=1.00 recall@3=1.00 ranked=[mysql-lock-troubleshooting prometheus-alert-triage login-rate-limit-guide] relevant=[mysql-lock-troubleshooting]
RAG-05 recall@1=0.50 recall@3=1.00 ranked=[payment-timeout-sop prometheus-alert-triage milvus-connection-playbook] relevant=[payment-timeout-sop prometheus-alert-triage]
```

## 怎么读这组数据

### 1. `avg_recall@1=0.90`

意思是：

- 平均来看
- 每个问题在第 1 个结果里
- 已经找回了 90% 的正确文档

这说明当前样例检索器的“第一名结果”已经比较靠谱。

### 2. `avg_recall@3=1.00`

意思是：

- 只要你看前 3 个结果
- 所有 case 的正确文档都能被找回来

这说明当前样例里：

- 召回能力够了
- 但排序还可以继续优化

### 3. 为什么 `RAG-05` 很重要

`RAG-05` 的结果是：

- `Recall@1 = 0.50`
- `Recall@3 = 1.00`

这正好是一个非常典型的 RAG 信号：

- 候选集没问题
- 排序还有空间

也就是：

- 不是“召回不到”
- 而是“召回到了，但第一名不够稳”

这类问题通常优先考虑：

- rerank
- query rewrite
- 结果去重和排序特征

## 这份结果对你面试有什么帮助

你可以这样讲：

> 我给项目补了一套离线 retrieval harness，并跑出一组可复现的 sample 指标。当前样例的 `avg_recall@1` 是 0.90，`avg_recall@3` 是 1.00，这说明候选召回已经够用，但排序仍有优化空间。这样我后续做 rerank 或 query rewrite 时，就有了可量化的对照基线。
