# Review 修复复盘：Approval Gate 与 Chat Memory 抽取

## 1. 背景

本轮 review 暴露了两个 P1 级问题：

1. `Approval Gate` 拒绝请求时，AI Ops 控制器仍把结果当成“内部错误”
2. `AI Ops` 路径虽然已经给 memory extraction 增加了超时保护，但 `/chat` 和 `/chat_stream` 仍然直接启动无界后台抽取

这两个问题的共同点是：

- 核心思路已经实现了一半
- 但没有在所有调用路径上完成一致性收口

这类问题很典型，说明当前项目已经进入“局部修复后需要系统收口”的阶段。

---

## 2. 问题 1：Approval Gate 被控制器误判为内部错误

## 2.1 原始问题

`RunAIOpsMultiAgent` 在审批门拒绝时会返回：

- 空 `result`
- `detail` 中带拒绝原因
- `error == nil`

但控制器中存在这段逻辑：

- 如果 `resp == ""`，直接返回 `内部错误`

结果：

- 用户看不到真实拒绝原因
- 系统把业务拒绝误报成内部异常

---

## 2.2 修复思路

不把审批门拒绝当成错误，而是把它当成一种可解释的业务结果返回。

本轮实现：

- `RunAIOpsMultiAgent` 在审批门拒绝时直接返回拒绝原因作为 `result`
- `detail` 保留同样的拒绝原因
- `trace_id` 为空，因为任务根本没有进入 runtime

这样可以保证：

- 控制器无需额外分支就能返回正确结果
- 前端或调用方可以直接展示拒绝原因
- 审批拒绝不再污染错误统计

---

## 2.3 为什么这样修

可选方案有两个：

1. 返回业务错误，再让控制器做特殊处理
2. 直接把审批拒绝包装成正常业务结果

本轮选择第 2 种，原因是：

- 改动最小
- 不需要新增一套错误码分支
- 更符合当前 AI Ops 的报告型接口语义

后续如果要做更严格的 API 语义治理，可以再引入显式状态码或业务码。

---

## 2.4 代码落点

- [ai_ops_service.go](/Users/agiuser/Agent/OpsCaptionAI/internal/ai/service/ai_ops_service.go)
- [ai_ops_service_test.go](/Users/agiuser/Agent/OpsCaptionAI/internal/ai/service/ai_ops_service_test.go)

---

## 3. 问题 2：Chat 主链路仍然无界后台抽取记忆

## 3.1 原始问题

虽然 `AI Ops` 已经通过 `MemoryService` 给异步记忆抽取加了 timeout，但下面两条路径还在直接调用：

- `go mem.ExtractMemories(context.Background(), ...)`
- `/chat`
- `/chat_stream`

结果：

- 主聊天链路和 AI Ops 链路的 memory safety 行为不一致
- 后台任务没有 deadline
- 一旦未来 memory extraction 变复杂，风险会快速放大

---

## 3.2 修复思路

统一所有入口都通过 `MemoryService.PersistOutcome(...)` 进行持久化和后台抽取，而不是由各个 controller 自己决定怎么写 short-term memory、怎么起 goroutine。

本轮实现：

- `/chat` 改为调用 `MemoryService.PersistOutcome`
- `/chat_stream` 在完整响应生成后也改为调用 `MemoryService.PersistOutcome`
- 去掉 controller 内部裸起的 `ExtractMemories(context.Background(), ...)`

---

## 3.3 为什么这样修

这次不是继续在 controller 里补 timeout，而是直接统一到 service 层，原因是：

- memory 持久化本来就应该是服务职责，而不是控制器职责
- 一旦以后还要改 timeout、限流、抽取策略，只需要改 `MemoryService`
- 这能避免同类修复再次只修一条路径

一句话说，就是把“局部修复”变成“统一入口修复”。

---

## 3.4 代码落点

- [chat_v1_chat.go](/Users/agiuser/Agent/OpsCaptionAI/internal/controller/chat/chat_v1_chat.go)
- [chat_v1_chat_stream.go](/Users/agiuser/Agent/OpsCaptionAI/internal/controller/chat/chat_v1_chat_stream.go)
- [memory_service.go](/Users/agiuser/Agent/OpsCaptionAI/internal/ai/service/memory_service.go)

---

## 4. 本轮修改后的系统状态

经过这一轮收口后：

- `AI Ops` 审批门拒绝现在是可解释业务结果，不再被误判为内部错误
- `AI Ops / chat / chat_stream` 三条主要入口都统一走有界的 memory extraction 路径
- memory 持久化职责进一步收敛到 `MemoryService`

这意味着：

- 运行语义更一致
- 后续更容易做 memory policy 统一治理
- review 里这种“只修到一半”的问题会减少

---

## 5. 复盘与经验

## 5.1 这次问题的根因

根因不是实现能力不足，而是“修复没有沿所有调用路径收口”。

具体表现：

- 先修了 AI Ops，再忘了 Chat
- service 逻辑变了，但 controller 仍保留旧假设

这属于典型的“跨层收口不完整”。

---

## 5.2 今后怎么避免

建议以后每次修复都检查这 3 件事：

1. 是否所有同类入口都已经统一接入
2. 是否还有 controller/handler 保留旧假设
3. 是否需要把逻辑上移到统一 service 层

建议的检查清单：

- 是否存在多条相似请求路径
- 是否有多个入口在做同一类 side effect
- 是否仍有 `context.Background()` 裸启动后台任务
- 是否“空结果”与“业务拒绝”被混为一谈

---

## 5.3 这轮最重要的工程经验

### 经验 1

有界异步必须是统一基础设施，不该靠每个 controller 自觉实现。

### 经验 2

审批拒绝、权限拒绝、策略拒绝，应该优先建模成业务结果，而不是简单归类为异常。

### 经验 3

当一个修复已经体现出“统一服务边界”的方向时，应该继续推进边界收口，而不是在多个入口复制同样的修法。

---

## 6. 后续建议

在这轮修复基础上，建议继续做两件事：

1. 为所有后台 AI 任务建立统一 helper，而不是只覆盖 memory extraction
2. 为业务拒绝类场景补统一 response contract，例如：
   - `status`
   - `reason`
   - `trace_id`
   - `detail`

这样后续无论是 approval gate、tool gate，还是 feature gate，都能复用同一套语义。

---

## 7. 一句话总结

这轮修复的核心价值不是“补了两个 bug”，而是：

> 把局部生效的安全修复，进一步收口成跨入口一致的系统行为，并且把审批拒绝从“错误”纠正为“可解释的业务结果”。

