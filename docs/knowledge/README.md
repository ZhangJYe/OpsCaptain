# Knowledge Corpus

> RAG 知识语料目录。此目录下的文档会被索引到 Milvus 向量数据库，供知识库检索使用。

## 目录结构

- `00_`-`12_` 编号文件：运维知识文档（Kubernetes、Prometheus、Helm、ArgoCD 等）
- `sli_slo.md`：SLI/SLO 相关知识
- `upstream/`：上游官方文档的中文摘要整理版

## 注意

- 工程参考文档（agent 设计、tool 说明等）请参阅 `docs/reference/`
- 新增语料请遵循编号命名规范，确保来源链接清晰
