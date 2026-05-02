# Harness Engineering P0 落地：AGENTS.md 设计与实践

> 当前统一口径（2026-05）
> - 本文形成于历史 Orchestrator / 多智能体讨论阶段。
> - 当前实现请以 `Chat ReAct` 与 `AIOps Runtime + Plan-Execute-Replan` 为准。

## 1. 背景

OpsCaption 项目在本文写作时仍处于 Orchestrator 编排模式的讨论阶段，具备 supervisor/triage/specialists/reporter 方案、统一协议、上下文工程、RAG 链路和 eval 骨架。但缺少一个关键工程基础设施：**仓库级 Agent 行为约束文件**。

在之前的开发过程中，大量工程经验沉淀在 `res/todo.md`（1798 行）里，但这些经验对 AI Agent 来说"等于不存在"——Agent 不会主动去读复盘文档。

---

## 2. 问题定义

当前项目面临的具体问题：

1. AI Agent 每次启动时不知道项目规则，犯过的错会反复犯
2. 历史踩坑经验（29 个章节、1798 行）只存在于人类可读文档里
3. 没有 `.golangci.yml`、没有 `.cursorrules`、没有 `.trae/rules/`
4. 团队知识散落在多个文档中，缺少统一入口

---

## 3. 为什么这样做

### 3.1 Hashimoto 原则

Mitchell Hashimoto（HashiCorp 创始人）的实践：

- 不凭空写规则
- 每踩一个坑，就加一条规则
- 规则文件随项目演进
- Agent 每次启动都加载

我们的落地方式：AGENTS.md 中的"历史踩坑规则"章节，每条规则都标注了来源章节。

### 3.2 OpenAI 原则

仓库是唯一事实源。写在 Slack、Wiki、飞书里的知识，对 Agent 等于不存在。

我们的落地方式：AGENTS.md 包含完整的文档索引，指向所有知识沉淀位置。

### 3.3 Linter 思维

错误消息不只报错，而是直接告诉 Agent 怎么改。

我们的落地方式：历史踩坑规则每条都包含"不要做什么"和"应该怎么做"。

---

## 4. 具体改动

### 4.1 创建 `AGENTS.md`

位置：项目根目录

包含章节：

- 项目概述
- 技术栈
- 核心架构
- 目录结构
- 代码规范
- 禁止事项
- 历史踩坑规则（20+ 条，每条对应 res/todo.md 中的真实失败案例）
- 测试要求
- 当前进行中的工作
- 关键设计决策记录
- 文档索引

### 4.2 创建 `.trae/rules/project_rules.md`

Trae IDE 在每次 Agent 启动时会自动加载此文件。内容指向 AGENTS.md，确保 Agent 行为约束生效。

### 4.3 历史踩坑规则提取

从 `res/todo.md` 的 29 个章节中提取了 20+ 条规则，包括：

- §6：Revoked token 内存泄漏
- §7：Long-term memory 无上限
- §8：Triage 硬编码 switch
- §9：strings.Title 废弃
- §10：Metrics Agent 错误处理不一致
- §11：loadEnvFile 手动实现
- §12：CORS Origin 校验过宽
- §13：知识库检索 hardcoded top 3
- §14：LongTermMemory.Retrieve 用了写锁
- §21.1：异步记忆抽取无超时
- §22.1：Runtime 重复创建
- §23.2：Chat memory 持久化不统一
- §24.1：Memory 污染 triage routing
- §24.2：Detail/Trace payload 膨胀
- §24.3：Knowledge retriever 重复初始化
- §27.2：记忆写入缺乏校验
- 以及 RAG、图谱、脚本相关的规则

---

## 5. 执行过程

1. 读取 `res/todo.md` 全部 1798 行，提取所有历史失败案例
2. 读取项目目录结构，确认技术栈和架构
3. 检查现有规则文件（.golangci.yml、.cursorrules、.trae/rules/）→ 全部不存在
4. 创建 AGENTS.md，填入完整规则体系
5. 创建 .trae/rules/project_rules.md，确保 Trae IDE 自动加载

---

## 6. 验证方式与结果

### 6.1 文件创建验证

- AGENTS.md 已创建在项目根目录
- .trae/rules/project_rules.md 已创建

### 6.2 规则覆盖验证

- res/todo.md 中的 9 类问题修复全部提取为规则
- P1 实现中的 5 个关键改动全部提取为规则
- 上下文工程 3 轮推进的关键约束全部提取为规则
- RAG 和图谱设计中的关键约束全部提取为规则

### 6.3 Trae 加载验证

- .trae/rules/project_rules.md 格式正确
- 下次 Agent 启动时会自动读取

---

## 7. 风险、边界、未完成项

### 风险

- AGENTS.md 是静态文件，需要人工维护。如果后续犯新错但忘记更新，反馈循环就断了
- 规则过多可能导致 Agent 上下文窗口压力增大

### 边界

- 当前只做了 AGENTS.md 和 .trae/rules/，没有做 .golangci.yml
- Linter 自定义规则是 P1 优先级，不在本次范围内

### 未完成项

- `.golangci.yml` 基础配置（P1）
- 自定义 linter 规则（P2）
- 自动化 Correction Pipeline（P2）
- AGENTS.md 版本变更追踪机制

---

## 8. 如果面对评审员，应该如何解释

评审员可能会问：

**"这不就是一个 README 吗？"**

回答：不是。README 面向人类开发者，AGENTS.md 面向 AI Agent。它的核心区别是：
- 每条规则都对应一个真实失败案例
- 格式设计为 Agent 可直接消费（禁令式、具体化、带上下文）
- 配合 .trae/rules/project_rules.md 实现 Agent 启动自动加载
- 维护方式是"犯错驱动"而非"提前设计"

**"怎么证明它有用？"**

回答：通过观察 Agent 行为变化。之前 Agent 经常犯的错（比如用 strings.Title、hardcode top_k、不走 ResultStatusDegraded），在加载 AGENTS.md 后应该不再出现。如果某条规则没生效，说明规则写得不够具体，需要迭代。

**"为什么不直接上 .golangci.yml？"**

回答：因为 AGENTS.md 的投入产出比更高。一个文件就能覆盖代码规范、禁止事项、历史经验、架构约束。.golangci.yml 只能覆盖静态代码检查，不能覆盖架构决策和工程经验。先做 AGENTS.md，后续再补 linter。

---

## 9. 我作为项目负责人应该学会什么

### 9.1 知识必须在仓库里才对 Agent 有效

你在 res/todo.md 里写了 1798 行非常有价值的复盘记录。但在创建 AGENTS.md 之前，这些知识对 AI Agent 完全不可见。

教训：**知识的价值取决于它的可访问性，而不只是它的正确性。**

### 9.2 规则来自失败，不来自设计

AGENTS.md 里最有价值的不是"项目概述"或"目录结构"，而是"历史踩坑规则"。这些规则之所以有价值，是因为每条都对应一个真实犯过的错。

教训：**不要凭空写规则，要等系统犯错后再沉淀。**

### 9.3 反馈循环比完美设计重要

AGENTS.md 第一版不需要完美。重要的是建立：Agent 犯错 → 人类更新规则 → Agent 下次不犯 的循环。

教训：**Harness Engineering 的核心不是文档，而是反馈循环。**

### 9.4 分层治理

AGENTS.md 是顶层索引，不是唯一文档。具体的设计决策在 res/ 和 todo/ 里，具体的学习材料在 Learn/ 里。AGENTS.md 的职责是让 Agent 快速定位到正确的知识源。

教训：**不要把所有知识都堆在一个文件里，要建立索引层。**
