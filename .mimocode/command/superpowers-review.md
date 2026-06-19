---
description: "使用 superpowers 对 OpsCaption 项目进行全面 code review，识别 P0-P3 级别问题并给出修复建议"
---

# Superpowers Project Review

对 OpsCaption 项目执行全面 code review。

## Review 范围

根据用户指定的范围执行（默认全量）：

1. **架构合规性**：检查 AGENTS.md 中的分层规则、import 规则、行为红线
2. **代码质量**：超大文件（>500 行）、Controller 职责越界、硬编码配置
3. **安全性**：secrets 暴露、import 违规（ai/→infra、utility/→internal）
4. **运维就绪度**：Dockerfile、docker-compose、healthcheck、日志轮转
5. **文档完整性**：教程、配置说明、部署文档

## 执行流程

1. 读取 `AGENTS.md` 获取项目护栏和规则
2. 读取 `Learn/system/01-system-architecture-guide.md` 获取架构设计
3. 按优先级扫描：
   - P0（阻塞部署）：import 违规、配置缺失、Docker 构建失败
   - P1（功能缺陷）：dedup 时序、错误处理、超时配置
   - P2（代码异味）：超大文件、Controller 越界、硬编码
   - P3（改进建议）：文档补充、测试覆盖、可观测性
4. 输出结构化 review 报告，每个发现包含：文件路径、行号、问题描述、修复建议、优先级

## 输出格式

```
## Review 报告

### P0 - 阻塞部署
- [文件:行号] 问题描述 → 修复建议

### P1 - 功能缺陷
- [文件:行号] 问题描述 → 修复建议

### P2 - 代码异味
- [文件:行号] 问题描述 → 修复建议

### P3 - 改进建议
- [文件:行号] 问题描述 → 修复建议
```

## 用户指定范围（$ARGUMENTS）

- 无参数：全量 review
- `架构`：只检查分层和 import 规则
- `安全`：只检查 secrets 和 import 违规
- `运维`：只检查 Docker 和部署配置
- `文档`：只检查教程和配置文档
- `最近修改`：只 review 最近 git 变更的文件
