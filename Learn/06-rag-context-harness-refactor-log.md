# OpsCaptain RAG / Context / Harness 改造复盘日志

## 第 1 阶段：先做 RAG 模块化

这一步的目标不是“让效果立刻变强”，而是先把工程结构理顺。

改造前的问题：

- `query_internal_docs` 自己维护一套 retriever 缓存
- `contextengine` 自己维护另一套 retriever 缓存
- `chat_pipeline` 每次自己初始化 retriever
- 文件上传入库在 controller 里直接操作 loader、Milvus client、index graph

这会导致 4 个直接后果：

1. 同样的 Milvus 连接失败，会在不同入口表现成不同错误。
2. 同样的 top-k / timeout / failure TTL，分散在不同包里，不容易统一治理。
3. 想做 trace 和评测时，没有统一的 RAG 运行时可观测面。
4. controller 直接知道太多 RAG 细节，不利于后续模块化和面试时解释分层。

这一步我做了两件关键事：

1. 新建统一的 RAG 模块：
   - [config.go](D:/Agent/OnCallAI/internal/ai/rag/config.go)
   - [retriever_pool.go](D:/Agent/OnCallAI/internal/ai/rag/retriever_pool.go)
   - [indexing_service.go](D:/Agent/OnCallAI/internal/ai/rag/indexing_service.go)
2. 让工具、上下文装配、聊天检索、文件入库都开始走这个模块。

你复盘时重点看这几个问题：

### 1. 为什么先抽 `RetrieverPool`

因为现在真正重复的不是“向量检索能力”，而是“检索器生命周期管理”。

生命周期管理包括：

- 何时初始化
- 失败后是否短 TTL 缓存
- 相同配置是否复用
- cache key 怎么定义

这类逻辑如果散在三个包里，后面很难做稳定性治理。

### 2. 为什么把入库也收进 `rag`

因为 RAG 不只是 retrieve，还包括 ingest。

如果只抽 retrieval，不抽 indexing，最后还是会出现：

- 检索路径是模块化的
- 入库路径还在 controller 里散着

那不算完整的模块化。

### 3. 面试时怎么讲这一步

你可以用下面这句话：

> 我先做的是工程层面的 RAG 模块化，把检索器缓存、失败 TTL、索引入口从业务层收口到统一的 `internal/ai/rag` 模块。这样后续做 trace、评测、召回率分析时，所有入口都能落到同一套运行时抽象上。

### 4. 这一步还没有解决什么

这一步还没有解决：

- 召回率是否足够高
- chunk 是否切得合理
- context 里如何展示更细的检索 trace
- harness 怎么自动做 recall 评测

这些会放在后面两个阶段继续做。
