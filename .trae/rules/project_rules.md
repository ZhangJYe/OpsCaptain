请参照项目根目录 AGENTS.md 作为本项目所有 Agent 行为的约束文件。

核心要点速查：

- 技术栈：Go 1.24+ / GoFrame v2 / cloudwego/eino / Milvus / DeepSeek V3
- 不加注释除非明确要求
- 错误处理统一走 ResultStatusDegraded
- 新增 agent 或 tool 前先补 replay case
- 配置项不硬编码，走 config.yaml
- commit message 用中文
- 不主动 commit 除非用户明确要求
- 不创建不必要的文件
- 不假设任何第三方库可用，先检查 go.mod

完整规则、历史踩坑记录、设计决策见 AGENTS.md。