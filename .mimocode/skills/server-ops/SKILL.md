---
name: server-ops
description: "OpsCaption 服务器运维操作：SSH 远程检查容器状态、部署验证、日志监控、API 测试"
---

# Server Ops Skill

通过 SSH 连接 OpsCaption 服务器执行运维操作。

## 服务器信息

- 地址：`root@124.222.57.178`
- 部署目录：`/opt/opscaptain`
- 容器命名：`opscaptain-{service}-1`（如 `opscaptain-backend-1`、`opscaptain-frontend-1`）
- Backend 绑定：`127.0.0.1:8000`
- 容器管理：`docker compose` 为唯一方式，不手动 `docker run`

## 常用操作

### 容器状态检查

```bash
# 检查所有容器状态
ssh root@124.222.57.178 "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"

# 检查特定容器健康状态
ssh root@124.222.57.178 "docker inspect opscaptain-backend-1 --format '{{.State.Health.Status}}'"

# 检查容器资源使用
ssh root@124.222.57.178 "docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}'"
```

### 日志查看

```bash
# 最近 N 分钟日志
ssh root@124.222.57.178 "docker logs --since=5m opscaptain-backend-1 2>&1 | tail -50"

# 搜索特定关键词
ssh root@124.222.57.178 "docker logs opscaptain-backend-1 2>&1 | grep -i 'error\|feishu\|aiops' | tail -20"
```

### API 测试

```bash
# 健康检查
ssh root@124.222.57.178 "curl -s http://127.0.0.1:8000/healthz"

# 测试变更事件 API
ssh root@124.222.57.178 'curl -s -X POST http://127.0.0.1:8000/api/change_events \
  -H "Content-Type: application/json" \
  -d '\''{"event_type":"deploy","service":"test","environment":"prod"}'\'''
```

### 部署验证

```bash
# 验证镜像版本
ssh root@124.222.57.178 "docker inspect opscaptain-backend-1 --format '{{.Config.Image}}'"

# 检查配置挂载
ssh root@124.222.57.178 "docker exec opscaptain-backend-1 cat /app/config.yaml 2>/dev/null | head -20"

# 验证 Milvus 连接
ssh root@124.222.57.178 "docker exec opscaptain-milvus-1 curl -s 'http://localhost:19530/v2/vectordb/collections/get_stats' \
  -X POST -H 'Content-Type: application/json' \
  -d '{\"collectionName\":\"aiops_evidence_build\"}'"
```

### 知识库索引

```bash
# 上传文件到容器
scp /tmp/data.tar.gz root@124.222.57.178:/tmp/
ssh root@124.222.57.178 "docker cp /tmp/data.tar.gz opscaptain-backend-1:/tmp/"

# 运行知识索引器
ssh root@124.222.57.178 "docker exec opscaptain-backend-1 /app/knowledge-indexer -dir /tmp/md_output/directory"

# 监控索引进度
ssh root@124.222.57.178 "docker logs --since=1m opscaptain-backend-1 2>&1 | grep -i 'index\|embed\|error' | tail -20"
```

## 注意事项

- 本地 Mac (arm64) `docker build` 生成 arm64 镜像，服务器 (amd64) 无法运行，应通过 CD 流水线构建
- config.prod.yaml 通过 volume mount 到 backend 容器
- 代码变更需要重新构建镜像 + 更新 compose 才能生效
- GoFrame `${VAR}` 不自动解析环境变量，需用 `common.ResolveEnv()` 包装
