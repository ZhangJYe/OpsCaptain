# 开源运维知识库入库策略

## 先说结论

可以导入外部开源运维文档，但我不建议把整站内容不加筛选地全部塞进 `docs/`。

更稳的做法是：

1. 只选择官方、开源、可追溯来源
2. 优先导入 runbook / troubleshooting / concepts 这三类内容
3. 原始来源可以很多，但落地时统一整理成结构化 Markdown
4. 每篇文档保留 `source_url`、主题标签、适用场景和更新时间

## 为什么不建议整站全量镜像

### 1. 噪声太大

很多官方站点同时包含：

- 安装手册
- 版本发布说明
- 参考 API
- 历史版本页面
- 多语言页面
- 老旧废弃页面

如果整站都进知识库，RAG 很容易召回：

- 过期命令
- 非故障排查内容
- 与当前问题无关的参考页

### 2. 版权和许可不统一

有些官方文档在 GitHub 上是开源仓库，可追溯、可引用、可二次分发。
有些站点虽然公开可访问，但不等于就应该整站镜像到自己的仓库里。

所以我建议：

- 可以批量导入：明确开源、许可清晰、最好有 GitHub 仓库的文档
- 谨慎处理：只有网站、没有清晰许可说明、或者偏商业文档的内容

### 3. 大量原文不一定比摘要更好

对当前项目来说，真正重要的是：

- 检索到的内容是否能回答 oncall 问题
- 文档是否能支撑 skills 命中
- 召回是否稳定

这比“文档数量很多”更重要。

## 当前推荐的入库分层

### 第一层：原始知识源

放在 `docs/`，作为 RAG 的基础语料。

特点：

- 面向检索
- 主题明确
- 内容稳定
- 可以是整理版，不必须是官网原文逐字复制

### 第二层：结构化能力卡

放在 `skills/`，作为 skill 的语义说明和能力边界。

特点：

- 面向路由和复盘
- 描述什么时候命中
- 描述调用哪些 tools
- 描述预期输出和 next actions

## 建议的来源白名单

- Kubernetes 官方文档
- Prometheus 官方文档
- kube-prometheus runbooks
- Helm 官方文档
- Argo CD 官方文档
- OpenTelemetry 官方文档
- etcd 官方文档

## 建议的导入规则

### 优先导入

- runbook
- troubleshooting
- concepts
- rollout / rollback
- alerting / monitoring / observability
- failure modes / disaster recovery

### 暂缓导入

- 全量 API reference
- 全量 release notes
- 非当前技术栈的长篇教程
- 同一主题的大量历史版本页面

## 适合你当前项目的做法

### 第一步：先做高质量摘要型知识库

- 围绕高频 oncall 场景
- 用中文重组
- 保留原始来源链接
- 让知识库更适合面试和项目演示

### 第二步：再做官方 Markdown 子集镜像

前提是来源满足两个条件：

1. 有明确开源许可
2. 能拿到原始 Markdown 仓库

这个阶段可以单独建立例如：

- `docs/upstream/kubernetes/`
- `docs/upstream/prometheus/`
- `docs/upstream/runbooks/`

但仍然不建议直接整站全量拉取。

## 面试怎么讲

如果面试官问“为什么不直接把官网全抓下来”，你可以这样回答：

> 我没有把知识库建设理解成简单爬虫，而是先做知识筛选和标准化。  
> 对 RAG 来说，低噪声、高相关、高可解释比文档数量更重要。  
> 所以第一阶段我先用官方来源整理高频运维场景，第二阶段再对许可清晰的官方 Markdown 仓库做子集化导入。

## 来源

- Source URL: https://kubernetes.io/docs/
- Source URL: https://prometheus.io/docs/
- Source URL: https://runbooks.prometheus-operator.dev/
- Source URL: https://helm.sh/docs/
- Source URL: https://argo-cd.readthedocs.io/en/stable/
- Source URL: https://opentelemetry.io/docs/
- Source URL: https://etcd.io/docs/
