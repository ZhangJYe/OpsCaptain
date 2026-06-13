# OpsCaption Go 初学者导读

> 欢迎！本文档面向有其他语言经验、刚开始学习 Go 的同学。我们将通过 OpsCaption 这个真实的企业级项目，边读代码边学 Go。

---

## 1. Go 语言基础速览（5 分钟）

### 1.1 Go 的核心哲学

Go 是一门**简洁、并发、编译型**语言。设计者希望你：

- **少写代码**：没有继承、没有泛型滥用（Go 2.0 才加）、没有异常机制
- **天生并发**：goroutine 是轻量级线程，channel 是通信管道
- **编译即部署**：编译成单一二进制文件，没有运行时依赖

### 1.2 package 和 import

Go 用 `package` 声明当前文件属于哪个包，用 `import` 引入其他包：

```go
package main  // main 包是程序入口

import (
    "fmt"                         // 标准库
    "github.com/gogf/gf/v2/frame/g"  // 第三方库
    "SuperBizAgent/internal/app"       // 本项目内部包
)
```

**命名规则**：
- 导入后用 `.` 后面的部分作为包名（如 `g.Log()` 就是调用 `frame` 包的 `Log` 函数）
- 可以用别名：`aiservice "SuperBizAgent/internal/ai/service"`

### 1.3 error 处理哲学

Go **没有 try-catch**，用返回值处理错误：

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("操作失败: %w", err)  // %w 包装错误
}
```

这个 `if err != nil` 模式在 Go 代码中随处可见，是最重要的习惯。

### 1.4 goroutine 和 channel

```go
// 启动一个 goroutine（轻量级线程）
go func() {
    fmt.Println("后台运行")
}()

// channel 是 goroutine 之间的通信管道
ch := make(chan string)
go func() {
    ch <- "数据"  // 发送
}()
msg := <-ch  // 接收，阻塞直到有数据
```

### 1.5 interface 的鸭子类型

Go 的接口是**隐式实现**的——不需要 `implements` 关键字：

```go
// 定义接口
type Writer interface {
    Write(data []byte) error
}

// 任何实现了 Write 方法的类型都自动满足这个接口
type File struct { name string }
func (f *File) Write(data []byte) error { /* ... */ }

// File 自动就是 Writer，不需要声明
var w Writer = &File{name: "test.txt"}
```

---

## 2. 项目结构导读

### 2.1 五层架构

OpsCaption 采用经典的**分层架构**，从上到下：

```
API Layer            Controller 层     参数解析、鉴权、响应映射
Application Layer    App 层            业务编排（ChatApp、AIOpsApp）
Domain Layer         AI/Domain 层      核心业务规则（Agent、RAG、Tool）
Infrastructure Layer Infra 层          外部系统适配（Milvus、RabbitMQ、Redis）
Common Layer         Utility 层        横切关注点（认证、限流、日志、健康检查）
```

### 2.2 用餐厅类比理解每一层

| 层 | 角色 | 做什么 | 不做什么 |
|----|------|--------|----------|
| **Controller** | 前台点餐 | 接收请求、验证参数、返回结果 | 不知道菜怎么做 |
| **App** | 厨师长 | 协调配菜、决定流程 | 不处理具体的向量检索 |
| **Domain/AI** | 核心配方 | Agent 推理、RAG 检索、工具调用 | 不关心用的是 Milvus 还是 Redis |
| **Infra** | 供应链 | 连接 Milvus、RabbitMQ、文件系统 | 不知道上层业务逻辑 |
| **Utility** | 餐具消毒 | 日志、认证、限流、健康检查 | 不参与业务决策 |

### 2.3 为什么要分层

**依赖方向**（箭头只允许向下）：

```
Controller → App → Domain → Infra
                    ↓
                 Utility
```

好处：
- **可测试**：Domain 层可以 mock Infra 层来测试
- **可替换**：换掉 Milvus 只需改 Infra 层，上层无感
- **职责清晰**：每个文件不超过 500 行，每个函数不超过 50 行

---

## 3. main.go 逐行解读

以下是 `main.go` 前 100 行的关键代码，逐行解释：

```go
// 第 47-50 行：加载环境变量文件
func main() {
    if err := common.LoadPreferredEnvFile(); err != nil {
        panic(err)  // 加载失败直接崩溃，因为没有配置什么都干不了
    }
```

**解读**：程序入口 `main()` 函数。先加载 `.env` 文件（数据库密码、API Key 等）。`panic(err)` 是 Go 的"快速失败"哲学——启动阶段出错直接崩溃，不要带病运行。

```go
    // 第 51 行：创建上下文
    ctx := gctx.New()
```

**解读**：`context.Context` 是 Go 的"通行证"，携带请求的元数据（trace ID、取消信号等）。几乎所有函数第一个参数都是 `ctx`。

```go
    // 第 53-55 行：初始化 Redis
    if err := common.ConfigureRedis(ctx); err != nil {
        panic(err)
    }
```

**解读**：配置 Redis 连接。注意 `if err := ...` 是 Go 的习惯写法——在 `if` 语句中声明并赋值。

```go
    // 第 57-59 行：初始化日志
    if err := logging.Configure(ctx); err != nil {
        panic(err)
    }
```

**解读**：配置日志系统。同样，启动阶段失败直接 panic。

```go
    // 第 61-64 行：初始化链路追踪
    traceShutdown, err := traceutil.Init(ctx)
    if err != nil {
        panic(err)
    }
```

**解读**：初始化分布式追踪（Jaeger）。`traceShutdown` 是一个**函数变量**，用于优雅关闭时释放资源。这是 Go 中常见的"返回清理函数"模式。

```go
    // 第 65 行：设置 Token 审计钩子
    models.SetTokenAuditHooks(aiservice.EnforceTokenLimitFromContext, aiservice.RecordTokenUsageFromContext)
```

**解读**：把两个函数作为参数传进去——这就是**依赖注入**。`models` 包不需要知道这两个函数的具体实现，只需要知道它们的签名。

```go
    // 第 89-96 行：认证配置检查
    authEnabled, _ := g.Cfg().Get(ctx, "auth.enabled")
    if authEnabled.Bool() {
        if err := auth.ValidateConfig(); err != nil {
            panic(err)
        }
    } else {
        g.Log().Warningf(ctx, "⚠️ AUTH DISABLED...")
    }
```

**解读**：从配置文件读取 `auth.enabled`。`g.Cfg().Get()` 是 GoFrame 框架的配置读取方式。如果认证关闭，打一条警告日志。

```go
    // 第 121-123 行：函数变量注入（依赖反转）
    health.CloseMySQLFunc = tools.CloseMySQL
    health.CloseAllMilvusClientsFunc = inframv.CloseAllClients
    health.MilvusReadyCheckFunc = inframv.PingMilvus
```

**解读**：这是 OpsCaption 的核心设计模式——**函数变量注入**。`health` 包定义了函数变量，`main.go` 在启动时把具体实现"注入"进去。这样 `health` 包不需要直接 import `infra` 包，避免了循环依赖。

```go
    // 第 171-173 行：创建 HTTP 服务器
    s := g.Server()
    s.SetGraceful(true)
    s.SetGracefulShutdownTimeout(30)
```

**解读**：创建 GoFrame HTTP 服务器，启用优雅关闭（收到 SIGTERM 后等 30 秒处理完正在处理的请求再退出）。

```go
    // 第 184-186 行：创建 App 实例（依赖注入）
    chatApp := app.NewChatApp()
    knowledgeApp := app.NewKnowledgeApp(infrafs.NewLocalUploadStore(common.FileDir))
    aiopsApp := app.NewAIOpsApp()
```

**解读**：创建三个应用层实例。注意 `knowledgeApp` 接收了一个 `FileStore` 参数——这就是依赖注入，App 层不直接创建存储对象。

```go
    // 第 251-257 行：注册路由和中间件
    s.Group("/api", func(group *ghttp.RouterGroup) {
        group.Middleware(middleware.CORSMiddleware)
        group.Middleware(middleware.AuthMiddleware)
        group.Middleware(middleware.RateLimitMiddleware)
        group.Middleware(middleware.ResponseMiddleware)
        group.Bind(chat.NewV1(chatApp, knowledgeApp, aiopsApp, ...))
    })
```

**解读**：把所有 `/api` 开头的请求交给 `chat.ControllerV1` 处理。中间件按顺序执行：CORS → 认证 → 限流 → 响应包装。

```go
    // 第 437-440 行：优雅关闭
    sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    <-sigCtx.Done()  // 阻塞，直到收到 SIGTERM 或 SIGINT
```

**解读**：`signal.NotifyContext` 监听系统信号。`<-sigCtx.Done()` 会阻塞当前 goroutine，直到进程收到终止信号。收到信号后，`waitForShutdown` 函数会依次关闭所有依赖。

---

## 4. 一个请求的完整旅程

让我们追踪一个 `/api/chat` 请求在代码中的完整路径：

### 4.1 请求到达

```
客户端 POST /api/chat
    ↓
middleware.CORSMiddleware          → 设置跨域头
    ↓
middleware.AuthMiddleware          → 验证认证
    ↓
middleware.RateLimitMiddleware     → 检查限流
    ↓
middleware.ResponseMiddleware      → 包装响应格式
    ↓
chat.ControllerV1.Chat()          → 参数解析
```

**文件路径**：`internal/controller/chat/chat_v1_chat.go:13`

```go
func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
    result, err := c.chatApp.HandleChat(ctx, &app.ChatInput{
        SessionID: req.Id,
        Question:  req.Question,
        SkillIDs:  req.SelectedSkillIds,
    })
```

**Controller 做了什么**：
- 接收 GoFrame 自动解析的请求参数 `req`
- 构造 `ChatInput` 传递给 App 层
- 处理错误映射（`PromptRejectedError` → 400 状态码）
- 将 App 层结果映射为响应结构体

### 4.2 App 层编排

```
chatApp.HandleChat()
    ↓
MemoryService.BuildChatPackage()   → 构建对话上下文
    ↓
ContextEngine 组装 history/memory/docs
    ↓
chat_pipeline.BuildChatAgentWithQuery()  → 构建 ReAct Agent
    ↓
Agent.Execute()                    → 执行推理
    ↓
Tools / RAG                        → 工具调用或知识检索
    ↓
返回 ChatResult
```

**文件路径**：`internal/app/chat_app.go:45`

```go
func NewChatApp() *ChatApp {
    a := &ChatApp{
        sessionLocks:   make(map[string]*sessionLockEntry),
        buildChatAgent: chat_pipeline.BuildChatAgentWithQuery,  // 函数变量注入
        degradationCheck: aiservice.GetDegradationDecision,
    }
    return a
}
```

### 4.3 Domain 层执行

Agent 会根据用户问题自主决定：
- 需要查日志？调用 `tools.LogQueryTool`
- 需要查指标？调用 `tools.MetricQueryTool`
- 需要查知识库？走 `rag.Retriever`
- 直接回答？生成 `schema.Message`

### 4.4 响应返回

```
ChatResult
    ↓
ControllerV1 映射为 ChatRes
    ↓
ResponseMiddleware 包装为统一 JSON 格式
    ↓
返回客户端
```

### 4.5 流式请求（SSE）

对于 `/api/chat_stream`，流程类似，但 App 层返回的是一个 `ChatStreamSink` 接口，Controller 层通过 SSE 实时推送：

```go
// internal/controller/chat/chat_v1_chat_stream.go:20
func (s *sseStreamSink) SendText(text string) {
    for _, chunk := range splitStreamChunks(text, 160) {
        s.client.SendToClient("message", chunk)  // 每 160 字符切一片推送
    }
}
```

---

## 5. 关键设计模式（用代码举例）

### 5.1 函数变量注入（Function Variable Injection）

这是 OpsCaption 最重要的设计模式，用于**避免循环依赖**。

**问题**：`health` 包需要调用 `infra/milvus` 的关闭函数，但 `health` 在 Utility 层，`infra` 在基础设施层，Utility 不能 import infra。

**解决方案**：用函数变量作为"插槽"，在 `main.go` 中注入：

```go
// utility/health/health.go — 定义函数变量
var (
    CloseAllMilvusClientsFunc func() error
    MilvusReadyCheckFunc      func(ctx context.Context) error
    CloseMySQLFunc            func() error
)

// 使用时检查是否已注入
func injectedMilvusReadyCheck(parent context.Context) error {
    if MilvusReadyCheckFunc == nil {
        return errCheckSkipped  // 未注入就跳过
    }
    return MilvusReadyCheckFunc(parent)
}
```

```go
// main.go — 启动时注入具体实现
health.CloseMySQLFunc = tools.CloseMySQL
health.CloseAllMilvusClientsFunc = inframv.CloseAllClients
health.MilvusReadyCheckFunc = inframv.PingMilvus
```

**类比**：就像插座和插头——`health` 包定义了插座（函数变量），`main.go` 在启动时插入具体的插头（函数实现）。

### 5.2 Interface vs Struct

**什么时候用 interface**：
- 需要支持多种实现（如 `ChangeEventStore` 可以是 Redis 实现或内存实现）
- 需要在测试中 mock
- 需要解耦上下游

**什么时候用 struct**：
- 只有一种实现
- 不需要 mock
- 追求性能（interface 有间接调用开销）

```go
// internal/infra/milvus/ — 这里是 struct，因为 Milvus 只有一个实现
type MilvusClient struct {
    client *v2.Client
}

// internal/ai/changeevent/ — 这里用 interface，因为有多种 store 实现
type ChangeEventStore interface {
    Add(ctx context.Context, event ChangeEvent) error
    Recent(ctx context.Context, limit int) ([]ChangeEvent, error)
    Cleanup(ctx context.Context, before time.Time) (int, error)
}
```

### 5.3 Context 传递

`ctx` 是 Go 的"万能通行证"，携带：
- **请求级元数据**：trace ID、user ID
- **取消信号**：客户端断开连接时通知下游
- **超时控制**：`context.WithTimeout(ctx, 5*time.Second)`

```go
// 到处都有 ctx 参数——这不是 Go 的坏习惯，而是刻意设计
func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (*v1.ChatRes, error) {
func (a *ChatApp) HandleChat(ctx context.Context, input *ChatInput) (*ChatResult, error) {
func MilvusReadyCheckFunc(ctx context.Context) error {
```

**为什么要传递 ctx**：
- 客户端断开连接 → 上游取消 ctx → 下游停止无用的数据库查询
- 请求超时 → 自动传播取消信号
- 分布式追踪 → trace ID 自动传递

### 5.4 Error Wrapping

```go
// 不好的写法：丢失了原始错误信息
return nil, errors.New("failed to connect to Milvus")

// 好的写法：用 %w 包装，保留原始错误
return nil, fmt.Errorf("milvus connect: %w", err)

// 调用方可以检查具体错误类型
if errors.Is(err, context.DeadlineExceeded) {
    // 超时处理
}
```

---

## 6. 常见 Go 初学者陷阱

### 6.1 Slice append 陷阱

```go
// 陷阱：slice 可能共享底层数组
a := []int{1, 2, 3}
b := a[:2]       // b = [1, 2]，但 b 和 a 共享底层数组
b = append(b, 99) // b = [1, 2, 99]，此时 a 也变成了 [1, 2, 99]！
```

**解决**：用 `copy` 或 `slices.Clone` 创建独立副本。

### 6.2 Goroutine 泄漏

```go
// 陷阱：goroutine 永远不会退出
func leaky() {
    ch := make(chan int)
    go func() {
        val := <-ch  // 如果没人发送，这个 goroutine 永远阻塞
        fmt.Println(val)
    }()
    // 函数返回后，goroutine 泄漏
}
```

**解决**：用 `context.WithCancel` 或 `done channel` 确保 goroutine 能退出。

### 6.3 nil Interface

```go
// 陷阱：nil 值赋给 interface 后，interface 本身不是 nil
var p *MyStruct = nil
var i interface{} = p
fmt.Println(i == nil)  // false！因为 interface 有类型信息
```

**解决**：检查错误时用 `errors.As` 和 `errors.Is`，不要直接 `== nil`。

### 6.4 defer 执行顺序

```go
// defer 是 LIFO（后进先出）
func order() {
    defer fmt.Println("1")
    defer fmt.Println("2")
    defer fmt.Println("3")
}
// 输出：3, 2, 1
```

**记忆**：defer 栈，后压先出。就像俄罗斯方块——最后放的最先掉下来。

---

## 7. 如何调试和测试

### 7.1 go test 使用

```bash
# 运行当前目录所有测试
go test ./...

# 运行特定包的测试
go test ./internal/app/...

# 运行特定测试函数
go test ./internal/app/ -run TestChatApp

# 显示详细输出
go test -v ./internal/app/...

# 运行测试并查看覆盖率
go test -cover ./internal/app/...
```

**OpsCaption 的测试结构**：
- 单元测试文件和源文件在同一目录，如 `chat_app_test.go`
- 用 `SetBuildChatAgent` 等方法替换依赖（函数变量注入的优势）

### 7.2 go vet 静态分析

```bash
# 检查代码中的常见错误
go vet ./...

# 例如：未使用的变量、错误的格式化参数、不可达的代码
```

### 7.3 pprof 性能分析

OpsCaption 已经内置了 pprof 服务器（`main.go:236`）：

```bash
# 查看 CPU profile
go tool pprof http://localhost:9090/debug/pprof/profile?seconds=30

# 查看内存分配
go tool pprof http://localhost:9090/debug/pprof/allocs

# 查看 goroutine 泄漏
go tool pprof http://localhost:9090/debug/pprof/goroutine

# 查看 heap
go tool pprof http://localhost:9090/debug/pprof/heap
```

### 7.4 实用调试技巧

```bash
# 查看 Go 版本
go version

# 格式化代码（别手动对齐缩进）
gofmt -w .

# 查看 import 是否正确
go mod tidy

# 编译检查（不生成二进制）
go build -o /dev/null .
```

---

## 附录：推荐阅读顺序

如果你是 Go 初学者，建议按以下顺序阅读 OpsCaption 代码：

1. `main.go` — 理解程序启动流程
2. `internal/controller/chat/chat_new.go` — 理解 Controller 层结构
3. `internal/controller/chat/chat_v1_chat.go` — 理解请求处理
4. `internal/app/chat_app.go` — 理解 App 层编排
5. `internal/app/types.go` — 理解数据结构
6. `utility/health/health.go` — 理解函数变量注入模式
7. `internal/ai/agent/chat_pipeline/` — 理解 Agent 构建

---

> 祝你学习愉快！Go 的哲学是"少即是多"，一开始可能觉得 `if err != nil` 很啰嗦，但当你习惯了这种显式错误处理，你会发现代码的可预测性和可维护性大幅提升。
