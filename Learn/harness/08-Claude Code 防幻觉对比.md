# Claude Code 防幻觉机制调研

> 调研 Claude Code（Anthropic 的 agentic coding 工具）如何防止大模型产生幻觉
> 基于 Anthropic 公开发表的研究论文、Claude Code 文档及架构分析

---

## 1. Claude Code 概述

Claude Code 是 Anthropic 的 agentic coding 工具，运行在终端/IDE 中，能读取代码库、编辑文件、执行命令、管理 git 工作流。它天然面临高准确性要求——代码生成错误会直接导致构建失败或 bug。

---

## 2. Claude Code 的防幻觉多层架构

```
用户
 │
 ├─ 第一层：Constitutional AI（模型层）
 ├─ 第二层：File-First 验证（地面实况层）
 ├─ 第三层：工具输出优先（真实数据优先于模型记忆）
 ├─ 第四层：Command Sandboxing（执行需审批）
 ├─ 第五层：Structured Thinking（分步推理可见）
 ├─ 第六层：Refusal Over Guessing（宁拒不知，不猜）
 ├─ 第七层：Project Rules 文件（仓库级约束）
 └─ 第八层：User Approval Gate（人的最终判断）
```

---

## 3. 第一层：Constitutional AI

**这是 Anthropic 最独特的防幻觉基础。**

### 3.1 训练方式

不同于 OpenAI 的 RLHF（人类反馈强化学习），Anthropic 使用 Constitutional AI：

1. **监督学习阶段**：用宪法原则（Constitution）指导模型生成无害回应
2. **RL 阶段**：用 AI 反馈（而非人类反馈）进行强化学习，AI 根据宪法原则评判模型输出

### 3.2 宪法原则中与防幻觉直接相关的条款

从 Anthropic 公开的 Claude Constitution 中提取：

```
- 选择最不可能被认为有害或不实的回应
- 如果无法验证某个事实，请说明这一点
- 不要假装对不存在的信息有把握
- 承认不确定性，而不是编造细节
- 如果被要求完成需要访问系统、文件或数据的任务，而这些能力模型不具备，
  模型应当明确说明，而不是假装执行了这些操作
```

### 3.3 实际效果

Constitutional AI 让 Claude 在**模型层面**就比其他 LLM 更倾向于说"我不知道"。这是防幻觉的根基——不是靠 prompt 约束，而是训练时就注入了"诚实优先于讨好"的价值观。

---

## 4. 第二层：File-First 验证

**Claude Code 最大的防幻觉机制：读真实文件，不靠记忆。**

### 4.1 强制文件读取

当用户问"这个项目怎么工作"时，Claude Code **不会**凭从训练数据中学到的开源项目知识来回答。而是：

```
1. 列出目录结构（ls / search_files）
2. 读取入口文件（read_file）
3. 追踪 import / require 依赖链
4. 基于实际代码回答
```

### 4.2 与 ChatGPT/通用 Claude 的对比

| | 通用 ChatGPT/Claude | Claude Code |
|---|---|---|
| 问"这个 React 项目的路由是什么" | 凭训练数据猜 | 读 package.json + App.tsx |
| 改代码时 | 凭记忆生成 | 先 read_file，再 patch |
| 调试时 | 推断可能原因 | 读错误栈 + 读相关文件 |

### 4.3 设计原理

**任何模型记忆都是"训练数据的快照"，不是"你的代码库的实时状态"。**

Claude Code 通过强制 I/O（读文件 → 分析 → 编辑文件）确保每一步操作都基于地面实况（ground truth），而非模型记忆。

---

## 5. 第三层：工具输出优先

**Claude Code 的系统 prompt 明确要求：优先用工具获取事实，而不是凭记忆。**

### 5.1 工具优先原则

```
- 需要了解项目结构？→ search_files / list files（不用记忆）
- 需要看具体实现？→ read_file（不用记忆）
- 需要运行测试？→ terminal: go test（不用记忆）
- 需要查文档？→ web_search（不用记忆）
```

### 5.2 工具输出的不可替代性

Claude Code 的设计确保了一个关键约束：

> **工具返回的数据是唯一的真相来源（source of truth），模型的知识只是辅助。**

这在 prompt 层面、系统设计层面、以及训练数据层面都是一致的。

---

## 6. 第四层：Command Sandboxing

**执行命令需要用户审批，且 Claude Code 必须先告知将执行什么。**

### 6.1 权限模式

```
Plan mode:   只能读，不能写/执行
Approve mode: 每次写/执行需要用户确认
Auto mode:   自动执行（仅在用户信任的范围内）
```

### 6.2 审批机制

```
Claude Code 提议: "我将运行 git commit -m 'fix: bug'"
                  ↓
用户审批:         批准 / 拒绝 / 修改后批准
                  ↓
Claude Code 执行:  系统记录该操作
```

### 6.3 防幻觉价值

这层机制间接防止了多种幻觉：
- "我帮你提交了代码" → 必须真实执行 git commit
- "我部署了" → 必须真实触发 CI/CD
- "我修复了" → 必须真实修改文件

**不能说谎，因为谎言会在审批和执行阶段被揭穿。**

---

## 7. 第五层：Structured Thinking

**Claude Code 在行动前展示完整推理过程。**

### 7.1 显式推理

在执行任何操作前，Claude Code 输出：

```markdown
## 分析
- 用户需求：修复 login 页面的 token 刷新 bug
- 相关文件：src/auth/token.ts, src/pages/Login.tsx
- 根因判断：token refresh 的 interceptor 在 401 时未重试原始请求

## 方案
1. 读取 src/auth/token.ts 确认 refresh 逻辑
2. 修改 interceptor 增加重试队列
3. 补充测试

## 风险
- 可能影响其他使用同一 interceptor 的页面
- 需要确认 refresh endpoint 的幂等性
```

### 7.2 防幻觉价值

分步推理本身不防幻觉，但**显式化推理过程**让幻觉更早暴露：
- 用户可以在执行前发现错误推理
- 错误的文件路径、不存在的 API 会立即被识别
- 不一致的逻辑在 reasoning 阶段就暴露

---

## 8. 第六层：Refusal Over Guessing

**Claude 的 Constitutional AI 训练使它更倾向于拒绝而非猜测。**

### 8.1 行为模式

| 场景 | 幻觉倾向模型 | Claude 的行为 |
|---|---|---|
| 问一个不存在于代码库的函数 | 编造一个"看起来合理"的实现 | "我没有在代码库中找到这个函数。请确认函数名" |
| 要求执行破坏性命令 | 假装执行了 | "这个命令会删除文件，你需要确认吗？" |
| 信息不足以回答问题 | 推断/猜测 | "我需要更多信息。你能提供 X 吗？" |

### 8.2 与 OpsCaption 的相似之处

这和 OpsCaption 的 `ResultStatusDegraded` 策略是同一种设计哲学：

> **宁可拒绝/降级，也不编造。**

区别在于 OpsCaption 是在系统架构层实现，Claude Code 是在模型训练 + 系统 prompt 层实现。

---

## 9. 第七层：Project Rules 文件

**CLAUDE.md / AGENTS.md — 仓库级约束文件。**

### 9.1 机制

项目根目录的 `CLAUDE.md`（或 `AGENTS.md`）文件在每次会话启动时被 Claude Code 加载到系统 prompt 中，包含：

```
- 项目技术栈
- 代码规范
- 禁止行为
- 历史踩坑规则
- 已知的限制和边界
```

### 9.2 与 OpsCaption 的相似之处

这直接启发了 OpsCaption 的 `AGENTS.md`。两个项目的设计理念完全一致：**仓库是唯一事实源，不在 Slack/Wiki 里的知识对 Agent 等于不存在。**

---

## 10. 第八层：User Approval Gate

**人类是防幻觉的最后一道防线。**

### 10.1 Diff Review

Claude Code 每次修改代码都展示 unified diff，用户逐行审查：

```diff
- const API_URL = "https://api.example.com"
+ const API_URL = process.env.API_URL || "https://api.example.com"
```

### 10.2 防幻觉价值

- 如果 Claude 编造了一个不存在的环境变量名，用户在 diff 中会看到
- 如果 Claude 漏改了某个文件，用户在测试结果中会发现
- 如果 Claude 的逻辑有误，用户在代码审查中会察觉

---

## 11. 总结：Claude Code vs OpsCaption 防幻觉策略对比

| 层次 | Claude Code | OpsCaption |
|---|---|---|
| **模型训练层** | Constitutional AI 训练，内化"诚实" | 依赖通用 LLM（DeepSeek）+ prompt 约束 |
| **地面实况层** | File-First：强制读取真实文件 | EvidenceItem：所有结论必须来自工具调用 |
| **工具优先** | 系统 prompt 要求先用工具查 | 架构要求 specialist 只输出工具结果 |
| **执行审批** | Command Sandboxing | Approval Gate（Redis 审批队列） |
| **推理可见** | Structured Thinking 显式分步 | Reporter 聚合展示 reasoning |
| **拒绝而非猜测** | Constitutional AI 内化倾向 | ResultStatusDegraded 架构层实现 |
| **仓库约束** | CLAUDE.md / AGENTS.md | AGENTS.md（同样设计） |
| **人类终审** | Diff Review + 命令审批 | Kill Switch + Approval Gate |

---

## 12. 关键差异与启示

### 12.1 训练层差异

Claude Code 最大的优势在**模型训练层**——Constitutional AI 让 Claude 在"出厂"时就天然更倾向于诚实和不编造。

OpsCaption 依赖的 DeepSeek 没有 Constitutional AI，所以必须在**系统架构层**做更多补偿——这也是为什么 OpsCaption 的 EvidenceItem、Contract MustNot、Degraded 降级体系比大多数项目都重。

**启示**：当底层模型不可控时，架构层的防幻觉投入必须翻倍。

### 12.2 Claude Code 可以借鉴的点

- **File-First → Evidence-First**：OpsCaption 已经实现了（EvidenceItem），可以强化
- **Structured Thinking → Reporter 展示**：已部分实现，可进一步要求每个 specialist 在回复中显式展示推理链
- **Diff Review → Evidence Trace**：OpsCaption 的 EvidenceItem 有 source_type/source_id，已经支持可追溯

### 12.3 OpsCaption 更优的点

- **Kill Switch**：Claude Code 没有全局紧急降级开关，OpsCaption 有（Redis 即时生效）
- **Multi-Agent Contract**：Claude Code 是单体 Agent，没有角色分工的 MustNot 约束。OpsCaption 的 contracts 体系更精细
- **Degradation 三态模型**：succeeded/failed/degraded 比 Claude Code 的 pass/fail 更细粒度

---

## 13. 结论

Claude Code 和 OpsCaption 的防幻觉策略殊途同归：

| | Claude Code | OpsCaption |
|---|---|---|
| 核心哲学 | 让模型内化"诚实" | 让架构约束"诚实" |
| 强项 | 训练层（Constitutional AI） | 架构层（Evidence + Degraded + Contract） |
| 最适合 | 问题不可预知、需灵活应变 | 问题域明确、需严格审计 |

两者互补：OpsCaption 可以学习 Claude Code 的 Structured Thinking 展示推理链，Claude Code 可以借鉴 OpsCaption 的 Kill Switch + Contract 体系。

---

## 相关文档

- [07-防幻觉体系设计](./07-防幻觉体系设计.md) — OpsCaption 防幻觉六层模型
- [01-概览与概念](./01-概览与概念.md) — Harness 工程教学
- [04-验证闭环设计](./04-验证闭环设计.md) — 补上运行时校验
