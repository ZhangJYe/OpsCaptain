# OpsCaptain 业务流程图

> 生成日期: 2026-04-08

## 总业务流程

```mermaid
flowchart TB
    User(("用户"))

    User -->|"对话提问"| Chat
    User -->|"运维分析"| AIOps
    User -->|"上传文档"| Upload

    subgraph Chat["Chat 对话"]
        C1["安全检查 & 降级检查"]
        C2["会话锁 & 缓存查询"]
        C3["Context Engine 组装上下文<br/>历史 + 记忆 + <b>RAG 文档检索</b>"]
        C4["ReAct Agent<br/>LLM ↔ Tools 循环 (最多25轮)<br/>可调用 query_internal_docs"]
        C5["输出过滤 & 记忆持久化"]
        C1 --> C2
        C2 -->|"缓存命中"| C_hit["直接返回"]
        C2 -->|"未命中"| C3 --> C4 --> C5
    end

    subgraph AIOps["AIOps 运维分析"]
        A1["安全检查 & 降级检查"]
        A2{"Approval Gate<br/>含高危操作?"}
        A3["Context Engine 组装记忆上下文<br/>(不含 RAG 文档)"]
        A4["Planner — LLM 制定分步计划"]
        A5["Executor — LLM + Tools 逐步执行<br/>可调用 query_internal_docs"]
        A6{"Replanner<br/>数据足够?"}
        A7["输出分析报告 & 记忆持久化"]
        A1 --> A2
        A2 -->|"高危"| A_wait["入队等待人工审批"]
        A2 -->|"通过"| A3 --> A4 --> A5 --> A6
        A6 -->|"不够"| A4
        A6 -->|"完成"| A7
    end

    subgraph Upload["知识索引"]
        U1["清理旧数据"]
        U2["FileLoader → Splitter → Milvus 写入"]
        U1 --> U2
    end

    Milvus[("Milvus<br/>向量数据库")]
    LLM["GLM-4.5-AIR"]

    C3 -->|"Stage3: RAG 检索"| Milvus
    C4 -->|"工具: query_internal_docs"| Milvus
    A5 -->|"工具: query_internal_docs"| Milvus
    U2 -->|"写入向量"| Milvus

    C4 --> LLM
    A4 --> LLM
    A5 --> LLM
    A6 --> LLM

    C5 --> R1(("返回响应"))
    C_hit --> R1
    A7 --> R2(("返回报告"))
    U2 --> R3(("返回索引结果"))

    A_wait -->|"审批通过"| A3
    A_wait -->|"审批拒绝"| A_reject["返回拒绝原因"]

    classDef process fill:#e8f5e9,stroke:#43a047,stroke-width:1px
    classDef decision fill:#fff8e1,stroke:#ff8f00,stroke-width:2px
    classDef infra fill:#e8eaf6,stroke:#5c6bc0,stroke-width:1px
    classDef warn fill:#ffebee,stroke:#e53935,stroke-width:1px

    class C1,C2,C3,C4,C5,A1,A3,A4,A5,A7,U1,U2 process
    class A2,A6 decision
    class LLM,Milvus infra
    class A_wait,A_reject warn
```

## RAG 使用说明

| 业务路径 | RAG 使用方式 | 触发时机 |
|---------|-------------|---------|
| **Chat** | ① Context Engine Stage 3 自动检索文档注入上下文 | 每次请求必触发 |
| **Chat** | ② LLM 主动调用 `query_internal_docs` 工具 | LLM 自主判断是否需要 |
| **AIOps** | ① Context Engine **不检索文档** (AllowDocs=false) | — |
| **AIOps** | ② Executor 阶段 LLM 可调用 `query_internal_docs` 工具 | LLM 自主判断是否需要 |
| **知识索引** | 文档向量化写入 Milvus | 用户上传时触发 |
