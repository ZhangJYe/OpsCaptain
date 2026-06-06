# 记忆系统 Redis 持久化设计

> 日期：2026-06-06
> 状态：待评审
> 目标：记忆重启不丢失 + 多实例实时共享，消除全量写入问题

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

### 新接口（增量 + 批量 + 读穿透）

```go
type LongTermMemoryStore interface {
    // 启动时全量加载
    LoadAll(ctx context.Context) ([]*MemoryEntry, error)
    // 读穿透：内存 miss 时从存储加载单条
    LoadEntry(ctx context.Context, id string) (*MemoryEntry, error)
    // 批量写入/更新（原子操作）
    SaveEntries(ctx context.Context, entries []*MemoryEntry) error
    // 批量删除（原子操作）
    DeleteEntries(ctx context.Context, ids []string) error
}
```

- `LoadAll`：启动时全量加载，填充内存 map
- `LoadEntry`：读穿透，内存 miss 时从 Redis 加载单条，实现多实例实时共享
- `SaveEntries`：批量写入，覆盖 Store + reinforce + retire 冲突记忆等场景
- `DeleteEntries`：批量删除，覆盖 evict + Forget 等场景

### 为什么需要批量

`StoreWithOptions` 的完整写路径：

```
StoreWithOptions
  ├── 内存 map 写入新 entry           → SaveEntries([entry])
  ├── retireConflictingMemories       → SaveEntries([retired entries...])
  └── evictIfNeeded                   → DeleteEntries([evicted ids...])
```

单条接口会导致 retire/evict 时多次 Redis 往返，批量接口一次 pipeline 搞定。

## 三、Redis 实现

### Key 设计

| Key | 类型 | 说明 |
|-----|------|------|
| `opscaptionai:memory:entry:{id}` | String (JSON) | 单条记忆 |
| `opscaptionai:memory:ids` | Set | 所有记忆 ID 索引 |

### 原子性保证

使用 Redis Pipeline（MULTI/EXEC）保证每批操作原子：

```
SaveEntries(entries):
  PIPELINE
    FOR each entry:
      SET entry:{id} = JSON(entry)
    SADD ids {id1} {id2} ...
  EXEC

DeleteEntries(ids):
  PIPELINE
    FOR each id:
      DEL entry:{id}
    SREM ids {id1} {id2} ...
  EXEC
```

### 读穿透（多实例共享）

```
RetrieveScoped → 内存 map 查找
  ├── 命中 → 直接用
  └── miss → store.LoadEntry(ctx, id) → 从 Redis 加载 → 放入内存 map → 使用
```

实现多实例实时共享：A 实例写入 Redis 后，B 实例下次读 miss 时自动从 Redis 加载。

### LoadAll 容错

启动时 `SMEMBERS` 拿到 ID 列表后，逐条 `GET`。单条失败（数据损坏/缺失）跳过并打 warning 日志，不阻塞启动。

### 序列化

`json.Marshal`/`json.Unmarshal`，与现有格式兼容。Redis 不设 TTL（记忆靠 relevance 衰减淘汰）。

## 四、LongTermMemory 内部改造

### persistLocked 改造（不删除）

保留 `persistLocked` 作为统一持久化出口，但改为 delta 语义：

```go
// 改前：全量序列化
func (ltm *LongTermMemory) persistLocked(ctx context.Context) {
    if ltm.store != nil {
        entries := make([]*MemoryEntry, 0, len(ltm.entries))
        for _, e := range ltm.entries { entries = append(entries, e) }
        ltm.store.Save(ctx, entries)  // 全量写
    }
}

// 改后：delta 持久化
type pendingChanges struct {
    upserts []*MemoryEntry
    deletes []string
}

func (ltm *LongTermMemory) persistChangesLocked(ctx context.Context, changes pendingChanges) {
    if ltm.store == nil { return }
    if len(changes.upserts) > 0 {
        ltm.store.SaveEntries(ctx, changes.upserts)
    }
    if len(changes.deletes) > 0 {
        ltm.store.DeleteEntries(ctx, changes.deletes)
    }
}
```

每个写路径收集 delta，最后统一调用 `persistChangesLocked`：

```
StoreWithOptions:
  changes.upserts = append(changes.upserts, newEntry)
  changes.upserts = append(changes.upserts, retiredEntries...)
  changes.deletes = append(changes.deletes, evictedIDs...)
  persistChangesLocked(ctx, changes)

Delete/Disable:
  changes.deletes = append(changes.deletes, id)
  persistChangesLocked(ctx, changes)

Forget:
  changes.deletes = append(changes.deletes, removedIDs...)
  persistChangesLocked(ctx, changes)
```

### 读路径改造

```
改前：RetrieveScoped → 纯内存 map 扫描
改后：RetrieveScoped → 内存 map 扫描
      ├── entry 在内存 → 直接用
      └── entry 不在内存 → store.LoadEntry(ctx, id) → 放入 map → 使用
```

实际上 RetrieveScoped 遍历的是 `ltm.entries`（内存 map），不会 miss。
真正需要读穿透的是 `StoreWithOptions` 中的 reinforce 路径（按 ID 查已有 entry）——
但这个 ID 是 content-addressed（SHA256），本地一定有，所以读穿透的实际触发场景是：

- 多实例部署：A 存了一条记忆，B 还没 LoadAll
- 解决：在 `StoreWithOptions` 的 reinforce 路径加 `LoadEntry` fallback

## 五、文件后端兼容

### JSONL 格式

```
每行一条操作：
{"op":"upsert","entry":{...}}
{"op":"delete","id":"abc123"}
```

### 旧 JSON array 迁移

`LoadAll` 时检测文件格式：
- 以 `[` 开头 → 旧 JSON array 格式，直接 `json.Unmarshal` 读取
- 否则 → JSONL 格式，逐行读取
- 读取后自动迁移到 JSONL 格式（首次写入时）

### JSONL Compaction

当 delete 操作累计超过 500 条时，触发 compaction：
- 读取所有有效 entry（跳过 tombstone）
- 重写 JSONL 文件
- 这个频率很低（记忆淘汰本身不频繁）

## 六、配置

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

## 七、改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/ai/memory/long_term.go` | 接口改造、persistLocked → persistChangesLocked、读穿透 |
| `internal/ai/memory/redis_store.go` | 新文件，Redis 实现（pipeline 原子写入） |
| `internal/ai/memory/file_store.go` | 从 long_term.go 拆出，改为 JSONL + 兼容旧格式 |
| `internal/ai/memory/long_term_test.go` | 适配新接口 |
| `manifest/config/config.yaml` | 新增 store_backend 配置 |

## 八、不改的部分

- 内存 map 热缓存机制不变
- BM25 检索、Jaccard 去重、relevance 衰减不变
- Session Memory（短期记忆）不变
- Memory Agent 提取逻辑不变
- 不新增配置开关（只有一个 store_backend）
