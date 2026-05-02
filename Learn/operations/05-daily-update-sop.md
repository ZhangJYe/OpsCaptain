# OpsCaptain 日常更新 SOP

这篇文档讲的是：

以后你改了前端或后端，应该怎么稳定发版。

目标不是“最复杂”，而是“你自己能重复做对”。

## 一、先记住更新链路

你现在的日常更新流程是：

1. 本地改代码
2. 提交到 GitHub
3. GitHub Actions 自动构建镜像
4. GitHub Actions 自动发布到 ECS
5. 你做上线验证

所以以后不是“重新部署项目”，而是“重复走同一条发布流水线”。

## 二、最常用的 3 条命令

每次改完代码，最常用的是：

```powershell
git add .
git commit -m "你的修改说明"
git push origin main
```

推送到 `main` 后：

- `ci` 会自动跑
- `cd` 也会自动跑

## 三、标准发布动作

### 场景 1：你改了前端页面

比如你改了：

- 文案
- 按钮样式
- 页面布局
- 前端交互

你的动作：

```powershell
git add .
git commit -m "Update frontend UI"
git push origin main
```

然后去 GitHub 看：

- `ci` 是否绿色
- `cd` 是否绿色

最后浏览器打开你的线上域名验证页面。

### 场景 2：你改了后端接口

比如你改了：

- Go 逻辑
- AI 调用逻辑
- 上传逻辑
- 路由处理

你的动作也是一样：

```powershell
git add .
git commit -m "Update backend logic"
git push origin main
```

## 四、每次上线后要检查什么

最少检查这 4 项：

1. 首页能不能打开
2. 能不能发起一次对话
3. `/healthz` 是不是正常
4. GitHub `cd` 工作流是不是绿色

如果你已经用了域名，检查：

```text
https://你的域名
https://你的域名/healthz
```

如果还在用 IP，检查：

```text
http://你的ECS公网IP
http://你的ECS公网IP/healthz
```

## 五、出问题时先看哪里

### 1. 先看 GitHub Actions

路径：

`GitHub -> Actions`

你先判断失败在：

- `ci`
- 还是 `cd`

规则很简单：

- `ci` 失败：一般是代码或构建问题
- `cd` 失败：一般是部署、镜像、服务器、Secret 问题

### 2. 再看服务器容器状态

登录 ECS 后执行：

```bash
cd /opt/opscaptain
set -a; . ./release.env; set +a
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

### 3. 再看容器日志

```bash
cd /opt/opscaptain
set -a; . ./release.env; set +a
docker compose --env-file .env.production -f docker-compose.prod.yml logs -f backend frontend caddy
```

## 六、什么情况下要改 GitHub Secret

不是所有改动都要改 Secret。

只有这些情况才要改：

1. 模型 API Key 变了
2. 域名变了
3. HTTPS 邮箱变了
4. 数据库地址变了
5. 你新增了外部服务

最常改的是：

- `PROD_ENV_FILE`

## 七、改 PROD_ENV_FILE 的标准方法

路径：

`GitHub -> Settings -> Environments -> production -> Secrets`

找到：

`PROD_ENV_FILE`

直接整体替换内容，不要只改半截。

改完后，再重新执行：

`Actions -> cd -> Run workflow`

## 八、最小回滚思路

如果某次上线后页面坏了，不要先慌着改线上机器。

先用最稳的办法回滚：

1. 回到本地代码
2. 切回上一个正常版本
3. 再次 `git push origin main`

也就是说：

上线回滚的核心不是“去服务器上瞎改”，而是“把正确代码重新发一遍”。

## 九、你以后真正要养成的习惯

每次更新都遵守这 5 条：

1. 改动尽量小，不要一次改太多
2. 提交信息写清楚
3. 先看 `ci`，再看 `cd`
4. 上线后做最小验证
5. 出错先看日志，不靠猜

## 十、给你的最短版本记忆法

你以后只要记住这一句：

```text
改代码 -> git push -> 看 Actions -> 打开线上页面验证
```

这就是你现在的日常更新 SOP。
