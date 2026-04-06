# Skills Catalog

这个目录放的是项目级 `skill cards`。

要区分两层：

- 代码层 skills：`internal/ai/skills/` 和 `internal/ai/agent/skillspecialists/`
- 文档层 skills：这个 `skills/` 目录里的 Markdown

文档层的作用不是给程序直接执行，而是给人看：

- 复盘当前项目到底有哪些 skill
- 学习每个 skill 的触发条件和边界
- 面试时快速说明 skill 设计
- 后续继续扩 skill 时保持统一格式

## 当前目录结构

- `skills/knowledge/`
- `skills/metrics/`
- `skills/logs/`

## 每张 skill card 都回答 6 个问题

1. 这个 skill 解决什么问题
2. 它通常在什么 query 下被选中
3. 它会给检索或工具调用增加什么 focus
4. 它主要调用什么 tool
5. 它输出什么 evidence / metadata / next actions
6. 如果失败，会怎么降级

## 运行时实现位置

- `internal/ai/skills/registry.go`
- `internal/ai/agent/skillspecialists/knowledge/agent.go`
- `internal/ai/agent/skillspecialists/metrics/agent.go`
- `internal/ai/agent/skillspecialists/logs/agent.go`
