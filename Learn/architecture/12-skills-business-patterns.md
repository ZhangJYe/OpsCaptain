# 第二轮 Skills 业务化改造笔记

## 1. 这一轮改了什么

第一轮我只是把项目补成了“有 skills 框架”的版本。  
这一轮做的是更重要的一步：

- 不再停留在 `generic skill`
- 开始补 `业务场景 skill`
- 让不同 skill 真正带不同的检索/执行意图

这次新增的业务 skill 分三组。

### Knowledge

- `knowledge_release_sop`
- `knowledge_rollback_runbook`

### Metrics

- `metrics_release_guard`
- `metrics_capacity_snapshot`

### Logs

- `logs_payment_timeout_trace`
- `logs_auth_failure_trace`

## 2. 为什么这一步更像“真实项目”

如果一个项目只有：

- `knowledge_sop_lookup`
- `metrics_alert_triage`
- `logs_evidence_extract`

那它虽然已经 skill 化了，但还是偏“技术抽象层”。

真正接近业务落地时，skill 应该围绕高频场景来命名和设计，比如：

- 发布前要不要继续 rollout
- 出现异常后是否需要回滚
- 支付超时到底卡在哪一层
- 登录失败是 token、jwt、权限还是 middleware 问题

这类 skill 有三个优势：

1. 更贴近日常排障任务
2. 更容易给面试官讲“这个 skill 解决什么业务问题”
3. 更适合后续做专项评测

## 3. 这次不是只加名字，而是加了“行为差异”

这轮最重要的点是：

> skill 不只是换个名字，而是要带不同的执行 focus。

### Knowledge skill 的行为差异

我给 knowledge skill 增加了 query rewrite。

例如：

- `knowledge_release_sop`
  会给检索 query 补上：
  - pre-check
  - post-check
  - verification
  - rollback

- `knowledge_rollback_runbook`
  会给检索 query 补上：
  - rollback triggers
  - mitigation actions
  - recovery steps
  - validation checklist

也就是说，同样是查知识库，skill 不同，送给 RAG 的 query 也不同。

### Metrics skill 的行为差异

metrics 这边没有新的检索 tool，所以这轮主要做了：

- 更细的 skill 选择
- 不同的 summary prefix
- 不同的 `NextActions`

例如：

- `metrics_release_guard`
  会给出和发布窗口、canary、rollback criteria 相关的 next actions

- `metrics_capacity_snapshot`
  会给出 CPU / memory / saturation / autoscaling 相关的 next actions

### Logs skill 的行为差异

logs 这边我让 payload 带上了 skill focus。

例如：

- `logs_payment_timeout_trace`
  payload 会倾向：
  - payment
  - order
  - checkout
  - gateway timeout
  - db timeout
  - downstream latency

- `logs_auth_failure_trace`
  payload 会倾向：
  - login
  - token
  - jwt
  - unauthorized
  - forbidden
  - auth middleware

这才是“业务 skill”和“通用 skill”的真正差别。

## 4. 这一轮的设计原则

你复盘时记住这 4 条就够了。

### 第一，不要为了 skill 而 skill

我没有把所有东西都拆成 skill。  
只把高频、可复用、容易解释的业务场景拆出来。

### 第二，specific skill 要排在 generic skill 前面

比如 `rollback` 查询里经常也带 `release`。  
所以 `knowledge_rollback_runbook` 必须排在 `knowledge_release_sop` 前面，不然会被错误匹配。

这是实际测试里暴露出来并修掉的。

### 第三，skill 要能留下证据

我这轮保留了这些信息：

- `skill_name`
- `skill_description`
- `knowledge_query`
- `metrics_focus`
- `log_focus`

这样以后做 trace、排错、面试讲设计，都会容易很多。

### 第四，先做“可解释差异”，再做“智能差异”

这轮 skill 的差异主要是：

- query rewrite
- focus metadata
- next actions

这已经能体现明显的业务差异。  
以后再继续升级成：

- skill-specific rerank
- skill-specific eval harness
- skill-specific success metrics

## 5. 这一轮怎么测试

### Knowledge

我测了：

- query 是否命中 `release_sop`
- query 是否命中 `rollback_runbook`
- 重写后的 query 是否真的包含 focus 语义

### Metrics

我测了：

- `release_guard` 是否被选中
- `capacity_snapshot` 是否被选中
- `NextActions` 是否真的带业务建议

### Logs

我测了：

- `payment_timeout_trace` 是否被选中
- `auth_failure_trace` 是否被选中
- payload 里是否真的带了不同 focus
- generic / raw review 回退是否仍然成立

## 6. 这一轮你最该学会什么

最重要的不是记住 skill 名字，而是学会这句话：

> 一个好的 skill，必须对应一个清晰业务场景，并且在执行行为上和别的 skill 有可观察的差异。

如果只是名字不同，但行为完全一样，那不叫 skill 设计，只是“换皮”。
