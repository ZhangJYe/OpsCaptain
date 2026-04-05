# OpsCaptain 域名和 HTTPS 配置

这篇文档只讲一件事：

让 `OpsCaptain` 从“公网 IP 访问”升级成“域名 + HTTPS 访问”。

我已经把部署结构改成了：

`浏览器 -> Caddy(HTTPS) -> frontend -> backend`

这套结构的优点是：

- 证书自动申请
- 证书自动续期
- 以后你不用手动折腾 Nginx 证书文件

## 一、你现在还缺什么

真正上线 HTTPS，还需要你准备 2 个值：

1. 一个域名
2. 一个邮箱

推荐你使用子域名，例如：

```text
opscaptain.你的域名.com
```

邮箱建议填你常用邮箱，例如：

```text
you@example.com
```

## 二、阿里云这边要做什么

### 1. 确认安全组放行

你的 ECS 安全组至少要放行：

- `22`
- `80`
- `443`

其中：

- `80` 用来做 HTTP 和证书校验
- `443` 用来做 HTTPS

### 2. 给域名加 A 记录

去你的域名 DNS 控制台，新增一条 A 记录：

如果你用子域名：

```text
主机记录：opscaptain
记录类型：A
记录值：8.136.46.241
```

如果你想直接用根域名：

```text
主机记录：@
记录类型：A
记录值：8.136.46.241
```

改完后，DNS 生效通常需要几分钟到几十分钟。

## 三、GitHub 里要改什么

你现在不需要新增新的 Secret 名字。

你只需要去修改现有的：

`PROD_ENV_FILE`

在里面补上这两行：

```env
DOMAIN_NAME=opscaptain.你的域名.com
TLS_EMAIL=你的邮箱
```

例如：

```env
DOMAIN_NAME=opscaptain.example.com
TLS_EMAIL=admin@example.com
```

## 四、建议你现在用的 PROD_ENV_FILE 写法

如果你已经能跑通当前版本，那么只要在原来的内容上增加这两行即可：

```env
GLM_API_KEY=你的大模型API密钥
DS_THINK_BASE_URL=https://open.bigmodel.cn/api/paas/v4
DS_THINK_MODEL=GLM-4.5-AIR
DS_QUICK_BASE_URL=https://open.bigmodel.cn/api/paas/v4
DS_QUICK_MODEL=GLM-4.5-AIR

DOMAIN_NAME=opscaptain.你的域名.com
TLS_EMAIL=你的邮箱

SILICONFLOW_API_KEY=
EMBEDDING_BASE_URL=https://api.siliconflow.cn/v1
EMBEDDING_MODEL=BAAI/bge-m3
EMBEDDING_DIMENSION=1024

FILE_DIR=/app/docs
RUNTIME_DATA_DIR=/app/var/runtime
UPLOAD_MAX_SIZE_MB=20

MILVUS_ADDRESS=
MYSQL_DSN=
PROMETHEUS_ADDRESS=
MCP_LOG_URL=
LOG_TOPIC_REGION=
LOG_TOPIC_ID=

AUTH_ENABLED=false
AUTH_JWT_SECRET=OpsCaptain_please_change_me_if_enable_auth_2026
AUTH_TOKEN_EXPIRY_HOURS=24
AUTH_RATE_LIMIT_PER_MINUTE=20
AUTH_RATE_LIMIT_BURST=30

CORS_ALLOWED_ORIGIN=https://opscaptain.你的域名.com
```

## 五、改完后怎么生效

改完 `PROD_ENV_FILE` 以后：

1. 打开 GitHub 仓库
2. 进入 `Actions`
3. 找到 `cd`
4. 点击 `Run workflow`

这次部署会多启动一个 `caddy` 容器。

## 六、怎么判断 HTTPS 成功了

部署完成后，你先测这 3 个地址：

1. `http://你的域名`
2. `https://你的域名`
3. `https://你的域名/healthz`

理想结果是：

- 第 1 个会跳到 HTTPS
- 第 2 个能打开首页
- 第 3 个返回 `200`

## 七、如果 HTTPS 没成功，优先查什么

按这个顺序排查：

1. 域名有没有真的解析到 `8.136.46.241`
2. 安全组有没有放行 `80` 和 `443`
3. `PROD_ENV_FILE` 里的 `DOMAIN_NAME` 有没有写错
4. `cd` 工作流有没有执行成功
5. ECS 上 `caddy` 容器是不是在运行

服务器上检查命令：

```bash
cd /opt/opscaptain
source ./release.env
docker compose --env-file .env.production -f docker-compose.prod.yml ps
docker compose --env-file .env.production -f docker-compose.prod.yml logs -f caddy
```

## 八、这一步完成后，你会得到什么

完成后，OpsCaptain 会从：

```text
http://8.136.46.241
```

变成：

```text
https://opscaptain.你的域名.com
```

这才更像正式上线的产品形态。
