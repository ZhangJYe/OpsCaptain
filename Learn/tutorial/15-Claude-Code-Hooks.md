# Claude Code Hooks 完整指南

> 本文档包含 Claude Code Hooks 的完整介绍、实战实现和面试拓展问题。

---

## 一、什么是 Claude Code Hooks？

### 1.1 一句话定义

**Hooks 是 Claude Code 的事件钩子系统**——在 Claude 执行特定操作的前后，自动运行你预设的脚本，实现安全防护、质量检查、流程自动化。

### 1.2 核心价值

```
┌─────────────────────────────────────────────────────────────┐
│                      Claude Code                            │
│                                                             │
│   用户输入 → [Claude 处理] → 工具调用 → [Claude 处理] → 输出  │
│                    ↑                              ↑         │
│               PreToolUse                    PostToolUse     │
│                    │                              │         │
│              ┌─────┴─────┐                  ┌─────┴─────┐   │
│              │  Hook 脚本 │                  │  Hook 脚本 │   │
│              │  - 拦截    │                  │  - 日志    │   │
│              │  - 校验    │                  │  - 检查    │   │
│              │  - 修改    │                  │  - 通知    │   │
│              └───────────┘                  └───────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 类比理解

| 概念 | 类比 |
|------|------|
| Hooks | Git Hooks（pre-commit, post-commit） |
| PreToolUse | 前端的表单校验——提交前检查 |
| PostToolUse | 后端的 AOP——操作后记录日志 |
| SessionStart | 服务器启动时的初始化脚本 |

---

## 二、Hook 事件详解

### 2.1 完整事件列表

| 事件 | 触发时机 | 输入参数 | 典型用途 |
|------|----------|----------|----------|
| `SessionStart` | 会话启动时 | session_id, cwd | 加载环境变量、初始化配置 |
| `Setup` | 首次运行时 | - | 安装依赖、检查环境 |
| `InstructionsLoaded` | 加载指令后 | instructions | 修改/补充指令 |
| `UserPromptSubmit` | 用户输入后 | prompt | 输入预处理、敏感词过滤 |
| `UserPromptExpansion` | 提示词展开后 | expanded_prompt | 动态扩展上下文 |
| `PreToolUse` | 工具调用前 | tool_name, tool_input | 拦截危险操作、参数校验 |
| `PermissionRequest` | 权限请求时 | permission | 自动授权/拒绝 |
| `PostToolUse` | 工具调用后 | tool_name, tool_output | 日志、验证、后处理 |
| `PostToolUseFailure` | 工具调用失败后 | error | 错误处理、重试 |
| `PreResponse` | 响应前 | response | 修改响应内容 |
| `PostResponse` | 响应后 | response | 日志、通知 |
| `SessionEnd` | 会话结束时 | - | 清理资源、保存状态 |

### 2.2 事件执行流程

```
用户输入
    │
    ▼
UserPromptSubmit ──→ [可拦截/修改输入]
    │
    ▼
UserPromptExpansion ──→ [可扩展上下文]
    │
    ▼
Claude 处理
    │
    ▼
PreToolUse ──→ [可拦截/修改工具调用]
    │
    ├── exit 0: 允许执行
    ├── exit 2: 阻止执行
    └── JSON: 修改输入参数
    │
    ▼
工具执行
    │
    ▼
PostToolUse ──→ [可验证/记录结果]
    │
    ├── exit 0: 正常继续
    ├── exit 2: 标记失败
    └── JSON: 添加上下文信息
    │
    ▼
PreResponse ──→ [可修改响应]
    │
    ▼
响应输出
    │
    ▼
PostResponse ──→ [日志/通知]
```

---

## 三、配置详解

### 3.1 配置文件位置

```
项目级: <project-root>/claude.config.json
用户级: ~/.claude/config.json
```

### 3.2 配置结构

```json
{
  "hooks": {
    "<事件名>": [
      {
        "name": "hook名称（用于日志和调试）",
        "command": "要执行的命令或脚本路径",
        "args": ["参数1", "参数2"],
        "matcher": "工具名匹配模式（可选）",
        "timeout": 5000,
        "enabled": true
      }
    ]
  }
}
```

### 3.3 模板变量

Hook 脚本可以通过模板变量接收上下文信息：

| 变量 | 说明 | 可用事件 |
|------|------|----------|
| `{{tool_name}}` | 工具名称 | PreToolUse, PostToolUse |
| `{{tool_input}}` | 工具输入（JSON） | PreToolUse, PostToolUse |
| `{{tool_output}}` | 工具输出（JSON） | PostToolUse |
| `{{response}}` | 响应内容 | PreResponse, PostResponse |
| `{{session_id}}` | 会话 ID | 所有事件 |
| `{{user_prompt}}` | 用户输入 | UserPromptSubmit |
| `{{cwd}}` | 当前工作目录 | 所有事件 |

### 3.4 Matcher 模式

```json
{
  "matcher": "Bash"           // 精确匹配
  "matcher": "Write|Edit"     // 多个工具（OR）
  "matcher": "git*"           // 通配符
  "matcher": "~(Bash|Write)"  // 正则表达式
}
```

---

## 四、Hook 输出格式

### 4.1 Exit Code

| Exit Code | 含义 | 行为 |
|-----------|------|------|
| 0 | 成功 | 继续执行 |
| 1 | 失败 | 记录错误，继续执行 |
| 2 | 阻止 | 阻止工具调用/响应 |

### 4.2 JSON 输出

Hook 脚本可以输出 JSON 来控制行为：

#### 添加上下文给 Claude
```json
{
  "context": "这条信息会添加到 Claude 的上下文中"
}
```

#### 阻止操作并给出原因
```json
{
  "decision": "block",
  "reason": "这个操作太危险了"
}
```

#### 允许操作
```json
{
  "decision": "allow"
}
```

#### 修改工具输入（PreToolUse）
```json
{
  "modified_input": {
    "新的参数": "值"
  }
}
```

#### 添加权限（PermissionRequest）
```json
{
  "update": [
    {
      "tool": "Bash",
      "pattern": "git status",
      "allow": true
    }
  ]
}
```

---

## 五、实战实现

### 5.1 项目结构

```
<project-root>/
├── claude.config.json      # Hook 配置
├── hooks/                  # Hook 脚本目录
│   ├── block-dangerous.sh  # 阻止危险操作
│   ├── protect-secrets.sh  # 保护敏感文件
│   ├── auto-vet.sh         # 自动 go vet
│   ├── audit-log.sh        # 审计日志
│   └── utils.sh            # 公共函数
└── .claude-audit.log       # 审计日志文件
```

### 5.2 完整配置文件

```json
{
  "hooks": {
    "SessionStart": [
      {
        "name": "init-environment",
        "command": "./hooks/session-start.sh",
        "timeout": 5000
      }
    ],
    "PreToolUse": [
      {
        "name": "block-dangerous-commands",
        "command": "./hooks/block-dangerous.sh",
        "args": ["{{tool_name}}", "{{tool_input}}"],
        "matcher": "Bash",
        "timeout": 3000
      },
      {
        "name": "protect-sensitive-files",
        "command": "./hooks/protect-secrets.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Write|Edit",
        "timeout": 3000
      },
      {
        "name": "block-direct-push",
        "command": "./hooks/block-push-main.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Bash",
        "timeout": 3000
      }
    ],
    "PostToolUse": [
      {
        "name": "auto-go-vet",
        "command": "./hooks/auto-vet.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Write|Edit",
        "timeout": 10000
      },
      {
        "name": "audit-log",
        "command": "./hooks/audit-log.sh",
        "args": ["{{tool_name}}", "{{tool_input}}", "{{tool_output}}"],
        "timeout": 3000
      }
    ],
    "PostToolUseFailure": [
      {
        "name": "error-handler",
        "command": "./hooks/error-handler.sh",
        "args": ["{{tool_name}}", "{{tool_output}}"]
      }
    ]
  }
}
```

### 5.3 Hook 脚本实现

#### hooks/utils.sh（公共函数）
```bash
#!/bin/bash
# 公共工具函数

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> .claude-audit.log
}

block() {
  local reason="$1"
  echo "{\"decision\":\"block\",\"reason\":\"$reason\"}"
  exit 2
}

allow() {
  echo "{\"decision\":\"allow\"}"
  exit 0
}

add_context() {
  local context="$1"
  echo "{\"context\":\"$context\"}"
  exit 0
}
```

#### hooks/block-dangerous.sh（阻止危险命令）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_NAME="$1"
TOOL_INPUT="$2"

# 阻止 rm -rf 关键目录
if echo "$TOOL_INPUT" | grep -qE 'rm\s+(-rf?|--recursive)\s+'; then
  if echo "$TOOL_INPUT" | grep -qE '\.(git|env|docker|ssh|config)'; then
    block "检测到删除关键目录操作，已阻止"
  fi
fi

# 阻止格式化磁盘
if echo "$TOOL_INPUT" | grep -qE '(mkfs|fdisk|dd\s+if=)'; then
  block "检测到磁盘操作命令，已阻止"
fi

# 阻止修改系统文件
if echo "$TOOL_INPUT" | grep -qE '(\/etc\/|\/usr\/|\/var\/)'; then
  block "检测到系统目录修改，已阻止"
fi

allow
```

#### hooks/protect-secrets.sh（保护真正敏感文件）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

SENSITIVE_PATTERNS=(
  "\.env$"
  "\.env\."
  "secret"
  "password"
  "credentials"
  "\.pem"
  "\.key"
  "id_rsa"
)

for pattern in "${SENSITIVE_PATTERNS[@]}"; do
  if echo "$TOOL_INPUT" | grep -qiE "$pattern"; then
    block "检测到敏感文件操作: $pattern，需要人工确认"
  fi
done

allow
```

> `config.yaml` 不再默认拦截读取或编辑。项目排障经常需要看配置，真正要保护的是 `.env`、key、pem、secret 等密钥类文件。

#### hooks/block-push-main.sh（提醒直接推送到 main）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

if echo "$TOOL_INPUT" | grep -qE 'git\s+push.*\b(main|master)\b'; then
  add_context "⚠️ 检测到推送到 main/master 分支，请确认这是预期操作"
fi

allow
```

#### hooks/auto-vet.sh（自动 go vet）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 只对 .go 文件触发
if ! echo "$TOOL_INPUT" | grep -qE '\.go"'; then
  exit 0
fi

log "检测到 Go 文件修改，运行 go vet..."

# 运行 go vet
VET_OUTPUT=$(go vet ./... 2>&1)
VET_EXIT=$?

if [ $VET_EXIT -ne 0 ]; then
  add_context "⚠️ go vet 发现问题:\n$VET_OUTPUT"
fi

exit 0
```

#### hooks/audit-log.sh（审计日志）
```bash
#!/bin/bash
TOOL_NAME="$1"
TOOL_INPUT="$2"
TOOL_OUTPUT="$3"

# 截断过长的输出
MAX_LEN=500
if [ ${#TOOL_OUTPUT} -gt $MAX_LEN ]; then
  TOOL_OUTPUT="${TOOL_OUTPUT:0:$MAX_LEN}...(截断)"
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] $TOOL_NAME" >> .claude-audit.log
echo "  Input: $TOOL_INPUT" >> .claude-audit.log
echo "  Output: $TOOL_OUTPUT" >> .claude-audit.log
echo "---" >> .claude-audit.log

exit 0
```

#### hooks/session-start.sh（会话初始化）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

log "=== 新会话开始 ==="

# 加载项目上下文
if [ -f ".env.local" ]; then
  add_context "已加载 .env.local 环境变量"
fi

# 检查 Go 版本
GO_VERSION=$(go version 2>/dev/null)
if [ -n "$GO_VERSION" ]; then
  add_context "Go 环境: $GO_VERSION"
fi

exit 0
```

#### hooks/error-handler.sh（错误处理）
```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_NAME="$1"
TOOL_OUTPUT="$2"

log "❌ 工具调用失败: $TOOL_NAME"
log "错误信息: $TOOL_OUTPUT"

# 可以在这里添加通知逻辑
# curl -X POST "https://hooks.slack.com/..." -d "{\"text\":\"Hook 失败: $TOOL_NAME\"}"

exit 0
```

---

## 六、OpsCaptain 项目专用配置

### 6.1 针对 Go 项目的优化

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "name": "protect-go-files",
        "command": "./hooks/protect-go-deps.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Write|Edit",
        "timeout": 2000
      },
      {
        "name": "block-rm-rf",
        "command": "./hooks/block-dangerous.sh",
        "args": ["{{tool_name}}", "{{tool_input}}"],
        "matcher": "Bash",
        "timeout": 2000
      }
    ],
    "PostToolUse": [
      {
        "name": "go-vet-check",
        "command": "./hooks/go-vet.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Write|Edit",
        "timeout": 15000
      },
      {
        "name": "go-test-check",
        "command": "./hooks/go-test.sh",
        "args": ["{{tool_input}}"],
        "matcher": "Write|Edit",
        "timeout": 60000
      }
    ]
  }
}
```

### 6.2 hooks/protect-go-deps.sh

```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 保护 Makefile
if echo "$TOOL_INPUT" | grep -qE 'Makefile'; then
  block "Makefile 修改需要人工确认"
fi

# 保护 Dockerfile
if echo "$TOOL_INPUT" | grep -qE 'Dockerfile'; then
  block "Dockerfile 修改需要人工确认"
fi

allow
```

> `go.mod` / `go.sum` 不再硬拦截。正常的 `go mod tidy` 和依赖修复会修改它们，硬拦截会卡住日常 CI 修复。依赖变更改为通过 review 和 CI 控制。

### 6.3 hooks/go-vet.sh

```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 只对 .go 文件触发
if ! echo "$TOOL_INPUT" | grep -qE '\.go"'; then
  exit 0
fi

log "运行 go vet..."

VET_OUTPUT=$(cd /Users/zhangjinye/workspace/Agent/OpsCaptain && go vet ./... 2>&1)
VET_EXIT=$?

if [ $VET_EXIT -ne 0 ]; then
  # 截断过长输出
  if [ ${#VET_OUTPUT} -gt 1000 ]; then
    VET_OUTPUT="${VET_OUTPUT:0:1000}..."
  fi
  add_context "⚠️ go vet 发现问题:\n\`\`\`\n$VET_OUTPUT\n\`\`\`"
fi

exit 0
```

### 6.4 hooks/go-test.sh

```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 只对 .go 文件触发
if ! echo "$TOOL_INPUT" | grep -qE '\.go"'; then
  exit 0
fi

# 只对 internal/ai/ 目录下的文件触发测试
if ! echo "$TOOL_INPUT" | grep -qE 'internal/ai/'; then
  exit 0
fi

log "运行相关测试..."

# 获取修改的包路径
PACKAGE=$(echo "$TOOL_INPUT" | grep -oE 'internal/ai/[^/]+/' | head -1)

if [ -n "$PACKAGE" ]; then
  TEST_OUTPUT=$(cd /Users/zhangjinye/workspace/Agent/OpsCaptain && go test "./$PACKAGE..." 2>&1)
  TEST_EXIT=$?

  if [ $TEST_EXIT -ne 0 ]; then
    add_context "❌ 测试失败:\n\`\`\`\n$TEST_OUTPUT\n\`\`\`"
  else
    add_context "✅ 测试通过: $PACKAGE"
  fi
fi

exit 0
```

---

## 七、调试与验证

### 7.1 查看生效的 Hooks

在 Claude Code 中输入：
```
/hooks
```

### 7.2 测试 Hook 脚本

```bash
# 手动测试
echo '{"tool_name":"Bash","tool_input":"rm -rf .git"}' | ./hooks/block-dangerous.sh

# 查看日志
cat .claude-audit.log
```

### 7.3 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| Hook 不执行 | 路径错误 | 使用绝对路径或相对于项目根目录 |
| 权限被拒绝 | 脚本没有执行权限 | `chmod +x hooks/*.sh` |
| 超时 | 脚本执行太慢 | 增加 timeout 或优化脚本 |
| JSON 格式错误 | 输出格式不对 | 检查 echo 输出是否为有效 JSON |

---

## 八、面试拓展

### 8.1 基础概念题

**Q1: 什么是 Claude Code Hooks？它解决了什么问题？**

> **答：** Claude Code Hooks 是一个事件钩子系统，允许开发者在 Claude 执行特定操作的前后自动运行自定义脚本。它解决的核心问题是**安全性和自动化**：
> - 安全性：拦截危险操作，保护关键文件
> - 自动化：自动运行检查、记录日志，减少人工干预
> - 规范性：将团队规范编码为可执行的规则

**Q2: Hooks 和 Git Hooks 有什么区别？**

> **答：**
> | 维度 | Git Hooks | Claude Code Hooks |
> |------|-----------|-------------------|
> | 触发时机 | Git 操作（commit, push） | Claude 操作（工具调用、会话） |
> | 作用范围 | 代码版本控制 | AI 辅助开发全流程 |
> | 粒度 | 文件级别 | 命令级别 |
> | 灵活性 | 较固定 | 可动态拦截/修改 |
>
> 两者可以互补：Git Hooks 管代码质量，Claude Hooks 管 AI 行为。

**Q3: Hook 的 exit code 有什么作用？**

> **答：**
> - `exit 0`: 成功，继续执行
> - `exit 1`: 失败，记录错误但继续
> - `exit 2`: 阻止，拦截当前操作
>
> 这个设计让 Hook 既能做检查（exit 0/1），也能做拦截（exit 2）。

---

### 8.2 设计思想题

**Q4: 为什么 Hooks 要设计成事件驱动的？有什么优势？**

> **答：** 事件驱动设计的优势：
> 1. **解耦**：Hook 逻辑和主流程分离，互不影响
> 2. **可扩展**：新增 Hook 不需要修改核心代码
> 3. **灵活组合**：多个 Hook 可以串联执行
> 4. **易于测试**：每个 Hook 可以独立测试
> 5. **符合开闭原则**：对扩展开放，对修改关闭

**Q5: 如果让你设计一个 Hook 系统，你会考虑哪些因素？**

> **答：**
> 1. **性能**：Hook 执行不能阻塞主流程，需要设置超时
> 2. **安全性**：Hook 脚本本身也需要权限控制
> 3. **可观测性**：完善的日志和监控
> 4. **错误隔离**：一个 Hook 失败不应影响其他 Hook
> 5. **配置管理**：支持项目级和全局配置
> 6. **调试友好**：提供查看和测试 Hook 的工具

**Q6: Hooks 的模板变量设计有什么考量？**

> **答：**
> 1. **安全性**：变量需要转义，防止注入攻击
> 2. **灵活性**：提供足够的上下文信息
> 3. **性能**：避免传递过多数据导致性能问题
> 4. **向后兼容**：新增变量不影响已有 Hook
> 5. **类型安全**：JSON 格式确保数据结构一致

---

### 8.3 实战应用题

**Q7: 在你的项目中，你会用 Hooks 实现哪些功能？**

> **答：** 在 OpsCaptain 项目中：
>
> **安全防护：**
> - 阻止高危删除和磁盘破坏命令
> - 保护 `.env`、key、pem、secret 等真正敏感文件
> - 推送 main/master 时提醒确认，但不硬拦截
>
> **质量保证：**
> - 修改 .go 文件后自动运行 go vet
> - 修改测试文件后自动运行相关测试
> - 代码格式检查（gofmt）
>
> **审计追踪：**
> - 记录所有工具调用
> - 统计代码修改频率
> - 错误操作回溯

**Q8: 如何处理 Hook 脚本的性能问题？**

> **答：**
> 1. **设置合理超时**：根据操作复杂度设置 timeout
> 2. **异步执行**：非关键 Hook（如日志）可以异步执行
> 3. **缓存结果**：重复检查可以缓存
> 4. **精简脚本**：避免在 Hook 中做复杂计算
> 5. **按需触发**：使用 matcher 只对相关工具触发

**Q9: Hook 和 CI/CD 如何配合？**

> **答：**
> ```
> 开发阶段（Claude Hooks）    CI/CD 阶段
>         │                        │
>    实时检查 ←──────→ 自动化测试
>         │                        │
>    快速反馈              完整验证
>         │                        │
>    └────┴────→ 互补 ←────┴────┘
> ```
>
> - Hooks 提供**即时反馈**，CI/CD 提供**完整验证**
> - Hooks 拦截明显问题，CI/CD 检查深层问题
> - 两者规则应该保持一致

---

### 8.4 架构设计题

**Q10: 如果要将 Hooks 系统做成平台级服务，你会怎么设计？**

> **答：**
> ```
> ┌─────────────────────────────────────────────────┐
> │                 Hook 管理平台                     │
> ├─────────────────────────────────────────────────┤
> │  ┌─────────┐  ┌─────────┐  ┌─────────┐         │
> │  │ 配置管理 │  │ 脚本仓库 │  │ 监控面板 │         │
> │  └────┬────┘  └────┬────┘  └────┬────┘         │
> │       │            │            │               │
> │  ┌────┴────────────┴────────────┴────┐          │
> │  │          Hook 执行引擎             │          │
> │  │  ┌─────┐  ┌─────┐  ┌─────┐       │          │
> │  │  │沙箱  │  │队列  │  │缓存  │       │          │
> │  │  └─────┘  └─────┘  └─────┘       │          │
> │  └───────────────────────────────────┘          │
> │                     │                           │
> │  ┌──────────────────┴──────────────────┐        │
> │  │           事件总线                   │        │
> │  └─────────────────────────────────────┘        │
> └─────────────────────────────────────────────────┘
> ```
>
> 核心组件：
> 1. **配置中心**：统一管理 Hook 配置
> 2. **脚本仓库**：版本控制 Hook 脚本
> 3. **执行引擎**：沙箱隔离执行
> 4. **监控面板**：执行统计和告警
> 5. **事件总线**：解耦事件分发

**Q11: 如何保证 Hook 脚本的安全性？**

> **答：**
> 1. **权限最小化**：Hook 只有必要的权限
> 2. **沙箱隔离**：在隔离环境中执行
> 3. **输入校验**：模板变量需要转义
> 4. **代码审查**：Hook 脚本也需要 review
> 5. **审计日志**：记录所有 Hook 执行
> 6. **超时控制**：防止恶意脚本阻塞

---

### 8.5 开放讨论题

**Q12: Hooks 和 Agent 的关系是什么？未来会如何发展？**

> **答：**
>
> **当前关系：**
> - Hooks 是 Agent 的"规则约束"
> - Agent 负责决策，Hooks 负责执行规则
> - 两者配合实现可控的 AI 辅助开发
>
> **未来趋势：**
> 1. **智能化**：Hook 本身也会用 AI 来判断
> 2. **自适应**：根据项目特点自动配置 Hook
> 3. **协同化**：多个 Agent 共享 Hook 规则
> 4. **平台化**：Hook 市场，共享最佳实践

**Q13: 你会如何推广 Hooks 在团队中的使用？**

> **答：**
> 1. **从痛点入手**：先解决团队最头疼的问题（如误操作）
> 2. **渐进式推广**：先在小范围试点
> 3. **文档完善**：提供清晰的使用指南
> 4. **效果量化**：统计 Hook 拦截的问题数量
> 5. **持续优化**：根据反馈调整规则
> 6. **文化建设**：让团队理解这是"保护"而非"限制"

---

## 九、总结

### 9.1 核心要点

1. **本质**：事件驱动的脚本触发器
2. **价值**：安全、质量、规范、审计
3. **配置**：claude.config.json + 脚本文件
4. **输出**：exit code + JSON 控制行为

### 9.2 学习路径

```
基础 → 配置语法 → 简单脚本 → 项目实践 → 高级用法
  │        │          │          │          │
  └────────┴──────────┴──────────┴──────────┘
           每个阶段都要动手实践
```

### 9.3 推荐资源

- [官方文档](https://docs.anthropic.com/en/docs/claude-code/hooks)
- [示例仓库](https://github.com/anthropics/claude-code-hooks-examples)
- [OpsCaptain 项目实践](./hooks/)

---

## 附录：快速参考

### 常用 Hook 模板

```json
{
  "hooks": {
    "PreToolUse": [{
      "name": "block-dangerous",
      "command": "./hooks/block.sh",
      "args": ["{{tool_input}}"],
      "matcher": "Bash",
      "timeout": 3000
    }],
    "PostToolUse": [{
      "name": "audit-log",
      "command": "./hooks/log.sh",
      "args": ["{{tool_name}}", "{{tool_output}}"],
      "timeout": 2000
    }]
  }
}
```

### 常用脚本模板

```bash
#!/bin/bash
source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 检查逻辑
if [ condition ]; then
  block "原因说明"
fi

allow
```

### 调试命令

```bash
# 查看 Hooks
/hooks

# 测试脚本
echo '{"test":"data"}' | ./hooks/test.sh

# 查看日志
tail -f .claude-audit.log
```
