# 前端重构实施记录 — React v2

> 从原生 HTML/CSS/JS → React 18 + TypeScript + Vite + Framer Motion

---

## 交付清单

### 源码（20 文件）

```
src/
├── main.tsx                          # React 入口
├── App.tsx                           # 根组件：主题 + 聊天 + 状态管理
├── index.css                         # Tailwind + 全局样式
├── types/chat.ts                     # TypeScript 类型定义
├── lib/
│   ├── utils.ts                      # API 地址、ID 生成
│   └── storage.ts                    # localStorage 会话持久化
├── hooks/
│   ├── useChat.ts                    # SSE 流式聊天 + 快速模式
│   └── useTheme.ts                   # 暗色/亮色主题切换
└── components/
    ├── layout/
    │   ├── MainLayout.tsx            # 整体布局 + 侧栏动画
    │   └── TopBar.tsx                # 顶栏：菜单/品牌/主题/模式
    ├── chat/
    │   ├── ChatView.tsx              # 聊天视图：消息列表 + 流式 + 输入
    │   ├── MessageBubble.tsx         # 消息气泡：用户/AI 两种样式
    │   ├── StreamingText.tsx         # 打字机逐字流式输出
    │   └── ChatInput.tsx             # 输入区域：模式切换 + 发送/停止
    ├── sidebar/
    │   ├── Sidebar.tsx               # 侧栏容器
    │   ├── OperatorCard.tsx          # 值班助手卡片 + Live 脉冲
    │   ├── ModeSelector.tsx          # 对话方式切换（动画标签）
    │   ├── HistoryPanel.tsx          # 历史会话 + 搜索 + 删除
    │   └── ObservabilityPanel.tsx    # 服务状态探测 + 状态灯
    └── welcome/
        └── WelcomeScreen.tsx         # 欢迎屏 + 快速诊断入口
```

### 配置文件

| 文件 | 说明 |
|---|---|
| `package.json` | 依赖声明 |
| `vite.config.ts` | Vite 构建 + API 代理 |
| `tsconfig.json` | TypeScript 严格模式 |
| `tailwind.config.ts` | Tailwind 暗色模式 + 自定义动画 |
| `postcss.config.js` | PostCSS 插件 |
| `index.html` | HTML 入口 |
| `Dockerfile` | 多阶段构建：Node 构建 → Nginx serve |
| `README.md` | 开发/部署文档 |

---

## 动态效果清单

| 效果 | 实现 | 文件 |
|---|---|---|
| 打字机流式 | useChat SSE + StreamingText 逐字渲染 | StreamingText.tsx |
| 消息滑入 | Framer Motion spring + scale | ChatView.tsx |
| 侧栏动画 | AnimatePresence + slide | MainLayout.tsx |
| 模式切换 | layoutId 下划线滑动 | ModeSelector.tsx |
| 思考中三点 | animate-pulse-dot CSS 动画 | ChatView.tsx |
| 状态灯脉冲 | animate-ping 环 + 实心点 | OperatorCard.tsx |
| 发送按钮变形 | 纸飞机↔停止方块 icon 切换 | ChatInput.tsx |
| 欢迎屏淡入 | Framer Motion fade + y | WelcomeScreen.tsx |
| 服务状态灯 | 绿/黄/红 + animate-pulse | ObservabilityPanel.tsx |

---

## 服务器构建步骤

```bash
# 1. 上传新前端目录到服务器
rsync -avz SuperBizAgentFrontend/ user@124.222.57.178:/path/to/SuperBizAgentFrontend/

# 2. SSH 到服务器
ssh user@124.222.57.178

# 3. 构建 Docker 镜像
cd /path/to/SuperBizAgentFrontend
docker build -t opscaption-frontend:v2 .

# 4. 替换旧容器
docker stop opscaption-frontend
docker rm opscaption-frontend
docker run -d --name opscaption-frontend \
  --network opscaption \
  -p 80:80 \
  opscaption-frontend:v2
```

---

## 生产构建产物

```
dist/
├── index.html        (~1KB)
├── assets/
│   ├── index-xxx.js  (~150KB gzip)
│   └── index-xxx.css (~15KB gzip)
└── config.js         (运行时配置)
```

总大小 ~170KB gzip，比旧版 292KB 更小（得益于 Vite tree-shaking + 代码分割）。
