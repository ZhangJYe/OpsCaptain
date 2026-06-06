# 记忆系统 Redis 持久化设计

> 日期：2026-06-06
> 状态：待评审
> 目标：记忆重启不丢失 + 多实例共享，消除全量写入问题

---

## 一、背景

当前记忆系统默认纯内存模式，重启丢失所有长期记忆。可选的文件持久化采用全量 JSON 序列化（每次变更重写整个文件），I/O 开销大且不支持多实例共享。

## 二、接口改造

### 现有接口（全量）

```go
type LongTermMemoryStore interface {
    Load(ctx context.Context) ([]*MemoryEntry, error)
    Save(ctx context.Context, entries []*MemoryEntry) error
}
```

### 新接口（增量）

```go
type LongTermMemoryStore interface {
    LoadAll(ctx context.Context) ([]*MemoryEntry, error)
    SaveEntry(ctx context.Context, entry *MemoryEntry) error
    DeleteEntry(ctx context.Context, id string) error
}
```

- `LoadAll`：启动时全量加载，填充内存 map
- `SaveEntry`：单条写入/更新（新增或 reinforce 时调用）
- `DeleteEntry`：单条删除（淘汰、过期、disable 时调用）

## 三、Redis 实现

### Key 设计

| Key | 类型 | 说明 |
|-----|------|------|
| `opscaptionai:memory:entry:{id}` | String (JSON) | 单条记忆 |
| `opscaptionai:memory:ids` | Set | 所有记忆 ID 索引 |

### 读写流程

```
启动 → LoadAll()
  SMEMBERS opscaptionai:memory:ids → 拿到所有 ID
  逐条 GET entry:{id} → 反序列化 → 填充内存 map

新增/更新 → SaveEntry(entry)
  SET entry:{id} = JSON(entry)  → 写单条
  SADD ids {id}                 → 加入索引

删除 → DeleteEntry(id)
  DEL entry:{id}                → 删单条
  SREM ids {id}                 → 从索引移除
```

- 不设 Redis TTL（记忆靠 relevance 衰减淘汰，不靠 Redis 过期）
- 序列化：`json.Marshal`/`json.Unmarshal`，与现有格式兼容

## 四、LongTermMemory 内部改造

### 读路径（不变）

```
RetrieveScoped → 内存 map 扫描 + BM25 匹配 + 排序 + 去重
```

内存仍是热缓存，读取性能不受影响。

### 写路径（改为增量）

```
改前：Store → 内存 map 写入 → persistLocked() 全量序列化
改后：Store → 内存 map 写入 → store.SaveEntry(ctx, entry) 单条 Redis

改前：Delete → 内存 map 删除 → persistLocked() 全量序列化
改后：Delete → 内存 map 删除 → store.DeleteEntry(ctx, id) 单条 Redis
```

### 删除

- `persistLocked()` 函数（全量写入逻辑）
- `fileLongTermMemoryStore` 改为增量 JSONL 实现（保留兼容）

## 五、配置

```yaml
memory:
  store_backend: "redis"    # redis / file / 空=纯内存
```

### 初始化逻辑

```
switch store_backend:
  "redis" → newRedisMemoryStore(g.Redis()) → LoadAll 填充内存
  "file"  → newFileMemoryStore(path)       → LoadAll 填充内存
  ""      → nil                            → 纯内存（默认）
```

### 向后兼容

- `store_backend` 为空 → 纯内存模式（现有行为不变）
- 旧配置 `long_term_store_path` 有值 → 自动走 file 后端
- 新配置 `store_backend: "redis"` → 走 Redis

## 六、改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/ai/memory/long_term.go` | 接口改造、删除 persistLocked、写路径改增量 |
| `internal/ai/memory/redis_store.go` | 新文件，Redis 实现 |
| `internal/ai/memory/file_store.go` | 从 long_term.go 拆出，改为增量 JSONL |
| `internal/ai/memory/long_term_test.go` | 适配新接口 |
| `manifest/config/config.yaml` | 新增 store_backend 配置 |

## 七、不改的部分

- 读路径（RetrieveScoped）不变
- 内存 map 热缓存机制不变
- BM25 检索、Jaccard 去重、relevance 衰减不变
- Session Memory（短期记忆）不变
- Memory Agent 提取逻辑不变
- 不新增配置开关（只有一个 store_backend）
