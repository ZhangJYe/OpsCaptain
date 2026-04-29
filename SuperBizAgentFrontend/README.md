# OpsCaption Frontend v2 — React 重构版

## 技术栈
React 18 · TypeScript · Vite · Tailwind CSS · Framer Motion · react-markdown · Lucide Icons

## 本地开发

```bash
npm install
npm run dev
```

访问 http://localhost:5173，API 代理到后端。

## 部署到服务器

### 方式一：Docker 多阶段构建（推荐）

```bash
docker build -t opscaption-frontend .
docker run -d -p 80:80 --network opscaption opscaption-frontend
```

### 方式二：手动构建

```bash
npm install
npm run build
# dist/ 目录即为静态文件，复制到 nginx 根目录
```

## 与旧版差异

| | v1（原生 HTML） | v2（React） |
|---|---|---|
| 包大小 | 292KB | ~1.5MB (gzip ~400KB) |
| 构建工具 | 无 | Vite |
| 动画 | 无 | Framer Motion |
| 类型安全 | 无 | TypeScript |
| 组件化 | 单体 JS class | 18 个组件 |
| 流式效果 | 整块刷新 | 打字机逐字输出 |
