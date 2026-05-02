# 第 12 章：Skills 渐进披露 + MCP 工具集成

> 当前统一口径（2026-05）
> - Chat 主链路已经收敛到 `ReAct Agent + ProgressiveDisclosure`
> - 文中涉及 `Supervisor / skillspecialists / specialist 内 skill 化` 的部分，主要用于解释历史设计动机和演进路径，不再代表当前聊天主链路结构。

> **本章目标**：理解 OpsCaption 的 Skills 架构设计和 MCP 工具集成方案。能向面试官解释"skill 和 tool 的区别"、"Progressive Disclosure 的动机"、"MCP 协议的价值"。

---

## 1. 白话理解：从 Agent + Tools 到 Agent + Skills + Tools

### 1.1 一句话解释

**Skills 改造 = 把 agent 的"路由逻辑"和"执行逻辑"拆开。** Agent 负责选 skill，Skill 负责执行套路，Tool 只保留原子能力。

### 1.2 一张图看懂三层

```
用户输入: "告警触发：checkoutservice CPU > 90%"
    │
    ▼
┌─────────────────────────────────────────────┐
│ Supervisor                                   │
│   orchestrate(task)                          │
└───────────────────┬─────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Triage                                       │
│   识别任务类型 → 路由到对应 Specialist          │
└───────────────────┬─────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Metrics Specialist (Agent)                   │
│                                              │
│   职责：接任务 → 选 Skill → 打 Trace           │
│   ┌───────────────────────────────┐          │
│   │       Skill Registry          │          │
│   │                               │          │
│   │  ┌─────────────────────────┐  │          │
│   │  │ metrics_alert_triage     │  │  命中   │
│   │  │ Match: 含"告警/severity" │──┼──────▶ │
│   │  │ Run: 调Prometheus+组装   │  │          │
│   │  └─────────────────────────┘  │          │
│   │                               │          │
│   │  ┌─────────────────────────┐  │          │
│   │  │ metrics_incident_snapshot│  │  回退   │
│   │  │ 默认 skill（兜底）       │  │         │
│   │  └─────────────────────────┘  │          │
│   └───────────────────────────────┘          │
└───────────────────┬─────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│ Tools (原子能力)                              │
│   ├─ Prometheus 指标查询                      │
│   ├─ Milvus 知识库检索                        │
│   ├─ MCP 日志查询 ────→ 远程 K8s 日志         │
│   └─ MySQL CRUD (OnDemand)                   │
└─────────────────────────────────────────────┘
```

### 1.3 三层关系速记

| 层 | 是什么 | 做什么 | 类比 |
|----|--------|--------|------|
| **Agent** | 执行者 | 接任务、选 skill、打 trace | 项目经理——分配任务 |
| **Skill** | 任务套路 | 匹配任务、调用 tool、组装结果 | 方案模板——决定了怎么做 |
| **Tool** | 原子能力 | 查数据、执行动作 | 工具箱——干活的 |

> **最短面试回答：** "tool 是动作，skill 是方法，agent 是执行者。"

---

## 2. Skill 架构：接口 + Registry + Metadata

### 2.1 Skill 接口

```go
// registry.go
type Skill interface {
    Name() string                                        // 唯一标识：metrics_alert_triage
    Description() string                                 // 描述：告警分类与分析
    Match(task *protocol.TaskEnvelope) bool              // 判定：这个任务该我处理吗？
    Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) // 执行
}
```

四个方法组成了一个完整的 skill 生命周期：**标识 → 描述 → 匹配 → 执行**。

### 2.2 Registry：选择 + 执行 + 打标

```go
type Registry struct {
    domain string    // 域：knowledge / metrics / logs
    skills []Skill   // 该域下的所有 skill
}

// Resolve: 按顺序匹配，第一个命中的就是选中的 skill
func (r *Registry) Resolve(task *protocol.TaskEnvelope) (Skill, error) {
    for _, skill := range r.skills {
        if skill.Match(task) {
            return skill, nil
        }
    }
    return r.skills[0], nil  // 没命中 → 回退到第一个 skill（兜底）
}

// Execute: Resolve → Run → AttachMetadata
func (r *Registry) Execute(ctx context.Context, task *protocol.TaskEnvelope) (*Execution, error) {
    skill, _ := r.Resolve(task)
    result, _ := skill.Run(ctx, task)
    result = AttachMetadata(result, r.domain, skill)  // 把 skill 信息写入 metadata
    return &Execution{Skill: skill, Result: result}, nil
}
```

**关键设计：Resolve 的兜底机制。** 不是"匹配不到就报错"，而是"回退到第一个 skill"。这让系统在未覆盖的 query 上也能工作——不会因为少注册一个 skill 就阻断主链路。

### 2.3 Metadata：让 skill 选择结果可观察

```go
func AttachMetadata(result *protocol.TaskResult, domain string, skill Skill) *protocol.TaskResult {
    result.Metadata["skill_domain"]       = domain              // "metrics"
    result.Metadata["skill_name"]         = skill.Name()        // "metrics_alert_triage"
    result.Metadata["skill_description"]  = skill.Description()  // "告警分类与分析"
    return result
}
```

运行时 trace 里会看到：

```
[metrics] selected skill=metrics_alert_triage
```

这让你能回答："这次请求为什么选了 metrics_alert_triage 而不是 metrics_incident_snapshot？"

---

## 3. 已落地的 Skills

### 3.1 Knowledge（知识域）

| Skill | 适用场景 | 匹配条件 |
|-------|---------|---------|
| `knowledge_release_sop` | 发布 SOP、发布流程、上线规范 | query 含 "发布/上线/deploy/release" 等关键词 |
| `knowledge_rollback_runbook` | 回滚手册、回滚操作指南 | query 含 "回滚/rollback/revert" 等关键词 |
| `knowledge_service_error_code_lookup` | 服务错误码查询、异常码解释 | query 含 "错误码/error code/异常码" 等关键词 |
| `knowledge_sop_lookup` | SOP、runbook、文档、操作步骤类问题 | query 含 "sop/runbook/文档/手册/操作步骤" 等关键词 |
| `knowledge_incident_guidance` | 默认回退，泛化故障分析 | 未命中其他 skill 时兜底 |

### 3.2 Metrics（指标域）

| Skill | 适用场景 | 匹配条件 |
|-------|---------|---------|
| `metrics_release_guard` | 发布守卫、发布监控 | query 含 "发布/deploy/上线/rollback" 等关键词 |
| `metrics_capacity_snapshot` | 容量快照、资源使用情况 | query 含 "容量/capacity/资源/水位" 等关键词 |
| `metrics_alert_triage` | 告警分类与分析 | query 含 "告警/alert/firing/critical/severity" |
| `metrics_incident_snapshot` | 默认回退，健康快照和宽泛问题 | 未命中其他 skill 时兜底 |

### 3.3 Logs（日志域）

| Skill | 适用场景 | 匹配条件 |
|-------|---------|---------|
| `logs_service_offline_panic_trace` | 服务下线 panic 排查 | query 含 "offline/panic/下线/中断" 等关键词 |
| `logs_api_failure_rate_investigation` | API 失败率分析 | query 含 "api/失败率/failure rate/调用失败" 等关键词 |
| `logs_payment_timeout_trace` | 支付超时排查 | query 含 "payment/timeout/支付/超时" 等关键词 |
| `logs_auth_failure_trace` | 认证失败排查 | query 含 "auth/认证/登录失败/401/403" 等关键词 |
| `logs_evidence_extract` | 通用报错、异常、panic、堆栈提取 | query 含 "error/exception/panic/crash" |
| `logs_raw_review` | 默认回退，原始日志片段 | 未命中其他 skill 时兜底 |

---

## 4. Progressive Disclosure（渐进披露）

### 4.1 问题：为什么工具不能一刀切暴露？

运维 Agent 有不同的工具——查询时间、查询知识库、查 Prometheus、查 K8s 日志、MySQL CRUD。如果**所有工具始终暴露给 LLM**：

```
❌ Prompt 太长 → Token 浪费 → 成本增加
❌ LLM 可能选错工具 → "查指标"却调了"查日志"
❌ 低风险查询暴露高风险工具 → SQL 注入面扩大
```

### 4.2 三层分级

```go
// progressive_disclosure.go
type ToolTier int

const (
    TierAlwaysOn  ToolTier = 0  // 始终暴露：时间、知识库
    TierSkillGate ToolTier = 1  // Skill 通过后才暴露：Prometheus、MCP 日志
    TierOnDemand  ToolTier = 2  // 按需扩展：MySQL CRUD
)

type TieredTool struct {
    Tool    tool.BaseTool
    Tier    ToolTier
    Domains []string  // 限定的领域：["metrics"] / ["logs"] / ["knowledge"]
}
```

| Tier | 工具 | 暴露条件 | 理由 |
|------|------|---------|------|
| **AlwaysOn** | 时间、知识库查询 | 始终可用 | 这些工具无风险、高频使用、不需要领域判断 |
| **SkillGate** | Prometheus 指标、MCP 日志 | 对应 skill 命中时 | 需要 skill 判定"当前任务真的需要查指标/日志"才暴露 |
| **OnDemand** | MySQL CRUD | 配置启用 + 领域匹配 | 最高风险——SQL 注入风险，只有在 `allowed_tables` 非空时才注册 |

### 4.3 工作流程

```go
// progressive_disclosure.go - Disclose
func (pd *ProgressiveDisclosure) Disclose(query string, selectedSkillIDs []string) DisclosureResult {
    result := DisclosureResult{
        DisclosedTier: make(map[ToolTier]int),
    }

    // 1. 如果前端用户手动选择了 skill，解析这些 selected skills
    selectedSkills := pd.ResolveSelectedSkills(selectedSkillIDs)
    // 2. 合并：query 自动匹配的 domains + 用户手动选择的 domains
    matchedDomains := mergeDomains(pd.matchDomains(query), domainsFromSelectedSkills(selectedSkills))
    result.SelectedSkills = selectedSkills
    result.MatchedDomains = matchedDomains

    // 3. 按 Tier 筛选工具
    for _, tt := range pd.tools {
        switch tt.Tier {
        case TierAlwaysOn:
            result.Tools = append(result.Tools, tt.Tool)  // 始终加入

        case TierSkillGate:
            if domainOverlap(matchedDomains, tt.Domains) {
                result.Tools = append(result.Tools, tt.Tool)  // skill 匹配才加入
            }
        }
    }

    return result
}
```

**示例**：

```
用户: "现在几点？" 
  → matched domains: []（不匹配任何领域）
  → 暴露工具: [时间, 知识库] （只有 AlwaysOn）

用户: "checkoutservice CPU 告警了怎么办"
  → matched domains: ["metrics"]
  → 暴露工具: [时间, 知识库, Prometheus] （AlwaysOn + metrics 域的 SkillGate）
```

### 4.4 SkillPanel：前端 Skill 可视化开关

Progressive Disclosure 不只是后端逻辑——前端有 **SkillPanel** 组件让用户主动参与工具选择。

```
┌─────────────────────────────────────┐
│  SkillPanel（前端侧栏卡片）          │
│                                     │
│  [✓] Metrics  [✓] Logs  [ ] MySQL  │
│   ├── alert_triage   ├── evidence   │
│   └── incident       └── raw_review │
│                                     │
│  [✓] Knowledge                      │
│   ├── sop_lookup                     │
│   └── incident_guidance             │
└─────────────────────────────────────┘
  ↓ 用户勾选 / 取消勾选
  ↓ selectedSkillIDs 传给后端 Disclose()
```

**关键设计：双通道匹配。**  后端的 `Disclose()` 接收两个来源的 domain 信号：
1. **query 自动匹配**：query 含 "告警" → metrics domain
2. **用户手动选择**：用户勾选了 Logs skill → logs domain

两个来源通过 `mergeDomains()` 合并。这意味着：即使 query 里没有日志相关关键词，但用户主动勾选了 logs skill，日志工具（MCP 日志查询）也会被暴露给 LLM。

**这解决了什么问题？** 运维有时知道问题和日志有关，但 query 里没有提到日志。手动选择 skill 让运维可以把领域知识注入到 Agent 的工具选择中——这是 "人机协作"，不是完全自动化。

---

## 5. MCP 集成：日志查询工具

### 5.1 MCP 是什么

**MCP（Model Context Protocol）** = Anthropic 提出的 LLM 工具调用标准协议。类似于 LSP（Language Server Protocol）之于 IDE——定义了一套统一的标准，让 LLM 可以通过同一套协议对接任何工具服务器。

### 5.2 OpsCaption 的 MCP 日志工具

```go
// tools/query_log.go
func GetLogMcpTool() ([]tool.BaseTool, error) {
    mcpURL, _ := g.Cfg().Get(ctx, "mcp.log_url")  // 从 config 读取 MCP 服务地址
    cli, _ := client.NewSSEMCPClient(mcpURL)       // 建立 SSE 连接
    cli.Start(ctx)                                   // 握手初始化
    
    initRequest := mcp.InitializeRequest{}
    initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
    initRequest.Params.ClientInfo = mcp.Implementation{
        Name:    "superbizagent-client",
        Version: "1.0.0",
    }
    cli.Initialize(ctx, initRequest)  // MCP 协议初始化
    
    // 用 Eino 的 MCP 适配器包装成 Eino Tool
    mcpTools, _ := e_mcp.GetTools(ctx, &e_mcp.Config{Cli: cli})
    return mcpTools, nil
}
```

### 5.3 MCP 服务端：K8s 日志查询

```python
# deploy/local-k8s-log-mcp.py
# 一个标准 MCP SSE Server，提供 query_freeexchanged_k8s_logs 工具

def tool_schema():
    return {
        "name": "query_freeexchanged_k8s_logs",
        "description": "Query recent FreeExchanged k3s pod logs on this server.",
        "inputSchema": {
            "properties": {
                "query":  {"type": "string"},  # 自然语言 query
                "limit":  {"type": "integer"}, # 最大返回条数
                "focus":  {"type": "string"},  # 可选的排查重点
            },
        },
    }
```

**完整交互流程**：

```
OpsCaption Agent                 MCP Server (K8s 节点)
      │                                │
      │── SSE connect ────────────────▶│  建立长连接
      │◀─ endpoint URL ────────────────│
      │                                │
      │── initialize ─────────────────▶│  协议握手
      │◀─ capabilities: {tools} ──────│
      │                                │
      │── tools/list ─────────────────▶│  Agent 想知道有什么工具
      │◀─ [query_freeexchanged_k8s_logs]
      │                                │
      │── tools/call ─────────────────▶│  LLM 决定调日志查询
      │   {query: "payment error"}     │  → kubectl logs --since=2h --tail=160
      │◀─ {logs: [...], success: true} │  返回结构化日志
```

### 5.4 分层集成：MCP 工具的 Tier

```go
// tools/tiered_tools.go
func BuildTieredTools() []skills.TieredTool {
    // MCP 日志工具 → TierSkillGate，只暴露给 logs 域
    mcpTools, err := GetLogMcpTool()
    for _, mt := range mcpTools {
        tiered = append(tiered, skills.TieredTool{
            Tool:    mt,
            Tier:    skills.TierSkillGate,     // ← 需要 logs skill 命中才暴露
            Domains: []string{"logs"},          // ← 只在 logs 域生效
        })
    }
    
    // Prometheus 指标 → TierSkillGate，只暴露给 metrics 域
    tiered = append(tiered, skills.TieredTool{
        Tool:    NewPrometheusAlertsQueryTool(),
        Tier:    skills.TierSkillGate,
        Domains: []string{"metrics"},
    })
    
    // MySQL CRUD → TierOnDemand，需配置启用 + 领域匹配
    if MySQLToolEnabled() {
        tiered = append(tiered, skills.TieredTool{
            Tool:    NewMysqlCrudTool(),
            Tier:    skills.TierOnDemand,
            Domains: []string{"logs", "metrics", "knowledge"},
        })
    }
}
```

### 5.5 为什么用 MCP 而不是直接调 K8s API

| 方案 | 问题 |
|------|------|
| **直接调 K8s API** | 需要在 Agent 进程里引入 k8s client-go → 增加依赖、内存和编译复杂度；Agent 进程获得不必要的 k8s 管理权限——安全面扩大 |
| **MCP 协议** | MCP Server 独立部署在 K8s 节点上，Agent 只能通过标准协议调用，不持有 k8s 凭证；日志查询、格式化、过滤都在 Server 端完成，Agent 只拿到结构化结果 |

> **MCP 的核心价值：** "工具执行在谁的上下文里？" 直接调 API = 工具执行在 Agent 进程里（权限大、风险大）。MCP = 工具执行在独立 Server 里（权限隔离、标准化协议）。

---

## 6. Skills 改造的工程策略：平行迁移

### 6.1 为什么不全量重写

```
旧方式（一把梭重写）：
  删除 old/specialists → 新建 skillspecialists → 改所有引用 → 💥 炸了就全跪

OpsCaption 的方式（平行迁移）：
  保留 old/specialists/ → 新建 skillspecialists/ → 只改注册点 → 渐进切流量
```

```go
// 平行迁移：只改 supervisor 和 service 的两个注册点
// supervisor.go  → 切到 skillspecialists
// ai_ops_service.go → 切到 skillspecialists

// 出问题回滚：把注册点切回 old/specialists，不需要恢复任何文件
```

### 6.2 有什么代价

| 方面 | 代价 | 收益 |
|------|------|------|
| **代码量** | 多了一套 skillspecialists 包，结构更复杂 | 后续加 skill 不再膨胀 Handle() |
| **理解成本** | 新人需要先读懂 skill 接口 + registry | 读懂后面加 skill 很容易——实现 4 个方法就行 |
| **运行期** | 多了一层 Resolve 调用 | 可观察性（能知道选了哪个 skill）、可扩展性、可测试性 |

---

## 7. 面试问答

### Q1: "你的项目里 skill 和 tool 有什么区别？"

> **tool 是动作，skill 是方法，agent 是执行者。**
>
> tool 是原子能力——查 Prometheus、查知识库、查日志 MCP。它不知道"什么时候该用"，只知道"怎么查"。
>
> skill 是任务套路——`metrics_alert_triage` 这个 skill 知道：当任务描述里出现告警/firing/severity 关键词时，我应该被选中。被选中后，我决定调用哪个 Prometheus tool、返回结果怎么组装。
>
> agent 是执行者——接任务 → 调用 Registry.Resolve() 选 skill → 让 skill.Run() 执行 → 把结果打上 skill_name 的 metadata。
>
> 举个例子：查询"现在是几点？"只走 tool（时间工具），不需要 skill 参与。但"checkoutservice CPU 告警"则需要 `metrics_alert_triage` 这个 skill 来决定查哪些指标、怎么组织回答。

### Q2: "Progressive Disclosure 解决什么问题？"

> 解决三个问题：
>
> **第一，Token 浪费。** 如果所有工具始终暴露给 LLM，每次调用的 System Prompt 都要列出所有工具的 schema——就算这次只查时间，也要把 Prometheus 和 MySQL 的工具描述列出来。Progressive Disclosure 只暴露 AlwaysOn + 当前 skill 匹配到的域的工具。
>
> **第二，LLM 选错工具。** 给 LLM 20 个工具，总有一个被误选。限定到当前场景真正需要的 3-5 个，准确率高得多。
>
> **第三，安全面最小化。** MySQL CRUD 是 TierOnDemand——不启用时根本不给 LLM 看到，即使启用也只暴露给特定 domain。MCP 日志工具是 TierSkillGate——只有 logs skill 命中时才暴露，平时 Agent 看不见。

### Q3: "MCP 和直接调 API 的区别是什么？"

> MCP 的核心价值是**执行上下文隔离**和**标准化协议**。
>
> 直接调 K8s API：Agent 进程需要 k8s client-go + k8s 凭证——权限大、依赖重。MCP：工具 Server 独立部署在 K8s 节点上，Agent 通过标准 MCP 协议发请求——Agent 不持有 k8s 凭证，不引入 k8s 依赖。
>
> 标准化协议的好处：换一个日志源（比如从 k3s 换成 EKS、从 kubectl 换成 Loki）只需要换 MCP Server 的实现，Agent 端零改动。这跟 LSP 之于 IDE 的逻辑一样——IDE 不需要知道每个语言的编译器细节，只要对接 LSP 协议。

### Q4: "你的 Skills 改造是平行迁移——为什么不全量重写？"

> 平行迁移是刻意的工程选择——先把旧的留着，新建一套 skillspecialists，只改注册点。三个好处：
>
> 1. **风险可控**——出问题回滚只需要把注册点切回旧实现，不用恢复任何文件
> 2. **可对照**——旧实现和新实现可以同时跑单测，验证行为一致性
> 3. **渐进式**——可以先让一小部分流量走新 skill specialist，稳定后再全量切换
>
> 这是一种增量重构策略——"先建新路，再封旧路"，比"炸了旧路建新路"安全得多。

### Q5: "SkillPanel 的双通道匹配有什么好处？"

> SkillPanel 让用户手动选择 skill 组合——query 自动匹配是"AI 判断"，用户手动选择是"领域知识注入"。两者合并后才是完整的工具暴露决策。
>
> 举个例子：运维怀疑 checkoutservice 的 CPU 问题是日志导致的，但 query 只写了"checkoutservice CPU 告警"。query 匹配只命中 metrics domain，但运维手动勾选了 logs skill，结果日志 MCP 工具也被暴露给 LLM。**这种人机协作比完全自动化更可靠——在 Agent 还不成熟的阶段，让运维有"手动扩大搜索范围"的能力。**

---

## 8. 自测

### 问题 1

skill 和 tool 的区别是什么？Agent 在三层中扮演什么角色？

<details>
<summary>点击查看答案</summary>

- **Tool**：原子能力——查数据、执行动作。不知道"什么时候该用"。
- **Skill**：任务套路——匹配任务、组合 tool、组装结果。决定"什么时候用什么工具"。
- **Agent**：执行者——接任务、选 skill、打 trace。是三层中的协调者。

最短回答：**tool 是动作，skill 是方法，agent 是执行者。**
</details>

### 问题 2

Progressive Disclosure 的三层分级（AlwaysOn / SkillGate / OnDemand）各自适用什么场景？

<details>
<summary>点击查看答案</summary>

- **AlwaysOn**：无风险、高频、不需要领域判断的工具——时间查询、知识库检索
- **SkillGate**：需要 skill 判定"当前任务真的需要"才暴露——Prometheus 指标、MCP 日志查询
- **OnDemand**：最高风险/最高成本——需要配置启用 + 领域匹配才暴露。MySQL CRUD 只有在 `allowed_tables` 非空时才注册
</details>

### 问题 3

MCP 协议和直接调 K8s API 相比，核心差异是什么？

<details>
<summary>点击查看答案</summary>

1. **执行上下文隔离**：直接调 API = Agent 持有 k8s 凭证；MCP = Server 独立部署，Agent 只通过标准协议调用
2. **标准化**：换日志源只需换 MCP Server 实现，Agent 端零改动
3. **安全面**：MCP Server 可以部署在目标节点上，Agent 不直接接触基础设施
</details>

---

> 📌 **面试时怎么自然引出 Skills + MCP：**
>
> 当面试官问"你的 Agent 和普通的 API 调用有什么区别"时，说：
>
> "我们做了 skills 改造和渐进披露。不是所有工具始终暴露给 LLM——工具分 AlwaysOn / SkillGate / OnDemand 三级。Skill 命中 metrics 域才暴露 Prometheus 工具，命中 logs 域才暴露 MCP 日志工具。MCP 服务器独立部署在 K8s 节点上，Agent 通过标准协议调用，不持有基础设施凭证。"
