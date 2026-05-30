# OpsCaption 部署手册

> 线上环境信息和常用验证命令。

---

## 环境信息

| 项目 | 值 |
|------|-----|
| 线上服务器 | `124.222.57.178` |
| 线上域名 | `https://opscaptain.top/ai/` |
| SSH 用户 | `root@124.222.57.178` |
| 部署目录 | `/opt/opscaptain` |
| 生产编排 | `docker-compose.prod.yml` + `.env.production` + `release.env` |

注意：执行 compose 命令前需要加载 `release.env`，否则 `BACKEND_IMAGE` / `FRONTEND_IMAGE` 变量会缺失。

---

## 常用只读验证命令

```bash
ssh root@124.222.57.178
cd /opt/opscaptain
set -a; . ./release.env; set +a
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl -sS http://127.0.0.1/ai/healthz
curl -sS http://127.0.0.1/ai/readyz
docker logs --since=10m opscaptain-backend-1
docker logs --since=10m opscaptain-caddy-1
```

## 对外健康检查

```bash
curl -k https://opscaptain.top/ai/healthz
curl -k https://opscaptain.top/ai/readyz
```
