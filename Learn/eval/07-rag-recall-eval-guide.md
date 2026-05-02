# OpsCaptain RAG 召回率评测入门

这份文档只做一件事：

教你作为初学者，怎么理解并复跑这次加进去的 RAG harness。

## 1. 这次我补了什么

新增了一个离线评测模块：

- [types.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/types.go)
- [runner.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/runner.go)
- [inmemory.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/inmemory.go)
- [samples.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/samples.go)
- [runner_test.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/runner_test.go)

还会把原来的 `recall_cmd` 改成真正可用的评测入口。

## 2. 为什么要做这个

因为你后面做 RAG，不可能只靠“感觉效果更好了”。

你至少要回答两个问题：

1. 检索出来的结果里，有没有包含正确文档？
2. 如果没有，是 top-k 太小，还是 query / chunk / embedding 有问题？

所以我先给你一个最小但可复跑的 harness。

## 3. 什么是 Recall@K

先记住一句最重要的话：

> Recall@K = 在前 K 个检索结果里，找回了多少“应该被找回的正确文档”。

举例：

- 某个问题真正相关的文档有 2 篇
- 你检索 top 3
- 最终前 3 个结果里命中了 1 篇

那么：

- `Recall@3 = 1 / 2 = 0.5`

如果前 3 个里两篇都命中了：

- `Recall@3 = 2 / 2 = 1.0`

## 4. 这次 harness 是怎么工作的

流程很简单：

1. 准备一组样本文档 corpus
2. 准备一组问题 cases
3. 为每个问题标注“正确文档 ID”
4. 跑检索
5. 统计 `Recall@1`、`Recall@3`

这次样例数据就在：

- [samples.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/samples.go)

你会看到两部分：

1. `SampleCorpus()`
   这是样本文档集合
2. `SampleCases()`
   这是评测问题，以及每个问题对应的正确文档 ID

## 5. 为什么先用离线样例，不直接跑 Milvus

因为你现在是初学者，先要学会“评测方法”，再去碰线上依赖。

离线样例的好处：

- 没有外部依赖
- 每次都能复跑
- 结果稳定
- 容易理解 Recall@K 是怎么算出来的

等你把这套看懂了，再把 `Searcher` 换成真实 Milvus retriever 就行。

## 6. 你以后怎么运行

在项目根目录执行：

```powershell
go run ./internal/ai/cmd/recall_cmd
```

你会看到：

- 总 case 数
- `avg_recall@1`
- `avg_recall@3`
- 每个 case 的命中情况

## 7. 你应该怎么读结果

### 如果 `Recall@1` 低，但 `Recall@3` 高

说明：

- 正确文档其实被召回了
- 只是排位不够靠前

这时通常优先看：

- rerank
- query rewrite
- 去重和排序

### 如果 `Recall@3` 也低

说明：

- 正确文档根本没进候选集

这时通常优先看：

- chunk 切分
- embedding
- top-k
- 文档内容质量

## 8. 面试时怎么讲

你可以这样说：

> 我不仅做了 RAG 模块化，还补了一套离线 harness。这个 harness 用固定的 corpus 和 golden cases 计算 Recall@K，让我能区分“没召回到”与“召回到了但排位不够高”，为后续 rerank、query rewrite 和 chunk 调优提供量化依据。

## 9. 下一步你可以怎么继续

如果你后面继续学，我建议你按这个顺序做：

1. 把样例 corpus 扩充到 10 到 20 条
2. 把 case 扩充到 10 条以上
3. 新增 `MRR`
4. 再接真实 Milvus retriever
5. 最后做 answer grounding 评测
