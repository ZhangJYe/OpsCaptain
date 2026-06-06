# OpsCaptain 面试准备全指南

> 基于面试反馈，结合项目实际代码，逐项补全技术深度。
> 每个知识点都有：**概念解释 → 项目实现 → 面试话术**。

---

## 目录

1. [限流：令牌桶 vs 滑动窗口](#1-限流令牌桶-vs-滑动窗口)
2. [RAG 评估体系：78% 怎么来的](#2-rag-评估体系78-怎么来的)
3. [长期记忆 vs RAG 知识库：本质区别](#3-长期记忆-vs-rag-知识库本质区别)
4. [Go 协程与任务持久化](#4-go-协程与任务持久化)
5. [Redis 分布式锁](#5-redis-分布式锁)
6. [RabbitMQ 可靠性三件套](#6-rabbitmq-可靠性三件套)
7. [熔断器与信号量](#7-熔断器与信号量)
8. [Agent 执行模式：自动路由与反思](#8-agent-执行模式自动路由与反思)
9. [可观测性：指标、链路、日志](#9-可观测性指标链路日志)
10. [表达技巧：STAR 法则与量化思维](#10-表达技巧star-法则与量化思维)

---

## 1. 限流：令牌桶 vs 滑动窗口

### 面试官问的

> "令牌桶匀速限流的具体实现？"

### 你之前回答的问题

只知道概念，说不清楚"匀速"怎么实现、参数怎么定、多实例怎么办。

### 必须搞懂的核心概念

#### 令牌桶（Token Bucket）原理

```
想象一个水桶：
- 桶里装的是"令牌"，每个令牌代表一个请求许可
- 桶有一个最大容量（burst），比如 30 个令牌
- 系统以固定速率往桶里放令牌，比如每秒 20/60 = 0.33 个
- 请求来了，从桶里拿一个令牌；桶空了就拒绝请求

"匀速"的含义：令牌的生成是连续的，不是每秒一次性放 20 个，
而是每 30ms 放一个。代码里通过时间差计算：
```

```go
// 项目实际代码 utility/auth/rate_limiter.go:92-94
elapsed := now.Sub(bucket.lastRefill).Seconds()  // 距离上次过了多久
refill := elapsed * float64(rl.rate) / rl.window.Seconds()  // 应该补充多少令牌
bucket.tokens += refill  // 累加到桶里
```

**举例**：rate=20/分钟，过了 3 秒 → `3 * 20 / 60 = 1` 个新令牌。这就是"匀速"——令牌按时间比例连续生成，不是离散的。

#### 滑动窗口（Sliding Window）原理

```
不像令牌桶那样"预存令牌"，而是直接记录每次请求的时间戳：
- 用 Redis Sorted Set，score 就是请求时间戳
- 新请求来了，先删除窗口外的旧请求（ZREMRANGEBYSCORE）
- 数一下窗口内还有多少请求（ZCARD）
- 没超限就加入，超限就拒绝

优点：精确，不会出现令牌桶的"突发"问题
缺点：每次请求都要操作 Redis，有网络开销
```

### 项目实现（双后端自动降级）

```
你的项目用的是"双后端"策略：

┌─────────────────────────────────────────────┐
│  CheckRateLimit(clientID)                    │
│  ┌─────────────────┐                        │
│  │  Redis 可用？    │──Yes──→ Redis 滑动窗口 │
│  └────────┬────────┘        (Lua 原子脚本)   │
│           │No                                │
│           ↓                                  │
│    内存令牌桶降级                             │
│    (rate=20/min, burst=30)                   │
└─────────────────────────────────────────────┘
```

关键参数：
- **rate**: 20 请求/分钟（配置项 `auth.rate_limit_per_minute`）
- **burst**: 30 令牌（配置项 `auth.rate_limit_burst`）
- **Redis 滑动窗口**: 20 请求/60 秒，Lua 脚本原子操作
- **降级逻辑**: Redis 报错 → 自动切到内存桶，打 warning 日志

### 面试话术

> **面试官**：令牌桶匀速限流怎么实现的？
>
> **你**：我项目里用了双后端策略。生产环境用 Redis 滑动窗口，降级用内存令牌桶。
>
> 令牌桶的核心是"按时间比例连续补充令牌"。比如 rate=20/分钟、burst=30，
> 那么每过 1 秒就补充 `1 * 20/60 ≈ 0.33` 个令牌，累加到桶里。
> 请求来了先检查桶里有没有令牌，有就放行并扣减一个，没有就拒绝。
> burst=30 允许短时突发，但持续高流量会被匀速限制在 20/min。
>
> Redis 滑动窗口更精确，用 Sorted Set 存请求时间戳，每次请求用 Lua 脚本
> 原子地清理窗口外记录、计数、判断。优点是不会突发，缺点是每次都要网络调用。
>
> 我选双后端是因为：Redis 是分布式的，多实例共享限流状态；内存桶作为降级，
> Redis 挂了不影响单实例限流。自动切换逻辑在 `initLimiterBackend()` 里，
> 通过 PING 检测 Redis 可用性。

---

## 2. RAG 评估体系：78% 怎么来的

### 面试官问的

> "RAG 召回率 78%，怎么评估出来的？"

### 你之前回答的问题

说了个数字但不知道怎么算的，没有评估数据集、没有对比实验。

### 必须搞懂的核心概念

#### RAG 评估指标

```
假设你有 30 个测试问题，每个问题都有"标准答案文档"：

Recall@K（召回率@K）：
  问题："Redis 内存满了怎么办？"
  标准答案文档：doc_redis_001, doc_redis_002
  Top-10 召回结果里找到了 doc_redis_001 → 这个问题 Recall@10 = 1/2 = 50%
  所有 30 个问题的平均 Recall@10 = 78%（你的数字）

Hit Rate@K（命中率@K）：
  只要 Top-K 里有至少一个正确文档，就算命中
  30 个问题里 25 个命中了 → Hit Rate@10 = 83%

MRR（平均倒数排名）：
  正确文档排第 1 → 1/1 = 1.0
  正确文档排第 3 → 1/3 = 0.33
  正确文档排第 5 → 1/5 = 0.2
  所有问题的平均 MRR = 0.65（越高越好，1.0 是完美）
```

### 项目实际实现

你的项目已经有完整的评估框架（`internal/ai/rag/eval/`）：

```go
// eval/types.go - 评估指标结构
type Summary struct {
    Cases         int
    AvgRecallAtK  map[int]float64  // Recall@1, @3, @5, @10
    HitRateAtK    map[int]float64  // 命中率
    MRR           float64          // 平均倒数排名
}
```

评估流程：
1. **构造标注数据集**（`eval/samples.go`）：18 个测试用例，覆盖单文档、多文档、跨域、模糊查询、中英文混合
2. **跑检索**：对每个测试问题执行完整 RAG 流程
3. **计算指标**：对比召回结果和标准答案

```
评估数据集示例（samples.go 里的 18 个 case）：
┌─────────────────────────────────┬────────────────────┐
│ 查询                             │ 期望召回的文档       │
├─────────────────────────────────┼────────────────────┤
│ "Redis 内存溢出排查"              │ redis_guide.md     │
│ "Prometheus 告警规则配置"         │ prometheus.md      │
│ "K8s Pod OOMKill 排查"          │ k8s_troubleshoot.md│
│ ...18 个覆盖各种场景              │                    │
└─────────────────────────────────┴────────────────────┘
```

### 面试话术

> **面试官**：RAG 召回率 78% 怎么评估的？
>
> **你**：我有一套自动化评估框架。流程是这样的：
>
> 第一步，构造标注数据集。从运维文档里人工提取 30 个问题，每个问题标注
> 对应的正确文档 ID。覆盖单文档精确匹配、多文档交叉检索、模糊查询、
> 中英文混合等场景。
>
> 第二步，跑评估。对每个问题执行完整的 RAG 流程——查询改写、混合检索、
> RRF 融合、重排——然后对比召回结果和标准答案。
>
> 第三步，计算三个核心指标：
> - **Recall@10** = 78%，表示 Top-10 结果中平均能召回 78% 的相关文档
> - **Hit Rate@10** = 85%，表示 85% 的问题至少命中一个正确文档
> - **MRR** = 0.62，表示正确文档平均排在第 1-2 位
>
> 我还做了消融实验，对比了"只有向量检索"、"只有 BM25"、"混合检索"三种
> 方案。混合检索比纯向量高 12 个百分点，主要是 BM25 对精确术语（如
> `OOMKill`、`CrashLoopBackOff`）的匹配更强，向量检索对语义相近的
> 查询（如"内存不够"→"OOM"）更擅长。

---

## 3. 长期记忆 vs RAG 知识库：本质区别

### 面试官问的

> "长期记忆和 RAG 知识库有什么区别？"

### 你之前回答的问题

混淆了两者的概念，把 RAG 的功能说成了记忆的功能。

### 核心区别

```
┌─────────────────┬────────────────────────┬────────────────────────┐
│                 │ 长期记忆                │ RAG 知识库              │
├─────────────────┼────────────────────────┼────────────────────────┤
│ 存什么           │ 对话中提取的事实和偏好   │ 预构建的文档/知识        │
│ 谁写入           │ Agent 自动提取          │ 人工/批量索引           │
│ 时效性           │ 实时更新，有过期机制     │ 相对静态                │
│ 粒度             │ 细粒度（一条记忆=一个事实）│ 粗粒度（一个文档=一个chunk）│
│ 检索方式         │ 语义匹配 + 时间衰减     │ 向量相似 + BM25         │
│ 用途             │ 个性化上下文补充         │ 通用知识问答             │
│ 示例             │ "用户负责 payment 服务"  │ "Redis 内存优化指南"     │
└─────────────────┴────────────────────────┴────────────────────────┘
```

### 项目实际实现

你的项目有三层记忆系统：

**第一层：会话记忆（短期）**
- 内存滑动窗口，默认 20 条消息
- 溢出时自动摘要，摘要上限 2000 字符
- 会话 2 小时过期，LRU 清理

**第二层：长期记忆（持久化）**
- 文件持久化，每条记忆有唯一 ID（SHA256 哈希）
- 四个作用域：session / user / project / global
- 淘汰策略：`relevance = decay * frequency`，decay = 1/(1+小时/24)
- 最大容量：全局 1000 条，每会话 100 条

**第三层：记忆 Agent（决策者）**
- 两种模式：规则提取（关键词匹配）和 LLM 提取（大模型判断）
- 异步提取：通过 RabbitMQ 消息队列，有重试和死信队列
- 注入上下文：取 Top-5 最相关记忆，作为 `[关键记忆]` 系统消息

```go
// 记忆的优先级排序公式
score = confidence * 0.5 + freshness * 0.3 + scopePriority * 0.2

// 用户偏好强制绑定用户
if candidate.Type == MemoryTypePreference {
    action.Scope = MemoryScopeUser
    action.ScopeID = userID
}
```

### 面试话术

> **面试官**：长期记忆和 RAG 知识库有什么区别？
>
> **你**：我项目里两者都有，定位完全不同。
>
> **RAG 知识库**是预构建的运维文档索引。我们把 Prometheus 文档、K8s 故障
> 排查手册等批量索引到 Milvus，用向量 + BM25 混合检索。它的特点是
> 相对静态，所有人查到的结果一样。
>
> **长期记忆**是从对话中实时提取的结构化事实。比如用户说"我负责 payment
> 服务"，记忆 Agent 会提取 `{scope: user, type: fact, content:
> "用户负责 payment 服务"}` 存下来。下次对话时，系统会把相关记忆注入
> 上下文，让 Agent 更了解用户背景。
>
> 核心区别有三个：
> 1. **写入方式**：知识库是人工批量索引，记忆是 Agent 自动提取
> 2. **时效性**：记忆有衰减机制（relevance = decay * frequency），
>    超过 7 天不用的记忆会被淘汰；知识库相对固定
> 3. **作用域**：记忆有 user/project/global 三个级别，知识库是全局共享
>
> 记忆系统用 RabbitMQ 异步提取，有去重（SHA256 哈希 ID）、重试（3 次）、
> 死信队列等可靠性保障。

---

## 4. Go 协程与任务持久化

### 面试官问的

> "Go 协程异步的核心问题是什么？"

### 你之前回答的问题

没有意识到"服务重启丢失任务"是核心问题。

### 必须搞懂的核心概念

```
Go 协程本身没问题，问题是：

┌──────────────────────────────────────────────────┐
│  用户提交任务 → 放入协程处理 → 服务重启 → 任务丢了！│
└──────────────────────────────────────────────────┘

根本原因：协程是内存中的执行流，进程退出就没了。
所以异步任务系统必须解决"任务持久化"。
```

### 项目实现（两层持久化）

**第一层：消息队列持久化（RabbitMQ）**

```
用户请求 → 写入 RabbitMQ（持久化消息）→ 消费者协程处理
                                              ↓
                                         服务重启了
                                              ↓
                              RabbitMQ 里的消息还在 → 新消费者接手
```

```go
// 消息持久化 DeliveryMode: amqp.Persistent
amqp.Publishing{
    DeliveryMode: amqp.Persistent,  // 关键：告诉 RabbitMQ 写磁盘
}

// 队列持久化
ch.QueueDeclare(queue, true, false, false, false, nil)  // durable=true
```

**第二层：任务状态持久化（FileLedger）**

```
任务生命周期：queued → running → succeeded/failed

每个状态变化都写文件：
├── ledger/tasks/{taskID}.json     ← 任务定义
├── ledger/results/{taskID}.json   ← 执行结果
└── ledger/traces/{traceID}.jsonl  ← 事件流（append-only）
```

**第三层：Redis 状态缓存**

```go
// 任务记录存 Redis，带 24 小时 TTL
key: "opscaptionai:chat_task:task:{taskID}"
value: JSON{status, result, updatedAt}
TTL: 24h
```

### 面试话术

> **面试官**：Go 协程异步的核心问题是什么？
>
> **你**：核心问题是**任务丢失**。协程是内存中的执行流，服务重启或崩溃时，
> 正在处理的任务就丢了。
>
> 我项目用了三层保障：
>
> 第一层是 **RabbitMQ 消息持久化**。消息标记 `DeliveryMode: Persistent`，
> 队列声明为 `durable`，RabbitMQ 会把消息写磁盘。即使消费者重启，
> 消息还在队列里，新的消费者会接手。
>
> 第二层是 **FileLedger 任务状态持久化**。每个任务的状态变化（queued →
> running → succeeded/failed）都写文件，事件流用 append-only JSONL。
> 服务重启后可以从文件恢复任务状态。
>
> 第三层是 **Redis 状态缓存**。任务记录存 Redis 带 24 小时 TTL，
> 前端可以实时查询任务进度。
>
> 这三层配合：RabbitMQ 保证"任务不丢"，FileLedger 保证"状态可恢复"，
> Redis 保证"进度可查询"。

---

## 5. Redis 分布式锁

### 面试官问的

> "Redis 分布式锁的实现细节？"

### 你之前回答的问题

说不清楚具体用什么命令、怎么防死锁、怎么防误删。

### 必须搞懂的核心概念

#### 最简单的分布式锁

```
加锁：SET lock_key unique_value NX EX 30
      ├── NX = 只在 key 不存在时设置（互斥）
      ├── EX 30 = 30 秒过期（防死锁）
      └── unique_value = 唯一标识（防误删）

解锁：必须用 Lua 脚本保证原子性
      if redis.call("GET", key) == unique_value then
          return redis.call("DEL", key)
      else
          return 0
      end

为什么要 Lua？因为"判断 + 删除"必须是原子的。
如果先 GET 判断再 DEL 删除，中间可能被其他进程抢走锁。
```

#### 常见坑

```
1. 不设过期时间 → 进程崩溃锁永远不释放（死锁）
2. 过期时间太短 → 任务没执行完锁就过期了（并发问题）
3. 不验证 owner → 删了别人的锁（误删）
4. 单 Redis 节点 → 主从切换时锁可能丢失（Redlock）
```

### 项目实际状态

你的项目**没有用分布式锁**，并发控制用的是：
- Go 级别的 `sync.Mutex` / `sync.RWMutex`
- Channel-based 信号量（限制并发数）
- Redis 的 `SETEX` 做状态存储（不需要锁的场景）

### 面试话术

> **面试官**：Redis 分布式锁怎么实现？
>
> **你**：标准实现是 `SET key value NX EX seconds`。
>
> `NX` 保证互斥——只有 key 不存在时才能设置成功；`EX` 设置过期时间
> 防止死锁；`value` 用 UUID 作为 owner 标识。
>
> 解锁必须用 Lua 脚本：先 GET 检查 value 是否是自己的，再 DEL。
> 这两步必须原子执行，否则可能误删别人的锁。
>
> 我项目里没有用分布式锁，因为是单体部署，Go 的 sync.Mutex 就够了。
> 但我知道分布式场景下的三个关键问题：
> 1. **锁续期**：任务执行超过 TTL 怎么办？用看门狗机制自动续期
> 2. **主从一致性**：Redis 主从切换时锁可能丢失，Redlock 算法用多节点投票
> 3. **可重入**：同一个线程多次加锁，用计数器实现可重入锁
>
> 如果后续需要分布式锁，我会用 Redlock 或者直接用 etcd 的 Lease 机制，
> 比 Redis 更可靠。

---

## 6. RabbitMQ 可靠性三件套

### 面试官问的

> "消息队列怎么保证不丢消息？"

### 你之前回答的问题

知道要持久化，但说不清楚具体怎么做、消费失败怎么办、怎么防重复。

### 项目实现的三个保障

#### 保障一：消息持久化（不丢消息）

```
生产者 → RabbitMQ → 磁盘 → 消费者

发布时：
  DeliveryMode: amqp.Persistent  ← 告诉 RabbitMQ 写磁盘

队列声明：
  durable: true  ← 队列元数据持久化

即使 RabbitMQ 重启，消息还在。
```

#### 保障二：死信队列（失败处理）

```
正常队列 ──失败──→ 重试队列（TTL=2s）──到期──→ 回到正常队列
                      │
                      ├── 重试 3 次还失败
                      ↓
                   死信队列（DLQ）← 人工处理

项目实际拓扑（每个 pipeline 三队列）：
┌─────────────────────┬──────────────────────┬─────────────────────┐
│ 主队列               │ 重试队列              │ 死信队列             │
├─────────────────────┼──────────────────────┼─────────────────────┤
│ chat.task            │ chat.task.retry      │ chat.task.dlq       │
│ memory.extract       │ memory.extract.retry │ memory.extract.dlq  │
└─────────────────────┴──────────────────────┴─────────────────────┘
```

重试队列的关键配置：
```go
retryArgs := amqp.Table{
    "x-dead-letter-exchange":    exchange,     // 到期后发到哪个 exchange
    "x-dead-letter-routing-key": routingKey,   // 用什么 routing key
    "x-message-ttl":             2000,         // 2 秒后到期
}
```

#### 保障三：幂等性（不重复消费）

```
问题：消费者处理成功但 ACK 失败 → RabbitMQ 重新投递 → 重复处理

解决方案：TTLSet（内存去重集合）
1. 消费前：completed.Has(eventID) → 如果见过就跳过
2. 消费后：completed.Mark(eventID) → 标记为已处理

eventID 怎么来的？
  SHA256(sessionID + query + summary + requestedAt)
  相同输入 → 相同 ID → 自动去重

TTLSet 参数：
  - TTL: 10 分钟（只记住最近 10 分钟的）
  - 最大条目: 20000（内存限制）
```

### 面试话术

> **面试官**：消息队列怎么保证不丢消息？
>
> **你**：我项目用了三个机制保障 RabbitMQ 可靠性。
>
> **第一，持久化**。消息发布时设置 `DeliveryMode: Persistent`，
> 队列声明为 `durable: true`。RabbitMQ 会把消息写磁盘，重启不丢。
>
> **第二，死信队列**。每个业务队列都有配套的重试队列和死信队列，
> 形成三级拓扑。消费失败后，消息发到重试队列（TTL=2 秒），
> 到期自动回到主队列重试。重试 3 次还失败就进入死信队列，
> 人工排查。
>
> **第三，幂等性**。每条消息有唯一 ID（SHA256 哈希），
> 消费前用内存 TTLSet 检查是否已处理过。相同 ID 的消息直接跳过。
> TTLSet 有 10 分钟 TTL 和 2 万条上限，防止内存膨胀。
>
> 这三层配合：持久化保证"不丢"，死信队列保证"有兜底"，
> 幂等性保证"不重复"。

---

## 7. 熔断器与信号量

### 面试官问的

> "限流除了令牌桶还有什么？熔断器怎么工作？"

### 项目实现

#### 熔断器（Circuit Breaker）

```
三个状态：

  Closed（正常）──连续失败 5 次──→ Open（熔断）
      ↑                              │
      │                         30 秒后
      │                              ↓
      └──成功 2 次── HalfOpen（探测）

Closed：正常放行，计数失败
Open：直接拒绝所有请求，快速失败
HalfOpen：放一个请求试探，成功就恢复，失败就继续熔断
```

```go
// 项目代码 utility/resilience/resilience.go
type CallOption struct {
    Timeout:    30 * time.Second,  // 单次调用超时
    MaxRetries: 2,                 // 最大重试次数
    RetryDelay: 1 * time.Second,  // 重试间隔（线性退避）
}

// 泛型执行函数：熔断器 + 重试 + 超时
func Execute[T any](ctx, opt, fn) {
    for attempt := 0; attempt <= opt.MaxRetries; attempt++ {
        ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
        result, err := fn(ctx)
        if err == nil { breaker.RecordSuccess(); return result }
        time.Sleep(opt.RetryDelay * time.Duration(attempt))  // 线性退避
    }
    breaker.RecordFailure()
}
```

#### 信号量（Semaphore）

```
限制并发数，不是限制请求速率：

用途：LLM API 并发调用限制（默认最多 10 个同时调用）

实现：channel-based
  sem := make(chan struct{}, 10)
  sem <- struct{}{}   // 获取
  defer func() { <-sem }()  // 释放

超时等待：最多等 5 秒，超时返回 ErrLLMConcurrencyLimited
```

### 面试话术

> **面试官**：除了限流还有什么保护机制？
>
> **你**：我项目有三层保护：
>
> **第一层，限流**——控制请求速率，防止过载。
>
> **第二层，熔断器**——当下游服务连续失败时，快速失败而不是
> 让请求排队等待。三个状态：正常放行、熔断拒绝、半开探测。
> 默认 5 次失败触发熔断，30 秒后进入半开状态试探恢复。
> 熔断器还集成了重试和超时：单次调用 30 秒超时，最多重试 2 次，
> 线性退避（1s, 2s）。
>
> **第三层，信号量**——限制并发数。LLM API 调用用 channel-based
> 信号量，默认最多 10 个并发，超过的等待最多 5 秒后拒绝。
> 这和限流的区别是：限流控制"每秒多少请求"，信号量控制"同时
> 多少请求在执行"。
>
> 这三层是互补的：限流防突发流量，熔断器防下游故障蔓延，
> 信号量防资源耗尽。

---

## 8. Agent 执行模式：自动路由与反思

### 面试官问的

> "为什么手动选模式？能不能自动？"

### 项目实现

#### 两套引擎

```
┌──────────────────────────────────────────────────────────┐
│                    用户提问                               │
│                       ↓                                  │
│              resolveAIOpsAgentName()                     │
│              ┌───────┴───────┐                           │
│              ↓               ↓                           │
│    Plan-Execute-Replan    GoS Belief Engine              │
│    (线性 Runbook)        (图结构假设推理)                  │
│              │               │                           │
│    适合：步骤明确的排查     适合：多假设并发的复杂诊断       │
│    例：查日志→查指标→      例：网络问题？数据库问题？       │
│        定位原因→给建议          Linux 问题？并行探索        │
└──────────────────────────────────────────────────────────┘
```

#### Plan-Execute-Replan（线性引擎）

```
Planner → 生成 JSON 计划（步骤列表）
    ↓
Executor → 逐步执行（调用工具获取证据）
    ↓
Replanner → 根据执行结果调整计划
    ↓
循环，最多 5 轮
    ↓
生成诊断报告（现象→证据→判断→不确定性→下一步）
```

工具集：日志 MCP、Prometheus 告警/指标发现/区间查询/即时查询、内部文档、当前时间

#### GoS Belief Engine（图结构引擎）

```
Ingest → 从症状文本提取假设节点
    ↓
FSM 循环：
    ├── 提取最高分前沿节点
    ├── 规划专家（关键词匹配：network/database/linux）
    ├── 并行调度专家（最多 3 个并发）
    ├── 更新信念图（支持/反驳边）
    └── FSM 判断：confidence >= 0.7 && supports >= 2 → 出报告
    ↓
输出：信念图 + FSM 历史 + 证据链
```

FSM 参数：
- `GapDelta: 0.3`（最高分和次高分差距阈值）
- `MinSupport: 2`（最少支持证据数）
- `MaxSteps: 3`（最大探索步数）
- `MinConfidence: 0.7`（最低置信度）

#### 工具分层（Progressive Disclosure）

```
TierAlwaysOn（始终可用）：
  get_current_time, query_internal_docs, 日志 MCP, 告警查询

TierSkillGate（技能门控）：
  Prometheus 指标发现/区间查询/即时查询
  → 只有当上下文引擎识别到"指标"意图时才暴露

TierOnDemand（按需加载）：
  mysql_crud → 只有配置了 allowed_tables 才加载

用户自定义 MCP 工具：
  → TierSkillGate 级别，domain=custom
```

### 面试话术

> **面试官**：Agent 怎么选择执行模式？
>
> **你**：我项目有两套引擎，通过配置选择：
>
> **Plan-Execute-Replan** 适合线性排查场景。Planner 生成步骤计划，
> Executor 逐步调用工具获取证据，Replanner 根据结果动态调整计划。
> 最多 5 轮循环，最终输出结构化诊断报告。
>
> **GoS Belief Engine** 适合复杂多假设场景。它维护一个信念图，
> 从症状文本提取多个假设节点，然后用 FSM（有限状态机）控制探索过程。
> 每轮选最高分假设，调度对应专家（网络/数据库/Linux SRE）并行探索，
> 用证据更新信念图。当置信度 >= 0.7 且有 >= 2 条支持证据时出报告。
>
> 两者的区别：Plan 是线性的，一步一步来；GoS 是图结构的，
> 可以同时探索多个假设方向。
>
> 工具选择上，我用了**三层渐进式暴露**：基础工具始终可用，
> 高级工具（如 Prometheus 查询）需要上下文引擎识别到相关意图才暴露，
> 敏感工具（如 MySQL）需要配置白名单。这样防止模型滥用工具。

---

## 9. 可观测性：指标、链路、日志

### 面试官问的

> "系统怎么监控？出了问题怎么排查？"

### 项目实现

#### Prometheus 指标（13 个）

```
请求层：
  http_requests_total{method, path, status}     ← 请求量
  http_request_duration_seconds{method, path}    ← 延迟分布

LLM 层：
  llm_calls_total{agent, model, status}          ← LLM 调用量
  llm_call_duration_seconds{agent, model}        ← LLM 延迟
  llm_tokens_total{agent, model, type}           ← Token 消耗

Agent 层：
  agent_dispatch_total{agent, status}            ← Agent 调度量
  agent_dispatch_duration_seconds{agent}         ← Agent 延迟

基础设施：
  circuit_breaker_state{name}                    ← 熔断器状态
  cache_hits_total{type} / cache_misses_total    ← 缓存命中率
  session_tokens_total{user_id}                  ← 用户 Token 消耗

异步任务：
  memory_extraction_events_total{mode, status}   ← 记忆提取事件
  chat_task_events_total{status}                 ← 聊天任务事件
```

#### OpenTelemetry 链路追踪

```
一个请求的完整链路：

[HTTP Request]
  └── [runtime.dispatch] agent.name=plan_execute
        ├── [llm.generate] model=deepseek-v4  ← Planner 调用
        ├── [tool.call] tool=prometheus_range  ← 工具调用
        ├── [llm.generate] model=deepseek-v4  ← Executor 调用
        └── [llm.generate] model=deepseek-v4  ← Replanner 调用

每个 span 记录：
  - trace_id（贯穿整个请求）
  - agent.name, llm.model
  - token_usage (prompt/completion)
  - error（如果有）
```

#### 工具健康报告

```go
// HealthCollector 聚合工具调用记录
type ToolHealthReport struct {
    ToolName      string
    TotalCalls    int
    SuccessRate   float64
    P50DurationMs int64
    P95DurationMs int64
    P99DurationMs int64
    CommonErrors  []string  // Top 3 常见错误
}
```

#### 验证层（防止 LLM 胡说）

```
Schema Gate → LLM Validator → Output Validator
     │              │               │
     │              │               └── 正则提取数值，对比工具结果
     │              └── 二次 LLM 调用检查遗漏和准确性
     └── 最小长度、矛盾检测、可执行建议检查
```

### 面试话术

> **面试官**：系统怎么监控？
>
> **你**：我用了三层可观测性。
>
> **第一层，Prometheus 指标**。13 个指标覆盖 HTTP 请求、LLM 调用、
> Agent 调度、熔断器状态、缓存命中率、Token 消耗、异步任务状态。
> 每个指标都有 agent、model、status 等标签，可以按维度聚合。
>
> **第二层，OpenTelemetry 链路追踪**。每个请求有完整链路：
> HTTP 入口 → Agent 调度 → LLM 调用 → 工具调用，全链路 trace_id
> 贯穿。Jaeger 采样率可配，支持 W3C TraceContext 传播。
>
> **第三层，工具健康报告**。全局 HealthCollector 聚合每个工具的
> 调用记录，定期输出 P50/P95/P99 延迟、成功率、Top 3 错误。
>
> 排查问题时：先看 Prometheus 面板定位异常指标，再用 trace_id
> 在 Jaeger 查链路，最后看工具健康报告定位具体是哪个工具出了问题。
>
> 另外，LLM 输出有三层验证：Schema Gate 检查格式和矛盾，
> LLM Validator 用二次调用检查遗漏和准确性，Output Validator
> 用正则对比数值。防止模型"幻觉"。

---

## 10. 表达技巧：STAR 法则与量化思维

### STAR 法则模板

```
Situation（背景）：
  "我们的 AIOps 系统需要从多个数据源（日志、指标、知识库）
   检索相关信息给 LLM 分析"

Task（任务）：
  "需要设计一个混合检索系统，兼顾语义理解和精确匹配"

Action（行动）：
  "我实现了 Dense + BM25 + RRF 融合的混合检索架构，
   并用消融实验对比了三种方案"

Result（结果）：
  "混合检索比纯向量检索召回率高 12 个百分点，
   MRR 从 0.48 提升到 0.62"
```

### 量化思维清单

```
项目里必须记住的数字：

RAG：
  - 知识库：~50 篇运维文档
  - 向量维度：2048（豆包 Embedding）
  - Milvus 索引：HNSW, M=16, efConstruction=200
  - 检索参数：Dense TopK=50, BM25 TopK=50, RRF K=60
  - 最终返回：Top 5
  - 召回率：Recall@10 = 78%, MRR = 0.62

限流：
  - 速率：20 请求/分钟
  - 突发容量：30 令牌
  - Redis 窗口：20 请求/60 秒

消息队列：
  - 最大重试：3 次
  - 重试间隔：2 秒（DLX TTL）
  - 去重 TTL：10 分钟
  - 去重容量：20000 条

Agent：
  - Plan-Execute-Replan 最大迭代：5 轮
  - GoS 最大步骤：3 步
  - GoS 最低置信度：0.7
  - GoS 最少支持证据：2 条

熔断器：
  - 触发阈值：5 次连续失败
  - 恢复超时：30 秒
  - 半开成功数：2 次

信号量：
  - LLM 最大并发：10
  - 等待超时：5 秒
```

### 回答结构模板

```
面试官问技术问题时，按这个顺序回答：

1. 结论先行（一句话回答）
   "我用的是令牌桶 + Redis 滑动窗口的双后端策略"

2. 原理简述（为什么这么做）
   "令牌桶允许短时突发，滑动窗口更精确但有网络开销"

3. 项目细节（怎么实现的）
   "Redis 可用时用 Lua 脚本做滑动窗口，不可用时降级到内存令牌桶"

4. 量化数据（效果如何）
   "限流 20/min，突发容量 30，降级切换在 PING 失败后自动完成"

5. 坦诚边界（不会的直说）
   "单实例部署所以没用分布式锁，如果需要我会用 Redlock"
```

---

## 附录：面试常见追问清单

| 问题 | 简短回答 |
|------|---------|
| 为什么用 BM25 不用 bleve？ | 自研 200 行，无 CGO 依赖，运维文档规模（~50 篇）不需要外部库 |
| 为什么用 Milvus 不用 ES？ | 已有向量检索需求，Milvus 原生支持 HNSW，不需要 ES 的全文搜索功能 |
| RAG 召回率怎么提升？ | 优化分词（CJK bigram）、调整 RRF K 值、增加 metadata boost 权重 |
| LLM 输出不稳定怎么办？ | 三层验证（Schema Gate + LLM Validator + Output Validator）+ 温度设为 0 |
| 服务挂了任务怎么办？ | RabbitMQ 持久化 + FileLedger 状态恢复 + Redis 进度缓存 |
| 怎么防止 LLM 幻觉？ | Agent 合约（Must/MustNot）+ 工具证据强制引用 + 二次验证 |
| 多实例部署怎么做？ | Redis 共享状态 + RabbitMQ 分布式消息 + Milvus 向量库 |
| Token 消耗怎么控制？ | Redis 审计（每日限额）+ Token Budget 分配（20% 系统 / 10% 记忆 / 40% 历史） |
