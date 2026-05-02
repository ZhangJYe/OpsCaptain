# 第二轮 Skills 测试与面试讲法

## 1. 这轮可以怎么讲给面试官

你可以这样说：

> 第一轮我把系统从 `agent + tools` 升级成了 `agent + skills + tools`。第二轮我继续把 skill 做业务化，让 release、rollback、capacity、payment timeout、auth failure 这些高频场景拥有独立 skill，并且让不同 skill 产生不同的 query focus、payload 和 next actions。

这句话比“我又加了几个 skill”强很多。

## 2. 面试官如果问“你怎么判断一个 skill 值不值得加”

你可以答：

> 我主要看 3 个标准。第一，高频不高频；第二，能不能沉淀成稳定排障套路；第三，加进去之后执行行为是否真的和别的 skill 不同。如果三条都不满足，我就不急着 skill 化。

## 3. 面试官如果问“第二轮比第一轮难在哪”

可以这样答：

> 第一轮主要是框架层重构，重点是抽象和接线。第二轮难点在于 skill 边界划分，因为 release 和 rollback、payment timeout 和 generic error 之间都存在关键词重叠，必须通过优先级和测试把错误命中修掉。

这是很好的回答，因为它说明你真的踩过坑。

## 4. 面试官如果问“你遇到过什么具体问题”

你可以直接说这次真实遇到的例子：

> 我在做 `knowledge_release_sop` 和 `knowledge_rollback_runbook` 时，发现 rollback 请求里经常也带 release 关键词，导致路由先命中 release skill。最后我是通过调整 registry 中 specific skill 的优先级，并用测试固定住这个行为来解决的。

这就是很标准的工程经验回答。

## 5. 这轮的测试命令

这轮的定向测试可以这样跑：

```powershell
go test ./internal/ai/agent/skillspecialists/knowledge
go test ./internal/ai/agent/skillspecialists/metrics
go test ./internal/ai/agent/skillspecialists/logs
```

如果你这台 Windows 机器继续报 Go 清理临时目录的权限问题，可以临时这样跑：

```powershell
New-Item -ItemType Directory -Force '.gotmp-local' > $null
New-Item -ItemType Directory -Force '.gocache-local' > $null
$env:GOTMPDIR=(Resolve-Path '.gotmp-local').Path
$env:GOCACHE=(Resolve-Path '.gocache-local').Path
go test ./internal/ai/agent/skillspecialists/logs
```

要注意：

- 这类报错多数发生在退出时清理临时目录
- 如果前面已经看到 `ok`，通常说明编译和测试本身已经通过
- 它更像环境清理问题，不是这轮代码逻辑错误

## 6. 这一轮你可以背的 3 句高频回答

### 回答 1

> 第一轮我解决的是“有没有 skill”问题，第二轮我解决的是“skill 像不像真实业务能力”问题。

### 回答 2

> 我不是简单新增 skill 名称，而是让不同 skill 带不同 query focus、payload 和 next actions，这样 skill 才有真正的执行差异。

### 回答 3

> 我通过优先级、metadata 和定向测试，把 skill 的命中逻辑固定下来，避免 specific skill 被 generic skill 覆盖，或者反过来误命中。

## 7. 下一轮如果面试官追问“你还会怎么做”

你可以说：

> 下一轮我会把 triage 和 skill 进一步打通，给 triage 增加 skill hints；再给 runtime 增加按 `skill_name` 统计 success / degraded / failed；最后补 skill-specific harness，统计不同 skill 的命中率和有效性。

这就是一条很完整的 roadmap。
