# 工具: query_internal_docs

## 功能说明
通过 RAG 链路检索内部知识库文档，包括 SOP、runbook、错误码解释、历史处理经验等。底层使用 Milvus 向量数据库进行语义检索。供 knowledge specialist 使用。

## 输入参数
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| query | string | 是 | 检索查询文本 |

输入示例：
```json
{"query": "支付服务超时如何处理"}
```

## 输出格式
JSON 数组字符串，每个元素为一条命中文档：

```json
[
  {
    "id": "doc-001",
    "title": "支付超时排查 SOP",
    "content": "当支付服务出现超时时，首先检查...",
    "source": "runbook/payment",
    "_score": 0.85
  }
]
```

字段说明：
- `id` / `title`：文档标识
- `content`：文档内容（agent 会截取前 160 字符作为 snippet）
- `source`：文档来源
- `_score`：检索相似度分数

## 错误处理
- Milvus 不可达：返回 degraded（confidence 0.25），附带错误信息
- 超时：默认 5s，配置项 `aiops.tools.knowledge_query_timeout_ms`
- 结果解析失败：返回 degraded，附带原始输出（confidence 0.3）
- 无命中：返回空数组，agent 输出"知识库未检索到可直接复用的文档"

## 配置项
| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `aiops.tools.knowledge_query_timeout_ms` | 5000 | 查询超时（毫秒） |
| `aiops.tools.knowledge_evidence_limit` | 跟随 `rag.top_k` | 最大返回文档数 |

Milvus 地址和 Embedding 模型配置通过 `manifest/config/config.yaml` 注入。

## 相关代码
- 工具实现: `internal/ai/tools/query_internal_docs.go`
- 测试: `internal/ai/tools/query_internal_docs_test.go`
- 调用方: `internal/ai/agent/specialists/knowledge/agent.go`
- RAG 链路: `internal/ai/rag/`
