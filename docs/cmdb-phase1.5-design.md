# CMDB Phase 1.5: 写 API 设计方案

> 目标：让运维人员通过 API 管理服务资产（新增/更新/删除），不用手动编辑 YAML。
> 约束：YAML 原子写保护、schema 校验、多 Pod 不保证一致。

---

## 1. 新增 API

| Method | Path | 说明 |
|--------|------|------|
| POST | `/api/cmdb/services` | 新增服务 |
| PUT | `/api/cmdb/services/{name}` | 更新服务（全量替换） |
| DELETE | `/api/cmdb/services/{name}` | 删除服务 |

## 2. 并发写保护

YAML 文件写入使用 **temp file + fsync + rename** 原子写模式：

```
1. 写入临时文件 /path/to/services.yaml.tmp
2. 调用 file.Sync() 确保数据落盘
3. 调用 os.Rename() 原子替换目标文件
```

同时使用 `sync.Mutex` 保护写操作串行化。

## 3. Schema 校验

必填字段：`name`, `owner`, `team`, `cluster`, `env`
唯一性约束：`name` 不能重复
name 格式：小写英文、数字、连字符，最长 128 字符

## 4. 多 Pod 注意事项

- MVP 只读 + 写 API，YAML 文件在本地磁盘
- 多 Pod 部署时，每个实例看到的 YAML 独立
- 写操作只影响当前实例，不保证跨实例一致
- 生产环境建议：单实例管理 CMDB，其他实例只读

## 5. 响应格式

所有写操作返回统一格式：
```json
{
  "success": true,
  "service": { ... },
  "message": "服务创建成功"
}
```

错误时：
```json
{
  "success": false,
  "error": "validation failed: name is required",
  "message": "请提供必填字段"
}
```
