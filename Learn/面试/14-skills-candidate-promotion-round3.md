# Skills 候选卡片落地第 3 轮

## 这轮做了什么

这次把 `skills/candidates` 里的 3 个候选卡片，真正落成了运行时代码 skill：

1. `logs_service_offline_panic_trace`
2. `logs_api_failure_rate_investigation`
3. `knowledge_service_error_code_lookup`

这一步很关键，因为它代表项目已经不是“有一堆设计想法”，而是把候选 skill 从 Markdown 变成了真正可执行、可测试、可复盘的 runtime skill。

## 为什么选这 3 个

选择标准很简单：

- 触发条件稳定
- 查询 focus 明确
- 排障套路固定
- 很适合在面试里讲清楚

对应关系是：

- `service_offline_panic_trace`：典型的日志排障套路
- `api_failure_rate_investigation`：典型的接口故障套路
- `service_error_code_lookup`：典型的结构化知识检索套路

## 代码上到底改了什么

### 1. 给 knowledge specialist 增加了精确 matcher

文件：

- `internal/ai/agent/skillspecialists/knowledge/agent.go`

这次不是只加一个新的 skill name，而是让 `knowledgeSkill` 支持 `matcher`。  
原因是错误码问题很适合“精确命中”：

- query 里要先出现数字型错误码
- 再结合 `error code / 错误码 / code` 这类语义

这样比只靠模糊关键词更稳定。

### 2. 给 knowledge skill 增加了结构化 metadata

这轮新增了：

- `extracted_error_codes`
- `knowledge_mode=service_error_code_lookup`
- 对应的 `next_actions`

这意味着结果不只是“查到几篇文档”，而是会把“这次具体命中了哪个错误码”也带出来。

### 3. 给 logs specialist 增加了场景级 matcher

文件：

- `internal/ai/agent/skillspecialists/logs/agent.go`

这是本轮最重要的工程点。  
如果只靠关键词，很容易发生误命中：

- 看到 `panic` 就误以为是“服务下线排查”
- 看到 `order` 就误以为是“支付超时排查”

所以这轮把它收紧成了“场景条件”：

- `logs_service_offline_panic_trace`
  必须同时满足：
  - panic 类信号
  - offline / restart / crashloop 类信号

- `logs_payment_timeout_trace`
  现在不再把所有带 `order` 的 query 都吞进去
  - 显式 `API failure rate / 5xx / endpoint / route` 会让位给 API 故障 skill

### 4. 给新的 logs skill 增加了 next actions

这一步很适合你准备面试时讲：

- `service_offline_panic_trace`
  - 对齐发布时间
  - 查 restart count / crash reason

- `api_failure_rate_investigation`
  - 先区分 4xx / 5xx
  - 再看是否是上下游依赖变化

这说明 skill 不只是“选工具”，而是“封装了排障套路”。

## 测试怎么做的

本轮补了定向单测：

- `internal/ai/agent/skillspecialists/knowledge/agent_test.go`
- `internal/ai/agent/skillspecialists/logs/agent_test.go`

验证点包括：

- skill 是否正确命中
- query 是否真的带上对应 focus
- `skill_mode` / `knowledge_mode` 是否正确
- `next_actions` 是否输出
- `extracted_error_codes` 是否真的提取到了错误码

本地测试命令：

```powershell
New-Item -ItemType Directory -Force '.gotmp-skill-promote-4' > $null
New-Item -ItemType Directory -Force '.gocache-skill-promote-4' > $null
$env:GOTMPDIR=(Resolve-Path '.gotmp-skill-promote-4').Path
$env:GOCACHE=(Resolve-Path '.gocache-skill-promote-4').Path
go test ./internal/ai/agent/skillspecialists/knowledge ./internal/ai/agent/skillspecialists/logs
```

测试结果里两个包都打印了 `ok`。  
最后的 `Access is denied` 还是你这台 Windows 机器清理 `.gotmp-*` 的老问题，不是本轮 skill 逻辑失败。

## 你面试时可以怎么讲

你可以这样说：

> 我把项目里的 skill 不再只当成“关键词路由”，而是逐步改成“场景化能力单元”。  
> 例如服务下线 panic 排查，需要同时看到 panic 信号和 restart/offline 信号；API 失败率排查则要和 payment timeout skill 区分边界。  
> 在 knowledge 侧，我给错误码检索做了精确 matcher、错误码提取和 next actions，让 skill 的输出更结构化。

这段话比“我写了几个 if-else”强很多。

## 这轮之后项目的状态

现在项目里已经形成了 3 层：

1. `docs/`
   原始知识库语料
2. `skills/candidates/`
   候选 skill cards
3. `internal/ai/agent/skillspecialists/...`
   已经落地的运行时代码 skill

这是一个比较成熟的演进路径，后面继续做 skill 会更顺。
