# Context Engineering 完成度矩阵

日期：2026-04-05

## 1. 结论

当前项目的上下文工程 **没有全部完成**，但 **P0/P1 的最小可运行版本已经完成**。

更准确的状态应表述为：

> 项目已经具备一套真实接入主链路的模块化上下文工程 MVP，覆盖 `history / memory / docs / tool outputs` 四类主来源；但上下文评测、信任治理、安全防护和更深的检索优化仍未完成。

这意味着：

- 不能说“上下文工程还只是设计稿”
- 也不能说“上下文工程已经全部做完”

最合适的描述是：

- **主体已落地**
- **治理版未完成**

---

## 2. 完成度分层定义

为避免“做了一部分”和“全部完成”混淆，这里使用四档判断：

### A. 已完成

定义：

- 已进入主链路
- 有明确代码落点
- 有自动化测试验证
- 已能改变系统运行行为

### B. 已有 MVP

定义：

- 已经有代码和运行行为
- 但能力还偏保守或规则驱动
- 后续仍需增强才能称为成熟方案

### C. 未完成

定义：

- 设计上有规划
- 代码中尚未形成完整能力
- 不能视作当前已交付

### D. 当前阶段不建议做

定义：

- 不是做不到
- 而是现在继续做会明显增加复杂度，且不适合当前项目目标

---

## 3. 总体完成度判断

| 模块 | 当前状态 | 判断 |
| --- | --- | --- |
| `ContextProfile` | 已接入主链路 | 已完成 |
| `ContextBudget` | 已接入 history/memory/docs/tool outputs | 已完成 |
| `ContextAssembler` | 已装配四类上下文源 | 已完成 |
| `ContextTrace` | 已通过 `detail` 暴露 | 已完成 |
| Long-term memory 写入校验 | 已有候选与过滤 | 已有 MVP |
| `history` 上下文治理 | 已预算化并显式裁剪 | 已完成 |
| `memory` 上下文治理 | 已检索、裁剪、staged 注入 | 已完成 |
| `docs` 上下文治理 | 已进入 assembler | 已完成 |
| `tool outputs` 上下文治理 | 已在 reporter 聚合阶段接入 | 已完成 |
| context replay / ablation eval | 尚未建立 | 未完成 |
| source trust / poisoning / redaction | 仅有字段与设计，没有完整治理 | 未完成 |
| retrieval rerank / hybrid retrieval | 尚未实现 | 未完成 |
| context query / inspection API | 尚未实现 | 未完成 |
| model-driven context policy | 尚未实现 | 当前阶段不建议做 |
| 复杂 selector DSL | 尚未实现 | 当前阶段不建议做 |

---

## 4. 已完成部分

## 4.1 统一上下文对象

代码：

- [types.go](/Users/agiuser/Agent/OnCallAI/internal/ai/contextengine/types.go)

已完成内容：

- `ContextRequest`
- `ContextProfile`
- `ContextBudget`
- `ContextItem`
- `ContextPackage`
- `ContextAssemblyTrace`

为什么算完成：

- 已不是设计名词
- 已有真实代码
- 已被 Chat / AI Ops / Reporter 使用

## 4.2 统一上下文装配

代码：

- [assembler.go](/Users/agiuser/Agent/OnCallAI/internal/ai/contextengine/assembler.go)

已完成内容：

- history 选择
- memory 选择
- docs 选择
- tool results 选择
- staged memory 注入
- dropped reason 记录

为什么算完成：

- 已真正进入主链路
- 已能改变实际注入给模型的上下文

## 4.3 Chat 主链路接入

代码：

- [chat_v1_chat.go](/Users/agiuser/Agent/OnCallAI/internal/controller/chat/chat_v1_chat.go)
- [chat_v1_chat_stream.go](/Users/agiuser/Agent/OnCallAI/internal/controller/chat/chat_v1_chat_stream.go)

已完成内容：

- legacy chat 不再直接走黑盒 `BuildEnrichedContext`
- 改成通过 `MemoryService.BuildChatPackage(...)`
- `detail` 中可见 context trace

为什么算完成：

- legacy chat 现在已经使用统一上下文层

## 4.4 AI Ops / Chat Multi-Agent 接入

代码：

- [ai_ops_service.go](/Users/agiuser/Agent/OnCallAI/internal/ai/service/ai_ops_service.go)
- [chat_multi_agent.go](/Users/agiuser/Agent/OnCallAI/internal/ai/service/chat_multi_agent.go)

已完成内容：

- memory context 走统一 `BuildContextPlan(...)`
- `context_detail` 合并进入主链路 detail

为什么算完成：

- Multi-Agent 链路现在也有上下文控制面的参与

## 4.5 文档检索进入统一上下文层

代码：

- [documents.go](/Users/agiuser/Agent/OnCallAI/internal/ai/contextengine/documents.go)
- [orchestration.go](/Users/agiuser/Agent/OnCallAI/internal/ai/agent/chat_pipeline/orchestration.go)

已完成内容：

- Chat 的 documents 不再留在 graph 内部 retriever node
- 现在由 `ContextAssembler` 统一检索与裁剪

为什么算完成：

- docs 已从黑盒检索变成上下文工程的一部分

## 4.6 Tool Outputs 进入统一上下文层

代码：

- [tool_items.go](/Users/agiuser/Agent/OnCallAI/internal/ai/contextengine/tool_items.go)
- [reporter.go](/Users/agiuser/Agent/OnCallAI/internal/ai/agent/reporter/reporter.go)

已完成内容：

- specialist evidence / summary 转 `ContextItem`
- reporter 聚合前先走统一上下文选择
- `tool_results selected=...` 写入 trace/detail

为什么算完成：

- 上下文层已经覆盖到第四类主来源

---

## 5. 已有 MVP 的部分

## 5.1 Long-term memory 写入校验

代码：

- [extraction.go](/Users/agiuser/Agent/OnCallAI/utility/mem/extraction.go)

当前状态：

- 已有：
  - candidate extraction
  - validation
  - drop report
- 当前仍然是规则驱动：
  - 长度边界
  - boilerplate
  - 代码块
  - 行数限制

为什么只算 MVP：

- 没有更强的语义分类
- 没有 quarantine / review / consolidation
- 没有 write-time trust scoring

结论：

- **能用**
- **但还不算成熟治理系统**

---

## 6. 未完成部分

## 6.1 Context Replay / Ablation Eval

状态：

- 设计方向明确
- 尚未实现

缺失影响：

- 现在还不能系统回答：
  - memory 是否真的提升了效果
  - docs 是否在某类 case 中产生噪声
  - tool outputs 是否真的改善了最终回答

判断：

- **未完成**
- 这是后续最值得继续做的部分

## 6.2 Source Trust / Poisoning / Redaction

状态：

- `ContextItem` 里已有：
  - `trust_level`
  - `safety_label`
  - `update_policy`
- 但它们还 mostly 是占位和轻量语义

缺失影响：

- 还没有真正的 trust policy
- 还没有 memory poisoning 防护闭环
- 还没有 redaction pipeline

判断：

- **未完成**

## 6.3 Retrieval Rerank / Hybrid Retrieval

状态：

- docs 已统一进入上下文层
- 但检索质量增强还没做

缺失影响：

- 当前 documents 仍主要依赖 retriever 原始结果
- 上下文工程现在更强在“治理”，还不强在“检索质量提升”

判断：

- **未完成**

## 6.4 Context Inspection API

状态：

- 目前通过 `detail` 暴露
- 还没有独立的 context query / inspection API

缺失影响：

- 复盘可做，但还不够精细
- 更适合开发态，不算完整治理态

判断：

- **未完成**

---

## 7. 当前阶段不建议做的部分

## 7.1 Model-driven Context Policy

为什么不建议现在做：

- 复杂度高
- 结果不稳定
- 不适合当前项目的上线与面试目标

## 7.2 复杂 Selector DSL / Context Service

为什么不建议现在做：

- 会把项目快速推向平台工程复杂度
- 与当前校招展示目标不匹配

结论：

- 不是不能做
- 而是**当前阶段故意不做**

---

## 8. 当前完成度矩阵

| 类别 | 数量 | 当前判断 |
| --- | --- | --- |
| 已完成 | 6 | 主体已落地 |
| 已有 MVP | 1 | 能用但不成熟 |
| 未完成 | 4 | 后续值得做 |
| 当前阶段不建议做 | 2 | 刻意延后 |

总体结论：

**上下文工程的主体已经完成，但不是治理完全体。**

更适合在面试或上线复盘里这样表述：

> 我已经把上下文工程从设计稿推进到真实代码，实现了统一的 `ContextProfile / ContextAssembler / ContextTrace`，并让 `history / memory / docs / tool outputs` 四类主来源进入统一治理；但 replay eval、trust policy 和更深的检索增强还在后续阶段。

---

## 9. 建议对外表述口径

### 9.1 不建议说

- “上下文工程已经全部做完了”
- “只是做了点 prompt 优化”

### 9.2 建议说

- “上下文工程主干已经落地”
- “已经形成可运行的模块化上下文控制面”
- “当前完成的是 P0/P1，治理增强和评测还在后续阶段”

---

## 10. 下一步最推荐做什么

如果继续推进，优先级建议：

1. `Context Replay / Ablation Eval`
2. `Source Trust / Poisoning / Redaction`
3. `Retrieval Rerank / Quality Eval`

其中最值钱的是第一项，因为它能回答：

- 哪类上下文真的有收益
- 哪类上下文只是增加了复杂度
