# 从零看懂一个项目 —— 以 OpsCaption 为例

> **写给初学者：** 这篇教程不假设你懂任何框架，只假设你认识 Go 的基本语法。
> 我们会用 OpsCaption 的**真实代码**当例子，手把手带你理解：拿到一个陌生项目，怎么一步步把它看懂。

---

## 📖 目录

1. [方法论：看懂任何项目的三步法](#第一步方法论看懂任何项目的三步法)
2. [第一步：看懂项目结构（大到小）](#第二步第一步看懂项目结构)
3. [第二步：追踪一条数据流（看懂逻辑）](#第三步第二步追踪一条数据流)
4. [第三步：读懂具体代码（外到内）](#第四步第三步读懂具体代码)
5. [实战演练：逐行解读 4 个核心文件](#第五步实战演练逐行解读-4-个核心文件)
6. [常见困惑解答](#第六步常见困惑解答)
7. [自测清单](#第七步自测清单)

---

## 第一步：方法论——看懂任何项目的三步法

```
项目结构 → 数据流 → 代码细节
   ↓           ↓         ↓
 "这个项目    "数据是    "这一行
  长什么样"   怎么走的"  在干什么"
```

核心原则：**不要从头到尾读代码。** 先建立全局地图，再聚焦到一条具体路径，最后深入细节。

---

## 第二步：第一步——看懂项目结构

### 2.1 先看目录树

打开 OpsCaption 项目，第一眼看的是**顶层目录**：

```
OpsCaptain/
├── main.go                 ← 🚪 入口文件！所有程序从这里开始
├── AGENTS.md               ← 📋 项目的"使用说明书"
├── api/                    ← 📝 定义 API 接口（请求/响应格式）
├── internal/               ← 💻 核心代码（Go 的约定：不对外暴露）
│   ├── ai/                 ←   AI 相关全部逻辑
│   │   ├── agent/          ←    各种 Agent（ReAct、Triage、Supervisor...）
│   │   ├── rag/            ←    RAG 检索（查询改写、召回、重排序）
│   │   ├── contextengine/  ←    上下文装配引擎
│   │   ├── service/        ←    服务层（记忆管理、异步任务...）
│   │   ├── tools/          ←    工具（查日志、查指标、查文档）
│   │   └── protocol/       ←    通信协议定义
│   ├── controller/         ←   HTTP 请求处理
│   └── logic/              ←   业务逻辑
├── utility/                ← 🧰 公共工具（认证、日志、监控...）
├── manifest/config/        ← ⚙️ 配置文件
├── deploy/                 ← 🚀 部署相关（Docker、K8s...）
├── SuperBizAgentFrontend/  ← 🖥️ 前端代码
├── scripts/                ← 🐍 数据预处理脚本
└── Learn/                  ← 📚 学习笔记（你现在在这里）
```

### 2.2 看懂目录的小技巧

每个目录回答一个问题：

| 目录 | 回答的问题 |
|------|-----------|
| `main.go` | 程序从哪开始？ |
| `api/` | 前端怎么跟我通信？传什么格式？ |
| `internal/controller/` | 收到请求后怎么处理？ |
| `internal/ai/agent/` | AI 的"大脑"在哪里？ |
| `internal/ai/rag/` | 知识检索怎么做？ |
| `utility/` | 有什么公共工具可用？ |
| `manifest/config/` | 哪些参数可以调？ |

### 2.3 第二步：看 main.go——程序的"总开关"

读 `main.go` 不需要读懂每一行，只需要理解**执行顺序**：

```go
func main() {
    // 1️⃣ 加载环境变量
    common.LoadPreferredEnvFile()

    // 2️⃣ 初始化基础设施
    ConfigureRedis()      // 连接 Redis
    ConfigureLogging()    // 配置日志
    InitTracing()         // 配置链路追踪

    // 3️⃣ 启动后台服务
    StartMemoryExtractionPipeline()  // 记忆异步抽取
    StartChatTaskPipeline()          // 聊天异步任务

    // 4️⃣ 注册 HTTP 路由
    s.Group("/api", func(group) {
        group.Middleware(Auth)        // 认证
        group.Middleware(RateLimit)   // 限流
        group.Bind(chat.NewV1())      // 绑定聊天接口
    })

    // 5️⃣ 启动服务器
    s.Start()

    // 6️⃣ 等待关闭信号
    waitForShutdown()
}
```

**你现在不需要知道每个函数内部怎么实现的。** 只需要知道："哦，这个程序先初始化，再注册路由，再启动。"

> 💡 **这就是"大到小"的思维**——先看骨架，不看血肉。

---

## 第三步：第二步——追踪一条数据流

看懂了项目结构后，下一步是追问：**用户发一条消息，系统经历了什么？**

### 3.1 画一条用户请求的旅行地图

```
用户输入："checkoutservice CPU 告警了怎么办"
        │
        ▼
┌──────────────────┐
│  HTTP Controller │  收到 POST /api/chat
│  (controller/)   │  解析请求，提取用户消息
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│    Service 层    │  准备上下文（注入记忆、RAG文档）
│   (service/)     │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  ReAct Agent     │  AI 开始思考...
│  (chat_pipeline/)│  "我需要查什么？"
└────────┬─────────┘
         │ 调用工具
         ▼
┌──────────────────┐
│     Tools        │  query_logs()    → 查日志
│   (tools/)       │  query_alerts()  → 查告警
│                  │  query_docs()    → 查知识库
└────────┬─────────┘
         │ 返回结果
         ▼
┌──────────────────┐
│  ReAct Agent     │  根据工具返回结果生成回答
│                  │  "根据日志和告警，建议..."
└────────┬─────────┘
         │
         ▼
      返回给用户
```

### 3.2 用真实代码验证这条路径

我们来看三块关键代码的**简化版**：

**① 入口 — Controller 收到请求（真实代码片段）：**

```go
// 文件: internal/controller/chat/chat_v1_chat.go
func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
    // 1. 构建上下文（记忆 + RAG 文档）
    contextPlan := memoryService.BuildContextPlan(ctx, req.SessionID, req.Query)

    // 2. 执行 Agent
    result := aiOpsService.Execute(ctx, req.Query, contextPlan)

    // 3. 返回结果
    return &v1.ChatRes{Answer: result.Answer}, nil
}
```

**② 核心 — ReAct Agent 的执行图（真实代码）：**

```go
// 文件: internal/ai/agent/chat_pipeline/orchestration.go
func BuildChatAgent(ctx context.Context) {
    g := compose.NewGraph()                           // 创建一张图

    g.AddLambdaNode("InputToChat", ...)               // 节点1: 处理输入
    g.AddChatTemplateNode("ChatTemplate", ...)        // 节点2: 组装 Prompt
    g.AddLambdaNode("ReactAgent", ...)                // 节点3: AI 推理+调用工具

    g.AddEdge(START, "InputToChat")                   // START → 输入处理
    g.AddEdge("InputToChat", "ChatTemplate")          // 输入处理 → Prompt组装
    g.AddEdge("ChatTemplate", "ReactAgent")           // Prompt组装 → AI推理
    g.AddEdge("ReactAgent", END)                      // AI推理 → END

    g.Compile()  // 编译成可执行图
}
```

**③ 路由 — Triage 如何决定走哪个路径（真实代码）：**

```go
// 文件: internal/ai/agent/triage/triage.go
var triageRules = []rule{
    // 如果用户消息包含"告警"关键词 →
    // 路由到 metrics + logs + knowledge 三个模块
    {intent: "alert_analysis", domains: []string{"metrics", "logs", "knowledge"},
     keywords: []string{"告警", "alert", "prometheus"}},

    // 如果用户消息包含"文档"关键词 →
    // 只路由到 knowledge 模块
    {intent: "kb_qa", domains: []string{"knowledge"},
     keywords: []string{"文档", "知识库", "runbook", "sop"}},

    // ...更多规则
}

func (a *Agent) Handle(task) {
    // 遍历规则表，匹配关键词
    for _, rule := range triageRules {
        if matchesRule(query, rule.keywords) {
            return rule  // 找到匹配的规则
        }
    }
}
```

> 💡 **这就是"追踪数据流"**——你不需要读懂每个文件，只需要知道：请求从哪进 → 经过谁 → 从哪出。

---

## 第四步：第三步——读懂具体代码

当你聚焦到**一个具体文件**时，用"黄金三问"：

### 4.1 黄金三问

对任何函数，问自己三个问题：

| 问题 | 怎么看 |
|------|--------|
| **输入是什么？** | 看函数参数 `(ctx, req)` |
| **输出是什么？** | 看返回值 `(res, err)` |
| **做了什么转换？** | 看函数体——输入怎么变成输出的 |

### 4.2 实战：逐行解析 Triage Agent

我们来逐行读 `triage.go`，用黄金三问：

```go
// ① 输入：task *protocol.TaskEnvelope
//    TaskEnvelope 是一个"任务信封"，里面装了用户的问题
func (a *Agent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {

    // ② 从任务信封中取出用户问题，转成小写
    query := strings.TrimSpace(task.Goal)  // "CheckoutService CPU 告警！"
    lower := strings.ToLower(query)        // "checkoutservice cpu 告警！"

    // ③ 默认规则：当没匹配到任何规则时，走告警分析
    selected := rule{
        intent:  "alert_analysis",
        domains: []string{"metrics", "logs", "knowledge"},
        // ...
    }

    // ④ 遍历规则表，看哪个规则的关键词匹配
    for _, candidate := range triageRules {
        if matchesRule(lower, candidate.keywords) {
            selected = candidate  // 匹配到了！
            break
        }
    }

    // ⑤ 额外检查：如果用户提到了严重/P0，提升优先级
    if strings.Contains(lower, "严重") || strings.Contains(lower, "p0") {
        selected.priority = "high"
    }

    // ⑥ 输出：返回分类结果
    return &protocol.TaskResult{
        TaskID:     task.TaskID,       // 任务ID原样返回
        Agent:      a.Name(),          // 告诉上级："我是 triage"
        Status:     "succeeded",       // 状态：成功
        Summary:    selected.summary,  // 分类摘要
        Metadata: map[string]any{      // 附加信息
            "intent":   selected.intent,    // "alert_analysis"
            "domains":  selected.domains,   // ["metrics","logs","knowledge"]
            "priority": selected.priority,  // "high"
        },
    }, nil
}
```

### 4.3 用黄金三问总结这个函数

| | 答案 |
|---|---|
| **输入** | 一个任务信封，里面有用户的问题 |
| **输出** | 分类结果：这是什么意图、应该查哪些模块、优先级多高 |
| **转换** | 关键词匹配 → 选规则 → 检查严重程度 → 返回分类 |

---

## 第五步：实战演练——逐行解读 4 个核心文件

现在你有了方法论，我们用它来读 OpsCaption 的 4 个核心文件。

### 5.1 main.go —— 程序入口（230 行）

**用"大到小"思维**——不需要读懂每行，抓住结构：

```
main() 做了什么（按顺序）：
  1. 加载配置
  2. 初始化 Redis、日志、追踪
  3. 校验 Token、Memory、ChatTask 配置
  4. 启动后台 Pipeline（记忆抽取、异步聊天）
  5. 注册 HTTP 路由（/api/chat, /healthz, /readyz, /metrics）
  6. 启动 HTTP 服务器
  7. 等待关闭信号 → 优雅退出
```

**关键代码位置：**
- 第 30 行：`LoadPreferredEnvFile()` — 加载 .env 文件
- 第 49 行：`ValidateStartupSecrets()` — 检查密钥是否配置
- 第 78 行：`StartMemoryExtractionPipeline()` — 记忆异步抽取启动
- 第 84 行：`StartChatTaskPipeline()` — 聊天异步任务启动
- 第 107 行：`s.Group("/api", ...)` — 注册 API 路由
- 第 115 行：`s.Start()` — 启动服务器

### 5.2 orchestration.go —— ReAct Agent 的核心（41 行）

这是项目的**心脏**。让我们一行行读懂：

```go
func BuildChatAgentWithQuery(ctx context.Context, query string) (
    r compose.Runnable[*UserMessage, *schema.Message],  // 返回一个"可运行的东西"
    err error,                                           // 或者返回一个错误
) {
    // 1. 给三个节点起名字（方便调试）
    const (
        ChatTemplate = "ChatTemplate"   // 节点1: 拼装 Prompt
        ReactAgent   = "ReactAgent"     // 节点2: AI 推理引擎
        InputToChat  = "InputToChat"    // 节点3: 输入处理
    )

    // 2. 创建一张"有向图"
    g := compose.NewGraph[*UserMessage, *schema.Message]()

    // 3. 往图里加三个节点
    //    先创建 ChatTemplate（Prompt 模板）
    chatTemplateKey, _ := newChatTemplate(ctx)
    g.AddChatTemplateNode("ChatTemplate", chatTemplateKey)

    //    再创建 ReactAgent（AI 推理）
    reactAgentLambda, _ := newReactAgentLambdaWithQuery(ctx, query)
    g.AddLambdaNode("ReactAgent", reactAgentLambda)

    //    最后创建 InputToChat（输入转换）
    g.AddLambdaNode("InputToChat", compose.InvokableLambdaWithOption(
        newInputToChatLambda,
    ))

    // 4. 连线！定义数据流向
    g.AddEdge(compose.START, "InputToChat")   // START → 输入处理
    g.AddEdge("InputToChat", "ChatTemplate")   // 输入处理 → Prompt拼装
    g.AddEdge("ChatTemplate", "ReactAgent")    // Prompt拼装 → AI推理
    g.AddEdge("ReactAgent", compose.END)       // AI推理 → END

    // 5. 编译成可执行图
    r, err = g.Compile(ctx)
    return r, err
}
```

**用黄金三问：**

| | 答案 |
|---|---|
| **输入** | `ctx`（上下文）+ `query`（用户问题） |
| **输出** | 一个"可执行的图"（把三个节点串起来的流水线） |
| **转换** | 创建图 → 加节点 → 连线 → 编译 |

**为什么这样设计？** 因为 Eino 框架把一个 AI 任务的执行流程建模成一张**有向图**：数据从 START 流入，经过几个处理节点，最终从 END 流出。`orchestration.go` 就是这张图的"施工图"。

### 5.3 triage.go —— 意图分类（108 行）

这是 Multi-Agent 系统的"调度员"。上面已经逐行解读过，这里复习核心思路：

```
Triage Agent 的工作：
  用户输入 → 关键词匹配 → 选规则 → 返回 (意图 + 领域 + 优先级)

设计亮点：
  1. 规则表驱动（不用 if-else 大嵌套）
  2. 有默认规则（兜底）
  3. 优先级可动态调整
```

### 5.4 chat_pipeline/prompt.go —— Prompt 工程

Prompt 分三层设计，这是工程化的关键：

```
┌─────────────────────────────────────────┐
│ 第1层：静态规则（System Prompt）          │
│   "你是一个运维助手，只回答运维相关问题"    │
│   可缓存，不随请求变化                     │
├─────────────────────────────────────────┤
│ 第2层：动态配置（也进 System Prompt）      │
│   "当前日志 topic: xxx, 地域: xxx"       │
│   从 config.yaml 读取                    │
├─────────────────────────────────────────┤
│ 第3层：运行时上下文（User Message）        │
│   "当前日期: 2026-04-25"                 │
│   "RAG 检索到的相关文档: ..."             │
│   不进 System Prompt，防注入攻击           │
└─────────────────────────────────────────┘
```

> 💡 **面试可以这样讲：** "我把 Prompt 分三层，静态规则可缓存，动态配置不硬编码，运行时上下文不进 System Prompt 防注入。这是从 Demo 到工程化的关键一步。"

---

## 第六步：常见困惑解答

### Q1: "我看到一个函数，但不知道谁调用了它，怎么办？"

三种方法：
1. **IDE 右键 → "Find References"**（最常用）
2. **用 grep 搜函数名**：`grep -r "Handle(" internal/`
3. **从入口倒推**：从 `main.go` → Controller → Service → Agent，一层层追

### Q2: "代码里好多 import，我都要看懂吗？"

不需要！import 只是告诉你"这段代码依赖了哪些外部包"。你只需要关注：
- **项目内部的 import**（`SuperBizAgent/...`）→ 这是你自己的代码
- **第三方框架的 import**（`github.com/...`）→ 知道它是干什么的就行，不用深究

### Q3: "有个包/结构体/函数名看不懂，怎么办？"

- **看名字**：Go 的命名通常就是功能的描述。`ChatTemplate` = 聊天模板，`MemoryService` = 记忆服务。
- **看注释**：虽然这个项目代码注释不多，但 AGENTS.md 和 Learn/ 下有很多文档。
- **问 AI**：把代码片段粘贴给 AI，让它用大白话解释。

### Q4: "什么时候需要深入读一个文件，什么时候可以跳过？"

| 场景 | 策略 |
|------|------|
| 你想理解整体架构 | 只读 `main.go` + 各模块的 `orchestration.go` |
| 你在排查一个 bug | 从 Controller 一路追到报错的那行 |
| 你想学某个技术点（如 RAG） | 聚焦 `internal/ai/rag/` 目录 |
| 你准备面试 | 重点读 `AGENTS.md` + `Learn/` 下的文档 |

### Q5: "这个项目太大了，我从哪开始？"

按这个顺序，每天 1 个文件：

| 天数 | 读什么 | 重点 |
|------|--------|------|
| 第1天 | `main.go` + `AGENTS.md` | 建立全局地图 |
| 第2天 | `internal/ai/agent/chat_pipeline/orchestration.go` | 理解 ReAct Agent |
| 第3天 | `internal/ai/agent/triage/triage.go` | 理解多 Agent 路由 |
| 第4天 | `internal/ai/rag/` 目录 | 理解知识检索 |
| 第5天 | `internal/ai/contextengine/` | 理解上下文管理 |
| 第6天 | `internal/ai/service/memory_service.go` | 理解记忆系统 |
| 第7天 | 回顾 + 自己画架构图 | 检验理解 |

---

## 第七步：自测清单

读完这篇教程后，试着回答以下问题。能答出来就说明你真的看懂了：

- [ ] 这个项目是干什么的？（一句话概括）
- [ ] 项目目录分几大块？每块干什么？
- [ ] 用户发一条消息，数据经过哪些模块？
- [ ] ReAct Agent 用的是什么框架？为什么用图来编排？
- [ ] Triage Agent 怎么决定把任务分给哪个 Specialist？
- [ ] Prompt 为什么要分三层？
- [ ] RAG 检索有几个步骤？
- [ ] 什么是 Context Engine？为什么需要它？
- [ ] 记忆系统有几层？记忆是怎么写入的？
- [ ] 如果让你给这个项目加一个新功能，你知道改哪几个文件吗？

---

> 📌 **记住核心心法：不要试图一次看懂所有代码。先建地图，再走一条路，最后深入细节。**
>
> 你现在手里有三份资料可以交叉阅读：
> 1. 这篇教程（从零开始的方法论）
> 2. `OpsCaption架构分析.md`（项目完整的技术文档）
> 3. `从零到面试通关学习指南.md`（面试视角的速记）
>
> 推荐顺序：先看这篇 → 再看架构分析 → 最后看面试指南。
