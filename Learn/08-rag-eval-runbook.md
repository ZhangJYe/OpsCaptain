# OpsCaptain RAG Eval 操作手册

这份是最终可执行版本，直接告诉你怎么跑这次补上的离线 RAG 评测。

## 入口

命令入口：

- [main.go](D:/Agent/OpsCaptionAI/internal/ai/cmd/rag_eval_cmd/main.go)

评测核心：

- [runner.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/runner.go)
- [samples.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/samples.go)

## 运行命令

```powershell
go run ./internal/ai/cmd/rag_eval_cmd
```

## 你会看到什么

运行后会输出 3 类信息：

1. 总 case 数
2. `avg_recall@1` 和 `avg_recall@3`
3. 每个 case 的命中详情

## 怎么解读

如果：

- `Recall@1` 低
- 但 `Recall@3` 高

说明：

- 正确文档已经被召回
- 只是排序不够靠前

你下一步优先改：

- rerank
- query rewrite
- 排序策略

如果：

- `Recall@3` 也低

说明：

- 正确文档根本没进候选集合

你下一步优先改：

- chunk 切分
- embedding
- top-k
- 文档内容质量

## 你复盘时最该看哪 3 个文件

1. [samples.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/samples.go)
   先看 golden cases 是怎么定义的。
2. [runner.go](D:/Agent/OpsCaptionAI/internal/ai/rag/eval/runner.go)
   再看 Recall@K 是怎么计算的。
3. [main.go](D:/Agent/OpsCaptionAI/internal/ai/cmd/rag_eval_cmd/main.go)
   最后看怎么把评测跑起来。
