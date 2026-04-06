# Skills 改造面试讲法

## 1. 先用一句话讲项目

你可以这样说：

> 我把一个原本是 multi-agent + tools 的运维助手，进一步改造成了 skills-driven agent architecture。现在 specialist agent 不再直接硬编码调用工具，而是先做 skill 选择，再由 skill 组合工具执行。

这句话很重要，因为它同时说明了：

- 你理解 multi-agent
- 你理解 tool
- 你知道怎么再往上抽象成 skill

## 2. 面试官如果问“为什么要做 skills”

你可以这样答：

> 原来的 specialist 把路由和执行都写在 `Handle()` 里，扩展性一般。每多一种处理套路，就会把 agent 继续写胖。我把它拆成 skill 以后，agent 负责选 skill，skill 负责具体执行套路，tool 只保留原子能力。这样更容易扩展、复用和观察。

关键词：

- 解耦
- 可扩展
- 可复用
- 可观察

## 3. 面试官如果问“你具体做了什么”

你可以按 4 点回答。

### 第一，我定义了通用 skill 抽象

我抽了一个 `Skill` 接口，里面有：

- `Name()`
- `Description()`
- `Match(task)`
- `Run(ctx, task)`

然后实现了一个 `Registry`，可以：

- 注册 skill
- 根据任务选择 skill
- 执行 skill
- 自动把 `skill_name / skill_description / skill_domain` 写进 result metadata

### 第二，我把 specialist 改成了 skills-driven

我没有直接推翻整个系统，而是先在 specialist 层落 skills：

- knowledge specialist
- metrics specialist
- logs specialist

每个 specialist 都有自己的 `skills.Registry`。

### 第三，我把 skill 选择结果接进了 trace

skill 不应该只是代码里存在，还要能观察到。

所以我在 runtime detail 里补了类似：

```text
[metrics] selected skill=metrics_alert_triage
```

这让系统从“能工作”升级成“能解释自己为什么这么工作”。

### 第四，我用平行迁移降低了改造风险

旧 specialist 我没有硬删，而是新增 `skillspecialists` 包，然后只改注册点。

这样做的好处是：

- 风险可控
- 容易回滚
- 便于和旧实现做对照

这是一种很典型的增量重构策略。

## 4. 面试官如果问“skill 和 tool 有什么区别”

这是高频题，你要答清楚。

### Tool

tool 是原子能力。

例子：

- 查 Prometheus
- 查知识库
- 查日志 MCP

### Skill

skill 是任务套路。

例子：

- `metrics_alert_triage`
- `knowledge_sop_lookup`
- `logs_evidence_extract`

它会决定：

- 什么时候该用这套套路
- 这套套路怎么调用 tool
- 结果怎样组织成 evidence / summary / metadata

### Agent

agent 是执行者。

它负责：

- 接任务
- 选择 skill
- 产出结果
- 写 trace

最短回答模板：

> tool 是动作，skill 是方法，agent 是执行者。

## 5. 面试官如果问“为什么不直接做全局 skill 路由”

你可以这样回答：

> 我这次故意没有一步到位做全局 skill router，而是先在 specialist 内部做 skill 化。因为这一步收益最大、风险最低，而且能保留现有 triage -> specialist 的稳定链路。等 specialist 内部稳定后，再往上抽全局 skill catalog 和统一 capability routing，会更稳。

这体现的是工程判断，不是技术炫耀。

## 6. 面试官如果问“你怎么验证它真的有效”

你可以这样说：

> 我做了三层验证。第一层是 skills registry 的单测，验证命中和回退逻辑。第二层是 specialist 单测，验证不同 query 会选中不同 skill，并把 `skill_name` 写进 metadata。第三层是 supervisor 和 service 的回归测试，确保整条链路切到 skills specialist 后仍然工作正常。

如果他继续追问命令，你可以说：

```powershell
go test ./internal/ai/skills
go test ./internal/ai/agent/skillspecialists/...
go test ./internal/ai/agent/supervisor ./internal/ai/service
```

## 7. 面试官如果问“最大的 tradeoff 是什么”

你可以答：

> 这次我引入了更多抽象层，所以代码文件会变多，初看会比直接写在 agent 里更绕。但换来的收益是新增能力时不用反复改一个大 `Handle()`，也更适合做 trace、评测和逐步扩展。

## 8. 可以直接背的 STAR 版本

### S

项目原来已经有 multi-agent 和 tools，但 specialist 逻辑都堆在 `Handle()` 里，扩展和复用一般。

### T

我要在不打断现有业务链路的前提下，把系统补成真正的 skills 版本，并保证可测试、可回滚、可讲清楚。

### A

我先抽了通用的 `Skill + Registry`，然后新增一套 `skillspecialists`，分别给 knowledge、metrics、logs 落了多个 skill。接着只修改 supervisor 和 runtime 注册点，把真实执行流量切到新的 skills specialist。最后补测试和 trace，把 skill 选择结果写进 detail。

### R

结果是系统从原来的 `agent + tools` 升级成了 `agent + skills + tools`。后续新增处理套路时，只需要加一个 skill，不需要持续膨胀原来的 agent。并且运行时可以直接观察本次请求选中了哪个 skill。

## 9. 你自己复盘时最该记住的 3 句话

1. 不要把 tool、skill、agent 混成一个概念。
2. 真正好的重构不是“一把梭重写”，而是“先平行迁移，再切注册点”。
3. 面试里不要只说“我做了抽象”，要说“我为什么这样抽，怎么验证，tradeoff 是什么”。
